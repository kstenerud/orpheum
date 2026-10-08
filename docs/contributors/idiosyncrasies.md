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

**How to use.** Scan the symptom index, then read the entry. **When you find a new one,** apply the
inclusion test, add the entry under its technology with **Symptom**, **Cause** (say "unverified"
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
| *Flow* (10-bit HEVC by x265) froze at the first frame of any chunk but the film's first. | [Firefox on macOS freezes on HEVC chunks that start on a CRA keyframe](#firefox-on-macos-freezes-on-hevc-chunks-that-start-on-a-cra-keyframe) |
| The browser takes a fragment's append without error but adds nothing to the buffer. | [hls.js reports a fragment the browser held nothing of, then does nothing about it](#hlsjs-reports-a-fragment-the-browser-held-nothing-of-then-does-nothing-about-it) |
| `RTCRtpReceiver.getCapabilities("audio")` lists no `audio/red`, so plain Opus is negotiated and voice has less loss protection. | [Firefox offers no RED (redundant audio) for Opus](#firefox-offers-no-red-redundant-audio-for-opus) |
| Voice hung at "Asking for your microphone" because opening the microphone awaited the context's resume. | [Firefox keeps `AudioContext.resume()` pending until it lets the page play sound](#firefox-keeps-audiocontextresume-pending-until-it-lets-the-page-play-sound) |
| With voice on, the film was turned down and the volume buttons stopped changing its volume. | [Firefox on Android makes a "call" of gain control and noise suppression, even with echo cancelling off](#firefox-on-android-makes-a-call-of-gain-control-and-noise-suppression-even-with-echo-cancelling-off) |
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
| X265 makes every keyframe after the first a CRA (open GOP) | [x265 makes every keyframe after the first a CRA (open GOP)](#x265-makes-every-keyframe-after-the-first-a-cra-open-gop) |
| The `segment` muxer misses a keyframe a hair before the cut time (10.009999 against 10.010) | [The `segment` muxer misses a keyframe a hair before the cut time (10.009999 against 10.010)](#the-segment-muxer-misses-a-keyframe-a-hair-before-the-cut-time-10009999-against-10010) |
| The `hls` muxer cuts by cumulative time and cannot keep a copied picture to a keyframe grid | [The `hls` muxer cuts by cumulative time and cannot keep a copied picture to a keyframe grid](#the-hls-muxer-cuts-by-cumulative-time-and-cannot-keep-a-copied-picture-to-a-keyframe-grid) |
| Each output file starts its fMP4 times at zero, and upstream has no flag for absolute `tfdt` | [Each output file starts its fMP4 times at zero, and upstream has no flag for absolute `tfdt`](#each-output-file-starts-its-fmp4-times-at-zero-and-upstream-has-no-flag-for-absolute-tfdt) |
| FMP4 init segments differ between runs unless the edit list is off | [fMP4 init segments differ between runs unless the edit list is off](#fmp4-init-segments-differ-between-runs-unless-the-edit-list-is-off) |
| An 8.7 ms seam where two runs meet. | [The AAC encoder adds 1024 priming samples at each run's start](#the-aac-encoder-adds-1024-priming-samples-at-each-runs-start) |
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
| `COLLATE NOCASE` folds only ASCII, and temp files follow TMPDIR | [`COLLATE NOCASE` folds only ASCII, and temp files follow TMPDIR](#collate-nocase-folds-only-ascii-and-temp-files-follow-tmpdir) |
| Go regexp's `\w`, `\d`, `\s` and `\b` are ASCII-only and fail silently on names like "Amélie" | [Go regexp's `\w`, `\d`, `\s` and `\b` are ASCII-only and fail silently on names like "Amélie"](#go-regexps-w-d-s-and-b-are-ascii-only-and-fail-silently-on-names-like-amélie) |
| `x/text/collate` is stuck at Unicode 6.2 and sorts Korean by code point (golang/go#19087) | [`x/text/collate` is stuck at Unicode 6.2 and sorts Korean by code point (golang/go#19087)](#xtextcollate-is-stuck-at-unicode-62-and-sorts-korean-by-code-point-golanggo19087) |
| "parallel golangci-lint is running". | [golangci-lint refuses to start beside another run (exit 3) and prints nothing on stdout](#golangci-lint-refuses-to-start-beside-another-run-exit-3-and-prints-nothing-on-stdout) |
| The default HTTP transport reads HTTP(S)_PROXY, and Go downloads another toolchain unless `GOTOOLCHAIN=local` | [The default HTTP transport reads HTTP(S)_PROXY, and Go downloads another toolchain unless `GOTOOLCHAIN=local`](#the-default-http-transport-reads-https_proxy-and-go-downloads-another-toolchain-unless-gotoolchainlocal) |
| Go's standard library reads the process's TZ | [Go's standard library reads the process's TZ](#gos-standard-library-reads-the-processs-tz) |
| Vitest strips types without type-checking, so a types-only change can never fail a test | [Vitest strips types without type-checking, so a types-only change can never fail a test](#vitest-strips-types-without-type-checking-so-a-types-only-change-can-never-fail-a-test) |
| Happy-dom does not carry a disabled `<fieldset>` to its controls | [happy-dom does not carry a disabled `<fieldset>` to its controls](#happy-dom-does-not-carry-a-disabled--to-its-controls) |
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

---

## Firefox


### Firefox on macOS freezes on HEVC chunks that start on a CRA keyframe

- **Technology:** Firefox 156 on macOS, HEVC through the system decoder, and x265's open-GOP keyframes.
- **Symptom:** *Flow* (10-bit HEVC by x265) froze at any start but the film's beginning: the player spun,
  paused at readyState 1, while its chunks kept arriving on a fast link. Safari played it everywhere, and so
  did Firefox from 0:00. Nothing says so in advance: `isTypeSupported` says HEVC is playable, and it is,
  from an IDR.
- **Cause:** x265's keyframes after the first are CRA, not IDR, so every chunk but the first starts on a CRA.
  Firefox on macOS takes such a chunk and **holds nothing of it**. Captured at the moment of giving up: the
  picture's buffer was empty while the sound's held 416.24–501.78, and hls.js reported
  `bufferAppendNoProgress: Fragment append did not increase buffered coverage`. Firefox's reason (likely not
  counting a CRA as a random access point on that path) is unverified.
- **What we do:** D266. A quality is given up for the playback, with every quality above it, and the video's
  decoding is built anew (`recoverMediaError`). This happens at once when two of its chunks are held nothing
  of, or after 6 s standing still while two chunks arrive. The core logs the quality, with the video as it
  was. There is no User-Agent branching.
  `web/packages/player/src/abr-choice.ts` (`isUnplayable`, `chunksNotHeld`); `web/packages/player/src/abr.ts`
  (`#lookAtPlaying`, `notHeld`); `internal/core/api/report.go` (`logUnplayable`).
- **Evidence:** D266; [owner] 2026-10-07 (Safari plays it, Firefox from the start only) and 2026-10-08 (the
  empty picture buffer and `bufferAppendNoProgress` above, 05:16 UTC on staging).
- **Caution:** agent-failures A22 (2026-10-06) records an earlier wrong diagnosis, "Firefox can't decode
  HEVC". Part of that freeze was our own gap bug. A stall report taken *after* a
  rebuild showed the sound missing instead: that was the new pipeline refilling, not the cause.
- **Firefox on Linux plays the same chunks:** headless Firefox 155 on Linux, through Playwright against
  staging on 2026-10-07/08, played Flow's own picture from mid-film chunks, picture and sound buffered alike.

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


### Firefox keeps `AudioContext.resume()` pending until it lets the page play sound

- **Technology:** Firefox autoplay policy + Web Audio.
- **Symptom:** voice hung at "Asking for your microphone" because opening the microphone awaited the context's
  resume.
- **What we do:** `resume()` is fire-and-forget, never awaited.
  `web/packages/sdk/src/together-voice-browser.ts:227-247` (comment at `:236-238`).
- **Evidence:** [measured] browser tier (Playwright Firefox), 2026-10-06; fix 22fba2c0;
  `research/audit-2026-10-06.md` §1.


### Firefox on Android makes a "call" of gain control and noise suppression, even with echo cancelling off

- **Technology:** Firefox for Android, getUserMedia.
- **Symptom:** with voice on, the film was turned down and the volume buttons stopped changing its volume.
  Chrome on Android does this only for echo cancelling (see 3.1).
- **Cause:** presumably the audio session switches to communication mode. Unverified for Firefox.
- **What we do:** `should_together_voice_level_microphone` (`shouldLevelMicrophone`) is off by default.
  `web/packages/sdk/src/together-voice-browser.ts:13-18`; `web/packages/sdk/src/together.ts:109,121`;
  `internal/core/settings/registry.go:296-301`; `internal/core/together/messages.go:262-267`.
- **Evidence:** [owner] 2026-10-07, Firefox 157 on Android 16; VQ23.

---

## Safari / iOS / WebKit


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


### Safari's rate nudges stutter or skip, especially with pitch-keeping on

- **Technology:** WebKit `playbackRate` + `preservesPitch`.
- **Symptom:** each rate change may make the sound skip. Rate and seek land 100-500 ms off, against 2 frames or
  less in Chrome and Firefox (hls.js #7616, shaka #2823). Jellyfin turned SpeedToSync off ("failing on Safari").
- **Cause:** inferred: WebKit swaps AVPlayerItem's pitch algorithm when the rate is not 1 [source].
- **What we do:** the `should_together_nudge_keep_pitch` setting, small and infrequent nudges, and seek-only as
  the fallback. `internal/core/settings/registry.go:281-284`.
- **Evidence:** D250 §7, D62; VQ22 (open); `research/safari-versions.md` §3, §5; [web].


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


### iPhone's ManagedMediaSource decides how far ahead the page may load (about 30 s)

- **Technology:** WebKit ManagedMediaSource.
- **Symptom:** `streaming` goes false at 30 s buffered and back true below 10 s, overriding hls.js's
  `maxBufferLength`. Content may be evicted at any time.
- **What we do:** catch-up arithmetic assumes 30 s can be loaded ahead.
  `web/packages/sdk/src/together-follow.ts:35-41` (`playerLoadsAheadMs`).
- **Evidence:** D250 §4; `research/playback-in-practice.md` §1; [source] WebKit `ManagedMediaSource.cpp`. The
  real thresholds on iOS are not device-confirmed (test T5).


### iOS: an element's volume is read-only to the page, and Web Audio output follows the ring/silent switch

- **Technology:** iOS Safari.
- **Symptom:** `element.volume = x` is ignored. Routing the film through Web Audio to apply a gain would mute it
  on a phone in silent mode.
- **What we do:** `canSetVolume` writes the volume and reads it back. Where it cannot be set, no gain is
  applied, and only a prepared copy with the gain baked in is levelled.
  `web/packages/player/src/gain.ts:18-33,97-104`.
- **Evidence:** D226 §4; VQ13 (open, device check).


### iPhone Safari's native HLS cannot be routed through Web Audio, so it cannot be given a gain

- **What we do:** the gain is carried in the prepared sound (`CarriedGainDB`,
  `internal/nodeproto/prepare.go:66-68`).
- **Evidence:** D226 §4-§5; VQ13.


### iOS suspends the AudioContext as the microphone opens

- **Symptom:** voice goes silent after getUserMedia.
- **What we do:** `context.resume()` again once the microphone is open.
  `web/packages/sdk/src/together-voice-browser.ts:236-239`.
- **Evidence:** the code comment only, with no date or device test. Related: WebKit 218012 [web].
  Cause unverified.


### iPhone video fullscreen is not the Fullscreen API, and it leaves page overlays behind

- **Technology:** iOS Safari `<video>`. iPhone has no element fullscreen, and its player is always fullscreen.
- **Symptom:** subtitles drawn by JASSUB over the video vanished in fullscreen ("they stop showing when I
  switch to full screen mode"). `fullscreenchange` never fires for the iPhone's player.
- **What we do:** the chosen track is also given to the browser as a WebVTT track, shown only in native
  fullscreen, and toggled on `fullscreenchange` and on WebKit's `webkitbeginfullscreen`/`webkitendfullscreen`.
  `web/packages/player/src/fullscreen-captions.ts:78-123`.
- **Evidence:** D254, [owner] 2026-10-07 (the owner's phone); VQ25.


### Safari's native HLS starts on the first variant listed, whatever start hint is given

- **What we do:** a lower start quality, chosen from the remembered link speed, is *listed first* in the master
  playlist (D257 §2).
- **Evidence:** D257 [web/research].


### iPhone Safari loads nothing for a media element until the user has tapped it

- **Symptom:** the film page's hidden warm-up player loads nothing on iPhone.
- **What we do:** on such browsers the warm-up only starts the chunks on the server (D260 §3).
- **Evidence:** D260; `research/safari-versions.md` §3 "preload" [source]. This is a
  limitation.


### Native HLS gives the page no track choice, no quality control and no link measurement

- **What we do:** `setAudioRendition` is a no-op and `estimateKbps` is undefined on the native path, so the
  dialogue-boost switch does nothing there. `web/packages/player/src/hls.ts:65-77`.
- **Evidence:** `research/safari-versions.md` §3 "Quality control", §6; `research/audit-2026-09-27.md`. This is a
  limitation.


### Native-HLS `<video>` (and `<img>`) cannot send an Authorization header

- **What we do:** a media cookie scoped to `/api/v1/bytes/`, while API calls and hls.js use the header.
  `internal/core/api/media_cookie.go`.
- **Evidence:** D154, D216. This is a limitation that forced the design.


### iOS Safari with the microphone open sends call audio to the earpiece, quietly (WebKit 230902)

- **What we do:** headphones are the supported voice set-up (D63). This is listed for the device lab.
- **Evidence:** D188 [web]. Whether `navigator.audioSession` fixes it is unverified.


### Safari supports zstd content-encoding only from 26 (iOS fully from 26.3)

- **What we do:** gzip to clients; zstd only between node and core (D129).
- **Evidence:** D129 [web, caniuse 2026-09-21]. This is a limitation.


### Safari needs a copied HEVC picture tagged `hvc1` (FFmpeg's MP4 muxer writes `hev1` by default)

- **What we do:** `-tag:v hvc1` when copying HEVC. `internal/node/prepare/arguments.go:103-110`;
  `internal/node/chunks/arguments.go:84-86`; the comments at `internal/nodeproto/prepare.go:43-44` and
  `internal/nodeproto/chunks.go:49-50`.
- **Evidence:** tests `TestArguments_TagCopiedHEVCForSafariAndReEncodeTo8BitH264` and
  `internal/node/chunks/arguments_test.go:60`; commit "feat(prepare): the node prepares fMP4 HLS renditions with every audio track, standard and boosted, and serves them on its byte socket" (2026-09-25). **Cause unverified in the
  repo:** the tests cite cut-a §6, which has no hvc1 text, and no Safari failure is recorded. It is
  well-known Apple behaviour [web].

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


### Chrome on Android with processing off captures quietly ("quiet, sometimes silent, streams")

- **Symptom:** the partner was too quiet even at 400% receiver gain.
- **Cause:** [source] Chromium's own comment in `aaudio_stream_wrapper.cc` about the unprocessed preset.
- **What we do:** the comment notes "without gain control a voice is quiet". Levelling is a setting, and VQ23
  tests Chrome with levelling on and echo cancelling off.
- **Evidence:** [owner] 2026-10-07; `research/voice-apps.md` §0.2. **Unsure:** the code workaround is only a
  setting, and the default is still off (see 1.4).


### Chrome plays a remote WebRTC stream into Web Audio only while a media element also holds it

- **Symptom:** remote voice routed only through Web Audio is silent.
- **What we do:** a muted `Audio` element holds the stream.
  `web/packages/sdk/src/together-voice-browser.ts:78-111` (comment at `:79-82`).
- **Evidence:** the code comment only. Cause unverified; it is long-standing Chrome behaviour [web].


### Brave on Android refused the microphone

- **Evidence:** [owner] 2026-10-06/07; `research/voice-apps.md` synthesis row 3. The cause has not been
  researched, and there is no workaround. **Unsure** whether to include it yet.

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


### A browser plays only the original file's first audio track

- **What we do:** choosing any other track forces a prepare (reason `audio_track`, D230).
- **Evidence:** D230. This is a limitation (AudioTrackList is unusable in most browsers).


### Browsers other than Chrome play no Matroska, and none plays TrueHD, DTS or most AC-3

- **What we do:** remux to fMP4 and transcode the sound (D3, D178).
- **Evidence:** working-notes.md; `research/cold-start.md` §4.4. This is a limitation.


### A browser `WebSocket` cannot send an Authorization header

- **What we do:** the Watch Together socket authenticates in-band or by cookie (D154 bans credentials in URLs).
- **Evidence:** `research/audit-2026-09-27.md`. This is a limitation.


### Browsers cap a cookie's Max-Age at 400 days

- **What we do:** `internal/core/api/media_cookie.go:22-31`.
- **Evidence:** plan-deviations P20. [doc] Low priority.

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


### It never aborts a slow chunk when stepping down to level 0

- **Cause:** `abortAndSwitch` aborts only if `nextLoadLevel > 0` (`abr-controller.ts:429-433`). The intent is
  undocumented.
- **What we do:** our give-up may choose the lowest quality, with `frag.abortRequests()` and
  `FRAG_LOAD_EMERGENCY_ABORTED`. `web/packages/player/src/abr-choice.ts:111-117`;
  `web/packages/player/src/abr.ts:384-393`.
- **Evidence:** D265 §3 ("the lowest included, which hls.js never gives up for"); abr-controller.md:132-138.


### After an abandon, it resets the estimator to a nominal bitrate

- **Cause:** `abr-controller.ts:389-392` and `:456` replace the measured history with the lower level's
  BANDWIDTH.
- **What we do:** an overridden `registerListeners` keeps the built-in abandon check from running, and our
  give-up never calls `resetEstimator`. `web/packages/player/src/abr.ts:429-455`. This is not commented in the
  code.
- **Evidence:** abr-controller.md:128-131, :278; D265.


### There is no cache-hit filter: a chunk from the browser's cache reads as link speed (hls.js#5826, open)

- **Symptom:** #5826 reads a throttled 1 Mb/s link as 404 MB/s. Here, the film page's preloaded chunks are read
  again by Play.
- **What we do:** chunks loaded in under 10 ms are not counted, and paced chunks count only at 0.9 of the pace
  or less. `web/packages/player/src/abr-choice.ts:32,38,180-186`; `web/packages/player/src/abr.ts:468-472`.
- **Evidence:** D265 §7; https://github.com/video-dev/hls.js/issues/5826; `research/slow-links.md` §2.


### The `abrController` config type is nominal, so a custom controller must subclass

- **Cause:** `config.ts:402` types the field `typeof AbrController`, which has private members. A class written
  from scratch needs `as unknown as`, which our lint forbids.
- **What we do:** `class OrpheumAbrController extends AbrController` plus a subclass factory for each playback.
  `web/packages/player/src/abr.ts:379,503-512`.
- **Evidence:** abr-controller.md:18-24.


### The base constructor calls the virtual `registerListeners()` before the subclass's fields exist

- **Cause:** `abr-controller.ts:62`.
- **What we do:** listeners are kept in a module-level `WeakMap` keyed by the controller.
  `web/packages/player/src/abr.ts:365-372,429-453` (comment: "hls.js registers them from its own constructor,
  before the controller's fields exist").
- **Evidence:** abr-controller.md:174-180.


### `EwmaBandWidthEstimator` is exported as a type only

- **Cause:** `hls.ts:1360,1398` use `export type`, so a from-scratch controller cannot build hls.js's estimator,
  which hls.js's other controllers read.
- **What we do:** subclassing keeps the parent's `bwEstimator` (`web/packages/player/src/abr.ts:374-385`).
- **Evidence:** abr-controller.md:65-68. This is an API limitation.


### The docs understate the ABR controller's interface, and the setter echoes its own value back

- **Cause:** `docs/API.md` lists only `nextAutoLevel` and `destroy`. The core also calls `firstAutoLevel`,
  `forcedAutoLevel`, `bwEstimator` and `resetEstimator`. The level controller writes `nextAutoLevel` back with
  the value it just read on every fragment (`stream-controller.ts:297` → `level-controller.ts:706-709`).
- **What we do:** override all of them. An echo changes nothing; a different value is an outside force.
  `web/packages/player/src/abr.ts:162-165,474-489`.
- **Evidence:** abr-controller.md:55-61, :75. The docs disagree with the behaviour.


### A 425 is never retried, and loads give up after about 10 s by default

- **Cause:** the default `fragLoadPolicy` does not retry 4xx statuses and times out at about 10 s. As stated in
  the comment; the default values were not re-quoted from hls.js's source.
- **What we do:** our own `LoadPolicy`: a 30 s first-byte wait, 120 s for the load, and many retries every 1-2 s,
  with `shouldRetry` retrying 425. `web/packages/player/src/chunked.ts:26-33,65-99` (comment at `:84-87`).
- **Evidence:** D250 §6, D247 §10. **Unsure:** this is a reasonable default meeting our non-standard use of 425
  ("being made"), not a bug.


### A held request is counted as download time, so the estimate collapses

- **Cause:** [source] the sample is `total − min(ttfb, ttfbEstimate)`, and the TTFB EWMA barely weighs one long
  wait (`abr-controller.ts:534-541`, `ewma-bandwidth-estimator.ts:62-68`).
- **What we do:** a chunk that is not ready is answered at once with 425, never held (`chunk_hold_ms` defaults to
  0). `internal/core/api/chunks.go:35-39,196-201`.
- **Evidence:** D250 §5; `research/playback-in-practice.md` §3; suspected on [staging] 2026-10-06.


### An open (EVENT, no ENDLIST) playlist is treated as live and starts near its end

- **Symptom:** "a film whose prepare had made 46 s began at 40 s".
- **What we do:** `startPosition: 0`. `web/packages/player/src/hls.ts:191-197`.
- **Evidence:** [staging] 2026-10-06. [doc] This is live-playlist behaviour that surprises for a growing VOD.


### hls.js moves the playhead itself (gap jumps, stall nudges), and the browser reports each as a seek

- **Symptom:** a Watch Together host broadcast hls.js's own gap steps as user seeks ("each such step became a
  jump for everyone"). A held follower whose seek was still loading kept being moved, and re-sought forever.
- **Cause:** `BUFFER_SEEK_OVER_HOLE` and `BUFFER_NUDGE_ON_STALL` set `currentTime`, and they surface only as
  non-fatal ERROR events.
- **What we do:** those events are re-dispatched as `orpheumplayermoveditself`
  (`web/packages/player/src/hls.ts:108-122`). The follower ignores the next `seeking`
  (`web/packages/sdk/src/together-follow.ts:176-182`) and tolerates drift while its own seek loads (`:637-639`).
- **Evidence:** [owner] 2026-10-05.


### A paused player is never stepped over a gap at the start, so it never becomes ready

- **Symptom:** "Both players held at 0 with 0.04 s to 60 s loaded … held the group up for good."
- **What we do:** a held player is moved onto the first frame it loaded.
  `web/packages/sdk/src/together-follow.ts:640-645`.
- **Evidence:** [staging] 2026-10-06. The cause is stated in the comment, but hls.js's source is not cited.


### After an audio-track switch, the audio already decoded in the old mix plays on for 2-4 s

- **Cause:** hls.js flushes the buffer but not the decoder ("said plainly it does not flush the decoder").
- **What we do:** a seek in place after the switch. `web/packages/player/src/hls.ts:236-246`.
- **Evidence:** agent-failures A7 (2026-09-26); VQ2 ticked ("works within about a second now"); D217 §3. The first
  diagnosis, segment length, was wrong (an agent error).


### Before any measurement, `bandwidthEstimate` returns its configured default, with no "unknown"

- **Symptom:** a phone reported its link as 500, then 5000 kb/s (hls.js's starting values), and the core
  remembered that.
- **What we do:** `measuredKbps` is undefined until `FRAG_LOADED`. `web/packages/player/src/hls.ts:143-156`;
  `abr.ts:352`.
- **Evidence:** [staging] 2026-10-07. **Borderline:** this is a documented default.

---

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

---

## WebRTC


### Chrome and Safari use RED only when it is listed first

- **Cause:** [source] libwebrtc generates RED but places it after Opus.
- **What we do:** `setCodecPreferences` with RED first.
  `web/packages/sdk/src/together-voice-codec.ts:73-86`; `together-voice-browser.ts:114-128`.
- **Evidence:** D262 §3; `research/voice-bad-links.md` §3 "RED".


### No browser API caps the jitter buffer: `jitterBufferTarget` is a floor, not a ceiling

- **Symptom:** voice arrived about 1.1 s late under the emulated congested link, about 0.9 s of it in NetEQ.
  Signal caps NetEQ at 500 ms; a web page cannot.
- **What we do:** remove the jitter instead. The film is paced server-side and capped at half the link while
  voice is live (D262 §2; `internal/platform/pacing`).
- **Evidence:** D262; `research/voice-bad-links.md` §3 [measured + source].


### `networkPriority` (DSCP) exists in Chrome and Safari but not Firefox, and `setParameters` may refuse it

- **What we do:** set both priorities and swallow a rejection.
  `web/packages/sdk/src/together-voice-browser.ts:130-143`.
- **Evidence:** D262 §3; `research/voice-bad-links.md` §3. This is a capability difference.


### mDNS-obfuscated host candidates cannot be resolved inside a container

- **Symptom:** two pages in the test container cannot call each other.
- **What we do:** Chromium gets `--disable-features=WebRtcHideLocalIpsWithMdns`, and Firefox gets
  `media.peerconnection.ice.obfuscate_host_addresses=false`. `web/e2e/browsers.ts:3-19`.
- **Evidence:** D252 §3. This is an environment limitation.

---

---

## FFmpeg, x265/x264, codecs and containers


### x265 makes every keyframe after the first a CRA (open GOP)

This is the encoder half of 1.1. It is listed so the FFmpeg/x265 group is complete. Evidence: D266.


### The `segment` muxer misses a keyframe a hair before the cut time (10.009999 against 10.010)

- **What we do:** a copied picture is cut 50 ms early (as Kyoo's `hls_time_delta 0.05`).
  `internal/node/chunks/arguments.go:14-18,49-62`.
- **Evidence:** `research/slow-links.md` §10 [measured 2026-10-05]; test
  `TestArguments_ReadTheRunsChunksAndCutAtEachAndAtTheEnd`.


### The `hls` muxer cuts by cumulative time and cannot keep a copied picture to a keyframe grid

- **What we do:** the `segment` muxer with `-segment_times` (`internal/node/chunks/arguments.go:96-104`).
- **Evidence:** `research/slow-links.md` §10 [measured].


### Each output file starts its fMP4 times at zero, and upstream has no flag for absolute `tfdt`

- **Cause:** `frag_discont` does not do it, and `absolute_tfdt` exists only in Kyoo's fork.
- **What we do:** the node rewrites every `tfdt` itself. `internal/node/chunks/fmp4.go:121-152`;
  `maker.go:255`.
- **Evidence:** `research/slow-links.md` §10 [measured: 3,533 frames, no gap];
  `TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes`.


### fMP4 init segments differ between runs unless the edit list is off

- **What we do:** `movflags=+frag_keyframe+empty_moov+default_base_moof:use_editlist=0`
  (`internal/node/chunks/arguments.go:102`).
- **Evidence:** `research/slow-links.md` §10 [measured].


### The AAC encoder adds 1024 priming samples at each run's start

- **Symptom:** an 8.7 ms seam where two runs meet.
- **What we do:** a sound run starts one chunk early and drops it. `internal/node/chunks/maker.go:35-37,269-290`.
- **Evidence:** `research/slow-links.md` §10 [measured]; `TestMake_TimesASoundBySamplesAfterItsLeadIn`. [doc]


### Forced keyframes: x264 needs `-sc_threshold 0`, and hardware encoders need "forced IDR", spelt differently per encoder

- **Cause:** NVENC ignores `-force_key_frames` without `-forced-idr 1`. QSV's option is `-forced_idr 1`.
  Jellyfin does not trust `-force_key_frames` on QSV.
- **What we do:** `internal/node/prepare/encoder.go:37-79`.
- **Evidence:** `research/slow-links.md` §4; `research/playback-in-practice.md` §3 [web/source]; D190; commit
  An earlier commit. Unverified on our QSV hardware.


### `-hls_init_time` does nothing for copied video

- **Symptom:** the first segment is as long as the source's first GOP (*Flow*: 10.43 s with
  `-hls_init_time 1`).
- **What we do:** ladder from the start (D257, D260) and the keyframe grid from the file's index (D259).
- **Evidence:** `research/cold-start.md` §4.4-§5 [measured 2026-10-07]. **Unsure:** arguably expected.


### Chromaprint refuses a 5.1 input

- **Symptom:** `[af#0:0] Task finished with error: Invalid argument` / `[out#0/chromaprint] Nothing was
  written into output file`.
- **What we do:** downmix with `-ac 2` first (an `assumptions/` test, D111).
- **Evidence:** D115 [measured 2026-09-20].


### FFmpeg's AC-3/E-AC-3 decoder applies DRC by default (`drc_scale = 1`)

- **What we do:** DRC is a per-device preference (D83). Listed in `assumptions/`.
- **Evidence:** working-notes.md; Jellyfin #10148 [web].


### `-c copy` remux rewrites every Matroska cluster, and mkvpropedit moves grown elements to the end

- **Symptom:** the planned head+tail sampling hash broke on every tag edit.
- **What we do:** file identity is a hash of the media payload (D179).
- **Evidence:** working-notes.md [measured].


### FFmpeg reads an input name as a protocol, and reads stdin by default

- **What we do:** `-nostdin`, `-protocol_whitelist file -i file:<abs>`.
  `internal/node/execx/execx.go:46-67`.
- **Evidence:** cut-a §7. [doc] This is security hardening.


### ffprobe's output quirks

- **Cause:** numbers come as strings, and "N/A" appears. A Matroska stream's duration is only a `DURATION`
  tag. The AAC `profile` is "" when the container does not say. Stream 0 may be cover art.
- **What we do:** `internal/node/probe/parse.go:175-205`; `internal/core/decide/classify.go:32-34`; a
  disposition-1024 filter (`internal/store/queries/read_through.sql:15`,
  `internal/core/library/streams.go:16-18`).
- **Evidence:** code comments only. [doc] Low priority, and could be merged into one entry.


### A Matroska file's cues may list fewer keyframes than the picture has

- **What we do:** trust each cue, and read through a file without cues (D259 §2-3;
  `internal/node/container/doc.go:3-7`).
- **Evidence:** D259. [doc] This is a muxer's choice.

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


### netem's `limit` counts every packet it holds, including packets only being delayed (and on `lo`, both directions)

- **Symptom:** `limit 100`, meant as about a quarter-second queue, gave CUBIC a 472 ms smoothed RTT with no
  loss. The default limit of 1000 packets is about 2.4 s at 5 Mbit/s.
- **What we do:** netem `limit 100000`, with the queue bounded by a `tbf … latency` child. With `rate`, netem
  also folds propagation delay into the queue, so delay goes in netem and the bottleneck in tbf.
- **Evidence:** `research/network-profiles.md` :51-53, §3.1-§3.2, §8 item 3; `research/voice-bad-links.md`
  (`q_netem.c` `.limit = 1000`); an earlier commit. D263 §1.


### The loopback's 64 KB MTU and GSO make netem and tbf see 64 KB packets

- **Symptom:** "a TCP packet of the film held a voice packet behind it for a tenth of a second" at 5 Mbit/s.
  Voice was 1.1 s late under *congested*, and much of that was the emulation's fault.
- **What we do:** `./dev` runs `ip link set dev lo mtu 1500 gso_max_size 1500` as root when the container
  starts (`dev:206-217`, `LOOPBACK_MTU`). The acceptance refuses a loopback that has not been set this way.
- **Evidence:** `research/network-profiles.md` §3.5; `research/voice-bad-links.md` §6; D262 §1; commit
  An earlier commit [measured 2026-10-07].


### One qdisc on `lo` is one shared link for both directions and both browsers

- **Symptom:** 5 Mbit/s is about 2.5 Mbit/s for each viewer, an RTT counts the queue twice, and TURN-relayed
  voice crosses the link twice.
- **What we do:** profiles are sized for both clients, with `iceTransportPolicy: "all"`. Per-browser network
  namespaces are deferred (D263 §4).
- **Evidence:** `research/network-profiles.md` §3.4 [measured]. [doc] This is inherent to loopback, not a
  bug. **Unsure** whether it belongs.


### `tbf`'s `burst` must be at least rate/HZ, or the rate is never reached

- **What we do:** a 16-64 KB burst. **Evidence:** `research/network-profiles.md` :190-191. [doc] Low priority.


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


### iproute2's `ip` drops file capabilities, so `setcap` on `ip` is useless (`tc` keeps them)

- **What we do:** `setcap cap_net_admin+ep` on `tc` only (`deploy/Containerfile:109-115`). The loopback's
  MTU is set with `ip` as root via `docker exec --user 0` when the container starts (`dev:210`).
- **Evidence:** the `dev:210` comment; an earlier commit. **Cause unverified in the repo.** From general
  knowledge, `ip` calls `drop_cap()` (lib/utils.c) for non-root users unless CAP_NET_ADMIN is inheritable.


### netem orphans delayed packets (`skb_orphan_partial`), so TCP Small Queues does not hold the sender back

- **Evidence:** `research/network-profiles.md` §3.1 [source]. This is context; it forced no workaround.
  **Probably exclude.**

---

---

## Linux kernel and OS (other)


### A process in uninterruptible disk wait ignores even SIGKILL

- **What we do:** a run that outlives SIGKILL is "left running", listed as stuck, and its slot is freed.
  `internal/node/execx/execx.go:41-44,357-377`; `stuck.go:11-12`.
- **Evidence:** D233 §10. [doc]


### Pdeathsig fires when the starting *thread* ends, not the process

- **What we do:** the node never ends a thread of its own. `internal/node/execx/process_linux.go:7-13`.
- **Evidence:** the comment; `prctl(2)`. [doc] This is latent and was never hit.


### Opening a named pipe read-only blocks until a writer appears

- **Symptom:** a pipe placed where a media file was held a byte request and its goroutine forever.
- **What we do:** open with `O_NONBLOCK` before the regular-file check.
  `internal/node/confine/mediaroot.go:81-88`.
- **Evidence:** commit "fix(confine): opening a file under a media root never waits for a pipe's writer" (2026-09-29). [doc] POSIX behaviour.


### Whether Landlock is available depends on the kernel's boot configuration, and the errno says why

- **Symptom:** `landlock_create_ruleset(...) → -1, errno 95` in an ordinary container. D112 first blamed
  Docker, wrongly. Proxmox VE leaves Landlock out of `CONFIG_LSM`, and Synology's kernels predate it (ENOSYS).
- **What we do:** the errno is mapped to a cause and a remedy. `internal/node/confine/landlock_linux.go:95-118`;
  `deploy/staging/README.md:101-103`.
- **Evidence:** D112; `research/audit-2026-09-23.md` §4.1 [measured]; VQ9.


### x32 syscalls share AUDIT_ARCH_X86_64, so a seccomp denylist can be bypassed

- **What we do:** refuse every number at or above `x32Bit`. `internal/node/confine/arch_linux_amd64.go:13-23`.
- **Evidence:** commit "feat(node): every FFmpeg run is confined by Landlock and seccomp through a re-exec shim (cut-a §7)". [doc]


### The kernel's `rotational` flag is wrong behind a USB bridge

- **Symptom:** the owner's USB SSD on devnas read as spinning.
- **What we do:** prefer udev's `ID_ATA_ROTATION_RATE_RPM`, and treat a "spinning" disk behind USB as unknown.
  `internal/node/hardware/storage.go:210-230`.
- **Evidence:** D240 §5; [owner] VQ17, 2026-10-03.


### btrfs and ZFS mounts do not name a block device

- **What we do:** follow btrfs through `/sys/fs/btrfs` and ZFS through udev. `./dev` bind-mounts the host's
  `/run/udev`. `internal/node/hardware/storage.go:261-263,339-340`.
- **Evidence:** D240 §5. [doc]


### A UDP socket bound to every address replies from the routing table's choice of source

- **What we do:** TURN listens on the tunnel's address `10.77.0.2`. `internal/core/turn/doc.go:28-29`;
  `deploy/vps/README.md:121-124`.
- **Evidence:** VQ19 (the owner's VPS). [doc]


### Unix socket paths are limited to 108 bytes on Linux and 104 on macOS

- **What we do:** `maxSocketPathBytes = 103` (`internal/platform/unixsocket/listen.go:14-16`), plus
  `--run-dir`. **Evidence:** D242 §8. [doc]


### inotify does not work over SMB/NFS and runs out of watches silently

- **What we do:** periodic scans correct missed events (D27, D60). **Evidence:** working-notes.md [web].


### The NFS client's default timeout is 60 s, retransmitted twice

- **What we do:** a 3-minute prepare stall window. `internal/node/prepare/prepare.go:31-35`.
  **Evidence:** the comment. [doc] Low priority.

---

---

## Docker, hosts and the network path


### A container sharing another's network loses it when that container restarts alone

- **Symptom:** `./staging status` shows the core unhealthy after the tunnel's container restarted on its own.
- **What we do:** the tunnel never exits on failure and retries inside the same namespace
  (`deploy/staging/tunnel.sh:38-42`), and `./staging` recreates both containers. The behaviour is documented
  at `deploy/staging/README.md:94-98`.
- **Evidence:** the README. No date is given.


### Killing the `docker exec` client leaves the process running inside the container

- **Symptom:** a second `./dev make dev` ran two dev loops that fought over ports.
- **What we do:** `./dev --stop` removes the container.
- **Evidence:** an agent's working note (2026-10-02).


### The dev container had no `tc` and no right to shape its network, and CI's container lacks `NET_ADMIN`

- **What we do:** iproute2 in the image, `--cap-add NET_ADMIN` (`dev:130-131`), and `NET_ADMIN` for CI.
- **Evidence:** D252, D261 §2; `research/audit-2026-10-06.md` Q2. This is Docker's capability model.
  **Unsure:** it may be just configuration.


### Ubuntu's AppArmor profile for `wg-quick` refuses PostUp's sysctl and nftables, even as root

- **Symptom:** `permission denied` from PostUp.
- **What we do:** a sysctl.d file and a separate `orpheum-forwards.service`. `deploy/vps/vps-wg0.conf:8-10`;
  `deploy/vps/README.md:59-61`.
- **Evidence:** [owner] the VPS, 2026-10-06.


### ufw silently drops forwarded packets

- **What we do:** `ufw route allow` rules. `deploy/vps/README.md:63-79`. [doc]


### Docker's storage is invisible to `du` and grows without bound

- **Symptom:** the disk reached 97-99% several times; the Go build cache was 36 GB.
- **What we do:** prune (no workaround in the repo). **Evidence:** an agent's working note. **Unsure:** this is
  resource behaviour rather than a quirk.


### DS-Lite needs a smaller WireGuard MTU

- **Symptom:** "Large pages or videos stall from outside, small ones load."
- **What we do:** a tunnel MTU of 1380 over IPv4 and 1420 over IPv6, with 1280 as the fallback.
  `deploy/staging/README.md:54-55,92-93`; `deploy/staging/staging.toml:21-23`; `deploy/staging/tunnel.sh:17`.
- **Evidence:** an agent's working note; D243 §7. Whether 1380 was measured is unverified.


### DS-Lite: no public IPv4 and a changing IPv6 prefix

- **What we do:** a VPS, which the core dials out to over WireGuard (D243 §7).
- **Evidence:** an agent's working note; `research/direct-reachability.md`. This is an ISP limitation.

---

---

## SQLite, sqlc and Go's `database/sql`


### sqlc miscounts a multibyte character in a query file and reports an unrelated syntax error

- **Symptom:** `edited query syntax is invalid: extraneous input ')'` when a comment holds `§`.
- **What we do:** query files are ASCII-only, enforced by `scripts/check_source.py:20-22,57,101-112`.
- **Evidence:** an agent's working note; `docs/contributors/standards/sql.md:95-97`;
  `research/audit-2026-09-24.md` §4.3; plan-deviations P13 (it recurred).


### `sqlc.slice()` and untyped SQLite columns generate `interface{}`

- **What we do:** `IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))`, overrides, and
  `TestGeneratedQueries_HaveNoUntypedValues` (`internal/store/sqlcgen_test.go:15-17`).
- **Evidence:** sql.md:85-94; an agent's working note.


### sqlc cannot parse FTS5 `MATCH`, `LIKE … ESCAPE` or SQLite 3.53's new `ALTER` forms

- **What we do:** hand-written statements. `internal/store/search.go:30-31`.
- **Evidence:** sql.md:88-105; an agent's working note.


### `sqlc vet` sees a query after `*` is expanded, so a "no SELECT *" rule can never fire

- **What we do:** `TestArch_QueriesNameEveryColumn` reads the .sql files instead (`sqlc.yaml:66-67`).
- **Evidence:** sql.md:55.


### A pragma set by `Exec` reaches only one connection of the pool, and the pure-Go driver's DSN syntax differs from mattn's

- **What we do:** pragmas go in the DSN as `_pragma=…`, are read back on every pool, and Open refuses if they
  are wrong. `internal/store/open.go:26-33,162+` (`verifyPragmas`).
- **Evidence:** sql.md:165-172; `research/audit-2026-09-23.md` §4.2;
  `TestVerifyPragmas_RefusesAConnectionWithoutThem`. [doc]


### `SQLITE_BUSY_SNAPSHOT` shares "database is locked" with `SQLITE_BUSY`, but `busy_timeout` retries only the latter

- **What we do:** one writer with `_txlock=immediate` and a `query_only` reader pool (`internal/store/open.go:27-33`).
- **Evidence:** D31; sql.md:186-194; `research/sqlite-reconsidered.md` §2 [web, Navidrome].


### Copying a live WAL database's `.db` file gives a file that opens cleanly and has no tables

- **Symptom:** `no such table`.
- **What we do:** `VACUUM INTO` for backups, `os.Stat` before a restore, and refusing `?` in paths (the driver
  splits the DSN there).
- **Evidence:** D77, D242 §5 [measured].


### SQLite's limits: 1000 OR terms ("Expression tree is too large") and 64 tables in a join

- **What we do:** EXISTS subqueries with batched ORs (D75).
- **Evidence:** working-notes.md [measured on 3.46.1].


### FTS5 tokeniser gaps

- **Symptom:** unicode61 makes a CJK run one token and shreds Thai. Neither tokeniser folds Ø, Æ, Œ, ß, Ð or Ł,
  and the operators are case-sensitive.
- **What we do:** two indexes (unicode61 and trigram), alternates normalised in Go, and the user's operators
  lowercased (D48).
- **Evidence:** working-notes.md [measured].


### `modernc.org/libc` imports `os/exec`, which trips "the core never execs"

- **What we do:** exactly that import edge is exempted in `TestArch_CoreNeverExecs` (D199).
- **Evidence:** D199; `research/audit-2026-09-24.md` §4.5 [measured, M1].


### SQLite's locking is unreliable on network filesystems

- **What we do:** refuse the persistent directory on ten statfs magic numbers.
  `internal/core/instancedir/netfs_linux.go:10-42`.
- **Evidence:** D31; SQLite's FAQ. [doc]


### `COLLATE NOCASE` folds only ASCII, and temp files follow TMPDIR

- **What we do:** usernames are case-insensitive for ASCII only (the owner accepted this), and
  `temp_store(MEMORY)`. `internal/store/queries/user.sql:18-19`; `internal/store/open.go:39-41`.
- **Evidence:** [doc] Low priority.

---

---

## Go standard library and toolchain


### Go regexp's `\w`, `\d`, `\s` and `\b` are ASCII-only and fail silently on names like "Amélie"

- **What we do:** `\p{L}` and the like. Those classes are banned in the naming corpus, which has a non-Latin
  fixture. `internal/core/identify/filename.go:35-38`.
- **Evidence:** D49. [doc]


### `x/text/collate` is stuck at Unicode 6.2 and sorts Korean by code point (golang/go#19087)

- **What we do:** materialised sort keys, with Korean accepted as wrong (D47). Custom SQLite collations were
  rejected.
- **Evidence:** working-notes.md [measured].


### golangci-lint refuses to start beside another run (exit 3) and prints nothing on stdout

- **Symptom:** "parallel golangci-lint is running". A gate read the empty stdout as "no findings" and passed.
- **What we do:** `--allow-serial-runners` (Makefile:117-123), and
  `scripts/lint_speculative_api.py:110-123` fails on any exit status but 0 or 1.
- **Evidence:** commit "fix(gates): the linters wait for another run's lock, and a linter that could not run fails" (2026-09-30).


### The default HTTP transport reads HTTP(S)_PROXY, and Go downloads another toolchain unless `GOTOOLCHAIN=local`

- **What we do:** a client with no proxy (`internal/platform/outbound/client.go:12-14`, forbidigo), and
  `deploy/Containerfile:68-71`.
- **Evidence:** D204; `TestNewClient_TakesNoProxyFromTheEnvironment`. [doc] **Unsure:** these are design
  choices about documented behaviour.


### Go's standard library reads the process's TZ

- **What we do:** the process zone is kept UTC, and the zone is passed explicitly (D246 §7).
- **Evidence:** D246 §7. [doc]

---

## Test tooling: Playwright, Vitest, happy-dom, TypeScript


### Vitest strips types without type-checking, so a types-only change can never fail a test

- **What we do:** a green TypeScript revert is also type-checked with `tsc`.
- **Evidence:** `research/audit-2026-09-27.md` "Four blind spots". [doc]


### happy-dom does not carry a disabled `<fieldset>` to its controls

- **What we do:** the test asserts on the fieldset's own `disabled`. `web/apps/web/src/busy-group.test.tsx:16-30`.
- **Evidence:** D244 §4.


### Playwright grants "microphone" only on Chromium; Firefox needs prefs

- **What we do:** `web/e2e/together-pages.ts:189-200`; `web/e2e/browsers.ts:15-19`.
- **Evidence:** inferred from the code, which has no comment. Cause unverified.


### TypeScript checks no excess properties on a body built with a computed key or held in a variable

- **Symptom:** after the D227 renames the client compiled while still sending the old field names.
- **What we do:** bodies take their names from the generated types via `satisfies`.
- **Evidence:** agent-failures A12 (2026-09-29). [doc] **Borderline:** TypeScript works as designed.


### Playwright's browsers are not installed by `npm ci`

- **Evidence:** `research/audit-2026-09-29.md` §6. [doc] Low priority.

---

---

## Development tooling: git, npm, Vite, Python, procps, yoloAI


### Git hooks export GIT_DIR, so a test's `git init` set `core.bare` on the project repository

- **What we do:** strip `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE` and related variables.
  `scripts/py_tests.py:26-36`.
- **Evidence:** commit "feat(gates): a passing check-pr marks where it ended, and the parallel gate's two races are closed" (2026-10-01).


### `npm ci` through a symlinked `node_modules` empties the link's target

- **What we do:** a real install, or `cp -al`, in a worktree. revert-red links each package instead
  (`scripts/revert_red_trees.py:11-24`).
- **Evidence:** an agent's working note (2026-10-01). **Unsure:** our Makefile's mtime rule
  triggers the `npm ci`, but the emptying is npm's.


### Two npm installs at once race

- **What we do:** `web-deps` runs before the parallel steps (Makefile:52-53). The error text was not recorded.


### Vite's dev server misses a `git stash pop` and keeps serving old modules

- **Symptom:** a white screen in every browser.
- **What we do:** no stash or checkout under `make dev`; restart Vite alone (no workaround in the repo).
- **Evidence:** an agent's working note (2026-09-28). The cause is inferred.


### Python's bytecode cache is keyed on size and an mtime with one-second resolution

- **What we do:** `PYTHONDONTWRITEBYTECODE=1` for probes. `scripts/check_revert_red.py:511-513`. [doc]


### `pgrep -f` and `pkill -f` match the shell that runs them

- **Symptom:** a wait loop waited half an hour on its own command line, and a kill killed the calling shell.
- **What we do:** `ps | grep "[x]"` (no workaround in the repo). **Evidence:** an agent's working note. [doc]


### A trailing-slash `.gitignore` pattern (`node_modules/`) does not match a symlink

- **What we do:** the pattern has no slash. **Evidence:** an agent's working note. [doc]


### `yoloai apply` replays with `git am`, which stops on an empty patch

- **Symptom:** "Patch is empty".
- **What we do:** no empty commits; trailers ride on a real change.
- **Evidence:** an agent's working note (2026-10-02). **Unsure:** yoloAI is the owner's own tool. It is
  external to Orpheum, but perhaps not in scope.

---

---

## Cloudflare, ACME and DNS


### A Cloudflare-proxied record ends TLS, carries no UDP/TURN, and the terms restrict video

- **What we do:** the records are DNS-only, and the certificate comes by ACME DNS-01 through the API (D72,
  D243 §6). `deploy/vps/README.md:84-86`.
- **Evidence:** working-notes.md [web]. [doc]


### There is no publicly trusted certificate for a bare IP, and private-IP DNS answers trip rebinding protection

- **What we do:** one public name with DNS-01 (D243).
- **Evidence:** `research/remote-access.md` §4 [web]. This is a limitation.

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
