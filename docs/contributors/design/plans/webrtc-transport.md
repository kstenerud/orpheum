> **ABOUTME:** The API and the media carried over WebRTC data channels (D273 §7, §8): the direct
> path to a core behind carrier NAT whose client lacks IPv6. Every request is an ordinary request
> to the core's own handler, so nothing above the transport changes. The phases, how the client
> chooses among its paths (discovery-protocol.md §3.3), and where each stands.

# The WebRTC transport

- **Status:** IN-PROGRESS — begun 2026-10-09 on the owner's word (*"Ok let's add webrtc"*). W1,
  W2a, W2b, W3 and W4a are built. W2c–W2f, W4c, W5 and W6 were revised 2026-10-13 to
  [discovery-protocol.md](../discovery-protocol.md) as D290, D291 and D292 left it. Since then the
  thin path is built and live (2026-10-14, on nas.example through the production service since
  2026-10-15): W2c's thin form, W2d but for ICE restart, W2f's offer proofs and answer
  checks, W4c's client at a discovery, since 2026-10-17 behind D308's loader
  ([client-from-core.md](client-from-core.md)), and W5's browser test. Left: ICE restart
  and the network-change triggers (discovery-core.md K3b, K4), W2e, W4c's remaining items, W5's
  measurement and W6; D313 puts the healing of connections on network changes in the closed beta's
  B0.
- **Depends on:** D273, D274 (WebRTC through the discovery is the baseline), D290 §2, §6, §14,
  D291 §3, D292 §4,
  [reachability-approaches-2026-10-09.md](../research/reachability-approaches-2026-10-09.md) §9.4
  (the measurement), [discovery-protocol.md](../discovery-protocol.md) §2.5 and §3,
  [discovery-core.md](discovery-core.md) (the core's side: K3, K7, K8b, K17, K18), [discovery-mode.md](discovery-mode.md) (W4b, the discovery every test here runs against).

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
- **Messages of at most 64 KiB**, which every browser carries; the core enforces it on receive too
  (protocol §2.5's bounds, K3a).
- **The peer's address is the request's.** A request's remote address is the address of the ICE
  pair the connection selected, so rate limits and the playback speeds keep working (D257).
- **Authentication is the request's own**: a bearer token in its head; no cookies cross the
  channel. A connection itself grants nothing. **The head is untrusted**: the page writes it, so the
  core strips `Origin`, `Host`, `Forwarded` and `X-Forwarded-*`, and no session minted over a
  channel carries admin (protocol §2.5, §3.7; D292 §4; K17).
- **WebSocket** (the Watch Together hub) rides on a channel too: a request asking for an upgrade
  keeps its channel open, each channel message one WebSocket message.
- **No discovery that only relays can sit in the middle.** Each answer that comes through a
  discovery carries the core's signature over its exact SDP, which the client checks against the
  identity key it remembers or the fingerprint its link carried (protocol §2.5, §3.5; D292 §4). The
  party that serves the client's JavaScript is trusted regardless (protocol §3.7).

## Phases

| Phase | What | State |
| --- | --- | --- |
| W1 | The core: `internal/core/rtcapi`, a peer per offer, each request channel handed to the core's handler with streamed, back-pressured responses; an offer endpoint on the core's API for a signed-in client (LAN and tests); bounds on connections. Tested with a pion client peer (`rtcapitest`) | built |
| W2a | The web client's API over channels: the SDK's `RtcTransport` over four connections and a `SwitchableFetch` the API client sends with; `?transport=webrtc` moves a signed-in tab onto it until W4 signals through the discovery | built |
| W2b | Media over channels: a service worker on the page passes what the browser loads itself (images, `<video>`, hls.js, subtitle tracks) to the page's transport, so no screen changes; credit-based flow control end to end (the core sends 4 MiB ahead of what the reader took). Not under Vite's dev server, which serves no `/sw.js` | built |
| W2c | Choosing among the paths (protocol §3.2, §3.3; D290 §2, D291 §3) | thin form built 2026-10-14: the discovery's page races the core's published names (`firstDirectPath`), takes the first to answer its health and sends its requests and media there (`directFetch`), else WebRTC; the rest below is planned |
| W2d | Channel recovery and ICE restart (protocol §2.5; C6) | built 2026-10-14: one connection first, growing to four, a lost one replaced; ICE restart waits for K3b (Phase 3), and until then a failed connection starts a new race |
| W2e | Early description on the client (protocol §2.5; C6, F2) | planned, below |
| W2f | The core's key and its signed answers checked (protocol §2.5, §3.5; D290 §3, §6, D291 §6, D292 §4) | built in part 2026-10-14: offers carry their proof, and answers are checked by the link's fingerprint; the rest below is planned |
| W3 | WebSocket over a channel: Watch Together's hub, through a `togetherSocket` interface the WebSocket and `rtcapi.MessageConn` both meet; the core's close a binary message, so no session message is taken for it | built |
| W4 | Signalling and STUN through the discovery (the service and the owner's own), so a client reaches a core HTTPS cannot reach: since D274 the baseline every core is reached by. In three parts: | |
| W4a | The core: STUN servers from its discovery, so its answers carry its public (server-reflexive) candidates; each connection's path recorded (direct over IPv6 or IPv4, punched, relayed) for the log and the System page (D274 §3) | built 2026-10-10: `iceServers` in the discovery's status answer, `rtcapi.Server.UseICEServers`, `rtcapi.Path` counted and shown in System → Discovery. The client's own report of its path waits for W4c |
| W4b | `orpheum discovery`, the single-target discovery (D273 §3): the core's side of the protocol answered for one core, the client's (locate, paths, offer) and STUN, in this repository, so W4 is tested end to end and self-hosters have one | in progress: [discovery-mode.md](discovery-mode.md) |
| W4c | The web client served by a discovery: finding its core, offering through the discovery, racing its paths (W2c), credentials per core | built in part 2026-10-14, and since 2026-10-17 served as each core's own client behind D308's loader ([client-from-core.md](client-from-core.md)); the rest below is planned |
| W5 | A browser-tier test forced onto the WebRTC path through a discovery; a measurement over a real link | the test built 2026-10-14 (`through-discovery.spec.ts`); the measurement planned, below |
| W6 | The voice relay moved from pion/turn v4 to v5, so the binary carries one TURN library | planned, below |

Order: W2f's checks and W4c's credentials per core in the security floor (Phase 1); then, for the
thin path (Phase 2), W2e and W2d's "keep what connects and replace what fails" (they decide whether
WebRTC through a discovery feels seamless, D290 §14), W2c, and W4c on top; W5 tests them all. W2d's
ICE restart, W2c's network-change triggers and W4c's remaining items come in the thickening
(Phase 3). W6 is independent.

### W2c — Choosing among the paths

`web/packages/sdk` (a new path chooser beside `SwitchableFetch`, imported by its own path, never
through `index.ts`, so revert-red's related set stays small, devloop §2) and
`web/apps/web/src/transport-choice.ts`, which today moves a tab to WebRTC only on
`?transport=webrtc`:

1. Every path the discovery gave is tried at once; the LAN name only when `paths` returned it
   (D290 §2); WebRTC with one connection (protocol §3.3 item 1, C5). Where the browser says whether
   the local-network permission is granted, denied or not yet asked, the client asks it first, and
   in a browser with no prompt the LAN name may be raced always (protocol §3.2; U2-8) [I]. *Built
   2026-10-20 (`web/apps/web/src/local-network.ts`): denied, the LAN name is never tried; still to ask,
   only the first race tries it, which has the browser ask, and the look for a better path leaves it
   out until granted, when it looks at once.*
2. **The first to answer becomes the control connection**: the page signs in and shows the household
   over it. Over it the page reads the core's API major and swaps to the build for that major if it
   holds another (§3.3 item 2, D289 §4).
3. The others get a few seconds (about 3 s, to measure) to succeed or fail (§3.3 item 3).
4. **Of those that succeeded, rank by kind**: the LAN, then IPv6, then IPv4, then WebRTC, and within
   WebRTC a direct pair, then a server-reflexive one, then the owner's TURN (D290 §2, D291 §3; C5);
   the round trip, the median of a few small requests on the open connection, breaks ties, and a
   path more than three times and 50 ms worse than the best is demoted (D291 §3). Throughput is not
   measured (D290 §2).
5. **Move between requests**, never mid-sign-in or mid-chunk; fetches under way finish on their
   path; the Watch Together socket stays until its path fails, then reconnects with its session's
   resume (§3.3 item 5).
6. **Move again only on failure or a network change**: a request error, or two timeouts in a row,
   uses the next path still known to work and races again behind; `online`, network-change events
   and a page shown after a long hide race again (§3.3 item 6).
7. **Remember which paths worked, per network** (§3.3 item 7), keyed on the `paths` answer's
   `clientPrefix` (protocol §3.2; D293 §8), so a later visit starts with them and survives the
   discovery being down. When nothing answers and the device seems to be at home, the
   page offers the core's home address as a link that leaves the discovery's origin, not as a path
   it races (protocol §3.1; D292 §6, replacing the last good LAN address).
8. A line explaining the browser's local-network prompt, and a help page for undoing a block (§7
   item 14, U1). *The line is built (2026-10-20): shown above the screen while the prompt waits,
   which the loader tells the client since its interface's minor 1; the help page is not. Firefox
   never uses the LAN name, and so never prompts (idiosyncrasies.md, the owner's choice 2026-10-20).* The service worker's own requests need the permission granted to its
   origin first (protocol §3.4).

`SwitchableFetch`'s comment still states D273 §7's replaced rule; it follows this section (C5).

**Tests.** With fake paths and a fake clock: the first to answer carries the sign-in; after the
window the LAN wins over IPv6, IPv6 over IPv4 and IPv4 over WebRTC; a WebRTC answer first then an
IPv6 one moves traffic only between requests; a path 3× and 50 ms worse is demoted; a request
already sent finishes on its path; one failing request moves to the next known path and starts a
race; a network change starts a race; remembered paths are per network and tried first; a denied
permission skips the LAN name.
**VQ:** a new item: in Chrome at home, the LAN path is taken and its prompt appears once with its
explanation; away from home on the same device, no prompt appears.

### W2d — Channel recovery and ICE restart

`web/packages/sdk/src/rtc-transport.ts`. Today `connectRtc` opens four connections and needs all
four, and a failed connection, answering at once, draws every request in the least-loaded choice
(protocol §2.5, C6):

- The race opens one connection; once WebRTC is the chosen path the client opens the others,
  keeping each that connects and retrying the rest.
- It watches each connection's state, takes a failed one out of the rotation at once and replaces
  it.
- A connection `disconnected` for more than a few seconds is **restarted** (ICE restart, RFC 8445
  §9), its offer going through the discovery, or over its own data channel while it lives, the
  existing channels carrying on meanwhile; a `failed` one starts a new race. Until the restart is
  built (Phase 3), a failed connection starts a new race.
- The core's side of a restart is discovery-core.md K3b.

**Tests.** With fake peers: a transport is usable with one connection and grows to four; a third
connection that fails leaves the other three in use; a failed connection takes no new request; a
`disconnected` connection is restarted while its channels go on; a `failed` one starts a race.
**VQ:** a new item: a phone playing over WebRTC moves from Wi-Fi to mobile data and the film goes on
after a short stall.

### W2e — Early description

The client sends its offer once its host candidates and its first server-reflexive candidate are in,
or after about a second, whichever comes first, rather than waiting for gathering to complete
(`gathered`, up to `connectTimeoutMs`' 10 s today) (protocol §2.5, C6). It is not RFC 8838's half
trickle, whose initiator gathers fully first (F2). The core does the same for its answer
(discovery-core.md K3b). **Tests:** with a fake peer, the offer is sent at the first
server-reflexive candidate, and after a second when none comes.

### W2f — The core's key, and its signed answers checked

`web/packages/sdk` (D292 §4; A7, A9):

- **First contact checks the fingerprint the link or QR carried** (protocol §3.1, §3.5): over a
  direct path the client asks the core to sign a nonce with its identity key (discovery-core.md
  K8b); over WebRTC the answer's signature verifies under that key. It keeps the public key per
  `coreId` and checks it on every later visit. A Safari tab whose storage was wiped re-learns it
  from the fingerprint in the person's own link.
- **Each offer carries its proof**, an HMAC over the offer's DTLS fingerprint keyed by
  `SHA-256(token)` of the client's session, or by the link's value for a bootstrap (protocol §2.5;
  A9).
- **Each answer is checked before it is used**: its `signature` over the exact answer SDP (with the
  `offerId` and the client's own fingerprint) against the remembered key
  or the fingerprint the link or QR carried, which is the identity key's, never a DTLS one (X7); an
  answer with more than one `a=fingerprint` or any hash but sha-256 is refused; the exact signed
  string is what goes to `setRemoteDescription` (protocol §2.5 item 4).
- **A changed key**: one the old key signed is accepted (D291 §6); one it did not sign blocks the
  client before it sends a password, until the person opens a fresh link from the core carrying the
  new fingerprint, never accepted on the discovery's word (D292 §4, revising D291 §6's notice).

What this enforces is WebRTC's: on HTTPS a page cannot pin the server's certificate, so a signed
nonce stops only an impostor that cannot reach the real core (protocol §3.5; T3).

**Tests:** an answer whose signature does not cover its exact SDP is refused and the connection
closed; one with a second fingerprint line is refused; a link whose fingerprint does not match the
core's key stops before any secret is sent; an unsigned key change stops the sign-in form until a
link with the new fingerprint is opened; a key signed by the old one is taken quietly.

### W4c — The web client served by a discovery

`web/apps/web`, built for a discovery's origin as well as the core's (D274 §5; the paid spec §9.1
items 15–17); in the beta served by the service itself at `app.orpheum-app.com`, and on staging at
`app-staging.orpheum-app.com`, whose builds never register at the production origin (D292 §2–§3;
coherence §5.3):

- **Credentials per core** (protocol §3.4; D292 §4; A8, W1): every credential, device id, device
  secret, locator and remembered key kept under its `coreId`, not one `orpheum.session` for the
  origin; an API client bound to one core attaching its credential only to that core's names or its
  WebRTC transport; a URL a playlist names elsewhere sent without it; joining another household's
  Watch Together anonymous there unless signed in to that core. The device proof is the device
  secret of discovery-core.md K18, never the session token.
- **Finding the core** (protocol §3.1): `/register`, `/together`, `/s` and `/p` read their `#…`
  (`<coreId>.<secret>.<fingerprint>`), check the fingerprint (W2f), ask `locate` (spent by a click
  on the page, never by loading it), keep the device locator the core then hands over, and race the
  paths (W2c). A used registration link opens sign-in as the person's own link, not "used" (D292
  §4). A device that knows no core gets its own page (open your link; paste your server's link;
  sign in with a phone that is; ask the person who runs your server) (D290 §10, U2; U2-2).
- **Staying remembered** (D290 §10; U2-2): *Add to Home Screen* offered on iPhones from the person's
  own-link page, the installed app starting there [I: a device test]; "Open in Safari" shown before
  the click that spends the locate, when an in-app browser is detected (§7 item 15; U2-1). Asking
  for persistent storage does not keep a Safari tab's storage, so it is not offered as a remedy
  (F8).
- **Cross-origin** (protocol §3.4): the API's base the chosen path's origin (`createApi`'s
  `origin`); every byte path by the header (discovery-core.md K10); a service worker carrying a
  remote kill switch (C16), allowing only the content types a byte path serves and adding
  `Content-Security-Policy: sandbox` and `nosniff` to what it returns (W11) [I].
- **Admin**: a session made through the discovery's client has no admin (protocol §3.7;
  discovery-core.md K17); the client reads the session's rights, not the role's, and admin pages say
  "manage this server from home, at <its home address>".
- **Saying what happens**: "Connecting to …" after a second, a "how this device is connected" line
  (§7 item 21, U11), and when nothing reaches the core, the message of protocol §3.6 written for
  whoever reads it (D290 §12), its "copy what happened", and the home address when at home.
- **The client's integrity** for the paid service (D290 §4, protocol §3.4): a reproducible build
  whose hash is published, Subresource Integrity on every script, `Integrity-Policy`, a strict
  content-security policy (`script-src 'self'`, no inline script), Trusted Types, no third-party
  script (§7 item 1); `index.html` and `/sw.js` small, served `no-cache`, the worker registered with
  `updateViaCache: "none"`, their hashes published (W2) [I]. These stop a CDN or cache swap and make
  tampering visible, not a compromised service (protocol §3.4). One build per supported core API
  major (D289 §4).
- **Device codes** on the paid tier, in the beta (D293 §2, revising D292 §3's deferral): the
  device's side of discovery-core.md K15: its ephemeral P-256 key made, and the blob opened by HPKE
  (DHKEM(P-256, HKDF-SHA256)) on Web Crypto's own P-256 and HKDF, no library, checked against RFC
  9180's vectors; the page entering a code warns "only enter a code shown on a screen in front of
  you".

**Tests.** Component and SDK tests: each link page locates on a click and not on load, and refuses
a link whose fingerprint fails; the no-core page offers its ways; a cross-origin API client sends
the header and no cookie, and core A's credential never goes to core B's names; a session without
admin shows no admin menus; the build has no inline script and every script has an integrity hash.
The browser-tier test is W4b.6/W5.
**VQ:** a new item: on an iPhone, from a person's own link through the discovery, *Add to Home
Screen* is offered and the home-screen app opens at the core, signed in after one sign-in there.

### W5 — Tests and the measurement

- A browser-tier test forced onto WebRTC through `single` ([discovery-mode.md](discovery-mode.md)
  W4b.6): a link, its fingerprint checked, the race with direct paths blocked, sign-in, a film's
  chunks, a Watch Together socket, and later an ICE restart. It is Orpheum's one discovery browser
  test in check-pr; the netlab tier runs the race across home-network shapes on demand (D292 §9).
- A measurement over a real link: set-up time and throughput through a discovery at several round
  trips, and, in the beta, how often WebRTC fails by the client's kind of network (D274 §3; protocol
  §3.2), which nothing measures for browsers today (C10).

### W6 — One TURN library

The voice relay moved from `pion/turn/v4` to `v5`, which `pion/webrtc/v4` already brings in, so the
binary carries one TURN library; the owner's TURN servers in the heartbeat (discovery-core.md K13)
use the same credentials.

## Questions the plans raise

1. **Which connection a restart restarts** (W2d): discovery-core.md's question 7.
2. **The key remembered paths are kept under** (W2c): discovery-core.md's question 12. *Answered*
   by D293 §8: the `paths` answer's `clientPrefix`.
3. **Counting WebRTC's failures in the beta** (W5): discovery-core.md's question 9. *Answered* by
   D293 §7: the client reports a dead end to the discovery, which counts per core per week.

A first visit over WebRTC alone (W2f) is answered: the link's fingerprint (D292 §4).
