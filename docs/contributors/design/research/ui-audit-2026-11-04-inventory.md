> **ABOUTME:** The UI audit's inventory: every screen of the web client as it stands on staging,
> measured on desktop (1920×1080, 1366×768), a phone (412×915 emulated, the Pixel 3a) and the
> Fire TV's D-pad. What is on each page, where focus lands, how many presses things take, and the
> layout problems seen. Observations only; the redesign belongs to the synthesis.

# UI Audit: Every Page as It Is

- **Date:** 2026-11-04, on staging.
- **Builds on:** item 13 of
  [navigation-and-widgets-answers-2026-10-27.md](../plans/navigation-and-widgets-answers-2026-10-27.md)
  (the arrows prefer a control in line). "Confirms item 13" below refers to it.
- **Requested by** the owner, 2026-11-04: a research run for a full UI audit, to see what should
  be reachable in how many clicks, where focus should land and how controls should be laid out.
  This file is the half that records the UI as it is. The outside research (experts, competitors)
  is a separate file.
- **Accounts:** `lead-tests` (a tester: every library, Watch Together) for the viewer's pages;
  `lead-admin` for Manage. Neither has the other's view of Home: an admin with nothing in
  progress sees no Continue watching row.
- **Screenshots:** `/yoloai/files/ui-audit-2026-11-04/`, named `<page>-<input>.png`:
  `desktop1920`, `desktop1366`, `phone412` (Playwright's headless Chromium, a 412×915 viewport,
  not a touch device), `pixel` (the real Pixel 3a, Chrome, a 392×675 CSS viewport), `tv-<n>`
  (the Fire TV's Silk, a 960×540 CSS viewport under Silk's own 120 px toolbar). A `-full`
  suffix is the whole page.
- **How measured:** positions are CSS pixels from the page's top (`top:left`). "Focus on load" is
  `document.activeElement` after the page settled. Fire TV walks pressed real keys through `adb`
  and read the focused element after each. Desktop Tab walks skip the shell's seven controls,
  which come first on every page.
- **Data caveat:** lead-tests and its person page hold about 115 devices, nearly all automated
  sign-ins. That inflates Your account and the person page, but the layout grows the same way for
  any account with many devices.

## The shell (on every page)

- **Bar, left to right:** logo (Home), Films, Series, Libraries (a list box), then on the right
  the realm name ("staging"), the admin's "Something needs your attention" (admin only), Watch
  Together, Cast to a device, Menu. Every control but the realm is an icon without a visible
  label (`home-desktop1920.png`, `home-tv-1.png`).
- **Menu, tester** (`shell-menu-desktop1920.png`, `menu-tv-1.png`): Device tests, This device,
  Your account, About, Switch household, Sign out. Focus lands on the first, Device tests.
- **Menu, admin** (`shell-menu-admin-desktop1920.png`): Needs a match, Libraries, People, Settings,
  Jobs, System, then the tester's six. Focus lands on Needs a match. There is no "Manage" heading
  dividing the two groups in the outline; the admin's group comes first.
- **Libraries list box:** Movies, Musicals, Musicals folders, Shows, All Libraries. Focus on the
  first (`libs2-tv-1.png`).
- **Watch Together dialog** (`shell-watch-together-desktop1920.png`, `wt-tv-1.png`): heading,
  Start a session, Close. Focus on Start a session. Desktop Tab cycles Close → body → Start: the
  page body takes a stop inside the dialog.
- **Cast to a device dialog** (`shell-cast-to-a-device-desktop1920.png`, `cast-tv-1.png`): "Cast
  to", one button per device (Chrome on Android, Silk on Fire TV), Close. Focus lands on **Close**,
  not on a device; the same body stop in the Tab cycle.
- **Fire TV, the bar:** the first press on any page, whatever its direction, lands on the page's
  first control, never in the bar. Up from there reaches the logo. Up from Menu moves sideways to
  Cast to a device (confirmed, `menu-up` walk). Right at the end of a row in the page jumps up into
  the bar (confirmed: Continue watching's last card → Watch Together; series page's Play S2E1 →
  Watch Together; Your account's Remove → Menu). Back closes a menu or dialog and returns focus to
  its button (confirmed for Menu, Libraries, Watch Together, Cast).
- **Observed:** on the TV the bar plus Silk's toolbar take 240 of 1080 screen pixels on every page;
  the shell has no visible labels for its places, and the tester's menu leads with a developer's
  page (Device tests).

## Sign in

- **Outline** (`signin-desktop1920.png`, `signin-phone412.png`): logo, "Sign in", Realm, Name,
  Password, "This is my own device: remember my realm", Sign in, Get a code. Centred form.
- **Focus on load:** body; the Realm field is not focused.
- `/register` shows the same sign-in page when signed out.
- **Observed:** three fields before the button, the first (Realm) one most people type once.

## Home

- **Outline, tester** (`home-desktop1920.png`, `home-desktop1366.png`, `home-phone412.png`):
  h1 Home; Continue watching (cards, each with a Hide button under it); New films (18 posters in
  one row, "See all" at the far right); New series (18, "See all"). Page 1348 px tall at
  1920×1080: New series' posters are cut by the fold at both desktop sizes.
- **Admin's Home** (`admin-home-desktop1920.png`): no Continue watching; New films first.
- **Continue watching cards link straight to `/play/<file>`**, not to the title page.
- **Focus on load:** body (desktop, TV, Pixel). On the TV the first press lands on the first card.
- **Back into Home** (desktop, from a title opened from New films): focus returns to the poster
  that was opened; scroll restored to 0 (the row was at the top).
- **Fire TV walk** (`home-tv-1.png`, `home-tv-2.png`): Down: first Continue watching card → its
  Hide → New films' first poster. Right along New films moves one by one to the 18th; further
  Right stays. Down from the row's end goes diagonally to "See all series"; Up from the row's end
  goes to "See all films", not to Continue watching. Left from "See all films" goes to Menu in the
  bar. Only New films and the first Continue watching card are on screen at load
  (`home-tv-1.png`): New films starts below the TV's fold.
- **Observed:** Hide is a focus stop under every in-progress card, between the card and the next
  row, so Down from a card needs two presses to leave the row. "See all" sits 1800 px from the
  row's heading at 1920 wide.

## Films and Series (`/films`, `/series`)

- **Outline** (`films-desktop1920.png`, `series-desktop1920.png`): h1 "All Films" / "All Series";
  at the top right, Search and Sort icon buttons; the poster grid (10 a row at 1920, 7 at 1366,
  3 on a phone); an A–Z rail down the right edge. 2229 px tall at 1920, 5827 px on a phone.
- **Search** (`films-search-desktop1920.png`): opens a field, focused, labelled "Search the
  library chosen, or every one".
- **Sort** (`films-sort-desktop1920.png`): a list box of six orders; focus on the current one.
- **Focus on load:** body. **Back into the list** (desktop): focus returns to the poster opened.
- **Fire TV walk** (`films-tv-1.png`): the first Down lands on **Search** at the top right; the next
  Down goes to the grid's **last column** (The Marrowgate Family 2019), not the first poster. Reaching
  the first poster takes five Lefts. Right from any poster in the last column goes into the A–Z
  rail and further Right stays there. Up from Search goes into the bar (Watch Together).
- **Observed:** the grid has no heading of its own; the page title says "All Films" while the
  shell's icon says "Films" and the breadcrumbs elsewhere say "Movies" (the library's name).

## Libraries page (`/browse`)

- **Outline** (`browse-desktop1920.png`): h1 the library's name (Movies); Configure, Search, Sort
  at the top right; the same grid and rail as Films. With no library in the address it resolves to
  the first by name.
- **Focus on load:** body. **Fire TV** (`browse-tv-1.png`): the first Down lands on Configure,
  then the last column, as on Films.
- **Observed:** a second way to the same films (Films vs Libraries → Movies) with a different
  title and an extra Configure button.

## Title page, a film (`/titles/5284`, Twelve Monkeys)

- **Outline** (`title-film-desktop1920.png`, `title-film-desktop1366.png`,
  `title-film-phone412-full.png`, `title-film-pixel.png`): breadcrumb (Movies › Twelve Monkeys);
  a full-width backdrop about 400 px tall; poster; h1; year and length; tagline; synopsis;
  h2 **Versions**, a panel holding **Play** and a "File details" disclosure; **Mark as watched**
  below the panel; for an admin, **More actions** beside it. No cast, no director, no genres, no
  rating, no resume point shown, no audio or subtitle choice on the page.
- **Where Play sits:** 1920×1080 at y 935 (inside the fold by 100 px; Mark as watched at 1040 is
  cut); **1366×768 at y 810, below the fold**; 412×915 at y 973, below; the real Pixel (675 tall)
  at y 932, **1.4 screens down**; on the TV, below the fold at load (`title-film-tv-1.png`).
- **Focus on load:** body. **Back into the page** from the player: Play (desktop; TV when the
  film had played); body on the TV when the film never started (player2 walk).
- **Fire TV walk** (`title-film-tv-1.png`, `-tv-2.png`): Down → "‹ Movies" (the TV shows an
  up-trail, not the breadcrumb) → Down only scrolls the page by 384 px, focus stays on ‹ Movies →
  Down → Play → File details → Mark as watched. Play is the third press.
- **Clicks:** desktop Home → poster → Play: 2 (plus a scroll at 1366). TV from Home: 10 presses to
  the page (Twelve Monkeys was the seventh New film), 4 more to play: 14.
- **Observed:** the page's one main action is the last thing on it, under a heading ("Versions")
  that names an internal concept; the backdrop takes the space Play needs; Mark as watched is
  outside the panel it relates to.

## Title page, an episode (`/titles/3666`, Copper Kettle S2E1)

- **Outline** (`title-episode-desktop1920.png`): breadcrumb (Shows › Copper Kettle › …); no backdrop;
  h1 the episode's name; a line "Copper Kettle · season 2" (two links); synopsis; Versions with Play
  (y 397); Mark as watched (y 502). 1080 px tall: Play above the fold at every size.
- **Focus on load:** body. **Fire TV:** Down → "‹ Copper Kettle" → Play: two presses. The "Copper Kettle"
  and "season 2" links in the line under the title are skipped by Down and reached only by Tab.
- **Back into the page** from the series page (desktop): body, not the link used.
- **Observed:** the episode page and the film page share a template but not a layout: one has a
  backdrop that pushes Play down 540 px, the other none.

## Series page and its seasons (`/series/3665`, `?season=n`)

- **Outline** (`series-page-desktop1920.png`, `series-page-desktop1366.png`,
  `season-desktop1920.png`): breadcrumb; poster with h1, year, synopsis, **Play S2E1** (the next
  episode) and **Mark as watched** beside it; a row of season cards; h2 "Season 2" with **Mark
  Season 2 as watched** at the far right (x 1735); the episodes as a list, one row each (87 px).
  1951 px tall. Play S2E1 at y 277, above the fold everywhere (Pixel y 390).
- The season shown is the one in progress; `?season=n` changes the list, not the page's top.
- Mark as watched's visible label is "Mark as watched"; its accessible name is "Mark all of Ted
  Lasso as watched".
- **Focus on load:** body. **Back into the page** from an episode: focus returns to that episode.
- **Fire TV walk** (`series-page-tv-1.png`, `season-tv-1.png`): Down → "‹ Shows" → Down goes to
  the first season card, **past Play S2E1 and Mark as watched** (confirms item 13). Down then
  walks the episodes. Up from the first episode goes sideways to "Mark Season 2 as watched" at the
  far right, then the season row, then Mark as watched, then Play S2E1: Play is reached only from
  below. Right from Play S2E1 jumps into the bar. Left from Mark as watched (scrolled) went to Films
  in the bar. Down from the bar's Menu also lands on ‹ Shows and then passes Play (confirms "Down
  from the banner passes Play").
- **Observed:** the series' Play is placed beside the synopsis (good on every screen) while the
  film's is at the bottom; "Mark Season n as watched" is 1700 px from the episodes it acts on.

## Player (`/play/<file>`) and its overlay

- **Desktop** (`player-desktop1920.png`, `player-overlay-desktop1920.png`,
  `player-desktop1920-full.png`, `player-desktop1366.png`): the player is inside the page, not
  full screen. Breadcrumb (Movies › Twelve Monkeys › Playing); a title line; the video (y 143 to
  1007 at 1920); a control bar under it: the timeline (y 1043), Back 10 s, Pause, Forward 10 s
  (y 1095, **below the 1080 fold**), Volume and Full screen at the far right; "Ends at". Under the
  bar: Start over, Find subtitles…, Level loudness (a checkbox and its note), "Sound on this
  device" with Sound 50 ms earlier/later, Dialogue boost, the tracks, picture timing. 1553 px tall.
- **Phone** (`player-phone412.png`, `player-pixel.png`): the same column; the controls at y 437.
- **Fire TV** (`player2-tv-1.png`): full screen with an overlay: "‹ Twelve Monkeys" (up link) and
  "Ends at" above the timeline; Back 10 s, Play/Pause, Forward 10 s at the left; **Subtitles**,
  **More**, Full screen at the right. Audio and subtitle choice live behind Subtitles and More; on
  desktop there is no Subtitles button for a film with no subtitle tracks, only "Find subtitles…".
- **Focus on load:** body (desktop); on the TV the timeline, with the overlay hidden
  (`player-tv-1.png`): focus sits on an invisible control.
- **Fire TV walk:** with the timeline focused, OK moved focus to the video and hid the overlay;
  Down and Right on the video did nothing visible; the next Down showed the overlay on the
  timeline. Down → Back 10 s; Right walks Play/Pause, Forward 10 s, Subtitles, More, Full screen.
  Up from the buttons → timeline → the up link. **After pausing, the first Back left the overlay
  for the page under it** (focus on Pause, page scrolled 367 px) and **the second Back left** to
  the title, with Play focused (player3 walk). This confirms item 13's "first Back does nothing
  visible": the first Back changes view, not page.
- **Clicks:** Continue watching → playing: 1 click, 2 TV presses. Title → playing: 1 click
  (after a scroll at 1366), 4 TV presses from load.
- **Observed:** on desktop the play/pause row is below the fold at both sizes unless Full screen
  is used; tuning controls (loudness, sync, dialogue) and diagnostics share the page with the
  playback controls.

## Watch Together and casting screens

- `/together` with no session redirects to Home with a dialog "The Watch Together session is no
  longer there." and OK (focused) and Close (`together-desktop1366.png`).
- `/watch-together` and `/casting` are old addresses that forward to a film's page or
  Home; the live surfaces are the shell's two dialogs (above).
- `/cast-ready` (a receiver between videos) was not opened: it needs a cast in progress.
- **Not walked:** a live Watch Together session and a live cast's remote; both need a second
  device in the session and were out of this run's scope.

## Your account (`/account`)

- **Outline** (`account-desktop1920.png`, `account-desktop1920-full.png`, `account-pixel.png`):
  h1; "Signed in as"; **Activate a device** (code field at the far right, x 1452, name field and
  Activate on the next line); **Your devices** (one row each, Rename and Remove; about 115 rows,
  52 px each); Sign out everywhere else (y 6353); **Your server at home** (a link); **Change
  password** (y 6805); **Playing** (a checkbox, y 6975); **Testing** (y 7096). 7164 px tall at
  1920; **19006 px on a phone**.
- **Focus on load:** body.
- **Fire TV walk** (`account-tv-1.png`): Down → the code field → Down goes to the first Rename,
  **past the name field and Activate** (the same in-line preference as item 13). Right from Remove
  jumps to Menu. Change password and the Playing setting sit behind about 230 focus stops.
- **Observed:** the account's settings are under its device list, so they move down as devices
  are added; the code field is far from its label at desktop widths.

## This device (`/this-device`)

- **Outline** (`this-device-desktop1920.png`, `this-device-tv-1.png`): h1; "Its name" with
  Rename; "Game controller" with three radio buttons (automatic, bottom, right); a link "Your
  other devices are on your account page." 1080 px: everything above the fold.
- **Focus on load:** body.
- **Fire TV walk:** Down → Rename → Down goes to the account link, **past the three controller
  choices** (confirms item 13). They are reached only by Up from the link.
- **Clicks:** desktop Menu → This device: 2. TV from Home: 11 (to Menu 9, Down 1, OK 1).

## About (`/about`)

- **Outline** (`about-desktop1920.png`): h2 About (the only page whose top heading is an h2);
  "Film and series information" (TMDB notice); "Built on", a grid of links to 13 projects.
- **Focus on load:** body. All above the fold.

## Manage: the admin's pages

Each is two clicks from Home on desktop (Menu, the item), except where noted. Focus on load is
body on every one.

- **Needs a match** (`/identify`, `admin-identify-desktop1920.png`): h1; two tab-like buttons,
  "Several matches (0)" and "No match (12)"; a list of names, each with Look again. Fits the
  screen.
- **Libraries** (`/libraries`, `admin-libraries-desktop1920.png`): h1; Create a library,
  Rescan everything, Full rescan of everything; one card a library with "Manage this library" at
  its bottom. Slightly over the fold at 1920 with four libraries.
- **A library** (`/libraries/1`, `admin-library-desktop1920.png`): ‹ Libraries; h1; Folders
  (each with "This folder is gone for good…" and "This folder moved…"), Add a folder…; Scans
  (a select and Save, "What the scanner saw", Scan now, Rescan, Full rescan); Deleting it. Three
  clicks from Home. The irreversible "gone for good" button is the first control in the page.
- **Create a library** (`/libraries/new`, `admin-new-library-desktop1920.png`): one long form,
  name, kind (3 radios), metadata source (3), scanning (3), a checkbox, a select, three
  checkboxes, Choose a folder…, Create and scan at y 1018 (at the fold at 1920, below at 1366).
  Three clicks.
- **What the scanner saw** (`/libraries/1/files`, `admin-files-desktop1920.png`): h1; a route
  chooser ("Everything"); collapsed groups by outcome with counts. Four clicks.
- **People** (`/people`, `admin-people-desktop1920.png`): h1; Create Role, Create Account, Invite
  Someone; Invitations; Accounts; Roles ("Administrator 2 people", "tester 1 person"). Fits.
- **A person** (`/people/4`, `admin-person-desktop1920-full.png`): h1; name and Rename; note and
  Keep the note; role select and Give this role; New link; Activate a device; **Their devices**
  (one Remove each); **Remove lead-tests…** at y 6529. 6621 px tall, 16063 on a phone. Three
  clicks.
- **Settings** (`/settings`, `admin-settings-desktop1920.png`): h1; search and "Show advanced
  settings"; sections (Updates, Keys and passwords, General, Accounts and sign-in, Metadata and
  artwork, Playback, Upload, Remote access, Backups and upkeep). Each row shows the **raw key**
  (`dns_provider_token`, `session_idle_expiry_hours`) at the left and its field at **x 1520**, with
  an info button and a delete button beside it. 1597 px tall.
- **Jobs** (`/jobs`, `admin-jobs-desktop1920.png`): h1 and a list of finished and running jobs,
  newest first, each "Making a film's chunks #134 / Done / Why… / Queued…". **30393 px tall**,
  no focusable control in the page, no filter, no heading per group.
- **System** (`/system`, `admin-system-desktop1920-full.png`): h1; This server; What doctor finds;
  Work waiting; Outside services; Machine (hardware encoding, disks with per-disk limits and Set);
  Now playing; Resource use; Backups (Back up now at y 4829); Reports of what was filed wrongly;
  Reached from outside (Check now); Remote access; Leave the discovery (y 7405). 7497 px tall,
  12736 on a phone; no table of contents.
- **How to update** (`/update`): h1 and one paragraph ("Its discovery has said nothing of newer
  versions yet."). The same sentence heads Settings. No menu item leads here.
- **Needs attention** (`/needs-attention`): h1; a film link; links to System, Libraries and Needs
  a match. Reached from the bar's warning icon (one click).
- **Admin's title page:** as the tester's, plus More actions beside Mark as watched.

## Pages no menu leads to

- `/lab` and its probes (the device lab), `/ui-gallery`, `/lab/extend/player`: tools, not
  walked. The tester's menu does lead to the lab (Device tests), first in the list.

## Summary table

Entry depth is clicks on desktop / presses on the Fire TV from Home with nothing focused. "Key
action" positions are at 1920×1080 unless noted.

| Page | Depth from Home | Focus on load | Key action's position | Biggest problem |
| --- | --- | --- | --- | --- |
| Sign in | — | body | Sign in, 4th control | Realm first, nothing focused |
| Home | 0 / 0 | body (TV: first card on first press) | first card y 161 | Hide is a stop under every card; New films below the TV's fold |
| Menu | 1 / 9 | first item | — | tester's menu opens on Device tests |
| Films, Series | 1 / 4–5 | body | grid y 135 | TV's first Down lands on Search, then the last column |
| Libraries (`/browse`) | 2 / 7 | body | grid y 135 | a second route to the same lists with other names |
| Title, film | 1–2 / 10 | body | Play y 935; below fold at 1366, phone, Pixel, TV | Play last, under "Versions", pushed down by the backdrop |
| Title, episode | 2 / ~12 | body | Play y 397 | links under the title skipped by Down |
| Series page | 1 / ~7 | body | Play S2E1 y 277 | TV Down passes Play; Up lands sideways |
| Player | 1–2 / 2–14 | body; TV: hidden timeline | play/pause y 1095, below the fold | first Back after pausing does not leave |
| Watch Together dialog | 1 / 7 | Start a session | centre | Tab stops on the page body |
| Cast dialog | 1 / 8 | Close | centre | focus on Close, not a device |
| Your account | 2 / 12 | body | Change password y 6805 | settings under an unbounded device list |
| This device | 2 / 11 | body | Rename y 217 | TV Down skips the controller choices |
| About | 2 / 12 | body | — | h2, not h1 |
| Needs a match | 2 | body | list y 262 | — |
| Libraries (manage) | 2 | body | Manage at each card's foot | — |
| A library | 3 | body | Scan now y 705 | an irreversible button is the first control |
| Create a library | 3 | body | Create and scan y 1018 | below the fold at 1366 |
| What the scanner saw | 4 | body | groups y 177 | — |
| People | 2 | body | Invite y 133 | — |
| A person | 3 | body | Remove account y 6529 | device list buries the account's actions |
| Settings | 2 | body | fields x 1520 | raw keys as labels, field 1500 px from its label |
| Jobs | 2 | body | — | 30 000 px of finished jobs, no filter |
| System | 2 | body | Back up now y 4829 | 7 500 px, no contents |
| How to update | 0 links | body | — | unreachable from the menu |

## Patterns that recur

- **Nothing is focused on load.** Every page leaves focus on body, on desktop, phone and TV; only
  menus and dialogs place it. On the TV the first press lands on the page's first control
  whichever way it points.
- **The arrows prefer a control in line over a nearer one beside it.** On the TV, Down passes
  Play S2E1 (series), the controller choices (This device), the device name and Activate (Your
  account), and the title line's links (episode). Up lands sideways (Mark Season n as watched,
  Cast to a device). Right at the end of a row jumps into the bar.
- **The main action is not first.** The film's Play is the last control; the desktop player's
  play/pause is below the fold; account settings come after the device list; a library's first
  control is "This folder is gone for good…".
- **Lists that grow push everything after them down.** Your account, a person's page, Jobs and
  System have no paging, folding or contents; their length is set by data.
- **Controls far from what they act on.** Mark Season n as watched at the far right of the
  episodes; Settings' fields 1500 px from their keys; "See all" 1800 px from its row's heading;
  the activation code field at x 1452 under a heading at x 32.
- **Similar pages, different layouts.** Film page: backdrop, Play at the bottom under
  "Versions". Episode page: no backdrop, Play near the top. Series page: Play beside the synopsis.
  The page title says "All Films", the icon "Films", the breadcrumb "Movies".
- **Labels without words.** The bar is icons only; Watch Together and Cast use similar icons; the
  TV shows no focus at load.
- **Back is uneven.** Desktop Back restores focus to the opened card (Home, Films, series
  episodes, title Play) but not from a series page to an episode (body). On the TV, Back from a
  paused player first changes the view, then leaves.
- **Developer and diagnostic material sits beside the viewer's.** Device tests heads the tester's
  menu; the player page carries tracks and picture timing; Your account ends with a Testing
  section.
