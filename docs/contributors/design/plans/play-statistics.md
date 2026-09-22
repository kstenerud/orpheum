> **ABOUTME:** Reading back the play log we already keep — the highest demand-to-cost ratio the
> feature sweep found, because 9,196★ of companion applications exist to show people data their
> own server is already storing and never displays.

# Play statistics

- **Status:** UNSPECIFIED — gap identified 2026-09-21 ([D149](../../decisions/working-notes.md)), from [feature-sweep-2026-09-21.md](../research/feature-sweep-2026-09-21.md).
- **Depends on:** [play-state.md](play-state.md) (the log this reads), [auth.md](auth.md) (whose statistics you may see), [performance-targets.md](performance-targets.md)
- **See also:** [observability.md](observability.md) — the *operator's* aggregates; this is the *user's*

## Why this exists

**9,196★ of separate programs exist to show a user their own watch history** — Tautulli 6,589,
Jellystat 2,472, Jellyfin's Playback Reporting plugin 135. Every one of them works by polling
a server that already has the data and will not show it.

[D81](../../decisions/working-notes.md) makes play state an **append-only event log** —
a decision taken for correctness, not for reporting, and it happens to make every one of these
questions a query we can already express. **Nothing in this tree reads it back.**

## What it is

The statistics a household actually looks at, which Tautulli's own home page names:
most-watched titles, most-active users, most-used devices and platforms, busiest libraries,
watch time over a period, and a per-user history with what, when, for how long and on what.

## Three rules that fall out of decisions already made

1. **It is per-user data, so it obeys the role.** A user sees their own history;
   [D130](../../decisions/working-notes.md)'s capability decides who sees the household's.
   "Top users" is an owner's view, not a child's, and a statistics page must not become the
   disclosure channel [D134](../../decisions/working-notes.md) closed everywhere else.
2. **Nothing on a landing surface is computed.** [principles §11](../../principles/README.md)
   and [D134](../../decisions/working-notes.md): "most watched this month" over an event log is
   exactly the shape that measured **43.6 ms against 0.063 ms** from a stored row. A dedicated
   statistics *page* may compute — a person who navigated there is waiting on purpose — and
   anything that appears without being asked for is a **cached result**, invalidated off the
   change journal, not an incrementally maintained counter
   ([D161](../../decisions/working-notes.md)).
3. **This is where [D13](../../decisions/working-notes.md) visibly pays.** The statistics are
   computed locally from a log that was never sent anywhere. That is worth saying in the
   product, once, on this page.

## The questions nobody has answered

- ~~**Retention.**~~ **Answered 2026-09-22 by [D165](../../decisions/working-notes.md):
  there is no horizon.** Once a heartbeat stops being a log row and a run of them becomes one
  watched span, five years of a four-person household is **13.4 MB and 279,453 rows**, and
  every query here gets 28–122× cheaper — *busiest device per month* falls from **4.42 s to
  42.7 ms**. "Watch time in 2019" simply works.
- **Does an export carry it?** [D142](../../decisions/working-notes.md) says an NFO export
  names one user or none; a statistics export is a different question with the same shape.
- **Is there a public-to-the-household view at all**, or only per-user plus owner?

## Done when

A user can answer *"how much did I watch last month, and what"* without installing anything,
the owner can answer it for the household, nobody can answer it for someone whose library they
cannot see, and no statistic appears on a screen without having been maintained rather than
computed.
