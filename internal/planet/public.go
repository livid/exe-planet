package planet

import "time"

// Hero describes the hero image the build found for an article, which is
// derived from the attachments on disk rather than stored. URL is where a
// browser finds it: the site's base, the slug or id, the file name.
type Hero struct {
	Filename string
	URL      string
	Width    *int
	Height   *int
}

// PublicArticle is the article.json a follower reads and the `article`
// a template sees, as a map keyed like Planet's PublicArticleModel. Absent
// optionals are left out, so the JSON writer omits them like Codable.
func PublicArticle(a *MyArticle, hero *Hero, audioDuration, audioByteLength *int) map[string]any {
	m := map[string]any{
		"articleType":  a.Type(),
		"id":           a.ID,
		"link":         a.LinkPath(),
		"slug":         deref(a.Slug, ""),
		"externalLink": deref(a.ExternalLink, ""),
		"title":        a.Title,
		"content":      a.Content,
		"created":      a.Created,
		"hasVideo":     a.HasVideo(),
		"hasAudio":     a.HasAudio(),
	}
	putInt(m, "articleNumber", a.ArticleNumber)
	putString(m, "articleReference", a.ArticleReference)
	putString(m, "contentRendered", a.ContentRendered)
	putDate(m, "modified", a.Modified)
	putString(m, "videoFilename", a.VideoFilename)
	putString(m, "audioFilename", a.AudioFilename)
	putInt(m, "audioDuration", audioDuration)
	putInt(m, "audioByteLength", audioByteLength)
	if a.Attachments != nil {
		m["attachments"] = a.Attachments
	}
	if hero != nil && hero.Filename != "" {
		m["heroImage"] = hero.URL
		m["heroImageURL"] = hero.URL
		m["heroImageFilename"] = hero.Filename
		putInt(m, "heroImageWidth", hero.Width)
		putInt(m, "heroImageHeight", hero.Height)
	} else {
		putInt(m, "heroImageWidth", a.HeroImageWidth)
		putInt(m, "heroImageHeight", a.HeroImageHeight)
	}
	if a.CIDs != nil {
		m["cids"] = a.CIDs
	}
	if a.Tags != nil {
		m["tags"] = a.Tags
	}
	putString(m, "originalSiteName", a.OriginalSiteName)
	putString(m, "originalSiteDomain", a.OriginalSiteDomain)
	putString(m, "originalPostID", a.OriginalPostID)
	putDate(m, "originalPostDate", a.OriginalPostDate)
	putDate(m, "pinned", a.Pinned)
	return m
}

// PublicPlanet is planet.json: the planet's public fields and the given
// articles, already projected.
func PublicPlanet(p *MyPlanet, articles []map[string]any) map[string]any {
	items := make([]any, len(articles))
	for i, a := range articles {
		items[i] = a
	}
	m := map[string]any{
		"id":       p.ID,
		"name":     p.Name,
		"about":    p.About,
		"ipns":     p.IPNS,
		"created":  p.Created,
		"updated":  p.Updated,
		"articles": items,
	}
	putBool(m, "plausibleEnabled", p.PlausibleEnabled)
	putString(m, "plausibleDomain", p.PlausibleDomain)
	putString(m, "plausibleAPIServer", p.PlausibleAPIServer)
	putBool(m, "juiceboxEnabled", p.JuiceboxEnabled)
	putInt(m, "juiceboxProjectID", p.JuiceboxProjectID)
	putInt(m, "juiceboxProjectIDGoerli", p.JuiceboxProjectIDGoerli)
	putBool(m, "acceptsDonation", p.AcceptsDonation)
	putString(m, "acceptsDonationMessage", p.AcceptsDonationMessage)
	putString(m, "acceptsDonationETHAddress", p.AcceptsDonationETHAddress)
	putString(m, "twitterUsername", p.TwitterUsername)
	putString(m, "githubUsername", p.GithubUsername)
	putString(m, "telegramUsername", p.TelegramUsername)
	putString(m, "mastodonUsername", p.MastodonUsername)
	putString(m, "discordLink", p.DiscordLink)
	if p.PodcastCategories != nil {
		m["podcastCategories"] = p.PodcastCategories
	}
	putString(m, "podcastLanguage", p.PodcastLanguage)
	putBool(m, "podcastExplicit", p.PodcastExplicit)
	if p.Tags != nil {
		m["tags"] = p.Tags
	}
	return m
}

// TemplatePlanet is the `planet` an article template sees: Planet fills
// the optionals with defaults there, unlike in planet.json.
func TemplatePlanet(p *MyPlanet, articles []map[string]any) map[string]any {
	m := PublicPlanet(p, articles)
	if m["plausibleEnabled"] == nil {
		m["plausibleEnabled"] = false
	}
	if m["plausibleAPIServer"] == nil {
		m["plausibleAPIServer"] = "plausible.io"
	}
	if m["juiceboxEnabled"] == nil {
		m["juiceboxEnabled"] = false
	}
	if m["acceptsDonation"] == nil {
		m["acceptsDonation"] = false
	}
	if m["podcastCategories"] == nil {
		m["podcastCategories"] = map[string][]string{}
	}
	if m["podcastLanguage"] == nil {
		m["podcastLanguage"] = "en"
	}
	if m["podcastExplicit"] == nil {
		m["podcastExplicit"] = false
	}
	if m["tags"] == nil {
		m["tags"] = map[string]string{}
	}
	return m
}

// ForTemplate converts a projection for the template engine: dates
// become time.Time so the date filters work, typed collections become
// the engine's generic ones.
func ForTemplate(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = forTemplate(v)
	}
	return out
}

func forTemplate(v any) any {
	switch x := v.(type) {
	case Date:
		return x.Time()
	case []string:
		items := make([]any, len(x))
		for i, s := range x {
			items[i] = s
		}
		return items
	case []any:
		items := make([]any, len(x))
		for i, e := range x {
			items[i] = forTemplate(e)
		}
		return items
	case map[string]string:
		out := make(map[string]any, len(x))
		for k, s := range x {
			out[k] = s
		}
		return out
	case map[string][]string:
		out := make(map[string]any, len(x))
		for k, s := range x {
			out[k] = forTemplate(s)
		}
		return out
	case map[string]any:
		return ForTemplate(x)
	}
	return v
}

// TimeOf reads a template-side date back.
func TimeOf(v any) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case Date:
		return x.Time(), true
	}
	return time.Time{}, false
}

func deref(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}

func putString(m map[string]any, k string, v *string) {
	if v != nil {
		m[k] = *v
	}
}

func putInt(m map[string]any, k string, v *int) {
	if v != nil {
		m[k] = *v
	}
}

func putBool(m map[string]any, k string, v *bool) {
	if v != nil {
		m[k] = *v
	}
}

func putDate(m map[string]any, k string, v *Date) {
	if v != nil {
		m[k] = *v
	}
}
