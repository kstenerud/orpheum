> **ABOUTME:** Why a Watch Together group takes 10 to 20 s to play again after the host jumps, as
> measured on staging on 2026-10-12, set against how SharePlay, Jellyfin SyncPlay (and its v2
> plugin), Plex, Syncplay, the shared-stream products and the adaptive players (hls.js, dash.js,
> Media3, Shaka) start again after a seek. Ends in options ranked by effect and cost, those that
> change the frozen design marked. Feeds the owner's next decision on D247 §5, D250 §3 and the
> chunk grid.

# Seek resync: why a jump takes 10 to 20 seconds, and what others do

Researched **2026-10-12**, after the owner: *"It does take a bit of time to resync after jumping
position. I suppose that's not the end of the world since jumping is rare, however let's do some
research into how others have handled this in the past and take a more holistic look at how we are
solving the problem."* A solo jump, the same day: *"felt a bit slow, but not terrible. I think that
any longer and I start wondering if it's broken."*

Sources were read first-hand where code exists. The clones and pages are cached under
`/yoloai/cache/seek-research/`: hls.js `c407464`, jellyfin `a45d141`, jellyfin-web `e74f66f`,
media3 `8c6678b`, shaka-player `22f1150`, dash.js `7448b87`, syncplay `2af7ace`,
kontell/jellyfin-plugin-syncplayv2 `48b5d79`, and Apple's pages. **[V]** marks what a source says,
**[I]** an inference.

## 1. What happened on staging

Three devices, one host jumping twice (the core's log, 2026-10-12 07:29). The group waits at the
new place ("sync point", D250 §2) until every player says it is ready, at most 20 s (D250 §3); a
player is ready only with 10 s of film loaded ahead (`refillAheadMs`, `together-media.ts`).

| | Jump to 24:21 | Jump to 46:00 |
|---|---|---|
| Mac Firefox, the film's own picture (copied) | ready at 11.0 s | 10.4 s |
| Android Chrome, the film's own picture | 10.4 s | 11.1 s |
| iPhone, 720p encoded from the jump | not ready by 20 s; the group played on without it, ready at ~22 s | 20.0 s, the group's start |
| iPhone's first 720p chunk made | 10.3 s after the jump | 8.8 s |
| 720p encode's speed (sharing the CPU with the sound's) | 0.9× playback | 1.3× |

- **The copied picture's chunks are about 4 MB.** Each 10 s chunk of it loaded in about 3 s at about
  11 Mbit/s (`chunkLoaded loadMs≈3000 loadedKbps≈11000`): the quality's 36 Mbit/s is its peak, and
  that scene averaged about 3.5 Mbit/s. Ten seconds loaded ahead is two to three chunks, one after
  another, since the jump lands inside the first (2.7 s into it, on the second jump): about 10 s.
- **The iPhone waits for an encode.** Its quality is made from the jump, and the first chunk took
  9 to 10 s, the encoder sharing one modest CPU with the sound's encode and the picture's copying;
  ready needs 10 s of film made past that.
- **Players already ready keep loading.** hls.js loads up to 30 s ahead (`loadsAheadAtMostS`,
  `chunked.ts`), and nothing stops it while the group waits, so the ready players keep drawing on
  the home server's upload the late ones need. **[V from our code; its cost here I]**
- **The run-up can need a chunk for 12 ms.** On the first jump an Android phone with no learned lag
  ran up 2 s, from 24:19.529, and the chunk boundary was at 24:19.541: a run of one chunk was
  queued for the 12 ms before it (`queued a run of chunks … firstChunk=143 lastChunk=143`). On an
  encoded quality that is a whole extra encode. A defect of D282, whatever else is chosen.
- **Alone, a start took 6.9 s** (`a playback started playing … playingAfterMs=6886
  linkKbps=10394`), the copied picture with no one to wait for. A solo jump was not logged then; it is
  now (`a player's seek was ready to play`, 2026-10-12).

## 2. What others do

**Apple SharePlay** (AVPlaybackCoordinator). A seek is shared and every device made to wait; *"when
all devices are ready to go again, playback resumes for everyone together"* (WWDC21 10225). Each
command carries a `completionDueDate`: a device that cannot finish by it suspends itself
(`beginSuspension`) rather than hold the group, and rejoins at the group's time when it ends the
suspension. The seek command says `shouldBufferInAnticipationOfPlayback`; the play carries a
`hostClockTime`. How long the deadline is, and how much a device buffers to be ready, Apple does not
publish: AVPlayer waits until it is "likely to keep up". **[V]**

**Jellyfin SyncPlay.** A seek moves the group to `WaitingGroupState`, every member buffering
(`WaitingGroupState.cs:297-331`); once all are ready it plays at `now + max(2 × highest ping,
500 ms)`. Ready is the client's: it seeks, unpauses, and takes the element's first `playing` as
ready, then pauses (`PlaybackCore.js:383-400`): **ready is "the browser could start", a fraction of
a second, not 10 s.** A `waiting` counts as buffering only after 3 s. The client waits 30 s for it.
**[V]** The v2 plugin splits the wait: 30 s for a load that may start a transcode, **10 s for a
stall or a seek within what is loaded** (`EngineTimings.cs:16-22`; its PR #21: *"starting a
transcode routinely takes longer than a rebuffer does"*). Its notes say a transcoded seek *"snaps
to its segment boundary"*, and it keeps the offset it lands at rather than seeking again. **[V]**

**Jellyfin's transcoder at a seek** restarts FFmpeg at the target, segments 3 s encoded and 6 s
copied (playback-in-practice.md §3). Plex starts a new transcoder at the target. Both rely on
hardware encoding to make the first segment fast; neither makes a low ladder ahead. **[V]**

**Plex Watch Together** shows each person "Buffering…" then "Ready", and plays once all are; no seek
policy is published (removed from Plex's new apps in February 2025, kept on the web). **[V, third
party]**

**Syncplay** applies a seek at once with no group wait: its files are local, a seek instant.
**Teleparty, GroupWatch, Prime Watch Party, Kast, Scener, Rave** publish nothing on seeks; each
streams from a CDN per viewer and never meets a home upload. **[V that nothing was found]**

**One shared stream** (Hyperbeam, Discord, a screen share): one encode, sent live to everyone; a seek
is new frames, nothing to wait for per device. Over the internet each viewer still takes its own
copy, so it saves only the encode, not the upload, and everyone gets one quality: the computers
would lose the film's own picture (D123). **[I]**

**Adaptive players after a seek** buffer far less before playing:

| Player | Before playing | After a stall | Where |
|---|---|---|---|
| Media3 (ExoPlayer) | 1 s | 2 s | `DefaultLoadControl.java:90-102` |
| Shaka | none (the browser decides) | same | `player_configuration.js:190` |
| dash.js | none | same | `Settings.js:468` |
| Jellyfin SyncPlay | the element's first `playing` | same | above |
| **Orpheum, in a group** | **10 s** | **10 s** | `together-media.ts` |

hls.js picks its first quality so its first fragment loads within 4 s (`maxStarvationDelay`).
dash.js resets its choice to start-up on every seek, and names our problem as its own open TODO:
*"If e.g. we have 10s fragments and seek, we might want to download the first fragment at a lower
quality to restart playback quickly"* (`BolaRule.js:113-114`). `HTMLMediaElement.fastSeek()` exists
to land on a keyframe quickly. hls.js can start a fragment before it has all arrived
(`progressive`, experimental), but only per moof, and our chunks have one moof per keyframe
(`+frag_keyframe`), so a copied chunk is one moof. **[V]**

**What people wait for.** Viewers start to leave after 2 s of start-up, 5.8% more per second
(Krishnan & Sitaraman, IMC 2012). A low-resolution stand-in shown meanwhile is tolerated longer than
black. No study measures a group's wait after a seek. **[V]** The owner's own bound, alone, is
about where the 6.9 s start was.

## 3. Where Orpheum differs

1. **Ready after a seek is 10 s of film, where every other player starts on a second or two.** Ours
   was set for stalls on a slow link, where a player ready at its first frame stalled again and held
   the group in a loop (the owner's first test, 2026-10-05). A host's seek is not a stall: the link
   has just been shown to keep up.
2. **The jump lands inside a 10 s chunk**, so the first chunk's head is loaded and not played, and
   an encode starts at a chunk's start, before the target.
3. **Ready players compete for the upload** while the late ones load.
4. **The encoded first chunk is 10 s of film**, made at about playback speed on a shared CPU.
5. **The group waits a fixed 20 s for anyone**, where SharePlay and the v2 plugin bound each
   device and let the late one rejoin.

## 4. Options

Effects are estimates from §1 **[I]**; "design" marks what changes a decision.

| # | What | Effect on the jump | Cost and risk | Design |
|---|---|---|---|---|
| 1 | **Stop loading on ready players while the group waits**, then go on | the late players get the whole upload; on a shared upload the last one's chunks come faster | none worth naming | no |
| 2 | **Keep the run-up within the chunk the group starts in** | no extra encode for a 12 ms run-up | none | no (a D282 defect) |
| 3 | **After a host's seek, ready on a few seconds, not 10**: e.g. 3 s loaded when the player's measured load speed is well above its quality's rate, else 10 s | copied-picture devices ready in ~3–4 s instead of ~10 s | a stall just after the start on a link barely fast enough; the stall then is handled as any (D250 §2) | refines the 10 s rule |
| 4 | **Snap the host's seek back to the start of its chunk** (a keyframe) | no wasted head; runs start at the target | the jump lands up to a chunk (10 s) before where the host aimed | sync points |
| 5 | **Show who the group waits for, and how far each is** (Plex's Ready list), and the target's still | shortens nothing; makes a wait read as progress, not as broken | small UI | M5 UI |
| 6 | **A shorter first chunk for an encode started at a seek** (2–4 s), or encoded qualities cut at 4 s | the iPhone's first chunk in ~3 s, not ~9; its 20 s to perhaps 6 | changes the chunk grid (D247 §10); more requests, felt more through Private Relay | grid |
| 7 | **Hardware encoding for runs started at a seek** (VAAPI/QSV where the node has it) | a few seconds off the encoded device's wait | picture and keyframe placement differ by encoder | node |
| 8 | **Each device bounded at a seek** (SharePlay's deadline): a player that predicts it will be late says so at once, the group plays without it, and it catches up | the group waits ~5–10 s at most | that person misses a few seconds | refines D250 §3 |
| 9 | **The first chunk after a seek at a lower quality**, stepping up soon after (dash.js's TODO) | copied-picture devices ready in ~1 s | a soft picture until the step up, which D265 holds for 120 s; adds a transcode unless one is made | ABR |
| 10 | Serve a chunk while it is made, several moofs per chunk, hls.js `progressive` | first frames ~1–2 s after encoding starts | hls.js marks it experimental; untested on Safari's ManagedMediaSource | chunk format |
| — | One shared live stream | saves one encode | the computers lose the film's own picture; no ladder | not recommended |

## 5. Recommendation

1. **Build 1 and 2 now**: neither changes the design, and both remove waste the log shows.
2. **Decide 3 and 4 together**: the two cheapest changes with the largest effect, together taking
   the copied-picture devices from ~10 s to ~3 s, inside the owner's "wondering if it's broken".
   Option 3 matters alone too: a solo jump has no group wait but the same chunk sizes.
3. **Then the encoded device**, which sets the worst case: 6 (or 7 where the node has hardware), and
   8 so one slow device never holds the others the full 20 s.
4. **5 with M5's player UI.**

The first test after any of them reads the new `a player's seek was ready to play` lines for a solo
jump, and the group's `everyoneReady` against its `seeked` for a group one.

## 6. What was decided (2026-10-12)

- **Built:** options 1, 2 and 3 (D283), and, found on the owner's solo jumps the same day, a video
  waiting after a seek for its sound is no longer taken for one that cannot decode its picture (it
  had given the film's own picture up for an encoded 720p), and a seek's log says where it came from.
- **Not taken:** option 4. A seek can only start at a keyframe, and snapping it to its chunk's start
  saves at most that chunk's unused head, about 1 to 2 s at the measured speeds, while it lands up to
  a chunk before where the person aimed.
- **Next, in place of options 6 and 10:** the 10 s chunks are this film's keyframe spacing (its
  picture has one every ~10.4 s, x264's default 250 frames at 24 fps), and the encoded qualities
  share the copied picture's grid with a keyframe only at each chunk's start (`-g 100000
  -force_key_frames`). Keeping the chunks but encoding a keyframe every 2 s, each its own fragment,
  and serving a chunk as it is made would let a device on an encoded quality start on its first 2 s:
  about 9 s to 2 or 3 after a jump, for 3 to 5% more bits. The owner's question to decide next.

