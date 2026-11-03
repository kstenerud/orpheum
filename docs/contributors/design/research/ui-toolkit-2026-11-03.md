# The UI toolkit: structure, input mediums, and build or adopt (2026-11-03)

The owner, after trying the player overlay (D407) on staging: the overlay is "roughly where I want
it, but the UI components we have are showing their clunkiness": none of the overlay's pop-ups needs
a close button, the volume pop-up is far too wide, and the subtitles list should show the current
value under the highlight with at least one entry above and one below. He suspected "a fault in how
our UI widget class hierarchy is set up" and asked for a research round: what GUI experts say about
widget hierarchies, layout and navigation rules per medium (mouse, touch, controller), whether the
mediums should look and behave differently, what we could do better, what competitors do, whether
to adopt an established UI library or build our own, and which wished-for features need significant
UI work. The aim: "a slick and speedy interface that can run on low-powered devices like set-top
boxes, with a low download payload. Security is also a big consideration."

Three read-only research passes fed this (our code and wish list; expert and platform guidance;
competitors and libraries). It builds on, and does not repeat,
[ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md),
[ui-design-practice-2026-10-05.md](ui-design-practice-2026-10-05.md),
[controller-input-2026-10-25.md](controller-input-2026-10-25.md),
[ui-widgets-audit-2026-10-27.md](ui-widgets-audit-2026-10-27.md),
[client-framework.md](client-framework.md),
[tv-apps-react-native-2026-10-24.md](tv-apps-react-native-2026-10-24.md) and
[web-client-efficiency-audit-2026-10-28.md](web-client-efficiency-audit-2026-10-28.md).

## 1. The short answer

- **The suspicion is right, in a precise form.** We have no deep class hierarchy; we have one base,
  `ModalDialog`, that decides chrome (an always-present ✕) and width (`min(26rem, 92vw)`) for
  everything built on it, and a `DropDown` on top that adds a `min-width: 13rem` and CSS that
  reaches into whatever is inside it. A pop-up's size and chrome belong to its *kind* and its
  *content*, not to a shared base. Every expert source we read says the same: compose small
  single-purpose parts; let no base decide width or chrome (§3.1).
- **Build our own, don't adopt a library.** Every service that reaches low-end TVs owns its widget
  and focus layer (Netflix, YouTube, Prime Video, Disney+, BBC); Jellyfin adopted MUI for desktop
  and phone and *kept its TV layout off it*. No web UI library handles D-pad focus and Back, the
  hardest part, which our own focus engine already does. The best library (Adobe's React Aria) is
  well run but costs ~20–30 KB gzip per complex widget and 13 packages against a ~200 KB first page
  (§5–6).
- **What to build:** a small primitive layer, composed rather than inherited: a `Popover` distinct
  from `Dialog`, a `ListBox` that keeps context, a press/input-modality layer, layout primitives,
  ten-foot tokens, and a payload budget (§7). The three overlay complaints fall out of the first two.

## 2. Our code today

Facts from the code (all in `web/apps/web/src/`):

- **One flat folder**, ~294 source files, 52k lines, 65 CSS files (~4,150 lines). No UI library;
  runtime dependencies are react/react-dom 19.3, i18next/react-i18next, qrcode-generator, hls.js,
  jassub, openapi-fetch (`deps/npm.allow`, 33 lines). No router library.
- **The shared layer is two deep:** `ModalDialog` (`modal-dialog.tsx:152`, native `<dialog>` +
  `showModal()`, Escape/Back/outside press/focus restore, always a ✕ unless `hasOwnClose`) is used
  by 18 files; `DropDown`/`DropDownList` (`drop-down.tsx:302`, `:366`) sits on it, places the list
  at its opener like a native select, and draws its own ✕. `RouteChooser`/`ActionChooser` wrap it.
  What goes *inside* a pop-up is bespoke each time (`player-menus.tsx:288-333`).
- **Focus and input are central and good:** `input-layer.tsx`, `focus-engine.ts` (our geometric
  spatial navigation, behind an interface another engine could replace), `focus-memory.ts`,
  `focus-navigation.ts`. This is the part competitors build themselves too.
- **Layout and sizing are per widget.** No layout primitives, no cascade layers; components accept
  `className`/`listClassName` and `drop-down.css:83-97` styles every `a` and `button` inside a list
  to `width: 100%`, which embedded controls then undo (`player-menus.css:14-17`). The 2026-10-05
  research's R1 (cascade layers), R2 (layout primitives) and R5 (components take no `className`)
  were never built; R4 (tokens), R7 (one focus engine) and R8 (the `/ui-gallery` specimen) were.
- **Pop-up kinds:** 1 dialog base, 1 drop-down, ~10 native `<select>`s (8 outside the lab, including
  the audio and subtitle menus under the film, `audio-tracks.tsx:159`, `subtitles.tsx:241`, which a
  TV browser cannot work), the tray's dialogs, `data-tip` tooltips, the over-video cards, the volume
  readout. Six to eight kinds; only dialog and drop-down shared.
- **Payload** (the existing `dist` build, 2026-11-03 01:45): the first page ~250 KB gzip of script;
  all script 583 KB gzip; CSS 10 KB gzip; the player chunk with hls.js 179 KB gzip; react-dom ~73 KB
  gzip of its chunk. **No size budget or test exists.**

### The three complaints, traced

1. **The ✕ on every overlay pop-up.** `DropDownList` always draws it (`drop-down.tsx:403-411`),
   with the list's top padding and scroll padding reserved for it (`drop-down.css:49-66`) and code
   to keep focus clear of it (`keepFocusedClearOfClose`, `drop-down.tsx:129-145`). It came from the
   owner's own rule: 2026-10-28, "a static close button in the top-right corner that the scrollable
   portion scrolls under", and 2026-10-30, "This should be a standard widget so they all behave the
   same" (`modal-dialog.tsx:134-137`). The experts split the same way (§3.1): dialogs and touch sheets
   carry a visible close; menus, listboxes, pickers and slider pop-ups do not. So the 10-30 rule was
   right for dialogs and wrong for pop-overs, because we had only one kind.
2. **The volume pop-up is far too wide.** It is a `DropDown` (`player-menus.tsx:150-172`):
   `min-width: max(13rem, opener width)` (`drop-down.css:10-12`, `:36-41`) on a big screen,
   `min(26rem, 92vw)` from `.modal-dialog` on a phone, plus `width: 100%` on its mute button, for a
   2.75rem slider. Nothing sizes it to its content.
3. **The subtitles list doesn't keep context.** On opening, the current item is laid level with the
   💬 button (`drop-down.tsx:122-126`); 💬 sits near the screen's bottom and the list is cut at that
   edge (`drop-down-place.ts:72-77`), so the current item lands at the list's bottom. Moving by
   arrow uses `scrollIntoView({block: "nearest"})` with scroll padding only for the ✕ band
   (`drop-down.css:56`), so the focused item sits against the edge.

## 3. What the experts say

### 3.1 Structure: composition, and kinds of surface

- **Composition over inheritance.** Flutter's hierarchy is "deliberately shallow and broad";
  padding and alignment are separate widgets, and "rather than subclassing Container… you can
  compose it" ([Flutter architecture](https://docs.flutter.dev/resources/architectural-overview)).
  React's team found no case for component inheritance hierarchies
  ([React docs](https://legacy.reactjs.org/docs/composition-vs-inheritance.html)). Brad Frost: each
  pattern does one thing ([Atomic Design ch. 2](https://atomicdesign.bradfrost.com/chapter-2/)).
  Nathan Curtis argues for subcomponents the implementer composes over monolithic components
  configured by ever more props (secondary sources only).
- **Behaviour apart from look, state as its own layer.** Devon Govett's "Building a Button" series
  (React Aria, ~2020) lists what raw events get wrong (emulated mouse after touch, sticky hover on
  touch, key repeat triggering a menu item as its menu opens) and unifies them in one press event
  ([part 1](https://react-aria.adobe.com/blog/building-a-button-part-1),
  [2](https://react-aria.adobe.com/blog/building-a-button-part-2),
  [3](https://react-aria.adobe.com/blog/building-a-button-part-3)). Zag.js models each widget as a
  small state machine, tracing most bugs to logic scattered in effects
  ([Zag](https://zagjs.com/overview/introduction)).
- **What a pop-over owns:** size to content ("only big enough to display its contents",
  [Apple HIG Popovers](https://developer.apple.com/design/human-interface-guidelines/popovers); the
  HTML UA stylesheet gives `[popover]` and `dialog` `fit-content`); placement at its anchor with
  flipping; light dismiss (outside press, Escape, Back:
  [Open UI](https://open-ui.org/components/popover.research.explainer/)); focus back to the opener
  ([APG Menu](https://www.w3.org/WAI/ARIA/apg/patterns/menubar/)); one at a time ("Never show a
  cascade… of popovers", Apple).
- **Close buttons, by kind:** Apple: a popover's Close is "worth including if it provides clarity,
  like exiting with or without saving"; otherwise it closes on outside press or a choice. Material's
  menus have none. The APG "strongly recommends" a visible button in *modal dialogs*
  ([APG Dialog](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)). Apple's sheets usually
  have Cancel/Close. Android TV: "Avoid showing a back button on the screen"; Back does it
  ([Navigation on TV](https://developer.android.com/design/ui/tv/guides/foundations/navigation-on-tv)).

### 3.2 Layout

- **Intrinsic sizing:** "constraints go down, sizes go up" (Flutter); in CSS, `fit-content` capped
  by the space available, never a fixed width on the container.
- **Ten-foot figures:** tvOS insets 60 pt top/bottom, 80 pt sides, body text 29 pt (min 23)
  ([HIG Layout](https://developer.apple.com/design/human-interface-guidelines/layout),
  [Typography](https://developer.apple.com/design/human-interface-guidelines/typography)); Fire TV
  keeps focus and text in the inner 90 %, body ≥ 14 sp (~28 px at 1080p)
  ([Amazon](https://developer.amazon.com/docs/fire-tv/design-and-user-experience-guidelines.html));
  Xbox lets lists run to the edge but keeps the focused item inside the safe area, at most "six
  clicks" across the screen
  ([Microsoft](https://learn.microsoft.com/en-us/windows/apps/design/devices/designing-for-tv));
  leave room for the focused item's growth (Android, Apple).
- **Scroll with context** (the subtitles complaint): Compose for TV keeps the focused item 30 % in
  from the edge (`BringIntoViewSpec`, `parentFraction = 0.3f`) and stops over-scrolling at the end
  ([Android](https://developer.android.com/training/tv/playback/compose/lists)); Leanback holds a
  "key line"; Microsoft's sample centres. Material 1's menus open with the current item over the
  anchor and a maximum height a row short of the view. The web's tool is `scroll-padding` (WCAG
  technique C43). The APG listbox focuses the selected option on entry
  ([APG Listbox](https://www.w3.org/WAI/ARIA/apg/patterns/listbox/)).

### 3.3 Navigation per medium

- **D-pad:** the CSS Spatial Navigation draft has not moved since 2019; `focusgroup` covers only
  arrow movement inside one group. LG's Enact (one React UI on every LG input): five-way mode always
  has a focused element, pointer mode uses hover, touch shows no focus, moving the mouse switches to
  pointer and a key press back, containers remember their last child, a closed pop-up returns focus
  to its opener ([React Summit 2025](https://gitnation.com/contents/from-tv-to-touch-how-we-made-react-ui-work-across-every-input-mode)).
  Samsung: exactly one focused object, one focus look. Apple moves focus without the user only when
  the focused item disappears.
- **Keyboard:** the APG: always one active component; the selected state looks different from
  focus; selection that follows focus is "extremely detrimental" when selecting is slow
  ([APG Keyboard](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/)) (our audio
  track menu commits on Enter, which is right).
- **Pointer and touch:** 44×44 px for coarse pointers (WCAG 2.5.5). Jason Grigsby: input is
  "undetectable" and "transient"; what was used a moment ago says nothing about what comes next
  ([Cloud Four](https://cloudfour.com/thinks/four-truths-about-input/)). Hence: track the *last
  input*, not the device.
- **Controller:** Microsoft: no tooltips on TV, nested actions into a context flyout.

### 3.4 One look or several?

The common answer: **one codebase; shared behaviour, logic and tokens; a ten-foot presentation and
focus layer chosen at run time; some widgets *replaced* on TV rather than restyled.** Google ships a
separate TV component set but has moved its TV lists back into the shared foundation; Apple keeps
one focus model but tvOS has no popovers, pop-up buttons or sliders at all; Microsoft shares the
framework and switches layout and scale on Xbox; Amazon (2026) says 70–85 % of a TV app is shareable,
"the focus logic is shared, the input handling is forked"; LG runs one React UI switched by input
mode. The dissent: Grigsby and Wroblewski warn against keying a heavy mode to the device type.
Reconciled: layout and scale by a per-device setting (guessed from the window), focus rings and hover
by the last input, behaviour identical everywhere, and a few widgets swapped on TV (a volume slider
becomes the remote's job or an Up/Down stepper; a `<select>` becomes our chooser; tooltips become
visible labels).

## 4. Competitors

| Who | UI stack | TV |
| --- | --- | --- |
| Jellyfin web | React + MUI for desktop/phone ("modern" layout); its own `emby-*` custom elements (v0, polyfilled) for the legacy and **TV** layouts; own geometric `focusManager`, TV only | TV kept off MUI |
| Jellyfin Android TV | Kotlin, Leanback + Compose foundation, **its own widget set** (`ui/base/`: buttons, dialog, popover, list, seekbar), no Material/`androidx.tv` | native |
| Stremio | its own components (dialog, popup, slider, multiselect, bottom sheet); react-aria for `useHover` only; react-focus-lock; a forked spatial-nav polyfill | same web |
| Plex (2025) | rewritten in React Native for mobile, TV and web, own components and native players | RN |
| Emby | Jellyfin's legacy layer's ancestor; Theater in Electron | TV mode |
| Kodi | C++ with its own GUI engine, XML skins | own |
| Netflix | React on Gibbon, its own renderer, since 2013 for weak TV CPUs | own runtime |
| YouTube TV | HTML5 on Cobalt, a lightweight memory-capped container; focus as app state | own runtime |
| Prime Video | React-Livingroom over a Rust/Wasm engine, now moving to a Rust UI SDK: collection page input latency ~250 ms in JS vs ~33 ms in Rust on sub-1 GHz dual cores ([InfoQ 2025](https://www.infoq.com/presentations/prime-video-rust)) | own runtime |
| Disney+ | a Rust "core engine and runtime" for consoles, set-top boxes and TVs (job ads) | own runtime |
| BBC iPlayer | JSX + CSS, light DOM, its own spatial library LRUD; problems are "memory related" ([W3C 2022](https://www.w3.org/2022/11/01-me-minutes.html)) | own nav |

**Pattern:** the giants own their renderer; mid-size and open-source players own their widget
layer on the platform's toolkit. None of those examined uses an off-the-shelf web component kit for
its ten-foot UI. Netflix's tech-blog posts, Disney's engine and the new Plex web client's stack could
not be read directly (403 or unpublished).

## 5. Libraries

Sizes from bundlephobia (min+gzip, React excluded), transitive package counts from deps.dev,
advisories from the GitHub database, all fetched 2026-11-03. Per-widget sizes share internals and
do not add up.

| Option | Licence | Cost | Packages | Fit |
| --- | --- | --- | --- | --- |
| react-aria-components 1.22 (Adobe) | Apache-2.0 | Popover 21.7 KB, ListBox 27.4, Select 29.5, Slider 9.0 | 13 | best accessibility; no D-pad model; overlays portal with their own focus scope, which must coexist with our engine and Back; 0 advisories |
| Radix dialog / popover | MIT | 13.2 / 23.5 KB | 25 / 34 | maintenance questioned, now WorkOS |
| Ariakit 0.4 | MIT | 66 KB whole | 11 | one main maintainer, still 0.x |
| Headless UI 2.2 | MIT | 63 KB | 22 (pulls react-aria) | |
| Base UI 1.9 (MUI) | MIT | 166 KB whole, Dialog 20.9 | 9 | |
| MUI / Mantine / Chakra | MIT | 157 / 175 / 314 KB whole | 32 / 19 / 99 | desktop and touch looks, no D-pad; MUI needs emotion |
| shadcn/ui | MIT | = Radix or Base UI + Tailwind + `cn` | same npm surface | copy-in is cosmetic: the behaviour still comes from npm |
| @bbc/tv-lrud-spatial | Apache-2.0 | 2.2 KB | 1 | an alternative engine to ours if ours loses a device comparison |
| Norigin spatial navigation | MIT | 16.6 KB | 4 | own focus tree, doesn't move DOM focus |
| LightningJS / Blits | Apache-2.0 | 97 KB | 4 | WebGL canvas: a rewrite off the DOM, loses DOM accessibility |
| Preact 11 + compat | MIT | ~9.5 KB vs ~60 KB React+DOM (secondary) | 1 | the cheapest lighter runtime (a Vite alias); React 19 features and react-i18next under compat unproven; one high advisory in 2026 |
| Solid / Svelte / Lit | MIT | 6–8 KB | | each a full rewrite |

**The platform's own floor** (MDN compatibility data against TV engines: Fire TV Silk Chromium 138
in our lab; Chromecast ~Chrome 90 (unverified); webOS 22/23 = Chromium 87/94; Tizen 2022/2023 =
M85/M94):

| Feature | Silk | Chromecast | 2022–23 TVs |
| --- | --- | --- | --- |
| `<dialog>` + `showModal`, `scroll-padding`/`scroll-margin` | yes | yes | yes |
| `:focus-visible` | yes | yes | yes, except Tizen 2022 |
| `inert`, `popover` | yes | **no** | **no** |
| `:has()`, container queries, anchor positioning | yes | no | no |
| customizable `<select>` (`base-select`) | yes (135+) | no | no |

So a pop-over primitive positions and light-dismisses in script, using `popover` and anchor CSS only
behind `@supports`. Anchor positioning is not usable at our own Safari 17.1 floor (D271) either.

## 6. Build or adopt

| | Payload | Supply chain | TV fit | Effort | Risk |
| --- | --- | --- | --- | --- | --- |
| (a) styled kit | +30–60 KB first widgets | 19–99 packages | poor | low start, high to restyle and make TV-safe | fighting its look and focus forever |
| (b) headless (React Aria) + our look | ~5–30 KB per complex widget | 13 packages, well run | neutral to awkward | medium | behaviour we don't own on the hardest platform |
| (c) copy-in (shadcn) | as (b) + Tailwind | same as (b) | none gained | medium | as (b) |
| **(d) our own primitives on the platform + our focus engine** | a few KB per widget | none | best | medium to high for the hard widgets | accessibility correctness is ours |

**Recommendation: (d).** The evidence is consistent across competitors; the hard part (D-pad
focus and Back) is already ours and no library solves it; it adds no packages and next to no bytes;
the platform features we can rely on (`<dialog>`, `scroll-padding`, `:focus-visible`) cover our TVs,
and the ones we can't (`popover`, `inert`) are easier to fall back from in our own code than inside a
library's portals. Accessibility risk is held by taking the APG patterns as the specification, the
existing `jsx-a11y` lint, browser-mode tests, and a screen-reader pass.

**What would change it:** a screen-reader audit finding our listbox or menu repeatedly wrong (then
React Aria's *hooks*, not components, for those widgets, after a Fire TV check that its overlays
coexist with our Back and focus); widgets that are expensive to get right arriving (date pickers,
virtualised grids with selection, async comboboxes); a measured Fire TV spike showing React Aria
cheap enough; or the TV route moving to React Native (D10, [client-framework.md](client-framework.md)),
leaving the web layer to desktop, phone and Cast. Any library is the owner's dependency decision
(AGENTS.md rule 3).

## 7. What we could do better: the proposal

Principles (each sourced in §3):

1. **Compose; no base decides width or chrome.** Small parts (anchor placement, dismissal, focus
   scope, scroll-with-context) composed into `Dialog`, `Popover`, `Menu`, `ListBox`, `Choice`.
2. **Two kinds of transient surface, not one.** `Dialog`: modal, a visible ✕ (the owner's 10-30
   rule stays here), focus trapped. `Popover`: anchored, sized to content (`fit-content`, capped by
   the space available), flips at edges, light-dismisses on outside press, Escape and Back, one at a
   time, focus back to its opener, **no ✕**. The overlay's 💬 🎧 ⁝ 🔈 become popovers.
3. **A `ListBox` that opens on the current choice with one row of context** above and below
   (`scroll-padding-block` of a row; maximum height a row short of the space), placed so the current
   item is inside the list rather than pinned to the opener's middle.
4. **Rows and grids hold focus at a key line** (~30 % in, clamped at the ends), inside the safe area
   while the row runs to the screen's edge.
5. **Selected and focused look different; costly selections commit on Enter.**
6. **The last input decides focus rings, hover and target sizes**, not the device: hover only for a
   mouse, rings after keys and D-pad, 44 px targets for coarse pointers. One press abstraction
   (cancel on leave or scroll, ignore key repeat for opening, no emulated mouse after touch).
7. **Focus is never lost and never moves by itself**, except to a neighbour when the focused item
   disappears; containers remember their last child.
8. **Ten-foot tokens, not ten-foot forks:** a 5 % safe area, body text ≥ 24–28 px at 1080p, focus
   growth with gutters for it, at most about six presses across the screen; the same components, and
   only a few *replaced* on TV (volume, selects, tooltips).
9. **Finish the 2026-10-05 architecture items:** cascade layers (R1), layout primitives such as
   Stack and Cluster (R2), and no `className` restyling of components (R5) — the cause of
   `drop-down.css` reaching into its children.
10. **A payload budget as a gate** (the owner's, rule 2): the first page and each route chunk,
    measured gzip, failing when it grows past its budget. None exists today.
11. **Retire the native `<select>`s** for the TV-safe chooser (the audit's open item).
12. **Hard widgets as small state machines** (open, closing, highlighted item), not scattered
    effects.

A separate, cheap lever for payload: a **one-day Preact spike** measured on the Fire TV stick (~50 KB
gzip off every first load if the secondary figure holds), independent of the widget decision.

### Sequencing against the deferred UX rewrite

The owner deferred the major UX rewrite until enough features are in (2026-10-29). The primitives
above are not that rewrite: they are what the rewrite and every feature before it would be built on,
and the overlay's complaints show their absence costing now. Proposed: a toolkit batch first
(`Popover` and `Dialog` split, `ListBox`, the overlay's pop-ups moved onto them, layout primitives,
the payload budget), with screens left as they are; the rewrite later on top.

## 8. Wished-for features that need significant UI work

From the plan files, the feature inventories, `code-debt.md`, `ui-competitors.md` and the owner's
requests:

- **Player:** skip intro/recap/credits and Up next at the credits (inventory A-5/A-6, D82); scrub
  thumbnails and chapter marks on the timeline (A-7/A-8); caption style with a live preview (A-11);
  night mode, speed, quality with "Auto" and why, sleep timer (A-3, feature inventory §12.3); a
  playback failure shown over the film (F-4); the remaining native selects under the film.
- **Navigation and screens:** season posters instead of the season choice; Watch Together and Cast
  as pages (B-4); the warning sign's "What's wrong" page (D-5); the tray's info icon for "update
  available" (D-12); Manage needing an index as admin pages grow (§12.1); the ten-foot TV layout
  (D-11, D313's B5); profiles with "Who's watching" and a PIN pad (E-6).
- **Browsing:** genres, filters, watched marks, watchlist, collections, missing episodes (C-7 to
  C-13): filter panels, badge slots on tiles, a list view; virtualised grids (no pagination,
  `ui-competitors.md` §6); multi-select with an action bar for bulk edits (§12.10); "Identified
  incorrectly" showing file and folder names (C-16); an uncertain match's candidates (F-5); library
  settings in a Configure dialog (C-3); a tree picker for a new library's folders (code-debt H-18).
- **Watch Together** (`plans/watch-together.md`): each viewer's own subtitles and track, a roster,
  notices with undo, text chat, a reconnecting state, groups above two.
- **Text entry and forms on TV:** sign-in, search, the manual metadata editor; settings that show
  where each value came from (D188); the Diagnostics page with guided tests and a sync calibration
  (F-1).
- **Later:** music (a new player and library kind), offline downloads, play statistics, parental
  controls and the rules engine, native phone apps. Live TV and photos are marked not wanted.

The widget kinds these need beyond today's: a virtualised tile grid with focus memory and a key
line, a filter panel, a tree picker, multi-select, text entry by remote, a timeline preview pop-up,
form controls with live preview, a PIN pad, and a chat panel. Several of these (the virtualised grid
with selection, text entry) are the "expensive to get right" widgets of §6's reconsideration.

## 9. For the owner to decide

1. **The close-button rule:** keep the ✕ on dialogs (the 2026-10-30 rule), drop it from every
   pop-over (menus, choosers, the overlay's 💬 🎧 ⁝ 🔈, the banner menu, ⁝ on pages)?
2. **Build or adopt:** our own primitives (recommended), or a library for some widgets?
3. **A toolkit batch** before the deferred rewrite, as §7 proposes, and whether the overlay's three
   complaints wait for it or get a quick fix now.
4. **A payload budget gate** (a gate change, the owner's).
5. **A Preact spike** on the Fire TV stick.
6. **On TV, is volume a pop-up at all?** tvOS has no sliders; the remote's own volume, or an Up/Down
   stepper, may replace the 🔈 pop-up there.
