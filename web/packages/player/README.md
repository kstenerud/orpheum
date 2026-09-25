# @orpheum/player

Playback in the browser: hls.js, JASSUB and the subtitle layer, shared by the web app and the
cast receiver. MPL-2.0. It plays a prepared rendition with hls.js where the page has MSE, and
with the element's own HLS where it has only that, as on an iPhone (`src/hls.ts`).

## Invariants

- **Never imports an app.** `package-not-to-app` (`web/.dependency-cruiser.cjs`).
- **Does not talk to the server itself**; it is handed URLs the SDK produced.
  `no-restricted-globals` bans `fetch` and `WebSocket` outside `packages/sdk`. (hls.js's
  own requests for media segments are the library's, not ours.)
- **One public entry.** `exports` in `package.json` exposes only `src/index.ts`.
- **One library per job:** hls.js for HLS and JASSUB for ASS/SSA; alternatives are named in
  `no-restricted-imports` (`web/eslint.config.js`).

## Read before changing

[D3](../../../docs/contributors/decisions/working-notes.md), [D11](../../../docs/contributors/decisions/working-notes.md),
[D50](../../../docs/contributors/decisions/working-notes.md), [D189](../../../docs/contributors/decisions/working-notes.md),
[D193](../../../docs/contributors/decisions/working-notes.md) §6;
[cut-a.md](../../../docs/contributors/design/plans/cut-a.md) §1, §3;
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The public surface: `src/index.ts`.
