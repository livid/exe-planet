package service

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/livid/exe-planet/internal/build"
	"github.com/livid/exe-planet/internal/markdown"
	"github.com/livid/exe-planet/internal/planet"
	"github.com/livid/exe-planet/internal/site"
)

// Status is what the daemon knows about a site's last build.
type Status struct {
	Site      string    `json:"site"`
	Public    string    `json:"public"`
	Built     time.Time `json:"built,omitempty"`
	Duration  int64     `json:"duration_ms"`
	Files     int       `json:"files"`
	Warnings  []string  `json:"warnings,omitempty"`
	Error     string    `json:"error,omitempty"`
	Building  bool      `json:"building"`
	Requested bool      `json:"requested,omitempty"`
}

// Service builds sites.
type Service struct {
	Store     *site.Store
	State     string // ~/.exe/planet
	Templates string // the Workspace's Planet/Templates
	Location  *time.Location
	Logf      func(format string, args ...any)

	mu     sync.Mutex
	sites  map[string]*siteState
	status map[string]Status
}

type siteState struct {
	building bool
	again    bool
}

// New makes a service; templates come from the store's Templates folder.
func New(store *site.Store, state string, loc *time.Location) *Service {
	return &Service{
		Store:     store,
		State:     state,
		Templates: filepath.Join(store.Root, site.TemplatesDir),
		Location:  loc,
		Logf:      log.Printf,
		sites:     map[string]*siteState{},
		status:    map[string]Status{},
	}
}

// PublicDir is where a site's build lives on this node.
func (s *Service) PublicDir(id string) string {
	return filepath.Join(s.State, id, "public")
}

// Status reports a site's last build.
func (s *Service) Status(slug string) (Status, error) {
	st, err := s.Store.Site(slug)
	if err != nil {
		return Status{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.status[slug]
	out.Site, out.Public = slug, s.PublicDir(st.ID)
	if ss := s.sites[slug]; ss != nil {
		out.Building, out.Requested = ss.building, ss.again
	}
	return out, nil
}

// Build builds one site now, or queues one more build if one is running:
// a burst of saves ends in exactly one build after the last.
func (s *Service) Build(slug string) (Status, error) {
	s.mu.Lock()
	ss := s.sites[slug]
	if ss == nil {
		ss = &siteState{}
		s.sites[slug] = ss
	}
	if ss.building {
		ss.again = true
		s.mu.Unlock()
		return s.Status(slug)
	}
	ss.building = true
	s.mu.Unlock()

	var st Status
	var err error
	for {
		st, err = s.buildOnce(slug)
		s.mu.Lock()
		s.status[slug] = st
		if !ss.again {
			ss.building = false
			s.mu.Unlock()
			break
		}
		ss.again = false
		s.mu.Unlock()
	}
	return st, err
}

func (s *Service) buildOnce(slug string) (Status, error) {
	started := time.Now()
	st, err := s.Store.Site(slug)
	if err != nil {
		return Status{Site: slug, Error: err.Error()}, err
	}
	posts, err := s.fullPosts(slug)
	if err != nil {
		return Status{Site: slug, Error: err.Error()}, err
	}
	in, err := s.input(st, posts)
	if err != nil {
		return Status{Site: slug, Public: s.PublicDir(st.ID), Error: err.Error()}, err
	}
	// Build beside the live folder and swap, so a page is never served
	// half written.
	final := s.PublicDir(st.ID)
	in.Out = final + ".new"
	os.RemoveAll(in.Out)
	res, err := build.Build(in)
	if err != nil {
		os.RemoveAll(in.Out)
		s.Logf("planet: build %s: %v", slug, err)
		return Status{Site: slug, Public: final, Error: err.Error()}, err
	}
	old := final + ".old"
	os.RemoveAll(old)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return Status{Site: slug, Public: final, Error: err.Error()}, err
	}
	if _, statErr := os.Stat(final); statErr == nil {
		if err := os.Rename(final, old); err != nil {
			return Status{Site: slug, Public: final, Error: err.Error()}, err
		}
	}
	if err := os.Rename(in.Out, final); err != nil {
		os.Rename(old, final)
		return Status{Site: slug, Public: final, Error: err.Error()}, err
	}
	os.RemoveAll(old)
	out := Status{Site: slug, Public: final, Built: time.Now(), Duration: time.Since(started).Milliseconds(), Files: len(res.Files), Warnings: res.Warnings}
	s.Logf("planet: built %s: %d files in %d ms", slug, len(res.Files), out.Duration)
	return out, nil
}

// fullPosts lists a site's posts with their bodies, which the listing
// leaves out.
func (s *Service) fullPosts(slug string) ([]*site.Post, error) {
	listed, err := s.Store.Posts(slug, "")
	if err != nil {
		return nil, err
	}
	posts := make([]*site.Post, 0, len(listed))
	for _, p := range listed {
		full, err := s.Store.Post(slug, p.Slug, p.Kind)
		if err != nil {
			if full != nil {
				full.Error = err.Error()
				posts = append(posts, full)
			}
			continue
		}
		posts = append(posts, full)
	}
	return posts, nil
}

// input assembles the builder's input for a site.
func (s *Service) input(st *site.Site, posts []*site.Post) (build.Input, error) {
	tpl, err := build.FindTemplate(s.Templates, st.Template)
	if err != nil {
		return build.Input{}, err
	}
	p, articles := FromSite(st, posts)
	if newest := newestChange(st.Dir); planet.DateOf(newest) > p.Updated {
		p.Updated = planet.DateOf(newest)
	}
	dirs := map[string]string{}
	for _, post := range posts {
		dirs[post.ID] = post.Dir
	}
	in := build.Input{
		Planet:   p,
		Articles: articles,
		Template: tpl,
		Settings: st.TemplateSettings,
		Attachments: func(a *planet.MyArticle) string {
			return dirs[a.ID]
		},
		Now:      time.Now(),
		Location: s.Location,
	}
	if _, err := os.Stat(filepath.Join(st.Dir, "avatar.png")); err == nil {
		in.AvatarPath = filepath.Join(st.Dir, "avatar.png")
	}
	if _, err := os.Stat(filepath.Join(st.Dir, "favicon.ico")); err == nil {
		in.FaviconPath = filepath.Join(st.Dir, "favicon.ico")
	}
	return in, nil
}

// Preview renders one post through the site's template without saving
// anything: the editor's right column. The post need not exist yet.
func (s *Service) Preview(slug string, post *site.Post) (string, error) {
	st, err := s.Store.Site(slug)
	if err != nil {
		return "", err
	}
	posts, err := s.fullPosts(slug)
	if err != nil {
		return "", err
	}
	in, err := s.input(st, posts)
	if err != nil {
		return "", err
	}
	if post.ID == "" {
		post.ID = "PREVIEW"
	}
	if post.Created.IsZero() {
		post.Created = time.Now()
	}
	if post.Kind == "" {
		post.Kind = site.KindPost
	}
	_, drafts := FromSite(st, []*site.Post{post})
	if len(drafts) == 0 {
		return "", errors.New("nothing to preview")
	}
	return build.Preview(in, drafts[0])
}

// OnChange is the watcher's callback: paths are relative to the Planet
// folder. A change under a site rebuilds that site; a change under
// Templates rebuilds every site.
func (s *Service) OnChange(paths []string) {
	slugs := map[string]bool{}
	all := false
	for _, p := range paths {
		first, _, _ := strings.Cut(p, "/")
		switch {
		case first == site.TemplatesDir:
			all = true
		case first == site.ArchivedDir, first == "", strings.HasPrefix(first, "."):
		default:
			if site.ValidSiteSlug(first) == nil {
				slugs[first] = true
			}
		}
	}
	if all {
		sites, _ := s.Store.Sites(false)
		for _, st := range sites {
			slugs[st.Slug] = true
		}
	}
	for slug := range slugs {
		if _, err := s.Store.Site(slug); err != nil {
			continue // deleted or broken; nothing to build
		}
		go func(slug string) {
			if _, err := s.Build(slug); err != nil {
				s.Logf("planet: rebuild %s after a change: %v", slug, err)
			}
		}(slug)
	}
}

// BuildAll builds every live site, for start-up.
func (s *Service) BuildAll() {
	sites, err := s.Store.Sites(false)
	if err != nil {
		return
	}
	for _, st := range sites {
		if st.Error != "" {
			continue
		}
		if _, err := s.Build(st.Slug); err != nil {
			s.Logf("planet: build %s: %v", st.Slug, err)
		}
	}
}

// InstallTemplates copies the shipped templates into the Workspace when
// missing or older by buildNumber, never over a folder with a .git.
func InstallTemplates(src, dst string) error {
	ents, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		from, to := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		shipped, err := build.LoadTemplate(from)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(to, ".git")); err == nil {
			continue
		}
		if have, err := build.LoadTemplate(to); err == nil {
			if have.BuildNumber != nil && shipped.BuildNumber != nil && *have.BuildNumber >= *shipped.BuildNumber {
				continue
			}
		}
		tmp := to + ".new"
		os.RemoveAll(tmp)
		if err := copyTree(from, tmp); err != nil {
			return fmt.Errorf("template %s: %w", e.Name(), err)
		}
		os.RemoveAll(to)
		if err := os.Rename(tmp, to); err != nil {
			return err
		}
	}
	return nil
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
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

// Attachments describes where a frame fetches an article's own files:
// the tokenised attachment route the app names, or nothing, in which
// case they are folded in as data URIs.
type Attachments struct {
	Base  string // e.g. /v1/svc/planet/v1/sites/notes/posts/hello/attachments/
	Query string // e.g. token=…
}

func (a Attachments) url(name string) string {
	if a.Base == "" {
		return ""
	}
	u := a.Base + pathEscape(name)
	if a.Query != "" {
		u += "?" + a.Query
	}
	return u
}

func attachmentURL(a Attachments) func(string) string {
	if a.Base == "" {
		return nil
	}
	return a.url
}

// View is the built page of a post with its resources folded in, for the
// window's page column.
func (s *Service) View(slug, postSlug string, att Attachments) (string, error) {
	st, err := s.Store.Site(slug)
	if err != nil {
		return "", err
	}
	p, err := s.Store.Post(slug, postSlug, "")
	if err != nil {
		return "", err
	}
	pub := s.PublicDir(st.ID)
	page, err := os.ReadFile(filepath.Join(pub, p.ID, "index.html"))
	if err != nil {
		return "", fmt.Errorf("%w: the site is not built yet", site.ErrNotFound)
	}
	return build.Inline(string(page), build.InlineOptions{
		PageDir:       filepath.Join(pub, p.ID),
		RootDir:       pub,
		AssetsDir:     filepath.Join(pub, "assets"),
		AttachmentURL: attachmentURL(att),
	}), nil
}

// RenderPath renders a Markdown file's text for the Writer: a post under
// Planet/<site>/ goes through that site's template with its front matter
// read, any other file renders as plain GitHub Markdown in a small page.
// The path is relative to the Workspace root; the text is the field's,
// which may not be saved yet.
func (s *Service) RenderPath(rel, text string, att Attachments) (string, error) {
	if !WorkspaceRel(rel) {
		return "", fmt.Errorf("%w: path must be relative to the Workspace, with no . or .. segments", site.ErrInvalid)
	}
	workspace := filepath.Dir(s.Store.Root)
	planetDir := filepath.Base(s.Store.Root)
	parts := strings.Split(rel, "/")
	if len(parts) == 5 && parts[0] == planetDir && parts[4] == "index.md" {
		kind := map[string]string{"posts": site.KindPost, "pages": site.KindPage, "drafts": site.KindDraft}[parts[2]]
		if kind != "" && site.ValidSiteSlug(parts[1]) == nil && site.ValidSlug(parts[3]) == nil {
			return s.renderPost(parts[1], kind, parts[3], text, att)
		}
	}
	m, body, err := site.ParseDoc([]byte(text))
	if err != nil {
		body = text
	}
	title := m.Title
	if title == "" {
		title = firstHeading(body)
	}
	if title == "" {
		title = path.Base(rel)
	}
	page := plainPage(title, markdown.Render(body))
	return build.Inline(page, build.InlineOptions{PageDir: filepath.Join(workspace, filepath.FromSlash(path.Dir(rel))), AttachmentURL: attachmentURL(att)}), nil
}

func (s *Service) renderPost(slug, kind, postSlug, text string, att Attachments) (string, error) {
	st, err := s.Store.Site(slug)
	if err != nil {
		return "", err
	}
	m, body, err := site.ParseDoc([]byte(text))
	if err != nil {
		return "", fmt.Errorf("%w: %v", site.ErrInvalid, err)
	}
	post := &site.Post{Kind: kind, Slug: postSlug, Meta: m, Body: body}
	if existing, err := s.Store.Post(slug, postSlug, kind); err == nil {
		post.Dir, post.Attachments = existing.Dir, existing.Attachments
		if post.ID == "" {
			post.ID = existing.ID
		}
		if post.Created.IsZero() {
			post.Created = existing.Created
		}
	}
	html, err := s.Preview(slug, post)
	if err != nil {
		return "", err
	}
	tpl, err := build.FindTemplate(s.Templates, st.Template)
	if err != nil {
		return "", err
	}
	return build.Inline(html, build.InlineOptions{
		PageDir:       post.Dir,
		RootDir:       s.PublicDir(st.ID),
		AssetsDir:     filepath.Join(tpl.Dir, "assets"),
		AttachmentURL: attachmentURL(att),
	}), nil
}

// WorkspaceRel accepts a path relative to the Workspace root: no leading
// slash, no backslash, no empty, "." or ".." segment.
func WorkspaceRel(p string) bool {
	if p == "" || len(p) > 1024 || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// firstHeading is a Markdown file's first ATX heading, its natural title.
func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "#") {
			return strings.TrimSpace(strings.TrimLeft(t, "#"))
		}
	}
	return ""
}

// plainPage wraps rendered Markdown for a file that belongs to no site.
func plainPage(title, body string) string {
	return "<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>" +
		htmlEscape(title) + "</title><style>body{margin:0;padding:24px 28px;font:16px/1.55 -apple-system,BlinkMacSystemFont,\"Segoe UI\",Helvetica,Arial,sans-serif;color:#222;background:#fff}" +
		"main{max-width:720px}img{max-width:100%}pre{background:#f4f4f4;padding:12px;overflow:auto}code{font:13px ui-monospace,Menlo,monospace}" +
		"blockquote{margin:0;padding-left:14px;border-left:3px solid #ccc;color:#555}table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:4px 8px}</style></head><body><main>" +
		body + "</main></body></html>"
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

func pathEscape(s string) string {
	const keep = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~!$&'()*+,;=:@"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if strings.IndexByte(keep, c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
