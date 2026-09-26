package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/planet"
)

// Goldens are real Planet builds fetched from Livid's Mac by `make
// goldens` into testdata/goldens/<name>/{My,Public}; without them the
// test skips. Each one is rebuilt here from its My/ folder and every
// file compared. Planet leaves files of earlier templates behind in
// Public/, so a golden file this build does not write is only logged.

var (
	timestamps   = regexp.MustCompile(`\b1[0-9]{9}\b`)
	spaces       = regexp.MustCompile(`\s+`)
	betweenTags  = regexp.MustCompile(`>\s+<`)
	styleHashes  = regexp.MustCompile(`sha256=[0-9a-f]{64}`)
	filterChains = regexp.MustCompile(`invert\([0-9.]+%\) sepia\([0-9.]+%\) saturate\([0-9.]+%\) hue-rotate\([0-9.]+deg\) brightness\([0-9.]+%\) contrast\([0-9.]+%\)`)
)

var binaryExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".ico": true, ".mp4": true, ".mov": true, ".m4a": true, ".mp3": true, ".pdf": true, ".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".heic": true, ".avif": true}

func normalise(rel string, data []byte) string {
	s := timestamps.ReplaceAllString(string(data), "TS")
	// A page rendered before the template's stylesheet last changed
	// carries the older hash; Planet re-renders pages only on a rebuild.
	s = styleHashes.ReplaceAllString(s, "sha256=HASH")
	// Planet's colour-filter solver is stochastic: two of its own builds
	// give two chains for one colour, so the chain is not compared.
	s = filterChains.ReplaceAllString(s, "FILTER")
	if rel == "rss.xml" || rel == "podcast.xml" {
		// SwiftSoup re-indents the descriptions; whitespace between tags
		// and runs of it inside text carry nothing a reader keeps.
		s = betweenTags.ReplaceAllString(s, "><")
		s = spaces.ReplaceAllString(s, " ")
	}
	return s
}

func TestGoldens(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "goldens")
	ents, err := os.ReadDir(root)
	if err != nil || len(ents) == 0 {
		t.Skip("no goldens fetched; run make goldens")
	}
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			my := filepath.Join(root, name, "My")
			pub := filepath.Join(root, name, "Public")
			// A stale.txt in the golden says its pages were rendered with a
			// template the Mac no longer has; it is skipped until the planet
			// is rebuilt there and fetched again.
			if note, err := os.ReadFile(filepath.Join(root, name, "stale.txt")); err == nil {
				t.Skip("stale golden: " + strings.TrimSpace(string(note)))
			}
			lib, err := planet.ReadLibrary(my)
			if err != nil {
				t.Fatal(err)
			}
			tpl, err := FindTemplate(filepath.Join("..", "..", "templates"), lib.Planet.TemplateName)
			if err != nil {
				t.Fatal(err)
			}
			in := Input{
				Planet:   lib.Planet,
				Articles: lib.Articles,
				Template: tpl,
				Settings: lib.TemplateSettings,
				Attachments: func(a *planet.MyArticle) string {
					return filepath.Join(pub, a.ID)
				},
				Now:      time.Date(2026, 9, 25, 12, 0, 0, 0, la),
				Location: la,
				Out:      t.TempDir(),
			}
			// EXE_PLANET_GOLDEN_OUT=<dir> keeps each build under <dir>/<name>
			// for looking at by hand.
			if keep := os.Getenv("EXE_PLANET_GOLDEN_OUT"); keep != "" {
				in.Out = filepath.Join(keep, name)
				os.RemoveAll(in.Out)
			}
			if lib.Filters != nil && lib.Filters.TemplateName == lib.Planet.TemplateName {
				in.Filters = lib.Filters.Values
			}
			// The Mac's template may have moved on since the site was built;
			// its assets at build time are what Public/assets holds.
			if fi, err := os.Stat(filepath.Join(pub, "assets")); err == nil && fi.IsDir() {
				in.AssetsDir = filepath.Join(pub, "assets")
			}
			if lib.HasAvatar {
				in.AvatarPath = filepath.Join(my, "avatar.png")
			}
			if lib.HasFavicon {
				in.FaviconPath = filepath.Join(my, "favicon.ico")
			}
			res, err := Build(in)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range res.Warnings {
				t.Log("warning:", w)
			}
			compare(t, in.Out, pub)
		})
	}
}

// compare checks every built file against the golden, and that the files
// a Planet follower depends on were built at all.
func compare(t *testing.T, out, golden string) {
	t.Helper()
	// Planet rewrites planet.json on every build but an article.json only
	// when that article is saved, so an old article.json lacks fields added
	// since. The article's entry in planet.json is the fresh projection and
	// is what article.json is compared with.
	var pl struct {
		Articles []map[string]any `json:"articles"`
	}
	if b, err := os.ReadFile(filepath.Join(golden, "planet.json")); err == nil {
		json.Unmarshal(b, &pl)
	}
	entries := map[string][]byte{}
	for _, a := range pl.Articles {
		if id, ok := a["id"].(string); ok {
			entries[id] = planet.MarshalFoundation(a)
			if slug, ok := a["slug"].(string); ok && slug != "" {
				entries[slug] = entries[id] // the slug copy of the folder
			}
		}
	}
	built := map[string]bool{}
	var mismatched []string
	shown := map[string]int{}
	filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(out, path)
		rel = filepath.ToSlash(rel)
		built[rel] = true
		want, err := os.ReadFile(filepath.Join(golden, rel))
		if entry, ok := entries[filepath.ToSlash(filepath.Dir(rel))]; ok && filepath.Base(rel) == "article.json" {
			want, err = entry, nil
		}
		if err != nil {
			t.Errorf("built %s, golden has no such file", rel)
			return nil
		}
		if binaryExt[strings.ToLower(filepath.Ext(rel))] {
			return nil
		}
		got, _ := os.ReadFile(path)
		g, w := normalise(rel, got), normalise(rel, want)
		if g != w {
			mismatched = append(mismatched, rel)
			kind := filepath.Base(rel)
			if shown[kind] < 2 {
				shown[kind]++
				t.Errorf("%s differs:\n%s", rel, firstDiff(g, w))
			}
		}
		return nil
	})
	if len(mismatched) > 0 {
		list := mismatched
		if len(list) > 30 {
			list = append(list[:30], "…")
		}
		t.Errorf("%d files differ: %s", len(mismatched), strings.Join(list, " "))
	}
	required := []string{"index.html", "planet.json", "rss.xml", "robots.txt"}
	for id := range entries {
		required = append(required, id+"/index.html", id+"/article.json", id+"/article.md")
	}
	for _, r := range required {
		if !built[r] {
			t.Errorf("required file not built: %s", r)
		}
	}
	var unbuilt []string
	filepath.WalkDir(golden, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(golden, path)
		rel = filepath.ToSlash(rel)
		if !built[rel] && !strings.Contains(rel, "/_grid.") {
			unbuilt = append(unbuilt, rel)
		}
		return nil
	})
	if len(unbuilt) > 0 {
		if len(unbuilt) > 12 {
			unbuilt = append(unbuilt[:12], "…")
		}
		t.Logf("golden files this build did not write (stale in Planet, or not yet built here): %s", strings.Join(unbuilt, ", "))
	}
}

// firstDiff shows the first differing line with a little context.
func firstDiff(got, want string) string {
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			from := i - 2
			if from < 0 {
				from = 0
			}
			var b strings.Builder
			for j := from; j < i; j++ {
				if j < len(wl) {
					b.WriteString("  " + wl[j] + "\n")
				}
			}
			b.WriteString("- " + cut(w) + "\n+ " + cut(g))
			return b.String() + "\n  (line " + itoa(i+1) + ")"
		}
	}
	return "(files differ only in length)"
}

func cut(s string) string {
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

func itoa(i int) string { return strconv.Itoa(i) }
