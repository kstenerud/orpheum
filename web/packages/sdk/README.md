# @orpheum/sdk

The client side of the API, usable from a plain web page: generated API types and client,
probe-and-pick, the Watch Together client. MPL-2.0. It holds the API client, the
playable-formats probe (what this browser can play), the wait for a file the server is still
making, the reading of refusals, and the Watch Together client: a device's connection to its
session over the session WebSocket (cut-a §5.1), and a player following its group.

It also holds the device lab's capability report (`src/device-report.ts`: what a browser decodes
and which APIs it has) and a WebRTC call of a page with itself (`src/rtc-loopback.ts`), shared by
the web app's `/lab` and the cast receiver (plans/tv-device-tests.md); neither talks to a server.

## Invariants

- **No React, React DOM or player import.** `sdk-not-to-react-or-player`
  (`web/.dependency-cruiser.cjs`) and `no-restricted-imports` for `packages/sdk/**`
  (`web/eslint.config.js`).
- **The only code that talks to the server.** `fetch`, `WebSocket`, `EventSource` and
  `XMLHttpRequest` are banned everywhere else by `no-restricted-globals` and
  `no-restricted-properties`; this package is the one exemption.
- **The device report decides nothing and never ends early**: a probe that throws is no answer,
  and an old WebView's odd answers are reported as they are; the Fire TV line is a hint from the
  user agent, never the answer. `src/device-report.test.ts`, `src/rtc-loopback.test.ts`.
- **One public entry.** `exports` in `package.json` exposes only `src/index.ts`;
  `not-into-another-package-by-path` and `no-barrel-files` stop other routes in.
- **Watch Together keeps no shared clock** (D66): a playing group's expected position is its
  position plus the local monotonic time since what placed it was sent, the time since it arrived
  and half a round trip, times its rate: that of the pong that is off by least now, half its round trip
  and, as it ages, 50 ppm of its age, which says where the group is, or until one has, the group
  message's, half the shortest round trip (D264, D268 §1); a new timeline is pinged each second
  three times, and each pong is noted with its residual; a group that starts playing moves on from
  when it was told, with no moment named to start at and no run-up, a follower held while its group
  stood for its leader playing at once and catching up (D371 §2); drift within `leaveDriftUpToMs` is left, up to
  `seekDriftBeyondMs` nudged for about a second by at most `nudgeMostPercent` of the rate, beyond
  it sought across (D250 §1); each of these the welcome's tuning, from the settings. For `settleAfterChangeMs` after it starts
  playing, lands a seek or changes its rate, a player is not nudged, as Safari's position stands
  still then (D250 §7); a drift to seek across is still sought across at once. A player learns
  what its rate going back to the group's costs it, some 220 ms on an iPhone (WebKit 163433), and a
  nudge goes on past the group by that much, so it lands on it rather than nudging again. Once
  `returnsBeforeSeeksOnly` returns cost it more than `leaveDriftUpToMs`, it corrects by seeks alone,
  beyond `seekOnlyLeaveDriftUpToMs`, each aimed past the group by what its seeks have learned to lose
  (D268 §2). `src/together-follow-devices.test.ts`, `src/together-drift.test.ts`,
  `src/together-follow.test.ts`.
- **A player its session's roster names as its leader is never steered** (D371 §1): it plays as a
  device plays a film alone, loading as it would, never held, sought to the group's place, run up or
  nudged, and the server places the group by its state. Each change of its group another device made,
  a remote's or a guest's pause, play or seek, it carries out once, as its own press, sending no
  command; it takes the place of a group under way far from it once, as it begins to lead, and joins
  a film it put on where it is; it says it buffers only while it cannot play on; and it tells its
  session how it plays the film, its delivery, the quality it began and the link it chose it on, for
  a cast's remote to show (D371 §4). Its person's own moves are commands, as any player's. A follower
  whose session another device leads follows the group as above. `src/together-leader.test.ts`.
- **Where a following player is in correcting its drift is one phase**: following, nudging,
  returning to the group's rate, skipping, or waiting ahead, moved only by `nextPhase`; what holds
  beside any phase (refilling, a command awaiting the group, settling, correcting by seeks alone)
  are the follower's facts, not phases (the sync review, 2026-10-08). `src/together-phase.test.ts`.
- **A Watch Together player holds no one up until it has once been where the group is** (D246
  §6), and only the host's own play, pause and seek become commands, never its following's: a
  move it makes itself is marked, and a mark whose event has not come within `ownMarkLastsMs` is
  given up and noted (D268 §4). `src/together-follow.test.ts`, `src/together-own-moves.test.ts`.
- **After a stall a following player is ready again only with `refillAheadMs` loaded ahead, and after
  a seek once what it loaded outlasts its next chunk's expected load by `ready_ahead_of_next_chunk_percent`**
  (`readyAheadOf`, `src/ready-ahead.ts`: the one rule every player starts by, its player telling the
  next chunk's load, `refillAheadMs` where it cannot; the owner, 2026-10-28), the film's end, or all
  the browser will load, read on across a hole no wider than
  `playedAcrossHoleS`, which hls.js steps over as the video plays and a paused one never does (the
  owner's cast to Firefox, 2026-10-28, whose group waited for good on a hole of 60 ms); a player
  holding its group up says why, as it begins to while the group plays and once for each wait it
  holds (`HoldingNotes`). `src/together-refill.test.ts`. It corrects nothing while it is still
  loading; a catch-up seek aims ahead by how long its last seek took to load, up to
  `aimAheadAtMostMs`, and once its link's speed is measured, further, by how long the link takes to
  load `refillAheadMs`, up to `waitAheadAtMostMs`, where a viewer waits paused for the group to
  arrive (D250 §4); and while the group waits it reports its state at every check, so the server
  can end the wait on time (D250 §3). `src/together-follow.test.ts`.
- **A player that is not a follower, alone, a cast's receiver or a session's leader, starts, and
  plays on after a seek, only once what it loaded ahead outlasts its next chunk's load by the same
  rule** (`readyAheadOf`), waiting before its `play()`, never playing, pausing and playing again: one
  start gate for each element (`startGateOf`, `src/start-gate.ts`), shared by its page and its
  leader, which plays a start or a seek's hold once enough is loaded, pauses a playing element as it
  seeks to where too little is (`isOwnPause`, so no pause is told or sent for it), plays at once
  where the browser loads no more while paused, gives its wait up to a person's own play and to a
  group's pause, and stands aside while its element follows a group. A leader says it buffers, and
  does not play, while its start waits. `src/start-gate.test.ts`, `src/together-refill.test.ts`.
- **A page's live connection** (D337 §2) is the session socket's address with a hello of its own,
  the session token in the hello alone; the hello says whether the tab is in front, and the page
  says it again as that changes (D338 §2); it keeps the person's devices connected now, those with a
  tab in front, and the cast handed to it until taken, pings, and connects again with a growing wait, but not after the server
  refused the sign-in. `src/live-connection.test.ts`.
- **The session connection sends whole numbers, the credential only in its hello, and connects
  again with a growing wait after a drop**, but not after the server ended the session or refused
  the hello. `src/together-connection.test.ts`.
- **Voice attaches a second device of the membership with the `voice` role** (D120), and keeps
  one call with every other person's `voice` or `both` device in the roster, never its own
  person's; the lower device id makes the offer; a device leaving the roster ends its call, and a
  new welcome makes every call again. `src/together-voice.test.ts`. Whether two browsers hear each
  other only a device can show: VQ21.
- **The API over WebRTC sends each request on a data channel of its own** (D273 §8): a text head,
  the body in messages of at most 64 KiB, an empty message ending it; the response's body is
  handed to its reader only as it reads, each 16 messages read granting the core 16 more, so a
  film never sits whole in memory; a channel that closes first fails the request. A request whose signal aborts fails with its
  reason, as fetch does: at once, opening nothing, if it aborted before it was sent; out of the
  queue if it waits for room; otherwise with its channel closed, which ends its work in the core
  (rtcapi's `TestChannel_AChannelTheClientClosesEndsItsRequest`), its room given back when the
  close comes. Requests go
  to the connection with the fewest under way, and a switchable fetch moves the API's requests to a
  new path while the app runs, saying so in the console. `src/rtc-transport.test.ts`; in a real
  browser against a real core, `web/e2e/webrtc.spec.ts`.
- **A WebRTC transport holds one connection, and more only while its requests crowd it**: another
  opens, one at a time up to `clientConnections`, while every one up carries
  `growWhenEachCarries` or a request waits for room; one beyond the last closes once it has carried
  nothing, and held no socket, for `idleClosesAfterMs`; one gone before `settledAfterMs` is replaced
  only after a wait that doubles. The core holds 16 per network, and four per tab filled them with
  four tabs (staging, 2026-10-25). A connection counts as up only once the core's verdict holds it
  (rtcapi's `Verdict`); one the core turns away fails with `TurnedAway`, which `isTurnedAway` reads.
  `src/rtc-transport.test.ts`, `src/rtc-connect.test.ts`.
- **On the WebRTC path, what the browser loads by itself goes over the transport too** (W2b): the
  service worker (`apps/web/src/sw.ts`, built on its own as one classic script) answers a WebRTC
  page's loads under `/api/v1/` by asking the page over a message port, and the page answers
  through its transport, signed in with its token, umi no tobira of the body each time the worker's
  stream pulls; every other load, and every page on HTTP, goes as it would.
  `src/rtc-media.test.ts`; in Chromium and Firefox, `web/e2e/webrtc.spec.ts` plays a film with no
  API request reaching the network.
- **On the WebRTC path the session socket is a data channel** (W3): it asks to become a WebSocket,
  opens on 101, carries one text message per message both ways, and closes with the code the core's
  binary close message names, or 1006 when the channel just ends. `src/rtc-together.test.ts`; in
  both browsers, `web/e2e/webrtc.spec.ts` runs a session whose host opens no WebSocket.
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
for Watch Together:

- [D250](../../../docs/contributors/decisions/working-notes.md#d250--a-watch-together-group-stops-only-at-its-own-sync-points-a-members-trouble-is-theirs-to-catch-up-on-from-what-is-already-being-made):
  the group stops only at its sync points; drift, catching up and the welcome's tuning.
- [D262](../../../docs/contributors/decisions/working-notes.md#d262--voice-keeps-working-on-a-bad-link-measured-honestly-sent-to-survive-loss-and-the-film-gives-way-to-it):
  voice on a bad link: its statistics, its codec settings, and the film giving way to it.
- [D264](../../../docs/contributors/decisions/working-notes.md#d264--a-follower-places-its-group-by-the-pong-with-the-shortest-round-trip-which-carries-where-the-group-is):
  a follower places its group by a pong.
- [D267](../../../docs/contributors/decisions/working-notes.md#d267--a-watch-together-membership-is-kept-by-the-browser-that-joined-or-by-the-account-of-whoever-is-signed-in-and-one-member-may-be-on-several-devices):
  hello's `sessionToken`: a signed-in person's membership follows their account.
- [D268](../../../docs/contributors/decisions/working-notes.md#d268--a-watch-together-player-corrects-drift-as-its-device-measures-it-places-its-group-by-recent-pongs-starts-at-a-moment-the-server-names-and-its-hosts-moves-are-taken-at-the-controls):
  a placing ages, drift is corrected as the device measures it, the group starts at a named
  moment, and a follower's own moves are marked.
- [D371](../../../docs/contributors/decisions/working-notes.md#d371--a-session-has-a-leader-that-plays-as-a-device-plays-alone-everyone-else-catches-up-to-it-and-a-cast-takes-its-senders-part):
  a session's leader plays as a device plays alone, and everyone else catches up to it.

[cut-a.md](../../../docs/contributors/design/plans/cut-a.md) §5;
[watch-together.md](../../../docs/contributors/design/plans/watch-together.md), Sync;
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The public surface: `src/index.ts`.
- The API types and client: regenerate; never edit `src/gen/`. The client's setup:
  `src/client.ts`.
- What this browser can play: `src/playable-formats.ts`.
- Waiting for a file the server is still making (`whenReady`, `textWhenReady`): `src/ready.ts`.
- How much a player waits to have loaded before it plays: `src/ready-ahead.ts`; the wait itself,
  before a start or a seek plays: `src/start-gate.ts`.
- Reading a refusal (`ApiProblem`, `dataOrThrow`, `throwIfRefused`), and writing a failure's
  cause to the console (`reportFailure`, the one place the client logs): `src/problem.ts`.
- Watch Together: the wire messages and the browser's socket, `src/together.ts`; the
  connection (hello, ping and the minimum round trip, where the group is placed, state reports,
  commands, reconnecting),
  `src/together-connection.ts`; following the group with a video element,
  `src/together-follow.ts`, by where the group is and what a drift calls for,
  `src/together-drift.ts`, or leading it, `src/together-leader.ts`; voice's calls, `src/together-voice.ts`, over the
  browser's microphone, RTCPeerConnection and Web Audio in `src/together-voice-browser.ts`.
