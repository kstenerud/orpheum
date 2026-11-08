> **ABOUTME:** The second ease-of-use audit of the discovery design, after D290 and D291: each journey walked step by step as the decided design and the built web client now have it (setup, claiming, sign-in links, device codes, re-key and recovery, a forgotten server, Watch Together, the dead end, the LAN prompt), compared with Plex, YouTube, Apple, GitHub and others. Done 2026-10-13.

# Discovery audit, second round: ease of use, setup and recovery (2026-10-13)

**Markers.** **[V]** is checked at a source under *Sources*, or in this repository's code at the
path named. **[I]** is my inference. `D<n>` cites a decision. "Protocol" is
[discovery-protocol.md](../discovery-protocol.md). "Spec" is the paid service's specification,
draft 5 (the shared files' `discovery-service-spec.md`). "The plan" is
[discovery-core.md](../plans/discovery-core.md). "First round" is
[discovery-audit-ux-2026-10-13.md](discovery-audit-ux-2026-10-13.md), read for context only; nothing
in it was trusted. Severity: **high** means a common journey ends in a dead end or a household-wide
outage; **medium** means extra steps, false alarms or confusion; **low** means polish.

Fetched pages are cached under `/yoloai/cache/discovery-audit2/`.

## Summary

D290 and D291 fixed the first round's worst points on paper. But several security choices, each
sound alone, combine into dead ends that no single document shows.

1. **The one link a person surely has stops working.** A registration link is single use at the
   discovery as well as on the core, and its locate is spent by a click before the person registers.
   So "Open in Safari", a closed tab, a wiped Safari and a new phone all end at "This link does not
   work". The link that is still in their messages should keep leading home.
2. **"Add to Home Screen" starts empty on iPhones.** A home-screen web app has its own storage, apart
   from Safari's, by Apple's design. The remedy D290 §10 chose for Safari's seven-day wipe opens the
   no-core page, with no locator and no session. QR codes scanned with the camera open Safari, not the
   home-screen app, so they miss it too.
3. **A restore from backup becomes a 72-hour household outage**, with a changed-key warning or the
   wrong dead-end message on every family device. This becomes the default disaster once scheduled
   backups leave out `identity.key` (D289 §2). Today they include it; the plan says the opposite.
4. **The owner's first setup has a missing screen and two e-mail loops.** The web client has no form
   for the setup code. On Safari, the claim page the core "opens" after an API call is a blocked
   pop-up. The claim token lives minutes, while a beta tester's invitation sends a second e-mail.
   The same person then confirms twice, the second time against a masked address.
5. **Code-activated devices with "ordinary rights" (D291 §5) are undefined at the edges.** The owner's
   own TV loses admin. The client still shows admin menus and answers "Only an administrator can do
   that" to an administrator. Getting admin back means typing a password with a remote.

The rest is medium or low: a dead-end message shown for causes that are not the network, an
"at home" guess that is wrong in both directions, location warnings that fire on the common, harmless
case, and step-up prompts that would add a third e-mail loop.

## The journeys, counted

Steps are actions; decisions are points where the person must choose or understand something. Counts
are for the design as decided, with the built client where it exists. All **[I]**.

| Journey | Steps | Decisions | Surfaces | Worst point |
| --- | --- | --- | --- | --- |
| Owner's first setup, paid beta (install → own phone on `app.`) | ~14, plus 3 per person invited | 4 | terminal, the core's page, mail (twice), `account.`, phone | no setup form; a blocked pop-up; the claim token runs out during the e-mail loops (U2-4) |
| Customer's sign-in link opened on another device | 4 (ask, open mail on phone, read code, type it on the laptop) | 1 | two devices | the waiting laptop tab must still be open |
| TV by code, own core (D288 as built) | ~8 (address with a remote, *Get a code*, phone menu, account, code, name, compare, confirm) | 2 | TV, phone | finding *Activate a device* |
| TV by code, paid tier (decided) | ~9, plus a location to judge | 3 | TV, phone | "another network" fires when the phone is on mobile data (U2-9) |
| Owner wants admin on their code-activated TV (D291 §5) | not designed; at best the password typed with a remote | 1 | TV | admin menus that refuse an admin (U2-5) |
| Restore from a scheduled backup, once D289 §2 is built | re-key button, `account.`, step-up, then 72 h | 2 | the core's page, mail, dashboard | the whole household is out for three days (U2-3) |
| Family member whose Safari forgot the server | 5–7, and a wait for the admin | 2 | phone, a message to the admin | the original link says "used" (U2-1) |
| Family member who added Orpheum to the home screen | 2 to discover it is empty, then as above | 1 | phone | the icon opens the no-core page (U2-2) |
| Watch Together guest who closed the page | 1 (tap the link again), same browser | 0 | one device | another browser gets "used already", with no way back but a new link (U2-11) |

---

## Findings

### U2-1 — The link a person surely has stops working after one use (high)

**Journeys:** a new family member; a Safari that forgot; an in-app browser; a new phone.
**Sections:** protocol §3.1 (use policies, "spent by a click"); D290 §10, §17; D291 §6 item 15
("Open in Safari"); the plan's K7 ("a registration link's hash goes once it is used").

**What the design says.** A registration link's locate-hash is single use. `/locate` is spent by a
click on the page, and returns a bootstrap token that lives "a few minutes". The person then
registers over the direct path. A Watch Together link and a person's own link are reusable, because
"locating grants nothing" (D290 §17). The registration link is not, though the same reason holds.

**What the person experiences.** [I] Four common cases end at the same dead end:

- **The in-app browser.** D291 §6 adds "Open in Safari" when an in-app browser is detected. If it
  shows after the click that located the core, Safari opens a link whose locate is spent. If it
  shows after registering, the core has spent it too. Both say "This link does not work"
  (`register.badLink`, which also tells them to ask for a new one).
- **A tab closed before registering.** The click spent the locate; the bootstrap token expired; the
  registration itself was never used. The link is dead at the discovery and alive on the core.
- **A Safari that forgot, or a new phone.** The no-core page says "Open the link you were sent"
  (protocol §3.1). The link they were sent is the registration link. It is spent.
- **Grandma's iPhone is her only device**, so "sign in using a phone where you are already signed in"
  does not apply to her. Her way back is a message to the admin and a wait.

**What comparable products do.** Nabu Casa gives each Home Assistant a stable remote address (a long
random name under `ui.nabu.casa`) that the owner can bookmark and reuse **[V, search summary]**.
Synology's QuickConnect ID is reusable by design **[V, first round]**. None of them make the first
link a person receives stop leading to the server.

**Fix.**
- **Let a registration link become the person's own link once used.** After registering, its
  locate-hash stays live with the "reusable, locates only" policy of `/s#`, until the admin revokes
  it. The page it opens then shows sign-in, not "used". Locating grants nothing (D290 §17), so this
  costs nothing in security. The person needs to keep nothing new.
- **Make the locate reusable while the registration is unused.** The core already spends the link
  when someone registers (K7's test). One use, enforced once, on the core.
- **Show "Open in Safari" before the click that does anything**, on the link's first page.
- **Word the used-link page by what happened:** "This link was already used to make Anna's account.
  Sign in below." Not "ask for a new one".

### U2-2 — "Add to Home Screen" starts with nothing, and QR codes open Safari, not the app (high)

**Journeys:** any iPhone user who follows D290 §10's advice; a TV activated by QR from an iPhone.
**Sections:** protocol §3.1 ("Staying remembered"); D290 §10; webrtc-transport.md W4c and its VQ
("the home-screen app finds the core a week later").

**What the design says.** iPhones are offered *Add to Home Screen*, since home-screen apps keep their
own seven-day count. The page asks for persistent storage where the browser allows.

**What the person experiences.** A home-screen web app on iOS does not share storage with Safari.
WebKit's maintainers closed the question in 2022: "The current behavior (on Apple platforms) is by
design. Home Screen apps are created as isolated entities without shared state with the browser"
**[V, WebKit bug 181849]**. So the person registers in Safari, adds the icon, taps it, and the app
opens with no locator and no session: the no-core page **[I]**. W4c's VQ would fail on its first day,
not after a week.

The same isolation breaks the QR codes. The phone's camera opens Safari. A person whose Orpheum lives
in the home-screen app scans the TV's QR (D291 §6 item 20) or their own link's QR, and lands in a
Safari that is not signed in **[I]**.

Persistent storage does not help a Safari tab: WebKit grants `persist()` "based on heuristics like
whether the website is opened as a Home Screen Web App" **[V, WebKit storage policy]**.

**What comparable products do.** Native apps (Plex, Jellyfin, Emby) avoid this by being apps. Web
apps that live with it pass state across by Cache Storage, which iOS shares between Safari and the
home-screen app, or have the person sign in inside the app **[V, search summary of the WebKit bug and
developer write-ups]**.

**Fix.**
- **Offer Add to Home Screen from the person's own link page**, and make the installed app start
  there. The icon must open `/s#<coreId>.<secret>`, or the app must find the link another way. Test
  on a device which of these iOS keeps: a manifest `start_url` set per person (a manifest made at
  run time), the current URL's fragment, or the locator copied through Cache Storage, which both
  sides share **[I, verify on iOS 18 and 26; a VQ item]**.
- **In the home-screen app with no core, offer a paste box** ("Paste your server link") beside the
  device code. Copying a link from Messages is a thing people already do.
- **Make the TV's QR useful from a signed-out Safari:** its page says "Open Orpheum from your home
  screen and enter K7XQ-2HPM under Activate a device", with the code large. Do not show a sign-in
  form there.
- **Drop "asks for persistent storage" as a remedy for Safari tabs** in the protocol; keep it for the
  home-screen app, where it is granted.

### U2-3 — A restore from backup becomes a three-day outage for the household (high)

**Journeys:** the owner's disk dies, and they restore last night's scheduled backup; the family's
devices during the wait and after it. **Sections:** protocol §2.1, §2.8, §3.5, §3.6; D289 §2;
D290 §3, §11; D291 §6; the plan's K8a, K8d; spec §2 (*Recovery*).

**What the design says.** Scheduled backups leave out `identity.key` (D289 §2). A core restored
without it detects the state and offers one button to re-key (D290 §11). Without the old key that is
a recovery: about 72 hours, mail at the start and end, a banner, and a step-up on the dashboard
(D290 §3, D291 §6). A client that remembered the old key "stops before it sends a password, with a
plain warning", and after a recovery shows "a plain notice that the server's identity changed"
(D291 §6).

**What is built.** Every scheduled backup holds `identity.key` today. The backup leaves out
`secret.key` alone and copies every other file in the persistent directory: the identity key, the
TLS key, the ACME account key and `discovery.json` **[V, `internal/core/backup/backup.go`
`leftOut`, `internal/core/instancedir/archive.go` `persistentFileNames`]**. So the outage below does
not happen yet; it arrives with K8a.

**What the person experiences, once K8a is built.** [I]
- **The owner** restores, presses *Re-connect*, signs in at `account.` (an e-mail loop), passes a
  step-up (perhaps another), and waits three days. Nothing they hold can shorten it.
- **During the wait the new key has no session.** No address updates, no signalling. A household
  only WebRTC reaches is unreachable from outside. One reached directly meets the new key on its
  first request, and every remembered client stops with a changed-key warning, at home as well.
- **The family** sees either that warning, or the dead-end message: "Can't reach *Stenerud home*
  from this network. Try your phone's mobile data". It sends them changing networks for a server
  that is being restored.
- **After the wait** every family device shows a notice that the server's identity changed. Nobody
  in the family can judge it. It is SSH's host-key warning, given to a grandmother.

**What comparable products do.** Apple's account recovery waits "several days or longer", and
"Contacting Apple Support can't help shorten the wait", but more "recovery factors" can, and a
recovery key replaces the wait entirely **[V, Apple 118574, 109345]**. So the wait is the fallback
for someone with nothing, not the path for someone with the right thing in hand.

**Fix.**
- **Give the owner a way to skip the wait.** At claiming, the System page shows a recovery code to
  print or save (Apple's 28-character recovery key is the model). A re-key with that code is
  immediate. A passkey on the dashboard could also shorten the wait. The 72 hours stay for an owner
  with only their e-mail.
- **Decide the backup policy knowingly.** Leaving the identity key out of scheduled backups trades a
  stolen-backup risk for a three-day outage after the commonest disaster. An option between the two:
  scheduled backups hold the identity key sealed under the recovery code.
- **Tell the family the cause.** `/paths` returns the core's state (U2-7): "Stenerud home is being
  moved to a new machine. It is back on Thursday at 14:00." No network advice.
- **After a completed recovery, accept the new key silently** for household members and show the
  notice to admins only. A warning nobody can act on teaches people to click through the one that
  matters.

### U2-4 — The owner's first setup has a missing screen, a blocked pop-up and two e-mail loops (high)

**Journeys:** the owner's first setup, paid beta. **Sections:** protocol §2.2, §3.1 (the table's last
row); spec §2 (*Sign-in for customers*, invitations), §3.11; D290 §1, §16; D291 §6 item 17; the plan's
K6, K12; `sign-in.tsx`; `deploy/release/README.md` step 6.

**The steps, as decided and built.** [I unless marked]
1. Run the image. 2. Run `orpheum setup-code` in a terminal. 3. Open `http://<host>:8440`. 4. Create
the first admin with the code: **there is no form for it**. The web client never calls `POST /setup`;
its sign-in page says only "No one can sign in until the first admin is created with the setup code
from the server's log" **[V, `web/apps/web/src` has no caller of `/setup`; `locales/en.ts`
`signIn.noAdminYet`]**. The release and staging READMEs tell the owner to "make the first admin" in
the browser **[V]**. D291 takes U5's fix (K12), but it sits in Lane D, not first.
5. System → *Remote access* → turn it on. 6. The core enrols, then opens the claim page.
7. The claim page needs a signed-in customer. A beta tester's invitation, opened, "sends a fresh
sign-in link then" (D290 §16): a second e-mail before the first sign-in. 8. Open that mail, often on
the phone, which shows a code to type back on the laptop. 9. Confirm "Add this server?".
10. Switch back to the core's tab; *Allow*. 11. Wait for the certificate and the probe.
12. Open the app on the phone: the owner has no locator there (below). 13–14. Invite the family.

**What goes wrong.**
- **The pop-up is blocked on Safari.** Step 6 opens a window after an `await` (enrolment). Safari's
  transient activation is short, iOS Safari's shorter: "any small delay, such as waiting for a Promise
  to resolve, is enough to miss this window" **[V, Don't Panic Labs; Ryan Thomson]**.
- **The claim token lives "a few minutes"** (protocol §2.2), and steps 7–8 are two e-mail round
  trips. Plex's claim token lasts four minutes, but Plex signs its owner in with a password, not a
  mailbox **[V, pms-docker]**. Nothing says what the claim page shows when its token has expired.
- **The owner's own devices know nothing.** The protocol's table says "the customer's own browser
  (paid): the core's System page, from which they claimed it". That page is the core's own origin.
  `app.orpheum-app.com` on the same laptop has no locator, and the phone has none. The spec dropped
  the customer's locator endpoint, so the owner too needs their own link, which nothing hands them.

**What comparable products do.** Plex claims a server when its signed-in owner opens it: "this will
initiate the process to claim the server" **[V]**. One sign-in, one click.

**Fix.**
- **Build the first-admin form first** (K12's item), on the core's own page. Pre-fill the code from
  the console when the browser is on the same machine; otherwise ask for it.
- **Open the claim tab inside the click:** `window.open()` at once, then set its address once the
  token arrives. Or show an *Open the claim page* button once the token is in hand.
- **Let the claim outlive the e-mail loop.** The claim page swaps its token for a pending claim the
  moment it loads, and that claim lasts as long as the sign-in link does. The System page offers
  *Open the claim page again*, which makes a fresh token, rather than a dead end.
- **Let a beta invitation sign the tester in on its click.** It proves the mailbox as a sign-in link
  does. Keep its 14 days, but spend it on the click and give a 24-hour session start (D290 §16's rule
  applies to the link, not to how the tester arrives).
- **End the claim with the owner's own link:** "Remote access is on. Open Orpheum anywhere:" the
  admin's own link, and its QR for the phone.

### U2-5 — Claiming asks the same person twice, the second time against a masked address (medium)

**Journeys:** the owner's first setup. **Sections:** protocol §2.2 items 2–3; spec §3.11; D290 §1;
D291 §6 item 5 (*This isn't me, release*; a typed code as fallback).

**What the design says.** The customer confirms on `account.` ("add it to your account?"). Then the
core's System page shows which account asks, **masked**, with *Allow* and *Refuse*.

**What the person experiences.** [I] In the normal case one person, in one browser, answers two
questions about the same act a minute apart. The check that matters is the second, on the core: it
catches a dashboard signed in as someone else. A mask such as `k***@g***.com` weakens exactly that
check. An attacker who knows the owner's address can pick one that masks the same. The person
reading it is the core's admin, who may see the address.

**What comparable products do.** Microsoft added number matching because people approve prompts by
habit, and showed context (the app and the place) to make the one prompt count **[V, first round's
number-matching page; Microsoft "additional context"]**.

**Fix.** One decision, on the core. The claim page says "Adding *Stenerud home* to
owner@example.com. Finish on your server's page." and returns there. The System page shows the
address in full: "owner@example.com wants to hold this server. Allow?". Show the waiting approval
in the admin's yellow-sign tray too (D280), in case the tab was closed. Keep *This isn't me, release*
in *Details*, not beside *Allow*.

### U2-6 — "Ordinary rights" for code-activated devices: the owner's TV, and what "ordinary" means (medium)

**Journeys:** the owner activates their own TV, then needs admin on it; a family member hosts Watch
Together from the TV. **Sections:** D291 §5; protocol §3.1 (placed under **Paid**); the plan's K15
and question 10; `internal/core/auth/pairing.go` (`AnswerPairing` issues a full session) **[V]**.

**What the design says.** A device activated by a code gets "playback and its own settings, never
admin, even when an admin entered the code; admin rights only once the person also signs in with
their password on that device". The protocol lists it under the paid tier's codes; D291 §5's words
do not limit it. The built core gives a code-activated device the person's full role **[V]**.

**What the person experiences.** [I]
- **The owner's TV shows admin menus that refuse an admin.** The client decides what to show by the
  person's role. The server would now decide by the session. The owner taps *System* and reads "Only
  an administrator can do that" (`problem.forbidden`).
- **Getting admin back means typing a password with a remote**, which device codes exist to avoid.
  The design does not say where that sign-in happens on a device already signed in.
- **"Playback and its own settings" is narrower than the role.** A person whose role may host Watch
  Together could not host from the TV, the screen they would host on. Downloads and fixing a film's
  information go too.
- **Own core or paid only?** Undecided (the plan's question 10). Two rules for one code would be two
  ways to do one job (AGENTS.md rule 11).

**What comparable products do.** GitHub asks for the password again only when a sensitive action is
attempted, then keeps "sudo mode" for two hours **[V, GitHub]**. YouTube and Netflix TV codes give the
TV the account's normal use **[V, YouTube help]**.

**Fix.**
- **Define "ordinary" as the role minus administering and minus account security**: making links,
  entering codes, changing the password, removing devices. Keep hosting, downloads and the rest the
  role gives.
- **The client reads the session's rights, not the role's**, and hides what the session cannot do.
- **Step up in place, when needed.** On an admin page: "This TV was signed in with a code. Enter your
  password to manage the server here." One field, the name already known. Most owners will never
  need it on a TV.
- **One rule for both tiers.** The phishing risk the rule answers is the same on an own core.

### U2-7 — The dead-end message is shown for causes that are not the network (medium)

**Journeys:** a family member away; a guest; any device while the core is off, recovering or lapsed.
**Sections:** protocol §3.2 (the `paths` answer), §3.6; D290 §12; D291 §6 item 18 (U7: "household
members told the cause"); spec §3.10.

**What the design says.** When nothing reaches the core: "Can't reach *Stenerud home* from this
network. Try your phone's mobile data, or another Wi-Fi. If it keeps happening, tell *Karl*."

**What the person experiences.** [I]
- **The same words for every cause.** The core may be off, mid-restart, recovering (U2-3), frozen
  for a lapse, or blocked by the person's own "Block" on Chrome's prompt. The message sends them to
  change networks in each case. The discovery knows several of these: liveness is the signalling
  connection's presence (protocol §2.4), and it knows the lapse and recovery state. But the `paths`
  answer has no field to say so. D291 took "household members told the cause" with nowhere to carry it.
- **The names are unknown on a first visit.** The discovery holds no household data, so a guest or a
  newly registered device that has never reached the core does not know "Stenerud home" or "Karl".
- **"Try your phone's mobile data"** is wrong on a phone already on mobile data. Safari does not
  expose the connection type.

**What comparable products do.** GOV.UK: say what happened and how to fix it **[V, first round]**.
YouTube's TV help names the condition it needs ("the same WiFi network") rather than a generic retry
**[V]**.

**Fix.**
- **Add a `state` to the `paths` answer:** `online`, `offline` (since when), `paused` (lapse),
  `recovering` (until when). Each has its own sentence: "Stenerud home is offline since 14:02. It is
  not your network." The dead-end message is shown only when the core is online and no path works.
- **Use the names the device remembers**; otherwise "the server" and "the person who sent you this
  link".
- **Condition the advice:** "If you are on Wi-Fi, try mobile data; if you are on mobile data, try
  Wi-Fi."
- **When Chrome's permission is denied and the core is at home,** say that, with the help page
  (D291 §6 item 14), not "try another network".

### U2-8 — The "at home" guess is wrong both ways, and the browser can be asked instead (medium)

**Journeys:** a family member at home; a guest on the same provider. **Sections:** protocol §3.2,
§3.3; D290 §2; D291 §6 item 14; webrtc-transport.md W2c.

**What the design says.** The LAN path is returned only when the request's source equals the core's
public IPv4, or shares its IPv6 /56 or /64, or the device remembers that the LAN won on this network.

**How it guesses wrong.** [I unless marked]
- **False "at home": a shared IPv4.** Behind carrier NAT or DS-Lite, the core's IPv4 as the
  discovery sees it is shared with many other customers. A guest of the same provider matches and
  meets Chrome's prompt. A "Block" then sticks for `app.`, every household's.
- **False "at home": the guest Wi-Fi** of the same router is another /64 in the same /56.
- **False "away": Safari with iCloud Private Relay.** Its traffic leaves through Apple's relays, so the
  discovery never sees the home's address **[V, Apple]**. The LAN path is never offered. Safari has
  no prompt to avoid, so this costs speed for nothing.
- **False "away": address families differ.** A device that reached the API over IPv4 while the core
  reported only IPv6 never matches.
- **The fallback for the LAN is weaker than it looks.** D291 §6 prefers WebRTC's host candidates for
  the LAN. Chrome hides host addresses behind random `.local` names, which "the remote peer resolves
  … on the local network" **[V, BlogGeek]**. A core in a container on a bridge network cannot answer
  multicast, so that path fails there.
- **The remembered win is circular.** The LAN can only win on a network where it was tried.

**What the browsers let us ask.** The Local Network Access spec makes the permission a "powerful
feature" (`local-network`; Chromium keeps `local-network-access` as an alias) **[V, LNA spec]**, so
`navigator.permissions.query` reports `granted`, `denied` or `prompt` without prompting **[I: the
query's support is from secondary write-ups, verify]**. Firefox prompts since 153 (154 for
WebSockets) **[V]**. Safari has no such prompt **[I]**.

**Fix.**
- **Ask the browser first.** Granted: race the LAN name always. Denied: never, and say so where it
  matters (U2-7). Prompt: use the guess, with D291's one-line explanation before it.
- **In browsers with no prompt (Safari), race the LAN name always.** Browser-specific policy is
  allowed here.
- **Compare IPv4 only when the core's IPv4 is its own**: mapped and probed, not shared.
- **Treat a /56 match as "maybe"**: offer the LAN only after a /64 match or a remembered win.

### U2-9 — The paid tier's location warning fires on the common, harmless case (medium)

**Journeys:** a TV activated at home, the phone on mobile data; a traveller in a hotel.
**Sections:** protocol §3.1 (*the person entering it sees where the device asking is*); D290 §5;
D291 §6 item 20 (U9: QR, default name, picking the check code from three).

**What the design says.** The person entering a code sees the asking device's approximate place and
network, and "This device is in another country" when it is not on the same network.

**What the person experiences.** [I] At home, many phones are on mobile data with Wi-Fi off. Then
the TV is "on another network", and the person is warned in the normal case. Mobile carriers'
addresses also place phones in the wrong city, or in the home country while roaming. With the check
code, its warning and a location, the person has three things to judge, and the warning that fires
wrongly teaches them to skip all three. YouTube instead tells people to put the phone on the same
Wi-Fi as the TV **[V, YouTube help]**. Microsoft shows the place as context, not as a verdict
**[V, Microsoft]**.

**Fix.** One check: picking the code from three (D291's U9). Show the place as a neutral line ("The
TV asking is near Berlin, on Telekom"). Warn only when the countries differ and the phone is not on
mobile data. Never say "same network" as if it proved anything.

### U2-10 — Step-up and other confirmations the security work adds (medium, preventive)

**Sections:** D291 §6 (protocol §7 item 4: step-up before re-key, removal and minting a locator);
spec §2; D278; D290 §16.

**What will happen.** [I] The dashboard signs in by e-mailed link. A "fresh authentication" there is
another e-mail loop, inside a recovery that already has two. The same will be asked of the core
(D291 §5's password on a device) and of claiming (U2-5).

**What comparable products do.** GitHub keeps two hours of "sudo mode" after one confirmation, and
offers a passkey, security key, mobile app or code before the password **[V, GitHub]**.

**How to keep it light.**
- **Step-up by passkey when one exists; otherwise a six-digit code sent by e-mail, typed in the same
  tab**, never a link that opens another tab.
- **One step-up covers a window** (GitHub's two hours), not each action.
- **Confirm only what cannot be undone** (D278's bright red): removing a core, a recovery re-key.
  Leaving, re-keying by rotation and allowing a claim are one click each.
- **Never stack checks on one act.** A claim has one approval; a device code has one check; a
  recovery has the wait or the recovery code, not both plus a step-up plus mail.

### U2-11 — A Watch Together guest on another browser has no way back but a new link (low)

**Journeys:** a guest who closed the page. **Sections:** D267 §1, §4; D290 §17; protocol §3.1;
`together-viewer.tsx`, `locales/en-together.ts` **[V]**.

**What is decided and built.** The membership is kept in the browser's local storage. The same link in
the same browser comes back as the same member. A link opened in another browser is refused.

**What the person experiences.** In the same browser it works: a real improvement. But a link tapped
first in WhatsApp's or Instagram's in-app browser, and later in Safari, is refused with "This link does
not work: it was used already, withdrawn, or the session is over" (`together.badLink`) **[V]**. Three
causes, one message, and the host must remove and re-invite them (D267 §4). The click that spends
`/locate` also adds a step to every return, though a Watch Together locate is reusable **[I]**.

**Fix.** Say which cause it is ("This link was used in another browser. Open it there, or ask the
host to let you in here"). Give the host a one-tap *Let them in here*, which moves the membership. Fold
the locate into the guest's existing *Join* or *Back to the film* button.

### U2-12 — Two cores with one key: the advice is wrong, and a restart may look like one (low)

**Sections:** protocol §2.3 (`instanceId`), §2.8; D290 §18; the plan's question 4.

**What the design says.** Both System pages say "another server is running with this identity"; the
admin "chooses which to keep and re-keys the other".

**What the person experiences.** [I] Re-keying binds a new key to the **same** `coreId`, so following
the advice moves the household to the test copy. What the owner wants is "this is a copy: disconnect
it". And until the plan's question 4 is answered, a core that crashes and restarts comes back with a
new `instanceId` while its old token lives: an ordinary restart could show the alarming message.

**Fix.** Offer *This is a copy: turn off remote access here*, which forgets `discovery.json` on that
machine. End the old instance when its signalling connection closes, so a restart is never "another
server".

### U2-13 — "Watch from outside the house" is a switch that does nothing (low)

**Sections:** `internal/core/auth/roles.go` (`remote_access` in the closed set); `locales/en-people.ts`
`roles.capability.remoteAccess` **[V]**.

**What is built.** The role editor offers *Watch from outside the house*. Nothing in the core reads
the capability **[V: `remote_access` appears only in the list]**. An admin who unticks it believes
something is restricted. Through the discovery, the core would also struggle to tell "outside": at
home a device often arrives by its global IPv6 address or by WebRTC, as from outside **[I]**.

**Fix.** Hide the switch until it is built, and decide then what "outside" means (the core's own
LAN prefixes, and the path kind the client reports).

---

## Errors in our documents or the first round

1. **The plan says "The identity key is not in any backup: `internal/core/backup` names
   `secret.key` alone".** The opposite holds: since only `secret.key` is left out, every scheduled
   backup holds `identity.key`, the TLS key, the ACME account key and `discovery.json` **[V]**. The
   protocol's §2.1 states D289 §2's rule without marking it unbuilt.
2. **Spec §2: "at most 24 hours, NIST's cap for a code sent by e-mail".** NIST SP 800-63B-4's 24 hours
   is for *recovery codes* sent to an e-mail address. For authentication it says "Email SHALL NOT be
   used for out-of-band authentication" **[V, NIST]**. The 24 hours is a fine choice; it is not
   NIST's rule for sign-in links.
3. **Protocol §7 item 4** puts a step-up before "minting a locator for the customer's browser". The
   spec removed that endpoint (§8: the customer uses their own link).
4. **Protocol §3.1's table:** "The customer's own browser (paid): the core's System page, from which
   they claimed it". That page is the core's origin; `app.` in the same browser has no locator.
5. **Protocol §1, §3.1 and the spec: "Safari erases a site's storage after seven days unvisited".**
   WebKit's rule is "after seven days of Safari use without user interaction on the site" **[V]**:
   days of use, and interaction, not a visit. And home-screen apps escape it by having separate
   storage, which none of our documents say (U2-2).
6. **Protocol §3.1: "Where the browser allows, the page asks for persistent storage."** Safari grants
   it by heuristics "like whether the website is opened as a Home Screen Web App" **[V]**, so it does
   not protect a Safari tab.
7. **The first round's U2** recommended *Add to Home Screen* without the storage isolation, and
   "Open in Safari" without saying it must come before the link is spent. D290 §10 and D291 took both.
8. **Protocol §3.6's message names the admin and the server**, which the discovery cannot know (it
   holds no household data, spec §1.2) on a device's first visit.
9. **Protocol §2.3: "re-keys the other"** moves the `coreId` to the other machine (U2-12).
10. **`deploy/release/README.md` step 6 and `deploy/staging/README.md` step 5** say to make the first
    admin in the browser; the web client has no such form **[V]**.
11. **Spec §11.3 still lists as open** the DNS provider and the probe's machine, which D291 §1–§2
    decided (§11.3b says so; the list was not updated).
12. **Still in the strings (first round U13):** `activate.explain` reads "a television say", and
    `system.discovery.claim` says "sign in at the discovery" **[V]**. K12 owns them.

## What is good

- **Claiming from the core** removes the code a stranger could ask the owner to read out. It is the
  right direction; it needs one confirmation, not two (U2-5).
- **"Locating grants nothing"** (D290 §17) is the right principle. Applied to registration links too, it
  fixes U2-1.
- **The reusable Watch Together link** answers the owner's own complaint: a guest who closed the page
  in the same browser is back with one tap **[V, `together-viewer.tsx` keeps the membership with its
  link]**.
- **A person's own link** is the right answer to "no account, no list of cores". It needs to be the
  link people already have, and to survive the home screen.
- **The sign-in link is spent by a click and tied to the asking browser**, which defeats mail
  scanners without breaking the phone-and-laptop case.
- **Device codes are entered on the trusted device** (D288), as YouTube and Apple's guidance do, and
  the core's caps refuse rather than push out a waiting TV (D288 §6 as amended, built).
- **K8d detects the restore-without-key state** and hides *Connect*, so the first round's worst trap
  is closed, and `discovery.json` is in today's backups, so the detection will work **[V]**.
- **The dead-end message is written per audience and counted** (D290 §12). It needs the cause, not
  new words.
- **The LAN only when at home** is the right instinct; asking the browser makes it exact (U2-8).

## Sources

Cached under `/yoloai/cache/discovery-audit2/` unless marked "first round" (`/yoloai/cache/discovery-audit/`).

- WebKit Bugzilla 181849, *"Add to homescreen" apps don't share storage with Safari* (comment 3, 2022:
  "by design"): <https://bugs.webkit.org/show_bug.cgi?id=181849> (`webkit-bug-181849.txt`)
- WebKit, *Full Third-Party Cookie Blocking and More* (seven days of Safari use; home-screen apps'
  own counter): <https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/>
  (`webkit-10218.txt`)
- WebKit, *Updates to Storage Policy* (persist() granted by heuristics such as a Home Screen Web
  App): <https://webkit.org/blog/14403/updates-to-storage-policy/> (`webkit-14403.txt`)
- Don't Panic Labs, *Understanding window.open() Behavior on iOS Safari* (2025):
  <https://dontpaniclabs.com/blog/post/2025/07/29/understanding-window-open-behavior-on-ios-safari/>
  (`dontpanic-ios-window-open.txt`); Ryan Thomson, *You Shouldn't Call window.open()
  Asynchronously*: <https://www.ryanthomson.net/articles/you-shouldnt-call-window-open-asynchronously/>
  (`ryanthomson-window-open.txt`)
- WICG, *Local Network Access* (draft, 7 August 2026; the `local-network` and `loopback-network`
  powerful features): <https://wicg.github.io/local-network-access/> (`lna-spec.txt`); Chrome for
  Developers, *New permission prompt for Local Network Access*:
  <https://developer.chrome.com/blog/local-network-access> (`chrome-lna-blog-live.txt`); Paul Serban,
  *Using navigator.permissions.query for Local Network Access in Chrome* (secondary, not cached):
  <https://www.paulserban.eu/blog/post/using-navigatorpermissionsquery-for-local-network-access-in-chrome/>
- Firefox 153 and 154 release notes (Local Network Access on by default; WebSockets in 154):
  <https://www.firefox.com/en-US/firefox/153.0/releasenotes/> (`ff153rn.txt`, `ff154.txt`)
- BlogGeek.me, *mDNS* (WebRTC host candidates as `.local` names):
  <https://bloggeek.me/webrtcglossary/mdns/> (`bloggeek-mdns.txt`)
- Apple, *About iCloud Private Relay*: <https://support.apple.com/en-us/102602>
  (`apple-private-relay-102602.txt`)
- Apple, *What to expect during account recovery*: <https://support.apple.com/en-us/118574>
  (`apple-recovery-118574.txt`); *About recovery keys*: <https://support.apple.com/en-us/109345>
  (`apple-recovery-key-109345.txt`)
- GitHub Docs, *Sudo mode* (two hours; passkey and other methods):
  <https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/sudo-mode>
  (`github-sudo-mode.txt`)
- YouTube Help, *Sign in to the YouTube app on your smart TV or game console* (QR or yt.be/activate;
  same Wi-Fi): <https://support.google.com/youtube/answer/3015415> (`youtube-tv-signin-3015415.txt`)
- Microsoft Learn, *Use additional context in Authenticator notifications* (application name and
  IP-based location): <https://learn.microsoft.com/en-us/entra/identity/authentication/how-to-mfa-additional-context>
  (`ms-mfa-additional-context.txt`)
- NIST SP 800-63B-4 (e-mail not for out-of-band authentication; 24 hours for recovery codes sent by
  e-mail): <https://pages.nist.gov/800-63-4/sp800-63b.html> (`nist-63b4.txt`)
- Plex, claiming a server, and `PLEX_CLAIM` (first round): `ux-plex-claim-docs.txt`, `ux-pms-docker.txt`
- Nabu Casa remote URLs (search summary only; the support pages refused the fetch):
  <https://support.nabucasa.com/hc/en-us/articles/26474279202973-Enabling-remote-access-to-Home-Assistant>
- In this repository: [discovery-protocol.md](../discovery-protocol.md);
  [discovery-core.md](../plans/discovery-core.md); [webrtc-transport.md](../plans/webrtc-transport.md);
  decisions D218, D221, D243, D246, D267, D273, D278, D280, D288, D289, D290, D291 in
  [working-notes.md](../../decisions/working-notes.md); `internal/core/backup/backup.go`,
  `internal/core/instancedir/archive.go`, `internal/core/identity/identity.go`,
  `internal/core/auth/pairing.go`, `internal/core/auth/roles.go`; `web/apps/web/src/` (`sign-in.tsx`,
  `code-sign-in.tsx`, `device-activation.tsx`, `register.tsx`, `account.tsx`, `person.tsx`,
  `together-viewer.tsx`, `locales/en.ts`, `locales/en-people.ts`, `locales/en-together.ts`);
  `deploy/release/README.md`, `deploy/staging/README.md`.
