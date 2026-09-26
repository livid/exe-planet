package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/kubo"
	"github.com/livid/exe-planet/internal/kubo/kubotest"
	"github.com/livid/exe-planet/internal/site"
)

// fakeExe records the routes exe would put in front of the daemon.
type fakeExe struct {
	mu     sync.Mutex
	routes map[string]string
	calls  []string
	auth   string
	fail   bool
	// what the node signed for a hub through exe (POST /v1/hub/publish)
	hubOps  []fakeHubOp
	hubFail string
}

type fakeHubOp struct {
	Hub, Type string
	Body      map[string]string
}

func newFakeExe(t *testing.T) (*fakeExe, *Exe) {
	f := &fakeExe{routes: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/routes", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = r.Header.Get("Authorization")
		var req struct{ Host, Backend string }
		json.NewDecoder(r.Body).Decode(&req)
		f.calls = append(f.calls, "add "+req.Host+" "+req.Backend)
		if f.fail {
			http.Error(w, `{"error":"cloudflare said no"}`, 500)
			return
		}
		f.routes[req.Host] = req.Backend
		w.Write([]byte(`{"host":"` + req.Host + `","backend":"` + req.Backend + `","url":"https://` + req.Host + `","dns":"ok","warnings":["advertise_host not set; skipped tunnel ingress update"]}`))
	})
	mux.HandleFunc("DELETE /v1/routes/{host}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, "del "+r.PathValue("host"))
		delete(f.routes, r.PathValue("host"))
		w.Write([]byte(`{"status":"removed"}`))
	})
	mux.HandleFunc("POST /v1/hub/publish", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var op fakeHubOp
		json.NewDecoder(r.Body).Decode(&op)
		f.calls = append(f.calls, "hub "+op.Type)
		if f.hubFail != "" {
			http.Error(w, `{"error":"`+f.hubFail+`"}`, 403)
			return
		}
		f.hubOps = append(f.hubOps, op)
		w.Write([]byte(`{"id":"x","status":"accepted"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, &Exe{URL: srv.URL, Token: "T", Domain: "example.test", HTTP: srv.Client()}
}

func newNodeService(t *testing.T) (*Service, *site.Store, *fakeExe, *kubotest.Fake) {
	svc, store := newService(t)
	fe, exe := newFakeExe(t)
	fk := kubotest.New(t)
	svc.Exe, svc.Kubo, svc.Backend, svc.Version = exe, kubo.New(fk.URL()), "http://127.0.0.1:7798", "test"
	svc.Start()
	return svc, store, fe, fk
}

func waitPublished(t *testing.T, svc *Service, slug string, pubs int, fk *kubotest.Fake) NodeInfo {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		n, err := svc.Node(slug)
		if err != nil {
			t.Fatal(err)
		}
		_, _, p, _ := fk.Snapshot()
		if !n.Publishing && len(p) >= pubs {
			return n
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no publish for %s", slug)
	return NodeInfo{}
}

func TestExpose(t *testing.T) {
	svc, store, fe, _ := newNodeService(t)
	store.CreateSite("notes", "Notes", "", "plain")
	store.CreatePost("notes", site.KindPost, "hello", site.Meta{Title: "Hello"}, "hi\n")
	if _, err := svc.Build("notes"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	on := true
	n, err := svc.SetNode(ctx, "notes", NodeUpdate{Expose: &on})
	if err != nil {
		t.Fatal(err)
	}
	if !n.Expose || n.Hostname != "notes.example.test" || n.URL != "https://notes.example.test/" || len(n.RouteWarnings) != 1 {
		t.Fatalf("expose: %+v", n)
	}
	if fe.routes["notes.example.test"] != "http://127.0.0.1:7798" || fe.auth != "Bearer T" {
		t.Fatalf("exe routes %v auth %q", fe.routes, fe.auth)
	}
	if dir := svc.HostDir("notes.example.test:443"); dir == "" || !strings.HasSuffix(dir, "public") {
		t.Fatalf("host dir %q", dir)
	}
	// the rebuild that follows carries the hostname into the feed
	rss, _ := os.ReadFile(filepath.Join(svc.HostDir("notes.example.test"), "rss.xml"))
	hello, _ := store.Post("notes", "hello", "")
	if !strings.Contains(string(rss), "<link>https://notes.example.test/</link>") || !strings.Contains(string(rss), "<link>https://notes.example.test/"+hello.ID+"/</link>") {
		t.Fatalf("rss after expose:\n%.600s", rss)
	}
	if svc.HostDir("other.example.test") != "" {
		t.Fatal("a stranger's host is served")
	}
	// the node file is what a restart reads
	svc2 := New(store, svc.State, svc.Location)
	svc2.Start()
	if svc2.HostDir("Notes.example.test") == "" {
		t.Fatal("hosts not reloaded")
	}
	// another subdomain moves the route
	sub := "blog"
	n, err = svc.SetNode(ctx, "notes", NodeUpdate{Subdomain: &sub})
	if err != nil {
		t.Fatal(err)
	}
	if n.Hostname != "blog.example.test" || fe.routes["notes.example.test"] != "" || fe.routes["blog.example.test"] == "" {
		t.Fatalf("move: %+v %v", n, fe.routes)
	}
	if svc.HostDir("notes.example.test") != "" || svc.HostDir("blog.example.test") == "" {
		t.Fatal("hosts did not move")
	}
	bad := "Not Valid"
	if _, err := svc.SetNode(ctx, "notes", NodeUpdate{Subdomain: &bad}); err == nil {
		t.Fatal("a bad subdomain was taken")
	}
	// off removes the route, and the feed goes back to the gateway form
	off := false
	n, err = svc.SetNode(ctx, "notes", NodeUpdate{Expose: &off})
	if err != nil || n.Expose || n.Hostname != "" || len(fe.routes) != 0 || n.Subdomain != "blog" {
		t.Fatalf("off: %+v %v %v", n, fe.routes, err)
	}
	st0, _ := store.Site("notes")
	rss, _ = os.ReadFile(filepath.Join(svc.PublicDir(st0.ID), "rss.xml"))
	if strings.Contains(string(rss), "example.test") {
		t.Fatalf("rss after off still carries the hostname:\n%.400s", rss)
	}
	// exe refusing leaves expose off
	fe.fail = true
	if _, err := svc.SetNode(ctx, "notes", NodeUpdate{Expose: &on}); err == nil || !strings.Contains(err.Error(), "cloudflare said no") {
		t.Fatalf("exe failure: %v", err)
	}
	if n, _ := svc.Node("notes"); n.Expose || n.Hostname != "" {
		t.Fatalf("after failure: %+v", n)
	}
	fe.fail = false
	// retire takes the route with it
	svc.SetNode(ctx, "notes", NodeUpdate{Expose: &on})
	st, _ := store.Site("notes")
	svc.Retire(ctx, "notes", st.ID)
	if len(fe.routes) != 0 || svc.HostDir("blog.example.test") != "" {
		t.Fatalf("retire: %v", fe.routes)
	}
	if n, _ := svc.Node("notes"); n.Expose || n.Hostname != "" {
		t.Fatalf("after retire: %+v", n)
	}
	// a node without exe says so
	svc.Exe = nil
	if _, err := svc.SetNode(ctx, "notes", NodeUpdate{Expose: &on}); err == nil || !strings.Contains(err.Error(), "exe is not configured") {
		t.Fatalf("no exe: %v", err)
	}
}

func TestIPFS(t *testing.T) {
	svc, store, _, fk := newNodeService(t)
	store.CreateSite("notes", "Notes", "", "plain")
	store.CreatePost("notes", site.KindPost, "hello", site.Meta{Title: "Hello"}, "hi\n")
	if _, err := svc.Build("notes"); err != nil {
		t.Fatal(err)
	}
	_, _, pubs, _ := fk.Snapshot()
	if len(pubs) != 0 {
		t.Fatal("published before IPFS was on")
	}
	ctx := context.Background()
	on := true
	n, err := svc.SetNode(ctx, "notes", NodeUpdate{IPFS: &on})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := store.Site("notes")
	if !n.IPFS || !n.Key || n.IPNS == "" || !strings.HasPrefix(n.IPNS, "k51qzi5uqu5d") || n.IPNSURL != "https://eth.sucks/ipns/"+n.IPNS+"/" {
		t.Fatalf("ipfs on: %+v", n)
	}
	if fi, err := os.Stat(svc.keyPath(st.ID)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v", fi, err)
	}
	if st.IPNS != n.IPNS {
		t.Fatalf("site.json ipns %q, node %q", st.IPNS, n.IPNS)
	}
	keys, _, _, _ := fk.Snapshot()
	if keys["exe-planet-"+st.ID] == "" {
		t.Fatalf("kubo keys %v", keys)
	}
	// the rebuild that carries the name published once
	n = waitPublished(t, svc, "notes", 1, fk)
	_, adds, pubs, pins := fk.Snapshot()
	if n.CID == "" || n.PublishedHash == "" || n.PublishedHash != n.BuiltHash || n.PublishError != "" || n.CIDURL != "https://eth.sucks/ipfs/"+n.CID+"/" {
		t.Fatalf("published: %+v", n)
	}
	if len(adds) != 1 || !strings.HasPrefix(adds[0], "public public/") || pubs[0] != "exe-planet-"+st.ID+"="+n.CID || !pins[n.CID] {
		t.Fatalf("kubo calls: adds %v pubs %v pins %v", adds, pubs, pins)
	}
	pj, _ := os.ReadFile(filepath.Join(svc.PublicDir(st.ID), "planet.json"))
	if !strings.Contains(string(pj), `"ipns" : "`+n.IPNS+`"`) {
		t.Fatalf("the rebuilt planet.json does not carry the IPNS name:\n%s", pj)
	}
	// the same content again: no second publish
	if _, err := svc.Build("notes"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, _, pubs, _ := fk.Snapshot(); len(pubs) != 1 {
		t.Fatalf("an unchanged build published: %v", pubs)
	}
	// a change publishes and lets the old CID go
	first := n.CID
	body := "changed\n"
	store.UpdatePost("notes", "hello", "", site.PostUpdate{Body: &body})
	if _, err := svc.Build("notes"); err != nil {
		t.Fatal(err)
	}
	n = waitPublished(t, svc, "notes", 2, fk)
	_, _, pubs, pins = fk.Snapshot()
	if n.CID == first || len(pubs) != 2 || pins[first] || !pins[n.CID] {
		t.Fatalf("second publish: %+v pubs %v pins %v", n, pubs, pins)
	}
	// publish now goes again, changed or not
	if _, err := svc.Publish("notes"); err != nil {
		t.Fatal(err)
	}
	waitPublished(t, svc, "notes", 3, fk)
	// a Kubo failure is reported and does not lose the last CID
	fk.Fail["name/publish"] = "routing: not found"
	svc.Publish("notes")
	deadline := time.Now().Add(5 * time.Second)
	for n.PublishError == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		n, _ = svc.Node("notes")
	}
	if !strings.Contains(n.PublishError, "routing: not found") || n.CID == "" {
		t.Fatalf("failed publish: %+v", n)
	}
	delete(fk.Fail, "name/publish")
	// off keeps the key and the name; on again reuses them, no new key
	off := false
	n, _ = svc.SetNode(ctx, "notes", NodeUpdate{IPFS: &off})
	if n.IPFS || !n.Key || n.IPNS == "" {
		t.Fatalf("off: %+v", n)
	}
	n2, err := svc.SetNode(ctx, "notes", NodeUpdate{IPFS: &on})
	if err != nil || n2.IPNS != n.IPNS {
		t.Fatalf("on again: %+v %v", n2, err)
	}
	if keys, _, _, _ := fk.Snapshot(); len(keys) != 2 {
		t.Fatalf("keys after on again: %v", keys)
	}
	// export carries the folder, the key and the node file
	var buf bytes.Buffer
	if err := svc.Export("notes", &buf); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"notes/site.json", "notes/posts/hello/index.md", "notes.ipns.key", "notes.node.json"} {
		if !names[want] {
			t.Errorf("export lacks %s: %v", want, names)
		}
	}
	// a node without Kubo says so
	svc.Kubo = nil
	store.CreateSite("other", "Other", "", "plain")
	if _, err := svc.SetNode(ctx, "other", NodeUpdate{IPFS: &on}); err == nil || !strings.Contains(err.Error(), "kubo is not configured") {
		t.Fatalf("no kubo: %v", err)
	}
	caps := svc.Caps(ctx)
	if caps.Kubo || !caps.Exe || caps.Domain != "example.test" || caps.Backend != "http://127.0.0.1:7798" {
		t.Fatalf("caps: %+v", caps)
	}
}
