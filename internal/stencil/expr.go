package stencil

import (
	"fmt"
	"strings"
)

// Expressions are the `if` grammar: a Pratt parse over space-separated
// components with Stencil's binding powers, where the operands are
// variables with filters.

type expr interface {
	eval(c *Context) (bool, error)
}

type varExpr struct{ v *variable }

func (e *varExpr) eval(c *Context) (bool, error) {
	val, err := e.v.resolve(c)
	if err != nil {
		return false, err
	}
	return truthy(val), nil
}

type notExpr struct{ x expr }

func (e *notExpr) eval(c *Context) (bool, error) {
	b, err := e.x.eval(c)
	return !b, err
}

type andExpr struct{ l, r expr }

func (e *andExpr) eval(c *Context) (bool, error) {
	l, err := e.l.eval(c)
	if err != nil || !l {
		return false, err
	}
	return e.r.eval(c)
}

type orExpr struct{ l, r expr }

func (e *orExpr) eval(c *Context) (bool, error) {
	l, err := e.l.eval(c)
	if err != nil || l {
		return l, err
	}
	return e.r.eval(c)
}

// cmpExpr is ==, !=, <, <=, >, >= and in. Stencil compares only when
// both sides are plain variables; an operator on either side is false.
type cmpExpr struct {
	op   string
	l, r expr
}

func (e *cmpExpr) eval(c *Context) (bool, error) {
	lv, lok := e.l.(*varExpr)
	rv, rok := e.r.(*varExpr)
	if !lok || !rok {
		return false, nil
	}
	l, err := lv.v.resolve(c)
	if err != nil {
		return false, err
	}
	r, err := rv.v.resolve(c)
	if err != nil {
		return false, err
	}
	switch e.op {
	case "==":
		return equal(l, r), nil
	case "!=":
		return !equal(l, r), nil
	case "in":
		return contains(r, l), nil
	}
	ln, lok := toNumber(l)
	rn, rok := toNumber(r)
	if !lok || !rok {
		return false, nil
	}
	switch e.op {
	case "<":
		return ln < rn, nil
	case "<=":
		return ln <= rn, nil
	case ">":
		return ln > rn, nil
	case ">=":
		return ln >= rn, nil
	}
	return false, nil
}

func equal(l, r any) bool {
	if l == nil && r == nil {
		return true
	}
	if l == nil || r == nil {
		return false
	}
	if ln, ok := toNumber(l); ok {
		if rn, ok := toNumber(r); ok {
			return ln == rn
		}
		return false
	}
	switch x := l.(type) {
	case string:
		y, ok := r.(string)
		return ok && x == y
	case bool:
		y, ok := r.(bool)
		return ok && x == y
	}
	return false
}

func contains(haystack, needle any) bool {
	switch h := haystack.(type) {
	case []any:
		for _, e := range h {
			if equal(e, needle) {
				return true
			}
		}
	case string:
		if n, ok := needle.(string); ok {
			return strings.Contains(h, n)
		}
	case map[string]any:
		if n, ok := needle.(string); ok {
			_, has := h[n]
			return has
		}
	}
	return false
}

var infixPower = map[string]int{"in": 5, "or": 6, "and": 7, "==": 10, "!=": 10, ">": 10, ">=": 10, "<": 10, "<=": 10}

const notPower = 8

type exprParser struct {
	toks []string
	pos  int
}

func parseExpr(comps []string) (expr, error) {
	if len(comps) == 0 {
		return nil, fmt.Errorf("empty condition")
	}
	p := &exprParser{toks: comps}
	e, err := p.parse(0)
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.toks) {
		return nil, fmt.Errorf("unexpected %q in condition", p.toks[p.pos])
	}
	return e, nil
}

func (p *exprParser) peek() (string, bool) {
	if p.pos < len(p.toks) {
		return p.toks[p.pos], true
	}
	return "", false
}

func (p *exprParser) parse(power int) (expr, error) {
	tok, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("condition ends early")
	}
	p.pos++
	var left expr
	switch {
	case tok == "not":
		x, err := p.parse(notPower)
		if err != nil {
			return nil, err
		}
		left = &notExpr{x}
	case infixPower[tok] > 0:
		return nil, fmt.Errorf("condition starts with %q", tok)
	default:
		v, err := parseVariable(tok)
		if err != nil {
			return nil, err
		}
		left = &varExpr{v}
	}
	for {
		op, ok := p.peek()
		if !ok {
			return left, nil
		}
		bp := infixPower[op]
		if bp == 0 || bp <= power {
			return left, nil
		}
		p.pos++
		right, err := p.parse(bp)
		if err != nil {
			return nil, err
		}
		switch op {
		case "and":
			left = &andExpr{left, right}
		case "or":
			left = &orExpr{left, right}
		default:
			left = &cmpExpr{op, left, right}
		}
	}
}
