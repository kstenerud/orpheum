> **ABOUTME:** How media apps and platform guidelines treat Back on a title page (from search, from
> a home row, from a series), and whether Orpheum's on-screen Back should retrace history or go
> up. Read before changing `web/apps/web/src/back-button.tsx` or `navigation.ts`'s `back`.

# Back From a Title Page: History or Up

- **Date:** 2026-10-24.
- **Requested by** the owner, 2026-10-24: drill-down pages now have an on-screen Back, because a
  TV's browser keeps its own back out of the remote's reach. Today it goes back in the history
  when the entry before is the app's, else up one level (commit `97ad58a9`). Search → episode →
  Back returns to search, not the season; Continue Watching → episode → Back returns to Home. The
  question: (1) strict **Up**, always the parent level, with the browser's back still retracing
  history; or (2) **Back**, retracing where you came from.
- **Builds on:** [ui-competitors.md](ui-competitors.md) and
  [ui-design-practice-2026-10-05.md](ui-design-practice-2026-10-05.md), which do not cover Back.
- **Cache:** pages under `/yoloai/cache/http/back-navigation/` (`<name>.html` or `.json` as
  fetched, `.txt` extracted, `.url` beside each). Clones in `/yoloai/cache/git/`: jellyfin-web
  `b290955` (2026-10-24) and TV Bro `6cb4b7c` (2026-07-29). Kodi's sources were fetched as single
  files from `xbmc/master` on 2026-10-24.
- **Labels:**
  - **[1P]** the vendor's own documentation, source code or staff post.
  - **[U]** a user report: forum thread, issue, review.
  - **[J]** a journalist's or reviewer's article.
  - **[S]** second-hand: a search engine's summary of a page that could not be read in full.
  - **[I]** my inference. **Unverified** marks a behaviour I know of but found no source for.
  - **Judgement** marks a recommendation that is an opinion.

---

## 1. Summary

No product found uses strict Up for its Back. Every TV platform's guideline and every product whose
behaviour could be sourced retraces history. Where a product does offer hierarchy, it is a
**separate, named control** on the page: Plex's *Show* and *Season* buttons, Jellyfin's series and
season name links, Apple's *Go To Show*. The one complaint that asked for Up-like behaviour
(Disney+, below) asked for it after playing from Continue Watching. It wanted a way to reach the
episode list, not a Back that skips the user's path.

**Recommendation (judgement): keep option (2).** Add an explicit way up beside it: the episode
page's place line, linking to the season as well as the series. §5 gives the reasons and the
refinements.

### 1.1 What Back does on a title page

"History" means it returns to the screen the user came from. "Up" means it goes to the parent,
whatever the path.

| Product / platform | From search | From a home row (Continue Watching) | From the series page | Hierarchy offered separately | Source |
| --- | --- | --- | --- | --- | --- |
| **Netflix, TV** | History (unverified) | A Continue Watching tile plays. *Episodes & Info* opens the title page [S]. Back returns to the row (unverified) | Seasons are a selector on the title page, not a level (unverified) | Episodes panel in the player | Engadget 2025-05-07 [J]: Back on the browse rows jumps to the top menu |
| **Netflix, web** | Details open as a modal over the page you were on: `/search?q=…&jbv=<id>`. Closing returns there [S] | Same modal over `/browse` [S] | Season dropdown in the modal (unverified) | — | §2.1 |
| **Disney+, TV** | not found | Continue Watching plays. Back from playback goes **Home**, not to the series. A reviewer complains | Season tabs on the series page [S] | None in playback (the complaint) | What Hi-Fi 2023-07-09 [J/U] |
| **Prime Video, TV** | History: Back returned to the search when the user wanted Home [S] | not found | not found | — | Samsung community thread [S] |
| **Max, Hulu** | not found | not found | not found | — | §6 |
| **Apple TV app** | not found | Up Next plays. Back during playback shows info and a *Go To Show* box, which lists the episodes | Seasons as tabs on the show page (unverified) | *Go To Show*; *Details* from a long press on an Up Next tile | Apple Community 2023-06-03, 2025-03-27 [U]; Apple support [1P] |
| **YouTube, TV** | History: users of SmartTube, a third-party YouTube client, call Back-to-Home after a search result a bug | not found | n/a | — | SmartTube #5778, 2026-05-14 [U] |
| **Plex, web** | History: "Clicking the Back button will return you to the previous screen" | History | History | Home button | Plex support [1P], modified 2025-04-08 |
| **Plex, TV (2026 app)** | History | History | History | *Show* and *Season* buttons on the episodes screen | Plex forum 942393, 2026-09 [U]; staff reply [1P] |
| **Jellyfin, web** | History: the header's back arrow calls `history.back()` | History | History | Series and season names on an episode's page are links | jellyfin-web `b290955` [1P] |
| **Jellyfin, Android TV** | History | History. Opened from the launcher's row, users expect Back → the app's Home | History | — | jellyfin-androidtv #5910, #5010 [U] |
| **Emby, web** | History (inferred from its shared lineage with Jellyfin) | — | — | — | Emby forum 126387 [U] (a back bug) |
| **Kodi** | n/a | A home widget opens the library window with `return`: Back from there returns Home | Back pops the window's folder history, which matches the hierarchy when drilling down | `..` parent item | Kodi source [1P]; xbmc #20134 [U] |
| **Infuse, tvOS/iOS** | History (the iOS navigation stack: the back label names the previous page) | History | History | — | Firecore 32382, 2021 [1P/U] |
| **Channels** | not researched | — | — | — | §6 |

---

## 2. Products, in detail

### 2.1 The big streaming services

- **Netflix (TV).** The 2025 TV redesign put the menu at the top. "You'll be able to switch to the
  top navigation bar by simply tapping the back button on your TV remote" (Engadget, 2025-05-07)
  [J]. A Stremio feature request (2021-09-25) cites Netflix and Kodi as apps where Back "simply
  doesn't exit … It just navigates backwards until the Home screen and then stops" [U]. It is
  **unverified** that Back from a title page reached from search returns to the results. A tap on
  a Continue Watching tile opens a menu with *Episodes & Info* [S, Tom's Guide via search]. So the
  path to the series is an explicit choice, not Back.
- **Netflix (web).** A search-result description of the *Netflix URL Fix* extension says opening a
  title's details from search gives `https://www.netflix.com/search?q=Title%20Name&jbv=70140358`
  [S]. So the details sit as a modal over the page you came from, and closing it leaves you there.
  A direct `/title/<id>` link opens it on its own [S]. A Medium write-up says Netflix's search
  query does not survive leaving the results; it could not be read (HTTP 403) [S].
- **Disney+.** What Hi-Fi (2023-07-09): "if you want to access the entire roster of episodes …
  you have to come out of that show entirely … Disney forces you to go out of the selected
  episode, scroll around to find the show's main tile (i.e. access it via its main page not via
  Continue Watching) … all it would take would be a small icon on the screen or a simple click of
  a back button to take you into the episode list rather than back to the home screen" [J/U].
  This is the one source that asks for Up-like behaviour. It concerns Back from **playback**
  started from a home row, and the first remedy it names is "a small icon", which is an explicit
  link.
- **Prime Video.** A Samsung community thread, summarised by search: "When trying to quit and
  return to the home screen by pressing return on the remote, it returns you back to the search
  rather than going home" [S]. The page did not render outside a browser.
- **Apple TV app.** To get from Up Next to the episodes: "Press the return button with backward
  arrow … Go to box with 'Go To Show' and select. Episodes will appear below" (Apple Community,
  2025-03-27) [U]. Or "click-and-hold on a tile in the Up Next list, then choose Details"
  (2023-06-03) [U]. Apple's guide: "During playback, press the Back button … to see additional
  information, extras, and an option to replay from the beginning" [1P]. Back itself does not jump
  to the show; the show is a named action.
- **YouTube on TV.** No first-party source found. SmartTube is a third-party YouTube client for
  Android TV. Its issue #5778 (2026-05-14) reports, as a bug: search, play a result, press Back →
  "Returns to Home"; "Pressing the back button always returned to the search term list" before
  [U]. Users of a YouTube-like 10-foot UI expect Back to retrace their path to the search.
- **Max, Hulu.** Nothing specific found. TechCrunch's 2019 piece on Hulu's redesign is cached but
  says nothing about Back.

### 2.2 Self-hosted and other players

- **Plex.** Plex Web: "The Home button will return you to the Dashboard from anywhere within Plex
  Web App. Clicking the Back button will return you to the previous screen" (support article,
  modified 2025-04-08) [1P]. On TV, the 2026 "new experience" has *Show* and *Season* buttons in
  the top row of a season's episodes screen (forum 942393, 2026-09-02, a focus bug about them;
  staff confirmed a fix in 2026.21.0) [U/1P]. Its TV apps had Back bugs that trapped users inside
  a series: "If I long hold on an episode and go to the series… I cannot go back out to the main
  menu" (939652, 2026-06-13). "I'm in a TV show's listing … I can't get out of it" (940193,
  2026-07-01). Staff: "We are aware … fixed in … 2026.13.0" [U/1P]. A long history makes Back
  tedious. A 2019 request (491675) counts six Backs after a chain through similar artists, and asks
  for long-press Back → Home [U].
- **Jellyfin web.** The header's back arrow runs `appRouter.back()`, which calls `history.back()`.
  It shows when `canGoBack()`: not on a start page, and `window.history.length > 1`
  (`src/components/router/appRouter.js:58`, `:124`; `src/scripts/libraryMenu.js:94`, `:626`)
  [1P]. `history.length` counts entries from other sites too. So in issue #102 (2018), the back
  arrow on a page opened from the address bar returned to Google. The reporter expected "an
  episode list (show detail page)". A maintainer answered: "most people would expect the back
  button to go to the previous state of their history, not redirect them by force to a state they
  haven't been to before" [U/1P]. For hierarchy, an episode's page shows the series and the season
  as links (`src/apps/legacy/controllers/itemDetails/index.js:443-454`) [1P].
- **Jellyfin Android TV.** Back pops the app's own history. #5910 (2026-10-22): an item opened
  from the launcher's Continue Watching row, from global search, or by deep link has no history,
  so Back closes the app. "I expect Back to return to the Home screen (and a second Back press to
  close the app)" [U]. #5010 is the same after the app returns from the background. #2053 (2022)
  is the opposite failure. Playing episode after episode from their info pages piles them up, and
  Back steps "through all the watched episodes of that series until you get back to the home
  screen". A maintainer's answer was a sidebar for quick navigation, not a change to Back [U/1P].
- **Emby.** Its web app shares Jellyfin's ancestry, so its Back is presumably history too
  [I, unverified]. Forum topic 126387 (2024-02) reports Back jumping to one fixed earlier page. A
  user said this "is not how the feature has worked in Emby" and it differed from the theater and
  phone apps. The admin had seen it "once or twice" [U].
- **Kodi.** In a library window, Back pops the window's folder history (`m_history`) to the parent
  folder. At the window's root, or at the folder it was opened on, Back falls through to the
  previous window (`CGUIMediaWindow::OnBack` and `GoParentFolder`,
  `xbmc/windows/GUIMediaWindow.cpp:208`, `:1255`) [1P]. A skin's home item opened with the
  `return` parameter makes Back from that folder return Home. When drilling down, the folder
  history and the hierarchy coincide. When the user jumps, they part: xbmc #20134 (2021) reports
  Back from a playlist landing "at the root library view which one otherwise never normally sees",
  fixed in 2026-09 [U/1P]. Kodi's remote keymap binds `back` to the `Back` action globally
  and in the media windows (`system/keymaps/remote.xml`) [1P].
- **Infuse.** A 2021 bug report shows the back label changing among "Startseite", "<", "Zurück"
  (the previous page's name, as iOS's navigation bar shows it). Fixed in 7.0.9 [U/1P]. On tvOS,
  users asked for Menu on the home screen to go to the top, not to exit (38455). Added in 8.3.2,
  2025-11-19 [1P]. "Is hitting the back button repeatedly the only way to get back to the home
  screen?" (59182, 2026-03-04) [U].
- **Stremio (Android TV).** The 2021 request above asks that Back at the root not exit at once [U].

### 2.3 TV Bro: why the page needs its own Back

In TV Bro, the remote's Back never reaches the page and never goes back in the history.
`handleBackNavigation` closes the cursor menu, hides full screen or the bottom panel, or else
**toggles the browser's menu bar** (`app/src/main/java/com/phlox/tvwebbrowser/activity/main/MainActivity.kt:888-907`,
`6cb4b7c`) [1P]. The history's back is an icon in that bar. MakeUseOf: "Hitting the back button on
your remote instantly returns you to the top navigation buttons" [S]. So on the owner's TV, the
on-screen Back is the only remote-friendly way back. This belongs in
[idiosyncrasies.md](../../idiosyncrasies.md) once a device confirms it [I].

LG's webOS app platform differs. It "handles the Back button press on Magic Remote using the DOM's
history object" unless the app opts out (`disableBackHistoryAPI`) [1P]. There the remote's Back
**is** `history.back()`, so an on-screen Back that also retraces history agrees with the remote.
One that went up would disagree with it.

---

## 3. Platform guidelines

### 3.1 Android: Up and Back, then one of them

- **The original distinction** (Android Design, "Navigation with Back and Up", c. 2012, mirrored
  at stuff.mit.edu) [1P]:
  - "The Up button is used to navigate within an app based on the hierarchical relationships between
    screens." Back goes "in reverse chronological order, through the history of screens the user
    has recently worked with".
  - Even then: "Sometimes a screen doesn't have a strict position within the app's hierarchy, and
    can be reached from multiple entry points … In this case, the Up button should choose to
    return to the referring screen, behaving identically to Back."
  - Changing view within a screen, "Switching views using a dropdown (aka collapsed tabs)", "does
    not change the behavior of Up or Back".
  - For a screen entered from a widget or notification, Back should follow "the complete upward
    navigation path to the app's topmost screen", inserted into the back stack.
- **Today** ("Principles of navigation", developer.android.com, updated 2026-10-23) [1P]:
  - "**Up and Back are identical within your app's task**." "Within your app's task, the Up and
    Back buttons behave identically."
  - "The Up button never exits your app"; it is not shown on the start destination.
  - A deep link replaces the back stack with a **synthetic** one: "the start destination and the
    deep link destination", plus the start destinations of nested graphs above it. It "should
    match a back stack that could have been achieved by organically navigating through the app".
  - Google kept Up's two promises: never leave the app, and still lead somewhere after a deep
    link. It dropped the separate hierarchy walk inside the app [I].
- **Material.** The top app bar's navigation icon may be "an up arrow, which navigates upward in
  an app's hierarchy, or a back arrow, which returns to the previous screen" [S]. m3.material.io
  renders only with script and was not read in full.

### 3.2 Android TV and Google TV

"Navigation on TV" (design guide, updated 2025-05-09) [1P]:

- "When the user presses the remote's back button, take them to the previous destination.
  Ultimately, the user should land on the Google TV Home or Launcher if they keep pressing the
  back button."
- "Ensure that the back button isn't gated by confirmation screens or part of an infinite loop."
- "**Don't display a back button.** Unlike on handheld devices, the back button on the remote is
  used to navigate backward on a TV. It's not necessary to show a virtual back button on the
  screen." This is written for apps that receive the remote's Back. TV Bro does not pass it to the
  page (§2.3), which is why Orpheum's page needs one [I].
- "If the user goes directly to a details page on the Moviestar app from Google TV and then presses
  the back button, they are taken to the home page of the Moviestar app."
- The TV quality checklist (updated 2026-06-29): **TV-DB** "Back button presses lead back to the
  Android TV home screen" [1P].

### 3.3 Apple

- **tvOS HIG, "Remotes"** [1P]: "In almost all cases, open the parent of the current screen when
  people press the Back button. At the top level of an app or game, the parent is the Apple TV
  Home Screen; within an app, the parent is defined by the app hierarchy, and **isn't necessarily
  the previous screen**." The same page's table gives Back as "Returns to previous screen. Exits
  to Apple TV Home Screen." It is the only guideline found that leans to Up. Its own table, and
  `UINavigationController`, which pops the stack the user pushed, both say "previous screen". So
  apps in practice retrace, as Infuse's labels show (§2.2) [I].
- **HIG, "Toolbars"** (iOS) [1P]: "People know that the standard Back button lets them retrace
  their steps through a hierarchy of information." Retracing steps is how iOS works: the back
  label names the screen before, wherever that sits in the hierarchy (Infuse's labels).

### 3.4 Samsung, LG, Microsoft

- **Samsung Tizen**, "Return Key Policy" [1P]: "From a detail page within an application, clicking
  the 'Return/Exit' key must display the previous page." From the home page it must ask before
  exiting. Long press exits; apps must not register it.
- **LG webOS**, "Back Button" [1P]: "If the previous page exists, move back to the previous page."
  At the entry page, webOS 6.0+ asks whether to exit. By default the platform's Back is the
  page's `history` (§2.3).
- **Windows**, "Navigation history and backwards navigation" [1P]: "The user expects the back
  button to navigate to the previous location in the app's navigation history." It is shown
  disabled when there is nothing to go back to.

### 3.5 The web

- The browser's Back is history, and an in-page control that disagrees with it gives a phone or a
  desktop two "backs" with different results on one screen [I].
- **NN/g, "Breadcrumbs"** (2018, modified 2026-09-01) [1P]: breadcrumbs "should display the
  current location in the site's hierarchical structure, not the session history". A history
  trail "won't provide any benefit for users who arrive on a deep page directly from an external
  link". The web's answer to "where am I in the hierarchy" is a **separate** location control. The
  Back control stays history.
- **SPA pitfalls** (Yiou, "The difficulty of maintaining routing history in a single page app"
  [U]): a page cannot tell from `popstate` whether the user went back or forward. Labels like
  "Back to category" need the app to track its own stack. Jellyfin's `history.length > 1` test is
  that pitfall: it counts other sites' entries (§2.2).

---

## 4. Patterns and pitfalls

1. **Back retraces; Up is a link.** Every sourced product keeps Back as history and puts the
   hierarchy on the page as named controls: Plex's *Show*/*Season*, Jellyfin's name links, Apple's
   *Go To Show*, Netflix's *Episodes & Info*. Android merged its two buttons inside the app (§3.1).
2. **Seasons are often not a level.** Android files a dropdown or tabs as a view change, not
   history (§3.1). Netflix's season selector and Disney+'s season tabs (unverified, [S]) treat a
   season as a view of the series. Orpheum's season is a route with a `season` parameter, and the
   season list is a separate screen. So Up from an episode has two levels to climb where Netflix
   has one [I].
3. **A deep entry needs a synthetic way back.** Google TV sends Back from a deep-linked details
   page to the app's Home. Android mobile builds a synthetic stack. Jellyfin Android TV's failure
   to do either (#5910) is reported as a bug. Jellyfin web's failure, leaving for Google (#102),
   is the web version. Orpheum's fallback goes up one level at a time, which ends at a list and
   never leaves the app.
4. **Back loops and traps.** Google forbids infinite loops (§3.2). Plex's 2026 TV app trapped users
   in a series (§2.2). Going up by **pushing** the parent creates a loop: up, then back, returns to
   the child. Orpheum avoids it by replacing the entry (`97ad58a9`).
5. **Long histories.** Back through many pages is tedious (Plex 491675, Jellyfin #2053, Infuse
   59182). Products answer with a one-move Home (long press, a sidebar, the top menu on Back), not
   with Up.
6. **Leaving the app.** On the web, `history.back()` from the first entry leaves the app. Up never
   does. An in-page Back must know whether the entry before is its own. Orpheum's `appDepth` in
   `history.state` does; `history.length` does not.
7. **Where Back lands matters as much as where it goes.** History is only worth retracing if the
   place is restored. SmartTube's users wanted the search list back (§2.1). Google TV's guidance on
   top and side menus is about focus on return. Infuse's users wanted the home screen scrolled to
   the top.

---

## 5. Recommendation for Orpheum

**Keep option (2): Back retraces where you came from.** Add hierarchy as named links, not through
Back. Judgement, for these reasons:

- **Every TV platform says "previous page".** Google TV, Samsung, LG and Windows all do (§3.2,
  §3.4), and so does Apple's own behaviour table (§3.3). Only one sentence of the tvOS HIG says
  parent, and its own table and UIKit contradict it.
- **It agrees with the other backs.** On a phone or a desktop the browser's Back is in view. Under
  option (1) the two would differ on the same page: search → episode, page Back → season, browser
  Back → search. On webOS the remote's Back is `history.back()` (§2.3), so option (1) would also
  disagree with the remote.
- **Nobody found does option (1).** Android dropped a distinct Up inside the app (§3.1). Users file
  "Back went Home instead of to my search" as a bug (SmartTube #5778).
- **The case for Up is a case for a link.** The only Up-like complaint, Disney+'s, asks for a way
  to the episode list after Continue Watching, and names "a small icon" first (§2.1).

### 5.1 Refinements

1. **Make the way up explicit on an episode's page.** Today `TitleFacts` renders one link,
   "Series · S1 E3", to the series' page without its season (`web/apps/web/src/title.tsx:118`). A
   link to the season too, like Jellyfin's series and season names or Plex's *Show*/*Season*,
   would serve search → episode → season and Continue Watching → episode → season without changing
   Back. The season page already has *All seasons* (`series.tsx:80`) [I].
2. **Keep the fallback for an entry with no app history.** That covers a bookmark, a typed address,
   a link opened in a new tab, and a TV Bro start page. Going up one level by **replacing** the
   entry matches Android's synthetic stack and Google TV's deep-link rule. Each further Back goes
   up again until a list, and it never leaves the app or loops. Google TV goes straight to Home.
   Orpheum's step-by-step path ends in a list either way, and each step shows a real parent, so I
   would keep it [I].
3. **Reloads keep history-based Back.** The depth lives in `history.state` (`navigation.ts`,
   `depthOf`), which survives a reload in every browser [I, from the HTML spec's session history;
   not tested on TV Bro]. A reloaded episode reached from search still goes back to the search.
4. **Restore the place Back returns to.** The search query is in the address (`/search?query=`,
   `router.ts:67`), so the results come back. Whether the list's scroll and the focused tile come
   back too was not checked. On a TV that decides whether Back is useful (§4 item 7) [I].
5. **One move to Home from deep in a history.** For long chains, a Home link that the remote can
   reach, not a smarter Back (§4 item 5). Whether Orpheum's header is one D-pad move away on TV Bro
   was not checked.
6. **Do not name the destination.** An iOS-style "‹ Search" label would need the app to track
   the entry before, which `popstate` makes hard (§3.5). Not worth it for two users [I]. Apple's
   HIG prefers the standard chevron to a text label that says Back; a word may read better at ten
   feet, which is the owner's call [I].
7. **No Back on top-level pages**, as now: Home, the lists and Search. Up never shows at the start
   destination (§3.1). The browser and the TV handle leaving.

Option (1) would be right if seasons and series were the only ways in. Search, Continue Watching
and the person pages are not.

---

## 6. Not tried

- **No product was run.** Every behaviour of Netflix, Disney+, Prime Video, Max, Hulu, the Apple TV
  app and YouTube on a TV or in a browser comes from documents and reports, or is marked
  unverified. A session with each on a TV and in a desktop browser would settle the table's
  unverified cells, above all from search → title → Back.
- **Max, Hulu and Channels:** no source found. Channels was not searched beyond the general queries.
- **Reddit** was not read; earlier research found it unreadable from here.
- **Material 3's top app bar page** renders only with script and was read through search
  summaries.
- **TV Bro on the owner's TCL TV:** the Back behaviour is read from source at `6cb4b7c`, not
  observed. Neither was whether `history.state` survives a reload there.
- **Samsung's and LG's own browsers** (not their app platforms): how their remote's Back treats a
  page was not looked up.
- **Orpheum's own scroll and focus restoration** on Back, and whether the header is reachable by
  D-pad on a TV.
