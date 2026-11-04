> **ABOUTME:** The UI audit's outside evidence: how many presses each task should take, where
> focus should land on each kind of page and after Back, and how to lay out controls so the
> remote's arrows reach them. Read with the audit's walk of the current pages before re-laying out
> any page or changing `web/apps/web/src/focus-engine.ts`.

# UI audit, research run: depth, focus landing and layout

- **Date:** 2026-11-04.
- **Requested by:** the owner, through the lead, as item 7 of the 11-06 batch. The owner's words:
  "consulting experts, studies, and competitors to see what kind of information should be
  accessible in what number of clicks, where focus should land on pages, how the controls should
  be laid out for best UX. Right now, many of the pages have just evolved over time, with little
  regard to layout, flow, or initial positioning (example: the video info page, and most of the
  'leaf' pages off the menu)."
- **Scope:** outside evidence only. A second agent walks the current pages; this file does not
  describe them, apart from the focus engine's scoring, which the owner's "long way round"
  complaint needed.
- **Cache:** `/yoloai/cache/ui-audit-research/` (Apple focus pages, Android's `FocusFinder`, the
  CSS draft). The jellyfin-web clone is `/yoloai/cache/gitsrc/jellyfin-web` at `f118ce6`
  (2026-11-02).
- **Labels**, as in [competitor-ux-rationale-2026-10-09.md](competitor-ux-rationale-2026-10-09.md):
  - **[1P]** the platform's or company's own words or code;
  - **[J]** a journalist or researcher reporting;
  - **[S]** second-hand: a summary of a source not read here;
  - **[U]** user reports;
  - **[I]** inference, mine.
  - **User / Commercial** marks whether a practice serves the viewer or engagement and ads.

## 1. What earlier research already settles

These are settled and not researched again. The audit should check the pages against them.

| Topic | Settled by | What it says |
| --- | --- | --- |
| Click budgets for top tasks | [ux-principles](ux-principles-2026-10-09.md) U4, D138 §1 | A table of top tasks with targets; click depth and focus latency are the measures. |
| Progressive disclosure | ux-principles U16 | At most two levels deep. |
| Every input works | ux-principles U21 | Pointer, touch, keyboard and remote; focus lands sensibly after every navigation and dialog. |
| Back and Up | [navigation-back-up](navigation-back-up-2026-10-27.md) R1–R7 | Back is history and closes overlays first; a season is a view that replaces the entry; arriving by Back or Up shows the person's place with the episode focused; Up adds no history; a TV base of Home. |
| Back restores place | ux-principles U3, [back-navigation](back-navigation-2026-10-24.md) §4 item 7 | Filter, sort and scroll come back; the player returns with the played item in view; "where Back lands matters as much as where it goes". |
| Focus mechanics | [controller-input](controller-input-2026-10-25.md) §3 | One item is always focused; roving tabindex; focus after Back lives in history state; TalkBack bypasses D-pad handlers, so document order must work too; the CSS spatial-navigation draft and WICG's archive. |
| Visible navigation | [ui-design-practice](ui-design-practice-2026-10-05.md) §2.1 | Visible beats hidden; no consensus on top bar vs left rail; leaving playback lands on the detail page with Resume. |
| Targets and focus visibility | ui-design-practice §2.9–2.10 | 24 px minimum (WCAG 2.5.8), 44 pt on iOS; 2.4.11 and 2.4.13; ring vs highlight. |
| Settings and forms | ui-design-practice §2.13 | Fewer settings; inputs sized to their content (GOV.UK); tables for records. |
| Where complaints cluster | [ui-competitors](ui-competitors.md) §3 | Admin and setup are 20.5% of Plex's votes and about 26% of Jellyfin's; browsing and search 23% of Jellyfin's. |
| Competitor home pages | [competitor-ux-rationale](competitor-ux-rationale-2026-10-09.md) §2.1–2.3 | Netflix: the upper left is seen most, and a stable home; 2025 TV redesign puts details in the focused tile. Prime Video's 2024 top bar. Disney+ 2025. |
| The remote in the player | [playback-ui](playback-ui-2026-10-28.md) §2.3, §6; [starts-chunks-remote](starts-chunks-remote-2026-10-29.md) §3 | Settled; not repeated. |
| Device checks | [plans/tv-device-tests.md](../plans/tv-device-tests.md) H9 | "Press Down: focus lands in the page (Continue Watching…)". |
| Open questions | [plans/m5.md](../plans/m5.md) Q5, Q7–Q9 | Keyboard arrows are in M5; a resume banner, poster size and an artwork tint are open. |

**The gaps this file fills:** the evidence behind the click budgets, split by input; a focus rule
per page kind (earlier work covers Back, not first arrival); the anatomy of detail, series,
settings and leaf pages; why the arrows sometimes skip a control; and phones.

## 2. Depth: how many presses each task should take

### 2.1 What the studies say

- **The three-click rule has no data behind it.** NN/g: it is "an arbitrary rule of thumb that is
  not backed by data"; its earliest print source, Zeldman 2001, gives none (Laubheimer, NN/g,
  2019) [1P]. Porter's UIE test (2003) found that "user dropoff does not increase when the task
  involves more than 3 clicks, nor does satisfaction decrease" [S, via NN/g].
- **Counting clicks misleads; cost does not.** NN/g: clicks differ in cost (a page load vs an
  expanding section), and "three slow clicks can be worse than five fast ones". What it
  recommends instead: labels with strong information scent, wayfinding (breadcrumbs, local
  subnavigation), the most important tasks surfaced from the home page and key pages, and hubs at
  key points of multistep paths [1P].
- **Broad and shallow beats deep, up to a point.**
  - Kiger 1984: 64 items, users preferred 8×2 and were fastest with it; "both preference and
    performance advantage for broad, shallow trees" [S].
  - Larson and Czerwinski 1998 (CHI, Microsoft Research): "increased depth did harm search
    performance on the web", but "a medium condition of depth and breadth outperformed the
    broadest shallow web structure overall" [1P, abstract].
  - The best breadths found cluster at about 8 per level [S].
- **Hick's law** says choice time grows with the logarithm of the number of equally likely
  options. It argues for grouping and for putting the frequent option first, not for fewer levels
  [S, textbook].
- **On a remote every press is a step.** A pointer jumps; a D-pad walks. Microsoft's Xbox
  guidance: crossing the screen from edge to edge "should take no more than six clicks", and
  labels shown beside icons because that "minimizes the number of clicks" [1P]. Android TV:
  "using a minimal number of clicks. Organize your information in a way that requires the fewest
  screens", and "Don't over-complicate by adding unnecessary layers of navigation" [1P]. Fire TV
  users complained when Plex moved actions "from two clicks to around six" [U, in
  ui-competitors].
- **Settings: 10 to 15 items per screen.** Android's settings guidelines: "Place frequently used
  settings at the top of the screen", "Showing more than 10-15 items can be overwhelming", and
  move rarer settings to a separate screen [1P].

**Consequence [I]:** budget two things per task: levels (page loads), counted on every input, and
presses, counted on the remote only. Levels should be few and each well labelled; presses on a
remote should be bounded by layout (aligned rows, the primary action first), not by flattening
everything onto one page.

### 2.2 What media apps do

- **Jellyfin's admin** is a drawer of about 20 pages in four sections (Server: dashboard, general,
  branding, users, libraries with display, metadata and NFO, playback with transcoding, resume,
  streaming and trickplay; Devices; Live TV; Advanced: networking, API keys, backups, logs,
  scheduled tasks) [1P, code]. Every page is one level from the drawer; the cost is breadth and
  vague labels, which is where its complaints sit (ui-competitors §3).
- **Plex** splits server settings into a long left list under the account's settings, and its
  complaints are similar [U, ui-competitors].
- **Netflix's 2025 TV redesign** moved the synopsis, runtime, cast and awards into the expanded
  focused tile, so many decisions need no detail page at all [J: Engadget, Android Authority].
  That serves the viewer (fewer pages) and engagement (callouts like "#1 in TV Shows") alike.

### 2.3 Proposed budgets

Levels are page loads or overlays opened; presses are D-pad presses including OK, from the place
the task starts, with the focus where rule F1–F7 (§3.4) puts it.

| Task | Levels (all inputs) | Pointer / touch | D-pad presses |
| --- | --- | --- | --- |
| Resume what I was watching, from Home | 0 (it is on Home) | 1 | 1–2 (lands on it, OK) |
| Play a title from its detail page | 0 | 1 | 1 (focus is on Play) |
| Next episode of a series, from the series page | 0 | 1 | 1 (focus is on Resume or Play next) |
| Pick a specific episode | 1 (season view, a replace) | 2–3 | at most 6 to reach any visible episode |
| Change audio or subtitles in playback | 1 (the panel) | 2 | 3–4 |
| Find a title by name | 1 (search) | 2 + typing | 2 + typing |
| Change a personal setting | 2 (menu, page) | 2–3 | at most 6 after the page opens |
| An admin task (add a library, a user) | 2–3 (admin, section, form) | 3–4 | at most 6 per page |

Sources: U4's table is the baseline; this adds the D-pad column from Microsoft's six-press width
and Android TV's "fewest screens" [1P], and it caps settings pages at Android's 10–15 items [1P].
Any task over its budget should be reached by layout first, then by a shortcut.

## 3. Where focus lands

### 3.1 Platform guidance

- **tvOS** [1P]:
  - The focus engine alone moves focus by direction; an app states where it would like focus to
    start, in priority order (`preferredFocusEnvironments`).
  - "Avoid changing focus without people's interaction." The exception is a remote, keyboard or
    controller whose focused item disappears: then move focus to a nearby remaining item.
  - "When a group receives focus, its primary item automatically receives focus too… You can make
    an item primary by increasing its priority."
  - Focus moves through groups in reading order: leading to trailing, top to bottom.
  - "In general, use a focus ring for a text or search field, but use a highlight in a list or
    collection."
- **Android TV** [1P]:
  - Back with a top menu scrolls to the top "and activates the focus to the menu"; with a left
    menu, "focus is taken to the active menu item".
  - One item is always focused (in controller-input).
  - No on-screen Back button; show Cancel only when the actions are confirming, destructive or a
    purchase.
- **Fire TV** [1P]: the app "should clearly indicate which on-screen element currently has the
  focus", and "every actionable on-screen element should be reachable with the D-pad". Its Detail
  View puts the actions under the description. It says nothing of initial focus. Its newer OS's
  `TVFocusGuideView` with `autoFocus` "Redirects the focus to the first focusable child on the
  first visit" and "Remembers the last focused child and redirects the focus to it on the
  subsequent visits".
- **WCAG** [1P]:
  - 2.4.3 Focus Order: the order preserves meaning and operability.
  - 2.4.7 Focus Visible; 2.4.11 Focus Not Obscured (Minimum, AA) and 2.4.12 (Enhanced, AAA): a
    sticky header or bottom bar must not cover the focused item.
  - 3.2.1 On Focus: receiving focus does not change context. So a focused tile may expand (Netflix)
    but must not navigate.
- **WAI-ARIA APG, dialogs** [1P]: "When a dialog opens, focus moves to an element inside the
  dialog." Generally the first focusable element; with long content, a static title or first
  paragraph with `tabindex="-1"`; for a step that is hard to undo, the least destructive action.
  On close, focus returns to what opened it.
- **Route changes in a single-page app.** The Gatsby study with Fable Tech Labs (Marcy Sutton,
  2019) tested five assistive-technology users: "Focusing on a heading was found to be the best
  experience"; a wrapper "worked okay"; the top of the app was overwhelming; wide focus outlines
  broke magnification. Its final advice: a skip link that takes focus on a route change, and an
  ARIA live region announcing the page [1P]. GOV.UK's and Deque's guidance agree on moving focus
  to the new content's heading or start [S].

### 3.2 What the apps do

- **Jellyfin** [1P, code]:
  - Autofocus is on only in the TV layout (`autoFocuser.isEnabled()` returns `layoutManager.tv`).
  - On a new page it tries, in order: the element last focused (for rebuilt buttons), `.btnPlay`,
    the first item of `.itemsContainer`, then the first focusable or `[autofocus]` element.
  - On Back (a restored view), outside the mobile layout it refocuses the element saved when the
    view was left.
  - On desktop and mobile it moves no focus on arrival.
- **Netflix, Prime Video, Disney+, Apple TV** [J, plus the existing corpus]: on a title page the
  primary button (Play or Resume) is focused; on a series, Resume or the next episode; on Home,
  the first row's first tile, below the menu. Back from a title restores the tile it was opened
  from. None of these were verified on a device in this run.
- **Kodi and Infuse** follow the same shape: the primary action first, the remembered item on
  return [S].

**User / Commercial [I]:** focusing Play serves the viewer (one press to the commonest task).
Landing on a promoted hero or an autoplaying trailer instead serves engagement; Orpheum has no
reason to do it.

### 3.3 Keyboard and TV vs pointer and touch

- **Remote and keyboard:** something must be focused at all times, so arrival must place focus
  (Android TV, tvOS). Placing it on the commonest action saves a press.
- **Pointer and touch:** no visible focus should appear on arrival (tvOS: don't move focus without
  interaction; Jellyfin turns autofocus off). For screen readers, focus moves to the page's
  heading, or the page is announced, so the reader starts at the new content.
- **[I]** One rule can serve both: on every arrival, move the browser's focus to the target named
  below, but show the ring only when the last input was a key (`:focus-visible` does this). For a
  screen reader the target is the `h1` when the page has no single primary action.

### 3.4 A rule per page kind

| Page kind | On first arrival | On return by Back or Up |
| --- | --- | --- |
| List or grid (Home, a library, search results) | The first item of the content, not the menu. Home: the first Continue Watching tile (H9). | The item that was opened, with filter, sort and scroll restored (U3, R3). |
| Detail (a film) | Play, or Resume when there is progress. | The control last focused, usually Play. |
| Series | Resume or Play the next episode. | The episode that was played or opened, in its season (R3). |
| Player | No focus ring; the controls hidden. The first press shows the controls with play/pause focused. | — |
| Settings or leaf page | The first field or control of the first group; on a page with no control, its heading. | The control last focused. |
| Dialog or panel | Its first control, or the least destructive one for a hard-to-undo step (APG). | On close, the control that opened it. |
| Any page, when the focused item disappears | The nearest remaining item in the same container (tvOS). | — |

Containers remember their last focused child, as Orpheum's `focus-memory.ts` already does and as
Fire TV's `TVFocusGuideView` and tvOS's focus groups do [1P].

## 4. Layout

### 4.1 Detail page anatomy

- **Android TV's Content Details template** [1P]: a horizontal stack of title, metadata, a short
  description, quick actions, then related clusters. Fire TV's Detail View: actions under the
  description [1P].
- **Order of the first screen [I, from the above and from the competitors]:**
  1. Title, then one line of facts (year, runtime, rating, quality, audio).
  2. The actions row: Play or Resume first and leftmost, then the secondary actions (trailer,
     watched, list, more as an overflow).
  3. A short description, clamped, with More to expand it.
  4. Below the first screen: cast, technical detail, related.
- **Secondary information belongs below or behind a disclosure, not in tabs.** Baymard's
  product-page testing: horizontal tabs overlooked by 27% of users vs 8% for vertically collapsed
  sections, and 7% for one long page with a sticky table of contents [J, Baymard's summary;
  method behind its paywall]. Tabs on a TV also cost presses to cross [I].
- **Commercial:** Netflix's awards and "#1" callouts in the tile [J]. Orpheum can show the facts a
  viewer uses (quality, audio, progress) without the ranking.

### 4.2 Series and season pages

- Android TV's Compilation template: details on the left, episodes on the right [1P].
- The common TV shape [J, I]: the series header and Resume on top; seasons as a row or list; the
  episodes of the chosen season as a list below or beside. Changing season is a view change, not
  history (R2).
- Keep the season picker and the episode list on one axis each: seasons along a row, episodes down
  a column (or the reverse), so Up and Down never land in the wrong list.

### 4.3 Settings and leaf pages

- **One column, grouped, frequent first.** Android: frequent settings at the top; groups separated
  by dividers that cluster, not separate single items; 10–15 items per screen at most; a feature's
  main switch at the top; an entity's page carries its header with at most two actions [1P].
  GOV.UK's one-thing-per-page and inputs sized to content are already in ui-design-practice
  §2.13.
- **Instant vs Save.** NN/g: "Toggle switches should take immediate effect" and need no Save;
  "Separate controls that produce instant results from those that require clicking a command
  button" (Kendrick, 2018) [1P]. So a page is either all instant (toggles, selects) or a form
  with one Save at its end, not a mix.
- **Where Save sits [I]:** at the end of the form, in the column, where focus arrives after the last
  field; on a TV the Down arrow from the last field must reach it.
- **Inline vs dialog [I, from APG and Android]:** edit a single value inline; use a dialog for
  creating a record, for a step that is hard to undo (with the least destructive action focused),
  or for a choice that needs its own list.
- **Android TV's Actions template** puts the title on the left and the options on the right [1P];
  a two-pane layout suits hierarchical, task-forward pages, and "Don't direct user attention
  back-and-forth between panels" [1P].

### 4.4 The menu: top bar or side rail on TV

- **No consensus** (ui-design-practice §2.1): Netflix moved from left to top in 2025 [J]; Max moved
  to a left rail; Prime Video uses a top bar.
- **Android TV's drawer** [1P]: 3–7 destinations, "list only five to six primary navigation
  destinations"; logo, profile and search on top, 1–3 actions (settings, help, profile) at the
  bottom; an active indicator; icons with labels.
- **Fire TV's launcher** has the menu on the left, and Left returns to it [1P].
- **What decides it [I]:** with a top bar, Up from the first row reaches the menu and Back scrolls
  to the top and focuses it (Android TV); with a rail, Left from the first column does. Either is
  one press away from content. Pick on how many destinations there are: a top bar holds five or
  six labelled items at 960 px; more needs a rail.

### 4.5 Reading patterns and Fitts

- **F-pattern** (NN/g eye tracking): on text-heavy pages people read the first lines and scan
  down the left edge [S]. Put the label of each group and each row's start on the left.
- **Upper left first:** Netflix found the upper left much more likely to be seen
  (competitor-ux-rationale §2.1) [1P].
- **Fitts's law:** time to a target grows with distance over size; edges and corners are large
  targets for a mouse [S, textbook]. Its D-pad analogue is the press count: distance in focusable
  items, not pixels [I]. A large far button costs a pointer little and a remote much.

### 4.6 Spatial navigation: the platforms' rule

**What Orpheum does now** (`focus-engine.ts`): a candidate must lie wholly past the source's
edge; its score is the distance along plus twice the distance across; **for Left and Right, any
candidate that does not overlap the source vertically is excluded.** The comment explains why: so
Right at the end of a grid's short last row goes nowhere rather than up a row. The cost: a control
beside but not level with another can never be reached by Left or Right, only by going round.
This is the likely cause of the owner's "reachable only the long way round" [I, from the code].

**What the platforms do:**

- **Android `FocusFinder`** (AOSP) [1P, code]:
  - A candidate need only be partly in the direction: for Right, its left edge is past the
    source's left edge and its right edge past the source's right edge.
  - A candidate inside the source's "beam" (overlapping on the cross axis) beats one outside it.
    For Left and Right that is absolute. For Up and Down it holds only if the beam candidate is
    nearer than the far edge of the other, because an out-of-beam item "could be reached by going
    down".
  - Otherwise the nearest wins by `13 × along² + across²`, weights "finely tuned, be sure to run
    all focus tests if you dare tweak it".
  - **Out-of-beam candidates stay eligible.** They lose only to a beam candidate.
- **W3C CSS Spatial Navigation Level 1** (Working Draft, 2019, LG and Rivoal) [1P]:
  - Candidates inside the source come first; otherwise those past its edge.
  - `distance = euclidean + displacement − alignment − sqrt(overlap)`, where displacement weighs
    the across-distance by **30 for Left and Right and 2 for Up and Down**.
  - So rows strongly prefer their own line but still reach off-line items; columns are looser.
  - The weights were chosen experimentally.
- **Jellyfin** [1P, code]: Euclidean distance; the across-distance is zero for overlapping boxes,
  else the difference of midpoints; off-line candidates allowed.
- **tvOS** [1P]: the focus engine searches in the direction; where nothing lies in the path, the
  app adds a `UIFocusGuide`, "an invisible, focusable region that can redirect focus movement to
  other views".
- **Fire TV Vega** [1P]: `TVFocusGuideView` "makes sure that focusable controls can be navigated
  to… even if they are not directly in line with other controls".
- **Android TV's design rule** [1P]: "If a user doesn't have a straight path to get to a control,
  consider relocating it"; "Avoid layouts that contain controls in hard-to-reach places"; place
  controls such as search "in locations that don't overlap with other clickable elements".

**The common rule:** prefer what is in line, by a strong weight, but never return nothing while
something lies in that direction. Where the layout leaves a gap, add a guide or move the control.

**The grid's short last row** is the case Orpheum's exclusion was written for. The platforms
handle it without excluding everything off-line: Fire TV's 1D lists wrap to the start, and the
CSS draft's `grid` function and tvOS's collection views keep Right in the row. A container-level
rule ("Right inside a grid stays in the grid's row") keeps that behaviour while the general rule
falls back [I].

### 4.7 Layout rules that keep the arrows predictable

From Android TV's templates and navigation guide [1P], with my reading [I]:

1. **One axis per job.** Categories on the vertical axis, items within a category on the
   horizontal (Android TV's Browse template). "Avoid complex and nested layout hierarchies."
2. **Align controls in rows and columns.** A control's row-mates share a centre line; a column's
   share a left edge. Nothing floats between two rows.
3. **Primary actions in one row**, under the facts, with nothing else on that row.
4. **No orphans.** A control alone at a page's edge (a small icon in a corner, a link beside a
   heading) either joins a row or gets a guide.
5. **Reading order is focus order.** Leading to trailing, top to bottom (tvOS; WCAG 2.4.3), so
   the D-pad and Tab and the screen reader agree.
6. **Nested controls** (a button inside a tile) move to the tile's menu: Xbox's guidance for
   nested UI is a context flyout [1P].
7. **Leave room for the focused state.** Scaling of 1.025–1.1 (controller-input) must not cover a
   neighbour; keep the focused item inside the 5% safe margins (Android TV, Xbox) [1P].
8. **Test reachability.** Android TV's checklist: "Ensure a user can navigate to all focusable
   elements". [I] A test can walk every focusable element by arrows and fail on any unreached.

## 5. Phones

- **How people hold them.** Hoober (2013, UXmatters): 1,333 observations; 49% one-handed, 36%
  cradled, 15% two-handed [1P]. His later work (2017): people "prefer to view and touch the center
  of the screen"; accurate targets are about 7 mm at the centre and 12 mm at the corners; 75% of
  touches are by one thumb [1P].
- **His guidance:** primary content at the centre; tabs and actions along the top and bottom
  edges; tertiary functions in a menu from a corner [1P].
- **Bottom or top navigation.** NN/g (Budiu, 2015): tab bars sit at the bottom (iOS) or top
  (Android) and stay visible; with four or five main options keep them all visible; more than five
  is hard to fit; hidden menus are less discoverable [1P]. The hamburger halves discoverability and
  Polar and Facebook moved to tab bars (ui-design-practice §2.1).
- **For a web client [I]:** a bottom bar is reachable by the thumb and is what iOS and current
  Android apps use; in a browser it competes with the browser's own bottom toolbar (Safari,
  Chrome's on some devices), so it needs the safe-area inset and a height that leaves the video
  room. A top bar avoids that conflict at a reach cost. Either beats a hamburger.
- **The detail page on a phone [I]:** artwork at the top, title and facts, then Play full width in
  the thumb's reach, then the secondary actions as an icon row, then the description clamped. The
  sticky Play on scroll is a common pattern; it must not cover the focused item (WCAG 2.4.11).

## 6. Proposed rules and questions for the owner

### 6.1 Ten proposed rules

1. **Budget levels on every input and presses on the remote**: levels per §2.3; at most six D-pad
   presses to cross any page. *Microsoft Xbox [1P]; Android TV [1P]; NN/g [1P].*
2. **Prefer broad and well labelled to deep**: about eight items a level, at most 10–15 settings a
   screen, frequent first. *Larson and Czerwinski 1998 [1P]; Kiger 1984 [S]; Android settings
   guidelines [1P].*
3. **On TV and keyboard, arrival focuses the page's primary action** (Play or Resume on detail and
   series pages, the first content item on lists, the first control on settings); on pointer and
   touch no ring shows. *Jellyfin [1P]; tvOS priority [1P]; Fire TV Vega [1P].*
4. **For screen readers, a route change moves focus to the new page's heading or announces it.**
   *Gatsby and Fable Tech Labs study [1P].*
5. **Back and Up restore the last focused control in each container**, and a dialog's close returns
   focus to its opener. *Jellyfin [1P]; Fire TV Vega [1P]; APG [1P]; R3.*
6. **The spatial search prefers in-line candidates but falls back to the nearest off-line one**
   rather than going nowhere; grids keep Right within the row by a container rule. *Android
   FocusFinder [1P]; CSS spatial navigation [1P]; Jellyfin [1P].*
7. **Lay out for straight paths**: one axis per job, aligned rows and columns, primary actions in
   one row, no orphan controls; relocate or guide any control without a straight path. *Android
   TV [1P]; tvOS UIFocusGuide [1P].*
8. **A detail page reads title, facts, actions row (Play first), short description, then the rest
   below or in collapsed sections, never tabs.** *Android TV Content Details [1P]; Fire TV [1P];
   Baymard [J].*
9. **A settings page is one column of groups, and either applies instantly or has one Save at its
   end, never a mix.** *NN/g toggles [1P]; Android settings [1P].*
10. **On a phone, the main destinations sit in a visible bar (bottom or top), not a hamburger; the
    primary action sits in the thumb's reach.** *NN/g [1P]; Hoober [1P].*

### 6.2 Questions only the owner can answer

1. **Top bar or left rail on TV?** The evidence is split (§4.4). How many destinations will the
   menu hold?
2. **Phone navigation: bottom bar or top bar?** A bottom bar is easier to reach but competes with
   the browser's own toolbar.
3. **Should Right at the end of a grid's short last row stay put, wrap to the row's start, or go up
   a row?** Fire TV wraps lists; Orpheum chose staying put on 2026-10-25.
4. **Expanded focused tiles (Netflix 2025)**, so a title's facts show without opening it, or keep
   the detail page as the place for them?
5. **Settings that apply instantly vs forms with Save:** which pages are which? The admin forms
   (libraries, users) look like Save forms; personal preferences look instant.
6. **On a pointer device, should arrival focus anything at all** (it helps keyboard users, and
   costs nothing visible with `:focus-visible`)?

## 7. Not yet covered

- Samsung Tizen and LG webOS focus guidance was not read; both use the same D-pad model and their
  browsers run the CSS spatial-navigation polyfill's ideas [S].
- Competitor title and series pages were not checked on a device in this run; §3.2's account of
  Netflix, Prime Video, Disney+ and Apple TV rests on reporting and the earlier corpus.
- GOV.UK and Deque's route-change pages were not refetched; §3.1 cites them second-hand.

## Sources

- NN/g, Page Laubheimer, "The 3-Click Rule for Navigation Is False", 2019.
  <https://www.nngroup.com/articles/3-click-rule/>
- Joshua Porter, "Testing the Three-Click Rule", UIE, 2003.
  <https://articles.centercentre.com/three_click_rule/>
- Larson and Czerwinski, "Web page design: implications of memory, structure and scent for
  information retrieval", CHI 1998.
  <https://www.microsoft.com/en-us/research/publication/web-page-design-implications-memory-structure-scent-information-retrieval/>
- Kiger, "The depth/breadth trade-off in the design of menu-driven user interfaces", IJMMS 20,
  1984, 201–213. <https://doi.org/10.1016/S0020-7373(84)80018-8>
- Microsoft, "Designing for Xbox and TV" (archived).
  <https://learn.microsoft.com/en-us/windows/apps/design/devices/designing-for-tv>
- Android Developers, "Navigation on TV".
  <https://developer.android.com/design/ui/tv/guides/foundations/navigation-on-tv>
- Android Developers, TV "Layouts".
  <https://developer.android.com/design/ui/tv/guides/styles/layouts>
- Android Developers, TV navigation drawer.
  <https://developer.android.com/design/ui/tv/guides/components/navigation-drawer>
- Android Open Source Project, settings design guidelines.
  <https://source.android.com/docs/core/settings/settings-guidelines>
- AOSP `FocusFinder.java` (android-19).
  <https://android.googlesource.com/platform/frameworks/base/+/refs/heads/kitkat-release/core/java/android/view/FocusFinder.java>
- W3C, CSS Spatial Navigation Level 1, Working Draft, 26 November 2019.
  <https://www.w3.org/TR/css-nav-1/>
- Apple, Human Interface Guidelines, "Focus and selection".
  <https://developer.apple.com/design/human-interface-guidelines/focus-and-selection>
- Apple, "About focus interactions for Apple TV".
  <https://developer.apple.com/documentation/uikit/about-focus-interactions-for-apple-tv>
- Apple, `UIFocusGuide`. <https://developer.apple.com/documentation/uikit/uifocusguide>
- Amazon, Fire TV design and user experience guidelines.
  <https://developer.amazon.com/docs/fire-tv/design-and-user-experience-guidelines.html>
- Amazon, Vega `TVFocusGuideView`.
  <https://developer.amazon.com/docs/vega/2.0/tvfocusguideview.html>
- Jellyfin web, `src/components/focusManager.js`, `autoFocuser.js`, `viewManager`, at `f118ce6`.
  <https://github.com/jellyfin/jellyfin-web>
- W3C, WCAG 2.2: 2.4.3 <https://www.w3.org/WAI/WCAG22/Understanding/focus-order.html>, 2.4.7
  <https://www.w3.org/WAI/WCAG22/Understanding/focus-visible.html>, 2.4.11
  <https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum.html>, 2.4.12
  <https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-enhanced.html>, 3.2.1
  <https://www.w3.org/WAI/WCAG22/Understanding/on-focus.html>
- W3C, WAI-ARIA Authoring Practices, Dialog (Modal) pattern.
  <https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/>
- Marcy Sutton, "What we learned from our accessible client-side routing user research", Gatsby,
  2019. <https://www.gatsbyjs.com/blog/2019-07-11-user-testing-accessible-client-routing/>
- NN/g, Alita Kendrick, "Toggle-Switch Guidelines", 2018.
  <https://www.nngroup.com/articles/toggle-switch-guidelines/>
- NN/g, Raluca Budiu, "Basic Patterns for Mobile Navigation", 2015.
  <https://www.nngroup.com/articles/mobile-navigation-patterns/>
- NN/g, "F-Shaped Pattern For Reading Web Content".
  <https://www.nngroup.com/articles/f-shaped-pattern-reading-web-content/>
- Steven Hoober, "How Do Users Really Hold Mobile Devices?", UXmatters, 2013.
  <https://www.uxmatters.com/mt/archives/2013/02/how-do-users-really-hold-mobile-devices.php>
- Steven Hoober, "Design for Fingers, Touch, and People, Part 1", UXmatters, 2017.
  <https://www.uxmatters.com/mt/archives/2017/03/design-for-fingers-touch-and-people-part-1.php>
- Baymard Institute, "Avoid horizontal tabs on product pages".
  <https://baymard.com/blog/avoid-horizontal-tabs>
- Engadget, "Netflix overhauls its TV app with a fresh UI and responsive recommendations", 2025.
  <https://www.engadget.com/entertainment/streaming/netflix-overhauls-its-tv-app-with-a-fresh-ui-and-responsive-recommendations-121511958.html>
- Android Authority, Netflix TV app redesign. <https://androidauthority.com/netflix-tv-app-redesign-3449668>
