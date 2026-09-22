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

**And it is not an open-source failing** ([D139](../../decisions/working-notes.md)). From 2,205
App Store reviews of the commercial services, Disney+: *"No automatic episode tracker. Every
other streaming service that is out there will keep track of what episode you're on… Except
Disney+. Every day, I have to scroll through MULTIPLE episodes to find where I left off."*
Paramount+: *"if I fall asleep watching and want to go back to an episode, when you scroll
down, it defaults to the series very last episode."* Companies that can afford to get this
right do not. **Treat play state as a headline feature rather than plumbing** — it is the
thing a household touches every single day.

## Two structures, not one

### 1. A guarded position

One row per **`(user, file)`** carrying the position **and a monotonic sequence**. A report
whose sequence is older than the stored one is recorded in the log and does not move the
position. Last-writer-wins is the bug; ordering is the fix.

**The position is File-relative** ([D157](../../decisions/working-notes.md), superseding
[D156](../../decisions/working-notes.md) and the Edition-relative rule in
[D81](../../decisions/working-notes.md)). **A File is the only object in the model with an
unambiguous timeline** — a Title has none, an Edition is an editorial fact rather than a
duration, and a Version has one only by concatenating its parts. It is also what the player
actually observed ([principles §13](../../principles/README.md)).

Multi-part gets *simpler*: `12:30 into part 2` **is** the position, and the part arithmetic
happens on read for display. So does the hot path — progress writes are every ~10 s and now
need no arithmetic at all.

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
user-authored, carried in the change journal ([D8](../../decisions/working-notes.md)), and
**never written to a sidecar** ([D98](../../decisions/working-notes.md)): the media tree is
shared with the household and often with a grantee, so per-user state beside a film exposes it
to all of them. They are database-only and therefore backup-critical
([D77](../../decisions/working-notes.md)).

They are in this plan rather than filed separately because they share the failure: Jellyfin
keeps them in the **same single mutable row** as the position, which is why *"Movie favourites
are randomly unfavourited"* (33 comments) sits in the tracker next to the resume bugs. One
race, three symptoms.

**And a watchlist is the one that was missing** ([D140](../../decisions/working-notes.md)).
*"I intend to watch this"* is a different verb from *"I love this"*, and the tree had
favourites, Continue Watching and Next Up but nothing for it. It is also the top navigation
complaint against Apple TV in a 2,205-review App Store sample — *"Finding your wish list or
watchlist is a chore… you have to scroll through at least three pages of the homepage just to
find it"*, and *"I add it to the list, but I don't know where this list is."* Cheap here and
awkward later: a per-user Title relation in this family, on the change journal, database-only
like the rest.

They attach at different levels, and the levels are not the same as the position's:

| Datum | Level | Why |
| --- | --- | --- |
| resume position | **Edition** | [D79](../../decisions/working-notes.md); a 1080p→4K switch must not lose it |
| played / play count | **Edition** | a cut is what you either finished or did not |
| favourite | **Title** | you favourite the film, not the director's cut of it |
| user rating | **Title** | same |
| **watchlist** | **Title** | you intend to watch the work, not a cut of it ([D140](../../decisions/working-notes.md)) |

## Next Up is a query over an ordering, and switching orderings moves nothing

[D159](../../decisions/working-notes.md), [D160](../../decisions/working-notes.md). *Next Up*
is *the earliest unwatched **member** after the latest completed one* — **a statement about a
sequence**, and therefore **derived, never stored**. Switching from aired to DVD order
recomputes it and leaves **every position byte-identical**, which is
[principles §13](../../principles/README.md) working as intended rather than as an act of
care.

**The word is "member", not "episode", and that is the whole generalisation**
([D160](../../decisions/working-notes.md)). A sequence is what *any* Collection produces, so
the same query drives a season, an eleven-film franchise run, and a composed
*Gruelstone → Lockfast*. Every incumbent has two systems here — a series with Next Up and
continue-watching, and a collection that is a static grid of posters — which is why nobody can
binge a franchise. We have one.

Two consequences:

- **The ordering is a preference at series scope** ([D24](../../decisions/working-notes.md)),
  so two people in one household get different Next Up for the same series.
- **Inside a multi-episode file, "next episode" is a seek**, not a load — the boundary is an
  `episode_boundary` segment ([media-segments.md](media-segments.md)), so the prompt fires
  mid-file where every incumbent shows one episode and stops.

**And the ordering is never inferred from behaviour.** Watching S01E05 then S01E03 might be
DVD order or might be somebody skipping about; offering to switch on the strength of it is the
shape [D158](../../decisions/working-notes.md) removed. It is one visible setting.

## The position never rolls up; completion always does

[D157](../../decisions/working-notes.md), 2026-09-22.

| | Attaches to | Rolls up? |
| --- | --- | --- |
| **playback position** | the **File** | **never** — it is a fact about one timeline |
| **completion** (`watched`) | the File, observed | **freely** to Version, Edition and Title — it is a fact about the *work*, so no timeline is involved and nothing can be wrong |

That split keeps the part of cross-version state people notice — *"have I seen this?"* — and
drops the part that cannot be made correct.

**Why "correct" is the word.** Two Versions of one cut do not reliably share a timeline. A
DVD rip carries a studio ident the Blu-ray does not; a PAL transfer runs **4.27% faster**
([D147](../../decisions/working-notes.md)'s own number), so a 116-minute cut is 111:15 and the
error is a **rate**:

| NTSC copy | same moment, PAL copy | drift |
| --- | --- | --- |
| 10:00 | 9:35 | 25 s |
| 40:00 | 38:22 | **98 s** |
| 100:00 | 95:54 | **246 s** |

No offset fixes that, and neither file is wrong.

**And we do not offer to bridge it** ([D158](../../decisions/working-notes.md)). Each file
shows **its own** progress; the user looks at one and scrolls in the other, which is one drag
and always correct. A hint would be wrong in three ordinary cases — it quotes a number 98
seconds out on a PAL pair, it points at a **broken rip** as the place you left off, and on a
hand-edited cut it invents a correspondence only the user knows.

**The general rule this is an instance of** ([principles §2](../../principles/README.md)):
*we group files for browsing and never relate their timelines.* If a feature needs a moment in
one file to correspond to a moment in another, it is out of scope. Multi-part is not an
exception — the parts of one Version are **concatenated**, declared by that Version's own
ordering, and nothing claims a correspondence between them.

**And this is what makes an uncertain Edition harmless.** A library holding four unidentified
cuts of *Glass Runner* has four Files, each with its own position, before and after anyone
labels anything. Getting the Edition wrong costs a misleading group in the UI and nothing
else; relabelling writes one column and moves no state.

**A changed file is a new timeline.** [D116](../../decisions/working-notes.md)'s sampling hash
answers *"is this the same file I saw before"* — so re-ripping a broken copy, or hand-editing
a cut, produces a file whose position starts at zero. The old one is **discarded rather than
carried**, because the moment it named no longer exists. Carrying it forward would point
somewhere wrong and look authoritative doing it.

## Who has play state, and who does not

Every **user** does, including the admin, and independently
([D125](../../decisions/working-notes.md)) — the role model has two roles and no separate
`Profile`, so `(user, edition)` is the only key there is.

A **Watch Together guest does not.** They are not a principal: no user row, nothing persisted
past the session ([D125](../../decisions/working-notes.md),
[D117](../../decisions/working-notes.md)). During a session **the host's position advances**,
because it is the host's library and the host's choice of what plays
([D122](../../decisions/working-notes.md)). This needs stating because the opposite is the
natural implementation — a membership looks enough like a user to get a row — and a guest who
acquires a durable identity is a griefer who survived rotation.

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

**Next Up is also the cast queue** ([D135](../../decisions/working-notes.md)). Casting to your
own television wants "and then the rest of the series", and that is this definition extended
past one item — no new concept, and it inherits the property that two people in one household
can cast the same series in different orders. Two constraints come with it, because a cast
queue is a manifest rendered on a screen: it contains only what the casting user's **role**
permits ([D130](../../decisions/working-notes.md)), and **a pending, unrated item is absent
from it including its title** ([D134](../../decisions/working-notes.md)).

**Extras are in neither** ([D162](../../decisions/working-notes.md)). A trailer you paused is
not something you are partway through, and watching one never marks the film watched — an
extra is a different work, so completion rolls up over *its* Versions and not the film's. The
position is still kept, because positions are per-File and cost nothing, so a 50-minute
making-of resumes when you open it. Extras likewise do not count toward a Collection's
sequence or progress ([D160](../../decisions/working-notes.md)).

An edition appears in exactly one of them. Both are ordinary predicates over the play tables,
so both are [rules-engine.md](rules-engine.md) queries rather than bespoke SQL, which is what
makes "why is this here?" answerable and "remove this from my list" possible without marking
something watched — a request with its own Jellyfin discussion.

## What reads this log back

[D149](../../decisions/working-notes.md), [D150](../../decisions/working-notes.md), 2026-09-21.

**Nothing did, and that is the largest cheap win the source sweep found.** The append-only
event log above was chosen for correctness; it happens to answer every question the **9,196★**
of companion statistics applications exist to answer — Tautulli 6,589, Jellystat 2,472,
Jellyfin's Playback Reporting 135. The surface is now [play-statistics.md](play-statistics.md);
the only thing this document owes it is the **retention horizon**, which is already flagged
open here and which a statistics feature is what makes worth deciding.

**And one honest note about the watchlist.** It entered this plan from
[D140](../../decisions/working-notes.md)'s sweep. Its measured demand is **one issue and three
reactions**, the lowest of anything in this tree we have committed to build — and **Plex
removed the same feature** (*Watch Later*) in 2018, saying it was *"costly to maintain and hard
to justify when only a handful of folks used it."* It stays, for a reason that is not demand:
as a **stored predicate over items** it is one row in a table
[D75](../../decisions/working-notes.md) needs anyway, and
[external-lists.md](external-lists.md) needs the same storage. **What it must never become is a
surface with its own screens and sync semantics** — that is the part Plex paid to remove.

## Done when

Two devices playing one film cannot rewind each other's position; a five-minute short resumes;
an item with unknown runtime is not silently marked watched; an offline device's progress
merges without losing either side; the same episode never appears in both Continue Watching and
Next Up; and a user can ask why an item is in a row and get an answer.
