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
   `present` tool plus `list`, `get`, `revoke`, `pin` and `extend`.
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
present(path, title?, project?, ttl?, name?, password?) → { url, name, expires, locked? }
```

The agent writes a self-contained HTML file or directory, calls `present`, and
gives you the returned `url`. It is the same URL the CLI prints, built from your
[configured domain](/config), so it resolves wherever your server is reachable.

To push an **update** to the same URL instead of minting a new one, the agent
passes `name` set to the slug a previous `present` returned. The preview is
replaced in place (stale files removed, expiry reset), so a link already shared
keeps working with fresh contents, and any tab already open on it refreshes
by itself (see [live reload](/serving); disable with `live_reload: false`). Omitting `name` gives a new random slug, as
before. `name` accepts lowercase letters, digits and single hyphens only.

To protect a preview, the agent passes `password` (8 to 72 bytes). Visitors then
need that password to see anything; glim stores only a bcrypt hash. When an agent
republishes with `name` and omits `password`, the existing password is kept.
`list` reports `locked: true` for protected previews. Since the password is
tool input, it passes through the agent's context; use the `glim lock` command
yourself if you would rather the agent never sees it.

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
| `fetch`/XHR of the preview's own files | fails (no CORS headers for an opaque origin): inline the data instead |
| Relative `<img>`, `<script>`, `<link>` | work |
| `localStorage`, `sessionStorage`, `document.cookie` | throw `SecurityError` |
| `indexedDB`, Cache API, service workers | throw `SecurityError` |
| `#fragment` navigation, blob Workers, forms, popups, modals, downloads | allowed (a popup still needs a user gesture) |

So the agent should ship a self-contained page, wrap any storage call in
`try/catch`, and keep data inline. The steering rule, the skill and the
`present` tool description all say this.

## The get tool

```
get(name, include_source?) → { name, url, title?, project?, pinned, expires?, locked?, views, lastSeen?, created, ... }
```

`get` inspects one live preview so a new session can read back what was
published. `expires` is omitted for pinned previews, and the password hash is
never returned. With `include_source: true` it also returns the published
source: the original file for a converted Markdown, text or JSON file, otherwise
`index.html`, capped at 200 KB (`truncated: true` when cut). For a directory
publish it returns `index.html` and lists the other file paths (relative, at
most 200). Images are never returned, only their file name and size
(`binary: true`). An unknown or expired slug gives the same "no such preview"
error as the other tools.

`present` also states the slug and expiry in its text (`name: <slug> · expires:
<RFC3339>`, or `pinned`) for clients that read only text, and `list` includes
each preview's expiry.

## Why a rule as well as a tool

Registering the tool makes it available, but some agents have a built-in preview
mechanism they would otherwise reach for. The steering rule tells the agent to use
glim's `present` tool for previews. Both together make it reliable. Claude's rule
also names Claude artifacts; the Codex and Cursor rule does not, since those
agents have none.

A `ttl` (on `present` and `extend`) must be greater than zero; zero or negative
values are rejected with an error instead of creating an already-expired preview.
