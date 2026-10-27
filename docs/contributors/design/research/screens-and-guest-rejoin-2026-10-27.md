> **ABOUTME:** Three questions from the owner's answers of 2026-10-27: whether Watch Together and
> Cast should be pages of their own instead of pop-ups, whether the warning sign should lead to a
> page, and how a Watch Together guest comes back in by the same link or QR code. What each implies
> on desktop, phone and TV, what competitors do, what it costs, and the questions left. Read before
> changing `status-tray.tsx`, `warning-sign.tsx`, `watch-together.tsx`, `together-viewer.tsx` or
> `together-host.tsx`'s leaving rule.

# Screens of Their Own, and a Guest Coming Back

- **Date:** 2026-10-27. Read-only on code, at `main` an earlier commit.
- **Asked by** the owner, in their answers to the two reports of the day, kept in
  `docs/contributors/design/plans/navigation-and-widgets-answers-2026-10-27.md` (widgets 2 and 3,
  navigation 3):
  - *"maybe watch together and cast should be their own screens rather than a popup. Research the
    implications. The warning sign should also take you to its own page."*
  - *"I wonder if it's a good idea to allow a transient guest who's followed a link or QR code to
    come back in via that same code? Only the browser that redeemed it would be able to."* Then:
    *"Remember the threat model here. The stakes aren't very high."*
- **Builds on:** [navigation-back-up-2026-10-27.md](navigation-back-up-2026-10-27.md) (called
  **the navigation report**; its rules R1–R7 and scenarios 3.11–3.13) and
  [ui-widgets-audit-2026-10-27.md](ui-widgets-audit-2026-10-27.md) (**the widgets audit**; its §3.3
  and §5 Q2–Q3). The owner agreed the navigation report's recommendation (model (d): Back is
  history, Up never adds history), which revises D367 §4–§5 once it is written.
- **Cache:** `/yoloai/cache/http/screens-and-guest-rejoin/` (`<name>.html` as fetched, `.txt`
  extracted, `.url` beside each).
- **Labels:** **[1P]** the vendor's own documentation or source; **[J]** a journalist or
  reviewer; **[S]** a search engine's summary of pages not read in full; **[I]** my inference;
  **judgement** an opinion.

---

## 1. Summary

### 1.1 Watch Together and Cast as pages

**Watch Together already has a page** (`/watch-together`, the top bar's item), and **the cast
remote already is one** (`/casting`, D364 §8). The pop-up is the tray's Watch Together panel
(`status-tray.tsx:110`), and it exists for one reason: **when the host leaves the Play page, the
film stops for everyone** (`together-host.tsx:117–127`, D246 §5). The owner asked for the pop-up
on 2026-10-11 so as to manage the session "without navigating away from the movie (which stops it
for everyone)" (D280 §2). A page in its place is only as good as what happens to the film when the
host goes to it.

**Recommendation (judgement):**

1. **The film plays on for the group while the host is away from the player,** as D364 §8 already
   has it for a cast: "browsing away leaves the video playing on". The group's clock is the
   server's, not the host's player's (`internal/core/together/group.go`), so the viewers are
   unaffected; the host stops it with a Stop of its own, not by leaving.
2. **The tray's Watch Together icon goes to the Watch Together page,** which gains a **remote of
   what plays** (title, position, play and pause, seek, Stop, "Back to the film"), beside what it
   has now: people, links, voice and the film's volume. It is the "more complete set of controls"
   D364 §11 sends a cast's page to. The pop-up goes.
3. **The host's Play page keeps the session strip it already shows under the film** (talk, the
   film's volume, the session's news: `together-host.tsx:158–166`), so voice is never a page away.
4. **The top bar's Cast goes to the Casting page:** with no session, the list of devices to cast to
   (D364 §1); with one, the remote (§8); with Watch Together also on, End casting and a way to Watch
   Together's page (§11). The tray's cast icon goes to the same page.
5. **No mini player or picture-in-picture now.** Both need the player to outlive the Play page (a
   player held by the app's frame), which is the largest cost here and does nothing on a TV. They
   are a later step if the owner misses the picture while on the page.

**Navigation**, per the navigation report: both pages are reached from anywhere, so they show
**no Up**; Back returns where the person was. The tray's link goes *back* when the page is the
entry directly behind (the navigation report's §3.13 option), so tray visits do not pile up.

**What competitors did:** a lobby page before playback (Plex, Disney+ GroupWatch); the social
panel beside the video, never over it (Prime Video Watch Party, Teleparty); and for casting, a
full-screen controller plus a small persistent one on every other screen (Google Cast's design
checklist, which casting apps follow). Three of the four first-party watch-together features were
withdrawn (Disney+ 2023, Prime Video 2024, Plex's new apps 2025) (§2.5).

### 1.2 The warning sign's page

**Recommendation (judgement):** the sign leads to **one "What's wrong" page per subject** (a
title, a library folder, a role), reached by a pushing link. Its parent is the subject, so its
**Up** goes to the subject's page and **Back** goes where the person came from. It says what is
wrong, what it affects and, for an admin, how to fix it, with a link to where it is fixed (D278 §2's
"shortcuts later" become those links). A non-admin sees the generic words and "tell your
administrator" (D278 §3). The admin's **Needs attention** page (D280 §4) becomes the list of the
same blocks, drawn by the same component. The inline pop-up goes, which settles the widgets audit's
Q3 (it answered neither Escape nor Back). Cost: small; nothing new on the server (§3.4).

### 1.3 A guest coming back

**It is already built** (D267 §1, 2026-10-08). The browser that joined keeps the guest's
membership in its storage (`localStorage`, `orpheum.together.viewer`), and opening the same link
again, or `/together` alone, comes back as the same member without asking the server about the
link (`together-viewer.tsx:79–82`). Any other browser is refused, because the link is spent
(`internal/core/together/links.go:127–128`). The credential is a header, never in the address
(D154 §2). Back leaves the page but not the session.

**Recommendation (judgement), kept proportionate as the owner asks:** keep the mechanism as it
is, and close three small gaps in what the guest is told:

1. The guest's page says how to come back: "Closed this page? Open your link again on this
   browser."
2. **Leave** says it is final (the place ends, and only the host's new link brings them back); Back
   and closing the tab are not.
3. A used link opened in another browser, or in a chat app's in-app browser, says so in words a
   guest can act on: "This link was already used, on another browser. Open it there, or ask
   ‹host› for a new one." The host's "new link for this place" (D267 §4, as amended) already moves
   them over.

No device binding, no fingerprinting, no expiry of its own: the membership already ends with the
session, the host's removal, or six idle hours (D346).

---

## 2. Watch Together and Cast as pages of their own

### 2.1 What is there today

| What | Where | What it does |
| --- | --- | --- |
| Watch Together's page | `/watch-together`, the top bar's "Watch Together" (`app-shell.tsx:103`) | Start a session, or run the one this tab hosts: links, people, voice, the film's volume (`WatchTogetherControls`) |
| The tray's Watch Together icon | `status-tray.tsx:110`, shown only while this tab hosts | Opens the **same** controls in a modal drop-down over the current screen |
| The host's Play page | `together-host.tsx:139–167` | The film, and under it the session's news, talk and the film's volume |
| Leaving the Play page | `commandOnLeavingFor`, `together-host.tsx:125` | Sends **stop** for everyone, unless going to another film's player (Up next, D339 §3) |
| The guest's page | `/together`, outside the app's frame (`app.tsx:84`) | The film, news, talk, volume and Leave: already a page of its own |
| The cast remote | `/casting` (`cast-remote.tsx`) | A page with no player: what the receiver plays, its commands, Stop casting |
| The host's voice | `HostVoiceProvider`, `app.tsx:138` | Held by the app, not by a page: it carries on across every page |

So the host already has a page, and going to it from the film **stops the film for everyone**.
That stop is the only thing the pop-up avoids.

### 2.2 The options

| | Film when the host goes to the page | Host sees the film while there | Cost |
| --- | --- | --- | --- |
| **A. Keep the pop-up** (today) | plays | yes, behind the panel | none |
| **B. Page, leaving still stops** | **stops for everyone** | — | none, and unusable (D280 §2) |
| **C. Page; the film plays on for the group** (recommended) | plays | no; "Back to the film" returns at the group's position | medium (§2.8) |
| **D. Page with a mini player** (C plus a persistent player) | plays | yes, small | large |
| **E. The page is the Play page** (the session panel inside the player screen) | plays | yes | medium on a computer, high on a TV (the overlay) |

**Why C (judgement):**

- **It removes the cause, not the symptom.** The pop-up works around "leaving stops the film". With
  C, leaving does not, so the host can also browse for the next film while the group watches, as
  D364 §8 lets a caster do.
- **It is what the server already does.** The group's position runs on the server's clock (D268
  §3), and a device with no player already commands a group: a cast's remote (D88, D337). Whether
  the hub accepts the host's commands from a device that is not playing is a check for the build,
  not a known gap [I].
- **One set of controls (D364 §11).** The owner assumed Watch Together's page "has the more
  complete set of controls". Today it has none for playback; C gives it them, so the cast page's
  "go here for playback controls" lands somewhere that has them.
- **Returning is already right.** The host's player puts its film on "unless the session plays it
  already" (`useLeadingPlayer`, `together-host.tsx:84`), and then follows the group, so coming back
  joins where the group is rather than restarting it.

**What C gives up:** the host does not see the picture while on the page. That is the case for D
(a mini player). D costs moving the player out of the Play page into the app's frame. Its own
video element, hls.js instance and the leading hooks would then outlive routes, which touches the
Play page, Up next and the TV overlay. It does nothing on a TV, where a page is full screen anyway.
A first step that keeps the cost small is C. D can follow if the owner misses the picture.

**Why not E alone:** the Play page already holds a strip of the session (§2.1). Making it hold
everything (people, links, invite dialogs) on a TV means putting them in the player's overlay,
whose focus order the controller report built for play, seek and tracks
([controller-input-2026-10-25.md](controller-input-2026-10-25.md)). On a computer a side panel
beside the video is what Prime Video and Teleparty did (§2.5), and it can come later as a layout of
the same page's parts.

**Open for the owner:** what makes the film stop, once leaving does not.

- (i) Only Stop, on the page or the Play page.
- (ii) Stop, or the host playing another film. That replaces what plays for everyone, as D246 §5
  has it, and asks first while one plays.
- (iii) As (ii), plus the host's tab closing for longer than a while.

(ii) is the recommendation; (iii) is the idle end D346 §3 already covers.

### 2.3 Navigation

Under the navigation report's model (d):

- **No Up** on either page: each is reached from anywhere (R4; Android's 2012 guide on screens with
  several entry points).
- **Back** is history: from the page to where the host was, the film included; from the film, Back
  goes to where the film was reached from, as for any film.
- **The tray icon:** a pushing link, except when the page is the entry directly behind, when it goes
  back (§3.13 there). Otherwise, film → tray → page → "Back to the film" → tray → page piles up
  entries.
- **"Back to the film"** on the page is a link to the Play page of what plays. When that Play page is
  the entry behind, it goes back to it, by the same rule.
- **The signed-in member** (not the host, D267 §2) has no tray icon yet: D280's status says "Not
  yet: a signed-in viewer's sign". Answer 3 of the navigation answers ("Leaving Watch Together's
  page by Back keeps the session, if signed in, the tray leading back") needs it. That is part of
  this work.

### 2.4 Devices

| | Pop-up (today) | Page (C) |
| --- | --- | --- |
| **TV (Silk, TV Bro), by remote** | A modal `<dialog>` over a full-screen film: focus is trapped in it, the remote's Back must close it (R1) and on TV Bro never reaches it, so it needs its own Close. The drop-down is sized for a mouse. | An ordinary page for spatial focus. The remote's Back is history (Silk); TV Bro needs the on-screen Back the navigation report keeps on every TV. Leaving full screen is the browser's. |
| **Phone** | The drop-down covers most of the screen anyway, and scrolls inside a dialog inside a page | A page scrolls as pages do. Spotify's device list and the Cast checklist's expanded controller are full screens on a phone (§2.5) |
| **Desktop** | The film stays visible behind the panel: the one real advantage | The film is out of sight until "Back to the film" (or a mini player, D) |
| **Installed app (iPhone)** | as the phone | as the phone; the edge swipe is Back |

The guest's page is unchanged: it is a page already, with no frame and nowhere else to go (§4).

### 2.5 Competitors

| Product | Where the session's controls live | Source |
| --- | --- | --- |
| **Plex Watch Together** | Started from a title's More menu. Friends (accounts only) are invited, a "Watch Together" **row on Home** lists the sessions, and each joins a **lobby** screen until everyone is ready. "Ending support … for most platforms" in the new apps, kept on the web app (2025-02-25). | [1P] support.plex.tv/articles/watch-together |
| **Disney+ GroupWatch** | An icon on the title's page opens a **lobby** with the party's avatars, "+" to copy an invite link, and **Start Stream**. In the player, reactions slide out from the side. Accounts required. Removed 2023-09-18 "without an official announcement". | [J] How-To Geek (how-to; removal) |
| **Prime Video Watch Party** | A party-hat icon on the title's page creates the link. A **chat panel on the right, beside the video** shows who joined, paused or skipped. Removed 2024-04-02. | [J] Pocket-lint; How-To Geek; [S] Tech Times |
| **Teleparty** | A browser extension: the party link, and a **chat sidebar beside the video**. A guest rejoins "using the original link". | [1P] teleparty.com/support; [J] MakeUseOf |
| **Google Cast (Netflix, YouTube and other senders)** | The design checklist requires an **expanded controller** (a full-screen remote) and a **mini controller**: "a small, persistent control … when the user navigates away from the current content page or expanded controller … Available in all screens of the app (except the expanded controller) … Tapping on the content area opens the expanded controller." | [1P] developers.google.com/cast/docs/design_checklist/sender |
| **YouTube (web)** | The **Miniplayer**: "the video will continue to play in the Miniplayer as you navigate around the site". On a TV, "your phone automatically becomes a companion to control playback". | [1P] YouTube Help 9162927, 3230451 |
| **Spotify Connect** | "Tap the device icon when listening": a device list from the **Now Playing bar**, which stays at the bottom of every screen | [1P] support.spotify.com, Spotify Connect |
| **Netflix (mobile casting)** | Unverified: Netflix's help pages describe casting but not its sender screens. Its mobile app is believed to follow the Cast checklist's two controllers. | [I] |

**What they agree on [I]:**

- **The session's full controls are a screen, never a pop-up:** a lobby, the expanded controller,
  a device list.
- **What stays in view while browsing is a small bar** (the mini controller, Now Playing, the
  Miniplayer), not the full controls. Orpheum's tray icon is that bar's smallest form. C makes it
  lead to the full screen, as the mini controller does.
- **The social panel sits beside the video,** not over it, on a computer (Prime Video, Teleparty).

### 2.6 What a person watching sees

- **The host** on the page sees no picture under C. Their voice carries on: it is held by the app
  (`HostVoiceProvider`), not by the page. The page's remote shows the position moving, as the cast
  remote does from the receiver's reports.
- **A viewer** sees nothing change: the group plays on. Today they see the film stop when the host
  leaves the player, and its screen go black (D246 §5).
- **Picture-in-picture:** the video-element API needs the element to stay in the page. Leaving the
  Play page unmounts it and ends the window, so it needs D's persistent player too. The
  Document Picture-in-Picture API (a whole page always on top) is Chromium's alone, and neither
  exists on the TV browsers [1P MDN; I].

### 2.7 Deep links

- `/watch-together` and `/casting` are already addresses. Each needs a signed-in person, and each
  finds the session from the server (D267 §2, D364 §6), so a reload or a bookmark lands right.
- Neither address carries a credential (D154 §2). The guest's `/together#secret` is the only one
  with a secret, after `#`, removed once used (D243 §3).

### 2.8 Cost (rough, for an agent; judgement)

| Piece | Size |
| --- | --- |
| C: leaving the Play page no longer stops (`commandOnLeavingFor`), with the owner's choice of what does (§2.2) | small; its test inverts |
| C: Watch Together's page gains the remote. Rule 11: one remote for a cast and for Watch Together, factored out of `cast-remote.tsx` (D88: a cast is Watch Together with a group of one). | medium |
| The tray icon becomes a link. Its drop-down and the lazy loading of the controls go (`status-tray.tsx:84–150`). | small |
| The signed-in member's tray icon (§2.3) | small–medium: the tray learns of memberships, not only of hosting |
| Casting page: the device list when no session (D364 §1, being built anyway) | small beyond D364 |
| Device checks: TV (Silk and TV Bro focus, Back), iPhone installed, Android | one VQ item |
| D: a persistent player and a mini player | large; not recommended now |

---

## 3. The warning sign's page

### 3.1 What is there today

| Kind | Where the sign is | Admin is told | Anyone else |
| --- | --- | --- | --- |
| **Cannot play** (`UnplayableSign`) | a title's and series' page notes, beside words that already say why (`status-cues.tsx:83`); over a poster only as an icon, not a control (the owner, 2026-10-25) | why (its machine away, folder or disk missing) and "go to Libraries" | not available; tell the admin |
| **Folder's removals wait** (`RootWarning`, withheld) | beside the folder, Libraries and a library's page | the reason, and Approve removals (D208, D277) | — (admin pages) |
| **Folder offline** (unreadable) | same | go to System | — |
| **Disk missing** | same | the paths; go to System | — |
| **Role reaches no library** | the roles section | that it reaches none | — |
| **Uncertain match** (D301 §4) | a note with its own icon, not a `WarningSign`; the words in full, no pop-up | an admin corrects it from the title's page | that it may be wrong |
| **Still reading** (D304 §4) | a note; "not a fault" | — | — |
| **The admin's tray sign** | the banner | already leads to a page: **Needs attention**, a placeholder of counts (D280 §4) | — |

The sign's pop-up is inline, absolutely placed, and answers neither Escape nor Back (the widgets
audit §3.3). On a title's page it mostly repeats the words beside it.

### 3.2 What the page shows

One route, its subject in the address: a title, a library folder, or a role. A heading names the
subject, then one block for each problem it has, in the status cues' order (cannot play first,
the owner's 2026-10-15 rule). Each block says:

| Kind | Admin | Anyone else |
| --- | --- | --- |
| Cannot play | why, each file affected and the folder or disk it is on, and a link to that folder's library page (or System for a machine away) | "This can't be played right now. Tell your administrator." |
| Folder's removals wait | what the last scan did not find (how many, a few names), why it waited, **Approve removals** (asking first, as the owner's answer on widget 4 has it) and a link to the library | — |
| Folder offline, disk missing | since when, the path, a link to System | — |
| Role reaches no library | the role, a link to its libraries | — |
| Uncertain match | the guess and its runner-up candidates, with a link to correct it on the title's page. D365 §5's reasons (year, runtime, votes, fit) are in the log today; showing them is the owner's call (Q5). | "This may be identified wrongly. Tell your administrator." |

**Needs attention** becomes the list of the same blocks across the household, each headed by its
subject's link: one component, two pages. That is rule 11's "one way", and it fills D280 §4's
placeholder.

### 3.3 Returning, TV and phone

- **Up** goes to the subject's page (a title, a library, the roles). The problem page is that
  subject's child, so Up replaces or goes back per R4.
- **Back** goes where the person came from, including from Needs attention.
- **TV:** a page is ordinary spatial focus. The sign itself is a focusable control on a title's
  page, and today's pop-up inside the focus order is gone. Posters keep their icon-only cue (no
  control, the owner's 2026-10-25 rule); the title's page leads on.
- **Phone:** no absolutely placed bubble that can run off a narrow screen.
- **The cost of a page:** one more press and a change of screen, for a non-admin, to read one
  sentence. Where the words are already beside the sign (a title's notes), they stay there, and the
  sign adds the detail and the fix (judgement).

### 3.4 Cost

Small to medium. The route, the page and the shared block: the subject's data is already served
(a title's `unavailable` and `isGuessed`, a root's scan outcome and missing paths, the
`/needs-attention` counts). Needs attention's list may want the items, not only the counts:
`GET /needs-attention` gains them. `WarningSign` becomes a link, its `forAdmin` and `forViewer`
props move into the page's blocks, and its tests change with it. A VQ item for TV focus.

---

## 4. A Watch Together guest coming back

### 4.1 What happens today (as built)

| Moment | What happens | Where |
| --- | --- | --- |
| **The link or QR code** | `…/together#<secret>`, or on a discovery's origin the core's locator around it. The secret is after `#`, so it reaches no server log (D243 §3). The QR code encodes the same address (`together-people.tsx:152`). | `secretFromHash`, `accounts.ts:38` |
| **Join** | `POST /together/join` with the secret and a name: the server admits the person the link was made for, **once**, while the link waits (an hour, D346 §1), and answers a **membership credential**. | `links.go:119` |
| **Kept** | `{sessionId, credential, link, locator}` in `localStorage` under `orpheum.together.viewer`: the browser's storage, not the tab's. The secret leaves the address and the history. | `together-viewer.tsx:121–124` |
| **Every request after** | `Authorization: Bearer <credential>`, never in the address (D154 §2) | `membershipHeaders` |
| **Reload, or the page closed and opened again** | the kept membership is used; no join | `together-viewer.tsx:79–82` |
| **The same link opened again in this browser** | recognised as the one joined by (`kept.link === secret`), the address cleaned, the same member back | same |
| **`/together` with no link** | the kept membership, if any; else "no link" | same |
| **Back** | leaves the page. The membership stays, in the browser and on the server, so the link brings them back. | — |
| **Leave** | `POST /together/leave`: the place ends, the browser forgets it, and the spent link admits no one | `together-viewer.tsx:302–333` |
| **Another browser, the same link** | refused: the link is used (`!used.waits(now)`) | `links.go:127–128` |
| **The host removes them** | their credential admits nothing, and their devices are told `ejected` | D119, D267 §4 |
| **The session ends, or six idle hours pass** | every credential ends; the page says why (`hostEnded`, `idle`) and forgets the membership | D346 §2–§3; `together-viewer.tsx:172–186` |
| **Sent away because the host's upload is full** | the membership is kept, with "Join again" | D286 §5 |
| **Moving to another device** | the host makes a new link for the place. Whoever opens it takes the place over, and the old devices are cut off and told why. | D267 §4 as amended, D318 |

So the owner's proposal, "only the browser that redeemed it would be able to", is D267 §1, built
since 2026-10-08. What the guest lacks is being **told**.

### 4.2 The gaps

1. **Nothing says they can come back.** A guest who presses Back, or closes the tab, believes they
   left, and asks the host for a new link. The host's new link then cuts off the browser that still
   held the place (D267 §4).
2. **Leave and Back look the same to a guest** but are not: Leave ends the place for good.
3. **"The same browser" is narrower than a guest thinks.** A link tapped in a chat app often opens
   in that app's own browser, whose storage is not the phone's browser's:
   - Since iOS 11, an in-app Safari view "no longer shares cookies with the standalone Safari
     browser", and its local storage and other website data are kept apart too [J Okta; S].
   - A home-screen web app's storage "is kept isolated from Safari" [1P WebKit].

   Opened first in the chat app's view and later in Safari, the link is "another browser" and
   refused. Android's Custom Tabs share Chrome's storage [I, unverified].
4. **The refusal's words** (D346 §5: "expired, was used or withdrawn, or the session is over") do
   not tell this guest the one thing that would help: their place is still held on the other
   browser.

### 4.3 The design (recommended; judgement)

Keep the mechanism; change the words, and one confirmation:

- **On the guest's page,** under Leave: "To come back, open your link again in this browser."
- **Leave asks first:** "Leave the session? You'll need a new link from ‹host› to come back." It is
  irreversible for the guest, so its button is the bright red (D278 §4).
- **A used link in a browser that did not use it:** "This link has already been used, maybe in
  another app's browser. Open it there, or ask ‹host› for a new one." The server says only "used",
  never by whom.
- **The host's person dialog** (D281 §5) already offers "a new link" for a place. Its words say it
  moves the guest to wherever the new link is opened.
- **Nothing else changes:**
  - No expiry of the membership beyond the session's: it already ends at the host's end, the host's
    removal or six idle hours (D346).
  - No link expiry on a rejoin: a rejoin never presents the link to the server, so the link's hour
    (D346 §1) does not apply, and it should not.
  - Safari's seven-day deletion of script-written storage [1P WebKit] is far longer than any
    session.

### 4.4 Security, proportionately

The stakes: a guest sees only what the host plays, has no library, no browsing and no account
(D246 §3), for at most the session's life. The owner: "The stakes aren't very high."

| Case | What it gives | Proportionate answer |
| --- | --- | --- |
| **Stolen storage** (malware, someone at the unlocked device, an XSS) | that guest's place until the session ends. The same XSS would read a signed-in person's token, a bigger prize; OWASP's advice against secrets in `localStorage` is about exactly this [1P]. | none beyond what exists: the host sees the roster and can remove anyone. The credential is opaque and revocable (D154 §1). |
| **A shared device or browser** | the next person in that browser, during the session, can open `/together` and be that guest | none; the session is short. Leave clears it, for a guest who cares. |
| **The link forwarded before use** | whoever opens it first is admitted, under the guest's note | inherent to a link (D119's "a join link admits"). The host sees a name they don't know and removes it. The hour limits it (D346 §1). |
| **The link forwarded after use** | nothing: refused | as built |
| **A private window** | the place is lost when the window closes | the host's new link; nothing to fix |

**What not to build:** binding to a device fingerprint, or a second factor for guests, or a
"knock" (ask the host to let a refused browser in, as Meet guests do). The new link per place
already does the knock's job with one press by the host (judgement).

### 4.5 How others handle a guest coming back

| Product | A guest comes back by | Source |
| --- | --- | --- |
| **Teleparty** | the original link, which stays valid for the party ("leaving and rejoining the party using the original link" is its own fix for bad sync) | [J] MakeUseOf; [S] |
| **Discord** | an invite link (with a lifetime and a number of uses). A "temporary membership" invite kicks the member when they disconnect, unless given a role: the opposite of what the owner wants. | [1P] Discord API, invite object; [J] MakeUseOf |
| **Google Meet** | the meeting link again; a guest without an account "knocks" and a host admits them. A host can turn knocking off, and anonymous askers are then refused. | [1P] Workspace Learning Center; Workspace Updates 2024-01 |
| **Zoom** | the meeting link again. A participant the host removed cannot rejoin unless the account's "Allow removed participants to rejoin" is on. | [1P via a university guide] U-M |
| **Plex, Disney+** | no anonymous guests: friends' accounts only, so coming back is signing in | [1P] Plex; [J] How-To Geek |

**What this suggests [I]:** links that stay good for the session (Teleparty, Meet, Zoom) are the
norm, and a removal that sticks is the norm too. Orpheum binds the link to the first browser
instead. That is stricter than Teleparty's, and right for links sent by chat to people the host
named.

---

## 5. Questions for the owner

1. **Leaving the film no longer stops it for everyone** (§2.2, option C): agreed? And what stops it:
   (i) only Stop; (ii) Stop, or the host playing another film after asking (recommended); or
   (iii) as (ii), plus the host's tab gone for a while?
2. **No mini player or picture-in-picture for now** (§2.2 D), the page showing the position
   instead: agreed, or is seeing the picture while managing the session a must?
3. **The top bar's Cast and the tray's cast icon both go to the Casting page** (devices, or the
   remote) (§1.1 item 4): agreed?
4. **A signed-in member's tray icon** (§2.3), owed since D280, is built with this: agreed?
5. **The warning sign's page** (§3): one page per subject, Needs attention its list. And should the
   uncertain match's page show the candidates and the reasons D365 §5 logs?
6. **The guest's words and Leave's confirmation** (§4.3): agreed, with the mechanism unchanged?

## 6. Not tried

- **No product was run.** Netflix's and YouTube's mobile sender screens, Prime Video's and
  GroupWatch's layouts are from their help pages and reviews, and both of those are gone.
- **The in-app browsers' storage** on the owner's guests' phones (WhatsApp, Signal and Telegram on
  iPhone and Android) was not tested. A VQ item when §4.3 is built.
- **The hub accepting the host's group commands from a device with no player** (§2.2) was read
  as likely from the cast remote's tests, not tested.

## Sources

Cached in `/yoloai/cache/http/screens-and-guest-rejoin/` under the names given.

- Plex, *Watch Together* (`plex-watch-together`): https://support.plex.tv/articles/watch-together/
- How-To Geek, *How to Watch Disney+ Remotely With Friends Using GroupWatch*
  (`howtogeek-groupwatch-howto`):
  https://www.howtogeek.com/693081/how-to-watch-disney-remotely-with-friends-using-groupwatch/
- How-To Geek, *Disney+ Quietly Removed Its GroupWatch Feature* (`howtogeek-groupwatch`):
  https://www.howtogeek.com/disney-quietly-removed-its-groupwatch-feature/
- Pocket-lint, *Amazon Prime Video Watch Party* (`pocketlint-prime-watch-party`):
  https://www.pocket-lint.com/amazon-prime-video-watch-party-how-virtual-watch-parties-with-friends-work/
- How-To Geek, *You Can't Use Watch Party in Prime Video Anymore* (`howtogeek-prime-watch-party`):
  https://www.howtogeek.com/prime-video-watch-party-shutdown/
- Tech Times, Prime Video Watch Party discontinued (search summary only):
  https://www.techtimes.com/articles/306472/20240708/amazon-discontinued-prime-video-watch-party-feature-users-noticed.htm
- Teleparty, *Support* (`teleparty-support`): https://ww1.teleparty.com/support
- MakeUseOf, *How to Join a Netflix Watch Party Using Teleparty* (`makeuseof-teleparty`):
  https://www.makeuseof.com/how-to-join-a-netflix-party/
- Google Cast, *Sender design checklist* (`cast-checklist-sender`):
  https://developers.google.com/cast/docs/design_checklist/sender
- YouTube Help, *Watch videos on the Miniplayer* (`youtube-miniplayer`):
  https://support.google.com/youtube/answer/9162927
- YouTube Help, *Watch YouTube on your smart TV* (`youtube-cast`):
  https://support.google.com/youtube/answer/3230451
- Spotify, *Spotify Connect* (`spotify-connect`):
  https://support.spotify.com/us/article/spotify-connect/
- MDN, *Picture-in-Picture API* (`mdn-pip`) and *Document Picture-in-Picture API* (`mdn-docpip`):
  https://developer.mozilla.org/en-US/docs/Web/API/Picture-in-Picture_API,
  https://developer.mozilla.org/en-US/docs/Web/API/Document_Picture-in-Picture_API
- Okta, *A History of the Mobile SSO Experience in iOS* (`okta-mobile-sso`):
  https://developer.okta.com/blog/2022/01/13/mobile-sso
- WebKit, *Full Third-Party Cookie Blocking and More* (`webkit-7day`):
  https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/
- OWASP, *HTML5 Security Cheat Sheet* (`owasp-html5-storage`):
  https://cheatsheetseries.owasp.org/cheatsheets/HTML5_Security_Cheat_Sheet.html
- Discord, *Invite resource* (`discord-invites-api`):
  https://discord.com/developers/docs/resources/invite
- MakeUseOf, *How to Grant Temporary Access to Your Discord Server* (`makeuseof-discord-temporary`):
  https://www.makeuseof.com/how-to-grant-temporary-access-discord-server/
- Google Workspace, *Tips to control meeting access and participation* (`meet-access-tips`):
  https://support.google.com/a/users/answer/11989526
- Google Workspace Updates, *Join meetings as a guest without a Google account on mobile devices*
  (`meet-guest-mobile`):
  https://workspaceupdates.googleblog.com/2024/01/join-a-meeting-without-a-google-account-on-mobile.html
- University of Michigan, *Zoom: Allow Participants to Rejoin Meeting After Removal*
  (`umich-zoom-rejoin`):
  https://teamdynamix.umich.edu/TDClient/30/Portal/KB/Article/13086/Zoom-Allow-Participants-to-Rejoin-Meeting-After-Removal
