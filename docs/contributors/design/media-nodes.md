> **ABOUTME:** The hub-and-spoke topology — a core that never touches a media filesystem, and
> N media nodes that live next to the files and do everything requiring access to them.
> Consolidates the scanner, the media worker and the transcoder into one deployable concept.

# Media Nodes

- **Status:** designed 2026-09-18, adopted as [D33](../decisions/working-notes.md). **Refined
  2026-09-23 by [D193](../decisions/working-notes.md)** (§1, §3, §4): the node does every media
  read and every FFmpeg run, and the core relays its bytes; the encoder is a node capability, not
  a separate role or plugin; FFmpeg is confined **per invocation**; there is **one transport**,
  HTTP+JSONL over a Unix socket (dial-out TCP later) — the file exchange channel, its signed
  command file and the file status record are **deleted** ([D127](../decisions/working-notes.md)
  superseded in part); liveness is an HTTP status endpoint plus the report stream. Sections
  below that describe the deleted mechanisms are marked and kept as history.
  [plans/cut-a.md](plans/cut-a.md) §2 and §4 are Cut A's form.

## The shape

```
                    ┌──────────────────────────────┐
                    │  CORE  (hub)                 │
                    │  HTTP · SQLite · jobs · API  │
                    │  never walks a filesystem    │
                    │  never parses a media file   │
                    └───▲─────────▲─────────▲──────┘
      reports / jobs    │         │         │     (HTTP+JSONL: a Unix socket
      and media bytes   │         │         │      locally, dial-out TCP later;
      relayed (D193)    │         │         │      D193)
              ┌─────────┴──┐ ┌────┴─────┐ ┌─┴──────────┐
              │ node @ NAS │ │ node @   │ │ node @ core│
              │  (inotify) │ │ seedbox  │ │  box       │
              └─────┬──────┘ └────┬─────┘ └─────┬──────┘
                 media         media         media
```

**One core. N nodes. A node lives where files live.** In the simplest and most common
deployment there is exactly one node and it runs on the same machine as the core.

## The consolidation that makes this worth doing

The proposal was for a standalone scanner binary. Following it through, **the scanner and the
media worker want to be the same process**, because they want the same three things:

| Wants | Scanner | Media worker ([D26](../decisions/working-notes.md)) | Transcoder ([D19](../decisions/working-notes.md)) |
| --- | --- | --- | --- |
| To be next to the files | yes | yes | yes |
| FFmpeg | yes (probe) | yes | yes (encode) |
| To be confined, holding no DB credentials | yes | yes | yes |

So the spoke is not "a scanner". It is a **media node**: the component that does *everything
requiring access to a media file* — discovery, probing, subtitle and font extraction,
thumbnails and trickplay, and **prepare** — remux, audio conversion, and video encoding where
the node advertises an encoder ([D193](../decisions/working-notes.md) §2–§3). It also serves
the bytes the core relays to clients, for direct play and prepared renditions alike (D193 §1).
Three previously separate concepts become one deployable thing.

This also **completes [D26](../decisions/working-notes.md) rather than half-satisfying it.**
That decision said the core must never parse attacker-controlled bytes. It stopped FFmpeg
running in the core, but left the core walking user filesystems and reading directory entries
whose names are also attacker-influenced. Now the core's entire input surface is: HTTP requests,
its own SQLite file, and node reports it validates at the boundary.

## One artifact, one process per role

**Non-negotiable, or this undoes [D31](../decisions/working-notes.md)'s single-file
simplicity.** The node is not a second download — it is the same binary in a different mode. But
it *is* a separate **process**, always, even co-located ([D43](../decisions/working-notes.md)):

```
orpheum run                 # supervises every role enabled in config
orpheum core                # or run exactly one role, foreground
orpheum node
orpheum transcode           # withdrawn by D193 §3 — the encoder is a node capability
orpheum relay               # added 2026-09-19 by D71 — deferred past Cut A by D193 §4
```

**~~The `transcode` role~~ is withdrawn** ([D193](../decisions/working-notes.md) §3). A node
advertises what it can produce — `remux`, `aac`, `h264_qsv`, `h264_nvenc`, `x264` — and the
core sends a job to a node that can do it. A GPU machine is a second node, not a third role.

**There is a further role, and it arrived after this document was written.** *(Deferred past
Cut A, which is reached by a port-forward, IPv6 or a standard tunnel — D193 §4.)*
[D71](../decisions/working-notes.md) makes the **relay** a mode of this same binary, run on any
publicly reachable host, which the core **dials out to** — no port forwarding, no dynamic DNS,
works behind CGNAT. It is a role in exactly the sense this section means: its own process, its
own confinement, its own status record, observation-based liveness and the report-don't-exit
rule, all inherited rather than re-specified ([D34](../decisions/working-notes.md),
[D37](../decisions/working-notes.md), [D43](../decisions/working-notes.md),
[D44](../decisions/working-notes.md)). Two things distinguish it from a node and both matter
here: it never touches a media filesystem, and it is **untrusted by construction** — a rented
box — so clients authenticate the *core*, never the relay. It is **blind but not opaque**
([D73](../decisions/working-notes.md)): it reports throughput, connection state, RTT per leg
and every drop, to the household's core, even when we operate the box. See
[remote-access.md](plans/remote-access.md).

Separate processes rather than goroutines because **Landlock is per-process and monotonic**
(D26): a scanner wants read on media, an encode wants read on media, write on a cache and **GPU
device access**. One process means applying the union of those rulesets, so the riskiest
component carries permissions it does not need. **[D193](../decisions/working-notes.md) §4
takes that argument one step further: confine each FFmpeg invocation**, not each role. A
re-exec shim applies, in order, argument hardening, read-only mounts, a seccomp denylist and
Landlock where the kernel has it — read access to the one source file, write access to the one
output directory — and then execs FFmpeg. That is stronger than per-role confinement and needs
no long-lived confined process ([D43](../decisions/working-notes.md) refined). Separate core
and node processes still give resource limits, failure isolation and the D42 boundary below;
two Go processes idle in tens of megabytes, so the cost is noise.

Equally important: **the boundary is then enforced by the OS rather than by discipline.**
[D42](../decisions/working-notes.md) says a node must not hold database credentials or make
policy; in one address space that is a convention, and across processes it is impossible.

`orpheum run` keeps the default install to one thing to start and stop. The single-role
forms exist so anyone with systemd, separate containers or Kubernetes can supervise properly —
that path must not be second-class. Local roles talk over the **same protocol as remote ones**,
on a Unix socket, so the common deployment exercises the code the rare one depends on.

## Why this is simpler than the strategy interface it replaces

[D27](../decisions/working-notes.md) had the *core* choosing between four change-detection
strategies per library root, plus a separate NAS agent as a special case — the same job
implemented twice, with the distributed path as a second-class citizen.

Under hub-and-spoke there is **one scanner implementation**, and it is normally *local to its
files*, so it uses `inotify`/`FSEvents`/`kqueue` and the hard network-filesystem watching
problem largely evaporates. It remains only for users who will not install a node on the
machine holding the files — and for them the node runs on the core box and walks the mount with
the mtime-pruned strategy from [D32](../decisions/working-notes.md). **Same binary, same code,
degraded mode.** No second implementation, and the common path and the awkward path exercise
the same code.

The D27 "NAS agent" stops being a distinct thing. It is just a node that happens to be on a NAS.

## Transport: ~~three, ranked by how many installs use them~~ one, over two carriers

**[D193](../decisions/working-notes.md) §4 keeps one transport**: HTTP+JSONL, over a Unix
socket in the default install and dial-out TCP for a remote node later. The file channel
(item 3 below) is **deleted**, with its signing, attribute-cache and torn-read handling; D127
itself called it *"cheaper to start and more expensive to finish"*. The report grammar and
[D59](../decisions/working-notes.md)/[D40](../decisions/working-notes.md)/[D41](../decisions/working-notes.md)
are unchanged.

The report *content* is one contract; how it travels is a deployment detail — but not an
arbitrary one. Surveyed and measured 2026-09-21
([node-transport.md](research/node-transport.md), [D127](../decisions/working-notes.md)):

1. **A Unix domain socket — the default install.** One machine, the core supervising a
   co-located node. No network, no TLS, no enrolment, no ports, and the socket's permissions
   are the security model in exactly the sense [D35](../decisions/working-notes.md) meant, with
   peer credentials on top. This is the primary path, not an aside, which is what makes the
   common deployment exercise the protocol the rare one depends on.
2. **HTTP, with the node dialling out — a remote node.** Every comparable system converged
   here: GitHub's self-hosted runners long-poll on a 50-second timeout, AWS's SSM agent speaks
   outbound 443 to a message gateway, and **Tdarr's node** — this exact problem in this exact
   domain — holds a persistent outbound connection with *"no inbound Node port required"*.

   **The reason to copy it is security, not firewalls.** [D107](../decisions/working-notes.md)
   found the dangerous direction is core→node, because `config.json` carries commands and,
   since [D89](../decisions/working-notes.md), deletion. If the node always initiates, **that
   direction stops existing as a network path**. Commands arrive as the *response to a poll the
   node made*; D35's `generation`/`observed_config` pair survives unchanged and rides the
   request and response.
3. ~~**A report file on a shared mount — the fallback.**~~ **Deleted by D193 §4.** The original index-file idea: no port,
   no protocol, no firewall rule, because both sides already have the share, and on a NAS with
   a share already mounted that is genuinely zero friction. Kept for that. Its two costs are in
   [node-transport.md](research/node-transport.md) §5 and neither is optional reading: the
   **NFS client attribute cache** (`acdirmax` 60 s by default, doubling while nothing changes)
   means a rename is atomic on the server and *not immediately visible to the core* — and
   polling does not fix it, because the poller reads the cache; and the **command file must be
   signed**, because there is no transport here to authenticate and a typical home share is
   writable by the whole LAN.

**A poke** composes with any of them: a signal file, a `SIGHUP`, or
[D32](../decisions/working-notes.md)'s inbound-notification endpoint, which the `*arr` stack
already drives.

**Dropped:** *"core polls the node's HTTP endpoint, for nodes behind a firewall that cannot
dial out"*. The justification was backwards — a node that cannot dial out is **less** reachable
inbound, not more — and every mode costs. SSH (`rffmpeg`'s answer) and a message broker are
declined: the first because the channel needs authenticating anyway, the second because it is a
second service at household scale ([principles §1](../principles/01-boring-technology.md)).

**What the measurements say.** Throughput does not discriminate — a 50,000-entry snapshot is
50.8 MB and every transport moves it in 536–551 ms. Two things do. **Latency to the first
usable record differs by ~5,000×** (0.1 ms streaming over HTTP against 551 ms waiting for a
rename), which is [D118](../decisions/working-notes.md)'s incremental ingest for free. And the
file channel makes you rebuild acknowledgement, compression negotiation, resume, backpressure
and error reporting — D35 already hand-built the first of those.

## The report contract

The hard design work, and where the traps are. Format and completeness settled in
[D39](../decisions/working-notes.md).

- **JSONL** — header line, entry lines, trailer line. Streams in both directions, appends
  safely, is greppable, and — the strongest reason — **degrades per line** where a single JSON
  document is all-or-nothing. This file is produced from attacker-influenced filenames and probe
  output, so one bad record should cost one entry, not the report.
- **Two kinds, marked per line.** *Inventory* (a full enumeration of a region; absence implies
  removal only where that region is complete) and *delta* (explicit `added`/`changed`/`removed`,
  because deletions are stated, never inferred from a region that was not complete — absence
  means removal only in a complete inventory, the rule [D166](../decisions/working-notes.md)
  applies to clients).
- **Record types: `header`, `entry`, `error`, `skip`, `trailer`** ([D40](../decisions/working-notes.md)),
  and **the core skips kinds it does not recognise, counts them, and does not treat the report
  as malformed** ([D61](../decisions/working-notes.md)). That rule has to exist before the first
  node ships, because it is what lets a record type be added later without a protocol break or
  a node/core upgrade ordering problem.
  Errors sit **in the stream, in order**, because position says what was and was not covered
  around the failure.
- **Per item:** node-local stable id, path, size, mtime, optional content hash, and the probe
  results — so the core never needs to open the file.
- **Observation only, never conclusion** ([D59](../decisions/working-notes.md)). The node
  reports the filename verbatim and does **not** parse it into series/season/episode — that
  needs library context it lacks, and more importantly a parser improvement must be replayable
  over the database without touching a filesystem. It does look *inside* the file, because
  people misname things: the extension is a hint about whether probing is worthwhile, never a
  conclusion about content, and where probe and extension disagree **both are reported**. Two
  corollaries: report the event you were given and never synthesise linkage (`A gone` +
  `B appeared`, not "moved" — unless the kernel handed you the `MOVED_FROM`/`MOVED_TO` cookie
  pair, in which case the link is something you saw); and **node-local state may change cost,
  never content** — delete the probe cache and the node must publish an identical snapshot,
  only slower.
- **Path is two fields, not one** ([D49](../decisions/working-notes.md)). A node on macOS and a
  node on Linux report **different bytes for the same file** — `Amélie` is 8 bytes decomposed
  and 7 composed — which under `UNIQUE(path)` is two library items and under a reconciling
  scanner is delete-and-add churn every time the reporting node changes. So the record carries
  an **NFC-normalised** form, which is what identity, dedup, display and search use, and the
  **raw bytes**, which are what opens the file: APFS normalises on lookup, ext4 does not, so an
  NFC string will not open a decomposed filename on Linux. Same shape as
  [D24](../decisions/working-notes.md)'s rule for track tags — normalise for use, keep the
  original for provenance.
- **The output is a snapshot plus a journal** ([D59](../decisions/working-notes.md)). The
  snapshot is a complete enumeration of the roots at a point in time — everything below about
  reports applies to it unchanged — and the journal is append-only observations since, in the
  same record grammar. Both carry a monotonic sequence and the snapshot names the sequence it
  corresponds to, so the core detects a gap and falls back to the snapshot. **A snapshot is
  always a valid recovery point, so the node never needs to know what the core has read** —
  which is what keeps retention a local storage decision rather than a protocol. **Taking a
  snapshot and discarding journal are separate operations**
  ([D60](../decisions/working-notes.md)): snapshots are temporal, because their cadence is the
  correctness floor; journal is discarded only once superseded *and* older than the retention
  window; and a byte ceiling exists as a loud backstop against filling the node's disk. Size is
  the wrong primary trigger because the burst that blows it is the anomalous event, so it would
  force a full re-enumeration exactly mid-disruption. And because a
  watcher can fail silently ([D27](../decisions/working-notes.md)), the periodic snapshot is not
  only compaction: it is the only thing that can correct for an event nobody delivered.
- **Completeness lives in the filename.** The node writes `report-<epoch>.jsonl.partial` and
  atomically renames on success, so completeness is an O(1) directory check. **Amended
  2026-09-20 ([D118](../decisions/working-notes.md)): the core *may* tail a `.partial` file**,
  so results reach the library as they are produced rather than when the scan ends — reading
  only up to the last newline, and treating everything so read as incomplete by definition.
  The rename keeps its one job, which is the moment removals may be applied. A trailer is kept for the *summary* — counts, duration,
  errors, checksum — not as the completeness signal, because the **absence** of a trailer cannot
  distinguish crashed from cancelled from still-writing. (That ambiguity is at least safe:
  unknown means incomplete means add-or-update only.)
- **Completeness is scoped, not global**, and this is what makes cancellation worth having. A
  scan cancelled after `/mnt/films` finished has complete coverage *of films* — removals there
  can be applied, and `/mnt/tv` is simply untouched. Conversely a scan that completed but could
  not read one subtree is complete everywhere except there, and **the files beneath an
  unreadable directory must never be treated as deleted.** That is the dead-node trap at finer
  granularity and the likeliest way to lose data in ordinary operation, because permission
  errors are common and undramatic.
- **The `error` record is how that scoping is expressed**, which makes it load-bearing rather
  than diagnostic. Each carries a **`scope`** — `entry` (one file; siblings unaffected),
  `subtree` (a directory could not be listed; nothing beneath it is known), `root` (share down),
  or `run` — plus a structured `path`, a stable project `code` (`permissionDenied`,
  `notFound`, `ioError`, `timeout`, `unsupported`, `probeFailed`, `cancelled`) rather than a
  platform errno, an optional raw platform detail for humans, and a human `reason`. Without
  `scope`, one unreadable file would invalidate its directory — or an unreadable directory would
  read as an empty one, which is the data-loss path.
- **A file that enumerated but failed to probe exists.** Present-but-unprobed, `entry`-scoped,
  never a removal.
- **Diagnose the boundary, do not count failures** ([D41](../decisions/working-notes.md)). On a
  read error the scanner **walks up the tree** — stat the parent, then its parent — until
  something succeeds. The highest failing ancestor *is* the `scope`, so ten thousand failures
  become one diagnosis plus a handful of `stat` calls. Memoise the failed prefix for the run.
  Aggregation and a cap remain as a backstop for pathological cases, not as the mechanism.
- **`skip` is not `error`.** A file excluded by an ignore rule is not a failure, but is also not
  in the inventory — and the right behaviour differs: something the user deliberately excluded
  probably should leave the library, something we failed to read definitely should not. Record
  the rule that caused the skip.
- **A declared protocol version**, with a stated compatibility policy. Version skew between core
  and nodes is the classic failure of every distributed system that did not plan for it.
- **Retention is trivial with epoch-named files** — keep the last N. Compression is worth it and
  does not break appending, since gzip concatenated streams stay valid.

### Do not copy Kyoo's coupling

Kyoo is the closest prior art — it already runs its scanner as a **separate service**, which
validates the "core doesn't scan" half. But its scanner holds a **direct Postgres connection**
(`asyncpg`, `POSTGRES_URL`) and **its own database migrations**, alongside calling the API.

That is exactly what not to do here. It violates [D31](../decisions/working-notes.md)'s
single-writer discipline, and it hands database credentials to the component that handles
hostile files — the opposite of what [D26](../decisions/working-notes.md) wants. **Nodes talk to
the core's API and nothing else. The core is the only writer.**

## The status record

**Simplified by [D193](../decisions/working-notes.md) §4:** node liveness is an **HTTP status
endpoint plus the report stream** — a node that is reporting is alive, and the endpoint says
what it is doing (`state`, `task`, `detail`, the last error). The status **file**, the reasons
for a file below, the progress pair's tick contract and the idle tick are withdrawn with the
file channel; the distinctions this section draws — idle versus wedged versus dead, core-side
timing, non-destructive escalation — still hold, and are what the endpoint and the stream are
read for.

Each node maintains a small JSON record — separate from its scan report — saying what it is
doing, when it last did something, and that it is still breathing. The core reads it to reason
about the node.

### Liveness and wedgedness are different failures, and a heartbeat only finds one

This is the whole reason the record needs more than a timestamp.

| Failure | Heartbeat | Progress | How it looks |
| --- | --- | --- | --- |
| **Healthy, working** | fresh | advancing | fine |
| **Healthy, idle** | fresh | static | **fine** — nothing to do |
| **Dead** | stale | static | process gone |
| **Wedged** | **fresh** | **static** | alive, making no progress |

A naive heartbeat cannot tell wedged from healthy, and cannot tell wedged from idle. So the
record carries **three independent things**:

- **`state`** — what the node believes it is currently doing (`idle`, `scanning`, `probing`,
  `extracting`, `transcoding`, `error`). This is what disambiguates *idle* from *wedged*: static
  progress is correct when idle and a problem when scanning.
- **A liveness pulse** — updated on a timer regardless of work.
- **A progress signal** — updated *only* when real work completes, with enough granularity that
  a sensible staleness threshold exists. A full scan of a large library may legitimately run for
  an hour, so progress must tick per directory or per item, not per scan.

### The core times its own probes; node clocks never enter the comparison

Clock skew dissolves without any clock synchronisation, because **the core only needs elapsed
time, not agreement about what time it is.**

The core reads the state file at its own `T1` and `T2`, and compares the two contents. If any
monotonic field changed, the node was alive somewhere in that window. Staleness is `T2 - T1`
measured entirely on the core's clock, against values that only increase. The node's own
timestamps are **decorative** — for a human reading the file — and NTP corrections, VM suspends
and a NAS whose clock is four hours out are all irrelevant.

The only requirement this places on the node is that **something always changes** while it
lives — satisfied by the progress pair, including when idle (below).

### ~~Why a file rather than an API~~ (withdrawn, D193 §4)

The proposal was a file, and it is the better choice for three reasons beyond convenience:

1. **It survives the process.** An API dies with the node; a file is still there afterwards
   saying what the node was doing when it stopped. That is exactly what post-mortem needs.
2. **It distinguishes three failure modes where an API gives one.** File present with a stale
   counter → the node died. File absent → it never ran, or the path is wrong. Share unreachable
   → a transport problem. An unanswered HTTP request cannot tell these apart.
3. **It works over the shared-mount transport** with no port, protocol or credentials,
   consistent with the rest of §Transport.

And a fourth that is worth as much: **a human can `cat` it.** "Logging around library scanning
being pretty difficult to interpret" is a direct complaint about Jellyfin in
[sentiment.md](research/sentiment.md); a legible status file is a cheap answer.

### Progress is a moving pair, and either half moving means alive

Progress is **task-defined**: a count of files, of directories, of bytes, of items enriched —
whatever the current task naturally counts. It is a `current` and an `end`, and **`end` is an
estimate that is allowed to move**. During a scan the denominator grows as directories are
discovered, which is itself evidence of life.

So the liveness rule is: **if either number moves, the node is working.** Only when both sit
still, for long enough, and the state is not `idle`, is the node wedged.

Four details this needs to be usable rather than merely present:

- **A `unit` label**, or the UI cannot render it. "3,121 / 48,000" of *what*.
- **`end: null` is legal.** Some tasks have no knowable end — watching for events, or a scan
  that cannot estimate. Then only `current` carries the signal.
- **An `epoch` that increments when a task restarts.** Otherwise a reset to zero is
  indistinguishable from being stuck at zero, and the core will call a fresh scan wedged.
- Within an epoch, `current` should be monotonic.

### Idle ticks the pair itself — no separate pulse

An idle node would otherwise change nothing, and so would a dead one. Rather than add a second
mechanism, **idle simply ticks the pair**: enter idle at `current=0, end=1`, then `1/2`, `2/3`.

This is honest precisely because idle is the one state with no task whose completion the
numbers could misrepresent. One mechanism, and the core's rule stays uniform — *did either
number move?* — with no branch on state. ([D36](../decisions/working-notes.md))

### The core judges the cadence, from observation

Rates differ by orders of magnitude — an idle node ticks every few seconds, a node probing a
60 GB remux over a slow link may legitimately show no movement for minutes. So a single global
threshold would call healthy nodes wedged.

The answer is **not** for the node to declare an expected interval
([D37](../decisions/working-notes.md)). The core is better placed: it has *history* for this
node, in this state, on this deployment, so it holds an empirical baseline of observed
intervals that adapts where a build-time constant cannot. And more fundamentally, **a wedged
node is not a reliable source of the standard it is judged against** — self-reported health
criteria are a smell, and the node most in need of catching is the one least able to report
accurately.

**The node's contribution is precision about *what it is doing*, not a number** — reported in
three tiers ([D38](../decisions/working-notes.md)):

| Field | Nature | Used by |
| --- | --- | --- |
| `state` | Small stable enum: `idle`, `scanning`, `transcoding`, `error` | API contract; UI headline |
| `task` | **Bounded enum** within a state: `enumerating`, `probing`, `hashing`, `extracting` | **the cadence baseline**; UI subtitle |
| `detail` | Free text: `films/the-unknown.mkv` | humans only; never keyed on |

**The core baselines on the `(state, task)` pair**, and `task` must be a bounded enum for that
to work. Putting the fine-grained information in free text would reintroduce the bimodal
baseline problem while looking solved — free text has unbounded cardinality and cannot be a
key.

### Escalate gradually; nothing is destructive

| Level | Trigger | Action |
| --- | --- | --- |
| **Healthy** | Moving within the observed baseline | none |
| **Late** | Past expectation, briefly | **nothing** — most lateness is a big directory or a slow file |
| **Stalled** | Persistently late, across consecutive observations | surface with `state` and `detail`; stop treating data as fresh |
| **Presumed dead** | No movement for a long multiple | mark that node's items **unavailable** |

Hysteresis both ways — consecutive observations to escalate, a clean run to recover — so the
indicator does not flap. The core picks its own polling interval, so "a missed check" is
core-side and can be short in absolute terms without making escalation twitchy.

**No level is destructive.** Even *presumed dead* only marks items unavailable; tombstoning
still needs a positive deletion report from a live, progressing node. Which is why precision
here was never worth chasing: **the cost of guessing wrong is a UI badge, not data loss.**

### Shape

Illustrative, not a specification:

```json
{
  "protocol": 1,
  "node_id": "a7f3…",
  "started_at": "2026-09-18T09:14:02Z",
  "node_clock": "2026-09-18T11:02:31Z",

  "observed_config": 17,

  "state": "scanning",
  "task": "probing",
  "state_since": "2026-09-18T10:41:12Z",
  "progress": {
    "epoch": 4,
    "current": 3121,
    "end": 48000,
    "unit": "entries",
    "detail": "films/the-unknown.mkv"
  },

  "roots": [
    { "path": "/mnt/films", "strategy": "inotify",
      "last_full_scan": "2026-09-18T04:00:11Z" }
  ],
  "last_error": null
}
```

`node_clock` is **diagnostic only** — useful for telling a user their NAS clock is four hours
out, and never an input to liveness logic. `roots[].last_full_scan` is where
[D32](../decisions/working-notes.md)'s "when did you last actually look?" requirement naturally
lives: the node knows, so the node reports it.

## Bidirectional control, ~~through the same directory~~ as the response to the node's poll

*Since [D193](../decisions/working-notes.md) §4 there is no directory: commands travel as the
response to a long-poll the node made ([D107](../decisions/working-notes.md),
[D127](../decisions/working-notes.md)), and `generation`/`observed_config` ride the request and
response. The command list, cancellation rules and acknowledgement below are unchanged; the
file-watching subsections are history.*

The core needs to ask for things — scan now, change the interval, pause, rescan one root — and
the same directory carries it: **the core writes `config.json`; the node watches and re-reads
it on change.**

### Commands are an ordered list

Alongside settings, the config carries an ordered list of commands from a closed vocabulary —
`cancel`, `scan`, `rescan <root>`, `pause`, `resume` — executed in sequence.

**A list, not a single command**, because `["cancel", "scan"]` is *one intention* and two
separate writes would race. This is the answer to a real and frequent frustration: files added
while a scan is running, and no way to restart without waiting it out.

Requirements for cancellation to mean anything:

- **Poll for commands during work, not between phases.** A phase boundary twenty minutes away
  is not "on a dime". Check at the same granularity as the progress tick.
- **Stop at a safe point**, mark the report incomplete, return to `idle` with a fresh `epoch` —
  which is how the core observes that the cancel landed.
- **`scan` while scanning is a no-op** (at most a rescan-when-done flag), never an implicit
  restart. That is exactly why the explicit two-element form is needed.
- **Every command must be safe to re-run**, because crash recovery re-executes anything where
  `generation` is ahead of `observed_config`.
- **Scope is background work.** Playback-driven transcoding has a viewer attached and its own
  lifecycle.

**And the trap, which is the dead-node trap in a different costume:** a cancelled scan has
enumerated only part of the library. If its partial inventory is treated as authoritative,
every file it had not reached looks absent and the core deletes most of the library — on the
strength of a scan the user interrupted themselves. So **a report carries a completeness flag,
and an incomplete inventory may only add or update, never remove.**

### Acknowledgement is the part that is easy to leave out

Writing a request tells you nothing about whether it was seen. So the config carries a
**`generation`** that the core increments on every change, and the node echoes the last applied
value as **`observed_config`** in its state file.

That is the desired-state / observed-state pattern, and it is what makes the channel reliable
over a dumb shared directory with no acknowledgement of its own: the core can see that a
command landed, or that it did not.

### The node must poll the config file as well as watch it

`inotify` is available to the node for files on its *own* disk — but the exchange directory may
be on the share, which is precisely where watching does not work
([change-detection.md](research/change-detection.md)). So the node **watches and also polls** on
a slow interval. It is one small file; the cost is nothing and it removes a silent failure.

### Where HTTP exists, prefer it

This file channel is the zero-configuration fallback. When the node can reach the core (or the
reverse) over HTTP, that is the better path for the same contract — lower latency, immediate
acknowledgement — and "scan now" in particular already has a better route through
[D32](../decisions/working-notes.md)'s inbound endpoint. The file mechanism exists for the
topologies where none of that is available.

## The exchange directory — deleted

**Deleted by [D193](../decisions/working-notes.md) §4**, with the file transport. Kept as
history, not normative. What survives from it: a **stable `node_id`** generated once and
persisted locally; **status is not the report**; and D107's rules below — a node's roots are
local configuration the core cannot add to, and a remote node's enrolment is a deliberate human
act. The node's Landlock ruleset is now per FFmpeg invocation (§One artifact, one process per
role), not a directory table.

```
<exchange-dir>/<node-id>/
  config.json      ← core writes,  node reads + watches
  payload/         ← core writes,  node reads   (bulk core→node; D142)
  state.json       ← node writes,  core reads
  reports/         ← node writes,  core reads   (scan output; large)
  log/             ← node writes,  core reads   (debug; rotated)
```

**`payload/` was added by [D142](../decisions/working-notes.md)** and the reason is worth
keeping, because the omission was deliberate and correct until it was not. This directory was
asymmetric by design: small desired-state downward, bulk upward. **Sidecar export inverts the
flow** — the core becomes the producer of thousands of small documents the node must write —
and pushing those through `config.json` would abuse a desired-state file. Mirroring the
directory keeps the permission split, the retention obligation and the node-id namespacing
identical. Over HTTP or a Unix socket this is a request body and needs nothing.

**Namespaced by node id**, because several nodes may share one exchange location and would
otherwise clobber each other. A duplicate `node_id` — from a copied config — must be detected
and reported rather than silently interleaved.

**Reports and logs need retention policy from day one.** Both grow without bound otherwise, and
an exchange directory that fills a share is a memorable way to break someone's NAS.

### The permissions split *is* the security model

This is the strongest part of the design, because it matches the trust model exactly. The node
is the component that handles hostile media ([D26](../decisions/working-notes.md)), so it is the
*less* trusted half — and the layout lets a careful user enforce that with filesystem
permissions alone:

| Path | Core | Node |
| --- | --- | --- |
| `config.json` | **write** | read |
| `state.json`, `reports/`, `log/` | read | **write** |
| media roots | (none needed, in a distributed topology) | **read-only**, unless a library was granted a writable path |

**And the same table is the node's Landlock ruleset** (D26): write access to exactly its own
exchange subdirectory, read access to its media roots, nothing else. The directory layout and
the sandbox policy are the same statement, which is a good sign both are right.

### The HTTP transport needs the same model stated, and it was not

[D107](../decisions/working-notes.md), 2026-09-19. The table above is the security model for
the **file** transport, and this document then says *"where HTTP exists, prefer it"* without
saying what authenticates either end of the preferred one.

The dangerous direction is the one this document's framing hides. The node is consistently
described as the *less* trusted half — correctly, it handles hostile media — so the attention
went to what a bad node can inject. But `config.json` flows the other way, it carries commands,
and since [D89](../decisions/working-notes.md) it carries **deletion**. A spoofed core is worse
than a spoofed node.

So, over HTTP: **mutual authentication, with enrolment as a deliberate human act** — reusing
[D93](../decisions/working-notes.md)'s pairing rather than inventing a second mechanism. A node
that can be adopted by whatever answers first can be adopted by an attacker on the LAN.

And: **a node's roots are local configuration; the core cannot add one.** The command
vocabulary already names roots by identifier rather than by path, which is the right shape —
this makes it a rule. The reasoning is the one two paragraphs up, applied one level out: *the
directory table is the node's Landlock ruleset*, and a Landlock ruleset is declared at startup
and cannot be widened at runtime. The core commands within a scope the node's operator
declared; it never expands it. Otherwise a compromised core turns every node into an arbitrary
filesystem enumerator reporting over the network, on the machine that by design has access to
everything.

The visible cost is that **adding a library is a two-sided act** — declared on the machine
holding the files, adopted by the core. On the default single-machine install both sides are
the same process and the user sees one step.

### Degraded modes, stated rather than discovered

- **Core cannot write the exchange directory** → no bidirectional control. The node runs on its
  own configured defaults and the core reads state only. Legitimate, and the core should say so
  rather than silently failing to apply settings.
- **Node cannot write anywhere shared** → the file transport is unavailable; use HTTP, or run
  the node on the core's machine pointed at a mount (D33's degraded mode).
- **Read-only exchange directory for both** → the user hand-writes `config.json`, and the core
  gets no liveness at all. Everything still *works*; nothing is observable. Worth supporting and
  worth warning about.

When installed as a daemon, the only thing in `/etc` is the path to this directory.

### Details that make it work rather than half-work

- **Stable `node_id`, generated once and persisted locally.** A fresh id per restart makes the
  core see a new node every reboot while the old one goes silent — which walks straight into the
  dead-node trap below. One file per node id, so nodes sharing a directory never clobber.
- **Atomic writes**: temp file, `fsync`, rename in the same directory, plus a length or checksum
  so a reader never trusts a torn file. Readers must handle a torn read gracefully, because SMB
  client caching can still surprise.
- **Write on transition *and* on a timer**, and always populate `last_error` on failure — a node
  that crashes mid-scan should leave behind a record that says what it was doing.
- **Status is not the report.** Two files with two cadences: status is small and frequent, the
  scan report is large and occasional. Conflating them makes the cheap thing expensive.

### What the core does with it

- **Surface dead and wedged differently in the UI**, because the remedies differ: dead means
  start it; wedged means look at `state` and `progress.detail` to see what it is stuck on.
- **Mark that node's items *unavailable*, never deleted** — see the trap below.
- **Never tombstone on silence.** Absence of a pulse is absence of information.
- Optionally restart a supervised co-located node, and back off polling a node that is not
  advancing.

## The silent failure, which is worse than any error

An unmounted mountpoint on Linux usually reverts to the empty directory beneath it. The path
exists, it is readable, listing it **succeeds and returns zero entries**, and **no error is
raised at all.** Every safety mechanism in this document is triggered by errors; this one has
none, and a scanner would honestly report a *complete* enumeration of an empty root — at which
point the removal rules permit deleting the entire library.

**This is the most dangerous failure mode in the design.** But note where the fix belongs —
**the node is a sensor, and the core decides** ([D42](../decisions/working-notes.md)). The
problem is not "the node must detect a missing mount"; it is:

> **The node must report "I listed this directory and it was empty" as distinct from "I did not
> enumerate this directory".**

That distinction is an observation only the node can make. Once it is in the report, the core —
which knows the root held forty thousand files yesterday — draws the conclusion. The node needs
no threshold, no history and no confirmation mechanism.

**The node reports:** observed-empty versus not-enumerated per directory; and per root, the
current `st_dev`, whether the path is a mountpoint, the filesystem type and the read-only flag.

**The core decides**, with two independent defences:

1. **Mount verification.** It compares the reported `st_dev` against what it recorded at setup —
   a mountpoint has a different device id from its parent, so equality means the mount is gone —
   before enumerating and again before applying removals.
2. **Implausible shrinkage.** If a root has lost a large fraction of the entries it held at the
   last complete scan, removals are **withheld pending confirmation**. This catches the whole
   no-error class: a wrong path after reconfiguration, a permissions change that empties a
   listing, a half-finished remount. It must *ask* rather than refuse — someone who genuinely
   emptied a library should be able to say so.

Both need history and configuration, which is exactly why neither belongs in the node.

Related: **a hung mount presents as wedged, not failed.** NFS hard mounts block instead of
erroring, so filesystem operations need timeouts — and usefully, a scanner blocked on a dead
mount stops moving its progress numbers while `state` stays `scanning`, which is exactly the
wedged signature the core already watches for.

And **recovery restores rather than re-adds**: when a mount returns, items marked unavailable
come back with their identity intact. Re-ingesting them as new would lose watch state, metadata
corrections and playlist membership — a data-loss event wearing a recovery costume.

## The traps, named

1. **A dead node is not deleted media.** This is the one that can destroy someone's library.
   If a node goes offline its items must **not** vanish; they become *unavailable*, not gone.
   Tombstoning happens **only** on a positive report of deletion from a live, progressing node —
   never on silence, and never on a wedged node, whose reports are by definition not current.
   The status record (§The status record) is what lets the core tell those apart.
2. **A node's report is attacker-influenced data.** Paths and probe output derive from hostile
   files. The core validates at the boundary — parse, don't validate — and treats a node as a
   semi-trusted peer rather than part of itself.
3. **Overlapping roots.** Two nodes can see the same file (both mount the same share). Identity
   must dedupe rather than double-count.
4. **Clock skew** between nodes, if mtimes are compared across them.
5. **Idempotency and ordering.** A report replayed or delivered late must not corrupt state.
6. **Two things to upgrade** in distributed deployments, even though it is one artifact. The
   compatibility policy has to be real.
7. **A node's own writes come back as observations**
   ([D142](../decisions/working-notes.md)). When the node writes sidecars into a tree it is
   watching, its watcher sees every one of them and journals them — a scan storm immediately
   after an export, which will be reported as a bug. **The node still reports them**: a sensor
   that suppresses its own output is what [principles §9](../principles/09-sensors-report.md) forbids,
   and *"I did this"* is a memory a node should not keep. **The core knows what it commanded
   and does not treat those paths as new information.** §9's own test, applied literally — the
   node saw a file change; only the core knows it asked for it.

## What this permits later, for free

~~In a true multi-machine topology the core may have **no access to the media at all** — so it
cannot serve direct play either.~~ **The core has no access to the media in any topology**
([D193](../decisions/working-notes.md) §1): for direct play it authenticates the request and
**reverse-proxies the byte range from the node** over the node's socket, and it relays prepared
renditions the same way. "The core never touches media, full stop" is therefore true from Cut A
on. A remote node later is the same proxy over TCP — or, where the relay hop costs too much,
**a redirect** to the node serving its own files, using the **signed, scoped, expiring URLs**
[D19](../decisions/working-notes.md) designed for the remote transcoder.

**And it is required sooner than "later", because the artefacts need it**
([D128](../decisions/working-notes.md)). The node *produces* trickplay sprites, subtitle
overlay bitmaps, posters and font subsets; the core *serves* them from `cache/` — or, for what
the node keeps, relays them (D128 refined by D193 §1); nothing said how they cross. Over the reference library that is **42 GB of overlay bitmaps and 5.7 GB of
trickplay** against **51 MB** for a snapshot report — three orders of magnitude between the
channel that was discussed and the channel that matters. So:

| | Where it lives | Why |
| --- | --- | --- |
| Large, cold, streamed — overlays, trickplay, prepared renditions | **the node**, relayed by the core (redirected to, from a remote node later) | 48 GB is not worth moving, and each is fetched at most once per viewing |
| Small, hot, many-per-page — posters, thumbnails | **the core's `cache/`**, uploaded at production | a home page is fifty posters, and fifty redirects is a worse page |

The rule is **requests per page**, not size alone. This is also Netflix's answer: an Open
Connect Appliance does exactly two things — report its status *and what files it holds*, and
serve content over HTTP when a client asks — with the control plane steering clients **via
URL**. Bytes never transit the control plane. *(Ours do in Cut A: on one machine the relay hop
through the core is a socket copy, and it keeps authentication in one place. The redirect is the
remote-node form — [D193](../decisions/working-notes.md) §1.)*
