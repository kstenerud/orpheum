# @orpheum/player

Playback in the browser: hls.js, JASSUB and the subtitle layer, shared by the web app and the
cast receiver. MPL-2.0. It plays a prepared rendition with hls.js where the page has MSE, and
with the element's own HLS where it has only that, as on an iPhone (`src/hls.ts`), and levels a
film's loudness with a gain after the element's volume (`src/gain.ts`, D226).

## Invariants

- **A debug tool's cap and drop shape every chunk load of the tab in the player's own fetch**
  (`src/link-shaping.ts`, D380): with a cap, a load's bytes are let through no faster than it, one
  load after another as one link carries them, its stats saying the bytes as they come, so the
  bitrate controller sees a slow link as it would, over WebRTC and HTTP alike; while the tab is
  dropped, each load fails as a lost network's does. Nothing shapes a tab that sets neither.
  `link-shaping.test.ts`.

- **Never imports an app.** `package-not-to-app` (`web/.dependency-cruiser.cjs`).
- **Its play report is the API's own shape**: `PlayReport`, `PlayEvent` and `PlaySpan` are
  derived from the SDK's generated types (`src/report.ts`), so a field the API renames fails
  `tsc -b` here instead of drifting from a hand copy.
- **Does not talk to the server itself**; it is handed URLs the SDK produced.
  `no-restricted-globals` bans `fetch` and `WebSocket` outside `packages/sdk`. (hls.js's
  own requests for media segments are the library's, not ours.)
- **Says no words.** A failure is a code (`PlaybackFailure`, `src/failure.ts`), never English or
  the browser's or hls.js's text; the app puts each in the viewer's words. Beside it goes what
  failed, for the core's log alone, never the viewer (`PlaybackFailed`): hls.js's error type,
  details, whether fatal, its message, the answer's status and what carried the load
  (`failureText`, `src/diagnosis.ts`), where the page's words alone said "the browser gave no reason"
  (staging, 2026-10-29). `failure.test.ts`, `load-again.test.ts`, and the app's `play.test.tsx` for
  the words and `play-failure.test.tsx` for the report.
- **A player playing a file as it is measures its link from how fast its buffer grows**, times
  the file's average bit rate, counting only time the element spends loading, never the pauses
  between loads, and starting afresh at a seek elsewhere; the estimate is the lower of two moving
  averages, so it falls fast and rises slowly (D247 §1; slow-links.md §2). `link.test.ts`.
- **A stall is a wait for data, while playing, of at least `stallAfterMs`, after the player has
  played since its last seek**: a seek's loading, a first load and a brief wait are not
  (D247 §2); and a load that has left it unable to play for each `slowLoadAfterMs`, counted only
  while the wait is the link's: never while it is paused and nothing asks it to play
  (`isAskedToPlay`, @orpheum/sdk: a film page's hidden player is not asked), nor while its chunk
  on its way waits on the core, which answered 425 (`isAwaitingTheCore`). `stalls.test.ts`,
  `chunked.test.ts`.
- **A chunked playback's every choice of quality is a bitrate controller of our own inside
  hls.js** (`src/abr.ts`, set inside hls.js by `src/abr-controller.ts`, its rules in
  `src/abr-choice.ts`; D265), choosing by its own estimate of the link, the centre in log space of
  the last five chunks that measured it, each counted once, Huber's, a sample further than two
  spreads from it counted as if that far, so no one sample rules it (`src/link-estimate.ts`;
  hls.js's estimator is still fed, for hls.js's own readers; D352 §2, revised 2026-10-29), the link
  its device last measured on its network, as its offer says it, counted among them as up to two
  samples, fewer as it ages, until two of its own agree against it or its own fill the five (P2),
  and the spread it reports with its speed kept by the core beside it for the next start: a playback's
  own start on its ladder chooses its first quality before anything loads from that remembered link,
  by the rule every later choice plays by (`steadyLevel`, nothing loaded ahead), so the first chunk is
  of the quality it plays and no switch lands as playback begins (`shouldStartByRememberedLink`; D381,
  a Fire TV blinking black as a low first rung switched to the top at play); a player that remembers
  nothing, a stalled player's lower quality and a start the page chose itself start at the quality
  given (D257 §2), never probing the lowest first, whose chunks are made only for a player that needs
  them (D247 §10), `abr-start.test.ts`;
  while starting, until the video played 20 s and its samples settle the ceiling (it is the same
  with any one left out, P5) or four measured the link, a choice below the ceiling is the next
  chunk's alone, and only a chunk given up or a stall lowers the ceiling, which
  at the end becomes the highest quality the confident estimate carries, no higher than the start
  (D352 §1), the confident estimate being the estimate's lower bound for a choice held over many
  chunks, e^(centre − z × spread / √n), z 1.28 with nothing loaded ahead, 0.84 at 10 s and 0 from
  20 s (`boundDeviationsByAheadS`), where it was the estimate over one plus its largest recent error
  (P4); holds the quality loading against any reading of the link, the
  next chunk's choice and the ceiling alike, while `holdsQualityAheadS` (20 s) is loaded ahead,
  noting the hold (D352's revision of 2026-10-28); gives up a slow chunk on its
  way, playing or not, by that chunk's own rate, for the highest quality below it the link's estimate
  carries, its sample counted in it, never by that one chunk's rate (D382 §1), the lowest too, a sample of the link only while its bytes were
  arriving (within `arrivingWithinMs`, and `measuresFromBytesAtLeast` of them after its first
  arriving piece), over the time from that piece to the give-up, and one stuck, or with only its
  first piece, no sample (D382 §2), and never one whose arrival a long task of the page overlapped
  (the browser's own, of 50 ms and more, where it reports them; `src/abr-busy.ts`), which is no
  sample either, as a loaded one is not, each such chunk noted with the task's length (D382 §3), lowering the ceiling while starting no further than the
  estimate carries, a drop either way, and noted always (P3); takes a share of the link that rises with what
  is loaded ahead; steps up after its steadiness level's hold with `stepUpBufferMs` loaded ahead,
  as far as the confident estimate carries at the level's headroom, several qualities at once only
  at level 0 with samples that agree (D352 §2); raises its level, which doubles the hold, at a climb
  that falls back within twice its hold and at a second drop (a chunk given up or a stall) within
  ten minutes, and relaxes it one level for each ten minutes with neither, each Watch Together
  player its own (`src/steadiness.ts`; D352 §3–§4), starting at the level its offer says, the one
  its device last ended at on its network as the core remembers it (D352 Q2); holds the film to half the link while voice is
  live; and counts neither a chunk from the browser's cache, as its Resource Timing says or, where
  it does not, a load under 10 ms, nor one its pace held back as a measure of the link, and counts a
  chunk that does by its bytes after its first arriving piece, its load's first progress
  (`src/first-piece.ts`), from that piece to its last byte, never the wait for its first (D247 §9):
  a transport that delivers in pieces brings the first with the first byte, and a Fire TV's chunk
  given up with only its first 128 KiB measured a 20 Mb/s link at 680 kb/s (D382 §2). It
  reports its estimate as the link's speed, never a bound of it and never from a lone first sample
  (D352 §1), with its spread, and its steadiness level, with every play report (D352 Q2, P2); and notes each choice with what it rests
  on, and its last five samples in every stall's and seek's report, in their own `linkSamples`
  beside the steps (D352 §5), and tells each switch of quality why it was chosen, with the samples
  then, for a Watch Together player's note (`whyChose`); and with every play report its notes on its
  link since the last, which the core logs (`linkNotes`): its prior, each sample with the time the
  estimate before it expected and its next-chunk bound's, "below" when slower than the bound, so the
  bound's calibration can be read from the log, and each chunk given up or stuck with the transport
  and WebRTC connection that carried it and how long since that connection last delivered bytes
  (P6; the owner, 2026-10-29), and each of its quality steps, as `step: …`, so a solo play's steps
  reach the core's log as a Watch Together player's do (D382 §4). A quality whose video, asked to play, stands still while its chunks arrive is
  given up as one the device cannot play, with every quality above it, and reported (D266); its
  standing still counts only while the video holds `standsWithAheadAtLeastS` (1 s) of what it plays
  ahead, picture and sound, by its own buffered ranges, never hls.js's picture alone, and while its
  loads arrive, afresh at each chunk that fails, sticks or is given up, at each ask to load on after
  hls.js stopped its loading, and while the browser is offline; each quality judged unplayable, and
  each long standing still not judged, is noted with why. A minute's outage stood a video 0.07 s short
  of the end of what it held, hls.js saying 33.9 s loaded ahead, and the top quality was given up for
  good (a local reproduction, 2026-10-29; VQ135). `abr-outage.test.ts`, `abr-choice.test.ts`, `abr.test.ts`, `abr-first-piece.test.ts`, `abr-given-up.test.ts`, `abr-busy.test.ts`, `abr-trust.test.ts`, `abr-measure.test.ts`,
  `cache-hit.test.ts`, `link-estimate.test.ts`, `steadiness.test.ts`, `step-up.test.ts`,
  `diagnosis.test.ts`, `chunked.test.ts`, `report.test.ts`.
- **A chunked playback never waits for good on a chunk stuck on its way**: one the core answered,
  nothing of it arriving for `asksAgainAfterMs` (8 s), is aborted and asked for again at whatever
  quality is chosen next, the lowest too, each time in a row waiting twice as long, up to
  `asksAgainAfterAtMostMs` (16 s), until a chunk loads; it is no sample and lowers nothing, a stuck
  transport being no slow link; each is noted with which time in a row it is, how long it waited
  and what carried it, and each chunk whose load failed or timed out is noted with what carried it.
  One not yet answered is hls.js's to time out, since the core may hold it while it makes it.
  Waited for, a chunk stuck after 19 bytes over WebRTC held the owner's Chrome, throttled and
  unthrottled, for good (staging, 2026-10-29; VQ125). `abr-stuck.test.ts`.
- **A chunked playback never ends at a load that failed**: a fatal hls.js error that only stopped its
  loading, any but the media's own (MEDIA_ERROR, MUX_ERROR) and the core's 429 and 503, which have
  their own answers (`isLoadStopped`, `src/failure.ts`), is marked recovered, so it is no failure,
  and hls.js is asked to load on from where the playback resumes (`resumePositionOf`,
  `src/resume-position.ts`): where the video stands, or, while a rebuild of its decoding has put the
  video at 0 and hls.js has not yet loaded enough to start, where the rebuild resumes from, which hls.js
  seeks the video to once loaded there; asked from 0, a film given up at 68 s started again from its
  beginning when the link came back (a local reproduction, 2026-10-29; VQ135); after the same waits as a chunk stuck
  on its way, in one count of times in a row (`ChunkOnItsWay.askingAgain`), and at once when the
  browser is back online (`loadOnAfterFailures`, `src/load-again.ts`); what is
  loaded plays on meanwhile, the page showing only its wait. Each is noted with what failed and what
  carried it, the time in a row and the wait, and each ask with why then. Taken as the end, a load
  that failed while the owner's Chrome was offline stopped its playback for good, though the link
  came back (staging, 2026-10-29; VQ125). `load-again.test.ts`, `resume-position.test.ts`.
- **A fatal decoding error (hls.js's MEDIA_ERROR) is recovered once before it fails the playback**,
  as hls.js's own guide asks: hls.js's decoding is built anew from where the playback resumes
  (`resumePositionOf`; `recoverMediaError`, `loadAgainFrom`, which also recovers a hole it cannot step over, D266) and
  the error marked recovered, so failOnFatal passes it by (`recoverFromDecodingErrors`,
  `src/decode-recovery.ts`); a second one within `decodingRecoveryWindowMs` (30 s) of the last
  recovery is not recovered again, and fails as today, since the rebuild did not mend it, and
  v1.7.3's guide no longer recommends `hls.swapAudioCodec()` first ("should no longer be required
  and is not recommended"). MUX_ERROR is left to fail as today too: the guide's own sample code has
  no case for it either. Each attempt and its outcome is noted (the owner, 2026-10-29).
  `decode-recovery.test.ts`.
- **A chunked playback keeps its sound's place as its quality moves between audio groups**: the
  top of a film's ladder plays its own sound in a group of its own, every group in one order
  (D368 §3), and the rendition playing, or chosen, is set again in each new group before hls.js
  matches one by name and channels, in the form of the sound loaded, so hls.js keeps the sound
  loaded and loads the new one on from its end; and a sound the person chooses, the dialogue boost
  turned on or off, is the one way a sound changes as the film plays, a prepared rendition's too: the
  sound loaded plays on while the new one's chunk at the playhead is asked for, starting the server
  making it, and the new one is heard from that chunk once it has loaded, each choice noted with its
  cause and again as it is heard; a boost turned on at the top of a film's ladder, the sound loaded
  thrown away, froze it 2.2 s (staging, 2026-10-29) (`keepAudioRendition`, `src/hls.ts`; D368's
  revision of 2026-10-28; idiosyncrasies.md). `hls.test.ts`.
- **A player that is not a follower, alone, a cast's receiver or a session's leader, starts, and
  plays on after a seek, only once what it loaded ahead, picture and sound both, outlasts its next
  chunk's expected load by `ready_ahead_of_next_chunk_percent` (150 to start)** (the owner,
  2026-10-28): a chunked playback tells the SDK how long its next chunk is expected to take, the
  chunk after what is loaded on the quality hls.js loads, its byte range's size, else its length at
  that quality's average rate, over its controller's next-chunk bound, e^(centre − z × spread ×
  √(1 + 1/n)) at what is loaded ahead, plus the median wait of its samples' first bytes: from the
  link its offer says its device last measured on its network (D257 §1) alone before any chunk,
  and blending in each chunk after, never the first chunk alone (D352 §1: a Fire TV's first chunk
  at 28 Mb/s on a 9.6 Mb/s link started it with 1 s loaded, and two samples outvoting a link
  remembered at 17 Mb/s started one on 753 kb/s, staging, 2026-10-28; P5), each start's and seek's
  wait saying which it rested on (`nextChunkLoadFrom`) (`src/next-chunk.ts`); one with nothing
  remembered and nothing measured waits for `refillAheadMs`. The
  wait comes before the `play()`, never as a play, a pause and a play: the page's video has no
  autoplay attribute, and its start gate plays it (the SDK's `start-gate.ts`), pausing a playing
  video as it seeks to where too little is loaded, before it can play there. A Fire TV played at
  canplay with 1.6 s loaded and stalled (idiosyncrasies.md). `next-chunk.test.ts`, the SDK's
  `start-gate.test.ts`, the app's `start-gate-play.test.ts`.
- **Every play report carries the frames the video showed and dropped since the last**, as its
  browser counts them (`src/frames.ts`), none where it counts none; the core logs a report that
  dropped more than 1% of them. `frames.test.ts`.
- **A film's player holds a screen wake lock while its video plays on a shown page**, lets it go
  when the video pauses or ends, asks again when the page is shown, and plays on without one
  where the browser gives none (D287). `wake.test.ts`.
- **A chunked playback keeps `keptBehindS` (30 s) of what it played in the browser's buffer**, not
  hls.js's everything, so a weak device's memory holds no film's played minutes (the efficiency
  audit's finding 5). `chunked.test.ts`.
- **A chunked playback loads ahead `loadsAheadBelowTopS` (30 s) below the top quality it may load
  and `loadsAheadAtTopS` (60 s) at it**, the top being the ladder's below any quality found
  unplayable, never a ceiling of the moment; **no further than a third of the browser's memory for
  its picture** at the quality's average rate (`videoLimitBytes`: Chromium's 150 MiB, 30 MiB on a
  Chromecast or where `navigator.deviceMemory` says 1 GB or less, about 100 MiB in Firefox, 100 MB in
  WebKit), **nor than a third of its memory for its sound** at the sound's own rate, read from its
  chunks as they load (`soundLimitBytes`: Chromium's 12 MiB, 2 MiB on the same weak devices, about
  30 MiB in Firefox, 100 MB in WebKit; a film's own copied 5.1 ran a Fire TV's full twice, staging,
  2026-10-29), but never below 20 s for either, a floor whose bytes the buffer cannot hold being
  noted and left to hls.js's cut; and no further than its Watch Together group's runway
  (D285 §2), hls.js's own cut after the browser refused a chunk for its memory, which nothing undoes,
  a runway lifted included, or what its page says (the film page's preload). hls.js's 60 MB never
  stretches it, as it did to ten minutes of 360p. With each play report its notes on its link say
  the target at each change, with its bytes and what bounds it, each buffer run full with what was
  loaded, the device's memory and the limit assumed, and how long each step up took to show
  (`src/forward-buffer.ts`; forward-buffer-2026-10-29.md P1, P2 and P6, accepted by the owner
  2026-10-29, "Agreed with your recommendations"; its P3, replacing loaded low quality on a step up,
  is not to be built). `forward-buffer.test.ts`, `chunked.test.ts`, `hls.test.ts`,
  `abr-trust.test.ts`.
- **A chunked playback notes each stall of its page's main thread of `mainThreadStallAtLeastMs`
  (1 s) or more**, with its linkNotes, naming the player's last events before it (a seek, a quality
  or sound begun, a buffer flushed or run full), at most `mainThreadStallNotesAtMost` a playback:
  from the browser's long tasks where it reports them (Chromium, Silk), else from a timer's
  lateness while the film plays on a shown page. A Fire TV's page froze about 10 s after a skip
  back, its picture then running fast to catch up, and nothing said so (staging, 2026-10-29;
  VQ121). How a freeze is found is the SDK's `watchFreezes`, the one way for the whole page: the
  web app reports every page's freezes through it too (its `page-freezes.ts`). `main-thread.test.ts`,
  `chunked.test.ts`.
- **A chunked playback's requests carry an Authorization only where one is handed in**, for a page
  with no media cookie on the core's origin: the lab's cast receiver (plans/tv-device-tests.md).
  `chunked.test.ts`.
- **One public entry.** `exports` in `package.json` exposes only `src/index.ts`.
- **One library per job:** hls.js for HLS and JASSUB for ASS/SSA; alternatives are named in
  `no-restricted-imports` (`web/eslint.config.js`).
- **A sound at the level never passes through Web Audio**; one to raise or lower does, once for
  the element's life, with every channel kept. And where a page cannot set the volume, as on iOS,
  no gain is applied, since there Web Audio follows the silent switch (D226 §4): `gainAfterVolume`
  checks the element itself. `gain.test.ts`.
- **A sound through Web Audio is moved earlier by the latency its device remembers until one is
  measured, and a measured latency moves it again only when it differs from the one in effect by
  `soundLatencyChangeIgnoredMs` or more**, which is then kept in the memory the app hands in
  (`SoundLatencyMemory`; the package reads no storage itself), so a device's later playbacks start
  with no sound thrown away. **A latency is read only as the element's sound plays through Web
  Audio** (its `playing`, or a route made while it already plays): one read as the context has just
  started is short of its own, and moved a Fire TV's cast off the remembered latency and back.
  Every move is told the element, from where to where, why, what moved it to where it moved from,
  the latency remembered and whether it threw the sound away (`playerMovedSoundEvent`), for a Watch
  Together player's log (the owner, 2026-10-28; idiosyncrasies.md, the TCL TV's entry).
  `gain-latency.test.ts`.

## Read before changing

[D3](../../../docs/contributors/decisions/working-notes.md), [D11](../../../docs/contributors/decisions/working-notes.md),
[D50](../../../docs/contributors/decisions/working-notes.md), [D189](../../../docs/contributors/decisions/working-notes.md),
[D193](../../../docs/contributors/decisions/working-notes.md) §6;
[D265](../../../docs/contributors/decisions/working-notes.md) and
[D352](../../../docs/contributors/decisions/working-notes.md) for choosing a quality;
[forward-buffer-2026-10-29.md](../../../docs/contributors/design/research/forward-buffer-2026-10-29.md)
for how far ahead it loads;
[cut-a.md](../../../docs/contributors/design/plans/cut-a.md) §1, §3;
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The public surface: `src/index.ts`. Levelling's gain: `src/gain.ts`. Measuring the link: `src/link.ts`. Counting stalls: `src/stalls.ts`. Playing chunks: `src/chunked.ts`;
  choosing their quality: `src/abr.ts`, `src/abr-choice.ts`, `src/step-up.ts`; its estimate of the
  link: `src/link-estimate.ts`; its steadiness: `src/steadiness.ts`; its place inside hls.js:
  `src/abr-controller.ts`; what it reads of its video's position, how long it played and where it
  stood still: `src/abr-video.ts`; the chunk on its way it judges: `src/abr-on-its-way.ts`; how long its next chunk is expected to take: `src/next-chunk.ts`; how far ahead it loads: `src/forward-buffer.ts`; noting its page's main thread's stalls: `src/main-thread.ts`; loading on after a load that failed: `src/load-again.ts`; where a playback resumes after its decoding is rebuilt: `src/resume-position.ts`. The
  frames a video showed and dropped: `src/frames.ts`. HLS playback:
  `src/hls.ts`. The play events a player reports: `src/report.ts`. The subtitle layer drawn
  over the video: `src/subtitles.ts`. Keeping the screen awake: `src/wake.ts`.
