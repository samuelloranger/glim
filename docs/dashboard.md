# Dashboard

`glim serve` includes a private dashboard at the site root. Use it to see every
live preview, with a live thumbnail and the time it has left. You can extend,
pin, lock or remove a preview, and copy or share its link. **Share** shows the
link as a QR code, to open it on a phone by scanning the screen, with a **Copy
link** button and, where the browser offers one, **Share…** for the system share
sheet. Changes made elsewhere, such as a
new publish from the CLI or an agent, or a preview expiring, show up within
about two seconds without a reload.

Password-protected previews carry a `locked` badge next to their title (the API's
`locked` field). Their thumbnail shows the unlock form. **Lock** asks for a
password (twice) and protects the preview at once; **Unlock** removes the
password. Either change signs out every visitor who unlocked it before.

## The glim bar

When you open a preview while signed in to the dashboard, glim adds a small
floating bar at the bottom of the page. Visitors, scrapers and `curl` never get
it: it is only added when the request carries your valid `glim_owner` cookie
(see [Seen indicator](#seen-indicator)), and everyone else receives the page
exactly as published.

The bar shows the preview's title (or its slug), the time it has left (or
`Pinned`), a lock icon when it is password-protected, a **Copy link** button, a
**Share** button and a link back to the dashboard. Share opens a panel above the
bar with the link as a QR code (drawn from data glim puts in the page, so it needs
no request from the sandbox) and, where the browser offers one, a **Share…**
button for the system share sheet; Escape or Share again closes it. It starts expanded on every load; the arrow
collapses it to a small pill in the bottom-right corner and tapping the pill
expands it again. Nothing is remembered between loads. It floats over the page
without shifting its layout, lives in a closed shadow DOM so the page's CSS
cannot restyle it, and follows the light or dark colour scheme.

Copy link uses the clipboard API and falls back to the older copy command; if
the browser refuses both inside the preview sandbox, the bar shows the link
selected so you can copy it by hand.

Clicks and key presses on the bar stay in the bar: the page's own handlers on
`window` or `document` don't see them (only capturing listeners can), so a deck
that turns the slide on any click doesn't move when you use the bar.

The bar is a serve-time injection, like live reload, so it is not added to:

- HTML larger than 5 MB, non-HTML files, and the password unlock page;
- pages loaded in an iframe (anything but a top-level navigation).

Pages are served with `Vary: Cookie`, and your copy with `Cache-Control: private,
no-store`, so no shared cache ever hands your bar to someone else.

Turn it off for one preview with `--no-header` on publish (or the MCP
`present` tool's `header: false`), which is not remembered on republish, or for
the whole server with `glim config --header=false` (config key `header`).

## Comments

The glim bar has a **Comment** button, with a badge counting the open comments.
It opens the list of comments on the preview. **Add** lets you click the part of
the page you mean (it is outlined as you hover, and the click does nothing on
the page itself), or choose **Whole page**. Write the comment and press
**Comment** (or Ctrl/Cmd+Enter). Open comments pinned to an element show as
numbered pins on the page; tapping a pin opens the list at that comment.

Your agent reads them with the MCP `comments` tool, changes the page, and
calls `resolve_comment`, optionally with a short reply that appears under the
comment (see [Agent integration](./agents.md#comments)). Each comment can be
resolved, reopened or deleted from the list. A comment made before the preview
was last republished is marked "on an earlier version", since the element it
points at may have changed. Dashboard cards show the number of open comments,
and the list refreshes each time you open it, so replies show up without a
reload.

Comments live in `~/.glim/glim.db` and are deleted with their preview. A preview
holds at most 500 comments of up to 4,000 characters. They are only reachable
from the bar, so a preview published with `--no-header`, or a server with the
bar turned off, has no comment UI.

The preview page is sandboxed and sends no cookies, so the bar talks to
`/_glim/api/bar/<slug>/comments` with a token in an `X-Glim-Bar` header, and
glim answers that path's `Origin: null` requests with CORS headers (never with
credentials). The token is minted from your `glim_owner` cookie when your copy of
the page is served: it is bound to that one preview and dies with your session.
The page's own scripts run in the same document and can read it, so a page can
read, add, resolve or delete its own comments, and nothing else. A comment a
page wrote looks like one of yours, author included. The `comments` tool tells
the agent to treat comment text as feedback on the page rather than as
instructions, but if you publish pages that load scripts you don't trust, read
the comments yourself before handing them to the agent.

## Listen

When the server has [Listen](/config#listen) set up, each card has a **Listen**
button and the glim bar has a headphones button. Either one opens the preview's
audio version: **Make the audio version** starts it and shows each step
(reading the page, writing the narration, recording the audio, part by part).
You can close the panel or dialog; it keeps going on the server.

When it's ready you get a player with 1×, 1.25×, 1.5× and 2× speeds, a short
summary, the length, when it was made and which model wrote it, and the full
transcript. **Make it again** writes a new one; after a republish, **Update**
replaces the outdated one. Audio keeps playing when you close the dialog, with
a small bar at the bottom of the dashboard to pause it or open it again; in the
glim bar the headphones button stays highlighted while it plays. Lock-screen
controls show the preview's title where the browser supports it.

Without a speech endpoint glim still writes the summary and the narration, and
**Read aloud** reads it with the device's built-in voice.

The bar fetches the audio with its token, like comments, so it works in the
sandboxed preview page. The dashboard, including when it's installed as an app,
streams it from the API with your session.

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

## Install as an app and notifications

The dashboard is an installable web app. Once installed it opens full screen
from your Home Screen and can send a notification whenever a preview is
published or republished, so you don't have to watch the terminal.

This needs the dashboard served over HTTPS (a domain set with
`glim config --domain https://...`, see [serving.md](serving.md)). Browsers
only allow push notifications on secure origins.

On iPhone (iOS 16.4 or later):

1. Open the dashboard in Safari.
2. Tap Share, then Add to Home Screen.
3. Open glim from the Home Screen icon. Notifications are only available to
   the installed app, not to a Safari tab.
4. Open the account panel, find Notifications, and switch it on. Allow the
   permission prompt.

On desktop browsers the same switch is available without installing.

Each notification reads "`<title> published`" (or "updated" for a republish) and
opens the preview when tapped. There is at most one notification per preview
every 30 seconds, and previews that already existed when `glim serve` started
do not notify. Notifications go to every browser that has them switched on.
Removing an account removes its subscriptions, and a subscription the push
service reports as gone is dropped automatically.

The server generates its push (VAPID) key pair the first time `glim serve`
runs and keeps it in `~/.glim/glim.db`. Deleting that file means every device
has to switch Notifications off and on again.

## Sign-in security

- Accounts and sessions live in `~/.glim/glim.db`, readable only by you.
  Passwords are stored as bcrypt hashes. Only a hash of each session token is
  stored.
- Session cookies are `HttpOnly` and `SameSite=Strict`. They are scoped to the
  dashboard's own paths, so previews never receive them, and they are `Secure`
  when your domain uses `https`.
- Repeated failed sign-ins, and repeated wrong "current password" guesses when
  changing a password, are slowed down per account and per client address.
  Idle entries are evicted periodically, so the tracking can't grow without bound.
  An account that has hit the throttle keeps its failure count through 24 hours
  of inactivity (or until a successful sign-in), so waiting out the backoff does
  not reset it.
  Behind a reverse proxy, the client address comes from `X-Forwarded-For`, but
  only when the proxy connects from a loopback or private address. The
  right-most address that is not itself a loopback or private proxy is used.
  When every hop is private (a client on your own network), the right-most
  hop, which the proxy appended, is used. A client on a private network can
  still prepend a public address that is believed; per-account backoff and the
  per-preview unlock cap bound what that buys.
- The glim bar's comment token (see [Comments](#comments)) only reaches
  that one preview's comments, and dies with the session like the cookie.
- The `glim_owner` cookie is scoped to `/` so it reaches previews. It is a
  signed marker tied to your session. It skips counting your own opens and
  bypasses [locked previews](./serving.md); it dies with the session and
  cannot be used to sign in.
- Previews run as an isolated origin (see [Serving](./serving.md)), so a
  preview's scripts can't act as you on the dashboard.
