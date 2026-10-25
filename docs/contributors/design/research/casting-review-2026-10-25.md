> **ABOUTME:** A second opinion on [casting-2026-10-25.md](casting-2026-10-25.md): where I agree, what it misses, and
> proposed answers to its §6 questions. Read alongside that document, not instead of it.

# Casting research: review and recommendations

- **Date:** 2026-10-25.
- **Reviews:** [casting-2026-10-25.md](casting-2026-10-25.md) (at an earlier commit).
- **Source:** an outside reviewer (Claude, in chat), working from that document and the owner's
  description of the product. No code was read and nothing was run on a device.
- **Verified:** the browser-support, Fire TV and Cast SDK claims were checked against web sources
  on 2026-10-25. §8 lists what was checked, what changed, and the sources.
- **Labels:** as in the reviewed document: [1P], [U], [J], [I], UNVERIFIED, Judgement.

## 1. Overall

The research is sound and the build order is right: extend today, then the remote page and the
playback-only credential, then a Cast spike, then the receiver. I would change three things:

1. **Add the phone-as-remote's own risk to the spike.** The research studies the TV side closely.
   The bigger unknown for "the phone carries voice and the controls" is whether a mobile browser
   keeps a WebRTC call and a websocket alive for two hours. Verification narrowed this: **a Safari
   tab keeps the microphone when locked or backgrounded, but a home-screen web app loses it** (§2).
2. **The AirPlay grant cannot be "minutes".** The TV fetches chunks for the whole film and cannot
   renew anything. D154 §2's `link` grant shape does not fit as written (§3, Q2).
3. **Which Fire TV runs which OS matters for the `orpheum.cc` route.** Vega OS sticks do have Silk,
   but whether it plays our hls.js/MSE player is unknown, and three current sticks run Vega (§5,
   Q8).

## 2. The missing risk: the remote page on a phone

The split-device design depends on a phone page that stays alive: voice in, voice out, the hub
connection, and the controls. That is the device mobile browsers are least willing to keep running.

What the sources say:

- **iOS Safari, in a tab**: Safari keeps microphone access when backgrounded or with the screen
  locked; installed home-screen web apps lose the microphone as soon as they go to the background
  or the screen locks. This comes from a comment on WebKit bug 239602, last modified Dec 2023 [U].
  It is not current-iOS-confirmed: UNVERIFIED on iOS 26.
- **AudioContext is suspended** on screen lock and when the page is minimised (WebKit bug 239996)
  [U]. If remote voice is played through Web Audio, playback stays silent until the code resumes
  the context. Resuming on lock does not always work, but it always works when the page returns.
  Playing remote tracks through an `<audio>` element avoids depending on this [I].
- **Interrupted calls**: after a phone call during a WebRTC call, the microphone sometimes stayed
  muted (WebKit bugs 247993/247994), reported fixed in iOS 16.4 [U].
- **Screen Wake Lock**:
  - **Support**: Safari 16.4+, Chrome 84+, Firefox 126+ [1P, web-features]. In home-screen apps on
    iOS it was broken until iOS 18.4 [J, progressier].
  - **Release**: the lock is released automatically when the user switches tab or app, so the page
    must request it again on `visibilitychange` [1P, MDN demo].
  - **Re-request in Safari**: an early bug report said re-requesting on `visibilitychange` failed
    with `NotAllowedError` in Safari (WebKit bug 254545) [U]. Test that it works now.
- **Android Chrome** is more lenient with an active call, but vendor battery savers (Samsung,
  Xiaomi) kill background tabs. UNVERIFIED [I].

**Recommendation (judgement):** test this in the same spike week as Cast, because it gates step 1
of the build order (the remote page) more than Cast does.

| Case | Expectation to check |
| --- | --- |
| iPhone, Safari tab, screen locked during a call, 30 min | Voice in and out continue? Websocket survives? (Sources say mic yes.) |
| iPhone, Safari tab, switch app 1 min and back | Page resumes, not reloads? Voice back without a tap? |
| iPhone, home-screen web app, screen locked | Expect the mic to drop (sources). Confirm on current iOS. |
| iPhone, Safari tab, incoming phone call mid-session | Mic and audio recover after the call? |
| iPhone, wake lock held, screen dimmed, 2 h | Battery drain; does the lock re-acquire after app switches? |
| Android, Chrome, screen locked, 2 h, one Samsung or Xiaomi phone | Call continues? Battery saver kills it? |

Design consequences, already safe to adopt:

- **Tell iPhone users to use the remote page in a Safari tab, not as a home-screen app**, unless the
  test shows that current iOS fixed it. This is the opposite of the usual "install the PWA" advice.
- **Play remote voice through `<audio>` elements, not Web Audio**, or resume the `AudioContext` on
  every `visibilitychange` and on the first touch after.
- **Reconnect without a tap**: on `visibilitychange` to visible, re-acquire the wake lock, rejoin
  the hub, and check that voice is flowing both ways.
- **A deliberately boring "in call" screen**: dark, large controls, minimal redraw, so a held wake
  lock costs little.

If the Safari-tab test fails on current iOS, that is the first concrete reason for a native app (a
thin iOS shell for voice and remote), likely before any TV app.

## 3. AirPlay: the grant has to last the film

§3.2 proposes D154 §2's `link` grant ("one item, one rendition, minutes"). For AirPlay that breaks:
the TV fetches playlists and chunks for the length of the film, from a URL fixed at hand-over, and
runs none of our code, so nothing can renew the grant [I].

A grant that works:

- **Scope:** one item, all renditions of it. The TV's AVPlayer chooses the rendition, and the
  research cites Dolby/THEOplayer that manual quality selection is not possible on AirPlay [1P via
  the research], so a one-rendition grant would stall on a switch.
- **Lifetime:** the film's duration plus a margin, **and** revoked server-side when the sender's
  playback session ends or the person presses Stop. Revocation is what makes a long lifetime
  acceptable.
- **In the path, not the query string** (`/g/<grant>/master.m3u8`), with relative URLs in every
  playlist the core writes, so the grant follows without rewriting each line. Paths are also less
  likely to be logged by intermediaries than query strings [I].
- **Bound to the sender's member and session**, visible in that person's device list while live,
  with a Revoke.
- **D154 §6 amended narrowly**: "No endpoint reads a token from a query string or path segment,
  except the media endpoints under `/g/`, which accept only a `link` grant." Keep the test, and
  change it to assert that exact exception.

## 4. Cast receiver: extra items for the spike

§5.2 step 2's spike list (WebRTC, hls.js on our ladder, LAN name resolution) is the right core. Add:

- **Idle closing.**
  - **What the reference says**: `disableIdleTimeout` stops the receiver being closed when it goes
    idle after active playback stops, and is meant "only for non-media apps".
    `maxInactivity` is the time before an idle *sender connection* is closed (CAF reference,
    `CastReceiverOptions`) [1P].
  - **Why it matters**: if our player replaces CAF's media playback, CAF may see the receiver as
    idle at once, so we likely need `disableIdleTimeout: true`. In CAF's terms our receiver *is* a
    non-media app [I].
  - **What to test**: a receiver paused for 20 minutes in a group. The idle period's length was not
    found: UNVERIFIED.
- **Own player vs. CAF's media namespace.** If our player replaces CAF's, the system's media
  controls (Android's notification and lock screen, the Google Home app, the Google TV remote's
  play/pause) see nothing unless we report media status ourselves. Decide whether to:
  - (a) implement enough of `PlayerManager`'s status for these to work, or
  - (b) accept that only our remote page controls playback.

  (b) is simpler and fits the design. The TV remote's play/pause doing nothing will confuse people,
  so test whether (a) is feasible. The CAF options `skipMplLoad`, `skipShakaLoad` and
  `skipPlayersLoad` show a custom player is an expected setup [1P].
- **Memory.** Watch the receiver's memory over a full 2 h film with hls.js on the weakest device you
  plan to support. A leak shows as a crash at minute 90, not in a 5-minute test [I].
- **Hardware for the spike:**
  - a used Chromecast with Google TV (the target);
  - optionally a 3rd-gen Chromecast as the worst case, to learn early whether old dongles must be
    declared unsupported for the custom receiver.

## 5. Answers I would give to §6

1. **Extend-to-TV for the beta: yes.** Write §5.1's recipe into the testers' help as a per-device
   chooser (Windows / Mac / Chrome), with screenshots. Add one line on what it does not do yet (no
   phone remote) so testers report other things.
2. **AirPlay credential:** yes, with the grant in §3 (whole film, all renditions, revocable, in the
   path), and D154 §6 amended for exactly that.
3. **AirPlay scope:** watching alone at home only, for now. Seeking the TV every so often behind a
   multi-second buffer will look worse in a group than not offering it. Revisit only if testers with
   Apple TVs ask (judgement).
4. **Cast spike first:** yes, and in the same week the phone remote spike from §2. One day each.
5. **Cast away from home without WebRTC:** accept the gap in the beta. A guest's Chromecast is
   unsupported away from home, and their laptop can extend instead. Do not add a relay for this
   alone (judgement). It contradicts D314, and the case is rare in a closed beta.
6. **Pairing by the Cast session:** enough to start, **but** the receiver should show
   "‹person› is casting" with the film's title on launch, and the remote page should show
   "Playing on ‹TV name›" with a Disconnect. It is cheap and gives a visible answer if someone else
   on the LAN casts unexpectedly.
7. **`setSinkId` for voice to earbuds while the film goes to the TV: yes**, for the extend case,
   the beta's main living-room route.
   - **Support (corrected after checking):**
     - `HTMLMediaElement.setSinkId` is in Chrome 49+, Edge 17+, Firefox 116+ and **Safari 18.4+**
       [1P, MDN compat data].
     - Daily's documentation reports that Safari 18.4's speaker selection does not work reliably
       [1P for their SDK]. Test it on a current Mac before promising it to Mac testers.
     - Android Chrome lacks it because of an Android limitation [1P, MDN compat note]. That doesn't
       matter here: the extend case is laptops.
   - **Setup:** offer it only where `'setSinkId' in HTMLMediaElement.prototype` and
     `enumerateDevices()` lists `audiooutput` devices. Otherwise show "Default" and hide the
     choice, as Daily recommends.
   - **Where:** put the choice in voice settings: "Voice through: [device list]".
8. **Fire TV first for the `orpheum.cc` route: yes, but name the sticks.**
   - **Fire OS (support these first):**
     - **Models:** Fire TV Stick 4K Max, 4K Plus, Cube, and the Fire TV-branded TVs announced in
       2025 [J, AFTVNews/Uscreen/Yahoo].
     - **Browser:** Silk is available, and the research's B5 presumably tested one of these.
   - **Vega OS (unknown):**
     - **Models:** Fire TV Stick 4K Select (2025), Fire TV Stick HD (2026), and the newest
       3rd-gen "Fire TV Stick 4K" without Plus or Max in the name. An owner can tell from Settings,
       My Fire TV, About: a version starting "1" or "2" means Vega [J, AFTVNews, Oct 2026].
     - **Browser:** Silk exists on Vega and displays web pages, but cannot download files
       [J, MakeUseOf].
     - **Playback:** whether Vega's Silk runs MSE and plays our hls.js player was not found:
       UNVERIFIED.
   - **Test hardware:** buy one of each if Fire TV is the first target. A Fire OS 4K Max is the
     safe one, and a Vega 4K Select (around €55 at launch) is what new buyers will have.
   - **Testers' help** should say which sticks work.

## 6. Small additions

- **A "Watch on TV" button that picks the route by browser** [I]:
  - Chromium → Cast.
  - Safari on Mac → the extend recipe now, AirPlay later.
  - iPhone → AirPlay later, or "use a laptop or your TV's browser".
  - Everything → "Open orpheum.cc on your TV".

  One entry point stops testers needing to know which family applies to them.
- **Ducking for the open-microphone case**: when anyone in the group is talking, the playback devices
  lower the film by a few dB for the duration. It helps everyone without headphones, extend users
  included, and complements Talk (D281).
- **Record per-tester setups in the beta feedback form** (sender device and browser, TV or receiver,
  route used, headphones or not). The research's §7 is long, and testers will fill much of it in
  for free.

## 7. Still not checked

- **iOS Safari WebRTC behaviour on current iOS**: the sources are WebKit bug comments from 2022–23.
  The device test in §2 settles it.
- **Re-acquiring a wake lock on `visibilitychange` in current Safari.**
- **Whether Vega OS's Silk supports MSE and plays hls.js.**
- **The length of CAF's idle timeout**, and whether a custom player trips it.
- **Whether AVPlayer refetches the master playlist on a rendition switch.** This affects whether a
  path-borne grant is enough. Relative URLs in every playlist should cover both cases.
- **Whether Safari 18.4's `setSinkId` unreliability, reported by Daily, persists in current Safari.**

## 8. Verification log (2026-10-25)

| Claim in the first draft | Result | Change |
| --- | --- | --- |
| Wake Lock in Safari 16.4+ and Chrome | **Confirmed.** Also Firefox 126+; home-screen apps on iOS fixed only in iOS 18.4; released on tab or app switch | Added detail to §2 |
| `setSinkId`: Safari does not support it | **Wrong.** Safari 18.4+ supports it, reportedly unreliably at first | Corrected Q7 |
| iOS Safari suspends WebRTC when locked | **Partly wrong.** A Safari tab keeps the mic; home-screen apps lose it; `AudioContext` is suspended | Rewrote §2; reversed the PWA advice |
| Vega OS may have no browser | **Wrong.** Silk exists on Vega; MSE support unknown | Rewrote Q8; added the model list |
| CAF `maxInactivity` / `disableIdleTimeout` | **Confirmed names**, but `maxInactivity` is for sender connections, and `disableIdleTimeout` is for non-media apps | Rewrote §4's first item |

Sources:

- [Screen wake lock, web-features explorer](https://web-platform-dx.github.io/web-features-explorer/features/screen-wake-lock/)
- [Screen Wake Lock PWA limitations, Progressier](https://progressier.com/pwa-capabilities/screen-wake-lock)
- [WebKit bug 254545: Wake Lock in home-screen apps](https://bugs.webkit.org/show_bug.cgi?id=254545)
- [MDN compat data via W3cubDocs: HTMLMediaElement.setSinkId](https://docs.w3cub.com/dom/htmlmediaelement/setsinkid)
- [MDN compat data via W3cubDocs: Audio Output Devices API](https://docs.w3cub.com/dom/audio_output_devices_api)
- [Daily: setOutputDeviceAsync and Safari 18.4](https://docs.daily.co/reference/daily-js/instance-methods/set-output-device-async)
- [WebKit bug 239602: audio in background, home-screen apps lose the mic](https://bugs.webkit.org/show_bug.cgi?id=239602)
- [WebKit bug 239996: WebRTC with AudioContext silent after lock](https://bugs.webkit.org/show_bug.cgi?id=239996)
- [WebKit bug 247993: audio after a phone call, fixed in iOS 16.4](https://bugs.webkit.org/show_bug.cgi?id=247993)
- [CAF reference: CastReceiverOptions](https://developers.google.com/cast/docs/reference/web_receiver/cast.framework.CastReceiverOptions)
- [AFTVNews: Fire TVs on Vega OS (Oct 2026)](https://www.aftvnews.com/these-are-the-fire-tvs-that-dont-support-sideloading-or-downloader-due-to-vega-os-replacing-fire-os/)
- [Uscreen: Watching on Vega OS Fire TV sticks](https://help.uscreen.tv/en/articles/15594566-watching-on-amazon-s-newest-fire-tv-sticks-vega-os)
- [MakeUseOf: what no longer works on Vega OS (Silk browser)](https://www.makeuseof.com/these-fire-tv-apps-tricks-wont-work-on-amazons-new-vega-os-devices/)
- [Notebookcheck: Fire TV Stick 4K Select, Vega OS, €54.99](https://www.notebookcheck.com/Amazon-enthuellt-guenstigen-Fire-TV-Stick-4K-Select-mit-Vega-OS-auf-Linux-Basis.1128370.0.html)
