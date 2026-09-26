package build

import (
	"encoding/base64"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// InlineOptions says where a page's relative resources live and what to
// do with the article's own files.
type InlineOptions struct {
	// PageDir is the folder the page's relative URLs resolve against: the
	// article's folder, built or source.
	PageDir string
	// RootDir is the site's root, where "../x" resolves (a site's
	// avatar.png sits there); empty leaves such references alone.
	RootDir string
	// AssetsDir is the template's assets folder, where "../assets/…" and
	// "assets/…" resolve, since a source folder has no assets beside it.
	AssetsDir string
	// AttachmentURL, when set, is where a file in PageDir is fetched from
	// instead of being folded in: the tokenised attachment route.
	AttachmentURL func(name string) string
	// MaxInline caps a folded file's size; larger ones keep their URL.
	MaxInline int64
}

var cssURL = regexp.MustCompile(`url\(\s*(['"]?)([^'")]+)(['"]?)\s*\)`)

// Inline folds a rendered page's relative stylesheets, scripts and
// pictures into it, so a frame shows it with no path of its own to the
// files: a stylesheet becomes a <style> with its url() references as
// data URIs, a script becomes inline, an image, video or audio becomes a
// data URI or the attachment route. Anything absolute is left alone.
func Inline(page string, o InlineOptions) string {
	if o.MaxInline <= 0 {
		o.MaxInline = 8 << 20
	}
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return page
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Link:
				// a relative preload (Planet's templates fetch ../planet.json
				// and ../rss.xml ahead) has no home in a frame without a
				// path: it goes, so the frame does not chase it.
				if attr(n, "rel") == "preload" && isRelative(attr(n, "href")) {
					n.Parent.RemoveChild(n)
					return
				}
				if attr(n, "rel") == "stylesheet" {
					if css, ok := o.read(attr(n, "href")); ok {
						style := &html.Node{Type: html.ElementNode, Data: "style", DataAtom: atom.Style}
						style.AppendChild(&html.Node{Type: html.TextNode, Data: o.inlineCSS(string(css), attr(n, "href"))})
						n.Parent.InsertBefore(style, n)
						n.Parent.RemoveChild(n)
						return
					}
				}
			case atom.Script:
				if src := attr(n, "src"); src != "" {
					if js, ok := o.read(src); ok {
						setAttr(n, "src", "")
						removeAttr(n, "src")
						for c := n.FirstChild; c != nil; c = n.FirstChild {
							n.RemoveChild(c)
						}
						n.AppendChild(&html.Node{Type: html.TextNode, Data: string(js)})
					}
				}
			case atom.Style:
				if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
					n.FirstChild.Data = o.inlineCSS(n.FirstChild.Data, "")
				}
			case atom.Img, atom.Video, atom.Audio, atom.Source, atom.Track:
				if src := attr(n, "src"); src != "" {
					if u := o.resource(src); u != "" {
						setAttr(n, "src", u)
					}
				}
				if p := attr(n, "poster"); p != "" {
					if u := o.resource(p); u != "" {
						setAttr(n, "poster", u)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			walk(c)
			c = next
		}
	}
	walk(doc)
	var b strings.Builder
	html.Render(&b, doc)
	return b.String()
}

// isRelative says whether a URL is one the page resolves against itself.
func isRelative(u string) bool {
	u = strings.TrimSpace(u)
	return u != "" && !strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "#") && !strings.Contains(u, ":") && !strings.HasPrefix(u, "data")
}

// resolve maps a relative URL to a file, or "" when it is not ours.
func (o InlineOptions) resolve(ref string) (file string, attachment bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "//") {
		return "", false
	}
	if u, err := url.Parse(ref); err != nil || u.Scheme != "" || u.Host != "" {
		return "", false
	}
	p := ref
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if p, err := url.PathUnescape(p); err == nil {
		clean := path.Clean("/" + p)
		if o.AssetsDir != "" {
			if rest, ok := strings.CutPrefix(clean, "/assets/"); ok && !strings.HasPrefix(ref, "/") {
				return filepath.Join(o.AssetsDir, filepath.FromSlash(rest)), false
			}
			// "../assets/x" cleaned from the page's folder
			if strings.HasPrefix(p, "../assets/") {
				return filepath.Join(o.AssetsDir, filepath.FromSlash(strings.TrimPrefix(p, "../assets/"))), false
			}
		}
		if o.PageDir != "" && !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "../") {
			return filepath.Join(o.PageDir, filepath.FromSlash(clean)), true
		}
		// "../x" is a file at the site's root, one level up and no further
		if o.RootDir != "" {
			if rest, ok := strings.CutPrefix(p, "../"); ok && !strings.HasPrefix(rest, "../") && !strings.Contains(rest, "/../") {
				return filepath.Join(o.RootDir, filepath.FromSlash(path.Clean("/"+rest))), false
			}
		}
	}
	return "", false
}

func (o InlineOptions) read(ref string) ([]byte, bool) {
	file, _ := o.resolve(ref)
	if file == "" {
		return nil, false
	}
	fi, err := os.Stat(file)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > o.MaxInline {
		return nil, false
	}
	b, err := os.ReadFile(file)
	return b, err == nil
}

// resource returns the replacement for a media URL: the attachment route
// for the article's own files when one is given, else a data URI.
func (o InlineOptions) resource(ref string) string {
	file, attachment := o.resolve(ref)
	if file == "" {
		return ""
	}
	if attachment && o.AttachmentURL != nil {
		if _, err := os.Stat(file); err == nil {
			return o.AttachmentURL(filepath.Base(file))
		}
		return ""
	}
	return dataURI(file, o.MaxInline)
}

func (o InlineOptions) inlineCSS(css, from string) string {
	base := ""
	if from != "" {
		if f, _ := o.resolve(from); f != "" {
			base = filepath.Dir(f)
		}
	}
	return cssURL.ReplaceAllStringFunc(css, func(m string) string {
		parts := cssURL.FindStringSubmatch(m)
		ref := strings.TrimSpace(parts[2])
		var file string
		switch {
		case strings.HasPrefix(ref, "data:"), strings.Contains(ref, "://"), strings.HasPrefix(ref, "//"):
			return m
		case base != "" && !strings.HasPrefix(ref, "/"):
			p := ref
			if i := strings.IndexAny(p, "?#"); i >= 0 {
				p = p[:i]
			}
			file = filepath.Join(base, filepath.FromSlash(path.Clean(p)))
		default:
			file, _ = o.resolve(ref)
		}
		if file == "" {
			return m
		}
		if u := dataURI(file, o.MaxInline); u != "" {
			return "url(" + parts[1] + u + parts[3] + ")"
		}
		return m
	})
}

func dataURI(file string, max int64) string {
	fi, err := os.Stat(file)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > max {
		return ""
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(file)))
	if mt == "" {
		mt = "application/octet-stream"
	}
	if i := strings.Index(mt, ";"); i >= 0 {
		mt = mt[:i]
	}
	return "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(b)
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, key, val string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

func removeAttr(n *html.Node, key string) {
	out := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key != key {
			out = append(out, a)
		}
	}
	n.Attr = out
}
