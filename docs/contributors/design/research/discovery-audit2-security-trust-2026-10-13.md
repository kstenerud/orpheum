> **ABOUTME:** Second-round security audit of the discovery (2026-10-13), lens: identity, keys and trust. It follows the trust chain from the core's identity key through claiming, sessions, re-keying, the web client, signalling, DNS and certificates, checked against RFCs, the ACME draft, browser data, vendor docs and the code. Findings T1–T14, then errors in our documents and the first audit.

# Discovery: second security audit — identity, keys and trust (2026-10-13)

**Scope.** `design/discovery-protocol.md` (the protocol), decisions D214, D273–D275, D288–D291 in
`decisions/working-notes.md`, the paid service's spec (shared files, draft 5), the plans
`discovery-core.md`, `discovery-mode.md`, `webrtc-transport.md` and the service's `service.md`, and
the code in `internal/core/identity`, `internal/core/discovery`, `internal/core/certificate`,
`internal/core/backup`, `internal/core/instancedir`, `discovery/` and `internal/core/rtcapi`, read
at an earlier commit. The first audit (`discovery-audit-security-2026-10-13.md`) was read for context only.
Nothing in it or in our documents was taken as true without a check.

**Markers.** **[V]** checked at the source named beside it: a document (cached under
`/yoloai/cache/discovery-audit2/`), a `dig` run on 2026-10-13, or the code at `file:line`. **[I]**
my inference. Severity is for the paid service unless the finding says otherwise.

## Summary

The trust chain has one strong link and several weak ones.

- **The strong link** is WebRTC: a browser enforces that the peer's DTLS certificate matches the
  fingerprint in the SDP it was given **[V, RFC 8842 §5]**, and pion makes a fresh certificate per
  connection **[V]**. So a fingerprint signed by the core's identity key is truly channel-bound.
  This holds only while the JavaScript that checks it is honest.
- **The identity key is not protected as the design says (T1).** Every backup, scheduled ones
  included, holds `identity.key`, the TLS key and the ACME account key. Only `secret.key` is left
  out. A thief of any backup can rotate the key "at once", silently, and clients accept it. The
  thief can then veto the owner's recovery. Our plan says the opposite of the code.
- **The CAA record leaves the namespace open (T2).** It sets only `issuewild`. RFC 8659 says
  `issuewild` is ignored for non-wildcard names. With computed answers, anyone can get a valid
  certificate for `<their-ip>.<victim coreId>.<zone>` by HTTP-01, from any CA, and burn the zone's
  weekly Let's Encrypt limit. `plex.direct` closes this with `issue ";"` **[V, dig]**.
- **A browser cannot pin a core's key over HTTPS (T3).** HPKP is gone **[V]**, and page script
  cannot see the TLS certificate. A signed nonce over HTTPS is relayable: anyone with a certificate
  for the name forwards the challenge to the real core. And D291 §6 turns an unsigned key change
  into a "notice" asserted by the discovery, which undoes the protection S1 asked for.
- **The paid service is a fully trusted party on the web, and our documents say otherwise
  (T4).** It serves the JavaScript, controls the DNS zone and its CAA records, and relays
  signalling. Through the JavaScript it reads every password and acts as every admin. The protocol
  says "no discovery can sit between a client and its core" and that a compromised discovery
  "opens no connection into the core". Neither is true against the service itself.
- **Account takeover still becomes household takeover (T5, T6).** A stolen mailbox runs the
  72-hour recovery, and every notice goes to that mailbox. The old core "may cancel", which needs a
  person watching a page. A stolen admin session can move the core to another account.
- **DNS-PERSIST-01 is not in production and its record format changed (T8).** Let's Encrypt said
  on 2026-06-25 it will not deploy it until a substitution attack is fixed; draft -02
  (2026-09-20) replaced the plain account URI with a hash **[V]**.
- **Bunny DNS has one account-wide API key (T9)**, so the "scoped token" in the plan cannot exist
  **[V]**.

The fixes are mostly small: leave the keys out of backups, add `issue ";"`, let a live old key veto
recovery automatically (as Apple does), never accept an unsigned key change on the discovery's
word, put fingerprints in links, keep admin powers off `app.`, and say plainly who is trusted.

## Findings

### T1 — critical — The identity key, TLS key and ACME account key are in every backup; a stolen key rotates silently and wins

**Where.** `internal/core/backup/backup.go:60` (`leftOut := []string{secrets.KeyFileName}`);
`internal/core/instancedir/archive.go:85-103` (`persistentFileNames`: every regular file but the
database and `leftOut`); `internal/core/identity/identity.go:19,96`;
`internal/core/certificate/certificate.go:36-40` (`tls-key.pem`, `acme-account-key.pem` in the
persistent directory). Protocol §2.1 (line 86-88), §2.8 (line 379-380); D289 §2; D291 §6;
`plans/discovery-core.md:35,375`; `plans/webrtc-transport.md` W2f ("a key signed by the old one is
taken quietly").

**What the design says.** The identity key is backed up "as `secret.key` is: out of scheduled
backups, in *Back up now* when its box is ticked" (D289 §2). The plan says "the identity key is not
in any backup". A rotation countersigned by the old key "takes effect at once". A client "accepts a
key the old key signed".

**What the code does [V].** `write` leaves out `secret.key` alone. `persistentFileNames` adds every
other regular file in the persistent directory. `identity.key`, `tls-key.pem` and
`acme-account-key.pem` all live there (`WritePersistentFile`). So every backup, scheduled or not,
holds all three in plain text. `backup/doc.go:29`'s invariant "a backup holds ciphertext only" is
false for them.

**The attack.** Anyone who reads one backup (a NAS share, an off-site sync, an old disk):

1. opens a session with the identity key and rotates to a key of their own, countersigned by the
   old one. It takes effect at once. Nothing says the customer is told [I: the protocol names no
   notice for a rotation];
2. reports a new ACME account; the service rewrites CAA and the persist record for it (§2.7). Or,
   with DNS-PERSIST-01, uses the stolen ACME account key directly: the persist record authorises
   that account until removed **[V, draft-ietf-acme-dns-persist-02 §4.2; LE blog: "protecting the
   ACME account key becomes the central concern"]**;
3. publishes its own addresses. Clients that remember the old key take the new one "quietly", and
   check its signed fingerprints. Every path, HTTPS and WebRTC, now ends at the thief;
4. the real core's session is refused. K8d tells its admin to run a recovery. The recovery waits
   72 hours, and "the old core may cancel it": the thief now holds the current key and a session,
   so the thief cancels. Only the operator can undo it [I, from §2.8].

The stolen TLS key alone also lets the thief impersonate the core's HTTPS names until the
certificate expires, if it can get traffic routed to it.

**Sources.** NIST and Tailscale both treat the signing key as the root: Tailnet Lock asks for at
least two signing nodes and disablement secrets kept apart **[V, tailscale.com/kb/1226]**.

**Fix.**
1. Leave `identity.key`, `acme-account-key.pem` and `tls-key.pem` out of scheduled backups, as
   `secret.key` is (one list, one test: a scheduled backup holds none of the four). In *Back up
   now* the box covers all of them. Fix `backup/doc.go`'s invariant.
2. A rotation is not silent. It mails the customer, shows on the dashboard, and is refused while
   another `instanceId` of the same key holds a live session, unless that instance asks for it.
3. On re-key (rotation or recovery) the core makes a new ACME account and **deactivates the old
   one**: draft -02 says deactivation stops every persist record of that account at once **[V,
   draft -02 §4.2 rule 1]**. The service rewrites CAA and the persist record to the new account.
   §2.8 today revokes only the old certificate.
4. K8d must tell "my key was replaced by someone else" (the old key file is still here and still
   refused) from "I lost my key" (no file). The first is an alarm, not a routine re-key.

### T2 — high — CAA sets only `issuewild`, so anyone can get certificates for names in any core's namespace

**Where.** Protocol §2.7 (line 354-357); spec §3.11 (line 339-341); first audit S12 item 1.

**What the design says.** `<coreId>.<zone> CAA 0 issuewild "letsencrypt.org; accounturi=…;
validationmethods=dns-01"`, "so only the core's own ACME account can get its certificate, whoever
can write a TXT record". The spec adds that "a stolen DNS token or a bug in the TXT proxy's name
check mints no certificate".

**What the standard says [V, RFC 8659 §4.3].** "Each issuewild Property MUST be ignored when
processing a request for an FQDN that is not a Wildcard Domain Name." And §4.2: if the relevant
RRset "contains no Property Tags that restrict issuance … CAA does not restrict issuance". The
relevant RRset is the first non-empty one climbing the tree (§4.2's `RelevantCAASet`), so for
`x.<coreId>.<zone>` it is the core's own record, and the zone apex's CAA is never read. Let's
Encrypt: "Records with the issue tag … control both normal and wildcard issuance in the absence
of any other records" **[V, letsencrypt.org/docs/caa]**.

**The attack.** `coreId`s are public: every wildcard certificate is in CT (§2.7 says so). Answers
are computed: `203-0-113-66.<victim coreId>.<zone>` resolves to `203.0.113.66` (§2.7, D290 §7;
`plex.direct` does the same **[V, dig: `1-2-3-4.<any hash>.plex.direct` → `1.2.3.4`]**). So
anyone can run HTTP-01 or TLS-ALPN-01 on their own address and get a publicly trusted certificate
for a name inside the victim's namespace, from Let's Encrypt or any other CA, since no `issue`
property restricts them [I]. Consequences:

- a valid certificate for a name that looks like the household's core, for phishing or for any
  path a client can be led to;
- the core's CT watch (T11) raises false alarms on demand;
- **the zone's issuance limit burns**: 50 new certificates per registered domain per week **[V,
  first audit; LE rate limits page, cached]**. An attacker issuing for computed names stops every
  new core from getting its certificate, every week, for free [I].

Before a core is claimed, or before it reports its account, it has no CAA record, and the apex's
record governs; that needs the same care.

**What Plex does [V, `dig CAA plex.direct`, 2026-10-13]:** `0 issue ";"`,
`0 issuewild "letsencrypt.org"`, `0 issuewild "digicert.com"`, `0 iodef "mailto:…"`. The
`issue ";"` forbids every non-wildcard certificate in the zone.

**Fix.** At the zone apex: `0 issue ";"` and `0 issuewild "letsencrypt.org;
validationmethods=dns-01"` (plus `iodef`). Per claimed core: `0 issue ";"` and `0 issuewild
"letsencrypt.org; accounturi=…; validationmethods=dns-01"`. Add a test vector for the records in
`discoveryproto`. Correct spec §3.11's claim (see T9 for the stolen-token half).

### T3 — high — The web client cannot pin a core's key over HTTPS; the decided design assumes it can, and D291 §6 weakens what is left

**Where.** Protocol §3.5 (line 583-587), §7 item 23 (line 752-754); D290 §3 (working-notes
line 19811); D291 §6 (line 19893); `plans/webrtc-transport.md` W2f; `plans/discovery-core.md` K8b
and question 14.

**What the design says.** "Over a direct path, it asks the core to sign a nonce with its identity
key … keeps the public key per `coreId`, and checks it on every later visit. A changed key stops
the client before it sends a password." After a recovery without the old key, "a plain notice
says the server's identity changed and when the owner confirmed it".

**What browsers can do [V].**
- HPKP was removed in Chrome 72 **[V, developer.chrome.com "Deprecations and removals in Chrome
  72"]**. No browser lets a page see or pin the TLS certificate of a `fetch`.
- WebRTC: the browser tears down a DTLS session whose certificate does not match the SDP
  fingerprint **[V, RFC 8842 §5]**, and page script can read the answer's SDP before
  `setRemoteDescription`. So a signature over the fingerprint is channel-bound.
- WebTransport's `serverCertificateHashes` makes the browser accept only a certificate with a given
  hash (ECDSA, valid under two weeks): Chrome 100, Firefox 125, Safari 26.4 **[V, MDN
  browser-compat-data; MDN WebTransport()]**. It is the only HTTPS-family channel a page can pin.
- RFC 8827's identity provider model (`peerIdentity`) exists in Firefox only **[V, MDN
  browser-compat-data]**, and does not protect data channels from the page (T4).

**The failures.**
1. **A signed nonce over HTTPS is relayable.** K8b is "an API endpoint, open to anyone". An
   attacker who holds a valid certificate for the core's name (T2, a recovery under T5, or zone
   control) receives the client's nonce, forwards it to the real core at its public address, and
   returns the real signature. The client is satisfied and sends the password. The check stops
   only an attacker who cannot reach the real core [I]. The first audit said this ("does not stop
   a full relay"); the protocol dropped the caveat.
2. **First contact is through the discovery** for WebRTC-only households: the key and the signed
   fingerprint both arrive through it (plan question 14). TOFU there protects nothing against the
   discovery.
3. **Safari erases the remembered key** after seven days without a visit **[V, WebKit ITP
   blog]**. An attacker can wait a week; the client then trusts on first use again, silently.
4. **D291 §6's "notice" for an unsigned change.** The client learns that a recovery happened, and
   its date, from the discovery. In the S1 attack the person who "confirmed" it is the attacker
   holding the customer's mailbox. If the notice does not block, the remembered key stops nothing
   after a recovery; if it blocks, the date adds nothing. W2f's text reads as non-blocking.
5. **Storage is per origin**: every household's remembered keys live in `app.`'s storage, writable
   by any script running there (T4).

**Fix.**
1. Say plainly in §3.5 what the remembered key does: it is enforced on WebRTC (signed
   fingerprints), and on HTTPS it only stops an attacker who cannot reach the real core.
2. **Put the identity key's fingerprint in every link the core makes** (`/register`, `/join`,
   `/s`, `/p`, after `#`). First contact then checks against a value from the household, not from
   the discovery, for every attacker but the one serving the JavaScript. This answers plan
   question 14.
3. **An unsigned key change blocks** until the person opens a link that carries the new
   fingerprint (their own link, re-issued by the core after the recovery). Never accept it on the
   discovery's word. Rewrite D291 §6's second half and W2f.
4. On HTTPS paths, prefer WebRTC (or later WebTransport with `serverCertificateHashes` signed by
   the identity key) when the client holds a remembered key and the HTTPS name's certificate is
   newer than the client last saw [I: an option to measure, not a decision].
5. Ask for persistent storage and offer *Add to Home Screen* (already decided), and keep the
   remembered key in the person's own link too, so a wiped store recovers from the link.

### T4 — high — The paid service is a fully trusted party on the web; the documents overclaim, and admin rights through `app.` widen it

**Where.** Protocol §1, §2.5 item 3-4 (line 236-245), §3.4 (line 573-576); D290 §4 and §6
(working-notes line 19814, 19820); spec §1.2 (line 128-131), §7.3; D274 §5 (own discovery serves
files taken from the core).

**What the documents say.** "No discovery can sit between a client and its core on this path."
"A bug in the discovery, or a compromised one, then opens no connection into the core." "The
service never sees their passwords or tokens." Nowhere does the protocol say the service is
trusted.

**What a malicious or compromised paid service can do [I, from the design; each step uses only
what the design gives it]:**

| Power | What it yields | What limits it |
| --- | --- | --- |
| Serves the web client (`app.`, from Cloudflare Pages) | reads every password and token typed into it; acts as any admin who uses it; skips every check of §3.5 and §2.5; reads data channels | nothing in the browser. SRI and `Integrity-Policy` stop a third-party host, not the origin **[V, first audit; MDN]**. Published build hashes help only someone who checks |
| Controls the zone (Bunny) | points names anywhere; rewrites CAA and persist records; gets certificates by DNS-01 | CT records the certificate (T11); the core can watch its own CAA (T9) |
| Relays signalling | drops or delays offers; knows every `locatorHash` | signed fingerprints, if the JS is honest |
| Holds the locator hashes | passes the core's own locator check (T7) | the core's sign-in |
| Runs recovery | binds any `coreId` to its own key with a database write; the 72 h is its own policy | only clients that block on an unsigned change (T3), and the old core's alarm |
| Sees addresses | every household's current home address and every device's | retention policy |

RFC 8827 says it directly: "Because sites can always tunnel data through the server, further
restrictions on the data channel do not provide any additional security" (§6.2), and
"a malicious signaling service can strip off any such identity assertions" (§9.1) **[V, RFC
8827]**. The parties inside this boundary include Cloudflare, which terminates TLS for `app.`,
`account.` and `api.` and serves the client's files [I: what proxying means], Bunny (the zone),
the registrar, and whoever holds deploy access to Pages.

**D290 §4 keeps admin rights for sessions made through `app.`.** The first audit's S2 item 1 asked
the opposite. So the System page's *Allow* for a claim (D290 §1), *Leave*, *Connect* and re-key
all run in the service's JavaScript. "The core's admin approves" is then not independent of the
service when the admin uses `app.` [I].

**How others handle it.**
- **Plex**: plex.tv serves the app, holds the accounts and gets every server's certificate through
  its DigiCert arrangement; it is trusted for everything **[V, Valsorda, "How Plex is doing
  HTTPS"]**.
- **Tailscale**: the coordination server is trusted by default; Tailnet Lock moves trust to
  signing nodes "even if Tailscale were malicious", but only for native clients, starting from
  TOFU, and its disablement needs secrets the server never holds **[V, kb/1226]**.
- **Apple**: HomeKit pairs locally by SRP with the accessory's code, and keys sync through
  end-to-end encrypted iCloud Keychain, so the cloud relays commands it cannot forge **[V, Apple
  Platform Security, HomeKit]**. iMessage Contact Key Verification adds key transparency because "if
  a powerful adversary were to compromise a key directory service, the service could start
  returning compromised keys" **[V, security.apple.com]**.

None of them protects a web client served by the coordinator from the coordinator.

**Fix.**
1. A short "Who is trusted" section in the protocol and the spec: on the web, the paid service and
   its providers are trusted by every household that uses `app.`; the measures stop outsiders and
   make a partial compromise visible; independence needs the core's own origin, an own discovery,
   or a native app.
2. Correct the overclaims (Errors E5–E7).
3. Revisit D290 §4: claim approval, *Leave*, *Connect*, re-key, users and library roots only from
   the core's own client at its own origin (on the LAN, or its names), as S2 item 1 proposed. A
   compromised `app.` then steals viewing, not the household.
4. Native apps are where trust can really move to the household (pinning, signed fingerprints, no
   per-load code from the service): plan them as the Tailnet Lock of this system.

### T5 — high — A stolen customer mailbox still becomes a household takeover; recovery's notices go to the attacker, and the old core must act by hand

**Where.** Protocol §2.8 (line 381-384); D290 §3; spec §2 ("Sign-in for customers", "Recovery"),
§8 (`POST /api/cores/{id}/recovery`); D291 §6 (step-up, protocol §7 item 4).

**What the design says.** Recovery takes effect after about 72 hours, "with mail to the customer
at the start and the end", a dashboard banner, and "while the old key still holds a session, the
old core may cancel it from its System page". Step-up before recovery. Sign-in links are tied to
the browser that asked.

**The attack [I].** With the mailbox:
1. the attacker asks for a sign-in link from their own browser and opens it there. The PKCE-style
   binding does nothing: the asking browser is theirs. Step-up is another mail to the same box
   (unless the customer has a passkey);
2. they start a recovery for the household's `coreId` with a key of their own, and delete the two
   mails (or set a filter). The banner is on a dashboard only they look at;
3. the old core shows the request on its System page. Nobody is looking. After 72 hours the
   `coreId` is theirs, and T3 item 4 lets remembered clients through with a notice.

**Races.** The account can also *remove* the core (`DELETE /api/cores/{id}`), which "invalidates
its tokens" (§2.8). If removal comes first, the old core has no session and cannot cancel; the
protocol does not say whether a removed core can be recovered. A core offline for 72 hours (a
power cut, a holiday) cannot cancel either.

**Is 72 hours enough?** Only with an automatic veto. Apple's recovery "might take several days or
longer", and "using an Apple device signed in to your account will cancel your account recovery
request" **[V, support.apple.com/118574]**: the veto needs no person. NIST requires a recovery
notification to the subscriber **[V, SP 800-63B-4]**, which here reaches the attacker.

**Fix.**
1. **Any authenticated contact from the old key during the wait cancels the recovery**,
   automatically, with an alarm on the core. A signed veto call works for a removed or frozen core
   too. Recovery succeeds only when the old key stays silent for the whole wait.
2. Longer wait (7 days) when the old key held a session in the last 30 days; 72 hours when it has
   been silent longer [I: the numbers are a proposal].
3. Notify through channels the mailbox thief does not hold: the old core raises the alarm to every
   admin session and to the household's clients ("your server's remote access is being moved"),
   and mails through the core's own mail settings if it has any.
4. Step-up by passkey where the customer has one; a mail step-up is no step-up against this
   attacker. Push passkeys after the first sign-in (spec §2 already plans it).

### T6 — medium — A stolen core admin session moves the core to another account

**Where.** Protocol §2.2, §2.8 (leave; "the same key reclaims it"); D290 §1; spec §3.11 (one core
per account; no rule for moving a core between accounts).

**The attack [I].** An admin bearer token (from XSS in `app.`, which renders data from any core a
person visits, Watch Together hosts included; or a stolen device) lets its holder: *Leave*; then
*Connect* again (enrolment is idempotent per key, and the same key reclaims its `coreId`); open
the claim page in their own browser with their own account; click *Allow* with the stolen
session. Their account now holds the household's `coreId`, and T5's recovery runs with mail going
to them. The household's account is never asked.

**Fix.** Moving a `coreId` from one account to another needs the holding account's consent or a
waiting period with mail to it. *Leave*, *Connect* and *Allow* only from the core's own origin
(T4 fix 3), with a password re-entry.

### T7 — medium — The core's locator check does not stop a compromised discovery

**Where.** Protocol §2.5 item 3 (line 236-239); D290 §6.

**What the design says.** The core matches `locatorHash` against the set it registered, so "a
bug in the discovery, or a compromised one, then opens no connection into the core".

**Why it fails [I].** The core registers those hashes with the discovery (`PUT /locators`,
`PUT /locate-hashes`). The discovery holds the whole set and can attach any of them to any offer.
The check stops a bug, not a compromise. For a WebRTC-only core this exposes its unauthenticated
endpoints (sign-in, registration, pairing) to the discovery.

**Fix.** The client proves the locator itself: it sends `HMAC(K, offer's DTLS fingerprint)` with
`K = SHA-256("orpheum-offer" ‖ locator)`, a value the core can compute (it issued the locator)
and the discovery cannot (it holds `SHA-256("orpheum-locate" ‖ …)`). Correct the sentence.

A related gap in the plan: S5's session proof is "an HMAC … keyed by the session's token"
(`discovery-core.md` K3), but the core keeps tokens hashed at rest (D154). Say which key the core
holds.

### T8 — medium — DNS-PERSIST-01 is not in production, its record format changed, and it moves the risk to the ACME account key

**Where.** Protocol §2.7 (line 341-346); spec §3.11, §10 risks; `plans/discovery-core.md` K9.

**Status [V].**
- Let's Encrypt announced it on 2026-02-18, staging late Q1, production "some time in Q2 2026"
  **[V, LE blog, cached]**.
- On 2026-06-25 Let's Encrypt's Aaron Gable wrote: "We will not be deploying dns-persist-01 until
  [issue #64] is resolved" **[V, LE community thread 246468]**. Issue #64 ("Record should include
  client-computed information") describes an account-substitution attack; it was closed on
  2026-09-23 **[V, GitHub API]**.
- `draft-ietf-acme-dns-persist-02` (2026-09-20, expires 2027-03-24) makes `accounturi` "a hashed
  URI that cryptographically binds the account to the domain", computed over the account key,
  and requires CAs to keep every key an account has used **[V, draft -02 §1, §4.1-4.2]**.
- A secondary source (CertKit, October 2026) says it is still not deployed and guesses mid-2027;
  Let's Encrypt's Boulder implemented `policy=wildcard` for `*.base` but not subdomain tree
  climbing **[secondary, certkit.io]**. For this design (one wildcard per core) that is enough.

**Consequences.**
1. The record in §2.7 (`accounturi=<the core's ACME account>`) is the draft-00 form and will not
   validate under -02. The hash needs the account key's thumbprint and the domain, so the **core**
   must compute it and the service write it. The draft asks that these values travel "over an
   authenticated channel" and that "domain owners SHOULD also publish a CAA accounturi" **[V,
   draft -02 §4.2]**: the core's session is that channel, and T2's CAA is that record.
2. A persist record is a standing authorisation for whoever holds the ACME account key, until
   `persistUntil` or deactivation **[V, LE blog; draft §4.2]**. That key is in every backup (T1).
3. Plan for DNS-01 by proxy as the method through at least the beta and the 45-day lifetimes; the
   TXT proxy stays on every renewal's path.

**Fix.** Mark §2.7's DNS-PERSIST-01 paragraph "not available; draft -02 format, computed by the
core", keep the ACME account key out of scheduled backups (T1), deactivate it on re-key, and
`persistUntil` set at claim to a bounded period (a year) renewed by the core, not left open.

### T9 — medium — The zone's master key cannot be scoped on Bunny, and nothing watches the CAA record

**Where.** D291 §2; spec §7.2 ("a token for the zone's API restricted to that zone"); `service.md`
P5 ("a scoped API token").

**Fact [V, docs.bunny.net/api-reference/authentication].** "You can have only one API key
associated with your account." It manages DNS zones, pull zones, storage, billing and the rest.
So the API host holds a key that can rewrite every core's CAA, persist record and the answering
script. Its theft is T4's zone row without the operator.

**Fix.**
1. Keep the Bunny key off the API host: a small writer on its own host that only sets
   `_acme-challenge.<coreId>` TXT (value checked, S18) and the validation records at claim, lapse
   and re-key, asked by the API host over an authenticated call. Correct spec §7.2 and P5.
2. **The core watches its own CAA and persist records** through public DNS, every few hours, and
   alarms when they stop naming its account. It needs no third party and warns *before* a
   certificate is issued, which CT cannot [I].
3. Registry lock and hardware-key 2FA on the Bunny account (already decided, D291 §6) matter more
   with one key.

### T10 — medium — Claim approval shows a masked account, and a leaked claim token can win the race

**Where.** Protocol §2.2 items 2-3; D290 §1; `service.md` P4 (typed code kept as fallback).

**The weakness [I].** The core's System page shows "which account asks to hold it, its address
masked". A mask like `k***@g***.com` matches the real customer and a look-alike the attacker
registered. The claim token lives in the URL fragment, so it is in the admin's browser history,
which browsers sync across devices, and visible to extensions. An attacker who gets it within its
few minutes confirms first; the admin, who just clicked *Connect*, sees a plausible masked address
and clicks *Allow*.

**Fix.** Bind the approval to the admin's own browser, as OAuth's `state` does: after the customer
confirms, `account.` redirects back to the System page with a one-time value after `#`, which the
service also hands the core in `GET /status`; the System page approves only a claim whose value
came back to it. Show the account's address in full to the core's admin (they are the customer).
A typed code, where kept, gets the same approval and a per-account attempt limit.

### T11 — medium — Watching Certificate Transparency is feasible but narrow, and it leaks

**Where.** Protocol §2.7 (line 367-368); D290 §3; plan K8e and question 16.

**Facts [V].** Let's Encrypt logs to its own static-CT (Sunlight) logs **[V, letsencrypt.org/docs/
ct-logs]**; reading logs directly is heavy. SSLMate's search API has a free tier of "100
single-hostname queries / hour, 10 full-domain queries / hour" and may change it "at any time
without notice" for free users, with an `include_subdomains` query **[V, sslmate.com]**. crt.sh is
the other free source [I: no checked limits].

**What it does and does not do [I].**
- It detects a certificate for `*.<coreId>.<zone>` or any name under it, after issuance (logs may
  lag up to their merge delay).
- It does not detect T4's script swap, the WebRTC path, or T2's certificates as an attack (they
  are expected noise until T2 is fixed).
- Each core querying SSLMate or crt.sh from home tells that party its `coreId` and its home
  address together, which the zone keeps apart elsewhere (spec §6).
- The alarm sits on a System page few people read.

**Fix.** The service monitors the whole zone with one paged query and passes alarms to cores
(cheap, but blind to the service's own misissuance); each core can also query on its own, off by
default, with the privacy cost said. Pair it with T9's CAA watch. Raise alarms to admin sessions
and household clients, not only the System page. Name the source in K8e.

### T12 — low — The signed fingerprint must cover the whole answer; a pairing QR cannot carry a DTLS fingerprint

**Where.** Protocol §2.5 item 4, §3.1 (pairing QR); plan W2f, question 14; first audit S6.

- An SDP may carry several `a=fingerprint` lines, at session and media level, and for several
  certificates **[V, RFC 8122 §5.1]**. pion reads the session-level line first, then a media
  line **[V, pion webrtc v4.2.23 `sdp.go:905-932`]**. If only one line is signed, a relay could add
  another. **Fix:** sign the exact answer SDP (or its hash) with `offerId`, and have the client
  pass that exact string to `setRemoteDescription`.
- pion makes a new ECDSA certificate for every `PeerConnection` when none is given **[V,
  `peerconnection.go:226-243`]**, as `rtcapi` does. So a DTLS fingerprint in a QR never matches a
  later answer. The QR must carry the **identity key's** fingerprint, which verifies the signed
  answer. Plan question 14 reads "as the pairing QR carries the DTLS one": correct it.

### T13 — low — An own discovery lets an unclaimed core get certificates under the owner's domain

**Where.** `discovery/single/single.go` `serveSignedIn`, `acmeChallenge` (func at line 375);
protocol §2.2 ("on the paid tier only a claimed core gets names and a certificate").

**Fact [V].** `acmeChallenge` checks only that the name is `_acme-challenge.<the held core's
id>.<zone>`; it does not check the core is claimed. Any key can enrol while no core is claimed
(`enrol`), open a session and set the TXT record through the owner's DNS provider. A stranger who
finds a fresh `orpheum discovery` before the owner's core is claimed gets a valid
`*.<id>.<owner's domain>` certificate [I: the window is short but real on a public VPS].

**Fix.** Refuse `PUT /acme-challenge` (and signalling) until the core is claimed, in `single` as on
the paid tier; have `orpheum discovery claim` print the fingerprint it binds.

### T14 — low — "One live instance per key" has no rule for when an instance dies, and favours whoever came first

**Where.** Protocol §2.3 (`instanceId`); plan question 4.

A crash leaves the old instance's token live, so the restarted core is refused as "another
server" (the plan asks this). With a stolen key (T1), the thief connects while the real core is
down and holds the slot. **Fix:** an instance is live while its signalling connection is open and
for at most two missed heartbeats after; a refusal names the other instance's first-seen time and
source network to both System pages, so the admin can tell a restart from a thief.

## Errors in our documents or the first audit

- **E1.** `plans/discovery-core.md:375` "The identity key is not in any backup: `internal/core/backup`
  names `secret.key` alone", and line 35 "holds `secret.key` alone today". The reverse is true:
  the backup leaves out `secret.key` alone and holds `identity.key`, `tls-key.pem` and
  `acme-account-key.pem` in every backup **[V, backup.go:60, archive.go:85-103]**.
  `internal/core/backup/doc.go:29` ("a backup holds ciphertext only") is false for those files.
- **E2.** Protocol §2.1 states D289 §2's rule as built ("out of scheduled backups") without a
  "the code must change" marker. It is not built (T1).
- **E3.** Protocol §2.7 and spec §3.11: the CAA record sets only `issuewild`, which does not
  restrict non-wildcard names **[V, RFC 8659 §4.3]**. "A stolen DNS token or a bug in the TXT
  proxy's name check mints no certificate" is false: the token can rewrite CAA (T9), and a
  name-check bug can mint a non-wildcard certificate (T2). The first audit's S12 item 1 has the
  same gap.
- **E4.** Protocol §2.7's persist record uses a plain account URI; draft -02 replaced it with a
  hashed URI computed over the account key **[V]**. "Whether Let's Encrypt runs it in production
  yet is unconfirmed": Let's Encrypt said it would not deploy it until issue #64 was resolved
  **[V, 2026-06-25]**; no production deployment was found [secondary, October 2026].
- **E5.** Protocol §2.5 item 4 and D290 §6: "no discovery can sit between a client and its core on
  this path". True for a signalling-only compromise with an honest client; false for the party that
  serves the client **[V, RFC 8827 §9.1, §6.2]**. The first audit's S6 said so
  correctly; the decision dropped the caveat.
- **E6.** Protocol §2.5 item 3: "a compromised one then opens no connection into the core". The
  discovery holds every hash the core checks (T7).
- **E7.** Spec §1.2: "the service never sees their passwords or tokens". It serves the code that
  handles them (T4).
- **E8.** Spec §7.2 and `service.md` P5: "a token … restricted to that zone", "a scoped API token".
  Bunny allows one account-wide API key **[V]**.
- **E9.** Protocol §3.5: "A changed key stops the client before it sends a password". Not over
  HTTPS against an attacker who relays the challenge (T3).
- **E10.** `plans/discovery-core.md` question 14: "as the pairing QR carries the DTLS one". A DTLS
  fingerprint cannot be carried ahead: pion makes a certificate per connection **[V]** (T12).
- **E11.** First audit, "What is sound": nonces "spent even when verification fails". Since
  An earlier commit, `single.isSigned` verifies first and spends only on a valid signature **[V,
  single.go `isSigned`]**. Harmless, but no longer true.
- **E12.** First audit S1 recommended "while the old key still holds a session, recovery is
  refused"; the protocol took the weaker "the old core may cancel it" (T5).

## What is sound (verified)

- **The identity key's making and keeping.** Ed25519 from `crypto/rand` (`cmd/orpheum/discovery.go:37`);
  written atomically, mode 0600 (`instancedir.WritePersistentFile`); a file that is not a key is
  refused, never replaced (`identity.go` `LoadOrMake`, `ParseKeyFile`) **[V]**. Only its backup
  handling is wrong (T1).
- **Purpose separation.** `proof.Message` is `orpheum-discovery-v1:<purpose>:<nonce>` with the
  purpose fixed per call, so one purpose's signature fails another's check **[V, proof.go]**. The
  planned binding to the discovery's origin (C8) closes cross-discovery relay.
- **Stateless nonces in `single`.** Expiry and HMAC in the nonce, spent once on a valid signature
  **[V, single.go `challenge`, `isSigned`]**.
- **One core per own discovery.** `enrol` refuses another key once a core is claimed **[V]**; the
  TXT name is checked exactly **[V]** (but see T13).
- **The core reaches its discovery over HTTPS only** (loopback excepted), with the standard
  library's certificate checks **[V, `checkBaseURL`]**.
- **No long-lived DTLS key.** pion makes a fresh certificate per connection **[V]**, and the
  browser enforces the fingerprint **[V, RFC 8842 §5]**. A signed fingerprint is a sound design
  for WebRTC with an honest client.
- **Claiming from the core with the admin's approval** stops an outsider who never sees the token;
  the PKCE-bound sign-in link stops a forwarded or leaked link (not a stolen mailbox, T5).
- **A recovery wait with notification** has the right shape: Apple's fixed wait, NIST's
  notification **[V]**; it needs the automatic veto (T5).
- **CAA `accounturi` and `validationmethods` are supported by Let's Encrypt** **[V,
  letsencrypt.org/docs/caa]**; the record only needs `issue ";"` beside it (T2).
- **HPKE-sealed device sessions** keep the session from a service that only relays, and the
  `TestArch_` claim is the right enforcement (protocol §3.1), within T4's limit.

## Sources

Cached under `/yoloai/cache/discovery-audit2/` unless marked (old) for `/yoloai/cache/discovery-audit/`.

Standards and drafts:
- RFC 8659, CAA (§4.2, §4.3): https://www.rfc-editor.org/rfc/rfc8659
- RFC 8657, CAA `accounturi`, `validationmethods` (old): https://www.rfc-editor.org/rfc/rfc8657
- RFC 8827, WebRTC Security Architecture, §9.1 and §6.2 on data channels (old): https://www.rfc-editor.org/rfc/rfc8827
- RFC 8842, DTLS in SDP offer/answer, §5: https://www.rfc-editor.org/rfc/rfc8842
- RFC 8122, SDP fingerprints, §5.1: https://www.rfc-editor.org/rfc/rfc8122
- draft-ietf-acme-dns-persist-02 (2026-09-20): https://www.ietf.org/archive/id/draft-ietf-acme-dns-persist-02.txt ; datatracker: https://datatracker.ietf.org/doc/draft-ietf-acme-dns-persist/
- Issue #64, "Record should include client-computed information" (closed 2026-09-23): https://github.com/ietf-wg-acme/draft-ietf-acme-dns-persist/issues/64
- NIST SP 800-63B-4, account recovery notification (old): https://pages.nist.gov/800-63-4/sp800-63b.html

Let's Encrypt and CT:
- DNS-PERSIST-01 announcement, 2026-02-18 (old): https://letsencrypt.org/2026/02/18/dns-persist-01.html
- Community, "Dns-persist-01 deployment status and timeline" (Aaron Gable, 2026-06-25): https://community.letsencrypt.org/t/dns-persist-01-deployment-status-and-timeline/246468
- CertKit, "What's taking DNS-PERSIST-01 so long?" (October 2026, secondary): https://www.certkit.io/blog/whats-taking-dns-persist-01-so-long
- CAA: https://letsencrypt.org/docs/caa/
- CT logs: https://letsencrypt.org/docs/ct-logs/
- SSLMate Certificate Search API and pricing: https://sslmate.com/help/reference/ct_search_api_v1 , https://sslmate.com/pricing/ct_search_api
- `dig CAA plex.direct` and a computed `plex.direct` answer, 2026-10-13: `dig-2026-10-13.txt`

Browsers:
- Chrome 72 deprecations and removals (HPKP): https://developer.chrome.com/blog/chrome-72-deps-rems
- MDN, `WebTransport()` and `serverCertificateHashes`: https://developer.mozilla.org/en-US/docs/Web/API/WebTransport/WebTransport
- MDN browser-compat-data (`peerIdentity`, `serverCertificateHashes`, `getRemoteCertificates`): https://github.com/mdn/browser-compat-data
- WebKit, ITP's seven-day cap on script-writable storage (old): https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/

Comparisons:
- Tailscale, Tailnet Lock: https://tailscale.com/kb/1226/tailnet-lock
- Filippo Valsorda, How Plex is doing HTTPS for all its users: https://words.filippo.io/how-plex-is-doing-https-for-all-its-users/
- Apple, iMessage Contact Key Verification: https://security.apple.com/blog/imessage-contact-key-verification/
- Apple Platform Security, HomeKit communication security: https://support.apple.com/guide/security/homekit-communication-security-sec3a881ccb1/web
- Apple, account recovery wait: https://support.apple.com/en-us/118574

Providers:
- Bunny, API authentication ("only one API key"): https://docs.bunny.net/api-reference/authentication

Code (Orpheum at `4bacd00d`; pion from the module cache):
- `internal/core/backup/backup.go:60`, `internal/core/backup/doc.go:29`, `internal/core/instancedir/archive.go:85-103`
- `internal/core/identity/identity.go`, `discovery/proof/proof.go`, `cmd/orpheum/discovery.go:37`
- `internal/core/certificate/certificate.go:36-40`
- `internal/core/discovery/discovery.go` (`checkBaseURL`), `internal/core/discovery/signal.go`
- `discovery/single/single.go` (`serve`, `serveSignedIn`, `isSigned`, `enrol`, `acmeChallenge`)
- `github.com/pion/webrtc/v4@v4.2.23/peerconnection.go:226-243`, `sdp.go:905-932`
