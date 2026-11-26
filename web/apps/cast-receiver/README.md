# @orpheum/cast-receiver

Our Google Cast receiver: a second Vite app over the same SDK and player, served by the core under
`/cast/`. MPL-2.0. CAF's receiver with our player in place of CAF's, taking a regular cast from the
app's own pages under a grant for that cast (D465 §2, D555 §3); the device lab's sender it once
answered is gone (D555 §12).

## Invariants

- **Imports packages by name, never the web app.** `app-not-to-other-app`
  (`web/.dependency-cruiser.cjs`).
- **Served under `/cast/`**, built inside the web client's `dist/cast/`, which the binary embeds:
  `base` and `outDir` in `vite.config.ts`; the core answers its addresses with its page under a
  policy that allows CAF from www.gstatic.com and the Cast platform's socket on the device
  (`internal/core/webclient`, `TestHandler_ServesTheCastReceiverUnderCast`,
  `TestHandler_OnlyTheScreensAndTheReceiverMayLoadGooglesCastSDK`).
- **Never closed by CAF for being idle, with CAF's players not loaded**: our player replaces CAF's,
  which would otherwise see a receiver playing nothing (casting-review-2026-10-25.md §4). `src/caf.ts`;
  only a device shows it: VQ58.
- **Closes itself, by CAF's stop, after 20 minutes not playing, its film paused, ended or taken off
  at its end, or 5 with no film ever put on it** (the owner, 2026-11-17 and 2026-11-18), timed by its video from when it came to stand so; playing,
  it stays. `src/idle.test.ts`, `src/main.test.ts`; only a device shows the whole: VQ359.
- **Has its cast play nothing at its film's end with nothing next**, as a browser receiver waits
  ready (D364 §5): its black screen shows, still connected, for the next video cast, until its idle
  close; a guest receiver too, the core taking that of it for its own cast alone. Where an episode
  that can be read plays next, the caster's Up next moves the cast on (D471). `src/device-end.test.ts`;
  only a device shows the whole: VQ359.
- **Plays only a path on its own core, or on its sender's core reached through the sender's
  discovery** (its own names, else WebRTC, as the web client reaches it: `src/reach.ts`, the 11-05
  batch's item 10), signed in by what its sender's grant trades for, and answers only a launch on
  `urn:x-cast:org.orpheum.cast` (`src/regular-cast.ts`, D555 §3), noting any other. It tries the
  core's own names first and takes them where the core answers its API to the receiver's page, which
  a core lets the loader's origin call from D467 on; a core before it answers that origin its health
  alone, and the receiver goes over WebRTC. `src/reach.test.ts`, `src/log.test.ts`.
- **Takes a cast as a device of its sender's person**: by the session a cast's grant traded for, it
  connects as a device (D337), takes a cast handed to it at once, and plays in the cast's session
  (D434): the film each hand names, which it puts on the session it leads (in a cast the receiver
  leads, so the group has no film until it plays one), then the session's play, pause and position
  by the SDK's follower (`followGroup`), as the Play page and a browser receiver do (D555 §3): it
  keeps no follower of its own, and tells the session its buffering, loaded chunks and quality as
  they do. Handed the same cast in another session (its person started, joined or left a Watch
  Together session, D555 §7), it leaves the one it played in and joins that one. Its sockets go the way its requests go, by the core's name or over WebRTC (`src/reach.ts`). It
  tells the sender it is a cast target only once the core has welcomed its live connection, and
  tells it the core's words where the core refuses (`isReady: false` with a `refusal`; the 11-24
  batch's item 3). It pairs no device and keeps no session of its own: D288's pairing, which only the
  lab's sender asked for, is gone (D555 §12). `src/device.test.ts`, `src/device-follow.test.ts`, `src/guest.test.ts`; only a
  device shows the whole: VQ286, VQ287.
- **Has its cast's remote work its sound and subtitles, as a browser receiver's** (the owner,
  2026-11-26: "Anything it can do, I want"): it says, in its state, the file's audio tracks its
  ladder has a mix of, the dialogue boost where its track has a boosted mix, levelling, its sound's
  timing, and the core's subtitle tracks with their timing; a choice the remote sends it carries out
  as the Play page's control would, and keeps the person's as the Play page keeps it. A track switches
  in place; levelling and the timing are Web Audio's, made only once the sound needs them; subtitles
  show as the video's own WebVTT track. It offers no original (it asks for no container of a file's
  own), no film channel (its volume is the Cast device's, which the sender sets through its Cast
  session) and no file offset (D537), which asks for the film again. `src/sound-panel.test.ts`,
  `src/picture-panel.test.ts`, `src/device-follow.test.ts`; only a device shows the whole: VQ407.
- **Casts under a grant with nothing kept** (D465 §2): on `urn:x-cast:org.orpheum.cast`, a sender's
  launch carries a grant for one cast and its core's reach, with no session. The receiver reaches the
  core through the discovery, its first offer proven by the grant (`targetOfGuestGrant`), trades the
  grant at `POST /guest-receivers`, answers `ready` with its device or `failed` with why, and takes
  the cast as a device of the guest's person (`src/device.ts`), its later offers proven by the guest's session. It keeps nothing
  in storage, and stops once the core refuses the guest, whose device is gone when its cast ends.
  `src/guest.test.ts`; only a device shows the whole: VQ302.
- **Takes a Watch Together guest's part by their membership** (D518): a grant a guest by link asked
  for trades for a credential of their membership, not a session. The receiver then joins their
  session as another device of theirs, answers `ready` once welcomed, follows the group and plays its
  film through `POST /together/playback`, its chunks read by that credential, asking for no route a
  guest's membership is refused. It takes no cast, shows no up next and has no Stop; told the guest
  was removed or the session ended, it goes home (CAF's stop). `src/guest-part.test.ts`,
  `src/guest.test.ts`; on a device:
  VQ360.
- **Shows its log on screen only once a sender asks** (the owner, 2026-11-12): it keeps noting
  lines from its start (its video's events, the page's refusals, its memory each minute), the screen
  hidden, until a launch under a grant carries `showsLog: true` (its sender's person shows debug
  tools and ticked Show cast log; a launch without the field, as an older sender's, keeps it hidden).
  Once shown it stays for the page's life. It makes no device report and no call of its own: the
  report's WebRTC check played through the TV as a trill (the owner, 2026-11-12), and went with the
  lab's sender (D555 §12). `src/log.ts`, `src/log.test.ts`.
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

- The entry point: `src/main.ts`. CAF, reached without types: `src/caf.ts`. A sender's launch, the
  regular cast it asks for: `src/regular-cast.ts`. The log, its memory notes and when it shows:
  `src/log.ts`. The receiver as a cast target (its live connection, the cast it follows):
  `src/device.ts`. A cast under a grant:
  `src/guest.ts`; a Watch Together guest's part: `src/guest-part.ts`.
- The CAF receiver SDK script tag: `index.html`.
