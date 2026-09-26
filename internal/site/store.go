package site

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Post kinds, each a folder inside a site.
const (
	KindPost  = "post"
	KindPage  = "page"
	KindDraft = "draft"
)

// Kinds lists the post kinds in lookup order.
var Kinds = []string{KindPost, KindPage, KindDraft}

var kindDirs = map[string]string{KindPost: "posts", KindPage: "pages", KindDraft: "drafts"}

// Reserved folder names under the Planet root.
const (
	TemplatesDir = "Templates"
	ArchivedDir  = "Archived"
)

var (
	// ErrNotFound is a site, post, attachment or template that is not there.
	ErrNotFound = errors.New("not found")
	// ErrExists is a slug that is already taken.
	ErrExists = errors.New("already exists")
	// ErrInvalid is a request the store cannot act on, with a reason.
	ErrInvalid = errors.New("invalid")
)

// Social holds the handles Planet templates link to.
type Social struct {
	Twitter   string `json:"twitter,omitempty"`
	GitHub    string `json:"github,omitempty"`
	Telegram  string `json:"telegram,omitempty"`
	Mastodon  string `json:"mastodon,omitempty"`
	Discord   string `json:"discord,omitempty"`
	Farcaster string `json:"farcaster,omitempty"`
}

// Plausible is the analytics block templates read.
type Plausible struct {
	Enabled bool   `json:"enabled"`
	Domain  string `json:"domain,omitempty"`
	Server  string `json:"server,omitempty"`
}

// SiteFile is site.json: everything about a site that is shared through
// the Workspace. No token or key lives here.
type SiteFile struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	About            string            `json:"about"`
	Template         string            `json:"template"`
	TemplateSettings map[string]string `json:"template_settings,omitempty"`
	IPNS             string            `json:"ipns,omitempty"`
	Created          time.Time         `json:"created"`
	Author           string            `json:"author,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
	Social           Social            `json:"social"`
	Plausible        Plausible         `json:"plausible"`
	// NoIndex asks search engines away: robots.txt disallows everything.
	NoIndex bool `json:"noindex,omitempty"`
}

// Site is a site as the API sees it: its file plus what the folder says.
type Site struct {
	SiteFile
	Slug     string `json:"slug"`
	Archived bool   `json:"archived,omitempty"`
	// Error is set when the folder holds a site.json the daemon cannot
	// read; the site is listed rather than silently dropped.
	Error  string `json:"error,omitempty"`
	Posts  int    `json:"posts"`
	Pages  int    `json:"pages"`
	Drafts int    `json:"drafts"`
	Dir    string `json:"-"`
}

// SiteUpdate carries the fields PUT may change; nil leaves a field alone.
// The id, slug and created date never change through the API.
type SiteUpdate struct {
	Name             *string            `json:"name"`
	About            *string            `json:"about"`
	Template         *string            `json:"template"`
	TemplateSettings *map[string]string `json:"template_settings"`
	IPNS             *string            `json:"ipns"`
	Author           *string            `json:"author"`
	Tags             *map[string]string `json:"tags"`
	Social           *Social            `json:"social"`
	Plausible        *Plausible         `json:"plausible"`
	NoIndex          *bool              `json:"noindex"`
}

// Post is a post, page or draft: its folder's index.md and attachments.
type Post struct {
	Kind string `json:"kind"`
	Slug string `json:"slug"`
	Meta
	Body        string   `json:"body,omitempty"`
	Attachments []string `json:"attachments"`
	Error       string   `json:"error,omitempty"`
	Dir         string   `json:"-"`
}

// PostUpdate carries the fields PUT may change; nil leaves a field alone.
// A body is stored as a text file, so it comes back ending in one newline.
type PostUpdate struct {
	Title   *string    `json:"title"`
	Body    *string    `json:"body"`
	Tags    *[]string  `json:"tags"`
	Hero    *string    `json:"hero"`
	Link    *string    `json:"link"`
	Created *time.Time `json:"created"`
}

// Template is a template folder's template.json.
type Template struct {
	Slug                    string `json:"slug"`
	Name                    string `json:"name"`
	Description             string `json:"description"`
	Author                  string `json:"author"`
	AuthorWebsite           string `json:"authorWebsite"`
	Version                 string `json:"version"`
	BuildNumber             int    `json:"buildNumber"`
	IdealItemsPerPage       int    `json:"idealItemsPerPage"`
	GenerateIndexPagination bool   `json:"generateIndexPagination"`
	GenerateTagPages        bool   `json:"generateTagPages"`
	GenerateArchive         bool   `json:"generateArchive"`
	Error                   string `json:"error,omitempty"`
}

// Store is the Planet folder. Every method reads the disk afresh; the
// folder is the only state, so an edit made by hand, by an agent or by a
// synced peer is seen at once.
type Store struct {
	Root string
	Now  func() time.Time
	mu   sync.Mutex
}

// New opens the Planet folder at root.
func New(root string) *Store {
	return &Store{Root: root, Now: time.Now}
}

// Init creates the root and its reserved folders.
func (s *Store) Init() error {
	for _, d := range []string{s.Root, filepath.Join(s.Root, TemplatesDir), filepath.Join(s.Root, ArchivedDir)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) now() time.Time { return s.Now().Truncate(time.Second) }

func (s *Store) siteDir(slug string, archived bool) string {
	if archived {
		return filepath.Join(s.Root, ArchivedDir, slug)
	}
	return filepath.Join(s.Root, slug)
}

// Sites lists the live sites, or the archived ones, sorted by slug. A
// folder without a site.json is not a site; a folder whose site.json
// cannot be read is listed with its error.
func (s *Store) Sites(archived bool) ([]*Site, error) {
	dir := s.Root
	if archived {
		dir = filepath.Join(s.Root, ArchivedDir)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if archived && os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sites []*Site
	for _, e := range ents {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if !archived && reservedSiteSlugs[strings.ToLower(name)] {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name, "site.json")); err != nil {
			continue
		}
		sites = append(sites, s.readSite(filepath.Join(dir, name), name, archived))
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].Slug < sites[j].Slug })
	return sites, nil
}

func (s *Store) readSite(dir, slug string, archived bool) *Site {
	st := &Site{Slug: slug, Archived: archived, Dir: dir}
	if err := ValidSiteSlug(slug); err != nil {
		st.Error = "folder name: " + err.Error()
		return st
	}
	b, err := os.ReadFile(filepath.Join(dir, "site.json"))
	if err != nil {
		st.Error = err.Error()
		return st
	}
	if err := json.Unmarshal(b, &st.SiteFile); err != nil {
		st.Error = "site.json: " + err.Error()
		return st
	}
	if st.ID == "" {
		st.Error = "site.json: id is missing"
		return st
	}
	for _, kind := range Kinds {
		n := s.countPosts(dir, kind)
		switch kind {
		case KindPost:
			st.Posts = n
		case KindPage:
			st.Pages = n
		case KindDraft:
			st.Drafts = n
		}
	}
	return st
}

func (s *Store) countPosts(siteDir, kind string) int {
	ents, err := os.ReadDir(filepath.Join(siteDir, kindDirs[kind]))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			if _, err := os.Stat(filepath.Join(siteDir, kindDirs[kind], e.Name(), "index.md")); err == nil {
				n++
			}
		}
	}
	return n
}

// Site returns one live site. A site the daemon cannot read comes back
// with its error, so the caller can show it.
func (s *Store) Site(slug string) (*Site, error) {
	if err := ValidSiteSlug(slug); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	dir := s.siteDir(slug, false)
	if _, err := os.Stat(filepath.Join(dir, "site.json")); err != nil {
		return nil, ErrNotFound
	}
	st := s.readSite(dir, slug, false)
	if st.Error != "" {
		return st, fmt.Errorf("%w: %s", ErrInvalid, st.Error)
	}
	return st, nil
}

// CreateSite makes the folder, its post folders and site.json.
func (s *Store) CreateSite(slug, name, about, template string) (*Site, error) {
	if err := ValidSiteSlug(slug); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	template = strings.TrimSpace(template)
	if template == "" {
		template = "plain"
	}
	if err := ValidSlug(template); err != nil {
		return nil, fmt.Errorf("%w: template %v", ErrInvalid, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, archived := range []bool{false, true} {
		if _, err := os.Stat(s.siteDir(slug, archived)); err == nil {
			return nil, ErrExists
		}
	}
	dir := s.siteDir(slug, false)
	for _, kind := range Kinds {
		if err := os.MkdirAll(filepath.Join(dir, kindDirs[kind]), 0o755); err != nil {
			return nil, err
		}
	}
	file := SiteFile{ID: newID(), Name: name, About: strings.TrimSpace(about), Template: template, Created: s.now()}
	if err := writeSite(dir, &file); err != nil {
		return nil, err
	}
	return s.readSite(dir, slug, false), nil
}

// UpdateSite applies the given fields to site.json.
func (s *Store) UpdateSite(slug string, u SiteUpdate) (*Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Site(slug)
	if err != nil {
		return nil, err
	}
	f := &st.SiteFile
	if u.Name != nil {
		if strings.TrimSpace(*u.Name) == "" {
			return nil, fmt.Errorf("%w: name is required", ErrInvalid)
		}
		f.Name = strings.TrimSpace(*u.Name)
	}
	if u.About != nil {
		f.About = strings.TrimSpace(*u.About)
	}
	if u.Template != nil {
		t := strings.TrimSpace(*u.Template)
		if err := ValidSlug(t); err != nil {
			return nil, fmt.Errorf("%w: template %v", ErrInvalid, err)
		}
		f.Template = t
	}
	if u.TemplateSettings != nil {
		f.TemplateSettings = *u.TemplateSettings
	}
	if u.IPNS != nil {
		f.IPNS = strings.TrimSpace(*u.IPNS)
	}
	if u.Author != nil {
		f.Author = strings.TrimSpace(*u.Author)
	}
	if u.Tags != nil {
		f.Tags = *u.Tags
	}
	if u.Social != nil {
		f.Social = *u.Social
	}
	if u.Plausible != nil {
		f.Plausible = *u.Plausible
	}
	if u.NoIndex != nil {
		f.NoIndex = *u.NoIndex
	}
	if err := writeSite(st.Dir, f); err != nil {
		return nil, err
	}
	return s.readSite(st.Dir, slug, false), nil
}

// DeleteSite removes the site's folder and everything in it.
func (s *Store) DeleteSite(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ValidSiteSlug(slug); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	dir := s.siteDir(slug, false)
	if _, err := os.Stat(filepath.Join(dir, "site.json")); err != nil {
		return ErrNotFound
	}
	return os.RemoveAll(dir)
}

// ArchiveSite moves a live site into Archived/.
func (s *Store) ArchiveSite(slug string) error {
	return s.moveSite(slug, false, true)
}

// UnarchiveSite moves an archived site back beside the live ones.
func (s *Store) UnarchiveSite(slug string) error {
	return s.moveSite(slug, true, false)
}

func (s *Store) moveSite(slug string, fromArchived, toArchived bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ValidSiteSlug(slug); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	from, to := s.siteDir(slug, fromArchived), s.siteDir(slug, toArchived)
	if _, err := os.Stat(filepath.Join(from, "site.json")); err != nil {
		return ErrNotFound
	}
	if _, err := os.Stat(to); err == nil {
		return ErrExists
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.Rename(from, to)
}

// WriteSiteFile writes a site.json into a site folder; the importer uses
// it to fill in what Planet knew.
func WriteSiteFile(dir string, f *SiteFile) error { return writeSite(dir, f) }

// writeSite writes site.json for people as much as for the daemon: two
// space indent, and no \u0026 for an ampersand in a site's name.
func writeSite(dir string, f *SiteFile) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "site.json"), b.Bytes(), 0o644)
}

// liveSite is a site the post methods may act on: present and readable.
func (s *Store) liveSite(slug string) (*Site, error) {
	st, err := s.Site(slug)
	if err != nil {
		return nil, err
	}
	return st, nil
}

func kindDir(kind string) (string, error) {
	d, ok := kindDirs[kind]
	if !ok {
		return "", fmt.Errorf("%w: kind must be post, page or draft", ErrInvalid)
	}
	return d, nil
}

// Posts lists a site's posts of one kind, or of every kind when kind is
// empty, newest first. Bodies are left out of listings.
func (s *Store) Posts(site, kind string) ([]*Post, error) {
	st, err := s.liveSite(site)
	if err != nil {
		return nil, err
	}
	kinds := Kinds
	if kind != "" {
		if _, err := kindDir(kind); err != nil {
			return nil, err
		}
		kinds = []string{kind}
	}
	var posts []*Post
	for _, k := range kinds {
		ents, err := os.ReadDir(filepath.Join(st.Dir, kindDirs[k]))
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dir := filepath.Join(st.Dir, kindDirs[k], e.Name())
			if _, err := os.Stat(filepath.Join(dir, "index.md")); err != nil {
				continue
			}
			p := readPost(dir, k, e.Name())
			p.Body = ""
			posts = append(posts, p)
		}
	}
	sort.Slice(posts, func(i, j int) bool {
		if !posts[i].Created.Equal(posts[j].Created) {
			return posts[i].Created.After(posts[j].Created)
		}
		return posts[i].Slug < posts[j].Slug
	})
	return posts, nil
}

func readPost(dir, kind, slug string) *Post {
	p := &Post{Kind: kind, Slug: slug, Dir: dir, Attachments: listAttachments(dir)}
	if err := ValidSlug(slug); err != nil {
		p.Error = "folder name: " + err.Error()
		return p
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		p.Error = err.Error()
		return p
	}
	m, body, err := ParseDoc(b)
	if err != nil {
		p.Error = "index.md: " + err.Error()
		return p
	}
	p.Meta, p.Body = m, body
	return p
}

func listAttachments(dir string) []string {
	names := []string{}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return names
	}
	for _, e := range ents {
		n := e.Name()
		if e.Type().IsRegular() && n != "index.md" && !strings.HasPrefix(n, ".") && !strings.HasPrefix(n, "_") {
			names = append(names, n)
		}
	}
	return names
}

// Post finds one post by slug, in the given kind or in any.
func (s *Store) Post(site, slug, kind string) (*Post, error) {
	st, err := s.liveSite(site)
	if err != nil {
		return nil, err
	}
	return s.findPost(st, slug, kind)
}

func (s *Store) findPost(st *Site, slug, kind string) (*Post, error) {
	if err := ValidSlug(slug); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	kinds := Kinds
	if kind != "" {
		if _, err := kindDir(kind); err != nil {
			return nil, err
		}
		kinds = []string{kind}
	}
	for _, k := range kinds {
		dir := filepath.Join(st.Dir, kindDirs[k], slug)
		if _, err := os.Stat(filepath.Join(dir, "index.md")); err == nil {
			p := readPost(dir, k, slug)
			if p.Error != "" {
				return p, fmt.Errorf("%w: %s", ErrInvalid, p.Error)
			}
			return p, nil
		}
	}
	return nil, ErrNotFound
}

// CreatePost makes a post folder with its index.md. The slug must be free
// across posts, pages and drafts, since every kind shares one URL space.
func (s *Store) CreatePost(site, kind, slug string, m Meta, body string) (*Post, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.liveSite(site)
	if err != nil {
		return nil, err
	}
	if err := ValidSlug(slug); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	kd, err := kindDir(kind)
	if err != nil {
		return nil, err
	}
	if _, err := s.findPost(st, slug, ""); err == nil {
		return nil, ErrExists
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	// A title may be empty: Planet allows it, and the Memories template
	// lays out a post without one.
	m.Title = strings.TrimSpace(m.Title)
	if m.ID == "" {
		m.ID = newID()
	}
	if m.Created.IsZero() {
		m.Created = s.now()
	}
	dir := filepath.Join(st.Dir, kd, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.md"), FormatDoc(m, body), 0o644); err != nil {
		return nil, err
	}
	return readPost(dir, kind, slug), nil
}

// UpdatePost applies the given fields and stamps modified.
func (s *Store) UpdatePost(site, slug, kind string, u PostUpdate) (*Post, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Post(site, slug, kind)
	if err != nil {
		return nil, err
	}
	if u.Title != nil {
		p.Title = strings.TrimSpace(*u.Title)
	}
	if u.Body != nil {
		p.Body = *u.Body
	}
	if u.Tags != nil {
		p.Tags = splitTags(strings.Join(*u.Tags, ","))
	}
	if u.Hero != nil {
		p.Hero = strings.TrimSpace(*u.Hero)
	}
	if u.Link != nil {
		p.Link = strings.TrimSpace(*u.Link)
	}
	if u.Created != nil && !u.Created.IsZero() {
		p.Created = u.Created.Truncate(time.Second)
	}
	p.Modified = s.now()
	if err := os.WriteFile(filepath.Join(p.Dir, "index.md"), FormatDoc(p.Meta, p.Body), 0o644); err != nil {
		return nil, err
	}
	return readPost(p.Dir, p.Kind, p.Slug), nil
}

// MovePost moves a post between kinds: drafts to posts publishes it.
func (s *Store) MovePost(site, slug, fromKind, toKind string) (*Post, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.liveSite(site)
	if err != nil {
		return nil, err
	}
	p, err := s.findPost(st, slug, fromKind)
	if err != nil {
		return nil, err
	}
	kd, err := kindDir(toKind)
	if err != nil {
		return nil, err
	}
	if p.Kind == toKind {
		return p, nil
	}
	to := filepath.Join(st.Dir, kd, slug)
	if _, err := os.Stat(to); err == nil {
		return nil, ErrExists
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return nil, err
	}
	if err := os.Rename(p.Dir, to); err != nil {
		return nil, err
	}
	return readPost(to, toKind, slug), nil
}

// DeletePost removes a post's folder with its attachments.
func (s *Store) DeletePost(site, slug, kind string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Post(site, slug, kind)
	if err != nil && p == nil {
		return err
	}
	return os.RemoveAll(p.Dir)
}

// AttachmentName checks a file name a client sends: the base name only,
// nothing hidden or generated, not the post itself.
func AttachmentName(name string) (string, error) {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if name == "" || name == "." || name == "/" || name == "index.md" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: invalid attachment name", ErrInvalid)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: invalid attachment name", ErrInvalid)
		}
	}
	return name, nil
}

// AddAttachment writes a file into the post's folder, replacing one of
// the same name.
func (s *Store) AddAttachment(site, slug, kind, name string, r io.Reader) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Post(site, slug, kind)
	if err != nil {
		return "", err
	}
	name, err = AttachmentName(name)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(filepath.Join(p.Dir, name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return "", err
	}
	return name, f.Close()
}

// AttachmentPath is the file behind an attachment name, for serving.
func (s *Store) AttachmentPath(site, slug, kind, name string) (string, error) {
	p, err := s.Post(site, slug, kind)
	if err != nil {
		return "", err
	}
	name, err = AttachmentName(name)
	if err != nil {
		return "", err
	}
	fp := filepath.Join(p.Dir, name)
	if fi, err := os.Stat(fp); err != nil || !fi.Mode().IsRegular() {
		return "", ErrNotFound
	}
	return fp, nil
}

// DeleteAttachment removes one attachment.
func (s *Store) DeleteAttachment(site, slug, kind, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fp, err := s.AttachmentPath(site, slug, kind, name)
	if err != nil {
		return err
	}
	return os.Remove(fp)
}

// Templates lists the template folders with a template.json.
func (s *Store) Templates() ([]*Template, error) {
	ents, err := os.ReadDir(filepath.Join(s.Root, TemplatesDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Template
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Root, TemplatesDir, e.Name(), "template.json"))
		if err != nil {
			continue
		}
		t := &Template{Slug: e.Name()}
		if err := json.Unmarshal(b, t); err != nil {
			t.Error = "template.json: " + err.Error()
		}
		t.Slug = e.Name()
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// newID is a random UUID in the upper-case form Planet writes.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]))
}
