package site

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A test store lives in its own temporary folder, never the Workspace.
func testStore(t *testing.T) *Store {
	t.Helper()
	s := New(filepath.Join(t.TempDir(), "Planet"))
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	n := 0
	s.Now = func() time.Time { n++; return base.Add(time.Duration(n) * time.Minute) }
	return s
}

func TestSiteLifecycle(t *testing.T) {
	s := testStore(t)
	st, err := s.CreateSite("charts", " Charts & Thoughts ", "about", "")
	if err != nil {
		t.Fatal(err)
	}
	if st.Slug != "charts" || st.Name != "Charts & Thoughts" || st.Template != "plain" || len(st.ID) != 36 || st.ID != strings.ToUpper(st.ID) {
		t.Fatalf("created %+v", st)
	}
	for _, d := range []string{"posts", "pages", "drafts"} {
		if fi, err := os.Stat(filepath.Join(s.Root, "charts", d)); err != nil || !fi.IsDir() {
			t.Fatalf("%s folder missing", d)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(s.Root, "charts", "site.json"))
	if !strings.Contains(string(raw), `"name": "Charts & Thoughts"`) || strings.Contains(string(raw), `"slug"`) || strings.Contains(string(raw), `"posts"`) {
		t.Fatalf("site.json holds derived fields:\n%s", raw)
	}
	if _, err := s.CreateSite("charts", "Again", "", ""); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	for _, slug := range []string{"Templates", "archived", "-x", ""} {
		if _, err := s.CreateSite(slug, "X", "", ""); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %v", slug, err)
		}
	}
	if _, err := s.CreateSite("nameless", "  ", "", ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty name: %v", err)
	}

	name, tpl := "Charts", "sepia"
	st, err = s.UpdateSite("charts", SiteUpdate{Name: &name, Template: &tpl, Social: &Social{GitHub: "livid"}})
	if err != nil || st.Name != "Charts" || st.Template != "sepia" || st.Social.GitHub != "livid" || st.About != "about" {
		t.Fatalf("update: %v %+v", err, st)
	}

	sites, err := s.Sites(false)
	if err != nil || len(sites) != 1 || sites[0].Slug != "charts" {
		t.Fatalf("list: %v %+v", err, sites)
	}
	if err := s.ArchiveSite("charts"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Site("charts"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived site still live: %v", err)
	}
	if arch, _ := s.Sites(true); len(arch) != 1 || !arch[0].Archived || arch[0].Name != "Charts" {
		t.Fatalf("archived list: %+v", arch)
	}
	if _, err := s.CreateSite("charts", "Clash", "", ""); !errors.Is(err, ErrExists) {
		t.Fatalf("archived slug reusable: %v", err)
	}
	if err := s.UnarchiveSite("charts"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSite("charts"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "charts")); !os.IsNotExist(err) {
		t.Fatal("site folder remains")
	}
	if err := s.DeleteSite("charts"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}

// A folder made by hand is a site once it holds a site.json; one whose
// site.json is broken is listed with the reason, never dropped.
func TestSitesFromDisk(t *testing.T) {
	s := testStore(t)
	os.MkdirAll(filepath.Join(s.Root, "handmade"), 0o755)
	os.WriteFile(filepath.Join(s.Root, "handmade", "site.json"), []byte(`{"id":"ABC","name":"Hand","template":"plain"}`), 0o644)
	os.MkdirAll(filepath.Join(s.Root, "broken"), 0o755)
	os.WriteFile(filepath.Join(s.Root, "broken", "site.json"), []byte(`{"id":`), 0o644)
	os.MkdirAll(filepath.Join(s.Root, "just-a-folder"), 0o755)
	os.MkdirAll(filepath.Join(s.Root, "Bad Name"), 0o755)
	os.WriteFile(filepath.Join(s.Root, "Bad Name", "site.json"), []byte(`{"id":"X","name":"Bad"}`), 0o644)
	os.WriteFile(filepath.Join(s.Root, TemplatesDir, "site.json"), []byte(`{}`), 0o644)
	sites, err := s.Sites(false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, st := range sites {
		got[st.Slug] = st.Error
	}
	if len(got) != 3 || got["handmade"] != "" || !strings.Contains(got["broken"], "site.json") || !strings.Contains(got["Bad Name"], "folder name") {
		t.Fatalf("listed %v", got)
	}
	if st, err := s.Site("broken"); !errors.Is(err, ErrInvalid) || st == nil || st.Error == "" {
		t.Fatalf("broken site: %v %+v", err, st)
	}
	if _, err := s.Post("broken", "x", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("posts of a broken site: %v", err)
	}
}

func TestPostLifecycle(t *testing.T) {
	s := testStore(t)
	if _, err := s.CreateSite("charts", "Charts", "", ""); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePost("charts", KindDraft, "hello-world", Meta{Title: "Hello", Tags: []string{"a", "b"}}, "# Hi\n")
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != KindDraft || p.ID == "" || p.Created.IsZero() || p.Body != "# Hi\n" || len(p.Attachments) != 0 {
		t.Fatalf("created %+v", p)
	}
	raw, _ := os.ReadFile(filepath.Join(s.Root, "charts", "drafts", "hello-world", "index.md"))
	if !strings.HasPrefix(string(raw), "---\nid: "+p.ID+"\ntitle: Hello\ncreated: 2026-09-25T10:") {
		t.Fatalf("index.md:\n%s", raw)
	}
	for _, kind := range Kinds {
		if _, err := s.CreatePost("charts", kind, "hello-world", Meta{Title: "Dup"}, ""); !errors.Is(err, ErrExists) {
			t.Errorf("slug reused as %s: %v", kind, err)
		}
	}
	if _, err := s.CreatePost("charts", KindPost, "untitled", Meta{}, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("no title: %v", err)
	}
	if _, err := s.CreatePost("charts", "blog", "x", Meta{Title: "X"}, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad kind: %v", err)
	}
	if _, err := s.CreatePost("nope", KindPost, "x", Meta{Title: "X"}, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("no site: %v", err)
	}

	body, hero := "# Changed\n", "cover.png"
	p, err = s.UpdatePost("charts", "hello-world", "", PostUpdate{Body: &body, Hero: &hero, Tags: &[]string{"c", "c", " d "}})
	if err != nil || p.Body != body || p.Hero != hero || strings.Join(p.Tags, ",") != "c,d" || p.Modified.IsZero() || p.Title != "Hello" {
		t.Fatalf("update: %v %+v", err, p)
	}
	if _, err := s.UpdatePost("charts", "hello-world", KindPost, PostUpdate{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update in the wrong kind: %v", err)
	}

	p, err = s.MovePost("charts", "hello-world", "", KindPost)
	if err != nil || p.Kind != KindPost || p.Body != body {
		t.Fatalf("move: %v %+v", err, p)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "charts", "drafts", "hello-world")); !os.IsNotExist(err) {
		t.Fatal("draft folder remains after move")
	}
	if _, err := s.CreatePost("charts", KindPage, "about", Meta{Title: "About", Created: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}, ""); err != nil {
		t.Fatal(err)
	}
	all, _ := s.Posts("charts", "")
	if len(all) != 2 || all[0].Slug != "hello-world" || all[1].Slug != "about" || all[0].Body != "" {
		t.Fatalf("listing %+v", all)
	}
	pages, _ := s.Posts("charts", KindPage)
	if len(pages) != 1 || pages[0].Kind != KindPage {
		t.Fatalf("pages %+v", pages)
	}
	if _, err := s.Posts("charts", "blog"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad kind listing: %v", err)
	}
	st, _ := s.Site("charts")
	if st.Posts != 1 || st.Pages != 1 || st.Drafts != 0 {
		t.Fatalf("counts %+v", st)
	}

	if err := s.DeletePost("charts", "hello-world", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Post("charts", "hello-world", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted post found: %v", err)
	}
}

func TestAttachments(t *testing.T) {
	s := testStore(t)
	s.CreateSite("charts", "Charts", "", "")
	s.CreatePost("charts", KindPost, "pic", Meta{Title: "Pic"}, "")
	name, err := s.AddAttachment("charts", "pic", "", "../../evil/../photo.jpg", strings.NewReader("jpeg"))
	if err != nil || name != "photo.jpg" {
		t.Fatalf("add: %v %q", err, name)
	}
	if b, _ := os.ReadFile(filepath.Join(s.Root, "charts", "posts", "pic", "photo.jpg")); string(b) != "jpeg" {
		t.Fatalf("attachment content %q", b)
	}
	for _, bad := range []string{"", ".", "..", ".hidden", "_grid.png", "index.md", "a\x00b", "/"} {
		if _, err := s.AddAttachment("charts", "pic", "", bad, strings.NewReader("x")); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q accepted: %v", bad, err)
		}
	}
	os.WriteFile(filepath.Join(s.Root, "charts", "posts", "pic", "_grid.png"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(s.Root, "charts", "posts", "pic", ".DS_Store"), []byte("x"), 0o644)
	p, _ := s.Post("charts", "pic", "")
	if strings.Join(p.Attachments, ",") != "photo.jpg" {
		t.Fatalf("attachments %v", p.Attachments)
	}
	if fp, err := s.AttachmentPath("charts", "pic", "", "photo.jpg"); err != nil || filepath.Base(fp) != "photo.jpg" {
		t.Fatalf("path: %v %s", err, fp)
	}
	if _, err := s.AttachmentPath("charts", "pic", "", "_grid.png"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("generated file served: %v", err)
	}
	if err := s.DeleteAttachment("charts", "pic", "", "photo.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAttachment("charts", "pic", "", "photo.jpg"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}

func TestTemplates(t *testing.T) {
	s := testStore(t)
	os.MkdirAll(filepath.Join(s.Root, TemplatesDir, "plain", "templates"), 0o755)
	os.WriteFile(filepath.Join(s.Root, TemplatesDir, "plain", "template.json"), []byte(`{"name":"Plain","version":"1.9","buildNumber":132,"generateTagPages":true}`), 0o644)
	os.MkdirAll(filepath.Join(s.Root, TemplatesDir, "empty"), 0o755)
	os.MkdirAll(filepath.Join(s.Root, TemplatesDir, "bad"), 0o755)
	os.WriteFile(filepath.Join(s.Root, TemplatesDir, "bad", "template.json"), []byte(`nope`), 0o644)
	ts, err := s.Templates()
	if err != nil || len(ts) != 2 || ts[0].Slug != "bad" || ts[0].Error == "" || ts[1].Name != "Plain" || ts[1].BuildNumber != 132 || !ts[1].GenerateTagPages {
		t.Fatalf("%v %+v", err, ts)
	}
}
