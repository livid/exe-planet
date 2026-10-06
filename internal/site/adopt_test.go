package site

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestIssue: an id and a date written into a file's front matter leave
// every other byte where it was, in the file's own line endings.
func TestIssue(t *testing.T) {
	const id, at = "339A627C-0AFB-4505-BFE2-75B79C7F5E84", "2026-10-06T10:00:00Z"
	for _, c := range []struct{ name, in, id, created, want string }{
		{"no front matter", "Words.\n", id, at, "---\nid: " + id + "\ncreated: " + at + "\n---\n\nWords.\n"},
		{"an empty file", "", id, at, "---\nid: " + id + "\ncreated: " + at + "\n---\n"},
		{"front matter with neither", "---\ntitle: T\nweight: 3\n---\n\nWords.\n", id, at, "---\nid: " + id + "\ntitle: T\nweight: 3\ncreated: " + at + "\n---\n\nWords.\n"},
		{"only the id wanting", "---\ntitle: T\ncreated: 2026-01-02T03:04:05Z\n---\n\nWords.\n", id, "", "---\nid: " + id + "\ntitle: T\ncreated: 2026-01-02T03:04:05Z\n---\n\nWords.\n"},
		{"only the date wanting", "---\nid: X\ntitle: T\n---\nWords.", "", at, "---\nid: X\ntitle: T\ncreated: " + at + "\n---\nWords."},
		{"an id line left empty", "---\ntitle: T\nid:\n---\n", id, "", "---\ntitle: T\nid: " + id + "\n---\n"},
		{"Windows line endings", "---\r\ntitle: T\r\n---\r\n\r\nWords.\r\n", id, at, "---\r\nid: " + id + "\r\ntitle: T\r\ncreated: " + at + "\r\n---\r\n\r\nWords.\r\n"},
		{"Windows line endings, no front matter", "Words.\r\nMore.\r\n", id, "", "---\r\nid: " + id + "\r\n---\r\n\r\nWords.\r\nMore.\r\n"},
		{"front matter closed at the file's end", "---\ntitle: T\n---", id, "", "---\nid: " + id + "\ntitle: T\n---"},
		{"front matter not closed", "---\ntitle: T\nWords.\n", id, at, "---\ntitle: T\nWords.\n"},
		{"nothing to issue", "---\ntitle: T\n---\n", "", "", "---\ntitle: T\n---\n"},
	} {
		got := string(Issue([]byte(c.in), c.id, c.created))
		if got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
			continue
		}
		// what was there reads as it did, with what was issued beside it
		if c.name == "front matter not closed" {
			continue
		}
		before, bodyBefore, _ := ParseDoc([]byte(c.in))
		after, bodyAfter, err := ParseDoc([]byte(got))
		if err != nil || bodyAfter != bodyBefore || after.Title != before.Title || after.Extra["weight"] != before.Extra["weight"] {
			t.Errorf("%s: the document changed: %v, body %q → %q", c.name, err, bodyBefore, bodyAfter)
		}
		if c.id != "" && after.ID != c.id {
			t.Errorf("%s: id reads %q", c.name, after.ID)
		}
		if c.created != "" && after.Created.Format(time.RFC3339) != c.created {
			t.Errorf("%s: created reads %v", c.name, after.Created)
		}
	}
}

// TestAdoptPosts: a post made by hand — a folder and an index.md, no
// id, no date — is issued both, once, into its file; the id is the same
// on any node that meets the same file; a post that has them is not
// touched.
func TestAdoptPosts(t *testing.T) {
	s := testStore(t)
	st, err := s.CreateSite("notes", "Notes", "", "")
	if err != nil {
		t.Fatal(err)
	}
	made, err := s.CreatePost("notes", KindPost, "made", Meta{Title: "Made here"}, "By the app.\n")
	if err != nil {
		t.Fatal(err)
	}
	write := func(kind, slug, doc string, at time.Time) string {
		dir := filepath.Join(st.Dir, kindDirs[kind], slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, "index.md")
		if err := os.WriteFile(file, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(file, at, at); err != nil {
			t.Fatal(err)
		}
		return file
	}
	read := func(file string) string { b, _ := os.ReadFile(file); return string(b) }
	written := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	plain := write(KindPost, "by-hand", "Just words.\n", written)
	titled := write(KindPost, "titled", "---\ntitle: Titled\nweight: 2\n---\n\nWords.\n", written.Add(time.Hour))
	dated := write(KindPage, "dated", "---\ntitle: Dated\ncreated: 2025-01-02T03:04:05Z\n---\n\nA page.\n", written)
	draft := write(KindDraft, "a-draft", "A draft.\n", written)
	broken := write(KindPost, "broken", "---\ntitle: never closed\n", written)
	madeFile := filepath.Join(made.Dir, "index.md")
	madeBefore, brokenBefore := read(madeFile), read(broken)

	adopted, err := s.AdoptPosts("notes")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(adopted, ","); got != "by-hand,titled,dated,a-draft" {
		t.Errorf("adopted %q", got)
	}
	if read(madeFile) != madeBefore || read(broken) != brokenBefore {
		t.Error("a post that wanted nothing, or one that does not read, was written to")
	}
	uuid := regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-5[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$`)
	ids := map[string]string{}
	for slug, c := range map[string]struct {
		kind, body string
		created    time.Time
	}{
		"by-hand": {KindPost, "Just words.\n", written},
		"titled":  {KindPost, "Words.\n", written.Add(time.Hour)},
		"dated":   {KindPage, "A page.\n", time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)},
		"a-draft": {KindDraft, "A draft.\n", written},
	} {
		p, err := s.Post("notes", slug, c.kind)
		if err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
		if !uuid.MatchString(p.ID) {
			t.Errorf("%s: id %q is not a version 5 UUID as newID writes one", slug, p.ID)
		}
		if !p.Created.Equal(c.created) {
			t.Errorf("%s: created %v, want the day the file was written, %v", slug, p.Created, c.created)
		}
		if p.Body != c.body {
			t.Errorf("%s: body %q", slug, p.Body)
		}
		ids[slug] = p.ID
	}
	if len(map[string]bool{ids["by-hand"]: true, ids["titled"]: true, ids["dated"]: true, ids["a-draft"]: true, made.ID: true}) != 5 {
		t.Errorf("ids are not all different: %v", ids)
	}
	if p, _ := s.Post("notes", "titled", ""); p.Title != "Titled" || p.Extra["weight"] != "2" {
		t.Errorf("titled lost its own lines: %+v", p.Meta)
	}

	// once: nothing is left to issue, and no file changes
	snapshot := map[string]string{}
	for _, f := range []string{plain, titled, dated, draft, madeFile, broken} {
		snapshot[f] = read(f)
	}
	if adopted, err = s.AdoptPosts("notes"); err != nil || len(adopted) != 0 {
		t.Errorf("a second pass adopted %v, %v", adopted, err)
	}
	for f, was := range snapshot {
		if read(f) != was {
			t.Errorf("a second pass wrote %s", f)
		}
	}

	// another node, the same site and the same file: the same id
	other := New(filepath.Join(t.TempDir(), "Planet"))
	other.Init()
	if err := os.MkdirAll(filepath.Join(other.Root, "notes", "posts", "by-hand"), 0o755); err != nil {
		t.Fatal(err)
	}
	sj, _ := os.ReadFile(filepath.Join(st.Dir, "site.json"))
	os.WriteFile(filepath.Join(other.Root, "notes", "site.json"), sj, 0o644)
	os.WriteFile(filepath.Join(other.Root, "notes", "posts", "by-hand", "index.md"), []byte("Just words.\n"), 0o644)
	if _, err := other.AdoptPosts("notes"); err != nil {
		t.Fatal(err)
	}
	if p, err := other.Post("notes", "by-hand", ""); err != nil || p.ID != ids["by-hand"] {
		t.Errorf("another node issued %v (%v), this one %s", p, err, ids["by-hand"])
	}

	// a post renamed away keeps its id; a new one made under the old name gets another
	if err := os.Rename(filepath.Join(st.Dir, "posts", "by-hand"), filepath.Join(st.Dir, "posts", "renamed")); err != nil {
		t.Fatal(err)
	}
	write(KindPost, "by-hand", "Other words.\n", written)
	if adopted, err = s.AdoptPosts("notes"); err != nil || strings.Join(adopted, ",") != "by-hand" {
		t.Fatalf("after the rename: adopted %v, %v", adopted, err)
	}
	renamed, _ := s.Post("notes", "renamed", "")
	again, _ := s.Post("notes", "by-hand", "")
	if renamed.ID != ids["by-hand"] || again.ID == renamed.ID || !uuid.MatchString(again.ID) {
		t.Errorf("renamed %s (was %s), the new by-hand %s", renamed.ID, ids["by-hand"], again.ID)
	}

	if _, err := s.AdoptPosts("nope"); err == nil {
		t.Error("a site that is not there adopted")
	}
}
