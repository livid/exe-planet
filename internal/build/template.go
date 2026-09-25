// Package build renders a site the way Planet's savePublic does: the
// same files in the same places with the same bytes, checked against
// sites Planet itself built.
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Setting is one entry of template.json's settings.
type Setting struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	DefaultValue string `json:"defaultValue"`
	Description  string `json:"description"`
	Advanced     *bool  `json:"advanced"`
}

// Template is a template folder: template.json and what is in templates/
// and assets/.
type Template struct {
	Dir                     string
	Name                    string             `json:"name"`
	Description             string             `json:"description"`
	Author                  string             `json:"author"`
	Version                 string             `json:"version"`
	IdealItemsPerPage       *int               `json:"idealItemsPerPage"`
	GenerateIndexPagination *bool              `json:"generateIndexPagination"`
	GenerateTagPages        *bool              `json:"generateTagPages"`
	GenerateArchive         *bool              `json:"generateArchive"`
	BuildNumber             *int               `json:"buildNumber"`
	GenerateNFTMetadata     *bool              `json:"generateNFTMetadata"`
	Settings                map[string]Setting `json:"settings"`

	HasSimple  bool
	HasTags    bool
	HasArchive bool
	HasIndex   bool
	// StyleCSSHash is the sha256 of assets/style.css, "" without one.
	StyleCSSHash string
}

// LoadTemplate reads a template folder. blog.html is required, as in
// Planet.
func LoadTemplate(dir string) (*Template, error) {
	b, err := os.ReadFile(filepath.Join(dir, "template.json"))
	if err != nil {
		return nil, err
	}
	t := &Template{Dir: dir}
	if err := json.Unmarshal(b, t); err != nil {
		return nil, fmt.Errorf("%s: template.json: %w", dir, err)
	}
	if _, err := os.Stat(t.file("blog.html")); err != nil {
		return nil, fmt.Errorf("%s: no templates/blog.html", dir)
	}
	exists := func(name string) bool { _, err := os.Stat(t.file(name)); return err == nil }
	t.HasSimple, t.HasTags, t.HasArchive, t.HasIndex = exists("simple.html"), exists("tags.html"), exists("archive.html"), exists("index.html")
	if css, err := os.ReadFile(filepath.Join(dir, "assets", "style.css")); err == nil {
		sum := sha256.Sum256(css)
		t.StyleCSSHash = hex.EncodeToString(sum[:])
	}
	return t, nil
}

func (t *Template) file(name string) string { return filepath.Join(t.Dir, "templates", name) }

// TemplatesDir is templates/ as an absolute path.
func (t *Template) TemplatesDir() string { return filepath.Join(t.Dir, "templates") }

// ItemsPerPage is idealItemsPerPage with Planet's default.
func (t *Template) ItemsPerPage() int {
	if t.IdealItemsPerPage == nil || *t.IdealItemsPerPage <= 0 {
		return 10
	}
	return *t.IdealItemsPerPage
}

func flag(b *bool) bool { return b != nil && *b }

// DefaultSettings maps each setting to its default value.
func (t *Template) DefaultSettings() map[string]string {
	out := map[string]string{}
	for k, s := range t.Settings {
		out[k] = s.DefaultValue
	}
	return out
}

// SettingsContext is template_settings as the templates see it.
func (t *Template) SettingsContext() map[string]any {
	out := map[string]any{}
	for k, s := range t.Settings {
		adv := false
		if s.Advanced != nil {
			adv = *s.Advanced
		}
		out[k] = map[string]any{"name": s.Name, "type": s.Type, "defaultValue": s.DefaultValue, "description": s.Description, "advanced": adv}
	}
	return out
}

// FindTemplate locates a template folder by the name in its template.json
// ("8-bit" lives in gamedb/), falling back to the lower-cased name as a
// folder.
func FindTemplate(templatesDir, name string) (*Template, error) {
	ents, err := os.ReadDir(templatesDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		t, err := LoadTemplate(filepath.Join(templatesDir, e.Name()))
		if err != nil {
			continue
		}
		if t.Name == name || strings.EqualFold(e.Name(), name) {
			return t, nil
		}
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return nil, fmt.Errorf("no template named %q in %s (have %s)", name, templatesDir, strings.Join(names, ", "))
}
