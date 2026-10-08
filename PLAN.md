# exe-planet — plan

A standalone Go daemon that builds static blog sites from Markdown files in
the exe Workspace, renders them with Planet's site templates, and writes a
site a Planet user can follow: the same `planet.json`, article folders and
RSS that Planet itself emits. Each site can, per node and off by default,
be exposed through exe's proxy under a hostname and published to IPFS
through the host's Kubo. The editor is an exe desktop app kept in this repo.

Status: phases 0 to 3 done 2026-09-25. The template engine and the builder reproduce Planet 0.22.4 byte for byte on the Sparkle and Memories goldens (four planets, every page, feed and JSON); the Plain and Sepia goldens on the Mac were rendered with template versions the Mac no longer holds and wait for a rebuild there. The daemon builds a site when its files change, on request, and at start, serves each build under `/sites/<slug>/`, renders previews, and imports a planet from a Planet library folder on this machine. No app or publishing yet.

**This file is the single source of truth for the project design.** Update
it whenever a decision changes; code follows the plan, not the other way
around.

## Decisions (2026-09-25, with Livid)

- **Separate daemon**, not a package inside exe. Reason: the builder will
  be rebuilt and restarted often, and an exe restart reboots the hub VM and
  reattaches every Terminal. Listens on `127.0.0.1:7799`, loopback only.
- **Files only.** Every user-created thing is a file or folder under the
  Workspace. No database anywhere in this project.
- **Site slug is required and validated** at creation and is the folder
  name. The title lives in `site.json` and may change at any time; the app
  never renames a folder on its own. UUID folder names were rejected because
  nobody opens such a folder by hand.
- **Markdown: goldmark now, no dependency later.** The renderer sits behind
  one interface. The end-game replacement is a hand-written renderer that
  passes a corpus built from Planet's own `article.md` and
  `contentRendered` pairs. Everything else is hand-rolled from the start.
- **Template engine is hand-rolled**, a Stencil subset sized to the six
  Planet templates, which are its test fixtures.
- **exe-planet generates and keeps every IPNS key** and imports it into
  Kubo over RPC. Kubo 0.43 answers `key/import` but 404 for `key/export`,
  so a Kubo-owned key could only be backed up as root.
- **v1 scope: builder plus import from a Planet library folder.** No
  following or reading of other planets. The importer is needed anyway for
  the golden tests.
- **Expose and IPFS are per node and default off.**
- **One key per site, three roles** (2026-09-26, Livid's proposal). The
  site's ed25519 key is its IPNS name, its hub author and its Solana
  address (tips, later). See Replies from the hub.

## Layout on disk

Shared, synced to every peer as plain files (`~/.exe/workspace` is synced
under the `@workspace` namespace: whole-file last-writer-wins, a concurrent
edit keeps the loser under `~/.exe/sync-conflicts`, hidden paths skipped,
files over 10 MB neither accepted by the file API nor synced):

```
~/.exe/workspace/Planet/
  Templates/                  shared by every site; reserved name
    plain/  sepia/  grid/  gamedb/  memories/  sparkle/  platinum/  paper/
  Archived/                   reserved; the builder ignores it
    old-notes/…
  charts/                     one site; folder = slug
    site.json                 identity and settings, no secrets
    avatar.png
    head.html                 optional custom code, injected verbatim
    body-start.html
    body-end.html
    posts/
      hello-world/            one post; folder = post slug
        index.md              front matter, then Markdown
        photo.jpg             attachments next to the text
    pages/
      about/index.md          in the navigation, not in RSS
    drafts/
      unfinished/index.md     built for preview only; move to posts/ to publish
  launcher/
    site.json
    …
```

Per node, never synced, derived or secret:

```
~/.exe/planet/
  <site uuid>/
    node.json                 expose {sub}, ipfs {enabled}, announce {hub}, last build, last CID, last published
    ipns.key                  PKCS#8 PEM, mode 600, the site's key: IPNS name, hub author, Solana address
    hub.json                  the site on its hub: each post's state and thread, the invite, the profile sent
    public/                   Planet's Public/<uuid> layout, rebuilt from the Workspace source
```

Rules:

- A site is any folder directly under `Planet/` holding a `site.json`.
  `Templates` and `Archived` are the only reserved names. A folder made by
  hand with a `site.json` is a site.
- The UUID in `site.json` is the identity that node state, the Kubo key
  name (`exe-planet-<uuid>`) and Planet followers key on. Renaming a folder
  is a plain move; the next scan picks it up.
- State changes are moves: `drafts/` to `posts/` publishes, into
  `Archived/` archives. Attachments travel with their folder.
- Post folders may carry a date prefix or not; order comes from `created`
  in the front matter.
- Deleting a site is deleting its Workspace folder. The daemon then removes
  `~/.exe/planet/<uuid>/`, drops the exe route and unpins the last CID.

### Slugs

`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`: lowercase letters, digits and
hyphens, no leading or trailing hyphen, at most 63 characters so a site slug
can be a DNS label. Reserved site slugs: `templates`, `archived`. The
new-site dialog defaults the slug from the title and refuses to create until
it validates. A post slug follows the same rule and becomes the post's
public path, as Planet's article slug does.

### site.json

```json
{
  "id": "4D65E2EA-076C-434B-AB9D-00582F48CEB4",
  "name": "Charts & Thoughts",
  "about": "…",
  "template": "sepia",
  "template_settings": { "highlightColor": "#ff3b30" },
  "ipns": "k51…",
  "created": "2026-09-25T10:00:00Z",
  "author": "",
  "tags": { "charts": "Charts" },
  "social": { "twitter": "", "github": "", "telegram": "", "mastodon": "", "discord": "", "farcaster": "" },
  "plausible": { "enabled": false, "domain": "", "server": "plausible.io" }
}
```

`ipns` is public information and is written back into `site.json` by the
node that generates the key, so every synced node builds an identical
`planet.json`. Dates in files people edit are RFC 3339; the builder converts
to Planet's encoding on output. There are no tokens or keys in this file.

### Front matter

A `---` block of `key: value` lines, no YAML library:

```
---
id: 339A627C-0AFB-4505-BFE2-75B79C7F5E84
title: Hello, world
created: 2026-09-25T10:00:00Z
modified: 2026-09-25T11:00:00Z
tags: charts, macos
hero: photo.jpg
link: https://example.com/original     optional external link
type: page                             only under pages/; default post
hub: https://hub.example.com/p/<64 hex>   the post's thread, written by the announcing node
---
```

`id` is issued when the post is created and never changes, so a rename of
the folder is an edit for followers, not a delete plus an add.

A post made by hand — a folder and an `index.md` written into the
Workspace by a person, an agent or a synced peer, with no front matter
or without these lines — is issued its `id` and its `created` by the
build that first meets it (`Store.AdoptPosts`, 2026-10-06), as two lines
in the file's front matter and no other byte changed (`site.Issue`); the
watcher sees the write and builds once more, which finds nothing left
to issue. The id is not drawn at random: it is the version 5 UUID of the
folder's name in the namespace of the site's id, so two nodes that meet
the same file before either's line has reached the other write the same
id. A name whose id another post holds (a post renamed away keeps the id
it was given under the old name) takes the next one made from it.
`created` is when the file was last written. Until then such a post
stopped its whole site from building (Livid: "fix the id-less post
build bug"): with no id its folder in the build was the build's own
root, and the copy made under its slug a copy of the whole build into
itself, over and over until the path was too long to write; and with no
date it was dated by an overflow, in 2293. The builder now refuses an
article whose id cannot name a folder, and the build leaves out a post
whose file could not be written to, so the rest of the site still
builds.

## Build output

Per site, `~/.exe/planet/<uuid>/public/` mirrors Planet's `Public/<uuid>/`
exactly, verified by diffing against sites built by Planet 0.22.4 on
Livid's Mac (`ssh livid@192.168.1.99`, the `xyz.planetable.Planet`
container; see `/home/livid/.claude` memory `planet-mac-golden-sites`):

- `index.html`, `page<N>.html` when the template paginates, `tags.html`,
  `archive.html`, per-tag pages, `rss.xml`, `podcast.xml` when enabled,
  `robots.txt`, `avatar.png`, `assets/` copied from the template,
  `templateSettings.json`, and `planet.json`.
- Per article `<uuid>/`: `index.html`, `simple.html` when the template has
  one, `article.md`, `article.json`, `nft.json` when the template asks,
  attachments, and the generated `_grid.png` and `_grid.jpg` hero grids. An
  article with a slug is copied whole to `<slug>/` as well, so both paths
  resolve.
- `planet.json` fields: `id, name, about, ipns, created, updated, tags,
  articles[]`, plus the optional social, Plausible, Juicebox, donation and
  podcast fields when set. Every `Date` is a double of seconds since
  2001-01-01, Foundation's default encoding; a Planet follower silently
  fails on anything else. `tags` maps slug to display name.
- `article.json` fields, as observed in a real build: `articleType,
  attachments, cids, content, contentRendered, created, externalLink,
  hasAudio, hasVideo, heroImage, heroImageFilename, heroImageHeight,
  heroImageURL, heroImageWidth, id, link, slug, tags, title`, and
  `modified, articleNumber, articleReference, pinned` when present. `cids`
  maps each attachment filename to its CIDv0.
- Planet's follow path fetches `<gateway>/ipfs/<cid>/planet.json` and only
  updates when `updated` has advanced, so every build that changes anything
  bumps `updated`.

## Template engine

A template is a folder with `template.json` (`name, description, author,
authorWebsite, version, buildNumber, idealItemsPerPage,
generateIndexPagination, generateTagPages, generateArchive,
generateNFTMetadata, settings`) and `templates/` holding at least
`blog.html`; `index.html`, `tags.html`, `archive.html`, `simple.html`,
`base.html` and `modules/*.html` are optional. `assets/` is copied into the
build; `assets/style.css` gets `style_css_sha256` in the context.

The grammar, measured across all six Planet templates (4,754 lines):

- Tags: `if` / `elif`-free `else` / `endif`, `for x in y` / `endfor`,
  `block` / `endblock`, `extends`, `include`. No `forloop`, no `where`,
  no `with`, no `set`.
- Expressions: dotted access, `dict['key']` subscript, Swift's `.count`
  on strings and arrays, truthiness (empty string, zero, empty array,
  nil are false), `==`, `!=`, `<`, `>`, `and`, `or`, `not`, string and
  number literals.
- Filters, none with arguments: `escape`, `escapejs`, `formatDate`
  (medium date and time), `formatDateC` (`yyyy-MM-dd'T'HH:mm:ssZZZZZ`),
  `ymd` (medium date), `mdyydot`. Stencil does not autoescape; neither do
  we.
- `include` paths are relative to the template's `templates/` folder;
  `./x.html` means the same folder.

Context, as the templates read it: `planet` (the public planet fields plus
social and Plausible), `article`, `articles`, `article_summary`,
`content_html`, `page_title`, `page_description`,
`page_description_html`, `assets_prefix`, `site_navigation`,
`archive_sections` with `archive[section]`, `tags`, `tag`, `tag_value`,
`tag_articles`, `current_page`, `total_pages`, `next_page`,
`previous_page`, `build_timestamp`, `social_image_url`, `og_image_url`,
`style_css_sha256`, `custom_code_head`, `custom_code_body_start`,
`custom_code_body_end`, `template_settings`, `user_settings`,
`current_item_type`, `has_avatar`, `has_podcast`, `planet_ipns`.

`user_settings` also carries `<key>Filter` for every setting ending in
`Color`, a CSS filter chain that tints a black glyph to that colour. Sepia
and Memories read `highlightColorFilter` and
`foregroundSecondaryColorFilter`. Port Planet's `ColorFilter.swift` solver
and pixel-check it against a Planet build of Sepia.

Templates ship vendored under `templates/` in this repo and are copied into
`Workspace/Planet/Templates/<name>/` when missing or older by `buildNumber`,
never over a copy that holds a `.git` directory. Croptop is not shipped: it
is an NFT minting UI.

**Platinum** (`templates/platinum`, 2026-09-25) is ours, the seventh,
published the same day as
[Planetable/SiteTemplatePlatinum](https://github.com/Planetable/SiteTemplatePlatinum)
and bundled as the `platinum` submodule of PlanetSiteTemplates (tag
0.9.0); the vendored copy here is that repo's checkout without `.git`,
so a change goes to the repo first (its pre-commit hook sets
`buildNumber` to the commit count) and is rsynced back. Every
page one window on the desk gray, drawn as the exe desk and its homepage
draw theirs. `assets/chrome.css` is a verbatim copy of exe-stats'
`chrome.css` (the window, striped bar, sunken frame, status strip, push
button; re-copy, never edit), `assets/style.css` is the site's own: a
crumb strip at the top of every page but the index, the site's avatar and
about in the head, posts as ruled lines with the date at the far end, the
manual's 12px type for a post with a black terminal for code and a sunken
field for a picture on its own, tags under the text, the button row
(Home ringed off the index, Archive, Tags when any, the site's pages,
RSS) and the status strip (counts and Newer/Older on the index, the site
and the date on a post). It reads the same context as Plain, uses no
`elif` (the engine has none) and no summary on the index (Planet's
article maps carry none). `TestPlatinumTemplate` builds every page kind. Its `assets/rss.xsl`
styles the feed: a template that ships that file gets
`<?xml-stylesheet type="text/xsl" href="assets/rss.xsl"?>` written into
`rss.xml` and `podcast.xml` after the XML declaration (the builder's
one rule; Planet's templates ship none, so their goldens hold), and a
browser opening the feed draws it as one more window, the posts as the
index lists them and the address to paste into a reader. Chromium 151
still transforms it, with a warning that every browser is dropping
XSLT; a reader never sees the stylesheet, so the feed loses nothing
when that day comes.
The live `exe` site (slug `exe`, avatar = the desk's icon) wears it; its
first post is "Meet exe".

**Paper** (`templates/paper`, 2026-09-28, Livid asked for a super
minimal template on a grey noise ground in Google's Noto Serif, for
literature and the humanities, with hub replies as Platinum has them;
I first called it Xuan, after xuan paper, and Livid renamed it Paper so
one word names it and the hub frame's look) is ours, the eighth. One
column (34rem, about 34 Chinese characters a line) of ink on a grey
sheet whose grain is an SVG `feTurbulence` tile in the CSS, so it stays
fine at every ratio; no boxes or shadows, hairlines only where a line
says something; light and dark by `prefers-color-scheme`. Its one
setting, **Language** (`zh-Hans` default, `zh-Hant`, `ja`, `en`; missing
or anything else reads as `zh-Hans`, since Planet's template preview
passes no settings), is the page's `lang`, and everything follows it:
the Google Fonts link (Noto Serif variable, plus Noto Serif SC, TC or
JP), the font stack (the CJK face first on a CJK site, so full-width
punctuation and the long dash come from it; Noto Serif first on an
English one), the words around the text from `modules/w_*.html` (one
file a word, the four languages as nested `if`s, since the engine has
no `elif` and no variables), and the dates. A post's date is `<time
datetime>` from `formatDateC` with `ymd` inside as the fallback; the
page script (`modules/script.html`, inline at the end of the body)
rewrites it with `Intl.DateTimeFormat` in the site's language, the day
read off the string so no reader's zone moves it, as figures
(2026.09.28) in lists and the month from the first date under an
archive heading. The same script marks a block written in Latin letters
alone `lang="en"` on a CJK site, so it sets in Noto Serif, ragged
right, with italics, instead of justified with CJK rules (an English
post on a Chinese site opened rivers). On an English site it marks the
other way round (2026-10-06, Livid: "fix the same gap in the paper
template", after the hub's frame got it in exe-hub 076ca7c): a block
that is mostly Chinese or Japanese — more of those characters than
Latin words, so a sentence that only names 宣纸 stays English — is
`lang="zh"`, or `"ja"` when a tenth of them are kana, and is set in the
CJK face first (SC, the one an English site loads; JP when the reader
has it), Medium, justified, with the emphasis dots, where it had
inherited the site's Regular in Noto Serif's fallback and slanted its
emphasis. An English block inside a marked one gets the site's language
back; a block the writer gave a `lang` keeps it; CJK sites are as they
were. CJK typography: justified text,
`line-break: strict`, `text-autospace` (never inside a date), emphasis
as 着重号 under Chinese and sesame dots over Japanese, a quotation set
in from both sides, a section break as three dots; figures in Noto
Serif for its tabular numerals. The text is Medium (500) on a CJK site
and Regular in Latin letters (`--weight`): Noto Serif SC's horizontal
strokes are 33/1000 em at every weight, 0.6px at 18px, so at Regular a
Chinese page read grey (Livid, 2026-10-06); and the light sheet keeps
macOS's own smoothing, since `-webkit-font-smoothing: antialiased`
thinned every stroke further there (the dark sheet keeps it). Under an announced post:
`modules/replies.html` (a heading, the composer, then the frame asked
for as `?look=paper&lang=<the site's language>`, 56px held for the
hub's empty line so nothing moves when it answers) and
`assets/replies.js`, Platinum's composer with every word in a table in
the four languages (the words the page starts with come from `w_*`
modules, so nothing flashes) and the scroll-into-view on the composer
itself; a fix to one copy goes to the other. `TestPaperTemplate` and
`TestPaperReplies` build every page kind in all four languages;
`~/tools/playwright/exe-planet-paper-test.js` walks the replies path on
scratch daemons (words in Chinese, the gate, a live reply, an English
answer, a reply to it, the held height, DPR 1, 1.5, 2, dark and a
phone); PlanetSiteTemplates' `swift test` passed with `paper` in its
Resources on the Mac (2026-09-28, a throwaway clone). Published
2026-10-06 on Livid's word as
[Planetable/SiteTemplatePaper](https://github.com/Planetable/SiteTemplatePaper)
(its history is this folder's three commits, buildNumber 1 to 3, so
buildNumber is the commit count as Platinum's is) and bundled as the
`paper` submodule of PlanetSiteTemplates (5d359b0, tag 0.10.0), whose
`swift test` passed with it in a Linux Swift 6.1 container (the
package's `import os` stubbed in the throwaway copy). From here Paper
ships as Platinum does: the repo first, rsynced back into
`templates/paper`, then the submodule bump and tag. In practice a change
is made and proved here first, and its commit replayed onto the repo
when Livid says push, one repo commit a buildNumber: the CJK blocks on
an English site and the reply's account went out that way the same day
(SiteTemplatePaper ffd5515 and a03c34a, buildNumber 4 and 5;
PlanetSiteTemplates 86d6d22, tag 0.10.2).

**To-do lists** in Platinum and Paper (2026-10-06; Livid: "sepia template
has a sophisticated impl for displaying and interact markdown todo list,
port its core functionality to paper and platinum"). Sepia's is two
halves. The template hides each `- [ ] words` item's checkbox, draws its
own mark in front of the item, and on a post's page gives the item the id
`todo-item-N`, counted from one in reading order, with only the mark
taking a click (`pointer-events`). The Planet app, in its article view,
injects a click handler: a click whose target id begins `todo-item-`
flips `#id > input[type=checkbox]` and tells the app, which flips the
N-th line of the Markdown that opens `- [ ] ` or `- [x] `, saves and
rebuilds. Both templates now carry the template half, in
`modules/todo.html` (the same script in each, included by `blog.html`
after the post) and their stylesheets, each in its own hand: Platinum
the desk's OS 9 check box (the Todo app's, copied: 12px, black border,
sunken bevel, the crisp pixel check) with the done words struck in grey;
Paper a 13px hairline box on the middle of the first line, a pen's tick
in ink through it and past its corner once done, the done words in the
quiet ink. The box stands in the bullet's place; a numbered item keeps
its number and takes the box inside. Where it departs from Sepia, on
purpose: an item's own checkbox decides (`li:has(> input)`), so a bullet
that only holds to-do items below it draws no mark of its own; the ids
go to the items Planet's count sees — bullets at the margin
(`.content > ul`) — so a nested, numbered or quoted item never ticks the
wrong line, it is simply not ticked from the page; a list with blank
lines between its items has its checkbox moved out of the paragraph to
be the item's first child, where Planet looks; and the item's words go
in a span and every child keeps its pointer events, so a link among the
words is still a link and the words still take a selection, which
Sepia's `pointer-events: none` on the whole item loses. All of it sits
under `@supports selector(li:has(> input))`; elsewhere the checkbox
stays as it comes. On the web nothing listens and the marks are still.

exe's own Planet app listens as Planet does (2026-10-06, Livid: "wire
the click in exe's planet app"). Every page the window's page column
shows carries a second script beside the links' (`todoBridge`,
internal/service): a click whose target id begins `todo-item-` flips
that item's checkbox at once and posts `{exe: "page-todo", item, done,
was}` to the window. `item` is the script's own count, not the id's
number: the bullets at the margin with a checkbox of their own, from
one, so Sepia's pages, which id other items too, are counted the same
way and a click on one of those others does nothing. `was` is how every
such item stood before the click, "1" for done. The window sends it on
as `POST /v1/sites/{site}/posts/{post}/todo?kind=` and the daemon
(`Service.TickTodo`) finds the item in the post's file with the parser
the page was written with (`markdown.TodoItems`: the task items of the
document's own unordered lists, each with the offset of the mark
between its brackets), checks the file's items stand as `was` says, and
changes that one byte; the watcher builds as after any edit. A file
that has moved on from the page — edited elsewhere while the build was
on its way — answers 409 and nothing is written: the window draws the
page again from the build, so the tick comes off, and says nothing,
since the build on its way brings what the file says; any other failure
is an alert. The count is by structure, not by Planet's rule of lines
opening `- [ ] `, so a `*` bullet, a capital `[X]` and a line inside a
code block are right here where Planet's count goes astray.

The build that follows a tick must not redraw what the frame already
shows. The window keeps the page as it was given and how its items
stand with the frame's ticks; a build's page that is the same but for
what is ticked, and ticked as the frame is, is taken without touching
the frame, and one that still lacks a tick made in the last ten seconds
is waited out (it was built before the tick; the next one has it). A
page that did change is drawn again where it was read to: the page says
how far down it is (`scrollBridge`, `{exe: "page-scroll", y}`), and the
window ends the new page with a line that scrolls there before it
paints, so it never shows at its top first. Sepia stamps every build
into its pages, so there the frame is always drawn again, in place.
A link to a place in the page itself (a footnote) is now followed in
the page: left to the sandboxed frame it went to `about:blank#…` and
the page was gone (found by the test's link in a to-do item).
`~/tools/playwright/exe-planet-app-todo-test.js` opens the live app on
its own against a scratch daemon and clicks boxes in the page column on
a Paper, a Platinum and a Sepia site: the tick, the request, the one
byte, the frame left alone or put back in place, a page behind its file
refused without a word, and the words, a link and a nested box doing
nothing.

`~/tools/playwright/exe-planet-todo-test.js` builds one post on a
Platinum site and on Paper sites in Chinese and English and walks each
in Chromium and in WebKit (Planet's view is a WKWebView) with Planet's
own click script read out of `/www/planet` and its `toggleToDoItem`
run over the Markdown: the marks, the ids, a click on a box naming its
item and flipping its own line, and a click on the words, a link, a
nested or a numbered box doing nothing. Paper buildNumber 6, Platinum
11, pushed on Livid's word the same day (SiteTemplatePaper 703f3f6,
SiteTemplatePlatinum fe9fa9b, PlanetSiteTemplates c181017, tag 0.10.3;
the Mac's checkout pulled to it, `swift test` passing there).

### What parity taught, so it is not relearned

- Stencil splits a tag into components with `smartSplit` and
  `appendWord`: a component ending in `,`, `|` or `:` absorbs the next
  word, so `{% if a == 0, b > 0 %}` compares `a` with the variable
  `0,b`, which is nil, and the condition is false without an error.
  Memories relies on it.
- A dictionary in a `for` loop is sorted by key and each item is a
  (key, value) tuple destructured by position: one variable gets the key.
- `article.created.timeIntervalSince1970` reaches the Date through
  Key-Value Coding and comes back as an NSNumber, which prints with
  sixteen significant digits, not Swift's shortest round trip; the sum
  is done in doubles on Planet's own 2001-epoch value, so the engine
  carries that value, not a time.Time.
- `.count` on a string counts grapheme clusters.
- `escape` is Kitura's `htmlEscape()`: `& < > " '` and every non-ASCII
  scalar become upper-case hex references.
- The feed description goes through SwiftSoup: bare `&` becomes
  `&amp;`, stray structural tags vanish, every void element ends in
  ` />`, boolean attributes are bare. `absoluteImageURL` is that pass
  with the image sources rewritten; only the re-indenting is not copied.
- cmark-gfm ends an HTML block with a newline even without one in the
  source, reads CR LF as LF, links an email only when its domain ends in
  a letter, and writes a task box as `<input type="checkbox" disabled="" />`.
- Planet writes `templateSettings.json` into the site only when the
  template has settings or the site already had the file.
- Planet's colour-filter solver is stochastic, so its chains are not
  compared; a build reuses Planet's cached values when it has them.
- A golden is only as consistent as its last rebuild: `planet.json` is
  rewritten every publish, an `article.json` only when that article is
  saved, `Public/assets` on every save, and article pages only on a
  rebuild. `article.json` is therefore compared with the article's entry
  in `planet.json`, hashes and filter chains are normalised, and a
  golden whose pages predate the Mac's template carries a `stale.txt`
  until it is rebuilt.

## Markdown

goldmark with the GFM extension (table, strikethrough, linkify, task
list), footnotes, raw HTML allowed and hard line breaks, matching Planet's
cmark-gfm options (`Planet/Helper/MarkdownUtils.swift:189-205`). The
renderer is one interface, `Render(markdown) (html)`, so the end-game
hand-written renderer can replace goldmark when it passes the corpus: every
article on the Mac carries both `article.md` and `contentRendered`, which is
hundreds of golden pairs for free.

## Daemon

- `cmd/exe-planet`, flags `-listen 127.0.0.1:7799`, `-workspace
  ~/.exe/workspace/Planet`, `-state ~/.exe/planet`, `-kubo
  http://127.0.0.1:5001`, `-exe http://127.0.0.1:7777` and `-exe-config
  ~/.exe/config.json`, read for the exe token and `cloudflare.domain`
  (an empty `-exe` or `-kubo` is a node without that). Every test passes
  its own scratch directories; nothing derives a test path from `EXE_HOME`
  or `HOME`, and nothing under `~/.exe` is touched by a test.
- Watches the Workspace by polling, not inotify (`internal/watch`): a
  stat walk every second, a change reported once two polls agree, hidden
  paths skipped. No watch limits, no dependency, and a tree of a few
  thousand files costs milliseconds. A post edited in the desk text
  editor, by an agent after a Terminal drop, or on a synced peer is seen
  the same way. Phase 1 logs changes and counts them in `/v1/status`;
  phase 2 hooks the builder to the same callback.
- Every API call reads the folder afresh; there is no cache to go stale.
  A folder whose `site.json` or `index.md` cannot be read is listed with
  an `error` field, never dropped. A body is stored as a text file and
  comes back ending in one newline. `DELETE` on a site or post removes the
  folder; archiving is `POST …/archive` and `…/unarchive`, a move into and
  out of `Archived/`. Attachments are multipart parts named `file`, kept
  under their base name; names starting with `.` or `_` are refused
  because `_grid.png` and friends are build output.
- API, JSON, loopback only, authenticated by exe's relay in front:
  - `GET /v1/sites`, `POST /v1/sites` (title, slug, template),
    `GET|PUT|DELETE /v1/sites/{slug}`
  - `GET /v1/sites/{slug}/posts`, `POST /v1/sites/{slug}/posts`,
    `GET|PUT|DELETE /v1/sites/{slug}/posts/{slug}` with
    `?kind=post|page|draft`, `POST …/move`
  - `POST /v1/sites/{slug}/posts/{slug}/attachments`, `DELETE …/{name}`
  - `POST /v1/sites/{slug}/preview` renders a body through the site's
    template without saving and answers text/html (an existing slug
    lends its id and attachments); `POST /v1/sites/{slug}/build` builds
    now and answers the status; `GET /v1/sites/{slug}/status` reports the
    last build, whether one is running and whether another is queued;
    `GET|PUT /v1/sites/{slug}/node` is the site's publishing on this node
    (`{expose, subdomain, ipfs}` in, the node file plus `url`, `ipns`,
    `ipns_url`, `cid_url`, `publishing`, `key`, `domain`, `exe`, `kubo`
    out), `POST /v1/sites/{slug}/publish` (202) goes to IPFS now,
    `GET /v1/sites/{slug}/export` is the zip, `GET /v1/node` says what the
    node has (`domain`, `exe`, `kubo`, `kubo_version`/`kubo_error`)
  - `GET /v1/sites/{slug}/avatar` (the folder's `avatar.png`, 404 without),
    `GET /v1/sites/{slug}/posts/{slug}/view?base=&query=` (the built page
    with its resources inlined; with a base, the article's own files are
    fetched from `<base><name>?<query>` instead), `POST /v1/render
    {path, text}` (a Workspace path and its text in Planet's Writer
    preview template, the folder's pictures folded in or fetched from
    `base`)
  - `GET /v1/templates` (each with its `settings` from template.json),
    `GET /v1/templates/{name}`
  - `GET /v1/import?library=<path>` lists the planets in a Planet library
    folder on this machine (or the one planet of a `My/<uuid>` folder);
    `POST /v1/import {library, id, slug}` brings one in and builds it
  - `GET /sites/{slug}/…` serves `public/` for the preview frame; a request
    whose `Host` matches an exposed hostname is served from that site's
    `public/` at the root, so exe's route backend is plain
    `http://127.0.0.1:7799`.
- Runs as the systemd user unit `exe-planet`, restarted with
  `XDG_RUNTIME_DIR=/run/user/1000 systemctl --user restart exe-planet`,
  never touching exe.

## exe integration, one time

Two additions to exe, after which exe never restarts for this project:

1. **Service relay.** A config map of service name to local URL, served
   under `/v1/svc/<name>/…` with the usual bearer token, generalising the
   Hub app's `/v1/hub/relay/` (`internal/server/hub.go:85`). The desk
   cannot reach `127.0.0.1:7799` over Tailscale Serve or from a phone.
2. **Route registration for a local backend.** `publishHost` in
   `internal/server/server.go:664` is generic; only VM expose and `exe
   site` call it. `POST /v1/routes {host, backend}` and `DELETE` let
   exe-planet register `charts.<domain> -> http://127.0.0.1:7799`, with
   Cloudflare DNS and tunnel ingress handled as today.

Config: `services: {"planet": "http://127.0.0.1:7799"}` and `apps_dirs`
gains `/www/exe-planet/apps`, so the `Planet` app folder is served live
from this checkout with no daemon code.

Built and applied on 2026-09-25 (exe commit 0454005). As shipped:

- `/v1/svc/{name}` and `/v1/svc/{name}/{path...}` for GET, POST, PUT,
  PATCH and DELETE; the app calls `/v1/svc/planet/v1/sites`. The token
  and cookies stop at exe; the service's `Set-Cookie` and CORS headers are
  stripped; nothing is sandboxed. Unknown service is 404, unreachable is
  502 with the dial error.
- `POST /v1/routes {"host", "backend"}` where backend is an http(s) origin
  with no path; answers the same `host, backend, url, dns, ingress,
  warnings` as a VM expose. `DELETE /v1/routes/{host}` removes it. CLI:
  `exe expose <host> -backend http://127.0.0.1:7799`.
- The live daemon runs with an empty `api_token`, so its `/v1` routes,
  the relay included, are open to whoever reaches its Tailscale address.

## The app

Two apps in `apps/`, each `app.json` plus one `index.html`, Mac OS 9
Platinum per `/www/exe/docs/platinum.md`, vanilla JS, no CDN, every UI
change screenshotted at device pixel ratios 1, 1.5 and 2 and on a phone.

**Writer** edits any Markdown file in the Workspace, not only a post: two
columns, the text in a field on the left and on the right a frame holding
the page `POST /v1/render` makes of it, debounced. The page is Planet's
own Writer preview, `WriterBasic.html` vendored into
`internal/service/writer.html` (MIT): the rendered body in a `.content`
div, front matter stripped, no title and no site chrome, just as the Mac
app previews while writing; the site's real template is the Planet
window's page column. The preview scrolls with the field (Planet's
`scrollPosition(percent)`, here a `preview-scroll` message into the
sandboxed frame, sent again when a fresh rendering says `preview-ready`,
so retyping never jumps the page back to the top). It
saves the file through exe's `PUT /v1/workspace/<path>` as you type, with
`X-Exe-Seq` and `X-Exe-Client`, and keeps an unsaved edit when the desk
says the file changed elsewhere (a clean file reloads). A dropped picture
lands beside the file and is inserted as `![name](name)`. A phone shows
Edit or Preview.

**Planet** is three columns like the Mac app's window. The sites sidebar
sits on the window colour (`#ccc`), each row the site's `avatar.png` at
24 by 24 (a pixel globe when there is none), archived sites under their
own heading. The article list is the Finder's list view from the HIG: a
list box (fig. 2-25: white, 16px rows, 1px black frame) under a window
list view header (fig. 2-43: 18px face, columns Title, Date and Kind, the
sorted column's face darker, `#888`, a 1px black line beneath), rows
sorted newest first, arrows, Home and End, type-select, Return to edit,
Delete asks. The page column is the built page of the chosen article,
`GET /v1/sites/{slug}/posts/{slug}/view` with the stylesheets, scripts and
pictures folded in so a sandboxed frame shows it without a path. The
frame's base is the app's own address, so a page's relative links would
land on `/apps/…` (Livid saw exe's 404 from Home, 2026-09-25); every
viewed page therefore carries a script that hands a clicked link to the
window as `{exe:"page-link", href, external}`, and the app resolves it
against the page's path: a post's folder picks the post in the list, a
feed or a file opens in a tab through the relay, another page of the
site (`GET /v1/sites/{slug}/view?path=archive.html`) shows in place with
no post chosen, an outside link opens in a tab. The templates keep
plain relative links; `TestPlatinumTemplate` refuses a root-relative
one. A row of
buttons above at the dialog guidelines' numbers (New Site…, Import…, New
Post, Edit, Site…, Publish…), contextual menus on a site row (Settings,
New Post, Archive, Delete) and an article row (Edit, Copy Path, Move to,
Delete), sheets for New Site (slug follows the title, validated live),
Site Settings (name, about, template, author, links, Plausible, robots,
the template's own typed settings) and Import. The status line reports the
last build and the article counts and polls while the window is shown.
Editing hands the post's `index.md` to the Writer through the desk
(`{exe:"open-app", app:"Writer", path}`); the desk forwards every
Workspace change (`workspace-changed`) so the list follows edits made in
the Writer, by an agent or on a peer. A phone shows one column at a time
with a Back button.

## Publishing

Per node, in `<state>/<id>/node.json` beside the build, never in the
Workspace: a site's folder travels with the site, where it is served from
does not. Built 2026-09-25 (0.5.0).

- **Change detection.** Every build hashes what it was made from (the
  site folder, the template folder, the daemon version) into
  `Status.hash`; a publish happens only when that moved since the last
  one. The output is not compared because Sepia and Memories stamp
  `build_timestamp` into their pages.
- **An exposed site builds with its hostname** as Planet's `domain`
  (`input()` reads the node file), so the feed's `<link>`s, the Open
  Graph image and every absolute URL carry `https://<hostname>/`
  instead of the IPNS gateway form, which with no name yet read
  `https://eth.sucks/ipns//`. The hostname is part of the input hash,
  and a route change rebuilds at once.
- **Expose** (`expose`, `subdomain`) asks exe for a route,
  `POST /v1/routes {host: <sub>.<zone>, backend: http://127.0.0.1:7799}`,
  with the sub defaulting to the slug and the zone read from exe's
  `cloudflare.domain`; exe's DNS and tunnel warnings are kept in
  `route_warnings`. The daemon then serves that site's `public/` at the
  root of any request whose `Host` is the hostname (exe's proxy keeps the
  public Host). Off, a new sub, a delete or an archive send
  `DELETE /v1/routes/{host}`. The route lives in exe, so it survives
  either daemon's restart; the daemon rereads its node files at start.
- **IPFS** (`ipfs`) on the first turn-on makes an ed25519 key (the
  site's one key, made by whichever switch needs it first, IPFS or
  Announce; see Replies from the hub), keeps it
  as PKCS#8 PEM in `<state>/<id>/ipns.key` (0600), lends Kubo a copy
  through `key/import?format=pem-pkcs8-cleartext` as `exe-planet-<id>`
  (a copy already there is checked to be the same key), writes the IPNS
  name into `site.json` (derived by hand from the public key: protobuf
  PublicKey, identity multihash, CIDv1 libp2p-key, base36, checked equal
  to Kubo's answer) and rebuilds, since the pages carry the name. Every
  changed build then adds `public/` over RPC as a pinned CIDv1
  directory, publishes it to the key with a 7200 h lifetime and
  allow-offline as Planet does, unpins the CID before it, and records
  the CID, the time and the hash. Kubo republishes its keystore itself.
  Off keeps the key and the name, so on again is the same site.
- **Publish Now** (`POST …/publish`) goes again, changed or not; one
  publish runs at a time per site, a request during one runs once more
  after. A failure is kept in `publish_error` beside the last good CID.
- **Export** (`GET …/export`) is a zip: `<slug>/` (the Workspace folder),
  `<slug>.ipns.key` and `<slug>.node.json`, which is how a site takes its
  publishing to another node.
- The app's Publish… sheet holds both switches, the address field, the
  URL, the IPNS name, the last CID with its time, Publish Now and Export…;
  a switch is dead, with the reason beside it, on a node without exe, a
  zone or Kubo.

## Replies from the hub (built 2026-09-26)

Publish a post and it announces itself on the exe hub; the replies it
gets there show under the post on the blog, live. My idea post on the
hub (`d89824f7`, 2026-09-25), worked out with Livid 2026-09-26. The
hub's side (invites, the replies frame) is in `/www/exe-hub/PLAN.md`,
"Replies under a blog post". Built the same day (exe-planet 0.6.0,
`internal/service/hub.go`; Platinum buildNumber 7); the live `exe` site
is not switched on yet — that and Meet exe as the first post are
Livid's say.

The day it lands: move a draft into `posts/`; a minute later the post
is on the hub with its link card, posted by the site itself; replies
made there show in a Replies window under the post, in the reader's
language, arriving live; a holder of 10,000 $V2EX answers from the Reply
window on the same page, signing with their wallet.

### One key per site

- The site's ed25519 key (`<state>/<id>/ipns.key`) is one public key in
  three spellings: the **hub id** (the hub's `Fingerprint`, 16 hex), the
  **IPNS name** (protobuf key, identity multihash, base36, `k51…`) and the
  **Solana address** (the raw 32 bytes in base58). The IPNS name holds the
  public key, so `ipns` in `site.json` is enough to derive the other two;
  `site.json` gains no field.
- What that buys: the hub profile, the IPNS site and a tip address are
  provably the same owner, with no link to trust; the site speaks on the
  hub as itself, with its own name and picture; and only the node holding
  the key can announce, so two synced nodes cannot both announce a post.
- Signatures do not cross over: the hub signs `exe-hub:v1\n` and the
  envelope, IPNS signs `ipns-signature:`-prefixed data, Solana signs a
  transaction message no such text parses as. The hub's wallet sign-in
  already rests on a Solana key signing hub messages.
- The key is made on the first switch that needs it (Announce, IPFS,
  later Tips), not at site creation: a site that never publishes needs
  none. The file keeps the name `ipns.key`.
- **An imported Planet site** has its identity in the key Planet holds on
  the Mac (`ipfs key gen <name>`, no type given: Kubo's default, ed25519,
  to be checked on one of Livid's planets). A new key would lose its
  Planet followers, so it announces only once the `.planet` key import
  (Later) brings that key; until then Announce is dead with the reason.
- Custody: the key lives in three places (`ipns.key`, Kubo's keystore,
  every Export zip). Kubo's RPC has no `key/export`; its `key sign`
  prefixes what it signs, which is to be verified on a probe key before
  a Solana balance ever rides on the key.

### Announcing

- **A switch per node**, `announce` in `node.json` with `hub`, the one hub
  address the site uses (`HubOrigin`: http(s)://host, nothing after): it
  posts there and its readers' pages frame that hub. One hub, so the post
  is there when a page first frames it and the invite is needed on that
  hub alone; peering carries the post to the other hub. The node answers
  `hub_reason`, why it cannot be switched on, in this order: no exe, a
  site whose IPNS name is a key this node does not hold (imported), not
  exposed (a link card needs a public URL), no hub address; the server
  refuses PUT with the reason. The Publish… sheet's Hub group: the
  switch, the Hub field with a line saying the node's key invites the
  site's, Id (the hub id · the Solana address, or when the key is made),
  and Now (Off / On: each post published from now on goes to … /
  Announcing… / Failed: …). The switch and field are dead only for the
  first two reasons: expose and the address can be set in the same sheet.
- **Turning it on** makes the key if there is none, then asks exe to
  invite it: `POST /v1/hub/publish {hub, type: "invite.set", body:
  {target: <site pubkey>, note: "Site: <name>, <hostname>"}}`, signed by
  the node's key, an admin on that hub. The sheet says so beside the
  switch. Then the site's `profile.set`: name = the site's name, avatar =
  `avatar.png` minted by that hub's `POST /v1/avatar`, bio = the about
  and the site's address; sent again whenever those change. Turning it
  off stops announcing and keeps the invite (the site's profile and posts
  stay on the hub); deleting the site sends `invite.lift`.
- **exe-planet signs its own envelopes** (`GET /v1/seq`, sign
  `exe-hub:v1\n` + the envelope, `POST /v1/msg`), not through exe's
  `/v1/hub/publish`, which signs with the node's key: the site's key
  never leaves this daemon. A hub envelope is a few dozen lines, no
  dependency.
- **When.** After a build is swapped in (`afterBuild`), one announcing
  pass per site runs (`startAnnounce`, publishing's running/again
  pattern): the profile if it moved, then every post under `posts/` with
  no thread, oldest first. The announcement is the title in bold, the
  post's opening paragraphs (`leadText`: whole paragraphs up to the first
  heading, list, code or picture-only line, while they fit in about 280
  characters; Planet's summary, the whole page's text cut at 280, ran the
  first post's heading into its sentence), and its address on the
  exposed hostname last, the first link, so the hub's link card
  comes from the live page's Open Graph tags. The hub's cooldown (60 s,
  which an invited key keeps) answers 429 with Retry-After: the pass
  stops and a timer brings it back after that, so a queue drains one post
  a cooldown; a cooldown is not shown as an error. Any other failure is
  `announce_error` and a retry in 5 minutes or at the next build. Drafts
  are never announced; pages (`pages/`) neither; an edit never announces
  again; deleting or archiving a post leaves its thread alone, since
  other people's replies are in it.
- **Older posts.** Switching on marks every post already published
  `before` in the ledger, so nothing is announced by itself; the Planet
  app's post menu has Announce on Hub (`POST
  /v1/sites/{site}/posts/{post}/announce`, 202), dead for a page, a
  draft or a post that has its line. Meet exe is the first, on Livid's
  say.

### Where the thread id is kept

Every post keeps the whole 64-hex id of its announcement (`reply_to`
takes whole ids only, and the frame is built from it), in two places:

- **The front matter is the record**: `hub: https://<hub>/p/<id>`. It
  travels with the post (sync, Export, a copy in the Finder, a folder
  rename) so every node builds the same Replies window; a person may edit
  it (point an old post at a thread that exists, or delete the line to
  take the window off). It is written by the daemon the way `ipns` is
  written into `site.json`.
- **A node-state ledger is the guard**: `<state>/<id>/hub.json`, keyed by
  the post's `id` (the UUID, never the slug: a rename must not announce
  again), each post `before`, `want` (asked by hand), `pending` or
  `announced` with its thread. Before sending, the envelope and its
  signature are kept as `pending`, and a retry sends those same bytes: a
  post that landed is a duplicate to the hub, which answers the same id,
  so a crash anywhere between signing and the file write never posts
  twice. A pending envelope the hub refuses for its seq (409) never
  landed, since a landed one would be a duplicate, so it is signed anew.
  The Writer keeps an unsaved edit over a changed file, so its next save
  can drop the line; the next pass sees the ledger entry without the line
  and writes it back (`Store.SetPostHub`, which leaves `modified`
  alone), and a build in between reads the thread from the ledger.
- A post counts as announced when either place has its id; a line the
  ledger does not know is learned into it.
- The ledger also keeps where the invite went (`invited`: a hub that
  changes is invited again and hears the profile anew), the avatar's
  hash and CID, and the hash of the profile last sent, so `profile.set`
  goes only when the name, the bio (the about, then the address) or the
  avatar moved.

### The Reply and Replies windows

- The builder gives the template a top-level `hub_thread` with `link`
  (the thread page), `frame` (`<hub>/p/<id>/replies`), `hub` (the
  origin) and `id` (the root) when the front matter has `hub:`. It stays
  out of `article.json` and `planet.json`, so the output keeps Planet's
  shape, and real Planet, which never sets it, draws nothing.
- Platinum grows `modules/replies.html`: under the post's window a Reply
  window (the composer), then a Replies window, as the hub's thread page
  puts Reply above the thread. Both shapes are there from the first
  paint, so the post never moves; the Replies window is the last thing
  on the page, so its growth pushes only the footer.
- **Replies** holds an `<iframe loading="lazy">` of the frame, its
  height from the frame's `{hub: "height", h}` messages, taken only from
  that frame's window and the hub's origin; its strip links the thread on
  the hub. A hub that cannot be reached leaves the placeholder.
- **Reply** is on the page itself (Livid, 2026-09-26: the composer on the
  same page as the post). It cannot live in the frame: Phantom injects
  its provider into no iframe and no http page, so only the top level of
  an https page reaches every wallet. The hub's plan has the reasons in
  full ("Answering from a blog").
  - The hub's Reply window in the template's Platinum blocks, its CSS
    copied from the hub's composer: signed out, a line and Sign in with
    Solana (a wallet picker when there are several); signed in, a who
    row (the face — the identicon on its pale ground when there is no
    picture — the name and id, linked to the hub profile) with Sign Out,
    the field, and a status line beside the Reply button. Reply wears no
    ring, as the hub's Post does: Return is a newline, Command or Control
    with Return replies. The head script classes `<html>` `js` and
    `wallet` so the window has its final shape before layout. It shows on
    every device: a phone without a wallet reads the line saying one is
    needed rather than a window that appears late.
  - `assets/replies.js`, loaded at the end of `modules/replies.html` (not
    in the head: `build.Inline` makes it an inline script for the Planet
    app's page column, where `defer` does nothing), only when
    `hub_thread` is set, and named by its hash
    (`replies.js?sha256={{ replies_js_sha256 }}`, the builder's, as
    `style_css_sha256` is; 2026-10-06): the file is served for four
    hours by its name alone, so a changed script reached a reader's
    browser, and the cache in front of the site, hours after the page
    that needed it, and each change wanted a purge by hand. The script: the Wallet Standard handshake with the
    `window.solana` fallback, as the hub's compose script does; `GET
    /v1/gate?author=` at sign-in ("This address holds less than 10,000
    tokens of 9raU…pump, so it can read but not reply.", the cooldown's
    countdown); then `GET /v1/seq`, `signMessage` over `exe-hub:v1\n` and
    the envelope (`post.create {text, reply_to}`), `POST /v1/msg`. The
    reply comes back through the frame, live.
  - An address check that cannot reach the hub is tried again, as on the
    hub's own pages (exe-hub's PLAN.md, Offline; Livid, 2026-10-08: "that
    gray dot pattern should also apply to comment embed"): a request that
    never reached it (offline — and the edge's own 502 while the hub
    restarts, which carries no CORS header, so the page sees a network
    failure) or a 502–504 with no word from the hub. Until one lands
    Reply stays disabled and the status line pulses a grey dot in place
    of words, the reason its tooltip and label: Platinum's the hub's 8px
    disc of pixels in the disabled grey, Paper's a round 7px dot in its
    faint ink (the night sheet's too), fading to a quarter and back each
    1.2 s, still under reduced motion; the box is the line's height, the
    dot centred in it, so the line keeps its height. Tried again 2 s,
    4 s, 8 s, then every 15 s, and at once when the browser says it is
    online or the page is shown. A press that never reached the hub says
    "Offline: the hub can't be reached…" in the site's language, not the
    browser's "Failed to fetch". `~/tools/playwright/exe-planet-retry-dot-test.js`.
  - A commenter is a 10,000 $V2EX holder, the hub's gate unchanged
    (Livid, 2026-09-26).
  - Reply to a reply: the page tells the frame `{hub: "signed-in", on}`
    (again when the frame says `ready`), the frame then shows a Reply link
    under each reply, and a click sends `{hub: "aim", id, name, words}`
    up; the composer shows the hub's "Replying to <name> — <words>" row
    with its clear button, and `reply_to` is that reply instead of the
    root. Pressing Reply first asks the hub for that reply (`GET
    /v1/post/{id}`): one deleted meanwhile is said so and nothing is sent.
  - Messages count only from the page's own frame window and the hub's
    origin; a page shown sandboxed (the Planet app's page column, an
    opaque origin) also hears its frame's "null", since every frame inside
    a sandboxed one is opaque too.
  - Signing in follows the hub's own pages, ported 2026-10-06 (exe-hub
    a9c65e2, 29311cc, 41fff4f; Livid: "port to both"). A wallet signed
    in on an earlier visit is asked nothing while the page is read: the
    remembered address is the account, so the gate is checked and the
    who row drawn with no wallet call, and the wallet is first asked at
    the first reply to sign — silently, then aloud if that brings
    nothing; one back on another account signs nothing, the window turns
    to that account and says so. The who row of a returning wallet is
    drawn by a script right after its markup from the name, id and
    picture the gate last gave (one look from the first paint, not "…",
    the short address, then the name). Sign Out leaves a mark
    (`exe-hub-signed-out`) and the next Sign in asks for a signature —
    Sign In With Solana where the wallet has it, else a connect and a
    "Sign in to <host>" message that never begins `exe-hub:v1` — since a
    wallet that still trusts the site (Glow) connects again without a
    prompt. The flow is the hub's: a fix there comes to both copies.
  - A reply is the account's that pressed the button (2026-10-06, from
    exe-hub 7eec742 and 27b677d; Codex on Spark held `/v1/seq` on both
    templates, turned the wallet from A to B, and B was asked to sign
    an envelope naming A; Livid: "Improve.", then "continue improve").
    The press keeps who it began as (`who`); `mine(who)` asks whether
    that is still `me` after the reply it answers is looked up, after
    the seq comes, before the wallet is asked and before what it signed
    is sent. Not theirs any more, the reply stops: nothing is signed,
    or what was signed goes nowhere, the words stay, and the status
    line says the wallet is on another account (`switched`, or
    `switchedSigned` when it had signed; Paper in its four languages).
    Sign Out stops it without a line. The window's cooldown and gate
    follow a reply's answer only while the window is still that
    account's. A remembered wallet being asked again (`resume`) looks
    between its steps too: after Sign Out it is asked for no connect
    aloud and no signature, whichever account it answers as, and the
    stop is silent, never "The wallet could not sign".
  - localStorage keeps the wallet's name and address (`exe-hub-wallet`,
    as the hub's pages do, with the hub id and the profile's name and
    picture once the gate answers), the Sign Out mark, and the unsent
    draft per post, every access in try/catch; nothing else. The draft is the words and the reply they
    answer, together (`{v: 1, text, reply_to}` under
    `exe-hub-draft:<root>`), so a reload brings the "Replying to" row
    back with the text, and a sign-out leaves the aim alone: it is the
    draft's, not the session's. Words alone make a draft — an aim with
    nothing written is not kept — and a draft an earlier build saved as
    bare words reads as words answering the post. An aim at a reply
    deleted meanwhile is refused at Reply by the check above, the words
    and the row kept until the reader clears it. (Codex on Spark found
    the words coming back without their reply after a reload,
    2026-09-26; Livid: "Improve it." SiteTemplatePlatinum buildNumber 8.)
  - The script is the template's own, fixed per build, not one the hub
    serves: a script from the hub on this page could ask the reader's
    wallet to sign anything if the hub were compromised. The cost is a
    second copy of the hub's wallet posting code; the envelope is the
    frozen `exe-hub:v1`, and one set of mock wallets tests both.
  - No wallet (Planet's own viewer, `window.PLANET.visitedFromPlanetClient`,
    or a browser without one): the status line says a Solana wallet is
    needed, Sign In… is dead, and the replies still read.
- It ships by the template routine: SiteTemplatePlatinum, then rsync into
  `templates/platinum`, then the PlanetSiteTemplates submodule and tag.
- Nothing is baked at build: a reply never costs a rebuild, and the IPFS
  copy carries the frame's address, not the replies.

### Tests

- Go (`internal/service/hub_test.go`): `TestAnnounce` against a fake hub
  in the test that checks every signature as the hub does, keeps seqs,
  answers duplicates with the same id and holds posts to a timed
  cooldown — the invite through the fake exe, what was there before left
  alone, the profile then the post, the hub line, Announce on Hub after
  waiting out the cooldown, a dropped line written back without a second
  post, a crash after the hub took a pending envelope, the profile sent
  again only when it moved, a draft never, the invite lifted;
  `TestAnnounceImportedSite`; `TestHubThreadTemplate` (the windows from
  a valid line only, the ledger standing in for a lost one, article.json
  unchanged). Run `-race` too: the announcer is goroutines.
- Playwright, `~/tools/playwright/exe-planet-replies-test.js`: the whole
  path on scratch daemons it starts (a real exe-hub on :17788 with a
  token gate on a fake RPC, exe-planet on :7798, a fake exe on :7796
  that signs as the hub's admin), mock Wallet Standard wallets: switch
  on, the invite, a draft moved to posts announced with its line, no
  wallet, a wallet below the gate, a reply that slides in live, the frame
  growing, an aimed reply to a reply nested under it, a draft aimed at
  a reply kept through a reload and a sign-out (the cross clears both,
  a bare-string draft from an earlier build still reads, a saved aim
  shows at once at every DPR), an aim at a reply deleted meanwhile
  refused at Reply with nothing signed, the page inlined into a
  sandboxed srcdoc frame as the app's column shows it, shots at DPR 1,
  1.5, 2 and a phone. `exe-planet-announce-sheet-test.js`: the
  Hub group and Announce on Hub on the live desk against a scratch
  daemon, every PUT and DELETE to the live daemon swallowed.
  `exe-planet-signin-test.js`: the sign-in on a Platinum site in English
  and a Paper site in Chinese (a remembered wallet asked nothing, the
  who row in every frame, Sign in after Sign Out).
  `exe-planet-switch-test.js`: on the same two sites, the hub's answer,
  the wallet's prompt and its reconnect held in turn while the wallet
  turns to another account or Sign Out is pressed; it fails on the
  templates before the port (`TEMPLATES=<dir>` points it at another
  folder).
- Real Stencil: PlanetSiteTemplates' `swift test` on the Mac renders the
  new module (SiteTemplatePlatinum cf39614, PlanetSiteTemplates 423e955,
  tag 0.9.4). The draft fix and the hub's sign-in followed on
  2026-10-06 (SiteTemplatePlatinum 2232e19, buildNumber 9;
  PlanetSiteTemplates b16b0b6, tag 0.10.1), `swift test` passing in a
  Linux Swift 6.1 container with Paper beside it, then on the Mac (a
  throwaway clone of the tag), whose ~/Developer/PlanetSiteTemplates
  is pulled to 0.10.1. The reply's account and the hashed script
  followed (SiteTemplatePlatinum b581c2e, buildNumber 10;
  PlanetSiteTemplates 86d6d22, tag 0.10.2, with Paper's two), the Mac's
  checkout pulled to it and its `swift test` passing there.

### Later

- Profile… in the Reply window, so a commenter can take a name and a
  picture without visiting the hub.
- Pictures in a reply from the blog (each one is another signature).

## Import

Done 2026-09-25 (`internal/importer`). Reads a copied Planet library:
`My/<uuid>/planet.json` and `Articles/*.json` for the text and metadata,
`Public/<uuid>/<article>/` for the attachments, since Planet keeps
attachments only in the built site. The library is any folder on this
machine; the Mac's is copied with
`rsync -a livid@<mac>:~/Library/Containers/xyz.planetable.Planet/Data/Documents/Planet/ <somewhere>/`.
Writes the Workspace layout: `site.json` with Planet's own id, name,
about, template (resolved to the installed folder, so "8-bit" becomes
`gamedb`), settings, author, tags, social, Plausible and noindex, without
the token fields or the `ipns`; `avatar.png`, `favicon.ico`, and the
custom code as `head.html`, `body-start.html`, `body-end.html` when
enabled; `posts/<slug>/index.md` (a page under `pages/`, with its weight)
carrying the article's id, title, created and modified to the
microsecond, tags by display name, hero when it is an attachment, and
external link; the listed attachments beside it. A slug is the article's
own when valid, else from its title, else the id lower-cased, with a
counter on a clash. A title may be empty, as Planet allows. A failure
part way removes the half-made site. The site keeps its `ipns` only when
the key is imported too, which needs a `.planet` backup and is a
follow-up. Measured: planet.local, 121 posts and 39 attachments, imports
in 0.2 s and builds in 0.2 s.

## Testing

- Go tests: engine fixtures are the six templates; builder goldens are
  real Planet builds fetched by `make goldens` from the Mac into
  `testdata/goldens/` (gitignored, never committed) and diffed file by
  file, HTML normalised for `build_timestamp`. Order: Sparkle (three
  files, no pagination) first, then Plain, Sepia, Memories, 8-bit, Grid.
- Kubo tests run against a fake RPC server; nothing creates keys or pins
  on the real node.
- UI tests: Playwright (`~/tools/playwright`) against a scratch exe-planet
  on another port with its own workspace and state directories.

## Phases

0. exe: service relay, route registration, config. Build and restart exe
   once. Done 2026-09-25.
1. Daemon skeleton: scan, slug validation, sites and posts API on files,
   watcher, systemd unit, tests. Done 2026-09-25: `make install` put the
   unit in place, `make restart` is the loop.
2. Template engine and builder to golden parity, template by template.
   Done 2026-09-25: Sparkle and Memories match; Plain and Sepia wait
   for a rebuild on the Mac; 8-bit has no current golden. The daemon
   side is in `internal/service`: Workspace files projected onto
   Planet's model (a page is an article of type 1 in the navigation, a
   draft is left out, tags are keyed by Planet's normalizedTag rule, a
   summary is the rendered text cut at 280), one build per site in flight
   with one more queued, a build made beside the live folder and swapped
   in, the watcher's changes mapped to sites (a template change rebuilds
   every site), every live site built at start, and the shipped
   templates installed into `Workspace/Planet/Templates` by buildNumber
   unless the folder holds a `.git`.
3. Importer, used first by the golden tests. Done 2026-09-25; the goldens
   are laid out as libraries, so each imports as is.
4. The app. Done 2026-09-25 (0.4.0): Writer and Planet as above; the
   desk grew `open-app`, `open` and `workspace-changed`; the daemon grew
   avatar, view and render routes and `internal/build.Inline`, which also
   drops relative preloads and resolves `../avatar.png` from the site
   root. Playwright: `exe-writer-test.js` and `exe-planet-app-test.js`
   (the latter against a scratch daemon, `PLANET=` its address).
5. Publishing: expose, keys, IPFS. Done 2026-09-25 (0.5.0), as the
   Publishing section says; `internal/kubo` (with `kubotest`, a fake
   whose shapes were checked against Kubo 0.43), `internal/service`'s
   `exe.go` and `node.go`. Tests: Go with fake exe and Kubo;
   `exe-planet-publish-test.js` drives the sheet against a scratch
   daemon with fakes the script runs (`-exe http://127.0.0.1:7796 -kubo
   http://127.0.0.1:7795 -exe-config <a file naming a domain>`).
6. Markdown without a dependency, gated on the corpus.
7. Replies from the hub, as that section says. Done 2026-09-26 (0.6.0);
   the live site switched on at Livid's say.

## Later

- The Finder opening `index.md` under `Planet/` in this editor.
- Import of a `.planet` backup including its key (an imported site
  announces on the hub only with its own key).
- **Tips** to the site's Solana address, the third role of its key: the
  address and a Tip… button on the page. SOL arrives as is (a first
  transfer must cover the rent minimum, about 0.00089 SOL); USDC and
  other SPL tokens land in a token account the sender's wallet makes
  and pays for (about 0.002 SOL); moving a token out costs a fee in SOL,
  so a site holding only USDC withdraws with Livid's wallet paying or
  once it holds some SOL. Withdraw… sweeps to Livid's own wallet to keep
  what rides on the key small; Export warns that its zip is a wallet;
  Kubo's `key sign` checked first (see One key per site).
- Podcast fields and `podcast.xml`.
