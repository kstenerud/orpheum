> **ABOUTME:** Which package does which job, where each kind of data lives and whether it is
> backed up, the seams between core, node, store and client, and the findings a map of them
> turned up. Re-derived from the code at every milestone's audit.

# Architecture map

- **Status:** drafted 2026-09-30 at the M3 closing audit, from the tree at an earlier commit; file:line
  evidence is as of that tree. The owner approved its gates and fixes the same day (*"go with
  your recommendations"*). What each finding became is under [§4](#4-findings-ranked).
- **Kept true by:** `scripts/check_architecture_map.py`, run by `make check`, which fails when
  a package has no row in §1 or a row names a package that is gone, and when schema.md's
  [owners table](plans/schema.md#who-writes-each-table) does not list exactly the tables the
  schema has; `TestArch_EachTableHasOneWritingPackage`, which fails when a table has a writer
  other than the one that table names; and the milestone audit's re-derivation
  ([audit-checklist.md](../audit-checklist.md#the-architecture-map)).

Frame: responsibilities R1 fetch, R2 itemize, R3 transform, R4 stream, R5 identity/access,
R6a configuration, R6b preferences/behaviour, R7 decide, R8 core–node coordination and jobs,
R9 presentation, OPS operations/observability, LIVE live sessions. Durability tiers:
T1 irreplaceable, T2 expensive to re-acquire, T3 expensive to re-derive, T4 cheap to re-derive,
TO observed.

The code has four responsibilities that fit none of the nine. They are named here and used in the
tables:

- **CONTAIN, containment of hostile input**: confine, execx, bounded, the textindex sanitiser,
  library/clean.go, probe ("hostile bytes stop here"). It is a real seam with its own gates
  (forbidigo `[reveal]`, `core-no-image`).
- **PERSIST, storage custody**: store, datadir. The design treats this as a layer, not a feature.
- **COMPOSE, composition and lifecycle**: cmd/orpheum wiring. The node supervisor went in the jobs
  rebuild's Phase 4a: the node is a service of its own (D233 §2, D235 §4).
- **PRIM, shared primitives**: platform/{clock,slogx,errs,ptr,randbytes,readonly}.

Test support packages are listed as TEST.

## 1. Package table

Where a package does more than one job, the other jobs are flagged with ⚑. The file:line after ⚑
is the evidence. The first column names each package by its path; the check reads it.

### Go

| Package | Resp. | Second job(s) |
| --- | --- | --- |
| `cmd/orpheum` | COMPOSE | ⚑ R8 scheduling: queueDueRefreshes metadata.go:146, queueMeasurements :174 (hard-coded `measureSweepEvery = time.Hour` :170), keepSortAndSearchKeysCurrent :199, maintainArtworkCache artwork.go:45. ⚑ OPS: a whole offline doctor, doctor.go:34-290 |
| `internal/core/api` | R9 | ⚑ R4: relays bytes (relayDirect playback.go:206, relayHLSFile :234) and serves art (art.go:51) and subtitles (subtitle.go:104). ⚑ OPS: doctor.go, system.go:270 measures disks. The doc admits the relay job |
| `internal/core/artwork` | R3 (derived artwork store) | ⚑ R1: fetches its images (retry.go), through the image server's provider service since 2026-09-30, which replaced its own breaker. ⚑ R4: serves images (serve.go). ⚑ R8: cache sweep (ceiling.go). The one writer of `artwork` and owner of the image vocabulary since 2026-09-30 (F5); it imports no identify |
| `internal/core/auth` | R5 | — |
| `internal/core/backup` | OPS | — |
| `internal/core/datadir` | PERSIST | — (its `ListenUnix`, which the node role called, moved to `internal/platform/unixsocket` on 2026-09-30: F8) |
| `internal/core/decide` | R7 | — (pure: imports nodeproto and the standard library alone; depguard `decide-sees-capabilities`; since 2026-10-03 also the levelling arithmetic, levels.go, F2) |
| `internal/core/identify` | R7 (matching, choosing) | ⚑ R2: builds Title/Version/Extra identity from paths (pass.go, titles.go, episodes.go, extras.go) and decides `file.filing` (match.go:407, titles.go:166), which it records through library since 2026-09-30. ⚑ R1: orchestrates provider fetches (refresh.go). ⚑ R3: sort keys (sortkeys.go). Finds an item's images and reports them; since 2026-09-30 it writes no `artwork` row and imports no artwork (F5) |
| `internal/core/providers` | R1 (each outside host as a provider service: health, rate, quota, credentials) | — (since 2026-09-30, the jobs rebuild's Phase 2; a host's health is its destination in the job queue) |
| `internal/core/providers/tmdb` | R1 | — (also the TMDB image `artwork.Fetcher`, same responsibility; moved from `identify/tmdb` on 2026-09-30, the jobs rebuild's Phase 2) |
| `internal/core/jobs` | R8 | — (Orpheum's use of `internal/platform/jobqueue` since the jobs rebuild's Phase 1: its kinds, targets and answers, and the job view) |
| `internal/core/library` | R2 | ⚑ R9 read models: browse.go, home.go, detail.go, art.go. ⚑ R6a: library creation and admin (create.go, admin.go) |
| `internal/core/nodes` | R8 | — |
| `internal/core/loudness` | R8 (the loudness measurement jobs and their sweep), R3 (the measurements, by payload hash) | — (split out of playback on 2026-10-03, F2) |
| `internal/core/playback` | R6b (play state) | — (the preferences store moved out on 2026-09-30; loudness, subtitles and renditions on 2026-10-03, F2) |
| `internal/core/renditions` | R8 (the prepare jobs), R3 (the index of prepared copies, in cache.db) | — (split out of playback on 2026-10-03, F2) |
| `internal/core/subtitles` | R8 (the subtitle extraction and conversion jobs), R3 (the texts, by source hash) | ⚑ R1: subtitle search and fetch through the provider client (fetch.go). Split out of playback on 2026-10-03, F2 |
| `internal/core/preferences` | R6b (a person's choices) | — (since 2026-09-30: F2's preferences part, F6) |
| `internal/core/providers/opensubtitles` | R1 | — (moved from `playback/opensubtitles` on 2026-09-30, the jobs rebuild's Phase 2) |
| `internal/core/secrets` | R5 (secret custody) | — |
| `internal/core/settings` | R6a | — (the server's alone: its unused `user` scope was removed on 2026-09-30, F6) |
| `internal/core/textindex` | R3 (cheap: folding, CLDR keys, search routing) | ⚑ CONTAIN: the one display sanitiser (Line/Prose, used by library/clean.go:22-37, auth, opensubtitles) |
| `internal/core/together`, `internal/core/turn` | LIVE (doc-only stubs) | — |
| `internal/core/webclient` | R9 (delivers the client) | — |
| `internal/node/agent` | R8 | — |
| `internal/node/artwork` | R3 | — |
| `internal/node/byteserver` | R4 | — |
| `internal/node/confine` | CONTAIN | ⚑ also the read-only `MediaRoot` handle that scan, namedfile and byteserver use (scan.go:47, namedfile.go:64) |
| `internal/node/execx` | CONTAIN | — |
| `internal/node/hardware` | OPS | — |
| `internal/node/hash` | R2 (file identity) | — |
| `internal/node/loudness` | R3 | — (borrows prepare's mix filters, measure.go:99) |
| `internal/node/namedfile` | R2 (identity-checked open) | — |
| `internal/node/prepare` | R3 | — |
| `internal/node/prepared` | R3 (rendition store custody) | — |
| `internal/node/probe` | R2 | — |
| `internal/node/scan` | R2 | — |
| `internal/node/subtitle` | R3 | — |
| `internal/nodeproto` | R8 | — |
| `internal/platform/outbound`, `internal/platform/bounded` | R1 support | — |
| `internal/platform/unixsocket` | R8 support | — (dialling and, since 2026-09-30, listening, for both roles) |
| `internal/platform/jobqueue` | R8 support | — (the generic job queue of D231 §2, since the jobs rebuild's Phase 1; it knows no kind, target or destination, and its storage is `internal/store`'s `JobStorage` on cache.db, so either role could use it) |
| `internal/platform/langtag` | R2 (normalise on entry) | — |
| `internal/platform/build`, `internal/platform/host`, `internal/platform/diskspace` | OPS | — |
| `internal/platform/clock`, `internal/platform/slogx`, `internal/platform/errs`, `internal/platform/ptr`, `internal/platform/randbytes`, `internal/platform/readonly` | PRIM | — |
| `internal/store`, `internal/store/sqlcgen`, `internal/store/cachegen` | PERSIST | — (the `job` table's one writer since the jobs rebuild's Phase 1, as the queue's storage: D231 §2) |
| `internal/archtest`, `internal/testutil`, `internal/store/storetest`, `internal/core/auth/authtest`, `internal/core/identify/identifytest`, `internal/core/jobs/jobstest`, `internal/core/nodes/nodestest`, `internal/core/library/librarytest`, `internal/core/settings/settingstest`, `internal/node/nodetest`, `internal/platform/clock/clocktest`, `internal/platform/jobqueue/jobqueuetest`, `internal/core/providers/providerstest` | TEST | — |

### Web

| Package / module | Resp. | Notes |
| --- | --- | --- |
| `web/packages/sdk` | R9 (client API access) | Also the client's sensing (playable-formats.ts), which is the client-side analogue of "nodes sense, the core decides" |
| `web/packages/player` | R4 (client end of streaming: hls.ts, subtitles.ts, gain.ts) | R6b capture: report.ts. Its `PlayReport` was a hand copy of the generated `ReportInputBody` until 2026-09-30, and is now derived from it (F10) |
| `web/apps/web` | R9 | — |
| `web/apps/cast-receiver` | R9 (stub until M5) | — |
| `web/apps/web`: session.ts, accounts.ts, device.ts, signed-out-watch.ts, browser-storage.ts, sign-in/register/code-sign-in/device-code | R5 (client side) | browser storage holds only the session |
| `web/apps/web`: play-state.ts | R6b capture | sends reports |
| `web/apps/web`: settings.tsx | R6a | — |
| `web/apps/web`: sound.tsx, levelling.tsx, subtitles.tsx, subtitle-search.tsx | R6b | — |
| `web/apps/web`: system.tsx, jobs.tsx, node-confinement.tsx | OPS views | — |
| `web/apps/web`: router, navigation, use-resource, use-request, use-pages, services, i18n, *-text, problem.ts | R9 plumbing | `problem.ts` re-exports the SDK's refusal type and only adds wording. No duplication found |
| `web/apps/web`: every other screen (browse, title, series, search, play, …) | R9 | — |

### Responsibilities with two or more owners doing the same kind of work

| Work | Owners | Evidence |
| --- | --- | --- |
| Send a node command, wait for its answer by job id, time out, cancel | artwork, playback ×3 | **Fixed 2026-10-01 (the jobs rebuild's Phase 4.0):** `nodes.Exchanges` (nodes/exchange.go) sends, waits on the core's clock and cancels on every way out; the five handlers build the command and take the result. Was: five copies of one `select` around `nodes.AnswerRouter`, in artwork/service.go, playback/loudness.go, playback/subtitles.go, playback/fetch.go and playback/prepare.go |
| Retry and back-off after a provider failure | jobs runner, artwork, identify | **Fixed 2026-09-30 (the jobs rebuild's Phase 2):** a host down or asking us to wait parks its destination, which the queue probes (internal/platform/jobqueue), and each host's provider service (internal/core/providers) is the one place a failure is read as the host's. Was: jobs/runner.go:138-148 doubled the wait; artwork/retry.go:21 had its own schedule and a per-host breaker (:43-78); the 429 → `RetryAtError` translation was copied (artwork/retry.go:55, identify/match.go:430). artwork keeps its retry schedule for an image's own failures |
| Periodic sweeps and timers | cmd/orpheum ×4, jobs runner, nodes supervisor (gone in Phase 4a) | Hand-written loops at metadata.go:146/174/199 and artwork.go:45. Each picks its own period: a setting, a constant, or a literal 24 h and 1 h |
| Doctor findings | cmd/orpheum, api | cmd/orpheum/doctor.go:25-38 (`finding`, verdicts `ok`/`warn`/`FAIL`) against api/doctor.go:17-28 (`FindingBody`, `ok`/`warn`/`fail`). They are joined only through `api.ConfinementFinding` (doctor.go:108) |
| Writing `file` | library, identify | **Fixed 2026-09-30:** library writes it alone; identify files through `library.FileAs` and `library.PutInVersion`. Was: library/observe.go:184 `UpdateFileFilingAndLastSeen`; identify/match.go:407 and titles.go:166 `SetFileFiling`; pass.go:367 `SetFileVersionAndIdentifiedPath` |
| Writing `artwork` | identify, artwork | **Fixed 2026-09-30:** the artwork service writes it alone (artwork/images.go); identify reports the images it found and the server's wiring hands them over (cmd/orpheum/artwork.go). Was: identify/match.go:284, localart.go:128, refresh.go:239; artwork/service.go:377,384 |
| A person's or device's choices | preferences (`preference`); auth (`device.audio_output`) | **Fixed 2026-09-30:** a person's choices live only in internal/core/preferences, and `setting` lost its unused `user` scope and `/me/settings`; how a device makes sound stays on its row, a fact about its speakers. Was: playback/preferences.go:140-170; auth/devices.go:74; settings/settings.go:249 |
| The admin's tunables | config file, `setting` | cmd/orpheum/config.go:101-160 holds starting values in code (prepared_budget_gib, probe_timeout_seconds, status_interval_seconds …); settings/registry.go holds D200 settings. config.go:141 couples one to the other in prose: "keep it well under the core's node_silent_after_seconds" |
| Media-request authentication | api ×2 spellings | **Fixed 2026-09-30:** `requireSignedInForMedia` returns the principal and signs every byte path in. Was: media_cookie.go:47, and the same steps inline at playback.go:209, :240 and subtitle.go:122 |
| Byte-path cache headers | api ×4 literals | **Fixed 2026-09-30:** one named constant per kind of bytes, in media_cookie.go. Was: art.go:19, playback.go:225/255/257, subtitle.go:142, with `"private, max-age=86400"` twice |

## 2. Storage table

"Backed up" means included in `orpheum backup` (VACUUM INTO of persistent.db only; backup.go:30-51,
with `secret.key` only under `--include-key`). Which package writes each table is schema.md's
[owners table](plans/schema.md#who-writes-each-table).

| What | Where | Tier (actual → should) | Backed up | Flag |
| --- | --- | --- | --- | --- |
| user, role, user_deny, device, grant | persistent.db | T1 | yes | — |
| setting, secret (sealed) | persistent.db | T1 | yes (secret.key: only with --include-key, D194 §4) | by design |
| play_position, play_event, play_span, progress_hide, preference, favourite, user_rating, watchlist, tag, item_tag | persistent.db | T1 | yes | tag, favourite, rating and watchlist have no writer yet |
| field_override, edition (user-made), extra_relation (asserted), collection (user) | persistent.db | T1 | yes | — |
| item, title, version, item_provider_id, collection_member, ordering, position | persistent.db | T1 identity + TO/T2 resolved columns | yes | ⚑ mixed tiers per column (title, collection: O/F/A) |
| file | persistent.db | T1 identity columns + TO | yes | ⚑ mixed; one writer since 2026-09-30 (§1) |
| root_state | persistent.db | TO + one T1 column (`are_removals_approved`) | yes | ⚑ mixed |
| library_root, root_scan, root_mount, stream | persistent.db | TO | yes | — |
| node | persistent.db | T1 (enrolment) + TO (`last_status_json`, `last_status_at`) | yes | ⚑ status is transient, and backed up |
| provider_response (body_gzip) | persistent.db | T2 | yes | correct (moved from cache.db by the second audit, schema.md:803) |
| fetched_subtitle (body_gzip) | persistent.db | T2 | yes | correct |
| loudness | persistent.db | T3 (reads the whole file; the schema calls it "class O") | yes | ⚑ T3 placed as TO; the one T3 that survives a cache discard |
| item_text (name, overview) | persistent.db | TO/T2, resolved | yes | — |
| item_text.search_alternates, item_fts, item_fts_tri, sort_key | persistent.db | T4 | yes | ⚑ cheap derived in the backup; re-derived at start anyway (metadata.go:199). See F12 |
| schema_meta, schema_step | persistent.db | runner record | yes | — |
| job | cache.db | T4 | no | correct (D231 §3). But the code is not yet level-triggered: only loudness is swept (loudness.go:157). First matches, artwork, subtitles and prepares are enqueued once (refresh.go:305/356, artwork/service.go:130, subtitles.go:203, prepare.go:115) |
| match_candidate | cache.db | T2 (a TMDB search) | no | ⚑ T2 in cache.db. Mitigated: chooser.go:155-178 searches again when the candidates are lost |
| art_served | cache.db | T4 | no | — |
| prepared_rendition (index of node renditions) | cache.db | T3 | no | ⚑ discarded on every cache schema bump (serve.go:202-212, now version 19). The node's files survive, but no row names them and a new `run_id` is minted, so the renditions are re-made and the old ones sit until the budget evicts them (prepared/doc.go) |
| subtitle_text (ass, vtt) | persistent.db | T3 for embedded tracks (reads the whole video); T4 for fetched ones | yes | correct: moved from cache.db by migration 0029 (F1, the jobs rebuild's Phase 3) |
| secret.key | state dir | T1 | only with --include-key | by design (D194) |
| setup-code file | state dir | transient | no | — |
| backups/*.sqlite, pre-restore and pre-migrate snapshots | state dir | T1 copies | — | same disk (doctor.go:268 says so); manual until M4's scheduled backups (cut-a M4, D223 §6) |
| art/<source sha256>/<size>.webp and .jpeg | cache dir | T3 (node-encoded); the provider source behind it is T2 | no | ⚑ provider images are not kept or backed up. Accepted by artwork.md:102 ("deleting cache/ costs time"); the refetch goes to the TMDB image CDN |
| prepared/<file_id>/<profile>/<run_id>/ | node prepared_dir | T3 | no | correct place; the index is the problem (above) |
| TLS/ACME keys | state dir (datadir doc) | T1/T2 | not in backup.go | M4; not built yet |

## 3. Seams

**Core ↔ node.** This seam is strong and enforced. depguard has `core-boundary`, `node-boundary`,
`nodeproto-is-types` and `exec-only-in-execx` in `.golangci.yml`. The archtests are
CoreNeverImportsNode, NodeNeverImportsCore, CoreNeverExecs and NodeprotoHasNoLogic. The one
crossing, the node role in cmd/orpheum importing `internal/core/datadir` for `ListenUnix`
(cmd/orpheum/node.go:20, :228), was closed on 2026-09-30: listening moved to
`internal/platform/unixsocket`, and the depguard rule `node-role-boundary` now holds
cmd/orpheum/node.go to the node's boundary (F8).

**Store.** `store-is-below-core` and `sql-only-in-store` keep SQL in one place. Every core feature
package imports `sqlcgen` (api, artwork, auth, identify, jobs, library, nodes, playback, secrets,
settings), so any package could write any table, and two tables had two writers (§1). Since
2026-09-30 each table has one writing package, named in schema.md, and
`TestArch_EachTableHasOneWritingPackage` holds it (F5). api imports `sqlcgen` only for a row type
(browse.go:261), which leaks the store's shape into R9.

**Fetching (R1).** Clean. All outbound HTTP goes through `outbound.NewClient` and `Exchange`
(forbidigo `[outbound]`). The callers are tmdb/client.go:67, tmdb/images.go:65,
opensubtitles/client.go:249 and opensubtitles/requests.go:177, and cmd/orpheum/metadata.go:57
builds the one client. The failure policy is unified since 2026-09-30: each host is a provider
service (internal/core/providers) owning its health, rate, quota and credentials, and a host that
is down parks its destination's work in the queue (D231 §5, §6).

**Transforming (R3).** FFmpeg work happens only on nodes (execx and confine). The core does three
cheap derivations: textindex folding and CLDR keys; artwork cutting JPEG from its own WebP (the
`core-no-image` exception, per D212 §5); and levelling-gain arithmetic
(playback/loudness.go:80-155). All three agree with D193 §1.

**Streaming (R4).** Direct play and HLS share one relay (`server.relay`, playback.go:343). Range and
If-Range are forwarded, and the node's byteserver answers both with `http.ServeContent`
(byteserver.go:94, :127). "425 while being made" is one helper, `tooEarly` (playback.go:327), used by
HLS and subtitles. Art (art.go:51) and subtitles (subtitle.go:104) are served from the core's own
stores. Since 2026-09-30 the four byte paths share one sign-in and one set of Cache-Control values
(F9); there is still no shared refusal policy across them.

**Decisions (R7).** `decide` is pure and gated. `identify.singleMatch` (match.go:151) and
`library.changedFactsReason` (complete.go:166) are pure. `library.withholdReason` (complete.go:181)
mixes the D42 withholding decision with reads of root_state and two settings. The rule for which
artwork is shown lives in SQL (queries/artwork.sql:32-40: chosen, then the library's preferred
source, then the newest); library/art.go:21 described it differently until 2026-09-30 (F11).

**Intent (R6).** Server settings have one mechanism (settings.Service, D200). Node and bootstrap
tunables use a second one, the config file with starting values in code. For a person's choices
there are three places (§1). Play behaviour has one owner (playback/report.go into play_*).

**Sensing vs deciding at the client.** `POST /playback` takes `network: local|remote` from the
client (cut-a §5 decision), and the web client hard-codes `"local"` (apps/web/src/play.tsx:42).
Today the remote bit-rate ceiling (`remote_bit_rate_ceiling_kbps`) is applied only if the client
declares that it is remote.

## 4. Findings, ranked

The owner decided F1, F2, F5, F6 and F13 on 2026-09-30 by approving the recommendations below.
Each finding says what it became.

**F1. Data in the wrong tier: T3 data has no consistent home, and a routine release throws it away.**
*Decided 2026-09-30; built with D230/D231.*
Embedded-subtitle text (subtitle_text) and the index of prepared renditions (prepared_rendition)
are in cache.db. cache.db is discarded on any cache schema change (serve.go:202-212), and there
have been 19 versions in two milestones. After a discard, every rendition is re-transcoded and
every embedded track re-extracted with a whole-file read. The old rendition directories stay on the
node, unreferenced, until the budget evicts them. Meanwhile loudness, the same kind of costly
measurement, sits in persistent.db and is backed up.

D231 §3 says losing cache.db "loses nothing but timing", and schema.md accepts "re-transcodes".
The frame's T3 rule ("kept across restarts") is stricter than both. The two choices were:

- (a) accept it, and write down that T3 data lives in cache.db and is lost on a bump; or
- (b) give T3 a store that survives a cache version bump. Options: a separate `derived.db`; keep
  T3 in persistent.db like loudness, and leave it out of the backup with VACUUM INTO filtering; or let
  the node report its renditions on hello so the core can re-adopt them.

Either way, `loudness` is re-classed as T3 in schema.md.

**F2. A package with five jobs: core/playback.** *Decided 2026-09-30; built with D230/D231. The
preferences part was built with D230 on 2026-09-30: internal/core/preferences keeps a person's
choices, which playback and the API use. The node-work split was built on 2026-10-03, after
D231: internal/core/loudness, internal/core/subtitles and internal/core/renditions, with the
levelling arithmetic in internal/core/decide; playback keeps play state and starts playbacks.*
It holds play state (R6b), the per-person preference store, three node job handlers (prepare,
subtitle extraction, loudness), subtitle fetching (R1) and the levelling decision (R7). The
recommendation: split the node-work orchestration out (subtitle text, loudness, and possibly
prepare), and move preferences to wherever F6 lands. cut-a §3 names the package layout, so a new
package is a layout change.

**F3. A responsibility with four owners: the node-command wait loop.** *No owner needed.*
The same send/await/timeout/cancel `select` appears in four places (§1). Recommend one helper in
core/nodes, for example `Ask(ctx, nodeID, command, jobID, patience) (Answer, error)`, used by all
four. This is rule 11 work with a revert-red test through one caller.

**F4. A responsibility with scattered owners: scheduling and retry.** *Decided by D231.*
The sweeps and timers are hand-written loops in cmd/orpheum, with a hard-coded policy value
(`measureSweepEvery`). Retry policy is split across the jobs runner, artwork's breaker and
identify. Work is mostly edge-triggered. D231 already decides the fix: a generic queue package,
level-triggered sweeps and destination parking. Treat this map's list as D231's acceptance
checklist, so every loop in cmd/orpheum becomes a sweep registered with the queue and
artwork/retry.go goes. Keeping `measureSweepEvery` as a constant is a D200 question to settle while
doing it.

**F5. A package with two jobs, and tables with two writers: identify and library.**
*Decided 2026-09-30. The one writer per table was built that day; identify's two jobs are
addressed with D230/D231.*
identify both itemizes (it builds Title, Version and Extra identity, and decides `file.filing`) and
matches. library also wrote `file.filing` (observe.go:184), and identify and artwork both wrote
`artwork`. Nothing assigned a table to one owner. Built: schema.md's
[owners table](plans/schema.md#who-writes-each-table) names one writing package per table;
library writes `file` and identify files through it; the artwork service writes `artwork` and
owns the image vocabulary, and identify, which finds an item's images, reports them for the
server's wiring to hand to it: neither package imports the other, and a refresh's request to look
at images again, and artwork's recheck of a missing image, cross through the wiring too, until
D231 §4 makes a refresh a stored fact (the owner, 2026-09-30: *"Wouldn't the artwork service be
responsible for the artwork stuff? Identify would make use of it."*);
`TestArch_EachTableHasOneWritingPackage` maps each query file's writes to the packages calling
them, by type-checking, and fails on a second writer.

**F6. A responsibility with three owners: a person's and a device's choices.**
*Decided 2026-09-30; built with D230 on 2026-09-30: a person's choices live only in
internal/core/preferences; `device.audio_output` stays on the device row, a fact about its
speakers rather than a choice; `setting`'s `user` scope, which nothing defined, is gone with
`/me/settings`.*
These live in `preference` (with a `device` scope), `device.audio_output`, and a `setting` scope
`user` that has no definitions. The recommendation: one home for per-person and per-device choices
(probably `preference`), and either drop the unused user scope or give it a stated purpose.

**F7. A responsibility with two owners: doctor.** *No owner needed.*
There are two finding types with two verdict spellings ("FAIL" and "fail"). Recommend one
`Finding` type and one verdict enum, used by the command and by `GET /doctor`. This is rule 11
work.

**F8. A seam crossed: the node role imports a core package.** *Fixed 2026-09-30.*
cmd/orpheum/node.go:228 called `datadir.ListenUnix`. `ListenUnix` moved to
platform/unixsocket as `Listen`, beside the dialling it already owned, and the depguard rule
`node-role-boundary` refuses any core or store import in cmd/orpheum/node.go.

**F9. A responsibility with two spellings: auth and cache headers on the byte paths.**
*Fixed 2026-09-30.* `requireSignedInForMedia` returns the principal and is used by all four byte
handlers, and the Cache-Control values are named constants: immutable, for a day, revalidate,
no-store.

**F10. A duplicated wire type: `PlayReport`.** *Fixed 2026-09-30.*
player/report.ts:22 was a hand copy of `ReportInputBody`, kept in step only because the app passes
one to the other and TypeScript checks structural assignability (play-state.ts:28). No
dependency-cruiser rule stops the player importing the SDK (only the reverse is forbidden), so the
player now derives `PlayReport`, `PlayEvent` and `PlaySpan` from the SDK's generated types.

**F11. A decision mixed with I/O, and policy in SQL.** *No owner needed; the comment is fixed.*
`withholdReason` could take `(prior, settings, counts)` and be pure. The artwork precedence rule
in SQL should be named in library's and artwork's doc.go. library/art.go:21's comment was
corrected on 2026-09-30.

**F12. Cheap derived data in the backup.** *Accepted.*
sort_key, both FTS tables, search_alternates and `node.last_status_json` are in persistent.db. The
bloat is small at Cut A's scale, and startup re-derives the sort and search keys anyway.
schema.md's conventions already accept backing persistent.db up whole: at the reference scale it is
46 MB and 177 ms, so splitting it by table bought nothing. Leaving the derived tables out needs
no schema change, but it is not cheap: `VACUUM INTO` copies the whole file, so emptying them means
a second pass over the copy, and a restore would then have to rebuild the FTS indexes, which the
start-up re-keying does not do when no name's fold changed.

**F13. Client-declared network locality.** *Decided 2026-09-30; joins M4's questions.*
Before M4's remote person, decide whether the core senses locality (from the request's address or
the HTTPS name) or trusts the client. The web client currently always says `local`.

**Not findings:**

- Media tokens (playback/tokens.go) grant nothing alone (playback doc:46), so R5 still has one
  owner.
- `problem.ts` in the app re-exports the SDK's refusal type.
- The one sanitiser is shared by library, auth and opensubtitles.
- `langtag` is the single normaliser for both sides.

## 5. Keeping it true

The milestone audit re-derives this map from the code, by the steps in
[audit-checklist.md](../audit-checklist.md#the-architecture-map). `make check` does the
mechanical part of steps 1, 3 and 4 on every change (see the status line above).
