> **ABOUTME:** Where the WebRTC path's throughput ceiling comes from: Chrome and Firefox loading
> from a real core over a data channel, through the service worker's relay, and over plain HTTP,
> at round trips from 0 to 50 ms; the cause found (the browser's UDP socket overflowing under the
> core's unpaced SCTP bursts, each overflow halving the congestion window); the suspects ruled out;
> fixes ranked by gain and cost; and the Throughput probe that measures the same on the owner's
> devices (VQ117).

# WebRTC Throughput: Where the Ceiling Comes From

- **Date:** 2026-10-28. Asked by the owner after the Fire TV (Silk 138, Android 9, AFTSSS) loaded
  chunks on the LAN's direct path (`path=direct_ipv4`) at a median of 15 Mb/s and at most 30 Mb/s,
  with about 50 ms to each request's first byte, while fast.com on the TV said 100 Mb/s. Measurement
  only: nothing on the playback path changed.
- **Method:** the core built from `main` an earlier commit plus the throughput body (below), run alone in a
  Linux network namespace joined to the sandbox by a veth pair, so `tc netem` shaped only its traffic
  (delay each way, so the round trip is twice the delay; `limit 100000`, so netem itself drops
  nothing). The pinned Playwright's Chromium (HeadlessChrome 153) and Firefox 155 loaded 32 MiB of
  incompressible bytes (`GET /api/v1/lab-throughput`) four ways: plain HTTP from the page; the data
  channel protocol driven by hand in the page, one request (`raw1`) and two at once (`raw2`) on one
  peer connection, granting credit as the SDK does (16 messages per 16 read); and the app's own
  Throughput probe (`/lab/throughput?transport=webrtc`), whose first leg is the page's transport
  and whose second the service worker's relay, as a film's chunks go. The page was served on
  loopback through a TCP forwarder to the namespace, so it was a secure context and had its service
  worker, while every byte crossed the shaped veth. pion's SCTP was traced (`PION_LOG_TRACE=sctp`)
  and the kernel's UDP counters (`/proc/net/snmp`) read before and after each run. The scripts were
  a throwaway, as the 2026-10-09 spike's were (reachability-approaches-2026-10-09.md §9.4).
- **One machine:** browser and core share eight cores, so the figures at a round trip near 0 are
  bound by the processor and vary by a factor of two between runs; from 10 ms up they are steady.

## 1. Summary

The ceiling is the **congestion window that loss keeps cutting**, and the loss is **the receiving
browser's own UDP socket overflowing**: pion's SCTP sends a whole window back to back with no
pacing, the burst arrives faster than the browser's network thread drains its socket, the kernel
drops what does not fit (`RcvbufErrors`), pion's RACK marks the gaps lost and retransmits, and fast
recovery halves the window. The window settles near 150–220 KB, so a connection carries about that
much per round trip: 25–30 Mb/s at 50 ms, 55–60 Mb/s at 20 ms, in both browsers, while plain HTTP
on the same link carries 240–2000 Mb/s.

Proven on this machine: across runs the receiver's dropped datagrams match pion's retransmissions
one for one (188 and 182, 548 and 543, 614 and 611); the browser's advertised receive window stayed
at 4.7 MB, never the limit; it reported no duplicate TSNs, so the retransmissions filled real holes.
Spreading the same packets out with a rate cap on the link, a stand-in for pacing, removed every
drop and **tripled** the throughput at 50 ms (24 to 72 Mb/s in Chrome, 22 to 71 in Firefox), though
the cap was the only change and is below what the link carried unpaced at a short round trip.

On the TV this is **the leading suspect, not proven**: the 15–30 Mb/s it saw is what a window of
~170 KB gives at a round trip of 45–90 ms, plausible on Wi-Fi under load, and a slow device drains
its socket more slowly still. The Throughput probe settles it there (VQ117): if the transport and
the relay both show 15–30 Mb/s and the address at home over HTTPS shows several times that, the
WebRTC path is the limit and not the TV's link or the relay.

## 2. Figures

32 MiB, Mbit/s, Chrome / Firefox. The round trip is netem's added delay, twice the delay each way;
the browser's own `currentRoundTripTime` read the same.

| Round trip | HTTP | raw1 | raw2 | Probe: transport | Probe: relay |
| --- | --- | --- | --- | --- | --- |
| ~0 (veth) | 8337 / 1322 | 300 / 133 | 478 / 140 | 401 / 144 | 287 / 171 |
| 4 ms | 1997 / 2049 | 128 / 106 | 103 / 79 | 168 / 171 | 170 / 154 |
| 10 ms | 1131 / 895 | 93 / 141 | 132 / 64 | 71 / 156 | 95 / 161 |
| 20 ms | 592 / 232 | 62 / 60 | 97 / 95 | 57 / 55 | 45 / 52 |
| 50 ms | 242 / 279 | 24 / 22 | 30 / 27 | 33 / 28 | 28 / 30 |

The same at 50 ms and at 10 ms, with the link's rate capped so packets arrive spaced (Chrome /
Firefox, one request, and the receiver's `RcvbufErrors` during the run):

| Round trip, cap | raw1 | Receiver drops |
| --- | --- | --- |
| 50 ms, none | 20–29 (8 MiB, Chrome) | 182–548 |
| 50 ms, 150 Mbit/s | 72 / 71 | 0 |
| 50 ms, 60 Mbit/s | 40 (8 MiB, Chrome) | 0 |
| 10 ms, none | 142 / 98 | 2172 |
| 10 ms, 400 Mbit/s | 223 / 199 | 413 |

From pion's trace of one 8 MiB load at 50 ms: 7,040 DATA packets, 350 retransmitted, 320 marked
lost by RACK, the window halved again and again (`ssthresh` 103,539 then 216,189; `cwnd` 171,180
in fast recovery at the end), and 261 SACKs on which it could not grow, 75 of them in fast
recovery. Every SACK carried `a_rwnd` near 4.7 MB.

## 3. What was ruled out

- **The service worker's relay** (an earlier commit's 1 MiB credit window): within the runs' noise of
  the transport at every round trip, in both browsers, and unchanged with the page's main thread
  slowed six times (Chrome's CPU throttling: relay 160 Mb/s against 108 unthrottled at 10 ms).
- **The core's credit window** (`InitialCreditMessages` 64, the page's `grantEvery` 16): never
  binding; a load granted unbounded credit at once (`raw1unbounded`) was no faster, at 10–50 ms
  23–135 Mb/s against 22–141 with the SDK's grants, and in Chrome slower at short round trips. Why
  unbounded credit was slower is not explained; the SDK's way is the faster one.
- **The channel's thresholds** (`bufferedHighBytes` 1 MiB, `bufferedLowBytes` 256 KiB): the core
  keeps up to a mebibyte queued behind the window, enough that pion's window may grow (it grows only
  while data waits, association.go's `pendingQueue.size() > 0`), and the window, not the queue,
  ran out first.
- **Message size** (`MaxMessageBytes` 64 KiB, the browsers' `maxMessageSize` 65536): each message is
  some 56 SCTP packets; the limit is per packet and per round trip, not per message.
- **The browser's SCTP receive window:** 4.7 MB advertised throughout.
- **Encryption and the processor:** at a round trip near 0 a connection carried 130–480 Mb/s on a
  shared machine, ten times the TV's figure; the throughput falls with the round trip, which a
  processor bound would not do.
- **The core's socket buffers and the two DNATs** (task 3): the core does not set `SO_RCVBUF` or
  `SO_SNDBUF` (`rtcapi.ListenLAN` opens its socket with `net.ListenUDP`, the kernel's 208 KiB
  default here), and staging's core container counts no `RcvbufErrors` and no `SndbufErrors` over
  177,416 datagrams sent; the sandbox's conntrack counts no `drop`, `early_drop` or
  `insert_failed`, its `eth0` no drops, and the DNAT rule for UDP 8443 is a lookup per packet, a few
  thousand a second at 30 Mb/s. The VM's own hop cannot be read from inside the sandbox; nothing
  here points at it. The drops are at the receiver, not the sender or the path.
- **Requests at once on one connection:** on loopback, two or four loads on one peer connection
  carried no more than one in all (139–478 Mb/s against 220–450, processor bound), so the shared
  window, not each channel, is the unit. hls.js's picture and sound loads share one window.

## 4. Fixes, ranked by gain for cost

1. **Pace the core's sending** (gain: about 3× at 20–50 ms, measured with a rate cap standing in for
   pacing; cost: medium). pion's SCTP has no pacing. A pacing `net.PacketConn` under the ICE mux,
   spacing each association's datagrams at a rate above what it carries (its window over its
   round trip, with headroom, or a fixed ceiling as a first step), keeps the burst from overflowing
   the browser's socket. Needs: pion's window and round trip per association (its SCTP transport's
   stats), and a test on loopback with netem that shows the drops gone. No new dependency.
2. **HTTPS at the LAN name where the browser can use it** (gain: 10× or more, HTTP carried
   240–2000 Mb/s at the same round trips; cost: per-browser policy). Silk sends `localNetwork=prompt`
   and was left out; the probe's third leg says whether Silk reaches the LAN name at all and how
   fast, which decides whether asking for the permission is worth building.
3. **Spread one load across the transport's connections** (gain: up to the connection count, 4;
   cost: medium, and on the playback path). Each peer connection has its own UDP socket and its own
   window, so a chunk asked for in ranges over several carries several windows. A ceiling-lifter,
   not a cure: each part still loses to its own bursts.
4. **Log each association's SCTP figures when it ends** (gain: diagnosis; cost: small). pion's
   SCTP transport stats (window, smoothed round trip, MTU) and the retransmission count on the
   `rtcapi: a connection ended` line would have shown this from the TV's own session.
5. **pion's minimum retransmission timeout** (1 s, a constant in pion/sctp's `rtx_timer.go`): a
   real Wi-Fi loss that defeats fast retransmission stalls a connection a second and restarts it from
   one packet. Not seen here (3 probe timeouts in 7,040 packets); a suspect on the TV, which the
   logging in item 4 would show. Changing it needs pion's change.

Not worth doing for throughput: a larger credit window, larger channel thresholds, larger messages,
or a faster window growth step (`SetSCTPCwndCAStep`): none of them is the limit, and growth is cut
back by the next overflow.

## 5. The Throughput probe

Device tests' **Throughput** (`/lab/throughput`, `web/apps/web/src/lab-throughput.tsx`) loads
32 MiB from the core three ways in turn and shows each one's rate, its first byte's wait and the
path the core says it came over: **as the app's requests go** (the page's transport, WebRTC or
HTTPS), **as a film's pieces go** (the browser's own load, which the service worker hands to the
page's transport while the page is on WebRTC), and **from the address at home** (the core's LAN
name over HTTPS, when one is typed; kept on the device). It sends the figures, with the user agent,
to the server's log as **"a throughput test ran"**, and the core logs each body it sent as **"a
throughput test's body was sent"** with its leg, the path it came over (`over`), the name it was
reached by (`host`), its bytes and its rate as the core saw it.

The body is `GET /api/v1/lab-throughput?bytes=&leg=`: random bytes no gzip shrinks, 1–256 MiB (32
by default), `Cache-Control: no-store`, the path in an `Orpheum-Over` header, behind the device
tests' gate (`should_offer_device_tests`), signed in only. A diagnostic endpoint: it serves no
library content and changes nothing; its bounds are justified constants, not settings.
