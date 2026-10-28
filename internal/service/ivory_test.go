package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/site"
)

// TestIvoryTemplate builds a site with the Ivory template through every
// page kind it has: the About over the first page of posts, a second
// page, a post with tags, pages in the navigation, the archive, the tag
// list and a tag page; and with its one setting, Hide, the About alone.
func TestIvoryTemplate(t *testing.T) {
	svc, store := newService(t)
	about := "Planet is a free and open-source macOS app.\n\n<figure>\n  <img src=\"assets/shot.png\" alt=\"Shot\" class=\"screenshot\" />\n  <figcaption>A shot</figcaption>\n</figure>\n\n## Open Source\n\nMIT.\n"
	if _, err := store.CreateSite("planetable", "Planet", about, "ivory"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Site("planetable")
	os.WriteFile(filepath.Join(st.Dir, "avatar.png"), []byte("png"), 0o644)
	day := time.Date(2022, 9, 27, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 12; i++ {
		m := site.Meta{Title: "Guide " + string(rune('A'+i-1)), Created: day.AddDate(0, 0, i)}
		if i == 12 {
			m.Tags = []string{"ENS"}
		}
		if _, err := store.CreatePost("planetable", site.KindPost, "guide-"+strings.ToLower(string(rune('A'+i-1))), m, "## One\n\nText.\n\n## Two\n\nMore.\n"); err != nil {
			t.Fatal(err)
		}
	}
	store.CreatePost("planetable", site.KindPage, "guides", site.Meta{Title: "Guides", Created: day, Extra: map[string]string{"weight": "1"}}, "## [Guide A](../guide-a/)\n\nThe first.\n")
	store.CreatePost("planetable", site.KindPage, "faq", site.Meta{Title: "FAQ", Created: day, Extra: map[string]string{"weight": "2"}}, "## Why?\n\nBecause.\n")
	if _, err := svc.Build("planetable"); err != nil {
		t.Fatal(err)
	}
	pub := svc.PublicDir(st.ID)
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(pub, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		return string(b)
	}
	has := func(rel, page string, wants ...string) {
		t.Helper()
		for _, w := range wants {
			if !strings.Contains(page, w) {
				t.Errorf("%s lacks %q", rel, w)
			}
		}
	}
	hasNot := func(rel, page string, nots ...string) {
		t.Helper()
		for _, w := range nots {
			if strings.Contains(page, w) {
				t.Errorf("%s has %q", rel, w)
			}
		}
	}
	index := read("index.html")
	has("index.html", index,
		`<title>Planet</title>`, `<meta name="theme-color" content="#fffff0">`, `assets/style.css?sha256=`,
		`<a class="logo" href="./"><img src="./avatar.png" alt=""></a>`, `<a class="site-name" href="./">Planet</a>`,
		`<a href="./" class="nav-item active">Home</a>`,
		`<a href="./guides/" class="nav-item">Guides</a>`, `<a href="./faq/" class="nav-item">FAQ</a>`,
		`<div class="about"><p>Planet is a free and open-source macOS app.</p>`, `<img src="assets/shot.png" alt="Shot" class="screenshot" />`,
		`<h2>Posts</h2>`, `<li><a href="./guide-l/">Guide L</a> <span class="date">Oct 9, 2022</span></li>`,
		`<div class="pager"><span></span><a href="./page2.html">Older</a></div>`,
		`<div class="footer"><a href="./archive.html">Archive</a> · <a href="./tags.html">Tags</a> · <a href="./rss.xml">RSS</a></div>`)
	hasNot("index.html", index, `>Guides</a> <span class="date">`)
	page2 := read("page2.html")
	has("page2.html", page2, `<h1>Posts</h1>`, `<a href="./page1.html">Newer</a>`, `<a href="./" class="nav-item">Home</a>`)
	hasNot("page2.html", page2, `class="about"`)
	post := read("guide-l/index.html")
	has("guide-l/index.html", post, `<title>Guide L</title>`, `href="../assets/style.css?sha256=`,
		`<h1>Guide L</h1>`, `<p class="page-info">Oct 9, 2022</p>`, `<div class="content">`, `<h2>One</h2>`,
		`<a href="../ens.html">ENS</a>`, `<button class="scroll-top"`, `Table of Contents`,
		`<a href="../" class="nav-item">Home</a>`, `<a href="../guides/" class="nav-item">Guides</a>`)
	page := read("guides/index.html")
	has("guides/index.html", page, `<h1>Guides</h1>`, `<a href="../guides/" class="nav-item active">Guides</a>`, `<h2><a href="../guide-a/">Guide A</a></h2>`)
	hasNot("guides/index.html", page, `page-info`)
	has("archive.html", read("archive.html"), `<title>Archive · Planet</title>`, `<h1>Archive</h1>`, `<a href="./guide-a/">Guide A</a>`)
	hasNot("archive.html", read("archive.html"), `<a href="./guides/">Guides</a> <span`)
	has("tags.html", read("tags.html"), `<a href="./ens.html">ENS</a> <span class="date">1</span>`)
	has("ens.html", read("ens.html"), `<title>ENS · Planet</title>`, `<h1>ENS</h1>`, `<a href="./guide-l/">Guide L</a>`)

	// Hide: the About alone on the home page
	hide := map[string]string{"homePosts": "Hide"}
	if _, err := store.UpdateSite("planetable", site.SiteUpdate{TemplateSettings: &hide}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Build("planetable"); err != nil {
		t.Fatal(err)
	}
	index = read("index.html")
	has("index.html (Hide)", index, `<div class="about">`)
	hasNot("index.html (Hide)", index, `<h2>Posts</h2>`, `class="posts"`, `>Older</a>`)
}
