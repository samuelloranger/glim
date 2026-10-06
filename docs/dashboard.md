# Dashboard

`glim serve` includes a private dashboard at the site root. Use it to see every
live preview, with a live thumbnail and the time it has left. You can extend,
pin or remove a preview, and copy its link. Changes made elsewhere, such as a
new publish from the CLI or an agent, or a preview expiring, show up within
about two seconds without a reload.

Password-protected previews carry a `locked` badge next to their title (the API's
`locked` field). Their thumbnail shows the unlock form.

## Seen indicator

Each card shows whether the link has been opened: `seen 3× · 5m ago`, or `not
opened yet`. The same numbers appear as the VIEWS and LAST SEEN columns of
`glim ls` and in the MCP `list` tool's output (`views`, `lastSeen`).

Only real page opens count. glim counts a request when it is a top-level
browser navigation (`Sec-Fetch-Dest: document`) to a preview's HTML page. If a
client sends no `Sec-Fetch-Dest`, a `GET` that accepts `text/html` counts. These
do not count:

- the dashboard's own thumbnails (iframes) and a page's sub-resources;
- bots, link unfurlers and scripted clients, recognised by `User-Agent`
  (bot, crawler, spider, slurp, facebookexternalhit, embedly, preview,
  whatsapp, telegram, discord, slack, curl, wget, python-requests,
  go-http-client);
- you: signing in also sets a `glim_owner` cookie for the whole site (`HttpOnly`,
  `SameSite=Lax`, `Secure` on `https`) so your own opens of your previews are
  skipped. It holds an HMAC of your user id and session under a random server secret,
  and only works while that session is alive: signing out, expiry, a password
  reset or removing the user all invalidate it. Signing out clears it. Browsers that never signed in
  here, or other people, are counted.

Counts are kept in `~/.glim/glim.db`. Republishing under the same name keeps
the count, since the link is the same. Removing a preview (dashboard, `glim rm`,
`revoke`) or letting it expire deletes its count.

## First sign-in

While no account exists, the dashboard shows a "Create your account" form.
Enter an email address and a password (at least 8 characters). That becomes
the first account; the form is gone for as long as any account exists.

Whoever reaches the dashboard first creates that account. Create it right after
you start `glim serve`, before the server is reachable from networks you don't
trust. Until then, `glim serve` logs `no account yet` at startup.

## Accounts

Anyone who is signed in can add or remove other people from the account panel,
and can change their own password there. Changing your password signs your
other devices out.

From a shell on the server:

```sh
glim user ls
glim user passwd <email>   # reset a forgotten password
glim user rm <email>
```

Removing the last account brings back the "Create your account" form.

## Sign-in security

- Accounts and sessions live in `~/.glim/glim.db`, readable only by you.
  Passwords are stored as bcrypt hashes. Only a hash of each session token is
  stored.
- Session cookies are `HttpOnly` and `SameSite=Strict`. They are scoped to the
  dashboard's own paths, so previews never receive them, and they are `Secure`
  when your domain uses `https`.
- Repeated failed sign-ins are slowed down per account and per client address.
  Behind a reverse proxy, the client address comes from `X-Forwarded-For`, but
  only when the proxy connects from a loopback or private address.
- The `glim_owner` cookie is scoped to `/` so it reaches previews. It is a
  signed marker tied to your session. It skips counting your own opens and
  bypasses [locked previews](./serving.md); it dies with the session and
  cannot be used to sign in.
- Previews run as an isolated origin (see [Serving](./serving.md)), so a
  preview's scripts can't act as you on the dashboard.
