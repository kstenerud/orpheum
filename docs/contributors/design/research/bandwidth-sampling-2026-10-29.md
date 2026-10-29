> **ABOUTME:** How to estimate a link's speed and reliability from the few chunks a playback has at
> its start: what statistics (not Shannon) says about few noisy samples, what Netflix, Google,
> Stanford's Puffer, CMU's CS2P and the open players do, why the three staging failures of
> 2026-10-28 happened, and a proposed estimator for the chunked player: in log space, robust, with
> the remembered link as a prior, a given-up chunk treated as censored, and decisions taken at a
> quantile set by what is loaded ahead. 2026-10-29.

# Sampling a link: early decisions from few samples

- **Date:** 2026-10-29. Asked by the owner: *"research what's the best way to collect samples to make
  decisions about bandwidth and network reliability … What's the best way to make an early decision
  with few samples?"*
- **Builds on:** [abr-adaptation-2026-10-27.md](abr-adaptation-2026-10-27.md) (the evidence behind
  [D352](../../decisions/working-notes.md#d352--the-chunked-player-trusts-its-link-as-far-as-the-evidence-goes-its-first-seconds-latch-nothing-it-climbs-as-far-as-consistent-samples-carry-and-it-grows-steadier-when-it-swings-or-its-link-flakes)),
  [abr-controller.md](abr-controller.md) (D265), [slow-links.md](slow-links.md),
  [webrtc-throughput-2026-10-28.md](webrtc-throughput-2026-10-28.md) and its
  [review](webrtc-throughput-review-2026-10-28.md). Those cover the controller, hls.js's estimator,
  TCP and SCTP windows, BBA, BOLA, MPC, Pensieve and the players' parameters. This document does not
  repeat them; it adds the statistics, the sources on prediction and priors, and a proposal.
- **Code read:** `web/packages/player/src/link-estimate.ts`, `abr.ts` (`#giveUp`, `measures`,
  `linkKbps`), `abr-choice.ts` (`giveUpFor`, `measuresTheLink`, `arrivalMs`), `next-chunk.ts`
  (`nextChunkLinkOf`), `web/packages/sdk/src/ready-ahead.ts`, at `main` an earlier commit.
- **Cache:** `/yoloai/cache/bandwidth-sampling/` (Puffer and BOCPD as HTML from ar5iv, CS2P's PDF and
  its text, the IETF drafts of BBR and of Google's WebRTC congestion control, Semantic Scholar
  records), and `/yoloai/cache/abr-research/` (the earlier papers and the players' sources).
- **Labels:** **[1P]** read at the source this session or in the cached source; **[A]** the
  abstract or record only; **[M]** from memory of a well-known source, not re-read here; **[I]**
  inference or my own arithmetic; **[G]** a guess, to be measured.
- **Not reached:** the full text of Oboe (ACM's site refused the download; its abstract was read),
  PANDA's full text (only its abstract), any Netflix publication on its initial-bitrate model beyond
  Sammy's paragraph (§3.1), and the staging log of the 753 kb/s start (§1.2).

---

## Summary: how to decide early with few samples

**The question has a textbook answer, and it is statistics, not information theory.** Shannon's
capacity is a property of a channel's physics; it says nothing about estimating what a path through
other people's traffic, a congestion controller and a relay will carry next. The right ancestors are
Laplace and Bayes (start from a prior and update it), Fisher (how fast error shrinks with samples),
Wald (decide as soon as the evidence is enough for *this* decision), Tukey and Huber (do not let one
bad sample rule), Page (notice when the link has changed), Kalman (blend old and new by their
uncertainty) and Kaplan and Meier (an unfinished measurement is a bound, not a value).

Put together, five rules:

1. **Start from a prior, not from nothing.** The best single predictor of a session's first
   throughput is what similar sessions saw: CS2P cut the initial prediction error from 35%+ to about
   20% that way, from 20 million sessions (§3.6); Netflix picks its first quality from the device's
   earlier sessions (§3.1); Media3 starts from a table by country and network type (§3.3). We
   already have the strongest prior there is: the same device, on the same network, minutes ago (the
   remembered link, D257). **It should count as a couple of samples, fading with its age**, and be
   dropped only when two of the new samples agree with each other and not with it.
2. **Average in log space, robustly.** Throughput errors are multiplicative: a chunk is "twice as
   fast", not "5 Mb/s faster". Take logs, use a robust centre (a median, a Huber mean, or a
   harmonic mean of winsorised samples) and a robust spread (MAD). **The harmonic mean is the right
   average for predicting a download time when samples are clean, and the worst one when they are
   not**: it can never exceed n times the smallest sample, so one sample near zero takes it to near
   zero. That is failure 1 exactly (§1.1).
3. **How many samples you need depends on the decision, not on a fixed count.** The error of an
   average shrinks as 1/√n, so the number of samples a decision needs grows as (noise ÷ margin)².
   With our links' noise (about ±38% per chunk on the Fire TV, §2.3), a rung that needs a fifth of
   the link is decided by **one** sample; a rung that needs 87% of it is not decided by fifty. Wald's
   sequential test says to decide the easy rungs at once and let the buffer, not more samples, carry
   the close call.
4. **Choose at a quantile set by the cost of being wrong.** A too-high pick risks a stall; a too-low
   one costs picture. Decision theory's answer to an asymmetric cost is a quantile, not a mean: if a
   stall costs four times a softer picture, aim at the 20th percentile of the predicted speed. The
   cost of a too-high pick falls as the buffer fills, so the quantile rises with it, towards the
   median. This is BBA's and BOLA's insight in one line, and it is what D265's share rising from 0.7
   to 0.95 with the buffer already approximates.
5. **Measure the right thing, and treat the unfinished as unfinished.** A chunk's sample is its bytes
   over the time from its first byte to its last (already built); the wait before the first byte is
   a separate estimate; a chunk given up with nothing arriving is **not** a speed sample of "almost
   nothing over seconds" but a censored one ("slower than this, perhaps; or the transport stopped"),
   which belongs to a reliability count, not to the speed. hls.js already does this: an abandoned
   fragment with no bytes samples only its wait (§3.4).

**For Orpheum** (§6, proposals for the owner): keep the five-sample window, but compute its centre
and spread in log space with a 2σ clip; blend in the remembered link as two pseudo-samples; never
count a given-up chunk with no progress as a speed; let the start gate and the ceiling read a lower
quantile of that estimate whose depth is set by what is loaded ahead; and log each chunk's predicted
time beside its actual one, so the estimator's calibration can be judged from staging's logs. Worked
through on the three failures, this keeps Nosferatu's estimate near 14 Mb/s where it read 69 kb/s,
reads one slow sample of five as 11.5 Mb/s where the old divisor read 1.3, and lets a remembered
17 Mb/s survive one bad early sample while two consistent slow ones still override it (§1, §6.7).

---

## 1. The three failures, explained

### 1.1 Nosferatu, 9018 → 696 kb/s on an estimate of 69 kb/s, for 75 s

The Fire TV's picture chunks had just loaded at 14–18 Mb/s. 2.3 s into play the estimate was
69 kb/s. The estimate is the harmonic mean of the last five samples (`link-estimate.ts`). The
harmonic mean of five is at most five times the smallest sample, and with four samples near
15 Mb/s, an estimate of 69 kb/s needs a fifth of **about 14 kb/s** (5 ÷ (4/15000 + 1/14) ≈ 70)
**[I]**. A chunk given up after a few seconds with a few kilobytes arrived gives exactly that: `#giveUp`
records `max(1, bytes arrived)` over the time since the chunk was asked for (`nowMs − sinceMs`),
which includes the wait for its first byte, against the rule `arrivalMs` applies to every other
sample **[1P]**. With nothing arrived it would be 1 byte, and the harmonic mean about 0.013 kb/s.

Why 75 s: while starting, a chunk given up lowers the ceiling (D352 §1), to the quality the chunk's
own arrival rate carries, which at 14 kb/s is the lowest. The near-zero sample then stays in the five
for five more chunks. When it leaves, each later sample was "predicted" at 69 kb/s and measured at
15 Mb/s; the relative error of each is near 100%, so the confident estimate is about half the mean
until those leave in turn. The step-up then waits its 30 s hold with 20 s loaded ahead. Several of
those, in series, give the order of a minute **[I]**. The lead's reading of staging (VQ116) reached
the same cause **[1P]**.

What it says about method: the **estimator** has no robustness to a low outlier (§2.4), and the
**sample** was not a speed at all (§5.3). Either fix alone would have prevented it; both are proposed.

### 1.2 A start that waited 10 s on a "confident" 753 kb/s, after the film before measured 17 Mb/s

The start gate waits for 150% of the next chunk's expected load (`ready-ahead.ts`). That load is
reckoned over the player's confident estimate "where it has one", else the remembered link
(`nextChunkLinkOf`) **[1P]**. "Has one" means two samples (`reportsLinkFromSamples = 2`), so two
samples outvote the remembered link entirely, and the confident estimate of two samples is their
harmonic mean divided by one plus the one prediction error between them. The remembered link was
also being lost at each deploy (`rememberedLinkKbps=0` after one, in the same night's notes) **[1P]**.

The log of that start (18:24:01) was not available here, so which of two causes it was is not known:

- **Two genuinely slow samples** (a link that really was slower, or a fresh WebRTC connection whose
  SCTP window was still opening, webrtc-throughput-2026-10-28.md). Then waiting was right for the
  link as it then was, and the fault is only that two samples on a fresh connection were trusted over
  a measurement minutes old.
- **One fast and one slow sample.** Then the one prediction error is huge (a 17 Mb/s prediction of a
  0.4 Mb/s sample is an error of 4100%), and the divisor makes the confident estimate a fraction of
  the slow sample. This is failure 3's mechanism at n = 2.

Either way, two samples cannot estimate a spread (§2.2), and a fresh, specific prior should not be
discarded by the first two samples unless they agree with each other against it (§6.2). The switch
notes now carry `linkSamples`, so the next case will say which it was.

### 1.3 One slow sample of five cut the ceiling from the top to the second-lowest

Five samples: 9.65, 18.2, 12.7, **2.51** and 22.6 Mb/s (376 KB in 1200 ms the slow one). Their
harmonic mean was 7.35 Mb/s; the largest prediction error was 460%, so the confident estimate was
7.35 ÷ 5.6 = 1.3 Mb/s **[1P]**. Trimming the most extreme error (built 2026-10-28) gives 4.35 Mb/s
**[I]**. In log space the slow sample is 4.7 standard deviations below the other four (their spread
is 0.38 in natural log) **[I]**: by any outlier rule it is not the same population. The relative
error RobustMPC uses is lopsided: a prediction above a small sample can be wrong by any multiple,
while one below a large sample is wrong by at most 100%, so the maximum of a few such errors is ruled
by the slowest sample (§2.4).

| Estimator, on failure 3's five samples | Mb/s | On failure 1's (four near 15, one at 14 kb/s) |
| --- | --- | --- |
| Arithmetic mean | 13.1 | 12.6 |
| Harmonic mean (today's estimate) | 7.35 | 0.07 |
| Today's confident estimate (error trimmed) | 4.35 | — |
| Geometric mean (mean of logs) | 10.5 | 3.85 |
| Median | 12.7 | 15.0 |
| Harmonic mean with the smallest dropped | 14.2 | 15.6 |
| Harmonic mean, each sample raised to at least 0.55 × the median | 11.8 | 13.3 |
| Huber mean in log space, clip at 2 MAD-σ | 11.5 | 13.8 |
| That Huber mean's 80% lower bound for the mean | 9.3 | — |

All **[I]**, computed from the logged samples. The robust rows agree with each other within 25% and
with the four good samples; the harmonic mean and the geometric mean do not.

---

## 2. Theory: what statistics says about a rate from few noisy samples

### 2.1 Shannon is not the ancestor

Shannon's channel capacity (C = B·log₂(1 + S/N), Shannon, "A Mathematical Theory of Communication",
Bell System Technical Journal 27, 1948) bounds what a channel's physics permits **[M]**. A chunk's
throughput over a home network to a server is set by cross traffic, a congestion window, a relay's
queue and a browser's socket (webrtc-throughput-2026-10-28.md §1), all far below capacity and
changing by the second. Estimating it is a problem of statistical estimation and decision under
uncertainty. Information theory enters only at one remove: Fisher information and the Cramér–Rao
bound (below) say how much a sample can tell. Honest summary: **no result of Shannon's helps decide
from five chunks.**

### 2.2 How much a few samples can say

- **Error shrinks as 1/√n** (Fisher 1922; Cramér 1946, Rao 1945: the variance of an unbiased
  estimator of a mean is at least σ²/n) **[M]**. Going from one sample to four halves the error of
  the mean; four to sixteen halves it again. The first few samples are worth the most; a sixth
  sample adds little to five.
- **The spread needs more samples than the centre.** With an unknown spread, a one-sided 90% bound
  uses Student's t instead of 1.28: 3.08 with two samples, 1.89 with three, 1.64 with four, 1.53
  with five (Student, Biometrika 1908) **[M]**. **Two samples say almost nothing about how variable a
  link is.** This is why RobustMPC's divisor, the largest of one or two errors, swings wildly at a
  start (failures 2 and 3), and why a spread should come from a prior or a floor until there are
  three or more samples.
- **The mean against the next sample.** The error of predicting the *next* chunk is the spread of a
  chunk plus the error of the mean: σ·√(1 + 1/n). It never shrinks below σ, because one chunk's luck
  is not averaged away. With σ = 0.38 in log (the Fire TV's clean samples), a 90% lower bound is
  this fraction of the centre **[I]**:

  | Samples n | for the mean over many chunks | for the next chunk alone |
  | --- | --- | --- |
  | 1 | 0.61 | 0.50 |
  | 2 | 0.71 | 0.55 |
  | 3 | 0.76 | 0.57 |
  | 4 | 0.78 | 0.58 |
  | 5 | 0.80 | 0.59 |

  The start gate predicts one chunk, so it needs the right column; a ceiling that will be held for
  many chunks, with a buffer to absorb one slow one, needs the left.

### 2.3 Work in logs

Throughput samples are positive, skewed and vary by ratios; a relative error is what every source
reports (RobustMPC's percentage error, CS2P's normalised error, the players' safety factors as
fractions) **[1P]**. Their logarithm is closer to symmetric, so means, medians and bounds behave.
That throughput is close to log-normal within a session is my assumption from these conventions and
our own samples, not a cited result **[I]**. The Fire TV's four clean samples have a spread of 0.38
in natural log, about ±46% for one standard deviation either way; CS2P found half of its sessions
with a normalised standard deviation of 30% or more and a fifth with 50% or more (§3.6) **[1P]**.
Those set the noise σ the rest of this document assumes: **0.3–0.4 on a steady home link [G]**.

### 2.4 Robust statistics: which average survives an outlier

- **Contaminated data is the normal case.** Tukey ("A survey of sampling from contaminated
  distributions", in *Contributions to Probability and Statistics*, Stanford University Press, 1960)
  showed that a few percent of samples from a wider distribution ruin the mean and the standard
  deviation, while a trimmed mean or a median barely moves **[M]**. Our contaminants are known: a
  chunk that waited on a crowded or fresh WebRTC connection, a transport that stuck, a cache hit
  (fast outlier), a small chunk ruled by its window's ramp.
- **The breakdown point** (Hampel, "A general qualitative definition of robustness", Annals of
  Mathematical Statistics 42(6), 1971, doi:10.1214/aoms/1177693054) is the share of samples that can
  drive an estimate anywhere **[M]**. The mean's is 0 (one huge sample, a cache hit, drives it up
  without bound). **The harmonic mean's is also 0, downward**: HM ≤ n·min. The geometric mean's is 0
  both ways, though it moves less per outlier. The median's is 50%. A mean trimmed of one sample in
  five has 20%.
- **Huber's M-estimator** (Huber, "Robust Estimation of a Location Parameter", Annals of
  Mathematical Statistics 35(1), 1964, doi:10.1214/aoms/1177703732) averages normally inside k
  standard deviations of the centre and clips beyond, with k ≈ 1.5–2; nearly as efficient as the mean
  on clean data, bounded under contamination **[M]**. Google's WebRTC congestion control does exactly
  this in production: its Kalman filter's noise estimate is updated with 3·σ̂ in place of any
  residual larger than that, "an additional outlier filter" (Holmer et al., draft-ietf-rmcat-gcc-02,
  IETF 2016, §5.3) **[1P]**.

**Why the harmonic mean, and why not alone.** For a chunk of B bits and a random speed r, the time is
B/r, and its expected value is B·E[1/r]. The harmonic mean is 1/mean(1/r): the plug-in estimate of
exactly that. So FESTIVE and RobustMPC were right that it is the average a download time follows
(abr-adaptation §3) **[I]**. Its weakness is that the average of 1/r is ruled by the smallest r. The
cure is not another average but robustness first: clip or winsorise each sample, then take whichever
mean. The table in §1.3 shows the effect.

### 2.5 Priors, shrinkage and the Kalman filter

- **A prior is a number of pseudo-samples.** Laplace's rule of succession (1814) and conjugate
  Bayesian updating say the same thing: a normal prior on the log-speed with variance σ²/n₀ is worth
  n₀ samples, and the posterior mean is (n₀·prior + Σ samples) ÷ (n₀ + n) **[M]**. After four own
  samples a two-sample prior is a third of the estimate; after ten, a sixth.
- **Shrinkage beats the raw mean when samples are few** (Efron and Morris, "Data analysis using
  Stein's estimator and its generalizations", JASA 70, 1975, doi:10.1080/01621459.1975.10479864):
  pulling a noisy estimate towards a well-chosen centre lowers its expected error **[M]**. The
  remembered link is that centre.
- **The Kalman filter** (Kalman, "A New Approach to Linear Filtering and Prediction Problems",
  Journal of Basic Engineering 82(1), 1960, doi:10.1115/1.3662552) is the same blend done over time:
  each sample moves the estimate by a gain P/(P + R), P the estimate's uncertainty and R the sample's
  **[M]**. A stale prior has a large P, so the first sample moves it a long way; a fresh one has a
  small P, so one sample moves it little. With a random-walk model of the link, the gain settles to a
  constant and the filter becomes an EWMA. Google's WebRTC congestion control uses a scalar Kalman
  filter, with a state noise of 10⁻³, on its delay signal (draft-ietf-rmcat-gcc-02 §5.3) **[1P]**.
  For us, the filter's lesson is a rule: **weigh the remembered link by how old it is.**
- **When the prior is wrong.** A prior can mislead when the device has moved room or network. The
  standard check is prior–data conflict: if the samples are far from the prior, by more than their
  combined uncertainty, drop or widen the prior **[M]**. §6.2 turns this into "two own samples that
  agree with each other and not with the prior outvote it".

### 2.6 Sequential analysis: decide as soon as the evidence is enough

Wald's sequential probability ratio test (Wald, "Sequential Tests of Statistical Hypotheses",
Annals of Mathematical Statistics 16(2), 1945, doi:10.1214/aoms/1177731118) takes samples one at a
time and stops as soon as the evidence crosses a threshold set by the error rates allowed. For the
same error rates it needs on average far fewer samples than a fixed-size test **[M]**. The number it
needs grows as (σ/δ)², δ the gap between the hypotheses. For a rung, δ is the log-gap between the
estimate and the speed the rung needs **[I]**:

| Rung's need as a share of a link measured at 15 Mb/s | δ (natural log) | δ/σ at σ = 0.38 | Samples a 90%-sure decision needs |
| --- | --- | --- | --- |
| 3 Mb/s (a fifth) | 1.61 | 4.2 | 1 |
| 7.5 Mb/s (a half) | 0.69 | 1.8 | 1 |
| 10 Mb/s (two thirds) | 0.41 | 1.1 | about 2 |
| 13 Mb/s (87%) | 0.14 | 0.38 | about 11 for the mean, never for the next chunk |

**[I]**, from n ≈ (1.28·σ/δ)² for the mean. This is the most useful answer to the owner's question:
**a fixed "four samples before trusting" is too slow for easy decisions and too fast for close
ones.** The easy decision ("is the link plainly far above the rung?") can be taken on the first
clean chunk. The close one should not be taken on samples at all but on the buffer: with 20 s
loaded, a wrong choice costs one chunk's swap, not a stall.

### 2.7 Asymmetric costs give a quantile

Under a loss that is linear and asymmetric, c_over per unit of overestimating and c_under per unit of
underestimating, the best point estimate is the quantile τ = c_under ÷ (c_under + c_over) of the
predictive distribution: the newsvendor result (Arrow, Harris and Marschak, "Optimal Inventory
Policy", Econometrica 19(3), 1951) and the pinball loss of quantile regression **[M]**; Zellner
("Bayesian estimation and prediction using asymmetric loss functions", JASA 81, 1986,
doi:10.1080/01621459.1986.10478289) gives the Bayesian form **[M]**. Applied **[I]**:

- With an empty buffer, a too-high pick stalls. If a stall costs four times a step of picture,
  τ = 0.2 (z = 0.84); nine times, τ = 0.1 (z = 1.28).
- With 20 s or more loaded, a too-high pick costs a swap later, not a stall: c_over falls towards
  c_under and τ rises towards 0.5, the median.

The players' safety factors are this rule frozen at one buffer level: Media3's 0.7, hls.js's 0.7 up
and 0.95 down, dash.js's 0.9 (abr-adaptation §3) **[1P]**. D265's share rising from 0.7 at 10 s to
0.95 at 25 s is it varying with the buffer. BBA and BOLA push it to its end: with a full buffer the
estimate hardly matters (abr-adaptation §3) **[1P]**. What the theory adds is that the margin should
also widen when the *estimate* is uncertain (few samples, large spread), since a quantile of a wide
distribution sits further below its centre. A fixed safety factor applied to a two-sample estimate
does not.

### 2.8 Change-point detection: the link changed, or a sample was unlucky?

Page's CUSUM (Page, "Continuous Inspection Schemes", Biometrika 41(1/2), 1954,
doi:10.1093/biomet/41.1-2.100) and the Page–Hinkley test (Hinkley, Biometrika 58(3), 1971,
doi:10.1093/biomet/58.3.509) sum the residuals beyond an allowance and signal a change when the sum
crosses a threshold: one large residual is absorbed, several moderate ones in a row are not **[M]**.
Adams and MacKay's Bayesian online change-point detection (arXiv:0710.3742, 2007) keeps a
distribution over "how long since the last change" and updates it with each sample **[1P]**. Oboe
(Akhtar et al., ACM SIGCOMM 2018) pre-computes the best ABR parameters per network state and switches
parameters at run time "for the current network conditions" **[A]**; that it detects the state with
Bayesian change-point detection is from memory **[M]**. Against that, Puffer "ha[s] not observed CS2P
and Oboe's observation of discrete throughput states" in its own sessions **[1P]**. So: a change-point
rule is a sound way to tell "the link fell" from "one chunk was slow", but the evidence that links
move between a few discrete states is mixed. D352's harmonic-mean-of-five already follows a real
fall within a chunk or two; a CUSUM-like "two consecutive samples beyond 2σ on the same side" is
proposed for the narrower job of deciding when the prior is wrong (§6.2).

### 2.9 Censored observations: an unfinished chunk is a bound

When a measurement stops before its event, it is censored: it says the value exceeded what was seen,
not what the value was. Kaplan and Meier ("Nonparametric Estimation from Incomplete Observations",
JASA 53(282), 1958, doi:10.1080/01621459.1958.10501452) showed how to estimate a distribution from
such data without treating the censored ones as events **[M]**. Treating a censored time as an
observed one biases the estimate; treating "the chunk had not arrived after 3 s" as "the link
carries 1 byte in 3 s" is that bias in its extreme form **[I]**. What a given-up chunk honestly says:

- **Bytes arrived for a while, then it was given up:** a speed over first byte to give-up, for the
  bytes that came. A fair sample, of a partial transfer; small if few bytes.
- **Nothing arrived, or arrival stopped:** the time to finish exceeded what was waited. That is
  evidence about *reliability* (the transport stalls) and about the first-byte wait, not about speed.
  The bytes that would have come were not slowed; they were not sent.

### 2.10 Heavy tails, and the bandit framing

- **Heavy tails.** Puffer: "the variability and heavy-tailed nature of network and algorithm behavior
  create hurdles"; with 1.75 years of video per scheme the 95% interval on a scheme's stall ratio was
  still wide, and "it takes about 2 stream-years of data" to tell schemes apart **[1P]**. Two lessons:
  rare events (a stuck transport) dominate stalls, so they need their own signal (§5.4); and no
  tuning of ours will be confirmed statistically from a household's viewing, so constants should be
  checked by calibration (§6.6), not by A/B testing.
- **Bandits** (Thompson, Biometrika 25, 1933; Robbins, Bulletin of the AMS 58, 1952) **[M]**: the
  explore–exploit trade-off matters when only the option tried reveals its value. Here any chunk
  measures the link, at any rung, so exploration is nearly free; only the size effect (larger chunks
  measure higher, §5.2) and competition for a shared link (PANDA, §3.7) make the rung matter. **Low
  relevance** for one player on a home link; relevant again if two players share an upload.

---

## 3. Practice, by organisation

### 3.1 Netflix

- **BBA** (Huang, Johari, McKeown, Trunnell, Watson, ACM SIGCOMM 2014): capacity estimation "is
  unnecessary in steady state … [but] important during the startup phase"; BBA-2 climbs from an empty
  buffer only when the last chunk arrived about 8× faster than it plays, relaxing to 2× as the buffer
  fills (abr-adaptation §3) **[1P]**. In this document's terms: a high quantile bar when the buffer is
  empty, falling as it fills.
- **Sammy** (Spang et al., ACM SIGCOMM 2023, doi:10.1145/3603269.3604839): production ABR has an
  initial phase and a playing phase; "The challenge in the initial phase is making bitrate selections
  with relatively few throughput measurements. ABR algorithms typically deal with this challenge by
  using historical throughput from the playing phase of previous sessions"; Sammy keeps a separate
  initial-phase history per device; its QoE gains came from the initial phase (initial VMAF +0.30%,
  play delay −0.40%); a treatment group starting without history had lower initial quality for over
  a week; "Using historical data improves performance, but the dependency creates challenges for
  evaluation." **[1P]** (cached notes). **Strong evidence, from production, that the per-device
  history is the start's main input.**
- **Ekanadham, "Using Machine Learning to Improve Streaming Quality at Netflix"**, Netflix TechBlog,
  2018-03-22,
  [link](https://netflixtechblog.com/using-machine-learning-to-improve-streaming-quality-at-netflix-9651263ef09f):
  "other characteristics such as stability and predictability make a big difference"; "How can we
  incorporate longer-term historical information about the network and device? … can we at least
  characterize the distribution of throughput that we expect to see given historical data?" **[1P]**.
  Questions, not results; it confirms the direction (history, distributions, not points).

### 3.2 Google: BBR, WebRTC's congestion control, Shaka

- **BBR** (Cardwell, Swett, Beshay et al., draft-ietf-ccwg-bbr-06, IETF, July 2026,
  [link](https://datatracker.ietf.org/doc/draft-ietf-ccwg-bbr/)): `max_bw` is "the windowed maximum
  recent bandwidth sample", over two probing cycles; `min_rtt` a windowed minimum over 10 s; samples
  taken while the sender was application-limited are marked, because a lower rate "simply because the
  sender is application-limited" is not the path's, and BBR "will not discard old max bandwidth
  samples that were not app-limited" in such phases. The draft names adaptive streaming video as an
  example of chunked, application-limited sending. **[1P]** The lesson: **delivery-rate samples are
  lower bounds on the bottleneck**, nearly all their noise is downward, so a capacity estimate takes a
  maximum. For an ABR the target is not capacity but what the next chunk will get, so a maximum is too
  bold; but it says the high samples are informative and the low ones often not **[I]**.
- **Google Congestion Control for WebRTC** (Holmer, Lundin, Carlucci, De Cicco, Mascolo,
  draft-ietf-rmcat-gcc-02, IETF 2016, [link](https://datatracker.ietf.org/doc/draft-ietf-rmcat-gcc/)):
  a scalar Kalman filter with a 3σ outlier clip on its noise estimate (§2.4, §2.5) **[1P]**.
- **Shaka Player**: the lower of two EWMAs (half-lives 2 s and 5 s), weighted by time; samples under
  16 KB dropped ("ignore very small downloads which would cause our estimate to be too low"); the
  default 1 Mb/s kept until 128 KB have been sampled; no switch until then (abr-controller.md,
  cached `shaka-player_configuration.js`) **[1P]**.
- **YouTube**: no published description of its client's ABR was found, here or in earlier research
  (abr-adaptation §4) **[1P, absence]**. CS2P reports YouTube "anecdotally" uses previous sessions of
  the same client **[1P]**.

### 3.3 Android: ExoPlayer / Media3

`DefaultBandwidthMeter` (`/yoloai/cache/abr-research/exo-DefaultBandwidthMeter.java`) **[1P]**:

- Starts from a table by **country group and network type**: Wi-Fi 4.3, 3.2, 2.4, 1.7 or 0.86 Mb/s
  by country group; 4G 2.5 to 0.68; 1 Mb/s when unknown. It resets when the network type changes
  (`resetOnNetworkTypeChange`).
- Each transfer's speed enters a sliding percentile weighted by **√bytes**, maximum weight 2000; the
  estimate is its **50th percentile**, and replaces the initial one only after **2 s or 512 KiB** in
  total.
- Media3's experimental meter keeps the **time to first byte apart**, as the median of the last 10
  (`PercentileTimeToFirstByteEstimator`, `/yoloai/cache/git/media3`, 2026-09-08) **[1P]**.

So Google's own Android player uses a weighted **median**, not a mean, and a **prior by network**.
The √bytes weight is a middle way between counting each chunk once (D352) and by bytes (hls.js).

### 3.4 hls.js and dash.js

- **hls.js 1.7.3** (cached `hlsjs-abr-controller.ts`) **[1P]**: on abandoning a fragment or a load
  timeout, *"If there has been loading progress, sample bandwidth using loading time offset by minimum
  TTFB time … If there has been no loading progress, sample TTFB"*. **A fragment with no bytes is not
  a bandwidth sample.** (Its abandon rule then resets the estimate to the lower level's nominal
  bitrate, which throws history away: rejected in abr-controller.md.)
- **dash.js** (cached `dashjs-ThroughputModel.js`, `dashjs-AbandonRequestsRule.js`) **[1P]**: latency
  (request to response) is stored apart from throughput, each with its own EWMA; cached responses
  under a threshold are dropped; the abandon rule computes the rate from the progress traces "ignor[ing]
  the first entries as they contain the latency", acts only after 6 traces and 500 ms since the first
  byte, abandons when the estimated time exceeds 1.8× the segment's duration, never at the lowest
  quality, and never when the buffer exceeds its stable time. Throughput modes include a byte-weighted
  harmonic mean and windows of 3–4 samples.

### 3.5 Stanford: Puffer and Fugu

Yan, Ayers, Zhu, Fouladi, Hong, Zhang, Levis, Winstein, "Learning in situ: a randomized experiment
in video streaming", USENIX NSDI 2020,
[paper](https://www.usenix.org/system/files/nsdi20-paper-yan.pdf),
[arXiv 1906.01113](https://arxiv.org/abs/1906.01113) **[1P]**, the strongest evidence in this
document (a blinded randomised trial, 8.5 stream-years in its main comparison):

- Fugu's **Transmission Time Predictor** predicts "the transmission time of a proposed chunk with a
  given filesize", not a throughput: "That observed throughput varies with filesize is a well-known
  effect … Fugu is the first to use this fact operationally."
- Its inputs: the sizes and transmission times of past chunks, the **size of the chunk to send**, and
  the sender's `tcp_info`: congestion window, packets in flight, smoothed RTT, minimum RTT, delivery
  rate. Its output: **a probability distribution** over 21 bins of transmission time (0.5 s each).
- **Ablations:** a predictor "that predicts throughput without regard to chunk size" and a
  "non-probabilistic" one "both performed markedly worse"; RTT, CWND and packets in flight "were also
  helpful"; a point-estimate Fugu deployed for 39 stream-days had **3–9× the rebuffering** with no
  gain in SSIM.
- MPC and RobustMPC on Puffer used "the harmonic mean of the last five throughput samples"; BBA was
  hard to beat. "Slow" paths (delivery rate under 6 Mb/s) were 16% of viewing and 82% of stalls.
- "Client-side ABR systems cannot typically access this structure [tcp_info] … these results should
  motivate the communication of richer data to ABR algorithms wherever they live."

What transfers to us without a neural network **[I]**: predict a chunk's *time* from its *size*
(§5.2); carry a spread, not a point; and let the core, which has the SCTP association's window and
round trip (pion's `Association.CWND()`, `SRTT()`, cached `pion-sctp-association.go`) **[1P]**, tell
the player. What does not: training needs data at a scale we will not have.

### 3.6 CMU and iQIYI: CS2P, priors from similar sessions

Sun, Yin, Jiang, Sekar, Lin, Wang, Liu, Sinopoli, "CS2P: Improving Video Bitrate Selection and
Adaptation with Data-Driven Throughput Prediction", ACM SIGCOMM 2016,
doi:10.1145/2934872.2934898 **[1P]** (`/yoloai/cache/bandwidth-sampling/cs2p-sigcomm2016.pdf`):

- 20M+ sessions. "Sessions sharing similar key features (e.g., ISP, region) present similar initial
  throughput values"; within a session, throughput is "stateful", modelled by a hidden Markov model
  per cluster.
- **The first epoch's prediction is the median of the matching cluster's sessions**; CS2P reached
  "≈20% median error vs. 35%+ for other predictors" for the initial epoch, and about 7% median error
  midstream, against 18% (median) and 40% (75th percentile) for last-sample, harmonic-mean and
  autoregressive predictors.
- The naive priors did badly: predicting from "clients sharing the same IP prefix /16" or the same
  server left "half of the sessions [with] normalized prediction error ≥ 50%".
- "When the error is ≤ 20%, the n-QoE of MPC is close to optimal (>85%)."

**What it means for the remembered link [I]:** CS2P's best clusters are broad (ISP, region, time).
Ours is the narrowest possible cluster, one device on one network, so it should predict better than
CS2P's 20%, though it is one past session, not a median of many. It is also exactly the "/16 prefix"
kind of key that failed when shared between clients; it works here because the key includes the
device.

### 3.7 Others

- **FESTIVE** (Jiang, Sekar, Zhang, ACM CoNEXT 2012): harmonic mean of 20; no switch before enough
  samples (abr-adaptation §3) **[1P]**.
- **PANDA** (Li, Zhu, Gahm, Pan, Hu, Begen, Akhshabi, IEEE JSAC 32(4), 2014,
  doi:10.1109/JSAC.2014.140405): "when multiple HAS clients compete at a network bottleneck, the
  discrete nature of the video bitrates results in difficulty for a client to correctly perceive its
  fair-share bandwidth"; it probes by small increments of its own request rate **[A]**. Relevant when
  two of our players share one upload.
- **RobustMPC** (Yin et al., ACM SIGCOMM 2015): harmonic mean of 5 over (1 + the largest recent
  percentage error) (abr-adaptation §3) **[1P]**. The divisor is the piece §1.3 finds fragile.
- **Oboe** (Akhtar, Nam, Govindan, Rao, Chen, Katz-Bassett, Zhang, ACM SIGCOMM 2018,
  doi:10.1145/3230543.3230558): ABR parameters tuned per network state, switched at run time; +24%
  QoE over Pensieve **[A]**.
- **Apple**: the HLS authoring specification sets the first variant and declared rates; how AVPlayer
  estimates bandwidth is not documented (abr-adaptation §4, slow-links §2) **[1P, absence]**.
- **Dean and Barroso, "The Tail at Scale"**, Communications of the ACM 56(2), 2013,
  doi:10.1145/2408776.2408794: a request that is slow because of a transient stall is best answered
  by sending a **hedged** copy elsewhere after a short delay, not by waiting or degrading **[M]**.
  Relevant to a chunk stuck on one WebRTC connection (§6.3).

### 3.8 Where the sources agree, and where they do not

- **Agree:** a prior from history for the start (Netflix, CS2P, Media3, dash.js's saved bitrate);
  first-byte wait kept apart (dash.js, Media3, hls.js, us); tiny and cached transfers excluded (Shaka,
  dash.js, Media3, us); a lower margin when the buffer is low (all).
- **Disagree:** the average. Mean of EWMAs weighted by time (hls.js, Shaka, dash.js by default),
  weighted median (Media3), harmonic mean (FESTIVE, MPC, us), maximum (BBR, for a different target),
  learned distribution (Fugu). Only Fugu was tested against the others in a large randomised trial,
  and it beat the harmonic mean, chiefly by using size and spread.
- **Disagree:** discrete network states (CS2P, Oboe) against none observed (Puffer).

---

## 4. Early decisions with few samples

Drawing §2 and §3 together:

1. **The first decision is the prior's.** Before any chunk, the start uses the remembered link
   (D257), which CS2P and Sammy's evidence say is the best input there is. It must survive a deploy:
   today it is held in the core's memory and lost with it (the lead's overnight notes, 2026-10-28) **[1P]**.
2. **Each sample moves the estimate by its weight against the prior's.** With the prior worth n₀
   samples, the first chunk moves the estimate 1/(n₀ + 1) of the way, the second 1/(n₀ + 2) **[M]**.
   n₀ shrinks with the prior's age: Kalman's lesson (§2.5).
3. **The prior is dropped when the data disagree with it consistently**, not when one sample does
   (§2.8). One slow sample against a fresh prior is far likelier to be a fresh connection or a stuck
   request than a link that changed in the minutes since.
4. **Easy decisions are taken at once; close ones are left to the buffer** (§2.6). "Not the top
   rung" after a first chunk at 1 Mb/s against a remembered 17 Mb/s is not easy: the sample and the
   prior disagree. "The top rung is fine" after a first chunk at 15 Mb/s and a prior of 17 is easy.
5. **The margin is a quantile whose depth follows the buffer and the uncertainty** (§2.7): deep (10th
   percentile of the next chunk's speed) with nothing loaded, shallow (near the median of the mean)
   with 20 s loaded.
6. **A spread needs a prior too.** Until three samples, use a remembered or assumed spread (0.3–0.4
   in log, [G]), never the spread of one or two (§2.2).

---

## 5. Measuring well

### 5.1 What a sample is

- **A chunk's bytes over its first byte to its last** (`arrivalMs`, built 2026-10-27) **[1P]**. This
  is what dash.js and Media3 do, and what hls.js's own loader reports.
- **The wait for the first byte is a separate estimate**, the median of the last few (Media3: the
  median of 10) **[1P]**, used to add to a predicted chunk's time. `next-chunk.ts` divides bits by
  speed and adds no first-byte wait today **[1P]**; on a crowded WebRTC start that wait was 1.5 s
  (abr-adaptation §1.2).
- **Excluded:** cache hits, chunks under 128 KB, paced chunks that met their pace (D265 §7) **[1P]**.
  These are right. Sound chunks are small and are not sampled; the choice is the picture's.
- **Idle and off periods are outside a chunk's arrival time by construction.** What they leave behind
  is a window that decays, so the next chunk reads low (IMC 2012's spiral; abr-adaptation §2) **[1P]**.

### 5.2 Size matters: fit time = wait + size ÷ speed

A chunk's arrival time is not size over a fixed speed: a window opening, an acknowledgement clock and
a relay's queue add a near-fixed cost per chunk. Puffer found a predictor that ignores size "markedly
worse" **[1P]**. The Fire TV's four clean chunks fit **arrival ≈ 132 ms + bytes ÷ 29.7 Mb/s** (least
squares on 192, 555, 801 and 895 KB) **[I]**, four points only: the marginal speed is twice the
smallest chunk's own (9.65 Mb/s) and close to the 25–30 Mb/s the WebRTC path's window allows
(webrtc-throughput-2026-10-28.md). So a small chunk reads the link low, and a ceiling sized from small
chunks undershoots the top rung's large ones. Two parameters need three or more chunks of differing
sizes; with fewer, a prior on the fixed cost (about two round trips, [G]) serves. This is an
improvement for later, not for the next batch (§6.8).

### 5.3 A given-up chunk is censored

As §2.9: **with progress, a sample over first byte to give-up; without progress, no speed sample**,
a drop for reliability (D352 §4), and a first-byte wait that is at least the time waited. hls.js does
the first two (§3.4). `#giveUp` today measures from the ask, not the first byte, and counts a chunk
with nothing arrived as 1 byte over the wait **[1P]**.

"Without progress" wants a definition: no byte for the last X ms while the chunk is on its way. The
stuck chunks of failure 1 delivered a few kilobytes and then nothing; a rate over the whole wait mixes
the two. A sample over the span in which bytes did arrive, plus a stall flag, separates them **[I]**.

### 5.4 Reliability is its own signal

A link's speed and its reliability are different quantities. A transport that stalls for 3 s once
in five minutes has a fine speed and a poor reliability; averaging the stall into the speed lowers the
picture for minutes and still does not prevent the next stall. What prevents a stall is **enough
loaded ahead to ride it out**, and what cures a stuck request is **asking again elsewhere** (§3.7,
Dean and Barroso) **[I]**. D352 §3–§4 already keep reliability apart (drops raise the steadiness
level, which lengthens the hold and widens the headroom) **[1P]**. The step it lacks is to keep the
drop *out* of the speed.

### 5.5 A transport's stall against the link's

On the WebRTC path a chunk can wait on its data channel, its connection's queue or the service
worker's relay without the link being slow (abr-adaptation §1.3) **[1P]**. Signals that tell them
apart, all cheap **[I]**: the connection's age when the chunk was asked (fresh connections ramp);
how many requests shared its connection; the round trip from `RTCPeerConnection.getStats()` on the
candidate pair (`currentRoundTripTime`) **[M]**; and, from the core, pion's SCTP window and smoothed
round trip at the moment the chunk's head was sent **[1P, the API exists]**. Logged beside each
sample, they would say whether a slow sample was the link.

---

## 6. Recommendation for Orpheum: proposals for the owner

Design is the owner's (AGENTS.md rule 1). These are proposals, each with what it changes in D352,
its expected effect on the three failures, and a test that would show it. Numbers marked [G] are
starting values to measure, as D352's are.

### 6.1 P1: the estimator, a robust centre and spread in log space

**Proposal.** Keep the last five chunks that measured the link (D352 §2). Take the log of each
speed. The **centre** is a Huber mean of the logs: start at their median; any sample further than
**2 spreads** from the centre counts as if it were at 2 spreads; iterate a few times. The **spread**
is 1.4826 × the median absolute deviation of the logs, **never below 0.3** [G], and, with fewer than
three samples, the prior's spread (P2). The estimate is e^centre.

A smaller step that keeps the harmonic mean: **raise each sample to at least 0.55 × the median of the
five** [G] before the harmonic mean (winsorising), and replace the confident estimate's divisor
(1 + the largest prediction error) by the lower bound of P4. §1.3's table shows both give the same
answer on our failures within 15%.

*Why:* §2.4. The harmonic mean is kept in spirit (it is the right average of clean samples) but no
single sample can rule it. *Rejects:* the median alone (wastes four samples' information at n = 5),
the time-weighted EWMA (abr-adaptation §1.4), BBR's maximum (it estimates a different thing).

### 6.2 P2: the remembered link as a prior

**Proposal.** At a start, the remembered link (D257) enters the centre as **n₀ pseudo-samples**, with
**n₀ = 2 when it is under an hour old, falling to 0 at `link_speed_fresh_hours`** [G]. The core keeps,
beside the speed and the steadiness level (D352 Q2), **the spread** the playback ended with, so the
prior carries one. The prior is **dropped** when **two of the playback's own samples lie more than 2
spreads from it on the same side and within 4 spreads of each other** [G]: they agree with each other
and not with it. The core should **persist** the remembered link across a restart (it is lost at each
deploy today); that is the store's business, not the player's.

*Why:* §2.5, §3.1, §3.6. The narrowest cluster there is, used as Laplace would. *Effect:* a fresh
17 Mb/s prior and one bad sample give about 12.6 Mb/s, not the bad sample (§6.7).

### 6.3 P3: a given-up chunk

**Proposal.**

1. **With progress** (bytes arrived in the last second [G] before the give-up, and at least
   `measuresFromBytesAtLeast` in all): a sample of the bytes arrived over **first byte to give-up**,
   as `arrivalMs` times a whole chunk.
2. **Without progress:** **no speed sample.** It is a drop (D352 §4) and a first-byte wait of at
   least the time waited, logged as `stuck`. hls.js's rule (§3.4).
3. **While starting, a chunk given up without progress lowers the ceiling only as far as the
   estimate carries** (D352 §1's "what the estimate then chooses", as a stall already does), not to
   what its own trickle carries.
4. **For the owner to consider, outside the estimator:** a chunk stuck without progress on a WebRTC
   connection is asked again on another connection, at the same quality, before any step down (a
   hedged request, Dean and Barroso). A lower quality's chunk does not travel a stuck transport any
   better. **[I]**; the transport's design is the owner's to decide, not this document's.

*Effect on failure 1:* the estimate stays near 15 Mb/s; the next chunk is chosen from it; the
ceiling is not latched at the bottom; the drop still counts toward the steadiness level.

### 6.4 P4: decisions at a quantile set by what is loaded ahead

**Proposal.** Every reading of the link for a choice uses a **lower bound**:
e^(centre − z × spread × √(1/n_eff)) for a choice held over many chunks (the ceiling, the step-up), and
e^(centre − z × spread × √(1 + 1/n_eff)) for one chunk (the start gate's next-chunk load, a give-up's
alternative), n_eff the samples plus the prior's pseudo-samples. **z falls with the buffer: 1.28 with
nothing loaded, 0.84 at 10 s, 0 at 20 s or more** [G]: the 10th percentile, the 20th, the median.
D265's share (0.7 → 0.95) stays as the headroom over a rung's average rate, as now; z replaces only
RobustMPC's divisor.

*Why:* §2.6–§2.7, BBA-2, BOLA. *Effect:* the confident estimate rises as samples agree and as the
buffer fills, smoothly, instead of jumping with the largest of a few errors.

### 6.5 P5: how many samples, and the start gate

**Proposal.**

- **The start gate** reckons the next chunk's load as the median first-byte wait plus its bits over
  P4's one-chunk bound, which with the prior exists from the first moment, so "none" (the 10 s
  refill wait) happens only with no prior and no sample. It no longer switches source from
  remembered to confident at two samples; the blend does that gradually.
- **Starting** (D352 §1) can end on **evidence, not a count**: when the ceiling chosen by P4's bound
  for the mean would be the same with one sample removed (any one) [G]. With a prior and agreeing
  samples that is two or three chunks; with disagreeing samples, more. Or keep four and 20 s, as now,
  if the owner prefers one rule fewer: P1–P4 do most of the good alone.

### 6.6 P6: what to log, to judge it

Each sample, beside today's level, bytes, arrival and first-byte wait (D352 §5) **[1P]**:

- **Its predicted arrival time**, from the estimate before it, and the bound P4 used. Over a few
  hundred chunks, the share of chunks slower than their 10th-percentile bound should be near 10%:
  **calibration**, the one test of an estimator a household's viewing can afford (§2.10).
- Whether it was **stuck** or **given up with progress**; its connection's **age** and **requests in
  flight** at its ask; its path (`direct`, relayed, HTTP); and, if the core adds them, the SCTP window
  and round trip at its head (§5.5).
- At a start: the prior, its age, n₀, and **whether and when it was dropped**.

### 6.7 Expected effect on the failures, worked

A sketch of P1 + P2 + P4 (Huber mean in log, clip 2, spread floor 0.3, prior two pseudo-samples, the
90% one-chunk bound), run on the logged numbers **[I]**; the scripts are a throwaway, the numbers
indicative:

| Case | Today | Proposed: estimate / 90% next-chunk bound | Prior |
| --- | --- | --- | --- |
| Failure 1: four near 15 Mb/s and one at 14 kb/s | 70 kb/s | 13.5 / 8.8 Mb/s (with P3, the 14 kb/s is not a sample at all: about 15 Mb/s) | none |
| Failure 1 with a 17 Mb/s prior | — | 14.6 / 9.6 Mb/s | kept |
| Failure 2(b): prior 17, samples 17 and 0.3 Mb/s | (≈0.3) | 13.9 / 9.0 Mb/s | kept |
| Failure 2(a): prior 17, two samples at 753 kb/s | 753 kb/s | 753 / 470 kb/s | dropped: two agree against it |
| Failure 3: 9.65, 18.2, 12.7, 2.51, 22.6 Mb/s | 1.3 (4.35 trimmed) | 11.5 / 5.4 Mb/s (the 80% bound for the mean, the ceiling's: 9.3) | none |
| A first sample of 1 Mb/s against a fresh 17 Mb/s prior | 1 Mb/s after two | 12.6 / 7.9 Mb/s | kept until a second agrees |
| A real fall: three samples near 1 Mb/s after a 17 Mb/s prior | — | 1.0 / 0.64 Mb/s | dropped at the second |

Failure 2(a) is unchanged, by design: if two samples really agree at 753 kb/s, the link at that
moment was slow and waiting was right. Whether that was the case needs the log (§1.2).

### 6.8 Tests that would show each failure fixed

Pure functions on a fake clock, in the style of `link-estimate.test.ts` and `abr-trust.test.ts`;
each fails with its change reverted (rule 4):

1. **Failure 1:** four samples at 14–18 Mb/s, then a chunk given up with 5 KB arrived and none for
   2 s: the estimate stays above 10 Mb/s, no speed sample is added, a drop is counted, and the next
   chunk is chosen at the top again once one more arrives at 15 Mb/s.
2. **Failure 2:** remembered 17 Mb/s, fresh; samples 17 Mb/s and 0.3 Mb/s: the next chunk's load is
   reckoned over at least 8 Mb/s. And its counterpart: two samples at 753 kb/s drop the prior, and the
   load is reckoned over about 0.5–0.75 Mb/s (a truly slow link still waits).
3. **Failure 3:** the five logged samples keep the top quality at starting's end with 10 s loaded
   (today's fix holds it only with 20 s loaded).
4. **No regression on a fall:** after a prior of 17 Mb/s, three samples near 1 Mb/s bring the estimate
   under 1.2 Mb/s by the second.
5. **Calibration, in the trace simulator abr-controller.md proposed:** log-normal samples with spread
   0.38 and 5% contaminants at a thousandth of the speed; the share of chunks slower than the 90% bound
   is 10% ± 3% [G].

Later, not now: the size-aware fit of §5.2 (time = wait + size ÷ speed over the last five), and the
core telling the player its SCTP window and round trip per chunk (§3.5). Both follow Puffer's
evidence; neither is needed to fix the three failures.

---

## 7. What the evidence does not settle

- **That our links' samples are log-normal with a spread of 0.3–0.4.** Assumed from conventions and
  five samples; P6's log would show it within a week of staging use.
- **n₀ = 2, the clip at 2, the spread floor 0.3, z by buffer.** Standard textbook values, not
  measured on our links. Each is a constant to be tuned against the calibration log.
- **Whether a stuck chunk is better re-asked than stepped down.** The reasoning is sound; no
  measurement of our transport's stalls exists yet.
- **Whether links have discrete states** worth a change-point detector (CS2P and Oboe yes, Puffer no).
- **What the 753 kb/s start's two samples were.** The proposal handles both cases; only one of them
  was a fault.

---

## Sources

Papers and standards, with strength:

- Yan et al., "Learning in situ: a randomized experiment in video streaming", USENIX NSDI 2020,
  https://www.usenix.org/system/files/nsdi20-paper-yan.pdf (read via
  https://ar5iv.labs.arxiv.org/html/1906.01113). Randomised trial, 8.5 stream-years: **strong**.
- Sun et al., "CS2P", ACM SIGCOMM 2016, doi:10.1145/2934872.2934898,
  https://www.cs.cmu.edu/~junchenj/cs2p.pdf. 20M sessions, trace-driven plus a pilot: **strong for
  initial prediction**.
- Spang et al., "Sammy", ACM SIGCOMM 2023, doi:10.1145/3603269.3604839,
  https://brucespang.com/papers/sammy.pdf. Netflix production A/B: **strong**.
- Huang et al., "A Buffer-Based Approach to Rate Adaptation", ACM SIGCOMM 2014,
  https://yuba.stanford.edu/~huangty/sigc040-huang.pdf. Netflix production: **strong**.
- Yin et al., "A Control-Theoretic Approach for Dynamic Adaptive Video Streaming over HTTP", ACM
  SIGCOMM 2015, https://conferences.sigcomm.org/sigcomm/2015/pdf/papers/p325.pdf. Testbed: **medium**.
- Jiang, Sekar, Zhang, "FESTIVE", ACM CoNEXT 2012,
  https://conferences.sigcomm.org/co-next/2012/eproceedings/conext/p97.pdf. Testbed: **medium**.
- Li et al., "Probe and Adapt (PANDA)", IEEE JSAC 32(4), 2014, doi:10.1109/JSAC.2014.140405,
  https://arxiv.org/abs/1305.0510. Abstract only here: **medium**.
- Akhtar et al., "Oboe: Auto-tuning Video ABR Algorithms to Network Conditions", ACM SIGCOMM 2018,
  doi:10.1145/3230543.3230558. Abstract only here: **medium**.
- Cardwell, Swett, Beshay et al., "BBR Congestion Control", draft-ietf-ccwg-bbr-06, IETF, 2026,
  https://datatracker.ietf.org/doc/draft-ietf-ccwg-bbr/. Deployed at Google: **strong for capacity
  estimation**, indirect for ABR.
- Holmer, Lundin, Carlucci, De Cicco, Mascolo, "A Google Congestion Control Algorithm for Real-Time
  Communication", draft-ietf-rmcat-gcc-02, IETF, 2016,
  https://datatracker.ietf.org/doc/draft-ietf-rmcat-gcc/. Deployed in Chrome: **strong**, indirect.
- Adams, MacKay, "Bayesian Online Changepoint Detection", 2007, https://arxiv.org/abs/0710.3742.
  Method: **theory**.
- Ekanadham, "Using Machine Learning to Improve Streaming Quality at Netflix", Netflix TechBlog,
  2018,
  https://netflixtechblog.com/using-machine-learning-to-improve-streaming-quality-at-netflix-9651263ef09f.
  Direction only: **weak**.
- Dean, Barroso, "The Tail at Scale", Communications of the ACM 56(2), 2013,
  doi:10.1145/2408776.2408794, https://research.google/pubs/the-tail-at-scale/. **Strong** for
  hedged requests in general; untested on our transport.
- Statistics, all **textbook-strong**, cited from memory: Wald 1945 (doi:10.1214/aoms/1177731118);
  Page 1954 (doi:10.1093/biomet/41.1-2.100); Hinkley 1971 (doi:10.1093/biomet/58.3.509); Huber 1964
  (doi:10.1214/aoms/1177703732); Hampel 1971 (doi:10.1214/aoms/1177693054); Tukey 1960 (Stanford
  University Press); Kalman 1960 (doi:10.1115/1.3662552); Kaplan, Meier 1958
  (doi:10.1080/01621459.1958.10501452); Efron, Morris 1975 (doi:10.1080/01621459.1975.10479864);
  Zellner 1986 (doi:10.1080/01621459.1986.10478289); Arrow, Harris, Marschak 1951 (Econometrica
  19(3)); Student 1908 (Biometrika 6(1)); Shannon 1948 (Bell System Technical Journal 27).

Code of open players, read in the cache (**strong** for what each does, not for whether it is right):
Media3 `DefaultBandwidthMeter.java` and `experimental/*` (androidx/media,
https://github.com/androidx/media, 2026-09-08); hls.js 1.7.3 `abr-controller.ts`
(https://github.com/video-dev/hls.js); dash.js `ThroughputModel.js`, `AbandonRequestsRule.js`
(https://github.com/Dash-Industry-Forum/dash.js); Shaka `player_configuration.js`
(https://github.com/shaka-project/shaka-player); pion/sctp `association.go`
(https://github.com/pion/sctp).
