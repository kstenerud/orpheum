> **ABOUTME:** An independent review of why the WebRTC path loads 10× slower than HTTPS on the
> Fire TV: what experts and the sources say a data channel should carry, what pion and Chrome
> actually do, new measurements of a pion sender against Chromium (round trip, pacing, socket
> buffer, starved receiver, processor cost per byte), what in the first write-up holds and what
> does not, and fixes ranked by expected gain.

# WebRTC Throughput: an Independent Review

- **Date:** 2026-10-28. Asked by the owner, who did not accept the first write-up's conclusion
  ([webrtc-throughput-2026-10-28.md](webrtc-throughput-2026-10-28.md)): "I could understand 1/3
  slower or even 50% slower, but 10x slower feels like it's the wrong conclusion."
- **Scope:** measurement and reading only. No product code changed. The throwaway bench (a pion
  sender shaped like `rtcapi`, a page, a Playwright driver, netem and cgroup scripts, raw results)
  is kept in the sandbox cache at `/yoloai/cache/webrtc-review/bench/`, with every fetched source
  under `/yoloai/cache/webrtc-review/`.

## 1. The answer

**The 10× is real, and it has two causes stacked on each other. One is ours and fixable for
about 2–3×. The other belongs to Chrome's data-channel receive path and will not go away.**

1. **Chrome's receive path costs about 15× the processor time per byte of an HTTPS download**
   (measured here, §4.4). Every 1,191-byte SCTP packet is read from its own UDP socket by the
   network service, sent over IPC to the renderer, DTLS-decrypted, and handled by dcSCTP in
   userspace. HTTPS gets kernel TCP, receive offload and 16 KiB TLS records. On a desktop this
   cost is hidden. On a Fire TV Stick (quad-core 1.7 GHz MediaTek MT8695D, 1 GB RAM, per
   [Amazon's specifications](https://developer.amazon.com/docs/device-specs/device-specifications-fire-tv-streaming-media-player.html)),
   which is also decoding the film and running the page, it is the ceiling. **Even a
   well-configured data channel should be expected to run several times slower than HTTPS on
   that device, not 1/3 slower.**
2. **The waste is a 64 KiB socket plus loss-based congestion control.** Chrome opens every WebRTC
   UDP socket with a **64 KiB receive buffer**. That is a constant in Chromium's source, and here
   the kernel reported it as `rb131072`. It holds about 50 packets, about 5 ms of data at
   100 Mb/s. pion sends its whole congestion window back to back. Whenever the receiving thread
   falls behind by a few milliseconds, the kernel drops the overflow. pion counts each drop as
   congestion and halves its window. On a desktop this alone costs 2–4× (§4.1). On a slow
   receiver the two causes feed each other: the slower the thread drains the socket, the more
   gets dropped, and the more the window is cut.

**Proven:** the 64 KiB buffer (Chromium source and the kernel's own report); the drops at the
receiving socket; the window cuts; pion's lack of pacing (source); the processor cost per byte on
this machine; that sender pacing, a larger receive buffer and pion's default growth step each
help on an unstarved receiver. **Plausible, not proven:** that the TV's 15–30 Mb/s is
mostly the processor ceiling of point 1, with point 2 on top. A starved receiver emulated
here lands on the TV's very range (31–41 Mb/s with Orpheum's settings) while HTTPS on the same
starved receiver still does 650–725 Mb/s (§4.3). Nothing has been measured on the TV itself.
**Wrong in the first write-up:** that the round trip explains the TV. At the 6–30 ms the TV's
pongs show, the same mechanism on a desktop gives 50–100 Mb/s, not 15–30. Also wrong: that the
channel thresholds and the window growth step are not worth touching (§5).

**How much slower it should be:** on a desktop LAN, a tuned pion sender to Chrome carried
200–320 Mb/s at 5–20 ms here. That is below HTTP's figure on the same veth link, but HTTP's
gigabits there are a property of the virtual link. On a real 100–200 Mb/s Wi-Fi it is close to
link rate. On the Fire TV the expectation is **2–5× below HTTPS once tuned, and the present
~10× is roughly 2–3× of our making on top of a 3–5× platform cost.** These factors are
estimates from the emulation, to be confirmed on the TV (§6, item 0).

## 2. What the sources say

| Source | What it says | Bearing |
| --- | --- | --- |
| Chromium [`services/network/p2p/socket_udp.cc`](https://chromium.googlesource.com/chromium/src/+/refs/heads/main/services/network/p2p/socket_udp.cc) | `const int kUdpRecvSocketBufferSize = 65536;  // 64K`, applied by `SetReceiveBufferSize` at socket init. Received packets are batched to the renderer at most `kUdpMaxBatchingRecvPackets = 64` or 1 ms ([`socket_udp.h`](https://chromium.googlesource.com/chromium/src/+/refs/heads/main/services/network/p2p/socket_udp.h)) | The receive buffer that overflows. Silk is Chromium and shares it |
| WebRTC [`media/base/media_constants.h`](https://webrtc.googlesource.com/src/+/refs/heads/main/media/base/media_constants.h), [`media/engine/webrtc_video_engine.cc`](https://webrtc.googlesource.com/src/+/refs/heads/main/media/engine/webrtc_video_engine.cc) | `kVideoRtpRecvBufferSize = 1'048'576`. A video **receive** channel sets `Socket::OPT_RCVBUF` to it on its transport ("Set the RTP recv/send buffer to a bigger size") | A page can make Chrome enlarge the socket buffer by negotiating a receive-only video m-line on the bundled transport (measured in §4.2) |
| WebRTC [`p2p/base/p2p_transport_channel.cc`](https://webrtc.googlesource.com/src/+/refs/heads/main/p2p/base/p2p_transport_channel.cc) | The field trial `WebRTC-SetSocketReceiveBuffer/Enabled-<kB>/` sets the receive buffer | Chrome's own escape hatch. A page cannot set it |
| WebRTC [`net/dcsctp/public/dcsctp_options.h`](https://webrtc.googlesource.com/src/+/refs/heads/main/net/dcsctp/public/dcsctp_options.h) | `kMaxSafeMTUSize = 1191`, `max_receiver_window_buffer_size = 5 * 1024 * 1024`, `cwnd_mtus_initial = 10`, `max_burst = 4` ("to avoid bursts that too quickly fill the send buffer"); [`media/sctp/dcsctp_transport.cc`](https://webrtc.googlesource.com/src/+/refs/heads/main/media/sctp/dcsctp_transport.cc) changes none of them | Chrome's own sender limits bursts to 4 packets per send opportunity. pion does not (§3). The 4.7 MB `a_rwnd` the first write-up saw is this 5 MB window |
| [RFC 9260 §6.1 D and §16](https://www.rfc-editor.org/rfc/rfc9260.html) | "the protocol parameter 'Max.Burst' SHOULD be used to limit the number of packets sent", suggested `Max.Burst: 4`. §7.2.1: slow start grows cwnd by "at most … L times the destination's PMDCS" per SACK. §7.2.2: "MUST NOT increment cwnd by more than PMDCS per RTT". §7.2.3: on loss `ssthresh = max(cwnd/2, 4*PMDCS)` | pion departs from the first two rules (§3). The third is the halving |
| [RFC 8831 §5–6](https://www.rfc-editor.org/rfc/rfc8831.html), [RFC 8261 §5](https://www.rfc-editor.org/rfc/rfc8261.html) | Data channels must be congestion controlled, TCP-friendly. The initial path MTU SHOULD NOT exceed 1200 bytes (IPv4) | Why every stack uses ~1,200-byte packets: many more packets per byte than TCP with offload |
| [pion/sctp#218](https://github.com/pion/sctp/issues/218) (2022, open) | pion↔pion on gigabit Ethernet: TCP 800 vs SCTP 200 Mb/s at 20 ms, 600 vs 85 at 50 ms. Maintainer Yutaka Takeda points at the fixed receive buffer. A later comment: 20 Mb/s over an 80 ms Wi-Fi/TURN path against 600+ for TCP even with an 8 MB buffer | pion's SCTP was 4–8× below TCP without any browser involved. The 2026-10 pion↔pion figures (880/600/370/65 Mb/s at 0/2/10/40 ms) agree |
| [pion/sctp#439](https://github.com/pion/sctp/issues/439) (2025-11, open) | Maintainers weighing BBR-style control. "most of new CCs … require an implementation of pacing" | pion has no pacing today, by its maintainers' own account |
| [pion/sctp#394](https://github.com/pion/sctp/pull/394) (2025-10, merged), [#354](https://github.com/pion/sctp/pull/354) (2024-11, merged) | Burst mitigation during loss recovery only ("RACK for SCTP" §5A); the `MinCwnd`/`FastRtxWnd`/`CwndCAStep` knobs, because "loss-based congestion control gets poor performance under high bandwidth, high RTT and packet loss" | Both are in v1.12.0, which Orpheum pins and which is the latest release. Upgrading gains nothing |
| Victor Boivie, [PSA: dcSCTP Library](https://groups.google.com/g/discuss-webrtc/c/hY3VkIw2-20/m/Gd2O0Q4aCQAJ) (discuss-webrtc, 2021) | "The congestion control algorithm hasn't been fully tuned, so performance may be slightly worse compared to usrsctp, but should generally be on par" | Chrome's team never claimed TCP parity |
| Eskola, [Massive drop in data channel throughput when increasing RTT](https://groups.google.com/g/discuss-webrtc/c/0synE_0zeCQ/m/j58MZ1KsEAAJ) (2014-07-01). Also Eskola & Nurminen, [Performance Evaluation of WebRTC Data Channels](https://doi.org/10.1109/ISCC.2015.7884873) (IEEE ISCC 2015). The paper itself was not opened; as search summarised it, implementations "do not adjust the SCTP window size from the default setting" | On a 100 Mb/s LAN with netem, Chromium fell from 7 MB/s at 0 ms to 1.5 MB/s at 20 ms. Wireshark showed "the sender will stop sending entirely and wait for ACK packets", in bursts | The same burst-and-stall shape, a decade ago, browser to browser |
| Linux [`socket(7)`](https://man7.org/linux/man-pages/man7/socket.7.html) | SO_RCVBUF: "The kernel doubles this value (to allow space for bookkeeping overhead)", capped by `rmem_max` | Why 64 KiB shows as `rb131072`. Each datagram is charged its buffer's true size, so about 50 packets fit |

What the sources do not give: a published figure for Chrome receiving from a server's data
channel on a LAN, and Android's `rmem_max` on Fire OS (which caps how far the video-m-line trick
can raise the buffer there). The literature research was cut short by the session's web search
quota. Mozilla's data channel, Lennart Grahl's RAWRTC measurements and LiveKit's figures were
not reached.

## 3. What pion v1.12.0 actually does

Read in `github.com/pion/sctp@v1.12.0/association.go`. Orpheum pins pion/webrtc v4.2.23 and
sctp v1.12.0, the latest sctp release (webrtc v4.2.26 exists and changes nothing here).

- **No pacing, no Max.Burst.** `writeLoop` writes every packet that `gatherOutbound` returns in
  one loop, one `Write` per packet. `popPendingDataChunksToSend` fills up to `cwnd` at once. The
  only burst budget (`tlr…`, quarter-MTU units) is active only during loss recovery.
- **Slow start is TCP-style doubling, not SCTP's.** `cwnd += min(bytesAcked, cwnd)`. The RFC line
  `cwnd += min(bytesAcked, MTU)` is present but commented out as "SCTP way (slow)". `ssthresh`
  starts at the peer's `a_rwnd` (5 MB from Chrome). So the window doubles each round trip until
  something drops, and the burst it sends doubles with it.
- **Congestion avoidance** grows by `max(MTU, cwndCAStep)` per window acknowledged, and only
  while data is waiting (`pendingQueue.size() > 0`). Orpheum sets `cwndCAStep` to 64 KiB, 55×
  pion's default of one MTU (`rtcapi.cwndStepBytes`).
- **Loss:** three miss indications (fast retransmit) or RACK. Entering fast recovery sets
  `ssthresh = max(cwnd/2, 4·MTU)` and `cwnd = ssthresh`. The retransmission timeout's floor is
  1 s (`rtx_timer.go`, `rtoMin`), against dcSCTP's 400 ms.
- **Packet size:** `sctpOutboundMTU = 1191` (pion/webrtc `constants.go`), not settable through
  the `SettingEngine`. It matches Chrome's.
- **`BufferedAmount` counts bytes in flight**, not only queued ones: a stream's amount is released
  on SACK (`onBufferReleased`). Orpheum's writer (`rtcapi/writer.go`) waits while
  `BufferedAmount() > 1 MiB`, and it is woken only when the amount falls to 256 KiB. So a channel
  keeps between 256 KiB and ~1 MiB outstanding, about 640 KiB on average. That caps a channel at
  about 640 KiB per round trip, ~100 Mb/s at 50 ms and ~250 Mb/s at 20 ms. It is not binding while
  losses keep the window near 200 KB, and it binds as soon as they stop (§4.1).

## 4. Measurements

**Set-up:** a minimal pion sender (pion/webrtc v4.2.23, sctp v1.12.0, the same as `go.mod`) with
`rtcapi`'s shape: one ICE UDP mux socket, 64 KiB messages, the 1 MiB / 256 KiB thresholds, and
the same `SettingEngine` (`CwndCAStep` 64 KiB, receive buffer 1 MiB, message size 64 KiB). It ran
in a network namespace joined by a veth pair with `tc netem` delay on both ends (`limit 100000`).
The receiver was the pinned Playwright's Chromium headless shell. Each data channel carried one
load, with a 1 MiB warm-up load first so the measured one is not the association's first. The
host is the sandbox VM, an i3-1315U with 8 vCPUs. "Drops" is the kernel's UDP `RcvbufErrors`
over the run, all at Chrome's socket (`ss -uam` showed the drops counter on it, `d3345`). Optional
**pacing** is real sender pacing: a token bucket under the ICE mux (`WriteTo`), 16 KiB burst. It
is not a link rate cap. 32 MiB per load, Mb/s, two or three runs each.

### 4.1 An unstarved receiver

| Round trip | HTTP | Orpheum's settings | pion default step | Paced 300 | Paced 300, thresholds 8/4 MiB | 4 peer connections | 4 channels, 1 connection |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ~0 | 4500–4900 | 54–100 | 377–392 | 260–263 | | 183–216 | 190 |
| 5 ms | 2500–3900 | 47–101 | 89–103 | 218–227 | 205 (video m-line) | 158–164 | 117 |
| 20 ms | 700–1235 | 54–90 | 44–54 | 143–144 | 206–212 | 101–133 | 108 |
| 50 ms | 277–469 | 26–34 | 13–16 | 80–83 | 129–169 | 69–102 | 29 |

Drops per 32 MiB load (about 27,500 data packets): Orpheum's settings 680–1,500 (2.5–5 %),
pion's default step 100–860, paced 0–34. The sender's UDP bytes over the delivered bytes were
1.08–1.10 unpaced and 1.057 paced, where 1.057 is the bare header and DTLS overhead, so the extra
3–5 % are retransmissions. These figures reproduce the first write-up's (57/33 Mb/s at 20/50 ms
in Chrome).

- **Pacing works** with real pacing too: 2.5–3× at 5–50 ms, with zero drops.
- **Once losses stop, the channel thresholds are the next limit:** 82 → 129–169 Mb/s at 50 ms and
  144 → 210 at 20 ms when they are raised to 8/4 MiB.
- **Orpheum's 64 KiB growth step hurts at home round trips:** at ~0 ms, 54–100 against 377–392 with
  pion's default. It helps only at 50 ms (26–34 against 13–16).
- **Several channels on one connection gain nothing. Four connections gain 1.5–3×**, as the first
  write-up said: each connection has its own socket, its own 64 KiB buffer and its own window.

### 4.2 Raising Chrome's socket buffer with a receive-only video m-line

The page adds `pc.addTransceiver("video", {direction: "recvonly"})` before its offer. The sender
registers pion's default codecs and answers it. No track is ever sent. Chrome's socket then
showed `rb425984` instead of `rb131072`: the 1 MiB request, capped here by `rmem_max`
(212,992, doubled). This sandbox cannot raise `rmem_max` (a container), so the full 1 MiB was not
tested.

| Round trip | Orpheum's settings | + video m-line | + video m-line, pion default step | + video m-line, paced 300, thresholds 8/4 MiB |
| --- | --- | --- | --- | --- |
| 5 ms | 47–57 | 136–148 | 282–322 | 204–206 |
| 20 ms | 58–90 | 159–169 | 55–87 | 184–218 |
| 50 ms | 26–28 | 73–75 | 47–50 | 152–172 |

The buffer alone gives **~2.5–3×** at every round trip, with no change to the sender's sending.
Drops fall but do not stop. Pacing still adds the rest at 20–50 ms.

### 4.3 A starved receiver (standing in for the TV)

Chromium was run in a cgroup holding it to 0.2 or 0.5 of a CPU across two cores (`cpu.max`,
10 ms period). A real TV is not throttled this way. Its threads are delayed by decoding,
compositing and JavaScript. Both starve the thread that drains the socket, but this emulation is
indicative, not a model. Round trip 10 ms:

| Receiver | HTTP | Orpheum's settings | pion default step | + video m-line, default step | Paced 100, 8/4 MiB | Paced 100, 8/4 MiB, + video m-line | Paced 200, 8/4 MiB |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 0.2 CPU | 654–726 | 31–41 | 50 | 67–70 | 34–36 | 64–69 | 31–33 |
| 0.5 CPU | 1038–1597 | 31–33 | 72–75 | | 57–70 | | 38–41 |

**A starved receiver brings HTTP down by a factor of ~5 and the data channel to the TV's
range**, with HTTP still 10–20× ahead. Here pacing helps little: the socket overflows during the
receiver's stalls, however evenly the packets arrive. What helps is a smaller burst after each
stall (pion's default step) and a larger socket buffer (the video m-line). Together they give
about 2×. Retransmission timeouts appeared too (the window reported at one MTU, 1,191 bytes, after
a run), each costing pion's 1 s minimum.

### 4.4 Processor time per byte in the browser

Chromium's processes' CPU time while loading, unstarved, round trip 10 ms, no loss (paced 200):

| Load | Wall time | Browser CPU time | Per MiB |
| --- | --- | --- | --- |
| HTTP, 32 MiB | 0.12–0.22 s | 0.07–0.10 s | ~2.5 ms |
| Data channel, 128 MiB | 6.9–7.1 s | 4.8–5.4 s | ~38 ms |

By thread, for the data channel's 128 MiB: the network service's IO thread (which reads the UDP
socket) 2.27 s, the renderer's WebRTC worker/network thread 1.53 s, the renderer's IO thread
(the IPC from the network service) 0.81 s, the page's main thread 0.22 s. **The data channel
costs the browser ~15× the processor time per byte of plain HTTP.** HTTPS adds AES-GCM, a few
milliseconds per 32 MiB, which does not change the ratio much. The socket's reader alone spends
~18 ms per MiB on an i3-1315U core. If the TV's cores are 5–8× slower per thread, which is an
assumption (the A53-class guess is not verified), that thread alone saturates at 60–90 Mb/s.
It shares its time with everything else the device does, and with a 5 ms buffer it starts
dropping long before it saturates.

## 5. The first write-up, point by point

| Its claim | Verdict |
| --- | --- |
| The ceiling is the window that loss keeps cutting, and the loss is the browser's own socket overflowing under pion's unpaced bursts | **Holds** on a desktop, reproduced. It missed why the socket is so small (Chrome's 64 KiB constant), and that the small socket makes the receiver's processor time the TV's binding limit |
| On the TV: a ~170 KB window at a 45–90 ms round trip | **Does not fit.** The TV's pongs show 6–30 ms, where this mechanism gives a desktop 50–100 Mb/s. A slow receiver is needed to reach 15–30, and the emulated one reaches it |
| Pacing triples the throughput at 50 ms | **Holds** with real sender pacing (2.5–3×). On a starved receiver it **does not hold**: ~1× (§4.3) |
| Ruled out: the channel thresholds | **Wrong once pacing is in.** They become the next ceiling (82 → 150–170 Mb/s at 50 ms) because pion's `BufferedAmount` includes bytes in flight |
| Not worth it: a different window growth step | **Wrong at home round trips.** Orpheum's 64 KiB step cuts ~0 ms throughput 4–7×, and pion's default doubles a starved receiver's throughput. It helps only at 50 ms and up |
| Ruled out: the relay, credit, message size, rwnd, the core's socket, DNAT | **Hold.** Message size and ordered delivery do not matter: SCTP fragments every message into 1,191-byte packets either way, and one load per channel never waits on another. The core's 4 MiB credit is not binding below 4 MiB per round trip |
| Ruled out: encryption and the processor ("throughput falls with RTT, which a processor bound would not do") | **Wrong for the TV.** It holds only for an 8-core desktop. In the browser, the data channel's per-byte cost is ~15× HTTP's (§4.4) |
| Fix 2, HTTPS at the LAN name, gains 10× or more | **Holds**, and it is the only fix that escapes the platform cost |
| Fix 3, spread a load over the transport's connections | **Holds** on a desktop (1.5–3×). On a starved TV it adds packets to the same starved threads, so the gain is uncertain |

## 6. Fixes, ranked by expected gain on the TV for their cost

0. **Measure on the TV first** (cost: none, the probe exists, VQ117). The Throughput probe's
   transport and relay legs must run while the page is **on WebRTC**. The 16:53 run was on HTTPS
   (`over=http` on both legs), so no WebRTC figure from the TV exists yet. With it, and the
   connection-end SCTP figures of fix 5, the split between the platform cost and ours becomes a
   measurement rather than this review's estimate.
1. **HTTPS at the LAN name wherever the browser allows it** (gain: the whole 5–10×; cost:
   per-browser policy; Silk's `localNetwork=prompt` needs deciding). Nothing on the data channel
   reaches HTTPS on a slow device. Kernel TCP with offload is ~15× cheaper per byte in the browser.
2. **A receive-only video m-line in the client's offer** (gain: ~2.5–3× on a desktop, measured.
   On the TV, up to what Fire OS's `rmem_max` allows, unknown; cost: small, with a new answer
   path in `rtcapi`, which must register a codec and answer the m-line without ever sending).
   This uses documented WebRTC behaviour (`kVideoRtpRecvBufferSize`), not a bug, but it is a
   side effect, so it gets a test and an idiosyncrasies entry. It needs the owner's decision.
3. **pion's default congestion-avoidance step, or a small one, at home round trips** (gain: up
   to 2× on a starved receiver, 4–7× at ~0 ms on a desktop; loses ~2× at 50 ms; cost: one
   constant, or a step chosen by path (`direct` vs relayed), since `rtcapi` knows the path).
4. **Pace the core's sending, and then raise the channel thresholds** (gain: 2.5–3× at 5–50 ms on
   an unstarved receiver, to 150–220 Mb/s; about nothing on a starved one; cost: medium, a pacing
   `PacketConn` under the mux at a rate from the window and round trip). The thresholds must move
   with it, or pacing is capped at ~80 Mb/s at 50 ms.
5. **Log each association's SCTP figures when it ends** (gain: diagnosis; cost: small). As the
   first write-up proposed: window, smoothed round trip, retransmissions, timeouts.
6. **Spread one load across connections** (gain: 1.5–3× on a desktop, uncertain on the TV; cost:
   medium, on the playback path). Behind 2–4.

Not worth doing: upgrading pion (v1.12.0 is the latest release, and nothing newer changes
pacing), larger messages, unordered or several channels on one connection, a larger SCTP
receive buffer on the core (it only bounds what clients send), and a larger MTU (1191 is not
settable in pion/webrtc and would save at most ~15 % of the packets).
