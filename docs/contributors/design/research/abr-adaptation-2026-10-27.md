> **ABOUTME:** Why the chunked player's first link sample read 1 Mb/s on a 12 Mb/s path, and what
> ABR research and practice say about start-up, multi-rung climbs, oscillation and unreliable links:
> the evidence behind D352's proposal, with what it takes and rejects from each source. 2026-10-27.

# Adapting to the link: start-up, climbs, swings and flaky networks

- **Date:** 2026-10-27, after the owner's report of 2026-10-26 (The Unknown on its lowest quality
  for minutes on staging). **Feeds** [D352](../../decisions/working-notes.md) (proposed).
- **Builds on:** [abr-controller.md](abr-controller.md) (2026-10-07, the algorithms surveyed for
  D265), [slow-links.md](slow-links.md), [cold-start.md](cold-start.md),
  [reachability-approaches-2026-10-09.md](reachability-approaches-2026-10-09.md) §9.4 (what a data
  channel carries).
- **Cache:** `/yoloai/cache/abr-research/`: each paper's PDF, the players' sources as read
  (hls.js, Shaka, dash.js, ExoPlayer/Media3, pion/sctp, fetched 2026-10-27), and a notes file per
  source.
- **Labels:** **[1P]** read at the source; **[S]** a search engine's summary only; **[I]** inference.
- **Not reached:** Mok et al. and Cranley et al. on switching (cited by FESTIVE, not read); any
  published description of YouTube's client controller (none found); a measurement of a fresh data
  channel's ramp on the owner's own path (none exists; §2).

---

## 1. Why the first sample read 1 Mb/s

### 1.1 What the log shows

From `/yoloai/files/bug-the unknown-low-quality-2026-10-26.log` (staging core, 19:14–19:17 UTC)
and the same film's earlier play at 19:07 (`bug-the unknown-audio-seek-2026-10-26.log`):

| When | What | Says |
| --- | --- | --- |
| 19:07:44–48 | the film's own picture is cut into 80 chunks, all made in 3.2 s (`a run of chunks finished … chunksMade=80`); the play starts at `linkKbps=12546` | the top quality's chunks are on disk from here on, and the path carried 12.5 Mb/s over warm connections |
| 19:11:11 | a later play, after a track change: `linkKbps=11294` | the same path, 11 Mb/s |
| 19:14:48.9, 19:14:49.9 | two new WebRTC connections, c136 and c137 (`connectMs` 570, 682) | the page's transport opens a connection beyond its first only while its requests crowd the ones it has (`web/packages/sdk/src/rtc-transport.ts`, `clientConnections`) |
| 19:14:52.7 | the film page's playback starts, on the top quality (`level 4`) | — |
| ~19:14:56.5 | the steps: `frag 4/0 loaded`, then `chose ceiling 4 to 0: link 1008 kb/s, 2.9 s ahead` | **one** chunk had measured the link (2.9 s ahead is that chunk alone), and hls.js's estimate of one sample is that sample |
| 19:14:56.6 | the first ask for the lowest quality's chunk 1 and its head | the step down was taken on that one sample |
| 19:15:13 – 19:15:51 | every seek report: `linkKbps` 2784, 3378, 4505, 4960, 5310, 5627 | the estimate recovered slowly (§1.4); the ceiling stayed at 0, D247 §9's step-up holding each quality 120 s |

So the chunk was not being made (it had been on disk for seven minutes), and the same bytes had
measured the path at 12.5 Mb/s. What differed was the transport: a page whose requests had just
crowded its connections. **[1P]** for each line. The split of that one chunk's time between waiting
and arriving was not logged, which is why the fix logs it (§1.5).

The path is not the house's network, though the owner was at home: every play is
`over=webrtc clientAddress=11.22.33.144`, the carrier's address, so the browser reaches staging
through the punched WebRTC path (the owner's DS-Lite line), and 11–14 Mb/s is that path's, not a
LAN's.

### 1.2 Where the time went: hls.js's sample

hls.js 1.7.3 samples a chunk in `AbrController.onFragBuffered` as

    processingMs = parsing.end − loading.start − min(loading.first − loading.start, ttfbEstimate)

and its TTFB estimate is a moving average whose each sample is weighted by √2·e^(−s²/2), s the
wait in seconds (`EwmaBandWidthEstimator.sampleTTFB`, `web/node_modules/hls.js/dist/hls.mjs`).
**[1P]** The weighting treats a long wait as an outlier: a 60 ms wait for the picture's head weighs
1.41, a 1.5 s wait for the first chunk 0.46, so the estimate after both is about 0.5 s and **about
1 s of the chunk's 1.5 s wait is counted as the link carrying it**. A 210 KB chunk that waited 1.5 s
and then arrived in 200 ms, 8.4 Mb/s, is sampled at 1.3 Mb/s
(`web/packages/player/src/abr-measure.test.ts`, red with hls.js's sampling restored). **[1P]**

### 1.3 The candidates, against the evidence

- **The wait for the first byte (TTFB) counted as transfer: the cause.** On the WebRTC path a
  chunk's request goes from hls.js's XHR to the service worker, to the page, onto a data channel of
  its own (opened with a round trip of its own), on whichever connection has the fewest requests
  under way, queued behind a dozen when the page is busy (`requestsPerConnection`); the core then
  reads the chunk and sends its head. All of that is before the first byte, and most of it stays in
  hls.js's sample (above). The XHR's `loading.first` is set at its HEADERS_RECEIVED, which on this
  path is when the page has the core's head, so first byte to last byte is the body alone. **[1P]**
- **A tiny first fragment dominated by latency.** The rule that a chunk under 128 KB measures
  nothing (`measuresFromBytesAtLeast`, A Woman of Paris on 2026-10-18) already stands; this chunk
  passed it, or it would not have counted. The class is the same, a fixed wait divided into few
  bytes, and Huang et al. found it in commercial players over TCP (§2). **[1P]**
- **A chunk served as it is made.** Not this one: the store had finished at 19:07:48. And the core
  holds a request for a chunk not made yet, then answers 425 (`waited for a chunk not made yet …
  outcome=askAgain`), so production waits fall before the head and are outside first-to-last byte
  too; hls.js times each ask afresh (a new loader, new stats, `FragmentLoader.load`). A core that
  sent its head and then trickled a chunk as FFmpeg wrote it would measure the encoder; it does not.
  **[1P]**
- **Time queued behind other messages on the data channel.** Part of the cause, before the head
  (above). After the head, a chunk shares its connection's SCTP association with the requests
  beside it (the sound's chunk, posters): that is the link being shared, and is the link's.
  **[1P]/[I]**
- **A fresh connection's congestion window.** Not removed by the fix, and not the link's capacity
  either: SCTP starts at a window of about 4 KB (§2), so the first hundreds of kilobytes on a
  connection a few seconds old arrive slower than the path allows. This is the start-up uncertainty
  D352 §1 answers, not a measurement error. **[1P]/[I]**

### 1.4 Why one sample held so long: hls.js weighs samples by their seconds

hls.js's estimate is the lower of a fast and a slow moving average (half-lives 3 s and 9 s for video
on demand, `abrEwmaFastVoD`, `abrEwmaSlowVoD`), and **each sample's weight is the seconds its chunk
took to load**, bias-corrected so that one sample is the estimate outright (hls.js `docs/API.md`:
"each of the sample is weighted by the fragment loading duration"). **[1P]** The estimate is so a
byte-weighted harmonic mean: a slow sample outweighs a fast one by the ratio of their times. A first
chunk that took 4–8 s at 1 Mb/s weighs 4–8; each chunk of the lowest quality loaded at 12 Mb/s
weighs 0.25–0.5. Worked through, the estimate reaches 90% of 12 Mb/s only after **30 to 80 such
chunks**, three to eight minutes of film at 6 s chunks, and the faster the link, the slower the
recovery, since a fast chunk weighs less. **[I]** The owner's log shows the shape: 2.8, 3.4, 4.5,
5.0, 5.3, 5.6 Mb/s over the first 40 s of seeks, under half the path's 12 Mb/s. **[1P]**

This is why D352 §2 has the controller choose by an estimate of its own, five samples each counted
once, rather than tuning hls.js's half-lives: no half-life undoes a weighting by seconds.

### 1.5 The fix (built, 2026-10-27)

The controller samples hls.js's estimator itself, over the time the chunk's bytes took to arrive,
from its answer's head to its last byte (`arrivalMs`, `web/packages/player/src/abr-choice.ts`;
`OrpheumAbrController.onFragBuffered`), which is what D247 §9 measures, "the speed measured while
chunks download". hls.js's own loader reckons a chunk's `bwEstimate` the same way
(`stats.total × 8000 / (loading.end − loading.first)`), and dash.js keeps latency and throughput
apart (§3). Each step down by the link now notes the chunk that last measured it: its quality,
bytes, time to arrive and the wait for its first byte. **[1P]**

---

## 2. How throughput samples go wrong

- **Huang, Handigol, Heller, McKeown, Johari, "Confused, Timid, and Unstable: Picking a Video
  Streaming Rate is Hard", ACM IMC 2012**, doi:10.1145/2398776.2398800,
  [PDF](https://cs.uwaterloo.ca/~brecht/courses/854-http-video-2012/readings/Picking-a-Video-Streaming-Rate-is-Hard-imc-2012.pdf).
  **[1P]** One service fell from 1750 to 235 kb/s on a 5 Mb/s link once a competing flow started,
  in 91% of 76 runs. A full buffer makes the player fetch one 4 s chunk per 4 s; the idle gap
  exceeds the retransmission timeout, TCP resets its window to its initial size, the chunk ends
  before the window recovers, the measured rate is low, a lower rate is chosen, its smaller chunks
  measure lower still: the downward spiral. Another service reopened its connection at every switch
  and paid slow start each time. Their fixes: less conservatism, the 80th percentile of the last 10
  samples, and larger requests. *D352 takes:* a sample on a fresh or idle connection reads low,
  and smaller chunks read lower (Q6); its estimate rests on several samples, not one.
- **TCP's windows** **[1P]**: RFC 6928's initial window is min(10·MSS, max(2·MSS, 14600)) bytes;
  RFC 5681 §4.1 drops the window to that after an idle time over one RTO; RFC 7661 (new-CWV,
  experimental) lets a rate-limited sender keep its window through such periods. A short transfer
  is round trips and slow start, not capacity.
- **SCTP, under every WebRTC data channel** **[1P]**: RFC 9260 §7.2.1 (which obsoletes RFC 4960)
  starts an association at min(4·PMDCS, max(2·PMDCS, 4404)) bytes, about 4.4 KB, a third of TCP's,
  grows it in slow start by at most one PMDCS per SACK, and decays it while idle; RFC 8831 runs SCTP
  over DTLS over ICE, one window per association for all its channels. pion/sctp, the core's,
  starts at 4380 B (an initial MTU of 1191) but grows by min(bytes acked, cwnd) in slow start,
  TCP's doubling, the RFC's slower rule commented out; it has no idle decay that the source shows.
  The 2026-10-09 measurement (reachability-approaches §9.4) found one connection window-bound,
  15–20 Mb/s at a 50 ms round trip, which is why the transport opens up to four. *D352 takes:* the
  first sample on a connection opened seconds earlier is ramp-limited; §1 makes it decide nothing
  alone.
- **The ON-OFF pattern** (Akhshabi et al. 2012, §3): a player measures only while it downloads, so
  between downloads it knows nothing, and two players whose downloads do not overlap each measure
  the whole link. A household of two on one upload is that case.

---

## 3. What the algorithms and players do

### Stability and oscillation

- **Jiang, Sekar, Zhang, "Improving Fairness, Efficiency, and Stability in HTTP-based Adaptive
  Video Streaming with FESTIVE", ACM CoNEXT 2012**,
  [PDF](https://conferences.sigcomm.org/co-next/2012/eproceedings/conext/p97.pdf). **[1P]** The
  harmonic mean of the last 20 chunks' throughputs, as "more robust to larger outliers", beat the
  mean, the median and an EWMA. No switch at all before enough samples: "its bandwidth estimate
  will be unreliable". Up one level at a time, from level k only after k chunks; down whenever the
  bitrate exceeds 0.85 of the estimate. A delayed update weighs a stability cost of 2ⁿ, n the
  switches in the last 20 s, against efficiency. Randomised request times end synchronised
  players. *D352 takes:* the harmonic mean (§2), no latching before several samples (§1), and a
  cost that grows exponentially with recent swings (§3's doubling hold). *Rejects:* its 20-sample
  window (minutes of chunks at our lengths) and its k-chunk climb, which is the slow climb the
  owner rejects.
- **Akhshabi, Begen, Dovrolis, "An Experimental Evaluation of Rate-Adaptation Algorithms in
  Adaptive Streaming over HTTP", ACM MMSys 2011**, doi:10.1145/1943552.1943574,
  [PDF](https://www.cc.gatech.edu/~dovrolis/Papers/final-saamer-mmsys11.pdf). **[1P]** Smooth
  Streaming climbed from its lowest to its top in 5–10 s, four steps, never more than two levels
  at once; a 2 s dip kept it low for 40–80 s, which the authors call wrong: "spikes should be
  absorbed by the buffer". Two players oscillated together. *D352 takes:* a short dip must not cost
  minutes (§1, §2).
- **Akhshabi, Anantakrishnan, Begen, Dovrolis, "What Happens When HTTP Adaptive Streaming Players
  Compete for Bandwidth?", ACM NOSSDAV 2012**,
  [PDF](https://www.streamingmediablog.com/wp-content/uploads/2012/07/nossdav-akhshabi.pdf).
  **[1P]** The steady state's ON-OFF pattern is "the main root cause" of instability and
  unfairness; instability peaks when the fair share falls mid-ladder. *D352 takes:* oscillation is
  real on a shared link, so the controller watches for it (§3); randomisation is left for when two
  of our players share one link and are seen to swing together.

### Buffer-based

- **Huang, Johari, McKeown, Trunnell, Watson, "A Buffer-Based Approach to Rate Adaptation:
  Evidence from a Large Video Streaming Service", ACM SIGCOMM 2014**,
  doi:10.1145/2619239.2626296, [PDF](https://yuba.stanford.edu/~huangty/sigc040-huang.pdf).
  **[1P]** Netflix's traces swing per chunk between 17 Mb/s and 500 kb/s; a capacity estimate safe
  against them would be hopelessly conservative, and 20–30% of rebuffers came from a poor choice of
  rate. Choosing from the buffer alone (a 90 s reservoir, a linear map) cut switches 60% and
  rebuffers 10–30%, but lost 100–175 kb/s, mostly at start-up. BBA-2 uses the capacity estimate at
  start-up only: from an empty buffer it steps up a rung only when the last chunk arrived about 8×
  faster than it plays, relaxing to 2× as the buffer fills. Their conclusion: "capacity estimation
  is unnecessary in steady state … [but] important during the startup phase". *D352 takes:* the
  estimate's job is start-up and recovery, and agreement between samples is what earns a fast
  climb (BBA-2's high bar). *Rejects:* a buffer-only controller: Netflix's 240 s buffer is not ours
  (a film page preloads 10 s; steady state holds about 30 s), and it conflicts with D265 §4.

### BOLA

- **Spiteri, Urgaonkar, Sitaraman, "BOLA: Near-Optimal Bitrate Adaptation for Online Videos",
  IEEE INFOCOM 2016; IEEE/ACM ToN 2020**, doi:10.1109/TNET.2020.2996964,
  [arXiv](https://arxiv.org/pdf/1601.06748). **[1P]** A Lyapunov rule from the buffer alone, within
  O(1/V) of optimal utility; switching is not in its objective, and it names three causes of
  oscillation: the link varying, thresholds too dense, and a link between two rungs. BOLA-O caps a
  step up at the previous chunk's measured rate. Against it, RobustMPC's harmonic mean "takes a
  while to react" to a large drop.
- **Spiteri, Sitaraman, Sparacio, "From Theory to Practice: Improving Bitrate Adaptation in the
  DASH Reference Player", ACM MMSys 2018; ACM TOMM 2019**, doi:10.1145/3336497,
  [PDF](https://groups.cs.umass.edu/wp-content/uploads/sites/3/2019/12/From-theory-to-practice-Improving-bitrate-adaptation-in-the-DASH-reference-player.pdf).
  **[1P]** BOLA took 24 s to reach the top on a steady 8 Mb/s link; a placeholder buffer took it
  there in 3 s. dash.js's DYNAMIC uses throughput below 6–12 s of buffer and BOLA above, with
  hysteresis. *D352 takes:* a link that is plainly fast should be climbed in seconds, not tens of
  seconds (§2's multi-quality climb). *Rejects:* BOLA itself, as abr-controller.md did: it needs
  10 s or more of buffer per decision and segment sizes that track the declared rate, which our
  copied top quality's do not.

### MPC and RobustMPC

- **Yin, Jindal, Sekar, Sinopoli, "A Control-Theoretic Approach for Dynamic Adaptive Video
  Streaming over HTTP", ACM SIGCOMM 2015**, doi:10.1145/2785956.2787486,
  [PDF](https://conferences.sigcomm.org/sigcomm/2015/pdf/papers/p325.pdf). **[1P]** QoE =
  Σ quality − λ·Σ|quality change| − μ·rebuffer time − μs·start delay, λ = 1, μ = μs = 3000 with
  quality in kb/s (one second of rebuffering costs as much as one chunk 3 Mb/s lower); "avoid
  instability" sets λ = 3. RobustMPC plans against a lower bound: the harmonic mean of the past 5
  chunks divided by (1 + the largest absolute percentage error of its predictions over the past
  5). Plain MPC gained nothing on a link with 40% prediction error; above 25%, buffer-based did
  better. *D352 takes:* the lower bound as its confident estimate (§2), and the error as its
  measure of agreement (20% names one rung). *Rejects:* the planner over a horizon: with five
  rungs and two people it adds weights to tune for little (abr-controller.md).

### Pensieve

- **Mao, Netravali, Alizadeh, "Neural Adaptive Video Streaming with Pensieve", ACM SIGCOMM
  2017**, doi:10.1145/3098822.3098843,
  [PDF](https://people.csail.mit.edu/hongzi/content/publications/Pensieve-Sigcomm17.pdf). **[1P]**
  Only for what its learned policies chose: most of their gain was less rebuffering, by building
  buffer first; on a link averaging 4.5 Mb/s RobustMPC's discounted estimate sat near 2 and never
  climbed, while the learned policy climbed once its buffer held a cushion, skipping rungs. One
  chunk of history was not enough; eight was the default. With TCP's slow start after idle on,
  smaller chunks measured less. *D352 takes:* the discount alone can be too cautious, so the climb
  also needs only the buffer and the agreement, and the discount shrinks as varying samples leave
  the five (§2, §4); skipping rungs when the evidence is strong. *Rejects:* a learned controller:
  no training data, and it failed on links unlike its training (BOLA ToN).

### The players' parameters (sources read, `/yoloai/cache/abr-research/`)

| Player | Estimate | Before it trusts it | Switching | **[1P]** |
| --- | --- | --- | --- | --- |
| hls.js 1.7.3 | min(EWMA 3 s, 9 s), weighted by seconds; TTFB less its estimate | its default 500 kb/s until a sample | up within 0.7 of the estimate, down within 0.95; no interval | `docs/API.md` |
| Shaka | min(EWMA 2 s, 5 s), weighted by time; samples under 16 KB dropped | 128 KB in total; 1 Mb/s until then | up within 0.85, down within 0.95; `switchInterval` 8 s, both ways; may jump rungs | `player_configuration.js` |
| dash.js | EWMA 3 s / 8 s by default (also harmonic, byte-weighted modes, windows of 3–4); latency kept apart | throughput rule first, BOLA from 12 s buffered | 0.9 safety; abandon at 1.8× the segment's length; SwitchHistoryRule forces a level down when drops outnumber 7.5% of its picks | `Settings.js`, rules |
| Media3 | 50th percentile, samples weighted √bytes, max weight 2000 | 2 s or 512 KiB | 0.7 of the estimate; up only with 10 s buffered; down deferred with 25 s buffered | `AdaptiveTrackSelection`, `DefaultBandwidthMeter` |

*D352 takes:* Shaka's 8 s interval and Media3's 10 s gate as the fast end of the hold (§3's 30 s
is three to four times them, our chunks being longer), dash.js's SwitchHistoryRule as precedent
for remembering swings, and Media3's 512 KiB as a floor to consider (Q6).

---

## 4. What viewers mind, and what practitioners write

- **Dobrian et al., "Understanding the Impact of Video Quality on User Engagement", ACM SIGCOMM
  2011**, doi:10.1145/2018436.2018478,
  [PDF](http://conferences.sigcomm.org/sigcomm/2011/papers/sigcomm/p362.pdf). **[1P]** Buffering
  "has the largest impact" on engagement across all content; switching was not measured.
- **Balachandran et al., "Developing a Predictive Model of Quality of Experience for Internet
  Video", ACM SIGCOMM 2013**, doi:10.1145/2486001.2486025. **[1P]** Engagement falls with buffering
  events up to 0.3 a minute, then viewers leave; "bitrate switching can reduce buffering".
- **Krishnan, Sitaraman, "Video Stream Quality Impacts Viewer Behavior", ACM IMC 2012.** **[1P]**
  Rebuffering for 1% of a video's length cuts 5% of the viewing.
- **Seufert et al., "A Survey on Quality of Experience of HTTP Adaptive Streaming", IEEE
  Communications Surveys & Tutorials 17(1), 2015**, doi:10.1109/COMST.2014.2360940. **[1P]**
  Stalls are to be avoided at the cost of start delay or quality; switches more often than once a
  second are worse than a constant low quality, and at 2 s or more apart matter little; the
  amplitude of a change matters most, a drop more than a rise, and the time spent on each quality
  more than the number of switches.
- **Netflix:** Watson, "HTTP Adaptive Streaming in Practice", MMSys 2011 keynote,
  [PDF](http://web.cs.wpi.edu/~claypool/mmsys-2011/Keynote02.pdf) **[1P]** (the last 10 s
  predict the next); Ekanadham, "Using Machine Learning to Improve Streaming Quality at Netflix",
  Netflix TechBlog 2018 **[1P]** (stability and predictability matter, not the mean; history picks
  the first quality); Spang et al., "Sammy: Smoothing Video Traffic to be a Friendly Internet
  Neighbor", ACM SIGCOMM 2023, [PDF](https://brucespang.com/papers/sammy.pdf) **[1P]**: production
  ABR is "MPC-style" with an initial phase and a playing phase, its first quality chosen from the
  device's earlier sessions' initial throughput, and "ABR algorithms … do not need accurate
  available bandwidth estimates", only whether a rung fits; taking the minimum of recent samples
  and choosing below half of it spirals down where throughput is capped near the bitrate.
- **Amazon Prime Video:** "SODA", ACM SIGCOMM 2024, doi:10.1145/3651890.3672260. **[1P]** Switching
  above 20% of decisions cut sports viewing; a squared switch penalty and a target buffer cut
  switches up to 89% and rebuffering 53% in production.
- **YouTube:** no client controller published; its Trickle (USENIX ATC 2012) paces the server
  after the first 30–40 s, so a client there measures the pace, as ours does under voice
  (D265 §7). **[1P]**
- **Apple, "HLS Authoring Specification for Apple Devices"** (2025-06-26). **[1P]** A ladder and a
  first variant (2 Mb/s on Wi-Fi), measured rates within 10% of the declared; **no guidance on how
  a client switches.**

*D352 takes:* rebuffering is the cost every study agrees on; switching costs little at our
intervals, so the controller may climb quickly where it is confident, and the cost it does
carry, a large swing back down, is what §3 guards against.

---

## 5. What D352 rejects, and why

- **BOLA, (Robust)MPC or a learned controller as the controller** (§3): each a larger design for a
  five-rung ladder and two people, and each with a known failure (BOLA's slow start, MPC's planner
  on a badly predicted link, Pensieve outside its training). D352 borrows their parts instead: the
  harmonic mean, the error discount, the buffer gate.
- **A buffer-only controller** (BBA): our buffers are a tenth of Netflix's, and its start-up loss is
  what the owner reported.
- **The median or a percentile** (Media3, IMC 2012's 80th): sound against outliers both ways, but
  it ignores a single slow chunk that does matter, a link that fell; the harmonic mean of five reacts
  to it at once and forgets it in five chunks.
- **A speed test or probe** (hls.js's `testBandwidth`, D247 §1): the owner ruled it out, and the
  film page's own preload measures.
- **Remembering the quality that failed** (a per-rung penalty like dash.js's SwitchHistoryRule):
  D352 remembers swings of the link, not of a rung, which is simpler and covers the same case on a
  short ladder.
- **Tuning hls.js's half-lives** (§1.4): no half-life undoes its weighting by seconds.
- **Randomised request times** (FESTIVE): no evidence yet of two of our players swinging together.

## 6. Where the sources disagree, and which way D352 goes

- **Throughput against buffer.** BBA and IMC 2012 say steady state needs no estimate; MPC and
  Sammy keep one, and practice (dash.js, Netflix) is hybrid. D352 stays a throughput rule with
  buffer gates (D265), as the shortest buffers here need, and takes BBA's point that the estimate
  is least trustworthy where it matters most, at start-up (§1).
- **Using the estimate at start-up.** BBA-2, BOLA-PL and Netflix use it, with a high bar or
  history; FESTIVE, Shaka and Media3 refuse to switch on it until enough bytes or samples. D352
  goes both ways at once: each chunk follows the estimate as it stands, but nothing is latched until
  four samples and 20 s (§1), and the remembered speed is the confident estimate.
- **Mean, harmonic mean or percentile.** FESTIVE and MPC take the harmonic mean; IMC 2012 and
  Media3 a percentile; hls.js, Shaka and dash.js the lower of two moving averages. The harmonic
  mean's sensitivity to a slow sample is what made hls.js's estimate hold (§1.4), but there it is
  weighted by seconds and unbounded in memory; over five samples counted once, the sensitivity
  lasts five chunks. D352 takes the harmonic mean of five.
- **How much switching costs.** Yin's default λ is light, BBA calls switching "debatable", Seufert
  finds frequency matters little at 2 s or more; FESTIVE's cost is exponential and Prime Video's
  data says sports viewers leave. D352 goes light on switching as such, quick climbs where the
  samples agree, and heavy on its one harmful form, a climb that falls back (§3).
- **Up against down.** Nearly every player falls fast and climbs slowly; Media3 even defers a fall
  with 25 s buffered, and Akhshabi calls 40–80 s recoveries from short dips a fault. D352 keeps the
  fast fall and makes the climb's speed depend on the evidence and the link's history (§2–§4).
