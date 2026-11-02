> **ABOUTME:** Letter shortcuts on a computer and touch gestures on phones and tablets for the
> player, and what each does to people who use speech input, a screen reader, sticky keys, a
> switch or another keyboard layout. Covers how WCAG 2.1.4 is met in practice, what a mobile
> browser can reliably do, and which gestures competitors turn on by default and let viewers turn
> off. Ends with options for the owner. Proposes, decides nothing.

# Letter shortcuts and touch gestures for the player, and their accessibility (2026-11-02)

*Decided in D407 §6 and §11: option A. No letter shortcuts, a tap only shows the overlay, and the overlay
never hides while paused, nor while focus is in it except when a TV's remote showed it.*

- **Asked by the owner, 2026-11-02:** D407 drafted that day keeps the
  computer's keys the same as the remote's (§8) and defers two things to research (§9): letter
  shortcuts (K, J/L, F, M, C, digits), with WCAG 2.1.4 in view, and touch gestures (double tap,
  long press, swipes, pinch).
- **Builds on** [player-controls-2026-11-02.md](player-controls-2026-11-02.md) §9 and §10: the
  per-player keyboard and gesture tables, the conventions, WCAG 2.1.4, F99 and G217 in outline,
  Media Session, the mobile-browser limits (iOS volume, iPhone full screen, WebKit bug 240183,
  the 300 ms delay) and `+`/`-` across layouts. This round does not repeat that survey. It goes
  into who each shortcut and gesture breaks, how players and auditors actually handle 2.1.4, and
  what viewers can turn off.
- **Method:** W3C Understanding documents, the WCAG issue tracker and the WAI mailing lists; the
  source of Able Player, Plyr, video.js 10 and jellyfin-web (cloned to the cache and read); vendor
  help pages (Google TalkBack, JW Player, Firecore Infuse, NoMercy); WebKit, Mozilla and Drupal
  bug trackers; MDN's browser-compat data; forums and the press. Reddit could not be fetched,
  so its reports arrive second-hand.
- **Labels:** **[V]** read today in a standard, a vendor's page, a bug tracker or source code;
  **[P]** a press report, forum thread, or a search summary of a page not opened; **[I]**
  inferred.

## Summary

- **Single letters hurt four groups, in different ways.** Speech users with Dragon and older
  tools can fire a string of shortcuts from one dictated word. Screen-reader users in browse mode
  never reach the keys, because the screen reader takes the letters. Users with tremor, or anyone
  changing the volume, press digits by accident and lose their place. Users of non-Latin or
  AZERTY layouts find the keys missing or wrong. The 2026 evidence is that current built-in voice
  tools (macOS and iOS Voice Control, Android and Windows Voice Access) do **not** turn dictated
  words into key presses, so the speech risk is now mostly Dragon and explicit "press K"
  commands.
- **"Active only while the player has focus" is not a settled way to pass 2.1.4.** WCAG issue
  #1950 asks exactly that and is still open after five years; the working group's members lean
  towards "a player is not a single component". The way that passes without argument is an **off
  switch** (Able Player, JW Player), or a **modifier** (Able Player's Ctrl+Alt default).
  YouTube's embed switch `disablekb=1` shows the trap: it turns off the arrows and Space as well,
  which cuts off keyboard users.
- **Which key value is matched decides who the shortcuts work for.** Plyr, video.js 10 and Able
  Player match `event.key`, so they fail on Cyrillic or Greek layouts and on AZERTY's top-row digits,
  which need Shift there. Jellyfin matches `event.code`, so it works on Cyrillic but puts M and
  the letters in the wrong place on AZERTY and Dvorak. YouTube's use of `keyCode` is still broken
  on AZERTY (Mozilla bug 1911796, open). PeerTube merged a fix for non-Latin layouts in 2023.
- **Gestures break screen readers by design, not by accident.** Under VoiceOver and TalkBack
  one finger explores and a double tap activates. A page's own double tap, long press and swipes
  are not reachable without a pass-through gesture that few users know. Every gesture therefore needs
  a button that does the same thing (WCAG 2.5.1). The buttons are lost when the overlay hides
  itself before a switch or speech user can reach them.
- **Competitors ship gestures on, and the careful ones let you turn each one off.** VLC for
  Android turns volume, brightness, seek and zoom swipes on by default but ships **tap-and-hold
  to speed up off by default**. YouTube's hold for 2x cannot be turned off, and users install
  extensions and patches to stop it. YouTube's double-tap length can be changed but not
  turned off.
- **A mobile browser cannot do all of it.** It cannot set the volume on iOS, cannot set the
  screen brightness anywhere, and cannot put an element into full screen on an iPhone. It cannot
  stop the Android back swipe or the iOS edge swipe from leaving the page. Long press competes
  with the browser's own context menu and text selection. A double tap costs every single tap
  a delay.

## 1. Letter shortcuts on a computer

### 1.1 Who a single-letter shortcut hurts

**Speech input.**

- The W3C Mobile Accessibility Task Force's 2016 page that led to SC 2.1.4 gives the case: in
  Gmail, saying "Hey Kim" while focus is in the wrong place fires *y*, *k* and *m* as commands,
  and "a spoken word can become a barrage of single key commands if the cursor focus happens to
  be in the wrong place" [V]
  (https://www.w3.org/WAI/GL/mobile-a11y-tf/wiki/Proposed_Single_Key_Shortcut_Alternative).
  For a player, a dictated sentence with a *k*, *f* or *m* in it would toggle play, full screen
  or mute [I].
- A 2026 test page by Adam Page, run on macOS 26.3, iOS 26.3, Android 16 and Windows 11, found
  that dictating "r" or "watermelon" outside a text field **never** fired `keydown` in macOS
  Voice Control, iOS Voice Control, Android Voice Access or Windows Voice Access. The explicit
  command "press r key" did fire on macOS everywhere, on iOS only inside a text field, and on
  Windows [P] (https://cdpn.io/pen/debug/jEMPwvL/06f138a6e48b1f8bd3fc93b6fc341160). Dragon was not
  tested.
- So with today's built-in tools the "barrage" happens only when the user means to press a key,
  and the danger is concentrated in Dragon, which has a long record of typing dictation as
  keystrokes into whatever has focus [I]. No report was found of a Dragon user tripping a video
  player's letter keys.
- Kim Patch, a speech user on the WCAG working group, wrote on issue #1950 in 2023 that speech
  users need the choice to turn shortcuts off whether or not the player has focus [V]
  (https://github.com/w3c/wcag/issues/1950).

**Screen readers.**

- NVDA and JAWS, in browse mode, take letter keys for quick navigation (H for heading, B for
  button and so on). On the ARIA list in 2016 Jason White said browse mode "takes over most of
  the letter keys" [V] (https://lists.w3.org/Archives/Public/public-aria/2016Apr/0129.html). A
  page's K or F never arrives unless the user switches to focus mode (NVDA Insert+Space, JAWS
  Insert+Z), as AppleVis users describe for YouTube [P]
  (https://www.applevis.com/forum/windows/issue-youtube-website-nvda). Pages that publish
  shortcuts, such as the VA's video help page, tell users to do the same [P].
- The usual way round this is `role="application"`, which makes the screen reader pass keys
  through. On the same list, Matt King (then of Facebook, now of the ARIA working group) warned
  against encouraging authors to "use the application role to thwart users' ability to use any of
  these screen reader keys" [V]. Able Player, the player built for accessibility, does not put
  `role="application"` on the player [V] (source, `scripts/`).
- The Fluid project's video player found in 2012 that JAWS did not switch to application mode on
  its video while NVDA did, and closed the bug Won't Fix [P]
  (https://issues.fluidproject.org/browse/VP-152/).
- Arrow keys are the sharper conflict. Lucy Greco, a blind accessibility specialist, wrote in
  August 2024 that arrow keys "hijacked to seek" stop a screen-reader user exploring the player,
  and that the editor she was using had J/K/L shortcuts that were missing and undiscoverable [V]
  (https://lists.w3.org/Archives/Public/w3c-wai-ig/2024JulSep/0079.html). This bears on D407's
  arrows as much as on letters [I].
- No current data was found on whether screen readers announce `aria-keyshortcuts`.

**Sticky keys and tremor.**

- The Understanding document gives the general case: "Keyboard-only users who have dexterity
  challenges can also be prone to accidentally hitting keys" [V]
  (https://www.w3.org/WAI/WCAG22/Understanding/character-key-shortcuts.html). It does not mention
  sticky keys.
- With sticky keys on, a latched Shift or Ctrl changes the next key [I]. A player that ignores
  any modifier (Plyr bails if Shift, Ctrl, Alt or Meta is held) does nothing when the user
  meant K. A player that ignores modifiers only for some keys may act on a key the user meant as
  a browser chord. Neither was found reported.
- Digit keys are the commonest accident. A Chrome extension exists only to turn off YouTube's
  number keys. A review says the writer reached for the volume, hit a digit, and lost their
  place in a long video [P]
  (https://chromewebstore.google.com/detail/lajiknjoinemadijnpdnjjdmpmpigmge).

**Switch users and on-screen keyboards.** A switch user reaches a shortcut only through an
on-screen keyboard, which needs scanning to the letter. The visible button is the same number of
steps or fewer [I]. Letter keys give a switch user nothing, and the cost to them is the hidden
overlay (§1.6).

### 1.2 How WCAG 2.1.4 is met in practice

The criterion (Level A) passes if a single-character shortcut can be **turned off**, can be
**remapped** to include a non-printable key such as Ctrl or Alt, or is **active only when the
component has focus**. It defines a component as "a part of the content that is perceived by
users as a single control for a distinct function", and says this is "not tied to programming
techniques" [V] (Understanding 2.1.4). G217 is the technique for providing a way to turn
shortcuts off or remap them; F99 is the failure [V].

**Is the player one component?** Not settled.

- WCAG issue #1950, "Is a video player considered a single component...", was opened on
  2021-07-08 after Vimeo's page-wide shortcuts. It is still open, labelled "In progress", and
  assigned to Patrick Lauke [V] (https://github.com/w3c/wcag/issues/1950).
- Bartosz Skibinski said no: a shortcut counts only while the play button itself has focus.
  Lauke said that would make the shortcut pointless, and that a player should count as one
  component.
- Detlev Fischer said in 2021 that shortcuts on by default with a setting to turn them off pass.
  Andrew Kirkpatrick said in 2023 that a player is likely not a single control.
- In January 2026 Fischer said the current draft wording "will not fly".
- An auditor who reads the criterion strictly can fail a player whose keys work "while the
  player has focus", and one who reads it like Lauke will pass it. An off switch passes under
  both readings [I].

**What auditors and integrators accept, as seen in the record.**

- A 2019 WAI-IG thread asked whether the YouTube embed fails 2.1.4. Gerard Copinga concluded
  the only way to pass was `disablekb=1`, which also turns off the arrows and Space [V]
  (https://lists.w3.org/Archives/Public/w3c-wai-ig/2019JulSep/0035.html).
- Drupal issue #3228051 (2021–2024, still "Needs work") proposes `disablekb=1` for Drupal's
  YouTube embeds. Comments 12–14 found the price: with it, the subtitle, translation and quality
  menus could no longer be reached by keyboard. A contributor reported this to YouTube in August
  2024 [V] (https://www.drupal.org/node/3228051).
- An off switch that also turns off the keyboard's basic operation trades one failure (2.1.4)
  for a worse one (2.1.1 Keyboard) [I]. Only the **letter** keys should be switchable. Space,
  Enter, Tab and the arrows on a focused control stay.

**How players do it.**

| Player | Scope | Off switch | Remap / modifier | Source |
| --- | --- | --- | --- | --- |
| Able Player | whole page with one player; player focus with several | Preferences › "Disable Keyboard Shortcuts" | Ctrl+Alt by default (DHTML Style Guide); Alt, Ctrl, Shift chosen in Preferences | [V] source `event.js`, `preference.js`; README |
| JW Player | player focused | right-click menu › Keyboard Shortcuts › Disabled | none | [V] https://docs.jwplayer.com/players/docs/players-web-player-accessibility |
| Plyr | player focused (`keyboard.focused: true`); page-wide opt-in (`global`) | integrator only | none | [V] source `listeners.js` |
| video.js 10 `<media-hotkey>` | player container focused; `target="document"` opt-in | integrator only | none; exposes `aria-keyshortcuts` | [V] source `hotkey.ts`, `hotkey.mdx` |
| Kaltura v7 | "only if the respective player button/plugin is the focused element" | not found | not found | [P] search summary; the page would not load |
| YouTube embed | player focused, and page-wide on youtube.com | `disablekb=1`, integrator only; also kills arrows and Space | none | [V] Drupal #3228051 |
| Vimeo | page-wide (the prompt for #1950) | none found on its accessibility page | none | [V] |
| Blue Billywig | player | a `\` key toggles shortcuts; `?` shows help | none | [P] |

video.js 10's documentation says: "Hotkeys supplement operable controls; they should never be the
only way" [V]. It ignores unmodified keys while focus is in an editable field and lets Space and
Enter press a focused control [V]. Able Player stopped using Space for play "as of 4.2.10" because
"users use that to scroll the page" [V] (source comment). No player was found that turns
letter shortcuts **off** by default for everyone; Able Player gets the same effect by requiring
Ctrl+Alt.

**A Ctrl+Alt default is not free.** On Windows, AltGr is reported as Ctrl+Alt, so a Polish or
German user typing an AltGr letter may trigger a Ctrl+Alt shortcut [I]. §10 of the controls round
covers AltGr. D407 §8 (now §10) leaves Ctrl, ⌘, Alt and AltGr to the browser, which rules out Able
Player's route.

### 1.3 Layouts, input methods and extensions

**Which value the code reads.**

| Read | Behaviour | Who it fails | Who uses it |
| --- | --- | --- | --- |
| `event.key` | the character typed | Cyrillic, Greek, Hebrew, Arabic and other non-Latin layouts (K gives "л" on Russian); AZERTY digits, which need Shift, so a player that rejects Shift never sees them | Plyr, video.js 10, Able Player [V] |
| `event.code` | the physical key, named after US QWERTY | AZERTY (the key labelled M is `Semicolon`; A and Q, Z and W swap), Dvorak, Colemak: the letter on the key is not the one that acts | jellyfin-web, via `KeyNames[keyCode] \|\| event.code`, with digit and numpad keys mapped to seek-to-percent and no Shift check [V] |
| `event.keyCode` | legacy, layout and browser dependent | AZERTY punctuation (comma gives 59 where YouTube expects 188) | YouTube [V] |

- Mozilla bug 1911796, "youtube broken shortcuts with French keyboard layout" (2024, NEW): Henri
  Sivonen wrote "YouTube is rather broken regardless of browser with AZERTY" [V]
  (https://bugzilla.mozilla.org/show_bug.cgi?id=1911796). A Firefox add-on exists to fix YouTube's
  AZERTY shortcuts [P].
- PeerTube merged "Non latin keyboard layout support player shortcut" on 2023-03-08 [V]
  (https://github.com/Chocobozzz/PeerTube/pull/5684).
- A common hybrid: use `key` when it is a Latin letter a–z, and fall back to `code` when it is
  not. That way K works where K is printed on Latin layouts, and on the key where K sits for
  Cyrillic [P] (faiwer.ru). Discourse users on non-Latin layouts say they must switch layout to
  use its shortcuts. In 2019 one wrote that Gmail and YouTube work without switching [P]
  (meta.discourse.org topic 57149).
- **Digits on AZERTY** sit under shifted symbols (& é " ' ( - è _ ç à), so digit-to-percent seeking
  needs Shift there. A player that reads `key` and rejects modifiers never sees them. One that
  reads `code` sees them but also fires them when the user meant "é" [I].
- **IME.** During composition, browsers send `keyCode` 229 and `isComposing` true. A handler
  that ignores both does not act on a Japanese or Chinese user's keystrokes [I]. The controls
  round's §10 covers this.

**Browser and extension conflicts.**

- Firefox's Quick Find opens on `/`, and links-only Quick Find on `'`. Users report both
  opening while they type [P] (support.mozilla.org question 835021). A player should not use
  either [I].
- Keyboard-navigation extensions such as Vimium bind most unmodified letters. Their users
  exclude video sites or press a pass-through key [P]. This is a choice the user makes, and an
  extension sees the key before the page does [I].
- The digit-key and hold-for-2x blocking extensions (§1.1, §2.2) show that some users want to
  turn off a default the site will not let them turn off [P].

### 1.4 What letter keys are worth

- K, J/L, F, M and C are the YouTube set and have spread to Plex, Jellyfin, Plyr defaults and
  video.js 10 [V] (controls round §9). A viewer who knows them from YouTube will try them, and
  F and M are the most often guessed [I].
- They add little that D407 does not already give. Space and Enter press a focused button.
  Left and Right seek. Up and Down show the overlay, and `+`/`-` set the volume. What letters
  add is single-key full screen (F), mute (M) and captions (C) without tabbing to a button [I].
- No player publishes how many viewers use its shortcuts. Studies of shortcuts in desktop
  software find that most users use few. They also find that hints shown at the moment of use,
  such as a tooltip naming the key, multiply how often shortcuts are learned [P] (TU/e and
  Saarland University studies, via search summaries). This fits the Plex and YouTube practice of
  putting the key in the button's tooltip [I].

### 1.5 Help overlays and discoverability

- YouTube, Plex and Blue Billywig show a list on `?` (Shift+/) [P] (controls round §9 for YouTube
  and Plex). The `?` character's key differs by layout. On AZERTY it is Shift+comma, so a check
  on `key === "?"` works and a check on `code` does not [I].
- `aria-keyshortcuts` on each button lets a screen reader announce the key, where supported.
  video.js 10 sets it [V]. Support was not checked (§4).
- A tooltip naming the key ("Full screen (F)") teaches without a separate overlay. Native
  tooltips do not appear on keyboard focus, so a custom one does that job [I].
- If the letters can be turned off, the help overlay and the setting can be one place: a list
  of the keys with a switch at the top [I].

### 1.6 The auto-hiding overlay, which matters more than the letters

- Hidden controls are invisible to speech commands ("click Pause" finds nothing). Patrick Lauke
  wrote in 2016 that controls should be at least faintly visible while paused, should not fade
  too quickly, and should never fade while focus is on any control [V]
  (https://lists.w3.org/Archives/Public/w3c-wai-ig/2016OctDec/0144.html).
- WebKit bug 117855 (2013) reported that native controls vanished from VoiceOver as soon as play
  began, so a VoiceOver user could not pause. It was closed WORKSFORME in 2014 after shadow-DOM
  accessibility work [V] (https://bugs.webkit.org/show_bug.cgi?id=117855). Firefox bug 1622995
  is similar [P].
- VLC for iOS issues 1872 and 1874 ask for longer control time-outs for Switch Control: the
  controls vanish before a scan reaches them, and one asks for about 20 s [P].
- YouTube's "Accessibility player" keeps the controls visible [P].
- D407's 3 s hide is shorter than any of these asks. Options: do not hide while focus is in the
  overlay, do not hide while paused, and a "keep controls visible" setting. These may help
  speech and switch users more than any letter rule [I].

## 2. Touch gestures on phones and tablets

### 2.1 How assistive technology meets a gesture

- **VoiceOver (iOS).** One finger explores and speaks, and a double tap activates the item under
  focus. A double tap on the video is the activation of whatever VoiceOver focused, not two taps
  the page can count [I]. WebKit bug 240222 (2022, fixed) recorded a double tap on a YouTube video
  in iOS Safari pausing it instead of seeking [V]. WebKit bug 222627 (2021, open) records that
  VoiceOver's double tap fires no pointer events inside an iframe, and that macOS VoiceOver fires
  none on click either, so a player should act on `click`, not on `pointerdown` [V]
  (https://bugs.webkit.org/show_bug.cgi?id=222627). VoiceOver's direct-touch mode passes touches to
  an element marked `allowsDirectInteraction` in a native app. A web page cannot set it [I]. The
  AppleVis page on Disney+'s direct-touch area returned 403.
- **TalkBack (Android).** Double tap activates. A two-finger double tap toggles media play and
  pause (TalkBack 9.1 and later), which gives a TalkBack user play and pause without the page
  doing anything. Sliders move up and down. "Most one-finger gestures become two-finger gestures".
  A four-finger double tap and hold passes the next gesture through: "the system interprets the
  next gesture like TalkBack is off" [V]
  (https://support.google.com/accessibility/android/answer/6151827). So a swipe to seek is
  reachable only through pass-through, which few users know [I].
- **Switch Control (iOS) and Switch Access (Android).** They scan to elements and activate them.
  A gesture with no element, such as a double tap on the left third or a vertical swipe on the
  right edge, can be performed only through a recorded custom gesture [I]. The buttons are the
  way in, and they must stay on screen long enough (§1.6).
- **Voice Access (Android) and Voice Control (iOS).** They act on labelled elements ("tap
  Pause"). They can perform "swipe up" or "double tap" on the screen, but not at a chosen
  zone without a grid overlay [I]. Hidden controls cannot be named.
- **WCAG 2.5.1 Pointer Gestures (A)** requires a single-pointer, non-path way to do anything a
  path-based or multipoint gesture does. **2.5.7 Dragging Movements (AA, 2.2)** requires a
  non-drag way to do anything a drag does [V] (controls round §9). Every gesture below therefore
  duplicates a button, and the button is what assistive-technology users actually use [I].

### 2.2 Gesture by gesture

| Gesture | What players do | Failure reports | Under assistive tech |
| --- | --- | --- | --- |
| **Single tap** | show or hide the overlay; some also pause | taps meant to show controls pause instead when tap means pause [I] | VoiceOver and TalkBack: a double tap activates the focused element; works if the video surface is one labelled button ("Show controls") [I] |
| **Double tap left/right** | seek ±10 s (YouTube 5–60 s, settable but not removable [P]; Infuse ±10 s, taps add up [V]; VLC 10 s [V]) | every single tap waits for a possible second tap. NoMercy's player docs say the 300 ms threshold "costs every single tap the same delay" [V] (https://docs.nomercy.tv/nomercy-video-player/plugins-adapters/touch-zones). video.js 10 cut its window from 300 to 200 ms [V] (https://github.com/videojs/v10/pull/1328) | not reachable as a zone gesture. A screen-reader double tap is an activation. WebKit 240222 shows the clash [V] |
| **Double tap centre** | play/pause (MX Player [P]); full screen in some players | competes with the browser's double-tap zoom unless `touch-action: manipulation` [V] (controls round §9) | as above |
| **Long press (tap and hold)** | 2x while held (YouTube); context menu in VLC's off-by-default "fastplay" | YouTube's hold for 2x is a 2023 experiment made standard. On the web, releasing pauses [P]. The extension "Disable YouTube 2x Speed" has some 500–750 users and was updated in November 2025 [P]. ReVanced users report that pulling down the notification shade in full screen triggers 2x [P]. Brave on Android shows its own context menu on a long press, 2025 [P] | iOS uses long press for the callout menu and text selection (`-webkit-touch-callout: none` is iOS-only [V]); TalkBack maps long press to a double tap and hold on the focused element [I] |
| **Horizontal drag** | scrub (Infuse, VLC "swipe seek" on [V]) | a drag that starts at the screen edge becomes Android's back gesture or the iOS edge swipe (WebKit 240183, controls round §9). iOS 17 removed drag-anywhere scrubbing from its own player [P] | a path gesture: needs the seek bar or buttons (2.5.1, 2.5.7) |
| **Vertical swipe, right side** | volume (VLC on [V], Infuse [V], Samsung Internet's Video Assistant [P]) | **impossible on iOS** from a page: volume is read-only [V] (controls round §9). On Android it sets the element volume, not the system volume, so it fights the hardware keys [I] | a path gesture |
| **Vertical swipe, left side** | brightness (VLC, Infuse, Samsung Video Assistant [P]) | **impossible in any browser**: there is no brightness API. A page can only dim with an overlay, which is not brightness [I] | a path gesture |
| **Swipe down** | close the player or leave full screen | on iPad, swiping down leaves element full screen, which cannot be stopped [V] (MDN compat note); collides with the notification shade at the top [I] | a path gesture |
| **Pinch** | zoom to fill or crop (VLC "scale gesture" on [V]; MX Player optional [P]) | conflicts with the page's pinch zoom unless `touch-action` blocks it, and blocking it removes zoom for low-vision users [I] | VoiceOver and TalkBack users zoom with the system magnifier (three-finger gestures), which is unaffected [I] |
| **Two-finger tap** | pause (Infuse [V]) | none found | TalkBack's two-finger double tap already toggles media [V] |

### 2.3 What a mobile browser can reliably do

- **`touch-action`** works on every current engine: `manipulation`, `none`, `pan-x` and `pan-y`
  everywhere, and `pan-left`, `pan-right`, `pan-up` and `pan-down` only in Chromium [V] (MDN
  browser-compat data, cached). `manipulation` removes the double-tap-to-zoom wait without
  removing pinch zoom [V].
- **`overscroll-behavior: contain`** works in Safari 16 and later, Chrome and Firefox. Safari
  ignores it on a container that does not scroll [V] (MDN compat data). It stops pull to refresh
  inside a player, not the system's edge gestures [I].
- **Edge gestures cannot be claimed.** The Pointer Events group discussed mapping `touch-action`
  to Android's gesture-exclusion rectangles in 2019. Apple's representative said "application-level
  gestures should always be respected", and the issue was closed in 2020 as the browser's choice
  [V] (https://github.com/w3c/pointerevents/issues/295). Whether Chrome ever did this was not found.
  A player should keep its swipes away from the left and right edges [I].
- **Full screen.** `Element.requestFullscreen` works on iPad (Safari 16.4 and later) with an exit
  button that cannot be hidden, but **not on iPhone**, where only the video's native full screen
  (`webkitEnterFullscreen`) exists, and the native player's own controls and gestures replace
  the page's [V] (MDN compat data). Element full screen on iPhone appeared behind a flag in the
  Safari 17.2 beta and was withdrawn. Forum reports in 2025 say it is still absent [P]. **So no
  page gesture runs in iPhone full screen.**
- **Volume and brightness.** The iOS volume cannot be set, and no browser exposes brightness
  [V] (controls round §9).
- **Long press** fights the context menu (`contextmenu` can be cancelled on Android Chrome but
  Brave shows its own [P]) and iOS text selection and callout [I].
- **Timing.** Telling a single tap from a double tap needs a window of 200–300 ms before a
  single tap acts [V] (NoMercy; video.js 10).
- **Pointer events and assistive tech.** Act on `click` for anything a screen-reader user must
  reach, because VoiceOver may fire no pointer events (WebKit 222627) [V].

### 2.4 Competitors' gesture settings and defaults

| App | Gestures on by default | Can each be turned off? | Source |
| --- | --- | --- | --- |
| VLC for Android | volume swipe, swipe seek, pinch scale, double tap to play/pause; double-tap seek 10 s | yes, each has a switch. In 2022 every gesture but pinch could be turned off, and a "Two finger zoom" switch came later. **Tap-and-hold "fastplay" is off by default**, at 2x | [V] `preferences_video_controls.xml` (https://mailman.videolan.org/pipermail/android/2024-July/022583.html); brightness and double-tap-seek defaults not in that excerpt |
| YouTube (app) | double tap ±10 s, hold for 2x, swipe up/down for full screen, pinch to fill | double-tap length 5–60 s ("Skip forward and back" on iOS); no off switch for double tap or hold for 2x | [P] |
| Infuse | two-finger tap pause, double tap sides ±10 s (taps add up), drag to seek, vertical swipes for brightness and volume | no settings listed | [V] https://support.firecore.com/hc/en-us/articles/215090987 |
| MX Player | double tap centre pause, pinch zoom and pan, two-finger vertical swipe for speed | zoom and pan are optional; defaults unknown | [P] |
| Plezy (Plex client) 2.18.0 | brightness, volume, pinch | separate switches for each | [P] |
| Samsung Internet | Video Assistant: edge swipes for brightness and volume in full screen | Settings › Useful features › Video Assistant; reportedly not on youtube.com | [P] |
| Jellyfin for Android (official) | no swipe gestures | n/a; a November 2025 forum request asks for them | [P] |
| Plex, Netflix, Prime, Disney+ | double tap to seek in some (controls round §9) | not found | not checked this round |

The pattern [I]: players that offer many gestures give each one its own switch. Hold for 2x is
the gesture most often kept off or fought, and the only one VLC ships off. Brightness and volume
swipes are native-app features that a browser cannot reproduce.

## 3. Options for the owner

Neither option is recommended. Both keep D407 §8 (now §10) as it stands (arrows, Space, Enter, `+`/`-`,
media keys) and add only what this round is about.

### Option A: the minimal safe set

- **No letter shortcuts.** The keyboard does what the remote does (D407 §8 (now §10)).
- **One gesture: a tap on the video shows or hides the overlay.** No double tap, long press,
  swipe or pinch of the page's own. On iPhone full screen the native player's gestures apply
  anyway.
- The video surface is one labelled control ("Show controls"), so VoiceOver, TalkBack, Switch
  Control and Voice Access reach it.
- The overlay does not hide while focus is inside it or while paused (§1.6).
- **Gains:** nothing to fail 2.1.4, 2.5.1 or 2.5.7; no layout or IME bugs; no delayed taps; one
  thing to test per device.
- **Costs:** YouTube users' K, F and M do nothing, and they will try them [I]. Phone users must
  open the overlay and press a button to seek, two taps where YouTube takes a double tap.

### Option B: a fuller set, with settings

- **Letters**: K (play/pause), J/L (seek), F (full screen), M (mute), C (captions), perhaps `?`
  for a help list. Digits are left out because they cause the most accidents (§1.1) and fail
  on AZERTY.
- Matched by **Latin `key` first, then `code`**, so non-Latin layouts work. Never while a text
  field has focus, and never during IME composition.
- Active **while focus is within the player**, *and* with a **setting to turn letter keys off**
  that leaves arrows, Space and Enter working. The off switch is what passes 2.1.4 under either
  reading of issue #1950. It belongs in the help list and the settings page. Each button names
  its key in a tooltip and in `aria-keyshortcuts`.
- **Gestures**: double tap left or right third to seek ±10 s, taps adding up, with a 200 ms
  window. Possibly a long press for 2x, **off by default**, as VLC ships it. No volume or
  brightness swipes, since iOS and browsers cannot honour them. No edge swipes, since the system
  owns the edges.
- **A setting per gesture**, or one "touch gestures" switch. Turning them off also removes the
  double-tap delay from single taps.
- **Gains:** what YouTube users expect; quicker seeking on a phone.
- **Costs:** the double-tap window slows every tap that shows the overlay. Two settings to build
  and keep, with their own UI and tests. Layout handling to test (QWERTY, AZERTY, Cyrillic, IME).
  A help overlay to design. None of the gestures reaches VoiceOver or TalkBack users, so the
  buttons must stand on their own anyway.

### Variants between the two

- **A with letters off by default** and a setting to turn them on: passes 2.1.4 plainly, and
  costs one setting. Few viewers will find the setting [I].
- **B without gestures**: the letters and their setting, but only the overlay tap on touch.
  Avoids the tap delay.
- **B with letters behind a modifier** (Able Player's route): ruled out by D407 §8 (now §10), which leaves
  Ctrl, Alt and AltGr to the browser, and clashes with AltGr on Windows.

### Questions the owner may want to answer

1. Is any page-level gesture beyond "tap shows the overlay" wanted at all?
2. If letters are added, is "on, with an off switch" acceptable, or should they be off until
   a viewer turns them on?
3. Does the 3 s hide stay as D407 says, or does it pause while focus is in the overlay or the
   video is paused, with a "keep controls visible" setting?
4. Should D407's Left/Right seek apply only while focus is on the video or the seek bar, so that
   arrows keep moving focus for screen-reader users (Greco's complaint, §1.1)?

## 4. Not verified

- **Dragon** with a video player's letter keys: no report found either way, and the 2026 voice
  test did not include Dragon.
- **Screen-reader support for `aria-keyshortcuts`** in NVDA, JAWS, VoiceOver and TalkBack today.
- **Kaltura's** v7 keyboard page (would not load) and its VPAT; **Brightcove's** shortcuts beyond
  Space and Enter; the **BBC's** media player and GEL guidance (not found).
- Whether **Chrome on Android** ever mapped `touch-action` to gesture-exclusion rectangles.
- Whether **iOS 18 or 26** brought back drag-anywhere scrubbing in the native player, or element
  full screen on iPhone.
- **Prime Video, Disney+, Netflix and Plex** gesture settings on phones.
- **MX Player's** and **VLC's** brightness and double-tap-seek defaults.
- How many viewers **use** player shortcuts or gestures: no vendor publishes it.
- Firefox bug 1622995 and VLC iOS issues 1872 and 1874 were read only through search summaries.
- **Reddit** threads on hold for 2x and on YouTube's digit keys were seen only through search
  summaries and extension reviews.
