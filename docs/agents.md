# Agent integration (MCP)

glim can act as an MCP server so a coding agent shows you an HTML preview through a
tool call instead of pasting raw markup. `glim install` wires this up for you.

```sh
glim install claude    # or codex, cursor
```

Restart the agent afterwards so it loads the new MCP server.

## What install does

For each agent it does two things:

1. **Registers the MCP server** — a single stdio server, `glim mcp`, exposing a
   `present` tool plus `list`, `get`, `revoke`, `pin`, `extend`, `lock`,
   `comments` and `resolve_comment`.
2. **Writes a steering rule** into the agent's global instructions so it prefers
   glim for previews rather than its own mechanism.

| agent | MCP registered in | steering rule in |
| --- | --- | --- |
| Claude | user MCP config | `~/.claude/CLAUDE.md` |
| Codex | `~/.codex/config.toml` | `~/.codex/AGENTS.md` |
| Cursor | `~/.cursor/mcp.json` | `~/.cursor/rules/glim.mdc` |

The rule and config blocks are marked as managed, so re-running `install` updates
them in place instead of duplicating. If `~/.codex/config.toml` already has its
own `[mcp_servers.glim]` table (for example from `codex mcp add glim`), install
adopts it into the managed block, keeping its extra keys, and `glim uninstall
codex` then removes those keys along with the block.

## Skill mode

If you would rather not run an MCP server, install a skill instead:

```sh
glim install --skill claude    # or codex, cursor
```

This writes a `SKILL.md` that tells the agent to publish with the `glim` CLI and
hand you the link. It does not register the MCP server or write a steering rule.
Re-running overwrites the file.

| agent | skill written to |
| --- | --- |
| Claude | `~/.claude/skills/glim/SKILL.md` |
| Codex | `~/.agents/skills/glim/SKILL.md` |
| Cursor | `~/.cursor/skills/glim/SKILL.md` |

## Uninstall

```sh
glim uninstall claude    # or codex, cursor
```

Removes the MCP registration, the managed steering blocks or rule file, and the
skill directory, whichever of them exist. Content around the managed blocks and
other MCP servers are left untouched. Re-running prints `nothing to remove`.

## The present tool

```
present(path | content, format?, title?, project?, ttl?, name?, password?) → { url, name, expires, locked? }
```

The agent writes a self-contained HTML file or directory, calls `present`, and
gives you the returned `url`.

To skip the file, the agent passes the page text as `content` instead of `path`
(exactly one of the two is required). `format` says how to read it: `html`
(the default, which must be self-contained), `md`, `txt` or `json`, converted
the same way as a file of that type. Content is capped at 20 MB. Without a
`title`, glim uses the first `<title>` (html) or heading (md), else `preview`.
Everything else (`name`, `ttl`, `password`, locking) behaves as for a file.

Either way, the returned `url` is the same one the CLI prints, built from your
[configured domain](/config), so it resolves wherever your server is reachable.

To push an **update** to the same URL instead of minting a new one, the agent
passes `name` set to the slug a previous `present` returned. The preview is
replaced in place (stale files removed, expiry reset), so a link already shared
keeps working with fresh contents, and any tab already open on it refreshes
by itself (see [live reload](/serving); disable with `live_reload: false`). Omitting `name` gives a new random slug, as
before. `name` accepts lowercase letters, digits and single hyphens only.

The owner of the dashboard sees a small glim bar on previews they open (see [the glim bar](/dashboard#the-glim-bar)). An agent that does not want it on a given preview passes `header: false`; the default is `true`, and it is not remembered, so a republish with `name` shows the bar again unless `header: false` is passed again.

To protect a preview, the agent passes `password` (8 to 72 bytes). Visitors then
need that password to see anything; glim stores only a bcrypt hash. When an agent
republishes with `name` and omits `password`, the existing password is kept.
`list` reports `locked: true` for protected previews. Since the password is
tool input, it passes through the agent's context; use the `glim lock` command
or the dashboard yourself if you would rather the agent never sees it.

To lock a preview that is already live without republishing it, the agent calls
`lock(name, password)`; calling it again with a new password replaces the old
one. `lock(name, locked: false)` removes the password. Either change signs out
every visitor who unlocked the preview before.

## What a preview page can do

Previews are served with `Content-Security-Policy: sandbox allow-scripts
allow-forms allow-popups allow-modals allow-downloads`, with no
`allow-same-origin`, so every page runs in an opaque origin. Checked in headless
Chrome:

| Feature | Result |
| --- | --- |
| Inline `<script>`, inline `<style>`, `eval` | works |
| Scripts, stylesheets, images and iframes from a CDN | work |
| Web fonts from a CDN | not verified |
| `fetch` to another site | no-CORS requests go through; reading the response needs that site to send CORS headers (cdnjs does) |
| `fetch`/XHR of the preview's own files | works (glim answers `Origin: null` with CORS headers); not for password-protected previews: inline the data there |
| Relative `<img>`, `<script>`, `<link>` | work |
| `localStorage`, `sessionStorage`, `document.cookie` | throw `SecurityError` |
| `indexedDB`, Cache API, service workers | throw `SecurityError` |
| `#fragment` navigation, blob Workers, forms, popups, modals, downloads | allowed (a popup still needs a user gesture) |

So the agent should ship a self-contained page, wrap any storage call in
`try/catch`, and keep data inline. The steering rule, the skill and the
`present` tool description all say this.

## Phone-sized pages

Previews are often opened on a phone, so the steering rule, the skill and the
`present` description also ask for a page that reads well at 375px wide:
fluid widths rather than fixed pixel widths, grids and flex rows that stack to
one column, long paths, URLs and code that wrap (`overflow-wrap: anywhere`),
and wide tables, code blocks and diagrams in their own `overflow-x: auto`
container so the page never scrolls sideways. glim adds a viewport tag to any
page that lacks one (see [link preview cards](/serving#link-preview-cards)).

## The get tool

```
get(name, include_source?) → { name, url, title?, project?, pinned, expires?, locked?, views, lastSeen?, created, ... }
```

`get` inspects one live preview so a new session can read back what was
published. `expires` is omitted for pinned previews, and the password hash is
never returned. With `include_source: true` it also returns the published
source: the original file for a converted Markdown, text or JSON file, otherwise
`index.html`, capped at 64 KB (`truncated: true` when cut). For a directory
publish it returns `index.html` and lists the other file paths (relative, at
most 200). Images are never returned, only their file name and size
(`binary: true`). An unknown or expired slug gives the same "no such preview"
error as the other tools.

`present` also states the slug and expiry in its text (`name: <slug> · expires:
<RFC3339>`, or `pinned`) for clients that read only text, and `list` includes
each preview's expiry.

## Comments

You leave comments on a preview from the glim bar (see
[Comments](/dashboard#comments)); the agent reads and answers them over MCP:

```
comments(name?, include_resolved?) → { comments: [{ id, preview, url, author?, body, selector?, quote?, reply?, resolved, stale?, created }] }
resolve_comment(id, reply?, resolved?) → { id, ..., resolved, reply? }
```

`comments` returns the open comments of one preview, or of every live preview
when `name` is omitted, oldest first. A comment pinned to an element carries a
CSS `selector` for it and `quote`, the start of its text when the comment was
made; a comment on the whole page has neither. `stale: true` means the preview
was republished since, so the element may have moved or changed.

The loop: the user says they left feedback, the agent calls `comments`, edits
the page, republishes it with `present` and the same `name`, then calls
`resolve_comment` for each comment it handled, with a short `reply` saying what
changed. The reply shows under the comment in the bar. `resolved: false` saves a
reply but leaves the comment open, for a question back to the user.

`list` and `get` report `comments`, the number of open comments, and a `present`
that updates a preview with open comments says so in its text.

Comments are stored in the dashboard database; until `glim serve` has created
it (by creating the dashboard account), both tools return an error.

## Why a rule as well as a tool

Registering the tool makes it available, but some agents have a built-in preview
mechanism they would otherwise reach for. The steering rule tells the agent to use
glim's `present` tool for previews. Both together make it reliable. Claude's rule
also names Claude artifacts; the Codex and Cursor rule does not, since those
agents have none.

A `ttl` (on `present` and `extend`) must be greater than zero; zero or negative
values are rejected with an error instead of creating an already-expired preview.
