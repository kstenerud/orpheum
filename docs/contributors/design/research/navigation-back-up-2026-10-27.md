> **ABOUTME:** How Back and Up should work across Orpheum's devices (desktop and phone browsers, the
> installed app, TV browsers driven by a remote), tested against 23 scenarios, the usability
> research, platform guidelines and what competitors do. Read before changing
> `web/apps/web/src/navigation.ts`, `back-button.tsx`, `play-up.tsx` or `up-next.tsx`'s history
> handling.

# Back and Up: History, Hierarchy, and What Up Does to the History

- **Date:** 2026-10-27.
- **Requested by** the owner, through the lead. The owner and the lead recorded a decision the
  same day in the working notes, called here **the Up decision**. It was not yet committed when
  this was written, so it is not cited by number. Its points:
  - §1: Back is the browser's back.
  - §2: Up goes to the page's parent on every page that has one.
  - §3: the library Up leads to is the one the person came through.
  - §4: Up is a history step like any other.
  - §5: the Play page's Back with nothing behind it goes up.

  The owner then had doubts about §4 ("click up a bunch, then press back, it brings you back
  down again") and about Back's use. They asked whether competitors are "effectively hijacking
  the back button to reverse-drill-out".
- **Builds on:** [back-navigation-2026-10-24.md](back-navigation-2026-10-24.md), which covers
  what Back does on a title page in each product and platform (its §1.1 table), and
  [controller-input-2026-10-25.md](controller-input-2026-10-25.md), which covers what a TV
  browser's remote Back reaches. Findings there are cited, not repeated.
- **Cache:** `/yoloai/cache/http/navigation-back-up/` (`<name>.html` or `.json` as fetched, `.txt`
  extracted, `.url` beside each). The earlier report's pages are in
  `/yoloai/cache/http/back-navigation/`. The jellyfin-web clone is at `/yoloai/cache/git/jellyfin-web`
  (2026-10-24).
- **Labels**, as in the earlier report:
  - **[1P]** the vendor's own documentation, source code or staff post.
  - **[U]** a user report.
  - **[J]** a journalist or reviewer.
  - **[S]** a search engine's summary of a page not read in full.
  - **[I]** my inference. **Unverified** marks a behaviour I know of but found no source for.
  - **Judgement** marks an opinion.

---

## 1. Summary

**Recommendation (judgement):**

1. **Back stays history.** It is the device's own Back: the browser's button, the phone's
   gesture, the remote's key.
2. **Up never adds history.** It goes to the page's parent, as Android's Up and iOS's pop do:
   - If an entry showing the parent is behind in this app's history, Up goes back to the nearest
     one, however many steps back that is.
   - Otherwise Up replaces the current entry with the parent.
3. **Up arrives at the parent showing where the person is:** the season reached, the child
   focused. A season, a sort and a search are **views** of a page, not levels. Changing one
   replaces the entry; it does not add one.
4. **Controls:**
   - **One on-screen Up**, labelled with the parent's name ("‹ Breaking Bad").
   - **No on-screen Back**, except on a TV whose browser never passes the remote's Back to the
     page (TV Bro), until there is an Android shell that does.

This is model (d) in §3. It answers the owner's objection: Up, then Back, never goes back down.
It also fixes three cases where the lead's model (b) still does: the up chain (§3.3), the binge
across seasons (§3.7), and the extra (§3.17).

**The owner's hypothesis, tested (§5):** competitors do not hijack Back to reverse the drill-down.

- **Their Back is their own history.** It looks like reverse drill-out because most paths into a
  title *are* drill-downs.
- **They keep history where the two differ.** Continue Watching → play → Back goes to **Home** on
  Disney+, and a reviewer complains (What Hi-Fi). Back from a Prime Video title goes to the
  **search**, though the user wanted Home (Samsung forum). Users of a third-party YouTube client
  filed Back-to-Home from a search result as a **bug** (SmartTube #5778).
- **A Jellyfin maintainer called redirecting Back to a parent "hijacking the back button":** "most
  people would expect the back button to go to the previous state of their history, not redirect
  them by force to a state they haven't been to before" (jellyfin-web #102) [1P].
- **Where they offer the hierarchy, it is a separate named control:** Plex's *Show* and *Season*,
  Apple's *Go To Show*, Jellyfin's series and season links. Netflix is removing the player's
  on-screen Back on TV, "as most users already rely exclusively on their remote's back button"
  (What's on Netflix, 2026-07-29) [J].

Where Back departs from strict history, it does so in five ways. Each is built from history
rather than replacing it:

1. **Binge and autoplay play inside one player screen**, so no entry is added per episode (Jellyfin
   source; Netflix, Plex) [1P/I].
2. **A deep link gets a synthetic parent:** Android's TaskStackBuilder, and Google TV's rule that
   Back from a deep-linked details page goes to the app's home [1P].
3. **Overlays close before Back navigates:** dialogs, the player's overlay, full screen [1P].
4. **On TV, focus moves before navigation:** Back from a home row goes to the top menu (Netflix),
   and Back at the root asks before exiting (Samsung, LG) [1P/J].
5. **Up skips intermediate pages and never pushes** (the 2012 Android design guide, NavUtils) [1P].

The one guideline that leans toward hierarchical Back is a sentence in Apple's tvOS HIG. Its own
table on the same page says "Returns to previous screen" [1P].

**Is it what users expect?** Yes, history:

- NN/g: users have "a strong mental model" that Back is "an undo". Some early mobile apps sent
  Back up the hierarchy; that "violated users' expectations", and "now it is rare to find that
  implementation" [1P].
- Back works because people "click until the desired page is recognized" (Greenberg & Cockburn,
  1999). That only works if Back revisits pages they saw.
- Baymard watched an in-page Back link that *added* history disorient a shopper. The site's Back
  and the phone's Back then disagreed (§4.1) [1P].

The owner's instinct is right about Up adding history. The answer is to make Up stop adding it,
not to make Back go up.

### 1.1 The four models in one table

The models are defined in §3. The rows are the scenarios that decide between them.

| Scenario (§3) | (a) history, Up pushes (the Up decision as written) | (b) Up pops when the parent is directly behind, else replaces | (c) Back = Up | (d) Up returns to the nearest parent behind, else replaces; seasons are views |
| --- | --- | --- | --- | --- |
| 3.3 Up ×3 from the player, then Back | Back walks back down | Back lands on the episode page | no history at all; the search, the row and Home are lost | Back → the page before the library |
| 3.4 Continue Watching → Play → Back | Home | Home | the season (the browser's Back disagrees) | Home |
| 3.5 Search → series → Play → Up → Back | Back → Play again | Back → the series' season list | Up and Back lose the search | Back → the search results |
| 3.7 Binge S1E10 → S2E1, then Up, Back | Back → playing S2E1 again | Back → the same series, at season 1 | season 2 | Up → the series at season 2; Back → Home |
| 3.17 Extra → Up → Back | Back → playing the extra | Back → the extra's page | the film; Back → its library | Back → the page before the film |
| 3.9 Shared link to Play, desktop | as (b) | Up → the parent in place; the browser's Back leaves | the on-screen Back and the browser's disagree | as (b) |

---

## 2. What Orpheum does today (2026-10-27)

- **`navigation.ts`:**
  - `go` pushes, and `replace` replaces.
  - `back(up)` goes back in the history when this app wrote the entry before (`appDepth` in
    `history.state`). Otherwise it replaces the current entry with `up`.
  - On a TV, a page opened with nothing before it (a reload, a bookmark) gets a **base** under it:
    a second entry, the same address marked `isBase`. The remote's Back (Silk's) then reaches the
    app, which goes up from the base (`layBase`, `upFromBase`).
- **`back-button.tsx`:** a drill-down page's on-screen Back calls `navigation.back(up)`.
  `input-layer.tsx` sends a remote's, a keyboard's or a controller's Back to the same place.
- **`play-up.tsx`:** the Play page's up:
  - an episode → its series at its season (`episodeUp`);
  - a film or an extra → the film's page;
  - an other video → its library;
  - else Home.

  On a TV the Play page shows no button. The remote's Back goes there.
- **`up-next.tsx:143`:** Up next **replaces** the Play entry with the next episode's.
- **`series.tsx`:**
  - The season list and each season are separate routes (`series-page` with or without `season`).
    A season is a pushed page, and *All seasons* is a link back to the list.
  - An episode in a season links to the episode's own page (`title`).
  - **Choosing an episode ordering pushes the series page again** (`series.tsx:204`,
    `navigation.go`). Back then shows the same series a second time. Under any model here, that
    should be a replace (§3.23).
- **`title.tsx:126`, `:177`:** an episode's page links to its series and its season. These are
  ordinary pushing links, and they go to the page's ancestors.
- **`modal-dialog.tsx`:**
  - Dialogs are the browser's own `<dialog>`, opened with `showModal()`.
  - `cancel` (Escape, and Android's Back since Chrome 119/120 — §4.5) closes them, and so does the
    input layer's Back command.
  - No history entry is added while a dialog is open.

---

## 3. Scenarios

**Pages:**

| Abbreviation | Page |
| --- | --- |
| `H` | Home |
| `Ls` | the Libraries page; what it shows (library, view, folder, sort, search) is in its address (D349 §2) |
| `L` | a library's list on it |
| `L/A`, `L/A/B` | an other library's folders |
| `S@all`, `S@2` | a series' page showing its season list, or season 2 |
| `E3` | an episode's own page |
| `P` | the Play page (`P:S2E3`) |
| `F` | a film's page |
| `X` | an extra's page |
| `WT` | Watch Together's page |
| `R` | the cast remote page (D364 §8) |
| `M` | an admin's library page |
| `out` | whatever is before the app: another site, a new tab, the TV's launcher |

**History** is written oldest first, with the current entry in brackets: `H S@2 [P]`. Entries
ahead of the current one, which only Forward reaches, follow a `|`.

**Models:**

- **(a) Pure history; Up is a link (the Up decision as written, §4).**
  - Every move pushes, Up included.
  - Up next replaces (as built).
  - The on-screen or remote Back with nothing of the app behind goes up in place (§5, as built).
- **(b) The lead's proposal.**
  - Up calls `history.back()` when the entry directly behind is the parent.
  - Otherwise Up replaces the current entry with the parent.
  - Back is history.
- **(c) Back = Up, hierarchical Back.**
  - The page's Back and the remote's Back always go to the parent, in place.
  - History is ignored.
  - A desktop's or a phone's own Back still goes through the history. The page cannot change it.
- **(d) Android's Up, adapted (the recommendation, §6).**
  - Up goes back to the **nearest** entry behind that shows the parent page, if there is one
    (`history.go(-n)`). Otherwise it replaces the current entry with the parent.
  - A page's identity leaves out its views. The series is one page whatever season it shows; the
    library list is one page whatever its sort or search; a folder *is* part of the identity.
    Choosing a season therefore replaces the entry.
  - The parent, reached by Up or returned to after playback moved the person's place, shows that
    place.
  - Back is history.

Under (d) the series' Up is its library: the season is not a level. Where (d)'s history differs
only because a season choice replaced the entry, the row says so.

### 3.1 Drill down, then Back

Path: H → Ls → L → S@all → S@2 → E3 → Play.

- (a), (b): `H Ls L S@all S@2 E3 [P]`.
- (c): no history kept.
- (d): `H Ls L S@2 E3 [P]`. S@all was replaced when season 2 was chosen.

| Press | (a), (b) | (c) | (d) |
| --- | --- | --- | --- |
| Back | E3 | S@2 (skips the episode page the person came from) | E3 |
| Back | S@2 | S@all | S@2 |
| Back | S@all | L | L |

**Verdict:** all history models retrace the way the person came. (c) loses the episode page.

### 3.2 Drill down, Up from the player, then Back

Start as §3.1.

| Press | (a) | (b) | (c) | (d) |
| --- | --- | --- | --- | --- |
| Up | S@2 — `… S@2 E3 P [S@2]` | S@2 — `… S@2 E3 [S@2]` (E3 behind is not the parent, so replace) | S@2 | S@2 — `H Ls L [S@2] \| E3 P` |
| Back | **P: plays again** (down) | **E3** (down) | S@all | L |

### 3.3 Up chain, then Back (the owner's case)

Start as §3.1. The person presses Up three times: Play → season → series → library.

| Press | (a) | (b) | (c) | (d) |
| --- | --- | --- | --- | --- |
| Up | S@2, pushed | S@2, replacing P | S@2 | S@2 (`go(-2)`) |
| Up | S@all, pushed | S@all, replacing (E3 behind) | S@all | L (`go(-1)`; the season is a view) |
| Up | L, pushed | L, replacing (E3 behind) | L | Ls (`go(-1)`) |
| History | `H Ls L S@all S@2 E3 P S@2 S@all [L]` | `H Ls L S@all S@2 E3 [L]` | none | `H [Ls] \| L S@2 E3 P` |
| Back | **S@all, then S@2, then P**: walks back down | **E3**: down to an episode | Ls, then leaves | **H** |

**Verdict:**

- (b) fixes the owner's case only when every parent is *directly* behind. A drill through the
  episode page puts a non-parent behind the player, and from then on every Up replaces. The
  history keeps the episode page, and Back lands on it.
- (d) goes back to the parent wherever it is, so the entries Up passed over leave the back
  history.

### 3.4 Continue Watching → play → Back, or Up

H → P:S2E3 from a Home row. History: `H [P]`.

| Press | (a) | (b) | (c) | (d) |
| --- | --- | --- | --- | --- |
| Back | H | H | S@2 (the browser's own Back says H) | H |
| *or* Up | S@2 — `H P [S@2]` | S@2 — `H [S@2]` | S@2 | S@2 — `H [S@2]` |
| then Back | **P** (down) | H | S@all | H |
| then Up, Up | S@all, L (pushed) | S@all, L (replacing) | — | L (replacing) |
| then Back | L's chain back down | H | — | H |

**Verdict:** Disney+ sends this Back to Home, and the complaint about it asks for "a small icon"
to the episode list, not a different Back (What Hi-Fi, 2023) [J]. Up is that icon.

### 3.5 Search → result → play → Back

H → Ls with a search for "breaking" (in the address) → the series' page S@all → its *Play next* →
P:S2E3. History: `H Ls?q S@all [P]`.

| Press | (a) | (b) | (c) | (d) |
| --- | --- | --- | --- | --- |
| Back | S@all | S@all | S@2 | S@2: S@all's entry, now showing the person's place, season 2 (§6 R3) |
| Back | Ls?q (results as left) | Ls?q | S@all, then L at its start: **the search is gone** | Ls?q |
| *or* Up from P | S@2 pushed; Back → P | S@2 replacing (S@all behind is a different route); Back → **S@all**, the same series again | S@2 | the series page behind (`go(-1)`), shown at season 2; Back → **Ls?q** |

### 3.6 Binge within a season

H → L → S@1 → E1 → P:S1E1. Up next then replaces it seven times, to P:S1E8. History:
`H L S@1 E1 [P:S1E8]`.

| Press | (a) | (b) | (c) | (d) |
| --- | --- | --- | --- | --- |
| Back | E1's page (episode 1, watched hours ago) | E1 | S@1 | E1 |
| *or* Up | S@1 pushed; Back → P | S@1 replacing (E1 behind); Back → **E1** | S@1 | S@1 (`go(-2)`), **E8 focused**; Back → L |

**Note:**

- Every history model returns Back to the page playback started from. That is "the way you came".
- Netflix and Plex start an episode from the series' page, so for them that page *is* the series.
  Orpheum starts it from the episode's own page.
- So Back after a long binge lands on episode 1's page. That page should say where the person is
  now ("Next: S1E9"), and Up is the way to the season.
- Played from the series page's *Play next*, the history is `H L S@1 [P:S1E8]`. Back is then the
  season, which should focus E8 (§6 R3).

### 3.7 Binge across seasons

H → S@1 from a Home row → *Play next* (S1E10) → P. Up next replaces it with P:S2E1. History:
`H S@1 [P:S2E1]`.

| Press | (a) | (b) | (c) | (d) |
| --- | --- | --- | --- | --- |
| Back | S@1: **season 1, stale** | S@1, stale | S@2 | the series page, **at season 2**, S2E1 focused: the page follows the place (§6 R3) |
| *or* Up | S@2 pushed — `H S@1 P [S@2]`; Back → **P:S2E1 plays again** | S@2 replacing (S@1 is a different route) — `H S@1 [S@2]`; Back → **the same series at season 1** | S@2 | the series page behind (`go(-1)`), shown at S2 — `H [S@2]`; Back → **H** |

**Verdict:** this is the lead's stale-season case. Whichever press got the person there, it is
solved only when the series page is one page whose season is a view (§6 R2, R3).

### 3.8 Home row → series → another season → play → Back ×3

H → S@all → S@3 → E2 → P.

| Press | (a), (b): `H S@all S@3 E2 [P]` | (c) | (d): `H S@3 E2 [P]` |
| --- | --- | --- | --- |
| Back | E2 | S@3 | E2 |
| Back | S@3 | S@all | S@3 |
| Back | S@all (a step inside one series) | L (Home never comes) | **H** |

### 3.9 A shared link, a bookmark or a notification, on a desktop or a phone

The Play page is opened in a new tab or from another site. History: `out [P:S2E3]`. A reload
anywhere keeps the history and its state (`history.state`) in every model.

| Press | (a) | (b) | (c) | (d) |
| --- | --- | --- | --- | --- |
| The browser's Back | out | out | out (the page cannot change it) | out |
| The on-screen control | Back → S@2 in place (§5 as built); Up → S@2 pushed, so the browser's Back → P | Up → S@2 in place — `out [S@2]` | Back → S@2, S@all, L, Ls; the browser's Back still leaves: **two Backs, two answers** | as (b); further Ups replace: `out [L]` |

**Note:**

- This is Android's synthetic back stack, built lazily: Up offers the parent without inserting
  entries the person never visited.
- Inserting them on a desktop would be the pattern that Chrome skips (entries added without a
  user activation, §4.4) and that Google now names spam (§4.4).
- A series page opened from a link (`out [S@2]`) behaves the same way: Up gives L, then Ls, in
  place. The browser's Back leaves.

### 3.10 The same on a TV (Silk), opened at a Play page by a bookmark or a reload

Today: `base [P]`. The base is the same address, marked.

| Press | Today, and (a) | (b), (d) | (c) |
| --- | --- | --- | --- |
| Remote Back | the base; the app goes up from it, in place → S@2 | same | S@2 |
| Remote Back | leaves Silk | leaves Silk | S@all, L, Ls, then leaves |
| *or* Up first | S@2 pushed: `base P [S@2]` | S@2 replacing: `base [S@2]`. The base still holds **P's address**, so the next Back reaches it, and the app goes up from P's up, **S@2, again** (a Back that changes nothing) [I] | — |

**Recommendation (judgement):**

- Lay the base as **Home**, as Google TV's rule has it: "If the user goes directly to a details
  page … and then presses the back button, they are taken to the home page of the Moviestar app"
  [1P].
- Back then goes P → H → out. Up is the way to the parent.

**Device check owed:**

- Silk is Chromium. Chromium marks entries a page adds *without a user activation* as skipped by
  the browser's own Back (§4.4).
- `layBase` pushes at load, before any press. A Back pressed before any other key could skip the
  base and leave Silk.
- The owner's Fire TV test on 2026-10-25 may have pressed a key first. A test should reload on
  the player and press Back at once.

### 3.11 Watch Together: a guest joins by a link

The link is opened from a chat app or a new tab: `out [WT]`. The host starts a film, which plays
on the guest's WT page.

| Press | (a), (b), (d) | (c) |
| --- | --- | --- |
| Back (the browser's, the phone's gesture) | out: **the guest leaves the session** | same: the page cannot change it |
| Remote Back on a TV (with today's base) | the base, then up: WT has no parent, so Home | Home |
| Up | none: WT is reached from anywhere | — |

**Note:**

- Android's 2012 guide: a screen reached "from multiple entry points … Up should … return to the
  referring screen, behaving identically to Back" [1P]. So WT, R and the account pages show no
  Up.
- The real risk is an accidental Back ending a session. Casting keeps its session in the tray
  across pages (D364 §7–§8).
- **Owner question:** should leaving WT's page keep the guest in the session, with the tray icon
  leading back?

### 3.12 Watch Together: the host starts a film

H → F → *Watch Together* → WT, where the film plays. History: `H F [WT]`.

- Back → F in every model except (c), which goes to WT's "parent": there is none, so Home.
- Whether the session survives the Back is the question in §3.11.

### 3.13 Casting: the remote page lives while the person browses elsewhere

H → F → Play, cast to the TV, which shows R. The person goes Home → S@2 → the tray icon → R.
History: `H F R H S@2 [R]`.

| Press | (a), (b), (d) | (c) |
| --- | --- | --- |
| Back | S@2, as D364 §8 says ("Back from the remote returns to where the person was") | none: R has no parent. Home breaks D364 §8 |
| Back again | H, then R (the earlier visit), then F | — |

**Note:**

- Repeated tray visits pile up R entries in the history. Jellyfin's Android TV app piles up
  watched episodes the same way, and users complained (#2053) [U].
- An option: the tray's link goes *back* to R when R is the entry directly behind. That is the
  same rule as Up's, applied to a page reached from anywhere. Minor; judgement.

### 3.14 TV remote in the player: overlay, then Back

Silk: `H S@2 E3 [P]`, full screen, with the overlay showing.

| Press | (a), (b), (d) | (c) |
| --- | --- | --- |
| Back | closes the overlay (as built) | same |
| Back | E3 | S@2 |

**TV Bro:**

- The remote's Back never reaches the page, in either input mode.
- In order, it turns the cursor back on, closes TV Bro's menus, exits full screen, or toggles
  TV Bro's menu bar, whose back icon is `history.back()`
  ([controller-input-2026-10-25.md](controller-input-2026-10-25.md) §2) [1P, source].
- So in TV Bro the player's overlay needs **Up** and, until an Android shell exists, an on-screen
  **Back**. Today `BackToTheList` shows no button on any TV, so on TV Bro the person's only way
  out of the player is TV Bro's menu bar [I].

### 3.15 The other library's folders: drill in, play, Back and Up

H → L → L/A → L/A/B → P:v. A folder is a level, so each step pushes in every model.

| Press | (a) | (b) | (c) | (d) |
| --- | --- | --- | --- | --- |
| Back ×3 | L/A/B, L/A, L | same | same (history and hierarchy agree) | same |
| *or* Up ×2 | L/A/B, L/A pushed; Back walks down | `back()` twice (each parent directly behind) | L/A/B, L/A | `go(-1)` twice |
| Reached by search instead: `H Ls?q [P:v]`, Up ×2, then Back | pushed; Back → L/A/B | replaced: `H Ls?q [L/A]`; Back → **Ls?q** | the search is lost | as (b) |

### 3.16 The Libraries page's own state (D349 §2: "Back restores it")

H → Ls (Films, sorted by year, search "alien") → F (Wings) → P. History: `H Ls{films,year,alien} F [P]`.

| Press | (a), (b), (d) | (c) |
| --- | --- | --- |
| Back, Back | F, then the list as left: sort, search, focus (the focus memory on the entry) | F, then **Films at its start**: the sort and the search are gone |
| Up, Up (b), (d) | F (`back()`), then the list as left (`back()`) | — |
| Up, Up (a) | F pushed, Films pushed **at its start**; Back walks down | — |

**Note:**

- (d) needs a page identity that counts the library chosen and the folder, but not the view,
  sort or search.
- Up then lands on "the list as the person left it" whenever that list is behind, which is D349
  §2's promise. When it is not behind, Up opens the list at its start, as the top bar's link does.

### 3.17 Film → extra → play the extra → Up

H → F → X → P:X. Under the Up decision §2, the extra's Up is the film's page.

| Press | (a) | (b) | (c) | (d) |
| --- | --- | --- | --- | --- |
| Up | F pushed | F replacing (X behind is not F) | F | F (`go(-2)`) — `H [F] \| X P` |
| Back | **P:X** (plays again) | **X** | L | **H** |

### 3.18 The same film in two libraries (D363; the Up decision §3)

H → Ls (Kids) → F → P.

- Up ×2 under (b) and (d) goes back to F, then back to **Kids**, because Kids is the entry behind.
- The Up decision's §3, "the library the person came through", is **what the history already
  holds**. (d) needs no record of its own for it.
- From Continue Watching (`H [P]`), Up → F replacing, then Up → the first of the person's
  libraries that lists it, by name (§3's rule), replacing.

### 3.19 Admin: a library's page → its folder dialog → Back

Admin → libraries → M. The folder picker (D359) opens as a modal: `… [M]`, dialog open, no entry.

| Back by | All four models today |
| --- | --- |
| Escape; a TV's or a controller's Back that reaches the page | closes the dialog (`modal-dialog.tsx`) |
| Android Chrome's Back gesture | closes it: a modal `<dialog>` receives Android's close request (Chrome 119/120, after a user activation) [1P] |
| A desktop browser's Back, iPhone's swipe, Silk's remote | **leaves M**. The dialog goes with what was picked in it |

**Recommendation (judgement):**

- A page-like modal (the folder picker, a full-screen menu) takes a history entry while open.
  Every Back then closes it, and closing it by its own button pops the entry. Jellyfin's
  `dialogHelper.js` does exactly this (`:100-125`) [1P].
- Baymard: "users should always be exited from an overlay when clicking 'Back'"; 37% of sites
  fail this [1P].

### 3.20 A small dialog open in the player (the track menu) when Back is pressed

The same as §3.19:

- On Android, the close request closes it.
- On a desktop or iPhone, Back leaves the player.
- On a TV, the input layer closes it if the key reaches the page.

The same rule applies (judgement): a menu that covers the player takes an entry.

### 3.21 The first page of the app

The app is opened at Home by its address, the installed app's icon or the TV's launcher:
`out [H]`.

- **Back leaves:** to the previous site or the new-tab page, closing an Android app, or Silk's
  own handling.
- This is the same in every model.
- Platforms want Back from an app's home to reach the launcher: Android TV's quality checklist
  TV-DB, and Fire TV [1P]. Samsung and LG want packaged apps to confirm before exiting [1P].
- A browser page cannot ask, nor should it. Home shows no Up.

### 3.22 iPhone, installed app (standalone)

- There is no browser bar. **The edge swipe is Back:** "Progressive Web Apps now have the
  navigation gestures enabled, similar to Safari. It works for every URL within your scope,
  including client-side routes and it's mostly useful for the back action as there is no back
  button on the screen while in standalone mode" (Firtman on iOS 12.2) [1P/J].
- With nothing behind, the swipe does nothing.
- So the brief's "no swipe back in standalone" is out of date. A 2018 Discourse thread predates
  iOS 12.2 [U].
- Under (d): Continue Watching → P, swipe → H. Search → S → P, swipe twice → the results.
- Up is on screen. (c) would put a hierarchical Back on screen beside a swipe that is history.
- **Device check owed:** that the swipe works in Orpheum's installed app, and that the player's
  full screen does not swallow it.

### 3.23 Choosing an episode ordering on a series page (found reading the code)

- `series.tsx:204` calls `navigation.go` after the ordering is saved, so the same series page is
  pushed a second time.
- Back then shows the same series, its season numbers now meaning the new ordering's groups.
- Under (d), and under Android's rule that "changing view options … does not change the behavior
  of Up or Back" [1P], this is a **replace**. It is a small change; filed here, not made.

---

## 4. Research

### 4.1 Usability research on Back

- **Back is heavily used, and used less than it was.** Weinreich, Obendorf, Herder and Mayer,
  "Not Quite the Average" (ACM TWEB, 2008), a long-term client-side study:
  - "the share of back button actions has dropped from over 30% in the mid-nineties to less than
    15% in our study".
  - In 1995, Catledge and Pitkow found Back at 41% of navigation actions, behind links at 52%.
  - One reason given: "most Web sites nowadays offer structural links on every page that allow
    returning to the home page or landmark pages of the site, without using the back button"
    [1P, study].
  - **Takeaway [I]:** people climb a hierarchy with links on the page, and keep Back for
    retracing. That is the split recommended here.
- **NN/g, "Top Ten Web Design Mistakes of 1999":** Back is "the lifeline of the web user and the
  second-most-used navigation feature" [1P]. NN/g later reports it fell to third, behind links and
  on-page buttons [S].
- **NN/g, "Mental Models":**
  - "Back is supposed to lead back to the previous screen, essentially acting as an undo."
  - "in some mobile apps, Back took users up in the hierarchy … This behavior violated users'
    expectations. Over the years, designers have luckily understood that, and now it is rare to
    find that implementation for Back in mobile apps" [1P].
  - **This is the most direct evidence against Back = Up.**
- **Greenberg and Cockburn, "Getting Back to Back" (1999):** Back works because it gives "rapid
  return to very recently visited pages", people "can use it even with a naive model of how it
  works", and it supports a "'click until the desired page is recognized' strategy" [1P, paper,
  via search summary]. Recognition needs pages the person saw. A synthesized parent they never
  visited is not recognized as "back".
- **Baymard, "4 Design Patterns That Violate 'Back' Button UX Expectations":**
  - "users expect the 'Back' button to take them back to what they perceived to be their previous
    page".
  - Overlays must close on Back (37% of sites fail), and filter and sort steps should be undoable
    by Back (27% fail).
  - The Walmart case: product list → product page → reviews. The site's in-page "Back" link went
    to the product page by adding an entry. The phone's swipe then went to the reviews, not the
    list: "Confused, she tapped the in-page 'Back' link again … Now she has forgotten her original
    intention" [1P].
  - **That is the owner's objection to Up adding history, observed in testing.**
- **Smashing Magazine, "Designing A Better Back Button UX" (Friedman, 2022):**
  - Users "rely extensively on the browser's 'Back' button. Yet users often seem to be thinking
    twice before actually hitting that button."
  - Users "trust custom buttons more". A custom control's behaviour must be obvious from its
    label [J].
  - **Judgement:** a labelled "‹ Breaking Bad" is obvious. An unlabelled "Back" that sometimes
    goes up is not.
- **NN/g, "Breadcrumbs":** a trail "should display the current location in the site's
  hierarchical structure, not the session history" [1P]. Hierarchy belongs in a control of its
  own, never in Back.
- **Filters and sorts, web against app:**
  - Baymard finds shoppers expect Back to undo each filter step on a product list.
  - Android's 2012 guide says sorting and filtering create no history [1P].
  - Orpheum's Libraries page today replaces on these (D349 §2's "Back restores it" is about
    returning to the page, not stepping through its sorts) [I].
  - **For a season, judgement:** a season choice is closer to a tab than a filtered product
    list, so replace it. The owner may prefer otherwise.

### 4.2 Android: Back and Up, then and now

- **2012 design guide, "Navigation with Back and Up"** [1P]:
  - "The Up button is used to navigate within an app based on the hierarchical relationships
    between screens." Back goes "in reverse chronological order".
  - "When the previously viewed screen is also the hierarchical parent of the current screen,
    pressing the Back button has the same result as pressing an Up button — this is a common
    occurrence."
  - For related detail views, "following each link does create history … Up should continue to
    bypass these related screens and navigate to the most recently viewed container screen".
  - "Up can return to a container … which the user hasn't previously navigated through."
  - "Changing view options for a screen does not change the behavior of Up or Back … no new
    navigation history is created": tabs, dropdowns, filtering, sorting.
  - For a deep entry from a widget or notification: "inserting into the task's back stack the
    complete upward navigation path to the app's topmost screen".
- **2013 training, "Providing Up Navigation"** [1P]:
  - `navigateUpFromSameTask()` "finishes the current activity and starts (or resumes) the
    appropriate parent activity. If the target parent activity is in the task's back stack, it is
    brought forward."
  - With `FLAG_ACTIVITY_CLEAR_TOP`, what was above the parent is cleared.
  - **Up never pushed.** It removed the current screen, and went back to the parent where the
    stack had one. That is model (d).
- **Today, "Principles of navigation"** [1P]:
  - "Within your app's task, the Up and Back buttons behave identically." "The Up button never
    exits your app."
  - On a deep link, Up goes "through a simulated back stack and not to the app that triggered
    the deep link".
  - Google merged the two buttons once the back stack was required to "match a back stack that
    could have been achieved by organically navigating through the app". The synthetic stack
    makes history and hierarchy agree at entry, so Up has nothing left to do differently [I].
- **Predictive back** (Android 13+, Android Developers Blog, 2022-07):
  - The system shows "an animated preview of the destination UI" while the person swipes.
  - It moves apps to "an ahead-of-time model for back event handling", deprecating
    `onBackPressed` and `KEYCODE_BACK` interception [1P].
  - Chrome on Android 15 shows the preview for web pages [S].
  - **Takeaway [I]:** the platform is making Back's destination visible *before* it happens, so a
    Back that is not the previous page will look wrong in the preview itself.

### 4.3 TV platforms

The earlier report's §3 has these with quotes. In short:

- **Google TV:** "take them to the previous destination"; never "gated by confirmation screens or
  part of an infinite loop"; "Don't display a back button"; a deep-linked details page → the
  app's home [1P].
- **Fire TV:**
  - "Return to the previous operation or screen (Activity), or cancel the current operation or
    prompt"; and "Make sure the app doesn't enter into an infinite loop of closing and opening the
    menu" (Controller Behavior Guidelines) [1P].
  - The Appstore test: "When pressing the Back button within the app, the UI should revert to the
    previous screen. … should not … enter into an infinite loop" (Test Criteria 4.5) [1P].
- **Samsung:** "From a detail page … the 'Return/Exit' key must display the previous page" [1P].
- **LG:** "If the previous page exists, move back to the previous page." The platform's Back is
  the page's `history` by default [1P].
- **tvOS HIG, "Remotes":**
  - "open the parent of the current screen … the parent is defined by the app hierarchy, and isn't
    necessarily the previous screen"
  - but the same page's table: Back "Returns to previous screen. Exits to Apple TV Home Screen"
  - and "people press and hold the Back button to go to the Home Screen from any location" [1P].

  The only platform text for hierarchical Back. It contradicts itself, and UIKit's navigation
  stack pops what was pushed.
- **Netflix's own TV player** is dropping its on-screen back "as most users already rely
  exclusively on their remote's back button to return to the menu" (What's on Netflix,
  2026-07-29) [J].

### 4.4 The web platform

- **Chrome's history manipulation intervention** [1P]:
  - Chrome makes "the browser's back button skip over pages that added history entries or
    redirected the user without ever getting a user activation".
  - "When a document adds a history entry without having a user activation, the
    should_skip_on_back_forward_ui_ for all same-document history entries will be set to true."
  - A later user activation clears it.
  - It "only impacts the browser back/forward buttons and not the history.back()/forward() APIs".
  - **Consequences for Orpheum [I]:**
    - Up and Up next run on a press, so they are unaffected.
    - Replacing adds no entry.
    - The TV base pushed at load is exactly what the intervention targets (§3.10).
- **Google Search's spam policy on "back button hijacking"** (announced 2026-04, enforced from
  2026-06-15) [1P]:
  - "a script … inserts or replaces deceptive or manipulative pages into a user's browser history
    preventing them from using their back button to immediately get back to the page they came
    from".
  - Orpheum is not a search landing page, so this is the industry's line rather than a rule that
    applies.
  - (d) stays on the right side of it [I]: Back always reaches the page the person came from,
    and Up only removes entries ahead of a press the person made.
- **Close requests** (WICG CloseWatcher; Chrome's intent to ship for `<dialog>` and `popover`)
  [1P]:
  - Android's Back closes a modal `<dialog>`.
  - Anti-abuse rules apply: the dialog must have opened on a user activation, and two Backs with
    no activation between them always get through.
  - Firefox has a bug open to enable it (1966073) [1P].
- **The Navigation API** is Baseline since January 2026 [S, MDN]:
  - `navigation.entries()` lists this origin's contiguous entries with their URLs.
    `traverseTo(key)` jumps to one. `navigate(url, {history: "replace"})` replaces.
    `canGoBack` tells whether there is anywhere to go.
  - That is everything (d) needs: find the nearest parent entry, then traverse or replace.
  - TV browsers (an old Android WebView in TV Bro, Silk's Chromium) may lack it [I].
  - The same can be done on `history` alone: each entry's `state` already keeps `appDepth`, and
    can keep the list of page identities beneath it, written at each push.
- **iOS standalone:** the edge swipe is Back for client-side routes since iOS 12.2 (§3.22) [1P/J].
- **SPA pitfalls:** an app cannot tell Back from Forward in `popstate`. It should keep its own
  stack in `history.state` and not trust `history.length`, which counts other sites' entries
  (the earlier report §3.5; Jellyfin #102).

---

## 5. Competitors

Behaviours already sourced in the earlier report are summarised with its section. New sources
are cited here. Nothing was run on a device: "unverified" means no source was found.

| Product | Back from the player | Back from a title reached by a row, search, or Continue Watching | On-screen back or up | Hierarchy | Binge and history | Complaints |
| --- | --- | --- | --- | --- | --- | --- |
| **Netflix, TV** | to the menu or previous screen (WoN 2026: "return to the menu") [J] | previous screen (unverified); Back on rows jumps to the top menu (Engadget 2025) [J] | removing the player's on-screen back (2026 test) [J] | seasons a selector on the title page (unverified) | inside the player [I] | none found |
| **Netflix, web** | the player's top-left arrow, "Back to Browse" [S] | details are a modal over the list (`/browse?jbv=<id>`, `/search?q=…&jbv=`): closing returns to the list [S] | the arrow | season dropdown in the modal (unverified) | unverified | a 2019 Firefox bug where the arrow could not be clicked [U] |
| **Disney+, TV** | Continue Watching → play → Back → **Home** | — | none in playback | season tabs [S] | — | What Hi-Fi 2023: wants "a small icon" to the episode list [J] |
| **Prime Video, TV** | — | Back → **the search**; the user wanted Home | — | — | — | Samsung forum [S] |
| **Apple TV app** | Back first shows info and *Go To Show*, then leaves | — | *Go To Show* | the show page lists the seasons | Up Next | Apple Community threads asking how to reach the episodes [U] |
| **YouTube, TV** (SmartTube, a third-party client) | — | search → result → Back → Home **filed as a bug**; "Pressing the back button always returned to the search term list" before | — | — | — | #5778 [U] |
| **YouTube, web** | history | history | none (the browser's) | — | autoplay changes the address; whether it pushes is unverified | — |
| **Plex, web** | history; the player can be minimized to a mini-player while browsing [S, support] | "Clicking the Back button will return you to the previous screen" [1P] | header back arrow; Home | *Show* and *Season* buttons (TV, 2026) [U/1P] | inside the player [I] | — |
| **Plex, Fire TV** | Back after binging could not leave the show (2026-06, staff: fixed) | — | — | — | — | forum 940099: "it is not letting me go back beyond the current show" [U] |
| **Jellyfin, web** | `appRouter.back()` = `history.back()`; the player is a route (`#/video`) and the next episode plays inside it (`htmlVideoPlayer/plugin.js:1123`; `playback/video/index.js:562`) [1P] | history | header back arrow (shown when `history.length > 1`) | series and season names are links | no entry per episode [1P] | #102: the back arrow left to Google; maintainer: redirecting Back is "hijacking" [1P] |
| **Jellyfin, dialogs** | a dialog pushes an entry and closing it calls `history.back()` (`dialogHelper.js:100-125`) [1P] | — | — | — | — | — |
| **Jellyfin, Android TV** | history | deep link or launcher row: Back closes the app; users want Home first (#5910) [U] | — | — | playing each episode from its info page piles them up (#2053) [U] | maintainer offered a sidebar, not a different Back |
| **Emby** | history (inferred from the shared lineage) | — | — | — | — | a Back bug, 2024 [U] |
| **Kodi** | Back pops the window's folder history, which *is* the hierarchy when drilling [1P] | a home widget opens with `return`, so Back goes Home [1P] | `..` item | folders | playlist → Back landed at a root never seen (#20134, fixed 2026-09) [U/1P] | — |
| **Infuse** | iOS stack: the back label names the previous page [U/1P] | history | iOS back | — | — | asked for Menu → top (added 8.3.2) [1P]; "Is hitting the back button repeatedly the only way…" [U] |
| **plx-native** (a third-party Plex client for tvOS) | — | a Continue Watching tile opens **the show on its season**, so Back is the show; a Recently Added episode opens its own page | — | seasons are tabs: "A separate 'Go to Season' is not needed" | — | #879 (2026-10-20): "an episode is a dead end for navigation" [1P, project] |

### 5.1 What the evidence says about "reverse drill-out"

1. **Every sourced product's Back is its own history** (or, on the web, the browser's). None was
   found that ignores how the person came. Kodi's folder Back is a history of folders. It agrees
   with the hierarchy while drilling and disagrees after a jump (#20134).
2. **They look like reverse drill-out for three reasons:**
   - Most paths are drill-downs.
   - Binge stays inside one player screen.
   - Shortcuts often land on the parent first: plx-native's Continue Watching opens the show, and
     Netflix's tile offers *Episodes & Info*.
3. **Where history and hierarchy part, products keep history,** and add a named way up: *Go To
   Show*, *Show*/*Season*, series and season links. The complaints in that gap ask for the link
   (Disney+), or for Back to keep retracing (SmartTube, Prime Video).
4. **Nobody pushes for Up in a way that walks back down.** Android's Up removed the current
   screen. iOS pops. Apple's and Plex's *Go to Show* push, but they are links among others, not a
   control called Up. Baymard's Walmart case shows what a pushing "Back" link does to people.
5. **TV apps add one non-history Back,** and it moves focus, not pages: Netflix's rows → top
   menu, and exit confirmations at the root. Orpheum's overlay-first Back is this kind.

So the hypothesis fails on the evidence. Back is not hijacked. It is history, made to agree with
the hierarchy by building the history well, at the entry and during binge.

---

## 6. Recommendation

Judgement throughout. The evidence for each rule is in §3 to §5.

### 6.1 Rules

- **R1. Back is the device's own Back, and it is history.**
  - The browser's button, Alt+Left, a mouse's back button, Android's gesture, iPhone's edge
    swipe, Silk's remote.
  - Before navigating, Back closes what is open over the page, in order:
    1. a dialog or menu (§3.19, which recommends an entry for page-like ones);
    2. the player's overlay;
    3. full screen where the platform gives it to Back.
- **R2. A page's identity is its subject, not its view.**
  - A series' page is one page; its season is a view.
  - The Libraries page's identity is the library and folder chosen; its view, sort and search
    are views.
  - Changing a view **replaces** the entry: choosing a season, choosing an episode ordering
    (§3.23).
  - Moving to another subject pushes: another title, a folder deeper.
  - **Owner question:** fold the season list and the seasons into one series page with a season
    picker, landing on the person's season, as Netflix, Disney+ and plx-native do [S/1P]. R2 can
    also be applied with today's two routes, treating `series-page` with any `season` as one page.
- **R3. A page shows the person's place when they arrive by Up, or return by Back after playback
  moved it.**
  - The series page shows the season of the episode reached, with that episode focused.
  - An episode's page that playback has moved past says what is next.
  - This is what makes Back after a binge across seasons right (§3.7) without touching the
    history.
- **R4. Up goes to the parent and never adds history.**
  - If an entry of this app showing the parent (by R2's identity) is behind, Up goes back to the
    **nearest** one (`history.go(-n)`, or `navigation.traverseTo`). The page then shows the place
    (R3).
  - Otherwise Up **replaces** the current entry with the parent.
  - The parents are the Up decision's §2 list, with the season level folded per R2.
  - A page reached from anywhere (Watch Together, the cast remote, account and admin tops) has no
    Up.
  - Up never leaves the app.
- **R5. A link to an ancestor is an Up.**
  - The episode page's series and season links (`title.tsx:126`, `:177`) follow R4, not push.
    Otherwise they recreate Baymard's Walmart case.
  - A crumb in a trail, if there is ever one, is the same.
- **R6. Up next and autoplay replace the Play entry** (as built, `up-next.tsx:143`).
- **R7. An entry with nothing of the app behind it:**
  - On a desktop or a phone, the device's Back leaves. That is the browser's to give (Jellyfin
    #102). Up offers the parent in place (§3.9).
  - On a TV whose remote's Back is the browser's (Silk), lay the base as **Home**, per Google TV
    (§3.10). Check on the device that Chromium's intervention does not skip it.

### 6.2 Controls

- **One on-screen Up** on every page with a parent:
  - top left, where Back is today;
  - labelled with the parent's name and a chevron: "‹ Breaking Bad", "‹ Season 2", "‹ Films";
  - reachable by the remote like any control;
  - in the player's overlay too.
- **A single labelled Up, not a breadcrumb trail.**
  - The hierarchy is at most four deep.
  - A trail costs a TV remote several presses and a phone its width.
  - NN/g's case for breadcrumbs is deep sites. Here the page's heading already names where it
    sits ("Breaking Bad · S2 E3").
- **No on-screen Back on a desktop, a phone or the installed app:**
  - Every one has a reachable Back.
  - Netflix is removing its own on TV.
  - A second Back on the page that disagrees with the device's is the two-Backs problem (§3.9).
- **On a TV, an on-screen Back only where the remote's Back never reaches the page** (TV Bro).
  - Today `isTelevision` cannot tell TV Bro from Silk by its user agent [I]. Until it can, or
    until the Android shell
    ([controller-input-2026-10-25.md](controller-input-2026-10-25.md) §5.3) exists, keep it on
    every TV.
  - Label it "Back", distinct from Up.
- **Up's absence says "top".** Home, Films, Series, Libraries and the account and admin tops show
  none. The top bar reaches them.

### 6.3 Each scenario under the recommendation

| § | Under (d) |
| --- | --- |
| 3.1 | Back retraces E3, the series, L |
| 3.2 | Up → the series at S2; Back → L |
| 3.3 | Up ×3 → Ls; Back → H |
| 3.4 | Back → H; Up → the series at S2, replacing; Back → H |
| 3.5 | Back → the series at S2 (R3), then the results; Up → the series behind, at S2; Back → the results |
| 3.6 | Back → E1's page, which says what is next (R3); Up → the series with E8 focused; Back → L |
| 3.7 | Back → the series at S2 (R3); Up → the same; then Back → H |
| 3.8 | Back ×3: E2, the series at S3, H |
| 3.9 | the browser's Back leaves; Up → the parent in place |
| 3.10 | the remote's Back → H (the base as Home), then leaves Silk; Up → the parent in place |
| 3.11 | no Up; Back leaves (owner question: keep the session in the tray) |
| 3.12 | Back → F; no Up |
| 3.13 | Back → where the person was (D364 §8); no Up on R |
| 3.14 | Back closes the overlay, then E3; on TV Bro the overlay shows Up and Back |
| 3.15 | Back and Up agree while drilling; after a search, Up replaces and Back returns to the results |
| 3.16 | Back and Up both return to the list as left |
| 3.17 | Up → F (`go(-2)`); Back → H |
| 3.18 | Up → F, then the library came through, from the history |
| 3.19, 3.20 | every Back closes a page-like dialog first (an entry while open) |
| 3.21 | Back leaves; no Up on Home |
| 3.22 | the swipe is Back; Up on screen (device check owed) |
| 3.23 | choosing an ordering replaces |

### 6.4 What changes in the Up decision

| Point | Change |
| --- | --- |
| §1 | Unchanged. Add: no on-screen Back except on a TV whose remote's Back cannot reach the page (§6.2). The on-screen Back's up-fallback becomes Up's replace (R4). |
| §2 | Unchanged targets, except the season: if the owner folds it into the series page (R2), "a season → its series" becomes a view change, and an episode's Up is the series showing its season. Add: pages reached from anywhere have no Up. |
| §3 | Unchanged as the fallback. When the library came through is behind, R4 finds it in the history without a record of its own. |
| §4 | **Replaced** by R4: Up never adds history; it goes back to the nearest parent entry behind, else replaces. |
| §5 | **Subsumed:** the Play page's "Back with nothing behind goes up" becomes Up's replace. On a TV, the base under a directly opened page becomes Home (R7). |
| New | R1's overlays-first order and page-like dialogs taking an entry; R2 views replace; R3 the parent shows the place; R5 links to ancestors are Ups; R6 Up next replaces. |

### 6.5 Owner questions

1. Fold the season list and the seasons into one series page with a season picker (R2)?
2. Should a season choice be undoable by Back (Baymard's filter finding), or replace (Android,
   Netflix)? The recommendation is replace.
3. Should leaving Watch Together's page by Back keep the session going, with the tray leading
   back, as casting does (§3.11)?
4. "‹ Parent name" or a plain "‹ Up" label? Apple prefers the standard chevron; a name reads
   better at ten feet (the earlier report §5.1 item 6).

### 6.6 Device checks owed

Each needs a VQ item when built:

- Silk, reloaded on the player: the remote's Back pressed at once, before any other key, against
  Chromium's intervention (§3.10).
- iPhone, installed app: the edge swipe goes back, including from the player (§3.22).
- TV Bro: Up and the on-screen Back in the player's overlay are reachable in cursor and direct
  modes (§3.14).
- Android Chrome: Back closes an open dialog rather than leaving the page (§3.19).

---

## 7. Not tried

- **No product was run.** Netflix's web and mobile Back from the player, YouTube web's autoplay
  history, and Prime Video's and Disney+'s web players are unverified.
- **Reddit** was not read; earlier research found it unreadable from here.
- **NN/g's newer "Back is third" article** was seen only in a search summary.
- **Greenberg and Cockburn's paper** was read through its abstract and a search summary.
- **The Navigation API** on the owner's TVs' browsers was not checked.
- **(d) was not prototyped.** `go(-n)` across a reload, and focus restoration on an entry reached
  by a multi-step traversal, need a browser test.

## Sources

New in this report (all fetched 2026-10-27; cached as listed above):

- Google Search Central, "Introducing a new spam policy for 'back button hijacking'":
  <https://developers.google.com/search/blog/2026/04/back-button-hijacking>
- Chromium, "History manipulation intervention":
  <https://chromium.googlesource.com/chromium/src/+/main/docs/history_manipulation_intervention.md>
- Android Developers, "Principles of navigation":
  <https://developer.android.com/guide/navigation/principles>
- Android Developers (2013 mirror), "Providing Up Navigation":
  <https://spot.pcc.edu/~mgoodman/developer.android.com/training/implementing-navigation/ancestral.html>
- Android Developers Blog, "Prepare your app to support predictive back gestures" (2022-07):
  <https://android-developers.googleblog.com/2022/07/prepare-your-app-to-support-predictive-back-gestures.html>
- Amazon, "Test Criteria for Amazon Appstore Apps":
  <https://developer.amazon.com/docs/app-testing/test-criteria.html>
- Amazon, "Controller Behavior Guidelines (Fire TV)":
  <https://developer.amazon.com/docs/fire-tv/controller-behavior-guidelines.html>
- NN/g, "The Top Ten Web Design Mistakes of 1999":
  <https://www.nngroup.com/articles/the-top-ten-web-design-mistakes-of-1999/>
- NN/g, "Mental Models and User Experience Design":
  <https://www.nngroup.com/articles/mental-models/>
- NN/g, "Accidental Dismissal of Overlays":
  <https://www.nngroup.com/articles/accidental-overlay-dismissal/>
- Baymard, "4 Design Patterns That Violate 'Back' Button UX Expectations":
  <https://baymard.com/blog/back-button-expectations>
- Smashing Magazine, "Designing A Better Back Button UX":
  <https://www.smashingmagazine.com/2022/08/back-button-ux-design/>
- Weinreich et al., "Not Quite the Average: An Empirical Study of Web Use" (2008):
  <https://vsis-www.informatik.uni-hamburg.de/getDoc.php/publications/315/Weinreich-2008_-_Empirical_Study_of_Web_Use.pdf>
- Greenberg and Cockburn, "Getting Back to Back" (1999):
  <https://www.csse.canterbury.ac.nz/andrew.cockburn/papers/hfweb.pdf>
- Firtman, "What's new on iOS 12.2 for Progressive Web Apps": <https://firt.dev/ios-12.2/>
- Discourse Meta, "Back button in iOS PWA" (2018): <https://meta.discourse.org/t/back-button-in-ios-pwa/93909>
- What's on Netflix, "Netflix Testing Out New TV Player Overhaul" (2026-07-29):
  <https://www.whats-on-netflix.com/news/first-look-netflix-new-tv-video-player-redesign/>
- Mozilla bug 1558087, "Netflix player 'back'-arrow unreliable":
  <https://bugzilla.mozilla.org/show_bug.cgi?id=1558087>
- WICG, CloseWatcher: <https://github.com/WICG/close-watcher>; Chromium intent to ship:
  <https://groups.google.com/a/chromium.org/g/blink-dev/c/jM5au7yYzHM>; Mozilla bug 1966073:
  <https://bugzilla.mozilla.org/show_bug.cgi?id=1966073>
- MDN, Navigation API: <https://developer.mozilla.org/en-US/docs/Web/API/Navigation_API>
- Plex forum 940099: <https://forums.plex.tv/t/940099>; Plex Web App Player:
  <https://support.plex.tv/articles/200392226-plex-web-app-player/>
- plx-native #879: <https://github.com/GLinnik21/plx-native/issues/879>
- jellyfin-web source at `b290955`: <https://github.com/jellyfin/jellyfin-web>
  (`src/components/router/appRouter.js`, `src/components/dialogHelper/dialogHelper.js`,
  `src/plugins/htmlVideoPlayer/plugin.js`, `src/apps/legacy/controllers/playback/video/index.js`)

Carried from [back-navigation-2026-10-24.md](back-navigation-2026-10-24.md):

- Android Design 2012, "Navigation with Back and Up":
  <https://stuff.mit.edu/afs/sipb.mit.edu/project/android/docs/design/patterns/navigation.html>
- Android TV, "Navigation on TV":
  <https://developer.android.com/design/ui/tv/guides/foundations/navigation-on-tv>
- Android TV quality checklist: <https://developer.android.com/docs/quality-guidelines/tv-app-quality>
- Apple HIG, "Remotes": <https://developer.apple.com/design/human-interface-guidelines/remotes>
- Samsung, "Terminating applications":
  <https://developer.samsung.com/smarttv/develop/guides/fundamentals/terminating-applications.html>
- LG, "Back Button": <https://webostv.developer.lge.com/develop/guides/back-button>
- NN/g, "Breadcrumbs": <https://www.nngroup.com/articles/breadcrumbs/>
- What Hi-Fi on Disney+:
  <https://www.whathifi.com/news/disney-is-great-but-i-wish-it-would-fix-these-two-really-irritating-issues>
- Engadget on Netflix's 2025 TV app:
  <https://www.engadget.com/entertainment/streaming/netflix-overhauls-its-tv-app-with-a-fresh-ui-and-responsive-recommendations-121511958.html>
- Samsung community thread on Prime Video:
  <https://eu.community.samsung.com/t5/tv/using-amazon-prime-app-is-difficult-on-samsung-tv-ru7020/td-p/1764781>
- Plex, "Interface overview": <https://support.plex.tv/articles/200484203-interface-overview/>
- jellyfin-web #102: <https://github.com/jellyfin/jellyfin-web/issues/102>
- jellyfin-androidtv #2053, #5910: <https://github.com/jellyfin/jellyfin-androidtv/issues/2053>,
  <https://github.com/jellyfin/jellyfin-androidtv/issues/5910>
- SmartTube #5778: <https://github.com/yuliskov/SmartTube/issues/5778>
- xbmc #20134: <https://github.com/xbmc/xbmc/issues/20134>
- Firecore 59182: <https://community.firecore.com/t/59182>
- Apple Community: <https://discussions.apple.com/thread/256024268>
