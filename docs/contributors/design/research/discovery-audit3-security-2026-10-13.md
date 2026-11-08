> **ABOUTME:** The third and final security audit of the discovery design (2026-10-13), done before
> building starts: a threat model, then findings against the public protocol, the core and the
> client, most severe first, each verified at its source (RFCs, CA/B Forum and Let's Encrypt,
> browser specifications and code, pion's source, `dig`, and our code at `d9779c6e`), and the
> decisions the owner must make. The paid service's own findings are in its repository.

# Discovery: third security audit, the protocol and the core (2026-10-13)

**Scope.** [discovery-protocol.md](../discovery-protocol.md) ("the protocol"); the plans
[discovery-core.md](../plans/discovery-core.md), [discovery-mode.md](../plans/discovery-mode.md),
[webrtc-transport.md](../plans/webrtc-transport.md); D214, D218, D221, D273, D274, D288–D291; the
two earlier rounds (`discovery-audit-*` and `discovery-audit2-*` of 2026-10-13, and the paid
service's `audit2-security-infra`); and the code at an earlier commit: `discovery/` (discoveryproto, proof,
safe32code, single), `internal/core/auth/`, `internal/core/api/` (pairing, idempotency, session),
`internal/core/discovery/`, `internal/core/rtcapi/`, `internal/core/webclient/`,
`internal/core/certificate/`, `cmd/orpheum/certificates.go`, `internal/deployment/templates/`,
`deploy/vps/`, `web/apps/web/src/session.ts`.

**What this round is.** The second round reversed parts of the first, so neither was trusted. Every
load-bearing claim below was checked again at its primary source or in the code. Where the two
earlier rounds were right, this report says so in a line and cites the check; where they were wrong
or incomplete, it says what is true. Findings about the paid service itself (its providers,
accounts, operations) are in the service's repository:
`orpheum-discovery/docs/research/audit3-security-2026-10-13.md` ("the service report").

**Markers.** **[V]** checked at the source named, fetched 2026-10-13 and cached under
`/yoloai/cache/discovery-audit3/` (subdirectories `acme/`, `infra/`, `browser/`, `webrtc/`, `dns/`,
`self/`; older copies reused from `discovery-audit2/` are named so), or in code at `file:line`.
**[I]** my inference. Severity: **critical** = a single compromise or a stranger takes a household
(its passwords, its admin, or its core's identity) or every household; **high** = the same with a
precondition that is realistic, or a whole class of households exposed; **medium** = a bounded
harm, or one that needs an unusual position; **low** = hardening.

## Summary

The design is sound in its strongest part (WebRTC whose DTLS fingerprint the core signs, checked by
an honest client) and in many details the earlier rounds got right. It still has five things that
must change before the beta, and the earlier rounds missed or mis-fixed three of them:

1. **The fix both earlier rounds proposed for the central client's admin power does not work.**
   "Admin only from the core's own origin" is defeated by anyone who learns the admin's password
   through `app.`: the `Origin` header is set by browsers, not by `curl`. What limits a compromised
   `app.` is a rule on the *credential*: a session minted through a discovery's client carries no
   admin, and an admin session needs a factor `app.` can never see (A1).
2. **Today every offer that reaches the core through a discovery is answered with no check at
   all**, by a WebRTC stack with none of RFC 8445's limits switched on. With WebRTC in the beta,
   that is an unauthenticated door into every core's API and a way to make it send packets into its
   own LAN (A2). And the planned locator binding is unsound as written because pion believes a
   session-level fingerprint over the one the browser signed (A9).
3. **`discovery/single`, the reference implementation, still lets a stranger take the core's
   signalling and certificates** (the second round's K13–K15, re-checked: unfixed) (A3).
4. **The certificate namespace is open**: CAA with `issuewild` alone restricts nothing for the
   names every core answers, and on the legacy path today the VPS (or whoever next holds its
   address) can get a certificate for `media.orpheum-app.com`, since that zone has no CAA at all
   (A4, A5).
5. **A stolen customer mailbox still becomes a household takeover**, and the fix for it is also the
   fix for the three-day outage after a restore: a recovery code made at claiming (A6).

The owner's twelve reopened questions each end in a verified finding and a recommendation; the
cross-reference is the table under *The reopened items*.

## Threat model

### Assets

| Asset | Where it lives | What its loss means |
| --- | --- | --- |
| Household passwords and session tokens | the core (hashed); typed into and held by the web client (`localStorage`, `session.ts:30`) | viewing, and with an admin's, the household |
| Admin authority on a core | a session whose user has `administer` | library roots, users, settings, keys, discovery connect/leave/claim approval |
| The core's identity key (`identity.key`) | the core's persistent directory, 0600; out of backups since an earlier commit | impersonating the core to the discovery and to clients that pinned it; re-keying |
| The core's TLS key and ACME account key | the core's persistent directory | serving the core's names; renewing as the core |
| The `coreId` binding (which key and which account hold a core) | the discovery's database | moving a household's links and names to someone else |
| Link secrets, locators, bootstrap tokens | the core (secrets), the discovery (hashes), the client | the home's current addresses, and the core's unauthenticated surface |
| The zone (names, CAA, TXT) | the DNS provider | certificates for every core's names |
| The web client's code at `app.` | the build, Cloudflare Pages, the service worker | everything typed into it, for every household |
| Home addresses and connection metadata | the discovery, Cloudflare, the probe, resolvers | privacy; targeting |
| Availability: DNS, signalling, certificate renewal | the service and its providers | remote reach for every household |

### Attackers

| Attacker | Starts with | Wants | Limited by (as designed) |
| --- | --- | --- | --- |
| **Compromised paid service** (operator, insider, a stolen provider login, a malicious build or npm dependency) | the JavaScript at `app.`, the zone, signalling, the `coreId` bindings, recovery | passwords, sessions, admin, silent persistence | nothing in the browser (A1, A7); CT and CAA watching detect certificate issuance; native apps and an own discovery are outside its reach |
| **Malicious household admin** (a paying customer, or anyone who installs a core) | a claimed core, a wildcard certificate under the zone, a signalling connection, the probe | other households' sessions (their members visit as Watch Together guests), the service's quotas, a scanner for hire | per-core namespaces; rate limits (several unbuilt); the guest's client renders their data |
| **Network attacker** (on-path Wi-Fi, a hostile LAN, a DNS spoofer, an internet scanner) | position | passwords, the core's address list, reflection | TLS with publicly trusted certificates, CAA (A4), DNSSEC (unsettled), SNI (A10) |
| **Malicious guest or link holder** (an ex-member, a forwarded Watch Together link, a phisher) | a link, a locator or a device code | the home's address, the core's unauthenticated endpoints, a household session through a code | single-use and revocable links, sign-in limits, the check code, device rights (A8, A11) |
| **Compromised VPS** (the legacy path, today) | the VPS's address, its WireGuard key, its root | a certificate for `media.orpheum-app.com`, spoofed client addresses, TURN abuse | today nothing for certificates (A5) |
| **Thief of a backup or a disk** | the core's files | the identity, the household | since an earlier commit, keys leave backups unless asked (verified, A6) |
| **Stolen customer mailbox** | the paid account | a household's `coreId` | a 72-hour wait whose notices go to the same mailbox (A6) |

### Trust, stated plainly

On the web, **the party that serves the JavaScript is trusted by every household that uses it**.
RFC 8826 §4.3: "the calling service (i.e., the Web server) controls not only the channel between the
communicating endpoints but also the application running on the user's browser … it is extremely
difficult to prevent a trusted but malicious calling service from actively attacking a user's
calls" **[V, `webrtc/rfc8826.txt`]**; RFC 8827 §6.2: "Because sites can always tunnel data through
the server, further restrictions on the data channel do not provide any additional security" **[V,
`discovery-audit/rfc8827.txt`]**. The paid service also holds the zone and so can get certificates
for any core's names by DNS-01. Within that trust, the design can still do three things, and every
recommendation below is one of them: **keep outsiders out**, **make a compromise visible** (CT,
CAA, signed keys), and **cap what one compromise yields** (no admin through `app.`, credentials per
core, a recovery the old key can veto). Independence from the service needs the core's own origin
with the core's own domain, an own discovery, or a native app. The protocol's §1, §2.5 and §3.5 and
the spec's §1.2 still say otherwise in places (the second round's E5–E7, W2: confirmed); they should
carry a short "Who is trusted" section.

## Findings

### A1 — critical — Admin power through `app.`: the decided remedy is missing, and the proposed one does not work

**Claim.** D290 §4: "sessions made through it keep their admin rights for now"; protocol §3.4
(line 573–576). The first round's S2 item 1, the second round's T4 fix 3 and W2 fix 1 propose
instead that admin actions be allowed only from the core's own client at its own origin.

**Verified.**
- As built every session carries the user's whole role: `issueSession` writes
  `Scope: grantScopeRole` (`internal/core/auth/auth.go:386`), for sign-ins and pairings alike.
- `Origin` is a *forbidden request-header* only for browsers' own fetches **[V, Fetch standard,
  `browser/fetch-spec.txt:1084`]**. A program (`curl`, a script) sends any `Origin` it likes, or
  none.
- Over WebRTC the core takes the request's head, `Origin` and `Host` included, from JSON the page
  wrote: `rtcapi` copies every client-supplied header into the request
  (`internal/core/rtcapi/channel.go:185-187`). Page script can therefore claim any origin there.
- The core's own client is reachable from the internet at its names (IPv6, mapped IPv4), so "the
  core's own origin" is not "at home".

**Attack.** A compromised `app.` (a malicious build, a stolen Pages token, an npm dependency, the
operator) captures the admin's password as it is typed (S2). The attacker then signs in with it from
anywhere, sending `Origin: https://<the core's own name>` or none, and receives a full admin session.
The origin rule never applies. Even without the password, page script that holds an admin session
acts as admin within the page's life (W2), and over WebRTC it can forge the head's `Origin`.

**Fix (two parts; both are needed).**
1. **A session minted through a discovery's client carries no admin.** "Through a discovery's
   client" is decided by what page script cannot forge: the browser's `Origin` on a direct HTTPS
   sign-in, and *every* sign-in or pairing that arrives over `rtcapi` (whose headers the core must
   treat as untrusted, stripping `Origin`, `Host`, `Forwarded`, `X-Forwarded-*` before the
   handler). The grant gets a scope narrower than `role` (the same scope A8 defines).
2. **An admin session needs a factor `app.` never sees.** Two workable forms:
   - **for the beta: admin sessions only from the core's own origin on a local network**, judged by
     the TCP source address against the prefixes of the core's own interfaces (never a bare
     "private address": the VPS tunnel's `10.77.0.1` is private, a container bridge's gateway is
     private, and a compromised VPS can inject any source, A5). Where the core cannot see real
     source addresses (a container proxy that rewrites them), it offers no remote-admin shortcut and
     says so;
   - **after the beta: a passkey (WebAuthn) whose RP ID is the core's own domain**, which a page at
     `app.` cannot exercise. This depends on A4 being fixed, since anyone holding a certificate for
     a name under the core's label could otherwise host a page that can.

   Admin pages on `app.` then say "manage this server from home, at <its own address>".
3. Keep the second round's other points: claim approval, *Leave*, *Connect*, re-key, users,
   library roots and keys are admin actions; destructive ones ask for the password again (on the
   admin origin), as K6 needs for T10.

**Owner.** This replaces D290 §4's "for now". It costs remote administration through `app.` in the
beta. Recommendation: take it; the household that administers away from home is the one whose
password a compromised `app.` would take.

### A2 — critical — Offers through a discovery are answered with no check, by a WebRTC stack with no limits

**Claim.** Protocol §2.5 items 3–4 and bounds (lines 236–264): the core checks each offer's locator,
bounds offers, closes idle connections; D290 §6.

**Verified.**
- The core answers every offer the discovery passes, unconditionally:
  `service.parts.Answerer.Answer(ctx, rtcapi.Description{Type: "offer", SDP: offer.SDP})`
  (`internal/core/discovery/signal.go:84-87`), one at a time. There is no locator, no session proof,
  no per-source limit; `rtcapi/doc.go`'s "answers only a signed-in client" holds only for its own
  `POST /rtc/offer` (the second round's K16: confirmed).
- `rtcapi.New` sets only `SetSCTPCwndCAStep` (`internal/core/rtcapi/rtcapi.go:71-72`). pion's
  defaults then accept **any number of remote candidates** with **no address filter**
  (`ice/v4@v4.4.7/agent.go:1295-1340`, filter nil at `:1370-1373`), and check every pair every
  200 ms with no pacing and no cap of 100 **[V, `webrtc/` report; `agent.go:795-815`,
  `agent_config.go:18`]**. RFC 8445 §19.5.1: agents "SHOULD limit the total number of connectivity
  checks they perform to 100" **[V]**. pion's SCTP checks message size only on send; inbound is
  bounded by a 1 MiB receive window and a read buffer that grows to 1 GiB
  (`sctp@v1.12.0/stream.go:314-316`; `webrtc/v4@v4.2.23/constants.go:28`) **[V]**.

**Attack.** Once the core holds a signalling connection (claimed, today with `single`, in the beta
with the service), anyone who can make the discovery forward an offer (as built `single` forwards
any, and the paid service is only "the first filter") gets: a connection into the core's whole API
over data channels, including sign-in, registration and pairing; 64 connections held until no
family member connects; and, by listing 50 remote candidates at LAN addresses and ports, the core
sending STUN checks into its own LAN every 200 ms. RFC 8826 §4.2 treats exactly this as the reason
for ICE consent **[V]**; consent stops the *data*, not the checks.

**Fix (K3, before any core holds a signalling connection to a paid discovery).**
1. The core refuses an offer that carries no proof it can check itself (A9's HMAC), not merely an
   unknown `locatorHash`.
2. `SettingEngine.SetRemoteIPFilter` refusing loopback, link-local, the core's own LAN prefixes and
   private ranges for offers through a discovery (the LAN path does not need ICE through it); at
   most ~10 remote candidates (D291 §6 item 11, now a must); a check budget.
3. `SetSCTPMaxReceiveBufferSize` and an application message cap; per-connection channel and
   in-flight caps; a deadline on a request's head and body (K16's list).
4. Per-source and per-locator connection caps on the core, not only at the discovery; slots counted
   on ICE connect; unproven connections short-lived and limited to sign-in, registration,
   pairing **and** Watch Together join and device codes (the consistency round's G4: confirmed, the
   list must include them).
5. A `TestArch_` claim that an offer through `internal/core/discovery` reaches `rtcapi` only after the
   core's own check.

### A3 — critical (before any deployment) — `discovery/single` still lets a stranger take its core's signalling and certificates

**Claim.** The plan's W4b.7 fixes these; `single` is "the reference" the paid service is tested
against (discovery-mode.md).

**Verified unfixed at an earlier commit.**
- Tokens are a map of token → expiry, bound to no core: `isSignedIn` checks only presence and expiry
  (`discovery/single/single.go:306-316`), so a token from a session opened while a stranger's key was
  the pending core stays valid after the owner's core replaces it.
- `enrol` replaces a pending core with any key that signs (`single.go:243`, the `default` arm).
- `acmeChallenge` checks only the name, not that the core is claimed (`single.go:375-392`).
- The signalling `accept` replaces the current connection for any valid token
  (`single/signal.go:47-61`).
- `coreIDChars = 8` (`single.go:49`): 40 bits.

**Attack (the second round's probes, re-read against the unchanged code).** A stranger enrols first
or while the owner's core is pending, keeps the token, and after the owner claims: takes the
signalling connection (every client's WebRTC now ends at the stranger, who reads each request's
bearer token), sets `_acme-challenge.<owner's core>` and gets its certificate, overwrites its
addresses, or deletes it. A pending owner's core can also be shown the stranger's claim code.

**Fix.** W4b.7 as planned, plus: bind every token to `(coreId, public key, instanceId)` and drop all
tokens when the record's core changes; refuse `acme-challenge`, `addresses` and `signal` until
claimed; let a pending core be replaced only after its code expires; 130-bit ids. The paid service
must not copy `single`'s session handling (the service report, B4).

### A4 — high — The certificate namespace is open, and the per-core record design meets DNS wildcard rules

**Claim.** Protocol §2.7 (lines 332–337, 354–357): answers computed for any address under an
existing `coreId`; per core `CAA 0 issuewild "letsencrypt.org; accounturi=…;
validationmethods=dns-01"`, "so only the core's own ACME account can get its certificate".

**Verified.**
- RFC 8659 §4.3: "Each issuewild Property MUST be ignored when processing a request for an FQDN that
  is not a Wildcard Domain Name"; §3: a relevant RRset with no restricting tags "does not restrict
  issuance"; the search stops at the first non-empty RRset climbing the tree, and for `*.X` starts
  at X **[V, `acme/rfc8659.txt`]**. Let's Encrypt's validator does the same (`boulder` `validateCAA`,
  `acme/boulder-va-caa.go@f877538d.go`) **[V]**.
- Core ids are in CT (every wildcard certificate names one), so any `<ip>.<coreId>.<zone>` resolves
  to an address the asker chooses.
- `plex.direct`, the model, publishes at its apex `0 issue ";"`, `issuewild "letsencrypt.org"`,
  `issuewild "digicert.com"`, `iodef`, and no CAA at the per-server label; a computed name
  `1-2-3-4.<any hash>.plex.direct` answers `1.2.3.4` **[V, `dns/dig-2026-10-13-audit3.txt`]**.
- RFC 4592: a wildcard `*.zone` synthesises nothing below a name that exists ("`host.subdel.example`
  is not synthesized because `subdel.example.` exists") **[V, `acme/rfc4592.txt` §2.2.1, §3.3.1]**. A
  CAA record at `<coreId>.zone`, and even the `_acme-challenge.<coreId>.zone` TXT record (which makes
  `<coreId>.zone` an empty non-terminal), would stop a zone-wide wildcard script from answering
  that core's names under strict RFC behaviour. Bunny documents that it deviates for empty
  non-terminals ("queries for an ENT receive the wildcard record") but not what happens below a
  node that holds records **[V, `infra/bunny-dns-records.md`]**.
- Bunny scripts can return A, AAAA, CNAME, TXT (and pull-zone) answers, never CAA **[V,
  `infra/bunny-dns-scriptable-query-response-object-types.md`, the CLI's `index.d.ts`]**. So any
  per-core CAA is a stored record, i.e. a node.
- The CA/B Baseline Requirements make `accounturi` and `validationmethods` a SHOULD now and a
  **MUST from 2027-03-15** (BR 2.3.1 §4.2.2.1.2, ballot SC098) **[V, `acme/cabf-br.txt`]**; and a CAA
  lookup that fails DNSSEC validation "MUST NOT be treated as permission to issue" **[V, BR
  §4.2.2.2.5]**.

**Attack.** Anyone points `6-6-6-6.<victim coreId>.<zone>` at their own server and gets a valid
certificate by HTTP-01 from any CA: phishing under the zone, CT alarms on demand, and the zone's
**50 new certificates per registered domain per week** burnt for every new household **[V,
`acme/le-rate-limits.txt`]** (the second round's T2/I3: confirmed).

**Fix (record set; a `discoveryproto` test vector for it).**
- **Apex:** `0 issue ";"`, `0 issuewild "letsencrypt.org; validationmethods=dns-01"`,
  `0 iodef "mailto:…"`. This alone closes the open namespace for every core, needs no per-core node,
  and matches `plex.direct`.
- **Per core, only if Bunny's behaviour is proven on a scratch zone** (the service report, B3): one
  scripted wildcard `*.<coreId>.<zone>` per claimed core (RFC-correct below the core's node, and it
  answers only for cores that exist without the script knowing any list), a stored CAA at
  `<coreId>.<zone>` with `issue ";"` and `issuewild "letsencrypt.org; accounturi=<the core's
  account>; validationmethods=dns-01"`, and the transient TXT. About two records a core: 5,000 per
  zone by default, "can be raised upon request" **[V, `infra/bunny-dns-limits.md`]**.
- What per-core `accounturi` adds over the apex record is protection against a TXT-proxy bug that
  sets another core's challenge; the zone key can rewrite CAA anyway. For the beta the apex record
  plus a tested TXT-name check is enough; per-core `accounturi` follows when the BR makes CAs honour
  it and Bunny's semantics are known [I].
- Prefer a zone under `.app` (HSTS-preloaded as a whole TLD: `"status": "preloaded"`,
  `"include_subdomains": true` **[V, `acme/hstspreload-status-app-dev.txt`]**), so a computed name
  can never serve plain `http://` either.
- DNSSEC on the zone only once it is known that scripted answers are signed (undocumented **[V]**);
  an unsigned answer in a signed zone is a CAA SERVFAIL, which Let's Encrypt treats as a refusal
  **[V, `acme/le-caa.txt`]**: every core's renewal would fail.

### A5 — high (today) — The legacy VPS can obtain the core's certificate and forge any client address

**Claim.** The legacy path is "a fallback"; the second round's I1/W3 treated only the Cloudflare
token.

**Verified.**
- `media.orpheum-app.com` resolves to the VPS, `198.51.100.139`; `orpheum-app.com` and `media.`
  have **no CAA** and no DS **[V, `dns/dig-2026-10-13-audit3.txt`]**.
- The VPS forwards only TCP 443 and 3478 into the tunnel (`deploy/vps/orpheum.nft`); port 80 and
  every other port reach the VPS itself.
- The core's end of the tunnel accepts any source address from the VPS:
  `AllowedIPs = 0.0.0.0/0, ::/0` (`internal/deployment/templates/wg0.conf:12`), and answers go back
  through the tunnel by policy routing (`deploy/vps/README.md` §3).

**Attacks.**
1. Root on the VPS, or whoever Hetzner gives its IPv4 after it is deleted while the record remains,
   passes HTTP-01 for `media.orpheum-app.com` and holds a valid certificate. Moving the DNAT then
   puts it in the middle of every legacy-path session: passwords, tokens, the WebRTC offers the core
   answers there [I].
2. The same root can send packets into the tunnel with any source address, private and the house's
   own included, and completes TCP handshakes because the replies come back to it. Every
   address-based rule on the core (the sign-in limiter's network key, the pairing caps, and any future
   "at home" rule, A1) is forgeable from the VPS [I, from the routing].
3. The core's Cloudflare token edits all of `orpheum-app.com` (I1: confirmed at Cloudflare: "granting
   `Zone DNS Read` access to a zone … only for that specific zone", nothing below a zone **[V,
   `infra/cf-fundamentals-api-get-started-create-token.md`]**; per-subdomain zones are Enterprise
   **[V, `infra/cf-dns-zone-setups-subdomain-setup.md`]**).

**Fix (now, cost nothing).**
- A CAA RRset **at `media.orpheum-app.com`** (a node record, so Cloudflare's apex certificates are not
  affected; Cloudflare adds its own CAs to CAA it finds for Universal SSL **[V,
  `self/cf-caa.md`]**): `0 issue "letsencrypt.org; accounturi=<the core's ACME account URI>;
  validationmethods=dns-01"` and `0 issuewild ";"`.
- Before `app.`, `account.` or `api.` exist in the zone: revoke the core's Cloudflare token and move
  the core's name to another domain, or CNAME `_acme-challenge.media.orpheum-app.com` into a
  throwaway zone whose token the core holds; Let's Encrypt follows the CNAME **[V,
  `acme/le-challenge-types.txt`]**.
- Treat tunnel-delivered addresses as untrusted for anything but logging and rate limits (A1's
  rule 2 excludes them); delete the DNS record before the VPS whenever it is retired.

### A6 — high — Recovery: a stolen mailbox takes the household, and a restore takes three days

**Claim.** Protocol §2.8 (lines 381–384): recovery takes effect after about 72 hours, with mail to
the customer, "the old core may cancel it"; D291 §6 adds a step-up; a client after a recovery shows
"a plain notice" (protocol §7 item 23).

**Verified.**
- Since an earlier commit backups leave out `secret.key`, `identity.key`, the TLS and ACME account keys
  unless asked (`internal/core/backup/backup.go`, `serverKeys`) **[V]**. T1 is fixed; U2-3 follows:
  a restore from a scheduled backup now always needs a recovery.
- NIST SP 800-63B-4: "account recovery SHALL cause a notification to be sent to the subscriber"
  **[V, `browser/nist-63b4.txt`]**: here it reaches the attacker who holds the mailbox.
- Apple: "The waiting period is fixed", and "Using an Apple device signed in to your account will
  cancel your account recovery request" **[V, `browser/apple-118574.txt`]**: the veto needs no
  person.

**Attacks.** (T5, confirmed.) With the mailbox: sign in from the attacker's own browser (the
browser binding protects nothing then), start a recovery for the household's `coreId` with a new key,
filter the mails; nobody reads the System page; after 72 hours the attacker's key holds the `coreId`,
gets a certificate for its names (DNS-01 through the service) and its signed fingerprints; clients
"see a notice". A removal (`DELETE /api/cores/{id}`) first, or a power cut, defeats "the old core may
cancel". (U2-3, confirmed.) The honest owner restoring a dead disk meets the same 72 hours, the whole
household out, and a warning nobody in the family can judge.

**Fix (one mechanism answers both).**
1. **A recovery code at claiming**: 128 random bits shown once on the System page to print or save,
   its hash with the service; the same code seals the identity key in scheduled backups (argon2id
   over the code, then XChaCha20-Poly1305, as `secrets` already does for `secret.key`'s contents).
   Restoring with the code restores the *same* key: no re-key, no client warning, no wait.
2. **Re-key by recovery code** (the key lost and the backup too): immediate, mailed, shown on every
   admin session, and refused while the old key holds a live session that does not ask for it.
3. **Recovery by mailbox alone**: seven days, cancelled automatically by *any* authenticated contact
   from the old key during the wait (a session, a heartbeat, a signed veto that also works for a
   removed or frozen core), with an alarm to every admin session and to the household's clients
   through the core. A removal never shortens it.
4. **A key change the old key did not sign blocks the client** until the person opens a link the
   core issued after the change, carrying the new key's fingerprint (A7). Never accept it on the
   discovery's word; drop D291 §6's "notice" for that case.
5. **The core tells "replaced" from "lost"**: its key file is still present and refused means someone
   else holds the `coreId`: an alarm, not K8d's routine re-key button.

**Owner.** The recovery code is new (it changes D289 §2's "out of scheduled backups" into "in them,
sealed under the recovery code"). Recommendation: take it.

### A7 — high — The remembered key protects WebRTC only; links must carry the fingerprint, and the fingerprint binding has to be exact

**Claim.** Protocol §3.5: "A changed key stops the client before it sends a password"; §2.5 item 4:
"no discovery can sit between a client and its core"; the pairing QR carries "the fingerprint".

**Verified.**
- No page can see or pin a fetch's TLS certificate; HPKP was removed in Chrome 72 **[V,
  `browser/chrome72-deps.txt`; the Fetch standard exposes no server certificate]**. A signed nonce
  over HTTPS is relayed by anyone with a certificate for the name (T3, I5: confirmed).
- The browser enforces the DTLS fingerprint: RFC 8842 §5.1 "MUST tear down the media session
  immediately" on a mismatch **[V, `discovery-audit2/rfc8842.txt`]**. So a fingerprint the core signs
  binds the channel, with an honest client.
- pion makes a fresh P-256 certificate per PeerConnection when none is set
  (`webrtc/v4@v4.2.23/peerconnection.go:226-244`) **[V]**: a QR cannot carry a DTLS fingerprint
  ahead of time (X7: confirmed).
- RFC 8122 §5.1: the endpoint picks the set using its preferred hash and needs one match per
  certificate **[V]**; JSEP §5.2.1 lets fingerprints sit at session or media level **[V]**. So signing
  one `a=fingerprint` line is not enough: an extra line can be added beside it.
- Safari deletes script-writable storage after "seven days of Safari use without user interaction
  on the site"; `persist()` does not stop it (WebKit bug 209563; the fix PR 48369 closed unmerged)
  **[V, `browser/webkit-10218.txt`, `webkit-bug-209563.txt`, `webkit-pr-48369.json`]**. A Safari tab
  re-learns the key weekly.

**Fix.**
1. Say in §3.5 what the key does: enforced on WebRTC; on HTTPS it stops only an impostor that cannot
   reach the real core.
2. **Every link the core makes carries `SHA-256(identity public key)` after `#`** (`/register`,
   `/join`, `/s`, `/p`; and the TV's QR). First contact then checks a value from the household. This
   answers discovery-core.md question 14 and covers the WebRTC-only first visit (W8: confirmed).
3. **Sign the exact answer SDP** (with `offerId` and the discovery's origin) and pass that exact
   string to `setRemoteDescription`; refuse an answer with more than one fingerprint or any hash but
   sha-256.
4. An unsigned key change blocks (A6 item 4).

### A8 — high — Credentials are not kept per core; the new device proof sends a live token

**Claim.** W4c: the central client's API "the chosen path's origin"; nothing says where each core's
credentials live. An earlier commit added a device proof for resuming a device.

**Verified.**
- One key holds the session for the whole origin: `orpheum.session` in `localStorage`
  (`web/apps/web/src/session.ts:30`). OWASP: "Do not store session identifiers in local storage",
  and "Avoid hosting multiple applications on the same origin, all of them would share the same
  localStorage object" **[V, `browser/owasp-HTML5_Security.md`]**.
- The device proof is **the live session token itself**: `signIn` stores `next` under
  `orpheum.deviceProof` (`session.ts:63-64`), and pairing and sign-in send it as
  `previousDeviceProof` (`internal/core/api/pairing.go:22`). The core accepts any token ever issued
  to that device, "live or not" (`auth.go`, `isProvenDevice`).
- CSP cannot keep tokens in: `connect-src https://*.zone` must allow every core's names, and CSP3
  wildcards match every subdomain **[V, `browser/csp3.txt`]**; anyone can enrol a core and get a name
  there.

**Attacks.** On `app.`: a person signed in to home core B opens core A's Watch Together link; if the
client sends "the" token or "the" device proof, A's owner receives a live session of B (W1,
confirmed, and now a second vector). On the paid tier the code request goes to the *service*, which
would receive the proof too. One XSS anywhere on `app.` reads every household's tokens.

**Fix.**
1. Every credential, device id, device proof, locator and remembered key keyed by `coreId`; an API
   client bound to one core attaches its credential only to that core's names or its WebRTC
   transport; a playlist's foreign URL goes without it (a `TestArch_` claim).
2. **The device proof is a separate random device secret**, issued at first sign-in and stored
   hashed on the device row; never a session token, and never sent to a discovery.
3. Later: a non-extractable WebCrypto key bound to each session (DPoP's pattern: "DPoP renders
   exfiltrated tokens alone unusable" **[V, `browser/rfc9449-raw.txt` §2]**; Ed25519 sign in Chrome
   137, Firefox 129, Safari 17 **[V, BCD]**).

### A9 — medium — The planned locator binding: right idea, unsound as written

**Claim.** Protocol §2.5 item 3: the core checks `locatorHash` itself, so "a compromised [discovery]
then opens no connection into the core"; the second round's T7 proposes the client send
`HMAC(K, offer's DTLS fingerprint)`, `K = SHA-256("orpheum-offer" ‖ locator)`; K3 keys a session
proof by "the session's token", which the core keeps only hashed.

**Verified.**
- The discovery holds every hash the core registers, so the hash check stops a bug, not a
  compromise (T7: confirmed).
- The HMAC idea is sound in principle: page script cannot use the DTLS private key (RFC 8827 §9.1)
  and the browser enforces the fingerprint (RFC 8842 §5.1) **[V]**.
- pion verifies the remote certificate against **one** fingerprint, and a **session-level line
  wins** over the media-level one (`webrtc/v4@v4.2.23/sdp.go:904-947`, `peerconnection.go:3107`), and
  it accepts md5 and sha-1 (`dtls/v3@v3.1.10/pkg/crypto/fingerprint/hash.go:14-23`) **[V]**.

**Attack.** A malicious discovery keeps the browser's media-level fingerprint (which the client's
HMAC covers) and adds a session-level `a=fingerprint` of its own certificate: pion checks the
discovery's, the HMAC still verifies, and the discovery terminates DTLS in the middle.

**Fix.** Refuse offers whose SDP does not hold exactly one `a=fingerprint`, sha-256; after DTLS
connects, compare `DTLSTransport.GetRemoteCertificate()`'s hash with the HMAC'd value; key the session
proof by the session's stored hash, `SHA-256(token)` (which the core holds and the discovery never
sees), and a link's by a value the core stores when it makes the link; make the binding mutual (the
answer's signature, A7 item 3, covers the client's fingerprint too). Correct §2.5's sentence: it
stops a discovery that only relays, not one that also serves the client.

### A10 — medium — The core hands its certificate to any scanner

**Verified.** When the core has only the discovery's certificate, `GetCertificate` returns it for any
`ServerName`, empty included (`cmd/orpheum/certificates.go`, `served.own == nil` branch;
`internal/core/certificate/certificate.go:171-177` ignores the hello) **[V]**. Censys "continuously
scans all 65,535 ports on the IPv4 Internet" and suggests searching "for IP addresses that serve
valid TLS certificates without requiring an SNI" **[V, `infra/censys-internet-scanning.md`,
`censys-blog-webfetch-notes.txt`]**. Port mapping is on by default (D290 §9, D291 §7), so every
mapped core's home IPv4 and `coreId` would be listed: the list D214 §3 forbids (I6: confirmed).

**Fix.** Refuse the handshake unless the SNI is a name the core serves; a test that a nameless
ClientHello fails. The probe already sends the name.

### A11 — medium — Device codes give the whole role; define "ordinary rights" once, for both tiers

**Claim.** D291 §5: a code-activated device gets "ordinary rights … never admin"; protocol §3.1 places
it under the paid tier (line 481); plan question 10 asks.

**Verified.** Pairings issue the whole role (`auth.go:386`, via `AnswerPairing` →
`issueSession`, `pairing.go`). An admin may enter a code for any user, admins included, with nothing
logged (`api/pairing.go`, `activateDevice`) **[V]**. RFC 8628 §5.4: a short code life "does limit the
viability of codes sent over email or text message" but not a real-time phisher **[V,
`browser/rfc8628-raw.txt`]**.

**Recommendation (U2-6 and W5).** One scope, `device`, for every code on every tier: the role minus
administering and minus account security (changing the password, making links and codes, entering
codes, removing devices); hosting, playback and the rest stay. The client reads the session's
rights, not the role's, and an admin page on such a device offers a password step-up on the core's
admin origin (A1). An admin activating a device *for another admin* gets a confirmation and a log
line. Show the asking device's place as a neutral line, not a verdict (U2-9 is right that "another
network" fires on the common mobile-data case).

### A12 — medium — Reusable links: take the usability fix, bound what the link reveals

**Claim.** Protocol §3.1 (lines 427–445): a registration link's locate is single use; a person's own
link "grants nothing".

**Assessment.** U2-1's dead ends are real [I, from the flow]; W7 is also right that `/locate` returns
names carrying the home's current addresses and a bootstrap token into the core's unauthenticated
surface. Recommendation: the locate stays reusable while the registration is unused (the core spends
the registration once, as built); once used, the link **becomes the person's own link**, revoked
when the person is removed, resets their password, or the admin revokes it, with a rolling life
(90 days, renewed by a signed-in device's visit) and a cap on distinct sources a day; a used Watch
Together link locates only for the device holding the membership. The privacy page says a link
reveals the home's address.

### A13 — medium — The sign-in limits still let a stranger lock an account out by name

**Verified.** After an earlier commit, a sign-in is refused once the account has 5 failures on one network,
**50 failures everywhere**, or the network 20, in 15 minutes (`auth.go:234`, `networkTimes = 4`,
`accountEverywhereTimes = 10`). Networks are IPv4 addresses or IPv6 /64s; a /48 holds 65,536 /64s.
Ten /64s lock the owner out everywhere every 15 minutes (the host's name shows in every Watch
Together roster). Concurrency and argon2 memory are now bounded (`hashSlots`, `attempt` reserving
first) **[V]**.

**Fix.** Let a device that proves it signed in before (A8's device secret) bypass the account-wide
limit; key IPv6 by /48 for the network limit; an account-wide *delay* rather than a refusal.

### A14 — medium — DNS-PERSIST-01 is not available; plan the beta without it

**Verified.** Let's Encrypt: "We will not be deploying dns-persist-01 until … Issue #64 … is
resolved" (2026-06-25); nothing newer in its announcements or blog through 2026-10-07 **[V,
`acme/le-community-246468.txt`, `le-feed.xml`]**. The method is in the BRs (§3.2.2.4.22, ballot
SC088) with validation reuse capped at 10 days for it **[V]**. Draft -02 makes `accounturi` a hash
over the domain, the account key's thumbprint and the account URL **[V,
`acme/draft-ietf-acme-dns-persist-02.txt` §4.1]**: only the core can compute it. Google Trust Services
runs draft-01, incompatible **[V, `acme/lec-251849.txt`]**. Let's Encrypt's CAA page lists only
`http-01`, `dns-01`, `tls-alpn-01` **[V]**, so the record as written would refuse persist (X5:
confirmed). Authorisation reuse is 30 days today, 10 from 2027-02-10, 7 hours from 2028-02-16 **[V,
`acme/le-90-to-45.txt`, `le-2026-10-07-64-day-certs.txt`]**. ARI renewals are "exempt from all rate
limits" **[V]**.

**Recommendation.** DNS-01 through the discovery is the method through the beta and likely 2027.
Mark §2.7's persist paragraph "not available"; the core computes the persist value when it ships;
CAA adds `dns-persist-01` then; on any re-key the core makes a new ACME account and **deactivates**
the old (draft -02: deactivation "stops all of that account's `_validation-persist` records from
validating immediately" **[V]**), and the service rewrites CAA. The lapse states must not rely on
`persistUntil`.

### A15 — medium — Claim approval can be won by a leaked token or a stranger's link

**Verified from the design** (protocol §2.2 items 2–3): the claim token rides in a URL fragment
opened in the admin's browser; the core shows the asking account masked. **Fix** (T10, W14, U2-5:
confirmed as reasoning): bind the approval to the admin's browser with a one-time value the claim
page returns to the System page (OAuth `state`'s pattern); show the account's address in full to the
core's admin; the claim page says "continue only if you just pressed *Connect* on your own server";
any change of the holding account needs the core admin's approval (A1 rules apply to *Allow*).

### A16 — medium — Watch CAA and CT from the core

**Verified.** CT shows issuance after the fact; Cloudflare offers CT monitoring for its own zones
**[V, `self/cf-caa.md`]**; the second round's T11 notes on SSLMate's limits stand (cached in
`discovery-audit2/`). **Fix.** The core resolves its own CAA (and later persist) records through
public DNS every few hours and alarms when they stop naming its account: it warns *before* issuance
and needs no third party. The service watches CT for the whole zone and passes alarms on; a core's
own CT query is opt-in (it tells the log server the core's id and home address). Alarms reach admin
sessions, not only the System page.

### A17 — low — Smaller points

- **TURN for members** (K9, unchanged in code): `mayRelayTo` takes no quota; add a `QuotaHandler`
  and refuse the VPS's own address. RFC 8656 only *allows* peer restrictions ("MAY") and pion's
  default handler allows every peer, so ours must stay explicit **[V, `webrtc/rfc8656.txt`;
  `turn/v4@v4.1.4/server_config.go:39-41`]**.
- **Voice exposes LAN addresses**: Chrome hides host candidates behind mDNS until camera or
  microphone permission is granted, then sends raw addresses **[V,
  `discovery-audit2/chromium-filtering_network_manager.cc:141-161`]**; Watch Together voice asks for
  the microphone, so guests' and hosts' LAN addresses reach each other. Say so; filter host
  candidates from offers relayed between households if not needed.
- **WebRTC is outside Local Network Access**: "LNA restrictions do not currently apply … WebRTC"
  **[V, `browser/lna-adoption-guide.txt`]**; the "LAN only at home" rule must hold for WebRTC host
  candidates by the core's filter (A2), not by the browser.
- **Plain-HTTP LAN listener**: a `Host` allow-list (S15) is still unbuilt; a hostile LAN reads
  passwords sent over it. Prefer the LAN name over HTTPS once A4 is settled.
- **pprof on the API mux** when only `server.listen` is loopback (K7): unchanged.

## The reopened items

| Item the owner named | Finding | Verdict | Recommendation |
| --- | --- | --- | --- |
| Trust placed in the paid service (T4/W2) | Threat model, A1, A7, A8 | Confirmed: it is fully trusted on the web; SRI and `Integrity-Policy` do not cover the document, workers or the service worker ("The possible values … are 'script' and 'style'" **[V, `browser/sri-spec.txt` §3.8]**; `register()` has no integrity option **[V, `sw-spec.txt`]**) | Say so in a "Who is trusted" section; cap the damage by A1 and A8; keep the core's own origin and the own discovery as the independent routes |
| Admin rights via `app.` (D290 §4) | A1 | The decided "keep admin" is the risk; the earlier "own origin only" remedy is bypassable | No admin for sessions minted through a discovery's client; admin sessions only with a factor `app.` cannot see (LAN-only for the beta; passkey later) |
| Bunny maybe not capable (I4) | A4; service report B3 | Partly: no CAA from scripts, wildcard-below-a-node semantics undocumented, ENTs answered by the wildcard | Apex-only CAA for the beta; one scripted wildcard per core if proven on a scratch zone |
| DNS-PERSIST not available (F1/T8) | A14 | Confirmed | DNS-01 by proxy through the beta; deactivate old ACME accounts on re-key |
| Recovery and recovery codes (U2-3, T5) | A6 | Confirmed both | Recovery code at claiming that also seals the key in scheduled backups; 7-day mailbox recovery with an automatic veto |
| Ordinary rights for code-activated devices (U2-6) | A11 | Confirmed: unbuilt; undefined | One `device` scope for every code, both tiers; step-up on the admin origin |
| Per-household credentials (W1) | A8 | Confirmed, plus a new vector (the device proof is the live token) | Everything keyed by `coreId`; a separate device secret |
| Registration link reuse (U2-1) | A12 | Confirmed, with W7's cost | Reusable while unused; becomes the person's own link, revocable and bounded |
| Add-to-Home-Screen (U2-2) | below | Confirmed: home-screen apps share nothing with Safari ("by design … without shared state with the browser" **[V, `browser/webkit-bug-181849.txt`]**) and are exempt from the 7-day cap **[V, `webkit-tracking-prevention.txt`]** | Offer it from the person's own-link page; in the empty app, a paste box and the device code (the Safari tab on the same phone enters it); no security cost |
| CAA `issue` gap (T2/I3) | A4, A5 | Confirmed at RFC 8659 and in `plex.direct`'s records | `issue ";"` at the zone apex; node CAA at `media.orpheum-app.com` now |
| SNI-less certificate (I6) | A10 | Confirmed in code | Refuse a handshake without a served name |
| "No relay ever" vs D274 (X6) | below | Confirmed: D274 §2 (later) keeps a paid relay "open in principle" | Write "no relay in the beta; decided on the beta's count"; any future relay carries only DTLS whose fingerprint the core signs, per-session credentials, peers limited to the core's candidates |
| The version window (R1) | below | Confirmed: §5 serves one previous major, D273 §7 says two | "Served until three newer majors exist and six months have passed since its successor shipped"; plus a security floor |

**Add to Home Screen, in detail.** There is no storage path from Safari into the home-screen app
that WebKit documents [V, as above]; whether a run-time manifest's `start_url` per person survives
installation is device-test territory (a VQ item). The paste box and the same-phone device code need
nothing new. Security: the own link in the app's start URL lives on the person's own device, as it
does in their messages.

**"No relay", in detail.** The protocol §1 (line 59) and the spec §1.2 say "no TURN, ever"; D274 §2
(2026-10-10, after D273) says a relay of failed connections is "open in principle but not built"
**[V, working-notes D274 §2]**. A TURN relay that only forwards DTLS sees nothing it can read *if*
the client checks the core's signed fingerprint (A7); it adds abuse and bandwidth exposure, not
confidentiality exposure. So the security prerequisite for a future relay is A7 and A9, done now.

**The version window, in detail.** D273 §7: "the previous two major versions, each for at least six
months after it is superseded, whichever is longer" **[V]**; protocol §5 (lines 657–660) drops a
major once two newer exist and six months passed: one previous major. Security addition: one client
build per major is served from `app.` (D289 §4), so **each served build must receive security
fixes for as long as its major is served**, and a major (or an Orpheum version) with a known
exploited flaw may be refused before its window ends, after notice on the System page.

## Corrections to our documents and the earlier rounds

1. **First round S2 item 1, second round T4 fix 3 and W2 fix 1**: "admin only from the core's own
   origin" does not stop a captured password (A1). Replace with A1's credential rule.
2. **Protocol §2.5 bounds, `rtcapi/doc.go`**: offers through a discovery are answered with no check
   today (A2); mark "the code must change" on the locator check.
3. **Protocol §2.7, spec §3.11**: the CAA record leaves non-wildcard names open (A4); per-core
   records collide with wildcard synthesis under RFC 4592.
4. **Second round T7's HMAC**: needs exactly one sha-256 fingerprint and a post-connect check, since
   pion lets a session-level fingerprint win (A9).
5. **Plan K3**: "an HMAC … keyed by the session's token" — the core holds `SHA-256(token)`; key by
   that (A9).
6. **An earlier commit's device proof** is the live session token (A8).
7. **Second round I1/W3** covered the Cloudflare token only; the VPS can obtain the certificate by
   HTTP-01 (A5).
8. **Protocol §3.1** "the page asks for persistent storage" as a remedy: `persist()` does not stop
   Safari's deletion (A7) — the second round's F8: confirmed.
9. **Protocol §3.4**: "Firefox 153" is right for fetch; WebSockets followed in 154; WebRTC is gated
   nowhere **[V]**.
10. **Consistency round F7, protocol §3.1**: browser HPKE needs X25519, which Safari has from 18.4
    (Apple: "Added support for X25519 for Web Cryptography"), though BCD says 17 **[V,
    `browser/safari-18_4-rn.txt`]**: set a minimum browser for paid device codes.
11. **Second round's C1 timeline**: authorisation reuse is 30 days today, not 10 (A14).

## What is sound (verified)

- **Backups** leave out every private key unless asked.
- **Device resumption** requires proof, and a new password ends other sessions.
- **Sign-in**: attempts are reserved before hashing; argon2 runs under a semaphore.
- **Pairing caps** refuse rather than evict: 1,000 total, 5 per /64, 20 per /48 (`pairing.go:21-31`).
- **The core's client** sends a strict CSP with `frame-ancestors 'none'`, `nosniff`,
  `Referrer-Policy: no-referrer` and HSTS over TLS (`webclient.go:17-41`).
- **Nonces in `single`** carry their own expiry and MAC and are spent once on a valid signature.
- **Signatures are separated by purpose** (`proof.Message`); binding them to the discovery and core
  (C8) is still to build.
- **The core reaches its discovery only over HTTPS** (`checkBaseURL`).
- **Signed DTLS fingerprints** are the right primitive (RFC 8842 §5.1, fresh certificates per
  connection), once A7 and A9 are done.
- **Let's Encrypt honours `accounturi` and `validationmethods`** and fails closed on CAA errors
  **[V, `acme/le-caa.txt`]**; wildcards need DNS-01 **[V, `acme/le-faq.txt`]**.

## Decisions for the owner

1. **Admin through `app.`** (A1): no admin for sessions minted through a discovery's client; admin
   sessions only from the core's own origin on its local network for the beta, by passkey later.
   *Recommend: yes.*
2. **A recovery code at claiming that also seals the identity key in scheduled backups**, replacing
   D289 §2's exclusion (A6). *Recommend: yes.*
3. **Mailbox-only recovery**: seven days, automatic veto by the old key, no shortcut by removal.
   *Recommend: yes.*
4. **An unsigned key change blocks** until a fresh link from the core (A6, A7), reversing D291 §6's
   "notice". *Recommend: yes.*
5. **The fingerprint in every link and QR** (A7). *Recommend: yes.*
6. **One `device` scope for every code, both tiers** (A11). *Recommend: yes.*
7. **Registration links become the person's own link once used**, bounded (A12). *Recommend: yes.*
8. **CAA**: apex `issue ";"` for the zone; per-core `accounturi` only after Bunny is proven (A4);
   node CAA at `media.orpheum-app.com` now (A5). *Recommend: yes.*
9. **The zone under `.app`** (A4). *Recommend: yes, before registering.*
10. **The protocol's "no relay ever" rewritten** to D274 §2 (X6). *Recommend: yes.*
11. **The version window** as D273 §7 says, plus a security floor (R1). *Recommend: yes.*
12. **The minimum browser for paid device codes**: Safari 18.4 (X25519), or a JS X25519 library
    (a dependency). *Recommend: Safari 18.4; no library.*

The beta must not open a paid signalling connection before A2, A3 (for anything built on `single`),
A9 and A10 are built, nor put `app.`, `account.` or `api.` into `orpheum-app.com` before A5's token is
revoked.

## Sources

Fetched 2026-10-13 into `/yoloai/cache/discovery-audit3/` unless marked (2) for
`discovery-audit2/` or (1) for `discovery-audit/`.

- **IETF:** RFC 8659 (CAA), 8657 (`accounturi`, `validationmethods`), 4592 (wildcards), 8555, 9773
  (ARI) — `acme/`; draft-ietf-acme-dns-persist-02 —
  https://datatracker.ietf.org/doc/draft-ietf-acme-dns-persist/ (`acme/`); RFC 8826, 8656 —
  `webrtc/`; RFC 8827 (1), 8829 (1), 8445 (1), 8838 (1), 8842 (2), 8122 (2), 8489 (2), 5780 (2);
  RFC 9449, 8628 — `browser/`; draft-uberti-behave-turn-rest-00 (2).
- **CA/B Forum:** Baseline Requirements 2.3.1 (§3.2.2.4.22, §4.2.2.1.2, §4.2.2.2.5) —
  https://cabforum.org/working-groups/server/baseline-requirements/ (`acme/cabf-br.txt`).
- **Let's Encrypt:** CAA https://letsencrypt.org/docs/caa/; rate limits
  https://letsencrypt.org/docs/rate-limits/; challenge types; FAQ; "From 90 to 45"
  https://letsencrypt.org/2025/12/02/from-90-to-45/; 64-day certificates (2026-10-07); community
  threads 246468 and 251849; the blog feed (all `acme/`); Boulder's `va/caa.go` at an earlier commit.
- **PSL and HSTS:** PSL guidelines (`acme/psl-guidelines.txt`); hstspreload.org status for `app`,
  `dev` (`acme/hstspreload-status-app-dev.txt`).
- **Providers:** Bunny DNS records, limits, scriptable DNS types, DNSSEC (`infra/`); Cloudflare
  token scoping, subdomain setup, CAA (`infra/`, `self/cf-caa.md`); Censys internet scanning
  (`infra/`).
- **Browsers:** Fetch standard, SRI spec, Service Worker spec, CSP3, LNA spec and Chrome's LNA guide,
  Firefox 153/154 notes, Chrome 72 removals, WebKit blog 10218 and 14403, WebKit bugs 181849 and
  209563, PR 48369, WebKit tracking-prevention page, Safari 17 and 18.4 notes, MDN
  browser-compat-data at an earlier commit (`/yoloai/cache/bcd`) — all `browser/`.
- **NIST, OWASP, Apple:** SP 800-63B-4; OWASP HTML5 Security, Forgot Password, Session Management;
  Apple 118574 — `browser/`.
- **Code:** Orpheum at `d9779c6e` (files and lines in each finding); pion `webrtc/v4@v4.2.23`,
  `ice/v4@v4.4.7`, `sctp@v1.12.0`, `dtls/v3@v3.1.10`, `turn/v4@v4.1.4` from the module cache.
- **Live checks:** `dig` for `orpheum-app.com`, `media.orpheum-app.com`, `plex.direct`,
  `orpheum-core.com` (`dns/dig-2026-10-13-audit3.txt`).
