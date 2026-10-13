> **ABOUTME:** Second-round security audit (2026-10-13), web lens: the central web client, everything a
> browser touches, and the as-built web code on the internet-facing staging server. Findings W1–W14,
> most severe first; errors found in our documents and in the first audit; sources.

# Discovery: second security audit, web and browser (2026-10-13)

**Scope.** `design/discovery-protocol.md` (the protocol), D154, D216, D217, D218, D221, D243, D246,
D267, D273, D288–D291, the paid service's spec (draft 5, shared files), `plans/discovery-core.md`,
`plans/webrtc-transport.md`, the service's `docs/plans/service.md`, and the code in
`web/apps/web/src/`, `web/packages/sdk/src/`, `internal/core/api/`, `internal/core/auth/`,
`internal/core/together/` at an earlier commit. The first audit (S1–S18) was read for context only. Nothing
in it or in our documents was taken on trust.

**Threats looked at.** A malicious web page; a malicious or compromised core whose data the central
client renders; a household member or ex-guest; a phisher; and a compromised service or build.

**Markers.** **[V]** checked at the source named (a spec, a vendor page, a release note, browser
compatibility data, an RFC, NIST or OWASP text, or our code at the file and line named). **[I]** my
inference. Pages are cached in `/yoloai/cache/discovery-audit2/` (new) and
`/yoloai/cache/discovery-audit/` (the first audit's, re-read where used).

## Summary

The core's API is in good shape for a browser: header-only sign-in, no credential in URLs, no HTML
sink in the React client, a WebSocket that checks `Origin`, and a media cookie that is `HttpOnly`,
`SameSite=Strict` and scoped to the byte paths. The weak points are new, and they sit where one
origin starts to serve many households.

- **W1 (high).** The central client has no rule for whose token goes where. As built, one session
  token sits in `localStorage` and is attached to every request the one API client makes. Carried to
  `app.`, a person signed in to their own core who opens another household's Watch Together link
  would send their home session token to that household's core. Any script on `app.` reads every
  household's tokens.
- **W2 (high).** The integrity measures of D290 §4 do not stop the threat they were taken for. SRI and
  `Integrity-Policy` never cover the HTML document, the service worker or workers. Whoever controls
  `app.`'s files (Cloudflare Pages, the build, the service) controls every household's client. The
  only mitigation that limits the damage, no admin rights for sessions made through `app.`, was
  declined ("keep their admin rights for now").
- **W3 (high).** The owner's own core is `media.orpheum-app.com` and holds a Cloudflare token that can
  edit every DNS record of `orpheum-app.com`, including `app.` and `account.`. A compromise of that
  internet-facing home server becomes a compromise of the paid service's every household.
- **W4 (medium, as built).** A device asking for a code chooses which existing device it claims to
  be. When a person enters its code, the intruder's session is filed under the person's own device:
  no new device is listed, and *sign out everywhere else* keeps it.
- **W5 (medium).** D291 §5 (code-activated devices never admin) is enforced nowhere. Every pairing
  issues the person's whole role. Nothing checks that the device is on the same network.
- **W6–W9 (medium).** The sign-in link misquotes NIST; reusable links leak the home's current
  addresses to whoever holds them; first contact over WebRTC cannot be authenticated; a password reset
  leaves every old session alive for an unbounded time.

## Findings

### W1 — high — The central client keeps one token for every household, and sends it everywhere

**Where.** Protocol §3.4, §3.5; webrtc-transport.md W4c; D267 §2; as built
`web/apps/web/src/session.ts:24-45`, `main.tsx:39-41`, `web/packages/sdk/src/client.ts:62-76`,
`together-viewer.tsx:159,183`.

**What is there.** The session token is kept under one key, `orpheum.session`, in `localStorage`
**[V, session.ts:24,30,41]**. One API client is made per page, and its middleware adds
`Authorization: Bearer <token>` to every request it sends **[V, client.ts:68-73, main.tsx:41]**. The
Watch Together join form, when the browser is signed in, calls `GET /me` and `POST /together/join`
through that client **[V, together-viewer.tsx:159,183]**, by design: D267 §2 makes a signed-in
joiner "that person". W4c plans the central client as "the API's base the chosen path's origin
(`createApi`'s `origin`)" and says nothing of where each core's token lives **[V]**. Today this is
safe, because the client is served by its own core.

**The attacks, on `app.` [I].**
1. **Token forwarding.** Ana is signed in to her home core B at `app.`. Her friend's core A, or
   anyone who runs a core and sends her a Watch Together link, gets her to open `/join#A.<secret>`.
   If the client keeps today's shape (one token, attached to every request of the current API client)
   and points the client at A, the `GET /me` and join carry B's token to A. A's owner now holds a
   session on B with Ana's whole role. There is no absolute expiry (D154 §3), so it lasts until Ana
   is idle past the window. The same holds for hls.js's loader if byte paths carry "the" token by the
   header (D289 §3): a playlist from A may name any URL.
2. **One origin, every household.** OWASP: "Do not store session identifiers in local storage";
   and "Avoid hosting multiple applications on the same origin, all of them would share the same
   localStorage object" **[V, OWASP HTML5 Security cheat sheet]**. On `app.`, one XSS anywhere, in
   code that renders data from any core a person ever visited (titles, member names chosen by
   strangers, metadata from TMDB), reads every token, locator and pinned key in that browser.
   I found no HTML sink today: no `dangerouslySetInnerHTML`, `innerHTML` or `eval` outside tests,
   and every `href`/`src` from data is an artwork URL or a video `src` **[V, grep of
   `web/apps`, `web/packages`]**. The risk is the blast radius of the first one.

**Fix.**
1. Write the rule into protocol §3.4 and W4c: **every credential is keyed by `coreId`**, and an API
   client is bound to one core. A token is attached only to requests whose origin is one of that
   core's own names (or its WebRTC transport). Any other URL, including one a playlist names, goes
   without it, or is refused. Joining another core's Watch Together sends no session: the joiner is
   anonymous there unless signed in to *that* core. Say in D267 §2 that "signed in" means signed in
   to the core that hosts the session.
2. **Bind sessions made through `app.` to the browser.** The browser makes a non-extractable key
   (WebCrypto Ed25519: Chrome 137, Firefox 129, Safari 17 **[V, MDN compat data]**), keeps it in
   IndexedDB, and signs each request, as DPoP does. RFC 9449: "If the private key is non-extractable
   ... DPoP renders exfiltrated tokens alone unusable" **[V, RFC 9449 §2]**. An XSS can still act
   while the page is open, but cannot carry tokens away for later.
3. Add a `TestArch_` claim: no request leaves the client with a token for a core other than the
   request's target.

### W2 — high — The decided integrity measures do not protect against a compromised `app.`

**Where.** Protocol §3.4 "The client's integrity"; D290 §4; spec §7.1; W4c; first audit S2;
as built `web/packages/sdk/src/rtc-media.ts:262-297`, `web/apps/web/src/sw.ts`.

**What is decided.** A reproducible build with published hashes, SRI on every script,
`Integrity-Policy`, `script-src 'self'`, Trusted Types; "sessions made through it keep their admin
rights for now" (D290 §4).

**What these do, checked.**
- **SRI protects subresources against the host that serves them, not against the page.** Its stated
  goal: "Compromise of a third-party service should not automatically mean compromise of every site
  which includes its scripts" **[V, SRI spec §1.1]**. The hashes ride in `index.html`. Whoever can
  change the scripts on Cloudflare Pages can change `index.html` too. The scripts are same-origin
  and content-hashed already, so SRI adds almost nothing here.
- **`Integrity-Policy` covers only the `script` and `style` destinations** **[V, SRI spec §3.8;
  MDN]**. A service worker, a dedicated worker (JASSUB, hls.js) and the HTML document are other
  destinations. Support: Chrome 138, Firefox 145, Safari 26 **[V, MDN browser-compat-data]**.
- **The service worker is unpinned.** `/sw.js` is registered with no integrity
  **[V, rtc-media.ts:297]**; the Service Worker API has no integrity option **[I]**. A malicious
  update controls every page of `app.` until it is replaced, and browsers re-check it against the
  network at most every 24 hours **[V, first audit S2's sources]**.
- **CSP cannot stop exfiltration here.** The client must connect to every core's names, so
  `connect-src` must allow the whole core zone. Anyone can enrol a core and get names there
  (protocol §2.2), so a script that runs can post tokens to the attacker's own core name **[I]**.
  CSP is still worth having against injection.
- **Trusted Types is now in every engine**: Chrome 83, Firefox 148, Safari 26 **[V, MDN
  compat data]**. Take it.
- **The sealed device-code session (HPKE) and "the discovery never learns a core's secrets" hold
  only against a passive service.** The code that holds the device's private key and reads the
  link's `#secret` is served by the service **[I]**.

**Fix.** Say plainly in §3.4 that these measures stop a CDN or cache swap and make tampering
detectable after the fact, not a compromised service. Then:
1. **Reopen D290 §4's "admin rights for now" with the owner.** Sessions made from a cross-origin
   client should lack admin powers (first audit S2 item 1). It is the only measure that limits what a
   compromised `app.` can do to a core.
2. Keep `index.html` tiny, set `Cache-Control: no-cache` on it and on `/sw.js`, register the worker
   with `updateViaCache: "none"`, and publish the hash of `index.html` and `sw.js` with the build.
3. Ship Trusted Types (`require-trusted-types-for 'script'`), with one policy for the worker URLs.

### W3 — high — The owner's core holds a token that can rewrite `app.` and `account.`

**Where.** D243 §6; `internal/deployment/templates/values.toml:11`; `deploy/release/README.md`;
`internal/core/certificate/certificate.go:53`.

**What is there.** The staging and release cores answer as `media.orpheum-app.com`
**[V, values.toml:11; release README]**. Their certificate comes by DNS-01 through "a Cloudflare API
token scoped to *Zone → DNS → Edit* on that zone" (D243 §6) **[V]**. That zone is `orpheum-app.com`,
which also holds `app.` and `account.` (spec §8). Cloudflare tokens are scoped by zone, not by record,
outside Enterprise plans **[V, Cloudflare Community threads; I: no official page states it]**.

**The attack [I].** Someone who gets code execution on the home server, which serves the internet on
443, reads the sealed token (the server has its key). They repoint `app.orpheum-app.com` to their own
server, get a certificate for it by DNS-01, and serve every paying household a client that sends them
passwords and tokens. They can also take `account.` and its sign-in links. Separately, `media.` is
the same site as `app.` and `account.`: script on the core's client can toss cookies to them, which
`__Host-` cookies (S9, D291 §6) block only if every cookie there uses the prefix.

**Fix.** Before `app.` serves anyone: move the owner's core to a name outside `orpheum-app.com` (its
own domain, or the core zone once it exists). Or delegate only
`_acme-challenge.media.orpheum-app.com` by CNAME to a separate zone the token edits; Let's Encrypt
follows such CNAMEs **[V, Let's Encrypt "Challenge types", DNS-01]**. HSTS-preload `orpheum-app.com`
with `includeSubDomains` **[I]**.

### W4 — medium — A device asking for a code chooses which of the person's devices it becomes

**Where.** D221 §2, D288 §4; as built `internal/core/auth/pairing.go:231-262,303`,
`internal/core/auth/auth.go:287-310`, `internal/core/auth/devices.go:96-110`,
`web/apps/web/src/code-sign-in.tsx:54`, `device-activation.tsx`.

**What is there.** `POST /pairings` needs no sign-in and takes `previousDeviceId`, a small integer
**[V, api/pairing.go]**. When a person enters the code, `ClaimPairing` looks that id up among *the
person's* devices and, if found, takes its name **[V, pairing.go:252-255]**. The person is shown
"*Karl's laptop* now shows a check code" **[V, en.ts `activate.checkExplain`]**. `issueSession` then
resumes that device row rather than making one **[V, auth.go:290-302]**. `SignOutElsewhere` keeps
every session of the device it is called from **[V, devices.go:96-110]**.

**The attack.** A phisher (first audit S4) or an ex-member asks for a code with
`previousDeviceId` set to a guess at the victim's current device (ids are sequential). The victim
enters the code. The screen names the victim's own device, which reads as harmless. The intruder's
session lands on that device's row. No new device appears in the list. If the victim later presses
*sign out everywhere else* from that device, the intruder's session survives. RFC 8628 §5.4 asks the
server to "display information about the device so that the user could notice if a software client
was attempting to impersonate a hardware device" **[V]**; here the device picks its own label.

**Fix.** Resume a device only on proof: the device keeps a device secret from its earlier session
and presents it; a bare id is ignored. Never resume the device the enterer is using. Show the
platform the asking device reported, and "a new device" when nothing was proven. Test it.

### W5 — medium — Code-activated devices get the full role; nothing checks where the device is

**Where.** D291 §5, D290 §5, protocol §3.1, discovery-core.md K15 and its question 10; as built
`internal/core/auth/pairing.go:303`, `auth.go:328`.

**What is there.** Every pairing issues `Scope: grantScopeRole`, "the user's role, entire"; the only
scopes in the code are `role`, `register` and `invitation` **[V, auth.go:55,328; accounts.go:25-27]**.
D291 §5 ("never admin, even when an admin entered the code") is not built, and the plan's question 10
asks whether it applies to the core's own codes at all **[V]**. An admin who is phished through a
code hands over an admin session.

**The location display (D290 §5) is a weak signal.** Geo-IP is spoofed by any proxy in the victim's
country **[I]**. RFC 8628 §5.4 says a short code life "doesn't prevent a phisher ... interacting with
the user in real time" **[V]**. The strong signal is "same network": the core can compare the
networks (`clientnet`) of the two requests itself, today, for its own codes **[V: `clientnet` exists
and keys pairings by network, pairing.go]**.

**HPKE in the browser is feasible without a library.** WebCrypto has X25519 (Chrome 133, Firefox 130,
Safari 17), HKDF and AES-GCM **[V, MDN compat data]**; the core has Go 1.26's `crypto/hpke`
**[V, GOROOT/src/crypto/hpke]**.

**Fix.** Answer question 10: yes, for every code. Issue a `device` scope (playback and own settings)
until the person signs in with a password on that device. By default refuse, or add a second step,
when the two requests come from different networks, and say so in plain words; let an admin allow
remote activation for one code.

### W6 — medium — The customer sign-in link: NIST misquoted, and one flow left ambiguous

**Where.** Spec §2 "Sign-in for customers" (line 233); D290 §16; protocol §6.

**What it says.** The link "lives at most 24 hours, NIST's cap for a code sent by e-mail [V, S8]".

**What NIST says.** "Email SHALL NOT be used for out-of-band authentication." The 24-hour cap is
for *issued recovery codes* sent to an e-mail address **[V, NIST SP 800-63B-4, out-of-band
authenticators; issued recovery codes]**. So an e-mailed link is never an authenticator in NIST's
terms, whatever its life. The first audit said this correctly; the spec turned it into a cap. Treat
the link as account recovery: NIST then requires notifying the subscriber, and S8's passkey push
follows.

**The ambiguous flow [I].** "Opened in another browser, it is confirmed ... by a short code shown
there and typed in the asking browser." The spec does not say which browser ends up signed in. If
the asking browser is signed in by that code, the code is a credential shown in the victim's mailbox
browser: an attacker asks for a link to the victim's address, then phones them: "read me the code".

**Fix.** Correct the citation. Write: only the browser that opened the link is ever signed in; a code
shown in the opening browser never signs in another. Rate-limit links per address, since each new
link cancels the last.

### W7 — medium — "Grants nothing" links reveal where the household is, for as long as they live

**Where.** Protocol §3.1 (a person's own link; a Watch Together link "finds its core while it
lives"), §2.7 (names encode addresses); D290 §10, §17.

**What it says.** A person's own link "grants nothing"; it is revoked by the admin. A Watch Together
link locates "again and again" while live.

**Why it matters [I].** `/locate` returns paths, and the paths are names that carry the core's
current IPv6 and IPv4 addresses (§2.7), plus a bootstrap token that opens WebRTC offers. So the link
gives its holder the home's current address, again after every change, and a door to the core's
unauthenticated surface. Holders include an ex-partner who kept their own link, anyone a guest
forwarded a used Watch Together link to, and a group chat. Nothing says removing a person revokes
their own link. Only `lan` paths are kept from strangers (§3.2).

**Fix.** Tie each person's own link to the person: removing them, or resetting their password,
revokes it. Give it a life (say 90 days, renewed on each visit by a signed-in device). Cap distinct
sources per link per day, show the admin when it was last used, and let a used Watch Together link
locate only from a device that holds the membership credential, by having the core hand that device
a locator rather than re-spending the link. Say on the privacy page that a link reveals the home's
address.

### W8 — medium — First contact over WebRTC through the discovery cannot be authenticated

**Where.** Protocol §2.5 item 4, §3.5; discovery-core.md question 14.

**What it says.** The client "remembers a core's identity key on first contact ... over a direct
path" and checks each WebRTC answer's signed fingerprint against it.

**Why it falls short [I].** For a household only WebRTC reaches, the first contact *is* through the
discovery: the key, the signed fingerprint and the DTLS fingerprint all come through it. A
discovery, or anyone who controls the signalling, can sit in the middle of that first session, which
carries the registration secret and the new password. Safari also deletes all script-writable
storage, `localStorage` and IndexedDB included, after seven days without interaction **[V, WebKit
ITP blog, 2020]**, so a Safari tab re-learns the key weekly, each time through the same path.

**Fix.** Carry the core's key fingerprint in every link the core makes (`/register`, `/join`, `/s`),
as the pairing QR carries the DTLS one. The client checks it before sending a secret. This answers
question 14.

### W9 — medium — A password reset keeps every old session, and the reset link lasts 7 days

**Where.** D218 §5 ("a reset signs no one out"); as built `internal/core/auth/accounts.go:115-141`.

**What is there.** Registering by a reset link sets the new password and issues a session; no other
session is revoked **[V, accounts.go:132-141]**. Sessions have no absolute expiry (D154 §3).

**Why it matters.** The usual reason for a reset is a password someone else learned. OWASP: "Ask the
user if they want to invalidate all of their existing sessions, or invalidate the sessions
automatically", and "After successful recovery, invalidate existing sessions" **[V, OWASP Forgot
Password cheat sheet]**. NIST caps an issued recovery code sent by e-mail at 24 hours **[V, NIST SP
800-63B-4]**; a reset link pasted into a chat lives 7 days here.

**Fix.** On a reset, end every other session of the person, or ask on the reset page with "sign out
everywhere" ticked. Give reset links (an existing person) a life of 24 hours; invitations for a new
person may keep 7 days. This changes D218 §5: the owner's.

### W10 — low — The core's client sends no CSP, no `frame-ancestors`, no HSTS (live on staging)

**Where.** `internal/core/webclient/webclient.go:21` (only `nosniff`); staging, checked 2026-10-13.

**What is there.** `https://media.orpheum-app.com/` answers with `nosniff` and no
`Content-Security-Policy`, `X-Frame-Options`, `Strict-Transport-Security` or `Referrer-Policy`
**[V, curl -I]**. API answers carry no `nosniff` **[V]**.

**Why it is low [I].** Clickjacking needs a signed-in frame; the token is in `localStorage`, which
browsers partition in third-party frames, so a framed client is signed out. Still, OWASP's defence
is `frame-ancestors` **[V, OWASP Clickjacking cheat sheet]**, and the System page's coming *Allow*
for a claim is a one-click action worth guarding.

**Fix.** On the core: `Content-Security-Policy: default-src 'self'; frame-ancestors 'none';
object-src 'none'; base-uri 'none'`, `Referrer-Policy: no-referrer`, HSTS on the HTTPS listener,
`nosniff` on every API answer.

### W11 — low — The WebRTC service worker makes a core's bytes same-origin to the page

**Where.** `web/apps/web/src/sw.ts`, `web/packages/sdk/src/rtc-media.ts:79-104`.

**What is there.** On the WebRTC path the worker answers same-origin `/api/v1/` loads with whatever
the core returned, its status and headers passed through **[V]**. Navigations are not taken (the
check is on `clientId`) **[V, sw.ts]**.

**Why it matters on `app.` [I].** A malicious core's byte answer then counts as `app.`'s own. Today
only `<img>`, `<video>` and hls.js read them, which run no script. One future sink (opening an image
in a tab, an `<object>`, a download link) would run a core's HTML or SVG as `app.`, with every
household's tokens in reach.

**Fix.** In the worker, allow only the content types a byte path serves (image, video, `mpegurl`,
VTT, ASS, JSON), and add `Content-Security-Policy: sandbox` and `nosniff` to what it returns.

### W12 — low — DNS rebinding: the `Host` check (S15) is not built; browser cover is partial

**Where.** D291 §6 (S15 taken); protocol §3.4; as built: no `Host` check in `internal/core`
**[V, grep]**; `cmd/orpheum/serve.go` mounts pprof on the API's mux when the listener is loopback.

**What browsers do.** Local Network Access checks the address actually connected to, on every
connection, "as DNS rebinding attacks may otherwise trick the user agent" **[V, LNA spec §5.2]**. It
*asks*; it does not block without a prompt. Chrome: fetch from 142, WebSocket and WebTransport from
147, WebRTC not gated **[V, Chrome blog; chromestatus via Sentry tracker]**. Firefox: on by default
for desktop from 153, WebSocket from 154 **[V, Firefox 153 and 154 release notes]**. Safari: nothing
found **[I]**. So a Safari user, or anyone who clicks *Allow* on a bad site, can be rebound onto the
core's plain-HTTP LAN listener, and in `make dev` onto a loopback listener with pprof.

**Fix.** Build S15: on the plain-HTTP listener accept only IP literals, `localhost` and the configured
names, else 421.

### W13 — low — No PSL entry: sibling cores can toss cookies onto each other's names

**Where.** D291 §4; `internal/core/api/media_cookie.go:28-33`.

**What is there.** The media cookie is `HttpOnly; SameSite=Strict; Path=/api/v1/bytes/` and its value
is the session token itself **[V]**. It cannot be `__Host-`, which requires `Path=/` **[V, MDN
Cookies]**. Without a PSL entry, a core with a certificate under the zone can set `Domain=<zone>`
cookies **[I]**: shadowing another core's media cookie for direct visitors, or stuffing cookies until
its byte paths fail. The central client sends no cookies cross-site, so it is not affected.

**Fix.** Keep the central client cookie-free (D289 §3 already). On the core's own client, make the
media cookie a separate, byte-path-only grant rather than the session token, so a leaked cookie is
not an API session.

### W14 — low — A claim link can be sent by a stranger

**Where.** Protocol §2.2 item 2; spec §3.11.

**What is there.** `account.orpheum-app.com/claim#<token>` is a URL. An attacker can enrol their own
core and send its claim URL to a customer ("activate your free month"). The customer confirms; the
attacker, as that core's admin, presses *Allow* **[I]**. With one core per account in the beta
(D290 §18), the customer's own core is then locked out of their account until they notice.

**Fix.** On the claim page: "Continue only if you just pressed *Connect* on your own server." Let the
customer remove a held core at once, and count claims per account.

## Errors in our documents or the first audit

1. **Spec §2, line 233**: "at most 24 hours, NIST's cap for a code sent by e-mail [V, S8]". NIST
   forbids e-mail for out-of-band authentication; 24 hours is the cap for an issued *recovery* code
   **[V, NIST SP 800-63B-4]** (W6).
2. **Protocol §3.4, "The client's integrity"**: presents SRI, `Integrity-Policy` and CSP as the
   client's integrity. They do not cover the document, the service worker or workers, and do not stop
   a compromised origin **[V, SRI spec]** (W2).
3. **Protocol §3.4, LNA line**: "Firefox 153" is the fetch gate on desktop only; WebSocket followed in
   Firefox 154 **[V, release notes]**. WebRTC is gated in neither browser **[V]**.
4. **Protocol §3.3 item 1**: `targetAddressSpace: "local"` is a mixed-content exemption for
   non-secure URLs **[V, LNA spec §3.1.2, §3.2]**. The LAN name is HTTPS, so it neither avoids nor
   changes the prompt. Harmless; the text should not suggest it helps.
5. **D290 §17 and protocol §3.1** cite D267 §2 for "the device holding the membership credential its
   first use gave". That is D267 §1 (a guest's membership kept by the browser); §2 is the signed-in
   person **[V]**.
6. **D290 §10, protocol §3.1**: a person's own link "grants nothing". It grants the home's current
   addresses and a WebRTC door (W7).
7. **D291 §5** is presented as decided and K15 as building it, but the core's own codes (built) still
   issue the full role, and question 10 shows it is unclear whether §5 applies to them **[V]** (W5).
8. **First audit S15**: "Chrome's Local Network Access blocks this without a prompt". LNA shows a
   permission prompt; it blocks only when refused **[V, Chrome LNA blog; LNA spec]**. "Other
   browsers do not, yet" is out of date: Firefox 153 **[V]**.
9. **First audit S2**: "`Integrity-Policy` ... (Chrome 138+)". Firefox 145 and Safari 26 have it too
   **[V, MDN compat data]**.
10. **First audit S6 and protocol §3.5** assume a direct path for first contact; for a WebRTC-only
    household there is none (W8).

## What is sound (verified)

- **No credential in a URL on the API.** The client sends the token only as `Authorization`
  **[V, client.ts:68-73]**; link secrets ride after `#` and leave the address once spent
  **[V, register.tsx:158-163; together-viewer.tsx:124-126]**.
- **No HTML sink in the React client**: no `dangerouslySetInnerHTML`, `innerHTML`, `eval` or
  `new Function` outside tests; data-driven `src` values are artwork and video URLs
  **[V, grep]**.
- **The API takes no ambient credential**, so CSRF needs no token: byte paths are `GET`/`HEAD` only,
  and the media and viewer cookies are `HttpOnly; SameSite=Strict`, scoped to `/api/v1/bytes/`
  **[V, media_cookie.go; route list]**.
- **CORS as specified** (a list, no credentials mode, `Vary: Origin`, never `null`) is right. Note that
  with no ambient credentials CORS is not the security boundary; the token is. `Authorization` must be
  named in `Access-Control-Allow-Headers`: the Fetch standard excludes it from the `*` wildcard, and
  drops it on a cross-origin redirect **[V, Fetch standard]**.
- **The Watch Together socket checks `Origin`**: `websocket.Accept(..., nil)` refuses an `Origin` whose
  host differs from `Host` **[V, together_socket.go:253; coder/websocket v1.8.15 accept.go]**. The
  central client will need `OriginPatterns` naming `app.`, never `*`. Credentials ride in `hello`, not
  cookies, so a cross-site socket gains nothing.
- **Watch Together as built matches D290 §17**: a used link admits no one new (`Join` refuses it), and
  the same browser comes back by its kept credential **[V, hub.go `Join`; together-viewer.tsx]**.
  Ejection and ending revoke credentials at once **[V, hub.go]**.
- **Device codes as built**: 10 minutes, single use, per-network cap of 5 and total of 100 with
  refusal rather than eviction, a wrong code counting against the enterer **[V, pairing.go]**.
- **LNA is real and on**: Chrome 142+ and Firefox 153+ (desktop) prompt before a public page reaches a
  private address **[V]**. D290 §2's "LAN path only when at home" avoids the prompt for guests.
- **The S9 measures** (D291 §6) do what they say: Go's `CrossOriginProtection` allows non-safe methods
  only for `Sec-Fetch-Site: same-origin` or `none`, else compares `Origin` with `Host`
  **[V, Go 1.26 `net/http/csrf.go`]**.
- **HPKE for paid device codes is buildable** on WebCrypto in every current engine and on Go 1.26's
  standard library **[V]**.

## Sources

Fetched 2026-10-13; cached in `/yoloai/cache/discovery-audit2/` unless marked (1), the first audit's
cache.

Specifications and standards:
- Local Network Access, WICG draft, 7 August 2026: https://wicg.github.io/local-network-access/
- Subresource Integrity, Editor's Draft 20 March 2026: https://w3c.github.io/webappsec-subresource-integrity/
- Fetch standard (CORS non-wildcard request-header; redirects): https://fetch.spec.whatwg.org/
- RFC 9449, DPoP, §2: https://www.rfc-editor.org/rfc/rfc9449
- RFC 8628 §5.4–5.5 (1): https://www.rfc-editor.org/rfc/rfc8628
- NIST SP 800-63B-4 (1): https://pages.nist.gov/800-63-4/sp800-63b.html
- OWASP HTML5 Security: https://cheatsheetseries.owasp.org/cheatsheets/HTML5_Security_Cheat_Sheet.html
- OWASP Clickjacking Defense: https://cheatsheetseries.owasp.org/cheatsheets/Clickjacking_Defense_Cheat_Sheet.html
- OWASP Forgot Password (1): https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html

Browsers:
- Chrome, New permission prompt for Local Network Access (1): https://developer.chrome.com/blog/local-network-access
- Chrome LNA milestones (142 fetch, 147 WebSocket/WebTransport, 156 policy removal), via Sentry's
  tracker of chromestatus 5152728072060928: https://github.com/getsentry/browser-updates-radar/issues/29
- Firefox 153 release notes: https://www.firefox.com/en-US/firefox/153.0/releasenotes/
- Firefox 154 release notes: https://www.firefox.com/en-US/firefox/154.0/releasenotes/
- MDN, Integrity-Policy: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Integrity-Policy
- MDN browser-compat-data (Integrity-Policy, TrustedTypePolicyFactory, SubtleCrypto, script
  integrity): https://github.com/mdn/browser-compat-data
- WebKit, Full Third-Party Cookie Blocking and More (7-day cap) (1): https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/

Services and libraries:
- Let's Encrypt, Challenge types (DNS-01 follows CNAME) (1): https://letsencrypt.org/docs/challenge-types/
- Cloudflare Community, tokens scoped per zone only: https://community.cloudflare.com/t/restrict-scope-api-tokens-to-a-subdomain/156702
- coder/websocket v1.8.15 `accept.go` (module cache)
- Go 1.26 `net/http/csrf.go`, `crypto/hpke` (GOROOT)

Our code and the live staging server: the files and lines named in each finding, at an earlier commit;
`curl -I https://media.orpheum-app.com/` on 2026-10-13.
