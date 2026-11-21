> **ABOUTME:** A read-only audit of the code of 2026-11-21 for architectural problems and
> duplication, following the audit checklist's "The code" and "The architecture map" sections. Read
> before scheduling a code-fixing batch. It proposes; the design and the gates are the owner's.

# A Code Audit for Architecture and Duplication, 2026-11-21

The last whole-code audit of the Go side was [the 2026-10-25 audit](audit-2026-10-25.md); the web
side had [its architecture audit](web-architecture-audit-2026-11-10.md) on 2026-11-10. This one read
`internal/`, `cmd/`, `pkg/` and `web/` as main stood on 2026-11-21 (its last commit "test(store):
the cache's queue holds a find_repeats job"), with most attention on what changed since 2026-11-10.
It leaves out what [code-debt.md](../../code-debt.md) lists,
[D229](../../decisions/working-notes.md) §2's settled look-alikes and
[the watch list](../../look-alike-watch-list.md).

Each finding was confirmed by reading the code at the lines named. They are ranked by what they
cost: who pays, and how often. "Agent" means a refactor an agent can make under the existing gates;
"owner's" means it needs a design or gate change first. Counts are as measured on 2026-11-21.

## What is sound, and stays

Read for duplication and found already one: retry delays (`internal/platform/backoff`'s `Doubled`
and `Jittered`), a host's refusals (`internal/platform/outbound`'s `RateLimitedError` and its
`Retry-After`), the web's problem text (`problem.ts`'s `problemText`) and its pollers (`polling.ts`,
[D237](../../decisions/working-notes.md)), paging cursors (`library.EncodeCursor` and
`DecodeCursor`, read by every list in `internal/core/api` through `decodeCursor` and
`nextCursorOf`), and language tags (`internal/platform/langtag`). No web package is imported past
its entry. The API's `JobKind` enum is derived from `jobs.Kinds()`
(`internal/core/api/jobs.go:19-31`), not copied.

## Findings, ranked

### 1. The node's work and result unions answer a missing case with "nothing"

`nodeproto.Work` is a type tag and one of several optional pointer payloads
(`internal/nodeproto/commands.go`). Three of its methods are tagless `switch {}` over which payload
is set, and each ends in a quiet default:

- `MediaFile()` (`commands.go:87-108`) answers `BytesFile{}, false`: a work whose payload it does
  not list reads no file.
- `Slot()` (`commands.go:148-166`) answers `""`: a work it does not list takes no slot, so runs
  unthrottled beside the node's other work.
- `Result.Stall()` (`internal/nodeproto/result.go:59-77`) answers `nil` and has no case for the
  artwork or system-video results.

The `exhaustive` linter does not see a tagless switch. The tests do not close the gap either:
`TestMediaFile`'s table (`internal/nodeproto/device_test.go:31`) lists neither `HashWhole`,
`FindRepeats` nor `MakeSystemVideos`, and the slot tests cover only chunks and read-through
(`device_test.go:65-80`). The node's dispatch, by contrast, refuses an unknown work loudly
(`internal/node/agent/agent.go:533-568`, `refuseUnknown`). This is the class the find_repeats work
just met, and every new node command pays it again, silently, until a device shows the effect.

- **Cost:** every new node command; the failure is wrong scheduling or a missing reader on a real
  disk, found late.
- **Fix:** switch on `Work.Type` (a named type the linter already checks) in each method, with a
  test that walks every type; or make the payload a sealed interface checked by `gochecksumtype`,
  already enabled, with `Slot()` and `MediaFile()` as its methods. **M, agent** (the sealed
  interface changes the wire types' Go shape, not their JSON).

### 2. A new job kind discards the whole cache

`internal/store/cache/schema.sql:10` lists the job kinds in a `CHECK`. A new kind changes the
schema, so it bumps the cache schema version (53, `schema.sql:6`) and every cache.db is deleted at
the next start: queued jobs, scans, match candidates, chunks, node attempts and resource use. The
version was bumped 15 times between 2026-10-21 and 2026-11-21, not all for kinds. Each costs a
server its made chunks and its queue on upgrade, and [code-debt.md](../../code-debt.md) already
holds back one wanted column (`keptCandidates`' original name) for want of a bump to share.

- **Cost:** every server, at every release that adds a kind; a re-made cache after it.
- **Fix:** drop the kind `CHECK` and check the kind in Go where a job is written, against
  `jobs.Kinds()`; or give the cache additive steps for new columns and tables, as the persistent
  store has. **S** for the first, **owner's** (the cache's versioning is a design choice and the
  schema a gate's input).

### 3. Watch Together and the live connection drop an unknown message unheard

`together.Hub.Handle` (`internal/core/together/hub.go:609-635`) switches on `message.Type`, refuses
the server-only types with `codeInvalid` (`:628-629`), and for a type it does not know sends nothing
and logs nothing. Its neighbours in the same package refuse: `group.go:29-47` ends in
`default: refuse(codeInvalid)` and `may()` (`group.go:63-75`) answers false by default. The live
socket's reader (`internal/core/api/live_socket.go:130-139`) ignores both unknown and server-only
types silently, and `api/together_socket.go:290-311` (`readInto`) refuses only a version. On the
web, the SDK's `parseLiveMessage` (`web/packages/sdk/src/live-connection.ts:237-268`) returns
`undefined` for an unknown type and `#hear` (`:430-462`) drops it without a log.

- **Cost:** each protocol change across a version skew (a page older or newer than the server): the
  sender waits on an answer that never comes, and nothing in a report says why.
- **Fix:** a `default:` that refuses with `codeInvalid` and logs at debug in `Handle` and the live
  reader; a debug log for the SDK's drop. **S, agent.**

### 4. "Is this path at or under that one" is written again beside the helper that does it

`internal/platform/pathtree/pathtree.go:11` (`IsAtOrUnder`) is the helper, and
`internal/node/hardware/storage.go:179` uses it. The same test is written by hand at:

- `internal/node/diskwatch/gate.go:85-87` (`isUnder`, a copy);
- `internal/node/watch/inotify_linux.go:359` and `internal/node/watch/watcher.go:291`;
- `internal/core/library/read_again.go:38` and `:52`, and `internal/core/library/compare.go:185`;
- `internal/core/api/cross_origin.go:62`;
- `internal/core/webclient/webclient.go:74`, `:97` and `:179`.

Each is `path == root || strings.HasPrefix(path, root+"/")` or a variant of it; a variant that
forgets the separator matches `/media/films2` under `/media/films`.
`internal/core/library/rootpath.go:58` compares bytes for another reason and is not one of them.

- **Cost:** each new folder rule; a missed separator is a reach or watch bug, not a crash.
- **Fix:** call `pathtree.IsAtOrUnder` at each (URL paths in `webclient` and `cross_origin` with a
  `/`-only form, if the helper is `filepath`-based). **S, agent.** A forbidigo rule on
  `HasPrefix(…, …+"/")` would keep it so: **owner's** (lint configuration).

### 5. The rule for a file's main picture is in five places

A video stream flagged `attached_pic` (cover art in the container) is not the film's picture. The
rule is written twice in Go, identically: `internal/core/decide/classify.go:126-133` (`MainVideo`)
and `internal/core/artwork/frames.go:241-248` (`pictureStreamOf`). It is written three more times in
SQL as the magic number `(disposition_flags & 1024) = 0`: `internal/store/queries/identify.sql:557`
and `:574`, and `internal/store/queries/read_through.sql:35`. The bit itself is computed in Go by
`library.DispositionBit` (`internal/core/library/streams.go:17-27`).

- **Cost:** the next change to what counts as the main picture (a second disposition, a stream order
  rule) must find all five; a missed one makes one path pick the cover.
- **Fix:** `artwork` calls `decide.MainVideo` (or both a helper in the package that owns streams);
  the SQL names the bit through a stored `is_attached_picture` column or view. **S** for Go,
  **agent**; the column is a schema change, **owner's**.

### 6. Job kinds: one registry exists; one switch and one test list stand beside it

The audit was asked whether a job-kind registry would make the roughly eight places a new kind
touches into one. `internal/core/assembly/jobs.go:58-122` (`runners`) already is one: per kind, its
handler, failure recorder, retry schedule, concurrency, what derives it and who runs it. The other
places each fail loudly when a kind is missing: the API enum is derived (`api/jobs.go:19-31`); the
web labels are compile-checked against it (`web/apps/web/src/jobs.test.tsx:19-37`, labels in
`locales/en-status.ts:75-89`); the cache takes every kind by test
(`internal/core/jobs/jobs_test.go:541`). Three places a registry would remove:

- the cache `CHECK` (finding 2);
- `subtitles.RecordFailure` (`internal/core/subtitles/extract.go:155-186`), a `switch job.Kind` over
  every kind in which all but two answer "is not a subtitle's job": each new kind adds a case to a
  package it has nothing to do with, because `exhaustive` demands it. Other packages' recorders take
  one kind and are named for it (`chunks/job.go:250`, `RecordMakeChunksFailure`;
  `library/scans_records.go:118` and `:141`);
- the list of kinds kept by their item rather than recorded
  (`internal/core/assembly/jobs_test.go:90-105`), which repeats a fact `runners` could carry.

- **Cost:** small per kind, but in three files a reviewer does not expect.
- **Fix:** split `RecordFailure` into one recorder per subtitle kind wired in `runners`, so the
  switch goes; let `runners` say whether a kind is kept by its item and the test read it. **S,
  agent.** No new registry is needed.

### 7. Files at the length cap, and packages past one job

Fifteen files were within 20 lines of the 800-line cap on 2026-11-21, among them
`web/apps/web/src/play.tsx` (800), `web/packages/player/src/chunked.ts` (800),
`web/apps/web/src/together-viewer.tsx` (799), `web/apps/web/src/cast-to.tsx` (798),
`internal/core/together/hub.go` (795), `cmd/orpheum/serve.go` (790), `internal/core/api/playback.go`
(782) and `internal/core/playback/playback.go` (781). The next change to each must split it first,
in the middle of other work.

`internal/core/library` held 13,662 lines in 60 files and a 554-line `doc.go`; since 2026-11-10 it
gained the find_repeats job's handler (`repeats.go`), segments (`segments.go`) and the home page's
next up (`home_next_up.go`), beside the copy and system-video handlers (`copies.go`,
`systemvideos.go`). The [architecture map](../architecture-map.md)'s row for it is already flagged;
the job handlers are a part with its own boundary (they read the store and send node work, and
browse never calls them). `internal/core/api` held 19,516 lines in 93 files, its `Dependencies` 45
fields (`internal/core/api/server.go:71`); its map row is flagged too. `web/apps/web/src` is one
flat directory of 428 files.

- **Cost:** each change to a capped file; a reviewer of `library` reads a package of unrelated jobs.
- **Fix:** split each capped file at its seam before the change that needs it (**S** each,
  **agent**); move the library's job handlers to a package of their own (**M**), and group
  `web/apps/web/src` by screen (**L**): both **owner's**, as the map's boundaries are.

### 8. A file's waits are polled outside the pollers

`useFileWaits` (`web/apps/web/src/file-waits.ts:21-46`) asks `GET /files/{id}/waits` from its own
`setInterval`. It gives up no request (the pollers' `askAnswersWithinMs`, `polling.ts:75`), does not
back off on failure, and starts a new request while the last is unanswered. `shownAndEveryPoller`
(`polling.ts:101-143`) does exactly this job with all three, and `levelling.tsx:160` uses it. On a
slow server or a lost connection the waits page stacks requests.

- **Cost:** a person watching a file's waits on a weak network or a busy server.
- **Fix:** run the ask through `shownAndEveryPoller(everySeconds * 1000)`. **S, agent.**

### 9. A film's clock is written three ways, and a page's end time in the browser's locale

`clockText` (`web/apps/web/src/timeline.tsx:13`, used by `skipped-parts.tsx:10` and `title.tsx:29`)
writes a position with no locale; `filmPositionText` (`web/apps/web/src/together-events.ts:9`)
writes the same with the locale's digits through `Intl`; `lab-timing.ts:92-96` has a third. The
convenience area's end time (`web/apps/web/src/convenience-area.tsx:67`) uses
`new Intl.DateTimeFormat([], …)`, the browser's locale, where every other time on a page goes
through `timeText(epochMs, locale)` (`date-time-text.ts:20`) and the person's chosen language.

- **Cost:** a person whose chosen language is not the browser's sees two formats on one screen; each
  future change to how a position reads touches three files.
- **Fix:** one position formatter taking the locale, used by all three; `convenience-area.tsx` calls
  `timeText`. **S, agent.**

### 10. Library lists by kind fall to films for anything they do not know

`countListed` (`internal/core/library/lists.go:40-47`) ends in `default: CountFilmsListed`, and
`listKind` (`lists.go:14-26`) is a tagless switch that sends every kind not "other" to the films or
series query. The kinds are plain strings (`internal/core/library/browse.go:33-36` and `:217-219`),
so `exhaustive` cannot see them. Today the API's enum (`internal/core/api/item_list.go:148`) admits
only the known kinds, so nothing wrong reaches it; the next kind (a music or a photo library) would
count as films without a failure.

- **Cost:** the next library kind; a wrong count, not an error.
- **Fix:** a named `ListKind` type with its constants, a `switch` the linter checks, and an error
  for an unknown one. **S, agent.**

## Smaller findings

- **The persistent store's set-aside name is built twice:** `cmd/orpheum/operations.go:252` and
  `cmd/orpheum/serve.go:431` each write `"persistent-set-aside-"` and a UTC stamp in
  `20060102T150405Z`, and `internal/core/identify/report.go:76` repeats the layout. One function in
  the store's package. **S, agent.**
- **`JobBody`'s target comment** (`internal/core/api/jobs.go`) lists the kinds whose target is a
  file only up to `make_chunks`, missing `hash_whole_file`, `make_system_videos` and `find_repeats`.
  **S, agent.**
- **A find_repeats job's `Target.FileID` is not the file the job is about:** it is the season's
  lowest unmeasured file (`internal/core/library/repeats.go:121-180`), standing for the season, and
  the jobs page shows it as the job's file (`web/apps/web/src/jobs.tsx:54-63`, `targetId`). A person
  reading the queue takes the job for one file's. Name the season in the target, or say so on the
  page: **S**, the target's shape is the API's, **owner's**.
- **`subtitles.RecordFailure`'s name** says nothing of which two kinds it records; goes with finding
  6.
- **Page and tile interest heartbeats** (`web/apps/web/src/page-interest.ts:30-58` and
  `tile-interest.ts:140-170`) each keep a timer that says interest again; they differ in what they
  name and when they stop. A candidate for [the watch list](../../look-alike-watch-list.md), not a
  merge.
- **Three packages are new since 2026-11-10:** `internal/core/problemreport`,
  `internal/core/unexpected` and `internal/node/fingerprint`. The
  [architecture map](../architecture-map.md) has a row for each (`make check-documentation` checks
  it); whether their `doc.go` invariants cite tests, this audit did not check.
- **`settings.fits`** (`internal/core/settings/settings.go:398-413`) answers false for a kind it
  does not know, which reads as "the value does not fit" rather than "the setting is broken". The
  kinds are a named type, so `exhaustive` covers the switch; low risk, noted for the next change to
  it.

## Not tried

- No code was run or measured: the costs are judged from reading.
- The checklist's items on derived data (9) and calls to other hosts (11) had only a light pass: the
  shared backoff and refusal handling were read, but not every new call's policy for a 404, a 429 or
  a timeout.
- `web/apps/cast-receiver` was read only for its message dispatch (`caf.ts`, `lab.ts`, `device.ts`).
- `pkg/` and `internal/store` beyond the cache schema and the queries named were not read for
  duplication.
- The new packages' invariants (above) were not checked.
- No search was made for duplicated SQL beyond the main-picture rule.
