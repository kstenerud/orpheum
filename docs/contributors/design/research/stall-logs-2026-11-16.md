> **ABOUTME:** What the stalls in production's and staging's logs are today, by kind and by whose
> wait, set against the core's chunk runs; the encode speeds per machine and rung; whether a stall
> the core caused led to a step down; what else a stall design must handle; and the fields missing
> to tell these apart.

# What stalls are in the logs, 2026-11-16

- **Date:** 2026-11-16. Logs pulled once, read-only, into `/yoloai/cache/stall-logs-2026-11-16/`
  (the raw `docker compose logs` and a parsed, deduplicated JSON-lines copy of each).
- **Asked by** the lead, so the owner can design how each kind of stall is handled. It measures;
  it proposes nothing. Background on what other players call a stall:
  [morning-questions-2026-11-16.md](morning-questions-2026-11-16.md) §1.
- **Devices** are named by their id and browser, never by account.

## Log spans

| Machine | What it is | Span read | Notes |
| --- | --- | --- | --- |
| ainas (Celeron J3455, `h264_vaapi`) | **Production**, since the failover of 2026-11-08 | 2026-11-15 20:38:32Z to 2026-11-16 14:03:29Z, 17 h 25 min, unbroken | Starts at the last redeploy: the compose log is all there is. Info level, no debug lines |
| nas.lan | Production before the RMA | none | No containers run on it |
| NUC7 (i5-7260U, `h264_vaapi`) | **Staging** | Six windows, 3 h 19 min in all: 11-14 05:20–05:34 and 09:01–09:36; 11-15 07:40–08:44, 10:02–10:37 and 10:50–11:39; 11-16 13:59–14:00 | Redeployed at 13:59Z on 11-16, so only the last 75 s is live; the rest is from earlier copies kept in the cache. Debug level |

Production saw 31 playbacks (21 from device 49, a Mac running Firefox 157, over WebRTC from outside
the home; 10 from device 34, a Mac running Chrome 157 over HTTP inside it), 19 of them
with a start-up report, and 15 seeks. Staging saw 17 playbacks (device 139, Android 17 Firefox 160; devices 289 and 308, Android Chrome).

## Summary

| Kind of stall | Production (17 h) | Staging (3.3 h) | Whose wait |
| --- | --- | --- | --- |
| Rebuffer mid-play (a `waiting` of 1 s or more) | 0 | 0 | none |
| Slow first load (8 s or more) | **1** | 0 | the core, making the chunk |
| Slow seek (8 s or more) | 0 (15 seeks, slowest ready in 3.4 s) | 0 | none |
| Watch Together fall-behind | 0 (no rooms) | 0 | none |
| "Cannot keep up" (D247 §8) | not logged | not logged | unknown |
| Lower quality offered after a stall | 0 | 0 | |
| Step down of any cause | 0 | 1, a data cap's | not a stall |
| "Stood still" in the link notes (D266) | 9: one is the stall, eight are episodes ending | 0 | the notes blame the link each time; it was never the link |

Every stall in 20 hours of logs is one, and it was the core's wait, which the player reported as
the link's.

## 1. Each stall, by kind and by whose wait

**The one stall: slow first load, the core's wait.** Production, 2026-11-15 21:08Z, device 49,
film 1738, resumed at 141 s, transcoded at `q8000k1080p` (the picture could not be copied).

- 21:08:00.256: the core queues a run of chunks from chunk 31 (the resume point) on
  `chunks-q8000k1080p`. The encode run beside it, on film 1739's run before it, and a sound run are
  still going.
- The run's first chunk is made 6762 ms after it was queued. Until then every ask for the chunk is
  answered 425 with `Retry-After: 1`.
- 21:08:08Z: the player reports a stall. Its fields: position 0, paused, readyState 0, the steps
  `fragLoadError` and `aborted` three times. The position is the one before the resume seek, and
  the "errors" are the 425s, which hls.js names as fragment load errors.
- The start-up report says it began playing after 10 120 ms, with the link measured at 47 Mb/s.
- The link notes for the same moment say "stood still … the link's to bring", with no chunk
  arrived. It was the encoder's to bring.

It was counted because `watchForReport` (`web/apps/web/src/play-state.ts`) calls `watchStalls`
without `isAwaitingTheCore`, so a load held up by the core's 425s counts after 8 s. `tellStalls`
in `web/packages/player/src/chunked.ts` passes it and would not have counted this one. The two
listeners count the same event differently.

**Close to the line.** Film 1739 started from 0 in 5.3 s, its first encoded chunk made after
3769 ms. No other production start took more than 2.4 s; most took 0.2 to 2.4 s.

**Staging.** No stall. The 20 debug lines "waited for a chunk not made yet" are each one ask held
1–20 ms and answered "ask again". Device 139 on film 207 at `q400k360p` (11-15 08:43:26–40) asked
chunks 596, 613, 618, 620 and 633 each when the run had made the one before: the player was reading
at the encoder's front, and nothing it reported shows the viewer waited.

**Transfer and device waits.** None seen, and none can be seen: the core does not log how long a
chunk took to send (see the missing fields), and no stall happened with a chunk in hand.

## 2. How fast the encoders make chunks

From the node's "a run of chunks finished" lines: film time made over wall time. Both machines
encode with `h264_vaapi` on the GPU (from the node's start-up line "this node chose its encoder";
the run lines do not say).

| Machine | Stream | Runs | Speed (film time ÷ wall time) | First chunk after |
| --- | --- | --- | --- | --- |
| ainas | `q8000k1080p` (encode) | 47, about 13 000 s of film | 2.0× to 2.8×, median 2.3× | median 3.6 s, slowest 7.7 s |
| ainas | `q810k488p` (encode) | few | 12.2× to 12.8× | |
| ainas | `picture` (copy) | many | 40× to 370×, median about 154× | slowest 4.8 s |
| ainas | `sound0-96k` (encode) | many | 7× to 20× | |
| ainas | own sound (copy) | many | 50× to 430× | |
| NUC7 | `q400k360p` (encode) | few | 6.3× to 21.7× | |
| NUC7 | `q400k302p` (encode) | few | 28× to 41× | |
| NUC7 | `picture` (copy) | many | 56× to 1053× | slowest 5.1 s |
| NUC7 | sound | many | 11× to 622× | |

- **1080p on ainas is 2.3 times real time, a chunk of about 3.8 s each.** A start or seek into a
  transcode waits for one whole chunk to be made: 3.6 s at the median and 7.7 s at worst, before
  the 425s end. Two such encodes at once on the J3455 would leave little to spare; the start-up
  probe "this node measured how fast it encodes the lowest quality" read 517% of real time on ainas
  against 1420–1440% on the NUC7.
- **No 1080p encode ran on staging** in these windows, so the NUC7 cannot be compared at that rung.
- **Concurrency.** At most 2 encodes ran at once on production, 3 runs of any kind. At the stall,
  the new run shared the machine with one other picture encode and a sound run.
- **Machine load** is not logged by the core or the node, so it cannot be set against the stall.

## 3. Did a step down follow a stall the core caused?

**No, never.** On production no lower quality was offered and the quality level never changed in
any playback (read from the level in each link-note sample). The one stall came at the start; the
core offers a lower quality only to a playback off its ladder that reported a stall
(`offerFor` in `internal/core/playback/report.go`), and this playback was on its ladder, which
moves by itself (D260 §1). Its own ladder did not move either.

On staging the one step down was device 139 on film 4443, 11-15 08:38:13Z, from level 2 (the
picture copied) to level 0 (`q400k360p`, an encode), with the note "step: ceiling 2 to 0: the
device's data cap, 496 kb/s". No stall came before it. The step itself caused new 425s, on the
`q400k360p` init segment and chunk 9: moving down from a copy to an encode put work on the encoder
and a wait on the player.

What the code would do: `MovingDown.stalled()` (`web/apps/web/src/moving-down.ts`) is told of every
stall `watchForReport` counts, 425 waits included, so a core-caused slow load does count toward
moving down (D247, D253). Nothing in these logs exercised it.

## 4. What else a stall design must handle

- **A film's end looks like a standstill.** Eight of the nine "stood still … the link's to bring
  … 0 chunks arrived" notes came at an episode's end (positions 1570–1729 s, films 1739–1746), about
  40 s after up-next started, about 6 s each. They were not reported as stalls, but the D266 note
  blames the link for them.
- **The first chunk after a seek or resume into a transcode** is the one wait the logs show: it is
  the encoder's first-chunk time, up to 7.7 s on ainas at 1080p, and is the only stall seen.
- **The position of a stall at start is wrong.** The slow-load stall is logged at position 0, the
  position before the resume seek, not the 141 s being loaded.
- **Backgrounded tabs.** Device 34 began playing once with the page hidden and without focus
  (11-16 11:28Z). Device 49's tab, left overnight, had 60 s timeouts on its background refresh at
  08:08Z and 79 slow requests (1.0–2.9 s, median 1.6 s, all over WebRTC from home) on the library
  list in that hour, plus 20 page failures. None was a stall, but a hidden page's waits would count
  as one if it played.
- **Disk spin-up.** Nothing logs it. The slowest first chunk of a copied picture is 4.8 s on
  production and 5.1 s on staging, which a sleeping disk may explain; neither crossed 8 s.
- **Cast receivers.** No cast playback in either span; nothing to measure.
- **Watch Together.** No room in either span.
- **Duplicate starts.** A play page and a film page started a playback at the same moment more than
  once, and 26 of production's 42 leaves were "replaced". A stall design keyed by playback must
  expect one viewer to hold two.
- **Disk pressure.** ainas evicted chunks at 96% disk; a chunk evicted is a chunk made again, and
  a new first-chunk wait.

## Missing fields

Each is named with where it would be logged.

1. **The stall's kind** (rebuffer, slow load, slow seek, Watch Together) and **how long it lasted**:
   `interface Stall` in `web/packages/player/src/stalls.ts` has only `count` and `positionMs`; the
   report's stall fields and the core's "a player reported stalls" line
   (`logReport`, `internal/core/api/report.go`) carry neither.
2. **Whether the wait was the core's** (the 425s) on the stall: `isAwaitingTheCore` is known to
   `tellStalls` but not passed by `watchForReport`, and not sent.
3. **The chunk, stream and HTTP status** being waited on, on the stall line; and the file id or
   playback id, so a stall joins the core's runs without matching by device and time.
4. **The position being loaded**, not the one before the resume seek, in `Stall.positionMs`.
5. **The core's total wait per chunk.** "waited for a chunk not made yet" (`logChunkWait`,
   `internal/core/api/chunks.go`) is debug only, off on production, and one line per ask, so a
   6.8 s wait is seven lines of a few milliseconds each.
6. **Chunk transfer time.** "a request was slow" (`internal/core/api/slow_requests.go`) leaves file
   bytes out, so a slow chunk send is never logged.
7. **The encoder, hardware or software, and the codec** on "a run of chunks finished"
   (`internal/node/agent/chunks.go`); today only the node's start-up line names the encoder.
8. **Per-chunk encode time.** The run line has totals and `firstChunkAfterMs`, not each chunk's.
9. **How many runs were going at once, and the machine's load**, on the run line or a periodic node
   line.
10. **"Cannot keep up" and quality switches** are not sent to the core
    (`followChunkedPlayback`, `web/apps/web/src/ladder-playback.ts`); the switches are read back
    from link-note samples.
11. **A 425 in the steps** is shown only as `fragLoadError`; the steps do not say "too early".
12. **Logs older than the last redeploy.** The compose log is the only copy and is lost at every
    redeploy, so production's history stops at 11-15 20:38Z and staging's at 11-16 13:59Z.
