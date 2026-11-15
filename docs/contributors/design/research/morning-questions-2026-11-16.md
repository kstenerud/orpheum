> **ABOUTME:** Evidence for four of the owner's open questions (what a stall is and who shares it,
> refetching metadata a provider left empty, plain versus styled subtitles in the web player, a
> loading spinner under reduced motion): what other players and servers do, the pitfalls, and options.

# Evidence for four of the owner's open questions

- **Date:** 2026-11-16 (searched and fetched 2026-11-15; notes cached in
  `/yoloai/cache/research/morning-questions-2026-11-16-notes.md`).
- **Asked by** the lead, for the owner's morning decisions. Not a repeat of
  [competitors-2026-11-16.md](competitors-2026-11-16.md), which covers data caps, the climb back to
  the original, cast rights, large lists and sign-in by code.
- **Sources:** vendor documentation and source code (hls.js 1.7.3 and JASSUB 2.5.16 read from
  `web/node_modules`), the W3C's Understanding pages, and vendor forums. Forum posts are anecdotes,
  dated where the page dates them. A claim not checked against its vendor's own page is
  **UNVERIFIED**.
- **The design is the owner's** (AGENTS.md rule 1). Every recommendation below is the research
  author's, marked as one, for the owner to accept, change or drop.

## Summary

| Question | Orpheum now | Headline finding | Recommendation (the author's) |
| --- | --- | --- | --- |
| 1. What is a stall | One `watchStalls` definition, four listeners, each filtering differently; two standing-still detectors | Every mature player keeps three things apart: the stuck-playhead fix, the bitrate controller's inputs (buffer and throughput, rarely stall events), and the QoE rebuffer count (startup and seeks counted separately) | One watcher that emits typed events (rebuffer, slow start, slow seek, fell behind; whose wait), each listener choosing kinds |
| 2. Refetching what is empty | D335 §1 decided; built only for series orderings, 20 a sweep, hourly | Nobody re-asks an empty field forever: Sonarr re-asks a placeholder title, and re-asks by age and status; Radarr stops after release; Jellyfin, Emby, Kodi leave it to a person | Stamp each fetch with what it asked for; re-ask an empty field only while the work is new, then let it stand |
| 3. Subtitles in the web | Every track drawn by JASSUB; WebVTT only in the browser's fullscreen | Jellyfin and Emby draw ASS with libass in WASM and plain text natively; Plex burns ASS in; TVs choke on effect-heavy ASS | Plain tracks as native WebVTT, styled ASS through JASSUB, the D78 overlay as the slow-device fallback |
| 4. Spinner under reduced motion | All animations set to 0 s; the spinner stands still | WCAG's own Understanding says a progress animation can be essential; Chrome's guide and Bootstrap slow a spinner rather than stop it; GitLab leaves it spinning | Slow the turn (about 3–6 s), or a pulse of opacity, which WCAG does not count as motion; keep the label |

## 1. What is a stall, and should every listener share one?

**Orpheum now.** `watchStalls` (`web/packages/player/src/stalls.ts`) counts three things as one
stall: a `waiting` lasting 1 s while playing, once the video has played since its last seek; a load
(the first or a seek's) that has left the video unable to play for 8 s, again every 8 s; and a Watch
Together fall behind the group. Its listeners differ. The bitrate controller's (`tellStalls`,
`chunked.ts`, made fresh per playback) skips a slow load while the core answers 425, still making
the chunk. The playback report's (`watchForReport`, `play-state.ts`) passes no such filter and **also calls `movingDown.stalled()`**, so D247's move down is fed
by a watcher that does count the core's own waits. That is worth checking against D247 §2's intent.
"Cannot keep up" (`ladder-playback.ts`) and the Watch Together diagnosis are the third and fourth.
Apart from these, two standing-still detectors: `StandingStill` (`abr-video.ts`, looked at every
500 ms, D266's judge of a quality the decoder cannot play while chunks arrive) and
`watchStandingStill` (`standstill.ts`, looked at every 1 s, reports after 5 s, skips paused,
seeking and ended; a report only).

**What others do.**

- **hls.js** (1.7.3, read in `web/node_modules`; upstream
  [gap-controller.ts](https://github.com/video-dev/hls.js/blob/master/src/controller/gap-controller.ts)):
  a stall is the playhead not moving for `detectStallWithCurrentTimeMs` (1250 ms), or a `waiting`
  after it has moved; tracking is cleared when a seek begins or ends, "so that we don't report stalls
  coming out of a seek"; paused and ended are never stalls. Its answer is mechanical: jump a hole,
  nudge `currentTime`, then raise `BUFFER_STALLED_ERROR` once per stall. The bitrate controller
  ([abr-controller.ts](https://github.com/video-dev/hls.js/blob/master/src/controller/abr-controller.ts))
  does **not** listen for that error: it reads throughput, time to first byte and a predicted
  `bufferStarvationDelay`, and abandons a fragment that would starve the buffer.
- **Shaka Player**
  ([stall_detector.js](https://github.com/shaka-project/shaka-player/blob/main/lib/media/stall_detector.js),
  read through [a mirror](https://ott.dolby.com/OnDelKits_dev/shaka-player-dlb/shaka-player/docs/api/lib_media_stall_detector.js.html)):
  a stall is the playhead not moving for a threshold **while it holds content** at least 500 ms
  ahead; with nothing buffered "we are not stalled, we are buffering". It skips ahead
  (`stallSkip`, 0.1 s) and counts `stallsDetected`. Buffering time is a separate statistic. Its
  `AbrConfiguration` has no stall input; the nearest is `minBufferAheadToUpgrade`
  ([shaka.extern](https://shaka-project.github.io/shaka-player/docs/api/shaka.extern.html)).
- **dash.js**: the InsufficientBufferRule caps the bitrate at safe throughput × buffer level ÷
  segment length, ignoring the first two segments
  ([dash.js docs](https://dashif.org/dash.js/pages/usage/abr/insufficient-buffer-rule.html)). Its
  older ABR wiki says the rule forces the lowest bitrate once a rebuffer happens (**UNVERIFIED**
  against current docs). It is the one player here whose ABR reacts to the empty buffer itself.
- **ExoPlayer/Media3**: the QoE side is a state machine
  ([PlaybackStats](https://developer.android.com/reference/androidx/media3/exoplayer/analytics/PlaybackStats)):
  `JOINING_FOREGROUND` ("buffering in the foreground for initial playback start"), `BUFFERING`
  ("buffering to resume active playback", the rebuffer) and `SEEK_BUFFERING` are distinct states,
  so `totalRebufferCount` and `getRebufferTimeRatio` never include the start or a seek; a pause
  during a rebuffer is its own count. Its track selection reads bandwidth and buffered duration.
- **AVPlayer** exposes `AVPlayerItemAccessLogEvent.numberOfStalls` and a playback-stalled
  notification, and `automaticallyWaitsToMinimizeStalling`; Apple's own definition of a stall was
  not found (**UNVERIFIED**).
- **Conviva**: Rebuffering Ratio counts rebuffering ÷ (play + rebuffering), excluding the time
  before the first frame, paused time and any continuous rebuffering over 90 s; it includes seek
  rebuffering ([Conviva](https://docs.conviva.ai/vsi-video/metric-dictionary/rebuffering-ratio/)).
  Its stricter **Connection Induced** ratio also excludes rebuffering that starts within 10 s of a
  seek ([CIRR](https://docs.conviva.ai/vsi-video/metric-dictionary/connection-induced-rebuffering-ratio/)).
  No minimum length: a stall under 200 ms still counts.
- **Mux**: Rebuffer Percentage is rebuffer duration ÷ watch time; seek stalls are "a separate
  metric … called Seek Latency", startup another
  ([Mux smoothness](https://www.mux.com/docs/guides/data-smoothness-metric)).
- **CMCD** (CTA-5004): `bs`, buffer starvation, a flag sent with the next request after the buffer
  ran dry, and in v2 `bsd`, its duration
  ([CTA WAVE](https://cta-wave.github.io/Resources/common-media-client-data--cta-5004-a.html)). Media3
  sent `bs` twice for one rebuffer until 1.4.0, and does not send it before the first load
  ([androidx/media#1124](https://github.com/androidx/media/issues/1124)). A third-party summary says
  CTA-2066 excludes startup from the rebuffering ratio (**UNVERIFIED**; the standard was not read).

**The pattern.** Three separate things, everywhere: (1) a **stuck playhead with data in hand**,
fixed mechanically (hls.js's gap controller, Shaka's StallDetector; Orpheum's `StandingStill` is
the same shape, judged differently, D266); (2) the **bitrate controller's inputs**, buffer level and
throughput, of which a stall is at most a late symptom (only dash.js reacts to it); (3) the **QoE
rebuffer**, counted after the first frame, outside seeks and pauses, with startup and seek time as
their own metrics. Orpheum's one `watchStalls` folds a slow start, a slow seek and a rebuffer into
one count, then each listener re-filters it.

**Pitfalls.**

- A slow start counted as a rebuffer makes a report's rate look worse than any industry figure it
  will be compared with, and drives a move down that the start, not the link, caused.
- Filters that differ per listener drift: the core's 425 wait is skipped by one and counted by the
  other, which is how the move down came to see it.
- 1 s versus 1250 ms versus Conviva's no minimum: thresholds differ by purpose; a shared definition
  needs the threshold to belong to the listener, not the event.
- A pause during a rebuffer (Media3 counts it apart) and a stall longer than 90 s (Conviva drops it as
  an outage) are both cases a single count blurs.

**Options for the owner.**

1. **One watcher, typed events.** `watchStalls` emits each event with its kind (rebuffer, slow
   start, slow seek, fell behind) and whose wait it is (link, core making the chunk, paused); each
   listener picks what it acts on, and the report sends the kind with every stall. Cost: a change in
   every listener and in the report's shape; keeps one place that defines "waiting".
2. **Keep separate watchers, rename them.** "Stall" means only the QoE rebuffer; the others become
   "slow start" and so on. Cheaper, but two definitions of "waiting for data" remain.
3. **Status quo, documented.** Write each listener's filter down and fix only the move down's
   source. Cheapest; the drift that produced it stays possible.
4. **Adopt an outside definition for the report** (Media3's states, or Conviva's ratios) alongside
   whatever the controller uses. Makes reports comparable; adds a second machine.

**Recommendation (the author's):** 1. It is the industry's separation without a second definition
of waiting: the report counts everything and says what kind (as Conviva reports both a ratio and its
connection-induced part), the bitrate controller and D247's move down take only the link's
rebuffers and slow loads, and "cannot keep up" what it takes now. The two standing-still detectors
could share one "asked to play, position not moving" primitive with their thresholds as parameters;
their purposes (D266's verdict, a report) stay separate. Whether the move down should see the core's
waits is the owner's call either way.

## 2. Refetching metadata a provider left empty

**Orpheum now.** D335 §1 decided a passive fill: the sweeps find "a field a fetch left empty, a part
a failed fetch never stored, or a field added to a type after its items were fetched" and fetch it
"as the first fetch would have", through the queue's back-off and giving up (D231), a little at a
time (§3). D335's status is "Decided; not built"; `internal/core/identify/missing.go` builds the
first kind only, a series' orderings, `missingFetchesPerSweep = 20`, at start and hourly. Background
work keeps under 80% of TMDB's rate and parks on a 429 for the time given (D231 §5). Not yet said: what
ends the re-asking when the provider's answer is genuinely empty, which D231's "give up" does not
cover, since an empty answer is a success.

**What others do.**

- **Sonarr** (`ShouldRefreshSeries`,
  [source](https://raw.githubusercontent.com/Sonarr/Sonarr/develop/src/NzbDrone.Core/Tv/ShouldRefreshSeries.cs)):
  refresh if the last sync is over 30 days old, or an aired episode is still titled "TBA" (a
  placeholder: Sonarr's "field left empty"); else not if synced within 6 hours; refresh a series not
  ended, or ended with its last episode aired within 30 days; otherwise never automatically. An
  error refreshes anyway.
- **Radarr** (`ShouldRefreshMovie`,
  [source](https://raw.githubusercontent.com/Radarr/Radarr/develop/src/NzbDrone.Core/Movies/ShouldRefreshMovie.cs)):
  refresh if over 180 days old; not within 12 hours; refresh while Announced or In Cinemas, or
  Released with its physical release within 30 days; otherwise not.
- **Jellyfin**: a person chooses "Search for missing metadata" or "Replace all metadata"; a library
  option, `AutomaticRefreshIntervalDays` (Never, 30, 60, 90), refreshes on age
  ([SDK](https://typescript-sdk.jellyfin.org/interfaces/generated-client.LibraryOptions.html),
  [forum](https://forum.jellyfin.org/t-refresh-metadata-every-30-days-doesn-t-seem-to-be-updating)).
  Users complain that Replace all drops films from their collections
  ([forum](https://forum.jellyfin.org/t-solved-possible-bug-refreshing-metadata-will-remove-items-from-collections)),
  and 10.11.2 left new items without titles until a manual refresh, fixed in 10.11.3
  ([forum, 2025-11](https://forum.jellyfin.org/t-solved-missing-metadata-for-new-items-until-force-refreshed)).
- **Emby**: no missing-only scheduled task; staff say the library scan "only looks for new and
  updated files" ([forum](https://emby.media/community/topic/20390-mbs-scheduled-task-to-find-and-populate-missing-metadata-files)).
  It throttled TMDB to about 40 requests per 10 s, ten times below TMDB's limit, until a user asked
  ([forum, 2024-06-19](https://emby.media/community/topic/129995-tmdb-throttling/)).
- **Plex**: "Refresh metadata periodically" refreshes music artists and TV shows "over the course
  of the month", a rolling spread; whether films are included is disputed on its forum
  ([forum](https://forums.plex.tv/t/refresh-metadata-periodically-how-often/222084)). Its support page
  refused the fetch (403); **UNVERIFIED** against Plex's own text.
- **Kodi**: per-item Refresh only; artwork whose URL has died stays missing until someone refreshes
  ([OSMC](https://discourse.osmc.tv/t/disappearing-artwork-infomation/91009)).
- **TMDB**: the 40-per-10-s limit went in 2019; now "somewhere in the 40 requests per second range",
  and clients should respect 429
  ([TMDB](https://developer.themoviedb.org/docs/rate-limiting)).

**Pitfalls.**

- **The genuinely empty field.** An obscure film with no overview on TMDB is "missing" forever; a
  sweep that only looks at the field asks every hour. Nobody above re-asks without a stopping rule:
  Sonarr and Radarr stop by age and status, the others by never asking unbidden.
- **A field added to a type** makes every item missing at once. At 20 an hour, 480 a day, a
  2,000-item library fills in about four days: gentle, but the owner should know the figure, and
  a person's priority (D324) must still jump it.
- **Missing versus provider-dropped.** D213 keeps what a provider dropped; a passive fill must not
  turn a dropped field into an endless re-ask either.
- **Replace all destroys hand edits** (Jellyfin's collections); D335 §2 already keeps a person's
  corrections by default.

**Options for the owner.**

1. **Stamp each fetch with what it asked for** (a version of the item type's fields). The sweep
   re-asks an item only when its stamp is older than the type; an empty answer to a current stamp
   stands. Ends cleanly; covers "a field added to a type", not "a provider filled it in later".
2. **Per-field asked-at with a decaying schedule:** re-ask an empty field after, say, 7, 30 and 180
   days, then stop. Covers late fills; one more fact per item and field.
3. **Recency, as Sonarr and Radarr:** re-ask empty fields while the work is new (released within
   about 90 days, or a series continuing), never once it is old. Matches where TMDB fills in later.
4. **Never re-ask an empty answer unbidden;** only a person's refresh (D335 §2). Simplest; leaves
   the "left empty" half of D335 §1 to people.

**Recommendation (the author's):** 1 with 3. The stamp answers "a field added to a type" exactly and
lets an empty answer stand; the recency rule re-asks where the provider is still filling in, which
is where an empty field is likely to change; anything older waits for a person's refresh. Keep the
count of 20 a sweep, and show the backlog on the System page so a schema change's four days are
visible.

## 3. Subtitles in the web player: native tracks or JASSUB

**Orpheum now.** The node converts every subtitle, and never shows it as found (D211 §3); the web
draws every chosen track with JASSUB (D217 §4; `web/packages/player/src/subtitles.ts`), and gives
the same track as WebVTT captions only for the browser's own fullscreen, where the canvas is not
seen (D254). JASSUB is built with `queryFonts: false` and no font files, so D55's served font set and
a file's own font attachments do not reach it. The core sends no COOP/COEP headers, so JASSUB runs
single-threaded (its README). D78's server-rendered overlay is provisional; the cast receiver ships
no renderer (D137).

**What others do.**

- **Jellyfin web** draws ASS with libass in WASM. It moved from SubtitlesOctopus to JASSUB and **back**:
  "JASSUB has some problems with non-Latin languages", fallback fonts broken
  ([PR #5296, 2024-03-24](https://github.com/jellyfin/jellyfin-web/pull/5296);
  [release notes](https://newreleases.io/project/github/jellyfin/jellyfin-web/release/v10.9.0)).
  Fallback fonts are served as WOFF2 from the server for the web client
  ([forum](https://forum.jellyfin.org/t-solved-how-to-make-ass-subtitles-with-external-fonts-work)).
- **Emby web** draws ASS with SubtitlesOctopus; in 2024 it still drew with the default font,
  ignoring a file's embedded ones ([forum](https://emby.media/community/topic/131842-use-ass-subtitles-embedded-fonts-in-web-app)).
- **Plex web** draws no ASS: it burns it in by transcoding, or, set to "Only image formats", plays
  it as plain text, losing "position and color"
  ([forum](https://forums.plex.tv/t/rendering-of-complex-soft-subtitles-ssa-ass-formats-in-plex-web-anime/159143)).
- **TVs**: plain ASS is fine on a Sony Android TV, "fancy subs with effects … brings my TV to its
  knees" ([Jellyfin forum](https://forum.jellyfin.org/t-android-tv-lag-with-some-ass-embedded-subtitles));
  an LG C4 stutters on new episodes with ASS only in its TV app
  ([forum](https://forum.jellyfin.org/t-lg-c4-tv-jellyfin-app-lagging-on-new-anime-with-ass-subtitles)).
  Burn-in on a weak server cannot keep up either. No Fire TV measurement of JASSUB was found.
- **JASSUB's own knobs** (README in `web/node_modules`, [npm](https://npmjs.com/package/jassub)):
  `prescaleFactor` below 1 trades sharpness for speed; `blendMode: 'wasm'` "will perform better on
  lower end devices"; threads need COOP/COEP, else one thread. It needs OffscreenCanvas, workers and
  `requestVideoFrameCallback`: Safari 17 (16.2 with flags), Chrome 80, Firefox 105.
- **Native WebVTT's limits:** Safari ignores some cue styling delivered through HLS
  ([WebKit 238540](https://bugs.webkit.org/show_bug.cgi?id=238540)), and cues can be illegible in
  iOS fullscreen ([WebKit 287867](https://bugs.webkit.org/show_bug.cgi?id=287867)); on iOS the person's caption settings govern the look
  ([Brightcove](https://beacon.support.brightcove.com/media/captions/styling-captions.html)), which
  is also the accessibility gain: native captions honour the system caption style, a canvas does not.

**Pitfalls.**

- **CPU for nothing:** a dialogue-only track (most SRT) costs a WASM rasteriser and a canvas
  composite per frame on a TV whose decoder is already busy, where a `<track>` costs almost nothing.
- **Fonts:** without D55's set, a CJK or Cyrillic track falls back to JASSUB's bundled default,
  which is Jellyfin's exact regression; a file's attachments are ignored.
- **Two renderers, two looks:** a person switching between window and fullscreen already sees both
  (D254); a split by track adds no new mismatch for plain tracks, which look alike either way.
- **Safari:** older iPads below 17 have no OffscreenCanvas by default; JASSUB then fails, and only
  the native path draws.

**Options for the owner.**

1. **Status quo, tuned:** JASSUB for all, with `blendMode: 'wasm'` and a `prescaleFactor` on weak
   devices, and D55's fonts wired in. One renderer; the TV still pays for plain dialogue.
2. **Split by the track:** a track whose ASS is plain (one default style, no positioning or
   effects, as every SRT conversion is) plays as native WebVTT; a styled track through JASSUB. Cheap
   on TVs, honours system caption styles, keeps authored styling.
3. **Plus a measured fallback:** where JASSUB's drawing costs dropped frames (the player already
   reads frame counts, `frames.ts`), switch a styled track to D78's server overlay, or to burn-in.
4. **By device class:** TVs always native, styling lost there. Simple; loses styled tracks on the
   devices a household watches on most.

**Recommendation (the author's):** 2, then 3 once D78 is built. The node already knows, at
conversion, whether a track carries styling, so the choice is a flag it stores, not a guess at play
time. Wire D55's fonts into JASSUB in either case; Jellyfin's revert shows that the font path, not
the rasteriser, is where JASSUB goes wrong for non-Latin text.

## 4. Reduced motion and the loading spinner

**Orpheum now.** `tokens.css` sets every animation and transition to 0 s under
`prefers-reduced-motion: reduce`, and `film-waits.css` sets the spinner's animation to none, so
the ring stands still with its lit quarter; it carries `aria-label` "getting ready" inside an
`aria-live` region. The owner has macOS Reduce motion on, so the spinner he sees never moves, and an
opacity pulse (`title-pulse`) stops too.

**What others do.**

- **WCAG 2.3.3** (Animation from Interactions, AAA) covers motion "triggered by interaction" unless
  essential; "motion animation does not include changes of color, blurring, or opacity"
  ([Understanding 2.3.3](https://www.w3.org/WAI/WCAG22/Understanding/animation-from-interactions.html)).
  **WCAG 2.2.2**'s Understanding: an animation in a preload phase "can be considered essential"
  where "not indicating progress could confuse users or cause them to think that content was frozen
  or broken" ([Understanding 2.2.2](https://www.w3.org/WAI/WCAG22/Understanding/pause-stop-hide.html)).
- **Apple HIG:** under Reduce Motion, reduce "automatic and repetitive animations, including zooming,
  scaling, and peripheral motion"; replace movement along the axes "with fades"
  ([HIG Accessibility](https://developer.apple.com/design/human-interface-guidelines/accessibility)).
  It says nothing of activity indicators.
- **Chrome's spinner guide:** "Slow down the animation significantly rather than stopping it
  entirely": 1.5 s a turn becomes 6 s, and the arc's dash animation stops
  ([Chrome](https://developer.chrome.com/docs/modern-web-guidance/guides/ui-components/spinner)).
- **Bootstrap 5:** "meaningful animations (such as spinners) will be slowed down", to twice the
  duration ([accessibility](https://getbootstrap.com/docs/5.3/getting-started/accessibility/),
  [PR #31882](https://github.com/twbs/bootstrap/pull/31882)).
- **GitLab Pajamas:** the spinner is "not changed by prefers-reduced-motion" since it is "essential
  to understanding the state of the application", citing 2.2.2 Note 4; shown only after 100 ms
  ([Pajamas](https://design.gitlab.com/components/spinner)).
- **eBay Evo:** its indeterminate bar stops resizing and moves more slowly
  ([Evo](https://opensource.ebay.com/evo-web/components/progress-bar-expressive/accessibility)).
- **Material / Android:** no official Material 3 rule was found; Compose's indicator reportedly
  shows a still dot when the system removes animations (**UNVERIFIED**,
  [Kotlin Slack](https://slack-chats.kotlinlang.org/t/5048569/just-got-some-reports-that-animations-no-more-runs-on-some-d?t=v2)).
- **YouTube, Netflix, GitHub:** no source found for what their spinners do under reduced motion;
  Primer documents the spinner's screen-reader behaviour only
  ([Primer](https://primer.style/product/components/spinner/accessibility)). A device test would tell.

**Pitfalls.**

- **A still spinner reads as frozen:** WCAG's own reason for calling progress essential, and what the
  owner sees.
- **The blanket 0 s reset** stops what WCAG does not count as motion (opacity pulses, colour), and
  makes every per-component reduced-motion choice moot unless it overrides `!important`.
- **`animation: none` on a native indeterminate `<progress>`** leaves an empty bar in Chromium
  (Chrome's progress guide); Orpheum's ring is a span, so not hit today.
- **Peripheral, fast, large** motion is what Apple names; a 3.5 rem ring in the middle of the
  picture, slowed, is none of these.

**Options for the owner.**

1. **Slow the turn:** one turn in about 3–6 s under reduced motion (Chrome 6 s, Bootstrap 2×).
   Still motion, but slow and small; the most common practice found.
2. **Pulse instead of turning:** the ring's lit quarter fades in and out (opacity), which 2.3.3
   does not count as motion and Apple's "fades" match.
3. **Still ring with text:** the ring stands and "Getting ready" shows beside it. No motion at all;
   text over a film is heavier, and a still sign still cannot say it is alive.
4. **Ignore the preference for this one indicator** (GitLab). Defensible by 2.2.2 Note 4; goes
   against what a person who turned it on asked.

**Recommendation (the author's):** 2, or 1 if the owner prefers the ring to keep its shape. Either
way the global reset should exempt the spinner (or stop forcing `animation-duration` and let each
component choose), since a reduced-motion user is exactly who most needs to tell "loading" from
"frozen".
