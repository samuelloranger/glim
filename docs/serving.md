# Serving & reverse proxy

glim serves previews itself. There is no separate web server — you either use the
loopback link directly, or put a reverse proxy in front for a public URL.

## The server

```sh
glim serve
```

It binds `bind:port` from config (default `127.0.0.1:8787`) and serves previews
from `root`. It serves each live preview's files and its `index.html`, never
lists a directory, never serves dot-prefixed paths (such as `.glim.json` or
`.env`), and returns 404 for a preview whose lifetime has elapsed, even before it
is garbage-collected. The server also prunes expired previews once a minute.
State is written to `~/.glim/serve.json` so a `--local` publish can find and
reuse it.

Every preview response carries
`Content-Security-Policy: sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads`.
Previews run as an isolated (opaque) origin: scripts, forms, pop-ups and
downloads work, but `localStorage`, `sessionStorage` and cookies are
unavailable — code that touches them without a `try`/`catch` will throw.

The site root serves the [dashboard](./dashboard.md). Its API lives under
`/_glim/`, and its live updates use Server-Sent Events. A standard reverse proxy
(including the `glim caddy` snippet) needs no extra configuration for them.

Set `--domain` to the public URL when you use a proxy. Sign-in and dashboard
actions check the browser's `Origin` against it, so they keep working behind
proxies that rewrite the `Host` header without sending `X-Forwarded-Host`.

## Run it as a service

`contrib/glim.service` is a systemd user unit:

```sh
mkdir -p ~/.config/systemd/user
cp contrib/glim.service ~/.config/systemd/user/
systemctl --user enable --now glim
loginctl enable-linger "$USER"   # keep it running after logout / across reboots
```

## Behind a reverse proxy

For a public URL, bind where the proxy can reach the server and set a domain:

```sh
glim config --domain https://glim.example.com --bind 0.0.0.0 --port 8787
```

Then point the proxy at `HOST:8787`. `glim caddy` prints a ready vhost:

```sh
glim caddy
```

```
glim.example.com {
    reverse_proxy 127.0.0.1:8787
}
```

If the proxy runs in a container, `127.0.0.1` refers to the container, not the
host — use the host's reachable address as the upstream and bind the server to
`0.0.0.0`.

## A note on access

A preview's URL has a readable slug plus a short random suffix. That is enough to
avoid collisions and casual guessing, but it is **not a secret**. Keep the proxy
behind your usual access controls (a private network, an auth layer, or a
firewall) for anything you would not put on the open web.

The dashboard itself requires an account, but preview links stay public by
design. Anyone with a link can open that preview.
