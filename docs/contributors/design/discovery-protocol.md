> **ABOUTME:** The discovery protocol: how a core, its clients and a discovery speak, the same for
> the paid service at app.orpheum-app.com (many cores) and an owner's own `orpheum discovery` (one
> core). Moved here 2026-10-13 from the paid service's spec, which keeps billing, accounts, abuse
> and its architecture. Revised the same day for the owner's rulings on three rounds of audits
> (D290, D291, D292) and on the questions they left open (D293).

# The discovery protocol

**Status:** a design, partly built; **the specification the code follows** from 2026-10-13, when
the third round's rulings (D292) were folded in. Moved into the repository on the owner's word
(2026-10-13: *"move the protocol parts into the repo"*) out of the paid service's specification
(its draft 4 moved it; the specification is kept in the paid service's own repository,
`orpheum-connect`, D292 §8), whose §3, §4, §8 and §9.3 it was. It is what the core
(`internal/core/discovery`), Orpheum's public building blocks (`discovery/discoveryproto`,
`discovery/proof`, `discovery/safe32code`, `discovery/single`) and the closed service all speak
(D275 §1). The plans that build it are [discovery-core.md](plans/discovery-core.md) (the core's
side, and the order of work), [discovery-mode.md](plans/discovery-mode.md) (W4b, the owner's own
discovery) and [webrtc-transport.md](plans/webrtc-transport.md) (the client's paths).

**Revised 2026-10-13, three times**, before any development, on the owner's request:

1. After four audits: [security](research/discovery-audit-security-2026-10-13.md) (findings
   S1–S18), [ease of use](research/discovery-audit-ux-2026-10-13.md) (U1–U13),
   [correctness](research/discovery-audit-correctness-2026-10-13.md) (C1–C16) and DDoS and
   availability (kept with the paid service). The owner ruled in
   [D290](../decisions/working-notes.md#d290--the-discovery-audits-findings-taken-claiming-on-the-core-paths-ranked-by-kind-re-keying-signed-by-the-old-key-computed-dns-behind-cloudflare-and-webrtc-in-the-beta)
   and
   [D291](../decisions/working-notes.md#d291--what-the-discovery-audits-raised-and-d290-left-open-taken-as-the-audits-recommend-the-probe-on-a-machine-of-its-own-dns-on-bunny-ipv6-before-ipv4)
   (§6, §7).
2. After a second round of five: [consistency](research/discovery-audit2-consistency-2026-10-13.md)
   (X1–X22, R1–R8, F1–F18, G1–G14), [the
   code](research/discovery-audit2-security-code-2026-10-13.md) (written *code K1–K20*, to tell them
   from discovery-core.md's steps), [trust](research/discovery-audit2-security-trust-2026-10-13.md)
   (T1–T14), [the web](research/discovery-audit2-security-web-2026-10-13.md) (W1–W14) and
   [ease of use](research/discovery-audit2-ux-2026-10-13.md) (U2-1–U2-13).
3. After a third round of four: [security](research/discovery-audit3-security-2026-10-13.md)
   (A1–A17), [coherence](research/discovery-audit3-coherence-2026-10-13.md) (N1–N11),
   [the development loop](research/discovery-devloop-2026-10-13.md) and
   [the core's infrastructure](research/discovery-infra-core-2026-10-13.md). The owner ruled on the
   consolidated plan in
   [D292](../decisions/working-notes.md#d292--the-third-audit-round-taken-no-admin-through-a-discovery-a-thin-path-first-two-machines-staging-inside-the-owners-own-names-the-household-zone-at-orpheum-corecom)
   (§8).

The questions those rulings left open were answered the same day in
[D293](../decisions/working-notes.md#d293--the-discoverys-open-questions-answered-a-live-instance-is-pinged-before-it-is-replaced-device-codes-through-the-service-sealed-by-hpke-on-p-256-in-the-beta-stagings-dns-token-in-the-sandbox-until-production)
(§9).

Where a report and a decision disagree, the decision holds; where the third round corrects the
second, the third holds. A second- or third-round recommendation no decision names is marked
**[I]** and cites its finding.

**Markers.** `D<n>` is a decision; a claim citing one comes from it. **[I]** is an inference or
recommendation not taken from a decision. **[V]** is a fact checked at its source, in
[reachability-approaches-2026-10-09.md](research/reachability-approaches-2026-10-09.md) ("the
research") or in the audit report its finding id names. **Built** says what exists today; **the
code must change** marks where the code disagrees with a design that stands; no question is the
owner's for the beta now (§9). **Deferred** marks what D292 §3 put after the closed beta and D293
did not bring back. Finding ids (S5, X7, A1…) name the audit finding a rule answers; an item of a
report's list of errors in our documents is cited by its report and number (*W errors 5*, *A
corrections 9*), and "the A report" or "the W report" is the third-round security or second-round
web report as a whole.

**Words.** A **discovery** is either the paid service or an owner's own. Where the two differ the
text says **paid** or **own**. A **core** is a household's Orpheum server; a **client** is a browser
or app; a **device** is a client that has found its core. **The beta** is the paid service's closed
beta: a few invited households, no payment, no public sign-up, one core per account (D290 §18).

## 1. What a discovery does, and does not

It connects a household's clients to that household's core and carries none of their traffic
(D273 §2, D214 §1):

1. **Serves the web client** (paid: at `app.orpheum-app.com`, served in the beta by the service's
   own machine, `prod-api`, and from Cloudflare Pages only at the public launch, D292 §3; own: at
   the owner's domain, D274 §5). After that, every API call, chunk, image and socket goes from the
   browser to the core. What the client's integrity measures do, and do not, is §3.4.
2. **Names and certificates** (paid only): hidden per-core names under the household zone,
   `orpheum-core.com`, with a record written for each published address (D292 §1); the core runs
   ACME and keeps its key, the discovery only writes the DNS records that prove the core's account
   (§2.7). An own discovery uses the owner's domain.
3. **Where the core is now:** the core reports candidate addresses; the discovery tests them from
   outside and hands the working ones to the core's devices (§2.4, §2.6).
4. **WebRTC signalling and STUN** for a core no HTTPS path reaches: offers and answers over one
   connection the core keeps open, a few kilobytes each, never media (D273 §7, §8; §2.5, §2.9).
   It is in the beta (D290 §14), and no signalling connection opens until offers are checked and
   bounded and `discovery/single`'s takeover flaws are closed (D292 §4; §2.2, §2.5).
5. **Finding the core:** links, QR codes and device codes lead a device to its core, and a
   reusable link per person leads it back (§3.1).

It holds no household data, signs no household user in (they sign in to the core, through the
client), and lists no cores (D214 §3, D187 §2). **No relay in the beta**: a relay for the
connections that fail waits on the beta's count (D292 §5, restating D274 §2 over the earlier "no
TURN, ever"; X6). A relay, if one is built, would carry only DTLS whose fingerprint the core signs
(§2.5), with credentials per session and peers limited to the core's candidates [I, the A report's
"No relay, in detail"]. A household's own TURN, on its own VPS, is the household's (§2.4).

**Staging** has no domains or accounts of its own (D292 §2): its client and API are
`app-staging.orpheum-app.com` and `api-staging.orpheum-app.com`, reached through a Cloudflare
Tunnel whose token edits no DNS, and its cores sit in a domain of their own, `orpheum-test.com` (D296). It
uses Let's Encrypt's staging CA [I, devloop §7]. Only the owner's own cores enrol with it, and no
credential able to touch production enters the agent's sandbox (D292 §2), with one exception
bounded by a date (D293 §3): a Cloudflare token cannot be scoped below a zone, so staging's zone
writer holds a token for `orpheum-core.com` while the zone holds nothing of value; before the first
production core enrols, staging's writes move behind the production zone writer, which accepts only
staging's subzone from it, and the token is revoked.

**What an outage stops** [I, from the availability research; N10]. Once a client has reached its
core over an HTTPS path, the discovery's API can go down and playing goes on. Three things still
need the discovery:

- **The zone's DNS is on the path to playing**: a returning device finds its core by a name under
  the zone, so the zone must keep answering (§2.7, D290 §7).
- **WebRTC needs signalling for every new connection**, and for recovering one (§2.5's ICE restart
  goes through the discovery). A household only WebRTC reaches loses every new connection while the
  discovery is down; one already open goes on until it fails.
- **A device rides out an outage only if it remembers its paths.** Safari deletes a site's
  script-writable storage after seven days of Safari use without interaction on the site **[V,
  R4]**, so a Safari tab needs the discovery again after that.

At home, with the house's internet or the zone's DNS down, every name fails and an HTTPS page may
not call the core's plain-HTTP LAN address (mixed content **[V, N3]**). The household reaches its
core then by **its home address**, which every household is told (D292 §6; §3.1).

## 2. Core ↔ discovery

All calls are HTTPS to `<discovery>/core/v1/…`, JSON, outbound only; the discovery connects in only
to probe (§2.6). **Built** (`discovery/discoveryproto`, `internal/core/discovery`): JSON keys are
camelCase; every call carries `Orpheum-Discovery-Protocol: 1.0`; an answer's
`Orpheum-Discovery-Deprecated: <date>` header is shown to the core's admin. §5 says how the version
signals change (C13).

Every retry the core makes backs off exponentially with jitter, so a discovery's restart is not met
by every core at once (C7). **The signalling connection's reopening starts near 1 s** and grows
from there, with jitter, to a cap, so a restart of the discovery (in development, every save) brings
every core back within seconds (D292 §9; devloop §6). Built (2026-10-13): `internal/platform/backoff`,
from the config's `[discovery]` table, 1 s doubling to 5 minutes, the development stack's capped at
1 s.

### 2.1 The core's identity key (D214 §4) — built

`internal/core/identity`: an Ed25519 key pair made on first start, kept as `identity.key` beside
`secret.key` (D194), mode 0600, never in the database. It is not the TLS key. Its **fingerprint**,
`SHA-256(public key)`, is carried in every link and QR the core makes in base64url, 43 characters,
which machines read (D292 §4; §3.1), and shown wherever a person reads, compares or types it in
safe32 groups, 52 characters in 13 groups of four, as the core's other codes are written: no letters
that look alike, case ignored (the owner, 2026-10-14). Built: both forms (`proof.Fingerprint`,
`proof.FingerprintText`), the System page showing the second.

**Backups and the recovery code** (D292 §4, revising D289 §2's leaving the key out; A6). When the
core is claimed it makes a **recovery code**, 128 random bits, shown once on the System page to
print or save. Scheduled backups then hold the identity key **sealed under the recovery code**
(argon2id over the code, then XChaCha20-Poly1305, as `secrets` seals `secret.key`'s contents
[I, A6]); restoring with the code brings back the same key, so nothing is re-keyed and no client is
warned. *Back up now* holds it as before when its box is ticked. The TLS and ACME account keys stay
out of scheduled backups, as built since an earlier commit (every private key left out unless asked): a
restored core makes new ones (§2.7 says what follows). **Not built:** the recovery code and the
sealed copy.

It signs, each under a purpose of its own (`discovery/proof`). The purposes are a closed set that
grows by decision (N9), today:

- enrolment and session challenges (§2.2, §2.3; built);
- a client's challenge on first contact, so the web client can remember the key (§3.5; not built);
- each WebRTC answer, over its exact SDP (§2.5; not built);
- a re-key: the new key's enrolment, countersigned by the old key (§2.8; deferred).

Native apps may later need a statement naming the core's current TLS key, so that an app that
pinned the identity key opens TLS to the LAN address with the internet down [I, coherence §1.1].

**Signatures are bound to the discovery** (C8): what is signed is
`orpheum-discovery-v1:<purpose>:<discovery base URL>:<coreId or public key>:<nonce>`, the base URL
being the one the core itself called, path included, without a trailing slash (X12), so a
discovery cannot pass another discovery's challenge to the core and come away with a session
there. Nothing is deployed, so this changes within v1. Built (`discovery/proof`; the core
names itself by `proof.KeyLabel` of its key until it has an id).

### 2.2 Enrolment and claiming — enrolment built; claiming by the account's key being built (B0 phase C)

**Claiming by the account's key** (D310 §5, §8, §9; D312 §5, §15; D318 §5), on the paid service from
the closed beta, replacing the operator's claim by code (the code stays beside it until phase C's
step 5 retires it). The household's account makes a key: 16 safe32 characters,
`xxxx-xxxx-xxxx-xxxx`, for an hour, several outstanding at once. The core's admin, signed in at
home, enters it on System → Discovery while the core is pending, and the core sends it with its
session: `POST /claim {key}` → `{account}`, the account's address. In one write the service finds
the live key, ends every key of the account (D318 §2), and claims the core for the account; a key
is spent by its first attempt, right or wrong, and wrong keys are refused (`wrong_key`) and limited
per core and per site. Then the account's core claimed before is removed at once: its records,
sessions, names and signalling go (D312 §15), and its identity key is remembered as removed, so
when it enrols again its `/enrol` and `/status` answers carry `removedAt`, its System page says it
was removed from the account, and only a new key claims it again. A claimed core's `/status` names
the account's address (`account`), which its System page shows (D310 §5). The own discovery takes
keys from its command line with the open beta (D310 §7); until then it answers `/claim` with
nothing and keeps its claim code.

The rest of this section is the beta's first design, by the operator's claim, kept until step 5.

1. **Enrolment** (built). `POST /challenge {publicKey}` → `{nonce}`;
   `POST /enrol {publicKey, nonce, signature}` → `{coreId, claimCode, claimExpiresAt}` (Unix ms).
   The body carries no version (§5; X11; built). A nonce works once. Enrolment is idempotent per public key: the same key gets the
   same pending core and `coreId`, with a fresh code once the old one expired (C12; `single` does
   this). A double click on *Connect* enrols once (C12; built).
2. **Claiming is by the code, on both tiers, in the beta.** The core's System page shows the claim
   code: 8 characters of safe32 shown `xxxx-xxxx` (`discovery/safe32code`), 30 minutes, once.
   - **Paid:** there are no customer accounts, sign-in links or mail in the beta; **the operator
     claims each core by its code**, read to them over a channel the tester and the operator already
     share (D292 §3). The tester's System page then shows the core claimed.
   - **Own:** the owner runs `orpheum discovery claim <code>` on the VPS (D274 §5); it prints the
     fingerprint it binds (T13). The owner types it on a machine they control, so nothing else is
     needed (S7). **Not built:** the command; `single.Claim` exists, used by its tests only. The
     own discovery's code stands on its own; D274 §5's "the paid tier's flow" was the reason, and
     the paid flow later changed (N8).
3. The core, polling `GET /status`, sees *claimed*, shows its recovery code (§2.1) and goes on to
   §2.3–§2.7.

**After the beta, before public sign-up** (D292 §3): customer accounts, and claiming from the core
as D290 §1 decided. The core enrols and opens `https://account.orpheum-app.com/claim#<claim
token>`, a token of full 128-bit entropy that lives a few minutes, in a tab opened within the
admin's click (U2-4) [I]; the customer confirms, and the core's System page shows which account asks
with *Allow* and *Refuse*, only *Allow* from an admin binding it (D290 §1). Taken into that design
[I, A15, T10, U2-5]: the approval bound to the admin's browser by a one-time value the claim page
returns to the System page (OAuth `state`'s pattern); the account's address shown in full to the
core's admin; the claim page saying "continue only if you just pressed *Connect* on your own
server"; any change of the holding account needing the core admin's approval; *Allow* an admin
action under §3.7. The typed code stays as a fallback that still needs *Allow* (D291 §6; X8).

**An expired code is replaced, not deleted** (C12, bringing the document in line with `single`): a
pending core asking `GET /status` after its code expired is given a new one, and the System page
says so ("The code expired; here is a new one", U5).

**A stranger cannot take a core's place** (D292 §4: `discovery/single`'s takeover flaws, the second
code audit's K13–K15, A3, closed before any signalling connection opens):

- while a pending core's code is live, another key's enrolment is refused with 409; a pending core
  is replaced only after its code expired (C12, S18);
- every session token is bound to `(coreId, public key, instanceId)` and checked against the
  record's core on every call; when the record's core changes, every token is dropped;
- `PUT /addresses`, `PUT /acme-challenge` and the signalling connection are refused until the core
  is claimed;
- `coreId` is 130 bits (§2.7).

Built in `single` (A3), but for the binding's `instanceId`, which a token is bound to through the
live instance it was opened by (§2.3). The paid service does not copy `single`'s session handling
(A3). A refusal the core acts on carries a `{code, message}` body of its own
(`discoveryproto.Refusal`): `enrol_again`, on which the core re-enrols and shows the new code (C12),
`another_instance` (§2.3) and `not_claimed`. Built, both sides.

Enrolment is rate-limited per source site (an IPv4 address or an IPv6 /48, as D288 §6 keys device
codes; N2 [I]), and only a claimed core gets names and a certificate (S13).

### 2.3 Session — built

`POST /challenge` → `{nonce}`; `POST /session {coreId, instanceId, nonce, signature}` →
`{token, expiresIn}`, an opaque bearer token for one hour, hashed at rest. Every other call sends
`Authorization: Bearer …`, never a token in a URL (D154 §2). No mTLS, which complicates proxies, and
no signature on every request.

- **`instanceId`** is random, made at each start of the core, kept in memory and never backed up. A
  discovery holds one live instance per `coreId` (D290 §18, C4). **A second instance pings the
  first** (D293 §1, replacing "the newest wins", coherence decision 7, and waiting out missed
  heartbeats, T14): when a core asks for a session with a new `instanceId` while another instance
  holds a signalling connection for the same `coreId`, the discovery pings the old instance on that
  connection and waits about 10 seconds. No answer, or no connection, and the newcomer replaces it;
  an answer, and the newcomer is refused as another copy with a named error ("another server is
  running with this identity"), which it shows in its log and on its System page, retrying every
  few minutes. A crash-restart is accepted in seconds, a real second copy never displaces the one
  serving, and a stolen key cannot knock a live core off. The advice on a copy is *This is a copy:
  turn off remote access here*, which forgets `discovery.json` on that machine, not a re-key, which
  would move the household to the copy (U2-12) [I]. Built in `single` and the core, but for the
  advice on the System page.
- **Expiry as a duration** (`expiresIn`, seconds), counted on the core's monotonic clock, so a core
  whose clock is wrong neither renews on every step nor sends expired tokens (C11). Built
  (2026-10-13), counted from before the core asked.
- **Renewal.** The core renews five minutes before expiry, and **at once, once, on any 401** from
  any call, then backs off; one renewal runs at a time, the step loop, the signalling loop and the
  certificate job sharing it (C11). Built (2026-10-13).
- **Tokens outlive a discovery's deploy** (C11): the paid service keeps them hashed in its database,
  so a deploy does not cut every core's signalling for a heartbeat's length. `single` keeps them in
  memory and prunes them as it checks one.

### 2.4 Heartbeat and addresses — built for interface and mapped candidates

The heartbeat rides on the signalling connection (§2.5) once that is open, and `PUT /addresses` is
its fallback while it is not (D291 §6, taking §7 item 22; X9). It is sent every 5 minutes
with ±20% jitter, and at once when an address or mapping changes:

```json
{
  "orpheumVersion": "0.4.0", "apiMajor": 1,
  "candidates": [
    {"address": "2a0f:1000:…:1", "port": 443,   "source": "interface"},
    {"address": "203.0.113.7",   "port": 51234, "source": "mapped", "method": "pcp"},
    {"address": "198.51.100.9",  "port": 443,   "source": "configured"},
    {"address": "192.168.1.20",  "port": 443,   "source": "interface", "scope": "lan"}
  ],
  "turn": [{"urls": ["turn:198.51.100.9:3478"], "username": "…", "credential": "…"}]
}
```

The body carries no version (C13, §5; built). `configured` candidates are built (2026-10-14). **The code must change:** the heartbeat is a `PUT /addresses` every 5 minutes, without jitter, and nothing rides on
the signalling connection.

- `interface`: an address on the machine (a global IPv6 address is native on DS-Lite). **Only stable
  addresses** (C7): on Linux the core reads its addresses through netlink and skips temporary,
  deprecated, tentative and failed ones, preferring the stable (RFC 7217 or EUI-64) address, since
  a temporary address lives a day or two and a router's IPv6 port sharing follows the stable one
  **[V, C7]**. Container bridges are skipped. On other systems `net.InterfaceAddrs` stays the
  fallback [I, coherence §1.1]. **The code must change:** it reads `net.InterfaceAddrs`, which
  carries no flags, polled every minute.
- **Non-public IPv4 is never public** (C7, C9): `100.64.0.0/10` (carrier NAT), `192.0.0.0/29`
  (DS-Lite) and the documentation ranges are classed non-public, which Go's `netip` does not do. One
  function classes addresses, in `discoveryproto`, for publishing and for the probe's deny list
  alike (G12) [I]. Built (2026-10-13): `discoveryproto.IsPublic`, which the core's interface
  candidates go through, a private address marked `lan` and the rest never sent.
- `mapped`: a public IPv4 and port from the router by UPnP IGD, NAT-PMP or PCP, **on by default**,
  shown on the System page, with a switch (D273 §7; built, `internal/core/portmap`). **It maps TCP
  only, never a UDP port** (D290 §9), and **asks for outside port 443 first, taking whatever port
  the router grants when 443 is held** (D291 §7, refining D290 §9's "443 only"; N4, X20). The rule
  needs no incident behind it: no UDP responder of the core is exposed. (Plex servers abused for
  amplification were on a DMZ or behind manual forwards, not mapped by UPnP, R3.) A router may grant
  another external port than asked, so every candidate carries its port; a port browsers refuse
  (the Fetch standard's bad ports) is never asked for or published (C9). The rest of C9, taken:
  - the external address the router reports is checked, and when it is not public (a second router,
    or the provider's NAT in front) the System page says so and nothing is published, rather than
    "port mapped";
  - NAT-PMP's and PCP's epoch is read on every answer and PCP's ANNOUNCE listened for, and every
    mapping re-made when the router restarted;
  - UPnP falls back to `AddAnyPortMapping`, then another port, on a conflict;
  - PCP's nonce is kept in the core's state, so a restarted core can renew or delete its mapping;
  - IPv6 pinholes are updated and deleted (`UpdatePinhole`, `DeletePinhole`), not re-added;
  - UPnP's description and control URLs are accepted only on the gateway's own address (code K20)
    [I];
  - the System page says which of the common cases it met (a FRITZ!Box grants mappings only to a
    device the owner allowed, **[V, C9]**).

  **The code must change** for each of these; as built it asks for the core's own `https.listen`
  port as the external one.
- `configured`: an address the admin set: a port-forward, or their own VPS. A household whose own
  VPS reaches its core (the owner's, through the WireGuard tunnel) keeps it as a `configured`
  candidate, with its TURN, rather than retiring it once the discovery works: for a household behind
  DS-Lite or carrier NAT it is an IPv4-only device's route (D292, the bring-up plan's item 21, from
  coherence §1.3; N11).
- `lan` addresses are published unprobed; they matter only at home (§3.2).
- `turn`: a TURN server the owner runs, passed to the core's clients; the owner's relay, never the
  paid service's. Its credentials are **time-limited** (the TURN REST pattern: a username of
  `<expiry>:<id>`, a password that is an HMAC under a secret the core and the TURN server share),
  minted by the core and refreshed with each heartbeat, never the static long-term ones (C14). Built
  2026-10-14: a username of `<expiry>:found`, lasting a day [I], which the relay admits with no
  membership, still relaying only to the public internet.
- The discovery records the request's source address too, but never publishes it unprobed: behind
  carrier NAT it is shared and unreachable. Behind Cloudflare (the paid service, reached through a
  Cloudflare Tunnel, D292 §2–§3) the source is taken from `CF-Connecting-IP`, trusted only from
  Cloudflare's connector [I, availability research].

The discovery publishes only public candidates its probe passed (§2.6), and keeps current addresses
only, no history. **Liveness is the signalling connection's presence** (C16): a core whose
connection closed is *offline* at once; one that has none and sent no heartbeat for 30 minutes is
offline too. Its names keep answering either way (§2.7). **The code must change:** `single` marks
nothing offline.

### 2.5 The signalling connection (D273 §7, §8) — built on the core, unchecked

One WebSocket the core keeps open, `<discovery>/core/v1/signal`, with the session's token, for a
claimed core only (§2.2). It carries WebRTC offers and answers, the heartbeat (§2.4), and the
discovery's ping to the instance holding it when a second instance asks for a session (§2.3,
D293 §1).

**No core holds a signalling connection to any discovery until this section's checks and bounds
are built** (D292 §4; A2). Built (2026-10-13): the fingerprint and candidate checks of item 3 and
the address filter, before `rtcapi` makes a connection (`rtcapi.Server.AnswerRelayed`, the one way
an offer through `internal/core/discovery` reaches it), and each connection's bounds below but the
idle close, the unproven connections and per-locator caps. Built since (2026-10-14): the proof
(item 3), checked by the core under the key it keeps for the offer's hash before `rtcapi` sees it,
and the answer's signature (item 4); `single` passes offers on with their hash and proof, at most
four to a request and eight open for one hash. **The code must change:** offers are answered one
at a time.

1. A client posts up to four SDP offers with its locator (§3.1), or the bootstrap token `/locate`
   gave it, and with each offer **its proof** (below).
2. The discovery checks the locator, as a first filter, and forwards
   `{"type": "offer", "offerId", "sdp", "locatorHash", "proof"}` (text messages, one JSON object
   each, at most 64 KiB; several offers to a message, so a client's connections need one round trip,
   C6) [I: the shape is the build's].
3. **The core checks the offer itself** (D290 §6, D292 §4; A2, A9) before `rtcapi` sees it, and
   refuses it otherwise:
   - **the proof**: an HMAC over the offer's DTLS fingerprint, keyed by a value the core holds and
     the discovery never sees: for a signed-in client, its session's stored hash,
     `SHA-256(token)`; for a link's bootstrap, `SHA-256("orpheum-offer" ‖ secret)`, which the core
     stored when it made the link and the client derives from the link (A9, T7). Matching
     `locatorHash` against the registered set (`PUT /locators`, `PUT /locate-hashes`) stops a bug
     in the discovery, not a compromised one, since the discovery holds every hash (T7, E6);
   - **exactly one `a=fingerprint` line, sha-256**: pion checks the remote certificate against one
     fingerprint, a session-level line winning over a media-level one, and accepts md5 and sha-1,
     so an extra line would let a discovery terminate DTLS in the middle **[V, A9]**. Once DTLS
     connects, the remote certificate's hash is compared with the value the proof covered;
   - **candidates**: at most about ten; loopback, link-local, private ranges and the core's own LAN
     prefixes refused for offers through a discovery (`SettingEngine.SetRemoteIPFilter`), since the
     LAN path does not need ICE through it; a budget of connectivity checks (RFC 8445 §19.5.1's
     100) (A2; D291 §6, S16). The client sends only what passes: it drops the candidates no core
     outside its network can use and keeps at most ten of the rest, relayed and UDP ones first, since
     a phone on Wi-Fi gathers more and the whole offer was refused (`web/packages/sdk/src/relayed-offer.ts`,
     2026-10-14);

   A `TestArch_` claim that an offer through `internal/core/discovery` reaches `rtcapi` only after
   the core's own check enforces it (A2).
4. The core answers as `internal/core/rtcapi` answers its own `POST /rtc/offer`:
   `{"type": "answer", "offerId", "sdp", "signature", "publicKey"}`, or
   `{"type": "refusal", "offerId", "reason"}`. **`signature`** is the core's identity key over the
   **exact answer SDP**, with the `offerId` and the client's offer fingerprint, under the answer's
   own purpose (D292 §4; A7, A9, T12). A7 also names the discovery's origin; it is left out, since
   the client knows the discovery by its client origin and the core by its API base URL, and the
   client's own fingerprint already ties the answer to one offer [I]. The client checks it against
   the key it remembers, or the fingerprint its link or QR carried (§3.1, §3.5): the answer carries the
   core's public key, which the client takes only when its SHA-256 is that fingerprint. It passes that exact string
   to `setRemoteDescription`; it refuses an answer with more than one fingerprint or any hash but
   sha-256 (A7). The binding is mutual: the answer covers the client's fingerprint too (A9). Built
   on the core (`proof.Answer`, `SignalMessage.signature`, the fields a line each, the SDP last);
   the client's check comes with the client (webrtc-transport W2f).
5. The discovery returns the answer; the data channels then run browser to core.

**What this stops** (E5, T4, A9): a discovery that only relays signalling cannot sit between an
honest client and its core. The party that also serves the client's JavaScript can (§3.7).

**Bounds** (D290 §6; S5; C6; A2):

- Offers are limited per locator at the discovery, in rate and in how many may be open at once (at
  most 8 offers or connections per locator, S5), and per core; **and on the core**, per source and
  per locator, not only at the discovery (A2).
- **The core answers offers concurrently**, each under the connection limit, so one slow gathering
  does not hold the next (C6, S5). **The code must change:** the signalling loop answers one at a
  time, each waiting up to 5 s.
- **Early description** (C6; F2: not RFC 8838's half trickle): each side sends
  its description once its host candidates and its first server-reflexive candidate are in, or
  after about a second, whichever comes first, rather than waiting for gathering to complete (the
  client waits up to 10 s today, the core 5 s and then refuses). **The code must change.**
- **A slot is counted once ICE connects**, an unconnected offer bounded to about 10 s; the cap is
  sized by memory, not a fixed 64, and the System page says when it refused anyone (C6). As built a
  slot is held from the offer until ICE fails, about 30 s, and 64 at four per client is 16 clients.
- **Each connection is bounded** (A2, code K16): SCTP's receive buffer
  (`SetSCTPMaxReceiveBufferSize`) and an application message cap; the data channels and requests in
  flight per connection; a deadline on a request's head and body.
- **Unproven connections** (a bootstrap token's, before a sign-in) are short-lived, few, evicted
  first, and limited to signing in, registering, pairing, joining Watch Together and device codes
  (D291 §6, S5 item 2; G4, A2).
- **Idle connections close** (D290 §6, S5): a connection that has carried no request with a valid
  sign-in for a few minutes is closed, so no one holds the core's slots by opening connections and
  sending nothing.

**A connection grants nothing**: every request over a channel carries its own bearer token to the
core. **The request's head is untrusted** (A1): `rtcapi` copies what the page wrote, so the core
strips `Origin`, `Host`, `Forwarded` and `X-Forwarded-*` before the handler, and nothing over
WebRTC counts as the core's own origin (§3.7). Built (`rtcapi.IsOverWebRTC`).

**Recovery** (C6, taken; webrtc-transport W2d):

- The client races WebRTC with **one** connection, and opens the others only once WebRTC is the
  chosen path, keeping each that connects (not all-or-nothing) and retrying the rest.
- It watches each connection's state, takes a failed one out of the rotation at once, and replaces
  it. **The code must change:** `connectRtc` needs all four, and a failed connection, answering at
  once, attracts every request in the least-loaded choice.
- **ICE restart** when a connection is `disconnected` for more than a few seconds, its offer going
  through the discovery (or over the data channel while it lives), the existing channels carrying on
  meanwhile (RFC 8445 §9 **[V, C6]**); a new race when it is `failed`. Until the restart is built, a
  failed connection starts a new race (coherence §3.2). Not built: every offer makes a new
  connection.
- The core pings its signalling connection every 25–30 s and treats two missed pongs as dead, and
  reopens it at once on the backoff of §2, starting near 1 s (C11, D292 §9). Built (2026-10-14):
  `internal/core/discovery`'s `keepAlive`, every 27.5 s ±9%; staging's connection, idle, had been
  closed by Cloudflare every two minutes.

**Leaving, re-keying, revocation and disabling close the connection** and invalidate the core's
tokens at once (C4). **The code must change:** after `DELETE /core` the socket and tokens stay live.

### 2.6 The outside probe

When a core's candidates change, the discovery tests each public candidate from a probe machine on
another network: HTTPS to the address with the candidate's name as SNI, the certificate checked, an
echo of a nonce the core issued (the check of `internal/core/reachability`, from outside, a failure
naming its hop, D152); and the owner's TURN servers. It probes on change and on clients' reports,
not hourly (D291 §6, §7 item 22), **at most once per core per 10 minutes** (S10 item 3, as
it reads once the hourly probe is gone; X9) [I]. The nonce comes in each heartbeat, as `probeNonce`, and the
core's echo recognises it for ten minutes, past the next heartbeat's. Built (2026-10-14): the
core's side (`reachability.Checker.ProbeNonce`).

**The probe runs on a machine of its own** (D291 §1), `prod-probe`, holding no secret but a token
that can only post results (D292 §3). Its limits (S10, D291 §1): global unicast only, the service's
own prefixes and the private, shared, documentation and cloud-internal ranges refused, by §2.4's one
classing function (G12); few ports; a cap on `configured` and TURN candidates; rate-limited per
core and per target; detail reported only once the target proved it is the core.

**The core's NAT measurement is deferred** (D292 §3; C10). When built, the core sends STUN binding
requests to two STUN servers on different addresses and compares the mapped ports, as RFC 5780
describes: the same port means endpoint-independent mapping (punching works with most clients),
different ports mean endpoint-dependent (punching works only with clients that have IPv6 or an open
NAT). A probe that punches from a data centre's public address passes whenever the core can send UDP
out, so it says little of clients behind their own NATs.

The heartbeat's answer carries the result, each probe as fields, not free text (C16):

```json
{"published": ["2a02-8070-0-0-0-0-0-1.k7x…q2.<zone>", "203-0-113-7.k7x…q2.<zone>:51234"],
 "probe": [{"address": "2a0f:1000::1", "port": 443, "kind": "https", "result": "reached"},
           {"address": "198.51.100.9", "port": 443, "kind": "https", "result": "refused"}],
 "reachableFrom": "ipv6"}
```

`reachableFrom` is named by what was tested (C10): `public_ipv4` (a public IPv4 on 443),
`public_ipv4_other_port` (a public IPv4 on another port, which networks allowing only 80 and 443 do
not reach), `ipv6`, or `nowhere`; and, once the NAT measurement is built, punchable with
endpoint-independent mapping or only from an open NAT. The core shows it on the System page, with
what a household nothing reaches can do (§3.6). Built (2026-10-13): the fields and the values; as
built, `single` publishes nothing and probes nothing (W4b.4).

The answer also carries the count of the core's clients that reached a dead end in the last week
(§3.6, D293 §7), for the System page. Not built.

### 2.7 Names and certificates (D273 §7, research B1, §9.2; D290 §7; D292 §1) — the core's side built

- **The household zone is `orpheum-core.com`** (D292 §1, replacing the `.app` domain and Bunny DNS
  of D291 §2), registered and served at Cloudflare; staging's cores sit in a domain of their own,
  `orpheum-test.com` (D296, revising D293 §3), so no staging token reaches it. It is never `orpheum-app.com`: every core holds a
  certificate there and could otherwise set cookies for the service's pages. The isolation is the
  separate registered domain. **The apex serves no HTTPS** (D293 §10, revising D292 §1's preload):
  Universal SSL stays off, so the apex's CAA stays `issue ";"`, and the HSTS preload of
  `orpheum-core.com` is not sought; each core's name sends its own HSTS. No Public Suffix List
  entry (D291 §4). DNSSEC on the zone [I: the bring-up plan's next step; written records
  are signed as any record is].
- **Names encode addresses** (Plex's shape): `<address>.<coreId>.<zone>`, the IPv4 address dashed
  (`192-168-1-20`) or the IPv6 address as eight dashed groups, lowercase, no zero padding and no
  compression (`2a02-8070-0-0-0-0-0-1`), so one address has one name and no label holds `--` (C15).
  The encoding is one function in `discoveryproto`, with test vectors, which the paid service,
  `single` and the client share (C15; built, `discoveryproto.AddressLabel`; the core sends its
  candidates as addresses, and the discovery names them). A port other
  than 443 goes in the URL.
- **`coreId` is 26 safe32 characters (130 bits)**, random, never reused for another key (§2.8).
  **The code must change:** `single` makes 8 characters (40 bits) (D290 §13).
- **One wildcard certificate per core**, `*.<coreId>.<zone>`: an address change needs no new
  certificate, and Certificate Transparency shows only the random label.
- **A record is written for each published address** (D292 §1): the discovery writes an A or AAAA
  record for each address it publishes (§2.6) under a claimed core's label, and deletes it when the
  address goes, so a name answers only for an address that core published, and the zone is not a
  rebinding resolver for anyone's. `lan` names are written only for private ranges, never
  `127.0.0.0/8` [I, S12]. A free Cloudflare zone holds 200 records (about 50 cores at four names)
  **[V, C2]**, ample for the beta; a computed answer, by a script at Bunny or elsewhere, is weighed
  again when the beta's count or a tested script calls for it (D292 §1). **TTLs are minutes, not
  days**, so a removed address or a disabled core stops answering soon after (G3) [I]. This replaces
  D290 §7's computed answers for the beta.
- **DNS is on the path to playing and is kept available** (D290 §7): Cloudflare's anycast name
  servers serve the zone (D292 §1).
- **CAA** (D292 §1; A4, T2, X5). RFC 8659 ignores `issuewild` for a name that is not a wildcard, and
  an RRset with no restricting tag restricts nothing **[V, A4]**, so `issuewild` alone left every
  name under a core open to any CA by HTTP-01. So:
  - at the apex, `0 issue ";"` and `0 issuewild ";"`, so nothing is issued in the zone but what a
    core's own record allows;
  - at each claimed core's own name, `<coreId>.<zone>`, `issue` and `issuewild` with
    `"letsencrypt.org; accounturi=<the core's ACME account>; validationmethods=dns-01"` (RFC 8657),
    so only that core's ACME account gets its certificate, by DNS-01 only. Written records keep
    clear of RFC 4592's wildcard rule, which a scripted wildcard below a stored node would meet
    (A4).

  The core reports its account URI (§4, `PUT /acme-account`, before each order; built 2026-10-14). A core that makes a new ACME account (a restore, which
  leaves the account key out, §2.1; a re-key) reports it, and the discovery rewrites the record
  before the core orders, or the CA refuses it (G2). CAA stops a bug in the TXT proxy's name check,
  not a stolen zone token, which can rewrite CAA too (X21). The CA/B Baseline Requirements make
  `accounturi` and `validationmethods` a MUST from 2027-03-15 **[V, A4]**.
- **DNS-01 by proxy** (built) is the method through the beta: the core runs ACME
  (`internal/core/certificate`) and calls `PUT /acme-challenge {name, value}`; the discovery checks
  the core is claimed (A3), the name is `_acme-challenge.<coreId>.<zone>` for this core and the
  value a 43-character base64url digest (S18), sets it, and clears it after an hour at the latest;
  the core asks the CA to validate, then clears it. The core keeps its ACME account, its key and its
  certificate. Taken from C1: the call returns only once every authoritative server serves the
  record, and the core checks it resolves before asking the CA (as built it waits a fixed 30 s).
  Clearing names the record in the path, not in a body, which DELETE does not define (C16; built
  2026-10-13, with `single`'s check of the value).
- **Renewal at two-thirds of the lifetime** (C1). **The code must change:** it renews 30 days
  before expiry, fixed, which for a certificate shorter than 30 days would reissue on every sweep.
- **Refuse a handshake without a served name** (A10): the core answers TLS only when the SNI is a
  name it serves, so a scanner sweeping addresses does not learn its `coreId` from the certificate
  (the list D214 §3 forbids). **The code must change:** `GetCertificate` returns the discovery's
  certificate for any server name, an empty one included.
- **Let's Encrypt's limits** (50 new certificates per registered domain a week **[V, C1]**) bite on
  new cores, not renewals; a limit increase for `orpheum-core.com` is asked for early, since it
  takes a few weeks (D290 §7; F11). A Public Suffix List entry is not the answer: the PSL refuses
  entries whose purpose is to get around rate limits **[V, C1, S12]**. Lifetimes shrink to 64 days
  in February 2027 and 45 in 2028; authorisation reuse is 30 days today, 10 from 2027-02-10 and 7
  hours from 2028-02-16 **[V, A14]**: every renewal of every core needs the discovery live.
- **Deferred** (D292 §3): **ARI** (renewal in the window Let's Encrypt's renewal information gives,
  exempt from its rate limits); **DNS-PERSIST-01**, which Let's Encrypt has not deployed and whose
  draft -02 replaced the plain account URI with a hash only the core can compute **[V, A14, F1]**
  (when it ships, the core computes the record's value, CAA adds `dns-persist-01` to
  `validationmethods`, a re-key deactivates the old ACME account, and no lapse state relies on
  `persistUntil` [I, A14]); and **Certificate Transparency watching** (D290 §3; its design is the
  service watching the whole zone and passing alarms to cores, a core's own query opt-in, alarms
  reaching admin sessions [I, A16, T11]).
- After leaving, the core stops serving the certificate with the rest (C16; **the code must
  change**).
- **Own:** the names are the owner's domain's, set through the owner's DNS provider (W4b, the plan's
  Q2), on a domain that holds nothing else the owner values, since a provider's token reaches a
  whole zone (infrastructure §2) [I].

### 2.8 Revocation, re-keying, recovery and leaving

- **Removing or leaving** (`DELETE /core`): records removed, locators dropped, the core marked gone,
  its signalling connection closed and its tokens invalidated (C4). Its `coreId` is never given to
  another key, so an old link never reaches someone else's server.
- **After a backup restored**: with the recovery code, the same key comes back from the sealed copy
  (§2.1) and nothing else happens. Without it, a core that remembers a `coreId` (`discovery.json`)
  but whose key no longer opens a session says so on System → Discovery and hides *Connect*, which
  would enrol a new core and strand every link (D290 §11, U6). A key file still present and refused
  means someone else holds the `coreId`: an alarm, not a routine re-key (A6 item 5) [I].
- **In the beta, a lost key is re-keyed by hand** (D293 §5): the operator binds the new key to the
  `coreId` after checking with the household (coherence §4.1).
- **Two cores with one key** (a backup restored onto a second machine while the first runs) are
  detected by `instanceId`: the live one answers the discovery's ping and the other is refused
  (§2.3, D290 §18, D293 §1, C4).
- **After a lapse** the `coreId` is retired for every other key but not for its own: the same key
  reclaims it on resubscribing, and the household keeps its links (D290 §11, U6).
- **Disabling** (paid, abuse): records removed, signalling and enrolment refused, the connection
  closed.

**Deferred: re-keying and mailbox recovery** (D292 §3), which wait with the customer accounts.
Decided for then: **rotation** signed by the old key, taking effect at once (D290 §3, S1);
**recovery** of a lost key after a wait of about 72 hours, with mail to the customer (D290 §3);
`orpheum discovery rekey` on an own discovery's VPS (C4). What a re-key does (C4): the discovery
binds the new key to the existing `coreId`, revokes every token of the old key and closes its
connection; the core with the new key makes a new ACME account and certificate, revokes the old
certificate, deactivates the old account and reports the new one for CAA [I, T1, A14]. Recommended
for then and not yet ruled [I, A6, T1, T5]: a rotation mailed and shown, not silent; a re-key by the
recovery code, immediate; a mailbox-only recovery of seven days, cancelled by any authenticated
contact from the old key during the wait, never shortened by a removal. Recovery's design waits
until it is built, after the beta (D293 §5; §9 item 3).

### 2.9 STUN

The discovery runs a STUN server (UDP 3478), given to clients and cores as their ICE server through
`GET /status`'s `iceServers` (built on the core, D274). It answers Binding requests only,
rate-limited per source, without `SOFTWARE`, which discloses the version (S16; F13). It carries
nothing else. Paid: on `prod-probe`'s addresses (D292 §3, the bring-up plan), never the API's,
which sits behind Cloudflare and whose address must not be given away [I, availability research].
A third, `stun.cloudflare.com`, was taken by D291 §6 (G8); it is named on the privacy page. The
NAT measurement's second server waits with the measurement (§2.6). For the thin path any public STUN
server will do (coherence §3.2).

## 3. Client ↔ discovery

**The page is the discovery's; the traffic is the core's.** The page asks how to reach its core,
races the paths, and from then on talks only to the core.

### 3.1 How a device finds its core

A device learns its core once and remembers it. **Every link and QR the core makes carries the
identity key's fingerprint** after `#` (D292 §4; A7, T3, W8), so first contact checks a value the
household gave, not one the discovery gave:

| Entry | Flow |
| --- | --- |
| Registration link (D218 §1) | `https://<discovery>/register#<coreId>.<secret>.<fingerprint>`; locates while unused, then becomes the person's own link (below) |
| Watch Together invite (D243 §3) | `https://<discovery>/together#<coreId>.<secret>.<fingerprint>`, reusable while live (below) |
| **A person's own link** (D290 §10) | `https://<discovery>/s#<coreId>.<secret>.<fingerprint>`, reusable; locates only |
| Pairing QR (D214 §1) | `https://<discovery>/p#<coreId>.<secret>.<fingerprint>`, for native apps and a household's first device; the identity key's fingerprint, never a DTLS one, which `rtcapi` makes afresh for each connection (X7) |
| Device code (D288) | the device asks for a code and shows it; a person signed in enters it (below) |
| **The home address** (D292 §6) | the core's own origin on the LAN, shown as a link and a QR (below) |

On the core's own origin the built client makes `/register#<secret>` and `/together#<secret>`
(X13). Links made while a discovery has claimed the core take the discovery's form, with `coreId`
and fingerprint (built 2026-10-14 for registration and Watch Together links).

- **The discovery never learns a core's secrets.** The core registers, for each live secret,
  `H = SHA-256("orpheum-locate" ‖ secret)` with its life as a duration and a **use policy**: a
  number of uses, or as many as wanted until it expires or is revoked (`PUT /locate-hashes`, the
  full set; C3; built 2026-10-13 for registration links, invitations and own links, and 2026-10-14
  for Watch Together's links, each unused one sent with a life to the end of the next day and sent
  again daily, since a session has no end the core knows of in advance [I]). The page sends `H`, and the secret goes to the core over the direct
  path. `/locate` is spent by a click on the page, never by its loading, so a mail scanner opening
  the link spends nothing [I, S11, U10]. "Open in Safari", where an in-app browser is detected,
  shows before that click (U2-1) [I].
- **The core is the only issuer of locators** (C3, S11). `/locate` returns the paths and a
  **bootstrap token**: issued by `/locate`, usable for `/paths` and offers for a few minutes, never
  renewed (X10). The device registers or signs in over the direct path; the core then hands it a
  **device locator**, a random token, registers its hash and adds it to its full set
  (`PUT /locators`), which is then the whole truth. A locator belongs to a grant on the core (a
  device's session, or a Watch Together membership): when the core ends the grant, it drops the
  locator too (S11). A device that reached its core another way gets its locator the same way, and
  the core hands a fresh one on each visit [I, availability research].
- **A registration link becomes the person's own link** (D292 §4; A12, U2-1). Its locate is reusable
  while the registration is unused, the core spending the registration once, as built. Once used,
  the same link leads the person back and opens sign-in, not "used". It is revoked when the person
  is removed, resets their password, or the admin revokes it; it has a rolling life (90 days,
  renewed by a signed-in device's visit) and a cap on distinct sources a day [I: the numbers are
  A12's].
- **A person's own link** (D290 §10, U2) leads a device back to its core, under the same bounds. It
  grants no sign-in, but it does reveal the home's current addresses and a door to the core's
  unauthenticated surface (W7), which the privacy page says. It is on their account page and on the
  screen after registering, as a link and a QR, and is revoked by the admin. It names one core to
  someone holding its secret, so it is not a list of cores (D214 §3) [I, U2].
- **A Watch Together link finds its core while it lives** (D290 §17, C3). Admission stays the
  core's: an unused link admits its one person, a used one only the device holding the membership
  credential its first use gave (D267 §1; W errors 5). A used link locates only for that device, by
  a locator the core handed it (A12) [I]. A guest who closed the page by mistake gets back in.
- **The home address** (D292 §6, replacing D291 §6's "last good LAN address kept for an outage",
  which an HTTPS page could not call; N3). The System page and each person's account page show the
  core's own address on the LAN, as a link and a QR, to bookmark; the no-core and dead-end pages,
  when the device seems to be at home, offer it as a link that leaves the discovery's origin (a
  top-level navigation is not mixed content). It is not a path the client races. Served on plain
  HTTP it is an insecure context: no service worker and no microphone (coherence §5.1).
- **A device that knows no core** (D290 §10, U2) gets a page of its own rather than a name and
  password form it cannot use: open the link you were sent; paste your server's link; sign in using
  a phone where you are already signed in (the device code, where there is one); ask the person who
  runs your server for your link [I: the paste box, U2-2].
- **Staying remembered** (D290 §10). Safari deletes script-writable storage after seven days of
  Safari use without interaction, and `persist()` does not stop it **[V, F8, A7]**. *Add to Home
  Screen* escapes that, but a home-screen app on an iPhone shares no storage with Safari **[V,
  U2-2]**, so it is offered from the person's own-link page and the installed app starts there
  [I, U2-2: whether a run-time manifest's `start_url` per person survives installation is a device
  test]. The camera opens QR codes in Safari, not in the home-screen app, so the TV's QR page tells
  a signed-out Safari what to do rather than showing a sign-in form [I, U2-2].
- **A new device with no link** cannot search for its core by name or e-mail: that would be a list
  of cores (D214 §3). It uses its person's own link, is sent a link, or is activated by its code
  (D288).

**Device codes (D288 §5).** A device not signed in opens its discovery's address and asks for a
code.

- **Own:** the one core issues it, as built (`internal/core/auth`'s pairing). Past a thousand
  waiting, or twenty not yet entered from one site (an IPv6 /48, or an IPv4 address), or five from
  one /64, a new code is refused with 429 and none waiting is dropped (D288 §6 as amended twice,
  D290 §15; built, `pairing.go`; N1).
- **One `device` scope for every code-activated device** (D292 §4, refining D291 §5; A11, W5,
  U2-6), on both tiers and for the core's own codes: the person's role minus administering and minus
  account security (changing the password, making links and codes, entering codes, removing
  devices); hosting, playback and the rest the role gives stay. The client reads the session's
  rights, not the role's, and hides what the session cannot do. Admin on such a device needs a
  password sign-in where §3.7 allows admin. An admin entering a code for another admin confirms it,
  and the core logs who did it for whom (A11, code K10) [I]. **The code must change:** every pairing
  issues the whole role (`auth.go`, `issueSession`).
- **Paid: in the closed beta, sealed by HPKE** (D293 §2, revising D292 §3's deferral). Not built;
  the design:
  - the page names no core, so the **service issues the code**; the core of the signed-in person who
    enters it claims it from the service. The service limits asking for codes per source site and
    fairly across sources (S13);
  - **the session is sealed to the device** (D290 §5, S3) with HPKE (RFC 9180) using
    **DHKEM(P-256, HKDF-SHA256)**, not X25519: P-256 has been in Web Crypto for a decade and
    television browsers run old engines, so no minimum browser rises and no library is needed
    (D293 §2). The device makes an ephemeral P-256 key pair; the core seals to it with Go's
    `crypto/hpke`; the client opens the blob with Web Crypto, checked against RFC 9180's test
    vectors. The sealed blob carries the session, the core's `coreId`, its identity key's
    fingerprint and a device locator (§3.1), so nothing the device needs crosses the service
    readable, which holds against a service that only relays (W2);
  - **the page entering a code warns** "only enter a code shown on a screen in front of you"
    (D293 §2). The line naming where the asking device is (D290 §5, S4: "The TV asking is near
    Berlin, on Telekom", neutral rather than a verdict, A11, U2-9) waits until after the beta,
    since it needs an IP-location database.

### 3.2 The paths

`POST /client/v1/paths` with a locator (or a bootstrap token) returns:

```json
{"coreId": "k7x…q2", "apiMajor": 1,
 "state": "online", "clientPrefix": "2a0f:3000:…::/64",
 "paths": [
   {"kind": "lan",    "url": "https://192-168-1-20.k7x…q2.<zone>"},
   {"kind": "ipv6",   "url": "https://2a02-8070-0-0-0-0-0-1.k7x…q2.<zone>"},
   {"kind": "ipv4",   "url": "https://203-0-113-7.k7x…q2.<zone>:51234"},
   {"kind": "webrtc", "offer": "/client/v1/cores/k7x…q2/offer"}],
 "iceServers": [{"urls": ["stun:stun.<zone>:3478"]},
                {"urls": ["turn:198.51.100.9:3478"], "username": "…", "credential": "…"}]}
```

`urls` is a list, as the code has it (`discoveryproto.ICEServer`). An entry with credentials is the
owner's TURN (§2.4); there is no `owner` field (D290 §13).

- **`state`** is the core's state as the discovery knows it (D293 §6), so a household member is
  told the cause when it is the core's (§3.6): `online`, `offline` (with `"offlineSince"`, Unix
  ms), `paused` for a lapse, or `restoring` [I: the spellings, from U7's causes]. The client shows
  the dead-end message only for `online` (§3.6).
- **`clientPrefix`** is the client's network prefix as the discovery sees it (D293 §8): the
  request's source as an IPv6 /64, or an IPv4 address as a /32 [I: the lengths], taken behind
  Cloudflare from `CF-Connecting-IP` (§2.4). It is the key under which the client remembers which
  paths worked (§3.3 item 7). It names the client's own address back to it, nothing of the core's.

**The `lan` path is returned only when the device seems to be at home** (D290 §2, U1, S11): the
request's source shares the core's IPv6 /64, or equals the core's public IPv4 when that address was
probed and is the core's own, never a shared carrier-NAT or DS-Lite address (G5, U2-8) [I], or the
device remembers the LAN as the path that won on this network (C5). Chrome asks permission for any
request from a public page to a name that resolves to a private address **[V, U1]**, so a LAN name
raced on every first visit would put the prompt in front of every guest, and a "Block" sticks for
the whole `app.` origin, every household's. Where the browser can say whether the permission is
granted, denied or not yet asked, the client asks it first; where a browser has no prompt (Safari),
the LAN name may be raced always (U2-8) [I].

| Path | Works when | Notes |
| --- | --- | --- |
| IPv6 | both ends have IPv6 and the router's IPv6 firewall lets the port in | a public address meets no Local Network Access prompt and no rebinding block |
| LAN name | the client is on the core's LAN | many routers drop public answers that point at private addresses (rebinding protection) **[V]**; Chrome asks permission first (§3.4) |
| Public IPv4 | a public IPv4 and a mapping | impossible behind carrier NAT or DS-Lite's shared IPv4; at home it needs hairpinning; a port other than 443 fails on networks that allow only 80 and 443 |
| WebRTC | the NATs on the two ends let a punch through: IPv6 on both ends, or one end endpoint-independent (§2.6) | the core's API over data channels; slower to start (0.45 s at 50 ms round trip) |

**How often WebRTC fails is not known for browsers** (C10). The research's "about 70%" is a libp2p
measurement among libp2p nodes, conditional on relay reservation and address discovery having
succeeded (arXiv 2604.12484, a separate paper from 2510.27500; F5) **[V, C10]**; WebRTC field data
had 22% of conferences needing a relay, and 40% of the cellular networks behind carrier NAT showed a
symmetric NAT, a lower bound per network, not a share of NATs **[V, F4]**. A phone on mobile data
may fail to reach a WebRTC-only household well over 30% of the time **[I, C10]**. The beta measures
it (D274 §3, §3.6).

### 3.3 Choosing among the paths (D273 §7 as amended; D290 §2; D291 §3)

The owner rejected moving to "a better one that answers shortly after": *"Simply moving to the next
better one that connects is moving the original problem one rung down rather than solving it. We
should be taking the first one that succeeds as a control connection, then giving a few seconds for
the other attempts to work or fail, and then out of all that succeeded, pick which one will work
best."* Then, on the audit (C5), how "best" is judged (D290 §2, D291 §3). So:

1. The client tries the paths it was given at once: the LAN name only when §3.2 returned it, and
   WebRTC with one connection (§2.5).
2. **The first that answers becomes the control connection**: the page can sign in and show the
   household over it at once, so nobody waits on the race. Over it the page reads the core's API
   major, and swaps to the client build for that major if it holds another (C16, §5).
3. **The others get a few seconds [I: about 3 s, to measure] to succeed or fail.**
4. **Of the paths that succeeded, the client ranks by kind** (D290 §2, D291 §3; X2): the LAN, then
   IPv6, then IPv4, then WebRTC, and within WebRTC a direct pair, then a server-reflexive one, then
   the owner's TURN (C5 item 2). Round-trip time breaks ties: the median of a few small requests
   over the open connection, not one sample (C5). A path whose round trip is more than three times
   and 50 ms over the best is demoted (D291 §3). **Throughput is not measured in the race**: a few
   seconds measure a fresh connection's congestion window, not the path (D290 §2, C5).
5. **The client moves its traffic there between requests**, never mid-sign-in or mid-chunk; fetches
   under way finish on their path, and the Watch Together socket stays where it is until its path
   fails, then reconnects with its session's resume (C5).
6. **It moves again only when its path fails** (a request error, or two timeouts in a row) **or the
   network changes** (D290 §2): it uses the next path still known to work and races again in the
   background. It races again on the browser's `online` and network-change events and when the page
   becomes visible after a long hide (C5).
7. **It remembers which paths worked, per network** (keyed on the `paths` answer's
   `clientPrefix`, the client's network prefix as the discovery sees it, §3.2, D293 §8), not
   globally (C5), so a later visit starts with them and survives the discovery being down.

`SwitchableFetch`'s comment still states D273 §7's replaced rule (C5); it follows this section.

### 3.4 What serving the client cross-origin asks of the core and the client (research A2)

The page's origin is the discovery's; the core's are its names. So:

- **CORS** for the configured client origins (a list, never a wildcard), no credentials mode,
  `Vary: Origin`, `null` never allowed, `Authorization` named in `Access-Control-Allow-Headers`
  (the Fetch standard leaves it out of `*`) (S18; the W report's "What is sound"). **The core
  learns its discovery's client origin from the discovery** (D293 §4): `GET /status` carries it as
  `clientOrigin`, accepted only as an `https` origin with no path, and shown on the System page. It
  gives the discovery no power it lacks, since the discovery already serves the client's code.
  Built, with the identity key's fingerprint beside it on the System page; the CORS that follows it
  is not. **D308** adds `loaderOrigin`, checked the same way: where the discovery serves the loader
  that hands a browser on to the core's own client origin, which `clientOrigin` then names. The
  core's CORS lets the loader's origin read only `/health` and ask for a hand-off code
  ([client-from-core.md](plans/client-from-core.md) §2.1).
- **Sign-in by header**, never a cookie, which would be a third-party cookie (the API already takes
  `Authorization: Bearer`).
- **Byte paths by the header on every request** (hls.js's loader, `fetch` and blob URLs for images
  and subtitles), no credential in a URL (D289 §3).
- **Credentials per core** (A8, W1). Every credential, device id, device secret, locator and
  remembered key is kept under its `coreId`. An API client is bound to one core and attaches its
  credential only to that core's names or its WebRTC transport; a URL a playlist names elsewhere
  goes without it. Joining another household's Watch Together sends no session: the joiner is
  anonymous there unless signed in to that core. A `TestArch_` claim enforces that no request
  leaves the client with a credential for a core other than its target [I: the claim's name is
  given when it is written]. **The code must change:** one token is kept for the whole origin
  (`session.ts`) and sent with every request.
- **The device proof is a device secret** (A8, code K1, W4): random, issued at the device's first
  sign-in, stored hashed on the device row, never a session token and never sent to a discovery; a
  device is resumed only on it. Built (2026-10-13): each sign-in, registration or code activation
  issues a new one, kept as an issued secret tied to the device (no schema change) and revoked with
  the device; the web client keeps it apart from the token. Later, a non-extractable WebCrypto key per session, as DPoP binds tokens [I, A8].
- **The Watch Together socket** accepts the client origins by `OriginPatterns`, never `*`; its
  credentials ride in `hello`, not cookies (the W report).
- **Local Network Access**: Chrome asks before a public page reaches a private address (Chrome 142
  for fetch, 147 for WebSocket; Firefox 153 for fetch, 154 for WebSocket) **[V, A corrections 9]**.
  WebRTC is gated in neither **[V, A17]**, so "the LAN only at home" holds for WebRTC's host
  candidates by the core's filter (§2.5), not by the browser. Racing IPv6 and WebRTC first does not
  avoid the prompt, since the LAN request starts at once (U1, C5); returning the LAN path only to a
  device at home does (§3.2). `targetAddressSpace: "local"` exempts non-secure URLs from mixed
  content; the LAN name is HTTPS, so it neither avoids nor changes the prompt (W errors 4). Requests
  from a service worker need the permission granted to the worker's origin first **[V, C5]**.
- **DNS rebinding protection** on many routers drops the LAN name's answer; the device then uses
  another path.
- **Version skew**: one client build per supported core API major (D289 §4), the page swapping
  builds when the core's major differs (§3.3); a service worker carries a remote kill switch, an
  update check that can unregister it (C16). Staging's builds never register at the production
  origin (coherence §5.3).
- **What the client's integrity measures do** (D290 §4; W2, the A report's reopened items). The paid
  web client is built reproducibly and the hash of every published build is listed publicly. It is
  served with Subresource Integrity on every script, `Integrity-Policy` requiring it, a strict
  content-security policy (`script-src 'self'`, no inline script) and Trusted Types, now in every
  engine (W2). These stop a swap by a CDN or cache and make tampering visible after the fact; they
  do **not** stop a compromised service: SRI and `Integrity-Policy` cover scripts and styles only,
  not the document, workers or the service worker, which registers with no integrity **[V, the A
  report]**. `index.html` is small and served `no-cache`, as is `/sw.js`, registered with
  `updateViaCache: "none"`, and their hashes are published with the build [I, W2]. Cloudflare's
  injected scripts stay off on the client's hosts (G13) [I]. What limits a compromised client is
  §3.7.
- **The service's own cookies** on `orpheum-app.com` are `__Host-` prefixed in production, so no
  other subdomain can set them (D292 §2). The client itself is cookie-free (D289 §3).

### 3.5 Watch Together, voice and native clients

Invites go through the discovery (§3.1). Voice is WebRTC between the people, its fallback relay the
core's own TURN or the owner's VPS, never the paid service's. Once a person grants the microphone,
Chrome sends raw host candidates, so guests' and hosts' LAN addresses reach each other in Watch
Together voice; the privacy page says so (A17) [I].

**The web client remembers a core's identity key** (D290 §3, S1; D292 §4). On first contact it
checks the fingerprint its link or QR carried (§3.1): over a direct path, it asks the core to sign a
nonce with its identity key (§2.1); over WebRTC, the answer's signature verifies under that key
(§2.5). It keeps the public key per `coreId`, and checks it on every later visit and every WebRTC
answer. Not built.

**What the remembered key does** (A7, T3, E9). On WebRTC it is enforced: the browser holds the DTLS
fingerprint the core signed. On HTTPS a page can neither see nor pin the server's certificate, so a
signed nonce stops only an impostor that cannot reach the real core; one holding a certificate for
the core's name can relay the challenge. A Safari tab re-learns the key after its storage is wiped
(§3.1), from the fingerprint in the person's link.

**A changed key**: one the old key signed is accepted (D291 §6; rotation is deferred, §2.8). One the
old key did not sign **blocks the client** until the person opens a fresh link from the core, which
carries the new key's fingerprint; it is never accepted on the discovery's word (D292 §4, revising
D291 §6's notice; A6 item 4).

Native apps (later, D273 §4) race the same paths, pin the identity key on first contact, and add
mDNS on the LAN and raw UDP hole punching; with the internet down they may reach the LAN address by
the pinned key (§2.1) [I, coherence §1.1].

### 3.6 When nothing reaches the core (D290 §12; U3)

When nothing direct reaches a household (NATs on both ends that map per destination and no IPv6, or
a network that allows only HTTPS to known names), only a relay could, and the paid service runs none
in the beta (§1). The message is written for whoever reads it:

- **Anyone but the admin** [I: wording, U3]: "Can't reach *Stenerud home* from this network. Try
  your phone's mobile data, or another Wi-Fi. If it keeps happening, tell *Karl*." A phone's hotspot
  is the likeliest fix in a hotel, since mobile networks mostly give phones IPv6 (D274 §4). The
  names are those the device remembers; on a first visit, which knows none, "the server" and "the
  person who sent you this link" (U2-7) [I]. The advice follows the device's network where the
  browser says it: Wi-Fi, try mobile data; mobile data, try Wi-Fi (U2-7) [I].
- **Not for every cause**: the message is shown only when the core is online and no path works.
  Household members are told the cause when it is the core's (D291 §6, U7): offline since a time,
  paused for a lapse, being restored. The `paths` answer carries that state as `state` (§3.2,
  D293 §6; U2-7). When Chrome's permission was denied and the device is at home, the page says so,
  with the help page (D291 §6) [I, U2-7]. At home it offers the home address (§3.1).
- **The admin**, on the System page, ahead of time from the probe: a public IPv4 from the ISP, or
  their own VPS as a `configured` address with its TURN (§2.4).
- **A tester can send what happened**: a "copy what happened" on the page (the race's results per
  path, ICE's candidate pairs and states, the network's kind), since the core keeps a household's
  facts and the owner sees nothing of a tester's failure (D73) [I, coherence §4.2].
- **The beta counts how often the message is shown**, by the client's kind of network, as an
  aggregate with no identifiers (D290 §12, D274 §3), so the owner sees the number before the paid
  launch.
- **The service counts failed connections per core** (D293 §7): the client reports a dead end to
  the discovery (§4's client API), which keeps a weekly count per core with no device identifier;
  the core reads it with its heartbeat's answer (§2.6) and the System page shows how many clients
  failed to connect last week (D290 §12, D291 §6's U3).

### 3.7 Who is trusted, and where admin is done (D292 §4; A1, T4, W2)

**On the web, the party that serves the client's JavaScript is trusted by every household that uses
it**: RFC 8826 §4.3 and RFC 8827 §6.2 say so of any calling service **[V, the A report's threat
model]**. The paid service also holds the zone, so it can get certificates for any core's names by
DNS-01. The design does three things within that trust: it keeps outsiders out, it makes a
compromise visible (signed keys and fingerprints, CAA, later CT), and it **caps what one compromise
yields**. Independence from the service needs the core's own origin, an own discovery, or a native
app.

- **A password sign-in carries the person's whole role wherever it comes from**, a discovery's
  client and WebRTC included (D294 §4, withdrawing D292 §4's "a session minted through a
  discovery's client never carries admin"): the client's own security is what protects every
  household's password, and an admin away from home administers through it. A device a code
  activated carries the `device` scope (§3.1). `rtcapi` still believes nothing a request's head
  says of where it came from (§2.5).
- **Admin works from anywhere** (D294, withdrawing D292 §4's "only from the core's own network",
  which a core in a container could not judge: it sees no LAN prefix of its own). The operator's
  way back in is `orpheum reset-admin` on the server's machine (D294 §2). **Later, a passkey**
  whose relying party is the core's own domain, which a page at `app.` cannot exercise, as an
  option the owner turns on, never required (D294 §3; A1).
- Admin pages on `app.` say "manage this server from home, at <its home address>" (§3.1).
  Destructive admin actions ask for the password again on the admin origin (A1 item 3) [I].

Built (2026-10-13): every code activation gets the `device` scope (`internal/core/auth`), and the
web client hides what such a session cannot do; `orpheum reset-admin` (D294 §2).

## 4. The API

Versioned by major in the path, JSON, additive by default (§5). Errors name their cause; a refusal
the core must act on (enrol again, another instance running) carries a code of its own (C12, C4).

### Core (`<discovery>/core/v1`): identity key → bearer token

| Method, path | Purpose | Built |
| --- | --- | --- |
| `POST /challenge` | public key → nonce, for enrolling or a session; rate-limited | yes |
| `POST /enrol` | public key + signed nonce → `coreId`, claim code, expiry; idempotent per key | yes |
| `POST /session` | `coreId` + `instanceId` + signed nonce → token bound to the core, `expiresIn` (1 h) | yes |
| `GET /status` | claimed?, account state, zone, protocol support, `iceServers`, the client origin (`clientOrigin`, §3.4) and the loader's (`loaderOrigin`, D308), messages for the admin | yes, but for the account state |
| `PUT /addresses` | candidates, the owner's TURN → published names, probe results, `reachableFrom`, last week's failed connections (§3.6); the heartbeat's fallback | yes |
| `GET /signal` (WebSocket) | offers (with proofs) in, answers (signed over the SDP) out; the heartbeat; pings | yes, without checks, signatures, heartbeat or pings |
| `PUT /acme-challenge`, `DELETE /acme-challenge/{name}` | set or clear `_acme-challenge.<coreId>.<zone>`, claimed cores only; returns once served | yes, but for waiting until served |
| `PUT /acme-account` | the core's ACME account URI (§2.7), before each order, for CAA's `accounturi`; claimed cores only | yes (2026-10-14); `single` takes it and keeps nothing, the owner's DNS holding its CAA |
| `PUT /locate-hashes` | the full set of live link-secret hashes, with lives and use policies | yes, `single` keeping them |
| `PUT /locators` | the full set of live device-locator hashes | yes, `single` keeping them |
| `DELETE /core` | leave; closes the connection and ends the tokens | yes |
| claim approval (§2.2) | the core's admin allows or refuses an account's claim | after the beta |
| re-key (§2.8) | a new key's enrolment, countersigned by the old key | deferred |
| paid device codes (§3.1) | the core claims a code the service issued, and passes back the outcome sealed by HPKE | no; in the beta (D293 §2) |

### Client (`<discovery>/client/v1`): a hash, code or locator the device holds; no account

| Method, path | Purpose |
| --- | --- |
| `POST /locate` | link-secret hash → paths + a bootstrap token; by the hash's use policy |
| `POST /paths` | locator or bootstrap token → current paths, ICE servers, the core's API major, its `state`, the client's `clientPrefix` |
| `POST /cores/{id}/offer` | locator + SDP offers (up to four), each with its proof → the core's answers, signed |
| device codes (§3.1) | own: the core's `/pairings`, passed through (D288); paid: issued by the service, the outcome sealed to the device (D293 §2) |
| dead-end report (§3.6) | a locator or bootstrap token → counted for its core's week, with no device identifier (D293 §7) |
| `GET /client-floor` | nothing → the client versions the discovery refuses, which its loader runs none of (D308 §6); an own discovery refuses none |

An own discovery answers the same client API for its one core. Built in `discovery/single`
(2026-10-14): `/locate`, with the hash's use policy and a bootstrap token of five minutes, and
`/paths`, with the core's `state` from its signalling connection and the client's `clientPrefix`,
WebRTC its one path until it publishes names. And `/cores/{id}/offer`, passing each offer on with
the hash it came under and its proof, and the dead-end report, counted over a week and told in the
heartbeat's answer (`deadEndsLastWeek`), which the System page shows.

## 5. Versions, and keeping the sides in step (D273 §6, §7)

For both APIs, core ↔ discovery and client ↔ discovery (native apps, cached clients and own
discoveries upgrade on their own schedule):

- **Additive by default:** new fields optional, unknown ones ignored, a new feature a new call; the
  core and the discovery never upgrade together.
- **One version signal** (C13): the path carries the major (`/core/v1`); the
  `Orpheum-Discovery-Protocol` header carries `major.minor`; no body carries one. A discovery
  accepts any minor of a major it serves and uses the minor only to know which optional features the
  core has; `/signal` checks the header too. Built (2026-10-13): `discoveryproto.ParseVersion`
  and `Serves`; `single` refuses another major, or none, with `unsupported_version` naming what it
  serves, on every call, `/signal` included.
- **A break is a new major** (`/core/v2`).
- **The window is promised from the public launch** (D292 §3), as D273 §7 states it: a major is
  served while it is one of the two before the newest, and in any case for at least six months after
  its successor shipped (R1). A core on a deprecated version says so on its System page with the end
  date (from `GET /status`); past the window, the refusal names the versions accepted. **In the
  beta** the protocol is unstable and testers update when told.
- **A security floor from the first day** (D292 §3): a core, or a client build, below it is refused
  after notice on the System page, window or not; each served client build receives security fixes
  for as long as its major is served (the A report's "The version window, in detail") [I: the second
  half].
- **One source of truth:** the public packages (D275 §1). A change lands there, then in the paid
  service accepting old and new, then in the core. The paid service imports Orpheum's working tree
  by `replace`, a pin file recording the commit it was built from (D292 §9).
- **Tests both ways:** the core's client runs against `discovery/single` as its reference (built);
  a conformance suite exported as `discovery/discoverytest` runs the protocol's cases against
  `single` and the paid service's handlers alike (devloop §3.2, taken with D292 §9's loop); the
  paid service's CI builds the newest and the oldest supported core and runs enrol → claim →
  heartbeat → offer → leave against it.

## 6. Answered by the owner (2026-10-13)

- **The paid service's questions that bind the protocol**, decided in
  [D289](../decisions/working-notes.md#d289--the-paid-discoverys-questions-answered-accounts-for-payers-only-the-identity-key-backed-up-as-the-secret-key-is-byte-paths-by-header-one-client-build-per-core-api-major):
  accounts for payers only, household users signing in to their core (Q5); the identity key backed
  up as `secret.key` is (Q12; revised by D292 §4, §2.1); byte paths by the header (Q17, §3.4); one
  client build per supported core API major (Q18, §3.4, §5). Q19's check code at claiming is
  replaced by claiming from the core (D290 §1, §2.2).
- **The first round's findings**, ruled on in D290: claiming from the core (§1; §2.2, after the
  beta); paths ranked by kind, the LAN name only at home (§2; §3.2, §3.3); re-keying signed by the
  old key, a slow recovery, the key remembered by the client, CT watched (§3; §2.8, §3.5, §2.7); the
  central client's integrity (§4; §3.4, its admin rights revised by D292 §4); sealed device-code
  sessions, the device's place shown, a narrower role (§5; §3.1); signalling checked by the core,
  idle connections closed, signed fingerprints (§6; §2.5); available DNS, CAA (§7; §2.7; its
  computed answers, DNS-PERSIST-01 and ARI revised or deferred by D292 §1, §3); hosting (§8, the
  spec; revised by D292 §3); TCP only for port mapping (§9; §2.4); a way back for a device (§10;
  §3.1); recovery after a restore (§11; §2.8); the dead-end message (§12; §3.6); the wire's
  spellings in line with the code, and the core's id kept at 130 bits (§13: the document otherwise
  keeps the design and marks where the code must change, as §18's correctness findings recommend,
  X22); WebRTC in the beta (§14); the two flaws fixed (§15; built); the customer's sign-in link
  (§16, the spec; after the beta); Watch Together links (§17; §3.1); one core per account in the
  beta and two cores with one key refused (§18; §2.3, the spec); the remaining correctness findings
  as the audit recommends, but for C12's check code, which D290 §1 replaced (§18; R2).

## 7. Decided after the first round (D291)

What the first round recommended that D290 neither decided nor followed from, decided by
[D291](../decisions/working-notes.md#d291--what-the-discovery-audits-raised-and-d290-left-open-taken-as-the-audits-recommend-the-probe-on-a-machine-of-its-own-dns-on-bunny-ipv6-before-ipv4):
each item is taken as its audit recommends, except where noted. Each names its finding; where D292
later changed an item, it says so.

1. **S2 items 3, (part of) 2:** WEBCAT enrolment later; Trusted Types; keeping `app.` free of
   third-party scripts and analytics (the CSP implies the first part).
2. **S4 items 2–4:** each core limiting how many codes it claims an hour; the device confirming the
   core's name before it takes its session; where the approximate place comes from (a geolocation
   database is a dependency, the owner's to approve when it is built). The narrower role became one
   `device` scope for every code (D292 §4; §3.1). Paid codes, deferred by D292 §3, are in the beta
   (D293 §2); the place line waits until after it, since it needs an IP-location database.
3. **S5 item 2:** two kinds of WebRTC connection, a signed-in client proving its session in the
   offer, and unproven ones limited to signing in, registering, pairing, joining Watch Together and
   device codes (G4), with a small quota and a short life (§2.5).
4. **S1 item 2:** a fresh authentication (step-up) before re-key and removal. With the accounts,
   after the beta; minting a locator for the customer's browser was dropped with that endpoint
   (U2 errors 3). Kept light when built: a passkey where one exists, one step-up for a window, never
   stacked on another check (U2-10) [I].
5. **S7 items 3–5:** a *This isn't me, release* action on the System page, signed by the core; the
   typed claim code kept as a fallback, with a dashboard warning and a per-account limit on
   attempts. After the beta, with claiming from the core (§2.2); in the beta the typed code is the
   way (D292 §3).
6. **S10:** the outside probe's limits (§2.6). **D291 §1:** the probe on a machine of its own,
   holding no secret but a token that can only post results; the spare holds the database. **D292
   §3:** the spare waits for the public launch.
7. **S12 items 2, 4, 5:** a zone under an HSTS-preloaded TLD, to be had by submitting
   `orpheum-core.com` to the preload list (D292 §1), then not sought: the apex serves no HTTPS and
   each core's name sends its own HSTS (D293 §10); registry lock, hardware-key 2FA, DNSSEC and a
   narrow API token. **D291 §4:** no PSL entry now.
8. **S13 item 1:** a cap on published names per core, which matters again now that each name is a
   record (§2.7).
9. **S14 items 1–4:** not a random high external port: the router is asked for 443 first and the
   port it grants is taken when 443 is held, since networks that allow only 80 and 443 reach no
   other (D291 §7); "your server is reachable from the internet at …" shown prominently;
   security-update notices through `GET /status`; sign-in limits tested over WebRTC.
10. **S15:** a `Host` allow list on the core's plain-HTTP LAN listener against DNS rebinding (built
    2026-10-14, also taking names public DNS never answers, such as `nas.example` [I]).
11. **S16:** at most about ten candidates per offer; loopback and link-local remote candidates
    dropped from offers through the discovery; STUN answering Binding requests only, rate-limited
    per source, without `SOFTWARE`. A2 made the candidate limits a must and widened the filter
    (§2.5).
12. **S17:** logging device requests without the core id or with a truncated address; saying on the
    privacy page that names carry addresses.
13. **S18:** checking the TXT value is a 43-character base64url digest and clearing it after an
    hour; CORS sending `Vary: Origin` and never allowing `null`.
14. **U1:** a line explaining Chrome's prompt before the LAN name is tried; a help page for undoing
    a block; preferring WebRTC's host candidates for the LAN.
15. **U2 (rest):** "Open in Safari" when an in-app browser is detected; an admin's link for a known
    person signing them in rather than resetting the password.
16. **U3 (rest):** the owner's own TURN as one switch; the System page saying how many clients
    failed to connect last week.
17. **U5:** plain names (*Remote access*) and the discovery's address hidden behind "use my own",
    which must keep working, since staging connects through it (infrastructure §3 change 3); the
    first-run setup code accepted in the browser.
18. **U7:** lapse and outage messages with dates and *Renew*; household members told the cause. The
    last good LAN address kept for an outage became the home address (D292 §6; §3.1).
19. **U8:** a list of locators with a switcher for a person on two servers. **After the beta**
    (D291 §6).
20. **U9:** a QR beside the TV's code; a default device name; picking the check code from three;
    renaming *Get a code*; a two-sentence warning.
21. **U11, U12:** "Connecting to …" after a second; a "how this device is connected" line; mail to
    the customer when `reachableFrom` drops (with the accounts, after the beta).
22. **Correctness, "something simpler" (not findings):** the heartbeat carried on the signalling
    connection; the hourly probe dropped for probe-on-change and clients' reports (§2.4, §2.6).
23. **S1 item 3, after a re-key:** a client that remembered the old key accepts the new one when the
    old key signed it (D291 §6). After a key recovered without it, the client blocks until a fresh
    link from the core (D292 §4, replacing D291 §6's notice; §3.5).
24. **C5 item 1 (part):** a path whose round trip is far worse (more than 3× and 50 ms over the
    best) is demoted, and IPv6 ranks before IPv4 (D291 §3).

## 8. Decided after the third round (D292)

The owner took the consolidated plan's recommendations
([D292](../decisions/working-notes.md#d292--the-third-audit-round-taken-no-admin-through-a-discovery-a-thin-path-first-two-machines-staging-inside-the-owners-own-names-the-household-zone-at-orpheum-corecom)),
with two changes: the zone at `orpheum-core.com` on Cloudflare, and staging inside the owner's own
names. In this document:

- the zone, its written records and its CAA (§1; §2.7), staging's names (§1), production's
  `__Host-` cookies (§3.4);
- the beta scoped down (§3): a thin path working end to end on staging first (the order of work is
  [discovery-core.md](plans/discovery-core.md)'s); no customer accounts, the operator claiming each
  core by its code (§2.2); two production machines, `prod-api` serving the client and `prod-probe`
  (§1, §2.6, §2.9);
- **deferred** (§3): mailbox recovery, re-keying (§2.8), CT watching, ARI and DNS-PERSIST-01 (§2.7),
  the NAT measurement (§2.6), HPKE device codes (§3.1; brought back into the beta by D293 §2); the
  version window promised from the public launch, a security floor from the first day (§5);
- security (§4): no admin through a discovery's client (§3.7); offers checked and bounded, and
  `single`'s K13–K15 closed, before any signalling connection opens (§2.2, §2.5); a recovery code
  sealing the identity key in scheduled backups (§2.1); an unsigned key change blocking (§3.5); the
  fingerprint in every link and QR, answers signed over the exact SDP (§3.1, §2.5); one `device`
  scope (§3.1); a used registration link becoming the person's own (§3.1);
- no relay in the beta (§5; §1); every household told its core's home address (§6; §3.1); the
  household's own VPS kept as a `configured` address with its TURN (§2.4);
- the development loop (§9): the paid service importing Orpheum by `replace` and a pin (§5), and
  the signalling reconnect starting near 1 s (§2).

## 9. Still the owner's

Questions no decision answered after the third round, each now answered by
[D293](../decisions/working-notes.md#d293--the-discoverys-open-questions-answered-a-live-instance-is-pinged-before-it-is-replaced-device-codes-through-the-service-sealed-by-hpke-on-p-256-in-the-beta-stagings-dns-token-in-the-sandbox-until-production)
or left until after the beta. None is open for the beta.

1. **When an instance stops being live** (§2.3; G9, T14, U2-12): its signalling connection's
   closing, two missed heartbeats, or "the newest wins" in the beta (coherence decision 7).
   *Answered* by D293 §1: a second instance makes the discovery ping the first on its signalling
   connection and wait about 10 seconds; no answer, or no connection, and the newcomer replaces
   it, an answer and the newcomer is refused as another copy (§2.3).
2. **A lost key in the beta** (§2.8): the operator re-keying by hand, as coherence §4.1 proposes,
   or nothing until re-keying is built. *Answered* by D293 §5: the operator re-keys by hand, after
   checking with the household.
3. **Recovery when it is built** (§2.8): D290 §3's 72 hours, or the third round's seven days with an
   automatic veto and a re-key by the recovery code (A6). *After the beta* (D293 §5): it waits on
   its own design, made when recovery is built.
4. **Where the core's state reaches a household member** (§3.6): a `state` field in the `paths`
   answer (U2-7), or another call. *Answered* by D293 §6: a `state` field in the `paths` answer
   (§3.2).
5. **The minimum browser for paid device codes** (§3.1): Safari 18.4 for X25519, or a library (a
   dependency) (the A report's decision 12). *Answered* by D293 §2: HPKE with DHKEM(P-256,
   HKDF-SHA256), which Web Crypto has had for a decade, so no minimum browser rises and no library
   is needed; and paid device codes are in the beta.

D293 also settled what the documents named in place without listing here: the core learning its
client origin from `GET /status` (§3.4, §4), the service counting failed connections (§3.6), the
`paths` answer carrying the client's network prefix (§3.2, §3.3 item 7), the zone's apex serving
no HTTPS and no HSTS preload (§2.7), and staging's subzone and DNS token (§1).
