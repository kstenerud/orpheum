> **ABOUTME:** D324 as it will be built: the core keeps what each session plays and looks at, orders
> the work by it, stops the work in its way, keeps a playing film's CPU, and says how long what it
> waits for will take. The shape, the steps, and the settings it adds.

# Work follows interest

- **Status:** PLANNED — written 2026-10-21 on the owner's word (*"Go with your recommendations. 1:
  next 3 eps. 2: kill only what's in the way. 3: those two are fine."*), and to build (*"Yes sounds
  good"*), the series' place clarified the same day.
- **Depends on:** D324; D234 §3 (a job's priority and its "wanted until"), D240 §5 and §6 (the core
  schedules, the node keeps its devices' limits), D309 (a player's interest in its chunks), D286 §4
  (an encoder that cannot keep up), D226 (loudness measured by the read-through).

## 1. Why

On the owner's staging, 2026-10-21, every film's page said its loudness was not measured, long after
its scans ended: 3,412 read-throughs waited in id order behind the episodes, at about 150 an hour,
and 41 of 324 films were measured. Nothing a person did moved them. Only a play raised work (D234
§3), and a play's work waited for the file another job was reading (D240 §6 pauses between files,
and a read-through is one file).

## 2. Interest

- **The register.** The core keeps, in memory, each session's interest: what it plays, and the
  film or series page it shows, with when each was last said. It is lost with a restart, which
  every page and player says again within seconds.
- **A page says it is shown.** A film's or series' page sends `PUT /interest` with its item while it
  is shown and visible, again every `interest_said_every_seconds` (15), and `DELETE /interest` when
  it is left or hidden (`pagehide`, a request kept alive). A page in a background tab says nothing.
- **A playback is shown by its reports.** The playback service already hears a playing player every
  ten seconds (`POST /playback/{id}/report`); its playback is interest while it is heard, and its
  page's interest gives way to it.
- **Interest lapses** `interest_lapses_after_seconds` (45) after it was last said: a page closed
  without a word, a player that crashed. `play_priority_kept_seconds` (D234 §3's "wanted until")
  becomes this, and is retired.

## 3. What interest raises, and how high

- **The order** (D324 §2): played work at `PriorityPlayNow` (1000, as today); looked-at work at
  `PriorityLookedAt`, between it and a person's request, the latest look highest, so of a few films
  perused the last one's work runs first; a person's request (20) and the background (0 to 2) as
  today. Everyone's interest counts.
- **A film's work** (D324 §3): its files' read-throughs, its artwork's sizes, its subtitles'
  extractions, its metadata fetch when owed, and, for a device whose playback decision needs one
  (decide), its prepared copy. A playing film's work is what its playback is already waiting on.
- **A series' work**: the same for `series_episodes_ahead` (3) episodes in season and number order,
  starting from the session's person's place in it: the episode they are partway through (in
  progress by `in_progress_after_percent` and `finished_after_percent`), the latest played when
  several are; else the episode after the last one they finished, the latest finished; else the
  first. Its artwork and metadata are the series'.
- **Raising** reprioritises the item's jobs that wait or run (`CommandReprioritize`, D234 §3), and
  queues those owed that the sweeps have not queued yet. **Lapsing** gives each its own priority
  again; nothing is cancelled for a lapse, and work that ran on keeps its result.

## 4. Work in the way is stopped

- When raised work waits for what lower work holds on a node, **a device's reader** (D240 §5's
  limit; a spinning disk's one) **or an encoder slot**, the core stops the lowest of the work holding
  it (`CommandCancel` to its attempt) and queues it again at its own priority. A read-through stopped
  part done starts again from its start; a prepare resumes as its own rules allow.
- Work that holds nothing the raised work needs runs on. Of several lower jobs, the lowest priority
  is stopped first, then the newest.
- Stopping is for interest that is current: the core stops nothing for work whose interest lapsed
  while it waited.

## 5. Playing and the CPU

- **A film played as it is, remuxed, or encoded on a GPU** (Quick Sync, NVENC) leaves its node's CPU
  free: other work runs beside it, within its devices' reader limits (§4).
- **A film encoded in software** keeps its node's CPU: no other encode (prepare, chunks) is given
  to that node while it plays, and lighter work (read-throughs, artwork, subtitles) is given while
  the playing run stays above `chunk_run_slowest_speed_percent` of playback speed (D250 §6's
  measure). Below it, the node's other work is stopped (§4) until the run is back above it for
  `chunk_run_speed_window_seconds`.

## 6. How long

- **A read-through**: the bytes left on its device ahead of it and its own, over the device's
  measured read speed, which each finished read-through on that device updates (bytes over seconds,
  kept in memory, `slowest_read_mib_per_second` until one finishes).
- **A prepare**: its progress (D234 §5) gives its speed; before it starts, the node's measured
  realtime factor for its profile, and what is ahead of it on the node's encoder slots.
- **Where it is said** (D324 §6): the film's page, in place of *"Its loudness is not measured yet"*:
  *"Its loudness is being measured: about 3 minutes"*, or *"…is waiting to be measured: about 12
  minutes"*; and the player while it waits for a prepared copy: *"Preparing this film for this
  device: about 2 minutes"*. The API gives a waiting item's work and seconds; the client words it.

## 7. Settings

| Setting | Start | Bounds | Why |
| --- | --- | --- | --- |
| `interest_said_every_seconds` | 15 | 5–120 | how often a shown page says so; sent in `/client-settings` |
| `interest_lapses_after_seconds` | 45 | 15–600 | three missed words, as a node's silence (cut-a §4) |
| `series_episodes_ahead` | 3 | 1–20 | the owner, 2026-10-21 |

`play_priority_kept_seconds` is retired into `interest_lapses_after_seconds`.

## 8. Steps

1. **I1 — the register and its order.** `PUT`/`DELETE /interest`; playback reports as interest;
   lapsing; a film's jobs raised and given back; `PriorityLookedAt`. The film page says it is shown.
2. **I2 — series.** The next episode after the last watched and the two after it; the series page
   says it is shown.
3. **I3 — work in the way.** The core stops the lowest work holding a reader or an encoder slot that
   raised work waits on.
4. **I4 — playing and the CPU.** No other encode beside a software-encoded playback; lighter work
   only while its run keeps up.
5. **I5 — how long.** Each device's read speed and each prepare's speed; the API's waiting work and
   seconds; the film page's note and the player's wait.
6. **I6 — the owner's check on staging**, with a VQ item: open a film whose loudness is unmeasured
   and see its estimate fall and the level arrive; play a series episode and see the next ones
   measured.

Each step is a commit with its tests, and check-pr runs once after I5.
