> **ABOUTME:** Why a viewer on an iPhone, on a link fast enough for her quality, saw a still frame
> every few seconds in Watch Together: what Safari does with hls.js, how other sync products treat
> a struggling member, and how other servers make segments on request. Ends in one ranked list of
> changes, logging first.

# Playback in practice

Researched **2026-10-06**, after the owner's staging test that day, in three parts run in
parallel: the iPhone's player, keeping a struggling member in sync, and making chunks on request.
Sources are primary where one exists: the source code of WebKit, hls.js (1.7.3 as installed, and
`3a2e7374`), Jellyfin (`00bb05e`), jellyfin-web (`dd6d1d6`), Syncplay (`2af7ace`), SyncLounge
(`322159e`), the 2015 Netflix Party (`4e4aceb`), Kyoo (`97093907`), python-plexapi and
UnicornTranscoder, read from clones; Apple's SDK headers, specifications and WWDC sessions;
WebKit's release notes; papers; and issue threads. Orpheum was read at `7e54916c`. Plex and Emby
are closed, so claims about their internals come from third parties and say so. A claim marked
*(inferred)* is our reading, not a source's words. It builds on [slow-links.md](slow-links.md) and
[watch-together.md](watch-together.md) and does not repeat them.

## 0. What happened, and what the research changes

The owner hosted from desktop Firefox. The viewer watched on an iPhone (iOS 26, Safari 26.6.1),
at an airport, through iCloud Private Relay, on a link measured at 2.4–3 Mbit/s. At 0:14 she was
moved to the 1.2 Mbit/s 480p chunked quality, and the node made its chunks (150 in about 41 s).
She still saw a still frame every few seconds and the film jumping constantly: "effectively
unwatchable". Her session socket dropped and reconnected twice in two minutes. For part of the
session the sandbox running staging was also loaded by the agent's own test gates (load 11–12 on
8 CPUs).

What our code does, verified: the follower leaves drift up to 60 ms alone, nudges `playbackRate`
between 0.5× and 2× for 1 s, seeks beyond 3 s, and checks every second
(`web/packages/sdk/src/together-follow.ts`). A catch-up jumps ahead up to 30 s and waits there. A
chunk is "covered" by any run that will make it within its 5-minute span
(`internal/core/chunks/runs.go` `plan`/`isCoveredLocked`, `ensure.go` `runSpanMs`). The core holds
a chunk request up to 20 s (`internal/core/api/chunks.go` `chunkWaitAtMost`) before answering 425.

Four findings, each detailed below:

1. **The leading suspicion is our own follower's corrections on Safari, not her bandwidth.**
   *(inferred)* 2.4–3 Mbit/s carries a 1.2 Mbit/s quality with room to spare. On iOS 26,
   `currentTime` is coarse and can be stale right after a rate change, and a rate change or seek
   takes 100–500 ms to land (§1). A 60 ms dead-band is far inside that noise. Each correction that
   ends in a seek outside the buffer costs seconds of frozen picture on 10 s chunks. Firefox,
   with an accurate `currentTime`, never enters that loop. This stays inferred until the
   per-device logging confirms it: commit `8f21be47` now logs each player's start and stop of
   playing and buffering.
2. **Every sync product that corrects tightly hears "stutter" and "jumps back and forth"**, and
   the two with the longest history moved towards tolerance (§2). Jellyfin turned drift
   correction off, first on mobile.
3. **Our server can make a far seek wait for an old run, and its 20 s hold makes hls.js think the
   link is slow** (§3). Every other server starts a new encoder past a distance.
4. **Private Relay adds latency and changes IP**, but is not shown to be the main cause (§4).

## 1. The iPhone's player

**Which player ran.** `hlsMethod()` (`web/packages/player/src/hls.ts:36-41`) picks hls.js
wherever `Hls.isSupported()`. In hls.js 1.7 that accepts ManagedMediaSource (MMS)
(`src/utils/mediasource-helper.ts:4-9`), which iOS has from 17.1. So the iPhone ran hls.js on
MMS, not Safari's native HLS.

**MMS decides when hls.js loads.** WebKit's `ManagedMediaSource.cpp` (main branch) turns
`streaming` false once 30 s ahead of `currentTime` is buffered, and true again once less than
10 s is (`UnifiedWebPreferences.yaml`: `ManagedMediaSourceLowThreshold: 10`,
`ManagedMediaSourceHighThreshold: 30`). These are embedder preferences, so iOS could set others.
A separate timer can turn streaming off 30 s after it starts ("Disabling streaming due to
policy"). hls.js 1.7.3 answers `endstreaming` with `pauseBuffering()` and `startstreaming` with
`resumeBuffering()` (`src/controller/buffer-controller.ts:475-493`;
[PR #6186](https://github.com/video-dev/hls.js/pull/6186)), which overrides `maxBufferLength`
([#6181](https://github.com/video-dev/hls.js/issues/6181)). The spec lets the browser evict MMS
content "for any reason"
([MSE editor's draft 2026-08-07, §7](https://w3c.github.io/media-source/#managedmediasource-interface));
hls.js turns that into `BUFFER_FLUSHED` ([PR #5915](https://github.com/video-dev/hls.js/pull/5915)).

- Our `waitAheadAtMostMs` of 30 s sits exactly at MMS's ceiling. A wait-ahead target past 30 s
  cannot fill. *(inferred)*
- A load-speed sample taken while Safari has paused loading reads as a dead link. The sampler must
  count `hls.bufferingEnabled === false` as "not loading". *(inferred)*

**Known MMS bugs** are handled by hls.js 1.7.3 or probably absent on 26.6:

- Seeking to an unbuffered position while `streaming` is false stalls for good
  ([WebKit 321919](https://bugs.webkit.org/show_bug.cgi?id=321919), a regression filed
  2026-08-17). hls.js names it "macOS and iOS 27 beta", and 1.7.1+ works around it
  ([#7984](https://github.com/video-dev/hls.js/pull/7984)).
- A cold-start failure after Safari is force-quit
  ([WebKit 305712](https://bugs.webkit.org/show_bug.cgi?id=305712), iOS 26.2) is recovered by
  hls.js 1.7.0 ([#7697](https://github.com/video-dev/hls.js/pull/7697)).
- Variant segments whose durations differ slightly (4.000 vs 4.004 s) broke Safari's decoder at a
  start switch ([#6441](https://github.com/video-dev/hls.js/issues/6441)). Our qualities share one
  grid to the microsecond ([slow-links.md](slow-links.md) §10), which helps. *(inferred)*
- hls.js sets `disableRemotePlayback` itself on MMS (`buffer-controller.ts:328-331`), as WebKit
  requires ([WebKit 17.1](https://webkit.org/blog/14735/webkit-features-in-safari-17-1/)). AirPlay
  is lost unless a `<source>` with the m3u8 is added
  ([#6482](https://github.com/video-dev/hls.js/issues/6482), open).

**Fixed only in Safari 27.0 (released 2026-09-17), so present on her iOS 26.6**
([WebKit features for Safari 27.0](https://webkit.org/blog/18325/webkit-features-for-safari-27-0/)):

- `currentTime` "updating only at fixed intervals" (170115677), and updating too rarely on iOS
  (175774587).
- `currentTime` "returning a stale value after the playback rate was changed from zero to a
  non-zero value" (177046564). Every `play()` after a paused wait is such a change.
- `timeupdate` fired during a seek before it completes (176861767). Safari has also been seen to
  fire `seeked` early ([#7583](https://github.com/video-dev/hls.js/issues/7583), 2025-10-22).
- MSE `readyState` not updated at once when a gap stalls playback (176330683), and a gap tolerance
  between buffered ranges loosened (180439090). WebKit main now tolerates 125 ms, or 250 ms where
  audio covers the gap (`MediaSourcePrivate.h`).

WebKit's MSE player sets its rate to 0 until every audio and video renderer has samples
([commit 4aa58c3](https://github.com/WebKit/WebKit/commit/4aa58c3908a0aa119de1ca6c1712154db4c6d515),
2024-09-01). With separate audio and video SourceBuffers, a hole in either stops both.

**Rate and seek on Safari.**

- A team syncing several players reports "a nondeterministic rate change and seek latency" in
  Safari, ending "100ms to 500ms from the ideal", against "at most 2 frames" in Chrome and Firefox
  ([hls.js #7616](https://github.com/video-dev/hls.js/issues/7616), 2025-11). The maintainer:
  rate and seek corrections "should only be used on muted elements".
- On iOS 13 native HLS, "each time the playback rate changes, there is a stutter", 50–100 ms
  ([shaka #2823](https://github.com/shaka-project/shaka-player/issues/2823), 2020, closed as
  external). Whether iOS 26 still does it is not confirmed.
- Jellyfin's own code says "SpeedToSync is failing on Safari"
  (`jellyfin-web/src/plugins/syncPlay/core/PlaybackCore.js:555-557`).

**What that means for our follower.** *(inferred)*

- After each `play()` the next drift sample can read a stale position. That looks like a large
  drift and triggers a seek, which ends in another paused wait, and the cycle repeats itself.
- A seek inside the buffer costs a decode from the preceding keyframe. A seek outside it makes
  hls.js load the whole fragment holding the target before showing anything. A 10 s chunk at
  1.2 Mbit/s is about 1.5 MB plus its sound, roughly 5–6 s at 2.4 Mbit/s, before Private Relay's
  round trips and FFmpeg's start-up. A follower that seeks again before that ends shows one frame
  per seek, which matches what she saw.
- A nudge to 2× on a 1.2 Mbit/s quality consumes about 2.4 Mbit/s, her whole link. Nudging a slow
  member faster drains the buffer it meant to protect.
- WebKit lifts the user-gesture restrictions for one element after its first gesture-driven play
  (`HTMLMediaElement.cpp` `removeBehaviorRestrictionsAfterFirstUserGesture()`). A new `<video>`
  element starts restricted again, and in Low Power Mode even muted autoplay is refused
  ([WebKit 2016 video policies](https://webkit.org/blog/6784/new-video-policies-for-ios/)). If we
  create a new element at a quality change, a programmatic resume can fail.

**What others choose on iPhone.**

| Player | iPhone default | Source |
| --- | --- | --- |
| Apple | native: "if all you care about are Apple devices, using HLS instead will likely make more sense" | [WWDC23 10122](https://developer.apple.com/videos/play/wwdc2023/10122/) |
| Jellyfin web | native: `if (browser.iOS) return false`, "hls.js is only in beta" | `src/components/htmlMediaHelper.js:41-49` |
| Mux Player | native on Apple platforms; overriding "is not recommended" | [Mux docs](https://www.mux.com/docs/guides/player-advanced-usage) |
| Video.js / VHS | native on Safari; MMS opt-in, which hit a Safari ≤26.3 stall | [#505](https://github.com/videojs/http-streaming/issues/505), [PR #1601](https://github.com/videojs/http-streaming/pull/1601) |
| hls.js README | hls.js first; its "alternative setup" uses native on modern Safari | [PR #7844](https://github.com/video-dev/hls.js/pull/7844) (2026-05) |
| Shaka Player | MMS: "Prefer MSE HLS over native HLS in Apple platform" since v4.8.0 (2024-04-26) | [#6188](https://github.com/shaka-project/shaka-player/issues/6188) |
| Bitmovin | MMS supported since 8.169.0 (2024-06-17); HLS preference not confirmed | [release notes](https://developer.bitmovin.com/playback/docs/release-notes-web) |

Native would give up our quality ladder and adaptation (`adaptQuality`, `beyondTheLink`,
`estimateKbps`), the chunk-load measurements the follower uses, and control of loading
*(inferred)*. It has its own bug on iOS 26.6.1: a seek made while paused with only metadata
loaded is dropped when playback starts
([video.js PR #9239](https://github.com/videojs/video.js/pull/9239), 2026-09-29, open). Any "join
paused at the group's position" path would hit it.

## 2. Keeping a struggling member in sync

**What our code does besides the follower's thresholds** (from reading it, not from logs):

- Each `group` message that changes the timeline re-bases the expected position on its arrival
  time (`together-connection.ts:149-158`). On a link whose delay swings by hundreds of
  milliseconds, each looks like drift. *(inferred)*
- After the first wait (`internal/core/together/group.go:126-143`), every stall over 3 s ends in
  `#catchUp` (`together-follow.ts:546-553`): a seek ahead of up to 30 s, then a paused wait. A link
  that stalls every minute skips film every minute.
- `Hub.Detach` (`internal/core/together/hub.go:470-492`) moves a playing group to `Waiting`
  whenever a ready player's socket goes, bypassing the stall cooldown in `stalled()`. The host
  waits up to `reloadWait` = 10 s per drop. On reconnect the `welcome` re-bases the viewer onto a
  group that paused, so it seeks back, refills, then seeks forward again. Two drops would be at
  least four jumps for her and two pauses for the host. *(inferred)*
- The client pings every 15 s but uses the pong only for round-trip time
  (`together-connection.ts:163`). Nothing closes a socket that stopped answering. The server gives
  up after 45 s (`internal/core/api/together_socket.go:35-37`). *(inferred)* The phone can miss
  the host's pauses and seeks all that time.

**What others do.** These add to [slow-links.md](slow-links.md) §5.

- **Jellyfin SyncPlay.** Drift correction is **off by default**
  (`PlaybackCore.js:69`). It went off on mobile first, because it "causes constant stuttering"
  (jellyfin-web PR #2946, 2021-09-14), then everywhere (#4972, 2023-11-09, fixed by PR #5003 in
  10.9.0). When enabled, it acts at most every 1.5 s, at 60 ms ≤ drift < 3 s by
  `speed = 1 + diff/1000` (floor 0.2×, no ceiling). Our 60 ms / 3 s figures in
  [watch-together.md](watch-together.md) §1.3 came from these now-disabled defaults.
  `MaxCatchUpOffset` = 60 000 ms (`IGroupStateContext.cs:40`, PR #17797, merged 2026-09-13) lets a
  recovering member rejoin up to 60 s behind without being sought; otherwise tolerance is 500 ms
  (`Group.cs:125`). After 30 s with no start, a client sends `IgnoreWait`
  (`core/QueueCore.js:189-196`). A closed socket leaves the group at once with no grace
  (`SyncPlayManager.cs:455-464`); a silent one is lost after 60 s.
- **Syncplay.** The room follows its slowest member (`server.py:597-608`). It rewinds beyond 4 s
  ahead and slows to 0.95× beyond 1.5 s, until within 0.1 s (`constants.py:72-89`); a user may
  change all three. It says why: "Slowing down due to time difference with {user}". The server
  drops a watcher after 12.5 s of silence, and the client reconnects up to 999 times
  (`client.py:210-214`, `:920-933`). It has no buffering signal.
- **SyncLounge.** A per-user "Sync Flexibility" of 0–10 s, default 3 s
  (`config/defaults.js:19`). Inside it nothing happens. A guest's sync is skipped while it buffers
  (`synclounge/actions.js:347`).
- **Netflix Party (2015).** No group waiting; a member more than 1 s off seeks to the estimate plus
  2 s (`content_script.js:296-297`). The same idea as D247 §4's amendment.
- **Apple SharePlay.** A stall suspends the staller automatically and the group plays on; ending
  the suspension "will attempt to rejoin the group, potentially seeking" (`AVPlayer.h`;
  [AVPlayerPlaybackCoordinator](https://developer.apple.com/documentation/avfoundation/avplayerplaybackcoordinator)).
  A command has a `completionDueDate`, after which the participant should suspend itself. There is
  no stall counting, no cooldown and no lag tolerance: a member who stalls every minute suspends
  and seeks forward every minute, which is the loop she saw. *(inferred)*
- **Watch2Gether** stopped relaying players' own play and pause, because a buffering member made
  everyone "play / pause, stutter"
  ([Sync System Update, 2020-05-28](https://community.w2g.tv/t/sync-system-update/117897)), and
  does not aim for frame accuracy
  ([2019](https://community.w2g.tv/t/tighten-up-the-sync/58010)).
- **Scener** tells a viewer "a few seconds out of sync" to re-launch
  ([FAQ](https://www.scener.com/faq)). Plex, Discord, Disney+, Prime Video and Hulu document no
  buffering policy.

**Complaints.** Tight, continuous correction draws the same words everywhere: Jellyfin's
"stuttery mess" and "skipping backwards and forwards"
([forum](https://forum.jellyfin.org/t-syncplay-issues-can-i-fix-this-with-hardware-upgrades)),
"Auto pause/resume, jumps forward and back" (jellyfin#16241, open), Plex's "rewinds back every
2-5 seconds and gets stuck in a loop"
([thread 751242](https://forums.plex.tv/t/playback-lagging-and-rewinding-consistently-while-using-watch-together/751242)).
Rejoin bugs are common too: a reconnected member dropped by a late `SessionEnded`
(kontell/jellyfin-plugin-syncplayv2 PR #45), no player or a jump back on rejoin (jellyfin-web
#8578, #8579).

**How much asynchrony people notice.**

- Montagud et al. (*Multimedia Systems* 18(6), 2012), summarising Geerts et al., CHI 2011
  ([hdl.handle.net/10251/44824](http://hdl.handle.net/10251/44824)): up to 1 s "might not be
  perceptible" with voice chat; above 2 s "really become annoying". The same text warns that no
  absolute limits can be derived.
- Montagud & Boronat (*NPA* 3(3), 2011): "playout speed variations of up to 25% are often
  unnoticeable".
- RFC 7272 (IDMS) syncs to the most-lagged client; one ahead slows or pauses.
- Our 60 ms dead-band is about 15× tighter than the 1 s voice chatters fail to notice.
  *(inferred)*

**Who lets a member lag, and who gives up.** Jellyfin (60 s after a buffer), SyncLounge (0–10 s per
user) and Syncplay (user thresholds) let a member run behind. Only Jellyfin relaxes sync for
mobile. **Nobody gives up on a member automatically.** D247 §8's "this will not work", with both
numbers, already goes further than any of them.

**What the evidence argues against.** Making the whole group wait more (everyone moved away from
it: Jellyfin's timeouts, Silo, Apple, W2G). Running the group at the slowest member, as Syncplay
does: for two people the host's film would freeze or rewind whenever she stalls.

## 3. Making chunks on request

**What other servers do.**

| Server | Far seek | A chunk not made yet | Segment length |
| --- | --- | --- | --- |
| Jellyfin | kills FFmpeg and restarts at the target if it is behind the encoder or more than `24 / SegmentLength` segments ahead (`DynamicHlsController.cs:1471-1530`) | held, no timeout, polled every 100 ms (`:1915-1987`) | 3 s encoded, 6 s copied (`StreamState.cs:73-104`) |
| Kyoo | new encoder head if more than 60 s of film from the nearest head's *current* position (`stream.go:506-575`) | held up to 60 s, re-checked every 2 s | every source keyframe, no minimum (`keyframes.go:255-261`) |
| Plex (third-party) | a new transcoder at the offset ([scaleplex SEEK.md](https://github.com/Varashi/scaleplex/blob/main/docs/SEEK.md)); UnicornTranscoder restarts beyond 10 chunks | UnicornTranscoder holds 10 s, then 404 (`utils/chunkStore.js:60-74`) | 8 s in captures |
| Orpheum | a chunk is covered by any run that will reach it within 5 min, however far ahead (`runs.go:84-91`) | held up to 20 s, polled every 250 ms, then 425 `Retry-After: 1` (`chunks.go:39-41,131-160`) | the source's keyframes, at least 2 s, no maximum (`grid.go`) |

None of them waits for an existing encoder to cross minutes of film, and none answers 425. Each
kills unwanted encoders: Jellyfin on restart, Kyoo 20 segments from any viewer
(`tracker.go:272-290`), Plex by throttling. Orpheum does neither: a run is not yet cancelled when
nobody is coming ([chunks.md](../plans/chunks.md) §5), and play-driven runs never wait for a slot
(`internal/node/agent/doc.go:18-23`, D240 §6). Jellyfin and Plex expose how far the encoder has
got (`TranscodingPositionTicks`; `maxOffsetAvailable`), for display only; no one feeds it into
the player's seek target. Our `runTracker.madeThrough` already knows it.

*(inferred)* A follower that seeks 3 min ahead into a 5-min run that is 30 s in waits for an
encoder at 5–10× real time to cover 15–35 s of encoding, beyond the 20 s hold.

**The hold teaches hls.js the link is slow.** hls.js samples bandwidth as
`total − min(ttfb, ttfbEstimate)` (`abr-controller.ts:534-541`), and its TTFB estimator gives a
15 s sample almost no weight (`ewma-bandwidth-estimator.ts:62-68`). So a 15 s hold counts as 15 s
of transfer, and the estimate collapses. After half a chunk's duration with no bytes, it may abort
and switch down (`abr-controller.ts:297-410`). A 425 retry takes no sample. *(inferred)* The
cascade is: seek, hold, down-switch, a new run on another store, another hold, the floor, then the
slow climb back (D247 §9). Each step leaves an x264 run going, none cancelled. That fits staging's
load of 11–12 on 8 CPUs, part of which was the gates. Not confirmed from logs.

**Other players and a 4xx.** Apple asks for 404 for a temporarily unavailable resource; 425 is not
in its list ([WWDC17 514](https://asciiwwdc.com/2017/sessions/514)). Media3's HTTP timeouts are
8 s, and a 425 is retried with growing delay (`DefaultLoadErrorHandlingPolicy.java:37-46,128-161`).

**Segment length.** Apple's authoring spec: IDRs every 2 s (1.13), target 6 s (7.5–7.6), at most
0.5 s over target (7.7). RFC 8216 §6.2.4 requires the same target duration in every variant, but
**not aligned boundaries**, so an encoded rung may be cut finer than the copied picture. An
unaligned switch was not tested. Bitmovin measured 2–3 s best on persistent connections, 6 s on
non-persistent ([blog](https://bitmovin.com/blog/mpeg-dash-hls-segment-length/)). *(inferred)* A
seek lands mid-chunk and hls.js loads the whole chunk, so on 10 s chunks about half a chunk
(≈0.9 MB, ≈3 s of a 2.4 Mbit/s link) is wasted per seek; and hls.js reacts to a long fragment
only after half its duration. Each short chunk pays a round trip, which Private Relay lengthens, so
on that path 4 s is a better floor than 2 s *(inferred)*.

**A ladder for 1–3 Mbit/s.** Apple: "For cellular delivery, the default video variant SHOULD be
the 730 kbit/s" (1.32b); stereo AAC 32–160 kbit/s (2.9). Our sound is 192 kbit/s
(`internal/node/chunks/arguments.go:21`). Kyoo's ladder: 360p 800k, 480p 1.2M
(`videoquality.go:44-90`). *(inferred)* Our 1.2M rung costs about 1.43 Mbit/s on average and
2.0 at peak with sound; on 2.4–3 Mbit/s through Private Relay that leaves no margin at peaks. The
core chose it because its `BANDWIDTH` (1.99 Mbit/s) fitted 70% of a ~2.9 Mbit/s measurement
(`stall.go:15`). hls.js chooses by `BANDWIDTH` unless the buffer is two fragments deep
(`abr-controller.ts:969-975`).

**Encoder.** `veryfast` is Plex's and Jellyfin's default; Kyoo uses `fast`. The chunk delay is not
the encoder's lookahead but FFmpeg's start-up and source seek, encoding a whole chunk, and the
core's 250 ms poll *(inferred)*. Uncapped, each run spawns about 1.5× the cores in threads. Jellyfin
does not trust `-force_key_frames` on QSV (`EncodingHelper.cs:2012-2070`); ours relies on it with
`-forced_idr 1`. No reliable figure for x264 at 480p on an N100 was found, and slow-links.md §4's
"x264 ≈26 fps 1080p" could not be found again on its page.

**A possible bug in the copied picture.** Keyframe times are rounded to the nearest ms
(`readthrough/timeline.go:114`) and the run seeks to that ms (`arguments.go:86`). When rounding
goes down (10.135125 s stored as 10135 ms), a stream copy may start at the previous keyframe, and
`startTicks` (`maker.go:273-281`) would stamp it at the bound: a jump back at a run start. Kyoo
seeks to the midpoint between keyframes (`stream.go:192-205`). Encoded rungs are not affected, so
her 1.2M picture was not *(inferred)*.

## 4. iCloud Private Relay and mobile networks

- Safari reaches Apple's ingress over QUIC (MASQUE) on UDP 443, falling back to HTTP/2 over TCP;
  an egress run by Akamai, Cloudflare or Fastly connects to our server
  ([Apple: prepare your network](https://developer.apple.com/support/prepare-your-network-for-icloud-private-relay/)).
- Throughput fell by up to 10× in speed tests, and "the choice of egress node has paramount
  implications on the achieved throughput, and this choice is not under the user's control"
  (Trevisan, Drago, Schmitt, Bronzino, *Measuring the Performance of iCloud Private Relay*,
  [PDF](https://par.nsf.gov/servlets/purl/10483619);
  [PAM 2023](https://link.springer.com/chapter/10.1007/978-3-031-28486-1_1)). Page loads
  were 60% longer. Responsiveness under load fell from 56–86 to 4–5 RPM on two of three Macs
  ([Eclectic Light, 2023-12-11](https://eclecticlight.co/2023/12/11/does-icloud-private-relay-reduce-network-responsiveness/)).
- *(inferred)* At 2–3 Mbit/s those ceilings do not apply. What matters is added latency, rising
  under load, on every chunk request, twice per chunk (sound and picture). Whether her limit came
  from the relay or the access link is unknown; turning the relay off for the site ("Show IP
  Address") is the cheap test.
- **IP changes.** Apple says the relay IP "will remain stable during a browsing session";
  Foliovision found it "changes even for a single website pageload"
  ([2025-01-17](https://foliovision.com/2025/01/icloud-private-relay-secure-streaming)). Nothing
  may be tied to the client's IP (also [slow-links.md](slow-links.md) §7.4).
- **WebSockets.** The two research parts disagree. One found iOS 16.4–17 beta WebSockets closing
  with 1006 after 5–6 minutes under Private Relay, suspected IP change
  ([WebKit 259743](https://bugs.webkit.org/show_bug.cgi?id=259743), 2023, resolved MOVED, "fixed at
  the server side"), and no newer report. The other found no source beyond anecdotes during relay
  outages ([Seafile forum](https://forum.seafile.com/t/apple-icloud-safari-private-relay-causes-socket-timeouts/15352)),
  and thinks the airport network's flapping the likelier cause of her drops *(inferred)*. Either
  way the socket should reconnect and resume quietly.
- WebRTC media is not carried by Private Relay (Trevisan et al.), which matters for voice chat.

## 5. Where this meets our design

Merged across the three parts and ranked by likely impact, logging first. "Tuning" changes a
number or check the code already marks "to be measured"; "design" amends a decision and needs the
owner ([D246](../../decisions/working-notes.md#d246--what-a-person-may-do-is-their-roles-an-invite-names-who-it-is-for-and-a-watch-together-viewer-is-an-ephemeral-account-that-sees-only-the-hosts-playback),
[D247](../../decisions/working-notes.md#d247--a-slow-link-gets-a-lower-version-made-at-its-first-stall-and-played-from-its-second-and-a-group-waits-for-a-stalled-viewer-once-then-lets-them-catch-up-alone)).

| Rank | Recommendation | Kind | Evidence |
| --- | --- | --- | --- |
| 1 | **Log each player's corrections**: kind, drift, `currentTime` and `requestVideoFrameCallback` `mediaTime`; `seeking`/`seeked`, `waiting`, `readyState`; each seek's from, to and why; hls.js `BUFFER_FLUSHED`, `bufferingEnabled` changes, `BUFFER_STALLED_ERROR`/`BUFFER_NUDGE_ON_STALL`/`BUFFER_SEEK_OVER_HOLE`; fragment TTFB and load time, sound and picture apart; level switches with the bandwidth estimate; reconnects with their gap; time waiting ahead. Builds on `8f21be47`. | logging | Today "jumps constantly" cannot be split between nudges, catch-up seeks and reconnect re-basing (§2) |
| 2 | **Log the core and node side**: per chunk request its hold time, whether it ended in 425, its store, and the covering run's `madeThrough` against the wanted chunk; per run its start, first-chunk latency, encode speed, the node's concurrent runs and load, and why it ended. | logging | Would confirm or refute the down-switch cascade and the pile-up (§3) |
| 3 | **Don't trust `currentTime` on WebKit < 27 right after `play()`, a seek or a rate change**: skip drift samples for a measured while (roughly 0.5–1 s), or read `requestVideoFrameCallback`; act only on drift that persists over two or more samples. | tuning, within D247 §4 | Safari 27.0 notes 170115677, 177046564; #7616 (§1) |
| 4 | **Correct small drift far less**: dead-band from 60 ms to about 250–500 ms (or per browser, measured in the device lab); nudges capped near ±10% over several seconds, never above 1× when the link is close to the bitrate, and none on iOS until measured; no correction for some seconds after a stall, seek or reconnect; smooth the expected position against message-arrival jitter, using D66's minimum RTT. | tuning, overriding watch-together.md §1.3's Jellyfin-derived figures | Jellyfin PR #2946, #4972; #7616; shaka #2823; Montagud's 1 s / 25% (§1, §2) |
| 5 | **Make catch-up seeks rare and land them where data is**: one in flight at a time, backing off (Silo's 10→60 s, [slow-links.md](slow-links.md) §5); aim at a chunk start or inside a buffered range; no correction while a member is loading. | tuning, within D247 §4 and its 2026-10-06 amendment | Each miss costs ~5 s on 10 s chunks at 2.4 Mbit/s (§1) |
| 6 | **Detect a dead socket on the client**: close and reconnect when no pong or message arrives within about two ping intervals; ping faster while playing (Syncplay: 12.5 s). | fix, no decision | `together-connection.ts:163`; server waits 45 s (§2) |
| 7 | **A drop is not a group event**: keep the group playing and mark the member reconnecting for a grace period (say 30–60 s), or at least apply the stall cooldown `Hub.Detach` bypasses; on rejoin, leave a device within tolerance where it is and apply only the commands it missed. | design: D247 §4 (and watch-together.md's "a drop is a state") | `hub.go:470-492`; kontell PR #45, jellyfin-web #8578, #8579 (§2) |
| 8 | **Stop the hold teaching hls.js the link is slow**: shorten the 20 s hold to about a chunk's usual time (2–4 s) and answer 425 sooner; in the player, reset or re-sample the estimate after a 425 or a long TTFB, or keep the core's chosen level until the first chunk arrives. | design, core and player: D247 §10, [chunks.md](../plans/chunks.md) §3–§4 | `abr-controller.ts:534-541`, `ewma-bandwidth-estimator.ts:62-68` (§3) |
| 9 | **Start a new run when the wanted chunk is far ahead of the covering run's current position** (within hold × encode speed, or a fixed 20–30 s of film), and cut the older run back where the new one begins. | design (small): D247 §10, chunks.md §3 | Jellyfin 24 s, Kyoo 60 s, Plex ~10 chunks; `runs.go:84-91` (§3) |
| 10 | **Cancel runs nobody is coming to** (about 20 chunks or 2 min from every player's last request, as Kyoo) and **bound concurrent play-driven encodes per node**, with an x264 `-threads` cap per run. | design: chunks.md §5's unbuilt step, D240 §6 | Kyoo `tracker.go:272-290`; staging load 11–12 on 8 CPUs (§3) |
| 11 | **Let a member who keeps stalling lag behind instead of jumping**, up to a bound (Jellyfin: 60 s), closing the gap gently (≤1.1–1.25×) or at the next host pause, showing the lag with a "catch up" control; optionally a per-member choice between "keep up (may skip)" and "smooth (may fall behind)". Lag against skips with voice chat is the owner's question to weigh. | design: D247 §4 and its amendment; answers D246 §9's worry another way | Jellyfin PR #17797; SyncLounge flexibility; Geerts via Montagud (§2) |
| 12 | **A "struggling" state with hysteresis**, for example three stalls or drops in ten minutes: no small corrections, item 11's lag allowance, the step-down D247 already plans, and a note to her and the host ("watching 14 s behind"); left only after minutes without trouble. | design: extends D247 §4 and §8 | No product surveyed has it; Apple handles one stall well and a repeating one badly (§2) *(inferred)* |
| 13 | **Cut encoded rungs finer than the source's GOPs**: about 4 s (2 s was suggested for the low rung; 4 s suits Private Relay's round trips), forced IDRs, every playlist the same `TARGETDURATION`. The copied picture keeps the coarse grid, or is left out of a slow-link playlist. Needs one hls.js test of an unaligned switch. | design: D247 §10, chunks.md §1 and §6 | RFC 8216 §6.2.4; Apple 7.5; Bitmovin (§3) |
| 14 | **Ladder**: add a ~400k@360p floor; choose 730k rather than 1.2M below about 3 Mbit/s, by the rung's average plus sound against 50–60% of the link, or its peak against 70% of a conservative estimate; make 1.5× `BANDWIDTH` true per chunk through `-maxrate`/`-bufsize`. | tuning: D247 §6's setting and `linkShareForQuality` | Apple 1.32b, 1.27; Kyoo's ladder (§3) |
| 15 | **Sound at 128k (or 96k) AAC-LC** in chunked playback: one constant (`arguments.go:21`) and the core's `soundBitRateBps`. | tuning | Apple 2.9 (§3) |
| 16 | **When a quality starts at a position, make the one below there too**: one run encoding two rungs from one decode, or q−1 queued at lower priority, so hls.js's own down-switch lands on chunks that exist. | design: D247 §7, §10 | The cascade in §3 *(inferred)* |
| 17 | **Respect MMS's window on iPhone**: keep every wait-ahead and refill target at or under 30 s, and count `hls.bufferingEnabled === false` as "not loading" in the load sampler. | tuning, within D247 §4–§5 | `ManagedMediaSource.cpp`, `buffer-controller.ts:475-493` (§1) |
| 18 | **Try native HLS on her iPhone once, as an A/B test against the same quality, not as a switch.** Native must `play()` before seeking, or wait for `loadeddata`. | experiment; adopting it would amend D247 §3 and §10 | Apple, Jellyfin, Mux, Video.js choose native; Shaka, Bitmovin MMS; video.js PR #9239 (§1) |
| 19 | **Keep one `<video>` element across quality changes**, handle `NotAllowedError` from `play()` by asking for a tap, and keep hls.js ≥ 1.7.1. | fix, no decision | WebKit gesture rules; #7984, #7697 (§1) |
| 20 | **Test the copied picture's `-ss` rounding** on a keyframe that rounds down; seek to the midpoint between keyframes as Kyoo does. | fix, needs a test | `timeline.go:114`, `arguments.go:86`, `maker.go:273-281` (§3) |
| 21 | **Verify QSV chunks start with an IDR exactly at the grid**, with ffprobe on devnas. | verification | Jellyfin `EncodingHelper.cs:2012-2070` (§3) |
| 22 | **Ask her to test once with Private Relay off for the site**, to learn how much of 2.4–3 Mbit/s was the relay. Key nothing on the client's IP. | experiment | Trevisan et al.; Foliovision (§4) |
| 23 | Lower priority: x264 `faster`/`fast` at ≤480p once devnas's CPU budget is measured; an I-frame playlist (native scrubbing only); an audio-only rung, the owner's call against D247 §8. | tuning / design | §3 |

## 6. Not confirmed

- Which mechanism produced the jumps: follower nudges, catch-up seeks, reconnect re-basing or the
  down-switch cascade. That needs items 1 and 2 of §5.
- iOS's actual MMS thresholds, and what WebKit's `streamingTimerFired` policy does on cellular.
- iOS 26's `currentTime` update interval, and its gap tolerance between buffered ranges.
- Whether WebKit 305712 persists in 26.6.
- Whether iOS 26 still stutters on a `playbackRate` change; the only report is iOS 13.6 (2020).
- Whether Safari exposes HLS alternate audio in `audioTracks`.
- What Plex, Emby, JW Player, YouTube and Netflix choose on iPhone; Bitmovin's HLS preference.
- Whether her limit was the relay or her access link, and whether Private Relay times out
  long-held requests or still breaks WebSockets.
- Safari-native timeouts for held segments.
- The default of SharePlay's `suspensionReasonsThatTriggerWaiting`, and the length of its
  `completionDueDate`.
- Geerts 2011's primary numbers; the 1 s and 2 s come through Montagud's survey.
- The behaviour of Teleparty, Rave, Disney+ GroupWatch, Prime Watch Party and Hulu; Plex's
  buffering policy and the date of thread 751242; the Emby developer quote about a dropped sync
  feature.
- Plex's hold time and segment length (the 8 s and 10 s figures are third-party); Emby's
  `BySegmentRequest`.
- x264 throughput at 480p on devnas, and whether Jellyfin's QSV caution still applies with
  `-forced_idr 1`.
- Whether hls.js switches cleanly between unaligned fragments.
