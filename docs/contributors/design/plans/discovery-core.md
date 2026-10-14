> **ABOUTME:** The core's side of the discovery protocol (D273): its identity key, enrolling with a
> discovery (the paid service at app.orpheum-app.com or the owner's own), keeping its addresses
> current, and answering WebRTC offers the discovery passes on. The phases built, the steps that
> bring the core to the protocol as decided after three rounds of audits (D290, D291, D292) and the
> questions they left (D293), and the order of work: the security floor, then a thin end-to-end
> path, then the rest.

# Discovery: the core's side

- **Status:** IN-PROGRESS — begun 2026-10-09 on the owner's word (*"Do the core side of the
  discovery protocol"*); phases C1–C5 are built. Steps K1–K18 below, written 2026-10-13 from
  [discovery-protocol.md](../discovery-protocol.md) and revised the same day for D292 and D293,
  are planned and not started: the documentation pass D292 asked for comes first, then
  [Phase 1](#phase-1--the-security-floor).
- **Depends on:** D273, D274, D275, D288, D289, D290, D291, D292, D293, D214 §4;
  [discovery-protocol.md](../discovery-protocol.md), the protocol both discoveries speak;
  [discovery-mode.md](discovery-mode.md) (`discovery/single`, the reference every core test runs
  against); [webrtc-transport.md](webrtc-transport.md) (the client's side of the same paths).
- **Blocks:** the paid service (`orpheum-connect`, renamed from `orpheum-discovery`, D292 §8), whose
  plans are kept in its own repository: it imports the wire types K1 settles (D275 §1), so it cannot
  be finished correctly before them. Its `docs/plans/beta-bring-up.md` holds the joint order across
  both repositories, which [The order of work](#the-order-of-work) below gives Orpheum's part of
  (G11: kept in one place each, not copied).

## The wire

The protocol is [discovery-protocol.md](../discovery-protocol.md): §2 between the core and its
discovery, §3 between a client and the discovery, §4 the calls, §5 versions. This plan does not
repeat it. Its types are `discovery/discoveryproto`'s, a public package the closed service imports
(D275 §1); what is signed is `discovery/proof`'s. Where the built wire differs, the protocol says
**the code must change**, and the steps below say where.

**Names in this plan.** Phases C1–C5 keep the names they were built under. The correctness audit's
findings are also called C1–C16, so the remaining steps are named K1–K18, and a finding is always
written as the protocol writes it, after the rule it answers (for example "(C11)"). The second
round's code audit named its findings K1–K20 too; they are written **code K13** and so on, never
bare (D292 §4's "K13–K15" are code K13–K15, `single`'s takeover flaws).

## Phases built

| Phase | What | State |
| --- | --- | --- |
| C1 | The identity key: Ed25519, made on first start, `identity.key` in the persistent directory beside `secret.key`, mode 0600 | BUILT 2026-10-09. Since an earlier commit every private key (`secret.key`, `identity.key`, the TLS and ACME account keys) is left out of a backup unless asked for (N5, X1, T1). D292 §4 then revised D289 §2: scheduled backups hold the identity key sealed under a recovery code made at claiming (K8a) |
| C2 | `discovery/discoveryproto` and `internal/core/discovery`: connect (challenge, enrol, claim code), poll the status until claimed, sessions, the heartbeat every 5 minutes with the core's candidates; `discovery.json` in the persistent directory; `GET/POST/DELETE /system/discovery` for an admin; the System page's Discovery section | BUILT 2026-10-09; its candidates are the machine's interface addresses (`cmd/orpheum/discovery.go`) and the mapped ones (C5); `configured` candidates and TURN are not sent |
| C3 | The signalling connection: offers answered by `rtcapi`, with no check of their own; reconnects | BUILT 2026-10-09; one offer at a time, a flat 30 s between dials, no ping. Every offer is answered unchecked, so no core may hold a signalling connection until K3a (D292 §4; A2) |
| C4 | Certificates through the discovery: a wildcard `*.<coreId>.<zone>` by DNS-01, the discovery setting the TXT record (`PUT/DELETE /acme-challenge`), kept as `discovery-tls-*.pem`, served for every name under it | BUILT 2026-10-09; renews 30 days before expiry, waits a fixed 30 s for the record, serves it for any server name (A10) |
| C5 | Port mapping: PCP → NAT-PMP → UPnP IGD for IPv4, UPnP IGD 2 pinholes for IPv6 (`internal/core/portmap`), `should_map_ports` on by default, only while the discovery publishes the core | BUILT 2026-10-10, on the standard library alone. TCP only already; asks for the core's own HTTPS port as the external one |

## The remaining steps

Each step names what it changes, the decision and the protocol section it builds, the tests that go
red when it is reverted (AGENTS.md rule 4), any `VQ` item a device alone can confirm, and what it
waits on. Tests are described, not named: a name is given when the test is written. Every test runs
against `discovery/single` or a fake discovery, a fake router, the fake CA
(`certificate/certificatetest`) and `clocktest`; none touches the network (rule 16). **Every
behaviour is turned red by an in-process package test**; a browser or netlab test confirms the
whole but is never the only test that sees it, so revert-red settles a discovery file in seconds,
not minutes (devloop §2, D292 §9).

Which phase each step belongs to is [The order of work](#the-order-of-work). Steps marked
**deferred** wait for after the beta (D292 §3); K15, deferred by D292 §3, is in the beta again
(D293 §2).

### K1 — The wire, revised in the public packages

**Partly built 2026-10-13 (Phase 1's A3, A7):** `instanceId` and the refusals' codes; signatures
bound to the discovery and the core, `proof.Answer` and the answer's `signature`;
`discovery/link`'s fragment and locate hash; `clientOrigin`; the identity key's fingerprint
(`proof.Fingerprint`). **Then (Phase 2):** the version signal (`major.minor` in the header alone,
`discoveryproto.Serves`; no body carries a version); `expiresIn`; `discoveryproto.IsPublic` and
`AddressLabel` with their vectors; `DELETE /acme-challenge/{name}`; the probe's fields and
`reachableFrom`'s values. The rest waits with what uses it: the signalling's several offers, proofs
and heartbeat with K3; locators, locate hashes and the client API with K7 and W4b.4; the ACME
account URI and CAA with K9.

- **Changes.** `discovery/discoveryproto`: `SessionRequest` gains `instanceId`; `SessionResponse`
  carries `expiresIn` (seconds) in place of `expiresAt`; the body's `protocol` is dropped from the
  heartbeat and from enrolment (§2.2, §2.4; X11), and the header carries `major.minor` (§5);
  `ProbeResult` becomes fields (`address`, `port`, `kind`, `result`) and `reachableFrom` takes the
  values named by what was tested (§2.6); `SignalMessage` carries several offers, each with its
  `locatorHash` and `proof`, and the answer its `signature` over the exact SDP (§2.5; A9), and the
  heartbeat (§2.4); a refusal the core must act on carries an error code of its own (enrol again,
  another instance running; §4); the TXT record is cleared by `DELETE /acme-challenge/{name}`
  (§2.7); new types for `PUT /locate-hashes` and `PUT /locators` (§3.1), the core's ACME account URI
  (§2.7), and the client API (`locate`, `paths` with its `state` and `clientPrefix`, `offer`, the
  dead-end report; §3.2, §4; D293 §6–§8); `GET /status`'s `clientOrigin` (§3.4; D293 §4); the
  heartbeat answer's count of failed connections (§2.6; D293 §7). The paid device code's claim and
  its sealed outcome come with K15 (D293 §2). The claim token and its approval, the holding account
  and the re-key wait with what uses them (after the beta, deferred).
  One function encodes an address as a name label, with test vectors (§2.7, C15). One function
  classes an address as public or not, counting `100.64.0.0/10`, `192.0.0.0/29` and the
  documentation ranges as not public, for publishing and for the probe's deny list alike (§2.4, C7,
  C9, G12). One function derives a link's locate-hash, `SHA-256("orpheum-locate" ‖ secret)`, and one
  reads and writes a link's fragment, `<coreId>.<secret>.<fingerprint>` (§3.1; D292 §4) [I]. The
  zone's CAA records, at the apex and a claimed core's name, as test vectors (§2.7; T2) [I].
- `discovery/proof`: `Message` binds the discovery and the core,
  `orpheum-discovery-v1:<purpose>:<discovery base URL>:<coreId or public key>:<nonce>` (§2.1, C8,
  X12), with new purposes for a client's first-contact challenge and the answer SDP; the re-key's
  waits (deferred). `internal/core/identity.Key` signs through it.
- `discovery/single` and `internal/core/discovery` follow in the same change, since nothing is
  deployed and v1 changes in place (§2.1, D290 §13); single's own share is
  [discovery-mode.md](discovery-mode.md) W4b.7.
- **Builds** §2.1, §2.4, §2.6, §2.7, §4, §5; D290 §13; findings C8, C13, C15, C16, X11, X12.
- **Tests.** A signature made for one discovery fails at another, and one made for one core fails
  for another. The name encoding's vectors: one address has one name, no label holds `--`, IPv6 in
  eight groups without compression. The address classes: a carrier-NAT, a DS-Lite and a
  documentation address are never public. A link's fragment round-trips and one without a
  fingerprint is refused. Each new message decodes what the protocol's examples show. A test that a
  discovery accepts any minor of its major and refuses another major with the versions it accepts.
- **Waits on:** nothing. It is the root of every other step, of W4b and of the paid service.

### K2 — Sessions, retries and instances

**Built 2026-10-13 (Phase 2):** one renewal at a time for every caller; renewal at once, once, on a
401; retries doubling from `[discovery] retry_first_milliseconds` to `retry_most_milliseconds`,
jittered (`internal/platform/backoff`), the development stack's capped at 1 s; the signalling
connection redialled after the first wait; `Connect` serialised; a copy's refusal said and waited
on however it came. The instance's ping and the named refusals were built with A3.

- **Changes.** `internal/core/discovery/discovery.go`: the session's expiry counted from
  `expiresIn` on the monotonic clock (C11); one renewal at a time, shared by the step loop, the
  signalling loop and the certificate job (`SetTXT`), renewing five minutes before expiry and at
  once, once, on any 401, then backing off (C11); every retry backing off exponentially with
  jitter, in place of `retryAfter`'s flat 30 s, **the signalling connection's reopening starting
  near 1 s** (D292 §9; devloop §6), every interval a config value with a production default, so the
  development stack caps it at 1 s and every core is back within about 2 s of a restart of the
  discovery; an `instanceId` made at each start, kept in memory, never backed up (§2.3, C4);
  answering the discovery's ping on the signalling connection, by which the discovery tells a live
  instance from a dead one when a second asks for a session (§2.3; D293 §1); acting on the named
  refusals: *enrol again* re-enrols and shows the new code, *another instance* is shown in the log
  and on the System page as another copy and retried every few minutes, not as an ordinary failure
  (§2.2, §2.3, C12, C4; D293 §1); `Connect` serialised, so a double click enrols once (§2.2, C12).
  `discovery/single` pings the old instance and waits about 10 seconds before replacing it
  ([discovery-mode.md](discovery-mode.md) W4b.7).
- **Builds** §2, §2.2, §2.3; D290 §18; D292 §9; D293 §1; findings C4, C7, C11, C12.
- **Tests.** After a 401 the next call carries a renewed token without waiting 30 s. Three callers
  asking at once open one session. A core whose wall clock jumps a day neither renews on every step
  nor sends an expired token. A dropped signalling connection is redialled within about a second the
  first time, and two cores' later waits grow and differ (the jitter's random source given by the
  test). Two `Connect`s at once enrol once. An *enrol again* refusal re-enrols and shows a new code.
  An *another instance* refusal shows on the status and is retried after a few minutes, not at
  once. Against `single`: a core restarted after a crash, its old instance silent, is accepted
  within seconds; a second copy while the first answers the ping is refused, and the first keeps its
  connection.
- **Waits on:** K1.

### K3 — The signalling connection, checked and bounded

In two parts: **K3a** is the security floor, without which no core holds a signalling connection to
any discovery (D292 §4; A2, A9); **K3b** is what makes the connection keep up.

**Built 2026-10-13 (Phase 1's A2):** `rtcapi.Server.AnswerRelayed`, the only way an offer through
`internal/core/discovery` reaches `rtcapi` (its `Answerer` names no other), checks the offer first:
exactly one sha-256 fingerprint, at most `MaxRelayedCandidates` (10) candidates, mDNS candidates
dropped; its connections check no remote address that is loopback, link-local, private,
carrier-grade NAT's or in the core's own interface prefixes. Every connection: SCTP's receive buffer
1 MiB and messages at most `MaxMessageBytes`; 16 requests in flight; a request's head and body whole
within 30 s; closed unless ICE connects within 10 s; 16 connections per network (an IPv4 address or
an IPv6 /56) once connected. The check budget is the candidate cap times pion's seven binding
requests a pair. **Built 2026-10-14 (Phase 2):** the proof, checked under the key auth keeps for
the offer's hash (a session's by its locator, a link's by its locate hash; `auth.OfferKey`), every
offer refused before `rtcapi` sees it when it does not hold; pion compares the DTLS certificate with
the one fingerprint the proof covers. **Still K3a's:** unproven connections, with the proof's kinds;
the head stripped, with A1; per-locator caps, with locators (K7); the `TestArch_` claim, which
needs the owner's gate (`internal/archtest`), in its place a review invariant in the package's
`doc.go`.

- **K3a changes.** `internal/core/discovery/signal.go`: each offer checked by the core before
  `rtcapi` sees it (§2.5 item 3): its `proof`, an HMAC over the offer's DTLS fingerprint keyed by
  `SHA-256(token)` of the signed-in session (the hash the core holds, A corrections 5) or, for a
  link's bootstrap, by the value the core stored when it made the link (K7) (A9, T7); exactly one
  `a=fingerprint`, sha-256; at most about ten candidates. `internal/core/rtcapi`:
  `SettingEngine.SetRemoteIPFilter` refusing loopback, link-local, private ranges and the core's own
  LAN prefixes for offers through a discovery, and a budget of connectivity checks; after DTLS
  connects, the remote certificate's hash compared with the value the proof covered (A9);
  `SetSCTPMaxReceiveBufferSize` and an application message cap; the data channels and requests in
  flight per connection capped; a deadline on a request's head and body (A2, code K16); per-source
  and per-locator connection caps on the core; **unproven connections** short-lived (about 60 s
  without a sign-in), few (about 4), evicted first, and limited to signing in, registering, pairing,
  joining Watch Together and device codes (§2.5; D291 §6, S5 item 2; G4); **the request head
  untrusted**, `Origin`, `Host`, `Forwarded` and `X-Forwarded-*` stripped before the handler (A1);
  each answer signed with the identity key over its exact SDP, with the `offerId` and the
  client's offer fingerprint (§2.5 item 4; D292 §4; A7, A9).
- **K3b changes.** A ping every 25–30 s, two missed pongs ending the connection, reopened on K2's
  backoff (§2.5, C11); the heartbeat carried on the connection, `PUT /addresses` its fallback (§2.4,
  D291 §6); offers answered concurrently under `rtcapi`'s limit, several in one message (§2.5, C6,
  S5); the connection closed and its tokens dropped on leaving, revocation and disabling (§2.5,
  C4); **early description**, the answer sent once the host candidates and the first
  server-reflexive one are in, or after about a second, in place of `gatherTimeout`'s wait for
  gathering to complete (§2.5, C6, F2); **a slot counted once ICE connects**, an unconnected offer
  bounded to about 10 s, the cap sized by memory rather than `MaxConnections`' 64, and refusals
  counted for the System page (§2.5, C6); **idle connections closed** when they carried no request
  with a valid sign-in for a few minutes, which needs the API's sign-in check to tell `rtcapi` per
  request whether it passed [I] (§2.5, D290 §6, S5). Later, in the thickening: answering an **ICE
  restart** offer on an existing connection (§2.5, C6; question 7); until then a failed connection
  starts a new race (coherence §3.2).
- **Builds** §2.4, §2.5; D290 §6; D291 §6 (§7 items 3, 11, 22); D292 §4; findings A1, A2, A9, S5,
  S6, S16, C4, C6, C11, G4.
- **Tests.** K3a: an offer without a valid proof is refused though the discovery passed it; one with
  two `a=fingerprint` lines, or one sha-1, is refused; a candidate at a LAN, loopback or link-local
  address is never checked; an eleventh candidate refuses the offer; a connection whose DTLS
  certificate does not match the proven fingerprint is closed; a request head naming another
  `Origin` reaches the handler without it; an unproven connection's request to the library is
  refused while a sign-in and a Watch Together join on it are answered; an unproven connection is
  closed after its minute; a half-sent body is dropped at its deadline; the answer's signature
  verifies over its exact SDP with the core's public key; a `TestArch_` claim that an offer through
  `internal/core/discovery` reaches `rtcapi` only after the core's own check, cited in the
  package's `doc.go`. K3b: a slow gathering does not hold the next offer's answer; a connection
  whose pongs stop is reopened; after leaving, the signalling connection is closed; an offer that
  never connects frees its slot after about 10 s; a connection idle past the limit is closed while
  one carrying signed-in requests stays; an answer leaves before gathering completes once a
  server-reflexive candidate is in (`rtcapitest` with a STUN server in the test); sign-in limits key
  on the ICE pair's address over WebRTC as over TCP (§7 item 9, S14). The ICE matrix between pion
  peers runs through pion's virtual network in-process (`pion/transport/v5`, direct by D292 §9),
  through a seam that hands `rtcapi` its `Net` (devloop §4.3).
- **VQ:** none; W5 tests the client's side in a browser.
- **Waits on:** K1, K2; K7 for the link's proof value; K8b for the signing.

### K4 — Addresses: stable, public and the admin's own

- **Changes.** The interface candidates move out of `cmd/orpheum/discovery.go` into
  `internal/core/discovery` [I: beside their only user]. On Linux they are read through netlink
  (`RTM_GETADDR` by the standard library's `syscall`, no new dependency), skipping temporary,
  deprecated, tentative and failed addresses and preferring the stable one; container bridges are
  skipped; a change is sent at once rather than found by the minute's poll (§2.4, C7); other
  systems keep `net.InterfaceAddrs`. Addresses are classed by K1's function, so carrier-NAT and
  DS-Lite ranges are never public. `configured` candidates come from the config: the admin's
  port-forward or own VPS (§2.4; built 2026-10-14 as `[discovery] configured_addresses`, which a
  deployment fills from its values' `vps.public_ipv4` and `vps.public_ipv6`). The staging core's direct IPv4 path is its VPS's
  address as a `configured` candidate (infrastructure §3 change 4), and a household keeps its own
  VPS this way rather than retiring it (§2.4; N11).
- **Builds** §2.4; findings C7, C9, N11.
- **Tests.** From recorded netlink messages: a temporary and a deprecated address are left out and
  the stable one kept; a bridge's address is left out; `100.64.0.1` and `192.0.0.2` are never sent
  as public; a configured address is sent with `source: configured`.
- **Waits on:** K1.

### K5 — Port mapping to the protocol

- **Changes.** `internal/core/portmap`: the router asked for **outside port 443 first**, and the
  port it grants taken when 443 is held (D291 §7, S14); never a port on the Fetch standard's
  bad-port list, asked or published (§2.4, C9); TCP only, as built, pinned by a test (D290 §9); the
  external address the router reports checked, and when it is not public nothing published and the
  System page saying why; NAT-PMP's and PCP's epoch read on every answer and PCP's ANNOUNCE listened
  for, every mapping remade when the router restarted; UPnP falling back to `AddAnyPortMapping`,
  then another port, on a conflict; UPnP's description and control URLs accepted only on the
  gateway's own address (code K20) [I]; PCP's nonce kept in the persistent directory, so a restarted
  core renews or deletes its mapping; IPv6 pinholes updated and deleted (`UpdatePinhole`,
  `DeletePinhole`), not re-added, and asked over IPv6 (the homelab research); the report naming the
  common case met (a FRITZ!Box that maps only for a device the owner allowed) (§2.4, C9).
- **Builds** §2.4; D290 §9, D291 §7; findings C9, S14.
- **Tests** against the fake routers `portmap_test.go` already has: 443 is asked first and a granted
  8443 is published; a bad port is never asked for; no UDP mapping is ever asked; a private
  external address publishes nothing and says why; an epoch that went back, and an ANNOUNCE, remake
  the mappings; a conflict falls back to `AddAnyPortMapping`; a control URL on another host is
  refused; a core restarted with its kept nonce deletes its old mapping; a pinhole's renewal is
  `UpdatePinhole`. miniupnpd confirms the fakes in the netlab tier (D292 §9).
- **VQ:** a new item: on a FRITZ!Box and on an OpenWrt router, the core maps 443 (or the port
  granted), the probe reaches it, and the System page names what it met.
- **Waits on:** K1.

### K6 — Claiming

- **Changes, for the beta** (D292 §3): on both tiers `Connect` enrols and the System page shows the
  claim code, which on the paid service the tester reads to the operator, who claims the core, and
  on an own discovery the owner types into `orpheum discovery claim` (§2.2); an expired code
  replaced, the System page saying so (U5); once claimed, the System page says so and shows the
  recovery code (K8a). `web/apps/web/src/system-discovery.tsx`: the code, its expiry and the claimed
  state.
- **After the beta** (D292 §3, with the customer accounts): claiming from the core (D290 §1). The
  core gets a claim token and opens the claim page in a tab opened within the admin's click (U2-4)
  [I]; it polls `GET /status` for a claim waiting and shows which account asks, its address in full,
  with *Allow* and *Refuse*, only an admin's *Allow* binding it, and only when the one-time value
  the claim page returned matches (A15, T10) [I]; the holding account kept and shown; *This isn't
  me, release*, signed by the core, in *Details* (§7 item 5, D291 §6; U2-5); the typed code a
  fallback needing *Allow* (X8). `internal/core/api`: the approval and the release for an admin
  only, under K17.
- **Builds** §2.2; D290 §1; D291 §6; D292 §3; findings S7, U5, C12, A15.
- **Tests.** For the beta: the System page shows the code and replaces an expired one with the
  message; a claimed core shows its state. After the beta, against a fake paid discovery: nothing
  binds until the admin's *Allow*, *Refuse* ends the claim, a claim whose returned value does not
  match is not shown, and a member who is not an admin cannot approve.
- **Waits on:** K1, K2; after the beta, the claim page's address (question 6).

### K7 — Links, locate hashes and locators issued by the core

**Partly built 2026-10-13 (Phase 2, step 0037):** each invitation's and registration link's grant
keeps its locate hash and offer key (`discovery/link`); a used one becomes its person's own link
(kind `link`, scope `own`, 90 days rolling, renewed by a visit), revoking the last; a device locator
per session (`locator`), handed in the session's answer and fresh from `POST /me/locator`; the core
sends both full sets (`PUT /locate-hashes`, `PUT /locators`) when they change and under each new
session, and `single` keeps them. **Built 2026-10-14:** registration and Watch Together links made
in the discovery's form while it has claimed the core (`link` beside `secret` in the answers; the
web client passes it on); Watch Together's unused links' locate hashes and each membership device's
locator (`locator` in the join's and `/together/mine`'s answers; the host's device has its own
session's), in memory in `internal/core/together`, an unused link's life at the discovery running
to the end of the next day and sent again daily [I]; the admin's revocation of an own link, on the
person's page beside until when it leads them back; the person's own link on their account page, as
text and a QR code (`GET /me/own-link`), its secret kept sealed under `secret.key` in its grant (step
0038; the owner, 2026-10-14). **Still K7's:** the web client keeping a membership's locator for its
next visit through a discovery; the `TestArch_` claim (owner's gate); the cap on distinct sources a
day, which is the discovery's.

- **Changes.** The store: a person's own link and device locators tied to their grant (a device's
  session, a Watch Together membership), each kept as a hash; for each link, the value an offer's
  proof is keyed by (K3a). A schema change under rule 8: until
  `internal/store/migrations/CHECKPOINT` exists it folds into the baseline (D272 §5).
  `internal/core/auth` and `internal/core/together`: a locator issued on signing in, registering,
  pairing and joining, a fresh one on each visit, and dropped when its grant ends (§3.1, C3, S11).
  **A registration link becomes the person's own link once used** (D292 §4; A12, U2-1): its locate
  reusable while unused, the registration spent once on the core as built; once used it locates
  only, revoked when the person is removed, resets their password, or the admin revokes it, with a
  rolling life of 90 days renewed by a signed-in device's visit and a cap on distinct sources a day
  [I: the numbers]. A used Watch Together link locates only for the device holding the membership,
  by the locator the core handed it (A12). `internal/core/discovery`: `PUT /locate-hashes`, the full
  set with each hash's life and use policy, and `PUT /locators`, the full set, sent on each change
  and on each new session. `internal/core/api`: where a client gets its locator over the direct path
  [I: the shape is the build's]. **Every link the core makes carries the identity key's
  fingerprint** (D292 §4; A7): while enrolled, `/register#<coreId>.<secret>.<fingerprint>`,
  `/together#…`, `/s#…`, `/p#…` at the discovery (§3.1); the built client makes
  `/register#<secret>` and `/together#<secret>` on its own origin. The web client: the person's own
  link and QR on their account page and after registering, revoked by an admin (D290 §10, U2); the
  admin's link that signs a known person in rather than resetting a password (§7 item 15, D291 §6)
  [I: a grant of its own, unlike the own link, which grants no sign-in].
- **Builds** §3.1; D290 §10, §17; D291 §6; D292 §4; findings C3, S11, U2, A7, A12, W7, X13.
- **Tests.** The core registers the hash of every live link and never the secret; a registration
  link's hash stays reusable while unused and becomes a locate-only hash once used; removing the
  person drops it; a Watch Together link's stays while the link lives; ending a session drops its
  locator from the next `PUT /locators`; revoking a person's own link drops its hash; a link made
  while enrolled names the core's id and its fingerprint. A `TestArch_` claim, cited in
  `internal/core/discovery`'s `doc.go`, that no link secret, session token or password crosses
  `discoveryproto` (§3.1).
- **Waits on:** K1, K8a for the fingerprint; the client origin from `GET /status` (K10; D293 §4).

### K8 — The identity key in full

- **K8a.** The fingerprint, `SHA-256(public key)`: in every link (K7) in base64url, which machines
  read, and on the System page in safe32 groups, which a person reads (§2.1; D292 §4; the owner,
  2026-10-14; both forms built). **The recovery code** (D292 §4, revising D289 §2; A6): made when the
  core is claimed, 128 random bits shown once to print or save; scheduled backups then hold
  `identity.key` sealed under it (argon2id, then XChaCha20-Poly1305, as `secrets` seals [I, A6]),
  and a restore given the code brings back the same key; *Back up now* holds the key as before when
  its box is ticked; the TLS and ACME account keys stay out of scheduled backups (§2.1).
  `internal/core/backup`, `internal/core/identity`. **Tests:** a scheduled backup made before
  claiming holds no identity key and one made after holds it only sealed; a restore with the code
  gives the same key and with a wrong code refuses; a backup with the box ticked holds the key; no
  scheduled backup holds the TLS or ACME account key.
- **K8b.** The core signs a client's nonce on first contact, over a direct path, so the web client
  can remember its key (§3.5, D290 §3) [I: an API endpoint, open to anyone, signing only under the
  client purpose]. **Tests:** the signature verifies with the core's key under that purpose and no
  other.
- **K8d.** After a backup restored without the key and without the recovery code: a core that
  remembers a `coreId` whose key no longer opens a session says so on System → Discovery and hides
  *Connect* (§2.8, D290 §11, U6); a key file still present and refused is shown as an alarm, not a
  routine state (A6 item 5) [I]; two cores with one key: the *another instance* refusal of K2 shown
  on both, with *This is a copy: turn off remote access here* (U2-12) [I]. **Tests:** a refused
  session with a remembered `coreId` shows the state and no *Connect*; a present, refused key raises
  the alarm.
- **K8c (deferred).** Re-key by rotation: the new key's enrolment countersigned by the old (§2.8,
  D290 §3); then a new ACME account and certificate, the old certificate revoked and the old account
  deactivated (C4, A14). Its re-key button in K8d comes with it.
- **K8e (deferred).** Certificate Transparency watched for the core's own names (§2.7, D290 §3,
  S1), by the source question 16 names.
- **Builds** §2.1, §2.8, §3.5; D289 §2 as revised; D290 §3, §11, §18; D292 §4; findings S1, U6, C4,
  A6, U2-12.
- **Waits on:** K1; K8a's recovery code on K6's claimed state.

### K9 — Certificates

- **Changes.** `internal/core/certificate`: renewal at two-thirds of the lifetime, replacing
  `renewBefore`'s fixed 30 days (§2.7, C1); the TXT record checked to resolve before the CA is
  asked, in place of the fixed 30 s, by `golang.org/x/net/dns/dnsmessage` (direct by D292 §9); the
  record cleared by its path (K1); after leaving, the discovery's certificate no longer served
  (C16); the core's ACME account URI reported to the discovery for CAA, and reported again whenever
  the core makes a new account (a restore leaves the account key out), before it orders (§2.7; G2).
  `cmd/orpheum/certificates.go`: **a TLS handshake whose SNI names no name the core serves is
  refused** (§2.7; A10), so a scanner does not learn the core's id. **Deferred** (D292 §3): ARI and
  DNS-PERSIST-01, and so whether `golang.org/x/crypto/acme` v0.57, which has neither and whose order
  options are sealed (F18), is extended by hand or replaced (a dependency, the owner's, rule 3).
- **Builds** §2.7; D290 §7; D292 §1, §3; findings C1, C16, S12, A10, G2.
- **Tests.** Against the fake CA: a 20-day certificate is renewed at about day 13 and not on every
  sweep; the CA is not asked before the record resolves; after leaving, the discovery's names are no
  longer served; a new ACME account is reported before the next order. A ClientHello with no server
  name, or another one, fails the handshake. Pebble confirms the fake in the integration tier
  (D292 §9).
- **VQ:** a new item: against Let's Encrypt's staging and the staging subzone of
  `orpheum-core.com`, a core obtains and renews its wildcard through the staging service, under the
  zone's CAA.
- **Waits on:** K1.

### K10 — Serving the client cross-origin

**Partly built 2026-10-14:** CORS on the API, byte paths included, for the client origin the
discovery names, and the session socket's `OriginPatterns` for it. Byte paths already take the header;
the page the discovery serves reaches them through its service worker, which carries the header.
**Still K10's:** the `Host` allow list on the plain-HTTP LAN listener, and the browser-tier test.

- **Changes.** `internal/core/api`: CORS for the discovery's client origins, a list, never a
  wildcard, no credentials mode, `Vary: Origin`, `null` never allowed, `Authorization` named in the
  allowed headers (§3.4, §7 item 13). Byte paths authenticated by the header on every request
  (D289 §3): images and subtitles by `fetch` into blob URLs, hls.js's loader carrying the header
  (`web/packages/sdk`, `web/packages/player`), the media cookie (`media_cookie.go`) kept for the
  core's own origin only [I]. The Watch Together socket accepting the client origins by
  `OriginPatterns`, never `*` (§3.4). A `Host` allow list on the core's plain-HTTP LAN listener
  against DNS rebinding (§7 item 10, S15). **The client origin comes from the discovery**
  (D293 §4): `GET /status`'s `clientOrigin`, accepted only as an `https` origin with no path,
  shown on the System page (K12), and used for CORS, the socket's `OriginPatterns` and the form of
  links (K7).
- **Builds** §3.4; D289 §3; D291 §6; D293 §4; findings S15, S18, W12.
- **Tests.** A preflight from a configured origin is answered and one from another, or from `null`,
  refused; an answer carries `Vary: Origin` and allows `Authorization`; a request on the LAN
  listener naming another `Host` is refused; a socket from another origin is refused. A
  browser-tier test: a page on another origin plays a film and shows its poster with the header
  alone. A `clientOrigin` with a path, or not `https`, is refused and the core keeps the last good
  one.
- **Waits on:** K1.

### K11 — Versions, status and the exit path

- **Changes.** `internal/core/discovery`: the `major.minor` header; the deprecation's date kept and
  shown (built), and past the window the refusal's accepted versions shown (§5, C13); **the security
  floor** shown and acted on: a core below it is told why it is refused (§5; D292 §3); the
  discovery's messages for the admin (security-update notices, §7 item 9) from `GET /status`. After
  the beta, with payment: the account's state with its dates, the lapse messages with dates and
  *Renew*, household members told the cause (§7 item 18, U7).
- **The exit path, tested** (D25; the paid spec §9.1 item 22): a fake discovery drives the core
  through `grace`, `frozen` and `released`; the core keeps serving its LAN and its own client, says
  what each state means, and names the way to an own discovery.
- **Builds** §5; the paid spec §3.10; D289 §5; D291 §6; D292 §3; findings C13, U7.
- **Tests.** The exit path's walk above; a deprecated discovery's date shows; an unknown minor is
  accepted; a refusal below the floor shows its reason.
- **Waits on:** K1, K2.

### K12 — The System page in plain words

- **Changes.** `web/apps/web/src/system-discovery.tsx`, `system-reachability.tsx` and their strings:
  **the first-run form** that makes the first admin with the setup code, which every tester's
  install needs and the web client lacks (§7 item 17, U2-4; agent-failures A29); **the home
  address**, as a link and a QR, on the System page and each person's account page (§3.1; D292 §6);
  *Remote access* as the name, the discovery's address behind "use my own", which staging depends on
  (§7 item 17, U5); "your server is reachable from the internet at …" shown prominently (§7 item 9,
  S14); `reachableFrom` by what was tested (§2.6); what a household nothing reaches can do, for the
  admin (§3.6, D290 §12); the connections `rtcapi` refused (K3) and the clients that failed to
  connect last week, the count read with the heartbeat (§7 item 16; K16, D293 §7); the discovery's
  client origin (K10; D293 §4); the claim code and the claimed state (K6); the fingerprint and the
  recovery code (K8a); the instance conflict, saying this is another copy (D293 §1), and the restore
  without the key (K8d); the port-mapping cases (K5); admin pages reached through a discovery saying
  "manage this server from home, at <its home address>" (§3.7; K17).
- **Builds** §2.6, §3.1, §3.6, §3.7; D290 §12; D291 §6; D292 §6; findings U3, U5, U7, S14, U2-4, N3.
- **Tests.** Component tests: each state renders its sentence and its action; the first-run form
  makes the first admin with a right code and refuses a wrong one; the home address renders as a
  link and a QR.
- **Waits on:** the steps whose state it shows.

### K13 — The owner's TURN, with credentials that expire

- **Changes.** The heartbeat's `turn` sent for the owner's TURN servers, with credentials minted by
  the core in the TURN REST shape (a username `<expiry>:<id>`, an HMAC under a secret shared with
  the TURN server) and refreshed with each heartbeat, never static ones (§2.4, C14). The core
  already mints such credentials for voice (`internal/core/turn/credentials.go`); this reuses them.
  The owner's TURN as one switch (§7 item 16). A quota of allocations per membership and the VPS's
  own address refused but on the relay range (code K9, A17) [I]. It is a household's route for
  IPv4-only devices behind DS-Lite or carrier NAT (N11), so the owner's own household wants it
  before the beta's end.
- **Builds** §2.4; findings C14, N11. Pairs with [webrtc-transport.md](webrtc-transport.md) W6.
- **Tests.** The heartbeat carries a credential that expires and verifies under the shared secret;
  a member's third allocation is refused. coturn confirms the credentials in the netlab tier.
- **Waits on:** K1.

### K14 — The core's NAT, measured (deferred)

Deferred (D292 §3). When built: the core sends STUN binding requests to two STUN servers on
different addresses and compares the mapped ports (RFC 5780): the same port is
endpoint-independent mapping (§2.6, C10). `pion/webrtc/v4` already brings `pion/stun/v4`, so the
binary carries one STUN library (G10) [I]. **Tests:** against two STUN servers in the test, one
mapping and two mappings are told apart. **Waits on:** how the result reaches the discovery
(question 8).

### K15 — Device codes through the paid service

In the closed beta (D293 §2, revising D292 §3's deferral), after the thin path (Phase 3).

- **Changes.** The core of the signed-in person who enters a code the service issued claims it
  from the service (§3.1, D288 §5). The outcome is sealed to the device's ephemeral P-256 key with
  HPKE (RFC 9180), **DHKEM(P-256, HKDF-SHA256)**, by Go's `crypto/hpke` on the core; the web client
  opens it with Web Crypto, so no minimum browser rises and no library is added (D293 §2). The
  sealed blob carries the session, the core's `coreId`, its identity key's fingerprint and a device
  locator (K7), the service passing on only the blob (D290 §5, S3). The session has the `device`
  scope of K17; the core limits how many codes it claims an hour (§7 item 2); the web client's side
  on the `app.` origin and U9's ease-of-use items (§7 item 20). The page entering a code warns
  "only enter a code shown on a screen in front of you"; the line naming where the asking device is
  (D290 §5, S4) waits until after the beta, since it needs an IP-location database (D293 §2). Its
  design stays open to "a member admits another device by its code", a guest's second device
  (coherence §1.2) [I].
- **Builds** §3.1; D288 §5; D290 §5; D293 §2; findings S3, A11.
- **Tests.** A blob the core seals opens under the client's Web Crypto and RFC 9180's P-256 test
  vectors open under both; a blob altered in transit fails to open; a code-activated session has
  the `device` scope; the core refuses claims past its hourly limit.
- **Waits on:** K1, K7, K17.

### K16 — The dead end counted, and told

- **Changes.** The message of §3.6 written for whoever reads it (the web client, W4c), and its
  showing counted by the client's kind of network, as an aggregate with no identifiers (D290 §12,
  D274 §3); a "copy what happened" a tester can send the owner (§3.6) [I, coherence §4.2]. **The
  service counts** (D293 §7): the client reports a dead end to the discovery, which keeps a weekly
  count per core with no device identifier; the core reads it with its heartbeat's answer for the
  System page (K12). The household's cause, when it is the core's, comes from the `paths` answer's
  `state` (§3.2; D293 §6).
- **Builds** §3.6; D290 §12; D293 §6, §7; finding U3.
- **Waits on:** K1.

### K17 — Session scopes: no admin through a discovery, one `device` scope

**Built 2026-10-13 (Phase 1's A1, as D294 left it).** D294 withdrew both of D292 §4's admin rules:
admin from anywhere, a discovery's client and WebRTC included; `orpheum reset-admin` the operator's
way back in; passkeys an option later. Built: the `device` scope for every code activation (a
WebRTC request's head no longer believed); `requireAccountManager` on the password, device removal, signing out elsewhere
and entering codes; `canManageAccount` in `/me`, by which the account page hides them. Not yet: the
log of an admin activating another admin's device, re-asking the password for destructive admin
actions, and the `TestArch_` claim (the owner's gate).

- **Changes.** `internal/core/auth`, `internal/core/api`: admin works from anywhere, a discovery's
  client and WebRTC included (D294 §1, §4, withdrawing D292 §4's two admin rules), with
  `orpheum reset-admin` the operator's way back in. **One
  `device` scope for every code-activated device** (§3.1; D292 §4; A11): the role minus
  administering and minus account security; an admin entering a code for another admin confirms it,
  and the core logs who did it for whom (code K10). The web client reads the session's rights, not
  the role's, and an admin page on a session without admin offers the step to the admin origin.
  Destructive admin actions ask for the password again (A1 item 3) [I]. A passkey for admin comes
  later (D292 §4).
- **Builds** §3.1, §3.7; D291 §5 as refined; D292 §4; findings A1, A11, W5, U2-6, code K10.
- **Tests.** A sign-in from another origin, and one over a WebRTC data channel, get a session that
  is refused an admin call; one from the core's own origin from an address in its LAN prefix is
  allowed admin; one from the tunnel's address is not; a code-activated session is refused changing
  the password and making links but can host Watch Together; an admin's activation for another admin
  is logged. A `TestArch_` claim, cited in `internal/core/auth`'s `doc.go`, that every session is
  issued with a scope the request's origin allows.
- **Waits on:** K1; K3a for the untrusted head.

### K18 — A device secret, and credentials per core

**Built 2026-10-13, but for credentials per core:** the device secret (an issued secret tied to the
device, renewed at each sign-in, no schema change), and the lock-out fix (sign-in limits keyed by
site, an IPv6 /48; past the person-everywhere limit a sign-in waits ten seconds; a known device
not held to it). Credentials kept per `coreId` come with the central client (W4c, Phase 2): today
each core serves its own client, one origin per core.

- **Changes.** `internal/core/auth`: **the device proof is a device secret** (§3.4; A8, code K1,
  W4): random, issued at a device's first sign-in, stored hashed on the device row, never a session
  token and never sent to a discovery; a device is resumed only on it, never on a bare id, and never
  as the device the person entering a code is using. `web/apps/web/src/session.ts` and
  `web/packages/sdk`: **every credential, device id, device secret, locator and remembered key kept
  under its `coreId`**, an API client bound to one core attaching its credential only to that core's
  names or its WebRTC transport (§3.4; A8, W1); the web side is W4c's. **The account lock-out
  fixed** (D293 §9; A13): a device that proves an earlier sign-in by its device secret passes the
  account-wide sign-in limit, networks are keyed by IPv6 /48, and the account-wide limit delays
  rather than refuses.
- **Builds** §3.4; D292 §4; D293 §9; findings A8, A13, W1, W4, code K1.
- **Tests.** A sign-in naming another device's id without its secret makes a new device; one with
  the secret resumes it; the session token is no longer accepted as a proof; a request for core A's
  names carries A's credential and never B's; a playlist's foreign URL goes without a credential. A
  `TestArch_` claim that no request leaves the client with a credential for a core other than its
  target. Past the account-wide limit, a sign-in from a device with its secret succeeds and one
  without is delayed, not refused; two addresses in one /48 count as one network.
- **Waits on:** K1.

## Found while planning

- **A core in a container sees the container's addresses**, not the host's: the bridge network's
  private ones. The heartbeat's `interface` candidates are only real under host networking. K4
  skips bridges; a containerised core then reports its `configured` addresses, and its discovery
  sees its source address (§2.4). The release compose file puts the core on a bridge, so a
  deployment without the tunnel chooses one of the two (infrastructure §1).
- **`newDiscovery` sets no `TURN`**, so the heartbeat never carries the owner's TURN; K13.
- **The server's keys were in every backup** until 2026-10-13: `internal/core/backup` left out
  `secret.key` alone, so the identity key and the certificates' and ACME account's private keys went
  into scheduled backups in plain text (T1, X1). Since an earlier commit a backup holds them only when
  asked; K8a seals the identity key into scheduled backups under the recovery code (D292 §4).
- **No CORS on the core's API**, and byte paths lean on the media cookie (D216), which a page on
  another site never sends; K10.
- **IPv6 pinholes and high ports** (from
  [homelab-networks-2026-10-10.md](../research/homelab-networks-2026-10-10.md) §6, read against
  miniupnpd's source): miniupnpd refuses a pinhole unless asked over IPv6 from the address opened
  and for a port of 1024 or more; stock OpenWrt maps only 1024–65535 for IPv4. D291 §7's 443 first
  will meet that refusal there and take the port granted; K5 asks pinholes over IPv6.
- **miniupnpd maps nothing for IPv4 when its own WAN address is private**: every inner router of a
  double NAT. K5's external-address check says so rather than retrying.
- **Every offer through a discovery is answered unchecked today**, and pion's defaults take any
  number of remote candidates at any address (A2); K3a, before any signalling connection.

## The order of work

The closed beta reaches a few invited households from `app.orpheum-app.com` wherever they are, over
a direct HTTPS path where one exists and WebRTC through the service where none does (D290 §14), with
no payment, no public sign-up, no customer accounts and one core per account (D290 §18, D292 §3).
The owner's rule is that nothing goes public until every component has been tested together, so the
components meet early and thin, and are thickened after (D292 §3). Orpheum's part, by phase; the
service's parts are in `orpheum-connect`'s bring-up plan.

### Phase 0 — before any discovery code

- **check-pr back under its budget** (D292 §9): from 2,498 s to 676 s with nothing loosened
 ; what remains over 300 s is the browser tier, to be cut by a recorded
  map of which browser spec exercises which source file, built after this documentation pass and
  Phase 1 (the owner's choice).
- **This documentation pass**: the second and third rounds folded into the protocol and these plans
  (D292).
- **The development loop** (D292 §9; the owner's gate and dependency work): the service imports
  Orpheum by `replace` and a pin file, with `make spot-both`; its own gates and Stop hook; Pebble
  pinned; a netlab tier on demand; `scripts/dev.py` running two cores; `pion/transport/v5` and
  `golang.org/x/net` direct.
- **The owner's long waits**: Let's Encrypt's rate-limit increase for `orpheum-core.com` (weeks,
  D290 §7); the staging Tunnel and its token; the edge VPS's Floating IPv4 and WireGuard peers
  (below); the CAA at `media.orpheum-app.com` now (A5). Outside these plans and possibly longer:
  testers' installation, a deployment without the VPS tunnel (D272 §3) and a way for testers to
  obtain the image ([beta-inventory-2026-10-09.md](../research/beta-inventory-2026-10-09.md) (d)
  2–3).

### Phase 1 — the security floor

In the public code, before any core holds a signalling connection to any discovery (D292 §4).
**K1 first**, with W4b.7's wire, since every other step speaks its types. Then, beside each other:

1. **A3** — `discovery/single`'s code K13–K15 closed: tokens bound to the core, no replacing a
   pending core, no certificate record, addresses or signalling before claiming, 130-bit ids
   ([discovery-mode.md](discovery-mode.md) W4b.7).
2. **A2, A10** — offers checked and bounded (K3a); no certificate for a handshake that names no
   served name (K9's SNI part). **Built 2026-10-13**, but for the parts K3a's status leaves to the
   steps below.
3. **A1** — withdrawn by D294: admin from anywhere, and `orpheum reset-admin`; the `device`
   scope for code-activated devices (K17) and an untrusted WebRTC head stay. **Built 2026-10-13.**
4. **A7, A9** — the identity key's fingerprint in every link and QR (K7's link form, K8a's
   fingerprint); answers signed over the exact SDP, holding exactly one sha-256 fingerprint (K3a,
   K8b; webrtc-transport.md W2f). **The core's half built 2026-10-13** (signatures bound to the
   discovery and core, answers signed, `discovery/link`, the fingerprint); the offer's proof waits
   on locators (K7) and the client's checks on the client, both Phase 2.
5. **A8** — credentials per core and a device secret that is not a live token (K18;
   webrtc-transport.md W4c's credential store). **The device secret built 2026-10-13**; credentials
   per core with the central client (Phase 2).
6. **A13** — the account lock-out fixed (D293 §9): a device proving an earlier sign-in by its
   device secret passes the account-wide limit, networks keyed by IPv6 /48, and the account-wide
   limit delaying rather than refusing (K18). **Built 2026-10-13.**

### Phase 2 — the thin path, end to end on staging

Before any step is thickened, one path works from end to end (D292 §3; coherence §3.2): the staging
core enrols with the staging service, the operator claims it, its name gets its certificate, and a
phone on mobile data opens `app-staging.orpheum-app.com` and plays through each path (direct IPv6,
the configured VPS address, WebRTC).

| Piece | Real in the thin path | Waits for Phase 3 |
| --- | --- | --- |
| Sessions (K2) | `expiresIn`, renewal on a 401, backoff starting near 1 s | the instance rule's ping (D293 §1) |
| Signalling (K3) | K3a in full; K3b's ping, heartbeat, concurrency, idle close | ICE restart (a failed connection starts a new race) |
| Links (K7) | locate hashes, device locators, the person's own link, fingerprints in links | the admin's sign-in link |
| Addresses (K4) | stable IPv6, the non-public ranges, `configured` | netlink's change events: polling stays |
| Port mapping (K5) | as built, TCP only | the C9 list; the FRITZ!Box VQ |
| Certificates (K9) | DNS-01 through the discovery as built, renewal at two-thirds, SNI refusal | — |
| Cross-origin (K10) | CORS, byte paths by header, the `Host` allow list | — |
| Identity (K8) | K8a's fingerprint, K8b | K8a's recovery code, K8d |
| Claiming (K6) | the code, claimed by the operator | — |
| Scopes (K17), device secret (K18) | in full, from Phase 1 | — |
| Client (W2c–W2f, W4c) | early description, fingerprint checks, keep what connects and replace what fails, the race ranked by kind, credentials per core | ICE restart, network-change triggers, the in-app-browser hint |
| STUN | any public STUN server | the discovery's own (W4b.3) |
| Versions (K11), System page (K12), TURN (K13), counting (K16) | the deprecation header as built | the rest |

In order: K1 with W4b.7; the core's thin steps and `single`'s client API (W4b.4) beside each other;
the browser-tier test through `single` (W4b.6), where all public parts first meet, which runs in CI
from then on; the web client's thin steps; the same test against the paid service's handlers, by
the conformance suite `discovery/discoverytest`; then the staging deployment.

**Orpheum's deployment, for staging** (D292 §7; infrastructure §3): the WireGuard tunnel's
addresses chosen per deployment, so the staging core (`10.77.0.3`, its own key) runs beside
nas.example (`10.77.0.2`, unchanged), revising D272 §8's one shared key; `deploy/vps/` for several
peers, forwards matched by destination address, a forward chain with policy drop and SSH over the
tunnel only; the staging core reconfigured with no `domain`, the staging discovery entered under
*use my own*, and its Cloudflare token deleted. The edge VPS becomes the staging server once
`media.` retires (D292 §7).

**What must be real from the first day** (coherence §3.2): the zone and its written records with
their CAA, a real browser on a real carrier network, and the central origin's service worker.

### Phase 3 — thickening, and what testers need

In the order the thin path's failures show, and before the first outside household:

- **What testers need**: K12's first-run form; the home address (K12; D292 §6); K8a's recovery code
  (D292 §4); a tester's diagnosis (K16); testers' installation (Phase 0's outside items); **device
  codes through the paid service** (K15; D293 §2), so a television joins by a code.
- **The rest of the steps**: K3b's ICE restart; K4's change events; K5's C9 list; K8d; K10's
  browser test; K11; K12; K13 (the owner's household wants it, N11); K16's counting; W2c's
  network-change triggers and W4c's remaining items.

### Phase 4 — production

Only once every component has been tested together on staging (the owner: *"Nothing will become
public until we've tested all of the components together"*): the service on its two machines,
`prod-api` and `prod-probe` (D292 §3); the legacy Cloudflare token revoked before the first
production name (A5), and staging's token for `orpheum-core.com` revoked before the first production
core enrols, staging's writes moved behind the production zone writer limited to
`staging.orpheum-core.com` (D293 §3); then the owner's staging core as the first household, then
nas.example, moved off `media.orpheum-app.com` before that certificate's renewal (about 2027-01-04,
infrastructure §4), then one outside household, then the invited ones.

### Deferred, after the beta

From D292 §3 and the steps above: customer accounts, sign-in links and mail, and with them claiming
from the core and its approval (K6's second part); re-keying and mailbox recovery (K8c, §2.8); CT
watching (K8e); ARI and DNS-PERSIST-01 (K9); the NAT measurement (K14); the line naming where a
device asking for a code is (K15; D293 §2); the version window's promise (K11, §5); the spare
machine and
Cloudflare Pages; a switcher for a person on two servers (D291 §6); `orpheum discovery`'s command
and its web serving (W4b.5, W4b.8), the self-hoster's route in the beta being the VPS tunnel
(`deploy/vps/`) [I, coherence §4.2]. Device codes through the paid service (K15), deferred by D292
§3, are in the beta (D293 §2; Phase 3).

## Questions the plans raise

Not decided here; each is the owner's, or a fix to the protocol document. Those the third round's
rulings, this pass or D293 settled say where; D293 left the rest until their features are built,
after the beta.

1. **The heartbeat on the signalling connection, and the hourly probe.** *Settled:* D291 §6's
   "something simpler", the heartbeat on the connection with `PUT /addresses` its fallback (§2.4),
   and probing on change, at most once per core per 10 minutes (§2.6) [I: X9's reading of S10].
2. **What a signature binds.** *Settled* (X12): the base URL the core called, path included, without
   a trailing slash (§2.1).
3. **The enrolment's `protocol` field.** *Settled* (X11): dropped (§2.2).
4. **When an instance stops being live** (K2): a discovery refuses a second `instanceId` per
   `coreId`. A core that restarts, or crashes without closing its connection, comes back with a new
   one while its old token still lives. Its signalling connection's closing and two missed
   heartbeats (T14), or the newest instance winning in the beta (coherence decision 7)? Without a
   rule, a restart is refused as "another server". Protocol §9 item 1. *Answered* by D293 §1: the
   discovery pings the old instance on its connection and waits about 10 seconds; no answer, or no
   connection, and the newcomer replaces it, an answer and the newcomer is refused (K2).
5. **How the core learns the discovery's client origin** (K7, K10): links take the form
   `https://<discovery>/register#…`, and CORS allows "the configured client origins", but the core
   knows only the discovery's API base URL (`https://api.orpheum-app.com/core/v1`). A field in
   `GET /status`, or a setting? *Answered* by D293 §4: `GET /status`'s `clientOrigin`, an `https`
   origin with no path, shown on the System page (K10).
6. **The claim page's address** (K6, after the beta): whether the URL comes from the enrolment's
   answer or is fixed in the core. The beta claims by the code (D292 §3), and the typed code stays a
   fallback after it (D291 §6; X8). *After the beta* (D293), when claiming from the core is built.
7. **Matching an ICE restart to its connection** (K3b, W2d): a restart's offer goes "through the
   discovery (or over the data channel while it lives)", but nothing in §2.5 or §4 names which
   existing connection an offer restarts. Not on the thin path.
8. **Where the NAT measurement goes** (K14, deferred): does the core report its finding in the
   heartbeat, or do the discovery's STUN servers observe it? *After the beta* (D293), with the
   measurement.
9. **Counting failures** (K12, K16): D290 §12 counts how often the dead-end message is shown, and §7
   item 16 has the System page say how many clients failed to connect last week. A client that
   failed reached neither the core nor, by any call in §4, the discovery. Which side counts, and by
   what call? *Answered* by D293 §7: the service, from the client's dead-end report, a weekly count
   per core with no device identifier, read by the core with its heartbeat (K16).
10. **Ordinary rights for which codes.** *Answered* by D292 §4: one `device` scope for every
    code-activated device, on both tiers (K17).
11. **The Watch Together link's path.** *Settled* (X13): `/together`, as the built client makes it
    (§3.1) [I].
12. **The remembered paths' key** (W2c): §3.3 item 7 keys remembered paths on "the client address
    prefix the discovery reports", but §3.2's `paths` answer has no field carrying it. *Answered* by
    D293 §8: the `paths` answer's `clientPrefix`, the client's network prefix as the discovery sees
    it (§3.2).
13. **The bootstrap token.** *Settled* (X10): issued by `/locate`, usable for its minutes, never
    renewed (§3.1).
14. **First contact over WebRTC alone.** *Answered* by D292 §4: every link and QR carries the
    identity key's fingerprint (K7).
15. **What the sealed blob carries** (K15): the session and the check code's outcome; also the
    core's id and a device locator, and is the check code itself sealed? *Answered* by D293 §2: the
    session, the core's id, its identity key's fingerprint and a device locator; nothing the device
    needs crosses the service readable.
16. **Certificate Transparency's source** (K8e, deferred): the service watching the whole zone and
    passing alarms on, a core's own query opt-in (A16) [I]. *After the beta* (D293), with CT
    watching.
17. **The ACME library** (K9): waits with ARI and DNS-PERSIST-01 (deferred). *After the beta*
    (D293).
18. **A lost key in the beta, and recovery after it** (§2.8): protocol §9 items 2–3. *Answered* by
    D293 §5 for the beta: the operator re-keys by hand after checking with the household; recovery
    waits on its own design, after the beta.
19. **Where a core's state reaches a household member** (K16, W4c): protocol §9 item 4. *Answered*
    by D293 §6: a `state` field in the `paths` answer (§3.2).
