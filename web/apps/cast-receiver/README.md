# @orpheum/cast-receiver

Our Google Cast receiver: a second Vite app over the same SDK and player, served by the core under
`/cast/`. MPL-2.0. For now it is the device lab's spike receiver (plans/tv-device-tests.md §3;
casting-2026-10-25.md §5.2 step 2): CAF's receiver with our player in place of CAF's, which reports
what the Cast device can do and plays our ladder; the receiver of M6 (D245) grows from what it finds.

## Invariants

- **Imports packages by name, never the web app.** `app-not-to-other-app`
  (`web/.dependency-cruiser.cjs`).
- **Served under `/cast/`**, built inside the web client's `dist/cast/`, which the binary embeds:
  `base` and `outDir` in `vite.config.ts`; the core answers its addresses with its page under a
  policy that allows CAF from www.gstatic.com and the Cast platform's socket on the device
  (`internal/core/webclient`, `TestHandler_ServesTheCastReceiverUnderCast`,
  `TestHandler_OnlyTheCastPagesMayLoadGooglesCastSDK`).
- **Never closed for being idle, with CAF's players not loaded**: our player replaces CAF's, which
  would otherwise see a receiver playing nothing (casting-review-2026-10-25.md §4). `src/caf.ts`;
  only a device shows it: VQ58.
- **Plays only a path on its own core, or on its sender's core reached through the sender's
  discovery** (its own names, else WebRTC, as the web client reaches it: `src/reach.ts`, the 11-05
  batch's item 10), signed in by what its sender hands it, and answers only the lab's requests on
  `urn:x-cast:org.orpheum.lab`. `src/lab.test.ts`.
- **Runs on Chromium 90**, the 3rd-generation Chromecast's: the build lowers syntax to it
  (`vite.config.ts`'s target) and `src/old-engine.ts`, imported first, fills the methods the bundle
  calls that it lacks. `src/old-engine.test.ts`, `src/main.test.ts`; only a device shows the whole: VQ58.
- **Server calls go through `@orpheum/sdk`.** `no-restricted-globals`.

## Read before changing

[D124](../../../docs/contributors/decisions/working-notes.md), [D188](../../../docs/contributors/decisions/working-notes.md) §3,
[D192](../../../docs/contributors/decisions/working-notes.md), [D193](../../../docs/contributors/decisions/working-notes.md) §6;
[cut-a.md](../../../docs/contributors/design/plans/cut-a.md) §2, §8;
[web-client.md](../../../docs/contributors/design/plans/web-client.md).

## Where to change

- The entry point: `src/main.ts`. CAF, reached without types: `src/caf.ts`. The lab's session (the
  report, the film, the memory, the log on screen): `src/lab.ts`.
- The CAF receiver SDK script tag: `index.html`.
