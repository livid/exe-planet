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

## Layout on disk

Shared, synced to every peer as plain files (`~/.exe/workspace` is synced
under the `@workspace` namespace: whole-file last-writer-wins, a concurrent
edit keeps the loser under `~/.exe/sync-conflicts`, hidden paths skipped,
files over 10 MB neither accepted by the file API nor synced):

```
~/.exe/workspace/Planet/
  Templates/                  shared by every site; reserved name
    plain/  sepia/  grid/  gamedb/  memories/  sparkle/
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
    node.json                 expose {sub}, ipfs {enabled}, last build, last CID, last published
    ipns.key                  PKCS#8 PEM, mode 600, the site's IPNS identity
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
---
```

`id` is issued when the post is created and never changes, so a rename of
the folder is an edit for followers, not a delete plus an add.

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

**Platinum** (`templates/platinum`, 2026-09-25) is ours, the seventh: every
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
article maps carry none). `TestPlatinumTemplate` builds every page kind.
The live `exe` site (slug `exe`, avatar = the desk's icon) wears it; its
first post is "Meet exe".

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
pictures folded in so a sandboxed frame shows it without a path. A row of
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
- **Expose** (`expose`, `subdomain`) asks exe for a route,
  `POST /v1/routes {host: <sub>.<zone>, backend: http://127.0.0.1:7799}`,
  with the sub defaulting to the slug and the zone read from exe's
  `cloudflare.domain`; exe's DNS and tunnel warnings are kept in
  `route_warnings`. The daemon then serves that site's `public/` at the
  root of any request whose `Host` is the hostname (exe's proxy keeps the
  public Host). Off, a new sub, a delete or an archive send
  `DELETE /v1/routes/{host}`. The route lives in exe, so it survives
  either daemon's restart; the daemon rereads its node files at start.
- **IPFS** (`ipfs`) on the first turn-on makes an ed25519 key, keeps it
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

## Later

- The Finder opening `index.md` under `Planet/` in this editor.
- Import of a `.planet` backup including its key.
- Podcast fields and `podcast.xml`.
