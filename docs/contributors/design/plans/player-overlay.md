# Plan: the player's controls and overlay as D407 has them (2026-11-02)

For the owner's go. D407 is in
[working-notes.md](../../decisions/working-notes.md); the research behind it is
[player-controls-2026-11-02.md](../research/player-controls-2026-11-02.md). Paths are under
`web/apps/web/src/` unless given in full. Nothing here is built. The owner answered §5's questions on 2026-11-02; D407 records the answers.

## 1. Today vs D407, per device

| Device | Today | D407 | Where the change lands |
|---|---|---|---|
| **TV (Silk, `isTelevision`)** | `TvControls` (`tv-controls.tsx:101`) opens the stage full screen (`useFullscreenStage` :306). The top row is "‹ S1E1" Up plus an on-screen Back (`OverlayUpAndBack`, `play-up.tsx`). Then one row: play, "Ends at", full screen (:147–157), with the timeline below. States are `none`/`scrub`/`all` (:24). It hides 5 s after the last press while playing (`overlayShownForMs` :21), never while paused (`useHiding` :229). While shown, arrows seek only on the scrub bar (:74–83). Up next takes OK/Left/Right when it has focus (:192–209). | Three rows: ‹ + title + convenience; seek bar with times; buttons ⏮⏪▶⏩⏭ … 💬 🎧 ⁝ ⤢ (no 🔈 on TV). States: hidden, convenience alone, scrub alone, paused, full. Hides 3 s after the last input while playing; never while paused or with a menu open (D407 §6). Up/Down move between rows and each row remembers its focus. First Back hides the overlay. Not full screen: controls below the video, nothing over it. | `tv-controls.tsx` is replaced by a new overlay used on every device in full screen. The pure answer `overlayAnswer` (:48) becomes a fuller state machine. |
| **Computer** | `PageControls` (`page-controls.tsx`): a bar over the film with play, full screen, sound and timeline. Hides after 5 s while playing (`useShownWhileActive` :71). Click plays/pauses, double click goes full screen (`usePressesOnTheFilm` :150). Down on the video enters the bar. Space plays/pauses anywhere in the player's controls (`useSpacePlaysAndPauses`, `player-commands.ts`). Seeking grows 10→30→60 s (`seek-ahead.ts`). Subtitles, audio and the rest live under the film (`play.tsx` `UnderTheVideo` :379) and cannot be reached in full screen. | Full screen: the same overlay and keys as TV, plus 🔈. Space presses the focused button. `+`/`=`/`−`/numpad change the volume. Media keys: play/pause, and next/previous change the episode. Not full screen: controls below the video, nothing over it. | `page-controls.tsx` becomes the below-video layout. `player-commands.ts` (Space). `input-buttons.ts`/`input-commands.ts` (volume, next/previous). |
| **Phone/tablet** | The computer's bar. Volume only where it can be set (`useIsVolumeSettable`, not iOS). A tap shows the bar. | The same overlay, no 🔈 on touch. A tap shows the whole overlay. Touch gestures are deferred. | The visibility rule uses `isThisHandheld` (`handheld.ts`) and `useIsVolumeSettable`. |
| **Cast receiver** | The TV overlay without Up/Back (`pageComposition`, `play.tsx:254`: `hasTelevisionControls = isTelevision \|\| receiving`). The mouse opens it (`useOpensAtThePointer` :270). | §12: the overlay without ‹ and without menus. | Q6. |
| **Cast remote** | `SessionRemote` (`session-remote.tsx`), the audio menu and `RemoteUpNext` in `cast-remote.tsx` (`RemoteOf` :317, :487). Always shown. | §12: unchanged. | Unchanged. Q6. |
| **Watch Together** | Host: their own player, with sound and voice under it. Link guest: the browser's own `<video controls>`, or a Full screen button when they have no rights (`together-viewer.tsx:725–757`). | §12: the host gets the overlay; the link guest keeps the browser's controls. | The host gets the new overlay through the shared player. The guest is unchanged (D369's waiting item). Q7. |

## 2. Components and states

### 2.1 Modules

Each stays under 800 lines, with functions of at most 60 lines and complexity at most 10.

- **`player-overlay-state.ts`** (new, pure, replaces `overlayAnswer`):
  - `OverlayState = { shown: "hidden" | "convenience" | "scrub" | "paused" | "full"; row: 0 | 1 | 2; menuOpen: boolean }`;
  - `overlayAnswer(state, command, ctx)` returns `{ state, isTaken, seekByMs?, focus?: "play" | "row", press?: "convenience" }`;
  - `hideAfter(state)` returns the delay, or `undefined`.
- **`player-overlay.tsx`** (new): `PlayerOverlay(props: FilmParts)`, used in full screen on every device.
  - It draws three row containers, each with `data-focus-container` and a ref remembering its focus.
  - The bodies come from `PlayerControls`.
  - It uses `useOverlayHiding`, which replaces `useHiding`, and `useRowFocus`.
- **`player-controls.tsx`**:
  - `PlayerControl` gains `"previous" | "next" | "subtitles" | "audio" | "more"`.
  - The buttons row splits into a `leading` group (⏮⏪▶⏩⏭) and a `trailing` group (🔈💬🎧⁝⤢), so both layouts draw from the one component (rule 11).
  - The timeline is no longer forced last; the overlay places it as its own row.
- **`player-tracks.tsx`** (new): `PlayerTracksContext`.
  - It lifts the chosen subtitle and audio tracks and their choosers out of `UnderTheVideo`, `DirectUnder` and `LadderUnder` (`play.tsx:379/476/512`). The piece drawn over the film has only `FilmParts`, so the overlay's 💬 and 🎧 need this context.
  - It also takes code out of `play.tsx`, which is at 784 lines.
- **`player-menus.tsx`** (new): the 💬, 🎧 and ⁝ openers.
  - They reuse `ActionChooser` with `openerIcon` (`route-chooser.tsx`) over `DropDown` (`drop-down.tsx:280`).
  - Each reports whether it is open, so the timer can hold.
- **`convenience-area.tsx`** (new): a slot over the film showing one of Up next, Still watching or, later, Skip intro.
  - With nothing on offer and the overlay full, it shows `EndsAt`, moved from `tv-controls.tsx:180–221`.
  - Below the video, the same offers are drawn among the controls.
- **`player-icons.tsx`**: adds `PreviousIcon`, `NextIcon`, `SubtitlesIcon`, `AudioIcon`, `MoreIcon` and `BackIcon` (for ‹), drawn through `Icon` and never as glyph text.
- **`full-screen-button.tsx`**: exports `useIsFullScreen`, widened to "the stage is full screen or fills the window" (`fillsWindowClass`, `film-fullscreen.ts`). It decides which layout applies.

### 2.2 States and transitions

| State | Draws | Entered by | Left by |
|---|---|---|---|
| hidden | nothing | the timer; Back from full | Left/Right → scrub. OK toggles play (→ paused when pausing). Up/Down, mouse movement or a tap → full, focus on ▶ |
| convenience | the convenience area alone | Up next, Still watching or Skip intro offered while hidden | OK presses the offer, with no guard. Up/Down → full. Left/Right seek and show the scrub bar; the offer stays (Q3) |
| scrub | line, position and length; the position shows the seek target | Left/Right while hidden | 1500 ms after the seek lands (Q1) → hidden |
| paused | a pause sign over the video, and the scrub bar | OK pausing while hidden | 3 s after the last input (Q2) → hidden, still paused; OK plays → hidden |
| full | the three rows | Up/Down, the pointer or a tap from hidden; the first input while paused | by §2.4's rule (D407 §6); the first Back → hidden, or in real full screen leaving full screen (§2.5) |

Rules for every state:

- `playPause`, `seekBack` and `seekForward` stay the player's in every state, as today (:53).
- Subtitles move up while `shown === "full"`.
  - JASSUB's layer comes from `useSubtitleLayer` (`subtitles.tsx:273`); native `::cue` uses `line`.
  - Both take a `data-overlay-shown` attribute on the stage, with a CSS offset.
- The convenience area becomes row 0, so it is no longer "another control". Its `isOnAnotherControlOverTheFilm` carve-out (`tv-controls.tsx:200`) moves inside the overlay.

### 2.3 Rows and remembered focus

The rows are: row 0, ‹ (`UpLink`) and the convenience area; row 1, the timeline; row 2, the buttons.

- `useRowFocus` keeps `lastFocused[row]`, updated on `focusin`.
- The overlay takes Up and Down (`isTaken`). They focus `lastFocused[row±1]`, or that row's first button (▶ for row 2).
- The focus engine handles Left and Right inside a row; on row 1 they seek.
- Entering full from hidden focuses ▶ (`useFocusToPlayOnceDrawn` :167 stays).
- `OverlayUpAndBack` has an on-screen Back, but D407's row 0 has only ‹. TV Bro's Back never reaches the page (README:350–368). The on-screen Back goes (Q5).

### 2.4 Timer

`useOverlayHiding(state, lastInputAt)` replaces `useHiding` (:229) and `useShownWhileActive` (`page-controls.tsx:71`).

- The delay is 3000 ms for full (`overlayShownForMs = 3000`), 1500 ms for scrub and 3000 ms for paused.
- Full never hides while paused or while `menuOpen` (D407 §6).
- How full was shown is kept (`shownBy: "remote" | "pointer"`, from `isTelevision` and the input's source):
  - shown by a remote, it hides 3 s after the last press while playing, focus or not;
  - shown by a mouse or a tap, it also holds while focus a key put there (`:focus-visible`) is in the overlay (`focusin`/`focusout`; D409).
- A computer's keyboard in full screen: pressing ▶ to play, or Down from the bottom row, moves focus to the video, so the timer runs (Q14).
- On hiding, focus in the overlay moves to the video, as today (:247–251).

### 2.5 Key map

The keys sit in one table in `input-buttons.ts` (rule 11).

- **Remote and keyboard:** the arrows, OK/Enter and Back/Escape (`commandOf`), and MediaPlayPause 179, as today.
- **Space:** presses the focused button while full, and plays/pauses while hidden or on the video. This changes `useSpacePlaysAndPauses` in `player-commands.ts`, which the README pins at 544–592.
- **New `Command`s:** `volumeUp`, `volumeDown`, `previous` and `next`.
- **Volume keys** (research §10.4, option A):
  - up is `+` (Shift allowed), `=` without Shift, or `NumpadAdd`;
  - down is `-` without Shift, or `NumpadSubtract`;
  - guards:
    - an editable target;
    - `isComposing`, or key code 229;
    - Ctrl, Meta, Alt, or `getModifierState("AltGraph")`.

    The existing `isLeftAlone` and `keysCommand` cover most of these.
  - Layouts where `+` needs a modifier (Swiss, Hungarian, Turkish Q, JIS) fall out of the guards.
  - The existing `AudioVolume*` keys stay the system's own.
- **Media Session:** `hearMediaSeeks` (`input-commands.ts:249`) registers `nexttrack` and `previoustrack` only while there is a next or previous, because Chrome shows those buttons once they are registered. Play/pause stays in `@orpheum/player`'s `event-context.ts`.
- **Back in real full screen** never reaches the page on Fire TV, or as Escape on a computer (idiosyncrasies:1331). Full screen stays the browser's real one (Q4, D407 §7). On `fullscreenchange` to not full screen, the page shows the below-video layout with focus on ▶; ⤢ returns, and the next Back goes to the info page.

### 2.6 Which controls show (D407 §2)

One function, `controlsShown(ctx)`, unit tested:

- ⏮ and ⏭: only within a sequence (`hasPrevious`, `hasNext`). A sequence is the set of videos being shown; for now a series' episodes in order without extras, later sets such as a collection (Q9).
- 🔈: not on TV or touch, and only where `useIsVolumeSettable`.
- 🎧: only with more than one audio track, matching `AudioMenu` returning null (`audio-tracks.tsx:96`).
- 💬 and ⤢: always, ⤢ on TV too.
- ⏪, ⏩, ▶ and ‹: always (‹ not on the cast receiver).

### 2.7 Menus (D407 §8)

- 💬 lists Off and then the tracks, reusing `trackLabel`, `startingTrackKey` and `keepSubtitleChoice` from `subtitles.tsx`, and then Find subtitles (Q8).
- 🎧 lists the tracks (`audioTrackLabel`, `keepAudioChoice`).
- ⁝ holds Start over, Subtitle offset, Sound offset, Levelling, Dialogue boost, Play original and the bare player; the debug tools stay under the film (Q8).
- The native `<select>`s of `TrackMenu` (`subtitles.tsx:204`) and `AudioMenu` give way to `DropDown` lists, the one widget.

### 2.8 Outside full screen (D407 §9)

`PageControls` is drawn in `under`, not `over` (`FilmComposition`, `film-player.tsx`).

- Its rows are the timeline; the buttons; and the offers (Up next, Still watching, Skip intro) as ordinary buttons.
- Nothing is drawn over the video: no pause sign, no scrub bar, no convenience area.
- Keys while the video has focus: Left/Right seek, OK plays (`player-commands.ts`), Up/Down move the focus.
- `usePressesOnTheFilm` (click plays or pauses, double click goes full screen) stays (Q10, D407 §9). "Ends 9:41" shows among the controls when nothing is on offer (Q13).

## 3. Steps

The work goes on branch `work/player-overlay` (in this sandbox, on `main`). `make spot` runs between steps, `make check` at each step's end, and `make check-pr` at the end.

| # | Commit | Files | Tests | VQ | Size |
|---|---|---|---|---|---|
| 1 | `feat(lab): record a key's modifiers, composing and full-screen leave in the button test` | `lab-keys.ts` (shift/ctrl/alt/meta/AltGraph, `location`, `isComposing` in `KeyEntry`/`heardKeyOf` :50); `lab-input.tsx` (`fullscreenchange`, `contextmenu`); `locales/en-lab.ts`; README:520–543 | `lab-keys.test.ts` (modifiers read); `lab-input.browser.test.tsx` (Shift+`=` logged with Shift) | VQ242: on Fire TV, record Menu (adb keyevent 82), Rewind 89, FF 90, and Back with the stage full screen; on a Mac, the ⏪⏩ media keys; on a computer, +/− with Swiss, Turkish and JIS layouts | S (~80 lines) |
| 2 | `feat(input): volume keys, and next and previous as commands` | `input-buttons.ts` (the `Command` union, the key table); `input-commands.ts` (guards; `hearMediaSeeks` :249 gains next/previous behind a flag) | `input-buttons.test.ts` (keys × modifiers × layouts, AltGr as Ctrl+Alt included); `input-commands.test.ts` (Media Session actions registered only when asked) | none | S–M (~150) |
| 3 | `refactor(player): track choices in a context the overlay can reach` | new `player-tracks.tsx`; `play.tsx` (`UnderTheVideo`, `DirectUnder`, `LadderUnder` publish into it); `subtitles.tsx`; `audio-tracks.tsx` | The existing tests stay unchanged; one new test that the context sees a choice made below the film | none | M (~200 moved; `play.tsx` drops ~80) |
| 4 | `feat(player): icons and shared full-screen state for the overlay` | `player-icons.tsx`; `full-screen-button.tsx` (exports `useIsFullScreen`, counting fills-window) | `player-widgets.browser.test.tsx` (the new icons at 2.75rem); `full-screen-button.test.tsx` (fills-window counts) | none | S |
| 5 | `feat(player): the overlay's states and rows (D407 §4–§7)`, **which fixes item 17** | new `player-overlay-state.ts`; new `player-overlay.tsx`; `tv-controls.tsx` reduced to its full-screen-on-open hook or removed; `player-controls.tsx` (groups, new controls); `tv-controls.css` → `player-overlay.css`; `play.tsx`'s `overTheFilm` choice | Unit: `player-overlay-state.test.ts`, one test per transition in §2.2, "paused full never hides", "remote-shown full hides after 3 s while playing", "pointer-shown full holds while focused" and "an open menu holds" among them. Browser (`input-player.browser.test.tsx`, the Fire TV block :209): Up/Down walk the rows and return to the remembered button; Right on ⤢ while shown walks, never seeks; Left/Right while hidden seek and show the scrub bar alone; OK while hidden pauses and shows the pause sign and scrub bar; Back while full hides. :299 and :316 (Space) updated | VQ243: on Fire TV Silk, walk every row, hold Right while hidden, pause and hide, Back with and without the stage full screen | L (~500 with tests) |
| 6 | `feat(player): Space presses the focused control, and the overlay serves a computer's full screen` | `player-commands.ts` (`useSpacePlaysAndPauses`); `page-controls.tsx` (full screen → `PlayerOverlay`); `film-player.tsx` (`over`/`under` by full-screen state) | Browser: on a computer, Space on ⤢ toggles full screen, not play; Space on the video plays. `page-controls.test.tsx:166` (5 s) rewritten for the layout | VQ244: full screen, Escape, mouse and Space on Chrome, Firefox and Safari on a computer | M |
| 7 | `feat(player): subtitles, audio and more in the overlay (D407 §1, §8)` | new `player-menus.tsx`; `play.tsx`/`UnderTheVideo` lose the `Subtitles` menu and `AudioMenu`, and the rest goes under ⁝; `subtitles.tsx` (`TrackMenu` removed, the layer kept, subtitles move up via `data-overlay-shown`) | Unit: 💬 lists Off, the tracks and Find subtitles; 🎧 the tracks; the timer holds while one is open. Browser: 💬 opens a `DropDown` in Silk's full screen; Left/Right close it (the known Silk issue, drop-down idiosyncrasy 1444); focus returns to 💬. `player-widgets.test.ts` stays green: no stray `<select>` or icon button | VQ245: on Fire TV in full screen, choose a subtitle track and an audio track; the subtitles move up and back | L |
| 8 | `feat(player): the convenience area, Up next and Still watching alone (D407 §3)` | new `convenience-area.tsx`; `up-next.tsx` (`UpNextOver` :170, `overTheFilm` :200; `useChoiceOfPresses` :375's Back dismissing kept); `still-watching.tsx`; `EndsAt` moved | Unit: an offer while hidden shows the convenience area alone; OK presses it; "Ends" shows only in full with nothing on offer. `tv-controls.test.tsx:172/216` rewritten as overlay tests; `up-next.test.tsx` updated | VQ246: on Fire TV, Up next at the end with the overlay hidden, and OK takes it; Still watching likewise | M–L |
| 9 | `feat(player): previous and next in a sequence (D407 §1–§2, §10)` | server: `internal/core/api/up_next.go` (previous too, or a sibling endpoint; Q9), the `api/` spec, the generated client; web: `up-next.tsx`'s query, ⏮⏭, and step 2's Media Session flag | A Go handler test for previous. Web unit: the buttons show only within a sequence. Browser: the MediaTrackNext key and Media Session's `nexttrack` play the next episode | VQ247: a computer's media keys and the Mac's ⏪⏩ change the episode (using step 1's recordings) | M–L (server and web) |
| 10 | `feat(player): volume button and keys` | `player-controls.tsx` `SoundControls` (:171) behind 🔈 as a vertical pop-up (Q11); `volume-range.tsx`; step 2's commands wired in `player-commands.ts` | Unit: +, = and − change the volume by 5 % (Q12), modifiers ignored; no 🔈 on TV or touch | VQ248: Swiss and JIS keyboards | M |
| 11 | `feat(player): controls below the film outside full screen (D407 §9)` | `page-controls.tsx`; `film-player.tsx` (`Stage`/`under`); `play.tsx`'s `pageComposition` (:254); CSS. A TV out of full screen uses it too | Browser: when not full screen, nothing is drawn over the video (the stage holds only the video and the subtitle layer), at computer and Silk widths; the offers sit among the controls | VQ249: a phone in portrait and landscape; Fire TV after leaving full screen | M |
| 12 | `docs(web): the player's controls as D407 has them` | `web/apps/web/README.md` (350–368, 415–430, 544–592, 598–613); `verification-queue.md`; item 17's status in `navigation-and-widgets-answers-2026-10-27.md` | none | none | S |

**Order:**
- Steps 1 and 2 can land first, each on its own.
- Step 3 comes before 7.
- Steps 5 and 6 come before 7, 8 and 10.
- Step 9 needs step 2 and the server.

**Item 17.** Step 5 is its fix:
- The 5 s hide goes.
- Whether the overlay is shown decides what Right does, so Right on a shown overlay always walks to ⤢.

Two caveats belong in the item and in VQ243:
- Research §5.1 found that Silk never delivers Rewind or FF, so the owner's "FF did nothing" cannot be fixed in the page.
- A 3 s timer can still change what Right does between presses while playing; paused, the overlay stays.

## 4. The key-event test page (D407 §10)

The existing lab button test (`lab-input.tsx` with `lab-keys.ts`) is extended rather than a page added (rule 11). It is reached from the lab (`lab.tsx`) as today (README:520–543).

Already there:
- It logs keys, keyup, popstate and every Media Session action, `nexttrack` and `previoustrack` included (`lab-input.tsx:33–40`).
- It sends them to the core's log (`lab-events.ts`).

Step 1 adds what D407 §10 needs:
- the modifier flags, `getModifierState("AltGraph")`, `location` and `isComposing`, for the +/− layout rule;
- `fullscreenchange` and `contextmenu`, for Back in full screen and Silk's Menu key.

## 5. Risks and questions

### Risks

- **R1. Back in real full screen** never reaches the page on Fire TV, or as Escape on a computer (idiosyncrasies:1331). D407's "the first Back hides" and "Back goes to the info page" cannot be kept there. Q4.
- **R2. `play.tsx` is at 784 lines.** Step 3 must take code out before steps 7 and 11 add to it.
- **R3. A `DropDown` inside the full-screen top layer on Silk** brings three known issues:
  - the modal's focus loss (drop-down idiosyncrasy 1399);
  - its layout at the page's top (1417);
  - Left/Right not closing it (1444).

  Step 7's browser test and VQ245 cover them.
- **R4. The 3 s timer** still changes what the arrows do while playing: item 17's side finding. Paused, it no longer hides.
- **R5. Registering `nexttrack`** makes Chrome show the buttons in its media hub, so it is registered only within a sequence.
- **R6. Space changes** from "always plays" (the README pin, tests :316 and :437). The cast receiver's fix of 2026-10-27 relied on it, so the receiver's Space must still play when nothing has focus.

### The owner's answers (2026-11-02)

1. **The scrub bar lingers** 1500 ms after the seek lands. Agreed.
2. **The pause sign stays** 3 s. Agreed.
3. **Left/Right while the convenience area shows alone** seek and show the scrub bar, and the offer stays. Still watching's Stop (VQ69) is reached through the full overlay's row 0. The recommendation.
4. **Full screen is the browser's real one**, even if that limits what the page can do (the owner). Leaving it unasked shows the below-video layout with focus on ▶. The stage never only fills the window.
5. **The on-screen Back for TV Bro** goes; ‹ remains. Agreed.
6. **The cast receiver** uses the overlay without ‹ and menus; **the cast remote** is unchanged. Agreed.
7. **Watch Together:** the host gets the overlay; the link guest keeps the browser's controls. Agreed.
8. **Find subtitles goes in the 💬 menu** (the owner); ⁝ holds the rest as §2.7 lists.
9. **A sequence is the set of videos being shown** (the owner): for now a series' episodes in order without extras; later sets such as everything of Metropolis. `up_next.go` returns previous as well as next.
10. **Click and double-click on the film** stay. Agreed.
11. **The volume pop-up** is vertical. Agreed.
12. **The +/− step** is 5 %. Agreed.
13. **"Ends 9:41" outside full screen** shows among the controls when nothing is on offer. Agreed.
- **Hiding** follows the accessibility guidance, with the remote exempt from the focus rule (D407 §6, §2.4).
- **Keyboard and touch:** the minimal option. No letter shortcuts, and a tap only shows the overlay (D407 §11).

14. **A computer's keyboard in full screen.** Up or Down puts focus on the scrub bar (D410; ▶ before it), and focus in the overlay holds it (D407 §6), so a keyboard user's overlay would stay until Escape leaves full screen. Pressing ▶ to play, or Down from the bottom row, moves focus to the video, which lets the 3 s timer run. Agreed.

## 6. The audit before building (2026-11-02)

An independent read of this plan against the code found the following. Each fix binds the step named, over what §2–§3 say.

### Things that would break

- **The cast remote's sound controls** (step 3). `AudioMenu` (`audio-tracks.tsx:118`), `BoostSwitch` and `LevelSwitch` (`sound.tsx:67`) register with the sound panel through `useSoundPart` while mounted; the remote reads and sets the receiver's track, boost and levelling through them (README:206–210). A `DropDown` mounts its contents only while open (`drop-down.tsx:307`), and the receiver has no menus. Step 3 moves the registrations and the choice state, boost and levelling included (`play.tsx:530`), into `PlayerTracksContext`, mounted whatever draws the menus.
- **A pointer-shown overlay that never hides** (step 5). Focusing ▶ on a mouse move or tap, with D407 §6's focus rule, would hold it for good. Only Up/Down focus ▶; the pointer leaves focus where it is, as `useOpensAtThePointer` does today (`tv-controls.tsx:267`). `shownBy` is "remote" only when `isTelevision`: a computer's keys also arrive as `"key"`. D407 §4 says so.
- **The controls leaving the stage** (step 11). ⤢ finds its stage by `closest(".film-player__stage")` (`full-screen-button.tsx:29`); `isInThePlayersControls` checks `controls.parentElement?.contains(video)` (`player-commands.ts:67`), which drives Space and the timeline's arrows; `useShownWhileActive` takes `overlayRef.parentElement` as the stage (`page-controls.tsx:100`). Each takes the stage or video by an explicit ref.
- **Main red after step 5.** Step 5 also covers: `page-controls.tsx:13` importing `overlayShownForMs` and `useOpensAtThePointer`; `overTheFilm` and `TvControlsAndUpNext` in `up-next.tsx:200–212`; `icon-button.test.tsx:49` and `player-widgets.browser.test.tsx:106`, which render `TvControls`; and the tests that assert the opposite of D407: `tv-controls.test.tsx:74` (no ⏪⏩), `:216` (Still watching keeps Left/Right) and `input-player.browser.test.tsx:267`, `:286` (the on-screen Back). A test removed because D407 reverses what it pins is replaced in the same commit by one pinning D407, and the commit carries `Test-Weakened:` naming D407. The efficiency tests (`tv-controls.test.tsx:271`, `page-controls.test.tsx:203`) carry over to the overlay.
- **The lab's server enums** (steps 1–2). Lab events are checked on the server (`internal/core/api/lab.go:145–146`, `Kind` and `Button`), so new fields, a `contextmenu` kind and new button names need `lab.go`, `lab_test.go` and `make generate`. Full-screen events are logged already (`lab-fullscreen.tsx:109–175`). `lab-input.tsx` is at 759 of 800 lines: the new probes go in a new file.
- **Hardware volume keys** (step 2). `ButtonName` already has `volumeUp`/`volumeDown` for `AudioVolume*` (`input-buttons.ts:112–113`), mapped to no command on purpose. The new commands are reached only from `+`, `=`, `-` and the number pad, with a test that `AudioVolumeUp` still gives none. `MediaTrackNext`/`MediaTrackPrevious` get `buttonsByKey` entries.
- **Back in real full screen with something open** (steps 5, 7, 8). On Fire TV, Back with a menu open leaves full screen (idiosyncrasies:1331); the layout switch must not unmount an open `<dialog>` with its opener, or focus lands on `<body>` (1399): the menu closes first, then the layout changes, focus going to ▶. `useChoiceOfPresses` (`up-next.tsx:388`) takes Back in capture to dismiss the offer: in the full overlay the first Back hides the overlay (D407 §5), and only while the convenience area shows alone does Back dismiss the offer. Browser tests pin both.
- **The gallery's icon count** (step 4): `ui-gallery.test.tsx:53` counts 9 player icons; `ui-gallery.tsx` and its test join step 4.
- **Watch Together's guest** (step 6). `film-player.tsx` (764 lines) also draws the guest's `FollowingGroup`/`GuestStalls` (`together-viewer.tsx:536`): the over/under switch is made in the Play page's own pieces, not in `Stage`.
- **`PlayerControls`' other users** (step 5): `session-remote.tsx:47` and `ui-gallery.tsx:336/344` rely on the timeline coming last; the default order stays, and the overlay passes its own.
- **A TV outside full screen has no way up** (step 11). `PlayTrail` draws nothing on a TV (`play-up.tsx:113`), and Q5 drops the on-screen Back, so a TV's layout below the video carries ‹.

### Corrections to the plan

- **Rule 11, focus memory** (step 5): `useRowFocus` is not written. The rows are `data-focus-container`s and use `rememberedIn` (`focus-memory.ts`, `input-commands.ts:177`, README:562).
- **The menus** (step 7): `DropDown` reports open through a new `onOpenChange`; ⁝ holds a stepper and toggles, which `ActionChooser` cannot (plain buttons only), so ⁝ is its own `DropDown` content. `drop-down.tsx` and `route-chooser.tsx` join step 7.
- **Subtitles moving up** (step 7): only the JASSUB layer moves. `::cue`'s `line` is a `VTTCue` property, not CSS, and native captions show only in the video element's own full screen (an iPhone), where the overlay is not drawn.
- **Step 9's server side**: the logic is `Browser.UpNext` in `internal/core/library/next_up.go:228`, also used by `moveCastOn` (`casts.go:143`), so previous gets a library test. `api/openapi.json` and `gen/api.ts` come from `make generate`. `cast-remote.tsx:491` uses `useUpNext` too. On the receiver ⏭ moves the cast on (`moveOnOrReady`); ⏮ there needs a cast "move back" and is left out until it exists. Whether `ReadMembers` leaves out extras is checked first.
- **The receiver** (step 5): its Back and double OK take control back before anything else (`cast-receiver.tsx:650–690`), so "the first Back hides" does not apply there; step 5 wires §12's receiver (no ‹, no menus).
- **Step 12** also covers README:230–245 (the cast section cites `tv-controls.test.tsx`) and 630–644 (the module list).
- **Row 0's title** comes from the Play page's trail data (`play-up.tsx`), the series and episode.
- **Which full screen** (step 5, the owner's choice after the audit): a TV opens real full screen when a film starts, as today (the hook from `useFullscreenStage` stays); where the browser refuses it, `fillTheScreen`'s fill-the-window fallback stays and counts as full screen for the overlay. `useIsFullScreen` watches `fullscreenchange` and the fill class, the latter by a `MutationObserver` or, better, by the fallback setting state the hook reads. Q4's "never only fills the window" is replaced by this.
- **On an iPhone** ⤢ goes to iOS's own player, so the overlay never shows there; nothing to build.

### Not verified, checked while building

JASSUB's inline styles against a CSS offset; a modal `<dialog>` over a full-screen stage in Silk (VQ245); whether `input-walk`/`input-pages` browser tests walk the Play page's controls; whether the overlay's layer stops clicks reaching the video (Q10 needs them through); `pointerType` on clicks in older iOS Safari.

## 7. Built (2026-11-02)

Each step's commit, and where it departs from §3:

1. `"feat(lab): record a key's modifiers, composing and context menus in the button test" feat(lab): record a key's modifiers, composing and context menus in the button test`.
2. `"feat(input): volume keys, and next and previous as commands" feat(input): volume keys, and next and previous as commands`.
3. `"refactor(player): hold the track choices around the stage, where the overlay can reach them" refactor(player): hold the track choices around the stage, where the overlay can reach them`.
4. `"feat(player): icons and shared full-screen state for the overlay" feat(player): icons and shared full-screen state for the overlay`.
5. `"feat(player): the overlay's states and rows (D407 §4-§7)" feat(player): the overlay's states and rows (D407 §4-§7)`, with
   `"refactor(player): the overlay's code as the web lint asks" refactor(player): the overlay's code as the web lint asks` after it. Item 17's fix
   and its two caveats are in that item.
6. `"feat(player): Space presses the focused control, and the overlay serves a computer's full screen" feat(player): Space presses the focused control, and the overlay serves a computer's full screen`.
7. `"feat(player): subtitles, audio and more in the overlay (D407 §1, §8)" feat(player): subtitles, audio and more in the overlay (D407 §1, §8)`. A cast's
   receiver draws no menus.
8. `"feat(player): the convenience area, Up next and Still watching alone (D407 §3)" feat(player): the convenience area, Up next and Still watching alone (D407 §3)`.
9. `"feat(api): the episode before a file in its sequence, beside up next" feat(api): the episode before a file in its sequence, beside up next`, then
   `"feat(player): previous and next in a sequence (D407 §2, §10)" feat(player): previous and next in a sequence (D407 §2, §10)`. On a cast's receiver
   ⏭ moves the cast on and ⏮ is left out until a cast can move back.
10. `"feat(player): volume button and keys (D407 §10, Q11)" feat(player): volume button and keys (D407 §10, Q11)`.
11. `"feat(player): controls below the film outside full screen (D407 §9)" feat(player): controls below the film outside full screen (D407 §9)`. The switch is in the
    Play page's pieces (`OverTheFilm` over, `PageControls` under, `page-controls.tsx`), not in
    `film-player.tsx`'s `Stage`. Under the film the subtitle, audio and more menus stay where they
    were before D407, under the bar on the page, rather than a second copy in the bar; the bar
    carries 🔈 where the device shows it. The bar never hides, so the efficiency audit's finding 7
    (read the time only while shown) no longer applies to it. The player's controls name the
    video they command (`markPlayersControls`), which ⤢ and the arrows' and Space's answers read.
12. `docs(web): the player's controls as D407 has them`. The README cites no `tv-controls.test.tsx`
    and there is no `tv-controls.tsx`, so nothing was renamed there.
