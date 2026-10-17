package build

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/livid/exe-planet/internal/planet"
)

// A post in more than one language (PLAN.md, A post in more than one
// language). The article as it was written is the page Planet builds,
// <id>/index.html; each other language is one more page beside it,
// <id>/index.<language>.html, the same template given that language's
// title and text. A page of the site — the home page, the archive, a
// tag — stays one page and is handed every title in each language. None
// of it reaches article.json, planet.json or rss.xml, which keep the
// article as it was written; each other language has a feed of its own
// beside it, rss.<language>.xml. An article in one language, on a site
// whose articles are all in one, builds as it always did.

// Version is an article in a language other than the one it was written
// in.
type Version struct {
	Lang    string // a BCP 47 tag in its usual letter case: zh-Hans
	Title   string
	Content string // Markdown
	Summary string
}

// articleLangs is what the builder knows of one article's languages.
type articleLangs struct {
	orig  string    // the language it was written in
	named bool      // its own front matter says so; otherwise it is the site's
	vers  []Version // in other languages, in the order of their tags
}

var langShape = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// langKey is a language as a file name and a script know it: lower case.
func langKey(tag string) string { return strings.ToLower(tag) }

// langFile is the page of an article in a language other than its own.
func langFile(tag string) string { return "index." + langKey(tag) + ".html" }

// siteLang is the language a site's posts are written in unless they
// say otherwise: the template's Language setting when it has one (the
// site's value, else the template's default), else English.
func (b *builder) siteLang() string {
	if set, ok := b.tpl.Settings["language"]; ok {
		for _, v := range []string{b.settings["language"], string(set.DefaultValue)} {
			if langShape.MatchString(v) {
				return v
			}
		}
	}
	return "en"
}

// langsOf is an article's languages, worked out once: a version in the
// language the article was written in is not another language.
func (b *builder) langsOf(a *planet.MyArticle) *articleLangs {
	if al, ok := b.langs[a.ID]; ok {
		return al
	}
	al := &articleLangs{orig: b.siteLang()}
	if b.in.Languages != nil {
		own, vers := b.in.Languages(a)
		if own != "" {
			al.orig, al.named = own, true
		}
		seen := map[string]bool{langKey(al.orig): true}
		for _, v := range vers {
			if k := langKey(v.Lang); !seen[k] && langShape.MatchString(v.Lang) {
				seen[k] = true
				al.vers = append(al.vers, v)
			}
		}
		sort.SliceStable(al.vers, func(i, j int) bool { return langKey(al.vers[i].Lang) < langKey(al.vers[j].Lang) })
	}
	if b.langs == nil {
		b.langs = map[string]*articleLangs{}
	}
	b.langs[a.ID] = al
	return al
}

// langItems is an article's languages as a template reads them, the one
// it was written in first. Each is {lang, key, name, title, file, href,
// url, original, current}: file is the page's name after the article's
// folder ("" for the page it was written as), href the way there from
// one of the article's own pages. The written page is asked for by name
// (?lang=), since on its bare address it picks the reader's language.
func (b *builder) langItems(a *planet.MyArticle, al *articleLangs, current string) []any {
	folder := b.base + b.articlePath(a) + "/"
	item := func(tag, title, file string, original bool) map[string]any {
		key := langKey(tag)
		href := file
		if original {
			href = "./?lang=" + key
		}
		return map[string]any{
			"lang": tag, "key": key, "name": langName(tag), "title": title,
			"file": file, "href": href, "url": folder + file,
			"original": original, "current": current != "" && key == langKey(current),
		}
	}
	items := []any{item(al.orig, a.Title, "", true)}
	for _, v := range al.vers {
		items = append(items, item(v.Lang, v.Title, langFile(v.Lang), false))
	}
	return items
}

// langContext adds what a post's page needs to know of its languages:
// page_lang, the language of this page, for a post that is in more than
// one or names its own; languages, for a post in more than one. A
// template whose Language setting lists the values it takes (options)
// is given the page's own language as that setting when it is one of
// them, so its words, its type and its dates are that page's; otherwise
// the setting stays the site's.
func (b *builder) langContext(ctx map[string]any, a *planet.MyArticle, v *Version) {
	al := b.langsOf(a)
	if !al.named && len(al.vers) == 0 {
		return
	}
	lang := al.orig
	if v != nil {
		lang = v.Lang
	}
	ctx["page_lang"] = lang
	if len(al.vers) > 0 {
		ctx["languages"] = b.langItems(a, al, lang)
		if feeds := b.siteFeeds(); feeds != nil {
			ctx["site_feeds"] = feeds
		}
	}
	if set, ok := b.tpl.Settings["language"]; ok && len(set.Options) > 0 {
		if opt := pickLang(lang, set.Options); opt != "" && opt != b.settings["language"] {
			user := clone(b.user)
			user["language"] = opt
			ctx["user_settings"] = user
		}
	}
}

// listLanguages gives each listed article that is in more than one
// language its languages, on the maps a page of the site lists.
func (b *builder) listLanguages(byID map[string]*planet.MyArticle, lists ...[]any) {
	for _, list := range lists {
		for _, it := range list {
			m, _ := it.(map[string]any)
			id, _ := m["id"].(string)
			a := byID[id]
			if a == nil {
				continue
			}
			if al := b.langsOf(a); len(al.vers) > 0 {
				m["languages"] = b.langItems(a, al, "")
			}
		}
	}
}

// siteLanguages is every language a post of the site can be read in, as
// keys, when any post is in more than one; nil otherwise.
func (b *builder) siteLanguages() []any {
	seen := map[string]bool{}
	for _, a := range b.in.Articles {
		if al := b.langsOf(a); len(al.vers) > 0 {
			seen[langKey(al.orig)] = true
			for _, v := range al.vers {
				seen[langKey(v.Lang)] = true
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]any, len(keys))
	for i, k := range keys {
		out[i] = k
	}
	return out
}

// feedLangs are the languages the site's feed is also written in: every
// language a post of the site can be read in but the site's own, whose
// feed is rss.xml. Each is a tag in its usual letter case, in the order
// of their keys.
func (b *builder) feedLangs() []string {
	site := langKey(b.siteLang())
	tags := map[string]string{}
	for _, a := range b.in.Articles {
		al := b.langsOf(a)
		if len(al.vers) == 0 {
			continue
		}
		for _, t := range append([]string{al.orig}, langTags(al.vers)...) {
			if k := langKey(t); k != site && tags[k] == "" {
				tags[k] = t
			}
		}
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = tags[k]
	}
	return out
}

func langTags(vers []Version) []string {
	out := make([]string, len(vers))
	for i, v := range vers {
		out[i] = v.Lang
	}
	return out
}

// feedFile is the feed in a language other than the site's own.
func feedFile(tag string) string { return "rss." + langKey(tag) + ".xml" }

// siteFeeds is feedLangs as a template reads them, each {lang, key,
// name, file}, for the head of a page to name beside rss.xml and the
// page script to send a reader's RSS link to; nil when there are none.
func (b *builder) siteFeeds() []any {
	var out []any
	for _, t := range b.feedLangs() {
		out = append(out, map[string]any{"lang": t, "key": langKey(t), "name": langName(t), "file": feedFile(t)})
	}
	return out
}

// feedItem is an article's entry in the feed in lang: as it was written
// unless it can be read in lang (by langCands, as a reader would be sent
// there), and then with that language's title and text, and langFile,
// the page that link and guid name after the article's folder.
func (b *builder) feedItem(pub map[string]any, lang string) map[string]any {
	id, _ := pub["id"].(string)
	var a *planet.MyArticle
	for _, x := range b.in.Articles {
		if x.ID == id {
			a = x
			break
		}
	}
	if a == nil {
		return pub
	}
	al := b.langsOf(a)
	pick := pickLang(lang, append([]string{al.orig}, langTags(al.vers)...))
	if pick == "" || langKey(pick) == langKey(al.orig) {
		return pub
	}
	for _, v := range al.vers {
		if langKey(v.Lang) == langKey(pick) {
			item := clone(pub)
			item["title"], item["content"], item["langFile"] = v.Title, v.Content, langFile(v.Lang)
			return item
		}
	}
	return pub
}

// versionPages writes an article's pages in its other languages into its
// folder, beside the index.html already there.
func (b *builder) versionPages(a *planet.MyArticle, dir string) error {
	for _, v := range b.langsOf(a).vers {
		html, err := b.renderArticleIn("blog.html", a, dir, &v)
		if err != nil {
			return err
		}
		if err := b.write(filepath.Join(a.ID, langFile(v.Lang)), []byte(html)); err != nil {
			return err
		}
	}
	return nil
}

// langCands lists the languages of have that someone asking for want
// reads, best first: the tag itself; then, in Chinese, the ones in the
// same script (Traditional is asked for by Hant, TW, HK or MO, and
// anything else is Simplified) before the others; in any other language
// the bare one before its regional forms. The page script in the
// templates (modules/lang.html) has the same rule, so the page a reader
// is sent to is the one a list showed them.
func langCands(want string, have []string) []string {
	w := langKey(want)
	base, _, _ := strings.Cut(w, "-")
	if base == "" {
		return nil
	}
	hant := func(t string) bool {
		for _, part := range strings.Split(t, "-")[1:] {
			switch part {
			case "hant", "tw", "hk", "mo":
				return true
			}
		}
		return false
	}
	var ranked [3][]string
	for _, h := range have {
		k := langKey(h)
		if hb, _, _ := strings.Cut(k, "-"); hb != base {
			continue
		}
		rank := 2
		switch {
		case k == w:
			rank = 0
		case base == "zh" && hant(k) == hant(w), base != "zh" && k == base:
			rank = 1
		}
		ranked[rank] = append(ranked[rank], h)
	}
	return append(append(ranked[0], ranked[1]...), ranked[2]...)
}

// pickLang is the first of them, or "".
func pickLang(want string, have []string) string {
	if c := langCands(want, have); len(c) > 0 {
		return c[0]
	}
	return ""
}

// langNames are languages by their own names, for the line on a post
// that lists the languages it can be read in.
var langNames = map[string]string{
	"ar": "العربية", "cs": "Čeština", "da": "Dansk", "de": "Deutsch", "el": "Ελληνικά",
	"en": "English", "es": "Español", "fi": "Suomi", "fr": "Français", "he": "עברית",
	"hi": "हिन्दी", "hu": "Magyar", "id": "Bahasa Indonesia", "it": "Italiano", "ja": "日本語",
	"ko": "한국어", "ms": "Bahasa Melayu", "nb": "Norsk bokmål", "nl": "Nederlands", "pl": "Polski",
	"pt": "Português", "ro": "Română", "ru": "Русский", "sv": "Svenska", "th": "ไทย",
	"tr": "Türkçe", "uk": "Українська", "vi": "Tiếng Việt",
	"zh": "中文", "zh-hans": "简体中文", "zh-hant": "繁體中文",
	"zh-cn": "简体中文", "zh-sg": "简体中文", "zh-tw": "繁體中文", "zh-hk": "繁體中文", "zh-mo": "繁體中文",
}

// langName is a language's own name: the tag's, else its language's,
// else the tag.
func langName(tag string) string {
	k := langKey(tag)
	if n, ok := langNames[k]; ok {
		return n
	}
	if base, _, cut := strings.Cut(k, "-"); cut {
		if n, ok := langNames[base]; ok {
			return n
		}
	}
	return tag
}
