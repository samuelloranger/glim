# CLI reference

## publish

```sh
glim <entry.html|dir> [--title T] [--project P] [--ttl 6h] [--name SLUG] [--local] [--qr]
```

Copies `entry` into a new preview and prints its URL. A single file is served as
`index.html`; a directory is copied as-is and must contain its own `index.html`.

| flag | meaning |
| --- | --- |
| `--title` | Human title; becomes the readable slug. Defaults to the filename. |
| `--project` | Stored as metadata, shown in `glim ls`. |
| `--ttl` | Lifetime, e.g. `6h`, `30m`. Defaults to the configured TTL. |
| `--name` | Reuse this exact slug to update in place at the same URL (created if absent). Omit for a fresh random link. |
| `--local` | Auto-start the built-in server and return a loopback link. |
| `--qr` | Also print a scannable QR code of the URL (to stderr, so stdout stays the plain URL). |

## Updating a preview in place

By default each publish gets a fresh random slug, so its URL is
unguessable and each publish is distinct. To push updates to the **same**
URL — while iterating on a page you have already shared — pass `--name`
with the slug from the first publish:

```sh
glim report.html                 # → https://glim.example.com/report-k3n7/
glim report.html --name report-k3n7   # same URL, new contents
```

`--name` accepts lowercase letters, digits and single hyphens only;
anything else is rejected. An update replaces the preview's files
completely (stale files from the previous version are removed) and resets
the expiry clock from now, exactly like a fresh publish.

## serve

```sh
glim serve [--bind ADDR] [--port N] [--root DIR]
```

Runs the preview server. Defaults come from config. Bind `0.0.0.0` to accept a
reverse proxy; `--port 0` picks a free port.

## config

```sh
glim config                                        # show resolved config
glim config [--domain URL] [--bind ADDR] [--port N] [--root DIR] [--ttl 6h]
```

With no flags, prints the resolved configuration and its file path. With flags,
updates `~/.glim/config.json`. See [Configuration](/config).

## caddy

```sh
glim caddy
```

Prints a ready-to-paste Caddy vhost that reverse-proxies your domain to the
server. Requires a domain to be set.

## ls · rm · gc

```sh
glim ls                 # list live previews (name, title, age, expiry)
glim rm <name>...       # remove previews now
glim gc                 # prune expired previews
```

Expired previews are also pruned automatically whenever you publish.

## extend · pin

```sh
glim extend <name> <ttl>   # set a new lifetime measured from now, e.g. 48h
glim pin <name>            # never expire until removed
```

`pin` exempts a preview from TTL expiry and garbage collection; `glim ls` shows
it as `pinned`. `extend` resets the expiry to `now + ttl`.

## open

```sh
glim open <name>
```

Prints the preview's URL. If a desktop session is present (`DISPLAY` set), it
also opens the link with `xdg-open`.

## status

```sh
glim status
```

Prints the store root, the live preview count (and how many are pinned), disk
use, and the next expiry due for garbage collection.

## mcp

```sh
glim mcp
```

Runs glim as an MCP server over stdio, exposing `present`, `list`, `revoke`,
`pin`, and `extend` tools. Normally you do not call this directly — `glim
install` wires it into an agent.

## install

```sh
glim install <claude|codex|cursor>
```

Registers the MCP server with the agent and writes a steering rule so it prefers
glim for previews. See [Agent integration](/agents).

## version

```sh
glim version
```

## user

```sh
glim user ls                # list dashboard accounts
glim user passwd <name>     # set a new password (prompts twice); signs that user out everywhere
glim user rm <name>         # remove an account and its sessions
```

Use these from a shell on the server to recover access to the dashboard. If
you remove the last account, the next `glim serve` start prints a new setup
code. `glim status` shows a pending setup code while no account exists.
