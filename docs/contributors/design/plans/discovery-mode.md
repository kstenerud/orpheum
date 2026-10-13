> **ABOUTME:** W4b: `orpheum discovery`, the single-target discovery a self-hoster runs on a VPS
> for their one core (D273 §3, D274 §5), built as public packages the closed multi-core service
> imports (D275 §1). The steps, revised to the protocol as decided after the audits (D290, D291),
> and what waits on the owner.

# W4b: the single-target discovery

- **Status:** IN-PROGRESS — begun 2026-10-11 on the owner's word (*"let's move on with the
  milestone"*), the next phase of [webrtc-transport.md](webrtc-transport.md) after W4a. W4b.1 and
  the package of W4b.2 are built; the steps were revised 2026-10-13 to
  [discovery-protocol.md](../discovery-protocol.md) as D290 and D291 left it, and are not started:
  the owner asked that building wait.
- **Depends on:** D273 §3 (the owner's own discovery, one core), D274 §5 (it has the paid tier's
  shape: it serves the web app, taking the files from the core; it claims its core by
  `orpheum discovery claim <code>`; it relays by TURN; the tunnel becomes optional), D275 §1 (public
  building blocks, the closed service assembling them for many cores), D290, D291,
  [discovery-core.md](discovery-core.md) (the core's side; its K1 changes the wire this package
  answers), and [discovery-protocol.md](../discovery-protocol.md), whose §2 and §3 it answers for
  one core.

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
- **`orpheum discovery`** is a mode of the one binary (`cmd/orpheum`), as core and node are: it
  serves `discovery/single` over HTTPS, runs STUN, and `orpheum discovery claim <code>` binds the core
  whose code it is.
- **Nothing for many cores in Orpheum** (D275 §1): accounts, billing, names under a shared zone,
  the outside probe machine and rate limits across customers are the closed service's, planned in
  its own repository.
- **`single` is the reference.** The core's client is tested against it (protocol §5), the web
  client's discovery paths are tested against it (W4b.6, W5), and the paid service runs its own
  handlers against the same tests. So its client API is on the beta's critical path even though the
  command and its VPS guide ship after the beta (the paid spec §10).

## Steps

| Step | What | State |
| --- | --- | --- |
| W4b.1 | The public tree: `discovery/discoveryproto` (moved from `internal/discoveryproto`) and `discovery/proof`, the core signing with it; the architecture tests that load `cmd/...` and `internal/...` load `discovery/...` too, and `TestArch_PublicPackagesImportNothingInternal` holds the tree free of `internal/` | BUILT 2026-10-11 |
| W4b.2 | `discovery/single`, the core-facing protocol for one core against the core's real client (`internal/core/discovery`); the state in one file | the package BUILT 2026-10-11 (with `discovery/safe32code` moved out of `internal/core/auth`): enrolment, claiming (`single.Claim`), sessions, status, heartbeat, the certificate's record through a DNS provider when given one, leaving, the core's signalling; nonces carrying their own expiry and MAC since an earlier commit (D290 §15). The command moved to W4b.8 |
| W4b.7 | `single` brought to the revised protocol (below) | first of the remaining steps, with discovery-core.md's K1 |
| W4b.3 | STUN: the discovery's `iceServers` point at its own STUN server (below) | |
| W4b.4 | The client-facing protocol: locate, paths and offer for the one core (below) | |
| W4b.6 | An end-to-end test: a core, its discovery and a browser, the browser reaching the core through the discovery (below); then the owner's real run (W5) | |
| W4b.5 | Serving the web app at the owner's domain, its files taken from the core (D274 §5) | waits on Q3 |
| W4b.8 | The command: `orpheum discovery`, `orpheum discovery claim <code>`, `orpheum discovery rekey` | waits on Q5 |

The table is in the order of work; the step numbers stay as other documents cite them (the
protocol's §2.6 cites W4b.4).

### W4b.7 — `single` brought to the revised protocol

Changes in `discovery/single`, landing with [discovery-core.md](discovery-core.md) K1, since the
core's tests run against it:

- **A 130-bit `coreId`**: 26 safe32 characters, random, never reused for another key; `coreIDChars`
  makes 8 (protocol §2.7, D290 §13).
- **A stranger cannot displace a pending core**: while a pending core's code is live, another key's
  enrolment is refused with 409; today `enrol` replaces a pending core with any key that signs
  (protocol §2.2, C12, S18). A refusal that means "enrol again" carries its error code (K1).
- **Sessions**: `expiresIn`; one live `instanceId` per core, a second refused by the named error
  (protocol §2.3, D290 §18, C4); the token map pruned of expired tokens (C11).
- **Liveness by the signalling connection**: a core whose connection closed is offline at once, and
  one with none and no heartbeat for 30 minutes too (protocol §2.4, C16).
- **Leaving closes the signalling connection and ends the tokens** at once (protocol §2.5, §2.8, C4);
  so do re-keying and revocation.
- **One version signal**: the header's `major.minor`, any minor of major 1 accepted, `/signal` checking
  it too (protocol §5, C13).
- **The TXT record cleared by its path** (protocol §2.7, C16), its value checked to be a
  43-character base64url digest and cleared after an hour (§7 item 13, S18).
- **The re-key**: rotation, the new key's enrolment countersigned by the old, taking effect at once;
  recovery on the VPS by the command (W4b.8) (protocol §2.8, D290 §3, C4).
- **Signalling**: offers carrying `locatorHash`, up to four in one message, limited per locator (at
  most 8 offers or connections) and per core (protocol §2.5, D290 §6, S5); loopback and link-local
  remote candidates dropped and about ten candidates at most (§7 item 11, S16).
- **Logging** device requests without the core's id, or with a truncated address (§7 item 12, S17).

**Tests.** A second key's enrolment is refused while the pending core's code lives, and accepted
after it expired. A core's id is 26 characters. An expired token is pruned. A closed signalling
connection marks the core offline. After `DELETE /core` its token is refused and its connection
closed. A header of `1.3` is accepted and `2.0` refused with the versions accepted. A countersigned
re-key keeps the `coreId` and one signed by the new key alone is refused. The existing
`TestEnrol_HoldsOneCoreAndRefusesAnotherWhileItIsClaimed` and
`TestCore_ACoreEnrolsIsClaimedAndSendsItsAddresses` keep passing on the new wire.

### W4b.3 — STUN

The discovery's `iceServers` in its status answer and its `paths` answer name its own STUN server
(protocol §2.9). It answers Binding requests only, rate-limited per source, without `SOFTWARE`
(§7 item 11, S16). The embedded TURN relay pion already gives the binary answers STUN; whether it
can be held to Binding only, or a small server on `pion/stun/v3` (allowed) is needed, is found when
built [I]. **Tests:** a Binding request is answered with the source's mapped address; any other
method is not; a source past its rate is not.

### W4b.4 — The client-facing protocol

`single` answers the client API for its one core (protocol §3, §4), with the hashes kept (Q4):

- `POST /client/v1/locate`: a link's hash, checked against the set the core registered with its
  use policy (`PUT /locate-hashes`), answers the paths and a bootstrap token (protocol §3.1, C3).
- `POST /client/v1/paths`: a locator or a bootstrap token answers the paths, the ICE servers and the
  core's API major; the `lan` path only when the device seems to be at home (protocol §3.2, D290 §2).
- `POST /client/v1/cores/{id}/offer`: up to four offers passed to the core over its signalling
  connection with the locator's hash, the signed answers returned (protocol §2.5).
- `PUT /locate-hashes` and `PUT /locators` on the core-facing side, the full sets.
- The paths themselves: the core's candidates as names under the owner's zone, once direct names
  are built (Q2); until then WebRTC alone (D274 §5: W4b is WebRTC first), and the `published`
  answer of the heartbeat empty, as built.
- **Device codes** on an own discovery come from the core itself (protocol §3.1, D288 §5); how a
  device with no link reaches the core to ask is a question below.

**Tests.** A hash the core never registered locates nothing; a registration link's hash locates
once and a Watch Together link's while it lives; a locator dropped from the core's set gets no
paths; an offer reaches the core with the locator's hash and its signed answer comes back; a ninth
open offer for one locator is refused; the `lan` path is returned to a request from the core's
public address and not to one from elsewhere.

### W4b.6 — End to end

A browser-tier test (Playwright, as `web/` already runs): a core, `single` and a browser in one run;
the browser opens a registration link through the discovery, locates the core, reaches it over
WebRTC through the discovery's offer, registers, and plays a film's first chunks. It is the test the
web client's W4c and W2d–W2f are built against ([webrtc-transport.md](webrtc-transport.md)). Then
the owner's own run on a VPS (W5 there).

### W4b.5 — Serving the web app

The discovery serves the core's web client at the owner's domain, its files taken from the core over
the core's own outbound connection, so their version is always the core's (D274 §5); one build per
core API major, the one the core reports (D289 §4). The protocol has no call for it yet (Q3).
**Tests:** the discovery serves the files the core sent, and a new core version's files replace
them.

### W4b.8 — The command

`orpheum discovery` serves `single` (W4b.7, W4b.3, W4b.4, W4b.5); `orpheum discovery claim <code>`
binds the core whose code it is (protocol §2.2 item 4; `single.Claim` exists, used by its tests
only); `orpheum discovery rekey` re-keys after a lost key (protocol §2.8, C4). How it is reached and
configured is Q5. **Tests:** the claim command binds a pending core by its code and refuses a wrong
or expired one, saying which; the re-key command binds a new key to the `coreId`.

## Questions for the owner

The questions of 2026-10-11, marked where D289–D291 or the protocol now answer them.

1. **Where the public packages live.** *Answered*: `discovery/` at the module's root, as W4b.1 built
   them; the protocol names `discovery/discoveryproto`, `discovery/proof`, `discovery/safe32code`
   and `discovery/single` as what the core, an own discovery and the closed service speak (protocol
   status, D275 §1).
2. **Names and the certificate on the owner's domain.** *Partly answered*: the names are the owner's
   domain's, set through the owner's DNS provider (protocol §2.7), and direct HTTPS names wait, since
   W4b is WebRTC first (D274 §5). *Still open*: whether a core's names are `<coreId>.<owner's zone>`
   as on the paid tier or one name the owner chooses, and which providers. The paid tier computes
   answers from the name (D290 §7) and Cloudflare cannot; an own discovery holding one core would
   write a record per address instead [I].
3. **"Taking the files from the core."** *Partly answered*: D274 §5 says over the core's own outbound
   connection. *Still open*: the protocol has no call that carries them (a message on the signalling
   connection, or a call the core makes with its build), and whether D290 §4's integrity measures
   (a reproducible build, Subresource Integrity, a strict content-security policy) apply to an own
   discovery's copy too.
4. **Locating on a single-target discovery.** *Answered*: keep the hashes. An own discovery answers
   the same client API for its one core (protocol §4), the core registering its link hashes and
   locators (§3.1), and a person's own link and Watch Together links work through it as through the
   paid service (D290 §10, §17).
5. **How `orpheum discovery` runs on the VPS.** *Still open*: its own certificate for its name (by
   ACME, through the same DNS provider as Q2), or plain HTTP on the loopback behind a reverse proxy or
   the tunnel the VPS already runs; and its settings as a config file in the core's form
   (`orpheum default-config --discovery`) or flags.

## Questions the plans raise

6. **A device with no link, on an own discovery** (W4b.4): D288 §5 has the VPS "pass the request
   for a code" to its core, and Q4 keeps the core's paths behind hashes. A device that holds no link
   cannot locate the core to ask it for a code. Does the discovery give paths without a locator for
   this one purpose (an unproven connection, limited to pairing, as K3 of discovery-core.md
   describes), or pass the request over the signalling connection, which would carry a request the
   discovery otherwise never carries?
7. **NAT measurement on one VPS** (W4b.3): the protocol's measurement needs two STUN servers on
   different addresses (§2.6). A VPS usually has one IPv4 address; an own discovery may then be unable
   to tell the core's mapping, and `reachableFrom` says less. Is that accepted for an own discovery?
8. **The instance rule's corner** is the same as discovery-core.md's question 4: when a restarted
   core's new `instanceId` is accepted while its old token lives.
