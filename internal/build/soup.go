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
// structural tags vanish, and every void element ends in " />". This
// file does the same with Go's HTML5
// parser and a serialiser that writes entities and attributes as jsoup
// does. jsoup also re-indents; only whitespace differs from Planet's
// output, and a feed reader collapses it anyway.

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
	w := &writer{}
	for _, n := range nodes {
		rewriteImages(n, prefix)
		w.serialize(n)
	}
	return w.b.String()
}

type writer struct {
	b strings.Builder
}

func rewriteImages(n *html.Node, prefix string) {
	if n.Type == html.ElementNode && n.DataAtom == atom.Img {
		for i, a := range n.Attr {
			if a.Key == "src" && articleRelative(a.Val) {
				n.Attr[i].Val = prefix + a.Val
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		rewriteImages(c, prefix)
	}
}

// articleRelative reports whether an image source names a file beside the
// article, the only kind the filter moves: not empty, not from the root or
// another host (/ or //), not a query or fragment alone, and with no
// scheme (https:, data:, ipfs:). A scheme is what a browser reads as one,
// a letter then letters, digits, + - or . up to the first colon, so
// "Shot 10:30.png" is still a file. Planet's absoluteImageURL has the same
// rule.
func articleRelative(src string) bool {
	if src == "" || strings.IndexByte("/?#", src[0]) >= 0 {
		return false
	}
	colon := strings.IndexByte(src, ':')
	if colon <= 0 {
		return true
	}
	for i := 0; i < colon; i++ {
		c := src[i]
		letter := 'a' <= c|0x20 && c|0x20 <= 'z'
		if !letter && (i == 0 || !('0' <= c && c <= '9' || c == '+' || c == '-' || c == '.')) {
			return true
		}
	}
	return false
}

func (w *writer) serialize(n *html.Node) {
	b := &w.b
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
		if voidElements[n.DataAtom] {
			// SwiftSoup closes every void element with a slash, whichever
			// way it was written.
			b.WriteString(" />")
			return
		}
		b.WriteByte('>')
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			w.serialize(c)
		}
		b.WriteString("</" + n.Data + ">")
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			w.serialize(c)
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
