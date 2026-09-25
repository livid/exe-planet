// Package stencil renders the subset of the Stencil template language
// that Planet's site templates use, with Stencil's own rules for value
// printing, truthiness, comparison and inheritance, so a template built
// here comes out as it does in Planet. It is written from the measured
// grammar of the shipped templates, and those templates are its fixtures.
package stencil

import (
	"fmt"
	"strings"
)

type tokenKind int

const (
	tokText tokenKind = iota
	tokVariable
	tokBlock
	tokComment
)

type token struct {
	kind      tokenKind
	contents  string
	trimLeft  bool // {%- : trim the whitespace before the tag
	trimRight bool // -%} : trim the whitespace after it
	line      int
}

// Error is a template error with its place.
type Error struct {
	Template string
	Line     int
	Msg      string
}

func (e *Error) Error() string {
	if e.Template == "" {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return fmt.Sprintf("%s:%d: %s", e.Template, e.Line, e.Msg)
}

// lex splits a template into text and tag tokens. A tag's contents are
// stripped the way Stencil strips them: split into lines, empty lines
// dropped, each line trimmed of spaces, joined by one space.
func lex(name, src string) ([]token, error) {
	var toks []token
	line := 1
	for len(src) > 0 {
		i := strings.Index(src, "{")
		start := -1
		var open, close string
		var kind tokenKind
		for i >= 0 && i+1 < len(src) {
			switch src[i+1] {
			case '{':
				start, open, close, kind = i, "{{", "}}", tokVariable
			case '%':
				start, open, close, kind = i, "{%", "%}", tokBlock
			case '#':
				start, open, close, kind = i, "{#", "#}", tokComment
			}
			if start >= 0 {
				break
			}
			j := strings.Index(src[i+1:], "{")
			if j < 0 {
				i = -1
			} else {
				i += 1 + j
			}
		}
		if start < 0 {
			toks = append(toks, token{kind: tokText, contents: src, line: line})
			break
		}
		if start > 0 {
			toks = append(toks, token{kind: tokText, contents: src[:start], line: line})
			line += strings.Count(src[:start], "\n")
		}
		end := strings.Index(src[start+2:], close)
		if end < 0 {
			return nil, &Error{Template: name, Line: line, Msg: "unclosed " + open}
		}
		raw := src[start+2 : start+2+end]
		t := token{kind: kind, line: line}
		if kind == tokBlock {
			if strings.HasPrefix(raw, "-") {
				t.trimLeft = true
				raw = raw[1:]
			}
			if strings.HasSuffix(raw, "-") {
				t.trimRight = true
				raw = raw[:len(raw)-1]
			}
		}
		t.contents = stripTag(raw)
		toks = append(toks, t)
		line += strings.Count(raw, "\n")
		src = src[start+2+end+len(close):]
	}
	return toks, nil
}

func stripTag(s string) string {
	var parts []string
	for _, l := range strings.Split(s, "\n") {
		if l == "" {
			continue
		}
		parts = append(parts, strings.Trim(l, " "))
	}
	return strings.Join(parts, " ")
}

// smartSplit splits on sep outside single or double quotes, as Stencil
// does, so a quoted argument may hold the separator.
func smartSplit(s string, sep byte) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
			cur.WriteByte(c)
		case c == sep:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	out = append(out, cur.String())
	return out
}

// components splits a tag on spaces the way Stencil's token.components
// does: quoted strings stay whole, runs of spaces make no empty parts.
func components(s string) []string {
	var out []string
	for _, c := range smartSplit(s, ' ') {
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}
