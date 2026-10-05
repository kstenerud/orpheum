> **ABOUTME:** What platform guidelines, WCAG and design research say a media library's web UI
> should do, what competitors' interfaces look like and feel like now, and what people say about
> them in 2025–2026. Read it before writing the UI principles and the skin that D245 asks for.

# UI Design Practice: Best Practice, Competitors' Visual Design and Recent Sentiment

- **Date:** 2026-10-05.
- **Requested by** the owner, in [D245](../../decisions/working-notes.md): *"The big thing we
  need before she starts using it is proper UX, with a skinned UI"*, and *"I don't need multiple
  skins (only one is fine)."* D245 §3 asks for research first: best practices for user
  interfaces, above all a media server's; what competitors do; and what people say about their
  interfaces. This document covers those three for the visual design. The web ecosystem's
  practices (D245 §3, third bullet) are not covered here.
- **Builds on** [ui-sentiment.md](ui-sentiment.md) (complaint volume, mods and skins) and
  [ui-competitors.md](ui-competitors.md) (what works and fails, by surface), and on the plan
  [web-client.md](../plans/web-client.md). It does not repeat them. Where it updates their
  numbers, it says so.
- **Sources:**
  - primary guidelines: Apple HIG (read through its JSON data endpoints), WCAG 2.2
    Understanding documents, W3C Media Accessibility User Requirements, W3C i18n, Material
    tokens (from the `material-web` and `material-components-android` repositories, because
    `m3.material.io` renders only in a browser), Fluent 2, Android window size classes, the
    GOV.UK Design System, MDN, eCFR
  - research writing: Nielsen Norman Group, Baymard
  - companies' own words: Netflix (About Netflix and its TechBlog by archive.org), Apple
    Newsroom, Disney+, Warner Bros. Discovery press, Plex's blog and forum, Jellyfin's release
    posts, Firecore's forum
  - measured: Plex's forum through its Discourse JSON API; GitHub's repository and issue APIs;
    Hacker News through Algolia; eighteen Jellyfin theme READMEs, coded by keyword
  - source read: `jellyfin-web` at `dd6d1d6` (2026-10-05), its themes and card sizes
- **Cache:** every fetched page is under `/yoloai/cache/http/uidp/` (`<name>.raw` as fetched,
  `<name>.txt` as extracted text); the `jellyfin-web` clone is in `/yoloai/cache/git/`.
- **Legend:** **[doc]** read in a primary document; **[src]** read in source code; **[M]**
  measured here, on 2026-10-05; **[S]** press or other secondary report; **[user]** a user's own
  words. Anything without a tag in §6 is this researcher's judgement and is marked as such.

## 1. Where the web client stands today

Read from `web/apps/web/src` on 2026-10-05 [src]:

- **One light palette**: background `#fafafa`, text `#1b1b1f`, muted `#5c5c66`, a white surface.
  The font is `system-ui`. There is one spacing token (`--space-page: 1rem`) and one type size
  token (`--font-size-title: 1.5rem`). Everything else is the browser's default.
- **The theming seam exists**: `tokens.css` holds every colour and size as a custom property,
  and component classes are prefixed with their file name so a theme can target them
  ([D138](../../decisions/working-notes.md) §4). Focus is the two-colour ring of
  [D189](../../decisions/working-notes.md), and reduced motion zeroes every transition.
- **The navigation is one flat bar** of links. An admin sees eleven: Films, Series, Search,
  Identify, Libraries, People, Settings, Jobs, System, Account, About. Viewing and administering
  share one row, and there is no home screen apart from Films.
- **The poster grid** is `repeat(auto-fill, minmax(9rem, 1fr))` with 2:3 posters, title and year
  below, and `content-visibility: auto` on each row.
- **The detail page** puts a backdrop above the content at up to 40% of the viewport height,
  then a poster beside the facts. Nothing sits over the art.

So the structure is sound and the skin is absent: it looks like what it is, a working
scaffold. The rest of this document is about what to put on the seam.

## 2. Best practice, by topic

### 2.1 Information architecture and navigation

- **Navigation that is visible beats navigation that is hidden, more on desktop than on
  phones.** NN/g's quantitative study found that hiding global navigation degrades the
  experience on both, and more on desktop [doc] ([NN/g, hidden
  navigation](https://www.nngroup.com/articles/find-navigation-mobile-even-hamburger/)).
- **A vertical, left-aligned list suits a broad or growing set of destinations**, at the cost of
  width, and should be kept visible and front-loaded with keywords [doc] ([NN/g, vertical
  navigation](https://www.nngroup.com/articles/vertical-nav/)). A person's libraries are such a
  set: it grows, and the owner names it.
- **Icons need text labels**, because almost no icon has a standard meaning [doc] ([NN/g, icon
  usability](https://www.nngroup.com/articles/icon-usability/)).
- **The services moved their shortcuts to where they can be seen.** Netflix's 2025 TV redesign:
  *"Until now, shortcuts to Search and My List were somewhat hidden on the left-hand side. We're
  moving them to the top of the page where they're more noticeable"* [doc] ([About Netflix,
  2025](https://about.netflix.com/en/news/unveiling-our-innovative-new-tv-experience)). Prime
  Video dropped its 2022 left sidebar for a top bar in 2024 [S]
  ([Engadget](https://www.engadget.com/amazon-prime-video-redesign-announced-204019411.html)).
  Max went the other way in January 2025, moving its top menu to a left rail [S]
  ([TV Technology](https://www.tvtechnology.com/news/max-updates-homepage-user-interface)). Plex
  moved its TV apps from a left sidebar to a top bar in 2025 and moved them back in August 2026
  (§4.1). **There is no consensus on top versus left; there is one on visible versus hidden.**
- **Exiting playback lands on the detail page of what was playing, with a resume option**, not
  on the home screen [doc] ([HIG, Playing
  video](https://developer.apple.com/design/human-interface-guidelines/playing-video)).

### 2.2 Poster grids, sizing and density

- **Artwork carries the browsing.** Netflix's 2014 research: artwork *"was not only the biggest
  influencer to a member's decision to watch content, but it also constituted over 82% of their
  focus while browsing"*, and members spent *"an average of 1.8 seconds considering each
  title"* [doc] ([About Netflix, The Power of a
  Picture](https://about.netflix.com/en/news/the-power-of-a-picture)).
- **Portrait posters are coming back, for density.** Apple's tvOS 26 app has *"new poster art
  that allows more shows and movies to be displayed"* [doc] ([Apple Newsroom,
  2025-06](https://www.apple.com/newsroom/2025/06/apple-tv-brings-a-beautiful-redesign-and-enhanced-home-entertainment-experience/)),
  and Disney+'s 2025 redesign added *"cinematic-style poster artwork"* [doc]
  ([Disney+](https://www.disneyplus.com/explore/articles/disney-plus-app-redesign-new-features)).
  Netflix's TV redesign went the other way, to larger tiles [doc], and drew the complaint that
  it takes more scrolling to see the same titles ([ui-sentiment.md](ui-sentiment.md) §6).
- **Apple's tvOS grids are the one published size table** [doc] ([HIG,
  Layout](https://developer.apple.com/design/human-interface-guidelines/layout)): a 1920-point
  screen inset 80 points at the sides, a horizontal gap of 40 points and at least 100 points
  between rows, with unfocused item widths from 860 points (two columns) to 160 points (nine).
  *"Use consistent spacing. When content isn't consistently spaced, it no longer looks like a
  grid and it's harder for people to scan."*
- **Jellyfin's web grid is a breakpoint table** [src] (`src/components/cardbuilder/card.scss`):
  portrait cards are 1/7 of the row from 1,400 CSS pixels, 1/8 from 1,600, 1/9 from 1,920 and
  1/10 from 2,100. At a 1,920-pixel window that is a card of about 210 pixels before gutters. On
  a television the row is fixed at six.
- **Which pixel sizes to fetch** is already researched in [artwork-sizes.md](artwork-sizes.md);
  Plex asks for about 240×360 for its grids, Kodi's 1080p wall slot is about 300 wide.
- **Density is a preference people state.** A Plex user on 2026-09-21, 25 likes [user]: *"I do
  not want to see the top half of my screen filled with a picture and lots of text… I do not
  want to see ONLY 6.5 poster images… I DO want to see the smaller poster images… I DO want to
  see the name below the small poster"* ([forum
  thread](https://forums.plex.tv/t/rant-new-atv-app-no-thanks-i-want-the-old-one-back/943162)).
  A Kodi user on Estuary: *"My only complaints ever are the large size posters"* [user]
  ([Kodi forum](https://forum.kodi.tv/showthread.php?tid=383662)).

### 2.3 Typography and type scales

- **Published scales converge.** Material 3's type tokens [doc]
  ([`_md-sys-typescale.scss`](https://github.com/material-components/material-web/blob/main/tokens/versions/v0_192/_md-sys-typescale.scss))
  run body 1rem on 1.5rem, title 1.375rem, headline 1.5–2rem and display 2.25–3.5625rem, with
  labels down to 0.6875rem. Fluent 2's web ramp [doc]
  ([Fluent typography](https://fluent2.microsoft.design/typography)) is 12/16, 14/20 (body),
  16/22, 20/26, 24/32, 28/36, 32/40, 40/52 and 68/92 pixels. Apple's macOS default is 13 points
  with a minimum of 10 [doc] ([HIG,
  Typography](https://developer.apple.com/design/human-interface-guidelines/typography)).
- **Avoid light weights.** Apple: *"prefer Regular, Medium, Semibold, or Bold font weights, and
  avoid Ultralight, Thin, and Light"* [doc]. NN/g on dark themes: thin fonts are *"swallowed
  up"* by a dark background [doc] ([NN/g, dark
  mode issues](https://www.nngroup.com/articles/dark-mode-users-issues/)).
- **Line length for reading: 50–75 characters** [doc]
  ([Baymard](https://baymard.com/blog/line-length-readability)). A synopsis set across a 1,920-pixel detail page breaks this.
- **Text must survive user spacing.** WCAG 1.4.12 (AA): no loss of content when line height is
  1.5 times the font size, paragraph spacing twice it, letter spacing 0.12 times and word
  spacing 0.16 times [doc] ([Understanding
  1.4.12](https://www.w3.org/WAI/WCAG22/Understanding/text-spacing.html)). Fixed-height text
  boxes fail this.
- **Hierarchy is preserved when people change text size**, and truncation is kept to a minimum
  as it grows [doc] (HIG, Typography).

### 2.4 Colour and dark themes

- **Light mode reads better; dark mode is preferred by many, and expected to follow the
  system.** NN/g's review of the literature: for normal vision, *"visual performance tends to be
  better with light mode"*, and the advantage grows as text gets smaller [doc] ([NN/g, dark vs
  light](https://www.nngroup.com/articles/dark-mode/)). Its survey of 115 users split about a
  third each way, and *"users think about dark mode at the operating-system level, not at the
  individual application level"*; the advice is to mirror the system setting by default [doc]
  ([NN/g, dark mode issues](https://www.nngroup.com/articles/dark-mode-users-issues/)).
- **For media, dark-only is sanctioned.** Apple: *"Avoid offering an app-specific appearance
  setting"*, but *"it can make sense for an app that supports immersive media viewing to use a
  permanently dark appearance that lets the UI recede"* [doc] ([HIG, Dark
  Mode](https://developer.apple.com/design/human-interface-guidelines/dark-mode)).
- **Dark grey, not black, for surfaces; lighter means higher.** Material's baseline dark theme
  is *"dark grey instead of black, which increases visibility for shadows and also reduces eye
  strain for light text"*, and raised surfaces get lighter [doc]
  ([Dark.md](https://github.com/material-components/material-components-android/blob/master/docs/theming/Dark.md)).
  Apple has the same idea as base and elevated backgrounds [doc]. Pure black on OLED screens
  also smears when scrolled, a practitioner report rather than a study [S]
  ([Lookup](https://medium.com/lookup-design/designing-a-dark-theme-for-oled-iphones-e13cdfea7ffe)).
  Plex keeps true black as an opt-in *"High Contrast"* theme, *"for accessibility purposes"*
  [doc] ([Plex blog, 2021](https://www.plex.tv/blog/finding-your-true-colors-with-plex/)).
- **Saturated colours on dark backgrounds fail.** NN/g: they *"have poor visibility"*, and a
  blue at 4.59:1 *"seems to blur slightly with the dark background"* [doc].
- **Brand colour sparingly.** Fluent: *"Avoid overusing brand colors or using them on large
  surfaces as they can dilute a hierarchy"* [doc] ([Fluent
  colour](https://fluent2.microsoft.design/color)).
- **The ratios.** WCAG 1.4.3 (AA): 4.5:1 for text, 3:1 for large text; 1.4.11 (AA): 3:1 for
  the parts of controls and graphics needed to understand them [doc] ([Understanding
  1.4.3](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html),
  [1.4.11](https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html)). Apple asks for
  at least 4.5:1 and to *"strive for a contrast ratio of 7:1, especially in small text"* [doc].

### 2.5 Spacing and alignment

- **One base unit and a short ramp.** Fluent's base is 4 pixels, with 2, 6 and 10 added only
  to align icons, *"a 4x system reduces confusion while being easy to implement"* [doc]
  ([Fluent layout](https://fluent2.microsoft.design/layout)).
- **Space is the grouping device.** Fluent: *"spacing creates logical sections of content on a
  page without having to use lines or other graphical elements as a divider"* [doc].
- **Alignment is what makes a page look ordered.** Apple: *"Alignment makes an app look neat and
  organized, and can help people track content while scrolling"* [doc] (HIG, Layout).
- **The focus rule sets the gutter**: under [D189](../../decisions/working-notes.md) a focused
  tile may grow by at most half the gutter on each side, so the gutter follows from the focus
  scale.

### 2.6 Motion and reduced motion

- **Durations**: most UI animation belongs in 100–500 ms; about 100 ms for small feedback such
  as a toggle, 200–300 ms for a panel entering, and *"at 500ms, animations start to feel like a
  real drag"* [doc] ([NN/g, animation
  duration](https://www.nngroup.com/articles/animation-duration/)). Material's duration tokens
  are short 50–200 ms, medium 250–400, long 450–600 [doc]
  ([`_md-sys-motion.scss`](https://github.com/material-components/material-web/blob/main/tokens/versions/v0_192/_md-sys-motion.scss)).
- **Purpose and frequency**: *"Don't add motion for the sake of adding motion"*, *"generally avoid
  adding motion to UI interactions that occur frequently"*, and *"don't make people wait for an
  animation to complete"* [doc] ([HIG,
  Motion](https://developer.apple.com/design/human-interface-guidelines/motion)).
- **Nothing moves on its own for long**: WCAG 2.2.2 (A) requires a way to pause anything that
  moves automatically for more than five seconds [doc] ([Understanding
  2.2.2](https://www.w3.org/WAI/WCAG22/Understanding/pause-stop-hide.html)); 2.3.3 (AAA) asks
  that motion triggered by interaction can be turned off [doc]. An auto-advancing hero carousel
  needs a pause control; NN/g notes people *"often scroll past carousels"* and that *"a static
  hero… may be better"* [doc] ([NN/g,
  carousels](https://www.nngroup.com/articles/designing-effective-carousels/)).

### 2.7 Imagery: backdrops, posters, scrims and legibility

- **Text over art needs a measured scrim.** NN/g: white text over images routinely fails 4.5:1,
  a 30% black box was *"still too low"*, 50% passed, and the fix must be checked *"for all
  possible images"*, not the one in the mock-up [doc] ([NN/g, text over
  images](https://www.nngroup.com/articles/text-over-images/)).
- **Translucent material over content is now an argued-against pattern.** NN/g on iOS 26's
  Liquid Glass: the interface is *"less legible, and constantly pulling focus rather than
  supporting seamless access to content"* [doc] ([NN/g, Liquid
  Glass](https://www.nngroup.com/articles/liquid-glass/)).
- **Never upscale art past its size.** Jellyfin 12.0 changed this deliberately: *"Low resolution
  posters now appear at their actual size instead of being blown up to fit, which looks smaller
  but sharper"* [doc] ([Jellyfin
  12.0](https://jellyfin.org/posts/jellyfin-release-12.0/)).
- **Reserve the box before the image arrives.** Images without dimensions or `aspect-ratio`
  are a main cause of layout shift [doc] ([web.dev,
  CLS](https://web.dev/articles/optimize-cls)).
- **Soften white in dark mode**: Apple suggests slightly darkening images with white
  backgrounds so they do not glow [doc] (HIG, Dark Mode).
- **Art-derived colour** is common: Plex builds its blurred gradient background from four corner
  colours taken from the theme or art [doc] (Plex blog, 2021).

### 2.8 Loading, empty and error states

- **The three limits**: 0.1 s feels instant, 1 s keeps the flow of thought, 10 s keeps attention;
  past 10 s show percent-done and a way to stop [doc] ([NN/g, response
  times](https://www.nngroup.com/articles/response-times-3-important-limits/)).
- **Skeletons for a page, spinners for a module, nothing under a second.** *"If a page takes less
  than 1 second to load, skeleton screens or spinners aren't necessary… the quick flashing page
  can cause users to feel like they can't keep up"*; skeletons and spinners suit waits under 10
  s, a progress bar anything longer [doc] ([NN/g, skeleton
  screens](https://www.nngroup.com/articles/skeleton-screens/)).
- **Show something at once** and let people do other things while the rest loads [doc] ([HIG,
  Loading](https://developer.apple.com/design/human-interface-guidelines/loading)). For
  playback: no loading screen if it starts quickly; after two seconds, *"a black loading screen
  with a centered activity spinner and no surrounding content"* [doc] (HIG, Playing video).
- **An empty state says why and offers the next step** [doc] ([NN/g, empty
  states](https://www.nngroup.com/articles/empty-state-interface-design/)). Errors: visible,
  constructive, and preserving the user's effort [doc] ([NN/g, error
  messages](https://www.nngroup.com/articles/error-message-guidelines/)). For forms: an error
  summary at the top that takes focus, and a message beside each field [doc] ([GOV.UK,
  validation](https://design-system.service.gov.uk/patterns/validation/)). The content of an
  error is already set by [web-client.md](../plans/web-client.md) (item, cause, one next action).

### 2.9 Responsive behaviour, phone to large desktop

- **Window classes, not devices.** Android's window size classes are compact below 600, medium
  600–839, expanded 840–1199, large 1200–1599 and extra-large from 1600 [doc] ([Android
  Developers](https://developer.android.com/develop/ui/compose/layouts/adaptive/use-window-size-classes)).
  They are a usable set of breakpoints for the web too.
- **Reflow at 320 CSS pixels** without two-dimensional scrolling (WCAG 1.4.10, AA), which is a
  1,280-pixel window at 400% zoom [doc] ([Understanding
  1.4.10](https://www.w3.org/WAI/WCAG22/Understanding/reflow.html)).
- **Container queries are stable in every engine** [doc]
  ([web.dev](https://web.dev/blog/cq-stable)), so a card can size itself from the space it is given rather than from the window.
- **Touch targets**: WCAG 2.5.8 (AA) sets 24×24 CSS pixels as the minimum, or enough spacing
  [doc] ([Understanding
  2.5.8](https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html)); Apple's iOS
  default is 44×44 points with 28×28 the minimum [doc] ([HIG,
  Accessibility](https://developer.apple.com/design/human-interface-guidelines/accessibility)).

### 2.10 Accessibility and the captions UI

The requirements Orpheum has already adopted are in [D188](../../decisions/working-notes.md) §2
and [accessibility.md](../plans/accessibility.md), including the 47 CFR §79.103 caption controls.
What the visual design adds:

- **Focus must stay in view** (2.4.11, AA): a sticky header, a toast or the player's controls
  must not cover the focused item [doc] ([Understanding
  2.4.11](https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum.html)).
  [D189](../../decisions/working-notes.md) §5 already requires this.
- **A focus indicator worth the name** (2.4.13, AAA): at least the area of a 2-pixel perimeter
  and a 3:1 change against the unfocused state [doc] ([Understanding
  2.4.13](https://www.w3.org/WAI/WCAG22/Understanding/focus-appearance.html)). D189's ring is
  3 pixels and two colours, so it passes.
- **Ring for fields, highlight for lists**: *"use a focus ring for a text or search field, but use
  a highlight in a list or collection"* [doc] ([HIG, Focus and
  selection](https://developer.apple.com/design/human-interface-guidelines/focus-and-selection)).
  D189 forbids darkening a tile, so for poster grids the ring and lift remain the rule.
- **Captions sit clear of the edge**: at least 1/12 of the screen height above the bottom
  [doc] ([MAUR CC-15](https://www.w3.org/TR/media-accessibility-reqs/)). The browser's own
  `::cue` styling allows only a short list of properties [doc]
  ([MDN](https://developer.mozilla.org/en-US/docs/Web/CSS/::cue)), so the full §79.103 set needs
  a caption layer the player draws, which [web-client.md](../plans/web-client.md) already has for
  other reasons.

### 2.11 Internationalisation: long strings and right-to-left

- **Translations are longer, most of all short ones**: W3C's averages are 200–300% expansion for
  English strings up to 10 characters, 180–200% for 11–20, falling to 130% over 70 [doc]
  ([W3C, text size in
  translation](https://www.w3.org/International/articles/article-text-size)). Buttons and tabs
  are the short strings.
- **Logical properties** (`margin-inline-start`, `inset-inline-end`) let one stylesheet serve
  both directions [doc]
  ([MDN](https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_logical_properties_and_values)),
  and metadata whose direction is unknown takes `dir="auto"` [doc] ([W3C,
  dir](https://www.w3.org/International/questions/qa-html-dir)).
- **Pseudo-localisation and font fallback** are already researched in
  [internationalisation.md](internationalisation.md) §8 and §9.

### 2.12 The player's controls

The behaviour is set by [web-client.md](../plans/web-client.md) ("The player, from what other web
players got wrong"). For the look:

- **A custom player should look and behave like the one people know**: *"A custom experience that
  diverges slightly from the system-provided experience can cause frustration because people don't
  know which of their habitual interactions they can continue to use"* [doc] (HIG, Playing video).
- **Overlays defer to the picture**: *"avoid large, distracting overlays"*, and prefer translucent
  SDR graphics to bright opaque ones because some screens retain images [doc] (HIG, Playing
  video).
- **Show the video at its own aspect ratio**, letterboxed rather than cropped [doc] (HIG, Playing
  video).

### 2.13 Settings and admin screens

- **Fewer settings, found where people expect them**: *"Minimize the number of settings you
  offer"*, and *"avoid using settings to ask for setup information you can get in other ways"*
  [doc] ([HIG, Settings](https://developer.apple.com/design/human-interface-guidelines/settings)).
- **Inputs sized to their content**: *"Postcode inputs should be postcode-sized"* [doc] ([GOV.UK,
  text input](https://design-system.service.gov.uk/components/text-input/)). A width says what is
  expected.
- **Tables for comparable records**: jobs, files and sessions are rows to scan and sort [doc]
  ([NN/g, data tables](https://www.nngroup.com/articles/data-tables/)).
- **Weak signifiers cost effort**: in NN/g's eye-tracking study, flat UIs with weak clickability
  cues needed more effort than strong ones [doc] ([NN/g, flat
  UI](https://www.nngroup.com/articles/flat-ui-less-attention-cause-uncertainty/)). Admin screens
  are where ghost buttons and unlabelled icons cost most.
- **Looks buy forgiveness, and hide problems in testing**: the aesthetic-usability effect [doc]
  ([NN/g](https://www.nngroup.com/articles/aesthetic-usability-effect/)). Polish is worth doing,
  and it is also a reason to test tasks, not impressions.

## 3. What competitors do visually, screen by screen

Observations from documents, source and forum posts. Nothing here comes from logged-in sessions
of the commercial services (§7). "UNVERIFIED" marks what this research could not check.

### 3.1 Media servers

| | Plex Web | Jellyfin web | Emby web | Kodi (Estuary) | Infuse 8 |
| --- | --- | --- | --- | --- | --- |
| **Navigation** | left sidebar of pinned, ordered libraries; the web kept it when the TV apps lost it [S] | **12.0: "Modern" (MUI) layout by default** on desktop and phone, the old one kept as "Legacy"; library toolbar merged into a sticky app bar [doc] | left drawer [UNVERIFIED in 2026] | left main menu with home widgets per category [user] | tabbed, platform-native [doc] |
| **Default appearance** | dark; Light, High Contrast (true black), and accent themes; tokens such as `accentBackground` [doc] | Dark (`#101010` background, `#202020` paper, `#00a4dc` accent, Noto Sans); Light, WMC, Blue Radiance, Apple TV, Purple Haze, all now on one CSS-variable base [src][doc] | dark [UNVERIFIED] | dark | dark |
| **Grid** | poster size slider [UNVERIFIED in 2026]; asks for 240×360 art | breakpoint table, 7–10 per row on desktop, 6 on TV [src] | — | 300-pixel wall slots at 1080p | — |
| **Detail page** | backdrop with a blurred colour field from the art [doc] | backdrop banner, poster, facts | — | — | a "New" detail layout in 8.0, the old "Modern" one kept as an option [doc] |
| **Admin** | a settings tree inside the same client | dashboard moved to the new stack in 10.11; in 12.0 server settings sit in the user menu [doc] | dashboard | settings levels | none (client only) |

**Infuse kept the old detail view as a choice.** A user: *"I'm not so keen on the New design. It
seems a little busy and cramped… So it is great that we have the choice"* (7 likes) [user]
([Firecore forum](https://community.firecore.com/t/infuse-8-now-available/51308)). That is
[D108](../../decisions/working-notes.md) practised by the most-praised client in the field.

**Kodi's default skin is valued for restraint.** In a 2026 thread asking for a new default: *"The
default skin is a good compromise between so much eye candy it's unusable and so little it looks
like the file manager from windows 3.1"*, and *"the whole point of the Estuary skin is that it's
simple and it works"* [user] ([Kodi forum](https://forum.kodi.tv/showthread.php?tid=383662)).

### 3.2 The subscription services

| | Netflix | Apple TV | Disney+ | Prime Video | HBO Max |
| --- | --- | --- | --- | --- | --- |
| **Navigation** | shortcuts moved from the left to the top on TV, 2025 [doc] | tvOS 26: Liquid Glass chrome [doc] | top tabs: For You, then one per brand subscribed to [doc] | top bar since 2024; left for discovery, right for paid add-ons [S] | left rail since January 2025 [S] |
| **Home** | larger tiles, the highlighted title expands in place; labels such as *"#1 in TV Shows"* [doc] | portrait poster art, *"allows more shows and movies to be displayed"* [doc] | video hero carousel, brand row, badges [doc] | hero, rows | hover plays a preview at the top of the page; video and audio can be switched off [doc] |
| **Appearance** | dark only | follows the system on Apple platforms (UNVERIFIED on the web) | dark | dark | dark |

**The trend in 2025–2026 is portrait art for density, badges on tiles, previews behind a
switch, and navigation moved where it can be seen.** Every preview that plays on its own ships
with an off switch: Max's says so in its launch release [doc]
([WBD](https://press.wbd.com/us/media-release/hbo-max/max-updates-homepage-user-interface-enhanced-video-elements)),
which is the pattern [ui-competitors.md](ui-competitors.md) §2 recorded.

### 3.3 What reads as polished, and what reads as amateur

Judgement, built from the guidelines in §2 and the theme authors' own feature lists in §4.2:

- **Polished:**
  - **one rhythm**: a single spacing ramp, so gaps repeat. ElegantFin's first claims are
    *"rounded corners and even spacing everywhere"* and *"neat layout that puts important stuff
    up front"*.
  - **one edge**: titles, rows and text blocks start on the same column.
  - **restraint**: one accent, used for the primary action, progress and focus; artwork supplies
    the colour.
  - **consistent containers**: one corner radius per kind of thing, one shadow scale.
  - **type with three or four sizes and two weights**, sized for the reading distance.
  - **states designed, not defaulted**: a missing poster is a titled card, not a broken-image
    icon; an empty library says what to do.
- **Amateur:**
  - margins chosen per screen
  - grey text on grey surfaces under 4.5:1
  - icon-only buttons with no labels
  - text laid straight over art
  - low-resolution art stretched to fill
  - several accents, gradients and glass effects competing with the posters
  - motion on every hover
  - a browser-default form inside a styled page; this is where most self-hosted admin screens
    show their age

## 4. What people think now (2025–2026)

### 4.1 Plex: the redesign backlash has plateaued, then restarted on Apple TV

Discourse `like_count`, measured on 2026-10-05 [M], against
[ui-competitors.md](ui-competitors.md) §0 (2026-09-23):

| Thread | Created | Likes then | Likes now | Views |
| --- | --- | --- | --- | --- |
| *New Experience Public Release Feedback* | 2025-03-31 | 6,482 | 6,486 | 42,939 |
| *New UI is an awful experience* | 2025-09-16 | 6,425 | 6,434 | 20,199 |
| *Roku New Experience Release Update* | 2025-09-18 | 4,759 | 4,759 | 34,399 |
| *Releasing the new app was a crazy decision* | 2025-03-31 | 2,715 | 2,717 | 25,618 |
| *Plex Just Ruined Their Roku App* | 2025-09-16 | — | 1,848 | 6,954 |
| ***Rant: New ATV app: no thanks. I want the old one back*** | **2026-09-21** | — | **1,139** | 4,337 |

The four largest now total **20,396** and have barely moved. **The new thread is the news**:
1,139 likes in fourteen days, after the new experience reached Apple TV. Its most-liked posts are
about function more than looks: a custom player replacing tvOS's native one, which *"allowed for
better scrubbing… and never hung"* (48 likes); *"It's not awful because it's new, it's awful
because it's a bad experience compared to the previous"* (40); the density complaint quoted in
§2.2 (25); and options hidden behind a long press, *"Plex continues to be obsessed with 'hiding'
options behind long taps/presses"* (18) [user].

**Plex reversed its navigation.** On 2026-08-17 Plex announced that the TV apps' primary
navigation returns *"to the lefthand side of the screen"*, with favourite libraries as top-level
sources, after *"consistent feedback from the start regarding the navigation. Our fall update was
the first step… but they weren't enough"* [doc] ([Plex
forum](https://forums.plex.tv/t/navigation-updates-coming-to-preview-users/941751)); the press
covered it the next day [S] ([How-To
Geek](https://www.howtogeek.com/plex-brings-back-sidebar-navigation-after-users-refuse-to-let-it-go/),
[XDA](https://www.xda-developers.com/after-months-backlash-plex-rolling-back-controversial-big-screen-app-redesign/)).
Fire TV users in May 2026 reported actions going *"from two clicks to around six"*, and that the
old sidebar *"was still present on the web"* [S] ([PiunikaWeb, reporting
Reddit](https://piunikaweb.com/2026/05/04/plex-fire-tv-redesign-unusable-users-slam-navigation/)).
**Plex Web itself has not had the new experience**; a January 2026 forum question asking when its
features would reach the web drew *"I hope not"* [user] ([Plex
forum](https://forums.plex.tv/t/are-there-any-plans-for-the-new-plex-experience-features-for-the-web-ui/935371)).

**What this adds to [ui-sentiment.md](ui-sentiment.md) §1.** Its finding still holds: the anger
is about information architecture and latency, not looks. Two visual points are now named
explicitly. **Density**: users object to a half-screen of art and text and to few, large posters.
**Hidden actions**: a long press as the only way to reach options is a signifier failure, the
same family as NN/g's flat-UI finding.

### 4.2 Jellyfin: the official client caught up on structure; the themes say what it lacks

**Jellyfin 12.0 (2026-09-07) made the Modern layout the default** and moved every built-in theme
onto one CSS-variable base [doc]. Six days later (2026-09-13), *"Version 12 looks and feels"*
on GitHub [user] ([discussion #18019](https://github.com/orgs/jellyfin/discussions/18019)): *"it
mixes 'Settings' with server settings. When I first started version 12 i was super confused.
Like WTH where are my settings gone"*. A contributor's reply agreed and opened jellyfin-web
#8531 about it: moving the dashboard into the
user menu reduced clutter in the top bar but cost discovery. **The admin entry point is a design
problem in its own right**, and hiding it to tidy the bar is the failure.

**Themes, measured [M]** (GitHub stars, 2026-10-05, against [ui-sentiment.md](ui-sentiment.md)
§3, 2026-09-21): ElegantFin 2,163 → 2,189; JellySkin 1,026 → 1,027; Ultrachromic 994 → 998;
skin-manager 660 → 665; finimalism 460 → 461; Jellyfin-Media-Bar 207 → 207. Flat over two
weeks, as expected. **The earlier table missed newer themes**: `abyss-jellyfin` 740, `scyfin`
735, `ZestyTheme` 531, `Jellyfish` 308, `GlassFin` 244, `Flow` 116 (*"Plex-inspired theme"*),
`finity` 76. A GitHub search for `jellyfin theme` returns 383 repositories.

**What theme authors advertise.** Eighteen theme READMEs, coded by keyword [M] (a crude method:
a word's presence, not its prominence):

| Feature advertised | READMEs of 18 |
| --- | --- |
| backdrops or blur | 14 |
| rounded corners | 13 |
| dark or black | 13 |
| works on mobile | 12 |
| spacing or alignment | 12 |
| a choosable accent colour | 10 |
| animation or hover effects | 10 |
| glass or translucency | 9 |
| a chosen font | 9 |
| less clutter, minimal | 9 |
| a hero, spotlight or banner | 8 |
| inspired by Netflix, Plex, Apple or Jellyseerr | 6 |
| **a light variant** | **2** (ElegantFin, catppuccin) |

**Reading it.** What the community adds to the default client is consistency (spacing, radius,
one font), art used as atmosphere (backdrops, blur), an accent the owner chooses, and a home
banner. Almost all of it is dark. abyss-jellyfin's *"Spotlight home banner"* is *"your current
Continue Watching item, complete with backdrop image… and a resume play button"* [doc]
([README](https://github.com/AumGupta/abyss-jellyfin)): a hero that serves resuming, not
promotion.

**Replacement clients now outdraw the official web client** [M]: Streamyfin 5,234 stars, Swiftfin
4,198, Plezy (Plex and Jellyfin) 3,605, Wholphin 2,993, Blink (desktop) 1,170, against 3,893 for
`jellyfin-web` and 1,707 for `jellyfin-vue`.

**Hacker News**, the *Jellyfin 12.0* story
([HN 49604861](https://news.ycombinator.com/item?id=49604861), 2026-09-08, 623 points, 351
comments) [M]: 18 comments mention the UI by keyword, and none says the web client is ugly. *"I'd
love a Kodi theme with the design and responsiveness of the Jellyfin Roku app"*; *"I always end
up going back to the default Kodi theme - it's ugly but it works"* [user]. A 2025 thread had the sharper complaint,
*"the UI jank. The thumbnail tiles are slow to load… Searches and filters flicker as you type"*,
and the general one, *"FOSS frequently has bad UI design"* [user]
([HN 43063167](https://news.ycombinator.com/item?id=43063167)). **Perceived polish here is
loading behaviour**: tiles that pop in and lists that flicker read as amateur before any colour
does.

### 4.3 The services

- **Netflix** kept its 2025 TV redesign. Its stated reasons: shortcuts that were *"somewhat
  hidden"*, and a *"clean and modern design"* [doc].
- **Apple's Liquid Glass** drew a sharp review from NN/g: *"restless, needy, less predictable,
  less legible"* [doc]. Infuse adopted it on iOS and tvOS in 8.2.5 [S] ([Firecore
  forum](https://community.firecore.com/t/infuse-8-2-5-now-available/57408)).
- **Max** shipped hover-to-play previews with a settings switch from the first day [doc].

## 5. What follows for one skin

Judgement throughout, each item with its reason. Where a decision already covers it, it is cited
and not reopened.

### 5.1 Principles

1. **Art is the colour; the chrome is neutral.** Posters supply the hue (82% of browsing
   attention, §2.2). Chrome uses near-neutral greys, and one accent marks the primary action,
   progress, the selected item and focus. Reason: Fluent's and NN/g's warnings about brand colour
   and saturated colour on dark (§2.4), and the theme authors' own move to one choosable accent.
2. **Dark for watching, near-black not black.** The browse and player surfaces are dark, with a
   background around `#111`–`#141414` and surfaces stepping lighter for elevation; true black only
   behind video. Reason: Apple sanctions dark-only for immersive media; Material's grey keeps
   shadows visible; pure black smears on OLED phones, which Cut A's phone user has. Whether there
   is a light palette is open (§5.3).
3. **One spacing ramp on a 4-pixel base**: 4, 8, 12, 16, 24, 32, 48, 64, as tokens beside the ones
   `tokens.css` already holds. Page side margins step with the window class: about 16 pixels below
   600, 24 to 1199, 32–48 above. Reason: rhythm is what makes a page read as designed (§3.3), and
   D189 needs the gutter to be a known value.
4. **Type: a short scale, sized for a laptop, set in two weights.** Body at 1rem (16 pixels) with
   line height 1.5; small text no lower than 0.75rem; headings at about 1.25, 1.5, 2 and 2.5rem;
   weights 400 and 600, with 700 for the title of a page. Tabular figures for times and sizes.
   Synopses capped at about 70 characters per line. Reason: the Material and Fluent ramps agree
   on these, light weights fail on dark (§2.3), and WCAG 1.4.12 rules out fixed-height text.
5. **Navigation visible and split in two.** On a wide window, a left sidebar: Home, then the
   person's libraries in their own order ([D138](../../decisions/working-notes.md) §2), Search;
   below a divider, the admin area ("Manage") for those who have it. On a phone, a bottom bar of at
   most four or five labelled destinations, with libraries one tap in. Account and About live in a
   menu at the edge. Reason: visible beats hidden (§2.1); a growing, user-named list suits a
   vertical bar; Plex's reversal and Jellyfin 12's buried dashboard (§4) show what goes wrong when
   either rule is broken. Today's flat bar of eleven links mixes the two audiences.
6. **Posters with titles, at a density the person chooses.** 2:3 posters with title and year
   beneath ([D188](../../decisions/working-notes.md)), sized by the container: about 150 pixels
   on a phone (three across at 390) and 170–200 on a laptop, with a small, medium or large
   choice kept as a setting, and the list view [D138](../../decisions/working-notes.md) §7 asks
   for. One gutter across every grid and row. Reason: the density complaints in §2.2 and §4.1;
   Apple's and Disney's return to portrait art.
7. **Home: Continue Watching first, then the person's rows. No autoplaying hero**
   ([D188](../../decisions/working-notes.md) declines it). If there is a banner at all, it is the
   most recent Continue Watching item with a resume button, static, and at most a third of the
   viewport. Reason: the half-screen hero is what Plex's users complain of, and the only banner
   the theme community built serves resuming.
8. **The detail page: art behind, never under, the words.** A backdrop at the top fading into
   the page background through a gradient scrim, with every text pixel checked against the
   worst case at 4.5:1. The poster, title, facts and one primary button (*Play* or *Resume
   at 41:12*) sit on the page colour, not on the art. Secondary actions are labelled. Episodes
   are a list with still, number, title and synopsis. Reason: NN/g's "all possible images" rule,
   and the Plex user who wants names under episode art.
9. **Images never shift, stretch or flash.** Every image box has its `aspect-ratio` reserved;
   art loads at the closed sizes of [artwork-sizes.md](artwork-sizes.md) through `srcset`; a
   poster never scales past its pixels (Jellyfin 12's change); a missing poster is a neutral card
   with the title set in type; images fade in over about 150 ms or, with reduced motion, appear.
   Reason: tiles that pop in were the polish complaint in §4.2.
10. **States are designed screens.** Nothing for waits under a second; a skeleton of the real
    layout for a page that takes longer; a spinner inside a module; a progress bar with a stop
    button for jobs. Every empty state says why and offers the next step. Errors follow
    [web-client.md](../plans/web-client.md). Reason: §2.8.
11. **Motion: short, rare and optional.** 100–150 ms for hover and focus, 200–250 ms for panels
    and menus, nothing longer than 300 ms; no parallax; nothing moves on its own. The existing
    reduced-motion rule stays. Reason: §2.6.
12. **The player looks like a browser player.** A dark gradient at the bottom only, the familiar
    layout (play and time at the start, tracks, volume and fullscreen at the end), controls that
    hide after a few seconds without input but never while paused or focused, touch targets of
    44 pixels, and the subtitle layer clear of them. Reason: Apple's "diverges slightly" warning
    (§2.12), and Plex's new tvOS player is that thread's most-liked complaint (§4.1).
13. **Admin screens use the same skin, denser.** A page template: title, one sentence saying what
    the page is for, then sections. Forms with labels above the field, widths matching the
    expected value, an error summary that takes focus. Tables for jobs, files and sessions, with
    sticky headers. Provenance shown where [web-client.md](../plans/web-client.md) asks for it. A
    compact density token for these screens rather than separate styles. Reason: §2.13; the
    browser-default form is where self-hosted software looks unfinished.
14. **Built for other languages from the first component.** Logical properties only; no width
    fixed to an English label; `dir="auto"` on metadata; every component checked under the
    pseudo-locale of [internationalisation.md](internationalisation.md). Reason: §2.11's 200–300%
    expansion lands on exactly the buttons and tabs.
15. **One corner radius per kind, one shadow scale, one icon set with labels.** Reason: the
    polished list in §3.3; consistency is cheap once it is tokens.

### 5.2 Borrow and avoid

| Borrow | From | Why |
| --- | --- | --- |
| A left sidebar of the person's libraries, in their order | Plex Web, and Plex's TV apps after the reversal | the one navigation users fought to keep |
| Dark grey surfaces that lighten with elevation | Material, Apple | depth without lines, shadows that show |
| True black as an opt-in high-contrast palette | Plex | a real accessibility need, kept out of the default |
| Every theme built on one set of variables | Jellyfin 12.0 | the seam D138 already chose |
| Keeping the old layout as an option when the new one ships | Infuse 8 | [D108](../../decisions/working-notes.md) in practice |
| A resume-first banner, if any banner | abyss-jellyfin | serves the user's task, not promotion |
| Restraint | Estuary, Infuse | what users praise in both |

| Avoid | Seen in | Why |
| --- | --- | --- |
| Navigation moved into overlays, or settings into a user menu | Plex TV 2025, Jellyfin 12.0 | both drew named complaints and one reversal |
| Options reachable only by long press or hover | Plex TV 2026 | no signifier; touch and keyboard users never find them |
| A hero filling half the screen | Plex TV 2026 | the density complaint |
| Autoplaying previews | Netflix, Max, Disney+ | declined by [D188](../../decisions/working-notes.md) |
| Translucent glass over content | tvOS 26, several themes | legibility (NN/g) |
| Text laid over art without a measured scrim | widespread | fails 4.5:1 on some art |
| Stretched low-resolution art | Jellyfin before 12.0 | looks broken |

### 5.3 Open questions for the owner

1. **Light, dark, or both?** Options: dark only (Apple's media exception; what every competitor
   ships by default); dark by default with a light palette that follows the system setting (NN/g's
   advice, more legible for the admin screens' dense text, one more palette to keep correct); or
   dark for the viewing screens and light for the admin screens. The tokens make the second
   cheap; the third is unusual and would need a reason.
2. **The accent colour.** One hue, desaturated enough to pass 3:1 on the dark surface and 4.5:1
   where it carries text. It is also the brand, and the name is provisional
   ([D195](../../decisions/working-notes.md)), so the owner may want to choose both together.
3. **The typeface.** The system stack costs nothing, renders natively and covers every script
   the operating system does, which suits [internationalisation.md](internationalisation.md)
   §9. A bundled variable font gives a recognisable look on every machine, at the cost of a
   download and a licence to check. Recommended: the system stack for Cut A.
4. **Is a static resume banner wanted on Home**, or only rows? D188 declined autoplay, not a
   banner.
5. **Is the poster-size choice in Cut A**, or one well-chosen size first?
6. **Does the colour of the artwork tint the detail page** (as Plex and Apple do), or is the page
   always the neutral background? Tinting looks richer and makes contrast harder to guarantee.
7. **Rounded or square.** A matter of taste; the rule is only that one radius is used per kind of
   thing.

## 6. Where the evidence is weak

- **Theme feature coding** counts words, not emphasis; a README that says "no blur" counts as
  "blur".
- **Sentiment** is from forums, GitHub and Hacker News, which over-represent technical users.
  Reddit returned 403 again.
- **Plex Web's current details** (poster slider, sidebar) come from secondary reports and earlier
  research, not from a session.
- **Competitors' contrast and spacing were not measured.** Statements about what reads as
  polished in §3.3 are judgement, informed by the cited guidelines.

## 7. Not tried

- No logged-in session of Plex Web, Emby, Netflix, Apple TV on the web, Disney+, Prime Video or
  HBO Max; no screenshots taken or measured. Their web layouts are described from their own
  announcements and the press, which mostly describe the TV apps.
- Emby's web client (closed source) and Channels were not researched beyond the table's
  UNVERIFIED cells.
- `m3.material.io` and the Netflix TechBlog on Medium were not read directly; Material's values
  come from its token files and Netflix's post from archive.org.
- No usability test of any design, and no measurement of the current Orpheum client's contrast.
- Kodi's forum was read in its printable view only; the Jellyfin forum and Emby's community were
  not searched.
- The web ecosystem's component and layout practices (D245 §3, third bullet) are outside this
  document.
