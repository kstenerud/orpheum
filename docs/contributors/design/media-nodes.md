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

### Compare counters, not timestamps

Timestamps in the record are **for humans reading it**. The core must not reason about liveness
by comparing the node's clock to its own: clock skew across machines, NTP corrections and VM
suspends all break it.

Instead the node includes a **monotonic counter** alongside each timestamp, and the core records
*its own* observation time when it first sees a given counter value. Staleness is then measured
entirely on the core's clock, from a value that only ever increases. Clock-independent, and
immune to a node whose clock jumps.

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

### Shape

Illustrative, not a specification:

```json
{
  "protocol": 1,
  "node_id": "a7f3…",
  "started_at": "2026-09-18T09:14:02Z",
  "pulse": { "seq": 48213, "at": "2026-09-18T11:02:31Z" },
  "state": "scanning",
  "progress": {
    "seq": 9042,
    "at": "2026-09-18T11:02:28Z",
    "detail": "films/  (3121/48000 entries)"
  },
  "roots": [
    { "path": "/mnt/films", "strategy": "inotify",
      "last_full_scan": "2026-09-18T04:00:11Z" }
  ],
  "last_error": null
}
```

`roots[].last_full_scan` is where [D32](../decisions/working-notes.md)'s "when did you last
actually look?" requirement naturally lives — the node knows, so the node reports it.

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
