package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/livid/exe-planet/internal/site"
)

// TestPostLanguages builds posts that are in more than one language
// (PLAN.md, A post in more than one language) on a Paper site and on a
// Platinum one: each language is a page of its own beside the post's,
// the site's pages carry every title, Planet's own files keep the post
// as it was written, and a post in one language builds as it always did.
func TestPostLanguages(t *testing.T) {
	svc, store := newService(t)
	day := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	post := func(slug, kind, name string, m site.Meta, body string, versions map[string]string) *site.Post {
		t.Helper()
		p, err := store.CreatePost(slug, kind, name, m, body)
		if err != nil {
			t.Fatal(err)
		}
		for file, text := range versions {
			if err := os.WriteFile(filepath.Join(p.Dir, file), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	e := func(s string) string { // the escape filter, as Planet's writes it
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

	if _, err := store.CreateSite("kong", "空山", "读书、写字。", "paper"); err != nil {
		t.Fatal(err)
	}
	zhi := post("kong", site.KindPost, "zhi", site.Meta{Title: "纸的纹理", Created: day.AddDate(0, 0, 5), Tags: []string{"散文"}},
		"纸也有*脾气*。[^1]\n\n- [ ] 抄一遍\n\n[^1]: 脚注。\n", map[string]string{
			"index.en.md":      "---\ntitle: The Grain of Paper\n---\n\nPaper has a *temper* too.[^1]\n\n- [ ] Copy it out\n\n[^1]: A note.\n",
			"index.ja.md":      "---\ntitle: 紙の肌理\n---\n\n紙にも*気性*がある。[^1]\n\n[^1]: 注。\n",
			"index.zh-hans.md": "---\ntitle: 重复\n---\n\n和原文同一种语言，不是另一种。\n", // the site's own language: not another one
			"index.fr.md":      "---\ntitle: Vide\n---\n",                  // no text: not built
			"index.old.md":     "kept by hand\n",                           // a file, not a language
		})
	// written in English on this Chinese site, and read in Traditional
	// Chinese and in Korean, a language Paper has no words for
	post("kong", site.KindPost, "letter", site.Meta{Title: "A Letter", Created: day.AddDate(0, 0, 4), Extra: map[string]string{"lang": "en"}},
		"Fog this morning.\n", map[string]string{
			"index.zh-Hant.md": "---\ntitle: 來信\n---\n\n今晨有霧。\n",
			"index.ko.md":      "---\ntitle: 편지\n---\n\n오늘 아침 안개.\n",
		})
	post("kong", site.KindPost, "week", site.Meta{Title: "本周", Created: day.AddDate(0, 0, 3)}, "一件事。\n",
		map[string]string{"index.en.md": "One thing.\n"}) // no title of its own: the post's
	post("kong", site.KindPost, "quiet", site.Meta{Created: day.AddDate(0, 0, 2)}, "秋夜，一个念头。\n",
		map[string]string{"index.en.md": "An autumn night, one thought.\n"})
	post("kong", site.KindPost, "shanju", site.Meta{Title: "山居", Created: day.AddDate(0, 0, 1)}, "山里的早晨来得慢。\n", nil)
	post("kong", site.KindPost, "note", site.Meta{Title: "A Note", Created: day, Extra: map[string]string{"lang": "en"}}, "Written in English.\n", nil)
	if _, err := svc.Build("kong"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Site("kong")
	pub := svc.PublicDir(st.ID)
	read := func(rel string) string {
		t.Helper()
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
	lacks := func(rel, page string, nots ...string) {
		t.Helper()
		for _, n := range nots {
			if strings.Contains(page, n) {
				t.Errorf("%s carries %q", rel, n)
			}
		}
	}
	missing := func(rels ...string) {
		t.Helper()
		for _, rel := range rels {
			if _, err := os.Stat(filepath.Join(pub, filepath.FromSlash(rel))); err == nil {
				t.Errorf("%s was built", rel)
			}
		}
	}

	// the post as it was written: the page it always was, plus the line of
	// its languages, the links to its other pages and the page script
	written := read("zhi/index.html")
	has("zhi/index.html", written,
		`<html lang="zh-Hans">`, `family=Noto+Serif+SC:wght`, `<title>`+e("纸的纹理")+`</title>`,
		`<article class="post" lang="zh-Hans">`, `<h1 class="post-title">`+e("纸的纹理")+`</h1>`, `<em>脾气</em>`,
		`<div class="meta"><p class="date"><time datetime=`,
		`<p class="langs"><span lang="zh-Hans" aria-current="true">简体中文</span><a href="index.en.html" lang="en" hreflang="en" data-lang="en">English</a><a href="index.ja.html" lang="ja" hreflang="ja" data-lang="ja">日本語</a></p></div>`,
		`const L = { here: "zh-hans", orig: "zh-hans", files: { "zh-hans": "", "en": "index.en.html", "ja": "index.ja.html", } };`,
		`<link rel="alternate" hreflang="zh-Hans" href="https://.eth.sucks/zhi/">`, `<link rel="alternate" hreflang="en" href="https://.eth.sucks/zhi/index.en.html">`,
		`<link rel="alternate" hreflang="ja" href="https://.eth.sucks/zhi/index.ja.html">`, `<link rel="alternate" hreflang="x-default" href="https://.eth.sucks/zhi/">`,
		`<a href="../archive.html">归档</a>`, `id="fn:1"`)
	lacks("zhi/index.html", written, "The Grain of Paper", "紙の肌理", "重复", "Vide", "hreflang=\"fr\"", `hreflang="old"`)
	if n := strings.Count(written, `id="fn:1"`); n != 1 {
		t.Errorf("zhi/index.html holds %d footnotes numbered 1", n)
	}

	// in English: its own page, the template's Language set to English for
	// it — the face, the words around the text, the frame's language
	en := read("zhi/index.en.html")
	has("zhi/index.en.html", en,
		`<html lang="en">`, `family=Noto+Serif+SC:wght`, `<title>The Grain of Paper</title>`,
		`<article class="post" lang="en">`, `<h1 class="post-title">The Grain of Paper</h1>`, `Paper has a <em>temper</em> too.`,
		`<meta property="og:title" content="The Grain of Paper" />`, `<meta property="og:description" content="Paper has a temper too.`,
		`<p class="langs"><a href="./?lang=zh-hans" lang="zh-Hans" hreflang="zh-Hans" data-lang="zh-hans">简体中文</a><span lang="en" aria-current="true">English</span><a href="index.ja.html" lang="ja" hreflang="ja" data-lang="ja">日本語</a></p>`,
		`const L = { here: "en", orig: "zh-hans", files: {`, `<a href="../archive.html">Archive</a>`, `<a href="../tags.html">Tags</a>`,
		`<a href="../`+"散文"+`.html"><span class="hash">#</span>`+e("散文")+`</a>`, `<link rel="alternate" hreflang="x-default" href="https://.eth.sucks/zhi/">`,
		`<input type="checkbox" disabled="" /> Copy it out`)
	lacks("zhi/index.en.html", en, "纸也有", "归档", "脚注")
	if n := strings.Count(en, `id="fn:1"`); n != 1 {
		t.Errorf("zhi/index.en.html holds %d footnotes numbered 1", n)
	}
	ja := read("zhi/index.ja.html")
	has("zhi/index.ja.html", ja, `<html lang="ja">`, `family=Noto+Serif+JP:wght`, `<h1 class="post-title">`+e("紙の肌理")+`</h1>`, `<em>気性</em>`,
		`<a href="../archive.html">アーカイブ</a>`, `<span lang="ja" aria-current="true">日本語</span>`, `const L = { here: "ja", orig: "zh-hans",`)
	// the folder under the post's id holds the same pages as the one under its slug
	for _, f := range []string{"index.html", "index.en.html", "index.ja.html"} {
		if read(zhi.ID+"/"+f) != read("zhi/"+f) {
			t.Errorf("%s differs between the id's folder and the slug's", f)
		}
	}
	// what is not another language is not a page: the site's own language
	// again, a file with no text, a file a person keeps; that last one is
	// still the attachment it always was, and no language file is one
	missing("zhi/index.zh-hans.html", "zhi/index.fr.html", "zhi/index.old.html", "zhi/index.en.md", "zhi/index.ja.md", "zhi/index.zh-hans.md", "zhi/index.fr.md")
	has("zhi/index.old.md", read("zhi/index.old.md"), "kept by hand")

	// Planet's own files keep the post as it was written
	for _, rel := range []string{"planet.json", "rss.xml", "zhi/article.json", "zhi/article.md"} {
		page := read(rel)
		lacks(rel, page, "Grain of Paper", "temper", "紙の肌理", "気性", "index.en", "index.ja", "languages")
	}
	if art := strings.Join(strings.Fields(read("zhi/article.json")), ""); !strings.Contains(art, `"attachments":["index.old.md"]`) || !strings.Contains(art, "纸的纹理") {
		t.Errorf("zhi/article.json: %s", art)
	}

	// written in English on a Chinese site (lang: en): the page is an
	// English page, and it is the one the others call the original
	letter := read("letter/index.html")
	has("letter/index.html", letter, `<html lang="en">`, `<article class="post" lang="en">`, `<a href="../archive.html">Archive</a>`,
		`<p class="langs"><span lang="en" aria-current="true">English</span><a href="index.ko.html" lang="ko" hreflang="ko" data-lang="ko">한국어</a><a href="index.zh-hant.html" lang="zh-Hant" hreflang="zh-Hant" data-lang="zh-hant">繁體中文</a></p>`,
		`const L = { here: "en", orig: "en", files: { "en": "", "ko": "index.ko.html", "zh-hant": "index.zh-hant.html", } };`)
	has("letter/index.zh-hant.html", read("letter/index.zh-hant.html"), `<html lang="zh-Hant">`, `family=Noto+Serif+TC:wght`, `<a href="../archive.html">歸檔</a>`,
		`<a href="./?lang=en" lang="en" hreflang="en" data-lang="en">English</a>`, e("來信"))
	// Korean, which Paper has no words for: the text is marked Korean and
	// the page stays in the site's language
	ko := read("letter/index.ko.html")
	has("letter/index.ko.html", ko, `<html lang="zh-Hans">`, `<article class="post" lang="ko">`, `<a href="../archive.html">归档</a>`, e("편지"),
		`<span lang="ko" aria-current="true">한국어</span>`)
	// a post that only names its language, with no other: an English page,
	// no line of languages, no script
	note := read("note/index.html")
	has("note/index.html", note, `<html lang="en">`, `<article class="post" lang="en">`, `<a href="../archive.html">Archive</a>`)
	lacks("note/index.html", note, `class="langs"`, `class="meta"`, "const L =", "hreflang")
	// a post in one language is the page it always was
	shanju := read("shanju/index.html")
	has("shanju/index.html", shanju, `<html lang="zh-Hans">`, `<article class="post">`, "\n  <p class=\"date\"><time datetime=")
	lacks("shanju/index.html", shanju, `class="langs"`, `class="meta"`, "const L =", "hreflang", "planet-lang", "data-l")
	// a version with no title takes the post's; an untitled post has one too
	has("week/index.en.html", read("week/index.en.html"), `<h1 class="post-title">`+e("本周")+`</h1>`, "One thing.")
	quiet := read("quiet/index.en.html")
	has("quiet/index.en.html", quiet, "An autumn night, one thought.", `<span lang="en" aria-current="true">English</span>`)
	lacks("quiet/index.en.html", quiet, `class="post-title"`)

	// the site's pages: one page each, every title in each language, the
	// written one first and marked, and the script told the site's languages
	for _, rel := range []string{"index.html", "archive.html"} {
		page := read(rel)
		has(rel, page,
			`data-langs="zh-hans en ja ">`,
			`<a class="t" href="./zhi/" data-l="zh-hans" lang="zh-Hans" data-o>`+e("纸的纹理")+`</a><a class="t" href="./zhi/index.en.html" data-l="en" lang="en">The Grain of Paper</a><a class="t" href="./zhi/index.ja.html" data-l="ja" lang="ja">`+e("紙の肌理")+`</a>`,
			`data-langs="en ko zh-hant ">`,
			`<a class="t" href="./letter/" data-l="en" lang="en" data-o>A Letter</a><a class="t" href="./letter/index.ko.html" data-l="ko" lang="ko">`+e("편지")+`</a><a class="t" href="./letter/index.zh-hant.html" data-l="zh-hant" lang="zh-Hant">`+e("來信")+`</a>`,
			`<a class="t" href="./shanju/">`+e("山居")+`</a>`, `<a class="t" href="./note/">A Note</a>`,
			`const L = { site: ["en", "ja", "ko", "zh-hans", "zh-hant", ] };`, `<html lang="zh-Hans">`)
		// an untitled post is listed as it was written
		lacks(rel, page, "An autumn night", `data-l="fr"`, "Vide", "重复")
	}
	has("index.html", read("index.html"), `<li class="untitled">`, `<a class="d" href="./quiet/">`)
	tags := read("tags.html")
	has("tags.html", tags, `const L = { site: [`)
	has("散文.html", read("散文.html"), `data-langs="zh-hans en ja ">`, `const L = { site: [`)
	css := read("assets/style.css")
	has("assets/style.css", css, `html:not(.lang-on) [data-l]:not([data-o]) { display: none; }`, `.meta .date { margin: 0; }`)

	// the window's page column asks for a post's page in a language
	if page, err := svc.View("kong", "zhi", "ja", Attachments{}); err != nil || !strings.Contains(page, "紙の肌理") || !strings.Contains(page, `"page-link"`) {
		t.Errorf("View in ja: %v", err)
	}
	if page, err := svc.View("kong", "zhi", "JA", Attachments{}); err != nil || !strings.Contains(page, "紙の肌理") {
		t.Errorf("View in JA: %v", err)
	}
	if _, err := svc.View("kong", "zhi", "ko", Attachments{}); err == nil || !strings.Contains(err.Error(), "is not built in ko") {
		t.Errorf("View in a language the post is not in: %v", err)
	}
	if _, err := svc.View("kong", "zhi", "../x", Attachments{}); err == nil || !strings.Contains(err.Error(), "is not a language") {
		t.Errorf("View in no language: %v", err)
	}

	// Platinum has no Language setting: its pages are English unless a
	// post's page is in another language, which is then the page's lang;
	// its own words stay English
	if _, err := store.CreateSite("desk", "exe", "Notes.", "platinum"); err != nil {
		t.Fatal(err)
	}
	post("desk", site.KindPost, "meet", site.Meta{Title: "Meet exe", Created: day.AddDate(0, 0, 1), Tags: []string{"exe"}}, "A personal cloud.\n",
		map[string]string{"index.zh-hans.md": "---\ntitle: 认识 exe\n---\n\n一朵个人云。\n", "index.ja.md": "---\ntitle: exe の紹介\n---\n\n個人用クラウド。\n"})
	post("desk", site.KindPost, "install", site.Meta{Title: "Install", Created: day}, "One line.\n", nil)
	post("desk", site.KindPage, "about", site.Meta{Title: "About", Created: day}, "About this site.\n",
		map[string]string{"index.zh-hans.md": "---\ntitle: 关于\n---\n\n关于这个站点。\n"})
	if _, err := svc.Build("desk"); err != nil {
		t.Fatal(err)
	}
	st, _ = store.Site("desk")
	pub = svc.PublicDir(st.ID)
	meet := read("meet/index.html")
	has("meet/index.html", meet, `<html lang="en">`, `<div class="title">Meet exe</div>`,
		`<div class="meta"><div class="date">`,
		`<div class="langs"><b lang="en" aria-current="true">English</b><a href="index.ja.html" lang="ja" hreflang="ja" data-lang="ja">日本語</a><a href="index.zh-hans.html" lang="zh-Hans" hreflang="zh-Hans" data-lang="zh-hans">简体中文</a></div></div>`,
		`const L = { here: "en", orig: "en", files: { "en": "", "ja": "index.ja.html", "zh-hans": "index.zh-hans.html", } };`,
		`<link rel="alternate" hreflang="ja" href="https://.eth.sucks/meet/index.ja.html">`)
	zh := read("meet/index.zh-hans.html")
	has("meet/index.zh-hans.html", zh, `<html lang="zh-Hans">`, `<div class="title">`+e("认识 exe")+`</div>`, `<h1 class="post-title">`+e("认识 exe")+`</h1>`,
		"一朵个人云。", `<a href="./?lang=en" lang="en" hreflang="en" data-lang="en">English</a>`, `<b lang="zh-Hans" aria-current="true">简体中文</b>`,
		`<span class="t">Archive</span>`, `<span class="quiet">Tags</span>`)
	lacks("meet/index.zh-hans.html", zh, "A personal cloud.")
	// a page, which has no date, has its languages all the same
	has("about/index.zh-hans.html", read("about/index.zh-hans.html"), `<div class="meta"><div class="langs">`, e("关于"))
	install := read("install/index.html")
	has("install/index.html", install, `<html lang="en">`, "\n  <div class=\"date\">")
	lacks("install/index.html", install, `class="langs"`, `class="meta"`, "const L =", "hreflang", "data-l")
	for _, rel := range []string{"index.html", "archive.html", "exe.html"} {
		page := read(rel)
		has(rel, page, `<div class="item" data-langs="en ja zh-hans ">`,
			`<a class="item-title" href="./meet/" data-l="en" lang="en" data-o>Meet exe</a><a class="item-title" href="./meet/index.ja.html" data-l="ja" lang="ja">exe `+e("の紹介")+`</a><a class="item-title" href="./meet/index.zh-hans.html" data-l="zh-hans" lang="zh-Hans">`+e("认识")+` exe</a>`,
			`const L = { site: ["en", "ja", "zh-hans", ] };`, `<html lang="en">`)
		if rel != "exe.html" { // (the tag's page lists the posts under it)
			has(rel, page, `<a class="item-title" href="./install/">Install</a>`)
		}
	}
	has("assets/style.css", read("assets/style.css"), `html:not(.lang-on) [data-l]:not([data-o]) { display: none; }`, `.langs { display: flex;`)

	// a feed in each other language: every article, in that language when
	// it can be read in it, as written when not; rss.xml as it always was
	jaFeed := read("rss.ja.xml")
	has("rss.ja.xml", jaFeed, `/rss.ja.xml" rel="self"`, "<language>ja</language>", "<title>exe "+e("の紹介")+"</title>",
		"個人用クラウド。", "/index.ja.html</link>", "/index.ja.html</guid>", "<title>Install</title>", "One line.",
		"<title>About</title>", `<?xml-stylesheet type="text/xsl" href="assets/rss.xsl"?>`)
	lacks("rss.ja.xml", jaFeed, "A personal cloud.", "index.zh-hans.html")
	zhFeed := read("rss.zh-hans.xml")
	has("rss.zh-hans.xml", zhFeed, `/rss.zh-hans.xml" rel="self"`, "<language>zh-Hans</language>", "一朵个人云。",
		"关于这个站点。", "/index.zh-hans.html</link>", "One line.")
	enFeed := read("rss.xml")
	has("rss.xml", enFeed, `/rss.xml" rel="self"`, "A personal cloud.", "About this site.")
	lacks("rss.xml", enFeed, "<language>", "index.ja.html", "langFile", "個人用")
	if _, err := os.Stat(filepath.Join(pub, "rss.en.xml")); err == nil {
		t.Error("the site's own language has a second feed")
	}
	// named in the head of the site's pages and of a post in more than one
	// language, beside rss.xml; a post in one language names none
	for _, rel := range []string{"index.html", "archive.html", "meet/index.html", "meet/index.ja.html"} {
		has(rel, read(rel), `data-feed="ja" href="`, `rss.ja.xml">`, `data-feed="zh-hans" href="`, `rss.zh-hans.xml">`, `title="exe (日本語)"`)
	}
	lacks("install/index.html", install, "rss.ja.xml")

	// a site whose posts are each in one language has none of it: no
	// script, no marks on its lists
	if _, err := store.CreateSite("plain", "Plain", "", "paper"); err != nil {
		t.Fatal(err)
	}
	post("plain", site.KindPost, "hello", site.Meta{Title: "你好", Created: day}, "只有一种语言。\n", nil)
	if _, err := svc.Build("plain"); err != nil {
		t.Fatal(err)
	}
	st, _ = store.Site("plain")
	pub = svc.PublicDir(st.ID)
	for _, rel := range []string{"index.html", "archive.html", "tags.html", "hello/index.html"} {
		lacks(rel, read(rel), "const L =", "planet-lang", "data-l", "data-langs", `class="langs"`, "hreflang")
	}
	for _, f := range []string{"rss.en.xml", "rss.zh-hans.xml", "rss.ja.xml"} {
		if _, err := os.Stat(filepath.Join(pub, f)); err == nil {
			t.Errorf("a site in one language has %s", f)
		}
	}
}
