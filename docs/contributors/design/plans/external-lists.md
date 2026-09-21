> **ABOUTME:** A collection whose membership is authored somewhere else — IMDb, Trakt, TMDb,
> Letterboxd — which is a third kind of collection alongside the explicit list and the
> predicate, and the reason Kometa exists.

# External lists

- **Status:** UNSPECIFIED — gap identified 2026-09-21 ([D149](../../decisions/working-notes.md)), from [feature-sweep-2026-09-21.md](../research/feature-sweep-2026-09-21.md).
- **Depends on:** [rules-engine.md](rules-engine.md) (the sibling mechanism), [identification.md](identification.md) (matching a list entry to our library), library-model.md (where a Collection lives), [auth.md](auth.md) (whose provider credential)

## Why this exists

**Kometa is 3,414★** and its whole job is turning lists that live on other people's websites
into collections on your server. Its builder set is assembled from **22 external services**,
with **129 distinct builder keys**, and its shipped defaults say what people actually want:
**16 award sets** (Oscars, BAFTA, Cannes, Emmy, Golden Globe, SAG, Sundance, Venice, Razzies…)
and **10 chart sources** (IMDb, TMDb, Letterboxd, MyAnimeList, AniList, Simkl, Tautulli…).

[D79](../../decisions/working-notes.md) already says a Collection is *"an explicit member list
or a predicate"*. **A list from elsewhere is neither**, and that is the whole design problem:

| Kind | Membership decided by | Refresh |
| --- | --- | --- |
| Explicit | a person, once | never |
| Predicate ([D75](../../decisions/working-notes.md)) | our own fields, continuously | on the change journal |
| **External list** | **somebody else, at a URL** | **on a schedule, and it can change under you** |

## What is hard about it

- **Matching.** A list gives you a title and a year, or an IMDb id if you are lucky.
  [identification.md](identification.md) owns this, and the failure mode is a collection that
  silently contains the wrong film.
- **The members we do not have.** Kometa's `show_missing` and `save_report` options exist
  because the interesting half of an awards list is *what is missing* — which is also
  [D149](../../decisions/working-notes.md)'s retention surface in reverse, and which collides
  with [D134](../../decisions/working-notes.md): a missing-items list rendered to a restricted
  role discloses titles.
- **It goes stale in both directions.** The remote list changes; our library changes. Either
  can make the collection wrong, and only one of them fires our change journal.
- **Credentials and rate budgets**, exactly as [D147](../../decisions/working-notes.md) found
  for subtitles: per-user keys ([D13](../../decisions/working-notes.md),
  [D16](../../decisions/working-notes.md)), a visible budget, and a clean refusal.
- **Provider scope.** Twenty-two services is a plugin catalogue, not a core feature. This is
  [D12](../../decisions/working-notes.md)'s provider tier with a new kind of provider, and the
  core should ship one or two and the seam.

## Done when

An owner can point a collection at an external list, see how many of its members the library
has and how many it does not, have it refresh on a schedule without a manual rescan, and have
a wrong match be correctable by hand — and a restricted role sees neither the missing members
nor anything else the rules would have hidden.
