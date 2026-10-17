> **ABOUTME:** Typed time ranges inside a video — intro, recap, outro, credits, commercials —
> the table that holds them, where they attach, and the detectors that fill them. What "skip
> intro" is underneath.

# Media segments

- **Status:** PLANNED — designed 2026-09-19 ([D82](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §4).
- **Depends on:** library-model.md
- **Blocks:** [library-model.md](library-model.md) — the table is schema

## Why this document and not just a plugin

The **detector** was already planned:
[plugins-and-workarounds.md](../research/plugins-and-workarounds.md) §3 calls Intro Skipper
*"the best available specification for D19's tier 3"* and schedules intro/credit detection as
the second tier-3 plugin we write. **What was never planned is the table it publishes into** —
and that is exactly the gap Jellyfin closed in 10.10. The same research records the symptom: the
plugin *"contains an EF migrations directory of its own"*, and then *"the core absorbed the
feature (media segments became native)"*. A plugin carrying its own schema is what a missing
core model looks like.

## The demand

`intro-skipper/intro-skipper` at **2,780★**, plus **968★** on the archived original it forked
from; an open *Skip button for media segments* request on Swiftfin; and on the commercial side
it is the single feature most associated with Netflix's player.

## Segments are not chapters

Jellyfin's own documentation states the reason, and it is correct: *"Unlike chapters, which have
no type, media segments can contain type information, allowing different actions based on the
type of each segment."*

So both exist. **Chapters** are authored container metadata — a list of named marks, whatever
the encoder put there. **Segments** are typed, derived, and carry provenance:

| Type | Action |
| --- | --- |
| `intro` | offer skip |
| `recap` | offer skip |
| `preview` | offer skip |
| `outro` / `credits` | offer skip, **and place the next-episode prompt** |
| `commercial` | skip automatically |
| `unknown` | display only |
| `episode_boundary` | **where one episode ends and the next begins inside a single file** ([D159](../../decisions/working-notes.md)) — makes "next episode" a *seek* rather than a load, and per-episode completion derivable |

**`episode_boundary` is the seventh type and it closes a gap nothing in the tree covered**
([D159](../../decisions/working-notes.md)). Anime and older television rips ship
`S01E01-E02.mkv` constantly, and *multi-episode* appeared in **zero** of this project's 86
documents. The resolution is the inverse of multi-part: where multi-part is one work across
several files, a multi-episode file is several works in one, so it holds **one position**
([D157](../../decisions/working-notes.md)) and the split between episodes is **a timecode
inside that file** — which is what this table already is. Per-episode completion is then
**derived** from the position against the boundary and never stored, so a wrong boundary
costs nothing and fixing it fixes the display immediately
([principles §13](../../principles/13-state-attaches-to-what-you-observed.md)). The detector is already here: a mid-file
boundary sits directly after an `outro`.

**And the `outro` boundary fixes a complaint at the other end of the episode**
([D139](../../decisions/working-notes.md)). Hulu, from the App Store corpus: *"Before the show
ends the interface minimizes the show and you try to see the last 20 seconds and you can't."*
That is a next-episode prompt fired on a **fixed timer** rather than on where the programme
actually ends. With a typed `outro` we place it on content — so a segment model bought for
skip-intro turns out to answer the opposite complaint too, and the two share one table.

## Where they attach: the Version

This is where we differ from Jellyfin, whose `MediaSegment.ItemId` points at a `Video` —
[D79](../../decisions/working-notes.md)'s conflated Edit/Manifestation/Item.

**A timecode indexes an encoding, not a work.** A 4K remux and a web-dl of the same episode
differ in lead-in; a PAL-speedup rip differs throughout. So segments attach at the **Version**.
Deriving a sibling Version's segments by runtime alignment is allowed and cheaper than a second
detection pass, and is recorded as a derivation rather than as an observation.

Each segment carries its **provider** and a **confidence**, and a **user override** beats any
detector and survives re-detection ([principles §3](../../principles/03-explain-the-decision.md)).

## Detectors, in the order worth building them

1. **Chapter-derived.** Nearly free, and a large fraction of ripped television already carries
   chapter marks with names like "Intro" or "Opening Credits". Jellyfin ships exactly this as an
   official plugin. Build it first.
2. **Audio fingerprint across episodes of a season** — the approach intro-skipper uses, the one
   that works on files with no chapters, and — measured 2026-09-20 — **the cheap one**:
   chromaprint runs at **802× realtime** where black-frame detection runs at 13.5×, because one
   is an audio pass and the other decodes every frame
   ([D115](../../decisions/working-notes.md)). It **fails outright on a 5.1 track** and needs a
   stereo downmix in front of it, which is the kind of thing found in production if it is not
   written down. It is the same repeated-signal problem Netflix
   solves with audio fingerprinting plus shot-boundary detection plus human review; we get the
   first third of that cheaply because a season is a small, closed corpus.
3. **Black-frame and silence detection** for outro boundaries — a **second-line** pass for what
   fingerprinting misses (a film, or a season with no repeated theme), not a default. It is 9.8
   single-core days over a reference library against the fingerprint's 4 hours.

**Detection runs in the node**, because it decodes audio and video and by
[D26](../../decisions/working-notes.md) the core never parses media. It **reports candidate
boundaries**; the core decides what to store ([D42](../../decisions/working-notes.md)). It is
plugin-shaped and should not be first-party at launch beyond the chapter provider.

**And the core supplies the set** ([D101](../../decisions/working-notes.md)). Detector 2 works
by comparing episodes of a season — which is library knowledge, produced by
[identification.md](identification.md) in the core from parses the node is forbidden to make
([D59](../../decisions/working-notes.md)). So it is a **job carrying an explicit file list**,
not a pass the node schedules for itself: the core says *compare these twelve*, the node says
*a 91-second segment repeats in all twelve, at these offsets*. Otherwise the node needs its own
copy of the grouping logic, and that is the filename parser again.

## Done when

An episode with chapter marks offers a skip button with no configuration; a segment set derived
for the 1080p version is correctly offset for the 4K one or is absent rather than wrong; a user
correction survives a re-detection pass; and turning a detector off removes its segments and
leaves user overrides alone.
