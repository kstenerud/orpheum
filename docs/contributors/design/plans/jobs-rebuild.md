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

**Built** (2026-09-30):

- `internal/platform/jobqueue`, generic over the target type (a comparable value it only
  compares) with kinds as names. Its `Storage` interface is `internal/store`'s `JobStorage` on
  cache.db's job table, which keeps a target in its own column per kind (D227 §2); store is now
  the table's one writer. `jobqueuetest.Memory` is the same storage in memory, for its tests.
- Merging (D231 §8): a repeat request merges into the queued job (the higher priority wins, with
  its reason; the soonest time asked for, unless the job is snoozed; the arguments by the kind's
  `ArgsMerge`, the queued job's kept by default). While a job runs, a request it covers is
  dropped (by default, the same arguments) and another becomes the one queued follow-up. A
  fetch's merge is `identify.FetchArgsMerge`, wired in `cmd/orpheum`: a refresh behind a refresh
  asks for the images again if either did, but a match and a refresh never fold into each other,
  since a refresh fetches only what is stored and a match fetches a new season's listing; that
  case keeps the queued job's arguments, as before, until §3's facts.
- Back-off: `job_first_retry_delay_seconds` doubling, `job_attempt_limit`, ±10% jitter on every
  wait the queue computes (back-off, a kind's own schedule, a hold, a tool's retry), none past a
  day. Snooze (`jobqueue.Snooze`) and `RetryAtError` wait at no cost of an attempt, and a repeat
  request does not bring them forward (cache.db's new `is_snoozed`).
- D233 §6: a panic in a handler is recovered and is the tool's failure: free the first time, the
  job's own the second in a row (`last_attempt_tool_failed`). A claim refuses a job whose count
  has passed its limit and fails it unrun, giving back the claim's attempt, so a job that stops
  the server every time ends (S86). What a stop left running keeps its attempt, as before.
- The job view's contract is unchanged; no state was added.
- Scenario tests: `TestScenario_S71_APersonsRefreshBehindARollingOneRunsAtTheirPriorityAndAsksForTheImagesAgain`,
  `TestScenario_S86_AJobThatBringsTheServerDownEveryTimeFailsAtItsLimitAndAPanicFailsOnlyItsJob`.

**Deviations and what Phase 2 must know:**

- A stall answered by a handler (`jobs.MarkStalled`, i.e. `jobqueue.ToolFailed`) stays the
  handler's to count: playback fails a second stall in a row itself, and the queue fails the job
  only on a panic after a tool failure. Two stalls in a row that a handler tolerates are not
  failed by the queue.
- No `discarded` state was added (the survey's River idea): D231 §10 records an exhausted job as
  the item's failure, which §3's facts make, and the job view's words are D231 §12's, Phase 6.
- The extension points: `jobqueue.Claim` is where a claim learns which destinations are parked,
  and each job's destination key belongs in the storage beside its target; `Blocked` and
  `RetryAtError`'s hold on a runner are what parking replaces.
- A graceful stop still costs every running job an attempt, as before (S72).

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
- **Provider services** (the owner, 2026-09-30): each outside host (`tmdb-api`, `tmdb-images`,
  `opensubtitles`) is one service owning its health (the destination state above), a request rate
  we keep ourselves to, its quota remaining and when it resets (OpenSubtitles, from its replies),
  and its credentials' state. A caller says how much its request matters (someone waiting, a
  person's request, background) and asks; the provider answers now, at a time, parked (why), or
  quota spent until a time. Reservations: background work uses at most 80% of TMDB's rate and
  never spends the last 5 OpenSubtitles downloads of the day. Not built: cost models, accounting,
  planners across providers. The TMDB client moves out of identify into this layer beside
  OpenSubtitles.
- Closes G3, G6, G12 (storage as a destination), S25, S29, S70, S73, S76, S77.

**Built** (2026-09-30):

- `internal/platform/jobqueue` keeps a destination beside each job's target (cache.db's
  `destination` column, which replaces the unused `node_id`; cache schema version 21). A handler
  answers `Park` when its destination is down: the job returns at no cost of an attempt, kept with
  it, and a claim passes over every parked destination whose probe is not due. One job at a time
  probes it, on a jittered back-off from 30 s doubling to an hour (the settings
  `destination_first_probe_delay_seconds` and `destination_longest_probe_delay_seconds`, read
  through the queue's `ProbePolicy`), or at the time a 429 named; a
  failed probe costs nothing, a failure that is not the probe's changes nothing, and a probe that
  succeeds unparks it and wakes every runner. A destination only a signal brings back
  (`AwaitsSignal`: a node, a root, a key, a full prepared directory) is never probed. A request at or
  above `jobs.PriorityPersonsRequest` probes its job's parked destination at once. The core's storage
  is a destination: a claim, a record or an answer that SQLite refuses for want of room or with an
  I/O error (`store.IsStorageFailure`) parks it, no runner claims until one claim finds room, and an
  outcome that could not be written is kept and written then. Parked state is in memory.
  `RetryAtError` and `Blocked`, the runner-level holds, are gone.
- `internal/core/jobs` names the destinations (`tmdb-api`, `tmdb-images`, `opensubtitles`,
  `node:<id>`, `root:<node>:<path>`, `prepared:<node>`, `any-node`, `core-storage`), describes them as
  a person reads them, and its view carries each job's destination and, while it waits, the
  destination's state. `outbound` reads only 502, 503 and 504 as a host down; a plain 500 is the
  item's (D231 §5).
- **Provider services** (`internal/core/providers`): each outside host is a `providers.Service`
  owning its health (its destination's state in the queue), the rate we keep to (a token bucket on
  our clock), its quota and reset (from OpenSubtitles' replies), and its credentials' state. Its
  limits are settings, read at each request, each with where its starting value came from
  (`settings.Definition.StartSource`, new): `tmdb_requests_per_second` 40 (the upper range TMDB's
  developer documentation names; the image server keeps to it apart, and documents only 20
  connections at once), `tmdb_background_rate_share_percent` 80 (the owner's choice),
  `opensubtitles_downloads_reserved_for_people_percent` 25 (the owner's choice of a percentage of
  the day's allowance, rounded up to whole downloads: 5 of a free account's 20, 250 of a VIP's 1000;
  the allowance comes from OpenSubtitles' login and account replies), and
  `opensubtitles_requests_per_second` 5 (its best practices). A caller's importance comes from its
  context: a job's priority (`providers.JobImportance`), and outside a job, someone waiting. Within
  a job, the service answers a host down, asking
  to wait or refusing its credentials as the job's parking, which the queue records; outside one, a
  person's call always tries a parked host and counts as its probe. A change of a host's key or login
  (the secret store's new `OnChange`) unparks it. TMDB's client moved to `providers/tmdb` and
  OpenSubtitles' to `providers/opensubtitles`; `clock.Clock` gained `Sleep`, so a wait for our own
  rate is testable.
- The node table (`internal/core/nodes`) says connected (a long-poll open, or heard within
  node_silent_after_seconds), silent, or awaiting its hello. Work is sent with `Send`, which parks the
  job on a node that is not connected; a wait for an answer that runs out parks it when the node
  went away (`Unanswered`). A hello unparks the node and `any-node`; a status with room unparks its
  prepared directory, on which the node's new `no_room` refusal code parks prepares. A job reading a
  file under a root or disk the last scan found missing parks on it (`library.UnavailableUnder`), a
  complete scan that finds them unparks them (`Ingester.SetOnPathsFound`), and the loudness sweep
  passes such files over.
- The API: `GET /jobs` carries each job's `destination` and `waitingFor` (name, reason, since, next
  check); `GET /system` lists the `destinations` that are down and each provider's state, and each
  node's `connection`. The web job view says "Waiting for TMDB: unreachable since …, next check …";
  the System page says a node is silent or awaiting its hello.
- Scenario tests: `TestScenario_S25_ATMDBOutageOvernightParksAndMatchesAllByMorning`,
  `TestScenario_S70_APersonsRefreshWhileTMDBIsParkedLooksAtOnceAndRunsFirstWhenItReturns`,
  `TestScenario_S73_ARefusedKeyParksUntilTheKeyChanges` (cmd/orpheum, on the server's own wiring);
  `TestScenario_S29_APrepareForAParkedNodeSaysWhatItWaitsFor`,
  `TestScenario_S76_ADownNodeBlocksNoOtherNodesWork` (internal/core/api);
  `TestScenario_S77_AFullCoreDiskParksTheQueueSaysWhyOnceAndResumesWhenThereIsRoom`
  (internal/core/jobs).

**Deviations and what Phase 3 must know:**

- "Gone" in the node table waits for Phase 3's retire action, the only way a node becomes gone. A
  retired node's, root's or library's parked work waits on a signal that never comes: the retire
  actions must cancel it (D231 §7), and nothing unparks a retired destination.
- A person's synchronous call to a host that asked us to wait (a 429's Retry-After) waits too,
  answered as rate limited; D231 §6's "always tries" is read as for a host that is down, not one
  that named a time. The owner may want to confirm.
- Jobs queued without a destination (extraction, conversion, loudness, artwork from a local image)
  learn theirs when first claimed: one claim, no attempt. Fetches, prepares and TMDB artwork name it
  when queued, so a person's request can probe it at once.
- When the storage's probe finds nothing to claim, the storage is taken to be back; the next write
  that fails parks it again. A person's own API request whose write fails is answered as the server
  failing and parks nothing until a job meets the full disk. A paid OpenSubtitles download is not
  lost: its link is kept three hours (unchanged).
- Artwork keeps D213 §5's retry schedule for an image's own failures; only its breaker went.
- A job that parks because its node went silent sends no cancel, and a node that returns may still
  be sent commands of jobs that ended (G4, Phase 4). A node's "changed since the scan" refusal is
  still retried, not answered with a rescan (S74).
- The player's words for a parked prepare (S29's Q11) and the System page's rendering of the
  destinations and providers are Phase 6's; the API carries them.
- Phase 3's sweeps run "when a destination unparks" (D231 §3): `jobqueue.Queue.Unpark` wakes the
  runners but has no hook yet for a sweep. A failure a provider service reads as the host's never
  reaches the item: Phase 3's item facts record only what `jobqueue.ParkingOf` does not claim. The
  loudness sweep's missing-root filter (`ListFilesToMeasure`) is the pattern for the other sweeps.

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

**Built** (2026-09-30); the weekly reminder followed the same day (P39):

- Step 0029 (D228: 0028 folded into the baseline) adds `fetch_failure`, `refresh_request`, an
  image's `refused_at`, `refusal` and `is_missing_at_source`, `library_root.retired_at`,
  `root_state.unavailable_since`, `root_mount.missing_since` and `subtitle_text` (cache schema
  version 22 drops cache.db's), and renames the grant kind `invite` to `issued_secret`.
- `jobqueue` runs sweeps (`AddSweep`, `Sweeping`: at start, on each sweep's interval, when asked,
  after every identification pass, and whenever a destination comes back); a derived request looks
  at no parked destination. A kind's `FailureRecorder` records a job that failed at its limit,
  permanently or refused at its claim as its item's own failure; a parked or cancelled job records
  nothing. `EverQueued` and its query are gone. The server registers six sweeps (cmd/orpheum):
  first matches, people's refreshes, rechecks, the rolling refresh, identification passes and
  loudness; each kind's work is derived by one or asked for by a play
  (`TestRunners_EveryKindsWorkIsDerivedByASweepOrAskedForByAPlayAndItsOwnFailureIsAFact`).
- A fetch's own failure is `fetch_failure`, shown on the title, series and identification pages and
  cleared by a person's refresh ("try again"); a refresh is `refresh_request`, queued by its sweep
  until a fetch that read it ends. A recheck is derived from the refused image a CDN's 404 lifetime
  later. A refused image beside the media is owed again once a scan has seen its file since (G9).
- The retire actions (`library.Admin`): `RetireRoot` tombstones the live files the root holds (those
  on disks mounted inside it too), and the root is scanned no more; `RetireNode` does that for
  each of the node's roots in every library, and the node service removes the node
  (`nodes.Service.RemoveNode`), so its name enrols a new node; `Delete` has identification hide the
  library's films, series and episodes (`identify.Pass.HideTitlesOfDeletedLibrary`: tombstones
  with no `merged_into`), then retires its roots and tombstones its files. Each then cancels, by
  `jobs.Queue.CancelRetired`, the queued, parked and running jobs of those files, their streams and
  fetched subtitles, the titles left with no live file, and the destinations it forgets (the roots,
  the disks mounted inside them, a node's own and its prepared directory's), then forgets them. A
  deleted library's film that reappears in another library, by its NFO's provider id or by a
  match, brings its title back with its history (D208 §2). The API: `POST
  /libraries/{id}/roots/retire`, `DELETE /libraries/{id}`, `POST /nodes/retire`; the web offers
  each, confirmed by a second press.
- "Offline since": a scan that cannot read a root, or withholds its removals, keeps
  `unavailable_since` from the first such scan in a row; a disk a scan did not find keeps
  `missing_since`; a scan that finds them clears it. `GET /system` lists them (`offline`), and the
  System page shows each with since when.
- The weekly reminder (P39, the owner's choice 2026-09-30): an admin sees, above every page, how
  many library folders and disks are offline since when, leading to the System page
  (`GET /offline-reminder`). Dismissing it (`POST /offline-reminder/dismissal`) keeps the moment as
  the admin's preference `offline_reminder_dismissed_at` and hides it from them for
  `offline_reminder_interval_days` (7); it comes back after that while anything is still offline.
  Tests: `TestOfflineReminder_ShowsWhatIsOfflineUntilDismissedThenReturnsAfterItsInterval`
  (internal/core/api), `TestOfflineReminder_ADismissalHidesItForTheIntervalFromThatAdminAlone`
  (internal/core/preferences), `src/offline-reminder.test.tsx` and `src/app.test.tsx` (web).
- Tests: `TestScenario_S22_ARetiredRootsFilesAreTombstonesItsWorkIsCancelledAndItsHistoryStays`,
  `TestScenario_S23_ARetiredNodesRootsAreRetiredAndItsParkedWorkCancelled`,
  `TestScenario_S24_ADeletedLibrarysWorkIsCancelledAndItsTitlesHiddenWithTheirHistory`
  (internal/core/api); `TestScenario_S71_ARepeatRequestRaisesWakesAndMerges` (internal/core/jobs);
  `TestScenario_S78_DiscardingCacheDBLosesOnlyTiming` (cmd/orpheum);
  `TestArch_NothingButTheQueueAndTheJobViewReadsJobHistory` (internal/archtest).

**Deviations and what Phase 4 must know:**

- **The weekly reminder is a banner, not a message** (P39): the code has no way to tell the admin
  anything outside a page they open, so the owner chose (2026-09-30) a banner above every page an
  admin opens, which they dismiss for `offline_reminder_interval_days`. An admin who opens no page
  is not reminded.
- A retired node leaves the node table rather than showing "gone": the table lists live nodes, and
  a retired one's name may enrol again as a new node.
- A prepare already queued or running for a retired file on a live node is not cancelled: its
  target is a rendition in cache.db, which the retire does not read. It is play-driven and ends on
  its own; one waiting on a missing root is cancelled with the root's destination.
- ~~A deleted library's series is hidden with its episodes but not brought back when its films
  reappear elsewhere: D231 §7 names a film, and a series has no provider-id lookup across
  libraries yet.~~ Reversed by the owner (2026-09-30): a deleted library's series comes back as a
  film does (D208 §2). Found by the provider id its tvshow.nfo gives, it comes back when its
  episodes are filed elsewhere, each episode with it as its file reappears; found by a match, the
  new series joins it, an episode at a place it had giving that episode its files and one it did
  not have moving into it, and the new series is left a tombstone pointing there.
  `TestPass_ADeletedLibrarysSeriesThatReappearsElsewhereComesBack`,
  `TestMatch_ADeletedLibrarysSeriesMatchedAgainElsewhereComesBack` (internal/core/identify).
- The retire's steps owned by other packages (hiding titles, removing the node) are separate
  transactions from the library's: a failure between them is finished by retrying the action.

## Phase 4 — the core drives the node

> **The re-cut, accepted by the owner (2026-10-01: "yes to your proposals").** Phase 4 was planned
> on D233 before D234 revised it, D235 put core and node in separate containers, and D236 added
> the scan's display. The questions at the end are answered as proposed.

D233 §2, §4, §6, §8, §9 as D234 keeps them; D234 throughout; D235 §4 and §6; D236 §2 and §3.

**What D234 settled of the readiness audit's open questions** ([audit-2026-09-30](../research/audit-2026-09-30.md) §2):

| Audit question | Settled by |
| --- | --- |
| 1. A job on a node with nobody waiting for it | D234 §7: a job given to a node is a row in the *assigned* state, resolved by its outcome, a timeout, a cancel or parking, never a waiting goroutine |
| 2. Awaiting hello | D234 §6: the core expects updates from a node with work; after a few intervals of silence it asks, and a node that does not answer is unreachable and its work parks (D231 §5). *How many intervals is question A below* |
| 3. The outbox | D234 §3, §5: the node's journal holds its jobs and their outcomes, each resent until acknowledged; §4: a node whose disk is full refuses new work. *Retention is question B. D238: the journal is the node's memory, not a `node.db`* |
| 4. The attempt token | D234 §3: every job, update and cancel names its attempt; the core deduplicates by result identity and attempt (§5) |
| 5. The rendition inventory | Not settled: question D |
| 6. A handler failing after its command reached the node | D234 §4, §6: both. The core cancels on every way out, and its reconciliation finds what a lost cancel left, reporting it as a broken invariant |
| 7. The node as its own service | D235 §4: `orpheum-core` and `orpheum-node`, two services of one image, restarted by the runtime; D234 §4: a hung node is the service manager's. *The dev loop's shape is question F* |
| 8. "Wanted until" | D234 §3: past it, the work falls to background priority, not cancelled. *How long is question C* |
| 9. What a person sees | D231 §12's rule holds; the words for Phase 4's new waits are question E |

**The steps.** Each ends with `make check-pr` green and the dev loop working, as every phase does.

### 4.0 — preparation, no behaviour change

- The clock's timers, and a bounded receive for tests; then a lint that refuses a bare receive
  from a channel in a test outside that helper, so a wait cannot hang a run (audit-2026-09-30
  §5.3; the owner, 2026-10-01: "Yes, all three").
- One request/answer envelope and one `nodes` seam that the five round trips use today
  (prepare, subtitle, loudness, artwork, scan), replacing a goroutine blocked on each answer.
- `nodes/service.go` split along that seam.
- The queue's *assigned* state, built and tested in `jobqueue`, not yet used. *As the owner accepted it
  (2026-10-02, "agreed"):* a handler answers `Assign(until)`, and the job is assigned, its runner
  free; the node's outcome is taken later by the runner's `Resolve`, through the same answers as a
  handler's; an assigned job past `until` is its own failure and spends an attempt (whether a node
  gone away parks it instead is 4c's reconciliation); assigned counts as running for one job per
  target, and for a cancel; a restart leaves it assigned, for 4c's reconciliation at hello.
  `cache.db`'s job gains the state and `assigned_until`.

### 4a — the node stands alone

- `orpheum-node` runs as its own process: the core no longer starts it as a child (D43 and D207's
  start end, as D233 §2 and D235 §4 say), and the supervisor goes. Node work is tied to the node,
  not to its session with the core, so a dropped connection cancels nothing.
- The hello carries a protocol number; a node whose number the core cannot serve is parked with
  that reason; an unknown request is refused explicitly (D233 §9).
- Closes S89.
- *Built 2026-10-02:* a node's work runs on its own lifetime; the hello carries
  `nodeproto.ProtocolVersion`, a node on another is refused and shown as needing an update, and a
  command it cannot read is refused by its job's id; `orpheum core` and `orpheum node`
  are two services, `run`, the supervisor and its two settings are gone, and the dev loop runs the
  two as two processes.

### 4a′ — CI runs in the development image

Not jobs work, but due before 4b (the owner, 2026-10-02): D235 §2 has CI run in the same image as
every other machine, and CI's first run since the tool step changed failed only because its runner's
make (4.3) differs from the image's (4.4.1). CI runs `./dev make check`, `./dev make
test-integration` and `./dev make check-pr`, in place of installing Go, Node, shellcheck and
Playwright's libraries; the image's layers are kept in GitHub's build cache, so a run builds it only
when the Containerfile or a pin changes; and `./dev` learns to keep Go's and npm's caches in a
directory CI saves, as setup-go does now. Publishing a prebuilt image stays shelved until the
public beta. The nested gate steps then run in parallel in CI too (make 4.3 ran them at -j1).
*Built 2026-10-02, to be confirmed by its first run on GitHub:* `./dev --image-tag` keys a saved,
zstd-compressed copy of the image, and `ORPHEUM_DEV_CACHE_DIR` keeps Go's, npm's and Playwright's
caches in a directory the workflow saves; the runner installs nothing but runs `./dev`.

### 4b — the journal and acknowledged outcomes

- The node's jobs by id and attempt, ordered within their priorities; a job running when the
  node restarted is reported interrupted. *D238: held in memory, not in a `node.db`.*
- Progress pushed while a job runs, throttled, newest superseding the rest; an outcome inline when
  small, by reference when large (fetched from the byte socket and checked by hash), kept and
  resent until acknowledged.
- An answer is taken only from the node the job was sent to; the node's "changed since the scan"
  refusal is retried, not answered with a rescan (S74); the negative ids of conversions done while
  a person waits go (O2 replaces them).
- A node whose disk is full refuses new work with its reason (D234 §4): its prepares wait and say
  why, and work that writes elsewhere goes on (S28).
- Closes G13, S28, S81, S83–S85.

*As the owner accepted it (2026-10-02, "Sounds good"), with D234's words made concrete, and as
D238 then revised it (the same day): each fact has one owner, and the node keeps no database.*

1. The node holds its work in memory: each command as it came, with its attempt token and
   priority; its unacknowledged outcomes; and the identities (token, kind, when it ended) of work
   that ended, to know a repeated command. No `node.db` (D238 §3).
2. Every hello carries an id for this start of the node. A node that restarted has lost the work
   its process ran; the core finds the difference at reconciliation (4c) and asks again, counting
   the work interrupted: the tool's failure source as a stall is (D233 §6), free the first time
   and the item's the second, so a file that crashes the node does not loop (S86).
3. Acknowledged means recorded. The core records every outcome as it arrives, whether or not a job
   still waits, each kind checked against what is current by content (a prepare by its rendition's
   run id, closing G13 and S81; a measurement or subtitle by the file's hash; artwork by its
   target), and only then answers. A job that waits is told the outcome. The node resends an
   unacknowledged outcome at each hello and with a doubling wait while connected; once it is
   acknowledged the node keeps only its identity, for a week (question B as D238 §2 revised it).
4. Every sending carries an attempt token, random, which the result echoes: a job's wait takes
   only its own token, from the node it sent to (S83, S84). Attempt numbers cannot fence, since an
   attempt given back is numbered again, and job ids repeat after a `cache.db` discard. Anything
   else goes only to the recording of item 3.
5. A fetched subtitle converted while a person waits is the conversion job that exists, at the top
   priority, and the request waits, bounded, for its text to be recorded: one way to ask a node for
   work, and no negative ids.
6. An outcome up to 1 MiB (a protocol constant) is inline; a larger one (artwork, a large
   subtitle) is held by the node, within the result's 64 MiB bound, and named in the result with
   its size and SHA-256; the core fetches it from the byte socket and checks it.
7. Progress has its own path, best effort and never resent, at most every
   `node.progress_every_seconds` (a node config key starting at 2, today's constant). Every kind
   says waiting for a slot and running; a prepare also says how far. Commands carry their priority,
   and the node starts what waits in priority order.
8. The "changed since the scan" refusal gets its own code and is retried with back-off as today;
   the scan that notices the change ends the old file's jobs.
9. A node whose prepared directory's disk has less than `node.min_free_mib` free (a node config key
   starting at 100; the owner, 2026-10-02: "a node balk and report full when less than 100mb space
   remains. Configurable"), beside the directory's own limits (D217 §5), refuses new work that
   writes there with its reason, and the core parks it until the node's status shows room, as
   prepares wait on a full prepared directory now (S28). Work that writes nothing on the node goes
   on.

The protocol version becomes 2. The steps: **4b.1** the work held in memory, run in priority
order, and the start id; **4b.2** tokens, the progress path, acknowledged outcomes and their
recording; **4b.3** large outcomes by reference; **4b.4** conversions as jobs, the changed
refusal, the full disk.

### 4c — the core reconciles

- The *assigned* state in use. At hello, and whenever it chooses, the core asks for the node's
  jobs and outcomes and squares them with its own model: adopt, re-issue or cancel, and tell the
  node to drop what the core does not know. Each difference is logged and counted as a broken
  invariant (D234 §6), never repaired silently.
- The rendition index rebuilt from the node's inventory (question D); commands of ended jobs never
  sent.
- The intermittent hang in
  `TestJobs_TheViewListsAPrepareAndACancelStopsItOnTheNodeSoTheNextPlayPreparesAgain`, a handler
  failing after its command reached the node, is closed by the cancel on every way out.
- Closes G4, G5, G8, S26, S27, S72, S82, S91.

### 4d — windows, slots and priority

- The core gives each node a window of work a little beyond its free slots and keeps global
  priority; the node schedules within it (D234 §3), replacing `runnerPool`.
- A play-now prepare pauses background reads on its node; play-driven work carries "wanted
  until" and falls to background priority past it (question C).
- *Seen on devnas (2026-10-02):* the node runs its commands one at a time and a scan in line with
  them, so a prepare a person asked for waited behind a 55-second scan of 329 films: the player's
  playlist answered "being prepared" from 05:19:05 until the scan ended at 05:19:38. Play-driven
  work must not queue behind a scan or background work on the node.
- Closes S87, S88.

### 4e — a scan is a job

- A scan's result is a record stream the core reads from an offset, which retires the report
  stream (D234 §8).
- Its pushed progress shows beside the root: *"Scanning… N files found so far"* (D236 §2); a
  scan asked for while one runs schedules one rescan after it, and the button says *"Scan again
  when this one ends"* (D236 §3, D38).
- A node's hello asks for no scan of a root no library covers: on devnas (2026-10-02), the dev
  loop's start read all 329 films of a root no library covered yet, for 58 seconds, and stored
  nothing ("a node scanned a root no library covers; what it saw is not stored").
- Last, because it moves the oldest path, the report stream, onto everything 4b–4d built.

S90 moves to a small step of its own with its schema change; S79 and S80 move to Phase 5.

**The owner's answers** (2026-10-01, every one as proposed). Each number is a best guess and a
setting, read where it is used (D200):

- **A. Silence.** How long a node with work may stay silent before the core asks what it is doing:
  proposed three of its status intervals (`status_interval_seconds`, 10 s in the dev config).
- **B. Retention.** How long the node keeps an outcome after the core acknowledged it: proposed a
  week, bounded by the cache cleanup, oldest unacknowledged first under pressure (D234 §5).
- **C. "Wanted until".** How long after the last person waiting stops a play-driven prepare keeps
  its play priority: proposed ten minutes.
- **D. The rendition inventory.** Rebuilding the core's index from the node needs a manifest beside
  each rendition (its profile, the source's identity, the gains, the run that made it), which
  nothing writes today. Proposed: the node writes one with each rendition, and reconciliation reads
  them.
- **E. What a person sees** for the new waits, in D231 §12's manner: a node on a version the core
  cannot serve (*"The media machine needs updating"*), a node that has not answered since the
  server restarted (*"Waiting for <node> to say what it is doing (since …)"*), and work waiting
  for a free slot (*"Waiting for a free slot on <node>"*).
- **F. The dev loop.** In the development image, core and node as two processes in the one
  container that `./dev make dev` starts and restarts (proposed), or two containers as in
  deployment.

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
- Every balk on a person's request says why and until when (D231 §12's 2026-09-30 note), closing
  the gaps Phase 2's report listed: no screen reads `retryAfterMs`; "unavailable", node-away and
  credential refusals carry no since or until; parked prepares look like "preparing". A test per
  kind of balk: a 429's countdown and automatic retry; a parked host's check-now, back, still-down
  states; quota's reset; pacing past a couple of seconds; credentials' who-can-fix-it; a play whose
  node is away or whose root is missing.
- `internal/core/playback` split along the map's lines: play state, subtitle fetching, node work,
  and the levelling decision into `decide`.
- The scenario-tier tests for every scenario the phases closed that is not yet one; D231's and
  D233's Status lines read back and updated.
