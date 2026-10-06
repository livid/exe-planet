package markdown

import (
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// TodoItem is one to-do item of a document ("- [ ] words"): whether it is
// done, and where in the source the mark between its brackets stands, the
// one byte that says so.
type TodoItem struct {
	Done bool
	At   int
}

// TodoItems lists the to-do items a document writes as bullets at the
// margin, in reading order: the items a template numbers todo-item-1..N
// on a post's page (the direct children of its content's own lists), and
// so the ones that can be ticked from the page. An item inside another
// item, a numbered one, one in a quotation and a line that only looks
// like one inside a code block are not among them. It reads the source
// with the parser Render writes the page with, so the two agree on what
// an item is; src is the Markdown as the file has it, and At counts in it.
func TodoItems(src []byte) []TodoItem {
	doc := Default().(*gm).md.Parser().Parse(text.NewReader(src))
	var items []TodoItem
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		list, ok := n.(*ast.List)
		if !ok || list.IsOrdered() {
			continue
		}
		for li := list.FirstChild(); li != nil; li = li.NextSibling() {
			block := li.FirstChild()
			if block == nil {
				continue
			}
			box, ok := block.FirstChild().(*east.TaskCheckBox)
			if !ok {
				continue
			}
			// the box opens the item's first line: "[ ]", "[x]" or "[X]"
			lines := block.Lines()
			if lines == nil || lines.Len() == 0 {
				continue
			}
			at := lines.At(0).Start
			if at+2 >= len(src) || src[at] != '[' || src[at+2] != ']' {
				continue
			}
			items = append(items, TodoItem{Done: box.IsChecked, At: at + 1})
		}
	}
	return items
}
