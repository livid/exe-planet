package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/service"
	"github.com/livid/exe-planet/internal/site"
)

type client struct {
	t   *testing.T
	srv *httptest.Server
}

// newClient serves a store in a temporary folder of its own.
func newClient(t *testing.T) (*client, *site.Store) {
	t.Helper()
	store := site.New(filepath.Join(t.TempDir(), "Planet"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: store, Version: "test", Started: time.Now(), Generation: func() uint64 { return 7 }}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return &client{t: t, srv: srv}, store
}

// do sends JSON and decodes the answer; a nil body sends nothing.
func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.srv.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 && raw[0] == '{' {
		json.Unmarshal(raw, &out)
	} else if len(raw) > 0 && raw[0] == '[' {
		var list []any
		json.Unmarshal(raw, &list)
		out = map[string]any{"list": list}
	}
	return resp.StatusCode, out
}

func TestSitesAPI(t *testing.T) {
	c, store := newClient(t)
	code, res := c.do("GET", "/v1/status", nil)
	if code != 200 || res["version"] != "test" || res["generation"].(float64) != 7 || res["sites"].(float64) != 0 {
		t.Fatalf("status %d %v", code, res)
	}
	code, res = c.do("POST", "/v1/sites", map[string]any{"title": "Charts & Thoughts", "about": "graphs"})
	if code != 201 || res["slug"] != "charts-thoughts" || res["template"] != "plain" || res["id"] == "" {
		t.Fatalf("create %d %v", code, res)
	}
	code, res = c.do("POST", "/v1/sites", map[string]any{"title": "数据", "slug": "data", "template": "sepia"})
	if code != 201 || res["slug"] != "data" || res["template"] != "sepia" {
		t.Fatalf("create with slug %d %v", code, res)
	}
	for _, body := range []map[string]any{
		{"title": "数据"},
		{"title": "X", "slug": "Templates"},
		{"title": "X", "slug": "bad slug"},
		{"title": "", "slug": "ok"},
		{"title": "X", "slug": "data"},
		{"title": "X", "slug": "ok", "extra": 1},
	} {
		code, res := c.do("POST", "/v1/sites", body)
		want := 400
		if body["slug"] == "data" {
			want = 409
		}
		if code != want {
			t.Errorf("%v: %d %v", body, code, res)
		}
	}
	code, res = c.do("GET", "/v1/sites", nil)
	if code != 200 || len(res["list"].([]any)) != 2 {
		t.Fatalf("list %d %v", code, res)
	}
	code, res = c.do("PUT", "/v1/sites/data", map[string]any{"name": "Data", "social": map[string]string{"github": "livid"}, "tags": map[string]string{"go": "Go"}})
	if code != 200 || res["name"] != "Data" || res["social"].(map[string]any)["github"] != "livid" || res["template"] != "sepia" {
		t.Fatalf("update %d %v", code, res)
	}
	if code, _ := c.do("PUT", "/v1/sites/data", map[string]any{"id": "hack"}); code != 400 {
		t.Fatalf("id change accepted: %d", code)
	}
	if code, _ := c.do("GET", "/v1/sites/nope", nil); code != 404 {
		t.Fatalf("missing site: %d", code)
	}
	os.WriteFile(filepath.Join(store.Root, "data", "site.json"), []byte("{"), 0o644)
	code, res = c.do("GET", "/v1/sites/data", nil)
	if code != 200 || res["error"] == nil {
		t.Fatalf("broken site %d %v", code, res)
	}
	if code, _ := c.do("GET", "/v1/sites/data/posts", nil); code != 400 {
		t.Fatalf("posts of a broken site: %d", code)
	}
	if code, _ := c.do("DELETE", "/v1/sites/data", nil); code != 200 {
		t.Fatalf("delete broken site: %d", code)
	}
	if code, _ := c.do("POST", "/v1/sites/charts-thoughts/archive", nil); code != 200 {
		t.Fatalf("archive: %d", code)
	}
	code, res = c.do("GET", "/v1/sites?archived=1", nil)
	if code != 200 || len(res["list"].([]any)) != 1 {
		t.Fatalf("archived list %d %v", code, res)
	}
	if code, _ := c.do("GET", "/v1/sites", nil); code != 200 {
		t.Fatal("live list")
	}
	if code, _ := c.do("POST", "/v1/sites/charts-thoughts/unarchive", nil); code != 200 {
		t.Fatalf("unarchive: %d", code)
	}
	if code, _ := c.do("POST", "/v1/sites/charts-thoughts/unarchive", nil); code != 404 {
		t.Fatalf("unarchive twice: %d", code)
	}
}

func TestPostsAPI(t *testing.T) {
	c, store := newClient(t)
	c.do("POST", "/v1/sites", map[string]any{"title": "Charts", "slug": "charts"})
	code, res := c.do("POST", "/v1/sites/charts/posts", map[string]any{"title": "Hello, World", "body": "# Hi\n", "tags": []string{"a"}, "kind": "draft"})
	if code != 201 || res["slug"] != "hello-world" || res["kind"] != "draft" || res["id"] == "" || res["body"] != "# Hi\n" {
		t.Fatalf("create %d %v", code, res)
	}
	if code, _ := c.do("POST", "/v1/sites/charts/posts", map[string]any{"title": "Hello World"}); code != 409 {
		t.Fatalf("duplicate slug across kinds: %d", code)
	}
	if code, _ := c.do("POST", "/v1/sites/charts/posts", map[string]any{"title": "你好"}); code != 400 {
		t.Fatalf("no slug: %d", code)
	}
	if code, _ := c.do("POST", "/v1/sites/charts/posts", map[string]any{"title": "X", "kind": "blog"}); code != 400 {
		t.Fatalf("bad kind: %d", code)
	}
	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	code, res = c.do("POST", "/v1/sites/charts/posts", map[string]any{"title": "About", "kind": "page", "created": created})
	if code != 201 || res["kind"] != "page" || !strings.HasPrefix(res["created"].(string), "2020-01-02T03:04:05") {
		t.Fatalf("page %d %v", code, res)
	}

	code, res = c.do("PUT", "/v1/sites/charts/posts/hello-world", map[string]any{"body": "changed", "tags": []string{"x", "y"}})
	if code != 200 || res["body"] != "changed\n" || res["modified"] == nil || res["title"] != "Hello, World" {
		t.Fatalf("update %d %v", code, res)
	}
	if code, _ := c.do("PUT", "/v1/sites/charts/posts/hello-world?kind=post", map[string]any{"body": "x"}); code != 404 {
		t.Fatalf("update in the wrong kind: %d", code)
	}
	if code, res := c.do("PUT", "/v1/sites/charts/posts/hello-world", map[string]any{"title": " "}); code != 200 || res["title"] != "" {
		t.Fatalf("a title may be emptied: %d %v", code, res)
	}
	if code, _ := c.do("PUT", "/v1/sites/charts/posts/hello-world", map[string]any{"title": "Hello, World"}); code != 200 {
		t.Fatalf("title back: %d", code)
	}
	code, res = c.do("POST", "/v1/sites/charts/posts/hello-world/move", map[string]any{"kind": "post"})
	if code != 200 || res["kind"] != "post" {
		t.Fatalf("move %d %v", code, res)
	}
	code, res = c.do("GET", "/v1/sites/charts/posts", nil)
	list := res["list"].([]any)
	if code != 200 || len(list) != 2 || list[0].(map[string]any)["slug"] != "hello-world" || list[0].(map[string]any)["body"] != nil {
		t.Fatalf("list %d %v", code, res)
	}
	code, res = c.do("GET", "/v1/sites/charts/posts?kind=page", nil)
	if code != 200 || len(res["list"].([]any)) != 1 {
		t.Fatalf("pages %d %v", code, res)
	}
	code, res = c.do("GET", "/v1/sites/charts", nil)
	if res["posts"].(float64) != 1 || res["pages"].(float64) != 1 || res["drafts"].(float64) != 0 {
		t.Fatalf("counts %v", res)
	}

	// Attachments: multipart in, bytes out, generated files never served.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "../photo.jpg")
	fw.Write([]byte("jpeg"))
	fw, _ = mw.CreateFormFile("file", "notes.txt")
	fw.Write([]byte("txt"))
	mw.WriteField("other", "ignored")
	mw.Close()
	req, _ := http.NewRequest("POST", c.srv.URL+"/v1/sites/charts/posts/hello-world/attachments", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var up map[string]any
	json.NewDecoder(resp.Body).Decode(&up)
	resp.Body.Close()
	if resp.StatusCode != 200 || len(up["saved"].([]any)) != 2 || len(up["attachments"].([]any)) != 2 {
		t.Fatalf("upload %d %v", resp.StatusCode, up)
	}
	resp, _ = http.Get(c.srv.URL + "/v1/sites/charts/posts/hello-world/attachments/photo.jpg")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != "jpeg" {
		t.Fatalf("serve %d %q", resp.StatusCode, b)
	}
	os.WriteFile(filepath.Join(store.Root, "charts", "posts", "hello-world", "_grid.png"), []byte("x"), 0o644)
	if code, _ := c.do("GET", "/v1/sites/charts/posts/hello-world/attachments/_grid.png", nil); code != 400 {
		t.Fatalf("generated file served: %d", code)
	}
	if code, _ := c.do("DELETE", "/v1/sites/charts/posts/hello-world/attachments/notes.txt", nil); code != 200 {
		t.Fatalf("delete attachment: %d", code)
	}
	if code, _ := c.do("DELETE", "/v1/sites/charts/posts/hello-world/attachments/notes.txt", nil); code != 404 {
		t.Fatalf("delete attachment twice: %d", code)
	}
	req, _ = http.NewRequest("POST", c.srv.URL+"/v1/sites/charts/posts/hello-world/attachments", strings.NewReader("raw"))
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("raw body accepted: %d", resp.StatusCode)
	}

	if code, _ := c.do("DELETE", "/v1/sites/charts/posts/hello-world", nil); code != 200 {
		t.Fatalf("delete post: %d", code)
	}
	if code, _ := c.do("GET", "/v1/sites/charts/posts/hello-world", nil); code != 404 {
		t.Fatalf("deleted post: %d", code)
	}
	if code, _ := c.do("GET", "/v1/nothing", nil); code != 404 {
		t.Fatalf("unknown route: %d", code)
	}
}

// TestTodoAPI: POST …/posts/{post}/todo ticks an item in the post's
// Markdown, answers 409 for a page behind its file and 404 for an item
// that is not there, and writes nothing then.
func TestTodoAPI(t *testing.T) {
	root := t.TempDir()
	store := site.New(filepath.Join(root, "Planet"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	svc := service.New(store, filepath.Join(root, "state"), time.UTC)
	s := &Server{Store: store, Builder: svc, Version: "test", Started: time.Now(), Generation: func() uint64 { return 7 }}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	c := &client{t: t, srv: srv}
	if _, err := store.CreateSite("notes", "Notes", "", ""); err != nil {
		t.Fatal(err)
	}
	p, err := store.CreatePost("notes", site.KindDraft, "week", site.Meta{Title: "Week"}, "- [ ] one\n- [x] two\n")
	if err != nil {
		t.Fatal(err)
	}
	read := func() string { b, _ := os.ReadFile(filepath.Join(p.Dir, "index.md")); return string(b) }
	code, out := c.do("POST", "/v1/sites/notes/posts/week/todo?kind=draft", map[string]any{"item": 1, "done": true, "was": "01"})
	if code != 200 || out["item"] != float64(1) || out["done"] != true || !strings.Contains(read(), "- [x] one\n- [x] two") {
		t.Fatalf("tick item 1: %d %v, %q", code, out, read())
	}
	now := read()
	if code, out = c.do("POST", "/v1/sites/notes/posts/week/todo", map[string]any{"item": 2, "done": false, "was": "01"}); code != 409 || read() != now {
		t.Errorf("a page behind its file: %d %v", code, out)
	}
	if code, _ = c.do("POST", "/v1/sites/notes/posts/week/todo", map[string]any{"item": 3, "done": true, "was": "11"}); code != 404 || read() != now {
		t.Errorf("item 3 of 2: %d", code)
	}
	if code, _ = c.do("POST", "/v1/sites/notes/posts/week/todo?kind=post", map[string]any{"item": 1, "done": false}); code != 404 {
		t.Errorf("the draft asked for as a post: %d", code)
	}
	if code, _ = c.do("POST", "/v1/sites/notes/posts/week/todo", map[string]any{"item": 1, "done": false, "words": "x"}); code != 400 {
		t.Errorf("an unknown field: %d", code)
	}
	if code, _ = c.do("POST", "/v1/sites/notes/posts/week/todo", map[string]any{"item": 2, "done": false, "was": "11"}); code != 200 || !strings.Contains(read(), "- [x] one\n- [ ] two") {
		t.Errorf("untick item 2: %d, %q", code, read())
	}
}

func TestTemplatesAPI(t *testing.T) {
	c, store := newClient(t)
	code, res := c.do("GET", "/v1/templates", nil)
	if code != 200 || len(res["list"].([]any)) != 0 {
		t.Fatalf("empty %d %v", code, res)
	}
	os.MkdirAll(filepath.Join(store.Root, site.TemplatesDir, "plain"), 0o755)
	os.WriteFile(filepath.Join(store.Root, site.TemplatesDir, "plain", "template.json"), []byte(`{"name":"Plain","buildNumber":1}`), 0o644)
	code, res = c.do("GET", "/v1/templates", nil)
	if code != 200 || res["list"].([]any)[0].(map[string]any)["name"] != "Plain" {
		t.Fatalf("listed %d %v", code, res)
	}
}

// Import lists a library and brings a planet in under a slug of ours,
// keeping Planet's id; the site is built when a builder is present.
func TestImportAPI(t *testing.T) {
	c, _ := newClient(t)
	lib := t.TempDir()
	id := "7AB9F0C5-BCAE-4952-A670-18A416D1C968"
	my := filepath.Join(lib, "My", id)
	os.MkdirAll(filepath.Join(my, "Articles"), 0o755)
	os.WriteFile(filepath.Join(my, "planet.json"), []byte(`{"id":"`+id+`","name":"Notes","about":"","ipns":"k51x","created":796032373,"updated":796032373,"templateName":"Sparkle"}`), 0o644)
	os.WriteFile(filepath.Join(my, "Articles", "A.json"), []byte(`{"id":"006662F5-AB90-4795-BB62-08C440DA45EE","title":"Hello","content":"hi","created":675853734,"attachments":[]}`), 0o644)

	code, res := c.do("GET", "/v1/import?library="+lib, nil)
	if code != 200 || len(res["planets"].([]any)) != 1 || res["planets"].([]any)[0].(map[string]any)["name"] != "Notes" {
		t.Fatalf("list %d %v", code, res)
	}
	code, res = c.do("GET", "/v1/import?library="+my, nil)
	if code != 200 || len(res["planets"].([]any)) != 1 {
		t.Fatalf("list by planet folder %d %v", code, res)
	}
	if code, _ := c.do("GET", "/v1/import?library="+t.TempDir(), nil); code != 400 {
		t.Fatalf("random folder listed: %d", code)
	}
	code, res = c.do("POST", "/v1/import", map[string]any{"library": lib, "id": id, "slug": "notes"})
	if code != 201 || res["site"] != "notes" || res["id"] != id || res["posts"].(float64) != 1 {
		t.Fatalf("import %d %v", code, res)
	}
	code, res = c.do("GET", "/v1/sites/notes", nil)
	if code != 200 || res["id"] != id || res["name"] != "Notes" {
		t.Fatalf("imported site %d %v", code, res)
	}
	if code, _ := c.do("POST", "/v1/import", map[string]any{"library": my, "slug": "again"}); code != 201 {
		t.Fatalf("import by planet folder: %d", code)
	}
	if code, _ := c.do("POST", "/v1/import", map[string]any{"library": lib, "slug": "third"}); code != 400 {
		t.Fatalf("import without id: %d", code)
	}
	if code, _ := c.do("POST", "/v1/import", map[string]any{"library": lib, "id": id, "slug": "notes"}); code != 409 {
		t.Fatalf("import over an existing site: %d", code)
	}
}

// TestPostLanguagesAPI: a post that is in more than one language (PLAN.md,
// A post in more than one language) lists its versions, which are not
// attachments, and the page column's view answers its page in a language.
func TestPostLanguagesAPI(t *testing.T) {
	root := t.TempDir()
	store := site.New(filepath.Join(root, "Planet"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	shipped, _ := filepath.Abs(filepath.Join("..", "..", "templates"))
	if err := service.InstallTemplates(shipped, filepath.Join(store.Root, site.TemplatesDir)); err != nil {
		t.Fatal(err)
	}
	svc := service.New(store, filepath.Join(root, "state"), time.UTC)
	svc.Logf = t.Logf
	s := &Server{Store: store, Builder: svc, Version: "test", Started: time.Now(), Generation: func() uint64 { return 7 }}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	c := &client{t: t, srv: srv}
	if _, err := store.CreateSite("desk", "exe", "", "platinum"); err != nil {
		t.Fatal(err)
	}
	p, err := store.CreatePost("desk", site.KindPost, "meet", site.Meta{Title: "Meet exe"}, "A personal cloud.\n")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(p.Dir, "index.zh-Hans.md"), []byte("---\ntitle: 认识 exe\n---\n\n一朵个人云。\n"), 0o644)
	if _, err := svc.Build("desk"); err != nil {
		t.Fatal(err)
	}

	code, out := c.do("GET", "/v1/sites/desk/posts", nil)
	list, _ := out["list"].([]any)
	if code != 200 || len(list) != 1 {
		t.Fatalf("posts: %d %v", code, out)
	}
	listed := list[0].(map[string]any)
	vs, _ := listed["versions"].([]any)
	if len(vs) != 1 || len(listed["attachments"].([]any)) != 0 {
		t.Fatalf("listed: %v", listed)
	}
	if v := vs[0].(map[string]any); v["lang"] != "zh-Hans" || v["file"] != "index.zh-Hans.md" || v["title"] != "认识 exe" || v["body"] != nil {
		t.Errorf("listed version: %v", v)
	}
	code, out = c.do("GET", "/v1/sites/desk/posts/meet", nil)
	if vs, _ = out["versions"].([]any); code != 200 || len(vs) != 1 || vs[0].(map[string]any)["body"] != "一朵个人云。\n" {
		t.Errorf("the post with its versions: %d %v", code, out)
	}

	view := func(query string) (int, string) {
		resp, err := http.Get(srv.URL + "/v1/sites/desk/posts/meet/view" + query)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, page := view(""); code != 200 || !strings.Contains(page, "A personal cloud.") || strings.Contains(page, "一朵个人云") {
		t.Errorf("view: %d", code)
	}
	if code, page := view("?lang=zh-hans"); code != 200 || !strings.Contains(page, "一朵个人云。") || !strings.Contains(page, `<html lang="zh-Hans">`) || strings.Contains(page, "A personal cloud.") {
		t.Errorf("view in zh-hans: %d", code)
	}
	if code, page := view("?lang=ja"); code != 404 || !strings.Contains(page, "is not built in ja") {
		t.Errorf("view in a language the post is not in: %d %s", code, page)
	}
	if code, _ := view("?lang=..%2Fplanet"); code != 400 {
		t.Errorf("view in no language: %d", code)
	}
	// no attachment route reaches a language's file
	resp, err := http.Get(srv.URL + "/v1/sites/desk/posts/meet/attachments/index.zh-Hans.md")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("a language's file as an attachment: %d", resp.StatusCode)
	}
}
