> **ABOUTME:** Third-round audit (2026-10-13) of the discovery design as a whole, from the public
> side (the protocol, the core, the web client): whether its architecture fits what Orpheum is to
> become (closed beta, then public, many households, native apps, split devices, cast, offline),
> what still contradicts what, the shortest order of work to an end-to-end test, what is over- or
> under-built for a beta of a handful of households, and how a household fares when its discovery
> is down. The paid service's side is in that service's own repository.

# Discovery: coherence audit against the end goal (2026-10-13)

**Read:** [discovery-protocol.md](../discovery-protocol.md); plans
[discovery-core.md](../plans/discovery-core.md), [discovery-mode.md](../plans/discovery-mode.md),
[webrtc-transport.md](../plans/webrtc-transport.md), [release.md](../plans/release.md),
[remote-access.md](../plans/remote-access.md), [m5.md](../plans/m5.md) and
[cut-a.md](../plans/cut-a.md) (for M6 and what follows Cut A); D267, D273, D274, D275, D288–D291;
the eight audit reports of 2026-10-13 in this directory (`discovery-audit-*`, `discovery-audit2-*`);
the paid service's specification and plan (read for the seam only); `deploy/vps/`; and the code at
An earlier commit where a finding rests on it (`cmd/orpheum/https.go`, `internal/core/auth/pairing.go`,
`internal/core/backup/`, `internal/core/clientnet/`).

**Markers.** **[V]** checked at its source this session (pages cached under
`/yoloai/cache/discovery-audit3/coherence/`, or the second audit's cache
`/yoloai/cache/discovery-audit2/` and the first's `/yoloai/cache/discovery-audit/`, re-read for
the quote given), or in the code at the path named. **[I]** my inference or recommendation.
`file:line` is at an earlier commit. "Audit 2" means the five second-round reports
(`discovery-audit2-*-2026-10-13.md`); their finding ids (X1, T2, W1, U2-2, …) are used as they
wrote them.

## Summary

**The architecture fits the end goal.** An outbound-only core, names computed from their label
(Plex's `plex.direct` shape, run at Plex's scale for years), sign-in to the core rather than to the
service, an identity key, public building blocks under `discovery/`, and a protocol that changes
additively: none of these paints Orpheum into a corner for many households, native apps or arm64.
Signing in to the core rather than to a central account is the one choice that most separates
Orpheum from Plex, whose own local playback failed during its 2026-07-14 outage because even a
local address needs plex.tv to authenticate **[V]**.

**Three corners remain, each cheap to avoid now and dear later:**

1. **At home with the internet or the zone's DNS down, a browser cannot reach its core through
   the discovery's client at all** (§5.1). Every path the client knows is a name under the zone, and
   the core's own LAN origin is plain HTTP, which an HTTPS page may not call (mixed content
   **[V]**). D291 §6's "the last good LAN address kept for an outage" cannot work as written. The
   household needs a home address it knows, opened as a page of its own.
2. **The television and the Cast receiver of M6 are the hardest client on the hardest network.**
   For a household behind DS-Lite or carrier NAT (the owner's own), a TV on an IPv4-only network
   reaches the core only over WebRTC data channels plus a service worker in the TV's browser or the
   Cast receiver, which no one has shown to work there (§1.3). The owner's VPS, as a `configured`
   candidate with the owner's TURN, is that household's route, so it should stay, not be retired.
3. **Credentials keyed per core, and a guest's second device, must be in the first central client.**
   Audit 2's W1 (one token sent to every household) and the split-device goal (voice on a phone,
   film on a TV, D267 §3) both meet in W4c and the device-code path. Built TV-only and
   single-token, both would be rebuilt before the public launch (§1.2).

**Consistency:** nothing decided since D291 folds in audit 2, so its contradictions stand; §2 lists
them at their current lines, with eleven new ones.

**Order of work:** the plan's critical path builds some forty items before any two components meet
(§3). A walking skeleton (core, discovery, central client, one real zone) can be tested end to end
after about a third of them; §3.2 says what must be real and what can be stubbed.

**Over-built for a handful of households:** the claim-and-approve flow, re-key by recovery, CT
watching, the NAT measurement, device codes with HPKE, the version window. **Under-built:** the
home address, a separate staging discovery and zone, a way for a tester to hand the owner a
diagnosis, and testers' installation (§4).

## 1. Does the architecture fit the end goal?

### 1.1 What fits

| Goal | What the design does | Verdict |
| --- | --- | --- |
| Many households, then public | Names computed from the label, one wildcard per core, no record per address (§2.7, D290 §7); the core holds all state about its people | Fits. Nothing grows per address or per person at the discovery |
| No relay of ours | WebRTC through the discovery, IPv6, mapped IPv4; the owner's TURN or VPS for the rest (D273 §2, D274 §2) | Fits, with a measured gap (§1.4) |
| Sign-in stays at home | Household users sign in to the core (D289 §1) | Better than Plex: an outage of the discovery never refuses a password |
| Native apps | Same paths, pinning the identity key, mDNS, raw UDP (protocol §3.5) | Fits. One step keeps the offline case open (below) |
| arm64 | Pure Go: pion, the hand-written port mapping, netlink by `syscall` | Fits. K4's netlink is Linux-only, so a core on another OS keeps `net.InterfaceAddrs` as its fallback [I] |
| Exit from the paid tier | Same binary, own discovery, the core works on its LAN through a lapse (D25, D289 §5) | Fits, but the own discovery ships after the beta (§4.2) |

**For native apps, one cheap step keeps the offline case open [I].** A native app can trust the
core's identity key without DNS or the web's certificate authorities: if the core publishes a
statement, signed by its identity key, naming its current TLS public key, an app that pinned the
identity key can open TLS to the LAN address directly, with the internet down. This needs nothing
in the beta; it needs only that §2.1's list of what the key signs stays open to it (today the list
says "and nothing else").

### 1.2 Split devices and many households: two seams to set in W4c

- **Every credential keyed by `coreId`** (audit 2 W1). The client served at the central origin
  will meet more than one core: a person's home, a friend's Watch Together, later a second
  household (U8, after the beta by D291 §6). Today one token is kept and sent with every request
  (`web/apps/web/src/session.ts`, `web/packages/sdk/src/client.ts`) **[V, audit 2 W1]**. If W4c
  ships that shape, every tester's browser holds it, and the public launch must migrate it. The
  owner's stance is to break early; this is the moment.
- **A guest's second device.** D267 §3 lets one member be on several devices, but only a signed-in
  person's membership follows them; a guest's stays with the browser that joined (D267 §1), and a
  new browser needs a new link that cuts off the first (D267 §4). Voice on the phone and the film on
  the TV, for a guest, is exactly that case. The paid device code (K15) and the pairing QR are the
  mechanism that could admit the TV *from the phone*. K15 is written for an account's own TV only,
  and is "last, and allowed to slip" (discovery-core.md:428). [I] Keep K15's design open to "a
  member admits another device by its code" even if it ships late.

### 1.3 Cast and the remote TV (M6)

cut-a.md's M6 is "her TV plays, her phone talks". Through a discovery, her TV reaches a core behind
DS-Lite or carrier NAT only:

- over IPv6, when her network has it and the core's router lets the port in; or
- over WebRTC data channels, with the service worker that carries the player's requests
  (webrtc-transport.md W2b), inside a TV browser or the Cast receiver.

Google documents WebRTC on "the Google Nest Smart Display and Chromecast with Google TV devices",
for camera streams **[V, Google Home camera-stream trait]**; WebRTC data channels and service
workers inside a Cast Web Receiver are documented nowhere I found **[I]**. So the paid tier's
promise of "no relay" and M6's TV meet where neither is verified. Two consequences:

- **The owner's VPS should stay as a `configured` candidate** for the owner's own household, with
  its TURN (K13), rather than being retired when the discovery works (D274 §5 says the tunnel
  "becomes optional, and later goes"). The protocol already carries both (`configured`, `turn`);
  K4's configured candidates and K13 are small.
- **A device check belongs in M6's first phase**: on the owner's Chromecast, a receiver page opens
  an `RTCPeerConnection` with a data channel and registers a service worker. One afternoon, and it
  decides whether the Cast receiver can serve WebRTC-only households at all.

### 1.4 What comparable products show

| Product | Remote path | Relay | What it teaches |
| --- | --- | --- | --- |
| Plex | `app.plex.tv` serves the client; `plex.direct` names; port forward or UPnP | Plex Relay, "limited to 2 Mbps maximum for streams" **[V]**; remote video needs Plex Pass or a Remote Watch Pass **[V]** | The central client and computed names work at scale. Its 2026-07-14 outage stopped local playback, since "even accessing your Plex server through its local IP address requires authentication with Plex" **[V]** |
| Jellyfin | None of its own: port forward, reverse proxy, VPN, a VPS reverse proxy, Tailscale **[V]** | none | The gap Orpheum's discovery fills |
| Emby Connect | A credentials directory; remote access still needs the server "setup and tested to allow Internet access" **[V]** | none | A central account without reachability helps little |
| Tailscale | Direct, else a peer relay, else DERP **[V]** | DERP, peer relays | Even a NAT-traversal specialist keeps relays |
| Nabu Casa (Home Assistant Cloud) | Every remote connection through its SNI proxy, TLS ending on the home instance, the Let's Encrypt key held there **[V, SniTun's README; Nabu Casa's pages refused fetching, so their wording is from a search summary]** | all traffic, end-to-end encrypted | A relay that cannot read is accepted by a privacy-minded audience; it is low-bandwidth there |
| Synology QuickConnect | Direct, then hole punching, then a relay that "can cause a significant delay" with "limited bandwidth" **[V, a guide quoting Synology]** | yes, slow | The same three layers as D274, with a relay last |

**Every comparator built for non-technical households has a relay.** Orpheum's decision to run
none is defensible only because D274 §3 measures the dead end in the beta and D274 §2 keeps a
relay of failed connections open. The protocol's "no TURN, ever" (discovery-protocol.md:59)
contradicts that (audit 2 X6) and should go [I].

## 2. Consistency

### 2.1 Audit 2's contradictions, still open

No decision after D291 takes audit 2's design findings; its code findings were fixed
. These remain, at today's lines:

| Id | Where | Against |
| --- | --- | --- |
| X2 | discovery-protocol.md:536 ranks "IPv6 or IPv4" | D291 §3 (IPv6 first, demotion); webrtc-transport.md W2c has it right |
| X5, T2 | discovery-protocol.md:354-356, CAA `issuewild` only, `validationmethods=dns-01` | RFC 8659 §4.3 (`issuewild` ignored for a non-wildcard name); a persist record would be refused |
| F1 | discovery-protocol.md:341-346, DNS-PERSIST-01 naming the account URI | draft-02's hashed value; Let's Encrypt not deploying it |
| X6 | discovery-protocol.md:59 "paid: no TURN, ever" | D274 §2 |
| X8 | discovery-protocol.md:115-116 "no code for anyone to read out" | D291 §6 (typed fallback kept) |
| X9 | discovery-protocol.md:165 (5-minute `PUT /addresses`), :289 (hourly probe) | D291 §6 (heartbeat on the signalling connection, probe on change) |
| X10 | discovery-protocol.md:434-436 bootstrap token "single-use" yet usable "for its few minutes" | itself |
| X11 | discovery-protocol.md:106-107 `protocol` in `/enrol` | discovery-protocol.md:650-651 (no version in the body) |
| X12 | discovery-protocol.md:98-100 "origin" vs "base URL" | itself |
| X13 | discovery-protocol.md:420-424 `/register#<coreId>.<secret>`, `/join#…` | the built client's `/register#<secret>`, `/together#<secret>`; no marker |
| X7 | discovery-protocol.md:244 and :424, a pairing QR carrying a DTLS fingerprint | `rtcapi` makes a certificate per connection; D214 §1's key fingerprint |
| R1 | discovery-protocol.md:657-658, the window | D273 §6–§7 (previous two majors) |
| R4, U2-2 | discovery-protocol.md:68-69, :453-455 (Safari "without a visit"; home-screen apps "keep their own count") | WebKit: days of use without interaction; home-screen apps are isolated from Safari's storage **[V, audit 2]** |
| R3 | discovery-protocol.md:193-194, the Plex amplification "through a port UPnP opened" | NETSCOUT and Plex: DMZ and manual forwards |
| F2 | discovery-protocol.md:255, discovery-core.md:118, webrtc-transport.md W2e, "half-trickle" | RFC 8838 §2 |
| G3 | discovery-protocol.md:336-337 long TTLs | :376 and :398 "names stop answering" |
| G4 | discovery-core.md:126-128 unproven connections limited to signing in, registering, pairing | a guest's Watch Together join over WebRTC |
| G5 | discovery-protocol.md:501-503 "at home" when the source equals the core's public IPv4 | shared IPv4 under carrier NAT and DS-Lite |
| G9 | discovery-protocol.md:148-151, one live `instanceId` | no rule for when an instance stops being live (discovery-core.md:456-460) |
| G11 | discovery-core.md:389-440 | the paid plan's copy of the same section |
| G14 | D274 §2 ("a high port of the core's choosing"), D290 (title, §8, §9), D72, D273 §7 | later decisions with no banner on the earlier ones |

### 2.2 New contradictions

| # | Where | Against | Right |
| --- | --- | --- | --- |
| N1 | discovery-protocol.md:463-465: own device codes refused "past a hundred waiting, or five … from one network (an IPv4 address or an IPv6 /64)", "built, an earlier commit" | D288 §6's second amendment (a thousand; twenty per site, an IPv6 /48) and the code, `internal/core/auth/pairing.go:29-31` (`pairingsAtMost = 1000`, `clientnet.SiteOf`) **[V]** | D288 §6 as amended again |
| N2 | discovery-protocol.md:138 and :468 key enrolment and paid codes on "an IPv6 /64" | D288 §6 keys codes on a /48 "which a provider gives one customer"; `clientnet` has both `Of` (/64) and `SiteOf` (/48) | [I] one rule: limits on strangers key on the site (/48); the network (/64) stays for "at home" |
| N3 | discovery-protocol.md:742-743 (§7 item 18) and webrtc-transport.md W2c item 7: "the last good LAN address kept for an outage" | Mixed content: an HTTPS page's request to `http://192.168.…` is blocked **[V, W3C Mixed Content]**, and the LAN *name* needs the zone's DNS | §5.1 |
| N4 | discovery-protocol.md:192-193 "maps the core's HTTPS port over TCP only" | discovery-core.md:161-162 and D291 §7 (outside port 443 first, else the port granted) | D291 §7 |
| N5 | discovery-core.md:35, the C1 row: "`internal/core/backup` holds `secret.key` alone today (K8)" | discovery-core.md:375-378 ("Since fixed") and an earlier commit (every private key left out unless asked) | the code |
| N6 | remote-access.md:6 ("the route is a VPS"), :36-53 (our own `relay` role, "Build tunnel mode"), :68-72 ("no free tier, no data, no accounts") | D273 §2, D274 §2 (no tunnel through the service; a relay only as TURN, on demand), D289 §1 (accounts for payers) | the later decisions; remote-access.md needs a banner pointing to the protocol |
| N7 | cut-a.md:108, the Cast receiver at `https://media.<domain>/cast/` | the paid spec's §3.11, one receiver at a fixed `app.` URL for many households, "not for the beta" | open; it decides where M6's receiver lives (§1.3) |
| N8 | discovery-mode.md:13-15, D274 §5: the own discovery "claims its one core by the paid tier's flow" | D290 §1: the paid flow is now a token opened from the core | D274 §5's reason no longer holds; the own discovery's code stands on its own (protocol §2.2 item 4) |
| N9 | discovery-protocol.md:91 "It signs … and nothing else" | §1.1's native-app statement, and §2.8's re-key needing the new key's ACME account named (audit 2 G2) | [I] list purposes as a closed set that grows by decision, not "nothing else" |
| N10 | discovery-protocol.md:63-69 "once a client has reached its core, the discovery can go down and playing goes on" | the WebRTC path needs signalling for every new connection, and W2d's ICE restart goes "through the discovery" (webrtc-transport.md:115-116) | true only for HTTPS paths and connections already open; say so |
| N11 | discovery-core.md:437-440 (owner's TURN "not needed for the beta") | §1.3: the owner's own household on DS-Lite needs it for IPv4-only devices once the tunnel goes | keep the VPS route until §1.3's check |

## 3. The order of work

### 3.1 What the plan does

The critical path (discovery-core.md:435-437) is K1 → K2 → K3 (with K7, K8b) → W4b.4 → W2e, W2d,
W2f → W2c → W4c → the service's P6, P7, P10 → staging → the production NAS → an outside household → the
invited ones, with lanes C, D and E beside it. Counting the plan's own steps: sixteen K-steps, six
W4b steps, four W2 steps, W4c, W5, and thirteen service parts. Nothing runs end to end until W4c,
which comes after almost all of Lane B; the first real network test is "end to end on the owner's
staging core", after everything.

The owner's rule is *nothing public until all components are tested together*. That is well served
by bringing all components together early, thin, and thickening them, rather than finishing each
before they meet. The riskiest unknowns are all at the joins: WebRTC through a discovery in real
browsers on real networks, cross-origin byte paths, computed DNS at the chosen provider, the
service worker on the central origin.

### 3.2 A walking skeleton first [I]

**Skeleton:** the staging core, a discovery, the central client, one real zone (a staging domain),
Let's Encrypt's staging CA, a browser on a phone off the home network. It reaches the core over
IPv6 when it can and over WebRTC when it cannot, signs in, and plays.

| Piece | Real in the skeleton | Stubbed or deferred |
| --- | --- | --- |
| Wire (K1, W4b.7) | all of it: it is types, and the owner breaks freely now | — |
| Sessions (K2) | `expiresIn`, renewal on a 401, jittered backoff | `instanceId`: the newest session wins and the old socket is closed, rather than a refusal with no liveness rule (G9) |
| Signalling (K3) | locator checked by the core, signed fingerprint, concurrent offers, ping, idle close | two kinds of connection (S5 item 2); ICE restart answered (a failed connection starts a new race instead) |
| Links and locators (K7) | locate hashes, device locators, a person's own link | the admin's sign-in link |
| Addresses (K4) | stable IPv6, the non-public ranges, `configured` | netlink's change events: polling stays |
| Port mapping (K5) | as built, TCP only | the C9 list (epochs, ANNOUNCE, PCP nonce); FRITZ!Box VQ |
| Certificates (K9) | DNS-01 through the discovery as built; renewal at two-thirds | ARI, DNS-PERSIST-01 (not deployed at Let's Encrypt **[V, audit 2 F1]**) |
| Cross-origin (K10) | CORS, byte paths by header, `Host` allow list | — |
| Identity (K8) | K8a fingerprint, K8b first-contact signature | rotation (K8c), restore guidance (K8d), CT watching (K8e) |
| Claiming (K6) | the typed claim code, bound by whoever runs the discovery | the token, the approval, *This isn't me* |
| Client (W2c–W2f, W4c) | early description (W2e), fingerprint check (W2f), keep what connects and replace what fails (W2d), the race ranked by kind (W2c), per-core credentials (§1.2) | ICE restart, the network-change triggers, the in-app-browser hint |
| STUN | any public STUN server for the skeleton | the discovery's own, and the NAT measurement (K14) |
| Device codes (K15) | — | TVs wait (as the plan already allows) |
| Versions (K11), System page (K12), TURN (K13), counting (K16) | the deprecation header as built | the rest |

Then in order: (1) K1 + W4b.7; (2) the core's skeleton steps and `single`'s client API (W4b.3,
W4b.4) beside each other; (3) the browser-tier test through `single` (W4b.6), which is the first
time all public parts meet and runs in CI forever after; (4) the web client's skeleton steps;
(5) the same test against the paid service's handlers; (6) the staging deployment of §5.3 with the
owner's phone on mobile data and a friend's network; (7) the production NAS joins it while keeping the VPS
route; (8) only then the thickening, in the order the skeleton's failures show.

**What must be real from the first day:** the zone and its DNS provider's computed answers (the
one unknown no fake can settle: whether one script record answers a wildcard subtree, audit 2 G1),
a real browser on a real carrier network, and the central origin's service worker. Everything else
can be faked in tests first.

### 3.3 Outside the discovery, and on the beta's critical path

- **A tester's installation.** R2 keeps the tunnel required (release.md:44), and testers cannot yet
  obtain the image (beta-inventory (d) 2–3). An outside household cannot join without both.
- **The owner's own core off the service's zone** (audit 2's I1, W3): the owner's core holds a
  token that edits every record of `orpheum-app.com`. That must end before `app.` or `api.` exist
  there.

## 4. Over- and under-built for a handful of households (public side)

### 4.1 Over-built for the beta [I]

- **Claiming from the core with approval** (K6). For households the owner invites and knows, the
  own discovery's typed code, read to the owner over a channel they already share, protects as well
  (S7's attacker is a stranger). The paid flow can follow public sign-up.
- **Re-key by recovery after 72 hours, with mail**, and the restore guidance (K8c, K8d). With keys
  out of every backup since an earlier commit, a restore loses the key, and audit 2 U2-3 shows the 72 hours
  become a household outage. For the beta the owner re-keys by hand after a call.
- **Certificate Transparency watching** (K8e), **the NAT measurement** (K14), **ARI and
  DNS-PERSIST-01** (K9), **the version window** (§5): useful later, invisible to five households. The
  owner's stance (break early) argues for calling the beta's protocol unstable and promising the
  window from the public launch.
- **HPKE and the device's place for codes** (K15): already allowed to slip.

### 4.2 Under-built [I]

- **A home address that works without the discovery** (§5.1).
- **A diagnosis a tester can send.** D73 keeps household facts on the core, so the owner sees
  nothing of a tester's failure. A "copy what happened" on the dead-end page (the race's results per
  path, ICE's candidate pairs and state, the network's kind) is what one person needs to debug five
  households by message. It is also what the memory note "digging means missing logs" asks.
- **A separate staging discovery and zone** (§5.3).
- **The self-hoster's route in the beta.** The owner's brief of 2026-10-09 put "a user sets up
  their own VPS" among the beta's needs; the plans now ship `orpheum discovery` after the beta
  (discovery-core.md:438-439). The existing VPS tunnel (`deploy/vps/`) is that route today, if it is
  documented for testers.

## 5. When the discovery is down

### 5.1 At home

What a household member actually has is `app.<service>` (or the own discovery's domain), and paths
that are names under the zone. So:

| Down | At home, a browser on the central client | The core's own client |
| --- | --- | --- |
| The discovery's API | works on a remembered LAN or IPv6 name (if the device remembered paths, and Safari has not wiped them) | works |
| The zone's DNS, or the house's internet | **nothing**: every name fails; `http://<LAN address>` from the HTTPS page is mixed content **[V]** | works at `http://<LAN address>:<port>`, an insecure context (no service worker, no microphone) |

The core serves its own client on `server.listen` (plain HTTP) and on `https.listen` once it holds
a certificate (`cmd/orpheum/https.go:92-115`) **[V]**. So the core *is* usable at home; nobody is
told where. [I] Fix, small and before the beta:

1. The System page and each person's account page show the **home address**, as a link and a QR,
   to bookmark.
2. The client's no-core and dead-end pages, when the device seems to be at home, offer it as a link
   that leaves the central origin (a top-level navigation is not mixed content).
3. D291 §6's "last good LAN address kept for an outage" becomes that link, not a path the client
   races.
4. Later, native apps connect to the LAN address by the pinned identity key (§1.1).

### 5.2 Away

Remembered HTTPS paths ride out an API outage while the zone answers (protocol §1). WebRTC-only
households lose every new connection; a connection already open goes on until it fails, and its
recovery (W2d) needs signalling too (N10). Safari tabs need the discovery after seven days of use
without interaction (audit 2 R4). These are as the protocol says, once N10 is worded.

### 5.3 Staging in the sandbox, as the owner asks [I]

The owner wants the live access points to go to a staging discovery inside this sandbox for now.
From the public side:

- **Use a staging domain for the zone and staging names for the client**, never the production
  `app.` origin: a service worker a staging build registers at the production origin stays in
  every tester's browser after the switch, until a kill switch (C16) removes it.
- **Let's Encrypt's staging CA** until the zone's computed answers are proven, so the weekly limit
  of the real zone is never spent on tests.
- **The staging core and the staging discovery in separate containers**, the discovery's secrets
  not readable by the core's: staging is internet-facing.
- **No outside household's core enrols with it.** The sandbox runs agents with every permission;
  it is fit for the owner's own cores only.
- The paid side of this (tunnel, STUN port, DNS account) is in the paid service's report.

## 6. Decisions for the owner

1. **Walking skeleton first** (§3.2): build the thin end-to-end path before thickening any step.
   *Recommend yes.*
2. **Per-core credentials and a guest's second device in W4c and K15's design** (§1.2). *Recommend
   yes, now.*
3. **A home address** shown and offered, replacing "the last good LAN address kept" (§5.1).
   *Recommend yes, before the beta.*
4. **Keep the owner's VPS as a `configured` candidate with its TURN** for the owner's household,
   and check WebRTC and service workers on the Chromecast before M6 relies on them (§1.3).
   *Recommend yes.*
5. **Claiming in the beta by the typed code**, the approval flow after (§4.1). *Recommend yes.*
6. **Defer** K8c–K8e, K14, ARI, DNS-PERSIST-01 and the version window's promise until the public
   launch; the beta's protocol called unstable. *Recommend yes.*
7. **Instances: the newest wins** in the beta, rather than a refusal with no liveness rule (G9).
   *Recommend yes.*
8. **Fold audit 2's design findings and §2.2 into the protocol in one revision** (a decision taking
   them, then one pass over protocol and plans). *Recommend yes, before K1, since K1 fixes the wire.*
9. **Drop "no TURN, ever"** for "no relay in the beta; a relay of failed connections waits on the
   beta's count" (D274 §2). *Recommend yes.*
10. **Document the VPS tunnel as the self-hoster's route in the beta**, `orpheum discovery` after
    (§4.2). *Recommend yes.*

## Sources

Cached under `/yoloai/cache/discovery-audit3/coherence/` unless marked.

- Plex: "Accessing a Server through Relay", https://support.plex.tv/articles/216766168-accessing-a-server-through-relay/
  (`/yoloai/cache/discovery-audit/ux-plex-relay.txt`); "Requirements for Remote Playback of Personal
  Media", https://support.plex.tv/articles/requirements-for-remote-playback-of-personal-media/
  (`plex-remote-req.txt`); How-To Geek, "Your Plex server is inaccessible right now, even on your
  home network", 2026-07-14 (`/yoloai/cache/discovery-audit/ux-htg-plex-down.txt`).
- Jellyfin networking, https://jellyfin.org/docs/general/post-install/networking/
  (`jellyfin-networking.txt`).
- Emby Connect (`/yoloai/cache/discovery-audit/emby-connect-doc.md`).
- Tailscale connection types, https://tailscale.com/kb/1257/connection-types
  (`/yoloai/cache/discovery-audit/tailscale-connection-types.txt`).
- SniTun README, https://github.com/NabuCasa/snitun (`/yoloai/cache/discovery-audit/snitun-readme.md`);
  Nabu Casa's "Remote access – Security aspects" and "Deep dive" refused fetching (HTTP 403); their
  content is taken from a search summary only.
- Synology QuickConnect, as quoted by WunderTech
  (`/yoloai/cache/discovery-audit/ux-wundertech-qc.txt`).
- Google Home, camera-stream trait, https://developers.home.google.com/cloud-to-cloud/traits/camerastream
  (`google-camerastream.txt`).
- W3C Mixed Content, https://w3c.github.io/webappsec-mixed-content/ (`mixed-content.txt`).
- RFC 6888 (`rfc6888.txt`).
- Code at an earlier commit: `cmd/orpheum/https.go`, `internal/core/auth/pairing.go`,
  `internal/core/clientnet/doc.go`, `deploy/vps/orpheum.nft`.
