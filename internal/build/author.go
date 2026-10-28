package build

import (
	"strings"

	"github.com/livid/exe-planet/internal/planet"
)

// Author is who wrote an article, from its front matter's author,
// author_link and author_avatar lines. Avatar is a picture in the
// article's folder or a URL.
type Author struct {
	Name   string
	Link   string
	Avatar string
}

// authorOf is an article's author as a page reads it, article.author =
// {name, link, avatar}, or nil. dir is the article's folder as the page
// addresses it, "" on its own page and "<id>/" on the site's lists, so
// an avatar in the article's folder shows on either.
func (b *builder) authorOf(a *planet.MyArticle, dir string) map[string]any {
	if b.in.Author == nil {
		return nil
	}
	au := b.in.Author(a)
	if au == nil || au.Name == "" {
		return nil
	}
	m := map[string]any{"name": au.Name}
	if au.Link != "" {
		m["link"] = au.Link
	}
	if av := au.Avatar; av != "" {
		if !strings.HasPrefix(av, "/") && !strings.Contains(av, "://") {
			av = dir + av
		}
		m["avatar"] = av
	}
	return m
}

// listAuthors puts each article's author on the site's lists.
func (b *builder) listAuthors(byID map[string]*planet.MyArticle, lists ...[]any) {
	for _, list := range lists {
		for _, it := range list {
			m, _ := it.(map[string]any)
			id, _ := m["id"].(string)
			if a := byID[id]; a != nil {
				if au := b.authorOf(a, id+"/"); au != nil {
					m["author"] = au
				}
			}
		}
	}
}
