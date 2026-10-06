package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livid/exe-planet/internal/site"
)

// TestTickTodo: a to-do item ticked from the post's page changes one
// byte of the post's file, the mark between that item's brackets, and
// nothing else; the items are the bullets at the margin, counted from
// one; a file whose items no longer stand as the page showed them is
// left alone; and the page a post is viewed as carries the script that
// sends the click.
func TestTickTodo(t *testing.T) {
	svc, store := newService(t)
	if _, err := store.CreateSite("wen", "空山", "", "paper"); err != nil {
		t.Fatal(err)
	}
	body := "这一周的功课。\n\n- [x] 磨墨\n- [ ] 临前二十个字\n- 一句话\n  - [ ] 它下面的一件事\n\n1. [x] 把砚台润湿\n\n- [ ] 空一行的清单\n\n- [x] 读起来也一样\n\n> - [ ] 引文里的一件事\n\n```\n- [ ] 代码里的\n```\n"
	p, err := store.CreatePost("wen", site.KindPost, "week", site.Meta{Title: "功课"}, body)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(p.Dir, "index.md")
	read := func() string {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	// one byte, and which
	diff := func(a, b string) (at []int) {
		if len(a) != len(b) {
			t.Fatalf("the file's length changed: %d → %d", len(a), len(b))
		}
		for i := range a {
			if a[i] != b[i] {
				at = append(at, i)
			}
		}
		return at
	}
	before := read()
	if !strings.HasPrefix(before, "---\n") {
		t.Fatalf("the post has no front matter to step over: %q", before[:20])
	}

	// item 2, as the page showed the four items: done, open, open, done
	if err := svc.TickTodo("wen", "week", "", 2, true, "1001"); err != nil {
		t.Fatal(err)
	}
	after := read()
	at := diff(before, after)
	if len(at) != 1 || after[at[0]] != 'x' || !strings.HasPrefix(after[at[0]-3:], "- [x] 临前二十个字") {
		t.Fatalf("ticking item 2 changed %v: %q", at, after)
	}
	// and back
	if err := svc.TickTodo("wen", "week", "post", 2, false, "1101"); err != nil {
		t.Fatal(err)
	}
	if read() != before {
		t.Errorf("unticking item 2 did not give the file back: %q", read())
	}
	// the loose list's items are 3 and 4; the nested, numbered, quoted and code-block lines are not items
	if err := svc.TickTodo("wen", "week", "", 3, true, "1001"); err != nil {
		t.Fatal(err)
	}
	if after = read(); len(diff(before, after)) != 1 || !strings.Contains(after, "- [x] 空一行的清单") {
		t.Errorf("ticking item 3: %q", after)
	}
	if err := svc.TickTodo("wen", "week", "", 4, false, "1011"); err != nil {
		t.Fatal(err)
	}
	if after = read(); !strings.Contains(after, "- [ ] 读起来也一样") || !strings.Contains(after, "  - [ ] 它下面的一件事") || !strings.Contains(after, "1. [x] 把砚台润湿") || !strings.Contains(after, "> - [ ] 引文里的一件事") || !strings.Contains(after, "```\n- [ ] 代码里的") {
		t.Errorf("ticking item 4 touched another line: %q", after)
	}
	now := read()

	// an item already as asked: nothing written
	if err := svc.TickTodo("wen", "week", "", 1, true, "1010"); err != nil || read() != now {
		t.Errorf("an item already done: %v, changed %v", err, read() != now)
	}
	// the page is behind the file: refused, nothing written
	if err := svc.TickTodo("wen", "week", "", 2, true, "1001"); !errors.Is(err, site.ErrChanged) || read() != now {
		t.Errorf("a page behind its file: %v, changed %v", err, read() != now)
	}
	// no such item, post or kind
	if err := svc.TickTodo("wen", "week", "", 5, true, "1010"); !errors.Is(err, site.ErrNotFound) {
		t.Errorf("item 5 of 4: %v", err)
	}
	if err := svc.TickTodo("wen", "week", "", 0, true, ""); !errors.Is(err, site.ErrNotFound) {
		t.Errorf("item 0: %v", err)
	}
	if err := svc.TickTodo("wen", "nope", "", 1, true, ""); !errors.Is(err, site.ErrNotFound) {
		t.Errorf("a post that is not there: %v", err)
	}
	if err := svc.TickTodo("wen", "week", "draft", 1, true, ""); !errors.Is(err, site.ErrNotFound) {
		t.Errorf("the post asked for as a draft: %v", err)
	}
	if read() != now {
		t.Error("a refused tick wrote the file")
	}

	// a file with Windows line endings and no front matter keeps them
	crlf := "- [ ] one\r\n- [x] two\r\n"
	if err := os.WriteFile(file, []byte(crlf), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.TickTodo("wen", "week", "", 1, true, "01"); err != nil || read() != "- [x] one\r\n- [x] two\r\n" {
		t.Errorf("a CRLF file with no front matter: %v, %q", err, read())
	}
	crlf = "---\r\ntitle: T\r\n---\r\n\r\n- [ ] one\r\n- [x] two\r\n"
	os.WriteFile(file, []byte(crlf), 0o644)
	if err := svc.TickTodo("wen", "week", "", 2, false, "01"); err != nil || read() != "---\r\ntitle: T\r\n---\r\n\r\n- [ ] one\r\n- [ ] two\r\n" {
		t.Errorf("a CRLF file with front matter: %v, %q", err, read())
	}

	// a code example that only looks like an item, before a real one: the
	// real one is item 1 here, where a count of lines opening "- [ ] "
	// (Planet's) would flip the example (Codex on Spark's fixture)
	fenced := "```\n- [ ] example\n```\n\n- [ ] task\n"
	os.WriteFile(file, []byte(fenced), 0o644)
	if err := svc.TickTodo("wen", "week", "", 1, true, "0"); err != nil || read() != "```\n- [ ] example\n```\n\n- [x] task\n" {
		t.Errorf("a fenced example before a task: %v, %q", err, read())
	}

	// the page the window shows carries the click's script, after the links' own
	if err := os.WriteFile(file, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Build("wen"); err != nil {
		t.Fatal(err)
	}
	page, err := svc.View("wen", "week", Attachments{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`parent.postMessage({exe:"page-todo",item:k+1,done:box.checked,was:was},"*")`,
		`document.querySelectorAll("ul > li")`, `!x.parentElement.closest("li, blockquote")`,
		`parent.postMessage({exe:"page-scroll",y:Math.round(scrollY)},"*")`,
		`parent.postMessage({exe:"page-link"`, `li.id = "todo-item-" + (++n);`,
		// a link to a place in the page is followed in the page, not handed to the frame
		`if(h.charAt(0)==="#"){try{var t=document.getElementById(decodeURIComponent(h.slice(1)));if(t)t.scrollIntoView()}catch(x){}return}`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the viewed page lacks %q", want)
		}
	}
	if i, j := strings.Index(page, `exe:"page-todo"`), strings.LastIndex(page, "</body>"); i < 0 || i > j {
		t.Error("the to-do script is not inside the page's body")
	}
}
