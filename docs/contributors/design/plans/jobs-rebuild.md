> **ABOUTME:** The order in which D231 (derived work, a regenerable queue, destinations that park)
> and D233 (work handed to a node survives restarts and lost messages) are built, phase by phase,
> each ending green and each closing named gaps and scenarios.

# Rebuilding jobs: the D231 and D233 build

[D231](../../decisions/working-notes.md) and [D233](../../decisions/working-notes.md) replace how
work is queued, run, retried and handed to nodes. They are built in phases so that each ends with
`make check-pr` green and the dev loop working, and so that a phase's report can be checked before
the next depends on it. The scenarios are [scenarios.md](scenarios.md) S22–S29, S70–S91; the gaps
G3–G15 are listed there. The research is
[queueing-2026-09-30](../research/queueing-2026-09-30.md). The architecture map's F1, F2 and F5
([architecture-map.md](../architecture-map.md)) are built here too.

Every phase: behaviour changes carry tests that fail when reverted; a scenario the phase closes
becomes a scenario-tier test; schema changes follow D228's fold rule; the decisions' Status lines
change only after their clauses are read back against the code (P31).

## Phase 1 — the generic queue package

D231 §2, §8, §10; D233 §6 (panic recovery, claim-time attempt check).

- A package that knows no Orpheum kind, target or destination: claiming, priority, merging a
  repeat request (higher priority, a job in back-off woken, at most one queued and one running per
  target), back-off with ±10% jitter capped at a day, snooze without an attempt, cancel, crash
  recovery, a claim that checks the attempt count, and recovery from a panic in a handler. Its
  storage is an interface that `internal/store` implements on cache.db.
- `internal/core/jobs` becomes Orpheum's use of it: the kinds, their targets and their handlers,
  moved over without losing behaviour; the job view keeps its contract.
- Closes G11 (repeat requests) and the crash loop of S86.

## Phase 2 — destinations and parking

D231 §5, §6, §11 (the node table's truth), §12 (the job view's words); replaces D213 §5's breaker.

- Destination state: `tmdb-api`, `tmdb-images`, `opensubtitles`, each node, each missing root or
  mount, the core's own storage. What counts as down (connection, DNS, timeout, 502/503/504; a
  node not connected or silent; a missing root; a failing write), a missing or refused key parking
  `tmdb-api` until the secret changes, a node's "no room" parking its prepares.
- Parked work is not claimed and spends no attempt; one probe per outside host on a jittered back-off
  to an hour; a person's request probes at once; a synchronous call is the probe; a node's hello
  and a scan finding a root are theirs. Work resumes in priority order.
- The node table says connected, last heard and gone truthfully; a command to a node that is not
  connected parks its job.
- Where the TMDB client lives (it serves both identify and artwork): with the outside hosts, not
  under identify.
- Closes G3, G6, G12 (storage as a destination), S25, S29, S70, S73, S76, S77.

## Phase 3 — derived work and facts

D231 §3, §4, §7, §9; F1.

- Every kind's work derived by a sweep from library.db; sweeps at start, on their timers, after a
  pass and on unpark. D170's "ever queued" goes: a first match is owed until the item has an answer.
- An item's own failure is a fact in library.db with its reason and time, shown and cleared by "try
  again"; a person's refresh is a fact. The D213 §1 recheck is derived.
- Retire actions for a root, a node and a library, cancelling their work; a deleted library's Titles
  tombstoned with their history; "offline since" and the weekly reminder.
- F1: extracted subtitle text moves to library.db; nothing expensive to re-derive stays in cache.db.
- One schema step: the facts, the retire records, subtitle text, and the grant kind `invite` renamed
  `issued_secret` (it covers registration invitations and device codes).
- `TestScenario_S78_DiscardingCacheDBLosesOnlyTiming`, and an architecture test that nothing reads
  job history as a fact. Closes G7, G9, G10, G14, G15, S22–S24, S71, S78.

## Phase 4 — the link to the node

D233 §2, §3, §4, §7, §9.

- The node as its own service (`orpheum-node`), the core watching only that it lives.
- Results in an outbox on the node's disk, acknowledged as the report stream is; results known by
  what they are, attempt tokens, an id for the core's run; the core ignores duplicates and keeps a
  still-valid late result.
- "Awaiting hello" after a core restart; reconciliation at hello (running, finished-unacked,
  renditions on disk; the core adopts, re-queues or parks, rebuilds its rendition index from the
  inventory, and says what to stop); commands of ended jobs never sent; a dropped session no longer
  cancels node work.
- Per-node slots in claiming; a play-now prepare pauses background reads on its node; play-driven
  work carries "wanted until".
- A protocol number in the hello; an incompatible node parked; an unknown command refused.
- Removes the negative ids of conversions done while a person waits (O2 replaces them).
- Closes G4, G5, G8, G13, S26–S28, S72, S79–S85, S87–S91.

## Phase 5 — the watchdog's second stage

D233 §5.

- A stall classified from `/proc` (CPU time, bytes read and written, process state, for every
  process in the group): deadlocked, spinning, or waiting on a disk, which parks on that root.
- Output growth as a second signal; loudness and extraction report progress; the core's lease on a
  running job renewed only by movement, waiting for a slot bounded on its own (replacing the
  5-minute silence limit and `maxSlotWait`); a stall recorded as a fact, not a failure's prefix.

## Phase 6 — what a person sees, and the rest of F2

D231 §12; D233 §8; F2.

- Play, subtitle and refresh answers can say "waiting for <destination> (since …)"; the job view's
  parked states; the System page's destinations and queue health; finished rows pruned after 30
  days.
- `internal/core/playback` split along the map's lines: play state, subtitle fetching, node work,
  and the levelling decision into `decide`.
- The scenario-tier tests for every scenario the phases closed that is not yet one; D231's and
  D233's Status lines read back and updated.
