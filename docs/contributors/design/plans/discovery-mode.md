> **ABOUTME:** W4b: `orpheum discovery`, the single-target discovery a self-hoster runs on a VPS
> for their one core (D273 §3, D274 §5), built as public packages the closed multi-core service
> imports (D275 §1). The steps, revised to the protocol as decided after three rounds of audits
> (D290, D291, D292) and the questions they left (D293), and what waits on the owner.

# W4b: the single-target discovery

- **Status:** IN-PROGRESS — begun 2026-10-11 on the owner's word (*"let's move on with the
  milestone"*), the next phase of [webrtc-transport.md](webrtc-transport.md) after W4a. W4b.1 and
  the package of W4b.2 are built; the steps were revised 2026-10-13 to
  [discovery-protocol.md](../discovery-protocol.md) as D290, D291 and D292 left it. Since then
  W4b.7 is built but for the conformance suite (2026-10-13, Phase 1 of
  [discovery-core.md](discovery-core.md#the-order-of-work)), W4b.4 in part and W4b.6 (2026-10-14,
  Phase 2's thin path). W4b.3 is Phase 3. The command and its web serving (W4b.8, W4b.5) come
  before the open beta, possibly during the closed one (D313 §7), and enrol the core as the paid
  service does: the discovery's command line gives a key, the core is pointed at the household's
  domain, and its admin enters the key at home (D310 §7), in place of `orpheum discovery claim
  <code>`.
- **Depends on:** D273 §3 (the owner's own discovery, one core), D274 §5 (it has the paid tier's
  shape: it serves the web app, taking the files from the core; it claims its core by
  `orpheum discovery claim <code>`; it relays by TURN), D275 §1 (public building blocks, the closed
  service assembling them for many cores), D290, D291, D292,
  [discovery-core.md](discovery-core.md) (the core's side; its K1 changes the wire this package
  answers), and [discovery-protocol.md](../discovery-protocol.md), whose §2 and §3 it answers for
  one core. D274 §5 claimed the core "by the paid tier's flow"; the paid flow later changed, so the
  own discovery's code stands on its own (protocol §2.2; N8), and in the beta the paid service
  claims by a code too (D292 §3). D274 §5's tunnel that "becomes optional, and later goes" stays as
  a household's `configured` address with its TURN while it is a household's IPv4 route (protocol
  §2.4; N11).

## The shape

- **Public packages under `discovery/`** at the module's root, not `internal/` (D275 §1), so the
  closed service imports them. Each has a `doc.go` with the same headings as every package's, and
  the gates that walk `cmd/` and `internal/` walk `discovery/` too.
  - `discovery/discoveryproto`: the wire types the core and every discovery speak.
  - `discovery/proof`: what a signature is over, and its check (protocol §2.1).
  - `discovery/safe32code`: the claim code's and the device code's shape.
  - `discovery/single`: the core-facing protocol answered for one core (challenge, enrol, session,
    status, addresses, signal, acme-challenge, leave) and the client-facing one (locate, paths,
    offer; protocol §3, §4), over one file of its own, since it holds one core.
  - `discovery/discoverytest`: the conformance suite, a function taking an `http.Handler` and a
    clock that runs the protocol's cases, which `single`'s tests and the paid service's handler
    tests both run, so the free and paid servers cannot drift (protocol §5; devloop §3.2, D292 §9).
- **`orpheum discovery`** is a mode of the one binary (`cmd/orpheum`), as core and node are: it
  serves `discovery/single` over HTTPS, runs STUN, and ~~`orpheum discovery claim <code>` binds the
  core whose code it is~~ gives the key its core enrols by (D310 §7).
- **Nothing for many cores in Orpheum** (D275 §1): accounts, billing, names under a shared zone,
  the outside probe machine and rate limits across customers are the closed service's
  (`orpheum-connect`, D292 §8), planned in its own repository.
- **`single` is the reference.** The core's client is tested against it (protocol §5), the web
  client's discovery paths are tested against it (W4b.6, W5), and the paid service runs the same
  conformance suite against its handlers. So its client API is on the beta's critical path even
  though the command and its VPS guide ship ~~after the beta~~ before the open beta (D313 §7).
  ~~In the beta, a self-hoster's route is the VPS tunnel as built (`deploy/vps/`) [I, coherence
  §4.2].~~ In the closed beta every household is on the paid service, and none sets up a VPS
  (D310 §1).

## Steps

| Step | What | State |
| --- | --- | --- |
| W4b.1 | The public tree: `discovery/discoveryproto` (moved from `internal/discoveryproto`) and `discovery/proof`, the core signing with it; the architecture tests that load `cmd/...` and `internal/...` load `discovery/...` too, and `TestArch_PublicPackagesImportNothingInternal` holds the tree free of `internal/` | BUILT 2026-10-11 |
| W4b.2 | `discovery/single`, the core-facing protocol for one core against the core's real client (`internal/core/discovery`); the state in one file | the package BUILT 2026-10-11 (with `discovery/safe32code` moved out of `internal/core/auth`): enrolment, claiming (`single.Claim`), sessions, status, heartbeat, the certificate's record through a DNS provider when given one, leaving, the core's signalling; nonces carrying their own expiry and MAC since an earlier commit (D290 §15). Its takeover flaws, code K13–K15, closed 2026-10-13 (A3; W4b.7). The command moved to W4b.8 |
| W4b.7 | `single` brought to the revised protocol, its takeover flaws closed, and the conformance suite (below) | built 2026-10-13 (Phase 1, with discovery-core.md's K1), but for the conformance suite `discovery/discoverytest`, which does not exist yet |
| W4b.4 | The client-facing protocol: locate, paths and offer for the one core (below) | built in part 2026-10-14 (Phase 2): locate, paths, offers and the dead-end report; the `lan` path and rate limits on strangers wait |
| W4b.6 | An end-to-end test: a core, its discovery and a browser, the browser reaching the core through the discovery (below); then the owner's real run (W5) | built 2026-10-14: `web/e2e/through-discovery.spec.ts`, against `orpheum dev discovery` (development builds only), in Chromium and Firefox; the Watch Together socket and ICE restart wait |
| W4b.3 | STUN: the discovery's `iceServers` point at its own STUN server (below) | Phase 3; the thin path uses any public STUN server |
| W4b.5 | Serving the web app at the owner's domain, its files taken from the core (D274 §5) | before the open beta (D310 §1, D313 §7); waits on Q3 |
| W4b.8 | The command: `orpheum discovery`, ~~`orpheum discovery claim <code>`~~ a core's enrolment key (D310 §7), `orpheum discovery rekey` | before the open beta (D310 §1, D313 §7); waits on Q5 |

The table is in the order of work; the step numbers stay as other documents cite them (the
protocol's §2.6 cites W4b.4).

### W4b.7 — `single` brought to the revised protocol

Changes in `discovery/single`, landing with [discovery-core.md](discovery-core.md) K1, since the
core's tests run against it. **First, the takeover flaws** (D292 §4: closed before any signalling
connection opens; A3, code K13–K15, T13). **Built 2026-10-13**, with the refusals the core acts on
(`discoveryproto.Refusal`: `enrol_again`, `another_instance`, `not_claimed`), the ping rule and
leaving below; the instance is bound through the live instance a token was opened by:

- **Tokens bound to the core**: each session token stored with `(coreId, public key, instanceId)`
  and checked against the record's core on every call; every token dropped when the record's core
  changes; signalling accepted only for the claimed core's token (protocol §2.2). Today the token
  map held only an expiry and `accept` replaced the connection for any valid token.
- **A stranger cannot displace a pending core**: while a pending core's code is live, another key's
  enrolment is refused with 409, and the pending core is replaced only once its code expired;
  `enrol` replaced a pending core with any key that signed (protocol §2.2, C12, S18). A
  refusal that means "enrol again" carries its error code (K1).
- **Nothing before claiming**: `PUT /acme-challenge`, `PUT /addresses` and the signalling
  connection refused until the core is claimed; `acmeChallenge` checked only the name.
- **A 130-bit `coreId`**: 26 safe32 characters, random, never reused for another key; `coreIDChars`
  made 8 (protocol §2.7, D290 §13).

Then the rest of the revised protocol:

- **Sessions**: `expiresIn`; one live `instanceId` per core (protocol §2.3, D290 §18, C4). **A
  second instance pings the first** (D293 §1): `single` pings the old instance on its signalling
  connection and waits about 10 seconds; no answer, or no connection, and the newcomer replaces it;
  an answer, and the newcomer is refused by the named error. The token map pruned of expired tokens
  (built 2026-10-13, with the ping rule), and enrolments bounded per source site (C11, code K17).
- **The record written atomically**: a temporary file at 0600, synced, renamed over the record, so
  a crash mid-write leaves it readable (code K18) [I].
- **Liveness by the signalling connection**: a core whose connection closed is offline at once, and
  one with none and no heartbeat for 30 minutes too (protocol §2.4, C16).
- **Leaving closes the signalling connection and ends the tokens** at once (protocol §2.5, §2.8,
  C4; built 2026-10-13); so does revocation.
- **One version signal**: the header's `major.minor`, any minor of major 1 accepted, `/signal`
  checking it too, no version in the enrolment's body (protocol §5, C13, X11).
- **The TXT record cleared by its path** (protocol §2.7, C16), its value checked to be a
  43-character base64url digest and cleared after an hour (§7 item 13, S18).
- **Signalling**: offers carrying `locatorHash` and their proof, several in one message, limited per
  locator (at most 8 offers or connections) and per core (protocol §2.5, D290 §6, S5); loopback and
  link-local remote candidates dropped and about ten candidates at most (§7 item 11, S16), as a
  first filter before the core's own check (K3a); the heartbeat carried on the connection, with
  `PUT /addresses` its fallback (protocol §2.4).
- **Logging** device requests without the core's id, or with a truncated address (§7 item 12, S17).
- **`discovery/discoverytest`**, the conformance suite, which `single`'s tests run (devloop §3.2).
- **Deferred** (D292 §3): the re-key, rotation and recovery by the command (protocol §2.8).

**Tests.** A stale token from a replaced core is refused on every call and on the signalling
connection; the owner's pending core is never shown another key's code; a second instance is
accepted when the first is silent or unconnected and refused while the first answers its ping; a
pending core's `acme-challenge`, addresses and signalling are refused; a second key's enrolment is
refused while the pending core's code lives, and accepted after it expired. A core's id is 26
characters. An expired token is pruned. A closed signalling connection marks the core offline. After
`DELETE /core` its token is refused and its connection closed. A header of `1.3` is accepted and
`2.0` refused with the versions accepted. A record interrupted mid-write still loads. The existing
`TestEnrol_HoldsOneCoreAndRefusesAnotherWhileItIsClaimed` and
`TestCore_ACoreEnrolsIsClaimedAndSendsItsAddresses` keep passing on the new wire.

### W4b.4 — The client-facing protocol

**Partly built 2026-10-14:** `ClientHandler` answers `/locate` (use policy, life, a five-minute
bootstrap token) and `/paths` (locator or bootstrap token; `state` from the signalling connection,
`offlineSince`; `clientPrefix` /64 or /32; WebRTC alone); `PUT /locate-hashes` and `PUT /locators`
kept since K7; `/cores/{id}/offer`, each offer passed on with the hash it came under and its proof,
four to a request, eight open for one hash; the dead-end report, counted over a week, no device
identifier kept. **Still:** the `lan` path; rate limits on strangers.

`single` answers the client API for its one core (protocol §3, §4), with the hashes kept (Q4):

- `POST /client/v1/locate`: a link's hash, checked against the set the core registered with its
  use policy (`PUT /locate-hashes`), answers the paths and a bootstrap token, usable for its
  minutes and never renewed (protocol §3.1, C3, X10).
- `POST /client/v1/paths`: a locator or a bootstrap token answers the paths, the ICE servers, the
  core's API major, its `state` (D293 §6) and the client's `clientPrefix` (D293 §8); the `lan` path
  only when the device seems to be at home (protocol §3.2, D290 §2), never on a match with a shared
  IPv4 (G5).
- The dead-end report, counted weekly per core with no device identifier and returned in the
  heartbeat's answer (protocol §3.6, D293 §7); `GET /status`'s `clientOrigin`, the owner's domain
  (protocol §3.4, D293 §4).
- `POST /client/v1/cores/{id}/offer`: up to four offers, each with its proof, passed to the core
  over its signalling connection with the locator's hash, the signed answers returned (protocol
  §2.5).
- `PUT /locate-hashes` and `PUT /locators` on the core-facing side, the full sets.
- The paths themselves: the core's candidates as names under the owner's zone, once direct names
  are built (Q2); until then WebRTC alone (D274 §5: W4b is WebRTC first), and the `published`
  answer of the heartbeat empty, as built.
- **Device codes** on an own discovery come from the core itself (protocol §3.1, D288 §5); how a
  device with no link reaches the core to ask is Q6.

**Tests.** A hash the core never registered locates nothing; an unused registration link's hash
locates again and again, and once used, as the person's own link; a Watch Together link's while it
lives; a locator dropped from the core's set gets no paths; an offer reaches the core with the
locator's hash and its proof and its signed answer comes back; a ninth open offer for one locator is
refused; the `lan` path is returned to a request from the core's /64 and not to one from elsewhere.

### W4b.6 — End to end

A browser-tier test (Playwright, as `web/` already runs): a core, `single` and a browser in one run;
the browser opens a registration link through the discovery, checks the fingerprint it carries,
locates the core, reaches it over WebRTC through the discovery's offer, registers, and plays a
film's first chunks. It is the first place all public parts meet, the test the web client's W4c and
W2c–W2f are built against ([webrtc-transport.md](webrtc-transport.md)), and Orpheum's one discovery
browser test in check-pr (devloop §5). Then the owner's own run on staging (W5 there).

### W4b.3 — STUN

The discovery's `iceServers` in its status answer and its `paths` answer name its own STUN server
(protocol §2.9). It answers Binding requests only, rate-limited per source, without `SOFTWARE`
(§7 item 11, S16). The embedded TURN relay pion already gives the binary answers STUN; whether it
can be held to Binding only, or a small server is needed, is found when built; if a server, on the
`pion/stun` that `pion/webrtc/v4` already brings, so the binary carries one STUN library (G10) [I].
**Tests:** a Binding request is answered with the source's mapped address; any other method is not;
a source past its rate is not.

### W4b.5 — Serving the web app (before the open beta)

The discovery serves the core's web client at the owner's domain, its files taken from the core over
the core's own outbound connection, so their version is always the core's (D274 §5); one build per
core API major, the one the core reports (D289 §4). The protocol has no call for it yet (Q3).
**Tests:** the discovery serves the files the core sent, and a new core version's files replace
them.

### W4b.8 — The command (before the open beta)

> **Revised by D310 §7 (2026-10-18):** the core enrols by a key the command gives, which its admin
> enters at home, as on the paid service; the claim by the core's code below is replaced.

`orpheum discovery` serves `single` (W4b.7, W4b.3, W4b.4, W4b.5); ~~`orpheum discovery claim <code>`
binds the core whose code it is and prints the fingerprint it binds~~ the command gives the core's
enrolment key, and the core is pointed at the household's domain (D310 §7) (protocol §2.2 item 2; T13;
`single.Claim` exists, used by its tests only); `orpheum discovery rekey` re-keys after a lost key
once re-keying is built (protocol §2.8, C4; deferred). How it is reached and configured is Q5.
**Tests:** ~~the claim command binds a pending core by its code, prints its fingerprint, and refuses a
wrong or expired one, saying which~~ to be rewritten for the key (D310 §7).

## Questions for the owner

The questions of 2026-10-11, marked where D289–D292 or the protocol now answer them.

1. **Where the public packages live.** *Answered*: `discovery/` at the module's root, as W4b.1 built
   them; the protocol names `discovery/discoveryproto`, `discovery/proof`, `discovery/safe32code`
   and `discovery/single` as what the core, an own discovery and the closed service speak (protocol
   status, D275 §1).
2. **Names and the certificate on the owner's domain.** *Partly answered*: the names are the owner's
   domain's, set through the owner's DNS provider (protocol §2.7), on a domain that holds nothing
   else the owner values [I, infrastructure §2], and direct HTTPS names wait, since W4b is WebRTC
   first (D274 §5). The paid service writes a record per published address in the beta (D292 §1),
   so an own discovery can do the same at any provider with an API, Cloudflare included. *Still
   open*: whether a core's names are `<coreId>.<owner's zone>` as on the paid tier or one name the
   owner chooses, and which providers.
3. **"Taking the files from the core."** *Partly answered*: D274 §5 says over the core's own
   outbound connection. *Still open*: the protocol has no call that carries them (a message on the
   signalling connection, or a call the core makes with its build), and which of the client's
   integrity measures (protocol §3.4) apply to an own discovery's copy too.
4. **Locating on a single-target discovery.** *Answered*: keep the hashes. An own discovery answers
   the same client API for its one core (protocol §4), the core registering its link hashes and
   locators (§3.1), and a person's own link and Watch Together links work through it as through the
   paid service (D290 §10, §17).
5. **How `orpheum discovery` runs on the VPS.** *Still open*: its own certificate for its name (by
   ACME, through the same DNS provider as Q2), or plain HTTP on the loopback behind a reverse proxy
   or the tunnel the VPS already runs; and its settings as a config file in the core's form
   (`orpheum default-config --discovery`) or flags.

## Questions the plans raise

6. **A device with no link, on an own discovery** (W4b.4): D288 §5 has the VPS "pass the request
   for a code" to its core, and Q4 keeps the core's paths behind hashes. A device that holds no link
   cannot locate the core to ask it for a code. Does the discovery give paths without a locator for
   this one purpose (an unproven connection, which may ask for device codes, protocol §2.5; G4), or
   pass the request over the signalling connection, which would carry a request the discovery
   otherwise never carries? The core keys its code limits by the asking device's site, so on this
   path it must see the device's address, not the VPS's (G6).
7. **NAT measurement on one VPS** (W4b.3): deferred with the paid tier's (D292 §3). When built it
   needs two STUN servers on different addresses (§2.6); a VPS usually has one IPv4 address, so an
   own discovery may be unable to tell the core's mapping, and `reachableFrom` says less. Is that
   accepted for an own discovery?
8. **The instance rule's corner** is the same as discovery-core.md's question 4: when a restarted
   core's new `instanceId` is accepted while its old token lives. *Answered* by D293 §1: the
   discovery pings the old instance and waits about 10 seconds; no answer, or no connection, and the
   new one replaces it at once (W4b.7).
