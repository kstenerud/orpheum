> **ABOUTME:** A security audit, before development, of the discovery protocol
> (`design/discovery-protocol.md`) and the paid service's spec (shared files, draft 4), checked against
> RFCs, NIST, OWASP, browser vendors and published incidents. Findings S1–S18, most severe first.

# Discovery: security audit (2026-10-13)

**Scope.** `docs/contributors/design/discovery-protocol.md` (the protocol), the paid service's
`discovery-service-spec.md` draft 4 (the spec), decisions D154, D214, D218, D273, D274, D275, D288,
and the as-built code in `internal/core/discovery/`, `discovery/` (`discoveryproto`, `proof`,
`safe32code`, `single`), `internal/core/auth/pairing.go` and `internal/core/rtcapi/`.

**Markers.** **[V]** a claim checked at the source named beside it (a document, or the code at
An earlier commit). **[I]** my inference. Severity is for the paid service at scale unless a finding says
otherwise.

## Summary

The design's core choices hold up: the core keeps its own TLS and identity keys, the discovery sets
only a TXT record whose name it checks, credentials stay out of URLs, signatures are
domain-separated, and the service carries no media. The weak points are where the **customer
account** and the **service itself** act on a household's behalf:

- **The customer account can rebind a core's identity (S1).** A re-key, done from the dashboard,
  moves a household's `coreId`, its names, its wildcard certificate and every device's locator to a
  new key. Whoever takes over the customer's mailbox can do it, since sign-in is a magic link alone.
  The result is a full man-in-the-middle of that household's browsers. This is the one critical
  finding.
- **The central web client holds every household's passwords and tokens (S2).** Serving it from
  `app.orpheum-app.com` makes the service, its build pipeline and its dependencies a single point
  whose compromise reaches every core. That is the design Plex runs too; the mitigations are to
  shrink what that client can do and to make tampering detectable.
- **Paid device codes leak sessions and scale phishing (S3, S4).** The protocol has the service
  carry the new device's session, which contradicts the spec's "the service never sees their
  tokens". And a code the service issues for no core can be sent to any household. D288's check
  code does not stop a phisher who reads it out in real time.
- **Signalling is an unauthenticated door with a small, shared lock (S5, S6).** Any locator holder,
  former guests included, can open WebRTC connections into the core and use up its 64. The core
  checks nothing itself. The discovery can also silently man-in-the-middle the data channels,
  because nothing binds the core's DTLS fingerprint to its identity key.
- **The owner's question on a claim check code (S7):** a check code in D288's shape helps little,
  because the attacker can read it out to the victim. A claim that starts from the core's own
  System page and is approved there helps much more.

Medium findings cover the 24-hour magic link (S8), same-site cookies between `app.` and `account.`
(S9), the outside probe as a scanner (S10), locators that the core cannot revoke (S11), the zone's
DNS and registrar as a master key (S12), and rate limits that attackers can turn against households
(S13). Low ones close the list.

## Findings

### S1 — critical — Re-key from the customer account rebinds a household's identity

**Concerns:** protocol §2.8 (re-key), §2.7, §3.1 (locators); spec §2 (sign-in, recovery), §8
(`POST /api/cores/{id}/rekey`).

**What the design says.** "Re-key (identity key lost or stolen): a claim code from the new key, typed
where the core was claimed; the old key stops at once; devices' locators go on working, the `coreId`
unchanged." On the paid tier, "where the core was claimed" is the customer dashboard. Sign-in there
is an e-mail magic link, with passkeys optional (spec §2).

**Why it is a problem.** An attacker who controls the customer's mailbox for a few minutes, through
a phished webmail password, a forwarded mail or a stolen phone, can:

1. sign in to `account.orpheum-app.com` with a magic link;
2. start a core of their own, enrol it, and type its claim code into *Re-key* for the victim's
   core;
3. from then on, the victim's `coreId` belongs to the attacker's key. The attacker's core calls
   `PUT /acme-challenge` for `_acme-challenge.<victim coreId>.<zone>`, which the name check allows,
   because the name is now the attacker's, and obtains a valid `*.<victim coreId>.<zone>`
   certificate. Its heartbeat publishes its own addresses under the victim's names **[I, from
   §2.4, §2.7]**;
4. every device of the household keeps its locator ("go on working"), asks `/paths`, gets the
   attacker's addresses, and connects with a valid certificate. The browser sends the household's
   passwords and bearer tokens to the attacker, who can relay them to the real core over its LAN
   address or WebRTC.

Nothing in a browser notices: the web client trusts the WebPKI name, not the identity key. Only
native apps pin the key (§3.5). The real core sees its session refused and shows an error on its
System page, but nobody is looking at it. Mailbox takeover is the commonest account-takeover route.
NIST SP 800-63B-4 forbids e-mail as an out-of-band authenticator for exactly this reason: "access
using only a password; interception in transit or at intermediate mail servers; rerouting
attacks" **[V, NIST SP 800-63B-4, "Out-of-Band Authenticators"]**.

**Best practice.**
- Key rotation is signed by the old key where it exists. Tailscale's Tailnet Lock exists so that "a
  compromised Tailscale coordination server [cannot add] a node": nodes refuse a key not signed by a
  key they already trust **[V, Tailscale tailnet-lock docs and blog]**.
- Recovery without the old credential is slow and loud. Apple's account recovery "might take
  several days or longer", and "Apple Support can't shorten the wait period" **[V, Apple support
  118574]**. NIST requires that "account recovery SHALL cause a notification to be sent to the
  subscriber" **[V, NIST SP 800-63B-4, "Account Recovery Notification"]**.
- Certificate Transparency monitoring detects an unexpected certificate for a name you own **[I]**.

**Recommendation.**
1. **Two re-key paths.** *Rotation* (the old key is still there): the new key's enrolment is
   countersigned by the old key, and takes effect at once. *Recovery* (the old key is lost): it
   takes effect only after a waiting period (72 hours, say), with an e-mail at the start, an
   e-mail at the end, and a banner on the dashboard. While the old key still holds a session,
   recovery is refused, or the old core may cancel it from its System page.
2. **Step-up for dangerous actions.** Re-key, remove, and minting a locator for "the customer's own
   browser" ask for a fresh authentication: a passkey where the customer has one, else a new magic
   link opened in the same browser (S8).
3. **The web client pins the core's identity key (trust on first use).** On first contact over a
   direct path the client asks the core to sign a nonce with its identity key (a new purpose in
   `discovery/proof`). The client keeps the public key per `coreId` and checks it on every later
   visit. A changed key stops the client before it sends a password, with a plain warning. This
   does not stop a full relay (S12), but it does stop the simple impersonation above.
4. **The core watches CT for its names** (or the service does it for every core). A certificate
   for `*.<coreId>.<zone>` that the core did not ask for is shown to its admin as an alarm.

### S2 — high — The central web client holds every household's credentials

**Concerns:** protocol §1 item 1, §3.4; spec §1.1, §2, §7.1, §7.6.

**What the design says.** `app.orpheum-app.com` serves Orpheum's web client to every household. The
client takes the household user's name and password, holds their bearer token and the device
locator, and talks to the core with the header (§3.4). The core's CORS allows that origin.

**Why it is a problem.** Whoever controls what that origin serves, through the server, a CDN, the
build pipeline, an npm dependency, or the operator under compulsion, can ship JavaScript that sends
every household's passwords and tokens elsewhere. It can also act as any signed-in admin on any core
it reaches. The service worker in §7.6 does not pin the client. Browsers re-check a service worker
script against the network at least every 24 hours, bypassing the HTTP cache **[V, web.dev
"service worker lifecycle"; Chrome "fresher service workers"]**, so the server can always replace
it.

A second effect: on Chrome, the LAN path needs the Local Network Access permission (§3.4). The user
grants it to `app.orpheum-app.com` as a whole; the prompt reads "Look for and connect to any
device on your local network" **[V, Chrome LNA blog]**. Once granted, a compromised `app.` origin
can reach every household's router and NAS too **[I]**.

The incidents are real. Plex, which runs the same model at `app.plex.tv`, had account databases
breached in 2015, 2022 and September 2025 **[V, BleepingComputer, The Register 2025]**. A Plex
server left unpatched on an engineer's home machine was the way into LastPass in 2022
**[V, The Hacker News, March 2023]**.

**Best practice.** There is no complete in-browser defence against the origin serving bad code.
- Subresource Integrity, made mandatory by the `Integrity-Policy: blocked-destinations=(script)`
  header (Chrome 138+), stops a CDN or a third-party host from swapping scripts. It does not stop
  the origin itself **[V, MDN Integrity-Policy; Scott Helme]**.
- WhatsApp Web's *Code Verify* extension compares the page's JavaScript against hashes published
  through Cloudflare **[V, SecurityWeek, The Register 2022]**. Freedom of the Press Foundation's
  WEBCAT checks a site's code against a publicly logged, signed manifest before it runs **[V,
  freedom.press, github.com/freedomofpress/webcat]**. Both are opt-in extensions.
- OWASP's third-party JavaScript guidance asks for SRI, a strict CSP and a minimal dependency set
  **[V, OWASP Third Party JavaScript Management cheat sheet]**.

**Recommendation.** Be explicit that the service is a trusted party, as it is anyway through DNS
(S12). Then reduce and expose it:
1. **Limit what a session made through `app.` may do.** The core issues sessions created from a
   cross-origin client without the admin's destructive powers: library roots, users, settings,
   anything that writes outside the media. Those stay on the core's own client, at its own origin.
   A compromised central client can then watch and steal tokens, but cannot reconfigure the core.
   Each household also keeps a way around `app.`: the core serves its own client at its names
   (A1), and the System page says so.
2. **Reproducible builds of the web client**, with the hash of every published `app.` build in a
   public, append-only list in the repository. Ship `Integrity-Policy` and SRI on every script, a
   strict CSP (`script-src 'self'`, no inline script), and Trusted Types. Keep `app.` free of
   third-party scripts and analytics.
3. **Later:** WEBCAT enrolment, so that a security-minded user can verify the code with an
   extension.
4. **Ask for Local Network Access only when the LAN path is the one in use** and IPv6 or WebRTC
   has failed. Never ask on first load.

### S3 — high — The paid service carries household sessions to new devices

**Concerns:** protocol §3.1 ("Device codes (D288 §5)"), §4 (paid device codes); spec §1.2, §2.

**What the design says.** On the paid tier "the core of the signed-in device that enters it claims
it from the service, and the service carries the check code and the session to the device". The
spec says, of household users, that "the service never sees their passwords or tokens" (§1.2).

**Why it is a problem.** These two statements contradict each other. A session token is a full
bearer credential with no absolute expiry (D154 §3). Whoever reads the service's memory or logs, or
an operator, gets a working session for every device activated this way. It also turns the
service from content-blind into a holder of household credentials, with the legal and breach
weight that brings **[I]**.

**Best practice.** Hybrid Public Key Encryption (RFC 9180) is the standard primitive for sealing a
message to a recipient's public key through an untrusted relay **[V, RFC 9180 §1]**.

**Recommendation.** The service should carry only things it can't use:
- When the device asks the service for a code, it makes an ephemeral X25519 key pair and sends the
  public half. The service hands that key to the core with the claim. The core seals the session
  to it with HPKE, and the service passes on only the sealed blob. Or,
- the service gives the device only a locator, and the device takes its session from the core
  over a direct path, with the pairing secret it already holds (D288 §1).

Either way, add a `TestArch_` claim that no session token crosses `discoveryproto`.

### S4 — high — Paid device codes enable cross-household code phishing

**Concerns:** protocol §3.1; D288 §1–§5; spec §3.11.

**What the design says.** On the paid tier the service issues codes for a page that names no core.
Any signed-in household user who enters one activates that device as themselves (D288 §4: "signed
in as the person", their full role). D288 §3's check code is shown on both devices, and the person
confirms they match.

**Why it is a problem.** This is the OAuth device-flow attack. In February 2025 Storm-2372 sent
fake Teams invitations carrying device codes it had generated. Victims who entered them handed the
attacker a session **[V, Microsoft Security blog, 2025-02-13]**. On the paid tier, an attacker asks
the service for codes as often as its rate limit allows, then sends them to Orpheum users of any
household: "enter this code on your account page to get the new app". The check code does not
stop this. The attacker's device shows the same check code, and the attacker reads it to the
victim, on a call or in a chat: "you'll see `k7x2-q9m4`, click *They match*". D288's warning is
the only barrier. RFC 8628 names this "remote phishing". It recommends telling the user they are
authorizing a device, confirming it is in their possession, and displaying "information about the
device". It also notes that a short code life "doesn't prevent a phisher from presenting a fresh
token, particularly if they are interacting with the user in real time" **[V, RFC 8628 §5.4]**.
Microsoft's mitigation was to block device-code flow where it isn't needed, or restrict it to
trusted networks and devices **[V, Microsoft blog]**.

**Recommendation.**
1. **Show where the device is.** The service knows the requesting device's address. When a code is
   entered, the core shows its approximate place (country and city from the IP address) and network,
   and whether it is on the same network as the person entering it. When it is not, the confirm
   step says so in plain words: "This device is in another country / not on your network".
2. **A device activated by code gets a narrow role.** It gets playback and its own settings, but
   never admin rights, even when an admin enters the code. Admin needs a password sign-in on that
   device. This caps the damage of a phished code at a stolen viewing session.
3. **Rate-limit codes at both ends.** The service limits how many codes it issues per address and
   in total (planned). Each core limits how many codes it claims per hour (see S13 for the reverse
   attack: a malicious core claiming other households' waiting devices).
4. **The device confirms too.** Before the device takes its session, it shows the core's name and
   asks its own user. A TV someone else's code claimed then ends up with a prompt nobody expects,
   rather than signed in to a stranger's server **[I]**.

### S5 — high — Signalling: a locator opens the core's door, and anyone can wedge it

**Concerns:** protocol §2.5, §3.1, §4 (`POST /cores/{id}/offer`); `internal/core/discovery/signal.go`,
`internal/core/rtcapi/rtcapi.go`.

**What the design says.** "The core answers an offer through the discovery without a signed-in check
of its own, since the discovery checked the locator." "A connection grants nothing: every request
over a channel carries its own bearer token." The core holds at most 64 connections
(`rtcapi.MaxConnections`). The discovery rate-limits offers per locator and per core.

**Why it is a problem.**
- **Who holds a locator.** Every device that ever reached the core does, including Watch Together
  guests (`/join`), recipients of forwarded registration links (S11), and a scanner that burned a
  link. Many locators are never revoked (S11). With D274's "WebRTC first", every core thereby
  exposes its unauthenticated API surface, including password sign-in, to all of them. That holds
  even with no port open. "Nothing opened inbound" is not "not exposed" **[I]**.
- **The core trusts the discovery entirely.** It never sees the locator, so a bug in the paid
  service's locator check, or a compromised service, can open connections into every core.
- **Exhaustion.** As built, a peer connection counts against the 64 from the offer onwards.
  Nothing closes a connected one that sends no request **[V, rtcapi.go: no idle timeout]**. One
  locator holder can open 64 connections, through 64 offers spread under the per-locator rate, and
  hold them. The household's WebRTC path then refuses everyone with "the core holds as many
  connections as it allows". Offers are also answered one at a time in the signalling read loop,
  each waiting up to `gatherTimeout` (5 s) **[V, signal.go `holdSignalling`, rtcapi.go]**, so a
  flood of offers starves real ones.

**Best practice.** OWASP's session-management guidance applies to idle unauthenticated state:
"all sessions should implement an absolute timeout" and an idle timeout **[V, OWASP Session
Management cheat sheet]**. RFC 8445 suggests limiting what a peer's offer can cost the answerer
**[V, RFC 8445 §19.5.1]**.

**Recommendation.**
1. **The core checks the locator itself.** The discovery forwards a hash of the client's locator
   with the offer. The core matches it against the set it registered (`PUT /locators`) and refuses
   unknown ones. The discovery's check stays as the first filter.
2. **Two kinds of connection.** A signed-in client adds to its offer request a proof the core can
   check and the discovery cannot use: its session's id and an HMAC over the offer's SDP
   fingerprint, keyed by the session token. Unproven connections may only sign in, register or
   pair. They get a small separate quota (4, say), a short life (60 s without a successful
   sign-in), and are evicted first.
3. **Idle timeout.** Close a connection that has carried no authenticated request for N minutes.
4. **Per-locator concurrency** at the discovery (at most 8 open offers or connections per locator)
   as well as per-locator rate.
5. **Answer offers concurrently**, bounded, so one slow gather does not block the next offer.

### S6 — medium — The discovery can silently man-in-the-middle the WebRTC path

**Concerns:** protocol §2.5, §3.5; `discovery/proof` (what the identity key signs).

**What the design says.** The identity key "signs enrolment and session challenges and nothing
else" (§2.1). Offers and answers, with their DTLS fingerprints, pass through the discovery.

**Why it is a problem.** In WebRTC, the DTLS fingerprints in the SDP are the only thing tying the
encryption to the peer. RFC 8827: "Even if HTTPS is used, the signaling server can potentially mount
a man-in-the-middle attack unless implementations have some mechanism for independently verifying
keys" **[V, RFC 8827 §9.1]**. The discovery, or anyone in its position, can swap both fingerprints
and read every request on the WebRTC path, passwords and bearer tokens included. The HTTPS paths at
least require mis-issuing a certificate, which CT records. This path leaves no trace.

**Recommendation.** The core signs its answer's `a=fingerprint` value with its identity key (a new
purpose, `orpheum-discovery-v1:dtls:<offerId>:<fingerprint>`). The client verifies it against the
pinned key (S1 item 3), or the fingerprint carried by the pairing QR (`/p#…<fingerprint>`, §3.1).
Native apps then get real end-to-end assurance. For a browser client served by the same discovery
it adds little against a malicious service (S2), but it does stop a compromised signalling node
alone. The cost is one signature per offer.

### S7 — medium — Claiming: the owner's question on a check code

**Concerns:** protocol §2.2 (and its "Owner (2026-10-13)" note); spec §2 (recovery by claim code),
§3.11, §11.3 Q19.

**What the design says.** The core shows an 8-character, 30-minute, single-use claim code. The
customer types it into the dashboard. The open proposal adds a D288-style check code, with the core's
name and fingerprint, shown on both the dashboard and the System page.

**Two attacks.**
- **(A) The victim claims the attacker's core.** The attacker sends a claim code ("type this in to
  activate your free month"). The victim's account now holds the attacker's core. The attacker gets
  a free slot on the victim's subscription. Worse, the dashboard's "locator for the customer's own
  browser" can lead the victim's browser to the attacker's core. The victim then types their
  household password into a page that sends it to the attacker **[I]**.
- **(B) The attacker claims the victim's core.** It needs the code from the victim's System page,
  by shoulder-surfing, a screenshot in a support forum, or "support" asking for it. The attacker's
  account then holds the core. It sees the core's home addresses, can remove its names, and can
  mint locators. Via S1, it could also re-key it.

**Does a check code help?** Little, in D288's shape. In (A) the attacker sees the check code on
their own System page and reads it to the victim. In (B) the check code appears on the victim's
System page, which the attacker can't see, but the dashboard confirm happens on the attacker's own
screen, so the attacker can click it without seeing anything. The comparison has to be made by the
person who controls the *other* side, and in neither attack is that person in the loop. RFC 8628
suggests showing the code on both sides only as a guard against the user not having the device in
hand, and says real-time phishers defeat short-lived codes **[V, RFC 8628 §5.4]**.

**What helps more.**
1. **Claim from the core, same browser, no typing (primary path).** The admin, signed in to the
   core's System page, clicks *Connect to Orpheum Discovery*. The core redirects the browser to
   `https://account.orpheum-app.com/claim#<claim token>`, with a token of full 128-bit entropy and
   a few minutes' life. The customer signs in if needed and sees "Connect core '<name>',
   fingerprint `xxxx-xxxx-…`, to <their e-mail>?". There is no code for anyone to read out, and the
   page is reached only from the core's own admin UI. Plex works this way: the server's own web UI
   claims it while signed in to plex.tv in the same browser. Headless set-ups use a claim token
   that expires in about four minutes **[V, Plexopedia; yaleman.org on plex.tv/claim]**.
2. **The core approves (fixes B).** After a code is typed in a dashboard, the core's System page
   shows "<masked e-mail> asks to hold this core: Allow / Refuse". Only *Allow*, given by a
   signed-in core admin, binds it. This is the "approve on the device you control" shape, which is
   stronger than comparing codes. Keep the typed code as the fallback.
3. **Show who holds the core (detects B).** `GET /status` returns the holding account's masked
   e-mail. The System page shows it permanently, with a *This isn't me — release* action that the
   core signs with its key.
4. **Warn on the dashboard (blunts A).** "Only enter a code shown by **your own** server's System
   page. Nobody from Orpheum will ever send you one."
5. **Rate-limit claim attempts per account** (a wrong code counts like a wrong password). The spec
   names none.

So: **yes to a confirmation, but on the core's side, not a compared code.** On an own discovery the
owner types the code on a VPS they control, and none of this is needed.

### S8 — medium — The 24-hour magic link

**Concerns:** spec §2 ("Sign-in for customers", "Recovery").

**What the design says.** Magic links last "at least 24 hours" and are single use. The token travels
after `#` and the page POSTs it. The next link asked for invalidates the previous one. The page
confirms "which address asked". Sessions have idle expiry only (D154's model).

**Why it is a problem, point by point.**
- **Lifetime.** NIST treats an e-mailed code as a *recovery* code, not an authenticator. It allows
  at most "24 hours when sent to an email address" **[V, NIST SP 800-63B-4, "Issued Recovery Codes"]**. "At least
  24 hours" exceeds that cap, and the spec makes the link a first-class sign-in, not recovery.
- **Mail scanners.** Microsoft Defender Safe Links, Mimecast and Proofpoint fetch the links in
  inbound mail. Several projects have had one-time links burnt this way **[V, better-auth issue
  #9690 and similar reports]**. The `#` fragment never reaches the server **[I]**. But if the page
  POSTs the token on load, a scanner that runs JavaScript in a sandbox spends it **[I: such
  sandboxes exist, but I found no vendor statement of exactly what they execute]**.
- **Forwarded or leaked mail.** Within 24 hours, anyone holding the mail signs in, from anywhere.
  Showing "which address asked" informs the person who clicks, but does not stop an attacker.
- **No absolute session timeout.** NIST: "a definite reauthentication overall timeout SHALL be
  established, which SHOULD be no more than 30 days at AAL1" **[V, NIST SP 800-63B-4, AAL1 "Reauthentication"]**.
  OWASP: "all sessions should implement an absolute timeout" **[V, OWASP Session Management]**.
  D154's reason for having none (a TV in the living room) does not apply to a billing dashboard
  that can re-key cores (S1).

**Best practice.** Bind the link to the browser that asked for it. Stytch applies PKCE to magic
links: the browser keeps a `code_verifier`, the request carries its hash, and completion requires
the verifier, so "the request starts and ends on the same device" **[V, Stytch docs, "Adding PKCE
to a magic link flow"]**. Spend the token only on an explicit click (POST), never on load **[V,
better-auth #9690]**.

**Recommendation.**
1. **Cap the link at 24 hours**, the NIST maximum, not "at least". Twenty-four hours meets the
   owner's need to "see the email and act upon it".
2. **Same browser signs in at once; another browser needs approval.** The requesting browser holds a
   PKCE-style verifier in a `__Host-` cookie. If the link opens there, a button click signs in. If
   it opens elsewhere, for example on a phone, the page shows a 6-digit code to type into the
   original browser, or asks the original browser to approve. Only when the original browser is
   gone (closed, more than an hour old) does it fall back to "sign in here", and then with a lower
   privilege: no re-key, no remove, no new locator, until a step-up (S1 item 2).
3. **Spend the token on a button click**, never on page load.
4. **Absolute session life of 30 days** for customer sessions, with idle expiry inside that. Notify
   by e-mail on each new sign-in, giving browser and place.
5. **Push passkeys after the beta.** They are the only phishing-resistant option NIST recognises
   **[V, NIST SP 800-63B-4, "Phishing Resistance"]**. Offer a passkey right after the first sign-in.

### S9 — medium — `app.` and `account.` are the same site; the operator lives on `account.`

**Concerns:** spec §2 ("its own origin"), §8 (dashboard cookie `SameSite=Lax`, operator `/admin`
on `account.`).

**What the design says.** The dashboard lives at `account.orpheum-app.com` "so a flaw in the web
client … can never reach the dashboard's session". The session cookie is `Secure; HttpOnly;
SameSite=Lax`, with a CSRF token. The operator's `/admin` is on the same origin.

**Why it is a problem.** `app.` and `account.` are different origins but the **same site**.
`SameSite` cookies are sent on requests from `app.`. Script on `app.`, the origin that renders data
from many cores, can also set cookies for `.orpheum-app.com` that `account.` then receives:
cookie tossing, session fixation, and defeat of a naive double-submit CSRF token. OWASP warns
that the naive double-submit cookie "is bypassable by an attacker who can write cookies on the
target domain (e.g., via a vulnerable sibling subdomain)" **[V, OWASP CSRF cheat sheet]**. MDN: an
application on a subdomain "can set a cookie with the Domain attribute … abused in a session fixation
attack". The `__Host-` prefix "guarantees that such cookies are only sent to the host that set
them" **[V, MDN Cookies guide]**. So the claim "can never reach" is not true as specified. An XSS
on `app.` could also post forms to the operator's `/admin`.

**Recommendation.**
1. Name the session and CSRF cookies `__Host-…`. Use a synchronizer token or OWASP's signed
   double-submit, never the naive one.
2. Refuse non-safe requests whose `Sec-Fetch-Site` is not `same-origin`. Go 1.25's
   `http.CrossOriginProtection` does exactly this: it allows only `same-origin` and `none` and
   rejects `same-site` **[V, Go source `net/http/csrf.go`; OWASP CSRF "Fetch Metadata"]**.
3. Move the operator console off the public web: its own host, reachable only over the operator's
   WireGuard or Tailscale, passkey-only, short sessions. Its powers (disable cores, invites, and
   indirectly the DNS token) make it the service's most valuable account.

### S10 — medium — The outside probe is a scanner for hire

**Concerns:** protocol §2.4 (`configured` candidates, `turn`), §2.6; spec §7.3.

**What the design says.** After each address change, and hourly, the probe connects to every
public candidate the core reports: HTTPS, the owner's TURN servers, and WebRTC. Each failure names
its hop. The probe machine is also "a warm spare".

**Why it is a problem.** Anyone can enrol a core. A core can report any address and port as
`configured`, and any host as TURN, and change them on every heartbeat. Each change triggers a
probe. The answer says whether the target refused, timed out, or spoke TLS with the wrong name. That
is a port scanner, aimed by strangers at third parties (abuse reports land at the service's host)
or at the service's own networks: Hetzner private networks, and the metadata service at
`169.254.169.254` **[V, Hetzner metadata docs]**. TLS-only probing limits what can be *read*, but
not the reachability oracle. A warm spare holds the service's secrets, which makes it a poor host
for a component that connects wherever strangers point it **[I]**.

**Best practice.** OWASP's SSRF guidance for user-chosen destinations: validate the IP address
itself, block private, loopback and link-local ranges with a deny list where no allow list is
possible, build the request yourself rather than pass input through, and keep the requester off the
internal network **[V, OWASP SSRF Prevention cheat sheet]**.

**Recommendation.**
1. Probe only global unicast addresses. Deny RFC 1918, loopback, link-local, `100.64.0.0/10`, ULA
   `fc00::/7`, multicast, documentation ranges, and the service's own prefixes. Do the check on the
   parsed `netip.Addr`.
2. Allow few ports: 443 and the high ports the core's own mapping reported. Allow at most N
   `configured` and TURN candidates per core (say 4).
3. Rate-limit probes per core (at most one immediate probe per 10 minutes, the rest hourly) and per
   target address across all cores.
4. Report detail only once the target has proved it is the core, by its certificate for the core's
   name and the nonce echo. Before that, report only "not reached".
5. Put the probe on a host with no secrets and no private network, not on the warm spare.

### S11 — medium — Locators: who issues them, who can revoke them, what they reveal

**Concerns:** protocol §3.1 (locate-hashes, locators), §4 (`/locate`, `/paths`, `PUT /locators`);
spec §6 (retention).

**What the design says.** `/locate` turns a link-secret hash into paths and a device locator, "one
use per secret". A device that reached the core another way gets a locator from the core. The core
sends "the full set" of locator hashes (`PUT /locators`), and "when the core ends a device's session
it drops the locator too".

**Why it is a problem.**
- **Ambiguous ownership.** A locator minted by the discovery on `/locate` is bound to the hash, not
  to the core accepting the secret. The core never learned it. Does the core's "full set" include
  it? If it does not, the next `PUT /locators` deletes it and the device loses its way. If the set
  does not replace such locators, the core cannot revoke them. Either reading is a defect **[I]**.
- **Leaked links mint permanent locators.** A forwarded registration mail, a link pasted in a group
  chat, or a JavaScript-running mail scanner (S8) spends the one `/locate` and walks away with a
  locator. The real user's `/locate` then fails. The core's own one-time secret survives, but the
  locator, which reveals the household's current home addresses and opens WebRTC to it (S5),
  stays with the wrong party.
- **Privacy after removal.** A former member, an ex-partner, or a Watch Together guest whose
  locator is never revoked keeps learning the household's *current* home address as it changes.
  That is a stalking vector **[I]**.
- **One use per secret** also breaks a Watch Together link shared to a group (D243 §3). The likely
  fix, allowing reuse, widens the leak above.

**Recommendation.**
1. **The core is the only authority on locators.** `/locate` returns paths and a *provisional*
   locator that lives 10 minutes. It becomes durable only when the core confirms it, after the
   secret was accepted over the direct path. The core then includes it in `PUT /locators`, which
   replaces the full set.
2. **Bind locator to grant.** Each locator belongs to a D154 grant: a device session, or a `join`
   grant. Revoking the grant, ending the Watch Together session, or removing the user drops it.
   Guests get locators that expire with their session.
3. **Spend `/locate` on a click**, not on load (as S8 item 3).
4. **Do not return `lan` paths to a locator whose device is not on the core's LAN.** The service
   can tell from the client's address matching the core's public address **[I]**. This avoids
   handing internal addresses to everyone.

### S12 — medium — The zone, its DNS credentials and its registrar are a master key

**Concerns:** protocol §2.7; spec §3.11, §6 (CT), §7.2.

**What the design says.** One dedicated zone holds every core's names. In the beta, Cloudflare's API
holds them, with "a token scoped to that one zone". Later the service runs its own DNS, answering
"from the name with no database". Each core gets a `*.<coreId>.<zone>` certificate by DNS-01.

**Why it is a problem.**
- **Whoever can write the zone can MITM every household.** They point names at their own server and
  get certificates by DNS-01. That is the service box (it holds the token), anyone who steals the
  token, the Cloudflare account, or the registrar account. Registrar takeover is a real attack: in
  July 2024, domains migrated from Google Domains to Squarespace were hijacked through weak account
  defaults and redirected to phishing **[V, BleepingComputer, SecurityWeek, The Register 2024]**.
- **Computed DNS answers** ("any `IP.*.plex.direct` resolves to `IP`") give everyone names under the
  zone that resolve to any private address. Filippo Valsorda noted of Plex that such a name "is
  exactly what a rebinding attack needs", and that Plex users are told to whitelist the zone in
  their router's rebinding protection **[V, words.filippo.io "How Plex is doing HTTPS"]**. The
  protocol's "help page" for rebinding (§3.4) would do the same. With a TLS name check rebinding
  fails, but only if the browser is forced onto HTTPS: an `http://…<zone>` origin is a free
  rebinding tool **[I]**.
- **PSL timing.** The spec plans a Public Suffix List entry "before growth" partly for Let's Encrypt
  limits. The PSL says it does "not accept entries whose sole purpose is to circumvent rate limits
  of third parties (such as Let's Encrypt …)". It also says there is "no way to expedite"
  propagation into browsers **[V, PSL wiki Guidelines]**. The valid reason is cookie isolation
  between mutually untrusting cores, which this design has. Until it propagates, cores can set
  cookies on the whole zone (cookie tossing and cookie-bombing a sibling's names) **[I]**.

**Recommendation.**
1. **CAA with `accounturi`** per core: `<coreId>.<zone> CAA 0 issuewild "letsencrypt.org;
   accounturi=<the core's ACME account>; validationmethods=dns-01"`. Then only the core's own ACME
   account can get its certificate. Anyone with TXT-only access, or a bug in the TXT proxy's name
   check, can no longer mint one **[V, RFC 8657 §3–§4]**. The service sets it when the core
   reports its account URI. It does not stop someone with full zone control, which is what CT
   monitoring (S1 item 4) is for.
2. **Pick a zone under an HSTS-preloaded TLD** (`.dev`, `.app`, `.page`). Google Registry preloads
   those whole TLDs, so no `http://` origin can exist under the zone **[V, registry.google .app
   page]**. Otherwise HSTS-preload the zone with `includeSubDomains`.
3. **No computed answers for unknown names.** Answer only addresses the core published (and
   probed). `lan` names only for private ranges the core reported, never `127.0.0.0/8`.
4. **Registrar and DNS hygiene**: registry lock on the zone and on `orpheum-app.com`, hardware-key
   2FA on registrar and Cloudflare accounts, DNSSEC, a Cloudflare token restricted to the DNS-edit
   permission on that zone and IP-restricted to the service's hosts, monitoring of NS and DS
   changes.
5. **Submit the PSL entry now**, with the cookie-isolation rationale, and ask Let's Encrypt for a
   limit override separately, through its form.

### S13 — medium — Limits that attackers can exhaust, and limits that hit the wrong people

**Concerns:** protocol §2.2 (enrolment limit), §2.4, §3.1 (code limits "per address and in all"),
§2.5; spec §3.11, §7.2; as-built `discovery/single/single.go`, `internal/core/auth/pairing.go`.

**What the design says, and what goes wrong.**
- **Cloudflare record quota.** The beta creates "an explicit record for each published address
  name". Free zones created after 2024-09-01 hold **200 records** **[V, Cloudflare DNS docs,
  "DNS records quota"]**. A few cores reporting many `lan` and `configured` addresses, or one
  malicious core, fill the zone, and the next core gets no names.
- **Let's Encrypt.** 50 certificates per registered domain per 7 days, a global limit **[V, Let's
  Encrypt rate limits]**. Enrolling throwaway cores uses up the week's issuance for everyone until
  the PSL entry propagates (S12).
- **Per-address limits** for enrolment and device codes hit everyone behind a carrier NAT, mobile
  users above all, and do nothing against an attacker with an IPv6 `/64` of addresses **[I]**.
  "In all" limits (device codes) let one attacker deny every new TV **[I]**.
- **As built, own discovery:** `single` keeps at most 256 outstanding nonces for 5 minutes, and
  `POST /challenge` needs no session **[V, single.go `noncesMost`, `challenge`]**. A stranger posting
  256 challenges every few minutes stops the core from renewing its session. Signalling, and
  with it the household's WebRTC, then stops.
- **As built, core:** at most 100 pairings, "the oldest not yet entered giving way" **[V,
  pairing.go `pairingsAtMost`, `makeRoomLocked`]**. Behind an own discovery, which passes code
  requests from the internet to the core (D288 §5), a stranger asking 100 codes evicts the TV's
  code before the person types it. D288 §6 bounds memory, but leaves this denial open.

**Recommendation.**
1. Cap candidates per core (at most 8 published names). Plan the beta zone on a paid plan or move to
   computed-but-restricted DNS (S12 item 3) before 200 records. Alert at 85%.
2. Bound enrolments: only claimed cores get certificates. Pending cores expire in 30 minutes
   (already) and count against the inviting account.
3. Key limits on IPv6 `/64` (or `/56`), not single addresses. Make the "in all" limits degrade by
   source (fair queuing), not globally.
4. `single`: make the session challenge stateless. Use a nonce that is an HMAC of a server secret
   and a timestamp, and keep a short replay cache of *used* nonces only. Or cap outstanding nonces
   per source address.
5. Pairings: evict the newest pairing from the *source* that holds the most, not the oldest overall.
   Or keep a per-address cap at the core even on an own discovery.

### S14 — low — Port mapping on by default puts the core on the internet without a choice

**Concerns:** protocol §2.4 (`mapped`, "on by default"); D273 §7; D274 §2.

**Why it matters.** The core asks the router for an inbound IPv4 port and an IPv6 pinhole without
asking its owner. That is decided (D273 §7). But it makes every Orpheum core a scannable target, and
self-hosted media servers are targets. CVE-2025-34158 in Plex Media Server was remotely exploitable.
Plex e-mailed owners to upgrade, and over 300,000 internet-facing instances were still vulnerable
weeks later **[V, Help Net Security 2025-08-27; BleepingComputer]**. With WebRTC as the baseline
(D274), the extra reach that mapping buys is a speed upgrade, not a necessity.

**Recommendation.** Keep the default, but: (1) map a random high port, never 443 or a fixed one
(D274 §2 allows it); (2) the System page shows, prominently, "your server is reachable from the
internet at …" with the switch beside it; (3) the core tells its admin about security updates
(through `GET /status` messages); (4) sign-in rate limits and lock-out are covered by tests that
also run over the WebRTC path.

### S15 — low — DNS rebinding against the core's plain-HTTP listener

**Concerns:** protocol §3.4; as built `cmd/orpheum` (`server.listen`, `0.0.0.0:8440` in
`internal/deployment/templates/core.toml`).

**Why it matters.** The core serves plain HTTP on the LAN and validates no `Host` header (no such
check found under `internal/core`) **[V, code search]**. A page a household member visits can
rebind its own name to the core's LAN address and reach its unauthenticated endpoints and sign-in
from inside the LAN. Chrome's Local Network Access blocks this without a prompt **[V, Chrome LNA
blog]**. Other browsers do not, yet **[I]**. Transmission's 2018 rebinding RCE (CVE-2018-5702) was
fixed by a Host allow list **[V, transmission PR #468]**.

**Recommendation.** On the plain-HTTP listener, accept only `Host` values that are IP literals, the
configured names, `localhost`, or the core's names under the zone. Refuse others with 421.

### S16 — low — ICE and STUN as reflectors

**Concerns:** protocol §2.5, §2.9; `internal/core/rtcapi`.

**Why it matters.** An offer's SDP lists the candidates the core will send STUN checks to. An offer
with 50 candidates pointing at a victim makes the core send checks there. RFC 8445 calls this the
STUN amplification attack and says agents "SHOULD limit the total number of connectivity checks
they perform to 100" and "MAY limit the number of candidates they will accept" **[V, RFC 8445
§19.5.1]**. Candidates on the core's own LAN let an offer probe its neighbours, though the
results do not come back **[I]**. The discovery's STUN server can be used as a reflector with
spoofed sources. The packet count is not amplified, but responses are larger than requests **[V,
RFC 8489 §16.1.2]**.

**Recommendation.** Refuse offers with more than ~10 candidates. In offers that came through the
discovery, drop remote candidates in loopback and link-local ranges. Rate-limit the STUN server per
source. Do not implement RFC 3489's `RESPONSE-ADDRESS` (reserved since RFC 5389) **[V, RFC 8489
§18.3.1]**. Omit `SOFTWARE` **[V, RFC 8489 §16.1.2]**.

### S17 — low — What the service learns

**Concerns:** spec §6 (data held), §5.2 ("say what the service sees"); protocol §2.4, §3.2.

**Observations.**
- **Address history.** The service keeps "current addresses only, no history", but request logs
  keep the source address with the core id for 7 days, for cores and for every device that calls
  `/paths`. That is a 7-day history of where each household's members are, travel included
  **[I]**.
- **Passive DNS.** Names carry the address. Every lookup of `2a02-….<coreId>.<zone>` through a
  large public resolver can end up in commercial passive-DNS databases, which tie a stable
  `coreId` to a history of home addresses that CT does not reveal **[I]**.
- **IPv6 interface identifiers.** An EUI-64 interface ID in a published name reveals the MAC
  address and is stable across prefixes **[I]**.
- **SDP** through signalling carries the client's public address (and LAN address when the browser
  does not mask it with mDNS) **[I]**.

**Recommendation.** Log device requests without the core id, or with a truncated address. Keep
the core id only in aggregate counters. State plainly on the privacy page that names contain
addresses and are visible to DNS resolvers. Prefer stable-privacy (RFC 7217) or temporary addresses
over EUI-64 when choosing the interface candidate.

### S18 — low — Smaller protocol and as-built points

- **TXT values are not checked.** `PUT /acme-challenge` sets any value under the core's name
  **[V, single.go `acmeChallenge`]**. Accept only a 43-character base64url value, the DNS-01 key
  authorisation digest **[V, RFC 8555 §8.4]**, and clear it automatically after an hour.
- **`coreId` length.** The protocol says 26 safe32 characters (130 bits). `single` makes 8 (40
  bits) **[V, single.go `coreIDChars`]**. That is harmless for one core under the owner's own
  domain, but the paid service must not copy it, and the two documents should agree.
- **An unclaimed own discovery takes any key.** `enrol` replaces a pending core with a stranger's
  **[V, single.go `enrol`]**. The owner's code then fails until their core re-enrols. That is a
  nuisance, not a takeover, since claiming needs a shell on the VPS. Refuse a new key while a
  pending core's code is live, or let `orpheum discovery claim` show the fingerprint it binds.
- **Recovery by claim code** (spec §2) lets "support" take over an account on the strength of a code
  read out to it. The same social-engineering risk as S7 (B) applies. Make it a signed request from
  the core (the System page's *Recover account* button) rather than a code a person repeats.
- **CORS**: reflect only allow-listed origins, send `Vary: Origin`, never allow `null`. Tie
  `Access-Control-Max-Age` to how quickly a removed origin must stop working **[I]**.

## What is sound

- **The core makes and keeps its own TLS key and identity key.** The discovery only sets a TXT
  record, and checks the name is exactly `_acme-challenge.<coreId>.<zone>` for the calling core
  **[V, single.go `acmeChallenge`; protocol §2.7]**. That is better than Plex's original model,
  which "distributes private keys to users" **[V, words.filippo.io]**.
- **Signatures are domain-separated** (`orpheum-discovery-v1:<purpose>:<nonce>`), so an enrolment
  signature can't stand in for a session one. Nonces are single use, and spent even when
  verification fails **[V, discovery/proof, single.go `isSigned`]**.
- **Opaque bearer tokens, hashed at rest, never in a URL** (D154 §1–§2). 192 to 256 bits of CSPRNG
  output, well above OWASP's 64-bit floor **[V, OWASP Session Management; single.go `secretBytes`;
  auth `randomSecret`]**.
- **Secrets travel after `#`**, so they don't reach the discovery's logs, and the discovery gets
  only `SHA-256("orpheum-locate" ‖ secret)`. With 256-bit link secrets the hash can't be inverted
  **[V, accounts.go `randomSecret`; I on the fragment]**.
- **A dedicated zone for core names, separate from `orpheum-app.com`**, with PSL inclusion
  planned. This is the PSL's intended use: subdomains given "to mutually-untrusting parties" **[V,
  PSL Guidelines]**.
- **One wildcard per core** keeps addresses out of CT logs **[I, the certificate names only
  `*.<coreId>.<zone>`]**.
- **D288's direction** (the device shows the code, a signed-in device enters it) is RFC 8628's
  direction. Its life (10 minutes), single use and per-account failure counting match §5.4's advice
  **[V, RFC 8628 §5.4; pairing.go]**. Its gaps on the paid tier are S3 and S4.
- **CORS from a list, no credentials mode, header sign-in** avoids third-party cookies and the
  CSRF class that comes with them **[I]**.
- **Bounds everywhere in the as-built code**: 64 KiB request bodies and signalling messages, capped
  nonces and pairings, `MaxBytesReader` **[V, single.go, signal.go]**. S13 is about who can
  exhaust them, not their absence.
- **No relay, no media, no household data** on the paid service, which keeps its breach surface
  and legal weight small.
- **The probe on another network** (spec §7.3) is the right instinct. S10 only says what it must
  refuse to do.
- **A separate origin for the dashboard** and **operator passkeys** are right as far as they go
  (S9 finishes the job).

## Sources

Fetched 2026-10-13; copies of most are cached under `/yoloai/cache/discovery-audit/`.

Standards and guidance:
- RFC 8628, OAuth 2.0 Device Authorization Grant, §5.4–5.5: https://www.rfc-editor.org/rfc/rfc8628
- RFC 8657, CAA `accounturi` and `validationmethods`: https://www.rfc-editor.org/rfc/rfc8657
- RFC 8827, WebRTC Security Architecture, §9.1: https://www.rfc-editor.org/rfc/rfc8827
- RFC 8445, ICE, §19.5.1: https://www.rfc-editor.org/rfc/rfc8445
- RFC 8489, STUN, §16.1.2, §18.3.1: https://www.rfc-editor.org/rfc/rfc8489
- RFC 9180, HPKE: https://www.rfc-editor.org/rfc/rfc9180
- RFC 8555, ACME, §8.4: https://www.rfc-editor.org/rfc/rfc8555
- NIST SP 800-63B-4: https://pages.nist.gov/800-63-4/sp800-63b.html
- OWASP SSRF Prevention: https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html
- OWASP CSRF Prevention: https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html
- OWASP Session Management: https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html
- OWASP Third Party JavaScript Management: https://cheatsheetseries.owasp.org/cheatsheets/Third_Party_Javascript_Management_Cheat_Sheet.html
- Public Suffix List guidelines: https://github.com/publicsuffix/list/wiki/Guidelines
- Go `net/http` CrossOriginProtection (source `src/net/http/csrf.go`, Go 1.25): https://pkg.go.dev/net/http#CrossOriginProtection

Browsers and platforms:
- Chrome, Local Network Access prompt: https://developer.chrome.com/blog/local-network-access
- MDN, Cookies (prefixes, session fixation): https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Cookies
- MDN, Integrity-Policy: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Integrity-Policy
- Scott Helme, Integrity Policy: https://scotthelme.co.uk/integrity-policy-monitoring-and-enforcing-the-use-of-sri/
- web.dev, The service worker lifecycle: https://web.dev/articles/service-worker-lifecycle
- Chrome, Fresher service workers by default: https://developer.chrome.com/blog/fresher-sw
- Google Registry, .app (HSTS-preloaded TLD): https://www.registry.google/domains/app/
- Let's Encrypt rate limits: https://letsencrypt.org/docs/rate-limits/
- Cloudflare DNS records quota: https://developers.cloudflare.com/dns/manage-dns-records/
- Hetzner Cloud metadata: https://pkg.go.dev/github.com/hetznercloud/hcloud-go/v2/hcloud/metadata
- Stytch, PKCE for magic links: https://stytch.com/docs/guides/magic-links/email-magic-links/adding-pkce
- better-auth issue #9690, mail scanners consuming magic links: https://github.com/better-auth/better-auth/issues/9690
- Apple, What to expect during account recovery: https://support.apple.com/en-us/118574
- Tailscale, Tailnet Lock: https://tailscale.com/docs/features/tailnet-lock and https://tailscale.com/blog/tailnet-lock
- Meta Code Verify: https://www.securityweek.com/meta-releases-open-source-browser-extension-checking-code-authenticity/ and https://www.theregister.com/2022/03/10/whatsapp_cloudflare_code/
- WEBCAT: https://freedom.press/tech/news/introducing-webcat-web-based-code-assurance-and-transparency/ and https://github.com/freedomofpress/webcat
- Plex claim codes: https://www.plexopedia.com/plex-media-server/general/claim-server/ and https://yaleman.org/post/2021/2021-10-18-getting-a-plex-claim-code-for-docker/
- Filippo Valsorda, How Plex is doing HTTPS for all its users: https://words.filippo.io/how-plex-is-doing-https-for-all-its-users/

Incidents:
- Microsoft, Storm-2372 device code phishing (2025-02-13): https://www.microsoft.com/en-us/security/blog/2025/02/13/storm-2372-conducts-device-code-phishing-campaign/
- LastPass breach via an unpatched Plex server: https://thehackernews.com/2023/03/lastpass-hack-engineers-failure-to.html
- Plex breach, September 2025: https://www.bleepingcomputer.com/news/security/plex-tells-users-to-reset-passwords-after-new-data-breach/ and https://www.theregister.com/2025/09/09/plex_breach/
- Plex CVE-2025-34158: https://www.helpnetsecurity.com/2025/08/27/plex-media-server-cve-2025-34158-attack/
- Squarespace domain hijacks, July 2024: https://www.bleepingcomputer.com/news/security/dns-hijacks-target-crypto-platforms-registered-with-squarespace/
- Transmission DNS rebinding fix, CVE-2018-5702: https://github.com/transmission/transmission/pull/468
