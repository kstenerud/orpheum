> **ABOUTME:** The discovery protocol: how a core, its clients and a discovery speak, the same for
> the paid service at app.orpheum-app.com (many cores) and an owner's own `orpheum discovery` (one
> core). Moved here 2026-10-13 from the paid service's spec, which keeps billing, accounts, abuse and
> its architecture. Revised the same day for the owner's rulings on the four audits (D290).

# The discovery protocol

**Status:** a design, partly built. Moved into the repository on the owner's word (2026-10-13:
*"move the protocol parts into the repo"*) from the paid service's specification (draft 3, kept in
the paid service's own repository), whose §3, §4, §8 and §9.3 it was. It is what the core
(`internal/core/discovery`), Orpheum's public building blocks (`discovery/discoveryproto`,
`discovery/proof`, `discovery/safe32code`, `discovery/single`) and the closed service all speak
(D275 §1). The plans that build it are [discovery-core.md](plans/discovery-core.md) (the core's
side, built), [discovery-mode.md](plans/discovery-mode.md) (W4b, the owner's own discovery) and
[webrtc-transport.md](plans/webrtc-transport.md) (the client's paths).

**Revised 2026-10-13** after four audits, done before any development on the owner's request:
[security](research/discovery-audit-security-2026-10-13.md) (findings S1–S18),
[ease of use](research/discovery-audit-ux-2026-10-13.md) (U1–U13),
[correctness](research/discovery-audit-correctness-2026-10-13.md) (C1–C16) and
DDoS and availability (kept with the paid service, in its own repository). The owner ruled on
them in
[D290](../decisions/working-notes.md#d290--the-discovery-audits-findings-taken-claiming-on-the-core-paths-ranked-by-kind-re-keying-signed-by-the-old-key-computed-dns-behind-cloudflare-and-webrtc-in-the-beta),
which this document follows; the correctness findings D290 does not name are taken as that audit
recommends (D290 §18). What the audits recommended and D290 did not decide is listed in §7, decided
by D291.

**Markers.** `D<n>` is a decision; a claim citing one comes from it. **[I]** is an inference or
recommendation not taken from a decision. **[V]** is a fact checked at its source, in
[reachability-approaches-2026-10-09.md](research/reachability-approaches-2026-10-09.md) ("the
research") or in the audit report its finding id names. **Built** says what exists today; **the
code must change** marks where the code disagrees with a design that stands. **Owner** marks a
question still the owner's. Finding ids (S5, U2, C3…) name the audit finding a rule answers.

**Words.** A **discovery** is either the paid service or an owner's own. Where the two differ the
text says **paid** or **own**. A **core** is a household's Orpheum server; a **client** is a browser
or app; a **device** is a client that has found its core.

## 1. What a discovery does, and does not

It connects a household's clients to that household's core and carries none of their traffic
(D273 §2, D214 §1):

1. **Serves the web client** (paid: at `app.orpheum-app.com`; own: at the owner's domain, D274 §5).
   After that, every API call, chunk, image and socket goes from the browser to the core. The paid
   client is built reproducibly and served with integrity and content-security policies (§3.4,
   D290 §4).
2. **Names and certificates** (paid only): hidden per-core names under a dedicated zone, answered
   by computing them from the name; the core runs ACME and keeps its key, the discovery only writes
   the DNS records that prove the core's account (§2.7). An own discovery uses the owner's domain.
3. **Where the core is now:** the core reports candidate addresses; the discovery tests them from
   outside and hands the working ones to the core's devices (§2.4, §2.6).
4. **WebRTC signalling and STUN** for a core no HTTPS path reaches: offers and answers over one
   connection the core keeps open, a few kilobytes each, never media (D273 §7, §8; §2.5, §2.9).
   It is in the paid service's closed beta (D290 §14).
5. **Finding the core:** links, QR codes and device codes lead a device to its core, and a
   reusable link per person leads it back (§3.1).

It does not relay anything (paid: no TURN, ever, D273 §2), hold household data, sign household users
in (they sign in to the core, through the client), carry their sessions in a form it can read
(§3.1), or list cores (D214 §3, D187 §2).

**What an outage stops** [I, from the availability research]. Once a client has reached its core,
the discovery can go down and playing goes on. But the zone's **DNS is on the path to playing**: a
returning device finds its core by a name under the zone, so the zone must keep answering (§2.7,
D290 §7). A household that only WebRTC reaches needs the discovery's signalling for every new
connection, though not for one already open. And a returning device rides out an outage only if it
still remembers its paths: Safari deletes a site's storage after seven days without a visit
**[V, U2]**, so a Safari tab needs the discovery again after a week away (§3.1's way back).

## 2. Core ↔ discovery

All calls are HTTPS to `<discovery>/core/v1/…`, JSON, outbound only; the discovery connects in only
to probe (§2.6). **Built** (`discovery/discoveryproto`, `internal/core/discovery`): JSON keys are
camelCase; every call carries `Orpheum-Discovery-Protocol: 1`; an answer's
`Orpheum-Discovery-Deprecated: <date>` header is shown to the core's admin. §5 says how the version
signals change (C13).

Every retry the core makes, and the signalling connection's reopening, backs off exponentially with
jitter, and the five-minute heartbeat carries ±20% jitter, so a discovery's restart is not met by
every core at once (C7). **The code must change:** it waits a flat 30 s, with no jitter.

### 2.1 The core's identity key (D214 §4) — built

`internal/core/identity`: an Ed25519 key pair made on first start, kept as `identity.key` beside
`secret.key` (D194), mode 0600, never in the database. It is backed up as `secret.key` is: out of
scheduled backups, in *Back up now* when its box is ticked (D289 §2); the backup page says what
leaving it out costs (§2.8) [I, U6]. It is not the TLS key. Its fingerprint, `SHA-256(public key)`
in safe32 groups, belongs on the System page (not built).

It signs, each under a purpose of its own (`discovery/proof`), and nothing else:

- enrolment and session challenges (§2.2, §2.3; built);
- a re-key: the new key's enrolment, countersigned by the old key (§2.8; not built);
- a client's challenge on first contact, so the web client can remember the key (§3.5; not built);
- the DTLS fingerprint of each WebRTC answer (§2.5; not built).

**Signatures are bound to the discovery** (C8): what is signed is
`orpheum-discovery-v1:<purpose>:<discovery origin>:<coreId or public key>:<nonce>`, the origin being
the base URL the core itself called, so a discovery cannot pass another discovery's challenge to
the core and come away with a session there. Nothing is deployed, so this changes within v1. **The
code must change:** `proof.Message` signs `orpheum-discovery-v1:<purpose>:<nonce>`.

### 2.2 Enrolment and claiming — enrolment built; claiming from the core not built

1. **Enrolment** (built). `POST /challenge {publicKey}` → `{nonce}`;
   `POST /enrol {publicKey, protocol, nonce, signature}` → `{coreId, claimCode, claimExpiresAt}`
   (Unix ms). A nonce works once. Enrolment is idempotent per public key: the same key gets the same
   pending core and `coreId`, with a fresh code once the old one expired (C12; `single` does this).
   A double click on *Connect* enrols once (C12; **the code must change**: nothing serialises
   `Connect`).
2. **Paid: claiming starts on the core** (D290 §1, replacing D289 §6's check code compared on two
   screens; S7, U5). The admin, signed in to the core's System page, chooses to connect it. The core
   enrols and opens, in the same browser, `https://account.orpheum-app.com/claim#<claim token>`: a
   token of full 128-bit entropy that lives a few minutes (S7), so there is no code for anyone to
   read out. The customer signs in if they need to, and the page shows the core's name and
   fingerprint and asks whether to add it to their account.
3. **The core's admin approves.** After the customer confirms, the core's System page (polling
   `GET /status`) shows which account asks to hold it, its address masked, with *Allow* and
   *Refuse*; only *Allow* from a signed-in admin of the core binds it (D290 §1, S7). From then on
   `GET /status` names the holding account, masked, and the System page shows it (D290 §1).
4. **Own:** the claim code stays 8 characters of safe32 shown `xxxx-xxxx` (`discovery/safe32code`),
   30 minutes, once, and the owner runs `orpheum discovery claim <code>` on the VPS (D274 §5). The
   owner types it on a machine they control, so nothing else is needed (S7). **Not built:** the
   command; `single.Claim` exists, used by its tests only.
5. The core, polling `GET /status`, sees *claimed* and goes on to §2.3–§2.7.

**An expired code is replaced, not deleted** (C12, bringing the document in line with `single`): a
pending core asking `GET /status` after its code expired is given a new one, and the System page
says so ("The code expired; here is a new one", U5).

**A stranger cannot displace a pending core** (C12, S18): while a pending core's code is live, an
own discovery refuses another key's enrolment with 409. **The code must change:** `single`'s
`enrol` replaces a pending core with any key that signs. A refusal that means "enrol again" carries
an error code of its own, which the core acts on: it re-enrols and shows the new code (C12; **the
code must change**: the core treats the bare 403 as an ordinary failure and retries forever).

Enrolment is rate-limited per source network (an IPv4 address or an IPv6 /64; paid), and on the
paid tier only a claimed core gets names and a certificate (S13).

### 2.3 Session — built

`POST /challenge` → `{nonce}`; `POST /session {coreId, instanceId, nonce, signature}` →
`{token, expiresIn}`, an opaque bearer token for one hour, hashed at rest. Every other call sends
`Authorization: Bearer …`, never a token in a URL (D154 §2). No mTLS, which complicates proxies, and
no signature on every request.

- **`instanceId`** is random, made at each start of the core, kept in memory and never backed up. A
  discovery holds one live instance per `coreId` and refuses a second with a named error ("another
  server is running with this identity"), which both System pages show; the admin chooses which to
  keep and re-keys the other (D290 §18, C4). Not built.
- **Expiry as a duration** (`expiresIn`, seconds), counted on the core's monotonic clock, so a core
  whose clock is wrong neither renews on every step nor sends expired tokens (C11). **The code must
  change:** the answer carries `expiresAt` (Unix ms), compared with the wall clock.
- **Renewal.** The core renews five minutes before expiry, and **at once, once, on any 401** from any
  call, then backs off; one renewal runs at a time, the step loop, the signalling loop and the
  certificate job sharing it (C11). **The code must change:** it waits 30 s after a 401, the
  signalling loop redials with its stale token, and three callers can renew at once.
- **Tokens outlive a discovery's deploy** (C11): the paid service keeps them hashed in its database,
  so a deploy does not cut every core's signalling for a heartbeat's length. `single` keeps them in
  memory and must prune them (**the code must change**: its token map is never pruned).

### 2.4 Heartbeat and addresses — built for interface and mapped candidates

`PUT /addresses` every 5 minutes (with jitter), and at once when an address or mapping changes:

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

As built the body also carries `"protocol": "1"`; it is dropped (C13, §5).

- `interface`: an address on the machine (a global IPv6 address is native on DS-Lite). **Only stable
  addresses** (C7): on Linux the core reads its addresses through netlink and skips temporary,
  deprecated, tentative and failed ones, preferring the stable (RFC 7217 or EUI-64) address, since
  a temporary address lives a day or two and a router's IPv6 port sharing follows the stable one
  **[V, C7]**. Container bridges are skipped. **The code must change:** it reads
  `net.InterfaceAddrs`, which carries no flags, polled every minute.
- **Non-public IPv4 is never public** (C7, C9): `100.64.0.0/10` (carrier NAT), `192.0.0.0/29`
  (DS-Lite) and the documentation ranges are classed non-public, which Go's `netip` does not do.
  **The code must change.**
- `mapped`: a public IPv4 and port from the router by UPnP IGD, NAT-PMP or PCP, **on by default**,
  shown on the System page, with a switch (D273 §7; built, `internal/core/portmap`). **It maps the
  core's HTTPS port over TCP only, and never a UDP port** (D290 §9): Plex servers were used to
  amplify attacks through a port UPnP opened **[V, availability research]**. A router may grant
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
  - the System page says which of the common cases it met (a FRITZ!Box grants mappings only to a
    device the owner allowed, **[V, C9]**).

  **The code must change** for each of these.
- `configured`: an address the admin set: a port-forward, or their own VPS.
- `lan` addresses are published unprobed; they matter only at home (§3.2).
- `turn`: a TURN server the owner runs, passed to the core's clients; the owner's relay, never the
  paid service's. Its credentials are **time-limited** (the TURN REST pattern: a username of
  `<expiry>:<id>`, a password that is an HMAC under a secret the core and the TURN server share),
  minted by the core and refreshed with each heartbeat, never the static long-term ones (C14). Not
  built: the core never sends `turn` today.
- The discovery records the request's source address too, but never publishes it unprobed: behind
  carrier NAT it is shared and unreachable. Behind a proxy (the paid service sits behind Cloudflare)
  the source is taken from `CF-Connecting-IP`, trusted only from Cloudflare's ranges [I,
  availability research].

The discovery publishes only public candidates its probe passed (§2.6), and keeps current addresses
only, no history. **Liveness is the signalling connection's presence** (C16): a core whose
connection closed is *offline* at once; one that has none and sent no heartbeat for 30 minutes is
offline too. Its names keep answering either way (§2.7). **The code must change:** `single` marks
nothing offline.

### 2.5 The signalling connection (D273 §7, §8) — built on the core

One WebSocket the core keeps open, `<discovery>/core/v1/signal`, with the session's token. It
carries only WebRTC offers and answers:

1. A client posts an SDP offer with its locator (§3.1), or the bootstrap token `/locate` gave it.
2. The discovery checks the locator and forwards `{"type": "offer", "offerId", "sdp",
   "locatorHash"}` (text messages, one JSON object each, at most 64 KiB). One message may carry up
   to four offers, so a client's connections need one round trip (C6) [I: the shape is the build's].
3. **The core checks the locator itself** (D290 §6, S5): it matches `locatorHash` against the set it
   registered (`PUT /locators`, or for a bootstrap token, the locate-hash it came from) and refuses
   one it does not know. The discovery's check stays as the first filter. A bug in the discovery, or
   a compromised one, then opens no connection into the core.
4. The core answers as `internal/core/rtcapi` answers its own `POST /rtc/offer`:
   `{"type": "answer", "offerId", "sdp", "fingerprintSignature"}`, or
   `{"type": "refusal", "offerId", "reason"}`. **`fingerprintSignature`** is the core's identity key
   over its answer's `a=fingerprint` value (`orpheum-discovery-v1:dtls:…`, bound as §2.1 says), which
   the client checks against the key it remembers (§3.5) or the fingerprint a pairing QR carried, so
   no discovery can sit between a client and its core on this path (D290 §6, S6). Not built.
5. The discovery returns the answer; the data channels then run browser to core.

**Bounds** (D290 §6; S5; C6, taken):

- Offers are limited per locator at the discovery, in rate and in how many may be open at once (at
  most 8 offers or connections per locator, S5), and per core.
- **The core answers offers concurrently**, each under the connection limit, so one slow gathering
  does not hold the next (C6, S5). **The code must change:** the signalling loop answers one at a
  time, each waiting up to 5 s.
- **Half-trickle** (C6): each side sends its description once its host candidates and its first
  server-reflexive candidate are in, or after about a second, whichever comes first, rather than
  waiting for gathering to complete (the client waits up to 10 s today, the core 5 s and then
  refuses). **The code must change.**
- **A slot is counted once ICE connects**, an unconnected offer bounded to about 10 s; the cap is
  sized by memory, not a fixed 64, and the System page says when it refused anyone (C6). As built a
  slot is held from the offer until ICE fails, about 30 s, and 64 at four per client is 16 clients.
- **Idle connections close** (D290 §6, S5): a connection that has carried no request with a valid
  sign-in for a few minutes is closed, so no one holds the core's slots by opening connections and
  sending nothing.

**A connection grants nothing**: every request over a channel carries its own bearer token to the
core.

**Recovery** (C6, taken; webrtc-transport W2):

- The client races WebRTC with **one** connection, and opens the others only once WebRTC is the
  chosen path, keeping each that connects (not all-or-nothing) and retrying the rest.
- It watches each connection's state, takes a failed one out of the rotation at once, and replaces
  it. **The code must change:** `connectRtc` needs all four, and a failed connection, answering at
  once, attracts every request in the least-loaded choice.
- **ICE restart** when a connection is `disconnected` for more than a few seconds, its offer going
  through the discovery (or over the data channel while it lives), the existing channels carrying on
  meanwhile (RFC 8445 §9 **[V, C6]**); a new race when it is `failed`. Not built: every offer makes a
  new connection.
- The core pings its signalling connection every 25–30 s and treats two missed pongs as dead, and
  reopens it within seconds of its closing, with jitter (C11). **The code must change:** no ping;
  a flat 30 s.

**Leaving, re-keying, revocation and disabling close the connection** and invalidate the core's
tokens at once (C4). **The code must change:** after `DELETE /core` the socket and tokens stay live.

### 2.6 The outside probe

After each address change, and hourly, the discovery tests each public candidate from a probe
machine on another network: HTTPS to the address with the candidate's name as SNI, the certificate
checked, an echo of a nonce the core issued (the check of `internal/core/reachability`, from
outside, a failure naming its hop, D152); and the owner's TURN servers.

**The core's NAT is measured, not guessed** (C10). A probe that punches from a data centre's public
address passes whenever the core can send UDP out, so it says little of clients behind their own
NATs. Instead the core sends STUN binding requests to two STUN servers on different addresses (the
discovery's and the probe machine's) and compares the mapped ports, as RFC 5780 describes: the same
port means endpoint-independent mapping (punching works with most clients), different ports mean
endpoint-dependent (punching works only with clients that have IPv6 or an open NAT).

The heartbeat's answer carries the result, each probe as fields, not free text (C16):

```json
{"published": ["2a02-8070-0-0-0-0-0-1.k7x…q2.<zone>", "203-0-113-7.k7x…q2.<zone>:51234"],
 "probe": [{"address": "2a0f:1000::1", "port": 443, "kind": "https", "result": "reached"},
           {"address": "198.51.100.9", "port": 443, "kind": "https", "result": "refused"}],
 "reachableFrom": "ipv6_or_webrtc"}
```

`reachableFrom` is, as built, `everywhere` (a public IPv4 path works), `ipv6_or_webrtc` or
`nowhere`; the core shows it on the System page, with what a household nothing reaches can do (§3.6).
**It is to be named by what was tested** (C10): a public IPv4 on 443, a public IPv4 on another port
(which networks allowing only 80 and 443 do not reach), IPv6, punchable with endpoint-independent
mapping, punchable only from an open NAT, nowhere [I: spelt in snake_case, as the built values
are]. **The code must change** for the fields and the values. As built, `single` publishes nothing
and probes nothing (W4b.4).

### 2.7 Names and certificates (D273 §7, research B1, §9.2; D290 §7) — the core's side built

- **Paid:** names under a dedicated registered domain, never `orpheum-app.com`: every core holds a
  certificate there and could otherwise set cookies for the service's pages.
- **Names encode addresses** (Plex's shape): `<address>.<coreId>.<zone>`, the IPv4 address dashed
  (`192-168-1-20`) or the IPv6 address as eight dashed groups, lowercase, no zero padding and no
  compression (`2a02-8070-0-0-0-0-0-1`), so one address has one name and no label holds `--` (C15).
  The encoding is one function in `discoveryproto`, with test vectors, which the paid service,
  `single` and the client share (C15; not built: the core sends addresses compressed). A port other
  than 443 goes in the URL.
- **`coreId` is 26 safe32 characters (130 bits)**, random, never reused for another key (§2.8).
  **The code must change:** `single` makes 8 characters (40 bits) (D290 §13).
- **One wildcard certificate per core**, `*.<coreId>.<zone>`: an address change needs no new
  certificate, and Certificate Transparency shows only the random label.
- **Answers are computed from the name, from the start** (D290 §7, C2): a name's A or AAAA answer is
  its own label, with no record per address, so no record limit (Cloudflare's free plan holds 200
  records, about 50 cores **[V, C2]**) and no clean-up. They are given only for `coreId`s that exist
  and are not released (C2), so the zone is not a rebinding resolver for anyone's addresses, and
  `lan` names only for private ranges, never `127.0.0.0/8` [I, S12]. Answers carry long TTLs, since
  a name's answer never changes [I, availability research].
- **DNS is on the path to playing and is kept available** (D290 §7): the zone is served by an
  anycast provider that absorbs floods; self-run name servers only as a second provider, never alone
  [I, availability research]. Which provider: the paid service's spec.
- **DNS-PERSIST-01** (D290 §7, C1): at claiming, the discovery writes one standing record,
  `_validation-persist.<coreId>.<zone>`, naming Let's Encrypt and the core's ACME account URI, with
  `policy=wildcard`; renewals then need nothing from the discovery. A lapse maps onto it: `frozen`
  sets `persistUntil` to the end of the frozen period, `released` deletes it. Whether Let's Encrypt
  runs it in production yet is unconfirmed **[I, verify; C1]**; until it does, DNS-01 by proxy is the
  method, and it stays the fallback.
- **DNS-01 by proxy** (built): the core runs ACME (`internal/core/certificate`) and calls
  `PUT /acme-challenge {name, value}`; the discovery checks the name is
  `_acme-challenge.<coreId>.<zone>` for this core, sets it, and the core asks the CA to validate,
  then clears it. The core keeps its ACME account, its key and its certificate. Taken from C1: the
  call returns only once every authoritative server serves the record, and the core checks it
  resolves before asking the CA (as built it waits a fixed 30 s). Clearing names the record in the
  path, not in a body, which DELETE does not define (C16; **the code must change**).
- **CAA naming the core's account** (D290 §7, S12, C1):
  `<coreId>.<zone> CAA 0 issuewild "letsencrypt.org; accounturi=<the core's ACME account>;
  validationmethods=dns-01"` (RFC 8657), so only the core's own ACME account can get its
  certificate, whoever can write a TXT record. The core reports its account URI (§4).
- **Renewal by ARI** (D290 §7, C1): the core renews in the window Let's Encrypt's renewal
  information gives, which is also exempt from its rate limits, and otherwise at two-thirds of the
  lifetime. **The code must change:** it renews 30 days before expiry, fixed, which for a certificate
  shorter than 30 days would reissue on every sweep.
- **Let's Encrypt's limits** (50 new certificates per registered domain a week **[V, C1]**) bite on
  new cores, not renewals; a limit increase is asked for early (D290 §7). A Public Suffix List entry
  is not the answer: the PSL refuses entries whose purpose is to get around rate limits **[V, C1,
  S12]**. Lifetimes shrink to 64 days in February 2027 and 45 in 2028, authorisation reuse to 7
  hours **[V, C1]**: without DNS-PERSIST-01 every renewal of every core needs the discovery live.
- **The core watches Certificate Transparency** for its own names and shows its admin a certificate
  it did not ask for as an alarm (D290 §3, S1). Not built.
- After leaving, the core stops serving the certificate with the rest (C16; **the code must
  change**).
- **Own:** the names are the owner's domain's, set through the owner's DNS provider (W4b, the plan's
  Q2).

### 2.8 Revocation, re-keying, recovery and leaving

- **Removing or leaving** (`DELETE /core`): names stop answering, records removed, locators dropped,
  the core marked gone, its signalling connection closed and its tokens invalidated (C4). Its
  `coreId` is never given to another key, so an old link never reaches someone else's server.
- **Re-key by rotation** (D290 §3, S1): while the old key exists, the new key's enrolment is
  countersigned by it and takes effect at once.
- **Re-key by recovery**, when the old key is lost: it takes effect only after about 72 hours, with
  mail to the customer at the start and the end (D290 §3), and a banner on the dashboard; while the
  old key still holds a session, the old core may cancel it from its System page (S1). An own
  discovery's owner re-keys on the VPS (`orpheum discovery rekey`, C4), which needs a shell there.
- **What a re-key does** (C4): the discovery binds the new key to the existing `coreId`, revokes every
  token of the old key and closes its connection; the core with the new key gets a **new
  certificate** and revokes the old one, whose key went wherever the identity key went. Devices'
  locators go on working, the `coreId` unchanged; a client that remembered the old key meets the
  changed-key warning (§3.5).
- **After a backup restored without the key** (D290 §11, U6): a core that remembers a `coreId`
  (`discovery.json`) but whose key no longer opens a session says so on System → Discovery: the
  server's identity key changed, and re-connecting keeps the household's links working, with one
  button for the re-key, *Connect* hidden. *Connect* would enrol a new core and strand every link.
- **Two cores with one key** (a backup with the key restored onto a second machine while the first
  runs) are detected by `instanceId` and refused (§2.3, D290 §18, C4).
- **After a lapse** the `coreId` is retired for every other key but not for its own: the same key
  reclaims it on resubscribing, and the household keeps its links (D290 §11, U6).
- **Disabling** (paid, abuse): names stop answering, signalling and enrolment refused, the
  connection closed.

### 2.9 STUN

The discovery runs a STUN server (UDP 3478), given to clients and cores as their ICE server through
`GET /status`'s `iceServers` (built on the core, D274). It carries nothing else. The NAT measurement
of §2.6 needs a second STUN server on another address (the probe machine's). Paid: on addresses of
their own, never the API's, which sits behind Cloudflare and whose address must not be given away
[I, availability research; D290 §8].

## 3. Client ↔ discovery

**The page is the discovery's; the traffic is the core's.** The page asks how to reach its core,
races the paths, and from then on talks only to the core.

### 3.1 How a device finds its core

A device learns its core once and remembers it:

| Entry | Flow |
| --- | --- |
| Registration link (D218 §1) | `https://<discovery>/register#<coreId>.<secret>`, single use |
| Watch Together invite (D243 §3) | `https://<discovery>/join#<coreId>.<secret>`, reusable while live (below) |
| **A person's own link** (D290 §10) | `https://<discovery>/s#<coreId>.<secret>`, reusable; locates only |
| Device code (D288) | the device asks the discovery for a code and shows it; a person signed in enters it (below) |
| Pairing QR (D214 §1) | `https://<discovery>/p#<coreId>.<secret>.<fingerprint>`, for native apps and a household's first device |
| The customer's own browser (paid) | the core's System page, from which they claimed it (§2.2) |

- **The discovery never learns a core's secrets.** The core registers, for each live secret,
  `H = SHA-256("orpheum-locate" ‖ secret)` with its life as a duration and a **use policy**: a
  number of uses, or as many as wanted until it expires or is revoked (`PUT /locate-hashes`, the full
  set; not built; C3). Registration links are single use; a Watch Together link and a person's own
  link are not. The page sends `H`, and the secret goes to the core over the direct path. `/locate`
  is spent by a click on the page, never by its loading, so a mail scanner opening the link spends
  nothing [I, S11, U10].
- **The core is the only issuer of locators** (C3, S11). `/locate` returns the paths and a
  **bootstrap token**, short-lived and single-use, which serves `/paths` and offers for its few
  minutes. The device registers or signs in over the direct path; the core then hands it a **device
  locator**, a random token, registers its hash and adds it to its full set (`PUT /locators`), which
  is then the whole truth. A locator belongs to a grant on the core (a device's session, or a Watch
  Together membership): when the core ends the grant, it drops the locator too (S11). A device that
  reached its core another way gets its locator the same way, and the core hands a fresh one on
  each visit [I, availability research].
- **A person's own link** (D290 §10, U2) leads a device back to its core and grants nothing: the
  person signs in after it. It is on their account page and on the screen after registering, as a
  link and a QR, can be bookmarked or mailed to oneself, and is revoked by the admin. It names one
  core to someone holding its secret, so it is not a list of cores (D214 §3) [I, U2].
- **A Watch Together link finds its core while it lives** (D290 §17, C3): locating grants nothing, so
  it works again and again as long as the link is live on the core. Admission stays the core's: an
  unused link admits its one person, a used one only the device holding the membership credential
  its first use gave (D267 §2). A guest who closed the page by mistake gets back in.
- **A device that knows no core** (D290 §10, U2) gets a page of its own rather than a name and
  password form it cannot use: open the link you were sent; sign in using a phone where you are
  already signed in (the device code); ask the person who runs your server for your link.
- **Staying remembered** (D290 §10): iPhones are offered *Add to Home Screen*, since Safari erases a
  site's storage after seven days unvisited and a home-screen app keeps its own count **[V, U2]**.
  Where the browser allows, the page asks for persistent storage [I, U2].
- **A new device with no link** cannot search for its core by name or e-mail: that would be a list
  of cores (D214 §3). It uses its person's own link, is sent a link, or is activated by its code
  (D288).

**Device codes (D288 §5).** A device not signed in opens its discovery's address (an app opens the
paid service's by default) and asks for a code.

- **Own:** the one core issues it, as built (`internal/core/auth`'s pairing). Past a hundred waiting,
  or five not yet entered from one network (an IPv4 address or an IPv6 /64), a new code is refused
  with 429 and none waiting is dropped (D288 §6 as amended, D290 §15; built).
- **Paid:** a page that names no core, so the **service issues the code**; the core of the signed-in
  device that enters it claims it from the service. The service rate-limits asking for codes, by
  source network (IPv4 address or IPv6 /64) and fairly across sources rather than in one global
  pool, since it alone faces strangers for many cores (S13). Not built.
  - **The session is sealed to the device** (D290 §5, S3): asking for a code, the device makes an
    ephemeral X25519 key pair and sends the public half; the service hands it to the core with the
    claim; the core seals the check code's outcome and the session to it with HPKE (RFC 9180), and
    the service passes on only the sealed blob. The service never holds a session it could use. A
    `TestArch_` claim that no session token crosses `discoveryproto` enforces it [I: a browser has
    no HPKE in WebCrypto; it is built from its X25519, HKDF and AES-GCM, or a library, which is the
    owner's call under rule 3].
  - **The person entering it sees where the device asking is** (D290 §5, S4): its approximate place
    and network, from the address the service saw, and whether it is on the same network as the
    device entering the code, said in plain words when it is not ("This device is in another
    country").
  - **A device activated by a code may start with a narrower role** (D290 §5): ordinary rights, never admin (D291 §5).

### 3.2 The paths

`POST /client/v1/paths` with a locator (or a bootstrap token) returns:

```json
{"coreId": "k7x…q2", "apiMajor": 1,
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

**The `lan` path is returned only when the device seems to be at home** (D290 §2, U1, S11): the
request's source address equals the core's public IPv4, or shares its IPv6 /56 or /64, or the device
remembers the LAN as the path that won on this network (C5). Chrome asks permission for any request
from a public page to a name that resolves to a private address **[V, U1]**, so a LAN name raced on
every first visit would put the prompt in front of every guest, and a "Block" sticks for the whole
`app.` origin, every household's.

| Path | Works when | Notes |
| --- | --- | --- |
| IPv6 | both ends have IPv6 and the router's IPv6 firewall lets the port in | a public address meets no Local Network Access prompt and no rebinding block |
| LAN name | the client is on the core's LAN | many routers drop public answers that point at private addresses (rebinding protection) **[V]**; Chrome asks permission first (§3.4) |
| Public IPv4 | a public IPv4 and a mapping | impossible behind carrier NAT or DS-Lite's shared IPv4; at home it needs hairpinning; a port other than 443 fails on networks that allow only 80 and 443 |
| WebRTC | the NATs on the two ends let a punch through: IPv6 on both ends, or one end endpoint-independent (§2.6) | the core's API over data channels; slower to start (0.45 s at 50 ms round trip) |

**How often WebRTC fails is not known for browsers** (C10). The research's "about 70%" was a libp2p
measurement, conditional on relay reservation and address discovery having succeeded, among libp2p
nodes, not browsers **[V, C10]**; WebRTC field data had 22% of calls needing a relay, and about 40%
of mobile carriers' NATs are symmetric **[V, C10]**. A phone on mobile data may fail to reach a
WebRTC-only household well over 30% of the time **[I, C10]**. The beta measures it (D274 §3, §3.6).

### 3.3 Choosing among the paths (D273 §7 as amended; D290 §2)

The owner rejected moving to "a better one that answers shortly after": *"Simply moving to the next
better one that connects is moving the original problem one rung down rather than solving it. We
should be taking the first one that succeeds as a control connection, then giving a few seconds for
the other attempts to work or fail, and then out of all that succeeded, pick which one will work
best."* Then, on the audit (C5), how "best" is judged (D290 §2). So:

1. The client tries the paths it was given at once: the LAN name only when §3.2 returned it, and
   WebRTC with one connection (§2.5). Where Chrome supports it, a LAN request is marked
   `targetAddressSpace: "local"` (C5).
2. **The first that answers becomes the control connection**: the page can sign in and show the
   household over it at once, so nobody waits on the race. Over it the page reads the core's API
   major, and swaps to the client build for that major if it holds another (C16, §5).
3. **The others get a few seconds [I: about 3 s, to measure] to succeed or fail.**
4. **Of the paths that succeeded, the client ranks by kind**: the LAN first, then IPv6 or IPv4,
   then WebRTC, round-trip time breaking ties (D290 §2). The round trip is the median of a few small
   requests over the open connection, not one sample (C5). **Throughput is not measured in the
   race**: a few seconds measure a fresh connection's congestion window, not the path (D290 §2, C5).
5. **The client moves its traffic there between requests**, never mid-sign-in or mid-chunk; fetches
   under way finish on their path, and the Watch Together socket stays where it is until its path
   fails, then reconnects with its session's resume (C5).
6. **It moves again only when its path fails** (a request error, or two timeouts in a row) **or the
   network changes** (D290 §2): it uses the next path still known to work and races again in the
   background. It races again on the browser's `online` and network-change events and when the page
   becomes visible after a long hide (C5).
7. **It remembers which paths worked, per network** (keyed on the client address prefix the
   discovery reports), not globally (C5), so a later visit starts with them and survives the
   discovery being down.

`webrtc-transport.md` W2c and `SwitchableFetch`'s comment still state the replaced rule (C5); they
follow this section.

### 3.4 What serving the client cross-origin asks of the core (research A2)

The page's origin is the discovery's; the core's are its names. So:

- **CORS** for the configured client origins (a list, never a wildcard), no credentials mode.
- **Sign-in by header**, never a cookie, which would be a third-party cookie (the API already takes
  `Authorization: Bearer`).
- **Byte paths by the header on every request** (hls.js's loader, `fetch` and blob URLs for images
  and subtitles), no credential in a URL (D289 §3).
- **Local Network Access**: Chrome asks before a public page reaches a private address (Chrome 142
  for fetch, 147 for WebSocket, WebRTC planned; Firefox 153 **[V]**). Racing IPv6 and WebRTC first
  does not avoid the prompt, since the LAN request starts at once (U1, C5); returning the LAN path
  only to a device at home does (§3.2). Requests from a service worker need the permission granted
  to the worker's origin first **[V, C5]**.
- **DNS rebinding protection** on many routers drops the LAN name's answer; the device then uses
  another path.
- **Version skew**: one client build per supported core API major (D289 §4), the page swapping
  builds when the core's major differs (§3.3); a service worker carries a remote kill switch, an
  update check that can unregister it (C16).
- **The client's integrity** (D290 §4, S2). The paid web client is built reproducibly and the hash of
  every published build is listed publicly. It is served with Subresource Integrity on every script,
  `Integrity-Policy` requiring it, and a strict content-security policy (`script-src 'self'`, no
  inline script). Sessions made through it keep their admin rights for now (D290 §4).

### 3.5 Watch Together, voice and native clients

Invites go through the discovery (§3.1). Voice is WebRTC between the people, its fallback relay the
core's own TURN or the owner's VPS, never the paid service's.

**The web client remembers a core's identity key on first contact** (D290 §3, S1): over a direct
path, it asks the core to sign a nonce with its identity key (§2.1), keeps the public key per
`coreId`, and checks it on every later visit. A changed key stops the client before it sends a
password, with a plain warning. A client accepts a key the old key signed (§2.8, D291 §6). It also
checks each WebRTC answer's signed fingerprint against that key (§2.5). Not built.

Native apps (later, D273 §4) race the same paths, pin the identity key on first contact, and add
mDNS on the LAN and raw UDP hole punching.

### 3.6 When nothing reaches the core (D290 §12; U3)

When nothing direct reaches a household (NATs on both ends that map per destination and no IPv6, or
a network that allows only HTTPS to known names), only a relay could, and the paid service runs none
(D273 §2). The message is written for whoever reads it:

- **Anyone but the admin** [I: wording, U3]: "Can't reach *Stenerud home* from this network. Try your
  phone's mobile data, or another Wi-Fi. If it keeps happening, tell *Karl*." A phone's hotspot is
  the likeliest fix in a hotel, since mobile networks mostly give phones IPv6 (D274 §4).
- **The admin**, on the System page, ahead of time from the probe: a public IPv4 from the ISP, or
  their own VPS (an own discovery's VPS can hold the tunnel and TURN).
- **The beta counts how often the message is shown**, by the client's kind of network, as an
  aggregate with no identifiers (D290 §12, D274 §3), so the owner sees the number before the paid
  launch.

## 4. The API

Versioned by major in the path, JSON, additive by default (§5). Errors name their cause; a refusal
the core must act on (enrol again, another instance running) carries a code of its own (C12, C4).

### Core (`<discovery>/core/v1`): identity key → bearer token

| Method, path | Purpose | Built |
| --- | --- | --- |
| `POST /challenge` | public key → nonce, for enrolling or a session; rate-limited | yes |
| `POST /enrol` | public key + signed nonce → `coreId`, claim code or token, expiry; idempotent per key | yes |
| `POST /session` | `coreId` + `instanceId` + signed nonce → token, `expiresIn` (1 h) | yes, without `instanceId`, with `expiresAt` |
| `GET /status` | claimed?, a claim waiting for approval, the holding account (masked), account state, zone, protocol support, `iceServers`, messages for the admin | yes, but for the claim and account |
| claim approval (§2.2) | the core's admin allows or refuses an account's claim | no |
| `PUT /addresses` | candidates, the owner's TURN → published names, probe results, `reachableFrom` | yes |
| `GET /signal` (WebSocket) | offers in, answers (with the signed fingerprint) out; pings | yes, without signatures or pings |
| `PUT /acme-challenge`, `DELETE /acme-challenge/{name}` | set or clear `_acme-challenge.<coreId>.<zone>`; returns once served | yes, the clearing with a body |
| the core's ACME account (§2.7) | the account URI, for CAA and DNS-PERSIST-01 | no |
| re-key (§2.8) | a new key's enrolment, countersigned by the old key | no |
| `PUT /locate-hashes` | the full set of live link-secret hashes, with lives and use policies | no |
| `PUT /locators` | the full set of live device-locator hashes | no |
| paid device codes (§3.1) | the core claims a code the service issued, and passes back the sealed outcome | no |
| `DELETE /core` | leave; closes the connection and ends the tokens | yes, leaving them live |

### Client (`<discovery>/client/v1`): a hash, code or locator the device holds; no account

| Method, path | Purpose |
| --- | --- |
| `POST /locate` | link-secret hash → paths + a bootstrap token; by the hash's use policy |
| `POST /paths` | locator or bootstrap token → current paths, ICE servers, the core's API major |
| `POST /cores/{id}/offer` | locator + SDP offer (up to four) → the core's SDP answers, signed |
| device codes (§3.1) | ask for a code with the device's public key; how its pairing stands; the sealed session once confirmed (as the core's own `/pairings`, D288) |

An own discovery answers the same client API for its one core. None of the client API is built in
`discovery/single`: it passes an offer to its core (`Discovery.Offer`) but serves no client calls.

## 5. Versions, and keeping the sides in step (D273 §6, §7)

For both APIs, core ↔ discovery and client ↔ discovery (native apps, cached clients and own
discoveries upgrade on their own schedule):

- **Additive by default:** new fields optional, unknown ones ignored, a new feature a new call; the
  core and the discovery never upgrade together.
- **One version signal** (C13): the path carries the major (`/core/v1`); the
  `Orpheum-Discovery-Protocol` header carries `major.minor`; the body carries none. A discovery
  accepts any minor of a major it serves and uses the minor only to know which optional features the
  core has; `/signal` checks the header too. **The code must change:** as built the header and the
  body both carry `"1"`, `single` requires the header to equal `"1"` exactly and never checks the
  body, and `/signal` checks nothing.
- **A break is a new major** (`/core/v2`).
- **The window** (D273 §7), stated without ambiguity (C13): a major is served until both two newer
  majors exist and six months have passed since its successor shipped. A core on a deprecated
  version says so on its System page with the end date (from `GET /status`); past the window, the
  refusal names the versions accepted.
- **One source of truth:** the public packages (D275 §1). A change lands there, then in the paid
  service accepting old and new, then in the core.
- **Tests both ways:** the core's client runs against `discovery/single` as its reference (built);
  the paid service runs its handlers against the same; its CI builds the newest and the oldest
  supported core and runs enrol → claim → heartbeat → offer → leave against it.

## 6. Answered by the owner (2026-10-13)

- **The paid service's questions that bind the protocol**, decided in
  [D289](../decisions/working-notes.md#d289--the-paid-discoverys-questions-answered-accounts-for-payers-only-the-identity-key-backed-up-as-the-secret-key-is-byte-paths-by-header-one-client-build-per-core-api-major):
  accounts for payers only, household users signing in to their core (Q5); the identity key backed
  up as `secret.key` is (Q12, §2.1); byte paths by the header (Q17, §3.4); one client build per
  supported core API major (Q18, §3.4, §5). Q19's check code at claiming is replaced by claiming
  from the core (D290 §1, §2.2).
- **The audits' findings**, ruled on in D290: claiming from the core (§1; §2.2); paths ranked by
  kind, the LAN name only at home (§2; §3.2, §3.3); re-keying signed by the old key, a slow recovery,
  the key remembered by the client, CT watched (§3; §2.8, §3.5, §2.7); the central client's integrity
  (§4; §3.4); sealed device-code sessions, the device's place shown, a narrower role (§5; §3.1);
  signalling checked by the core, idle connections closed, signed fingerprints (§6; §2.5); computed
  and available DNS, DNS-PERSIST-01, ARI, CAA (§7; §2.7); hosting (§8, the spec); TCP 443 only for
  port mapping (§9; §2.4); a way back for a device (§10; §3.1); recovery after a restore (§11;
  §2.8); the dead-end message (§12; §3.6); the document in line with the code but for the core's id
  (§13); WebRTC in the beta (§14); the two flaws fixed (§15; built); the customer's sign-in link
  (§16, the spec); Watch Together links (§17; §3.1); one core per account in the beta and two cores
  with one key refused (§18; §2.3, the spec); the remaining correctness findings as the audit
  recommends (§18).

## 7. Decided after the audits (D291)

What the audits recommended that D290 neither decided nor followed from, now decided by
[D291](../decisions/working-notes.md#d291--what-the-discovery-audits-raised-and-d290-left-open-taken-as-the-audits-recommend-the-probe-on-a-machine-of-its-own-dns-on-bunny-ipv6-before-ipv4):
each item is taken as its audit recommends, except where noted. Each names its finding.



1. **S2 items 3, (part of) 2:** WEBCAT enrolment later; Trusted Types; keeping `app.` free of
   third-party scripts and analytics (the CSP implies the first part).
2. **S4 items 2–4:** which narrower role a code-activated device gets (S4: playback and its own
   settings, never admin, even when an admin entered the code); each core limiting how many codes
   it claims an hour; the device confirming the core's name before it takes its session.
   Where the approximate place comes from (a geolocation database is a dependency, the owner's to
   approve when it is built). **D291 §5:** ordinary rights, never admin, admin only after the
   person signs in with their password on that device.
3. **S5 item 2:** two kinds of WebRTC connection, a signed-in client proving its session in the
   offer, and unproven ones limited to signing in, registering or pairing, with a small quota and a
   short life.
4. **S1 item 2:** a fresh authentication (step-up) on the dashboard before re-key, removal and
   minting a locator for the customer's browser.
5. **S7 items 3–5:** a *This isn't me, release* action on the System page, signed by the core;
   whether a typed claim code stays as a fallback (then with a dashboard warning and a per-account
   limit on attempts).
6. **S10:** the outside probe's limits: global unicast only, the service's own prefixes and
   private, shared and documentation ranges denied; few ports; a cap on `configured` and TURN
   candidates; probes rate-limited per core and per target; detail reported only once the target
   proved it is the core. **D291 §1:** the probe on a machine of its own, holding no secret but a
   token that can only post results; the spare holds the database.
7. **S12 items 2, 4, 5:** a zone under an HSTS-preloaded TLD (`.app`, `.dev`); registry lock,
   hardware-key 2FA, DNSSEC and a narrow API token. **D291 §4:** no PSL entry now.
8. **S13 item 1:** a cap on published names per core (moot for DNS records once answers are
   computed, not for probes).
9. **S14 items 1–4:** not a random high external port: the router is asked for 443 first and the
   port it grants is taken when 443 is held, since networks that allow only 80 and 443 reach no
   other (D291 §7);
   "your server is reachable from the internet at …" shown prominently; security-update notices
   through `GET /status`; sign-in limits tested over WebRTC.
10. **S15:** a `Host` allow list on the core's plain-HTTP LAN listener against DNS rebinding.
11. **S16:** at most about ten candidates per offer; loopback and link-local remote candidates
    dropped from offers through the discovery; STUN answering Binding requests only, rate-limited
    per source, without `SOFTWARE`.
12. **S17:** logging device requests without the core id or with a truncated address; saying on the
    privacy page that names carry addresses.
13. **S18:** checking the TXT value is a 43-character base64url digest and clearing it after an hour;
    CORS sending `Vary: Origin` and never allowing `null`.
14. **U1:** a line explaining Chrome's prompt before the LAN name is tried; a help page for undoing a
    block; preferring WebRTC's host candidates for the LAN.
15. **U2 (rest):** "Open in Safari" when an in-app browser is detected; an admin's link for a known
    person signing them in rather than resetting the password.
16. **U3 (rest):** the owner's own TURN as one switch; the System page saying how many clients
    failed to connect last week.
17. **U5:** plain names (*Remote access*) and the discovery's address hidden behind "use my own";
    the first-run setup code accepted in the browser.
18. **U7:** lapse and outage messages with dates and *Renew*; household members told the cause; the
    last good LAN address kept for an outage.
19. **U8:** a list of locators with a switcher for a person on two servers. **After the beta**
    (D291 §6).
20. **U9:** a QR beside the TV's code; a default device name; picking the check code from three;
    renaming *Get a code*; a two-sentence warning.
21. **U11, U12:** "Connecting to …" after a second; a "how this device is connected" line; mail to
    the customer when `reachableFrom` drops.
22. **Correctness, "something simpler" (not findings):** the heartbeat carried on the signalling
    connection; the hourly probe dropped for probe-on-change and clients' reports.
23. **S1 item 3, after a re-key:** a client that remembered the old key accepts the new one when the
    old key signed it; after a key recovered without it, a plain notice says the server's identity
    changed and when the owner confirmed it (D291 §6).
24. **C5 item 1 (part):** a path whose round trip is far worse (more than 3× and 50 ms over the best)
    is demoted, and IPv6 ranks before IPv4 (D291 §3).
