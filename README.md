<p align="center">
  <img src="docs/public/icon.svg" width="96" alt="glim logo" />
</p>

<h1 align="center">glim</h1>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/github/license/samuelloranger/glim" alt="License: MIT" /></a>
  <a href="https://github.com/samuelloranger/glim/releases"><img src="https://img.shields.io/github/v/release/samuelloranger/glim" alt="Latest release" /></a>
  <a href="https://github.com/samuelloranger/glim/actions/workflows/ci.yml"><img src="https://github.com/samuelloranger/glim/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="https://github.com/samuelloranger/glim/actions/workflows/release.yml"><img src="https://github.com/samuelloranger/glim/actions/workflows/release.yml/badge.svg" alt="Release builds" /></a>
  <img src="https://img.shields.io/badge/Go-single%20binary-00ADD8?logo=go&logoColor=white" alt="Go single binary" />
  <a href="https://buymeacoffee.com/samlo122"><img src="https://img.shields.io/badge/Buy%20me%20a%20coffee-FFDD00?logo=buymeacoffee&logoColor=black" alt="Buy me a coffee" /></a>
</p>

Publish a self-contained HTML file or directory and get a short, readable link to
show someone — from the command line or from a coding agent via MCP. glim runs its
own server; put it behind a reverse proxy for a public URL, or use `--local` for a
loopback link with no proxy at all.

Self-host the HTML artifacts a coding agent produces. Instead of a preview that
lives on a vendor's servers, glim keeps every agent-generated page on your own
infrastructure, at your own domain, under your own access controls.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/samuelloranger/glim/main/install.sh | sh
```

Or build from source: `go build -o glim .`

## Quick start

```sh
glim config --domain https://glim.example.com    # optional; omit for localhost links
glim serve &                                     # run the preview server
glim report.html --title "Diff review"           # -> https://glim.example.com/diff-review-a1b2/
```

Without a domain, links are `http://127.0.0.1:PORT/...`. `--local` auto-starts the
server and returns a loopback link:

```sh
glim report.html --title "Quick look" --local
```

## Dashboard

`glim serve` also serves a private dashboard at `/`. It shows live thumbnails of
every preview and a countdown for each one, and lets you extend, pin or remove
them. It updates in real time. The first visit creates the account with an
email and password, so do that before exposing the server. See
[docs/dashboard.md](docs/dashboard.md).

## Commands

```
glim <entry.html|dir> [--title T] [--project P] [--ttl 6h] [--name SLUG] [--local] [--qr]  publish, print URL
glim serve [--bind ADDR] [--port N] [--root DIR]                       run the preview server
glim config [--domain URL --bind ADDR --port N --root DIR --ttl 6h]    show or set config
glim caddy                                                             print a reverse-proxy vhost
glim ls | rm <name>... | gc                                           manage previews
glim extend <name> <ttl> | pin <name>                                 change a preview's lifetime
glim open <name> | status                                             open a link / show status
glim mcp                                                              run as an MCP server
glim install <claude|codex|cursor>                                    wire into an agent
glim user ls | passwd <email> | rm <email>                            manage dashboard accounts
glim version
```

## Config

`~/.glim/config.json` (overridable by `GLIM_DOMAIN`, `GLIM_BIND`, `GLIM_PORT`,
`GLIM_ROOT`, `GLIM_TTL`). A published preview lives under `root` until its TTL
elapses; expired previews are pruned on the next command and by `glim gc`.

## Agent integration (MCP)

`glim install <agent>` registers glim's MCP server (tools: `present`, `list`,
`revoke`, `pin`, `extend`) and writes a steering rule so the agent prefers glim
for previews. Supported: Claude, Codex, Cursor.

## Behind a reverse proxy

Run `glim serve` bound where your proxy can reach it (`--bind 0.0.0.0`), point the
proxy at `HOST:PORT`, and set `--domain` so links use the public URL. `glim caddy`
prints a ready Caddy vhost. Keep it behind your usual access controls; a link's
readable slug carries a short random suffix but is not a secret.

## Run as a service

`contrib/glim.service` is a systemd user unit:

```sh
mkdir -p ~/.config/systemd/user
cp contrib/glim.service ~/.config/systemd/user/
systemctl --user enable --now glim
loginctl enable-linger "$USER"   # survive logout/reboot
```

## License

MIT
