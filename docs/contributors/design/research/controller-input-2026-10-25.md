> **ABOUTME:** How remotes, game controllers and keyboards reach a web page on each TV and
> browser, what competitors do with them, and whether Orpheum's web client can be driven well by
> them or needs a native app. Read before building a TV mode, a focus engine or a key map.

# Controller Input in the Web Client

- **Date:** 2026-10-25.
- **Requested by** the owner, 2026-10-25: "how best to handle controller input in a web app. Also
  look at what our competitors do. Can we make the web app work well with controllers (and maybe
  also keyboard control)? Or is that going to require a native app?"
- **Builds on:** [back-navigation-2026-10-24.md](back-navigation-2026-10-24.md) (TV Bro's Back),
  [tv-apps-react-native-2026-10-24.md](tv-apps-react-native-2026-10-24.md) (the native-app
  recommendation), [ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md) §3.7 and R7
  (one focus-navigation module), [milestone-reassessment-2026-10-18.md](milestone-reassessment-2026-10-18.md)
  (B1, a TV mode of the web client), the plan [tv-client.md](../plans/tv-client.md) (the remote's
  contract, focus never costs legibility), and [D138](../../decisions/working-notes.md),
  [D188](../../decisions/working-notes.md), [D189](../../decisions/working-notes.md) and
  [D190](../../decisions/working-notes.md) (remote key to focus moved ≤ 250 ms).
- **Cache:** pages under `/yoloai/cache/http/controller-input/` (`<name>.html`, `.txt` or
  `.json`, with `<name>.url` beside each), fetched 2026-10-25. Chromium and AOSP files are from
  `main` on that date, cached as `cr-*.txt`. Clones in `/yoloai/cache/git/`:

  | Repository | Commit | Date |
  | --- | --- | --- |
  | TV Bro (`tv-bro`) | `6cb4b7c` | 2026-07-29 |
  | jellyfin-web | `b290955` | 2026-10-24 |
  | jellyfin-webos | `ab47940` | 2025-11-03 |
  | jellyfin-tizen | `31408c1` | 2025-10-24 |
  | stremio-web | an earlier commit | 2026-10-24 |
  | Stremio's spatial-navigation fork (`stremio-spatial-navigation`) | an earlier commit | 2022-06-01 |
  | WICG spatial-navigation (`spatial-navigation`) | an earlier commit | 2026-03-23 |
  | Norigin-Spatial-Navigation | an earlier commit | 2026-10-23 |
  | BBC lrud-spatial | an earlier commit | 2026-07-09 |
  | Enact | an earlier commit | 2026-09-02 |

  Also cloned for reading, not cited by line: js-spatial-navigation, lrud,
  react-tv-space-navigation.
- **Orpheum read at** An earlier commit (2026-10-25).
- **Labels:**
  - **[1P]** the vendor's own documentation, source code or staff post.
  - **[U]** a user report: forum thread, issue.
  - **[J]** a journalist's, reviewer's or vendor-marketing article.
  - **[S]** second-hand: a search engine's summary of a page that could not be read in full.
  - **[I]** my inference. **Unverified** marks a claim no source here confirms.
  - **Judgement** marks a recommendation that is an opinion.
- **Method:** three research passes in parallel (platform input; spatial navigation and focus
  practice; competitors), then spot checks of the load-bearing claims in the cached Chromium
  sources and the TV Bro clone. Nothing was run on a device.

---

## 1. Summary

**A web app can be driven well by a remote, a controller and a keyboard.** YouTube's TV app is a
web app (Leanback on Cobalt), Jellyfin's Samsung and LG apps are its web client, and Emby
Theater and Plex's LG app are web stacks. All of them are D-pad first (§4). None of it comes
from the browser: each product carries a key map, a focus engine and a ten-foot layout of its
own. The platform's only help is narrow: `focusgroup` for arrows inside one widget, shipped in
Chromium 150 only, and Android WebView's built-in spatial navigation (§3.1).

**What reaches the page decides how good it can be, and TV Bro gives the page the least.**

- **Cursor mode is TV Bro's default.** The page gets no arrow or Enter keys at all. The D-pad
  moves a pointer, and OK is a synthetic touch.
- **Direct navigation mode** is chosen from the cursor menu. In it, the keys reach the page, and
  WebView moves focus between links by itself.
- **The remote's Back never reaches the page in either mode.** In direct mode it switches the
  cursor back on.
- **TV Bro takes the play/pause, stop, rewind and fast-forward keys.** It drives the page's first
  `<video>` itself.

So in TV Bro a D-pad TV mode works only after the viewer enters direct mode, and every press of
the remote's Back throws them out of it (§2.1).

**Answer to "web or native?" (judgement):**

- **The web client can and should get a controller and keyboard mode.** It is needed anyway:
  - keyboard users need it (WCAG 2.1.1; [D188](../../decisions/working-notes.md))
  - Samsung and LG apps are packaged web apps, which receive every key the app registers (§2.2,
    §2.3)
  - [tv-apps-react-native-2026-10-24.md](tv-apps-react-native-2026-10-24.md) §10.1 already keeps
    the web client for Samsung, LG and the browsers
- **A native app is needed only for what a browser keeps for itself:**
  - the remote's Back and the media keys in TV Bro and Silk
  - a launcher entry, voice search and the Home key
- **On Android TV the cheapest native piece is a thin WebView shell, not a new client** (§5.3).
  It loads the same web client, gets every key and Back, and costs about a week or two. It does
  not change the WebView's playback, so it does not touch the TCL's sound lag.

**Recommended plan (judgement, §6):**

- **Step 0:** try today's client in TV Bro's direct mode. It costs an evening and no code.
- **Step 1:** one input layer and one focus engine, used on every page and by desktop keyboards.
- **Step 2:** a TV layout and the remote's player model for Home, the lists, title, series and
  Play.
- **Step 3:** game controllers.
- **Then decide:** the Android shell, and Tizen/webOS packaging.

The web TV mode is roughly 6–9 focused weeks (rough, §6.4). It is the same work a Tizen or webOS
app needs, and it does not compete with the react-native-tvos app: that app rewrites the screens
for Android TV; this makes the existing screens usable everywhere else.

### 1.1 What reaches the page, by platform

| Platform | Arrows, OK | Back | Media keys | Gamepad API | Source |
| --- | --- | --- | --- | --- | --- |
| **TV Bro, cursor mode (default)** | **No**: a pointer moves; OK is a touch | Never: menu bar, fullscreen, panels | Play/pause, stop, rewind, ffwd taken; TV Bro drives the first `<video>` | No: controller motion is withheld | §2.1 [1P] |
| **TV Bro, direct mode** | Yes, as `ArrowUp`… and `Enter` 13; WebView's own spatial navigation moves focus if the page does not | Never: it leaves direct mode | Same as cursor mode; `MediaPlay`/`MediaPause` unbound, so they likely reach the page (unverified) | Yes; using it stops a controller's D-pad producing keys | §2.1, §2.4 [1P] |
| **Android WebView in an app of our own** | Yes | The app gets it and decides | `MediaPlayPause` 179, `MediaRewind` 227, `MediaFastForward` 228 reach the page; no Media Session | Yes | §2.1 [1P] |
| **Fire TV, Silk** | Captured for its cursor (Amazon staff, 2019) | Browser's | Only play/pause reached a page in one 2026 measurement | not checked | §2.1 [1P/U] |
| **Fire TV, Amazon web-app runtime** | 37–40, Select 13 | `keyCode` 4 | 179, 227, 228 | "supports the W3C standard Gamepad APIs" | §2.1 [1P] |
| **Samsung Tizen app** | 37–40, 13, delivered without registering | 10009, delivered without registering; long press exits | 415, 19, 10252, 412, 417, 413 after `registerKey` | not checked | §2.2 [1P] |
| **Samsung's own browser** | not checked | not checked | "not possible" | not checked | §2.2 [1P] |
| **LG webOS app** | 37–40, 13; Magic Remote switches between pointer and 5-way | `popstate` by default; 461 with `disableBackHistoryAPI` | 415, 19, 413, 412, 417 on the conventional remote | not checked | §2.3 [1P] |
| **Desktop Chrome, Firefox, Safari** | Keyboard; the browser moves nothing on arrows | Alt+Left, a mouse's back button: `popstate` | To Media Session | Chrome, Firefox, Safari | §2.4–§2.6 [1P] |

---

## 2. What input reaches a web page

### 2.1 Android TV: WebView, TV Bro, Chrome, Fire TV

**Chromium's mapping from Android keys to DOM keys**
(`ui/events/keycodes/keyboard_code_conversion_android.cc`, cached as `cr-kbconv-android.txt`)
[1P]:

| Android key | DOM `key` | `keyCode` |
| --- | --- | --- |
| `DPAD_UP` / `DOWN` / `LEFT` / `RIGHT` | `ArrowUp` / `ArrowDown` / `ArrowLeft` / `ArrowRight` | 38 / 40 / 37 / 39 |
| `DPAD_CENTER` | `Enter` | 13 |
| `BACK` | `GoBack` | browser-back |
| `MEDIA_PLAY_PAUSE`, `MEDIA_PLAY`, `MEDIA_PAUSE` | `MediaPlayPause`, `MediaPlay`, `MediaPause` | 179 |
| `MEDIA_REWIND` / `MEDIA_FAST_FORWARD` | `MediaRewind` / `MediaFastForward` | 227 / 228 |
| `MEDIA_STOP` / `NEXT` / `PREVIOUS` | `MediaStop` / `MediaTrackNext` / `MediaTrackPrevious` | 178 / 176 / 177 |
| `PROG_RED` and the other colours | `ColorF0Red` … | no case, probably 0 (unverified) |
| `BUTTON_A`, `BUTTON_B` … | none: listed as unsupported | 0 |

**Back never reaches the page as a key in a WebView or in Chrome.**
`ContentUiEventHandler.shouldPropagateKeyEvent` returns false for `KEYCODE_HOME`,
`KEYCODE_BACK`, the call, power, camera and volume keys and SysRq, so they go to the embedding
app (`cr-ContentUiEventHandler.txt:181-200`, checked) [1P]. The `GoBack` mapping above is never
used for a hardware Back. Whether a page gets anything for Back is the app's choice.

**WebView on a device without a touchscreen moves focus with the D-pad by itself.**
`AwSettings.java:412-417`: "Best-guess a sensible initial value based on the features supported
on the device", then `mSpatialNavigationEnabled = !…hasSystemFeature(PackageManager.FEATURE_TOUCHSCREEN)`
(checked) [1P]. Blink runs it in the default key handler, after the page's own handlers:
"SpatNav does not need to handle this arrow key because the webpage had a key-handler that
already moved focus" (`spatial_navigation_controller.cc:184-189`) [1P]. A page that calls
`preventDefault()` on an arrow therefore takes over [I]. Whether the owner's TCL declares a
touchscreen feature is unverified; Android TVs normally do not [I].

**WebView has no Media Session.** `aw_main_delegate.cc:159-161`: "WebView does not support
MediaSession API since there's no UI for media metadata and controls", then
`AppendSwitch(switches::kDisableMediaSessionAPI)` (checked) [1P]. Media keys that the app lets
through arrive as key events instead. Orpheum's `play-state.ts` already guards
`"mediaSession" in navigator`, so this costs nothing today [I].

**TV Bro** (`6cb4b7c`) [1P]:

- **Cursor mode is the default.** `isVirtualCursorMode()` returns `viewParent?.cursorEnabled ?: true`
  (`WebViewWebEngine.kt:224-226`). In it, `CursorLayout.dispatchKeyEvent` hands the D-pad, centre,
  Enter and `BUTTON_A` to its own adapter and consumes them
  (`app/common/.../cursor/CursorLayout.kt:64-74`; the key list in
  `NavigationReservedShortcutKeyCodes.kt:10-23`). OK becomes a synthetic finger touch at the
  cursor (`CursorDrawerDelegate.kt:183-235`). Joystick and gamepad motion is withheld from the
  WebView (`WebViewEx.kt:503-506`). **So in cursor mode the page sees a touch pointer and no
  keys.** Today's client already works this way: every control is a link or a button, which
  a pointer reaches [I].
- **Direct navigation mode** is entered from the cursor menu (`CursorMenuView.kt:204-207`). It is
  not saved: it is a property of the tab's view, set back to the cursor by Back
  (`MainActivity.kt:891`) [I, from the code; not observed]. Its help text
  (`res/values/strings.xml:129`): "Direct navigation hides the on-screen cursor and sends keys
  from your remote or game controller straight to the web page, so sites that support TV or
  keyboard focus can highlight links and controls. To exit this mode: press Back on your TV
  remote, or press and hold the B button on a game controller. This mode works best with the
  WebView engine: WebView supports moving focus through the page with the D-pad."
- **Back.** `handleBackNavigation` (`MainActivity.kt:888-905`), in order: turns the cursor back on
  if it was off, closes the cursor menu, leaves grab mode, exits fullscreen, hides the bottom
  panel, else toggles the menu bar. It never calls `history.back()`; the history's back is the
  menu's button (`:178`, `:389-399`). The page gets neither a key nor a `popstate`
  ([back-navigation-2026-10-24.md](back-navigation-2026-10-24.md) §2.3 found the same).
- **Media keys.** `Shortcut.kt:18-21` binds `MEDIA_PLAY_PAUSE`, `MEDIA_STOP`, `MEDIA_REWIND` and
  `MEDIA_FAST_FORWARD`. `ShortcutMgr.handle` consumes them (`ShortcutMgr.kt:109-153`) and runs
  injected script on `document.querySelector('video')`: toggle, stop, or `currentTime` ±10 s
  (`app/src/main/assets/generic_injects.js:36-67`). The page sees only the video's own events.
  Our player has one `<video>` per page, so these would act on it [I]; a seek this way bypasses
  hls.js's own seek path only as any `currentTime` change does [I].

**Chrome on Google TV:** no official TV build was found; secondary sources say it is sideloaded
only. Unverified for 2026.

**Fire TV.**

- **Amazon's web-app runtime** ("Supporting Controllers in Web Apps", updated 2026-02-19) [1P]:
  "Select (D-pad Center) 13 … Up 38 … Down 40 … Left 37 … Right 39 … Play/Pause 179 … Rewind 227
  … Fast Forward 228 … Back 4". "The Home, Menu, and Voice Search buttons cannot be captured."
  "Amazon WebView supports the W3C standard Gamepad APIs."
- **Silk.** An Amazon employee, 2019-01-24 (web-app-starter-kit-for-fire-tv #63): "Both Silk and
  Firefox seem to be capturing the remote's button presses for the virtual cursor. This is how
  they were designed" [1P].
  - A third-party repository from 2026-10-21 reports: "only Play/Pause ever reached a web page.
    D-pad, Select and Back never did" (Fire OS 8, one measurement) [U].
  - A spatial-navigation toggle in Silk appears in a search summary only [S]. The owner has
    played Orpheum in Silk ([milestone-reassessment-2026-10-18.md](milestone-reassessment-2026-10-18.md)
    B1), with the cursor [I].

### 2.2 Samsung Tizen

From Samsung's "Remote Control" guide [1P]:

- **Codes:** "ArrowLeft 37 ArrowUp 38 ArrowRight 39 ArrowDown 40 Enter 13 Back 10009".
  - Media: "MediaPlayPause 10252 MediaRewind 412 MediaFastForward 417 MediaPlay 415 MediaPause 19
    MediaStop 413 MediaRecord 416 MediaTrackPrevious 10232 MediaTrackNext 10233".
  - Colours 403–406.
- **Registration:** "The 'ArrowLeft', 'ArrowUp', 'ArrowRight', 'ArrowDown', 'Enter', and 'Back'
  key events are detected automatically, and do not require separate registration." Every other
  key needs `tizen.tvinputdevice.registerKey` (or `registerKeyBatch`) and the
  `http://tizen.org/privilege/tv.inputdevice` privilege. `getSupportedKeys()` lists what the
  device has.
- **Back policy:** "Long-pressing the 'Return/Exit' key forces the application to terminate. Do
  not register the 'Return/Exit' key long-press."
  - From a detail page, Back "must display the previous page"; from the home page, "an exit
    confirmation popup".
- **The built-in browser:** "it is not possible to make the remote control special keys, such as
  the color or playback control keys, work in the Web browser" (FAQ Q9). Whether its arrows reach
  a page or move a pointer was not found.
- **The engine:** Tizen 10 (2026) is the only platform found that lists CSS spatial navigation's
  properties as supported (web engine specifications, table 140). Whether it works is unverified.

### 2.3 LG webOS

From LG's Magic Remote and Back Button guides [1P]:

- **Codes:** "Left 37 Up 38 Right 39 Down 40 OK 13 Back 461 Red 403 Green 404 Yellow 405 Blue
  406". The conventional remote adds "Play 415 Pause 19 Fast-forward 417 Rewind 412 Stop 413",
  which "do not exist on Magic Remote".
- **Back is the page's history by default.** "By default, the webOS TV Platform itself handles the
  Back button press on Magic Remote using the DOM's history object… your app receives the
  popstate event." With `"disableBackHistoryAPI": true` in `appinfo.json`, the app gets keydown
  461 instead. `webOS.platformBack()` shows the exit popup (webOS 6.0+).
- **Pointer and 5-way.** "Pressing any of the arrow keys… while using pointer mode will switch the
  navigation mode to 5-way mode. Shaking the Magic Remote… will switch… to Pointer mode", and
  "all apps MUST also support 5-way mode." `cursorStateChange` reports which [1P/S]. A TV mode
  must therefore handle a pointer and the D-pad on the same screen [I].
- The webOS built-in browser was not checked.

### 2.4 Game controllers: the Gamepad API

- **Status.** An editor's draft; Working Drafts through 2025 [S, w3.org blocked]. **The secure
  context requirement was dropped**: w3c/gamepad PR #194, merged 2024-02-22 ("Drop SecureContext
  extended attrs from interfaces"); WebKit followed (commit `5300e77`) [1P]. It matters for a
  plain-HTTP LAN client [I]. Chromium throws `SecurityError` only when the `gamepad`
  Permissions-Policy is disabled (`navigator_gamepad.cc:108-111`) [1P].
- **Support** (MDN compatibility data): Chrome 35, Firefox 29, Safari 10.1 and iOS alike;
  **Android WebView as Chrome** [1P].
- **It must be polled.** The spec: "the gamepad data should be polled as closely as possible to
  immediately before the animation callbacks", that is, in `requestAnimationFrame` [1P].
  `getGamepads()` returns an empty list until a button is pressed ("If [[hasGamepadGesture]] is
  false, then return an empty list"); `gamepadconnected` fires then [1P]. A button press counts as
  user activation in Chromium (`navigator_gamepad.cc:125-131`), which can unlock autoplay [1P].
- **Standard mapping** [1P]: buttons 0–3 the face buttons (0 bottom, 1 right, 2 left, 3 top),
  4–7 shoulders and triggers, 8/9 select/start, 10/11 stick presses, **12–15 the D-pad**, 16 the
  centre button. Axes 0/1 the left stick, 2/3 the right. On Chrome for Android, Xbox, DualShock,
  DualSense, Shield, Stadia and Amazon pads have mappings; Switch pads do not
  (`GamepadMappings.java:60-135`) [1P].
- **On Android, using the Gamepad API swallows the controller's keys.**
  `ContentUiEventHandler.dispatchKeyEvent` offers each key to the gamepad code first (`:152`).
  Once the page polls the API, `GamepadList.handleKeyEvent` takes D-pad and gamepad button keys
  from joystick-source devices (`GamepadList.java:183-190`, `:276-300`) [1P]. **A page that polls
  stops getting arrow keys from a controller, though a TV remote, not a joystick, is unaffected**
  [I, from the code]. Without the API, a controller's D-pad arrives as arrows, but its A button
  arrives as `keyCode` 0 with no `key` [I, from the mapping table]. Android's fallback of A to
  centre and B to Back (AOSP `Generic.kcm`) is unlikely to fire, because Chromium reports the key
  handled (unverified).
- **Xbox's Edge** once offered `navigator.gamepadInputEmulation`: "Setting it to `keyboard` turns
  off mouse mode, and instead gamepad input generates DOM keyboard events" (Microsoft, archived,
  2017-02-08) [1P]. Whether Chromium Edge on Xbox keeps it is unverified.

### 2.5 Keyboards

- Tab and Shift+Tab move focus in document order; the browser does nothing on arrows outside a
  form control or a scrolling box [1P, HTML focus navigation].
- **YouTube's player shortcuts** (help article 7631406, ©2026) [1P]: k play/pause, j/l −/+10 s,
  arrows ±5 s on the seek bar, up/down volume, 0–9 jump to 0–90 %, f fullscreen, m mute,
  c captions, `/` search. "You must click the video player before using keyboard shortcuts" in
  the new desktop interface. **Netflix:** Enter/Space, F, Esc, ←/→ ±10 s, ↑/↓ volume, M, S
  (help 24855) [1P].
- **WCAG 2.2** [1P]:
  - 2.1.1 Keyboard: "All functionality… operable through a keyboard interface".
  - 2.1.4 Character Key Shortcuts: a single-letter shortcut must be possible to "Turn off",
    "Remap", or be "Active only on focus". k/j/l-style keys fall under this.
  - 2.4.3 Focus Order, 2.4.7 Focus Visible.
  - 2.4.11 Focus Not Obscured (AA): focus not "entirely hidden due to author-created content".
    A sticky header or a player bar can break it.

### 2.6 Media keys and Media Session

- **Support** (MDN): Chrome 73, Firefox 82, Safari 15; **not Android WebView** (§2.1) [1P].
  Actions play, pause, seekbackward, seekforward, previoustrack and nexttrack in all three;
  seekto from Chrome 78 [1P].
- "Hardware media keys are supported… web developers can handle this user action whether the
  browser is in the foreground or the background" (web.dev, 2024-06-10) [1P]. Whether desktop
  Chrome also fires a keydown for them is unverified; WebKit's Jer Noble, 2017: "I don't think
  that the media buttons will ever generate key events on Mac platforms" (w3c/mediasession #56)
  [1P].
- **So media keys come two ways:** as Media Session actions on desktops and phones, as keydowns
  (179, 415, 10252…) on TVs and in WebViews. One key map has to accept both [I].

### 2.7 What is taken before the page

- **Android WebView and Chrome:** Back, Home, volume and power never reach Blink (§2.1).
- **TV Bro:** Back always; the D-pad and OK in cursor mode; four media keys always (§2.1).
- **Silk and Firefox on Fire TV:** the D-pad, Select and Back, for the cursor (§2.1).
- **Fire TV, any app:** Home, Menu and voice search (§2.1).
- **Tizen:** a long press of Return/Exit ends the app (§2.2).
- **webOS:** Back becomes `history.back()` unless the app opts out; a long press goes Home (§2.3).
- **Fullscreen:** "User agents should provide a means of exiting fullscreen that always works";
  Esc can be held back only through keyboard lock (WHATWG Fullscreen, 2026-10-19) [1P].
- **Desktop media keys:** to Media Session and the browser's global media controls (§2.6).

---

## 3. Spatial navigation and focus in web apps

### 3.1 What the platform offers

- **CSS Spatial Navigation** stopped at "W3C Working Draft 26 November 2019" (css-nav-1). The
  editors' draft moved to `css-spatial-nav-1` (an earlier commit, 2026-06-01; draft dated
  2026-05-14). WICG's repository is archived; its last commit, 2026-03-23, is "Merge pull request
  #231 from WICG/transition" [1P]. Its polyfill, 1.3.1 (2019-11-29), is unmaintained [1P].
- **Chromium has an implementation** behind `--enable-spatial-navigation`
  (`content_switches.cc:398-399`; `web_contents_impl.cc:4100-4101`), on by default in a WebView
  on a device without a touchscreen (§2.1) [1P]. It focuses the element and then dispatches a
  synthetic mousemove (`spatial_navigation_controller.cc:400-408`), so hover styles fire too
  [1P].
- **`focusgroup` shipped in Chrome 150** (stable 2026-06-30): "declaratively give composite
  widgets arrow key navigation, a guaranteed tab stop, and last-focused memory, replacing
  hand-coded roving tabindex scripts" [1P]. Android WebView 150 has it; Firefox and Safari do not
  (MDN). Mozilla's position is positive (standards-positions #1348); WebKit's is open with
  "concerns: device independence" (#171) [1P]. It moves within one widget, not between a row and
  the header [I]. No Tizen or webOS engine has it: webOS 26 is Chromium 132, Tizen 10 is M130
  [1P]. It was in an origin trial when
  [ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md) §3.7 was written.
- **`inert`** (Chrome 102, Firefox 112, Safari 15.5) and **`:focus-visible`** (Chrome 86,
  Firefox 85, Safari 15.4) are usable on webOS 24+ and Tizen 8+ [1P]. Selectors 4: "If the
  previously-focused element indicated focus, and a script causes focus to move elsewhere,
  indicate focus on the newly focused element" [1P]. So a ring drawn by `:focus-visible`, as
  Orpheum's `tokens.css` does, follows a focus engine that calls `.focus()` after a key [I].
- **`focus({preventScroll: true})` is not honoured on Chrome for Android or Android WebView**
  (MDN data; crbug 41453122) [1P]. A TV mode that scrolls rows itself cannot rely on it.

### 3.2 Libraries

Versions from npm and stars from GitHub, 2026-10-25; sizes are bundlephobia's minified/gzipped
[1P].

| Library | Latest | Licence | Size | Model | State |
| --- | --- | --- | --- | --- | --- |
| `@noriginmedia/norigin-spatial-navigation` (core 4.2.0, react 3.3.0) | 2026-10-23 | MIT | 52/17 KB (core) | React hooks over its own tree | Active, 492★ |
| `@bbc/tv-lrud-spatial` | 0.0.18, 2026-07-09 | Apache-2.0 | 5.1/2.2 KB | DOM: finds the next element, real `.focus()` | Active, 68★ |
| `@enact/spotlight` | 5.6.0, 2026-09-02 | Apache-2.0 | 102/27 KB | LG's Enact framework; from js-spatial-navigation | Active; React `^19.2.8` as a dependency |
| `js-spatial-navigation` | 1.0.1, 2017 | MPL-2.0 | 11/4 KB | DOM | Dormant |
| `react-tv-space-navigation` | 5.2.0, 2025-07 | MIT | 64/16 KB | React Native; web only through react-native-web | Slow |
| `lrud` (BBC, abstract tree) | 8.0.0, 2021 | Apache-2.0 | — | Not DOM | Archived |
| `spatial-navigation-polyfill` | 1.3.1, 2019 | MIT | 17/5 KB | The CSS draft as script | Unmaintained; Stremio uses a fork |

- **Norigin.** `useFocusable` takes `focusKey`, `saveLastFocusedChild` (default true),
  `trackChildren`, `isFocusBoundary`, `onEnterPress`, `onArrowPress` and others
  (`packages/react/src/useFocusable.ts:52-79`); `setKeyMap` changes the codes
  (`SpatialNavigation.ts:55-61`) [1P]. **By default it does not move DOM focus**: "By default, the
  library manages focus entirely in JavaScript without calling HTMLElement.focus()";
  `shouldFocusDOMNode` is advised for "screen reader compatibility" [1P]. A route's components
  are deleted with their last focused child (`SpatialNavigation.ts:1579-1589`), so restoring
  focus on Back needs the app's own memory [1P]. Its repository builds against React 18; React 19
  is allowed by `>=16.8` but untested there (unverified) [1P].
- **BBC lrud-spatial.** `getNextFocus(currentFocus, keyOrKeyCode, [scope])`. Focusable:
  `[tabindex]`, `a`, `button`, `input`. Containers: `nav`, `section` or `.lrud-container`, with
  `data-focus` to remember the last child and `data-block-exit` [1P]. No overlay (z-order)
  handling and no `inert` awareness [1P/I]. Its tests are browser tests that press arrows and
  read `document.activeElement` (§3.6).
- **Engines inside products** (§4): Jellyfin's `focusManager.js` and Stremio's polyfill fork are
  both geometry searches over real DOM focus, with containers that bound the search.
- **Products that do not use the DOM:** Netflix's TV app is "an SDK installed natively on the
  device, a JavaScript application … and a rendering layer known as Gibbon" (Netflix TechBlog,
  2017, Wayback) [1P]; React-Gibbon draws to Gibbon, not to a DOM. Lightning (Blits) draws to
  WebGL. Disney's TV stack is a native runtime with WebAssembly by search summaries only [S].
  These are evidence that JavaScript TV interfaces work, not that DOM ones do [I].

### 3.3 Focus practice at ten feet

- **One item is always focused.** Android TV: "always have an item in focus that a user can take
  action on immediately after your app starts", and "Use a uniform scheme for indicating focus"
  [1P].
- **Google TV's focus system** (2024-03-21): states default, focused, pressed; indicators scale,
  border, glow and colour; "Default scaling values are: 1.025, 1.05 and 1.1x" [1P]. Within
  [D189](../../decisions/working-notes.md)'s bound (growth inside half the gutter), 1.025–1.05 fits
  [I].
- **Overscan.** Android TV's 960×540 dp canvas with "5% margin of 48dp on the left and right
  sides, and 27dp" top and bottom [1P]. Microsoft scales HTML apps on Xbox to 150 %, and asks that
  crossing the screen take "no more than six clicks" ("Designing for Xbox and TV", 2026-03-13)
  [1P].
- **Roving tabindex** (WAI-ARIA APG): "set tabindex="-1" on the element that has tabindex="0".
  Set tabindex="0" on the element that will become focused… element.focus()". If the focused
  element "is hidden or removed from the DOM … browsers move focus to the body element" [1P].
  That is the hazard of virtualised lists: Orpheum's lists page in as they scroll
  (`use-pages.ts`), and must keep the focused tile mounted [I].
- **Scroll on focus.** Jellyfin centres the focused element on every `focusin` in TV layout
  (`scrollManager.js:484-486`, `:582-588`; `scrollHelper.ts:84-133`) [1P]. A fixed focus position
  in a row, as streaming apps are said to do, has no source here (unverified).
- **Focus after Back.** Back is only worth taking if the focused tile comes back
  ([back-navigation-2026-10-24.md](back-navigation-2026-10-24.md) §4 item 7). No library keeps it
  across a route change (Norigin above), so it belongs in the app's history state, beside
  `appDepth` (`navigation.ts`) [I].
- **The player.** [D188](../../decisions/working-notes.md) and
  [tv-client.md](../plans/tv-client.md) already fix the remote's contract: left and right always
  seek from the current position, 10 s steps, the controls hide quickly, track menus are buttons.
  Jellyfin's player shows its controls on the first arrow or Enter and focuses the seek bar or
  play (`apps/legacy/controllers/playback/video/index.js:1269`, `:1290`) [1P]. How the browser's
  own `<video controls>` behave under the D-pad in WebView was not checked (unverified); Orpheum's
  Play page uses them today (`film-player.tsx`, `controls={props.hasControls}`).
- **Text on a TV.**
  - **Tizen:** "When an input or textarea element … is focused, the virtual keyboard is shown";
    Back or Done hides it [1P].
  - **webOS:** the keyboard opens on focus and "cannot be disabled programmatically"; it covers a
    third of the screen [1P].
  - **Android WebView:** uses the system input method [I].
  - **Jellyfin** has no on-screen keyboard of its own, only an A–Z jump bar (`alphaPicker`) [1P].
  - The custom keyboards of YouTube and Netflix have no source here (unverified).
  - Orpheum signs a TV in by a code ([D93](../../decisions/working-notes.md)), so typing is needed
    only for search [I].

### 3.4 Screen readers and spatial navigation

- "When Talkback is switched on, D-pad events are not passed to the D-pad handler defined by
  developer. Instead, accessibility services receive the key events so they can move the
  accessibility focus" (Android TV accessibility guide, 2026-09-09) [1P]. A WebView report says
  the same (react-native-webview #1882, 2021) [U].
- **So a script focus engine is off for TalkBack users, and the page must also work in plain
  document order with real roles and names** [I]. A focus engine that moves real DOM focus keeps
  the two models the same; one that keeps its own focus state, Norigin's default, makes them two.
  [tv-client.md](../plans/tv-client.md) ("building the focus model twice") asks for one.

### 3.5 What Orpheum has today

- **Every control is a real link or button.** Posters are `<Link>`s in a `<ul>` (`browse.tsx`),
  the header is a `<details>` menu (`app-shell.tsx`), dialogs use `<dialog>` with `showModal()`
  (`modal-dialog.tsx`). So Tab reaches everything, and WebView's built-in spatial navigation has
  real targets [I].
- **One focus ring for all**: `:focus-visible` with a two-colour ring (`tokens.css`), as
  [D189](../../decisions/working-notes.md) asks.
- **No key handling of its own** beyond Escape for the banner menu and dialogs, and no arrows,
  `tabIndex` management, gamepad code or TV layout. `play-state.ts` listens to Media Session
  actions to label pauses, not to act on them.
- **The Play page uses the browser's `<video controls>`.** An on-screen Back exists for drill-down
  pages because TV Bro keeps its own (`back-button.tsx`).
- **The test tiers can drive it**: Vitest browser mode runs Chromium and Firefox through
  Playwright (`web/vitest.browser.config.ts`), and the Playwright tier the same
  (`web/e2e/playwright.config.ts`).

### 3.6 Testing focus in a browser

- **Arrows:** BBC's lrud-spatial tests are Puppeteer: `page.keyboard.press('ArrowRight')`, then
  `page.evaluate(() => document.activeElement.id)`, over HTML layout fixtures (`test/lrud.test.js`)
  [1P]. Playwright's `keyboard.press` is the same [1P]. Norigin tests in jsdom with stubbed
  offsets, never in a browser [1P]. jellyfin-web has no focus tests [1P].
- **Gamepads:** Playwright has no gamepad API, and the DevTools protocol has no gamepad emulation
  [1P]. Playwright's documented route for a missing API is `page.addInitScript` replacing it
  [1P]: a fake `navigator.getGamepads` returning an object the test changes, and a dispatched
  `gamepadconnected`. Chromium's `gamepadController` exists only in content_shell [1P].
- **Platform key codes** (461, 10009, 10252…): `keyboard.press` sends only named keys, so a test
  would dispatch a `KeyboardEvent` whose `keyCode` is defined on it [I, unverified as a pattern].
- **WebView's own spatial navigation** could be reproduced by launching Chromium with
  `--enable-spatial-navigation`, to catch our engine and Blink's fighting. Whether headless
  Chromium honours the flag is unverified.
- **What a browser cannot test**: [D190](../../decisions/working-notes.md)'s "remote key to focus
  moved ≤ 250 ms" on the worst device, TV Bro's modes, and Tizen/webOS codes. Those are `VQ`
  items for the device (AGENTS.md rule 4).

---

## 4. Competitors

| Product | Web client and keys | TV apps | Source |
| --- | --- | --- | --- |
| **Jellyfin** | Arrows move focus **only in "TV" display mode**; Tizen/webOS codes mapped; gamepad turned into keys (off by default) | Tizen: jellyfin-web packaged; webOS: the server's jellyfin-web in an iframe; Android TV: native Kotlin | §4.1 [1P] |
| **Plex** | "The Web app has not had any arrow key navigation. We support `tab`" (staff, 2018) | LG app "rewritten from the ground up… taking advantage of WebGL" (2020); 2026 apps React Native | §4.2 [1P] |
| **Emby** | Remote support mainly in TV mode | Emby Theater: Electron, "designed around 6 buttons"; Xbox: "a static web app" | §4.3 [1P] |
| **Stremio** | Arrows move focus everywhere (spatial-navigation polyfill); gamepad by its own polling, opt-in | Samsung, LG (web player backends), Android TV | §4.4 [1P] |
| **YouTube** | Desktop: shortcuts; youtube.com/tv: Leanback, the TV app as HTML5 | Cobalt runs the same HTML5 app on TVs | §4.5 [1P] |
| **Netflix** | Desktop: player shortcuts only | JavaScript on Gibbon, not a DOM | §4.5 [1P] |
| **Kodi** | Its web interface (Chorus2) is a remote control for Kodi, not a ten-foot UI | Native, XML keymaps | §4.6 [1P] |

### 4.1 Jellyfin

The closest analogue: a web client that is also its own TV app.

- **One keydown listener** (`src/scripts/keyboardNavigation.js:152-260`) [1P].
  - Its key table (`:14-64`) names Xbox codes, the Tizen/webOS media keys 412, 413, 415, 417,
    Back as 461 (webOS) and 10009 (Tizen), and Tizen's 10232, 10233, 10252.
  - `KeyAliases` (`:66-81`) turns gamepad keys into arrows and B into Escape.
  - **Arrows move focus only in TV layout**: `if (!layoutManager.tv && isNavigationKey(key))
    return;` (`:163`).
  - Escape means Back only in TV layout (`:212-217`).
  - Hisense VIDAA sends Backspace for Back (`:203-210`).
  - Every key becomes a named command for `inputManager.handleCommand`.
- **Commands** (`src/scripts/inputManager.js:55-249`): a cancelable `command` event goes first
  to the focused element or the top dialog, so a page can take one. `back` calls
  `appRouter.back()`, or `appHost.exit()` with no history (`:112-118`) [1P].
- **Focus search by geometry** (`src/components/focusManager.js:233-385`) [1P].
  - Candidates are focusable tags and `.focusable`, beyond the current element in the arrow's
    direction.
  - Nearest wins, with the cross axis counting zero where the two overlap.
  - `focuscontainer`, `-x`, `-y` and the edge variants bound the search (`:146-197`).
  - With nothing focused it focuses the first element (`:242-244`).
- **TV layout is detected, or chosen** [1P]:
  - `browser.js:1-22` guesses a TV from the user agent ("This is going to be really difficult to
    get right").
  - The user's "Display mode" setting (Auto/Desktop/Mobile/TV) overrides the guess.
  - TV is a legacy mode, served by the old app, not the new React one (`RootAppRouter.tsx:26`).
- **Gamepad** (`src/scripts/gamepadtokey.js`, Microsoft's 2016 MIT code) [1P]:
  - It polls every 20 ms with `setInterval` (`:374-377`) and fires synthetic keydown/keyup at the
    focused element (`:202-213`).
  - Repeat is 200 ms, then 40 ms (`:177-186`).
  - It is loaded on `gamepadconnected` only if the setting is on, which is off by default. Its
    help text: "Requires: 'TV' Display Mode".
- **The packaged apps** [1P]:
  - **webOS** loads the server's jellyfin-web in an `<iframe>` and injects `NativeShell`, whose
    default layout is `'tv'` (`js/index.js:455-511`; `webOS.js:69-71`). `appinfo.json` sets
    `"disableBackHistoryAPI": true`, so Back arrives as 461.
  - **Tizen** builds jellyfin-web into the package (`gulpfile.babel.js:10`). It registers the
    media keys on load, and `MediaPlayPause` only on the queue and video pages (`tizen.js:182-201`).
- **Complaints** (jellyfin-web issues) [U]:
  - #303 (2019, webOS): the highlight "does not move with arrow keys".
  - #4296 (2023, open; Steam Deck): the advice is "Try changing the `Display mode` … to `TV`";
    2026-10-13: "Seems like it's broken again".
  - #5016 (Xbox, closed 2026-01): "full controller navigation when using the TV display mode and
    with 'Enable Gamepad'".
  - #7834 (2026-04, Edge with an Xbox pad): A, B and the stick fire "way too often".
  - #2623: arrows do not scroll to follow focus. #3413: focus always lands on the first item.
  - #8609 (2026-10): the header vanishes in "Auto".
  - A 2025-08-22 forum script overrides "Jellyfin's (imo broken) 'TV-mode'".
- **The lesson** [I]: the mechanism is sound. The failures come from keeping it as a separate
  mode that must be guessed from the user agent, and from a second, legacy app behind it. An
  engine that is on for every keyboard, with only the layout switching, avoids both.

### 4.2 Plex

- **Plex Web has no arrow navigation.** Staff, 2018-04-03: "The Web app has not had any arrow key
  navigation. We support `tab` (forward) and `shift + tab` (backward)" [1P]. Arrow navigation
  lived in Plex Media Player, a Qt WebEngine host of the web client, archived 2022.
- **Plex HTPC** (2021-03-23) pairs "the app framework used by the Plex Windows and macOS apps with
  the UI that powers our modern smart TV and game console apps", with DualShock 4 and Xbox One
  mappings and JSON input maps [1P].
- **Its LG app is web.** The webOS preview (2020-06-30) was "rewritten from the ground up",
  "taking advantage of WebGL"; a user there found the "magic remote cursor … doesn't seem to work
  at all" [1P/U]. The 2026 apps are React Native
  ([tv-apps-react-native-2026-10-24.md](tv-apps-react-native-2026-10-24.md) §8).
- **Complaints** [U]: Back "takes you back to the navigation bar entry where you started"
  (920359, 2025-05-27). On Roku: "I can't simply use the UP/DOWN arrow buttons to move between
  library items and the menu bar" (931048, 2025-09-16).

### 4.3 Emby

- Emby Theater: "Chromium, Electron, and NodeJS", "designed for 10ft", and "all functions are being
  designed around 6 buttons - up, down, left, right, ok, and back" (Emby blog, c. 2016-03) [1P].
- Xbox: "The D-Pad works just like any other remote, with no mouse cursor needed"; "It's a static
  web app" (2016-03-13) [1P].
- Its key article covers the web app and Tizen, Xbox and PS4 alike: Alt+Left back, Space play,
  left/right seek "when OSD not showing" [1P].
- Remote support is mainly TV mode's: "the degree of support for this in non-TV mode will not be
  as much"; "non TV mode has other requirements to fulfill that sometimes conflict" (forum 143745,
  2025-10) [1P].

### 4.4 Stremio

- **Arrows move focus on every layout.** `src/App/App.js:3` requires its fork of the W3C
  polyfill. The fork's keydown listener navigates unless the event was handled or a text field
  has focus (`polyfill/spatial-navigation-polyfill.js:75-100`) [1P]. Menus and the player bound
  the search with `--spatial-navigation-contain: contain`.
- **Gamepads have a second system** [1P]:
  - `GamepadProvider.tsx:143-257` polls on each animation frame while the document has focus,
    with stick repeat that quickens with deflection. It is off unless the profile enables it.
  - `useContentGamepadNavigation.tsx:55-90` scores neighbours by distance along the arrow plus
    three times the sideways distance. **That is two focus searches in one app**, against
    AGENTS.md rule 11 if we copied it [I].
- On Samsung (2023-11): "navigation only works in the movie selection" [U].

### 4.5 YouTube and Netflix

- **YouTube's TV interface is a web app.** Cobalt is "a lightweight application container …
  compatible with a subset of the W3C HTML5 specifications", built to run "our HTML5-based video
  browse and play application" [1P].
  - youtube.com/tv reports client `TVHTML5` and build `youtube.kabuki.web_20261021_13_RC00`.
  - Its script draws focus with its own class (`.zylon-focus`, `zylon-unfocusable`) and has flags
    such as `enable_virtual_list_focus_lock` [1P, minified]. So focus is the app's state, drawn
    by a class [I].
  - In a browser, Leanback is blocked on purpose; with a spoofed user agent "it doesn't support
    controllers outside of the official app" (VacuumTube README, 2026-10-18) [U].
  - No report was found of youtube.com/tv used in TV Bro or Silk.
- **Netflix** on TVs runs JavaScript over Gibbon (§3.2). Its focus engine is not public.
- **This is the strongest evidence that a web stack can meet TV users' expectations at scale**
  [I]. YouTube also controls its container (Cobalt), and so the keys that reach the page.

### 4.6 Kodi

Kodi's input is native XML keymaps: "Kodi falls back to the global map when the window-specific
map hasn't bound the key being pressed" (kodi.wiki) [1P]. Chorus2 is a remote control for Kodi
("Remote control to navigate Kodi UI and control playback") [1P], not a pattern for us.

### 4.7 Patterns

1. **Keys become commands.** Jellyfin's `handleCommand`, Plex HTPC's input maps and Kodi's
   keymaps all turn each platform's codes into named commands (back, play/pause, seek) that pages
   handle. Platform differences live in one table [I].
2. **Focus search by geometry over real DOM focus**, bounded by containers: Jellyfin, Stremio,
   BBC, Blink's own [1P].
3. **A mode guessed from the user agent goes wrong.** Jellyfin's detection, and a Steam Deck or an
   Xbox falling outside it, are its most persistent complaints [U].
4. **Gamepads are turned into the same commands** (Jellyfin, Microsoft's guidance), or into a
   second engine (Stremio). Microsoft: "The quality of gamepad and remote behavior that you get
   out-of-the-box depends on how well keyboard is supported in your app" (2026-03-04) [1P].
5. **The ten-foot apps own their container** where they can: Cobalt, Gibbon, Electron, Qt
   WebEngine, a packaged Tizen/webOS app, Jellyfin's webOS iframe. None relies on a third-party
   TV browser [I].

---

## 5. Feasibility

### 5.1 What a web app can do well

- **Everything inside the page**: a key map, a focus engine, rows that scroll to their focused
  tile, focus restored on Back, a ten-foot layout, the remote's player model and code sign-in.
  Jellyfin, Emby and Stremio do it in the DOM; YouTube does it at Google's scale (§4).
- **Keyboards for free.** The same engine serves a desktop keyboard user, a Steam Deck or an Xbox
  controller in a desktop browser, and WCAG 2.1.1 [I].
- **Packaged TVs.** A Tizen or webOS app gets arrows, OK and Back without asking, and the media
  keys by registering them (§2.2, §2.3). The web TV mode is what such a package needs; without it
  a package is the desktop layout under a cursor [I].

### 5.2 What costs

All rough, in focused days of the owner with agents [I]:

- **An input layer:** one key map (DOM keys, legacy `keyCode`s per platform, Media Session
  actions, gamepad buttons) to named commands. 2–4 days.
- **A focus engine:** geometry search over real DOM focus, containers, last-focused memory per
  container and per history entry, scroll-into-view by our own code, and the virtualised list's
  focused tile kept mounted. 5–8 days, more if the D-pad latency gate needs work on the TCL.
- **A ten-foot layout:** larger type and tiles, overscan margins, one focused item on arrival,
  no hover-only controls, the header reachable in one move. Per page, not a second app. 8–12
  days for the pages in §6.
- **The player under a remote:** our own controls in TV mode (left/right seek from any state,
  OK shows the controls, track buttons), because the browser's `<video controls>` give no control
  over the D-pad (unverified how they behave). 6–10 days.
- **Choosing the mode:** a per-device setting, with a guess from the input actually seen (the
  first arrow key, a `cursorStateChange`, `pointer: none`) rather than from the user agent alone.
  1–2 days.

### 5.3 What needs a native app

| Need | Why the web cannot | Smallest native answer |
| --- | --- | --- |
| **The remote's Back in TV Bro, Silk, Chrome** | Android gives Back to the app, never the page (§2.1); TV Bro uses it for its own UI | Any app owning the WebView: a thin shell |
| **Keys at all in TV Bro's default mode and in Silk** | Their cursor consumes the D-pad (§2.1) | Direct mode by hand each time, or a shell |
| **Media keys in TV Bro** | Taken by TV Bro (§2.1) | A shell |
| **A launcher icon, a home-screen row** | A browser bookmark is not one (unverified for TV Bro's own shortcuts) | A shell; launcher rows need more native code |
| **Voice search, the Home and Menu keys** | Taken by the system (§2.1, Amazon) | Native only; not needed for two users [I] |
| **Playback the browser cannot do** (frame-rate matching, HDR switching, a different A/V clock) | The WebView's player | The native player of [tv-apps-react-native-2026-10-24.md](tv-apps-react-native-2026-10-24.md); **a shell does not help** |

**A thin Android shell** is an `Activity` hosting a `WebView` that loads the client, passes every
key, maps Back to a command (the web's Back, or exit at the start page), and keeps the screen on
[I]. Amazon's own web-app runtime is this shape (§2.1), and Jellyfin's webOS app is the same idea
on LG (§4.1). Rough cost: 1–2 weeks, sideloaded by APK like
[D108](../../decisions/working-notes.md)'s versions, with Google's developer verification still
applying ([app-stores.md](app-stores.md) §4). It keeps the TCL's WebView, so the sound lag is
whatever the web fix makes it
([tv-apps-react-native-2026-10-24.md](tv-apps-react-native-2026-10-24.md) §5) [I]. Whether such
a shell is a separate client in the owner's sense (it is a native app in an app store's sense) is
the owner's call.

### 5.4 Our TV Bro case, concretely

- **Today, in cursor mode,** the client is a pointer interface on a TV, and it works because
  everything is a link or a button. The remote's Back toggles TV Bro's menu bar, so the on-screen
  Back is the only way back (§2.1).
- **In direct mode, with no code,** WebView's own spatial navigation should already move focus
  between our links and buttons with the D-pad, and OK should follow them [I, from
  `AwSettings.java` and TV Bro's help text; unverified on the TCL]. What it will not do: keep
  focus on the tile Back returns to, scroll rows sensibly, or drive the player's controls in our
  model (unverified).
- **Once our engine handles arrows,** it takes over from WebView's (it calls `preventDefault()`),
  and behaves the same in TV Bro's direct mode, in a shell, on Tizen and webOS, and on a desktop
  keyboard [I].
- **What stays broken in TV Bro whatever we do:**
  - the remote's Back leaves direct mode, so after each Back the viewer re-enters it from the
    cursor menu (unverified how many presses that takes)
  - the four media keys act on the `<video>` directly, not through our commands
- **So TV Bro is a test bench for the TV mode, not its home.** A shell or a packaged TV app is
  where it is smooth (judgement).

---

## 6. Recommendation for Orpheum

**Build controller and keyboard input into the web client, as one input layer and one focus
engine that are always on, with a ten-foot layout that switches per device.** Judgement, for
these reasons:

- It is the only route that also serves keyboards, desktops with controllers, Samsung and LG
  (§5.1), and it is the B1 item already asked for
  ([milestone-reassessment-2026-10-18.md](milestone-reassessment-2026-10-18.md)).
- It does not need a native app except where a browser keeps keys for itself (§5.3). On Android
  TV that need is met by a thin shell more cheaply than by the react-native-tvos client, which
  stays the answer for playback reasons, not input ones.
- It follows [ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md) R7: one module,
  replaceable inside, now that `focusgroup` exists in Chromium only.

### 6.1 The shape

1. **Input → commands** (one module): `keydown` with `key` first, then the platform `keyCode`s
   (Tizen 10009, 10252…; webOS 461; Android 179, 227, 228; VIDAA's Backspace), Media Session
   actions, and gamepad buttons, all to commands: `up`, `down`, `left`, `right`, `select`, `back`,
   `playPause`, `seekBack`, `seekForward`, `menu`. Pages subscribe to commands, never to keys.
   Single-letter shortcuts only while the player is focused (WCAG 2.1.4).
2. **Focus engine** (one module, behind a narrow interface): moves **real DOM focus** with
   `.focus()`, so `:focus-visible` (`tokens.css`), screen readers and Tab agree (§3.4). Geometry
   search bounded by containers marked in the markup (a row, the header, a dialog, the player);
   `inert` outside an open dialog. Remembers the last focused element per container, and per
   history entry for Back. Scrolls a row to its focused tile itself.
   - **Build or take (owner's call, AGENTS.md rule 3):** `@bbc/tv-lrud-spatial` (Apache-2.0,
     5 KB, real DOM focus, browser-tested) does the search and containers; Norigin is larger and
     keeps its own focus by default, which is the wrong model for us (§3.2, §3.4). A search of
     our own is a few hundred lines, as Jellyfin's is [I]. Either way, memory and scrolling are
     ours.
3. **Ten-foot layout** as a mode of the same pages: tokens for type, tile size, gutter and
   overscan, chosen by a per-device setting with a guess from the input seen (§5.2). The engine
   is on in every mode; only the layout switches (§4.1's lesson).
4. **The player in TV mode** follows [tv-client.md](../plans/tv-client.md)'s remote contract with
   our own controls; the browser's controls stay elsewhere.
5. **Gamepads** are polled in `requestAnimationFrame` after `gamepadconnected`, only when no key
   events arrive from them (on Android, polling would swallow the controller's keys, §2.4), and
   feed the same commands.

### 6.2 Phases

- **Step 0: no code** (an evening, on the next TV visit).
  - In TV Bro, choose direct mode from the cursor menu and try Home, a poster grid, a title,
    a series, search and Play with the D-pad.
  - Note what WebView's own navigation gets right and wrong, how many presses it takes to
    re-enter direct mode after Back, and what the four media keys do to our player.
  - Try a gamepad in TV Bro if one is to hand.
  - Add what is confirmed to [idiosyncrasies.md](../../idiosyncrasies.md).
- **Step 1: input layer and focus engine** (1.5–2.5 weeks, rough).
  - Rows and grids on Home, Films, Series and search results; the header; dialogs.
  - Focus restored on Back through `history.state`.
  - Desktop keyboards benefit at once.
  - **Pass:** arrows reach every control the pointer does, in Chromium and Firefox, and Back
    returns focus to the tile it left.
- **Step 2: ten-foot layout and the player** (3–5 weeks, rough).
  - Pages: Home, the lists, title, series and season, Play, the on-screen Back.
  - Then search, with the platform keyboard.
  - The administration pages stay pointer-and-keyboard: the phone does them.
  - **Pass:** D190's ≤ 250 ms from key to focus moved, on the TCL in TV Bro's direct mode, as a
    `VQ` item.
- **Step 3: game controllers** (3–5 days, rough). Standard mapping to commands, with repeat
  timing (Jellyfin's 200 ms then 40 ms is a starting point). **Pass:** an Xbox pad in desktop
  Chrome and Firefox drives the same pages.
- **Step 4: decide the container.**
  - The thin Android shell (1–2 weeks) if Back and the media keys in TV Bro matter enough before
    the react-native-tvos app.
  - Tizen and webOS packaging (1–2 weeks each, plus certification,
    [tv-apps-react-native-2026-10-24.md](tv-apps-react-native-2026-10-24.md) §10.3) when the owner
    wants those TVs.

### 6.3 Testing in our tiers

- **Vitest browser mode** (`*.browser.test.tsx`, Chromium and Firefox): the engine against layout
  fixtures, with `userEvent.keyboard` or Playwright's `keyboard.press('ArrowDown')` and assertions
  on `document.activeElement`, as BBC tests lrud-spatial (§3.6).
- **Platform codes:** dispatch `KeyboardEvent`s carrying 461, 10009, 10252 and 179, to show each
  maps to its command (the pattern is unverified, §3.6).
- **Gamepads:** an init script replacing `navigator.getGamepads` with a fake the test changes,
  then `gamepadconnected`.
- **WebView's built-in navigation:** a Chromium instance with `--enable-spatial-navigation`, to
  show our engine wins (headless support unverified).
- **The Playwright tier:** one walk per page in TV mode: arrive, move, select, Back, and focus
  is where it was.
- **Devices:** the latency gate, TV Bro's modes and media keys, and each packaged platform are
  `VQ` items ([verification-queue.md](../../verification-queue.md)).

### 6.4 Effort

All rough, in focused weeks of the owner with agents, **unverified estimates** by analogy with
the products in §4 and the web client's history:

| Piece | Estimate |
| --- | --- |
| Input layer and focus engine, with focus restored on Back | 1.5–2.5 weeks |
| Ten-foot layout for Home, the lists, title, series, search | 2–3 weeks |
| The player's remote model, our own controls in TV mode | 1.5–2.5 weeks |
| Game controllers | 3–5 days |
| **Web TV mode, total** | **6–9 weeks** |
| Thin Android WebView shell (optional) | 1–2 weeks |
| Tizen or webOS packaging, each | 1–2 weeks plus certification |

### 6.5 How it fits the native-app recommendation

[tv-apps-react-native-2026-10-24.md](tv-apps-react-native-2026-10-24.md) §10.1 recommends a
react-native-tvos Android TV app with a Kotlin player, Swift on Apple TV, and the web client for
Samsung, LG and the browsers. This research changes none of that and adds:

- **The web client's TV mode is a prerequisite for "the web client on Samsung and LG"**, not an
  alternative to the Android app.
- **On Android TV, input alone is not a reason for the React Native app.** A shell over the web
  client solves Back and the media keys. The app's reasons stay playback: the TCL's sync if step
  0b of that research fails, frame-rate matching, HDR and the launcher rows.
- **The two share design, not code.** The commands, the remote's contract, the focus rules of
  [D189](../../decisions/working-notes.md) and the latency gate apply to both. react-native-tvos
  has its own focus engine (that research's §3.1).
- **Order (judgement):** steps 0–1 here first, because they also help desktop keyboards and cost
  little; then that research's steps 0a and 0b; then either the shell or the React Native spike,
  depending on what they show.

---

## 7. Not tried

- **Nothing was run on a device.** TV Bro's direct mode with WebView's spatial navigation, its
  media keys on our player, and how many presses re-enter direct mode, are read from source.
- Whether the owner's TCL declares a touchscreen feature, which decides whether WebView's spatial
  navigation is on.
- Silk's current behaviour beyond a 2019 staff comment and one 2026 measurement; Silk's
  spatial-navigation toggle.
- Samsung's and LG's own browsers; the `key` strings on Tizen and webOS (for example whether Back
  reports `GoBack` or `XF86Back`).
- Chrome on Google TV in 2026.
- How the browser's `<video controls>` behave under the D-pad in WebView.
- Whether Android's fallback keys (A to centre, B to Back) reach a WebView; Switch pad mappings
  on desktops; whether Chromium Edge on Xbox keeps `gamepadInputEmulation`.
- Whether desktop Chrome fires keydowns for media keys, and whether Esc that leaves fullscreen
  reaches the page.
- Norigin on React 19; Tizen 10's CSS spatial navigation; headless Chromium with
  `--enable-spatial-navigation`.
- YouTube TV's and Netflix's focus engines, seek models and on-screen keyboards, beyond minified
  identifiers; Disney's TV stack.
- Plex's current Samsung and LG app technology; Stremio's TV app UI source.
- Reddit was not read.
- The effort estimates are analogies, not measurements.
