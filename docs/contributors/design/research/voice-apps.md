> **ABOUTME:** What watch-party and VoIP apps do for voice beside a film, what browsers allow, and
> what users say about it. Gathered 2026-10-07 while the owner tested Watch Together's voice prototype.

# Voice for Watch Together: what VoIP and watch-party apps do, and what users say (2026-10-07)

Three research passes, combined:
1. Watch-party products.
2. How VoIP apps engineer voice, and what browsers allow.
3. What users like and hate about voice apps.

Every claim in parts 1–3 has a source URL; **[unverified]** marks what couldn't be confirmed.

Gaps:
- The web-search budget ran out partway through.
- Reddit and the app-store review pages refused the fetcher, so sentiment leans on Hacker News, Apple Community, Trustpilot and press.
- Brave's microphone permission wasn't researched.

## Synthesis: our four field findings, explained

| Field finding | Cause (sourced) | What others do |
| --- | --- | --- |
| Android: the film is barely audible and the side buttons stop controlling it | When the microphone opens with echo cancellation, Chrome sets `MODE_IN_COMMUNICATION`. Every output it then creates plays on the call stream, Android caps music at call volume + 3 dB, and the keys control call volume. Chrome's `echoCancellation: "remote-only"` is compiled out on Android and iOS. | Discord has had the same complaint since 2019 and only offers a Call/Media toggle. Nobody has solved it. |
| The partner is too quiet even at 400% | With processing off, nothing raises the sender's level. Chromium's own comment: capture without automatic gain control "results in quiet, sometimes silent, streams". | Apps fix the voice's level at the sender, then lower the competing sound at the receiver (SharePlay's "smart volume", Mumble, Discord's Attenuation). Users' most-asked fix is **separate film and voice sliders** (Rave added them). |
| Brave on Android refused the microphone | Not researched. | Google Meet's two-step prompt (its own question first, then the browser's) raised first-join grants by 14%. |
| Congested link: a 0.9 s jitter buffer, 4.3% concealed | Probably mostly our test: one 5 Mbit netem queue (up to ~2.4 s deep if the default limit is 1000 packets [unverified]) carries both films and both voices. Browsers let a page set only a *minimum* jitter buffer (`jitterBufferTarget`), never a maximum. Opus's in-band error correction, as browsers configure it, barely helps. | RED (sending the last frames again in each packet, RFC 2198) cut concealment from 18% to 0.7% at 20% loss in Jitsi's tests, and Chrome already offers it. Firefox's support is unconfirmed. |

On iOS (from WebKit's source; needs a device check):
- Opening the microphone switches Safari's audio session to PlayAndRecord/VideoChat.
- With echo cancellation on, Apple's voice processing already lowers our film while someone speaks, at the gentlest SharePlay-style level.
- With echo cancellation off, playback may be quieter.
- AirPods drop to call-quality Bluetooth (HFP) while the microphone is open. This is the most widely hated mobile behaviour across all voice apps, and no app avoids it.

## What users care about most

1. **Latency, which they don't recognise as latency.**
   - ITU-T G.114 counts under 150 ms mouth-to-ear as transparent.
   - The same people took 135 ms to hand over a turn face to face and 487 ms on Zoom.
   - People blame each other's silences, not the link.
2. **Volume balance between the film and the voices.** This is the complaint specific to watch parties, and it runs both ways.
3. **Echo from the one person on speakers.** Scener all but requires headphones. Hyperbeam claims echo cancellation by delivering the film as a WebRTC audio track, which the browser's canceller can hear [unverified].
4. **Over-aggressive noise suppression eating words**, and voice gates clipping speech.
5. **Friction at first contact beats quality:** accounts, installs, permission prompts, links that expire.
6. **Silent sync failure:** "my friend happily watches it play… with no warning I'm being left behind" (Rave).
7. **Products that dropped watch-together (Disney+, Prime, Plex) didn't fail on voice.** None of them had voice built in. Plex's remaining users are long-distance couples and family, the pair Cut A is built for. Shipping voice built in is a differentiator, and also where the audio complaints collect.

Drift tolerances others use:
- **Jellyfin SyncPlay:** changes speed for drift between 60 ms and 3 s, and seeks beyond 400 ms. Its code comments call speed changes flaky on Safari and Android Chrome.
- **Syncplay:** plays at 0.95× beyond 1.5 s behind, and seeks beyond 4–5 s.

## Options, ranked by value against effort (several touch frozen design: owner's call)

1. **Separate film and voice volumes, plus a limiter after the partner's gain.**
   - Low effort, and it answers the top complaint.
   - Caveat: iOS ignores a page's changes to video volume, so on iOS it's Apple's own lowering or routing the film through Web Audio.
2. **Android: software gain control (and gentle noise suppression) with echo cancellation off.** Chromium's source says this runs without call mode. Very low effort, and needs a device check (a VQ item).
3. **Prefer RED for voice.** A codec-ordering change, costing ~30–60 kb/s more while talking.
4. **Fix the congested test profile before tuning against it.** Give each browser its own link, as VQ24 on nuc13 already does, and check the queue limit.
5. **A "headphones / speakers" choice** that sets processing and explains the trade-offs, including Bluetooth's call quality.
6. **Our own question before the browser's microphone prompt**, and a recoverable denial.
7. **Lower the film slightly while the partner talks** (ducking): gentle, adjustable and switchable off.
8. **iOS: create the audio context after the microphone opens** (a workaround for an old WebKit volume-drop bug).
9. **Pace the film's segment downloads while voice is on**; mark voice priority "high".
10. **A custom jitter buffer with a hard cap**, only if 1–4 aren't enough. High effort.

---



---

## Watch-party products: how they mix voice and film, and what users say

Research date: 2026-10-07. Scope: co-watching products that put synced video together with voice or video chat, and how they handle film-versus-voice balance, phone call-mode routing, Bluetooth, open mic, sync and bad networks. The second half covers customer sentiment and why products were discontinued.

**Limits on the research.** The session's WebSearch budget ran out partway through (200 calls, shared across this session's agents). Reddit (JSON and HTML), Discord's help centre, Teleparty's support page and Kast's Zendesk all returned 403 to the fetcher. So:

- Discord's claims come from search-result snippets of its help pages. They are marked *(snippet)*.
- Reddit sentiment is mostly missing. App-store and forum sentiment comes from aggregator sites (justuseapp, appsupports, Trustpilot), Apple Community, the Plex forum, ResetEra and Apple's public review RSS.
- Anything not confirmed from a primary page is marked **[unverified]**.

Fetched data is cached under `/yoloai/cache/http/voip/` (`notes.md`, `gh/`, `appstore/`, `hyperbeam-*`).

---

### 1. Summary table

| Product | Voice/video built in? | Voice vs film mixing | Sync mechanism (where documented) | Status |
|---|---|---|---|---|
| Apple SharePlay (FaceTime) | Yes (FaceTime) | **Automatic ducking ("smart volume")**, no user mixer | AVPlaybackCoordinator: waits for every participant, "suspension" to leave the group | Active |
| Discord (Watch Together activity, Go Live) | Yes | **Attenuation** (ducking) slider on desktop; per-user volume; separate stream volume | Activity: YouTube embed synced over the Discord SDK. Go Live: a video stream, so no sync problem | Active |
| Teleparty | Premium only (voice/video) | Each user sets their own video volume; no ducking documented | Extension syncs playback controls | Active |
| Rave | Yes (voice, text) | **Separate video-volume and VoIP-volume sliders**, added after complaints | Server-synced; users report "left behind" | Active |
| Kast | Yes (voice, webcam) | The host's screen/tab audio is streamed; reviews say there is no usable volume control | Screen share (no sync needed) | Active, poorly reviewed |
| Scener | Yes (video + audio chat) | No mixing control documented; **headphones "strongly recommended"** | Extension sync | Active (per status checker) |
| Hearo | Yes (voice) | Content volume "too low" complaints; praised for no echo | Server-synced | App Store reviews 2019–2021 |
| Prime Video Watch Party | **Text chat only** | n/a | Synced | **Discontinued early 2024** |
| Disney+ GroupWatch | Reactions only **[unverified: emoji-only]** | n/a | Synced | **Discontinued 18 Sep 2023** |
| Hulu Watch Party | Text chat | n/a | Synced | Removed **[unverified date/reason]** |
| Plex Watch Together | **No chat at all**; Plex told users to bring their own | n/a | Synced, anyone can pause/seek | **Dropped from new apps Feb 2025**; web app only |
| Jellyfin SyncPlay | No | n/a | SpeedToSync 60 ms–3 s; SkipToSync ≥ 400 ms | Active |
| Syncplay (mpv/VLC) | No (text chat) | n/a | Slow to 0.95× when > 1.5 s ahead; rewind > 4 s; fast-forward > 5 s behind | Active |
| Watch2Gether | Webcam + text chat | Not documented | "Synchronized player" | Active |
| Metastream | No (text chat) | n/a | Synced | **Open source discontinued Aug 2024** |
| Hyperbeam | Yes | Film audio is a **WebRTC track**; claims "audio call echo cancellation" | Cloud browser streamed to all | Pivoted to API; consumer app at watch.hyperbeam.com |
| Zoom / Meet screen-share | Yes | The film is mixed into the call; volume is set at the sharer's source app | Single stream, no sync | Active |
| Messenger Watch Together | Yes (video call, 8 / Rooms 50) | Not documented | Synced | Launched Sep 2020; current status not checked **[unverified]** |

---

### 2. Product by product

#### 2.1 Apple SharePlay (FaceTime)

**Mixing: automatic ducking.** Apple's WWDC21 "Meet Group Activities" session says: *"With smart volume, when people speak up during playback, we'll automatically duck the audio of the content and bring it back up when appropriate. So people can communicate via voice, video, and text naturally, just like they would if they were sitting in the very same room."* (https://developer.apple.com/videos/play/wwdc2021/10183/). Apple's FaceTime guide adds: *"With smart volume, audio is adjusted automatically, so you can continue to chat while watching or listening."* (https://support.apple.com/guide/facetime/watch-and-listen-together-fctm725416ba/mac). Third-party guides say smart volume "works best with AirPods" and works with speakers too (https://allthings.how/how-to-use-facetime-shareplay-to-share-screen-on-iphone/). None of these sources shows a user-facing switch or mixer; one claim that turning off SharePlay disables it is **[unverified]**.

**Routing.** Because the platform owns both the call and the player, iOS can duck the media while keeping it on the media route. This is an OS privilege that a web page does not have. Users still complain about the coupling of call and media volume: *"if you turn down the volume for the media, it also turns down the call volume"* (search summary of Apple Community thread https://discussions.apple.com/thread/251838337; the fetched thread is actually about macOS Screen Sharing ducking a softphone call, *"drops to a barely audible level"*).

**Sync.** AVPlaybackCoordinator *"coordinates playback start across those devices, with the goal of no one missing any content."* It intercepts every rate or seek change. A participant can begin a **suspension** to leave the group, and ending it *"will rejoin the group's current time and rate."* The coordinator *"will often tell you to begin buffering, even if your device is already ready to go"* while it waits for others (https://developer.apple.com/videos/play/wwdc2021/10225/). In other words, the group waits for the slowest member rather than leaving that member behind. No numeric tolerance is published.

**Bad network.** Not documented beyond waiting at the start, as above.

#### 2.2 Discord (Watch Together activity, Go Live streams)

- **Attenuation (ducking).** Desktop Discord has an "Attenuation" slider that lowers *other applications* when someone speaks (or when you speak). 0% leaves them alone and 100% mutes them. *(snippet)* https://support.discord.com/hc/en-us/articles/206342888 ; https://www.technipages.com/discord-enable-voice-attenuation/. It is a well-known cause of "the video got quiet" complaints. Discord's own help article is titled "How do I stop Discord from lowering my volume when someone else is talking?", which shows users hit it often enough to need the article.
- **Per-user volume.** You can right-click a user and set their volume, up to 200% *(snippet; 200% figure from the cached notes in /yoloai/cache/http/voip/notes.md, support.discord.com/hc/en-us/community/posts/360031044891)*. Users ask for more than 200%, the same complaint as our (2).
- **Android call mode.** *(snippet, support.discord.com community "Keep Discord mobile from hogging the audio stream", https://support.discord.com/hc/en-us/community/posts/360037123891)*: *"Discord voice chat is currently classified by the phone as an actual call, and the 'call' volume slider controls the volume."* Joining in call mode attenuates other audio, *"mono and filtered audio … making it nearly impossible to play games or other audio."* Users have asked for a media-volume option since at least 2019. Discord's mitigation is an **Android "Audio Mode" setting with "Call" and "Communication"** options. Discord support (2019): *"If you're on Android could you try switching between Communication and Call Mode in your User Settings > Voice & Video"* (https://x.com/discord/status/1180248331794178048, text from a search snippet; the page returned 402). This is the same problem as our field finding (1). Even Discord did not make it go away; it exposed a toggle.
- **Bluetooth.** When a Bluetooth headset's mic is in use, quality drops. The common workaround is to untick "Phone calls" for the headset so it stays on A2DP, giving up its mic *(snippet, guidingtech / Samsung community)*. Cached notes also record the Spotify community thread "Spotify sound quality lowers while on Discord" (https://community.spotify.com/t5/Desktop-Mac/Spotify-Sound-Quality-lowers-while-on-Discord/td-p/4887161; body not retrievable).
- **Go Live.** Go Live is a video stream of the sharer's screen and audio, so viewers never drift apart. The film arrives as a stream inside the call; quality depends on the sharer's upload.

#### 2.3 Teleparty (formerly Netflix Party)

Voice and video chat are a **Premium** feature. Only Premium users can turn on camera or mic, but everyone receives them. Each user *"has complete control over their own audio and subtitle settings, which includes volume"* (https://treblab.com/blogs/news/what-is-teleparty, summarising https://www.teleparty.com/support, which returned 403 to the fetcher). No ducking is documented **[unverified]**.

#### 2.4 Rave

- **Separate sliders.** Rave announced: *"Control the video volume and voip (voice-chat) volume! When you're in a rave, tap the settings icon in the top left corner."* (https://www.facebook.com/Getrave/videos/1583008411892284/; undated).
- **Complaints before and around the sliders, collected from App Store reviews on aggregators:**
  - *"Voice calls are always louder than the movie. There's no way to fix it other than having both users log off the call, turn up the volume of the movie and then return to the call."* (https://appsupports.co/929775122/rave-watch-party/negative-reviews)
  - *"the volume of the mic in comparison to the video isn't balanced properly"*; *"The microphone is so loud and so echoey you cant even have it on while watching the video"*; *"The persons every move is sonified and LOUD. So you can't even hear the video or movie"* (https://justuseapp.com/en/app/929775122/rave-watch-party/reviews)
  - Bluetooth: *"major issues with bluetooth wireless headphones as they are disconnecting and connecting all the time"* (justuseapp); *"When you try to connect AirPods or any Bluetooth it doesn't work."* (appsupports)
  - Sync: *"my friend happily watches it play on their side with no warning I'm being left behind"*, *"slow, laggy and will not catch up"* (justuseapp); *"Rave keeps skipping frame by frame … When I'm on calls"* (appsupports).

  "Voice too loud relative to the film" is the inverse of our symptom, but it comes from the same cause. On phones, opening a voice session moves the media into the call's audio session, where it is attenuated **[inference; consistent with Discord's Android description above]**.
- Common Sense Media mentions only that "there's also the option for voice chat" (https://www.commonsensemedia.org/app-reviews/rave-watch-together).

#### 2.5 Kast

Kast is screen sharing (host tab or computer audio, plus webcam and voice). Its reviews are heavily negative:

- Echo, and voice-versus-film audio conflicts: in order for viewers to hear the movie *"the host has to set output to computer audio, and if voice chat is used at the same time, viewers can't hear what the host says"* (search snippet of https://www.trustpilot.com/review/kast.gg).
- Volume: *"There's no volume control, so I can't hear anything above a whisper"*; *"I keep trying to turn the volume down … but it doesn't go down"* (https://justuseapp.com/en/app/1467026423/kast-watch-together/reviews).
- Sync on mobile: *"The audio is so out of sync on mobile that it's not watchable."* (justuseapp).
- Battery: *"Kast somehow managed to chew up over 70% of my phone's battery in about 90 minutes."* (justuseapp).
- Reliability: *"Laggy as all hell, audio bugs out"*; *"I spend on average 15-20 minutes just trying to fix the damn audio"* (https://www.trustpilot.com/review/kast.gg?page=2).

#### 2.6 Scener

Scener has a video-chat sidebar during the film. Its FAQ: *"Everyone should wear headphones while their microphones are turned on – otherwise participants will hear feedback."* (https://www.scener.com/faq). Its tips page: *"Headphones are strongly recommended when on camera and unmuted. Otherwise, everyone watching will hear an echo."* The fallback offered is to *"mute each participant individually"*. On bad networks: *"On slow connections, the show will buffer and the video chat may disconnect."* (https://www.scener.com/watch-party-tips). The mobile app allows 4 on camera; rooms allow 10 with audio. A press review found large-audience chatter *"became less fun as the show played"* (https://www.seattlemet.com/arts-and-culture/2020/10/how-the-virtual-movie-theater-is-becoming-a-thing-seattle-scener).

#### 2.7 Hearo (extra: a voice-first watch app)

These are App Store reviews (Apple public review RSS, id 1385627895, cached in `/yoloai/cache/http/voip/appstore/`). The feed came back empty on a second fetch, so the quotes come from the first run's output.

- *"audio is too quiet even after i turn everything all the way up"* (1★, 2021-05-16)
- *"My only complaint is that the volume is insufficient. Even turned up all the way on the app and phone, dialogue is difficult to hear."* (4★, 2020-05-02)
- *"the volume of the streamed content being so low is a big issue"* (4★, 2019-07-14)
- *"a nasty little echo destroys our calls"* (4★, 2021-05-02)
- Praise: *"I love that there is no echo when I talk without headphones which I have encountered a lot thru Discord."* (5★, 2020-03-23)

This is the same symptom as our field finding (1): with voice on, the content is too quiet on the phone even at maximum. The likely cause, the iOS voice-processing audio session, is **[inference]**.

#### 2.8 Prime Video Watch Party: discontinued

It had **text chat only, no voice** (https://www.documentarytube.com/blog/how-to-watch-amazon-prime-video-with-friends-after-watch-party-was-discontinued/ via search snippet). It disappeared in early 2024 with no announcement; support pages were removed and *"Amazon did not officially acknowledge the removal"* (https://www.techtimes.com/articles/306472/20240708/amazon-discontinued-prime-video-watch-party-feature-users-noticed.htm). The Twitch Prime Video watch-party feature ended 2 April 2024 (https://alternativeto.net/news/2024/3/twitch-to-discontinue-prime-video-watch-party-feature-on-april-2). No reason was given.

#### 2.9 Disney+ GroupWatch: discontinued 18 Sep 2023

Disney support said: *"We're always working to deliver the best product experience on Disney+, and as part of recent updates, the GroupWatch feature was removed."* (https://www.howtogeek.com/disney-quietly-removed-its-groupwatch-feature/). The press speculated low use and a cost and profitability push alongside the October 2023 price rise (https://dapsmagic.com/2023/09/groupwatch-discontinued-on-disney/ ; https://www.kennythepirate.com/2023/09/19/disney-just-quietly-canceled-one-popular-feature-for-everyone/). There was no built-in voice. ResetEra users paired it with **Discord or FaceTime** for talking. One said *"it sucked and never synced"* (https://www.resetera.com/threads/disney-just-nuked-the-group-watch-feature.766988/).

#### 2.10 Hulu Watch Party

It launched in 2020 for ad-free subscribers with text chat (https://forums.macrumors.com/threads/hulus-watch-party-feature-rolling-out-to-all-users.2273081/). An SEO site claims it was removed in November 2021 because of "<3% adoption" and studio licensing (https://smartpartyprep.com/articles/does-hulu-have-watch-party). **[unverified; low-quality source, contradicts other snippets]**

#### 2.11 Plex Watch Together: dropped from new apps in Feb 2025

- **No built-in chat ever.** At launch (2020): *"users are asked to choose their favorite non-Plex app to chat—video, audio, or text"* (https://techcrunch.com/2020/05/28/plex-launches-a-co-watching-experience-for-its-on-demand-library-and-users-personal-media).
- **Reason given.** Plex spent *"two years [rebuilding] our apps from the ground up to boost our development speed"*, which meant *"ending support for some features that we've grown to love, like Watch Together"*. It stays in the web app (https://www.pcworld.com/article/2619590/plex-is-dropping-a-popular-feature-from-its-new-streaming-apps.html, 25 Feb 2025). Android Police reports a staff remark that the feature "isn't perfect" and is less needed after the COVID-era lockdowns (https://www.androidpolice.com/plex-wont-include-watch-together-feature-in-planned-overhaul/). No usage numbers were given.
- **User reaction** (https://forums.plex.tv/t/please-do-not-remove-watch-together/906929 ; https://forums.plex.tv/t/watch-together-going-away-in-app/906895):
  - *"Watch Together is my entire reason for sticking with Plex instead of just running Jellyfin."*
  - *"This was the only way I was able to watch with my dad who lived in the UK and only had a TV."*
  - *"I'm a LDR and this gives us the ability to do something together"*
  - *"My fiancé lives abroad...watching movies together is the main way we hang out digitally"*

  The core users are **couples and family at a distance**, which matches Orpheum's Cut A pair.

#### 2.12 Jellyfin SyncPlay (no voice)

From source code (`jellyfin-web/src/plugins/syncPlay/core/PlaybackCore.js`, cached in `/yoloai/cache/http/voip/gh/jf-web/`):

- **SpeedToSync.** Applies when the drift is between `minDelaySpeedToSync` = **60 ms** and `maxDelaySpeedToSync` = **3000 ms**. It changes the playback rate by `1 + diff/speedToSyncDuration` over **1000 ms**.
- **SkipToSync.** Seeks when the drift is ≥ **400 ms** (`minDelaySkipToSync`).
- **Known gaps, from the source's own TODOs:** *"SpeedToSync sounds bad on songs"*, *"SpeedToSync is failing on Safari (Mojave)"*, *"both SpeedToSync and SkipToSync seem to have a hard time keeping up on Android Chrome as well."* (https://github.com/jellyfin/jellyfin-web/blob/master/src/plugins/syncPlay/core/PlaybackCore.js)

#### 2.13 Syncplay (no voice)

From `syncplay/constants.py` (https://github.com/Syncplay/syncplay/blob/master/syncplay/constants.py):

- `SLOWDOWN_RATE = 0.95` above `DEFAULT_SLOWDOWN_KICKIN_THRESHOLD = 1.5` s
- `DEFAULT_REWIND_THRESHOLD = 4` s
- `DEFAULT_FASTFORWARD_THRESHOLD = 5` s
- `SEEK_THRESHOLD = 1`

Its tolerance is seconds, not milliseconds. That is acceptable only because voice runs over a separate app.

#### 2.14 Watch2Gether, Metastream, Hyperbeam

- **Watch2Gether** offers *"Webcam support"*, integrated text chat and a *"Synchronized player for video and audio"* (https://w2g.tv/en/). There is no voice or mixing documentation.
- **Metastream** has text chat only. *"As of August 2024, Metastream has discontinued publishing under open source and is no longer accepting contributions."* (https://github.com/samuelmaddock/metastream)
- **Hyperbeam** runs a cloud browser streamed to everyone over WebRTC. Its client SDK exposes `audioTrackCb`: *"Callback called with a MediaStreamTrack of the virtual computer's audio stream"* (https://docs.hyperbeam.com/client-sdk/javascript/reference.md). Its homepage says it *"Supports audio call echo cancellation … Removing audio call echo when embedding apps is non-trivial—so we did it for you."* (https://hyperbeam.com/). **Implication [inference]:** because the film audio is a WebRTC remote track rather than a `<video>` element or Web Audio output, the browser's AEC may be able to use it as an echo reference. The mechanism is not documented. Hyperbeam has pivoted to a "Virtual Computer API"; the consumer app lives at watch.hyperbeam.com.

#### 2.15 Zoom / Google Meet / Messenger

- **Zoom "Share computer sound"** mixes the film into the meeting audio. Volume is set only at the source: *"you must adjust the volume from the application playing the sound or the overall device volume directly"*. A participant on both phone and computer gets the audio twice and must mute one to avoid feedback (https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0063608). Mono or "Stereo (high fidelity)" can be chosen. Viewers cannot balance film against voice themselves.
- **Google Meet** has a Co-Watching API (`CoWatchingClient`, `notifyPlayoutRate()`) for add-ons. No audio, ducking or tolerance guidance is published there (https://developers.google.com/workspace/meet/add-ons/guides/use-CoWatchingAPI).
- **Messenger Watch Together** (Sep 2020) puts synced video inside a video call: up to 8 people in calls, 50 in Rooms, on mobile (https://about.fb.com/news/2020/09/introducing-watch-together-on-messenger/). No audio-mixing details are published.

---

### 3. Customer sentiment, by theme

1. **Voice-versus-film balance is the top audio complaint, in both directions.** Users say voice is too loud for the film: Rave (*"Voice calls are always louder than the movie"*) and SharePlay (*"The volume is so loud it is hard to have a conversation during media. Please add volume mixer!"*, https://discussions.apple.com/thread/254244229). Users also say content is too quiet with voice on: Hearo (*"volume is insufficient. Even turned up all the way"*), FaceTime (*"the sound is significantly lower (even with paired bluetooth headphones and the other person on mute)"*, https://discussions.apple.com/thread/251676871) and Discord on Android (call mode). The fix users ask for, over and over, is **a mixer: separate film and voice volume**. Rave shipped exactly that.
2. **Ducking is valued in principle, but people complain when it's automatic and can't be changed.** Apple sells smart volume as a headline feature. Discord users look up how to turn attenuation off (support article title). Apple Community users want *"control center control media volume and volume button control the call volume"* separately (251676871). No source found praising or criticising SharePlay smart volume in detail; Reddit was unreachable **[gap]**.
3. **Echo is solved by telling users to wear headphones.** Scener's FAQ and tips require headphones, Kast reviews complain of echo, and Hearo was praised specifically for "no echo … without headphones".
4. **Bluetooth.** Users complain about headset disconnects (Rave) and about quality dropping when the mic opens (HFP). Discord's workaround is to disable the headset's call profile, which costs the headset mic.
5. **Sync.** "Left behind with no warning" (Rave), "never synced" (GroupWatch on ResetEra), "audio out of sync on mobile" (Kast). Users want to be told when they fall behind, not just have it silently fail.
6. **Battery.** Kast: 70% in 90 minutes. This comes from a screen-share architecture, which is heavier than local playback.
7. **Discontinuations: no product cited audio or voice as the reason.** Every corporate removal (GroupWatch 2023, Prime 2024, Plex 2025, Hulu [unverified]) was silent or vague. The common threads are post-COVID usage decline and the cost of maintaining it on every device platform during a rewrite (Plex). Plex's remaining users are couples and family at a distance who called it their reason to pay.
8. **The big platforms mostly don't ship voice.** Prime, Hulu, GroupWatch, Plex, Jellyfin, Syncplay, Metastream and W2G (text and webcam) leave voice to Discord or FaceTime. Users then run two apps, which brings separate volume controls, call-mode attenuation of the film, and echo between the two. The products that do ship voice (Rave, Kast, Scener, Hearo) collect most of the audio complaints.

---

### 4. Takeaways for Orpheum's design (inference, drawn from the above)

- **Ship a two-slider mixer (film / voice), not only per-partner gain.** It is the most requested fix and the one Rave shipped. Users must be able to raise the film independently of the call.
- **If we duck, make it gentle and user-adjustable, with an off position.** That is SharePlay's model plus Discord's slider. This interacts with AEC, as already covered in watch-together.md.
- **Phone call mode is a known industry pain.** Discord exposes an Android "Call / Communication" audio-mode toggle rather than solving it. Hearo reviews and FaceTime complaints show the same media attenuation on iOS. A page can't choose the Android audio mode directly, so the remaining options are what `getUserMedia` constraints trigger and offering voice without processing (headphones).
- **Echo: everyone falls back to "use headphones".** Hyperbeam's approach of delivering the film as a WebRTC audio track, so the browser's AEC sees it, deserves a quick experiment **[unverified mechanism]**.
- **Tell users when they fall behind.** Use Jellyfin-style rate correction from about 60 ms and a seek from 400 ms to 3 s, versus Syncplay's 1.5 s. Speed changes are reported broken on Safari and Android Chrome, so a seek is the safe fallback there.
- **On bad networks, Scener lets the chat drop while the film buffers.** SharePlay instead holds everyone until all are ready. No product found documents degrading voice quality gracefully.


---

## How VoIP apps engineer voice, and what browsers allow: research for Watch Together voice

Date: 2026-10-07. Scope: web research only. Raw copies of the pages are cached under
`/yoloai/cache/http/voip/` (each `<hash>.url` names its source). A sparse WebKit checkout
(commit b7161cda, 2026-10-06) is in `/yoloai/cache/webkit-src/`.

Labels used below:
- **[source]**: read in the cited document or source file.
- **[inferred]**: my reading of the cited code or docs. Nobody has confirmed it on a device.
- **[unverified]**: from memory or general knowledge; I could not fetch a source for it this
  session (the session's web-search budget ran out partway through).

---

### 0. Summary

1. **Chrome on Android is the cause of finding (1), and it is in Chromium's source.** If the
   microphone is opened with a platform echo-cancelling effect, `AudioManagerAndroid` calls
   `AudioManager.setMode(MODE_IN_COMMUNICATION)`. Every low-latency output stream created after
   that gets `AAUDIO_USAGE_VOICE_COMMUNICATION` (the call stream). Android's audio policy then
   caps the music stream at the voice-call volume plus 3 dB, and the volume keys control
   `STREAM_VOICE_CALL`. When all processing is off, the effect mask is `NO_EFFECTS` and Chrome
   leaves the mode alone. Chrome's newer `echoCancellation: "remote-only"` mode would run
   software AEC without call mode, but it is compiled out on Android and iOS.
2. **The partner's voice is quiet (finding 2) mainly because the sender has no gain
   control.** With `echoCancellation:false`, Chrome on Android opens the mic with the GENERIC
   preset. A comment in Chromium's own code says unprocessed capture without AGC "results in
   quiet, sometimes silent, streams". Real apps fix loudness at the sender with AGC, then duck
   the other audio at the receiver: Apple's voice processing and FaceTime SharePlay, Mumble's
   "attenuation", and Windows communications ducking, which Discord works around. Receiver gain
   of 4× (+12 dB) on an un-normalised mic cannot catch up with a film mix.
3. **Loss resilience:** Opus in-band FEC as WebRTC configures it barely helps. RED (RFC 2198),
   used by Jitsi, recovers most random loss: at 20% loss, concealment fell from 18% to 0.7%
   with distance 2. Chrome already offers `red/48000/2` in every SDP. It is used only if the app
   puts it first with `setCodecPreferences`.
4. **Jitter buffer:** browsers expose only a *minimum* (`jitterBufferTarget`, 0–4000 ms).
   NetEQ's ceiling cannot be set from JS; `jitterBufferMaximumDelay` is still only a proposal.
   The 0.9 s in our test is far more likely queueing delay (bufferbloat) than jitter in the
   normal sense. The film's HTTP traffic and the voice share one 5 Mbit netem queue.
5. **iOS Safari:** any capture puts WebKit's session into `PlayAndRecord` + `VideoChat`. With
   echo cancellation on, WebKit uses Apple's voice-processing I/O with *advanced ducking at
   level Min*. That is the SharePlay-style ducking: other audio, including our `<video>`, is
   lowered while someone speaks, and Min keeps it as loud as Apple allows. On a headset that
   supports both profiles, HFP takes priority over A2DP.

---

### 1. The field findings, mapped to mechanisms

| Finding | Mechanism | Evidence |
|---|---|---|
| (1) Android: EC/NS/AGC on → film barely audible, side keys control call volume | Chrome sets `MODE_IN_COMMUNICATION` when the input stream carries a platform effect (ECHO_CANCELLER). Output streams created afterwards are tagged `USAGE_VOICE_COMMUNICATION`. AOSP caps music volume to voice volume + 3 dB while in call. Volume keys follow `STREAM_VOICE_CALL` while in communication | [source] Chromium `audio_manager_android.cc`, `AudioManagerAndroid.java`; AOSP `AudioPolicyManager.cpp`, `policy.h`, `AudioService.java` (§2.1) |
| (2) Partner voice far too quiet even at 400% | Sender-side AGC is off, and the GENERIC input preset gives a low level (Chromium says so in a comment). Receiver gain then has to make up 20–30 dB, and the film is mixed for loudness peaks | [source] Chromium `aaudio_stream_wrapper.cc` comment; WebRTC AGC defaults (§5) |
| (3) Brave/Android mic permission trouble | Not researched; no source fetched | [unverified] |
| (4) 0.9 s jitter buffer, 4.3% concealment on the congested link | NetEQ grows its target with the delay spread it sees over ~2 s windows, and JS can't cap it. Under a 5 Mbit `rate` limit, netem queues the film's TCP bursts in front of the voice packets. If the voice is relayed, it may cross the shaped loopback twice | [source] NetEQ docs, webrtcHacks, MDN, W3C issue #199, `tc-netem(8)`; test profile in `web/e2e/acceptance-server.ts` (§4.3) |

---

### 2. What browsers do when the microphone opens

#### 2.1 Chrome on Android [source]

- **Mode switch.** `AudioManagerAndroid::MakeAudioInputStream` contains this comment and the
  branch after it: *"By default, the audio manager for Android creates streams intended for
  real-time VoIP sessions and therefore sets the audio mode to MODE_IN_COMMUNICATION. However,
  the user might have asked for a special mode where all audio input processing is disabled,
  and if that is the case we avoid changing the mode."*
  `if (params.effects() != AudioParameters::NO_EFFECTS) { … SetCommunicationAudioModeOn(true); }`.
  The Java side calls `mAudioManager.setMode(AudioManager.MODE_IN_COMMUNICATION)` and restores
  `MODE_NORMAL` when the last input stream closes. On Android, desktop-class devices with
  `kAAudioPerStreamDeviceSelection` skip this.
  <https://chromium.googlesource.com/chromium/src/+/main/media/audio/android/audio_manager_android.cc>,
  <https://chromium.googlesource.com/chromium/src/+/main/media/base/android/java/src/org/chromium/media/AudioManagerAndroid.java>
- **Output streams follow the mode.** `MakeLowLatencyOutputStream` uses
  `communication_mode_is_on_ ? AAUDIO_USAGE_VOICE_COMMUNICATION : AAUDIO_USAGE_MEDIA`. OpenSL ES
  uses `SL_ANDROID_STREAM_VOICE` vs `…_MEDIA` in the same way. *Streams created before the mode
  switch keep their usage* [inferred: the usage is fixed when the stream is constructed]. So the
  film's audio sink and the Web Audio context can land on different Android streams depending
  on the order in which they were created. (Same source file.)
- **Input preset.** AAudio input uses `AAUDIO_INPUT_PRESET_VOICE_COMMUNICATION` when
  `ECHO_CANCELLER` is set, and `GENERIC` otherwise. The code says so explicitly: *"We do not use
  AAUDIO_INPUT_PRESET_UNPROCESSED … because the lack of automatic gain control results in
  quiet, sometimes silent, streams."*
  <https://chromium.googlesource.com/chromium/src/+/main/media/audio/android/aaudio_stream_wrapper.cc>
- **What sets the effect bit.** The device advertises `ECHO_CANCELLER` when Android's
  `AcousticEchoCanceler` is available. Blink keeps the bit only when the selected canceller is
  "PlatformProvided", which is the default answer to `echoCancellation: true`. It strips AEC and
  AGC platform bits otherwise. Android only advertises the AEC bit, so
  **`echoCancellation:false` with `autoGainControl:true, noiseSuppression:true` should run
  WebRTC's *software* AGC2/NS in the renderer without entering call mode** [inferred from
  `media_stream_audio_processing_layout.cc` → `ConfigureEchoCancellationEffects`,
  `ComputeWebrtcProcessingSettings`; device-verify].
  <https://chromium.googlesource.com/chromium/src/+/main/third_party/blink/renderer/modules/mediastream/media_stream_audio_processing_layout.cc>,
  <https://chromium.googlesource.com/chromium/src/+/main/third_party/blink/renderer/platform/mediastream/media_stream_audio_processor_options.cc>
- **`echoCancellation: "remote-only"` / `"all"`.** These are standardised in mediacapture-main:
  `remote-only` = "remove the sound from incoming audio MediaStreamTracks sourced from WebRTC
  RTCPeerConnections". Chrome has them enabled (`GetUserMediaEchoCancellationModes: stable`),
  **but** `// kRemoteOnly is not supported on mobile platforms. #if !BUILDFLAG(IS_ANDROID) &&
  !BUILDFLAG(IS_IOS)`. On Android the only options are platform AEC (call mode) or none.
  <https://chromium.googlesource.com/chromium/src/+/main/third_party/blink/renderer/modules/mediastream/media_stream_constraints_util_audio.cc>,
  <https://chromium.googlesource.com/chromium/src/+/main/third_party/blink/renderer/platform/runtime_enabled_features.json5>,
  <https://www.w3.org/TR/mediacapture-streams/>
- **What Android then does.**
  - `is_state_in_call()` is true for `MODE_IN_CALL` *and* `MODE_IN_COMMUNICATION`
    (<https://android.googlesource.com/platform/frameworks/av/+/refs/heads/main/services/audiopolicy/common/include/policy.h>).
  - `AudioPolicyManager::computeVolume`: *"in-call: always cap volume by voice volume + some low
    headroom"*. It caps the music, alarm, notification and system sources at voice volume +
    `IN_CALL_EARPIECE_HEADROOM_DB = 3`
    (<https://android.googlesource.com/platform/frameworks/av/+/refs/heads/main/services/audiopolicy/managerdefault/AudioPolicyManager.cpp>).
  - `AudioService.getActiveStreamType`: when `isInCommunication()`, the volume keys get
    `STREAM_VOICE_CALL`, or `STREAM_BLUETOOTH_SCO` on SCO
    (<https://android.googlesource.com/platform/frameworks/base/+/refs/heads/main/services/core/java/com/android/server/audio/AudioService.java>).

  Together these explain finding (1) exactly.
- **Hardware NS.** On desktop, Chrome turns *off* hardware noise suppression while an
  `echoCancellation` stream is open and leaves it alone otherwise.
  <https://developer.chrome.com/blog/disabling-hardware-noise-suppression>

#### 2.2 Chrome desktop: what the echo canceller's reference contains [source]

- `kChromeWideEchoCancellation` (enabled by default): *"If echo cancellation for a mic signal is
  requested, mix and cancel all audio playback going to a specific output device in the audio
  service."* It is supported on Windows, macOS and Linux (`chrome_wide_echo_cancellation_supported
  = is_win || is_mac || is_linux`). **So on desktop, the film's `<video>` audio is in the AEC
  reference.**
- `kSystemLoopbackAsAecReference` (Windows and macOS, enabled by default) uses system loopback,
  "to be able to cancel echo from all audio processes and not only audio from Chrome". It adds
  170 ms of capture delay.
- Sources: <https://chromium.googlesource.com/chromium/src/+/main/media/base/media_switches.cc>,
  <https://chromium.googlesource.com/chromium/src/+/main/media/media_options.gni>.
- On Android, the canceller is the platform's (VOICE_COMMUNICATION preset). Whether the vendor
  AEC uses *media*-usage playback as its reference is device-dependent [unverified].

#### 2.3 Safari / WebKit on iOS [source unless marked]

- **Category selection** (`MediaSessionManagerCocoa::updateSessionState`):
  - If anything is capturing, or audio is playing while the category is already PlayAndRecord →
    `PlayAndRecord`, mode `VideoChat`.
  - Audible video → `MediaPlayback`.
  - Only Web Audio → `Ambient`.
  - The buffer size drops to about 20 ms while capturing.

  (`/yoloai/cache/webkit-src/…/platform/audio/cocoa/MediaSessionManagerCocoa.mm`;
  <https://github.com/WebKit/WebKit/blob/main/Source/WebCore/platform/audio/cocoa/MediaSessionManagerCocoa.mm>)
- **PlayAndRecord options** (`AudioSessionIOS::setCategory`): `AllowBluetooth | AllowBluetoothA2DP
  | AllowAirPlay`, plus `DefaultToSpeaker` unless the receiver is the preferred speaker. Mode
  `VideoChat` becomes `VoiceChat` when the receiver (earpiece) is preferred.
  <https://github.com/WebKit/WebKit/blob/main/Source/WebCore/platform/audio/ios/AudioSessionIOS.mm>
- **Echo cancellation = Apple VPIO with advanced ducking at Min.** `CoreAudioCaptureUnit` uses
  the voice-processing I/O unit when `echoCancellation` is on. When it does, it sets
  `AUVoiceIOOtherAudioDuckingConfiguration { true, kAUVoiceIOOtherAudioDuckingLevelMin }`.
  (`Source/WebCore/platform/mediastream/cocoa/CoreAudioCaptureUnit.cpp` lines ~334–369.)
- **What that ducking means** (WWDC23 "What's new in voice processing"):
  - All audio other than the voice stream is "other audio" and gets ducked.
  - "Advanced ducking … applies more ducking when user from either side is talking, and it
    reduces ducking when neither side is talking. This is very similar to the ducking in
    FaceTime SharePlay."
  - Min = "if you want other audio volume to be as loud as possible".

  <https://developer.apple.com/videos/play/wwdc2023/10235/>

  Our `<video>` is "other audio", so with EC on, iOS lowers the film around speech by itself
  [inferred].
- **Chat mode without voice processing plays quieter.** Apple's docs for
  `AVAudioSession.Mode.videoChat`/`voiceChat`: apps in a chat mode that don't use
  VoiceProcessingIO get reduced processing, which "disables dynamic processing on input and
  output, which results in a lower playback level". WebKit keeps mode VideoChat even when EC is
  off, so **turning EC off on iOS may lower playback level** [inferred; device-verify].
  <https://developer.apple.com/documentation/avfaudio/avaudiosession/mode-swift.struct/videochat>
- **Bluetooth.** "If this option and the allowBluetooth option are both set, when a single
  device supports both HFP and A2DP, the system gives hands-free ports a higher priority." So
  AirPods drop to HFP while the mic is open.
  <https://developer.apple.com/documentation/avfaudio/avaudiosession/categoryoptions-swift.struct/allowbluetootha2dp>
- **Known WebKit bugs.**
  - Bug 218012: audio volume drops considerably once mic permission is granted (iOS 13–14).
    Workaround from the WebKit engineer: create/resume the AudioContext *after* capture starts,
    or suspend and resume it during capture. "Resolved: configuration changed".
    <https://bugs.webkit.org/show_bug.cgi?id=218012>
  - Bug 230902: iOS 15 routed WebRTC audio to the earpiece / too quietly. The workaround was
    playing remote audio through `AudioContext` + gain instead of a media element. Partial fix
    in 15.2; youenn: "In iOS 15.4 beta, no workaround should be needed". A 2025 comment reports
    it gone on iOS 17/18/26.
    <https://bugs.webkit.org/show_bug.cgi?id=230902>
- **`navigator.audioSession`** (W3C Audio Session, editors Apple and Mozilla). Types are
  `playback | transient | transient-solo | ambient | play-and-record | auto`. WebKit has
  `DOMAudioSessionEnabled` on by default ("mature"); the type sets WebKit's category override.
  But `updateSessionState` still forces mode `VideoChat` for `PlayAndRecord`, so it can't select
  a non-chat mode while capturing.
  <https://w3c.github.io/audio-session/>,
  `/yoloai/cache/http/webkit/UnifiedWebPreferences.yaml`,
  <https://github.com/WebKit/WebKit/blob/main/Source/WebCore/Modules/audiosession/DOMAudioSession.cpp>
- iOS Safari ignores writes to `HTMLMediaElement.volume` (volume is under the user's physical
  control) [unverified; long-standing Apple guidance]. So app-side ducking of the film on iOS
  would need the film's audio routed through Web Audio.

#### 2.4 Firefox

- Firefox's AEC reference and its Android audio-mode behaviour were not researched from source
  this session [unverified]. The acceptance test runs Firefox as the guest, so the same
  questions apply there.

#### 2.5 Bluetooth: HFP/HSP vs A2DP, and LE Audio [source]

- Classic Bluetooth: opening a BT mic moves the headset from A2DP (stereo, output-only) to HFP
  (mono, mSBC wideband at best). Android's LE Audio overview: "Bluetooth classic lowers audio
  quality when using Bluetooth microphones. With BLE Audio, input and output sampling can reach
  32 kHz". LC3 replaces SBC (A2DP) and mSBC (HFP).
  <https://developer.android.com/develop/connectivity/bluetooth/ble-audio/overview>
- Android 13+ VoIP apps should use `setCommunicationDevice()` instead of `startBluetoothSco()`.
  <https://developer.android.com/develop/connectivity/bluetooth/ble-audio/audio-manager>
  Chrome does this through its `CommunicationDeviceSelector` / SCO listener (Java file above).
- [inferred] On Chrome/Android with EC off (GENERIC preset, MODE_NORMAL), capture likely uses
  the phone's mic and output stays on A2DP. With EC on, routing goes to the communication device,
  which may be SCO. Device-verify both.

---

### 3. How the major apps do it

| App | Documented engineering | Source |
|---|---|---|
| **Discord** | Browser: the browser's WebRTC. Native: one C++ engine on libwebrtc so they can "circumvent auto-ducking behavior of the default communications device on Windows", run their own volume control, VAD, cut bandwidth in silence, and add priority-speaker metadata | <https://discord.com/blog/how-discord-handles-two-and-half-million-concurrent-voice-users-using-webrtc> |
| **Zoom** | Reverse-engineered: SILK with LBRR carrying up to **two** previous frames, so packets are 150–300 bytes. Nothing official found | <https://webrtchacks.com/red-improving-audio-quality-with-redundancy/> |
| **Google Meet / Duo** | Lyra neural codec at 3 kbps (≈ Opus 8 kbps). WaveNetEQ generative PLC in Duo replaces NetEQ PLC, because signal-processing PLC "does not sound good" for gaps ≥ 60 ms. Meet's own use of RED: [unverified] | <https://research.google/blog/lyra-a-new-very-low-bitrate-codec-for-speech-compression/>, <https://research.google/blog/improving-audio-quality-in-duo-with-waveneteq/> |
| **WhatsApp / Messenger / Instagram (Meta)** | Opus everywhere until MLow (CELP-based, 10% less CPU than Opus, POLQA 3.9 vs 1.89 at 6 kbps). "Opus … needs a minimum of 19 kbps to encode any inband FEC at 10 percent packet loss". MLow frees bits for FEC at low rates | <https://engineering.fb.com/2024/06/13/web/mlow-metas-low-bitrate-audio-codec/> |
| **Microsoft Teams / Skype** | Satin: AI codec, super-wideband from 6 kbps, each packet independently decodable, extra bitrate spent on redundancy for burst loss. QoS: DSCP marking plus port ACLs, voice prioritised | <https://techcommunity.microsoft.com/blog/microsoftteamsblog/satin-microsoft%E2%80%99s-latest-ai-powered-audio-codec-for-real-time-communications/2141382>, <https://learn.microsoft.com/en-us/microsoftteams/qos-in-teams> |
| **FaceTime / SharePlay** | Voice processing ducks "other audio". SharePlay uses *advanced* ducking: media loud when nobody talks, lowered as soon as someone talks | WWDC23 10235 (above) |
| **Signal** | RingRTC, built on WebRTC; no audio-tuning write-up found | <https://github.com/signalapp/ringrtc> |
| **Mumble** | Opus with user-selectable bitrate and frames per packet (10 ms risky, 20 ms recommended). Adjustable jitter buffer. "Maximum amplification" input normalisation. AEC needed with speakers, "not so much with headphones". **Attenuation**: lowers *other applications* while others talk, "you being able to hear others talking better over other audio playback" | <https://www.mumble.info/documentation/user/audio-settings/> |
| **TeamSpeak** | No engineering documentation found [unverified] | — |

Common pattern: **the voice is normalised at the sender** (AGC / max amplification), and **the
competing audio is ducked at the receiver** (Apple, SharePlay, Mumble, the Windows
communications device). Nobody relies on receiver-side boost of an un-normalised voice.

---

### 4. Codecs, loss resilience and the jitter buffer

#### 4.1 Opus settings [source]

- Xiph's recommendation for VoIP: mono, 10–24 kb/s ("24 Kb/s should give fullband").
  <https://wiki.xiph.org/Opus_Recommended_Settings>
- Opus RTP fmtp (RFC 7587): `useinbandfec`, `usedtx`, `maxaveragebitrate`, `stereo`, `cbr`,
  `maxplaybackrate`. <https://www.rfc-editor.org/rfc/rfc7587.txt>
- WebRTC's in-band FEC is a weak fix:
  - It is enabled only once loss is detected and capped around 25%.
  - It is *subtracted* from the target bitrate.
  - It protects only the previous frame. At 20% loss: no RED 18% concealment, FEC disabled 20%.

  <https://webrtchacks.com/red-improving-audio-quality-with-redundancy/>
- libopus 1.5 adds deep PLC (decoder complexity ≥ 5) and **DRED**: up to 1 s of redundancy for
  about 12–32 kb/s extra, aimed at burst loss. Both are disabled by default, need build and
  runtime switches, and are not available in browsers' WebRTC [inferred].
  <https://opus-codec.org/release/stable/2024/04/16/libopus-1_5.html>

#### 4.2 RED (RFC 2198) in browsers [source]

- Jitsi and Philipp Hancke landed Opus RED in libwebrtc. Concealment rates measured:

  | Loss | No RED | RED distance 1 | RED distance 2 |
  |---|---|---|---|
  | 20% | 18% | 4% | 0.7% |
  | 60% | 60% | 32% | 18% |

  Costs: about 2–3× the audio bitrate (30 → 60/90 kb/s), reduced by DTX/VAD. Enabled by moving
  `red/48000/2` ahead of opus with `RTCRtpTransceiver.setCodecPreferences`.
  <https://webrtchacks.com/red-improving-audio-quality-with-redundancy/>
- Current libwebrtc *always* adds a `red` entry after every Opus codec it offers
  (`webrtc_voice_engine.cc`), so this is a codec-ordering choice, not a flag.
  <https://webrtc.googlesource.com/src/+/refs/heads/main/media/engine/webrtc_voice_engine.cc>
  Safari is libwebrtc-based, so it probably behaves the same [inferred]. Firefox's RED support:
  [unverified].
- `concealedSamples / totalSamplesReceived` from `getStats()` is the metric webrtcHacks used.
  It is the same one our acceptance test uses.

#### 4.3 NetEQ and how far its delay can be bounded [source]

- NetEQ is an adaptive buffer. It decodes 10 ms per `GetAudio`, time-stretches
  (accelerate/preemptive expand) towards a target, and runs PLC (expand) when the next packet
  is missing. It also handles RED/FEC splitting and NACK.
  <https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/neteq/g3doc/index.md>
- How the target is set (since 2022):
  - "Relative delay" against the fastest packet in a 2 s window.
  - An underrun histogram (forget factor 0.983, 0.95 quantile) and a reorder optimiser
    (cost = delay + 20 ms × loss%). The larger of the two wins.
  - Shrinking from 140 ms to 20 ms took ~3.5 s in their example.

  <https://webrtchacks.com/how-webrtcs-neteq-jitter-buffer-provides-smooth-audio/>
- **Browser control is a minimum only.** `RTCRtpReceiver.jitterBufferTarget` (0–4000 ms)
  "influences … but does not directly set" the target.
  <https://developer.mozilla.org/en-US/docs/Web/API/RTCRtpReceiver/jitterBufferTarget>
  The W3C issue proposing `jitterBufferMaximumDelay` says: "upper bound is not controllable
  from application layer". <https://github.com/w3c/webrtc-extensions/issues/199>
  Native libwebrtc has `audio_jitter_buffer_max_packets = 200` and
  `audio_jitter_buffer_fast_accelerate`, not exposed to JS.
  <https://webrtc.googlesource.com/src/+/refs/heads/main/media/base/media_config.h>
- **Reading the 0.9 s / 4.3% result** [inferred]. The congested profile is
  `delay 150ms 50ms loss 3% rate 5mbit` on `lo`, "applied to every packet"
  (`web/e2e/acceptance-server.ts`).
  - `rate 5mbit` makes netem a 5 Mbit bottleneck *queue* shared by the film's HTTP segment
    bursts and the voice. Netem holds up to `limit` packets ("Limits the maximum number of
    packets the qdisc may hold"; <https://man7.org/linux/man-pages/man8/tc-netem.8.html>). The
    default is 1000 [unverified], which is ~2.4 s at 5 Mbit with full-size packets. That is
    classic bufferbloat, and NetEQ's relative-delay estimator turns it into a large target.
  - If a voice packet crosses `lo` twice (browser → embedded TURN → browser), it sees about
    2×150 ms base delay and 1−0.97² ≈ 5.9% loss. That alone breaks a 300 ms / 2% bound. Check
    which path the test's call takes.
  - With ±50 ms jitter per packet and 20 ms packet spacing, netem may reorder unless `rate`
    serialises the queue [unverified]. Reordered packets push NetEQ's target up further.

#### 4.4 Bitrate and QoS sharing between film and voice [source]

- RFC 8837 DSCP for WebRTC: audio at priority medium/high = **EF (46)**, low = DF, very-low =
  LE. Interactive video uses AF41/42/43. "Even so, there is a benefit to marking traffic even if
  it only benefits the first few hops." <https://www.rfc-editor.org/rfc/rfc8837.txt>
- RFC 8325: on Wi-Fi, EF should map to UP 6, the **Voice access category (AC_VO)**. This is
  where marking helps in homes. <https://www.rfc-editor.org/rfc/rfc8325.txt>
- W3C webrtc-priority: `RTCRtpEncodingParameters.priority` (local bandwidth/queueing) and
  `networkPriority` (DSCP only). <https://w3c.github.io/webrtc-priority/>
- Chrome maps `networkPriority` into libwebrtc (`rtc_rtp_sender.cc`), and libwebrtc's
  `enable_dscp` now defaults to true.
  <https://chromium.googlesource.com/chromium/src/+/main/third_party/blink/renderer/modules/peerconnection/rtc_rtp_sender.cc>,
  <https://webrtc.googlesource.com/src/+/refs/heads/main/media/base/media_config.h>
  Whether the OS actually applies the mark (Windows, Android) is [unverified].
- `priority` is supported in Chrome 69+, Firefox 46+, Safari 11+ (MDN BCD).
- Teams' guidance: QoS first, then bandwidth; QoS must be consistent across the whole path.
  <https://learn.microsoft.com/en-us/microsoftteams/qos-in-teams>
- [inferred] WebRTC congestion control can't slow the *film's* TCP/HTTP traffic. The only
  sender we fully control for the film is our own fetch scheduler.

---

### 5. Loudness: making a remote voice audible over a film

- **Sender AGC is how apps do it.** WebRTC AGC2 adaptive digital defaults: `headroom_db 5`,
  `max_gain_db 50`, `initial_gain_db 15`, `max_gain_change_db_per_second 6`. AGC1 targets
  −3 dBFS with 9 dB compression gain. It raises quiet talkers by tens of dB before encoding, so
  the receiver never has to.
  <https://webrtc.googlesource.com/src/+/refs/heads/main/api/audio/audio_processing.h>
  Mumble's "maximum amplification" does the same job.
- **Receiver gain alone clips.** Web Audio's output is limited to ±1.0 at the destination, so a
  `GainNode` of 4 on a voice peaking near full scale will clip [unverified about the exact
  clipping behaviour per browser]. Use a `DynamicsCompressorNode` after the gain as a
  compressor/limiter (threshold, knee, ratio, attack, release).
  <https://developer.mozilla.org/en-US/docs/Web/API/DynamicsCompressorNode>
- **Ducking the competing audio is the standard technique.**
  - Apple VPIO ducks other audio, with advanced/SharePlay-style speech-driven ducking.
  - Mumble attenuates other applications.
  - Windows ducks on the communications device (Discord fights it because games need their own
    audio).

  In a browser we can duck our *own* film by lowering `video.volume` while partner VAD is active
  (an `AnalyserNode` on the received track). Not on iOS: see §2.3.
- Android in-call: music is capped at voice volume + 3 dB (§2.1), which is OS-imposed "ducking"
  of the film.
- iOS 15 history (bug 230902): apps needed a gain of 4–10 on the Web Audio path. Now reportedly
  unnecessary.

---

### 6. Echo cancellation with media playing, and disabling it with headphones

- Desktop Chrome: the reference includes all Chrome audio going to the device, and system
  loopback on Windows/macOS (§2.2). The film's own audio is cancelled from the mic.
- Android Chrome: hardware/platform AEC costs call mode (§2.1). There is no software
  "remote-only" option on mobile.
- iOS Safari: VPIO, with Min-level advanced ducking of the film (§2.3).
- **Disabling AEC with headphones is reasonable.** Mumble's docs say echo is "common with a
  speaker setup. Not so much with headphones". Discord-style native apps also let users turn
  processing off. With speakers and AEC off, the partner hears the film (delayed) and their own
  voice back through your mic. A noise gate/VAD on the sender reduces this but does not remove
  it [inferred].
- Browsers do not reliably tell a page whether output is headphones or speaker. `enumerateDevices`
  labels are heuristic [unverified]. Ask the user.

---

### 7. Implications for Orpheum (ranked by value ÷ effort)

These are options for the owner to decide. Several touch frozen Cut A design (D193 §8) and the
voice bounds, so they are questions, not plans.

1. **Turn on software AGC (and NS) on the sender while keeping `echoCancellation:false` on
   Android.** Constraints: `{echoCancellation:false, autoGainControl:true, noiseSuppression:true}`.
   - Value: high (addresses finding 2 at the source).
   - Effort: very low.
   - Per Chromium source this runs AGC2/NS in the renderer without `MODE_IN_COMMUNICATION`.
   - Confirm on a device: the film stays on media volume, and the side keys still control media
     (a VQ item). Check `track.getSettings()` reports what was asked.
2. **Prefer RED for voice** (`setCodecPreferences` with `audio/red` before `audio/opus`).
   - Value: high (3% random loss → concealment far below 2% in the RED measurements).
   - Effort: low.
   - Cost: about +30–60 kb/s while talking.
   - Keep Opus mono, about 24–32 kb/s, `useinbandfec=1`.
   - Check Firefox's support before relying on it in the Chrome↔Firefox acceptance pairing.
3. **Add a limiter and speech-driven film ducking on the receiver.**
   - Change: partner gain → `DynamicsCompressorNode` (limiter) → destination. While the partner
     speaks, lower the film's `video.volume` by a few dB (fast attack, ~0.5–1 s release),
     modelled on SharePlay's advanced ducking.
   - Value: high. Effort: medium.
   - iOS: `video.volume` is ignored [unverified]. Either accept the OS's Min ducking, which needs
     EC on, or route the film through Web Audio there.
4. **Pace the film's segment fetches while voice is on**, and mark voice
   `priority/networkPriority: "high"`.
   - Value: high on shared home links. Effort: medium.
   - Pacing: stay ahead of the playhead but cap throughput below the link, so bursts don't fill
     the bottleneck queue in front of voice packets.
   - Priority "high" gives DSCP EF, which maps to Wi-Fi AC_VO (RFC 8325), and costs nothing.
5. **Re-examine the congested acceptance profile** before tuning against it.
   - Value: high (avoids chasing a test artifact). Effort: low.
   - Check whether voice crosses `lo` once or twice (relay → doubled delay and ≈5.9% loss).
   - Check netem's queue `limit` against the 5 Mbit `rate` (bufferbloat), and whether ±50 ms
     jitter reorders packets.
   - Log `jitterBufferTargetDelay/EmittedCount` beside the film's bytes in flight.
   - Consider bounds per path segment.
6. **Let the user say "headphones" or "speaker"** (EC off/on), and explain the trade-off: on
   Android, speaker mode means call mode with a quieter film and volume keys controlling the
   call. Medium value, low effort. On iOS, EC on gives Apple's ducking. EC off may lower
   playback level (Apple docs on chat modes without VPIO) — device-verify.
7. **iOS hygiene:** create or resume the `AudioContext` *after* `getUserMedia` resolves (bug
   218012 workaround), and keep playing partner voice through Web Audio. Low effort, with
   device verification.
8. **Bluetooth expectations:** with a BT headset's mic open, quality drops to HFP. iOS prefers
   HFP over A2DP when both are allowed. Consider defaulting the mic to the phone's built-in mic
   when a BT output is present. Whether a browser keeps A2DP output then is [unverified] and
   needs device tests. LE Audio removes most of this, but phone support varies.
9. **Bounding delay harder** would need a custom jitter buffer: Opus over an unreliable data
   channel + WebCodecs `AudioDecoder` + AudioWorklet playout, with a hard maximum. Alternatively
   wait for `jitterBufferMaximumDelay`. High effort; only if 1–5 don't bring delay within bounds.
10. **Brave/Android mic permission (finding 3):** not covered here; needs its own investigation.


---

## What people like and hate about consumer voice and VoIP apps

Research for the Watch Together voice feature, 2026-10-07. Web research only.

**Sources and their limits.** The sources are Hacker News threads and comments (through the Algolia API),
standards, peer-reviewed studies, vendor posts, bug trackers, Trustpilot and press. Reddit was
blocked (its JSON search returns an HTML wall). App Store and Play Store review pages could not be
fetched, so app-store review themes are **not covered first-hand**. The session's web-search budget
ran out partway through, so later sources came from direct fetches. HN commenters are a technical,
self-selected group: treat their quotes as examples of a sentiment, not as a measure of how common it is.
Raw material is cached in `/yoloai/cache/http/voip/` (`notes.md` plus fetched JSON and PDFs).

Markers: **[unverified]** means I could not confirm the claim from a primary source in this session.
**[anecdote]** means one person's report.

---

### 1. Call quality: what makes it feel good or bad

#### 1.1 Latency and talking over each other (the most-cited quality complaint among technical users)

- **The standard.** ITU-T G.114 (2003): "Although a few applications may be slightly affected by
  end-to-end ('mouth-to-ear') delays of less than 150 ms, if delays can be kept below this figure,
  most applications, both speech and non-speech, will experience essentially transparent
  interactivity. While delays above 400 ms are unacceptable for general network planning…". Its
  Figure 1 maps mouth-to-ear delay to the G.109 satisfaction bands, from "users very satisfied"
  down to "nearly all users dissatisfied". Appendix II estimates mouth-to-ear delay at under 150 ms
  for most intra-regional routes (5,000 km or less) and about 225 ms for 10,000 km, "still quite
  satisfactory for the vast majority of users".
  <https://www.itu.int/rec/dologin_pub.asp?lang=e&id=T-REC-G.114-200305-I!!PDF-E&type=items>
- **The human baseline is fast.** Stivers et al. (PNAS 2009), ten languages: answers to yes/no
  questions peak within 0 to 200 ms of the question ending. The mean offset is +208 ms, ranging from
  Japanese at +7 ms to Danish at +469 ms. <https://www.pnas.org/doi/10.1073/pnas.0903616106>
- **Calls stretch the gaps far more than the network delay explains.** Boland, Fonseca et al.
  ("Zoom disrupts the rhythm of conversation", J. Exp. Psych.: General, 2021/22) measured the
  same pairs of people:
  - turn transitions: 135 ms face to face, 487 ms over Zoom;
  - yes/no answers: 297 ms locally, 976 ms remotely.

  Both gaps are far larger than the estimated 30 to 70 ms of audio transmission delay.
  <https://www.researchgate.net/publication/356018506_Zoom_disrupts_the_rhythm_of_conversation>
- **People blame each other, not the delay.** Seuren et al., "Whose turn is it anyway?"
  (J. Pragmatics 2021) analysed 25 video consultations. Latency produced silences "at points where
  talk should occur" and extended overlaps. Participants rarely recognised latency as the cause:
  they "act as if they share the same reality". The worked examples ran at about 750 ms round
  trip. The summary says about 100 ms was benign and about 700 ms caused routine struggles
  **[that summary line is unverified against the full text]**.
  <https://pmc.ncbi.nlm.nih.gov/articles/PMC7819463/>
- **Effects persist.** Edwards (Speech Communication, 2025) added latency partway through 61
  audio-only conversations. Overlap grew in proportion to the latency, and the silences between
  speakers grew and **stayed longer after the latency was removed**. The details come from a search
  summary; the full text returned 403. <https://doi.org/10.1016/j.specom.2025.103226>
- **What users say:**
  - "The worst thing that I've found with teams is the latency… just slightly too high (and
    noticeably higher than Google meet and zoom)… absolutely critical in keeping the flow of
    conversation and preventing people from accidentally talking over each other."
    <https://news.ycombinator.com/item?id=39381176>
  - "Discord has a higher latency and sometimes when you start a new discussion you overlap… which
    is awkward. but discord has one of the best noise cancellation."
    <https://news.ycombinator.com/item?id=27650002>
  - "People get really annoyed by… constantly talking over each other. It subconsciously makes you
    dread talking to the other person. When everybody is using wired headphones, you suddenly like
    your coworkers more!" The comment's premise is that Bluetooth headset latency is large.
    <https://news.ycombinator.com/item?id=40484559>
  - "'Could you repeat that' and people talking over each other is much more common now than 20
    years ago." <https://news.ycombinator.com/item?id=8190650>
  - "I miss the low latency and not constantly talking over each others sentences" (on copper
    landlines). <https://news.ycombinator.com/item?id=48789252>
  - Mumble users praise its latency: "In all the years I've used it I never noticed any latency."
    <https://news.ycombinator.com/item?id=27649456>
- **Scale data.** Microsoft and Skype's Via study (SIGCOMM 2016) covered 88 million calls from
  1,900 ASes in 126 countries. Problem calls were spread across geography and networks, and
  choosing a better relay cut poor network conditions by up to 66%.
  <https://www.microsoft.com/en-us/research/publication/via-improving-internet-telephony-call-quality-using-predictive-relay-selection/>
  The paper's exact rating thresholds for RTT, loss and jitter could not be retrieved; the slides
  are images **[unverified]**.

#### 1.2 Robotic audio, dropouts and codec quality

- Opus-based apps get praise when the network holds: "audio quality on Signal voice calls is
  uniformly excellent, unless one person's end is experiencing very high packet loss."
  <https://news.ycombinator.com/item?id=26575762>
- FaceTime Audio is repeatedly called better than a phone call: "the audio quality is shockingly
  better" <https://news.ycombinator.com/item?id=18305005>. One commenter attributes this to AAC
  near full-band and a higher bitrate <https://news.ycombinator.com/item?id=42389668>
  **[technical claim unverified]**.
- WhatsApp splits opinion:
  - "WhatsApp's call quality is second to none… ubiquitous" <https://news.ycombinator.com/item?id=44335184>
  - "WhatsApp VC is rubbish… people still mainly use WhatsApp… because everyone is already using
    WhatsApp" <https://news.ycombinator.com/item?id=36810397>
- Signal's video draws complaints, especially between iOS and Android: "very often in Signal you
  are minecraft" <https://news.ycombinator.com/item?id=44335668>, and "video call quality pales in
  comparison to WhatsApp and FaceTime" <https://news.ycombinator.com/item?id=45172072>.
- In a head-to-head "Why is Zoom audio quality better?" thread, a team ran the same meeting on Meet
  and on Zoom and "all participants agreed the audio quality was better" on Zoom. A replier
  credited Zoom's tuning for laptop mics and speakers and its getting-in UX.
  <https://news.ycombinator.com/item?id=23573158>
- **Audio quality changes how people judge the speaker.** Newman and Schwarz (Science
  Communication, 2018) played identical talks and interviews in high and low audio quality. With
  low quality, people "evaluated the research and researcher less favorably".
  <https://doi.org/10.1177/1075547018759345>

#### 1.3 Echo, and the one person on laptop speakers

- "poor feedback cancelling from that one person who joins with the laptop speakers and mic" is
  one reason Mumble can't win over non-technical friends.
  <https://news.ycombinator.com/item?id=27649203>
- "Just having one non-headset user killed it for us… people heard themselves" (self-hosted
  Jitsi). <https://news.ycombinator.com/item?id=22484643>
- "pre-covid I would go around BEGGING everyone… to get a headset, because otherwise everyone
  else's voice would echo back… 0.75s later". The same commenter says echo cancellation has since
  improved enough to take calls on speakers. <https://news.ycombinator.com/item?id=40921264>
- Echo cancellation and latency trade off: "A feedback cancelling solution would probably increase
  latency." <https://news.ycombinator.com/item?id=27649559>

#### 1.4 Noise suppression: welcome until it eats words

- Discord's Krisp has a support-forum thread titled "Krisp Noise suppression cuts out my voice"
  (body 403; title only).
  <https://support.discord.com/hc/en-us/community/posts/360052542413-Krisp-Noise-suppression-cuts-out-my-voice>
- On Jitsi's enhanced suppression: "the noise suppression is aggressive to the point that it's
  eating critical signal with the noise" <https://news.ycombinator.com/item?id=33047846>. Another
  reply: "I didn't understand the first three words, for Alice it was the next two, and for Bob it
  was the last four. How many people are going to ask to repeat?"
  <https://news.ycombinator.com/item?id=33049134>
- RNNoise's author, quoted in that thread, says not to expect better intelligibility. The gain is
  that the result is "much less annoying to listen to and likely causes less listener fatigue".
  <https://news.ycombinator.com/item?id=33051803> (quoting <https://jmvalin.ca/demo/rnnoise/>)
- Cost: RNNoise WASM "added something like 10% CPU usage and in the end we disabled it."
  <https://news.ycombinator.com/item?id=33047222>
- Voice-activity gates clip words: "words being dropped not only at the end but also in the middle
  of someone talking if they're using voice activation"
  <https://news.ycombinator.com/item?id=15877874>, and "we complain a lot when someone on the
  server has an improper voice activation threshold" <https://news.ycombinator.com/item?id=17955443>.
- Some Discord users find it worse than TeamSpeak at filtering system sounds.
  <https://news.ycombinator.com/item?id=26554511>

#### 1.5 Volume too low or uneven between people

- Discord's per-user volume slider is valued, and people ask to go beyond 200%: "Allow Discord
  volume to be increased beyond what's currently possible".
  <https://support.discord.com/hc/en-us/community/posts/360031044891-Allow-Discord-volume-to-be-increased-beyond-what-s-currently-possible>
  One user lists among Discord's advantages that it "allows to change volume level per user".
  <https://news.ycombinator.com/item?id=24830592>
- **Watch-party specific.** "Is there a way to lower the volume of the media being screen shared
  over FaceTime? It's always way too loud and makes it hard for me to hear anyone in the call
  talking." <https://discussions.apple.com/thread/253483990> SharePlay's answer is automatic: "the
  volume will automatically adjust so that you can hear everyone talking without missing the
  show." <https://www.macrumors.com/how-to/watch-movies-together-facetime/> Apple's own support
  text for this "smart volume" could not be fetched **[mechanism unverified]**.
- **iOS Safari WebRTC playback is quiet or goes to the earpiece.**
  - WebKit bug 230902, "REGRESSION(Safari 15 - iOS15): [WebRTC] MediaStreamTrack audio volume too
    low", was P1. Audio went to the earpiece and the volume dropped once the mic was granted. It
    was fixed in iOS 15.4. <https://bugs.webkit.org/show_bug.cgi?id=230902>
  - WebKit bug 218012, "Audio Volume reduces considerably on accepting the mic permissions". A
    WebKit engineer explained that capture puts the page in play-and-record, which routes output to
    the receiver. `navigator.audioSession.type` (iOS 16.4+) is the partial remedy.
    <https://bugs.webkit.org/show_bug.cgi?id=218012>
  - A developer forum report: "voice is coming too low… used to come through loudspeaker but now it
    don't"; incoming audio played at about two-thirds of its former volume.
    <https://developer.apple.com/forums/thread/691647>

---

### 2. Friction: accounts, installs, permissions, links

- **Browser join with no account wins first contact.**
  - Google Meet lets guests join from a browser without a Google account if the host admits them.
    <https://support.google.com/meet/answer/9303069>
  - Zoom was widely criticised for pushing its app and hiding "join from browser": "Zoom
    auto-download their app, and then pretend that you don't have a choice but to install it"
    <https://hackernoon.com/how-zoom-uses-dark-patterns-to-increase-app-downloads>. See also
    <https://shkspr.mobi/blog/2020/03/zooms-forced-app-is-irresponsible/> and TechCrunch's "PSA: Yes
    you can join a Zoom meeting in the browser"
    <https://techcrunch.com/2020/03/20/psa-yes-you-can-join-a-zoom-meeting-in-the-browser/>.
  - Teams and Skype for Business: "infinite loops of download installer, install, now go download
    installer again." <https://news.ycombinator.com/item?id=22675914>
- **But low friction alone doesn't win.** One user moved eight friends from Hangouts to Jitsi
  ("Click a URL. No accounts"), yet "Friends preferred Hangouts".
  <https://news.ycombinator.com/item?id=22670307> FaceTime is praised because there is nothing to
  set up: "No accounts or meeting/room IDs to remember… it just works"
  <https://news.ycombinator.com/item?id=22670869>. For others, though, "maintaining an Apple
  account is a bridge too far" <https://news.ycombinator.com/item?id=22672041>. A link is what
  works for non-technical people: "Granny won't set up a room, but you can send her a link."
  <https://news.ycombinator.com/item?id=22670411>
- **Why Discord beat Mumble and TeamSpeak** (HN, 2021):
  - "People want Discord because of (in this order): network effect, access from the browser,
    polished UI." <https://news.ycombinator.com/item?id=27650298>
  - "Nothing can compete with discord where you just press a button to create a 'server' and send
    out a link." <https://news.ycombinator.com/item?id=27649441>
  - "You don't need to set up a separate account per server… didn't have the user limits of
    Teamspeak… didn't have the ads of Skype." <https://news.ycombinator.com/item?id=28312893>
  - Mumble: "Great software, poor UX" <https://news.ycombinator.com/item?id=27649243>. "I had an
    infographic for how to install and connect but still spent hours every week walking people
    through troubleshooting" <https://news.ycombinator.com/item?id=27649481>. A certificate
    "accept?" popup blocked a friend <https://news.ycombinator.com/item?id=27649773>.
- **Account pain is the top complaint on review sites, not call quality.** Both Discord
  (1.4/5, 447 reviews) and WhatsApp (1.4/5, 1,718 reviews) are dominated by bans, verification
  loops and unreachable support. One Discord reviewer: "an endless loop of 40+ CAPTCHA puzzles".
  <https://www.trustpilot.com/review/discord.com> <https://www.trustpilot.com/review/www.whatsapp.com>
  On HN: "For each single login, discord makes me solve 2 captchas, enter my password twice and
  click on an emailed link… On mumble… it just works. Also, it instantly reconnects."
  <https://news.ycombinator.com/item?id=27650286>
- **Setup cost of the watch-party alternatives.** MakeUseOf puts Discord setup for a watch party at
  "20 and 30 minutes" (accounts, a server, friending), against Teleparty's simpler flow.
  Teleparty's voice and video cost $4 a month extra.
  <https://www.makeuseof.com/teleparty-vs-discord-best-way-watch-with-friends/> On Trustpilot
  Teleparty scores 2.0/5 (13 reviews). One reviewer: "the whole point of watching a film with
  people is being able to talk with them"; others report it "laggy" and that "synchronization
  didn't work sometimes". <https://www.trustpilot.com/review/teleparty.com>
- **Microphone permission prompts.**
  - Add Pipe measured 100,000 Chrome getUserMedia calls: about 21% showed a prompt, and about 10%
    of those prompted denied or dismissed it (about 2.3% overall).
    <https://blog.addpipe.com/using-permissions-api-to-detect-getusermedia-responses/>
  - Google Meet's case study: overlapping Meet, browser and OS prompts on join caused "overwhelm".
    Meet now first asks in its own UI whether you want to be heard or seen, and only then triggers
    the browser prompt, with the option to add the mic later. That gave a **14% increase** in
    people allowing camera and mic on their first join, and fewer "Block" clicks, so fewer support
    requests for recovering from a block.
    <https://web.dev/case-studies/google-meet-permissions-best-practices>
  - In a CHI 2024 study of Chrome desktop prompts, microphone prompts were among the least
    annoying when granted (79.1% "not annoying"). <https://dl.acm.org/doi/full/10.1145/3613904.3642252>
  - Recovering from a denied permission is a common failure: Azure lists "microphone permission
    denied" as a standard user-facing diagnostic.
    <https://learn.microsoft.com/en-us/azure/communication-services/resources/troubleshooting/voice-video-calling/references/ufd/microphone-permission-denied>
- **Links.** Signal added call links in November 2024 so that anyone can join a call without first
  building a group chat. <https://signal.org/blog/call-links/>

---

### 3. Mobile and headset annoyances

#### 3.1 Bluetooth drops to "phone call" quality when the mic opens (very widely hated)

- Apple: "If you listen to music or other audio with Bluetooth headphones and then open an app that
  uses the Bluetooth headphones' microphone, audio quality and volume are reduced."
  <https://support.apple.com/en-hk/102217>
- AirPods: "the sound output seems to be automatically downsampled to… super low-quality MP3…
  this is system-wide (e.g. a YouTube tab or Spotify)" <https://news.ycombinator.com/item?id=23214739>.
  A reply says HFP uses CVSD or mSBC <https://news.ycombinator.com/item?id=23216906>, and another:
  "You just can't get audio that sounds better than a 2005 earpiece over Bluetooth if you need a
  mic." <https://news.ycombinator.com/item?id=23216451>
- The mic switch also breaks stereo and spatial playback: "my damn Airpods switching down to mono
  when I turned on the microphone" (binaural web app). <https://news.ycombinator.com/item?id=23215740>
- "horrible playback audio quality when the microphone on my bluetooth headset is active. E.g,
  listening to music while being in a muted Skype-call." <https://news.ycombinator.com/item?id=24339900>
- People pay for workarounds. A macOS app that keeps AirPods for output and forces the built-in mic
  drew more than 300 comments (<https://news.ycombinator.com/item?id=41695756>). Spotify's forum
  has a thread titled "Spotify Sound Quality lowers while on Discord".
  <https://community.spotify.com/t5/Desktop-Mac/Spotify-Sound-Quality-lowers-while-on-Discord/td-p/4887161>
- On Windows, the common forum workaround is to disable "Hands-Free Telephony" for the headset,
  which leaves it without a mic (Steam and Tom's Guide forums, via search summary)
  **[secondary]**. LE Audio (Android, Windows 11 "super wideband stereo") is the long-term fix
  **[product claims not fetched]**:
  <https://blog.google/products/android/le-audio-auracast-support/>
  <https://techcommunity.microsoft.com/blog/windowsosplatform/cutting-the-wire-without-cutting-the-audio-quality/4447942>

#### 3.2 Volume keys, routing, earpiece and speaker

- Android: volume keys control media volume, except during a call, when they control call volume.
  <https://news.ycombinator.com/item?id=27833190> **[anecdote; platform behaviour not checked
  against Android docs]**
- iOS Safari WebRTC routing went to the earpiece and was quiet (WebKit 230902 and 218012, section
  1.5). Phones that "suddenly [go] on speaker phone so the sound would blow up my ear" are a
  complaint about native calling. <https://news.ycombinator.com/item?id=33434120>
- **Watch-together relevance [inference]:** on iOS, opening the mic switches the page's audio
  session to play-and-record. The film's own audio can then change route or level (WebKit 218012).
  That needs device verification for Orpheum's player.

#### 3.3 Other audio ducked or paused

- SharePlay deliberately ducks the film while people speak (MacRumors, above). The opposite
  complaint, shared media drowning out voices, appears in the Apple Community thread above.
- I found no first-hand source for apps pausing music when a call starts **[gap]**.

#### 3.4 Network switching and battery

- "Duo seamlessly handles switching from Wifi to LTE when walking outside my house. Most other
  services drop the call." <https://news.ycombinator.com/item?id=22944460> **[anecdote]**
- Desktop voice apps get blamed for idle CPU and battery drain: "I can have these apps running with
  literally no voice or video activity… CPU moves between 5-10%… the fan will be running at max,
  and the battery drains fast" (Zoom, Teams, Discord)
  <https://news.ycombinator.com/item?id=32085799>. Another: "discord (the website) drains my laptop
  battery almost as fast as gaming does." <https://news.ycombinator.com/item?id=35210833>

---

### 4. Social features

- **Push-to-talk against an open mic.** Gamers love push-to-talk. One complaint is that Skype and
  Teams lacked it: "push-to-talk is basically great feature to allow you work comfortably while
  being on call". The reply: "It's not an intuitive feature for people. With phones you just
  talk." A mute toggle is "for most 'easier'". Zoom (desktop) and Jitsi offer hold-space to talk.
  <https://news.ycombinator.com/item?id=24429842>
- **Mute indicators.** "You're on mute" is the defining videoconference cliché
  (<https://news.ycombinator.com/item?id=45073219>); nothing quantitative was found **[gap]**.
- **Per-person volume** is valued (section 1.5). **Who is talking:** Jitsi's automatic active-speaker
  view was noted as a feature (<https://news.ycombinator.com/item?id=22670307>). Discord's speaking
  ring is standard **[from product knowledge, not sourced]**.
- **Dropping in and out of voice.** Audio-only, always-open rooms are remembered fondly: "audio-only
  is far more conducive to ad-hoc conversation. The closest I've ever felt to virtually 'hanging
  out'… was… Ventrilo / Teamspeak / Mumble… I'd sit in there doing homework."
  <https://news.ycombinator.com/item?id=27979980> Gather's walk-up-and-leave model was praised for
  "walk up to people you know, chat for a few seconds, and then step away"
  <https://news.ycombinator.com/item?id=25039963>. Others dropped it after a week, finding it heavy
  on resources and "childish" <https://news.ycombinator.com/item?id=27980595>.
- **Raise hand.** Signal added it in 2024 so people can ask to speak without interrupting.
  <https://signal.org/blog/call-links/>
- **Spatial audio.** Mumble's positional audio "completely changes the feeling of talking with
  people online; they become their avatar." <https://news.ycombinator.com/item?id=23407521>
  Academic: Baldis, "Effects of spatial audio on memory, comprehension, and preference during
  desktop conferences" (CHI 2001) <https://doi.org/10.1145/365024.365092>. Its findings (better
  memory and speaker identification, and a preference for spatial audio) are recalled, not
  re-read: **[unverified]**. Bluetooth headsets drop to mono when the mic is on (section 3.1),
  which undercuts spatial audio on the most common phone setup.
- **Video fatigue: an argument for voice-first.** Bailenson (2021) proposed "nonverbal overload":
  close-up gaze, self-view, being tethered to one spot, and the effort of sending and reading cues.
  Fauville et al. (2023) surveyed 9,787 people and found fatigue rose with more and longer calls
  and fewer breaks, and was higher for women, driven by "mirror anxiety".
  <https://vhil.stanford.edu/publications/video-conferencing-usage-dynamics-and-nonverbal-mechanisms-exacerbate-zoom-fatigue>
  Riedl and Müller-Putz (2023) measured EEG and ECG in 35 students and found more fatigue after a
  50-minute video lecture than after an in-person one. <https://spectrum.ieee.org/zoom-fatigue>

---

### 5. Privacy and trust

- **E2EE has become the expected default for personal calls.**
  - Messenger turned on default E2EE for personal messages and calls on 6 December 2023.
    <https://about.fb.com/news/2023/12/default-end-to-end-encryption-on-messenger/>
  - Discord launched its DAVE protocol (audited by Trail of Bits) for DMs, group DMs, voice
    channels and Go Live on 17 September 2024. It shows a "voice privacy code" that changes when
    anyone joins or leaves. <https://discord.com/blog/meet-dave-e2ee-for-audio-video>
  - Signal has had E2EE calls since 2014. <https://signal.org/blog/call-links/>
  - Telegram's 1:1 calls (2017) are E2EE and verified by comparing four emoji, peer-to-peer where
    possible. <https://telegram.org/blog/calls> Telegram group-call E2EE status was not checked
    **[unverified]**.
- **Overclaiming is punished.** The FTC (9 November 2020) found Zoom "misled users by touting that
  it offered 'end-to-end, 256-bit encryption'".
  <https://www.ftc.gov/news-events/news/press-releases/2020/11/ftc-requires-zoom-enhance-its-security-practices-part-settlement>
- **Why self-hosting appeals.**
  - Ownership: "Discord is a service owned by somebody, Mumble isn't."
    <https://news.ycombinator.com/item?id=27650019>
  - No login gauntlet, and reconnects instantly. <https://news.ycombinator.com/item?id=27650286>
  - Low latency on a nearby server: "the level of QoS you get from hosting on a low load computer
    with like 4ms ping… is insane compared to modern alternatives"
    <https://news.ycombinator.com/item?id=27653194>; quality was acceptable for 4 family members
    in 2 countries on rural ADSL <https://news.ycombinator.com/item?id=22758643>.
- **What goes wrong with self-hosting.**
  - Self-hosted Jitsi "developed into an IT support nightmare"
    <https://news.ycombinator.com/item?id=25939415>; streams "either didn't start or froze later"
    <https://news.ycombinator.com/item?id=22610834>.
  - Poor UX and onboarding (section 2).
  - Hosts have to supply the network quality that Zoom and Webex build for themselves
    <https://news.ycombinator.com/item?id=27649562>. A Matrix lead listed a low-latency SFU among
    the gaps to close with Discord <https://news.ycombinator.com/item?id=30569939>.
  - Exposure of IP addresses: on P2P "people will DoS any IP they see", which is relevant to gaming
    communities and less so to friends. <https://news.ycombinator.com/item?id=27653110>

---

### 6. Why Skype and Clubhouse fell

- **Skype** shut down on 5 May 2025, and users moved to Teams. Microsoft framed it as focus: "a
  simpler message and drive faster innovation".
  <https://techcrunch.com/2025/02/28/microsoft-hangs-up-on-skype-service-to-shut-down-may-5-2025>
  Users fell from 150M monthly at Microsoft's 2011 acquisition to about 23M by 2020.
  <https://restofworld.org/2025/skype-shutting-down/> Other figures: about 300M in the mid-2010s
  to about 23M in 2025 (<https://www.foxbusiness.com/technology/microsoft-shuts-down-skype-after-22-years-shifting-users-teams>),
  and 36M daily in 2023 (Wikipedia). Causes as recorded:
  - It was desktop-first and missed the move to mobile, where WhatsApp and others took over (Rest
    of World).
  - A 2017 Snapchat-style redesign was "not well-received, with numerous negative reviews and
    complaints that the new client broke existing functionality", and was rolled back.
  - It moved from P2P to Microsoft's cloud in 2017 and became messaging-first.
    <https://en.wikipedia.org/wiki/Skype>
  - Performance: "Skype is, in terms of performance, one of the worst apps… I have to deal with"
    (2015). <https://news.ycombinator.com/item?id=9658187>
  - Teams cannibalised its business users.

  What users remember valuing: "It was basically the only way that we could be in contact, and for
  free." (Rest of World)
- **Clubhouse**: downloads went from 3.5M to 8.1M during the first half of February 2021, and
  weekly active users passed 10M by March. Usage fell 21% in three weeks; the app was iOS-only
  until mid-2021; 1.3M scraped user records were published; moderation failures followed.
  <https://en.wikipedia.org/wiki/Clubhouse_(app)>
  <https://www.androidpolice.com/why-did-clubhouse-and-other-social-audio-apps-fail/>
  Secondary claims of an 80% loss within six months are **[unverified]**
  (<https://medium.com/@achu_83391/what-happened-to-clubhouse-a-data-driven-analysis-of-social-medias-fastest-rise-and-fall-7c1ee1386ec9>).
  What users said about the rooms:
  - "too much noise, very little signal" <https://news.ycombinator.com/item?id=29564195>
  - "A core group of speakers… tended to dominate many of the rooms"
    <https://news.ycombinator.com/item?id=29564590>
  - Yet: "Nothing really compares to Agora/Clubhouse audio quality."
    <https://news.ycombinator.com/item?id=29563838>

  The lesson is that the content was strangers' rooms. Audio quality wasn't the problem.

---

### 7. Quantitative summary

| Measure | Value | Source |
|---|---|---|
| Mouth-to-ear delay for "essentially transparent" interactivity | < 150 ms; > 400 ms unacceptable for planning | ITU-T G.114 |
| Natural gap between turns | mode 0–200 ms, mean +208 ms | Stivers 2009 |
| Turn transition, face to face vs Zoom | 135 ms vs 487 ms | Boland et al. 2021 |
| Yes/no answer, local vs remote | 297 ms vs 976 ms | Boland et al. 2021 |
| Latency that disrupted clinical video calls | ~750 ms RTT in the examples | Seuren et al. 2021 |
| Lingering effect | longer silences persist after latency is removed | Edwards 2025 |
| Chrome getUserMedia calls that prompt / of those, denied | 21% / 10% | Add Pipe |
| Meet's two-step permission ask | +14% allowing on first join | web.dev case study |
| Zoom-fatigue survey | n = 9,787; worse with frequency, duration, fewer breaks, women | Fauville 2023 |
| Poor-network calls fixable by better relays | up to 66% fewer | Via, SIGCOMM 2016 |

---

### Implications for a watch-together voice feature

These follow from the evidence above. Where a point is my inference rather than something a
source states, it says so.

1. **Latency is the quality that matters most, and people blame each other for it.** Keep the voice
   path as short as possible: direct peer-to-peer or a nearby relay, with a small jitter buffer.
   Aim for mouth-to-ear under about 150 ms (G.114) and treat 400 ms as a hard ceiling. Seuren and
   Edwards show that people won't name the problem, and that a stretch of high latency keeps
   affecting the conversation after it ends. Bluetooth headsets add delay on top **[size
   unverified]**. Inference: voice latency matters more than shaving the last tens of milliseconds
   off film sync, because film sync is spread across everyone equally and voice delay is felt
   between speakers.
2. **The film and the voices compete; give people a balance control and consider ducking.** The
   most specific watch-party complaint is shared media drowning out voices. SharePlay's answer is
   to lower the film automatically while someone speaks. Offer:
   - a per-person volume slider, which Discord users value and ask to push beyond 100%;
   - a film-against-voices balance;
   - optional ducking.

   Echo is the other risk. Film audio playing from someone's speakers goes back into their mic
   (the "one person on laptop speakers" problem). Echo cancellation must treat the local film audio
   as its reference signal. Inference: browser echo cancellation does this only if the film plays
   through the same output path, and that needs device verification.
3. **Noise suppression on by default, but gentle, and switchable per person.** Aggressive
   suppression and voice-activity gates are the second most common quality complaint (Krisp,
   Jitsi, Discord voice activation), and they cost CPU and battery. Inference: a film soundtrack is
   exactly the kind of non-speech sound a suppressor can mangle or let through. Test both.
4. **Mute by default, with an obvious and reversible way in.** Don't call `getUserMedia` on page
   load. Copy Meet's two-step pattern: an in-app "Join voice?" choice first, the browser prompt
   only after the user opts in, and voice addable later. That gave Meet +14% and fewer Blocks.
   Provide clear recovery steps for a denied permission, per browser and OS. Push-to-talk should be
   an option, not the default: non-gamers find it unnatural, while gamers and noisy households want
   it.
5. **Voice must be separable from the session.** Joining or leaving voice must not stop playback
   or drop the person from Watch Together. The best-loved voice spaces (Ventrilo, Mumble, Gather)
   let people drift in and out. Show who is in voice, who is speaking, and who is muted, to
   everyone.
6. **Be honest about Bluetooth.** On phones and laptops, opening the mic on Bluetooth headphones
   drops the whole device, film included, to phone-call quality: mono, narrow band, lower volume.
   This is the most widely hated mobile behaviour. Inference: detect a Bluetooth headset if the
   platform allows it, and offer listen-only, push-to-talk that opens the mic only while held, or
   "use the phone's or laptop's own mic". Never quietly hold the mic open. Spatial audio is
   mostly lost on this setup.
7. **iOS Safari needs device testing.** Opening the mic can send audio to the earpiece or lower
   playback volume (WebKit 218012 and 230902). Test the film and the voices together on iPhone,
   including with the mute switch, AirPods, and a screen lock in the middle of a call. This is a
   verification-queue item, not a unit test.
8. **Survive network switches.** Wi-Fi to cellular is when voice drops; Duo was praised for
   surviving it. Inference: reconnect voice automatically and quietly (ICE restart), and show a
   small "reconnecting" state rather than dropping the person.
9. **No accounts for guests, and links that work in a browser.** Friction stops first-time joiners:
   Zoom's install push, Teams' login loops, Discord's 20–30 minute setup and its CAPTCHA loops,
   Mumble's wizard and certificate prompt. Voice should be one tap inside a session people already
   joined from a link. Watch for idle CPU and battery cost; users notice it.
10. **Encryption: say exactly what is true.** Users now expect E2EE for personal calls, and the FTC
    punished Zoom for overclaiming. Inference: a self-hosted server where voice runs peer-to-peer
    or through a relay the owner controls is a privacy point worth stating. Don't call it E2EE
    unless the media is encrypted end to end against the relay; with WebRTC, DTLS-SRTP through a
    TURN relay is end to end, but through an SFU it isn't **[technical inference]**. Self-hosting
    appeals for ownership and latency, and fails on setup, UX and an "IT support nightmare". The
    voice feature has to work with no configuration beyond what Orpheum already needs.
11. **Voice-first, video optional.** The research on video fatigue (gaze, self-view) points to
    voice as the default companion to a film. The film is the shared visual.
12. **Don't build Clubhouse.** Social audio among strangers failed on content and moderation, not
    on audio. Watch Together is small groups of friends, which is where voice works best.

#### Gaps worth closing later
- First-hand App Store and Play Store review themes for WhatsApp, Discord, Teleparty and Rave.
  These could not be fetched; Reddit was blocked.
- Primary data on music being ducked or paused when a web call starts on Android Chrome, and on how
  volume keys behave during WebRTC in mobile browsers.
- The Via paper's rating thresholds for RTT, loss and jitter, and the full text of Baldis 2001 and
  Edwards 2025.
