package stencil

import (
	"fmt"
	"strings"
)

type node interface {
	render(c *Context, w *strings.Builder) error
}

type textNode struct{ text string }

func (n *textNode) render(c *Context, w *strings.Builder) error {
	w.WriteString(n.text)
	return nil
}

type variableNode struct{ v *variable }

func (n *variableNode) render(c *Context, w *strings.Builder) error {
	val, err := n.v.resolve(c)
	if err != nil {
		return err
	}
	w.WriteString(stringify(val))
	return nil
}

type ifBranch struct {
	cond  expr // nil for else
	nodes []node
}

type ifNode struct{ branches []ifBranch }

func (n *ifNode) render(c *Context, w *strings.Builder) error {
	for _, b := range n.branches {
		if b.cond == nil {
			return renderNodes(b.nodes, c, w)
		}
		ok, err := b.cond.eval(c)
		if err != nil {
			return err
		}
		if ok {
			return renderNodes(b.nodes, c, w)
		}
	}
	return nil
}

type forNode struct {
	vars  []string
	src   *variable
	where expr
	body  []node
	empty []node
}

func (n *forNode) render(c *Context, w *strings.Builder) error {
	val, err := n.src.resolve(c)
	if err != nil {
		return err
	}
	items := iterate(val)
	if n.where != nil {
		var kept []any
		for _, it := range items {
			c.push(n.bind(it))
			ok, err := n.where.eval(c)
			c.pop()
			if err != nil {
				return err
			}
			if ok {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	if len(items) == 0 {
		return renderNodes(n.empty, c, w)
	}
	parent, _ := c.lookup("forloop")
	for i, it := range items {
		loop := map[string]any{
			"first":    i == 0,
			"last":     i == len(items)-1,
			"counter":  i + 1,
			"counter0": i,
			"length":   len(items),
		}
		if parent != nil {
			loop["parentloop"] = parent
		}
		m := n.bind(it)
		m["forloop"] = loop
		c.push(m)
		err := renderNodes(n.body, c, w)
		c.pop()
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *forNode) bind(it any) map[string]any {
	m := map[string]any{}
	if len(n.vars) == 1 {
		m[n.vars[0]] = it
		return m
	}
	pair, _ := it.([]any)
	for i, v := range n.vars {
		if i < len(pair) {
			m[v] = pair[i]
		} else {
			m[v] = nil
		}
	}
	return m
}

// iterate lists what a for loop walks: an array's items, a map's sorted
// (key, value) pairs, a string's characters.
func iterate(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case map[string]any:
		keys := sortedKeys(x)
		out := make([]any, len(keys))
		for i, k := range keys {
			out[i] = []any{k, x[k]}
		}
		return out
	case string:
		var out []any
		for _, r := range x {
			out = append(out, string(r))
		}
		return out
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}

type blockNode struct {
	name  string
	nodes []node
}

func (n *blockNode) render(c *Context, w *strings.Builder) error {
	if c.blocks != nil {
		if child := c.blocks.pop(n.name); child != nil {
			// The child's block renders in place of this one; block.super
			// inside it renders this one.
			var super strings.Builder
			if err := renderNodes(n.nodes, c, &super); err != nil {
				return err
			}
			c.push(map[string]any{"block": map[string]any{"super": super.String()}})
			err := renderNodes(child.nodes, c, w)
			c.pop()
			return err
		}
	}
	return renderNodes(n.nodes, c, w)
}

type extendsNode struct {
	parent *variable
	blocks map[string]*blockNode
}

func (n *extendsNode) render(c *Context, w *strings.Builder) error {
	name, err := n.parent.resolve(c)
	if err != nil {
		return err
	}
	tpl, err := c.env.template(stringify(name))
	if err != nil {
		return err
	}
	if c.blocks == nil {
		c.blocks = &blockContext{blocks: map[string][]*blockNode{}}
	}
	for name, b := range n.blocks {
		c.blocks.push(name, b)
	}
	return renderNodes(tpl.nodes, c, w)
}

// blockContext queues each block name's overrides, most derived first.
type blockContext struct {
	blocks map[string][]*blockNode
}

func (b *blockContext) push(name string, n *blockNode) {
	b.blocks[name] = append(b.blocks[name], n)
}

func (b *blockContext) pop(name string) *blockNode {
	q := b.blocks[name]
	if len(q) == 0 {
		return nil
	}
	b.blocks[name] = q[1:]
	return q[0]
}

type includeNode struct {
	name *variable
	ctx  string
}

func (n *includeNode) render(c *Context, w *strings.Builder) error {
	name, err := n.name.resolve(c)
	if err != nil {
		return err
	}
	tpl, err := c.env.template(stringify(name))
	if err != nil {
		return err
	}
	if n.ctx != "" {
		sub, _ := c.lookup(n.ctx)
		m, _ := sub.(map[string]any)
		if m == nil {
			m = map[string]any{}
		}
		c.push(m)
		defer c.pop()
	}
	return renderNodes(tpl.nodes, c, w)
}

func renderNodes(nodes []node, c *Context, w *strings.Builder) error {
	for _, n := range nodes {
		if err := n.render(c, w); err != nil {
			return err
		}
	}
	return nil
}

// parser turns tokens into nodes.
type parser struct {
	name string
	toks []token
	pos  int
}

func parse(name, src string) ([]node, error) {
	toks, err := lex(name, src)
	if err != nil {
		return nil, err
	}
	applyTrim(toks)
	p := &parser{name: name, toks: toks}
	nodes, stop, err := p.parseUntil(nil)
	if err != nil {
		return nil, err
	}
	if stop != "" {
		return nil, p.errorf("unexpected %s", stop)
	}
	return nodes, nil
}

// applyTrim honours {%- and -%} by trimming the neighbouring text.
func applyTrim(toks []token) {
	for i, t := range toks {
		if t.kind != tokBlock {
			continue
		}
		if t.trimLeft && i > 0 && toks[i-1].kind == tokText {
			toks[i-1].contents = strings.TrimRight(toks[i-1].contents, " \t\r\n")
		}
		if t.trimRight && i+1 < len(toks) && toks[i+1].kind == tokText {
			toks[i+1].contents = strings.TrimLeft(toks[i+1].contents, " \t\r\n")
		}
	}
}

func (p *parser) errorf(format string, args ...any) error {
	line := 0
	if p.pos > 0 && p.pos-1 < len(p.toks) {
		line = p.toks[p.pos-1].line
	}
	return &Error{Template: p.name, Line: line, Msg: fmt.Sprintf(format, args...)}
}

// parseUntil parses nodes until a block tag named in stops, returning
// which one ended it (its components are left for the caller).
func (p *parser) parseUntil(stops []string) ([]node, string, error) {
	var nodes []node
	for p.pos < len(p.toks) {
		t := p.toks[p.pos]
		p.pos++
		switch t.kind {
		case tokText:
			nodes = append(nodes, &textNode{t.contents})
		case tokComment:
		case tokVariable:
			v, err := parseVariable(t.contents)
			if err != nil {
				return nil, "", p.errorf("%v", err)
			}
			nodes = append(nodes, &variableNode{v})
		case tokBlock:
			comps := components(t.contents)
			if len(comps) == 0 {
				return nil, "", p.errorf("empty tag")
			}
			for _, s := range stops {
				if comps[0] == s {
					p.pos-- // leave the stop for the caller
					return nodes, s, nil
				}
			}
			n, err := p.parseTag(comps)
			if err != nil {
				return nil, "", err
			}
			if n != nil {
				nodes = append(nodes, n)
			}
		}
	}
	return nodes, "", nil
}

func (p *parser) take() []string {
	t := p.toks[p.pos]
	p.pos++
	return components(t.contents)
}

func (p *parser) parseTag(comps []string) (node, error) {
	switch comps[0] {
	case "if", "ifnot":
		return p.parseIf(comps)
	case "for":
		return p.parseFor(comps)
	case "block":
		if len(comps) != 2 {
			return nil, p.errorf("'block' takes one argument, the block name")
		}
		body, stop, err := p.parseUntil([]string{"endblock"})
		if err != nil {
			return nil, err
		}
		if stop == "" {
			return nil, p.errorf("block %s is not closed", comps[1])
		}
		p.take()
		return &blockNode{name: comps[1], nodes: body}, nil
	case "extends":
		if len(comps) != 2 {
			return nil, p.errorf("'extends' takes one argument, the template file to be extended")
		}
		parent, err := parseVariable(comps[1])
		if err != nil {
			return nil, p.errorf("%v", err)
		}
		rest, stop, err := p.parseUntil(nil)
		if err != nil {
			return nil, err
		}
		if stop != "" {
			return nil, p.errorf("unexpected %s", stop)
		}
		blocks := map[string]*blockNode{}
		for _, n := range rest {
			if b, ok := n.(*blockNode); ok {
				blocks[b.name] = b
			}
		}
		return &extendsNode{parent: parent, blocks: blocks}, nil
	case "include":
		if len(comps) < 2 || len(comps) > 3 {
			return nil, p.errorf("'include' takes a template name and an optional context")
		}
		name, err := parseVariable(comps[1])
		if err != nil {
			return nil, p.errorf("%v", err)
		}
		n := &includeNode{name: name}
		if len(comps) == 3 {
			n.ctx = comps[2]
		}
		return n, nil
	}
	return nil, p.errorf("unknown tag %q", comps[0])
}

func (p *parser) parseIf(comps []string) (node, error) {
	n := &ifNode{}
	cond, err := parseExpr(comps[1:])
	if err != nil {
		return nil, p.errorf("%v", err)
	}
	if comps[0] == "ifnot" {
		cond = &notExpr{cond}
	}
	for {
		body, stop, err := p.parseUntil([]string{"elif", "else", "endif"})
		if err != nil {
			return nil, err
		}
		n.branches = append(n.branches, ifBranch{cond: cond, nodes: body})
		switch stop {
		case "":
			return nil, p.errorf("if is not closed")
		case "endif":
			p.take()
			return n, nil
		case "else":
			p.take()
			body, stop, err := p.parseUntil([]string{"endif"})
			if err != nil {
				return nil, err
			}
			if stop == "" {
				return nil, p.errorf("if is not closed")
			}
			p.take()
			n.branches = append(n.branches, ifBranch{nodes: body})
			return n, nil
		case "elif":
			c := p.take()
			cond, err = parseExpr(c[1:])
			if err != nil {
				return nil, p.errorf("%v", err)
			}
		}
	}
}

func (p *parser) parseFor(comps []string) (node, error) {
	// for <x[, y]> in <src> [where <cond>]
	in := -1
	for i, c := range comps {
		if c == "in" {
			in = i
			break
		}
	}
	if in < 2 || in+1 >= len(comps) {
		return nil, p.errorf("'for' statements should use the syntax: `for <x> in <y> [where <condition>]`")
	}
	var vars []string
	for _, v := range strings.Split(strings.Join(comps[1:in], ""), ",") {
		if v = strings.TrimSpace(v); v != "" {
			vars = append(vars, v)
		}
	}
	src, err := parseVariable(comps[in+1])
	if err != nil {
		return nil, p.errorf("%v", err)
	}
	n := &forNode{vars: vars, src: src}
	if in+2 < len(comps) {
		if comps[in+2] != "where" || in+3 >= len(comps) {
			return nil, p.errorf("'for' statements should use the syntax: `for <x> in <y> [where <condition>]`")
		}
		n.where, err = parseExpr(comps[in+3:])
		if err != nil {
			return nil, p.errorf("%v", err)
		}
	}
	body, stop, err := p.parseUntil([]string{"empty", "endfor"})
	if err != nil {
		return nil, err
	}
	n.body = body
	if stop == "empty" {
		p.take()
		n.empty, stop, err = p.parseUntil([]string{"endfor"})
		if err != nil {
			return nil, err
		}
	}
	if stop != "endfor" {
		return nil, p.errorf("for is not closed")
	}
	p.take()
	return n, nil
}
