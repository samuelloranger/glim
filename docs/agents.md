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
   `present` tool.
2. **Writes a steering rule** into the agent's global instructions so it prefers
   glim for previews rather than its own mechanism.

| agent | MCP registered in | steering rule in |
| --- | --- | --- |
| Claude | user MCP config | `~/.claude/CLAUDE.md` |
| Codex | `~/.codex/config.toml` | `~/.codex/AGENTS.md` |
| Cursor | `~/.cursor/mcp.json` | `~/.cursor/rules/glim.mdc` |

The rule and config blocks are marked as managed, so re-running `install` updates
them in place instead of duplicating.

## The present tool

```
present(path, title?, project?, ttl?, name?) → { url, name, expires }
```

The agent writes a self-contained HTML file or directory, calls `present`, and
gives you the returned `url`. It is the same URL the CLI prints, built from your
[configured domain](/config), so it resolves wherever your server is reachable.

To push an **update** to the same URL instead of minting a new one, the agent
passes `name` set to the slug a previous `present` returned. The preview is
replaced in place (stale files removed, expiry reset), so a link already shared
keeps working with fresh contents. Omitting `name` gives a new random slug, as
before. `name` accepts lowercase letters, digits and single hyphens only.

## Why a rule as well as a tool

Registering the tool makes it available, but some agents have a built-in preview
mechanism they would otherwise reach for. The steering rule tells the agent to use
glim's `present` tool for previews. Both together make it reliable.
