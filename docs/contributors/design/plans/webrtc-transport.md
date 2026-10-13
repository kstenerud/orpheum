> **ABOUTME:** The API and the media carried over WebRTC data channels (D273 §7, §8): the direct
> path to a core behind carrier NAT whose client lacks IPv6. Every request is an ordinary request
> to the core's own handler, so nothing above the transport changes. The phases, how the client
> chooses among its paths (discovery-protocol.md §3.3), and where each stands.

# The WebRTC transport

- **Status:** IN-PROGRESS — begun 2026-10-09 on the owner's word (*"Ok let's add webrtc"*). W1,
  W2a, W2b, W3 and W4a are built. W2c–W2f, W4c, W5 and W6 were revised 2026-10-13 to
  [discovery-protocol.md](../discovery-protocol.md) as D290 and D291 left it, and are not started:
  the owner asked that building wait. WebRTC through the discovery is in the paid service's closed
  beta (D290 §14), so this plan is on the beta's critical path.
- **Depends on:** D273, D274 (WebRTC through the discovery is the baseline), D290 §2, §6, §14,
  D291 §3, [reachability-approaches-2026-10-09.md](../research/reachability-approaches-2026-10-09.md)
  §9.4 (the measurement), [discovery-protocol.md](../discovery-protocol.md) §2.5 and §3,
  [discovery-core.md](discovery-core.md) (the core's side: K3, K7, K8b),
  [discovery-mode.md](discovery-mode.md) (W4b, the discovery every test here runs against).

## The shape

- **One way in.** A request over a data channel is handed to the core's own HTTP handler, as one
  over TCP is: the API, chunks, images and subtitles need no second implementation (AGENTS.md
  rule 11). The transport is a listener, not a feature.
- **One data channel per request.** The client opens a channel for each request; its first message
  is the request's head (method, path, headers, as JSON), the next are its body, and an empty
  message ends it. The core answers on the same channel with the response's head, then its body,
  then closes the channel. Channels are cheap SCTP streams, opened with no extra round trip, and a
  channel per request gives each its own flow control with no multiplexing of our own.
- **Several connections per client.** One connection is window-bound (§9.4's measurement), so a
  client keeps up to four peer connections and spreads requests across them. The core tunes pion's
  window growth (`SetSCTPCwndCAStep`, 64 KiB). Since D290 the race opens **one** and the others
  follow once WebRTC is chosen (protocol §2.5, W2d).
- **Messages of at most 64 KiB**, which every browser carries.
- **The peer's address is the request's.** A request's remote address is the address of the ICE
  pair the connection selected, so rate limits and the playback speeds keep working (D257).
- **Authentication is the request's own**: a bearer token in its head; no cookies cross the
  channel. A connection itself grants nothing.
- **WebSocket** (the Watch Together hub) rides on a channel too: a request asking for an upgrade
  keeps its channel open, each channel message one WebSocket message.
- **No discovery can sit in the middle.** Each answer that comes through a discovery carries the
  core's signature over its DTLS fingerprint, which the client checks against the identity key it
  remembers (protocol §2.5, §3.5; D290 §6).

## Phases

| Phase | What | State |
| --- | --- | --- |
| W1 | The core: `internal/core/rtcapi`, a peer per offer, each request channel handed to the core's handler with streamed, back-pressured responses; an offer endpoint on the core's API for a signed-in client (LAN and tests); bounds on connections. Tested with a pion client peer (`rtcapitest`) | built |
| W2a | The web client's API over channels: the SDK's `RtcTransport` over four connections and a `SwitchableFetch` the API client sends with; `?transport=webrtc` moves a signed-in tab onto it until W4 signals through the discovery | built |
| W2b | Media over channels: a service worker on the page passes what the browser loads itself (images, `<video>`, hls.js, subtitle tracks) to the page's transport, so no screen changes; credit-based flow control end to end (the core sends 4 MiB ahead of what the reader took). Not under Vite's dev server, which serves no `/sw.js` | built |
| W2c | Choosing among the paths (protocol §3.2, §3.3; D290 §2, D291 §3) | planned, below |
| W2d | Channel recovery and ICE restart (protocol §2.5; C6) | planned, below |
| W2e | Half-trickle on the client (protocol §2.5; C6) | planned, below |
| W2f | The core's key remembered and its fingerprints checked (protocol §2.5, §3.5; D290 §3, §6, D291 §6) | planned, below |
| W3 | WebSocket over a channel: Watch Together's hub, through a `togetherSocket` interface the WebSocket and `rtcapi.MessageConn` both meet; the core's close a binary message, so no session message is taken for it | built |
| W4 | Signalling and STUN through the discovery (the service and the owner's own), so a client reaches a core HTTPS cannot reach: since D274 the baseline every core is reached by. In three parts: | |
| W4a | The core: STUN servers from its discovery, so its answers carry its public (server-reflexive) candidates; each connection's path recorded (direct over IPv6 or IPv4, punched, relayed) for the log and the System page (D274 §3) | built 2026-10-10: `iceServers` in the discovery's status answer, `rtcapi.Server.UseICEServers`, `rtcapi.Path` counted and shown in System → Discovery. The client's own report of its path waits for W4c |
| W4b | `orpheum discovery`, the single-target discovery (D273 §3): the core's side of the protocol answered for one core, the client's (locate, paths, offer) and STUN, in this repository, so W4 is tested end to end and self-hosters have one | in progress: [discovery-mode.md](discovery-mode.md) |
| W4c | The web client served by a discovery: finding its core, offering through the discovery, racing its paths (W2c) | planned, below |
| W5 | A browser-tier test forced onto the WebRTC path through a discovery; a measurement over a real link | planned, below |
| W6 | The voice relay moved from pion/turn v4 to v5, so the binary carries one TURN library | planned, below |

Order: W2e and W2d first (they decide whether WebRTC through a discovery feels seamless, D290 §14),
then W2f once the core signs (discovery-core.md K3, K8b), W2c, and W4c on top; W5 tests them all;
W6 is independent.

### W2c — Choosing among the paths

`web/packages/sdk` (a new path chooser beside `SwitchableFetch`) and
`web/apps/web/src/transport-choice.ts`, which today moves a tab to WebRTC only on
`?transport=webrtc`:

1. Every path the discovery gave is tried at once; the LAN name only when `paths` returned it
   (D290 §2), with `targetAddressSpace: "local"` where Chrome supports it; WebRTC with one connection
   (protocol §3.3 item 1, C5).
2. **The first to answer becomes the control connection**: the page signs in and shows the household
   over it. Over it the page reads the core's API major and swaps to the build for that major if it
   holds another (§3.3 item 2, D289 §4).
3. The others get a few seconds (about 3 s, to measure) to succeed or fail (§3.3 item 3).
4. **Of those that succeeded, rank by kind**: the LAN, then IPv6, then IPv4, then WebRTC (D290 §2,
   D291 §3); the round trip, the median of a few small requests on the open connection, breaks
   ties, and a path more than three times and 50 ms worse than the best is demoted (D291 §3).
   Throughput is not measured (D290 §2).
5. **Move between requests**, never mid-sign-in or mid-chunk; fetches under way finish on their
   path; the Watch Together socket stays until its path fails, then reconnects with its session's
   resume (§3.3 item 5).
6. **Move again only on failure or a network change**: a request error, or two timeouts in a row,
   uses the next path still known to work and races again behind; `online`, network-change events
   and a page shown after a long hide race again (§3.3 item 6).
7. **Remember which paths worked, per network** (§3.3 item 7), so a later visit starts with them and
   survives the discovery being down; the last good LAN address kept for an outage (§7 item 18).
8. A line explaining Chrome's prompt before the LAN name is tried, and a help page for undoing a
   block (§7 item 14, U1). The service worker's own requests need the permission granted to its
   origin first (protocol §3.4).

`SwitchableFetch`'s comment still states D273 §7's replaced rule; it follows this section (C5).

**Tests.** With fake paths and a fake clock: the first to answer carries the sign-in; after the
window the LAN wins over IPv6 and IPv6 over WebRTC; a WebRTC answer first then an IPv6 one moves
traffic only between requests; a path 3× and 50 ms worse is demoted; a request already sent finishes
on its path; one failing request moves to the next known path and starts a race; a network change
starts a race; remembered paths are per network and tried first.
**VQ:** a new item: in Chrome at home, the LAN path is taken and its prompt appears once with its
explanation; away from home on the same device, no prompt appears.

### W2d — Channel recovery and ICE restart

`web/packages/sdk/src/rtc-transport.ts`. Today `connectRtc` opens four connections and needs all
four, and a failed connection, answering at once, draws every request in the least-loaded choice
(protocol §2.5, C6):

- The race opens one connection; once WebRTC is the chosen path the client opens the others,
  keeping each that connects and retrying the rest.
- It watches each connection's state, takes a failed one out of the rotation at once and replaces it.
- A connection `disconnected` for more than a few seconds is **restarted** (ICE restart, RFC 8445
  §9), its offer going through the discovery, or over its own data channel while it lives, the
  existing channels carrying on meanwhile; a `failed` one starts a new race.
- The core's side of a restart is discovery-core.md K3.

**Tests.** With fake peers: a transport is usable with one connection and grows to four; a third
connection that fails leaves the other three in use; a failed connection takes no new request; a
`disconnected` connection is restarted while its channels go on; a `failed` one starts a race.
**VQ:** a new item: a phone playing over WebRTC moves from Wi-Fi to mobile data and the film goes on
after a short stall.

### W2e — Half-trickle

The client sends its offer once its host candidates and its first server-reflexive candidate are in,
or after about a second, whichever comes first, rather than waiting for gathering to complete
(`gathered`, up to `connectTimeoutMs`' 10 s today) (protocol §2.5, C6). The core does the same for
its answer (discovery-core.md K3). **Tests:** with a fake peer, the offer is sent at the first
server-reflexive candidate, and after a second when none comes.

### W2f — The core's key remembered, its fingerprints checked

On first contact over a direct path the client asks the core to sign a nonce with its identity key
(discovery-core.md K8b), keeps the public key per `coreId` and checks it on every later visit; a
changed key stops the client before it sends a password, with a plain warning (protocol §3.5,
D290 §3). A key the old key signed is accepted (D291 §6); after a recovery without it, a plain notice
says the server's identity changed and names the date the owner confirmed it (D291 §6). Each answer
that came through a discovery is checked: its `fingerprintSignature` against the remembered key, or
the fingerprint a pairing QR carried (protocol §2.5, D290 §6). **Tests:** an answer whose fingerprint
the signature does not cover is refused and the connection closed; a changed key stops the sign-in
form; a key signed by the old one is taken quietly.

### W4c — The web client served by a discovery

`web/apps/web`, built for a discovery's origin as well as the core's (D274 §5; the paid spec §9.1
items 15–17):

- **Finding the core** (protocol §3.1): `/register`, `/join`, `/s` and `/p` read their `#…`, ask
  `locate` (spent by a click on the page, never by loading it), keep the device locator the core then
  hands over, and race the paths (W2c). A device that knows no core gets its own page (open your
  link; sign in with a phone that is; ask the person who runs your server) (D290 §10, U2).
- **Staying remembered** (D290 §10): *Add to Home Screen* offered on iPhones; persistent storage
  asked for; "Open in Safari" when an in-app browser is detected (§7 item 15).
- **Cross-origin** (protocol §3.4): the API's base the chosen path's origin (`createApi`'s
  `origin`); every byte path by the header (discovery-core.md K10); a service worker carrying a
  remote kill switch (C16).
- **Saying what happens**: "Connecting to …" after a second, a "how this device is connected" line
  (§7 item 21, U11), and when nothing reaches the core, the message of §3.6 written for whoever
  reads it (D290 §12).
- **The client's integrity** for the paid service (D290 §4, protocol §3.4): a reproducible build
  whose hash is published, Subresource Integrity on every script, `Integrity-Policy`, a strict
  content-security policy (`script-src 'self'`, no inline script), Trusted Types, no third-party
  script (§7 item 1). One build per supported core API major (D289 §4).
- **Device codes** on the paid tier: the device's side of discovery-core.md K15.

**Tests.** Component and SDK tests: each link page locates on a click and not on load; the no-core
page offers its three ways; a cross-origin API client sends the header and no cookie; the build has
no inline script and every script has an integrity hash. The browser-tier test is W4b.6/W5.
**VQ:** a new item: on an iPhone, from a registration link through the discovery, *Add to Home
Screen* is offered and the home-screen app finds the core a week later.

### W5 — Tests and the measurement

- A browser-tier test forced onto WebRTC through `single` ([discovery-mode.md](discovery-mode.md)
  W4b.6): a link, the race with direct paths blocked, sign-in, a film's chunks, a Watch Together
  socket, an ICE restart.
- A measurement over a real link: set-up time and throughput through a discovery at several round
  trips, and, in the beta, how often WebRTC fails by the client's kind of network (D274 §3; protocol
  §3.2), which nothing measures for browsers today (C10).

### W6 — One TURN library

The voice relay moved from `pion/turn/v4` to `v5`, which `pion/webrtc/v4` already brings in, so the
binary carries one TURN library; the owner's TURN servers in the heartbeat (discovery-core.md K13)
use the same credentials.

## Questions the plans raise

1. **Which connection a restart restarts** (W2d): discovery-core.md's question 7.
2. **The key remembered paths are kept under** (W2c): discovery-core.md's question 12.
3. **A first visit over WebRTC alone** (W2f): discovery-core.md's question 14.
4. **Counting WebRTC's failures in the beta** (W5): discovery-core.md's question 9.
