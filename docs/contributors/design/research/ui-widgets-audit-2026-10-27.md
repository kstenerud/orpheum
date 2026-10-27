> **ABOUTME:** Where the web client's UI widgets are duplicated: each kind of widget, every
> implementation of it with file and line, how the copies differ, and the standard set proposed to
> replace them, in the order to build it, with the checks that would hold it. Read before adding a
> button, chooser, dialog, slider or player control to `web/apps/web`, and before D364's controls.

# UI Widgets Audit: One Standard Set in Place of Bespoke Copies

- **Date:** 2026-10-27. Asked by the owner: *"I want an audit of the ts code to make sure we're DRY
  with the rest of it. We should have a standard set of UI widgets rather than bespoke everywhere."*
- **Method:** read-only, at `main` an earlier commit. Every `.tsx` and `.css` file in `web/apps/web/src/`
  (tests left out of the counts), counted by script and then read where the count pointed. The
  packages (`web/packages/sdk`, `web/packages/player`) hold no UI; the cast receiver app
  (`web/apps/cast-receiver`) builds two DOM elements by hand and is left out. Library browsing,
  sorting, Home and settings were being changed by another agent at the time; they are read as they
  stand on `main`, and their line numbers will move.
- **The device lab** (`lab*.tsx`, `/lab`) is a set of probes for development. Its widgets are
  counted apart, since its look is deliberately large and plain, and nothing in it should block a
  consolidation.
- **Labels:** counts are of this snapshot, not a gate, and go stale with the next commit.

---

## 1. Summary

The client already has the base the owner asks for, and uses it more often than not: `base.css`
gives every plain `<button>`, `<input>`, `<select>`, `<fieldset>` and `[role="alert"]` one look
from tokens, and there are thirteen shared components (`ConfirmButton`, `ModalDialog`,
`BusyGroup`, `FormRow`, `Loaded`, `Link`, `Icon`, `VolumeRange`, `PosterCard`/`PosterGrid`,
`WarningSign`, `BackButton`, `CredentialsForm` with `PasswordField`, `EndOfList`). Of 163
`<button>` elements, 111 carry no class and take the base look. The duplication is in the
*variants* the base does not name, and in the player:

| Kind | Implementations today | Of which bespoke copies |
| --- | --- | --- |
| Filled (primary) button look | 6 rule blocks + `button[type=submit]` | 5 (`libraries__create`, `title__play`, `watch-together__start`, `device-activation__match`, `lab__button[aria-pressed]`) |
| Icon-only button | 7 rule blocks | 6, each with its own size, radius, hover and glyph (`▶`, `❚❚`, `⤢`, `×` as text, the rest `Icon`) |
| Chooser (pick one of several) | native `<select>` ×11 (+2 lab), `RouteChooser` ×1 | the 11 natives, which README says a television's browser cannot work, 4 of them on the Play page |
| Overlay / pop-up | `ModalDialog`, the menu's `<details>`, the warning sign's inline pop-up, the settings tooltip | 2 (the menu and the warning sign close by their own rules; the warning sign answers no Escape or Back) |
| Two-step irreversible confirmation | `ConfirmButton` ×18 uses | 1 hand copy (`settings.tsx:84` `RemoveQuestion`) and 1 filled red button with no confirmation (`library-manage.tsx:151`) |
| Labelled slider row | `VolumeRange` + 3 label wrappers | 3 identical wrappers and CSS blocks (`film-volume`, `together-viewer__volume`, `together-voice__partner`) |
| Media timeline (scrub) | 2 | 2 (`tv-controls.tsx:216`, `cast-remote.tsx:285`): different units, one debounced, neither with the iPhone touch fix `VolumeRange` has |
| Earlier/later stepper | 2 | 2 (`subtitles.tsx:235`, `sound-delay.tsx:45`), laid out differently |
| Problem line | `<p role="alert">{problem}</p>` ×49, `<output>{problem}</output>` ×6, `CredentialsProblem`, `settings__problem` | all hand-written; the same failure is an alert on one page and a silent `<output>` on the next |
| Loading line | `Loaded` ×34 uses, plus 4 bare `<p>{t("common.loading")}</p>` | 4; `Loaded` itself borrows `title.css`'s `title__loading` class |
| Saved-setting checkbox | `OwnChoiceSwitch` + 3 hand copies | 3 (`autoplay-setting.tsx`, `account.tsx:95`, `left-out-libraries.tsx`), which report a failure as `<output>` where the original uses an alert |
| Panel / card surface | 13 rule blocks of `surface + radius-card + padding` | all hand-written, with five different paddings |
| Over-the-video surfaces | 9 raw colour values (`#fff`, `#000`, `rgb(0 0 0 / 55…85%)`, `rgb(255 255 255 / 12%)`) | no token, though `--color-video-background` exists and goes unused |

**The top consolidations, by gain:**

1. **Player controls, before D364 builds more.** A `PlayerButton` (round icon button over the film),
   a `Timeline` (scrub bar: units, debounce, touch-anywhere, commands-for-arrows), and an
   over-the-video token set. Today the TV overlay, the cast remote, Up next, Still watching, the
   Watch Together viewer and the host's sound-on each draw their own; D364's remote page, tray icon
   and "Ready" page, and the computer's own controls, would be the seventh to tenth copies. Gain:
   one place for remote focus, touch and look on every player surface; the cast remote's timeline
   gets the iPhone fix it lacks.
2. **`Choice` (a focusable drop-down list), replacing the eleven native `<select>`s.** README's
   own invariant says a native `<select>` is unreliable on a television's browser
   (library-browsing.md §5.5), and `RouteChooser` was built for that, but the Play page's audio
   track, subtitle track, subtitle search language and the series' ordering are still native
   selects. Gain: the player's track menus work by remote, and the choosers look alike everywhere.
3. **`Button` variants as classes in `base.css`** (`primary`, `quiet`, `icon`, `on-video`,
   `irreversible`), and `ButtonLink` for a link drawn as a button. Gain: five copies of the filled
   look and six of the icon look fold into one each; `className="quiet"`, used twice today, starts
   meaning something (no stylesheet defines it).
4. **`Problem` and `Loading` lines.** Gain: 49 + 6 hand-written problem lines become one
   component that decides alert or status in one place; the four bare loading lines get `Loaded`'s
   delay and pulse.
5. **Layout and surface primitives** (`Panel`, `Stack`, `Cluster`), as
   [ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md) R2 recommended and nothing built.
   Gain: about a fifth of the CSS (the 21 hand-written list resets and 13 card blocks) stops being
   per-screen.

Each consolidation keeps the look it replaces except where noted in §4, so the risk is in the
tests that pin class names (§4.3) and in the few places where the copies' behaviour genuinely
differs, which §3 lists.

## 2. What exists already

The shared pieces, how many places use them, and whether `/ui-gallery` shows them. The gallery
shows the status cues, the icons, a plain button, a disabled one and a `ConfirmButton`; it draws
the banner tray and the poster card by copying their markup and classes
(`ui-gallery.tsx:110–128`, `:141–154`) rather than rendering the components, so a change to either
reaches the gallery only by hand.

| Component | File | Uses | In the gallery |
| --- | --- | --- | --- |
| Base element look | `base.css:77–211` | every plain control | plain and disabled button |
| `ConfirmButton` | `confirm-button.tsx:11` | 18 in 13 files | yes |
| `ModalDialog` | `modal-dialog.tsx:69` | 7 in 7 files | no |
| `BusyGroup` | `busy-group.tsx:11` | 15 in 14 files | no (it looks like nothing) |
| `FormRow`, `submitWith` | `form-row.tsx:23`, `:11` | 10 in 7 files; `submitWith` 5 more forms | no |
| `Loaded` | `loaded.tsx:14` | 34 in 28 files | no |
| `Link` | `link.tsx:9` | 31 in 16 files | no |
| `Icon` | `icon.tsx:15` | every icon (held by `icon.test.tsx`) | yes |
| `VolumeRange` | `volume-range.tsx:31` | 3 | no |
| `PosterCard`, `PosterGrid`, `Poster`, `Still` | `poster-card.tsx`, `poster.tsx` | 2, 3, 3, 5 | copied markup |
| `WarningSign`, `StatusTray`/`StatusNotes` | `warning-sign.tsx:32`, `status-cues.tsx` | 5, 7 | yes |
| `BackButton` | `back-button.tsx:18` | 5 | no |
| `CredentialsForm`, `PasswordField`, `CredentialsProblem` | `credentials-form.tsx` | 16, 7, 9 | no |
| `EndOfList`, `RequestWait` | `end-of-list.tsx`, `request-wait.tsx` | 4, 2 | no |
| `FullScreenButton` | `full-screen-button.tsx:16` | 1 (the TV overlay) | no |
| `RouteChooser` | `library-browse.tsx:439`, not exported | 3 (library, view, sort) | no |
| `AboutButton` (icon + tooltip + dialog) | `server-settings.tsx:72` | 2 | no |
| `OwnChoiceSwitch` | `sound.tsx:41` | 2 (boost, levelling) | no |

The `Icon` consolidation of an earlier commit is the model: one component, one stylesheet rule for its
colours, and `icon.test.tsx` reading every source to prove nothing else draws an `<svg>` or paints
a shape.

## 3. Inventory by kind

### 3.1 Buttons

**Plain:** 111 of 163 `<button>`s carry no class (`base.css:86–118`): bordered, raised surface,
`--size-control` tall. Good, and the rule to keep.

**Filled (primary).** `base.css:105` fills a `type="submit"` button with the accent. Five other
rule blocks repeat that fill for a button or link that is not a submit:

| Where | Element | Differs from the base fill |
| --- | --- | --- |
| `libraries.css:43–62` (`libraries__create`, `libraries__manage`) | `Link` | a link drawn as a button: repeats all of `base.css`'s button block, then the fill |
| `title.css:96–110` (`title__play`) | `Link` | padding `--space-6`, no border; pinned by `title.test.tsx`, `input-pages.browser.test.tsx`, `input-player.browser.test.tsx` |
| `watch-together.css:8–16` (`watch-together__start`) | `button` | identical to submit |
| `device-activation.css:60–64` (`device-activation__match`) | `button` | identical, no hover |
| `lab.css:117` (`lab__button[aria-pressed="true"]`) | `button` | the fill as a pressed state |

A second filled colour, the status blue, is used for "the one tap the browser waits for":
`together-voice.css:88–94` (`together-voice__tap`) and the pressed Talk button.

**Quiet (text-only).** `continue-watching.css:11–25` makes `poster-card__action` (Hide) a muted
word with no border. `what-happened.tsx:51` and `discovery-gate.tsx:553` write `className="quiet"`,
which no stylesheet defines: the author meant this look and got the plain button.

**Icon-only.** Seven rule blocks, each its own size and hover:

| Where | Size | Shape | Content | Hover / state |
| --- | --- | --- | --- | --- |
| `settings.css:56–103` (`settings__control`, 3 uses) | `--size-control` square | radius-control | `Icon` | surface; tooltip from `data-tip` |
| `status-tray.css:15–37` (`status-tray__button`) | 2.25rem × control | radius-control | `Icon` | raised; `aria-expanded` underline |
| `warning-sign.css:31–41` (`warning-sign__button`) | auto, `--space-1` pad | none | `Icon` | raised |
| `app-shell.css:105–128` (the menu's `<summary>`) | text padding | none | `Icon` | colour only |
| `tv-controls.css:35–44` (`tv-controls__play`, `__full-screen`) | 3rem round | pill | text glyphs `▶`, `❚❚`, `⤡`, `⤢` | none |
| `up-next.css:48–52` (`up-next__dismiss`) | base height | pill | text `×` | none |
| `together-voice.css:33–75` (`together-voice__talk`) | 3.25rem round | 50% | `Icon` | pressed fill, level ring |

The TV overlay's and Up next's glyphs are characters, not `Icon`s: `icon.test.tsx` cannot see
them, their size follows the font, and a television's font may lack `❚` or `⤢`.

**Toggle (`aria-pressed`).** Five uses. `together-voice__talk` and `lab__button` style the pressed
state; **`identify.tsx:137–147` (the Needs-a-match filter) does not**: its two buttons look the
same pressed or not, so which list is shown is told only to a screen reader. No `base.css` rule
covers `[aria-pressed="true"]`.

**Irreversible.** `ConfirmButton` holds it (D278 §4), 18 uses. Two places use its classes by hand:
`settings.tsx:84–98` (`RemoveQuestion`) rebuilds ConfirmButton's second step under an icon
trigger, and `library-manage.tsx:149–157` gives "Approve removals" the filled red with no first
step. Whether approving withheld removals is irreversible is the owner's call; if it is, it should
ask first, by README's own invariant.

**Lab:** 20 `lab__button` (bigger type, `lab.css:111`), 1 `lab__tile`. A size variant of the
plain button, kept apart deliberately.

### 3.2 Choosers, drop-downs and selects

| Where | What it chooses | Kind | Note |
| --- | --- | --- | --- |
| `library-browse.tsx:439` `RouteChooser` | library, view, sort | button + `ModalDialog` drop-down of `Link`s | the TV-safe one; not exported |
| `audio-tracks.tsx:95` `AudioMenu` | the audio track | native `<select>` | **Play page** |
| `subtitles.tsx:204` `TrackMenu` | the subtitle track | native `<select>` | **Play page** |
| `subtitle-search.tsx:172` `LanguagePicker` | a search's language | native `<select>` | **Play page** |
| `series.tsx:175` `OrderingChoice` | episode ordering | native `<select>`, `htmlFor` label | series page |
| `browse.tsx:215` `SortChoice` | name or newest | native `<select>` | Films and Series: the job `RouteChooser` does for the Libraries page |
| `create-library.tsx:139`, `library-manage.tsx:250`, `repoint-root.tsx:94` | a node or library kind | native | admin |
| `people.tsx:156`, `person.tsx:259` | a role | native | admin |
| `files.tsx:106` | a filing filter | native | admin |
| `identify.tsx:137` | which waiting list | `aria-pressed` buttons in a `fieldset` | a segmented control; see §3.1 |
| `create-library.tsx:256`, `this-device.tsx:32` | kind, OK button | radios in a `fieldset` | |
| `lab-cast.tsx:151`, `lab-extend.tsx:209` | probe options | native | lab |

`focus-navigation.ts:230–275` works around the native select for a controller (`showPicker`, else
step to the next option), which helps a pad but not a remote whose browser's own list is the
problem (library-browsing.md §5.5). The Play page's three are the ones a television meets.

### 3.3 Dialogs, overlays and pop-ups

| Where | Kind | Closes by | Focus back | Back / Escape |
| --- | --- | --- | --- | --- |
| `ModalDialog` (`modal-dialog.tsx:69`): cast-asked, folder browser, `RouteChooser`, `AboutButton`, the Watch Together tray panel, an invitation, the headphones notice | native modal `<dialog>` | onClose; outside press optional | yes (D361 §3) | both |
| The menu, `BannerMenu` (`app-shell.tsx:424`) | `<details>` positioned under its `<summary>` | outside press, focus leaving | to its summary on Back | Back (own `useCommands`) |
| `WarningSign` (`warning-sign.tsx:32`) | inline `position: absolute` pop-up, `role="note"` | its own button only | n/a | **neither** |
| The settings tooltip (`settings.css:77–90`) | CSS `::after` from `data-tip` | hover / focus out | n/a | n/a |
| Disclosures: `title.tsx:238`, `files.tsx:66`, `filing-report.tsx:146`, `system-discovery.tsx:362`, `what-happened.tsx:66` | `<details>` | itself | n/a | n/a |
| Show/hide toggles: `identify.tsx:86`, `match-correction.tsx:33` | `aria-expanded` button showing a section | itself | n/a | n/a |

`ModalDialog` is the one dialog, and nothing bypasses it with a raw `<dialog>`. The pop-up panel
look is written three times: `.modal-dialog` (`modal-dialog.css:4`), the menu's list
(`app-shell.css:196–209`) and `.warning-sign__detail` (`warning-sign.css:43–60`), each with
`surface-raised`, `radius-card`, a border and a shadow, two different shadows between them.

### 3.4 Lists, grids, rows and cards

- **Poster grid and row:** `PosterGrid` (`browse.tsx:51`) is a component, used by Films, Series
  and the Libraries page. The row is not: `home.tsx:60` and `continue-watching.tsx:124` each write
  `<ul className="poster-row" data-focus-container="">` by hand, and the comment above
  `.poster-row` (`poster-card.css:149`) names a `poster-row.tsx` that does not exist.
- **Row lists** (a `ul` reset to a grid with a gap): 21 `list-style: none` blocks, with gaps of
  `--space-1` to `--space-4`: `your-devices.css:3`, `people.css:3`, `jobs.css:3`, `cast.css:58`,
  `libraries.css:13`, `libraries.css:36`, `folder-picker.css:9`, `watch-together.css:49`, `:89`,
  `series.css:40`, `loader-households.css:8`, `library-browse.css:30`, `:58`, `play.css:30`,
  `app-shell.css` ×2, `browse.css` ×2, `lab.css` ×2.
- **Cards** (`surface` + `radius-card` + padding): `jobs.css:11`, `libraries.css:20`, `:65`,
  `title.css:68` (`title__file`), `watch-together.css:18`, `:96`, `credentials-form.css:24`,
  `browse.css:137` (empty state, dashed), `device-activation.css:28`, `app-shell.css:325`,
  `warning-sign.css:43`, `modal-dialog.css:4`, `library-browse.css:67`. Paddings:
  `space-2/3`, `space-3/4`, `space-4`, `space-6/5`, `space-7/5`.
- **Device lists:** `DeviceList` (`your-devices.tsx`) and the cast target list
  (`cast-to.tsx:246`) both list a person's devices by name and last use, separately written; D364's
  top-bar "Cast" list will be a third.

### 3.5 Form fields, labels and problem text

- **Labels:** 58 `<label>` elements wrap their control; one uses `htmlFor` (`series.tsx:190`). The
  wrapping label's layout is set once, by `app-shell.css:338` for every label in `main`. Good.
- **Password fields:** `PasswordField` (7 uses); `people.tsx:244` and `system-discovery.tsx:260`
  write the same field by hand.
- **Saved-setting checkbox:** `OwnChoiceSwitch` (`sound.tsx:41`) toggles, keeps it on the server
  and shows the failure as an alert. `autoplay-setting.tsx:15`, `account.tsx:95` (admin away) and
  `left-out-libraries.tsx:76` do the same by hand, showing the failure as an `<output>`.
- **Number fields:** 4 (`candidate-chooser.tsx:72`, `create-library.tsx:355`,
  `library-manage.tsx:267`, `system-work.tsx:155`), plain and alike.
- **Problem text:** `<p role="alert">{problem}</p>` written out 49 times in 34 files, 23 of them
  the same line `problem === undefined ? null : <p role="alert">{problem}</p>`; `<output>{problem}</output>`
  6 times; `CredentialsProblem` (alert or quiet); `settings__problem`. Which one a failure gets is
  chosen per file, not by what kind of failure it is.

### 3.6 Notes, status cues, empty and loading states

- **Status notes:** `status-note` (`status-cues.tsx:82`, `still-reading.tsx:47`,
  `uncertain-match.tsx:42`) is shared. `app-shell__note` (status-blue edge) is shared by
  `local-network-note.tsx` and `plain-http-note.tsx`.
- **Edged callouts** (a panel with a coloured edge): `app-shell__note` (1px status border),
  `modal-dialog` (3px status top), `warning-sign__detail` (4px warning start),
  `device-activation__warning` (6px warning start), `together-voice__headphones` (warning top).
  One look, four edge widths.
- **Muted secondary text:** `color: var(--color-muted)` in 35 rule blocks; at least
  `play__reason`, `levelling__note`, `together-voice__hint`/`__reason`, `libraries__summary`,
  `title__facts`, `jobs__job > span` and `lab__small` are the same "quiet line".
- **Empty states:** `browse__empty` (dashed panel) used by `browse.tsx:314` and, across files, by
  `library-browse.tsx:211`, `:557`, `:641`; every other empty list is a bare `<p>`.
- **Loading:** `Loaded` (34 uses) shows `<p className="title__loading">`, a class from `title.css`
  (`title.css:122`) that holds the delayed fade-in and pulse. `app.tsx:216`, `cast-to.tsx:226`,
  `status-tray.tsx:145` and `end-of-list.tsx:38` write `t("common.loading")` bare, with no delay
  or pulse; `cast-to.tsx:218` (`Targets`) re-implements `Loaded`'s loading/failed/empty switch by
  hand.

### 3.7 Progress, counts, menus, tabs and focus wrappers

- **Progress:** `<progress>` twice (`jobs.tsx:131`, `poster-card.tsx:65`), styled apart. Fine.
- **Menus:** the app menu (`BannerMenu`) and the Watch Together tray panel (`ModalDialog`
  drop-down) are two pop-up mechanisms in the same banner, side by side: one closes on focus
  leaving and keeps the page usable, the other is modal. D364's "Cast" top-bar item and tray icon
  will need one of them; the choice should be made once.
- **Tabs:** no `role="tab"` anywhere. The top bar's destinations are links with an underline on
  `aria-current` (`app-shell.css:122`); the Needs-a-match filter is `aria-pressed` buttons. Both
  are fine as written, but the second needs its pressed look (§3.1).
- **Focus wrappers:** `data-focus-container=""` written by hand 13 times; there is no component
  for it, which is acceptable (an attribute on the element that is the row or grid).
  `data-focus-arrows-are-commands` once (`tv-controls.tsx:236`).

### 3.8 Player overlay controls

These matter most: D364 (casting as a session) and the computer's own controls will add the next
ones.

| Surface | File | Play/pause | Seek | Timeline | Volume | Full screen | Sound-on tap | Look |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| TV overlay | `tv-controls.tsx:96` | round button, glyph `▶`/`❚❚` | arrows on the scrub bar | `ScrubBar` (`:216`): seconds, `data-focus-arrows-are-commands`, no debounce | none | `FullScreenButton`, glyph `⤢`/`⤡` | n/a | `#fff` on `rgb(0 0 0 / 85%)` gradient, `rgb(255 255 255 / 12%)` buttons |
| Cast remote | `cast-remote.tsx:222` | plain button, words | plain buttons "Back 10 s"/"Forward 10 s" | `Timeline` (`:285`): ms, debounced by `scrubPauseWindowMs`, no touch fix | none | n/a | n/a | page look |
| Up next / Still watching | `up-next.tsx:227`, `still-watching.tsx:71` | n/a | n/a | n/a | n/a | n/a | n/a | `#fff` on `rgb(0 0 0 / 80%)`; `×` glyph |
| Watch Together viewer | `together-viewer.tsx:631` (`FilmButtons`), `:286` (`VolumeSlider`) | browser's | browser's | browser's | `VolumeRange` in a hand label | own `enterFullscreen` (`:666`), words | `together-viewer__sound-on` button | base button, `opacity: 0.9` |
| Host's player | `together-host.tsx:175` (`SoundOn`), `film-volume-slider.tsx:33` | browser's | browser's | browser's | `FilmVolumeSlider` | browser's | plain button | page look |
| Cast receiver | `cast-receiver.tsx:303` | TV overlay | | | | | `<output>` line, `rgb(0 0 0 / 70%)` | its own over-video box |
| Voice | `together-voice.tsx:278` | n/a | n/a | n/a | partner `VolumeRange` in a hand label (`:318`) | n/a | `together-voice__tap` | round Talk, status fill |
| Under the video | `play.tsx:338` `UnderTheVideo` | n/a | Start over | n/a | n/a | n/a | n/a | page look; native selects (§3.2); two different steppers (§1) |

Three ways into full screen: `fillTheScreen` on the stage (`film-fullscreen.ts`, used by
`FullScreenButton` and the TV overlay), `requestFullscreen` on the `<video>` with the iPhone's
`webkitEnterFullscreen` fallback (`together-viewer.tsx:666`), and `requestFullscreen` on the
document (`cast-asked.tsx:45`). The receiver's whole-page request has a reason (the stage does not
exist yet, D358); the viewer's is the only one with the iPhone fallback, which `fillTheScreen` lacks.

Over-the-video colours are raw: `#fff` ×3, `#000` ×4 (while `--color-video-background` exists),
`rgb(0 0 0 / 55%)` ×3, `/ 60%`, `/ 65%`, `/ 70%`, `/ 80%` ×2, `/ 85%`, `rgb(255 255 255 / 12%)`
×2, across `tv-controls.css`, `up-next.css`, `cast.css`, `poster-card.css`, `modal-dialog.css` and
`play.css`. `cast.css:16` and `:84` use `--space-2` as a radius.

### 3.9 CSS in general

- **Tokens:** colour, space, radius and size tokens are used nearly everywhere outside the player
  surfaces above. Raw lengths are mostly sizes of things (icon boxes, card widths), which have no
  tokens to use; `volume-range.css:5` writes `44px` for what `--size-control` (2.75rem) already is.
- **Repeated blocks:** the base button block repeated for a link (`libraries.css:43`); the labelled
  slider row three times (`film-volume-slider.css`, `together-viewer.css:63`,
  `together-voice.css:14`); the list reset 21 times; the card 13 times; the pop-up panel three times;
  "a button is as wide as its words" twice (`play.css:36`, `together-voice.css:10`).
- **Classes used across components:** `title__loading` (in `Loaded`), `browse__empty` (in
  `library-browse.tsx`), `poster-grid`/`poster-card__*` (in `ui-gallery.tsx`), `tv-controls__full-screen`
  (in `full-screen-button.tsx`), `play__reason` (in `frame-timing-note.tsx`, `sound-delay.tsx`),
  `up-next__play` (in `still-watching.tsx`). Each is a shared widget waiting for a name of its own;
  the typescript standard prefixes a class with its component's file.

## 4. The proposed standard set

The principle, as for `Icon`: one component (or one base class, where a component adds nothing)
per kind, its props the variants the app really uses, its look from tokens, and a source-reading
test that fails when a screen writes the kind by hand.

### 4.1 The set

| Widget | Kind | Props | Replaces |
| --- | --- | --- | --- |
| **Base button classes** in `base.css`: `.button--primary`, `.button--quiet`, `.button--status`, `[aria-pressed="true"]` | look | none (classes) | `libraries__create` fill, `watch-together__start`, `device-activation__match`, `together-voice__tap`, `continue-watching .poster-card__action`, the two `quiet`, `lab__button[aria-pressed]`; gives `identify.tsx:137` its pressed look |
| **`ButtonLink`** | component over `Link` | `to`, `variant?: "primary"`, `label?` | `libraries__manage`, `libraries__create`, `title__play` |
| **`IconButton`** | component | `label` (accessible name, required), `icon`, `size?: "control" \| "player"`, `tip?` (the settings tooltip), `isPressed?`, `isExpanded?`, `onClick`, `ref` | `settings__control`, `status-tray__button`, `warning-sign__button`, `up-next__dismiss`, `tv-controls__play`, `tv-controls__full-screen` (with play, pause, full-screen, leave and close drawn as `Icon`s) |
| **`ConfirmButton`** (exists) gains `trigger?: ReactNode` for an icon first step | component | as today + `trigger` | `settings.tsx:84` `RemoveQuestion` |
| **`Choice`** (from `RouteChooser`, exported) | component | `label`, `isLabelShown?`, `options: { key, label, isCurrent }[]`, and either `to` per option (a route, as today) or `onChoose(key)` | the 11 native selects; `SortChoice`; `RouteChooser` itself |
| **`ModalDialog`** (exists) | component | as today | no change; becomes the one pop-up for `WarningSign`'s detail and, if the owner chooses, the banner menu (§5 Q2) |
| **`Panel`** | component or class | `edge?: "status" \| "warning"`, `tone?: "raised" \| "plain" \| "dashed"` | the 13 card blocks; the 5 edged callouts; `browse__empty` as `EmptyState` over it |
| **`Stack`, `Cluster`** | layout components (ui-architecture R2) | `space` (a step name), `as?: "ul" \| "div"` | the 21 list resets and most `display: flex; flex-wrap: wrap; gap` rows |
| **`LabelledRange`** | component over `VolumeRange` | `label`, `value`, `most`, `onChange` | `FilmVolumeSlider`'s label, `together-viewer.tsx:286`, `together-voice.tsx:318` and their 3 CSS blocks |
| **`Stepper`** | component | `earlierLabel`, `laterLabel`, `value` (text), `onStep(direction)` | `subtitles.tsx:235`, `sound-delay.tsx:74` |
| **`SettingSwitch`** (`OwnChoiceSwitch`, generalised) | component | `label`, `isOn`, `onSwitch(isOn): Promise<void>` | `autoplay-setting.tsx`, `account.tsx:95`, `left-out-libraries.tsx:76` |
| **`Problem`** | component | `children`, `isAlert?` (default true; false for what happened before the person arrived) | the 49 `<p role="alert">`, the 6 `<output>{problem}`, `CredentialsProblem`, `settings__problem` |
| **`Loading`** | component | none | `Loaded`'s line (moving `title__loading` to `loading.css`), the 4 bare lines; `cast-to.tsx:218` uses `Loaded` |
| **`Note`** | component | `children` | the "quiet line": `play__reason`, `levelling__note`, `together-voice__hint`, `lab__small` and the like |
| **Player set** | §4.2 | | |

### 4.2 The player set, first

| Widget | Props | Replaces | Built on |
| --- | --- | --- | --- |
| **Over-video tokens** in `tokens.css`: `--color-on-video`, `--color-video-scrim` (strong, for a bar), `--color-video-card` (for Up next, the receiver's lines), `--color-video-control` (a control's ground), and the existing `--color-video-background` | tokens | the raw values of §3.8 | |
| **`PlayerButton`** (= `IconButton size="player"`) | `label`, `icon`, `onClick`, `ref` | `tv-controls__play`, `tv-controls__full-screen`, `up-next__dismiss`, the cast remote's play/pause and ±10 s (as icons with names) | `IconButton`, `Icon` |
| **`Timeline`** | `atMs`, `durationMs`, `onSeek(ms)`, `isDisabled?`, `settleMs?` (debounce; 0 = each step), `arrowsAreCommands?` (a TV overlay), units always ms; the touch-anywhere handling of `VolumeRange` | `tv-controls.tsx:216`, `cast-remote.tsx:285`; the computer's controls and D364's remote | `volumeAt`'s touch code, generalised |
| **`ClockTimes`** | `atMs`, `durationMs` | the two `x / y` spans | `clockText` (exists) |
| **`FullScreenButton`** (exists) | `target?: "stage" \| "video"` | `together-viewer.tsx:646` and its `enterFullscreen`; `fillTheScreen` gains the iPhone's `webkitEnterFullscreen` | `film-fullscreen.ts` |
| **`SoundOnButton`** | `onTap` | `together-host.tsx:175`, `together-viewer.tsx:641`, `together-voice__tap` (status fill) | `.button--status` |
| **`OverVideoCard`** | `children` | `up-next__play`'s card (and `still-watching__card`, which borrows it), `cast-receiver__note`, `cast-receiver__sound-on` | the tokens |
| **`PlayerControls`** (the bar) | `videoRef` or a remote's `connection`, `controls: ("play" \| "seek" \| "timeline" \| "volume" \| "fullScreen" \| "tracks")[]` | `TvControls`' layout; then the computer's own controls (replacing Chrome's) and the cast remote's | all of the above |

`PlayerControls` should take its state from one source: `useStanding(videoRef)` for a local video
(`tv-controls.tsx:353`) or `usePositionNow(snapshot)` for a remote (`cast-remote.tsx:201`), behind
one interface `{ atMs, durationMs, isPaused, play(), pause(), seekTo(ms) }`. The TV overlay, the
computer's controls and the cast remote are then one bar with three sources, and D364's remote (a
page the person returns to from anywhere, §8) is that bar again.

### 4.3 Risk of each replacement

| Replacement | What differs today | Tests that pin it |
| --- | --- | --- |
| Primary/quiet classes | `title__play` has more padding and no border; Hide has no hover ground | `title.test.tsx`, `input-pages.browser.test.tsx`, `input-player.browser.test.tsx` find `.title__play` (keep the class on the link as well, or move them to role + name) |
| `IconButton` for `status-tray__button` | narrower (2.25rem) on purpose (the owner, 2026-10-11) | `app.test.tsx` ×4, `ui-gallery.test.tsx` |
| `IconButton` for the TV overlay | glyph to `Icon` changes what the remote shows; size stays 3rem | `tv-controls.test.tsx` (`tv-controls__full-screen`), VQ61/VQ62 on devices |
| `IconButton` for `warning-sign__button` | prevents the click's default and stops its propagation | `warning-sign.test.tsx` |
| `ConfirmButton` with an icon trigger | the settings row disables the bin while asking, and puts the question on the row's full width | `confirmed-actions.test.tsx` ("removes a stored secret only when asked again") |
| `Choice` for the track menus | a native select's change is one event; the drop-down's is a press then a close; focus returns to the button | `subtitles.test.tsx`, `sound.test.tsx` (audio choice asks for the film again), `input-player.browser.test.tsx`; the controller's `showPicker` path in `focus-navigation.ts:230` then serves only the admin's forms |
| `Choice` for `SortChoice`, `OrderingChoice` | `OrderingChoice` disables while busy and navigates after | `browse.test.tsx`, `episode-orderings.test.tsx`, `series.test.tsx` |
| `Timeline` | TV: seconds, each change seeks; remote: ms, settles after `scrubPauseWindowMs` (300 ms, the SDK's); the TV's arrows are commands | `tv-controls.test.tsx`, `input-player.browser.test.tsx` (`tv-controls__scrub`), `cast-remote.test.tsx` |
| `FullScreenButton` for the viewer | the viewer's is a worded button shown only without the browser's controls | `together-viewer.test.tsx` |
| `Problem` | 6 places change from a silent `<output>` to an alert, or must pass `isAlert={false}` | `screen-refusals.test.tsx` finds failures by role; the tests of the 6 screens that use `<output>` (`account`, `autoplay-setting`, `identify`, `libraries`, `left-out-libraries`, `match-correction`) need reading for how they find a failure |
| `Loading` | the 4 bare lines gain a 0.4 s fade-in before they show | none found by text; a test reading the line's presence still sees it (the delay is an animation) |
| `Panel`, `Stack`, `Cluster` | paddings and gaps unify to a few steps: a visible change, small | none by class; the browser walks measure geometry (`input-walk.browser.test.tsx`) |
| `WarningSign` on `ModalDialog` | becomes modal and gains Escape and Back; its pop-up moves from beside the triangle to the drop-down's place | `warning-sign.test.tsx`, `app-shell.browser.test.tsx` |

### 4.4 The order

1. **Over-video tokens, `IconButton`/`PlayerButton`, the play, pause, full-screen and close icons.**
   Small, and every later player step uses them.
2. **`Timeline` and `ClockTimes`**, then the TV overlay and the cast remote on them. This is where
   behaviour differs most, and D364's remote reuses it.
3. **`PlayerControls` with its two sources**, the TV overlay rebuilt as its first user. The
   computer's own controls and D364's remote page are then built on it, not beside it.
4. **`Choice`**, and the Play page's three track menus on it, then `SortChoice` and
   `OrderingChoice`, then the admin's selects.
5. **`SoundOnButton`, `OverVideoCard`, `LabelledRange`, `Stepper`, `FullScreenButton` for the
   viewer** — the rest of the player surfaces.
6. **Base button classes and `ButtonLink`**, `ConfirmButton`'s icon trigger, the toggle's pressed
   look.
7. **`Problem`, `Loading`, `Note`, `SettingSwitch`.** Many files, each change mechanical.
8. **`Panel`, `Stack`, `Cluster`**, last: the widest change and the least behaviour.

Each step adds its widgets to `/ui-gallery`, rendered by the component rather than by copied
markup, and the gallery's poster and tray sections are switched to render the real components.

### 4.5 Checks that would hold it

Like `icon.test.tsx`, each is a test in `web/apps/web/src/` that reads the sources (no gate file
changes; an ESLint `no-restricted-syntax` selector would do the same job but is a gate, the owner's):

- **`select.test.tsx`:** no `<select` outside `choice.tsx` and `lab*.tsx`.
- **`dialog.test.tsx`:** no `<dialog`, `showModal(` or `role="dialog"` outside `modal-dialog.tsx`
  (true today; this keeps it).
- **`button.test.tsx`:** in the stylesheets, no rule outside `base.css` and `icon-button.css`
  sets a button's `background` to `--color-accent`, `--color-status` or `--color-irreversible`; no
  `className="irreversible` outside `confirm-button.tsx`; no `<button` whose content is a single
  non-letter character (a glyph icon).
- **`range.test.tsx`:** no `type="range"` outside `volume-range.tsx` and `timeline.tsx`.
- **`problem.test.tsx`:** no `role="alert"` outside `problem.tsx` (and `modal-dialog.tsx`'s
  `alertdialog`), no `t("common.loading")` outside `loading.tsx`.
- **`video-colours.test.tsx`:** no `#fff`, `#000` or `rgb(` in a stylesheet outside `tokens.css`
  and the gallery's sample posters.
- **`class-owner.test.tsx`:** a class written in a `.tsx` file starts with that file's own name
  or a shared widget's (the typescript standard's prefix rule, review-only today), which catches
  `title__loading` in `loaded.tsx` and `browse__empty` in `library-browse.tsx`.

Each test has the icon test's fourth case: one that feeds it a stray and expects it found, so the
check can fail.

## 5. Questions for the owner

1. **The status tray's narrower buttons and the TV overlay's round ones** are deliberate sizes.
   Should `IconButton` have exactly two sizes (control and player), or keep the tray's 2.25rem as
   a third?
2. **One pop-up for the banner.** The menu closes when focus leaves and leaves the page usable; the
   Watch Together panel is modal. D364 adds "Cast" beside them. Should all banner pop-ups be the
   modal drop-down, or all non-modal?
3. **The warning sign's detail:** move it to a modal drop-down (it gains Escape and a remote's
   Back) or keep it as an inline pop-up and add Escape and Back to it?
4. **"Approve removals"** (`library-manage.tsx:151`) is filled red with no confirmation. Is it
   irreversible, and so a `ConfirmButton`?
5. **Primary buttons:** `title__play` is larger than the other filled buttons. Is the Play link a
   size of its own (`size="large"`), or should it match?
