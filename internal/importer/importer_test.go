package importer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/planet"
	"github.com/livid/exe-planet/internal/service"
	"github.com/livid/exe-planet/internal/site"
)

// A small library made by hand: one planet with a post, a page with a
// weight, an attachment, a hero, tags, custom code and an avatar.
func writeLibrary(t *testing.T) string {
	t.Helper()
	lib := t.TempDir()
	id := "7AB9F0C5-BCAE-4952-A670-18A416D1C968"
	my := filepath.Join(lib, "My", id)
	os.MkdirAll(filepath.Join(my, "Articles"), 0o755)
	os.MkdirAll(filepath.Join(my, "Drafts"), 0o755)
	os.WriteFile(filepath.Join(my, "planet.json"), []byte(`{
  "id": "`+id+`", "name": "Release Notes", "about": "What *changed*", "ipns": "k51abc", "created": 796032373.386509, "updated": 811600897.450024,
  "templateName": "Sparkle", "authorName": "Livid", "tags": {"release": "Release"}, "githubUsername": "livid",
  "plausibleEnabled": true, "plausibleDomain": "notes.example.com", "doNotIndex": true,
  "customCodeHeadEnabled": true, "customCodeHead": "<meta name=\"x\" content=\"y\">",
  "customCodeBodyEndEnabled": false, "customCodeBodyEnd": "ignored",
  "cloudflarePagesAPIToken": "secret-token", "filebaseAPIToken": "secret-2"
}`), 0o644)
	os.WriteFile(filepath.Join(my, "templateSettings.json"), []byte(`{"highlightColor": "#ff3b30"}`), 0o644)
	os.WriteFile(filepath.Join(my, "avatar.png"), []byte("png"), 0o644)
	post := "006662F5-AB90-4795-BB62-08C440DA45EE"
	page := "01473E82-0408-4384-AC4E-AFACE4DE125B"
	os.WriteFile(filepath.Join(my, "Articles", post+".json"), []byte(`{
  "id": "`+post+`", "title": "Hello, World", "content": "# Hi\n\nfirst *post*", "created": 675853734, "modified": 675853800.5,
  "attachments": ["photo.jpg", "notes.txt"], "heroImage": "photo.jpg", "tags": {"macos": "macOS", "release": "Release"},
  "articleNumber": 9, "slug": "hello", "externalLink": "https://example.com/x"
}`), 0o644)
	os.WriteFile(filepath.Join(my, "Articles", page+".json"), []byte(`{
  "id": "`+page+`", "title": "About", "content": "about us", "created": 675853735, "articleType": 1,
  "isIncludedInNavigation": true, "navigationWeight": 3, "attachments": []
}`), 0o644)
	pubPost := filepath.Join(lib, "Public", id, post)
	os.MkdirAll(pubPost, 0o755)
	os.WriteFile(filepath.Join(pubPost, "photo.jpg"), []byte("jpeg"), 0o644)
	os.WriteFile(filepath.Join(pubPost, "_grid.png"), []byte("generated"), 0o644)
	return lib
}

func newStore(t *testing.T) *site.Store {
	t.Helper()
	store := site.New(filepath.Join(t.TempDir(), "Planet"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	shipped, _ := filepath.Abs(filepath.Join("..", "..", "templates"))
	if err := service.InstallTemplates(shipped, filepath.Join(store.Root, site.TemplatesDir)); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestListAndImport(t *testing.T) {
	lib := writeLibrary(t)
	store := newStore(t)
	entries, err := List(lib)
	if err != nil || len(entries) != 1 || entries[0].Name != "Release Notes" || entries[0].Articles != 2 || !entries[0].Public || entries[0].Template != "Sparkle" {
		t.Fatalf("list: %v %+v", err, entries)
	}
	if _, err := List(t.TempDir()); err == nil {
		t.Fatal("a folder without My listed")
	}

	res, err := Import(store, filepath.Join(store.Root, site.TemplatesDir), lib, entries[0].ID, "notes")
	if err != nil {
		t.Fatal(err)
	}
	if res.Posts != 1 || res.Pages != 1 || res.Attachments != 1 || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "notes.txt") {
		t.Fatalf("result %+v", res)
	}
	st, err := store.Site("notes")
	if err != nil {
		t.Fatal(err)
	}
	if st.ID != entries[0].ID || st.Template != "sparkle" || st.About != "What *changed*" || st.Author != "Livid" || st.Social.GitHub != "livid" || !st.Plausible.Enabled || st.Plausible.Domain != "notes.example.com" || !st.NoIndex || st.TemplateSettings["highlightColor"] != "#ff3b30" || st.Tags["release"] != "Release" {
		t.Fatalf("site %+v", st.SiteFile)
	}
	if planet.DateOf(st.Created).String() != "796032373.386509" {
		t.Fatalf("created lost its fraction: %v", st.Created)
	}
	raw, _ := os.ReadFile(filepath.Join(st.Dir, "site.json"))
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "ipns") {
		t.Fatalf("site.json carries what it must not:\n%s", raw)
	}
	if b, _ := os.ReadFile(filepath.Join(st.Dir, "head.html")); string(b) != `<meta name="x" content="y">` {
		t.Fatalf("head.html %q", b)
	}
	if _, err := os.Stat(filepath.Join(st.Dir, "body-end.html")); !os.IsNotExist(err) {
		t.Fatal("disabled custom code was written")
	}
	if _, err := os.Stat(filepath.Join(st.Dir, "avatar.png")); err != nil {
		t.Fatal("avatar not copied")
	}

	p, err := store.Post("notes", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != site.KindPost || p.ID != "006662F5-AB90-4795-BB62-08C440DA45EE" || p.Body != "# Hi\n\nfirst *post*\n" || p.Hero != "photo.jpg" || p.Link != "https://example.com/x" || strings.Join(p.Tags, ",") != "macOS,Release" {
		t.Fatalf("post %+v", p)
	}
	if got := planet.DateOf(p.Created); got != 675853734 {
		t.Fatalf("created %v", got)
	}
	if got := planet.DateOf(p.Modified).String(); got != "675853800.5" {
		t.Fatalf("modified %v", got)
	}
	if strings.Join(p.Attachments, ",") != "photo.jpg" {
		t.Fatalf("attachments %v", p.Attachments)
	}
	pg, err := store.Post("notes", "about", "")
	if err != nil || pg.Kind != site.KindPage || pg.Extra["weight"] != "3" {
		t.Fatalf("page %v %+v", err, pg)
	}
	if _, err := Import(store, "", lib, entries[0].ID, "notes"); err == nil {
		t.Fatal("imported over an existing site")
	}
	if _, err := Import(store, "", lib, "NOPE", "other"); err == nil {
		t.Fatal("imported a missing planet")
	}
	// A failure part way leaves no site: make a post's folder unwritable.
	broken := writeLibrary(t)
	os.WriteFile(filepath.Join(broken, "My", "7AB9F0C5-BCAE-4952-A670-18A416D1C968", "Articles", "bad.json"), []byte(`{`), 0o644)
	if _, err := Import(store, "", broken, "7AB9F0C5-BCAE-4952-A670-18A416D1C968", "half"); err == nil {
		t.Fatal("a broken article imported")
	}
	if _, err := store.Site("half"); !errors.Is(err, site.ErrNotFound) {
		t.Fatalf("a failed import left the site: %v", err)
	}
	if _, err := Import(store, "", lib, entries[0].ID, "Bad Slug"); err == nil {
		t.Fatal("imported under an invalid slug")
	}

	// The imported site builds, with Planet's ids and dates intact.
	svc := service.New(store, t.TempDir(), time.UTC)
	svc.Logf = t.Logf
	status, err := svc.Build("notes")
	if err != nil || status.Files == 0 {
		t.Fatalf("build %v %+v", err, status)
	}
	var pj struct {
		ID       string `json:"id"`
		Articles []struct {
			ID      string  `json:"id"`
			Created float64 `json:"created"`
			Link    string  `json:"link"`
		} `json:"articles"`
	}
	b, _ := os.ReadFile(filepath.Join(svc.PublicDir(st.ID), "planet.json"))
	json.Unmarshal(b, &pj)
	if pj.ID != st.ID || len(pj.Articles) != 1 || pj.Articles[0].ID != p.ID || pj.Articles[0].Created != 675853734 || pj.Articles[0].Link != "/hello/" {
		t.Fatalf("built planet.json %+v", pj)
	}
	if b, _ := os.ReadFile(filepath.Join(svc.PublicDir(st.ID), "robots.txt")); !strings.Contains(string(b), "Disallow") {
		t.Fatal("noindex was not built into robots.txt")
	}
}

func TestSlugsAndRoots(t *testing.T) {
	taken := map[string]bool{}
	a := &planet.MyArticle{ID: "AAAA", Title: "Hello, World"}
	slug := func() string { return uniqueSlug(a, taken) }
	if s := slug(); s != "hello-world" {
		t.Fatal(s)
	}
	if s := slug(); s != "hello-world-2" {
		t.Fatal(s)
	}
	a.Slug = ptr("custom")
	if s := slug(); s != "custom" {
		t.Fatal(s)
	}
	a.Slug, a.Title = ptr("Bad Slug"), "数据"
	if s := slug(); s != "aaaa" {
		t.Fatal(s)
	}
	lib := writeLibrary(t)
	if root, id, err := LibraryRoot(lib); err != nil || root != lib || id != "" {
		t.Fatalf("library root %q %q %v", root, id, err)
	}
	my := filepath.Join(lib, "My", "7AB9F0C5-BCAE-4952-A670-18A416D1C968")
	if root, id, err := LibraryRoot(my); err != nil || root != lib || id != "7AB9F0C5-BCAE-4952-A670-18A416D1C968" {
		t.Fatalf("my root %q %q %v", root, id, err)
	}
	if _, _, err := LibraryRoot(t.TempDir()); err == nil {
		t.Fatal("random folder accepted")
	}
}

func ptr(s string) *string { return &s }

// A real golden library, when fetched, imports and rebuilds with every
// article id and creation date as Planet had them.
func TestImportGolden(t *testing.T) {
	lib := filepath.Join("..", "..", "testdata", "goldens", "sparkle-release")
	if _, err := os.Stat(filepath.Join(lib, "My")); err != nil {
		t.Skip("no golden fetched")
	}
	store := newStore(t)
	entries, err := List(lib)
	if err != nil || len(entries) != 1 {
		t.Fatalf("%v %+v", err, entries)
	}
	res, err := Import(store, filepath.Join(store.Root, site.TemplatesDir), lib, entries[0].ID, "release-notes")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Articles []struct {
			ID      string  `json:"id"`
			Created float64 `json:"created"`
			Title   string  `json:"title"`
		} `json:"articles"`
	}
	b, _ := os.ReadFile(filepath.Join(lib, "Public", entries[0].ID, "planet.json"))
	json.Unmarshal(b, &golden)
	if res.Posts != len(golden.Articles) {
		t.Fatalf("imported %d posts, golden lists %d", res.Posts, len(golden.Articles))
	}
	svc := service.New(store, t.TempDir(), time.UTC)
	svc.Logf = t.Logf
	if _, err := svc.Build("release-notes"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Site("release-notes")
	var built struct {
		Articles []struct {
			ID      string  `json:"id"`
			Created float64 `json:"created"`
			Title   string  `json:"title"`
		} `json:"articles"`
	}
	b, _ = os.ReadFile(filepath.Join(svc.PublicDir(st.ID), "planet.json"))
	json.Unmarshal(b, &built)
	if len(built.Articles) != len(golden.Articles) {
		t.Fatalf("built %d articles, golden %d", len(built.Articles), len(golden.Articles))
	}
	for i := range golden.Articles {
		g, m := golden.Articles[i], built.Articles[i]
		if g.ID != m.ID || g.Created != m.Created || g.Title != m.Title {
			t.Fatalf("article %d: golden %+v, built %+v", i, g, m)
		}
	}
}
