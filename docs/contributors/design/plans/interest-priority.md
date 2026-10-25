> **ABOUTME:** D324 as it will be built: the core keeps what each session plays and looks at, orders
> the work by it, stops the work in its way, keeps a playing film's CPU, and says how long what it
> waits for will take. The shape, the steps, and the settings it adds.

# Work follows interest

- **Status:** PLANNED — written 2026-10-21 on the owner's word (*"Go with your recommendations. 1:
  next 3 eps. 2: kill only what's in the way. 3: those two are fine."*), and to build (*"Yes sounds
  good"*), the series' place clarified the same day.
- **Revised by D329** (2026-10-22, [work-scheduling.md](work-scheduling.md)): the interest register
  stays, as a source of wants. Raising and demoting numeric priorities, and stopping work in the
  way by them, give way to tiers, admission and pieces.
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
  stays for a play's own work, its prepare and chunks, which a playback asks for as it plays and
  interest does not find by its film or files; folding the two waits for I4.

## 3. What interest raises, and how high

- **The order** (D324 §2): played work at `PriorityPlayNow` (1000, as today); looked-at work at
  `PriorityLookedAt`, between it and a person's request, the latest look highest, so of a few films
  perused the last one's work runs first; a person's request (20) and the background (0 to 2) as
  today. Below a person's request, **"user has shown interest"** (`PriorityShownInterest`, a rung
  above the background): the work of anything anyone has watched, raised for good, and the work of
  any look or play once its interest is past, where the demotion of work no one waits for now puts
  it (it put it on the background before). Everyone's interest counts.
- **A film's work** (D324 §3): its files' read-throughs, its artwork's sizes, its subtitles'
  extractions, its metadata fetch when owed, and, for a device whose playback decision needs one
  (decide), its prepared copy. A playing film's work is what its playback is already waiting on.
- **A series' work** (D324 §3; the owner, 2026-10-25: *a series' page is its next episode's*): the
  same as that episode's own page raises, for its person's **Next Up** alone, the D160 query over
  the series' ordering (the aired one until a person chooses theirs, D159): the episode they are
  partway through (in progress by `in_progress_after_percent` and `finished_after_percent`), the
  latest played when several are; else the episode after the last one they finished, the latest
  finished; else the first; none once the last is finished. Its artwork and metadata are the
  series'. When that episode's files are not read yet, the look asks for their read before what
  else waits (D304 §4, as its own page does), once for the look. The series' page offers it to play:
  *Continue S1E3* where the person is partway through it, which resumes where they were, else *Play
  S1E4*; where it cannot be played now, why, as its page says. One rule serves both
  (`library.MembersAhead`), and the series' detail (`GET /collections/{id}`) carries it as `nextUp`.
- **A season's work** (D324 §3; the owner, 2026-10-24: interest follows the drill-down, a series'
  page to the series, a season's to the season, an episode's to the episode): a series' page that
  shows one season says which (`PUT /interest/{id}?season=<n>`, 0 for the specials), and its look
  raises the series' own work and `series_episodes_ahead` of that season's episodes, from the
  person's place among them by the same rules; else, when they have watched none of it or finished
  its last, from its first. And it quick-scans the season (the owner, 2026-10-25): as the look
  begins it asks for the read of every unread file of the season's episodes, in episode order, once
  for the look; said again, it asks for none (a read asked for while its scan lists asks for one
  more scan after it, so asking at every word would scan again and again). A session's look at one
  season and at another, or at the series' own page, are different looks. A series of one season
  shows its episodes on the series' own page, whose look is the series'.
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
| `series_episodes_ahead` | 3 | 1–20 | the owner, 2026-10-21; a season's page's since 2026-10-25, a series' own raising its next episode alone |

`play_priority_kept_seconds` stays for now (§2).

## 8. Steps

1. **I1 — the register and its order.** `PUT`/`DELETE /interest`; playback reports as interest;
   lapsing; a film's jobs raised and given back; `PriorityLookedAt`. The film page says it is shown.
2. **I2 — series.** The next episode after the last watched and the two after it; the series page
   says it is shown.
3. **I3 — work in the way.** The core stops the lowest work holding a reader or an encoder slot that
   raised work waits on. *Built 2026-10-21:* D255 §2's bump already stops the lowest work of a lower
   tier where a slot or a device is full, and looked-at work is a person's tier; what was missing is
   that raised work parked on the full room waited there until something ended. The raiser now
   brings that room back, so the raised job's claim bumps; and a person's whole-file read no longer
   waits behind a scan's quick pass, which D255 §3 meant for the background.
4. **I4 — playing and the CPU.** No other encode beside a software-encoded playback; lighter work
   only while its run keeps up. *Built 2026-10-21:* a played encode is any play-priority encode a
   node holds, a quality's run or a prepare a person waits on; a run that falls behind holds its
   node back (nodes.Exchanges.HoldBackFor), and eviction among runs waits a window for it. Between
   two runs of one playback no encode is held, so a queued encode may start in that gap; the hold
   stops it if the next run falls behind. `play_priority_kept_seconds` is not folded: it keeps a
   playback's own prepare and runs wanted, which interest finds by neither film nor file.
5. **I5 — how long.** Each device's read speed and each prepare's speed; the API's waiting work and
   seconds; the film page's note and the player's wait. *Built 2026-10-21* (`internal/core/waits`,
   `GET /files/{id}/waits`): the read speed is a node's, the median of its last five full reads timed
   from when the node said each ran, its slowest read before any; a node reads one file at a time,
   so the bytes ahead are read at that speed. A prepare not yet saying how far it is goes at its
   node's last realtime factor for the profile, or says nothing. The loudness note is the player's,
   under the levelling switch, where the sentence was; the film page has none.
6. **I6 — the owner's check on staging**, with a VQ item: open a film whose loudness is unmeasured
   and see its estimate fall and the level arrive; play a series episode and see the next ones
   measured.

Each step is a commit with its tests, and check-pr runs once after I5.
