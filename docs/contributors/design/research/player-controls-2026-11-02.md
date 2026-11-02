> **ABOUTME:** Which controls the player's overlay should carry and where, gathered before the
> overlay is redesigned: what Orpheum shows today on each device, what the competitors show, what
> viewers want and complain about, and which purpose-built remote buttons reach a web page. Ends
> with the common core, the open questions and three candidate layouts for the owner. Proposes,
> decides nothing.

# The player's overlay controls: what others show and what remotes offer (2026-11-02)

- **Asked by the owner, 2026-11-02:** *"Before we fiddle too much with the video overlay controls,
  I'd like to give some thought to what controls should be there, and the layout of those
  controls. Right now it's all tekitou, having accumulated controls with no real design. Do a
  research round to see what our competitors do, what controls people actually want (and don't
  want to see), and how we can make use of purpose-built buttons on various remote controls."*
- **Builds on** [playback-ui-2026-10-28.md](playback-ui-2026-10-28.md), which answered the
  volume question and proposed per-input layouts. The owner answered its eight questions on
  2026-10-28 ([navigation-and-widgets-answers-2026-10-27.md](../plans/navigation-and-widgets-answers-2026-10-27.md),
  "The player's controls"). This round does not repeat that work. It adds the inventory, the
  wider field, what viewers say, and the remote's buttons.
- **Method:** the code read at "fix(together): hand members the STUN servers, so a listener outside the house is reached"; prior research and decisions; vendors' help and
  developer pages, the press, and vendor forums. Reddit could not be fetched directly, so its
  sentiment arrives only second-hand.
- **Labels:** **[V]** read today in a vendor's page, a standard, or source code; **[P]** a press
  report or forum thread read today; **[K]** carried from prior research, which cites its own
  source; **[I]** inferred, or a secondary source not checked.

## Summary

Today's overlay is a single bar of three to five buttons over a timeline: play, full screen and,
off the TV, mute and volume. The audio, subtitle and sound controls sit under the film, outside
the stage, so a TV in full screen cannot reach them while the film plays. Across the field, the
common core is small and stable: a full-width timeline with times, play/pause, a 10 s step each
way, one **audio & subtitles** panel, next episode and episodes for series, and full screen off
the TV. Everything else (speed, quality, sound settings, subtitle styling) sits behind one
settings button. On a TV the trend in 2025–26 runs one way. Netflix's TV test removes the
on-screen play and back buttons and leaves those to the remote. YouTube's TV player now shows
the UI on the first Left or Right, not a seek. Google's guideline has OK pause and show the
controls, and Up or Down peek at them. Viewers' loudest complaints are about custom players that
break habits: seeking that stops working (Netflix and Plex on Apple TV), more presses for the
same action (Plex on Fire TV), and a bar that covers subtitles while paused. Skip Intro is wanted
and is not resented when it lands right. Of the purpose-built buttons, only play/pause reaches a
page in Silk. Amazon says Menu, Home and voice "cannot be captured" by a web app. Rewind, fast
forward and the volume keys do not arrive (our own test). So in Silk every action must be
reachable by the D-pad and OK, and Menu is a bonus at most, untested on our stick. The owner's
open questions are in §6.4; three candidate layouts follow in §6.5.

---

## 1. What we have now ("fix(together): hand members the STUN servers, so a listener outside the house is reached")

One bar, `PlayerControls` (`web/apps/web/src/player-controls.tsx`, D369), is composed per device.
Its row is play, the 10 s steps where asked, an *aside* the page fills, full screen, then mute
and volume, with the timeline (`timeline.tsx`, "0:41:07 / 2:23:17") **under** the row. Every icon
button is 2.75rem.

### 1.1 Per device

| Device | Over the film (inside the stage, so in full screen too) | Shows / hides | Keys that reach it |
| --- | --- | --- | --- |
| **Computer** (`page-controls.tsx`) | Bottom bar: Play/Pause, Full screen, Mute + horizontal volume (last, so Left/Right are not eaten); timeline below | Shown at open, while paused, on a mouse move, a click/tap, a key while focus is in the player, focus entering the bar; hides after 5 s still while playing (`overlayShownForMs`) | Space/OK play/pause; Left/Right on the video or timeline seek 10 s, growing to 30 s after 2 s and 60 s after 4 s of holding, one seek as presses rest (`seek-ahead.ts`); Down on the video enters the bar; click toggles play, double click full screen; Media Session `seekbackward`/`seekforward` (play/pause via `watchMediaSession`) |
| **Phone/tablet** | Same bar; the volume slider only where the page can set volume (not iOS) | A tap **shows** it; no tap hides it; 5 s timeout | No double-tap seek, no gestures |
| **TV** (`tv-controls.tsx`; on `isTelevision` UA) | Opens full screen. Top: "‹ S1E1 Pilot" (Up) and an on-screen **Back** (TV Bro's remote Back never reaches the page). Row: Play/Pause, "Ends at 9:41", Full screen. Timeline below. No volume | States `none` / `scrub` / `all`. Hidden: Left/Right seek and show the timeline alone; OK, Up, Down open all with focus on Play. All: Back hides; Left/Right on the timeline seek, elsewhere move focus. Hides 5 s after the last press while playing; never while paused | Arrows, OK, `MediaPlayPause` (179); Back as `popstate` |
| **Cast receiver** (any device) | The TV overlay without Up/Back; the receiver's lines over it | as TV; a mouse move opens it (D338 §5) | as TV |
| **Cast remote** (sender, `session-remote.tsx`) | Play, −10, +10, timeline, not over a film; audio menu in `cast-remote.tsx` | always shown | — |
| **Up next / Still watching** (`up-next.tsx`) | Card over the film 30 s before the end; 10 s countdown with autoplay; Still watching after 3 autoplays | Takes focus at the end; Left/Right go between its answers (VQ69 fix "fix(web): Right on a television's Still watching reaches Stop, not a seek") | — |

**Under the film, not in the stage** (`play.tsx`, `UnderTheVideo`): Start over, Subtitles, Find
subtitles, Audio track, Levelling, Sound offset, Dialogue boost (where the track has a centre),
Play original (on a ladder), the bare player's button and the debug link's tools. Above the film:
the trail and the title. In full screen, none of these show. On a TV, which opens full screen,
none can be reached while the film plays.

**Watch Together's additions**, all outside the film (`together-host.tsx`, `together-viewer.tsx`,
`together-voice.tsx`):

- **Host:** "Sound on", "You're hosting", news of who joined, voice (Talk/Stop talking, partner
  volume), and the film's own volume slider, under the player.
- **Guest by link:** the **browser's own** `<video controls>` when the host grants play or seek
  (D369 named this as waiting), plus "Sound back", a Full screen button, their own volume, voice
  and Leave, all outside the stage.
- So in full screen nobody in a session sees voice or Leave.

### 1.2 What this adds up to

- The bar's row was never designed as a whole. Play and full screen are on every device. Volume
  depends on the device. The steps appear only on the cast remote. The clock appears only on the
  TV, as "Ends at". Tracks never appear in the bar.
- **The Fire TV finding of today:** the whole overlay hides 5 s after the last press. A viewer who
  opens it with OK (focus on Play) and pauses to read before pressing Right finds it hidden, and
  Right, with the overlay hidden, seeks (`answerWhileHidden`). Moving focus and seeking share one
  key, and the timer decides which one you get.
- **A tension with a standing contract:** [tv-client.md](../plans/tv-client.md)'s remote contract
  (from D188) says *"Left and right always seek from the current position, from any on-screen
  state, whatever has focus"*. The as-built overlay walks its row with Left and Right instead. That
  row is the only way to Full screen today.
- **Of the owner's 2026-10-28 answers**, built: one 10 s step; holding grows the step (Kodi's
  scheme, built as 10/30/60 s). **Not yet built:** OK pauses on a TV with the bar hidden (#4; as
  built OK opens the bar, "fix(web): OK on a television's hidden player overlay puts focus on play"); the vertical volume pop-up (#1); no volume on touch (#2);
  Up/Down as volume on a computer (#3); the Audio & subtitles and Settings panels inside the
  player (#6).
- **Keys planned and not built** ([web-client.md](../plans/web-client.md), D188): `k`, `j`/`l`,
  `f`, `m`, `c`, `0`–`9`; Media Session `nexttrack`, `togglemicrophone`, `hangup`.

## 2. What is settled, and what is open

| Settled | Where |
| --- | --- |
| The app's own controls on every device, one widget set, one bar | D369 §1–§2 |
| Track menus are visible buttons on the bar, one level deep, each track named | tv-client.md (D188) |
| Caption settings reachable during playback in at most two remote presses | D90; tv-client.md |
| 10 s steps; scrub thumbnails eventually | tv-client.md; owner 2026-10-28 #5, #7 |
| No volume on TV (system's) or touch (device's); vertical pop-up on a computer | owner 2026-10-28 #1, #2 |
| OK pauses on TV with the bar hidden | owner 2026-10-28 #4 (not built) |
| Two panels in the player: Audio & subtitles; Settings (boost, levelling, offsets) | owner 2026-10-28 #6 (not built) |
| Back is the history; the TV overlay keeps an on-screen Back beside its Up | D367 §6, §8; README |
| Subtitles move up while the controls show | web-client.md (from jellyfin-web's largest complaint class) |
| Next episode after a visible countdown with cancel; Still watching stops the chain | U11 ([ux-principles](ux-principles-2026-10-09.md)); D339 |
| Skip Intro needs media segments first; trust that it lands right matters more than the button | [media-segments.md](../plans/media-segments.md), D82, D115; [ui-competitors.md](ui-competitors.md) §1 |

**Open:** what the TV row holds and in what order; where focus lands when the overlay opens;
what Left/Right do while it shows (the D188 contract vs. walking the row); the timeout and what
pauses it; whether TV keeps an on-screen play button; where Watch Together's voice and Leave go
in full screen; next episode/episodes/skip on the bar; the phone's centre cluster and gestures
(proposed in playback-ui §2.2, not yet answered by the owner); and what Menu does where it
arrives.

## 3. Competitors

### 3.1 Layout and grouping

| Player | Top | Centre | Bottom | Behind a menu |
| --- | --- | --- | --- | --- |
| **Netflix web** [K] | back, title | — | timeline on top; left: play, −10, +10, volume; right: next episode, episodes, audio & subtitles, speed, full screen | audio & subtitles is one panel |
| **Netflix TV, test from 2026-07-29** [V] | — (the top-left back is **removed**) | — (on-screen play/pause **removed**; OK and the remote's play/pause do it) | remaining icons in a bottom-left cluster, Skip Recap among them; Next Episode and settings just above a larger progress bar | audio, subtitles and subtitle appearance merged into **one** menu, opening as a bottom overlay ([whats-on-netflix](https://www.whats-on-netflix.com/news/first-look-netflix-new-tv-video-player-redesign/)) |
| **YouTube web** [K] | title | — | timeline on top; left: play, next, volume, time, chapter; right: autoplay, captions, settings, theatre, full screen | gear: quality, speed, subtitles |
| **YouTube TV, 2025-12** [V] | title top-left, a Description button | play/pause and skip buttons (new) | timeline, subscribe | Left/Right now **open the UI** rather than seek ([9to5Google](https://9to5google.com/2025/12/12/youtube-redesigned-video-player-for-tvs-20th-birthday/)) |
| **Prime Video** [K] | tracks, settings, volume, full screen cluster top-right (web) | transport centred | timeline | X-Ray on Fire TV; Skip/Watch Credits buttons since 2025-05 [P] ([cordcuttersnews](https://cordcuttersnews.com/amazon-prime-video-app-update-gives-users-more-control-over-how-they-watch-tv/)) |
| **Disney+** [K] | — | transport over the timeline | timeline at the bottom | one audio & subtitles panel |
| **Apple TV app (tvOS)** [K] | — | — | timeline; info/subtitles/audio as buttons on the bar (moved there from the swipe-down panel in tvOS 15 for *"discoverability"*) | — |
| **Plex** [K][P] | minimise, full screen | — | settings (quality, audio, subtitles), chapters, play queue, volume | most things under settings; 2026.5.0 removed skip and next buttons ([forum](https://forums.plex.tv/t/2026-5-0-new-player-feedback-ui-bugs/937762)) |
| **Jellyfin web** [K] | back, title | — | timeline with times; previous, rewind, play, forward, next; time; favourite, subtitles, audio, mute + slider, settings, AirPlay, PiP, full screen | settings: quality, speed, aspect |
| **Emby** [P] | — | — | OSD with chapters, next episode; a user asked for *"the simplistic controls"* back; a hide-transport option was the answer ([forum](https://emby.media/community/topic/135323-tv-app-player-controls-simpler)) | — |
| **Infuse** [V] | — | — | timeline; options by swipe-down on tvOS | the options panel ([Firecore](https://support.firecore.com/hc/articles/215090987)) |
| **Kodi** [V] | — | — | OSD on OK: transport, then audio, subtitles, video settings, bookmarks | subtitle dialog `l`; audio/subtitle cycle by long Up/Down ([keyboard.xml](https://raw.githubusercontent.com/xbmc/xbmc/master/system/keymaps/keyboard.xml)) |
| **VLC / mpv** | toolbar under the picture (desktop app) | — | — | keys: mpv `j` subtitles, `#` audio; VLC `V`/`B` [V] ([mpv manual](https://mpv.io/manual/stable/)) |

Three agreements hold across the table. The timeline sits at the bottom, full width, with the
transport near it. Tracks are one button that opens a panel. Full screen comes last, on the
right, wherever the platform has one; TVs have none. The services differ mainly in how much
they put on screen at all.

### 3.2 On a TV with a D-pad

| | Overlay hidden: Left/Right | Hidden: OK | Up/Down | Focus when it opens | Hiding |
| --- | --- | --- | --- | --- | --- |
| **Android TV guideline** [V] | ±N s; hold scrubs | play/pause; pausing shows the controls | *"peek"* controls without pausing | — | — ([controls](https://developer.android.com/training/tv/playback/controls)) |
| **Fire TV guideline** [K] | arrows move focus; Left/Right move inside scrubbers | — | — | — | Menu shows and hides the chrome (native apps) |
| **Apple tvOS** [K] | click edge ±10 s; ring/swipe scrubs | clickpad centre plays/pauses | rested finger or Up shows timeline; Down shows info (subtitles, audio) | timeline | — |
| **Netflix TV** [K][V] | ±10 s | pauses | Up shows controls | — | Back returns (no on-screen back in the 2026 test) |
| **YouTube TV 2025** [V] | **open the UI**; a reader reports focus then on the progress bar, so the second press seeks [I] | — | — | progress bar [I] | — |
| **Jellyfin TV** [K] | step and show the bar (what Orpheum copied) | opens the bar | open the bar | play | 3 s [K] |
| **Kodi** [V] | step, escalating; 750 ms skip delay | opens the OSD; long press plays/pauses | chapter / big step | — | — |
| **Emby Android TV** [P] | ±10 s | — | chapters; Up shows the chapter row | — | — |
| **Infuse tvOS** [V] | edge click 10 s (or a chapter); hold for faster | plays/pauses | swipe down: options | — | — |
| **Roku** [V] | — | — | — | — | ★ opens options (captions, audio); Instant Replay steps back ([Roku](https://developer.roku.com/dev/docs/remote-control-buttons)) |

**The pattern:** with nothing shown, Left/Right seek and OK pauses (Android, Apple, Netflix),
or the first press shows the UI (YouTube since 2025, Jellyfin for OK). Up/Down **peek**. When
the controls show, the timeline is what Left/Right work on, and the other actions sit in a
separate row or panel that you reach with Down (Netflix, YouTube) or open with a dedicated key
(Menu on Fire TV, ★ on Roku, swipe-down on Apple). None of the big players documents its
timeout; Jellyfin's is 3 s, ours 5 s.

### 3.3 On a phone

- **Double-tap a side for ±10 s:** YouTube [K], Netflix [P, 2018], Infuse [V], VLC (and it can
  be turned off) [P].
- **Centre cluster** (−10, play, +10), timeline at the bottom, tracks and full screen in the
  corners: Netflix, Disney+, Prime, Apple's iOS player [K].
- **Vertical swipes** for volume (right) and brightness (left): Infuse [V], VLC's design wiki
  [P]. Neither is possible for a web page on iOS, where the page cannot set volume.
- **Tap toggles** the controls (shows them, and hides them again) [K]; Orpheum's tap only shows.
- **Two-finger tap** plays/pauses: Infuse, VLC [V][P].

### 3.4 Skip intro, credits, next episode

- **Skip Intro** (Netflix): shown only when needed, one press, *"136 million times a day"* [K].
  Netflix's `S` key presses it on the web [V].
- **Plex:** Skip Intro and Skip Credits each Disabled / Manual (a button, default) / Automatic;
  Up Next "during final credits" by default; autoplay countdown default None, up to 30 s [P].
- **Prime Video** (2025-05): next video after a countdown by default; Skip Credits and Watch
  Credits buttons on every device [P].
- **Emby** added Skip Intro to its OSD (Android 3.2.51, server 4.7+) [P].
- Orpheum: Up next 30 s before the end and a 10 s countdown (up-next.md); Skip Intro waits on
  media segments.

## 4. What viewers want and do not want

| Theme | Evidence | Strength |
| --- | --- | --- |
| **A custom player that breaks the habitual seek** is the loudest complaint | Netflix's custom tvOS player (2026-04) dropped Left/Right 10 s skips and Enhance Dialogue ([BGR](https://www.bgr.com/2146676/apple-tv-netflix-app-updated-controversial/)); Plex's tvOS player *"imprecise, unpredictable"* and *"unusable"* for subtitle readers, users ask for the native one ([forum](https://forums.plex.tv/t/apple-tv-use-tvos-player-ui/903103?page=3)); HBO Max restored the native tvOS player in 2021 ([9to5Mac](https://9to5mac.com/2021/06/09/hbo-max-restores-native-video-player-interface-on-apple-tv/)) [P] | Strong |
| **More presses for the same action** | Plex on Fire TV: menu actions from about 2 presses to about 6 ([PiunikaWeb](https://piunikaweb.com/2026/05/04/plex-fire-tv-redesign-unusable-users-slam-navigation/)); Plex 2026.5.0's episode list cannot be closed, its menu often scrubs instead of opening [P] | Medium |
| **The bar covers subtitles while paused** | Jellyfin (a deaf family member could not follow), Infuse, VLC, Tablo, Roku Media Player threads; users ask for subtitles to move above the bar, or the bar to hide during a pause [P] ([Jellyfin](https://forum.jellyfin.org/t-how-to-hide-the-progress-bar-during-pause-unless-the-mouse-moves-subtitle-visibilit), [Infuse](https://community.firecore.com/t/shift-subtitles-above-the-progress-bar-while-pausing/15587)) | Strong |
| **A quick skip should not bring up the whole UI** | Infuse request to suppress the GUI on a tap-skip ([forum](https://community.firecore.com/t/infuse-tvos-suppress-playback-gui-during-skipping-via-siri-remote-tap/56270)) [P]; Orpheum already shows the timeline alone after a seek | Weak |
| **Arrows that do something unexpected** | YouTube TV's 2025 change: *"a muscle memory I'll have to unlearn"* [V]; Samsung focus jumping to Pause after a seek and seeks snapping to marks [K]; Jellyfin TV needing Play after a seek [P] | Medium |
| **Less on screen** | Jellyfin's community "simple OSD" script (pause and timeline only) ([GitHub](https://github.com/JSethCreates/jellyfin-script-controlOSD)); Emby's "simplistic controls"; Netflix's own test removing on-screen play and back [P][V] | Medium |
| **Skip Intro/Credits wanted, and wanted reliable** | Plex's manual/auto setting; Plex's semi-persistent auto-skip request, 125 votes [K]; Prime's double press, Plex's hidden Skip Credits, Jellyfin's plugin dependence as failure reports [P]. No evidence found of people disliking the button itself | Strong (wanted) |
| **Autoplay and next episode under the viewer's control** | Plex's countdown default None; Prime's 2025 Watch Credits; Tristan Harris on autoplay [K] | Medium |
| **Pause screens with logos, dimming or ads** | Roku and Emby threads [P] | Weak–medium |
| **Tracks you cannot tell apart; defaults at the wrong scope** | Plex *show track names* 417 votes; per-series subtitles 701 votes [K] | Strong |
| Volume on a TV or phone overlay | none found asking for it; nobody shows it [K] | — |

**Not found:** any study ranking which player controls people use most. The design guides list
play/pause, next episode, volume, subtitles and full screen as the usual set, but give no data
for it. "Are you still watching" complaints were not found specifically for players.

**Accessibility** [V]:

- WCAG 2.2 **2.5.8** (AA): pointer targets at least 24×24 CSS px. **2.5.5** (AAA): 44×44. Our
  2.75rem (44 px) meets both.
- **2.4.11** (AA): a focused control must not be hidden entirely by author content. The overlay
  must not hide what has focus, and a hidden overlay must not keep focus.
- **2.2.1** (timing): whether an overlay's auto-hide is a "time limit" is unsettled [I]. It hides
  only controls that come back, but a slow viewer meets it, as today's Fire TV finding shows.
- **W3C MAUR:** everything by keyboard (IC-1). The user turns captions on and off (CC-24) and
  chooses among the tracks during playback (DAC-7)
  ([MAUR](https://www.w3.org/TR/media-accessibility-reqs/)).
- **FCC 47 CFR 79.107(a)(4)(vi)–(vii):** a function to turn captions on and off, and the display
  settings, both accessible ([eCFR](https://www.ecfr.gov/current/title-47/part-79/section-79.107)).
- **FCC 24-79** ("readily accessible" settings) names the factors proximity, discoverability,
  previewability, consistency and persistence. D90 already plans for it. The phrase
  *"comparable prominence to volume"* belongs to a Section 508 proposal (36 CFR 1194, 413.1.1),
  not to FCC rule text [I]. No source says "top level".

## 5. Remote buttons

### 5.1 Fire TV and Silk (the box that matters)

What we know from our own tests, in
[idiosyncrasies.md](../../idiosyncrasies.md#silk-on-fire-tv-keeps-the-d-pad-for-its-pointer-unless-its-spatial-navigation-mode-is-on),
is that Silk 138 on an AFTSSS behaves differently from what Amazon documents for its web-app
runtime.

| Button | In Silk (spatial mode), to the page | Amazon's web-app runtime [V] | Convention (native apps) |
| --- | --- | --- | --- |
| D-pad, OK | `Arrow*`; `Enter`/`NumpadEnter` 13 (a held OK repeats as fresh presses with no key up) | 37–40, 13 | move focus; activate |
| Back | never a key: a history step (`popstate`) | keyCode 4 | previous screen |
| Play/Pause | `MediaPlayPause` 179; no Media Session action | 179 | toggle |
| Rewind / Fast forward | **do not arrive** | 227 / 228 | *"skips backward/ahead in media contexts"* [V] |
| Volume, mute | do not arrive (system) | — | system |
| **Menu (≡)** | **untested.** Amazon: *"The Home, Menu, and Voice Search buttons cannot be captured"* [V]. Consumer guides say Silk opens its own menu on it [I]. If Chromium passed it, it would be `ContextMenu`, keyCode 93 (Chromium maps `KEYCODE_MENU` to `VKEY_APPS`) [V, source] | cannot be captured | **opens options**: in Prime Video, Menu during playback opens subtitles and audio [V]; native `KEYCODE_MENU` opens the options menu ([remote input](https://developer.amazon.com/docs/fire-tv/remote-input.html)) |
| Held Up | Silk takes it for its address bar and menu | — | — |
| Home, voice, Guide, app buttons, Pro's 1/2/★/Settings | not to the page [V for Home/voice; I for the rest] | not captured | system |

Sources: [Supporting controllers in web apps](https://developer.amazon.com/docs/fire-tv/supporting-controllers-in-web-apps.html),
[remote input](https://developer.amazon.com/docs/fire-tv/remote-input.html), Chromium's
[`keyboard_code_conversion_android.cc`](https://raw.githubusercontent.com/chromium/chromium/main/ui/events/keycodes/keyboard_code_conversion_android.cc).
Amazon also warns that play/pause, rewind and fast forward *"might not be present on all
remotes"*, so the D-pad must always suffice [V].

**The Menu test was not run.** The Fire TV (`192.168.1.150`) was asleep, display off, on its
home screen, with Silk in the background. Running the test would have meant waking it, which may
turn the television on over HDMI-CEC, and bringing Silk to the front. That would disturb the
household, so it was skipped. The test takes a minute when the owner is at the TV, or with his
leave:

1. Open any Orpheum page in Silk.
2. Over CDP, add a `keydown`/`keyup`/`popstate`/`visibilitychange`/`blur` logger.
3. Send `adb shell input keyevent 82`, then `89` and `90`.

Expected: nothing for any of the three, or Silk's own menu opening (a `blur`). If Menu does
arrive, look for `ContextMenu`/93.

### 5.2 The other platforms

| Platform | Options key | Seek keys | Reaches a web page? | Convention / guideline |
| --- | --- | --- | --- | --- |
| **Android TV / Google TV** | Menu/Settings, Streamer's ★ (remappable) | rew/ff on some remotes | Chromium maps keys as above (`MediaRewind` 227, `MediaFastForward` 228, `GoBack`, `ContextMenu`) if the browser receives them [V source]; browser delivery unverified [I] | OK pauses (TV-PC quality rule), Left/Right ±N s, hold scrubs, Up/Down peek [V] |
| **Apple TV** | none; swipe or press Down on the clickpad for info | ring/edge clicks | no browser | click edge ±10 s; Down: info with subtitles/audio; *"avoid responding to taps during live video"* ([HIG remotes](https://developer.apple.com/design/human-interface-guidelines/remotes)) [V] |
| **LG webOS** | (no menu key for apps); colour keys 403–406 on one Magic Remote model | 412 rew, 417 ff, 415 play, 19 pause, 413 stop (standard remote; the Magic Remote has none) | Back 461 (may arrive as key `Unidentified`); pointer and wheel as mouse; built-in browser unverified [V][I] ([Magic Remote](https://webostv.developer.lge.com/develop/guides/magic-remote)) | — |
| **Samsung Tizen** | Menu 18, Info 457, Guide 458 | 412, 417 (long press: track prev/next 10232/10233) | only arrows, Enter, Back (10009) without `tizen.tvinputdevice.registerKey`, which a page in the built-in browser cannot call [V][I] ([Samsung](https://developer.samsung.com/smarttv/develop/guides/user-interaction/remote-control.html)) | — |
| **Roku** | ★ opens the options (captions, audio); the app gets it if it shows a HUD | rew/ff at three speeds; Instant Replay | no browser | map Instant Replay's step to the first Left where the remote lacks it [V] |
| **Game controllers** | Start / Select | triggers rew/ff (our map) | on Fire TV, Silk takes the pad (idiosyncrasies) | [controller-input-2026-10-25.md](controller-input-2026-10-25.md) |
| **Desktop/phone media keys, headsets** | — | Media Session `seekbackward`/`seekforward` (we answer), `previoustrack`/`nexttrack` (we do not), `togglemicrophone`/`hangup` (planned for voice) | yes, as Media Session actions on Chrome 73+, Firefox 82+, Safari 15+; not in Android WebView [K] | lock-screen and notification controls; a headset's double press likely `nexttrack` [I] |
| **Keyboard** | — | YouTube: `j`/`l` 10 s, arrows 5 s on the bar, `k`, `f`, `m`, `c`, `0`–`9`, `<`/`>` speed; Netflix: arrows 10 s, Up/Down volume, `S` skip intro [V] | yes | D188 chose YouTube's map; the owner chose one 10 s step |

**The upshot.** In the browsers Orpheum actually runs in on a TV, the purpose-built buttons are
almost all taken before the page sees them. Play/pause is the one dependable exception. Where
Menu/★/Info/`ContextMenu` or the media keys do arrive (a packaged app later, Android TV
browsers, webOS's standard remote), the field's convention is clear enough to adopt cheaply:
**Menu/★ opens the options panel (audio & subtitles)**, rewind and fast forward step or scrub,
and next/previous go to the next or previous episode. The input layer
(`input-buttons.ts`) already names rewind, fast forward and the volume keys. It has no `menu`
command, though [controller-input](controller-input-2026-10-25.md) §6.1 listed one.

## 6. Synthesis (options for the owner, not a design)

### 6.1 The common core

Nearly every player shows these on its overlay:

1. **The timeline**, full width, at the bottom, with the time so far and the length or time left
   (the TV also shows "Ends at" in Apple's and Jellyfin's style).
2. **Play/pause.** Present on every touch and desktop player. On TV it is the one being dropped
   from the screen (Netflix's test) because OK and the play key already do it.
3. **±10 s steps.** Buttons on desktop and touch; on TV, the arrows or edge clicks.
4. **One audio & subtitles button**, which opens a panel.
5. **Title** (and episode), usually top-left or above the timeline.
6. **For a series:** next episode and an episodes list.
7. **Off the TV:** full screen, last. A desktop also has volume. The cast button sits at the top
   on phones.

### 6.2 Usually behind a menu

Speed; quality; subtitle appearance (size, colour; D90's set); sound settings (Orpheum's boost,
levelling, sound and subtitle offsets); aspect/zoom; chapters (Plex, Kodi; YouTube marks them on
the timeline); info/description (YouTube, Prime's X-Ray); "play original" and the debug tools
(ours alone). Skip Intro/Recap/Credits are never in a menu. They appear only when relevant, as
one large button above the timeline.

### 6.3 Patterns that work on a D-pad

- **Nothing shown:** Left/Right seek and show the timeline alone (Android, Apple, Netflix,
  Jellyfin, ours), *or* show the UI first (YouTube 2025, which drew a complaint). OK pauses
  (Android rule, Netflix, Apple; the owner's answer #4). Up/Down peek.
- **Shown:** focus starts on the **timeline** (Apple, YouTube TV) or on play (Jellyfin, ours).
  When focus is on the timeline, Left/Right keep seeking, which keeps D188's "always seek". The
  other buttons sit in a row that Down reaches (Netflix, YouTube) or behind a key (Menu, ★,
  swipe-down).
- **One action per key in each state, not decided by a timer.** Our finding today is what
  happens when a timer chooses between "move focus" and "seek". Players that keep focus on the
  timeline do not meet it, because Right seeks in both states.
- **The timer stops while focus is off the default control or a panel is open** (playback-ui
  §2.6 already proposes this for panels).
- **Back closes the innermost thing:** the panel, then the overlay, then the page.
- **Subtitles move up** while anything shows, paused included.

### 6.4 Open questions for the owner

1. **The TV's focus when the overlay opens:** on play (as built), or on the timeline (Apple,
   YouTube TV), so Left/Right keep seeking whatever shows (tv-client.md's D188 contract)?
2. **How the TV reaches its other buttons:** walked with Left/Right along one row (as built),
   or a second row reached by Down (Netflix, YouTube)?
3. **On-screen play/pause on TV:** keep it (and Back), or drop it as Netflix's test does, since
   OK and the remote's play key do it? We keep an on-screen Back for TV Bro, whose Back never
   reaches the page.
4. **The timeout:** keep 5 s; hold it while focus is anywhere but the default control; or
   lengthen it once the viewer moves off the default. WCAG 2.2.1 suggests the viewer at least
   never loses a choice mid-move.
5. **Full screen on TV:** the stage already opens full screen. Does the TV need the button at
   all, or only a "fill" fallback where the browser refused?
6. **Series controls on the bar:** next episode, episodes list, and later Skip Intro/Recap as an
   above-the-timeline button?
7. **Watch Together in full screen:** should Talk/mute and Leave live in the overlay (a "people"
   button or a small voice chip), and should the link guest lose the browser's own controls for
   ours (D369's waiting item)?
8. **The phone:** adopt playback-ui §2.2's centre cluster, tap-toggles and double-tap ±10 s?
   Those were proposed on 2026-10-28 and not yet answered.
9. **Menu and other keys where they arrive:** map `ContextMenu`/Menu/★/Info to "open Audio &
   subtitles" (Prime's and Roku's convention); map `MediaTrackNext`/`nexttrack` to next
   episode? This costs little in the input layer and does nothing on Silk.
10. **Title on the overlay:** today the TV shows "‹ S1E1 Pilot" as the Up link. Should it also
    show the film's own name, as every service does?
11. **May the Fire TV be woken** for the one-minute Menu/rewind/FF key test (§5.1)?

### 6.5 Three candidate layouts

All three keep the owner's settled answers: one 10 s step, OK pauses with the bar hidden, no
volume on TV or touch, the two panels inside the player, and subtitles moving up.

**A. One bar, made whole (closest to today; Netflix web / Jellyfin).** The same row on every
device, with what does not apply left out.

```
 ┌──────────────────────────────────────────────────────────────────┐
 │ ‹ S1E1 Pilot   [Back]                          Brass Arena · S1E2   │  TV: Up + Back; others: title
 │                                                                  │
 │                       [ Skip intro ]                             │  only when relevant
 │ ━━━━━━━━━━━━━━━━━━━━━━━●──────────────────────── 0:41 / 0:58     │  timeline, full width
 │ [▶] [−10] [+10] [Vol]  Ends 9:41    [Next] [Eps] [Aa] [⚙] [⤢]    │  row, walked by Left/Right
 └──────────────────────────────────────────────────────────────────┘
```

- **For:** one mental model, the smallest change from what is built, and every action visible.
- **Against:** on TV the row is long, about 8 stops. Left/Right walk it, so either they stop
  seeking (contrary to D188) or the timer bug's shape remains. Reaching Aa from Play takes 5
  presses.

**B. Timeline first on TV, actions a row below (Netflix TV / YouTube TV / Android TV).** The TV
gets its own arrangement; the computer keeps A.

```
 ┌──────────────────────────────────────────────────────────────────┐
 │ Brass Arena · S1E2 “The Ledger”                          Ends 9:41  │  info (Up peeks this)
 │                                                                  │
 │                       [ Skip intro ]                             │
 │ ━━━━━━━━━━━━━━━━━━━━━━━◉──────────────────────── 0:41 / 0:58     │  ◉ focus on open: Left/Right seek
 │ [Audio & subtitles] [Settings] [Next episode] [Episodes] [‹ Back]│  Down reaches; Left/Right walk it
 └──────────────────────────────────────────────────────────────────┘
   OK = play/pause anywhere on the timeline · Menu/★ (where it arrives) = Audio & subtitles
```

- **For:** Left/Right always seek on the timeline, which keeps D188's contract and removes
  today's timer bug. Audio & subtitles is two presses from anywhere (Down, OK), which meets D90's
  "two presses". No on-screen play is needed: OK and the play key do it, as in Netflix's test.
- **Against:** it diverges from the computer's bar. A viewer must learn that Down holds the
  actions. Dropping the play button removes a visible affordance for anyone who has not learned
  that OK pauses.

**C. Three zones, touch first (Netflix/Disney+/Prime on phones).** The phone and tablet get a
centre cluster. The computer and TV take A or B.

```
 ┌──────────────────────────────────────────┐
 │ ‹ S1E1    Brass Arena · S1E2      [👥] [Aa] │  top: way out, title, together, tracks
 │                                          │
 │        [↺10]      [ ▶ ]      [↻10]       │  centre, large; double-tap sides ±10
 │                                          │
 │ 0:41 ━━━━━━━━━●──────────── 0:58  [⏭][⤢] │  bottom: timeline, next, full screen
 └──────────────────────────────────────────┘
```

- **For:** the thumb's zone; matches every big service on phones; leaves room for Watch
  Together's people/voice chip at the top.
- **Against:** a third arrangement to keep consistent. The centre cluster sits over the picture.
  It is poor on a D-pad (focus would hop across the screen), so it should not be used on TV.

A likely combination, offered as an option and not a recommendation: **A** on the computer,
**B** on the TV, **C** on touch, all built from the one `PlayerControls` widget set (D369 §1),
with the composition differing per device.

## 7. Not verified

- **Menu, rewind and fast forward in Silk** (§5.1): not run, because the TV was asleep.
- **Netflix, Disney+, Prime: overlay timeout and focus on open:** documented nowhere found. Only
  Android's guideline and the press accounts above.
- **Netflix's 2026 TV player beyond the test article:** whether the test has rolled out, and its
  D-pad behaviour.
- **YouTube TV:** that focus lands on the progress bar after the first press comes from one
  reader comment.
- **Apple's tvOS help pages:** returned only landing pages today. Apple's behaviour here is
  carried from playback-ui (2026-10-28).
- **Disney+:** nothing primary; IINA's guide page returned 404.
- **Reddit:** not fetched. Sentiment comes from vendor forums and the press.
- **LG and Samsung built-in browsers' key delivery:** not found. The sources cover packaged apps.
- **Media Session:** how each desktop's media keys and an iOS headset map to actions is not
  verified per browser.
- **FCC 24-79's compliance date:** sources conflict (2024-09 vs 2026-08). D90 records 2026-08-17.
