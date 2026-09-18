# Configuration

glim reads `~/.glim/config.json`. Set it from the command line:

```sh
glim config --domain https://glim.example.com --bind 0.0.0.0 --port 8787
glim config                     # show the resolved config
```

## Keys

| key | default | meaning |
| --- | --- | --- |
| `domain` | _(none)_ | Public base URL for links. Unset → links use `http://127.0.0.1:PORT`. |
| `bind` | `127.0.0.1` | Address the server binds. Use `0.0.0.0` behind a reverse proxy. |
| `port` | `8787` | Server port. |
| `root` | `~/.glim/pub` | Where previews are stored and served from. |
| `ttl` | `6h` | Default lifetime for a preview. |

## Environment overrides

Every key can be overridden by an environment variable, which wins over the file:

`GLIM_DOMAIN`, `GLIM_BIND`, `GLIM_PORT`, `GLIM_ROOT`, `GLIM_TTL`.

`GLIM_SESSION_ID`, if set, is recorded on each preview as metadata.

## How links are built

- With a `domain`, a link is `https://domain/<slug>-<id>/`.
- Without one, it is `http://127.0.0.1:<port>/<slug>-<id>/`.

The `<slug>` comes from the title or filename; the short `<id>` suffix keeps names
unique. The MCP `present` tool returns the same URL the CLI prints, so an agent
always hands you a link that resolves.

## Lifetime and cleanup

Each preview stores its expiry. Expired previews are removed on the next `glim`
command and by `glim gc`. Run `gc` from a periodic timer if the server is
long-lived and rarely invoked.
