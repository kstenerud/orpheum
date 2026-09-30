> **ABOUTME:** User-story scenarios for Cut A: concrete things the two people do to their library
> and their viewing, each with what they should see and what the model must hold, walked through
> the design to find where it is silent or wrong before the code meets the same case.

# Scenarios

- **Status:** M2's scenarios walked through 2026-09-24; M3–M5's written, to be walked before
  each of those milestones starts. Asked for by the owner: *"We haven't done user story
  scenarios yet to test our model."*
- **Depends on:** [library-model.md](library-model.md), [scanner.md](scanner.md),
  [identification.md](identification.md), [play-state.md](play-state.md),
  [backup-restore.md](backup-restore.md), [schema.md](schema.md).

## Why

Plans describe mechanisms and decisions answer questions, one at a time. Nothing walked a
sequence of ordinary events through all of them and asked *what does each table hold now, and is
that what the person expects?* The first walk found a decided defence that was never built
(agent-failures A3) and three questions nobody had asked.

## How to read an entry

**Story** is what a person does. **Expected** is what they should see and what the model must
hold. **Design** names the decisions that answer it. **Status** is one of:

- **holds** — built, with the test that shows it;
- **holds on paper** — the design answers it and the code does not reach it yet;
- **gap G\<n\>** — the design answers it and the code does not do it; to build;
- **question Q\<n\>** — the design is silent or contradicts itself; for the owner;
- **walk before M\<n\>** — not walked yet.

Every M2 scenario that the code reaches becomes a test in the scenario tier: the ingest,
identification and API driven by scripted node reports, with no filesystem and no nodes, which is
what [D42](../../decisions/working-notes.md) says the core's decisions can be tested with.

The people are **the owner**, at home, admin, and **the partner**, remote, a user
([D174](../../decisions/working-notes.md): invited from outside).

## M2 — the library

### S1 — Fixing a typo in a filename

- **Story:** the owner renames `Metropolis (1959).mkv` to `Metropolis (1995).mkv` in the same folder.
- **Expected:** the same film, not a new one: its row, play state and any correction follow it.
- **Design:** a new path with a payload last seen under this root is a move (D179).
- **Status:** **holds**: the file's row follows it
  (`TestIngest_D179MatchesAFileByItsPayloadWhereverItIs`), and it is identified again from its new
  name (`TestPass_ARenamedFileIsIdentifiedAgainUnlessConfirmed`, D208 §1).

### S2 — Fixing track titles with mkvpropedit

- **Story:** the owner sets a film's title and renames its German audio track in place.
- **Expected:** the same file; the new track name shows; nothing else changes.
- **Design:** a tag edit keeps the payload hash (D179); the probe is re-read and the streams
  re-derived.
- **Status:** **holds** (`TestPayloadIdentity_D179TableWithTheRealTools`; the ingest re-derives streams
  on every observation).

### S3 — Replacing a film with a better copy at the same path

- **Story:** the owner overwrites `Metropolis (1995).mkv` with a 4K remux of the same name.
- **Expected:** the film is still watched; the old file's resume position does not carry to the
  new file, because two files' timelines are never related.
- **Design:** a different payload at a path is a new file and the old row is tombstoned (D173);
  watched and identification survive at the Title (D157, D173).
- **Status:** file level **holds** (the D179 test's replaced case); Title level **holds on paper**.

### S4 — Keeping the 1080p beside a new 4K

- **Story:** the owner adds `Metropolis (1995) 2160p.mkv` next to the 1080p file.
- **Expected:** one film with two playable entries, told apart by resolution and filename; not a
  quality menu, and no "Unknown Edition".
- **Design:** two Versions under the Title, edition unknown (D155).
- **Status:** **holds on paper**; identification builds it.

### S5 — Reorganising folders inside a library

- **Story:** the owner moves twenty films into `Crime/`, `Sci-Fi/` and so on, within one root.
- **Expected:** all twenty are the same films, with their history.
- **Status:** **holds** (the D179 test's moved case).

### S6 — Moving a film to another library

- **Story:** the owner moves a film from *Films* to *Kids*, a different root.
- **Expected:** the same film, still watched by whoever watched it.
- **Design:** a payload match in another root is a duplicate, not a move, because that root was
  not looked at (fixed 2026-09-24, `TestIngest_AFileInAnotherRootIsNeverTakenForAMove`); the old
  row is tombstoned when its own root's next scan completes. A Title belongs to a library
  (`item.library_id`).
- **Status:** **holds** for a film with a provider id: its Title, with no live files left in
  *Films*, moves to *Kids* (`TestPass_ATitleFollowsItsFilesToAnotherLibrary`, D208 §2). A film
  with no provider id arrives as a new Title.

### S7 — A USB disk unplugged, leaving an empty mount point

- **Story:** the disk holding *Films* is unplugged. `/media/films` is now the empty directory
  under the mount point, and a scan runs.
- **Expected:** nothing is removed; the films show as unavailable; the admin is told the root
  looks unmounted.
- **Design:** the silent case (D41): the listing succeeds and is empty, no error fires. The node
  reports each root's device id and mount facts and tells "listed, empty" from "not
  enumerated"; the core compares the device id against what it recorded and withholds removals
  on implausible shrinkage, **asking** rather than refusing (D41, D42).
- **Status:** **holds** (`TestScenario_S7_S8_AnUnpluggedDiskRemovesNothingAndItsReturnRestoresEverything`,
  `TestScenario_S7_RemovalsPastTheThresholdWaitAndFewerGoAhead`): the node reports the root's
  device and mount facts; a changed device or mount, or a removal past D208 §3's threshold, is
  withheld and recorded in `root_state` with its reason. The admin's confirmation, which applies
  a withheld removal, comes with the scanner view.

### S7b — A disk unplugged inside a root, with /mnt as the root

- **Story:** the owner's root is `/mnt`, with disks mounted at `/mnt/disk1` and `/mnt/disk2`.
  `disk1` is unplugged; meanwhile a film on `disk2` is deleted.
- **Expected:** `disk1`'s films stay, unavailable; the deleted film goes.
- **Design:** the root's own facts do not change, and `/mnt/disk1` lists as an empty directory. The
  node reports each mount boundary it crosses and the core compares them with the last complete
  scan's (D208, as corrected; D42).
- **Status:** **holds** (`TestScenario_S7b_ADiskUnpluggedInsideARootRemovesNothingUnderIt`,
  `TestScenario_S7b_ADifferentDiskAtAMountPointHidesTheOldOnesFiles`,
  `TestScenario_S7b_AnEmptiedDirectoryThatWasNeverAMountIsRemovedAsUsual`). Asked by the owner
  after the first walk, which had missed it.

### S8 — The same disk plugged back in a week later

- **Story:** the disk comes back and a scan runs.
- **Expected:** every film as it was, with its history.
- **Design:** "recovery restores; it does not re-add" (D41).
- **Status:** **holds** (the same test). While it was away, `root_state` recorded the root's
  outcome; showing its films as unavailable (D179) is the scanner view's and browse's.

### S9 — A disk whose mount point is gone too

- **Story:** the directory itself is missing, or unreadable.
- **Expected:** nothing removed.
- **Status:** **holds** (a root error withholds every removal:
  `TestIngest_OnlyACompletedScanRemovesAndNeverWhereItCouldNotRead`).

### S10 — Deleting a film by mistake, and putting it back

- **Story:** the owner deletes a film in the file manager, a scan runs, and the next day they
  copy it back from their own backup to the same place.
- **Expected:** the film is back with its history: they watched it last month.
- **Design:** the scan tombstones it (correctly: it was gone). When the same payload reappears,
  the ingest looks only at live rows, so it makes a new row, and the history stays on the
  tombstone. D41 says a returning mount is restored, not re-added; it does not say what happens to
  a returning file.
- **Status:** **holds** (`TestScenario_S10_AFilmPutBackRevivesItsTombstone`, D208 §4).

### S11 — A copy of a film in a second folder

- **Story:** the owner copies a film to `To Watch/` and leaves the original.
- **Expected:** both play; the admin view shows them as duplicates, naming both paths (D179 §4).
- **Status:** **holds** (`TestIngest_ACopyIsADuplicateAndTheRecordedPathKeepsTheIdentity`); the
  admin view is M2's scanner view.

### S12 — A scan cut off by a reboot

- **Story:** the machine reboots mid-scan.
- **Expected:** nothing removed; the next scan finishes the job.
- **Status:** **holds** (the same test). A file moved within its root appears only after a scan
  completes, because a move is decided at completion; recorded, not a gap.

### S13 — A film still downloading during a scan

- **Story:** a scan reaches a film that is half copied.
- **Expected:** it appears once it is complete, and nothing odd happens to it later.
- **Design:** a truncated container is hashed whole; the complete file has a payload hash, so the
  next scan sees a different payload at the path and replaces the row.
- **Status:** **holds on paper**, with one row of churn. Harmless unless someone started playing
  the half-copied file; not worth a mechanism now.

### S14 — A wrong match fixed by hand, then the file renamed

- **Story:** identification matched the wrong *Heat*; the owner picks the right one; later they
  rename the file.
- **Expected:** the right *Heat*, still.
- **Design:** a confirmed match is permanent (D172); the file's row follows the rename (D179).
- **Status:** **holds** (`TestPass_ARenamedFileIsIdentifiedAgainUnlessConfirmed`: a confirmed
  match does not move).

### S15 — Arriving from Kodi with years of NFO files

- **Story:** the owner points a library at a Kodi library with `.nfo` files and locked fields.
- **Expected:** the locked fields come through as corrections; the rest of each NFO is the
  source where the library was told to use NFO; nobody's `playcount` is imported.
- **Design:** identification.md, Reading the sidecars (D140, D182, D183).
- **Status:** **holds** for NFO read (`TestParseNFO_AKodiMovieWithItsCorrectionsLocked`,
  `TestPass_TheDevLibrarysFilms`): the node sends NFO bytes as sidecars, locked fields become
  overrides, watch counts are ignored. A conflict with a later edit here is shown once overrides
  have their API.

### S16 — An NFO edited in Kodi after a correction here

- **Status:** **holds on paper** (identification.md: the later change wins and the conflict is
  shown).

### S17 — The database lost, with a backup

- **Story:** the disk holding `library.db` dies; the owner restores last night's backup and the
  server scans.
- **Expected:** everything as of last night; files added since appear as new.
- **Status:** **holds on paper**: restore is built (M1) and the ingest matches by path and
  payload. Becomes a scenario test.

### S18 — The database lost, with no backup

- **Expected:** the library rebuilds from the files; watch history and corrections not in NFO
  files are gone. The backup page said so ([backup-restore.md](backup-restore.md)).
- **Status:** **holds on paper**.

### S19 — Libraries built from symlinks, as Jellyfin users do

- **Story:** a migrating owner has *Kids* as a folder of symlinks into *Films*.
- **Expected:** it works; each film in both.
- **Design:** links into a media root are followed (D206); the same files seen through two roots
  are duplicates, and each library has its own Titles.
- **Status:** **holds** at file level; watch history is split between the two Titles, which is
  **Q2** again.

### S20 — A new season arrives

- **Status:** **holds** for a TMDB library (`TestSeries_ANewEpisodeFetchesItsSeasonAgain`): the
  series is matched once, each season's listing is fetched once, and a new episode missing from a
  listing fetched before it arrived fetches that listing again (D209 §3).

### S21 — A name only macOS writes

- **Story:** `Fièvre.mkv` copied from a Mac arrives decomposed.
- **Status:** **holds** (`TestIngest_APathIsStoredInNFCBesideItsBytes`).

## M3 — play in a browser (walk before M3)

- **S30** — The owner starts a film on the laptop, pauses at 1:12:00, and resumes on the phone in
  bed: it starts at 1:12:00.
- **S31** — Two devices report out of order; the newer position is never rewound (M3
  acceptance).
- **S32** — The owner resumes a film whose file was replaced by a better copy (S3): what do they
  see? The position does not carry (D173); is anything offered?
- **S33** — The owner hides a film from Continue Watching and takes it back.
- **S34** — The partner fetches Spanish subtitles for a film; later the owner renames the file
  (the subtitle follows the row), and later still replaces it (does the partner spend another
  download?).
- **S35** — Dialogue boost on the TV; `cache.db` deleted; the next play prepares it again.
- **S36** — An MKV with TrueHD and ASS plays in Chrome, Firefox and iPhone Safari with the reason
  on screen (M3 acceptance).

## M4 — two people, one remote (walk before M4)

- **S40** — The owner invites the partner; the partner signs in from home; the owner revokes
  access; the partner's next request fails.
- **S41** — Watch Together across two houses for a whole film: one pauses, the other's network
  drops for a minute, they rejoin and stay in sync.
- **S42** — A friend joins Watch Together without an account; their viewing is not recorded; the
  partner's is.

## M5 — the remote TV (walk before M5)

- **S50** — The partner watches a whole film on their TV with voice on the phone, and resumes it
  at home later from where they stopped.
- **S51** — "Get this ready" before a film night; it plays over a congested link without
  stalling.

## Running it (walk before M3)

- **S60** — An upgrade runs a migration: a snapshot is kept, `/health` says migrating, and a
  failure leaves the server up saying why (M1, holds).
- **S61** — A typo in the config file: doctor names it, and the server runs on the starting
  value.

## Jobs and destinations (walked 2026-09-30; decided by D230, D231)

### S22 — A disk holding a root dies for good

- **Story:** the disk under *Films* fails and is never coming back.
- **Expected:** nothing is removed until the admin says so, and the films show as unavailable.
  No job hammers the dead disk. The admin sees "offline since", is reminded after a week, and
  retires the root; its files become tombstones, and their history stays on the Title.
- **Today:**
  - The scan withholds removals (S7/S9), and `root_state` records why. The files stay live, so
    every job for them runs.
  - The node refuses each command (the file is missing). Each refusal is an ordinary error and
    spends an attempt, so the job fails after about 7.5 minutes.
  - `QueueMeasurements` queues them all again every hour, forever.
  - A local image beside the media gets the node's refusal recorded as the image's own
    (`recordRefusal`). It is then "held", so it is never owed again, even if the disk returns
    (G9).
  - There is no retire action and no reminder.
- **Design:** the owner's three answers:
  1. A root or mount that a scan found missing is an unavailable destination. Its jobs park, and
     sweeps queue nothing for it.
  2. A retire action at each level cancels the jobs of what it removes (D173).
  3. Offline never ends without the admin (D41); the System page shows "offline since", with a
     reminder after a week.
- **Status:** **holds**, but for the weekly reminder (G7, open: P39)
  (`TestScenario_S22_ARetiredRootsFilesAreTombstonesItsWorkIsCancelledAndItsHistoryStays`, the
  jobs rebuild's Phase 3): the System page says since when a root or disk is offline, and the
  admin's retire tombstones its files, cancels their work and forgets the root. G9 **closed**
  2026-09-30: a refused image beside the media is owed again once a scan has seen its file since.
  G6 **closed** 2026-09-30 (the jobs rebuild's Phase 2): work reading a
  file under a missing root or disk parks on it, a local image included, and the loudness sweep
  passes such files over
  (`TestLoudness_AFileUnderAMissingRootIsNotSweptAndItsMeasurementWaitsForTheRoot`,
  `TestUnavailableUnder_AFileUnderAMissingRootOrDiskWaitsAndAScanThatFindsThemSaysSo`).

### S23 — The node machine is retired for good

- **Story:** the owner decommissions the second machine and its node never says hello again.
- **Expected:** the admin retires the node. Its roots are retired, which means their files are
  tombstoned and their jobs cancelled; history stays on the Titles. Until then, its jobs wait
  without spending anything.
- **Today:**
  - The core cannot tell. `connectedNodes` never forgets a node, so `IsConnected` stays true
    until the core restarts. Commands pile up in memory.
  - Each prepare, subtitle or loudness job waits `maxSilence` (5 minutes; loudness waits longer,
    in proportion to file size) and spends an attempt. The artwork job waits 2 minutes and runs
    through D213 §5's 9-day schedule.
  - After a core restart, `Enqueue` silently drops the commands.
  - There is no retire action.
- **Design:** D231: node jobs wait while the node is configured, and removing the node cancels
  them. Answer 2 covers the retire action.
- **Status:** **holds** (`TestScenario_S23_ARetiredNodesRootsAreRetiredAndItsParkedWorkCancelled`,
  the jobs rebuild's Phase 3): the admin retires the node; its roots are retired, its parked work
  cancelled, and it is removed, so its name enrols a new node. G3 **closed** 2026-09-30: the node table says connected, silent or
  awaiting its hello truthfully, and its jobs park on it at no cost
  (`TestNodeConnection_IsConnectedWhileItPollsOrWasHeardLatelyAndSilentOtherwise`).

### S24 — The admin deletes a library on purpose

- **Story:** the owner removes *Kids*.
- **Expected:** its jobs are cancelled and nothing refetches for it. Watch history survives on
  the Title (D173).
- **Today:** there is no delete-library action. Its jobs would run until they failed.
- **Design:** answer 2 (a library is deleted and its jobs cancelled). **Still open: what a
  deleted library's Titles become.**
- **Status:** **holds** (`TestScenario_S24_ADeletedLibrarysWorkIsCancelledAndItsTitlesHiddenWithTheirHistory`,
  the jobs rebuild's Phase 3): its work is cancelled and its titles hidden with their history; a
  film of it that reappears elsewhere brings its title back
  (`TestPass_ADeletedLibrarysFilmThatReappearsElsewhereBringsItsTitleBack`,
  `TestMatch_ADeletedLibrarysFilmMatchedAgainElsewhereBringsItsTitleBack`). Q14 answered by
  D231 §7.

### S25 — TMDB is down overnight while 200 films are added

- **Story:** a big copy lands at 23:00, and TMDB's API is unreachable from 22:00 to 07:00.
- **Expected:** the films appear at once on local data (D170 §2). By morning they are matched,
  and TMDB was asked a handful of times, not a thousand.
- **Today:**
  - `outbound` failures are plain errors. Each job tries, waits up to `provider_timeout_seconds`
    (20 s) and spends an attempt, and fails after 5 attempts (about 7.5 minutes).
  - All 200 fail. `queueOwedFetches` checks `EverQueued` against job history, so **none is ever
    queued again**.
  - In the morning 200 films are "unidentified". On the identify page they look exactly like
    films awaiting their first match. The only fix is the Refresh button, 200 times.
- **Design:** D231. The first down-class failure parks `tmdb-api`, and one jittered probe runs
  from 30 s up to about 1 h. Nothing spends an attempt, and the queue resumes in priority order.
  - Under Q5, even a lost job is derived again, from `identification_state = unidentified` with
    no answer recorded.
- **Status:** the outage **holds** (`TestScenario_S25_ATMDBOutageOvernightParksAndMatchesAllByMorning`,
  the jobs rebuild's Phase 2: one failure, then a dozen probes overnight, and every film
  matched at its first attempt). A first match lost some other way is still **gap G10** (Phase 3).

### S26 — A node restarts mid-prepare

- **Story:** the node process is restarted (by the supervisor or the owner) while it is making
  a rendition someone is waiting on.
- **Expected:** the prepare starts again as soon as the node says hello, at no cost to the job.
  The person sees it waiting, then preparing.
- **Today:**
  - The node's FFmpeg dies with it. The core's job waits out `maxSilence` (5 minutes), then
    spends an attempt and backs off before the command is re-sent. Nothing ties the node's
    hello to its running jobs.
  - A person's play waits more than 5 minutes.
  - If the job reaches its limit through silence or refusals, the rendition stays `running`,
    because only FFmpeg's own failure calls `SetRenditionFailed`. The player then shows
    "preparing" until it is reloaded.
- **Design:** D231: the node's hello is its recovery signal and wakes its destination. The
  design does not yet say that a hello also ends the jobs that were **running** on the node.
  Those should go back to the queue at no cost.
- **Status:** **gap G5**, **G8**.

### S27 — A node restarts with commands queued for jobs that already gave up

- **Story:** the node is wedged for 20 minutes, and several jobs time out and fail. Then it
  restarts.
- **Expected:** it gets only the work that is still wanted.
- **Today:**
  - A new session polls from `after_command_seq=0` and receives every unacknowledged command,
    including those of failed jobs. It runs them, and their answers are logged as "for no
    waiting job".
  - A job that gives up on silence sends no `CancelCommand`.
  - Its retry re-sends the same job id and the same `RenditionDir` (the run id is unchanged
    unless `want` restarts the row). An orphaned run and its retry can therefore write one
    directory at once, and the node's `startJob` overwrites the first cancel func.
- **Design:** the research (§3.5), within D231: drop commands whose jobs have ended.
- **Status:** **gap G4**.

### S28 — A full disk on a node

- **Story:** the node's prepared directory fills up, and eviction cannot free space because
  everything is in use or within its grace.
- **Expected:** prepares on that node wait and say why. The admin sees it, and other work
  continues: subtitles and loudness write nothing to that disk.
- **Today:**
  - `MakeRoom` refuses with `ErrNoRoom`. The core treats the refusal as an ordinary error, so it
    spends attempts, fails after about 7.5 minutes, and the rendition stays `running` (G8).
  - The System page shows the disk (D225), but nothing links it to the waiting job.
- **Design:** silent. D231's "down" covers an unreachable node, not a node that answers "no
  room".
- **Status:** **gap G8**. Q8's "no room" is built (2026-09-30): the node names the refusal, and the
  node's prepares park on its prepared directory until its status shows room
  (`TestPrepare_ARefusalForWantOfRoomParksTheNodesPreparesUntilItsStatusShowsRoom`,
  `TestStatus_ShowingRoomBringsBackThePreparesParkedOnThePreparedDirectory`).

### S29 — A person presses play while the file's node is parked

- **Story:** the partner starts a film whose node is down.
- **Expected:**
  - Direct play is refused at once: "the node that holds this file is not answering".
  - A prepare is queued at the highest priority, and the node is looked at immediately. The
    player says "waiting for node X (unreachable since …)", not "preparing".
- **Today:**
  - Direct play: the relay returns `node_silent` (503). This **holds**.
  - Prepare: the job is sent to a node that `IsConnected` still claims. The player shows
    "preparing" for 5 minutes per attempt, and then indefinitely (G8).
- **Design:** D231: a person's request wakes its job and tries to unpark its destination. For a
  node there is no probe to send, since the hello is the signal. The design does not say what
  the waiting player is shown.
- **Status:** direct play **holds**; the prepare **holds** in the queue
  (`TestScenario_S29_APrepareForAParkedNodeSaysWhatItWaitsFor`, 2026-09-30: it waits for the node at
  no cost, the job view says "waiting for node local: not connected since …", and the hello brings
  it back). What the player says (Q11) is Phase 6's; **gap G8** stays.

### S70 — A person asks "look again" while TMDB is parked

- **Story:** the owner presses Refresh on a film during the outage in S25.
- **Expected:** TMDB is tried at once. If it answers, everything waiting resumes. If not, the
  request waits, shown as waiting for TMDB, and runs first when TMDB returns.
- **Today:**
  - The refresh is queued at priority 20 and sent right away. It fails and spends attempts like
    any other job (G3).
  - If a rolling refresh is already open for that item, `Enqueue` drops the request. The person
    is told "already waiting", and their `shouldRetryImages` and priority are lost (G11).
  - The chooser's search runs synchronously. It answers `provider_error/unavailable` at once,
    which is right.
- **Design:** D231: the request triggers an immediate probe and merges with the job already
  open. The design does not say whether the chooser's synchronous search should honour the
  parked state or count as the probe.
- **Status:** **holds** (`TestScenario_S70_APersonsRefreshWhileTMDBIsParkedLooksAtOnceAndRunsFirstWhenItReturns`,
  2026-09-30): the refresh probes TMDB at once, waits shown as waiting for TMDB, and runs first when
  it returns; a person's synchronous call to TMDB always tries and is its probe (Q15). G11 closed
  in Phase 1.

### S71 — The same job is requested again: while queued, while waiting to retry, while parked, while running

- **Story:** the same target is asked for twice, in each of four states.
- **Expected:** one job, which runs as soon as the most urgent request needs, and does
  everything that any of the requests asked for.
- **Today:** `Enqueue` finds an open job for (kind, target) and drops the new request.
  - **While queued:** priority is not raised, and args are not merged. Examples:
    - A person's refresh behind a rolling refresh loses its retry-images flag and waits at
      priority −10.
    - An artwork size requested on demand behind an owed-artwork job (the same item target) is
      dropped.
    - A D213 §1 recheck behind an open fetch is dropped for good (see G14).
  - **While waiting to retry:** the request waits out the back-off, which is up to 4 minutes,
    or up to a day on the artwork schedule.
  - **While parked:** the state does not exist yet.
  - **While running:** dropped. For a prepare, `want()` can `RestartRendition` the row (a new
    run id) while its job still runs with the old command. The job then marks the row
    `complete` under the new run id, whose directory is empty. Playback gets a 404, and
    `PrepareAgain` fixes it with a wasted prepare.
- **Design:** D231 merges a request while queued, waiting or parked: the higher priority wins,
  a job in back-off is woken, and args are merged where the kind allows. The design is silent
  on a request that arrives while the job is running.
- **Status:** **holds** for queued, waiting and parked, and for a request a running job covers or
  not (`TestScenario_S71_ARepeatRequestRaisesWakesAndMerges`,
  `TestScenario_S71_APersonsRefreshBehindARollingOneRunsAtTheirPriorityAndAsksForTheImagesAgain`);
  G11 **closed** (the jobs rebuild's Phase 1), Q9 answered by D231 §8. **Gap G13** (a rendition
  restarted while its job runs) is the node link's, Phase 4.

### S72 — The core restarts with jobs parked, running and waiting to retry

- **Expected:** every job survives. Running jobs start again, waiting jobs keep their time, and
  parked destinations are probed once.
- **Today:**
  - `RequeueRunning` sets running jobs back to queued. The attempt `StartJob` counted is not
    given back, so a restart costs one attempt (which also guards against a job that crashes
    the core).
  - The wait times of retrying jobs are stored in cache.db and survive.
  - Everything held in memory is forgotten: `pausedUntil` (a `Retry-After` of up to an hour),
    the artwork breaker (five more failures, each spending an attempt) and the node command
    queues. The nodes' sessions end, and their FFmpeg runs are killed with the session context.
    Hellos then scan and run the pass again.
- **Design:** D231 §7: destination state lives in memory, and the first due job after a restart
  probes.
- **Status:** **holds** for running and retrying jobs; parked state is held in memory, and the first
  job for a destination after a restart probes it (2026-09-30, the jobs rebuild's Phase 2).

### S73 — The TMDB key is removed or refused while fetches are queued

- **Expected:** fetches wait at no cost, the admin is told, and they resume the moment the key
  is fixed.
- **Today:**
  - `MarkBlocked` holds the one fetch runner for 30 s, at no cost to the job.
  - A missing key is found locally. A refused key sends one request every 30 s, about 2,900 a
    day, with a key TMDB already refused.
  - The block is not visible anywhere except in each job's `last_failure`.
  - Artwork continues, because TMDB's image host needs no key.
- **Design:** D231 keeps `RetryAtError`, but does not say whether "blocked" becomes a
  destination state that is parked until the key changes and shown on the System page.
- **Status:** **holds** (`TestScenario_S73_ARefusedKeyParksUntilTheKeyChanges`, 2026-09-30): a missing
  or refused key parks `tmdb-api` until the secret changes; TMDB is asked once, the System page says
  the key was refused, and the fetches resume at no cost when it is changed.

### S74 — A file is replaced (new payload hash) while its prepare, loudness or subtitle job is queued

- **Expected:** the old file's jobs end quietly, and the new file gets its own.
- **Today:**
  - **Before a scan notices the change:** the node refuses with `ErrChanged`. The core treats it
    as an ordinary error, spends 5 attempts and fails; the prepare's rendition stays `running`
    (G8). Nothing asks for a scan. Scans run only on a node's hello or an admin request, so the
    file stays unplayable until then.
  - **After a scan notices:** the old file row is tombstoned (D173).
    - Its prepare fails permanently with `ErrNoFile`, and the rendition is recorded as failed.
    - Its loudness job fails permanently.
    - Its subtitle job ends permanently.
    - The new file row gets measured by the next sweep, and prepared and extracted on its next
      play.
- **Status:** once scanned, **holds on paper**. Before a scan: **gap G8** and **question Q8**
  (a changed-file refusal should ask for a rescan of that path).

### S75 — OpenSubtitles' daily quota is spent while conversions are queued

- **Expected:** conversions finish, and only new downloads are refused, with the reset time.
- **Today:** a conversion is node work on a body already stored in library.db, and never calls
  OpenSubtitles. Search and download are synchronous, and running out of quota returns
  `quota_exhausted`.
- **Status:** **holds**. OpenSubtitles stays a reserved destination (D231).

### S76 — Two nodes, one down

- **Expected:** work for the live node's files, and downloaded artwork that needs any node to
  re-encode it, is unaffected. Work for the down node's files parks.
- **Today:**
  - `LastHeardNodeID` picks the live node to re-encode downloads. This **holds**.
  - Work for the down node's files is sent to a node `IsConnected` still claims.
  - Extraction, conversion, loudness and artwork each run one job at a time. So a doomed attempt
    blocks the queue: a play-now subtitle on the live node waits up to 5 minutes behind one,
    and artwork waits 2 minutes per attempt.
- **Design:** D231: jobs are parked per node and never claimed, so nothing blocks the queue.
- **Status:** **holds** (`TestScenario_S76_ADownNodeBlocksNoOtherNodesWork`, 2026-09-30): the down
  node's work parks on it and is passed over; the live node's runs.

### S77 — The core's own disk is full (cache.db and library.db writes fail)

- **Expected:** nothing is lost that cost something. The server says why, and work resumes on
  its own once there is space.
- **Today:**
  - Every runner's `claim` fails and is logged every second, which is about ten runners' worth
    of log lines per second on the same full disk.
  - A handler that finished, but whose `writeFinish` failed, leaves its job `running` until the
    next restart. `FindOpenJob` then blocks the target from being queued again.
  - **`FetchSubtitle` downloads first and then writes to library.db, so a failed write loses a
    download that was paid for** (D224 §6 keeps a subtitle because it cost a download).
  - A node's report is sent again from its last ingested sequence. This holds.
- **Design:** silent.
- **Status:** **holds** (`TestScenario_S77_AFullCoreDiskParksTheQueueSaysWhyOnceAndResumesWhenThereIsRoom`,
  2026-09-30): the core's storage is a destination (Q16); a claim, a record or a job's write that
  SQLite refuses for want of room parks it, the log says so once, no job is left running, and work
  resumes on its own. A paid OpenSubtitles download is not lost: its link is kept three hours, and
  fetching the file again within them spends nothing
  (`TestDownload_ALinkThatFailsIsRetriedWhileItWorksAndSpendsNoSecondDownload`). A person's own
  request whose write fails is answered as the server failing; it parks nothing until a job meets
  the full disk.

### S78 — cache.db is deleted under a running library

- **Story:** a build with a new cache schema version discards cache.db at start (versioned by
  discard). The library has jobs queued, jobs in back-off, jobs running, jobs failed for good,
  a person's refresh waiting, and a D213 §1 recheck due tomorrow.
- **Expected (the owner's principle):** nothing is lost but timing. Every piece of work that the
  library implies is derived again, and every request a person made is still honoured.
- **Today:**
  - These are derived again:
    - owed artwork, at the next pass, since the hellos trigger a scan;
    - season listings;
    - rolling refreshes;
    - loudness, whose facts live in library.db;
    - prepares and subtitles, on the next play (S35);
    - match candidates, by searching again (D172).
  - First matches are queued again **for every title never answered**, because `EverQueued` now
    finds no history. So a discard happens to fix S25's failures. The behaviour depends on
    whether cache.db survived, which is the symptom of the problem.
  - **Lost:** the person's refresh (and its retry-images flag), and the D213 §1 recheck. The
    missing image is already recorded as refused, so it is held and never owed again.
- **Status:** **holds** (`TestScenario_S78_DiscardingCacheDBLosesOnlyTiming`, the jobs rebuild's
  Phase 3): the matches owed, the person's refresh with its images and the recheck are derived
  again from library.db, and a failure of the item's own is not retried. G10, G14, G15
  **closed**; Q5, Q6, Q7 answered by D231.

---

### Cross-cutting: is every job derived again, and can a person always ask?

Asked by the owner: *"jobs that only get triggered once for a particular file, never to be
brought up again in a sweep. If that job gets killed somehow, we'll never get what it was
supposed to fetch, and there will be no way to ask."*

The owner then added a test for each kind: *"If we can remove state from the queue, then that
means the queue is 100% regeneratable … no contention for the main db … doesn't contribute to
the backup's size."* In practice: **could cache.db be deleted now at the cost of timing only?**

"Lost" in the table below covers all of these:
- killed (cancelled);
- cache.db discarded;
- failed for good;
- a crash mid-run, which `RequeueRunning` recovers everywhere, so it is not repeated per row;
- merged away by `Enqueue`'s deduplication.

| Kind / use | Queued by | If lost | Queued again? | Can a person ask? | State only in the queue today | Where it must live | Flag |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `fetch_metadata`: **first match** (D170) | The pass after a scan: derived from `ListTitlesAwaitingMatch`/`ListSeriesAwaitingMatch`, **then gated by `EverQueued`** (an event read from job history) | Failed for good (an outage longer than 7.5 min, or the item's own 5xx), cancelled, or the title died before its job ran and was then revived (S10) | **Never**, except by an accidental cache.db discard | Yes, by Refresh on the identify page, but a failed match looks the same as one not tried yet | **`EverQueued`: "was a match ever attempted"**, a fact about the item held in job history | library.db already says "never answered" (`unidentified`, `followed_source` NULL). An item-caused failure becomes a fact there, with its reason (Q6) | **YES** |
| `fetch_metadata`: season listing (D209 §3) | The pass, from `AwaitingListings` | Derived again | At the next pass | Yes, refresh the series | Nothing | Already in library.db | Sweep runs only after a scan (Q13) |
| `fetch_metadata`: rolling refresh (D213 §3) | `Refresher.Sweep`: at start, then every `metadata_refresh_check_every_hours`, from response ages | Derived again | Yes | Yes | Nothing | Already in library.db | — |
| `fetch_metadata`: **a person's refresh** (+ retry images) | The API, `Refresher.Request` | Lost on a discard or a failure for good; merged into an open rolling job, losing priority 20 and `shouldRetryImages` | No | Yes, by asking again, but they are never told the request died | **The request itself and its args** | library.db: `refresh_requested_at` (+ retry-images) on the item, cleared by a refresh that succeeds after it; a sweep derives the job (Q7) | **YES** (partly) |
| `fetch_metadata`: **D213 §1 recheck** of a missing image | The artwork handler, on the first 404, `RunnableAt` = now + `RecheckAfter` | Lost on a discard; **dropped by `Enqueue` if a fetch job is already open for the item**; failed for good | **Never.** The image is recorded as refused, so it is "held" and not owed | Only by a person's refresh with images, and nobody knows to ask | **The recheck's due time** (the miss itself is in library.db: `CountEarlierMisses`) | Derived from the artwork refusal row: miss count 1 and refused-at + `RecheckAfter` ≤ now | **YES** |
| `make_artwork_sizes`: owed images (D212) | The pass's report → the artwork service's `Take` (through the server's wiring) | Derived again (not held means owed) | At the next pass, and a pass runs only after a scan (node hello or admin) | Yes, refresh with images | A snapshot of the args (the refs) | Derived; the args are a cache | Q13. **The node's refusal of an unreadable local image is recorded as permanent** (G9) |
| `make_artwork_sizes`: sizes made on request (D212 §3) | Serving a size that was not made | Dropped next to an open owed job, or lost | By the next request | Yes, by viewing | A snapshot of the args | Forgettable | — |
| `prepare` | Play (`want`), and `PrepareAgain` when the node answers 404 | Failed for good on ordinary errors, **leaving the rendition `running`** | By the next play (`want` queues whenever it is not complete) | Yes, by playing | Nothing beyond `prepared_rendition`, which is a cache | cache.db (a note of the node's cache, D217 §5) | G8, G13 |
| `extract_subtitle` | Play (`wantText`) | — | By the next play | Yes | Nothing | Text by payload hash in cache.db | — |
| `convert_fetched_subtitle` | Play (`wantText`) | — | By the next play; the body is in library.db | Yes | Nothing | — | — |
| `measure_loudness` (D226 §2) | `QueueMeasurements`: at start, after each pass, and hourly | Derived again; a failure is kept in library.db | Yes | No button, and none needed | Nothing | Already in library.db | Queues offline roots every hour (G6) |
| `scan` (M4; today a node command held in memory) | A node's hello (a setting), or an admin request | An admin's scan of a node that has not said hello since the core started is **dropped silently**; lost on a restart | At the next hello | Yes, by asking again | The request | When it becomes a job, derived from root state (never scanned, withheld, offline, the admin's request) | Mild |
| `backup` (M4) | Not built | — | — | — | — | Derived from the last backup time against its schedule | Design at M4 |

**What is genuinely forgettable, and belongs only in cache.db:**
- attempt counts and back-off;
- destination state (D231 §7);
- `pausedUntil`;
- job history shown in the job view;
- `art_served` recency;
- match candidates (D172 searches again);
- renditions and subtitle text, which are caches of node output.

### The principle that fixes the class (Q5)

**Every job is the image of a predicate over durable state; the queue is a cache of the work
that state implies.** Each kind registers three things with the generic queue:
- its handler;
- a **sweep** that derives the kind's open work from library.db and configuration;
- the destination it needs.

A sweep runs:
- at start;
- on a timer (per kind, hourly by default);
- after each pass;
- when a destination unparks.

`Enqueue` becomes a *hint* to run a job sooner, for a person waiting or a new file. It is never
the only record that the work is wanted.

It follows that:
- A person's request is either redone by the person's own action (a play), or recorded as a fact
  that the sweep reads (a refresh request).
- A failure caused by the item is recorded as a fact with its reason, so the sweep stops asking
  and the person sees why and can clear it ("try again"). This is the dead-letter state, kept
  where it survives a discard.
- A cancel stops a run; if the state still implies the work, the sweep queues it again (Q12).
- No handler or hook ever reads job history as a fact. `EverQueued` is deleted.

**What it takes, kind by kind:**
- **First match:**
  - delete `EverQueued`;
  - derive the job from the awaiting-match queries alone;
  - add a library.db fact for an item-caused failure (`match_failed_at`, `match_failure`),
    which the query excludes and a person's refresh clears;
  - with D231's parking, an outage no longer produces failures.
  - This keeps D170's intent (one fetch per *answer*, never a flood) and drops a mechanism that
    turned a lost attempt into never.
- **A person's refresh:** add `refresh_requested_at` (+ a retry-images flag) on the item; the
  refresh sweep queues items whose request is newer than their last ask.
- **D213 §1 recheck:** the refresh sweep also queues items with one recorded miss whose recheck
  time has passed.
- **Owed artwork and season listings:** already derived; run their sweep on a timer and on
  unpark, not only after a scan.
- **Loudness:** already derived; have the sweep skip unavailable roots (answer 1).
- **Prepare, subtitles, conversion:** already driven by play. Record a rendition failed when its
  job fails for good (G8).
- **Scan (M4):** derive it from root state.

**The test that proves it:**
- `TestScenario_S78_DiscardingCacheDBLosesOnlyTiming`:
  - Script a library through scripted node reports, the fake TMDB, a fake node and `clocktest`.
  - Leave jobs in every state: queued, in back-off, running, failed for good, cancelled, a
    person's refresh waiting, and a recheck scheduled.
  - Run the script twice to quiescence, once with cache.db discarded at that point and once
    without.
  - Assert that library.db's facts are the same in both runs: identification, artwork rows,
    loudness, requested refreshes done, and item failures.
- A structural guard, an architecture test, still to write, that nothing reads job history as a fact: only the queue package and the
  job view read the job table.
- The queue's registration takes a sweep that is not nil, so a kind without one does not compile.

---


### Scenario-tier tests for S22–S78

Each of these can be driven by scripted node reports, the fake TMDB, a fake node channel and
`clocktest`, with no filesystem.

- `TestScenario_S25_ATMDBOutageOvernightParksAndMatchesAllByMorning`: two attempts per job at
  most, one probe per step, all 200 matched. **Built 2026-09-30** (cmd/orpheum): one attempt each.
- `TestScenario_S78_DiscardingCacheDBLosesOnlyTiming`: the property test above, the owner's
  principle. **Built 2026-09-30** (cmd/orpheum).
- `TestScenario_S22_AnOfflineRootParksItsJobsAndSweepsQueueNothing`, and its retire follow-up.
  The follow-up **built 2026-09-30** (internal/core/api) as
  `TestScenario_S22_ARetiredRootsFilesAreTombstonesItsWorkIsCancelledAndItsHistoryStays`; the
  parking half holds by Phase 2's tests.
- `TestScenario_S26_ANodesHelloReturnsItsRunningJobsWithoutCost`.
- `TestScenario_S27_AReconnectedNodeIsSentOnlyLiveCommands`.
- `TestScenario_S71_ARepeatRequestRaisesWakesAndMerges`: queued, back-off, parked and running.
  **Built 2026-09-30** (internal/core/jobs).
- `TestScenario_S73_ARefusedKeyParksUntilTheKeyChanges`. **Built 2026-09-30** (cmd/orpheum).
- `TestScenario_S70_APersonsRefreshWhileTMDBIsParkedLooksAtOnceAndRunsFirstWhenItReturns`.
  **Built 2026-09-30** (cmd/orpheum).
- `TestScenario_S74_AChangedFileEndsItsJobsAndIsRescanned`.
- `TestScenario_S76_ADownNodeBlocksNoOtherNodesWork`. **Built 2026-09-30** (internal/core/api).
- `TestScenario_S29_APrepareForAParkedNodeSaysWhatItWaitsFor`, and G8's
  `…_APrepareThatFailsForGoodSaysSo`. The first **built 2026-09-30** (internal/core/api), for the
  queue's side; the player's words are Phase 6's.
- `TestScenario_S77_AFullCoreDiskParksTheQueueSaysWhyOnceAndResumesWhenThereIsRoom`. **Built
  2026-09-30** (internal/core/jobs), with a real SQLite "disk full" error (`storetest.DiskFullError`,
  from a scratch database held to its pages) behind cache.db's job storage.
- S75 already holds and needs only a regression test: a conversion runs with quota at zero.

### Scenarios S79–S91: work handed to a node (walk before D233; decided by D233)

- **S79 — A hung FFmpeg during a prepare.** An encode stops making progress, with its process
  alive. *Expected:* the node's watchdog kills it within about 2 minutes and answers "stalled";
  the core retries at no cost to the item; the slot frees; other encodes proceed. A second stall
  on the same file records an item fact with "stalled twice". *Today:* a livelock (#4).
- **S80 — A hung loudness measurement.** *Expected:* as S79. The item is not failed for a hang,
  and the node's measure slot comes back. *Today:* the slot is gone until the node restarts, and
  the file is eventually blamed.
- **S81 — A finished prepare's `done` is lost in flight.** *Expected:* the outbox redelivers it;
  the core accepts it by run id even if the job gave up; nothing is re-encoded. *Today:* the core
  waits 5 minutes, then re-encodes over the complete rendition.
- **S82 — The core restarts during an hour-long encode.** *Expected:* the encode continues; at
  hello the node lists it; the core adopts it without re-dispatching; progress resumes in the job
  view. *Today:* the co-located node is killed; the job requeues, dispatches before hello, and
  waits 5 minutes.
- **S83 — A late answer from an earlier attempt.** Attempt 1 is slow, attempt 2 starts, then
  attempt 1's `done` arrives. *Expected:* it is rejected by token, or accepted as the rendition
  only if its run id is the current one. *Today:* attempt 2 accepts it while its own FFmpeg is
  still writing.
- **S84 — cache.db is discarded while a node holds finished-unacked results.** *Expected:* the
  results match by content (hash, run id), never by a reused job id.
- **S85 — One artwork answer fails to post (the core is busy, or the answer is over its bound).**
  *Expected:* it goes to the outbox; other work is untouched. *Today:* the session ends, every
  FFmpeg dies, and every root is rescanned.
- **S86 — A file whose probe or parse panics the core, or crashes the node.** *Expected:* one
  job fails; the next start refuses it at its limit; the node reports "crashed during X"; the
  System page shows a crash loop. *Today:* an endless restart loop.
- **S87 — A play-now prepare on a node parked overnight.** *Expected:* the viewer has gone, so
  it is dropped or demoted; tonight's play is not queued behind it. *Today:* it runs first, at
  priority 1000.
- **S88 — Two nodes; node A's prepares wait on a busy slot.** *Expected:* a play on node B
  starts at once. *Today:* it can wait behind global prepare runners held by A's jobs.
- **S89 — A remote node on an older build receives a command it does not know.** *Expected:* it
  refuses explicitly, or the core parks the node with the reason "version mismatch". *Today:*
  silence, timeouts and attempts spent.
- **S90 — A double click on "download subtitle".** *Expected:* one quota spent, and the same
  answer twice. *Today:* two downloads.
- **S91 — The owner stops the core with a cancel pending for a running node job.** *Expected:* at
  hello, reconciliation tells the node to stop it. *Today:* the cancel is lost with memory (moot
  today only because the node dies too).

## Gaps and questions

| | What | Scenarios |
| --- | --- | --- |
| **G1** | Built: the node reports each root's device id and mount facts; the core withholds removals past D208 §3 and records why. The admin's confirmation comes with the scanner view | S7, S8 |
| **G2** | Recorded: `root_state` keeps each root's last outcome; the scanner view and browse show a root's files as unavailable | S8 |
| **Q1** | Answered by D208 §1: re-identified from its new path unless a person confirmed the match | S1, S14 |
| **Q2** | Answered by D208 §2: a Title with no live files follows its files to the new library | S6, S19 |
| **Q3** | Answered by D208 §3: withhold past half and twenty, or all, or a changed device or mount | S7 |
| **Q4** | Answered by D208 §4: a returning payload revives its tombstone | S10 |
| **G3** | **Closed 2026-09-30** (the jobs rebuild's Phase 2): a down destination parks its work at no cost and is probed; the node table is truthful; the per-runner holds and the artwork breaker are gone. Was: down-class failures spent attempts; node jobs never checked the node; `IsConnected` meant "said hello since the core started"; blocked and `RetryAt` were per runner | S23, S25, S29, S70, S76 |
| **G4** | A reconnected node is sent the commands of jobs that have ended; a job that gives up on silence sends no cancel; a retry can run a second FFmpeg in the same rendition directory | S26, S27 |
| **G5** | A node's hello does not return the jobs that were running on it to the queue; they wait out `maxSilence` and spend an attempt | S26 |
| **G6** | **Closed 2026-09-30:** a root or disk a scan found missing is a destination; its files' jobs park on it and the loudness sweep passes them over. Was: its files' jobs ran and refused, and the sweep queued them again every hour | S22 |
| **G7** | **Closed 2026-09-30** but for the weekly reminder (P39: no channel to the admin exists yet): a root, a node and a library each have a retire action that cancels their work, and the System page says since when storage is offline. Was: no retire action; no "offline since" or reminder | S22, S23, S24 |
| **G8** | A prepare that fails for good on silence or refusals leaves its rendition `running`; the player shows "preparing" until it is reloaded | S26, S28, S29, S74 |
| **G9** | **Closed 2026-09-30:** a refused image beside the media is owed again once a scan has seen its file since. Was: a node's refusal to read a local image (its disk offline) was the image's permanent refusal | S22, S76 |
| **G10** | **Closed 2026-09-30:** a first match is owed until the item has an answer, and a failure of its own is a fact. Was: a lost first match was never queued again (`EverQueued`); a failed match looked the same as one not yet tried | S25, S78 |
| **G11** | **Closed 2026-09-30** (the jobs rebuild's Phase 1): a repeat request raises the priority, wakes a job in back-off, and merges its arguments. Was: it was dropped | S70, S71 |
| **G12** | **Closed 2026-09-30:** the core's storage is a destination that parks the queue, logged once, with an unwritten outcome written when there is room; a paid download's link is kept. Was: a failed `writeFinish` left a job `running`; runners logged every second | S77 |
| **G13** | A rendition restarted while its job runs is marked complete under the new run id, with nothing in that directory | S71 |
| **G14** | **Closed 2026-09-30:** the D213 §1 recheck is derived from the refused image. Was: it existed only as a future-dated job, lost on a discard and dropped next to an open fetch | S71, S78 |
| **G15** | **Closed 2026-09-30:** a person's refresh is a fact the sweep queues until a fetch that read it ends. Was: it existed only as a job row, lost on a discard and merged away by an open rolling refresh | S70, S78 |
| **Q5** | Answered by D231 (2026-09-30): A principle is missing: is work edge-triggered or level-triggered? | S22–S29, S70–S78 |
| **Q6** | Answered by D231 (2026-09-30): A fact is missing: where does "this item failed, for its own reasons" live? | S22–S29, S70–S78 |
| **Q7** | Answered by D231 (2026-09-30): A person's intent has no durable home: is a request a fact or forgettable? | S22–S29, S70–S78 |
| **Q8** | Answered by D231 (2026-09-30): A class of failure the design does not name: a node that answers but refuses | S22–S29, S70–S78 |
| **Q9** | Answered by D231 (2026-09-30): A behaviour D231 leaves open: a repeat request while the job is running | S22–S29, S70–S78 |
| **Q10** | Answered by D231 (2026-09-30): Scope of "destination": is a missing or refused key a parked state? | S22–S29, S70–S78 |
| **Q11** | Answered by D231 (2026-09-30): What the waiting person sees | S22–S29, S70–S78 |
| **Q12** | Answered by D231 (2026-09-30): What cancel means once work is derived | S22–S29, S70–S78 |
| **Q13** | Answered by D231 (2026-09-30): A sweep's trigger: owed work is derived only after a scan | S22–S29, S70–S78 |
| **Q14** | Answered by D231 §7 (2026-09-30): what a deleted library's Titles become | S22–S29, S70–S78 |
| **Q15** | Answered by D231 (2026-09-30): A synchronous call to a parked destination | S22–S29, S70–S78 |
| **Q16** | Answered by D231 (2026-09-30): Is the core's own storage a destination? | S22–S29, S70–S78 |
