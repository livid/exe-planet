package service

import (
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/livid/exe-planet/internal/build"
	"github.com/livid/exe-planet/internal/kubo"
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
	// Hash is what the build was made from (the site folder, the
	// template, the engine); a publish happens when it moves.
	Hash string `json:"hash,omitempty"`
	id   string
}

// Service builds sites.
type Service struct {
	Store     *site.Store
	State     string // ~/.exe/planet
	Templates string // the Workspace's Planet/Templates
	Location  *time.Location
	Logf      func(format string, args ...any)
	// Exe and Kubo are the node's publishing; nil is a node without.
	// Backend is this daemon's own origin, the route target of an expose.
	Exe     *Exe
	Kubo    *kubo.Client
	Backend string
	// HubHTTP is the client the site's key reaches its hub with; nil is a
	// plain one with a timeout.
	HubHTTP *http.Client
	// Version is the daemon's; a new engine is a reason to publish again.
	Version string

	mu         sync.Mutex
	sites      map[string]*siteState
	status     map[string]Status
	hosts      map[string]string // public hostname -> site id
	publishing map[string]*pubState
	nodeMu     sync.Mutex // node.json read-modify-write
	// the site on the hub (hub.go): one announcing pass per site, a timer
	// for one the hub's cooldown sent away, and hub.json's lock
	announcing     map[string]*pubState
	announceTimers map[string]*time.Timer
	hubMu          sync.Mutex
}

type siteState struct {
	building bool
	again    bool
}

// New makes a service; templates come from the store's Templates folder.
func New(store *site.Store, state string, loc *time.Location) *Service {
	return &Service{
		Store:      store,
		State:      state,
		Templates:  filepath.Join(store.Root, site.TemplatesDir),
		Location:   loc,
		Logf:       log.Printf,
		sites:      map[string]*siteState{},
		status:     map[string]Status{},
		hosts:      map[string]string{},
		publishing: map[string]*pubState{},

		announcing:     map[string]*pubState{},
		announceTimers: map[string]*time.Timer{},
	}
}

// Start reads the node files, so exposed hostnames answer at once.
func (s *Service) Start() { s.loadHosts() }

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
		if err == nil && st.Hash != "" {
			s.afterBuild(slug, st.id, st.Hash)
		}
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
	// A post made by hand has no id yet, and cannot be built without one:
	// it is issued now, into its file (the watcher sees that and builds
	// again, which finds nothing left to issue).
	adopted, err := s.Store.AdoptPosts(slug)
	if len(adopted) > 0 {
		s.Logf("planet: %s: issued an id or a date to %s", slug, strings.Join(adopted, ", "))
	}
	if err != nil {
		s.Logf("planet: %s: a post made by hand could not be written to: %v", slug, err)
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
	out := Status{Site: slug, Public: final, Built: time.Now(), Duration: time.Since(started).Milliseconds(), Files: len(res.Files), Warnings: res.Warnings, id: st.ID}
	// Two templates stamp build_timestamp into their pages, so the
	// output never compares equal; the inputs do.
	salt := s.Version
	if in.Planet.Domain != nil {
		salt += " " + *in.Planet.Domain
	}
	if h, err := inputHash(st.Dir, in.Template.Dir, salt); err == nil {
		out.Hash = h
	}
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
	// An exposed site is Planet's "domain" case: its feed and its absolute
	// links carry the hostname instead of an IPNS gateway.
	if n := s.loadNode(st.ID); n.Expose && n.Hostname != "" && p.Domain == nil {
		host := n.Hostname
		p.Domain = &host
	}
	if newest := newestChange(st.Dir); planet.DateOf(newest) > p.Updated {
		p.Updated = planet.DateOf(newest)
	}
	dirs := map[string]string{}
	threads := map[string]map[string]any{}
	lost := s.ledgerHubs(st.ID) // a thread whose hub: line a save dropped still shows
	for _, post := range posts {
		dirs[post.ID] = post.Dir
		hub := post.Hub
		if hub == "" && post.Kind == "post" {
			hub = lost[post.ID]
		}
		if t := hubThreadFor(hub); t != nil {
			threads[post.ID] = t
		}
	}
	in := build.Input{
		Planet:   p,
		Articles: articles,
		Template: tpl,
		Settings: st.TemplateSettings,
		Attachments: func(a *planet.MyArticle) string {
			return dirs[a.ID]
		},
		HubThread: func(a *planet.MyArticle) map[string]any {
			return threads[a.ID]
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
	if p.ID == "" {
		// made by hand a moment ago: its id comes with the build on its way
		return "", fmt.Errorf("%w: the site is not built yet", site.ErrNotFound)
	}
	page, err := os.ReadFile(filepath.Join(pub, p.ID, "index.html"))
	if err != nil {
		return "", fmt.Errorf("%w: the site is not built yet", site.ErrNotFound)
	}
	return withLinkBridge(build.Inline(string(page), build.InlineOptions{
		PageDir:       filepath.Join(pub, p.ID),
		RootDir:       pub,
		AssetsDir:     filepath.Join(pub, "assets"),
		AttachmentURL: attachmentURL(att),
	})), nil
}

// ViewPath answers any page of a site's build by its path (index.html
// when empty, a folder's index.html for a folder), its resources folded
// in as data URIs, for the Planet window's page column.
func (s *Service) ViewPath(slug, rel string) (string, error) {
	st, err := s.Store.Site(slug)
	if err != nil {
		return "", err
	}
	if rel == "" {
		rel = "index.html"
	}
	clean := path.Clean("/" + rel)
	if clean == "/" || strings.HasSuffix(rel, "/") {
		clean = strings.TrimSuffix(clean, "/") + "/index.html"
	}
	pub := s.PublicDir(st.ID)
	file := filepath.Join(pub, filepath.FromSlash(clean))
	if fi, err := os.Stat(file); err == nil && fi.IsDir() {
		file = filepath.Join(file, "index.html")
	}
	page, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("%w: no page %s in the build", site.ErrNotFound, strings.TrimPrefix(clean, "/"))
	}
	return withLinkBridge(build.Inline(string(page), build.InlineOptions{
		PageDir:   filepath.Dir(file),
		RootDir:   pub,
		AssetsDir: filepath.Join(pub, "assets"),
	})), nil
}

// linkBridge is the script a viewed page carries: in the Planet window's
// sandboxed frame a relative link would resolve against the app's own
// address, so every link is handed to the window around it instead,
// which shows the page or picks the post. A link to a place in the page
// itself (a footnote, "#fn:1") is followed here, by scrolling to it: left
// to the frame it went to about:blank#fn:1, and the page was gone.
const linkBridge = `<script>(function(){document.addEventListener("click",function(e){var a=e.target&&e.target.closest?e.target.closest("a[href]"):null;if(!a)return;var h=a.getAttribute("href")||"";if(!h)return;e.preventDefault();if(h.charAt(0)==="#"){try{var t=document.getElementById(decodeURIComponent(h.slice(1)));if(t)t.scrollIntoView()}catch(x){}return}parent.postMessage({exe:"page-link",href:h,external:/^([a-z][a-z0-9+.-]*:|\/\/)/i.test(h)},"*")},true)})()</script>`

// todoBridge is the Planet window's part of a template's to-do lists
// (PLAN.md, To-do lists): what Planet's own article view does with a
// click on an item the page ids todo-item-N. It ticks the item's
// checkbox at once and tells the window around it which item that is,
// what it is now, and how every item stood before (was), so the window
// can have the line flipped in the post's Markdown, and the daemon can
// tell a page that is behind its file. The count is its own, not the
// id's: the bullets at the margin with a checkbox of their own, in
// reading order, as markdown.TodoItems counts them in the source. A
// template that ids other items (Sepia ids an item for a box nested
// inside it) has those clicks answered by nothing.
const todoBridge = `<script>(function(){var own=function(li){return li.querySelector(':scope > input[type="checkbox"], :scope > p:first-child > input[type="checkbox"]')};document.addEventListener("click",function(e){var li=e.target;if(!li||!li.id||li.id.indexOf("todo-item-")!==0)return;var items=[].filter.call(document.querySelectorAll("ul > li"),function(x){return own(x)&&!x.parentElement.closest("li, blockquote")});var k=items.indexOf(li);if(k<0)return;var was=items.map(function(x){return own(x).checked?"1":"0"}).join("");var box=own(li);box.checked=!box.checked;parent.postMessage({exe:"page-todo",item:k+1,done:box.checked,was:was},"*")},true)})()</script>`

// scrollBridge tells the window how far down the page is read, so the
// same page shown again after a build (a ticked item is one) can be put
// back where it was: the frame is sandboxed, and the window cannot look.
const scrollBridge = `<script>(function(){var t=0;addEventListener("scroll",function(){if(t)return;t=requestAnimationFrame(function(){t=0;parent.postMessage({exe:"page-scroll",y:Math.round(scrollY)},"*")})},{passive:true})})()</script>`

func withLinkBridge(html string) string {
	bridges := linkBridge + todoBridge + scrollBridge
	if i := strings.LastIndex(html, "</body>"); i >= 0 {
		return html[:i] + bridges + html[i:]
	}
	return html + bridges
}

// TickTodo sets one to-do item of a post done or not done in its
// Markdown: item counts the bullets at the margin with a box, from one
// (markdown.TodoItems), and was, when given, is how the caller saw every
// item stand, "1" for done, in order. A file whose items stand otherwise
// has moved on from the page the caller looked at: nothing is written
// and the error is site.ErrChanged. One byte of the file changes, the
// mark between the item's brackets; an item already as asked changes
// nothing. The watcher builds the site from the changed file as it does
// after any edit.
func (s *Service) TickTodo(slug, postSlug, kind string, item int, done bool, was string) error {
	return s.Store.RewritePost(slug, postSlug, kind, func(data []byte) ([]byte, error) {
		off := site.BodyOffset(data)
		if off < 0 {
			return nil, fmt.Errorf("%w: front matter is not closed", site.ErrInvalid)
		}
		items := markdown.TodoItems(data[off:])
		if was != "" {
			var now strings.Builder
			for _, it := range items {
				if it.Done {
					now.WriteByte('1')
				} else {
					now.WriteByte('0')
				}
			}
			if now.String() != was {
				return nil, fmt.Errorf("%w: the post's to-do items are not as the page shows them", site.ErrChanged)
			}
		}
		if item < 1 || item > len(items) {
			return nil, fmt.Errorf("%w: no to-do item %d", site.ErrNotFound, item)
		}
		it := items[item-1]
		if it.Done == done {
			return data, nil
		}
		out := append([]byte(nil), data...)
		if done {
			out[off+it.At] = 'x'
		} else {
			out[off+it.At] = ' '
		}
		return out, nil
	})
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
	_, body, err := site.ParseDoc([]byte(text))
	if err != nil {
		body = text
	}
	return build.Inline(writerPage(markdown.Render(body)), build.InlineOptions{
		PageDir:       filepath.Join(workspace, filepath.FromSlash(path.Dir(rel))),
		AttachmentURL: attachmentURL(att),
	}), nil
}

// writerTemplate is Planet's own Writer preview page (WriterBasic.html):
// the rendered Markdown in a .content div, no title, no site chrome. The
// site's real template is the Planet window's business.
//
//go:embed writer.html
var writerTemplate string

// writerPage puts rendered Markdown into the Writer template.
func writerPage(contentHTML string) string {
	return strings.Replace(writerTemplate, "{{ content_html }}", contentHTML, 1)
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
