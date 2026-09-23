> **ABOUTME:** How competitors' interfaces work and fail — the big streamers' documented design
> rationale, the third-party clients people pay for or switch to, complaints about the admin,
> player, browsing and accessibility surfaces, and web players and Watch Together UIs.

# Competitor interfaces: what works, what fails, and why

- **Date:** 2026-09-23. Requested by the project owner: *"how are our competitors doing their
  user interfaces? What works and why? What doesn't and why? What do people complain about, and
  how could we avoid falling into the same traps?"*
- **Builds on** [ui-sentiment.md](ui-sentiment.md) (complaint volume, App Store theme coding,
  mods and skins), which this does not repeat, and **corrects its §0** (below).
- **Sources measured:**
  - forums:
    - Plex: the top 120 suggestions by vote, 54,809 votes, plus 49 threads read in full
    - Jellyfin's feature tracker (Fider): 2,000 requests, 58,548 votes
    - Emby: its top 100 feature requests
  - 2,712 App Store reviews across 13 clients, and Google Play ratings for 13
  - reaction-sorted issues across 20 repositories
  - 67 primary documents from the big companies: Netflix, Apple HIG and WWDC, Roku certification,
    Google TV and Android TV guidelines
- **Working notes** (verbatim quotes, URLs; [V] read at source, [S] secondary):
  `/yoloai/cache/research/ui-{bigco,clients,complaints,web-wt}.md`; raw pulls in
  `/yoloai/cache/http/ui2/`.
- **Not reached:** Reddit (login wall), the Kodi forum and Apple Community (bot walls), Netflix
  TechBlog on Medium (403; two posts recovered from archive.org). No peer-reviewed study of
  ten-foot UI was found.

## 0. A correction: the Plex backlash is about twenty times larger than recorded

[ui-sentiment.md](ui-sentiment.md) §0 sampled the past year's top threads and found a largest of
1,124 likes. The four biggest threads about Plex's "new experience" were created just outside
that window, and counted the same way (Discourse topic `like_count`) they carry **about 20,400
likes** [V, 2026-09-23]:

| Likes | Posts | Thread | Created |
| --- | --- | --- | --- |
| **6,482** | 1,531 | *New Experience Public Release Feedback* | 2025-03-31 |
| **6,425** | 757 | *New UI is an awful experience* | 2025-09-16 |
| 4,759 | — | *Roku New Experience Release Update* | 2025 |
| 2,715 | — | *Releasing the new app was a crazy decision* | 2025 |

**The conclusion of §0 stands and is understated.** The largest thing on Plex's forum after the
redesign is not PM4K (5,312) but the redesign itself.

## 1. What works, and the reason it works

### The first screen answers "what was I watching?"

- **Netflix, 2026, on its own homepage** [V]: *"a Continue Watching row near the top has high
  stopping power: users tend to resume a title they have already started rather than explore
  new content… rows with larger artwork tend to reduce further scrolling."* Netflix has to
  trade that against discovery, because discovery is its business. **A personal library has no
  such trade**: nothing on it is for sale.
- **Roku's certification makes it a rule** [V]: selecting a Continue Watching item must launch
  *"directly into playback (apps may not launch into a profile selection screen, content
  details screen, or any other screen…)"*. Scrub thumbnails are required for anything over 15
  minutes, and resume points must survive at least 30 days.
- **Apple's HIG** [V]: *"Avoid asking people if they want to resume playback."* Nothing sits
  between "play" and the picture, player menus are at most one level deep, and the skip
  interval went from 15 s to 10 s, *"making it easier to track how far you've jumped."*
- **Google writes its definitions down** [V]: a film is *started* after 3% or two minutes, and
  *finished* when the end credits start; a series contributes one episode; a finished series
  drops out. **Written, inspectable thresholds** are what [play-state.md](../plans/play-state.md)
  already argues for.

### One-purpose buttons that appear only when relevant

**Skip Intro** [V, Netflix, 2022-03-16] came from observing that members skipped forward by hand
in the first five minutes about 15% of the time. The brief was *"a single purpose solution"*
that *"should appear on screen only when needed and… work with a single click."* It is pressed
**136 million times a day**. Its designer names **trust that it lands in the right place** as a
design goal. A skip button that lands wrong once is used less afterwards, so segment accuracy
([media-segments.md](../plans/media-segments.md)) matters more than the button.

### The clients people pay for or switch to win on correctness and ownership, not looks

| Client | Evidence | What its users name |
| --- | --- | --- |
| **Infuse** | 28,224 App Store ratings at 4.67, against 480 for Jellyfin Mobile and 272 for Swiftfin [V] | *"direct play of everything I could attempt to throw at it"*, *"Better frame pacing than plex app"*, *"I only see my media"* |
| **PM4K** (Plex via Kodi) | 5,312 likes; its feature list opens with "Directplay anything" [V] | correct stream identification (Atmos, DV), reading files over SMB/NFS, sections that can be **moved and hidden** |
| **Wholphin** (Jellyfin, Android TV) | 4.77 on Play, 50K+ installs in about a year [V] | rows that can be **reordered, hidden and pinned**, Continue Watching merged with Next Up, removing a show from Next Up |
| **Plezy** (Plex) | 42% of its reviews compare it with the official app, averaging 4.31★ [V] | speed: *"why is plezy so fast and plex so slow?"* |
| **Streamyfin, Findroid** | exist because of Jellyfin's most-wanted request, offline downloads (1,817 votes, open since 2019) [V] | downloads |

**Four clients ship their own server plugin** (Streamyfin, Moonfin, Wholphin, InfuseSync) [V].
The inference [S] is that at least three exist to store per-person settings or home layout on
the server, because the server offers no place for them.

**Infuse's complaints are about its business model, not its interface**: price is 26% of recent
reviews [V]. **Plexamp** confirms the "focus" lesson with a twist: people who chose it love it;
people moved into it when Plex split music out of the main app average **2.22★** [V].

## 2. What they reversed, and what the reversals say

| Reversal | Evidence |
| --- | --- |
| **Autoplay previews became optional everywhere; nobody removed them** | Netflix per-profile switch (2020); Fire TV "Featured Content" switch; **Roku requires every app to obey one system-wide autoplay switch since 2024-10-01** [V] |
| **"Remove from Continue Watching", added after years of asks** | Netflix on TV 2022-01-27 with undo [V]; Disney+ 2025-03-24, *"You asked, and we listened!"* [V]; Jellyfin's second most-wanted request at **1,722** votes [V] |
| **In-progress and want-to-watch split apart** | Apple merged them in Up Next for eight years and split them into Continue Watching and Watchlist in tvOS 18.1 (2024) [S]; Jellyfin "Watchlist" 1,292 votes [V] |
| **Hidden controls made visible** | Apple removed tvOS's swipe-down subtitle/audio panel in tvOS 15; the controls became visible buttons because that *"improved the discoverability"* [V, WWDC21] |
| **A default that picked for the user, turned off by the press** | tvOS 26's profile picker on wake, on by default; the coverage explained how to disable it [S] |
| **Low-use features quietly dropped** | Netflix "Surprise Me" (Jan 2023) [S]; Disney+ GroupWatch (Sept 2023) [S]; Prime Video AI recaps pulled for factual errors (Dec 2025) [S] |
| **A redesign kept despite backlash** | Netflix's 2025 TV redesign stayed, with a lighter version for older televisions [S] |

**Three lessons.** Anything that plays or moves without being asked becomes a switch in the end,
so ship the switch. Users keep asking to *curate* the automatic lists, because a list the system
fills and the user cannot edit is wrong about them forever. And generated content that can be
wrong (AI recaps) is worse than an honest empty space, which is YouTube's choice with history
off [V].

## 3. What people complain about, by surface

Share of votes, coded by title (the coding is the researcher's):

| Surface | Plex top 120 | Jellyfin, all 58,548 votes |
| --- | --- | --- |
| **Admin and setup** | 20.5% | ~26% |
| **Player** | 12.6% | 8% |
| **Browsing and search** | 4.2% | **23%** |
| Multi-user | 2.7% | 2.7% |
| Accessibility | 0% | 0.02% |

### Admin and setup

1. **Silent transcoding from per-device quality defaults.** Plex's *Default All Clients to Max
   Internet Streaming* has **3,069 likes and 71k views**, the forum's largest admin topic.
   Jellyfin's *How to disable transcoding* has 108k views; Emby's force-direct-play request
   has 165 replies [V].
   - **Cause:** a value that should be measured is exposed as a setting, set per device, and
     invisible to the admin. The reason for a transcode appears only in a log.
2. **Wrong matches, and corrections that don't stick.** *Plex incorrectly identifying TV
   episodes* has 126k views, and its accepted fix is a folk ritual users call the "Plex Dance".
   A Jellyfin refresh overwrites IDs the user chose, and Plex's *Lock Posters* request has 194
   votes, with posters reported to "unlock" themselves [V].
   - **Cause:** no record that *a person decided this*.
3. **Folder naming as a hard requirement.** Jellyfin's docs say files must match
   "character-for-character"; the two naming threads have 42k and 36k views [V].
4. **Errors that give no way forward.** "Conversion failed" (Plex, 138k views), "Fatal Player
   Error" (Jellyfin, 80k), "client isn't compatible" (Jellyfin, 42k + 36k). Among the 69 top
   Jellyfin troubleshooting threads, **39% get "send logs" as an early reply**, and 55% of the
   playback ones do (regex count, approximate) [V].

### The player

- **Track defaults kept at the wrong scope.** Plex's per-series subtitles request has 701 votes:
  *"I have to select 'None' 90 individual times."* Jellyfin's default-language request is filed
  separately on six clients, 133 reactions in total [V].
- **Tracks you cannot tell apart.** Plex's *show track names* has 417 votes: SDH, commentary and
  forced tracks look identical [V].
- **Subtitles out of sync after a resume or seek.** jellyfin-web #2547, +105, open since 2020; web
  #4346 is its most-reacted open issue [V].
- **Seeking treated as a button, not as a remote command.** On Samsung, focus jumps to Pause
  after one press; seeks snap to fixed 30 s marks; Plex's skip-length request has 453 votes [V].
- **"Watched" overloaded.** Removing something from Continue Watching should not mean marking it
  watched; 1,722 votes [V].

### Browsing and search

*Remove pagination / use lazy loading* has **1,176** votes. Search returns at most 24 results in
Jellyfin and 30 in Emby. Filters either never persist or persist with no reset. Posters carry no
titles (+28, +19), and list view has +30 [V].

- **Cause:** page sizes and caps that suit a demo library, and view state with no stated
  lifetime.

### Accessibility: absent from the votes, so absent from the products

Complaints are almost all **unlabelled focus on television**. The Plex Fire TV screen reader
reads *"1… 241… 36"*, which are unwatched-count badges, not titles. Swiftfin on tvOS is
*"completely inaccessible"*. Blind users call Emby and Jellyfin *"basically the same"* [V].

- **Cause:** every one of these projects triages by votes, and a population that cannot easily
  use the product does not vote on it. **Accessibility has to be a release gate; it will never
  win a ranking.**

### Household and settings scope

Emby's *User Settings – Server vs Device*, jellyfin-web #5456 (*"saved for the browser, not for
the user"*) and *Allow global defaults* (258 votes) are the same complaint. Parental
restrictions leak through artwork, actor pages and "Latest" because they are enforced per list
[V].

- **Cause:** there is no chain from server default to person to device override, and nothing
  shows where a value came from.

## 4. The web player and Watch Together: Cut A's two surfaces

### Web player conventions

- **Keyboard** [V]: YouTube and Netflix agree on Space, `f`, `m`, arrows and Esc. jellyfin-web
  copies YouTube's `k`, `j`/`l`, `0`–`9` (jump to a tenth of the film) and `,` `.` (frame step);
  YouTube adds `c` for captions and `Shift+?` for the list.
  - **Traps:** Space re-activates the last focused button (jellyfin-web #4857); shortcuts fail on
    non-Latin layouts (#7397), so bind `KeyboardEvent.code`, not `.key`, and ignore anything held
    with Ctrl, Alt or Cmd.
- **Subtitles** are the web client's largest complaint class [V]. They desync after a seek
  (#4346), vanish in 4K fullscreen when the controls fade (#6728), sit under the controls
  (#1865), and are missing in picture-in-picture (#7217, #7444). **A subtitle layer that moves up
  when the controls appear**, and native WebVTT where possible.
- **Audio and subtitles in one panel**, with the likely languages first and the rest under
  "Other" (Netflix) [V].
- **A network drop is a countdown, not an error page**: *"Retrying in [X] seconds"* (Netflix) [V].
- **Media Session** carries lock-screen and media-key controls, and it has `togglemicrophone`
  and `hangup` actions, so the call can be controlled from the lock screen too [V].

### Web platform traps

- **iPhone has no element fullscreen** (caniuse: *"only iPad, not iPhone"*) [V]. Fullscreen on an
  iPhone hands playback to the system player, which hides our controls, our Watch Together
  notices, our voice controls and any subtitles we draw. **Inline landscape plus
  picture-in-picture** is the iPhone design.
- **Autoplay policy** [V]: a "play" arriving from a partner is not a gesture on this device, so the
  browser may block it. Jellyfin #4901 hangs silently on the splash screen. **Joining must take
  one tap** (which also unlocks audio), and a blocked play needs a visible "Tap to resume".
- **Cast from a browser** needs HTTPS and Chrome; the stock receiver cannot take part in sync
  [V], which [D124](../../decisions/working-notes.md) already accounts for.

### Watch Together: what works

| Pattern | Evidence |
| --- | --- |
| The invite link lands **in the session**, not in an app store or a website | Apple: *"get them to the shared content quickly"*; Teleparty (2.8★ iOS) and Kosmi reviews complain of links that open a store page [V] |
| Everyone may pause and seek, last change wins | SharePlay guidance; Discord recommends its sharing mode for small groups [V] |
| **Named events**: *"{} paused at {}"*, *"{} jumped from {} to {}"*, *"{} left, {} paused"* | Syncplay [V]. Jellyfin has a *"{0} is buffering…"* string that nothing uses, so a partner sees an unexplained spinner [V, local source] |
| **Undo seek** | Syncplay [V] |
| Subtitles, audio track and volume **per person** | SharePlay, Discord; the second most-liked post (135) in Plex's withdrawal thread uses Watch Together *"in 2 different languages"* [V] |
| **On a TV, the TV plays and the phone talks** | Apple's SharePlay design [V]; top post (157) in Plex's thread: dad *"only had a TV… web browser is too clunky"* [V] |

### Watch Together: what fails

- **Invites that need a prior friendship or an app restart** (Plex), and re-inviting everyone for
  every episode (Plex) [V].
- **Voice in a separate app**: iOS lowers media during a call, *"you can't hear anything from the
  show while you're on FaceTime"*. Teleparty charges for voice [V].
- **One volume for the partner and the film**: Hearo users say voice *"overtakes the video
  sound"* and ask for a separate slider; Discord gives each stream its own [V].
- **Ducking on by default, with a help page to turn it off** (Discord) [V]. Push-to-talk works in
  a browser only while the window has focus [V].
- **Sync tuning in milliseconds exposed to users**, and an icon that reads as "refresh"
  (Jellyfin SyncPlay, #2340) [V].
- **iOS Safari with the microphone open** sends call audio to the earpiece at low volume (WebKit
  #230902, from iOS 15) [V]. `navigator.audioSession` is the lever and its behaviour is
  UNVERIFIED.

## 5. What kills clients

- **The single maintainer leaves.** DUNE was archived in January 2026, after issues titled *"another
  dead client?"* and *"Where is SAM?"*; Aeon Nox has been dormant since 2022 [V].
- **The vendor stops caring.** Plex Media Player is end-of-life; the official Plex for Kodi was last
  updated 2023-12; Plex staff say the tvOS app is *"more than a year out of date"* [V].
- **The platform churns.** Arctic Fuse was rewritten twice in three years; Jellyfin's Samsung app
  spent years on seller approval (+402) [V].
- **A server release breaks shipped clients.** Jellyfin 10.11 broke Swiftfin and Dolby Vision on
  LG [V].
- **The upstream closes a gate.** From September 2026 Plex requires a paid pass for remote play
  in third-party apps such as Infuse [V].

**And a method note:** lifetime store ratings sit one to two stars above recent reviews (Plex
4.61 lifetime against 2.62 recent; Infuse 4.67 against 3.19) [V], so recent written reviews are
the instrument.

## 6. Against this project's design

### Already decided, and confirmed

| Finding | Where we already stand |
| --- | --- |
| Correct direct play is the product (Infuse, PM4K) | [D2](../../decisions/working-notes.md), [D17](../../decisions/working-notes.md) |
| Track choices stick per series; per-client defaults are the complaint | [D24](../../decisions/working-notes.md) |
| A user's correction is permanent; no "Plex Dance" | [D170](../../decisions/working-notes.md), [D183](../../decisions/working-notes.md) |
| Show what the scanner understood instead of demanding renames | [D163](../../decisions/working-notes.md) |
| The user orders their own home | [D138](../../decisions/working-notes.md) §2 |
| Never remove the old layout without a way back | [D108](../../decisions/working-notes.md), [D139](../../decisions/working-notes.md) |
| A restriction holds on every access path, including artwork and people | [D76](../../decisions/working-notes.md), [D164](../../decisions/working-notes.md) |
| Written thresholds; missing runtime is never "watched" | [play-state.md](../plans/play-state.md) |
| A failure names the failing hop | [D152](../../decisions/working-notes.md), [D92](../../decisions/working-notes.md) |
| Nothing to sell, so no store outranks the library | [D13](../../decisions/working-notes.md) |
| Downloads are expected on phones | [offline-download.md](../plans/offline-download.md), deferred past Cut A |

### Gaps this round found

- **Continue Watching:**
  - it opens the home screen by default, ordered by recency
  - selecting an item **plays immediately with no resume prompt**, and "start over" lives in the
    player
  - every item can be **hidden with undo on every client**, and hiding is not "watched"
  - `play-state.md` makes removal *possible* but does not require the surface
- **Unattended playback:** a "still watching?" rule (Netflix: three episodes and 90 minutes with
  no input), and plays nobody watched must not mark anything watched.
- **Track menus name the track:** SDH, forced, commentary and language, from the kind inference
  [D24](../../decisions/working-notes.md) already specifies.
- **The remote's seek model:** left and right seek relative to the current position from any
  on-screen state, 10 s steps, thumbnails for anything over 15 minutes.
- **Browsing:** no pagination (virtualised lists), no search cap, titles on posters by default,
  and a list view.
- **Settings provenance:** every effective setting shows where it came from (person, library,
  series, device override). The scope chain exists; its visibility does not.
- **The transcode reason** is shown in the player's info panel and in the admin's sessions view,
  not only in a log.
- **Errors:** every error names the item, the cause and one next action.
- **Accessibility:** a release gate, not a ranked item. Every focusable card carries an
  accessible name derived from its item.
- **Autoplay previews:** none by default, and one switch every client obeys if they are ever
  added.
- **The web player (Cut A):**
  - the keyboard map above, bound on `code`, with a `?` overlay and the Space trap closed
  - a subtitle layer that clears the controls
  - iPhone inline plus picture-in-picture
  - join takes one tap, and a blocked play shows "Tap to resume"
  - a drop shows a retry countdown
  - Media Session including the microphone
- **Watch Together (Cut A):**
  - named events, and undo for a seek
  - per-person subtitles, track and volume
  - **two volume sliders, film and partner**
  - voice inside the page
  - a "reconnecting" state instead of an ended session
  - the session survives from one episode to the next
  - on a TV, the TV plays and the phone talks
- **Compatibility:** each server release is tested against the previous client release (a
  [D85](../../decisions/working-notes.md) CI item made explicit).
