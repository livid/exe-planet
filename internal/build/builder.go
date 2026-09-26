package build

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/livid/exe-planet/internal/markdown"
	"github.com/livid/exe-planet/internal/planet"
	"github.com/livid/exe-planet/internal/stencil"
)

// reservedTags are Planet's RESERVED_KEYWORDS_FOR_TAGS: names that would
// collide with the site's own pages.
var reservedTags = map[string]bool{"index": true, "tags": true, "archive": true, "archives": true}

// Input is everything a build needs. Articles come in Planet's order.
type Input struct {
	Planet   *planet.MyPlanet
	Articles []*planet.MyArticle
	Template *Template
	// Settings are the site's template settings; defaults fill the rest.
	Settings map[string]string
	// Filters are cached <key>Filter values; missing ones are solved.
	Filters map[string]string
	// Attachments names the folder holding an article's files, or "".
	Attachments func(a *planet.MyArticle) string
	AvatarPath  string
	FaviconPath string
	// HubThread is an article's thread on the exe hub for the template,
	// {link, frame, hub, host, id}, or nil (PLAN.md, Replies from the
	// hub). It reaches the page as hub_thread and nothing else: Planet
	// never sets it, and article.json keeps Planet's shape.
	HubThread func(a *planet.MyArticle) map[string]any
	// AssetsDir replaces the template's assets/ folder, for a golden test
	// whose site was built with an older copy of the template.
	AssetsDir string
	Now       time.Time
	Location  *time.Location
	Out       string
}

// Result reports what was written.
type Result struct {
	Files    []string
	Warnings []string
}

type builder struct {
	in        Input
	tpl       *Template
	env       *stencil.Env
	rssEnv    *stencil.Env
	loc       *time.Location
	now       time.Time
	base      string // https://…/ the site's browser base
	dwg       string // domain with gateway, "" when none
	settings  map[string]string
	user      map[string]any // user_settings for templates
	tsettings map[string]any
	hasAvatar bool
	pubs      []map[string]any // every article's JSON projection, Planet's order
	res       *Result
}

// Build renders the site into in.Out.
func Build(in Input) (*Result, error) {
	if in.Planet == nil || in.Template == nil || in.Out == "" {
		return nil, fmt.Errorf("build: planet, template and out are required")
	}
	b := &builder{in: in, tpl: in.Template, loc: in.Location, now: in.Now, res: &Result{}}
	if b.loc == nil {
		b.loc = time.Local
	}
	if b.now.IsZero() {
		b.now = time.Now()
	}
	b.env = stencil.NewEnv(stencil.DirLoader(b.tpl.TemplatesDir()))
	registerPlanetFilters(b.env, b.loc)
	b.rssEnv = stencil.NewEnv(nil)
	registerPlanetFilters(b.rssEnv, b.loc)

	b.dwg = domainWithGateway(in.Planet.Domain)
	if b.dwg != "" {
		b.base = "https://" + b.dwg + "/"
	} else {
		b.base = "https://" + in.Planet.IPNS + ".eth.sucks/"
	}
	if err := os.MkdirAll(in.Out, 0o755); err != nil {
		return nil, err
	}
	if err := b.static(); err != nil {
		return nil, err
	}
	b.prepareSettings()
	if err := b.articles(); err != nil {
		return nil, err
	}
	if err := b.pages(); err != nil {
		return nil, err
	}
	sort.Strings(b.res.Files)
	return b.res, nil
}

// domainWithGateway is Planet's: an .eth name through eth.limo, .sol
// through .build, .bit through .site, a plain domain as itself.
func domainWithGateway(domain *string) string {
	if domain == nil {
		return ""
	}
	d := strings.TrimSpace(*domain)
	switch {
	case strings.HasSuffix(d, ".eth"):
		return d + ".limo"
	case strings.HasSuffix(d, ".sol"):
		return d + ".build"
	case strings.HasSuffix(d, ".bit"):
		return d + ".site"
	}
	return d
}

func (b *builder) write(rel string, data []byte) error {
	p := filepath.Join(b.in.Out, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return err
	}
	b.res.Files = append(b.res.Files, rel)
	return nil
}

func (b *builder) warn(format string, args ...any) {
	b.res.Warnings = append(b.res.Warnings, fmt.Sprintf(format, args...))
}

// static copies the template's assets, the avatar and the favicon.
func (b *builder) static() error {
	assets := filepath.Join(b.tpl.Dir, "assets")
	if b.in.AssetsDir != "" {
		assets = b.in.AssetsDir
		b.tpl.StyleCSSHash = styleHash(filepath.Join(assets, "style.css"))
	}
	if fi, err := os.Stat(assets); err == nil && fi.IsDir() {
		dst := filepath.Join(b.in.Out, "assets")
		os.RemoveAll(dst)
		if err := copyTree(assets, dst); err != nil {
			return err
		}
		b.res.Files = append(b.res.Files, "assets/")
	}
	if b.in.AvatarPath != "" {
		if err := copyFile(b.in.AvatarPath, filepath.Join(b.in.Out, "avatar.png")); err == nil {
			b.hasAvatar = true
			b.res.Files = append(b.res.Files, "avatar.png")
		}
	}
	if b.in.FaviconPath != "" {
		if err := copyFile(b.in.FaviconPath, filepath.Join(b.in.Out, "favicon.ico")); err == nil {
			b.res.Files = append(b.res.Files, "favicon.ico")
		}
	}
	return nil
}

// prepareSettings is writeTemplateSettings plus templateSettingsAndFilters:
// the site's settings normalised to the template's defaults, then the
// colour filters added for the templates.
func (b *builder) prepareSettings() {
	defaults := b.tpl.DefaultSettings()
	b.settings = map[string]string{}
	for k, v := range b.in.Settings {
		b.settings[k] = v
	}
	if len(defaults) > 0 {
		for k, v := range defaults {
			if _, ok := b.settings[k]; !ok {
				b.settings[k] = v
			}
		}
		for k := range b.settings {
			if _, ok := defaults[k]; !ok {
				delete(b.settings, k)
			}
		}
	}
	b.user = map[string]any{}
	for k, v := range SettingsWithFilters(b.settings, b.in.Filters) {
		b.user[k] = v
	}
	b.tsettings = b.tpl.SettingsContext()
}

func (b *builder) siteNavigation() []any {
	type item struct {
		m      map[string]any
		weight int
	}
	var items []item
	for _, a := range b.in.Articles {
		if a.IsIncludedInNavigation == nil || !*a.IsIncludedInNavigation {
			continue
		}
		weight := 1
		if a.NavigationWeight != nil {
			weight = *a.NavigationWeight
		}
		slug := a.ID
		if a.Slug != nil && *a.Slug != "" {
			slug = *a.Slug
		}
		m := map[string]any{"id": a.ID, "title": a.Title, "slug": slug, "weight": weight}
		if a.ExternalLink != nil {
			m["externalLink"] = *a.ExternalLink
		}
		items = append(items, item{m, weight})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].weight < items[j].weight })
	out := make([]any, len(items))
	for i, it := range items {
		out[i] = it.m
	}
	return out
}

func (b *builder) hasPodcast(pubs []map[string]any) bool {
	for _, p := range pubs {
		if p["audioFilename"] != nil {
			return true
		}
	}
	return false
}

func (b *builder) ogImageURL() string {
	if b.dwg != "" {
		return "https://" + b.dwg + "/avatar.png"
	}
	return "https://" + b.in.Planet.IPNS + ".eth.sucks/avatar.png"
}

// customCode renders the planet's own snippets against the page context.
func (b *builder) customCode(ctx map[string]any) map[string]any {
	out := map[string]any{"custom_code_head": "", "custom_code_body_start": "", "custom_code_body_end": ""}
	p := b.in.Planet
	render := func(key string, enabled *bool, code *string) {
		if enabled == nil || !*enabled || code == nil {
			return
		}
		if s, err := b.env.RenderString(*code, ctx); err == nil {
			out[key] = s
		}
	}
	render("custom_code_head", p.CustomCodeHeadEnabled, p.CustomCodeHead)
	render("custom_code_body_start", p.CustomCodeBodyStartEnabled, p.CustomCodeBodyStart)
	render("custom_code_body_end", p.CustomCodeBodyEndEnabled, p.CustomCodeBodyEnd)
	return out
}

func (b *builder) articlePath(a *planet.MyArticle) string {
	if a.Slug != nil && *a.Slug != "" {
		return *a.Slug
	}
	return a.ID
}

// articles renders every article folder, as savePublicInternal does.
func (b *builder) articles() error {
	for _, a := range b.in.Articles {
		src := ""
		if b.in.Attachments != nil {
			src = b.in.Attachments(a)
		}
		dir := filepath.Join(b.in.Out, a.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if src != "" {
			names := append([]string{}, a.Attachments...)
			for _, extra := range []string{"_videoThumbnail.png", "_preview.png", "_cover.png"} {
				if _, err := os.Stat(filepath.Join(src, extra)); err == nil {
					names = append(names, extra)
				}
			}
			for _, name := range names {
				if err := copyFile(filepath.Join(src, name), filepath.Join(dir, name)); err != nil {
					b.warn("%s: attachment %s: %v", a.ID, name, err)
				}
			}
		}
		if err := b.write(filepath.Join(a.ID, "article.md"), []byte(a.Title+"\n\n"+a.Content)); err != nil {
			return err
		}
		html, err := b.renderArticle(a, dir)
		if err != nil {
			return fmt.Errorf("%s: %w", a.ID, err)
		}
		if err := b.write(filepath.Join(a.ID, "index.html"), []byte(html)); err != nil {
			return err
		}
		if b.tpl.HasSimple {
			simple, err := b.renderArticleWith("simple.html", a, dir)
			if err != nil {
				return fmt.Errorf("%s: %w", a.ID, err)
			}
			if err := b.write(filepath.Join(a.ID, "simple.html"), []byte(simple)); err != nil {
				return err
			}
		}
		if err := b.write(filepath.Join(a.ID, "article.json"), planet.MarshalFoundation(b.pubs[len(b.pubs)-1])); err != nil {
			return err
		}
		if a.Slug != nil && *a.Slug != "" {
			dst := filepath.Join(b.in.Out, *a.Slug)
			os.RemoveAll(dst)
			if err := copyTree(dir, dst); err != nil {
				return err
			}
			b.res.Files = append(b.res.Files, *a.Slug+"/")
		}
	}
	return nil
}

// renderArticle renders blog.html for one article whose files are in
// dir, recording its projection for the planet-level pages.
func (b *builder) renderArticle(a *planet.MyArticle, dir string) (string, error) {
	return b.renderArticleWith("blog.html", a, dir)
}

func (b *builder) renderArticleWith(file string, a *planet.MyArticle, dir string) (string, error) {
	p := b.in.Planet
	if a.ContentRendered == nil && a.Content != "" {
		r := markdown.Render(a.Content)
		a.ContentRendered = &r
	}
	hero := detectHero(a, dir, b.base+b.articlePath(a)+"/")
	pub := planet.PublicArticle(a, hero, nil, nil)
	if file == "blog.html" {
		b.pubs = append(b.pubs, pub)
	}
	anyAudio := false
	for _, other := range b.in.Articles {
		if other.HasAudio() {
			anyAudio = true
		}
	}
	summary := ""
	if a.Summary != nil {
		summary = *a.Summary
	}
	social := b.ogImageURL()
	if hero != nil {
		social = hero.URL
	}
	ctx := map[string]any{
		"page_description_html": markdown.Render(p.About),
		"planet":                planet.ForTemplate(planet.TemplatePlanet(p, nil)),
		"site_navigation":       b.siteNavigation(),
		"has_avatar":            b.hasAvatar,
		"has_podcast":           anyAudio,
		"planet_ipns":           p.IPNS,
		"assets_prefix":         "../",
		"template_settings":     b.tsettings,
		"user_settings":         b.user,
		"article_id":            a.ID,
		"article":               planet.ForTemplate(pub),
		"article_type":          a.Type(),
		"article_title":         a.Title,
		"article_summary":       summary,
		"page_title":            a.Title,
		"content_html":          markdown.Render(a.Content),
		"build_timestamp":       int(b.now.Unix()),
		"style_css_sha256":      b.tpl.StyleCSSHash,
		"current_item_type":     "blog",
		"social_image_url":      social,
	}
	if b.in.HubThread != nil {
		if t := b.in.HubThread(a); t != nil {
			ctx["hub_thread"] = t
		}
	}
	for k, v := range b.customCode(ctx) {
		ctx[k] = v
	}
	return b.env.Render(file, ctx)
}

// Preview renders one article's page to a string without writing
// anything: the same context as a build, for the editor's live view.
func Preview(in Input, a *planet.MyArticle) (string, error) {
	if in.Planet == nil || in.Template == nil {
		return "", fmt.Errorf("preview: planet and template are required")
	}
	b := &builder{in: in, tpl: in.Template, loc: in.Location, now: in.Now, res: &Result{}}
	if b.loc == nil {
		b.loc = time.Local
	}
	if b.now.IsZero() {
		b.now = time.Now()
	}
	b.env = stencil.NewEnv(stencil.DirLoader(b.tpl.TemplatesDir()))
	registerPlanetFilters(b.env, b.loc)
	b.dwg = domainWithGateway(in.Planet.Domain)
	if b.dwg != "" {
		b.base = "https://" + b.dwg + "/"
	} else {
		b.base = "https://" + in.Planet.IPNS + ".eth.sucks/"
	}
	b.hasAvatar = in.AvatarPath != ""
	b.prepareSettings()
	src := ""
	if in.Attachments != nil {
		src = in.Attachments(a)
	}
	return b.renderArticle(a, src)
}

// pages renders the index, tags, archive, feeds and planet.json, as
// MyPlanetModel.savePublic does.
func (b *builder) pages() error {
	p := b.in.Planet
	var blog []map[string]any
	for i, a := range b.in.Articles {
		if a.Type() == planet.TypeBlog {
			blog = append(blog, b.pubs[i])
		}
	}
	publicPlanet := planet.PublicPlanet(p, blog)
	planetCtx := planet.ForTemplate(publicPlanet)
	all := make([]any, len(b.pubs))
	for i, pub := range b.pubs {
		all[i] = planet.ForTemplate(pub)
	}
	blogCtx := planetCtx["articles"].([]any)
	common := map[string]any{
		"planet":                planetCtx,
		"planet_ipns":           p.IPNS,
		"site_navigation":       b.siteNavigation(),
		"has_avatar":            b.hasAvatar,
		"og_image_url":          b.ogImageURL(),
		"has_podcast":           b.hasPodcast(blog),
		"has_podcast_cover_art": false,
	}

	// Index pages.
	per := b.tpl.ItemsPerPage()
	if flag(b.tpl.GenerateIndexPagination) && len(blog) > per {
		pages := int(math.Ceil(float64(len(blog)) / float64(per)))
		for i := 1; i <= pages; i++ {
			end := i * per
			if end > len(blog) {
				end = len(blog)
			}
			ctx := clone(common)
			ctx["page"], ctx["pages"], ctx["articles"] = i, pages, blogCtx[(i-1)*per:end]
			html, err := b.renderIndex(ctx)
			if err != nil {
				return err
			}
			if err := b.write(fmt.Sprintf("page%d.html", i), []byte(html)); err != nil {
				return err
			}
			if i == 1 {
				if err := b.write("index.html", []byte(html)); err != nil {
					return err
				}
			}
		}
	} else {
		ctx := clone(common)
		ctx["articles"] = blogCtx
		html, err := b.renderIndex(ctx)
		if err != nil {
			return err
		}
		if err := b.write("page1.html", []byte(html)); err != nil {
			return err
		}
		if err := b.write("index.html", []byte(html)); err != nil {
			return err
		}
	}

	// Tags.
	if flag(b.tpl.GenerateTagPages) {
		tagArticles := map[string][]any{}
		var order []string
		for _, pub := range all {
			m := pub.(map[string]any)
			tags, _ := m["tags"].(map[string]any)
			for key := range tags {
				if reservedTags[key] {
					continue
				}
				if _, ok := tagArticles[key]; !ok {
					order = append(order, key)
				}
				tagArticles[key] = append(tagArticles[key], m)
			}
		}
		sort.Strings(order)
		for _, key := range order {
			value := key
			if v, ok := p.Tags[key]; ok {
				value = v
			}
			ctx := clone(common)
			ctx["tag_key"], ctx["tag_value"] = key, value
			ctx["current_item_type"] = "tags"
			ctx["articles"] = tagArticles[key]
			ctx["page_title"] = p.Name + " - " + value
			html, err := b.renderIndex(ctx)
			if err != nil {
				return err
			}
			if err := b.write(key+".html", []byte(html)); err != nil {
				return err
			}
		}
		if b.tpl.HasTags {
			tags := map[string]any{}
			for k, v := range p.Tags {
				if !reservedTags[k] {
					tags[k] = v
				}
			}
			ta := map[string]any{}
			for k, v := range tagArticles {
				ta[k] = v
			}
			ctx := clone(common)
			ctx["tags"], ctx["tag_articles"] = tags, ta
			html, err := b.renderPage("tags.html", p.Name+" - Tags", "tags", ctx)
			if err != nil {
				return err
			}
			if err := b.write("tags.html", []byte(html)); err != nil {
				return err
			}
		}
	}

	// Archive.
	if flag(b.tpl.GenerateArchive) && b.tpl.HasArchive {
		archive := map[string]any{}
		var sections []any
		for _, pub := range all {
			m := pub.(map[string]any)
			created, _ := planet.TimeOf(m["created"])
			section := created.In(b.loc).Format("January 2006")
			if _, ok := archive[section]; !ok {
				sections = append(sections, section)
				archive[section] = []any{}
			}
			archive[section] = append(archive[section].([]any), m)
		}
		ctx := clone(common)
		ctx["articles"], ctx["archive"], ctx["archive_sections"] = all, archive, sections
		html, err := b.renderPage("archive.html", p.Name+" - Archive", "archive", ctx)
		if err != nil {
			return err
		}
		if err := b.write("archive.html", []byte(html)); err != nil {
			return err
		}
	}

	// Feeds: every article, pages included.
	if err := b.rss(false); err != nil {
		return err
	}
	if b.hasPodcast(b.pubs) {
		if err := b.rss(true); err != nil {
			return err
		}
	}

	if err := b.write("planet.json", planet.MarshalFoundation(publicPlanet)); err != nil {
		return err
	}
	robots := ""
	if p.DoNotIndex != nil && *p.DoNotIndex {
		robots = "User-agent: *\nDisallow: /"
	}
	if err := b.write("robots.txt", []byte(robots)); err != nil {
		return err
	}
	// Planet writes the settings file only for a template that has
	// settings, and otherwise copies the site's own file when it has one.
	if len(b.tpl.DefaultSettings()) > 0 || b.in.Settings != nil {
		return b.write("templateSettings.json", planet.MarshalFoundation(b.settings))
	}
	return nil
}

// renderIndex is Template.renderIndex: the base context, then the
// caller's keys on top.
func (b *builder) renderIndex(over map[string]any) (string, error) {
	p := b.in.Planet
	page, pages := 1, 1
	if v, ok := over["page"].(int); ok {
		page = v
	}
	if v, ok := over["pages"].(int); ok {
		pages = v
	}
	ctx := map[string]any{
		"assets_prefix":         "./",
		"template_settings":     b.tsettings,
		"user_settings":         b.user,
		"page_title":            p.Name,
		"page_description":      p.About,
		"page_description_html": markdown.Render(p.About),
		"articles":              []any{},
		"build_timestamp":       int(b.now.Unix()),
		"style_css_sha256":      b.tpl.StyleCSSHash,
		"current_item_type":     "index",
		"current_page":          page,
		"total_pages":           pages,
	}
	if page < pages {
		ctx["next_page"] = fmt.Sprintf("page%d.html", page+1)
	}
	if page > 1 {
		ctx["previous_page"] = fmt.Sprintf("page%d.html", page-1)
	}
	for k, v := range over {
		ctx[k] = v
	}
	for k, v := range b.customCode(ctx) {
		ctx[k] = v
	}
	return b.env.Render("index.html", ctx)
}

// renderPage is renderTags and renderArchive.
func (b *builder) renderPage(file, title, kind string, over map[string]any) (string, error) {
	p := b.in.Planet
	ctx := map[string]any{
		"assets_prefix":         "./",
		"template_settings":     b.tsettings,
		"user_settings":         b.user,
		"page_title":            title,
		"page_description":      p.About,
		"page_description_html": markdown.Render(p.About),
		"articles":              []any{},
		"build_timestamp":       int(b.now.Unix()),
		"style_css_sha256":      b.tpl.StyleCSSHash,
		"current_item_type":     kind,
	}
	for k, v := range over {
		ctx[k] = v
	}
	for k, v := range b.customCode(ctx) {
		ctx[k] = v
	}
	return b.env.Render(file, ctx)
}

// rss is renderRSS: Planet's RSS.xml over every article, or over the
// audio ones for podcast.xml.
func (b *builder) rss(podcast bool) error {
	p := b.in.Planet
	var items []map[string]any
	for _, pub := range b.pubs {
		if podcast && pub["audioFilename"] == nil {
			continue
		}
		items = append(items, pub)
	}
	rootPrefix := "https://eth.sucks/ipns/" + p.IPNS
	if b.dwg != "" {
		rootPrefix = "https://" + b.dwg
	}
	hasDomain := p.Domain != nil && *p.Domain != "" && !strings.Contains(*p.Domain, ":")
	author := ""
	if p.AuthorName != nil {
		author = *p.AuthorName
	}
	ctx := map[string]any{
		"planet":                planet.ForTemplate(planet.PublicPlanet(p, items)),
		"has_domain":            hasDomain,
		"domain":                b.dwg,
		"root_prefix":           rootPrefix,
		"podcast":               podcast,
		"podcast_author_name":   author,
		"has_podcast_cover_art": false,
	}
	xml, err := b.rssEnv.RenderString(rssTemplate, ctx)
	if err != nil {
		return err
	}
	// A template that ships assets/rss.xsl gets the feed styled: a browser
	// opening rss.xml renders the stylesheet's page, a reader ignores it.
	if _, err := os.Stat(filepath.Join(b.tpl.Dir, "assets", "rss.xsl")); err == nil {
		if head, rest, ok := strings.Cut(xml, "\n"); ok && strings.HasPrefix(head, "<?xml") {
			xml = head + "\n" + `<?xml-stylesheet type="text/xsl" href="assets/rss.xsl"?>` + "\n" + rest
		}
	}
	name := "rss.xml"
	if podcast {
		name = "podcast.xml"
	}
	return b.write(name, []byte(xml))
}

func clone(m map[string]any) map[string]any {
	out := make(map[string]any, len(m)+8)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}
