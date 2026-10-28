> **ABOUTME:** How the leading players lay out their playback controls on a computer, a touch screen
> and a television remote, what the design guidelines and accessibility rules say about them, and
> what that means for Orpheum's player bar (D369): one layout per input, the volume control's form
> (the owner's vertical question), the timeline, the track and sound menus, show and hide, and the
> keyboard map. Read before changing `player-controls.tsx`, `page-controls.tsx`, `tv-controls.tsx`,
> `timeline.tsx` or `volume-range.tsx`.

# Playback UI: Controls per Input, and the Volume Question

- **Date:** 2026-10-28. Asked by the owner: *"It feels like the volume widget should be vertically
  oriented. Do a research round into how our competitors design their playback UIs (desktop,
  touch, and d-pad interface), and what UI experts have to say."*
- **Method:** read-only, at `main` an earlier commit: D369 and its as-built notes, the player's files in
  `web/apps/web/src`, the [UI widget audit](ui-widgets-audit-2026-10-27.md), the
  [remote's contract](../plans/tv-client.md) and the [web player's plan](../plans/web-client.md).
  Then vendors' own help and design pages, Jellyfin's web client read locally (a clone at
  4a745fe, 2026-10-27), Kodi's keymap file, and the accessibility standards. Fetched pages are
  cached in `/yoloai/cache/http/playback-ui/`.
- **Labels:** **[doc]** a vendor's own help or design page, or a standard; **[code]** read in
  source; **[rep]** a third-party report; **[K]** how a product behaves as widely known, not
  re-checked against a page today. Netflix, Disney+ and Prime Video document their keys but not
  their layouts, so their layouts below are [K] unless a page is cited.
- **Builds on** [ui-competitors.md](ui-competitors.md) §4 and
  [ui-design-practice-2026-10-05.md](ui-design-practice-2026-10-05.md) §2.12 and item 12, which
  said *"the familiar layout (play and time at the start, tracks, volume and fullscreen at the
  end)"*; and on [controller-input-2026-10-25.md](controller-input-2026-10-25.md) for the remote.

---

## 1. Summary

**The vertical volume, answered.** Yes, on a computer, as a pop-up from the mute button; and on a
touch screen or a television, no volume slider at all, in either orientation.

- **Nobody shows a volume slider on a television.** The TV's own volume is the remote's. Fire TV
  makes volume *"a system function"* an app *"cannot"* map [doc]; tvOS has no sliders at all
  (*"Not supported in tvOS"*) [doc]; Jellyfin removes mute and slider on every TV, console, phone
  and iPad (`PhysicalVolumeControl`) [code]. Orpheum's TV bar already has none.
- **Nobody worth copying shows one on a phone either.** Apple: *"Don't use a slider to adjust audio
  volume"* on iOS [doc], and iOS ignores a page's volume anyway
  ([idiosyncrasies](../../idiosyncrasies.md#ios-an-elements-volume-is-read-only-to-the-page-and-web-audio-output-follows-the-ringsilent-switch)).
  Where a touch player does give volume on screen, it is a vertical *drag* on the right half of the
  picture (Infuse [doc], VLC for mobile [K]), not a widget. Orpheum shows its horizontal slider on
  Android today.
- **On a computer both orientations are established.** Netflix's is a vertical pop-up: *"Hover
  over the icon until you get a slider pop-up, then slide the control knob to the top"* [doc], and
  Safari's own video controls pop one up the same way [rep]. YouTube's, Jellyfin's, Chrome's and
  VLC's are horizontal beside the speaker, YouTube's hidden until hovered [rep][code][K]. Neither
  is wrong for a mouse.
- **What decides it for Orpheum is the arrows.** One bar serves the mouse, the keyboard and the
  remote (D369 §1), and its row is walked with Left and Right. A horizontal slider eats Left and
  Right, which is why the as-built bar has to put the volume last, with nothing after it (D369
  status). A vertical slider uses Up and Down, the direction it moves, so Left and Right go on
  walking the row. The constraint goes, and the bar can follow the usual order: sound beside
  play, as YouTube and Netflix put it.

**The recommendations** (§2 has the detail; §8 lists what is the owner's to choose):

1. **Put the timeline above the buttons, the full width of the bar,** on every device. Every
   player read puts it there (YouTube, Netflix, Jellyfin, Apple, Android TV) [code][doc][K]. The
   as-built bar puts it below.
2. **Computer:** `play · −10 · +10 · sound · time` on the left, `audio & subtitles · settings ·
   full screen` on the right. The volume is a vertical pop-up from the sound button. The keys
   are YouTube's (already decided in D188, not yet built), with one 10 s step.
3. **Touch:** a large play button centred, with −10 and +10 beside it, as Netflix and
   Apple's own iOS player do [K]. The timeline sits along the bottom, and audio & subtitles and
   full screen at the corners. No volume slider. A tap shows the controls and a tap hides them.
   Double-tapping either side steps 10 s (YouTube [doc], Infuse [doc]).
4. **Remote:** the bar as it is, with the timeline on top and audio & subtitles added (today the
   track menus are not on the TV's bar, and in full screen they are off screen). Hold Left or
   Right to go faster, and run the steps of repeated presses together into one seek.
5. **One "Audio & subtitles" panel over the film,** opened from the bar on every device, and a
   settings panel for boost, levelling, the sound offset and the subtitle offset. Both move from
   under the film into the player, in at most two presses on a remote (D90's bound, via
   [tv-client.md](../plans/tv-client.md)).
6. **Show and hide:** keep the 5 s timer. Hide at once when the mouse leaves the player, and hide
   the pointer with the bar. Never hide while paused, or while a pointer, focus or an open panel
   is in the bar.

---

## 2. Recommendations for Orpheum

### 2.1 Computer (mouse and keyboard)

```
 ┌──────────────────────────────────────────────────────────────────┐
 │                                                                  │
 │                         (the film)                               │
 │                                                                  │
 │  ░░░░░░░░░░░░░░░░░░░ scrim, bottom only ░░░░░░░░░░░░░░░░░░░░░░░░  │
 │  ━━━━━━━━━━━━━━━━━━━━━━━●──────────────────────────────  1:42:10 │  timeline, full width
 │  Play -10 +10 Vol  0:41:07 / 2:23:17      [Aa] [Set] [Full]     │  the row
 └──────────────────────────────────────────────────────────────────┘
                 ▲
                 └ hover or focus: a vertical slider pops up above it
```

- **Timeline on top, the full width,** with where the film stands to the left of the row and
  either the length or what is left at the right end. NN/g saw people *"regularly hovered over
  video controls while the video was playing to check how much time was left"* [doc]
  ([video usability](https://www.nngroup.com/articles/video-usability/)). On hover, a time
  tooltip shows where the pointer is. That is cheap, and every desktop player has it [K].
- **The row, left group:** play, −10 s, +10 s (Netflix's web bar has them [K]; Orpheum's desktop
  bar has none today), sound, the clock. **Right group:** audio & subtitles, settings, full
  screen. Full screen is last everywhere: YouTube, Netflix, Jellyfin, Plex [K][code].
- **The volume:** the sound button mutes on a click. Hovering it, or focusing it, opens a vertical
  slider above it (§3 has the rules a pop-up must keep). Up and Down on the focused sound button
  or the slider set the volume in 5% steps (YouTube's step [doc]). Esc closes it. The mouse wheel
  over it sets it too [rep]. Left and Right leave it for the next button, so its place in the row
  is free.
- **Keys** (D188 decided YouTube's map; [web-client.md](../plans/web-client.md) has it, and none of
  it except Space is built): Space and `k` play or pause; `j` and `l`, and Left and Right, step
  10 s; Up and Down change the volume; `m` mutes; `f` toggles full screen; `c` toggles subtitles;
  `0`–`9` jump to tenths; Home and End; Esc leaves full screen. Bind on `KeyboardEvent.code`;
  ignore Ctrl, Alt and Cmd; only while focus is in the player (WCAG 2.1.4).
  - **One step, not two:** web-client.md copied YouTube's 5 s arrows beside its 10 s `j`/`l`.
    Netflix, Prime Video and Disney+ all step 10 s on the arrows [doc], Orpheum's remote does too,
    and a film is not a vlog. One 10 s step everywhere is simpler. The owner's call (§8).
  - **Up and Down's conflict:** today Down on the video moves focus into the bar (D369 status).
    With Up and Down as volume, which is Netflix's, Prime's, Disney+'s, YouTube's and
    Jellyfin's [doc][code], Tab is the way into the bar, as YouTube's is, and Down from the video
    no longer reaches it. The other way is to keep Down for the bar and make volume Up and Down
    only on the sound button. The owner's call (§8).

### 2.2 Touch (phone and tablet)

```
 ┌──────────────────────────────────────────┐
 │ ←                              [Aa]  ⤢   │  top corners
 │                                          │
 │          ↺10       ▶       ↻10           │  centre cluster, large
 │                                          │
 │ 0:41:07 ━━━━━━━━━━●─────────── 2:23:17   │  timeline along the bottom
 └──────────────────────────────────────────┘
```

- **The centre cluster:** a large play button with −10 and +10 beside it, which is where a thumb
  is. This is Netflix's and Apple's iOS player's arrangement, and YouTube's with previous and next
  in place of the steps [K]. Double-tapping the left or right third steps 10 s (YouTube: *"double
  tap the left side … to rewind"* [doc]; Infuse the same [doc]).
- **No volume slider, and no mute,** on any device whose primary pointer is coarse, as Jellyfin
  decides by device [code]. The phone's buttons are the volume. This removes the Android
  horizontal slider the bar shows today. Whether a mute button stays on a phone is minor; none of
  the big apps has one [K].
- **The timeline** keeps its touch-anywhere handling (range-touch.ts). While a drag is under way, a
  time label rides above the finger (and later a thumbnail, §2.4). Lifting the finger outside
  the bar cancels, as YouTube's *"Release to cancel"* does [doc].
- **A tap toggles:** shown → hidden, hidden → shown. Today a tap only shows the bar (D369 status),
  so the only way to hide it is waiting. Netflix and YouTube toggle [K].
- **Targets:** 2.75rem (44 px) already meets Apple's 44 pt and WCAG 2.5.5's 44 px (AAA); the centre
  play can be larger.

### 2.3 Remote (D-pad: Fire TV, Android TV, a TV browser)

- **Keep the as-built model,** which follows Jellyfin's on the owner's Fire TV (`overlayAnswer`):
  hidden, Left and Right step and show the timeline alone; OK, Up or Down open the bar with focus
  on play; Back hides it.
  - **One difference from the platform rule, to know about:** Android TV's quality rule TV-PC says
    *"While a video or audio is playing, pressing the D-pad center button pauses the media"* [doc],
    and Netflix's TV app pauses on OK too [rep]. In Orpheum's bar the first OK opens and the
    second pauses. Apple's remote opens the controls with a rested finger or Up, and pauses with
    the clickpad's centre [doc]. Either is defensible. The owner's call (§8).
- **Add audio & subtitles to the TV's bar.** Today it has play, full screen and the timeline. The
  track and sound controls sit under the film, outside the stage the TV puts in full screen, so a
  remote cannot reach them while the film plays (as read in the code; to confirm on a device).
  tv-client.md's contract wants *"Track menus are visible buttons on the control bar, one level
  deep"*, and Apple moved its tvOS track panel to buttons because they *"improved the
  discoverability"*.
- **Step and speed:** a press steps 10 s, the same everywhere: Apple [doc], Android TV's *"+N
  seconds"* [doc], Netflix [rep], Kodi's first step [doc]. **Holding** goes faster: Android TV
  scrubs on hold [doc], Apple cycles 2×, 3×, 4× [doc], Kodi escalates 10 s → 30 s → 60 s on
  repeated presses [doc]. **Repeated presses run together:** Kodi waits its *skip delay*, 750 ms
  by default, and seeks once [doc]. `Timeline` already has `settleMs` for a remote's drag; a
  remote's steps could use the same, so five presses make one seek of 50 s, not five seeks.
- **No volume on screen.** The remote's volume is the TV's (Fire TV [doc]). The bar already
  omits it.
- **Timeout:** 5 s, as now. Jellyfin uses 3 s [code]; Apple and Android do not fix one.

### 2.4 The timeline, on every device

- **Above the row, full width,** for the reason in §1, and because a wider timeline is a finer
  one: NN/g, *"Most sliders have a narrow width, so users often have a hard time to hit a specific
  spot accurately"* [doc] ([sliders](https://www.nngroup.com/articles/sliders-knobs/)).
- **Thumbnails while seeking** are the field's standard: Roku requires them for anything over
  15 minutes [doc]; Apple shows *"a preview thumbnail … above the timeline"* [doc]; Android TV
  shows *"an overlay of thumbnails"* [doc]; YouTube on touch adds a row of them [doc]. Orpheum has
  none; they need the node to make an image track. A feature to schedule (§8), not a widget change.
- **Chapter marks** where the file has chapters: YouTube draws *"a vertical line in the progress
  bar"* [doc]; Jellyfin has a marker layer on its slider (`sliderMarkerContainer`) [code]. The same
  layer can later mark an intro (D82, D115).
- **Over a remote connection** (a cast's remote), the drag waits for its settle before it seeks,
  as built.

### 2.5 Menus: audio, subtitles and sound

- **One "Audio & subtitles" panel** (Netflix's, one button with two columns [K]; ui-competitors.md
  §4 recommended it), opening over the film from the bar. It names each track (language, SDH,
  forced, commentary) and has finding subtitles at its foot.
- **A settings panel** for what is Orpheum's own: boost, levelling, the sound offset, the subtitle
  offset, and later quality and speed. YouTube's and Plex's gear holds the same kind of thing
  [K][rep].
- **Both are panels inside the stage,** so they work in full screen and from a remote. Back or Esc
  closes them, and focus returns to their button.
- **`c` and `a`:** Prime Video cycles subtitles with `C` and audio tracks with `A` [doc]; YouTube
  toggles captions with `c` [doc]. Toggle with `c`, as YouTube does; cycling is Prime's alone.

### 2.6 Show and hide, on every device

| Shows on | Hides on | Never hides while |
| --- | --- | --- |
| Computer: a mouse move over the player, a key in it, focus coming into the bar | 5 s without input while playing; the mouse leaving the player, at once | paused; the pointer over the bar; focus or an open panel in it |
| Touch: a tap | a tap; 5 s without input while playing | paused; a drag under way; a panel open |
| Remote: OK, Up or Down (the whole bar); Left or Right (the timeline alone) | Back; 5 s without a press while playing | paused (the whole bar); a panel open |

Hiding the bar also hides the mouse pointer over the film [K]. Subtitles move up while the bar
shows ([web-client.md](../plans/web-client.md), from jellyfin-web's largest complaint class).

### 2.7 What changes from D369 as built

| As built (D369 status) | Proposed |
| --- | --- |
| Timeline under the buttons | Timeline above them, full width |
| Volume horizontal, last in the row, because a slider keeps Left and Right | Computer: a vertical pop-up from the sound button, the sound beside play. Touch and TV: none |
| Desktop bar: play, sound, full screen, timeline; no steps | Adds −10 and +10, the clock, audio & subtitles, settings |
| Phone: the same bar; a tap only shows it | Centre cluster; a tap toggles; double-tap steps 10 s |
| TV bar: play, full screen, timeline, "Ends at" | Adds audio & subtitles (and settings); holding goes faster; repeated presses run together |
| Tracks, boost, levelling, offsets under the film, reached by Down | Two panels in the player, opened from the bar |
| Keys: Space, Left and Right 10 s, OK on the video | D188's YouTube map, a 10 s step throughout |

`VolumeRange` stays the one slider for a volume. It gains a vertical form for the pop-up and a
touch path on `clientY`. Watch Together's two volumes (film and partner, ui-competitors.md §4) are
a different control, a mix, and are not changed by this.

---

## 3. The vertical volume, in detail

### What the players do

| Player | Computer | Touch | TV |
| --- | --- | --- | --- |
| Netflix | vertical pop-up on hover, *"slide the control knob to the top"* [doc] ([help 468](https://help.netflix.com/en/node/468)) | none; the phone's buttons [K] | none |
| YouTube | horizontal, beside the speaker, expands on hover; *"back where it belongs—right next to the play/pause button"* after the 2025 redesign [rep] ([notebookcheck](https://www.notebookcheck.net/YouTube-s-controversial-video-player-redesign-is-popping-up-again-and-honestly-it-doesn-t-look-half-bad.1108496.0.html)) | none [K] | none |
| Jellyfin | horizontal, 9em, beside mute; hidden under 43em wide [code] (`videoosd.scss`) | none: mute and slider removed on phones and iPad [code] | none: removed on TVs and consoles [code] |
| Emby | Jellyfin forked from it in 2018 and the player still matches; not read separately [K] | | |
| Plex | mute and volume in the web bar [rep] ([Plex support](https://support.plex.tv/articles/200392226-plex-web-app-player/), which refused our fetch; from its summary) | none [K] | none |
| Disney+, Prime Video | a pop-up from the speaker icon [K]; keys Up and Down [doc] | none [K] | none |
| Apple TV app, Safari's own controls | Safari pops a vertical slider upward from the speaker [rep] ([wBlock #7](https://github.com/0xCUB3/wBlock-userscripts/issues/7)) | iOS: *"Don't use a slider to adjust audio volume"* [doc] | tvOS: no sliders [doc] |
| Infuse | n/a | drag up and down on the right half [doc] ([Firecore](https://support.firecore.com/hc/en-us/articles/215090987-Playback-Controls)) | none |
| VLC | horizontal wedge in the bar; Ctrl+Up/Down [doc] ([hotkeys](https://wiki.videolan.org/QtHotkeys/)) | vertical swipe on the right [K] | n/a |
| Kodi | an on-screen bar shown by the volume keys; no slider in the OSD [K] | | volume keys |
| Chrome's, Firefox's own controls | horizontal, beside mute [K] | | |

### What the guidelines say

- **Orientation, either:** Material 3 offers sliders *"horizontally or vertically"* and names
  volume as a use; it warns off vertical only for range sliders, since *"People are used to most
  sliders being horizontal"* [doc] ([M3 sliders](https://m3.material.io/components/sliders/guidelines),
  via search; the page needs a script to read; Material's Android slider ships `orientation:
  vertical` [doc]). Apple: minimum at the bottom, maximum at the top, for a vertical slider [doc]
  ([HIG sliders](https://developer.apple.com/design/human-interface-guidelines/sliders)); and on
  visionOS, *"Prefer horizontal sliders. It's generally easier for people to gesture from side to
  side"*, a point about hand gestures, not a mouse.
- **A pop-up on hover has rules.** WCAG 1.4.13 (AA): content shown on hover or focus must be
  *dismissible* without moving the pointer (Esc), *hoverable* (the pointer can move onto it without
  it vanishing) and *persistent* [doc]
  ([Understanding 1.4.13](https://www.w3.org/WAI/WCAG22/Understanding/content-on-hover-or-focus.html)).
  video.js's vertical slider had a bug filed against exactly this, Esc not closing it [rep]
  ([video.js #6004](https://github.com/videojs/video.js/issues/6004)). So: opens on focus as well
  as hover; a bridge with no gap between button and slider, and a short close delay; Esc closes;
  it never opens on hover alone on a touch laptop, where hover does not exist.
- **Dragging must not be the only way.** WCAG 2.5.7 (AA): *"All functionality that uses a dragging
  movement … can be achieved by a single pointer without dragging"*; the slider example is
  clicking anywhere on the track [doc]
  ([Understanding 2.5.7](https://www.w3.org/WAI/WCAG22/Understanding/dragging-movements.html)).
  `VolumeRange` and `Timeline` already set the value where a touch lands; a click on a native
  range does too.
- **The keyboard reaches it.** W3C's media requirements, IC-1: *"Support operation of all
  functionality via the keyboard"* [doc]
  ([MAUR](https://www.w3.org/TR/media-accessibility-reqs/)); the W3C's player guidance asks for a
  visible focus indicator and clear labels [doc] ([WAI](https://www.w3.org/WAI/media/av/player/)).

### Why vertical, for Orpheum's computer bar

1. **The arrows.** As above: the slider's own direction is the direction its keys move, and Left
   and Right stay the row's. This is the one argument specific to Orpheum's single bar for mouse,
   keyboard and remote, and the as-built note shows it was already a cost.
2. **The row's width.** A pop-up takes one button's width; the horizontal slider takes 6rem more
   and wraps the row on a narrow window, which is why Jellyfin hides its slider under 43em
   [code]. The steps, the clock and the two panels' buttons need that room.
3. **Netflix's form,** the film service most viewers know, and Safari's own.

The case for horizontal is real but smaller: YouTube's and Chrome's are horizontal and familiar,
and a horizontal slider needs no pop-up and so none of 1.4.13's rules. If the owner prefers it,
YouTube's collapsed form (only the button until hovered or focused, then expanding beside it)
keeps the width; it does not fix the arrows.

---

## 4. Computers: what the players do

| | Bar layout | Steps | Tracks | Keys |
| --- | --- | --- | --- | --- |
| **YouTube** | timeline on top; left: play, next, volume, time, chapter; right: autoplay, subtitles, settings, theatre, full screen, the right group *"a pill-shaped capsule"* (2025) [rep] | none on the bar; `j`/`l` 10 s, arrows 5 s [doc] | `c`; settings gear holds quality, speed, subtitles [K] | the fullest map [doc] ([7631406](https://support.google.com/youtube/answer/7631406?hl=en)) |
| **Netflix** | timeline on top; left: play, −10, +10, volume; title centred; right: next episode, episodes, audio & subtitles, speed, full screen [K] | ±10 buttons; arrows 10 s [doc] | one audio & subtitles panel [K] | Space/Enter, F, Esc, arrows, M, S skip intro [doc] ([24855](https://help.netflix.com/en/node/24855)) |
| **Prime Video** | transport centred over the picture; tracks, settings, volume and full screen in a top-right cluster [K] | arrows 10 s [doc] | `C` cycles subtitles, `A` audio [doc] | ([help](https://www.primevideo.com/help?nodeId=GE4VW9VWJD7FM6DR)) |
| **Disney+** | timeline at the bottom, transport over it [K] | arrows 10 s [rep] | one panel [K] | Space, arrows, M, F, Esc [rep] ([How-To Geek](https://www.howtogeek.com/456101/how-to-make-streaming-disney-easier-using-keyboard-shortcuts/)) |
| **Jellyfin** | timeline on top with times either side; buttons: previous, rewind, play, forward, next, then time, favourite, subtitles, audio, mute + slider, settings, AirPlay, PiP, full screen [code] | rewind/forward buttons (the user's skip length) [code] | separate subtitle and audio buttons [code] | YouTube's `k`, `j`/`l`, arrows, `,`/`.`, `f`, `m`, Up/Down volume, Home/End, PageUp/Down chapters [code] |
| **Plex** | bottom bar with settings (quality, audio, subtitles, size, autoplay), chapters, play queue, mute and volume; top: minimise, player, full screen [rep] | | under settings [rep] | |
| **VLC** | a desktop app's toolbar under the picture | Shift/Alt/Ctrl/Ctrl+Alt arrows for 5 s, 10 s, 1 min, 5 min [doc] | `V` subtitles, `B` audio [doc] | ([hotkeys](https://wiki.videolan.org/QtHotkeys/)) |

Three agreements: the timeline is on top and full width; play is at the bottom left; full screen
is at the bottom right. The arrows step 10 s in every film service (YouTube's 5 s is the outlier).
And Up and Down are the volume in every one that documents keys.

Apple's HIG on a custom player: *"reference the behavior and interface of the system video
player … A custom experience that diverges slightly from the system-provided experience can cause
frustration"*; and *"people expect to press Space … to play or pause"* on every device [doc]
([HIG, playing video](https://developer.apple.com/design/human-interface-guidelines/playing-video)).

## 5. Touch: what the players do

- **YouTube:** double-tap a side for 10 s [doc]; slide up from the scrubber for *"a row of
  thumbnails"*, chapter lines, a vibration and *"Release to cancel"* [doc]
  ([12825599](https://support.google.com/youtube/answer/12825599?hl=en&co=GENIE.Platform%3DAndroid)).
  No volume control on screen [K].
- **Infuse:** two-finger tap plays or pauses; double-tap sides step 10 s; dragging left or right
  scrubs *"the longer the drag, the further"*; up and down on the right half is volume, on the left
  brightness [doc].
- **Netflix, Disney+, Prime Video, Apple's iOS player:** a centre cluster of −10, play, +10, the
  timeline along the bottom, track and full-screen buttons at the corners, no volume [K].
- **Jellyfin:** the computer's layout with the volume removed (`PhysicalVolumeControl` on
  `browser.mobile` and `browser.ipad`), and the rewind and forward buttons removed under 50em
  wide [code].
- **Apple:** *"Don't use a slider to adjust audio volume"* on iOS; use the system's volume view,
  which a web page cannot [doc].

## 6. Remotes: what the players and platforms do

| | Hidden: Left/Right | Hidden: OK | Up/Down | Hold | Tracks |
| --- | --- | --- | --- | --- | --- |
| **Android TV** (Leanback, quality rules) | ±N s [doc] | pauses (TV-PC) [doc] | *"peeks up controls but does not pause"* [doc] | scrubs [doc] | n/a |
| **Fire TV** | the D-pad moves focus; Left/Right *"move inside media scrubbers"*; Menu shows and hides the chrome [doc] | | | Rewind and Fast Forward keys scrub or skip [doc] | |
| **tvOS / Apple TV app** | ±10 s [doc] | the clickpad centre plays or pauses [doc] | a rested finger or Up shows controls; swipe up for subtitles and audio [doc] | 2×, 3×, 4× [doc] | buttons on the bar; skipping back 10 s turns subtitles on [doc] |
| **Netflix TV** | ±10 s [rep] | pauses [rep] | Up shows the controls [rep] | faster [rep] | a panel from the bar [K] |
| **Jellyfin TV** | step and show the bar (the model Orpheum copied) | opens the bar | open the bar | | buttons on the bar [code] |
| **Kodi** | small steps, 10 s, escalating on repeats; 750 ms skip delay [doc] | opens the OSD; long press plays/pauses [code] | chapter or big step; long press cycles audio and subtitles [code] | | `l` subtitle dialog; Ctrl+`a` audio [code] |
| **Infuse tvOS** | edge clicks 10 s or chapters [doc] | plays/pauses [doc] | swipe down for options [doc] | faster with each click [doc] | the swipe-down panel [doc] |
| **Plex** | its tvOS player's scrubbing is its loudest complaint: *"imprecise, unpredictable"* [rep] ([Plex forum](https://forums.plex.tv/t/apple-tv-use-tvos-player-ui/903103?page=2)) | | | | |

Sources: [Android transport controls](https://developer.android.com/training/tv/playback/leanback/transport-controls),
[Android TV app quality](https://developer.android.com/docs/quality-guidelines/tv-app-quality),
[Fire TV controller guidelines](https://developer.amazon.com/docs/fire-tv/controller-behavior-guidelines.html),
[Apple TV, control playback](https://support.apple.com/guide/tv/control-playback-atvb120650e1/tvos),
[Apple TV, control video playback](https://support.apple.com/guide/tv/atvb7944597f/tvos),
[Apple HIG, remotes](https://developer.apple.com/design/human-interface-guidelines/remotes),
[Kodi skip steps](https://kodi.wiki/view/Skip_steps), Kodi's
[keyboard.xml](https://github.com/xbmc/xbmc/blob/master/system/keymaps/keyboard.xml),
[Roku trick mode](https://developer.roku.com/docs/developer-program/media-playback/trick-mode/trick-mode.md),
Netflix TV as reported in [comic-reader #991](https://github.com/dougiefresh49/comic-reader/issues/991).

Apple's remote guidance also bears on the bar's taps: *"avoid responding to taps during live video
playback"*, since a thumb resting on the remote taps it [doc].

## 7. Expert and standards guidance, gathered

- **Apple HIG:** follow the system player's behaviour; Space plays and pauses everywhere; overlays
  defer to the content; *"make sure your actions don't take more than a step or two"* on tvOS;
  tvOS has no sliders; iOS uses no slider for volume; a vertical slider has its minimum at the
  bottom [doc].
- **Android TV and Fire TV:** OK pauses, Left and Right step, a hold scrubs, Up and Down peek,
  Menu toggles the chrome, volume is the system's [doc].
- **Roku:** thumbnails while seeking anything over 15 minutes, a certification rule [doc]
  ([certification](https://developer.roku.com/dev/docs/cert-tests)).
- **Material 3:** sliders horizontal or vertical; volume a named use; vertical not for range
  sliders [doc].
- **NN/g:** people check time left by hovering the controls; give control over start, stop, mute
  and volume [doc]; a slider is hard to hit precisely when narrow, and needs feedback within
  0.1 s [doc]. NN/g has no study of player layouts as such.
- **Baymard:** its video research is about product videos on shop pages, and has nothing on a
  film player's controls ([product page
  videos](https://baymard.com/blog/embedding-product-page-videos)).
- **WCAG 2.2:** 1.4.13 for anything that pops up on hover or focus; 2.5.7 for dragging; 2.1.4 for
  single-letter keys (on only while the player has focus); 1.4.11, controls at 3:1 against what is
  behind them, which over video means the scrim; 2.5.8 targets of at least 24 px (AA), 2.5.5
  44 px (AAA); 2.4.11, focus not hidden by the bar [doc].
- **W3C media requirements:** everything by keyboard (IC-1); captions on and off by the user
  (CC-24); separate volumes for separate tracks where they exist (CA-3, DV-6), which is what
  Watch Together's film and partner volumes already are [doc].

---

## 8. For the owner: decisions and defaults

**Decisions, the owner's:**

1. **The volume's form on a computer:** a vertical pop-up from the sound button (recommended, §3),
   or YouTube's collapsed horizontal slider.
2. **No volume on touch:** remove the slider (and the mute button?) where the primary pointer is
   coarse, as Jellyfin does, or keep it on Android.
3. **Up and Down on a computer:** the volume (every service's keys; Tab then enters the bar), or
   moving into the bar (as built).
4. **OK on a TV while the bar is hidden:** opens the bar (as built, Jellyfin's) or pauses
   (Android TV's TV-PC rule, Netflix's, Apple's clickpad centre).
5. **One 10 s step everywhere,** or YouTube's 5 s arrows beside 10 s `j`/`l` as web-client.md
   wrote.
6. **Moving the track and sound controls** from under the film into two panels in the player.
7. **Thumbnails while seeking:** schedule the node's image track (a feature, not a widget).
8. **Holding Left or Right on a remote goes faster**, and repeated presses run together into one
   seek: the scheme (Apple's speeds or Kodi's growing steps).

**Defaults, unless the owner says otherwise:** the timeline above the buttons; the computer's row
as §2.1 draws it; the touch centre cluster and tap-to-toggle; double-tap sides for 10 s; hiding
when the mouse leaves; the pointer hidden with the bar; a time tooltip on the timeline; D188's
keys, already decided, built; `c` toggles subtitles; the 5 s timeout kept.
