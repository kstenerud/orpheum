> **ABOUTME:** The build contract for Cut A: what runs where, the byte path, the repository
> layout, every API endpoint and message, the error and job models, how *prepare* works, and six
> milestones with acceptance tests. Start here to build; the other plans are the reasons.

# Cut A — the build contract

- **Status:** PLANNED — 2026-09-23 ([D193](../../decisions/working-notes.md) §8). **Cut A's
  design is frozen from this document on**: a new decision is made only when code or a
  measurement forces one.
- **The record of intent** (the owner, 2026-09-30: "agreed", [M3's closing audit](../research/audit-2026-09-29.md) §9 D): this document
  stays what was planned. Its layout, key lists, endpoint and message lists and counts are not
  kept in step with the code; where the code differs,
  [P36](../../plan-deviations.md#p36--the-code-outgrew-cut-as-layout-lists-and-counts-and-the-frozen-text-still-shows-the-old-ones-m3-2026-09-29--plan-gap)
  lists it. Names D227 renamed are corrected in place.
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
| Subtitles | embedded and sidecar text tracks via JASSUB; sync offset; **fetch on request** from the play screen, from OpenSubtitles, labelled unverified; off until a person chooses a track (D223) | [subtitle-acquisition.md](subtitle-acquisition.md) |
| Quiet dialogue, quiet films | dialogue boost on or off, as a prepared audio rendition; the device declares its speakers; **loudness levelled**: each sound measured once by the node and brought towards one level by the player, on until a person turns it off (D226) | [audio-presentation.md](audio-presentation.md) |
| People | local accounts, sessions, invites from outside | [auth.md](auth.md) |
| Together | Watch Together for two with voice, the remote person's voice on her phone's browser while her computer's browser plays; reachability; one public HTTPS name. The cast receiver and the TV come right after Cut A (D245) | [watch-together.md](watch-together.md), [remote-access.md](remote-access.md) |
| Interface | one polished, skinned web UI, from research and a UI principles document (D245) | [web-client.md](web-client.md) |
| Running it | backup and restore, now playing, the job view, `doctor` | [backup-restore.md](backup-restore.md), [observability.md](observability.md) |

**Out:**
- parental controls (roles beyond `admin` and `user`, which this line once left out, are in: the
  admin makes them, D243 §4);
- device pairing (D191);
- the TV client, and offline download;
- trickplay, segment detection and subtitle verification;
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
│ orpheum core + orpheum node — one image, two services, on nas (D235)      │
│                                                                           │
│  core   HTTPS listener · REST API · Watch Together hub (WebSocket)        │
│         · embedded TURN (pion/turn) · sessions/grants · identification    │
│         · job queue · backup · serves the embedded web client and the     │
│         cast receiver · relays media bytes from the node                  │
│    │  ▲                                                                   │
│    │  │ node dials core: status, results + command long-poll   (D234)     │
│    ▼  │ core reads bytes from node's read-only socket          (D193 §1)  │
│  node   its own service · reads the media (read-only bind mount)          │
│         · probe, payload hash, subtitle extraction · prepare jobs         │
│         · every FFmpeg run confined per invocation (§7)                   │
└───────────────────────────────────────────────────────────────────────────┘
   /media (ro)      core instance: persistent/ (backed up) · cache/ · backups/ · run/
                    node instance: persistent/ (its config) · cache/prepared/ · run/      (D242)
```

- **One binary, two processes.** `orpheum run` starts the core, and the core supervises the node as
  a child ([D43](../../decisions/working-notes.md), [D44](../../decisions/working-notes.md)).
  The systemd unit runs `orpheum run`. *Revised by D233 §2 and D235 §4, built in the jobs rebuild's
  Phase 4a (2026-10-02): `orpheum core` and `orpheum node` are two services of one image, which the
  runtime starts and restarts; `run` and the core's supervisor are gone, and the dev loop runs the
  two as two processes.*
- **Two sockets, each in one direction, and each for one purpose:**
  - **The node connects to the core** on `core.sock` in the core instance's `run/`, which the
    node's `node.core_socket` names (D242 §7). Over it the node posts reports and long-polls for
    commands, so commands always travel on a connection the node opened
    ([D107](../../decisions/working-notes.md)).
  - **The core reads bytes from the node** on `node.sock` in the node instance's `run/`, which
    the node's hello names. This socket serves `GET` only: file ranges and prepared-output files,
    no commands.
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

- **`orpheum core --instance <dir>` and `orpheum node --instance <dir>`** (since D242, built
  2026-10-03; before it `orpheum run --config <file>`, and from Phase 4a `--config` on each). An
  instance is one directory: its top-level `orpheum.toml` says its kind and where its
  `persistent/`, `cache/`, `backups/` (a core's) and `run/` are, and the full config,
  `persistent/config.toml`, holds the keys of its kind, those of
  [`deploy/dev/core.dev.toml`](../../../../deploy/dev/core.dev.toml) and
  [`deploy/dev/node.dev.toml`](../../../../deploy/dev/node.dev.toml); nothing defaults in code
  ([D200](../../decisions/working-notes.md), [D202](../../decisions/working-notes.md)).
  `orpheum default-config [--node] [--run-dir <dir>] <dir>` makes one, and refuses one whose
  socket's path would be past a Unix socket's 103 bytes: `--run-dir` puts its run directory
  somewhere shorter, as `/run/orpheum`. Paths arrive absolute.
  - a core's `[server]`: `listen`, `read_header_timeout_seconds`, `shutdown_grace_seconds`
  - a core's `[https]`: `domain` (its public name; empty serves no HTTPS), `contact` (the e-mail
    address its CA writes to) and `listen` (`":443"`); the certificate comes by ACME DNS-01 through
    the DNS provider's token, a secret (D243 §6)
  - a core's `[database]`: `read_connections_minimum`, `migrate_free_space_db_multiple` (D201)
  - a node's `[node]`: `name` (the core enrols it by it, D207), `core_socket`, `root_paths`,
    `ffmpeg_dir`, and `confinement_policy`, which is `"require"` or `"report"`; a production
    install writes `"require"`. What its machine can do: `encode_slot_count`, `remux_slot_count`,
    `slowest_read_mib_per_second` (plan-deviations P36). Its prepared renditions, in its
    instance's `cache/prepared/`, are kept within D217 §5's budget by `prepared_budget_gib` (200),
    `prepared_kept_after_play_hours` (72), `prepared_disk_used_limit_percent` (95) and
    `prepared_disk_min_free_mib`. And `max_errors_per_root`, `probe_timeout_seconds`,
    `status_interval_seconds`, `progress_every_seconds`, `ended_work_kept_hours`,
    `read_header_timeout_seconds`.
  - both's `[log]`: `level`, `format` (`text` or `json`), `destination` (`stderr` or a file)

  A missing key with a starting value runs on it, and doctor lists it; one with none refuses
  start (D202).

  This is the D182 §3 file: only what must exist before the database opens. Unknown keys are
  reported and ignored, never fatal ([packaging-history.md](../research/packaging-history.md)
  lesson 11). pprof and `/metrics` are the `should_serve_debug_pprof` and `should_serve_debug_metrics` settings, served on
  the listen address only when it is loopback; every opinionated value is a setting (D200).
- **`orpheum dev seed --instance <dir> --seed <file>`, in `-tags dev` builds only** (M1 for the
  users and settings, M2 for the libraries). It applies [`deploy/dev/seed.toml`](../../../../deploy/dev/seed.toml):
  - the two users with their public dev passwords;
  - the two libraries, with D184's three questions answered;
  - the server settings;
  - `ORPHEUM_DEV_TMDB_KEY` and `ORPHEUM_DEV_OPENSUBTITLES_USERNAME`/`_PASSWORD`, sealed
    (D194) when set; the OpenSubtitles key is the application's, in the source (D224).

  A release binary has no `dev` subcommand; `TestDispatch_DevSubcommandExistsOnlyInDevBuilds`
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
LICENSE (GPL-3.0) · CONTRIBUTING.md · AGENTS.md · CLAUDE.md (imports AGENTS.md)
Makefile            # make check (the gate) · make check-pr · make generate · make dev · make help
go.mod              # one module; sqlc and golangci-lint are pinned in the Makefile
cmd/orpheum/        # main: flags → core | node | default-config | migrate | backup | restore
                    #   | doctor | setup-code | openapi | version | dev (dev builds only)
internal/core/      # GPL. May not import os/exec, internal/node, or any media/image package
  api/              # huma handlers; the OpenAPI document is generated from here
  auth/  secrets/  settings/  preferences/  backup/  certificate/  webclient/
  library/  identify/  providers/  textindex/  artwork/  jobs/  nodes/
  decide/  playback/  loudness/  subtitles/  renditions/
  together/         # Watch Together's sessions, in memory (M4)
  turn/             # M4's; doc.go only
  instancedir/      # the only core package that touches the filesystem
internal/node/      # GPL. Imports nothing from core
  agent/  scan/  probe/  hash/  prepare/  prepared/  subtitle/  loudness/  artwork/  hardware/
  byteserver/  namedfile/  execx/ (the only exec.Command)  confine/  nodetest/
internal/nodeproto/ # wire types between core and node; no logic
internal/store/     # the only importer of database/sql and the driver
  migrations/  queries/  sqlcgen/ (generated, committed)  cache/  cachegen/  storetest/
internal/platform/  # the sanctioned wrappers and shared mechanisms: clock, slogx, jobqueue,
                    #   pathtree, unixsocket, outbound, bounded, … (each package's doc.go)
internal/archtest/  # the architecture tests (TestArch_…)
internal/testutil/
api/openapi.json    # generated, committed
web/                # MPL-2.0 (web/LICENSE)
  packages/sdk/     # React-free: generated types, probe-and-pick, the six methods, WT client
  packages/player/  # hls.js + JASSUB + the subtitle layer
  apps/web/  apps/cast-receiver/
scripts/            # the gates, in Python, with their tests
deps/               # the dependency allowlists
deploy/             # the container images (D235); ./dev runs the development one
.claude/  .githooks/
```

The linting, hooks and gates for this tree are listed in the second audit
([audit-2026-09-23.md](../research/audit-2026-09-23.md) §7) and become `standards/` in the
standards round ([D192](../../decisions/working-notes.md) §3). **They land in M0, before any
feature code.**

## 4. The node protocol

*Rewritten from `internal/nodeproto` at protocol version 21 (the pre-M4 audit, 2026-10-04). The
protocol before D234, a JSONL report stream, a `scan` command and a core that restarted its node,
is in this file's history.* Every message is JSON, and every type is in
[`internal/nodeproto`](../../../../internal/nodeproto/doc.go), which is the specification; this
section is its map. `nodeproto.ProtocolVersion` is raised with any change a node or core of
another build would misread.

**Node → core**, on the core's socket, which the node's `node.core_socket` names (D242 §7). The
node dials; the core answers.

| Request | Body and answer |
| --- | --- |
| `POST /node/v1/hello` | `Hello`: `protocolVersion` (refused with 409 when it is not the core's, and the node waits for an update; D233 §9), `name` (a node is enrolled by it, D207), `startId` (new at each start, so the core counts what a restarted node held as interrupted; D238 §3), `build`, `host`, `byteSocketPath` (where its byte socket is), `rootPaths`, `ffmpegVersion`, `ffmpegSha256`, `capabilities` (`remux`, `aac`, `h264_qsv`, `h264_nvenc`, `x264`), `confinement` per mechanism (D112, D227), `uid`, `gid`, `hardware` (GPUs, encoder trials, the chosen encoder; D225 §3), `capacity` (encode and remux slots, slowest read), `devices` (what its roots and prepared directory are on, by key and kind: `spinning`, `ssd`, `network`, `unknown`; D240 §5), `renditions` (the manifests on its disk; D238 §2). Answer, `HelloReply`: `nodeId`, `keptRenditionDirs` (the node evicts the rest), `deviceReaderLimits` |
| `GET /node/v1/commands?afterCommandSeq=<n>` | long-poll; `Commands`: the commands after the last one the node took, and `deviceReaderLimits` as the core's settings say now |
| `POST /node/v1/status` | `Status`, each tick: `sentAt`, `preparedUsage`, `disks`, `held` (each attempt it holds: `waiting`, `running`, `paused` or `ended`, and how long since it moved), `takenCommandSeq`, `stuck` (a tool blocked in the kernel, with its pid and paths) |
| `POST /node/v1/progress` | `Progress`, best effort, the newest superseding the rest: `jobId`, `attemptToken`, `state` (`waiting`, `running`, `paused`), and a prepare's `preparedThroughMs` (D234 §5) |
| `POST /node/v1/result` | `Result`, how a job ended, by `jobId` and `attemptToken`, echoing its `work`: exactly one of `prepare`, `subtitle`, `loudness`, `artwork`, `listing`, `examined`, or a `refusal` (a command it does not know; D233 §9). One too large to post says `held{sizeBytes, sha256}`, and the core fetches it from the byte socket. Acknowledged means recorded (D234 §5, D238 §2); one refused is sent again |

**Commands** (core → node, by the long-poll). Each is an envelope, `{commandSeq, jobId,
attemptToken, priority, isPlayDriven, type, <payload>}`, around one kind of work. The attempt
token names this sending of the job, so a result is taken only for the attempt that waits (D238
§2, S83, S84); `isPlayDriven` work pauses the background work reading its device (D240 §6).

- `list{rootPath, path, depth}`: a folder's entries, the first half of a scan (D241); the answer is
  a `listing` of scan records (`scan_begin`, `listed`, `mount`, `skip`, `error`, `scan_complete`)
- `examine{rootPath, files[{path, kind}]}`: probe, hash and read the sidecars of a batch; the
  answer is `examined` records (`observed`, `sidecar`, `skip`, `error`)
- `prepare{video, profile, renditionDir, audio[]}`: a rendition (§6)
- `extract_subtitle{video, streamIndex}`, `{sidecar}` or `{fetchedSubRip}`: one job makes both ASS
  and WebVTT ([D217](../../decisions/working-notes.md) §6)
- `measure_loudness{video, tracks[]}`: each audio track's integrated loudness, range and true peak,
  and its stereo mix's when it has more than two channels, in one read ([D226](../../decisions/working-notes.md) §1)
- `encode_artwork{images[]}`: images made into the sizes a person sees (D212)
- `cancel`: the envelope alone; stops the attempt it names
- `reprioritize`: the envelope alone; the attempt's new priority, and whether it is still
  play-driven (D234 §3, D240 §6)

**A command names a file by root, path *and* payload hash**, and the node refuses it if they no
longer match ([D100](../../decisions/working-notes.md)). A path is bytes, base64url on the wire
(plan-deviations P25).

**Core reads from the node**, on the byte socket its hello names, `GET` only:
- `GET /bytes/v1/file?rootPath=…&path=…&payloadHash=…`, honouring `Range`;
- `GET /bytes/v1/prepared/<file_id>/<profile>/<run_id>/<name>` (D217 §5);
- `GET /bytes/v1/result/<attemptToken>`: a result held on the node (D234 §5).

**Liveness.** A node is heard by its status, its progress and its polls. The core marks it
*silent* after `node_silent_after_seconds` with no word, and an attempt whose progress stops moving
is stalled: the node says why, `never_moved`, `stood_still` or `past_deadline`, with its
condition (`waiting_on_disk` and the file it waits on, `spinning`, `stuck`; D244 §6). Restarting a
node that hangs or exits is its service manager's, not the core's (D234 §4). A node that failed
*says so*; silence is only ever read as silence ([D34](../../decisions/working-notes.md)).

## 5. The client API

REST under `/api/v1`, written as huma handlers; `api/openapi.json` is the full list, and this
table its map (checked against it at the pre-M4 audit, 2026-10-04). A POST that carries an
`Idempotency-Key` is done once, and a repeat of it is given the first answer (D244 §4). The OpenAPI document is generated from them,
committed, and diffed in CI with `oasdiff` ([D85](../../decisions/working-notes.md)). The TS SDK
is generated with `openapi-typescript` + `openapi-fetch`. Credentials travel in a header or a
cookie, **never in a URL** ([D154](../../decisions/working-notes.md)). Media URLs carry an opaque
single-playback token that the core maps to a grant.

| Group | Endpoints |
| --- | --- |
| Setup and auth | `POST /setup` (one-time setup code from the log → first admin); `POST /session` (with the device, D217 §8), `DELETE /session`; `GET /me`; accounts ([D218](../../decisions/working-notes.md)): `GET /users`, `POST /users` (admin: name and role → the person and a registration invitation), `POST /users/{id}/registration-invitation` (admin: a new one, for a forgotten password), `POST /registration` (the link's secret, from after `#`, in the body, and a password → a session), `POST /device-codes` (a code for a device, one's own, or anyone's for an admin), `POST /session/code` (a device signs in with it), `PUT /me/password`; a person's devices, listed and revocable (D218 §4): `GET /me/devices`, `PUT /me/devices/{id}` (rename), `DELETE /me/devices/{id}`, `POST /me/devices/sign-out-elsewhere`, and for the admin `GET /users/{id}/devices`, `DELETE /users/{id}/devices/{deviceId}`; `GET /me/device-code` (whether one's code still waits, D221), `GET /users/{id}` and `PUT /users/{id}` (a person's page and their name, for the admin); roles, each a person's whole access (D243 §4): `GET /roles`, `POST /roles`, `PUT /roles/{id}` (its name, capabilities and libraries; a library left out is gone for its members at their next request), `DELETE /roles/{id}`, `PUT /users/{id}/role`; `POST /users` takes the role and `isOutsider`, an invite. An `auth_required` answer's `detail.reason` says why: `session_ended`, `session_expired` or `no_session`. The M4 invite for someone outside the household is D174's and comes with M4 |
| Settings | `GET/PUT /settings` (server scope, admin; what the settings page shows); `GET/PUT /config` (`?onlyModified=true`; with the config file's keys, read-only), `POST /config/{key}/reset`, `GET /config/unused`, `DELETE /config/unused/{key}` (every server setting beside its starting value, admin; D200, D202); `GET /client-settings` (the settings every client reads: how often a screen asks again, D237); `GET /secrets`, `PUT /secrets/{name}`, `DELETE /secrets/{name}` (admin; every secret is the server's, D224 §9; write-only: `GET` shows `isSet`, `lastChars` (`hint` until D227) and `needsReentry`, never a value — [D194](../../decisions/working-notes.md)); `PUT /me/devices/{id}` (name, `audioOutput`, asked on first playback — D184) |
| Libraries | `POST /libraries` (the three questions, D184); `GET /libraries`; `GET /nodes` (each node and the roots it configured, for a new library; P24); `POST /libraries/{id}/scan`; `GET /files?filing=&libraryId=&cursor=&limit=` (what the scanner saw, D163); `POST /libraries/{id}/removals` (approve a root's withheld removals) and `POST /libraries/{id}/mounts/gone` (a disk missing from a mount point is gone for good) — the admin's two answers D208 asks for (plan-deviations P18); what is gone or moved (D231 §7, D241): `DELETE /libraries/{id}`, `POST /libraries/{id}/roots/retire`, `POST /libraries/{id}/roots/repoint` (a root moved; a scan confirms each file), `POST /nodes/retire`, `GET /files/{id}/availability` (whether a file can be read now, and why not), `POST /files/{id}/retire`; `GET /offline-reminder`, `POST /offline-reminder/dismissal`; `PUT /nodes/device-reader-limit` (D240 §5) |
| Identification | `GET /items?identificationState=ambiguous` (titles and series; the parameter was `ident`, then `identification`, until D227); `GET /items/{id}/candidates`; `POST /items/{id}/match`; `PUT /items/{id}/overrides/{field}`; `DELETE /items/{id}/overrides/{field}`; `POST /items/{id}/refresh` (a person's refresh, whenever they like — [D213](../../decisions/working-notes.md), plan-deviations P21) — under `/items` since a series is what is identified for shows ([D209](../../decisions/working-notes.md), plan-deviations P16) |
| Browse | `GET /home` (Continue Watching, Next Up, recently added); `GET /titles?kind=&sort=&cursor=`; `GET /collections/{id}` (a series); `GET /items/{id}` (detail with editions, versions, files, streams, extras); `GET /search?query=`; `GET /bytes/art/{sourceSha256}/{variant}` (a variant is a size and a format: `portrait-m.webp`) |
| Play state | `POST /playback/{token}/report` (state changes and spans with the device's sequence, D81/D165); `POST /items/{id}/hide`, `DELETE /items/{id}/hide` (undo); `PUT /files/{id}/preferences` (sync offset, subtitle choice); `PUT /me/preferences` (the person's own: the dialogue boost, D222 §3, D223 §10; loudness levelling, D226 §5) |
| Playback | `POST /playback` → the decision (§6); `GET /bytes/media/direct/{token}` (range, relayed); `GET /bytes/media/hls/{token}/master.m3u8` and `GET /bytes/media/hls/{token}/{renditionName}/{fileName}` (playlists, init, segments, relayed); `GET /bytes/media/sub/{token}/{fileName}` (the stream's ASS or WebVTT; the parameters were `{rendition}` and `{file}` until D227) — every byte path under `/bytes/`, the media cookie's one scope ([D216](../../decisions/working-notes.md)). "Get this ready" (`POST /files/{id}/prepare`) is M5's (plan-deviations P36) |
| Subtitles | `GET /files/{id}/subtitle-candidates?language=` (unverified in Cut A); `POST /files/{id}/subtitles` (fetch one; spends one of the server's OpenSubtitles account's daily downloads, as OpenSubtitles counts them, D224 §4) |
| Watch Together | `POST /together` (host: item, abilities → session and link); `POST /together/join` (link → membership credential); `GET /together/ws` (WebSocket, §5.1); `GET /together/{id}/ice` (short-lived TURN credentials); `POST /together/{id}/rotate`, `POST /together/{id}/eject/{member}`, `PUT /together/{id}/cap`, `DELETE /together/{id}` (host) |
| Running it | `GET /now-playing`, `POST /now-playing/{id}/stop` (with a message); `GET /jobs`, `POST /jobs/{id}/cancel`; `GET /backups`, `POST /backups` (a backup of the persistent directory now, the key in it only when asked; D242 §5); `GET /doctor`; `GET /system` (the System page: the server's facts and each node's, with doctor's findings, [D225](../../decisions/working-notes.md)); `GET /health` (unauthenticated: `starting`, `migrating`, `ready`, or `degraded` with reasons) |

Restore is a command-line operation with the server stopped (`orpheum restore`), never an endpoint.

### 5.1 The Watch Together messages

One WebSocket per device. Every message has `v`, `type` and, from the server, `seq`. The group
state and its rules are [watch-together.md](watch-together.md)'s; this is their wire form. There
is **no clock synchronisation** ([D66](../../decisions/working-notes.md)): positions are sent with
the sender's monotonic time, and round trips are measured with `ping`.

| Direction | `type` | Fields |
| --- | --- | --- |
| client → | `hello` | `credential` (the membership's), `sessionToken` (a signed-in person's, optional; D243 §2), `role` (`playback`, `voice`, `both`), `canSetRate` — within 10 s of opening |
| client → | `ping` | `tMono` |
| client → | `state` | `positionMs`, `tMono`, `playing`, `buffering`, `rttMs` (the shortest round trip its pings measured) — sent on every change and every 60 s |
| client → | `command` | `command`: `play`, `pause`, `seek` (`positionMs`), `rate` (`rate`), `changeItem` (`fileId`; host, to a film the host reaches); with `basedOnSeq` |
| client → | `suspend` / `resume` | `reason`: `stallRecovery`, `audioSessionInterrupted`, `userActionRequired`, `userPausedLocally`, `userIsChangingCurrentTime` |
| client → | `rtc` | `toDeviceId`, `sdp` or `candidate` (voice signalling only; media goes browser to TURN) |
| → client | `welcome` | `sessionId`, `memberId`, `deviceId`, `abilities`, `roster`, the current `group` |
| → client | `pong` | the echoed `tMono` |
| → client | `group` | `seq`, `group`: `state` (`Idle`, `Waiting`, `Paused`, `Playing`), `positionMs`, `rate`, `fileId`, and the `event` with the `byMemberId` whose command caused it (named events, D188) |
| → client | `roster` | `roster`: members (`id`, `name`, `isHost`) and their devices (`id`, `role`, `isBuffering`, `suspended`, `rttMs`); `refusedJoins`, the joins the cap refused |
| → client | `rejected` | `basedOnSeq`, `code` (§5.2: `forbidden`, `stale`, `invalid_request`, `not_found`) |
| → client | `rtc` | `fromDeviceId`, and the relayed signalling |
| → client | `ended` | `reason` (`hostEnded`, `ejected`); the socket closes after it |

**The server applies commands in arrival order, and each `group` it sends carries a new
`seq`.** A command that names a stale `basedOnSeq` is still applied if it is not a conflict.
If it is (a seek against a seek), the latest wins and the loser receives `rejected`. Drift is
corrected by the client: nudge the rate if it can, otherwise seek. The thresholds are in the
plan and are measured in the device lab.

**Recording (D191).** A membership that is signed in, on any of its devices, has its viewing
recorded as that user's spans, with the playback device reporting. The host's viewing is always
recorded.

### 5.2 The error model

One shape everywhere, one enumeration shared by the Go server and the SDK:

```json
{"error": {"code": "quota_exhausted", "message": "…", "detail": {"downloadsResetAt": 1790661600000}, "failedHop": "provider"}}
```

`code` is one of:
- `setup_required`, `auth_required`, `forbidden`
- `not_found`, `invalid_request`, `conflict`, `stale`
- `rate_limited`, `quota_exhausted` (OpenSubtitles), `provider_error`
- `node_silent`, `file_unavailable` (the node cannot see it; never "deleted")
- `preparing` (with `425`: a rendition's file does not exist yet; ask again after `Retry-After`),
  `prepare_failed`, `migrating`, `internal`

`failedHop`, where present, names the failing leg for reachability errors: `client`, `core`, `node`,
`provider` or `tunnel` ([remote-access.md](remote-access.md)).

`detail` is typed by the code, and the API document gives each code its form (`ProblemBody`), so a
number is a number: `auth_required` carries `reason`; `invalid_request` carries `problems`, each a
`location` (a field such as `body.name`, or `body` for the body as a whole; always present) and a
`message`; `rate_limited` carries `retryAfterMs`; `quota_exhausted` carries
`downloadsResetAt`, in Unix milliseconds; `provider_error` carries `reason`, in the provider's
terms. The other codes carry none (M3's closing audit, under D227).

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
  `runnable_at` has passed and whose runner has a free slot. A node's slots are its own: the
  encodes and remuxes its config says, and one each of subtitle extraction, loudness
  measurement and artwork (`nodeproto.Slot`), and a device it reads is held to its reader limit
  (D240 §5). A provider has one fetch at a time under its rate limit, and there is one backup.
- **Node jobs** are given to their node, not waited for: each becomes a command, and its result
  names the job and the attempt (§4, D234 §7).
- **"Play this now"** sets the highest priority.
- **Cancelling** a job sends `cancel` to the node.
- **After a stop, `running` jobs return to `queued`.** Output is a cache, so they restart from
  zero. An assigned job stays with its node, which still holds it (D234 §7).
- **The job view** is `GET /jobs`: kind, its target's id (one field per kind, D227), state,
  `progressFraction`, reason, `lastFailure` (the last error, named so by D227).

## 6. Prepare

[D193](../../decisions/working-notes.md) §2 is the decision; this is the mechanism.

**The decision (`POST /playback`).** The request carries the file, the device id, the client's
prioritised `playableFormats` per axis ([D21](../../decisions/working-notes.md)) with the
containers it plays as they are ([D217](../../decisions/working-notes.md) §1), whether its
`network` is local or remote, and a person's `preferredDelivery` (`direct` or `hls`, the answer's `delivery` values) when
they override it. The answer names every reason that stopped direct play, as
`notDirectReasons`, not only the first (the names are D227's). The core resolves it in this
order:
1. **The original is acceptable on every axis** (container, video, audio, and the device is not
   on stereo output with a sound of more than two channels, whether the boost is on or off —
   [D222](../../decisions/working-notes.md)): `{delivery: "direct", url}`.
2. **A complete or running prepared rendition matches:** `{delivery: "hls", url, notDirectReasons}`.
3. **Otherwise:** queue `prepare` at the highest priority and return `{delivery: "hls", url,
   notDirectReasons, isPreparing: true}`. The playlist URL answers `425` until the first segment exists; the SDK
   waits and shows *"Preparing — ready in a few seconds"*.

The response also lists the subtitle tracks, each with an `.ass` and a `.vtt` URL, and the
audio renditions.

**The profiles:**

| Profile | Chosen when | Video | Audio renditions |
| --- | --- | --- | --- |
| `remux` | the video codec is in the client's list | `-c:v copy` | `aac-stereo` (standard downmix) and `aac-stereo-boost` (centre raised) |
| `h264-8m` | the video codec is not in the list, or the `network` is remote and the source is above ~10 Mbps | `h264_qsv`, else `h264_nvenc`, else `libx264`; ~8 Mbps, 2 s GOP | the same two |

**One FFmpeg run per job** (a sketch; M3 verifies the exact arguments):

```
ffmpeg -nostdin -hide_banner -protocol_whitelist file -i <source>
  -map 0:v:0 -c:v copy | -c:v h264_qsv -b:v 8M -maxrate 10M -bufsize 16M -g <2 s>
  -map 0:a:<chosen> -c:a aac -ac 2 -b:a 192k                                  # standard
  -map 0:a:<chosen> -c:a aac -b:a 192k -af "pan=stereo|FL=<centre-boost matrix>|FR=<…>"
  -f hls -hls_segment_type fmp4 -hls_time <hls_segment_length_seconds: 2> -hls_playlist_type event
  -master_pl_name master.m3u8 -var_stream_map "v:0,agroup:a a:0,agroup:a,default:yes a:1,agroup:a"
  <out_dir>/v%v/stream.m3u8
```

- **Text subtitles** are extracted by `extract_subtitle` jobs queued alongside, one per track.
  The output is ASS for JASSUB and WebVTT for the cast receiver, served as sidecar files rather
  than as HLS renditions in Cut A.
- **The boost matrix** is one fixed matrix whose centre gain is set in M3 by listening test on a
  television, and recorded as an `assumptions/` test.
- **Where the output lives:** `<file_id>/<profile>/<run_id>/` under the node's own prepared directory,
  `cache/prepared/` in its instance (D242 §3; before it, the `prepared_dir` key). The node owns it
  and is the only thing that deletes there (D217 §5).
- **The `prepared_rendition` row** is written when the job starts and marked complete when it
  ends. Play state records against the source file
  ([D180](../../decisions/working-notes.md)).
- **Eviction** is the node's: least-recently-served beyond a size budget in the node's
  configuration, keeping a rendition while someone is partway through its title or for a short
  grace after its last play, within the budget; when the disk that holds the directory is past
  its limit (95% full to start), renditions are evicted as they are past the budget, and when
  nothing can be evicted a new prepare is refused with a reason
  ([D217](../../decisions/working-notes.md#d217--m3s-playback-contract-the-client-sends-ranked-lists-the-media-token-is-a-reference-every-audio-track-is-prepared-and-hlsjs-jassub-and-playwright-are-allowed) §5). ~~A rendition that a live session is playing is never
  evicted.~~ *(Amended 2026-09-30 to D217 §5's wording, which the node builds: a rendition
  played within the grace is kept only while the budget allows. The owner: "agreed",
  [M3's closing audit](../research/audit-2026-09-29.md) §9 D.)* When the node answers that a rendition is gone, the core prepares it again.

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
| **M1 — schema, migrations, backup** *(done 2026-09-24, VQ1 open; [audit-2026-09-24.md](../research/audit-2026-09-24.md))* | the config file and `orpheum dev seed` for users and settings (§2a); [schema.md](schema.md) as migration 0001 through the D181 runner and sqlc (folded since, with the steps after it, into the baseline by [D228](../../decisions/working-notes.md#d228--until-migrations-are-live-librarydbs-schema-is-a-baseline-and-one-step-and-a-schema-change-folds-the-last-step-into-the-baseline)); the `setting` registry; sealed secrets and `secret.key`; argon2id passwords; `VACUUM INTO` backup; `orpheum restore`; the setup code and the first admin | A fresh schema equals the golden. Killing the process mid-migration and restarting resumes. A database whose `compatible_version` is newer is refused, and the refusal names the snapshot. A backup restored into an empty data directory reproduces every authored table. A file without the `application_id` is refused. `/health` says `migrating` before the migration starts. **A canary secret appears in no API response, log, `doctor` output or backup except as ciphertext, nor in a bug report once they exist.** A restore without `secret.key` marks secrets *needs re-entering* and loses nothing else. argon2id takes ≤ 500 ms on devnas. |
| **M2 — scan and identify** | node child and both sockets; enumerate, probe, payload hash; JSONL reports and ingest; the filename parser; the job queue with fetches; TMDB; NFO read; the ambiguous chooser; overrides; the scanner view; artwork; the rolling refresh, and the web client's About section with TMDB's logo and notice and our acknowledgments (D213 §7); browse and search | On the tier-1 corpus plus a 200-file fixture tree, every file has a filing and a reason. D179's table holds as tests: a `mkvpropedit` title edit keeps identity, and an `ffmpeg -c copy` remux gets a new one. No ambiguous title is auto-matched. An override survives re-derivation. With the media root at mode 000 to the core's user, the core still works: it never opens a media path. |
| **M3 — play in a browser** | the playback decision; direct play relayed; **prepare** with both profiles; subtitles via JASSUB and the offset; fetching a subtitle on request ([D223](../../decisions/working-notes.md#d223--the-m3-audits-decisions-contract-names-are-made-clear-now-subtitles-start-off-and-fetching-a-subtitle-on-request-is-in-cut-a) §4); the dialogue boost; loudness levelling in the web player (D226); play state, Continue Watching with hide and undo; accounts; the web client usable on a phone; the job view; confinement and its report; the System page with hardware encoding (D225) | An MKV with H.264, TrueHD and ASS plays in Chrome, Firefox and iPhone Safari within 8 s of pressing play, with the reason on screen. The boost measurably raises the centre channel's share of the stereo output. A quiet film's measured gain brings it towards -23 LUFS, no further than its peak allows. Two devices reporting out of order never rewind the position. `mediastreamvalidator` passes on a prepared rendition. The escape test is green on devnas. |
| **M4 — two people, one remote** | the HTTPS name; reachability; invites; Watch Together over §5.1; voice through embedded TURN; recording for signed-in members; now playing; scans and backups as jobs in the job view, with scheduled backups ([D223](../../decisions/working-notes.md#d223--the-m3-audits-decisions-contract-names-are-made-clear-now-subtitles-start-off-and-fetching-a-subtitle-on-request-is-in-cut-a) §6) | On D190's remote-household VM under its netem profiles, two Playwright clients stay within the drift bound for 30 minutes. Fake-microphone voice arrives within the jitter and loss bounds. An invite works once. Revocation ends access at the next request. No credential appears in any URL in a HAR capture. A signed-in guest's history advances; an anonymous one's does not. |
| **M5 — the polished UI** ([D245](../../decisions/working-notes.md)) | one skinned web UI, from research into best practices, competitors and what people say of them, and into what the web's practice forgot of older UI toolkits; a UI principles document, and the standards that touch the UI revisited against it; an architecture supple enough to iterate on | The two people use it on their web browsers, the remote one's voice on her phone's browser, and the owner calls it polished. A change to one screen's look needs no change elsewhere. |
| **M6 — the remote TV** (after Cut A, D245) | the first working path from [D188](../../decisions/working-notes.md)'s table for the remote person's actual TV; "Get this ready" and the session-start prepare of `h264-8m`; the cast receiver's loudness levelling ([D226](../../decisions/working-notes.md#d226--loudness-is-levelled-in-cut-a-the-node-measures-each-sound-and-its-stereo-mix-the-core-sets-a-gain-towards--23-lufs-that-no-peak-exceeds-and-the-player-applies-it-after-its-volume) §4, moved from M3 on 2026-09-30) | On that TV class, with the film on the TV and voice on the phone, a whole film completes with no manual resynchronisation. A prepared rendition plays without stalling over the "congested" profile. Resuming at home continues from the remote position. Deleting `cache.db` loses nothing but prepared renditions. |

**Cut A is done** after M5 and a week of real use by its two users. The first public beta
([D192](../../decisions/working-notes.md)) is a separate decision.

## 9. Before M0: what only the owner can do

- **Buy a Chromecast.** Phase 0 ([playback-survey.md](playback-survey.md)) needs one, and M6
  may too. Buy the same model as the remote person's if they have one.
- **Test the ISP:** whether a forwarded port is reachable from mobile data, and whether IPv6 is
  present.
- **Choose which domain** carries `media.<domain>`, and give its DNS provider's API access to the
  certificate client.
- **Enable Landlock on nas's kernel** before M3's escape test runs there, or accept *unconfined*.
- **Find out the remote person's TV setup**, which is due in about two weeks and needed by M6,
  not before.
