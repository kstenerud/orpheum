> **ABOUTME:** What a realistic home, mobile, congested and lossy link looks like, from the IETF's test
> cases, measurement studies and the kernel's own source; how to emulate each with netem on the
> loopback without the artefacts the acceptance has now; and whether BBR keeps the film alive on a
> lossy link. Gathered 2026-10-07. Feeds a revision of D243 §9 and D252.

# Network profiles: realistic bad links, emulated correctly, and BBR

Researched **2026-10-07**, after the acceptance's *congested* profile (D243 §9: 150 ± 50 ms, 3% loss,
5 Mbit/s) left the film unable to keep up. The owner agreed, pending verification, to redefine
*congested* as a bottleneck with a bounded queue, competing traffic and little random loss; to add a
*lossy* profile with about 3% random loss; and to try BBR on the server's media sockets. Sources are
primary where one exists: RFCs 8867, 8868 and 8869, the kernel's `sch_netem.c`, `tcp_bbr.c`,
`tcp_cong.c` and `sysctl_net_ipv4.c` read from `torvalds/linux` master on the day, the iproute2 man
pages, the IETF BBR draft, Google's BBRv3 slides, and measurement papers. The web-search quota was
exhausted before this began, so sources were fetched by known URL and through arXiv's catalogue;
anything not verified that way is marked. Pages are cached under `/yoloai/cache/http/netprofiles/`.

The claims were also **measured** in this sandbox: in a private network namespace (so the dev loop
and staging on the container's own loopback were untouched), loopback MTU 1500 and `gso_max_size`
1500, kernel 6.12, one 20-second TCP transfer per cell (Python, the sender setting its congestion
control), and a 50-packet-a-second UDP probe standing in for voice. One run per cell: the numbers
show direction and size, not a distribution. A claim marked *(inferred)* is our reading.

## 0. Summary

**Recommended profiles**, for the loopback the acceptance runs on now (D252 "As built"). On `lo`
every packet, in either direction, passes the root qdisc once, so `delay` is a one-way delay and a
round trip is twice it. The netem stage adds delay, in-order jitter and loss; the `tbf` child is the
bottleneck and its tail-drop queue. netem's `limit` is raised so that netem itself never drops.

| Profile | Commands | One-way base / RTT | Bottleneck and queue | Loss |
| --- | --- | --- | --- | --- |
| home | `tc qdisc replace dev lo root handle 1: netem delay 20ms limit 100000`<br>`tc qdisc add dev lo parent 1:1 handle 2: tbf rate 100mbit burst 64kb latency 30ms` | 20 / 40 ms | 100 Mbit/s, 30 ms | none |
| mobile | `tc qdisc replace dev lo root handle 1: netem delay 40ms 10ms distribution normal rate 1gbit loss 0.2% limit 100000`<br>`tc qdisc add dev lo parent 1:1 handle 2: tbf rate 20mbit burst 32kb latency 300ms` | 40 / 80 ms, ±10 ms in order | 20 Mbit/s, 300 ms | 0.2% |
| congested | `tc qdisc replace dev lo root handle 1: netem delay 40ms 5ms distribution normal rate 1gbit loss 0.5% limit 100000`<br>`tc qdisc add dev lo parent 1:1 handle 2: tbf rate 5mbit burst 16kb latency 200ms`<br>plus one long-lived CUBIC download through `lo` for the whole run (§4) | 40 / 80 ms, ±5 ms in order | 5 Mbit/s, 200 ms, shared with bulk traffic | 0.5% |
| lossy | `tc qdisc replace dev lo root handle 1: netem delay 40ms 10ms distribution normal rate 1gbit loss 3% limit 100000`<br>`tc qdisc add dev lo parent 1:1 handle 2: tbf rate 20mbit burst 32kb latency 100ms` | 40 / 80 ms, ±10 ms in order | 20 Mbit/s, 100 ms | 3% each way |

`tc qdisc del dev lo root` removes both stages. The `rate 1gbit` in the netem stage is not a
bottleneck: it is what keeps jittered packets in order (§3.3).

**BBR:** use it. Set `TCP_CONGESTION` to `bbr` on the server's listening socket (accepted
connections inherit it), fall back to the default with one log line when the kernel refuses, and
make sure the host loads `tcp_bbr`. Mainline Linux has BBRv1 only; BBRv3 is not merged. BBR needs
no `fq` qdisc since Linux 4.13. Under 3% random loss it carried 17.7 Mbit/s of a 20 Mbit/s link where
CUBIC carried 1.0. Its costs are a standing queue of about one round trip at the bottleneck and a
larger share against CUBIC; D262 §2's pacing cap bounds both while voice is live. Steps in §6.

**What was wrong** in the claims and in the current emulation, in full in §8: RFC 8868 is not the
wireless test cases (RFC 8869 is); correlated jitter does not stop netem reordering, and neither
does a `pfifo` child (the current *mobile* profile reorders 23% of voice packets and 47% of a
500-packet-a-second stream); netem's `limit` counts every packet it holds, including those only
being delayed and the other direction's, so `limit 100` is not a quarter-second queue (CUBIC saw a
472 ms round trip under it with no loss); netem's `rate` absorbs the propagation delay into the
queue instead of adding them; and the *mobile* profile's 1% loss already caps a CUBIC download at
about 1.5 Mbit/s, so the film's trouble was not only *congested*'s.

## 1. What the IETF's test cases specify

**RFC 8867** (test cases for congestion control of interactive real-time media, January 2021),
§4.2's defaults for every test unless the test overrides them: reference bottleneck **1 Mbit/s**,
one-way propagation delay **50 ms**, maximum end-to-end jitter **30 ms**, a **tail-drop** queue
(FQ-CoDel and PIE "encouraged" as variants) of **300 ms**, and **0% path loss**. The tests that
matter here:

- §5.6, *media competing with a long TCP flow*: bottleneck **2 Mbit/s**, queue **300–1000 ms**, one
  long-lived TCP flow with the default congestion control (other algorithms "encouraged") from 0 to
  119 s, video and audio from 5 s.
- §5.7, *media competing with short TCP flows*: **2 Mbit/s**, ten short-lived TCP flows in an ON/OFF
  pattern, two starting ON.
- §5.1/5.2: a bottleneck that varies over time (1 Mbit/s reference, 50–100 ms delay); §5.4 and §5.5:
  several media flows, with one-way delays of 10, 25, 50, 100 and 150 ms for the RTT-fairness test.

No test in RFC 8867 combines a bottleneck with random loss; loss is a separate axis.

**RFC 8868** is not the wireless document; it is *Evaluating Congestion Control for Interactive
Real-Time Media*, the evaluation guidelines whose §4 lists the network parameters:

- One-way propagation delay: 0–1 ms, **50 ms**, **150 ms** ("high"), 300 ms ("extreme").
- End-to-end loss: 0%, **1%**, **5%**, 10%, 20%. "Many paths in the Internet today are largely
  lossless"; loss is for "interference in wireless networks, … remote regions, or high/fast
  mobility". Independent random loss "must be used" as the minimum model; Gilbert-Elliott and
  queue-based models "could be considered".
- Drop-tail queue length, as time to drain: QoS-aware **70 ms**, nominal **300–500 ms**,
  buffer-bloated **1000–2000 ms**; "home gateways usually have larger queue length".
- Jitter (§4.5): two bounded models, and the one that **must** be used is *NR-BPDV*, jitter that
  never reorders packets within a flow: a packet may not leave before the one ahead of it has
  finished serialising, because "for the vast majority of cases, there is no packet reordering" and
  mobile links have "per-flow queues prior to base station scheduling". Recommended distribution: a
  truncated Gaussian, **std 5 ms**, cut at 3 std.
- Traffic models (§5): long-lived TCP; short TCP as 30 connections of 30–50 KB with idle gaps of
  mean 10 s; CUBIC "especially" encouraged. QUIC "will exhibit behavior conceptually similar in
  nature to TCP flows".

**RFC 8869** is the wireless test cases. Cellular: a fixed peer 10 ms beyond the radio network,
end-to-end RTT **40–150 ms**, 3–30 km/h mobility, and background TCP of up to 4 Mbit/s a cell
downlink, 2 Mbit/s uplink. It says plainly that on the default bearer "packets are queued up and
transmitted once the handover is completed" rather than lost. Wi-Fi (802.11n): the wired-bottleneck
case is **1 Mbit/s, 50 ms one-way, 30 ms jitter, 300 ms drop-tail, 0% loss**, against a long-lived
TCP flow; the Wi-Fi-bottleneck case is 100 Mbit/s wired, with the radio (about 20 Mbit/s of
application throughput at MCS 11) as the bottleneck.

**Newer:** I found no successor to these three RFCs. The current IETF work is on the algorithms
themselves (the BBR draft below; L4S, RFC 9330). *(Not verified beyond these: search was
unavailable.)*

## 2. What real bad links look like

**Residual loss after the link layer is small.** Cellular and Wi-Fi both retransmit at the link
layer, so what reaches IP is mostly delay, not loss:

- LTE: Huang et al., *An In-depth Study of LTE* (SIGCOMM 2013, a large US carrier's traffic): across
  downlink TCP flows, **38.1% had no retransmission and the median retransmission rate was
  0.06%**, "comparable to those in wired networks"; "most transport-layer losses are concealed by
  physical/MAC-layer retransmission and reduced by buffering". The same paper found that the large
  downstream queues often collapse TCP's window on a single loss.
- RFC 8869 (above): the default cellular bearer queues during a handover instead of dropping.
- 3GPP's QoS classes give the design target: a packet error loss rate of 10⁻⁶ for the default
  best-effort bearer (QCI 9) and 10⁻² for conversational voice (QCI 1)
  ([summary](https://en.wikipedia.org/wiki/QoS_Class_Identifier); not read from TS 23.203 itself).
- Wi-Fi: I could not fetch a primary measurement of residual IP loss. RFC 8869 describes its loss as
  collision- and interference-driven and qualitatively unlike wired congestion loss, i.e. bursty.
  *(Not verified with numbers.)*
- For calibration, Microsoft Teams classes an audio stream as poor above **10%** average loss, 30 ms
  jitter or 500 ms round trip
  ([stream classification](https://learn.microsoft.com/en-us/microsoftteams/stream-classification-in-call-quality-dashboard)).

So **sustained random loss of 1% or more is the exception**: a weak Wi-Fi signal, a cell edge, a
satellite link. It is a legitimate stress test, which is what *lossy* is for, but it is not what a
typical phone on LTE sees, which is low loss, deep buffers and delay that varies without
reordering.

**Queues are deep where there is no AQM.**

- Comcast's measurements over millions of cable modems
  ([arXiv 2107.13968](https://arxiv.org/abs/2107.13968)): upstream latency under load averaged
  **about 250 ms without AQM**, against **15–30 ms with it** (DOCSIS-PIE).
- Netalyzr (Kreibich et al., IMC 2010, 130,000 runs): access-device buffers cluster at **128 KB and
  256 KB**, enough for "250 ms of additional latency during file transfers" even on an 8 Mbit/s
  uplink, and over a second at 1 Mbit/s.
- RFC 8868's nominal 300–500 ms and bloated 1–2 s (above).

**AQM is deployed but not universal.** DOCSIS 3.1 modems must implement DOCSIS-PIE with a **10 ms**
latency target (RFC 8034 §4.1). Linux's Wi-Fi stack has had FQ-CoDel-style queueing and airtime
fairness in mainline since 2017 (Høiland-Jørgensen et al., *Ending the Anomaly*,
[arXiv 1703.00064](https://arxiv.org/abs/1703.00064)); OpenWrt's SQM uses CAKE or fq_codel. Ookla
now reports "loaded latency" alongside idle ping
([Ookla](https://www.ookla.com/articles/introducing-loaded-latency)), but I could not fetch its
aggregate numbers. Starlink was found to use drop-front queues rather than drop-tail or per-flow
queueing ([arXiv 2605.27717](https://arxiv.org/abs/2605.27717)).

**What a bad-link profile should use, then.** For *congested*: a few Mbit/s, a tail-drop queue of
**about 200–250 ms** (the no-AQM norm, and inside RFC 8867's range), competing bulk TCP, and little
or no random loss; the bottleneck itself supplies the loss. For *mobile*: a round trip in RFC 8869's
40–150 ms, jitter that never reorders, a deep queue (RFC 8868's 300 ms), and loss of a few tenths of
a percent at most. For *lossy*: RFC 8868's independent random loss at a value between its 1% and 5%
points, which 3% is.

## 3. Emulating a bottleneck with netem, correctly

### 3.1 How netem holds packets (from `sch_netem.c`)

- Every packet goes into netem's time-sorted queue (`tfifo`) with its own `time_to_send`. A child
  qdisc, if there is one, receives packets **after** they leave the tfifo (`netem_dequeue`). A child
  therefore cannot undo reordering that the tfifo already did; the man page's advice to "replace the
  internal queue discipline tfifo with a simple FIFO" describes older kernels.
- netem's `limit` is checked against `sch->q.qlen`, which counts **every packet netem holds**: those
  only waiting out their delay, those in the child, and on `lo` both directions, ACKs included. At
  5 Mbit/s and 150 ms one-way, about 60 of `limit 100` are taken by the delay line alone.
- With `rate`, a new packet's delay is measured from the last queued packet: `delay -= last->time_to_send
  - now; delay = max(0, delay); now = last->time_to_send`, then the serialisation time is added. So
  a packet leaves at `max(now + delay, last packet's departure) + its own serialisation`.
  Two consequences: (a) packets **never reorder** when `rate` is set; (b) once a queue has formed,
  the propagation delay is **absorbed** into it rather than added to it. Real one-way delay is
  propagation plus queueing.
- With delay, jitter or rate set, netem orphans the packet from its socket (`skb_orphan_partial`),
  so TCP Small Queues does not throttle the sender against netem's queue.
- Correlation (`delay 80ms 30ms 25%`) blends each random draw with the previous one. The man page
  calls it "not a true statistical correlation, but an approximation"; it narrows the spread and does
  not prevent reordering (measured below).

### 3.2 Where the rate stage goes

Put netem at the root for delay, jitter and loss, with a large `limit`, and a real shaper as its
child for the bottleneck and its queue. This is the man page's own pattern (netem with a `tbf` child)
and makes the two delays add as they should: a packet waits out its propagation delay in netem, then
queues at the bottleneck.

- **`tbf rate R burst B latency Q`** gives a tail-drop FIFO bounded in time: the man page says
  `latency` is "the maximum amount of time a packet can sit in the TBF", converted to a byte limit
  from the rate and burst. This is the typical unmanaged home router. `burst` must be at least the
  rate divided by the kernel's HZ, or the rate is not reached (5 Mbit/s at HZ 250: 2.5 KB; 16 KB is
  safe).
- **`cake bandwidth R`** (or `htb` + `fq_codel`) is a router with AQM. It is worth a run, as the case
  a good router produces (§5), but not as the default *congested*, which should be the common bad
  case.
- **netem's own `rate` with a small `limit`** (the current profile) has the problems in §3.1: the
  limit includes the delay line and the reverse direction, and the delay disappears into the queue.

### 3.3 Jitter without reordering

Measured: UDP at 500 packets a second (3,000 packets) and at 50 a second (voice's rate, 500 packets),
through netem alone on a 1500-byte loopback.

| netem arguments | Out of order, 500/s | Out of order, 50/s | Delay p5 / p50 / p95 |
| --- | --- | --- | --- |
| `delay 80ms 30ms 25%` (current *mobile*) | 47.4% | 22.6% | 53 / 80 / 107 ms |
| `delay 150ms 50ms 25%` (current *congested* without its `rate`) | | 32.4% | 106 / 149 / 196 ms |
| `delay 40ms 10ms distribution normal` | 42.8% | | 24 / 40 / 56 ms |
| `delay 40ms 10ms distribution normal rate 1gbit` | 0.1% | | 39 / 49 / 61 ms |
| `delay 40ms 10ms 50% distribution normal rate 1gbit` | 0.0% | | 39 / 48 / 60 ms |
| `delay 40ms slot 0ms 20ms` | 0.0% | | 41 / 46 / 56 ms |

Two ways keep order:

- **`rate` set far above the bottleneck** (`rate 1gbit`, 12 µs a 1500-byte packet). By the source
  above, this is exactly RFC 8868's NR-BPDV: each packet gets its jittered delay unless that would
  put it ahead of the packet before it. The cost is a delay distribution shifted upward (median
  49 ms for a 40 ms base), more so in dense bursts. Use the RFC's std of 5 ms where jitter is not the
  point, 10 ms for a mobile link.
- **`slot MIN MAX`** releases packets only at slot boundaries spaced between MIN and MAX apart, "a
  crude approximation of bursty MACs such as DOCSIS, WiFi, and LTE" (man page). In order, but bursty
  by construction.

The table recommends `rate 1gbit`, because it is the RFC's model and keeps per-packet variation.

### 3.4 The loopback

- **No ingress shaping is needed on `lo`.** A packet sent to a local address is transmitted on `lo`
  and so passes its root qdisc once, whichever way it goes: server to browser, browser to server,
  and browser to browser for voice on a direct path. Ingress shaping (via `ifb`) is only needed on a
  real interface, to shape what arrives. Voice through our TURN relay would cross `lo` twice each
  way (browser → relay → browser) and so get **twice the delay and nearly twice the loss**; the
  acceptance uses `iceTransportPolicy: "all"`, which picks the direct host path on one machine.
- **But one qdisc means one shared link.** On `lo` the bottleneck is shared by both directions, both
  browsers, the film's ACKs and voice. A real host and guest are in different homes, each with a
  full-duplex access link. Two consequences: the 5 Mbit/s is split between two viewers' films; and
  ACKs and returning voice queue behind film data, so a measured round trip counts the queue twice.
  In the 5 Mbit/s runs below, a BBR film raised the voice probe's round trip to 150 ms median on its
  own, of which about 80 ms is the double-counted queue.
- **To make it real**, return to what D252 §1 first said, each client its own link: put each browser
  in its own network namespace joined to the server's by a veth pair, and shape each veth end's
  egress (one is that client's downlink, the other its uplink). Playwright can start a browser through
  a wrapper `executablePath` that runs `ip netns exec <ns>` and drops back to the test's user; that
  needs root for the wrapper. *(inferred: not built or tried here.)* Until then, size the `lo` rate
  for both clients together, or accept that *congested* means a household sharing one link.

### 3.5 Packet size

`lo` at MTU 1500 with `gso_max_size` 1500 (as the acceptance now sets) makes TCP send MSS-sized
packets, so netem and the shaper see what a real link carries. `lo` has no GRO and no device queue
below the qdisc, so the qdisc is the only queue.

## 4. Competing bulk traffic

RFC 8867 §5.6's competitor is **one long-lived TCP flow** with the default congestion control
(CUBIC on Linux), running before the media starts and for the whole test. No new tool is needed:

- In the acceptance, start a small HTTP server on another loopback port that streams zeros, and a
  client that reads and discards, both in the Playwright process (Node's `http`), or `curl -o
  /dev/null` against it. Both use the system's default congestion control. A download of a large
  file from the Orpheum server itself would also do, but shares its handlers.
- iperf3 is the usual generator, and flent's RRUL (four TCP flows each way plus latency probes) is
  the standard bufferbloat test ([RRUL spec](https://www.bufferbloat.net/projects/bloat/wiki/RRUL_Spec/)).
  Both would be new packages in the development image, the owner's decision.

**Loopback TCP is realistic once there is a bottleneck.** The flows measured here reached the
configured rates, retransmitted at roughly the configured loss, and their smoothed round trips match
the base delay plus the queue. The man page's warning that netem must sit on the receiver's ingress
for realistic TCP is about TSQ throttling a sender against its own netem; netem orphans the packet
when it delays it (§3.1), and the transfers measured were not held below the shaper's rate. What
stays unrealistic is the shared link (§3.4), and that the sender and receiver share one CPU.

## 5. Measurements: CUBIC and BBR under each profile

One TCP transfer of 20 seconds, the sender's congestion control as named; "retrans" is the
sender's `tcpi_total_retrans`. Mainline BBR (v1) on kernel 6.12.

| Profile | CUBIC | BBR |
| --- | --- | --- |
| current *mobile* (`delay 80ms 30ms 25% loss 1%`, no rate) | 1.51 Mbit/s, srtt 166 ms | 15.96 Mbit/s, 5212 retrans, srtt 133 ms |
| current *congested* (`… loss 3% rate 5mbit limit 100`) | 0.49 Mbit/s, srtt 328 ms | 3.82 Mbit/s, srtt 425 ms |
| current *congested* without its loss | 3.05 Mbit/s, srtt **472 ms** | 4.11 Mbit/s, srtt 392 ms |
| recommended *congested* (§0), alone | 4.06 Mbit/s, srtt 69 ms | 4.59 Mbit/s, srtt 148 ms |
| recommended *mobile* (§0) | 8.43 Mbit/s, srtt 200 ms | 17.70 Mbit/s, srtt 148 ms |
| recommended *lossy* (§0) | **1.04 Mbit/s** | **17.66 Mbit/s**, 943 retrans |

The Mathis bound (RFC 3819 §8.5) predicts the CUBIC rows: rate ≈ 1.22 × MSS / (RTT × √p). At
1448-byte segments, 3% loss and a 300 ms round trip it is about 0.27 Mbit/s; at 1% and 160 ms,
0.88 Mbit/s. CUBIC does a little better than Reno at these window sizes, which the measured 0.49
and 1.51 show. The claim that 3% random loss makes a TCP download crawl is right; so does 1%.

BBR's 5212 retransmissions under the current *mobile* profile are mostly spurious: that profile
reorders nearly half the packets (§3.3).

**With competition and a voice probe**, recommended *congested* (5 Mbit/s, 200 ms tail-drop queue,
0.5% loss each way). The probe is a 160-byte UDP echo every 20 ms; its round trip crosses the shared
queue twice (§3.4).

| Film's congestion control | Film | Background CUBIC | Probe RTT p50 / p95 | Probe loss |
| --- | --- | --- | --- | --- |
| (idle) | | | 61 / 73 ms | 0.2% |
| CUBIC, alone | 3.12 Mbit/s | | 67 / 82 ms | 0.5% |
| CUBIC, with background | 2.16 Mbit/s | 2.50 Mbit/s | 94 / 130 ms | 1.2% |
| BBR, alone | 4.41 Mbit/s | | 150 / 195 ms | 0.6% |
| BBR, with background | 2.61 Mbit/s | 2.13 Mbit/s | 307 / 413 ms | 1.5% |
| CUBIC, with background, `cake bandwidth 5mbit` instead of `tbf` | 2.19 Mbit/s | 2.01 Mbit/s | 69 / 78 ms | 0.8% |
| BBR, with background, `cake` | 2.28 Mbit/s | 2.01 Mbit/s | 70 / 79 ms | 0.7% |

What this says *(inferred from one run each)*: at 0.5% loss on a tight bottleneck, BBR gains the
film little (about 10–40%) and takes a little more than an even share from a CUBIC neighbour, but
keeps the tail-drop queue fuller, which is what voice feels. On an AQM router the difference
disappears. Under heavy random loss the gain is an order of magnitude.

## 6. BBR today, and how to turn it on

**Versions.**

- Mainline Linux's `net/ipv4/tcp_bbr.c` is **BBRv1** (states STARTUP, DRAIN, PROBE_BW, PROBE_RTT;
  "the core algorithm does not react directly to packet losses"), as of `torvalds/linux` master on
  2026-10-07. Present since Linux 4.9.
- **BBRv3 is not in mainline.** It is released out of tree by Google
  ([google/bbr v3](https://github.com/google/bbr/blob/v3/README.md)), dual GPLv2/BSD. Google's IETF
  117 slides (July 2023) said BBRv3 was "TCP CC for all Google.com public Internet traffic" and all
  internal WAN traffic, under A/B test for YouTube TCP, with a plan to submit it for mainline "in
  August" and upgrade the `bbr` module in place; it has not landed. BBRv2 is "obsolete/deprecated" in
  the same slides.
- The IETF specification is
  [draft-ietf-ccwg-bbr](https://datatracker.ietf.org/doc/draft-ietf-ccwg-bbr/) (July 2026 revision,
  Experimental), which specifies BBRv3 and lists Linux TCP (Google's branch) and Chromium's QUIC as
  production implementations. BBRv3 caps its probing at a loss rate of **2%** (`BBR.LossThresh`) per
  round; BBRv1 has no such threshold, which is why it rides through random loss and also why it can
  push a neighbour hard.

**Under random loss** (Google's own figures,
[GCP blog](https://cloud.google.com/blog/products/networking/tcp-bbr-congestion-control-comes-to-gcp-your-internet-just-got-faster)):
on a 10 Gbit/s, 100 ms path with 1% loss, CUBIC about 3.3 Mbit/s and BBR over 9,100 Mbit/s; on a
10 Mbit/s, 40 ms last mile with a 1000-packet buffer, median RTT 1090 ms with CUBIC against 43 ms
with BBR; on YouTube, 4% more throughput, 33% lower RTT and 11% longer mean time between rebuffers.
Our own runs (§5): 17.7 against 1.0 Mbit/s at 3% loss.

**Fairness.** BBRv1's steady-state share against loss-based flows depends on the buffer, not the
number of flows: it can take more than its share in shallow buffers and holds about one round trip
of queue in deep ones (the steady-state model of Ware et al., IMC 2019, confirmed for BBRv1 by
[arXiv 2505.07741](https://arxiv.org/abs/2505.07741), which also finds BBRv3 no longer fits it).
Wi-Fi studies find BBRv3 fair under FQ-CoDel or CAKE and CUBIC dominant behind a FIFO
([arXiv 2512.18259](https://arxiv.org/abs/2512.18259)). For two viewers of a home server this is a
concern for their own households' other traffic only, and D262 §2's pacing cap
(`SO_MAX_PACING_RATE`) bounds it while voice is live: BBR's pacing rate is clamped to
`sk_max_pacing_rate` (`bbr_bw_to_pacing_rate`).

**Who uses it.** Google: google.com, YouTube and Google Cloud's front ends (GCP blog; IETF 117).
Dropbox measured faster downloads "across all percentiles" in a BBR experiment on its edge
([Dropbox](https://dropbox.tech/infrastructure/optimizing-web-servers-for-high-throughput-and-low-latency)).
Netflix sponsored FreeBSD's `tcp_bbr` (FreeBSD 13.0+, written by Randall Stewart; FreeBSD man page);
whether Netflix's CDN runs it by default is not verified here. Cloudflare's use is not verified here.

**Whose congestion control matters.** The sender's. For the film the sender is the server, so the
server's socket decides; the browser's congestion control only governs what it uploads, which for
Watch Together is requests and ACKs. Voice is WebRTC over UDP with the browser's own rate control
and is untouched by this choice, except through the bottleneck it shares with the film.

**Does BBR need `fq`?** No, since Linux 4.13: `tcp_bbr.c` says BBR "might be used with the fq qdisc
… otherwise TCP stack falls back to an internal pacing using one high resolution timer per TCP
socket and may use more resources". `fq` matters for CPU at scale, not for two viewers. (Dropbox's
note says the same: fq was "a requirement for BBR (not anymore though)".)

**Who may select it** (`tcp_cong.c`, `sysctl_net_ipv4.c`):

- `setsockopt(TCP_CONGESTION, "bbr")` succeeds for an **unprivileged** process once the module is
  loaded, because `tcp_bbr_cong_ops` carries `TCP_CONG_NON_RESTRICTED`. `tcp_allowed_congestion_control`
  does not need changing for BBR; it is for algorithms without that flag.
- If the module is **not loaded**, the kernel only loads it on demand when the caller is
  `capable(CAP_NET_ADMIN)` in the initial user namespace (`tcp_ca_find_autoload`); an unprivileged
  caller gets `ENOENT`. Verified here: as the sandbox's user, `ENOENT`; as root, the call loaded
  `tcp_bbr` and `tcp_available_congestion_control` became `reno cubic bbr`; then the unprivileged
  call succeeded. (That load persists in the host kernel until it reboots.)
- `tcp_congestion_control` and `tcp_allowed_congestion_control` appear in each network namespace,
  but the allowed flag lives on the shared algorithm, and a non-initial namespace may only set its
  default to an unrestricted algorithm.
- Passive connections inherit the listener's choice (ip-sysctl.rst: "For passive connections, the
  listener congestion control choice is inherited"), so setting it once on the listener covers every
  accepted connection.

**Steps.**

1. Host: load the module at boot, e.g. `/etc/modules-load.d/bbr.conf` containing `tcp_bbr` (or
   `modprobe tcp_bbr`). Debian builds it as a module. A container cannot `modprobe` without the
   module files, but a root process in a container with `CAP_NET_ADMIN` and no user-namespace
   remapping triggers the host's loader through the `setsockopt` itself, as above; so the acceptance
   can load it with one privileged call next to its `tc` calls.
2. Server, Go standard library only (`syscall.TCP_CONGESTION` is 0xd on Linux; `golang.org/x/sys/unix`
   is already allowed too). In `cmd/orpheum/main.go`'s `net.ListenConfig`:

   ```go
   config := net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
       var setErr error
       if err := raw.Control(func(fd uintptr) {
           setErr = syscall.SetsockoptString(int(fd), syscall.IPPROTO_TCP, syscall.TCP_CONGESTION, "bbr")
       }); err != nil {
           return err
       }
       if setErr != nil {
           // ENOENT: module not loaded; EPERM: restricted. Keep the default, say so once.
           logger.Warn("tcp congestion control stays the kernel default", "wanted", "bbr", "err", setErr)
       }
       return nil
   }}
   ```

   A Linux-only file with a no-op elsewhere, as `internal/platform/pacing` does for
   `SO_MAX_PACING_RATE`. A test can read it back with `getsockopt(TCP_CONGESTION)` on an accepted
   connection, as `pacing_linux_test.go` reads the pacing rate. *(Where it lives is a design choice;
   whether it is a setting is D262-adjacent and the owner's.)*
3. Confirm on a live connection: `ss -ti` shows `bbr` and its `bw`/`mrtt` fields.

**Recommendation.** Turn BBR on for the media listener. It is the largest single lever for the film
on a lossy link (17×), it costs nothing on a clean one, it needs no new dependency, and its known
cost on a tail-drop bottleneck (a fuller queue that voice feels) is already bounded while voice is
live by D262 §2's pacing. Keep the acceptance's *congested* run as the check that voice still holds
its bounds with BBR on. Do not wait for BBRv3; if it reaches mainline, the same `"bbr"` name is
meant to carry it.

## 7. HTTP/3 and QUIC

QUIC does its congestion control in user space at the sender, so the picture is the same with a
different sender: what matters is the **server's** QUIC stack. RFC 9002's default is NewReno-like
("similar to TCP NewReno"), loss-based, and a sender "can unilaterally choose a different algorithm
to use, such as CUBIC". Google's servers run BBR over QUIC (the BBR draft lists
Chromium's QUIC BBR as production), so YouTube over QUIC tolerates loss *(inferred)*; that is their
server, not the browser.

For us: Go's standard library has no HTTP/3 server; `quic-go` would be a new dependency, and its
congestion control is **CUBIC only** (`internal/congestion` holds `cubic.go` and `cubic_sender.go`,
nothing else). It would not help the film on a lossy link and would add a UDP path some networks
treat worse. QUIC's real gains (no head-of-line blocking between streams, unambiguous
retransmissions) do not address a single download's response to random loss. **No change.**

## 8. Corrections to the claims and to what is built

1. **RFC 8868 is not the wireless test cases.** It is the evaluation guidelines with the parameter
   lists (§1). The wireless test cases are RFC 8869.
2. **Correlated jitter does not stop reordering**, and a `pfifo` child does not either (§3.1, §3.3).
   The current *mobile* profile reorders 22.6% of 50-packet-a-second voice and 47% of a 500-a-second
   stream. This contradicts the acceptance's comment (`web/e2e/acceptance-server.ts`: "Jitter is
   correlated … as on a real link") and D262 §1's "jitter that is correlated rather than independent",
   and the advice in [voice-bad-links.md](voice-bad-links.md) §6 and its ranked recommendation 1
   to use correlation or a `pfifo` child. Only `rate` (RFC 8868's NR-BPDV) or `slot` keep order.
3. **`limit 100` is not a quarter-second queue.** netem's limit counts the packets it is only
   delaying and the other direction's, and with `rate` the 150 ms delay is absorbed into the queue.
   Without its loss, the current *congested* profile gave CUBIC a 472 ms smoothed round trip; the
   same comment and [voice-bad-links.md](voice-bad-links.md) §6's "netem `limit` to about 100–250 ms
   of packets" are wrong in the same way. Bound the queue in a shaper child (`tbf latency`).
4. **The film's trouble was not only *congested*.** *mobile*'s 1% random loss with its reordering
   held a CUBIC download to 1.5 Mbit/s. And 1% sustained random loss is not typical of mobile data:
   LTE's median retransmission rate was 0.06% (§2).
5. **On `lo`, a rate limit is one link for everything**: both directions, both browsers, voice and
   ACKs (§3.4). 5 Mbit/s on `lo` is about 2.5 Mbit/s for each viewer's film, and a round trip counts
   the queue twice. D252's "As built" already notes that host and guest share the shaping.
6. **"Typical home router, 100–250 ms"** is right for routers without AQM (Comcast: about 250 ms
   average upstream; Netalyzr: 128–256 KB buffers), on the short side of RFC 8868's 300–500 ms
   "nominal". AQM routers hold 10–30 ms. 200 ms is a fair middle.
7. **BBR**: the brief's steps are right in outline. Corrections: `tcp_allowed_congestion_control`
   does not need changing for BBR (it is unrestricted by design); what an unprivileged server needs
   is only that the module is loaded. BBRv3 is not merged. `fq` is not required. The sender's
   congestion control is the one that matters, as the brief says.
8. **"With realistic packet sizes, 3% random loss makes each TCP download crawl"**: right, measured
   at 0.49 Mbit/s for CUBIC under the current profile and 1.0 Mbit/s with 3% loss and no tight
   bottleneck.

## Sources

- RFC 8867, *Test Cases for Evaluating Congestion Control for Interactive Real-Time Media*:
  <https://www.rfc-editor.org/rfc/rfc8867.txt>
- RFC 8868, *Evaluating Congestion Control for Interactive Real-Time Media*:
  <https://www.rfc-editor.org/rfc/rfc8868.txt>
- RFC 8869, *Evaluation Test Cases for Interactive Real-Time Media over Wireless Networks*:
  <https://www.rfc-editor.org/rfc/rfc8869.txt>
- RFC 3819 §8.5 (TCP throughput formulae): <https://www.rfc-editor.org/rfc/rfc3819.txt>
- RFC 8034 (DOCSIS-PIE, 10 ms target): <https://www.rfc-editor.org/rfc/rfc8034.txt>
- RFC 9002 (QUIC loss detection and congestion control): <https://www.rfc-editor.org/rfc/rfc9002.txt>
- Linux, read on 2026-10-07: `net/sched/sch_netem.c`, `net/ipv4/tcp_bbr.c`, `net/ipv4/tcp_cong.c`,
  `net/ipv4/sysctl_net_ipv4.c`, `net/ipv4/tcp.c`, `Documentation/networking/ip-sysctl.rst`:
  <https://github.com/torvalds/linux>
- Man pages tc-netem(8), tc-tbf(8), tc-cake(8): <https://man7.org/linux/man-pages/man8/tc-netem.8.html>,
  <https://man7.org/linux/man-pages/man8/tc-tbf.8.html>, <https://man7.org/linux/man-pages/man8/tc-cake.8.html>
- draft-ietf-ccwg-bbr (BBRv3): <https://datatracker.ietf.org/doc/draft-ietf-ccwg-bbr/>
- Google BBR v3 release: <https://github.com/google/bbr/blob/v3/README.md>
- Cardwell et al., *BBRv3: Algorithm Bug Fixes and Public Internet Deployment*, IETF 117:
  <https://datatracker.ietf.org/meeting/117/materials/slides-117-ccwg-bbrv3-algorithm-bug-fixes-and-public-internet-deployment-00>
- Google Cloud, *TCP BBR congestion control comes to GCP*:
  <https://cloud.google.com/blog/products/networking/tcp-bbr-congestion-control-comes-to-gcp-your-internet-just-got-faster>
- Dropbox, *Optimizing web servers for high throughput and low latency*:
  <https://dropbox.tech/infrastructure/optimizing-web-servers-for-high-throughput-and-low-latency>
- FreeBSD tcp_bbr(4): <https://man.freebsd.org/cgi/man.cgi?query=tcp_bbr>
- Huang et al., *An In-depth Study of LTE*, SIGCOMM 2013:
  <https://conferences.sigcomm.org/sigcomm/2013/papers/sigcomm/p363.pdf>
- Kreibich et al., *Netalyzr: Illuminating the Edge Network*, IMC 2010:
  <http://www.icir.org/christian/publications/2010-imc-netalyzr.pdf>
- Livingood et al., *Improving Latency with Active Queue Management (AQM) During COVID-19*:
  <https://arxiv.org/abs/2107.13968>
- Høiland-Jørgensen et al., *Ending the Anomaly*: <https://arxiv.org/abs/1703.00064>
- *BBR's Sharing Behavior with CUBIC and Reno*: <https://arxiv.org/abs/2505.07741>
- *TCP BBR Performance over Wi-Fi 6*: <https://arxiv.org/abs/2512.18259>
- *Characterizing the Configuration of Starlink Queuing*: <https://arxiv.org/abs/2605.27717>
- Microsoft Teams stream classification:
  <https://learn.microsoft.com/en-us/microsoftteams/stream-classification-in-call-quality-dashboard>
- quic-go congestion control: <https://github.com/quic-go/quic-go/tree/master/internal/congestion>
- flent RRUL: <https://www.bufferbloat.net/projects/bloat/wiki/RRUL_Spec/>

**Limits of this research.** Web search was unavailable, so measurement sources are the ones
reachable by known URL or arXiv's catalogue; Wi-Fi residual-loss figures and Ookla's and
Waveform's aggregate bufferbloat numbers are missing. The measurements are single runs in a
namespace on this sandbox's kernel, not a distribution. The per-client namespace setup (§3.4) and the
Go change (§6) were not built.
