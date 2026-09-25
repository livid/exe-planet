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
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
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
			goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Linkify, extension.TaskList, extension.Footnote),
			goldmark.WithRendererOptions(html.WithUnsafe(), html.WithHardWraps(), html.WithXHTML()),
		)}
	})
	return current
}

// Render is Default().Render after the YouTube pass.
func Render(markdown string) string {
	return Default().Render(markdown)
}

func (g *gm) Render(markdown string) string {
	var buf bytes.Buffer
	if err := g.md.Convert([]byte(ReplaceYouTubeLinks(markdown)), &buf); err != nil {
		return ""
	}
	return buf.String()
}
