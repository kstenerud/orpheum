> **ABOUTME:** Whether the chunk ladder and its on-time grid can carry every playback that still
> falls back to a whole-film prepared copy, and what removing the prepare pipeline takes. Read
> before touching `internal/core/renditions`, `internal/node/prepare` or the prepared directory's
> renditions.

# Retiring the whole-film prepare

- **Status:** BUILDING — 2026-10-26. The owner agreed to the audit's proposal (C9/N6 of
  [audit-2026-10-25.md](../research/audit-2026-10-25.md)): check whether the on-time grid
  ([D269](../../decisions/working-notes.md)) can carry the remaining fallbacks, then retire
  prepare, or record why it stays. §1 is the check; every case is carried, so §3 removes it.
  Decided as [D344](../../decisions/working-notes.md).
- **Governing decisions:** [D247](../../decisions/working-notes.md) §10 (chunks on the source's
  keyframes, made on request), [D260](../../decisions/working-notes.md) §1 (every film that can be
  cut plays from its ladder), [D269](../../decisions/working-notes.md) (a film with no index is
  cut on time), [D323](../../decisions/working-notes.md) §1 (the twin), and the prepare decisions
  D344 narrows: [D193](../../decisions/working-notes.md) §2, [D217](../../decisions/working-notes.md)
  §3 and §5, [D225](../../decisions/working-notes.md) §3.

## 1. Where a playback still prepares

`playback.Service.Start` queues a whole-file prepare when the decision is HLS (the device cannot
play the file as it is) and the playback is not on its ladder. `startOnLadder` says why it is not:

| Case | When it happens | Carried by the ladder? |
| --- | --- | --- |
| `noGridYet` | The file has an index, but its keyframes were not read within `play_ladder_wait_ms` (2 s): a busy node, a slow disk, or a read-through retried for the person's play. | **With a small change.** Cut on time (D269 §1) once the wait is over: the playback is offered the encoded qualities at once and keeps that grid to its end (D269 §3); the read-through goes on, and the next playback is cut on the keyframes. |
| `cannotBeCut`, FFmpeg could not read the file through | The picture's timeline records a failure: no index and a full read that failed. | **With a small change.** The on-time grid needs no keyframes, only the probe's length: cut on time. Each chunk is its own FFmpeg run from a seek, so a damaged stretch fails its own chunks, where a prepare, one run over the whole file, failed at the damage (and the read-through had already failed on it). |
| `cannotBeCut`, no keyframe before the end | `GridOf` finds no keyframe to start a chunk on. | **With a small change.** Cut on time, as above. |
| `cannotBeCut`, no picture | The probe has no video stream. | **Not a regression.** A prepare cannot carry it either: its command maps the main picture, and with none it mapped stream 0 as the video. Such a file plays as it is where the device can; otherwise its play is refused as one that cannot be converted (§2). |
| `cannotBeCut`, no known length and no keyframes | The probe has no duration and the read-through failed, so neither grid can be built. | **Not a regression in practice.** The read-through that failed is a full FFmpeg read; a prepare is another, and fails the same way. Refused as one that cannot be converted (§2). |
| `noQualityOffered` | Cut on keyframes, the player cannot decode the picture, and the picture is shorter than the lowest quality allows a twin (`qualities.MinHeight`, 144 lines), so nothing is offered. | **With a small change.** Offer the one quality an on-time film already gets when none is below its picture (`onlyQuality`, D269 §1), whatever the grid. |
| `noLadder` | `Parts.Chunks` is nil: only in tests; `serve.go` always wires it. | Removed: chunks become a required part. |
| `originalChosen` | The person chose the original. | Never prepared: `decide.Decide` answers direct play for it. |

**Quality on time.** A film cut on time is not offered its own picture copied (a copy needs a
segment to start on a keyframe), so a player that decodes the picture would get only the
qualities below it, 720p at most, where a remux prepare copied it. D323's twin, H.264 at the
picture's own height, closes that: on time, the top of the ladder is the twin whether or not the
player decodes the original. That is today's ladder for a player that cannot decode it, which
the node already makes in real time; the speed rules (D250 §6, D309 §6) move a player down when
it cannot.

Every case is carried, so prepare is retired (§3).

## 2. A playback that cannot be on its ladder

With prepare gone, a playback whose decision is HLS and that cannot start on its ladder has no
other way to play. Its start is refused with `cannot_convert` (422) and the reason, which the
player shows as it shows any refusal. Only the two cases above that a prepare could not carry
either reach it.

## 3. What depends on prepared renditions, and what becomes of it

| Dependant | Today | After |
| --- | --- | --- |
| `core/renditions` | wants, queues, records and adopts renditions; the System page's finished prepares | **Removed.** Its audio renditions (`AudioRenditionsOf`, `PreparedAudio`), which the chunks' sounds are, and `ContentType` move to `core/chunks`. |
| `core/playback` | `wantPrepared`, `hls.go` (a prepared playback's files), the token's profile, `leave.go`'s `ReleaseWanted`, `tracks.go`'s prepared picture and sounds | **Removed;** a start not on its ladder is refused (§2). |
| `core/api` | `/bytes/media/hls/{token}/…` and the prepared master route, `isPreparing`, `audioRenditions`, the file's `prepare` wait, the System page's recent prepares | **Removed.** |
| `core/waits` | `prepareOf`, from open prepare jobs | **Removed.** |
| `core/jobs` | the `prepare` kind, its `rendition_id` target, the prepared directory's destination | **Removed**; the prepared directory's destination stays while chunk runs park on it. |
| `core/nodes` | reconciles the index with a hello's renditions (`AdoptRenditions`, `KeptRenditionDirs`) | **Removed.** |
| `store` (cache.db) | `prepared_rendition`, `job.rendition_id`, the `prepare` kind | **Removed**, the cache schema's version raised (versioned by discard). No persistent.db step. |
| settings | `system_page_recent_prepares_per_node`; `prepare_progress_every_seconds`, which read-throughs also use | The first **retired**; the second **renamed** `node_progress_every_seconds`. `hls_segment_length_seconds` and `play_ladder_wait_ms` stay, their words changed. |
| node config | `prepared_budget_gib` and four more | **Stay:** they bound the chunk stores, which live in the same directory. |
| `node/prepare` | the prepare run; the encoder trial; the sound mixes | The run is **removed**; the encoder trial and the mixes, which chunks and loudness use, move to `node/encode`. |
| `node/prepared` | renditions' shelving, eviction, manifests, the hello's inventory and its evictions | Keeps the stores of chunks, their budget and their eviction; a rendition's parts are **removed**, and any rendition directory left on disk is evicted as a directory with no manifest of a store. |
| `node/agent`, `node/byteserver`, `nodeproto` | the prepare command and report, the hello's renditions, a rendition's byte route | **Removed.** |
| web | a prepared playback's waiting, its audio renditions, the System page's recent prepares, the jobs page's prepare kind | **Removed.** |
| Watch Together, cast | each viewer's player starts its own playback | Unchanged: they play what the start answers. |
| Offline download ([offline-download.md](offline-download.md), not built) | designed as a rendition plus a lease | A design question for when it is built: a download can be one quality's chunks, all made, which are an fMP4 HLS stream. Nothing built depends on it. |

## 4. Steps

1. The ladder carries the cases of §1 (one commit, with its tests).
2. The prepare path is removed (§3), core, node, store and web, with its tests, which go with the
   behaviour (`Test-Weakened:` citing D344).
