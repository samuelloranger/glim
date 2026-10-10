# CLI reference

## publish

```sh
glim <entry.html|dir> [--title T] [--project P] [--ttl 6h] [--name SLUG] [--password] [--local] [--qr]
```

Copies `entry` into a new preview and prints its URL. A single file is served as
`index.html`; a directory is copied and must contain its own `index.html`.

### Single-file formats

Besides HTML, a single file can be one of these. glim converts it into a styled,
self-contained `index.html` (inline CSS, light and dark via `prefers-color-scheme`,
system fonts, no scripts, no external requests). The original file is also kept
next to `index.html` under its own name, and the page links to it as `raw`.

| Extension | Result |
|---|---|
| `.md`, `.markdown` | Rendered with GitHub-flavoured Markdown (tables, task lists, strikethrough, autolinks). Raw HTML in the source is not passed through. Page title is `--title`, else the first H1, else the file name. |
| `.txt`, `.log` | Escaped, wrapping `<pre>`. |
| `.json` | Validated and pretty-printed in an escaped `<pre>`. Invalid JSON fails the publish. |
| `.png` `.jpg` `.jpeg` `.gif` `.webp` `.avif` `.svg` | Centered image viewer page. |

Converted text formats are limited to 20 MB.

When a single Markdown or HTML file references local files with relative
`src`/`href` paths (for example `![](shot.png)` or `[data](data.csv)`), glim also
copies those files, and only those, next to the page. Only regular files inside
the published file's own folder are copied: no `..` escapes, absolute paths,
symlinks, hidden names or directories, and files over 25 MB (100 MB in total)
are skipped silently.

Publishing is deliberately conservative so you cannot expose files by accident:

- The entry must not be a symlink or have a name starting with `.`.
- A single file must be a regular file of a supported type: `.html`/`.htm`
  (served as is) or one of the formats below, which are converted at publish time.
- In a directory, dot-prefixed files and directories (`.env`, `.git`, ...) are
  skipped silently. A symlink or other non-regular file inside the directory
  fails the publish with an error naming the path.
- A directory may hold at most 5000 files and 200 MB in total.
- Republishing to an existing `--name` is atomic: the new version is built
  aside and swapped in, so a failed publish leaves the old preview untouched.

| flag | meaning |
| --- | --- |
| `--title` | Human title; becomes the readable slug. Defaults to the filename. |
| `--project` | Stored as metadata, shown in `glim ls`. |
| `--ttl` | Lifetime, e.g. `6h`, `30m`; must be greater than zero. Defaults to the configured TTL. |
| `--name` | Reuse this exact slug to update in place at the same URL (created if absent). Omit for a fresh random link. |
| `--password` | Protect the preview with a password (see [Password protection](#password-protection)). |
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
glim config [--domain URL] [--bind ADDR] [--port N] [--root DIR] [--ttl 6h] [--live-reload true|false]
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
glim ls                 # list live previews (name, title, age, expiry, views, last seen)
glim rm <name>...       # remove previews now
glim gc                 # prune expired previews
```

Expired previews are also pruned automatically whenever you publish.

## Password protection

```sh
glim report.html --password      # publish behind a password
glim lock <name>                 # add or change the password of a live preview
glim unlock <name>               # remove it
```

A password-protected preview shows visitors a glim password form instead of its
content; nothing from the page (not even its title) is revealed until they
unlock it. Passwords are 8 to 72 bytes, like dashboard passwords.

The password is never taken from a flag or an environment variable, so it does
not land in shell history or process listings. On a terminal glim prompts
without echo (and asks twice); when stdin is piped it reads one line:

```sh
printf '%s\n' "$PREVIEW_PASSWORD" | glim report.html --password
```

Only a bcrypt hash is stored (in the preview's manifest). Republishing a locked
preview with `--name` keeps its password unless you pass `--password` again.
`glim unlock` removes it, and changing or removing the password signs every
visitor out. The dashboard shows a `locked` badge on locked previews, and you
(signed in to the dashboard in the same browser) are never asked. See
[Serving](./serving.md#password-protected-previews) for how it works.

## extend · pin

```sh
glim extend <name> <ttl>   # set a new lifetime measured from now, e.g. 48h
glim pin <name>            # never expire until removed
glim unpin <name>          # expire again after the default lifetime
```

`pin` exempts a preview from TTL expiry and garbage collection; `glim ls` shows
it as `pinned`. `unpin` makes it expire again at `now + the configured default
ttl`. `extend` resets the expiry to `now + ttl`; on a pinned preview it also
unpins it, because the lifetime you chose wins. A zero or negative ttl is rejected, so a preview is never created already expired.

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

Prints the store root, whether the server is running (and its port), the live
preview count (and how many are pinned), disk use, and the next expiry due for
garbage collection. When the domain is `https` and the server is running, it adds
a one-line note that glim itself serves plain http and needs a TLS-terminating
proxy that sends `X-Forwarded-Proto: https`.

## mcp

```sh
glim mcp
```

Runs glim as an MCP server over stdio, exposing `present` (optionally with a `password`), `list`, `get` (inspect one preview, optionally with its published source), `revoke`,
`pin` (pass `pinned: false` to unpin), and `extend` tools. Prefer an absolute `path` for `present` (a leading `~/` is expanded): a relative path resolves against the directory the MCP server was started in, which may not be the agent's current one. With no domain configured it starts the local server on demand. `list` includes each preview's view count and
last-opened time (see [Dashboard](./dashboard.md#seen-indicator)). Normally you do not call this directly — `glim
install` wires it into an agent.

## install

```sh
glim install [--skill] <claude|codex|cursor>
```

Registers the MCP server with the agent and writes a steering rule so it prefers
glim for previews. With `--skill` it writes a `SKILL.md` instead, with no MCP
server. See [Agent integration](/agents).

## uninstall

```sh
glim uninstall <claude|codex|cursor>
```

Removes everything `install` added, in either mode. Safe to re-run.

## version

```sh
glim version
```

## user

```sh
glim user ls                # list dashboard accounts
glim user passwd <email>    # set a new password (prompts twice); signs that user out everywhere
glim user rm <email>        # remove an account and its sessions
```

Use these from a shell on the server to recover access to the dashboard. If
you remove the last account, the dashboard offers the "Create your account"
form again.
