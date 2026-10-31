> **ABOUTME:** Why YouTube's page plays in time in TV Bro on the owner's TCL TV while Orpheum's
> sound came out about half a second late: what YouTube's player JavaScript does and does not do
> on a TV browser, and what Orpheum can do in the page. Read before changing how the player routes,
> levels or offsets a film's sound (`web/packages/player/src/gain.ts`, `audio-shift.ts`).

# YouTube in a TV Browser: Why Its Sound Keeps Time

- **Date:** 2026-10-24.
- **Requested by** the owner, 2026-10-24: on the TCL TV in TV Bro (an Android WebView browser),
  Orpheum's sound came out about 0.5 s after its picture, through hls.js and MSE with AAC or Opus,
  and in a plain `<video src>`; YouTube in the same browser played in time. Look at YouTube's
  player to see how it treats TV browsers.
- **Builds on:** the idiosyncrasies entry [A TCL TV in TV Bro plays a page's sound about half a
  second late, and the browser cannot tell](../../idiosyncrasies.md#a-tcl-tv-in-tv-bro-plays-a-pages-sound-about-half-a-second-late-and-the-browser-cannot-tell)
  and [agent-failures A43](../../agent-failures.md), which explains why the clips that beeped every
  second cannot tell a lag apart from its direction.
- **Cache:** `/yoloai/cache/http/youtube-player/`: the pages as served to each user agent
  (`<name>.html`, with `.ua` and `.url` beside them), the four player scripts of player version
  `cbcfbf2f` (`base-desktop.js`, `base-ias.js`, `base-mweb.js`, `tv-player-ias.js`,
  `tv-player-es6.js`, each with a `.url`), and fetched sources under `web/`. TV Bro's source:
  `/yoloai/cache/git/tv-bro` at an earlier commit (2026-07-29).
- **Labels:**
  - **[1P]** read in the vendor's own code or documentation.
  - **[U]** a user's or third party's report.
  - **[S]** a search engine's summary of a page that was not read in full.
  - **[I]** my inference.

---

## 1. Summary

**YouTube does nothing special for a TV browser, and nothing about sound latency at all.** A
WebView's user agent is sent to `m.youtube.com` and gets the ordinary mobile web player (`MWEB`).
That player, like the desktop and TV players, has no code that reads or compensates an output
latency: no `outputLatency`, `baseLatency` or `getOutputTimestamp`, and no audio offset. It plays
through MSE with separate sound and picture buffers, as hls.js does, and **never routes the element
through Web Audio**: its loudness normalisation lowers `element.volume`, never above 1.

**Orpheum did route the sound through Web Audio,** and Chromium tells an element routed that way
that its sound has no output delay (§4.1). The picture is then timed to a sound that actually comes
out later by the context's base and output latency, which this TV reports as 96 + 320 to 328 ms.
The one clip that measured the lag without aliasing (the clip beeping every four seconds) was
played through Web Audio. **[I]** That route accounts for about 420 of the roughly 500 ms, and is
the most likely reason YouTube keeps time and our page did not.

**Not yet shown:** that the element's own path, without Web Audio, keeps time on this TV. The
reports that a bare `<video>` and direct play lag too came from the one-second clips, which A43
shows cannot tell a lag's direction, and do not say whether the sound was routed. §6 names the test
that settles it.

**What Orpheum can do (judgement):** keep the sound off Web Audio wherever it can, as an earlier commit
began; where it must route (levelling), count the context's reported latency, which an earlier commit
already does by moving the ladder's sound earlier. No web API reports the latency of an element's
own sound path; if that path turns out to lag too, the only remedies are a manual per-device offset
or a native player.

---

## 2. What YouTube serves a TV browser

Fetched 2026-10-24 with `curl`, the video `dQw4w9WgXcQ`. **[1P]**, read from each page's `ytcfg`.

| User agent | `www.youtube.com/watch` goes to | Client (`INNERTUBE_CLIENT_NAME`, version) | Player script |
| --- | --- | --- | --- |
| Desktop Chrome 130 on Linux | stays | `WEB` 2.20261023.01.00 | `/s/player/cbcfbf2f/player_es6.vflset/en_US/base.js` |
| Android WebView, "Smart TV" model, no `Mobile` | `m.youtube.com/watch` | `MWEB` 2.20261022.04.00 | `/s/player/cbcfbf2f/player-plasma-es6-en_US.vflset/base.js` |
| Android WebView, a TCL model name, `Mobile Safari` | `m.youtube.com/watch` | `MWEB` 2.20261022.04.00 | the same |
| The same WebView, at `www.youtube.com/tv` | stays | `TVHTML5` 7.20261021.13.00 | `/s/player/cbcfbf2f/tv-player-ias.vflset/tv-player-ias.js` |
| Cobalt (YouTube's own TV app engine), at `/tv` | stays | `TVHTML5` 7.20261021.13.00 | `/s/player/cbcfbf2f/tv-player-es6.vflset/tv-player-es6.js` |

- The site's home page for the "Smart TV" WebView also redirected to `m.youtube.com` (`MWEB`); it
  did not send the browser to `/tv`. **[I]** So the owner's YouTube in TV Bro was most likely the
  mobile web player, unless they opened `/tv` or chose a desktop user agent in TV Bro's settings.
- TV Bro (WebView flavour) sends the WebView's own user agent unless one is chosen; its other
  choices are Chrome and Firefox, desktop or mobile
  (`app/src/main/java/com/phlox/tvwebbrowser/activity/main/SettingsModel.kt:21-27`). **[1P]**
- TV Bro does nothing for YouTube or for media timing: its only media settings are
  `mediaPlaybackRequiresUserGesture` (`webengine/webview/WebViewEx.kt:134`), and its injected
  script only adds play, pause and ±10 s for the remote (`app/src/main/assets/generic_injects.js`).
  **[1P]** Whatever keeps YouTube in time is the page's or Chromium's, not TV Bro's.
- The exact user agent of the owner's TV was not seen; the WebView strings above are
  representative, not the TV's. **UNVERIFIED** that the TV gets the same redirect.

---

## 3. What the player scripts contain

Counts of each term in `base-desktop.js` / `base-mweb.js` / `tv-player-ias.js`, then what the
occurrences do. **[1P]** for what the code reads as; **[I]** for what it is for.

| Term | desktop | mweb | tv |
| --- | --- | --- | --- |
| `outputLatency`, `baseLatency`, `getOutputTimestamp` | 0 | 0 | 0 |
| `createMediaElementSource`, `AudioWorklet`, `createGain` | 0 | 0 | 0 |
| `AudioContext` | 4 | 2 | 2 |
| `requestVideoFrameCallback` | 0 | 0 | 2 |
| `audio_delay`, `audioOffset`, `audioDelay`, `lipsync`, `av_sync` | 0 | 0 | 0 |
| `avsync` | 3 | 3 | 3 |
| `tunnelmode` | 1 | 1 | 4 |
| `decodingInfo` | 7 | 7 | 7 |
| `timestampOffset` | 89 | 84 | 88 |
| `Android TV`, `SMART-TV`, `BRAVIA`, `WebOS` | 1 each | 0 | 1 each |
| `Cobalt` / `cobalt` | 9 | 9 | 12 |

### 3.1 Web Audio: a feature test only

`AudioContext` appears only in a test of whether the browser has one, which gates one format choice
(`base-mweb.js` offset 353860):

```js
rla=function(a){if(g.BK(a)&&$K)return!1;if(zJ){...}else if(g.Hs)return!1;
  return!(!window.AudioContext&&!window.webkitAudioContext)};
...if(h=c)h=a.te()?rla(a):!1;h&&(w.ea=!0);
```

**[I]** It decides whether spatial (360°) sound formats may be chosen. Nothing in any of the three
scripts creates a `MediaElementAudioSourceNode`: the film's sound goes from the element to the
speakers without Web Audio.

### 3.2 Loudness: the element's volume, attenuation only

YouTube's normalisation turns the loudness the server reports into a factor no larger than 1
(`base-mweb.js` offset 379871):

```js
var l=Math.min(h-c,0);var n=c+l;e=Math.min(1,10**(l/20))||e;
```

and applies it to the element's volume (offset 714715):

```js
b=a.N.Eb?1:eM(b.vn,c);c=a.mediaElement;c.Y(a.Jg.muted);
try{c.setVolume(a.Jg.volume*b/100)}catch{...}
```

**[1P]** It only ever turns a film down, so it needs no gain above 1 and no Web Audio. "Stable
volume" is a separate sound format the server marks `isDrc` (`hasDrcAudioTrack`,
`setDrcUserPreference`), chosen like any other track: compression done before delivery, not in the
page. **[I]** This is the shape Orpheum's an earlier commit moved toward: a sound already levelled, or not,
chosen as a rendition.

### 3.3 "avsync": segment timestamps, not output timing

The `qoe.avsync` reports fire when a segment's media time drifts from where the previous segment
ended, in YouTube's own streaming protocol (`base-mweb.js` offset 477160):

```js
e=l-a.Cg;var n=e-a.Np;...if(Math.abs(n)>1E-4){...
  n={audio:`${+a.ye()}`,sq:r.toFixed(),...,totalDrift:(e*1E3).toFixed(),segDrift:(n*1E3).toFixed(),...};
  a.loader.handleError("qoe.avsync",n);...}
```

with a sibling `ssdai.avsync` when ad-stitched sound and picture disagree on a segment number, and
`webmDurationAdjustment` logging `videoDrift`/`audioDrift`. **[I]** They keep the timestamps written
into the buffers continuous; none of them knows how late the speaker plays. Orpheum's equivalent
is already proven: our chunks' picture and sound agree within about 20 ms (the owner's
2026-10-24 report: each picture fragment's start within about 20 ms of its sound's).

### 3.4 Tunnelled playback: Cobalt only

Every script knows a `tunnelmode` MIME parameter, a Cobalt extension (YouTube's own TV app engine),
and adds it only when the user agent says Cobalt (`base-mweb.js`):

```js
else if(e.experiments.Bv().j.sa(dua)&&g.fr()){let {name:h,valid:k}=sF.TUNNELMODE;b.Dm=`; ${h}=${k}`}
// g.fr=function(){return dr("cobalt")}
```

The TV script also probes it once and only logs the answer (`tv-player-ias.js` offset 1748682,
`"dttg",{dtt:b,gl:c,tm:d}`). Cobalt's Android sources describe tunnel mode as Android's
`FLAG_HW_AV_SYNC` path, where the hardware times sound and picture together (Cobalt PR 13234, 13061)
**[S]**. **[I]** Not used in a WebView, and a WebView's `isTypeSupported` ignores the parameter, so
tunnelling is not how YouTube keeps time in TV Bro.

### 3.5 Other probes

- **Codecs:** the mobile script probes VP9 (`vp09.00.51.08…`, `vp9.2`), AV1 (`av01.0.08M.08` and
  others), H.264 (`avc1.42001E`, `avc1.4d401f`, `avc1.64001e`), AAC (`mp4a.40.2`, `mp4a.40.5`),
  Opus in WebM, AC-3, E-AC-3 and IAMF. `decodingInfo` is asked of each video format and of 2160p60
  AV1 (`c.smooth&&c.powerEfficient&&(a.O=2160)`). **[1P]** Nothing in these probes concerns sound
  timing.
- **`requestVideoFrameCallback`** appears only in the TV script, to copy frames into a WebGL
  texture (`b.texImage2D(...)` beside it): **[I]** 360° video, not timing.
- **`getVideoPlaybackQuality`** feeds dropped-frame downgrades (`html5_df_downgrade_thresh`) and a
  frame-stall detector (`qoe.videoframestall`). **[1P]**
- **User-agent lists:** the TV-name list (`Android TV;…;BRAVIA;…;SMART-TV;…;WebOS`) in the desktop
  and TV scripts is in the ad-viewability measurement code beside `CrKey`, `PlayStation`, `Roku`
  and `Xbox` checks. **[I]** It decides how ad visibility is measured, not playback. The mobile
  script has no TV names at all; its Android checks (`er`: Android and Chrome and not Cobalt;
  `VJ`: `google tv` and Chrome) gate format choices.

---

## 4. Chromium: how the sound's latency is counted

### 4.1 An element routed into Web Audio is told its sound has no delay

When a page routes an element through `createMediaElementSource`, Chromium's
`WebAudioSourceProviderImpl` pulls the element's sound for Web Audio and tells the element's
renderer the delay is zero (`third_party/blink/renderer/platform/media/web_audio_source_provider_impl.cc`,
`main`, fetched 2026-10-24, cached under `web/`):

```cpp
  // TODO(fhernqvist): If we need glitches propagated through WebAudio, plumb
  // them through here.
  tee_filter_->Render(base::TimeDelta(), base::TimeTicks::Now(), {},
                      bus_wrapper_.get());
```

**[1P]** The renderer times its clock, and so which frame is shown, from the delay it is handed. A
delay of zero means the element's clock counts none of Web Audio's buffering and output latency,
while the sound waits through all of it. **[I]** Any page that routes a film through Web Audio plays
its sound late by about `baseLatency + outputLatency`; on a desktop that is tens of milliseconds and
unnoticed, on this TV about 420 ms. [WebAudio high latency on Android
(40103372)](https://issues.chromium.org/issues/40103372) reports a total near 330 ms on some
Android devices that lack the low-latency path **[S]**.

### 4.2 The element's own path counts its sink's delay

Without Web Audio, the element's sound goes to Chromium's audio output, whose reported delay the
renderer uses for its clock (`media/renderers/audio_renderer_impl.cc`, "audio_clock",
[source](https://chromium.googlesource.com/chromium/src/media/+/refs/heads/master/renderers/audio_renderer_impl.cc))
**[S]**. On Android that delay comes from the platform's playback position and timestamp (as
`AudioTrack.getTimestamp` gives an app; [Fire TV's A/V sync
guide](https://developer.amazon.com/docs/fire-tv/audio-video-synchronization.html)) **[S]**. **[I]**
If the TV's audio HAL reported its latency wrongly, YouTube's page, on the same path, would lag
alike; it does not, which is the strongest sign that the element's path on this TV is right.

### 4.3 Other reports

- Chromium 129 and later delayed sound 100 to 200 ms over Bluetooth on macOS when a change dropped
  the device latency from the total (`ExcludeDeviceLatencyFromTotalLatency`, [issue
  385190289](https://issues.chromium.org/issues/385190289)) **[S]**. A platform fault of this kind
  hits every page alike, YouTube's included.
- mrowser, a TV browser, says WebView players "slowly lose A/V sync. Lips lead the sound, and it
  never recovers", and hands streams to ExoPlayer instead ([mrowser](https://m-salehi-v.github.io/mrowser/),
  fetched 2026-10-24) **[U]**. It cites no measurement, describes a drift rather than a constant
  lag, and is not what the owner saw.
- No report was found of a WebView's element sound lagging its clock by a constant half second on
  an Android TV (searched 2026-10-24).

---

## 5. Why YouTube keeps time and our page did not

1. **Most likely (strong):** our sound went through Web Audio and YouTube's never does. The only
   unaliased measurement, the four-second-beep clip on the ladder, was through Web Audio; the context reported
   96 + 320 ms; Chromium does not count it (§4.1). 416 ms of a lag judged by eye at "about 0.5 s"
   leaves little to explain. Levelling and its undoing (before an earlier commit) routed every ladder
   playback, and the 600 ms sound delay routed every playback it touched,
   the bare player's included (an earlier commit's `soundDelay` routes only when the delay is above 0).
2. **Possible, unproven:** the element's own path also lags on this TV for our files and not for
   YouTube's: a codec, container or sample-rate difference the TV's decoder or mixer handles
   differently (YouTube's likely formats here: VP9 or AV1 with Opus in WebM, or H.264 with AAC,
   all through MSE). The evidence for it, the bare `<video>` and direct play lagging, came from the
   one-second clips (A43), and whether those plays were routed is not recorded.
3. **Ruled out by reading:** YouTube compensates a TV's latency in the page (no code for it, §3);
   it uses tunnelled playback in a WebView (Cobalt only, §3.4); TV Bro treats YouTube specially
   (§2); our chunks' timestamps disagree (proven within about 20 ms).

---

## 6. What Orpheum can do in the browser

Judgement, in order:

1. **Keep a film's sound off Web Audio wherever possible.** An earlier commit does this for a person
   without levelling. For a person with levelling, the levelled sound can be a rendition, as
   YouTube's stable volume is, and any remaining turn-down can be `element.volume` (attenuation
   only, §3.2); only a boost needs Web Audio.
2. **Where it must route, count the context's latency.** `applyOffset` in `gain.ts`
   already subtracts `baseLatency + outputLatency` from the wanted offset and moves the ladder's
   sound fragments earlier by the rest, so a routed ladder playback should now keep time with an
   offset of 0. A direct play cannot have its sound moved earlier; there, routing for a boost costs
   the latency, and the player could decline the boost on a device whose latency is large (the
   startup report already logs it).
3. **Settle cause 2 with one test:** either sync-test clip, offset 0, levelling off, in the bare
   player (no Web Audio) and on the ladder after an earlier commit. In time: the cause is §5.1 and the
   manual offset is a fallback. Still late: the element's own path lags for our files, and the next
   test is the same clip as VP9 + Opus WebM and H.264 + AAC MP4 at 48 kHz, against YouTube on the
   same TV.
4. **No API exposes the element's own sink latency.** `outputLatency` and `getOutputTimestamp`
   belong to an `AudioContext`; an element has nothing like them, and `requestVideoFrameCallback`
   reports only the picture. If the element's path is what lags, the remedies are the per-device
   offset or a native player.

---

## 7. Not tried

- **Nothing was played on the TV,** or in any WebView: every statement about the TV comes from the
  owner's reports and the staging log. Which formats YouTube chose there, and whether it was `MWEB`
  or `/tv`, was not seen.
- **The owner's TV's user agent** was not read; the redirects were tried with representative
  WebView strings. A desktop user agent chosen in TV Bro would get the `WEB` player, which §3 finds
  no different on this question.
- **Only player version an earlier commit** (2026-10-24) was read, minified, by searching for terms; code
  that uses other names for latency would be missed. YouTube's server-side experiment flags, which
  can turn code paths on per device, were not seen.
- **Chromium's Android audio output** (`AAudioOutputStream`, `AudioTrackOutputStream`) and how it
  reports delay on a TV were not read; §4.2 rests on a summary.
- **TV Bro's GeckoView flavour** was not considered; the owner's is assumed to be the WebView one.
