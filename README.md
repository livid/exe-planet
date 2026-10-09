# exe-planet

Write a blog in [exe](https://github.com/livid/exe). Markdown files in the
exe Workspace become static sites, built with [Planet](https://planetable.xyz)'s
site templates into the same `planet.json`, pages and RSS that Planet writes,
so a Planet user can follow them. A site can stay on your node, go out under a
name in your domain through exe's Cloudflare tunnel, go to IPFS, or both.

<img src="https://hub.v2core.com/v1/embed/bafkreiawlo2odksnfrqdqi4txaanzhb4b2cq6enapldb6a3e3uusvlj4ny" alt="The Planet app: sites on the left, the chosen site's posts in the middle, the built page on the right" width="762">

It is a small Go daemon on `127.0.0.1:7799` plus two exe desktop apps:
**Planet** (your sites, their posts, the built page) and **Writer** (any
Markdown file with its page beside it, saved as you type).

## Install with an agent

Paste this into Claude Code or Codex:

> Install exe (https://github.com/livid/exe) and exe-planet (https://github.com/livid/exe-planet) on this machine by following their READMEs, and keep both daemons running. Then open http://127.0.0.1:7777 and start a first site with me in the Planet app.

## Install by hand

Needs [exe](https://github.com/livid/exe) running and Go 1.26 or newer.

```sh
git clone https://github.com/livid/exe-planet.git
cd exe-planet
make build
./exe-planet      # keeps this terminal; on Linux, `make install` runs it as a systemd user service
```

Run it from the checkout: it installs its templates from `./templates`.
Then add it to exe's `~/.exe/config.json` and restart exe:

```json
"apps_dirs": ["/path/to/exe-planet/apps"],
"services": { "planet": "http://127.0.0.1:7799" }
```

Planet and Writer appear on the desktop. Sites are folders under `Planet/`
in the Workspace; build output and keys stay in `~/.exe/planet/`.
Publishing is off until you turn it on in a site's **Publish…** sheet:
exposing a site needs exe's [Cloudflare setup](https://exe.v2core.com/docs/config#cloudflare-setup-one-time),
IPFS needs [Kubo](https://docs.ipfs.tech/install/command-line/) with its RPC
API on `127.0.0.1:5001`. `./exe-planet -h` lists the flags.

## More

[PLAN.md](PLAN.md) is the design and its single source of truth;
[CLAUDE.md](CLAUDE.md) is the working agreement for agents in this repo.
