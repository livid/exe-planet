package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/kubo"
	"github.com/livid/exe-planet/internal/kubo/kubotest"
	"github.com/livid/exe-planet/internal/service"
	"github.com/livid/exe-planet/internal/site"
)

// TestNodeRoutes drives publishing through the API with a fake exe and a
// fake Kubo: expose puts a route in exe and the site answers under its
// hostname at the root, IPFS lends Kubo a key and publishes, export is a
// zip, and deleting the site takes the route down.
func TestNodeRoutes(t *testing.T) {
	root := t.TempDir()
	store := site.New(filepath.Join(root, "Planet"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	shipped, _ := filepath.Abs(filepath.Join("..", "..", "templates"))
	if err := service.InstallTemplates(shipped, filepath.Join(store.Root, site.TemplatesDir)); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	routes := map[string]string{}
	exeMux := http.NewServeMux()
	exeMux.HandleFunc("POST /v1/routes", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Host, Backend string }
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		routes[req.Host] = req.Backend
		mu.Unlock()
		w.Write([]byte(`{"host":"` + req.Host + `","backend":"` + req.Backend + `","url":"https://` + req.Host + `"}`))
	})
	exeMux.HandleFunc("DELETE /v1/routes/{host}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		delete(routes, r.PathValue("host"))
		mu.Unlock()
		w.Write([]byte(`{"status":"removed"}`))
	})
	exeSrv := httptest.NewServer(exeMux)
	t.Cleanup(exeSrv.Close)
	fk := kubotest.New(t)

	svc := service.New(store, filepath.Join(root, "state"), time.UTC)
	svc.Exe = &service.Exe{URL: exeSrv.URL, Domain: "example.test", HTTP: exeSrv.Client()}
	svc.Kubo = kubo.New(fk.URL())
	svc.Backend, svc.Version = "http://127.0.0.1:7798", "test"
	svc.Start()
	s := &Server{Store: store, Builder: svc, Templates: svc.Templates, Version: "test", Started: time.Now(), Generation: func() uint64 { return 1 }}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	c := &client{t: t, srv: srv}

	c.do("POST", "/v1/sites", map[string]any{"title": "Notes", "slug": "notes", "template": "plain"})
	c.do("POST", "/v1/sites/notes/posts", map[string]any{"title": "Hello", "slug": "hello", "body": "hi\n"})
	if code, _ := c.do("POST", "/v1/sites/notes/build", nil); code != 200 {
		t.Fatalf("build %d", code)
	}

	code, caps := c.do("GET", "/v1/node", nil)
	if code != 200 || caps["domain"] != "example.test" || caps["exe"] != true || caps["kubo"] != true || caps["kubo_version"] != "0.43.0" {
		t.Fatalf("caps %d %v", code, caps)
	}
	code, n := c.do("GET", "/v1/sites/notes/node", nil)
	if code != 200 || n["expose"] != false || n["ipfs"] != false || n["key"] != false {
		t.Fatalf("node %d %v", code, n)
	}

	// expose: the route lands in exe and the hostname serves the site
	code, n = c.do("PUT", "/v1/sites/notes/node", map[string]any{"expose": true})
	if code != 200 || n["hostname"] != "notes.example.test" || n["url"] != "https://notes.example.test/" {
		t.Fatalf("expose %d %v", code, n)
	}
	mu.Lock()
	if routes["notes.example.test"] != "http://127.0.0.1:7798" {
		t.Fatalf("exe routes %v", routes)
	}
	mu.Unlock()
	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.Host = "notes.example.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Notes") || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("host root: %d %s %q", resp.StatusCode, resp.Header.Get("Content-Type"), body[:min(len(body), 120)])
	}
	req, _ = http.NewRequest("GET", srv.URL+"/planet.json", nil)
	req.Host = "notes.example.test:443"
	resp, _ = http.DefaultClient.Do(req)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"name" : "Notes"`) {
		t.Fatalf("host planet.json: %d %s", resp.StatusCode, body)
	}
	// the API is still the API under its own host
	if code, _ := c.do("GET", "/v1/status", nil); code != 200 {
		t.Fatalf("status under the daemon's host %d", code)
	}
	if code, _ := c.do("PUT", "/v1/sites/notes/node", map[string]any{"subdomain": "No Good"}); code != 400 {
		t.Fatalf("bad subdomain %d", code)
	}

	// IPFS: a key, a name in site.json, a publish
	code, n = c.do("PUT", "/v1/sites/notes/node", map[string]any{"ipfs": true})
	if code != 200 || n["ipfs"] != true || n["key"] != true || !strings.HasPrefix(n["ipns"].(string), "k51") {
		t.Fatalf("ipfs %d %v", code, n)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, n = c.do("GET", "/v1/sites/notes/node", nil)
		if n["publishing"] == false && n["cid"] != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never published: %v", n)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n["cid_url"] != "https://eth.sucks/ipfs/"+n["cid"].(string)+"/" || n["ipns_url"] != "https://eth.sucks/ipns/"+n["ipns"].(string)+"/" {
		t.Fatalf("urls %v", n)
	}
	_, st := c.do("GET", "/v1/sites/notes", nil)
	if st["ipns"] != n["ipns"] {
		t.Fatalf("site.json ipns %v vs %v", st["ipns"], n["ipns"])
	}
	if code, _ := c.do("POST", "/v1/sites/notes/publish", nil); code != 202 {
		t.Fatalf("publish now %d", code)
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		_, _, pubs, _ := fk.Snapshot()
		if len(pubs) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("publish now never ran")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// export is a zip named after the site
	resp, _ = http.Get(srv.URL + "/v1/sites/notes/export")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/zip" || !strings.Contains(resp.Header.Get("Content-Disposition"), `notes.zip`) || !strings.HasPrefix(string(body), "PK") {
		t.Fatalf("export %d %s %d bytes", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}

	// deleting the site takes the route down
	if code, _ := c.do("DELETE", "/v1/sites/notes", nil); code != 200 {
		t.Fatalf("delete %d", code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(routes) != 0 {
		t.Fatalf("routes after delete %v", routes)
	}
}
