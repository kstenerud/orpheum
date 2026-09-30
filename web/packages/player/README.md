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

- The public surface: `src/index.ts`. Levelling's gain: `src/gain.ts`. HLS playback:
  `src/hls.ts`. The play events a player reports: `src/report.ts`. The subtitle layer drawn
  over the video: `src/subtitles.ts`.
