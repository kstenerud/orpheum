> **ABOUTME:** Behaviour of platforms and tools Orpheum depends on but cannot change — browsers,
> hls.js, WebRTC, FFmpeg and codecs, the Linux kernel and its traffic shaping, Docker, SQLite and
> sqlc, Go, test and dev tooling — that is buggy, flaky, surprising or contrary to its own
> documentation, and what we do about each. Read it before diagnosing a platform problem; the
> symptom index maps what you see to an entry.

# Idiosyncrasies

Durable facts about **technology we depend on but cannot fix**, where it contradicts its docs,
behaves surprisingly, or forced a non-obvious workaround. The first reference when a browser, a
library or the kernel misbehaves (the owner, 2026-10-08, after the yoloAI document of the same kind).

**The inclusion test.** If every line of Orpheum's code were deleted, would the surprising behaviour
still exist in the tool? If yes, it belongs here; the entry documents the external constraint, not
our workaround. Our own bugs do not belong: they live in commit messages, decisions and
[agent-failures.md](agent-failures.md).

**How to use.** Scan the symptom index, then read the entry.

**When you suspect a new one, search for it before building on it** (the owner, 2026-10-08). Look up
the symptom and the error text online: the project's issue tracker and changelog, browser bug
trackers (Bugzilla, WebKit, Chromium), the spec, forums. Find whether others know it, what causes
it, which versions it affects, and what they tried and what worked. Someone else's diagnosis and
workaround can save hours of chasing ghosts, and a known bug may already have a fix to wait for or
a version to pin. Cite what you found as **[web]** evidence; a search that found nothing is worth a
line too, so the next agent does not repeat it.

**When you find a new one,** apply the inclusion test, add the entry under its technology with **Symptom**, **Cause** (say "unverified"
where it is), **What we do** (code paths or decision IDs) and **Evidence** (owner's test, staging,
measurement, source, or web), and add its row to the index, in the commit with the workaround.
Tags in Evidence: **[owner]** the owner's test, **[staging]**, **[measured]** here, **[source]** the
project's own code, **[web]** docs or issues only, **[doc]** documented but surprising.

Behaviour browsers differ on is designed for per browser, by feature detection, never by
User-Agent (D263; the User-Agent is logged only).

---

## Symptom index

| Symptom | Entry |
|---|---|
| At a discovery's page at home, every request failed with "NetworkError when attempting to fetch resource." after a realm sign-in. | [Firefox fails a public page's requests to a LAN name until its local-network prompt is answered, and again after a grant not remembered](#firefox-fails-a-public-pages-requests-to-a-lan-name-until-its-local-network-prompt-is-answered-and-again-after-a-grant-not-remembered) |
| `dig CAA` shows CAs nobody wrote beside the zone's own `issue ";"`. | [Cloudflare adds its own CAA records to any zone with CAA while Universal SSL is on](#cloudflare-adds-its-own-caa-records-to-any-zone-with-caa-while-universal-ssl-is-on) |
| *Flow* (10-bit HEVC by x265) froze at the first frame of any chunk but the film's first. | [Firefox on macOS freezes on HEVC chunks that start on a CRA keyframe](#firefox-on-macos-freezes-on-hevc-chunks-that-start-on-a-cra-keyframe) |
| After a jump, the sound's first chunk is asked for only once the picture's has come. | [hls.js asks for a jump's sound only after its picture](#hlsjs-asks-for-a-jumps-sound-only-after-its-picture) |
| The browser takes a fragment's append without error but adds nothing to the buffer. | [hls.js reports a fragment the browser held nothing of, then does nothing about it](#hlsjs-reports-a-fragment-the-browser-held-nothing-of-then-does-nothing-about-it) |
| After a quality is given up at 0:00, only the sound loads again: the picture's loader asks for nothing. | [hls.js leaves its picture loader idle after `recoverMediaError` when a chunk was loading](#hlsjs-leaves-its-picture-loader-idle-after-recovermediaerror-when-a-chunk-was-loading) |
| `RTCRtpReceiver.getCapabilities("audio")` lists no `audio/red`, so plain Opus is negotiated and voice has less loss protection. | [Firefox offers no RED (redundant audio) for Opus](#firefox-offers-no-red-redundant-audio-for-opus) |
| Voice hung at "Asking for your microphone" because opening the microphone awaited the context's resume. | [Firefox keeps `AudioContext.resume()` pending until it lets the page play sound](#firefox-keeps-audiocontextresume-pending-until-it-lets-the-page-play-sound) |
| A tab throttled to "Slow 4G" in DevTools still loaded a film at full speed, and its player never stepped down. | [Chrome DevTools' network throttling does not slow a service worker's requests](#chrome-devtools-network-throttling-does-not-slow-a-service-workers-requests) |
| With voice on, the film was turned down and the volume buttons stopped changing its volume. | [Firefox on Android makes a "call" of gain control and noise suppression, even with echo cancelling off](#firefox-on-android-makes-a-call-of-gain-control-and-noise-suppression-even-with-echo-cancelling-off) |
| A volume slider on an iPhone moves only when the touch lands on its thumb. | [iPhone Safari moves a range slider only by a touch that starts on its thumb](#iphone-safari-moves-a-range-slider-only-by-a-touch-that-starts-on-its-thumb) |
| The position reads as not advancing, which looks like false drift and causes spurious nudges. | [Safari's `currentTime` stands still for up to about 250 ms after play, seek or a rate change](#safaris-currenttime-stands-still-for-up-to-about-250-ms-after-play-seek-or-a-rate-change) |
| Each rate change may make the sound skip. | [Safari's rate nudges stutter or skip, especially with pitch-keeping on](#safaris-rate-nudges-stutter-or-skip-especially-with-pitch-keeping-on) |
| `window.MediaSource` is undefined on iPhone, so a probe that uses only MSE misses ManagedMediaSource, and hls.js cannot run below 17.1. | [iPhone has no `MediaSource`, only `ManagedMediaSource`, and only from iOS 17.1](#iphone-has-no-mediasource-only-managedmediasource-and-only-from-ios-171) |
| `streaming` goes false at 30 s buffered and back true below 10 s, overriding hls.js's `maxBufferLength`. | [iPhone's ManagedMediaSource decides how far ahead the page may load (about 30 s)](#iphones-managedmediasource-decides-how-far-ahead-the-page-may-load-about-30-s) |
| `element.volume = x` is ignored. | [iOS: an element's volume is read-only to the page, and Web Audio output follows the ring/silent switch](#ios-an-elements-volume-is-read-only-to-the-page-and-web-audio-output-follows-the-ringsilent-switch) |
| IPhone Safari's native HLS cannot be routed through Web Audio, so it cannot be given a gain | [iPhone Safari's native HLS cannot be routed through Web Audio, so it cannot be given a gain](#iphone-safaris-native-hls-cannot-be-routed-through-web-audio-so-it-cannot-be-given-a-gain) |
| Voice goes silent after getUserMedia. | [iOS suspends the AudioContext as the microphone opens](#ios-suspends-the-audiocontext-as-the-microphone-opens) |
| Subtitles drawn by JASSUB over the video vanished in fullscreen ("they stop showing when I switch to full screen mode"). | [iPhone video fullscreen is not the Fullscreen API, and it leaves page overlays behind](#iphone-video-fullscreen-is-not-the-fullscreen-api-and-it-leaves-page-overlays-behind) |
| Safari's native HLS starts on the first variant listed, whatever start hint is given | [Safari's native HLS starts on the first variant listed, whatever start hint is given](#safaris-native-hls-starts-on-the-first-variant-listed-whatever-start-hint-is-given) |
| The film page's hidden warm-up player loads nothing on iPhone. | [iPhone Safari loads nothing for a media element until the user has tapped it](#iphone-safari-loads-nothing-for-a-media-element-until-the-user-has-tapped-it) |
| A Watch Together guest's iPhone shows one still after another, with no sound. | [iPhone Safari refuses a film's sound that no recent tap started, and a tap long before does not count](#iphone-safari-refuses-a-films-sound-that-no-recent-tap-started-and-a-tap-long-before-does-not-count) |
| Native HLS gives the page no track choice, no quality control and no link measurement | [Native HLS gives the page no track choice, no quality control and no link measurement](#native-hls-gives-the-page-no-track-choice-no-quality-control-and-no-link-measurement) |
| Native-HLS `<video>` (and `<img>`) cannot send an Authorization header | [Native-HLS `<video>` (and `<img>`) cannot send an Authorization header](#native-hls--and--cannot-send-an-authorization-header) |
| IOS Safari with the microphone open sends call audio to the earpiece, quietly (WebKit 230902) | [iOS Safari with the microphone open sends call audio to the earpiece, quietly (WebKit 230902)](#ios-safari-with-the-microphone-open-sends-call-audio-to-the-earpiece-quietly-webkit-230902) |
| Safari supports zstd content-encoding only from 26 (iOS fully from 26.3) | [Safari supports zstd content-encoding only from 26 (iOS fully from 26.3)](#safari-supports-zstd-content-encoding-only-from-26-ios-fully-from-263) |
| Safari needs a copied HEVC picture tagged `hvc1` (FFmpeg's MP4 muxer writes `hev1` by default) | [Safari needs a copied HEVC picture tagged `hvc1` (FFmpeg's MP4 muxer writes `hev1` by default)](#safari-needs-a-copied-hevc-picture-tagged-hvc1-ffmpegs-mp4-muxer-writes-hev1-by-default) |
| The film became barely audible and the side buttons controlled the call volume instead. | [Android: echo cancellation puts the phone in a call, which turns the film down and takes the volume buttons](#android-echo-cancellation-puts-the-phone-in-a-call-which-turns-the-film-down-and-takes-the-volume-buttons) |
| The partner was too quiet even at 400% receiver gain. | [Chrome on Android with processing off captures quietly ("quiet, sometimes silent, streams")](#chrome-on-android-with-processing-off-captures-quietly-quiet-sometimes-silent-streams) |
| Remote voice routed only through Web Audio is silent. | [Chrome plays a remote WebRTC stream into Web Audio only while a media element also holds it](#chrome-plays-a-remote-webrtc-stream-into-web-audio-only-while-a-media-element-also-holds-it) |
| Brave on Android refused the microphone | [Brave on Android refused the microphone](#brave-on-android-refused-the-microphone) |
| A video with `autoplay` that never gets its first frame reports `paused === true` and `readyState` 1 indefinitely. | [An autoplaying video reads as paused until it can play, so a video that never can looks paused forever](#an-autoplaying-video-reads-as-paused-until-it-can-play-so-a-video-that-never-can-looks-paused-forever) |
| At `http://devnas.example:5173` every click threw before it sent anything, and sign-in "did nothing". | [Secure-context APIs are simply absent on plain HTTP off localhost](#secure-context-apis-are-simply-absent-on-plain-http-off-localhost) |
| A browser plays only the original file's first audio track | [A browser plays only the original file's first audio track](#a-browser-plays-only-the-original-files-first-audio-track) |
| Browsers other than Chrome play no Matroska, and none plays TrueHD, DTS or most AC-3 | [Browsers other than Chrome play no Matroska, and none plays TrueHD, DTS or most AC-3](#browsers-other-than-chrome-play-no-matroska-and-none-plays-truehd-dts-or-most-ac-3) |
| A browser `WebSocket` cannot send an Authorization header | [A browser `WebSocket` cannot send an Authorization header](#a-browser-websocket-cannot-send-an-authorization-header) |
| Browsers cap a cookie's Max-Age at 400 days | [Browsers cap a cookie's Max-Age at 400 days](#browsers-cap-a-cookies-max-age-at-400-days) |
| An iPhone held 0.26 s of a film's first chunk of its own picture and stopped at the hole. | [iPhone Safari keeps a sliver of a well-formed copied chunk](#iphone-safari-keeps-a-sliver-of-a-well-formed-copied-chunk) |
| A hidden preload player, or one waiting for its first frame, "waited out a whole chunk" on a slow link. | [The abandon rule is skipped while the video is paused](#the-abandon-rule-is-skipped-while-the-video-is-paused) |
| It never aborts a slow chunk when stepping down to level 0 | [It never aborts a slow chunk when stepping down to level 0](#it-never-aborts-a-slow-chunk-when-stepping-down-to-level-0) |
| After an abandon, it resets the estimator to a nominal bitrate | [After an abandon, it resets the estimator to a nominal bitrate](#after-an-abandon-it-resets-the-estimator-to-a-nominal-bitrate) |
| #5826 reads a throttled 1 Mb/s link as 404 MB/s. | [There is no cache-hit filter: a chunk from the browser's cache reads as link speed (hls.js#5826, open)](#there-is-no-cache-hit-filter-a-chunk-from-the-browsers-cache-reads-as-link-speed-hlsjs5826-open) |
| The `abrController` config type is nominal, so a custom controller must subclass | [The `abrController` config type is nominal, so a custom controller must subclass](#the-abrcontroller-config-type-is-nominal-so-a-custom-controller-must-subclass) |
| The base constructor calls the virtual `registerListeners()` before the subclass's fields exist | [The base constructor calls the virtual `registerListeners()` before the subclass's fields exist](#the-base-constructor-calls-the-virtual-registerlisteners-before-the-subclasss-fields-exist) |
| `EwmaBandWidthEstimator` is exported as a type only | [`EwmaBandWidthEstimator` is exported as a type only](#ewmabandwidthestimator-is-exported-as-a-type-only) |
| The docs understate the ABR controller's interface, and the setter echoes its own value back | [The docs understate the ABR controller's interface, and the setter echoes its own value back](#the-docs-understate-the-abr-controllers-interface-and-the-setter-echoes-its-own-value-back) |
| A 425 is never retried, and loads give up after about 10 s by default | [A 425 is never retried, and loads give up after about 10 s by default](#a-425-is-never-retried-and-loads-give-up-after-about-10-s-by-default) |
| A held request is counted as download time, so the estimate collapses | [A held request is counted as download time, so the estimate collapses](#a-held-request-is-counted-as-download-time-so-the-estimate-collapses) |
| "a film whose prepare had made 46 s began at 40 s". | [An open (EVENT, no ENDLIST) playlist is treated as live and starts near its end](#an-open-event-no-endlist-playlist-is-treated-as-live-and-starts-near-its-end) |
| A Watch Together host broadcast hls.js's own gap steps as user seeks ("each such step became a jump for everyone"). | [hls.js moves the playhead itself (gap jumps, stall nudges), and the browser reports each as a seek](#hlsjs-moves-the-playhead-itself-gap-jumps-stall-nudges-and-the-browser-reports-each-as-a-seek) |
| "Both players held at 0 with 0.04 s to 60 s loaded … held the group up for good." | [A paused player is never stepped over a gap at the start, so it never becomes ready](#a-paused-player-is-never-stepped-over-a-gap-at-the-start-so-it-never-becomes-ready) |
| After an audio-track switch, the audio already decoded in the old mix plays on for 2-4 s | [After an audio-track switch, the audio already decoded in the old mix plays on for 2-4 s](#after-an-audio-track-switch-the-audio-already-decoded-in-the-old-mix-plays-on-for-2-4-s) |
| A phone reported its link as 500, then 5000 kb/s (hls.js's starting values), and the core remembered that. | [Before any measurement, `bandwidthEstimate` returns its configured default, with no "unknown"](#before-any-measurement-bandwidthestimate-returns-its-configured-default-with-no-unknown) |
| Chrome and Safari use RED only when it is listed first | [Chrome and Safari use RED only when it is listed first](#chrome-and-safari-use-red-only-when-it-is-listed-first) |
| Voice arrived about 1.1 s late under the emulated congested link, about 0.9 s of it in NetEQ. | [No browser API caps the jitter buffer: `jitterBufferTarget` is a floor, not a ceiling](#no-browser-api-caps-the-jitter-buffer-jitterbuffertarget-is-a-floor-not-a-ceiling) |
| `networkPriority` (DSCP) exists in Chrome and Safari but not Firefox, and `setParameters` may refuse it | [`networkPriority` (DSCP) exists in Chrome and Safari but not Firefox, and `setParameters` may refuse it](#networkpriority-dscp-exists-in-chrome-and-safari-but-not-firefox-and-setparameters-may-refuse-it) |
| Two pages in the test container cannot call each other. | [mDNS-obfuscated host candidates cannot be resolved inside a container](#mdns-obfuscated-host-candidates-cannot-be-resolved-inside-a-container) |
| A phone on Wi-Fi cannot connect over WebRTC; the discovery logs "… 11 candidates; at most 10". | [A phone on Wi-Fi gathers more than ten ICE candidates](#a-phone-on-wi-fi-gathers-more-than-ten-ice-candidates) |
| Every offer refused: "gather the core's candidates: context deadline exceeded". | [pion waits 5 s for each STUN server while gathering, and a family with no way out never answers](#pion-waits-5-s-for-each-stun-server-while-gathering-and-a-family-with-no-way-out-never-answers) |
| X265 makes every keyframe after the first a CRA (open GOP) | [x265 makes every keyframe after the first a CRA (open GOP)](#x265-makes-every-keyframe-after-the-first-a-cra-open-gop) |
| The `segment` muxer misses a keyframe a hair before the cut time (10.009999 against 10.010) | [The `segment` muxer misses a keyframe a hair before the cut time (10.009999 against 10.010)](#the-segment-muxer-misses-a-keyframe-a-hair-before-the-cut-time-10009999-against-10010) |
| A copied picture's chunks each held the chunk before, retimed: the film jumped back, and chunks overlapped. | [An input seek at a keyframe of a Matroska film lands on the keyframe before it](#an-input-seek-at-a-keyframe-of-a-matroska-film-lands-on-the-keyframe-before-it) |
| A chunk held a fraction of its frames, with a gap after it. | [The segment muxer lists the file it was writing as finished when FFmpeg is stopped](#the-segment-muxer-lists-the-file-it-was-writing-as-finished-when-ffmpeg-is-stopped) |
| The `hls` muxer cuts by cumulative time and cannot keep a copied picture to a keyframe grid | [The `hls` muxer cuts by cumulative time and cannot keep a copied picture to a keyframe grid](#the-hls-muxer-cuts-by-cumulative-time-and-cannot-keep-a-copied-picture-to-a-keyframe-grid) |
| Each output file starts its fMP4 times at zero, and upstream has no flag for absolute `tfdt` | [Each output file starts its fMP4 times at zero, and upstream has no flag for absolute `tfdt`](#each-output-file-starts-its-fmp4-times-at-zero-and-upstream-has-no-flag-for-absolute-tfdt) |
| FMP4 init segments differ between runs unless the edit list is off | [fMP4 init segments differ between runs unless the edit list is off](#fmp4-init-segments-differ-between-runs-unless-the-edit-list-is-off) |
| An 8.7 ms seam where two runs meet. | [The AAC encoder adds 1024 priming samples at each run's start](#the-aac-encoder-adds-1024-priming-samples-at-each-runs-start) |
| The sound hiccuped every five minutes, where one run of chunks met the next. | [An input seek of an Opus track to a keyframe decodes it without its pre-roll](#an-input-seek-of-an-opus-track-to-a-keyframe-decodes-it-without-its-pre-roll) |
| Forced keyframes: x264 needs `-sc_threshold 0`, and hardware encoders need "forced IDR", spelt differently per encoder | [Forced keyframes: x264 needs `-sc_threshold 0`, and hardware encoders need "forced IDR", spelt differently per encoder](#forced-keyframes-x264-needs--sc_threshold-0-and-hardware-encoders-need-forced-idr-spelt-differently-per-encoder) |
| The first segment is as long as the source's first GOP (*Flow*: 10.43 s with `-hls_init_time 1`). | [`-hls_init_time` does nothing for copied video](#-hls_init_time-does-nothing-for-copied-video) |
| `[af#0:0] Task finished with error: Invalid argument` / `[out#0/chromaprint] Nothing was written into output file`. | [Chromaprint refuses a 5.1 input](#chromaprint-refuses-a-51-input) |
| FFmpeg's AC-3/E-AC-3 decoder applies DRC by default (`drc_scale = 1`) | [FFmpeg's AC-3/E-AC-3 decoder applies DRC by default (`drc_scale = 1`)](#ffmpegs-ac-3e-ac-3-decoder-applies-drc-by-default-drc_scale--1) |
| The planned head+tail sampling hash broke on every tag edit. | [`-c copy` remux rewrites every Matroska cluster, and mkvpropedit moves grown elements to the end](#-c-copy-remux-rewrites-every-matroska-cluster-and-mkvpropedit-moves-grown-elements-to-the-end) |
| FFmpeg reads an input name as a protocol, and reads stdin by default | [FFmpeg reads an input name as a protocol, and reads stdin by default](#ffmpeg-reads-an-input-name-as-a-protocol-and-reads-stdin-by-default) |
| Ffprobe's output quirks | [ffprobe's output quirks](#ffprobes-output-quirks) |
| A Matroska file's cues may list fewer keyframes than the picture has | [A Matroska file's cues may list fewer keyframes than the picture has](#a-matroska-files-cues-may-list-fewer-keyframes-than-the-picture-has) |
| `delay 80ms 30ms 25%` reordered 47.4% of a 500 pkt/s stream and 22.6% of 50 pkt/s voice. | [netem's jitter reorders packets unless netem also has a `rate`; correlation does not prevent it](#netems-jitter-reorders-packets-unless-netem-also-has-a-rate-correlation-does-not-prevent-it) |
| `limit 100`, meant as about a quarter-second queue, gave CUBIC a 472 ms smoothed RTT with no loss. | [netem's `limit` counts every packet it holds, including packets only being delayed (and on `lo`, both directions)](#netems-limit-counts-every-packet-it-holds-including-packets-only-being-delayed-and-on-lo-both-directions) |
| "a TCP packet of the film held a voice packet behind it for a tenth of a second" at 5 Mbit/s. | [The loopback's 64 KB MTU and GSO make netem and tbf see 64 KB packets](#the-loopbacks-64-kb-mtu-and-gso-make-netem-and-tbf-see-64-kb-packets) |
| 5 Mbit/s is about 2.5 Mbit/s for each viewer, an RTT counts the queue twice, and TURN-relayed voice crosses the link twice. | [One qdisc on `lo` is one shared link for both directions and both browsers](#one-qdisc-on-lo-is-one-shared-link-for-both-directions-and-both-browsers) |
| `tbf`'s `burst` must be at least rate/HZ, or the rate is never reached | [`tbf`'s `burst` must be at least rate/HZ, or the rate is never reached](#tbfs-burst-must-be-at-least-ratehz-or-the-rate-is-never-reached) |
| `setsockopt(TCP_CONGESTION, "bbr")` returns ENOENT as a normal user, while root autoloads the module. | [Mainline Linux has only BBRv1, and `tcp_bbr` must already be loaded for an unprivileged process](#mainline-linux-has-only-bbrv1-and-tcp_bbr-must-already-be-loaded-for-an-unprivileged-process) |
| Iproute2's `ip` drops file capabilities, so `setcap` on `ip` is useless (`tc` keeps them) | [iproute2's `ip` drops file capabilities, so `setcap` on `ip` is useless (`tc` keeps them)](#iproute2s-ip-drops-file-capabilities-so-setcap-on-ip-is-useless-tc-keeps-them) |
| Netem orphans delayed packets (`skb_orphan_partial`), so TCP Small Queues does not hold the sender back | [netem orphans delayed packets (`skb_orphan_partial`), so TCP Small Queues does not hold the sender back](#netem-orphans-delayed-packets-skb_orphan_partial-so-tcp-small-queues-does-not-hold-the-sender-back) |
| A process in uninterruptible disk wait ignores even SIGKILL | [A process in uninterruptible disk wait ignores even SIGKILL](#a-process-in-uninterruptible-disk-wait-ignores-even-sigkill) |
| Pdeathsig fires when the starting *thread* ends, not the process | [Pdeathsig fires when the starting *thread* ends, not the process](#pdeathsig-fires-when-the-starting-thread-ends-not-the-process) |
| A pipe placed where a media file was held a byte request and its goroutine forever. | [Opening a named pipe read-only blocks until a writer appears](#opening-a-named-pipe-read-only-blocks-until-a-writer-appears) |
| `landlock_create_ruleset(...) → -1, errno 95` in an ordinary container. | [Whether Landlock is available depends on the kernel's boot configuration, and the errno says why](#whether-landlock-is-available-depends-on-the-kernels-boot-configuration-and-the-errno-says-why) |
| X32 syscalls share AUDIT_ARCH_X86_64, so a seccomp denylist can be bypassed | [x32 syscalls share AUDIT_ARCH_X86_64, so a seccomp denylist can be bypassed](#x32-syscalls-share-audit_arch_x86_64-so-a-seccomp-denylist-can-be-bypassed) |
| The owner's USB SSD on devnas read as spinning. | [The kernel's `rotational` flag is wrong behind a USB bridge](#the-kernels-rotational-flag-is-wrong-behind-a-usb-bridge) |
| Btrfs and ZFS mounts do not name a block device | [btrfs and ZFS mounts do not name a block device](#btrfs-and-zfs-mounts-do-not-name-a-block-device) |
| A UDP socket bound to every address replies from the routing table's choice of source | [A UDP socket bound to every address replies from the routing table's choice of source](#a-udp-socket-bound-to-every-address-replies-from-the-routing-tables-choice-of-source) |
| Unix socket paths are limited to 108 bytes on Linux and 104 on macOS | [Unix socket paths are limited to 108 bytes on Linux and 104 on macOS](#unix-socket-paths-are-limited-to-108-bytes-on-linux-and-104-on-macos) |
| Inotify does not work over SMB/NFS and runs out of watches silently | [inotify does not work over SMB/NFS and runs out of watches silently](#inotify-does-not-work-over-smbnfs-and-runs-out-of-watches-silently) |
| The NFS client's default timeout is 60 s, retransmitted twice | [The NFS client's default timeout is 60 s, retransmitted twice](#the-nfs-clients-default-timeout-is-60-s-retransmitted-twice) |
| `./staging status` shows the core unhealthy after the tunnel's container restarted on its own. | [A container sharing another's network loses it when that container restarts alone](#a-container-sharing-anothers-network-loses-it-when-that-container-restarts-alone) |
| A second `./dev make dev` ran two dev loops that fought over ports. | [Killing the `docker exec` client leaves the process running inside the container](#killing-the-docker-exec-client-leaves-the-process-running-inside-the-container) |
| The dev container had no `tc` and no right to shape its network, and CI's container lacks `NET_ADMIN` | [The dev container had no `tc` and no right to shape its network, and CI's container lacks `NET_ADMIN`](#the-dev-container-had-no-tc-and-no-right-to-shape-its-network-and-cis-container-lacks-net_admin) |
| `permission denied` from PostUp. | [Ubuntu's AppArmor profile for `wg-quick` refuses PostUp's sysctl and nftables, even as root](#ubuntus-apparmor-profile-for-wg-quick-refuses-postups-sysctl-and-nftables-even-as-root) |
| Ufw silently drops forwarded packets | [ufw silently drops forwarded packets](#ufw-silently-drops-forwarded-packets) |
| The disk reached 97-99% several times; | [Docker's storage is invisible to `du` and grows without bound](#dockers-storage-is-invisible-to-du-and-grows-without-bound) |
| "Large pages or videos stall from outside, small ones load." | [DS-Lite needs a smaller WireGuard MTU](#ds-lite-needs-a-smaller-wireguard-mtu) |
| DS-Lite: no public IPv4 and a changing IPv6 prefix | [DS-Lite: no public IPv4 and a changing IPv6 prefix](#ds-lite-no-public-ipv4-and-a-changing-ipv6-prefix) |
| `edited query syntax is invalid: extraneous input ')'` when a comment holds `§`. | [sqlc miscounts a multibyte character in a query file and reports an unrelated syntax error](#sqlc-miscounts-a-multibyte-character-in-a-query-file-and-reports-an-unrelated-syntax-error) |
| `sqlc.slice()` and untyped SQLite columns generate `interface{}` | [`sqlc.slice()` and untyped SQLite columns generate `interface{}`](#sqlcslice-and-untyped-sqlite-columns-generate-interface) |
| Sqlc cannot parse FTS5 `MATCH`, `LIKE … ESCAPE` or SQLite 3.53's new `ALTER` forms | [sqlc cannot parse FTS5 `MATCH`, `LIKE … ESCAPE` or SQLite 3.53's new `ALTER` forms](#sqlc-cannot-parse-fts5-match-like--escape-or-sqlite-353s-new-alter-forms) |
| `sqlc vet` sees a query after `*` is expanded, so a "no SELECT *" rule can never fire | [`sqlc vet` sees a query after `*` is expanded, so a "no SELECT *" rule can never fire](#sqlc-vet-sees-a-query-after--is-expanded-so-a-no-select--rule-can-never-fire) |
| A pragma set by `Exec` reaches only one connection of the pool, and the pure-Go driver's DSN syntax differs from mattn's | [A pragma set by `Exec` reaches only one connection of the pool, and the pure-Go driver's DSN syntax differs from mattn's](#a-pragma-set-by-exec-reaches-only-one-connection-of-the-pool-and-the-pure-go-drivers-dsn-syntax-differs-from-mattns) |
| `SQLITE_BUSY_SNAPSHOT` shares "database is locked" with `SQLITE_BUSY`, but `busy_timeout` retries only the latter | [`SQLITE_BUSY_SNAPSHOT` shares "database is locked" with `SQLITE_BUSY`, but `busy_timeout` retries only the latter](#sqlite_busy_snapshot-shares-database-is-locked-with-sqlite_busy-but-busy_timeout-retries-only-the-latter) |
| `no such table`. | [Copying a live WAL database's `.db` file gives a file that opens cleanly and has no tables](#copying-a-live-wal-databases-db-file-gives-a-file-that-opens-cleanly-and-has-no-tables) |
| SQLite's limits: 1000 OR terms ("Expression tree is too large") and 64 tables in a join | [SQLite's limits: 1000 OR terms ("Expression tree is too large") and 64 tables in a join](#sqlites-limits-1000-or-terms-expression-tree-is-too-large-and-64-tables-in-a-join) |
| Unicode61 makes a CJK run one token and shreds Thai. | [FTS5 tokeniser gaps](#fts5-tokeniser-gaps) |
| `modernc.org/libc` imports `os/exec`, which trips "the core never execs" | [`modernc.org/libc` imports `os/exec`, which trips "the core never execs"](#moderncorglibc-imports-osexec-which-trips-the-core-never-execs) |
| SQLite's locking is unreliable on network filesystems | [SQLite's locking is unreliable on network filesystems](#sqlites-locking-is-unreliable-on-network-filesystems) |
| A backup with a row its CHECK forbids passed `integrity_check` | [On a file opened `mode=ro`, `integrity_check` skips CHECK constraints](#on-a-file-opened-modero-integrity_check-skips-check-constraints) |
| `COLLATE NOCASE` folds only ASCII, and temp files follow TMPDIR | [`COLLATE NOCASE` folds only ASCII, and temp files follow TMPDIR](#collate-nocase-folds-only-ascii-and-temp-files-follow-tmpdir) |
| Go regexp's `\w`, `\d`, `\s` and `\b` are ASCII-only and fail silently on names like "Amélie" | [Go regexp's `\w`, `\d`, `\s` and `\b` are ASCII-only and fail silently on names like "Amélie"](#go-regexps-w-d-s-and-b-are-ascii-only-and-fail-silently-on-names-like-amélie) |
| `x/text/collate` is stuck at Unicode 6.2 and sorts Korean by code point (golang/go#19087) | [`x/text/collate` is stuck at Unicode 6.2 and sorts Korean by code point (golang/go#19087)](#xtextcollate-is-stuck-at-unicode-62-and-sorts-korean-by-code-point-golanggo19087) |
| govet's shadow check suddenly flags every `fixture := newFixture(t)` in a package's tests. | [govet's shadow check flags every `x := …` once a later file mentions a type named `x`](#govets-shadow-check-flags-every-x---once-a-later-file-mentions-a-type-named-x) |
| "parallel golangci-lint is running". | [golangci-lint refuses to start beside another run (exit 3) and prints nothing on stdout](#golangci-lint-refuses-to-start-beside-another-run-exit-3-and-prints-nothing-on-stdout) |
| A copy of git's index takes a file edited to the same size in the same second for unchanged | [A copy of git's index with a new time hides a file changed in the second the index was written](#a-copy-of-gits-index-with-a-new-time-hides-a-file-changed-in-the-second-the-index-was-written) |
| The default HTTP transport reads HTTP(S)_PROXY, and Go downloads another toolchain unless `GOTOOLCHAIN=local` | [The default HTTP transport reads HTTP(S)_PROXY, and Go downloads another toolchain unless `GOTOOLCHAIN=local`](#the-default-http-transport-reads-https_proxy-and-go-downloads-another-toolchain-unless-gotoolchainlocal) |
| Go's standard library reads the process's TZ | [Go's standard library reads the process's TZ](#gos-standard-library-reads-the-processs-tz) |
| Vitest strips types without type-checking, so a types-only change can never fail a test | [Vitest strips types without type-checking, so a types-only change can never fail a test](#vitest-strips-types-without-type-checking-so-a-types-only-change-can-never-fail-a-test) |
| Happy-dom does not carry a disabled `<fieldset>` to its controls | [happy-dom does not carry a disabled `<fieldset>` to its controls](#happy-dom-does-not-carry-a-disabled--to-its-controls) |
| An iPhone pauses a video as its own full-screen player closes | [An iPhone pauses a video as its own full-screen player closes](#an-iphone-pauses-a-video-as-its-own-full-screen-player-closes) |
| An iPhone in a group paused everyone as its screen locked | [iOS pauses a playing video as its screen locks, before the page is told it is hidden](#ios-pauses-a-playing-video-as-its-screen-locks-before-the-page-is-told-it-is-hidden) |
| Safari sizes a `height: 100%` video in an `aspect-ratio` box wrongly, cutting off its controls | [Safari sizes a `height: 100%` video in an `aspect-ratio` box wrongly, cutting off its controls](#safari-sizes-a-height-100-video-in-an-aspect-ratio-box-wrongly-cutting-off-its-controls) |
| Playwright grants "microphone" only on Chromium; | [Playwright grants "microphone" only on Chromium; Firefox needs prefs](#playwright-grants-microphone-only-on-chromium-firefox-needs-prefs) |
| After the D227 renames the client compiled while still sending the old field names. | [TypeScript checks no excess properties on a body built with a computed key or held in a variable](#typescript-checks-no-excess-properties-on-a-body-built-with-a-computed-key-or-held-in-a-variable) |
| Playwright's browsers are not installed by `npm ci` | [Playwright's browsers are not installed by `npm ci`](#playwrights-browsers-are-not-installed-by-npm-ci) |
| Git hooks export GIT_DIR, so a test's `git init` set `core.bare` on the project repository | [Git hooks export GIT_DIR, so a test's `git init` set `core.bare` on the project repository](#git-hooks-export-git_dir-so-a-tests-git-init-set-corebare-on-the-project-repository) |
| `npm ci` through a symlinked `node_modules` empties the link's target | [`npm ci` through a symlinked `node_modules` empties the link's target](#npm-ci-through-a-symlinked-node_modules-empties-the-links-target) |
| Two npm installs at once race | [Two npm installs at once race](#two-npm-installs-at-once-race) |
| A white screen in every browser. | [Vite's dev server misses a `git stash pop` and keeps serving old modules](#vites-dev-server-misses-a-git-stash-pop-and-keeps-serving-old-modules) |
| Python's bytecode cache is keyed on size and an mtime with one-second resolution | [Python's bytecode cache is keyed on size and an mtime with one-second resolution](#pythons-bytecode-cache-is-keyed-on-size-and-an-mtime-with-one-second-resolution) |
| A wait loop waited half an hour on its own command line, and a kill killed the calling shell. | [`pgrep -f` and `pkill -f` match the shell that runs them](#pgrep--f-and-pkill--f-match-the-shell-that-runs-them) |
| A trailing-slash `.gitignore` pattern (`node_modules/`) does not match a symlink | [A trailing-slash `.gitignore` pattern (`node_modules/`) does not match a symlink](#a-trailing-slash-gitignore-pattern-node_modules-does-not-match-a-symlink) |
| "Patch is empty". | [`yoloai apply` replays with `git am`, which stops on an empty patch](#yoloai-apply-replays-with-git-am-which-stops-on-an-empty-patch) |
| A Cloudflare-proxied record ends TLS, carries no UDP/TURN, and the terms restrict video | [A Cloudflare-proxied record ends TLS, carries no UDP/TURN, and the terms restrict video](#a-cloudflare-proxied-record-ends-tls-carries-no-udpturn-and-the-terms-restrict-video) |
| There is no publicly trusted certificate for a bare IP, and private-IP DNS answers trip rebinding protection | [There is no publicly trusted certificate for a bare IP, and private-IP DNS answers trip rebinding protection](#there-is-no-publicly-trusted-certificate-for-a-bare-ip-and-private-ip-dns-answers-trip-rebinding-protection) |
| A machine has a public IPv6 address but no IPv6 default route, and IPv6 fails with "network is unreachable". | [A host that forwards IPv6 ignores the router's announcements, and its default route, unless `accept_ra` is 2](#a-host-that-forwards-ipv6-ignores-the-routers-announcements-and-its-default-route-unless-accept_ra-is-2) |
| Inside a Proxmox container there is no IPv6 but Docker's own. | [A Proxmox container has no IPv6 until its `net0` is given it](#a-proxmox-container-has-no-ipv6-until-its-net0-is-given-it) |
| After an interface was taken down and up in a container, IPv4 had no route. | [Taking an LXC container's interface down and up loses its IPv4 default route](#taking-an-lxc-containers-interface-down-and-up-loses-its-ipv4-default-route) |
| Pulling the image fails with "network is unreachable" though IPv6 works. | [GitHub's image registry, `ghcr.io`, has no IPv6 address](#githubs-image-registry-ghcrio-has-no-ipv6-address) |
| A phone on a travel eSIM reaches no server: one IPv4 candidate abroad, no IPv6. | [A travel eSIM's data leaves through its operator's gateway, over IPv4 alone](#a-travel-esims-data-leaves-through-its-operators-gateway-over-ipv4-alone) |

---

## Firefox


### Firefox fails a public page's requests to a LAN name until its local-network prompt is answered, and again after a grant not remembered

- **Symptom:** at app.orpheum-app.com at home, the page signed in by realm and then sat there:
  every request failed with `TypeError: NetworkError when attempting to fetch resource.`, reported
  dozens of times in the core's log as "a page reported a failure"; another time the page signed in
  but chose the public IPv6 name, and showed no admin. A private window behaved differently from a
  normal one.
- **Cause:** Firefox's Local Network Access (on for everyone from Firefox 151; first behind Strict
  tracking protection in 149) asks before a public origin's page reaches a private address, as the
  core's LAN name resolves to. Until the prompt is answered the request fails at once, rather than
  waiting; a grant without "Remember" lasts only for a while, after which requests fail again; and
  one the prompt answered "Block" with is kept per site, so a profile that once blocked never asks
  again. The page had chosen the LAN name while it answered, and kept it.
- **What we do:** the page keeps its requests on a path that works (`web/packages/sdk/src/path-keeper.ts`):
  a request that fails for the network's sake moves the page to another path, the next of the core's
  names that answers, else WebRTC, and is sent again; every 30 s, and when the browser comes back
  online, the page asks whether a better path answers now, so the LAN is moved to once the prompt is
  allowed (webrtc-transport.md W2c items 5 and 6). Asking the permission's state first (W2c item 1)
  and the line explaining the prompt (item 8) are still to come.
- **And with a service worker:** even granted ("prompt action: auto_allow" in the console), every
  request a page made to the LAN name failed once the page's service worker controlled it: the
  health check and the sign-in, made before, went through; the core received each request after
  and Firefox closed the connection (the core logging `http2: received GOAWAY`), the console saying
  "CORS request did not succeed", and the page, on the public name, was offered no admin. Setting
  `network.lna.blocking` to false made it work at once; Firefox misjudges a load a worker lets pass
  ([Mozilla support 1594177](https://support.mozilla.org/de/questions/1594177)). The worker
  fetching the load itself fails the same way, and broke the page's other requests besides; it was
  withdrawn. A page in the house reaches its core over WebRTC instead, which the check does not
  touch, and the core counts a direct pair from the house as home (D300).
- **Only a burst, near the start:** with WebRTC at home in place (staging, 2026-10-15), the failures
  came once, as a page began, then stopped: the page moved to WebRTC (a direct pair in the house),
  its 30 s look for a better path found the LAN name answering, and every request after, a film's
  chunks too, went there without one failing. Unconfirmed: the connection opened before the worker
  controlled the page is the one that fails, and a new one does not. The film did not stall.
- **Not only a burst:** on nas.example (2026-10-18), a host's Firefox on macOS moved from the LAN name to WebRTC
  and back 171 times in a 53-minute Watch Together session, 3 to 5 times a minute from start to end: the
  look for a better path found the LAN name answering each time, and its next request failed. The film
  did not stall, but a request failed at each move. The look for a better path now waits twice as long
  after each move away from a failed path, from 30 s up to 10 minutes, and as at first once the browser
  is back online (`betterPathFirstMs` in `web/apps/web/src/kept-path.ts`).
- **Evidence:** nas.example's and staging's logs, 2026-10-15, Firefox 157 on macOS; [Mozilla bug
  2059274](https://bugzilla.mozilla.org/show_bug.cgi?id=2059274) on when Firefox asks;
  undoing a block: the site's permissions in the address bar, or `about:preferences#privacy`'s
  permission settings.

### Firefox on macOS freezes on HEVC chunks that start on a CRA keyframe

- **Technology:** Firefox 154 and later on macOS (156 seen), MSE with HEVC, and x265's open-GOP keyframes.
- **Symptom:** *Flow* (10-bit HEVC by x265) froze at any start but the film's beginning: the player spun,
  paused at readyState 1, while its chunks kept arriving on a fast link. Safari played it everywhere, and so
  did Firefox from 0:00. Nothing says so in advance: `isTypeSupported` says HEVC is playable, and it is,
  from an IDR.
- **Cause:** x265's keyframes after the first are CRA, not IDR, so every chunk but the first starts on a CRA.
  Since Firefox 154, on macOS only, Firefox's MP4 demuxer demotes a CRA to a non-keyframe (a workaround for
  a VideoToolbox decode failure, Mozilla bug 2049615). File playback falls back to an earlier IDR. An MSE
  append has none to fall back to, so the coded frame group never opens and every sample is dropped
  silently: appends succeed, `readyState` stops at HAVE_METADATA, and `buffered` stays empty. Captured at the
  moment of giving up, the picture's buffer was empty, the sound's held 416.24–501.78, and hls.js reported
  `bufferAppendNoProgress: Fragment append did not increase buffered coverage`.
- **Known upstream:** [Mozilla bug 2065822](https://bugzilla.mozilla.org/show_bug.cgi?id=2065822), "MSE
  silently drops an HEVC coded frame group that starts on a CRA picture (macOS, Gecko >=154)", NEW. A patch
  (treat every IRAP as a random access point, skip undecodable RASL pictures in AppleVTDecoder) was attached on
  2026-08-27 and has not landed. The reporter found **no container-level workaround**: `tfhd`/`trun` flags,
  split init segments, SourceBuffer modes, and relabelling the CRA as IDR_N_LP or BLA_N_LP all failed. Their
  shipped workaround re-encodes to H.264 in the browser. Re-check this entry when Firefox updates; once the fix
  ships, the original picture plays mid-film again and D266 stops firing for it on its own.
- **What we do:** D266. A quality is given up for the playback, with every quality above it, and the video's
  decoding is built anew (`recoverMediaError`). This happens at once when two of its chunks are held nothing
  of, or after 6 s standing still while two chunks arrive. The core logs the quality, with the video as it
  was. There is no User-Agent branching. The 6 s count only while what the video plays is loaded where it
  stands, a seek's sound coming late being no sign; the held-nothing count needs nothing loaded, since this
  picture is never loaded anywhere. Requiring it loaded for both left a Mac in a group waiting on Flow for
  good (the owner's test, 2026-10-12).
  `web/packages/player/src/abr-choice.ts` (`isUnplayable`, `chunksNotHeld`); `web/packages/player/src/abr.ts`
  (`#lookAtPlaying`, `notHeld`); `internal/core/api/report.go` (`logUnplayable`).
- **Evidence:** D266; [owner] 2026-10-07 (Safari plays it, Firefox from the start only) and 2026-10-08 (the
  empty picture buffer and `bufferAppendNoProgress` above, 05:16 UTC on staging); [web] Mozilla bugs 2065822
  and 2049615, read 2026-10-08.
  **[web]** re-checked 2026-10-08: [Mozilla bug 2065822](https://bugzilla.mozilla.org/show_bug.cgi?id=2065822)
  is still NEW (last change 2026-10-05; marked as regressed by 2049615, release status set so it counts as
  confirmed); [Mozilla bug 2049615](https://bugzilla.mozilla.org/show_bug.cgi?id=2049615), "Seeking in video
  fails", RESOLVED FIXED in 154, is the regressor. Related: [Mozilla bug
  2072130](https://bugzilla.mozilla.org/show_bug.cgi?id=2072130), "MP4+H264 keyframe workarounds likely cause
  more seeking issues than they solve", ASSIGNED, patches attached 2026-09-15/29 that argue for removing the
  MP4 demuxer's keyframe special-casing (H.264 only so far); [Mozilla bug
  2006226](https://bugzilla.mozilla.org/show_bug.cgi?id=2006226), HEVC in MKV freezing after a skip, FIXED in
  158 (file playback, not MSE; does not cover our case).
- **Caution:** agent-failures A22 (2026-10-06) records an earlier wrong diagnosis, "Firefox can't decode
  HEVC". Part of that freeze was our own gap bug. A stall report taken *after* a
  rebuild showed the sound missing instead: that was the new pipeline refilling, not the cause.
- **Firefox on Linux plays the same chunks:** headless Firefox 155 on Linux, through Playwright against
  staging on 2026-10-07/08, played Flow's own picture from mid-film chunks, picture and sound buffered alike:
  the demotion is under `MOZ_APPLEMEDIA`, macOS only.

### Firefox offers no RED (redundant audio) for Opus

- **Technology:** Firefox WebRTC.
- **Symptom:** `RTCRtpReceiver.getCapabilities("audio")` lists no `audio/red`, so plain Opus is negotiated and
  voice has less loss protection.
- **Cause:** Firefox's `DefaultCodecPreferences.cpp` has no audio RED [source].
- **What we do:** RED is preferred where it is listed. A pair with Firefox is held only to best-effort voice
  bounds on the *lossy* profile.
  `web/packages/sdk/src/together-voice-codec.ts:73-86`; `web/e2e/acceptance-server.ts:28`;
  `web/e2e/acceptance.spec.ts:264-276`.
- **Evidence:** D262 §3, D263 §2; `research/voice-bad-links.md` §3 "RED".
  **[web]** [Mozilla bug 1801367](https://bugzilla.mozilla.org/show_bug.cgi?id=1801367), "Support RFC 2198
  redundancy (RED) for audio": ASSIGNED (Philipp Hancke); a patch "implement audio/red for WebRTC" was
  attached 2026-06-18 and has not landed (checked 2026-10-08). The patch enables RED only when it comes before
  Opus in the codec order, as Chrome does, so our "prefer RED where listed" code should pick it up unchanged
  once it ships. Related: [Mozilla bug 1886010](https://bugzilla.mozilla.org/show_bug.cgi?id=1886010), DRED
  and LACE/NoLACE with Opus 1.5, NEW.


### Firefox keeps `AudioContext.resume()` pending until it lets the page play sound

- **Technology:** Firefox autoplay policy + Web Audio.
- **Symptom:** voice hung at "Asking for your microphone" because opening the microphone awaited the context's
  resume.
- **What we do:** `resume()` is fire-and-forget, never awaited.
  `web/packages/sdk/src/together-voice-browser.ts:227-247` (comment at `:236-238`).
- **Evidence:** [measured] browser tier (Playwright Firefox), 2026-10-06; fix 22fba2c0;
  `research/audit-2026-10-06.md` §1.
  **[web]** this is the specified behaviour, not a Firefox bug: the [Web Audio spec's
  `resume()`](https://webaudio.github.io/web-audio-api/#dom-audiocontext-resume) says "If the context is not
  allowed to start, append promise to [[pending promises]] and [[pending resume promises]] and abort these
  steps", so any browser that blocks autoplay may leave the promise pending until a gesture (read 2026-10-08).
  Unrelated open ordering bug: [Mozilla bug 2070740](https://bugzilla.mozilla.org/show_bug.cgi?id=2070740),
  suspend/resume/close promises resolving out of order, NEW. **Contradicts:** the heading frames this as
  Firefox-specific; the spec allows it in every browser, so the never-await rule applies everywhere.


### Firefox on Android makes a "call" of gain control and noise suppression, even with echo cancelling off

- **Technology:** Firefox for Android, getUserMedia.
- **Symptom:** with voice on, the film was turned down and the volume buttons stopped changing its volume.
  Chrome on Android does this only for echo cancelling (see 3.1).
- **Cause:** presumably the audio session switches to communication mode. Unverified for Firefox.
- **What we do:** `should_together_voice_browser_level_microphone` (`shouldBrowserLevelMicrophone`) is off by default.
  `web/packages/sdk/src/together-voice-browser.ts:13-18`; `web/packages/sdk/src/together.ts:109,121`;
  `internal/core/settings/registry.go:296-301`; `internal/core/together/messages.go:262-267`.
- **Evidence:** [owner] 2026-10-07, Firefox 157 on Android 16; VQ23.
  **[web]** [Mozilla bug 1711032](https://bugzilla.mozilla.org/show_bug.cgi?id=1711032), "Android headset
  microphone does not work when EchoCancellation is disabled" (NEW): its comments trace Firefox's input path
  to `passThrough = !(aec || agc || noise)` in `MediaEngineWebRTCAudio.cpp`, and cubeb opens a *voice* input
  only when `passThrough` is false (cubeb_opensl.c, 2021). That sharpens our unverified cause: any one of the
  three processing switches, not echo cancelling alone, selects the voice-communication input. [Mozilla bug
  1839885](https://bugzilla.mozilla.org/show_bug.cgi?id=1839885), "Add APIs for controlling platform/hw echo
  cancellation and other processing on Android", NEW. No report of the film being turned down found (searched
  bugzilla.mozilla.org, 2026-10-08).

---

## Safari / iOS / WebKit


### iPhone Safari moves a range slider only by a touch that starts on its thumb

- **Technology:** `<input type="range">` in Safari on iOS (seen on iOS 18.5, 2026-10-12).
- **Symptom:** the owner's Watch Together volume sliders "wouldn't move unless I hit it just so": a tap on
  the track does nothing, and a slide moves the slider only when the finger landed on its small thumb.
- **Cause:** WebKit's touch handling of a native range starts a drag only from the thumb; a touch elsewhere
  on the track is neither a jump nor a drag.
- **What we do:** every volume slider is one `VolumeRange`, which takes `touchstart` and `touchmove` itself
  (not passive), sets the value where the finger is along the slider and follows it, and is 44 px tall
  with `touch-action: none` so the page does not scroll under the finger. A mouse uses the native slider.
  `web/apps/web/src/volume-range.tsx`, `volume-range.css`. VQ31.
- **Evidence:** the owner's iPhone, 2026-10-12. **[web]** Others met it widely enough that Plyr's author
  wrote a library for it alone, [RangeTouch](https://github.com/sampotts/rangetouch) ("make
  `<input type='range'>` sliders work better on touch devices"), which does the same: it sets the value
  from the touch's position.


### Safari's `currentTime` stands still for up to about 250 ms after play, seek or a rate change

- **Technology:** WebKit media players (MSE, ManagedMediaSource and native HLS), Safari 18.4-26.x. On 18.3 and
  earlier the value runs ahead and then goes *backwards*.
- **Symptom:** the position reads as not advancing, which looks like false drift and causes spurious nudges.
  It is linked to the iPhone viewer's "still frame every few seconds, film jumping constantly" (2026-10-06).
  That link is inferred, not confirmed from the logs.
- **Cause:** [source] `MediaPlayerPrivateRemote` freezes its time estimate on `rateChanged` until the next
  GPU-process time update (0.1-0.25 s).
- **What we do:** a settling time (`settleAfterChangeMs`, default 300) in which no nudge is made.
  `web/packages/sdk/src/together.ts:96-100,118`; `web/packages/sdk/src/together-follow.ts:272,534-551`;
  `internal/core/together/messages.go:258-260`; `internal/core/settings/registry.go:278-279`.
- **Evidence:** D250 §7 (amended 2026-10-06); `research/safari-versions.md` §3-§4.
  **[web]** [WebKit bug 258945](https://bugs.webkit.org/show_bug.cgi?id=258945), "[HTMLMediaElement]
  currentTime shifts after updating playbackRate": NEW since 2023-07, no fix (checked 2026-10-08). It reports
  the time jumping forward or back after a rate change, the same family; no report of the 250 ms freeze itself
  was found.


### Safari's rate nudges stutter or skip, especially with pitch-keeping on

- **Technology:** WebKit `playbackRate` + `preservesPitch`.
- **Symptom:** each rate change may make the sound skip. Rate and seek land 100-500 ms off, against 2 frames or
  less in Chrome and Firefox (hls.js #7616, shaka #2823). Jellyfin turned SpeedToSync off ("failing on Safari").
- **Cause:** inferred: WebKit swaps AVPlayerItem's pitch algorithm when the rate is not 1 [source].
- **What we do:** the `should_together_nudge_keep_pitch` setting, small and infrequent nudges, and seek-only as
  the fallback. `internal/core/settings/registry.go:281-284`. Each player also learns what its rate going back
  to the group's costs it, and a nudge goes on past the group by that much, so it lands on it: on two iPhones
  each nudge's end cost about 220 ms, beyond the 150 ms allowance, and they were nudged some 40 times in two
  minutes, their sound stuttering at each (the owner's tests, 2026-10-08; `learnedLossMs` and
  `driftCorrection` in `web/packages/sdk/src/together-drift.ts`; each nudge's note logs `returnCostMs`).
  Once two returns have cost it more than the drift left alone, the player stops nudging and corrects
  by seeks alone, beyond `together_seek_only_drift_left_alone_ms` (400 ms), each seek aimed past the
  group by what its seeks have learned to lose, as MediaSync does on Safari (D268 §2;
  `seekOnlyCorrection`; the log's `a player corrects by seeks alone`). VQ27 checks it on an iPhone.
- **Evidence:** D250 §7, D62; VQ22 (open); `research/safari-versions.md` §3, §5; [web].
  **[web]** [WebKit bug 163433](https://bugs.webkit.org/show_bug.cgi?id=163433), "playbackRate update while
  playing creates a small pause in the playback" (about 80 ms on iOS, 300 ms on macOS): NEW since 2016;
  [WebKit bug 225868](https://bugs.webkit.org/show_bug.cgi?id=225868), "REGRESSION: Distorted audio when
  altering playback-rate on HTMLVideoElement" (since Safari 14.1): NEW;
  [hls.js#7616](https://github.com/video-dev/hls.js/issues/7616) closed 2026-05 with the maintainer's advice
  to send Safari bugs to Apple and to change rate or seek only on muted elements, one element playing sound at
  1x; [shaka-player#2823](https://github.com/shaka-project/shaka-player/issues/2823) closed as WebKit 163433
  (all checked 2026-10-08).


### iPhone has no `MediaSource`, only `ManagedMediaSource`, and only from iOS 17.1

- **Technology:** iOS WebKit (every iPhone browser). iPad has classic MSE.
- **Symptom:** `window.MediaSource` is undefined on iPhone, so a probe that uses only MSE misses ManagedMediaSource,
  and hls.js cannot run below 17.1.
- **What we do:** feature detection only. `isTypeSupported` is used only where `MediaSource` exists, otherwise
  `canPlayType`. Watch Together's floor is iOS 17.1 (the page tells older devices to update), and solo
  playback falls back to native HLS.
  `web/packages/sdk/src/playable-formats.ts:64-73`; `web/packages/player/src/hls.ts:38-45`; the `cannotFollow`
  and `cannotHost` strings in `web/apps/web/src/locales/en.ts`.
- **Evidence:** D250 §7; `research/safari-versions.md` §2; [owner] 2026-10-06 (the iPhone at the airport). This
  is a limitation, not a bug.
  **[web]** [MDN
  browser-compat-data](https://github.com/mdn/browser-compat-data/blob/main/api/ManagedMediaSource.json):
  ManagedMediaSource from Safari 17 and iOS 17.1, `MediaSource` "exposed in Mobile Safari on iPad but not on
  iPhone"; [WebKit bug 309965](https://bugs.webkit.org/show_bug.cgi?id=309965), "MediaCapabilities can't be
  used to query ManagedMediaSource support on iPhone": NEW (2026-03), so a MediaCapabilities probe with type
  `media-source` also reports nothing playable on iPhone (checked 2026-10-08).


### iPhone's ManagedMediaSource decides how far ahead the page may load (about 30 s)

- **Technology:** WebKit ManagedMediaSource.
- **Symptom:** `streaming` goes false at 30 s buffered and back true below 10 s, overriding hls.js's
  `maxBufferLength`. Content may be evicted at any time.
- **What we do:** catch-up arithmetic assumes 30 s can be loaded ahead.
  `web/packages/sdk/src/together-follow.ts:35-41` (`playerLoadsAheadMs`).
- **Evidence:** D250 §4; `research/playback-in-practice.md` §1; [source] WebKit `ManagedMediaSource.cpp`. The
  real thresholds on iOS are not device-confirmed (test T5).
  **[web]** [WebKit bug 326390](https://bugs.webkit.org/show_bug.cgi?id=326390), "ManagedMediaSource doesn't
  fire startstreaming after seeking outside the buffered range": NEW, filed 2026-10-05 against Safari 27.0.1
  on macOS and iOS. After `endstreaming` at about 30 s buffered, a seek past the buffer never sets `streaming`
  true again, so hls.js waits forever and the seek never finishes; its repro shows `endstreaming` at 30.1 s
  buffered. [WebKit bug 305712](https://bugs.webkit.org/show_bug.cgi?id=305712), "REGRESSION (iOS 26.2):
  ManagedMediaSource Cold Start Failure" (simultaneous appends to two SourceBuffers on the first MMS after
  Safari starts fail; sequential appends work): NEW (both checked 2026-10-08).


### iOS: an element's volume is read-only to the page, and Web Audio output follows the ring/silent switch

- **Technology:** iOS Safari.
- **Symptom:** `element.volume = x` is ignored. Routing the film through Web Audio to apply a gain would mute it
  on a phone in silent mode.
- **What we do:** `canSetVolume` writes the volume and reads it back. Where it cannot be set, no gain is
  applied, and only a prepared copy with the gain baked in is levelled.
  `web/packages/player/src/gain.ts:18-33,97-104`.
- **Evidence:** D226 §4; VQ13 (open, device check).
  **[web]** Apple's [Safari HTML5 Audio and Video Guide, iOS-Specific
  Considerations](https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/Using_HTML5_Audio_Video/Device-SpecificConsiderations/Device-SpecificConsiderations.html)
  [doc]: "The volume property is not settable in JavaScript. Reading the volume property always returns 1";
  [MDN browser-compat-data](https://github.com/mdn/browser-compat-data/blob/main/api/HTMLMediaElement.json)
  says the same for iOS. [WebKit bug 237322](https://bugs.webkit.org/show_bug.cgi?id=237322), "webaudio api is
  muted when the iOS ringer is muted": RESOLVED CONFIGURATION CHANGED; WebKit's Jean-Yves Avenard (2024-09)
  says the default audio session type is `ambient`, hence muted by the switch. **Better workaround:** since
  iOS 17, `navigator.audioSession.type = "playback"` keeps Web Audio playing with the switch on silent (same
  bug, comment 6); the older workaround was a looping silent `<audio>` element. A gain through Web Audio may
  therefore be possible on iOS 17+, subject to the MSE and HLS Web Audio bugs in the next entry.
  **Not adopted (2026-10-08):** it gives the film nothing, since a streamed film through Web Audio is silent
  on Safari (next entry) and an element's volume stays read-only; and voice, the one thing we play
  through Web Audio, played only once the microphone was open, when iOS has already made the session
  play-and-record, which the silent switch does not mute. Setting "playback" then could disturb the
  microphone. It matters again only if something plays through Web Audio without the microphone.
  **Adopted (2026-10-12):** the page's session is "playback" from its start, as a video app's,
  and "auto" only while a microphone is open (`web/packages/sdk/src/page-audio.ts`). That alone did not
  give an iPhone its sound with the switch on: [WebKit bug 323104](https://bugs.webkit.org/show_bug.cgi?id=323104)
  leaves a tab stuck in the switch-muted category after Safari's GPU process restarts, as iOS does freely
  in the background, while every API says playback; quitting Safari cleared it on the owner's iPhone,
  which then played with the switch on. The page heals it by setting "ambient" and, a tick later, its
  type again, at each tap for sound, each return of the page to visible, and each `emptied` of a film's
  video (`healPageAudio`). design/research/ios-silent-switch-2026-10-12.md. VQ38.


### iPhone Safari's native HLS cannot be routed through Web Audio, so it cannot be given a gain

- **What we do:** the gain is carried in the prepared sound (`CarriedGainDB`,
  `internal/nodeproto/prepare.go:66-68`).
- **Evidence:** D226 §4-§5; VQ13.
  **[web]** [WebKit bug 180696](https://bugs.webkit.org/show_bug.cgi?id=180696), "createMediaElementSource()
  not working with Hls stream": NEW since 2017; [WebKit bug
  306493](https://bugs.webkit.org/show_bug.cgi?id=306493), "Safari: MediaElementAudioSourceNode outputs
  silence for HLS (TS + fMP4) while playback is audible; progressive MP4 works": NEW (2026-01). [WebKit bug
  266922](https://bugs.webkit.org/show_bug.cgi?id=266922), "MediaElementAudioSourceNode doesn't pass audio
  sample data to subsequent node if based on MSE": NEW since 2023, reproduced on iPadOS and, in a 2025
  comment, with hls.js on ManagedMediaSource. **Contradicts:** the limitation is not only native
  HLS: on Safari an MSE or ManagedMediaSource element also gives Web Audio silence, so hls.js on iPhone cannot
  be given a Web Audio gain either (checked 2026-10-08).


### iOS suspends the AudioContext as the microphone opens

- **Symptom:** voice goes silent after getUserMedia.
- **What we do:** `context.resume()` again once the microphone is open.
  `web/packages/sdk/src/together-voice-browser.ts:236-239`.
- **Evidence:** the code comment only, with no date or device test. Related: WebKit 218012 [web].
  Cause unverified.
  **[web]** [WebKit bug 218012](https://bugs.webkit.org/show_bug.cgi?id=218012), "Audio Volume reduces
  considerably on accepting the mic permissions": RESOLVED CONFIGURATION CHANGED. WebKit's youenn fablet
  (comment 5) says capture changes the audio session category; an AudioContext created before capture plays
  quietly, and suspending and resuming it during capture restores the volume, which matches our resume after
  the microphone opens. The remainder for Web Audio moved to [WebKit bug
  236219](https://bugs.webkit.org/show_bug.cgi?id=236219), "[iOS] Volume of non MediaStreamTrack-based audio
  reduces when capturing microphone": NEW. Workarounds reported there: render Web Audio through a
  `MediaStreamAudioDestinationNode` (youenn, 218012 comment 32), or set `navigator.audioSession.type =
  'play-and-record'` before capture and `'playback'` after (comment 37, iOS 16.4+), though later comments say
  it works only sometimes and routes playback to the receiver. [WebKit bug
  273511](https://bugs.webkit.org/show_bug.cgi?id=273511), "AudioContext stuck on Interrupted" (`resume()`
  does not help): NEW (all checked 2026-10-08).


### iPhone video fullscreen is not the Fullscreen API, and it leaves page overlays behind

- **Technology:** iOS Safari `<video>`. iPhone has no element fullscreen, and its player is always fullscreen.
- **Symptom:** subtitles drawn by JASSUB over the video vanished in fullscreen ("they stop showing when I
  switch to full screen mode"). `fullscreenchange` never fires for the iPhone's player.
- **What we do:** the chosen track is also given to the browser as a WebVTT track, shown only in native
  fullscreen, and toggled on `fullscreenchange` and on WebKit's `webkitbeginfullscreen`/`webkitendfullscreen`.
  `web/packages/player/src/fullscreen-captions.ts:78-123`.
- **Evidence:** D254, [owner] 2026-10-07 (the owner's phone); VQ25.
  **[web]** [MDN browser-compat-data](https://github.com/mdn/browser-compat-data/blob/main/api/Element.json):
  `requestFullscreen` on iOS Safari is "Only available on iPad, not on iPhone"; [WebKit bug
  206854](https://bugs.webkit.org/show_bug.cgi?id=206854), "Add Fullscreen API to iOS": NEW since 2020;
  [WebKit bug 265438](https://bugs.webkit.org/show_bug.cgi?id=265438), "no fullscreen api functions available
  in iOS 17.2": NEW, where a WebKit engineer notes the iOS Fullscreen API was disabled again from Safari 17.2
  beta 3 (all checked 2026-10-08).


### Safari's native HLS starts on the first variant listed, whatever start hint is given

- **What we do:** a lower start quality, chosen from the remembered link speed, is *listed first* in the master
  playlist (D257 §2).
- **Evidence:** D257 [web/research].
  **[web]** Apple's [HLS authoring specification for Apple
  devices](https://developer.apple.com/documentation/http-live-streaming/hls-authoring-specification-for-apple-devices)
  [doc], item 1.32: "Defaults are the first variant listed in the Multivariant Playlist within a group of
  variants having compatible audio", and the default video variant SHOULD be the 2000 kbit/s one (read
  2026-10-08).


### iPhone Safari loads nothing for a media element until the user has tapped it

- **Symptom:** the film page's hidden warm-up player loads nothing on iPhone.
- **What we do:** on such browsers the warm-up only starts the chunks on the server (D260 §3).
- **Evidence:** D260; `research/safari-versions.md` §3 "preload" [source]. This is a
  limitation.
  **[web]** Apple's [Safari HTML5 Audio and Video Guide, iOS-Specific
  Considerations](https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/Using_HTML5_Audio_Video/Device-SpecificConsiderations/Device-SpecificConsiderations.html)
  [doc]: on iOS "preload and autoplay are disabled. No data is loaded until the user initiates it", and
  `play()` and `load()` do nothing unless triggered by a user action (archived 2012 guide, read 2026-10-08).


### iPhone Safari refuses a film's sound that no recent tap started, and a tap long before does not count

- **Symptom:** a Watch Together guest on an iPhone saw one still picture after another and heard
  nothing: her player sat paused, each catch-up seek showing its frame, for the 95 s she watched (the
  owner's test, 2026-10-08, iOS 26.6.2). She had tapped Join 35 s before the film started.
- **Cause:** `play()` on a video with sound is refused with `NotAllowedError` unless a user gesture
  started it; the gesture must be recent, and the element that plays is made only when the film
  starts. A muted video with `playsinline` may play without one. Low Power Mode refuses even that.
- **What we do:** the follower plays a refused film muted, fires `playedMutedEvent` and notes
  `playedMuted`; the viewer's page shows **Tap for sound** over it, whose tap turns the sound on and
  plays it (`soundOnAtTap`, `web/packages/sdk/src/together-follow.ts`; the owner's choice,
  2026-10-08). Before, a refused play was swallowed and tried again each second, silently.
- **Evidence:** **[owner]** staging's log of her session: `playing=false` throughout, drift growing a
  second a second between catch-up seeks. **[web]** [WebKit, "New `<video>` Policies for
  iOS"](https://webkit.org/blog/6784/new-video-policies-for-ios/): videos without audio, or muted, may
  autoplay with `playsinline`; `play()` of a video with sound needs a user gesture, and its promise
  rejects otherwise. Apple's guide (above) [doc].

### Native HLS gives the page no track choice, no quality control and no link measurement

- **What we do:** `setAudioRendition` is a no-op and `estimateKbps` is undefined on the native path, so the
  dialogue-boost switch does nothing there. `web/packages/player/src/hls.ts:65-77`.
- **Evidence:** `research/safari-versions.md` §3 "Quality control", §6; `research/audit-2026-09-27.md`. This is a
  limitation.
  **[web]** **Contradicts:** Safari exposes a native HLS stream's `EXT-X-MEDIA` audio renditions as
  `video.audioTracks` (Safari 7+, [MDN
  browser-compat-data](https://github.com/mdn/browser-compat-data/blob/main/api/HTMLMediaElement.json)), so a
  track choice is possible on the native path, though buggy: [WebKit bug
  297904](https://bugs.webkit.org/show_bug.cgi?id=297904), "REGRESSION (iOS 26): video.audioTracks mislabels
  default language and fails to switch audio tracks reliably with HLS": NEW; [WebKit bug
  301928](https://bugs.webkit.org/show_bug.cgi?id=301928), duplicated `AudioTrack` IDs within one group: NEW;
  [WebKit bug 318785](https://bugs.webkit.org/show_bug.cgi?id=318785), AudioTrack objects re-created in Safari
  26.3-26.5: NEW (checked 2026-10-08). No public API for quality control or link measurement on native HLS was
  found.


### Native-HLS `<video>` (and `<img>`) cannot send an Authorization header

- **What we do:** a media cookie only the byte paths under `/api/v1/bytes/` take, while API calls and
  hls.js use the header. Over TLS it is `__Host-` named, at `/`, as that prefix requires, and a byte
  path takes it only when `Sec-Fetch-Site` is absent, `same-origin` or `none`: every core is one site
  under `orpheum-core.com`, so `SameSite` alone let other cores' pages use or set it (the D308
  audit's F-6). `internal/core/api/media_cookie.go`.
- **Evidence:** D154, D216. This is a limitation that forced the design.
  **[web]** no tracker; it is how media elements fetch. [Stack Overflow
  56146913](https://stackoverflow.com/questions/56146913/using-token-based-authentication-for-html5-video) and
  [68483348](https://stackoverflow.com/questions/68483348/how-do-i-provide-token-in-the-query-string-for-every-subsequent-m3u8-file-reques)
  show the usual workarounds: a header only where a JavaScript player (hls.js, video.js) makes the requests,
  otherwise a cookie, or a token in the URL of every playlist and chunk (searched Stack Overflow and
  whatwg/html, 2026-10-08).


### iOS Safari with the microphone open sends call audio to the earpiece, quietly (WebKit 230902)

- **What we do:** headphones are the supported voice set-up (D63). On a device with a touch screen
  and Safari's audio session, without headphones, the first press of Talk on a page opens the
  microphone and pops up a notice, closed by OK, that only headphones carry the film and the voices
  together (the owner, 2026-10-11, whose iPhone's film went to the earpiece each time it talked, playing on); when voice ends, the session is set to `playback`, so iOS leaves the call at once rather
  than some 20 s later (the owner's test, 2026-10-08, where a guest's film went silent on the phone's
  speaker; VQ26 confirms it on a phone). `web/apps/web/src/together-voice.tsx`
  (`movesSoundToTheEarpiece`), `web/packages/sdk/src/together-voice-browser.ts` (`endCallAudio`).
- **Evidence:** D188 [web]. Whether `navigator.audioSession` fixes it is unverified.
  **[web]** [WebKit bug 230902](https://bugs.webkit.org/show_bug.cgi?id=230902), "REGRESSION(Safari 15 -
  iOS15): [WebRTC] MediaStreamTack audio volume too low": RESOLVED CONFIGURATION CHANGED; Twilio and others
  there diagnosed the route as the earpiece rather than a low volume. **Fixed upstream:** WebKit's youenn
  fablet (comment 116) says iOS 15.4 needs no workaround for MediaStreamTrack audio, 15.2-15.3 partly; the fix
  covers MediaStreamTrack rendering only, and the Web Audio and media-element part is [WebKit bug
  236219](https://bugs.webkit.org/show_bug.cgi?id=236219): NEW. On `navigator.audioSession`: a comment on
  [WebKit bug 218012](https://bugs.webkit.org/show_bug.cgi?id=218012) (iOS 18.6) reports that `type =
  "play-and-record"` routes playback to the receiver, so it does not fix the earpiece; youenn (236219 comment
  6) says getUserMedia switches the session to play-and-record at the call ("VC") volume level (all checked
  2026-10-08).


### Safari supports zstd content-encoding only from 26 (iOS fully from 26.3)

- **What we do:** gzip to clients; zstd only between node and core (D129).
- **Evidence:** D129 [web, caniuse 2026-09-21]. This is a limitation.
  **[web]** [caniuse zstd data](https://github.com/Fyrd/caniuse/blob/main/features-json/zstd.json): Safari
  26.0-26.2 decodes zstd but does not request it; 26.3+ supports it, on macOS only with macOS 26.3 Tahoe.
  [WebKit bug 279815](https://bugs.webkit.org/show_bug.cgi?id=279815), "Support zstd content-encoding": still
  NEW; [WebKit bug 311087](https://bugs.webkit.org/show_bug.cgi?id=311087), "Safari hangs on HTTP/1.1
  responses with Content-Encoding: zstd + Transfer-Encoding: chunked over TLS" (Safari 26.3.1; gzip
  unaffected): NEW, a further reason to keep gzip to clients (checked 2026-10-08).


### Safari needs a copied HEVC picture tagged `hvc1` (FFmpeg's MP4 muxer writes `hev1` by default)

- **What we do:** `-tag:v hvc1` when copying HEVC. `internal/node/prepare/arguments.go:103-110`;
  `internal/node/chunks/arguments.go:84-86`; the comments at `internal/nodeproto/prepare.go:43-44` and
  `internal/nodeproto/chunks.go:49-50`.
- **Evidence:** tests `TestArguments_TagCopiedHEVCForSafariAndReEncodeTo8BitH264` and
  `internal/node/chunks/arguments_test.go:60`; commit "feat(prepare): the node prepares fMP4 HLS renditions with every audio track, standard and boosted, and serves them on its byte socket" (2026-09-25). **Cause unverified in the
  repo:** the tests cite cut-a §6, which has no hvc1 text, and no Safari failure is recorded. It is
  well-known Apple behaviour [web].
  **[web]** Apple's [HLS authoring specification for Apple
  devices](https://developer.apple.com/documentation/http-live-streaming/hls-authoring-specification-for-apple-devices)
  [doc], item 1.10: "use 'avc1', 'hvc1', or 'dvh1' rather than 'avc3', 'hev1', or 'dvhe'" (parameter sets in
  the sample description, not in the samples); [Stack Overflow
  32152090](https://stackoverflow.com/questions/32152090/encode-h265-to-hvc1-codec): QuickTime needs `hvc1`,
  and `-c:v copy -tag:v hvc1` relabels without re-encoding (read 2026-10-08). No WebKit bug states the
  rejection of `hev1`.

---

---

## Chrome and Android (getUserMedia and audio)


### Android: echo cancellation puts the phone in a call, which turns the film down and takes the volume buttons

- **Technology:** Chrome on Android (`AudioManagerAndroid`) + Android audio policy.
- **Symptom:** the film became barely audible and the side buttons controlled the call volume instead.
- **Cause:** [source] an input with platform AEC makes Chrome call `setMode(MODE_IN_COMMUNICATION)`. AOSP then
  caps music near voice volume and sends the volume keys to `STREAM_VOICE_CALL`. Software
  `echoCancellation: "remote-only"` is compiled out on Android and iOS.
- **What we do:** `shouldCancelMicrophoneEcho` is off by default.
  `web/packages/sdk/src/together-voice-browser.ts:13-28`; `web/packages/sdk/src/together.ts:103-107,120`;
  `internal/core/settings/registry.go:288-292`.
- **Evidence:** [owner] 2026-10-06 (an Android host with headphones); D63; `research/voice-apps.md` §0.1, §2.1.
  **[web]** [Chromium issue 243506](https://issues.chromium.org/issues/40319017), "improve webrtc audio on
  Android", FIXED (2013): per a [Stack Overflow
  answer](https://stackoverflow.com/questions/32215497/chrome-on-android-webrtc-volume-slider-doesnt-work-when-using-microphone)
  quoting it, Chrome chose communication mode on purpose so the VOICE stream's volume control works, which is
  why the side buttons move the call volume. No open bug asking to keep media volume found (searched Stack
  Overflow and Mozilla Bugzilla; issues.chromium.org search needs sign-in, 2026-10-08).


### Chrome on Android with processing off captures quietly ("quiet, sometimes silent, streams")

- **Symptom:** the partner was too quiet even at 400% receiver gain.
- **Cause:** [source] Chromium's own comment in `aaudio_stream_wrapper.cc` about the unprocessed preset.
- **What we do:** the comment notes "without gain control a voice is quiet". Levelling is a setting, and VQ23
  tests Chrome with levelling on and echo cancelling off.
- **Evidence:** [owner] 2026-10-07; `research/voice-apps.md` §0.2. **Unsure:** the code workaround is only a
  setting, and the default is still off (see 1.4).
  **[web]** no public report found beyond Chromium's own source comment (searched Stack Overflow;
  issues.chromium.org search needs sign-in, 2026-10-08).


### Chrome plays a remote WebRTC stream into Web Audio only while a media element also holds it

- **Symptom:** remote voice routed only through Web Audio is silent.
- **What we do:** a muted `Audio` element holds the stream.
  `web/packages/sdk/src/together-voice-browser.ts:78-111` (comment at `:79-82`).
- **Evidence:** the code comment only. Cause unverified; it is long-standing Chrome behaviour [web].
  **[web]** [Chromium issue 933677](https://issues.chromium.org/issues/40094084), "MediaStream from RTC is
  silent (in WebAudio, MediaRecorder, etc.) if not attached to a media element": filed 2019-02-20, still
  ASSIGNED, last changed 2026-06-11 (read 2026-10-08). The muted media element is the workaround others use
  too ([Stack Overflow
  41784137](https://stackoverflow.com/questions/41784137/webrtc-doesnt-work-with-audiocontext),
  [53325793](https://stackoverflow.com/questions/53325793/no-audio-from-webrct-stream-on-chrome-without-audio-tag)).


### Chrome DevTools' network throttling does not slow a service worker's requests

- **Symptom:** with "Slow 4G" or "3G" set in a tab's Network panel, a film on the client origin
  went on at 2.3 Mb/s and its player never stepped down (the owner, staging, 2026-10-18).
- **Cause:** throttling is per DevTools target; the page's requests the service worker answers,
  and the worker's own fetches to the network, belong to the worker, which the tab's setting does
  not touch.
- **What we do:** to test a slow link, tick "Bypass for network" under Application → Service
  workers and reload, or open the worker from `chrome://inspect/#service-workers` and throttle in
  its own window. Nothing in the code: the worker (`web/apps/web/src/sw.ts`) is the client
  origin's loader (D308).
- **Evidence:** the owner's test on staging, 2026-10-18. **[web]** "Setting network throttle
  doesn't affect requests handled by the worker" ([mswjs/msw discussion
  804](https://github.com/mswjs/msw/discussions/804)); Chrome's own [request conditions
  documentation](https://developer.chrome.com/docs/devtools/request-conditions) throttles per
  panel (read 2026-10-18).

### Brave on Android refused the microphone

- **Evidence:** [owner] 2026-10-06/07; `research/voice-apps.md` synthesis row 3. The cause has not been
  researched, and there is no workaround. **Unsure** whether to include it yet.
  **[web]** no report matching ours found (searched brave/brave-browser issues, 2026-10-08). Nearest:
  [brave-browser#37176](https://github.com/brave/brave-browser/issues/37176), Android, the microphone stays
  off after a first "block" is changed to "Ask every time", labelled waiting on Chromium upstream (Chrome does
  the same), open; [brave-browser#3725](https://github.com/brave/brave-browser/issues/3725) is Web Speech
  being disabled (desktop), not getUserMedia.

---

---

## Web platform, all browsers


### Secure-context APIs are simply absent on plain HTTP off localhost

- **Technology:** `[SecureContext]` APIs: `crypto.randomUUID`, `navigator.mediaDevices`, and others.
- **Symptom:** at `http://devnas.example:5173` every click threw before it sent anything, and sign-in "did
  nothing". `navigator.mediaDevices` is `undefined`, a TypeError rather than a permission error. Its sibling
  `crypto.getRandomValues` is *not* restricted.
- **What we do:** keys come from `getRandomValues` (`web/apps/web/src/idempotent.ts:13-22`), and a source scan
  bans `randomUUID`. The microphone check throws `NotSupportedError` with a clear message
  (`web/packages/sdk/src/together-voice-browser.ts:31-38`). Playwright reproduces this with
  `--host-resolver-rules`. The guest's voice needs a public HTTPS name (D243).
- **Evidence:** an agent's working note; fix an earlier commit (2026-10-05); D197. [doc]
  **[web]** [MDN, features restricted to secure
  contexts](https://developer.mozilla.org/en-US/docs/Web/Security/Secure_Contexts/features_restricted_to_secure_contexts)
  lists the Web Crypto API and `getUserMedia`, and [MDN
  `getRandomValues`](https://developer.mozilla.org/en-US/docs/Web/API/Crypto/getRandomValues) says it is "the
  only member of the Crypto interface which can be used from an insecure context" (read 2026-10-08). For
  testing only, Chrome can treat a named HTTP origin as secure with
  `chrome://flags/#unsafely-treat-insecure-origin-as-secure` ([Chromium, deprecating powerful features on
  insecure
  origins](https://www.chromium.org/Home/chromium-security/deprecating-powerful-features-on-insecure-origins/));
  it does not help real users.


### A browser plays only the original file's first audio track

- **What we do:** choosing any other track forces a prepare (reason `audio_track`, D230).
- **Evidence:** D230. This is a limitation (AudioTrackList is unusable in most browsers).
  **[web]** [MDN browser-compat-data for
  `HTMLMediaElement.audioTracks`](https://bcd.developer.mozilla.org/bcd/api/v0/current/api.HTMLMediaElement.json):
  Safari 7+ only; Chrome and Edge behind `#enable-experimental-web-platform-features`, Firefox behind
  `media.track.enabled` (read 2026-10-08). [Mozilla bug
  1999618](https://bugzilla.mozilla.org/show_bug.cgi?id=1999618), audioTracks not listing all tracks in
  Firefox, UNCONFIRMED.


### Browsers other than Chrome play no Matroska, and none plays TrueHD, DTS or most AC-3

- **What we do:** remux to fMP4 and transcode the sound (D3, D178).
- **Evidence:** working-notes.md; `research/cold-start.md` §4.4. This is a limitation.
  **[web]** [Mozilla bug 1422891](https://bugzilla.mozilla.org/show_bug.cgi?id=1422891), [meta] Support
  Matroska in Firefox: [Mozilla bug 1991752](https://bugzilla.mozilla.org/show_bug.cgi?id=1991752), "Enabled
  matroska by default", FIXED in 145 (H.264, AAC, Opus, Vorbis, VP8/9, AV1, HEVC; FLAC, MP3 and PCM still
  ASSIGNED), but [Mozilla bug 1988447](https://bugzilla.mozilla.org/show_bug.cgi?id=1988447), "Support
  Matroska in MSE", is NEW (read 2026-10-08). **Contradicts:** Firefox 145+ plays Matroska files directly, so
  "other than Chrome" is out of date for file playback; through MSE/hls.js, which is our path, it still does
  not, so the remux stays.


### A browser `WebSocket` cannot send an Authorization header

- **What we do:** the Watch Together socket authenticates in-band or by cookie (D154 bans credentials in URLs).
- **Evidence:** `research/audit-2026-09-27.md`. This is a limitation.
  **[web]** [whatwg/websockets#16](https://github.com/whatwg/websockets/issues/16), "Support for custom
  headers for handshake", open since 2017 (checked 2026-10-08); [Stack Overflow
  4361173](https://stackoverflow.com/questions/4361173/http-headers-in-websockets-client-api) collects the
  usual workarounds (cookie, token in the subprotocol, first message).


### Browsers cap a cookie's Max-Age at 400 days

- **What we do:** `internal/core/api/media_cookie.go:22-31`.
- **Evidence:** plan-deviations P20. [doc] Low priority.
  **[web]** [httpwg/http-extensions#1600](https://github.com/httpwg/http-extensions/issues/1600), "RFC6265bis:
  Standardize maximum Max-Age cookie attribute value/handling", closed; the cap is in RFC 6265bis, and [#2789](https://github.com/httpwg/http-extensions/issues/2789), asking why the limit binds the user agent, was closed in 2024 by PR #2792.

---

### An autoplaying video reads as paused until it can play, so a video that never can looks paused forever

- **Technology:** HTML media elements (all browsers), the `autoplay` attribute.
- **Symptom:** a video with `autoplay` that never gets its first frame reports `paused === true` and
  `readyState` 1 indefinitely, while the page shows it loading; nothing says it was ever asked to play.
- **Cause:** the spec's autoplaying flag starts playback only once the element reaches
  HAVE_ENOUGH_DATA; until then `paused` stays true [doc].
- **What we do:** D266 counts a video as waiting to play when it is not paused **or** is paused short
  of HAVE_FUTURE_DATA (`isWaitingToPlay`, `web/packages/player/src/abr-choice.ts`); stall reports carry
  `readyState` and `isPaused` (`internal/core/api/report.go`).
- **Evidence:** [owner] 2026-10-08: Flow resumed part-way in Firefox on macOS sat paused at readyState 1,
  and the first form of D266, which ignored paused videos, never fired.
  **[web]** [HTML spec, media elements](https://html.spec.whatwg.org/multipage/media.html): only on reaching
  HAVE_ENOUGH_DATA, and if "eligible for autoplay", "the user agent may ... Set the paused attribute to
  false"; whereas `play()`'s internal play steps "Change the value of paused to false" at once, whatever the
  readyState (read 2026-10-08). **Better workaround:** starting the video with `play()` instead of the
  `autoplay` attribute makes `paused` false immediately, so a video that never gets a frame reads as waiting
  rather than paused (a check like `isWaitingToPlay` still covers autoplay-blocked cases).

---

## hls.js (pinned at 1.7.3)
All except 5.9-5.13 are from reading hls.js 1.7.3's source (`research/abr-controller.md`, tag e5ff3583,
2026-10-07) and are what led to D265's own controller.


### The abandon rule is skipped while the video is paused

- **Symptom:** a hidden preload player, or one waiting for its first frame, "waited out a whole chunk" on a
  slow link.
- **Cause:** `abr-controller.ts:285-290` returns early on `media.paused || !playbackRate || !readyState`.
- **What we do:** our controller looks at the chunk in flight every 500 ms, playing or not.
  `web/packages/player/src/abr-choice.ts:110-118`; `web/packages/player/src/abr.ts:33,243-262`.
- **Evidence:** D265 §3; [staging] 2026-10-07; abr-controller.md:119-123.
  **[web]** no public report found of the paused early return (searched hls.js issues and DuckDuckGo,
  2026-10-08). Nearest: [hls.js#6689](https://github.com/video-dev/hls.js/issues/6689), "Playback stalls in
  case of strong bandwidth fluctuations": closed by [PR #6763](https://github.com/video-dev/hls.js/pull/6763)
  for 1.6.0, which reworked the check but kept the early return; at 1.7.3 the return is also skipped when a
  level change is pending (`levelChange`), so it bites only a paused player with no switch under way.


### It never aborts a slow chunk when stepping down to level 0

- **Cause:** `abortAndSwitch` aborts only if `nextLoadLevel > 0` (`abr-controller.ts:429-433`). The intent is
  undocumented.
- **What we do:** our give-up may choose the lowest quality, with `frag.abortRequests()` and
  `FRAG_LOAD_EMERGENCY_ABORTED`. `web/packages/player/src/abr-choice.ts:111-117`;
  `web/packages/player/src/abr.ts:384-393`.
- **Evidence:** D265 §3 ("the lowest included, which hls.js never gives up for"); abr-controller.md:132-138.
  **[web]** no public report found (searched hls.js issues, 2026-10-08). The guard came with [PR
  #6763](https://github.com/video-dev/hls.js/pull/6763), "Check-in on abandon rules check and switch to lowest
  when stuck on in-flight frag" (2024-10-09, resolves
  [hls.js#6689](https://github.com/video-dev/hls.js/issues/6689), in 1.6.0); its commit message gives no
  reason for `nextLoadLevel > 0`, and the same block logs `nextLoadLevel > 0 ? 'and switching down' : ''`, a
  branch that can never take the empty side, which suggests the level-0 case was not thought through rather
  than chosen.


### After an abandon, it resets the estimator to a nominal bitrate

- **Cause:** `abr-controller.ts:389-392` and `:456` replace the measured history with the lower level's
  BANDWIDTH.
- **What we do:** an overridden `registerListeners` keeps the built-in abandon check from running, and our
  give-up never calls `resetEstimator`. `web/packages/player/src/abr.ts:429-455`. This is not commented in the
  code.
- **Evidence:** abr-controller.md:128-131, :278; D265.
  **[web]** deliberate upstream: [PR #6082](https://github.com/video-dev/hls.js/pull/6082), "Fix regression in
  emergency switch down" (2024-01-05, resolves [hls.js#6079](https://github.com/video-dev/hls.js/issues/6079),
  in 1.5.0), added "Reset estimate when candidate bitrate is lower than adjusted estimate": the reset happens
  only when `estimate × abrBandWidthUpFactor` exceeds the lower level's bitrate, and [PR
  #6763](https://github.com/video-dev/hls.js/pull/6763) (1.6.0) added a second reset to the lowest switch
  level after an in-flight abort. No report found asking to keep the measured history (searched hls.js issues,
  2026-10-08).


### There is no cache-hit filter: a chunk from the browser's cache reads as link speed (hls.js#5826, open)

- **Symptom:** #5826 reads a throttled 1 Mb/s link as 404 MB/s. Here, the film page's preloaded chunks are read
  again by Play.
- **What we do:** a chunk whose Resource Timing entry transferred nothing for its body came from the cache
  and is not counted; one that transferred is counted however fast it came; where the browser says nothing,
  chunks loaded in under 10 ms are not counted. Paced chunks count only at 0.9 of the pace or less. The
  page's timing buffer is emptied each time it fills. `web/packages/player/src/cache-hit.ts`,
  `abr-choice.ts` (`measuresTheLink`), `abr.ts` (`measures`); D265 §7, amended 2026-10-08.
- **Evidence:** D265 §7; https://github.com/video-dev/hls.js/issues/5826; `research/slow-links.md` §2.
  **[web]** [hls.js#5826](https://github.com/video-dev/hls.js/issues/5826), "Cached Fragments increase
  BandwidthEstimate causing suboptimal quality selection": open, labelled "Browser issue" and "Missing
  Feature" (checked 2026-10-08). The maintainer says hls.js cannot filter cache hits because XHR cannot tell
  that a response came from cache, and suggests setting `hls.bandwidthEstimate` (a setter since 1.5.0) or a
  cache buster in `xhrSetup`; no change after 1.7.3 on master. **Better workaround:** [MDN,
  `PerformanceResourceTiming.transferSize`](https://developer.mozilla.org/en-US/docs/Web/API/PerformanceResourceTiming/transferSize)
  is 0 for a resource fetched from a local cache (and for a cross-origin one without `Timing-Allow-Origin`),
  so for our same-origin chunks a Resource Timing entry could mark a cache hit directly instead of the
  under-10 ms guess. Adopted 2026-10-08 (the owner).


### The `abrController` config type is nominal, so a custom controller must subclass

- **Cause:** `config.ts:402` types the field `typeof AbrController`, which has private members. A class written
  from scratch needs `as unknown as`, which our lint forbids.
- **What we do:** `class OrpheumAbrController extends AbrController` plus a subclass factory for each playback.
  `web/packages/player/src/abr.ts:379,503-512`.
- **Evidence:** abr-controller.md:18-24.
  **[web]** [TypeScript handbook, "Type
  Compatibility"](https://www.typescriptlang.org/docs/handbook/type-compatibility.html): a target type with a
  private member accepts only a source whose private member originated from the same class, which is why
  `typeof AbrController` is nominal. hls.js has a structural `AbrComponentAPI` type (from [PR
  #4842](https://github.com/video-dev/hls.js/pull/4842), merged 2023-01-16) but `config.ts` does not use it
  for the field. Upstream treats subclassing as the route: [PR
  #7040](https://github.com/video-dev/hls.js/pull/7040), "Separate the calculation of the next level into a
  protected method", merged 2025-02-24 for someone extending `AbrController`. No issue asks to type the field
  structurally (searched hls.js issues, 2026-10-08).


### The base constructor calls the virtual `registerListeners()` before the subclass's fields exist

- **Cause:** `abr-controller.ts:62`.
- **What we do:** listeners are kept in a module-level `WeakMap` keyed by the controller.
  `web/packages/player/src/abr.ts:365-372,429-453` (comment: "hls.js registers them from its own constructor,
  before the controller's fields exist").
- **Evidence:** abr-controller.md:174-180.
  **[web]** [MDN, "Public class
  fields"](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Classes/Public_class_fields): a
  derived class's fields are defined only after `super()` returns, so the base constructor cannot see them;
  this is the language, not hls.js. No hls.js issue found about it (searched hls.js issues, 2026-10-08).


### `EwmaBandWidthEstimator` is exported as a type only

- **Cause:** `hls.ts:1360,1398` use `export type`, so a from-scratch controller cannot build hls.js's estimator,
  which hls.js's other controllers read.
- **What we do:** subclassing keeps the parent's `bwEstimator` (`web/packages/player/src/abr.ts:374-385`).
- **Evidence:** abr-controller.md:65-68. This is an API limitation.
  **[web]** no public report found (searched hls.js issues, 2026-10-08); still `export type` on master after
  1.7.3. `docs/API.md` says a custom controller needs `abrController.bwEstimator.getEstimate()` for
  `hls.bandwidthEstimate()` to work, without offering a way to build hls.js's estimator.


### The docs understate the ABR controller's interface, and the setter echoes its own value back

- **Cause:** `docs/API.md` lists only `nextAutoLevel` and `destroy`. The core also calls `firstAutoLevel`,
  `forcedAutoLevel`, `bwEstimator` and `resetEstimator`. The level controller writes `nextAutoLevel` back with
  the value it just read on every fragment (`stream-controller.ts:297` → `level-controller.ts:706-709`).
- **What we do:** override all of them. An echo changes nothing; a different value is an outside force.
  `web/packages/player/src/abr.ts:162-165,474-489`.
- **Evidence:** abr-controller.md:55-61, :75. The docs disagree with the behaviour.
  **[web]** [hls.js#3562](https://github.com/video-dev/hls.js/issues/3562), "hls.bandwidthEstimate() uses
  undocumented abrController property": closed 2021-03-08 (fixed for `bwEstimator` only);
  [hls.js#4824](https://github.com/video-dev/hls.js/issues/4824), "Do Custom ABR Controllers really need
  get/set `autoLevelCapping`?": closed 2023-01-16, where the maintainer asked for a typed interface, and [PR
  #4842](https://github.com/video-dev/hls.js/pull/4842) added one, merged 2023-01-16 (checked 2026-10-08).
  That interface is `AbrComponentAPI` in `src/types/component-api.ts` (exported as a type): `firstAutoLevel`,
  `forcedAutoLevel`, `nextAutoLevel`, `bwEstimator`, `resetEstimator`, `destroy`, which matches what we found
  the core calls; `docs/API.md` at 1.7.3 still lists only `nextAutoLevel` and `destroy`. No report found of
  the `nextAutoLevel` echo.


### A 425 is never retried, and loads give up after about 10 s by default

- **Cause:** the default `fragLoadPolicy` does not retry 4xx statuses and times out at about 10 s. As stated in
  the comment; the default values were not re-quoted from hls.js's source.
- **What we do:** our own `LoadPolicy`: a 30 s first-byte wait, 120 s for the load, and many retries every 1-2 s,
  with `shouldRetry` retrying 425. `web/packages/player/src/chunked.ts:26-33,65-99` (comment at `:84-87`).
- **Evidence:** D250 §6, D247 §10. **Unsure:** this is a reasonable default meeting our non-standard use of 425
  ("being made"), not a bug.
  **[web]** deliberate upstream: 4xx has been final since [PR
  #2442](https://github.com/video-dev/hls.js/pull/2442) (2020, "HTTP 4XX responses are now marked as
  'final'"), and `retryForHttpStatus` at 1.7.3 says "Do not retry on status 4xx"; `docs/API.md` documents the
  `shouldRetry` override we use. [RFC 8470](https://www.rfc-editor.org/rfc/rfc8470.html) defines 425 (Too
  Early) for replay risk of TLS early data, so our "being made" meaning is our own. The 1.7.3 defaults, read
  from `src/config.ts`: `maxTimeToFirstByteMs` 10000 and `maxLoadTimeMs` 120000, with a timeout retried 4
  times at once and an error 6 times at 1-8 s; so "about 10 s" is the first-byte wait, and a silent server
  fails the load after about 50 s. No report found of 425 specifically (searched hls.js issues, 2026-10-08).


### A held request is counted as download time, so the estimate collapses

- **Cause:** [source] the sample is `total − min(ttfb, ttfbEstimate)`, and the TTFB EWMA barely weighs one long
  wait (`abr-controller.ts:534-541`, `ewma-bandwidth-estimator.ts:62-68`).
- **What we do:** a chunk that is not ready is answered at once with 425, never held (`chunk_hold_ms` defaults to
  0). `internal/core/api/chunks.go:35-39,196-201`.
- **Evidence:** D250 §5; `research/playback-in-practice.md` §3; suspected on [staging] 2026-10-06.
  **[web]** no public report found (searched hls.js issues for TTFB and first-byte estimates, 2026-10-08). The
  sample at 1.7.3 is `loading.start` to parse end minus `min(ttfb, ttfbEstimate)` (`abr-controller.ts`
  `onFragBuffered`), as stated.


### An open (EVENT, no ENDLIST) playlist is treated as live and starts near its end

- **Symptom:** "a film whose prepare had made 46 s began at 40 s".
- **What we do:** `startPosition: 0`. `web/packages/player/src/hls.ts:191-197`.
- **Evidence:** [staging] 2026-10-06. [doc] This is live-playlist behaviour that surprises for a growing VOD.
  **[web]** the HLS spec asks for it: [RFC 8216 §6.3.3](https://www.rfc-editor.org/rfc/rfc8216.html), "If the
  EXT-X-ENDLIST tag is not present and the client intends to play the media normally, the client SHOULD NOT
  choose a segment that starts less than three target durations from the end of the Playlist file". hls.js
  `docs/API.md` (`startPosition`, default -1) starts live by `liveSyncDuration`/`liveSyncDurationCount`. No
  hls.js issue asks for EVENT playlists to start at 0 (searched 2026-10-08); `startPosition: 0` is the
  documented route.


### iPhone Safari keeps a sliver of a well-formed copied chunk

- **Symptom:** an iPhone in a Watch Together group, playing Copper Kettle S2E2's own picture copied as it
  is, buffered 0.08–0.34 s of the first chunk and nothing more of it, the next chunks whole from 10.22 s;
  hls.js stepped over the hole, found the playhead still, and gave the playback up as fatal
  (`bufferSeekOverHole (fatal): Playhead still not moving after seeking over hole`). S2E1 played.
- **Cause:** not known. The chunk is well formed: one fragment, 242 H.264 frames over 10 s, its
  decode time right, its sample sizes filling its `mdat` exactly, presentation times continuous;
  `ffprobe` reads it whole. Safari (ManagedMediaSource, iOS 18.5) held 6 of its frames. Not checked on a
  Mac's Safari.
- **What we do:** a fatal hole is recovered from rather than failed: the quality is given up as
  unplayable here (D266) and loaded again below it, from where the video stands (`passAHole`,
  `web/packages/player/src/abr.ts`; `failOnFatal` passes a recovered error by).
- **Evidence:** [owner] 2026-10-13, staging logs and the chunk taken from the node. Not yet searched
  for in WebKit's tracker.

### hls.js moves the playhead itself (gap jumps, stall nudges), and the browser reports each as a seek

- **Symptom:** a Watch Together host broadcast hls.js's own gap steps as user seeks ("each such step became a
  jump for everyone"). A held follower whose seek was still loading kept being moved, and re-sought forever.
- **Cause:** `BUFFER_SEEK_OVER_HOLE` and `BUFFER_NUDGE_ON_STALL` set `currentTime`, and they surface only as
  non-fatal ERROR events.
- **What we do:** those events are re-dispatched as `orpheumplayermoveditself`
  (`web/packages/player/src/hls.ts:108-122`). The follower ignores the next `seeking`
  (`web/packages/sdk/src/together-follow.ts:176-182`) and tolerates drift while its own seek loads (`:637-639`).
- **Evidence:** [owner] 2026-10-05.
  **[web]** [hls.js#7680](https://github.com/video-dev/hls.js/issues/7680), "Audio gap causing seek events on
  Chrome only": closed (checked 2026-10-08); its logs show a gap jump arriving as `seeking`/`seeked` on the
  media element, as here. No report asks for hls.js to mark its own moves. **Better workaround:** since 1.7.0
  the gap controller is replaceable (`config.gapController`, [1.7.0 release
  notes](https://github.com/video-dev/hls.js/releases/tag/v1.7.0)); a subclass could flag its own
  `currentTime` writes at the source instead of inferring them from non-fatal ERROR events (untested).


### A paused player is never stepped over a gap at the start, so it never becomes ready

- **Symptom:** "Both players held at 0 with 0.04 s to 60 s loaded … held the group up for good."
- **What we do:** a held player is moved onto the first frame it loaded.
  `web/packages/sdk/src/together-follow.ts:640-645`.
- **Evidence:** [staging] 2026-10-06. The cause is stated in the comment, but hls.js's source is not cited.
  **[source]** confirmed in hls.js 1.7.3 `gap-controller.ts`: `pausedEndedOrHalted` (`media.paused &&
  !seeking`) returns before any gap handling, and the start-gap jump itself runs only `if (!media.paused)`.
  **[web]** no public report found (searched hls.js issues for start gaps while paused, 2026-10-08).


### After an audio-track switch, the audio already decoded in the old mix plays on for 2-4 s

- **Cause:** hls.js flushes the buffer but not the decoder ("said plainly it does not flush the decoder").
- **What we do:** a seek in place after the switch. `web/packages/player/src/hls.ts:236-246`.
- **Evidence:** agent-failures A7 (2026-09-26); VQ2 ticked ("works within about a second now"); D217 §3. The first
  diagnosis, segment length, was wrong (an agent error).
  **[web]** [hls.js#5881](https://github.com/video-dev/hls.js/issues/5881), "Audio track switching does not
  apply immediately" (Firefox, old audio plays on for seconds): open, milestone 1.8.0, labelled "Browser
  issue", "Suggested-Workaround" and "seek-to-flush-rendering-pipeline";
  [hls.js#6355](https://github.com/video-dev/hls.js/issues/6355) has the same label for a video level switch
  in Chrome and Edge, open (both checked 2026-10-08). The maintainers' own label names our workaround: a seek
  flushes the browser's rendering pipeline, which a SourceBuffer flush does not.


### Before any measurement, `bandwidthEstimate` returns its configured default, with no "unknown"

- **Symptom:** a phone reported its link as 500, then 5000 kb/s (hls.js's starting values), and the core
  remembered that.
- **What we do:** `measuredKbps` is undefined until `FRAG_LOADED`. `web/packages/player/src/hls.ts:143-156`;
  `abr.ts:352`.
- **Evidence:** [staging] 2026-10-07. **Borderline:** this is a documented default.
  **[web]** [hls.js#5949](https://github.com/video-dev/hls.js/issues/5949), "Improving initial video quality
  based on previous sessions": closed, answered (checked 2026-10-08), someone persisting
  `hls.bandwidthEstimate` across sessions, where the default and a measurement look alike. **Better
  workaround:** hls.js's estimator has `canEstimate()` (`ewma-bandwidth-estimator.ts`, true once a sample has
  enough weight), and `getEstimate()` returns the default only while it is false; our subclass holds
  `bwEstimator`, so it could ask that directly rather than inferring from `FRAG_LOADED`.

---

### hls.js asks for a jump's sound only after its picture

- **Technology:** hls.js 1.7.3, `audio-stream-controller.ts`, a picture and a sound in separate playlists.
- **Symptom:** a jump on a quality made on request waited 12.5 s: the picture's first chunk came at 7.4 s,
  and the sound's run was only queued then, taking 5 s more (the owner's test, 2026-10-12, Flow on a Mac).
- **Cause:** the sound's loader loads at most one fragment past the main one loading
  (`audio-stream-controller.ts:429-461`, "Request audio segments up to one fragment ahead of main
  stream-controller"): a sound fragment that starts past the end of `mainFragLoading` waits. After a jump the
  picture's fragment there is still on its way, so the sound is asked for only once it has come [source;
  that it was the picture's chunk it waited on, the core's log].
- **What we do:** the core makes the sound beside the picture: a picture's file asked for where a seek took
  its player (`Orpheum-Sought`) starts the sound the playback last asked for at the same chunk.
  `internal/core/chunks/files.go` (`WantAlongside`), `internal/core/api/chunks.go`,
  `internal/core/playback/chunks.go` (`SoundOf`).
- **Evidence:** [owner] 2026-10-12, the core's log: the sound's run queued 7.4 s after the picture's;
  `/yoloai/cache/seek-research/hls.js` (v1.7.3-24-gc407464).

### hls.js reports a fragment the browser held nothing of, then does nothing about it

- **Technology:** hls.js 1.7.3, `bufferAppendNoProgress`.
- **Symptom:** the browser takes a fragment's append without error but adds nothing to the buffer. hls.js
  raises a non-fatal `bufferAppendNoProgress` error, and its error controller answers with a do-nothing action.
  The player goes on loading chunk after chunk that is never held, and the video never starts.
- **Cause:** `error-controller.ts` groups `BUFFER_APPEND_NO_PROGRESS` with stalls and nudges, which need no
  action [source].
- **What we do:** D266. The bitrate controller counts these for picture chunks while the video stands still.
  Two of them give the quality up at once (`notHeld` in `web/packages/player/src/abr.ts`).
- **Evidence:** [owner] 2026-10-08, Flow in Firefox on macOS; `/yoloai/cache/hls.js`
  `src/controller/error-controller.ts` and `buffer-controller.ts` at v1.7.3.
  **[web]** `bufferAppendNoProgress` is new in 1.7.0: [PR
  #7941](https://github.com/video-dev/hls.js/pull/7941), "Mark fragments as gaps after repeated appends
  without buffered range growth", merged 2026-07-27, and [PR
  #7979](https://github.com/video-dev/hls.js/pull/7979) fixed false positives in interstitial assets, merged
  2026-08-19. **Contradicts:** in part. The stream controller does act: it marks a fragment as a gap once the
  *same* fragment has been appended `appendErrorMaxRetry` (default 3) times without growth
  (`stream-controller.ts`, `BUFFER_APPEND_NO_PROGRESS` case). The count is per fragment, so a run of different
  chunks each held nothing of, as with Flow, never reaches it; the do-nothing answer in `error-controller.ts`
  is only the error controller's part. Related:
  [hls.js#8035](https://github.com/video-dev/hls.js/issues/8035), "locally determined gaps do not survive
  recoverMediaError()": open (checked 2026-10-08), which matters because D266 rebuilds with
  `recoverMediaError`.

### hls.js leaves its picture loader idle after `recoverMediaError` when a chunk was loading

- **Technology:** hls.js 1.7.3, `recoverMediaError`, any browser.
- **Symptom:** after D266 gave up the film's own picture at 0:00, only the sound loaded again: the steps read
  `detached; error aborted; sound 0/0 loading; attached; sound 0/0 loaded`, then nothing. The picture's loader
  never chose a quality or asked for a playlist or a chunk, and the video held nothing for as long as it was
  left. Mid-film, the same give-up recovered.
- **Cause:** `recoverMediaError` detaches the media, which stops the stream controller (`STOPPED`) and aborts
  the chunk it was loading. The abort's error comes after, and `handleFragLoadAborted` calls
  `resetFragmentLoading`, which sets `IDLE` whenever no chunk is current, as the stop left it. On the new
  attach, `onMediaAttached` restarts only a loader still `STOPPED`, so this one is skipped. It idles with its
  old load position, the end of what it had loaded, and with no metadata yet it looks for a chunk there.
  `recoverMediaError` calls `startLoad(currentTime)` itself only when `currentTime` is not 0, so a give-up
  mid-film was rescued by that call and one at 0:00 was not [source] [measured]. It is a race: a chunk that
  finished loading before the detach left nothing to abort, and the loader restarted.
- **What we do:** `loadAgainFrom` in `web/packages/player/src/abr.ts` starts loading again from where the video
  stood once the new media source is attached, whatever state hls.js left its loader in.
- **Evidence:** [owner] 2026-10-18 on staging, A Woman of Paris from 0:00 in Firefox on macOS, the stall's
  steps above. [measured] headless Firefox on Linux, our player on a stream whose first video SourceBuffer
  silently empties every append (`.dev/hlsrepro/ours.cjs`): three of four trials stuck, the loader `IDLE`
  while detached with `nextLoadPosition` 40.08 s; with `loadAgainFrom`, six of six played. [source]
  `base-stream-controller.ts` (`resetFragmentLoading`, `onMediaAttached`) and `hls.ts` (`recoverMediaError`)
  at v1.7.3. **[web]** searched 2026-10-18: no hls.js issue names it.
  [hls.js#2652](https://github.com/video-dev/hls.js/issues/2652) shows the same `STOPPED`, `IDLE` transitions
  around a `recoverMediaError`, and [hls.js#8035](https://github.com/video-dev/hls.js/issues/8035) is a
  related loss of state across it.

---

## WebRTC


### Chrome and Safari use RED only when it is listed first

- **Cause:** [source] libwebrtc generates RED but places it after Opus.
- **What we do:** `setCodecPreferences` with RED first.
  `web/packages/sdk/src/together-voice-codec.ts:73-86`; `together-voice-browser.ts:114-128`.
- **Evidence:** D262 §3; `research/voice-bad-links.md` §3 "RED".
  **[web]** [webrtcHacks, "RED: Improving Audio Quality with
  Redundancy"](https://webrtchacks.com/red-improving-audio-quality-with-redundancy/) (the libwebrtc author):
  RED is not enabled by default because of its bandwidth cost; to use it, put it before Opus with
  `setCodecPreferences` or SDP munging. This is by design, not a bug. [Mozilla bug
  1801367](https://bugzilla.mozilla.org/show_bug.cgi?id=1801367) (comment 0) shows Chrome, Edge and Safari
  offering RED second, right after Opus, and its 2026-06-18 patch for Firefox keeps the same "enabled only if
  listed before Opus" rule (checked 2026-10-08).


### No browser API caps the jitter buffer: `jitterBufferTarget` is a floor, not a ceiling

- **Symptom:** voice arrived about 1.1 s late under the emulated congested link, about 0.9 s of it in NetEQ.
  Signal caps NetEQ at 500 ms; a web page cannot.
- **What we do:** remove the jitter instead. The film is paced server-side and capped at half the link while
  voice is live (D262 §2; `internal/platform/pacing`).
- **Evidence:** D262; `research/voice-bad-links.md` §3 [measured + source].
  **[web]** [w3c/webrtc-extensions#199](https://github.com/w3c/webrtc-extensions/issues/199), "Add
  jitterBufferMaximumDelay attribute to RTCRtpReceiver" (2024-02-09): states that `jitterBufferTarget` sets
  only a minimum and the upper bound is not controllable from the page; closed, and no maximum exists in the
  spec (checked 2026-10-08).


### `networkPriority` (DSCP) exists in Chrome and Safari but not Firefox, and `setParameters` may refuse it

- **What we do:** set both priorities and swallow a rejection.
  `web/packages/sdk/src/together-voice-browser.ts:130-143`.
- **Evidence:** D262 §3; `research/voice-bad-links.md` §3. This is a capability difference.
  **[web]** [Mozilla bug 1249575](https://bugzilla.mozilla.org/show_bug.cgi?id=1249575), "Enable dscp support
  for WebRTC": NEW since 2016, no patch (checked 2026-10-08). No public report found of Chrome or Safari
  rejecting it (searched GitHub and Bugzilla, 2026-10-08).


### mDNS-obfuscated host candidates cannot be resolved inside a container

- **Symptom:** two pages in the test container cannot call each other.
- **What we do:** Chromium gets `--disable-features=WebRtcHideLocalIpsWithMdns`, and Firefox gets
  `media.peerconnection.ice.obfuscate_host_addresses=false`. `web/e2e/browsers.ts:3-19`.
- **Evidence:** D252 §3. This is an environment limitation.
  **[web]** [draft-ietf-mmusic-mdns-ice-candidates
  §3.2.1](https://datatracker.ietf.org/doc/html/draft-ietf-mmusic-mdns-ice-candidates): an mDNS candidate that
  does not resolve is ignored, so when both peers obfuscate and multicast does not reach, neither has a host
  candidate for the other and only peer-reflexive learning (which needs one side's address) or STUN/TURN
  candidates remain. No browser bug tracks it: it is the design working as specified (searched Bugzilla and
  GitHub, 2026-10-08).

### pion waits 5 s for each STUN server while gathering, and a family with no way out never answers

- **Symptom:** on staging every offer through the discovery was refused with "gather the core's
  candidates: context deadline exceeded", and a phone with no direct path could not reach the core.
- **What we do:** `rtcapi` sets pion's STUN gather timeout to 2 s (`stunGatherTimeout`), well within
  the 5 s an answer waits for its candidates. `TestUseICEServers_AStunServerThatNeverAnswersDoesNotSinkTheAnswer`.
- **Evidence:** staging, 2026-10-14. pion/ice v4.4.7 `agent_config.go`: `defaultSTUNGatherTimeout = 5 *
  time.Second`; gathering completes only once every server-reflexive attempt has answered or timed
  out. The core's container had an IPv6 default route to Docker's bridge and no IPv6 beyond its
  host, so the IPv6 binding to the STUN server never came back, and gathering took the answer's
  whole 5 s. pion's `SettingEngine.SetSTUNGatherTimeout` is the knob.

### A phone on Wi-Fi gathers more than ten ICE candidates

- **Symptom:** a phone on home Wi-Fi got "Can't reach the server from this network" over WebRTC,
  while on mobile data it connected; the discovery logged "refused a client's offer … 11
  candidates; at most 10".
- **What we do:** the page sends an offer through a discovery with only the candidates a core
  outside its network can use, at most ten, relayed and UDP ones first
  (`web/packages/sdk/src/relayed-offer.ts`), so the cap the core and the discovery keep (the
  protocol's §2.5, S16) holds.
- **Evidence:** staging, 2026-10-14, Chrome on Android. The offer's lines were not logged, so which
  candidates made the eleven is not known; a browser gathers one per address and protocol, and a
  Wi-Fi interface holds an IPv4 address and several IPv6 ones (stable and temporary) where mobile
  data holds fewer, and a page granted the microphone gets real host addresses rather than mDNS
  names.

---

---

## FFmpeg, x265/x264, codecs and containers


### x265 makes every keyframe after the first a CRA (open GOP)

This is the encoder half of 1.1. It is listed so the FFmpeg/x265 group is complete. Evidence: D266.
- **Evidence:** **[web]** [x265 CLI docs](https://x265.readthedocs.io/en/master/cli.html): `--open-gop` "allow
  I-slices to be non-IDR", default enabled, so this is documented default behaviour, not a bug (checked
  2026-10-08). **Better workaround:** for pictures we encode ourselves, `-x265-params open-gop=0` makes every
  keyframe an IDR, and libx265's `-forced-idr 1` ([FFmpeg codecs docs](https://ffmpeg.org/ffmpeg-codecs.html))
  makes forced keyframes IDR; copied pictures keep their CRAs.


### The `segment` muxer misses a keyframe a hair before the cut time (10.009999 against 10.010)

- **What we do:** a copied picture is cut 50 ms early (as Kyoo's `hls_time_delta 0.05`).
  `internal/node/chunks/arguments.go:14-18,49-62`.
- **Evidence:** `research/slow-links.md` §10 [measured 2026-10-05]; test
  `TestArguments_ReadTheRunsChunksAndCutAtEachAndAtTheEnd`.
  **[web]** [FFmpeg formats docs, segment muxer](https://ffmpeg.org/ffmpeg-formats.html): documents exactly
  this, "key frame times ... may not be set accurately because of rounding issues", and offers
  `segment_time_delta` (a keyframe starts a segment if PTS >= start_time - time_delta). Related: [FFmpeg
  ticket 9523](https://trac.ffmpeg.org/ticket/9523), "segment_times splits at I-frames that are not
  keyframes": open (checked 2026-10-08). **Better workaround:** `-segment_time_delta 0.05` is the muxer's own
  flag for our 50 ms early cut, so the cut times could stay exact.


### An input seek at a keyframe of a Matroska film lands on the keyframe before it

- **Symptom:** a copied picture's run asked to start at a keyframe began one keyframe early, and
  every chunk of the run held the chunk before it, retimed to its own place: *Flow* jumped back and
  repeated, and its store had chunks that overlapped (picture chunk 38 held 10.43 s where its chunk
  is 9.30 s; staging, 2026-10-08). Only a stream copy shows it: a decode trims to the seek exactly.
- **Cause:** `fftools/ffmpeg_demux.c` moves an input `-ss` 3/23 s (130 ms) earlier for a format
  without `AVFMT_SEEK_TO_PTS`, Matroska, AVI and MPEG-TS among them (MP4 has it), when any stream
  has B-frame delay, and the demuxer then lands on the keyframe at or before that. A seek exactly at
  a keyframe so lands on the one before it.
- **What we do:** a copied picture's run seeks midway to the next keyframe, at least 131 ms past
  its first and before the next (`chunks.PictureSeekMs` in `internal/core/chunks/job.go`;
  `nodeproto.MakeChunks.PictureSeekMs`); the muxer's cut times stay relative to the first keyframe,
  where the copy begins. The node also refuses a raw file whose listed span is not its chunk's
  (`internal/node/chunks/listed.go`), which a whole run begun early passes when chunks are as long as
  the stretches between keyframes.
- **Evidence:** **[measured]** on *Flow* with the pinned FFmpeg 8.1.2: `-ss 388.930`, a keyframe,
  wrote 378.503–388.930 as the run's first file; `-ss 391.078`, midway, wrote the grid's chunks.
  `TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes` compares a Matroska copy's packets with
  the film's, and fails without the seek. **[source]**
  [ffmpeg_demux.c](https://github.com/FFmpeg/FFmpeg/blob/release/8.0/fftools/ffmpeg_demux.c)
  (`dts_heuristic`, `seek_timestamp -= 3*AV_TIME_BASE / 23`). **[web]**
  [Kyoo PR #320](https://github.com/zoriya/Kyoo/pull/320): "`-ss` seeks to the keyframe prior the
  given time ... if the given time is a keyframe, this is UB"; Kyoo's `transcoder/src/stream.go`
  seeks midway to the next keyframe for the same reason. FFmpeg's trac refuses automated search, so
  no FFmpeg ticket was checked (2026-10-08).

### The segment muxer lists the file it was writing as finished when FFmpeg is stopped

- **Symptom:** a chunk held 23 frames of its 10 s, and the stream had a gap after it (*Flow*'s
  picture chunk 37, staging, 2026-10-08).
- **Cause:** on SIGINT, FFmpeg's graceful quit, the muxer writes its trailer, closing the file it
  was in the middle of, and adds it to `-segment_list` like any other: the list does not say a file
  was cut short. The node's watchdog stops a stalled run with SIGINT first.
- **What we do:** a raw file is made a chunk only when its listed span is its chunk's, within
  500 ms, and the run's last chunk only once FFmpeg has ended, so whether a cut or the end closed it
  is known (`internal/node/chunks/listed.go`, `maker.go`).
- **Evidence:** **[measured]** FFmpeg 8.1.2 given SIGINT two seconds into a run listed
  `00040.mp4,4.379000,10.927000`, a file whose cut was at 14.68 s.
  `TestMake_NeverMakesAChunkOfAFileNotCutAtItsBounds`. **[web]** no public report found (searched
  GitHub issues of Kyoo and Jellyfin, 2026-10-08).

### The `hls` muxer cuts by cumulative time and cannot keep a copied picture to a keyframe grid

- **What we do:** the `segment` muxer with `-segment_times` (`internal/node/chunks/arguments.go:96-104`).
- **Evidence:** `research/slow-links.md` §10 [measured].
  **[web]** [hlsenc.c](https://raw.githubusercontent.com/FFmpeg/FFmpeg/master/libavformat/hlsenc.c) (read
  2026-10-08): the cut test is `end_pts = recording_time * vs->number`, i.e. cumulative time from the start,
  with no list of cut times. Kyoo hit the same and added an `hls_times` option to its own FFmpeg fork
  ([zoriya/Kyoo#1675](https://github.com/zoriya/Kyoo/pull/1675), merged 2026-08-31); upstream has none.


### Each output file starts its fMP4 times at zero, and upstream has no flag for absolute `tfdt`

- **Cause:** `frag_discont` does not do it, and `absolute_tfdt` exists only in Kyoo's fork.
- **What we do:** the node rewrites every `tfdt` itself. `internal/node/chunks/fmp4.go:121-152`;
  `maker.go:255`.
- **Evidence:** `research/slow-links.md` §10 [measured: 3,533 frames, no gap];
  `TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes`.
  **[web]** [zoriya/Kyoo#1578](https://github.com/zoriya/Kyoo/pull/1578) (merged 2026-07-12) rewrote every
  `tfdt` as we do; [Kyoo#1675](https://github.com/zoriya/Kyoo/pull/1675) (merged 2026-08-31) then dropped it
  because "manually patching tfdt had some issues with some codecs" (E-AC-3 named) and moved to its fork's
  `absolute_tfdt` movflag.
  [movenc.c](https://raw.githubusercontent.com/FFmpeg/FFmpeg/master/libavformat/movenc.c) (read 2026-10-08):
  with `frag_discont` and no edit list, the first fragment's `tfdt` is the packet's own DTS. **Better
  workaround:** (unverified here) `-copyts` with `movflags=+frag_discont` keeps source DTS, which is how
  Jellyfin gets film-time `tfdt` ([jellyfin#18049](https://github.com/jellyfin/jellyfin/issues/18049), open:
  its catch is a negative `tfdt` when the audio starts early).


### fMP4 init segments differ between runs unless the edit list is off

- **What we do:** `movflags=+frag_keyframe+empty_moov+default_base_moof:use_editlist=0`
  (`internal/node/chunks/arguments.go:102`).
- **Evidence:** `research/slow-links.md` §10 [measured].
  **[web]** no public report of the run-to-run difference found (searched FFmpeg trac, 2026-10-08). Related:
  [FFmpeg ticket 11034](https://trac.ffmpeg.org/ticket/11034), "Stream copy with -use_editlist 0 introduces
  delay by default": new; it shows the edit list carrying each output's start offset, which plausibly is what
  varies between runs (unverified).


### The AAC encoder adds 1024 priming samples at each run's start

- **Symptom:** an 8.7 ms seam where two runs meet.
- **What we do:** a sound run starts one chunk early and drops it. `internal/node/chunks/maker.go:35-37,269-290`.
- **Evidence:** `research/slow-links.md` §10 [measured]; `TestMake_TimesASoundBySamplesAfterItsLeadIn`. [doc]
  **[web]** [FFmpeg ticket 10464](https://trac.ffmpeg.org/ticket/10464), "MP4 muxer: muxing with empty moov
  fails to signal audio encoder delay": new; with `empty_moov` (as ours) no edit list says to drop the 1024
  priming samples. [Ticket 11031](https://trac.ffmpeg.org/ticket/11031) fixed the same gap for `cmaf` only
  (checked 2026-10-08).


### An input seek of an Opus track to a keyframe decodes it without its pre-roll

- **Symptom:** watching *Bram the Wanderer* (Opus 5.1 in Matroska) on staging, the sound hiccuped
  every five minutes, at each run of chunks' start. Run 1's sound was 86.0 ms later than its
  timestamps, run 2's 135.3 ms: a 49 ms jump, and a 65 ms gap in the timestamps, where they met.
- **Cause:** a run starts on a keyframe, and `-ss` before `-i` lands at a keyframe there. Opus
  needs 80 ms of what came before it to be settled (its seek pre-roll); the demuxers record it,
  but nothing in `fftools` uses it, so a decode from there comes out 60 to 90 ms later than the
  time it is labelled with. Away from a keyframe the same seek is exact (−1 ms against a decode
  that never seeks, at 4680, 4690, 4697.111, 4700.010, 4708.777 s; +87, +60, +89, +90, +91 ms
  at 4694.273, 4694.300, 4704.491, 4714.710, 4724.929 s, each on or just after a keyframe).
- **What we do:** a sound's decode starts two seconds early and trims to its run's start after
  decoding (`atrim`, then `asetpts=PTS-STARTPTS`): `internal/node/chunks/arguments.go`
  (`soundSeekEarlyMs`). Every start measured −1 ms, and a run holds exactly the samples its span
  has. `TestArguments_StartASoundAChunkEarlyAndCutItAtTheBounds`.
- **Evidence:** measured 2026-10-11 with the node's own command on the staging film. **[web]**
  [FFmpeg's matroskadec.c](https://www.ffmpeg.org/doxygen/trunk/matroskadec_8c_source.html) sets
  `seek_preroll` from the track; others met the same, e.g.
  [onixlabs-media-player#58](https://github.com/onix-labs/onixlabs-media-player/pull/58), "audio
  running ahead of video after seeking" (read 2026-10-11).


### Forced keyframes: x264 needs `-sc_threshold 0`, and hardware encoders need "forced IDR", spelt differently per encoder

- **Cause:** NVENC ignores `-force_key_frames` without `-forced-idr 1`. QSV's option is `-forced_idr 1`.
  Jellyfin does not trust `-force_key_frames` on QSV.
- **What we do:** `internal/node/prepare/encoder.go:37-79`.
- **Evidence:** `research/slow-links.md` §4; `research/playback-in-practice.md` §3 [web/source]; D190; commit
  An earlier commit. Unverified on our QSV hardware.
  **[web]** [FFmpeg codecs docs](https://ffmpeg.org/ffmpeg-codecs.html) list `forced-idr` for libx264 and
  libx265 and `forced_idr` among the QSV common options. [FFmpeg ticket
  8857](https://trac.ffmpeg.org/ticket/8857), "h264_qsv force_key_frames incorret" (every frame became a
  keyframe): fixed in 2021. Jellyfin's
  [EncodingHelper.cs](https://raw.githubusercontent.com/jellyfin/jellyfin/master/MediaBrowser.Controller/MediaEncoding/EncodingHelper.cs)
  (`GetHlsVideoKeyFrameArguments`, read 2026-10-08) says "Unable to force key frames using these encoders" for
  QSV, NVENC, AMF, RKMPP and SVT-AV1 and sets `-g` and `-keyint_min` to the segment length instead. **Better
  workaround:** a fixed GOP equal to the chunk length, as Jellyfin does, does not depend on each encoder
  honouring a forced keyframe.


### `-hls_init_time` does nothing for copied video

- **Symptom:** the first segment is as long as the source's first GOP (*Flow*: 10.43 s with
  `-hls_init_time 1`).
- **What we do:** ladder from the start (D257, D260) and the keyframe grid from the file's index (D259).
- **Evidence:** `research/cold-start.md` §4.4-§5 [measured 2026-10-07]. **Unsure:** arguably expected.
  **[web]** [FFmpeg formats docs](https://ffmpeg.org/ffmpeg-formats.html): "Segment will be cut on the next
  key frame after this time has passed", so with copied video the first cut waits for the source's second
  keyframe. This settles the **Unsure** above: documented behaviour. Related misbehaviour: [FFmpeg ticket
  10098](https://trac.ffmpeg.org/ticket/10098) ("ignores -hls_time if -hls_init_time is set", new) and [ticket
  9484](https://trac.ffmpeg.org/ticket/9484) (reopened) (checked 2026-10-08).


### Chromaprint refuses a 5.1 input

- **Symptom:** `[af#0:0] Task finished with error: Invalid argument` / `[out#0/chromaprint] Nothing was
  written into output file`.
- **What we do:** downmix with `-ac 2` first (an `assumptions/` test, D111).
- **Evidence:** D115 [measured 2026-09-20].
  **[web]**
  [libavformat/chromaprint.c](https://raw.githubusercontent.com/FFmpeg/FFmpeg/master/libavformat/chromaprint.c)
  (read 2026-10-08): FFmpeg's `chromaprint` muxer returns EINVAL with "Only up to 2 channels are supported"
  when the input has more than two channels; the refusal is the muxer's, before Chromaprint is called. No
  tracker issue found (searched FFmpeg trac and acoustid/chromaprint issues, 2026-10-08).


### FFmpeg's AC-3/E-AC-3 decoder applies DRC by default (`drc_scale = 1`)

- **What we do:** DRC is a per-device preference (D83). Listed in `assumptions/`.
- **Evidence:** working-notes.md; Jellyfin #10148 [web].
  **[web]** [FFmpeg codecs docs, AC-3 decoder](https://ffmpeg.org/ffmpeg-codecs.html): `drc_scale` default 1,
  0 disables DRC; documented, not a bug. [jellyfin#10148](https://github.com/jellyfin/jellyfin/issues/10148),
  "Can't set Dynamic Range Compression (DRC) decoding preference": closed 2023-08-24 (checked 2026-10-08).


### `-c copy` remux rewrites every Matroska cluster, and mkvpropedit moves grown elements to the end

- **Symptom:** the planned head+tail sampling hash broke on every tag edit.
- **What we do:** file identity is a hash of the media payload (D179).
- **Evidence:** working-notes.md [measured].
  **[web]** no public report found; the [mkvpropedit manual](https://mkvtoolnix.download/doc/mkvpropedit.html)
  does not describe where a grown element is written (searched the manual, FFmpeg trac and GitHub,
  2026-10-08).


### FFmpeg reads an input name as a protocol, and reads stdin by default

- **What we do:** `-nostdin`, `-protocol_whitelist file -i file:<abs>`.
  `internal/node/execx/execx.go:46-67`.
- **Evidence:** cut-a §7. [doc] This is security hardening.
  **[web]** [FFmpeg protocols docs](https://ffmpeg.org/ffmpeg-protocols.html): "All protocols are allowed by
  default" (`protocol_whitelist`), and the `file:` prefix forces the file protocol; [ffmpeg
  docs](https://ffmpeg.org/ffmpeg.html): stdin interaction is "on by default unless standard input is used as
  an input", disabled by `-nostdin`. Both documented (checked 2026-10-08).


### ffprobe's output quirks

- **Cause:** numbers come as strings, and "N/A" appears. A Matroska stream's duration is only a `DURATION`
  tag. The AAC `profile` is "" when the container does not say. Stream 0 may be cover art.
- **What we do:** `internal/node/probe/parse.go:175-205`; `internal/core/decide/classify.go:32-34`; a
  disposition-1024 filter (`internal/store/queries/read_through.sql:15`,
  `internal/core/library/streams.go:16-18`).
- **Evidence:** code comments only. [doc] Low priority, and could be merged into one entry.
  **[web]** no tracker issue found for the Matroska `DURATION` tag or the empty AAC profile (searched FFmpeg
  trac, 2026-10-08); related: [FFmpeg ticket 11584](https://trac.ffmpeg.org/ticket/11584), "ffprobe returns
  "n/a" for bitrate in opus audio file": new.


### A Matroska file's cues may list fewer keyframes than the picture has

- **What we do:** trust each cue, and read through a file without cues (D259 §2-3;
  `internal/node/container/doc.go:3-7`).
- **Evidence:** D259. [doc] This is a muxer's choice.
  **[web]** [RFC 9559 §22.1](https://www.rfc-editor.org/rfc/rfc9559#section-22.1): "each keyframe SHOULD be
  referenced by a CuePoint", a recommendation, not a requirement, so a conforming file may index fewer. No
  tracker issue found for a specific muxer (searched FFmpeg trac and GitHub, 2026-10-08).

---

---

## Linux networking: netem, tc, loopback, BBR, iproute2


### netem's jitter reorders packets unless netem also has a `rate`; correlation does not prevent it

- **Symptom:** `delay 80ms 30ms 25%` reordered 47.4% of a 500 pkt/s stream and 22.6% of 50 pkt/s voice. BBR
  showed 5212 mostly spurious retransmissions.
- **Cause:** [source] each packet gets its own `time_to_send` in a time-sorted tfifo, and "correlation" only
  narrows the spread. A `pfifo` child does not help either, because it receives packets after the tfifo.
  **Contrary to the docs:** the man page's "replace tfifo with a FIFO" advice describes older kernels.
- **What we do:** each profile's netem stage carries `rate 1gbit` only to keep packets in order (RFC 8868's
  NR-BPDV). The bottleneck is a `tbf` child. D263 §1, correcting D262 §1.
- **Evidence:** `research/network-profiles.md` :38-51, §3.1, §3.3, §8 item 2 [measured, kernel 6.12,
  2026-10-07].
  **[web]** [tc-netem(8)](https://man7.org/linux/man-pages/man8/tc-netem.8.html): "Packets will also get
  reordered if jitter is large enough" because tfifo "keeps packets in order by time to send"; it still
  advises a `pfifo` child to avoid it, and calls delay correlation "not a true statistical correlation, but an
  approximation"; [Stack Overflow
  36994367](https://stackoverflow.com/questions/36994367/how-to-emulate-jitter-without-packet-reordering-using-tc-and-netem)
  reports the man page's `pfifo` child "doesn't work! Packets still get reordered", and one answer on Linux
  4.17 found reordering whenever the jitter exceeds the delay; [sch_netem.c
  (mainline)](https://github.com/torvalds/linux/blob/master/net/sched/sch_netem.c) `netem_enqueue`: only when
  `q->rate` is set does it take the queue's last `time_to_send` as the reference point ("Last packet in queue
  is reference point (now)"), so with `rate` no packet is scheduled before an earlier one, which confirms our
  cause. The only other workaround found (same Stack Overflow page) changes a fixed delay from a loop, which
  reorders too when the delay falls.


### netem's `limit` counts every packet it holds, including packets only being delayed (and on `lo`, both directions)

- **Symptom:** `limit 100`, meant as about a quarter-second queue, gave CUBIC a 472 ms smoothed RTT with no
  loss. The default limit of 1000 packets is about 2.4 s at 5 Mbit/s.
- **What we do:** netem `limit 100000`, with the queue bounded by a `tbf … latency` child. With `rate`, netem
  also folds propagation delay into the queue, so delay goes in netem and the bottleneck in tbf.
- **Evidence:** `research/network-profiles.md` :51-53, §3.1-§3.2, §8 item 3; `research/voice-bad-links.md`
  (`q_netem.c` `.limit = 1000`); an earlier commit. D263 §1.
  **[web]** [tc-netem(8)](https://man7.org/linux/man-pages/man8/tc-netem.8.html): `limit` "Limits the maximum
  number of packets the qdisc may hold when doing delay";
  [sch_netem.c](https://github.com/torvalds/linux/blob/master/net/sched/sch_netem.c) drops when `sch->q.qlen
  >= sch->limit`, delayed packets included; [Stack Overflow
  18792347](https://stackoverflow.com/questions/18792347/what-does-option-limit-in-tc-netem-mean-and-do)
  quotes Stephen Hemminger: "The value 1000 is low, you want about 50% more than the max packet rate * delay,
  unless you are trying to emulate a router with a small queue", and an answerer uses `limit 100000`, as we
  do.


### The loopback's 64 KB MTU and GSO make netem and tbf see 64 KB packets

- **Symptom:** "a TCP packet of the film held a voice packet behind it for a tenth of a second" at 5 Mbit/s.
  Voice was 1.1 s late under *congested*, and much of that was the emulation's fault.
- **What we do:** `./dev` runs `ip link set dev lo mtu 1500 gso_max_size 1500` as root when the container
  starts (`dev:206-217`, `LOOPBACK_MTU`). The acceptance refuses a loopback that has not been set this way.
- **Evidence:** `research/network-profiles.md` §3.5; `research/voice-bad-links.md` §6; D262 §1; commit
  An earlier commit [measured 2026-10-07].
  **[web]** [loopback.c](https://github.com/torvalds/linux/blob/master/drivers/net/loopback.c):
  `loopback_setup` gives `lo` an MTU of `64 * 1024`, software GSO and `GSO_MAX_SIZE`;
  [sch_tbf.c](https://github.com/torvalds/linux/blob/master/net/sched/sch_tbf.c) segments a GSO packet only
  when it is larger than the bucket (`qdisc_pkt_len(skb) > q->max_size`), so a 64 KB burst passes a 64 KB
  packet whole; [sch_netem.c](https://github.com/torvalds/linux/blob/master/net/sched/sch_netem.c) segments
  GSO packets only when it corrupts. No public report of the effect on voice found (searched Stack Exchange,
  2026-10-08).


### One qdisc on `lo` is one shared link for both directions and both browsers

- **Symptom:** 5 Mbit/s is about 2.5 Mbit/s for each viewer, an RTT counts the queue twice, and TURN-relayed
  voice crosses the link twice.
- **What we do:** profiles are sized for both clients, with `iceTransportPolicy: "all"`. Per-browser network
  namespaces are deferred (D263 §4).
- **Evidence:** `research/network-profiles.md` §3.4 [measured]. [doc] This is inherent to loopback, not a
  bug. **Unsure** whether it belongs.
  **[web]** [Stack Overflow
  21614595](https://stackoverflow.com/questions/21614595/netem-and-tcpdump-loopback-interface-differing-ingress-and-egress):
  100 ms of netem on `lo` delays a ping to 127.0.0.1 by 200 ms, the queue counted twice as here;
  [tc-netem(8)](https://man7.org/linux/man-pages/man8/tc-netem.8.html) recommends netem "on the ingress of the
  receiver host" for realistic TCP results, which per-browser namespaces would allow.


### `tbf`'s `burst` must be at least rate/HZ, or the rate is never reached

- **What we do:** a 16-64 KB burst. **Evidence:** `research/network-profiles.md` :190-191. [doc] Low priority.
- **Evidence:** **[web]** [tc-tbf(8)](https://man7.org/linux/man-pages/man8/tc-tbf.8.html): "The minimum
  buffer size can be calculated by dividing the rate by HZ"; "For 10mbit/s on Intel, you need at least 10kbyte
  buffer if you want to reach your configured rate!" Documented, not a bug.


### Mainline Linux has only BBRv1, and `tcp_bbr` must already be loaded for an unprivileged process

- **Symptom:** `setsockopt(TCP_CONGESTION, "bbr")` returns ENOENT as a normal user, while root autoloads the
  module. Falling back to CUBIC matters: at 3% random loss CUBIC carried 1.0 of 20 Mbit/s, against BBR's
  17.7.
- **Cause:** [source] `tcp_ca_find_autoload` needs CAP_NET_ADMIN. BBRv3 is still out of tree (announced for
  mainline in 2023).
- **What we do:** listeners ask for BBR, accepted connections inherit it, and the server logs once and keeps
  the default where BBR is missing. The host loads `tcp_bbr`.
  `internal/platform/pacing/listen.go:10-21`; `internal/platform/pacing/socket_linux.go:43-55`;
  `cmd/orpheum/https.go:74-80`; `deploy/staging/README.md:34-37`.
- **Evidence:** D263 §3; `research/network-profiles.md` §5-§6 [measured + source]; an earlier commit;
  `TestListenConfig_AcceptsConnectionsThatSendWithBBRWhereTheKernelHasIt`.
  **[web]** [tcp_cong.c](https://github.com/torvalds/linux/blob/master/net/ipv4/tcp_cong.c):
  `tcp_ca_find_autoload` calls `request_module` only `if (!ca && capable(CAP_NET_ADMIN))`, else ENOENT;
  [tcp_bbr.c](https://github.com/torvalds/linux/blob/master/net/ipv4/tcp_bbr.c) registers with
  `TCP_CONG_NON_RESTRICTED`, so once loaded it is usable unprivileged even though
  [tcp(7)](https://man7.org/linux/man-pages/man7/tcp.7.html) says unprivileged processes are limited to
  `tcp_allowed_congestion_control` (default "reno" plus the default algorithm) [measured here: an unprivileged
  `setsockopt("bbr")` succeeds with the module loaded]; [google/bbr v3
  README](https://github.com/google/bbr/blob/v3/README.md): BBRv3 is still a separate kernel branch "intended
  to enable research collaboration and wider testing".


### iproute2's `ip` drops file capabilities, so `setcap` on `ip` is useless (`tc` keeps them)

- **What we do:** `setcap cap_net_admin+ep` on `tc` only (`deploy/Containerfile:109-115`). The loopback's
  MTU is set with `ip` as root via `docker exec --user 0` when the container starts (`dev:210`).
- **Evidence:** the `dev:210` comment; an earlier commit. **Cause unverified in the repo.** From general
  knowledge, `ip` calls `drop_cap()` (lib/utils.c) for non-root users unless CAP_NET_ADMIN is inheritable.
  **[web]** confirms the cause: [iproute2
  lib/utils.c](https://github.com/iproute2/iproute2/blob/main/lib/utils.c) `drop_cap()` clears every
  capability for a non-root user unless CAP_NET_ADMIN is in the process's inheritable set, and `ip/ip.c` calls
  it except for `ip vrf exec`; `tc/tc.c` never calls it. Added by
  [ba2fc55b](https://github.com/iproute2/iproute2/commit/ba2fc55b99f8363c80ce36681bc1ec97690b66f5) (2018) and
  relaxed by [9b13cc98](https://github.com/iproute2/iproute2/commit/9b13cc98f5952f62b825461727c8170d37a4037d)
  ("do not drop capabilities if net_admin=i is set", for callers using ambient capabilities). **Better
  workaround:** a caller holding CAP_NET_ADMIN as an inheritable and ambient capability (e.g. `setpriv
  --inh-caps +net_admin --ambient-caps +net_admin ip ...`) keeps it through `ip` without root; root via
  `docker exec --user 0` stays simpler for us.


### netem orphans delayed packets (`skb_orphan_partial`), so TCP Small Queues does not hold the sender back

- **Evidence:** `research/network-profiles.md` §3.1 [source]. This is context; it forced no workaround.
  **Probably exclude.**
  **[web]** [sch_netem.c](https://github.com/torvalds/linux/blob/master/net/sched/sch_netem.c): "If a delay is
  expected, orphan the skb" (`if (q->latency || q->jitter || q->rate) skb_orphan_partial(skb)`);
  [tc-netem(8)](https://man7.org/linux/man-pages/man8/tc-netem.8.html) LIMITATIONS: "Due to mechanisms like
  TSQ (TCP Small Queues), for TCP performance test results to be realistic netem must be placed on the ingress
  of the receiver host."

---

---

## Linux kernel and OS (other)


### A process in uninterruptible disk wait ignores even SIGKILL

- **What we do:** a run that outlives SIGKILL is "left running", listed as stuck, and its slot is freed.
  `internal/node/execx/execx.go:41-44,357-377`; `stuck.go:11-12`.
- **Evidence:** D233 §10. [doc]
  **[web]** [LWN, "TASK_KILLABLE" (2008)](https://lwn.net/Articles/288056/): a `TASK_UNINTERRUPTIBLE` sleeper
  "will wait forever and there is usually nothing that anybody can do about it short of rebooting";
  `TASK_KILLABLE` (merged in 2.6.25) lets fatal signals through for code converted to it, the NFS client among
  the first, so whether SIGKILL works depends on the wait; a block-device read wait usually stays unkillable.


### Pdeathsig fires when the starting *thread* ends, not the process

- **What we do:** the node never ends a thread of its own. `internal/node/execx/process_linux.go:7-13`.
- **Evidence:** the comment; `prctl(2)`. [doc] This is latent and was never hit.
  **[web]** [PR_SET_PDEATHSIG(2const)](https://man7.org/linux/man-pages/man2/PR_SET_PDEATHSIG.2const.html)
  CAVEATS: the "parent" is "the thread that created this process", so the signal comes when that thread
  terminates; [golang/go#27505](https://github.com/golang/go/issues/27505) (closed 2026-08-12) made Go's
  `SysProcAttr.Pdeathsig` doc say "the signal is sent on thread termination, which may happen before process
  termination"; [golang/go#14592](https://github.com/golang/go/issues/14592) ("let idle OS threads exit",
  open) is why Go threads normally live on, though a goroutine that exits while `runtime.LockOSThread`-ed ends
  its thread.


### Opening a named pipe read-only blocks until a writer appears

- **Symptom:** a pipe placed where a media file was held a byte request and its goroutine forever.
- **What we do:** open with `O_NONBLOCK` before the regular-file check.
  `internal/node/confine/mediaroot.go:81-88`.
- **Evidence:** commit "fix(confine): opening a file under a media root never waits for a pipe's writer" (2026-09-29). [doc] POSIX behaviour.
  **[web]** [fifo(7)](https://man7.org/linux/man-pages/man7/fifo.7.html): "Normally, opening the FIFO blocks
  until the other end is opened also"; with O_NONBLOCK, "opening for read-only succeeds even if no one has
  opened on the write side". Documented, as we do.


### Whether Landlock is available depends on the kernel's boot configuration, and the errno says why

- **Symptom:** `landlock_create_ruleset(...) → -1, errno 95` in an ordinary container. D112 first blamed
  Docker, wrongly. Proxmox VE leaves Landlock out of `CONFIG_LSM`, and Synology's kernels predate it (ENOSYS).
- **What we do:** the errno is mapped to a cause and a remedy. `internal/node/confine/landlock_linux.go:95-118`;
  `deploy/staging/README.md:99-101`.
- **Evidence:** D112; `research/audit-2026-09-23.md` §4.1 [measured]; VQ9.
  **[web]**
  [landlock_create_ruleset(2)](https://man7.org/linux/man-pages/man2/landlock_create_ruleset.2.html):
  EOPNOTSUPP means "Landlock is supported by the kernel but disabled at boot time";
  [landlock(7)](https://man7.org/linux/man-pages/man7/landlock.7.html): the `lsm=` boot parameter "must
  contain the string landlock", falling back to `CONFIG_LSM`; Proxmox's kernel build
  [config-common.opts](https://git.proxmox.com/?p=pve-kernel.git;a=blob_plain;f=debian/rules.d/config-common.opts;hb=HEAD)
  sets `CONFIG_LSM lockdown,yama,integrity,apparmor`, without landlock, confirming our Proxmox cause (remedy:
  add `landlock` to `lsm=` on the kernel command line).


### x32 syscalls share AUDIT_ARCH_X86_64, so a seccomp denylist can be bypassed

- **What we do:** refuse every number at or above `x32Bit`. `internal/node/confine/arch_linux_amd64.go:13-23`.
- **Evidence:** commit "feat(node): every FFmpeg run is confined by Landlock and seccomp through a re-exec shim (cut-a §7)". [doc]
  **[web]** [seccomp(2)](https://man7.org/linux/man-pages/man2/seccomp.2.html): "The x86-64 ABI and the x32
  ABI both use AUDIT_ARCH_X86_64"; a policy "must either deny all syscalls with __X32_SYSCALL_BIT or it must
  recognize syscalls with and without" it, or it "can be bypassed"; before Linux 5.4 numbers 512-547 were also
  accepted with confused semantics. Our refusal of every number at or above the x32 bit is the man page's
  first option.


### The kernel's `rotational` flag is wrong behind a USB bridge

- **Symptom:** the owner's USB SSD on devnas read as spinning.
- **What we do:** prefer udev's `ID_ATA_ROTATION_RATE_RPM`, and treat a "spinning" disk behind USB as unknown.
  `internal/node/hardware/storage.go:210-230`.
- **Evidence:** D240 §5; [owner] VQ17, 2026-10-03.
  **[web]** [sd.c](https://github.com/torvalds/linux/blob/master/drivers/scsi/sd.c)
  `sd_read_block_characteristics` clears the rotational flag only when VPD page B1 reports a rotation rate of
  1 (non-rotating), so a disk is "rotational" by default; [usb-storage
  scsiglue.c](https://github.com/torvalds/linux/blob/master/drivers/usb/storage/scsiglue.c) sets
  `skip_vpd_pages` ("Some devices don't handle VPD pages correctly"), so behind a usb-storage bridge the page
  is never read; [Ask Ubuntu
  911007](https://askubuntu.com/questions/911007/ssd-features-rotational-discard-not-detected-properly-via-usb)
  reports the same for an SSD over USB. The common fix others use is a udev rule writing `queue/rotational`,
  which changes the kernel's flag, not our reading of it.


### btrfs and ZFS mounts do not name a block device

- **What we do:** follow btrfs through `/sys/fs/btrfs` and ZFS through udev. `./dev` bind-mounts the host's
  `/run/udev`. `internal/node/hardware/storage.go:261-263,339-340`.
- **Evidence:** D240 §5. [doc]
  **[web]** [Unix & Linux
  137344](https://unix.stackexchange.com/questions/137344/get-device-with-major-number-0-btrfs-subvolume): a
  btrfs subvolume's device number has major 0, an anonymous device that names no block device; no
  authoritative kernel document found for ZFS (searched Stack Exchange, 2026-10-08), where the mount source is
  a dataset name.


### A UDP socket bound to every address replies from the routing table's choice of source

- **What we do:** TURN listens on the tunnel's address `10.77.0.2`. `internal/core/turn/doc.go:28-29`;
  `deploy/vps/README.md:121-124`.
- **Evidence:** VQ19 (the owner's VPS). [doc]
  **[web]** [IP_PKTINFO(2const)](https://man7.org/linux/man-pages/man2/IP_PKTINFO.2const.html): a datagram
  socket can receive the packet's destination address and set the source on `sendmsg`, which is the standard
  way one wildcard socket replies from the address it was reached on; no tracker entry, it is how routing
  works. **Better workaround:** IP_PKTINFO/IPV6_PKTINFO would keep a wildcard listener correct on a
  multi-homed host, if the TURN library supported it (not checked); binding the tunnel address, as we do, is
  simpler.


### Unix socket paths are limited to 108 bytes on Linux and 104 on macOS

- **What we do:** `maxSocketPathBytes = 103` (`internal/platform/unixsocket/listen.go:14-16`), plus
  `--run-dir`. **Evidence:** D242 §8. [doc]
- **Evidence:** **[web]** [unix(7)](https://man7.org/linux/man-pages/man7/unix.7.html): "On Linux, sun_path is
  108 bytes in size", "some implementations have sun_path as short as 92 bytes", and a 108-byte path loses its
  null terminator when read back (BUGS); Apple's [xnu
  bsd/sys/un.h](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/un.h) declares
  `sun_path[104]`.


### inotify does not work over SMB/NFS and runs out of watches silently

- **What we do:** periodic scans correct missed events (D27, D60). **Evidence:** working-notes.md [web].
- **Evidence:** **[web]** [inotify(7)](https://man7.org/linux/man-pages/man7/inotify.7.html): it "does not
  catch remote events that occur on network filesystems. (Applications must fall back to polling"; past
  `max_queued_events` events "are dropped, but an IN_Q_OVERFLOW event is always generated";
  [inotify_add_watch(2)](https://man7.org/linux/man-pages/man2/inotify_add_watch.2.html) returns ENOSPC when
  `max_user_watches` is reached. **Contradicts:** running out of watches is not silent at the API: adding a
  watch fails with ENOSPC ("no space left on device", easily misread) and a full queue reports IN_Q_OVERFLOW;
  it is silent only where a library drops those errors.


### The NFS client's default timeout is 60 s, retransmitted twice

- **What we do:** a 3-minute prepare stall window. `internal/node/prepare/prepare.go:31-35`.
  **Evidence:** the comment. [doc] Low priority.
- **Evidence:** **[web]** [nfs(5)](https://man7.org/linux/man-pages/man5/nfs.5.html): "For NFS over TCP the
  default timeo value is 600 (60 seconds)", with linear backoff (each retransmission adds `timeo`, up to 600
  s); without `retrans` "the NFS client tries ... each TCP request twice", then logs "server not responding";
  and the default `hard` mount retries "indefinitely", so a read on a dead server never fails on its own, it
  only stalls (`soft` returns EIO, `softerr` ETIMEDOUT).

---

---

## Docker, hosts and the network path


### A container sharing another's network loses it when that container restarts alone

- **Symptom:** `./staging status` shows the core unhealthy after the tunnel's container restarted on its own.
- **What we do:** the tunnel never exits on failure and retries inside the same namespace
  (`deploy/staging/tunnel.sh:38-42`), and `./staging` recreates both containers. The behaviour is documented
  at `deploy/staging/README.md:94-98`.
- **Evidence:** the README. No date is given.
  **[web]** [docker/compose#10263](https://github.com/docker/compose/issues/10263), "restart behavior when
  using network_mode: service": closed by [docker/compose#10273](https://github.com/docker/compose/pull/10273)
  (merged 2023-02-14), which makes `docker compose restart` restart the services that share the namespace too
  (`--no-deps` keeps the old behaviour); a restart by the daemon's restart policy still leaves them cut off.
  [moby/moby#11777](https://github.com/moby/moby/issues/11777), "Daemon restart not starting containers with
  shared network in the correct order (--network=container:...)": open since 2015 (checked 2026-10-08).


### Killing the `docker exec` client leaves the process running inside the container

- **Symptom:** a second `./dev make dev` ran two dev loops that fought over ports.
- **What we do:** `./dev --stop` removes the container.
- **Evidence:** an agent's working note (2026-10-02).
  **[web]** [moby/moby#9098](https://github.com/moby/moby/issues/9098), "Kill `docker exec` command will not
  terminate the spawned process": open since 2014, still reported in 2026; the exec kill API it needs
  ([moby/moby#35703](https://github.com/moby/moby/issues/35703),
  [moby/moby#41548](https://github.com/moby/moby/pull/41548)) is open and unmerged (checked 2026-10-08).
  Others tag the command (`exec -a <id>`) and `pkill` it inside the container before killing the client, or
  wrap exec to forward signals through the API.


### The dev container had no `tc` and no right to shape its network, and CI's container lacks `NET_ADMIN`

- **What we do:** iproute2 in the image, `--cap-add NET_ADMIN` (`dev:130-131`), and `NET_ADMIN` for CI.
- **Evidence:** D252, D261 §2; `research/audit-2026-10-06.md` Q2. This is Docker's capability model.
  **Unsure:** it may be just configuration.
  **[web]** [Docker's run reference](https://docs.docker.com/engine/containers/run/) lists `NET_ADMIN` among
  the capabilities not granted by default, which may be added; so this is documented configuration, not a bug
  (checked 2026-10-08).


### Ubuntu's AppArmor profile for `wg-quick` refuses PostUp's sysctl and nftables, even as root

- **Symptom:** `permission denied` from PostUp.
- **What we do:** a sysctl.d file and a separate `orpheum-forwards.service`. `deploy/vps/vps-wg0.conf:8-10`;
  `deploy/vps/README.md:59-61`.
- **Evidence:** [owner] the VPS, 2026-10-06.
  **[web]** The [upstream `wg-quick`
  profile](https://gitlab.com/apparmor/apparmor/-/raw/master/profiles/apparmor.d/wg-quick) (added 2025-02-07)
  lets its `sysctl` child write only `net/ipv4/conf/all/src_valid_mark` and runs `nft` and `ip` in narrow
  child profiles, which explains the denials; it ends with `include if exists <local/wg-quick>` for site
  additions. Related reports: [Launchpad
  #2133632](https://bugs.launchpad.net/ubuntu/+source/apparmor/+bug/2133632), "wg-quick segfaults when running
  in LXC -- apparmor" (Confirmed; `aa-complain wg-quick` works around it), and [Launchpad
  #2127851](https://bugs.launchpad.net/ubuntu/+source/apparmor/+bug/2127851), the same profile denying `stat`
  on 25.10 (Confirmed). No report about PostUp itself found (searched Launchpad, 2026-10-08). A rule in
  `/etc/apparmor.d/local/wg-quick` is the other route; ours keeps the forwarding out of the profile entirely.


### ufw silently drops forwarded packets

- **What we do:** `ufw route allow` rules. `deploy/vps/README.md:63-79`. [doc]
- **Evidence:** [doc] **[web]** [ufw(8)](https://manpages.ubuntu.com/manpages/noble/en/man8/ufw.8.html):
  routed traffic needs `route` rules (netfilter FORWARD chain) and IP forwarding enabled separately, and ufw's
  own sysctl settings "may be overridden" (checked 2026-10-08).


### Docker's storage is invisible to `du` and grows without bound

- **Symptom:** the disk reached 97-99% several times; the Go build cache was 36 GB.
- **What we do:** prune (no workaround in the repo). **Evidence:** an agent's working note. **Unsure:** this is
  resource behaviour rather than a quirk.
- **Evidence:** **[web]** Go's build cache trims only entries unused for 5 days and has no size cap:
  [golang/go#81830](https://github.com/golang/go/issues/81830), "cmd/go: build cache management" (open,
  NeedsDecision), collects reports of caches of 100 GB and more;
  [golang/go#69879](https://github.com/golang/go/issues/69879), a configurable cutoff, was closed. Docker's
  [build cache garbage collection](https://docs.docker.com/build/cache/garbage-collection/) is on by default
  with size limits (checked 2026-10-08); images, volumes and stopped containers are not collected.


### DS-Lite needs a smaller WireGuard MTU

- **Symptom:** "Large pages or videos stall from outside, small ones load."
- **What we do:** a tunnel MTU of 1380 over IPv4 and 1420 over IPv6, with 1280 as the fallback.
  `deploy/staging/README.md:55-56,93-94`; `internal/deployment/templates/values.toml:26-28`, `internal/deployment/values.go` (`TunnelMTU`); `deploy/staging/tunnel.sh:17`.
- **Evidence:** an agent's working note; D243 §7. Whether 1380 was measured is unverified.
  **[web]** [RFC 6333 §5.3](https://www.rfc-editor.org/rfc/rfc6333.txt): DS-Lite's IPv4-in-IPv6 tunnel costs
  40 bytes and path MTU discovery is "not a reliable method" for it. WireGuard's overhead is 60 bytes over
  IPv4 and 80 over IPv6 ([Donenfeld, wireguard list,
  2017](https://lists.zx2c4.com/pipermail/wireguard/2017-December/002201.html)), so over DS-Lite IPv4 on a
  1500-byte link the most is 1500-40-60 = 1400; 1380 leaves 20 bytes spare, and 1420 over IPv6 is the standard
  figure.


### DS-Lite: no public IPv4 and a changing IPv6 prefix

- **What we do:** a VPS, which the core dials out to over WireGuard (D243 §7).
- **Evidence:** an agent's working note; `research/direct-reachability.md`. This is an ISP limitation.
  **[web]** [RFC 6333](https://www.rfc-editor.org/rfc/rfc6333.txt) defines DS-Lite as IPv4 carried to the
  ISP's AFTR and NATed there, so the subscriber holds no public IPv4 by design (checked 2026-10-08).

### A host that forwards IPv6 ignores the router's announcements, and its default route, unless `accept_ra` is 2

- **Symptom:** the machine has a public IPv6 address but `ip -6 route show default` prints nothing;
  anything over IPv6 fails with `network is unreachable`, a DNS server given as an IPv6 address
  included, so even names do not resolve.
- **Cause:** Docker sets `net.ipv6.conf.all.forwarding = 1` for a network with IPv6, as the
  release's `outside` network has. With forwarding on, the kernel takes router advertisements only
  where the interface's `accept_ra` is 2, and the address can still come by DHCPv6, so the machine
  has an address and no route.
- **What we do:** the install script names the case and its fix (`ipv6Report` in
  `internal/deployment/templates/install.sh`, `test_an_ipv6_address_with_no_route_out_while_forwarding_is_named_with_its_fix`);
  `docs/users/setup-problems.md` says it for households.
- **Evidence:** **[owner]** nas.example's Proxmox container, 2026-10-19: `forwarding = 1`,
  `accept_ra = 1`, a `/128` address and no default route; `accept_ra = 2` brought `default via
  fe80::… proto ra`. **[doc]** the kernel's `ip-sysctl` documentation: `accept_ra` 1 accepts
  advertisements "if forwarding is disabled", 2 "overrule forwarding behaviour".

### A Proxmox container has no IPv6 until its `net0` is given it

- **Symptom:** inside the container `ip -6 addr show scope global` lists only Docker's own
  addresses; the house's other machines have IPv6.
- **Cause:** a container's `net0` takes IPv6 only when `ip6` is set (`auto` for SLAAC, `dhcp`, or a
  static address); left unset, it has none.
- **What we do:** the install script says "a public IPv6 address: no" and how to set it;
  `docs/users/setup-problems.md`.
- **Evidence:** **[owner]** nas.example, CT 102, 2026-10-19. **[doc]** `pct` manual, `--net[n]`'s `ip6`.

### Taking an LXC container's interface down and up loses its IPv4 default route

- **Symptom:** after `ip link set eth0 down && ip link set eth0 up` in the container, IPv6's route
  came back (from the next router advertisement) but IPv4 had none: `network is unreachable` to
  any IPv4 address.
- **Cause:** the kernel drops a link's routes when it goes down, and a static route is set again
  only by what configured it, at the container's start; nothing re-adds it on link up.
- **What we do:** `docs/users/setup-problems.md` says to restart the container instead.
- **Evidence:** **[owner]** nas.example, 2026-10-19; `pct reboot` restored both routes.

### GitHub's image registry, `ghcr.io`, has no IPv6 address

- **Symptom:** a pull fails with `dial tcp 140.82.…:443: connect: network is unreachable` on a
  machine whose IPv6 works and whose IPv4 does not.
- **Cause:** `ghcr.io` and `github.com` publish no AAAA record; only the blob host,
  `pkg-containers.githubusercontent.com`, has IPv6. An install or update needs IPv4.
- **What we do:** `docs/users/setup-problems.md`.
- **Evidence:** **[measured]** `getent ahostsv6` here, 2026-10-19: none for `ghcr.io` or
  `github.com`, `2606:50c0:8000::154` for the blob host. **[owner]** nas.example, 2026-10-19.

### A travel eSIM's data leaves through its operator's gateway, over IPv4 alone

- **Symptom:** a phone on mobile data offers one ICE candidate, a server-reflexive IPv4 address
  in another country, and no IPv6; it reaches no server behind DS-Lite without a relay.
- **Cause:** unverified beyond one eSIM (bcengi): such eSIMs route their data through the
  operator's own network (home routing), behind a shared IPv4 address, without IPv6. A VPN does
  the same.
- **What we do:** nothing can: without a relay the pair cannot meet (D314 §4), and the failure is
  counted; `docs/users/setup-problems.md` tells a household what to try.
- **Evidence:** **[owner]** 2026-10-19: candidates `srflx 11.22.55.141` alone, against nas.example's
  `srflx 11.22.33.144` (DS-Lite's AFTR), every pair failed.

---

---

## SQLite, sqlc and Go's `database/sql`


### sqlc miscounts a multibyte character in a query file and reports an unrelated syntax error

- **Symptom:** `edited query syntax is invalid: extraneous input ')'` when a comment holds `§`.
- **What we do:** query files are ASCII-only, enforced by `scripts/check_source.py:20-22,57,101-112`.
- **Evidence:** an agent's working note; `docs/contributors/standards/sql.md:95-97`;
  `research/audit-2026-09-24.md` §4.3; plan-deviations P13 (it recurred).
  **[web]** [sqlc#4372](https://github.com/sqlc-dev/sqlc/issues/4372), "SQLite: non-ASCII character (em-dash)
  in SQL comment corrupts generated queries" (v1.30.0): closed 2026-04-17;
  [sqlc#4523](https://github.com/sqlc-dev/sqlc/issues/4523), "multi-byte UTF-8 in a query comment silently
  truncates the generated query string" (v1.31.1): closed 2026-08-18. The cause is that the ANTLR SQLite
  parser counted runes while sqlc sliced the query text by bytes; besides a syntax error it can silently drop
  a trailing `?` or `ORDER BY` from the generated query. **Fixed upstream:**
  [sqlc#4535](https://github.com/sqlc-dev/sqlc/pull/4535) replaced the ANTLR parser with a hand-written one
  (merged 2026-08-01) and [sqlc#4568](https://github.com/sqlc-dev/sqlc/pull/4568) pins it with tests; no
  release carries it yet (latest v1.31.1, 2026-04-22, checked 2026-10-08), so keep the ASCII rule until a
  release does.


### `sqlc.slice()` and untyped SQLite columns generate `interface{}`

- **What we do:** `IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))`, overrides, and
  `TestGeneratedQueries_HaveNoUntypedValues` (`internal/store/sqlcgen_test.go:15-17`).
- **Evidence:** sql.md:85-94; an agent's working note.
  **[web]** [sqlc#3754](https://github.com/sqlc-dev/sqlc/issues/3754), "sqlc always convert something like
  BOOLEAN(2) and CHAR(255) to interface{}": open; [sqlc#3383](https://github.com/sqlc-dev/sqlc/issues/3383),
  "interface{} instead of time.Time for json extracted queries": open;
  [sqlc#1830](https://github.com/sqlc-dev/sqlc/issues/1830), "`json_each`'s `value` parameter isn't
  supported": open; [sqlc#2452](https://github.com/sqlc-dev/sqlc/issues/2452), "`sqlc.slice()` breaks SQLite
  queries that contain other parameters": open; [sqlc#3451](https://github.com/sqlc-dev/sqlc/issues/3451),
  "SQLite cannot use an empty slice for `NOT IN`": open (all checked 2026-10-08).
  [sqlc#4574](https://github.com/sqlc-dev/sqlc/pull/4574) (merged 2026-08-20, unreleased) only spells
  `interface{}` as `any`; the untyped values remain.


### sqlc cannot parse FTS5 `MATCH`, `LIKE … ESCAPE` or SQLite 3.53's new `ALTER` forms

- **What we do:** hand-written statements. `internal/store/search.go:30-31`.
- **Evidence:** sql.md:88-105; an agent's working note.
  **[web]** [sqlc#3204](https://github.com/sqlc-dev/sqlc/issues/3204), "SQLite: Add support for WHERE clause
  on the FTS5 Table Name": open; [sqlc#3901](https://github.com/sqlc-dev/sqlc/issues/3901), "`rowid` doesn't
  work with SQLite FTS5 tables": open (checked 2026-10-08). No issue found for `LIKE … ESCAPE` or the 3.53
  `ALTER` forms (searched sqlc issues). Unreleased work on main may change this:
  [sqlc#4535](https://github.com/sqlc-dev/sqlc/pull/4535) replaced the ANTLR SQLite parser with one ported
  from SQLite's own grammar (merged 2026-08-01) and [sqlc#4579](https://github.com/sqlc-dev/sqlc/pull/4579)
  models FTS5's hidden columns (closed 2026-08-25); re-test the hand-written statements once a release after
  v1.31.1 ships.


### `sqlc vet` sees a query after `*` is expanded, so a "no SELECT *" rule can never fire

- **What we do:** `TestArch_QueriesNameEveryColumn` reads the .sql files instead (`sqlc.yaml:66-67`).
- **Evidence:** sql.md:55.
  **[web]** no public report found (searched sqlc issues for vet with `SELECT *`, star, expanded, and the vet
  docs, which do not say `query.sql` is the expanded text; 2026-10-08).


### A pragma set by `Exec` reaches only one connection of the pool, and the pure-Go driver's DSN syntax differs from mattn's

- **What we do:** pragmas go in the DSN as `_pragma=…`, are read back on every pool, and Open refuses if they
  are wrong. `internal/store/open.go:26-33,162+` (`verifyPragmas`).
- **Evidence:** sql.md:165-172; `research/audit-2026-09-23.md` §4.2;
  `TestVerifyPragmas_RefusesAConnectionWithoutThem`. [doc]
  **[web]** [mattn/go-sqlite3#1058](https://github.com/mattn/go-sqlite3/issues/1058), "PRAGMA Value Gets
  Reset": closed, the same pool cause, resolved by moving the pragma into the DSN;
  [mattn/go-sqlite3#762](https://github.com/mattn/go-sqlite3/issues/762), "how to set cache_size for every
  connection in connection pool?": closed. **Contradicts:** the DSN syntaxes have partly converged:
  modernc.org/sqlite v1.55.0 (2026-07-20,
  [CHANGELOG](https://gitlab.com/cznic/sqlite/-/raw/master/CHANGELOG.md), [MR
  134](https://gitlab.com/cznic/sqlite/-/merge_requests/134)) accepts mattn's `_busy_timeout`,
  `_foreign_keys`, `_journal_mode`, `_synchronous`, `_auto_vacuum` and `_query_only` keys and validates their
  values, so a mattn-style key is no longer silently ignored (we pin v1.59.0). The [driver
  docs](https://pkg.go.dev/modernc.org/sqlite) also say each `_pragma` value runs as SQL text, so anything
  after a `;` runs too, unless `StrictPragmas(true)` is set (checked 2026-10-08).


### `SQLITE_BUSY_SNAPSHOT` shares "database is locked" with `SQLITE_BUSY`, but `busy_timeout` retries only the latter

- **What we do:** one writer with `_txlock=immediate` and a `query_only` reader pool (`internal/store/open.go:27-33`).
- **Evidence:** D31; sql.md:186-194; `research/sqlite-reconsidered.md` §2 [web, Navidrome].
  **[web]** [SQLite result codes, SQLITE_BUSY_SNAPSHOT (517)](https://www.sqlite.org/rescode.html): raised in
  WAL mode when a read transaction is promoted to a write after another connection wrote;
  [sqlite3_busy_handler](https://www.sqlite.org/c3ref/busy_handler.html): "If SQLite determines that invoking
  the busy handler could result in a deadlock, it will go ahead and return SQLITE_BUSY" without calling it.
  Both match our cause; the documented fix is the one we use, starting write transactions with `BEGIN
  IMMEDIATE`. The [modernc.org/sqlite CHANGELOG](https://gitlab.com/cznic/sqlite/-/raw/master/CHANGELOG.md)
  also records a fixed driver bug where SQLITE_BUSY during COMMIT left a pooled connection inside a
  transaction (checked 2026-10-08).


### Copying a live WAL database's `.db` file gives a file that opens cleanly and has no tables

- **Symptom:** `no such table`.
- **What we do:** `VACUUM INTO` for backups, `os.Stat` before a restore, and refusing `?` in paths (the driver
  splits the DSN there).
- **Evidence:** D77, D242 §5 [measured].
  **[web]** [How To Corrupt An SQLite Database File, §1.2 and §1.4](https://www.sqlite.org/howtocorrupt.html)
  and [WAL mode](https://www.sqlite.org/wal.html): "The WAL file is part of the persistent state of the
  database and should be kept with the database if the database is copied or moved"; documented, no bug to
  track. The [modernc.org/sqlite docs](https://pkg.go.dev/modernc.org/sqlite) confirm the `?` split: a plain
  file name has its query removed before it reaches SQLite, so every key the driver does not interpret is
  ignored, `mode=ro` included (checked 2026-10-08).


### SQLite's limits: 1000 OR terms ("Expression tree is too large") and 64 tables in a join

- **What we do:** EXISTS subqueries with batched ORs (D75).
- **Evidence:** working-notes.md [measured on 3.46.1].
  **[web]** [Limits In SQLite](https://www.sqlite.org/limits.html): `SQLITE_MAX_EXPR_DEPTH` defaults to 1000
  and can only be lowered at run time; the 64-table join limit comes from the planner's bitmaps and cannot be
  changed at all. Documented, no bug to track (checked 2026-10-08).


### FTS5 tokeniser gaps

- **Symptom:** unicode61 makes a CJK run one token and shreds Thai. Neither tokeniser folds Ø, Æ, Œ, ß, Ð or Ł,
  and the operators are case-sensitive.
- **What we do:** two indexes (unicode61 and trigram), alternates normalised in Go, and the user's operators
  lowercased (D48).
- **Evidence:** working-notes.md [measured].
  **[web]** [SQLite FTS5 docs](https://www.sqlite.org/fts5.html): unicode61 classes characters and folds case
  by Unicode 6.1, takes only categories `L* N* Co` as token characters by default, and removes diacritics
  only, so ligatures and letters like Ø or ß are not folded; `AND`, `OR`, `NOT` and `NEAR` are case-sensitive
  keywords; FTS5 has no ICU tokenizer. All documented, no bug to track (checked 2026-10-08). **Better
  workaround:** for Thai, whose vowel and tone marks are category Mn, `tokenize="unicode61 categories 'L* N*
  Co Mn'"` (the docs' own example) keeps the marks inside a token instead of splitting on them; a Thai run
  then becomes one token like CJK, which the trigram index already covers.


### `modernc.org/libc` imports `os/exec`, which trips "the core never execs"

- **What we do:** exactly that import edge is exempted in `TestArch_CoreNeverExecs` (D199).
- **Evidence:** D199; `research/audit-2026-09-24.md` §4.5 [measured, M1].
  **[web]** no public report found (searched the gitlab.com/cznic/libc issue tracker for exec, 2026-10-08).
  [source] In libc v1.75.7 the import serves C's `system()` (`Xsystem` runs `exec.Command("sh", "-c", …)` in
  `libc_musl.go`), which SQLite never calls.


### On a file opened `mode=ro`, `integrity_check` skips CHECK constraints

- **Symptom:** a copy holding a row its table's CHECK forbids answers `PRAGMA integrity_check`
  with `ok` when opened `file:…?mode=ro`, and with `CHECK constraint failed in user` when opened
  by its plain path with `query_only(1)`. Measured on SQLite 3.53.4 (modernc), 2026-10-16.
- **What we do:** a backup's copy is checked through its plain path with `query_only(1)`.
  `checkCopy` in `internal/store/backup.go`; `TestBackupInto_RefusesACopyThatFailsItsIntegrityCheck`.
- **Evidence:** measured here; the [pragma documentation](https://www.sqlite.org/pragma.html#pragma_integrity_check)
  says CHECK constraints are verified and says nothing of read-only files. **[web]** no report found
  on the SQLite forum (searched 2026-10-16).

### SQLite's locking is unreliable on network filesystems

- **What we do:** refuse the persistent directory on ten statfs magic numbers.
  `internal/core/instancedir/netfs_linux.go:10-42`.
- **Evidence:** D31; SQLite's FAQ. [doc]
  **[web]** [SQLite Over a Network](https://www.sqlite.org/useovernet.html) and [WAL
  mode](https://www.sqlite.org/wal.html): "WAL does not work over a network filesystem", because WAL needs
  shared memory between processes; the FAQ's advice holds. Documented, no bug to track (checked 2026-10-08).


### `COLLATE NOCASE` folds only ASCII, and temp files follow TMPDIR

- **What we do:** usernames are case-insensitive for ASCII only (the owner accepted this), and
  `temp_store(MEMORY)`. `internal/store/queries/user.sql:18-19`; `internal/store/open.go:39-41`.
- **Evidence:** [doc] Low priority.
  **[web]** [Datatypes, collating sequences](https://www.sqlite.org/datatype3.html): NOCASE folds "only ASCII
  characters"; [Temporary files](https://www.sqlite.org/tempfiles.html): the directory is chosen from
  `temp_store_directory`, then `SQLITE_TMPDIR`, then `TMPDIR`, then /var/tmp, /usr/tmp, /tmp and ".".
  Documented (checked 2026-10-08). **Better workaround:** if Unicode case-insensitive names are ever wanted,
  modernc.org/sqlite can register a Go collation with `RegisterCollationUtf8`
  ([docs](https://pkg.go.dev/modernc.org/sqlite)) instead of NOCASE.

---

---

## Go standard library and toolchain


### Go regexp's `\w`, `\d`, `\s` and `\b` are ASCII-only and fail silently on names like "Amélie"

- **What we do:** `\p{L}` and the like. Those classes are banned in the naming corpus, which has a non-Latin
  fixture. `internal/core/identify/filename.go:35-38`.
- **Evidence:** D49. [doc]
  **[web]** [regexp/syntax](https://pkg.go.dev/regexp/syntax) documents `\w` as `[0-9A-Za-z_]` and `\b` as an
  "ASCII word boundary" (RE2's semantics). No issue proposing Unicode-aware classes found (searched golang/go
  issues, 2026-10-08).


### `x/text/collate` is stuck at Unicode 6.2 and sorts Korean by code point (golang/go#19087)

- **What we do:** materialised sort keys, with Korean accepted as wrong (D47). Custom SQLite collations were
  rejected.
- **Evidence:** working-notes.md [measured].
  **[web]** [golang/go#19087](https://github.com/golang/go/issues/19087), "x/text/collate: collation does not
  work for Korean": open, NeedsFix, since 2017 (checked 2026-10-08). The maintainer's diagnosis there is that
  the CLDR tables give Hangul a higher primary weight than Latin and the package lacks script reordering, so
  Korean sorts after Latin.
  [`collate/tables.go`](https://raw.githubusercontent.com/golang/text/master/collate/tables.go) on master
  still says `UnicodeVersion = "6.2.0"`, `CLDRVersion = "23"`.


### golangci-lint refuses to start beside another run (exit 3) and prints nothing on stdout

- **Symptom:** "parallel golangci-lint is running". A gate read the empty stdout as "no findings" and passed.
- **What we do:** `--allow-serial-runners` for `make go-lint`'s runs (`Makefile`, `go-lint`), and
  `scripts/lint_speculative_api.py` fails on any exit status but 0 or 1. The speculative-API check
  lints its platforms at once with `--allow-parallel-runners`, each on a `GOLANGCI_LINT_CACHE` of its
  own, since nothing says what the lock protects; waiting on it took 36 s of a `make check` (the test
  audit, 2026-10-08).
- **Evidence:** commit "fix(gates): the linters wait for another run's lock, and a linter that could not run fails" (2026-09-30).
  **[web]**
  [`pkg/commands/run.go`](https://raw.githubusercontent.com/golangci/golangci-lint/main/pkg/commands/run.go):
  the lock is `golangci-lint.lock` in `os.TempDir()`; without `--allow-serial-runners` it retries for 5 s and
  then fails with "parallel golangci-lint is running"; `--allow-parallel-runners` skips the lock. Reported as
  [golangci/golangci-lint#869](https://github.com/golangci/golangci-lint/issues/869) (closed 2020); no issue
  about the empty stdout found (searched the repo's issues, 2026-10-08).


### govet's shadow check flags every `x := …` once a later file mentions a type named `x`

- **Technology:** golangci-lint's govet `shadow` analyzer (strict off).
- **Symptom:** "shadow: declaration of "fixture" shadows declaration at line 30" on every unchanged
  `fixture := newFixture(t)` in `internal/core/auth`'s tests, line 30 being `type fixture struct`, as soon
  as a new test file (sorting after them) named the type `fixture` in a helper's signature. Without that
  file, none.
- **Cause [I]:** the analyzer takes the package-level type as the shadowed object and counts a mention of
  it in a later file as a use after the shadowing declaration, comparing positions across files.
- **What we do:** a test helper takes the fixture's parts (`*Service`, the time), not a parameter of the
  type `fixture` (`internal/core/auth/locators_test.go`, 2026-10-13).
- **Evidence:** observed here, 2026-10-13. **[web]** [golangci/govet#2](https://github.com/golangci/govet/issues/2)
  reports a shadow at a line that held nothing, the same mismatch of positions; no issue naming this case
  found.


### A copy of git's index with a new time hides a file changed in the second the index was written

- **Technology:** git's index stat cache ("racy git").
- **Symptom:** a tree built from a copy of the index (`GIT_INDEX_FILE`, `git add -A`,
  `git write-tree`) records a file's old content when it was edited to the same size within the
  second the real index was written: `make check`'s area stamp skipped a step that should have run
  (`test_area_stamp.py` failed under load, 2026-10-08).
- **Cause:** git trusts an entry whose size and mtime match the file, and re-hashes it only when the
  file's mtime is not before the index file's own: a "racily clean" entry. A copy given a new mtime
  makes every such entry look safely older than its index.
- **What we do:** `shutil.copy2`, which keeps the index's time, in `working_tree_id`
  (`scripts/check_stamp.py`), which both the commit stamp and `scripts/area_stamp.py` use.
  `test_a_file_changed_in_the_second_the_index_was_written_is_seen` sets the times and checks it.
- **Evidence:** [doc] git's `Documentation/technical/racy-git.txt`
  (https://git-scm.com/docs/racy-git): an entry is racily clean when its mtime is not before the
  index file's, and git then compares contents.


### The default HTTP transport reads HTTP(S)_PROXY, and Go downloads another toolchain unless `GOTOOLCHAIN=local`

- **What we do:** a client with no proxy (`internal/platform/outbound/client.go:12-14`, forbidigo), and
  `deploy/Containerfile:68-71`.
- **Evidence:** D204; `TestNewClient_TakesNoProxyFromTheEnvironment`. [doc] **Unsure:** these are design
  choices about documented behaviour.
  **[web]** [net/http `DefaultTransport`](https://pkg.go.dev/net/http) "uses HTTP proxies as directed by the
  environment variables HTTP_PROXY, HTTPS_PROXY and NO_PROXY"; [Go Toolchains](https://go.dev/doc/toolchain):
  "The default GOTOOLCHAIN setting is auto", set in `$GOROOT/go.env`, and a toolchain repackaged without that
  file assumes `local` (checked 2026-10-08).


### Go's standard library reads the process's TZ

- **What we do:** the process zone is kept UTC, and the zone is passed explicitly (D246 §7).
- **Evidence:** D246 §7. [doc]
  **[web]** [time.Local](https://pkg.go.dev/time): "On Unix systems, Local consults the TZ environment
  variable"; no TZ means `/etc/localtime`, and `TZ=""` means UTC (checked 2026-10-08).

---

## Test tooling: Playwright, Vitest, happy-dom, TypeScript


### Vitest strips types without type-checking, so a types-only change can never fail a test

- **What we do:** a green TypeScript revert is also type-checked with `tsc`.
- **Evidence:** `research/audit-2026-09-27.md` "Four blind spots". [doc]
  **[web]** [Vite's features guide](https://vite.dev/guide/features) says Vite "only performs transpilation on
  `.ts` files and does **NOT** perform type checking", and recommends `tsc --noEmit` beside it; [Vitest's
  type-testing guide](https://vitest.dev/guide/testing-types) offers `--typecheck`, which runs `tsc --noEmit`
  over `*.test-d.ts` files by default (`typecheck.include`) without running them (read 2026-10-08). Working as
  designed; no tracker issue.


### happy-dom does not carry a disabled `<fieldset>` to its controls

- **What we do:** the test asserts on the fieldset's own `disabled`. `web/apps/web/src/busy-group.test.tsx:16-30`.
- **Evidence:** D244 §4.
  **[web]** [happy-dom#2385](https://github.com/capricorn86/happy-dom/issues/2385), ":disabled does not match
  form controls inside a disabled <fieldset>" (20.14.0): open, fix in [PR
  #2386](https://github.com/capricorn86/happy-dom/pull/2386), open, last updated 2026-09-13; the related
  [happy-dom#2184](https://github.com/capricorn86/happy-dom/issues/2184) (FormData keeps disabled controls) is
  open too (checked 2026-10-08). Re-check when #2386 merges.


### An iPhone pauses a video as its own full-screen player closes

- **What we do:** a following player takes the pause near `webkitendfullscreen` for its own, not its
  person's: a pause waiting to be sent is dropped, one still to come is marked, and the video is put
  back where the group is. `web/packages/sdk/src/together-follow.ts` (`browserPaused`).
- **Evidence:** the owner's test, 2026-10-11: each time an iPhone guest with the right to pause left
  full screen by its X, the log showed its player send a pause, and the group paused for everyone.
  An iPhone has no element Fullscreen API, only a video's own full-screen player
  (`webkitEnterFullscreen`, Apple Developer Forums thread 770080, searched 2026-10-11); which comes
  first, the pause or `webkitendfullscreen`, is not documented, so both orders are handled.


### iOS pauses a playing video as its screen locks, before the page is told it is hidden

- **Symptom:** an iPhone in a Watch Together group, left until its screen locked, sent a pause, and
  the group paused for everyone: an Android phone playing beside it stopped (the owner's test,
  2026-10-12; the core's log, `a leading player sent a command … command=pause` with no seek after).
- **Cause:** WebKit interrupts a playing video the moment the screen locks, before any page script
  runs, and the `pause` can come before the `visibilitychange` to hidden; to the page it reads as
  its person's tap.
- **What we do:** a leading player's pause waits `browserPauseWindowMs` (500 ms) before it is sent,
  and the page hiding within it, or before it, makes it the browser's pause, as the full-screen
  player's is above. `web/packages/sdk/src/together-follow.ts` (`browserPaused`, `followGroup`'s
  `page`). The films' players also hold a screen wake lock while they play (D287), so a screen left
  alone does not lock mid-film. VQ35.
- **Evidence:** the owner's test, 2026-10-12. **[web]** The same order, and the same half-second
  wait to tell a tapped pause from the lock's, in
  [jackywxd/haijieliu#11](https://github.com/jackywxd/haijieliu/pull/11) (read 2026-10-12).


### Safari sizes a `height: 100%` video in an `aspect-ratio` box wrongly, cutting off its controls

(The heading names the first suspect; see What we do.)

- **What we do:** the guest's video is laid out as the Play page's is, in the flow at the page's width
  with nothing clipping it, and the film's title sits above it, not over it.
  `web/apps/web/src/together-viewer.css`. Neither this nor the first fix, placing the video absolutely
  in the clipping stage, has been seen on the phone: its guest tab most likely ran an older client
  through both (agent-failures A28). Unconfirmed until VQ29 runs on a freshly loaded page.
- **Evidence:** the owner's test, 2026-10-11: on an iPhone on iOS 18.5 a guest saw none of the player's
  controls, which Chrome and Firefox showed, and the title lay over the picture (VQ29).
  **[web]** [WebKit 261556](https://bugs.webkit.org/show_bug.cgi?id=261556) and [a write-up of the
  same bug](https://blog.duvallj.pw/posts/2024-09-14-safari-css-bug.html) (2024): Safari resolves a
  media element's percentage height against an `aspect-ratio` box wrongly; `height: fit-content` or
  absolute placement avoids it; [media-chrome discussion
  831](https://github.com/muxinc/media-chrome/discussions/831) meets Safari's video misplaced against its
  controls (searched 2026-10-11). Unconfirmed on the phone until VQ29 runs.


### Playwright grants "microphone" only on Chromium; Firefox needs prefs

- **What we do:** `web/e2e/together-pages.ts:189-200`; `web/e2e/browsers.ts:15-19`.
- **Evidence:** inferred from the code, which has no comment. Cause unverified.
  **[web]** [playwright#7635](https://github.com/microsoft/playwright/issues/7635), "support 'microphone'
  permission on Firefox": a maintainer confirmed "Firefox indeed doesn't support microphone permission"
  (2021), closed by the stale-issue sweep, not fixed;
  [playwright#11714](https://github.com/microsoft/playwright/issues/11714) says the same for WebKit (closed
  the same way, 2022); [playwright#20563](https://github.com/microsoft/playwright/issues/20563) (Firefox and
  Safari, closed 2023-11) reports `Unknown permission: camera`. The [grantPermissions
  docs](https://playwright.dev/docs/api/class-browsercontext) say only that "Supported permissions differ
  between browsers" (read 2026-10-08). This confirms the cause. Users in #20563 grant it in Firefox with
  `firefoxUserPrefs` `permissions.default.microphone: 1`, browser-wide; our
  `media.navigator.permission.disabled` does the same job.


### TypeScript checks no excess properties on a body built with a computed key or held in a variable

- **Symptom:** after the D227 renames the client compiled while still sending the old field names.
- **What we do:** bodies take their names from the generated types via `satisfies`.
- **Evidence:** agent-failures A12 (2026-09-29). [doc] **Borderline:** TypeScript works as designed.
  **[web]** [TypeScript#36920](https://github.com/microsoft/TypeScript/issues/36920), "No excess property
  error for computed property": open, In Discussion/Suggestion (reopens #22427);
  [TypeScript#60274](https://github.com/microsoft/TypeScript/issues/60274) on computed keys was closed Working
  as Intended ("this property is possibly valid, therefore not excess"). The
  [handbook](https://www.typescriptlang.org/docs/handbook/2/objects.html) documents that assigning the literal
  to a variable first skips the check (read 2026-10-08).


### Playwright's browsers are not installed by `npm ci`

- **Evidence:** `research/audit-2026-09-29.md` §6. [doc] Low priority.
  **[web]** [Playwright's browsers page](https://playwright.dev/docs/browsers) documents it: each version
  needs its own browser builds, installed by `npx playwright install` (`--with-deps` in CI);
  `PLAYWRIGHT_BROWSERS_PATH` shares them between runs (read 2026-10-08). Working as designed; no tracker
  issue.

---

---

## Development tooling: git, npm, Vite, Python, procps, yoloAI


### Git hooks export GIT_DIR, so a test's `git init` set `core.bare` on the project repository

- **What we do:** strip `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE` and related variables.
  `scripts/py_tests.py:26-36`.
- **Evidence:** commit "feat(gates): a passing check-pr marks where it ended, and the parallel gate's two races are closed" (2026-10-01).
  **[web]** [githooks(5)](https://git-scm.com/docs/githooks) documents it: GIT_DIR, GIT_WORK_TREE and others
  "are exported so that Git commands run by the hook can correctly locate the repository", and a hook working
  in a foreign repository "should clear these environment variables" (read 2026-10-08). **Better workaround:**
  the docs clear them with `unset $(git rev-parse --local-env-vars)`, git's own list; on git 2.47 it also
  names GIT_CONFIG, GIT_CONFIG_PARAMETERS, GIT_CONFIG_COUNT, GIT_IMPLICIT_WORK_TREE, GIT_GRAFT_FILE,
  GIT_NO_REPLACE_OBJECTS, GIT_REPLACE_REF_BASE and GIT_SHALLOW_FILE, which our hand-written list lacks.


### `npm ci` through a symlinked `node_modules` empties the link's target

- **What we do:** a real install, or `cp -al`, in a worktree. revert-red links each package instead
  (`scripts/revert_red_trees.py:11-24`).
- **Evidence:** an agent's working note (2026-10-01). **Unsure:** our Makefile's mtime rule
  triggers the `npm ci`, but the emptying is npm's.
  **[web]** the [npm ci docs](https://docs.npmjs.com/cli/v11/commands/npm-ci) say "If a `node_modules` is
  already present, it will be automatically removed before `npm ci` begins its install"; through a link, that
  removal reaches the target's contents. [npm/cli#3669](https://github.com/npm/cli/issues/3669), "npm install
  deletes an existing symlinked node_modules directory" (npm 7.21), was closed with no fix shown;
  [npm/cli#2380](https://github.com/npm/cli/issues/2380), "npm install removes linked packages", is open
  (checked 2026-10-08). No report of exactly the `npm ci` emptying found.


### Two npm installs at once race

- **What we do:** `web-deps` runs before the parallel steps (Makefile:52-53). The error text was not recorded.
- **Evidence:** **[web]** [npm/cli#4383](https://github.com/npm/cli/issues/4383), "multiple `npm install`
  processes in the same folder" (2022), closed with no answer shown;
  [npm/cli#496](https://github.com/npm/cli/issues/496), "lchown error ... when running multiple installations
  in parallel", closed; [npm/cli#694](https://github.com/npm/cli/issues/694), "Transient ENOENT / file not
  found in npm ci", closed (checked 2026-10-08). npm documents no lock for concurrent installs in one folder;
  serialising them, as we do, is the only workaround seen.


### Vite's dev server misses a `git stash pop` and keeps serving old modules

- **Symptom:** a white screen in every browser.
- **What we do:** no stash or checkout under `make dev`; restart Vite alone (no workaround in the repo).
- **Evidence:** an agent's working note (2026-09-28). The cause is inferred.
  **[web]** no public report found (searched vitejs/vite issues for "git stash" and for branch switches with
  HMR not updating, 2026-10-08). The nearest is [vite#13593](https://github.com/vitejs/vite/issues/13593),
  "Support @parcel/watcher" (open), which calls chokidar "slow and known to have issues with file descriptor
  limits"; [server.watch](https://vite.dev/config/server-options) offers `usePolling` at a CPU cost.


### Python's bytecode cache is keyed on size and an mtime with one-second resolution

- **What we do:** `PYTHONDONTWRITEBYTECODE=1` for probes. `scripts/check_revert_red.py:511-513`. [doc]
- **Evidence:** **[web]** [PEP 552](https://peps.python.org/pep-0552/) (Final, 3.7): a pyc is prefixed by "the
  source timestamp, and the source file size"; it added hash-based pycs
  ([py_compile](https://docs.python.org/3/library/py_compile.html) `CHECKED_HASH`), which only
  `py_compile`/`compileall` write. [Command line docs](https://docs.python.org/3/using/cmdline.html):
  `PYTHONDONTWRITEBYTECODE` stops only the *writing* of pycs, so a stale pyc already in `__pycache__` is still
  read (read 2026-10-08). **Better workaround:** `PYTHONPYCACHEPREFIX` set to a fresh empty directory; per
  [sys.pycache_prefix](https://docs.python.org/3/library/sys.html#sys.pycache_prefix) (3.8+) Python then reads
  and writes pycs only there and ignores every `__pycache__` in the source tree.


### `pgrep -f` and `pkill -f` match the shell that runs them

- **Symptom:** a wait loop waited half an hour on its own command line, and a kill killed the calling shell.
- **What we do:** `ps | grep "[x]"` (no workaround in the repo). **Evidence:** an agent's working note. [doc]
- **Evidence:** **[web]** [pgrep(1)](https://man7.org/linux/man-pages/man1/pgrep.1.html) (procps-ng) says only
  that the running pgrep or pkill "will never report itself as a match"; with `-f` "the full command line is
  used", so the shell whose command line holds the pattern is not excluded (read 2026-10-08). No tracker issue
  found.


### A trailing-slash `.gitignore` pattern (`node_modules/`) does not match a symlink

- **What we do:** the pattern has no slash. **Evidence:** an agent's working note. [doc]
- **Evidence:** **[web]** [gitignore(5)](https://git-scm.com/docs/gitignore) documents it: "The pattern `foo/`
  will match a directory `foo` and paths underneath it, but will not match a regular file or a symbolic link
  `foo`" (read 2026-10-08). [doc]


### `yoloai apply` replays with `git am`, which stops on an empty patch

- **Symptom:** "Patch is empty".
- **What we do:** no empty commits; trailers ride on a real change.
- **Evidence:** an agent's working note (2026-10-02). **Unsure:** yoloAI is the owner's own tool. It is
  external to Orpheum, but perhaps not in scope.
  **[web]** [git-am(1)](https://git-scm.com/docs/git-am) documents `--empty=(drop|keep|stop)` for a message
  without a patch, with `stop` the default; it came in [Git
  2.35](https://raw.githubusercontent.com/git/git/master/Documentation/RelNotes/2.35.0.adoc) (read
  2026-10-08). yoloAI has no public tracker to search. **Better workaround:** if yoloAI passed `--empty=keep`
  to `git am`, an empty commit would replay as an empty commit; that is a change to yoloAI, not to Orpheum.

---

---

## Cloudflare, ACME and DNS



### Cloudflare adds its own CAA records to any zone with CAA while Universal SSL is on

- **Technology:** Cloudflare DNS, with Universal SSL.
- **Symptom:** after adding `0 issue ";"` and `0 issuewild ";"` at `orpheum-core.com`'s apex, `dig CAA`
  also showed `issue` and `issuewild` for `letsencrypt.org`, `pki.goog`, `digicert.com`, `ssl.com` and
  `comodoca.com`, on Cloudflare's own name server and on public resolvers. The dashboard shows only the
  two written; the rest appear only to `dig`. With them, `";"` restricts nothing: those CAs may issue
  for the whole zone, and a claimed core's `accounturi` CAA means nothing.
- **What we do:** Universal SSL is off on `orpheum-core.com` (D293 §10, done 2026-10-14), after which the
  added records went and only the two remained. Check with `dig CAA orpheum-core.com` after any change to
  the zone's SSL settings.
- **Evidence:** observed 2026-10-14. **[web]** [Add CAA records · Cloudflare SSL/TLS docs](https://developers.cloudflare.com/ssl/edge-certificates/caa-records/):
  "Cloudflare adds CAA records automatically when you have Universal SSL and add any CAA records to your
  zone", and they "will not appear in the Cloudflare dashboard".

### A Cloudflare-proxied record ends TLS, carries no UDP/TURN, and the terms restrict video

- **What we do:** the records are DNS-only, and the certificate comes by ACME DNS-01 through the API (D72,
  D243 §6). `deploy/vps/README.md:84-86`.
- **Evidence:** working-notes.md [web]. [doc]
  **[web]** [Cloudflare's service-specific
  terms](https://www.cloudflare.com/service-specific-terms-application-services/): video and other large files
  must be served through paid services (Stream, Images, the Developer Platform), and Cloudflare may limit the
  CDN for anyone suspected of serving video without them; [proxy
  status](https://developers.cloudflare.com/dns/proxy-status/) covers HTTP/HTTPS traffic of a proxied record
  (checked 2026-10-08).


### There is no publicly trusted certificate for a bare IP, and private-IP DNS answers trip rebinding protection

- **What we do:** one public name with DNS-01 (D243).
- **Evidence:** `research/remote-access.md` §4 [web]. This is a limitation.
  **[web]** [Let's Encrypt, "6-day and IP Address Certificates are Generally
  Available"](https://letsencrypt.org/2026/01/15/6day-and-ip-general-availability) (2026-01-15): IP address
  certificates for IPv4 and IPv6, valid 160 hours, from the `shortlived` profile; [the first-IP-certificate
  post](https://letsencrypt.org/2025/07/01/issuing-our-first-ip-address-certificate) says only http-01 and
  tls-alpn-01 can validate them, not DNS-01. [dnsmasq's
  `--stop-dns-rebind`](https://thekelleys.org.uk/dnsmasq/docs/dnsmasq-man.html) rejects private-range answers
  from upstream, the router behaviour behind the second half. **Contradicts:** a publicly trusted certificate
  for a bare IP has existed since January 2026, though it needs port 80 or 443 reachable on that IP and
  renewal every few days, so it fits the VPS's public IP but not the home's DS-Lite address.

*Nothing idiosyncratic was found in `internal/core/certificate` or `providers/cloudflare`. The paged-zone
miss was our own bug (Appendix B).*

---

---

## Watch list: researched, not yet met here

External behaviour the research found that has shaped designs but has not yet forced a workaround
in the code. Promote an item to an entry when it does.
- **WebKit:** a per-version bug table (`research/safari-versions.md` §2). It covers `seeked` never firing (17.0),
  stuck seeks (≤18.3), a fast-forward flash after a buffer flush (26.6), catch-up seeks freezing forever
  (27.0.1, and the reason hls.js stays at 1.7.1 or later), and audio going silently dead (WebKit 323104).
- **iPhone, seeking and the UA:** a seek made while paused before the first frame is dropped. Safari 26
  freezes the iOS version in the User-Agent at 18.x, and iPad sends a Mac UA.
- **iOS audio:** capture forces a VideoChat session with ducking, AirPods drop to HFP, and WebKit 218012
  lowers the volume.
- **hls.js:** `testBandwidth` ignores `firstAutoLevel`. Other controllers write `autoLevelCapping`.
  `resetEstimator` at manifest load overwrites the prior.
- **Opus and WebRTC:** in-band FEC starts late. No browser ships DRED. TURN over TCP adds head-of-line stalls.
- **Network APIs:** `navigator.connection` is capped and Chrome-only. TCP slow start restarts after idle.
- **Watch Together:** Web Audio is not a reliable echo-cancellation reference. iCloud Private Relay drops
  WebSockets.
- **Cast:** no MKV, SRT, ASS, PGS or DTS. Some receivers may accept HLS-TS only (unverified).
- **TVs and tvOS:** tvOS has no web runtime. react-native-webrtc has no data channels on tvOS.
- **Captions and codes:** a transcode destroys CEA-608/708. Language codes mix ISO 639-2/B and /T.
- **ISPs:** routers block inbound IPv6, IPv6 prefixes change, MAP-E has no 80/443, and UPnP is unreliable.
- **Lab hardware:** NVENC session limits, QSV on the J3455 encoding 8-bit HEVC only, and Vega OS Fire TVs.
- **Platform builds:** jellyfin-ffmpeg and Playwright's browsers have no riscv64 builds.
- **FFmpeg:** copy-seek snaps to the previous keyframe, which is mixed with our millisecond rounding and
  unverified. Uncapped x264 threads.
- **libass and JASSUB:** libass is VSFilter-compatible by default, and JASSUB lacks libunibreak. D50 is still
  provisional.
- **Android fonts:** Android has no last-resort font. Firefox draws hex boxes instead of Last Resort (D53/D54).
  These are font decisions that shaped bundling, so they could be promoted if wanted.
- **Media dropped from the main list:** iPhone's "0 dropped of 0 frames", and OpenSubtitles quotas and
  sync rates.
