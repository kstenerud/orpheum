> **ABOUTME:** The chain of requests from Play to a film's first frame, after the measurements of
> 2026-11-15 on the Fire TV and the Pixel (the owner's morning question 6, D502). Its first section
> is D502 §1: why the master playlist's first byte is slow over WebRTC on the Fire TV. It records
> what was measured and read; what was built on it is D502's.

# A film's start chain, 2026-11-16

The measurements (taken 2026-11-15, the owner's files, `measurements-2026-11-16.md` §1) timed each
request from the click on **Play** to the first frame, three runs a film on each device. Their
finding 3: the master playlist took 255–420 ms to its first byte over WebRTC on the Fire TV, where
the variant playlists (29–106 KB) took 36–99 ms and the same core answered the master to the Pixel
in 65–116 ms. Its finding 4: the start's first frame waited for picture chunk 1, which hls.js asks
for only once chunk 0 is appended.

## 1. The master playlist's slow first byte (D502 §1)

**It is not the core's work.** The time goes before the core is reached, or after it answers,
during the first round trip the Fire TV's page makes after the click.

### What the core does for the master

`GET …/chunks/<token>/master.m3u8` (`getChunksMasterPlaylist`, `internal/core/api/chunks.go`) finds
the playback's chunks, writes the master playlist and the ladder from the film's cut (its keyframes
and bytes in each second, read from the store and unmarshalled, then the grid), and keeps both on
the playback. All of it is in the core's own memory and SQLite: nothing waits on the media node,
FFmpeg or another request. It reads the cut twice (the master, then the ladder), where a variant
playlist reads it once.

Timed locally (a throwaway test over the API's harness, the fake node, a two-hour film's timeline
with a keyframe every 2 s, 50 asks each):

| path | bytes | first ask | median | slowest |
|---|---|---|---|---|
| `master.m3u8` | 834 | 5.4 ms | 7.0 ms | 32 ms |
| `picture/stream.m3u8` | 162,143 | 4.0 ms | 4.5 ms | 6.8 ms |
| `sound0-96k-raw/stream.m3u8` | 90,143 | 5.4 ms | 4.3 ms | 6.5 ms |

The master costs the core 1–3 ms more than a variant: two orders of magnitude short of the 200–350
ms the Fire TV's master took beyond its variants. Staging's NUC is slower than this machine, but the
Pixel's 65–116 ms through the same core bounds the core's share from above.

### What the Fire TV's runs show

- **A request with no work at all was as slow.** The `DELETE /interest/page` the film page's loader
  sends as the Play page opens (asked at 73 ms in the laddered run in full) only drops the page's
  interest in the core's memory (`withdrawPageInterest`), yet took 380–780 ms. In that run it
  answered at 494 ms and the master, asked 240 ms after it, at 614 ms: the two answers came within
  120 ms of each other, whatever each had asked of the core.
- **Requests asked later are fast.** The variant playlists, asked at 655 ms, answered 39–46 ms
  later; init at 163 ms, chunk 0's headers at 172 ms.
- **The Pixel, through the same core and transport, answers its master in 65–116 ms.** Its film
  page preloads (finding 6), so its media connection was carrying data seconds before the click;
  the Fire TV's had carried nothing since the decision, 9 s earlier.

Staging's core log holds no time per request (the core logs no access line for a playlist), and
holds only the hours since its last restart (22:11 on 2026-11-16): the 2026-11-15 runs are not in
it, and nothing in it times a master.

### The cause, as far as the browser and the code show

What the first requests after the click share, and the later ones do not, is the moment: the Fire
TV's page is drawing the Play page and starting hls.js (finding 2, 300–630 ms of the page's own work
before the first request). Over WebRTC every request a media element or hls.js makes goes to the
service worker, which hands it to the page, whose loader sends it on a data channel of its own
(D273 §8) and hands the answer back: two hops through the page's main thread on each request,
neither of which runs while the thread is drawing. A second cause adds to it on a link idle for
9 s: the Wi-Fi radio's power save and the connection's first packets after a silence. Which of the
two takes more is not visible from the browser; a run with the core's receive and answer times
beside the page's (a debug access line on the WebRTC path) would separate them.

### What it means for D502

- **D502 §3 (a) removes the master's round trip rather than moving work into the decision**: the
  decision is the film page's, 9 s before the click, so its playlists are already in the page when
  Play is pressed. The core's 5–7 ms move with them, a cost no one waits on.
- The first request the start then makes, init and chunk 0, may meet the same slow first round trip
  instead of the master: (a) saves the playlists' 85–156 ms after the master and the master's own
  core time with certainty, and the master's 255–420 ms only as far as the delay is the request's
  and not the page's.
- **D502 §2 (b)** asks for chunk 1 with chunk 0, which removes finding 4's wait whichever cause the
  first round trip has.
