> **ABOUTME:** The core's side of the discovery protocol (D273): its identity key, enrolling with a
> discovery (the paid service at app.orpheum-app.com or the owner's own), keeping its addresses
> current, and answering WebRTC offers the discovery passes on. The phases built, and the ordered
> steps that bring the core to the protocol as decided after the audits (D290, D291).

# Discovery: the core's side

- **Status:** IN-PROGRESS — begun 2026-10-09 on the owner's word (*"Do the core side of the
  discovery protocol"*); phases C1–C5 are built. Steps K1–K16 below, written 2026-10-13 from
  [discovery-protocol.md](../discovery-protocol.md) as revised for D290 and D291, are planned and
  not started: the owner asked that building wait (2026-10-13).
- **Depends on:** D273, D274, D275, D288, D289, D290, D291, D214 §4;
  [discovery-protocol.md](../discovery-protocol.md), the protocol both discoveries speak;
  [discovery-mode.md](discovery-mode.md) (`discovery/single`, the reference every core test runs
  against); [webrtc-transport.md](webrtc-transport.md) (the client's side of the same paths).
- **Blocks:** the paid service, whose plan is kept in its own repository: it imports the wire types
  K1 settles (D275 §1), so it cannot be finished correctly before them.

## The wire

The protocol is [discovery-protocol.md](../discovery-protocol.md): §2 between the core and its
discovery, §3 between a client and the discovery, §4 the calls, §5 versions. This plan does not
repeat it. Its types are `discovery/discoveryproto`'s, a public package the closed service imports
(D275 §1); what is signed is `discovery/proof`'s. Where the built wire differs, the protocol says
**the code must change**, and the steps below say where.

**Names in this plan.** Phases C1–C5 keep the names they were built under. The correctness audit's
findings are also called C1–C16, so the remaining steps are named K1–K16, and a finding is always
written as the protocol writes it, after the rule it answers (for example "(C11)").

## Phases built

| Phase | What | State |
| --- | --- | --- |
| C1 | The identity key: Ed25519, made on first start, `identity.key` in the persistent directory beside `secret.key`, mode 0600 | BUILT 2026-10-09. D289 §2 since decided it is backed up as `secret.key` is; `internal/core/backup` holds `secret.key` alone today (K8) |
| C2 | `discovery/discoveryproto` and `internal/core/discovery`: connect (challenge, enrol, claim code), poll the status until claimed, sessions, the heartbeat every 5 minutes with the core's candidates; `discovery.json` in the persistent directory; `GET/POST/DELETE /system/discovery` for an admin; the System page's Discovery section | BUILT 2026-10-09; its candidates are the machine's interface addresses (`cmd/orpheum/discovery.go`) and the mapped ones (C5); `configured` candidates and TURN are not sent |
| C3 | The signalling connection: offers answered by `rtcapi`, without a signed-in check of their own; reconnects | BUILT 2026-10-09; one offer at a time, a flat 30 s between dials, no ping |
| C4 | Certificates through the discovery: a wildcard `*.<coreId>.<zone>` by DNS-01, the discovery setting the TXT record (`PUT/DELETE /acme-challenge`), kept as `discovery-tls-*.pem`, served for every name under it | BUILT 2026-10-09; renews 30 days before expiry, waits a fixed 30 s for the record |
| C5 | Port mapping: PCP → NAT-PMP → UPnP IGD for IPv4, UPnP IGD 2 pinholes for IPv6 (`internal/core/portmap`), `should_map_ports` on by default, only while the discovery publishes the core | BUILT 2026-10-10, on the standard library alone. TCP only already; asks for the core's own HTTPS port as the external one |

## The remaining steps

Each step names what it changes, the decision and the protocol section it builds, the tests that go
red when it is reverted (AGENTS.md rule 4), any `VQ` item a device alone can confirm, and what it
waits on. Tests are described, not named: a name is given when the test is written. Every test runs
against `discovery/single` or a fake discovery, a fake router, the fake CA
(`certificate/certificatetest`) and `clocktest`; none touches the network (rule 16).

Order: **K1 first**, since everything speaks its types. Then K2 and K3 (the session and the
signalling path that WebRTC in the beta stands on, D290 §14), with K7 and K8b, which K3's checks
need. K4–K6 and K9–K10 can run beside them. K11–K16 follow. The order across the core, the web
client, W4b and the paid service is [The beta's order of work](#the-betas-order-of-work) below.

### K1 — The wire, revised in the public packages

- **Changes.** `discovery/discoveryproto`: `SessionRequest` gains `instanceId`; `SessionResponse`
  carries `expiresIn` (seconds) in place of `expiresAt`; the body's `protocol` is dropped from the
  heartbeat (§2.4) and the header carries `major.minor` (§5); `ProbeResult` becomes fields
  (`address`, `port`, `kind`, `result`) and `reachableFrom` takes the values named by what was
  tested (§2.6); `SignalMessage` carries `locatorHash` and `fingerprintSignature`, and one message
  up to four offers (§2.5); a refusal the core must act on carries an error code of its own (enrol
  again, another instance running; §4); the TXT record is cleared by
  `DELETE /acme-challenge/{name}` (§2.7); new types for the claim token and its approval (§2.2), the
  holding account and the account's state in `GET /status`, `PUT /locate-hashes` and
  `PUT /locators` (§3.1), the core's ACME account URI (§2.7), the re-key (§2.8), the paid device
  code's claim (§3.1), and the client API (`locate`, `paths`, `offer`, device codes; §3.2, §4).
  One function encodes an address as a name label, with test vectors (§2.7, C15). One function
  classes an address as public or not, counting `100.64.0.0/10`, `192.0.0.0/29` and the
  documentation ranges as not public (§2.4, C7, C9) [I: in `discoveryproto`, so the core and every
  discovery class addresses alike]. One function derives a link's locate-hash,
  `SHA-256("orpheum-locate" ‖ secret)` (§3.1).
- `discovery/proof`: `Message` binds the discovery and the core,
  `orpheum-discovery-v1:<purpose>:<discovery origin>:<coreId or public key>:<nonce>` (§2.1, C8),
  with new purposes for the re-key, a client's first-contact challenge and the DTLS fingerprint.
  `internal/core/identity.Key` signs through it.
- `discovery/single` and `internal/core/discovery` follow in the same change, since nothing is
  deployed and v1 changes in place (§2.1, D290 §13); single's own share is
  [discovery-mode.md](discovery-mode.md) W4b.7.
- **Builds** §2.1, §2.4, §2.6, §2.7, §4, §5; D290 §13; findings C8, C13, C15, C16.
- **Tests.** A signature made for one discovery fails at another, and one made for one core fails
  for another. The name encoding's vectors: one address has one name, no label holds `--`, IPv6 in
  eight groups without compression. The address classes: a carrier-NAT, a DS-Lite and a
  documentation address are never public. Each new message decodes what the protocol's examples
  show. A test that a discovery accepts any minor of its major and refuses another major with the
  versions it accepts.
- **Waits on:** nothing. It is the root of every other step, of W4b and of the paid service.

### K2 — Sessions, retries and instances

- **Changes.** `internal/core/discovery/discovery.go`: the session's expiry counted from
  `expiresIn` on the monotonic clock (C11); one renewal at a time, shared by the step loop, the
  signalling loop and the certificate job (`SetTXT`), renewing five minutes before expiry and at
  once, once, on any 401, then backing off (C11); every retry backing off exponentially with jitter,
  and the five-minute heartbeat carrying ±20% jitter (§2, C7), in place of `retryAfter`'s flat 30 s;
  an `instanceId` made at each start, kept in memory, never backed up (§2.3, C4); acting on the
  named refusals: *enrol again* re-enrols and shows the new code, *another instance* is shown on the
  System page and not retried as an ordinary failure (§2.2, §2.3, C12, C4); `Connect` serialised,
  so a double click enrols once (§2.2, C12).
- **Builds** §2, §2.2, §2.3; D290 §18; findings C4, C7, C11, C12.
- **Tests.** After a 401 the next call carries a renewed token without waiting 30 s. Three callers
  asking at once open one session. A core whose wall clock jumps a day neither renews on every step
  nor sends an expired token. Two cores' retry waits grow and differ (the jitter's random source
  given by the test). Two `Connect`s at once enrol once. An *enrol again* refusal re-enrols and
  shows a new code. An *another instance* refusal shows on the status and stops the retries.
- **Waits on:** K1.

### K3 — The signalling connection, bounded and checked

- **Changes.** `internal/core/discovery/signal.go`: a ping every 25–30 s, two missed pongs ending
  the connection, and a reopening within seconds, with jitter, on the shared session (§2.5, C11);
  offers answered concurrently under `rtcapi`'s limit, several in one message (§2.5, C6, S5); each
  offer's `locatorHash` checked against the set the core registered (K7), or for a bootstrap token
  the locate-hash it came from, and refused when unknown (§2.5, D290 §6, S5); each answer's
  `a=fingerprint` signed with the identity key as `fingerprintSignature` (§2.5, D290 §6, S6);
  the connection closed and its tokens dropped on leaving, re-keying, revocation and disabling
  (§2.5, C4); offers through the discovery stripped of loopback and link-local remote candidates,
  and refused past about ten candidates (§7 item 11, D291 §6, S16).
- `internal/core/rtcapi`: **half-trickle**, the answer sent once the host candidates and the first
  server-reflexive one are in, or after about a second, in place of `gatherTimeout`'s wait for
  gathering to complete (§2.5, C6); **a slot counted once ICE connects**, an unconnected offer
  bounded to about 10 s, the cap sized by memory rather than `MaxConnections`' 64, and refusals
  counted for the System page (§2.5, C6); **idle connections closed** when they carried no request
  with a valid sign-in for a few minutes, which needs the API's sign-in check to tell `rtcapi` per
  request whether it passed [I] (§2.5, D290 §6, S5); **two kinds of connection**: one whose offer
  carries the client's proof of its session (the security audit's S5 item 2: the session's id and
  an HMAC over the offer's DTLS fingerprint keyed by the session's token, which the discovery
  cannot use), and unproven ones, limited to signing in, registering and pairing, with a small quota
  of their own (about 4), a life of about 60 s without a sign-in, and evicted first (§7 item 3,
  D291 §6); answering an **ICE restart** offer on an existing connection, through the discovery or
  over the connection's own data channel while it lives (§2.5, C6; see the questions).
- **Builds** §2.5; D290 §6; D291 §6 (§7 items 3, 11); findings S5, S6, S16, C4, C6, C11.
- **Tests.** An offer whose `locatorHash` the core never registered is refused though the discovery
  passed it. The answer's signature verifies over its fingerprint with the core's public key. A slow
  gathering does not hold the next offer's answer. A connection whose pongs stop is reopened within
  seconds. After leaving, the signalling connection is closed. An offer that never connects frees
  its slot after about 10 s. A connection idle past the limit is closed while one carrying signed-in
  requests stays. An unproven connection's request to the library is refused while a sign-in on it
  is answered. An answer leaves before gathering completes once a server-reflexive candidate is in
  (`rtcapitest` with a STUN server in the test). Sign-in limits key on the ICE pair's address over
  WebRTC as over TCP (§7 item 9, S14).
- **VQ:** none; W5 tests the client's side in a browser.
- **Waits on:** K1, K2; K7 for the locator set; K8b for the proof's key.

### K4 — Addresses: stable, public and the admin's own

- **Changes.** The interface candidates move out of `cmd/orpheum/discovery.go` into
  `internal/core/discovery` [I: beside their only user]. On Linux they are read through netlink
  (`RTM_GETADDR` by the standard library's `syscall`, no new dependency), skipping temporary,
  deprecated, tentative and failed addresses and preferring the stable one; container bridges are
  skipped; a change is sent at once rather than found by the minute's poll (§2.4, C7). Addresses
  are classed by K1's function, so carrier-NAT and DS-Lite ranges are never public. `configured`
  candidates come from the config: the admin's port-forward or own VPS (§2.4; not built).
- **Builds** §2.4; findings C7, C9.
- **Tests.** From recorded netlink messages: a temporary and a deprecated address are left out and
  the stable one kept; a bridge's address is left out; `100.64.0.1` and `192.0.0.2` are never sent
  as public; a configured address is sent with `source: configured`.
- **Waits on:** K1.

### K5 — Port mapping to the protocol

- **Changes.** `internal/core/portmap`: the router asked for **outside port 443 first**, and the port
  it grants taken when 443 is held (D291 §7, S14); never a port on the Fetch standard's bad-port
  list, asked or published (§2.4, C9); TCP only, as built, pinned by a test (D290 §9); the external
  address the router reports checked, and when it is not public nothing published and the System
  page saying why; NAT-PMP's and PCP's epoch read on every answer and PCP's ANNOUNCE listened for,
  every mapping remade when the router restarted; UPnP falling back to `AddAnyPortMapping`, then
  another port, on a conflict; PCP's nonce kept in the persistent directory, so a restarted core
  renews or deletes its mapping; IPv6 pinholes updated and deleted (`UpdatePinhole`,
  `DeletePinhole`), not re-added, and asked over IPv6 (the homelab research); the report naming the
  common case met (a FRITZ!Box that maps only for a device the owner allowed) (§2.4, C9).
- **Builds** §2.4; D290 §9, D291 §7; findings C9, S14.
- **Tests** against the fake routers `portmap_test.go` already has: 443 is asked first and a granted
  8443 is published; a bad port is never asked for; no UDP mapping is ever asked; a private
  external address publishes nothing and says why; an epoch that went back, and an ANNOUNCE, remake
  the mappings; a conflict falls back to `AddAnyPortMapping`; a core restarted with its kept nonce
  deletes its old mapping; a pinhole's renewal is `UpdatePinhole`.
- **VQ:** a new item: on a FRITZ!Box and on an OpenWrt router, the core maps 443 (or the port
  granted), the probe reaches it, and the System page names what it met.
- **Waits on:** K1.

### K6 — Claiming from the core

- **Changes.** `internal/core/discovery`: on the paid service, `Connect` gets a claim token and
  opens the claim page in the same browser (§2.2, D290 §1); the core polls `GET /status` for a
  claim waiting, shows which account asks (masked) with *Allow* and *Refuse*, and only an admin's
  *Allow* binds it; the holding account, masked, kept and shown from then on; an expired code
  replaced, the System page saying so (U5); *This isn't me, release*, signed by the core (§7
  item 5, D291 §6). On an own discovery the claim code stays (§2.2 item 4). `internal/core/api`:
  the approval and the release for an admin only. `web/apps/web/src/system-discovery.tsx`: opening
  the claim page on the admin's click, the approval, the holding account.
- **Builds** §2.2; D290 §1; D291 §6; findings S7, U5, C12.
- **Tests.** Against a fake paid discovery: nothing binds until the admin's *Allow*, and *Refuse*
  ends the claim. A member who is not an admin cannot approve. The System page opens the claim page
  the core was given and shows who asks; once claimed it shows the holding account. An expired code
  shows a new one with the message.
- **Waits on:** K1, K2; the claim page's URL (see the questions).

### K7 — Links, locate hashes and locators issued by the core

- **Changes.** The store: a person's own link (one per person, revocable) and device locators tied
  to their grant (a device's session, a Watch Together membership), each kept as a hash. A schema
  change under rule 8: until `internal/store/migrations/CHECKPOINT` exists it folds into the
  baseline (D272 §5). `internal/core/auth` and `internal/core/together`: a locator issued on
  signing in, registering, pairing and joining, a fresh one on each visit, and dropped when its grant
  ends (§3.1, C3, S11). `internal/core/discovery`: `PUT /locate-hashes`, the full set with each
  hash's life and use policy (a registration link once; a Watch Together link while it is live on
  the core, D290 §17; a person's own link until revoked, D290 §10), and `PUT /locators`, the full
  set, sent on each change and on each new session. `internal/core/api`: where a client gets its
  locator over the direct path [I: the shape is the build's]. The links take the discovery's
  form: `/register#<coreId>.<secret>`, `/join#…`, `/s#…`, `/p#…` (§3.1); the built client makes
  `/register#<secret>` and `/together#<secret>` on its own origin. The web client: the person's own
  link and QR on their account page and after registering, revoked by an admin (D290 §10, U2); the
  admin's link that signs a known person in rather than resetting a password (§7 item 15, D291 §6)
  [I: a grant of its own, unlike the own link, which grants nothing].
- **Builds** §3.1; D290 §10, §17; D291 §6; findings C3, S11, U2.
- **Tests.** The core registers the hash of every live link and never the secret; a registration
  link's hash goes once it is used; a Watch Together link's stays while the link lives; ending a
  session drops its locator from the next `PUT /locators`; revoking a person's own link drops its
  hash; a link made while enrolled names the core's id. A `TestArch_` claim, cited in
  `internal/core/discovery`'s `doc.go`, that no link secret, session token or password crosses
  `discoveryproto` (§3.1).
- **Waits on:** K1; how the core learns the discovery's client origin (see the questions).

### K8 — The identity key in full

- **K8a.** The fingerprint, `SHA-256(public key)` in safe32 groups, on the System page (§2.1);
  `identity.key` in *Back up now* when its box is ticked and never in scheduled backups, as
  `secret.key` is (D289 §2, `internal/core/backup`), and the backup page saying what leaving it out
  costs (U6). **Tests:** a backup with the box ticked holds the key and a scheduled one does not; a
  restore brings it back.
- **K8b.** The core signs a client's nonce on first contact, over a direct path, so the web client
  can remember its key (§3.5, D290 §3) [I: an API endpoint, open to anyone, signing only under the
  client purpose]. **Tests:** the signature verifies with the core's key under that purpose and no
  other.
- **K8c.** Re-key by rotation: the new key's enrolment countersigned by the old (§2.8, D290 §3);
  then a new certificate and the old one revoked (C4). **Tests** against `single`: the countersigned
  key takes the same `coreId`; one not countersigned does not; the old certificate is revoked.
- **K8d.** After a backup restored without the key: a core that remembers a `coreId` whose key no
  longer opens a session says so on System → Discovery, offers the re-key and hides *Connect*
  (§2.8, D290 §11, U6); on an own discovery it names `orpheum discovery rekey`. Two cores with one
  key: the *another instance* refusal of K2 shown on both, with the choice of which to keep (§2.3,
  D290 §18). **Tests:** a refused session with a remembered `coreId` shows the re-key and no
  *Connect*.
- **K8e.** Certificate Transparency watched for the core's own names, a certificate it did not ask
  for shown to the admin as an alarm (§2.7, D290 §3, S1). **Tests:** against a fake log, an unknown
  certificate raises the alarm and the core's own does not. Waits on the source (see the
  questions).
- **Builds** §2.1, §2.7, §2.8, §3.5; D289 §2; D290 §3, §11, §18; D291 §6; findings S1, U6, C4.
- **Waits on:** K1; K8c on K9's revocation.

### K9 — Certificates

- **Changes.** `internal/core/certificate`: renewal in the window of Let's Encrypt's renewal
  information (ARI, RFC 9773), otherwise at two-thirds of the lifetime, replacing `renewBefore`'s
  fixed 30 days (§2.7, D290 §7, C1); the core's ACME account URI reported to the discovery for CAA
  and DNS-PERSIST-01 (§2.7); DNS-PERSIST-01 used once Let's Encrypt runs it in production, DNS-01
  through the discovery staying the fallback; the TXT record checked to resolve before the CA is
  asked, in place of the fixed 30 s; the record cleared by its path (K1); after leaving, the
  discovery's certificate no longer served (C16); a new certificate on re-key (K8c).
  `golang.org/x/crypto/acme` v0.57 has no ARI and no DNS-PERSIST-01 (checked: no `renewalInfo` in
  its source). ARI's lookup is an unauthenticated GET the standard library makes; a new order's
  `replaces` field and answering a `dns-persist-01` challenge may need ACME requests the package
  does not expose [I, verify]. Hand-writing them, as port mapping was, or a library is **the owner's
  decision** (rule 3).
- **Builds** §2.7; D290 §7; findings C1, C16, S12.
- **Tests.** Against the fake CA with renewal information: renewal falls in its window; a 20-day
  certificate is not reissued on every sweep; the CA is not asked before the record resolves; after
  leaving, the discovery's names are no longer served.
- **VQ:** a new item: against Let's Encrypt's staging, a core obtains and renews its wildcard
  through the paid service, by DNS-PERSIST-01 if offered.
- **Waits on:** K1; the dependency decision.

### K10 — Serving the client cross-origin

- **Changes.** `internal/core/api`: CORS for the discovery's client origins, a list, never a
  wildcard, no credentials mode, `Vary: Origin`, `null` never allowed (§3.4, §7 item 13). Byte paths
  authenticated by the header on every request (D289 §3): images and subtitles by `fetch` into blob
  URLs, hls.js's loader carrying the header (`web/packages/sdk`, `web/packages/player`), the media
  cookie (`media_cookie.go`) kept for the core's own origin only [I]. A `Host` allow list on the
  core's plain-HTTP LAN listener against DNS rebinding (§7 item 10, S15).
- **Builds** §3.4; D289 §3; D291 §6; findings S15, S18.
- **Tests.** A preflight from a configured origin is answered and one from another, or from `null`,
  refused; an answer carries `Vary: Origin`; a request on the LAN listener naming another `Host` is
  refused. A browser-tier test: a page on another origin plays a film and shows its poster with the
  header alone.
- **Waits on:** the client origin (see the questions).

### K11 — Versions, status and the exit path

- **Changes.** `internal/core/discovery`: the `major.minor` header; the deprecation's date kept and
  shown (built), and past the window the refusal's accepted versions shown (§5, C13); the account's
  state with its dates, and the discovery's messages for the admin (security-update notices, §7
  item 9) from `GET /status`; the lapse messages with dates and *Renew*, household members told the
  cause (§7 item 18, U7).
- **The exit path, tested** (D25; the paid spec §9.1 item 22): a fake discovery drives the core
  through `grace`, `frozen` and `released`; the core keeps serving its LAN and its own client, says
  what each state means, and names the way to an own discovery.
- **Builds** §5; the paid spec §3.10; D289 §5; D291 §6; findings C13, U7.
- **Tests.** The exit path's walk above; a deprecated discovery's date shows; an unknown minor is
  accepted.
- **Waits on:** K1, K2.

### K12 — The System page in plain words

- **Changes.** `web/apps/web/src/system-discovery.tsx`, `system-reachability.tsx` and their
  strings: *Remote access* as the name, the discovery's address behind "use my own" (§7 item 17,
  U5); "your server is reachable from the internet at …" shown prominently (§7 item 9, S14);
  `reachableFrom` by what was tested (§2.6); what a household nothing reaches can do, for the admin
  (§3.6, D290 §12); the connections `rtcapi` refused (K3) and the clients that failed to connect last
  week (§7 item 16); the holding account and the claim (K6); the fingerprint (K8a); the instance
  conflict and the restore without the key (K8d); the port-mapping cases (K5); the first-run setup
  code accepted in the browser (§7 item 17).
- **Builds** §2.6, §3.6; D290 §12; D291 §6; findings U3, U5, U7, S14.
- **Tests.** Component tests: each state renders its sentence and its action.
- **Waits on:** the steps whose state it shows.

### K13 — The owner's TURN, with credentials that expire

- **Changes.** The heartbeat's `turn` sent for the owner's TURN servers, with credentials minted by
  the core in the TURN REST shape (a username `<expiry>:<id>`, an HMAC under a secret shared with the
  TURN server) and refreshed with each heartbeat, never static ones (§2.4, C14). The core already
  mints such credentials for voice (`internal/core/turn/credentials.go`); this reuses them. The
  owner's TURN as one switch (§7 item 16).
- **Builds** §2.4; finding C14. Pairs with [webrtc-transport.md](webrtc-transport.md) W6.
- **Tests.** The heartbeat carries a credential that expires and verifies under the shared secret.
- **Waits on:** K1.

### K14 — The core's NAT, measured

- **Changes.** The core sends STUN binding requests to two STUN servers on different addresses (the
  discovery's and the probe machine's) and compares the mapped ports (RFC 5780): the same port is
  endpoint-independent mapping (§2.6, C10). `pion/stun/v3` is already allowed.
- **Builds** §2.6; finding C10.
- **Tests.** Against two STUN servers in the test, one mapping and two mappings are told apart.
- **Waits on:** K1; how the result reaches the discovery (see the questions).

### K15 — Device codes through the paid service

- **Changes.** `internal/core/auth` and `internal/core/discovery`: the core of the signed-in person
  who enters a code the service issued claims it from the service (§3.1, D288 §5); the person sees
  where the device asking is, from the service (D290 §5, S4); the check code's outcome and the
  session sealed to the device's X25519 key with HPKE (RFC 9180), the service passing on only the
  blob (D290 §5, S3). Go 1.26's standard library has `crypto/hpke`, so the core needs no new
  dependency [I, verify its suites against the browser's]. A code-activated device gets ordinary
  rights, never admin, until its person signs in with their password on it (D291 §5). The core
  limits how many codes it claims an hour (§7 item 2). The web client: the device's side on the
  `app.` origin (an ephemeral key pair, opening the sealed blob) and the ease-of-use items of U9 (a
  QR beside the code, a default device name, the check code picked from three, *Get a code*
  renamed, the two-sentence warning; §7 item 20). In the browser, HPKE is built from WebCrypto's
  X25519, HKDF and AES-GCM, or taken from a library: **the owner's decision** (rule 3).
- **Builds** §3.1; D288 §5; D290 §5; D291 §5, §6; findings S3, S4, S13, U9.
- **Tests.** The blob the fake service passes on opens only with the device's key; a session token
  never appears in what the service is sent (the `TestArch_` claim of K7); a code-activated device's
  session is refused an admin call until a password sign-in on it; past the hour's cap the core
  claims no more codes.
- **VQ:** a new item: a television's browser on `app.orpheum-app.com` asks for a code, a phone
  enters it, both show the check code, and the television plays.
- **Waits on:** K1, K7; the paid service's device-code calls.

### K16 — The dead-end counted

- **Changes.** The message of §3.6 written for whoever reads it (the web client, W4c), and its
  showing counted by the client's kind of network, as an aggregate with no identifiers (D290 §12,
  D274 §3).
- **Builds** §3.6; D290 §12; finding U3.
- **Waits on:** which side counts and by what call (see the questions).

## Found while planning

- **A core in a container sees the container's addresses**, not the host's: the bridge network's
  private ones. The heartbeat's `interface` candidates are only real under host networking. K4
  skips bridges; a containerised core then reports its `configured` addresses, and its discovery
  sees its source address (§2.4).
- **`newDiscovery` sets no `TURN`**, so the heartbeat never carries the owner's TURN; K13.
- **The server's keys were in every backup** until 2026-10-13: `internal/core/backup` left out
  `secret.key` alone, so the identity key and the certificates' and ACME account's private keys went
  into scheduled backups in plain text (the second audit's T1). Since fixed: a backup holds them only
  when asked for them, as D289 §2 says; K8a builds the rest of the identity key's rules.
- **No CORS on the core's API**, and byte paths lean on the media cookie (D216), which a page on
  another site never sends; K10.
- **IPv6 pinholes and high ports** (from
  [homelab-networks-2026-10-10.md](../research/homelab-networks-2026-10-10.md) §6, read against
  miniupnpd's source): miniupnpd refuses a pinhole unless asked over IPv6 from the address opened and
  for a port of 1024 or more; stock OpenWrt maps only 1024–65535 for IPv4. D291 §7's 443 first will
  meet that refusal there and take the port granted; K5 asks pinholes over IPv6.
- **miniupnpd maps nothing for IPv4 when its own WAN address is private**: every inner router of a
  double NAT. K5's external-address check says so rather than retrying.

## The beta's order of work

The closed beta reaches a few invited households from `app.orpheum-app.com` wherever they are, over
a direct HTTPS path where one exists and WebRTC through the service where none does (D290 §14), with
no payment, no public sign-up and one core per account (D290 §18). The work runs in lanes; a lane
starts when what it waits on is done.

**Owner's lane, started first: the long waits.** Register the zone's domain (see the questions);
file Let's Encrypt's rate-limit increase, which takes weeks (D290 §7); confirm that one Bunny script
record answers a whole wildcard subtree (the spec's §7.2 left it unverified) and whether Let's
Encrypt runs DNS-PERSIST-01 in production; open the Cloudflare, Hetzner, second-provider and mail
accounts; decide the dependencies the plans name (the ACME library, K9; HPKE in the browser, K15;
the geolocation database, P9). Outside these plans, and possibly longer than the service itself
(the spec's §10 risks): a deployment without the VPS tunnel (D272 §3) and a way for testers to
obtain the software ([beta-inventory-2026-10-09.md](../research/beta-inventory-2026-10-09.md)
(d) 2–3).

**Lane A, the wire: on the critical path.** discovery-core.md K1 with discovery-mode.md W4b.7, in
one change. Every other lane waits on it.

**Lane B, WebRTC through a discovery: the critical path.** It is the largest piece and the least
known (the spec's §10), and the beta's promise rests on it:

1. Core: K2 (sessions), then K3 (signalling bounded and checked), with K7's locators and K8b's
   proof, which K3's checks use.
2. W4b: W4b.3 (STUN) and W4b.4 (the client API), the reference every client test runs against.
3. Web: W2e (half-trickle), W2d (recovery and ICE restart), W2f (signed fingerprints), then W2c
   (choosing among the paths), then W4c (the client served by a discovery).
4. W4b.6 and W5: the browser-tier test through `single`.
5. Service: P6 and P7 on the same tests, P10 serving W4c's build.

**Lane C, direct paths and certificates, beside Lane B after K1:** K4 (addresses), K5 (port mapping),
K9 (certificates), K10 (cross-origin), K14 (NAT measurement); the service's P5 and P8.

**Lane D, claiming, identity and the System page, after K2:** K6 (claiming), K8a, K8c–K8e
(identity), K11 (versions and the exit path), K12 (the System page); the service's P2, P3, P4, P12.

**Lane E, the service's base, after K1:** P1, P11, P13; it needs nothing else from Orpheum.

**Last, and allowed to slip:** device codes through the service (K15, P9, W4c's device side). Without
them no television joins through `app.`; if they are late, testers are told a television must wait
(the spec's §10 item 4).

**Then, in order:** end to end on the owner's staging core; nas.example; one outside household; the
invited households.

**The critical path**: K1 → K2 → K3 (with K7, K8b) → W4b.4 → W2e, W2d, W2f → W2c → W4c → P6, P7,
P10 → staging → nas.example → an outside household → the invited ones; beside it, the owner's long waits
(the rate-limit increase, the domain, the testers' images), any of which can become the critical
path if started late. Not on it: the `orpheum discovery` command and its web serving (W4b.5, W4b.8),
which ship after the beta; billing; K13 (the owner's TURN), useful to a household with its own VPS
but not needed for the beta.

## Questions the plans raise

Not decided here; each is the owner's, or a fix to the protocol document.

1. **The heartbeat on the signalling connection, and the hourly probe.** D291 §6 takes the
   correctness audit's "something simpler" (protocol §7 item 22): the heartbeat carried on the
   signalling connection, `PUT /addresses` kept as the fallback, and the hourly probe replaced by
   probing on change and clients' reports. The protocol's §2.4 and §2.6 still describe a five-minute
   `PUT /addresses` and an hourly probe. K2 and K3 are written to §2.4 as it stands; which does the
   build follow?
2. **What a signature binds** (K1): §2.1 says "the discovery origin" and, in the same sentence, "the
   base URL the core itself called". The base URL carries a path (`/core/v1`); an origin does not.
3. **The enrolment's `protocol` field** (K1): §2.2 shows `POST /enrol {publicKey, protocol, …}`, while
   §5 says the body carries no version.
4. **When an instance stops being live** (K2): a discovery refuses a second `instanceId` per
   `coreId`. A core that restarts, or crashes without closing its connection, comes back with a new
   one while its old token still lives. What ends the old instance: its signalling connection's
   closing, its token, a time without heartbeat? Without a rule, a restart is refused as "another
   server".
5. **How the core learns the discovery's client origin** (K7, K10): links take the form
   `https://<discovery>/register#…`, and CORS allows "the configured client origins", but the core
   knows only the discovery's API base URL (`https://api.orpheum-app.com/core/v1`). A field in
   `GET /status`, or a setting?
6. **The claim page's address, and a typed fallback** (K6): §2.2 has the core open
   `https://account.orpheum-app.com/claim#<token>`, but not whether the URL comes from the
   enrolment's answer or is fixed in the core. And §7 item 5 asks "whether a typed claim code stays
   as a fallback"; D291 §6 takes the audit's recommendation, which keeps it (security audit S7 item
   2). Does the paid enrolment then return both a code and a token?
7. **Matching an ICE restart to its connection** (K3, W2d): a restart's offer goes "through the
   discovery (or over the data channel while it lives)", but nothing in §2.5 or §4 names which
   existing connection an offer restarts.
8. **Where the NAT measurement goes** (K14): the core compares the mapped ports, yet "the heartbeat's
   answer carries the result". Does the core report its finding in the heartbeat (a new field), or
   do the discovery's STUN servers observe it, and if so how do they know the requests are the core's?
9. **Counting failures** (K12, K16): D290 §12 counts how often the dead-end message is shown, and §7
   item 16 has the System page say how many clients failed to connect last week. A client that
   failed reached neither the core nor, by any call in §4, the discovery. Which side counts, and by
   what call?
10. **Ordinary rights for which codes** (K15): D291 §5 gives a code-activated device ordinary rights.
    The protocol places it under the paid tier's codes; does it apply to the core's own codes
    (D288, built, used on an own discovery and at home) too?
11. **The Watch Together link's path** (K7): §3.1 has `https://<discovery>/join#<coreId>.<secret>`;
    the built client makes `/together#<secret>`, with no "the code must change" marker.
12. **The remembered paths' key** (W2c): §3.3 item 7 keys remembered paths on "the client address
    prefix the discovery reports", but §3.2's `paths` answer has no field carrying it.
13. **The bootstrap token** (K3, K7): §3.1 calls it "short-lived and single-use" and says it "serves
    `/paths` and offers for its few minutes". Is it spent once by `/locate`, then usable for its
    minutes, or spent by its first use?
14. **First contact over WebRTC alone** (K8b, W2f): the client learns the core's key "over a direct
    path". For a household only WebRTC through the discovery reaches, the key and the signed
    fingerprint both arrive through the discovery on a first visit. Should registration, Watch
    Together and own links carry the key's fingerprint, as the pairing QR carries the DTLS one?
15. **What the sealed blob carries** (K15): the session, and "the check code's outcome". Does it also
    carry the core's id and a device locator, so the device can find its core afterwards, and is the
    check code itself sealed, so the service never sees it?
16. **Certificate Transparency's source** (K8e): the core must ask some log or monitor (crt.sh, or a
    log's own API) for its names; that third party learns which core asks. The protocol names none.
17. **The ACME library** (K9): ARI's `replaces` and DNS-PERSIST-01 beyond `golang.org/x/crypto/acme`
    v0.57: hand-written, or a library (a dependency, the owner's).
