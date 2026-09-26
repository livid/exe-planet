package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/planet"
	"github.com/livid/exe-planet/internal/site"
)

// newService makes a store and a service in scratch folders with the
// shipped Sparkle template installed, never touching the Workspace.
func newService(t *testing.T) (*Service, *site.Store) {
	t.Helper()
	root := t.TempDir()
	store := site.New(filepath.Join(root, "Planet"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	shipped, _ := filepath.Abs(filepath.Join("..", "..", "templates"))
	if err := InstallTemplates(shipped, filepath.Join(store.Root, site.TemplatesDir)); err != nil {
		t.Fatal(err)
	}
	la, _ := time.LoadLocation("America/Los_Angeles")
	svc := New(store, filepath.Join(root, "state"), la)
	svc.Logf = t.Logf
	return svc, store
}

func TestBuildSiteFromWorkspace(t *testing.T) {
	svc, store := newService(t)
	st, err := store.CreateSite("notes", "Release Notes", "What changed & why", "sparkle")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreatePost("notes", site.KindPost, "hello", site.Meta{Title: "Hello, World", Tags: []string{"Release Notes", "v1"}}, "# Hi\n\nfirst *post*\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreatePost("notes", site.KindPage, "about", site.Meta{Title: "About", Extra: map[string]string{"weight": "2"}}, "about us\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreatePost("notes", site.KindDraft, "later", site.Meta{Title: "Later"}, "not yet\n"); err != nil {
		t.Fatal(err)
	}
	status, err := svc.Build("notes")
	if err != nil {
		t.Fatal(err)
	}
	if status.Files == 0 || status.Error != "" || status.Building {
		t.Fatalf("status %+v", status)
	}
	pub := svc.PublicDir(st.ID)
	for _, f := range []string{"index.html", "page1.html", "planet.json", "rss.xml", "robots.txt"} {
		if _, err := os.Stat(filepath.Join(pub, f)); err != nil {
			t.Errorf("%s not built", f)
		}
	}
	posts, _ := store.Posts("notes", "")
	byslug := map[string]*site.Post{}
	for _, p := range posts {
		byslug[p.Slug] = p
	}
	for _, slug := range []string{"hello", "about"} {
		id := byslug[slug].ID
		for _, f := range []string{id + "/index.html", id + "/article.json", id + "/article.md", slug + "/index.html"} {
			if _, err := os.Stat(filepath.Join(pub, f)); err != nil {
				t.Errorf("%s not built", f)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(pub, byslug["later"].ID)); !os.IsNotExist(err) {
		t.Error("a draft was built")
	}

	// planet.json is Planet's: 2001-epoch dates, only blog articles, the
	// site's tags as slug to display name.
	raw, _ := os.ReadFile(filepath.Join(pub, "planet.json"))
	var pj struct {
		ID       string            `json:"id"`
		Name     string            `json:"name"`
		About    string            `json:"about"`
		Created  float64           `json:"created"`
		Updated  float64           `json:"updated"`
		Tags     map[string]string `json:"tags"`
		Articles []struct {
			ID      string            `json:"id"`
			Link    string            `json:"link"`
			Slug    string            `json:"slug"`
			Tags    map[string]string `json:"tags"`
			Created float64           `json:"created"`
			Content string            `json:"content"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(raw, &pj); err != nil {
		t.Fatal(err)
	}
	if pj.ID != st.ID || pj.Name != "Release Notes" || pj.About != "What changed & why" {
		t.Fatalf("planet.json %+v", pj)
	}
	if pj.Created < 700_000_000 || pj.Created > 1_500_000_000 || pj.Updated < pj.Created {
		t.Fatalf("dates are not seconds since 2001: %v %v", pj.Created, pj.Updated)
	}
	if len(pj.Articles) != 1 || pj.Articles[0].Slug != "hello" || pj.Articles[0].Link != "/hello/" || pj.Articles[0].Tags["release-notes"] != "Release Notes" || pj.Articles[0].Tags["v1"] != "v1" {
		t.Fatalf("articles %+v", pj.Articles)
	}
	if pj.Tags["release-notes"] != "Release Notes" {
		t.Fatalf("site tags %v", pj.Tags)
	}
	if !strings.Contains(string(raw), `"about" : "What changed & why"`) {
		t.Fatal("planet.json is not in Foundation's layout")
	}
	page, err := os.ReadFile(filepath.Join(pub, "hello", "index.html"))
	if err != nil {
		ents, _ := os.ReadDir(pub)
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Fatalf("article page: %v; public holds %s", err, strings.Join(names, " "))
	}
	if !strings.Contains(string(page), "<h1>Hello, World</h1>") || !strings.Contains(string(page), "first <em>post</em>") {
		t.Fatalf("article page (%d bytes):\n%s", len(page), page)
	}

	// A second build swaps the folder in place.
	if _, err := svc.Build("notes"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pub + ".new"); !os.IsNotExist(err) {
		t.Fatal("scratch build folder left behind")
	}
	s2, _ := svc.Status("notes")
	if s2.Built.IsZero() || s2.Public != pub {
		t.Fatalf("status %+v", s2)
	}
}

func TestPreviewAndChanges(t *testing.T) {
	svc, store := newService(t)
	store.CreateSite("notes", "Notes", "", "sparkle")
	html, err := svc.Preview("notes", &site.Post{Meta: site.Meta{Title: "Draft title"}, Body: "**bold**"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<h1>Draft title</h1>") || !strings.Contains(html, "<strong>bold</strong>") {
		t.Fatalf("preview:\n%s", html)
	}
	if _, err := svc.Status("notes"); err != nil {
		t.Fatal(err)
	}
	if st, _ := svc.Status("notes"); !st.Built.IsZero() {
		t.Fatal("a preview built the site")
	}

	// A change under the site builds it; one under Archived does not.
	svc.OnChange([]string{"notes/posts/x/index.md", "Archived/old/site.json", ".Trash/x"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, _ := svc.Status("notes")
		if !st.Built.IsZero() && !st.Building {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("change did not build the site")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := svc.Status("nope"); err == nil {
		t.Fatal("status of a missing site")
	}
}

// Builds of one site coalesce: requests during a build end in one more.
func TestBuildCoalesces(t *testing.T) {
	svc, store := newService(t)
	store.CreateSite("notes", "Notes", "", "sparkle")
	for i := 0; i < 30; i++ {
		store.CreatePost("notes", site.KindPost, "p"+string(rune('a'+i)), site.Meta{Title: "P"}, "x\n")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.Build("notes")
		}()
	}
	wg.Wait()
	st, _ := svc.Status("notes")
	if st.Building || st.Requested || st.Built.IsZero() {
		t.Fatalf("after the burst: %+v", st)
	}
}

func TestTagKeyAndSummary(t *testing.T) {
	for in, want := range map[string]string{"Release Notes": "release-notes", "macOS": "macos", "数据 图表": "数据-图表", " C++ ": "c", "Hello, World!": "hello-world", "a--b": "a-b", "-x-": "x"} {
		if got := TagKey(in); got != want {
			t.Errorf("TagKey(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("word ", 100)
	if s := Summary("<p>" + long + "</p>"); len([]rune(s)) != 283 || !strings.HasSuffix(s, "...") {
		t.Fatalf("summary %d %q", len([]rune(s)), s[len(s)-10:])
	}
	if s := Summary("<h1>Hi</h1>\n<p>a  b\n c</p>"); s != "Hi a b c" {
		t.Fatalf("summary %q", s)
	}
	_ = planet.Date(0)
}

func TestViewAndRenderPath(t *testing.T) {
	svc, store := newService(t)
	store.CreateSite("notes", "Notes", "", "sparkle")
	p, _ := store.CreatePost("notes", site.KindPost, "hello", site.Meta{Title: "Hello"}, "![pic](photo.jpg)\n")
	store.AddAttachment("notes", "hello", "", "photo.jpg", strings.NewReader("jpeg"))
	if _, err := svc.View("notes", "hello", Attachments{}); err == nil {
		t.Fatal("view before a build")
	}
	if _, err := svc.Build("notes"); err != nil {
		t.Fatal(err)
	}
	page, err := svc.View("notes", "hello", Attachments{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "<h1>Hello</h1>") || !strings.Contains(page, `src="data:image/jpeg;base64,anBlZw=="`) {
		t.Fatalf("view folded nothing:\n%s", page)
	}
	page, _ = svc.View("notes", "hello", Attachments{Base: "/att/", Query: "token=T"})
	if !strings.Contains(page, `src="/att/photo.jpg?token=T"`) {
		t.Fatalf("view route:\n%s", page)
	}

	// The Writer's render: a post's path goes through the site's template
	// with the field's unsaved text; any other file is plain Markdown.
	rel := "Planet/notes/posts/hello/index.md"
	page, err = svc.RenderPath(rel, "---\nid: "+p.ID+"\ntitle: Draft title\n---\n\n**bold** ![pic](photo.jpg)\n", Attachments{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "<h1>Draft title</h1>") || !strings.Contains(page, "<strong>bold</strong>") || !strings.Contains(page, "base64,anBlZw==") {
		t.Fatalf("post render:\n%s", page)
	}
	os.MkdirAll(filepath.Join(filepath.Dir(store.Root), "Notes"), 0o755)
	os.WriteFile(filepath.Join(filepath.Dir(store.Root), "Notes", "pic.png"), []byte("png"), 0o644)
	page, err = svc.RenderPath("Notes/todo.md", "# Todo\n\n- [ ] a ![p](pic.png)\n", Attachments{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "<title>Todo</title>") || !strings.Contains(page, "<h1>Todo</h1>") || !strings.Contains(page, "base64,cG5n") || !strings.Contains(page, `type="checkbox"`) {
		t.Fatalf("plain render:\n%s", page)
	}
	for _, bad := range []string{"", "../x.md", "Planet/../../etc/passwd"} {
		if _, err := svc.RenderPath(bad, "x", Attachments{}); err == nil {
			t.Errorf("%q rendered", bad)
		}
	}
}
