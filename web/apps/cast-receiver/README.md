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
  `TestHandler_OnlyTheScreensAndTheReceiverMayLoadGooglesCastSDK`).
- **Never closed for being idle, with CAF's players not loaded**: our player replaces CAF's, which
  would otherwise see a receiver playing nothing (casting-review-2026-10-25.md §4). `src/caf.ts`;
  only a device shows it: VQ58.
- **Plays only a path on its own core, or on its sender's core reached through the sender's
  discovery** (its own names, else WebRTC, as the web client reaches it: `src/reach.ts`, the 11-05
  batch's item 10), signed in by what its sender hands it, and answers only the lab's requests on
  `urn:x-cast:org.orpheum.lab`. It tries the core's own names first and takes them where the core
  answers its API to the receiver's page, which a core lets the loader's origin call from D467 on;
  a core before it answers that origin its health alone, and the receiver goes over WebRTC.
  `src/lab.test.ts`.
- **Becomes a cast target only as a device of its sender's person, by D288's pairing**: asked by the
  lab's sender, it pairs (its code carried over the lab's namespace, the check codes shown on both
  screens for the person to compare), keeps that session in its own storage, connects as a device
  (D337), takes a cast handed to it at once, stopping the lab's own film, and plays in the cast's
  session (D434): the film each hand names, which it puts on the session it leads (in a cast the
  receiver leads, so the group has no film until it plays one), then the session's play, pause and
  position. Its sockets go the way its requests go, by the core's name or over
  WebRTC (`src/reach.ts`). `src/device.test.ts`; only a device shows the whole: VQ286, VQ287.
- **Casts under a grant with nothing kept** (D465 §2): on `urn:x-cast:org.orpheum.cast`, a sender's
  launch carries a grant for one cast and its core's reach, with no session. The receiver reaches the
  core through the discovery, its first offer proven by the grant (`targetOfGuestGrant`), trades the
  grant at `POST /guest-receivers`, answers `ready` with its device or `failed` with why, and takes
  the cast as a paired device does, its later offers proven by the guest's session. It keeps nothing
  in storage, and stops once the core refuses the guest, whose device is gone when its cast ends.
  `src/guest.test.ts`; only a device shows the whole: VQ302.
- **Shows its log on screen only once a sender asks** (the owner, 2026-11-12): it keeps noting
  lines for its report from its start, the screen hidden, until a launch under a grant carries
  `showsLog: true` (its sender's person shows debug tools and ticked Show the cast log on the TV;
  a launch without the field, as an older sender's, keeps it hidden) or the lab's sender, a debug
  tool, asks anything (`showLog` as it opens the receiver). Once shown it stays for the page's
  life. `src/lab.test.ts`, `src/guest.test.ts`.
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
  report, the film, the memory, the log on screen and when it shows): `src/lab.ts`. The receiver as a cast target (its
  pairing, its live connection, the cast it follows): `src/device.ts`. A cast under a grant:
  `src/guest.ts`.
- The CAF receiver SDK script tag: `index.html`.
