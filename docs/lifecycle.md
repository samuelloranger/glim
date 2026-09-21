# Preview lifecycle

Every glim preview has an expiry timestamp. By default, a preview is eligible
for removal once its TTL has elapsed. The lifecycle commands let an operator or
an MCP client reset that deadline or make a preview persistent.

## Goals

- Keep ordinary previews temporary by default.
- Let a user preserve a useful link without republishing its content.
- Give CLI users and MCP clients the same lifecycle operations.
- Keep deletion explicit: neither operation removes a preview.

## Commands

### Extend

```sh
glim extend <name> <ttl>
```

`extend` replaces the preview's expiry with `now + ttl`; it does not add to its
previous expiry. `ttl` uses Go duration syntax, such as `30m`, `6h`, or `48h`.
On success, glim prints the new expiry in RFC 1123 format.

Examples:

```sh
glim extend design-review-abcd 48h
glim extend incident-notes-abcd 30m
```

### Pin

```sh
glim pin <name>
```

`pin` makes a preview non-expiring. It remains available until an explicit
`glim rm <name>` or MCP `revoke` removes it. The existing `expires` timestamp
is retained as metadata, but does not control visibility or garbage collection
while the preview is pinned.

`glim ls` displays `pinned` in the expiry column for pinned previews, and
`glim status` includes their count.

## MCP contract

The stdio MCP server exposes the same operations:

```text
pin({ name }) -> { pinned }
extend({ name, ttl }) -> { name, expires }
```

`name` is the preview slug. `ttl` is a Go duration string. `extend.expires` is
an RFC 3339 timestamp; `pin.pinned` echoes the pinned slug.

## Persistence and expiry rules

Each preview directory contains `.glim.json`. The manifest persists both its
`expires` timestamp and an optional boolean `pinned` field. Older manifests
without `pinned` continue to behave as unpinned previews.

| State | Listed by `glim ls` / MCP `list` | Removed by `glim gc` |
| --- | --- | --- |
| Unpinned, before expiry | Yes | No |
| Unpinned, after expiry | No | Yes |
| Pinned, regardless of expiry timestamp | Yes | No |

Expiry is evaluated when listing or garbage collecting. A preview is expired
only after the current time has passed its stored expiry timestamp. Publishing
also runs garbage collection, so expired unpinned previews may be pruned during
later publishes.

Calling `extend` on a pinned preview updates the stored expiry but leaves it
pinned. There is intentionally no `unpin` operation; remove and republish when
a temporary preview is wanted again.

## Errors and boundaries

- Both operations fail for a preview that does not exist.
- `extend` fails when its TTL cannot be parsed as a duration.
- `pin` and `extend` are metadata operations: they do not alter preview files,
  URLs, titles, projects, or creation time.
- Neither operation revives a preview that has already been removed by GC.

## Verification

The lifecycle behavior is covered by store tests for pinning through expiry and
GC, resetting an expiry from a controlled clock, and missing-preview errors.
MCP tests cover successful `pin` and `extend`, invalid TTL input, and missing
previews.
