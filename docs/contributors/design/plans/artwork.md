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
  already writes thumbnails; a poster at three sizes is the same kind of work. **The core never
  parses an image at all** ([D99](../../decisions/working-notes.md)): TMDB is community-editable,
  so a poster is attacker-supplied in the ordinary sense, and unlike a film it arrives
  unprompted during a scan's fetch. A provider image fetched by the core is bytes in
  flight, handed to the confined worker. Serving a derived image from `cache/` is not parsing
  it. `threat-model.md` counts 39 CISA KEV entries touching media, image or font parsing;
  CVE-2023-4863 (libwebp, 0-click, KEV 2023-09-14) is the one that shipped inside everything.
- **Content-addressed, immutable, cacheable.** The URL contains a hash of the source plus the
  size, so `Cache-Control: immutable` is honest, a changed poster is a new URL, and the cache
  never needs invalidating. This is the same argument
  [D70](../../decisions/working-notes.md) makes for segment URLs, and it must not be
  undermined the same way by per-user signing.
- **A closed set of sizes**, named like the rendition menu ([D21](../../decisions/working-notes.md))
  rather than an arbitrary `?w=` parameter — which is an open grammar, a cache-fragmentation
  machine and a trivial denial-of-service.
- **User-chosen artwork is user data about the *work***, so it is one of the things that *does*
  belong in a sidecar ([D98](../../decisions/working-notes.md)) — where a writable path has been
  granted; in the database regardless. It goes in the change journal and a metadata refresh can
  never replace it. That is the Jellyfin bug above, and it is
  [D7](../../decisions/working-notes.md).
- **All of it lives in `cache/`** except the user's own choice, which does not. `cache/` is
  always safe to delete ([overview.md](../overview.md)).
- **And `cache/` is two caches, one per side** ([D128](../../decisions/working-notes.md)). The
  node produces these images and the core serves them, and nothing said how they cross — which
  is invisible in the default install, where both are the same machine, and is **48 GB** in a
  remote-node topology once trickplay and subtitle overlays are counted. The split is by
  **requests per page**: posters and thumbnails are uploaded to the core, because a home page
  is fifty of them and fifty redirects is a worse page; overlays and trickplay stay on the node
  and the core redirects, the way Netflix's control plane steers a client to an appliance by
  URL. Both halves keep the property that matters — deleting either costs time and nothing
  else.

## The type enumeration, which this plan never stated

[D149](../../decisions/working-notes.md), 2026-09-21. Everything above is about **sizes,
languages, caching and provenance**, and nowhere does this plan say **which kinds of image
exist**. Kodi has a `GetAvailableArtTypes` call; Plex models Art, Poster, Logo, Theme and
SquareArt as separate classes. The ones televisions actually consume are **poster, backdrop,
logo (clearlogo) and banner**; disc art and clearart are Kodi-lineage and cheap to carry
because the NFO/sidecar conventions ([D141](../../decisions/working-notes.md)) already name
them. It is a closed enumeration, which makes it the same kind of object as
[D21](../../decisions/working-notes.md)'s size list.

**And badges are a client render, never a rewritten file.** Kometa's most visible feature is
**24 overlay families** — resolution, audio codec, ratings, content-rating flags, and
`mediastinger` for a post-credits scene — and it implements them by **writing them into the
poster**. That is destructive to an image we do not own
([principles §2](../../principles/README.md)) and it makes the artwork cache
content-unaddressable. If we ever want badges they are drawn over an untouched image by the
client, from fields it already has.

## Open — answered by [D212](../../decisions/working-notes.md)

- ~~**Format.**~~ WebP, re-encoded by the node from every source; the client names the format in
  the URL; JPEG cut by the core from its base on the first request for it
  ([artwork-formats.md](../research/artwork-formats.md)).
- ~~**How many sizes.**~~ Nine named sizes by shape, cropped to it; a phone and a 1080p TV share
  the poster size ([artwork-sizes.md](../research/artwork-sizes.md)).
- ~~**Total cache ceiling.**~~ A setting, starting at 20 GiB, evicting what can be remade first,
  never a live item's poster-wall size, and warning the admin at high water
  ([artwork-cache.md](../research/artwork-cache.md)).

## Done when

A home page of fifty posters loads within the [performance-targets.md](performance-targets.md)
budget on the low-end reference machine; a changed poster appears without a cache purge; a
user's chosen poster survives a metadata refresh, a rescan and a rebuild-from-empty; and
deleting `cache/` costs time and nothing else.
