package markdown

import (
	"strings"
	"testing"
)

// TestTodoItems: the to-do items written as bullets at the margin, in
// order, each with the byte that says whether it is done; and nothing
// that only looks like one.
func TestTodoItems(t *testing.T) {
	src := strings.Join([]string{
		"A week.",
		"",
		"- [x] one, done",
		"- [ ] two, with **bold** and a [link](#x)",
		"- a plain bullet",
		"  - [ ] nested: not counted",
		"  - [x] nor this",
		"",
		"1. [x] numbered: not counted",
		"",
		"* [X] three, a star and a capital",
		"",
		"+ [ ] four, loose",
		"",
		"+ [x] five, loose",
		"",
		"> - [ ] quoted: not counted",
		"",
		"```",
		"- [ ] in a code block: not counted",
		"```",
		"",
		"   - [ ] six, three spaces in",
		"- [ ]",
		"- [-] not a box",
		"",
	}, "\n")
	for _, c := range []struct{ name, src string }{{"LF", src}, {"CRLF", strings.ReplaceAll(src, "\n", "\r\n")}} {
		items := TodoItems([]byte(c.src))
		var got []string
		for _, it := range items {
			if c.src[it.At-1] != '[' || c.src[it.At+1] != ']' {
				t.Fatalf("%s: At %d is not between brackets: %q", c.name, it.At, c.src[it.At-1:it.At+2])
			}
			if done := c.src[it.At] != ' '; done != it.Done {
				t.Errorf("%s: item at %d: Done %v, the source says %q", c.name, it.At, it.Done, c.src[it.At])
			}
			line := c.src[it.At+2:]
			line, _, _ = strings.Cut(line, "\n")
			got = append(got, strings.TrimSpace(strings.SplitN(line, ",", 2)[0]))
		}
		// (the seventh is "- [ ]" with no words: the page draws its box too)
		want := "one|two|three|four|five|six|"
		if strings.Join(got, "|") != want {
			t.Errorf("%s: items %q, want %q", c.name, strings.Join(got, "|"), want)
		}
		// the page numbers the same items: one <li> with its own checkbox for each, at the top level
		page := Render(c.src)
		if n := strings.Count(page, "\n<li><input type=\"checkbox\"") + strings.Count(page, "\n<li>\n<p><input type=\"checkbox\""); n < len(items) {
			t.Errorf("%s: the page has %d boxed items, fewer than the %d found", c.name, n, len(items))
		}
	}
	if items := TodoItems([]byte("no lists here\n")); len(items) != 0 {
		t.Errorf("a document with no lists has %d items", len(items))
	}
	if items := TodoItems(nil); len(items) != 0 {
		t.Errorf("an empty document has %d items", len(items))
	}
}
