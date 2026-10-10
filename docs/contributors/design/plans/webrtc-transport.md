> **ABOUTME:** The API and the media carried over WebRTC data channels (D273 §7, §8): the direct
> path to a core behind carrier NAT whose client lacks IPv6. Every request is an ordinary request
> to the core's own handler, so nothing above the transport changes. The phases and where each
> stands.

# The WebRTC transport

- **Status:** IN-PROGRESS — begun 2026-10-09 on the owner's word (*"Ok let's add webrtc"*). W1, W2a,
  W2b and W3 are built.
- **Depends on:** D273, D274 (WebRTC through the discovery is the baseline), [reachability-approaches-2026-10-09.md](../research/reachability-approaches-2026-10-09.md)
  §9.4 (the measurement), the discovery service's spec (signalling through it).

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
  client opens four peer connections and spreads requests across them. The core tunes pion's
  window growth (`SetSCTPCwndCAStep`, 64 KiB).
- **Messages of at most 64 KiB**, which every browser carries.
- **The peer's address is the request's.** A request's remote address is the address of the ICE
  pair the connection selected, so rate limits and the playback speeds keep working (D257).
- **Authentication is the request's own**: a bearer token in its head; no cookies cross the
  channel. A connection itself grants nothing.
- **WebSocket** (the Watch Together hub) rides on a channel too: a request asking for an upgrade
  keeps its channel open, each channel message one WebSocket message.

## Phases

| Phase | What | State |
| --- | --- | --- |
| W1 | The core: `internal/core/rtcapi`, a peer per offer, each request channel handed to the core's handler with streamed, back-pressured responses; an offer endpoint on the core's API for a signed-in client (LAN and tests); bounds on connections. Tested with a pion client peer (`rtcapitest`) | built |
| W2a | The web client's API over channels: the SDK's `RtcTransport` over four connections and a `SwitchableFetch` the API client sends with; `?transport=webrtc` moves a signed-in tab onto it until W4 signals through the discovery | built |
| W2b | Media over channels: a service worker on the page passes what the browser loads itself (images, `<video>`, hls.js, subtitle tracks) to the page's transport, so no screen changes; credit-based flow control end to end (the core sends 4 MiB ahead of what the reader took). Not under Vite's dev server, which serves no `/sw.js` | built |
| W2c | The race (D273 §7): every path tried, the first to answer used at once, a better one answering a short while later taking over | |
| W3 | WebSocket over a channel: Watch Together's hub, through a `togetherSocket` interface the WebSocket and `rtcapi.MessageConn` both meet; the core's close a binary message, so no session message is taken for it | built |
| W4 | Signalling and STUN through the discovery (the service and the owner's own), so a client reaches a core HTTPS cannot reach: since D274 the baseline every core is reached by. In three parts: | |
| W4a | The core: STUN servers from its discovery, so its answers carry its public (server-reflexive) candidates; each connection's path recorded (direct over IPv6 or IPv4, punched, relayed) for the log and the System page (D274 §3) | built 2026-10-10: `iceServers` in the discovery's status answer, `rtcapi.Server.UseICEServers`, `rtcapi.Path` counted and shown in System → Discovery. The client's own report of its path waits for W4c |
| W4b | `orpheum discovery`, the single-target discovery (D273 §3; the spec's item 20): the core's side of the protocol answered for one core, the client's (locate, paths, offer) and STUN, in this repository, so W4 is tested end to end and self-hosters have one | |
| W4c | The web client: finds its core through the discovery, offers through it, and races the paths it is given (W2c), saying which it took | |
| W5 | A browser-tier test forced onto the WebRTC path; a measurement over a real link | |
| W6 | The voice relay moved from pion/turn v4 to v5, so the binary carries one TURN library | |
