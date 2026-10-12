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
  hls.js** (`src/abr.ts`, its rules in `src/abr-choice.ts`; D265), keeping hls.js's estimator:
  it starts at the quality the server chose, never probing the lowest first, whose chunks are made
  only for a player that needs them (D247 §10); gives up a slow chunk on its way, playing or not,
  for the lowest quality too; takes a share of the measured link that rises with what is loaded
  ahead; steps up only one quality at a time, after `holdBeforeStepUpMs` at its quality, with
  `stepUpBufferMs` loaded ahead and a link `stepUpHeadroom` times the next quality's rate (D247
  §9); holds the film to half the link while voice is live; and counts neither a chunk from the
  browser's cache, as its Resource Timing says or, where it does not, a load under 10 ms, nor one
  its pace held back as a measure of the link. A quality whose video,
  asked to play, stands still while its chunks arrive is given up as one the device cannot play,
  with every quality above it, and reported (D266).
  `abr-choice.test.ts`, `abr.test.ts`, `cache-hit.test.ts`, `step-up.test.ts`.
- **A film's player holds a screen wake lock while its video plays on a shown page**, lets it go
  when the video pauses or ends, asks again when the page is shown, and plays on without one
  where the browser gives none (D287). `wake.test.ts`.
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
[cut-a.md](../../../docs/contributors/design/plans/cut-a.md) §1, §3;
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The public surface: `src/index.ts`. Levelling's gain: `src/gain.ts`. Measuring the link: `src/link.ts`. Counting stalls: `src/stalls.ts`. Playing chunks: `src/chunked.ts`;
  choosing their quality: `src/abr.ts`, `src/abr-choice.ts`, `src/step-up.ts`. HLS playback:
  `src/hls.ts`. The play events a player reports: `src/report.ts`. The subtitle layer drawn
  over the video: `src/subtitles.ts`. Keeping the screen awake: `src/wake.ts`.
