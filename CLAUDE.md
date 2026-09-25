# exe-planet

A standalone Go daemon that builds static blog sites from Markdown in the
exe Workspace with Planet's site templates, writes Planet-compatible output,
and optionally exposes or publishes each site to IPFS. The editor is the
exe desktop app in `apps/Planet/`.

**PLAN.md is the single source of truth for every project decision.**
Before implementing, check PLAN.md; when a design decision changes, update
PLAN.md first or alongside the code. If code and PLAN.md disagree, PLAN.md
wins and the code is the bug.

## Workflow

- Commit on `main`. No branches, no PRs. First line `Area: what changed`
  (Daemon:, Builder:, Engine:, App:, Import:, Docs:), body says why.
- The repo is public: no real email address in code, fixtures, docs or
  commit messages; use `someone@example.com`. `.githooks` refuses one;
  enable it once per checkout with `git config core.hooksPath .githooks`.
- Build: `export PATH=$PATH:/usr/local/go/bin && go build ./cmd/exe-planet`.
  Restart: `XDG_RUNTIME_DIR=/run/user/1000 systemctl --user restart
  exe-planet`. This daemon restarts freely; exe does not need to.
- Listens on `127.0.0.1:7799`. The desk reaches it through exe's service
  relay; the app in `apps/Planet/` is served live by exe from this
  checkout, so UI edits need no restart at all.
- Test: `go test ./...`. Tests take their own scratch directories; never
  derive a path from `EXE_HOME` or `HOME`, never touch `~/.exe/workspace`
  or `~/.exe/planet`, never create keys or pins on the real Kubo.
- Goldens are real Planet builds from Livid's Mac, fetched by `make
  goldens` into `testdata/goldens/`, gitignored.
- UI follows `/www/exe/docs/platinum.md` and the app conventions in
  `/www/exe-apps/CLAUDE.md`. Screenshot every UI change in headless
  Chromium at device pixel ratios 1, 1.5 and 2 and look at it.
- Two agents may share this tree. Run `git status` before editing and
  leave the other agent's uncommitted files alone.
- When something notable is finished, post it to the hub in the first
  person, short, with a screenshot of the relevant window only.

## Data rules

- Every user-created thing is a file or folder under
  `~/.exe/workspace/Planet/`. No database.
- Site and post folders are slugs, validated on creation; titles live in
  `site.json` and front matter. The app never renames a folder by itself.
- Derived and secret state lives in `~/.exe/planet/<site uuid>/`: the
  build output, `node.json`, and the site's `ipns.key`.
- Output must match Planet 0.22.4 byte for byte where the goldens say so;
  dates in `planet.json` are seconds since 2001 as doubles.
