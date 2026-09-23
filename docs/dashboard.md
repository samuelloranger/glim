# Dashboard

`glim serve` includes a private dashboard at the site root. Use it to see every
live preview, with a live thumbnail and the time it has left. You can extend,
pin or remove a preview, and copy its link. Changes made elsewhere, such as a
new publish from the CLI or an agent, or a preview expiring, show up within
about two seconds without a reload.

## First sign-in

When no account exists, `glim serve` prints a one-time setup code:

```
setup code: K7QM-4XPR-9T — open https://glim.example.com/ to create your account
```

`glim status` shows the same code. Open the dashboard, enter the code, and
choose a username and password (at least 12 characters). The code stops
working as soon as the first account exists.

## Accounts

Anyone who is signed in can add or remove other people from the account panel,
and can change their own password there. Changing your password signs your
other devices out.

From a shell on the server:

```sh
glim user ls
glim user passwd <name>   # reset a forgotten password
glim user rm <name>
```

Removing the last account brings back the setup code on the next `glim serve`
start.

## Sign-in security

- Accounts and sessions live in `~/.glim/glim.db`, readable only by you.
  Passwords are stored as bcrypt hashes. Only a hash of each session token is
  stored.
- Session cookies are `HttpOnly` and `SameSite=Strict`. They are scoped to the
  dashboard's own paths, so previews never receive them, and they are `Secure`
  when your domain uses `https`.
- Repeated failed sign-ins are slowed down per username and per client address.
  Behind a reverse proxy, the client address comes from `X-Forwarded-For`, but
  only when the proxy connects from a loopback or private address.
- Previews run as an isolated origin (see [Serving](./serving.md)), so a
  preview's scripts can't act as you on the dashboard.
