// Package importer brings a planet out of a Planet library folder into
// the Workspace: My/<uuid> for the text and settings, Public/<uuid> for
// the attachments, which Planet keeps only in the built site. Nothing is
// read from the library that a follower could not already see, except
// the planet's own settings; its API tokens stay behind.
package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/livid/exe-planet/internal/build"
	"github.com/livid/exe-planet/internal/planet"
	"github.com/livid/exe-planet/internal/site"
)

// Entry describes one planet in a library, for a picker.
type Entry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Template string `json:"template"`
	Articles int    `json:"articles"`
	Archived bool   `json:"archived"`
	Public   bool   `json:"public"` // Public/<uuid> exists, so attachments can come along
	Error    string `json:"error,omitempty"`
}

// List reads a library's My/ folder.
func List(library string) ([]Entry, error) {
	my := filepath.Join(library, "My")
	ents, err := os.ReadDir(my)
	if err != nil {
		return nil, fmt.Errorf("%w: %s has no My folder", site.ErrInvalid, library)
	}
	var out []Entry
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		entry := Entry{ID: e.Name()}
		lib, err := planet.ReadLibrary(filepath.Join(my, e.Name()))
		if err != nil {
			entry.Error = err.Error()
		} else {
			entry.Name, entry.Template, entry.Articles = lib.Planet.Name, lib.Planet.TemplateName, len(lib.Articles)
			entry.Archived = lib.Planet.Archived != nil && *lib.Planet.Archived
		}
		_, err = os.Stat(filepath.Join(library, "Public", e.Name()))
		entry.Public = err == nil
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Result reports an import.
type Result struct {
	Site        string   `json:"site"`
	ID          string   `json:"id"`
	Posts       int      `json:"posts"`
	Pages       int      `json:"pages"`
	Attachments int      `json:"attachments"`
	Warnings    []string `json:"warnings,omitempty"`
}

// Import creates the site <slug> from <library>/My/<id>. templates is
// the folder the template name is resolved in.
func Import(store *site.Store, templates, library, id, slug string) (*Result, error) {
	if err := site.ValidSiteSlug(slug); err != nil {
		return nil, fmt.Errorf("%w: %v", site.ErrInvalid, err)
	}
	my := filepath.Join(library, "My", id)
	if fi, err := os.Stat(filepath.Join(my, "planet.json")); err != nil || fi.IsDir() {
		return nil, fmt.Errorf("%w: no planet %s in %s", site.ErrNotFound, id, library)
	}
	lib, err := planet.ReadLibrary(my)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", site.ErrInvalid, err)
	}
	pub := filepath.Join(library, "Public", id)
	if _, err := os.Stat(pub); err != nil {
		pub = ""
	}
	res := &Result{Site: slug, ID: lib.Planet.ID}

	template := templateSlug(templates, lib.Planet.TemplateName)
	st, err := store.CreateSite(slug, lib.Planet.Name, lib.Planet.About, template)
	if err != nil {
		return nil, err
	}
	// The site was ours to make; a failure part way leaves nothing behind.
	done := false
	defer func() {
		if !done {
			store.DeleteSite(slug)
		}
	}()
	// The site keeps Planet's id, so a follower sees the same planet.
	f := st.SiteFile
	f.ID = lib.Planet.ID
	f.Created = lib.Planet.Created.Time()
	f.TemplateSettings = lib.TemplateSettings
	f.Author = deref(lib.Planet.AuthorName)
	f.Tags = lib.Planet.Tags
	f.Social = site.Social{
		Twitter:  deref(lib.Planet.TwitterUsername),
		GitHub:   deref(lib.Planet.GithubUsername),
		Telegram: deref(lib.Planet.TelegramUsername),
		Mastodon: deref(lib.Planet.MastodonUsername),
		Discord:  deref(lib.Planet.DiscordLink),
	}
	f.Plausible = site.Plausible{
		Enabled: lib.Planet.PlausibleEnabled != nil && *lib.Planet.PlausibleEnabled,
		Domain:  deref(lib.Planet.PlausibleDomain),
		Server:  deref(lib.Planet.PlausibleAPIServer),
	}
	f.NoIndex = lib.Planet.DoNotIndex != nil && *lib.Planet.DoNotIndex
	if err := site.WriteSiteFile(st.Dir, &f); err != nil {
		return nil, err
	}
	for _, name := range []string{"avatar.png", "favicon.ico"} {
		if err := copyFile(filepath.Join(my, name), filepath.Join(st.Dir, name)); err != nil && !os.IsNotExist(err) {
			res.Warnings = append(res.Warnings, name+": "+err.Error())
		}
	}
	custom := []struct {
		enabled *bool
		code    *string
		file    string
	}{
		{lib.Planet.CustomCodeHeadEnabled, lib.Planet.CustomCodeHead, "head.html"},
		{lib.Planet.CustomCodeBodyStartEnabled, lib.Planet.CustomCodeBodyStart, "body-start.html"},
		{lib.Planet.CustomCodeBodyEndEnabled, lib.Planet.CustomCodeBodyEnd, "body-end.html"},
	}
	for _, c := range custom {
		if c.enabled != nil && *c.enabled && c.code != nil && strings.TrimSpace(*c.code) != "" {
			if err := os.WriteFile(filepath.Join(st.Dir, c.file), []byte(*c.code), 0o644); err != nil {
				return nil, err
			}
		}
	}

	taken := map[string]bool{}
	for _, a := range lib.Articles {
		kind := site.KindPost
		if a.Type() == planet.TypePage {
			kind = site.KindPage
		}
		postSlug := uniqueSlug(a, taken)
		m := site.Meta{
			ID:      a.ID,
			Title:   a.Title,
			Created: a.Created.Time(),
			Tags:    tagValues(a.Tags),
			Link:    deref(a.ExternalLink),
		}
		if a.Modified != nil {
			m.Modified = a.Modified.Time()
		}
		if a.HeroImage != nil && contains(a.Attachments, *a.HeroImage) {
			m.Hero = *a.HeroImage
		}
		if kind == site.KindPage && a.NavigationWeight != nil && *a.NavigationWeight != 1 {
			m.Extra = map[string]string{"weight": fmt.Sprint(*a.NavigationWeight)}
		}
		post, err := store.CreatePost(slug, kind, postSlug, m, a.Content)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", a.Title, err)
		}
		if kind == site.KindPage {
			res.Pages++
		} else {
			res.Posts++
		}
		if pub == "" {
			if len(a.Attachments) > 0 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %d attachments not copied, the library has no Public/%s", a.Title, len(a.Attachments), id))
			}
			continue
		}
		for _, name := range a.Attachments {
			src := filepath.Join(pub, a.ID, name)
			if err := copyFile(src, filepath.Join(post.Dir, name)); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s: attachment %s: %v", a.Title, name, err))
				continue
			}
			res.Attachments++
		}
	}
	done = true
	return res, nil
}

// templateSlug finds the installed template folder for a Planet template
// name ("8-bit" lives in gamedb/), else the lower-cased name.
func templateSlug(templates, name string) string {
	if templates != "" {
		if t, err := build.FindTemplate(templates, name); err == nil {
			return filepath.Base(t.Dir)
		}
	}
	if s := site.Slugify(name); s != "" {
		return s
	}
	return "plain"
}

// uniqueSlug is the article's own slug when it has a valid one, else one
// from its title, else its id lower-cased; a clash gets a counter.
func uniqueSlug(a *planet.MyArticle, taken map[string]bool) string {
	base := ""
	if a.Slug != nil && site.ValidSlug(*a.Slug) == nil {
		base = *a.Slug
	} else if s := site.Slugify(a.Title); s != "" {
		base = s
	} else {
		base = strings.ToLower(a.ID)
	}
	slug := base
	for n := 2; taken[slug]; n++ {
		suffix := fmt.Sprintf("-%d", n)
		if len(base)+len(suffix) > 63 {
			slug = base[:63-len(suffix)] + suffix
		} else {
			slug = base + suffix
		}
	}
	taken[slug] = true
	return slug
}

func tagValues(tags map[string]string) []string {
	if len(tags) == 0 {
		return nil
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		v := tags[k]
		if v == "" {
			v = k
		}
		out = append(out, v)
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return errors.New("not a regular file")
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

// LibraryRoot accepts either a Planet library folder (holding My/) or a
// My/<uuid> folder, and returns the library and the id.
func LibraryRoot(path string) (library, id string, err error) {
	path = filepath.Clean(path)
	if _, err := os.Stat(filepath.Join(path, "My")); err == nil {
		return path, "", nil
	}
	if _, err := os.Stat(filepath.Join(path, "planet.json")); err == nil && filepath.Base(filepath.Dir(path)) == "My" {
		return filepath.Dir(filepath.Dir(path)), filepath.Base(path), nil
	}
	return "", "", fmt.Errorf("%w: %s is neither a Planet library nor a My/<id> folder", site.ErrInvalid, path)
}

var _ = json.Marshal
