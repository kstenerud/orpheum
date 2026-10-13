> **ABOUTME:** A correctness audit (sound design, best practice) of the discovery protocol and the
> paid discovery service's spec, before development, checked against RFCs, Let's Encrypt's and the
> Public Suffix List's published rules, browser behaviour and NAT-traversal measurements, and
> against the code as built on 2026-10-13.

# Discovery audit: correctness (2026-10-13)

**Audited:** [discovery-protocol.md](../discovery-protocol.md) and the paid service's spec
(`/yoloai/files/discovery-service-spec.md`, draft 4), with D273 §7 (amended 2026-10-13), D274,
D275 and D288, the research
[reachability-approaches-2026-10-09.md](reachability-approaches-2026-10-09.md), and the code in
`internal/core/discovery`, `internal/core/portmap`, `internal/core/certificate`,
`internal/core/rtcapi`, `discovery/` and `web/packages/sdk/src/rtc-transport.ts`. Security and
ease of use are other audits'; they appear here only where a correctness defect causes them.

**Markers.** **[V]** verified at the source named (a standard, a vendor's page, or the code at
file:line). **[I]** inferred.

## Summary

The shape is sound and follows proven practice: outbound-only cores, a core-held key and
certificate, Plex's address-encoded names under one wildcard per core, a discovery that carries no
traffic, and a race of several paths. The defects are in the edges the document leaves unwritten.

The ones that matter most:

1. **The certificate plan does not survive growth or Let's Encrypt's 2027–2028 changes as
   written** (C1). 50 new certificates per registered domain per week caps enrolments at about 50
   cores a week. The spec's main remedy, a Public Suffix List entry, is one the PSL says it refuses
   ("entries whose sole purpose is to circumvent rate limits"). Authorisation reuse falls to 7
   hours in 2028, so every renewal needs the discovery live. Let's Encrypt's new DNS-PERSIST-01
   fits this design almost exactly and should be adopted.
2. **Beta DNS on Cloudflare's free plan holds 200 records per zone** (C2): about 50 cores.
3. **The device-locator lifecycle contradicts itself** (C3): the discovery issues locators the
   core does not know, and the core's "full set" upload would delete them. And **`/locate`'s "one
   use per secret" breaks Watch Together join links**, which are shared and multi-use (D243).
4. **Two cores with the same identity key** (a restored backup) are neither detected nor handled
   (C4): they alternate on the signalling socket every 30 s and overwrite each other's addresses.
5. **The path choice measures what a few seconds cannot measure** (C5): throughput over a data
   channel is still in slow start after 3 s. Rank by kind, tiebreak by round trip, add hysteresis
   and a re-race trigger, and make the LAN attempt not trigger Chrome's Local Network Access prompt
   when a public path already answered.
6. **WebRTC set-up has no failure recovery** (C6): no trickle and up to 10 s gathering on the
   client, all four connections must connect or none is used, a dead connection attracts every
   request in the client's least-loaded choice, no ICE restart, and the core's 64-connection cap
   means 16 WebRTC clients.
7. **IPv6 candidates include temporary (privacy) addresses, and CGNAT or double-NAT external
   addresses are published as public** (C7, C9).
8. **Signatures are not bound to the discovery they are made for** (C8): one discovery can relay
   another's challenge to a core it serves.
9. **The "about 70%" punching figure is conditional and not from browsers** (C10): the study's
   later version says 70% *given* relay reservation and address discovery succeeded, among libp2p
   nodes. WebRTC field data had 22% of calls needing TURN, and mobile carrier NATs are about 40%
   symmetric. The probe as designed cannot tell a core which side of that it is on.
10. **The code disagrees with the document on several wire details** (§ Mismatches): `urls` is an
   array in code and a string in the document, `"1"` versus `"1.0"`, `ipv6_or_webrtc` versus
   `ipv6OrWebrtc`, and a coreId of 40 bits in `discovery/single` against 130 in the document.

## Findings

### C1 — Certificates: issuance limits, the PSL route, and the 2028 renewal path (high)

**Sections:** protocol §2.7; spec §3.11, §7.6, §10 "After the beta" item 6.

**The design says.** One wildcard per core, `*.<coreId>.<zone>`, by DNS-01 through the discovery;
"before growth, the zone goes on the PSL or the service asks for a higher limit". Renewal starts at
two-thirds of the lifetime, and the TXT proxy "must be up more often".

**Defect.**

- *Enrolment cap.* Let's Encrypt allows "up to 50 certificates … per registered domain every 7
  days", refilling one every 202 minutes, a global limit across all accounts **[V, LE rate
  limits]**. Every new core is a new certificate under the zone's registered domain. So the service
  enrols at most about 50 cores a week; a launch or a press mention stalls at the 51st, with the
  core retrying under backoff. Renewals with the same identifier set are exempt from this limit,
  and ARI-coordinated renewals are "exempt from all rate limits" **[V]**, so the limit bites on new
  cores, not on the fleet.
- *The PSL route is likely closed.* The PSL's guidelines say: "We do not accept entries whose sole
  purpose is to circumvent rate limits of third parties (such as Let's Encrypt rate limits - use
  their form instead)"; its contributing notes list "Smaller, private projects with <2000
  stakeholders" among common rejections; and they require at least two years left on the
  registration, a `_psl` TXT record kept in place, and they promise no timeline **[V, PSL
  guidelines]**. The cookie-isolation reason (spec §3.11) is a legitimate one, but the cores use
  header sign-in and no cookies cross-site (§3.4), so it is weak for this zone. **[I]** Plan on Let's
  Encrypt's override form, which "takes several weeks" **[V]**, filed before the beta ends.
- *2027–2028.* Let's Encrypt's default profile goes to 64-day certificates with a 10-day
  authorisation reuse on 10 February 2027, and to 45 days with **7-hour** reuse on 16 February 2028
  **[V, LE "From 90 to 45"]**. From then on every renewal is a fresh DNS-01 validation, so the
  discovery's TXT proxy is on the path of every renewal of every core. The spec's §7.6 ("weeks")
  holds, but the margin is about 15 days at 45-day lifetimes.
- *Validation from several places.* Let's Encrypt validates from a primary and several remote
  perspectives and needs a quorum **[V, LE community]**. The core waits a fixed 30 s after the TXT
  is set and does not check it resolves (`certificate.go:282, 295–307`) **[V]**. With the
  discovery's own authoritative DNS (spec §7.2) on two nodes, a record set on one but not yet on
  the other fails validation intermittently **[I]**.
- *Renewal timing in code.* `renewBefore = 30 days` is fixed (`certificate.go:48`), with no ARI
  (RFC 9773) **[V]**. It works for 64- and 45-day certificates; it would reissue on every sweep for
  any certificate shorter than 30 days, and it forgoes ARI's exemption and Let's Encrypt's
  recommended practice **[V, LE recommends ARI]**.

**Practice.** Let's Encrypt announced **DNS-PERSIST-01** (CA/B Forum ballot SC-088v3, passed
October 2025; IETF ACME draft): one standing TXT record at `_validation-persist.<name>` naming the
CA and the ACME account URI, with `policy=wildcard` covering `*.<name>` and an optional
`persistUntil` expiry; "this removes DNS changes from the critical path". Staging was planned for
late Q1 2026 and production for Q2 2026 **[V, LE blog 2026-02-18]**; whether it is in production
today is unconfirmed **[I, verify]**.

**Recommendation.**

1. Adopt DNS-PERSIST-01 as the primary method once Let's Encrypt has it in production: at claim,
   the discovery writes `_validation-persist.<coreId>.<zone>` with the core's account URI and
   `policy=wildcard`. Renewal then needs nothing from the discovery. Lapse maps onto it cleanly:
   `frozen` sets `persistUntil` to the end of the frozen period, `released` deletes the record.
   Keep the DNS-01 proxy as the fallback.
2. Implement ARI in `internal/core/certificate` and renew in its window; fall back to two-thirds of
   the lifetime (what Let's Encrypt says is acceptable), not a fixed 30 days.
3. Have `PUT /acme-challenge` return only once the record is served by every authoritative server,
   and have the core check it resolves before asking the CA.
4. File Let's Encrypt's rate-limit override for the zone before the beta ends. Treat a PSL entry as
   unlikely and not a plan.
5. Set a CAA record on the zone limiting issuance to Let's Encrypt, with
   `validationmethods=dns-01` (RFC 8657), so a misissued HTTP-01 for a core's name is refused **[I]**.

### C2 — Beta DNS: Cloudflare's free plan holds 200 records (high for the beta)

**Sections:** spec §7.2 ("Check the free plan's record limits [verify]").

**Defect.** Cloudflare: free zones created on or after 1 September 2024 hold 200 records; Pro and
Business 3,500 **[V, Cloudflare docs]**. The beta's design creates an explicit record per published
address name, plus ACME TXT records. A core with a LAN, an IPv6 and an IPv4 name uses three, plus a
TXT during renewal: the zone is full at about 50–60 cores, and a new zone at 3,500 holds about a
thousand. Every IPv6 change adds a record before the old one is removed.

**Recommendation.** Use a Pro zone for the beta, or (better, and simpler at scale) run the computed
authoritative DNS of spec §7.2 from the start: a name's answer is its own label, as plex.direct does
("any IP.*.plex.direct domain resolves to IP" **[V, Valsorda]**), with no records at all. **[I]**
Answer only for coreIds that exist (a set in memory), so the zone is not a general-purpose
rebinding resolver for anyone's addresses, which also makes routers' and filters' treatment of it
less hostile.

### C3 — Locators and link secrets: who owns the set, and multi-use links (high)

**Sections:** protocol §3.1, §4 (`POST /locate`, `PUT /locate-hashes`, `PUT /locators`).

**The design says.** On a matching link-secret hash, the discovery returns paths and "a device
locator, a random token hashed at rest". A device that reached its core another way gets a locator
from the core, which registers its hash; "when the core ends a device's session it drops the
locator too (`PUT /locators`, the full set)". `/locate` is "one use per secret".

**Defect.**

- *Two issuers, one full-set owner.* Locators the discovery issues on `/locate` are not known to
  the core. The next `PUT /locators` (the full set) either deletes them, so every device that came
  by a link loses its way back, or the discovery keeps a second, core-invisible set, which the core
  can then never revoke when it ends that device's session. Either way the stated invariant ("when
  the core ends a device's session it drops the locator too") fails. **[I, from the text]**
- *Multi-use links.* A Watch Together join link is shared and used by several people; ejecting
  someone raises "would you also like to rotate the join link so they can't come back?" (D243, the
  decision's text at working-notes line ~7596) **[V]**. "One use per secret" makes the second guest
  fail to locate the core.
- *Expiry by two clocks.* The core sends absolute expiries for hashes, which the discovery enforces
  by its clock **[I]**.

**Recommendation.** Make the core the only issuer of locators. `/locate` returns the paths and a
short-lived, single-use *bootstrap* token only; the device signs in or registers over the direct
path, and the core then hands it a locator and adds it to its full set. The `PUT /locators` full set
is then the whole truth, and full-set replacement stays idempotent. Give each locate-hash its own
use policy from the core (`maxUses` or "until expiry"), so registration links stay single-use and
join links do not. Send lifetimes as durations (`expiresIn`), not instants.

### C4 — Two cores with one identity, and re-key (high)

**Sections:** protocol §2.1, §2.5, §2.8; spec §6 Q12.

**The design says.** The key "never changes for the instance's life"; restoring a backup without
it means re-keying; re-keying makes "the old key stop at once".

**Defect.**

- *Restored backups with the key.* Q12 puts the key in *Back up now* when its box is ticked. Restore
  that on new hardware while the old machine still runs (a migration, a test restore, a NAS swap)
  and two cores share one `coreId`. In `discovery/single` the newest signalling socket wins and the
  old is closed (`single/signal.go:624–630`); the loser redials 30 s later, so the two alternate
  every 30 s, offers land on whichever holds the socket, and addresses are last-writer-wins
  (`single.go:327`) **[V]**. Clients flip between two servers with different databases. Nothing
  detects or reports it.
- *Re-key is unspecified.* How a new key's claim code is bound to an existing `coreId` is not
  written; `single` refuses a new key while claimed (`single.go:206–207`) **[V]**, and nothing
  builds re-key. "The old key stops at once" says nothing of the old key's open signalling socket
  and live tokens, nor of the **TLS key**: if the identity key was stolen, the wildcard's private
  key was most likely taken with it, and it stays valid for its lifetime.
- *Leave leaves a live socket.* After `DELETE /core`, the core's signalling loop keeps its socket
  until it drops (`internal/core/discovery/signal.go:615–664`) and `single` closes neither socket
  nor tokens (`single.go:371–381`) **[V]**.

**Practice.** A key that identifies a machine cannot by itself tell two copies of that machine
apart; a second, per-run identifier that is never backed up can **[I]**.

**Recommendation.**

1. Each start of a core makes a random **instance id** (in memory, not backed up), sent on
   `/session` and on the socket. The discovery holds one live instance per `coreId`: a second one
   is refused with a named error ("another server is running with this identity"), which both
   System pages show, and the admin chooses which to keep (the other re-keys).
2. Specify re-key: the dashboard (paid) or `orpheum discovery rekey` (own) binds a new key's
   enrolment to the existing `coreId`; the discovery then revokes every token of the old key and
   closes its socket; the core with the new key gets a **new certificate** and revokes the old one
   (RFC 8555 §7.6 allows revocation by an account holding authorisations for the names) **[I]**.
3. Revocation, re-key, disabling and leave each close the signalling socket and invalidate tokens at
   once; state this in §2.8 and test it.

### C5 — Choosing a path: what can be measured, switching, hysteresis (high)

**Sections:** protocol §3.3 (D273 §7 amended), §3.4; `webrtc-transport.md` W2c.

**The design says.** The first path to answer is the control connection; the others get "about 3
s"; the client then picks the best by "its round trip, and where it can, its throughput" and by
kind; "where kind and measurement disagree, measurement decides".

**Defect.**

- *Throughput is not measurable in 3 s on these paths.* A fresh TCP or SCTP connection is in slow
  start for its first round trips; the research's own measurement shows a data channel's throughput
  set by its congestion window and round trip, not the link (§9.4: 15–20 Mbit/s at 50 ms on one
  connection, 58 on four) **[V, research]**. A short transfer measures the window, not the path, and
  costs the household's upload bandwidth on every page load for every path raced. Measurement
  "deciding" against kind will pick WebRTC over IPv6 when one sample was lucky.
- *Round trip alone is noisy* in the first second (TLS handshakes, DNS, ICE checks); a single sample
  per path can invert the order.
- *No hysteresis or re-race rule.* Nothing says what happens after the choice: when the chosen path
  fails mid-film (a phone leaves Wi-Fi, an IPv6 prefix changes), when a better path appears later,
  or how often the client may switch.
- *Switching sockets.* `SwitchableFetch` moves new requests and lets in-flight ones finish on their
  path (`rtc-transport.ts:447–464`) **[V]**, which is right for fetches. The Watch Together socket
  and long-lived streams are not fetches; moving them mid-session drops session state unless
  reconnection is designed.
- *The race itself triggers the Local Network Access prompt.* Chrome 142 asks permission for any
  request from a public page to a destination that *resolves* to a private address; WebSocket
  followed in 147 and WebRTC is planned **[V, Chrome LNA]**. Racing the LAN name in parallel means
  every Chrome user whose router resolves it sees a permission prompt on first load, even when IPv6
  would have answered first. A denial is sticky. And requests from a service worker need the
  permission granted to the worker's origin beforehand **[V]**: W2b routes media through a service
  worker.
- *The plan is stale.* `webrtc-transport.md` W2c still says "a better one answering a short while
  later taking over", the rule the owner replaced **[V]**.

**Recommendation.**

1. **Rank by kind first; measure round trip only to break ties within a kind, and to demote a path
   whose round trip is far worse** (for example more than 3× and more than 50 ms over the best).
   Use the median of 3–5 small requests over the already-open connection (`GET` of a tiny
   endpoint), not one sample. Throughput is not measured at choice time; it is learnt from real
   chunk downloads afterwards and used only on the next visit.
2. **Order:** LAN HTTPS > IPv6 HTTPS > IPv4 HTTPS > WebRTC (direct candidate pair) > WebRTC
   (server-reflexive pair) > owner's TURN.
3. **Hysteresis:** after the choice, switch only on failure of the current path (a request error,
   or two consecutive timeouts), never on a slightly better measurement. On failure, use the next
   path still known good, and re-race in the background. Re-race on `online`/network change events
   and on page visibility after a long hide.
4. **Keep sockets where they are.** The Watch Together socket stays on its path until that path
   fails; on a switch it reconnects with its session's resume (the hub already has a `hello`).
5. **Do not race the LAN name blind.** Start it only if no public path has answered within ~300 ms,
   or when the remembered winner for this network was the LAN. Where Chrome supports it, annotate
   the request with `targetAddressSpace: "local"`. Explain the prompt before triggering it.
6. **Remember winners per network** (keyed on something stable the page can see, such as the
   discovery-reported client address prefix), not globally.
7. Update W2c of `webrtc-transport.md` to the amended rule.

### C6 — WebRTC set-up and recovery (high)

**Sections:** protocol §2.5, §3.2; spec §1.3; `rtc-transport.ts`, `internal/core/rtcapi`.

**The design says.** No trickle: "candidates gathered first". Four connections per client; the core
refuses past 64. Hole punching succeeds in "about 70%" **[V in research]**.

**Defect.**

- *All or nothing.* `connectRtc` opens four connections with `Promise.all`
  (`rtc-transport.ts:368–378`) **[V]**: if one of the four fails (a refusal at the core's limit, a
  lost offer, one ICE check failing), the whole transport fails and the three that connected are
  discarded.
- *A dead connection attracts traffic.* `RtcTransport.fetch` sends each request on the link with the
  fewest under way (`rtc-transport.ts:331–343`) **[V]**. A connection that has failed answers with
  an error at once, so its count stays at zero and it is chosen for every next request **[I]**. Nothing
  watches `connectionstatechange` after the connection is up (`:419–436`) **[V]**.
- *No ICE restart.* Neither side renegotiates (`rtcapi`: every offer makes a new connection)
  **[V]**. RFC 8445 §9: changing a stream's destinations "can be done only by using an ICE
  restart", and "during the restart, data can continue to be sent using existing data sessions"; the
  W3C API is `restartIce()` **[V]**. RFC 7675 ends a dead path at 30 s without consent **[V]**. A
  phone that moves from Wi-Fi to mobile data loses the connection, and the page has no plan for it.
- *Gathering is slow by design.* The client waits for gathering to complete or 10 s
  (`rtc-transport.ts:21, 402–417`) **[V]**; the core gathers with a 5 s limit and refuses when it
  passes (`rtcapi.go:37, 151–163`) **[V]**, the same as pion's own 5 s STUN gather timeout
  (pion/ice v4.4.7) **[V]**, so a slow STUN answer turns into a refusal. In Chrome, an unanswered
  STUN server holds back "complete" through libwebrtc's retransmission schedule (250 ms initial,
  9 sends, 8 s cap), about 40 s in total **[V constants; I sum]**: the client's 10 s cap is what
  ends it, on every page load from such a network. Trickle ICE (RFC 8838, with its half-trickle
  mode) exists to avoid exactly this **[V]**.
- *LAN WebRTC from a container.* Browsers hide their host addresses behind `.local` names unless the
  page has microphone permission **[V draft; S Chrome]**; pion resolves them only by multicast DNS
  (default mode QueryOnly) **[V]**. On the LAN the browser still reaches pion's own host candidate
  and pion learns the browser's address as peer-reflexive **[I]**, so the LAN path depends on the
  core's host candidate being reachable, which it is not when the core runs on a Docker bridge
  network **[I]**.
- *Four offers in series.* The core's signalling loop handles offers one at a time
  (`signal.go:652–664`) **[V]**, so a client's four offers queue behind each other's gathering:
  up to 4 × 5 s.
- *Capacity.* 64 connections at four per client is 16 WebRTC clients per core; a slot is held from
  the offer until ICE fails (about 30 s) even for a client that gave up (`rtcapi.go:104–131`)
  **[V]**. A Watch Together evening with more than 16 remote viewers behind carrier NAT is refused.
- *Racing WebRTC on every load* sends four offers through the discovery and holds four core slots
  for every client that then picks HTTPS.

**Recommendation.**

1. Race WebRTC with **one** connection. Open the other three only once WebRTC is chosen, and use
   each as it connects (`Promise.allSettled`, keep the ones that work, retry the rest later).
2. Watch each connection's state; take a failed one out of the rotation at once, and replace it.
3. **Half-trickle:** send the offer once host candidates and the first server-reflexive candidate
   are in, or after ~1 s, whichever is first; let the core do the same with its answer. If full
   trickle over the signalling socket is wanted later, RFC 8838 is the shape.
4. Handle offers concurrently on the core (each in its own goroutine, under the connection limit).
5. ICE restart through the discovery (or over the data channel itself while it lives) when the
   connection goes `disconnected` for more than a few seconds; fall back to a new race on `failed`.
6. Count a slot only once ICE connects, with a short (≈10 s) bound on unconnected offers; size the
   cap by memory, not a fixed 64, and say on the System page when it refused anyone.
7. Bundle a client's offers: one signalling round trip may carry up to four offers.

### C7 — Which addresses the core reports, and how fast they change (medium)

**Sections:** protocol §2.4, §2.6; `cmd/orpheum/discovery.go:164–183`.

**The design says.** Interface addresses, polled every minute, sent at once on change; heartbeat
every 5 minutes; the probe after each change and hourly.

**Defect.**

- *Temporary IPv6 addresses are published.* `net.InterfaceAddrs` exposes no flags, so temporary
  (privacy), deprecated and tentative addresses are all sent **[V, code]**. RFC 8981 temporary
  addresses have a default preferred lifetime of 1 day and valid lifetime of 2 days, with up to
  three at once **[V]**. Each rotation sends a new candidate set and a new probe, and a client that
  remembered a temporary address's name finds it gone. A FRITZ!Box's IPv6 port sharing follows the
  device's configured interface identifier; users report pinning it to the stable suffix before it
  works reliably **[S, nocksoft.de; AVM's own page now 404]**, so a temporary address is usually not
  reachable anyway.
- *Prefix changes.* A Telekom DSL user reports the delegated prefix changing every night since
  October 2023, while other lines change only on reconnect or after up to 180 days **[V, forum posts
  only: weak]**. Assume daily changes are possible. RFC 8978: after a router reboot hosts may keep
  using stale prefixes "on the order of several days", since an unauthenticated RA cannot shorten a
  valid lifetime below two hours **[V]**. The probe is what removes such addresses from what clients
  see, which is right; a heartbeat of 5 minutes with send-on-change is ample against this churn
  **[I]**.
- *Non-public IPv4 published as public.* Go's `netip` does not class `100.64.0.0/10` (RFC 6598
  shared address space, carrier NAT and Tailscale), `192.0.0.0/29` (DS-Lite's B4) or documentation
  ranges as private, so they are sent as public candidates and probed **[V, code]**. Docker bridge
  addresses (`172.17.0.1`) are sent as LAN candidates.
- *Intervals are adequate.* One-minute polling with send-on-change keeps the discovery within about a
  minute of a change, far inside prefix or DHCP churn. Netlink's address events would make it
  immediate and cheaper **[I]**. Heartbeats and retries carry no jitter (`discovery.go:50, 456`)
  **[V]**: after a discovery restart, every core returns within the same 30 s.

**Recommendation.** On Linux, read addresses through netlink (`RTM_GETADDR`) and skip
`IFA_F_TEMPORARY`, `IFA_F_DEPRECATED`, `IFA_F_TENTATIVE` and `IFA_F_DADFAILED`; prefer the stable
(RFC 7217 or EUI-64) address; classify RFC 6598, RFC 7335 and documentation ranges as non-public;
skip container bridges. Add ±20% jitter to heartbeats and exponential backoff with jitter to every
retry and to the signalling reconnect (the document says "backoff"; the code waits a flat 30 s,
`signal.go:626` **[V]**).

### C8 — Signatures are not bound to the discovery (medium)

**Sections:** protocol §2.2, §2.3; `discovery/proof/proof.go:9–19`.

**The design says.** The core signs `orpheum-discovery-v1:<purpose>:<nonce>`; a nonce works once;
the two purposes cannot stand in for each other.

**Defect.** The signed text names neither the discovery nor the `coreId` **[V]**. A discovery the
core talks to (an own discovery run by someone else, a misconfigured URL, or the core moving from
one discovery to another) can fetch a session challenge from a second discovery where the same key
is enrolled, pass it to the core as its own challenge, and get a valid session signature back. It
then holds a session as that core there: it can set the ACME TXT for `*.<coreId>.<zone>` (and so
obtain a certificate for the core's names) and take its signalling socket. This is a textbook relay;
the practice is to bind the audience into what is signed, as WebAuthn binds the RP ID and origin
**[I, practice]**.

**Recommendation.** Sign `orpheum-discovery-v1:<purpose>:<discovery origin>:<coreId or
public key>:<nonce>`, where the origin is the base URL the core itself called. Bump the signature
context in the same major, since nothing is deployed.

### C9 — Port mapping: double NAT, router restarts, ports browsers refuse (medium)

**Sections:** protocol §2.4 (`mapped`); spec §9.1 item 14; `internal/core/portmap`.

**Defect.**

- *Double NAT and carrier NAT.* The external address returned by PCP, NAT-PMP or UPnP's
  `GetExternalIPAddress` is not checked; a private or `100.64/10` address (the inner router of a
  double NAT, or a router behind carrier NAT) is published as a public `mapped` candidate
  (`pcp.go:533, 585`; `upnp.go:331`) **[V]**. The probe fails it, but the System page says "port
  mapped" for a mapping that cannot work, which is the wrong message.
- *"On by default" is not "works by default".* A FRITZ!Box blocks all inbound connections by
  default and grants UPnP IGD or PCP mappings only to a device for which the owner ticked "Permit
  independent port sharing" **[V, AVM knowledge base]**. Tailscale: CGNAT ISPs "tend to turn [port
  mapping] off" in the routers they supply **[V]**. UPnP answered in about 35–40% of home networks
  measured **[V, PAM 2012; IMC 2016]**, and mappings are "purely advisory": routers may expire them
  silently **[V, libp2p study]**. The System page should say which of these it met, with the FRITZ!Box
  step spelled out.
- *Router restarts.* The NAT-PMP and PCP epoch is never read and ANNOUNCE is not listened for
  **[V]**. RFC 6886: on an epoch lower than expected, the client "MUST immediately renew all" its
  mappings; RFC 6887 detects a reboot by the epoch "going backwards" and announces it on
  224.0.0.1:5350 **[V]**. A rebooted router loses the mapping for up to 30 minutes.
- *Nested NAT.* RFC 6887: a mapping on an outer NAT is "pointless" unless the inner one is mapped
  first **[V]**; asking only the default router (as built) is right, but its answer must be checked
  (below).
- *Port conflicts.* UPnP always asks for external port = internal port, with no fallback on 718
  (ConflictInMappingEntry); IGD 2's `AddAnyPortMapping` exists for this (`upnp.go:305–336`) **[V]**.
- *Ports browsers refuse.* D274 lets port mapping "use a high port of the core's choosing", and a
  router may grant another than asked. The Fetch standard's bad-port list includes 4190, 5060, 5061,
  6000, 6566, 6665–6669, 6679, 6697 and 10080 **[V, Fetch §2.10]**: a page cannot fetch from them at
  all.
- *IPv6 pinholes* are re-added rather than updated on renewal, and old ones are not deleted
  (`route.go:629–645`) **[V]**; they may pile up or be refused, router by router.
- *PCP nonce per process:* after a crash, a restarted core's request for the same mapping is likely
  refused until the old one lapses **[V nonce; I router behaviour]**.

**Recommendation.** Classify the external address; when it is not public, report "a second router or
your provider's NAT is in front: mapping cannot reach the internet" and publish nothing. Read the
epoch on every answer and listen for ANNOUNCE; on an epoch going backwards, re-create every mapping.
Fall back to `AddAnyPortMapping` and then to another port on conflict. Never request, and never
publish, a bad port. Keep the PCP nonce in the core's state so a restart can renew or delete. Use
`UpdatePinhole` and `DeletePinhole`.

### C10 — The outside probe tests less than `reachableFrom` claims (medium)

**Sections:** protocol §2.6; spec §1.3, §7.3.

**The design says.** The probe tests WebRTC "from behind an ordinary NAT", and `reachableFrom`
reports `everywhere`, `ipv6OrWebrtc` or `nowhere`.

**Defect.**

- *The probe passes where clients fail.* A probe VM in a data centre has a public address.
  Punching from a public address to the core succeeds whenever the core can send UDP out,
  regardless of the core's NAT type, so a passing WebRTC probe says little about clients behind
  their own NATs **[I]**. `everywhere` also overstates a mapped IPv4 on a high port for clients on
  networks that allow only 80 and 443 (hotels, some corporate networks).
- *The 70% is not the figure for this design.* The libp2p measurement (4.4 million attempts) found
  70% ± 7.1%, but its later version calls it "a conditional success rate … given that prerequisite
  relay reservation and public address discovery succeed", among libp2p nodes, not browsers
  **[V, arXiv 2510.27500 and 2604.12484]**. In WebRTC field data, 22% of conferences needed a TURN
  relay **[V, callstats via webrtcHacks, 2016]**. Carrier NAT is in over 90% of cellular networks,
  and those are "bimodal": about 40% symmetric **[V, Richter et al., IMC 2016]**. Tailscale reaches
  "well north of 90%" direct only with birthday probing, port mapping and its relays, and says "two
  devices, each behind 'hard NAT,' will almost always need to use a relay" **[V]**. A browser has
  none of those tools. So the share of WebRTC-only households a phone on mobile data cannot reach
  is likely well above 30% **[I]**. That matters to D274's choice of WebRTC as the baseline, and to
  the no-relay promise of the paid tier: say the number honestly, and measure it in the beta (D274
  §3 already plans this).

**Recommendation.** Measure the core's NAT mapping behaviour directly, as RFC 5780 describes: the
core sends STUN binding requests to the discovery's STUN server and the probe VM's STUN server (two
addresses) and compares the mapped ports. The same port means endpoint-independent mapping (punching
works with most clients); different ports means endpoint-dependent (punching works only with clients
that have IPv6 or an open NAT). Report that, and name `reachableFrom` by what was tested
(`publicIpv4On443`, `publicIpv4OtherPort`, `ipv6`, `punchableEim`, `punchableOnlyFromOpenNat`,
`nowhere`).

### C11 — Session handling: 401, clock skew, concurrent renewals (medium)

**Sections:** protocol §2.3; `internal/core/discovery/discovery.go`.

**Defect.**

- The document says "renews … at once on a 401"; the code waits 30 s (`discovery.go:444–457`; the
  test expects `wait == retryAfter`) **[V]**.
- After a discovery restart (paid: a deploy) a claimed core's step makes no call until its next
  heartbeat, while its signalling loop redials every 30 s with the stale token and its 401 never
  clears it (`discovery.go:411–413`, `signal.go:636–647`) **[V]**: signalling is down for up to 5
  minutes after every deploy of the discovery. Every WebRTC-only household is unreachable for that
  time.
- Expiry is compared as an absolute instant from the server's clock (`discovery.go:464, 483`)
  **[V]**. A core clock 55 minutes fast renews on every step; a slow one sends expired tokens.
- Three callers renew concurrently (step loop, signal loop, certificate job) with no lock; each
  spends a challenge **[V]**. `single`'s token map is never pruned (`single.go:265, 278`) **[V]**.
- No WebSocket ping on either side **[V]**: a half-open socket (NAT idle timeout on the home router,
  commonly minutes for TCP) is found only by TCP keepalive.

**Recommendation.** Send `expiresIn` (seconds) and compute expiry on the core's monotonic clock.
One renewal at a time (a single-flight around `session()`). On any 401, from any caller, clear the
token and renew at once, once; then back off. Ping the signalling socket every 25–30 s from the
core, and treat two missed pongs as dead. The discovery should accept a session token through a
deploy (it is hashed in SQLite: keep it), and the core should reopen signalling within seconds of
the socket closing, with jitter.

### C12 — Enrolment and claiming edge cases (medium)

**Sections:** protocol §2.2; `discovery/single/single.go:190–236`; `discovery.go:228–256, 326–328`.

**Defect.**

- *Any key replaces a pending core* in `single` **[V]**: a stranger who can reach `/enrol` during the
  claim window replaces the owner's pending core; the owner's core then gets 403 "enrol again" on
  `/session` but treats it as an ordinary failure and retries every 30 s forever (`discovery.go:
  326–328, 553–554`) **[V]**. With the 256-nonce cap shared by everyone (`single.go:34, 168–175`), an
  unauthenticated caller can also block enrolment and session renewal **[V]**.
- *Lost enrol response:* `single` returns the same coreId for the same key (good); the paid service's
  behaviour is unspecified. A double click enrols twice (no lock across `Connect`) **[V]**.
- *Expiry:* the document says an unclaimed pending core is deleted at 30 minutes; `single` instead
  issues a fresh code on the next `/status` **[V]**. The latter is the better behaviour for a core
  still polling; the document should say so.

**Recommendation.** Specify enrolment as idempotent per public key (the same pending core and coreId,
a fresh code if the old one expired). Never let a second key displace a pending core in an own
discovery; refuse it with 409. Treat "enrol again" (a distinct error code, not a bare 403) as a
state the core acts on: re-enrol and show the new code. Per-source limits for nonces, not one global
pool. Add the claim check code (Q19) for the paid dashboard: it is the standard defence against a
code read out by someone else (the same reasoning as D288 §3).

### C13 — Versioning: three version signals, none negotiated (low)

**Sections:** protocol §5; `discoveryproto.go:14`; `single.go:115–119`.

**Defect.** The major is in the path (`/core/v1`), in a header (`Orpheum-Discovery-Protocol: 1`) and
in the body (`"protocol": "1.0"` in the document, `"1"` in code) **[V]**. `single` requires the
header to equal `"1"` exactly; the body field is never checked; `/signal` skips the header check
**[V]**. The document's "full version in every request" and its minor version have no defined
meaning. "The previous two majors … each for at least six months after it is superseded, whichever
is longer" is ambiguous (longer than what?).

**Recommendation.** One source: the path carries the major; one header carries `major.minor`; drop
the body field. A discovery accepts any minor of a major it serves (additive) and uses the minor
only to know which optional features the core has. Restate the window as "a major is served until
both (a) two newer majors exist and (b) six months have passed since its successor shipped". Check
the header on `/signal` too.

### C14 — The owner's TURN credentials travel through the discovery (low)

**Sections:** protocol §2.4 (`turn`), §3.2 (`iceServers` with `"owner": true`).

**Defect.** A TURN server needs credentials; the JSON shows none, and the struct has static
`username`/`credential` fields (`discoveryproto.go:121–126`) **[V]**. Static long-term credentials
handed to every holder of a locator, through the discovery, are reusable by anyone who copies them.
(The code never sends `turn` today: `cmd/orpheum/discovery.go:41–48` **[V]**.)

**Recommendation.** Use time-limited credentials (the TURN REST API pattern: username
`<expiry>:<id>`, password an HMAC with a secret shared by the core and the TURN server, as coturn's
`use-auth-secret` implements) **[I, practice]**, minted by the core per client, and refreshed with
each heartbeat if they must pass through the discovery.

### C15 — Names that encode addresses (low)

**Sections:** protocol §2.7.

**Checked and sound:** an IPv6 address as eight dashed groups is at most 39 characters, inside DNS's
63-octet label and 253-octet name limits (RFC 1035 §2.3.4) with a 26-character coreId and any
ordinary zone **[I, arithmetic]**; it never begins or ends with a hyphen; one wildcard label covers
it, since `*.<coreId>.<zone>` matches exactly one label. Plex uses the same scheme
(`1-2-3-4.<hash>.plex.direct`) with per-server wildcard certificates **[V, Valsorda]**.

**Gaps.**

- *Compression is not ruled out.* The core sends addresses as `netip.Addr.String()`, which
  compresses (`2001:db8::20`) **[V]**, and nothing yet builds names. A name built by replacing `:` with
  `-` gives `2001-db8--20`: a doubled hyphen, and a trailing hyphen for an address ending in `::`.
  Labels with `--` in positions 3–4 are reserved (RFC 5891 §4.2.3.1), and strict IDNA processing
  rejects them **[I]**.
- *Canonical form.* Decide lowercase, no leading zeros (`2a02-8070-0-0-0-0-0-1`) or zero-padded, so
  one address has one name.
- *Zones and scopes.* Link-local addresses (with a zone id) are excluded by the code (good); ULA
  addresses are classed `lan` and would be named; IPv4-mapped addresses are unmapped first (good).
- *Ports.* A non-443 port goes in the URL; see C9 on bad ports.

**Recommendation.** Write the encoding into `discoveryproto` as a function with test vectors (eight
groups, lowercase, no zero padding, no compression), so the paid service, `single` and the client
share one implementation (rule 11).

### C16 — Smaller gaps (low)

- **Offline is not marked** in `single`, and the 30-minute rule is late when a socket closing says
  it at once **[V]**. Use the signalling socket's presence as liveness; keep the 30-minute rule for
  heartbeats only.
- **`DELETE /acme-challenge` carries a JSON body** (`discovery.go:173–180`) **[V]**. Bodies on DELETE
  have no defined semantics (RFC 9110 §9.3.5) and some proxies drop them; put the name in the path.
- **The `frozen` lapse state** keeps names "at the last addresses"; with computed DNS (C2) names
  answer regardless, so `frozen` is a property of `/paths` and of TXT/persist records, not of DNS.
  Write it that way.
- **The service worker and API majors.** A cached client build for one core API major meets a core
  that upgraded its major while the discovery was down. The page must read the core's major on the
  control connection and swap builds; a service worker needs a remote kill switch (an update check
  that can unregister it) **[I]**.
- **Probe candidate text** is `"2a02:…:1"` in one example and `"198.51.100.9:443"` in another
  **[V]**: specify `address`, `port` and `kind` as fields.
- **The certificate stays served after leaving** (`cmd/orpheum/certificates.go:56–69`) **[V]**:
  harmless (names are removed) but it should be dropped with the rest.

## Is the split right; is there something simpler?

**The split is right.** The core holds everything secret (its key, its ACME account, its certificate
key, its users) and makes every outbound call; the discovery holds only what must be public or
shared (names, current addresses, a rendezvous socket). That is Plex's split for names and
certificates and the usual one for rendezvous services. The one place the split blurs is locators
(C3): make the core their only issuer.

**Simpler, doing the same job:**

1. **One connection, not two.** Carry the heartbeat (`addresses`) on the signalling WebSocket the
   core already holds. Liveness becomes the socket's presence (instant offline), renewal and
   re-authentication happen once per socket, and a whole class of races (C11) goes. Keep `PUT
   /addresses` only as the fallback when the socket cannot open (a proxy that refuses WebSockets).
   This is the long-poll control channel shape that Tailscale's coordination server uses **[I]**.
2. **Computed DNS from the start** (C2): no per-address records, no Cloudflare record limit, no
   record clean-up, and `frozen`/`released` become a lookup in a set of coreIds.
3. **DNS-PERSIST-01** (C1): one record at claim; no TXT proxy in the renewal path; the lapse states
   map onto `persistUntil`.
4. **A fixed preference order with round-trip tie-breaks** (C5), instead of a weighting to be
   settled on measurements.
5. **Drop the hourly HTTPS probe** in favour of probe-on-change plus the clients' own reports of
   which path won (D274 §3 already records them). The hourly probe of every candidate adds load and
   finds what the clients would report anyway **[I]**.

## What is sound

- **Outbound-only cores with a held socket for signalling**, no inbound port needed for the
  baseline (D274 §2). It is the right baseline for carrier NAT and DS-Lite.
- **The core runs ACME and keeps its key; the discovery only writes DNS.** The discovery never
  holds a TLS key, and the CT logs show only a random label per core.
- **Address-encoded names under one per-core wildcard**, the plex.direct shape, proven at the scale
  of millions of servers **[V, Valsorda]**: an address change needs no new certificate and no DNS
  propagation.
- **A dedicated registered domain for core names**, never `orpheum-app.com`.
- **IPv6 first.** A public IPv6 address is "public" to Chrome's Local Network Access, so it meets
  no prompt **[V, Chrome LNA]**, and no rebinding filter.
- **Header sign-in, no cookies cross-site, CORS by list**: the right answer for a page on one
  origin calling cores on others.
- **Publishing only probed public candidates**, with no history kept.
- **Port-mapping lifetimes**: one hour asked, renewed at half of what was granted (`portmap.go:30,
  312–327`) **[V]**, as RFC 6886 and 6887 recommend.
- **Nonce single use and purpose separation** in signatures (`proof.go`; `single.go:182–188`)
  **[V]**, with the audience gap of C8.
- **Full-set replacement** for locate-hashes and locators is idempotent by construction (once C3's
  ownership is fixed).
- **Additive JSON, unknown fields ignored everywhere** (`DisallowUnknownFields` nowhere) **[V]**.
- **Restart mid-enrolment** recovers (`discovery.json` keeps base URL, coreId and zone) **[V]**.
- **Coming from a remembered path when the discovery is down** (spec §7.6) is right, provided C5's
  re-race and the per-network memory exist.

## Mismatches with the code

| Document | Code | Where |
| --- | --- | --- |
| `"urls": "turn:…"` (a string) in §2.4 and §3.2 | `URLs []string`; a discovery following the document's example makes every `GET /status` fail to decode | `discoveryproto.go:123`, `discovery.go:558` |
| `"protocol": "1.0"` | `ProtocolVersion = "1"` in header and body | `discoveryproto.go:14` |
| `reachableFrom: ipv6OrWebrtc` | `ipv6_or_webrtc` (also in the API enum and SDK) | `discoveryproto.go:147`, `internal/core/api/discovery.go:35` |
| `coreId` 26 safe32 characters (130 bits) | 8 characters (40 bits) in `single` | `single.go:40, 417–424` |
| Renews "at once on a 401" | after 30 s | `discovery.go:444–457` |
| Signalling reopened "with backoff" | flat 30 s, no jitter | `signal.go:626` |
| Unclaimed pending core deleted after 30 min | a fresh code issued on the next `/status` | `single.go:290–295` |
| A core silent 30 min marked offline | not marked | `single.go:327` |
| Leave removes the core | the signalling socket and tokens stay live | `signal.go:615–664`, `single.go:371–381` |
| Re-key by a claim code from the new key | not built; `single` refuses a new key while claimed (409) | `single.go:206–207` |
| IPv6 names as eight dashed groups | no name builder; addresses sent compressed | `cmd/orpheum/discovery.go:164–183` |
| `turn` sent with the heartbeat | never set | `cmd/orpheum/discovery.go:41–48` |
| `iceServers` entry `"owner": true` | no such field | `discoveryproto.go:121–126` |
| Probe `candidate` with and without port | free text | `discoveryproto.go:139–142` |
| "The full version in every request"; previous majors served | exact match on `"1"`; body field unchecked; `/signal` unchecked | `single.go:107–119` |
| `orpheum discovery claim <code>` | `single` is used only by its tests; no command | `discovery/single/core_test.go` |
| W2c: "a better one answering a short while later taking over" | superseded by the amended D273 §7 (document text, not code) | `plans/webrtc-transport.md` |
| `SwitchableFetch`'s comment: "a better one answering later takes over" | the same superseded rule | `rtc-transport.ts:443–446` |

## Sources

Standards and specifications:

- RFC 1035 (DNS names and labels), RFC 5891 §4.2.3.1 (hyphen restrictions), RFC 9110 §9.3.5
  (DELETE): https://www.rfc-editor.org/
- RFC 8555 (ACME), RFC 9773 (ACME Renewal Information):
  https://www.rfc-editor.org/rfc/rfc9773.html
- RFC 8657 (CAA `accounturi`, `validationmethods`).
- RFC 5780 (NAT behaviour discovery), RFC 6598 (shared address space), RFC 6333 (DS-Lite).
- Fetch standard, "bad port": https://fetch.spec.whatwg.org/ (cached `fetch-bad-ports.html`).

Let's Encrypt and the PSL:

- Rate limits: https://letsencrypt.org/docs/rate-limits/ (cached `le-rate-limits.html`).
- "From 90 to 45 days", 2025-12-02: https://letsencrypt.org/2025/12/02/from-90-to-45 (cached
  `le-90-to-45.html`).
- "DNS-PERSIST-01", 2026-02-18: https://letsencrypt.org/2026/02/18/dns-persist-01 (cached
  `le-dns-persist.html`).
- Multi-perspective validation:
  https://community.letsencrypt.org/t/lets-encrypt-is-adding-two-new-remote-perspectives-for-domain-validation/214123
- PSL contributing notes and guidelines:
  https://github.com/publicsuffix/list/blob/main/CONTRIBUTING.md,
  https://github.com/publicsuffix/list/wiki/Guidelines (cached `psl-guidelines.md`,
  `psl-wiki-guidelines.md`).

Browsers, DNS, Plex:

- Chrome, "New permission prompt for Local Network Access":
  https://developer.chrome.com/blog/local-network-access (cached `chrome-lna-blog.html`).
- Cloudflare DNS record quotas: https://developers.cloudflare.com/dns/manage-dns-records/ (cached
  `cf-manage-dns.md`).
- Filippo Valsorda, "How Plex is doing HTTPS for all its users":
  https://words.filippo.io/how-plex-is-doing-https-for-all-its-users/ (cached `filippo-plex.html`).

NAT traversal, WebRTC, port mapping, IPv6:

- Trautwein et al., DCUtR measurement: https://arxiv.org/abs/2510.27500 and the later version
  https://arxiv.org/abs/2604.12484 (the "conditional" wording).
- Tailscale, "How NAT traversal works": https://tailscale.com/blog/how-nat-traversal-works; "NAT
  traversal improvements, pt. 1": https://tailscale.com/blog/nat-traversal-improvements-pt-1
- webrtcHacks, callstats usage statistics (2016): https://webrtchacks.com/usage-stats/
- Richter et al., "A Multi-perspective Analysis of Carrier-Grade NAT Deployment", IMC 2016:
  https://arxiv.org/abs/1605.05606
- Probe and Pray (UPnP), PAM 2012: https://www.icir.org/christian/publications/2012-pam-upnp.pdf
- RFC 8445 (ICE) §9, §11; RFC 8838 (Trickle ICE); RFC 7675 (consent freshness); RFC 8841
  (max-message-size); W3C WebRTC `restartIce()`: https://www.w3.org/TR/webrtc/
- libwebrtc `p2p/base/stun_request.cc` (STUN retransmission constants); pion/ice v4.4.7
  `agent_config.go` (gather and ICE timeouts); draft-ietf-mmusic-mdns-ice-candidates-03.
- RFC 6886 (NAT-PMP), RFC 6887 (PCP), RFC 6970 (UPnP IGD lease), RFC 7648 (PCP proxy).
- AVM, automatic port sharing on the FRITZ!Box:
  https://uk.fritz.com/service/knowledge-base/dok/FRITZ-Box-7530-AX/894_Setting-up-automatic-port-sharing-in-the-FRITZ-Box/
- RFC 8981 (temporary addresses), RFC 8978 (flash renumbering), RFC 7217 (stable addresses),
  RFC 6092 (home gateway firewall).
- FRITZ!Box IPv6 sharing and the interface ID:
  https://nocksoft.de/tutorials/dyndns-fuer-ipv6-server-hinter-fritzbox-konfigurieren/; Telekom
  prefix changes (forum, weak):
  https://telekomhilft.telekom.de/conversations/festnetz-internet/ipv6-obfuscation/668942954ae73561da881da4

Fetched pages are cached under `/yoloai/cache/discovery-audit/`; facts taken from search snippets
only are listed in `websearch-notes-2026-10-13.txt` there.
