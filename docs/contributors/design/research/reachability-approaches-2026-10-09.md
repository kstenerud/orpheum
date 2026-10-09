> **ABOUTME:** Every way a client can reach a household's core to talk, stream and voice-chat, laid
> out by layer with pros and cons, setting aside what was built so far: where the web client comes
> from, how the core gets a name browsers trust, how packets reach it, how clients find it, and how
> voice and text travel. Ends with the combinations that meet D273 and a recommended order.

# Reaching a core: the approaches, by layer

- **Date:** 2026-10-09.
- **Asked by** the owner (D273 §1): *"let's take a step back, not putting weight on what we've
  already chosen for this functionality, and figure out what approaches will help us reach this
  goal."* The goal: *"clients can talk to servers, stream video, and users can voice chat (and
  eventually text chat as well)"*; web users go to orpheum-app.com, log in, and land on
  app.orpheum-app.com *"talking to their core, with as much traffic as possible going direct"*; no
  relaying by us; a free single-target discovery of the owner's own; Landlock only warns.
- **Builds on:** [self-hosting-ways-2026-10-09.md](self-hosting-ways-2026-10-09.md) (how hosters
  expose servers, the survey numbers), [beta-inventory-2026-10-09.md](beta-inventory-2026-10-09.md)
  (what is decided and hardcoded), [direct-reachability.md](direct-reachability.md) and
  [remote-access.md](remote-access.md) (the earlier research), D214 and D273.
- **Legend:** **[V]** checked at its source this session (see Sources); **[I]** this author's
  inference; untagged is engineering common knowledge.

## 1. The short answer

The goal splits into five independent choices. Each has a best default and fallbacks; no single
mechanism covers every household, so the client tries several and keeps the first that works, as
WebRTC's ICE does.

| Layer | Recommended default | Fallbacks |
| --- | --- | --- |
| A. Where the web client comes from | app.orpheum-app.com serves it; it calls the core directly | the core serves its own client (the self-hosted path) |
| B. A name browsers trust for the core | a per-core name under a dedicated domain, its certificate got by DNS-01 through the service, the core holding its key | the owner's own domain; the proxy's certificate |
| C. How packets reach the core | direct HTTPS: LAN, IPv6, IPv4 with a port mapping (automatic, else manual) | a WebRTC data channel punched through NAT; the owner's own VPS |
| D. How a client finds the core | the service's list of the core's tested addresses, raced by the client | the owner's single-target discovery; LAN discovery for native apps |
| E. Voice and text | WebRTC between the people, the core as signalling and as TURN when it is reachable by UDP | the owner's VPS as TURN; voice-less with a plain explanation |

Two findings shape everything below:

- **The browser is the hard client.** It can only open HTTPS, WebSocket, WebTransport and WebRTC. A
  native app can do anything (raw UDP hole punching, its own certificate pinning), so a design that
  works for the browser works for native apps too, and native apps get extra fallbacks.
- **Video is the bulk.** Control, chat and even voice are small. Whatever carries video must be
  direct (no relay by us, D273 §2), so the methods that cannot carry video directly matter only as
  last resorts.

## 2. Layer A — where the web client comes from

The owner wants the address bar to say app.orpheum-app.com while the bytes flow to the core. Four
ways:

### A1. The core serves its own client at its own name (today)

- **How:** the browser opens `https://<the core's name>`; the page, the API and the media are one
  origin.
- **Pros:** simplest; cookies just work; the client always matches its core's version; works with
  no service at all, which D25's exit path needs.
- **Cons:** the user sees and remembers the core's name, not app.orpheum-app.com (against D273 §2);
  every core needs its own name and certificate anyway.
- **Keep for:** self-hosters, and as the exit path when the service is gone.

### A2. app.orpheum-app.com serves the client; the client calls the core cross-origin (the Plex model)

- **How:** the page, its JavaScript and its sign-in come from app.orpheum-app.com. After sign-in it
  asks the service where its core is, then sends every API call, chunk request and WebSocket straight
  to the core's own HTTPS name, which answers with CORS headers allowing app.orpheum-app.com. This is
  what app.plex.tv does with `*.plex.direct` **[V]**; Emby Connect likewise only finds servers and
  signs people in, "all connections are direct from your devices to your Emby Server" **[V]**.
- **Pros:** exactly the owner's picture: one address for everyone, traffic direct; the page is a
  secure context everywhere, so the microphone works even when the core is on the LAN (today's
  `http://devnas.example` cannot use it); the client updates once, centrally.
- **Cons:**
  - The core still needs a name and certificate browsers trust (layer B); it is only hidden.
  - **Cookies stop working.** The core's session cookie becomes a third-party cookie, which browsers
    block. Sign-in to the core becomes a token the page holds and sends in a header; media elements,
    which cannot send headers, use the short-lived media tokens Orpheum already puts in URLs.
  - **Version skew.** One central client talks to cores of many versions: the API needs versioning
    and the client must cope with older cores (or load the core's own client, A4).
  - **Local network access.** When the core is on the user's LAN, a public page calling a private
    address meets Chrome's Local Network Access permission prompt (Chrome 142, October 2025, for
    fetch and subresources; WebSocket and WebTransport from Chrome 147; WebRTC not yet) **[V]**, and
    Firefox's from 153 (July 2026) **[V, second-hand]**; the user allows it once per site. Only a
    secure page may ask, which app.orpheum-app.com is.
- **Fits:** the paid tier's web clients.

### A3. app.orpheum-app.com frames the core's own client

- **How:** a thin page at app.orpheum-app.com signs in, finds the core, and shows the core's own
  client (A1) in a full-window iframe.
- **Pros:** the client always matches its core; the address bar says app.orpheum-app.com.
- **Cons:** cookies inside the frame are third-party (partitioned cookies, CHIPS, make them work,
  but every browser's rules must be kept up with); fullscreen, the microphone and casting need
  explicit iframe permissions; back/forward and deep links are awkward. The Local Network Access
  prompt applies as in A2.

### A4. app.orpheum-app.com loads the core's own client code

- **How:** the thin page fetches the client bundle from the core (CORS) and runs it in the
  app.orpheum-app.com origin.
- **Pros:** no version skew, no iframe.
- **Cons:** the service's origin runs code a core served: a compromised core can attack the
  service's session and other things in that origin. Needs a strict separation (the service's own
  session never readable there). **[I]** Not recommended.

**Recommendation:** A2 for the paid tier's web client, with an API version the client checks; A1
kept for self-hosting and the exit path. Both are the same client code, built twice.

## 3. Layer B — a name browsers trust

A browser speaks HTTPS only to a name with a certificate from a public CA (WebTransport's certificate
hashes are the one exception, B5). Every layer-A choice needs one.

### B1. A per-core name under a dedicated service domain, issued through the service

- **How:** the service owns a domain of its own (not orpheum-app.com: see the spec's cookie finding)
  and gives each core a name under it. The core makes its own key and asks Let's Encrypt for the
  certificate; the service writes the DNS-01 TXT record on its behalf. Plex goes further and encodes
  each address in the name (`192-168-1-5.<hash>.plex.direct`, IPv6 as eight dashed groups), with a
  wildcard certificate per server from Let's Encrypt since 2021, so one certificate covers its LAN,
  IPv4 and IPv6 names **[V]**.
- **Pros:** works for every core whether or not the owner has a domain; the service never holds a
  core's private key; LAN names give HTTPS at home too.
- **Cons:**
  - Let's Encrypt allows 50 certificates per registered domain per week, and a name under a Public
    Suffix List entry counts as its own registered domain **[V]**. `plex.direct` is not on the list
    **[V]**, so Plex presumably has an exemption from Let's Encrypt **[I]**. Either the domain goes on
    the list or the service asks Let's Encrypt for a higher limit before it grows. Certificate
    lifetimes are shrinking (Let's Encrypt's default to 64 days in February 2027 and 45 in 2028)
    **[V]**: renewal is automatic and frequent.
  - Certificate Transparency logs publish every name **[V]**: anyone can list the cores (Plex has the
    same property).
  - **DNS rebinding protection** in many home routers (FRITZ!Box, OpenWrt/dnsmasq, pfSense) refuses
    public names that resolve to private addresses, which breaks LAN names **[V]**; the client then
    falls back to the public address, which may hairpin through the router or fail. Plex tells
    users to exempt the domain (dnsmasq's `rebind-domain-ok`, pfSense's `private-domain`, the
    FRITZ!Box's DNS-Rebind-Schutz list) or to use another DNS service **[V]**.
- **Fits:** the paid tier; also the owner's single-target discovery with the owner's own domain.

### B2. Certificates for IP addresses

- **How:** Let's Encrypt has issued certificates for IPv4 and IPv6 addresses since 15 January 2026,
  in its short-lived profile only (160 hours), by HTTP-01 or TLS-ALPN-01, not DNS-01 **[V]**.
- **Pros:** no domain at all.
- **Cons:** public addresses only (no LAN), the core must answer the challenge on port 80 or 443,
  renewals every few days, and an address change means a new certificate. **[I]** A fallback at most.

### B3. The owner's own domain (today's staging)

- **How:** the owner's domain, DNS-01 through their DNS provider, or HTTP-01/TLS-ALPN-01 on a
  forwarded 443.
- **Pros:** the owner's name, the owner's control; 88% of self-hosters have a domain
  (self-hosting-ways §1).
- **Cons:** needs a DNS provider integration per provider, or a forwarded port.
- **Fits:** self-hosting; native clients with a custom domain (D273 §4).

### B4. The hoster's reverse proxy holds the certificate

- **How:** Caddy, Traefik or nginx terminates TLS and passes plain HTTP to the core.
- **Pros:** what most self-hosters already run (67%, self-hosting-ways §1).
- **Cons:** the core must trust the proxy's forwarded client address, and only the proxy's; voice's
  UDP still needs its own way in (layer C).
- **Fits:** self-hosting.

### B5. WebTransport with certificate hashes

- **How:** the browser connects with `serverCertificateHashes`, pinning a short-lived self-signed
  certificate the service vouched for.
- **Pros:** no CA, no name.
- **Cons:** WebTransport only (no plain fetch, no media elements, no WebSocket); ECDSA P-256
  certificates of under 14 days; Chrome, Firefox and Safari 26.4 support the option, but Safari has
  been reported failing with some HTTP/3 servers **[V]**. **[I]** Not a base to build on today.

**Recommendation:** B1 for the service, B3 and B4 for self-hosters. All three are the same core
code with a different certificate source.

## 4. Layer C — how packets reach the core

| Method | Video direct? | Voice (UDP)? | Works behind CGNAT / DS-Lite? | Owner effort | Notes |
| --- | --- | --- | --- | --- | --- |
| C1. LAN | yes | yes | n/a | none | the client at home; B1's LAN name or the core's own name |
| C2. IPv6 | yes | yes | yes (DS-Lite gives native IPv6) | open the router's IPv6 firewall for the core | the client needs IPv6 too: most mobile networks have it, many hotels and offices do not |
| C3. IPv4 port mapping, automatic (UPnP IGD, NAT-PMP, PCP) | yes | yes | no: no German DS-Lite provider runs PCP on its carrier NAT **[V]** | none when the router allows it | the core asks the router; a 2019 scan found UPnP on in 76% of routers **[V]**; security-minded owners turn it off |
| C4. IPv4 port forwarding, manual | yes | yes | no | moderate: router settings | the classic; the core can test it from outside through the service's probe |
| C5. WebRTC data channel, hole-punched | yes, slower to build | yes | **often**: hole punching succeeded about 70% of the time in a 2026 measurement of 4.4 million attempts **[V]** | none | the service signals; browser and core punch a path; video goes over the data channel into the player; no CA certificate needed (DTLS with fingerprints exchanged by the service) |
| C6. The owner's own VPS (L3 tunnel, today's staging) | via the owner's VPS | yes, with address-preserving forwarding | yes | high: a VPS with IPv4, about €4.50–6.50 a month (Hetzner's smallest rose to €5.49 in June 2026) **[V]** | it relays, but through the owner's own machine: the free alternative of D273 §3 |
| C7. Third-party tunnels (Cloudflare Tunnel, Tailscale Funnel, ngrok) | Cloudflare's CDN terms let it disable service that is mostly video, which a Tunnel hostname goes through **[V, I]**; Funnel relays everything with unpublished bandwidth limits **[V]** | no UDP **[V]** | yes | low | not a method for Orpheum's video; Tailscale (not Funnel) works where every client runs Tailscale |
| C8. A relay run by us | — | — | yes | none | excluded by D273 §2 |

Notes on the ones that matter:

- **C3 automatic port mapping** is the cheapest win: zero setup where it works, and the core can
  tell from the service's outside probe whether it did. It does nothing behind CGNAT or DS-Lite's
  shared IPv4, where IPv6 (C2) or C5 must carry it.
- **C5 WebRTC data channels** are the one direct path for households behind CGNAT whose visitors
  have no IPv6 (the owner's household on IPv4-only networks). Browsers already do WebRTC for voice;
  the core would hold a WebRTC peer (pion is already a dependency for TURN) and serve chunks over
  it, with the player fed through a custom loader (p2p-media-loader plugs into hls.js and plays
  segments fetched over data channels **[V]**). Costs: a second transport for every request the client makes, connection set-up time
  (a second or two of ICE before the first chunk), and failure when both ends are behind symmetric
  NAT or carrier CGNAT that maps per destination. **[I]** Worth building after the direct paths, as
  the paid tier's answer to D214 §2's gap; native apps can do the same over raw UDP.
- **C6 the owner's VPS** stays as the self-hosted answer for CGNAT, and is what D273 §3's
  single-target discovery runs on anyway (one VPS: discovery + tunnel + TURN).

**Recommendation:** C1, C2, C3 and C4 first (all plain HTTPS, all the same core code: listen on
443 or behind a proxy, report the addresses); C6 for self-hosters behind CGNAT; C5 as the paid
tier's CGNAT answer after the beta.

## 5. Layer D — how a client finds the core

### D1. The service knows the addresses; the client races them

- **How:** the core reports its candidate addresses (LAN, IPv6, mapped IPv4, its VPS if any) to the
  service every few minutes; the service tests which answer from outside and hands them, with their
  B1 names, to a signed-in client; the client tries them at once and keeps the first to answer, the
  LAN one preferred (ICE's idea, for HTTPS).
- **Pros:** fast, survives address changes, picks LAN at home without the user knowing.
- **Cons:** the service must be up for a first connection (a client can remember the last good
  addresses to ride out an outage).

### D2. The owner's single-target discovery (D273 §3)

- **How:** one small program on the owner's VPS, at the owner's domain: it serves the same web
  client as app.orpheum-app.com (A2), knows exactly one core, and answers D1's questions for it. On
  the same VPS it can hold the WireGuard tunnel and TURN (C6, E2) for a core behind CGNAT.
- **Pros:** free apart from the VPS; one binary (`orpheum discovery`, say); nothing competes with
  app.orpheum-app.com because it serves one core by design.
- **Cons:** a VPS costs more than the service (D273 §3's argument); the owner keeps it updated.

### D3. LAN discovery for native apps

- mDNS/DNS-SD or SSDP on the LAN, so a native app at home finds the core with no service. Browsers
  cannot do it. Later, with the native apps.

## 6. Layer E — voice and text

- **E1. WebRTC between the people, the core as signalling.** Voice goes peer to peer whenever two
  browsers can reach each other; the core (reached by layer C) passes the offers. Small groups (D246)
  suit a mesh. Pros: no media through anyone's machine. Cons: needs a fallback when peers cannot
  reach each other.
- **E2. TURN on the core** (today, D243 §8): the fallback relay is the household's own machine, so no
  relay by us. It needs UDP to reach the core: works with C1–C4 and C6, not with C5 alone (a data
  channel is not TURN). **[I]** With C5, peers can still try direct ICE between themselves; when that
  fails, voice can go over the data channel to the core as an SFU — a bigger build.
- **E3. TURN over TLS on 443** would let voice through networks that block UDP, at a latency cost.
  Later.
- **Text chat** rides on whatever carries the API (the core's WebSocket): no extra transport.

## 7. Combinations that meet D273

| Combination | Who it serves | Our traffic | Build size |
| --- | --- | --- | --- |
| **P1. Service, direct HTTPS:** A2 + B1 + C1–C4 + D1 + E1/E2 | households with IPv6 visitors, a forwardable IPv4, or UPnP | discovery only | medium: CORS and token sign-in, identity key, address reporting, service-issued certificates, UPnP/PCP |
| **P2. P1 + WebRTC data channels (C5)** | adds CGNAT households whose visitors lack IPv6 | discovery and signalling only | large: a second transport for chunks, the player's loader |
| **S1. Self-hosted, own domain:** A1 + B3/B4 + C1–C4 | self-hosters with a proxy or a port | none | small: proxy mode, trusted proxies, TLS-ALPN-01/cert files, no tunnel required |
| **S2. Self-hosted single-target discovery:** A2 + B3 + D2 + C6 + E2 on one VPS | owners behind CGNAT who will not pay | none | medium: the `discovery` mode, the VPS setup scripted |

**Recommended order [I]:**

1. **For the closed beta:** S1 (it is small and most testers already run a proxy) and the core work
   P1 needs anyway: the identity key, token sign-in beside cookies, CORS for the service's origin,
   address reporting, and UPnP/PCP mapping with an outside test. The service's beta (the spec) adds
   B1 names and D1.
2. **Then:** S2, so CGNAT households have a free route; and P2's data channels, so they have a paid
   one.
3. **Later:** TURN over TLS, LAN discovery and custom domains in the native apps.

## 8. Open questions for the owner

1. **Is the Local Network Access prompt acceptable** for users at home (A2 talking to a LAN core)?
   The alternative is sending home users to the core's own name (A1) when on the LAN.
2. **May the central client run against older cores** (A2 with a versioned API), or should a core
   older than the client be told to update first?
3. **Is a dedicated domain for core names acceptable** (B1), on the Public Suffix List, given that
   Certificate Transparency makes the names public?
4. **Should the core map its own port by UPnP/PCP by default** (C3), or only when the owner turns it
   on?
5. **Is P2 (WebRTC data channels for video) worth its size** as the paid tier's answer to CGNAT, or
   is "use IPv6, a port-forward or your own VPS" enough for now?

## Sources

Checked on 2026-10-09; pages cached in `/yoloai/cache/http/reachability-facts/`.

- Plex: [How Plex is doing HTTPS for all its users](https://words.filippo.io/how-plex-is-doing-https-for-all-its-users/) (2015);
  [TLS certificate transition](https://forums.plex.tv/t/tls-certificate-transition/689056) (2021, to Let's Encrypt);
  [How to use secure server connections](https://support.plex.tv/articles/206225077-how-to-use-secure-server-connections/);
  [IPv6 with plex.direct](https://cetteup.com/200/how-to-set-up-fully-dynamic-ipv6-remote-access-with-plex/);
  [FRITZ!Box rebind exemption](https://www.it-halbwissen.de/artikel/plex-dns-rebinding-fritzbox-nextdns/).
- Emby: [Emby Connect](https://emby.media/connect.html).
- Chrome: [Local Network Access](https://developer.chrome.com/blog/local-network-access) (Chrome 142);
  [WebSocket and WebTransport in 147](https://github.com/getsentry/browser-updates-radar/issues/29);
  [WebRTC intent to prototype](https://groups.google.com/a/chromium.org/g/blink-dev/c/CDy8LAs-DoA/m/H5pNeS3WAAAJ).
- Let's Encrypt: [IP certificates generally available](https://letsencrypt.org/2026/01/15/6day-and-ip-general-availability);
  [first IP certificate](https://letsencrypt.org/2025/07/01/issuing-our-first-ip-address-certificate);
  [rate limits](https://letsencrypt.org/docs/rate-limits/); [from 90 to 45 days](https://letsencrypt.org/2025/12/02/from-90-to-45).
- WebRTC and NAT: [hole punching at scale](https://arxiv.org/abs/2604.12484) (IMC 2026);
  [5G carrier NAT](https://arxiv.org/abs/2311.04658); [p2p-media-loader](https://github.com/Novage/p2p-media-loader).
- WebTransport: [MDN, serverCertificateHashes](https://developer.mozilla.org/en-US/docs/Web/API/WebTransport/WebTransport);
  [Safari interop report](https://github.com/hyperium/h3/issues/347).
- Port mapping: [Trend Micro, UPnP-enabled devices](https://www.trendmicro.com/en_us/research/19/c/upnp-enabled-connected-devices-in-home-unpatched-known-vulnerabilities.html) (2019);
  [PCP and DS-Lite](https://www.vodafonekabelforum.de/viewtopic.php?t=36462).
- Tunnels: [Tailscale Funnel](https://tailscale.com/docs/features/tailscale-funnel);
  [Cloudflare service-specific terms](https://www.cloudflare.com/service-specific-terms-application-services/).
- VPS prices: [Hetzner price adjustment](https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/).
