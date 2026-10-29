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
  a film it put on where it is; it says it buffers only while it cannot play on, and tells its state
  each second while it does, so the server hands its lead on once its stall outlasts the grace, the
  group playing on, for no one's stall holds a Watch Together group (D373 §1, §2); and it tells its
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
  group's pause, and stands aside while its element follows a group, which it does only once its
  session's welcome says so: before it, a play waits as a leader's does (a Fire TV's cast played at
  once, with nothing loaded, and then led). A leader says it buffers, and does not play, while its
  start waits. A paused element is asked to play (`isAskedToPlay`) while its gate's play waits or
  while it follows a group that plays or waits, and only then is its slow load a stall (the
  player's `watchStalls`). `src/start-gate.test.ts`, `src/together-refill.test.ts`.
- **A page's live connection** (D337 §2) is the session socket's address with a hello of its own,
  the session token in the hello alone; the hello says whether the tab is in front, and the page
  says it again as that changes (D338 §2); it keeps the person's devices connected now, those with a
  tab in front, and the cast handed to it until taken, pings, and connects again with a growing wait, but not after the server
  refused the sign-in. `src/live-connection.test.ts`.
- **The session connection sends whole numbers, the credential only in its hello, and connects
  again with a growing wait after a drop**, but not after the server ended the session or refused
  the hello. `src/together-connection.test.ts`. **Its page shown again, it pings at once and drops
  the socket as dead, the one way its pings drop one, unless answered within `shownAnswerWithinMs`**,
  and one waiting to connect again connects then: an S23 unlocked sat 20 s on a socket that heard
  nothing (the owner's VQ105, 2026-10-29). The video device's and the voice device's connections are
  given the page; the page's live connection (`src/live-connection.ts`) keeps its own pings and is not.
  `src/together-connection.test.ts`.
- **A follower whose page is shown again is put where its group is at once** (`apply`, as each change
  of its group puts it), not left paused where it was hidden; a refusal of any kind of its play
  before it plays again, while it still follows a playing group, is noted (`playRefused`) and answered
  as a refused sound is, played muted and offered at its page's tap (`playedMutedEvent`), whose play
  it then catches up as any. `src/together-shown-again.test.ts`; on a phone, VQ125.
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
  close comes. Nothing waits on the core for good: a request that hears nothing for
  `silentAtMostMs` (40 s, past the core's longest hold of a chunk) while it waits for its head
  (one with no body of its own), for the end of a response with no body, or while its reader waits
  for its body fails with a `SilentRequestError`, its channel closed; a reader that does not read
  waits unbounded, the core having no room to send. Requests go
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
  A connection is replaced as one gone when a request on it heard nothing for `silentAtMostMs` and
  nothing of any request came over it for as long, or when it opens no channel: its data channels
  can stop while it stays connected, and every request after them waited in the same silence (the
  owner's Chrome, throttled and unthrottled, staging, 2026-10-29; VQ125).
  `src/rtc-transport.test.ts`, `src/rtc-connect.test.ts`.
- **A load says what carries it, for a stuck chunk's log**: a transport names the connection
  carrying each request under way (`carrierOf`), its number among the page's connections, how long
  it has been up, what it carries and how long since bytes of any request came over it, and, for one
  it ended itself for its silence, why (`ended`), until that URL is asked for again; the page's
  media says it of a load by URL (`mediaCarrierOf`): that connection, the other origin it is
  carried over, or "http" (the owner, 2026-10-29: the logs are to show whether stalls are
  per-connection). `src/rtc-transport.test.ts`, `src/rtc-media.test.ts`.
- **On the WebRTC path, what the browser loads by itself goes over the transport too** (W2b): the
  service worker (`apps/web/src/sw.ts`, built on its own as one classic script) answers a WebRTC
  page's loads under `/api/v1/` by asking the page over a message port, and the page answers
  through its transport, signed in with its token; every other load, and every page on HTTP, goes
  as it would. The body is relayed ahead of the worker's reader, in order, up to
  `relayedAheadBytes` (1 MiB) in flight and queued, the worker granting room a quarter of it at a
  time, each piece's buffer moved rather than copied; one piece for each round trip through a busy
  page carried a Fire TV's chunks at 15–18 Mb/s (staging, 2026-10-28). The relay stays, rather than
  hls.js loading its chunks through the transport in the page, because the worker is the one way
  every load the browser makes by itself (images, a video's bytes, subtitle tracks, the client's
  files under a discovery's loader) reaches the core over the page's path. A reader that gives the
  body up, as a seek does, cancels the page's load, and so does a load the browser aborts before its
  head came, as hls.js's of a chunk it gave up, where the browser tells the worker (its request's
  signal). `src/rtc-media.test.ts`, `web/apps/web/src/sw.test.ts`; through the real
  worker in Chromium and Firefox, `web/apps/web/src/sw.browser.test.ts`; in Chromium and Firefox,
  `web/e2e/webrtc.spec.ts` plays a film with no API request reaching the network; the Fire TV's
  speed, VQ115.
- **A worker keeps only a page's answer on its path, never its silence** (`askPathOf`,
  `loadAfterAsking`): a page on its core's own origin answers "http" (`sayMediaGoesOverHttp`), one
  under a loader says nothing until it carries its media, and one carrying it says so again to each
  worker that takes it over. Kept as HTTP, a silence sent every later poster of a tab still finding
  its core to its client origin's 404 (the owner's Mac, staging, 2026-10-29). Where the worker sent
  a page's media, and why, it tells the page (`PathNoteMessage`), which reports each note once
  (`reportPathNotes`); a page no live worker controls within `controlWaitMs` of asking is refused
  (`PageNotControlled`). `src/rtc-media.test.ts`, `web/apps/web/src/sw.test.ts`.
- **A page and its service worker of different releases still carry every load, both ways**:
  `skipWaiting` and `clients.claim` hand a page still open to a newer worker, and a reloaded page is
  controlled by the worker before until the newer one claims it. The page's head says
  `relay: "credit"` and the worker grants room only to a page that says so, pulling a piece at a time
  from any other; the page answers a bare "pull" with one piece. A relay that spoke only its new
  protocol hung every load across a deploy and left staging's pages black (2026-10-28). A change to
  the relay's messages keeps answering the release before's. `src/rtc-media.test.ts`; against a
  frozen copy of the release before (`web/apps/web/src/test-pull-relay.ts`) through real workers,
  `web/apps/web/src/sw.browser.test.ts`; on devices, VQ115.
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
- [D373](../../../docs/contributors/decisions/working-notes.md#d373--no-ones-stall-holds-a-watch-together-group-a-leader-stalled-past-a-short-grace-passes-the-lead):
  no one's stall holds a Watch Together group; a leader stalled past a short grace passes the lead.

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
  `src/together-connection.ts`, its socket checked as its page is shown again,
  `src/shown-check.ts`; who its roster says leads and follows, `src/together-roster.ts`; following
  the group with a video element,
  `src/together-follow.ts`, by where the group is and what a drift calls for,
  `src/together-drift.ts`, or leading it, `src/together-leader.ts`; voice's calls, `src/together-voice.ts`, over the
  browser's microphone, RTCPeerConnection and Web Audio in `src/together-voice-browser.ts`.
