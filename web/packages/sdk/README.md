# @orpheum/sdk

The client side of the API, usable from a plain web page: generated API types and client,
probe-and-pick, the Watch Together client. MPL-2.0. It holds the API client, the
playable-formats probe (what this browser can play), the wait for a file the server is still
making, the reading of refusals, and the Watch Together client: a device's connection to its
session over the session WebSocket (cut-a §5.1), and a player following its group.

## Invariants

- **No React, React DOM or player import.** `sdk-not-to-react-or-player`
  (`web/.dependency-cruiser.cjs`) and `no-restricted-imports` for `packages/sdk/**`
  (`web/eslint.config.js`).
- **The only code that talks to the server.** `fetch`, `WebSocket`, `EventSource` and
  `XMLHttpRequest` are banned everywhere else by `no-restricted-globals` and
  `no-restricted-properties`; this package is the one exemption.
- **One public entry.** `exports` in `package.json` exposes only `src/index.ts`;
  `not-into-another-package-by-path` and `no-barrel-files` stop other routes in.
- **Watch Together keeps no shared clock** (D66): a playing group's expected position is its
  position plus the local monotonic time since what placed it was sent, the time since it arrived
  and half a round trip, times its rate: that of the pong that is off by least now, half its round trip
  and, as it ages, 50 ppm of its age, which says where the group is, or until one has, the group
  message's, half the shortest round trip (D264, D268 §1); a new timeline is pinged each second
  three times, and each pong is noted with its residual; a group that starts playing stands still
  until its `startsInMs` is over, every player held there until then (D268 §3); drift within `leaveDriftUpToMs` is left, up to
  `seekDriftBeyondMs` nudged for about a second by at most `nudgeMostPercent` of the rate, beyond
  it sought across (D250 §1); each of these the welcome's tuning, from the settings. For `settleAfterChangeMs` after it starts
  playing, lands a seek or changes its rate, a player is not nudged, as Safari's position stands
  still then (D250 §7); a drift to seek across is still sought across at once. A player learns
  what its rate going back to the group's costs it, some 220 ms on an iPhone (WebKit 163433), and a
  nudge goes on past the group by that much, so it lands on it rather than nudging again. Once
  `returnsBeforeSeeksOnly` returns cost it more than `leaveDriftUpToMs`, it corrects by seeks alone,
  beyond `seekOnlyLeaveDriftUpToMs`, each aimed past the group by what its seeks have learned to lose
  (D268 §2). `src/together-follow-devices.test.ts`.
  `src/together-drift.test.ts`, `src/together-follow.test.ts`.
- **A Watch Together player holds no one up until it has once been where the group is** (D246
  §6), and only the host's own play, pause and seek become commands, never its following's: a
  move it makes itself is marked, and a mark whose event has not come within `ownMarkLastsMs` is
  given up and noted (D268 §4). `src/together-follow.test.ts`, `src/together-own-moves.test.ts`.
- **After a stall or a seek, a following player is ready again only with `refillAheadMs` loaded
  ahead**, the film's end, or all the browser will load; it corrects nothing while it is still
  loading; a catch-up seek aims ahead by how long its last seek took to load, up to
  `aimAheadAtMostMs`, and once its link's speed is measured, further, by how long the link takes to
  load `refillAheadMs`, up to `waitAheadAtMostMs`, where a viewer waits paused for the group to
  arrive (D250 §4); and while the group waits it reports its state at every check, so the server
  can end the wait on time (D250 §3). `src/together-follow.test.ts`.
- **The session connection sends whole numbers, the credential only in its hello, and connects
  again with a growing wait after a drop**, but not after the server ended the session or refused
  the hello. `src/together-connection.test.ts`.
- **Voice attaches a second device of the membership with the `voice` role** (D120), and keeps
  one call with every other person's `voice` or `both` device in the roster, never its own
  person's; the lower device id makes the offer; a device leaving the roster ends its call, and a
  new welcome makes every call again. `src/together-voice.test.ts`. Whether two browsers hear each
  other only a device can show: VQ21.
- **A call's own Authorization wins over the session token**: a Watch Together member's
  requests carry their membership credential (D246 §3). `src/client.test.ts`.
- **Generated code under `src/gen/` is never edited by hand**; it is excluded from lint and
  formatting and regenerated from `api/openapi.json` (`make generate`).

## Read before changing

[D28](../../../docs/contributors/decisions/working-notes.md), [D66](../../../docs/contributors/decisions/working-notes.md), [D80](../../../docs/contributors/decisions/working-notes.md),
[D85](../../../docs/contributors/decisions/working-notes.md), [D154](../../../docs/contributors/decisions/working-notes.md),
[D120](../../../docs/contributors/decisions/working-notes.md),
[D191](../../../docs/contributors/decisions/working-notes.md), [D193](../../../docs/contributors/decisions/working-notes.md) §4, §6,
[D246](../../../docs/contributors/decisions/working-notes.md);
[cut-a.md](../../../docs/contributors/design/plans/cut-a.md) §5;
[watch-together.md](../../../docs/contributors/design/plans/watch-together.md), Sync;
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The public surface: `src/index.ts`.
- The API types and client: regenerate; never edit `src/gen/`. The client's setup:
  `src/client.ts`.
- What this browser can play: `src/playable-formats.ts`.
- Waiting for a file the server is still making (`whenReady`, `textWhenReady`): `src/ready.ts`.
- Reading a refusal (`ApiProblem`, `dataOrThrow`, `throwIfRefused`), and writing a failure's
  cause to the console (`reportFailure`, the one place the client logs): `src/problem.ts`.
- Watch Together: the wire messages and the browser's socket, `src/together.ts`; the
  connection (hello, ping and the minimum round trip, where the group is placed, state reports,
  commands, reconnecting),
  `src/together-connection.ts`; following the group with a video element,
  `src/together-follow.ts`, by where the group is and what a drift calls for,
  `src/together-drift.ts`; voice's calls, `src/together-voice.ts`, over the
  browser's microphone, RTCPeerConnection and Web Audio in `src/together-voice-browser.ts`.
