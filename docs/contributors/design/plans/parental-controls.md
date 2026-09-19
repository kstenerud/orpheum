> **ABOUTME:** Age ratings and who can watch what — why a certification is not a number, and
> why a kids profile turns out to be a share with a different name.

# Parental controls

- **Status:** PLANNED — designed 2026-09-19 ([D84](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §6).
- **Depends on:** [rules-engine.md](rules-engine.md), library-model.md
- **Blocks:** [library-model.md](library-model.md) — certifications are schema

## The demand

`jellyfin-androidtv`'s **third-most-reacted issue** is *PIN protection for users* (+56), behind
only the playback rewrite and SyncPlay. This project's whole positioning is a **household** —
"a household of people who are not going to read documentation" — and a household with children
in it cannot use a media server that has no answer here.

## Netflix's model, including the part everyone forgets

Two levels, and the second is the one homegrown implementations miss:

- **Per profile:** a profile-lock PIN, a maturity-rating ceiling, and explicitly blocked titles.
- **Per account:** **a PIN required to create a profile.** Without it a child makes an
  unrestricted profile and every other control is theatre.

Copy both. The second costs one setting and closes the only bypass that matters.

## A certification is `(system, code)`, not a number

Netflix's own help text: *"Maturity ratings are either set by Netflix or by a local standards
organization... Different countries use different rating systems."*

BBFC 15, FSK 16, MPAA R, ACB MA15+ and PEGI 18 are not points on one scale, and TMDB returns
certifications as a **list keyed by country**. So:

- a certification row is `(item, system, code)`, many per item;
- **ordering is defined per system**, as a curated data table — there is no authoritative
  machine-readable source covering every national system, so it starts with the handful the
  providers actually return and grows;
- the household declares which system it uses, which is a locale-shaped choice sitting beside
  [D45](../../decisions/working-notes.md)'s language handling rather than a new axis.

**Flattening this fails open**, which is the direction that matters. An MPAA "R" mapped onto a
single integer scale and compared against an FSK ceiling produces a number that is wrong in an
unpredictable direction — and a wrong rating ceiling shows a child something, rather than
hiding something from an adult.

**An item with no certification in the household's system is unrated, not permitted.** Unrated
is a third state and whether to show it is an explicit choice, not a fall-through.

## The access half is a predicate

A kids profile is **a grantee plus a predicate** — [D76](../../decisions/working-notes.md)
again, and therefore a **fifth binding** of [rules-engine.md](rules-engine.md) alongside smart
playlists, smart collections, retention and sharing. "Rating at or below X in system Y, and not
tagged `horror`" is a rule.

It inherits D76's two rules, both correct here:

- **It fails closed.** An unresolvable predicate grants nothing. A migration must never widen
  what a child can see.
- **Enforce at every access path, not only at listing** — omit from the manifest so a correct
  client never asks ([principles §10](../../principles/README.md)), *and* deny by id, because a
  restricted profile is a different trust domain from the account owner.

## What it is not

Not content filtering, not scene skipping, not scanning. It is a visibility predicate and a PIN.
[content-safety.md](content-safety.md) is a different subject entirely.

## Done when

A child's profile shows only what the predicate allows; the same items are unreachable by id;
a new film appears in or out of that profile according to its certification without
intervention; an item with no certification in the household's system is handled by a stated
rule rather than by accident; a child cannot create a profile; and the owner can see exactly
why a title is hidden.
