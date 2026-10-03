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

- Every kind's work derived by a sweep from persistent.db; sweeps at start, on their timers, after a
  pass and on unpark. D170's "ever queued" goes: a first match is owed until the item has an answer.
- An item's own failure is a fact in persistent.db with its reason and time, shown and cleared by "try
  again"; a person's refresh is a fact. The D213 §1 recheck is derived.
- Retire actions for a root, a node and a library, cancelling their work; a deleted library's Titles
  tombstoned with their history; "offline since" and the weekly reminder.
- F1: extracted subtitle text moves to persistent.db; nothing expensive to re-derive stays in cache.db.
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

*Done 2026-10-03:* 4.0 to 4e are built, the last an earlier commit. Phases 5 and 6 come next, then M4.

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

*4b.1 built 2026-10-02:* the node holds its work in memory, each job starting once a slot of its
kind is free, the highest priority first (the command carries its job's), and beside its scans
rather than behind them, which settles 4d's devnas finding for prepares; a cancel drops a job that
waits. Every hello names its start (`startId`); a hello from another start ends the waits of the
jobs on that node at once, the tool's failure (`jobs.MarkInterrupted`), and the target's on a
second in a row. Node protocol 2.

*4b.2 built 2026-10-02:* every sending carries a random attempt token, which the node holds the
work by, a cancel names and every result echoes, and a job's wait takes only its own, from the node
it asked (S83, S84); progress has its own path, best effort, and every kind says waiting and running
(`node.progress_every_seconds`); a result names the work it answers (`Result.Work`; a command is an
envelope around its `Work`), is posted until acknowledged, and is recorded by its kind's recorder
as it arrives, by content, whether or not its job waits, before the node is answered (S81, S85):
a making by its run, which a later run's or a late word never undoes (G13). The node remembers an
ended attempt for `node.ended_work_kept_hours`. Node protocol 4.

*4b.3 built 2026-10-02:* a result whose JSON is over 1 MiB (`nodeproto.MaxPostedResultBytes`) is
held by the node, served on the byte socket by its attempt's token, and posted as its job, attempt,
work, size and SHA-256; the core fetches it and takes it only whole, then records it as any other.
Node protocol 5.

*4b.4 built 2026-10-02:* a fetched subtitle is converted by its job, at play priority, while the
fetch waits for its text, at most the silence limit, and is discarded if not converted (D224 §6);
the prepared directory's disk keeps `node.min_free_mib` free (100), evicting for it and refusing a
prepare with nothing to evict, and the core's "room" reads the floor too; a refusal for a file
changed since the scan is named (`changed`) and said apart, tried again as before. Node protocol 7.
4b is built.

### 4c — the core reconciles

- The *assigned* state in use. At hello, and whenever it chooses, the core asks for the node's
  jobs and outcomes and squares them with its own model: adopt, re-issue or cancel, and tell the
  node to drop what the core does not know. Each difference is logged and counted as a broken
  invariant (D234 §6), never repaired silently.
- The rendition index rebuilt from the node's inventory (question D); commands of ended jobs never
  sent.
- *The owner (2026-10-02): "the node will need to be able to date and identify things in its cache
  dir so that it can find after a restart, and evict things the core knows nothing about."* Each
  rendition's manifest (question D) says what it is (the source's identity, profile, gains, run)
  and when it was made, beside the last use its directory's time already keeps (D217 §5), so a
  node that restarts knows what it holds from its disk alone. At reconciliation the core names the
  renditions it knows, and the node evicts the rest, whatever their kept time; a run directory with
  no manifest, from a making that never finished, is evicted too, never while it is being written.
- The intermittent hang in
  `TestJobs_TheViewListsAPrepareAndACancelStopsItOnTheNodeSoTheNextPlayPreparesAgain`, a handler
  failing after its command reached the node, is closed by the cancel on every way out.
- Closes G4, G5, G8, S26, S27, S72, S82, S91.

*As the owner accepted it (2026-10-02, "Yes, use your recommendations"):*

1. A node job's handler sends its command, keeps its attempt token and node on the job row, and
   answers Assign: nothing waits. The node's result, recorded as 4b records it, then resolves the
   assigned job by the token on its row, with the handler's own answers; so it does after a core
   restart. `cache.db` gains the token and node (version 24).
2. Artwork too, as a scan is (the owner: *"the core tells it what images it already has, and the
   node deals with any missing things"*): the core fetches what the job's images need from their
   providers, which only it reaches, and records what is missing or refused there; one command
   carries every image the job still wants, bytes or a path beside the media, with the sizes for
   each; the job is assigned; the node works through them, saying where it is, and answers once for
   all (by reference when large); the result stores the sizes, records each item's image and notes
   what cannot be made.
3. The node's status lists the attempts it holds, with their state, and the tokens whose results
   are not yet acknowledged. The core squares its assigned jobs with that list at each status, not
   by asking: there is no separate question. A node silent past `node_silent_after_seconds` parks
   its assigned work, as now.
4. Squaring, judging only commands the node has taken: held and assigned is adopted, progress
   resuming (S82); assigned and not held, the node restarted, is interrupted, the tool's failure;
   assigned and not held otherwise is a broken invariant, logged and counted per node, and sent
   again; held and not assigned, or ended, is a broken invariant and the node is told to drop it
   (S91, G4). The count is for the System page in Phase 6.
5. A job that ends or is cancelled takes its commands the node has not taken out of the node's
   queue (S27).
6. A manifest beside each rendition: its file, payload hash, profile, run, gains, languages and
   when it was made. The hello lists the node's renditions by their manifests; the core adopts
   those whose source is still that file, which rebuilds the index after a `cache.db` discard, and
   its reply names the renditions it keeps; the node evicts the rest, and a run directory with no
   manifest, never one being written.
7. `assigned_job_limit_hours` (a setting, 24) is the backstop past which an assigned job fails; a
   prepare's failure marks its rendition failed (G8).
8. The steps: **4c.1** tokens on rows, handlers that assign, results that resolve, the limit;
   **4c.2** artwork as one assigned command; **4c.3** the held list in the status and the squaring;
   **4c.4** commands of ended jobs dropped; **4c.5** manifests, the hello's renditions and eviction.

*4c.1 built 2026-10-02:* prepares, extractions, conversions and measurements are given to their node
(`Exchanges.Assign`), their attempts kept in `cache.db`'s `node_attempt` (version 24) and taken back
at start (`Load`); the node's word resolves each by its kind's `Assignee`; a silence past the limit
(`Watch`) or a restart is unanswered, the node told to stop; a cancel tells the node and the kind
(`Queue.OnCancel`); `assigned_job_limit_hours` (24) is the backstop. Only artwork still waits (4c.2).
A node's work is sent once its job is recorded assigned (`AssignThen`), so a quick answer finds it.

*4c.2 built 2026-10-02:* an artwork job fetches what its images need, then gives every image it
still wants to one node in one command (`ArtworkBatch`, each image named by a key), and is assigned;
the node re-encodes each in turn and answers once for all; the answer stores each image as before.
Images beside the media on another node than the first's are left for the next pass. The core
waits on no node now. Node protocol 8.

*4c.3 built 2026-10-02:* the status lists the attempts the node holds and those whose results wait
for acknowledgement, with the last command it took; the core squares them at each status, judging
none whose command is on its way: held is adopted, lost is a broken invariant resolved as the tool's
failure (`ErrWorkLost`), stray work is a broken invariant and dropped; counts per node for Phase 6
(`BrokenInvariants`). Node protocol 9.

*4c.4 built 2026-10-02:* a job that ends or is cancelled before its node took its command has the
command withdrawn from the node's queue (`Service.Withdraw`), never sent, and nothing sent to stop it.

*4c.5 built 2026-10-02:* a made rendition's manifest (`nodeproto.RenditionManifest`, the command
that made it and how) is written beside it while it is still marked being written; each hello lists
the renditions on the node's disk by them; the core adopts those it does not know whose source is
still that file on that node, completes one whose making it names but whose end it did not hear,
and replies with those it keeps (`HelloReply.KeptRenditionDirs`); the node evicts the others it
listed and every run directory without a manifest, never one being written or made since the
listing. A rendition the index holds complete that the node no longer has is not marked at the
hello, which could race a making that ends meanwhile: the play that finds it gone prepares it again,
as before (`PrepareAgain`). Node protocol 10.

*Since 4c.1, for tests:* a job given to a node ends only when the node's answer is taken, not when
its runner returns. A test that answers from a goroutine waits for that answer before it checks the
job: S76 and S29 did not, and failed under the gates' load.

### 4d and 4e, re-cut by D240 (2026-10-02)

The owner stepped back before 4d was built: *"we're best off keeping most of the smarts in the
core"*. D240 records the design: the node knows no library; a scan is a listing the core compares;
the work it implies names operations, one folder per command, answered file by file; the core
schedules by global priority over each node's devices and slots; the node enforces each device's
limit and pauses background work for play. Until it is in use, the protocol and both databases
change freely (the owner: *"I don't care about protocol changes or format changes [...] at this
point"*). The steps below replace 4d's window, in which the node scheduled (D234 §3), and 4e's
scan as the node's walk.

What the earlier 4d and 4e found still holds:

- *Seen on devnas (2026-10-02):* a prepare a person asked for waited behind a 55-second scan of 329
  films. Since 4b the node runs scans apart from the work it holds, so a prepare no longer queues
  behind one; they still share the disk, which 4d.2's pausing settles.
- Since 4c.1 nothing bounds what the core hands a node: a runner gives every due job of its kind at
  once, so a loudness sweep puts all of a library's measurements in the node's memory, where the
  24-hour limit and a restart's interruption reach all of them. 4d.3 bounds it.
- A node's hello asks for no scan of a root no library covers. *Built 2026-10-02, ahead of 4e:*
  after a fresh start on devnas the films page stayed empty for two minutes while that root was read
  ahead of the dev library's own. The owner: *"Just because a potential root is available to
  orpheum does not mean it should scan it. Only when a path is added to a library and its type
  (film, show, home video, etc) is defined should it be scanned."*
- *Seen in 4b's branch gates (2026-10-02), once in sixteen runs:*
  `TestM2_EveryFileHasAFilingAndAReasonAndTheCoreNeverOpensAMediaPath` filed two specials
  (`S00E01`) differently in the replayed scan than in the live one, under the load of the gates'
  parallel steps: where a record landed in the ingest's batches decided a special's filing. Filing
  from what the core holds, once the answers are in (4e.3), ends it.

#### 4d — the node's devices, their limits, and the core's scheduling

- **4d.1 Devices.** The node finds the device of each root and of each mount inside one, and what
  kind it is (spinning, SSD, network, or unknown), following a pool, array or volume to its member
  disks (D240 §5): ZFS through the host's `/run/udev`, mounted read-only into the node's container,
  checked on devnas as a verification item since no ZFS is here. Its hello lists them. Each device's concurrency limit is a core setting per node and device, starting from
  the kind: one for spinning or unknown, several for SSD; the core gives the node its limits in the
  hello's reply and again when one changes.
  *4d.1 built 2026-10-02:* the node finds the devices of its roots and prepared directory at each
  hello (`hardware.Devices`) and lists them; the core's reply gives each the reader limit of its
  kind's setting (`device_readers_spinning` 1, `_ssd` 4, `_network` 2, `_unknown` 1) and logs
  them; `./dev` mounts the host's `/run/udev` read-only. Sending them again when a setting changes comes
  with 4d.2, where the node first uses them. *Since 2026-10-03:* an admin sets a device's limit
  over its kind's (`device_reader_limit`, migration 0030; `PUT /nodes/device-reader-limit`), and
  the System page lists each node's devices with theirs; its controls come with Phase 6. ZFS on devnas is VQ17. Node protocol 11.
- **4d.2 The node enforces and yields.** Every command names the device it reads. The node runs at
  most a device's limit of readers on it, beside the CPU and GPU slots (cut-a §5.3). Play-driven
  work arriving for a device pauses the background work reading it: an FFmpeg's process group
  stopped with `SIGSTOP` and continued with `SIGCONT` when the play-driven work there ends, a hash
  at its next chunk. Paused time is neither a stall nor spent deadline. The status says what is
  paused, and the job shows *"Paused while a film is prepared"*.
  *4d.2 built 2026-10-02:* each command's work reads the device its file is on (`Work.MediaFile`,
  `Devices.KeyOf`) and runs only with a reader of it free; a play-driven command (play priority) that
  finds its device's readers taken pauses the background work there (`execx.Pausing`, `SIGSTOP`
  and `SIGCONT` to the process group, the watchdog's clock moved on by the paused time), which
  continues when it ends; a scan waits between files meanwhile. The status and progress say
  `paused`; the job view shows a job given to a node as running, with *"Waiting for a free slot on
  the media machine"* or *"Paused while a film is prepared"*, where it showed the raw `assigned`
  since 4c.1. Each poll's answer carries the reader limits as the settings say then. Slots are not
  taken from a running job: only readers. Node protocol 12.
- **4d.3 The core schedules.** The core hands a node only what its limits can start now, plus one
  more of each (`node_work_window_extra`, 1), by global priority; a job beyond it waits, parked:
  *"Waiting for a free slot on <node>"*, back when one of that node's attempts of its device or
  slot ends. One runner per kind; `runnerPool` and the hello's prepare slots go.
  *4d.3 built 2026-10-02:* `Exchanges.Assign` gives a node only as many jobs of a slot as it runs,
  and as many reading a device as may read it, each with `node_work_window_extra` (1) more; work
  beyond parks at no cost on the slot or device that is full (`SlotDestination`,
  `ReaderDestination`, *"Waiting for a free slot on node …"*), back when one of the node's jobs
  there ends or it says hello. The slots are `nodeproto.Slot`, the node's and the core's one list.
  One runner per kind: `runnerPool` and the hello's prepare slots are gone. A prepare records its
  making running only once it is given to the node. Play-driven work is never held back for its
  device's readers, which the node pauses background work to give it: the browser tier's timed play
  caught a play waiting 9 s behind background measurements.
- **4d.4 Wanted until (S87).** Each time a play asks for its prepare, subtitle or fetch, the job's
  "wanted until" moves to ten minutes on (question C, a setting); past it, the job falls to
  background priority, in the core's queue and on the node, where it pauses nothing.
- Closes S87, S88.

*4d.4 built 2026-10-02:* a job a person asks for to play is wanted for `play_wanted_seconds` (600)
from the last ask: a play's request, and each request a player makes for a rendition still being
prepared (`KeepWanted`, `cache.db`'s `wanted_until`, version 25). Every 30 seconds the jobs past it
fall to background priority (`DemoteUnwanted`), and the node holding one is told
(`CommandReprioritize`), where it takes its place in line and pauses nothing more. Work a demoted
job had already paused continues as room allows: nothing is preempted. Node protocol 13. *4d is
built.*

#### 4e — a scan is a listing the core compares

- **4e.1 The listing.** The node lists under a path, within its roots, to a depth: each entry's raw
  path, kind, size, modified time and device, or its error, with the root's filesystem facts and its
  mount points. It comes whole, fetched from the byte socket (D240 §8). The core can ask for its
  roots again. It serves the library folder picker when that comes.
- **4e.2 The comparison.** A scan lists a library's root and compares: a file whose size and
  modified time are as stored needs nothing; new or changed is examined; a removal follows D208. The
  skip rules (D40) and the extension lists move to the core as settings. A move is still told by its
  payload hash, once the file at its new path is examined (D240 §8).
- **4e.3 Examining, a batch at a time.** The core sends a folder's work in batches of up to a
  setting's count of videos, each with the sidecars named for it: probe and hash each video, read
  each NFO, hash each subtitle. A batch is answered whole and acknowledged (D240 §8); the core
  ingests it through the existing rules and asks again for a batch a node lost. Identification runs
  as answers arrive. Progress shows beside the root: *"Scanning… N files found so far"*, then the
  files examined (D236 §2); a scan asked for while one runs schedules one rescan after it, *"Scan
  again when this one ends"* (D236 §3, D38).
*Built 2026-10-03, 4e.1 to 4e.3, with their progress on the library screen (D236 §2, §3):* the node lists
(`CommandList`) and examines (`CommandExamine`), each holding only its device's reader, an examining
waiting between files while it is paused for play (node protocol 14, then 15 without the scan
command, the scan's records on the report, or the status's activity). A scan is a `scan` job, which
lists the root, compares the listing by the library's rules (`scan_video_extensions`,
`scan_subtitle_extensions`) and queues `examine` jobs of `scan_examine_batch_videos` videos of one
folder each; it completes, applying its removals, after the last, and one asked for meanwhile runs
once more after it (D236 §3). `cache.db`'s `scan` and `scan_batch` hold a scan in progress (version
26); a scan whose listing the core had taken when it stopped starts again. The node's
`video_extensions` and `subtitle_extensions` are gone from its config.

- **4e.4 The report stream goes.** The status becomes a post of its own, and the walk, its records
  and the ingest's batch path are removed (D234 §8).

*4e.4 built 2026-10-03:* a node posts its status to `/node/v1/status` each tick, and the report
stream, its numbered records and the hello reply's report sequence are gone (node protocol 16);
step 0031 drops `node.last_ingested_report_seq`, and step 0030 folds into the baseline (D228).

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

D233 §5, §6, §10; D241.

- A stall classified from `/proc` (CPU time, bytes read and written, process state, for every
  process in the group): deadlocked, spinning, or waiting on a disk, which parks on that root.
- Output growth as a second signal; loudness and extraction report progress; the core's lease on a
  running job renewed only by movement, waiting for a slot bounded on its own (replacing the
  5-minute silence limit and `maxSlotWait`); a stall recorded as a fact, not a failure's prefix.

**Cut** (2026-10-03, after the owner's answers recorded as D233 §10):

- **5a — the node says why a run stalled.** The watchdog samples `/proc` for each process in the
  run's group at each check: state, CPU time, and bytes read and written. A stalled run is
  *waiting on the disk* when a process is in uninterruptible sleep (`D`), *spinning* when its CPU
  time grew while its position did not, and *stuck* otherwise. Bytes written, or the output a
  caller names growing, is movement as the position is. A run whose group outlives `SIGKILL` (a
  process in `D` ignores it) is answered as stalled once the kill's grace has passed, its slot
  freed, and its process kept in a list of stuck processes until it exits; the node's status
  carries the list. `Stall` carries the condition. Node protocol 17.
- **5b — all the node's work moves or stalls.** A probe and an artwork run get a deadline and are
  stopped as stalled past it. Hashing and listing, which run in the node's own process, say each
  chunk read or entry listed; one that does not move for its window is answered as stalled on the
  disk, its goroutine left to return when the disk does. Every attempt's progress says how long
  since it moved, as the node counts it, paused time apart; a waiting attempt moves when the work
  ahead of it does, so a wait lasts only while something moves.
- **5c — the core holds work only while it moves.** An attempt whose node says it has not moved
  for the core's window, longer than any of the node's own, is stopped and answered as the tool's
  failure; silence still means the node went away. `maxSlotWait` and the prepare's own wait bound
  go. A stall's failure is a fact with its kind (`stalled`, `unreadable`) beside FFmpeg's words,
  not a prefix of them.
- **5d — a disk stall parks its root.** The first stall on the disk makes the file's root
  unavailable, asks for a scan of it, and parks the job on it; the scan that reads it brings it
  back. A second disk stall on the same file after that is the file's own failure, "the disk could
  not read this file". The System page lists a node's stuck processes. `TestScenario_S79_…` and
  `TestScenario_S80_…`.

**Built** (2026-10-03), as cut:

- 5a: `execx` samples `/proc` for a watched run's process group at each check; a stall
  carries its condition (`waiting_on_disk`, `spinning`, `stuck`); bytes read or written, or stdout
  written, are movement; a run whose process outlives `SIGKILL` by `killGrace` is answered at once
  and listed by `Runner.Stuck` until it ends. Node protocol 17.
- 5b: each held job counts its movement (`execx.WithMoved`, `execx.Moved`), paused time
  apart, waiting work moving as the work running does; the status says how long each has not moved.
  A listing or an examining whose reading does not move for the read window (five minutes past
  `probe_timeout_seconds`) is answered as stalled on its disk, its reading listed as stuck. The core
  never takes a stalled answer as a root's contents. Node protocol 18. A probe and an artwork run
  were already bounded by `probe_timeout_seconds`, so they kept it.
- 5c: the core stops work its node says has not moved for `movementWindow`, thirty
  minutes, as a stall (`ErrNotMoving`); one `nodes.StallOutcome` answers every kind's stall.
- 5d: the first stall on a disk makes its root unavailable (`MarkRootUnavailable`), asks
  for a scan and parks the job on the root, the queue keeping that its tool failed; the same stall
  after is the target's own failure, `its disk could not be read`. The System page lists each
  machine's stuck programs.

**Deviations, for the owner:**

- `maxSlotWait` is gone (the owner, 2026-10-03: *"yes, remove the limit"*): waiting is bounded by
  movement (5b, 5c), and the two tests that asserted the cap went with it.
- A stall's failure is not yet a fact with a kind: the failure text begins with the kind's words
  (`FFmpeg stalled twice: `, `stalled on two attempts in a row: `, `its disk could not be read: `).
  A kind matters for a person reading it in their words, Phase 6's job; it moves there.
- A stall waiting on a disk is taken as the media root's disk. A prepare whose FFmpeg waits on the
  prepared directory's disk instead would make the media root unavailable until a scan reads it,
  which the scan does at once. Telling the two apart needs the node to read which file the process
  waits on (`/proc/<pid>/syscall` and its descriptors), which is left for when it matters.
- `TestScenario_S79_…` and `TestScenario_S80_…` move to Phase 6's scenario-tier bullet: S79's and
  S80's parts are tested where they are built (execx, agent, nodes, playback).

## Phase 6 — what a person sees, and the rest of F2

D231 §12; D233 §8; D241; F2.

- D241: a hello marks missing each of the node's library roots it does not name; a Title whose
  every copy is unavailable is listed and marked so, with why and since when; a play of it fails at
  once, naming what is missing, and queues nothing, and a play-driven job whose source goes missing
  is cancelled; an unavailable file can be retired by itself; a root can be repointed to a new path;
  a film whose fetch fails mid-play asks the server why and tells the viewer (§7), with a
  verification-queue item for the browser's playing out its buffer.

- Subtitle and refresh answers can say "waiting for <destination> (since …)" (a play of what is
  known unavailable fails instead, D241 §4); the job view's
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

## After Phase 6 — one instance directory

D242: the instance directory with its small top-level config, `persistent/`, `cache/`, `backups/`
and `run/`; `orpheum default-config <dir>`; every subcommand started with the instance directory;
a backup as a tarball of `persistent/`, the key left out unless asked for, from the admin interface
and the command line; the log written where the full config says; the development loop's own files (`bin`, sample media, browser results, the
seed) out of the instance. Built before the pre-M4 audit.
