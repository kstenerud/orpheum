> **ABOUTME:** An audit of every path a device takes to its core, at home and away, per browser, as
> built on 2026-10-28, set beside how Plex, Jellyfin, Emby, Home Assistant, Immich, Synology,
> Syncthing, Tailscale, the HLS-over-P2P loaders and others solve the same problems, read from their
> code, their served bundles and what their engineers published. Ranks what we do wrong, says what
> we do right, and proposes a target with its steps and costs. It proposes; it decides nothing.

# The network stack: an audit against peers

- **Date:** 2026-10-28, on an earlier commit. **Asked by** the owner: *"I also want an audit of our network
  stack as it is now. We've got to be doing something wrong here. Consult expert opinions and even
  look at js code by other orgs in a similar space."*
- **Trigger:** at home, the Fire TV (Silk 138) reaches the core over a WebRTC data channel on a
  direct LAN pair and loads media at 15–30 Mb/s; the same stick, opening the core's LAN name
  (`https://192-168-1-199.<id>.orpheum-test.com:8443`) as a top-level page, loads at 190–224 Mb/s.
- **Not here:** why the data channel itself is slow (pion's unpaced SCTP bursts overflowing the
  browser's socket), which [webrtc-throughput-2026-10-28.md](webrtc-throughput-2026-10-28.md)
  covers; the client's JavaScript costs, which
  [web-client-efficiency-audit-2026-10-28.md](web-client-efficiency-audit-2026-10-28.md) covers.
  This audit is about the architecture and its choices.
- **Builds on:** [reachability-approaches-2026-10-09.md](reachability-approaches-2026-10-09.md) (the
  layers and the first peer survey), [direct-reachability.md](direct-reachability.md),
  [local-path-2026-10-27.md](local-path-2026-10-27.md), [house-ipv6-2026-10-27.md](house-ipv6-2026-10-27.md),
  [discovery-protocol.md](../discovery-protocol.md) §3, [webrtc-transport.md](../plans/webrtc-transport.md),
  [client-from-core.md](../plans/client-from-core.md).
- **Method:** read the client's path code (`web/apps/web/src/discovery-start.ts`, `local-network.ts`,
  `kept-path.ts`, `sw.ts`; `web/packages/sdk/src/rtc-transport.ts`, `rtc-media.ts`), the core's
  `internal/core/rtcapi`, `discovery` and `portmap` packages, the sibling `orpheum-connect`
  (`internal/origins`, `internal/webclient`, `internal/signals`), the decisions (D243, D263, D273,
  D274, D290–D292, D297, D300, D308, D314, D354, D356) and `orpheum-ops`. Peers: their source,
  cloned to `/yoloai/cache/git/` (`python-plexapi`, `jellyfin-web`, `jellyfin-sdk-typescript`,
  `Emby.ApiClient.Javascript`, `snitun`, `hass-nabucasa`, `p2p-media-loader`, `simple-peer`,
  `webtorrent`, `filepizza`, `tailscale`, `syncthing`); the web clients Plex and Emby serve to every
  browser, fetched as a browser would on 2026-10-28 and kept in
  `/yoloai/cache/http/network-audit/bundles/` (no sign-in, no private API called); and the
  browser makers' and engineers' own writing, kept in `/yoloai/cache/http/network-audit/`.
- **Labels:** **[C]** read in Orpheum's code or config; **[L]** seen on a device or in a log (the
  owner's tests and the throughput study); **[V]** checked at its source, cited; **[B]** read in a
  closed product's served JavaScript bundle; **[I]** inference.

---

## 1. The plain answer

**Yes, one thing is wrong, and it is structural: at home, Orpheum carries all of a film's bytes
through a tunnel built for crossing NATs, when every peer carries them over plain HTTP(S) to the
server's LAN address.** The rest follows from where the client's page lives. Ranked by what it costs
the person, with the peer evidence:

| # | What we do | Why it is wrong or costly | What peers do | Fix, cost |
| --- | --- | --- | --- | --- |
| 1 | **At home, Silk and Firefox reach the core only over WebRTC data channels** (Firefox by design, `usesLanName`; Silk because its local-network permission sits at `prompt` with no prompt shown) **[C, L]** | 15–30 Mb/s against 190–224 Mb/s on the same TV; a 4K rung does not fit; the TV is the living room's main device | **None of the surveyed peers carries its own HTTP or media over data channels, at home or away** (§3). Jellyfin, Emby, Home Assistant, Immich and Synology serve the client from the server, same-origin; Plex calls a per-IP HTTPS name (`plex.direct`) **[V, B]** | Send a browser at home to the core's own origin at its LAN name, by a top-level navigation carrying a D308 hand-off code: proven at full speed on Silk **[L]**; small–medium (§5 step 1) |
| 2 | **The client's page lives at a public origin** (`<core id>.orpheum-client.com`, D308), so every request to the LAN is a public page reaching a private address | It is what puts Chrome's and Firefox's Local Network Access in front of the fast path, and what Silk cannot pass at all | Plex is the one major peer with a central web origin, and it took the same hit: Chrome's prompt, and on refusal *"it would instead have to try and make a remote or indirect connection"* (Plex staff, 2025-11-06) **[V]**. Emby's hosted `app.emby.media` is the other **[B]**. Everyone else serves the client from the server | Keep the discovery's origin for the first contact and for away-from-home without a direct name; run the client at the core's own names wherever one answers (§5 step 1–2); medium |
| 3 | **WebRTC data channels are the baseline path for everything** (D274 §2), API, posters and every chunk | A data channel is a NAT-traversal transport: unpaced loss-based SCTP in pion, no BBR (which D263 chose for HTTP media for its loss handling), a 64 KiB message framing of our own, a service-worker relay | Every HLS-over-P2P product keeps HTTP primary and uses data channels only as offload: p2p-media-loader *"should not perform worse than a player configured without P2P at all"* (`FAQ.md:54`) **[V]**. The one production system whose whole transport is a data channel is Tor's Snowflake, built for reach under censorship, not throughput **[V]** | Rank WebRTC as the last direct path, for the CGNAT household only, as D273 §7 first ranked it; pace it (the throughput study's fix 1) for the cases left; no new build beyond step 1 |
| 4 | **Production has no way in for an IPv4-only visitor whose punch fails, and no relay** (D292 §5, D314 §4) | A phone on IPv4-only mobile data or a hotel network may not reach nas.example at all; the person sees "nothing reaches the server" | Every peer aimed at non-technical people has a relay fallback, capped and discouraged for media: Plex Relay (reported 1–2 Mb/s), Synology QuickConnect's relay, Tailscale's DERP and peer relays, Syncthing's relays, Nabu Casa's SNI proxy **[V]**. Those without one (Jellyfin, Immich, PhotoPrism) tell the owner to run a proxy or a VPN **[V]** | Not wrong as a beta stance: D274 §3 counts the failures to decide it. If the count calls for it, TURN on the owner's VPS or ours, end-to-end encrypted; medium (§5 step 4) |
| 5 | **The browser's own loads go through a service worker that asks the page to fetch them over its transport** (`sw.ts`, `rtc-media.ts`) | Not the throughput limit (measured, the throughput study §3), but it makes every image and chunk depend on the page's main thread and defeats the HTTP cache (the efficiency audit's finding 1: a film page's preload fetched twice) | p2p-media-loader and the commercial P2P loaders plug into hls.js as a loader, not a service worker **[V]** | Low priority once step 1 lands, since the WebRTC path then carries little at home; an hls.js loader for chunks if the WebRTC path stays busy; medium |
| 6 | **The home fallback rests on WebRTC being outside Local Network Access**, which the standard's own explainer says will change | The day Chrome gates WebRTC's local candidates, Silk (no prompt) loses the LAN pair too, and Firefox's at-home path with it | Not met by peers, who use no WebRTC | Step 1 removes the dependency: a same-origin page at the LAN name is outside the check entirely; none now |

**What is right, and matches the best peers:**

- **Per-IP LAN names with a real certificate** (`192-168-1-5.<id>.orpheum-core.com`) are Plex's
  `plex.direct` scheme, wildcard per server **[V: words.filippo.io, 2015]**; the core holds its own
  key (D292 §1), which Plex's design also does.
- **Racing paths and ranking them by kind** (D273 §7, D290 §2, D291 §3) is what Plex's clients do
  (`python-plexapi` orders `['local', 'remote', 'relay']` × `['https', 'http']`, `myplex.py:1459`,
  and races them in threads) **[V]**, what Jellyfin's and Emby's connection managers do (local at
  0 ms, manual at 100 ms, remote at 200 ms, *"a small hack to try and ensure the remote address doesn't
  resolve first"*, `jellyfin-web/src/lib/jellyfin-apiclient/connectionManager.js:474–528`) **[V]**,
  and what Syncthing (TCP-LAN 10 … relay 50, `lib/config/optionsconfiguration.go:77–81`) and
  Tailscale (`betterAddr`, extra points for loopback, link-local and private addresses,
  `wgengine/magicsock/endpoint.go:2065, 2113–2122`) do **[V]**. The ranking is right; what is wrong is
  that the fastest rank is out of reach for two of our browsers (row 1).
- **Port mapping by PCP, NAT-PMP and UPnP** (`internal/core/portmap`), as Plex does; **IPv6 first**
  (D274 §4), which no surveyed media peer handles as well (Plex's DS-Lite users have asked for it
  since 2012, house-ipv6 §2) **[V]**.
- **Logging the path and its reasons** (D274 §3, D354 §5): more than any peer exposes.
- **64 KiB messages and credit-based flow control**: simple-peer, p2p-media-loader and PairDrop all
  converged on 64 KiB (`simple-peer/index.js:10`; `p2p-media-loader-core/src/core.ts:63`) **[V]**. The
  framing is not the problem.
- **The core signs its DTLS fingerprint** (D290 §6), so the discovery cannot sit in the middle:
  stronger than Plex's model, where plex.tv brokers every connection.

---

## 2. Every path, as built

### 2.1 How the path is chosen today [C]

A page at the client origin (D308) learns the core's paths from the discovery (`POST
/client/v1/paths`): `directUrls` (the core's published names: LAN, IPv6, mapped IPv4) and `lanUrls`
(the LAN names among them), the LAN name included only when the device *seems at home* by the
discovery's `isAtHome` (D290 §2, D356 §5). `reachCore` (`web/apps/web/src/discovery-start.ts`) then:

1. reads the local-network permission (`local-network.ts`); **in Firefox it is always "denied"**
   (`usesLanName` returns false for any Firefox, the owner's choice of 2026-10-20);
2. races the direct names that the permission allows (`firstDirectPath`); with the permission at
   `prompt`, it sends the LAN name one request apart to raise the prompt and races without it;
3. falls back to WebRTC through the discovery when no name answers;
4. looks for a better path every 30 s and on `online` (`kept-path.ts`), backing off to 10 minutes
   after moves away from a failed path, and gives a path up after two failures until a reload.

On WebRTC the page holds up to four peer connections (`clientConnections`), one data channel per
request, 64 KiB messages, credit granted 16 messages at a time; the service worker hands the
browser's own loads (images, hls.js chunks, subtitles, even the client's own files under the loader)
to the page, which fetches them over the transport (`sw.ts`, `rtc-media.ts`).

### 2.2 The paths, one by one

Throughputs are the throughput study's on one machine unless marked; the TV's are the owner's.

| Path | When it is taken | Throughput, set-up | Fails when | What the person sees | What is logged |
| --- | --- | --- | --- | --- | --- |
| **A. The core's own origin at its LAN name, top-level** (the "home address", protocol §3.1) | only when the person opens the link from the System or account page; never raced, never offered automatically (protocol §3.1: *"It is not a path the client races"*) | **190–224 Mb/s on the Fire TV [L]**; HTTP/2 over TCP with BBR (D263); no set-up beyond TLS | DNS rebinding protection drops the name; the device is not on the LAN | a browser error page; the person must find the link again | the core's ordinary request lines; no path line, as no discovery is involved |
| **B. Client origin → LAN name, cross-origin fetch** | Chrome, Edge, Chrome Android, Safari, when the discovery judges the device at home and the permission is `granted` or `unknown` | HTTP speed (240–2000 Mb/s in the study) [L] | Chrome's prompt refused; **Silk: `prompt` forever** [L]; **Firefox: never tried**; rebinding protection | a line saying why the browser asks (W2c item 8); otherwise silently WebRTC | `POST /client-paths`: family, permission, race outcomes, the path taken (D354 §5) |
| **C. Client origin → the core's IPv6 name** | when the core publishes one (its router lets the port in) and the device has IPv6 | HTTP speed; a public address, so **no Local Network Access check and no rebinding block** [V] | no IPv6 on the device (many hotels, offices, some mobile networks); the router's IPv6 firewall | as B | as B |
| **D. Client origin → mapped public IPv4** | a public IPv4 and a mapping | HTTP speed; at home it hairpins through the router | **impossible behind DS-Lite or carrier NAT**, which is nas.example and the owner's home [L] | as B | as B |
| **E. WebRTC, direct host pair on the LAN** (D300) | Firefox at home; Silk at home; any browser whose B failed | **15–30 Mb/s on the Fire TV [L]**; 25–60 Mb/s at 20–50 ms in the study; first byte ~0.45 s at 50 ms round trip after ICE | the core's UDP port not published (staging until fixed, local-path cause 2) → hairpin (G); **a future Chrome that gates WebRTC's local candidates** [V: WICG explainer] | nothing tells them they are on the slow path; films start lower and climb slowly | `rtcapi`'s connection line (`path=direct_ipv4`), counts on the System page, the page's pairs at 5 s |
| **F. WebRTC, punched** | away, behind NATs a punch crosses | the study's curve: about one window (~170 KB) per round trip, so ~25 Mb/s at 50 ms | symmetric NAT on either end; UDP blocked | "Connecting…", then the dead-end message (D290 §12) | the path and the failure, counted (D274 §3, D314 §4) |
| **G. WebRTC, hairpinned** (D354) | a device at home whose LAN pair failed, out to the carrier's NAT and back | bound by the house's uplink (43.7 Mb/s for the owner) [L] | — | nothing; the admin is warned | a warning line; the doctor's finding |
| **H. A relay** | none in production; staging's and self-hosters' TURN only (D274 §5) | — | — | "nothing reaches the server" | counted as failed (D314 §4) |

### 2.3 Per browser, at home, today

| Browser | Path at home | Why | Speed |
| --- | --- | --- | --- |
| Chrome / Edge desktop | B once the prompt is allowed; E before | Chrome 142's Local Network Access prompt [V] | HTTP |
| Chrome Android | B after the prompt [V: in the same trial as desktop] | as above | HTTP |
| **Silk (Fire TV)** | **E** | the permission reads `prompt` and no prompt appears [L]; the most likely cause is a Chromium 138 fork with the blocking check and no prompt UI, which landed in 142 [I] | **15–30 Mb/s** |
| **Firefox** | **E** | `usesLanName` is false: a service-worker-controlled page's loads to the LAN name fail even with the permission granted (idiosyncrasies.md, "Firefox fails a public page's requests to a LAN name…"); Mozilla's own service-worker LNA changes (bug 1996495, fixed in 153) are the likely neighbourhood [V, I] | data channel |
| Safari (macOS, iOS) | B | no per-page prompt; one OS-level "Local Network" switch per app [V: Apple DTS, developer.apple.com/forums/thread/811690] | HTTP |

So the two browsers the owner uses most in the living room and on the Mac (Silk, Firefox) are the two
that never get HTTP at home.

### 2.4 Away from home

- **Dual-stack visitor, IPv6 core:** C, at HTTP speed, no prompt. The best path we have; how often
  nas.example publishes it, and the router's IPv6 firewall, decide whether it is there.
- **IPv4-only visitor (many hotels, some mobile carriers):** F if the punch works, else nothing.
  The protocol's own estimate: a phone on mobile data may fail *"well over 30% of the time"* against
  a WebRTC-only household (protocol §3.2, **[I]** there, from field data on symmetric NATs).
- **What a peer would add here:** a relay, capped (row 4).

---

## 3. The peers

### 3.1 Media servers and self-hosted apps

- **Plex.** At home, HTTPS to `<dashed-ip>.<server hash>.plex.direct`, a wildcard certificate per
  server, the name resolving to the LAN address by public DNS **[V: words.filippo.io/how-plex-is-doing-https-for-all-its-users,
  2015]**. The web app at `app.plex.tv` calls it cross-origin. Away: UPnP or a manual port, else
  Plex Relay, capped (forum reports of 1 Mb/s free, 2 Mb/s with Plex Pass; Plex's support page could
  not be fetched, 403) **[V, second-hand]**; the served bundle carries a connection model with
  `relay` and `local` flags and lowers quality when `maxUploadBitrateReason` is `"relay"` **[B:
  app.plex.tv main bundle, fetched 2026-10-28]**. Chrome's Local Network Access hit it head on: *"If
  you deny the prompt, the web app will not be allowed to make a local connection to the Plex Media
  Server, meaning that it would instead have to try and make a remote or indirect connection to the
  server"*, and a person without Plex Pass who blocks it is offered a paid remote pass **[V:
  forums.plex.tv/t/933264, 2025-11-06]**. **No `RTCPeerConnection` or data channel anywhere in its
  web bundle [B].**
- **Jellyfin.** The client is served by the server, same-origin. At home a plain `http://<LAN
  address>:8096`; no LAN certificate scheme. Its connection manager races local, manual and remote
  addresses with staggered starts (`connectionManager.js:474–528`) **[V]**; the newer TypeScript SDK
  scores candidates instead (`jellyfin-sdk-typescript`, `discovery-service.ts`) **[V]**. Away: the
  owner's reverse proxy or VPN; no relay (*"you must manually expose it to the internet"*,
  jellyfin.org/docs/general/post-install/networking) **[V]**. No WebRTC in jellyfin-web or the
  server **[V: grep]**.
- **Emby.** Emby Connect *"simply helps the device locate that server"*; connections are direct
  (emby.media/connect.html) **[V]**. Its `connectionmanager.js` has the same staggered race Jellyfin
  inherited **[V]**. It also serves a hosted client at `app.emby.media` **[B]**, Plex's pattern. No
  WebRTC **[V]**.
- **Home Assistant + Nabu Casa.** The frontend is served by the instance itself. The companion apps
  keep an internal and an external URL and choose by the home Wi-Fi's name **[V:
  companion.home-assistant.io]**. Remote UI is `snitun`: a TLS proxy that routes by the ClientHello's
  SNI without ending TLS (`snitun/server/listener_sni.py:30`), the instance holding its own ACME
  certificate (`hass_nabucasa/remote.py`) **[V]**: an end-to-end-encrypted relay, slow for media by
  forum reports **[V, second-hand]**. No WebRTC for HTTP.
- **Immich, PhotoPrism, Nextcloud.** Same-origin clients; away by reverse proxy or Tailscale
  (docs.immich.app/guides/remote-access) **[V]**. Immich's mobile app switches to a local endpoint by
  Wi-Fi name, not by a race, and has had bugs doing so (immich issues 21930, 32481) **[V]**.
  Nextcloud Talk uses WebRTC with TURN for calls only, never for files or the API **[V]**.
- **Synology QuickConnect.** Tries the direct LAN and WAN addresses, then a hole-punch the broker
  arranges, then Synology's relay, explicitly slower, with DDNS and a port forward advised when it is
  **[V: Synology's QuickConnect white paper, via search]**; no WebRTC; its DSM page is served by the
  NAS.

### 3.2 Bulk data over WebRTC data channels

- **p2p-media-loader** (Novage): an hls.js and Shaka *loader* plugin; HTTP primary, P2P only for
  segments another peer already holds (`FAQ.md:48, 54`); 64 KiB − 1 messages
  (`p2p-media-loader-core/src/core.ts:63`), `bufferedamountlow`-driven sending
  (`webtorrent/data-channel-sender.ts`) **[V]**. Peer5, Streamroot, CDNBye and SwarmCloud are the same
  shape, CDN first; only Teleport puts P2P first, still with the CDN behind it **[V]**.
- **WebTorrent, simple-peer, FilePizza, PairDrop, ShareDrop**: data channels are their only path
  because both ends are browsers. None publishes throughput; ShareDrop plateaued at 30 MB/s tab to tab
  with 1 MB chunks and ack-per-chunk, fixed by 64 KB and `bufferedAmountLowThreshold` (ShareDrop issue
  154) **[V]**.
- **Tor's Snowflake** is the one production system whose whole transport is a data channel: 2.7 Gb/s
  in aggregate over about 35,000 users in March 2024 (bamsoftware.com/papers/snowflake), about 80 kb/s
  a user. Its job is reach under censorship **[V]**.
- **neko, go2rtc, Cloudflare Realtime, LiveKit**: media on RTP tracks; data channels for control,
  chat and telemetry (LiveKit: reliable packets ≤ ~15 KiB, docs.livekit.io) **[V]**. **Tailscale's
  browser SSH** runs DERP over a WebSocket from WebAssembly, not WebRTC (tailscale.com/blog/ssh-console)
  **[V]**.
- **The browser makers' own writing**: Firefox's data channels once capped throughput at a 128 KiB
  SCTP window over the round trip (Mozilla bug 1051685) and added 40–100 ms of latency on a LAN from
  Nagle on SCTP (bug 976115) **[V]**. Chrome presents WebTransport, not data channels, as the
  client-to-server bulk transport (developer.chrome.com/docs/capabilities/web-apis/webtransport) **[V]**.

**So nobody surveyed serves a video app's HTTP over data channels as its main path, at home or
away [V, within this survey].** Those who move video over data channels keep HTTP as the floor; those
who must cross NATs without a relay use an overlay (Tailscale) or a hole-punched native protocol
(Synology), not a browser data channel.

### 3.3 How the rest cross NAT

- **Tailscale** starts every connection on DERP and upgrades to a direct path *"after a few
  seconds"* (tailscale.com/blog/how-nat-traversal-works) **[V]**; trust in a path is renewed by pings
  (`trustBestAddrUntil`, `endpoint.go`) **[V]**; peer relays (2025) for networks where DERP is too
  slow **[V]**. The relay is the floor, never a failure.
- **Syncthing**: local discovery by broadcast every 30 s (`lib/discover/local.go:51`), global
  discovery, relays by default but used only when direct fails, with periodic retries of direct
  (docs.syncthing.net/users/relaying.html) **[V]**.
- **Cloudflare Tunnel**: video through Cloudflare's CDN or Tunnel without a Cloudflare media product
  may be restricted (Cloudflare's 2023 terms) **[V]**; idiosyncrasies.md already records it.

### 3.4 The comparison

| Peer | At home | Away, through NAT | Relay | Client's origin | HTTP over data channels |
| --- | --- | --- | --- | --- | --- |
| **Orpheum** | LAN name cross-origin (Chrome, Safari); **data channel (Silk, Firefox)** | IPv6 name; mapped IPv4; punched data channel | none (beta) | public, per core (D308) | **yes, as the baseline** |
| Plex | `plex.direct` LAN name, cross-origin, prompt | UPnP/port, IPv6 names | Plex Relay, capped | public (`app.plex.tv`), plus the server's own `/web` | no |
| Jellyfin | `http://` LAN address, same-origin | owner's proxy or VPN | none | the server | no |
| Emby | as Jellyfin | owner's proxy | none | the server; also `app.emby.media` | no |
| Home Assistant | internal URL by Wi-Fi name | Nabu Casa's SNI proxy, end-to-end TLS | the proxy is the path | the instance | no |
| Immich / PhotoPrism / Nextcloud | same-origin, local URL by Wi-Fi (Immich) | proxy or Tailscale | none | the server | no (Talk's calls only) |
| Synology QuickConnect | direct LAN | broker hole-punch (native) | Synology's, slower | the NAS | no |
| Syncthing / Tailscale | LAN by discovery / by path score | hole punch | relays / DERP, always there | native | no |
| p2p-media-loader & CDN P2P | — | — | the CDN is the floor | the site | segments only, HTTP primary |

---

## 4. Browser constraints

| Mechanism | Chrome desktop | Chrome Android | Silk (Fire TV) | Firefox | Safari |
| --- | --- | --- | --- | --- | --- |
| Public page `fetch` to a private address | prompt since 142 (2025-10) [V] | same, in the same trial [V] | `prompt`, none shown [L]; Chromium 138 base [L] | prompt (149 → everyone by 151–153) [V/I]; service-worker loads unreliable [L] | no page prompt; OS switch per app [V] |
| WebSocket to a private address | gated from 147 [V] | same [I] | unknown | covered by `network.lna.*` [I] | none |
| WebRTC to a private address | **not yet gated; planned** [V] | same | works [L] | works [L] | none |
| Service worker's own fetches | need the permission granted to the worker's origin first [V] | same | unknown | changed in 153 (bug 1996495) [V] | — |
| **Top-level navigation to a private address** | **not covered** (fetch, subresources and *subframe* navigation are) [V]; named *"a risk"* and future work, perhaps an interstitial [V: WICG explainer] | same | **works, 190–224 Mb/s** [L] | not covered [I] | not covered [I] |
| Same-origin requests on a private-address page | outside the check: it is public → private that is gated [V] | same | same | same | — |
| Mixed content | a private IP literal or `.local`, or `targetAddressSpace: "local"`, is exempt [V]; Orpheum's names are HTTPS anyway | same | same | — | — |

Sources: developer.chrome.com/blog/local-network-access (2025-06-09, updated for Chrome 142) **[V]**;
github.com/WICG/local-network-access explainer **[V]**; bugzilla.mozilla.org 1996495, 2059274 **[V]**;
MDN, Mixed content **[V]**. Kept in `/yoloai/cache/http/network-audit/`.

What this means:

- **A public page cannot rely on reaching a LAN name**: Chrome asks, Firefox asks and then misbehaves
  under a service worker, Silk cannot ask.
- **A page at the LAN name itself is outside every check**: its requests are same-origin to a
  private address. That is why Silk ran at full speed there, and why Firefox's fault would not apply.
- **Top-level navigation is today's open door, and it is the door the standard names as a risk.**
  An interstitial would cost a click, not the path; WebRTC's gating, which is planned outright, would
  close today's fallback for Silk.
- **Each origin is its own world**: cookies, storage and the service worker do not carry from the
  client origin to the LAN name. A session crosses by a one-use code, which D308 §4 already built
  (`POST /session/handoff`, `/session/redeem`, client-from-core.md §2.1).
- **DNS rebinding protection** (dnsmasq's `stop-dns-rebind`, the FRITZ!Box, pfSense, Pi-hole,
  NextDNS) refuses public names that resolve to private addresses, ours and Plex's alike; Plex tells
  its users to exempt `plex.direct` (reachability-approaches §3, B1) **[V]**. A navigation to a name
  that does not resolve shows the browser's own error page, which a fetch's failure does not.

---

## 5. Target architecture

### 5.1 The shape

**The client runs at the core's own origin whenever a direct name answers; the discovery's origin is
for first contact and for the household no direct name reaches.** In order:

1. **At home**: the core's own origin at its LAN name (path A), plain HTTPS, same-origin, BBR. Every
   browser, Silk and Firefox included. Admin works there, as the requests come from the house
   (D297 §6).
2. **Away, with IPv6 on both ends**: the client origin calling the core's IPv6 name (path C), as
   today; or, as at home, the core's own origin at its IPv6 name.
3. **Away, IPv4 only, the punch works**: WebRTC (path F), paced.
4. **Away, nothing direct**: a relay of the failed connections, if the count calls for it (D274 §2).

This is Plex's model (central sign-in, per-IP names, direct first, relay last) with Jellyfin's
same-origin client at home, and WebRTC kept for what it is uniquely good at: reaching a CGNAT
household with no relay.

### 5.2 Steps, costs, and what is proven

1. **Send a browser at home to the core's LAN origin, with its session** — *proven in its essential
   part, the speed on Silk [L]; the rest suggested.*
   - When: the WebRTC connection is proven at home (D356 §1: a direct host pair from the house), or
     the discovery judges the device at home (`isAtHome`), and the browser cannot use the LAN name
     cross-origin (Firefox, or a permission not `granted`).
   - How: the page asks the core for a hand-off code over the path it has (D308 §4's
     `/session/handoff` takes a password today; it needs a variant that takes the signed-in session,
     the code bound to the device), and navigates to `https://<lan name>/#handoff=…`, where the core's
     own client redeems it. The first time, the page asks the person ("Use the faster home
     connection?") rather than navigating unasked, and remembers the answer per network
     (`clientPrefix`, protocol §3.3 item 7), because a name refused by rebinding protection shows
     the browser's error page, not ours.
   - The LAN origin's page, failing to reach its core (the laptop left the house), navigates back
     to the client origin, which its service worker can do offline.
   - Costs: a session per origin (a person signs in to both, once each, through the code); storage
     and settings kept per origin; Watch Together and cast links keep naming the client origin, which
     hands on again; a test per browser at home (a `VQ` item). Small to medium: the hand-off and the
     core's own client exist.
   - Gains: Silk 15–30 → 190–224 Mb/s **[L]**; Firefox off WebRTC at home; admin at home in every
     browser; the at-home path no longer depends on WebRTC staying outside Local Network Access.
2. **Ask whether the client origin is needed at all once a direct name answers** — *suggested.* The
   same hand-off to the core's IPv6 name away from home gives a same-origin page there too. Weigh
   against D308 §3's reason for client origins (a core's code kept apart from other households'):
   the core's own names already keep it apart, each core under its own `*.<id>` wildcard; the
   remaining cross-household risk is cookies on `orpheum-core.com`, a registrable domain shared by
   all cores (D291 §4 left it off the Public Suffix List), which D308 §7's disciplines (no client
   cookies, `__Host-` byte cookies) would have to cover there as well. Medium; the owner's.
3. **Pace the WebRTC path** — *measured with a stand-in, suggested as a build* (the throughput
   study's fix 1: about 3× at 20–50 ms). Whatever still crosses data channels (away, CGNAT) gets it.
   Medium.
4. **A relay for the connections that fail**, when D274 §3's count says it is wanted — *peer-proven,
   not measured here.* TURN, as voice already has (D243 §8), on the owner's VPS (self-hosting) or the
   service's (paid tier), encrypted end to end by DTLS with the core's signed fingerprint (D290 §6),
   capped as Plex caps its relay. Medium; it is a cost per byte for the service.
5. **Keep the IPv6 path open and visible** — *suggested.* Path C is the only one that is fast, direct
   and prompt-free both at home and away. Check that nas.example publishes an IPv6 name and that its
   router lets the port in (portmap's UPnP IGD 2 pinhole, or by hand); say on the System page when it
   does not. Small.
6. **Tell the person, and log it, when they are on the slow path at home** — *suggested.* Silk at home
   on path E today says nothing; a line with the "faster home connection" offer, and the per-connection
   SCTP figures on `rtcapi`'s end-of-connection line (the throughput study's fix 4), would have shown
   this on day one. Small.
7. **Then, only if the WebRTC path still carries much**: an hls.js loader for chunks on WebRTC in
   place of the service-worker relay, which also ends the preload's double fetch (efficiency audit
   finding 1). Medium.

### 5.3 What is not a fix

- **WebTransport**: a client-to-server QUIC transport built for this job (Chrome's own framing)
  **[V]**, but it needs the core reachable by UDP (it punches no NAT), it is gated by Local Network
  Access from Chrome 147 like `fetch` **[V/I]**, and Silk 138's support is unknown. It could one day
  replace the data channel where a direct UDP port exists; it does not reach a CGNAT household and
  does not beat HTTP at home.
- **"Use WebRTC to connect, then switch to an HTTPS socket"**: a browser cannot turn an ICE path into
  a TCP connection; the HTTP path needs a name and an address the browser can open, which is what
  step 1 uses.
- **Larger messages, more credit, more channels**: ruled out by the throughput study §3.

### 5.4 For the owner

1. **Is a top-level navigation to the core's LAN origin acceptable as the at-home path** (step 1),
   with a session per origin and the first switch asked of the person? It reverses protocol §3.1's
   *"not a path the client races"* and refines D308's "the client a browser runs is its core's, at its
   client origin".
2. **Should the client run at the core's own names away from home too** (step 2), leaving the client
   origin for first contact and for WebRTC?
3. **Does WebRTC stay the baseline (D274 §2)** or become the CGNAT fallback below every direct name,
   as D273 §7 first ranked it? In code the ranking already puts it last; the question is what
   "baseline" promises.

---

## Sources

Fetched or cloned 2026-10-28; pages in `/yoloai/cache/http/network-audit/`, served bundles in
`/yoloai/cache/http/network-audit/bundles/` (app.plex.tv's root page and main bundle, Emby's
`app.emby.media` root and Connect page, Synology's QuickConnect page: read for their design only,
nothing copied), repositories in `/yoloai/cache/git/`.

- Plex: [How Plex is doing HTTPS for all its users](https://words.filippo.io/how-plex-is-doing-https-for-all-its-users/);
  [Important note about the Plex Web app - Local Network Access](https://forums.plex.tv/t/933264) (2025-11-06);
  [python-plexapi](https://github.com/pkkid/python-plexapi) `plexapi/myplex.py:1459`; app.plex.tv's
  served main bundle **[B]**.
- Jellyfin: [jellyfin-web](https://github.com/jellyfin/jellyfin-web) `src/lib/jellyfin-apiclient/connectionManager.js:474–528`;
  [jellyfin-sdk-typescript](https://github.com/jellyfin/jellyfin-sdk-typescript) `discovery-service.ts`;
  [networking docs](https://jellyfin.org/docs/general/post-install/networking/).
- Emby: [Emby Connect](https://emby.media/connect.html);
  [Emby.ApiClient.Javascript](https://github.com/MediaBrowser/Emby.ApiClient.Javascript) `connectionmanager.js`;
  `app.emby.media` **[B]**.
- Home Assistant: [snitun](https://github.com/NabuCasa/snitun) `snitun/server/listener_sni.py`;
  [hass-nabucasa](https://github.com/NabuCasa/hass-nabucasa) `hass_nabucasa/remote.py`;
  [companion app URLs](https://companion.home-assistant.io/).
- Immich: [remote access](https://docs.immich.app/guides/remote-access); PhotoPrism:
  [proxies](https://docs.photoprism.app/getting-started/proxies/); Nextcloud Talk:
  [TURN](https://nextcloud-talk.readthedocs.io/en/latest/TURN/).
- P2P and data channels: [p2p-media-loader](https://github.com/Novage/p2p-media-loader) `FAQ.md`,
  `packages/p2p-media-loader-core/src/core.ts`; [simple-peer](https://github.com/feross/simple-peer)
  `index.js`; [webtorrent](https://github.com/webtorrent/webtorrent) `lib/torrent.js`;
  [FilePizza](https://github.com/kern/filepizza); [ShareDrop issue 154](https://github.com/szimek/sharedrop/issues/154);
  [Snowflake paper](https://www.bamsoftware.com/papers/snowflake/);
  [LiveKit data packets](https://docs.livekit.io/home/client/data/messages/);
  [Lennart Grahl, data channel size limits](https://lgrahl.de/articles/demystifying-webrtc-dc-size-limit.html);
  Mozilla bugs [1051685](https://bugzilla.mozilla.org/show_bug.cgi?id=1051685) and
  [976115](https://bugzilla.mozilla.org/show_bug.cgi?id=976115);
  [WebTransport](https://developer.chrome.com/docs/capabilities/web-apis/webtransport);
  [Tailscale SSH console](https://tailscale.com/blog/ssh-console).
- NAT traversal: [Tailscale, how NAT traversal works](https://tailscale.com/blog/how-nat-traversal-works);
  [tailscale](https://github.com/tailscale/tailscale) `wgengine/magicsock/endpoint.go:2065–2122`;
  [syncthing](https://github.com/syncthing/syncthing) `lib/config/optionsconfiguration.go:77–81`,
  `lib/discover/local.go:51`; [Syncthing relaying](https://docs.syncthing.net/users/relaying.html).
- Browsers: [Chrome, Local Network Access](https://developer.chrome.com/blog/local-network-access);
  [WICG Local Network Access explainer](https://github.com/WICG/local-network-access/blob/main/explainer.md);
  Mozilla bugs [1996495](https://bugzilla.mozilla.org/show_bug.cgi?id=1996495) and
  [2059274](https://bugzilla.mozilla.org/show_bug.cgi?id=2059274);
  [MDN, Mixed content](https://developer.mozilla.org/en-US/docs/Web/Security/Mixed_content);
  [Apple developer forums 811690](https://developer.apple.com/forums/thread/811690).
