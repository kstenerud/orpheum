> **ABOUTME:** Three proposals for the owner from the first beta tester's install: one button that
> links a server to its orpheum.cc account in place of the key, Connect and Claim; a TMDB key
> shipped with each release; and a library form that offers one library per kind of subfolder.
> A proposal, not a decision: each section names the decisions it would amend.

# Setting up with fewer steps (proposal, 2026-11-19)

- **Status:** SKETCH — proposed to the owner 2026-11-19; nothing here is decided or built.
- **Depends on:** —

The input is [the first beta install](../research/beta-install-report-2026-11-19.md): the tester's
away-from-home chain stopped after his account was made (report §"The lead's reading"), he had no
posters for want of a TMDB key (#20), and his first library took the whole media folder (#16).
[deployment-ease-2026-11-08.md](deployment-ease-2026-11-08.md) and the audit it cites are the
earlier round; its D448 §9 already adopted the outcome of the first proposal and was never built.

## 1. One button links the server to orpheum.cc

**Recommendation.** Build D448 §9 now, as the paid service's spec §3.11 "Claiming from the core"
already describes it: the server's Remote access has one **Link to orpheum.cc** button; pressing it
asks for the admin's password once, opens account.orpheum.cc in the same browser, where the person
signs in (or redeems the invitation and makes the account there), chooses the household's name if
it has none, and approves *this server*; the browser comes back to the server's page, which shows
the account's address in full and one **Allow**; then the server is linked. The key, Connect and
Claim go from the page; the typed key stays as a fallback behind "Link another way". Record it as
D451, which D448 §9 reserved for exactly this.

**What the person does today** (after the invitation e-mail): 12 steps in 3 places.

| # | Where | Step |
| --- | --- | --- |
| 1–3 | e-mail, account.orpheum.cc | open the invitation, make the account, open the verification link |
| 4 | account.orpheum.cc | choose the realm (D298 §2) |
| 5–6 | account.orpheum.cc | press Make a key, type the account's password again (D312 §1) |
| 7 | account.orpheum.cc | copy the key (80 bits, an hour) |
| 8–9 | the server, at home | Manage → System → Remote access → Connect, with the server password (D391 §3) |
| 10–12 | the server, at home | paste the key, type the server password again (D297 §3), press Claim |

Then each person signs in once at home, so the server can make their realm key. The tester stopped
after step 4, and nothing said a step was left.

**What they would do:** 6 steps in 2 places, and the e-mail only if the account is new.

1. On the server, at home: System → Remote access → **Link to orpheum.cc**.
2. Type the server password (the only time).
3. At account.orpheum.cc, opened by the server: sign in, or "I have an invitation" → make the
   account (verification as now: the page waits for the e-mailed link and goes on by itself).
4. Choose the household's name, if the account has none (D526's word).
5. **Approve** the server the page names, with its name and fingerprint.
6. Back on the server's page: the account's address in full, **Allow**. Linked.

**The account page** says, as its first line until a server is linked: *"Your server: not linked
yet. On your server, at home, open Manage → System → Remote access and press Link to
orpheum.cc."* Today it says "No server is enrolled with your account yet." in a section below the
realm, after the getting-started text; the operator panel's Accounts table keeps its Server column.

**How it works.** It is OAuth's authorisation-code pattern, not the TV's device code. A device code
(D293 §2, `internal/core/discovery/devicecodes.go`, connectd's `internal/devicecodes`) suits a
device with no browser that shows a code typed elsewhere; here the admin has a browser at home, and
a redirect with a bound value resists the remote phishing a typed code invites (RFC 8628 §5.4).

- **Core, `internal/core/api/discovery.go`.** A new `linkDiscovery` beside `claimDiscovery`:
  admin only and from home (D297 §6, D300 §3), checks the password first as `claimDiscovery` does;
  enrols with the suggested discovery if the core is not enrolled (today's Connect, the password
  satisfying D391 §3); asks the discovery for a link token; returns the account page's address with
  the token and a one-time `state` the browser keeps. A second call, on return, takes `state` and
  the Allow, and finishes.
- **Core, `internal/core/discovery`.** `Link` beside `Claim` (`claim.go`): a signed call for the
  token, then the status the core already asks for (`askStatus`) shows it claimed with its realm.
  **The password** stays in the core's memory, as a `secrets.Value`, from the press until the claim
  or the token's expiry (ten minutes); at the claim the core makes the admin's realm key from it
  (`Auth.MakeRealmKey`, D297 §3, salted by the master realm D298 §4) and drops it. It never reaches
  disk or the discovery. (The other way, asking the password on return instead, would need no
  holding but would let a press enrol without it, amending D391 §3; I recommend holding it.)
- **`discovery/discoveryproto`.** Two paths: the core asks for a link token; the account's
  approval is reported back through the core's status. Shared with connectd through its
  `ORPHEUM_PIN`.
- **`discovery/single`.** Unchanged beyond the shared types: an own discovery has no accounts
  (D298 §1) and keeps its claim code. The button shows only where the discovery's signed address
  document names an account origin (D391 §3), so a self-hosted household never sees it.
- **orpheum-connect.** A link token (128 bits, ten minutes, one use) per core, stored by its hash;
  an approval page at the account origin that carries the token through sign-in, invitation
  redemption, verification and the household's name; the approval recorded where the account sees
  it (D312 §1); the spec's §3.11 measures (A15, B7): the warning "continue only if you just pressed
  Link on your own server", a core moving between accounts only with the holding account's consent
  or a seven-day wait, claims counted per account. The key path (`internal/core/auth/corekeys.go`)
  stays for the fallback.
- **Web client.** `system-discovery.tsx`: the Connect and Claim forms give way to the button and its
  password dialog; a return route for `state`; the key form behind "Link another way". Strings in
  `locales/en-system.ts`.

**Cost.** My estimate: core and web about a day with tests; connectd one to two days (a migration,
the page through its four entry states, the measures); an end-to-end run on staging with a fresh
account. Both repositories ship together, so it waits for a connectd deploy to production, the owner's step.

**Security.** Better than the key on the whole: the token is longer, shorter-lived, never typed or
read aloud, and bound to the admin's browser by `state`, so a stranger cannot hand the admin a code
or slip an approval into their flow. The risk the key did not have: an attacker who presses Link on
*their* server and talks a victim into approving it would point the victim's household at the
attacker's server; the approval page's warning, the server named with its fingerprint, the
seven-day rule and the account's record of enrolments (D312 §1) are the answers, as spec §3.11
planned. The password held in memory for up to ten minutes is the cost of typing it once. Admin
stays at home (D297 §6); the account's reconfirmation (D312 §1) is met by a sign-in made in the
same flow, or asked for when the session is older.

**Not changed.** Each other person still signs in once at home before their household sign-in works
(their key needs their password, and the server keeps only hashes, D297 §3); the server's sign-in
page could say so, separately.

**Decisions it amends:** D310 §5 and D298 §5 (a key or code entered on the core), D334 §3 (the
terminal asking for the key), D312 §1 and D318 (what a key's making asks and how keys coexist, now
for the fallback alone), D391 §3 only as worded for the Connect form; fulfils D448 §9 and D290 §1;
keeps D297 §3 and §6, D298 §4, D300 §3 and D292 §3's operator fallback.

## 2. A TMDB key ships with each release

**Recommendation.** The owner registers Orpheum with TMDB once, as Jellyfin's and Kodi's authors
did, and asks TMDB in writing whether a key shipped in a free, self-hosted server, with a separately
paid discovery that never calls TMDB, is non-commercial use. On a yes, the key goes in the source as
OpenSubtitles' does (D224 §7), opened into a `secrets.Value` at the TMDB client so it still never
prints (D194 §5); Settings' TMDB field stays as an override. Posters then come with no step at all.

**What the person does today:** 5 steps in 2 places, found only if they read the System page:
make a TMDB account, verify it, apply for an API key (a form about the use), copy it, paste it into
Settings. The guide does not say so (#20). **What they would do:** nothing.

**What TMDB's terms say** (read 2026-11-19):

- [Terms of use](https://www.themoviedb.org/api-terms-of-use) (updated 2023-10-20): §2.A, no
  commercial use without a separate written agreement, TMDB deciding what is commercial; §3, the
  TMDB logo and the notice "uses TMDB and the TMDB APIs but is not endorsed, certified, or
  otherwise approved by TMDB"; §1.A, the licence is non-transferable and non-sublicensable; §1.C,
  no caching past six months and no excessive bandwidth. **Nothing on embedding or sharing a key.**
- [FAQ](https://developer.themoviedb.org/docs/faq): free for non-commercial use with attribution;
  "commercial if the primary purpose is to create revenue for the benefit of the owner".
- [Rate limiting](https://developer.themoviedb.org/docs/rate-limiting): "somewhere in the 40
  requests per second range", may change, respect 429; the old 40-in-10-seconds limit was removed in
  December 2019. TMDB staff on its [forum](https://www.themoviedb.org/talk/5317af69c3a3685c4a0003b1)
  say the limit is by IP, not by key: each household's server has its own allowance, so one shared
  key does not pool them.

Orpheum already shows the notice in About (`locales/en.ts`, `tmdbNotice`), keeps data 180 days
(`tmdbRefreshAfterDays`, D213 §3) and paces to 40 a second (`tmdbRequestsPerSecond`, D231 §5).

**How others ship theirs.** Jellyfin: a constant in source,
`MediaBrowser.Providers/Plugins/Tmdb/TmdbUtils.cs` (`ApiKey`), with a `TmdbApiKey` override in the
plugin's configuration, kept off the settings page. Kodi: a constant in its TMDB scraper's source
(`python/lib/tmdbscraper/tmdbapi.py`, `TMDB_PARAMS`), with no override. Neither injects at build.

**In source or injected at build.** In source, as recommended: the key is readable from any
released image either way (`strings` finds it), so injection hides nothing from a user; it would
leave forks, development builds and the tests' fake without posters, and put a release secret in CI.
Injection's one gain is keeping the key out of code search, which matters only if TMDB revokes keys
found there; it has not done so for Jellyfin's or Kodi's. `secrets.Value` keeps rule 12's point,
that it never prints, without pretending it is secret.

**Owed by the owner:** register the application under the project's account (not personal), with
the paid discovery described; the written answer on commerciality; the key. If TMDB says
commercial, the choice is an agreement or keeping today's user-supplied key, and the guide then says
so plainly at install.

**No key at all.** D497 already gives every video a screenshot made by the node as a low-priority
task, shown where a poster would be, and "No poster" until then. What is lost without a key is the
match itself (title, year, overview, cast), not only the picture; a poster cropped from a frame is
possible but would look like a match when there is none, so I suggest D497's screenshot stays the
fallback, with the System page's notice pointing at the override.

**Security.** A shared key can be lifted and abused, and TMDB could revoke it for every install at
once: the Settings override is the way out, and a release can carry a new key. The key reaches no
one but TMDB; requests still go from each server's own address, so nothing passes through us (D13).

**Decisions it amends:** D16 ("provider API keys are always supplied by the user", already amended
for OpenSubtitles by D224 §2 and §7), with D224 §7 as the precedent; keeps D194 §5, D13, D497.

## 3. The library form offers one library per kind of subfolder

**Recommendation.** When the folder chosen, or a node's configured root, holds subfolders, the New
library page opens on them: *"Your media folder holds these folders. Make a library of each?"*, one
row per subfolder with a tick, a name (the folder's) and a kind guessed from the name, shown and
changeable; one press makes each ticked library. Taking the whole folder stays possible, below,
with a warning when it holds folders of different kinds.

**What the person does today:** Manage → Libraries → New library; name; kind; the configured root
is offered ticked-or-not as the one visible folder, and a subfolder needs Browse inside, a dialog
(D359 §1, D361 §3); then repeat the page for each library: about 6 steps a library, so 12 for films
and series. Ticking the root is the shortest path, and it is the one the tester took (#16).
**What they would do:** open New library, check the two rows it offers (Movies as films, TV Shows
as series), press Make these libraries: 3 steps for any number.

**How it works.** Web client only. `create-library.tsx` already lists a folder's folders through
`GET /nodes/{name}/folders?path=` (D359 §2), which the node answers within five seconds. A table
of names to kinds, case-insensitive and in the shipped languages: Movies, Films, Film → films;
TV, TV Shows, Shows, Series → series; anything else unticked, kind "other". Each ticked row is the
existing create call, one per library; a refusal names its row. The warning on the whole folder
fires when its subfolders match two kinds. Every library made here takes the first library's
rescan default, which also settles #19.

**Cost.** About half a day with tests; no core, node or connectd change.

**Security.** None new: folders stay within the node's configured roots (D107, D359 §1).

**Decisions it amends:** D359 and D361 §3 (how the folders are chosen; extended, not withdrawn).
D183's rule holds: the guessed kind is a default that is visible where it acts and harmless to get
wrong (change the kind and the library rescans), and the metadata source is still asked.

## For the owner to answer

1. One button linking the server, held password, recorded as D451? Or ask the password on return?
2. Register with TMDB and ship the key in source, once TMDB answers on commerciality?
3. Offer one library per kind of subfolder, kind guessed from the folder's name?
