> **ABOUTME:** Why Signal and WhatsApp kept voice working on a link where Watch Together's broke up,
> read from RingRTC, libwebrtc and the browsers' source, and a ranked list of what a browser app can
> do. Gathered 2026-10-07. Feeds D262.

# Voice on bad links: what Signal and WhatsApp do, and what a browser can do

Research for Watch Together voice. Prepared 2026-10-07. Sources were read from primary code wherever
possible: RingRTC, libwebrtc, Chromium, WebKit and Firefox source, and the W3C specifications. Pages are
cached under `/yoloai/cache/http/voip/`, and RingRTC is cloned at `/yoloai/cache/git/ringrtc`.

**What was measured.** Under the `congested` profile (`delay 150ms 50ms loss 3% rate 5mbit` on `lo`, from
`web/e2e/acceptance-server.ts`), a host in Chrome and a guest in Firefox talked while both played the film.
Voice arrived about 1.1 s late. About 0.9 s of that was the receiver's jitter buffer, and 4.3% of samples
were concealed. The owner reports that Signal and WhatsApp had no problems on the same bad real-world
connection.

---

## Executive summary

**Codec settings are not where most of the gap comes from. Delay variation is.** NetEQ is the jitter
buffer in Chrome, Safari and Firefox, and Signal uses it too. It sets its target to roughly the 95th
percentile of how late packets arrive, compared with the fastest recent packet. It never caps that
target in a browser. Of the 1.1 s, about 0.2 s is netem's base delay plus packetisation. The other
~0.9 s is NetEQ covering delay *variation*. Two things generate that variation in our setup, and neither
was present in the owner's Signal or WhatsApp call:

1. **The film's download shares the bottleneck queue with the voice packets.** HLS fetches are bursty
   TCP. They fill whatever queue sits in front of the bottleneck, then go idle, so the voice packets
   behind them see swings of hundreds of milliseconds. Inside Signal and WhatsApp, every flow on the link
   is RTP under one delay-based congestion controller (googcc). Video backs off as soon as queueing delay
   grows, so the queue stays short. A Watch Together film is a TCP flow that the voice's congestion
   controller cannot see and cannot slow down.
2. **The emulation makes that much worse than a real link would**, in three ways. All three are checked
   against source:
   - **Oversized packets.** `lo` has an MTU of 65536 (`gso_max_size 65536`), so the TCP packets that
     reach netem are up to 64 KB each. One such packet takes about 105 ms to serialise at 5 Mbit/s, and
     a voice packet queued behind it waits that long.
   - **A very deep queue.** netem's default queue `limit` is 1000 packets
     ([iproute2 `q_netem.c`](https://github.com/iproute2/iproute2/blob/main/tc/q_netem.c):
     `struct tc_netem_qopt opt = { .limit = 1000 }`). Even at 1500-byte packets that is about 2.4 s of
     queue at 5 Mbit/s, far deeper than most real access links.
   - **Per-packet jitter that reorders.** `delay 150ms 50ms` gives every packet its own independent
     jitter of up to ±50 ms. With 20 ms voice packets that reorders them often, as the
     [tc-netem man page](https://man7.org/linux/man-pages/man8/tc-netem.8.html) says ("Packets will also
     get reordered if jitter is large enough"). NetEQ's reorder optimiser then raises its target further.
     Real networks rarely reorder (see the
     [webrtcHacks NetEQ article](https://webrtchacks.com/how-webrtcs-neteq-jitter-buffer-provides-smooth-audio/)).

   In addition, every flow in the test shares the one 5 Mbit/s queue on `lo`: both directions of voice,
   and both browsers' film downloads.

**Signal's audio settings** come from RingRTC's own defaults
(`src/rust/src/webrtc/media.rs`, `peer_connection_factory.rs`, `common/mod.rs`):

- **Opus:** 60 ms packets, constant bitrate fixed at 32 kbit/s, complexity 9, in-band FEC on, DTX on.
- **Loss tools:** Opus DRED plus neural packet-loss concealment were integrated in 2025–26, with the
  DRED duration chosen by the app. RED was tried as "lbred" and then removed.
- **Jitter buffer:** the target delay is capped at 500 ms, and the buffer holds at most 50 packets.
- **Bandwidth:** at most 2 Mbit/s for all media, or 300 kbit/s in "low data" mode.

WhatsApp's stack is closed. Meta says it used Opus everywhere, and in 2024 began rolling out its MLow
codec on WhatsApp, partly so that it can afford FEC at low bitrates.

### Ranked recommendations

| # | Recommendation | Expected effect | Browser support | Effort |
|---|---|---|---|---|
| 1 | **Measure before tuning.** (a) Log the receiver's own statistics: `jitterBufferTargetDelay`, `jitterBufferMinimumDelay`, `concealedSamples − silentConcealedSamples`, `packetsDiscarded` (arrived too late), `packetsLost`, `fecPacketsReceived`, `insertedSamplesForDeceleration`/`removedSamplesForAcceleration`, the candidate pair's `currentRoundTripTime`, and the local candidate's `relayProtocol`. (b) Make the emulation realistic: MTU 1500 / GSO and TSO off (or a veth pair in network namespaces), a rate stage with a realistic buffer (`tbf`/`cake bandwidth 5mbit`, or netem `limit` ≈ 100–250 ms of packets), and jitter with correlation, or netem `slot`, instead of independent ±50 ms per packet. (c) Add a control run with voice only, which is the fair comparison with the Signal test. | Separates how much of the 4.3% is late packets and how much is real loss, and how much of the 0.9 s is artefact. Likely shows most of the 1.1 s comes from the test setup plus the film's bursts, not from Opus settings. | All browsers. These are standard `RTCInboundRtpStreamStats` / `RTCIceCandidateStats` fields. | Low |
| 2 | **Stop the film's download from filling the shared queue while voice is live.** Server side: pace each HLS response to about 1.3–1.5× the rendition's bitrate. In Go on Linux that is `setsockopt(SO_MAX_PACING_RATE)` on the connection ([socket(7)](https://man7.org/linux/man-pages/man7/socket.7.html), [tc-fq(8)](https://man7.org/linux/man-pages/man8/tc-fq.8.html)), or a token-bucket writer. Client side: while voice is connected, cap the film's rendition at about half the measured link speed. Step it down further when the voice statistics show `jitterBufferTargetDelay` or round-trip time climbing. Keep the forward buffer modest, so the film is fetched in small, steady pieces rather than big bursts. | **The largest real-world gain.** It removes the on/off bursts that drive NetEQ's 95th-percentile target. It is the browser's equivalent of what one congestion controller over all media gives Signal and WhatsApp. | All browsers, including iOS. Pacing is entirely server side. | Medium |
| 3 | **Turn on RED (RFC 2198 `audio/red`)** by putting `red/48000/2` before `opus` with `RTCRtpTransceiver.setCodecPreferences`. Chrome and Safari offer it but use it only when it is listed first. Firefox does not offer it, so negotiation falls back to plain Opus on its own. | With independent 3% loss, one redundant copy leaves roughly 0.1% unrecovered. In the webrtcHacks test at 20% loss, concealment fell from 18% to 4%. Costs about +30 kbit/s, which is negligible next to the film. Does nothing for delay. | Chrome and Edge. Safari, judging from WebKit's libwebrtc source. **Not Firefox.** | Low |
| 4 | **Set explicit Opus parameters** in the SDP the remote side reads: keep `useinbandfec=1` (the default), add `usedtx=1` (as Signal does), and set `maxaveragebitrate` to 32000–40000 so in-band FEC has room. Optionally add `a=ptime:40` or `a=ptime:60`. That cuts overhead from ~50 to ~17–25 packets/s, which matters most over TURN-TCP, at the cost of +20–40 ms delay. Do not lower the bitrate. | Small. In-band FEC only starts after loss is reported, and the loss estimate is smoothed over a 10 s time constant. A longer ptime helps on constrained or relayed links but makes each lost packet a larger gap. | Chrome and Safari (libwebrtc) honour these. Firefox honours `useinbandfec`/`maxaveragebitrate`; check `ptime` before relying on it. | Low (SDP munging) |
| 5 | **Mark the traffic for home routers.** On the audio sender: `setParameters` with `encodings[0].priority = "high"` and `networkPriority = "high"`, which marks packets EF (DSCP 46, [RFC 8837](https://www.rfc-editor.org/rfc/rfc8837)). On the server: mark HLS sockets LE (DSCP 1, `IP_TOS` 0x04, [RFC 8622](https://www.rfc-editor.org/rfc/rfc8622)). | Helps only where a router or Wi-Fi access point honours DSCP. Examples: `cake diffserv3/4` puts EF in the Voice tin and LE in the Bulk tin, and Wi-Fi WMM has a voice access category. ISPs usually strip the marks. Free where it works, harmless elsewhere. | `networkPriority` exists in Chrome and Safari only. Firefox has `priority` but no `networkPriority`. | Low |
| 6 | **Keep the relay on UDP.** Offer `turn:…?transport=udp` first and TCP/TLS on 443 only as a fallback. Prefer direct paths, as Signal does ("Increase priority of non-relay candidates", RingRTC v2.49.0). Put the VPS relay close to both households. Log `relayProtocol`, so a call that fell back to TCP is visible. | Over TCP, one lost packet holds back every packet behind it for at least a round trip (head-of-line blocking), and NetEQ reads that as jitter. On a lossy link, TURN over TCP alone can add hundreds of milliseconds. | All browsers. | Low |
| 7 | **Do not use `jitterBufferTarget` to cut delay.** Chrome maps it to NetEQ's *minimum* delay, so it can only add delay. No standard API caps the jitter buffer as Signal's 500 ms cap does. Chrome's non-standard `rtcAudioJitterBufferMaxPackets` is gated as "experimental" behind an origin trial and is not deployable. | Avoids an action that does nothing (or harm). Cut delay by removing jitter instead (#2). | `jitterBufferTarget`: Chrome 124, Firefox 115, Safari 27 (BCD). | — |
| 8 | **Not for Cut A: DRED, MLow-style low-bitrate FEC, or neural concealment.** No browser ships them. Chromium's and Firefox's Opus builds leave out the neural (DNN) code. The only route would be Opus 1.5 compiled to WebAssembly, carried over an unreliable DataChannel or WebTransport, with our own jitter buffer and playout through WebAudio. In other words, replacing WebRTC's audio path. | Large under heavy or bursty loss (DRED carries up to 1 s of redundancy per packet). Barely relevant at 3% random loss. | Would work anywhere WebAssembly and AudioWorklet run. | Very high |

Two notes on the acceptance bound. D243 §9 and m4.md Phase H ask for one-way delay under 300 ms and
audible loss under 2%. Once the film is paced and the emulation is fixed (#1 and #2), that is plausible.
Opus settings alone (#3 and #4) cannot meet it while the test link carries 64 KB packets through a
1000-packet queue.

---

## 1. Signal (RingRTC)

RingRTC wraps Google's libwebrtc; RingRTC v2.72.1 is built on WebRTC `7871n`. The values below are read
from the cloned source at `/yoloai/cache/git/ringrtc`, from [github.com/signalapp/ringrtc](https://github.com/signalapp/ringrtc).

### Opus encoder (`src/rust/src/webrtc/media.rs`, `impl Default for AudioEncoderConfig`)

| Setting | Signal default | Browser default (libwebrtc) |
|---|---|---|
| Packet time (ptime) | **60 ms** (initial, minimum and maximum all 60) | 20 ms (`kDefaultFrameSizeMs = 20`) |
| Bitrate | **32 kbit/s**, minimum = maximum = 32000, **CBR on** | 32 kbit/s, fixed by default (min = max = target in `webrtc_voice_engine.cc`), VBR |
| Complexity | 9 | 9 on desktop, 5 on Android/iOS (`audio_encoder_opus_config.h`) |
| In-band FEC (LBRR) | on | on (`useinbandfec=1` in the default fmtp) |
| DTX | **on** | off (`usedtx` absent) |
| DRED | `dred_duration` set by the app (0–100, in 10 ms units) | not available |
| Bandwidth | Auto | Auto (`maxplaybackrate` 48000) |

The RingRTC changelog (`CHANGELOG.md`) records:

- **Packet time:** "Set audio packet time to 60ms" (v2.27.0), then "Adjust max audio jitter buffer size
  to support increased packet time" (v2.28.1).
- **Jitter buffer:** "Configure audio jitter buffer max delay" (v2.29.0) and "Allow configuration of audio
  jitter buffer max target delay" (v2.32.0).
- **RED:** "Add low bitrate redundancy support" (v2.34.3), then "Remove lbred experiment" (v2.44.0) and
  "Remove RED support" (v2.49.2). No reason is given.
- **Opus versions:** "Update to use Opus 1.5" (v2.40.0) and "Update to opus 1.6.1" (v2.65.3).
- **Neural tools:** "Enable Opus PLC" and "Add opus DRED model 0.0.1 and DRED duration config" (v2.66.0),
  then "Integrate Opus DRED" (v2.68.1).
- **Relays and TCP:** "Increase priority of non-relay candidates" (v2.49.0), and for group calls "Allow
  connection to a TCP+TLS server candidate".

### Jitter buffer (`peer_connection_factory.rs`, `AudioJitterBufferConfig::default`)

`max_packets: 50, min_delay_ms: 0, max_target_delay_ms: 500, fast_accelerate: false`. Android passes
its own `audioJitterBufferMaxPackets` / `audioJitterBufferMaxTargetDelayMs`
(`src/android/api/org/signal/ringrtc/CallManager.java`). The **500 ms cap on NetEQ's target delay**
is the key difference from a browser. Under heavy jitter, Signal accepts more late, concealed audio rather
than letting delay grow without limit. In browsers the only bound is 200 packets
(`audio_jitter_buffer_max_packets = 200` in `media_config.h`), which is 4 s at 20 ms.

### Bandwidth and video (`common/mod.rs`, `DataMode`)

The cap for all media is 2 Mbit/s in Normal mode and 300 kbit/s in Low mode. All media runs over one
transport with googcc and transport-cc, so video yields as soon as queueing delay rises. Signal's
[group-calls engineering post](https://signal.org/blog/how-to-build-encrypted-group-calls/) explains why
they chose googcc: "TCP's congestion control algorithms generally don't work well for video calls because
they tend to cause increases in latency".

### Relays

1:1 calls go direct by default. "Always relay calls", the `hide_ip` setting, forces the TURN path
(`RffiPeerConnectionKind::Relayed` in `native.rs`). Calls use UDP, with TCP/TLS only as a fallback.

### What users report

Public accounts are mostly anecdotal. One HN commenter, walrus01 in 2021: Signal "uses opus by default…
audio quality on Signal voice calls is uniformly excellent, unless one person's end is experiencing very
high packet loss" ([HN 26575762](https://news.ycombinator.com/item?id=26575762)). Another, trome in 2017,
on Signal video: "shut off video if you get into a low bandwidth situation, as opus can only cover up up
to 30% packet loss, and video shoved down too small a pipe can induce much more packetloss"
([HN 13646238](https://news.ycombinator.com/item?id=13646238)). That is the same failure as ours, a big
competing flow, seen from inside a single app.

## 2. WhatsApp (Meta)

- **Opus, then MLow.** Meta "has used Opus for all its RTC needs". In 2024 it shipped **MLow**, a
  low-complexity CELP-family codec, to all Instagram and Messenger calls and began rolling it out on
  WhatsApp. It claims POLQA MOS 3.9 against Opus's 1.89 at 6 kbit/s wideband
  ([Meta Engineering, 2024-06-13](https://engineering.fb.com/2024/06/13/web/mlow-metas-low-bitrate-audio-codec/)).
- **Why it matters for loss.** Low-bitrate quality makes FEC affordable: "Opus is not able to encode any
  inband FEC [at 14 kbps]. It needs a minimum of 19 kbps to encode any inband FEC at 10 percent packet
  loss". Meta says it is "continuing to work on improving the audio recovery in heavy packet loss
  networks by pumping out more redundant audio".
- **Who it is built for.** Meta notes that "a significant chunk of calls have poor network connections",
  and that video on a poor link "pushes the audio bit[rate] down". Its stack is built for bandwidth-starved
  links on old phones, which is not our problem: we have bandwidth, but it is shared with a bursty TCP flow.
- WhatsApp's jitter buffer, congestion control and relay design are not public. Calls go through Meta's
  relays when direct paths fail or when users choose to hide their IP address.

## 3. The browser's audio path: libwebrtc defaults that matter

### In-band FEC (Opus LBRR)

The browser offers `minptime=10;useinbandfec=1`
([`audio_encoder_opus.cc`](https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/codecs/opus/audio_encoder_opus.cc)).
The packet-loss hint (`OPUS_SET_PACKET_LOSS_PERC`) comes from RTCP-reported loss. It is passed through an
exponential filter with a **10 s time constant**, capped at 20%. libopus only spends bits on LBRR when the
hinted loss and the bitrate allow it. Fippo's analysis adds three points
([webrtcHacks, RED](https://webrtchacks.com/red-improving-audio-quality-with-redundancy/)): FEC switches on
only after loss has been reported, its rate is subtracted from the main stream's bitrate, and at 20% loss
it barely moved the concealment metric (18% with it, 20% without).

### RED (RFC 2198)

libwebrtc always generates `red/48000/2` with fmtp `111/111`, placed after Opus
([`webrtc_voice_engine.cc`](https://webrtc.googlesource.com/src/+/refs/heads/main/media/engine/webrtc_voice_engine.cc)).
"RED codec needs to be negotiated before the actual codec", so the app must reorder the codecs.

Redundancy today defaults to **one** previous frame (`kRedNumberOfRedundantEncodings = 1` in
[`audio_encoder_copy_red.cc`](https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/codecs/red/audio_encoder_copy_red.cc)).
Two or more frames need the `WebRTC-Audio-Red-For-Opus/Enabled-N/` field trial, which a web app cannot set.

Fippo measured at 20% random loss ([webrtcHacks](https://webrtchacks.com/red-improving-audio-quality-with-redundancy/)):

| Configuration | Concealed |
|---|---|
| No RED | 18% |
| RED, distance 1 | 4% |
| RED, distance 2 | 0.7% |

Jitsi's SFU work is described in
[Implementing REDundant audio on an SFU](https://webrtchacks.com/implementing-redundant-audio-on-an-sfu/).

Support by browser:

- **Safari:** WebKit's libwebrtc contains the same RED generation code
  ([WebKit `webrtc_voice_engine.cc`](https://github.com/WebKit/WebKit/blob/main/Source/ThirdParty/libwebrtc/Source/webrtc/media/engine/webrtc_voice_engine.cc)).
- **Firefox:** its default audio codecs are Opus, G722, PCMU, PCMA and telephone-event. RED appears only
  for video ([`DefaultCodecPreferences.cpp`](https://github.com/mozilla-firefox/firefox/blob/main/dom/media/webrtc/jsapi/DefaultCodecPreferences.cpp)).

### DRED and neural concealment

Opus 1.5 DRED sends up to 1 s of redundancy in every packet, at about 12–32 kbit/s extra. It needs the
jitter buffer to cooperate, and its bitstream "is not yet standardized"
([Opus 1.5 release notes](https://opus-codec.org/demo/opus-1.5/)). Xiph's DRED patches live in a
`webrtc-opus-ng` fork.

Neither browser enables it. Chromium's `third_party/opus/BUILD.gn` compiles no `dnn/` sources and defines
neither `ENABLE_DRED` nor `ENABLE_DEEP_PLC`. Firefox's `media/libopus/moz.build` defines neither.

### NetEQ (the jitter buffer)

Source: [`delay_manager.h`](https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/neteq/delay_manager.h),
[`delay_manager.cc`](https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/neteq/delay_manager.cc),
and the [webrtcHacks NetEQ deep-dive](https://webrtchacks.com/how-webrtcs-neteq-jitter-buffer-provides-smooth-audio/).

- **How the target is set.** It starts at 80 ms. It then follows the **0.95 quantile** of each packet's
  delay relative to the fastest packet in a 2 s window (forget factor 0.983).
- **Reordering.** A reorder optimiser is on by default. It weighs delay against loss at 20 ms per 1% of
  loss. This is why netem's reordering jitter inflates the target.
- **How fast it shrinks.** Coming down is slow: per webrtcHacks, about 3.5 s to drop from a 140 ms target
  to 20 ms at 20 ms packets. NetEQ shrinks the buffer by speeding playback up ("accelerate").
- **No upper cap.** Nothing limits the target except `max_packets_in_buffer` (200 in Chrome). Chrome's
  `jitterBufferTarget` calls `SetJitterBufferMinimumDelay`
  ([`rtc_rtp_receiver.cc`](https://chromium.googlesource.com/chromium/src/+/main/third_party/blink/renderer/modules/peerconnection/rtc_rtp_receiver.cc)),
  so it is a floor, not a ceiling ([MDN](https://developer.mozilla.org/en-US/docs/Web/API/RTCRtpReceiver/jitterBufferTarget)).
- **Chrome's non-standard knobs.** `rtcAudioJitterBufferMaxPackets`, `rtcAudioJitterBufferFastAccelerate`
  and `rtcAudioJitterBufferMinDelayMs` exist in `RTCConfiguration`, but sit behind the experimental
  `RtcAudioJitterBufferMaxPackets` feature and its origin trial (`runtime_enabled_features.json5`).
  Not usable in production.
- **Neural concealment exists elsewhere, not in browsers.** Google's Duo replaced NetEQ's concealment with
  WaveNetEQ, because classic concealment "does not sound good when … gaps of 60ms or more"
  ([Google Research](https://research.google/blog/improving-audio-quality-in-duo-with-waveneteq/)).

### Bitrate and congestion control

In current libwebrtc the audio stream's minimum and maximum bitrate both equal its target (32 kbit/s, or
`maxaveragebitrate`), so bandwidth estimation does not squeeze the audio. Bandwidth estimation and
`priority` only share bandwidth *among a peer connection's own RTP streams*. Our film is HTTP outside the
peer connection, so `priority`/`bitrate_priority` cannot protect voice from it.

Also, when `networkPriority` is above `low`, Chrome leaves the audio out of the allocation
(`has_dscp` in `audio_send_stream.cc`). That is harmless for voice-only.

### Priority and DSCP

- **The specification** defines `priority` (relative bitrate and queueing within the browser) and
  `networkPriority` (DSCP only) ([W3C webrtc-priority](https://w3c.github.io/webrtc-priority/)).
  For audio, RFC 8837 maps "medium" and "high" to EF 46 and "very-low" to LE 1.
- **Chrome** maps `networkPriority` to libwebrtc's `network_priority`
  ([`rtc_rtp_sender.cc`](https://chromium.googlesource.com/chromium/src/+/main/third_party/blink/renderer/modules/peerconnection/rtc_rtp_sender.cc)).
  Chrome also exposes the non-standard `adaptivePtime`.
- **Safari's** IDL has `networkPriority`
  ([WebKit `RTCRtpEncodingParameters.idl`](https://github.com/WebKit/WebKit/blob/main/Source/WebCore/Modules/mediastream/RTCRtpEncodingParameters.idl)).
- **Firefox's** IDL has `priority` and `maxBitrate` only
  ([`RTCRtpParameters.webidl`](https://github.com/mozilla-firefox/firefox/blob/main/dom/webidl/RTCRtpParameters.webidl)).
- DSCP takes effect only on networks that honour it. Microsoft's Teams guidance assumes managed networks
  ([Teams QoS](https://learn.microsoft.com/en-us/microsoftteams/qos-in-teams)).
- At home, `cake diffserv4` has these tins: Bulk (CS1 and LE) at 6.25% threshold, Best Effort, Video,
  and Voice (EF and others) at 25% ([tc-cake(8)](https://man7.org/linux/man-pages/man8/tc-cake.8.html)).

### Browser support (MDN browser-compat-data, fetched 2026-10-07)

| API | Chrome | Firefox | Safari |
|---|---|---|---|
| `RTCRtpSender.setParameters` | 68 | 64 | 11 |
| `encodings[].maxBitrate` | 69 | 46 | 11 |
| `encodings[].priority` | 69 | 46 (default `low` from 110) | 11 |
| `encodings[].networkPriority` | yes (source) | no (IDL) | yes (IDL) |
| `encodings[].dtx` | no | no | 11 |
| `RTCRtpTransceiver.setCodecPreferences` | 76 | 128 | 13.1 |
| `RTCRtpReceiver.jitterBufferTarget` | 124 | 115 | 27 |
| `audio/red` send and receive | yes | **no** | yes (WebKit libwebrtc source) |
| Opus DRED / deep PLC | no | no | no evidence |

All browsers on iOS use WebKit, so the Safari column applies to them.

## 4. Transport: UDP, TCP or TLS TURN, and where the relay sits

- **UDP is preferred.** TURN supports TCP and TLS "because some firewalls are configured to block UDP
  entirely", and "it is expected that most clients will prefer to use UDP"
  ([RFC 8656 §3.1](https://www.rfc-editor.org/rfc/rfc8656)).
- **TCP stalls the queue behind a loss.** When media rides TCP, a lost segment holds back everything
  behind it until it is retransmitted, at least one round trip. At 3% loss and 300 ms RTT that is a
  constant stream of 300+ ms stalls, which NetEQ turns straight into buffer delay. Keep a UDP relay, and
  use TCP/TLS on 443 only as a fallback.
- **Watch which path a call took.** `relayProtocol` on the local candidate, and `currentRoundTripTime` on
  the selected pair, show a call that fell back.
- **Relay placement matters at scale.** Skype's "Via" study (SIGCOMM 2016, 88M calls) found that choosing
  relays well cut poor-network incidents by up to 66%
  ([Microsoft Research](https://www.microsoft.com/en-us/research/publication/via-improving-internet-telephony-call-quality-using-predictive-relay-selection/)).
  For two households, put the VPS in the same region as both and prefer the direct path.

## 5. What a web app can do about the film sharing the link

The browser gives a page no way to rank its own HTTP fetches below its WebRTC traffic on the network.
`fetchpriority` only orders requests inside the browser. These levers are left:

1. **Server pacing**, the strongest. Linux TCP paces each socket, and `SO_MAX_PACING_RATE` caps that rate
   ("an application can specify a maximum pacing rate using the SO_MAX_PACING_RATE setsockopt";
   since Linux 4.20 TCP sets departure times itself, [tc-fq(8)](https://man7.org/linux/man-pages/man8/tc-fq.8.html)).
   - Pacing segments at about 1.3–1.5× the rendition's bitrate turns on/off bursts into a steady flow
     that never fills the queue.
   - While voice is live, cap the rendition at roughly half the link, so the pacing rate stays well below
     the link speed.
   - Watch the interaction with ABR: the throughput hls.js measures will equal the pacing rate, so it must
     stay above `level.bitrate / abrBandWidthFactor` (0.95 by default).
2. **Rendition cap and smaller fetch bursts on the client.**
   - Limit the rendition while voice is connected (hls.js `autoLevelCapping`, or our own step-down logic).
   - Use a smaller `maxBufferLength` (hls.js default 30 s) and `maxMaxBufferLength`, so each refill is a
     short burst.
   - React to voice health: `jitterBufferTargetDelay` rising above ~150 ms, or RTT rising, steps the film
     down. That mirrors what googcc does for video in Signal.
3. **Marking.** Set LE (DSCP 1) on the server's HLS sockets and EF on the voice (`networkPriority:"high"`).
   It helps only on routers that honour it, and costs nothing.

## 6. Emulation fixes, so the test resembles the owner's link

- **Packet size.** Shape a veth pair between network namespaces with MTU 1500, or set
  `ip link set lo mtu 1500` and turn off GSO/TSO, so netem sees packets the size a real link carries.
- **Rate and buffer.** Separate them from delay, for example `tbf rate 5mbit burst 32kbit latency 100ms`,
  or `cake bandwidth 5mbit` for an un-bloated router, with netem for delay and loss. Alternatively set
  netem `limit` to about 100–250 ms of packets (about 40–100 packets at 5 Mbit/s). Run a deliberately
  bloated variant too, because real routers are often bloated.
- **Jitter.** Use correlated jitter (`delay 150ms 20ms 50%`), or `pfifo` as netem's child queue to stop
  reordering, or netem `slot` for Wi-Fi/LTE-style bursts, instead of independent ±50 ms per packet.
- **Control run.** Run voice alone under the same profile. That is the comparison that matches the
  owner's Signal and WhatsApp test.

## Source index

**Code**

- RingRTC, v2.72.1: <https://github.com/signalapp/ringrtc> — `src/rust/src/webrtc/media.rs`,
  `src/rust/src/webrtc/peer_connection_factory.rs`, `src/rust/src/common/mod.rs`, `CHANGELOG.md`.
- libwebrtc: `media/engine/webrtc_voice_engine.cc`, `modules/audio_coding/codecs/opus/audio_encoder_opus.cc`,
  `api/audio_codecs/opus/audio_encoder_opus_config.h`, `modules/audio_coding/codecs/red/audio_encoder_copy_red.cc`,
  `modules/audio_coding/neteq/delay_manager.{h,cc}`, `api/neteq/neteq.h`, `api/rtp_receiver_interface.h`,
  `api/rtp_parameters.h`, `audio/audio_send_stream.cc`, `media/base/media_config.h` —
  <https://webrtc.googlesource.com/src/>
- Chromium: `third_party/blink/renderer/modules/peerconnection/rtc_rtp_sender.cc`, `rtc_rtp_receiver.cc`,
  `rtc_peer_connection.cc`, `runtime_enabled_features.json5`, `third_party/opus/BUILD.gn` —
  <https://chromium.googlesource.com/chromium/src/>
- WebKit: `Source/ThirdParty/libwebrtc/.../webrtc_voice_engine.cc`, `RTCRtpEncodingParameters.idl` —
  <https://github.com/WebKit/WebKit>
- Firefox: `dom/media/webrtc/jsapi/DefaultCodecPreferences.cpp`, `dom/webidl/RTCRtpParameters.webidl`,
  `media/libopus/moz.build` — <https://github.com/mozilla-firefox/firefox>
- iproute2 `tc/q_netem.c` (default `limit = 1000`).

**Articles and specifications**

- webrtcHacks, RED: <https://webrtchacks.com/red-improving-audio-quality-with-redundancy/>
- webrtcHacks, RED on an SFU: <https://webrtchacks.com/implementing-redundant-audio-on-an-sfu/>
- webrtcHacks, NetEQ: <https://webrtchacks.com/how-webrtcs-neteq-jitter-buffer-provides-smooth-audio/>
- Meta, MLow: <https://engineering.fb.com/2024/06/13/web/mlow-metas-low-bitrate-audio-codec/>
- Opus 1.5 (DRED, deep PLC, LACE/NoLACE): <https://opus-codec.org/demo/opus-1.5/>
- Signal, encrypted group calls and googcc: <https://signal.org/blog/how-to-build-encrypted-group-calls/>
- Google, WaveNetEQ: <https://research.google/blog/improving-audio-quality-in-duo-with-waveneteq/>
- Discord voice architecture: <https://discord.com/blog/how-discord-handles-two-and-half-million-concurrent-voice-users-using-webrtc>
- W3C webrtc-priority: <https://w3c.github.io/webrtc-priority/>
- MDN `jitterBufferTarget`: <https://developer.mozilla.org/en-US/docs/Web/API/RTCRtpReceiver/jitterBufferTarget>
- MDN `setParameters`: <https://developer.mozilla.org/en-US/docs/Web/API/RTCRtpSender/setParameters>
- MDN browser-compat-data: <https://github.com/mdn/browser-compat-data> (`api/RTCRtpSender.json`, `api/RTCRtpReceiver.json`, `api/RTCRtpTransceiver.json`)
- RFC 8837 (DSCP for WebRTC), RFC 8622 (LE), RFC 8656 (TURN), RFC 7587 (Opus RTP) — <https://www.rfc-editor.org/>
- Man pages: tc-netem(8), tc-fq(8), tc-cake(8), socket(7) — <https://man7.org/linux/man-pages/>
- Microsoft Teams QoS: <https://learn.microsoft.com/en-us/microsoftteams/qos-in-teams>
- Via (Skype relay selection): <https://www.microsoft.com/en-us/research/publication/via-improving-internet-telephony-call-quality-using-predictive-relay-selection/>

**Limits of this research.** The web-search quota ran out at the start, so this report rests on primary
code, specifications and pages I could fetch directly. WhatsApp's internals beyond Meta's MLow post are
not public. Two things are inferred from WebKit's source rather than from release notes: Safari's RED
behaviour, and which version first shipped it. Firefox's handling of `ptime` was not checked.
