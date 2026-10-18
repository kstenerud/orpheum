> **ABOUTME:** The owner's stock-take of 2026-10-18: every feature the competitors have and every
> one Orpheum plans, put into proposed milestones for closed-beta households, with the owner's
> decisions so far, the ordering questions only the owner can answer, and the plan documents that
> have fallen behind the code. A proposal: nothing here changes cut-a.md until the owner decides.

# Milestone reassessment, 2026-10-18

## Why now

M0–M4 are built, and so is a track no milestone names: the paid discovery, the loader that runs
the core's own client, realms, the WebRTC transport, live on nas.example since 2026-10-17 (D273–D309).
Against the field ([feature-standing-2026-10-18.md](feature-standing-2026-10-18.md)) Orpheum leads
on remote access, Watch Together with voice and adaptive quality, and is behind on the TV, the time
between episodes, profiles, the title page and some picture formats. The owner, 2026-10-18: "We
have a solid base, and now it's time to take stock of all features (competitors, and our planned
features) and prioritize them."

## The owner's decisions so far (2026-10-18)

1. **The UI revamp (cut-a's M5, D245) waits until the features are in:** "doing the ui revamp
   would risk handcuffing us." The skin's first tier (D251) stays; the rest comes after.
2. **Intros are detected automatically**, not hand-marked: skip intro and credits are D82's media
   segments, found from the files.
3. **The next milestones are for closed-beta households:** what another household needs to install,
   reach, trust and enjoy it.
4. **The TV without app stores, for now:** "I'd like to put off the app store hell, so if there's a
   way to support at least some TVs without apps, that would be ideal." The Fire TV's browser
   already reaches Orpheum and plays films.
5. **Music comes into scope** (D1 said "audio second"; D186 kept its schema open).
6. **Plugins and an API: the possibility, not the implementation, at the closed beta.** The API is
   kept stable enough to build on (D85's compatibility gate), and nothing is built on it yet.
7. **The discovery we run today matches neither outcome the owner wants:** "(a) a standalone
   single-core discovery server that users can run on their own VPS, and (b) my paid tier with
   users and easy registration and eventually billing, connected off the orpheum home page."
   Today (a) exists only as a package (`discovery/single`) behind `orpheum dev discovery`, its
   command (W4b.8) and its serving of the web app (W4b.5) planned "after the beta" and waiting on
   discovery-mode's Q3 and Q5; and (b), `connectd`, is run by its operator: cores claimed by their
   code from the console (P4), realms given by the command line, no customer accounts (P2), no
   invitations (P3), no billing (P14), nothing on orpheum-app.com that leads to it.

To record as decisions once the order below is settled.

8. **The closed beta runs on the paid service, free; the self-run discovery waits for the open
   beta** ([D310](../../decisions/working-notes.md#d310--the-closed-beta-runs-on-the-paid-service-free-an-account-gives-one-key-for-one-core-the-cores-admin-enters-it-at-home-and-the-household-then-uses-the-services-app)):
   a beta household's account is free for life; it makes the account itself and is given one key
   for one core; its admin enters the key at home on a core that is very simple to set up; then the
   household uses app.orpheum-app.com. The owner tests lapse and termination on their own accounts.
   So Db joins B0, and Da moves to the open beta (question 6 answered).

## Proposed milestones

Each milestone is what a beta household would notice, in the order proposed. Sizes are rough:
S days, M a week or two, L several weeks, XL longer. Sources are the plan or decision that already
describes the item; *unplanned* means only the feature-standing audit names it.

### B0: a household can join the beta

What blocks the first outside household. Nothing here is visible to the owner's own household, but
nothing after it reaches anyone else without it.

| Item | Source | Size |
| --- | --- | --- |
| Testers' installation: an image a tester can run, and a deployment that needs no VPS tunnel | discovery-core Phase 0; beta-inventory-2026-10-09 (d)2–3; D272 §3 (*undesigned*) | L |
| Device codes through the paid service (K15, M-S5) | D293 §2 | M |
| A recovery code for the server's identity key (K8a) | D292 §4 | M |
| ICE restart; network change events (K3b, K4) | discovery-core Phase 3 | M |
| The System page in plain words, first-run form, home address (K12); counting dead ends (K16) | discovery-core | M |
| Bug reports from inside the app (P10) | bug-reports.md; D74 | M |
| Log levels changeable at run time | the owner, 2026-10-15 | M |
| The API compatibility gate, so the API stays something to build on | api-compatibility.md; D85 | S–M |
| Terms, privacy policy, Impressum | service.md P13 | ? |
| M4's close: the 30-minute netem acceptance (VQ24) | m4.md | S |

### D: the two discoveries

The owner's two outcomes (decision 7), each a product of its own. Which of them a beta household
meets first is question 6.

**Da: a discovery a household runs on its own VPS**

| Item | Source | Size |
| --- | --- | --- |
| The command: `orpheum discovery`, which gives the core's key (D310 §7), and re-keying | discovery-mode.md W4b.8; Q5 | M |
| Serving the web app and the loader at the household's own domain | W4b.5; D274 §5; Q3 | M |
| Names and certificates at the household's domain, by its DNS provider | discovery-mode Q2 | M |
| Its own STUN and TURN on the VPS | W4b.3; W6 | S–M |
| An install a household can follow: image, VPS sizing, DNS, the claim | discovery-core Phase 0 (*undesigned*) | M |

**Db: the paid service, with accounts, from the home page**

| Item | Source | Size |
| --- | --- | --- |
| Customer accounts, signed in by username and password (a passkey later), the account's panel with its server and *Generate key* | service.md P2; D289 §1; D310 §8–§13 | L |
| Enrolment: a one-use safe32 key from the account, typed into the core at home within the hour; a new key moves the account to a new core | D310 §5, §8–§9; D311 | M–L |
| Invitations for testers, from the operator's admin panel, with notes, issued again to pass an account on | P3; D310 §10; D311 | S–M |
| Realms chosen by the household, not given by the operator | D297, D298 (code entry waits for accounts) | M |
| The service whole (probe, names, caps, client integrity, lapse) | M-S1–M-S4, M-S6 | L |
| Billing: a merchant of record, monthly and yearly, a trial, lapse | P14; Q7; D273 §9 | L, later |

### B1: the living room, without app stores

The TV through what a TV already has. Three paths, none needing a store listing:

- **The TV's own browser** (Fire TV's Silk proven by the owner; Samsung, LG and Android TV have
  browsers): a "TV mode" of the web client, driven by the remote's arrows, with a ten-foot layout
  and sign-in by a code the TV shows (RFC 8628).
- **Google Cast from the browser:** a Cast receiver is a web page registered with Google, not an
  app-store app; the receiver is a stub today.
- **AirPlay from Safari**, which a `<video>` element offers for free: to be checked, not built.

| Item | Source | Size |
| --- | --- | --- |
| TV mode: remote navigation (roving focus), ten-foot layout | m5.md Q5 (answered: wanted); *unplanned as a TV mode* | M–L |
| TV sign-in by a code the TV shows (RFC 8628) | D93, D288 | M |
| Cast receiver: plays, levels loudness, follows Watch Together | cut-a M6; D226 §4; D88, D135–D137 | L |
| Voice on a second device as the same person (film on the TV, voice on the phone) | watch-together.md; D120; cut-a M6 | M |
| "Get this ready" and the session-start prepare | cut-a M6; m5.md | S |
| The remote person's TV, end to end (cut-a's M6 acceptance) | cut-a M6; D188 §3 | — |
| Checked on Fire TV, a Samsung or LG browser, Chromecast, AirPlay | device-lab.md (*planned*) | M |

### B2: between episodes

What every streaming service has made a reflex.

| Item | Source | Size |
| --- | --- | --- |
| Next Up: the Home row and a series' next episode | play-state.md; cut-a §1 lists it in Cut A | S–M |
| Media segments, detected: chapters first, then audio fingerprints across a season for intros and credits | media-segments.md; D82, D115; the owner, 2026-10-18 | M then L |
| Skip intro, skip recap, skip credits | D82 | S after segments |
| The next-episode prompt at the real ending, with a countdown | D82, D139 | S after segments |
| Chapters in the player | media-segments.md | S |
| Trickplay thumbnails on the scrub bar | D102, D114 (*no plan file*) | M |

### B3: a library worth browsing

| Item | Source | Size |
| --- | --- | --- |
| Cast, crew, genres and ratings on the title page; people pages | D95, D171, D172 (*a gap in the title page*) | M–L |
| Browse filters and more sorts | *unplanned* | S–M |
| Watched badges; mark watched and unwatched | *unplanned* | S |
| Watchlist, favourites, own rating | D140 §3, D150 §3 (tables exist) | S |
| Collections, made by hand first | D75, D159, D160 | M |
| Episode orderings (DVD, absolute) | D96, D210 | M |
| Missing episodes, duplicates, folder browsing | D187, D116, D176 | S–M each |

### B4: a household of several people

| Item | Source | Size |
| --- | --- | --- |
| Profiles: who's watching, a PIN to switch | D150 (*picker unplanned*) | M |
| The rules engine, which the next three need | rules-engine.md; D75 | XL |
| Parental controls | parental-controls.md; D84, D130–D134 | XL |
| Ceilings per role (quality, streams) | D180 | M |
| The sharing page | sharing-model.md; D76 | M |

### B5: every file plays well

| Item | Source | Size |
| --- | --- | --- |
| HDR tone mapping to SDR | transcoding.md (*unplanned beyond mentions*) | M |
| VAAPI and AMD encoding | transcoding.md | S–M |
| Bitmap subtitles (PGS, VobSub): shown or burnt in | D23, D78 | M–L |
| Caption style controls; playback speed | D90, D137; D46 | S–M |
| Language defaults per person; forced subtitles | D23, D24, D138 §7 | M |
| The playback survey: how often files play directly | playback-survey.md (*never run*) | M |

### B6: music

D1's "audio second", now in scope. Its own research exists ([competitors-audio.md](competitors-audio.md):
Navidrome, Plexamp, OpenSubsonic). Size XL; its shape is a question below.

### Then: the UI revamp, and the public release

- **The UI revamp** (cut-a's M5, m5.md), over the features above.
- **Before a public release:** translations (D45–D58), the accessibility floor as a gate (D90),
  pinning and auditing every dependency (D220), the first-run legal notice (D249 §5).
- **After the beta, on demand:** plugins and the API's clients (D12, D15, D18), store apps (tv-client.md,
  D10), offline download (D87), 2FA, passkeys and OIDC, our own relay (D274 §2), billing and
  accounts (P2, P3, P14), remote media nodes, rebroadcast (D177), live TV (out of scope).

## Questions for the owner

1. **B1 or B2 first?** A beta household judges the TV first, and the episode furniture every
   evening after. B2 is smaller and lands sooner; B1 opens the living room.
2. **How much of B0 before the first outside household?** All of it, or a thinner first beta (an
   install path, recovery code, bug reports) with the service's thickening during it?
3. **Music's shape:** a music library in the same app, or an OpenSubsonic API so existing music
   players (Symfonium, Feishin) work with it? The second fits "the API's possibility" and is far
   smaller.
4. **The rules engine (B4) is XL and gates parental controls, sharing and smart collections.**
   Profiles and a PIN can come without it. Is a household of children a beta need, or later?
6. ~~**Which discovery first, for the beta?**~~ Answered by D310: Db for the closed beta, Da for the open.
   *(As asked:)* **Which discovery first, for the beta?** Da lets a technical household join with no account
   anywhere; Db is the path for everyone else, and the one billing grows from. Both share the
   protocol and the core's side, which are built.
7. **Is there anything here you want sooner, or not at all?** This list is the field's and the
   plans'; your own wants weigh more.

## Plan documents behind the code

Found while taking stock; to fix once the milestones are decided, as one change:

- **cut-a.md §8 has no milestone for the beta track** (D273–D309), and still names M5 the polished UI.
- **ROADMAP.md** still puts the cast receiver and "her television" in Cut A, describes reachability
  as port forwarding then IPv6 then a tunnel (replaced by D273, D274), and presents a playback
  survey as done first that was never run.
- **Status lines behind the code:** client-from-core.md (D308 is live), release.md (R8 says not
  started; nas.example runs v0.2.0-nas.10), webrtc-transport.md (W2d is built),
  discovery-core.md (its header says not started), discovery-mode.md, m4.md (still in progress),
  remote-access.md (superseded).
- **Contradictions:** "Get this ready" is M5 in m5.md and M6 in cut-a and D245; m5.md's handed-on
  list names per-person volume and guest rights as missing, both built (D281 §4, D271 §4); text chat
  is declined by D63 and D69 and wanted by D188 and D246 §5; cut-a §1 lists device pairing as out,
  but device codes are built (D218, D288).
- **plan-deviations.md stops at P56 (2026-10-08):** the beta pivot, the data-layer rebuild
  (D303–D305) and the M5 deferral are not recorded.
- **Next Up is "in" Cut A** but has no milestone row and is not built.
- **Every kind of code, against D311:** core invitations, Watch Together links and device codes can
  be withdrawn while outstanding, but none can yet be issued again for an entitlement already made
  (the escape hatch).
