> **ABOUTME:** Second-round audit (2026-10-13) of the discovery design for consistency and factual accuracy: contradictions between decisions D72–D291, discovery-protocol.md, the paid spec (draft 5), the four plans and the code; decisions recorded differently from what the owner or the audits said; wrong or stale facts, re-checked at primary sources; and gaps no document decides.

# Discovery: consistency and fact audit, round 2 (2026-10-13)

**Read:** `design/discovery-protocol.md`; D72, D214, D218, D221, D243, D246, D267, D273, D274, D275,
D288, D289, D290, D291; the paid spec (`/yoloai/files/discovery-service-spec.md`, draft 5); plans
`discovery-core.md`, `discovery-mode.md`, `webrtc-transport.md` and
`orpheum-discovery/docs/plans/service.md`; the four first-round reports; the code in
`discovery/`, `internal/core/discovery/`, `internal/core/portmap/`, `internal/core/certificate/`,
`internal/core/auth/pairing.go`, `internal/core/backup/`, `internal/core/instancedir/`,
`internal/core/rtcapi/`, `web/packages/sdk/src/rtc-transport.ts`, at an earlier commit.

**Markers.** **[V]** checked this session at the source named under *Sources* (pages cached in
`/yoloai/cache/discovery-audit2/`) or in the code or module source at the path given. **[I]** my
inference. Precedence: a later decision overrides an earlier one; the owner's quoted words override a
paraphrase.

## Summary

The design is mostly coherent, but the fast rewrites left three kinds of damage.

1. **The code contradicts a decision in the opposite direction the plans say.** Every scheduled
   backup holds `identity.key`, the ACME account key and the TLS key, in plain text today. D289 §2
   says scheduled backups never hold the identity key, and the plans say no backup holds it (X1).
2. **Later rulings were not carried through.** The protocol's ranking still says "IPv6 or IPv4"
   (D291 §3 orders them). The spec's body still calls the DNS provider, the probe's host, the PSL
   and S8, S9, S17 and U7 "open", though its own §11.3b records D291 (X2, X3). "No relay, ever"
   contradicts D274 §2 and the owner's own "for now" (X6).
3. **Key facts behind the certificate plan have moved.** DNS-PERSIST-01 is **not** in Let's
   Encrypt's production, and LE will not ship it until the record carries client-computed data.
   Draft-02 replaced the plain account URI with a hash the core must compute (F1). The CAA record
   the protocol prescribes (`validationmethods=dns-01`) would **refuse** DNS-PERSIST-01 if it
   shipped (X5).

Other findings worth action:

- The protocol's version window drops one major too early against D273 §7 (R1).
- The pairing QR cannot carry a DTLS fingerprint, since `rtcapi` makes a fresh certificate for each
  connection (X7).
- The plans call their early-answer rule "half-trickle", which is not what RFC 8838 means (F2).
- The UPnP story behind D290 §9 is wrong: NETSCOUT and Plex blame DMZ and manual port forwards, not
  UPnP (F3).
- Safari's 7-day rule is about user interaction, and `persist()` does not lift it (F8).
- The spec's database load figure is wrong by a factor of 1,000 (F9).

Of the planner's 17 + 3 + 5 raised questions, I confirm every contradiction it named (X4, X8–X15).
The other items below are new.

## Contradictions

**X1. The identity key is in every scheduled backup (code), against D289 §2 and the plans.** [V]
- D289 §2 and protocol §2.1 say it is "out of scheduled backups, in *Back up now* when its box is
  ticked". `discovery-core.md` (C1 row, K8a, "Found while planning") says "`internal/core/backup`
  holds `secret.key` alone", read as "the identity key is not in any backup".
- The code is the reverse. `backup.write` leaves out only `secrets.KeyFileName`, and
  `instancedir.persistentFileNames` archives every other persistent file. So every scheduled backup
  holds `identity.key` (the raw Ed25519 seed, `identity.EncodeKeyFile`), `acme-account-key.pem`,
  `tls-key.pem` and `discovery.json`.
- This also breaks `internal/core/backup/doc.go`'s invariant "A backup holds ciphertext only".
- U6's scenario ("a dead disk restored from a scheduled backup always loses the key") is the
  design, not what was built. As built, a restore brings the key back, and a restore on second
  hardware produces C4's twin cores.
- **Right:** D289 §2.
- **Fix:** correct the C1 row and "Found while planning". K8a's test must be "a scheduled backup
  does **not** hold `identity.key`", which fails today. Decide whether the ACME account key and TLS
  key follow `secret.key` too (they let anyone with a backup impersonate the core's names). Fix the
  backup doc's invariant or the code.

**X2. Protocol §3.3 item 4 ranks "IPv6 or IPv4"; D291 §3 puts IPv6 first and demotes slow paths.**
- The protocol records D291 §3 only in §7 item 24. Its body (§3.3 item 4) still has D290 §2's
  unordered pair and no demotion rule. `webrtc-transport.md` W2c has the D291 form.
- C5 item 2 (taken by D290 §18) also orders WebRTC direct before server-reflexive, with the owner's
  TURN last. §3.3 says nothing of either.
- **Fix:** rewrite §3.3 item 4: LAN > IPv6 > IPv4 > WebRTC (direct pair > reflexive > owner's
  TURN), with ties broken by the median round trip, and demotion past 3× and 50 ms over the best.

**X3. The paid spec's body contradicts its own §11.3b and D291.** The spec says it was written
from an earlier commit, before D291. Only §11.3b was added. Stale places:
- §7.2: "the provider is open (§11.3)". D291 §2 chose Bunny. §10's risks say "the zone's DNS has no
  home".
- §7.3: "Where the outside probe runs is open". D291 §1 decided it. §11.2 Q14 still says "two
  machines"; there are now three. §7.3's "€15–25 a month" lacks D291 §1's €4.
- §3.11: "whether to file [a PSL entry] for cookie isolation is open". D291 §4 says no PSL entry now.
- §2 says "An absolute life (S8: 30 days) and mail on each new sign-in are open". §2 Recovery says
  "proof from the core … open". §3.10 marks U7's wording "open". §8 says "S9's, open". All are
  taken by D291 §6.
- §6's retention table logs "source address, core id" for requests. D291 §6 takes S17: device
  requests are logged without the core id. `service.md` P1 already has this.
- §10 item 2 says "port mapping on TCP 443 only". D291 §7 says 443 first, else the port granted.
- **Fix:** fold D291 into the spec's body (draft 6) and keep §11.3 for history only.

**X4. Which machines run STUN (planner's service.md Q2): confirmed.**
- Spec §7.1 and §7.3: STUN on the API machine and the spare.
- Protocol §2.6 and §2.9: the second STUN server is on "the probe machine".
- D291 §1 has the spare run "nothing that talks to strangers until it takes over".
- **Right:** D291 §1 is later. STUN runs on the API machine's own address and on the probe machine,
  not on the spare until promotion.
- **Fix:** spec §7.1 and §7.3.

**X5. Protocol §2.7's CAA record would block DNS-PERSIST-01, its own primary method.** [V]
- §2.7 and spec §3.11 set `issuewild "letsencrypt.org; accounturi=…; validationmethods=dns-01"`.
- RFC 8657 §4: a CA "MUST only consider a Property with the 'validationmethods' parameter to
  authorize issuance where the validation method being used is identified by one of the … labels
  listed".
- draft-ietf-acme-dns-persist-02 §7.7 names the label `dns-persist-01`.
- **Fix:** `validationmethods=dns-01,dns-persist-01` once LE lists the label. Its CAA page
  (2026-10-04) still lists only http-01, dns-01 and tls-alpn-01. Until then keep `dns-01` and change
  the record when persist ships. Also see F1 on `accounturi`.

**X6. "No relay, ever, in this tier" contradicts D274 §2 and the owner's brief.**
- Protocol §1 says "paid: no TURN, ever, D273 §2". Spec §1.2 says "No relay, ever in this tier …
  Not 'for now'". Spec §11.1 says "No relay, no TURN, ever".
- But the spec quotes the owner's brief as *"no content relay for now"*.
- D274 §2 (2026-10-10, later than D273): a relay of failed connections "is open in principle but
  not built: *'I'm open to the idea of a relay, but I'd want to see how much demand there is'*".
- U3, D290 §12 and D274 §3 count the dead end precisely so this can be decided.
- **Right:** D274 §2 and the owner's words.
- **Fix:** "no relay in the beta; a relay of failed connections waits on the beta's count (D274 §2,
  §3)".

**X7. The pairing QR's "fingerprint" is read as a DTLS fingerprint, which cannot be fixed.**
- Protocol §2.5 item 4, W2f and S6 have the client check an answer "against … the fingerprint a
  pairing QR carried", which is a DTLS fingerprint.
- D214 §1 has the QR carry "the core's identity (a key fingerprint)". Protocol §2.1 defines the
  fingerprint as `SHA-256(identity public key)`.
- `rtcapi` sets no `Certificates`, so pion makes a new DTLS certificate per peer connection [V:
  no certificate configured in `internal/core/rtcapi`]. A QR printed once cannot hold it.
- **Right:** D214 §1.
- **Fix:** the QR carries the identity-key fingerprint. The client checks `fingerprintSignature`
  against the key that fingerprint names.

**X8. "There is no code for anyone to read out" vs a typed fallback (planner's Q6): confirmed.**
- Protocol §2.2 item 2 and spec §3.11 say claiming has no code.
- D291 §6 takes S7 item 2 ("Keep the typed code as the fallback") and U5 ("Typing the code stays
  as the fallback"). `service.md` P4 builds the fallback with a dashboard warning and a
  per-account limit.
- **Right:** D291 §6.
- **Fix:** §2.2: the token is the primary path. The typed code stays as a fallback that still needs
  the core admin's *Allow*. Enrolment returns both.

**X9. The heartbeat on the socket and the hourly probe (planner's Q1): confirmed, and worse.**
- D291 §6 takes protocol §7 item 22: the heartbeat moves onto the socket and the hourly probe is
  dropped. Protocol §2.4 and §2.6 still describe them.
- D291 §6 also takes S10 item 3 ("at most one immediate probe per 10 minutes, the rest hourly").
  That item assumes the hourly probe exists.
- The blanket adoption therefore took two items that conflict.
- **Fix:** the owner picks one. If probing is on change only, S10's limit becomes "at most one probe
  per core per 10 minutes".

**X10. The bootstrap token (planner's Q13): confirmed.** §3.1 calls it "short-lived and
single-use" and also says it "serves `/paths` and offers for its few minutes". C3's original was
single-use for the hand-off. **Fix:** "spent by `/locate`'s issue; usable for `/paths` and offers
for N minutes; never renewed".

**X11. The enrolment's `protocol` field (planner's Q3): confirmed.** §2.2 shows
`POST /enrol {publicKey, protocol, …}`; §5 says the body carries no version. K1 drops it from the
heartbeat only, and `EnrolRequest.Protocol` stays. **Fix:** drop it from enrolment too (C13).

**X12. "Discovery origin" vs "base URL" (planner's Q2): confirmed.** The contradiction comes from
C8's own wording: "the origin is the base URL the core itself called". **Fix:** bind the base URL
the core called, path included, normalised without a trailing slash.

**X13. Link paths (planner's Q11): confirmed, and wider.** §3.1 has `/register#<coreId>.<secret>`
and `/join#…`. The built client makes `/register#<secret>` and `/together#<secret>` on its own
origin. No "the code must change" marker is on either. **Fix:** add the marker, or keep
`/together` and change §3.1.

**X14. The zone's TLD (planner's service.md Q1): confirmed.**
- D273 §9 (the owner: *"maybe something like orpheum-core.com? I'll see what I can register"*), spec
  §3.11 and spec §11.1 name `.com`.
- D291 §6 takes S12 item 2: a TLD on the HSTS preload list. `.app`, `.dev` and `.page` are preloaded
  whole [V, registry.google].
- **Right:** D291 §6 is later, and the owner's `.com` was tentative. The owner should still confirm
  it, since it is the one thing hard to change later.

**X15. Whom D291 §5's "ordinary rights" covers (planner's Q10): confirmed.** D291 §5 names no tier.
Protocol §3.1 places it under the paid tier. D288 §4 signs the device in "as the person". These read
differently for the core's own codes. Owner's question.

**X16. `service.md` contradicts its own location.**
- It sits uncommitted in `orpheum-discovery/docs/plans/`. It says it is "planned here,
  in Orpheum" and that "`orpheum-discovery` holds an empty repository".
- Its relative links (`../discovery-protocol.md`, `discovery-core.md`, `discovery-mode.md`,
  `webrtc-transport.md`, `../research/beta-inventory-…`) all break there.
- Its Q5 asks where it should live, yet it has already moved.
- **Fix:** settle Q5. Rewrite the text and links for wherever it lands. Link to the Orpheum docs by
  path, since they are in another repository.

**X17. The device-shows-QR flow is described two ways.**
- Spec §11.1 (D273 §9's option): the QR "names the core's locator and a one-time pairing secret".
- U9 and K15 (D291 §6): the QR beside the TV's code is `app.orpheum-app.com/activate#<code>`, the
  device code itself.
- The two disagree on what the QR carries and who approves.
- **Right:** D291 §6 builds D273 §9's option as U9's QR.
- **Fix:** spec §11.1.

**X18. The spec's §9.1 contradicts itself.** Item 11 lists port mapping (14) as "not yet"; item 14
says it was built 2026-10-10. Item 10 says the key is "built … but for the fingerprint", then lists
the fingerprint as done. Item 14 still says "a new dependency for UPnP/PCP, if any", though it was
hand-written.

**X19. Spec §2's actor table names "invited outsiders (D243 §4)".** D246 §1 abolished the outsider
kind of account.

**X20. D290 §9 says "maps TCP 443 only"; D291 §7 says 443 first, else the port granted.**
- D291 §7 is later. The protocol (§2.4, "the core's HTTPS port over TCP only") and K5 follow D291.
- D290 has no banner saying so (see G14), and spec §10 item 2 still says "443 only".
- As built, port mapping asks for the core's own `https.listen` port as the external port
  (`portmap.go:272`, `:284`), not 443.

**X21. Spec §3.11 overstates CAA.** It says CAA means "a stolen DNS token or a bug in the TXT proxy's
name check mints no certificate". S12 says CAA stops only someone with **TXT-only** access. A stolen
zone token can rewrite the CAA record too. **Fix:** "a bug in the TXT proxy's name check"; for
stolen tokens, CT monitoring and a narrow token.

**X22. D290 §13 vs the protocol.**
- D290 §13: "the document is brought in line with the code, but for a core's id".
- The protocol keeps the document and marks "the code must change" on renewal at once on a 401,
  backoff, offline marking, closing on leave, and IPv6 names, as C11, C16 and C4 recommend (D290
  §18).
- Only the wire spellings (`urls` as a list, `ipv6_or_webrtc`) and the pending-core expiry moved to
  the code.
- **Right:** D290 §18 together with the findings. The protocol's reading is right, but §6's summary
  repeats §13 as if it held generally. **Fix:** narrow §6's wording.

## Decisions recorded wrongly

**R1. The version window is restated one major short.**
- D273 §6, the owner: *"we should support the previous 2-3 versions"*.
- D273 §7: "answers the previous two major versions, each for at least six months after it is
  superseded, whichever is longer".
- Protocol §5, from C13: "a major is served until both two newer majors exist and six months have
  passed since its successor shipped". Once major N exists, N−2 has two newer majors, so it may be
  dropped. That serves only one previous major.
- **Fix:** "a major is served until **three** newer majors exist and at least six months have
  passed since its successor shipped". The paid spec's CI test must then build the oldest supported
  core accordingly.

**R2. D290 §18 "the rest of the correctness findings as the audit recommends" includes C12's
recommendation to "Add the claim check code (Q19)", which D290 §1 explicitly replaced.**
- **Right:** D290 §1, the specific ruling.
- The protocol is right. D290's text should exclude C12's check-code sentence.

**R3. D290 §9's reason is not what the sources say.** [V]
- D290 §9 and protocol §2.4 ("Plex servers were used to amplify attacks through a port UPnP opened
  **[V]**"), and the availability research ("because UPnP mapped their discovery port").
- NETSCOUT, citing Plex: the abused servers were "deployed on a public-facing network DMZ … or with
  manually configured port-forwarding rules". Plex: the device was "exposed to the public internet …
  or the server administrator has explicitly forwarded UDP traffic".
- The numbers are right: about 37,000 reflectors, 4.68:1, UDP 32410 and 32414.
- The rule (TCP only, no UDP responder exposed) still stands on its own [I].
- **Fix:** the reason, and drop the [V].

**R4. D290 §10 records Safari's rule as "seven days unvisited".** Protocol §1 and §3.1 say "seven
days without a visit".
- WebKit says "seven days of Safari use without user interaction on the site" [V]. The days are
  days the browser is used, and a visit without interaction does not count.
- WebKit's current tracking-prevention page also says home-screen web apps are **exempt**. They do
  not have "their own count", as the protocol says.
- **Fix:** the wording in protocol §1 and §3.1, spec §1.2 and §7.6, and D290 §10 (by a note).

**R5. D290's title says "computed DNS behind Cloudflare".** No item of D290 puts DNS behind
Cloudflare: §7 says computed and available, and §8 puts Cloudflare in front of HTTPS. D291 §2 put the
zone on Bunny. Spec §11.3 item 1 noticed it, but no banner on D290 says so (G14).

**R6. One owner label covers two rulings.** D290 §17 (Watch Together links, "B11; correctness C3")
and §18 (one core per account, "B11") both cite B11. The owner's two quotes are attached one to
each, so which words cover which ruling cannot be traced. **Fix:** relabel, or say B11 had two
parts.

**R7. D291 §6 adopts "the rest of the open list", but the list is incomplete.** These
recommendations are in neither protocol §7 nor spec §11.3, so whether they were taken cannot be
told:
- U6's dashboard prompt ("Is this a replacement for *Stenerud home*?"). It interacts with D290 §18's
  one core per account.
- S13 item 2: pending cores count against the inviting account.
- U11's 10-second message.
- C5 item 1's throughput, learnt from real chunks and used on the next visit.
- S18's tie of CORS `Max-Age` to revocation.

**Fix:** list them and ask, rather than infer.

**R8. Smaller paraphrase slips.**
- Spec §11.3b says the probe holds "no secret"; D291 §1 says "no secret but a token that can only
  post probe results".
- The protocol's status says it came from "draft 3" of the spec; the spec's changelog moves it in
  draft 4.

## Wrong or stale facts

**F1. DNS-PERSIST-01 is not in Let's Encrypt production, and its record changed.** [V]
- Protocol §2.7 and spec §3.11: "naming Let's Encrypt and the core's ACME account URI, with
  `policy=wildcard`". Production is "unconfirmed" [I, verify].
- Let's Encrypt (aarongable, 2026-06-25): "We will not be deploying dns-persist-01 until … Record
  should include client-computed information · Issue #64 … is resolved".
- LE's CP/CPS v6.2 (2026-08-12) lists no 3.2.2.4.22. No API announcement exists.
- draft-ietf-acme-dns-persist-02 (2026-09-20, a WG document) makes `accounturi` a hashed value,
  `<accountHashPrefix><hash-alg>/<base64url H(len(domain)‖domain‖key‖account URL)>`. Draft-01 and
  draft-02 records are incompatible. Google Trust Services ships draft-01.
- The core holds the key, so it must compute the value and report it. The discovery cannot derive
  it from an account URI alone.
- **Fix:** §2.7 and §4: the core reports the record's value, not just the URI. DNS-01 by proxy is
  the method for the beta. The 2027–2028 lifetimes stand [V], so the TXT proxy stays on every
  renewal's path until persist ships.

**F2. "Half-trickle" is the wrong name.** [V]
- RFC 8838 §2: in half trickle "the initiator gathers a full generation of candidates strictly
  before" sending. Only the responder trickles.
- Protocol §2.5, K3, W2e and C6 use the term for "each side sends after its host and first
  reflexive candidate, or about a second". That is a partial-gathering timeout with no trickle at
  all.
- The design is fine [I]. The name invites someone to implement RFC 8838 instead. **Fix:** rename
  it, for example "early description".

**F3. The Plex amplification cause** (R3).

**F4. "About 40% of mobile carriers' NATs are symmetric" overstates the source.** [V]
- Richter et al.: "For cellular networks we observe a bimodal outcome … (40% symmetric)". The unit
  is CGN-positive cellular ASes, each labelled by the most permissive STUN type seen, so it is a
  lower bound per network, not a share of NATs.
- "Over 90%" counts cellular ASes.
- **Fix:** protocol §3.2, spec §1.3 and C10.

**F5. libp2p's "conditional" wording is in another paper.** [V] arXiv 2604.12484 (IMC '26) is a
separate paper, not a later version of 2510.27500. The figures (70% ± 7.1%, 4.4 million attempts)
are right. Fix the citation in C10, protocol §3.2 and spec §1.3.

**F6. The Tailscale quotes in C10** ("well north of 90%"; two hard NATs "almost always need … a
relay") come from "How Tailscale is improving NAT traversal (part 1)", 2025-10-15. They are not from
"How NAT traversal works" [V].

**F7. Browser HPKE needs X25519, which Safari has only from 18.4.** [V]
- Protocol §3.1 and K15: HPKE in the browser is "built from WebCrypto's X25519, HKDF and AES-GCM".
- WebCrypto has no HPKE. X25519 arrived in Chrome 133, Firefox 130 and Safari 18.4 (WebKit's own
  notes; MDN says 17, which WebKit's 17 notes do not support).
- An iPhone below iOS 18.4 cannot activate by code on the paid tier unless a library supplies
  X25519. See G7.
- Go 1.26 does have `crypto/hpke`, with DHKEM(X25519), HKDF-SHA256 and AES-128-GCM [V,
  `$GOROOT/src/crypto/hpke`].

**F8. Safari storage: persistence does not help, and home-screen apps are exempt.** [V]
- Protocol §3.1 and W4c: "the page asks for persistent storage" as a remedy.
- WebKit says it grants `persist()` "based on heuristics like whether the website is opened as a Home
  Screen Web App". WebKit bug 209563 reports that ITP still deletes after 7 days with persistence
  granted.
- The working remedy is the one D290 §10 chose: *Add to Home Screen*. Asking for persistence is
  harmless, but it is no fix for Safari.

**F9. Spec §7.1's load figure is wrong.** "One heartbeat per core per 5 minutes is 0.3 writes per
second at 100,000 cores". It is 100,000 / 300 s ≈ **333 writes per second**. SQLite can still take
it [I], but the claim "load is tiny" needs restating.

**F10. Hetzner's price.** [V]
- Spec §1.4: "about €4.50–6.50 a month (Hetzner's smallest rose to €5.49 in June 2026)".
- Hetzner raised prices twice in 2026, on 1 April and 15 June. CX23 in Germany is €5.49 **excluding
  the IPv4 address** and VAT.
- The €4.50 lower bound is stale. **Fix:** "about €5.50 plus the IPv4 address".

**F11. The rate-limit increase form "takes several weeks"** (spec §3.11, C1). LE's page says "a few
weeks" [V]. The plan stands.

**F12. S12 misuses the PSL's "no way to expedite".** That sentence is about browsers picking up the
list once a pull request is merged, not about acceptance. Acceptance has no ETA either [V]. The
conclusion (no PSL entry) stands.

**F13. S16's reason for omitting `SOFTWARE` is wrong.**
- RFC 8489 §16.1.2 gives version disclosure as the reason, not response size. The base text says
  servers SHOULD add it, made configurable.
- The amplification sentence says only "a small increase in the amount of data" [V].
- The recommendation is fine. Fix its citation.

**F14. The availability research says serve-stale is "capped at 7 days".** RFC 8767 suggests a
maximum stale period of 1 to 3 days. Seven days is its cap on TTLs [V].

**F15. The correctness audit's line numbers cannot be right.** It cites `single/signal.go:624–630`
(121 lines), `internal/core/discovery/signal.go:615–664` (97 lines), `route.go:629–645` (59 lines)
and `pcp.go:533, 585` (217 lines), at an earlier commit as now [V, `git show an earlier commit:<file> | wc -l`]. The
behaviours they back were confirmed in the code this round (flat 30 s retry, one offer at a time,
`Promise.all`, no ping, socket kept after leave). The citations are not reliable.

**F16. Service workers re-check "at least every 24 hours" (S2) is imprecise.** The 24-hour rule
marks a registration stale and is triggered by navigations and events, not a timer. The main script
already bypasses the HTTP cache on every update check by default [V]. S2's conclusion (the server
can always replace the worker) stands.

**F17. Local Network Access: the details check out, with one omission.** [V]
- Chrome 142 for fetch and 147 for WebSocket. WebRTC is still "Proposed". `targetAddressSpace:
  "local"` is the current name ("private" was the old one). Service workers need the permission
  granted in a window first. Firefox 153 is right.
- Omitted: the spec classes `100.64.0.0/10` as **local**. A `configured` or `mapped` name at a CGN
  address would prompt too, which strengthens C7 and C9's classing.
- `Integrity-Policy` now ships in Chrome 138, Firefox 145 and Safari 26. S2 named Chrome alone.

**F18. The ACME library: the plan's [I] is half right.** [V, module source v0.57.0]
- There is no ARI.
- `OrderOption` is sealed (`privateOrderOpt`), so neither ARI's `replaces` nor an ACME profile (LE's
  `tlsserver`, `shortlived`) can be set.
- `Client.Accept` POSTs `{}` to any challenge URL, so answering a dns-persist-01 challenge needs
  nothing new. Draft-02's step of verifying a delegated account (a POST-as-GET with an empty payload)
  is already how the package fetches accounts.
- What persist adds is computing the record's value (F1) from `accountHashPrefix` in the
  directory's `meta`.

**Checked and correct** [V]:
- **Let's Encrypt:** 50 certificates per registered domain per 7 days, refilling every 202 minutes.
  Same-set renewals are exempt from that limit and from new orders. ARI renewals are "exempt from
  all rate limits". The lifetime timeline is right: 45-day `tlsserver` opt-in from 2026-05-13 (now
  live); classic at 64 days with 10-day authorisation reuse on 2027-02-10; 45 days with 7-hour reuse
  on 2028-02-16. LE validates from 1 primary and 4 remote perspectives.
- **Standards:** ARI is RFC 9773 (June 2025). LE honours RFC 8657's `accounturi` and
  `validationmethods` (since 2022-12). The PSL refuses entries made to get around rate limits. The
  RFC 8445 §9 and §19.5.1 quotes are right. RFC 8981's 1-day and 2-day lifetimes are right. The TURN
  REST username `<expiry>:<id>` is right.
- **NIST SP 800-63B-4:** 24 hours for issued recovery codes sent by e-mail; 30 days at AAL1; e-mail
  "SHALL NOT be used for out-of-band authentication"; account recovery "SHALL cause a
  notification".
- **Cloudflare:** free zones created on or after 2024-09-01 hold 200 records, Pro 3,500. Free has 1
  rate-limiting rule. Pages allows 20,000 files of 25 MiB. Spectrum UDP is an Enterprise add-on. DNS
  cannot compute answers (only Enterprise DNS Firewall in front of one's own servers).
- **Bunny DNS:** free for 500 zones, no query fees for standard or Smart (scripted) queries, a $1
  account minimum, 5,000 records per zone. A script gets the queried hostname and can return
  computed A and AAAA records.
- **Other services and browsers:** `.app`, `.dev` and `.page` are HSTS-preloaded. callstats' 22% is
  of conferences, not calls. Plex's plex.direct incidents (Jun 9, Jul 27, Sep 10, Oct 2 2026) and
  Hetzner's 2026-02-25 UDP over-blocking (1 h 32 min) are right. SFSafariViewController's separate
  storage since iOS 11 is right. Chrome hides host candidates behind mDNS unless camera or microphone
  permission was granted.
- **Code and libraries:** Go's `http.CrossOriginProtection` allows only `same-origin` and `none`
  [`$GOROOT/src/net/http/csrf.go`]. pion v4.2.23 restarts ICE on a remote offer with new
  credentials (`peerconnection.go:1378`) and offers `ICERestart`. pion/ice's STUN gather timeout is
  5 s.

## Gaps

**G1. Bunny's scripted DNS may not be able to do what D291 §2 asks.** [V docs; I design]
- The docs do not say a script record may sit at a wildcard (`*.<zone>` or `*.*.<zone>`), and they
  give no execution limits. Bunny "intentionally deviates from RFC 4592" for empty non-terminals.
- Nothing says how a script learns the live set of `coreId`s (100,000+), or released and disabled
  ones, to "answer only for core ids that exist".
- Nothing says how the stored `_acme-challenge`, `_validation-persist` and CAA records coexist
  under a scripted wildcard.
- "Returns once every authoritative server serves the record" is undefined on anycast.
- Hands-on test before the zone is registered (the plans' owner lane names only the first point).

**G2. A re-key or a restore can change the core's ACME account, and nothing updates CAA.**
- `acme-account-key.pem` sits beside the identity key. A core restored without its files, or a new
  machine, registers a new account.
- The CAA `accounturi` (and the persist record) then **refuse** its certificate until the discovery
  rewrites them.
- §2.8 and K8c must say the new account URI is reported and the records replaced with the re-key.

**G3. Long TTLs vs "names stop answering".** §2.7 wants TTLs of days. §2.8 says names stop answering
on removal, disabling and `released`. Resolvers keep a cached answer up to their cap: a day in
Unbound, a week in BIND [V, availability research]. Choose a TTL that bounds how long a disabled
core stays reachable, or say it.

**G4. Unproven WebRTC connections leave out Watch Together and device codes.** D291 §6 (S5 item 2)
limits them to "signing in, registering or pairing". A guest's first request is a join, and on an
own discovery a device with no link asks for a code (discovery-mode.md Q6). List join and code
requests, or a guest cannot get in over WebRTC.

**G5. The "at home" test misfires under shared IPv4.** §3.2 returns the `lan` path when the request's
source "equals the core's public IPv4". Under carrier NAT or DS-Lite, which is the owner's own kind,
that address is shared by many households. A neighbour then gets the LAN names and Chrome's prompt,
and S11's privacy concern returns. Use IPv4 equality only when the core's IPv4 was probed public
(not CGN), or prefer the IPv6 prefix.

**G6. Per-network code limits need the device's real address.** D288 §6 as amended and §3.1 key
on the requester's IPv4 or /64. On an own discovery the request may reach the core relayed, from the
VPS's address, so five waiting codes would refuse every device. Say what address the core sees on
each path.

**G7. No minimum browser is decided for paid device codes.** This follows from F7. X25519 needs
Safari 18.4+. Otherwise a JS X25519 library is needed, which is the owner's dependency call.

**G8. `stun.cloudflare.com` was taken but placed nowhere.** D291 §6 adopts spec §11.3 item 10, a
third STUN server. Protocol §2.9, W4b.3 and P8 omit it. The privacy page and spec §6's processor list
must name Cloudflare too. More generally, spec §6's "data-processing agreements with hosting, DNS,
e-mail and MoR" omits Cloudflare as proxy and Pages host, which sees every sign-in request.

**G9. No liveness rule for instances** (planner's Q4, confirmed). Without one, every crash-restart is
refused as "another server".

**G10. Two STUN libraries.** K14 and W4b.3 plan on `pion/stun/v3` (direct). `pion/webrtc/v4` already
brings `pion/stun/v4` [V, go.mod]. This is W6's "one library" argument again.

**G11. Plans duplicated by hand.** "The beta's order of work" appears word for word in
`discovery-core.md` and `service.md`. It will drift (rule 11). Keep it in one place.

**G12. A carrier-NAT IPv4 is never public, yet the probe may test it.** K1's classing helps
publishing, but S10's deny list for the probe and K1's function should be the same function (rule
11). Today they are two lists in two documents.

**G13. CSP and Cloudflare.** A strict `script-src 'self'` with SRI conflicts with any
Cloudflare-injected script (e.g. challenge pages, Web Analytics, email obfuscation) on proxied
hosts. Spec §7.3 should say these features stay off [I].

**G14. Missing banners in the decision log.**
- No "Revised by" or "Refined by" notes on: D72 (by D214, D289 §1); D267 §1 (by D290 §17); D273 §7
  (by D290 §2, D291 §3); D274 §2's "a high port of the core's choosing" (by D291 §7); D290 (by D291
  §2, §3, §5, §7); D243 §7 (the VPS route, by D273 and D274).
- Readers of the older decisions get superseded rules (X20, R5).

## Sources

All cached in `/yoloai/cache/discovery-audit2/` unless marked as code.

- **Let's Encrypt:**
  - Rate limits: https://letsencrypt.org/docs/rate-limits/ (`le-rate-limits.txt`)
  - "From 90 to 45": https://letsencrypt.org/2025/12/02/from-90-to-45 (`le-90-to-45.txt`)
  - DNS-PERSIST-01: https://letsencrypt.org/2026/02/18/dns-persist-01 (`le-dns-persist.txt`)
  - Deployment status: https://community.letsencrypt.org/t/246468
    (`lec-dns-persist-01-deployment-status-and-timeline.txt`)
  - CAA: https://letsencrypt.org/docs/caa/ (`le-caa.txt`)
  - CP/CPS: https://cps.letsencrypt.org/ (`le-cps.txt`)
  - CAA binding: https://community.letsencrypt.org/t/189588 (`lec-189588.txt`)
  - Perspectives: https://community.letsencrypt.org/t/214123 (`lec-214123.txt`)
  - tlsserver: https://community.letsencrypt.org/t/247049 (`lec-247049.txt`)
- **IETF and CA/B:**
  - draft-ietf-acme-dns-persist-02: https://datatracker.ietf.org/doc/draft-ietf-acme-dns-persist/
    (`draft-ietf-acme-dns-persist-02.txt`)
  - CA/B SC-088v3: https://cabforum.org/2025/10/09/ballot-sc-088v3-dns-txt-record-with-persistent-value-dcv-method/
  - RFC 9773, 8657, 8838, 8445, 5780, 4787, 8981, 8489, 8767 (`rfc*.txt`)
  - draft-uberti-behave-turn-rest-00
- **PSL:** https://github.com/publicsuffix/list/wiki/Guidelines and CONTRIBUTING.md
  (`psl-guidelines-raw.md`, `psl-CONTRIBUTING.md`).
- **Cloudflare:**
  - https://developers.cloudflare.com/dns/manage-dns-records/ (`cf-dns-manage.md`)
  - /waf/rate-limiting-rules/ (`cf-ratelimit.md`)
  - /pages/platform/limits/ (`cf-pages-limits.md`)
  - /spectrum/protocols-per-plan/ (`cf-spectrum-protocols.md`)
  - /dns/dns-firewall/ (`cf-dns-firewall.md`)
- **Bunny:**
  - https://bunny.net/pricing/dns/ (`bunny-dns-pricing.txt`)
  - /docs/dns/limits.md
  - /docs/dns/scriptable/introduction.md (`bunny-scriptable-intro.md`, `bunny-scriptable-types.md`)
  - /docs/dns/records (`bunny-dns-records.txt`)
- **Hetzner:**
  - https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/
    (`hetzner-price-adjustment.txt`)
  - Press statement, April 2026 (`hetzner-statement-april-2026.txt`)
  - Status incident an earlier commit
- **Registry:** registry.google .app/.dev/.page (`registry-google-*.txt`).
- **Browsers:**
  - Chrome LNA blog (`chrome-lna-blog-live.txt`), adoption guide (`lna-adoption-guide.txt`),
    chromestatus 5152728072060928 and 5065884686876672, WICG LNA spec (`lna-spec.txt`)
  - Firefox 153 and 154 release notes (`ff153rn.txt`, `ff154.txt`)
  - WebKit blog 10218, 14403, Safari 18.4 and 26 notes, tracking-prevention page, bug 209563
  - Chrome 138 notes; MDN Firefox 145 and 130 notes
  - Service Worker spec (`sw-spec.txt`); Fetch bad ports (`fetch-spec.html`)
  - Chromium `filtering_network_manager.cc`
- **NAT and measurements:**
  - arXiv 2510.27500 and 2604.12484; arXiv 1605.05606 (Richter et al.)
  - webrtcHacks usage stats
  - Tailscale NAT traversal, 2020 and part 1 (2025)
- **Incidents:**
  - NETSCOUT PMSSDP (`netscout-pmssdp.txt`); Plex forum 687162
  - Plex status feed (`plex-status-incidents.json`)
- **NIST:** SP 800-63B-4: https://pages.nist.gov/800-63-4/sp800-63b.html (`nist-63b4.txt`).
- **Code** (no cache):
  - `internal/core/backup/backup.go`, `internal/core/instancedir/archive.go`,
    `internal/core/identity/identity.go`, `internal/core/certificate/certificate.go`,
    `internal/core/portmap/portmap.go`, `internal/core/rtcapi/rtcapi.go`,
    `discovery/single/single.go`, `discovery/discoveryproto/discoveryproto.go`
  - `golang.org/x/crypto@v0.57.0/acme/{types.go,acme.go}`
  - `github.com/pion/webrtc/v4@v4.2.23/peerconnection.go`, `github.com/pion/ice/v4@v4.4.7/agent_config.go`
  - Go 1.26.2 `src/crypto/hpke`, `src/net/http/csrf.go`
