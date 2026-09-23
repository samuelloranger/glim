# Dashboard

`glim serve` includes a private dashboard at the site root. Use it to see every
live preview, with a live thumbnail and the time it has left. You can extend,
pin or remove a preview, and copy its link. Changes made elsewhere, such as a
new publish from the CLI or an agent, or a preview expiring, show up within
about two seconds without a reload.

## First sign-in

While no account exists, the dashboard shows a "Create your account" form.
Enter an email address and a password (at least 12 characters). That becomes
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
- Previews run as an isolated origin (see [Serving](./serving.md)), so a
  preview's scripts can't act as you on the dashboard.
