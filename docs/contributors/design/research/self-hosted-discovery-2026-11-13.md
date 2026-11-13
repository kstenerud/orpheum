> **ABOUTME:** Research and a proposed design for the discovery a household runs itself instead
> of the paid service: how comparable systems let a person reach home without the vendor, what
> our protocol already asks of a discovery, the shapes it could take, and the build steps and
> owner's questions that would make it real. It proposes; it decides nothing.

# A household's own discovery: research and a proposed design

- **Date:** 2026-11-13. Asked by the owner: *"We're going to need the self hosted discovery at
  some point, so a proper research and design would be good."*
- **Scope:** reading the repository, the paid service's specification (`orpheum-connect`, read
  only) and the public documentation of comparable systems. No code changed, nothing in
  staging or production touched, nothing measured. Fetched pages are cached in
  `/yoloai/cache/http/self-hosted-discovery-2026-11-13/`, each as `.html` and a stripped `.txt`.
- **Marks:** **[source]** a page fetched today, 2026-11-13, listed under Sources; **[repo]** a
  fact from this repository, with its path; **UNVERIFIED** seen only in a search summary or
  remembered, not read today. Unmarked text is analysis or proposal.
- **History, not specification:** this document sits in `design/research/`. Where it proposes,
  the proposal binds nothing until the owner records a decision.

## Summary

- **What is decided already.** A household may run its own single-target discovery, "one VPS
  points to one core"
  ([D273](../../decisions/working-notes.md#d273--clients-reach-a-core-through-apporpheum-appcom-or-the-owners-own-discovery-by-whatever-methods-work-with-no-traffic-relayed-by-us-a-node-without-landlock-runs-and-warns)
  §3), with the paid tier's shape
  ([D274](../../decisions/working-notes.md#d274--a-core-is-reached-first-by-what-needs-nothing-from-its-owner-webrtc-through-the-discovery-direct-paths-are-upgrades-and-a-relay-waits-for-demand)
  §5), from public packages
  ([D275](../../decisions/working-notes.md#d275--orpheums-discovery-is-public-building-blocks-that-a-closed-multi-core-service-imports-and-the-server-moves-to-the-agpl-with-a-contributor-agreement)),
  enrolled by a one-time key as on the paid service
  ([D310](../../decisions/working-notes.md#d310--the-closed-beta-runs-on-the-paid-service-free-an-account-gives-one-key-for-one-core-the-cores-admin-enters-it-at-home-and-the-household-then-uses-the-services-app)
  §7), signed by the home core with software keys while the VPS holds none
  ([D394](../../decisions/working-notes.md#d394--keys-fit-the-threat-the-paid-services-by-ceremony-a-self-hosters-and-stagings-by-software-keys-suspend-and-resume-a-guided-tool-runs-the-ceremony)
  §1–2), and depending on nothing of the owner's
  ([D391](../../decisions/working-notes.md#d391--a-discovery-is-a-key-its-names-are-locations-greenfield-names-signed-address-documents-and-release-records-an-own-discovery-depending-on-nothing-of-the-owners)
  §8). It comes with the open beta (D310 §1, D313 §7). This document does not reopen any of that.
- **What the others do.** Every system that reaches home through CGNAT without a vendor has the
  person rent a machine with a public address: headscale and a DERP server, Syncthing's
  discovery and relay, a WireGuard VPS. The systems that need no rented machine (Jellyfin, Home
  Assistant without Nabu Casa, Plex without its relay, dynamic DNS) all need an inbound port at
  home, and Home Assistant's own page says CGNAT "is becoming fairly common". Cloudflare Tunnel
  needs no port and no machine but its terms limit video through the CDN. The Tailscale and
  Syncthing pattern, a key-rooted identity with a rentable rendezvous and a relay that cannot
  read the traffic, is the one ours already follows.
- **Recommended shape: one small VPS running `orpheum discovery`**, a mode of the one binary
  and image. On the household's own domain it serves `discovery/single` (built), the loader at
  the loader origin and at the one client origin (D308's roles, served from the binary), the
  signed `/.well-known/orpheum-discovery` files the core pushes to it, and STUN. Its certificate
  comes from its own ACME client. A TCP path on 443 is an optional part of the same VPS, decided
  with the household-relay research running beside this one.
- **The two other shapes are partial.** The home core alone, with a forwarded port and dynamic
  DNS, can be its own discovery for a household with a public IPv4 or IPv6 address. It fails
  under CGNAT, shows nothing when the house is down, and puts the loader on the core's network.
  A static host with signed files can be a second location for the key set and the address
  document (D393's fetch from every location), but it cannot signal, so it is a complement and
  never a discovery on its own.
- **What D308 changed.** W4b.5's "files taken from the core" is no longer a discovery's job. The
  loader fetches the client from the core under the core's signed manifest, so the discovery
  serves only the loader, which is in its own binary. W4b.5 shrinks to serving the loader and
  its frozen endpoints (D425 §4) at two origins of the household's domain.
- **The train report is the strongest evidence for a TCP path.** On 2026-11-13 the train's WiFi
  starved our UDP while YouTube played [repo: `idiosyncrasies.md`]. An own discovery with STUN
  alone would fail the same way. TURN over TLS on 443, or D438's relay with a TCP side, on the
  same VPS is the remedy. Both need the paid-service answer first, so they are a later step here.
- **Build steps** (§11), in order: the conformance suite; the client API's missing parts; STUN;
  the core making its software key set and pushing the signed files; the loader at the
  household's origins; the command with ACME and a config file; switching in the core's UI; the
  guide and a VM-tested compose file; then the TCP path; Cast stays an open question.
- **The owner's questions** (§12): the client origin's domain, the certificate route, where the
  recovery key lives, the TCP path, Cast, whether a core may be enrolled at two discoveries, the
  device with no link, NAT measurement on one address, and whether the home-core shape is
  offered at all.

## 1. What a discovery must do today

The paid service and an own discovery answer one protocol
([discovery-protocol.md](../discovery-protocol.md)) [repo]. Its parts, by who calls them:

- **The core, at `/core/v1`:** `challenge`, `enrol`, `session`, `status` (which returns
  `iceServers`, `clientOrigin` and `loaderOrigin`, D293 §4), `addresses` (including the core's
  own TURN), `signal` (a WebSocket carrying offers), `acme-challenge` and `acme-account` (the
  DNS-01 record for a core's own certificate), `locate-hashes`, `locators`, and `DELETE /core`.
- **A client, at `/client/v1`:** `locate`, `paths`, `cores/{id}/offer`, device codes (on an own
  discovery passed through to the core's `/pairings`), the dead-end report, and `client-floor`,
  which an own discovery uses to refuse no client version.
- **The loader's frozen endpoints** (D425 §4): `/session/handoff`, `/session/redeem`,
  `/client-manifest`, `/client-problems`, `/rtc/offer`, `/reachability/{nonce}`, `/pairings`,
  `/pairings/state`, `/pairings/session`.
- **The signed files at `/.well-known/`** (D391, D393): the key set's genesis and statements,
  and the address document naming the issuer, the core and client APIs, the loader origin, the
  client-origin pattern, the account origin, a homepage, a support address and fallback
  locations. A core is given an issuer once, with its admin's password, requires an exact match
  of the issuer and a signature that chains to the root set, and then pins the root set.
- **Every answer** carries `Orpheum-Discovery-Protocol`, and a deprecated major carries
  `Orpheum-Discovery-Deprecated`. The two previous majors are served for at least six months
  (D273 §6).

What exists for one core [repo]:

- `discovery/single` answers the core API and most of the client API over one state file
  (`record.go`), and `wellknown.go` serves the signed files byte for byte through
  `trust.Served`.
- `orpheum dev discovery --listen --state [--realm]` (`cmd/orpheum/dev_discovery.go`, built only
  with the `dev` tag) serves it with the web client on one origin, and
  `web/e2e/through-discovery.spec.ts` reaches a core through it in Chromium and Firefox (W4b.6).
- The core's side is complete: `orpheum pair --discovery <issuer>` (D334) and the admin's
  `/system/discovery` page connect, claim with the key, leave and ask again; `connect` asks for
  the discovery's identity when the issuer is not the suggested one (`suggested.go`, D391 §7).
- `cmd/discovery-sign` and `cmd/discovery-watch` sign and watch the paid service's key set.

What is missing, from [discovery-mode.md](../plans/discovery-mode.md) and the draft guide for the
open beta [repo]:

- W4b.3, STUN; W4b.4's `lan` path and rate limits on strangers; W4b.7's conformance suite
  `discovery/discoverytest`;
- W4b.5, serving the web app; W4b.8, the command, its enrolment key and `rekey`;
- D394 §2's core making its own key set and getting the signed files to the VPS, whose transport
  D394 leaves to "when this is built".

## 2. How comparable systems do it

Each system below is described by the same questions: what the person owns or runs, how a
client learns the address, where trust is rooted, what happens behind NAT and CGNAT, the work
it asks, and where it fails. Quotes are from pages fetched 2026-11-13.

### 2.1 Syncthing's own discovery and relay servers

- **Runs:** `stdiscosrv` (discovery) and optionally `strelaysrv` (relay), on any machine with a
  public address; the discovery listens on ":8443" by default [source: stdiscosrv].
- **Client learns it:** by configuration on every device. "If you wish to use only your own
  discovery server, remove the default" [source: stdiscosrv].
- **Trust:** the device ID is the hash of the device's certificate, so a discovery server
  cannot impersonate a device. The server itself may use a CA certificate or "any certificate
  pair and let clients authenticate the server based on its 'device ID'", in which case "this
  device ID (fingerprint) must be given to" the clients [source: stdiscosrv]. Syncthing is
  frank about what the operator sees: "The operator of the discovery server can map arbitrary
  device addresses to IP addresses, and deduce which devices are connected to each other"
  [source: security].
- **NAT:** the relay carries what cannot connect directly; relayed data "is encrypted as usual"
  [source: security]. A private relay takes `-token`, which "Disables joining any pools", and
  can run "on an unprivileged port with port forwarding from e.g. 443", with `-global-rate` and
  `-per-session-rate` limits [source: strelaysrv].
- **Burden:** two small daemons, a certificate, and a fingerprint typed into each device.
- **Fails when:** a device was not configured; the server is down (devices that know each other
  keep their last addresses).
- **For us:** the closest match. Identity by key, a rentable rendezvous, a relay limited to its
  own users by a token, and rate limits as flags. Our D391 identity check at `connect` is
  Syncthing's typed fingerprint.

### 2.2 Tailscale with headscale, and DERP

- **Runs:** headscale, "An open source, self-hosted implementation of the Tailscale control
  server" with "a narrow scope, a _single_ Tailscale network (tailnet), suitable for a personal
  use, or a small open-source organisation" [source: headscale README]. It needs "A server with
  a public IP address", dual-stack recommended, "served via HTTPS on port 443", and port 80
  only for Let's Encrypt's HTTP-01 challenge [source: headscale requirements]. A custom DERP
  relay needs HTTP 80, HTTPS 443 and STUN 3478 open, and Tailscale calls running one "an
  advanced operation that requires significant resources on your part to set up and maintain"
  [source: custom DERP].
- **Client learns it:** a login server URL typed into each client.
- **Trust:** the control server distributes node keys, so it is trusted completely; the relay
  is not, since "Tailscale private keys never leave the local device that generated them, it's
  impossible for a DERP server to decrypt your traffic" [source: DERP servers].
- **NAT:** DERP is "both our fallback of last resort to get connectivity, and our helper to
  upgrade to a peer-to-peer connection". It "runs over HTTP, which is handy on networks with
  strict outbound rules" [source: NAT traversal]. Two hard NATs in one path break direct
  traversal: "It only takes one of them in the whole path to break our current traversal
  plans" [source: NAT traversal]. A private DERP is restricted to its own tailnet by
  `--verify-clients` [source: custom DERP].
- **Burden:** a VPS, a domain, a certificate, client updates to track ("To stay compatible with
  Tailscale client updates, you might need to update" DERP) [source: custom DERP].
- **Fails when:** the control server is down for new logins; a client that cannot reach the
  DERP over TCP 443 and is behind hard NAT.
- **For us:** DERP over HTTPS on 443 is exactly the path the train report says we lack. Its
  trust split (the coordinator trusted, the relay blind) is ours: the discovery signals, the
  relay carries DTLS it cannot open (D438).

### 2.3 Matrix's well-known delegation

- **Runs:** a homeserver anywhere; a static file on the domain the user's ID names.
- **Client learns it:** from the user ID's domain. A client makes "a GET request to
  `https://hostname/.well-known/matrix/client`"; a 404 means use the name as is, any other
  failure means "FAIL_PROMPT" [source: client-server API]. Servers use
  `/.well-known/matrix/server` the same way [source: server-server API]. The client API
  recommends `Access-Control-Allow-Origin: *` so web clients can read it [source:
  client-server API].
- **Trust:** the Web PKI for the domain, and the homeserver's signing keys for federation:
  "authentication provided by public key signatures both at the TLS transport layer and in HTTP
  Authorization headers" [source: server-server API].
- **NAT:** not addressed. The homeserver must be reachable.
- **Burden:** one static JSON file; the person already owns the domain.
- **Fails when:** the domain lapses, or the file is wrong.
- **For us:** the model for `/.well-known/orpheum-discovery` (D391), with one difference: ours
  is signed under the household's own key set, so a host that serves it cannot change it
  undetected by a core.

### 2.4 Home Assistant without Nabu Casa

- **Runs:** the home instance, with a forwarded port, a VPN, or a reverse proxy [source: HA
  remote access]. Nabu Casa's paid route is "secured with end-to-end encryption" through an SNI
  proxy (SniTun), whose source is public [source: Nabu Casa].
- **Client learns it:** an external URL the person types; dynamic IPs are solved "by using a
  free Dynamic DNS service like DuckDNS" [source: HA remote access].
- **Trust:** the Web PKI for the URL.
- **NAT:** "check if your ISP provides you with a dedicated IP, instead of one shared with other
  users via a CG-NAT. This is becoming fairly common nowadays due to the shortage of IPv4
  addresses. Some, if not most ISPs will require you to pay an extra fee to be assigned a
  dedicated IPv4 address" [source: HA remote access]. The VPN route is Tailscale or ZeroTier.
- **Burden:** port forwarding by router model, certificates, and "Just putting a port up is not
  secure" [source: HA remote access].
- **Fails when:** CGNAT; an ISP blocking inbound ports; a router without forwarding.
- **For us:** the reason the home-core shape cannot be the only one. The vendor's own guide
  sends CGNAT households to a VPN or to the paid service.

### 2.5 Jellyfin and Plex without the relay

- **Jellyfin runs** only at home: ports 8096 (HTTP) and 8920 (HTTPS), and 7359/UDP for local
  client discovery [source: Jellyfin networking]. Remote access is a forwarded port or a reverse
  proxy, with a Tailscale page beside it in its guide; there is no relay and no rendezvous.
  Clients are given a URL. Trust is the Web PKI if the person sets up a certificate.
- **Plex runs** at home and asks the router by UPnP or NAT-PMP, or the person forwards a port
  to 32400 by hand; "Double-NAT" is named as a cause of failure [source: Plex remote access].
  Plex's account service tells clients the address (UNVERIFIED). When nothing works, Plex relays
  through its own servers: "Connections are limited to 2 Mbps maximum for streams (this applies to both
  Plex Pass and Remote Watch Pass subscribers)", and with secure connections "the content is
  encrypted end-to-end and tunneled through our Relay" [source: Plex relay].
- **Fails when:** CGNAT or double NAT, without the vendor's relay.
- **For us:** Plex's relay is the paid-service question D438 answers for a household's own
  machine, and its 2 Mbps cap is the same kind of limit D438 sets.

### 2.6 Cloudflare Tunnel

- **Runs:** `cloudflared` at home, which "creates outbound-only connections to Cloudflare's
  global network", so origins serve "without a publicly routable IP address" [source: Tunnel].
  A domain on Cloudflare's DNS.
- **Client learns it:** an ordinary hostname.
- **Trust:** Cloudflare terminates TLS, so it sees every byte.
- **NAT:** solved, since every connection is outbound.
- **Burden:** low.
- **Fails when:** the content is video. Unless the person is an Enterprise customer, Cloudflare
  "offers specific Paid Services (e.g., the Developer Platform, Images, and Stream) that you
  must use in order to serve video and other large files via the CDN" and reserves the right to
  limit use "if you use or are suspected of using the CDN without such Paid Services to serve
  video" [source: Cloudflare terms]. HTTP through the tunnel is also not WebRTC.
- **For us:** fine for the signalling and the loader, wrong for playback. The repository's
  earlier tunnel (`deploy/vps/`, D274 §5's optional tunnel) is the same idea on a person's own
  VPS.

### 2.7 A VPS with WireGuard

- **Runs:** a VPS with a public address and a WireGuard peer at home.
- **Client learns it:** the VPS's name, which then forwards to the home peer.
- **Trust:** WireGuard keys between the VPS and home; the Web PKI for whatever the VPS serves.
- **NAT:** the home peer dials out and keeps the mapping open with persistent keepalives: "if a
  peer behind NAT or a firewall wishes to receive incoming packets, he must keep the
  NAT/firewall mapping valid, by periodically sending keepalive packets" [source: WireGuard].
  CGNAT is no obstacle, since the VPS never dials home.
- **Burden:** two configurations, and every byte crosses the VPS, so its traffic allowance
  matters.
- **Fails when:** the VPS is down; its traffic allowance runs out.
- **For us:** the owner's own Edge VPS ran this way [repo: `orpheum-ops/machines.md`]. D274 §5
  makes the tunnel optional for an own discovery. D438's relay keeps the dial-out and drops the
  terminate-everything: the relay forwards DTLS it cannot open.

### 2.8 Dynamic DNS

- **Runs:** an updater at home calling a provider, for example DuckDNS's
  `https://www.duckdns.org/update?domains=…&token=…&ip=`, where a blank `ip` means "we detect
  your gateway ip" [source: DuckDNS]. DuckDNS is free and runs "at almost zero outlay excluding
  our time" [source: DuckDNS].
- **Client learns it:** a hostname.
- **Trust:** the Web PKI; the certificate needs HTTP-01 on port 80 or DNS-01 through the
  provider's API. HTTP-01 "can only be done on port 80" and does not issue wildcards; DNS-01
  does, and "it only makes sense to use DNS-01 challenges if your DNS provider has an API"
  [source: Let's Encrypt].
- **NAT:** none; it names an address that must already be reachable.
- **Burden:** small, and the free service's continuity is the person's risk.
- **Fails when:** CGNAT, since the gateway address it detects is shared.
- **For us:** a piece of the home-core shape (§4.2), never the whole of anything.

### 2.9 What the comparison says

- **A rented public machine is the only way through CGNAT without a vendor.** Every system that
  reaches a CGNAT household (headscale with DERP, Syncthing with its relay, WireGuard) has one;
  the rest require an inbound port. So the VPS shape is the general answer and the home-core
  shape a special case.
- **Trust is rooted in a key the person holds, and the rendezvous is replaceable.** Syncthing's
  device IDs, Tailscale's node keys and Matrix's signing keys all let the rendezvous be
  untrusted for the data. Our design does the same (D391, D394 §2); what stays trusted is
  whoever serves the web page's JavaScript (protocol §3.7, RFC 8826 §4.3).
- **The relay of last resort runs over TCP 443.** DERP does, Syncthing's relay can be forwarded
  from 443, ICE-TCP exists because "overly restrictive NAT and firewall devices prevent
  UDP-based communications altogether, but general TCP-based communications are permitted"
  [source: RFC 6544]. The train report is that case.
- **Everyone tells the person to configure each client.** Ours is better: a link carries the
  core's fingerprint, and the core gives the discovery its key set, so no client is configured
  by hand. Only the issuer, given once to the core, is typed.

## 3. The duties: minimum and parity

**Minimum, for a household reaching its core from away in a browser:**

- serve the signed `/.well-known/` files the core made, byte for byte (built in `single`);
- the core API: enrol, session, status, addresses, signal (built);
- the client API: locate, paths, offer (built), with rate limits on strangers (missing);
- the loader, at the loader origin and the client origin, with the frozen endpoints its
  hand-off and manifest check use (D308, D425 §4) (missing outside the dev command);
- STUN on its own address (W4b.3, missing; a public STUN server works meanwhile);
- its own certificate for its names (missing; Q5).

**Parity with the paid service:**

- device codes for a TV with no link, passed through to the core's `/pairings` (Q6 below);
- the `lan` path for a client on the core's network (W4b.4);
- `acme-challenge` for the core's own direct HTTPS names, through the household's DNS
  provider (protocol §2.7);
- the dead-end report and `/client-problems`, so the household's admin sees failed connections
  as the paid service's do (D314 §4);
- `/reachability/{nonce}`, the outside check of the core's address;
- a TCP path for networks that throttle or block UDP (§7);
- the last-known paths while the core is away, as the paid service's frozen state gives [repo:
  `orpheum-connect/docs/spec.md` §3.10];
- a Cast receiver at the loader's origin with its own application ID (D467; §12 Q5);
- watching the key set from outside, as `discovery-watch` does for the paid service.

**Not needed by an own discovery:** realms (D298 keeps self-hosting realm-free), accounts,
billing, lapse states, many cores, and refusing old clients by `client-floor`.

## 4. Deployment shapes

### 4.1 A small VPS (recommended)

- **What runs:** `orpheum discovery` from the same image the core uses, on a VPS with a public
  IPv4 and IPv6 address, under the household's domain: an API name, a loader name and one
  client-origin name, all on one address.
- **What it gives:** every duty in §3, including signalling while the house's network changes,
  a status answer while the core is away, STUN from an address outside the home, and room for
  the TCP path or D438's relay on the same machine.
- **What it costs:** a domain and a VPS. The paid spec prices the free alternative at about
  €5.50 a month plus IPv4 at Hetzner [repo: `orpheum-connect/docs/spec.md` §1.4];
  [relay-costs-2026-11-06.md](relay-costs-2026-11-06.md) found a €5.99 Hetzner server with 20 TB
  included. D273 §3 states the comparison: the paid tier's $3 is cheaper.
- **What it gives up:** nothing the paid service has, except the owner's watching and the
  owner's key ceremony (D394 §1 makes a self-hoster's keys software keys).

### 4.2 The home core as its own discovery, no VPS

- **What runs:** the core also serves `discovery/single` and the loader, on a forwarded port 443
  under a name kept current by dynamic DNS, or on its own public IPv6 address.
- **What it gives:** no rent. With a reachable address the core hardly needs WebRTC through a
  rendezvous at all; direct paths are D274's upgrades.
- **What it gives up:**
  - every CGNAT household, which Home Assistant calls "fairly common" (§2.4);
  - an outside STUN mapping and an outside reachability check, since both ends are the core;
  - any answer while the house is offline, so a device away from home cannot even say "the
    server is off";
  - the separation of the loader from the core: one machine compromised gives both, where D308
    put the loader's integrity check on another host;
  - port 80 or a DNS API for the certificate (§2.8).
- **Fit:** households with a public address that want no VPS. Not decided by D273 §3, which names
  a VPS, so whether to offer it is an open question (§12 Q9).

### 4.3 A static host with signed files

- **What runs:** any static host (a web host, an object store) serving files the core signed.
- **What it gives:** the `/.well-known/` files, so a second location for the key set and the
  address document. D393 already has a core keep fetching from every location used in the past
  year, and D391's address document lists fallback locations; a static mirror is cheap
  insurance against a VPS that goes dark.
- **What it gives up:** signalling, so no WebRTC; the loader's endpoints, which are dynamic
  (`/session/handoff`, `/rtc/offer`); STUN. A household could live on it only with a directly
  reachable core and native clients, which do not exist yet.
- **Fit:** a complement to §4.1, never a discovery alone. Proposed as an optional later step.

## 5. Keys, signing and trust

- **The core makes the household's key set at setup** (D394 §2): software keys (D394 §1), an
  everyday key that may only suspend and a recovery key for every other change (D395, enforced
  by `trust.Root.onlySuspends`). The core signs the genesis and the address document and sends
  the signed files to the VPS, which holds no key.
- **Where the recovery key lives is not decided.** Keeping it beside the everyday key on the core
  makes it worth nothing against a taken core, which is the case it exists for. Proposed: the
  core shows it once at setup as a secret to print or save, as the identity key's backup is
  handled (D289), and keeps only the everyday key (§12 Q3).
- **How the files travel (D394 §2 leaves this to the build).** Options:
  - **A call on the core API, within the core's session** (proposed): `PUT /core/v1/published`
    carrying the files. The discovery checks that they chain to the root set it was enrolled
    under, stores them atomically beside its state, and serves them byte for byte through the
    `trust.Served` path `wellknown.go` already has. The core then fetches its own
    `/.well-known/orpheum-discovery` back from the public name and compares bytes, so a VPS
    that serves something else is caught at once.
  - A message on the signalling WebSocket: no new route, but it mixes a rare write into the
    stream of offers.
  - The admin copies the files by hand: no code, and a step people forget.
- **Enrolment roots the VPS's trust in the core.** The command prints a one-time key (D310 §7);
  the core presents it at `enrol`. From then the core's identity key authenticates its session,
  and the root set in the first files the core pushes is the one the discovery accepts later
  pushes under. A re-key is W4b.8's `rekey`.
- **The core's own check is the same as for the paid service.** The issuer is the household's
  `https://` name, which `connect` requires to match exactly (RFC 8414 §3.3), with the identity
  shown for the admin to compare since it is not the suggested one (D391 §7). Here the core
  compares against the key set it made itself.
- **What a taken VPS can do** (D394 §2): it cannot change what a core accepts, but browsers and
  TVs do not check the chain, so it can show a false sign-in page or go dark. Whoever serves the
  JavaScript is trusted (protocol §3.7). Mitigations: the loader's service worker keeps files
  by hash, so a returning browser keeps its checked loader until an update; the core's
  read-back check (above) and a `discovery-watch` run from the core can tell the admin when the
  VPS serves files the core did not sign.

## 6. How clients find it: origins and CSP

- **A link names the discovery.** The core builds links on its `clientOrigin`, which `status`
  returns (D293 §4). An own discovery's links lead to its own client origin, so no client is
  configured by hand.
- **The origins.** D308 gives each core its own client origin on a separate registrable domain
  (`<coreId>.orpheum-client.com`), so that cores sharing one loader cannot read each other's
  storage. The loader takes the core id from its own hostname [repo: `client-from-core.md` §1].
  An own discovery has one core, so the cross-core reason is gone; what remains is keeping the
  core's client apart from the loader, which an origin boundary does on its own. Proposed: the
  client origin is `<coreId>.client.<household domain>` and the loader `app.<household
  domain>`, one certificate covering both names, with the same headers the paid service sends
  (no cookies on the client origin, `Origin-Agent-Cluster: ?1`, `COOP: same-origin`). The
  alternative, a second registrable domain, costs a second domain a year and a second DNS zone
  (§12 Q1).
- **CSP.** The loader's `connect-src` names the discovery's API and the core's zone; the paid
  service builds it from its configuration [repo: `orpheum-connect`]. An own discovery builds it
  from its address document, so the names in the CSP and the signed document cannot differ.
  `script-src 'self'`, SRI and Trusted Types stay as the protocol's §3.4 has them.
- **CORS** on the API answers the configured loader and client origins only, as the paid
  service does (protocol §3.4).
- **A device with no link** cannot locate the core, since paths stay behind link hashes (W4b's
  Q4). This is W4b's Q6 (§12 Q7). Without a realm (D298), a new browser cannot sign in by name
  either; on an own discovery every new device starts from a link or a code.

## 7. STUN, TURN and the household relay

- **STUN (W4b.3)** is a Binding-only server on the VPS, rate-limited per source and without
  `SOFTWARE`, on the `pion/stun` the module already requires (`go.mod`: `github.com/pion/stun/v3`)
  [repo]. The status answer's `iceServers` name it, and Watch Together's browsers use it beside
  the core's own (D406).
- **NAT measurement needs two addresses**, and a VPS usually has one IPv4 address (W4b's Q7).
  Proposed: accept the gap, as the paid tier deferred its own (D292 §3), and report the core's
  mapping as unknown rather than guessed.
- **The train report.** On a train's WiFi on 2026-11-13, the prod core's connections reached the
  train's address over punched IPv4, every congestion window collapsed to 1191 bytes, and
  YouTube still played, so TCP or QUIC on 443 worked where our UDP did not [repo:
  [idiosyncrasies.md](../../idiosyncrasies.md), the entry on pion's SCTP backing its resends off
  to 60 s]. The SCTP fix (VQ316) helps a starved UDP path; it cannot help a blocked one. The core
  has no ICE-TCP and the household has no relay. An own discovery with STUN alone would fail on
  that train exactly as the paid service did.
- **The ways to a TCP path on an own discovery:**
  - **TURN over TLS on the VPS's 443.** The core's TURN credentials already come from the
    discovery and last 15 minutes [repo: `internal/core/turn`]. pion/turn serves TCP and TLS
    listeners. The cost is port 443: HTTPS is there too, so it needs a second IPv4 address, a
    demultiplexer on the TLS handshake (RFC 7443 defines the `stun.turn` ALPN value; whether
    browsers send it is UNVERIFIED), or TURN on 5349, which strict networks may block.
  - **D438's relay on the same VPS**, the core dialling out to it (which works behind CGNAT),
    forwarding DTLS it cannot open, capped as D438 caps it. With a TCP or TLS listener on 443 for
    clients it covers the train case. D438 defers it until the beta's data asks; the train is the
    first such datum.
  - **ICE-TCP to the core directly.** pion supports passive ICE-TCP (`SettingEngine.SetICETCPMux`
    in `pion/webrtc/v4`) [repo: module cache], and Chrome and Firefox open active ICE-TCP
    connections to a server's passive candidates (UNVERIFIED: search summaries of a 2017 Mozilla
    post and a pion issue). It needs a reachable TCP port on the core, so it serves the
    households of §4.2, not CGNAT ones.
- **Proposed:** keep the own discovery's first release STUN-only, as D314 accepts a failing
  connection and counts it, and build the TCP path once for both services, from the answer to
  the household-relay research running beside this one (`research/household-relay-2026-11-13.md`,
  not in this tree when written). On an own discovery the relay's host is already there: the
  same VPS (§12 Q4).

## 8. Installs, upgrades and switching

- **One image, a mode.** `orpheum discovery` is a mode of `cmd/orpheum` as core and node are
  (W4b's shape), so the VPS runs the core's public image (D314) with a different command, and its
  compose file follows the core's contract: frozen when released, changed additively, versioned
  (D461), tested in a VM (D463).
- **Its settings** as a config file in the core's form (`orpheum default-config --discovery`)
  rather than flags (W4b's Q5): the core's config already has `[turn]` and `[webrtc]` tables, so
  a discovery's `[stun]`, `[tls]` and names read the same way.
- **Its certificate:** its own ACME client with TLS-ALPN-01 or HTTP-01 on its own names, so a
  person needs no reverse proxy; a flag for plain HTTP on loopback for those who already run one.
  `golang.org/x/crypto` is allowed for other uses [repo: `deps/go.allow`]; whether its `acme`
  package may be used is a dependency question for the owner (rule 3; §12 Q2).
- **Upgrades.** The discovery serves the loader from its own binary, and the core serves the
  client, so they upgrade independently within the protocol's window: the two previous majors
  served for six months (D273 §6). Proposed order in the guide: the discovery first, then the
  core. The loader's frozen endpoints (D425 §4) are what keep an old loader working against a
  new discovery.
- **Arm64 and small NAS boxes** matter for the core, not the discovery, which runs on a rented
  VPS. A beta tester's Synology DS218 (arm64, kernel 4.4, no official Docker; from the lead's notes,
  UNVERIFIED here) is a D463 install-route question.
- **Switching from the paid service to an own discovery:**
  1. Set up the VPS and the domain (§9), and take the enrolment key.
  2. In the core's `/system/discovery`, leave the paid service. Its wording already says
     "Clients outside the house will no longer find this server through it."
  3. Connect to the household's issuer, compare the identity shown, enter the key.
  4. The core pushes its signed files; the read-back check passes.
  5. Send everyone new links. The paid spec calls this "the true cost of leaving" [repo:
     `orpheum-connect/docs/spec.md` §3.10]: links lead to the old client origin. A device
     that already knows the core's direct addresses keeps working until they change.
- **And back:** the same key reclaims the old core id on the paid service [repo: spec §3.10],
  and the links must be sent again. Proposed: the core's page says so before the admin leaves
  either one.
- **Two discoveries at once** would remove the cost of switching, and D391 already lets a
  discovery have several locations. But a core pins one issuer; holding two would change D391.
  Proposed: not now (§12 Q6).

## 9. What a person needs, and their steps

**Needs:** a domain they control, with DNS they can edit; a VPS with a public IPv4 and IPv6
address, ports 80, 443 and 3478 open; a core already installed at home; a terminal.

**Steps, as the guide would give them:**

1. Rent the VPS and point three names at it: the API, `app.`, and the client origin.
2. On the VPS, write the compose file and `orpheum default-config --discovery` with the domain;
   start it. It takes its certificates and prints its enrolment key.
3. At home, in the core's admin page under Discovery, or by `orpheum pair --discovery
   https://<domain>`, give the issuer and the admin's password.
4. Compare the identity the page shows with the one the VPS printed; enter the key.
5. The core makes its key set, shows the recovery secret to save, and pushes the signed files.
   The page says the discovery serves them.
6. Open a link the core makes, on a phone away from home, and play.

That is the paid flow with steps 1–2 added. D334's one command at setup can take the issuer as
it takes the key.

## 10. Security and abuse

- **Open to the internet**, so the stranger-facing calls (`locate`, `paths`, `offer`, the
  loader's `/rtc/offer` and `/pairings`) need W4b.4's missing rate limits before release.
- **STUN reflection:** Binding only, per-source limits, no `SOFTWARE` (W4b.3), so the server is
  no amplifier.
- **An open relay:** a TURN or D438 relay on the VPS takes only credentials the core issued, as
  Tailscale's `--verify-clients` and Syncthing's `-token` restrict theirs (§2.1, §2.2), with
  D438's caps on bytes.
- **A taken VPS:** §5. It can phish and deny service, not change the trust a core holds.
- **A taken core** controls the household anyway; the recovery secret kept off the core is what
  lets the admin take the key set back (§5, D395).
- **Privacy:** the operator is the household, so the VPS's logs are theirs; Syncthing's warning
  that the discovery operator can "deduce which devices are connected to each other" applies to
  the VPS's hosting provider instead.
- **Port 80 and certificates:** HTTP-01 needs port 80 open; the server answers only the
  challenge and redirects the rest.
- **What the owner is not responsible for:** an own discovery depends on nothing of the owner's
  (D391 §8). Abuse reports about a household's VPS go to that household, not to the owner.

## 11. Build steps

The sizes are S (a day or less), M (a few days), L (a week or more). Each replaces or updates a
W4b step as marked; the plan itself is not edited here.

| Order | Step | Size | W4b |
| --- | --- | --- | --- |
| 1 | `discovery/discoverytest`, the conformance suite `single` and the paid service both run | M | the rest of W4b.7, unchanged |
| 2 | The client API's missing parts: the `lan` path and per-source rate limits on every stranger-facing call | M | the rest of W4b.4, unchanged |
| 3 | STUN, Binding only, on `pion/stun`, named in `iceServers` | S | W4b.3, unchanged |
| 4 | The core makes its software key set at setup, shows the recovery secret once, signs genesis and address document, pushes them by `PUT /core/v1/published`, and reads them back; `single` verifies and serves them | M | new, from D394 §2 |
| 5 | The loader served at the household's loader origin and client origin, from the binary, with the frozen endpoints, CORS and CSP built from the address document | M | replaces W4b.5: D308 moved the files to the core, so nothing is taken from it |
| 6 | `orpheum discovery`: the mode, `default-config --discovery`, its own ACME certificate or plain HTTP on loopback, the enrolment key, `rekey` | L | W4b.8, with Q5 answered as §8 proposes |
| 7 | Switching in the core's page: the wording before leaving either discovery, and new links | S | new |
| 8 | The guide, a versioned compose file, a VM test of the whole setup from a clean VPS image | M | new, from D461 and D463 |
| 9 | `through-discovery.spec.ts` against the real `orpheum discovery`, not the dev command | S | extends W4b.6 |
| 10 | The TCP path: TURN over TLS or D438's relay, on 443, with credentials from the core | L | new; waits on the household-relay research and the owner |
| 11 | A static mirror location for the signed files, pushed by the core | S | new, optional |
| 12 | Cast for an own discovery | depends | new; waits on §12 Q5 |

Steps 1–3 help the paid service too and can start at any time. Step 4 is the first that is the
own discovery's alone, and 5–6 are the ones the draft guide says are missing.

## 12. Questions for the owner

1. **The client origin's domain.** A subdomain of the household's domain
   (`<coreId>.client.<domain>`), or a second registrable domain as D308 gives the paid service?
   *Recommendation:* the subdomain. With one core the cross-core isolation D308 bought is moot,
   and the origin boundary still keeps the core's client apart from the loader.
2. **The certificate.** The discovery's own ACME client (on `golang.org/x/crypto/acme`, already
   in the module for other uses, so the owner's word on its use), or plain HTTP behind the
   person's own reverse proxy? *Recommendation:* its own ACME by default, plain HTTP on
   loopback as a flag (W4b's Q5).
3. **Where the recovery key lives.** On the core beside the everyday key, or shown once to the
   admin to save off the machine? *Recommendation:* shown once and kept off the core, since it
   is the key that recovers from a taken core (D395).
4. **The TCP path.** In the own discovery's first release, or after the paid service's answer?
   *Recommendation:* after, built once for both, but designed so the own discovery's VPS hosts
   it; the train report argues it should not wait long.
5. **Cast on an own discovery.** The Cast receiver is served at the loader's origin under an
   application ID registered to that URL (D467; Google requires registration for a custom
   receiver and HTTPS once published [source: Cast registration]; the fee is UNVERIFIED). Each
   household registers its own receiver, or an own discovery uses the owner's published
   receiver, which then depends on the owner's service (against D391 §8, but by choice as its
   published locations are), or an own discovery goes without Cast? *Recommendation:* the
   owner's receiver as an opt-in setting, Cast off by default.
6. **Two discoveries at once.** May a core be enrolled at the paid service and its own at the
   same time, so switching costs no links? *Recommendation:* not now; it changes D391's single
   issuer.
7. **A device with no link** (W4b's Q6): paths without a locator for code requests only, or the
   request over the signalling connection? *Recommendation:* over signalling, so paths stay
   behind hashes, with the device's address carried for the core's per-site limits.
8. **NAT measurement on one address** (W4b's Q7): accept that an own discovery reports the
   mapping as unknown? *Recommendation:* yes.
9. **The home-core shape.** Offer §4.2 at all? *Recommendation:* not as a supported shape in the
   first release: it fails CGNAT households silently and joins the loader to the core. Revisit
   if beta testers with public addresses ask.
10. **The static mirror.** Build step 11 at all? *Recommendation:* yes, later; it is small, and it
    is what keeps cores fetching the key set when a VPS goes dark (D393).

## Sources

All fetched 2026-11-13 and cached in `/yoloai/cache/http/self-hosted-discovery-2026-11-13/`.

- stdiscosrv: <https://docs.syncthing.net/users/stdiscosrv.html>
- strelaysrv: <https://docs.syncthing.net/users/strelaysrv.html>
- security: <https://docs.syncthing.net/users/security.html>
- Syncthing global discovery v3: <https://docs.syncthing.net/specs/globaldisco-v3.html>
- headscale README: <https://raw.githubusercontent.com/juanfont/headscale/main/README.md>
- headscale requirements: <https://headscale.net/stable/setup/requirements/>
- DERP servers: <https://tailscale.com/kb/1232/derp-servers>
- custom DERP: <https://tailscale.com/kb/1118/custom-derp-servers>
- NAT traversal: <https://tailscale.com/blog/how-nat-traversal-works>
- client-server API: <https://spec.matrix.org/latest/client-server-api/>
- server-server API: <https://spec.matrix.org/latest/server-server-api/>
- HA remote access: <https://www.home-assistant.io/docs/configuration/remote/>
- Nabu Casa: <https://www.nabucasa.com/config/remote/>
- Jellyfin networking: <https://jellyfin.org/docs/general/post-install/networking/>
- Plex remote access: <https://support.plex.tv/articles/200289506-remote-access/>
- Plex relay: <https://support.plex.tv/articles/216766168-accessing-a-server-through-relay/>
- Tunnel: <https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/>
- Cloudflare terms: <https://www.cloudflare.com/service-specific-terms-application-services/>
- WireGuard: <https://www.wireguard.com/quickstart/>
- DuckDNS: <https://www.duckdns.org/faqs.jsp>
- Let's Encrypt: <https://letsencrypt.org/docs/challenge-types/>
- RFC 6544: <https://www.rfc-editor.org/rfc/rfc6544.txt>
- Cast registration: <https://developers.google.com/cast/docs/registration>
- Search summaries only (UNVERIFIED), on browsers' ICE-TCP:
  <https://developer.mozilla.org/en-US/docs/Web/API/RTCIceCandidate/tcpType>,
  <https://github.com/pion/webrtc/issues/2125>

Earlier research this document leans on rather than repeats:
[self-hosting-ways-2026-10-09.md](self-hosting-ways-2026-10-09.md),
[reachability-approaches-2026-10-09.md](reachability-approaches-2026-10-09.md),
[homelab-networks-2026-10-10.md](homelab-networks-2026-10-10.md),
[relay-costs-2026-11-06.md](relay-costs-2026-11-06.md),
[root-key-audit-2026-10-31.md](root-key-audit-2026-10-31.md),
[domain-assumptions-audit-2026-10-31.md](domain-assumptions-audit-2026-10-31.md),
[client-from-core-2026-10-17.md](client-from-core-2026-10-17.md).

## Not tried

- **Nothing was run.** No `orpheum discovery` exists to run; no VPS was set up, no certificate
  taken, no STUN server measured. Every size in §11 is an estimate from reading.
- **No measurement of how often a household needs the TCP path.** The train is one report; the
  household-relay research and the beta's counts (D314 §4) are where the number comes from.
- **Browsers' ICE-TCP today.** Only search summaries of old posts were seen; Safari and the TV
  browsers were not checked at all.
- **Whether pion/turn can share 443 with HTTPS** by ALPN, and whether browsers send RFC 7443's
  `stun.turn` value.
- **Hosting providers' current prices and traffic allowances** beyond what the paid spec and
  relay-costs-2026-11-06.md give; whether a cheap VPS's sustained throughput carries relayed
  video.
- **The Google Cast developer console's terms and fee** for a household registering its own
  receiver.
- **Free dynamic DNS beyond DuckDNS**, and whether any of them serves IPv6 well.
- **The paid service's code** (`orpheum-connect`) beyond its specification: how its loader
  builds the CSP was taken from the spec's description, not its source.
- **headscale's and Syncthing's operational reports** from people running them: forums, issue
  trackers and outage histories were not read.
