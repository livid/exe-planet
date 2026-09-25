package site

import (
	"strings"
	"testing"
	"time"
)

func TestParseDocRoundTrip(t *testing.T) {
	in := "---\nid: 339A627C-0AFB-4505-BFE2-75B79C7F5E84\ntitle: Hello, world: a subtitle\ncreated: 2026-09-25T10:00:00+08:00\ntags: charts, macOS\nhero: photo.jpg\ncolor: red\n---\n\n# Hi\n\nbody `code`\n"
	m, body, err := ParseDoc([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "339A627C-0AFB-4505-BFE2-75B79C7F5E84" || m.Title != "Hello, world: a subtitle" || m.Hero != "photo.jpg" {
		t.Fatalf("meta %+v", m)
	}
	if got := m.Created.Format(time.RFC3339); got != "2026-09-25T10:00:00+08:00" {
		t.Fatalf("created %s", got)
	}
	if strings.Join(m.Tags, "|") != "charts|macOS" {
		t.Fatalf("tags %v", m.Tags)
	}
	if m.Extra["color"] != "red" {
		t.Fatalf("extra %v", m.Extra)
	}
	if body != "# Hi\n\nbody `code`\n" {
		t.Fatalf("body %q", body)
	}
	if out := string(FormatDoc(m, body)); out != in {
		t.Fatalf("round trip differs:\n%s\n---want---\n%s", out, in)
	}
}

func TestParseDocForms(t *testing.T) {
	m, body, err := ParseDoc([]byte("just text\n"))
	if err != nil || m.ID != "" || body != "just text\n" {
		t.Fatalf("no front matter: %v %+v %q", err, m, body)
	}
	m, _, _ = ParseDoc([]byte("---\ntags: a, b, A, ,b\n---\n"))
	if strings.Join(m.Tags, "|") != "a|b" {
		t.Fatalf("tags folded to %v", m.Tags)
	}
	m, body, err = ParseDoc([]byte("---\r\ntitle: T\r\n---\r\nbody\r\n"))
	if err != nil || m.Title != "T" || body != "body\n" {
		t.Fatalf("crlf: %v %+v %q", err, m, body)
	}
	m, body, err = ParseDoc([]byte("---\ntitle: T\n---"))
	if err != nil || m.Title != "T" || body != "" {
		t.Fatalf("unterminated body: %v %+v %q", err, m, body)
	}
	if _, _, err = ParseDoc([]byte("---\ntitle: T\n")); err == nil {
		t.Fatal("unclosed front matter accepted")
	}
	if _, _, err = ParseDoc([]byte("---\nnot a pair\n---\n")); err == nil {
		t.Fatal("line without colon accepted")
	}
	if _, _, err = ParseDoc([]byte("---\ncreated: yesterday\n---\n")); err == nil {
		t.Fatal("bad date accepted")
	}
	m, _, err = ParseDoc([]byte("---\ncreated: 2026-09-25\nmodified: 2026-09-25 14:30\n---\n"))
	if err != nil || m.Created.Day() != 25 || m.Modified.Hour() != 14 || m.Modified.Minute() != 30 {
		t.Fatalf("short dates: %v %+v", err, m)
	}
}

func TestFormatDocOmitsEmpty(t *testing.T) {
	out := string(FormatDoc(Meta{ID: "X", Title: "T"}, ""))
	if out != "---\nid: X\ntitle: T\n---\n" {
		t.Fatalf("%q", out)
	}
	out = string(FormatDoc(Meta{ID: "X"}, "no trailing newline"))
	if !strings.HasSuffix(out, "---\n\nno trailing newline\n") {
		t.Fatalf("%q", out)
	}
}
