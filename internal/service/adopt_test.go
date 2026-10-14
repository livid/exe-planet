package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/build"
	"github.com/livid/exe-planet/internal/planet"
	"github.com/livid/exe-planet/internal/site"
)

// TestBuildAdoptsHandMadePosts: a post made by hand, a folder with an
// index.md and no front matter, used to stop its whole site from
// building — with no id its folder in the build was the build's own
// root, and the copy made under its slug a copy of the build into
// itself, until the path was too long. The build now issues it an id
// and a date, into its file, and builds it like any other.
func TestBuildAdoptsHandMadePosts(t *testing.T) {
	svc, store := newService(t)
	if _, err := store.CreateSite("wen", "空山", "", "paper"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Site("wen")
	made, err := store.CreatePost("wen", site.KindPost, "made", site.Meta{Title: "Made here"}, "By the app.\n")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(st.Dir, "posts", "by-hand")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "index.md")
	os.WriteFile(file, []byte("Written into the folder, nothing else.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("an attachment"), 0o644)
	written := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	os.Chtimes(file, written, written)

	if _, err := svc.Build("wen"); err != nil {
		t.Fatalf("the site does not build with a post made by hand in it: %.200v", err)
	}
	p, err := store.Post("wen", "by-hand", "")
	if err != nil || p.ID == "" || !p.Created.Equal(written) || p.Body != "Written into the folder, nothing else.\n" {
		t.Fatalf("the post after the build: %+v, %v", p, err)
	}
	doc, _ := os.ReadFile(file)
	if !strings.HasPrefix(string(doc), "---\nid: "+p.ID+"\ncreated: 2026-03-04T") || !strings.HasSuffix(string(doc), "---\n\nWritten into the folder, nothing else.\n") {
		t.Errorf("the file was not given just an id and a date: %q", doc)
	}
	pub := svc.PublicDir(st.ID)
	for _, rel := range []string{p.ID + "/index.html", "by-hand/index.html", p.ID + "/note.txt", made.ID + "/index.html", "made/index.html", "index.html", "planet.json"} {
		if _, err := os.Stat(filepath.Join(pub, filepath.FromSlash(rel))); err != nil {
			t.Errorf("the build lacks %s", rel)
		}
	}
	page, _ := os.ReadFile(filepath.Join(pub, "by-hand", "index.html"))
	if !strings.Contains(string(page), `<time datetime="2026-03-04T`) || !strings.Contains(string(page), "Written into the folder, nothing else.") {
		t.Errorf("the post's page has not its words under the day its file was written")
	}
	// the build is as deep as a post's folder and no deeper
	deepest := 0
	filepath.WalkDir(pub, func(path string, d os.DirEntry, err error) error {
		rel, _ := filepath.Rel(pub, path)
		if n := strings.Count(filepath.ToSlash(rel), "/"); n > deepest {
			deepest = n
		}
		return nil
	})
	if deepest > 2 {
		t.Errorf("the build nests %d folders deep", deepest)
	}
	if pj, _ := os.ReadFile(filepath.Join(pub, "planet.json")); !strings.Contains(string(pj), p.ID) || !strings.Contains(string(pj), made.ID) {
		t.Error("planet.json lacks one of the two posts")
	}
	// again: the same id, the file as it is
	if _, err := svc.Build("wen"); err != nil {
		t.Fatal(err)
	}
	if q, _ := store.Post("wen", "by-hand", ""); q.ID != p.ID {
		t.Errorf("a second build issued another id: %s, then %s", p.ID, q.ID)
	}
	if again, _ := os.ReadFile(file); string(again) != string(doc) {
		t.Error("a second build wrote the file again")
	}

	// a post whose file cannot be written to is left out; the site still builds
	locked := filepath.Join(st.Dir, "posts", "locked")
	os.MkdirAll(locked, 0o755)
	os.WriteFile(filepath.Join(locked, "index.md"), []byte("No id, and no leave to write one.\n"), 0o444)
	if os.Getuid() != 0 {
		if _, err := svc.Build("wen"); err != nil {
			t.Fatalf("a post that cannot be written to stops the build: %.200v", err)
		}
		if _, err := os.Stat(filepath.Join(pub, "locked")); err == nil {
			t.Error("a post with no id was built")
		}
		if _, err := os.Stat(filepath.Join(pub, "by-hand", "index.html")); err != nil {
			t.Error("the other posts were not built beside it")
		}
		if _, err := svc.View("wen", "locked", "", Attachments{}); err == nil {
			t.Error("a post with no id has a page to view")
		}
	}
}

// TestBuildRefusesAnArticleWithNoID: the builder itself never writes an
// article whose id cannot name a folder, whatever hands it one.
func TestBuildRefusesAnArticleWithNoID(t *testing.T) {
	svc, store := newService(t)
	if _, err := store.CreateSite("wen", "W", "", "plain"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Site("wen")
	posts, _ := svc.fullPosts("wen")
	in, err := svc.input(st, posts)
	if err != nil {
		t.Fatal(err)
	}
	slug := "week"
	for _, id := range []string{"", ".", "..", "a/b"} {
		in.Out = filepath.Join(t.TempDir(), "out")
		in.Articles = []*planet.MyArticle{{ID: id, Title: "No id", Slug: &slug, Content: "words", Tags: map[string]string{}}}
		if _, err := build.Build(in); err == nil || !strings.Contains(err.Error(), "has no id") {
			t.Errorf("an article with the id %q: %v", id, err)
		}
		if _, err := os.Stat(filepath.Join(in.Out, "week")); err == nil {
			t.Errorf("an article with the id %q was written", id)
		}
	}
}
