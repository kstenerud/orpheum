# What the player shows today, for the playback redesign (2026-11-07)

> **ABOUTME:** A snapshot of the playback view as built at "docs(plans): file the RealMedia sound drift the owner found": the full-screen overlay, Cast,
> Watch Together and voice, with file:line references, taken for the owner's redesign of 2026-11-07
> (situational controls in the overlay: one volume control with a slider per voice, a microphone,
> one Cast covering every cast kind, a casting controller, a Watch Together button). History, not a
> specification. Paths are under `web/apps/web/src/` unless given in full.

## 1. The film player and its controls

**How it is composed.** `film-player.tsx` draws only the stage: the `<video>` and the page's `over`
piece inside `.film-player__stage`, the `under` piece after it, and the `around` piece holding both
(`film-player.tsx:106-134`, `:240-284`). The Play page fills those pieces with
`overTheFilm(hasTelevisionControls)`, `PlayUnderOfPage` and `PlayTracksOfPage` (`play.tsx:308-319`).
The stage fills the window on every device (D418 §1, `play.tsx:268-284`). It goes full screen as it
opens on a television, on a cast receiver, or where the browser is already full screen
(`up-next.tsx:278-290`, `play.tsx:286-296`).

**Full screen against windowed.** `OverTheFilm` draws `PlayerOverlay` only while `useIsFullScreen()`
holds, meaning the browser's full screen or the stage filling the window (`page-controls.tsx:44-60`).
Otherwise `PageControls` puts a bar under the film (`page-controls.tsx:70-160`, D407 §9). On the Play
page the stage always fills the window, so **the overlay is the controls in practice**. The bar under
the film only shows where the stage does not fill the window. The sound and track controls that
"stay under the film on the page" are mounted under the film too, so their state stays. In full
screen the overlay holds them (`play.tsx:458-476`, `OverlayHeld`).

**The overlay's rows** (`player-overlay.tsx:294-352`, D407 §1). A remote moves between the rows by
Up and Down. Each row is a `data-focus-container` that remembers its last focus (`focus-memory.ts`).

| Row | Controls, in focus order | Shown when (`controlsShown`, `player-overlay-state.ts:373-404`) |
| --- | --- | --- |
| 1 top `:315-322` | Up (‹ back to the title), convenience area (Up next / Still watching offer, `convenience-area.tsx`), **Watch Together sign**, **Cast sign** (`SignsOverTheFilm`, `:274-284`) | Up: not on a receiver. Together sign: only while a session panel exists (see §3). Cast sign: only signed in (`cast-sign-over-the-film.tsx:5-14`) |
| 2 scrub `:323-334` | Timeline; its arrows are seek commands (`hasArrowsAsCommands`) | always; first focus stop (D410) |
| 3 buttons `:337-350` | ⏮ previous, ⏪⏩ steps, ▶ play, ⏭ next, then the menus: 🔈 volume, 💬 subtitles, 🎧 audio track, ⁝ more, then ⤢ full screen | ⏮/⏭ only where the sequence has them. **🔈 only off TV, off touch, and where volume is settable** (`:381`; not iOS). 💬 and ⁝ not on a receiver. 🎧 only with 2 or more tracks, not on a receiver (D407 §12) |

**What the menus hold** (`player-menus.tsx`):
- 🔈 is a pop-up with an upright volume slider and mute (`:166-200`, D407 §10, D408 §2).
- 💬 holds Off, the subtitle tracks and Find subtitles (`:252-280`). Find subtitles opens a modal
  (`:400-423`).
- 🎧 lists the audio tracks (`:281-300`).
- ⁝ holds Start over, the subtitle offset, the sound timing, levelling, the dialogue boost, the
  original, the bare player and For testers (`:304-385`).

There is no Cast, Watch Together, microphone or voice control in row 3.

**What shows and hides the overlay** (`player-overlay.tsx:135-180`, `:611-700`;
`player-overlay-state.ts`):
- **Opening:** it opens hidden on every device (`openedWith`, `:257`).
- **Remote (D410):**
  - While hidden, Left and Right seek and show the scrub bar alone for `scrubShownForMs` 1500 ms
    (`player-overlay-state.ts:85`).
  - OK plays, or pauses and shows the pause sign (`:105`).
  - Up and Down show it all, with focus on the scrub bar.
- **Pointer:** a mouse's move, a click or a tap shows it all and leaves focus where it is.
- **Touch (D415):** a tap on the picture shows or hides it. A double tap on a side third seeks 10 s
  (`doubleTapWaitMs` 250, `player-overlay-state.ts:413`).
- **Hiding:**
  - It hides 3000 ms (`fullShownForMs`, `:79`) after the last press.
  - It never hides while a menu or a sign's dialog is open (`onOpenChange` → `isMenuOpen`, `:83`).
  - It never hides while paused (`hideAfter`).
  - As it hides, focus moves to the offer or to the video (`useFocusLeavesWhatHides`, `:673`).
  - Pressing play, or Down from the buttons, puts focus on the video so the overlay hides (D407 §6,
    `:504`).
- **Volume:** + and − change the volume by 5 % from anywhere, and show the level for 1500 ms
  (`VolumeShown`, `:235-250`; `player-commands.ts`). On a TV and on touch, this (or the device's own
  buttons) is the only volume control, because 🔈 is hidden.

**Gamepad and keys.** Every input goes through one command layer (`player-commands.ts`, the input
layer). The geometric focus engine (`focus-engine.ts`, `focus-navigation.ts`) searches the
`data-focus-container` around focus, and within an open dialog alone (README "Input"). A controller's
OK button follows the This device setting (`controller-choice.ts`). Back closes a menu first, then
leaves full screen, then leaves the page.

## 2. Cast today

**Only one kind of cast exists in production: Orpheum's own** (D88, D337, D364). A cast means one of
the person's signed-in devices plays the film and this one becomes its remote.
- **Google Cast is a lab spike only.** The sender SDK is loaded only on `/lab/cast` (`lab-cast-sdk.ts:10-15`). The receiver
  `web/apps/cast-receiver/src/main.ts:8-13` is "for now the lab's spike receiver" on the lab's
  namespace. It handles no play, seek, volume or track message.
- **AirPlay has no code at all.** No `airplay`, `webkitShowPlaybackTargetPicker` or `RemotePlayback`
  in `web/`.
- `repoint-root.tsx` is the library-root move, not casting. D434 is about the cast's sound messages
  (see §5).

**Where a cast starts**, two entry points into the same dialog:

| Entry | Where | In full screen |
| --- | --- | --- |
| Tray sign `status-tray.tsx:52`, `:112-125` | banner, every screen; blue while casting (D364 §13) | no: the stage covers the page and its tray (D418 §1) |
| Sign over the film `status-tray.tsx:133-161` (D422) | overlay row 1, last; its dialog is drawn inside the stage | yes |

- **`CastDialog` (`cast-dialog.tsx:27-52`):**
  - With no cast on, it lists `CastTargets` (`cast-to.tsx:44`): the person's other connected devices
    by name. A device already receiving a cast is disabled (D386 §1).
  - Choosing a device moves the film there at its position (D364 §1), after the receiver's "Use?"
    (`cast-asked.tsx`, D358).
  - There is no choice of cast *kind*, because there is only one kind.
- **With a cast on,** the dialog shows `CastOn` (`cast-dialog.tsx:71-120`): "Playing X on Y", "Back
  to the film" and Stop casting.

**What the sender shows while casting.** The Play page's video area becomes `CastRemote`
(`cast-remote.tsx:50`, chosen by `PlayOrSendOn`, `cast-to.tsx:415-457`). It is an ordinary page with
no player, no stage and no full screen. `Remote` / `RemoteOf` (`cast-remote.tsx:117-150`, `:336-370`)
show the following.

| Control (own cast) | Works? | Where |
| --- | --- | --- |
| Play / pause | yes | `session-remote.tsx:54` `controls={["play","steps","timeline"]}` |
| Seek (timeline, ±10 s steps) | yes, sent once the presses rest | same |
| Audio track (`<select>`, 2 or more tracks) | yes | `ReceiverSound` `cast-remote.tsx:419-484` |
| Dialogue boost, levelling | yes (boost only when the receiver has a boosted mix) | `RemoteSwitch` `:530` |
| Sound timing earlier/later | yes, when the receiver reports one | `RemoteSoundOffset` `:485` |
| Up next / next episode | yes (`moveCastOn`) | `RemoteUpNext` `:556` |
| Stop casting | yes; the film plays on here from the receiver's place (D384 §2, D385) | `Remote`, also the dialog |
| Delivery, quality, link | shown read-only (D371 §4) | `ReceiverPlayback` `:388` |
| **Volume, mute** | **absent**: no message exists; the receiver's own volume only | — |
| **Subtitles, subtitle offset** | **absent** from the remote | — |
| **Quality choice, ⏮ previous** | **absent** | — |

The receiver carries out each remote choice as its own control's press, and reports its sound state
back (`cast-receiver-sound.ts:23`). The receiver's overlay hides 💬 🎧 ⁝ and Up (D407 §12), and keeps
the TV controls: Back, or a double OK or tap, takes control back and ends the cast (D338 §6, D385).
Tests: `cast-remote.test.tsx`, `cast-receiver-sound.test.tsx` and `cast-uncontrolled.test.tsx`. The
uncontrolled test covers a reload in mid-cast only. Google Cast and AirPlay have no tests because
they have nothing to test.

## 3. Watch Together today

**Signs:**
- **Tray sign** (`status-tray.tsx:49-50`, `:82-105`; D280, D364 §13, D384 §2):
  - It is shown to a person who may host, or who is hosting or viewing.
  - It opens `TogetherDialog` (`together-dialog.tsx:35-50`). That dialog holds the guest's session
    or the host's `WatchTogetherControls`. The host's view is in `watch-together.tsx:320-369`: what
    plays (`TogetherPlays`), voice and film volume (only when no panel is drawn, `:332-333`), people
    (`SessionPeople`), links and End session.
- **Sign over the film** (`together-over-the-film.tsx:87-136`; D423, D433):
  - It is in overlay row 1 and **shows only while a session panel is drawn**
    (`hasPanel`, `:46`, `:114`).
  - It opens the tray's same dialog over the film. The live panel is *moved* (portalled, not
    redrawn) into the dialog's `panelPlace`, so the voice keeps running (`TogetherPanel`, `:58-79`;
    `:161-180`).
  - It closes when the session ends (`:142`).
  - **A session cannot be started from full screen**: with no session there is no sign over the
    film, and the tray is hidden.
- **Join and leave notes:** `ComingAndGoing` (`:212`) shows for 5000 ms over the stage.

**The session panel (`TogetherPanel`), on the page under the film**:

| | Host (`together-host.tsx:183-193`) | Guest (`together-viewer.tsx:344-351`) |
| --- | --- | --- |
| News | `TogetherNews` | `TogetherNews` |
| Voice status, "tap to hear", Talk 🎤 toggle | `VoiceControls` (`together-voice.tsx:392-433`) | same |
| Per-person voice volumes | `VoiceVolumes` (`together-voice.tsx:355-369`): one 0–4× slider per other person; also in the host's per-person dialog (`together-people.tsx:222-240`) | same |
| Film volume | `FilmVolumeSlider` (`film-volume-slider.tsx:36`) | own `VolumeSlider` (`together-viewer.tsx:383-397`); buttons note on iOS |
| Leave | in the dialog (End session) | `LeaveButton` |

- **Talk** (`TalkToggle`, `together-voice.tsx:285`) is one toggle (D281 §7). An iPhone's first Talk
  warns that the sound moves to the earpiece (`movesSoundToTheEarpiece`, `:377`; D336 §4).
- The headphones hint is always shown (D63).

**In full screen today:**
- A signed-in host or guest on the Play page reaches the panel only through the sign over the film.
  That is three presses: Up, Right to the sign, then OK. It opens a modal holding the whole dialog
  (people, links, voice, sliders).
- The volume, Talk and per-person sliders are not in the overlay itself.
- 🔈 sets the film's volume only, and only off TV and off touch.
- **A guest by link alone (not signed in)** has `TogetherViewer`:
  - Their film has its own `over` piece, `FollowingGroup` (`together-viewer.tsx:560-561`, `:630`),
    which holds "sound back" and a Full screen button that calls
    `enterVideoFullScreen(video)` (`:762-789`).
  - That button puts the *video element* full screen, so nothing of the panel can show over it, and
    there is no Cast sign (nobody is signed in).
  - Whether their page draws the overlay was not traced; flagged.

**How the volumes are built:**
- **Film volume:** one tab-wide value, 0–1 (`film-volume.ts:1-60`). `followFilmVolume` (`:45`)
  applies it through `setHeardVolume` (`web/packages/player/src/gain.ts:118`), which is the
  element's `volume`, so 🔈, + and −, and the panel's slider all move one value.
  - iOS lets no page set the volume (`canSetVolume`, `gain.ts:33`).
- **`gainAfterVolume` (`gain.ts:440-500`) is the film's loudness levelling (D226 §4), not the
  per-person volumes.** It routes the element through Web Audio (`createMediaElementSource` → gain)
  only once a gain other than 0 dB is asked for. An element handed to Web Audio stays so for its
  life.
  - Where the device's kept Web Audio latency is above `webAudioLateAboveMs` = 100 ms (`gain.ts:86`)
    and the film plays its own file, it skips Web Audio: a lowering uses the element's volume
    (`heardVolumeOf`, `:109`) and a raise is left to the ladder (D436).
- **Voices:** WebRTC (D63). Each call's stream goes to its own `<audio>` element and a Web Audio
  `GainNode` to `destination` (`web/packages/sdk/src/together-voice-browser.ts:225-258`). `routeOf`
  picks the element's volume or the Web Audio gain, which allows above 1×. The per-person gain is
  0..`partnerGainMost` = 4 (`web/packages/sdk/src/together-voice.ts:114`, `:284-295`; D281 §4).
  - The microphone is levelled through its own Web Audio chain with a limiter
    (`together-voice-level.ts:117-140`).
- **Limits:**
  - On the owner's TCL TV, Web Audio put the film's sound 400–500 ms behind the picture
    (`docs/contributors/design/plans/navigation-and-widgets-answers-2026-10-27.md:2057-2063`).
    Option (b) was chosen and built as D436 (`:2064-2091`, `:2145-2149`).
  - Voices still go through Web Audio, untouched by D436. Voice lag matters less than film lag but
    has not been measured.
  - iOS: no settable volume, so the film slider becomes a note. Opening the microphone moves the
    sound to the earpiece.

## 4. Voice while casting

- **Nothing in the voice code knows about casting** (no `cast` in `together-voice.tsx` or
  `web/packages/sdk/src/together-voice.ts`).
- **On the host's sending tab, voice runs on.** The host's voice lives in `HostVoiceProvider`, at app
  level in the sending tab (`app.tsx:180`), so it carries on there, through the phone or laptop. The
  receiver does not play it.
  - While casting, the Play page is `CastRemote`, and the host's panel is not drawn
    (`HostPlaysTogether` needs the page's video, `cast-receiver.tsx:76-92`).
  - Voice is then reached from the tray's dialog (`watch-together.tsx:332`, drawn there when there
    is no panel).
- **A casting guest** keeps their viewer page's own voice, with "Playing on X" and the session remote
  (`together-viewer.tsx:430-453`).
- **The receiver following a guest's group** sets up no voice (`together-viewer.tsx:403-418`).
- **Not established:** whether a receiver that leads a hosted session (D364 §11) draws
  `VoiceControls` with a host voice. That needs `hostMembership` in the receiver's tab; not traced.
- **No test covers voice during a cast.**

## 5. Decisions that govern these areas

| D | What it fixes |
| --- | --- |
| D63 | Voice is WebRTC; headphones are the supported setup |
| D88 | A cast is remote control of another client: Watch Together with a group of one |
| D226 §4 | Loudness is levelled by a gain after the element's volume; 100 is the film at the level |
| D266 | A quality whose video stands still is given up (cast and stall behaviour) |
| D278 / D280 | Yellow problem signs; the status tray, whose Watch Together and Cast signs are the entry points |
| D281 | Everyone hears from the start; Talk opens the mic; volume per person apart from the film's (§4); the host's pop-up runs voice (§5) |
| D336 | The sound plays as it is; no one is asked about their device (no headphones question) |
| D337 / D338 / D358 | A cast goes to the person's own devices; the receiver keeps TV controls and leaves by Back or a double OK; it asks "Use?" first |
| D364 | A cast is a session with a device; the tray shows it; the receiver may lead the host's Watch Together (§11) |
| D369 | Every device uses the app's own player controls, built from one widget set |
| D371 §4 | The receiver's playback is reported to the remote; a guest's cast follows the group |
| D384 / D385 / D386 / D388 / D389 | Cast ends: the receiver goes home and the sender plays on from there; no "play here instead"; a session's end ends its casts |
| D407 | The full-screen overlay: three rows, arrows, menus, hide timing, receiver hides menus (§12), controls under the film outside full screen (§9) |
| D408 | Own widget layer: a pop-up is not a dialog; a list opens on its current choice |
| D410 / D411 / D415 | Scrub bar is the first stop; a press goes on from it; touch tap and double-tap seek |
| D418 | The player fills the window on every device, its controls over the picture |
| D421 | A dialog's safe choice comes first and takes focus |
| D422 | Cast's sign over the playing film; at the time, Watch Together stayed in the tray |
| D423 / D433 | Watch Together's sign over the film, opening the tray's same dialog with the panel moved in |
| D434 | A cast's remote works the receiver's sound in the *cast's* session alone, even while it leads Watch Together |
| D436 | No Web Audio levelling for an own file where Web Audio is late |

The web README's "Read before changing" also names D188 (every focusable has a name), D361 (a modal
gives focus back) and D408.

## 6. Constraints the redesign must respect

- **TV and D-pad:**
  - Every control must be reached by arrows, inside `data-focus-container` rows. The walks run at
    Silk's 960 × 480 and at desktop size in Chromium and Firefox (`input-walk.browser.test.tsx`,
    `input-rows.browser.test.tsx`, `input-pages.browser.test.tsx`).
  - Overlay behaviour: `player-overlay.test.tsx`, `input-player.browser.test.tsx`.
  - Back closes the innermost menu or dialog first.
  - A sliders pop-up must not trap Up and Down. In 🔈's pop-up the range is `isFocusable={false}`,
    and Up and Down step it wherever focus is (`volume-range.tsx:38-52`).
- **Hiding:** an open pop-up or dialog must report `onOpenChange` so the overlay holds and leaves the
  presses to it (D407 §6, §8). Leaving full screen with a menu open moves focus to the scrub bar
  (`player-overlay.tsx:476-497`).
- **Touch and phones:**
  - 🔈 is hidden on touch, and iOS cannot set the volume at all, so a "film" slider there must
    become a note, as `VolumeSlider` does.
  - Pop-ups must fit a phone's width inside the screen (`popover.tsx`, "the owner's half-screen
    pop-up").
  - Double-tap zones take the picture's side thirds (D415).
- **Receiver:** a cast receiver draws no menus (D407 §12). Its remote holds the choices.
- **10-foot:** the documented rules are D407's: big rows, a 3 s hide, no ring at open, + and −
  global. There is no separate 10-foot standard in `docs/contributors/standards/`.
- **Accessibility:** every focusable element has an accessible name, enforced by `jsx-a11y-x` strict
  and `control-has-associated-label` (D188). No inline text (D51). A modal returns focus (D361 §3).
  A dialog's safe choice is focused first (D421).
- **Architecture:** the overlay must not import the tray's code (Cast's sign comes through
  `CastSignOverTheFilmContext`, `cast-sign-over-the-film.tsx`). Together's panel is moved, never
  drawn twice, so that voice is not restarted (`together-over-the-film.tsx:24-29`).

## Gaps against the owner's goal

1. **No Google Cast and no AirPlay in production.** "One Cast control covering three kinds" needs two
   of them built first: the Google Cast sender is a lab spike, the receiver app is a lab receiver
   (D245 still planned), and AirPlay has no code.
2. **The cast remote is a page, not a full-screen view.** It lacks volume, mute and subtitles in
   every mode, has no volume message at all, and offers no "controls that work in this mode" logic,
   because there is one mode.
3. **The overlay has no volume row of sliders and no mic.** 🔈 is film-only and hidden on TV and
   touch. Talk and the per-person sliders live only in the Together dialog's panel.
4. **Watch Together cannot be started from full screen.** Its sign shows only once a session panel
   exists. A link guest's "Full screen" puts the bare video full screen, which hides everything.
5. **Voice while casting is untested and implicit.** It stays on the sending tab. Whether a receiver
   that leads Watch Together plays voice is not established.
