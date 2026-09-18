> **ABOUTME:** The hub-and-spoke topology — a core that never touches a media filesystem, and
> N media nodes that live next to the files and do everything requiring access to them.
> Consolidates the scanner, the media worker and the transcoder into one deployable concept.

# Media Nodes

- **Status:** designed 2026-09-18, adopted as [D33](../decisions/working-notes.md).

## The shape

```
                    ┌──────────────────────────────┐
                    │  CORE  (hub)                 │
                    │  HTTP · SQLite · jobs · API  │
                    │  never walks a filesystem    │
                    │  never parses a media file   │
                    └───▲─────────▲─────────▲──────┘
      reports / jobs    │         │         │     (HTTP, or a file on a
      over whatever     │         │         │      shared mount, or a poke)
      transport fits    │         │         │
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
thumbnails and trickplay, and optionally transcoding. Three previously separate concepts become
one deployable thing.

This also **completes [D26](../decisions/working-notes.md) rather than half-satisfying it.**
That decision said the core must never parse attacker-controlled bytes. It stopped FFmpeg
running in the core, but left the core walking user filesystems and reading directory entries
whose names are also attacker-influenced. Now the core's entire input surface is: HTTP requests,
its own SQLite file, and node reports it validates at the boundary.

## One artifact, two roles

**Non-negotiable, or this undoes [D31](../decisions/working-notes.md)'s single-file
simplicity.** The node is not a second download. It is the same binary in a different mode:

```
mediaserver serve                     # core
mediaserver node --core=<url>         # spoke
```

And in the default single-machine install, **the core supervises a co-located node as a child
process**, so the user still installs one thing and runs one thing. Distribution is an
opt-in topology change, not the baseline experience.

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

## Transport is deliberately pluggable

"Whatever transport sits in between" is the right instinct, because the topologies differ
wildly. All of these are valid and the node should support more than one:

- **HTTP POST, node → core.** The default when the node can reach the core.
- **Core polls the node's HTTP endpoint.** For nodes behind a firewall that cannot dial out.
- **A report file on a shared mount.** The original index-file idea: no port, no protocol, no
  authentication, no firewall rule, because both sides already have the share. Still the
  elegant option when a share exists.
- **A poke.** A signal file, a `SIGHUP`, or an API call that means "look now" — composing with
  [D32](../decisions/working-notes.md)'s inbound-notification endpoint, which the `*arr` stack
  can already drive.

The report *content* is one contract; how it travels is a deployment detail.

## The report contract

The hard design work, and where the traps are.

- **Full inventory and incremental delta.** Deltas for speed; periodic full inventory for
  reconciliation, because deltas drift.
- **Deletions are explicit, never inferred.** The same lesson as
  [D8](../decisions/working-notes.md): you cannot derive a deletion from a stream of changes.
- **Per item:** node-local stable id, path, size, mtime, optional content hash, and the probe
  results — so the core never needs to open the file.
- **A declared protocol version**, with a stated compatibility policy. Version skew between core
  and nodes is the classic failure of every distributed system that did not plan for it.

### Do not copy Kyoo's coupling

Kyoo is the closest prior art — it already runs its scanner as a **separate service**, which
validates the "core doesn't scan" half. But its scanner holds a **direct Postgres connection**
(`asyncpg`, `POSTGRES_URL`) and **its own database migrations**, alongside calling the API.

That is exactly what not to do here. It violates [D31](../decisions/working-notes.md)'s
single-writer discipline, and it hands database credentials to the component that handles
hostile files — the opposite of what [D26](../decisions/working-notes.md) wants. **Nodes talk to
the core's API and nothing else. The core is the only writer.**

## The status record

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

### Why a file rather than an API

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

## Bidirectional control, through the same directory

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

## The exchange directory

```
<exchange-dir>/<node-id>/
  config.json      ← core writes,  node reads + watches
  state.json       ← node writes,  core reads
  reports/         ← node writes,  core reads   (scan output; large)
  log/             ← node writes,  core reads   (debug; rotated)
```

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
| media roots | (none needed, in a distributed topology) | **read-only** |

**And the same table is the node's Landlock ruleset** (D26): write access to exactly its own
exchange subdirectory, read access to its media roots, nothing else. The directory layout and
the sandbox policy are the same statement, which is a good sign both are right.

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

## What this permits later, for free

In a true multi-machine topology the core may have **no access to the media at all** — so it
cannot serve direct play either. The architecture already answers this:
[D19](../decisions/working-notes.md) built **signed, scoped, expiring URLs and client
redirection** for the remote transcoder. The same mechanism lets a **node serve its own files
directly to the client**, with the core issuing the redirect.

Not required for v1 — on one machine it is pointless indirection — but it means "the core never
touches media, full stop" is reachable without new machinery.
