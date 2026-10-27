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
  player its own (`src/steadiness.ts`; D352 §3–§4); holds the film to half the link while voice is
  live; and counts neither a chunk from the browser's cache, as its Resource Timing says or, where
  it does not, a load under 10 ms, nor one its pace held back as a measure of the link, and times a
  chunk that does from its first byte to its last, never the wait for its first (D247 §9). It
  reports its confident estimate as the link's speed, never from a lone first sample (D352 §1), and
  notes each choice with what it rests on, and its last five samples for every stall's and seek's
  report (D352 §5). A quality whose video, asked to play, stands still while its chunks arrive is
  given up as one the device cannot play, with every quality above it, and reported (D266).
  `abr-choice.test.ts`, `abr.test.ts`, `abr-trust.test.ts`, `abr-measure.test.ts`,
  `cache-hit.test.ts`, `link-estimate.test.ts`, `steadiness.test.ts`, `step-up.test.ts`,
  `diagnosis.test.ts`, `chunked.test.ts`.
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
  `src/abr-controller.ts`. HLS playback:
  `src/hls.ts`. The play events a player reports: `src/report.ts`. The subtitle layer drawn
  over the video: `src/subtitles.ts`. Keeping the screen awake: `src/wake.ts`.
