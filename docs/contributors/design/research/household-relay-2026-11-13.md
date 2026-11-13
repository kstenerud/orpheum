> **ABOUTME:** Research and a proposed design for the household relay D438 defers: a relay the
> household runs on a small VPS or an arm64 box, paired with its one core, that clients reach over
> TCP 443 when nothing direct works. It works with the paid discovery and the household's own.
> Read it before building the relay or reopening D438. It decides nothing.

# The Household Relay: Research and a Proposed Design

- **Date:** 2026-11-13. The owner asked for *"research into a relay service that they host but
  can also work in concert with the paid discovery service"*.
- **Scope:** reading, the repository and arithmetic. No code changed. Fetched pages are noted with
  their URLs in `/yoloai/cache/http/household-relay-2026-11-13/notes.md`.
- **Bounds:** [D438](../../decisions/working-notes.md#d438--a-households-own-relay-stays-deferred-until-the-betas-data-asks-for-it-when-built-one-core-pairs-with-one-relay-enrolled-by-the-core)
  settles the shape:
  - one core to one relay;
  - the core enrols the relay by a code and dials out to it;
  - the core tells its discovery the relay's address;
  - the relay terminates nothing;
  - relayed playback is capped;
  - building waits for the beta's data.

  [D448](../../decisions/working-notes.md#d448--a-server-installs-and-finds-its-parts-with-as-little-skill-as-possible-the-deployment-audits-proposals-adopted)
  §10 adds that the relay works with the owner's own discovery as with the paid one. Everything
  below that goes further is a proposal, and §8 lists each one for the owner.
- **Marks:**
  - **[source]** a page read today, with a quote;
  - **[repo]** this repository or `orpheum-connect`, with its path;
  - **[unverified]** seen only in a search summary, a forum or an aggregator;
  - **[est]** an estimate, with its arithmetic.

## Summary

- **The transport: TURN, served by `orpheum relay`, a mode of the one binary**
  ([D71](../../decisions/working-notes.md#d71--one-relay-artefact-three-jobs-and-the-core-dials-out-to-it)).
  - It is built on the pion/turn the core already embeds.
  - It listens on UDP 3478 and on TCP 443. On 443, one demultiplexer separates plain TURN, TURN over
    TLS and the core's control connection.
  - The client already handles every piece: its own ICE, `turns:` URLs, the heartbeat's `turn`
    field and the `iceServers` the paths answer carries.
  - So the loader, the client, the Cast receiver and Fire TV's Silk all reach a core behind carrier
    NAT over TCP 443 without a new client transport.
  - The relay sees DTLS, never content.
  - An HTTPS passthrough (D71's tunnel mode) is the possible second step, if relayed throughput ever
    matters. Under a cap of a few Mb/s it does not.
- **A closed relay, two credential roles.**
  - The core's own credentials, never handed out, may relay to the public internet.
  - Clients' credentials last 15 minutes, as today's do, and reach only the core's own allocations
    on the same box.
  - So a leaked client credential reaches the core and nothing else. That meets D438 §3's "no open
    relay" by construction, not by a deny-list.
- **Pairing and dial-out.**
  - The relay prints a 12-character code. The admin enters it with the relay's address on the
    System page, from home.
  - Both sides prove the code over a TLS connection, and each pins the other's key.
  - The core keeps one control connection open to the relay on 443, with the keepalive the
    signalling socket uses.
  - It advertises the relay only while that connection is up.
- **The cap.** The core enforces it: a relayed connection is one more ceiling in upload control,
  and the viewer is told of it, as D72 requires. The relay polices a looser limit as a backstop.
  At the relay report's 2 Mb/s per stream and 4 Mb/s per household, the worst month is
  1,296 GB [repo].
- **The discoveries.** Both already pass the heartbeat's `turn` entries into `iceServers`
  (`orpheum-connect/internal/clients/clients.go`, `discovery/single/client.go`) [repo]. Neither
  needs a change to hand the relay out.
- **The train (2026-11-13) shows a gap the relay alone does not close.**
  - The punched UDP pair passed ICE's small checks and then starved.
  - ICE would still pick it over a relay.
  - The client needs a "starved, retry relay-only over TCP" step (§4.7), and the beta needs a
    count of starved connections, which D314's dead-end count cannot see (§6).
- **Build:** about nine steps, roughly four to six weeks of work, the measurements first (§5).
- **Questions for the owner:** §8.

## 1. What D438 settles and what is open

- **Settled:**
  - one core and one relay, with no accounts and no abuse handling of the relay's own (§2);
  - pairing by a code the relay shows (§3);
  - the core dials out (§3);
  - the core alone tells the discovery, signed under its session (§3);
  - DTLS and TLS pass through untouched (§3);
  - a cap that keeps a household's worst month near 1.3 TB (§4).
- **Open, and answered below as proposals:**
  - the transport;
  - the pairing's protocol;
  - the control connection;
  - the credentials;
  - the cap's mechanism;
  - the certificate;
  - the package;
  - the client's fallback order;
  - the measurements that reopen D438.
- **Nothing to reuse for the pairing.** Nodes do not pair by a code today: they are enrolled by
  name on a local socket ([repo] `internal/core/nodes/doc.go`). D448 §11 keeps them on the LAN,
  "and joining one by a code" is not planned. So the relay's pairing is the first pairing by code
  over the network.
  - The nearest mechanisms are the setup code (12 safe32 characters, `internal/core/auth`) and
    the device codes
    ([D288](../../decisions/working-notes.md#d288--a-device-is-activated-by-the-code-it-shows-entered-on-a-device-already-signed-in-and-a-check-code-both-show),
    `discovery/safe32code`) [repo].

## 2. What the evidence says

### 2.1 TURN over TCP and TLS, and on 443

- **Between client and server, TURN runs over any transport.** RFC 8656 §3.1: the client may reach
  the server over UDP, TCP, TLS over TCP, or DTLS. The relayed leg to the peer is UDP alone:
  *"This specification describes only UDP allocations"* [source: RFC 8656].
  - The reason for TCP is stated: *"some firewalls are configured to block UDP entirely"*
    [source].
  - The default ports are 3478, and 5349 for TLS. The RFC says nothing of 443 [source].
- **RFC 6062's TCP allocations are not needed, and pion/turn has none** [repo: the module at
  `pion/turn/v4@v4.1.4`]. A browser opens TCP or TLS to the TURN server, and the relayed leg to
  the core stays UDP.
  - That leg is the relay's own network, not the client's, so UDP being blocked at the client
    does not matter.
- **Permissions are per IP address, not per port:** *"The IP address and transport protocol (but
  not the port)"*. A permission lasts 5 minutes and an allocation 10, unless refreshed
  [source: RFC 8656 §2, §3.2, §3.3]. A permission check alone cannot pin a client to one port of
  the core; §4.5 adds a check on the relayed socket.
- **443 is what every network allows.** The repository's own survey:
  - TCP 443 is the only column every network in its reachability table passes;
  - hotels are to be treated as "TCP 443 only, UDP unreliable";
  - 5% of QUIC connections had UDP impaired (SIGCOMM 2017);
  - Zoom and Teams keep a TCP 443 fallback.

  [repo: [direct-reachability.md](direct-reachability.md) §2.3–3]. Tailscale makes the same bet:
  *"Any device that can open an HTTPS connection to an arbitrary host can build a tunnel using
  DERP relays"* [source: Tailscale, connection types].
- **No relay helps against an allowlist** [repo: direct-reachability.md]. A network that passes
  only named sites stays a dead end, as D314 §4 counts it.

### 2.2 What each client can use

| Client | Engine | TURN over TCP | `turns:` | Evidence |
| --- | --- | --- | --- | --- |
| Chrome, Edge (desktop, Android) | Chromium's libwebrtc | yes | yes | **Certificates:** a WebRTC commit says a URL holding only an IP needs a host name for SNI and for verification, and LiveKit's TURN test failed on an IP-address certificate [unverified: search summaries]. So use a name. |
| Firefox | its own ICE stack | yes; active ICE-TCP by default since 54 | yes, since 53 | **Proxies:** bug 1922559 made `turns:` really use TLS (fixed in 133), and bug 2011209 (TCP through a configured proxy, broken from 128) is open [unverified: search summaries of bugzilla]. |
| Safari, iOS and macOS | WebKit's libwebrtc | yes | yes | **Certificates:** WebKit bug 219274, `turns:` failing on Let's Encrypt certificates because of a hard-coded root list, was fixed on 2020-12-10. An iOS 14.6 report after it was never confirmed [source]. **URLs:** bug 320931 had Technology Preview 249 refusing TURN URLs with a query string (`?transport=tcp`); it was fixed on 2026-08-18 (319365@main). Whether any shipping Safari carried it is [unverified]. |
| Fire TV (Silk 138) | Chromium | as Chrome | as Chrome | [repo: the owner's Fire TV runs Silk 138, [idiosyncrasies.md](../../idiosyncrasies.md)]. TURN itself is [unverified] on the device. |
| Cast receiver | Chromium on the device | as Chrome, [unverified] | [unverified] | **WebRTC:** the receiver connects over WebRTC through the discovery [repo: `web/apps/cast-receiver/src/reach.ts`]. |

- **The relay offers TURN-TCP and TURNS both, and UDP.** The URLs:
  - `turns:<name>:443?transport=tcp`;
  - `turn:<address>:443?transport=tcp`;
  - `turn:<address>:3478?transport=udp`.

  Each client takes what works. Safari's history argues for keeping a query-free `turns:<name>:443`
  beside the others.
- **Every row needs a device check** before the relay is called working: §5's VQ items.

### 2.3 coturn and pion/turn on a small machine

- **coturn** is the usual server.
  - It does TURN REST natively: `use-auth-secret`, `static-auth-secret` "for TURN REST API only".
  - It has a per-session `max-bps` and a total `bps-capacity`, quotas, and `denied-peer-ip` /
    `allowed-peer-ip` ("If an ip address is specified as both allowed and denied, then the ip
    address is considered to be allowed").
  - It has `no-tcp-relay`, and TLS on 5349 by default [source: coturn's example
    `turnserver.conf`].
  - It has no notion of "this allocation may only reach that one", nor a pairing.
- **coturn's peer ACLs have been bypassed repeatedly:**
  - CVE-2020-26262, an IPv4-mapped loopback, fixed in 4.5.2 [source: Enable Security's advisory,
    listed in search];
  - CVE-2026-73212, IPv4 `denied-peer-ip` ranges bypassed on the RFC 6062 TCP path, fixed in
    4.13.1 [unverified: aggregators];
  - CVE-2026-73213, the IPv6 comparison [unverified: aggregators].

  A design whose safety rests on a deny-list inherits that history. §4.5's allow-only rule does
  not.
- **pion/turn** is what the core already runs for Watch Together's voice and for clients the
  discovery sends [repo: `internal/core/turn/doc.go`]. Its v4.1.4 server gives every hook the
  design needs [repo: module cache]:
  - an `AuthHandler` keyed by username and source;
  - a `PermissionHandler(clientAddr, peerIP)`;
  - a `QuotaHandler`;
  - an `EventHandler` carrying `OnAllocationCreated` and `OnAllocationDeleted` with the username;
  - `ListenerConfigs` taking any `net.Listener`, so a TLS listener or a demultiplexed one is
    accepted;
  - a `RelayAddressGenerator` that can wrap each relayed socket, where a rate limit and a
    destination check go.
- **On a CX23 or a CAX11.** A Go TURN server relaying a few Mb/s for one household is far below
  what 2 vCPUs carry [est: the cap is 4 Mb/s, and a TLS-wrapped UDP relay at that rate is a few
  hundred packets a second]. No benchmark was run (Not tried).
- **The machines.** Hetzner's cost-optimised plans, read today [source]:
  - CX23, x86: 2 vCPU, 4 GB, 20 TB, €5.99 a month;
  - CAX11, arm64: 2 vCPU, 4 GB, 20 TB, €6.49 a month.

  Both showed "currently unavailable" today. The brief's "€4 VPS" no longer exists at Hetzner.
  The release image is already built for amd64 and arm64 [repo:
  `.github/workflows/release-image.yml`].

### 2.4 Locking TURN to one peer

- **TURN REST credentials** (draft-uberti-behave-turn-rest-00, July 2013, expired January 2014,
  "no formal standing") [source]:
  - the username is "a colon-delimited combination of the expiration timestamp and the username
    parameter";
  - the password is `base64(hmac(secret key, returned username))`;
  - the draft recommends a day's life;
  - rotation of the secret is RECOMMENDED, with two secrets accepted during rollover;
  - credentials cannot be revoked: *"waiting for the timeout to expire is a valid approach"*.
- **The core already does this.** Usernames are `<expiry>:found`, lasting 15 minutes, and an
  allocation made while they were good survives their expiry [repo:
  `internal/core/turn/relay.go`, `grantedAllocations`;
  [discovery-protocol.md](../discovery-protocol.md) §2.4].
- **A credential does not lock a relay to a peer; the permission rule does.** Enable Security:
  *"TURN credentials are often handed out to browsers freely, so treat every authenticated user
  as a potential attacker"*, and if peer-to-peer is not needed, allow-list the media servers'
  addresses [source]. Our clients' credentials reach the discovery's `/paths` answer, so they are
  handed out freely in exactly this sense.

### 2.5 Byte forwarding instead of TURN

- **SNI or ALPN passthrough to the core's dial-out** (D71's tunnel mode).
  - The relay reads the TLS ClientHello's name, splices the stream into a multiplexed connection
    the core holds open, and the core terminates TLS at home.
  - The paid zone already makes this cheap. Names encode addresses, `<address>.<coreId>.<zone>`,
    under one wildcard certificate per core, and a `configured` candidate is "an admin's
    port-forward or VPS" [repo: discovery-protocol.md §2.4, §2.7].
  - So the core could publish the relay's address as a `configured` candidate, and the client
    would get an `https://` path to it carrying the core's own certificate.
  - **What it gains:** plain HTTP/2 over one TCP connection, and no SCTP over TCP.
  - **What it needs:**
    - a path kind ranked after WebRTC, because an `ipv4` path through a relay would otherwise beat
      a punched direct pair (§3.3's ranking puts IPv4 above WebRTC);
    - a multiplexer on the core's dial-out;
    - the core serving TLS on streams it did not accept from a socket;
    - care that a tunnelled stream is never taken as from home (D297 §6's admin rule reads the
      source address, and a tunnelled stream's source is the relay's connection).
  - It is impossible where the core has no names: an own discovery without a domain.
- **WebSocket or WebTransport to the relay, piped to the core.** A browser cannot carry our
  existing transport over either without a new client transport. WebSocket would have the relay
  terminate TLS, unless the client ran its own inner encryption in JavaScript. WebTransport needs
  `serverCertificateHashes` or a name, and is *"not a base to build on today"* [repo:
  [reachability-approaches-2026-10-09.md](reachability-approaches-2026-10-09.md) B5].
  Rejected.
- **The WireGuard VPS the deployment already documents.** `deploy/vps/` forwards TCP 443, TURN
  and the relay ports into a WireGuard tunnel the core's machine dials, *"which works on DS-Lite
  and carries UDP"*, and keeps the client's address [repo: `deploy/vps/README.md`, D243 §7].
  - It is a working household relay for someone who can run root, nftables and WireGuard.
  - It has no pairing, no cap and no lock to one core beyond WireGuard's key.
  - It needs UDP from the house to the VPS.
  - It stays the documented route by hand. It is not the package.
- **On middleboxes:**
  - **Port-based firewalls:** pass TURN-TCP on 443, TURNS and passthrough HTTPS alike.
  - **Protocol-checking firewalls:** pass TLS and refuse plain STUN on 443, so TURNS and passthrough
    pass and plain TURN-TCP fails.
  - **TLS interception on a managed device:** breaks neither if the device trusts the inspector's
    root. The DTLS inside TURNS, and the manifest check, stay end to end.
  - **Explicit HTTP proxies:** favour passthrough, which is ordinary HTTPS through CONNECT. TURN
    through a proxy is browser-specific: Firefox's bug 2011209 is open [unverified].
  - **Captive portals:** block everything until signed in, then behave as their network does.
  - So TURNS on 443 covers every case passthrough does except the explicit proxy, which is rare on
    the networks a household meets (hotels, trains, mobile).

### 2.6 How others relay, and their caps

| System | How | What the relay sees | Cap |
| --- | --- | --- | --- |
| Tailscale DERP | **Transport:** an HTTP upgrade over HTTPS on 443. Packets are routed *"to clients using curve25519 keys as addresses"*, framed as a 1-byte type and a 4-byte length, with nothing over 64 KB. **Keepalive:** at least every 60 s, with jitter [source: `tailscale.com/derp` v1.104.7]. Every connection starts on DERP and upgrades [source: connection types]. | WireGuard ciphertext | a token bucket the server advertises (`TokenBucketBytesPerSecond`, zero meaning unspecified); *"limited quality of service"*, *"may offer lower maximum throughput"* [source] |
| Syncthing relay | **Protocol mode:** a device keeps a TLS connection open (`JoinRelayRequest`). **Session mode:** a peer asks with `ConnectRequest`, both get a `SessionInvitation` with a key and open session connections, and the relay *"for every byte written by one device, sends the same set of bytes to the other device"*. The first byte (0x16 for TLS) picks the mode [source: relay protocol v1]. | the devices' inner TLS: *"completely meaningless to the relay"* [source] | `-global-rate` and `-per-session-rate`, in bytes a second. `-token` makes it private and *"Disables joining any pools"*. Port 22067, with 443 recommended [source: strelaysrv] |
| Plex Relay | **Access:** a fallback when the app *"isn't able to make a direct connection"*, on by default [source]. | *"encrypted end-to-end and tunneled through our Relay"* with secure connections [source] | *"limited to 2 Mbps maximum for streams (this applies to both Plex Pass and Remote Watch Pass subscribers)"* [source]. D72's "1 Mbps free" is older. |

- **Every one runs over 443, keeps a long-lived dial-out from the side that cannot be reached,
  carries ciphertext it cannot open, and caps rate.**
  - Syncthing's design is closest to D438. It has a permanent connection from the device, a
    private token, per-session and global rates, and the same first-byte split this design uses on
    443.
  - Plex's 2 Mb/s is the figure D438 §4 points at.

### 2.7 TURN abuse

- TURN servers *"are also powerful proxies that hackers have been abusing since at least 2017"*.
  Relay abuse is like SSRF: peers *"could be anything"*, internal ranges and cloud metadata among
  them, and the Slack case reached the metadata service and took IAM credentials [source: Enable
  Security].
- A TURN server can also be used to reflect and amplify floods [source].
- The core's TURN already relays only to the public internet [repo: `mayRelayTo`, enforced by
  `TestRelay_ServesTCPAndRelaysOnlyToThePublicInternet`]. Hetzner's terms forbid scanning
  [repo: [relay-costs-2026-11-06.md](relay-costs-2026-11-06.md) §5].

### 2.8 What the train shows

- **What happened** [repo: memory of the 2026-11-13 batch;
  [navigation-and-widgets-answers-2026-10-27.md](../plans/navigation-and-widgets-answers-2026-10-27.md),
  "The core's data link goes quiet on a lossy network"]:
  - On a train's Wi-Fi, from 17:30 every connection to the production core went quiet.
  - The connections were on a punched IPv4 pair.
  - Every SCTP congestion window collapsed to 1,191 bytes, including a device that had moved 3 MB
    just before.
  - Small packets passed; nearly every full-size one was lost.
  - YouTube still played, so TCP, or QUIC on UDP 443, worked where our UDP to high ports did not.
  - The SCTP fix (VQ316) keeps a starved path from going silent. It cannot make it carry video.
- **What it means for the relay:**
  - **The case exists for the owner's own household.** The production core is reached over
    WebRTC, not a direct name.
  - **ICE would not have chosen the relay.** Connectivity checks are small packets, and they
    passed on the punched pair, which outranks any relayed pair. A relay helps a network that
    blocks UDP outright at once. It helps one that starves UDP only if the client notices the
    starvation and retries on the relay alone (§4.7).
  - **D314's dead-end count never saw it.** The connection connected, and only its throughput
    died.

## 3. The transport, compared

| | **A. TURN on the relay (recommended)** | **B. HTTPS passthrough (D71 tunnel)** | **C. The WireGuard VPS (`deploy/vps`)** |
| --- | --- | --- | --- |
| Client change | none for UDP-blocked networks; a starved-path retry (§4.7) | a new path kind and its ranking | none |
| Discovery change | none, both pass `turn` through [repo] | a `configured` candidate the probe passes; an own discovery needs names | names, as for any port forward |
| Core change | a TURN client allocation on the relay; control connection; cap | stream multiplexer; TLS on tunnel streams; home-ness; cap | none; the tunnel is the machine's |
| Needs the core's names | no; a name for the relay helps `turns:` | yes | yes |
| Works with no UDP at the client | yes, over TCP or TLS 443 | yes | yes, for HTTPS |
| Works with no UDP at the house | yes, TURN-TCP from the core | yes | no: WireGuard is UDP |
| Efficiency | SCTP over DTLS over TURN over TCP: two reliable layers, so loss shows as delay | one TCP, HTTP/2 | native |
| Relay sees | addresses, sizes, timing, DTLS handshakes | addresses, SNI, sizes, timing | everything a router sees |
| Lock to one core | credential roles and an allow-only peer rule (§4.5) | it forwards only to the core's dial-out | WireGuard's key |
| Install | one container, a code | one container, a code, names | root, nftables, WireGuard by hand |
| Size | M–L (§5) | L, plus most of A's pairing and control | done; packaging is M |

- **Recommendation: A, then B only if the beta shows relayed playback hurting.**
  - Under a 2–4 Mb/s cap, B's efficiency buys nothing a viewer would see.
  - A reaches a core with no names: an own discovery on a bare IP, or an own discovery behind
    carrier NAT.
  - A reuses everything the client already does.
  - B shares A's pairing, control connection and cap, so nothing in A is wasted if B follows.

## 4. The proposed design

### 4.1 Shape

- **`orpheum relay`**, a mode of `cmd/orpheum` beside core, node and discovery. It ships in the
  core's public image, run with a different command (D71;
  [D235](../../decisions/working-notes.md#d235--development-and-deployment-run-in-containers-one-image-family-one-command-each-built-by-goreleaser)).
- **What it holds:**
  - its own Ed25519 key;
  - after pairing, the core's public key and the TURN secret the core gave it;
  - a counter of bytes this month.

  It holds no discovery account or key (D438 §3), and no user, film or session data.
- **Its packages:** a new `internal/relay` for the mode, its listeners and its control side. It
  reuses `internal/core/turn`'s username parsing and `mayRelayTo`, moved to a package both may
  import (rule 11: one way to do each job).

### 4.2 Ports and the 443 demultiplexer

- **UDP 3478:** TURN over UDP, for clients and for the core when UDP works.
- **UDP 443:** the same TURN. A network that passes QUIC's port and not others (the train, perhaps)
  may pass it [est: YouTube played there]. It costs one listener.
- **TCP 443**, split on the first byte as Syncthing's relay does:
  - STUN's first byte (`0x00` or `0x01`) goes to plain TURN-TCP;
  - `0x16` goes to TLS, and within TLS the ClientHello's ALPN decides:
    - `orpheum-relay/1` is the core's control connection (§4.4);
    - `h2` or `http/1.1` is the co-located own discovery's HTTPS, if one shares the machine (§7);
    - anything else, `stun.turn` (RFC 7443) or none, is TURN over TLS.

    The rule does not depend on whether browsers send `stun.turn`, which the self-hosted
    discovery research marks unverified.
- **Relay ports:** a fixed UDP range, as the core's 49160–49199 (D243 §8). These are reached only
  by the relay itself and by the core's own traffic (§4.5), so the firewall may close them to
  everyone else.

### 4.3 Pairing and keys (D438 §3)

1. **The relay, first started,** makes its key and prints, to its log and to `orpheum relay code`:
   - its address;
   - a 12-character safe32 code (60 bits, the setup code's shape), good for an hour or until
     used.
2. **The admin, on the System page and from home** (D297 §6), enters the relay's address and the
   code.
3. **The core dials the relay's TCP 443** with ALPN `orpheum-relay/1`. Each side presents a
   self-signed certificate on its key, and neither knows the other's yet. Both compute an HMAC
   under the code over:
   - a label;
   - both public keys;
   - the TLS exporter of this connection.

   Each checks the other's. A mismatch ends the attempt and counts against the code; after five,
   the code is void and the relay prints a new one.
4. **On success:**
   - each pins the other's key;
   - the core makes a TURN secret for this relay (256 random bits, sealed with the core's
     secrets) and sends it;
   - the relay seals it at rest.
5. **The relay refuses** every later pairing and every control connection from any other key,
   until its owner resets it on its own machine (`orpheum relay unpair`).

- **Why no PAKE.** Someone in the path of the core's first dial can take the side that answers
  second, and try codes offline against the first side's HMAC.
  - At 60 bits and about 10^10 tries a second, that is about 3 years against a code that lives
    an hour [est].
  - A PAKE (SPAKE2, CPace) would remove even that, but none is in `deps/go.allow`, and a
    dependency is the owner's decision (rule 3).
  - An 8-character device code (40 bits) would not be enough here.
- **A dedicated key on the core for the relay, not the identity key** that signs the manifest and
  fingerprints. One key, one use. A taken relay then learns nothing it could replay against the
  identity.

### 4.4 The core's dial-out and keepalive

- **One control connection:**
  - TLS 1.3 to the relay's 443, both keys pinned;
  - a WebSocket on it (`coder/websocket` is allowed) at `/control/v1`;
  - a version in its hello, changed additively (D273 §6).
- **Keepalive, as the signalling socket's:** the core pings every 27.5 s ± 9%, and two missed
  pongs end the connection [repo: discovery-protocol.md §2.5, `keepAlive`]. It reconnects with
  jittered backoff, from 1 s to 60 s. DERP's 60 s with jitter, and Syncthing's 1 min ping, are the
  same idea [source].
- **What it carries:**
  - **From the relay:** its version, listeners, allocations, load and bytes this month.
  - **From the core:** secret rotation (two secrets accepted across a rollover, as the TURN REST
    draft advises), the cap's numbers, a drain before an unpair, and an unpair.
  - Everything from the relay is read with the strict decoder and bounded. A taken relay is a
    stranger.
- **The core advertises the relay only while the connection is up,** and the System page shows
  its state: paired, connected, last heard, bytes this month, version.
- **The core's allocations dial out too.** Each WebRTC connection's ICE agent allocates on the
  relay with the core's credentials. Pion's ICE keeps the NAT binding alive with its own checks,
  and the allocation refresh keeps the relay's side. Behind carrier NAT this is outbound UDP 3478,
  or TCP 443 where UDP fails.

### 4.5 Credentials and the closed relay

- **Two roles, one secret, the role inside the HMAC:**
  - **`<expiry>:core`** credentials are minted by the core for its own allocations and never leave
    it.
  - **`<expiry>:client`** credentials last 15 minutes and are refreshed each heartbeat, exactly as
    `found` ones are today. An allocation made while they were good survives their expiry.

  A client cannot turn one role into the other without the secret.
- **The permission rule, allow-only:**
  - **A core allocation** may reach any public address. It must reach clients' server-reflexive
    addresses anywhere. `mayRelayTo` refuses private, shared and reserved ranges.
  - **A client allocation** may reach the relay's own relayed address alone: in practice, the
    core's allocations on the same box. Permissions are by IP only (§2.1), so the wrapped relayed
    socket of a client allocation also drops any packet whose destination address and port is not
    a live core allocation, which `OnAllocationCreated` records by username role.
- **So the pairs are:**
  - **The client reaches UDP:** the client's server-reflexive or public candidate pairs with the
    core's relayed one. The client allocates nothing it uses.
  - **The client has no UDP, or both sides are symmetric:** the client's relayed candidate pairs
    with the core's relayed one, on one box, by loopback.
  - **The client relayed and the core server-reflexive:** this pair is refused by design.
    Relay-to-relay covers it.
- **Quotas:**
  - a few allocations per client source;
  - a total near twice the core's `webrtc.max_connections`;
  - `QuotaHandler` refuses the rest.
- **On a provider with 1:1 NAT** (the public address not on the interface), relay-to-relay
  traffic to the public address may not loop back. The wrapper then delivers to the bound address.
  Hetzner Cloud puts the public IPv4 on the interface [unverified].
- **IPv6.** Clients and the core may reach the relay over IPv6. Whether pion/turn's relayed
  addresses can be IPv6 was not checked (Not tried). IPv4 relayed addresses suffice.

### 4.6 How the discoveries learn and hand out the relay

- **The core puts the relay in the heartbeat's `turn` field** (discovery-protocol.md §2.4), signed
  under its session as D438 §3 asks, with fresh client credentials each heartbeat:
  - `turns:<name>:443?transport=tcp` when the relay has a name and certificate (§4.9);
  - `turns:<name>:443`;
  - `turn:<address>:443?transport=tcp`;
  - `turn:<address>:3478?transport=udp`;
  - `turn:<address>:443?transport=udp`.

  This field is already "the owner's TURN" in the protocol. The relay is that, run elsewhere.
- **The paid discovery** passes the entries into `/paths`' `iceServers` today
  (`orpheum-connect/internal/clients/clients.go`, `pathsOf`) [repo]. It stores the heartbeat
  whole (`internal/store/heartbeat.go`) and does not check the list.
  - Proposed, small: bound the count, the schemes (`turn`, `turns`) and the lengths at intake,
    so a core cannot hand its clients junk.
- **The own discovery** (`discovery/single`) appends `held.Addresses.TURN` the same way
  (`discovery/single/client.go`) [repo]. Nothing to build.
- **The core's own heartbeat today** fills `turn` from its embedded TURN with `found` credentials
  (`cmd/orpheum/discovery.go`, `relayForFound`) [repo].
  - With a paired relay, the relay's entries come first. The embedded TURN's stay while the core
    is reachable on 3478 (a port-forwarded house).
  - Behind carrier NAT the embedded TURN's entries are dead weight, and the core can drop them
    when its outside probe (§2.6) never reaches 3478.
- **D438 §3's "the relay never registers with the discovery"** holds. The relay talks to the core
  alone.

### 4.7 The client's order of paths, the loader, and the starved path

- **The race is unchanged** (discovery-protocol.md §3.3):
  - LAN, then IPv6, then IPv4, then WebRTC;
  - within WebRTC, ICE's own priorities: host, then peer-reflexive, then server-reflexive, then
    relayed.

  So the relay is last by construction, and a punched pair always wins when it passes its checks.
- **Gathering already waits for a relay** when a TURN URL is configured
  [repo: `web/packages/sdk/src/rtc-connect.ts`, `gathered`, `hasRelayServer`]:
  - with a server-reflexive candidate, it waits up to `gatherForRelayMs` (1 s);
  - with no UDP there is no server-reflexive candidate, and the first relayed one ends the wait
    after `gatherAfterReflexiveMs` (300 ms);
  - `connectTimeoutMs` (10 s) bounds it all.

  The offer keeps relayed candidates first among its 10 (`relayed-offer.ts`). A paired relay adds
  up to a second to a connection that ends up direct, which the beta's timings should check (§6).
- **The loader loads over the relay with no change.**
  - Its own page and files come from the discovery over HTTPS.
  - It finds the core by the race.
  - Over the WebRTC path it fetches `/client-manifest`, then the client's `/assets/` files, over
    the kept path [repo: [client-from-core.md](../plans/client-from-core.md) §2.2a].
  - When UDP is blocked, that path is the relayed pair over TCP 443.
  - The Cast receiver reaches the core by the same WebRTC through the discovery [repo:
    `reach.ts`].
- **New: the starved path.**
  - When a connected WebRTC path makes no progress for a set time while requests wait, the client
    retries on the relay alone: `iceTransportPolicy: "relay"`, with only the TCP and TLS URLs.
    The cue is the SCTP stall the train showed. The time is proposed as 10 s, under the page's
    40 s silence limit.
  - It remembers, per `clientPrefix` (§3.3 already keeps which paths worked), that this network
    needs the relay, and starts there next time.
  - Without this, the relay fixes UDP-blocked networks and carrier-NAT cores, but not the train
    (§2.8). It is §8's question 4.

### 4.8 The cap (D438 §4)

- **The numbers, proposed** [repo: the relay report's addendum]:
  - 2 Mb/s per relayed stream and 4 Mb/s per household, held by the relay as its default;
  - at most 4 Mb/s × 30 days = 1,296 GB a month, about 1.3 TB, inside a CX23's 20 TB.

  Hetzner bills outbound traffic, and relay-to-relay crosses the box's network once [unverified:
  billing detail].
  - Because the household pays for its own box, §8's question 5 asks whether the admin may raise
    it.
- **The core enforces it, and says so.**
  - A connection whose selected pair is relayed (`pathOf` gives `PathRelayed` [repo]) is one more
    bound in upload control. The core already lowers the playback needing most onto a quality
    made, and tells the player its ceiling with each chunk [repo: `internal/core/playback`].
  - The household's relayed playbacks share the household figure.
  - The player shows the limit, because D72 says *"state the allowance; never throttle
    silently"*.
- **The pacer follows the relay.** rtcapi does not pace a relayed connection's sends to its TURN
  server today [repo: `internal/core/rtcapi/doc.go`]. With the core's allocation on the relay,
  everything that connection sends goes to the relay's address, so the pacer must follow that
  address at the cap's rate.
- **The relay backstops it.** Each allocation and the household total sit in token buckets
  about 1.5 times the core's figures, so a well-behaved core never meets them.
  - A core that is wrong, or a client credential pushing upward, meets them and loses packets.
  - The month's bytes are counted and reported. The relay refuses new client allocations past a
    monthly budget, if the owner wants one (§8's question 5).
- **Not capped:** browsing, artwork and control. They are small, and capping them would make the
  relayed client feel broken rather than slower.

### 4.9 A certificate for `turns:`

- **Chromium may not accept an IP-address certificate for TURNS** [unverified: §2.2]. A name is
  the safe choice.
- **On the paid service:**
  - The core already gets a wildcard certificate `*.<coreId>.<zone>` by DNS-01 through the
    discovery, and every address it publishes gets a name `<address>.<coreId>.<zone>`
    (discovery-protocol.md §2.7).
  - Proposed: the core gets a separate certificate for the relay's address name alone, and hands
    it and its key to the relay over the control connection.
  - The relay then holds a certificate for one name that means "the relay", never the core's
    wildcard.
  - A taken relay gains nothing from it: the DTLS fingerprint the core signs is what clients
    trust, not the TURN server's certificate.
  - The name record needs the discovery to publish the relay's address under the core. That is a
    `configured` candidate, which the protocol already has.
- **On an own discovery:**
  - with a domain, the relay takes a name from it by its own ACME, TLS-ALPN-01 on 443 (it owns
    443), on `golang.org/x/crypto/acme`, already in the module;
  - without one, a Let's Encrypt IP certificate by TLS-ALPN-01 (issued since January 2026, 160
    hours long) [repo: reachability-approaches B2], or no `turns:` at all and plain TURN-TCP on
    443.
- **Plain `turn:` on 443 always stays in the list,** for when the TLS fails.

### 4.10 Image, install, upgrade, monitoring

- **Install, on a CX23 or a CAX11, or any arm64 or amd64 box with a public IPv4:**
  1. Open TCP 443, UDP 443 and UDP 3478 in the provider's firewall.
  2. Paste a compose file that runs the core's public image as `orpheum relay`, with one volume
     for its key and secret.
  3. `docker compose up -d`, then read the code from `docker compose logs` or
     `docker compose exec relay orpheum relay code`.
  4. Enter the address and code on the core's System page, at home.
- **The compose file follows
  [D461](../../decisions/working-notes.md#d461--a-pasted-compose-file-keeps-working-released-files-frozen-and-tested-changes-additive-a-version-on-the-file):**
  frozen when released, additive, versioned. It is tested from a clean VM as
  [D463](../../decisions/working-notes.md#d463--more-install-routes-least-work-first-each-ours-to-keep-each-tested-in-a-vm)
  asks.
- **Upgrade:** pull and restart. The core reconnects, and the control hello's versions say what
  each side speaks. An older relay keeps working with a newer core within the protocol's major,
  as D273 §6 asks of every protocol here.
- **Monitoring:**
  - The core's System page is the household's monitor, from the control connection: connected,
    last heard, version, allocations, bytes this month, cap reached.
  - A relay silent past two pings shows as down, and the core stops advertising it.
  - There is no metrics endpoint on the relay to secure.
- **Status record and supervision,** inherited from the binary's other modes (D71). The relay runs
  under its container's restart policy, as the discovery mode does.

### 4.11 Security and abuse

- **No open relay** (D438 §3), from §4.5's two rules:
  - a client credential reaches only the core's allocations;
  - the core's credential, which never leaves the core, reaches only the public internet.

  §5 lists the tests that hold it.
- **What a leaked client credential costs:** traffic to the core, which the link holder could send
  anyway, within the backstop's rate and quota.
- **What a lost or taken-over relay costs** (D438 §3: "only its address"):
  - **It can:**
    - refuse or delay relayed connections, which fall back to whatever else works, or count as
      dead ends;
    - see the metadata in §4.12;
    - mint core credentials and relay UDP to the public internet on the household's own bill,
      as anyone holding the box could;
    - send the core junk on the control connection, which is bounded and strictly decoded.
  - **It cannot:**
    - read content;
    - pose as the core (the client checks the DTLS fingerprint the core signed);
    - reach the house's network (every core allocation is the core's outbound);
    - touch the household's discovery account.
- **Recovery:**
  - The admin unpairs on the System page. The core stops advertising, closes its relayed
    connections, and drops its key and secret.
  - Client credentials still out die within 15 minutes, and allocations end with their
    connections.
  - The admin pairs a new relay with a new code.
- **Abuse from outside:**
  - unauthenticated requests are rate-limited per source, against reflection [source: Enable
    Security];
  - only the ports §4.2 names are open;
  - the TURN secret is rotated over the control connection.
- **Pairing:** only an admin at home starts it (D297 §6). The code is used once and dies after
  five wrong tries (§4.3).

### 4.12 What the relay sees

- **It sees:**
  - the core's public address and every relayed client's;
  - when each connection starts and ends, and its bytes and packet timing;
  - TURN usernames, which carry an expiry and a role and nothing else;
  - the DTLS handshake's certificates, which are per-connection and self-signed;
  - SNI for TURNS: its own name.
- **It never sees** content, a user, a session or a film's id.
- **Traffic analysis is possible in principle.** Variable-bitrate video has been identified from
  encrypted traffic by its burst pattern [unverified: recalled, not fetched; "Beauty and the
  Burst", USENIX Security 2017].
  - The household's own box, under the household's own control, is the party that could do it, and
    a cloud provider could do the same at the house's own uplink.
  - The cap blurs it somewhat. Stated, not mitigated.

## 5. Build steps and tests

Sizes: S is a day or less, M a few days, L a week or more.

| Order | Step | Size |
| --- | --- | --- |
| 0 | **The measurements first** (§6): the heartbeat carries the core's path counts (the relay report's question 7, the owner's call); the dead-end report carries which candidate kinds were gathered; a starved connection is counted. Useful with or without a relay. | S–M |
| 1 | **`orpheum relay`:** the mode, its config, its key, the UDP 3478 and 443 listeners, the TCP 443 demultiplexer, pion/turn with the two roles, the allow-only permission rule and the wrapped relayed sockets, quotas. `internal/core/turn`'s shared parts move to a package both import. | L |
| 2 | **Pairing** (§4.3): the code on the relay; the System page's entry (address and code); the HMAC exchange over TLS; the keys pinned; the secret handed over and sealed; unpair on both sides. | M |
| 3 | **The control connection** (§4.4): the core's dial-out, the WebSocket, keepalive and backoff, status, rotation, drain. | M |
| 4 | **The core uses the relay:** `UseICEServers` adds the relay with core credentials; the heartbeat's `turn` carries client credentials while the control connection is up; the embedded TURN's entries are dropped when unreachable; the System page's relay card. | M |
| 5 | **The cap** (§4.8): a relayed connection's ceiling in upload control, the household figure, the player's note, the pacer following the relay, the relay's token buckets and monthly bytes. | M |
| 6 | **The certificate** (§4.9): the core's certificate for the relay's name handed over, and the relay's own ACME for an own discovery. | M |
| 7 | **The starved-path retry** (§4.7): detection, a relay-only retry over TCP, remembered per `clientPrefix`. Waits on §8's question 4. | M |
| 8 | **Package:** the compose file, the install guide (Hetzner CX23 and CAX11), and the VM test (D463). | M |
| 9 | **The paid discovery's bounds** on the `turn` list (§4.6). | S |

- **Total:** roughly four to six weeks of work [est: the sizes above]. Steps 1–4 make a relay that
  works, 5 makes it allowed under D438 §4, and 6–9 make it good.
- **Tests, each failing when its change is reverted (D190):**
  - **The fallback.** A client whose UDP is dropped reaches the core through the relay over TCP,
    and a core behind a symmetric NAT is reached through relay-to-relay.
    - pion's `vnet` cannot carry this alone. In `transport/v5` v5.1.1, `vnet.Net.DialTCP` and
      `ListenTCP` return `ErrNotSupported` [repo: module cache].
    - So the test gives the client a `transport.Net` that sends UDP through `vnet`, with a filter
      dropping every UDP packet, and TCP through the loopback.
    - The lossy-link harness is the model [repo: `lossy_link_internal_test.go`]. Like it, the test
      is not parallel, because vnet's MAC counter is unlocked.
  - **The lock.**
    - A client allocation refuses a permission or a send to any address but a live core
      allocation.
    - A core allocation refuses private, shared and reserved ranges, and the relay's own other
      ports.
    - A client username cannot be made into a core one.
    - An expired client credential is refused, but an allocation it made stays.
  - **Pairing:**
    - a wrong code fails and counts;
    - the fifth failure voids the code;
    - a second core's key is refused after pairing;
    - a replayed transcript from another connection fails, because the exporter binds it.
  - **Control:**
    - under `synctest`, two missed pongs end the connection, and the core stops advertising the
      relay at once;
    - reconnection backs off.
  - **The cap:** a relayed playback's ceiling is the cap's, the player is told, and the relay's
    bucket drops past 1.5 times.
  - **The discoveries:** `through-discovery.spec.ts` runs a connection with
    `iceTransportPolicy: "relay"` over the relay's TCP URL, against `dev discovery` and against
    `single`.
- **Device checks (VQ items, rule 4):** each over TURN-TCP and over TURNS, on a network where UDP
  is blocked:
  - Chrome on a computer and on Android;
  - Firefox;
  - Safari on iOS;
  - Silk on the Fire TV;
  - the Cast receiver.

  A phone hotspot behind a firewall dropping UDP is the lab's network, and the train is the field's.

## 6. Measurements for D438 §1, and whether the train counts

- **What the beta needs to know, and the gap in each:**
  - **How many remote connections fail outright.** D314 §4 counts dead ends, but there is no
    denominator. The core counts its paths (`direct_ipv6` … `relayed`, `failed` [repo:
    `internal/core/rtcapi/path.go`]), and whether it reports them is the owner's call (the relay
    report's question 7). Step 0 proposes yes.
  - **Why they fail.** The dead-end report carries the client's ICE stage [repo: `RtcStage`], but
    not whether a server-reflexive candidate was gathered. That one fact separates "UDP blocked
    here", which a relay fixes over TCP, from "both sides symmetric", which a relay fixes over
    UDP.
  - **How many connect and then starve.** Nothing counts this today. The train was one.
    - Proposed: count a connection whose channel made no progress for 10 s while requests waited,
      by path kind.
  - **Whether a configured relay slows connections that end up direct** (§4.7's up-to-a-second).
    This needs gathering timings, which `slowConnection` already reports when slow [repo].
  - **Complaints.** The testers' word, as D438 §1 says.
- **A threshold, proposed for the owner:** reopen when a month's failed and starved remote
  connections together pass about 2% of remote connection attempts, or when any tester cannot
  reach their core from somewhere they regularly need [est: the relay report's 5–20% of bytes
  is a share of traffic, not of attempts, and 2% is a judgement].
- **Does the train already count?** As evidence of the kind, yes, not as a rate.
  - It is the owner's own household, failing where YouTube worked, and it is the complaint D438
    §1 anticipates ("people complain about not being able to reach their home servers").
  - It also shows the dead-end count would have missed it.
  - So the recommendation is to build step 0 now, whatever is decided about the relay, so the beta's
    data includes the starved case.

## 7. Where it meets the self-hosted discovery research

The parallel research is `research/self-hosted-discovery-2026-11-13.md`, read in its working
tree, not yet in this one.

- **The same machine.**
  - That research recommends a small VPS for the own discovery, with *"room for the TCP path or
    D438's relay on the same machine"* (its §4.1).
  - It leaves its TCP path to this document (its §7, its step 10, its question 4).
  - This design fits on that VPS: the relay's demultiplexer on 443 sends `h2` and `http/1.1` to
    the discovery's HTTPS (§4.2).
  - That avoids the second IPv4 address, or the port 5349, that its §7 weighs.
- **One process or two** is §8's question 7. Either way the discovery and the relay keep separate
  keys, and the relay still never talks to the discovery (D438 §3).
- **The same field.** The own discovery hands out `turn` as the paid one does. This needs no
  change on either side (§4.6).
- **The same code shape.** Both use a 12-character safe32 code from `discovery/safe32code` for
  anything a person types, with the setup code's rate limits.
- **Its question 4,** whether the TCP path comes in the own discovery's first release, is this
  document's step order. One build serves both.
- **Its STUN on the VPS** (its §7) and this relay's TURN answer Binding requests on the same
  3478 if they share a box. That is one listener, the relay's, which already answers STUN.

## 8. Questions for the owner

1. **The transport.** TURN on the relay first (A), with HTTPS passthrough (B) only if the beta
   shows relayed playback hurting?
   *Recommendation:* yes, A. It works without the core's names, needs no client transport, and B
   reuses its pairing, control connection and cap.
2. **The closed relay.** Client credentials reach only the core's own allocations, so every
   relayed pair is relay-to-relay or the client's direct address to the core's relayed one.
   *Recommendation:* yes. It makes "no open relay" a property, not a deny-list, at the cost of a
   pair type relay-to-relay already covers.
3. **The pairing code.** 12 safe32 characters with an HMAC exchange and no new dependency, or a
   PAKE, which needs one?
   *Recommendation:* the 12-character code. Its offline margin is years against an hour's life.
4. **The starved-path retry** (§4.7). Should the client retry on the relay alone when a connected
   path stalls, and remember the network?
   *Recommendation:* yes. Without it the relay does not fix the train.
5. **The cap's numbers, and who may change them.** 2 Mb/s per stream and 4 Mb/s per household by
   default, with the admin allowed to raise them on their own relay? A monthly budget?
   *Recommendation:* the defaults as stated, the admin may raise them (the box and the bill are
   theirs, and D72's rule is to state the limit, not to impose ours), and no monthly budget,
   since 4 Mb/s already bounds a month at 1.3 TB.
6. **The relay's certificate on the paid service.** The core obtains a certificate for the relay's
   address name and hands it over (§4.9)?
   *Recommendation:* yes. A name makes `turns:` reliable across browsers, and the certificate
   grants nothing.
7. **On an own discovery's VPS, one process or two?** `orpheum discovery` with the relay inside
   it, or two services sharing 443 through the relay's demultiplexer?
   *Recommendation:* two services, the relay owning 443 and handing HTTP over by ALPN. Each stays
   one mode of the binary with its own keys, and a household can run either alone.
8. **Measurements now** (§6, step 0). Report path counts in the heartbeat, gathered candidate kinds
   in the dead-end report, and starved connections?
   *Recommendation:* yes, now, before the beta's data is wanted. It is the data D438 §1 waits for.
9. **Does the train reopen D438 §1?**
   *Recommendation:* it is evidence of the kind, from the owner's own household. Build step 0
   now, and decide on the relay when a month of beta data, with the starved count, is in.
10. **Watch Together's voice.** It keeps the core's embedded TURN. The closed relay deliberately
    carries nothing between two browsers.
    *Recommendation:* keep voice off the household relay. A carrier-NAT household's voice then
    still relies on direct pairs, which is stated, not hidden.
11. **Plain TURN-TCP on 443 beside TURNS.** Offer both, since plain TCP fails on
    protocol-checking firewalls and TURNS fails on certificate quirks?
    *Recommendation:* both, and UDP 443, and let the device checks show which each client takes.

## Sources

All read 2026-11-13. Notes with the quotes are in
`/yoloai/cache/http/household-relay-2026-11-13/notes.md`.

- RFC 8656, TURN: <https://www.rfc-editor.org/rfc/rfc8656.html>
- draft-uberti-behave-turn-rest-00:
  <https://datatracker.ietf.org/doc/html/draft-uberti-behave-turn-rest-00>
- coturn's example configuration:
  <https://raw.githubusercontent.com/coturn/coturn/master/examples/etc/turnserver.conf>
- Enable Security, TURN server security threats:
  <https://www.enablesecurity.com/blog/turn-server-security-threats/>
- Tailscale, connection types: <https://tailscale.com/docs/reference/connection-types>
- Tailscale's DERP package, v1.104.7: <https://pkg.go.dev/tailscale.com/derp>
- Syncthing relay server: <https://docs.syncthing.net/users/strelaysrv.html>
- Syncthing relay protocol v1: <https://docs.syncthing.net/specs/relay-v1.html>
- Plex, accessing a server through Relay:
  <https://support.plex.tv/articles/216766168-accessing-a-server-through-relay/>
- WebKit bug 219274: <https://bugs.webkit.org/show_bug.cgi?id=219274>
- WebKit bug 320931: <https://bugs.webkit.org/show_bug.cgi?id=320931>
- Hetzner Cloud, cost-optimised plans: <https://www.hetzner.com/cloud/cost-optimized/>
- **[unverified], search summaries only:**
  - Mozilla bugs 1056934, 1922559 and 2011209;
  - the Chromium WebRTC SNI commit;
  - LiveKit's IP-certificate thread;
  - coturn CVE-2026-73212 and CVE-2026-73213 (SentinelOne, OSV, OpenCVE).

## Not tried

- **No code was run:**
  - no pion/turn on a CX23 or a CAX11;
  - no TURN-TCP or TURNS through a real browser;
  - no throughput of SCTP over TURN over TCP at the cap's rate.
- **No device was checked:** Safari on iOS, Silk, the Cast receiver, Chrome on Android. §2.2's rows
  for them come from engines and bug trackers.
- **Browser behaviour not checked:**
  - whether Chromium accepts an IP-address certificate for TURNS;
  - whether any browser sends ALPN `stun.turn`.
- **Not checked in pion/turn:** IPv6 relayed addresses; relay-to-relay on a provider with 1:1 NAT.
- **Not checked at Hetzner:** whether the public IPv4 is on the interface, and how loopback
  traffic is billed.
- **Not checked at the paid discovery:** how its `ProbeTURN` probe treats a relay's TURN.
- **The train's own network was not tested for UDP 443 or TCP 443 to a TURN server.** Whether the
  relay would have served it is inferred from YouTube playing.
- **Not fetched:** the "Beauty and the Burst" paper.
