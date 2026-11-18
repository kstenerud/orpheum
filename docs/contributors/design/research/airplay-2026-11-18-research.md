> **ABOUTME:** Back to basics on AirPlay from a web page: what WebKit tells a page and lets it do,
> how the receiver's sound is chosen, how other players do it, which signals a page can trust,
> experiments for the owner's iPhone and Mac, and three candidate designs with a recommendation.

# AirPlay from a web page, from first principles

- **Date:** 2026-11-18.
- **Asked by** the owner. The owner's words: the current AirPlay is "a hacky, brittle solution";
  "go back to basics to find a proper solution. Research, competitor analysis, experiments, the
  works."
- **Sources:**
  - WebKit's source at `7520736a` (2026-11-18), read locally. Paths below are under `Source/`.
  - WebKit's bugs and blog, Apple's documentation and its developer forum, and the W3C Remote
    Playback spec.
  - The source of the open players: hls.js `c4637e5` (2026-11-17) and 1.7.3, the release the
    product pins; video.js `e9da7b8`; Shaka `af79f03`; jellyfin-web `f34d0ce`; media-chrome
    `8fd6cd2`; vidstack `0133da6`; Mux elements `97c8a86`.
  - The docs of the closed ones.
  - Clones and fetched pages are cached in `/yoloai/cache/airplay-research/`.
  - A claim checked only in a search summary, not on the page itself, is marked **UNVERIFIED**.
- **Nothing here was run on a device.** The sandbox has no Apple device and no FFmpeg. Every
  behaviour a device must show is left open, with the experiment that settles it.
- **The design is the owner's** (AGENTS.md rule 1). The designs and the recommendation below are
  the research author's, for the owner to accept, change or drop. This document records no
  decision.

## The problem

**How the product does it now ("way B", D473).** The page plays the film with hls.js under
Managed Media Source (MMS). For AirPlay, `playOnAirPlay` (`web/packages/player/src/airplay.ts`)
does four things:

1. It lets hls.js go.
2. It sets the element's `src` to a grant's master playlist at the current position. The grant is
   served to the sender's address and bound to the receiver's (D474).
3. It shows `webkitShowPlaybackTargetPicker` within the press.
4. It ends AirPlay on the wireless flag's falling edge, at Stop, or after 30 s with nothing
   picked. It then empties the element and hands it back to hls.js.

Two more decisions shape it. Stop shows the picker (D521). The grant's master lists only the sound
the page plays (D522). The lab's way A, a second element, failed (`plans/airplay.md`).

**Open faults (VQ366).**

1. A sound changed on the iPhone during AirPlay does not reach the Mac until AirPlay ends.
2. After Stop AirPlay, when the person does not choose the iPhone in the picker, the page still
   says AirPlay plays.

**Earlier faults** (agent-failures A81; `idiosyncrasies.md`, the three AirPlay rows):

- The page cannot end the iOS sound route.
- Safari sends pauses and plays the page never asked for, and its position arrives late.
- Safari swaps the element's source for AirPlay.

## Summary

| Question | Finding | Confidence |
| --- | --- | --- |
| Can a page end an AirPlay route? | **No.** The Remote Playback API has no disconnect for the page. On iOS the route is the system's, not the element's. Apple's DTS: "There is no reliable way to disconnect your app's audio playback from an AirPlay route". The sanctioned answer is to show the picker, which D521 does. | Source, Apple forum |
| Which signal says "AirPlay plays now"? | `webkitCurrentPlaybackTargetIsWireless` and its changed event: on iOS it is `AVPlayer.isExternalPlaybackActive`. `remote.state` and `prompt()` are not reliable for AirPlay. The falling edge needs about 1 s of settling. | Source, video.js measured |
| What does the picker tell the page? | On macOS a dismissal rejects `remote.prompt()` with `NotAllowedError`. On iOS the picker reports nothing back to WebCore, so a dismissal is silent. `prompt()` resolves on a connect *and* on a disconnect. | Source |
| Why did the receiver play English, not the `DEFAULT` sound (D522)? | WebKit turns off AVFoundation's automatic selection. It then picks the first option sorted by the user's preferred languages, ignoring `DEFAULT`. A page's own `audioTracks[i].enabled` turns WebKit's automatic pick off and selects that option. | Source, bug 188068 |
| Does `audioTracks[i].enabled` reach the receiver mid-AirPlay? | **Unknown.** WebKit maps it to `selectMediaOption:inMediaSelectionGroup:` on the same `AVPlayerItem` that is playing externally. Whether AirPlay forwards that is not documented. | Needs E1 |
| Does WebKit itself move MMS playback to native HLS for AirPlay? | **Yes, by design.** With an MSE blob `<source>` first and an `application/x-mpegURL` `<source>` after it, WebKit saves time, rate and pause, moves to the next `<source>` 500 ms after the wireless flag rises, and restores them. Setting `src` defeats it (bug 274142). video.js 10 and Shaka do this; the product does not. | Source, WebKit blog |
| Does a source change during AirPlay reach the receiver? | video.js measured that it does, on Safari 26.4 for macOS. iOS is untested. | Measured by others; needs E1 |
| Where do unasked pauses come from? | A likely source: on iOS a route change pauses every session that can make sound (`activeAudioRouteDidChange(ShouldPause::Yes)`). The receiver's own presses also arrive as pause and play. Neither can be told from a page's own by the event alone. | Source; needs E4 |

**Recommendation in one line.** Whichever design is chosen, drive every AirPlay state the page
shows from the debounced wireless flag. Then let the experiments choose between the product's
handover with a regrant on each sound change (design A) and WebKit's own MMS-with-alternative
handover (design B). Design B is the platform's intended path and what video.js and Shaka ship.

## 1. What WebKit tells a page and lets it do

### 1.1 The picker

- **`webkitShowPlaybackTargetPicker()`** (`WebCore/html/HTMLMediaElement.cpp:7652`) asks the
  session to show the picker. It needs a user gesture.
- **`remote.prompt()`** (`WebCore/Modules/remoteplayback/RemotePlayback.cpp:188–260`) shows the
  same picker. It rejects in these cases:
  - `InvalidAccessError` without a gesture;
  - `InvalidStateError` when remote playback is disabled;
  - `NotSupportedError` when it is disconnected and unavailable;
  - off iOS only, `NotSupportedError` before `HAVE_METADATA`. On iOS the check is skipped:
    `WebCore/html/MediaElementSession.cpp:907–934`.
- **What settles a pending `prompt()`.**
  - `shouldPlayToRemoteTargetChanged` (`RemotePlayback.cpp:262–300`) resolves every pending
    prompt whether the person connected or disconnected. On connect the state goes to
    `connecting`; on disconnect it calls `disconnect()` (`:340–367`).
  - `playbackTargetPickerWasDismissed` (`:379–397`) rejects pending prompts with
    `NotAllowedError`.
- **Whether a dismissal is ever reported depends on the platform's picker.**
  - **macOS:** `AVRoutePickerViewTargetPicker.mm:229–268` calls `devicePickerWasDismissed`, which
    reaches `HTMLMediaElement::playbackTargetPickerWasDismissed` (`HTMLMediaElement.cpp:7740`).
    The page learns of a dismissal.
  - **iOS:** `WebKit/UIProcess/ios/forms/WKAirPlayRoutePicker.mm:74–105`: the
    `didDismissHandler` clears the picker's own state and tells WebCore nothing (it is shown from
    `WKContentViewInteraction.mm:9844`).
  - **So on iOS a dismissed picker is silent, and a `prompt()` may never settle.** This is read,
    not run; E2 checks it.

### 1.2 The wireless flag, and availability

- **`webkitCurrentPlaybackTargetIsWireless`** (`HTMLMediaElement.cpp:7922`) is the player's.
  - On iOS, `MediaPlayerPrivateAVFoundationObjC::isCurrentPlaybackTargetWireless`
    (`WebCore/platform/graphics/avfoundation/objc/MediaPlayerPrivateAVFoundationObjC.mm:3404`)
    returns `[AVPlayer isExternalPlaybackActive]`.
  - The `webkitcurrentplaybacktargetiswirelesschanged` event is dispatched from a queued task
    (`HTMLMediaElement::setIsPlayingToWirelessTarget`, `:7671–7705`). The flag is therefore true
    a little before the page hears of it.
- **`webkitplaybacktargetavailabilitychanged`** carries `availability`, either `available` or
  `not-available`. On iOS it comes from the system's route monitoring
  (`MediaSessionManageriOS::externalOutputDeviceAvailableDidChange`,
  `WebCore/platform/audio/ios/MediaSessionManagerIOS.mm:175–182`). `remote.watchAvailability` is
  the standard twin.
- **The flag flickers during a handover.** video.js measured, on Safari 26.4 for macOS, that the
  flag reads false for a moment during the MMS-to-HLS handover. It therefore waits out 1 s on a
  falling edge before believing it (`packages/spf/src/playback/behaviors/dom/airplay.ts:21`,
  `REMOTE_INACTIVE_SETTLE_MS = 1000`).

### 1.3 The Remote Playback API, and ending a route

- **The spec gives the page no way to disconnect** ([W3C Remote
  Playback](https://www.w3.org/TR/remote-playback/)). Its only lever is `disableRemotePlayback`,
  which §5.3.2 says must disconnect a live session.
- **WebKit does not honour that for AirPlay.** video.js measured that setting it true did not
  end the session on Safari 26.4 (`airplay.ts:204`).
- **On iOS the flag only stops video going out.** `setWirelessVideoPlaybackDisabled`
  (`MediaPlayerPrivateAVFoundationObjC.mm:3543`) maps it to `allowsExternalPlayback = NO`. That
  brings the video home and leaves the sound on the route. This matches the product's
  idiosyncrasies row and A81.
- **On iOS the route is the app's, not the element's** (`MediaSessionManagerIOS.mm:136–217`):
  - `activeVideoRouteDidChange` gives the target to the now-playing-eligible session.
  - `applyActiveVideoRouteToSession` runs again at `sessionDidCompleteAdmission` and when the
    now-playing session changes. So an element that becomes now-playing joins a live route; E1's
    E5 button tests it.
  - `activeAudioRouteDidChange(ShouldPause::Yes)` (`:189–200`) pauses every session that can make
    sound.
- **Apple's DTS** ([forum thread 810035](https://developer.apple.com/forums/thread/810035),
  December 2025) said: "There is no reliable way to disconnect your app's audio playback from an
  AirPlay route (aside from the user taking action to change the route in Control Center...)".
  It recommends presenting an `AVRoutePickerView` when the person stops. A follow-up in the
  thread reports that `allowsExternalPlayback = false` does not stop the sound going to AirPlay.
- **The attributes** (`HTMLMediaElement.cpp:1134–1143`, `:7785–7817`).
  `x-webkit-airplay="deny"`, `webkitwirelessvideoplaybackdisabled` and `disableremoteplayback`
  each disable the target. Bug 239031 notes that the `webkit…` attribute, once set, cannot be
  removed again.
- **Support** (MDN's compatibility data):
  - the Remote Playback API: Safari 13.1, and the same on iOS;
  - `disableRemotePlayback`: iOS 13;
  - `ManagedMediaSource`: Safari 17 and iOS 17.1.

### 1.4 MSE, MMS and native HLS on AirPlay

- **AirPlay needs a URL the receiver can fetch itself.** An MSE blob cannot be sent; the
  supported target types are an `AVOutputContext` (and a mock)
  (`MediaPlayerPrivateAVFoundationObjC.mm`).
- **WebKit's own MMS pattern** ([WebKit blog, "How to use Media Source Extensions with AirPlay",
  2024-02-16](https://webkit.org/blog/15036/how-to-use-media-source-extensions-with-airplay/)):
  put the MMS blob `<source>` first and an `application/x-mpegURL` `<source>` second; the element
  "will switch over from MSE to the AirPlay-compatible URL". The mechanism in the source:
  - `HTMLMediaElement::checkPlaybackTargetCompatibility` (`:1506–1546`) runs 500 ms after the
    wireless flag changes.
  - If the player cannot play to the target and the element loaded from a `<source>` child with
    another child after it (`hasWirelessPlaybackTargetAlternative`, near `:7760`; it needs
    `LoadingFromSourceElement` and a source that is not a blob), WebCore saves `{time, rate,
    paused}` in `m_remotePlaybackConfiguration`, moves to the next `<source>`, and applies the
    saved state once that has loaded (near `:3580`).
  - Otherwise it sets `m_failedToPlayToWirelessTarget` and turns the target off.
  - The media-source player's side is at
    `MediaPlayerPrivateMediaSourceAVFObjC.mm:1342–1372`.
- **MMS opens only with `disableRemotePlayback` set or an AirPlay alternative present**
  ([Safari 17.1 notes](https://webkit.org/blog/14735/webkit-features-in-safari-17-1/)).
- **Bug 274142** ([MMS + AirPlay does not
  work](https://bugs.webkit.org/show_bug.cgi?id=274142), NEW): playback did not move to the
  receiver. Avenard's diagnosis: the page set `src`, and `src` makes `<source>` children ignored.
  The related bug 274195 is FIXED.
- **hls.js.**
  - **1.7.3, the product's pin, and master both** set `disableRemotePlayback` under MMS, remove
    every `<source>` child and append the blob's own
    (`src/controller/buffer-controller.ts:355–370` in master; the same comment, "ManagedMediaSource
    will not open without disableRemotePlayback set to false or source alternatives", is in
    1.7.3's `dist/hls.mjs:20877`).
  - **Only master** has `isMediaSourceReplaced` (`buffer-controller.ts:2109`, used at `:603`,
    `:1093`, `:2020`). It notices that the browser chose the alternate `<source>` for AirPlay and
    detaches instead of resetting over it. **1.7.3 does not have it** (no occurrence in
    `dist/hls.mjs`).
  - Issue [#6482](https://github.com/video-dev/hls.js/issues/6482) reports that AirPlay from hls.js
    1.5.11 on iOS 17.5.1 did not stream, except with autoplay or native HLS (**UNVERIFIED**, seen
    in a search summary).
- **Other bugs.**
  - Bug 270437: YouTube's AirPlay played sound only; fixed in 275730@main (March 2024).
  - Bug 159088: an `AVPlayerItem`'s asset tracks read null during AirPlay (**UNVERIFIED**, a search
    summary).

## 2. Choosing the sound on the receiver

- **What the HLS spec asks** ([RFC 8216
  §4.3.4.1](https://www.rfc-editor.org/rfc/rfc8216#section-4.3.4.1)):
  - `DEFAULT=YES`: the client should play this rendition when the user has given no preference.
  - `AUTOSELECT=YES`: the client may choose it to match the environment, such as the system
    language.
  - `AUTOSELECT` must be `YES` where `DEFAULT` is.
- **What WebKit actually does.**
  - It sets `appliesMediaSelectionCriteriaAutomatically = NO` on its `AVPlayer`
    (`MediaPlayerPrivateAVFoundationObjC.mm:1096`), then chooses for itself.
  - While a group's automatic flag holds, `MediaSelectionGroupAVFObjC::updateOptions`
    (`WebCore/platform/graphics/avfoundation/MediaSelectionGroupAVFObjC.mm:168–195`) sorts the
    options by `userPreferredLanguages()`, filters them by characteristics, and selects the first.
  - **`DEFAULT` plays no part.** That explains D522's English and bug 188068 (Safari ignores
    `DEFAULT` for audio, NEW, no engineer's comment).
- **What a page can do.**
  - `audioTracks[i].enabled = true` reaches `MediaSelectionGroupAVFObjC::setSelectedOption`
    (`:198–208`). That clears the automatic flag and calls
    `selectMediaOption:inMediaSelectionGroup:` on the playing item (`:210–221`).
  - So on native HLS a page **can** pin the sound, and WebKit's preferred-language pick no longer
    overrides it.
  - Whether the selection reaches a receiver during external playback is not documented by Apple
    and was not found reported either way. That is E1's first question.
- **What a master can do.** Marking every sound but the wanted one `AUTOSELECT=NO` may keep
  WebKit's language sort from choosing them. Whether AVFoundation's
  `mediaSelectionOptionsFromArray:filteredAndSortedAccordingToPreferredLanguages:` honours
  `AUTOSELECT` is not documented. E1 tests it with `master-noauto.m3u8`.
- **What players do.**
  - The product (D522) lists only one sound in the grant's master, which is why a change cannot
    reach the receiver without a new grant (fault 1).
  - Jellyfin changes sound by a new stream URL when it transcodes or cannot switch tracks
    (`src/components/playback/playbackmanager.js:1332–1350`).
  - video.js rebuilds its source, and measured that the receiver follows (§3).
  - No open player was found pinning the receiver's sound by `audioTracks` during AirPlay.

## 3. Competitors

| Player | How it plays HLS on Safari, and how AirPlay gets a URL | How it knows AirPlay is on or off | Sound change during AirPlay | Stop | Reported trouble | Source |
| --- | --- | --- | --- | --- | --- | --- |
| **video.js 10** (hls.js adapter) | hls.js under MMS, plus an appended `<source type="application/x-mpegURL">` of the same URL, with `disableRemotePlayback=false` unless the author opts out. WebKit switches itself. | The wireless flag only. A falling edge counts after 1 s settled. `remote.state` is deliberately not used: "Safari's W3C media.remote events don't fire reliably for AirPlay". | Rebuilds the source; the receiver follows (measured, Safari 26.4 macOS). | None: `disableRemotePlayback=true` did not end the session. At the settled falling edge it snapshots the position and play state and restores them after `emptied`. | The flag flickers false during the handover; Safari "re-sets the source when we turn AirPlay off". | `packages/adapters/hlsjs-video/src/airplay-bridge.ts:1–107`; `packages/spf/src/playback/behaviors/dom/airplay.ts:21, 75, ~150–204`; `packages/core/src/dom/store/features/remote-playback.ts:36–58` |
| **Shaka** | Needs MMS. `addSecondarySource(assetUri)` for HLS when the device supports AirPlay; appends a `<source>` and sets `disableRemotePlayback=false`. | The remote `connect`, `connecting` and `disconnect` events, else the wireless flag; `isRemotePlayback` is `remote.state != 'disconnected'`. | Not found. | Not found. | — | `lib/player.js:2318–2324, 3800–3815, 10207`; `lib/media/media_source_engine.js:347–362`; `lib/device/apple_browser.js:130` |
| **hls.js** | Forces `disableRemotePlayback` under MMS; master detaches cleanly when the browser picks an alternate `<source>`. | — (the player's job) | — | — | #6482 (UNVERIFIED) | `src/controller/buffer-controller.ts:355–370, 2109` |
| **Jellyfin web** | Never hls.js on iOS; native HLS wherever it can (Safari). | Calls `document.AirPlayEnabled`, `requestAirPlay` and `exitAirPLay` (sic). None exists, so in effect only `webkitShowPlaybackTargetPicker` runs. | A new stream URL (`changeStream`) when transcoding or when the player cannot switch. | Nothing that works. | — | `src/components/htmlMediaHelper.js:43–62`; `src/plugins/htmlVideoPlayer/plugin.js:2044–2068`; `playbackmanager.js:1332–1350` |
| **Mux** (playback-core) | `shouldDefaultToMSE` is false on Safari: native HLS. | — | — | — | — | `packages/playback-core/src/index.ts:458–475` |
| **media-chrome** | (the UI layer) | `mediaIsAirplaying` always returns false: "Cannot know if airplaying since Safari doesn't fully support … remote::state". Pressing calls `webkitShowPlaybackTargetPicker`. | — | — | — | `src/js/media-store/state-mediator.ts:1085–1108` |
| **vidstack** | (the UI layer) | `watchAvailability`, the remote connect and disconnect events, and `playing`; it prompts by `webkitShowPlaybackTargetPicker`. | — | — | — | `packages/vidstack/src/providers/html/remote-playback.ts:28–70` |
| **Bitmovin** | Its native player by default on Safari; MMS needs an HLS alternative. | — | — | — | — | [vendor page](https://bitmovin.com/managed-media-source) (UNVERIFIED, search summary) |
| **Flowplayer** | Its AirPlay plugin turns hls.js off and plays native HLS, because "Safari doesn't support airplay when MSE … [is] used". | — | — | — | — | [docs](https://docs.flowplayer.com/plugins/airplay) (UNVERIFIED, search summary) |
| **THEOplayer** | Not stated. | Only two states, `available` and `connected`, "because Safari exposes limited information"; a `statechange` event. | Not stated. | `player.cast.airplay.stop()`, its effect not stated. | — | [docs](https://optiview.dolby.com/docs/theoplayer/how-to-guides/web/cast/airplay/introduction/) |
| **Plex web** | No AirPlay from the web app; it casts through Google Cast. | — | — | — | Long-running requests for it. | [forum](https://forums.plex.tv/t/plex-app-windows-web-cannnot-see-network-player/560388) (UNVERIFIED, search summary) |
| **JW Player, Emby, Vimeo, YouTube** | Not found in public docs. YouTube does AirPlay from Safari; bug 270437 is its sound-only fault. | — | — | — | Bug 270437 | — |
| **Native: `AVPlayer` with `AVRoutePickerView`** (contrast) | `AVPlayer` with `allowsExternalPlayback` (default YES) hands the item to the route. | Key-value observation of `isExternalPlaybackActive`. | `selectMediaOption:inMediaSelectionGroup:`; whether it reaches the receiver is not stated. | The app shows the route picker; it cannot end the route either (DTS above). | — | [`allowsExternalPlayback`](https://developer.apple.com/documentation/avfoundation/avplayer/allowsexternalplayback), [`appliesMediaSelectionCriteriaAutomatically`](https://developer.apple.com/documentation/avfoundation/avplayer/appliesmediaselectioncriteriaautomatically) |

**What the table says.**

- No one ends a route.
- Everyone who gets AirPlay right on WebKit either plays native HLS throughout (Jellyfin, Mux,
  Flowplayer, Bitmovin by default) or uses WebKit's MMS-plus-alternative handover (video.js,
  Shaka).
- No one but the product sets `src` over an MMS element. That is what bug 274142's diagnosis
  warns against, though the product's way B worked in the lab.
- The one player that measured its signals (video.js) trusts only the wireless flag, with a
  settle on the falling edge.

## 4. The state model

| Signal | Trust it for | Not for |
| --- | --- | --- |
| `webkitCurrentPlaybackTargetIsWireless` and its changed event | "AirPlay plays now". On iOS this is `isExternalPlaybackActive`. Wait about 1 s before believing a falling edge (video.js). | The sound route: the sound can stay on the receiver after the flag falls (A81). |
| `webkitplaybacktargetavailabilitychanged`, `remote.watchAvailability` | Whether to show the AirPlay button. | Whether AirPlay plays. |
| `remote.state`, `connect`, `disconnect` | Nothing, for AirPlay (video.js, media-chrome, THEOplayer all say so). | — |
| `remote.prompt()` settling | macOS: a rejection with `NotAllowedError` means the picker was dismissed. | iOS: it may never settle; a resolution means "something changed", connect or disconnect. |
| A press on the page | Nothing about the route. | Showing "AirPlay plays", which is fault 2's cause wherever the page shows it from a press rather than the flag. |
| `pause`, `play`, `seeking` during AirPlay | That the element's state changed. | Who asked: the page, the receiver's remote, the lock screen, or a route change. Only the page knows its own presses. |
| `currentTime` during AirPlay | The receiver's position, late by an amount E4 measures. | Instant truth after a seek or a handover. |

**Fault 2 needs E2 before it can be fixed.** After Stop shows the picker and the person dismisses
it, the receiver probably plays on: nothing on iOS ends the route. Saying "AirPlay plays" would
then be true, and the fault is the promise Stop made. If the flag has fallen while the page still
says AirPlay plays, the page is reading its own presses, not the flag. E2 tells which, and the
answer decides between changing the words ("Still on the TV: choose this iPhone to bring it
back") and changing the logic.

## 5. Experiments for the owner

The pages are under
[`airplay-experiments/`](airplay-experiments/):

- plain HTML and JavaScript, no build;
- each logs every media, `webkit…`, `remote`, `audioTracks`, page and attribute event with its
  time, and every promise's outcome;
- a 250 ms poll catches what no event announced;
- a pause or play within 1.5 s of a page button is marked `asked by …`, any other `UNASKED`;
- **Copy log** puts the whole log on the clipboard to paste back.

### Setup, once on the Mac

The stream is made, not taken from the library (AGENTS.md rule 12): a counting test picture, and
two sounds that beep once a second, a low tone (`en`) and a high tone (`fr`). On an English iPhone
WebKit would choose the low tone, though `DEFAULT` marks the high one. The FFmpeg commands below
were not run here (the sandbox has no FFmpeg); check the video's `CODECS` against `ffprobe` if
AirPlay refuses it.

```sh
mkdir -p /tmp/airplay-lab && cp docs/contributors/design/research/airplay-experiments/* /tmp/airplay-lab/
cp web/node_modules/hls.js/dist/hls.js /tmp/airplay-lab/hls.js   # 1.7.3, the product's
cd /tmp/airplay-lab && mkdir -p video low high
ffmpeg -f lavfi -i testsrc=size=1280x720:rate=25 -t 600 -c:v libx264 -preset veryfast -pix_fmt yuv420p \
  -g 50 -keyint_min 50 -sc_threshold 0 -b:v 2500k -f hls -hls_time 4 -hls_playlist_type vod \
  -hls_segment_type fmp4 -hls_fmp4_init_filename init.mp4 -hls_segment_filename 'video/%05d.m4s' video/stream.m3u8
for tone in low:440 high:880; do name=${tone%%:*}; hz=${tone##*:}
  ffmpeg -f lavfi -i "sine=frequency=$hz:beep_factor=4:sample_rate=48000" -t 600 -c:a aac -b:a 128k \
    -f hls -hls_time 4 -hls_playlist_type vod -hls_segment_type fmp4 -hls_fmp4_init_filename init.mp4 \
    -hls_segment_filename "$name/%05d.m4s" "$name/stream.m3u8"
done
python3 -m http.server 8000
```

**Open the pages by the Mac's LAN name**, for example `http://<mac>.local:8000/e1-sound.html`, on
the iPhone, never by `localhost`. The receiver fetches the stream itself, and relative URLs
resolve against the page.

**The masters.**

| File | Contents |
| --- | --- |
| `master.m3u8` | Both sounds, `DEFAULT` on the high tone. |
| `master-noauto.m3u8` | The same, but the low tone is `AUTOSELECT=NO`. |
| `master-low.m3u8`, `master-high.m3u8` | One sound each, as a grant's master is under D522. |

**Any field takes a staging grant's master URL instead,** with the real ladder and sounds. Pass
it as `?full=…`, `?src=…` or `?alt=…`.

**Who sends to whom.** For the iPhone runs, the Mac is the receiver. For the macOS runs, the
receiver must be another device (an Apple TV or an AirPlay TV). Where there is none, the macOS
column waits.

**Hosting.** If the receiver will not fetch over plain HTTP, serve the folder from staging's
HTTPS instead.

### E1: a sound changed during AirPlay (`e1-sound.html`)

**Settles** whether the sound can be chosen, and changed during AirPlay, without a new grant:

- by `audioTracks` on a master that lists every sound;
- by `AUTOSELECT=NO` in the master;
- or only by loading a new source, which the receiver must then follow.

**Steps** (iPhone to Mac):

1. **Load full master, then Play.** Note which tone plays and the `chosen at load` line. The
   expected result is the low tone, despite `DEFAULT`.
2. **Press Enable sound 1, the high tone.** Confirm the iPhone switches.
3. **AirPlay to the Mac.** Note which tone the Mac plays.
4. **Press Enable sound 0, then Enable sound 1,** during AirPlay. After each press, listen to the
   Mac for 10 s and watch the `tracks+…ms` lines.
5. **Load `?full=master-noauto.m3u8`,** play it locally, then AirPlay it. Note the tone.
6. **Reload to a one-sound master during AirPlay.** Load low-only and AirPlay it, then press
   Reload here: high-only during AirPlay. Note:
   - whether the Mac follows;
   - how long it is silent;
   - whether the wireless flag falls;
   - whether it keeps the position.
7. **Second element plays high-only (E5),** during AirPlay. Does the new element join the route,
   or play on the iPhone? This is the product's lab way A, run again in isolation.

**What each result means.**

| Result | What it means for the design |
| --- | --- |
| Step 4 reaches the Mac | A grant's master can list every sound again (D522 relaxed), and a sound change is one property set, in any design. |
| Only step 5 holds | A full master marked `AUTOSELECT=NO` pins the receiver's starting sound, but a change still needs a reload. |
| Only step 6 holds | Fault 1's fix is a new grant and a reload at each change (design A or B). |
| Step 6 drops AirPlay | A sound change during AirPlay must end and restart it, and the design should say so to the person. |
| Step 7 joins the route | The route follows the now-playing element. A reload stays on AirPlay; way A's failure lay elsewhere. |

### E2: the picker, and ending AirPlay (`e2-picker.html`)

**Settles** what the page learns:

- when the picker is dismissed;
- when another receiver is chosen;
- when this device is chosen;
- whether anything the page does ends a route, or only its video.

**Steps.** Run each on the iPhone, then on the Mac as a sender if a receiver exists.

1. **Load and play,** then AirPlay (webkit picker). Dismiss it.
2. **The same by AirPlay (remote.prompt),** dismissed. Watch whether the promise settles; the log
   repeats "still pending" every 10 s.
3. **AirPlay to the Mac.** Then open the picker again and choose another receiver, if there is
   one. Then open it again and choose this iPhone. For each, note:
   - the `WIRELESS-CHANGED` lines;
   - the `remote.*` lines;
   - any `poll-saw` that no event explained;
   - how long each took.
4. **Run the "remote.prompt without a press" button.** Expect `InvalidAccessError`.
5. **During AirPlay, try each way to let go.** Press each of these in its own run:
   - `disableRemotePlayback = true`;
   - `x-webkit-airplay = deny`;
   - **Empty the element**.

   After each, note whether the flag falls, whether the Mac stops showing the picture, and
   whether the Mac stops playing sound. Then press **Play a short sound** and note where the tone
   is heard.

**What each result means.**

| Result | What it means for the design |
| --- | --- |
| iOS dismissal gives no event and `prompt()` never settles | The page cannot know a dismissal on iOS. Stop must end in a state that is true whether or not the person picked: driven by the flag, with words for "still on the TV". |
| macOS dismissal rejects `prompt()` | On the Mac, a dismissal can be acted on (for example "AirPlay goes on"). |
| Choosing another receiver keeps the flag true | A change of receiver is invisible except to the grant's own binding (D474): a second receiver's fetches would be refused. Worth knowing for the binding. |
| Something in step 5 ends the sound route | A81's limit is lifted, and Stop can end AirPlay itself. Not expected: DTS says no. |

### E3: MMS with an HLS alternative (`e3-mms.html`)

**Settles** whether WebKit's own handover works on iOS with the product's hls.js and stream, and
how it compares with the product's handover.

**Steps.** Press **Load hls.js** first; the log shows the version.

1. **A: native HLS, Play, AirPlay.** This is the baseline: note the time from the pick to sound
   on the Mac, and the position.
2. **B: hls.js, alternative at attach, Play, AirPlay.** Note:
   - whether `currentSrc` moves from the blob to `master-low.m3u8` about 500 ms after the flag
     rises;
   - whether the Mac starts where the iPhone was;
   - the `hls.*` lines (1.7.3 has no clean detach: watch for `BUFFER_RESET` or a reload over the
     alternative).

   Then choose this iPhone in the picker and note:
   - whether WebKit goes back to the blob by itself (video.js says Safari "re-sets the source");
   - whether hls.js plays on;
   - whether **Re-attach hls.js** is needed.
3. **C: hls.js, no alternative.** Press AirPlay and note whether the picker even offers the Mac
   with `disableRemotePlayback` true. Then add the alternative now and AirPlay: does the late
   alternative work? This is what design B needs if the grant is made at the press.
4. **D: the product's handover.** Compare the time, the position and the events with B.
5. **Repeat B** with **Toggle stopLoad** off, to see whether hls.js's loading during AirPlay
   matters.
6. **If an hls.js master build is at hand,** drop it in as `?hls=hls-master.js` and repeat B. Its
   `isMediaSourceReplaced` should detach cleanly.

**What each result means.**

| Result | What it means for the design |
| --- | --- |
| B moves to the Mac at the position and returns cleanly | Design B is sound on iOS. |
| B moves but hls.js fights it on 1.7.3 | Design B waits for an hls.js release with `isMediaSourceReplaced`. |
| C works with a late alternative | The grant can still be made at the press. |
| C fails | The grant must exist before the press, so it is made at attach while receivers are available. |
| B fails on iOS but works on macOS | Keep design A on iOS. |

### E4: unasked pauses and the late position (`e4-unasked.html`)

**Settles** which pause, play and seek events reach a page during AirPlay unasked, and how late
the receiver's position and its own presses arrive.

**Steps.**

1. **Load, Play.** Turn on Log the position every second. AirPlay to the Mac.
2. **Note every `UNASKED` line** around the pick, and whether a `pause` comes with the route
   change.
3. **Pause, Play, Seek +30 s and Seek -30 s on the page.** For each, note:
   - the delay from `press` to the event;
   - the delay to the Mac's picture changing (watch the test picture's counter);
   - any `position-jump`.
4. **Press Mark** when the Mac's counter turns over. Compare the logged `t` with the counter: that
   is the position's lag.
5. **Pause and play from the Mac's own controls and from the iPhone's lock screen.** Note the
   events and their delays.
6. **Repeat with Own the lock screen's buttons on,** so lock-screen presses log as asked.
7. **Lock the iPhone for 30 s, then unlock it.** Note what fired.

**What each result means.**

| Result | What it means for the design |
| --- | --- |
| A `pause` arrives at the route change, unasked | The page should treat pause and play within a second of a flag edge as the platform's, not the person's (watch-together's sync must not send them, as D522 already does for guests). |
| The receiver's presses arrive as plain `pause` and `play` | The page can follow the receiver's state, but must not echo it back. |
| The position lags by a measured amount | Sync and the resume point use the position only after a settle, or the page carries its own clock across the handover. |

## 6. Candidate designs

All three share one rule, the state rule: **what the page shows about AirPlay comes only from
the wireless flag, with a 1 s settle on the falling edge, never from a press**. Stop shows the
picker (D521, Apple's own advice) and makes no promise the flag does not keep.

### A. Keep the handover; regrant on a sound change

**How it works.** This is the product's way B as it is, with three changes:

- **the state rule;**
- **a sound change during AirPlay** makes a new grant for the new sound and reloads the element's
  `src` where it was (E1 step 6), as the lead first proposed (the "back to basics" section of
  `plans/navigation-and-widgets-answers-2026-10-27.md`);
- **Stop's words** follow E2: "Still on the TV" while the flag holds.

**For:**

- the smallest change;
- the lab passed way B;
- no dependency change.

**Against:**

- it keeps the page's own handover at the press (let hls.js go, set `src`, seek), which bug
  274142's diagnosis warns against and which the product had to harden piece by piece;
- each sound change costs a grant and a reload, a gap the person hears;
- the 30 s pick timer stays.

### B. WebKit's own handover: MMS with an HLS alternative

**How it works.**

- hls.js keeps the element under MMS. After `MEDIA_ATTACHED` the page appends `<source
  type="application/x-mpegURL" src=<grant master>>` and sets `disableRemotePlayback=false`, as
  video.js and Shaka do.
- WebKit switches to it 500 ms after the flag rises, carrying the position, rate and pause
  itself, and switches back when AirPlay ends.
- The AirPlay button in the native controls and in Control Center then works too.
- A sound change during AirPlay replaces the alternative and rebuilds, which video.js measured
  the receiver follows. If E1 step 4 passes, the alternative can list every sound instead and
  only `audioTracks` changes.

**For:**

- it is the platform's intended path;
- the position and pause cross the handover inside WebKit, not in the page;
- the page's handover code (`playOnAirPlay`, the pick timer, `emptied`) mostly goes;
- AirPlay started outside the page's button (Control Center, the native controls) works too.

**Against:**

- **The grant must exist before AirPlay starts,** unless E3 step 3 shows a late alternative
  works. It would then be made at attach whenever receivers are available. That means:
  - a grant for many playbacks that never AirPlay: each lasts a day, revocable (D154), and binds
    to the first other address that presents it (D474);
  - a sound change before AirPlay re-makes it.

  Whether that is acceptable is the owner's call under D473.
- **hls.js 1.7.3 lacks the clean detach.** It needs a release carrying `isMediaSourceReplaced`.
  Moving hls.js is a dependency change for the owner (AGENTS.md rule 3).
- **The evidence is thin:** measured by video.js on macOS only; iOS is untested until E3.

### C. Native HLS throughout on Safari

**How it works.** As Jellyfin, Mux and Flowplayer do: on Safari the element plays the grant's
master by `src` from the start, with no hls.js, and the sound is chosen by `audioTracks`. AirPlay
is only a change of route.

**For:**

- one element, one source, no handover;
- AirPlay, Picture in Picture and the lock screen all behave as Apple intends.

**Against:**

- **It gives up hls.js's control on Safari,** a whole player path:
  - the product's ABR;
  - the moved-down ladder and its sound;
  - the 425 and `Retry-After` handling (the core already holds AirPlay's requests for AVPlayer,
    `airPlayHold`, `internal/core/api/airplay.go`);
  - the sync measurements.
- **Every Safari playback needs a URL credential,** a grant.
- **It needs E1 step 4 to pass,** or its sound changes are reloads too.
- It is the biggest change.

### Recommendation (the research author's)

1. **Build the state rule now, in whichever design.** It is small and design-independent, and
   E2 then tells only how Stop's words should read. It is the likely fix for fault 2, unless E2
   shows the receiver really stops.
2. **Run E1 to E4,** on the iPhone first.
3. **Then choose by the results:**
   - **B,** if E3's B row passes on iOS and E1 shows the receiver follows a new alternative. It
     is the path the platform and the two best-maintained players take. It removes the handover
     code that produced most of this batch's faults. It wants an hls.js release with the clean
     detach and the owner's word on minting grants ahead.
   - **A,** if B fails on iOS. Its sound changes become regrant-and-reload.
   - **C,** only if the owner wants Safari to stop being an hls.js path for other reasons.
4. **Separately, if E1 step 4 passes,** the one-sound master (D522) can go back to listing
   every sound, with the page pinning it by `audioTracks`.

## 7. What the experiments must settle, in short

1. **E1.** Does `audioTracks[i].enabled` reach the receiver during AirPlay? Does `AUTOSELECT=NO`
   beat WebKit's language pick? Does a reload stay on AirPlay and keep the position?
2. **E2.**
   - On iOS, is a dismissed picker silent, and does `prompt()` never settle?
   - After Stop is dismissed, is the flag still true and the Mac still playing?
   - Does anything end the route?
3. **E3.** Does WebKit's MMS-to-HLS switch work on iOS with hls.js 1.7.3, at the position, and
   back? Can the alternative be added at the press?
4. **E4.**
   - Do route changes send unasked pauses?
   - How late are the receiver's position and presses?

## Open questions for the owner

1. **Grants made ahead (design B).** Is minting a grant at attach acceptable, for every Safari
   playback while receivers are available? It would last a day and be revoked at the end
   (D154, D473, D474).
2. **Moving hls.js.** Is it acceptable to move hls.js past 1.7.3 to a release with
   `isMediaSourceReplaced`, once one ships?
3. **Stop's promise.** Should Stop say "choose this iPhone to bring it back" rather than promise
   to stop, given that no page can end the route?
