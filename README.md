# glim

Publish a self-contained HTML file or directory and get a short, readable link to
show someone — from the command line or from a coding agent via MCP. glim runs its
own server; put it behind a reverse proxy for a public URL, or use `--local` for a
loopback link with no proxy at all.

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

## Commands

```
glim <entry.html|dir> [--title T] [--project P] [--ttl 6h] [--local]   publish, print URL
glim serve [--bind ADDR] [--port N] [--root DIR]                       run the preview server
glim config [--domain URL --bind ADDR --port N --root DIR --ttl 6h]    show or set config
glim caddy                                                             print a reverse-proxy vhost
glim ls | rm <name>... | gc                                           manage previews
glim mcp                                                              run as an MCP server
glim install <claude|codex|cursor>                                    wire into an agent
glim version
```

## Config

`~/.glim/config.json` (overridable by `GLIM_DOMAIN`, `GLIM_BIND`, `GLIM_PORT`,
`GLIM_ROOT`, `GLIM_TTL`). A published preview lives under `root` until its TTL
elapses; expired previews are pruned on the next command and by `glim gc`.

## Agent integration (MCP)

`glim install <agent>` registers glim's MCP server (a `present` tool) and writes a
steering rule so the agent prefers glim for previews. Supported: Claude, Codex,
Cursor.

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
