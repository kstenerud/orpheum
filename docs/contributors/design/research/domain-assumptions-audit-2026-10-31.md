> **ABOUTME:** An outside audit of where Orpheum, orpheum-connect, orpheum-site and orpheum-ops
> assume domain names, ahead of D390's move: every hard-coded name classified, how the core, the web
> client and connectd each learn their discovery today, a design in which the app knows no domain
> and learns its discovery securely (an issuer, pinned at enrolment, and a `.well-known` document),
> checked against OAuth/OIDC, WebAuthn, CORS and the cloud relays of Plex, Home Assistant,
> Tailscale and Bitwarden; one source of truth for the paid service's names; and an ordered change
> list. It proposes; it decides nothing.

# Where the code assumes domains, and how it could know none

- **Date:** 2026-10-31, on Orpheum an earlier commit; orpheum-connect, orpheum-site and orpheum-ops as
  checked out beside it the same day. **Asked by** the owner, before D390's migration, with three
  requirements: *"Ideally the orpheum app shouldn't need to know anything, and be able to figure out
  which discovery server it was launched from (make sure this can be done securely). Maybe it might
  need an email address pointer, or a link to the homepage, which is fine."*; *"The paid discovery
  server might need to reference specific domains, and if it's necessary, then those should be
  encoded in one source of truth rather than scattered about."*; *"Make sure this is checked against
  expert opinion and best practices."*
- **Read:** D390 and the [domains audit](domains-audit-2026-10-31.md) (this report builds on its
  layout and does not repeat its site, mail and TLD findings); D204, D273, D275, D293 §4, D297,
  D308, D334 in the working notes; in Orpheum `internal/core/discovery` (`discovery.go`, `urls.go`,
  `status.go`), `internal/core/api` (`cross_origin.go`, `discovery.go`, `accounts.go`),
  `cmd/orpheum/pair.go`, `internal/deployment/templates/install.sh`, `discovery/proof`,
  `discovery/single`, `web/apps/web/src` (`discovery-start.ts`, `loader-config.ts`,
  `loader-main.tsx`, `loader-households.ts`, `client-reaches.ts`, `system-discovery.tsx`, `sw.ts`);
  in orpheum-connect `cmd/connectd` (`config.go`, `open.go`, `main.go`), `internal/origins`,
  `internal/webclient`, `internal/api/cors.go`, `internal/panel` (pages, CSP), `internal/core/auth`,
  `internal/webauthn`, `deploy/production` and `deploy/staging`; orpheum-site whole; orpheum-ops'
  READMEs and scripts.
- **Searched for:** `orpheum-app.com`, `orpheum-client.com`, `orpheum-core.com`, `orpheum-test.com`,
  `orpheum-client-test.com`, `orpheum.cc`, `orpheum.dev`, `orpheum.mov`, `example`, and the code that
  derives a host (`ServiceURL`, `passkeySite`, CORS, CSP, `ByHost`, meta tags, mail, cookies,
  service-worker scope, `postMessage`).
- **Markers:** **[V]** verified against the code or a source listed at the end; **[I]** inferred;
  **[U]** unverified.

## 0. The plain answer

**The code is already close to knowing nothing.** connectd takes every name it serves from flags,
with no domain default; the core learns its client and loader origins from its discovery's status;
the web client learns its discovery from meta tags in the page its own origin served; the passkeys
are the only thing whose name is *derived* rather than told **[V]**. What is left is small:

- **In production code: 12 sites** (§1.1): 9 hard-coded names shown to people (Orpheum 2, connect
  7), 2 defaults (Orpheum: one URL, written twice) and 1 derivation (connect's RP ID), plus two
  configuration defects. **Two are security-relevant**: the RP ID derived from the account host (`cmd/connectd/open.go:205`), and the
  paid discovery's URL compiled into the core **twice**, once in Go and once in TypeScript
  (`internal/core/discovery/discovery.go:38`, `web/apps/web/src/system-discovery.tsx:27`). The rest
  are words shown to people, and one of them is wrong today: **staging's account pages tell testers
  to sign in at production's `app.orpheum-app.com`** (`internal/panel/pages/home.html:14`, `join.html:14`,
  `signin.html:4`).
- **Configuration: the real scatter.** Each environment's names sit in its compose file as eight
  flags, one repeated as an environment variable, again in Cloudflare's dashboard (the tunnel's
  routes, which no repository holds), again in orpheum-ops' scripts and runbooks, and again in prose
  (§1.2).
- **The top risks are not names but trust** (§3): the core opens its CORS to, and points every
  share link (with its secret in the fragment) at, whatever origin its discovery names, any HTTPS
  origin at all; the password realm sign-in on the typed origin is phishable by any lookalike that
  hosts the open-source loader, which only passkeys stop; and a core's signatures are bound to its
  discovery's API URL, so that URL can never move without every core re-enrolling.

**Recommended design** (§4): a discovery's identity is an **issuer**, the HTTPS origin a person
types (`https://orpheum.cc`, or `https://media.example.org` for an own discovery). The issuer serves
`/.well-known/orpheum-discovery`, a small JSON document naming its core API, client API, loader
origin, client-origin pattern, account origin, homepage and support address. The core is given the
issuer once (by the install script, the System page or the account page's command), fetches the
document over HTTPS, refuses it unless its `issuer` is exactly the one asked (RFC 8414 §3.3), and
pins it in `discovery.json`; from then on it accepts from its discovery only the origins the pinned
document names. The web client keeps learning from its own origin's page, which is already the
right trust root. Orpheum keeps **one** pointer, the suggested issuer and homepage, in one Go file,
served to the web client instead of copied into it. connectd reads all of its names from **one names
file per environment**, from which every flag, header, policy, page text, ops script and tunnel
checklist derives, with tests over both files that keep staging and production apart.

**For D390's staging rehearsal** only seven small changes are needed (§6, marked ●): the RP ID as a
setting, the panel's words from configuration, the support address from configuration, redirects for
the old and `www.` hosts, HSTS from configuration, staging's names, and the install script's last
line. The issuer document is the right end state but is **not** needed for the move: it belongs
before the self-deployed discovery ships and before the public launch.

## 1. Inventory

Counts are of distinct sites (a file and line, or a block); repeated mentions in one passage count
once. Third-party hosts (Cloudflare's API, Let's Encrypt, TMDB, OpenSubtitles, Google's cast SDK)
are out of scope: they are providers, not Orpheum's names.

| Kind | Orpheum | connect | site | ops | Total |
| --- | --- | --- | --- | --- | --- |
| (a) production code hard-codes a name | 2 (O3, O4) | 7 (C1–C7) | 0 | 0 | **9** |
| (a′) a host derived where it should be told | 0 | 1 (C10, the RP ID) | 0 | 0 | **1** |
| (b) configuration | 1 (`dev` docker label, not a host) | 2 compose files (~30 lines; C9 a defect), `up.sh` (C8) | Cloudflare Pages custom domain (dashboard) | 3 scripts | — |
| (c) default value | 2 (O1, O2: one URL, twice) | 0 | 0 | 0 | **2** |
| (d) test fixture | 25 files | 6 files | 0 | 0 | 31 files |
| (e) documentation, comments, help text | 42 files (~350 mentions), 9 code comments | 10 docs (~195), 3 comments | README | 5 READMEs | — |

### 1.1 Production code (a, a′, c)

| # | File:line | Name | What it does | Kind | Should be |
| --- | --- | --- | --- | --- | --- |
| O1 | `internal/core/discovery/discovery.go:38` | `https://api.orpheum-app.com/core/v1` | `ServiceURL`, the default of `orpheum pair --discovery` (`cmd/orpheum/pair.go:37`), which the install script's pairing uses with no way to override it | (c) | the one Orpheum pointer (§5.2); later an issuer (§4) |
| O2 | `web/apps/web/src/system-discovery.tsx:27` | the same URL | `serviceDiscoveryUrl`, the System page's prefilled address (line 83) | (c) | read from the core, not copied: two sources of one value break rule 11 |
| O3 | `internal/deployment/templates/install.sh:432` | `app.orpheum-app.com` | "from away: sign in at … with the realm" after pairing | (a) | the loader origin the discovery named, which `pair` can print |
| O4 | `web/apps/web/src/locales/en-lab.ts:41` | `https://192-168-1-10.….orpheum-app.com:8443` | a lab page's placeholder | (a) | a reserved example name; it is also wrong (home names are under the core zone, not `orpheum-app.com`) |
| C1 | `internal/core/auth/accounts.go:149` | `support@orpheum-app.com` | a refusal's text | (a) | from configuration (§5.1) |
| C2 | `internal/core/auth/corekeys.go:184` | `support@orpheum-app.com` | a refusal's text | (a) | same |
| C3 | `internal/core/auth/reset.go:133` | `support@orpheum-app.com` | a refusal's text | (a) | same |
| C4 | `internal/panel/pages/join.html:14` | `app.orpheum-app.com` | "the name your household signs in … at" | (a) | the loader's host from configuration; **wrong on staging today** |
| C5 | `internal/panel/pages/home.html:14` | `app.orpheum-app.com` | "Your household signs in … at" | (a) | same |
| C6 | `internal/panel/pages/home.html:17` | `app.orpheum-app.com` | realm chooser's text | (a) | same |
| C7 | `internal/panel/pages/signin.html:4` | `app.orpheum-app.com` | sign-in page's text | (a) | same |
| C8 | `deploy/staging/up.sh:14,21` | `dns-edit-orpheum-test.com.token` | a token file's name | (a) | harmless: names the zone the token reaches; may stay |
| C9 | `deploy/production/compose.yaml:52`, `deploy/staging/compose.yaml:54` | `noreply@orpheum-app.com` | `-mail-from`; **staging sends as production's domain** | (b) but listed as a defect | staging's own sender, or the outbox only |
| C10 | `cmd/connectd/open.go:204–205` | RP ID = `hostOf(accountOrigin)` | `passkeySite`: the passkeys' relying party follows the account host | (a′) | an explicit setting (D390 §3), with an exact list of accepted origins |

Everything else in connectd that names a host takes it from a flag **[V]**: `-core-api`,
`-client-api`, `-zone`, `-dns-zone`, `-client-origin`, `-client-origins`, `-account-origin`,
`-mail-from` (`cmd/connectd/config.go:90–153`), none with a domain default. From them derive the
CORS allow-list (`internal/api/cors.go`, through `internal/origins`), the loader page's CSP
`connect-src` (`internal/webclient/webclient.go:66`), its meta tags (lines 91–93), the routing by
`Host` (`cmd/connectd/main.go:158–171`), the cores' client and loader origins in their status
(`internal/cores/held.go:41`), and the mail's links (`auth.Parts.PagesOrigin`). The panel's CSP is
`'self'`-only and its links relative (`internal/panel/panel.go:101`, pages) **[V]**.

In Orpheum, nothing in the core or the web client names a discovery except O1–O4 **[V]**: the core's
CORS takes the discovery's origins from its status (`internal/core/api/cross_origin.go:34–36,
59–89`), its share links the discovery's client origin (`internal/core/api/accounts.go:64–71`),
the device-code address the loader's host from the page (`client-reaches.ts:44`), and the web client
has no cross-origin `postMessage` (all its messages are page↔worker and `MessageChannel` within one
origin) **[V]**. Comments and help text that use real names as examples: `discovery/discoveryproto/doc.go:4`,
`web/apps/web/src/loader-config.ts:11,14,27`, `services.tsx:49`, `internal/core/certificate/certificate.go:105`,
`internal/core/api/media_cookie.go:25`, `cmd/orpheum/config.go:304` (the `https.domain` flag's help);
in connect `internal/panel/doc.go:3`, `internal/webauthn/webauthn.go:45,47`.

### 1.2 Configuration (b)

| Where | Names | Note |
| --- | --- | --- |
| connect `deploy/production/compose.yaml:19,24,25,28,29,38,40,49,52` | API (twice), core zone (twice), loader, client pattern, account (twice: flag and `CONNECTD_ACCOUNT_ORIGIN`), mail-from | eight facts, eleven lines |
| connect `deploy/staging/compose.yaml:24,25,26,27,37,42,51,54,61` | the same for staging | staging's mail-from is production's (C9) |
| Cloudflare dashboard: each tunnel's public hostnames | `api.`, `app.`, `account.`, `*.orpheum-client.com` (production); the `-staging` names and `*.orpheum-client-test.com` | **in no repository**; described by hand in ops `production/README.md:62–63,382,514` and `staging/README.md:41` |
| Cloudflare dashboard: Pages custom domain | `orpheum-app.com`, `www.` | described in orpheum-site `README.md:21–22` |
| ops `production/update-prod-api.sh:23–24` | `https://api.orpheum-app.com/health` | the deploy's check |
| ops `staging/rebuild.sh:30` | `DISCOVERY=https://api-staging.orpheum-app.com/core/v1` | staging's core enrols here |
| ops `edge/orpheum.nft:8` | `media.orpheum-app.com` (comment) | legacy, retires with `media.` |
| Orpheum `dev:85–86` | `orpheum.dev` | a Docker label namespace, not a host; the owner now owns the name, which makes it a correct reverse-DNS label |

### 1.3 Test fixtures (d)

Orpheum: 25 test files use real names, mostly `media.orpheum-app.com` and `*.k7xq2.orpheum-app.com`
for certificates (`cmd/orpheum/certificates_test.go`, `https_test.go`, `internal/core/certificate/certificate_test.go`,
`internal/core/providers/cloudflare/client_test.go`), `app.orpheum-app.com` as a discovery's origin
(`internal/core/api/link_test.go:24`, `scope_test.go:49`, `web/apps/web/src/system-discovery.test.tsx:88`,
`kept-path.test.ts:290`, `sw.test.ts:578`, `device-activation.test.tsx:186`), `api.orpheum-app.com/core/v1`
in the proof tests (`discovery/proof/proof_test.go:17`, `nonces_test.go:16`), `*.orpheum-core.com` home
names (`kept-path.test.ts`, `app-shell.test.tsx:81`), `api-staging.orpheum-app.com` (`discovery-gate.test.tsx:92`).
connect: `staging.orpheum-core.com` in `internal/cores/*_test.go`, `cmd/connectd/claim_test.go:74`, and
`noreply@orpheum-app.com` in `internal/mail/mail_test.go:42,51`.

None tests a production name *as such*; each uses one as a plausible string. They need not change for
the move. They should drift to reserved names (`.example`, `.test`; RFC 2606, RFC 6761) when touched,
so that a grep for a real name finds only real uses, which is how this audit had to be done by hand.

### 1.4 Documentation (e)

Orpheum: 42 files, about 350 mentions, most in design history (D273, D292, D296, D308 and the plans
that cite them), which is history and should not be rewritten; the living ones are
`deploy/vps/README.md`, `deploy/staging/README.md`, `deploy/release/README.md:85`, the discovery
protocol and `client-from-core.md`. connect: 10 docs, about 195 mentions (`docs/spec.md` §2,
`docs/plans/accounts.md`). ops: the production and staging runbooks and the top README. site: its
README's title and Pages steps; `index.html` names no domain at all **[V]**.

## 2. How each piece learns its discovery today

### 2.1 The core

1. **Default:** `discovery.ServiceURL`, compiled in (O1). The System page offers its own copy (O2).
2. **Given:** `POST /system/discovery {baseUrl}` by an admin session (`internal/core/api/discovery.go:155–167`),
   or `orpheum pair --discovery`, whose default is O1; the install script runs `pair` without the flag
   (`install.sh:406–407`), so an installed core can only pair with the paid service. The URL must be
   HTTPS, or HTTP on this machine (`internal/core/discovery/urls.go:30–40`) **[V]**. Connecting needs
   an admin session but not the admin's password, which only the claim asks for (lines 98–113) **[V]**.
3. **Pinned:** the base URL and the core's id in `discovery.json` in the persistent directory; a
   restart carries on with them (`discovery.go:33–35`) **[V]**. Every signature the core makes to the
   discovery covers that base URL (`discovery/proof`, "a signature to a discovery serves only that
   discovery and that core") **[V]**: this is audience binding, the property RFC 9207's `iss` and
   RFC 8707's resource indicators give OAuth.
4. **Learnt from the discovery afterwards:** its client origin and loader origin, from every status
   answer (`discovery.go:473–474`), kept if each is an HTTPS origin and nothing else (`urls.go:17–25`)
   **[V]**. These then decide the core's CORS (`cross_origin.go`) and where its share links point
   (`accounts.go:64–71`). The core has no independent statement of which origins its discovery may
   name: whatever the API at the pinned URL answers is believed.

### 2.2 The web client

- **At a discovery's origins** (the loader and each core's client origin), connectd serves Orpheum's
  built client with three meta tags injected into `<head>`: `orpheum-discovery-client-api`,
  `orpheum-client-origins`, `orpheum-loader-origin`, plus `orpheum-discovery-realms` on the paid
  service (`internal/webclient/webclient.go:91–96`) **[V]**. The client reads them
  (`discovery-start.ts:43`, `loader-config.ts:34–44`) and tells where it is by comparing its own
  `location.origin` with the pattern (`loader-main.tsx:63`). The tags arrive in the same response as
  the script that reads them, from the same origin, under a CSP that allows only `'self'` scripts:
  **their trust is exactly the origin's, which is the right trust root** — nothing another origin
  says can reach them [I, from the same-origin policy].
- **At a core's own origin** (home, `this machine`), no meta tag exists, so no discovery code runs
  (`discoveryClientApiOf` answers `""`) **[V]**; the System page shows the core's discovery from the
  core's API.
- **Between origins:** the loader takes a household into its list only when the request's referrer is
  that household's own client origin, built from the pattern (`loader-households.ts:199–220`, D308
  §8) **[V]**; links are built from the pattern and a core id, never from a URL a page was given
  (`loader-config.ts:62–64`) **[V]**. The service worker is registered at client origins only; the
  loader unregisters any at its own origin (`loader-main.tsx:63–72`) **[V]**.

### 2.3 connectd

connectd *is* the discovery: it is told its names by flags (§1.1) and derives everything else from
them, except the RP ID, which it derives from the account host (C10). It serves a request by its
`Host`: the account host to the panel, the loader host and any client-origin host to the web client,
and **every other host to the API** (`cmd/connectd/main.go:158–171`) **[V]**. So after the move a
request for an old name that still reaches connectd (the old `app.` or `account.`, or `www.orpheum.cc`
if routed) gets the API's 404, not a redirect, unless Cloudflare redirects it first.

### 2.4 The self-deployed discovery

`discovery/single` takes its base URL, zone, client origin and loader origin as parts, from `orpheum
discovery`'s flags **[V]**, and answers the same status. It needs nothing from this report except
whatever the issuer document (§4) adds, which it should serve too, from the same public type.

## 3. Risks, ranked

| # | Risk | Real for the beta? | Today's defence | Missing |
| --- | --- | --- | --- | --- |
| R1 | **The discovery names any origin, and the core believes it.** A misconfigured, compromised or hostile discovery (including one an admin was talked into connecting to) can name an attacker's origin as the client origin: the core then lets that origin call its API from browsers and **points every share, invitation and Watch Together link at it, with the link's secret in the fragment** (`accounts.go:68`). | Yes, but bounded: it needs the discovery's API (or its configuration) to lie, or an admin to connect to the wrong one. | HTTPS to the pinned URL; the link's fingerprint lets a device check the core, but not the page the link opens. | An independent statement of which origins this discovery serves (§4.3); a change of named origin shown to admins; the password asked on Connect, as on Claim. |
| R2 | **The realm sign-in is a password typed at the typed origin**, and the loader is open source, so `orpheum.co` (registered by someone else, domains audit §3.5) can host a faithful copy whose meta tags point at the phisher's own API. The core's CORS refuses that origin, but a phisher relays through its own server. | Yes: it is the most valuable credential typed on the most guessable name. | Typing once, then bookmarks, QR and device codes (domains audit §3.5). | Only an origin-bound credential defeats it: a passkey under RP ID `orpheum.cc` cannot be exercised at `orpheum.co` (WebAuthn's RP ID rule). Origin checks, CORS and `postMessage` checks cannot. |
| R3 | **The RP ID follows the account host** (C10): the move would make it `account.orpheum.cc` silently and for good. | Yes, until the first passkey exists; then it cannot be undone. | None. | D390 §3: an explicit setting and an exact origin list. |
| R4 | **A core's signatures are bound to its discovery's API URL**, so the URL cannot move without every core re-enrolling; there is no "moved" statement. | Not now: D390 keeps `api.orpheum-app.com`. Staging's move does re-enrol staging's core (§6). | Keeping the API's name. | A way to move, for the provisional name's sake (D195): §4.5. |
| R5 | **Names drift between copies.** Staging's pages already send testers to production's loader (C4–C7); staging already mails as production's domain (C9); the tunnel's routes exist only in a dashboard. | Yes: the move multiplies copies by two environments. | Review. | One names file per environment, tests over it (§5). |
| R6 | **Unknown hosts fall through to the API** (§2.3), and HSTS is `max-age` only, with no `includeSubDomains` or `preload`, on the web client and none from the panel (`webclient.go:105`; the panel sets none) **[V]**. | Yes at the move: preloading `orpheum.cc` needs the apex to send `includeSubDomains; preload` (hstspreload.org). | Cloudflare's edge settings, if set by hand. | Redirect hosts and HSTS from the names file. |
| R7 | **The household list is per loader origin.** The list lives in the loader origin's storage; at `orpheum.cc` it starts empty. | Yes, a one-time cost. | Client origins are unchanged, so sessions survive, and visiting one's client origin puts the household back (D308 §8's referrer rule). | A sentence in the move's notice; no transfer (a fragment that fills the list would undo D308 §8). |
| R8 | **Cloudflare terminates TLS** for the API, the loader and the account pages, so it could rewrite any statement a discovery makes. | No new risk: it already serves the loader's script, which a forged statement could not beat. | Contract and account security. | Signed statements would defend only the core↔discovery API, not the browser; overkill now (§4.6). |

Not risks, checked: cookies (the service sets only `__Host-` cookies on the panel, the loader and
client origins none; domains audit §1.3); service-worker scope (client origins only, `/`); cross-origin
`postMessage` (none); CORS reflection (both connectd's and the core's allow exact origins, never `*`
or `null`, with `Vary: Origin`, as OWASP asks).

## 4. Design: the app knows nothing, and learns its discovery securely

### 4.1 Principle

**A discovery is identified by an issuer, an HTTPS origin a person can type, and everything else
about it is discovered from that issuer and pinned at enrolment.** This is how OAuth 2.0 and OpenID
Connect solve the same problem: a client given an issuer fetches
`/.well-known/oauth-authorization-server` (RFC 8414) or `/.well-known/openid-configuration`, and *"the
issuer value returned MUST be identical to the authorization server's issuer identifier value into
which the well-known URI string was inserted … If these values are not identical, the data contained
in the response MUST NOT be used"* (RFC 8414 §3.3). Matrix does the same for a homeserver
(`/.well-known/matrix/client` names the API's `base_url`). The issuer is the identity; the API's URL
is a property of it.

### 4.2 The document

Served by every discovery at its issuer, `GET https://orpheum.cc/.well-known/orpheum-discovery`
(RFC 8615 well-known URI; the type in `discovery/discoveryproto`, so connectd and `discovery/single`
serve the same thing, D275 §1):

```json
{
  "issuer": "https://orpheum.cc",
  "name": "Orpheum",
  "coreApi": "https://api.orpheum-app.com/core/v1",
  "clientApi": "https://api.orpheum-app.com/client/v1",
  "loaderOrigin": "https://orpheum.cc",
  "clientOrigins": "https://{core}.orpheum-client.com",
  "accountOrigin": "https://account.orpheum.cc",
  "homepage": "https://orpheum.mov",
  "support": "support@orpheum.cc"
}
```

For an own discovery: `issuer` and `loaderOrigin` its one name, `clientOrigins` without `{core}` (one
household), no `accountOrigin`. Rules: every URL HTTPS; `loaderOrigin` is the issuer (the typed name
*is* the loader, D390 §1), which also makes the document unforgeable by anything but that origin's
server; no secrets; cacheable for an hour.

### 4.3 The core

1. **Given an issuer, once.** The admin types `orpheum.cc` (or pastes the account page's command, or
   the install script passes `--discovery orpheum.cc`); the core normalises it to `https://orpheum.cc`,
   fetches the document, checks `issuer` is exactly that, `coreApi` HTTPS, and shows *"Connect to
   Orpheum at orpheum.cc?"* with its homepage before enrolling. The admin's password is asked here as
   at the claim (R1): rebinding a core's discovery is as sensitive as claiming it.
2. **Pinned.** `discovery.json` keeps the issuer and the document, beside today's base URL. Signatures
   keep binding `coreApi` (no protocol change).
3. **Statements checked against the pin.** A status's `clientOrigin` must be `clientOrigins` with the
   core's id, and its `loaderOrigin` the document's. If one differs, the core fetches the document
   again from the issuer; if it still differs, it keeps the last good origins and says so on the
   System page. This closes R1 against a lying or misconfigured API host and makes a move of the
   loader (D390's very move) a change at one place, the issuer's document, which every core follows
   within its refresh. It does not defend against a compromise of connectd as a whole (it serves
   both), which nothing short of a key outside connectd would; that is §4.6.
4. **Existing cores need no admin.** The status answer gains `issuer`; a core with a base URL but no
   pinned issuer fetches the document from it and pins it **only if** its `coreApi` equals the base
   URL it already holds. A discovery cannot move a core this way, only describe itself.
5. **Standalone stays empty.** No issuer, no document, no discovery code runs, as today.

### 4.4 The web client

- **Keep the meta tags.** Same-origin, CSP-guarded configuration is the strongest statement a page can
  have; fetching the well-known document from the page's own origin would add a request and nothing
  else. Two small hardenings: the loader should refuse to start if `orpheum-loader-origin` is not its
  own `location.origin` (a misconfiguration today passes silently) [I]; and a fourth tag,
  `orpheum-homepage`, gives the loader the "what is Orpheum" link a stranger needs at the apex
  (domains audit §2), from the discovery and not from Orpheum's code.
- **The System page** (served by the core) shows the pinned issuer and name, not a URL, and offers the
  suggested issuer (§5.2) from the core's API instead of its own copy (O2).
- **Never accept a discovery from a page.** No query parameter, fragment or `postMessage` may tell the
  client or the core which discovery to use; only the admin, at the core, through its own
  authenticated API. (Today's code already holds to this **[V]**; the invariant deserves a test in
  `internal/core/api` once §4.3 lands.)
- **Native apps later (D273 §4):** key every stored thing by issuer, and send the issuer with each
  request a core answers through a discovery, which is RFC 9207's lesson for a client that may know
  several authorisation servers. A browser loader cannot mix discoveries up, since each origin is one.

### 4.5 Moving an API later (R4)

With the issuer pinned, an API move becomes: the issuer's document names a new `coreApi`; the core
sees it at a refresh, re-signs a session to the new URL (the protocol already does a session per
discovery URL), and connectd accepts both URLs for the move's window. Not needed for D390; worth
reserving the field's meaning now.

### 4.6 What is overkill for the beta

- **A discovery signing key, pinned at enrolment (TOFU), with signed statements.** It would defend the
  core against a compromised edge or API host that the issuer's TLS does not; but the browser side is
  served by the same edge, so the household gains little, and key management (rotation, loss) is
  exactly what made HPKP *"too much of a footgun"* (Chrome's deprecation, 2017–2019). Revisit for the
  public launch if the API leaves Cloudflare's proxy or the core starts trusting statements for more
  than routing.
- **Certificate or public-key pinning of the discovery's TLS.** Same lesson; WebPKI plus CT is the
  accepted baseline (Chrome removed HPKP in version 72).
- **Mutual TLS between core and discovery.** The core already signs every call with its identity key
  bound to the discovery's URL, which is the same property without a CA.

### 4.7 Against the field

| Product | How a device learns its cloud | Comparable to |
| --- | --- | --- |
| **Tailscale** | `controlplane.tailscale.com` compiled in; `--login-server` overrides it for Headscale; the client keeps it in its state | today's Orpheum exactly (O1 + `pair --discovery` + `discovery.json`) |
| **Bitwarden** | clients offer `bitwarden.com`, `bitwarden.eu` or "Self-hosted" with a typed server URL | the System page's choice, with a suggested service |
| **Plex** | `plex.tv` compiled in; a server is claimed by a 4-minute single-use token from `plex.tv/claim`; per-server certificates `*.<hash>.plex.direct` | the account key (D310) and `<addr>.<id>.orpheum-core.com` (D292) |
| **Home Assistant (Nabu Casa)** | the cloud's endpoints compiled into `hass-nabucasa`; the remote UI is an SNI proxy that does not terminate TLS, the certificate's key only on the instance | stronger than Orpheum's direct path in one way (the relay sees nothing), which Orpheum matches by carrying no traffic (D273 §2) |
| **Jellyfin** | no cloud; the person types the server's address; Quick Connect is a sign-in code, not discovery | the standalone mode |
| **Matrix, OIDC** | the typed name is an issuer; a `.well-known` document names the API; the issuer is checked | the recommended design (§4.2) |

**Verdict:** a compiled-in default with an override is the industry norm and is not a weakness in
itself, because the default is resolved over HTTPS and then pinned. What Orpheum adds by moving to an
issuer is not security against outsiders so much as **one place to change a name** (the loader moves
by editing a document, not by releasing cores) and **a check that the discovery's API speaks only for
its own origins** (R1). The owner's "knows nothing" is reachable without cost to security; "knows one
pointer, in one file" is equally sound and is what Tailscale and Plex ship.

## 5. One source of truth

### 5.1 The paid service (orpheum-connect)

**A names file per environment, read by connectd, from which everything else is derived.**
`deploy/production/names.conf` and `deploy/staging/names.conf`, plain `key = value` lines (no
interpolation, no environment), passed to connectd as `-names /etc/connect/names.conf` (a config file
read by flag, which keeps D204's spirit that a binary takes nothing ambient):

```
issuer          = https://orpheum.cc
www             = https://www.orpheum.cc            # redirects to issuer
account         = https://account.orpheum.cc
api             = https://api.orpheum-app.com      # core API = api + /core/v1, client API = api + /client/v1
client-origins  = https://{core}.orpheum-client.com
core-zone       = orpheum-core.com
dns-zone        = orpheum-core.com
passkey-rp-id   = orpheum.cc
mail-from       = noreply@orpheum.cc
support         = support@orpheum.cc
homepage        = https://orpheum.mov
retired         = https://app.orpheum-app.com=https://orpheum.cc, https://account.orpheum-app.com=https://account.orpheum.cc
hsts-preload    = yes
```

| Derived from it | Today |
| --- | --- |
| `-core-api`, `-client-api`, `-zone`, `-dns-zone`, `-client-origin`, `-client-origins`, `-account-origin`, `-mail-from` | eight flags in each compose file, one repeated as an environment variable |
| CORS allow-list, the loader's CSP `connect-src`, the meta tags (with `orpheum-homepage` added) | from the flags already **[V]** |
| RP ID and the accepted origins (`account`, later `issuer`) | derived from the account host (C10) |
| The panel's words (C4–C7), the support address (C1–C3), the mail's links and sender | hard-coded, or flags |
| `Host` routing: redirects for `www` and `retired`, 308 with path and fragment kept; HSTS with `includeSubDomains; preload` at the issuer when `hsts-preload` | unknown hosts fall to the API (R6) |
| `/.well-known/orpheum-discovery` at the issuer (§4.2) | none |
| `connectd names hosts`: every public hostname the tunnel must route, one per line; `connectd names check`: each answers over HTTPS, the apex sends no `Set-Cookie`, HSTS as configured, the non-sending domains carry null MX, SPF `-all` and DMARC `reject` (domains audit §5) | the dashboard and two runbooks, by hand |

**Tests over the two files** (in connect, plain Go): staging's and production's names share no
registrable domain (the domains audit's §4.3, D390 §5); `passkey-rp-id` is a registrable suffix of
`account` and of `issuer`; `api`, `client-origins` and `core-zone` are not under the RP ID's site
(domains audit §1.5–1.6); `mail-from`'s domain is the RP ID's site in production; staging's
`mail-from` is not production's (C9). The registrable domain needs the Public Suffix List; Go's is
`golang.org/x/net/publicsuffix`, a dependency the owner would have to allow (rule 3); without it, the
test can hold a short list of the suffixes used (`cc`, `dev`, `mov`, `com`), which is enough for these
files and honest about being so.

**Who else reads it:**
- **orpheum-ops** scripts read the file by its sibling path (`../orpheum-connect/deploy/production/names.conf`):
  `update-prod-api.sh`'s health URL is `api` + `/health`, `staging/rebuild.sh`'s `DISCOVERY` is
  staging's `issuer` (once §4.3 lands) or `api` + `/core/v1` (until then). A ten-line shell reader is
  enough; the runbooks name keys, not hosts, where they can.
- **The tunnels' routes** stay in Cloudflare's dashboard (a remotely managed tunnel keeps them there),
  but `connectd names hosts` prints what they must be and `names check` fails when one is missing.
  Moving to a locally managed tunnel configuration, generated from the file, is possible later; not
  needed now.
- **orpheum-site** links to the app and nothing else; one hand-written link (`https://orpheum.cc`) in
  a one-page site is cheaper than a build step. The site is a separate supply chain on its own
  domain (domains audit §3.2), and should read nothing from connect.

### 5.2 What Orpheum keeps

**One Go file, the only place Orpheum names its paid service:** for example
`internal/core/discovery/suggested.go` with the suggested issuer (`https://orpheum.cc` once §4.3
lands; until then the API URL that is O1) and the homepage. Everything else reads it:

- `orpheum pair --discovery`'s default and the install script (which passes nothing, and prints the
  loader origin the discovery named instead of O3's literal);
- the System page, through the core's discovery status (a `suggested` field), so O2 disappears;
- no support address: a core's people write to their own admin; the service's support address is the
  service's to show, from its names file.

A fork or a household that wants none sets nothing: standalone needs no suggestion, an own discovery
types its own issuer. Whether even the suggestion should go (the account page's command carrying
`--discovery orpheum.cc`, and the System page asking with no default) is the owner's choice; it costs
one more thing to type at the one moment a person is most likely to make a mistake, and buys
nothing in security (§4.7), so this report recommends keeping the suggestion.

## 6. Change list

In order. ● = needed before D390's staging rehearsal (the audit's step 4); ◐ = advised before it, so
the rehearsal proves it; ○ = later, with when. Sizes: XS under an hour, S a few hours, M a day or two.

| # | Repo | Change | Size | When |
| --- | --- | --- | --- | --- |
| 1 | connect | `passkey-rp-id` as a setting; the verifier takes an exact list of origins (`internal/webauthn` `checkClientData`), today the account origin; `passkeySite` deleted (C10, R3) | S | ● |
| 2 | connect | The panel's pages take the loader's host from configuration (C4–C7), so staging's pages stop naming production's loader | S | ● |
| 3 | connect | The support address from configuration (C1–C3); staging's `mail-from` its own or the outbox (C9) | S | ● |
| 4 | connect | Redirect hosts: `www` to the issuer, and retired hosts to their successors, path and fragment kept, before the API's fallthrough (R6); or the same as Cloudflare redirect rules, which then must be written down | S | ● |
| 5 | connect | HSTS from configuration: `includeSubDomains; preload` at the issuer when asked, on the panel as well (R6) | XS | ● |
| 6 | Orpheum | `pair` prints the loader origin the discovery named; the install script says it instead of `app.orpheum-app.com` (O3). The core API's discovery status gains `loaderOrigin` beside `clientOrigin` | S | ● |
| 7 | connect + ops | Staging's names on `orpheum.dev` (compose, tunnel routes, `rebuild.sh`, runbook); staging's core leaves and connects again, since its signatures bind the old API URL (R4) | S | ● |
| 8 | connect | The names file (§5.1) replacing the flags, with `connectd names hosts` and `names check`, and the tests over both files | M | ◐ |
| 9 | connect | The loader page's fourth meta tag, `orpheum-homepage`; Orpheum's loader links "what is Orpheum" to it and refuses a loader-origin tag that is not its own origin | S + S | ○ before production's app move (D390 step 5) |
| 10 | Orpheum | The System page's default from the core (O2 gone); the suggestion in one Go file (§5.2) | S | ○ with 9 |
| 11 | Orpheum | The lab placeholder (O4) and the comments and help text (§1.1) to reserved names | XS | ○ any time |
| 12 | ops, site, Orpheum deploy READMEs | The runbooks and the site's README for the new names; living docs only, history left | M | ○ with each step it describes |
| 13 | Orpheum | `discoveryproto` gains the issuer document's type; `discovery/single` serves it | S | ○ before the self-deployed discovery ships |
| 14 | connect | connectd serves the document at the issuer (from the names file) | XS after 8 | ○ with 13 |
| 15 | Orpheum | The core connects by issuer: fetch, exact `issuer` check, confirm, password on Connect, pin; status origins checked against the pin; existing cores pin by `coreApi` match (§4.3); `install.sh --discovery` | M | ○ before the public launch |
| 16 | Orpheum | Test: no page, link or message can choose the core's discovery (§4.4) | XS | ○ with 15 |
| 17 | Orpheum, connect | The test fixtures drift to reserved names when touched | — | ○ ongoing |
| 18 | — | A discovery key, signed statements, an API-move protocol (§4.5, §4.6) | M+ | ○ revisit at the public launch |

What the move does **not** need, by D390's own choices: no core release (the API stays; client
origins stay), no change to `ServiceURL`, no migration of `discovery.json`, no change to the client
zone or core zone.

## Sources

- RFC 8414, OAuth 2.0 Authorization Server Metadata, §3.3 issuer validation:
  https://www.rfc-editor.org/rfc/rfc8414 ; the requirement as MCP clients apply it:
  https://modelcontextprotocol.io/specification/draft/basic/authorization/authorization-server-discovery
- RFC 9207, OAuth 2.0 Authorization Server Issuer Identification (mix-up attacks; clients of one
  server are not vulnerable): https://www.rfc-editor.org/info/rfc9207/
- RFC 8707 (resource indicators), RFC 8615 (well-known URIs), RFC 2606 and RFC 6761 (reserved names):
  https://www.rfc-editor.org/rfc/rfc8707 , https://www.rfc-editor.org/rfc/rfc8615 ,
  https://www.rfc-editor.org/rfc/rfc2606 , https://www.rfc-editor.org/rfc/rfc6761
- Matrix client-server discovery, `/.well-known/matrix/client`:
  https://spec.matrix.org/latest/client-server-api/
- WebAuthn RP ID and Related Origin Requests: https://www.w3.org/TR/webauthn-3/ ,
  https://web.dev/articles/webauthn-rp-id (and the domains audit's sources on passkeys)
- OWASP HTML5 Security Cheat Sheet (exact origin checks for `postMessage`; never reflect `Origin` in
  CORS): https://cheatsheetseries.owasp.org/cheatsheets/HTML5_Security_Cheat_Sheet.html
- HPKP's deprecation and removal (hostile pinning, "too much of a footgun"):
  https://groups.google.com/a/chromium.org/g/blink-dev/c/he9tr7p3rZ8/m/eNMwKPmUBAAJ ,
  https://developer.chrome.com/blog/chrome-72-deps-rems
- HSTS preload requirements: https://hstspreload.org/
- Tailscale `--login-server`: https://tailscale.com/docs/reference/tailscale-cli/up ,
  https://tailscale.com/kb/1507/custom-control-server
- Bitwarden's server choice: https://bitwarden.com/help/change-client-environment/
- Plex: per-server certificates, https://words.filippo.io/how-plex-is-doing-https-for-all-its-users/ ;
  the claim token, https://docs.linuxserver.io/images/docker-plex/
- Home Assistant Cloud remote access (SNI proxy, keys only on the instance):
  https://support.nabucasa.com/hc/en-us/articles/26508882007581-Remote-access-Security-aspects ,
  https://support.nabucasa.com/hc/en-us/articles/25619268678557-Remote-access-Deep-dive
- Jellyfin has no relay; Quick Connect is a sign-in code: https://en.wikipedia.org/wiki/Jellyfin ,
  https://www.wundertech.net/how-to-access-jellyfin-remotely/
