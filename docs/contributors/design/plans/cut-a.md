> **ABOUTME:** The build contract for Cut A: what runs where, the byte path, the repository
> layout, every API endpoint and message, the error and job models, how *prepare* works, and six
> milestones with acceptance tests. Start here to build; the other plans are the reasons.

# Cut A — the build contract

- **Status:** PLANNED — 2026-09-23 ([D193](../../decisions/working-notes.md) §8). **Cut A's
  design is frozen from this document on**: a new decision is made only when code or a
  measurement forces one.
- **Depends on:** [schema.md](schema.md) (the tables), [migrations.md](migrations.md),
  [testing-strategy.md](testing-strategy.md) and [device-lab.md](device-lab.md) (what "passes"
  means).
- **Scope:** [D178](../../decisions/working-notes.md) — the owner at home and one person remote,
  watching together with subtitles — as narrowed by
  [D180](../../decisions/working-notes.md), [D191](../../decisions/working-notes.md),
  [D192](../../decisions/working-notes.md) and [D193](../../decisions/working-notes.md).

## How to use this document

**It says *what*, in enough detail to write code and tests. The *why* lives elsewhere.** Each
section names the plan sections that govern it. When a plan and this document disagree, this
document wins for Cut A and the disagreement is a bug to report, not a choice to make. Anything
this document does not mention is out of Cut A. A plan's material for later phases is read as
background, never as a to-do list.

## 1. What is in Cut A

| Area | In | Governing plans |
| --- | --- | --- |
| Library | scan, identify (TMDB, NFO read), ambiguous-match chooser, manual fixes, the four levels, versions, extras, **what the scanner saw** | [scanner.md](scanner.md), [identification.md](identification.md), [library-model.md](library-model.md) |
| Finding and resuming | browse, search, detail pages, artwork, play state, Continue Watching (with hide and undo), Next Up | [play-state.md](play-state.md), [artwork.md](artwork.md), [web-client.md](web-client.md) |
| Playback | direct play, and **prepare** (§6) for everything else | [stream-service.md](stream-service.md) |
| Subtitles | embedded and sidecar text tracks via JASSUB; sync offset; **fetch on request** from OpenSubtitles, labelled unverified; on by default as a preference | [subtitle-acquisition.md](subtitle-acquisition.md) |
| Quiet dialogue | dialogue boost on or off, as a prepared audio rendition; the device declares its speakers | [audio-presentation.md](audio-presentation.md) |
| People | local accounts, sessions, invites from outside | [auth.md](auth.md) |
| Together | Watch Together for two with voice; reachability; one public HTTPS name; the cast receiver | [watch-together.md](watch-together.md), [remote-access.md](remote-access.md) |
| Running it | backup and restore, now playing, the job view, `doctor` | [backup-restore.md](backup-restore.md), [observability.md](observability.md) |

**Out:**
- roles beyond `admin` and `user`, and parental controls;
- device pairing (D191);
- the TV client, and offline download;
- trickplay, segment detection, loudness measurement and subtitle verification;
- bitmap subtitles, NFO write, and the rules engine's authoring surfaces;
- ceilings, OIDC/LDAP, play statistics and the Jellyfin adapter;
- WASM plugins, our own relay role, and live transcoding;
- any second node.

## 2. What runs where

```
 owner's browser/phone ─┐                   ┌─ remote TV: browser, or our cast receiver
 remote person's phone ─┤  https://media.<domain>  (one name, DNS-01 certificate)
                        ▼                   ▼
          port-forward 443 (+ TURN)  │  or IPv6  │  or a standard tunnel to a VPS
                        │
┌───────────────────────▼───────────────────────────────────────────────────┐
│ orpheum run — one Go binary, one systemd unit, in a Debian LXC on nas     │
│                                                                           │
│  core   HTTPS listener · REST API · Watch Together hub (WebSocket)        │
│         · embedded TURN (pion/turn) · sessions/grants · identification    │
│         · job queue · backup · serves the embedded web client and the     │
│         cast receiver · relays media bytes from the node                  │
│    │  ▲                                                                   │
│    │  │ node dials core: reports (JSONL) + command long-poll   (D127)     │
│    ▼  │ core reads bytes from node's read-only socket          (D193 §1)  │
│  node   child process · reads the media (read-only bind mount)            │
│         · probe, payload hash, subtitle extraction · prepare jobs         │
│         · every FFmpeg run confined per invocation (§7)                   │
└───────────────────────────────────────────────────────────────────────────┘
   /media (ro)      /var/lib/orpheum (library.db — backed up)      /var/cache/orpheum (cache.db, prepared/, art/)
```

- **One binary, two processes.** `orpheum run` starts the core, and the core supervises the node as
  a child ([D43](../../decisions/working-notes.md), [D44](../../decisions/working-notes.md)).
  The systemd unit runs `orpheum run`.
- **Two sockets, each in one direction, and each for one purpose:**
  - **The node connects to the core** on `/run/orpheum/core.sock`. Over it the node posts reports and
    long-polls for commands, so commands always travel on a connection the node opened
    ([D107](../../decisions/working-notes.md)).
  - **The core reads bytes from the node** on `/run/orpheum/node.sock`. This socket serves `GET`
    only: file ranges and prepared-output files, no commands.
  - **Both sockets are mode 0600, owned by the service user.** The permissions are the
    authentication; there is no enrolment credential in Cut A (schema.md, `node`).
- **Where it runs:**
  - **Development:** on devnas, with a copied corpus. It has Landlock and Quick Sync.
  - **Deployment:** a Debian LXC on nas, beside the library, with `/dev/dri` passed through for
    Quick Sync. Until nas's kernel is booted with Landlock
    ([D193](../../decisions/working-notes.md) §7), the node reports itself *unconfined*.
- **Reachability**, in this order: a port-forward of 443 plus the TURN port; IPv6; a standard
  tunnel (rathole, frp or WireGuard) to a VPS. Whichever works is recorded in the admin view's
  reachability check ([D152](../../decisions/working-notes.md)).
- **The name.** `media.<domain>` on one of the owner's domains, with a Let's Encrypt
  certificate through DNS-01. Split-horizon DNS or router hairpin makes the name work inside the
  house too. The cast receiver is served from `https://media.<domain>/cast/` and registered,
  unpublished, to the remote person's device ([D192](../../decisions/working-notes.md)).

## 2a. The development loop

`make dev` runs the app for debugging, with opinionated settings that it prints and that live in
committed files, not in code ([D197](../../decisions/working-notes.md)). `make help` lists the
other debugging targets (`dev-server`, `dev-lan`, `dev-reset`, `dev-db`, `dev-logs`,
`dev-debug`, `pprof`, `probe`, `test-one`, `cover`). **What the server must implement for the
loop to work:**

- **`orpheum run --config <file>`**, reading TOML with exactly the keys in
  [`dev/orpheum.dev.toml`](../../../../dev/orpheum.dev.toml); nothing defaults in code
  ([D200](../../decisions/working-notes.md), [D202](../../decisions/working-notes.md)). Paths arrive absolute (the loop renders them).
  - `[server]`: `listen`, `data_dir`, `cache_dir`, `read_header_timeout_s`, `shutdown_grace_s`
  - `[database]`: `readers_minimum`, `migrate_space_factor` (D201)
  - `[node]`: `roots`, `ffmpeg_dir`, and `confinement`. `confinement` is `"require"` or
    `"report"`; a production install writes `"require"`.
  - `[log]`: `level`, `format` (`text` or `json`)

  A missing key with a starting value runs on it, and doctor lists it; one with none refuses
  start (D202).

  This is the D182 §3 file: only what must exist before the database opens. Unknown keys are
  reported and ignored, never fatal ([packaging-history.md](../research/packaging-history.md)
  lesson 11). pprof and `/metrics` are the `debug_pprof` and `debug_metrics` settings, served on
  the listen address only when it is loopback; every opinionated value is a setting (D200).
- **`orpheum dev seed --config <file> --seed <file>`, in `-tags dev` builds only** (M1 for the
  users and settings, M2 for the libraries). It applies [`dev/seed.toml`](../../../../dev/seed.toml):
  - the two users with their public dev passwords;
  - the two libraries, with D184's three questions answered;
  - the server settings;
  - `ORPHEUM_DEV_TMDB_KEY` and `ORPHEUM_DEV_OPENSUBTITLES_USERNAME`/`_PASSWORD`/`_API_KEY`,
    sealed (D194) when set.

  A release binary has no `dev` subcommand; `TestRun_DevSubcommandExistsOnlyInDevBuilds`
  guards that.
- **The dev library** (`make dev-media`, [`scripts/dev_media.py`](../../../../scripts/dev_media.py))
  is the M2–M5 manual test bed. It holds one synthetic file per Cut A path: direct play, remux,
  TrueHD, HEVC, two versions and a trailer, a three-episode series, an AppleDouble file, a text
  file named `.mkv`, and an unidentifiable file. Their NFO files make identification work with
  no TMDB key. The centre channel is a pulsing tone, so the dialogue boost can be heard and
  measured.

## 3. Repository layout

One private GitHub repository ([D192](../../decisions/working-notes.md)), one Go module, one npm
workspace. Every directory boundary is also a licence boundary or a trust boundary, because
linters can see paths and nothing else.

```
LICENSE (GPL-3.0) · CONTRIBUTING.md · AGENTS.md (≤150 lines) · CLAUDE.md (imports AGENTS.md)
Makefile            # make check (fast gate) · make test-all · make generate
go.mod              # tool directives pin sqlc, golangci-lint, huma's generator
cmd/orpheum/        # main: flags → run | core | node | migrate | backup | restore | doctor | setup-code
internal/core/      # GPL. May not import os/exec, internal/node, or any media/image package
  api/              # huma handlers; the OpenAPI document is generated from here
  auth/  secrets/  library/  identify/  decide/  playback/  together/  turn/  jobs/  backup/  settings/
  datadir/          # the only core package that touches the filesystem
internal/node/      # GPL. Imports nothing from core
  scan/  probe/  hash/  prepare/  byteserver/  execx/ (the only exec.Command)  confine/
internal/nodeproto/ # wire types between core and node; no logic
internal/store/     # the only importer of database/sql and the driver
  migrations/  queries/  sqlcgen/ (generated, committed)
internal/platform/  # clock, slogx: the sanctioned wrappers the linters point at
api/openapi.json    # generated, committed, diffed (D85)
web/                # MPL-2.0 (web/LICENSE)
  packages/sdk/     # React-free: generated types, probe-and-pick, the six methods, WT client
  packages/player/  # hls.js + JASSUB + the subtitle layer
  apps/web/  apps/cast-receiver/
testdata/  assumptions/  tools/gates/  .claude/{settings.json,hooks/,agents/,rules/}
```

The linting, hooks and gates for this tree are listed in the second audit
([audit-2026-09-23.md](../research/audit-2026-09-23.md) §7) and become `standards/` in the
standards round ([D192](../../decisions/working-notes.md) §3). **They land in M0, before any
feature code.**

## 4. The node protocol

Every message is versioned JSON; reports are JSONL. Reports follow
[scanner.md](scanner.md) and [D39](../../decisions/working-notes.md)–[D42](../../decisions/working-notes.md),
with the file channel deleted.

**Node → core** (`/run/orpheum/core.sock`):

| Request | Body |
| --- | --- |
| `POST /node/v1/hello` | node id, version, FFmpeg version and hash, capabilities (`remux`, `aac`, `h264_qsv`, `h264_nvenc`, `x264`), confinement report per mechanism with errno and remedy ([D112](../../decisions/working-notes.md)) |
| `POST /node/v1/report` | a JSONL stream, one record per line, each with a node sequence number. Record types: `scan_begin{root}`, `observed{path, size, mtime, payload_hash, payload_len, probe}`, `gone{path}`, `skip{path, rule}`, `error{path, scope, code, detail}`, `scan_complete{root}`, `job{job_id, state, progress, error}`, `status{…}` |
| `GET /node/v1/commands?after=<seq>` | long-poll; returns the next commands |

**Commands** (core → node, delivered by the long-poll):
- `scan{root, priority_paths[]}`
- `prepare{job_id, path, payload_hash, profile, out_dir}`
- `extract_subtitle{job_id, path, stream_index, format}`
- `cancel{job_id}`

**A command names its target by path *and* payload hash**, and the node refuses it if they no
longer match ([D100](../../decisions/working-notes.md)).

**Core reads from the node** (`/run/orpheum/node.sock`, `GET` only):
- `GET /bytes/v1/file?path=…&hash=…`, honouring `Range`;
- `GET /bytes/v1/prepared/<job_id>/<name>`.

**Liveness:** `status` records carry the node's clock and what it is doing. The core marks the
node *silent* after 30 s with no record, and restarts the child after 120 s. A node that has
failed *says so* through `error` records; silence is only ever read as silence
([D34](../../decisions/working-notes.md), simplified by D193).

## 5. The client API

REST under `/api/v1`, written as huma handlers. The OpenAPI document is generated from them,
committed, and diffed in CI with `oasdiff` ([D85](../../decisions/working-notes.md)). The TS SDK
is generated with `openapi-typescript` + `openapi-fetch`. Credentials travel in a header or a
cookie, **never in a URL** ([D154](../../decisions/working-notes.md)). Media URLs carry an opaque
single-playback token that the core maps to a grant.

| Group | Endpoints |
| --- | --- |
| Setup and auth | `POST /setup` (one-time setup code from the log → first admin); `POST /session`, `DELETE /session`; `GET /me`; `POST /invites` (admin: role, asked); `POST /invites/{code}/redeem` |
| Settings | `GET/PUT /settings` (server scope, admin; what the settings page shows); `GET/PUT /config` (`?modified=true`; with the config file's keys, read-only), `POST /config/{key}/reset`, `GET /config/unused`, `DELETE /config/unused/{key}` (every server setting beside its starting value, admin; D200, D202); `GET/PUT /me/settings`; `GET /secrets`, `PUT /secrets/{name}`, `DELETE /secrets/{name}` (server scope for admins, `/me/secrets/…` for a user's own; write-only: `GET` shows `set`, `hint` and `needs_reentry`, never a value — [D194](../../decisions/working-notes.md)); `PUT /devices/{id}` (name, `audio_output`, asked on first playback — D184) |
| Libraries | `POST /libraries` (the three questions, D184); `GET /libraries`; `POST /libraries/{id}/scan`; `GET /files?filing=&library=&cursor=` (what the scanner saw, D163) |
| Identification | `GET /titles?ident=ambiguous`; `GET /titles/{id}/candidates`; `POST /titles/{id}/match`; `PUT /items/{id}/overrides/{field}`; `DELETE /items/{id}/overrides/{field}` |
| Browse | `GET /home` (Continue Watching, Next Up, recently added); `GET /titles?kind=&sort=&cursor=`; `GET /collections/{id}` (a series); `GET /items/{id}` (detail with editions, versions, files, streams, extras); `GET /search?q=`; `GET /art/{hash}/{size}` |
| Play state | `POST /playback/{id}/report` (state changes and spans with the device's sequence, D81/D165); `POST /items/{id}/hide`, `DELETE /items/{id}/hide` (undo); `PUT /files/{id}/prefs` (sync offset, track choices, boost) |
| Playback | `POST /playback` → the decision (§6); `GET /media/direct/{token}` (range, relayed); `GET /media/hls/{token}/{name}` (playlists, init, segments, relayed); `GET /media/sub/{token}/{stream}.{ass,vtt}`; `POST /files/{id}/prepare` ("Get this ready") |
| Subtitles | `GET /files/{id}/subtitle-candidates?lang=` (unverified in Cut A); `POST /files/{id}/subtitles` (fetch one; spends one of the user's daily downloads) |
| Watch Together | `POST /together` (host: item, abilities → session and link); `POST /together/join` (link → membership credential); `GET /together/ws` (WebSocket, §5.1); `GET /together/{id}/ice` (short-lived TURN credentials); `POST /together/{id}/rotate`, `POST /together/{id}/eject/{member}` (host) |
| Running it | `GET /now-playing`, `POST /now-playing/{id}/stop` (with a message); `GET /jobs`, `POST /jobs/{id}/cancel`; `GET /backups`, `POST /backups`; `GET /doctor`; `GET /health` (unauthenticated: `starting`, `migrating`, `ready`, or `degraded` with reasons) |

Restore is a command-line operation with the server stopped (`orpheum restore`), never an endpoint.

### 5.1 The Watch Together messages

One WebSocket per device. Every message has `v`, `type` and, from the server, `seq`. The group
state and its rules are [watch-together.md](watch-together.md)'s; this is their wire form. There
is **no clock synchronisation** ([D66](../../decisions/working-notes.md)): positions are sent with
the sender's monotonic time, and round trips are measured with `ping`.

| Direction | `type` | Fields |
| --- | --- | --- |
| client → | `hello` | membership credential, device role (`playback`, `voice`, `both`), `can_set_rate` |
| client → | `ping` | `t_mono` |
| client → | `state` | `position_ms`, `t_mono`, `playing`, `buffering` — sent on every change and every 60 s |
| client → | `command` | `play`, `pause`, `seek{position_ms}`, `rate{r}`, `change_item{file_id}` (host), with `based_on_seq` |
| client → | `suspend` / `resume` | `reason`: `stallRecovery`, `audioSessionInterrupted`, `userActionRequired`, `userPausedLocally`, `userIsChangingCurrentTime` |
| client → | `rtc` | `to`, `sdp` or `candidate` (voice signalling only; media goes browser to TURN) |
| → client | `welcome` | session, roster, abilities, current `group` |
| → client | `pong` | the echoed `t_mono` |
| → client | `group` | `seq`, `state` (`Idle`, `Waiting`, `Paused`, `Playing`), `position_ms`, `rate`, `item`, and the member whose command caused it (named events, D188) |
| → client | `roster` | members, their devices, per-member health and suspensions |
| → client | `rejected` | `based_on_seq`, `code` (§5.2) |
| → client | `rtc` | relayed signalling |
| → client | `ended` | reason |

**The server applies commands in arrival order, and each `group` it sends carries a new
`seq`.** A command that names a stale `based_on_seq` is still applied if it is not a conflict.
If it is (a seek against a seek), the latest wins and the loser receives `rejected`. Drift is
corrected by the client: nudge the rate if it can, otherwise seek. The thresholds are in the
plan and are measured in the device lab.

**Recording (D191).** A membership that is signed in, on any of its devices, has its viewing
recorded as that user's spans, with the playback device reporting. The host's viewing is always
recorded.

### 5.2 The error model

One shape everywhere, one enumeration shared by the Go server and the SDK:

```json
{"error": {"code": "prepare_failed", "message": "…", "detail": {"…": "…"}, "hop": "node"}}
```

`code` is one of:
- `setup_required`, `auth_required`, `forbidden`
- `not_found`, `invalid_request`, `conflict`, `stale`
- `rate_limited`, `quota_exhausted` (OpenSubtitles), `provider_error`
- `node_silent`, `file_unavailable` (the node cannot see it; never "deleted")
- `prepare_failed`, `migrating`, `internal`

`hop`, where present, names the failing leg for reachability errors: `client`, `core`, `node`,
`provider` or `tunnel` ([remote-access.md](remote-access.md)).

**The transcode reason** travels in the playback decision, not as an error. It is one of:
- `container` (MKV in a browser)
- `audio_codec`
- `video_codec`
- `bandwidth`
- `dialogue_boost`
- `user_choice`

It is shown on screen (D188).

### 5.3 Jobs

Every piece of background work is a row in `cache.db`'s `job` table ([schema.md](schema.md)):
fetches, prepares, scans and backups.
- **Scheduling.** One scheduler in the core picks the highest-priority queued job whose
  `not_before` has passed and whose runner has a free slot. The slots are: one encode per node,
  two remux or extract jobs per node, one fetch per provider under its rate limit, and one
  backup.
- **Node jobs** become commands, and their progress arrives as `job` records.
- **"Play this now"** sets the highest priority.
- **Cancelling** a job sends `cancel` to the node.
- **After a crash, `running` jobs return to `queued`.** Output is a cache, so they restart from
  zero.
- **The job view** is `GET /jobs`: kind, subject, state, progress, reason, last error.

## 6. Prepare

[D193](../../decisions/working-notes.md) §2 is the decision; this is the mechanism.

**The decision (`POST /playback`).** The request carries the file or version, the device id,
the client's prioritised menu per axis ([D21](../../decisions/working-notes.md)) and whether the
link is local or remote. The core resolves it in this order:
1. **The original is acceptable on every axis** (container, video, audio, and the boost is off
   or the device is not on stereo speakers): `{kind: "direct", url}`.
2. **A complete or running prepared rendition matches:** `{kind: "hls", url, reason}`.
3. **Otherwise:** queue `prepare` at the highest priority and return `{kind: "hls", url, reason,
   preparing: true}`. The playlist URL answers `425` until the first segment exists; the SDK
   waits and shows *"Preparing — ready in a few seconds"*.

The response also lists the subtitle tracks, each with an `.ass` and a `.vtt` URL, and the
audio renditions.

**The profiles:**

| Profile | Chosen when | Video | Audio renditions |
| --- | --- | --- | --- |
| `remux` | the video codec is in the client's list | `-c:v copy` | `aac-stereo` (standard downmix) and `aac-stereo-boost` (centre raised) |
| `h264-8m` | the video codec is not in the list, or the link is remote and the source is above ~10 Mbps | `h264_qsv`, else `h264_nvenc`, else `libx264`; ~8 Mbps, 2 s GOP | the same two |

**One FFmpeg run per job** (a sketch; M3 verifies the exact arguments):

```
ffmpeg -nostdin -hide_banner -protocol_whitelist file -i <source>
  -map 0:v:0 -c:v copy | -c:v h264_qsv -b:v 8M -maxrate 10M -bufsize 16M -g <2 s>
  -map 0:a:<chosen> -c:a aac -ac 2 -b:a 192k                                  # standard
  -map 0:a:<chosen> -c:a aac -b:a 192k -af "pan=stereo|FL=<centre-boost matrix>|FR=<…>"
  -f hls -hls_segment_type fmp4 -hls_time 6 -hls_playlist_type event
  -master_pl_name master.m3u8 -var_stream_map "v:0,agroup:a a:0,agroup:a,default:yes a:1,agroup:a"
  <out_dir>/v%v/stream.m3u8
```

- **Text subtitles** are extracted by `extract_subtitle` jobs queued alongside, one per track.
  The output is ASS for JASSUB and WebVTT for the cast receiver, served as sidecar files rather
  than as HLS renditions in Cut A.
- **The boost matrix** is one fixed matrix whose centre gain is set in M3 by listening test on a
  television, and recorded as an `assumptions/` test.
- **Where the output lives:** `/var/cache/orpheum/prepared/<file_id>/<profile>/`.
- **The `prepared_rendition` row** is written when the job starts and marked complete when it
  ends. Play state records against the source file
  ([D180](../../decisions/working-notes.md)).
- **Eviction** is least-recently-played beyond a size budget set in settings. A rendition that a
  live session is playing is never evicted.

## 7. Confinement

Every FFmpeg or ffprobe run goes through `node/execx`, which re-executes `orpheum` as a shim. The
shim:
1. applies the seccomp **denylist**: no inet sockets for probe and extract; no `ptrace`,
   `mount`, `bpf`, `keyctl`, `unshare` or `setns`;
2. applies Landlock where available: read access to the one source file, and read-write to the
   one output directory;
3. then `execve`s the bundled FFmpeg with the hardened arguments.

- **What the node reports.** It reports per mechanism what it got. When Landlock is missing, the
  report translates the errno:
  - `EOPNOTSUPP`: *"enable Landlock at boot: add `lsm=…` to the kernel command line"*;
  - `ENOSYS`: *"kernel too old"*;
  - `EPERM`: *"the container runtime blocks it"*.
- **The escape test** ([D112](../../decisions/working-notes.md), [D190](../../decisions/working-notes.md))
  runs on devnas in CI and in the lab.
- **Other platforms.** On macOS and Windows builds, the shim compiles to a no-op that reports
  *unconfined*.

## 8. Milestones

Each milestone ends with its acceptance tests green in CI or in the device lab, and each test
must first be seen to fail ([D190](../../decisions/working-notes.md)).

| | Contents | Accepted when |
| --- | --- | --- |
| **M0 — skeleton and gates** *(done 2026-09-23, [D196](../../decisions/working-notes.md); CI runs on the first push)* | the layout of §3, the licence files, `make check`, CI on the private repo, the Stop and pre-edit hooks, the dependency allowlist, the first `TestArch_` tests; the standards round's output ([D192](../../decisions/working-notes.md) §3) | CI is green on an empty server. Each of these turns it red: a SQLite import outside `store`; an `exec.Command` in core; an import of `internal/node` from core; editing a lint config through the agent. |
| **M1 — schema, migrations, backup** | the config file and `orpheum dev seed` for users and settings (§2a); [schema.md](schema.md) as migration 0001 through the D181 runner and sqlc; the `setting` registry; sealed secrets and `secret.key`; argon2id passwords; `VACUUM INTO` backup; `orpheum restore`; the setup code and the first admin | A fresh schema equals the golden. Killing the process mid-migration and restarting resumes. A database whose `compatible_version` is newer is refused, and the refusal names the snapshot. A backup restored into an empty data directory reproduces every authored table. A file without the `application_id` is refused. `/health` says `migrating` before the migration starts. **A canary secret appears in no API response, log, `doctor` output, bug report or backup except as ciphertext.** A restore without `secret.key` marks secrets *needs re-entering* and loses nothing else. argon2id takes ≤ 500 ms on devnas. |
| **M2 — scan and identify** | node child and both sockets; enumerate, probe, payload hash; JSONL reports and ingest; the filename parser; the job queue with fetches; TMDB; NFO read; the ambiguous chooser; overrides; the scanner view; artwork; browse and search | On the tier-1 corpus plus a 200-file fixture tree, every file has a filing and a reason. D179's table holds as tests: a `mkvpropedit` title edit keeps identity, and an `ffmpeg -c copy` remux gets a new one. No ambiguous title is auto-matched. An override survives re-derivation. With the media root at mode 000 to the core's user, the core still works: it never opens a media path. |
| **M3 — play in a browser** | the playback decision; direct play relayed; **prepare** with both profiles; subtitles via JASSUB and the offset; the dialogue boost; play state, Continue Watching with hide and undo; accounts; the web client usable on a phone; the job view; confinement and its report | An MKV with H.264, TrueHD and ASS plays in Chrome, Firefox and iPhone Safari within 8 s of pressing play, with the reason on screen. The boost measurably raises the centre channel's share of the stereo output. Two devices reporting out of order never rewind the position. `mediastreamvalidator` passes on a prepared rendition. The escape test is green on devnas. |
| **M4 — two people, one remote** | the HTTPS name; reachability; invites; Watch Together over §5.1; voice through embedded TURN; recording for signed-in members; now playing | On D190's remote-household VM under its netem profiles, two Playwright clients stay within the drift bound for 30 minutes. Fake-microphone voice arrives within the jitter and loss bounds. An invite works once. Revocation ends access at the next request. No credential appears in any URL in a HAR capture. A signed-in guest's history advances; an anonymous one's does not. |
| **M5 — the remote TV** | the first working path from [D188](../../decisions/working-notes.md)'s table for the remote person's actual TV; "Get this ready" and the session-start prepare of `h264-8m` | On that TV class, with the film on the TV and voice on the phone, a whole film completes with no manual resynchronisation. A prepared rendition plays without stalling over the "congested" profile. Resuming at home continues from the remote position. Deleting `cache.db` loses nothing but prepared renditions. |

**Cut A is done** after M5 and a week of real use by its two users. The first public beta
([D192](../../decisions/working-notes.md)) is a separate decision.

## 9. Before M0: what only the owner can do

- **Buy a Chromecast.** Phase 0 ([playback-survey.md](playback-survey.md)) needs one, and M5
  may too. Buy the same model as the remote person's if they have one.
- **Test the ISP:** whether a forwarded port is reachable from mobile data, and whether IPv6 is
  present.
- **Choose which domain** carries `media.<domain>`, and give its DNS provider's API access to the
  certificate client.
- **Enable Landlock on nas's kernel** before M3's escape test runs there, or accept *unconfined*.
- **Find out the remote person's TV setup**, which is due in about two weeks and needed by M5,
  not before.
