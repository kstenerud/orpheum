> **ABOUTME:** What the server remembers about what you watched — a guarded resume position and
> an append-only play-event log — and the Continue Watching and Next Up surfaces built on them,
> which are the most-used screens in the product and the most-complained-about in the field.

# Play state

- **Status:** PLANNED — designed 2026-09-19 ([D81](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §3).
- **Depends on:** library-model.md
- **Blocks:** [library-model.md](library-model.md) — the position guard and the event log are schema

## Why it needs a plan at all

Because the field's implementation is one mutable row and the tracker shows what that produces.
Jellyfin keeps `PlaybackPositionTicks`, `Played`, `PlayCount` and `LastPlayedDate` per
`(user, item)` — no device, no sequence, no history — so two clients reporting progress are two
writers to one cell:

| Jellyfin issue | Comments |
| --- | --- |
| *Most movies now appear to be 75% watched in Continue watching* | 44 |
| *Movie favourites are randomly unfavourited* | 33 |
| *'Continue Watching' not displaying on home screen* | 30 |
| *Media playback randomly starts over again or jumps back — STILL* | 29 |

And the absence of history is a whole separate product: **Tautulli, 6,590★**, exists to give
Plex users the watch history and statistics the server does not keep.

## Two structures, not one

### 1. A guarded position

One row per `(user, edition)` carrying the position **and a monotonic sequence**. A report whose
sequence is older than the stored one is recorded in the log and does not move the position.
Last-writer-wins is the bug; ordering is the fix.

**The position is Edition-relative.** A Version may be multi-part, so each File carries its
offset within the Version's timeline and the stored position is in the Edition's — which is
also what makes resume survive the 1080p→4K switch [D79](../../decisions/working-notes.md)'s
fixture requires.

### 2. An append-only event log

`started`, `progress`, `paused`, `seeked`, `completed`, `abandoned` — each with the user, the
edition, the version actually played, the device, the server timestamp **and the client's own
monotonic clock**. That last field is what makes an offline device's events orderable relative
to each other when its wall clock is wrong ([D87](../../decisions/working-notes.md)).

It is the only table in the schema with an unbounded row count, so it carries a stated
retention horizon from the first migration rather than acquiring one after someone's log
reaches a million rows.

### And the rest of the per-user item data belongs here too

Favourites and the user's own rating are the same kind of thing — per `(user, item)`,
user-authored, and therefore mirrored to sidecars and carried in the change journal
([D7](../../decisions/working-notes.md), [D8](../../decisions/working-notes.md)). They are in
this plan rather than filed separately because they share the failure: Jellyfin keeps them in
the **same single mutable row** as the position, which is why *"Movie favourites are randomly
unfavourited"* (33 comments) sits in the tracker next to the resume bugs. One race, three
symptoms.

They attach at different levels, and the levels are not the same as the position's:

| Datum | Level | Why |
| --- | --- | --- |
| resume position | **Edition** | [D79](../../decisions/working-notes.md); a 1080p→4K switch must not lose it |
| played / play count | **Edition** | a cut is what you either finished or did not |
| favourite | **Title** | you favourite the film, not the director's cut of it |
| user rating | **Title** | same |

## Reporting cadence

Plex's is the field's de-facto shape and there is no reason to differ: on every play-state
change, and otherwise **every ~10 s on LAN/WAN and ~20 s on a metered link**. Cadence is the
transport; the log is the content.

## Thresholds are policy, and live in the core

Per [principles §9](../../principles/README.md). Jellyfin's are a reasonable starting point and
two of its behaviours are not:

| Rule | Jellyfin | Ours |
| --- | --- | --- |
| Ignore progress below | `MinResumePct = 5` | same, configurable |
| Treat as finished above | `MaxResumePct = 90` | same, configurable |
| Items shorter than | `MinResumeDurationSeconds = 300` → **marked fully played on open, never resumes** | **no floor**; a five-minute item resumes like anything else |
| Runtime unknown | `data.Played = true` — *"we'll just have to assume it was fully played"* | **unknown**, and it says so |

The second and fourth are the ones to copy down as things not to do: a missing fact must never
become a destructive assertion about the user's state.

## Continue Watching and Next Up

They are two different questions and Jellyfin's users cannot tell them apart —
*"the difference between the two lists is so minute that users often have the same episodes on
both lists at the same time."* State the definitions and make them inspectable:

- **Continue Watching** — editions with a position between the two thresholds, ordered by most
  recent activity. It answers *"what did I stop in the middle of?"*
- **Next Up** — for a series with no in-progress episode, the earliest unwatched episode after
  the latest completed one **in the ordering in use**. It answers *"what comes next?"* — and
  "earliest" is meaningless without [D96](../../decisions/working-notes.md)'s explicit
  ordering, which is why that decision is a prerequisite rather than a neighbour. With it, two
  people in one household can watch the same series in different orders.

An edition appears in exactly one of them. Both are ordinary predicates over the play tables,
so both are [rules-engine.md](rules-engine.md) queries rather than bespoke SQL, which is what
makes "why is this here?" answerable and "remove this from my list" possible without marking
something watched — a request with its own Jellyfin discussion.

## Done when

Two devices playing one film cannot rewind each other's position; a five-minute short resumes;
an item with unknown runtime is not silently marked watched; an offline device's progress
merges without losing either side; the same episode never appears in both Continue Watching and
Next Up; and a user can ask why an item is in a row and get an answer.
