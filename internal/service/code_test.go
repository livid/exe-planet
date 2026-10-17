package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/site"
)

// TestCustomCode builds a site with its own code for the head, the start
// and the end of every page's body (site.CustomCode): each lands in its
// place on every page Planet's model renders, a part switched off or
// emptied is gone from them, and a site with none builds as before.
func TestCustomCode(t *testing.T) {
	svc, store := newService(t)
	for _, tpl := range []string{"platinum", "paper"} {
		slug := "code-" + tpl
		if _, err := store.CreateSite(slug, "Code", "", tpl); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreatePost(slug, site.KindPost, "hello", site.Meta{Title: "Hello", Created: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}, "Hi.\n"); err != nil {
			t.Fatal(err)
		}
		head := `<script defer data-site="ab40f4ece84a" src="https://stats.example.com/js/s.js"></script>`
		c, err := store.SetCustomCode(slug, site.CodeUpdate{
			Head:      &site.CodePart{Code: head, Enabled: true},
			BodyStart: &site.CodePart{Code: "<!-- start of {{ planet.name }} -->", Enabled: true},
			BodyEnd:   &site.CodePart{Code: "<!-- the end -->", Enabled: false},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !c.Head.Enabled || c.Head.Code != head+"\n" || !c.BodyStart.Enabled || c.BodyEnd.Enabled || c.BodyEnd.Code != "<!-- the end -->\n" {
			t.Fatalf("%s: as written: %+v", tpl, c)
		}
		if _, err := svc.Build(slug); err != nil {
			t.Fatal(err)
		}
		st, _ := store.Site(slug)
		if strings.Join(st.CodeOff, ",") != "body_end" {
			t.Errorf("%s: code_off %v", tpl, st.CodeOff)
		}
		for _, f := range []string{"head.html", "body-start.html", "body-end.html"} {
			if _, err := os.Stat(filepath.Join(st.Dir, f)); err != nil {
				t.Errorf("%s: %s: %v", tpl, f, err)
			}
		}
		pub := svc.PublicDir(st.ID)
		read := func(rel string) string {
			t.Helper()
			b, err := os.ReadFile(filepath.Join(pub, rel))
			if err != nil {
				t.Fatalf("%s %s: %v", tpl, rel, err)
			}
			return string(b)
		}
		for _, rel := range []string{"index.html", "archive.html", "hello/index.html"} {
			page := read(rel)
			h := strings.Index(page, head)
			if h < 0 || h > strings.Index(page, "</head>") {
				t.Errorf("%s %s: the head code is not in the head", tpl, rel)
			}
			// a template, as in Planet: it reads the page's context
			if b := strings.Index(page, "<!-- start of Code -->"); b < strings.Index(page, "<body") {
				t.Errorf("%s %s: the body-start code is not after <body>", tpl, rel)
			}
			if strings.Contains(page, "the end") {
				t.Errorf("%s %s: the switched-off part is written", tpl, rel)
			}
		}
		// on again, the head emptied
		if _, err := store.SetCustomCode(slug, site.CodeUpdate{
			Head:    &site.CodePart{Code: "  \n", Enabled: true},
			BodyEnd: &site.CodePart{Code: "<!-- the end -->", Enabled: true},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Build(slug); err != nil {
			t.Fatal(err)
		}
		st, _ = store.Site(slug)
		if len(st.CodeOff) != 0 {
			t.Errorf("%s: code_off %v after switching on", tpl, st.CodeOff)
		}
		if _, err := os.Stat(filepath.Join(st.Dir, "head.html")); !os.IsNotExist(err) {
			t.Errorf("%s: an emptied part keeps its file: %v", tpl, err)
		}
		page := read("hello/index.html")
		if strings.Contains(page, "stats.example.com") {
			t.Errorf("%s: an emptied part is written", tpl)
		}
		if e, b := strings.Index(page, "<!-- the end -->"), strings.Index(page, "</body>"); e < 0 || e > b {
			t.Errorf("%s: the body-end code is not before </body>", tpl)
		}
	}
	// too much is refused, whole
	if _, err := store.SetCustomCode("code-paper", site.CodeUpdate{Head: &site.CodePart{Code: strings.Repeat("x", site.MaxCode+1), Enabled: true}}); err == nil {
		t.Error("a part over MaxCode is taken")
	}
}
