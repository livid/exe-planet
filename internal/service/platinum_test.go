package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/site"
)

// TestPlatinumTemplate builds a site with the Platinum template, ours,
// through every page kind it has: a paginated index, a post with tags and
// a picture, a page in the navigation, an untitled post, the archive, the
// tag list and a tag page.
func TestPlatinumTemplate(t *testing.T) {
	svc, store := newService(t)
	if _, err := store.CreateSite("exe", "exe", "A personal VM cloud with a Mac OS 9 face.", "platinum"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Site("exe")
	os.WriteFile(filepath.Join(st.Dir, "avatar.png"), []byte("png"), 0o644)
	day := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	for i := 1; i <= 12; i++ {
		m := site.Meta{Title: "Post " + string(rune('A'+i-1)), Created: day.AddDate(0, 0, i)}
		if i == 12 {
			m.Tags = []string{"exe", "Desk Things"}
		}
		body := "Body of post " + string(rune('A'+i-1)) + ".\n"
		if i == 12 {
			body = "# Hello\n\nA *first* line with `code`.\n\n```sh\n$ exe serve\n```\n\n![shot](shot.png)\n\n- one\n- two\n\nThen:\n\n- [x] done\n- [ ] to do\n"
		}
		if _, err := store.CreatePost("exe", site.KindPost, "post-"+strings.ToLower(string(rune('A'+i-1))), m, body); err != nil {
			t.Fatal(err)
		}
	}
	store.AddAttachment("exe", "post-l", "", "shot.png", strings.NewReader("png"))
	store.CreatePost("exe", site.KindPost, "quiet", site.Meta{Created: day.AddDate(0, 0, 20)}, "Just a thought.\n")
	store.CreatePost("exe", site.KindPage, "about", site.Meta{Title: "About", Created: day}, "About this site.\n")
	store.CreatePost("exe", site.KindPage, "badge", site.Meta{Title: "Badge", Created: day}, "The badge.\n")
	if _, err := svc.Build("exe"); err != nil {
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
	index := read("index.html")
	has("index.html", index,
		`<div class="title">exe</div>`, `assets/chrome.css`, `assets/style.css?sha256=`,
		`<img src="./avatar.png" alt="" width="32" height="32" class="avatar">`,
		`<h1>exe<small><p>A personal VM cloud with a Mac OS 9 face.</p>`,
		`class="item-title" href="./quiet/">Sep 21, 2026</a>`, `<div class="item-body content"><p>Just a thought.</p>`,
		`href="./post-l/">Post L</a>`, `<span class="item-date">Sep 13, 2026</span>`,
		`10 posts · page 1 of 2</span>`, `<a href="./page2.html">Older</a>`,
		`<a class="btn" href="./">Home</a>`,
		`<a class="btn glyph" href="./archive.html" title="Archive" aria-label="Archive"><span class="t">Archive</span><svg class="ix"`,
		`<a class="btn glyph" href="./tags.html" title="Tags" aria-label="Tags"><span class="t">Tags</span><svg class="ix"`,
		`<a class="btn" href="./about/" title="About" aria-label="About"><span class="t">About</span></a>`,
		`<a class="btn glyph" href="./badge/" title="Badge" aria-label="Badge"><span class="t">Badge</span><svg class="ix"`,
		`<a class="btn glyph" href="./rss.xml" title="RSS" aria-label="RSS"><span class="t">RSS</span><svg class="ix"`,
		`Built with <a href="https://exe.v2core.com/"`)
	if strings.Contains(index, `class="strip tools"`) || strings.Contains(index, `btn default`) {
		t.Error("the index wears a crumb strip or a ringed button")
	}
	page2 := read("page2.html")
	has("page2.html", page2, `page 2 of 2`, `<a href="./page1.html">Newer</a>`, `href="./post-a/">Post A</a>`)
	post := read("post-l/index.html")
	// a to-do list: the checkboxes as the Markdown writes them, the script
	// that numbers the items for the app showing the page, and the
	// stylesheet's own box in the checkbox's place
	has("post-l/index.html", post, `<li><input type="checkbox" checked="" disabled="" /> done</li>`, `<li><input type="checkbox" disabled="" /> to do</li>`,
		`li.id = "todo-item-" + (++n);`, `if (li.parentElement.matches(".content > ul"))`)
	has("assets/style.css", read("assets/style.css"), `@supports selector(li:has(> input)) {`,
		`.content li[id^="todo-item-"] { pointer-events: none; }`, `.content li[id^="todo-item-"] > *, .content li[id^="todo-item-"]::before { pointer-events: auto; }`,
		`box-shadow: inset 1px 1px 0 var(--g500), inset -1px -1px 0 #fff; }`, `shape-rendering="crispEdges"`,
		`.content li > input[type=checkbox]:checked + .todo-words, .content li > input[type=checkbox]:checked + p { color: #777; text-decoration: line-through; }`)
	if strings.Contains(read("index.html"), "todo-item-") {
		t.Error("the index numbers to-do items: only a post's own page does")
	}
	has("post-l/index.html", post,
		`<div class="title">Post L</div>`, `<a href="../">exe</a></span><svg class="sep"`, `<b>Post L</b>`,
		`<h1 class="post-title">Post L</h1>`, `<div class="date">Sep 13, 2026`,
		`<h1>Hello</h1>`, `<code>code</code>`, `<pre><code class="language-sh">$ exe serve`, `<img src="shot.png" alt="shot"`,
		`<a href="../exe.html" class="tag">exe</a>`, `<a href="../desk-things.html" class="tag">Desk Things</a>`,
		`<a class="btn default" href="../">Home</a>`, `<span><a href="../">exe</a></span>`, `<svg class="ix" viewBox="0 0 `,
		`<meta property="og:type" content="article" />`)
	about := read("about/index.html")
	has("about/index.html", about, `<div class="title">About</div>`, `<h1 class="post-title">About</h1>`)
	if strings.Contains(about, `<div class="date">`) {
		t.Error("a page shows a date")
	}
	archive := read("archive.html")
	has("archive.html", archive, `<div class="title">exe — Archive</div>`, `<h2 class="section">September 2026</h2>`, `href="./post-l/">Post L</a>`, `15 posts in 1 month</span>`)
	tags := read("tags.html")
	has("tags.html", tags, `<div class="title">exe — Tags</div>`, `href="./desk-things.html">Desk Things</a>`, `1 post</span>`, `2 tags</span>`)
	tag := read("exe.html")
	has("exe.html", tag, `<div class="title">exe — exe</div>`, `<b>exe</b></div></div>`, `<h1>exe<small>1 post tagged</small></h1>`, `href="./post-l/">Post L</a>`)
	rss := read("rss.xml")
	if !strings.HasPrefix(rss, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<?xml-stylesheet type=\"text/xsl\" href=\"assets/rss.xsl\"?>\n<rss ") {
		t.Errorf("rss.xml has no stylesheet line:\n%.200s", rss)
	}
	// every link a page makes to the site itself is relative, so the site
	// works at any root and the Planet window can follow it
	abs := regexp.MustCompile(`(?:href|src)="/`)
	for rel, page := range map[string]string{"index.html": index, "page2.html": page2, "post-l/index.html": post, "about/index.html": about, "archive.html": archive, "tags.html": tags, "exe.html": tag} {
		if m := abs.FindString(page); m != "" {
			t.Errorf("%s has a root-relative link: %s", rel, m)
		}
	}
	for _, f := range []string{"assets/chrome.css", "assets/style.css", "assets/rss.xsl", "rss.xml", "planet.json", "avatar.png"} {
		if _, err := os.Stat(filepath.Join(pub, filepath.FromSlash(f))); err != nil {
			t.Errorf("no %s in the build", f)
		}
	}
	if strings.Contains(read("assets/chrome.css"), "{{") {
		t.Error("chrome.css went through the template engine")
	}
}
