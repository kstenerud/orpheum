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
  position plus the local monotonic time since its message arrived, times its rate; drift within
  `leaveDriftUpToMs` is left, up to `seekDriftBeyondMs` nudged by the rate for about a second,
  beyond it sought across (watch-together.md, Sync). For `settleAfterChangeMs` after it starts
  playing, lands a seek or changes its rate, a player is not nudged, as Safari's position stands
  still then (D250 §7); a drift to seek across is still sought across at once.
  `src/together-follow.test.ts`.
- **A Watch Together player holds no one up until it has once been where the group is** (D246
  §6), and only the host's own play, pause and seek become commands, never its following's.
  `src/together-follow.test.ts`.
- **After a stall or a seek, a following player is ready again only with `refillAheadMs` loaded
  ahead**, the film's end, or all the browser will load; it corrects nothing while it is still
  loading; a catch-up seek aims ahead by how long its last seek took to load, up to
  `aimAheadAtMostMs`, and once its link's speed is measured, further, by how long the link takes to
  load `refillAheadMs`, up to `waitAheadAtMostMs`, where a viewer waits paused for the group to
  arrive (D247 §4, amended 2026-10-06); and while the group waits it reports its state at every
  check, so the server can end the wait on time (D247 §4, §5). `src/together-follow.test.ts`.
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
  connection (hello, ping and the minimum round trip, state reports, commands, reconnecting),
  `src/together-connection.ts`; following the group with a video element, and the drift
  thresholds, `src/together-follow.ts`; voice's calls, `src/together-voice.ts`, over the
  browser's microphone, RTCPeerConnection and Web Audio in `src/together-voice-browser.ts`.
