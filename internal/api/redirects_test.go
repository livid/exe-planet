package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestServeBuildRedirects: a built site's _redirects answers only the
// paths it has no file for, under a hostname and under /sites/<slug>.
func TestServeBuildRedirects(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(text), 0o644)
	}
	write("index.html", "home")
	write("archive.html", "archive")
	write("20220102/index.html", "post")
	write("404.html", "missing")
	write("_redirects", "/blog/ /archive.html\n/blog/* /:splat\n/index.html /archive.html\n/old https://example.com/new 302\n/pretty /archive.html 200\n/* /404.html 404\n")
	get := func(prefix, target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		serveBuild(w, httptest.NewRequest("GET", target, nil), dir, prefix)
		return w
	}
	for _, c := range []struct {
		prefix, path string
		code         int
		location     string
		body         string
	}{
		{"", "/", 200, "", "home"},
		{"", "/20220102/", 200, "", "post"},
		{"", "/blog/", 301, "/archive.html", ""},
		{"", "/blog/20220102/?utm=x", 301, "/20220102/?utm=x", ""},
		{"", "/old", 302, "https://example.com/new", ""},
		{"", "/pretty", 200, "", "archive"},
		{"", "/nothing", 404, "", "missing"},
		{"", "/archive.html", 200, "", "archive"}, // a file wins over the catch-all
		{"/sites/cl", "/sites/cl/blog/20220102/", 301, "/sites/cl/20220102/", ""},
		{"/sites/cl", "/sites/cl/", 200, "", "home"},
	} {
		w := get(c.prefix, c.path)
		if w.Code != c.code || w.Header().Get("Location") != c.location || (c.body != "" && w.Body.String() != c.body) {
			t.Errorf("%s%s: %d %q %q", c.prefix, c.path, w.Code, w.Header().Get("Location"), w.Body.String())
		}
	}
	// with no _redirects a missing path is the file server's own 404
	os.Remove(filepath.Join(dir, "_redirects"))
	if w := get("", "/blog/"); w.Code != http.StatusNotFound {
		t.Errorf("no _redirects: %d", w.Code)
	}
}
