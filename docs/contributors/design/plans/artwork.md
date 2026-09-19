> **ABOUTME:** Posters, backdrops, thumbnails and logos — where they come from, what sizes
> exist, and why the home screen is an image-serving benchmark rather than a metadata one.

# Artwork

- **Status:** PLANNED — designed 2026-09-19 ([audit-2026-09-19.md](../research/audit-2026-09-19.md) §18).
- **Depends on:** library-model.md, scanner.md

## Why it needs a plan

The home screen is fifty posters. Whatever else the server does, that request is the one every
user makes most often and judges the product by, and nothing in the design says how an image
gets from a provider to a television.

It is also multiplied by two decisions already taken:

- **Artwork is per-language** ([D45](../../decisions/working-notes.md)) — posters have title
  text in the pixels — so the poster axis is `(item, kind, language)`, not `(item, kind)`.
- **Four levels** ([D79](../../decisions/working-notes.md)) — a Title, an Edition and a
  Collection can each have their own artwork, and an Edition without its own falls back to its
  Title's.

And the field's evidence that it goes wrong: Jellyfin's *Some custom primary images/art seem to
have automatically reverted or replaced with defaults* (39 comments) — user-chosen artwork
destroyed by a refresh, which is [principles §2](../../principles/README.md) in image form.

## The shape

- **Derived sizes are produced at scan time, in the node.** The node already opens files and
  already writes thumbnails; a poster at three sizes is the same kind of work. The core never
  resizes an image on the request path, and never parses an image at all — an image decoder is
  attacker-supplied-data surface exactly like a media file
  ([D26](../../decisions/working-notes.md)), and the poster came off the internet.
- **Content-addressed, immutable, cacheable.** The URL contains a hash of the source plus the
  size, so `Cache-Control: immutable` is honest, a changed poster is a new URL, and the cache
  never needs invalidating. This is the same argument
  [D70](../../decisions/working-notes.md) makes for segment URLs, and it must not be
  undermined the same way by per-user signing.
- **A closed set of sizes**, named like the rendition menu ([D21](../../decisions/working-notes.md))
  rather than an arbitrary `?w=` parameter — which is an open grammar, a cache-fragmentation
  machine and a trivial denial-of-service.
- **User-chosen artwork is user data.** It goes in the change journal and mirrors to a sidecar,
  and a metadata refresh can never replace it. That is the Jellyfin bug above, and it is
  [D7](../../decisions/working-notes.md).
- **All of it lives in `cache/`** except the user's own choice, which does not. `cache/` is
  always safe to delete ([overview.md](../overview.md)).

## Open

- **Format.** AVIF is smaller and is not universally decodable on the televisions we target;
  WebP is. The answer is probably "WebP with a JPEG fallback, re-examined when the device lab
  reports", but it is a measurement, not an opinion.
- **How many sizes**, and whether the TV's poster-wall size is the same as the phone's. A
  number for [performance-targets.md](performance-targets.md).
- **Total cache ceiling**, and what evicts. A 20,000-item library at three sizes in two
  languages is a real number nobody has computed.

## Done when

A home page of fifty posters loads within the [performance-targets.md](performance-targets.md)
budget on the low-end reference machine; a changed poster appears without a cache purge; a
user's chosen poster survives a metadata refresh, a rescan and a rebuild-from-empty; and
deleting `cache/` costs time and nothing else.
