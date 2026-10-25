> **ABOUTME:** The device tests the owner runs before casting, extending and controller input are
> chosen: which device, which lab page, what to do, what to write down, and which result decides
> what. Grouped by where the owner will be: at home, on the next Berlin visit, and when a
> Chromecast arrives.

# TV and device tests

- **Status:** PLANNED — 2026-10-25. The lab pages are built (web client `/lab`, the cast receiver
  under `/cast/`); the runs are the owner's.
- **Depends on:** —
- **Why:** the owner, 2026-10-25: *test first, then choose the casting and controller approach*;
  "extend to the TV" in the beta if possible, even primitive. The evidence it weighs:
  [casting-2026-10-25.md](../research/casting-2026-10-25.md) §5–§6,
  [casting-review-2026-10-25.md](../research/casting-review-2026-10-25.md),
  [controller-input-2026-10-25.md](../research/controller-input-2026-10-25.md) §1 and §6.
- **Hardware:** Fire TV sticks at home (models unknown, so Fire OS or Vega OS unknown), a Windows
  laptop, an iPhone and an Android phone; the TCL TV in Berlin (Google TV, TV Bro, Chromecast built
  in, most likely); a used Chromecast with Google TV, being sought.
- **Device checks:** each test that settles something is a `VQ` item in
  [verification-queue.md](../../verification-queue.md) (VQ55–VQ59), ticked with what was seen.

## 0. The lab pages, and reading their results

Every page is under **`/lab`** on the core, so a TV's remote types five characters after the
address it already has. The menu is a vertical list of large links: the browser's own D-pad
navigation, Tab or a pointer reaches each.

| Page | Address | What it does |
| --- | --- | --- |
| Menu | `/lab` | The probes, one big link each |
| Device report | `/lab/device` | User agent and the Fire OS/Vega hint, screen and page size, devicePixelRatio; MSE, ManagedMediaSource and `canPlayType` for H.264 (five profiles and levels), HEVC, AV1, VP9, AAC-LC, HE-AAC (v1, v2), AC-3, E-AC-3, Opus and FLAC in fMP4; native HLS; then, after **Run**, the audio context's base and output latency, the number of audio outputs, and a WebRTC call with itself (data-channel round trips, a tone's received level). Lists which of setSinkId, Screen Wake Lock, getUserMedia, RTCPeerConnection, getScreenDetails, the Gamepad API, Media Session, Fullscreen, the Presentation API and Remote Playback the browser has |
| Remote and keys | `/lab/input` | The last 30 key events (key, code, keyCode, repeat, ms since the one before; Backs marked), Media Session actions, Back as history steps, gamepads, and a header row over a 6 × 4 grid of tiles, moved across by the browser's own navigation or by our geometric search, with each move's latency from key to the frame after focus moved (median and worst, against D190's 250 ms) |
| Phone as remote | `/lab/remote` | Screen Wake Lock held and asked again when shown; the microphone through a call with the page itself, played back through an `<audio>` element (not Web Audio), its level sampled each second; a 1 s ticker whose gaps over 2 s mean the page was stopped; visibility and page lifecycle. Sends its log every 30 s |
| Extend to the TV | `/lab/extend` | Where the browser can place a window on another screen (Window Management API, Chromium), opens a test film there (a counter, a sweep, a flash and a beep each second, full screen at its first press) and stays as its remote (play/pause, ±10 s); elsewhere, says so and gives the manual recipe. Then two tones on two audio outputs at once (setSinkId) |
| Cast | `/lab/cast` | Chromium only, over HTTPS. Google's Default Media Receiver handed an address, or our own receiver by its app id, spoken to on `urn:x-cast:org.orpheum.lab`. Must be opened by its own address (typed, or from the menu, which loads it afresh): only that page may load Google's Cast SDK |
| Test film | `/lab/extend/player` | Opened by the extend page; needs no sign-in |

**Results.** Each page shows what it found in large type and has **Send to server**, which posts
the result to the core (and each page sends by itself when its test ends). The page then shows
**Sent as `K3J9QW2A`**: write that name down beside what you saw. In the core's log the result is
one line at info, *a lab page sent its results*, with `labResultId`, `probe`, `userId`,
`deviceId`, `userAgent` and the whole `result` in it. On staging, `grep K3J9QW2A` in the core's
log finds it. A page needs sign-in like the rest of the app; a TV signs in with a device code from the
sign-in page (`/account`'s *Activate a device* on the phone).

**Fire OS or Vega OS.** On the stick: **Settings → My Fire TV → About**. A version starting **5,
6, 7 or 8** is Fire OS (Android); one starting **1 or 2** is Vega OS (Linux)
([review](../research/casting-review-2026-10-25.md) §5 Q8). Write down the model name shown there,
and compare with `/lab/device`'s *Fire TV* line, which reads the user agent's `AFT…` model code
(only a hint).

## 1. At home

### H1. Device report on each Fire TV stick

- **Device:** every Fire TV stick, in **Silk**.
- **Open:** the core's address, sign in with a device code, then `/lab/device`.
- **Steps:** note Fire OS or Vega OS and the model (§0). Wait for the report; press **Run the sound
  and WebRTC tests**; note the sent name.
- **Record:** OS and model; *Fire TV* line; MSE yes/no; H.264 High 4.0 and 4.2, HEVC Main 10,
  AAC-LC, E-AC-3 under MSE; native HLS; sound delay; WebRTC line.
- **Decides:**
  - MSE and H.264 High under MSE → our hls.js player can run there, so the `orpheum.cc` route
    (casting §5.2 step 4) may target that stick. **Vega OS without MSE** → its Silk cannot play our
    player; Vega sticks are unsupported until a native app (review §5 Q8).
  - WebRTC connected with the tone heard → the stick can take a playback-only device's call path
    of the split-device sketch; failed → it plays by plain HTTP at home only.
  - The output latency is the figure to compare with the TCL's 320 ms (§2's sound test).
- **VQ55.**

### H2. Input probe in Silk

- **Device:** each Fire TV stick, Silk, with its own remote.
- **Open:** `/lab/input`.
- **Steps:**
  1. Press each remote key once: the D-pad, OK, Back, Home, Menu (≡), play/pause, rewind, fast
     forward. Note which show in the key log, and with what `key` and `keyCode`.
  2. Move across the grid with the D-pad in **Browser's own navigation**, then in **Our geometric
     search**, about 20 moves each.
  3. Press **Back** three times.
  4. Press **Play a quiet tone**, then the media keys again.
  5. Press **Send to server**.
- **Record:** which keys reach the page and their `key`/`keyCode`; whether Silk's own cursor
  takes the D-pad (no arrows in the log) or the arrows reach the page; whether either mode moves
  focus; median and worst latency per mode; Back's history steps and Back keys; Media Session
  actions fired.
- **Decides:**
  - Arrows reaching the page, and our search moving focus within 250 ms → the web client's TV mode
    (controller research §6.2 step 1) works on Fire TV's browser.
  - Silk's cursor taking the D-pad → Fire TV gets the web client only as a pointer interface; the
    controller work targets packaged apps first.
  - The keys that never reach the page (Back, media keys) are what a native shell would add
    (controller research §5.3). Add each confirmed one to
    [idiosyncrasies.md](../../idiosyncrasies.md).
- **VQ59** (with §2's TV Bro run).

### H3. Miracast extend from a Windows laptop to a Fire TV

- **Devices:** the Windows laptop (Chrome or Edge) and a Fire TV stick.
- **Steps:**
  1. On the Fire TV: hold **Home**, choose **Mirroring** (or Settings → Display & Sounds → Enable
     Display Mirroring).
  2. On the laptop: **Win+K**, pick the Fire TV; then **Win+P** → **Extend**. Note whether Extend
     is offered (casting §7: Miracast *extend* to a Fire TV was not confirmed).
  3. Open `/lab/extend` on the laptop; press **Find screens** (allow window management), then
     **Open the test film on** the TV's screen. Where no screens are listed, use **Open the test
     film in a new window** and drag it across.
  4. On the TV, press the test film once (full screen, sound). Drive it from the laptop: play,
     pause, ±10 s. Watch for ten minutes.
  5. Film the TV and listen: the flash and the beep should coincide.
  6. **Send to server.**
- **Record:** Extend offered or only Duplicate; screens listed; the window opened on the TV and
  went full screen; remote commands answered (the laptop shows the player's position); delay and
  softness of picture; whether sound went to the TV by itself; flash-to-beep offset; any drops.
- **Decides:** whether the beta's "extend to the TV" (casting §6 Q1, the owner's *in the beta if
  possible*) includes Fire TV by Miracast, and whether the helper page (placing the window for
  the tester) is worth building beyond the recipe.

### H4. Phone as remote: endurance

- **Devices:** the iPhone (Safari) and an Android phone (Chrome). Earphones in, to keep the call's
  playback from feeding back.
- **Open:** `/lab/remote`; press **Start**, allow the microphone.
- **Runs** (the review's §2 table), each started fresh:
  1. iPhone, **Safari tab**, screen locked, 30 min.
  2. iPhone, Safari tab: switch to another app for 1 min and back, three times; take or make a
     phone call mid-run.
  3. iPhone, **home-screen app** (Share → Add to Home Screen, open it from there), screen locked,
     30 min.
  4. iPhone, Safari tab, screen on (the wake lock held), 2 h, on battery: note the battery before
     and after.
  5. Android, Chrome, screen locked, 2 h (a Samsung or Xiaomi phone if one is to hand).
- **Record** per run: the sent names (one every 30 s; the last says how far it got); *Sound
  arriving* and its share; *Page stopped* count and longest; wake lock held/let go; the event log's
  `microphone mute`/`ended`, `hidden`/`visible`, `freeze`; battery drop.
- **Decides:**
  - Tab keeps sound flowing locked and the home-screen app does not → the remote page's help tells
    iPhone users to use a Safari tab (review §2), and voice plays through `<audio>`.
  - Both fail → the first concrete reason for a thin native iOS shell (review §2), ahead of any TV
    app.
  - Wake lock not re-acquired after an app switch → WebKit bug 254545 still stands; the remote page
    keeps the screen on otherwise or tells the person.
- **VQ56.**

### H5. Voice to earbuds, film to the TV (setSinkId)

- **Devices:** the laptop with Bluetooth earbuds paired, and the TV as a second screen (or the
  laptop's own speakers as the second output).
- **Open:** `/lab/extend`, **Two sound outputs at once**.
- **Steps:** **List sound outputs** (allow the microphone, which lets their names show); choose
  the earbuds for tone A and the TV or speakers for tone B; **Play both**; listen to each; **Stop
  both**; **Send to server**. On a Mac, repeat in Safari (18.4+).
- **Record:** outputs listed; each tone heard on its own output, or the error shown; Safari's
  result separately (review §5 Q7: Safari 18.4 was reported unreliable).
- **Decides:** casting §6 Q7 — whether voice can go to earbuds while the film plays on the TV, and
  on which browsers, before *Voice through: [device]* is offered.
- **VQ57** (with H3).

## 2. On the next Berlin visit (the TCL TV)

### B1. Controller step 0: today's client in TV Bro's direct mode

As [controller-input-2026-10-25.md](../research/controller-input-2026-10-25.md) §6.2 step 0.

- **Device:** the TCL, TV Bro, its remote; a gamepad if one is to hand.
- **Steps:** in TV Bro, open the cursor menu and choose **direct navigation**. Try Home, a poster
  grid, a title, a series, search and Play with the D-pad. Press Back once and count the presses
  it takes to get back into direct mode. Press the four media keys while a film plays.
- **Record:** what WebView's own navigation gets right and wrong; presses to re-enter direct mode;
  what each media key did to our player; the gamepad's behaviour.
- **Decides:** whether TV Bro is a usable test bench for the TV mode (controller §5.4); the
  confirmed facts go into [idiosyncrasies.md](../../idiosyncrasies.md).

### B2. Input probe in TV Bro

- **Open:** `/lab/input`, in **cursor mode** first, then in **direct mode**.
- **Steps and record:** as H2. In cursor mode expect no arrows at all (controller §1.1).
- **Decides:** **the D190 latency gate** — our geometric search's worst key-to-focus at or under
  250 ms in direct mode passes controller §6.2 step 2's line on the TCL; over it, the engine or the
  page's weight needs work before step 1. Whether WebView's own navigation (browser mode) already
  moves between the tiles says how much step 1 must do there.
- **VQ59.**

### B3. Device report on the TCL

- **Open:** `/lab/device` in TV Bro (and in Chrome, if the TV has it). **Run** the tests.
- **Record and decide:** as H1; the output latency beside the 2026-10-24 figures (Web Audio base
  96 + output 328 ms). It is the same report the Chromecast built into the TV gives in C1, from the
  other side.

### B4. Cast through the TV's built-in Chromecast

- **Devices:** the laptop or an Android phone with Chrome, on the TV's Wi-Fi; the TCL's built-in
  Chromecast (Google TV's *Chromecast built-in*).
- **Steps:** C1's Default Media Receiver run, then, if the TV's serial is registered (§3's setup),
  C2's custom receiver run, on the TV in place of a Chromecast.
- **Decides:** whether a Google TV set's own Cast receiver runs our receiver as a dongle does, so
  a household with such a TV needs no Chromecast.

### B5. The sound test still owed

From the 2026-10-24 report (the TV's sound about 0.5 s late in TV Bro).

- **Open:** staging's library 4 *Sync tests*: **Sync Fig (2026)** and **Sync Elder**, in TV Bro.
- **Steps:** set the sound offset to **0** (This device) and **levelling off** (since an earlier commit a
  playback with levelling off takes the unlevelled sound and no Web Audio). Play each for a minute
  from the start and after a seek; film the screen with sound. Then set the offset to **−500 ms**
  and repeat.
- **Record:** how far the sound lags the picture at 0 and at −500, from the film; whether a seek
  changes it.
- **Decides:** at offset 0 in sync (or nearly) → dropping Web Audio was the fix and the offset
  stays a fine trim; still about 0.5 s late → the TV's AAC path itself lags, and the negative offset
  (ladder only, direct play cannot go earlier) or a native player (tv-apps research step 0b) is the
  answer.

## 3. When a Chromecast arrives

The spike of [casting-2026-10-25.md](../research/casting-2026-10-25.md) §5.2 step 2, with the
review's §4 additions. A Chromecast with Google TV is the target; a 3rd-generation Chromecast, if
one turns up, is the worst case.

### Setup only the owner can do (once, for the custom receiver)

The Default Media Receiver run (C1) needs none of this.

1. **Register as a Cast developer** at the Google Cast SDK Developer Console
   (`https://cast.google.com/publish`), with the Google account to own the receiver; Google charges
   a one-time registration fee (US$5 when last documented).
2. **Add a Custom Receiver application.** Its **receiver URL** is the core's `/cast/`, at the
   core's **HTTPS** name, which the Chromecast must itself resolve and reach: at home, the core's
   LAN name (an HTTPS name in public DNS pointing at its private address,
   [casting research](../research/casting-2026-10-25.md) §3.1), for example
   `https://<core's LAN name>/cast/`. A plain-HTTP or self-signed address does not load. Note the
   **application ID** the console gives.
3. **Register the device for development.** An unpublished receiver runs only on devices whose
   **serial number** is registered in the console: the serial is on the device's box or label and in
   the Google Home app's device settings. After registering, wait about 15 minutes and restart the
   Chromecast (Google's documented wait; the console says when it is ready).
4. **Debugging (optional):** with the device registered, Chrome's `chrome://inspect` on a computer
   on the same network shows the receiver page's console (Google's Web Receiver debugging page;
   unverified on a Chromecast with Google TV).
5. Open `/lab/cast` in Chrome **at the same HTTPS name** as the receiver URL (the receiver plays
   paths on its own origin, and the Cast SDK needs HTTPS), type the application ID once (kept in
   that browser).

### C1. Default Media Receiver

- **Open:** `/lab/cast` in desktop Chrome at the core's HTTPS name.
- **Steps:**
  1. Choose a film, **Start a playback of it**; the address of its ladder fills in.
  2. **Cast it to Google's receiver**; pick the Chromecast.
  3. Then replace the address with a public fMP4 HLS stream (for example Apple's
     `https://devstreaming-cdn.apple.com/videos/streaming/examples/img_bipbop_adv_example_fmp4/master.m3u8`)
     and cast again.
  4. **Stop casting and send.**
- **Record:** the log's player states and *load failed* codes for each.
- **Decides:** our ladder is expected to be refused (the Default Media Receiver carries no
  credential: D154 has no URL grant yet), which confirms that Cast needs our own receiver or the
  review's §3 grant; the public stream playing confirms the Chromecast plays fMP4 HLS at all.

### C2. Our receiver: the spike

- **Open:** `/lab/cast`, the application ID typed; **Cast to our receiver**.
- **Steps and record**, each its own line in the receiver's on-screen log, mirrored in the
  sender's log and the core's (`probe=receiver` for the receiver's own reports):
  1. **The receiver loads** (*CAF started*; the core's policy for `/cast/` lets CAF load). A
     *blocked by the page's policy* line names what the policy must still allow.
  2. **Ask for its report:** the device report from the Chromecast itself, codecs and
     `RTCPeerConnection` included, and **the WebRTC call with itself** (connected, round trips,
     tone heard). *This settles casting §7's open question: WebRTC in a custom receiver.*
  3. **Play the film on it**, with *Send this device's sign-in* ticked (see §4). Record: does it
     start, its quality, every `stall` line, smoothness over 10 minutes.
  4. **The LAN name:** the receiver loading at all, and the film playing, show the Chromecast
     resolved the core's LAN name through the router (casting §7).
  5. **Idle closing:** **Pause or play on it**, and wait **20 minutes** paused; record whether the
     receiver is still there (a *SHUTDOWN* line, or the TV's home screen), then play on. The
     receiver starts with `disableIdleTimeout` (review §4).
  6. **Memory over a 2 h film:** leave a film playing two hours; the receiver logs
     `memory … MiB` each minute and sends it. Record the first and last figures and any crash.
  7. **The TV remote's play/pause** while our player plays: record whether anything happens (review
     §4: CAF's media controls see nothing of our player).
- **Decides:**
  - WebRTC works → Cast away from home is possible through the discovery (casting §5.2: Cast
    before the `orpheum.cc` code); fails → Cast is at home only and the `orpheum.cc` code moves
    ahead for the away case.
  - hls.js plays our ladder without stalls → the receiver of casting §5.2 step 2 is built on
    `@orpheum/player`; stalls or a crash by minute 90 → the receiver needs a lighter player, or
    the weakest dongles are declared unsupported.
  - The 20-minute pause survives → `disableIdleTimeout` is enough; it does not → the receiver must
    report media status to CAF (review §4 option a).
  - Play/pause from the TV remote doing nothing → decide review §4's (a) or (b).
- **VQ58.**

## 4. Open points for the owner

- **The spike hands the receiver this device's own sign-in** (the checkbox on `/lab/cast`, off by
  default), since the playback-only `cast` grant of D154 §3 is not built. It is for the test only;
  the receiver keeps it in memory and never shows or logs it. Whether a spike may do this is the
  owner's call; without it, C2 step 3 cannot play our ladder.
- **The receiver plays only a ladder** (our chunked master playlist, with our bitrate controller),
  and sends the core no play reports or interest: a stall there may be the missing interest rather
  than the device. The receiver of step 2 would carry both.
- **`/lab/cast` starts its playback as a play page does** (`cause: playPage` in the core's log),
  since the playback's causes have no lab cause.

## 5. Results

### 2026-10-25, at home: Firefox on Linux, and one Fire TV stick

Lab results `Z46BBSZ6` and `II3BRKYJ` (Firefox 158, Linux, keyboard); `D2F4K33W` and `XOSUILPC`
(Fire TV Stick, model AFTSSS, Fire OS, Silk 138). Both opened over plain http on the house's
address, so features a browser gives only to a secure context (the microphone, the wake lock,
`setSinkId`, `getScreenDetails`) read as missing on both: rerun H1, H4 and H5 over HTTPS.

- **H1, the stick:** MSE, with H.264 to High 5.1, HEVC including Main 10, AV1, VP9 profile 0,
  AAC, Opus and FLAC; no VP9 profile 2, AC-3 or E-AC-3. Native HLS "maybe". Web Audio's delay
  43 ms base + 312 ms output, the TCL's pattern (96 + 320 ms): sound routed through Web Audio is
  likely late here too, so the sound offset can be tried at home. The in-page call connected in
  294 ms with the tone heard. 960 × 540 CSS pixels at 2×.
- **H2, the stick:** Silk's cursor takes the D-pad: no arrow reached the page, only play/pause, as
  a key (`MediaPlayPause`, keyCode 179), never as a Media Session action. Back stepped the
  page's history once and no Back key was heard. As controller research §2.1 expected; the owner
  saw nothing respond to the media keys.
- **H2, Firefox:** the arrows, Enter and Backspace all reached the page, but nothing moved, and
  the media keys did nothing. Both were the probe's fault, not the browser's:
  - It opened in the browser's own navigation, which Firefox does not have, and our search acted
    only once a tile already had focus. It now opens in ours, and an arrow while no tile has focus
    focuses the first.
  - Media keys reach a page only while it plays sound, and the tone was off until pressed. The
    first key or press now starts it.
