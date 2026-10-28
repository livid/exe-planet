package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/site"
)

// TestCloudTemplate builds a site with the Cloud template through every
// page kind it has: the About over the first page of posts, a second
// page, a post with its own author and tags, a page in the sidebar, the
// archive as the Blog, the tag list and a tag page; and the site's
// static/ folder, which reaches the root of the build without replacing
// a file the build writes.
func TestCloudTemplate(t *testing.T) {
	svc, store := newService(t)
	about := "As a developer, you run many processes.\n\n<p><img src=\"screenshots/app.png\" alt=\"App\" class=\"screenshot\"></p>\n\n## Open Source\n\nFree and open source.\n"
	if _, err := store.CreateSite("launcher", "CodeLauncher", about, "cloud"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Site("launcher")
	os.WriteFile(filepath.Join(st.Dir, "avatar.png"), []byte("png"), 0o644)
	for name, text := range map[string]string{
		"static/screenshots/app.png": "png",
		"static/_redirects":          "/blog/* /:splat\n",
		"static/index.html":          "not the index",
		"static/.DS_Store":           "hidden",
	} {
		p := filepath.Join(st.Dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(text), 0o644)
	}
	day := time.Date(2022, 1, 1, 9, 0, 0, 0, time.UTC)
	for i := 1; i <= 12; i++ {
		m := site.Meta{Title: "Post " + string(rune('A'+i-1)), Created: day.AddDate(0, 0, i)}
		if i == 12 {
			m.Tags = []string{"Release"}
			m.Extra = map[string]string{"author": "Xin Liu", "author_link": "https://twitter.com/livid", "author_avatar": "livid.png"}
		}
		if _, err := store.CreatePost("launcher", site.KindPost, "post-"+strings.ToLower(string(rune('A'+i-1))), m, "Body.\n\n```\nDEVELOPMENT_TEAM = X\n```\n"); err != nil {
			t.Fatal(err)
		}
	}
	store.AddAttachment("launcher", "post-l", "", "livid.png", strings.NewReader("png"))
	store.CreatePost("launcher", site.KindPage, "use-cases", site.Meta{Title: "Use Cases", Created: day}, "What it is for.\n")
	status, err := svc.Build("launcher")
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Warnings) != 1 || !strings.Contains(status.Warnings[0], "static/index.html") {
		t.Errorf("warnings %q", status.Warnings)
	}
	postL, _ := store.Post("launcher", "post-l", "")
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
	sidebar := `<a href="./use-cases/" class="sidebar-item">Use Cases</a>`
	index := read("index.html")
	has("index.html", index,
		`<title>CodeLauncher</title>`, `assets/style.css?sha256=`, `<meta name="theme-color" content="#00215e">`,
		`<a href="./" class="sidebar-item active">CodeLauncher</a>`, sidebar,
		`<a href="./archive.html" class="sidebar-item">Blog</a>`, `<a href="./tags.html" class="sidebar-item">Tags</a>`,
		`<img src="./avatar.png" alt="" class="app-icon" width="48" height="48">`, `<h1>CodeLauncher</h1>`,
		`<div class="content about"><p>As a developer, you run many processes.</p>`,
		`<img src="screenshots/app.png" alt="App" class="screenshot">`, `<h2>Open Source</h2>`, `<h2>Blog</h2>`,
		`<div class="list-item-prefix">Jan 13, 2022</div>`,
		`<a href="https://twitter.com/livid" target="_blank" title="Xin Liu"><img src="`+postL.ID+`/livid.png" alt="Xin Liu" class="avatar"`,
		`<a href="./post-l/">Post L</a>`, `<a href="./page2.html">Older</a>`)
	hasNot("index.html", index, "Use Cases</a></div>") // a page is in the sidebar, not the list
	page2 := read("page2.html")
	has("page2.html", page2, `<h1>Blog</h1>`, `<a href="./post-a/">Post A</a>`, `<a href="./page1.html">Newer</a>`)
	has("page2.html", page2, `<a href="./" class="sidebar-item">CodeLauncher</a>`, `<a href="./archive.html" class="sidebar-item active">Blog</a>`)
	hasNot("page2.html", page2, `content about`)
	post := read("post-l/index.html")
	has("post-l/index.html", post,
		`<title>Post L</title>`, `href="../assets/style.css?sha256=`,
		`<div id="breadcrumb"><a href="../">CodeLauncher</a><span class="chevron"></span><a href="../archive.html">Blog</a><span class="chevron"></span></div>`,
		`<h1>Post L</h1>`, `<a href="https://twitter.com/livid" target="_blank"><img src="livid.png" alt="" class="avatar"`,
		`<a href="https://twitter.com/livid" target="_blank">Xin Liu</a>`, `<div class="page-info">Jan 13, 2022</div>`,
		`<pre><code>DEVELOPMENT_TEAM = X`, `<a href="../release.html">Release</a>`,
		`<a href="../archive.html" class="sidebar-item active">Blog</a>`, `<a href="../use-cases/" class="sidebar-item">Use Cases</a>`)
	if got := read(postL.ID + "/index.html"); got != post {
		t.Error("the post's id and slug folders differ")
	}
	if strings.Contains(read(postL.ID+"/article.json"), "Xin Liu") || strings.Contains(read("planet.json"), "Xin Liu") {
		t.Error("the author reached Planet's JSON")
	}
	plain := read("post-a/index.html")
	has("post-a/index.html", plain, `<div class="page-info">Jan 2, 2022</div>`)
	hasNot("post-a/index.html", plain, `class="author"`)
	page := read("use-cases/index.html")
	has("use-cases/index.html", page, `<h1>Use Cases</h1>`, `<a href="../use-cases/" class="sidebar-item active">Use Cases</a>`, `<a href="../archive.html" class="sidebar-item">Blog</a>`)
	hasNot("use-cases/index.html", page, `breadcrumb`, `page-info`)
	archive := read("archive.html")
	has("archive.html", archive, `<title>Blog · CodeLauncher</title>`, `<h1>Blog</h1>`, `<a href="./archive.html" class="sidebar-item active">Blog</a>`, `<a href="./post-a/">Post A</a>`, `<a href="./post-l/">Post L</a>`)
	hasNot("archive.html", archive, `>Use Cases</a></div>`)
	has("tags.html", read("tags.html"), `<a href="./release.html">Release</a>`, `<div class="list-item-count">1</div>`, `<a href="./tags.html" class="sidebar-item active">Tags</a>`)
	has("release.html", read("release.html"), `<h1>Release</h1>`, `<a href="./tags.html">Tags</a>`, `<a href="./post-l/">Post L</a>`)
	// the site's own files: at the root, but never over the build's
	if read("screenshots/app.png") != "png" || read("_redirects") != "/blog/* /:splat\n" {
		t.Error("static/ did not reach the root")
	}
	if strings.Contains(index, "not the index") {
		t.Error("static/index.html replaced the built index")
	}
	if _, err := os.Stat(filepath.Join(pub, ".DS_Store")); err == nil {
		t.Error("a hidden file was copied")
	}
}
