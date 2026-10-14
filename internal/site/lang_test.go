package site

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLangTag(t *testing.T) {
	for in, want := range map[string]string{
		"en": "en", "JA": "ja", "zh-hans": "zh-Hans", "ZH-HANT": "zh-Hant", "pt-br": "pt-BR",
		"zh-Hans-CN": "zh-Hans-CN", "es-419": "es-419", " ko ": "ko",
	} {
		if got, ok := LangTag(in); !ok || got != want {
			t.Errorf("LangTag(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	// three letters are files a person keeps, not languages; so is anything else
	for _, in := range []string{"", "e", "old", "bak", "new", "zh_CN", "zh-", "english", "v2", "en-", "zh-Hans-", "中文"} {
		if got, ok := LangTag(in); ok {
			t.Errorf("LangTag(%q) = %q, want none", in, got)
		}
	}
}

func TestLangFile(t *testing.T) {
	for name, want := range map[string]string{
		"index.ja.md": "ja", "index.zh-hans.md": "zh-Hans", "index.zh-Hant.md": "zh-Hant", "index.EN.md": "en",
	} {
		if got, ok := LangFile(name); !ok || got != want {
			t.Errorf("LangFile(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"index.md", "index.old.md", "index.ja.txt", "notes.ja.md", "index.ja.md.bak", "Index.ja.md", "index..md"} {
		if got, ok := LangFile(name); ok {
			t.Errorf("LangFile(%q) = %q, want none", name, got)
		}
	}
	if LangFileName("zh-Hans") != "index.zh-hans.md" {
		t.Errorf("LangFileName: %q", LangFileName("zh-Hans"))
	}
}

// A post's folder may hold the post in other languages, as
// index.<language>.md files: they are its versions, not its attachments,
// and no attachment can be put under such a name.
func TestPostVersions(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "Planet"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSite("wen", "Wen", "", "paper"); err != nil {
		t.Fatal(err)
	}
	p, err := store.CreatePost("wen", KindPost, "zhi", Meta{Title: "纸的纹理"}, "纸也有脾气。\n")
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(p.Dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.ja.md", "---\ntitle: 紙の肌理\n---\n\n紙にも気性がある。\n")
	write("index.en.md", "Paper has a temper too.\n")              // no front matter: no title of its own
	write("index.zh-Hant.md", "---\ntitle: 紙的紋理\n---\n\n紙也有脾氣。\n") // the tag in its usual case
	write("index.ko.md", "---\ntitle: broken\n")                   // front matter never closed
	write("index.old.md", "kept by hand\n")                        // a file, not a language
	write("photo.jpg", "jpeg")

	got, err := store.Post("wen", "zhi", "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Attachments, []string{"index.old.md", "photo.jpg"}) {
		t.Errorf("attachments: %v", got.Attachments)
	}
	if len(got.Versions) != 4 {
		t.Fatalf("versions: %+v", got.Versions)
	}
	want := []Version{
		{Lang: "en", File: "index.en.md", Body: "Paper has a temper too.\n"},
		{Lang: "ja", File: "index.ja.md", Title: "紙の肌理", Body: "紙にも気性がある。\n"},
		{Lang: "ko", File: "index.ko.md", Error: "index.ko.md: front matter is not closed"},
		{Lang: "zh-Hant", File: "index.zh-Hant.md", Title: "紙的紋理", Body: "紙也有脾氣。\n"},
	}
	if !reflect.DeepEqual(got.Versions, want) {
		t.Errorf("versions:\n got %+v\nwant %+v", got.Versions, want)
	}
	if got.Title != "纸的纹理" || got.Body != "纸也有脾气。\n" {
		t.Errorf("the post itself: %q %q", got.Title, got.Body)
	}

	// a listing names the languages and leaves the texts out, as it does the post's
	list, err := store.Posts("wen", "")
	if err != nil || len(list) != 1 {
		t.Fatalf("posts: %v %v", list, err)
	}
	for _, v := range list[0].Versions {
		if v.Body != "" {
			t.Errorf("a listing carries %s's text", v.Lang)
		}
	}
	if len(list[0].Versions) != 4 || list[0].Versions[1].Title != "紙の肌理" {
		t.Errorf("listed versions: %+v", list[0].Versions)
	}

	// two names for one language: the one in lower case is read
	write("index.JA.md", "---\ntitle: shouted\n---\n\nSHOUTED\n")
	got, _ = store.Post("wen", "zhi", "")
	if len(got.Versions) != 4 || got.Versions[1].File != "index.ja.md" || got.Versions[1].Title != "紙の肌理" {
		t.Errorf("case twins: %+v", got.Versions)
	}
	if !reflect.DeepEqual(got.Attachments, []string{"index.old.md", "photo.jpg"}) {
		t.Errorf("attachments with a twin: %v", got.Attachments)
	}

	// the attachment routes keep out of the languages' names
	for _, name := range []string{"index.fr.md", "index.zh-Hans.md"} {
		if _, err := store.AddAttachment("wen", "zhi", "", name, strings.NewReader("x")); !errors.Is(err, ErrInvalid) {
			t.Errorf("AddAttachment(%s): %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(p.Dir, name)); err == nil {
			t.Errorf("%s was written as an attachment", name)
		}
	}
	if _, err := store.AttachmentPath("wen", "zhi", "", "index.ja.md"); !errors.Is(err, ErrInvalid) {
		t.Errorf("AttachmentPath(index.ja.md): %v", err)
	}
	if err := store.DeleteAttachment("wen", "zhi", "", "index.ja.md"); !errors.Is(err, ErrInvalid) {
		t.Errorf("DeleteAttachment(index.ja.md): %v", err)
	}
	if _, err := store.AddAttachment("wen", "zhi", "", "index.old.md", strings.NewReader("y")); err != nil {
		t.Errorf("index.old.md is a file like any other: %v", err)
	}
}
