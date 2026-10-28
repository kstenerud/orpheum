# @orpheum/player

Playback in the browser: hls.js, JASSUB and the subtitle layer, shared by the web app and the
cast receiver. MPL-2.0. It plays a prepared rendition with hls.js where the page has MSE, and
with the element's own HLS where it has only that, as on an iPhone (`src/hls.ts`), and levels a
film's loudness with a gain after the element's volume (`src/gain.ts`, D226).

## Invariants

- **Never imports an app.** `package-not-to-app` (`web/.dependency-cruiser.cjs`).
- **Its play report is the API's own shape**: `PlayReport`, `PlayEvent` and `PlaySpan` are
  derived from the SDK's generated types (`src/report.ts`), so a field the API renames fails
  `tsc -b` here instead of drifting from a hand copy.
- **Does not talk to the server itself**; it is handed URLs the SDK produced.
  `no-restricted-globals` bans `fetch` and `WebSocket` outside `packages/sdk`. (hls.js's
  own requests for media segments are the library's, not ours.)
- **Says no words.** A failure is a code (`PlaybackFailure`, `src/failure.ts`), never English or
  the browser's or hls.js's text; the app puts each in the viewer's words.
  `failure.test.ts`, and the app's `play.test.tsx` for the words.
- **A player playing a file as it is measures its link from how fast its buffer grows**, times
  the file's average bit rate, counting only time the element spends loading, never the pauses
  between loads, and starting afresh at a seek elsewhere; the estimate is the lower of two moving
  averages, so it falls fast and rises slowly (D247 §1; slow-links.md §2). `link.test.ts`.
- **A stall is a wait for data, while playing, of at least `stallAfterMs`, after the player has
  played since its last seek**: a seek's loading, a first load and a brief wait are not
  (D247 §2). `stalls.test.ts`.
- **A chunked playback's every choice of quality is a bitrate controller of our own inside
  hls.js** (`src/abr.ts`, set inside hls.js by `src/abr-controller.ts`, its rules in
  `src/abr-choice.ts`; D265), choosing by its own estimate of the link, the harmonic mean of the
  last five chunks that measured it, each counted once (`src/link-estimate.ts`; hls.js's estimator
  is still fed, for hls.js's own readers; D352 §2): it starts at the quality the server chose, never
  probing the lowest first, whose chunks are made only for a player that needs them (D247 §10);
  while starting, until four chunks measured the link and the video played 20 s, a choice below the
  ceiling is the next chunk's alone, and only a chunk given up or a stall lowers the ceiling, which
  at the end becomes the highest quality the confident estimate (the estimate over one plus its
  largest recent error) carries, no higher than the start (D352 §1); gives up a slow chunk on its
  way, playing or not, for the lowest quality too; takes a share of the link that rises with what
  is loaded ahead; steps up after its steadiness level's hold with `stepUpBufferMs` loaded ahead,
  as far as the confident estimate carries at the level's headroom, several qualities at once only
  at level 0 with samples that agree (D352 §2); raises its level, which doubles the hold, at a climb
  that falls back within twice its hold and at a second drop (a chunk given up or a stall) within
  ten minutes, and relaxes it one level for each ten minutes with neither, each Watch Together
  player its own (`src/steadiness.ts`; D352 §3–§4), starting at the level its offer says, the one
  its device last ended at on its network as the core remembers it (D352 Q2); holds the film to half the link while voice is
  live; and counts neither a chunk from the browser's cache, as its Resource Timing says or, where
  it does not, a load under 10 ms, nor one its pace held back as a measure of the link, and times a
  chunk that does from its first byte to its last, never the wait for its first (D247 §9). It
  reports its confident estimate as the link's speed, never from a lone first sample (D352 §1), and
  its steadiness level, with every play report (D352 Q2); and notes each choice with what it rests
  on, and its last five samples in every stall's and seek's report, in their own `linkSamples`
  beside the steps (D352 §5). A quality whose video, asked to play, stands still while its chunks arrive is
  given up as one the device cannot play, with every quality above it, and reported (D266).
  `abr-choice.test.ts`, `abr.test.ts`, `abr-trust.test.ts`, `abr-measure.test.ts`,
  `cache-hit.test.ts`, `link-estimate.test.ts`, `steadiness.test.ts`, `step-up.test.ts`,
  `diagnosis.test.ts`, `chunked.test.ts`, `report.test.ts`.
- **A chunked playback keeps its sound's place as its quality moves between audio groups**: the
  top of a film's ladder plays its own sound in a group of its own, every group in one order
  (D368 §3), and the rendition playing, or chosen, is set again in each new group before hls.js
  matches one by name and channels (`keepAudioRendition`, `src/hls.ts`; idiosyncrasies.md).
  `hls.test.ts`.
- **A player that is not a follower, alone, a cast's receiver or a session's leader, starts, and
  plays on after a seek, only once what it loaded ahead, picture and sound both, outlasts its next
  chunk's expected load by `ready_ahead_of_next_chunk_percent` (150 to start)** (the owner,
  2026-10-28): a chunked playback tells the SDK how long its next chunk is expected to take, the
  chunk after what is loaded on the quality hls.js loads, its byte range's size, else its length at
  that quality's average rate, over its link's confident estimate, or before one the lower of the
  link its offer says its device last measured on its network (D257 §1) and the rate its first
  chunk of picture loaded at, never that first chunk alone (D352 §1: a Fire TV's first chunk at
  28 Mb/s on a 9.6 Mb/s link started it with 1 s loaded, staging, 2026-10-28), each start's and
  seek's wait saying which it rested on (`nextChunkLoadFrom`) (`src/next-chunk.ts`); one that can
  say nothing waits for `refillAheadMs`. The
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
[cut-a.md](../../../docs/contributors/design/plans/cut-a.md) §1, §3;
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The public surface: `src/index.ts`. Levelling's gain: `src/gain.ts`. Measuring the link: `src/link.ts`. Counting stalls: `src/stalls.ts`. Playing chunks: `src/chunked.ts`;
  choosing their quality: `src/abr.ts`, `src/abr-choice.ts`, `src/step-up.ts`; its estimate of the
  link: `src/link-estimate.ts`; its steadiness: `src/steadiness.ts`; its place inside hls.js:
  `src/abr-controller.ts`; how long its next chunk is expected to take: `src/next-chunk.ts`. The
  frames a video showed and dropped: `src/frames.ts`. HLS playback:
  `src/hls.ts`. The play events a player reports: `src/report.ts`. The subtitle layer drawn
  over the video: `src/subtitles.ts`. Keeping the screen awake: `src/wake.ts`.
