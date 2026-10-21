> **ABOUTME:** An inventory of Orpheum's two job queues in series, the core's job queue and the
> node's work list: every kind of job, every priority, every place a job can wait, and where the
> two queues' ideas of priority disagree or get lost. It maps the 2026-10-21 dead-share waits onto
> that flow and ends with design questions for the owner. Read from the code at an earlier commit on
> 2026-10-21. It proposes and does not decide.

# Job queues inventory, 2026-10-21

**What prompted it.** In the dead-share tests on 2026-10-21, a scan's listing of a library root was
queued at priority 20 (`jobs.PriorityPersonsRequest`) and sent to the node at once. It then waited
163 s on the node for the disk's one reader. A play-driven read-through (priority 1000) of the film
being watched held that reader. The read-through was stuck on the dead NFS share, and once the share
came back it read the whole film. Earlier the same day, a listing waited about 55 s and was then
answered "not tried". Bringing a root back used to need that listing; D326 and D327 have since
removed that need, with a status that brings the root back and a probe that runs outside every
queue. D327 §5 left "how the core's queue and the node's work list share a disk's capacity between
work a person waits for and maintenance" as a design of its own. The owner: *"This is starting to
feel like an architectural problem with how we queue and prioritize jobs."* This document is the
inventory that the design starts from.

**How it was done.** The code at an earlier commit was read: `internal/platform/jobqueue`,
`internal/core/jobs`, `internal/core/nodes`, `internal/core/interest`, `internal/core/library`,
`internal/node/agent`, `internal/node/diskwatch` and each kind's enqueuers. The decisions were read
in `docs/contributors/decisions/working-notes.md`. Every claim about the code cites `file:line` at
that commit. The staging logs were not read again here. §4 takes its timings from the owner's
account and the session's notes, and says which links in its chain are inferred.

## Summary

- **The scheduling is done in three places, and each compares priority by a different rule.**
  1. The core's claim compares the numbers, **within one kind only**.
  2. The core's window and bump compare **three tiers**: a person's (priority ≥ 20), fast work and
     slow work.
  3. The node compares the numbers, plus **one flag, "play-driven" (priority ≥ 1000)**, which
     ignores every limit.

  No single place sees all the work that competes for a disk.
- **Bump frees a place in the core's window, not the reader.** The core's window lets one job more
  than the reader limit onto the node (`node_jobs_beyond_free_slots` = 1). A higher-tier job that
  fits that extra place is sent without bumping anything, and on the node it waits behind whatever
  holds the reader, however long that runs. When bump does fire, it prefers a *waiting* attempt over
  the running one that actually holds the reader.
- **The node never preempts for priority except for play-driven work.** Work at priority 20 or 100
  that a person is waiting for is "background" to the node. It waits behind a running background job,
  and play-driven work can pause it.
- **Interest raises work that no player blocks on to "play-driven".** A playing film's full
  read-through (loudness and timeline) is raised to 1000 by the interest raiser. It then runs past
  every reader limit, and it cannot be bumped by anything.
- **Work stuck on a dead disk keeps its slot and its reader** for up to its stall window: 3 min for
  an FFmpeg read. Neither the node's work list nor the core's window knows the disk is dead, although
  the node's disk gate knows.

## 1. Inventory

### 1.1 The priorities

Each named priority, highest first. "Tier" is `nodes.tierOf` (`internal/core/nodes/bump.go:25-33`):
a priority of 20 or more is the persons tier. Below 20, work is slow if it reads whole films or
encodes, and fast otherwise.

| Constant | Value | Where | Meaning | Tier on the node's window |
| --- | --- | --- | --- | --- |
| `jobs.PriorityPlayNow` | 1000 | `internal/core/jobs/queue.go:123` | Someone waits on it to watch (cut-a §5.3, D234 §3). It is also the threshold of "play-driven" (`nodes/assign.go:97`, `nodes/exchange.go:290`). | persons; never held back by the window (`assign.go:124-126`) |
| `jobs.PriorityLookedAt` | 100, down to 21 | `internal/core/jobs/interest.go:18` | The work of a page being looked at. The latest look is highest, each earlier look a rung lower, and none goes below 21 (`interest/raise.go:111`) (D324 §2). | persons |
| `jobs.PriorityPersonsRequest` | 20 | `jobs/queue.go:139` | A person's request, such as a refresh or a scan the admin pressed. It also covers a scan the core asks for itself after a disk stall (`library/scans.go:248`). A request at 20 or more probes its parked destination at once (`platform/jobqueue/queue.go:160-162`, D231 §6). | persons |
| `jobs.PriorityBringsBack` | 10 | `jobs/queue.go:138` | The scan of an unavailable root (`nodes/enrol.go:133-138`, `library/schedule.go:129-130`). | fast |
| *(unexported)* `artwork.makePriority` | 10 | `internal/core/artwork/serve.go:33` | A size of an image someone is waiting for. It has the same number as `PriorityBringsBack` but is unrelated. | n/a: holds no reader |
| `jobs.PriorityShownInterest` | 5 | `jobs/interest.go:23` | Work of anything anyone has played, looked at or partly watched, once that interest has passed. Demoted play work lands here (`jobs/queue.go:319-332`) (D324 §2). | slow or fast, by kind |
| `jobs.PriorityIndexFirst` | 2 | `jobs/queue.go:133` | The sweep's read of a file's keyframe index alone (D269 §5). | fast (`TestTierOf_AReadOfAVideosIndexAloneIsFastWork`) |
| `jobs.PriorityAheadOfSweep` | 1 | `jobs/queue.go:130` | A full read of a file that has no keyframe index (D269 §4). | slow |
| `jobs.PriorityBackground` | 0 | `jobs/queue.go:126` | A sweep's work, or play work past its "wanted until" (before D324). | slow or fast, by kind |
| *(unexported)* `identify.rollingPriority` | −10 | `internal/core/identify/refresh.go:107` | The rolling metadata refresh, behind everything else. | n/a: core only |

### 1.2 The kinds

There are eleven kinds (`internal/core/jobs/queue.go:24-53`). Each kind has **one runner, with a
concurrency of 1** (`cmd/orpheum/metadata.go:264-317`). A runner claims only its own kind
(`platform/jobqueue/doc.go:20-22`). A node kind's handler calls `Exchanges.Assign` and returns at
once (`nodes/assign.go:80-111`, D234 §7), so "one at a time" bounds how fast a kind is handed out,
not how many of it run.

"Reads media" means the work names a file under a root (`nodeproto.Work.MediaFile`,
`internal/nodeproto/commands.go:81-99`), so it takes a reader of that file's device. "Slot" is
`Work.Slot` (`commands.go:137-155`). Extraction, read-through and artwork each have one slot
(`commands.go:163-171`). Encode and remux have as many slots as the node's capacity says. The
durations are typical figures taken from the code's own comments where it gives them, and are
marked "est." where it does not.

| Kind | Enqueued by (priority) | Destination when queued | Slot / reader on the node | Typical duration | Does a person wait? |
| --- | --- | --- | --- | --- | --- |
| `fetch_metadata` | First-match sweep (0), `identify/refresh.go:415-420`, `cmd/orpheum/metadata.go:742-752`. A person's refresh (20), `identify/refresh.go:376`, `:399-400`. Rolling refresh (−10), `refresh.go:529`. Interest raises it (5, 21–100, 1000). | `identify.FetchDestination` (a provider host) | Runs in the core, not on a node | Seconds, set by the provider and its rate limit (est.); providers may snooze it (`providers/service.go:411`, D170) | On a refresh or a look |
| `make_artwork_sizes` | Identification passes (0), `artwork/service.go:121-124`. A size someone waits for (10), `artwork/make.go:93-96`. Interest raises it. | The image's provider | Slot `artwork`; reads no media device (`commands.go:150-151`; `MediaFile` does not name it) | Under a second per batch (est.) | Yes, for the size someone waits for |
| `prepare` | A play that needs a prepared copy (1000), `renditions/prepare.go:95-99` | `node:<id>` | Slot `encode` or `remux`; reader of the file's device | Minutes for a remux; an encode takes as long as the film plays or longer (est.) | Yes |
| `extract_subtitle` | A play that offers the track (1000), `subtitles/subtitles.go:180-183`. Interest. | none | Slot `extract`; reader of the video's device, or of the sidecar's | A whole read of the video for an embedded track (`agent/subtitle.go:23-24`): minutes | Yes |
| `convert_fetched_subtitle` | A person who fetched a subtitle (1000), `subtitles/fetch.go:267-270` | none | Slot `extract`; no device (the text travels in the command) | Under a second (est.) | Yes |
| `read_through` (index only) | A player waiting on the film's keyframes (1000), `readthrough/readthrough.go:282-287`. The sweep's fast tier (2), `readthrough.go:190-192` | none | **No slot** (`commands.go:146`); reader of its device | "A fraction of a second" (`readthrough.go:376-380`) | Yes, at 1000 |
| `read_through` (full) | Sweep (0 or 1), `readthrough.go:183-193`. After an index read (0 or 1), `readthrough.go:540-546`. A play on time (1), `readthrough.go:306-309`. **Interest raises it to 1000 while its film plays** (`interest/raise.go:98`), and to 21–100 while its page is looked at. | none | Slot `read_through`; reader of its device | "Minutes" (`nodes/quickpass.go:127-130`): the film's size divided by the device's read speed | **No one blocks on it.** The page shows loudness as not measured (D324 §6). |
| `make_chunks` | A player waiting for chunks (1000), `chunks/ensure.go:97-100` | `node:<id>` | Slot `encode` (a quality) or `remux`; reader | Seconds per run (est.) | Yes |
| `scan` (the listing) | Admin (20), `library/admin.go:123`, `:401`, `move.go:113`. A stall on the root's disk (20), `scans.go:241-248`. A root not answering again at hello (10), `enrol.go:133-138`. The unavailable-root sweep (10), `schedule.go:129-130`. Scheduled scans or a watcher (0), `schedule.go:76`, `:150`. A root back (0), `scans.go:264`. | `node:<id>` (`scans.go:282-288`) | **No slot**; reader of the root's device | Seconds to tens of seconds for a large root (est.) | When the admin pressed it, or for a root that has to come back (before D326 and D327) |
| `examine` (one batch) | The listing's result, **at the scan's own priority** (`scans.go:564-566`). `ReadNow` for a file a person asked to play (20), `scans.go:213-216`. | `node:<id>` | **No slot**; reader of the first file's device (`commands.go:95-96`) | "A batch of files in seconds" (`quickpass.go:127-128`) | When the scan is a person's |
| `backup` | Admin (20), `backup/jobs.go:90`. Schedule (0), `jobs.go:110`. | none | Runs in the core | Seconds to minutes (est.) | When the admin pressed it |
| `renew_certificate` | Sweep (0), `certificate/certificate.go:202-203` | none | Runs in the core | Seconds to minutes; DNS-01 (est.) | No |

Two quirks of the inventory itself:

- **The persons tier holds scans no person asked for.** A disk stall's scan is queued at 20
  (`scans.go:248`) and so are all its examines (`scans.go:565`). The same goes for a scan the admin
  pressed on a root of thousands of files. Each of them is "a person's" tier for bumping, and each
  probes parked destinations as a person's request does.
- **Priority 10 means two things.** It is the root's return (fast tier) and an artwork size someone
  waits for. The number is the same, but they never compete, because they are different kinds and
  artwork holds no reader.

## 2. How a job flows, and every place it waits

The core's half runs from enqueue to send. The node's half runs from hold to result. Each wait names
its code and the decision behind it.

### 2.1 In the core

1. **Enqueue and merge.** `jobqueue.Queue.Enqueue` (`platform/jobqueue/queue.go:141-163`) inserts
   the job, or merges the request into the open job for that kind and target. The higher priority
   wins (`queue.go:216-218`). A request the running job covers is dropped (`queue.go:224-232`)
   (D231 §8). A play's priority is "kept wanted" in memory (`core/jobs/queue.go:234-238`, `:248-264`;
   D234 §3, D303 DL10).
2. **Wait: runnable time, back-off, snooze.** A failure retries on a jittered, doubling back-off up
   to `job_attempt_limit` (`runner.go:398-418`; D231 §10). A snooze (a provider's rate limit) costs no
   attempt (`runner.go:372-376`; D170).
3. **Wait: a parked destination.** A claim passes over every job whose destination is parked and not
   due a probe (`destination.go:258-269`; `runner.go:187`; D231 §5, §6). A parked destination is
   probed by one job at a time, on a back-off from 30 s to an hour (`destination.go:111-156`), unless
   it awaits a signal. The back-pressure destinations (slot, reader, playing) await a signal and are
   unparked when an attempt is forgotten (`nodes/assign.go:186-195`). **An unpark wakes every runner
   of every kind and asks for every sweep** (`destination.go:182-196`).
4. **Claim.** This is the highest priority whose time has come, then the earliest, then the oldest,
   **of the runner's own kind** (`jobqueue/doc.go:20-22`; `runner.go:174-212`). Kinds are never
   compared with each other here.
5. **Handler checks that park before sending.** These include a root that is unavailable
   (`library/playable.go:116-127`; D231 §5, D241 §4, where a play-driven job on an unreadable file
   ends instead) and a node that is not connected (`readthrough.go:399-401`; `assign.go:85-87`).
6. **Wait: the window** (`Exchanges.parkIfWindowFull`, `nodes/assign.go:121-159`; D240 §5). Work at
   1000 or more skips all of it (`assign.go:124-126`; D240 §6). For the rest, in order:
   - **Beside playing** (`nodes/playing.go:205-230`; D324 §5): everything waits while the node's
     played software encode is behind playback, and an encode waits while a played software encode
     runs.
   - **Slot full**: the attempts held for the slot have reached `SlotCount + node_jobs_beyond_free_slots`
     (default 1, `settings/registry.go:433`) (`assign.go:145-150`).
   - **Device full**: the attempts held on the device have reached `readerLimit + node_jobs_beyond_free_slots`
     (`assign.go:151-157`). Reader limits come from settings by kind of device: spinning 1, SSD 4,
     network share 2, unknown 1 (`settings/registry.go:437-447`); a device not named gets 1
     (`nodes/exchange.go:61-66`) (D240 §5, D244 §2). **The count includes play-driven attempts**
     (`assign.go:162-179`).
   - **Bump instead of waiting** (`nodes/bump.go:41-65`; D255 §2, D324 §4). Where the slot or device
     is full, an attempt of a *lower tier* is stopped and goes back to the head of its priority's
     queue at no cost (`runner.go:368-371`; `answers.go:97-99`). Of the candidates, the lowest tier
     goes first, then the lowest priority, then **one the node holds waiting before one it runs**,
     then the newest (`bump.go:82-98`).
   - **Behind a quick pass** (`nodes/quickpass.go:146-172`; D255 §3): background work that reads
     whole files waits while a scan on its disk has not finished its listing and examining. It is
     re-checked every 30 s (`quickpass.go:130`).
   Each of these waits is a back-pressure parking: logged at debug and never counted as an outage
   (`jobqueue/destination.go:36-40`).
7. **Assign and send.** An attempt token is minted. The job is recorded as assigned until
   `assigned_job_limit_hours`, and the command, **carrying the job's priority and
   `IsPlayDriven = priority >= 1000`**, is queued to the node (`assign.go:91-110`; D234 §7, D238 §2).
   A raise or a demotion afterwards sends `CommandReprioritize` with the same flag rule
   (`assign.go:220-233`; `exchange.go:288-293`; `interest/raise.go:167-190`;
   `cmd/orpheum/metadata.go:652-674`).

### 2.2 On the node

8. **Hold.** `Agent.holdCommand` (`node/agent/agent.go:434-470`) takes the core's priority, the
   play-driven flag, the slot and the file's device key (`agent.go:441-450`), and adds the work to
   the in-memory work list (`working.go:120-148`; D238 §3).
9. **Wait: slot and reader** (`workList.startDue`, `working.go:161-180`; D240 §5, §6). Work is taken
   in order of numeric priority and then arrival (`working.go:151-153`; D234 §3). For each job:
   - **Not play-driven:** it starts only when its slot (if it has one) has room and its device has
     fewer reading jobs than the limit (`working.go:166`, `:172-174`, `:200-223`). **A running job is
     never displaced for a higher one**, and a reprioritize only reorders the waiting work
     (`working.go:291-298`).
   - **Play-driven:** it ignores its slot's count and the device's limit. If the reader is taken, it
     first pauses the lowest-priority non-play-driven work reading the device (`working.go:166-171`,
     `:227-237`; D240 §6). Pausing is SIGSTOP of FFmpeg (`execx/pause.go:66-77`), or, for an
     examining, waiting between files (`working.go:241-257`).
10. **Run, and checks at start.** An answering job (a listing, an examining, a read-through or a
    subtitle) **first asks the disk gate whether its root is held stuck**. If it is, the job is
    answered "not tried" at once (`agent/subtitle.go:62-73`; D233 §10). A file is opened through the
    gate with a 30 s open window (`agent/prepare.go:138-160`; `agent.go:107`).
11. **Wait: stuck on the disk.** An FFmpeg run is watched by execx. A read-through may stand still for
    3 min (`node/readthrough/read.go:28`), a prepare for 3 min (`node/prepare/prepare.go:36`), and a
    chunk run for 2 min (`node/chunks/maker.go:41`) before it is answered as stalled. A run that
    outlives SIGKILL is left running, and its job ends, which frees its slot (D233 §10). A listing or
    an examining reads in the node's own process: once its reading stands still for the read window,
    it is answered as stalled and holds the root stuck in the gate until the read returns
    (`subtitle.go:117-136`; `working.go:340-354`).
12. **Result.** The result is posted until it is acknowledged (`agent/answer.go:136-170`; D234 §5).
    The core resolves the job by its token (`assign.go:321-349`). A stall on a root's disk makes the
    root unavailable, asks for its scan at 20, and parks the job on the root, except for a listing or
    an examining, which is retried (`nodes/disk.go:56-80`; D233 §10, D241).

Outside every queue, the node's status measures each disk (D326), and the core's probe of a root on
the byte socket (`node/byteserver/probe.go:97`; `library/schedule.go:110-121`; D327) answers whether
it can be read, without a slot, a reader or a place in the window.

## 3. Where the two queues disagree, or lose the priority

1. **Three orderings and no total order.**
   - The core's claim is numeric and per kind (`runner.go:187`, `jobqueue/doc.go:20-22`).
   - The window is by tier (`bump.go:25-33`).
   - The node is numeric, plus one flag (`working.go:151-153`, `:166-174`).

   A scan at 20 and a read-through at 0 are never compared until they meet on a device. When a
   reader frees, **every kind's runner is woken** (`destination.go:193`, `queue.go:313-322`), and the
   window place goes to whichever kind claims first, not to the highest priority across kinds.
2. **The node knows the number but not the tier, and the core knows the tier but not the node's
   reader.** The node receives the priority (`agent.go:441`). It acts on that number only to order
   the work that waits. Its only preemption is the play-driven flag. Work a person waits for at 20 to
   999, a looked-at page's read-through or an admin's scan, gets no preemption on the node and can be
   paused by play-driven work (`working.go:232`: "background" means `!isPlayDriven`).
3. **Bump frees a window place, not the reader.** Up to `readerLimit + 1` attempts may be held on a
   device (`assign.go:151-152`; `registry.go:433`). Three things follow:
   - A higher-tier job that fits the "+1" is sent without a bump. On the node it waits for the
     running job to end (`working.go:172-174`), and nothing on either side then stops that job.
   - When bump does fire, `isBumpedBefore` prefers the attempt the node holds **waiting**
     (`bump.go:90-93`). The incoming job takes that waiting job's place and waits behind the same
     running job. `TestAssign_WorkOfAHigherTierBumpsTheLowestWhereNothingIsFree`
     (`nodes/bump_test.go:88-117`) shows exactly this: read-through 2, the waiting one, is bumped, and
     read-through 1 keeps the reader.
   - D255 §2 says a higher priority "evicts a running job". The interest plan says the core "stops the
     lowest of the work holding it" (`design/plans/interest-priority.md:61-65`). The code follows
     D324's "kill only what's in the way", read as "lose the least work". Whether a waiting attempt
     counts as "in the way" is the open point.
4. **The persons tier cannot bump the persons tier.** Bump needs a strictly lower tier
   (`bump.go:45-46`). Play work at 1000, looked-at work at 21–100 and a scan at 20 are all in the
   persons tier. A listing at 20 can therefore never displace a read-through raised to 1000, or even
   one at 100. Inside the persons tier, only the node's numeric order counts, and it never displaces
   anything.
5. **"Play-driven" is defined by a number, and interest raising moves non-blocking work across it.**
   `IsPlayDriven = priority >= 1000` (`assign.go:97`, `exchange.go:290`). The interest raiser lifts
   every job on a playing file to 1000 (`interest/raise.go:98`), including the full read-through for
   loudness, which the player does not wait on. That job then:
   - skips the core's window entirely (`assign.go:124-126`);
   - ignores the node's reader limit and pauses other work (`working.go:166-171`);
   - cannot be bumped (it is in the persons tier, `bump.go:27`);
   - is not even held back for an unavailable root: it fails at once instead
     (`library/playable.go:120-125`).

   D240 §6's exemption was made for work "a person waits for, to play". D324 §3 raises a playing
   film's read-through for interest. The two meet in one number.
6. **A job stuck on a dead disk keeps its slot and its reader.** The work list counts a job as
   reading while it runs and is not paused (`working.go:79`, `:217-221`). It does not ask the disk
   gate, which knows the root is stuck (`diskwatch/gate.go:121-135`).
   - An FFmpeg run holds its slot and reader for its whole stall window, 3 min for a read-through
     (`read.go:28`). If the share comes back inside that window, the run carries on and holds the
     reader for the rest of the film.
   - The core's window counts the attempt too (`assign.go:162-179`).

   D233 §10's "a process the kernel will not kill frees its slot" holds only once the stall is
   declared. The NFS research recommends counting such processes apart from the device's limits
   (`nfs-client-recovery-2026-10-21.md`, recommendation 4).
7. **Pausing a listing does nothing to its reading, and stops its stall watch.** For in-process
   work, `pauseBackgroundFor` sets the flag (`working.go:232-235`). A listing never checks it:
   `runList` has no "between" hook (`agent/listing.go:42`), while the examining does
   (`listing.go:66-67`). A paused listing still reads, but it no longer counts against the reader, and
   `stallStillReads` skips paused work (`working.go:344`), so a paused listing on a dead disk would
   never be declared stalled. This was found by reading the code. No test exercises it.
8. **A cheap job can wait behind an expensive one of higher priority on the same device, with no
   bound.** A listing (seconds) at 20 waits behind a read-through (minutes) at 100 or 1000 on a
   one-reader device. Nothing in either queue weighs how long the job ahead will hold the device.
   D255 used cost only to rank the tiers below a person's (`bump.go:28-31`).
9. **Demotion does not stop running work, and promotion does not start it.** A reprioritize changes
   the order of waiting work only (`working.go:291-298`). A play-driven job demoted to 5 while it runs
   keeps its reader, so the work it paused waits for it to end, or to continue (`working.go:184-198`).
10. **The "+1" exists for throughput and works against priority.** It exists "so the next starts
    without waiting for the core" (`assign.go:113-115`). Once a job is on the node, though, it is
    committed to that node's local order, which knows nothing of the core's global order (D240 §5,
    "the core schedules").

## 4. The failure modes of 2026-10-21, on this flow

The chain is reconstructed from the owner's account and the session's notes. **Inferred** marks a
link read from the code that the logs were not re-read to confirm.

1. **The read-through at 1000.** The film being watched had its full read-through raised to 1000 by
   interest (`raise.go:98`), which made it play-driven on the node (`exchange.go:290`). *Inferred:*
   the logs say "play-driven read-through at 1000". An index-only read at 1000 (`readthrough.go:282-287`)
   takes a fraction of a second, so the job that read the whole film was the full one.
2. **The share dies.** The read-through's FFmpeg blocks in the kernel. It holds the `read_through`
   slot and the device's one reader (§3.6). Its 3 min stall window is longer than the about 50 s
   cable pull plus about 48 s of NFS recovery, so it is never declared stalled (`read.go:28`).
   *Inferred:* the one-reader limit. Staging's share reaches the node through virtiofs, which
   `isNetworkFilesystem` does not name (`node/hardware/storage.go:20-26`), so it is probably read as
   an unknown disk, with a limit of 1 (`registry.go:446`).
3. **The listing at 20.** A stall elsewhere made the root unavailable and asked for its scan at 20
   (`scans.go:241-248`). In the core:
   - The window counted one attempt on the device against `1 + 1` (`assign.go:151-152`), so the
     listing was sent at once, with no bump (§3.3).
   - Bump could not have helped: the read-through was in the persons tier (§3.4).

   On the node, the listing was not play-driven, so it waited for the reader (`working.go:172-174`).
   When the share returned, the read-through read the whole film. The listing waited 163 s in all.
4. **The 55 s "not tried".** A listing that waited for the reader, then on starting found the root
   held stuck by an earlier reading, is answered "not tried" at once (`subtitle.go:62-73`).
   - Its wait was all in the queue, and its answer said nothing new.
   - The core answers a listing's stall by retrying it, not by parking it (`disk.go:74`). It also
     asks for the root's scan again (`disk.go:70`), which merges into the scan already open.
5. **Bringing the root back depended on the listing.** Before D326 and D327, only a scan's listing
   could make an unavailable root available, so the 163 s went straight into the viewer's wait. D326
   (the status) and D327 (a probe outside every queue) took it off that path. The underlying gap
   remains: work a person waits for, below 1000, can sit behind work that will hold a device for
   minutes.

## 5. Design questions for the owner

Each is framed as the kind of gap first, then the options. None is decided here.

1. **What "a person waits for this" means.** *The gap:* one number does two jobs. It says how soon a
   job should run, and, at 1000 or more, it grants exemption from every limit. Interest raising pushes
   work across that line (§3.5). *Options:*
   - (a) Make "blocks a person now" a separate attribute of the request, set by the caller (a player
     waiting for chunks, a prepare, an index read) and never by interest.
   - (b) Keep one number, but cap what interest raises below the exemption line.
   - (c) Keep it as it is, and accept that a playing film's loudness read can take the disk.
2. **Who preempts, and where.** *The gap:* bump acts on the core's view of a window, and the node
   acts on the play-driven flag. Neither stops the job that actually holds the reader, for work a
   person waits for below 1000 (§3.3, §3.4, §3.8). *Options:*
   - (a) The core bumps the *running* holder when the incoming job is of a higher class, and the "+1"
     is never used to dodge a bump.
   - (b) The node preempts locally by priority class, not only for play, since it alone knows who
     holds the reader.
   - (c) There is no "+1" for work below the persons tier.
   - (d) Bump by expected cost: a job expected to take seconds may cut in ahead of one expected to
     take minutes, as a short-job-first rule.
3. **What a dead disk's work holds.** *The gap:* the disk gate knows a root is stuck, but the
   accounting of slots and readers does not (§3.6). *Options:*
   - (a) Work on a root the gate holds stuck releases its reader and slot at once, and is counted
     apart, as the NFS research suggests.
   - (b) Shorten the read-through's stall window for shares that "may wait forever".
   - (c) Leave it, now that D326 and D327 remove the root's return from this path.
4. **One scheduler or two in series.** *The gap:* there is no total order across kinds anywhere
   (§3.1), and the node's local order freezes whatever the core sent (§3.10). *Options:*
   - (a) The core is the only scheduler: it sends exactly what may run now, with no "+1", and accepts
     a round trip between jobs.
   - (b) The node is the scheduler of its own devices and the core sends everything by class (D234
     §3's original window).
   - (c) Keep both, but share one comparator: a class, then a priority, then cost, used by the claim
     (across kinds), the window and the work list alike.
5. **Which classes exist.** *The gap:* the persons tier mixes a viewer's play, a looked-at page, an
   admin's scan and a scan the core asked for itself after a stall (§1.2's quirks). *Options:* name
   the classes explicitly, for example these, each with its own preemption rights:
   - blocking play;
   - interactive (a page, an admin);
   - recovery (a root coming back);
   - maintenance, fast;
   - maintenance, slow.
6. **The small defects found on the way** (each wants a test whichever design is chosen). These are
   offered as findings, not decisions:
   - pausing a listing is bookkeeping only and silences its stall watch (§3.7);
   - every back-pressure unpark asks for every sweep (`destination.go:194`);
   - a scan the core asks for after a stall takes a person's priority (`scans.go:248`), and so do all
     its examines.

### Principles any redesign keeps

- **The queue is a regenerable cache of state.** Work is derived by sweeps, and a destination that is
  down parks its work at no cost (D231 §3, §5, §6).
- **The core never blocks on a node.** A job given to a node is a row in the assigned state (D234 §7).
  The node holds its work in memory and owns its run to the end (D234 §4, D238).
- **The core keeps the smarts and schedules. The node enforces its devices' limits and yields to play
  at once**, because a round trip cannot (D240 §5, §6).
- **One person's watching never waits for another's** (D240 §6).
- **Three ranks below a person's: a fast operation's above a slow one's.** A higher one bumps the
  lowest when nothing is free, and a scan's quick pass comes first on its disk (D255).
- **Interest orders the work:**
  - played, then looked at, then a person's request, then shown interest, then the background;
  - only work that is in the way is stopped;
  - playing is not starved;
  - what waits says how long.
  (D324.)
- **A process the kernel will not kill frees its slot, and a stall makes its root unavailable before
  it is blamed on the file** (D233 §10).
- **A question about a root is not work.** It is answered outside every queue, bounded and shared
  (D327). Pages pull and are never pushed (D325).
- **The node does all media work**, one encode per node's slot count (D193 §2, cut-a §5.3).
