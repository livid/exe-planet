package build

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Planet's absoluteImageURL filter runs the article HTML through
// SwiftSoup, a port of jsoup: the fragment is parsed as HTML5 and
// serialised again, which rewrites the image sources and, as a side
// effect, normalises the markup. A feed reader therefore gets the parsed
// tree, not the author's bytes: bare ampersands become &amp;, stray
// structural tags vanish, void elements lose their slash. This file does
// the same with Go's HTML5 parser and a serialiser that writes entities
// and attributes as jsoup does. jsoup also re-indents; only whitespace
// differs from Planet's output, and a feed reader collapses it anyway.

// booleanAttributes are written without a value, as jsoup writes them.
var booleanAttributes = map[string]bool{
	"allowfullscreen": true, "async": true, "autofocus": true, "checked": true, "compact": true, "declare": true,
	"default": true, "defer": true, "disabled": true, "formnovalidate": true, "hidden": true, "inert": true,
	"ismap": true, "itemscope": true, "multiple": true, "muted": true, "nohref": true, "noresize": true,
	"noshade": true, "novalidate": true, "nowrap": true, "open": true, "readonly": true, "required": true,
	"reversed": true, "seamless": true, "selected": true, "sortable": true, "truespeed": true, "typemustmatch": true,
}

var voidElements = map[atom.Atom]bool{
	atom.Area: true, atom.Base: true, atom.Br: true, atom.Col: true, atom.Embed: true, atom.Hr: true, atom.Img: true,
	atom.Input: true, atom.Keygen: true, atom.Link: true, atom.Meta: true, atom.Param: true, atom.Source: true,
	atom.Track: true, atom.Wbr: true,
}

// soupImageURLs parses a body fragment, prefixes every relative img src
// and serialises the body's children.
func soupImageURLs(fragment, prefix string) string {
	body := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), body)
	if err != nil {
		return fragment
	}
	var b strings.Builder
	for _, n := range nodes {
		rewriteImages(n, prefix)
		serialize(&b, n)
	}
	return b.String()
}

func rewriteImages(n *html.Node, prefix string) {
	if n.Type == html.ElementNode && n.DataAtom == atom.Img {
		for i, a := range n.Attr {
			if a.Key == "src" && !strings.HasPrefix(a.Val, "https://") && !strings.HasPrefix(a.Val, "http://") {
				n.Attr[i].Val = prefix + a.Val
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		rewriteImages(c, prefix)
	}
}

func serialize(b *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		if p := n.Parent; p != nil && p.Type == html.ElementNode && (p.DataAtom == atom.Script || p.DataAtom == atom.Style) {
			b.WriteString(n.Data)
		} else {
			b.WriteString(escapeText(n.Data))
		}
	case html.CommentNode:
		b.WriteString("<!--" + n.Data + "-->")
	case html.ElementNode:
		b.WriteByte('<')
		b.WriteString(n.Data)
		for _, a := range n.Attr {
			b.WriteByte(' ')
			b.WriteString(a.Key)
			if (a.Val == "" || a.Val == a.Key) && booleanAttributes[a.Key] {
				continue
			}
			b.WriteString(`="`)
			b.WriteString(escapeAttr(a.Val))
			b.WriteByte('"')
		}
		b.WriteByte('>')
		if voidElements[n.DataAtom] {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			serialize(b, c)
		}
		b.WriteString("</" + n.Data + ">")
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			serialize(b, c)
		}
	}
}

func escapeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case ' ':
			b.WriteString("&nbsp;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func escapeAttr(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case ' ':
			b.WriteString("&nbsp;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
