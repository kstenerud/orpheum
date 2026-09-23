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

## And we write them back, because that is what the promise says

[D141](../../decisions/working-notes.md). The README already commits to *"sidecar files beside
your media carrying your metadata corrections **in a format any other media server can
read**"*, and [principles §2](../../principles/README.md) calls sidecars *"portability, the
part that means 'survive us'"*. **A portable format only we can read is a contradiction**, so
reading NFO settles the writing question too — the writer is the parser's inverse.

Four rules, and the third is the one that gets skipped:

1. **Write what is not re-derivable** — the user's corrections plus the provider identifiers
   needed to re-find the work, **not the whole scrape**.
   [D77](../../decisions/working-notes.md)'s backup rule in a different medium, and
   [D7](../../decisions/working-notes.md) already says the rest is rebuildable by definition.
2. **Preserve elements we do not model, verbatim.** A read-edit-write cycle must not delete
   what tinyMediaManager or a scraper nobody here has heard of put there. Same instruction as
   [D61](../../decisions/working-notes.md)'s unknown record kinds and
   [D85](../../decisions/working-notes.md)'s unknown fields — third consumer, so treat it as
   the rule it is.
3. **Write `lockdata`/`lockedfields`, or the export is pointless.** Unlocked corrections are
   overwritten by Kodi's first scrape, and the sidecars we carefully wrote become the thing
   that loses the user's work. **A correction must survive as a correction, not as a value.**
4. **Never per-user state.** [D98](../../decisions/working-notes.md) forbids it and the format
   invites it — Jellyfin writes `playcount` and `lastplayed` into the shared file. So our NFO
   is **deliberately lossy**: someone leaving for Kodi takes their corrections and not their
   watch history, which leaves in the [D77](../../decisions/working-notes.md) backup instead.

**Export asks whose watch state to include, or none** ([D142](../../decisions/working-notes.md)),
and the default **follows the number of accounts** ([D143](../../decisions/working-notes.md)):
**one account defaults to that user, more than one defaults to none.** That is not an
exception to [D98](../../decisions/working-notes.md) but its own scope — the rule exists
*because the media tree is shared with the household*, and **with one account there is no
household**. It is checkable rather than guessed, since
[D130](../../decisions/working-notes.md) makes a grantee a user with a role. The single-user
prompt says one more thing, because it is permanent: *"these files stay readable if you add
other people to this server later."*

**An export stops any scan running on that root** ([D143](../../decisions/working-notes.md)).
It removes the contention rather than designing around it — a scan and an export walking the
same tree while one writes is a race — and stopping is nearly free, because
[D102](../../decisions/working-notes.md) makes passes restartable and
[D118](../../decisions/working-notes.md) makes the scan incremental, so a resumed scan picks
up rather than starting over. If the inference is wrong it costs one **rescan** button, which
is the cheaper error. One user, never several — an NFO holds a single `playcount`, so
"everybody's" is not expressible and offering it would misrepresent the file. And the prompt
**discloses rather than asks**: *"this writes what Alice has watched into files beside your
media; everyone in the household, and anyone you share with, can read them"* — the same shape
as [D119](../../decisions/working-notes.md)'s ejection prompt, naming the consequence rather
than the mechanism.

**Three things make the write side harder than the read side**
([D142](../../decisions/working-notes.md)):

- **Granting a writable path needs a node restart.** The permission table in
  [media-nodes.md](../media-nodes.md) *is* the node's Landlock ruleset
  ([D35](../../decisions/working-notes.md)), and a ruleset is declared at startup and cannot
  be widened at runtime ([D107](../../decisions/working-notes.md)). Say so when the path is
  granted, or the user grants it, presses export, and gets a permission error from a component
  that was never going to comply.
- **Read-only is normal and is probed, not discovered.** By our default, by the mount, by the
  NAS, by the filesystem or by POSIX — the user cannot be expected to know which. The node
  reports writability **per root** at startup, so the core knows before it offers the button,
  and **partial success is the ordinary outcome**: a library spanning a writable disk and a
  read-only archive mount exports half, reported per root and per item.
- **Over the file transport there was nowhere to put the content.** The exchange directory had
  no bulk core→node path; `payload/` was added for this.

**Reading needs no permission; writing needs the opt-in writable path**
([principles §2](../../principles/README.md),
[D89](../../decisions/working-notes.md)'s per-library grant), happens in the node
([D33](../../decisions/working-notes.md)), and is triggered **on change, never on a schedule** —
rewriting thousands of files periodically is churn in someone's media tree, which
[D94](../../decisions/working-notes.md) already prices.

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
- Provider matching and its confidence handling are **unspecified**. An ambiguous match now
  has a landing place ([D170](../../decisions/working-notes.md)): it is filed on local data, marked, and fixed through the
  same screen as any other correction.

## Where metadata comes from, and when it is fetched

[D170](../../decisions/working-notes.md), 2026-09-23, by the project owner. **A title follows one source, fetched once.**

- A library has a **default source**. A scan reads the free local sources (filename parse,
  embedded tags, NFO) and fetches the default source **once per newly identified title** —
  including a new episode of a series already here, which is a first fetch, not a refresh.
- The fetch is a **queue behind the scan**, not part of it: a title is listed on local data as
  soon as it is filed. The queue honours `429` and `Retry-After` and a per-provider budget.
  Rate limiting delays metadata, never the library.
- **Per title**, the user may choose another source; the server fetches that one title from it
  and shows the result. **Per field**, the user may type a value, and **a manual value is
  permanent** — it is the lock. Choosing a source for that field is how it is removed.
- **Every refetch is requested by the user** — per title or per library — refetches the
  selected source, and never touches a manual field. There is no TTL-driven refresh.
- Stored: the value as a column; the **selected source and provider ID** per title; **which
  fields are manual**. The last two are what [D141](../../decisions/working-notes.md) writes to NFO. The provider's raw
  response is kept as a cache (principles §14), so a parser change or switching back to an
  already-fetched source costs no call.

Why not keep every provider's value and resolve between them: it needs every source fetched for
every title at scan time. OMDb's free key allows **1,000 requests a day** — 3.5 days for the
reference library's 3,500 films, against about 90 s from TMDB.

## When the edition is unknown

[D155](../../decisions/working-notes.md), 2026-09-22. Most files do not say which cut they
are, so **"identified as this Title, edition unknown" is the ordinary outcome**, not a
failure. The model now represents it directly — a Version may hang off the Title with no
Edition ([library-model.md](library-model.md)) — which matters *here* because this document
exists to be **replayable**: a parser improvement that later determines the cut must be able
to **attach** an Edition to an existing Version, not merge two rows and reconcile watch state
between them.

So the assertion this plan owes: **a Version promoted from no-Edition to a real Edition keeps
its files, its play state and its user-authored data**, and the promotion is an ordinary
replay rather than a special case. Since
[D157](../../decisions/working-notes.md) that is nearly free — the position lives on the File
and no promotion touches it — which is the point: **this document's hardest inference no
longer owns anything the user would miss.**

**And some cuts are not identifiable in principle.** A user who hand-edits a film to remove
scenes they dislike has produced a cut that exists in no database, matches no runtime and
will never be identified. The correct behaviour is not to try harder: it is to leave the
Version without an Edition, let them name one if they want, and **never re-identify over that
name** ([D158](../../decisions/working-notes.md),
[D141](../../decisions/working-notes.md)'s lock markers). Replayability cuts both ways — a
parser that improves must not improve over a human's answer.

## Done when

A pathological real-world library identifies correctly; a parser improvement can be applied to an
existing library **with every node switched off**; a user's manual override survives a
re-interpretation pass, a provider change and a file move; and the log answers "why is this file
not in my library, and why did you call it *that*?" without a debug build.

**And the round trip, which is the only test that proves the portability promise**
([D141](../../decisions/working-notes.md)): import a Kodi library, correct something, export,
and **Kodi reads the correction back and does not re-scrape over it**. Plus a file containing
elements we do not model survives read-edit-write with those elements byte-comparable.
