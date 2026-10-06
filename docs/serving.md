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

Previews are kept out of search engines: every preview response carries
`X-Robots-Tag: noindex, nofollow`, and `/robots.txt` serves `Disallow: /`.

## Link preview cards

When a preview's HTML page (`index.html` or any `.html` file, up to 5 MB) is
fetched, glim inserts Open Graph and Twitter tags before `</head>` so a pasted
link unfurls in chat and email: `og:title` (the preview's title, else its slug),
`og:description` (the project, when set, plus "expires in ..." or "pinned"),
`og:site_name`, `og:type`, `og:url`, `og:image` and `twitter:card`. The image is
a fixed card served at `/_glim/og.png`. A tag the page already declares is never
overridden or duplicated. Larger and non-HTML files are streamed unchanged, and
`HEAD` and `Range` requests keep working.

## Live reload

Republishing a preview under the same name (`glim <file> --name X`, or an agent
calling `present` with `name`) refreshes any open tab of that preview, and the
dashboard thumbnails. glim inserts a small inline script before `</body>` of each
served HTML page; it opens an `EventSource` to `/_glim/live/<slug>` and calls
`location.reload()` when the stream reports `changed`. It is wrapped in an IIFE,
reconnects with backoff, and stops after repeated failures or a `gone` event
(the preview was removed or expired). The password unlock page never gets it. Pages that carry the script are served
with `Cache-Control: no-cache` and an `ETag` of the injected bytes, so a reload
after a pin, extend or lock change is never answered with a stale `304`.

The stream endpoint is public, needs no cookie, and answers with
`Access-Control-Allow-Origin: *` because previews run in an opaque origin. It
only ever says `changed` or `gone` for a slug the client already knows, never
content, so it does not need the preview unlocked. It sends a heartbeat comment
every 25 seconds and allows at most 200 concurrent streams, 20 per client IP;
beyond that it answers `429`. Turn the feature off with
`glim config --live-reload=false` (config key `live_reload`).

## Password-protected previews

A preview whose manifest has a `password_hash` (set with `glim <entry>
--password`, `glim lock`, or the MCP `present` tool's `password`) is locked.
Every request without a valid unlock cookie is refused: the HTML page gets a
glim-owned unlock form with status `401`, and every other file gets a plain
`401`. The form needs no external assets, and its link card says only
"Password-protected preview", with no title, project or lifetime.

Submitting the form `POST`s to the page's own path. A correct password sets a
`glim_unlock_<slug>` cookie (`HttpOnly`, `Path=/<slug>/`) and redirects back
(`303`). Its value is an HMAC of the slug and the current password hash under
the server's secret, so changing or removing the password invalidates every
earlier unlock. Attempts are rate-limited per client address and preview
(repeated failures are answered with `429` and `Retry-After`). Unlock attempts
never count as [views](./dashboard.md#seen-indicator), and a signed-in dashboard
owner (the `glim_owner` cookie, valid only while their session is live)
bypasses the lock. When the owner opens a locked page this way, glim also sets
that preview's unlock cookie so its scripts, styles and images load too.
Publishing, locking, unlocking, pinning and extending a preview are serialized,
and republishing a locked preview whose manifest cannot be read fails rather
than silently dropping the lock.

Previews run in a sandbox with an opaque origin, which browsers treat as
cross-site for their sub-resource requests. They only send the unlock cookie
there when it is `SameSite=None; Secure`, so glim sets those attributes when
the base URL is `https://` (set `--domain` behind a TLS proxy). Over plain
`http` the cookie is `SameSite=Lax`: a locked single-file preview works, but a
locked directory preview's scripts, styles and images will not load. Use https
for protected directory previews.

The site root serves the [dashboard](./dashboard.md). Its API lives under
`/_glim/`, and its live updates use Server-Sent Events. A standard reverse proxy
(including the `glim caddy` snippet) needs no extra configuration for them.

Set `--domain` to the public URL when you use a proxy. Sign-in and dashboard
actions check the browser's `Origin` against it, so they keep working behind
proxies that rewrite the `Host` header without sending `X-Forwarded-Host`.

When the domain is `https`, glim logs a one-line note at startup (and
`glim status` repeats it). It also logs a single warning the first time a request
arrives over plain http with no `X-Forwarded-Proto: https` (or `Forwarded:
proto=https`) header. That usually means the proxy isn't terminating TLS or isn't
forwarding the header, and browsers won't keep the `Secure` sign-in cookie over
plain http.

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
