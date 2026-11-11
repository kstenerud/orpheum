> **ABOUTME:** Which controls the player's overlay should carry and where, gathered before the
> overlay is redesigned: what Orpheum shows today on each device, what the competitors show, what
> viewers want and complain about, and which purpose-built remote buttons reach a web page. Ends
> with the common core, the open questions and three candidate layouts for the owner. Proposes,
> decides nothing. §8–10 are follow-ups from the design conversation that followed (D407).

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
 │ ‹ S1E1 Pilot   [Back]                          Tin Comet · S1E2  │  TV: Up + Back; others: title
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
 │ Tin Comet · S1E2 “The Spoon”                          Ends 9:41  │  info (Up peeks this)
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
 │ ‹ S1E1    Tin Comet · S1E2     [👥] [Aa] │  top: way out, title, together, tracks
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

## 8. Skip intro, Up next and Still watching: how others guard OK (follow-up)

*Asked when the owner's draft had OK press the convenience area (D407 §3). Decided there: OK presses it, with no guard.* Reported 2026-11-02 in the design conversation; tags as above: [V] verified, [P] press or forum, [I] inferred.

I couldn't verify the main question for the big services. Netflix, Prime Video, Disney+, Hulu, Paramount+ and the Apple TV app don't document whether the TV app's Skip button takes focus or what OK does while it shows. What I could check comes from Jellyfin's and Kodi's source code, Apple's developer docs, and Plex's, Emby's and Max's help pages. None of those players protects OK with a delay or a confirm step.

Jellyfin's Android TV app already works the way the owner's draft does. Kodi's Up Next add-on has just added a setting that addresses the exact risk.

Tags: [V] official doc or source code, [P] press or forum, [I] my inference. I left out what §3.4 of the prior research already covers. No repository file was touched. Clones and fetched pages are under `/yoloai/cache/repos/` and `/yoloai/cache/http/`. I used about 31 web searches.

### 8.1 Per player

#### Jellyfin, Android TV app (0.18+, media segments, which arrived with server 10.10)
- **Detection.** The server needs a segment provider plugin: Chapter Segments Provider reads chapter names, and Intro Skipper detects intros from audio fingerprints [P] ([forum](https://forum.jellyfin.org/t-intro-skipping-isn-t-working-on-jellyfin-for-android-tv)). Segment types are intro, outro, preview, recap and commercial. Each type can be set to Skip, Ask to skip or Do nothing; intro and outro default to Ask [V] ([release post](https://jellyfin.org/posts/androidtv-v0.18.0/), `UserPreferences.kt`).
- **Ask to skip.** A small "(Select icon) Skip" box appears at the bottom right. OK seeks to the segment's end. Back dismisses it without seeking. It hides itself after 8 s. It is shown only while the playback controls, guide and popups are hidden, so it always appears alone [V] (`CustomPlaybackOverlayFragment.java`, key listener around line 485; `MediaSegmentRepository.kt`).
- Only `DPAD_CENTER`, `ENTER`, Back, `BUTTON_B` and Escape are intercepted. The media play/pause key still pauses [V, code].
- **Guards.** There is no delay before OK works. Segments shorter than 3 s get no prompt. The prompt fires only when playback crosses the segment's start, not when you seek into the middle of it [V] (`MediaSegmentAction.kt`).
- **Up Next.** A full-screen Next Up screen appears after the episode ends. Its timer defaults to 7 s and the screen can be Extended, Minimal or Disabled [V] (`UserPreferences.kt`).
- **Still watching.** Off by default. When on, it triggers after 2/3/5/8 episodes or 1/1.5/2.5/4 hours [V] (`StillWatchingBehavior.kt`).

#### Jellyfin web
- **Skip button.** Labelled "Skip Intro/Outro…", hides after 8 s. Opening the controls brings it back, kept visible but not focused [V] (`skipsegment.ts`).
- **Focus.** In TV layout the button takes focus if nothing else has it, so Enter activates it. In desktop layout it is not focused, so Space still pauses [V; the Space behaviour is I].
- **Seeking back.** Seeking back into a segment does not auto-skip it again [V] (`mediaSegmentManager.ts`).
- **Up Next.** Shows 30 s before the end, or 35 s for episodes of 40 min or more and 40 s for 50 min or more. It counts down to the end of the file. TV layout focuses "Start now". If an outro runs to the file's end and Up Next is due, the Skip button is suppressed [V] (`video/index.js`, `upnextdialog.js`).
- **Intro Skipper plugin.** Detects with Chromaprint audio fingerprints across a season, plus silence, black frames, chapter names and subtitles. It keeps an epilogue that follows a credits chapter as a separate segment. Since 10.10 it does not change the UI [V] (`intro-skipper` README, `docs/credits.md`).

#### Kodi
- **Core player.** No skip-intro button. EDL "Commercial Break" ranges are skipped automatically once, and seeking back lets you watch them [V] ([Kodi wiki](https://kodi.wiki/view/Edit_decision_list)).
- **Up Next add-on (v1.3.x, released 2026-10-24 to 11-02)** [V, code and README]:
  - **Popup timing.** Default 30 s before the end, or 15–60 s scaled to episode length. Default action: play next.
  - **Countdown keys.** Back cancels the countdown and keeps the episode playing. Pause freezes it along with the video.
  - **Still watching.** "Still there?" after 3 episodes.
  - **Credits.** An optional "advance during credits after N s" is off by default. The README warns that "an early transition can skip post-credits scenes."
  - **New in v1.3.1: "Use OK/Select for playback controls"** (off by default). While the popup shows, focus sits on an invisible control, so Select opens the player's controls. Select only acts on Next/Close once the viewer navigates into the popup; Up returns to the neutral spot.
  - **v1.3.0: Skip intro / Skip recap button**, fed by the add-on's segment data. Auto-skip is off by default. Back dismisses it for that segment.

#### Plex
- **Intro detection.** Audio matching across a season's episodes. Intros under 20 s, or ending past the episode's halfway point, are ignored. The skip lands "a couple of seconds before the intro ends" [V] ([support](https://support.plex.tv/articles/skip-content/)).
- **Credits detection.** On by default, with markers shared through an online cloud. Skip Credits appears "when the credits start" and jumps to any extra scenes detected during or after the credits. At the final marker some apps shrink the video into the post-play screen [V] ([support](https://support.plex.tv/articles/credits-detection/)).
- **Player settings** [P] ([forum mirror of the docs](https://forums.plex.tv/t/player-experience/857990/1.md)):
  - Skip Intro and Skip Credits: Disabled / Manually (default) / Automatically.
  - Up Next "During final credits" is the default, but only Android TV did it then.
  - Autoplay countdown: Immediate to 60 s, default 10 s on mobile and 15 s on TV. **This contradicts the prior research's "default None"; re-check.**
  - "Passout Protection" stops after 2 h by default.
- **tvOS change.** Plex for tvOS 8.30 (about 2024-01) stopped shrinking the video at the final credit marker and shows a Skip Credits button instead [P] ([forum](https://forums.plex.tv/t/play-next-episode/860513)).
- **Complaints** [P]:
  - The button lasts about 8–10 s; "only useful if you have the remote in-hand" ([forum](https://forums.plex.tv/t/end-credits-button-disappears-way-too-quickly/836152.md)).
  - It appears a few seconds *before* the intro, which spoils cold-open punchlines ([forum](https://forums.plex.tv/t/delay-skip-intro-button/599690)).
  - On Android TV, OK skips; a remote that sends `ENTER` instead of `DPAD_CENTER` cannot ([forum](https://forums.plex.tv/t/one-button-skip-intro-for-andoidtv/770246)).
  - Credits are skipped in two steps around extra scenes ([forum](https://forums.plex.tv/t/request-option-to-skip-post-credits-segments/943728)).

#### Emby
- **Detection.** Server 4.7+ with Premiere. It works per season, needs at least 2 episodes and looks at the first 10 minutes. Each client can ignore intros, skip them automatically or show a button. The button can be on screen at 0:00 [V] ([docs](https://emby.media/support/articles/Intro-Skip.html)). I found no first-party credits detection, only plugins [P].
- **Bugs** [P]:
  - Xbox: an invisible but still active Skip button re-skipped on every later press of the remote's middle button. Fixed in 1.1.472 ([forum](https://emby.media/community/topic/114527-skip-intro-button-is-always-active)).
  - Emby Theatre on Windows: pressing Space to pause re-triggered "Play next" ([forum](https://emby.media/community/topic/132514-next-episode-button-space-bar-issue)).

#### Max (HBO)
- **Skip Intro / Skip Recap.** Jumps to "the last second" of the segment. The button "disappears in 10 seconds"; tapping or clicking the screen brings it back [V] ([help](https://help.hbomax.com/US-en/answer/detail/000002510)).
- **Next Episode.** Appears "when the credits begin" with a 15 s countdown. Autoplay is on by default. Dismiss with the X on phone or computer, Menu on Apple TV, or Back on other TVs. After dismissing, the next episode won't start until the episode, including its preview scenes, has completely finished [V]. Nothing documents what OK does.

#### Netflix
- Netflix's guiding principle: the button shows "only when needed" and works "with a single click". `S` presses it on the web [V] ([Netflix](https://about.netflix.com/en/news/looking-back-on-the-origin-of-skip-intro-five-years-later)). How intros are marked is not public.
- **Credits.** Episodes offer a "Watch Credits" option; once you pick anything else, it is gone. After a movie or a series finale, the credits shrink into a window that you click to enlarge [V] ([help](https://help.netflix.com/en/node/114058)).
- **Autoplay.** A per-profile setting [P].
- **"Are you still watching?"** Sources conflict: "2 episodes without interaction, 2 min into the next" ([HowToGeek](https://www.howtogeek.com/685919/why-netflix-asks-are-you-still-watching-and-how-to-stop-it/)) versus "3 episodes or 90 min" [P].
- **Complaints** [P]:
  - The BoJack Horseman creator and a 2020 petition objected to the shrunken credits ([/Film](https://www.slashfilm.com/571347/bojack-horseman-creator-calls-out-netflix-shrinking-end-credits/)).
  - On Apple TV, the countdown started with about 20 s left, or at 0 s ([MacRumors](https://forums.macrumors.com/threads/1756501/)).

#### Prime Video
- **2025 update.** "Skip Credits" and a prominent "Watch Credits" button on every device. By default it moves to the next video "after a brief countdown" [P] ([Cord Cutters News](https://cordcuttersnews.com/amazon-prime-video-app-update-gives-users-more-control-over-how-they-watch-tv/)).
- An older "Next Up" card sat at the bottom right from the credits offset, whatever the autoplay setting [P].
- On Android TV, OK on the highlighted Skip Intro/Recap icon was unreliable [P] ([Sony forum](https://community.sony.es/t5/android-tv/prime-video-skip-recap-skip-intro-function/td-p/2521153)).

#### Disney+
- Since 2019, a countdown fills the lower half of the screen during an episode's credits [P] ([WDWNT](https://wdwnt.com/2019/10/disney-will-feature-autoplay-function-within-disney-app/)).
- Autoplay is a per-profile toggle, on by default and off for Kids profiles [P].
- Whether Skip Credits appears on Marvel films depends on the film and its post-credit scenes [P] (Tom's Guide headline only).

#### Apple: tvOS platform behaviour (the AVKit player many tvOS apps use)
- **Skip buttons.** The developer API is `contextualActions`, "such as showing a Skip Intro button during a title sequence". The app sets and clears them on time observers. The player "presents contextual actions only when the transport bar isn't visible" [V] ([doc](https://developer.apple.com/documentation/avkit/avplayerviewcontroller/contextualactions)), so they always appear alone, like the owner's draft.
- WWDC21 says interaction is "consistent with focus behavior" [V] ([session 10191](https://developer.apple.com/videos/play/wwdc2021/10191/)). That a click activates the button is [I].
- **Up Next.** The API is `AVContentProposal`. `automaticAcceptanceInterval` counts down from the end of playback, and NaN disables auto-accept. `preferredPlayerViewFrame` lets the video animate into a smaller frame or hide [V].

#### Infuse
- Skip Intros & Credits can be set to Auto (8.4, community-sourced timestamps) [P].
- The Next Episode button appears at a percentage of the runtime, sometimes well before the credits [P] ([forum](https://community.firecore.com/t/custom-next-episode-timing/38552)).
- **On tvOS it is focused by default whenever the seek bar shows.** A left-click meant for −10 s starts the next episode instead ("Super annoying") [P] ([forum](https://community.firecore.com/t/next-episode-button-hindrance/48698)).
- Auto-skip gives no visual notice and cannot be cancelled; users have asked for both [P].

#### Hulu, Paramount+
Little primary material. Hulu's Skip Intro reached Android in 2021, after the web and some TVs [P] ([Android Police](https://www.androidpolice.com/2021/05/30/hulus-latest-app-update-adds-a-long-overdue-tool-for-efficient-binge-watching/)). Paramount+ on Roku: with autoplay on, you can't rewind through the credits [P].

#### Other evidence of accidental activation [P]
- **ITVX (UK):** Skip Intro reappears after every ad break, and an accidental press jumps back to the start of the episode ([Sky forum](https://helpforum.sky.com/t5/Sky-Stream/ITVX-Skip-Intro/m-p/5237510)).
- **Channels on Fire TV:** Select doesn't act on the "What's Next" card, and pressing Back to dismiss it sometimes exits the show (Channels community forum).

### 8.2 Accessibility
- **WCAG 2.2.1.** The expiry of "a window of opportunity for a user to react" counts as a time limit. But content on a timer "does not need to be time adjustable if there is an alternative that does not rely on a timer" (the toast example) [V] ([Understanding 2.2.1](https://www.w3.org/WAI/WCAG22/Understanding/timing-adjustable.html)).
- A Skip button that hides after 8–10 s is likely fine if the same skip stays available in the full overlay, as in Plex, Max and Jellyfin web [I].
- An autoplay countdown is a time limit leading to a change of context. An autoplay-off setting meets the "turn off before encountering it" option [I].
- Screen readers: the Jellyfin Android TV prompt is not a focusable view and its icon has no content description, so TalkBack likely doesn't announce it [I from code]. Jellyfin web uses a real `<button>` with no live region [V]. I found nothing on how Netflix, Disney+ or Apple announce these buttons.

### 8.3 Summary

**Common patterns**
- The Skip button appears alone, without the other controls (Apple's API enforces this; Jellyfin Android TV does the same).
- It hides after about 8–10 s (Jellyfin 8 s, Max 10 s, Plex about 8 s) and comes back with the full overlay.
- Back dismisses it.
- It is not offered again when you seek into the middle of the segment (Jellyfin; Kodi's EDL skips once).
- There are per-type settings of Off / Ask / Auto, with Ask the default (Plex, Jellyfin, Emby, Kodi).
- Up Next shows either at a fixed 30–40 s before the end or at a credits marker, with a 7–15 s countdown, autoplay on by default, and Back to keep watching.

**Variations**
- Whether OK activates the button: yes in Jellyfin Android TV and Plex Android TV; Kodi now makes it optional for Up Next.
- Video shrinking at the end: Netflix and Plex Android TV shrink it; Plex tvOS dropped that for a button.
- Skipping credits stops at extra scenes in Plex.
- A "Watch Credits" button exists in Prime and Netflix.

**No guard against an accidental OK was found anywhere**: no delay before OK works, no confirm step, no undo. The reported failures are:
- a focused button catching a press meant for something else (Infuse, Emby);
- an invisible button that is still active (Emby Xbox);
- the button appearing early and spoiling a punchline (Plex);
- a countdown cutting into post-credits scenes (Kodi warning, Netflix).

**Options for the OK-activates-convenience rule** (options only; the design is the owner's):
1. **Keep the rule, with Jellyfin's limits.** OK acts only while the area shows alone. It times out (about 8 s), after which OK pauses again. Back dismisses. The remote's play/pause key always pauses. A missed skip stays reachable from the full overlay.
2. **Treat the two cases differently.** OK skips an intro or recap, which seeking back undoes. For Up Next, OK opens the controls and the viewer must move into the card to accept (Kodi v1.3.1). Leaving an episode, and possibly a post-credits scene, costs more than a skipped intro.
3. **Two steps.** The first OK shows the overlay with the convenience button focused; a second OK activates it. This is safer but costs a press; Plex users complained about a three-press flow.
4. **A short grace period.** Ignore OK for about 1 s after the area appears. No competitor was found doing this [I].
5. **Undo after a skip.** Briefly offer "Back to intro". Not found elsewhere [I].
6. **Pause freezes the Up Next countdown, and Back keeps the credits playing** (Kodi, Max).
7. **Show Up Next at a credits marker rather than at fixed seconds,** stopping at extra scenes as Plex does, and never auto-advancing on a timer during the credits by default (Kodi warns that early advance skips post-credits scenes).
8. **A per-type Off / Ask / Auto setting with Ask as default.** This also gives Off as a WCAG way out of the countdown.

### 8.4 Not verified
- **Focus and OK on TV** for Netflix, Prime, Disney+, Hulu, Paramount+ and the Apple TV app: no help page or primary source found. I also don't know whether pressing OK during their Skip Intro pauses or skips.
- **Apple TV app (tvOS):** its own Skip Intro and Up Next behaviour; Apple's help pages don't cover it.
- **Timings:** Prime's and Disney+'s countdown lengths, and Netflix's Up Next timing and its 2026 TV player.
- **Plex's autoplay countdown default:** the docs mirror says 10 s mobile / 15 s TV, against the prior research's "None". Also unchecked: whether Up Next "during final credits" now reaches more platforms.
- **Hulu and Paramount+:** current skip-credits behaviour.
- **Screen-reader announcements** in any commercial app.
- **Reddit** was not fetched.

## 9. Keyboard on a computer and touch gestures (follow-up)

*Decided in D407 §10: Up and Down show the overlay on a computer too, volume on + and −, 10 s steps growing while held, Space presses a focused button, media keys' next and previous change the episode. Letter shortcuts and gestures are left out for now (D407 §11).* Reported 2026-11-02 in the design conversation; tags as above: [V] verified, [P] press or forum, [I] inferred.

**Labels:** **[V]** official documentation or source code read today. **[P]** press, forum, or a third-party guide. **[I]** inferred.

### 9.1 Keyboard on a computer

#### 9.1.1 Per player

"—" means the source lists no binding. "?" means not verified.

| Player | Play/pause | Seek | Frame `,` `.` | Volume / mute | Full screen | Subtitles / audio | Speed | 0–9, Home/End | Next/prev | Help, Esc |
|---|---|---|---|---|---|---|---|---|---|---|
| **YouTube** [V] [support](https://support.google.com/youtube/answer/7631406) | Space (acts on the seek bar when it is selected; activates a focused button), K | ←/→ 5 s (seek bar); J/L 10 s; Ctrl/⌥+←/→ chapter | yes, while paused | ↑/↓ 5 % (seek bar); M | F | C toggles; no size key | `<` `>` | 1–9 = 10–90 %, 0 = start; Home/End | Shift+N, Shift+P (playlist only) | Shift+?; Esc exits full screen. **You must click the player first** |
| **Netflix web** [V] [help](https://help.netflix.com/en/node/24855) | Space, Enter | ←/→ 10 s | — | ↑/↓; M | F | — | — | — | — | Esc exits; **S = Skip Intro** |
| **Prime Video web** [V] [Amazon help](https://digprjsurvey.amazon.com/csad/help/node/GH5GWCRVQCKEEFM8) | Space | ←/→ 10 s | — | ↑/↓; M | F (Fn+F on macOS) | **C** toggles and cycles subtitle languages; **A** cycles audio, including AD and Dialogue Boost | — | — | — | Esc: leave full screen **or stop playback** |
| **Disney+ web** [P] ([Windows Central](https://www.windowscentral.com/how-use-keyboard-shortcuts-disney-plus)) | Space | ←/→ 10 s | — | M | F | — | — | — | — | Esc. No official list found |
| **Max web** [V] ([help](https://help.hbomax.com/AU/Answer/Detail/000002553)) | Space "chooses the selected item" | — | — | — | — | — | — | — | — | Esc closes menus and overlays. Only Tab/arrows navigation is documented; extensions add the rest [P] |
| **Plex Web** [P] ([forum](https://forums.plex.tv/t/keyboard-shortcuts/852749)) | Space | ←/→ 10 s; **↑/↓ 10 min** | ? | ? | ? | ? | ? | ? | ? | ? lists the shortcuts; **they must first be enabled** in Settings › Web. Official page returned 403 |
| **Jellyfin web** [V] (source: `jellyfin-web/src/apps/legacy/controllers/playback/video/index.js`) | Space, K. On TV, Enter with the OSD hidden | ←/→ and J/L: **back 10 s, forward 30 s** (user settings); PgUp/PgDn chapter | yes | ↑/↓, wheel; M | F | G/H subtitle offset only | Shift+`,` / Shift+`.` | 0–9 = %; Home/End | Shift+N/P | Esc/Back hides the OSD. Ctrl/Alt/Meta-modified keys are ignored |
| **Emby web** [V] ([support](https://emby.media/support/articles/Keyboard-and-Remote-Support.html)) | Space, Ctrl+P | ←/→ **only while the OSD is hidden**; Shift+←/→ always | — | F8/F9/F10; M | F | Ctrl+U subtitles, Ctrl+Shift+A audio | — | — | Ctrl+F/B chapter or track; PgUp/PgDn chapter | — |
| **Vimeo** [V] ([help](https://help.vimeo.com/hc/en-us/articles/12425998125073)) | Space, K | ←/→, J/L (step not stated) | **Shift+←/→** | ↑/↓; M | F | C captions | (H opens preferences) | — | — | ? (Shift+/). Works "whenever the player is engaged" |
| **Twitch** [P] | Space/K (userscript source) | ←/→ 10 s (VODs) | — | M | F | — | — | — | — | ? overlay; Alt+T theatre mode |
| **VLC** [V] ([wiki, 2019](https://wiki.videolan.org/Hotkeys/)) | Space | Shift ±3 s, Alt ±10 s, Ctrl ±1 min, Ctrl+Alt ±5 min; plain arrows ? | E (forward only) | Ctrl+↑/↓; M | F | V cycles subtitles; B cycles audio | `+` `-` `=`, `[` `]` | — | N/P | Esc leaves full screen |
| **mpv** [V] (`etc/input.conf`) | Space, p | ←/→ 5 s; **↑/↓ 60 s**; Shift+arrows exact 1 s / 5 s; Ctrl+←/→ next/previous subtitle line | yes | **9/0**, wheel; m | f | v show/hide, j/J cycle, `#` audio, F/G size | `[` `]` ×1.1, `{` `}` ×2 | Home = start | `<` `>`, Enter | ? shows bindings; Esc leaves full screen |
| **IINA** [V] (`iina-default-input.conf`) | Space, p | ←/→ 5 s; Shift+←/→ by subtitle line; Shift+PgUp/PgDn 10 min | yes; also ⌥+←/→ | **↑/↓ 5 %**, ⌥ 1 %; m | f, Ctrl+⌘+F; Enter enters full screen | v visibility, F/G size, Z/X delay | ⌘[ ⌘] | — | ⌘+←/→ | Esc leaves full screen |
| **QuickTime** [V] ([Apple](https://support.apple.com/guide/quicktime-player/keyboard-shortcuts-and-gestures-qtpa4808515d/mac)) | Space | **←/→ = frame step**; ⌘+←/→ cycle rewind/forward speeds; ⌥+←/→ start/end | (←/→) | ↑/↓; ⌥+↑/↓ max/min | ? | — | via ⌘+← / ⌘+→ | ⌥ start/end | — | — |
| **Chrome `<video controls>`** [V] (Chromium `media_controls_impl.cc`) | Space, Enter | ←/→ go to the timeline slider (step not found); Home/End | — | ↑/↓ = 5 slider steps | (double-click) | — | — | Home/End | — | Off under spatial navigation |
| **Firefox `<video controls>`** [V] (`videocontrols.js`) | Space (not when a button has focus) | ←/→ 5 s; Ctrl+←/→ 10 % | — | ↑/↓ 10 %; Ctrl+↓ mute, Ctrl+↑ unmute | double-click | Esc closes the track list | — | Home/End | — | Only when the `controls` attribute is set |
| **Safari `<video controls>`** [V] (WebKit `media-controller.js`) | **Space, only in full screen** | — | — | — | — | — | — | — | — | — |
| *Shaka UI* [V] (`ui.js`) | Space, K | ←/→ 5 s; PgUp/PgDn 60 s | yes | M | F | C | `<` `>` | 0–9, Home/End | — | **every key remappable in config** |
| *Plyr* [V] | Space, K | ←/→ 10 s | — | ↑/↓ 10 %; M | F | C | — | 0–9 | — | L loops. Focused-only by default (`keyboard.global: false`) |
| *video.js 8* [V] | Space, K | — | — | M | F | — | — | — | — | Hotkeys are opt-in |

#### 9.1.2 Conventions

- **Near-universal:**
  - Space plays and pauses.
  - F toggles full screen.
  - Esc leaves full screen.
  - M mutes.
  - ←/→ seek, by 5 s or 10 s. QuickTime is the exception (frame step), and VLC needs a modifier.
- **Common in the YouTube family** (YouTube, Jellyfin, Vimeo, Shaka, Plyr, video.js): K, J/L, C, 0–9, `,`/`.`, `<`/`>`, Shift+N/P, Home/End, and `?` for help.
- **↑/↓ are volume almost everywhere on a computer:** Netflix, Prime, YouTube (on the bar), Jellyfin, Vimeo, IINA, QuickTime, both browsers' native controls and Plyr. The exceptions are mpv (±60 s) and Plex (±10 min) [P].
  - Orpheum's decision (↑/↓ show the overlay) therefore breaks a computer habit that nearly every web service shares, though it matches the TV convention.
  - This is the largest keyboard divergence in the decisions so far.
- **Step sizes vary:** 5 s (YouTube arrows, Firefox, mpv, IINA, Shaka), 10 s (Netflix, Prime, Disney+, Plex, YouTube J/L, Plyr), and 10 s back / 30 s forward in Jellyfin.

#### 9.1.3 Focus and conflicts

- **Space on a focused button** activates that button rather than toggling play.
  - Followed by YouTube, Firefox, Plyr and Jellyfin (on TV) [V].
  - This is the main source of "Space did the wrong thing" reports.
- **Shortcuts scoped to the player:**
  - YouTube requires clicking the player first [V]. Vimeo says "engaged" [V]. Plyr and Shaka listen only on the player by default [V].
  - Outside the player, Space scrolls the page. That draws complaints both ways: people who expect pause ([How-To Geek](https://www.howtogeek.com/243362/)) and people who want Space to scroll on YouTube ("restore spacebar" extensions) [P].
  - A Disney+ spacebar-fix extension also exists [P].
- **A focused slider takes the arrows.** After a click on YouTube's volume slider, ←/→ change volume instead of seeking. Userscripts and extensions exist to "fix" this ([greasyfork](https://greasyfork.org/scripts/38643), [Lemmy](https://lemmy.sudovanilla.org/post/5995)) [P].
  - Firefox deliberately routes arrows by the focused control (volume slider vs timeline) [V].
  - Jellyfin hides the OSD on Esc only when no dialog is open [V].
- **Screen readers:** in NVDA/JAWS browse mode, single letters are taken for navigation (H, B and so on). YouTube's Shift+N works only in focus mode ([AppleVis](https://www.applevis.com/forum/windows/issue-youtube-website-nvda)) [P].
- **Browser shortcuts:** Ctrl/⌘ combinations belong to the browser (⌘+←/→ is history on macOS) [I]. Jellyfin ignores all modified keys for this reason [V].
- **WCAG 2.1.4 Character Key Shortcuts (Level A)** [V] ([Understanding](https://www.w3.org/WAI/WCAG21/Understanding/character-key-shortcuts.html)):
  - Letter-only shortcuts must meet one of three conditions: the user can turn them off, the user can remap them to include a modifier, or they are active only while the component has focus.
  - Failure F99 is shortcuts that can be neither turned off nor remapped. Technique G217 is providing that mechanism.
  - The guidance names speech-input users, whose dictated words can fire letter keys.
  - Two field approaches fit this: Plex makes shortcuts opt-in, and Shaka lets every key be remapped.
  - Whether "the player has focus" satisfies the focus-only condition is an interpretation, not stated in the guidance [I].
  - Space, arrows, Esc, Home/End and digits are not letters, so 2.1.4 does not cover them [I].

#### 9.1.4 Media keys (Media Session)

From MDN's browser-compatibility data [V]:

| Platform | API since | Actions |
|---|---|---|
| Chrome desktop | 73 | play, pause, seekbackward/forward, previous/nexttrack; seekto 78; stop 77; togglemicrophone, togglecamera and **hangup** 93; skipad 128 |
| Chrome Android | 57 | as desktop |
| Firefox desktop | 82 | play/pause/seek/track/stop/skipad. **No togglemicrophone, no hangup** |
| Firefox Android | 82 | exposes the API but shows **no media-control interface** |
| Safari macOS / iOS | 15 | play/pause/seek/track/stop/skipad; togglemicrophone and togglecamera **18.4**; **no hangup** |
| Android WebView | none | — |

- **Chrome 73** [V] ([blog](https://developer.chrome.com/blog/media-updates-in-chrome-73)):
  - The hardware play/pause key works by default, even with Chrome in the background.
  - Next and previous reach the page only if it registers `nexttrack`/`previoustrack`.
  - At launch: Windows, macOS and ChromeOS; "Linux later" (whether that shipped is unverified).
- **Firefox:** hardware keys go through MPRIS on Linux, controlled by `media.hardwaremediakeys.enabled` [P].
- **iOS lock screen:** reportedly shows a "10" skip icon whatever the page's actual step [P] ([dbushell](https://dbushell.com/2023/03/20/ios-pwa-media-session-api/)).
- **Headset mapping (double press = next, triple = previous):** unverified [I].

### 9.2 Touch gestures on phones and tablets

#### 9.2.1 Per player

| Player | Single tap | Double tap left/right | Long press | Horizontal drag | Vertical, by side | Swipe down / up | Pinch | Two fingers |
|---|---|---|---|---|---|---|---|---|
| **YouTube app** | shows/hides the UI [I] | ±10 s [V] ([help](https://support.google.com/youtube/answer/7509567)); length configurable 5–60 s, cannot be turned off [P] | **2× while held, anywhere** [V, Android and iOS] | scrubber; slide up for precise seeking with thumbnails; "release to cancel" [V] ([help](https://support.google.com/youtube/answer/12825599)) | — | full-screen enter/exit; help pages contradict each other; swipe down minimises [V] ([help](https://support.google.com/youtube/answer/72689)) | zoom/fill in full screen, screens wider than 16:9 [V] ([help](https://support.google.com/youtube/answer/12820428)) | — |
| **YouTube mobile web** | ? | works [P] ([How-To Geek](https://www.howtogeek.com/749519/)) | — [P] | ? | — | — | — | — |
| **Netflix app** | UI | ±10 s (2018) [P] ([GSMArena](https://www.gsmarena.com/netflix_update_brings_redesigned_controls_on_android-news-31384.php)) | — | ? | — | ? | ? | screen-lock button [P] |
| **Prime Video app** | ? | ? (nothing found) | ? | ? | ? | ? | ? | ? |
| **Disney+ app** | UI | ±10 s [P] ([How-To Geek](https://www.howtogeek.com/667493/)) | ? | ? | ? | ? | ? | ? |
| **Plex app** | UI | seek [P] (changelog). On-screen seek buttons were removed, users complained, and they are being restored in 2026.9 [P] ([forum](https://forums.plex.tv/t/time-skip-missing-on-2026-9-0-client/939064)) | ? | ? | requested, absent [P] | requested, absent [P] | requested [P] | — |
| **Jellyfin web** [V] | touch **toggles the OSD**; mouse click plays/pauses **after a 300 ms wait** | none | — | slider | — | — | — | — |
| **Jellyfin Android app** | — | ? | — | — | absent; requested 2025-11 [P] ([forum](https://forum.jellyfin.org/t-swipe-controls-for-mobile-app-player)) | — | — | — |
| **Infuse** [V] ([Firecore](https://support.firecore.com/hc/en-us/articles/215090987)) | UI | ±10 s, **repeated taps add up** | — | drag right/left; longer drag goes farther | left = brightness, right = volume | — | — | **2-finger tap plays/pauses** |
| **VLC Android/iOS** | UI | ±10 s on the edges; length configurable since 2024; can be turned off [P] ([commit](https://code.videolan.org/videolan/vlc-android/commit/5eea8d8515a0f714dcb7e7c78a36ab7715873b34), [ML](https://mailman.videolan.org/pipermail/android/2024-July/022583.html)) | — | seek | volume (right) / brightness (left) | — | fit/fill; the only gesture you cannot turn off (2022) | 2-finger tap pauses |
| **MX Player** [P] | UI | seek; centre double tap = pause (optional) | — | scrub | brightness/volume | — | zoom and pan | lock button |
| **iOS native player** [P] ([9to5Mac, iOS 16](https://9to5mac.com/2022/06/23/ios-16-video-player/)) | **centre tap plays/pauses** | **does nothing** since iOS 16 (removed so single taps need no wait) | — | drag-anywhere scrub added in iOS 16, **removed in iOS 17** [P] ([PiunikaWeb](https://piunikaweb.com/2023/09/21/ios-17-rewind-fast-forward-video-gestures-not-working/)) | — (system volume) | swipe down closes [I] | **pinch = fill/fit** | — |
| **Chrome Android `<video controls>`** [V] (Chromium) | toggles the controls | **±10 s, 300 ms window** | — | — | — | — | — | — |
| *video.js 8 + mobile-ui plugin* [V] ([README](https://github.com/mister-ben/videojs-mobile-ui)) | toggles UI | ±10 s, tapTimeout 300 ms | — | — | — | optional swipe up/down for full screen ("except iPhone"); full screen on rotate | — | — |
| *video.js 10 `<media-gesture>`* [V] ([docs](https://videojs.org/docs/framework/html/reference/gesture)) | tap must finish within 250 ms | configurable regions (halves or thirds); seekStep 10 s | — | — | — | — | — | — |
| *Shaka UI* [V] | `singleClickForPlayAndPause` off on mobile | `seekOnTaps` on for mobile, 10 s | — | — | — | — | — | — |
| *Plyr* [V] | touch only shows controls | none | — | — | — | — | — | — |

The iOS native player row is the one that applies on an iPhone in full screen, since our code hands off to it there. Its 10 s skip buttons and the playback-speed menu are standard since iOS 16 [P].

#### 9.2.2 What a mobile browser cannot do, or does unreliably

- **Volume (iOS):** cannot be set. "The volume property is not settable in JavaScript. Reading the volume property always returns 1" [V] ([Apple](https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/Using_HTML5_Audio_Video/Device-SpecificConsiderations/Device-SpecificConsiderations.html)). Android can set the element's volume, which is not the system volume [I].
- **Brightness:** no shipped web API [I]. The left-side vertical swipe is app-only.
- **iPhone full screen:** goes through the native player (`webkitEnterFullscreen`), so the page's gestures do not apply there [V Apple, I].
- **Edge back-swipe (iOS):** Safari ignores `overscroll-behavior-x: contain` for back navigation ([WebKit bug 240183](https://bugs.webkit.org/show_bug.cgi?id=240183)). The workaround is `preventDefault` on a `touchstart` near the edge, iOS 13.4+ [P] ([pqina](https://pqina.nl/blog/blocking-navigation-gestures-on-ios-13-4/)).
- **Pull-to-refresh:** `overscroll-behavior-y` is unreliable on iOS [P]. Android's system back gesture cannot be excluded from a page [I].
- **Long press:** browsers open a context menu. Brave on Android shows download/share/print instead of YouTube's 2× ([Brave community](https://community.brave.app/t/new-context-menu-is-interfering-with-youtube-s-2x-play-speed-when-pressing-the-video/638826)) [P]. It needs `contextmenu` suppressed and `-webkit-touch-callout: none` [I].
- **Double tap:** the page must suppress the browser's double-tap zoom (`touch-action: manipulation`) [I].
  - Every implementation waits about 300 ms (Chromium, Jellyfin, video.js) before acting on a single tap.
  - Apple dropped double tap in its own player to remove that wait [P].
- **Vertical swipes near the top or bottom edge** collide with the notification shade and Control Center [I].

#### 9.2.3 Complaints

- **Accidental double-tap seeks:** VLC added switches to turn off double-tap seek (2017) and double-tap pause (2021) [P]. YouTube's cannot be turned off, only resized [P]. Infuse users want custom lengths [P].
- **Drag-anywhere scrubbing:** some iOS 17 users welcomed its removal because of accidental rewinds; others asked for a toggle [P].
- **Gestures replacing buttons:** Plex's removal of the seek buttons was walked back [P]. VoiceOver users on Disney+ could not skip except with the buttons ([AppleVis](https://www.applevis.com/forum/ios-ipados/disney-plus-direct-touch-area)) [P].

#### 9.2.4 Accessibility guidance

- **WCAG 2.5.1 Pointer Gestures (A)** [V] ([Understanding](https://www.w3.org/WAI/WCAG22/Understanding/pointer-gestures.html)):
  - Multipoint and path-based gestures (pinch, two-finger tap, swipes along a path) need a single-pointer alternative.
  - Tap, double tap and long press count as single-pointer, so they are allowed.
  - Gestures defined by the OS or browser are out of scope.
- **WCAG 2.5.7 Dragging Movements (AA)** [V] ([Understanding](https://www.w3.org/WAI/WCAG22/Understanding/dragging-movements.html)):
  - Dragging needs an alternative that works without dragging. For a slider, tapping a point on the track is acceptable.
  - A keyboard equivalent alone does not satisfy it.
  - So scrubbing by drag needs tap-to-position on the timeline, or the ±10 buttons.
- **Upshot:** every gesture needs a visible button. The video.js 10 docs say gestures "should supplement visible, keyboard-operable controls" [V].

### 9.3 Near-universal sets

**Keyboard:**

| Key | Action | Notes |
|---|---|---|
| Space | play/pause | not when a button has focus |
| ←/→ | seek | 5 s or 10 s |
| F | full screen | |
| Esc | leave full screen, close menus | |
| M | mute | |
| ↑/↓ | volume | on computers |

The YouTube-style extras appear in roughly half the players: K, J/L, C, 0–9, `,`/`.`, `<`/`>`, Shift+N/P, Home/End, `?`.

**Touch:**

- Tap shows the overlay. It toggles in most players. iOS's native player plays/pauses on a centre tap.
- Double tap left/right ±10 s (YouTube, Netflix, Disney+, Plex, Infuse, VLC, MX, Chrome native, Shaka, video.js plugin). Repeated taps accumulate in Infuse [V] and probably YouTube [I].
- Drag the scrubber.

App-only: long-press 2× (YouTube, with a browser context-menu conflict), brightness/volume swipes (impossible or unreliable on the web), pinch to fill, and two-finger tap to pause.

### 9.4 Questions for the owner

1. **↑/↓ on a computer:** keep "show the overlay" everywhere (the TV habit), or keep volume on a computer keyboard (Netflix, Prime, YouTube, Jellyfin, both browsers)? Or volume only while the overlay is hidden?
2. **Seek step for ←/→ and J/L:** 10 s to match the owner's single step and Netflix/Prime, or YouTube's split (5 s arrows, 10 s J/L)? Does the hold-to-grow scheme (10/30/60 s) apply to keys on a computer?
3. **Which letter keys to adopt** (K, J/L, F, M, C, 0–9, `,`/`.`, `<`/`>`, Shift+N/P, `?`, and Netflix's S for Skip Intro), and how to meet WCAG 2.1.4: a setting to turn them off (Plex), remapping (Shaka), or active only while focus is in the player (YouTube)?
4. **Space when a button in the overlay has focus:** activate the button (field convention), or always play/pause?
5. **Scope of the shortcuts:** whole page while the player page is open, or only after the player is clicked or focused?
6. **A help overlay on `?`:** yes or no?
7. **Media keys:** map `nexttrack`/`previoustrack` to next and previous episode? Note `hangup` is Chrome-only, and Firefox has no `togglemicrophone`.
8. **Single tap on touch:** toggle the overlay (field), or tap the centre to play/pause (iOS native)?
9. **Double tap ±10 s:**
   - Adopt it? Accept the ~300 ms single-tap delay, or show the overlay at once and convert to a seek on the second tap?
   - Should repeated taps accumulate?
   - Offer a setting to turn it off (VLC)?
10. **Long-press 2×:** adopt it, given the context-menu conflicts in some browsers?
11. **Horizontal drag anywhere to scrub:** adopt it, given iOS 17 dropped it over accidental rewinds?
12. **Swipe down to exit full screen, and pinch to fill:** adopt them, given iOS edge and pull-to-refresh conflicts and that the iPhone is native anyway?
13. **Tablets in full screen** (where our page keeps control): the same gesture set as phones?

### 9.5 Not verified

- **Prime Video and Disney+ mobile gestures:** nothing primary found.
- **Netflix:** pinch or fill on mobile not found.
- **Official shortcut lists:** none from Disney+, Max or Twitch. Plex's support page returned 403.
- **VLC desktop:** what plain ←/→ do.
- **QuickTime:** the full-screen key and J/K/L.
- **Chrome's native timeline arrow step:** the slider uses `step="any"`; the size per press was not found.
- **Safari desktop's native skip interval:** not found.
- **iOS native player:** swipe-down-to-close, and whether anything changed after iOS 17.
- **YouTube:** whether double taps accumulate, and whether double tap is on by default in m.youtube.com in Chrome today.
- **Media keys and headsets:** how desktop media keys and headset presses map per browser and OS. Chrome on Linux.
- **WCAG 2.1.4:** whether "focus anywhere in the player" counts as "active only when that component has focus".
- **Reddit** was not fetched; sentiment is second-hand.

## 10. Where + and − sit on the world's keyboard layouts (follow-up)

*Decided in D407 §10: option A, the character rule; the four layouts of group 3 get no unshifted press.* Reported 2026-11-02 in the design conversation; tags as above: [V] verified, [P] press or forum, [I] inferred.

I found where "+" and "−" sit on every layout you listed. **"−" is easy: it is an unshifted key on every one of them.** "+" splits into three groups, and four layouts need special handling for the plain press:

- Group 1: "+" is on its own key, unshifted.
- Group 2: "+" is Shift plus a key that gives "=".
- Group 3: "+" is Shift plus a key that gives something else (Swiss "1", Hungarian "3", Turkish Q "4", Japanese JIS ";").

The positions come from compiling each layout with libxkbcommon (the xkeyboard-config data, cloned to `/yoloai/cache/repos/xkeyboard-config`). The script is at `/tmp/claude-1000/-home-karl-orpheum/7ad7ac38-978e-4374-bdfa-c69b98cc6f55/scratchpad/xkb.py`. I did not edit or commit anything in the repo.

### 10.1 Where "+" and "−" are

"+ key" is the physical key (`code`), the modifier it needs, and what that key gives with no modifier. Codes are `KeyboardEvent.code` names, which always mean the US position.

| Layout | + key | − key (always unmodified) |
|---|---|---|
| US, UK, US-Intl, Colemak, Chinese pinyin (US), Korean | `Equal`, Shift, "=" | `Minus` |
| Canadian French / Canadian Multilingual | `Equal`, Shift, "=" | `Minus` |
| Polish programmer, Brazil ABNT2 | `Equal`, Shift, "=" | `Minus` |
| Russian, Ukrainian, Greek, Hebrew, Arabic | `Equal`, Shift, "=" | `Minus` |
| French AZERTY (Windows) | `Equal`, Shift, "=" | `Digit6` |
| Belgian AZERTY | `Slash`, Shift, "=" | `Equal` |
| French Mac | `Slash`, Shift, "=" | `Equal` |
| Dvorak (US) | `BracketRight`, Shift, "=" | `Quote` |
| Programmer Dvorak | `Digit9`, none, "+" | `Quote` |
| German, Austrian (Windows and Mac) | `BracketRight`, none, "+" | `Slash` |
| Italian, Spanish (Spain), Latin American | `BracketRight`, none, "+" | `Slash` |
| Portuguese (Portugal) | `BracketLeft`, none, "+" | `Slash` |
| Portuguese Mac | `Equal`, none, "+" | `Slash` |
| Swedish, Finnish, Norwegian, Danish (also the Mac versions) | `Minus`, none, "+" | `Slash` |
| Dutch (nl) | `Semicolon`, none, "+" | `Slash` |
| Dutch Mac (as UK) | `Equal`, Shift, "=" | `Minus` |
| Polish 214 (QWERTZ) | `Minus`, none, "+" | `Slash` |
| Czech, Slovak | `Digit1`, none, "+" | `Slash` |
| Turkish F | `Backquote`, none, "+" | `Equal` |
| **Swiss German/French (Windows and Mac)** | `Digit1`, Shift, **"1"** | `Slash` |
| **Hungarian** | `Digit3`, Shift, **"3"** | `Slash` |
| **Turkish Q** | `Digit4`, Shift, **"4"** | `Equal` |
| **Japanese JIS** | `Semicolon`, Shift, **";"** | `Minus` (Shift gives "=") |
| Italian Mac (as xkb models it) | `Equal`, Shift, "=" | `Minus` |
| Every layout | `NumpadAdd`, "+" | `NumpadSubtract`, "−" |

No common layout needs AltGr for its primary "+" or "−", and none puts them on a dead key.

### 10.2 What a page can use

- **`event.key`** is the character the layout produces with the modifiers held: "+" for Shift+= on US, "+" for plain + on German, "=" for plain = on US. It is the same in all four browsers. Dead keys give `"Dead"`.
  - On Windows, AltGr is reported as `ctrlKey && altKey`, with a fake ControlLeft keydown first.
  - On Linux and macOS it shows as `getModifierState("AltGraph")`, and Option sets `altKey`.
  - During IME composition, `key` is `"Process"` and `keyCode` is 229.
- **`event.code`** is the physical position, named after US. It is stable across layouts but says nothing about what the key prints.
- **`event.keyCode`** should not be used here; it differs by browser and OS.
  - Chrome on Windows passes the Windows virtual key (187 = VK_OEM_PLUS, which each layout's author assigns).
  - Chrome and Safari on Mac/Linux use the US keyCode of the character.
  - Firefox has its own rules: 61 for "=", 171 for "+", 173 for "−".
  - Swiss German Windows has no VK_OEM_PLUS key at all (kbdlayout.info).
- **`navigator.keyboard.getLayoutMap()`** is in Chromium only (Chrome, Edge, Opera). Mozilla rated it harmful and Apple objected on fingerprinting grounds, so neither will ship it.
  - It needs a secure context. In iframes it needs the `keyboard-map` permission, which defaults to same-origin, so a cross-origin frame needs `allow="keyboard-map"`.
  - It reports only the **unshifted** character, and dead keys become their spacing form.
  - If the active layout cannot type Latin letters (Russian, for example), it reports the first installed layout that can.
  - So it can never tell you directly that `Digit1`+Shift gives "+". You can only infer it from a known-layout fingerprint, which is what Figma does.
- **How others handle this:**
  - YouTube has no "+" shortcut; volume is on the arrow keys.
  - Figma maps the getLayoutMap characters to known layouts by heuristic, has a manual layout picker covering six layouts, and its staff tell users of other layouts to switch to US. Its blog lists dead-key pitfalls (Safari fires `compositionstart` before `keydown`, Chrome the other way round) and Shift+numpad overriding NumLock.
  - VS Code web, from memory and not checked: it uses getLayoutMap where present, otherwise guesses the layout by comparing `key` against `code` on presses.

### 10.3 Browser zoom (never take it)

- **Chrome/Edge on Windows and Linux:** Ctrl (or Ctrl+Shift) with the layout's VK_OEM_PLUS or VK_OEM_MINUS key, plus Ctrl with NumpadAdd or NumpadSubtract (`accelerator_table.cc`, cached under `/yoloai/cache/http/chromium/`).
- **Chrome on Mac:** Cmd+"−", and Cmd(+Shift) with the OEM_PLUS key, which also covers Cmd+= (`accelerators_cocoa.mm`).
- **Firefox:** I could not fetch its keyset. From memory, it matches Ctrl/Cmd with "+" or "=", whichever key produces them, plus the numpad.
- **Safari:** Cmd+"+" or "=", and Cmd+"−" (not checked).

So the rule that keeps zoom safe is simple: **do nothing and don't call preventDefault when `ctrlKey || metaKey` is set.** That also drops Windows AltGr, which matches your no-modifier rule.

### 10.4 Implementation options (the owner chose A, D407)

All of them share some guards:
- Skip events aimed at editable fields.
- Skip IME events (`isComposing` or keyCode 229).
- Skip anything with `ctrlKey`, `metaKey`, `altKey` or `getModifierState("AltGraph")`.
- Down is `key === "-"` with no Shift, or `NumpadSubtract`. That is complete for every layout above.

**A. Character rule only (all browsers, no API).**
- How: up is `key === "+"` (Shift allowed), or `key === "="` with no Shift, or `NumpadAdd`.
- Covers: every layout except the four in group 3 (Swiss, Hungarian, Turkish Q, JIS). Those users still get Shift+key and the numpad, but the plain press does nothing.
- Side effect: on Czech and Slovak the plain "=" key (`Minus` code) also raises volume, which is harmless.

**B. A plus learning from the first shifted press.**
- How: when a "+" arrives from a non-numpad key with Shift held, remember its `code`. After that, a plain press of that `code` also raises volume.
- Covers: all layouts, after the user has pressed Shift+"+" once.
- Failure modes:
  - Nothing happens before that first press.
  - After a layout switch the learned key is stale: someone who learned on Swiss and switched to US would get volume up from "1".
  - Fix: forget it when that key with Shift produces something other than "+", or when the layout map changes.
- Collision: on Swiss/Hungarian/Turkish Q, plain "1"/"3"/"4" becomes volume up, which clashes with any digit shortcut such as seek-to-percent. I found no digit shortcut in `web/apps/web/src/input-buttons.ts` today.

**C. A plus getLayoutMap fingerprints (Chromium only).**
- How: detect Swiss, Hungarian, Turkish Q and JIS from the map (for example `KeyZ`="y" with `Semicolon`="ö" for Swiss), then map their digit or ";" key.
- Covers the four from the first press, in Chromium only.
- Costs: Firefox and Safari still need B or go without; it adds a fingerprint table to maintain; and it is fragile with custom layouts.

**D. A plus a manual layout setting** (as Figma does). This is deterministic but a chore for users.

### 10.5 Not verified

- Apple's own layouts for Spanish, Belgian and Italian Mac: xkb's `es(mac)` and `be(mac)` failed to compile, and `it(mac)` may model "Italian" rather than "Italian – Pro" (which matches Windows).
- Firefox's and Safari's exact zoom key lists.
- Whether Japanese or Korean IMEs swallow "+" and "−" when focus is not on a text field.
- VS Code web's actual approach.
- Windows layouts other than Swiss German were taken from xkb rather than kbdlayout.info; xkb and Windows agree on these keys as far as I know.

Sources: [MDN getLayoutMap](https://developer.mozilla.org/en-US/docs/Web/API/Keyboard/getLayoutMap), [Keyboard Map spec](https://wicg.github.io/keyboard-map/), [MDN keyCode](https://developer.mozilla.org/en-US/docs/Web/API/KeyboardEvent/keyCode), [The Register on Chrome 97](https://www.theregister.com/2022/01/06/chrome_97_privacy), [ghacks](https://www.ghacks.net/?p=176406), [kbdlayout.info kbdsg](https://kbdlayout.info/kbdsg/virtualkeys), [Joplin Swiss zoom report](https://discourse.joplinapp.org/t/zoom-in-broken-on-some-windows-keyboard-layouts/42065), [Figma blog](https://www.figma.com/blog/behind-the-scenes-international-keyboard-shortcuts/), [Figma layout help](https://figma-signup.helpjuice.com/change-your-preferences/select-keyboard-layout), [Chromium IME 229](https://codereview.chromium.org/2127843002).
