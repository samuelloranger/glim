# CLI reference

## publish

```sh
glim <entry.html|dir> [--title T] [--project P] [--ttl 6h] [--local]
```

Copies `entry` into a new preview and prints its URL. A single file is served as
`index.html`; a directory is copied as-is and must contain its own `index.html`.

| flag | meaning |
| --- | --- |
| `--title` | Human title; becomes the readable slug. Defaults to the filename. |
| `--project` | Stored as metadata, shown in `glim ls`. |
| `--ttl` | Lifetime, e.g. `6h`, `30m`. Defaults to the configured TTL. |
| `--local` | Auto-start the built-in server and return a loopback link. |

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

## mcp

```sh
glim mcp
```

Runs glim as an MCP server over stdio, exposing a `present` tool. Normally you do
not call this directly — `glim install` wires it into an agent.

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
