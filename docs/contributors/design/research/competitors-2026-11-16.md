> **ABOUTME:** How competitors handle five features Orpheum has only loosely defined (data caps,
> the climb back to the original file, what a cast target may do, large lists and letter jumps,
> signing a TV in by code), what users complain about, the pitfalls, and options for the owner.

# How competitors handle five loosely defined features

- **Date:** 2026-11-16 (searched and fetched 2026-11-15; notes cached in
  `/yoloai/cache/research/competitors-2026-11-16-notes.md`).
- **Asked by** the owner: for features that are poorly defined, how do competitors do them, and
  what must we watch out for?
- **Sources:** vendor help centres and developer docs, the vendors' forums, two RFCs and a handful
  of papers. Forum posts are anecdotes, dated where the page dates them. Figures from third-party
  sites are marked as such; a claim not checked against its vendor's own page is **UNVERIFIED**.
- **The design is the owner's** (AGENTS.md rule 1). Every recommendation below is the research
  author's, marked as one, for the owner to accept, change or drop.

## Summary

| Topic | Orpheum now | Headline finding | Recommendation |
| --- | --- | --- | --- |
| 1. Data caps | GB-per-hour list in ⋯, per network kind, browser-kept ([D488](../../decisions/working-notes.md), [D489](../../decisions/working-notes.md), [D491](../../decisions/working-notes.md)) | Everyone splits Wi-Fi from mobile; only Prime Video caps casting; no web app can tell a hotspot from Wi-Fi | Keep the design; add a one-time "on mobile data" note, and a cast cap only if the owner wants it |
| 2. Back up to the original | Decided: automatic, unasked; the hold time is open | No competitor moves a direct-played file up mid-film; players wait about 10 s of buffer and use a 0.7 safety factor | Require a long hold (about 60 s at 1.3× the file's rate), at most once per film, and say so in ⋯ |
| 3. Cast target rights, Up next | Target may do what it can; Up next picked on the caster, shown on the receiver | Google's own rule: receiver shows the countdown, sender a static "Up next"; one queue, kept in one place | Keep the queue on the core; the receiver only renders it |
| 4. Large lists, letter rail | Pages of 60 with a total; whole-list text fetch decided (plan) | Jellyfin pages at 100 and warns at 20,000; Plex's letter jumps land wrong in large libraries | Fetch the whole list's text once, and compute letter offsets from it |
| 5. Sign-in by code | Code on the TV, entered on a signed-in phone, check code on both ([D288](../../decisions/working-notes.md), [D469](../../decisions/working-notes.md)) | Orpheum's flow is already stricter than Plex's or Jellyfin's; the live threat is remote phishing | Keep the check code; name the device and its network on the approving phone |

## 1. Data caps and data saver

**Orpheum now.** ⋯ holds a list of GB-per-hour figures, one per rung, each a ceiling of the chunked
player's choice ([D488](../../decisions/working-notes.md) §1–§3); the film's own picture counts as one
more rung ([D491](../../decisions/working-notes.md)). Where `navigator.connection.type` names the
network there are two caps (mobile data, Wi-Fi), else one; all start at No limit, kept in the
browser ([D489](../../decisions/working-notes.md) §3). Casting stays uncapped (D489 §4). Still open
from D488: a phone on a hotspot reads as Wi-Fi; no cap holds without Media Source Extensions.

**What competitors do.**

- **Netflix**: under Cellular Data Usage, Automatic, Wi-Fi Only, Save Data and Maximum Data;
  Netflix gives Automatic as "about 4 hours per GB" and Save Data as "about 6 hours per GB"
  ([help.netflix.com/en/node/87](https://help.netflix.com/en/node/87)). Wi-Fi quality is a separate,
  per-profile setting (Low 0.3 GB/h, Medium 0.7, High up to 3 HD or 7 4K, same page).
- **YouTube**: Video quality preferences hold two defaults, "On mobile networks" and "On Wi-Fi"
  (Auto, Higher picture quality, Data saver); a choice in the player is for that video alone
  ([9to5Google, 2020-11-09](https://9to5google.com/2020/11/09/youtube-video-quality-preferences/)). A
  reviewer found Data saver still playing 720p on a fast link
  ([GSMArena](https://www.gsmarena.com/youtube_android_ios_mobile_app_new_video_resolution_settings-news-48821.php)).
- **Plex**: three ceilings: Home, Remote, and "Limit Cellular Data", shown only on devices with a
  mobile connection, default 0.7 Mbps ([support.plex.tv/?p=39056](https://support.plex.tv/?p=39056));
  the server owner can also cap remote bitrate.
- **Disney+**: App Settings, Video Playback, separate "Cellular Data Usage" (Automatic, Save Data)
  and "Wi-Fi Data Usage", the latter added in April 2020 for people with capped home internet
  ([WDWNT, 2020-04](https://wdwnt.com/2020/04/update-to-disney-app-allows-users-to-restrict-wi-fi-data-usage/)).
  Kept per device, for every profile on it (Disney's note as quoted by
  [iDownloadBlog](https://www.idownloadblog.com/2020/12/23/disney-plus-data-usage-tutorial/); UNVERIFIED
  against Disney's own page).
- **Prime Video**: Stream & Download: Best, Better, Good, Data Saver, plus "Stream on Wi-Fi only"
  and "Notify when streaming on mobile data". Amazon publishes no figures; third-party ones
  disagree. Alone among these, version 3.0.264 added **Google Cast data usage** (Unlimited,
  Balanced at about 1.8 GB/h, Data saver); "changes to the data usage mode will be reflected in the
  currently playing video stream" ([XDA, 2020-03-06](https://www.xda-developers.com/amazon-prime-video-chromecast-data-usage-options/)).
- **Jellyfin**: one maximum streaming bitrate per client, no mobile-versus-Wi-Fi split found; its
  Auto measures once, at the start ([forum](https://forum.jellyfin.org/t-client-determining-bitrate)).

**Common complaints.** A cap that holds when it should not: Plex on iOS kept a cellular ceiling with
the limit off ([forum 826296](https://forums.plex.tv/t/cellular-data-is-limited-even-if-its-not-selected-know-bug-in-ios/826296)).
A cap that does not hold: YouTube's Data saver at 720p, above. A cap's single top that is too low:
Plex's cellular list stopping short ([forum 803769](https://forums.plex.tv/t/limit-cellular-data-increase-highest-limit-option/803769)).
Settings that silently differ by network, so a person sets one and is governed by the other.

**Pitfalls.**

- **A hotspot looks like Wi-Fi to the web.** Android marks a network metered
  (`NET_CAPABILITY_NOT_METERED` absent) and a user forum reports it marks hotspots so
  ([Spotify community](https://community.spotify.com/t5/Android/Android-app-does-not-respect-metered-WiFi-networks/m-p/1921488);
  [Android docs](https://developer.android.com/training/monitoring-device-state/connectivity-status-type)),
  but the browser's `metered` attribute exists only in a 2021 proposal
  ([webkit-dev](https://lists.webkit.org/pipermail/webkit-dev/2021-August/031963.html)). Native apps
  can see it; a web app cannot. `navigator.connection.saveData` (the `Save-Data` header) is the one
  signal a browser sends, and since Chrome's Lite mode went in 2022 it is rarely set
  ([blink-dev](https://groups.google.com/a/chromium.org/g/blink-dev/c/HR7tWmewbSA/m/R0QYg-ZAAAAJ)).
- **Casting moves the data, not the cap.** A Chromecast fetches the film itself, so a cap on the
  phone saves nothing, but a Chromecast on a phone's hotspot or a capped home line spends exactly
  the data the person meant to save. Prime Video is the only service found that answers this.
- **Natively played HLS ignores a ceiling** set in hls.js; Safari on iOS before 17.1 and other
  non-MSE browsers choose for themselves. The only lever there is the master playlist the core
  sends (omitting rungs above the cap).
- **Average versus peak.** A figure built from AVERAGE-BANDWIDTH undercounts for grainy or
  action-heavy films; Netflix says "about", and Prime publishes nothing.

**Options for the owner.**

1. **Keep D488 to D491 as they are.** No extra signal; the hotspot case stays a known gap. Cheapest;
   the person who tethers learns by their bill.
2. **Use the master playlist as the cap where hls.js is absent.** The core leaves out rungs above
   the device's cap when the device asks for a ladder; this closes the non-MSE gap and works for a
   Chromecast too. Costs a playlist per cap and a test on Safari.
3. **A cast cap, as Prime Video has.** One more line under the cast menu, carried to the receiver
   with the cast. Reverses D489 §4; small use, real for hotspot casters.
4. **A one-time note on mobile data**, "Playing on mobile data: 0.6 GB per hour", when playback
   starts on cellular with No limit, as Prime's "Notify when streaming on mobile data" does. Costs a
   string; nags if repeated.

**Recommendation (the author's):** 1 with 4, and 2 when the non-MSE gap matters to a beta tester.
Leave the hotspot gap documented: no browser signal exists to close it.

## 2. Moving back up to the original after starting low

**Orpheum now.** Plan
([navigation-and-widgets-answers-2026-10-27.md](../plans/navigation-and-widgets-answers-2026-10-27.md),
"The original chosen on a link too slow for it"): the original chosen on a link known to be short of
its rate starts on the highest rung the link carries, says so in ⋯, and moves to the original "once
the link has carried the file's rate for a while"; the owner decided the move is automatic, and that
an unmeasured link tries the original first. "For a while" and what the screen shows are open.

**What competitors do.**

- **Plex**: Automatically adjust quality is a ceiling the stream drops below and climbs back to,
  but "If content is already directly playing at the original quality (e.g. Direct Play or Direct
  Stream), then Plex will not adjust the quality dynamically" (Plex support article 115007570148, as
  quoted in [search results](https://support.plex.tv/articles/115007570148-automatically-adjust-quality-when-streaming/);
  the page refused a direct fetch). No move from a transcode back to Direct Play was found.
- **Infuse 8.5** (2026-07-30) added transcoding for Plex, Emby and Jellyfin as a manual choice
  (long-press Play, Transcoding); "By default, Infuse will continue to play the original video at
  full quality" ([Firecore blog](https://firecore.com/blog/infuse-85-smarter-streaming)). No
  automatic switch is described.
- **Jellyfin**: "A bitrate change mid-session does not change a stream in progress"
  ([forum](https://forum.jellyfin.org/t-automatic-quality-switching-transcoding-if-connection-too-slow)).
- **Netflix and YouTube** never leave their ladder, so their climbs are rung changes, not a change of
  delivery. Netflix's published buffer-based algorithm treats the start differently from the steady
  state: during start-up "the buffer occupancy carries little or no information", so it climbs on a
  throughput estimate, then on the buffer alone
  ([Huang et al., SIGCOMM 2014](https://web.stanford.edu/class/cs244/papers/sigcomm2014-video.pdf)).
- **Player defaults** encode the hysteresis everyone uses: ExoPlayer moves up only with 10 s of
  media buffered (`DEFAULT_MIN_DURATION_FOR_QUALITY_INCREASE_MS`) and spends 0.7 of measured
  bandwidth ([Android reference](https://developer.android.com/reference/androidx/media3/exoplayer/trackselection/AdaptiveTrackSelection));
  hls.js uses 0.95 of it to stay and 0.7 to climb (`abrBandWidthFactor`, `abrBandWidthUpFactor`,
  [hls.js API](https://hlsjs.video-dev.org/api-docs/hls.js.abrcontrollerconfig)).

**Research on oscillation.** Players sharing a link with other traffic underestimate it and pick too
low ("Confused, timid, and unstable",
[Huang et al., IMC 2012](https://cs.uwaterloo.ca/~brecht/courses/854-http-video-2012/readings/Picking-a-Video-Streaming-Rate-is-Hard-imc-2012.pdf));
two players on one link make each other unstable
([Akhshabi et al., NOSSDAV 2012](https://sites.cc.gatech.edu/people/home/dovrolis/Papers/final-nossdav12.pdf)).
Viewers mind down-switches and frequent switches; a single abrupt up-switch is not found to hurt
(Seufert et al.'s survey and Hoßfeld et al., QoMEX 2014, as summarised in
[Garcia et al.](https://link.springer.com/10.1186/1687-1499-2014-216); effect sizes UNVERIFIED).

**Common complaints.** Plex's "Original Quality" beginning as a very low transcode, fixed only by
choosing 720p and then Original again
([forum 888575, 2024-09-17](https://forums.plex.tv/t/original-quality-isnt-really-original-quality-when-starting-video/888575),
no staff answer); Auto "never actually adjusts anything"
([forum 826939](https://forums.plex.tv/t/automatically-adjust-quality-never-actually-adjusts-anything/826939)).
People notice when a choice they made is not what plays.

**Pitfalls.**

- **The move up is not a rung change.** Going from the ladder to the file is a new delivery: a
  reload, new buffers, possibly another connection. Its cost is a stall the person did not ask for,
  so it must be rarer than a rung change.
- **A ladder rung measures the link at that rung's rate.** A player fetching 1.2 Mbps chunks rarely
  sees whether 1.7 Mbps would hold; idle gaps between chunks hide capacity. "The link carried the
  file's rate" needs a measure taken while fetching faster than playback (the load ahead, or a probe).
- **Variable-rate files.** A file's average can sit well under a scene's peak; a move at the
  average stalls at the first busy scene.
- **Flapping.** Up, stall, down, up again is the worst case the research names; a move that failed
  once must not be retried at once.
- **A silent move breaks the person's mental model** after they chose the original; Plex's
  complaints above are of exactly this.

**Options for the owner.**

1. **Short hold:** move after about 10 s of buffer with throughput at or above the file's rate ÷ 0.7
   (the players' defaults). Fast; most likely to flap on a marginal link like VQ337's.
2. **Long hold:** move after the link has carried at least 1.3× the file's average for about 60 s of
   fetching, with the buffer full; at most one try per film; a failed try (a drop within, say, two
   minutes) ends the attempts for that film. Slower to reach the original; the film looks good
   sooner on the ladder's top anyway.
3. **Move at a natural break:** as 2, but take the move at a chapter or a scene cut, or while paused.
   Hides the reload; needs scene data the core may not have.
4. **Say it:** ⋯'s line reads "Playing 720p until the link carries the original" and then "Original";
   no toast. Can be combined with any of 1–3.

**Recommendation (the author's):** 2 with 4. The research's point is that the up-switch itself is not
what hurts; the stall after it is, and a delivery change stalls more easily than a rung change.

## 3. What a cast target may do, and Up next across caster and receiver

**Orpheum now.** The owner, 2026-11-13 (plan, "The owner's answers on the 11-25 batch's reports",
items 3 and 9): a cast target may do what it is capable of, and then the caster and the target can
both do it; "A target without input, as a Chromecast, only shows"; "A Chromecast has no remote, so it
shows Up next on screen, but Up next is picked from the caster." Casting is a session with a device
([D364](../../decisions/working-notes.md)); a browser receiver may take presses
([D468](../../decisions/working-notes.md)).

**What competitors do.**

- **Google's Cast Autoplay checklist**, all items marked Required: "The receiver displays a countdown
  timer and the sender displays a static 'Up Next' message to prevent timing issues"; it appears "at
  least 30 seconds before the end of the video" or when the credits start; Stop "stops automatic
  playback ... and finishes content currently playing", and "does not clear the queue"; an "Are you
  still watching?" stays 10–20 minutes; a queue view can reorder, remove and clear
  ([developers.google.com](https://developers.google.com/cast/docs/design_checklist/cast-autoplay)).
- **Google's receiver checklist**: "Do not present elements as interactive controls"; stop after
  5 minutes idle and 20 minutes paused
  ([receiver](https://developers.google.com/cast/docs/design_checklist/receiver)). The **sender
  checklist**: every sender's controls stay in sync "even when another sender or a remote control
  made the change"; leaving the controller never stops the cast; a dropped sender leaves the film
  playing ([sender](https://developers.google.com/cast/docs/design_checklist/sender)).
- **Netflix** has stopped casting from phones to Chromecast with Google TV and the Google TV Streamer
  ([Android Authority](https://androidauthority.com/netflix-casting-chromecast-google-tv-streamer-3620784/),
  late 2025). Its replacement, Companion Mode (beta), leaves the TV app the player and makes the
  phone a remote that can "jump to another episode" and change subtitles
  ([help.netflix.com/en/node/132821](https://help.netflix.com/en/node/132821)).
- **YouTube**: phone and TV share one TV queue; the phone adds and removes items
  ([support.google.com/youtube/answer/7583931](https://support.google.com/youtube/answer/7583931)).
  Since 2026-09-02 the phone controls the TV when both are signed in to the same account, without
  the same Wi-Fi ([PPC Land](https://ppc.land/youtube-drops-shared-wi-fi-from-phone-control-of-tv-playback/)).
- **Plex**: Plex Companion lets one app drive another on the local network
  ([support.plex.tv/?p=38685](https://support.plex.tv/?p=38685)); the queue is the player's, and
  controlling another player takes over or overwrites it
  ([forum 568106](https://forums.plex.tv/t/manage-a-currently-playing-device-queue-from-another-device/568106)).
- **Jellyfin SyncPlay**: pause, seek and next item act on the whole group
  ([API docs mirror](https://eb48e63b.jellyfin-org.pages.dev/developers/docs/api/syncplay)); the
  admin sets per user whether they may create or join groups (UNVERIFIED: from the author's memory of
  the user policy, not a page fetched).

**Common complaints.** Plex's autoplay while casting: the countdown appears, then returns to the
library ([forum 677689](https://forums.plex.tv/t/plex-web-app-fails-to-load-next-episode-automatically/677689));
Up next hangs with no timer on Shield and Chromecast
([forum 834614](https://forums.plex.tv/t/autoplay-next-episode-wont-work/834614)); the phone's
autoplay setting not applied to the Chromecast
([forum 200907](https://forums.plex.tv/t/please-add-a-setting-to-disable-automatic-advance-to-next-video/200907)),
later fixed per staff ([forum 230072](https://forums.plex.tv/t/autoplay-chromecast-playback-issues/230072)).
All three are the same fault: two sides each holding their own idea of what comes next.

**Pitfalls.**

- **Two countdowns drift.** Google's reason for a static label on the sender is this exact bug.
- **Two queues disagree.** If the caster picks Up next and the receiver plays it, a caster that
  sleeps, loses signal or is closed must not leave the receiver with nothing next or with a stale
  pick; the sender checklist requires playback to go on without the sender.
- **Whose settings apply?** Autoplay on or off, skip intro, "still watching" are the caster's
  person's, not the receiver's; Plex's bug was exactly a setting not carried.
- **A capable target, two remotes.** When both a TV's remote and the phone can press Next, two
  presses in a second must not skip two episodes: the core must order them.
- **A guest target** ([D469](../../decisions/working-notes.md)) must not see the caster's queue beyond
  what it will play.

**Options for the owner.**

1. **The core keeps the session's queue and countdown deadline;** caster and receiver both render
   it; either may act if it can, and the core orders the acts. One truth, as YouTube's TV queue.
2. **The caster owns Up next,** as now planned, and sends each next item to the receiver. Simple
   while the phone is awake; fails Google's "keeps playing without the sender" when it is not.
3. **The receiver owns Up next** once given the series and the person's settings, as Netflix's TV
   app does. Robust; duplicates the choosing logic on the receiver.

**Recommendation (the author's):** 1. It keeps the owner's "picked from the caster" in effect (the
caster's person's settings decide), follows Google's countdown rule, and survives a sleeping phone.

## 4. Large libraries: whole-list text versus paging, and the letter rail

**Orpheum now.** Pages of 60 carry the list's total; a letter jump asks for the letter's first title
and then reads pages one after another until it is loaded, which the owner saw take 1–3 s (plan,
"The alphabet rail's slow jump"). The owner confirmed 2026-11-11 an up-front fetch of the whole
list's names and years, posters following as they near the view, split later "if we get issues with
larger libraries" (plan, item 3, "The blank poster").

**What competitors do.**

- **Jellyfin** added pagination to the web client in 10.9.0 (PR #4784,
  [release notes](https://newreleases.io/project/github/jellyfin/jellyfin-web/release/v10.9.0)):
  100 per page by default, per user and per client, 0 meaning all, which a forum member called "not
  recommended"; a project owner warned that 20,000+ items "will likely make your browser struggle"
  ([forum](https://forum.jellyfin.org/t-library-page-size-default-for-all-users)). A request for
  pageless scrolling has been open since 2019 ([forum](https://forum.jellyfin.org/t-infinite-scroll)).
- **Emby**'s alpha picker filters the list to the letter; in 2017 staff called the old
  toggle-back a bug: "If you click 'R' then you should go to the Rs"
  ([Emby forum](https://emby.media/community/topic/48108-alpha-picker-new-behaviour/)).
- **Plex**'s server pages by offset (`X-Plex-Container-Start`, `-Size`), can return the total alone
  with a size of 0, and warns "the response might not be paginated at all"
  ([developer.plex.tv](https://developer.plex.tv/pms/)); it also groups a section by
  `firstCharacter` ([plexpy SDK](https://github.com/LukeHagar/plexpy/blob/master/docs/sdks/library/README.md)),
  which is what a letter rail needs: each letter's count, hence its offset.
- **Infuse 8.1** added "an alphabet scrollbar to quickly jump between sections" on iPhone, iPad and
  Apple TV; it hides for short lists and non-alphabetical sorts
  ([Firecore](https://community.firecore.com/t/infuse-8-1-alphabet-scrollbar/54109)).
- **Apple's tvOS** collection view has the index built in (`indexTitles(for:)`, tvOS 10.2+,
  [Apple](https://developer.apple.com/documentation/uikit/uicollectionviewdatasource/collectionview(_:indexpathforindextitle:at:))).
- **Kodi** jumps by Shift and a letter on a keyboard; remote-only users have long asked for more
  ([Kodi forum](https://forum.kodi.tv/showthread.php?tid=45502)).

**Common complaints.** Plex's jump landing on the wrong letter in libraries of 600 to 1,400 titles
([forum 111320](https://forums.plex.tv/t/bug-jump-to-letter-completely-wrong-in-large-library/111320),
[91871](https://forums.plex.tv/t/jump-to-letter-broken/91871)); tvOS scrolling resetting to the
start ([forum 665849](https://forums.plex.tv/t/scrolling-in-plex-for-tvos-atv4k/665849)), again in
the Summer 2026 server ([forum 943116](https://forums.plex.tv/t/summer-2026-server-update-bugs/943116));
Roku's new app with "just a slider bar with no letters" (Oct 2025,
[forum 932720](https://forums.plex.tv/t/pagination-added-to-category-pages/932720)); a Plex Web build
looping requests and drawing hundreds of blank posters on a fast scroll to the end
([forum 857752](https://forums.plex.tv/t/latest-plex-web-update-added-hundreds-of-black-blank-posters-to-end-of-movie-library/857752));
Jellyfin 10.11 libraries of 20,000 not loading
([forum](https://forum.jellyfin.org/printthread.php?tid=14171)) and a count reading "1-50 of 50"
([forum](https://forum.jellyfin.org/t-solved-library-pages-not-working-as-expected)).

**Pitfalls.**

- **The rail and the list must sort alike.** "The", accents, digits and a sort title differing from
  the title are where a letter's offset goes wrong; Plex's wrong landings look like this
  (UNVERIFIED as their cause). One collation, the server's, must give both.
- **A list that changes while open.** A scan adding titles shifts every offset after them; the total
  on each page (the owner's item 1) detects it, but the rail's offsets must be refreshed with it.
- **Size.** Names and years for 1,000 titles are tens of kilobytes; 20,000 are around a megabyte
  before compression (the author's estimate, not measured). On a Fire TV stick the cost is less the
  bytes than parsing and holding 20,000 objects; the DOM must still be virtualised.
- **Filter versus jump.** Emby filters, Plex and Infuse scroll. Filtering hides that the person is
  inside a filter (Emby's 2017 complaint); jumping keeps context.
- **Letters with nothing** must be shown disabled or skipped, not land on the next letter silently.

**Options for the owner.**

1. **Whole list's text up front, as decided;** the rail's letter offsets are computed on the client
   from it, so a jump is local and instant. Simplest; cost grows with the library.
2. **A letter index from the server** (each letter's count and first offset, as Plex's
   `firstCharacter`), with one direct page fetch at that offset. Cheap at any size; two requests a
   jump; needs the server's collation.
3. **Both:** 1 below a threshold (say 5,000 titles), 2 above it. Best everywhere; two paths to test.

**Recommendation (the author's):** 1 now, as the owner decided, but with the offsets computed from
the fetched list and refreshed when a page's total changes; add 2 only when a beta library proves 1
too slow on the stick.

## 5. Guest receivers and signing in on a TV by code

**Orpheum now.** A device not signed in shows an eight-character code for ten minutes; a signed-in
person enters it; both screens then show a check code, with a plain warning to compare it with their
own eyes ([D288](../../decisions/working-notes.md)). Scanned from a phone, the code offers Claim this
device or Cast to this device, the latter a guest receiver for one cast, ended when idle 30 minutes
([D469](../../decisions/working-notes.md)). Open there: the wording, and Cast for a typed code.

**What competitors do.**

- **OAuth device authorization grant**, [RFC 8628](https://www.rfc-editor.org/rfc/rfc8628), the
  pattern behind most TV sign-ins: the device shows a short user code; the person approves it on a
  signed-in device. §6.1 suggests eight letters from "BCDFGHJKLMNPQRSTVWXZ" ("WDJB-MJHT").
- **Plex**: "a 4-character link code" entered at plex.tv/link, or a QR code on some platforms; "your
  player app will refresh itself a few seconds later and be linked with your account"
  ([support.plex.tv](https://support.plex.tv/articles/203395277-connect-app-to-your-plex-account/)). No
  confirmation on the TV is described.
- **Jellyfin Quick Connect**: the admin enables it; the new client shows a six-digit code, entered in
  a signed-in client's settings ([jellyfin.org](https://jellyfin.org/docs/general/server/quick-connect));
  codes last about ten minutes (server source, UNVERIFIED against a release).
- **Netflix**: the TV shows a QR code; the phone opens it and taps a button to sign the TV in; or
  netflix.com/tv2 ([help.netflix.com](https://help.netflix.com/en/node/311830241325668)).
- **YouTube**: "Link with TV code" pairs a phone as a remote without signing the TV in; guests can use
  the TV signed out, which "won't affect your account's recommendations"; linked devices can only be
  unlinked all at once ([support.google.com](https://support.google.com/youtube/answer/7583931)).
- **Chromecast's guest mode** (a 4-digit PIN sent by inaudible tones, reset daily) was removed in
  August 2023 without a stated reason ([9to5Google](https://9to5google.com/2023/08/23/chromecast-guest-mode/)).

**Common complaints.** Codes that expire before they can be typed
([Plex forum 944170](https://forums.plex.tv/t/plex-link-code-expires-too-fast/944170)); a QR flow stuck
"checking code" ([forum 860729](https://forums.plex.tv/t/plex-stuck-at-sign-in-link-qr-checking-code/860729));
a TV joined to whichever account the browser happened to be signed in to
([Techdows](https://techdows.com/how-to-enter-the-tv-code-on-plex-tv-link.html), third-party).

**Pitfalls.**

- **Remote phishing is the live attack.** RFC 8628 §5.4: an attacker starts a flow on their own
  device and sends the victim the code; "it is RECOMMENDED to inform the user that they are
  authorizing a device" and to show the device's details. It is no longer theoretical: Microsoft
  disclosed Storm-2372's device-code phishing in February 2025, through fake meeting invitations
  ([Microsoft](https://www.microsoft.com/en-us/security/blog/?p=146350)), and the Cloud Security
  Alliance counted 340+ organisations hit by March 2026
  ([CSA](https://labs.cloudsecurityalliance.org/research/csa-research-note-oauth-device-code-phishing-m365-20260325-c/)).
  For Orpheum the prize is a stranger's device signed in as the person, with their library.
  D288's check code defeats it only if the person compares two screens they can both see; it
  is not proof against someone who reads the check code to them on a call, which is why D288's
  warning matters.
- **Guessing.** §5.1 asks that rate limits allow only a handful of guesses per code life. Plex's four
  characters (about 1.7 million codes) and Jellyfin's six digits (one million) rest on that alone;
  Orpheum's eight safe32 characters (about 2^40) rest less on it, and D288 §1 leaves limiting codes
  to the paid discovery.
- **Session spying** (§5.5): a code on a TV in a shared room can be approved by anyone who sees it
  first; the check code covers this too.
- **A QR code skips typing, and so skips the moment of reading.** §5.4 notes the complete-URI
  shortcut makes confirming possession more important, not less.
- **Guest leftovers.** Netflix and Plex leave a signed-in TV behind; a guest receiver that removes
  itself (D469 §3) avoids the hotel-TV problem the others have.

**Options for the owner.**

1. **Keep D288 and D469 as built.** Already stricter than Plex and Jellyfin.
2. **Name the device on the approving phone,** "A TV called *Living room*, on a network in
   *country/ISP*, asked 2 minutes ago", as RFC 8628 §5.4 recommends. Costs an address lookup and care
   not to leak the address itself.
3. **Offer Cast to this device for a typed code too,** closing D469's open item; same risks as the
   scanned path.
4. **Shorten the life of a code that has been scanned** (for example to two minutes once opened on a
   phone), so a phished link goes stale quickly. Costs retries on slow TVs.

**Recommendation (the author's):** 1 with 2. The device's name and rough place are the one
mitigation the RFC asks for that Orpheum's flow lacks; the check code already covers the rest.

## Not tried

- No app was run: every behaviour here is from documentation, vendor posts and forums, not a device.
  None of Netflix, YouTube, Disney+ or Prime Video was observed casting, capping or climbing.
- Plex's support article on automatic quality refused a direct fetch (HTTP 403); it is quoted from
  search results. Disney+'s and Prime Video's own help pages were not reached.
- Jellyfin SyncPlay's permission model and Quick Connect's code life were not read from a release's
  source.
- Whether Chrome on Android reports a hotspot as `cellular` in `navigator.connection.type` on any
  phone was not tested; nor what Safari does with a ceiling on native HLS.
- The size of a 20,000-title list's text was estimated, not measured; Plex's `firstCharacter`
  response shape was not fetched.
- The QoE papers were read through their abstracts and a survey; their effect sizes were not
  checked.
- Apple TV's own TV app and Emby's current clients were not examined.
