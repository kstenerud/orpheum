# Each core serving its own client: an independent security audit of D308's options

*2026-10-17. An adversarial review of
[client-from-core-2026-10-17.md](client-from-core-2026-10-17.md) (the designer's proposal) and of
options A–F for D308. No code was changed. Each claim is labelled **[V]** (verified: I read the code
or document, or the source is cited) or **[I]** (inferred: my reasoning, not tested). Sources fetched
are cached under `/yoloai/cache/client-from-core-audit/`.*

## 0. Verdict

- **Option A (shared origin) must not be built.** The designer's core claim is right, and A is worse
  than the proposal says. Any core anyone claims would run code at `app.orpheum-app.com`, which holds
  every household's sessions, device secrets and locators. Through the locators it also learns other
  households' home addresses. On a browser at home, it can **administer another household's core**,
  because that core's CORS admits `app.` and its LAN name gives admin (§2.2, A-3). Every version of
  every client any core runs would become part of `app.`'s attack surface.
- **Option B (Universal SSL on `orpheum-core.com`) should not be built as described.** By
  Cloudflare's own documentation, Universal SSL on a zone with CAA records adds **hidden, unrestricted
  `issue`/`issuewild` records** for four CAs, Let's Encrypt among them. Users report that this cancels
  `accounturi`/`validationmethods` restrictions (§3.2). If that happens at each core's CAA node, it
  undoes D292 §1's per-core CAA. It also undoes D293 §10, which turned Universal SSL off for exactly
  this zone. There are smaller problems too: the zone's per-core CAA nodes stop a wildcard DNS record
  from covering the client names, so B costs a record per core in a 200-record zone (§3.3).
- **Option C (a Let's Encrypt certificate per name, issued to the discovery) cannot be proxied on
  Cloudflare's free plan.** It would expose `prod-api`'s address. It also has to weaken each core's
  CAA, or add new CAA nodes (§3.4).
- **Recommended: Option E, done properly.** That means a per-core client origin on a **new registered
  domain** (say `orpheum-client.com`), with Universal SSL and a proxied wildcard record there, and
  `orpheum-core.com` left exactly as it is. It needs the controls in §6. The most important are:
  - the origin is bound to its core by its hostname;
  - the client code is checked against a **manifest signed by the core's identity key** before it runs;
  - `app.` holds no sessions at all.

  A Public Suffix List entry is the long-term fix for the same-site problems between client origins.
  The PSL is likely to refuse a project this size for now (§3.5).
- **The designer's threat analysis is right in direction but incomplete.** It missed the admin
  escalation, the address leak, the CAA injection, the hostname-to-core binding and today's same-site
  cookie exposure. It also overstated the service-worker takeover (§2.3).

## 1. What is there today (the facts the analysis rests on)

| Fact | Where | |
| --- | --- | --- |
| `app.` stores, per core, the session token, device id, **device secret** and **locator** under `orpheum.core.<coreId>.` in one `localStorage` | `web/apps/web/src/session.ts` (`corePrefix`, `createSession`); `main.tsx` `sessionOf`/`onConnected` | V |
| `app.` also keeps the last core (`coreId`, identity fingerprint, realm) and the remembered realm | `discovery-start.ts` `keptCoreKey`, `keptRealmKey` | V |
| The Watch Together **viewer membership is under one key for the whole origin** (`orpheum.together.viewer`), not per core, contrary to protocol §3.4 | `together-storage.ts` `membershipKeys`; `main.tsx` passes it to `heldWith` for every core | V (low impact, see F-9) |
| connectd sets **no cookies** anywhere | grep of `orpheum-connect` (non-test Go) for `Cookie`/`Set-Cookie`/`__Host` finds none | V |
| connectd's client API allows CORS from exactly one origin, `clientOrigin`, with no credentials, `Vary: Origin` and no `null` | `orpheum-connect/internal/api/cors.go`; `cmd/connectd/main.go` `routes` | V |
| Every client API call carries a held secret (link hash, bootstrap token, locator, realm proof). No ambient authority exists | `orpheum-connect/internal/clients/clients.go` `Handler`, `locate`, `paths` | V |
| `app.`'s CSP is `script-src 'self'` and `connect-src` covers **every** core name (`https://*.orpheum-core.com:*`, `wss://*.…`) | `orpheum-connect/internal/webclient/webclient.go` `policyOf` | V |
| A core allows CORS from **exactly** its discovery's client origin, with `Authorization` among the allowed headers | `internal/core/api/cross_origin.go:36` | V |
| A core sets **cookies** on its own names: `orpheum_media` (the **session token** itself) and `orpheum_viewer`, both `HttpOnly; SameSite=Strict; Path=/api/v1/bytes/`, with no `__Host-` prefix. Their comment assumes that "a forged cross-site request can only fetch what its user may already see" | `internal/core/api/media_cookie.go` | V |
| Admin takes effect only on a request reaching the core directly from its own network | D297 §6; `discovery-start.ts` `reachCore` comment (D300 §4) | V |
| The service worker relays only same-origin `/api/v1` loads, and only for pages that say they are on WebRTC. Navigations (`clientId` empty) go to the network | `web/apps/web/src/sw.ts`; `sdk/src/rtc-media.ts` `loadAfterAsking` | V |
| A link's fragment parser checks the fingerprint's length but **not the shape of the `coreId`** | `sdk/src/discovery.ts:31-32` | V |
| On a **direct** path, the core's identity is authenticated by TLS alone. The signed-nonce check is "Not built" (and could not stop a relay anyway) | `discovery-protocol.md` §3.5 | V |
| `orpheum-core.com`: apex `issue ";"` and `issuewild ";"`; each claimed core's `<coreId>.<zone>` has `issue`/`issuewild "letsencrypt.org; accounturi=…; validationmethods=dns-01"`; Universal SSL off; not on the PSL | protocol §2.7; D292 §1; D293 §10; D291 §4 | V (as documented, not checked live) |

## 2. Threat model

### 2.1 Actors and what each already has today

| Actor | Already has today | What D308 changes |
| --- | --- | --- |
| **Malicious core / operator (M)** | Its own users' data. Its own names, and a valid wildcard for `*.<M>.orpheum-core.com`. Arbitrary script at its own names, through its own `internal/core/webclient` | What origin its script runs at |
| **Benign core running a vulnerable or old client (H′)** | Its own users at risk from its own client | Whether its bug lands on a shared origin |
| **Network attacker (N)** | Nothing, given TLS and the signed WebRTC answers | Whether the code is authenticated as well as the channel |
| **Compromised discovery / Cloudflare account (D)** | Everything: it serves `app.`'s code and writes the zone, CAA included (RFC 8826 §4.3, protocol §3.7, X21) | Whether anything is capped |
| **Malicious guest or user (G)** | What their role allows. Data they put in front of others (display name, chat) | Which origin's client parses that data |
| **Other web sites (W)** | Cross-site requests to the cores and the discovery | Same-site status, if names move |

**The owner's line:** damage to M's own users through M is accepted. Anything beyond that is ours.
The case that decides it is **a person of household H who once opened M's link** (a Watch Together
guest, or a friend). Damage to H through that browser is beyond the line.

### 2.2 Option A: shared origin

| # | Threat | Real? | Notes |
| --- | --- | --- | --- |
| A-1 | M's client reads H's token, device secret and locator from `localStorage` | **Real** [V] | session.ts keeps them at one origin. The **device secret** lets M resume H's device after the token ends, so the theft persists. |
| A-2 | M learns **H's home addresses** | **Real, missed by the designer** [V+I] | With H's locator, M calls `paths` (any origin can) and gets `directUrls`, whose labels encode H's IPv4 and IPv6 addresses. |
| A-3 | M **administers H**: when H's admin opens M's link at home, M's code calls H's LAN name with H's token. The request comes from the LAN, and H's CORS admits `app.` | **Real, missed by the designer** [I from D297 §6 + cross_origin.go] | This breaks D297 §6's "admin only from home" for every household whose admin ever opened another household's link at home. D295 §1: admin is where writes, and real damage, happen. |
| A-4 | Fake realm sign-in shown under `app.`'s own URL | **Real** [I] | Perfect phishing: the address bar is ours. |
| A-5 | Persistence after the tab closes | **Partly real** [I] | Registering its own service worker does not work: the worker's script fetch bypasses service workers and goes to connectd, which serves only the built files. Persistence still comes three other ways. **(a)** It can poison the Cache API store the proposal's §4 would add for client assets, so H's next load runs M's code. **(b)** It can rewrite pinned fingerprints and the remembered core in `localStorage`. **(c)** It can script other same-origin tabs and windows it opens (`window.open('/')`) while the user signs in elsewhere. |
| A-6 | XSS in **any** core's client (old, forked, or fed hostile data by a guest G) gives app.-wide compromise | **Real, missed by the designer** [I] | The attack surface becomes the union of every client version every core runs. |
| A-7 | "Riding the discovery's cookies" | **Not real today** [V]: connectd sets none, and the API has no ambient authority | It becomes real the day an account dashboard or any `__Host-` cookie lands on `app.` (D292 §2 plans `__Host-` cookies on `orpheum-app.com`). |
| A-8 | CSP limits M's code | **No** [V] | The core's bytes arrive as same-origin loads, so they *are* `'self'`. If the worker ever answered a navigation with core bytes, the core would also set the document's CSP. |

**Blast radius of A:** every household of every person who ever opened any malicious core's link,
plus admin at home. **Reject.**

### 2.3 Testing the designer's specific claims

| Claim (proposal §3, §6) | Assessment |
| --- | --- |
| Cross-core session theft | **Real** under A [V]. Also understated: device secrets, locators and therefore addresses, and admin at home (A-1–A-3). |
| Persistent service-worker takeover of `app.` | **Overstated** as a worker registration [I, from the service-worker spec]. It is **real** through the asset cache the proposal itself adds, and through `localStorage` poisoning (A-5). |
| Realm password capture | **Real** under A [I]. Not new under B/E: M's own names can already show a fake form today. |
| Riding the discovery's cookies | **Not real today** [V], but a real constraint on future design (A-7). Same-site with `api.` matters more for **cookie tossing and bombing**: a `<core>-app.orpheum-app.com` origin could set `Domain=orpheum-app.com` cookies and break `app.`/`api.` for that browser (§3.1). |
| "Under `orpheum-core.com` every core's code would be same-site with every other core's name" (an argument against that option) | **Already true today** [V]: every core already serves its own script at its own names in that zone. The argument is not a reason against a per-core origin there. The CAA problem is (§3.2). |
| "The transport check proves the code came from that core, not that the core is benign" | **Correct**. Under A it means nothing. Under B/E it is exactly what is needed: it binds origin X's code to core X. That holds only if the binding covers the **code** and not just the channel (§4.2). |

## 3. Options B, C, D, E and F

### 3.1 Same-site but cross-origin, with no PSL entry (affects B and C, and E among its client origins)

- **Origin-keyed storage holds** [V, platform behaviour]: `localStorage`, IndexedDB, the Cache API,
  service workers and permissions (microphone, local network access) are all per origin. Moving each
  core to its own origin removes A-1 to A-6.
- **Cookie tossing and cookie bombing** [I, with sourced limits]. Script at
  `<M>.orpheum-core.com` (or at M's own names today) can set `Domain=orpheum-core.com` cookies, which
  every other core's names and client origins receive.
  - **Cookie bomb:** Chrome keeps about 180 cookies of up to 4 KB each per domain, roughly 720 KB.
    Cloudflare refuses requests whose headers exceed **128 KB** [V,
    `cf-conn-limits.md`]. So under B, M can make H's client origin, which Cloudflare proxies,
    unloadable in that browser until its cookies are cleared. That is damage to H, beyond the line.
    The core's direct paths are fetched with CORS, without credentials, so no cookies go [I]. Go's
    1 MB header limit probably survives navigations to a core's own names [I].
  - **Cookie fixation on byte paths, today:** M can toss `orpheum_media=junk;
    Domain=orpheum-core.com; Path=/api/v1/bytes/images/`. The more specific path is sent first, so
    H's images fail. **Existing finding F-6.**
- **SameSite gives no protection between cores** [V+I]. Every core's names are one site, so H's
  `SameSite=Strict` media cookie (the **session token**) is sent on `<img>`/`<video>` loads that M's
  page makes to H's names. M cannot read the bytes (opaque responses), but load/error events and
  timing reveal which items exist in H's library: an XS-leak. This exists **today** at M's own names
  (F-6). `media_cookie.go`'s "cross-site" assumption is wrong for this zone.
- **`document.domain`** needs both pages to opt in. Honest clients never do. Chrome ignores the
  setter by default (origin-keyed agent clusters). Send `Origin-Agent-Cluster: ?1` anyway [I].
- **Spectre-class process sharing:** Chrome isolates by *site*, so pages of all cores can share a
  renderer. Send `Cross-Origin-Opener-Policy: same-origin` and `Cross-Origin-Resource-Policy:
  same-origin` on client origins and core names [I].
- **Fetch Metadata** would say `same-site`, not `cross-site`, for M→H. Nothing in the core uses it
  today [V: grep finds no `Sec-Fetch`], but no future check may rely on it.

### 3.2 Option B: Universal SSL on `orpheum-core.com`, apex CAA relaxed

- **CAA semantics** [V, RFC 8659 §3, cached]. The relevant RRset is found by climbing from the name
  toward the root to the first node that holds CAA. A request for `*.X` uses `RelevantCAASet(X)`.
  `issuewild` governs wildcards when present. The Universal certificate covers `orpheum-core.com` and
  `*.orpheum-core.com`. Both are checked at the **apex**, so the apex's `issue` *and* `issuewild`
  must allow Cloudflare's CAs. `*.<coreId>.<zone>` and `<addr>.<coreId>.<zone>` still find the core's
  own node first.

  On RFC semantics alone, then, relaxing the apex opens only names that have no CAA node of their
  own: unclaimed ids, and any window where a core's record is missing during a rewrite or because of
  a bug. That is real defence-in-depth lost, but small.
- **Cloudflare's actual behaviour is worse** [V, Cloudflare docs `cf-caa.md`; community reports]:
  - Docs: *"Cloudflare adds CAA records automatically when you have Universal SSL and add any CAA
    records to your zone… these records will not appear in the Cloudflare dashboard."* They are
    `issue`/`issuewild` for `pki.goog`, `letsencrypt.org`, `ssl.com` and `sectigo.com`, **with no
    `accounturi`**, and *"this list is not exhaustive."* The behaviour does not apply to Advanced
    Certificate Manager.
  - Users report that after moving a zone onto Cloudflare, CAA records with
    `accounturi`/`validationmethods` were answered as a plain `issue "letsencrypt.org"`. Cloudflare's
    community thread "Cloudflare nameservers ignore CAA record validationmethods and accounturi" says
    as much (titled in search results; the page returned 403 to fetching). So does a 2026 write-up on
    the jabber.ru attack (zenodo 18330221).
  - CAA entries in one RRset combine as alternatives. An unrestricted `issue "letsencrypt.org"` beside
    the core's restricted one lets **any** Let's Encrypt account issue, by **any** method.
  - **If** Cloudflare adds these at every node holding CAA (each `<coreId>.<zone>`), turning Universal
    SSL on erases the per-core `accounturi` and DNS-01-only restriction. A certificate for
    `<addr>.<H>.orpheum-core.com` could then be had by HTTP-01 or TLS-ALPN-01 by whoever controls
    that address's port 80 or 443: a reassigned dynamic address, another device behind the same
    port-mapped IPv4, a hostile network on the path. This is exactly what audit A4 and D292 §1
    closed, and exactly what D293 §10 turned Universal SSL off to keep. **Not verified per node.** It
    must be tested on a throwaway zone (`dig CAA <sub>`) before anyone relies on the opposite.
- **The wildcard DNS record doesn't reach claimed cores** [I, RFC 4592]. `<coreId>.<zone>` already
  exists as a node (its CAA and its children), so a proxied `*.orpheum-core.com` record is not
  synthesised for it. B needs one proxied record per core. The free zone holds about 200 records
  (protocol §2.7), so that takes about 20% of the space, and adds one more record per core for the
  zone writer to get wrong.
- **What the browser trusts under B** [I]: Cloudflare's edge and connectd serve the shell at
  `<coreId>.orpheum-core.com`, as they serve `app.` today. The core is trusted only for what it sends
  back over the authenticated path. This root of trust is unchanged.
- **Whether a core could get a certificate for its own client name** [I]: yes, today. Its CAA node
  covers the exact name `<coreId>.<zone>`, and the DNS-01 proxy accepts `_acme-challenge.<coreId>.<zone>`,
  which is the challenge for both `*.<coreId>` and `<coreId>`. That is only of use against its own
  users, given a DNS position, so it is acceptable. Under C the discovery would hold the same right.
- **Verdict on B: reject.** The client names would also sit inside the zone whose isolation D292
  bought.

### 3.3 Option C: a Let's Encrypt certificate per client name, issued to the discovery

- **Cloudflare's free plan cannot proxy with your own certificate.** Custom certificates are a
  Business/Enterprise feature [V, `cf-custom-certs.md`]. C therefore means DNS-only names pointing
  at `prod-api` (or a new host). That gives away the address D290 §8 and protocol §2.9 keep behind
  Cloudflare.
- **CAA conflict:** to issue `<coreId>.<zone>`, the discovery's account must be named in the CAA at
  that node. That node's `issue` also governs `<addr>.<coreId>.<zone>` (non-wildcard names below it).
  So the discovery's account could issue the core's address names without touching DNS.

  The alternatives are a sibling name (`c-<coreId>.<zone>`) with its own CAA node, or a subzone
  `c.orpheum-core.com` with one wildcard and its own CAA. The second avoids per-name limits but is a
  second-level name the free plan cannot proxy.
- **Rate limits** [V, `le-rate-limits.html`]: 50 new certificates per registered domain per 7 days,
  shared with the cores' own certificates, so new cores per week roughly halve. ARI renewals are
  exempt.
- **Verdict: workable but strictly worse than E.**

### 3.4 Option D: per-core origin in an iframe under `app.`

- It still needs a per-core name, so it inherits B, C or E's naming problem.
- Third-party storage is **partitioned** by top-level site in all three engines [V, platform
  behaviour], and Safari's ITP caps script-writable storage. Everything must also be delegated
  (`allow=` for microphone, fullscreen, autoplay, local network access, picture-in-picture), and
  service workers in third-party frames are partitioned.
- Worst for security: a hostile core draws a fake realm sign-in **inside a page whose address bar
  says `app.orpheum-app.com`** (A-4 again).
- **Reject.**

### 3.5 Option E: a new registered domain for client origins (recommended)

- `orpheum-client.com`, at Cloudflare, Universal SSL on, a proxied `*.orpheum-client.com` record to
  `prod-api`.
- **Nothing in `orpheum-core.com` changes:** its apex stays `;`, cores keep `accounturi`, and
  Universal SSL stays off. Cloudflare's hidden CAA lands on a zone where the only certificates are
  Cloudflare's own [V for the behaviour, I for the fit].
- The wildcard record synthesises for every `<coreId>.orpheum-client.com`, since no other nodes
  exist there, so no records per core.
- No cookies tossed from cores' names reach client origins, and none from client origins reach
  cores' names (different sites).
- **What remains:** client origins are same-site *with each other*. M's code at
  `<M>.orpheum-client.com` can cookie-bomb `<H>.orpheum-client.com` through Cloudflare (§3.1).
  Mitigations:
  - the shell is served from the per-origin service worker cache once installed, so navigations need
    no network [I];
  - no client origin ever sets or reads a cookie;
  - a **PSL private-section entry** for `orpheum-client.com`.

  On the PSL [V, `psl-guidelines.html`]: this is the legitimate use (each subdomain a different
  party), and the guidelines refuse only entries "whose sole purpose is to circumvent rate limits".
  They also say *"projects not serving more than thousands of users are quite likely to be
  declined"*, require more than 2 years of registration left, and there is "no way to expedite"
  propagation into browsers. **Plan for it at the public launch, not the beta.**
- **The certificate's trust is Cloudflare's and the discovery's,** as today. Code integrity comes
  from §4.2's manifest.

### 3.6 Option F: status quo plus a compatible client API

- Security is **today's posture**: only our code runs at `app.`. But one origin holds every session,
  so any XSS in our client fed by any core's data (titles, names, subtitles) is all-household. That
  is A-6 limited to one codebase we control.
- E is strictly safer than F, because it makes even our own client's bugs per household.
- F is the right *operational* fallback while E is built. It is not a reason to keep pooled sessions.

### 3.7 Summary matrix

| Threat (beyond M's own users) | A | B | C | D | E | F |
| --- | --- | --- | --- | --- | --- | --- |
| Session, device secret and locator theft across households | **open** | blocked | blocked | blocked | blocked | blocked (our code only); open on an XSS in our client |
| Other households' addresses leaked | **open** | blocked | blocked | blocked | blocked | as above |
| Admin of another household at home | **open** | blocked | blocked | blocked | blocked | as above |
| Phishing under our address bar | **open** | at a zone name (as today) | as B | **open** (framed) | at a client-domain name | n/a |
| Per-core CAA weakened | no | **likely** (Cloudflare injection) | needs CAA changes | depends | no | no |
| Cookie bomb of another household's client origin | n/a | **open** (Cloudflare 128 KB) | open (milder; Go 1 MB) | depends | open within the client domain until a PSL entry | n/a |
| Compromised discovery | total | total | total | total | total (drains per origin, slightly slower) | total |

## 4. The loader, the handoff and code authentication

### 4.1 The handoff

- **Links go straight to the client origin.** The core already builds links from `clientOrigin`
  (`internal/core/api/accounts.go:61-68` [V]). Under E it builds
  `https://<coreId>.orpheum-client.com/#<coreId>.<secret>.<fp>`, so `app.` never sees the fragment.
  The D293 §4 field becomes a template the core fills with its own id, accepted only for that one
  domain.
- **Bind the origin to its core** (missed by the designer; **critical**) [I]. The shell at
  `<X>.orpheum-client.com` must take X **from its hostname** and refuse:
  - a fragment whose `coreId` differs;
  - a remembered core or a realm answer whose `coreId` differs;
  - any path whose answer is signed by a key other than X's pinned or linked one.

  Otherwise a link like `https://<H>.orpheum-client.com/#<M>.<secret>.<M-fp>` loads M's client into
  H's origin, and every per-core benefit is lost. `readLinkFragment` must also check the shape of the
  `coreId` (26 safe32 characters) [V that it doesn't today].
- **No open redirect:** `app.`'s loader must build the target only as
  `https://` + validated `coreId` + `.orpheum-client.com/`, never from a URL in the fragment or a
  query string.
- **Realm sign-in** (D297): the person types realm, name and password once, at `app.`, which runs
  only our code. D297 §2 needs all three to find the core, so the password cannot wait for the
  client origin.
  - Do not put the password or a session token in the redirect's fragment. It lands in session
    history and tab restore, and possibly in synced open tabs [I]. OAuth dropped the implicit flow
    for this.
  - Recommended route: `app.`'s loader finds the core, checks the realm tag (already built,
    `sdk/src/realm.ts`), reaches the core as today, signs in, and asks the core for a **single-use
    handoff code** (about 60 s life, bound to the device id). It hands that code in the fragment and
    keeps nothing. The client origin exchanges it for a session. The token never sits in `app.`'s
    storage.
  - Alternative: the client origin asks for the password again, once per browser.
  - Either way, **`app.` stores no session for any core**.
- **Migration** [I]: existing `orpheum.core.*` keys at `app.` must be **deleted** on the loader's
  first run, not moved. A move would mean passing them through a URL. The old `/sw.js` at `app.` is
  replaced by the loader's, and `app.` never runs core code while legacy keys might exist.

### 4.2 How the client's code is authenticated

- **What the browser trusts** [I]: the client origin's TLS is Cloudflare's, and its shell is
  connectd's. The trusted party is the same as for `app.` today (protocol §3.7).
- **What the shell trusts:**
  - Over WebRTC, the DTLS fingerprint the core signed, checked against the linked or pinned identity
    key [V, `verifyAnswer`]. That is strong.
  - Over a **direct** path, only TLS for `<addr>.<coreId>.orpheum-core.com` [V, protocol §3.5]. That
    is WebPKI plus CAA, so it is exactly as strong as §3.2's CAA. Under B it could fall to "whoever
    holds the address's port 80".
- **Control: a signed client manifest** [I]. The core signs, with its identity key, a manifest of
  the client's build (version, and the SHA-256 of `index`, every chunk, worker and wasm). The shell
  checks the signature against the key it holds for X. The service worker then answers each client
  load only with bytes whose hash matches.

  This makes the code's authenticity independent of the path and of TLS/CAA. It defeats network
  attackers, a poisoned per-origin cache and a mis-issued certificate. It also gives the downgrade
  control something to work on. Vite's 8-character names (about 48 bits) are not hashes to rely on.
- **Does the fingerprint check mean anything when the core supplies the code?** Yes, before the
  client runs: it is what puts X's code, and only X's, into X's origin. After that, the core's client
  is free to do what it likes in its own origin. The loader's "interface" (proposal §2 item 4) is an
  **API convenience, not a security boundary**: script in one origin can bypass it. Don't let the
  design treat it as one.
- **Downgrade and old clients** [I]: under E, an old vulnerable client harms only its own origin.
  The shell can still enforce a **security floor**: the manifest names the version, and the discovery
  publishes a deny-list of client versions or manifest hashes with known holes, which the shell
  refuses to run, with a message to the admin. A benign but stale core is the realistic case: G
  feeds hostile data into H's old client.
- **CSP per origin:**
  - `app.` (the loader only): `script-src 'self'`, `connect-src` the client API and the core names
    it races. No `img`/`media` to cores is needed if the loader shows nothing of a core.
  - `<X>.orpheum-client.com`: the shell's policy, from the discovery's header, never the core's.
    `connect-src` is narrowed to `https://*.<X>.orpheum-core.com:*`, the client API and `wss` the
    same. That is narrower than today's `*.orpheum-core.com` [V], which lets `app.` talk to every core.
  - The worker must never answer a **navigation** with core bytes, so the document's headers stay
    the discovery's.
  - Add `frame-ancestors 'none'`, COOP/CORP same-origin and `Origin-Agent-Cluster: ?1`.
  - CSP does not hold back the core's own code (its bytes are `'self'`). It keeps the shell's update
    and kill-switch channel the discovery's [I].
- **The service worker's power** [I]: at `app.` under A it would be every household's. Under E it is
  per origin; a hostile core can replace or unregister its own origin's worker, which affects only
  its own users. Worker script fetches bypass workers, so the worker script is always connectd's.
- **Phishing:** a malicious core can draw a fake realm form at its own client origin. It can do the
  same at its own names today. Keep realm sign-in **only at `app.`**, let the shell never show a
  realm field, and make `app.` say so. Passkeys later, bound to the RP ID, end this.

## 5. Other findings, independent of the option

| # | Finding | Severity | |
| --- | --- | --- | --- |
| F-1 | Option A gives admin over other households at home (A-3) | Critical (if A is built) | I |
| F-2 | Universal SSL injects unrestricted CAA, likely cancelling `accounturi`; this is option B's hidden cost (§3.2) | High (if B is built) | V (docs) / I (per node) |
| F-3 | The client origin must be bound to its `coreId` by hostname, or one core's code loads into another's origin (§4.1) | High (in B/C/E) | I |
| F-4 | Direct-path code authenticity rests on CAA alone; a signed manifest is needed (§4.2) | High (in B/C/E) | I |
| F-5 | `app.` today pools every household's sessions, so any XSS in our client is all-household (F) | Medium (existing) | V |
| F-6 | **Today:** cores' `SameSite=Strict` media cookies (holding the session token) are sent on same-site loads from *other cores'* pages (no PSL). That allows library XS-leaks, and cookie tossing that breaks images. `media_cookie.go`'s comment assumes cross-site. Fix: the `__Host-` prefix (stops tossing with the same name), CORP `same-origin` and Fetch Metadata checks on `/bytes/` (refuse `same-site` as well as `cross-site`), or drop the cookie once all byte loads use headers or the relay | Medium (existing) | V (code) / I (exploit) |
| F-7 | `app.`'s CSP `connect-src` spans every core; narrow it per origin under E | Low | V |
| F-8 | `readLinkFragment` does not check the shape of `coreId`; once that becomes a hostname it must | Low now, High under E | V |
| F-9 | The viewer membership at `app.` is one key, not per core (contrary to protocol §3.4). The locator and core-id check stops misuse today, so the impact is low | Low (existing) | V |
| F-10 | A compromised discovery stays total under every option. E slows its sweep slightly, since it must navigate the top-level window to each client origin: partitioned iframes see empty storage | Info | I |

## 6. Recommendation: E, with the minimum controls

1. Register `orpheum-client.com` (another domain, as D296 did for staging), at Cloudflare, Universal
   SSL on, DNSSEC, a proxied wildcard to `prod-api`, and `ByHost` routing for the client suffix only.
   **Leave `orpheum-core.com` untouched.**
2. `app.` serves only the loader: realm sign-in, the list of remembered cores (`coreId` and
   fingerprint, no secrets), and the redirect. **No session, device secret or locator of any core is
   stored at `app.`.** Legacy keys are deleted on first run.
3. The client origin's shell binds X from its hostname and refuses any other `coreId` (F-3/F-8).
4. The core signs its client manifest; the shell verifies it, and the service worker serves only
   bytes whose hashes match (F-4). Add a deny-list of client versions the discovery publishes (the
   floor).
5. Links point straight to the client origin. Realm sign-in hands off by a single-use handoff code,
   never a password or token in a URL.
6. The core's CORS and its WebSocket `OriginPatterns` allow **exactly** `https://<own
   id>.orpheum-client.com` (and `app.` only for the handoff endpoint), never a suffix match.
7. Headers on client origins: the shell's CSP (narrowed `connect-src`, `frame-ancestors 'none'`),
   COOP and CORP `same-origin`, `Origin-Agent-Cluster: ?1`, HSTS. The worker never answers
   navigations with core bytes.
8. No cookies on client origins. Fix F-6 on cores' names (`__Host-`, CORP, Fetch Metadata).
9. A PSL entry for `orpheum-client.com` at the public launch, when the user count can support it.
10. A `TestArch_` claim for "no storage key at `app.` holds a core credential", and one for "the
    shell loads only the core its hostname names" (rule 10).

Use F (keep the client↔core API compatible across a window) as the bridge while E is built. Its
risk is today's.

## 7. Questions for the owner

1. **A new registered domain** (`orpheum-client.com` or similar), given that B's Universal SSL
   probably cancels the per-core CAA that D292 §1 and D293 §10 chose?
2. If B is still wanted: may the agent **test Cloudflare's hidden CAA on a throwaway zone** (a
   subdomain with an `accounturi` CAA, Universal SSL on, `dig CAA`) before anything is decided?
3. **Realm hand-off:** a single-use handoff code minted by the core (a new core endpoint), or the
   password asked again at the client origin once per browser?
4. **A signed client manifest and a security floor** (the deny-list): accepted as part of D308's
   "checked before it runs"?
5. **Existing sessions at `app.`:** deleting them means everyone signs in once more after the change.
   Acceptable?
6. **F-6** (same-site media cookies between cores): fix now, independent of D308?
7. **PSL:** apply at the public launch, accepting that until then client origins can cookie-bomb each
   other (a denial of service within one browser, not theft)?

## Sources (cached in `/yoloai/cache/client-from-core-audit/`)

- Cloudflare, [Add CAA records](https://developers.cloudflare.com/ssl/edge-certificates/caa-records/)
  (`cf-caa.md`); [Universal SSL](https://developers.cloudflare.com/ssl/edge-certificates/universal-ssl/);
  [Custom certificates](https://developers.cloudflare.com/ssl/edge-certificates/custom-certificates/);
  [Connection limits](https://developers.cloudflare.com/fundamentals/reference/connection-limits/)
  (128 KB request headers); [Advanced Certificate Manager](https://developers.cloudflare.com/ssl/edge-certificates/advanced-certificate-manager/).
- [Let's Encrypt community: certificate issued despite CAA](https://community.letsencrypt.org/t/certificate-issued-despite-caa-record/174583)
  (the hidden records shown by `host -t CAA`); Cloudflare community, "Cloudflare nameservers ignore CAA
  record validationmethods and accounturi" (thread 758109, seen in search results; 403 to fetching);
  [zenodo 18330221](https://zenodo.org/records/18330221).
- [RFC 8659](https://www.rfc-editor.org/rfc/rfc8659) §3 (relevant RRset, climbing, issuewild);
  RFC 8657 (`accounturi`, `validationmethods`); RFC 4592 (wildcards).
- [Let's Encrypt rate limits](https://letsencrypt.org/docs/rate-limits/) (50 per registered domain
  per week; ARI renewals exempt).
- [PSL guidelines](https://github.com/publicsuffix/list/wiki/Guidelines) (the size bar, 2-year
  registration, no expediting, no rate-limit motive).
