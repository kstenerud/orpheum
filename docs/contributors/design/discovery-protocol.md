> **ABOUTME:** The discovery protocol: how a core, its clients and a discovery speak, the same for
> the paid service at app.orpheum-app.com (many cores) and an owner's own `orpheum discovery` (one
> core). Moved here 2026-10-13 from the paid service's spec, which keeps billing, accounts, abuse and
> its architecture.

# The discovery protocol

**Status:** a design, partly built. Moved into the repository on the owner's word (2026-10-13:
*"move the protocol parts into the repo"*) from the paid service's specification (draft 3, the
shared files' `discovery-service-spec.md`), whose §3, §4, §8 and §9.3 it was. It is what the core
(`internal/core/discovery`), Orpheum's public building blocks (`discovery/discoveryproto`,
`discovery/proof`, `discovery/safe32code`, `discovery/single`) and the closed service all speak
(D275 §1). The plans that build it are [discovery-core.md](plans/discovery-core.md) (the core's
side, built), [discovery-mode.md](plans/discovery-mode.md) (W4b, the owner's own discovery) and
[webrtc-transport.md](plans/webrtc-transport.md) (the client's paths).

**Markers.** `D<n>` is a decision; a claim citing one comes from it. **[I]** is an inference or
recommendation not taken from a decision. **[V]** is a fact checked at its source in
[reachability-approaches-2026-10-09.md](research/reachability-approaches-2026-10-09.md) ("the
research"). **Built** says what exists today. **Owner** marks a question still the owner's.

**Words.** A **discovery** is either the paid service or an owner's own. Where the two differ the
text says **paid** or **own**. A **core** is a household's Orpheum server; a **client** is a browser
or app; a **device** is a client that has found its core.

## 1. What a discovery does, and does not

It connects a household's clients to that household's core and carries none of their traffic
(D273 §2, D214 §1):

1. **Serves the web client** (paid: at `app.orpheum-app.com`; own: at the owner's domain, D274 §5).
   After that, every API call, chunk, image and socket goes from the browser to the core.
2. **Names and certificates** (paid only): hidden per-core names under a dedicated zone; the core
   runs ACME and keeps its key, the discovery only sets the DNS-01 TXT record (§2.7). An own
   discovery uses the owner's domain.
3. **Where the core is now:** the core reports candidate addresses; the discovery tests them from
   outside and hands the working ones to the core's devices (§2.4, §2.6).
4. **WebRTC signalling and STUN** for a core no HTTPS path reaches: offers and answers over one
   connection the core keeps open, a few kilobytes each, never media (D273 §7, §8; §2.5, §2.9).
5. **Finding the core:** links, QR codes and device codes lead a device to its core once (§3.1).

It does not relay anything (paid: no TURN, ever, D273 §2), hold household data, sign household users
in (they sign in to the core, through the client), or list cores (D214 §3, D187 §2). Once a client
has reached its core, the discovery can go down and playing goes on.

## 2. Core ↔ discovery

All calls are HTTPS to `<discovery>/core/v1/…`, JSON, outbound only; the discovery connects in only
to probe (§2.6). **Built** (`discovery/discoveryproto`, `internal/core/discovery`): JSON keys are
camelCase; every call carries `Orpheum-Discovery-Protocol: 1`; an answer's
`Orpheum-Discovery-Deprecated: <date>` header is shown to the core's admin.

### 2.1 The core's identity key (D214 §4) — built

`internal/core/identity`: an Ed25519 key pair made on first start, kept as `identity.key` beside
`secret.key` (D194), mode 0600, never in the database. It never changes for the instance's life;
restoring a backup without it means re-keying (§2.8). It signs enrolment and session challenges and
nothing else; it is not the TLS key. Its fingerprint, `SHA-256(public key)` in safe32 groups, belongs
on the System page (not built). Whether scheduled backups carry it: answered (§6, Q12), as
`secret.key` is treated.

### 2.2 Enrolment and claiming — built

1. The admin opens **System → Discovery** and chooses *Connect*.
2. `POST /challenge {publicKey}` → `{nonce}`; `POST /enrol {publicKey, protocol, nonce, signature}`
   → `{coreId, claimCode, claimExpiresAt}` (Unix ms). The signature is Ed25519 over
   `orpheum-discovery-v1:enrol:<nonce>` (`discovery/proof`); a session's is over
   `…:session:<nonce>`, so neither stands in for the other. A nonce works once.
3. The discovery makes a **pending core** with a random `coreId` and a **claim code**: 8 characters
   of safe32 shown `xxxx-xxxx` (`discovery/safe32code`), 30 minutes, once. The core shows it, and a
   QR of the claim link (not built).
4. **Paid:** the customer, signed in to the dashboard, types the code; the core is bound to their
   account if it may hold another. **Own:** the owner runs `orpheum discovery claim <code>` on the
   VPS (W4b.2).
5. The core, polling `GET /status`, sees *claimed* and goes on to §2.3–§2.7.

A pending core not claimed in 30 minutes is deleted; enrolment is rate-limited per address (paid).

**Owner (2026-10-13): a check code at claiming?** The owner asked whether claiming should end, as
device activation does (D288 §3), with a check code both screens show. **[I]** Yes, for the paid
dashboard: after the code is typed, the dashboard and the core's System page both show a check code
(and the core's name and fingerprint), and only the customer's "they match" binds the core. It
closes the case of a customer typing a code someone else read out to them, which would hand that
person's core the customer's account and names, or theirs the customer's household. On an own
discovery the owner types the code on the VPS they control, and the check adds little. To decide.

### 2.3 Session — built

`POST /challenge` → `{nonce}`; `POST /session {coreId, nonce, signature}` → `{token, expiresAt}`, an
opaque bearer token for one hour, hashed at rest. The core renews five minutes before it expires and
at once on a 401. Every other call sends `Authorization: Bearer …`, never a token in a URL (D154 §2).
No mTLS, which complicates proxies, and no signature on every request.

### 2.4 Heartbeat and addresses — built for interface and mapped candidates

`PUT /addresses` every 5 minutes, and at once when an address or mapping changes (the core polls
its interfaces every minute):

```json
{
  "protocol": "1.0", "orpheumVersion": "0.4.0", "apiMajor": 1,
  "candidates": [
    {"address": "2a0f:1000:…:1", "port": 443,   "source": "interface"},
    {"address": "203.0.113.7",   "port": 51234, "source": "mapped", "method": "pcp"},
    {"address": "198.51.100.9",  "port": 443,   "source": "configured"},
    {"address": "192.168.1.20",  "port": 443,   "source": "interface", "scope": "lan"}
  ],
  "turn": [{"urls": "turn:198.51.100.9:3478"}]
}
```

- `interface`: an address on the machine (a global IPv6 address is native on DS-Lite).
- `mapped`: a public IPv4 and port from the router by UPnP IGD, NAT-PMP or PCP, **on by default**,
  shown on the System page, with a switch (D273 §7; built, `internal/core/portmap`). A router may
  grant another port than asked, so every candidate carries its port.
- `configured`: an address the admin set: a port-forward, or their own VPS.
- `lan` addresses are published unprobed; they matter only at home.
- `turn`: a TURN server the owner runs, passed to the core's clients; the owner's relay, never the
  paid service's.
- The discovery records the request's source address too, but never publishes it unprobed: behind
  carrier NAT it is shared and unreachable.

The discovery publishes only public candidates its probe passed (§2.6), keeps current addresses
only, no history, and marks a core silent for 30 minutes *offline* while its names keep answering.

### 2.5 The signalling connection (D273 §7, §8) — built on the core

One WebSocket the core keeps open, `<discovery>/core/v1/signal`, with the session's token. It
carries only WebRTC offers and answers:

1. A client posts an SDP offer with its locator (§3).
2. The discovery checks the locator and forwards `{"type": "offer", "offerId", "sdp"}` (text
   messages, one JSON object each, at most 64 KiB).
3. The core answers as `internal/core/rtcapi` answers its own `POST /rtc/offer` (candidates
   gathered first, no trickle): `{"type": "answer", "offerId", "sdp"}`, or
   `{"type": "refusal", "offerId", "reason"}`.
4. The discovery returns the answer; the data channels then run browser to core.

A client opens four connections per session (research §9.4); the core bounds its connections (64)
and refuses past that; the discovery rate-limits offers per locator and per core. **A connection
grants nothing**: every request over a channel carries its own bearer token to the core. The core
answers an offer through the discovery without a signed-in check of its own, since the discovery
checked the locator. A dropped connection is reopened with backoff; offers in the gap fail fast and
the client's other paths race on.

### 2.6 The outside probe

After each address change, and hourly, the discovery tests each public candidate from a probe
machine on another network: HTTPS to the address with the candidate's name as SNI, the certificate
checked, an echo of a nonce the core issued (the check of `internal/core/reachability`, from
outside, a failure naming its hop, D152); WebRTC, an offer through the signalling connection from
behind an ordinary NAT; and the owner's TURN servers. The heartbeat's answer carries the result:

```json
{"published": ["2a02-8070-…-1.k7x…q2.<zone>", "203-0-113-7.k7x…q2.<zone>:51234"],
 "probe": [{"candidate": "2a0f:1000:…:1", "result": "reached"},
           {"candidate": "198.51.100.9:443", "result": "refused"},
           {"candidate": "webrtc", "result": "reached"}],
 "reachableFrom": "everywhere"}
```

`reachableFrom` is `everywhere` (a public IPv4 path works), `ipv6OrWebrtc`, or `nowhere`; the core
shows it on the System page, with what a household nothing reaches can do (§5).

### 2.7 Names and certificates (D273 §7, research B1, §9.2) — the core's side built

- **Paid:** names under a dedicated registered domain, never `orpheum-app.com`: every core holds a
  certificate there and could otherwise set cookies for the service's pages. On the Public Suffix
  List it isolates cores from each other too.
- **Names encode addresses** (Plex's shape): `<address>.<coreId>.<zone>`, the IPv4 address dashed
  (`192-168-1-20`) or the IPv6 address as eight dashed groups. `coreId` is 26 safe32 characters
  (130 bits), random, never reused. A port other than 443 goes in the URL.
- **One wildcard certificate per core**, `*.<coreId>.<zone>`: an address change needs no new
  certificate, and Certificate Transparency shows only the random label.
- **DNS-01 by proxy:** the core runs ACME (`internal/core/certificate`) and calls
  `PUT /acme-challenge {name, value}`; the discovery checks the name is
  `_acme-challenge.<coreId>.<zone>` for this core, sets it, and the core asks the CA to validate,
  then `DELETE /acme-challenge`. The core keeps its ACME account, its key and its certificate.
- **Own:** the names are the owner's domain's, set through the owner's DNS provider (W4b, the plan's
  Q2).
- Let's Encrypt's limits and shrinking lifetimes (64 days from February 2027, 45 in 2028 **[V]**)
  are the paid service's to plan for: its spec.

### 2.8 Revocation, re-keying and leaving

- **Removing or leaving** (`DELETE /core`): names and TXT removed, locators dropped, the core marked
  gone; its `coreId` never reused, so an old link never reaches someone else's server.
- **Re-key** (identity key lost or stolen): a claim code from the new key, typed where the core was
  claimed; the old key stops at once; devices' locators go on working, the `coreId` unchanged.
- **Disabling** (paid, abuse): names removed, signalling and enrolment refused.

### 2.9 STUN

The discovery runs a STUN server (UDP 3478) on its own address, given to clients and cores as their
ICE server through `GET /status`'s `iceServers` (built on the core, D274). It carries nothing else.

## 3. Client ↔ discovery

**The page is the discovery's; the traffic is the core's.** The page asks how to reach its core,
races the paths, and from then on talks only to the core.

### 3.1 How a device finds its core

A device learns its core once and remembers it:

| Entry | Flow |
| --- | --- |
| Registration link (D218 §1) | `https://<discovery>/register#<coreId>.<secret>` |
| Watch Together invite (D243 §3) | `https://<discovery>/join#<coreId>.<secret>` |
| Device code (D288) | the device asks the discovery for a code and shows it; a person signed in enters it (below) |
| Pairing QR (D214 §1) | `https://<discovery>/p#<coreId>.<secret>.<fingerprint>`, for native apps and a household's first device |
| The customer's own browser (paid) | the dashboard hands it a locator |

- **The discovery never learns a core's secrets.** The core registers, for each live secret,
  `H = SHA-256("orpheum-locate" ‖ secret)` with its expiry (`PUT /locate-hashes`, not built); the
  page sends `H`, and the secret goes to the core over the direct path.
- **The locator.** On a match the discovery returns the paths (§3.2) and a **device locator**, a
  random token hashed at rest, bound to that core, which the page keeps for every later visit. A
  device that reached its core another way gets one from the core, which registers its hash; when
  the core ends a device's session it drops the locator too (`PUT /locators`, the full set).
- **A new device with no link** cannot search for its core by name or e-mail: that would be a list
  of cores (D214 §3). It is sent a link, or activated by its code (D288).
- **Device codes (D288 §5).** A device not signed in opens its discovery's address (an app opens
  the paid service's by default) and asks for a code. **Own:** the one core issues it, as built
  (`internal/core/auth`'s pairing). **Paid:** a page that names no core, so the **service issues
  the code**; the core of the signed-in device that enters it claims it from the service, and the
  service carries the check code and the session to the device. The service rate-limits asking for
  codes, per address and in all, since it alone faces strangers for many cores; an own discovery
  and its core limit nothing there (the owner, 2026-10-12). Not built on the paid side.

### 3.2 The paths

`POST /client/v1/paths` with a locator returns:

```json
{"coreId": "k7x…q2", "apiMajor": 1, "protocol": "1.0",
 "paths": [
   {"kind": "lan",    "url": "https://192-168-1-20.k7x…q2.<zone>"},
   {"kind": "ipv6",   "url": "https://2a02-8070-…-1.k7x…q2.<zone>"},
   {"kind": "ipv4",   "url": "https://203-0-113-7.k7x…q2.<zone>:51234"},
   {"kind": "webrtc", "offer": "/client/v1/cores/k7x…q2/offer"}],
 "iceServers": [{"urls": "stun:stun.<zone>:3478"},
                {"urls": "turn:198.51.100.9:3478", "owner": true}]}
```

| Path | Works when | Notes |
| --- | --- | --- |
| IPv6 | both ends have IPv6 and the router's IPv6 firewall lets the port in | a public address meets no Local Network Access prompt and no rebinding block |
| LAN name | the client is on the core's LAN | many routers drop public answers that point at private addresses (rebinding protection) **[V]**; Chrome asks permission first (§3.4) |
| Public IPv4 | a public IPv4 and a mapping | impossible behind carrier NAT or DS-Lite's shared IPv4; at home it needs hairpinning |
| WebRTC | hole punching succeeds (about 70% **[V]**) | the core's API over data channels, four connections; slower to start (0.45 s at 50 ms round trip) |

### 3.3 Choosing among the paths (D273 §7, as the owner amended it 2026-10-13)

The owner rejected moving to "a better one that answers shortly after": *"Simply moving to the next
better one that connects is moving the original problem one rung down rather than solving it. We
should be taking the first one that succeeds as a control connection, then giving a few seconds for
the other attempts to work or fail, and then out of all that succeeded, pick which one will work
best."* So:

1. The client tries every path at once.
2. **The first that answers becomes the control connection**: the page can sign in and show the
   household over it at once, so nobody waits on the race.
3. **The others get a few seconds [I: about 3 s, to measure] to succeed or fail.**
4. **Of every path that succeeded, the client picks the one that will work best**, by what it
   measured of each (its round trip, and where it can, its throughput) and by kind: on the LAN over
   across the internet, plain HTTPS over WebRTC. Where kind and measurement disagree, measurement
   decides **[I]**; the weighting is the client's (webrtc-transport W2) to settle on measurements.
5. The client moves its traffic there, and remembers the paths that worked, so a later visit starts
   with them and survives the discovery being down.

### 3.4 What serving the client cross-origin asks of the core (research A2)

The page's origin is the discovery's; the core's are its names. So: **CORS** for the configured
client origins (a list, never a wildcard), no credentials mode; **sign-in by header**, never a
cookie, which would be a third-party cookie (the API already takes `Authorization: Bearer`);
**byte paths by the header on every request** (hls.js's loader, `fetch` and blob URLs for images
and subtitles), no credential in a URL (answered, §6, Q17); **Local Network Access** prompts for a
public page calling a private address (Chrome 142 for fetch, 147 for WebSocket; Firefox 153
**[V]**), which racing IPv6 and WebRTC first mostly avoids; **DNS rebinding protection** on many
routers, covered by the race and a help page; and **version skew**, one client build per supported
core API major (answered, §6, Q18).

### 3.5 Watch Together, voice and native clients

Invites go through the discovery (§3.1). Voice is WebRTC between the people, its fallback relay the
core's own TURN or the owner's VPS, never the paid service's. Native apps (later, D273 §4) race the
same paths, pin the identity key on first contact, and add mDNS on the LAN and raw UDP hole punching.

## 4. The API

Versioned by major in the path, JSON, additive by default (§5). Errors name their cause.

### Core (`<discovery>/core/v1`): identity key → bearer token

| Method, path | Purpose | Built |
| --- | --- | --- |
| `POST /challenge` | public key → nonce, for enrolling or a session; rate-limited | yes |
| `POST /enrol` | public key + signed nonce → `coreId`, claim code, expiry | yes |
| `POST /session` | `coreId` + signed nonce → token (1 h) | yes |
| `GET /status` | claimed?, account state, zone, protocol support, `iceServers`, messages for the admin | yes |
| `PUT /addresses` | candidates, the owner's TURN → published names, probe results, `reachableFrom` | yes |
| `GET /signal` (WebSocket) | offers in, answers out | yes |
| `PUT`, `DELETE /acme-challenge` | set or clear `_acme-challenge.<coreId>.<zone>` | yes |
| `PUT /locate-hashes` | the full set of live link-secret hashes with expiries | no |
| `PUT /locators` | the full set of live device-locator hashes | no |
| paid device codes (§3.1) | the core claims a code the service issued, and passes back the check code and the session | no |
| `DELETE /core` | leave | yes |

### Client (`<discovery>/client/v1`): a hash, code or locator the device holds; no account

| Method, path | Purpose |
| --- | --- |
| `POST /locate` | link-secret hash → paths + a device locator; one use per secret |
| `POST /paths` | locator → current paths, ICE servers, the core's API major |
| `POST /cores/{id}/offer` | locator + SDP offer → the core's SDP answer |
| device codes (§3.1) | ask for a code; how its pairing stands; the session once confirmed (as the core's own `/pairings`, D288) |

An own discovery answers the same client API for its one core.

## 5. Versions, and keeping the sides in step (D273 §6, §7)

For both APIs, core ↔ discovery and client ↔ discovery (native apps, cached clients and own
discoveries upgrade on their own schedule):

- **Additive by default:** new fields optional, unknown ones ignored, a new feature a new call; the
  core and the discovery never upgrade together.
- **A break is a new major** (`/core/v2`), with the full version in every request.
- **The window:** the previous two majors are answered, each for at least six months after it is
  superseded, whichever is longer. A core on a deprecated version says so on its System page with
  the end date (from `GET /status`); past the window, the refusal names the versions accepted.
- **One source of truth:** the public packages (D275 §1). A change lands there, then in the paid
  service accepting old and new, then in the core.
- **Tests both ways:** the core's client runs against `discovery/single` as its reference (built);
  the paid service runs its handlers against the same; its CI builds the newest and the oldest
  supported core and runs enrol → claim → heartbeat → offer → leave against it.

When nothing direct reaches a household (symmetric NAT on both ends without IPv6, or a network that
allows only HTTPS to known names), only a relay could, and the paid service runs none: the page and
the System page say so plainly ahead of time, and point to a public IPv4 from the ISP or the owner's
own VPS (an own discovery's VPS can hold the tunnel and TURN).

## 6. Answered by the owner (2026-10-13)

Questions of the paid service's spec that bear on the protocol, answered *"Follow your
recommendation"*:

- **Q12:** the identity key is treated as `secret.key` is (D243 §11): out of scheduled backups, in
  *Back up now* when its box is ticked; losing it costs a re-key (§2.8).
- **Q17:** byte paths take the header on every request from a cross-site page; no credential in a
  URL (§3.4).
- **Q11, Q16, Q8** are the paid service's (its customers with their own VPS, the dashboard's own
  origin, what happens when payment stops); its spec records them.
- **Q18** (one client build per supported core API major) is explained to the owner and open.
