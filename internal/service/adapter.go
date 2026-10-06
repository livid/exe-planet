// Package service runs builds for the daemon: it turns a site's Workspace
// files into Planet's model, builds into the node's state folder, and
// keeps one build per site in flight.
package service

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"

	"github.com/livid/exe-planet/internal/markdown"
	"github.com/livid/exe-planet/internal/planet"
	"github.com/livid/exe-planet/internal/site"
)

// TagKey is String.normalizedTag() in Planet: lower-cased, split at
// punctuation, symbols and spaces except the hyphen, joined by hyphens,
// runs of hyphens folded, edges trimmed. Planet also folds diacritics
// and full-width forms, which is left for the corpus to demand.
func TagKey(display string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(display) {
		if r != '-' && (unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r) || unicode.IsControl(r)) {
			b.WriteByte('-')
		} else {
			b.WriteRune(r)
		}
	}
	key := b.String()
	for strings.Contains(key, "--") {
		key = strings.ReplaceAll(key, "--", "-")
	}
	return strings.Trim(key, "-")
}

func ptr[T any](v T) *T { return &v }

// FromSite projects a site and its posts and pages onto Planet's own
// model, the input the builder takes. Drafts are left out; they are
// previewed, not published.
func FromSite(st *site.Site, posts []*site.Post) (*planet.MyPlanet, []*planet.MyArticle) {
	p := &planet.MyPlanet{
		ID:                st.ID,
		Name:              st.Name,
		About:             st.About,
		IPNS:              st.IPNS,
		Created:           planet.DateOf(st.Created),
		TemplateName:      st.Template,
		Tags:              map[string]string{},
		PlausibleEnabled:  ptr(st.Plausible.Enabled),
		JuiceboxEnabled:   ptr(false),
		AcceptsDonation:   ptr(false),
		PodcastCategories: map[string][]string{},
		PodcastLanguage:   ptr("en"),
		PodcastExplicit:   ptr(false),
	}
	if st.Author != "" {
		p.AuthorName = ptr(st.Author)
	}
	if st.NoIndex {
		p.DoNotIndex = ptr(true)
	}
	if st.Plausible.Domain != "" {
		p.PlausibleDomain = ptr(st.Plausible.Domain)
	}
	if st.Plausible.Server != "" {
		p.PlausibleAPIServer = ptr(st.Plausible.Server)
	}
	set := func(dst **string, v string) {
		if v != "" {
			*dst = ptr(v)
		}
	}
	set(&p.TwitterUsername, st.Social.Twitter)
	set(&p.GithubUsername, st.Social.GitHub)
	set(&p.TelegramUsername, st.Social.Telegram)
	set(&p.MastodonUsername, st.Social.Mastodon)
	set(&p.DiscordLink, st.Social.Discord)
	for k, v := range st.Tags {
		p.Tags[k] = v
	}

	var articles []*planet.MyArticle
	updated := st.Created
	for _, post := range posts {
		// (a post with no id is one made by hand whose file could not be
		// written to: its folder in the build would be the build's own)
		if post.Kind == site.KindDraft || post.Error != "" || post.ID == "" {
			continue
		}
		a := &planet.MyArticle{
			ID:          post.ID,
			ArticleType: ptr(planet.TypeBlog),
			Link:        ptr("/" + post.ID + "/"),
			Slug:        ptr(post.Slug),
			Title:       post.Title,
			Content:     post.Body,
			Created:     planet.DateOf(post.Created),
			Attachments: append([]string{}, post.Attachments...),
			Tags:        map[string]string{},
		}
		if post.Kind == site.KindPage {
			a.ArticleType = ptr(planet.TypePage)
			a.IsIncludedInNavigation = ptr(true)
			weight := 1
			if w, err := strconv.Atoi(post.Extra["weight"]); err == nil {
				weight = w
			}
			a.NavigationWeight = ptr(weight)
		}
		if !post.Modified.IsZero() {
			a.Modified = ptr(planet.DateOf(post.Modified))
		}
		if post.Hero != "" {
			a.HeroImage = ptr(post.Hero)
		}
		// Planet marks the first video and the first audio attachment.
		for _, name := range post.Attachments {
			switch strings.ToLower(filepath.Ext(name)) {
			case ".mp4", ".mov", ".m4v", ".webm":
				if a.VideoFilename == nil {
					a.VideoFilename = ptr(name)
				}
			case ".mp3", ".m4a", ".wav", ".aac", ".ogg":
				if a.AudioFilename == nil {
					a.AudioFilename = ptr(name)
				}
			}
		}
		if post.Link != "" {
			a.ExternalLink = ptr(post.Link)
		}
		for _, t := range post.Tags {
			key := TagKey(t)
			a.Tags[key] = t
			if _, ok := p.Tags[key]; !ok {
				p.Tags[key] = t
			}
		}
		if post.Body != "" {
			rendered := markdown.Render(post.Body)
			a.ContentRendered = ptr(rendered)
			a.Summary = ptr(Summary(rendered))
		}
		for _, t := range []time.Time{post.Created, post.Modified} {
			if t.After(updated) {
				updated = t
			}
		}
		articles = append(articles, a)
	}
	p.Updated = planet.DateOf(updated)
	planet.SortArticles(articles)
	return p, articles
}

// Summary is Planet's: the rendered HTML's text, whitespace collapsed,
// cut to 280 characters with an ellipsis.
func Summary(rendered string) string {
	text := htmlText(rendered)
	runes := []rune(text)
	if len(runes) > 280 {
		return string(runes[:280]) + "..."
	}
	return text
}

func htmlText(fragment string) string {
	z := html.NewTokenizer(strings.NewReader(fragment))
	var b strings.Builder
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(b.String()), " ")
		case html.TextToken:
			b.Write(z.Text())
			b.WriteByte(' ')
		}
	}
}

// newestChange is the latest modification time under a folder, the
// signal that a site's `updated` should move.
func newestChange(dir string) time.Time {
	var newest time.Time
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && path != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}
