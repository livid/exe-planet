package service

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/site"
)

// TestPaperTemplate builds a site with Paper, ours, through every page kind
// it has — a paginated index, a post with tags, emphasis and a footnote,
// a page in the navigation, an untitled post, the archive, the tag list
// and a tag page — in its default language, then in the other three its
// Language setting names, and once with the hub's replies under a post.
func TestPaperTemplate(t *testing.T) {
	svc, store := newService(t)
	if _, err := store.CreateSite("wen", "宣纸集", "读书，写字，偶尔*出门*。", "paper"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Site("wen")
	os.WriteFile(filepath.Join(st.Dir, "avatar.png"), []byte("png"), 0o644)
	day := time.Date(2026, 9, 1, 19, 0, 0, 0, time.UTC)
	for i := 1; i <= 13; i++ {
		m := site.Meta{Title: "第" + string(rune('A'+i-1)) + "篇", Created: day.AddDate(0, 0, i)}
		body := "正文 " + string(rune('A'+i-1)) + "。\n"
		if i == 13 {
			m.Title, m.Tags = "荷塘月色", []string{"散文", "Zhu Ziqing"}
			body = "这几天心里颇不宁静。今晚在院子里坐着乘凉，忽然想起日日走过的荷塘，在这满月的光里，总该另有一番样子吧。\n\n" +
				"> 采莲南塘秋，莲花过人头。\n\n我*忽然*想起采莲的事情来了。[^1]\n\n---\n\n## 二\n\n`code`\n\n- [x] 读完\n- [ ] 抄一遍\n\n[^1]: 《西洲曲》。\n"
		}
		if _, err := store.CreatePost("wen", site.KindPost, "p"+string(rune('a'+i-1)), m, body); err != nil {
			t.Fatal(err)
		}
	}
	quiet, err := store.CreatePost("wen", site.KindPost, "quiet", site.Meta{Created: day.AddDate(0, 0, 20)},
		"秋夜，一个念头。\n\n![月](moon.png)\n\n<img width=\"640\" src=\"moon.png\">\n\n![远](https://example.com/far.png)\n")
	if err != nil {
		t.Fatal(err)
	}
	store.AddAttachment("wen", "quiet", "", "moon.png", strings.NewReader("png"))
	store.CreatePost("wen", site.KindPage, "about", site.Meta{Title: "关于", Created: day}, "关于这里。\n")
	if _, err := svc.Build("wen"); err != nil {
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
	// the escape filter writes every non-ASCII character as a reference,
	// as Planet's does
	e := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if r < 0x80 {
				b.WriteRune(r)
			} else {
				fmt.Fprintf(&b, "&#x%X;", r)
			}
		}
		return b.String()
	}
	lacks := func(rel, page string, nots ...string) {
		t.Helper()
		for _, n := range nots {
			if strings.Contains(page, n) {
				t.Errorf("%s carries %q", rel, n)
			}
		}
	}

	index := read("index.html")
	has("index.html", index,
		`<html lang="zh-Hans">`,
		`family=Noto+Serif:ital,wght@0,100..900;1,100..900&amp;family=Noto+Serif+SC:wght@200..900&amp;display=swap`,
		`assets/style.css?sha256=`, `<header class="mast home">`,
		`<h1 class="site"><a href="./">`+e("宣纸集")+`</a></h1><div class="about"><p>读书，写字，偶尔<em>出门</em>。</p>`,
		`<a href="./about/">`+e("关于")+`</a>`, `<a href="./archive.html">归档</a>`, `<a href="./tags.html">标签</a>`,
		`<li class="untitled">`, `<div class="content"><p>秋夜，一个念头。</p>`,
		`<a class="t" href="./pm/">`+e("荷塘月色")+`</a>`,
		`<time datetime="2026-09-13T12:00:00-07:00" data-f="num">Sep 13, 2026</time>`,
		`<a class="older" href="./page2.html" rel="next">下一页 →</a>`, `<span class="at">1 / 2</span>`,
		`<a href="./rss.xml">RSS</a>`, `由 <a href="https://exe.v2core.com/"`, `data-f`, `Intl.DateTimeFormat(lang`)
	lacks("index.html", index, `id="compose"`, `class="replies"`, `aria-current`)
	// an untitled post is shown whole on the index, which is a folder above
	// the post's own: its pictures are found in the post's folder
	has("index.html", index, `<img src="./`+quiet.ID+`/moon.png" alt="月" />`, `<img width="640" src="./`+quiet.ID+`/moon.png" />`,
		`<img src="https://example.com/far.png" alt="远" />`)
	has("page2.html", read("page2.html"), `<a class="newer" href="./page1.html" rel="prev">← 上一页</a>`, `2 / 2`, `href="./pa/">`+e("第A篇")+`</a>`)

	post := read("pm/index.html")
	has("pm/index.html", post,
		`<header class="mast">`, `<a class="site" href="../">`+e("宣纸集")+`</a>`, `<a href="../archive.html">归档</a>`,
		`<h1 class="post-title">`+e("荷塘月色")+`</h1>`,
		`<p class="date"><time datetime="2026-09-14T12:00:00-07:00" data-f="long">Sep 14, 2026</time></p>`,
		`<blockquote>`, `<em>忽然</em>`, `<hr`, `<h2>二</h2>`, `<code>code</code>`, `class="footnotes"`,
		`<a href="../散文.html"><span class="hash">#</span>`+e("散文")+`</a>`, `<a href="../zhu-ziqing.html"><span class="hash">#</span>Zhu Ziqing</a>`,
		`<meta property="og:type" content="article" />`)
	lacks("pm/index.html", post, `id="compose"`, `replies.js`)
	// a to-do list: the checkboxes as the Markdown writes them, the script
	// that numbers the items for the app showing the page, and the
	// stylesheet's own box in the checkbox's place
	has("pm/index.html", post, `<li><input type="checkbox" checked="" disabled="" /> 读完</li>`, `<li><input type="checkbox" disabled="" /> 抄一遍</li>`,
		`li.id = "todo-item-" + (++n);`, `if (li.parentElement.matches(".content > ul"))`)
	has("assets/style.css", read("assets/style.css"), `@supports selector(li:has(> input)) {`,
		`.content li[id^="todo-item-"] { pointer-events: none; }`, `.content li[id^="todo-item-"] > *, .content li[id^="todo-item-"]::before { pointer-events: auto; }`,
		`width: 13px; height: 13px; border: 1px solid var(--quiet); }`, `background: var(--tick) no-repeat; pointer-events: none; }`,
		`.content li > input[type=checkbox]:checked + .todo-words, .content li > input[type=checkbox]:checked + p { color: var(--quiet); }`)
	lacks("index.html", index, "todo-item-")
	about := read("about/index.html")
	has("about/index.html", about, `<h1 class="post-title">`+e("关于")+`</h1>`)
	lacks("about/index.html", about, `<p class="date">`)
	archive := read("archive.html")
	has("archive.html", archive, `<title>归档 · `+e("宣纸集")+`</title>`, `<h1 class="page-title">归档</h1>`,
		`<a href="./archive.html" aria-current="page">归档</a>`,
		`<h2><time data-f="month">September 2026</time></h2>`, `href="./pm/">`+e("荷塘月色")+`</a>`,
		`<li class="untitled"><div class="t snip"><p>秋夜，一个念头。</p>`)
	tags := read("tags.html")
	has("tags.html", tags, `<title>标签 · `+e("宣纸集")+`</title>`, `<h1 class="page-title">标签</h1>`,
		`<a href="./tags.html" aria-current="page">标签</a>`, `<a href="./散文.html">`+e("散文")+`</a><span class="n">1</span>`)
	tag := read("散文.html")
	has("散文.html", tag, `<title>`+e("散文")+` · `+e("宣纸集")+`</title>`, `<h1 class="page-title"><span class="hash">#</span>`+e("散文")+`</h1>`, `href="./pm/">`+e("荷塘月色")+`</a>`)
	lacks("散文.html", tag, `aria-current`, `class="mast home"`)

	// every link a page makes to the site itself is relative, so the site
	// works at any root and the Planet window can follow it
	abs := regexp.MustCompile(`(?:href|src)="/`)
	for rel, page := range map[string]string{"index.html": index, "pm/index.html": post, "about/index.html": about, "archive.html": archive, "tags.html": tags, "散文.html": tag} {
		if m := abs.FindString(page); m != "" {
			t.Errorf("%s has a root-relative link: %s", rel, m)
		}
		if strings.Contains(page, "{%") || strings.Contains(page, "{{") {
			t.Errorf("%s has template syntax left in it", rel)
		}
	}
	for _, f := range []string{"assets/style.css", "assets/replies.js", "rss.xml", "planet.json", "avatar.png"} {
		if _, err := os.Stat(filepath.Join(pub, filepath.FromSlash(f))); err != nil {
			t.Errorf("no %s in the build", f)
		}
	}
	// a block in another script than the site's is set as its own
	// language: the page script marks it either way round (Latin letters
	// alone on a CJK site; mostly Chinese or Japanese on an English one),
	// and the stylesheet gives the marked block its face and its weight
	has("pm/index.html", post, `if (latin.test(s) && !cjk.test(s)) el.lang = "en";`,
		`el.lang = kana * 10 >= kana + han ? "ja" : "zh";`, `else if (marked.has(el.parentElement.closest("[lang]"))) el.lang = lang;`)
	has("assets/style.css", read("assets/style.css"),
		`.content :is(p, li, blockquote, td):lang(en), .list .t:lang(en) { font-weight: 400; }`,
		`:root:lang(en) .list .t:lang(zh) { font-family: "Noto Serif SC", "Noto Serif", "Songti SC", "STSong", "SimSun", serif; }`,
		`:root:lang(en) .list .t:lang(ja) { font-family: "Noto Serif JP", "Noto Serif SC", "Noto Serif", "Hiragino Mincho ProN", "Yu Mincho", serif; }`,
		`:root:lang(en) .content :is(p, li, blockquote, td):is(:lang(zh), :lang(ja)), :root:lang(en) .list .t:is(:lang(zh), :lang(ja)) { font-weight: 500; }`)

	// the Language setting: the page's lang, the CJK face, the words
	for _, c := range []struct{ lang, family, archive, tags, older, replies string }{
		{"zh-Hant", "Noto+Serif+TC", "歸檔", "標籤", "下一頁", "回覆"},
		{"ja", "Noto+Serif+JP", "アーカイブ", "タグ", "次へ", "返信"},
		{"en", "Noto+Serif+SC", "Archive", "Tags", "Older", "Replies"},
	} {
		ts := map[string]string{"language": c.lang}
		if _, err := store.UpdateSite("wen", site.SiteUpdate{TemplateSettings: &ts}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Build("wen"); err != nil {
			t.Fatal(err)
		}
		has(c.lang+" index.html", read("index.html"), `<html lang="`+c.lang+`">`, `&amp;family=`+c.family+`:wght@200..900`,
			`>`+c.archive+`</a>`, `>`+c.tags+`</a>`, c.older+` →</a>`)
		has(c.lang+" archive.html", read("archive.html"), `<h1 class="page-title">`+c.archive+`</h1>`)
		// and the Replies heading under a post announced on a hub
		id := strings.Repeat("ab", 32)
		store.SetPostHub("wen", "pm", site.KindPost, "https://hub.example.test/p/"+id)
		svc.Build("wen")
		has(c.lang+" pm/index.html", read("pm/index.html"), `<h2 class="replies-title">`+c.replies+`</h2>`)
	}
}

// TestPaperReplies: under a post announced on a hub, Paper draws the
// composer with its words in the site's language and frames the hub's
// replies page in the paper look, and loads its own script.
func TestPaperReplies(t *testing.T) {
	svc, store := newService(t)
	store.CreateSite("wen", "宣纸集", "", "paper")
	st, _ := store.Site("wen")
	id := strings.Repeat("ab", 32)
	store.CreatePost("wen", site.KindPost, "with", site.Meta{Title: "有回复", Hub: "https://hub.example.test/p/" + id}, "x\n")
	if _, err := svc.Build("wen"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(svc.PublicDir(st.ID), "with", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	js, err := os.ReadFile(filepath.Join(svc.PublicDir(st.ID), "assets", "replies.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`document.documentElement.classList.add('js')`,
		`<section class="replies" id="replies">`, `<h2 class="replies-title">回复</h2>`,
		`id="compose" data-hub="https://hub.example.test" data-root="` + id + `"`,
		`<button type="button" class="btn signin">用 Solana 登录</button>`,
		`placeholder="写下回复"`, `<span class="grow status">正在查询这个地址……</span>`,
		`<button type="button" class="btn send" disabled>回复</button>`,
		`src="https://hub.example.test/p/` + id + `/replies?look=paper&amp;lang=zh-Hans"`,
		`<a href="https://hub.example.test/p/` + id + `" target="_blank" rel="noopener">hub.example.test</a>`,
		`<script defer src="../assets/replies.js?sha256=` + fmt.Sprintf("%x", sha256.Sum256(js)) + `"></script>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("with/index.html lacks %q", want)
		}
	}
}
