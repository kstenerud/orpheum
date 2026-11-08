> **ABOUTME:** An outside audit of the proposed move to `orpheum.cc`, `orpheum.mov` and
> `orpheum.dev`: what sits on one site with what (cookies, CSRF, `__Host-`, WebAuthn's relying
> party, CSP and CORS, Cloudflare), the apex, the three TLDs and TV keyboards, the Public Suffix
> List, staging's isolation, mail, the self-deployed discovery, and a recommended layout with a
> migration order. It proposes; it decides nothing.

# The domain layout: an outside audit

- **Date:** 2026-10-31, on an earlier commit. **Asked by** the owner, after buying `orpheum.cc`,
  `orpheum.dev` and `orpheum.mov` (the owner's answers of 2026-10-31 in
  [navigation-and-widgets-answers-2026-10-27.md](../plans/navigation-and-widgets-answers-2026-10-27.md)).
- **The proposal audited:** the app (the loader) at the apex `orpheum.cc`; `api.orpheum.cc` and
  `account.orpheum.cc` under it; the website at `orpheum.mov`; `orpheum.dev` redirecting to the
  website; `orpheum-app.com` redirecting to the new homes; `orpheum-client.com` and
  `orpheum-core.com` unchanged. The owner: *"We have a lot of domains to play with, so it's no
  problem to use the longer domains for things users don't need to type in."*
- **Read:** [discovery-protocol.md](../discovery-protocol.md) §1, §2.7, §3.4, §3.7;
  [client-from-core.md](../plans/client-from-core.md); the
  [client-from-core security audit](client-from-core-security-audit-2026-10-17.md); D273, D274,
  D289–D294, D296, D308, D319 in the working notes; in `orpheum-connect`: `docs/spec.md` §2,
  `docs/plans/accounts.md` §6, §7, §11, `deploy/production/compose.yaml`,
  `deploy/staging/compose.yaml`, `internal/panel/` (cookies, CSP, `http.CrossOriginProtection`),
  `internal/webauthn/`, `internal/core/auth/` and `cmd/connectd/open.go` (how the RP ID is chosen);
  in Orpheum: `internal/core/discovery/discovery.go` (`ServiceURL`),
  `internal/deployment/templates/install.sh`. Live DNS was looked up for the lookalike names (§3.5).
- **Markers:** **[V]** verified against a source listed at the end or against the code; **[I]**
  inferred, reasoned rather than shown; **[U]** unverified, to be measured.

## 0. The plain answer

The proposal is **right about the two things that matter most to people**: a typed address with no
hyphen, and one brand domain that the app, the account pages and the mail all live on. It is
**wrong, or under-specified, in five places**:

1. **The API should not move under `orpheum.cc`.** Nobody types it, every enrolled core has its URL
   saved (`discovery.json`) and every core binary has it compiled in (`discovery.ServiceURL`), and
   on the brand site it only enlarges the set of hosts that share cookies and the passkeys' relying
   party. Keep `api.orpheum-app.com` (§1.6). This removes a whole class of migration work (cores,
   the install template, the production NAS's state) from the move.
2. **Staging must leave production's site.** Today `app-staging.`, `api-staging.` and
   `account-staging.orpheum-app.com` are the *same site* as production's account pages; carried
   over as `*-staging.orpheum.cc` it would stay so. Code built in the sandbox can then toss cookies
   onto production's whole site, in the very browser (the owner's) that uses both. Put staging's
   front names on a registrable domain of their own: `orpheum.dev`, which is short enough for the
   owner's TV and HSTS-preloaded by its TLD (§4.3). Its redirect to the website is not worth that
   name.
3. **Decide the passkeys' relying party now, explicitly.** As built, the RP ID is the account
   pages' host (`cmd/connectd/open.go` `passkeySite`), so the move would silently make it
   `account.orpheum.cc`. The recommendation is the registrable domain `orpheum.cc`, with the server
   accepting an exact list of origins (§1.5). No passkey exists yet, so this is the one free moment.
4. **Typing is not the only usability risk; the `.com` habit is.** `orpheum.com`, `orpheum.co` and
   `orpheum.net` are registered by others (§3.5), and the page a TV types is where every household
   types its core's password (the realm sign-in, D308 §4). Typing is for the first visit only;
   everything after must be a bookmark, a QR or a code.
5. **Mail must move with the brand, and every domain that sends nothing must say so** (null MX,
   `v=spf1 -all`, DMARC `p=reject`), `.mov`, `.dev` and the households' zones included (§5).

On the rest it agrees: the app at the apex is sound (§2); `account.` on the brand site is right
(§1.4); `.cc` for the typed name is acceptable with HSTS preload (§3.1); the website on `.mov` is
acceptable, with costs it should take knowingly (§3.2); `orpheum-client.com` and `orpheum-core.com`
are correctly apart, and both belong on the Public Suffix List at the public launch, not now (§4).

## 1. What should share a site with the app

### 1.1 What "same site" gives and does not give

A *site* is the scheme plus the registrable domain (the label below a public suffix); every host
under `orpheum.cc` is one site, and `orpheum.cc` itself is one of them **[V, PSL; MDN on
SameSite]**. What being one site means here:

- **SameSite does not separate siblings.** `SameSite=Strict` stops cookies on *cross-site*
  requests; a request from `orpheum.cc` to `account.orpheum.cc` is same-site, so a page on any
  sibling, compromised or hostile, can send navigations and form posts that carry the account's
  `Strict` cookies **[V, PortSwigger's "bypassing SameSite via sibling domains"; jub0bs]**.
- **Cookie tossing.** Any host on the site can set a cookie with `Domain=orpheum.cc`, which the
  browser then sends to every host on the site, the apex included **[V, RFC 6265 §5.3; csrf.page]**.
  It can fix a session for a server that reads an unprefixed cookie, or *bomb* the site: enough
  large cookies that every request exceeds the edge's header limit and the site stops answering for
  that browser.
- **`__Host-` stops overwriting, not bombing.** A `__Host-` cookie must be `Secure`, `Path=/` and
  carry no `Domain`, so a sibling cannot set or replace one **[V, MDN]**. The *Cookie Crumbles*
  study (USENIX Security 2023) broke the prefix with nameless cookies (`=__Host-x=…`) in browsers
  and in server parsers (CVE-2022-2860 Chrome, CVE-2022-40958 Firefox, CVE-2023-23934 Werkzeug)
  **[V]**; the browsers are fixed, and Go's cookie parser drops a nameless pair [I], so the prefix
  holds today. It does nothing against a bomb of other names.
- **Fetch Metadata does separate siblings.** The account pages run Go's
  `http.CrossOriginProtection`, which passes an unsafe request only when `Sec-Fetch-Site` is
  `same-origin` or `none` and refuses `same-site` **[V, Go's `net/http/csrf.go`, the switch on the
  header]**, plus a form token. That is what actually protects the account from a sibling, and it
  holds whatever the layout. Safe methods (GET) pass by design, so no GET may change anything (the
  panel's invariants already say so).
- **CORS and CSP are per origin, not per site.** `api.` is cross-origin to the apex whether or not it
  is same-site, so the CORS list and the loader's `connect-src` are the same work either way.

**So the cost of a sibling is bounded by what the siblings run.** The rule that follows: *a site
holds only hosts served by the same program, from the same review, with the same deploy*. On the
proposal, `orpheum.cc`, `account.` and `api.` are all `connectd` (one process, one tunnel), so
an RCE gives everything regardless of names; what names change is what an **XSS or a hostile
build on one host** gives on the others.

### 1.2 What a flaw on each host would give on the others

| Flaw on | Its pages render | What it gets on the other hosts of the site |
| --- | --- | --- |
| the apex (the loader) | household names and realms from the list, a core's error text | bomb `account.` (and the apex); same-site navigations to `account.` carry its cookies, but POSTs are refused (Fetch Metadata) and the cookies are `HttpOnly` |
| `account.` | the account's own data, server-rendered, `script-src 'self'` | bomb the apex: the loader then serves its kept page (D308 §7), so a returning browser still works; nothing else, the loader holds no cookie |
| `api.` | JSON from every core on the internet (the most exposed parser) | the same bombing of the apex and `account.`, if it ever answered HTML with a reflected value |
| staging under the same site | code built in the agent's sandbox | all of the above, against **production**, in the owner's browser |

The last row is the real finding: the sandbox is the least-reviewed code that will ever be served
under these names, and the owner's browser is the one that visits both staging and production.
§4.3 takes it out.

### 1.3 `__Host-` coverage

Every cookie the service sets is `__Host-` (`__Host-session`, `__Host-browser`, `__Host-operator`;
`internal/panel/panel.go`, `operator.go`) and `SameSite=Strict`, `HttpOnly` **[V, code]**; the loader
and the client origins set none (D289 §3, D308 §7). That coverage is complete, and the move keeps
it. Two gaps are outside our code:

- **Cloudflare's own cookies.** Bot Fight Mode (`__cf_bm`), challenges (`cf_clearance`), rate-limit
  rules (`_cfuvid`), Always Online and load-balancer affinity set cookies the origin never sends
  **[V, Cloudflare's cookie reference]**. Cloudflare's documentation does not state their `Domain`
  scope [U]. Keep those features off on `orpheum.cc`, `account.`, and the client zones, and check
  at the edge, not in a unit test: `curl -sI https://orpheum.cc/` must carry no `Set-Cookie`.
- **Rocket Loader, Email Obfuscation, Zaraz, Web Analytics** inject script into HTML; on the loader
  and the panel they would break `script-src 'self'` or, worse, be allowed by someone loosening the
  CSP to make them work. The panel already sends `Cache-Control: no-transform`; the loader should
  too (discovery-protocol §3.4 says "injected scripts stay off", G13) **[I]**.

### 1.4 Should `account.` be on the typed domain?

**Yes, on the brand site; no, not at the apex.** Arguments, adversarially:

- *For a separate registrable domain* (e.g. `orpheum-account.com`): `SameSite=Strict` would then
  really separate it from the loader, and no sibling could bomb it.
- *Against, and decisive:* the account is reached by **mail links** (invitation, reset,
  verification) and from the app. Its credential is the one that decides which core the loader sends
  a household's realm sign-ins to (accounts.md §1, D312's finding), so it is the most valuable
  phishing target the service has. The defence that scales to ordinary people is *one domain to
  learn*: mail from `orpheum.cc`, links to `orpheum.cc`, the app at `orpheum.cc`. A second
  unfamiliar domain in the mail teaches people that Orpheum's links come from anywhere, which is
  exactly what phishing needs [I; the same reason large providers keep sign-in on the brand domain].
  The sibling risks of §1.2 are already met by Fetch Metadata, `__Host-` and the loader's kept page.
- *Not at the apex itself:* the apex is the loader, whose origin holds the household list in
  storage and a service worker; the account's cookie-bearing, server-rendered pages belong on their
  own origin, as `account.` gives them (spec §2's reasoning, kept).

### 1.5 The passkeys' relying party

How it works: a passkey is bound to an RP ID, which must equal the page's host or be a registrable
suffix of it; it is then usable on that host and every host below it, and nowhere else **[V,
web.dev; WebAuthn L3]**. Related Origin Requests (a `/.well-known/webauthn` list on the RP ID's
domain naming up to five other sites) let other domains use it; Chrome 128, Safari 18 and Firefox
152 (May 2026) ship it, and WebAuthn Level 3 became a W3C Recommendation on 2026-08-25 **[V]**. ROR
lets *other origins use* an RP ID; nothing moves an RP ID. A passkey made for one RP ID is lost to
another for good.

As built, `passkeySite` takes the RP ID from the account origin's host **[V, `cmd/connectd/open.go`]**,
so the move would make it `account.orpheum.cc` without anyone deciding it. The choices:

| RP ID | Usable on | For | Against |
| --- | --- | --- | --- |
| `account.orpheum.cc` | `account.` and below | tightest; the apex cannot exercise it | if the app ever signs in to the account (a TV's "find my server" by passkey from the phone), it needs ROR for its own site's apex; password managers show the subdomain |
| **`orpheum.cc`** | the apex and every host under it | the name people know, shown in their passkey manager; the apex and `account.` both work with no ROR; standard practice (one RP ID per brand) | every sibling can *ask* for an assertion; the server must accept only an exact list of origins |

**Recommendation: `orpheum.cc`**, with the server's accepted origins an exact list (today
`https://account.orpheum.cc`; the apex added only when a ceremony there is built). The verifier
already compares `clientData.origin` to one exact origin and refuses a framed ceremony **[V,
`internal/webauthn/webauthn.go` `checkClientData`]**; it needs a list, not a suffix match. The
against-column is why §1.6 keeps `api.` and §3.2 keeps the website off the site: the smaller the
site, the cheaper a site-wide RP ID. **Staging must never be under `orpheum.cc`** (its pages could
then run ceremonies for production's RP ID, refused only by the server's origin check). And because
the name is provisional (D195), once the first passkey exists **`orpheum.cc` is kept registered
forever**: a future brand domain can still use those passkeys through ROR, published at
`https://orpheum.cc/.well-known/webauthn`.

The other two relying parties are untouched: the operator's panel is `localhost` (an SSH forward),
and a core's own passkeys (D294 §3) are the core's own names.

### 1.6 Should the API live on a non-typed domain?

**Yes: keep it at `api.orpheum-app.com`.**

- **Machines hold it, not people.** Every enrolled core keeps its discovery's base URL in its state
  file (`discovery.json`), and the default is compiled in (`internal/core/discovery`
  `ServiceURL = "https://api.orpheum-app.com/core/v1"`) **[V, code]**. Moving it means rewriting
  the production NAS's state, changing the binary, the install template and the docs, and a WebSocket
  signalling client does not follow a 301 the way a page does [I]. Keeping it removes all of that
  from the move.
- **It is the most exposed parser** (offers and addresses from every core on the internet, D292 §4);
  off the brand site, a flaw there gets no sibling position on the account (§1.2) and no claim on
  the RP ID's site (§1.5).
- **Cross-site costs nothing here.** The client API is cookie-free (only the panel sets cookies,
  **[V, `grep SetCookie`]**) and authorises by header, so third-party-cookie rules, SameSite and
  storage partitioning never touch it. CORS and `connect-src` are needed for a cross-origin API in
  any case. One more TLS handshake to another host is paid already, since `api.` is another host
  either way [I].
- **The provisional name** (D195): the names machines are configured with should change least. If
  the brand moves again, the typed domain and mail move; the API, the client zone and the core zone
  do not.

`orpheum-app.com` then is not "just redirects": its apex, `app.` and `account.` redirect; `api.`
serves. Its only other host, `media.orpheum-app.com`, still resolves (to `198.51.100.139`)
**[V, dig]**; a core-held certificate on the API's site is what D292 §1 moved cores away from, so it
should be retired as the replace-media plan intends. With `account.` gone from `orpheum-app.com`,
nothing on that site holds a cookie any more.

## 2. The app at the bare apex

**Sound.** The owner's reason (no hyphen, fewest characters) is the right one, and the classic
objections mostly do not apply:

- **"Apex cookies leak to every subdomain."** A cookie set *without* a `Domain` attribute is
  host-only: the apex's cookies reach the apex alone **[V, RFC 6265 §5.3 step 6]**. Only a cookie set
  *with* `Domain=orpheum.cc` reaches all hosts, and `__Host-` forbids that. The loader sets no
  cookie at all. The real apex risk is the reverse: every `Domain=orpheum.cc` cookie a sibling or
  Cloudflare sets is sent *to* the apex (§1.3).
- **CNAME at the apex.** DNS forbids it; Cloudflare flattens a proxied apex CNAME to the Tunnel
  (`<uuid>.cfargotunnel.com`), automatically and always **[V, Cloudflare]**. The cost is lock-in: a
  move off Cloudflare DNS needs a provider with ALIAS/ANAME [I].
- **HSTS preload.** To preload `orpheum.cc`, the apex must send
  `max-age>=31536000; includeSubDomains; preload` and every subdomain must serve HTTPS **[V,
  hstspreload.org]**. That forces nothing new here (every host is behind Cloudflare's HTTPS), and it
  rules out ever putting a plain-HTTP name under `orpheum.cc` (no core, no LAN name: they are
  elsewhere). Removal from the list takes months, so submit once the hosts are settled.
- **Preload matters more for a typed TV address than usual.** Chrome 154 (October 2026) makes
  "Always use secure connections" the default **[V]**, but TV browsers ship older Chromium and older
  preload lists (Silk 138 on the owner's stick, network-stack-audit). Until `orpheum.cc` is in the
  lists those browsers carry, a first typed visit may start over HTTP and can be stripped by anyone
  on the household's network [I]. `.dev` and `.mov` are preloaded at the TLD **[V]**; `.cc` is not.
- **`www.orpheum.cc`** must exist and redirect to the apex: people type `www.` from habit, and some
  TV keyboards lead with it (a third-party guide to Samsung's browser advises starting with
  `www.`; second-hand).
- **A stranger who types `orpheum.cc`** gets the loader; it must say what Orpheum is and link to the
  website, since the website no longer owns the name people hear.

## 3. The TLDs, and typing on TVs

### 3.1 `.cc`

- **Registry:** operated by eNIC, a Verisign subsidiary, since 2001 **[V]**: stable.
- **Reputation:** mixed and worsening. A 2025 analysis singles out `.cc`'s year-on-year growth in
  malicious registrations, helped by $2.40 promotions **[V, Forescout / Suped citing it]**; a
  specific "91 % malicious" figure circulates but its source could not be traced [U]. Mail from
  `.cc` meets some filters' suspicion **[V, Network Solutions]**. HaGeZi's "most abused TLDs"
  list, the one self-hosters most often add to Pi-hole or AdGuard, does **not** list `cc` (nor
  `mov`, `dev`, `zip`) as of its 2026-10-29 build **[V, the list, cached at
  `/yoloai/cache/domains-audit/`]**, and Pi-hole blocks no TLD by default **[V]**.
- **Verdict:** acceptable for the typed web name, with preload (§2), with mail authentication done
  carefully (§5), and with deliverability checked at Gmail, Outlook and GMX/web.de before invitations
  depend on it. The alternative worth naming: **`orpheum.dev` is strictly better on security** (TLD
  preload in every browser's list since 2017, Google's registry and its abuse policy), and worse only
  in connotation: a household may read "dev" as "not finished". This audit takes the owner's `.cc`
  and gives `.dev` to staging (§4.3); swapping them is a defensible choice.

### 3.2 `.mov`

- **Why security people objected (2023):** `.zip` and `.mov` look like file names, so text that
  mentions a file (`setup.mov`) is auto-linked by mail clients, chat and forums to whoever owns that
  domain, and attackers registered such names for phishing **[V, BleepingComputer, Dark Reading,
  Neowin, Intego, Silent Push via Dark Reading]**. The Register's counterpoint: the risk is real but
  modest, and the outrage outsized **[V]**.
- **Blocking is a documented practice, not a rumour:** Cisco documents blocking `*.zip` and `*.mov`
  by a DNS policy, Zscaler by TLD category, and Pi-hole users asked for it by default (declined)
  **[V]**. Corporate and school networks therefore sometimes cannot open a `.mov` site [I on
  prevalence].
- **For the website:** acceptable. It holds no credential, takes no input, sends no mail, and is
  found by search and links, not typed on a TV. Its costs (some workplaces block it; a
  security-literate self-hosting audience remembers the controversy; `orpheum.mov` in plain text reads
  like a video file) are bounded because nothing *works* through it. The film pun is real value.
- **A typed or credential-bearing address on `.mov` would be a mistake:** blocked networks would
  break the product, not just the brochure, and a sign-in page on the TLD best known for phishing
  works against the "learn one domain" defence.
- **It must not be on the brand site anyway:** the website is Cloudflare Pages built from
  `orpheum-site`, a different supply chain from `connectd`, so §1.1's rule puts it on its own site.
  `www.orpheum.cc` for the website would shrink the domains people meet to one, at the cost of a
  second deploy pipeline that could bomb the app and the account and ask for the RP ID's assertions.

### 3.3 `.dev`

HSTS-preloaded at the TLD since 2017, so every name under it is HTTPS-only in every major browser
with no submission **[V, Google Registry; Wikipedia]**. Good reputation. Best used for what an
engineer types: staging (§4.3). As a redirect to the website it wastes its one property that the
others lack.

### 3.4 What a TV's keyboard makes easy

The evidence is thin, and this section says so.

- **No browser at all** on Apple TV (tvOS has no Safari and no web view for apps) and none by
  default on Google TV / Chromecast **[I, well known; not re-verified here]**. A typed address
  serves **Fire TV (Silk), Samsung (Tizen's Internet), LG (webOS's Web Browser)** and Android TV
  sets with a browser installed.
- **Android TV's Gboard** has distinct layouts for email, password, text and number fields **[V,
  Android developers]**; the documentation does not show a `.com` key or where `.` sits in a URL
  field [U]. Fire TV's keyboard offers a phone keyboard by QR (`keyboard.amazonfiretvapp.com`)
  **[V]**, which removes most of the typing problem. A third-party guide to Samsung's browser says
  to start with `www.` and that the keyboard then suggests popular sites (second-hand). Whether `.` and `-` are on the first plane,
  and whether a `.com` key exists, on each of the owner's devices, is **[U]**.
- **What holds whatever the keyboards do:** a dot is in *every* candidate address, so the saving of
  `orpheum.cc` over `app.orpheum-app.com` is length (10 characters against 19) and the hyphen,
  which is more often off the first plane than the dot [I]. A `.com` key, where it exists, helps
  only a `.com` name and nudges people toward `orpheum.com` (§3.5).
- **Measure it, cheaply:** on the owner's Fire TV, and on any Samsung or LG within reach, count the
  remote presses for `orpheum.cc`, `orpheum.dev` and `app.orpheum-app.com` in the browser's address
  bar, and note whether a `.com` key or suggestions appear. A `VQ` item is the right home for it.

### 3.5 The lookalikes, and why typing must be a one-time act

Looked up on 2026-10-31 **[V, dig]**: `orpheum.com` (live, Azure DNS), `orpheum.co` (registered,
one letter from `.cc`), `orpheum.net` (parked for sale at Afternic), `orpheum.app` (Porkbun) and
`orpheum.tv` (Cloudflare, but a different name-server pair from the owner's zones, so most likely not
the owner's) are all registered. The owner's `orpheum.cc` and `orpheum.mov` share one pair.

The typed page is where households type **their core's password** (the realm sign-in, D308 §4). A
TV's `.com` key, phone autocorrect, or simple habit sends some of them to `orpheum.com`; a squatter
on `orpheum.co` needs one wrong key. So:

- **typing is for the first visit only**: the loader must offer to be bookmarked or installed on
  every device, and its link and QR (on the core's System page, in invitations) are the normal way
  onto a phone or laptop;
- the device-code path (a code on the TV, entered on a phone) is the safer way to bring a TV in, as
  the protocol already has it (§3.1 there);
- watch for new registrations close to `orpheum.cc` (certificate transparency on `orpheum` names)
  [I]; buy `orpheum.net` if its price is small, since it is for sale.

## 4. The Public Suffix List, the households' zones, and staging

### 4.1 `orpheum-client.com` on the PSL

The private section is for "owners of privately-registered domains who themselves issue subdomains
to mutually-untrusting parties", which is exactly `<core id>.orpheum-client.com` **[V, PSL]**. On
the list, each household's client origin becomes its **own site**: no cookie tossing or bombing
between households, cross-site SameSite between them, storage and cache partitioned per household.
But the guidelines also say projects "not serving more than thousands of users are quite likely to
be declined" (a 15,000 floor has been proposed), require more than two years of registration left,
and offer no way to speed browsers' uptake **[V, PSL guidelines and issues #2805, #3311]**.

**Agree with D308 §7:** not now; the disciplines (no cookie ever on a client origin, the loader
deleting what it sees and keeping its page, `Origin-Agent-Cluster`, COOP) cover a beta of known
households. Apply at the public launch, with the registration extended past two years first.

### 4.2 `orpheum-core.com`

Correctly separated from the service: cores hold certificates there and could otherwise set
cookies for the service's pages (D292 §1, §2.7 of the protocol) **[V]**. Between households it is
one site, as the client zone is: a core can set `Domain=orpheum-core.com` cookies that reach every
other core's names. The core authorises by header and the media cookie's leak was fixed apart from
D308 (D308 §6), so what is left is a bomb against another household's direct path, which then falls
back to WebRTC [I]. **Recommend the same PSL entry at the public launch.** The guidelines refuse an
entry whose *sole* purpose is rate limits (protocol §2.7, F11); this one's purpose is separation of
mutually untrusting parties, which is the list's own purpose, and per-core Let's Encrypt limits
would follow as a side effect [I: whether the PSL reviewers see it so is theirs to say].

### 4.3 Staging on its own registrable domain

Today staging's front names are under production's (`app-staging.`, `api-staging.`,
`account-staging.orpheum-app.com`; D292 §2, kept by D296 §3) **[V, compose files]**. D292 §2's
reasoning was that `__Host-` stops staging overwriting production's cookies, and that no production
credential enters the sandbox. Both are true and not sufficient:

- staging's pages can **bomb** production's site and make same-site navigations to its account
  pages (§1.1), from code the sandbox built, in the owner's own browser;
- under a site-wide RP ID (§1.5), staging's pages could run production's passkey ceremonies,
  refused only by the server's origin check;
- staging's Cloudflare features and experiments (a challenge, a cookie) land on production's site.

D296 already moved staging's *households* to `orpheum-test.com` for the same class of reason, and
cited Mozilla's separate test domain (`allizom.org`) as the common answer. The front names deserve
the same. **Recommend `orpheum.dev` for staging's front:** the apex as staging's loader,
`account.orpheum.dev`, `api.orpheum.dev`. It is the owner's typed staging address on the TV (a
short one matters: the owner types staging far more often than any household types production),
and its TLD preload makes it HTTPS-only with no submission. Staging's API on its own site while
production's is cross-site from its app is a harmless asymmetry: the API is cookie-free, so the
browser treats both the same (§1.6). Staging's sandbox then holds a tunnel token that edits no DNS
and the `orpheum-test.com` zone token, as today; nothing of production's.

## 5. Mail

- **Move the sender to the brand: `noreply@orpheum.cc`** (or a dedicated subdomain such as
  `mail.orpheum.cc` for its envelope, keeping the visible `From` on `orpheum.cc`). The links in the
  mail go to `account.orpheum.cc`; a `From` on another domain than the link is both a phishing
  heuristic some filters weigh and the lesson §1.4 wants nobody to learn [I].
- **Authentication:** DKIM signed with `d=orpheum.cc` (the Scaleway sender's key), SPF naming
  Scaleway only, DMARC aligned, starting at `p=none` with reports, then `quarantine`, then `reject`
  once reports are clean [I, standard practice]. A new `.cc` domain has no sending history: send the
  first invitations to the owner's own Gmail, Outlook and GMX addresses and read the headers' verdicts
  before relying on it (§3.1). Google Postmaster Tools on `orpheum.cc` shows reputation [I].
- **Every domain that sends no mail says so** (M3AAWG's parked-domain practice, the UK NCSC's
  guidance): a null MX (`MX 0 .`, RFC 7505), `v=spf1 -all`, and `_dmarc` `p=reject` **[V]** on
  `orpheum.mov`, `orpheum.dev`, `orpheum-client.com`, `orpheum-core.com`, `orpheum-test.com`,
  `orpheum-client-test.com`, and on `orpheum-app.com` once its mail has moved. Without it, anyone
  can send convincing "Orpheum" mail from the website's or the households' domains.

## 6. The self-deployed discovery

The layout generalises **if every name is configuration, and the roles, not the names, are the
design**:

| Role | Paid | Own discovery (example) |
| --- | --- | --- |
| typed loader | `orpheum.cc` | `media.example.org` |
| account pages | `account.orpheum.cc` | none: one household, the core's own sign-in |
| discovery API | `api.orpheum-app.com` | `media.example.org/api` or `discovery.example.org` |
| per-core client origin | `<id>.orpheum-client.com` | not needed: one household is one trust domain |
| core names | `<addr>.<id>.orpheum-core.com` | `<addr>.<id>.home.example.org`, or the core's own name |

What the code must keep true for that: `connectd`'s flags already name every origin
(`-client-origin`, `-client-origins`, `-account-origin`, `-zone`, `-mail-from`) **[V, compose]**,
and the core learns its client and loader origins from `GET /status` (D293 §4, D308) **[V]**. The
pieces that assume the paid names are `ServiceURL` (a default, overridable by `pair -discovery`)
**[V]**, the install template's "sign in at app.orpheum-app.com" **[V]**, and the RP ID's derivation
(§1.5), which should become an explicit setting. An own discovery will usually put loader and core
on one registrable domain; that is acceptable for one household, which trusts its own core, and the
documentation should say why the paid service does not do the same.

## 7. Recommended layout

### 7.1 Production

| Host | Role | Typed by people? | Why |
| --- | --- | --- | --- |
| `orpheum.cc` | the app: the loader, the household list, the realm sign-in | **yes**, once per device (TV) | short, no hyphen; HSTS preloaded by submission; cookie-free; links to the website |
| `www.orpheum.cc` | 301 to `orpheum.cc` | by habit | people type `www.`; required HTTPS for preload |
| `account.orpheum.cc` | account pages: sign-in, join, reset, passkeys | no; mail links and the app | brand domain for phishing resistance; `__Host-`, Fetch Metadata, strict CSP |
| RP ID `orpheum.cc` | passkeys for the account (later the apex) | — | the name people know; exact origin list on the server; ROR if the brand moves |
| `noreply@orpheum.cc` | account mail | — | `From` matches the links; DKIM `d=orpheum.cc`, DMARC to `reject` |
| `api.orpheum-app.com` | discovery API, `/core/v1` and `/client/v1` | no | unchanged: compiled into cores and saved in their state; most exposed parser off the brand site; cookie-free |
| `orpheum-app.com`, `app.`, `account.orpheum-app.com` | 301 to the website, `orpheum.cc`, `account.orpheum.cc`, path and fragment kept | no | old links and mail keep working; keep registered for good |
| `media.orpheum-app.com` | retire | no | a core-held name on the API's site |
| `<core id>.orpheum-client.com` | each household's client origin | no | unchanged; PSL entry at the public launch |
| `<address>.<core id>.orpheum-core.com` | each core's names and certificates | no | unchanged; PSL entry at the public launch |
| `orpheum.mov`, `www.orpheum.mov` | the website (Cloudflare Pages) | rarely; found by search and links | a separate supply chain on its own site; TLD-preloaded; no credentials, so `.mov`'s costs are bounded |

### 7.2 Staging

| Host | Role | Typed by people? | Why |
| --- | --- | --- | --- |
| `orpheum.dev` | staging's loader | **yes**, by the owner on test devices | its own registrable domain: no cookie, RP ID or Cloudflare setting reaches production; TLD-preloaded |
| `account.orpheum.dev`, `api.orpheum.dev` | staging's account pages and API | no | mirror production's roles; the API's site differs harmlessly (cookie-free) |
| `<core id>.orpheum-client-test.com`, `<address>.<core id>.orpheum-test.com` | staging's client and core zones | no | unchanged (D296, D319) |

## 8. Migration order

Each step stands alone and can stop there.

1. **Decide the RP ID** (§1.5) and make it a setting in `connectd`, not a derivation. Free while no
   passkey exists; costly forever after.
2. **Mail hygiene on every non-sending domain** (§5): null MX, SPF `-all`, DMARC `reject`. No risk,
   no dependency.
3. **Harden the new zones:** registrar lock, hardware-key 2FA on the registrar and Cloudflare,
   DNSSEC, CAA (`letsencrypt.org` plus Cloudflare's CAs on `orpheum.cc` and `orpheum.mov`; staging
   alike), and Cloudflare's script- and cookie-injecting features off (§1.3).
4. **Staging first, on `orpheum.dev`:** tunnel routes, `connectd`'s staging flags, the staging
   account pages, a passkey made and used, and the Fire TV typing `orpheum.dev`. This rehearses the
   production move with nothing at stake. Then retire `*-staging.orpheum-app.com`.
5. **Production's app and account on `orpheum.cc`:** tunnel routes for the apex, `www.` and
   `account.`; `connectd`'s `-client-origin`, `-account-origin`, `-mail-from` and RP ID; the
   cores learn the new loader origin from `GET /status` with no core release (D293 §4, D308) [I:
   confirm the production NAS's CORS follows it]. The install template's text changes in Orpheum's next
   release; `ServiceURL` does not.
6. **Mail on `orpheum.cc`:** DKIM, SPF, DMARC `p=none` with reports; invitations tested at Gmail,
   Outlook and GMX; then `quarantine`, then `reject`.
7. **Retire the old loader before redirecting it.** The loader at `app.orpheum-app.com` has a
   service worker; ship it one last update that unregisters itself (the kill switch, protocol §3.4)
   and forwards to `orpheum.cc`, then put the 301s in place. Redirects keep the fragment that carries
   an invitation's or reset's secret, since a `Location` without a fragment inherits the request's
   (RFC 9110 §10.2.2) **[V]**, so old mails' links still work.
8. **The website to `orpheum.mov`;** `orpheum-app.com`'s apex redirects there.
9. **HSTS preload** for `orpheum.cc` and `orpheum-app.com` after some weeks of serving
   `includeSubDomains` with nothing broken. Retire `media.orpheum-app.com`.
10. **At the public launch:** PSL entries for `orpheum-client.com` and `orpheum-core.com`, the
    registrations first extended past two years.

## 9. Where this audit disagrees with the proposal, in short

| Proposal | This audit | Why |
| --- | --- | --- |
| `api.orpheum.cc` | keep `api.orpheum-app.com` | machines hold it (compiled in, saved by enrolled cores); exposed parser off the brand site; cookie-free so cross-site is free |
| `orpheum.dev` redirects to the website | `orpheum.dev` is staging's front | staging is today same-site with production; the sandbox's code must not share production's site; `.dev` is short and preloaded |
| (silent) the RP ID | `orpheum.cc`, an explicit setting, exact origin list | the code would make it `account.orpheum.cc` by accident; it cannot change later |
| (silent) mail stays at `orpheum-app.com` | `noreply@orpheum.cc`; null-mail records everywhere else | one domain to trust; unprotected domains are spoofable |
| "users type `orpheum.cc`" | type once, then bookmark, QR or code | `orpheum.com`, `.co`, `.net` are others'; the typed page takes core passwords |

## Sources

- Public Suffix List, guidelines and the private section: https://publicsuffix.org/ ,
  https://github.com/publicsuffix/list ; issues https://github.com/publicsuffix/list/issues/2805 ,
  https://github.com/publicsuffix/list/issues/3311
- RFC 6265 (cookies) §5.3: https://www.rfc-editor.org/rfc/rfc6265 ; RFC 9110 §10.2.2 (Location and
  fragments): https://www.rfc-editor.org/rfc/rfc9110 ; RFC 7505 (null MX):
  https://www.rfc-editor.org/rfc/rfc7505
- MDN, Set-Cookie and cookie prefixes: https://developer.mozilla.org/docs/Web/HTTP/Headers/Set-Cookie
- Squarcina et al., *Cookie Crumbles* (USENIX Security 2023):
  https://www.usenix.org/system/files/usenixsecurity23-squarcina.pdf ; CVE-2023-23934:
  https://github.com/advisories/GHSA-px8h-6qxv-m22q
- PortSwigger, CSRF and SameSite bypasses: https://portswigger.net/web-security/learning-paths/csrf ;
  jub0bs, *The great SameSite confusion*: https://jub0bs.com/posts/2021-01-29-great-samesite-confusion/ ;
  cookie tossing: https://csrf.page/guide/cookie-tossing
- Go `net/http` CrossOriginProtection: https://pkg.go.dev/net/http#CrossOriginProtection , and the
  source (`src/net/http/csrf.go`, the `Sec-Fetch-Site` switch), read locally; Calhoun,
  https://www.calhoun.io/csrf-protection-via-headers-in-go-125/
- WebAuthn Level 3: https://www.w3.org/TR/webauthn-3/ ; web.dev on RP IDs and Related Origin
  Requests: https://web.dev/articles/webauthn-rp-id , https://web.dev/articles/webauthn-related-origin-requests ;
  Chrome's ROR announcement: https://developer.chrome.com/blog/passkeys-updates-chrome-129 ; Firefox
  ROR: https://bugzilla.mozilla.org/show_bug.cgi?id=2010193 ; Corbado's ROR guide:
  https://www.corbado.com/blog/webauthn-related-origins-cross-domain-passkeys ; *Choosing your RP ID*:
  https://securityboulevard.com/2026/07/choosing-your-rp-id-the-passkey-decision-you-cant-undo/
- HSTS preload: https://hstspreload.org/ ; OWASP HSTS cheat sheet:
  https://cheatsheetseries.owasp.org/cheatsheets/HTTP_Strict_Transport_Security_Cheat_Sheet.html ;
  Chrome 154 secure connections by default:
  https://www.infosecurity-magazine.com/news/chrome-https-mandatory-2026/
- Google Registry's preloaded TLDs: https://www.registry.google/tlds/tech/ ; .dev:
  https://en.wikipedia.org/wiki/.dev ; SecurityWeek on TLD preloading:
  https://www.securityweek.com/google-expands-hsts-preload-list/
- `.zip`/`.mov` reactions: https://www.bleepingcomputer.com/news/security/new-zip-domains-spark-debate-among-cybersecurity-experts/ ,
  https://www.darkreading.com/endpoint-security/google-zip-mov-domains-social-engineers-shiny-new-tool ,
  https://www.neowin.net/news/security-researcher-new-zip-and-mov-top-level-domains-from-google-pose-phishing-risks/ ,
  https://www.intego.com/mac-security-blog/expect-zip-and-mov-domains-to-be-used-in-phishing-and-malware-attacks/ ,
  https://circleid.com/posts/20230517-new-google-domains-spark-cybersecurity-concerns-risks-and-reactions-to-.zip-and-.mov-top-level-domains ,
  counterpoint https://www.theregister.com/security/2023/05/17/googles-zip-and-mov-domains-arent-the-end-of-the-world/619430
- Blocking `.zip`/`.mov`: Cisco community, https://community.cisco.com/t5/web-security/block-tld-s-top-level-domains/td-p/4556412 ;
  Zscaler firewall filtering, https://help.zscaler.com/zia/configuring-firewall-filtering-policy ;
  Pi-hole, https://discourse.pi-hole.net/t/block-zip-and-mov-by-default/63027 ; HaGeZi's most abused
  TLDs, https://cdn.jsdelivr.net/gh/hagezi/dns-blocklists@latest/adblock/spam-tlds-adblock.txt
- `.cc`: https://en.wikipedia.org/wiki/.cc ; https://www.networksolutions.com/blog/what-is-cc-domain-name/ ;
  Forescout 2025, https://www.forescout.com/blog/from-urls-to-malware-how-threat-actors-abuse-domain-security-in-2025/ ;
  Suped, https://www.suped.com/learn/blocklists/what-is-the-most-abused-tld-top-level-domain-for-spam ;
  Spamhaus TLD reputation, https://www.spamhaus.org/resource-hub/domain-reputation/spamhaus-presents-the-worlds-worst-top-level-domains/
- Cloudflare: cookies, https://developers.cloudflare.com/fundamentals/reference/policies-compliances/cloudflare-cookies/ ;
  CNAME flattening, https://developers.cloudflare.com/dns/cname-flattening/ ; Tunnel FAQ,
  https://developers.cloudflare.com/argo-tunnel/faq/
- TV keyboards: Android TV on-screen keyboard,
  https://developer.android.com/training/tv/get-started/onscreen-keyboard ; Fire TV's phone keyboard,
  https://www.aftvnews.com/fire-tv-gains-new-web-based-virtual-remote-and-keyboard-for-quick-and-simple-control-from-anywhere/ ;
  Samsung's browser, https://www.airdroid.com/screen-mirror/samsung-tv-internet-browser/
- Parked domains and mail: M3AAWG, https://www.m3aawg.org/sites/default/files/doc_files/m3aawg_parked_domains_bcp-2022-06.pdf ;
  UK NCSC, https://www.ncsc.gov.uk/blog-post/protecting-parked-domains
