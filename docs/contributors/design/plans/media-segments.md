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
| `outro` / `credits` | offer skip, and trigger next-episode |
| `commercial` | skip automatically |
| `unknown` | display only |

## Where they attach: the Version

This is where we differ from Jellyfin, whose `MediaSegment.ItemId` points at a `Video` —
[D79](../../decisions/working-notes.md)'s conflated Edit/Manifestation/Item.

**A timecode indexes an encoding, not a work.** A 4K remux and a web-dl of the same episode
differ in lead-in; a PAL-speedup rip differs throughout. So segments attach at the **Version**.
Deriving a sibling Version's segments by runtime alignment is allowed and cheaper than a second
detection pass, and is recorded as a derivation rather than as an observation.

Each segment carries its **provider** and a **confidence**, and a **user override** beats any
detector and survives re-detection ([principles §3](../../principles/README.md)).

## Detectors, in the order worth building them

1. **Chapter-derived.** Nearly free, and a large fraction of ripped television already carries
   chapter marks with names like "Intro" or "Opening Credits". Jellyfin ships exactly this as an
   official plugin. Build it first.
2. **Audio fingerprint across episodes of a season** — the approach intro-skipper uses and the
   one that works on files with no chapters. It is the same repeated-signal problem Netflix
   solves with audio fingerprinting plus shot-boundary detection plus human review; we get the
   first third of that cheaply because a season is a small, closed corpus.
3. **Black-frame and silence detection** for outro boundaries.

**Detection runs in the node**, because it decodes audio and video and by
[D26](../../decisions/working-notes.md) the core never parses media. It **reports candidate
boundaries**; the core decides what to store ([D42](../../decisions/working-notes.md)). It is
plugin-shaped and should not be first-party at launch beyond the chapter provider.

## Done when

An episode with chapter marks offers a skip button with no configuration; a segment set derived
for the 1080p version is correctly offset for the 4K one or is absent rather than wrong; a user
correction survives a re-detection pass; and turning a detector off removes its segments and
leaves user overrides alone.
