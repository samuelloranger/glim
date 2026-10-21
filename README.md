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

Publish a self-contained HTML file or directory (or a Markdown, text, JSON or image file, rendered to a styled page) and get a short, readable link to
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

`glim serve` also serves a private dashboard at `/`. It shows a thumbnail of
every preview, a countdown and a seen count for each one, and lets you extend, pin, lock or
remove them. It updates in real time. The first visit creates the account with an
email and password, so do that before exposing the server. See
[docs/dashboard.md](docs/dashboard.md).
Comments you leave on a preview from its glim bar reach your agent through the
MCP `comments` tool, and its replies come back under them.
**Listen** makes an audio version of a preview on demand: a vision model
describes its text, pictures and design, and a speech endpoint reads that aloud.
Both are any OpenAI-compatible API, hosted or local; see
[docs/config.md](docs/config.md#listen).
Over HTTPS it also installs as an app (iPhone Home Screen included) and can send a
notification when a preview is published; see
[docs/dashboard.md](docs/dashboard.md#install-as-an-app-and-notifications).

## Commands

```
glim <entry.html|dir|-> [--title T] [--project P] [--ttl 6h] [--name SLUG] [--local] [--qr] [--json]  publish, print URL or JSON (`-` reads stdin)
glim serve [--bind ADDR] [--port N] [--root DIR]                       run the preview server
glim config [--domain URL --bind ADDR --port N --root DIR --ttl 6h --live-reload true|false --self-fetch true|false --header true|false]  show or set config
glim config [--llm-url URL --llm-model M --tts-url URL --tts-model M --tts-voice V --tts-voices fr=V --chrome PATH|off]  set up Listen
glim caddy                                                             print a reverse-proxy vhost
glim ls [--project P] [--json] | rm <name>... | gc                      manage previews
glim extend <name> <ttl> | pin <name> | unpin <name>                  change a preview's lifetime
glim <entry> --password | lock <name> | unlock <name>                 password-protect a preview
glim open <name> | status                                             open a link / show status
glim mcp                                                              run as an MCP server
glim install [--skill] <claude|codex|cursor>                          wire into an agent
glim install service                                                  run the server in the background
glim uninstall <claude|codex|cursor|service>                          undo install
glim user ls | passwd <email> | rm <email>                            manage dashboard accounts
glim version
```

## Config

`~/.glim/config.json` (overridable by `GLIM_DOMAIN`, `GLIM_BIND`, `GLIM_PORT`,
`GLIM_ROOT`, `GLIM_TTL`, `GLIM_LIVE_RELOAD`, `GLIM_SELF_FETCH`). A published preview lives under `root` until its TTL
elapses; expired previews are pruned on the next command and by `glim gc`.

## Agent integration (MCP)

`glim install <agent>` registers glim's MCP server (tools: `present`, `list`,
`get`, `revoke`, `pin`, `extend`, `lock`, `comments`, `resolve_comment`) and writes a steering rule so the agent prefers glim
for previews. Supported: Claude, Codex, Cursor. `glim install --skill <agent>`
writes a `SKILL.md` instead (no MCP server), and `glim uninstall <agent>` removes
either setup.

## Behind a reverse proxy

Run `glim serve` bound where your proxy can reach it (`--bind 0.0.0.0`), point the
proxy at `HOST:PORT`, and set `--domain` so links use the public URL. `glim caddy`
prints a ready Caddy vhost. Keep it behind your usual access controls; a link's
readable slug carries a short random suffix but is not a secret.

## Run as a service

```sh
glim install service     # systemd user unit on Linux, launchd agent on macOS
glim uninstall service
```

It writes the unit (`~/.config/systemd/user/glim.service`) or agent
(`~/Library/LaunchAgents/glim.plist`), starts it, and waits until the server
answers. Re-run it after moving or upgrading the binary. On Linux, run
`loginctl enable-linger "$USER"` to keep the server running after you log out. On
macOS the agent only runs while you are logged in; its log is
`~/Library/Logs/glim/serve.log`. Put API keys in `~/.glim/env` (see
[docs/config.md](docs/config.md)).

Manage it with:

```sh
systemctl --user status glim            # or: restart, stop
journalctl --user -u glim -n 50         # logs
launchctl kickstart -k gui/$(id -u)/glim   # macOS: restart
```

## License

MIT
