> **ABOUTME:** Which package does which job, where each kind of data lives and whether it is
> backed up, the seams between core, node, store and client, and the findings a map of them
> turned up. Re-derived from the code at every milestone's audit.

# Architecture map

- **Status:** drafted 2026-09-30 at the M3 closing audit, from the tree at an earlier commit; the owner
  approved its gates and fixes the same day (*"go with your recommendations"*). **Re-derived
  2026-10-04 at the pre-M4 audit** ([audit-2026-10-03.md](research/audit-2026-10-03.md) §6),
  from the tree at an earlier commit: §1's rows, §2 whole, §3's crossings and §4's statuses. file:line
  evidence older than that is as of the tree it names. What each finding became is under
  [§4](#4-findings-ranked).
- **Kept true by:** `scripts/check_architecture_map.py`, run by `make check`, which fails when
  a package has no row in §1 or a row names a package that is gone, when §2 does not name each
  table of both databases exactly once, in a row of its database, and when schema.md's
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
- **PERSIST, storage custody**: store, instancedir. The design treats this as a layer, not a feature.
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
| `cmd/orpheum` | COMPOSE | ⚑ R8 scheduling: the seventeen sweeps it registers with the queue (`jobServices.sweeps`, metadata.go; D231 §3), a runner per kind each ticking every second, and loops of its own beside them (`startBackgroundLoops` and `startUpkeep`, metadata.go): `demoteUnwanted` (every 5 s), the interest raiser's (5 s), `pruneFinished` (hourly), the silence watch it ticks (`Exchanges.Watch`), `maintainArtworkCache` (artwork.go), the sessions' uses written in batches, and the playback's upload watch and share (2026-10-26). ⚑ PERSIST: makes an instance's layout and config (instance.go, D242 §4). ⚑ OPS: a whole offline doctor, doctor.go |
| `internal/deployment` | OPS | — (writes a deployment's directory from its values file: the compose file, the configs, the tunnel's keys; `orpheum write-deployment` runs it in the image, D272 §3) |
| `internal/core/api` | R9 | ⚑ R4: relays bytes (relayDirect playback.go:206, relayHLSFile :234) and serves art (art.go:51) and subtitles (subtitle.go:104). ⚑ OPS: doctor.go, system.go:270 measures disks. The doc admits the relay job |
| `internal/core/artwork` | R3 (derived artwork store) | ⚑ R1: fetches its images (retry.go), through the image server's provider service since 2026-09-30, which replaced its own breaker. ⚑ R4: serves images (serve.go). ⚑ R8: cache sweep (ceiling.go). The one writer of `artwork` and owner of the image vocabulary since 2026-09-30 (F5); it imports no identify |
| `internal/core/auth` | R5 | — |
| `internal/core/backup` | OPS | — (a backup is a job since 2026-10-05, D243 §11) |
| `internal/core/certificate` | OPS (the HTTPS certificate of the server's name) | ⚑ R1: asks the CA by ACME, through the outbound client (D243 §6) |
| `internal/core/providers/cloudflare` | R1 (the DNS provider: a DNS-01 challenge's TXT record) | — |
| `internal/core/instancedir` | PERSIST | — (its `ListenUnix`, which the node role called, moved to `internal/platform/unixsocket` on 2026-09-30: F8) |
| `internal/core/decide` | R7 | — (pure: imports nodeproto and the standard library alone; depguard `decide-sees-capabilities`; since 2026-10-03 also the levelling arithmetic, levels.go, F2) |
| `internal/core/identify` | R7 (matching, choosing) | ⚑ R2: builds Title/Version/Extra identity from paths (pass.go, titles.go, episodes.go, extras.go) and decides `file.filing` (match.go:407, titles.go:166), which it records through library since 2026-09-30. ⚑ R1: orchestrates provider fetches (refresh.go). ⚑ R3: sort keys (sortkeys.go). Finds an item's images and reports them; since 2026-09-30 it writes no `artwork` row and imports no artwork (F5) |
| `internal/core/providers` | R1 (each outside host as a provider service: health, rate, quota, credentials) | — (since 2026-09-30, the jobs rebuild's Phase 2; a host's health is its destination in the job queue) |
| `internal/core/providers/tmdb` | R1 | — (also the TMDB image `artwork.Fetcher`, same responsibility; moved from `identify/tmdb` on 2026-09-30, the jobs rebuild's Phase 2) |
| `internal/core/jobs` | R8 | — (Orpheum's use of `internal/platform/jobqueue` since the jobs rebuild's Phase 1: its kinds, targets and answers, and the job view) |
| `internal/core/interest` | R8 | — (D324: what each session plays and looks at, kept in memory, raising and releasing the work of it on the one queue) |
| `internal/core/live` | LIVE (D337: which of each person's devices are connected now, and the casts between them, in memory; built 2026-10-25) | — (the presence socket is internal/core/api's; a cast's control is internal/core/together's) |
| `internal/core/waits` | R8 | — (D324 §6: how long the work a viewer waits on will take, a file's read-through and its prepared copy, from each node's measured read speed, kept in memory) |
| `internal/core/library` | R2 | ⚑ R9 read models: browse.go, home.go, detail.go, art.go. ⚑ R6a: library creation and admin (create.go, admin.go, retire.go, move.go). ⚑ R8: scans as `list` and `examine` jobs (scans.go, batches.go; D241), and the one rule for parking work under an unavailable root (`Unavailability`, `ParkIfUnder`; unavailable.go, playable.go) |
| `internal/core/nodes` | R8 | ⚑ R6a: an admin's reader limit for one of a node's devices (devices.go, D240 §5) |
| `internal/core/loudness` | R7 (the gains that level a file's sounds, from its measurements) | — (split out of playback on 2026-10-03, F2; its job, sweep and writes moved to `internal/core/readthrough` on 2026-10-05) |
| `internal/core/readthrough` | R8 (the read-through jobs and their sweep), R3 (the loudness measurements and the pictures' timelines, by payload hash) | — (was `internal/core/loudness`'s job until 2026-10-05, made general for D247's grid) |
| `internal/core/chunks` | R7 (a film's grid of chunks, from its picture's timeline, and the playlists a playback is offered), R8 (the `make_chunks` runs, and in memory each store's chunks made and runs on their way) | — (D247 §10; built 2026-10-05) |
| `internal/core/chunks/qualities` | R7 (the setting `chunk_qualities`, read and checked by one function) | — (its own package so the settings registry checks the value with it, D247 §6) |
| `internal/core/playback` | R6b (play state) | — (the preferences store moved out on 2026-09-30; loudness, subtitles and renditions on 2026-10-03, F2) |
| `internal/core/renditions` | R8 (the prepare jobs), R3 (the index of prepared copies, in cache.db) | — (split out of playback on 2026-10-03, F2) |
| `internal/core/subtitles` | R8 (the subtitle extraction and conversion jobs), R3 (the texts, by source hash) | ⚑ R1: subtitle search and fetch through the provider client (fetch.go). Split out of playback on 2026-10-03, F2 |
| `internal/core/preferences` | R6b (a person's choices) | — (since 2026-09-30: F2's preferences part, F6) |
| `internal/core/providers/opensubtitles` | R1 | — (moved from `playback/opensubtitles` on 2026-09-30, the jobs rebuild's Phase 2) |
| `internal/core/rtcapi` | R9 (the API over WebRTC data channels, D273 §8) | — |
| `internal/core/discovery` | OPS (the core's standing with its discovery, D273) | ⚑ R1: calls the discovery through the outbound client; ⚑ R9: hands the offers it passes on to rtcapi |
| `internal/core/portmap` | OPS (the home router's port mapping, D273 §7) | ⚑ R1: speaks PCP, NAT-PMP and UPnP to the LAN's router |
| `internal/core/clientnet` | R1 (the network a client's address is on: link speeds, D257 §1; device codes, D288 §6) | — (moved out of playback on 2026-10-13) |
| `internal/core/identity` | PERSIST (the core's identity key, identity.key, D273) | — |
| `internal/core/rtcapi/rtcapitest` | TEST | — |
| `internal/core/secrets` | R5 (secret custody) | — |
| `internal/core/settings` | R6a | — (the server's alone: its unused `user` scope was removed on 2026-09-30, F6) |
| `internal/core/textindex` | R3 (cheap: folding, CLDR keys, search routing) | ⚑ CONTAIN: the one display sanitiser (Line/Prose, used by library/clean.go:22-37, auth, opensubtitles) |
| `internal/core/together` | LIVE (Watch Together's sessions, in memory; built 2026-10-05) | — (the WebSocket and endpoints are internal/core/api's) |
| `internal/core/reachability` | OPS (the System page's check of the public path, from outside in; built 2026-10-05) | ⚑ R1: asks the server's own public name and relay through the outbound client |
| `internal/core/turn` | LIVE (the TURN relay carrying voice, and its credentials; built 2026-10-05) | — (pion/turn does the protocol; the ICE endpoint is internal/core/api's) |
| `internal/core/webclient` | R9 (delivers the client) | — |
| `internal/node/agent` | R8 | — |
| `internal/node/artwork` | R3 | — |
| `internal/node/byteserver` | R4 | — |
| `internal/node/container` | R2 (container structure: Matroska elements, MP4 boxes) | — (hash anchors identity on them; read-throughs read a film's keyframes from their indexes, D259 §2) |
| `internal/node/confine` | CONTAIN | ⚑ also the read-only `MediaRoot` handle that scan, namedfile and byteserver use (scan.go:47, namedfile.go:64) |
| `internal/node/execx` | CONTAIN | — |
| `internal/node/fdproxy` | CONTAIN | — (D330: the node's side of fdproxy, over which FFmpeg reads its input film through the node, every frame FFmpeg sends judged and a breach, a failed read or a passed budget killing the run; execx runs a watched run's film through it (WatchedRun.Film); no run kind uses it yet, waiting on the patched FFmpeg being fetchable) |
| `internal/node/hardware` | OPS | — |
| `internal/node/hash` | R2 (file identity) | — |
| `internal/node/namedfile` | R2 (identity-checked open) | — |
| `internal/node/diskwatch` | R8 | — (D233 §10: one stuck operation at most on each root; a disk that does not answer is a stall, and its root is read no more until the stuck operation returns) |
| `internal/node/chunks` | R3 | — (borrows prepare's encoder and mix filters; its stores are `internal/node/prepared`'s, D247 §10) |
| `internal/node/diskschedule` | R8 | — (D329 §3, work-scheduling.md §8: for one device, which read goes next, by tier, deadline and arrival, with background reads held to a rate a slow playback read halves; a pure decider the node and `internal/schedtest` both drive; not yet wired into the node) |
| `internal/node/prepare` | R3 | — |
| `internal/node/prepared` | R3 (rendition store custody) | — |
| `internal/node/probe` | R2 | — |
| `internal/node/readthrough` | R3 | — (borrows prepare's mix filters, loudness.go; was `internal/node/loudness` until 2026-10-05) |
| `internal/node/scan` | R2 | — |
| `internal/node/subtitle` | R3 | — |
| `internal/node/watch` | R2 (change detection, D27) | — (notices what changed under each root for the core to scan, D272 §9) |
| `internal/nodeproto` | R8 | — |
| `discovery/discoveryproto` | R1 (the wire between a core and its discovery, D273) | — (public: the closed service imports it, D275 §1) |
| `discovery/link` | R1 (a core's link as a client reads it, `<coreId>.<secret>.<fingerprint>`, and its locate hash, the protocol's §3.1) | — (public: the closed service and the client read links one way, D275 §1) |
| `discovery/proof` | R1 (what a core's signatures cover, to its discovery and over a WebRTC answer, their checks, and the identity key's fingerprint, D273) | — (public: the closed service imports it, D275 §1) |
| `discovery/safe32code` | R1 (the codes a person types, written one way: a core's device and setup codes, a discovery's claim code, D218 §3, D223 §2) | — (public: the closed service writes the same codes, D275 §1) |
| `discovery/single` | R1 (a discovery for one core: the core-facing protocol, its record, and the core's signalling, W4b) | — (public: the closed service assembles the same blocks for many cores, D275 §1) |
| `internal/platform/outbound`, `internal/platform/bounded` | R1 support | — |
| `internal/platform/unixsocket` | R8 support | — (dialling and, since 2026-09-30, listening, for both roles) |
| `internal/platform/jobqueue` | R8 support | — (the generic job queue of D231 §2, since the jobs rebuild's Phase 1; it knows no kind, target or destination, and its storage is `internal/store`'s `JobStorage` on cache.db, so either role could use it) |
| `internal/platform/langtag` | R2 (normalise on entry) | — |
| `internal/platform/querytally` | R4 support (a request's database statements counted, D303 DL6, DL17) | — (the api counts each request's; the store counts into it) |
| `internal/platform/waiting` | R4 support (a person's writes before background work, D303 DL11) | — (the api marks each request; the store's writer gate reads the mark) |
| `internal/platform/pacing` | R4 support (a film's download giving way to live voice, D262 §2) | — (the core's api paces media requests by the player's header; built 2026-10-07) |
| `internal/platform/build`, `internal/platform/host`, `internal/platform/diskspace` | OPS | — |
| `internal/platform/clock`, `internal/platform/localtime`, `internal/platform/slogx`, `internal/platform/errs`, `internal/platform/ptr`, `internal/platform/pathtree`, `internal/platform/randbytes`, `internal/platform/readonly`, `internal/platform/backoff`, `internal/platform/atomics`, `internal/platform/httpgzip` | PRIM | — |
| `internal/store`, `internal/store/sqlcgen`, `internal/store/cachegen` | PERSIST | — (the `job` table's one writer since the jobs rebuild's Phase 1, as the queue's storage: D231 §2) |
| `internal/archtest`, `internal/testutil`, `internal/store/storetest`, `internal/store/storesources`, `internal/core/auth/authtest`, `internal/core/identify/identifytest`, `internal/core/jobs/jobstest`, `internal/core/certificate/certificatetest`, `internal/core/providers/cloudflare/cloudflaretest`, `internal/core/nodes/nodestest`, `internal/core/library/librarytest`, `internal/core/settings/settingstest`, `internal/node/nodetest`, `internal/node/container/containertest`, `internal/node/fdproxy/fdproxytest`, `internal/platform/clock/clocktest`, `internal/platform/jobqueue/jobqueuetest`, `internal/core/providers/providerstest` | TEST | — |
| `internal/schedtest` | TEST | — (D329's scheduling bench, work-scheduling.md §16 phase 0: a discrete-event simulation of disks, playbacks and the policy between them, with today's node work list as the baseline; outside core and node so it can drive the deciders of both) |

### Web

| Package / module | Resp. | Notes |
| --- | --- | --- |
| `web/packages/sdk` | R9 (client API access) | Also the client's sensing (playable-formats.ts), which is the client-side analogue of "nodes sense, the core decides"; and Watch Together's client (together*.ts: the connection, following the group, voice), a second responsibility the M4 audit flags for a package of its own |
| `web/packages/player` | R4 (client end of streaming: hls.ts, chunked.ts, link.ts, stalls.ts, step-up.ts, subtitles.ts, gain.ts) | R6b capture: report.ts. Its `PlayReport` was a hand copy of the generated `ReportInputBody` until 2026-09-30, and is now derived from it (F10) |
| `web/apps/web` | R9 | — |
| `web/apps/cast-receiver` | R9 (stub until M6, after Cut A) | — |
| `web/apps/web`: session.ts, accounts.ts, device.ts, signed-out-watch.ts, browser-storage.ts, sign-in/register/code-sign-in/device-code | R5 (client side) | browser storage holds the session, and for Watch Together each tab's memberships and a viewer's speakers (together-storage.ts) |
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
| Periodic sweeps and timers | the queue's sweeps; cmd/orpheum ×4 | **Mostly fixed (D231 §3, the jobs rebuild):** six sweeps are registered with the queue, which runs each at its interval, at start and when a destination comes back. Loops stay in cmd/orpheum (§1), each with its own period: `demoteUnwantedEvery` (5 s), `raiseInterestEvery` (5 s), `pruneFinishedEvery` (1 h), the artwork cache's two settings, the silence watch's one-second tick and the upload's `uploadLogEvery` and `uploadKeptEvery` (2026-10-26). Four sweeps share `derivedWorkSweepEvery`, a constant D231 §3 means to be a setting |
| Parking a job under an unavailable root | library, artwork | **Fixed 2026-10-04:** `library.ParkIfUnder` is the one. Was: `library.ParkIfUnavailable` (playable.go:107) and `artwork.Service.localFile` (service.go:170) |
| Waiting for a node's word on a job | nodes (`Exchanges.Watch`), subtitles | `subtitles.convertWhileWaiting` (fetch.go:265) times its person's wait with its own timer at the node-silence limit, beside the exchange's own silence watch. Two clocks on one silence; watch |
| Doctor findings | cmd/orpheum, api | **Fixed 2026-10-04 (F7):** one `api.Verdict` (`ok`, `warn`, `fail`), which the command's findings use too and print with a failure in capitals. Was: cmd/orpheum/doctor.go's own `verdict` (`FAIL`) against api/doctor.go's strings (`fail`) |
| Writing `file` | library, identify | **Fixed 2026-09-30:** library writes it alone; identify files through `library.FileAs` and `library.PutInVersion`. Was: library/observe.go:184 `UpdateFileFilingAndLastSeen`; identify/match.go:407 and titles.go:166 `SetFileFiling`; pass.go:367 `SetFileVersionAndIdentifiedPath` |
| Writing `artwork` | identify, artwork | **Fixed 2026-09-30:** the artwork service writes it alone (artwork/images.go); identify reports the images it found and the server's wiring hands them over (cmd/orpheum/artwork.go). Was: identify/match.go:284, localart.go:128, refresh.go:239; artwork/service.go:377,384 |
| A person's or device's choices | preferences (`preference`); auth (`device.audio_output`) | **Fixed 2026-09-30:** a person's choices live only in internal/core/preferences, and `setting` lost its unused `user` scope and `/me/settings`; how a device makes sound stays on its row, a fact about its speakers. Was: playback/preferences.go:140-170; auth/devices.go:74; settings/settings.go:249 |
| The admin's tunables | config file, `setting` | cmd/orpheum/config.go:101-160 holds starting values in code (prepared_budget_gib, probe_timeout_seconds …); settings/registry.go holds D200 settings. The node's status cadence, once a config key coupled in prose to node_silent_after_seconds, is derived from it by the core since 2026-10-21 |
| Media-request authentication | api ×2 spellings | **Fixed 2026-09-30:** `requireSignedInForMedia` returns the principal and signs every byte path in. Was: media_cookie.go:47, and the same steps inline at playback.go:209, :240 and subtitle.go:122 |
| Byte-path cache headers | api ×4 literals | **Fixed 2026-09-30:** one named constant per kind of bytes, in media_cookie.go. Was: art.go:19, playback.go:225/255/257, subtitle.go:142, with `"private, max-age=86400"` twice |

**Watch list** (alike, not yet the same job done twice): "the deepest path holding this one"
in three places; an atomic file write in instancedir and in the node's prepared shelf, neither
syncing; instance directories made by `makeInstance` (cmd/orpheum/instance.go) and
`instancedir.Open`; "a device not named has one reader" in both the core and the node.

## 2. Storage table

"Backed up" means in a backup: a tar.gz of the instance's `persistent/` directory, made by
`orpheum backup` or the System page's "Back up now" into `backups/`, with persistent.db as a
`VACUUM INTO` snapshot and `secret.key` only when asked (instancedir/archive.go; D242 §5). Which
package writes each table is schema.md's [owners table](plans/schema.md#who-writes-each-table).
`check_architecture_map.py` holds each table of both databases to one row here, in its database's.

| What | Where | Tier (actual → should) | Backed up | Flag |
| --- | --- | --- | --- | --- |
| user, role, role_library, invitation, invitation_link, user_deny, device, grant, locator, realm_key | persistent.db | T1 | yes | — |
| setting, secret (sealed) | persistent.db | T1 | yes (`secret.key` only when asked, D194 §4) | by design |
| play_position, play_event, play_span, purged_watching, progress_hide, preference, favourite, user_rating, watchlist, tag, item_tag | persistent.db | T1 | yes | tag, favourite, rating and watchlist have no writer yet |
| field_override, edition (user-made), edition_text, group_text, extra_relation (asserted), collection (user) | persistent.db | T1 | yes | edition_text and group_text have no writer yet |
| library, library_root | persistent.db | T1 (the admin's) | yes | — |
| item, title, version, item_provider_id, collection_member, ordering, position | persistent.db | T1 identity + TO/T2 resolved columns | yes | ⚑ mixed tiers per column (title, collection: O/F/A) |
| file | persistent.db | T1 identity columns + TO | yes | ⚑ mixed; one writer since 2026-09-30 (§1) |
| file_probe | persistent.db | TO | yes | what a scan probed, beside its file (D303, M8) |
| file_nfo | persistent.db | TO | yes | an NFO's bytes, beside its file (D303, M8) |
| root_state | persistent.db | TO + one T1 column (`are_removals_approved`) | yes | ⚑ mixed |
| root_scan, root_mount, stream | persistent.db | TO | yes | — |
| unavailable_place | persistent.db | TO | yes | availability kept per place, one writer (D328) |
| node | persistent.db | T1 (enrolment) | yes | — (its status is kept in memory alone since D303's step 0049) |
| device_reader_limit | persistent.db | T1 (the admin's, D240 §5) | yes | — |
| refresh_request, fetch_failure | persistent.db | T1 (a person's ask) / TO (a fetch's own failure, D231 §4) | yes | facts that were job rows (F1, step 0029) |
| guessed_match | persistent.db | TO (the matcher's guess, cleared by a person's choice, D301) | yes | — |
| provider_response (body_gzip) | persistent.db | T2 | yes | correct (moved from cache.db by the second audit) |
| fetched_subtitle (body_gzip) | persistent.db | T2 | yes | correct; one per file and provider's file (step 0032, D244 §4) |
| artwork | persistent.db | T1 (chosen) + T2 (the provider's list) | yes | the image bytes are the cache's (below) |
| loudness | persistent.db | T3 (reads the whole file; the schema calls it "class O") | yes | the T3 kept with T1, as subtitle_text is |
| video_timeline (a picture's keyframes and bytes in each second, by payload hash) | persistent.db | T3 (reads the whole file, copying the picture's packets) | yes | the T3 kept with T1, as loudness is, whose read gathers it (step 0035; one version in cache.db before, F1) |
| subtitle_text (ass, vtt) | persistent.db | T3 for embedded tracks (reads the whole video); T4 for fetched ones | yes | correct: moved from cache.db by step 0029 (F1) |
| item_text (name, overview) | persistent.db | TO/T2, resolved | yes | — |
| item_text.search_alternates, item_fts, item_fts_tri, sort_key | persistent.db | T4 | yes | ⚑ cheap derived in the backup; re-derived at start anyway. See F12 |
| schema_meta, schema_step | persistent.db | runner record | yes | — |
| job | cache.db | T4 | no | correct (D231 §3): level-triggered by the queue's six sweeps (§1) |
| node_attempt | cache.db | T4 (each job given to a node, by attempt token; D234 §7) | no | lost with cache.db: the node's later word is for no attempt, and the job runs again |
| scan, scan_batch | cache.db | T4 (a scan in progress; D241) | no | a lost scan is asked for again |
| match_candidate | cache.db | T2 (a TMDB search) | no | ⚑ T2 in cache.db. Mitigated: the chooser searches again when the candidates are lost |
| art_served | cache.db | T4 | no | — |
| chunk_store (a stream of a film on a grid, the target of its runs) | cache.db | T4 | no | made again by the next run asked for; the chunks themselves are the node's |
| prepared_rendition (index of node renditions) | cache.db | T3 | no | mitigated by D238 §2: a node's hello lists its renditions, and the core adopts them. cache.db is at version 27 |
| full config (`config.toml`) | instance's persistent/ | T1 | yes | D242 §3 |
| secret.key | instance's persistent/ | T1 | only when asked | by design (D194) |
| setup-code file, sockets, the server's lock | instance's run/ | transient | no | — |
| backups (tar.gz), pre-restore and pre-migrate snapshots | instance's backups/ and persistent/ | T1 copies | — | same disk (doctor says so); scheduled backups are M4's (D243) |
| art/<source sha256>/<size>.webp and .jpeg | instance's cache/ | T3 (node-encoded); the provider source behind it is T2 | no | ⚑ provider images are not kept or backed up. Accepted by artwork.md ("deleting cache/ costs time") |
| prepared/<file_id>/<profile>/<run_id>/ | node instance's cache/prepared/ | T3 | no | correct; the core re-adopts it (above) |
| TLS/ACME keys | — | T1/T2 | — | M4 (D243); not built yet |

## 3. Seams

**Core ↔ node.** This seam is strong and enforced. depguard has `core-boundary`, `node-boundary`,
`nodeproto-is-types` and `exec-only-in-execx` in `.golangci.yml`. The archtests are
CoreNeverImportsNode, NodeNeverImportsCore, CoreNeverExecs and NodeprotoHasNoLogic. The one
crossing, the node role in cmd/orpheum importing `internal/core/instancedir` for `ListenUnix`
(cmd/orpheum/node.go:20, :228), was closed on 2026-09-30: listening moved to
`internal/platform/unixsocket`, and the depguard rule `node-role-boundary` now holds
cmd/orpheum/node.go to the node's boundary (F8).

**Store.** `store-is-below-core` and `sql-only-in-store` keep SQL in one place. Every core feature
package imports `sqlcgen` (api, artwork, auth, identify, jobs, library, loudness, nodes, playback,
renditions, secrets, settings, subtitles), so any package could write any table, and two tables had two writers (§1). Since
2026-09-30 each table has one writing package, named in schema.md, and
`TestArch_EachTableHasOneWritingPackage` holds it (F5). api imports `sqlcgen` only for a row type
(browse.go:261), which leaks the store's shape into R9.

**Core packages that use one another** (2026-10-04; a package's doc.go says why). library uses
nodes and jobs (its scans are jobs on nodes); artwork, loudness, subtitles and renditions each use
library (the playable file, the unavailable root), jobs and nodes; providers/tmdb uses library,
identify and artwork (it is their fetcher); playback uses auth, renditions and subtitles. None of
these crosses the core–node seam, and `TestArch_EachTableHasOneWritingPackage` holds each table
to one writer whichever package calls it.

**Fetching (R1).** Clean. All outbound HTTP goes through the one client `outbound.NewClient`
makes (forbidigo `[outbound]`), built in cmd/orpheum/metadata.go. The metadata and subtitle
providers and Cloudflare's DNS (providers/cloudflare/client.go) read through `Exchange`; the
failure policy is unified since 2026-09-30: each such host is a provider service
(internal/core/providers) owning its health, rate, quota and credentials, and a host that is down
parks its destination's work in the queue (D231 §5, §6). Two callers make their own requests on
that client, by design (M4's audit): ACME, whose library (`golang.org/x/crypto/acme`) speaks its
protocol and whose failures the certificate job retries on its back-off; and the reachability
check, which reads only an echo's status and names the hop that failed (D243 §7).

**Transforming (R3).** FFmpeg work happens only on nodes (execx and confine). The core does three
cheap derivations: textindex folding and CLDR keys; artwork cutting JPEG from its own WebP (the
`core-no-image` exception, per D212 §5); and levelling-gain arithmetic (decide/levels.go, since
2026-10-03). All three agree with D193 §1.

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

**Sensing vs deciding at the client.** `POST /playback` took `network: local|remote` from the
client, and the web client hard-coded `"local"`. D247 §11 (2026-10-05) removed it and the remote
bit-rate ceiling with it: nothing knows where a client is, and a slow link shows itself by its
stalls.

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

**F3. A responsibility with four owners: the node-command wait loop.** *No owner needed; built
2026-10-01 as `nodes.Exchanges` (§1).*
The same send/await/timeout/cancel `select` appears in four places (§1). Recommend one helper in
core/nodes, for example `Ask(ctx, nodeID, command, jobID, patience) (Answer, error)`, used by all
four. This is rule 11 work with a revert-red test through one caller.

**F4. A responsibility with scattered owners: scheduling and retry.** *Decided by D231; built in
the jobs rebuild (2026-09-30 to 2026-10-03): six sweeps registered with the queue, artwork's
breaker gone, a host's failure read in its provider service. Four loops are left in cmd/orpheum
(§1), and `derivedWorkSweepEvery` is still a constant.*
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

**F7. A responsibility with two owners: doctor.** *No owner needed; fixed 2026-10-04: one
`api.Verdict`.*
There are two finding types with two verdict spellings ("FAIL" and "fail"). Recommend one
`Finding` type and one verdict enum, used by the command and by `GET /doctor`. This is rule 11
work.

**F8. A seam crossed: the node role imports a core package.** *Fixed 2026-09-30.*
cmd/orpheum/node.go:228 called `instancedir.ListenUnix`. `ListenUnix` moved to
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
sort_key, both FTS tables and search_alternates are in persistent.db (a node's status was too, until
D303's step 0049 kept it in memory alone). The
bloat is small at Cut A's scale, and startup re-derives the sort and search keys anyway.
schema.md's conventions already accept backing persistent.db up whole: at the reference scale it is
46 MB and 177 ms, so splitting it by table bought nothing. Leaving the derived tables out needs
no schema change, but it is not cheap: `VACUUM INTO` copies the whole file, so emptying them means
a second pass over the copy, and a restore would then have to rebuild the FTS indexes, which the
start-up re-keying does not do when no name's fold changed.

**F13. Client-declared network locality.** *Decided 2026-09-30; joins M4's questions.* Answered
by D247 §11 (2026-10-05): neither; nothing senses locality.
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
