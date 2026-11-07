> **ABOUTME:** Research for the owner to decide whether Watch Together's voice chat can travel with
> a cast, per cast method (Google Cast, AirPlay, Orpheum's own receiver in a TV browser): whether
> the receiver can play the others' voices, whether it can take a microphone, and what the split
> model (microphone on the phone, voices on the TV) costs in echo and delay. Read before designing
> any "voices on the TV" option.

# Voice Chat and Casting

- **Date:** 2026-11-07.
- **Requested by** the owner, 2026-11-07: *"I'm undecided on whether voice chat should be part of
  the cast or not (or if that's even possible). Do a research round to find out. It's fine if we
  have to keep it on the local device and not cast it. If we can cast it, I'd want it optional. I'm
  also fine with splitting voice recording such that it's done on the source device, and the
  cast-to device plays back the voices of others."*
- **Builds on:** [watch-together-video-chat-2026-11-06.md](watch-together-video-chat-2026-11-06.md)
  §8–9 (its platform table left "Cast receiver (CAF): WebRTC unknown");
  [watch-together-split-devices-2026-10-24.md](watch-together-split-devices-2026-10-24.md) §4.10
  (echo once the film is on speakers); [voice-apps.md](voice-apps.md) (SharePlay's smart volume,
  Discord, Android's call mode); D63, D65, D434. It does not repeat them.
- **Marks:** [V] verified in a cited source; [R] read in this repository; [I] inferred, not
  verified. Device facts not yet measured are named as such.

## Short answer

- **Microphone on the TV: no, for every cast method.** No Cast device, Apple TV (to a web page) or
  TV browser in the device lab gives a page a usable microphone. The microphone stays on the phone.
- **Voices on the TV:**
  - **AirPlay: not possible.** The receiver runs none of our code, and iOS refuses to AirPlay a
    WebRTC stream or call audio, except by mirroring the whole screen.
  - **Google Cast: possible with caveats, unproven on a device.** The receiver is a Chromium page
    that has `RTCPeerConnection`. Its limits on concurrent media elements and autoplay are
    unmeasured.
  - **Orpheum's own receiver in a TV browser: possible.** It is proven on Fire TV Silk (a WebRTC
    call with a tone heard). The TCL and Android TV browsers are unmeasured. On a desktop browser
    it is straightforward.
- **The split model's real cost is echo, not plumbing.** When the others' voices come out of the
  TV and the phone's microphone hears them, they return to their speakers late. The phone's browser
  echo canceller cannot remove them, because its reference is the phone's own output, not the TV's
  (D63) [V for how AEC works; I for our case]. Today, voices on the phone's speaker are cancelled
  by the phone's own canceller. Moving them to the TV gives that up.
- **What products do:** Apple keeps the call on the phone when SharePlay's video goes to an Apple
  TV. Google Meet's cast keeps the computer's microphone and sound. Apple and Facebook's own TV
  calling devices, which do play voices on the TV, both drew echo complaints.
- **Recommendation:** keep voices on the phone by default for every method. Offer "Voices on the
  TV" as an option only for Orpheum's receiver and Google Cast, once the device lab shows it works.
  When on, it changes the phone's echo handling (§6).

## 1. What exists today [R]

- **Voice** is a WebRTC mesh between voice-role devices (D281). It has per-person volume (0–4×),
  echo cancellation and AGC as settings, and headphones as the supported configuration (D63). The
  playback offset is an echo control (D65).
- **Split devices** (sketch, 2026-10-24): the TV holds a playback-only role. Voice and controls
  run on the phone. The others' voices are heard on the phone.
- **Casting** has three paths: Google Cast with our CAF receiver (`web/apps/cast-receiver`); AirPlay
  from Safari; and Orpheum's own receiver page in a TV's browser (D434). Under D434 the receiver's
  sound (offset, levelling, track) is set in the cast's session, never in Watch Together's.
- **The Cast receiver already uses WebRTC, at least as code.** It reaches its sender's core through
  the discovery "else WebRTC" (`src/reach.ts`, commit "feat(cast-receiver): the lab's receiver reaches its sender's core from away, on Chromium 90", built for Chromium 90). The lab's
  receiver runs a WebRTC call with itself carrying a tone (`loopbackWithTone` in `src/lab.ts`).
  The Chromecast run of that test (plans/tv-device-tests.md C2, VQ58) has no recorded result yet.
- **Fire TV Silk passed the same test** (tv-device-tests.md §5, 2026-10-25, Silk 138 on an AFTSSS
  stick): *"The in-page call connected in 294 ms with the tone heard."* It also lists Opus under
  MSE, and Web Audio output latency of 43 ms base + 312 ms output. The TCL measured 96 + 320 ms.
  That run was over plain http, so `getUserMedia` read as missing; it was not tested.

## 2. Per cast method

### 2.1 Google Cast (custom Web Receiver)

**Can it play the others' voices?** Probably yes, with limits to measure.

- Google's documentation describes a Custom Web Receiver as an HTML5 app hosted by us
  ([Web Receiver overview](https://developers.google.com/cast/docs/web_receiver)) [V]. It says
  nothing about WebRTC either way. No official statement of support or of its absence was found [V:
  searches 2026-11-07].
- People have run WebRTC in receivers:
  - A 2018 report streamed WebRTC from iOS to a Chromecast receiver based on appr.tc. Audio
    played, but 30 fps video played at about 10 fps, with lip-sync drift
    ([discuss-webrtc, 2018-02-08](https://groups.google.com/g/discuss-webrtc/c/hLspzd8B_Ds)) [V].
  - A 2018 Stack Overflow question reports a WebRTC mirroring receiver that played audio and video
    and then crashed after about 30 s, unanswered
    ([SO 50433412, mirror](https://kaweah.freedombox.rocks/kiwix/stackoverflow.com_en_all_2023-11/questions/50433412/chromecast-reciever-with-webrtc-peerconnection-crashes-after-30sec))
    [V].
  - Home Assistant's WebRTC camera card casts to Chromecast and Android TV
    ([AlexxIT/WebRTC](https://github.com/AlexxIT/WebRTC)) [V]. Users there report casts starting
    muted ([HA forum](https://community.home-assistant.io/t/go2rtc-project-help-thread/454451?page=24))
    [V].
  - go2rtc notes a Chrome 70 Chromecast (1st gen) failing `addTransceiver` outside unified plan
    ([go2rtc www](https://beta.pkg.go.dev/github.com/AlexxIT/go2rtc/www)) [V, snippet]. Our
    receiver's floor is Chromium 90, where unified plan is the default [I].
- **Limits that matter for voices beside our film:**
  - **Concurrent media elements.** A 2015 answer says a Chromecast receiver allowed one active
    media element: a song and a sound effect played together on desktop, but only one played on
    the device
    ([SO 29262998, mirror](https://kiwix.lema.org/stackoverflow.com_en_all_2023-11/questions/29262998/google-chromecast-receiver-doesn-t-play-simultaneous-audio))
    [V, snippet; old]. This collides with our own finding that **Chrome plays a remote WebRTC
    stream into Web Audio only while a media element also holds it** (idiosyncrasies.md) [R]. So
    voices may need a second media element on a device that may allow only one. Whether today's
    devices keep that limit is unknown. The device lab must test the film and a voice element
    playing together.
  - **Sound on start.** Our receiver is started by a message, so the browser gives it no sound
    without a gesture (idiosyncrasies.md, "A cast receiver is started by a message") [R]. Whatever
    unmutes the film must also cover the voice element [I].
  - **Bitstreamed film audio.** If the film's audio ever goes to the TV or AVR as a bitstream
    (AC-3/E-AC-3 passthrough), a PCM voice cannot be mixed into it [I]. Our ladder sends AAC, so
    today this is moot.
  - **Memory and CPU.** One Opus decode per speaker is small next to the film's video [I]. Our
    receiver already logs memory each minute over a 2 h film (C2 step 6) [R]. Mixing the voices into
    one track at the core would cut N−1 decodes to one, and one element (§6) [I].
- **Can it take a microphone?** No.
  - Chromecast dongles have no microphone. The Google TV remote's microphone serves the Assistant;
    no source shows a Web Receiver reaching it [I].
  - Google's own design keeps the call device's microphone. Meet casting says *"You can still use
    the camera, microphone, and audio from your computer"*
    ([Cast Google meetings to your TV](https://support.google.com/meet/answer/10023891)) [V].
    Coverage of Meet's launch on Chromecast says it uses the phone's or computer's microphone
    ([9to5Google, 2020-08-18](https://9to5google.com/2020/08/18/google-meet-chromecast-tv-support/))
    [V].
  - Google ended Meet calling on Android TV in early 2025 (video-chat report §9) [R].
  - A receiver also has no user to grant a permission prompt [I].

### 2.2 AirPlay

**Can it play the others' voices?** No, not in a way that holds a conversation.

- Safari's AirPlay hands the Apple TV a media element's URL. **A video element whose `srcObject`
  is a stream does not AirPlay**: the Apple TV stops what it played and shows nothing
  ([WebKit bug 281693](https://bugs.webkit.org/show_bug.cgi?id=281693), filed 2024-10, still NEW
  in the search results) [V]. WebKit's remote player once rejected any MediaStream outright
  ([bug 254321](https://www2.webkit.org/show_bug.cgi?id=254321)) [V, snippet].
- Call audio cannot be routed to AirPlay either. Apple's QA1803: *"When an application uses the
  `AVAudioSessionModeVoiceChat` mode, AirPlay is not allowed"*, and `PlayAndRecord` *"supports only
  the mirrored variant of AirPlay"*
  ([QA1803](https://developer.apple.com/library/ios/qa/qa1803/_index.html)) [V]. A page with an
  open microphone is in a play-and-record session (voice-apps.md) [R]. The only route to the TV
  is screen mirroring of the whole phone [I].
- Putting the voices into the film's own stream at the core (an HLS audio mix) would reach the
  Apple TV, but late. AirPlay 2 holds about 2 s of buffer
  ([Apple Community](https://discussions.apple.com/thread/8053774);
  [Plex forum](https://forums.plex.tv/t/pause-play-takes-2-5-seconds-to-stop-start-when-using-airplay-carplay/870185))
  [V, user reports]. Add HLS segment latency, and a reply would come seconds after its question:
  unusable for talk [I].
- **Can it take a microphone?** No. The Apple TV runs no page of ours. Its own FaceTime uses an
  iPhone or iPad as a Continuity Camera and microphone
  ([Apple TV guide](https://support.apple.com/en-euro/guide/tv/use-your-iphone-or-ipad-as-a-camera-atvb45658929/tvos))
  [V], which is open only to Apple's and native apps.
- **One risk to check.** While the iPhone's microphone is open, the play-and-record session might
  disturb the film's own (non-mirrored) AirPlay [I]. Probably it does not: the Apple TV fetches the
  URL itself. A device run should confirm that voice and an AirPlayed film coexist.

### 2.3 Orpheum's own receiver in a TV browser (D434)

**Can it play the others' voices?** Yes where WebRTC works, which is the case we can test best.

| Browser | WebRTC audio in | Microphone | Evidence |
| --- | --- | --- | --- |
| Fire TV, Silk | **yes**: call connected in 294 ms, tone heard; Opus present | none in practice; `getUserMedia` untested (the run was over http) | tv-device-tests.md §5 H1 [R] |
| TCL Google TV, TV Bro | unmeasured | none in practice | B1 / VQ59 pending [R] |
| Android TV / Google TV, Chrome-based browsers | unmeasured; likely, being Chromium | none in practice; USB webcams worked on some models only | video-chat report §9 [R] |
| Desktop Chrome, Edge, Firefox, Safari | yes | yes, if it has one | standard |

- A TV browser page is an ordinary page with a remote. It has neither the Cast receiver's
  media-element question nor its message-started muting, though it has the same autoplay rule
  until a key is pressed [I]. The Silk run shows sound through Web Audio is late: 312 ms output
  latency (§1) [R].
- **Microphone:** TV boxes have none a page can open. Fire TV's remote microphone is the Alexa
  button's [I]. A desktop or laptop acting as the TV has one. With Chrome's wide echo
  cancellation, which includes the page's own output (video-chat report §8) [R], it could hold
  both microphone and voices like any member's device. That is just an ordinary member, not a
  split.

## 3. The split model: microphone on the phone, voices on the TV

### 3.1 Why the phone's echo canceller does not help

- A canceller subtracts a copy of what went to the loudspeaker, the "render" or reference signal,
  from what the microphone hears. That copy must be the far-end audio before it is mixed with
  anything else ([Fora Soft, 2026-06](https://www.forasoft.com/learn/ai-for-video-engineering/articles-ai/echo-cancellation-aec-ai-hybrid-webrtc);
  [Switchboard on AEC3](https://switchboard-website-astro.pages.dev/hub/how-webrtc-aec3-works)) [V].
  AEC3 finds the delay between reference and echo by cross-correlation, and expects 20–200 ms on
  mobiles (Switchboard) [V].
- A browser's canceller takes its reference from the same device's output. When the voices play
  on the TV, the phone outputs nothing, so it has nothing to subtract (D63) [R/I].
- **Today's state is better for voices.** On the phone's speaker with echo cancellation on, the
  phone cancels the voices it plays. Only the film, from the TV, leaks, which D65's offset
  addresses. Moving the voices to the TV adds a second leak: **each speaker hears their own voice
  come back** after their send, the TV's jitter buffer and output latency (300+ ms on the TCL and
  the Fire TV, §1), the room, and the return trip. That is plainly audible echo [I].

### 3.2 Ways to handle it, best first [I unless marked]

1. **Earbuds or a headset microphone on the phone.** Less of the TV reaches the microphone, but it
   does not remove it. Earbuds also make sending the voices to the TV pointless.
2. **A canceller of our own on the phone, fed the voices as reference.**
   - The phone already receives every voice stream in the mesh. It could keep receiving them
     unplayed and feed them to an AEC3 build as the render signal.
     [`@ennuicastr/webrtcaec3.js`](https://npmjs.com/package/@ennuicastr/webrtcaec3.js) is WebRTC's
     AEC3 in WebAssembly, BSD-licensed, run in a worker, and works at 48 kHz [V].
   - Two questions are open. Is the TV's extra delay (receive-time difference plus 300+ ms output)
     within what AEC3's delay estimator follows? And does the TV's speaker and room colouring defeat
     it? Facebook's Portal TV is a warning. It owned both its output and its microphone, yet users
     reported poor cancellation through TV speakers and soundbars, with half-duplex cutting
     ([Portal feedback board](https://portal.uservoice.com/forums/916228-portal-from-facebook/suggestions/39172813-add-option-to-use-portal-tv-inbuilt-speaker-as-def);
     [Sonos community](https://en.community.sonos.com/home-theater-228993/could-not-use-sony-tv-sound-keep-switching-back-to-sonos-soundbar-6858409))
     [V, user reports].
   - It would be a new dependency, the owner's call (rule 3).
3. **Half-duplex suppression on the phone.** Lower or gate the phone's microphone while the TV is
   playing a voice (speaker detection: `getSynchronizationSources().audioLevel`, video-chat report
   §8). It is cheap and reliable, but people cannot talk over each other. Portal's users disliked
   exactly this.
4. **Mute the phone's microphone by default while voices are on the TV, with push-to-talk.**
   Simple and echo-free, but a different way of using it.

### 3.3 What products do

- **Apple SharePlay, video moved to an Apple TV:**
  - The FaceTime call stays on the iPhone. Apple: *"The video plays in sync on Apple TV 4K and on
    all the devices on the FaceTime call"*, and you move it back *"to the device with the active
    FaceTime call"*
    ([Apple TV guide](https://support.apple.com/en-au/guide/tv/atvb65186d08/tvos)) [V]. A
    MacRumors user who moved SharePlay to Apple TVs on both ends heard *"the audio from our FaceTime
    call ... in our AirPods"* and the show on the TV
    ([MacRumors](https://forums.macrumors.com/threads/apple-shareplay-audio-and-apple-tv.2330378/))
    [V].
  - So Apple's AirPlay answer is our split-devices sketch: **voices local, film on the TV**.
  - Smart volume ducks the content on the calling device
    ([FaceTime guide](https://support.apple.com/guide/facetime/fctm725416ba)) [V]. No source shows
    the Apple TV ducking the film for a call it does not hold [V: not found].
- **FaceTime on Apple TV (tvOS 17+):**
  - Here the voices do play on the TV, and the iPhone is the Continuity microphone [V, Apple
    guide above].
  - Apple owns both ends, and has measured the TV's sound latency with the iPhone's microphone
    since 2019 (Wireless Audio Sync,
    [Apple guide](https://support.apple.com/guide/tv/atvb228b7711/tvos)) [V]. That is the
    reference and the delay a canceller needs [I that FaceTime uses it].
  - Even so, users report echo at the far end
    ([Apple Community 256083228](https://discussions.apple.com/thread/256083228)) [V], and
    microphone dropouts ([255411270](https://discussions.apple.com/thread/255411270)) [V].
    MacStories found no feedback with the phone above the TV's speakers
    ([tvOS 17 review](https://www.macstories.net/reviews/tvos-17-the-macstories-review/4/)) [V].
- **Google Meet cast to a Chromecast:** the TV shows the meeting. The computer keeps its camera,
  microphone and sound (Meet help, §2.1) [V]. Users report casting *with no audio* on the TV
  ([Nest community](https://www.googlenestcommunity.com/t5/Apps-Account/Google-Meet-casting-to-TV-but-NO-AUDIO/m-p/401133);
  [Android Central forum](https://forums.androidcentral.com/threads/why-wont-meet-incoming-audio-cast-to-chromecast.1052675))
  [V]. Whether that is a design choice or a fault, I could not establish [I].
- **Facebook Portal TV:** voices on the TV with its own microphone and AEC, and echo complaints
  anyway (§3.2) [V].
- **Discord, Teleparty, Plex Watch Together, Disney+ GroupWatch** have no "voices on the cast"
  feature. Discord's voice stays on the device. Teleparty's voice is browser-only. Plex and
  GroupWatch had no voice (voice-apps.md §2) [R]. No source shows Zoom or Teams sending call audio
  to a Chromecast [V: not found].

## 4. Sync and delay

- **Voices are not lined up with the film; they play as they arrive.** What a person says relates
  to their own film, which D65's offset aligns. Voices on the TV only add the TV's output latency
  to the mouth-to-ear delay [I].
- **Mouth-to-ear budget.** ITU-T G.114 treats up to 150 ms one-way as fine for most uses, 150–400
  ms as acceptable with care, and above 400 ms as unacceptable for planning
  ([ITU-T G.114](https://www.itu.int/ITU-T/recommendations/rec.aspx?rec=6254); bands via secondary
  sources) [V]. Phone to phone in the house or across a city is roughly 100–200 ms [I]. Add the
  TV's 300+ ms (Silk 312 ms and TCL 320 ms through Web Audio; a plain media element may be less,
  unmeasured) and the TV voices sit at or beyond 400 ms. Talk becomes noticeably turn-based [I].
- **The cast receiver adds a second clock.** Cast and TV-browser receivers already need a sound
  offset (D434's sound timing) because their output is late. Voices on the TV get no such
  correction, and need none, but they are later than the same voices on the phone [I].
- **Lip sync matters only if faces are shown.** If video tiles (video-chat report) were shown on
  the TV, their audio must play in the same peer connection on the same device, or WebRTC's
  audio-video sync is lost. Tiles on the TV with voices on the phone would be out of sync by the
  TV's latency [I]. That argues for "voices and tiles together, wherever they are".
- **The phone's microphone and the film.** Unchanged from today: the film leaks from the TV and
  D65's offset is the control (split-devices §4.10) [R].

## 5. Verdicts

| Cast method | Voices on the receiver | Microphone on the receiver | Verdict |
| --- | --- | --- | --- |
| Google Cast Web Receiver | likely; `RTCPeerConnection` exists in Chromium receivers; one-media-element limit and autoplay unmeasured | no | **possible with caveats**, after the C2/VQ58 device run plus a film+voice run |
| AirPlay | no: WebRTC streams and call audio do not AirPlay (WebKit 281693, QA1803); a core-side mix is seconds late | no | **not possible**; voices stay on the phone |
| Orpheum receiver, Fire TV Silk | yes (tone heard, 294 ms connect) | no | **possible with caveats**: echo and 300+ ms output latency |
| Orpheum receiver, TCL TV Bro / Android TV browsers | unmeasured | no | **possible with caveats**, pending B1/VQ59 |
| Orpheum receiver, desktop browser | yes | yes | **straightforward**; it can simply be a full voice member |

In every "possible" case, the cost is §3's echo, which the phone's own canceller cannot fix.

## 6. What "optional" would mean

A sketch for the owner, not a decision [I]:

- **Default: voices on the phone.** Phone speaker with echo cancellation on, or earbuds, as today.
  This is Apple's choice and the split-devices sketch's.
- **Option "Play voices on the TV",** offered only when the receiver reports it can (a capability
  in its device report: WebRTC audio in, plus a film-and-voice test passed). It is never offered
  for AirPlay.
  - **Where it lives:** under D434, the receiver's sound is set in the cast's session, so the
    switch belongs in the cast dialog's sound panel. The voices themselves are Watch Together's.
    Which session owns the switch is a question for the owner (Q2).
  - **What turns on with it:**
    - The TV joins the voice as a listen-only device. It receives either each voice or one mix from
      the core. A mix is one element and one decode, which helps the Cast receiver's media-element
      question.
    - Per-person volumes, set on the phone, are applied at the TV.
    - The phone stops playing voices.
    - The phone switches to one of §3.2's echo handlings: our own canceller with the voices as
      reference, half-duplex, or push-to-talk.
  - **What the person sees:** a short warning on choosing it. For example: "Voices will come from
    the TV. Your phone's microphone may send them back; use push-to-talk or earbuds if others hear
    an echo." Also an "echo check": the phone plays a tone through the TV and listens for it,
    as Apple's Wireless Audio Sync does. That measures the delay, and can recommend against the
    option in a loud, echoing room.
  - **Turning it off** returns voices to the phone at once, mid-session.

## 7. Open questions for the owner

1. **Is voices-on-TV worth its echo cost?** Is it worth it with half-duplex or push-to-talk on the
   phone? Or only once a canceller of our own (a new dependency, AEC3 in WebAssembly) proves
   itself on a device?
2. **Which session owns the switch:** the cast dialog (D434: the receiver's sound) or Watch
   Together's panel (the voices)?
3. **Should the core mix the voices** for a listen-only TV? This would be a new job for the SFU
   the video-chat report proposes. Or should the TV join the mesh as one more receiver?

## 8. What only a device can confirm

- Chromecast (C2/VQ58): the WebRTC self-call with tone. Then **the film and a WebRTC voice element
  playing at once**, and whether the voice starts sounding without a gesture once the film has
  sound.
- TCL TV Bro and an Android TV browser (B1/VQ59): the same pair.
- Fire TV Silk: the film and a voice playing together, and a media element's output latency for
  the voice compared with Web Audio's 312 ms.
- iPhone: an AirPlayed film stays up while the page's microphone is open.
- A phone beside a loud TV playing voices: how bad the echo is with the browser canceller alone,
  and with half-duplex.

## Sources

Fetched or searched 2026-11-07. Repository sources are cited inline.

- Google: [Web Receiver overview](https://developers.google.com/cast/docs/web_receiver);
  [Cast Google meetings to your TV](https://support.google.com/meet/answer/10023891);
  [9to5Google on Meet and Chromecast](https://9to5google.com/2020/08/18/google-meet-chromecast-tv-support/).
- WebRTC on Cast: [discuss-webrtc 2018](https://groups.google.com/g/discuss-webrtc/c/hLspzd8B_Ds);
  [SO 50433412](https://kaweah.freedombox.rocks/kiwix/stackoverflow.com_en_all_2023-11/questions/50433412/chromecast-reciever-with-webrtc-peerconnection-crashes-after-30sec);
  [SO 29262998](https://kiwix.lema.org/stackoverflow.com_en_all_2023-11/questions/29262998/google-chromecast-receiver-doesn-t-play-simultaneous-audio);
  [AlexxIT/WebRTC](https://github.com/AlexxIT/WebRTC);
  [go2rtc www](https://beta.pkg.go.dev/github.com/AlexxIT/go2rtc/www);
  [HA go2rtc thread](https://community.home-assistant.io/t/go2rtc-project-help-thread/454451?page=24).
- Apple: [QA1803](https://developer.apple.com/library/ios/qa/qa1803/_index.html);
  [WebKit 281693](https://bugs.webkit.org/show_bug.cgi?id=281693);
  [WebKit 254321](https://www2.webkit.org/show_bug.cgi?id=254321);
  [SharePlay on Apple TV](https://support.apple.com/en-au/guide/tv/atvb65186d08/tvos);
  [iPhone as camera for Apple TV](https://support.apple.com/en-euro/guide/tv/use-your-iphone-or-ipad-as-a-camera-atvb45658929/tvos);
  [Wireless Audio Sync](https://support.apple.com/guide/tv/atvb228b7711/tvos);
  [FaceTime SharePlay guide](https://support.apple.com/guide/facetime/fctm725416ba);
  [MacRumors SharePlay audio](https://forums.macrumors.com/threads/apple-shareplay-audio-and-apple-tv.2330378/);
  [Apple Community echo](https://discussions.apple.com/thread/256083228);
  [Apple Community mic drop](https://discussions.apple.com/thread/255411270);
  [MacStories tvOS 17](https://www.macstories.net/reviews/tvos-17-the-macstories-review/4/);
  [AirPlay buffer](https://discussions.apple.com/thread/8053774).
- Echo: [Fora Soft on AEC](https://www.forasoft.com/learn/ai-for-video-engineering/articles-ai/echo-cancellation-aec-ai-hybrid-webrtc);
  [Switchboard on AEC3](https://switchboard-website-astro.pages.dev/hub/how-webrtc-aec3-works);
  [webrtcaec3.js](https://npmjs.com/package/@ennuicastr/webrtcaec3.js);
  [Portal TV feedback](https://portal.uservoice.com/forums/916228-portal-from-facebook/suggestions/39172813-add-option-to-use-portal-tv-inbuilt-speaker-as-def);
  [Sonos community on Portal](https://en.community.sonos.com/home-theater-228993/could-not-use-sony-tv-sound-keep-switching-back-to-sonos-soundbar-6858409).
- Delay: [ITU-T G.114](https://www.itu.int/ITU-T/recommendations/rec.aspx?rec=6254).

**Limits.** About 25 web searches. No official Google page confirms or denies WebRTC in a Web
Receiver, so §2.1 rests on community reports and our own receiver code. Several Stack Overflow
pages were read through mirrors or search snippets. Nothing here was measured on a device in this
round.
