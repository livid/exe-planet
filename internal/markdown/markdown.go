// Package markdown renders Markdown the way Planet does: cmark-gfm with
// the table, autolink, strikethrough and task-list extensions, footnotes,
// raw HTML passed through, and a newline inside a paragraph as a line
// break. Today the renderer is goldmark; the interface is one function so
// a hand-written renderer can replace it once it passes the corpus of
// Planet's own article.md and contentRendered pairs.
package markdown

import (
	"bytes"
	"regexp"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// Renderer turns Markdown into HTML.
type Renderer interface {
	Render(markdown string) string
}

// youtube matches the links Planet turns into embeds before rendering.
var youtube = regexp.MustCompile(`https?://(?:www\.)?youtu(?:be\.com/watch\?v=|\.be/)([\w\-]*)(?:&(?:amp;)?[\w?=]*)?`)

const embed = `<iframe width="100%" style="aspect-ratio: 16/9" src="https://www.youtube.com/embed/$1" title="YouTube Video" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>`

// ReplaceYouTubeLinks is Planet's first pass: a bare YouTube link becomes
// an embedded player.
func ReplaceYouTubeLinks(text string) string {
	return youtube.ReplaceAllString(text, embed)
}

// cmark-gfm's autolink extension links an email only when its domain has
// a dot and ends in a letter, so admin@192.168.1.22 stays text.
var emailPattern = regexp.MustCompile(`[a-zA-Z0-9._+-]+@(?:[a-zA-Z0-9-]+\.)+[a-zA-Z0-9-]*[a-zA-Z]`)

var linkify = extension.NewLinkify(extension.WithLinkifyEmailRegexp(emailPattern))

type gm struct {
	md goldmark.Markdown
}

var (
	once    sync.Once
	current Renderer
)

// Default is the shared renderer.
func Default() Renderer {
	once.Do(func() {
		current = &gm{md: goldmark.New(
			goldmark.WithExtensions(extension.Table, extension.Strikethrough, linkify, extension.TaskList, extension.Footnote),
			goldmark.WithRendererOptions(html.WithUnsafe(), html.WithHardWraps(), html.WithXHTML(),
				renderer.WithNodeRenderers(util.Prioritized(&taskCheckBox{}, 100))),
		)}
	})
	return current
}

// Render is Default().Render after the YouTube pass.
func Render(markdown string) string {
	return Default().Render(markdown)
}

func (g *gm) Render(markdown string) string {
	// cmark reads CR LF as a line ending and writes LF; goldmark would
	// carry the CR into an HTML block.
	src := strings.ReplaceAll(ReplaceYouTubeLinks(markdown), "\r\n", "\n")
	// cmark ends an HTML block with a newline even when the document
	// has none; goldmark does not, so the document gets one.
	if src != "" && !strings.HasSuffix(src, "\n") {
		src += "\n"
	}
	var buf bytes.Buffer
	if err := g.md.Convert([]byte(src), &buf); err != nil {
		return ""
	}
	return buf.String()
}

// taskCheckBox writes a task list item's box as cmark-gfm does:
// type first, then checked, then disabled.
type taskCheckBox struct{}

func (r *taskCheckBox) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(east.KindTaskCheckBox, r.render)
}

func (r *taskCheckBox) render(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	if node.(*east.TaskCheckBox).IsChecked {
		w.WriteString(`<input type="checkbox" checked="" disabled="" /> `)
	} else {
		w.WriteString(`<input type="checkbox" disabled="" /> `)
	}
	return ast.WalkContinue, nil
}
