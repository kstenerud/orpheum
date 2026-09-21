> **ABOUTME:** Turning a node's observations into library items — filename parsing, track-kind
> derivation, provider matching and user overrides. Runs in the **core**, on stored data, so it
> can be re-run without touching a filesystem.

# Identification

- **Status:** PLANNED — split out of [scanner.md](scanner.md) on 2026-09-19 by [D59](../../decisions/working-notes.md); the content is not new, its home is.
- **Depends on:** [scanner.md](scanner.md), library-model.md

## Why this is a separate document from the scanner

[D33](../../decisions/working-notes.md) put scanning in a media node and
[D42](../../decisions/working-notes.md) made nodes sensors. A node sees one subtree, has no
library, no user and no history but its own, and therefore cannot know that the file in front of
it is episode 4 of anything. Everything on this page needs context the node does not have, so it
runs in the core.

The **decisive** reason is not purity, though. It is that **all of this gets better over time**.
A filename parser is a pile of regexes that will be wrong about somebody's release group; a
track-kind heuristic reads strings people typed by hand. If those run in the node, improving them
means re-scanning every node across every filesystem, including the NAS over SMB and the one
that is currently switched off. If they run in the core over stored raw observations, improving
them is a **re-interpretation pass over the database that opens no files at all** — the same
property [D13](../../decisions/working-notes.md) wanted from a rebuildable index.

The input is the node's snapshot and journal ([D59](../../decisions/working-notes.md)): verbatim
paths, probe output, disposition flags and raw track titles.

## The filename parser

**A pure function from path to candidate structure**, with a large table-driven corpus of
real-world names as its test suite. `guessit` is the best prior art, and Kyoo uses a **fork** of
it — which tells you it needed patching. Anime needs explicit handling: absolute numbering,
version suffixes, fansub brackets. The position to adopt, from Kyoo: *"media not being scanned
correctly (even with weird names) is a bug"* — no mandated folder layout.

Because it is a pure function over stored strings, it is also the easiest component in the
project to test and the cheapest to improve: a corpus, a table, and a re-interpretation pass.

**The corpus is Unicode-safe or it is worthless** ([D49](../../decisions/working-notes.md)). Go's
`\w \d \s \b [[:alpha:]]` are ASCII-only, unlike .NET's and Python's, so Jellyfin's episode
expressions ported verbatim fail on `進撃の巨人` — and on **`Amélie`**. `\s` also misses U+3000
IDEOGRAPHIC SPACE, the usual separator in Japanese and Chinese filenames, and U+00A0, which
Windows renamers emit. The failure is **silent**: the expression does not match, the name falls
through to a worse one, and the result is a misparse rather than an error. So the corpus bans
those classes, requires `\p{L} \p{N} \p{M} \p{Z}`, and CI runs every expression against a
non-Latin fixture. This is also the most likely reason the top four Jellyfin metadata plugins are
Chinese-market.

**The filename is a hint, never an authority.** The node has already looked inside the file
([D59](../../decisions/working-notes.md)); where the probe contradicts the name, the probe wins
on matters of fact — codec, duration, track count — and the disagreement itself is worth
surfacing, because a `.mkv` that is not a Matroska file is something the user wants to know
about.

## Deriving track kind

The node reports disposition flags as found and the track title verbatim. Neither is reliable
alone: real files set `FORCED`, `HEARING_IMPAIRED`, `COMMENT`, `DESCRIPTIONS`, `DUB` and
`ORIGINAL` wrongly or not at all, so the title string — "English (SDH)", "Signs & Songs",
"Commentary by the director" — is frequently the only signal there is.

Reading those is a heuristic over human-typed text, which is precisely why it lives here rather
than in the node. Two requirements:

- **Record which source was used** — disposition, title heuristic, or user override — so a user
  correction beats a bad heuristic and is not overwritten on the next pass.
- **Same table-driven, real-corpus test suite as the filename parser.** It is only ever right
  against real files.
- **Audio description is a first-class kind, not a flag we happen to read**
  ([D90](../../decisions/working-notes.md)). It must reach the client contract and the
  preference axes ([D24](../../decisions/working-notes.md)) so "prefer audio description where
  available" is a setting, rather than the user remembering a track number.

## Reading the sidecars that are already there

[D140](../../decisions/working-notes.md). **This was assumed to be planned and was owned by
nobody.** [D98](../../decisions/working-notes.md)'s status says *"NFO is the field's de-facto
answer and reading it is already planned"* — this document covers *writing* overrides back and
never covered importing what exists.

**It is a migration defect pointed at our own promise.** The user this project is for arrives
from Kodi, Emby or Jellyfin with **years of corrections sitting in `.nfo` files beside their
media**, and [D7](../../decisions/working-notes.md) promises a user's corrections survive. A
first scan that ignores them destroys exactly what we promise to protect, silently, at the
moment of highest goodwill. The demand is measured: *Plex NFO Agent Forum Preview* is **291
likes and 510 posts, the most-replied thread on their forum this year**.

So: **on import, an existing sidecar is read as a user-authored override** — the same
precedence as a correction made here, above any provider, and recorded with its source so the
user can see where a field came from. It is **Phase 1**, because first-run is when it matters
and a later fix cannot recover a user who already gave up.

**What is actually in one.** Read locally from Jellyfin's `MediaBrowser.XbmcMetadata`, which
is the de-facto reference implementation: parsers for movie, episode, season and series, over
a field set including `title`, `originaltitle`, `sorttitle`, `plot`, `premiered`, `runtime`,
`mpaa`, `genre`, `country`, `actor`, `director`, `credits`, `ratings`, provider ids, and
artwork paths. Most of it is ordinary metadata we would fetch anyway.

**The valuable part is the two lock fields.** `lockdata` and `lockedfields` are the format's
way of recording *"a human set this; do not overwrite it on refresh"* — Jellyfin maps
`lockdata` straight onto `item.IsLocked`. **That is a user's correction, explicitly marked as
one, in a file they already have.** It is the difference between importing metadata and
importing somebody's work, and it is why this is a
[D7](../../decisions/working-notes.md) concern rather than a convenience.

Three details that will bite.

- **The node reports the sidecar's existence and bytes like any other file**
  ([D42](../../decisions/working-notes.md) — it draws no conclusions); parsing is core work
  over stored observations, like everything else here.
- **NFO is a family, not a format.** Kodi's, Emby's and various scrapers' dialects differ, so
  this wants the same table-driven real-corpus test suite as the filename parser.
- **An NFO can contain per-user state with no user attached**, and we have to decide what to
  do with it. Jellyfin's `NfoUserDataSaver` hooks its user-data events and writes `playcount`,
  `lastplayed` and watched state **into the shared file beside the media** — which is exactly
  what [D98](../../decisions/working-notes.md) forbids us from doing, for the reason D98
  gives: the media tree is shared with the household and often with a grantee. So on import we
  will meet a `playcount` belonging to nobody in particular. **The default should be to ignore
  those fields** — a watch history cannot be attributed safely, and guessing it belongs to the
  importing user would hand one household member another's viewing. Offering it as an explicit
  *"import watch history as mine"* choice is defensible; doing it silently is not.

## Matching, overrides and duplicates

- **Manual identification override that survives re-scans, provider changes and file moves**
  ([D7](../../decisions/working-notes.md)) — written back to sidecars where a writable path has
  been granted ([D98](../../decisions/working-notes.md)), because the database is an index and
  the user's corrections are not. An identification override is a fact about the *work*, which
  is what makes it sidecar-able where a track preference is not.
- **Duplicate and re-encode detection** within a library via perceptual hashing; Stash's
  `goimagehash` is the prior art.
- **People are matched on provider identity, never on the display name**
  ([D95](../../decisions/working-notes.md)). Jellyfin's *Actors with the same name appear as
  one person* has been open since 2020-06, and its music twin states the cause in the title:
  *"artists with the same name are merged while MBIDs are ignored"*. Merging two Person rows
  that turn out to be one human across two providers is the same unsolved merge problem as
  elsewhere.
- Provider matching, its confidence handling, and what happens to an ambiguous match are
  **unspecified** and belong with `library-model.md`'s provenance tables.

## Done when

A pathological real-world library identifies correctly; a parser improvement can be applied to an
existing library **with every node switched off**; a user's manual override survives a
re-interpretation pass, a provider change and a file move; and the log answers "why is this file
not in my library, and why did you call it *that*?" without a debug build.
