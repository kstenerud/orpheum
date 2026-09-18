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

## The traps, named

1. **A dead node is not deleted media.** This is the one that can destroy someone's library.
   If a node goes offline its items must **not** vanish; they become *unavailable*, not gone.
   That means last-seen timestamps, an explicit node state, and tombstoning that only ever
   happens on a positive report of deletion from a live node — never on silence.
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
