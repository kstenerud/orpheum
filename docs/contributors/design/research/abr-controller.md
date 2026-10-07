> **ABOUTME:** What a bitrate controller of our own for hls.js 1.7.3 must implement and how hls.js
> calls it, the ABR algorithms that fit a VOD ladder with a voice cap and paced downloads, and a
> recommended design that absorbs the patches on hls.js's own controller. Gathered 2026-10-07.
> Feeds D265.

# A custom bitrate controller for the chunked player

Research for replacing the patches layered on hls.js's built-in ABR (`stepDownWhileFilling`,
`measuredKbps`, `settledAtBps`, `voiceCap`, `adaptQuality`) with one `abrController` class.
2026-10-07. hls.js read at tag v1.7.3 (`e5ff3583`, cloned to `/yoloai/cache/hls.js`); dash.js
5.2.2 and Shaka read at HEAD (`/yoloai/cache/git/`). Line numbers are hls.js `src/` unless
stated. Note: the web-search budget ran out early in this session, so the algorithm survey relies
on papers verified by DOI via Semantic Scholar, on source code read locally, and on this repo's
earlier research (slow-links.md, cold-start.md). Items marked *[unverified]* were not re-checked.

## Summary / recommendation

1. **Build it, as a subclass of hls.js's `AbrController`, not as a separate class.** The config
   field is typed `abrController: typeof AbrController` (`config.ts:402`), and `AbrController`
   has private members. That makes the type nominal, so a class written from scratch only fits
   with `as unknown as`. Our lint forbids that in production code
   (`web/eslint.config.js:162`, `consistent-type-assertions: never`). A subclass that overrides
   `registerListeners`/`unregisterListeners` and the public accessors keeps hls.js's EWMA
   estimator, its TTFB estimate and its timeout sampling, and replaces every decision.
2. **The algorithm: a throughput rule for decisions, with buffer and pacing awareness**, not BOLA
   or MPC:
   - **Start** at a known-good level: the preload's handed-over estimate, else the remembered
     link (D257), else the top (D260 §2).
   - **An in-flight abandon rule that runs whether or not the video plays.** This absorbs
     `stepDownWhileFilling`, fixes the built-in's refusal to abort a request when stepping to
     level 0, and fires `FRAG_LOAD_EMERGENCY_ABORTED` instead of `stopLoad`/`startLoad`.
   - **Step down** when a buffer-scaled share of the estimate no longer carries the level.
   - **Step up** by the existing D247 §9 rule, unchanged.
   - **Ceilings:** the voice cap (D262 §2) and `hls.maxAutoLevel`.
   - **Samples:** a paced download's sample counts only if it fell short of the pace, and
     cache hits are dropped.
3. **A real bug to fix either way: while voice is live, the voice cap ratchets the film to the
   lowest quality.** Paced downloads are measured at the pace, roughly 1.25×BANDWIDTH of the
   level being loaded, so `voiceCap` (0.5×estimate against AVERAGE-BANDWIDTH) always comes out
   one level lower. Details are under Risks, R1.
4. **Size:** about 450 lines of production code, from about 200 lines of pure decision functions
   plus a class shell of about 200 lines. Against that, about 250 lines go: `fill.ts`, and
   `adaptQuality`/`voiceCap`/`settledAtBps`/`measuredKbps`. Tests come to about 600 lines,
   including a small trace-driven simulator on a fake clock.
5. **Owner decisions owed (rule 1, the design is frozen):**
   - the buffer-scaled share for stepping down (0.7 when the buffer is low, up to 0.95);
   - the rule that a paced sample counts only when it falls short of the pace;
   - aborting to the lowest level.

   Everything else implements D247 §6/§9, D257, D260 and D262 §2 as written.

## hls.js 1.7.3 custom controller interface

### What hls.js requires

- **`AbrComponentAPI`** (`types/component-api.ts:7-13`): `firstAutoLevel: number`,
  `forcedAutoLevel: number`, `nextAutoLevel: number` (get and set),
  `readonly bwEstimator?: EwmaBandWidthEstimator`, `resetEstimator(n)`, and `destroy()` from
  `ComponentAPI`. The docs (`docs/API.md:1338-1349`) list only `nextAutoLevel` and `destroy`,
  plus `bwEstimator.getEstimate()` for `hls.bandwidthEstimate`. **The docs understate it**: core
  code calls the other members too.
- **Construction** (`hls.ts:210,220`): `new config.abrController(hls)` runs before every other
  controller. The instance is a core component, destroyed with `hls` (`hls.ts:284`).
- **The config type** is `typeof AbrController` (`config.ts:402`). `AbrController` is exported
  at runtime (`dist/hls.mjs` export list). `EwmaBandWidthEstimator` is exported **as a type
  only** (`hls.ts:1360` `export type {…}`, line 1398), so a from-scratch controller cannot
  construct one. It would have to write its own, and the `bwEstimator?: EwmaBandWidthEstimator`
  type then needs another assertion.

### Who calls what

| Member | Called by | Notes |
| --- | --- | --- |
| `nextAutoLevel` get | `level-controller.ts:675-680` (`nextLoadLevel`), polled by `stream-controller.ts:265` on **every idle tick** (100 ms) before each fragment | Must be cheap and must not change state when called repeatedly. The built-in caches by `bw_starvation` key (`abr-controller.ts:643-652`) |
| `nextAutoLevel` set | `stream-controller.ts:297` → `level-controller.ts:706-709` writes back **the value it just read**, every fragment; `error-controller.ts:595` (switch down after a load error); `abandon` | The setter must tell an echo of its own choice from an external force. The built-in does it with `nextAutoLevelKey` (`:1041-1047`, `:600-605`). `deriveNextAutoLevel` clamps to [minAutoLevel, maxAutoLevel] (`:1049-1055`) |
| `forcedAutoLevel` | `hls.ts:844-850` (`startLevel` getter when startLevel is −1) | −1 unless forced |
| `firstAutoLevel` | `level-controller.ts:528-536` (startLevel when neither `hls.startLevel` nor `config.startLevel` is set), `stream-controller.ts:153-160` (startLevel −1 and `testBandwidth` false), `audio-track-controller.ts:294` | This is where a known-good start belongs |
| `bwEstimator.getEstimate/getEstimateTTFB/defaultEstimate` | `hls.ts:909-939` → `hls.bandwidthEstimate`, `ttfbEstimate`, `abrEwmaDefaultEstimate`; used by `base-stream-controller.ts:2496-2498` (nearly-downloaded check), `:2545-2546` (`calculateOptimalSwitchPoint` for `nextLevelSwitch` flushes), CMCD `mtp`, content steering | **Must stay finite.** NaN poisons the switch-point maths |
| `resetEstimator(bps)` | `hls.bandwidthEstimate = x` (`hls.ts:917-919`); `level-controller.ts:343-354` sets it to the first variant's bitrate (at most `abrEwmaDefaultEstimateMax`, 5 Mb/s) at manifest load, **unless `userConfig.abrEwmaDefaultEstimate` is set** | Pass the prior as `abrEwmaDefaultEstimate` in userConfig, or it is overwritten |

### Events the built-in listens to (`abr-controller.ts:86-97`)

| Event | What the built-in does on it |
| --- | --- |
| `MANIFEST_LOADING` | resets |
| `FRAG_LOADING` | starts the 100 ms abandon timer (`:156-167`) |
| `FRAG_LOADED` | samples TTFB; sets `lastLoadedFragLevel`; clears the forced level (`:472-513`) |
| `FRAG_BUFFERED` | samples bandwidth from `parsing.end − loading.start − min(ttfb, est)` (`:515-551`) |
| `LEVEL_SWITCHING` | clears the timer |
| `LEVEL_LOADED` | VoD/live half-lives; playlist load time (`:226-241`) |
| `LEVELS_UPDATED` | resets the cached level selection |
| `MAX_AUTO_LEVEL_UPDATED` | resets the cached level selection |
| `ERROR` | `FRAG_LOAD_TIMEOUT` sampling; resets after an append or codec error (`:176-211`) |

Only MAIN, non-init fragments count (`ignoreFragment`, `:553-556`).

### autoLevelCapping, startLevel, capLevelToPlayerSize, minAutoBitrate

- **`hls.maxAutoLevel`** (`hls.ts:1000-1020`) is `autoLevelCapping`, or the top level when it is
  −1, lowered further by HDCP. Setting `autoLevelCapping` fires `MAX_AUTO_LEVEL_UPDATED`
  (`level-controller.ts:755-765`).
- **Other code writes `autoLevelCapping` too:** `cap-level-controller.ts:181`
  (`capLevelToPlayerSize`; it also calls `nextLevelSwitch()` to flush when the cap rises,
  `:182-190`) and `fps-controller.ts:112` (`capLevelOnFPSDrop`). Today we use
  `autoLevelCapping` as our own step-up state (`chunked.ts:188-217`), which would fight either
  of them. **A custom controller should keep its ceiling in its own state** and treat
  `hls.maxAutoLevel` only as an outside limit.
- **`startLevel`:**
  - `hls.startLevel = n` beats `config.startLevel`, which beats `firstAutoLevel`
    (`level-controller.ts:528-538`).
  - With `testBandwidth: true` and startLevel −1, stream-controller loads level 0 as a bitrate
    test and ignores `firstAutoLevel` (`stream-controller.ts:153-157`). **Keep
    `testBandwidth: false`**, which `chunkedHlsConfig` already sets.
  - The recommended design leaves `startLevel` unset and answers through `firstAutoLevel`.
- **`minAutoBitrate`** gives `minAutoLevel` (`hls.ts:980-995`). **`abrSwitchInterval`**
  throttles switches in `level-controller.ts:686-703`. Leave it 0: our controller does its own
  holding.

### The built-in abandon rule, and why we patched around it (`abr-controller.ts:247-470`)

- **It is skipped while paused** (`:285-290`): `media.paused || !playbackRate || !readyState`
  returns early, unless a level change is pending. The hidden preload player and a player
  waiting for its first frame are paused, which is why `fill.ts` exists.
- **Its timing:**
  - It waits until loading time exceeds `max(ttfbEstimate, duration/2)` (`:300-305`).
  - It abandons only if the fragment's remaining load time exceeds the buffer left (`:335`)
    **and** a lower level would load sooner (`:370`). It ignores lower levels that would take
    more than 10× the duration (`:375`).
- **It wipes the estimate.** It resets the estimator to the lower level's BANDWIDTH whenever
  `estimate×0.7 > that bitrate` (`:389-392`), and again after aborting (`:456`). Measured history
  is thrown away and replaced by a nominal rate.
- **Quirk: it never aborts when stepping to level 0.** `abortAndSwitch` aborts only if
  `nextLoadLevel > 0` (`:429-433`). The lower-level search stops at `minAutoLevel` (`:346-350`),
  so on a link that only fits the lowest rung, the slow fragment is **never aborted**. hls.js
  just sets the next level, after the current one finishes. This is in the source; the intent
  is not documented. It matches the staging symptom, "waited out a whole chunk", in our worst
  case.
- **How it aborts:** `frag.abortRequests()`, then
  `hls.trigger(FRAG_LOAD_EMERGENCY_ABORTED, {frag, part, stats})` (`:469`).
  `stream-controller.ts:1083-1092` handles that event: state goes to IDLE, and if the start has
  not buffered enough yet, `startFragRequested` is reset and loading restarts at
  `lastCurrentTime`. **This is the supported way for a controller to abort**, cleaner than
  `fill.ts`'s `stopLoad()` and `startLoad(startPosition)`.

### Level choice in the built-in (`findBestLevel`, `:768-1039`)

- **The levels it considers:**
  - From the top down, it skips levels that change codec set, video range or frame rate
    (`:931-946`).
  - It asks `navigator.mediaCapabilities.decodingInfo` per level and **removes a level that
    reports unsupported** (`:855-927`; `useMediaCapabilities` is on by default, `config.ts:539`).
  - It skips levels in the error penalty box (`loadError`/`fragmentError`, `:987-989`).
- **The rate it compares:**
  - It uses `maxBitrate`, which is BANDWIDTH, unless the buffer is at least two fragments and no
    rebuffering is expected; then it uses `averageBitrate` (`:969-975`).
  - The factors are 0.95 to stay or go down and 0.7 to go up (`config.ts:511-512`). Fetch time
    adds TTFB, plus a playlist load for a level not yet loaded (`:213-224`).
- **For us:** our top rung's BANDWIDTH is 36 Mb/s against an average of 5.1 Mb/s
  (`fill.test.ts:17-21`). The built-in therefore holds the top rung only on a link above
  roughly 38–51 Mb/s while the buffer is short.

### The estimator (`utils/ewma-bandwidth-estimator.ts`)

- Each sample is weighted by its duration in seconds (`:48-60`), and the estimate is the minimum
  of a fast (3 s) and a slow (9 s) EWMA (`:74-84`). A sample's minimum duration is
  `min(50 ms, bytes/100000)`.
- **There is no cache-hit filter.** Shaka and dash.js drop responses under 5–10 ms
  (slow-links.md §2), and hls.js#5826 (still open) reads a throttled 1 Mb/s link as 404 MB/s.
- `link.ts`'s `MovingAverage` re-implements the same EWMA for files. Rule 11 applies if we
  write a third one.

### Known pitfalls for a subclass

- **The base constructor calls the virtual `registerListeners()`** (`:62`) before the subclass's
  field initialisers run. An override must use only prototype methods, and must create its state
  lazily or in a method called after `super()`.
- **Override `nextAutoLevel`'s getter and setter together.** Overriding one accessor of a pair
  hides the other.
- **Private parent members** (`_abandonRulesCheck`, `findBestLevel`, `onLevelsUpdated`) cannot
  be reached or re-registered. If `registerListeners` is overridden, they simply never run,
  which is what we want. The parent's `onLevelLoaded` calls `_abandonRulesCheck` only if
  `timer > -1`, and that timer is started only by the parent's `onFragLoading`. Do not register
  that one.
- `hls` is `protected` and typed non-null, with "TODO: Make nullable in v1.8" (`:37`). hls.js is
  pinned exactly (`"hls.js": "1.7.3"`). Any bump has to re-read this file.
- `resetEstimator` writes `hls.config.abrEwmaDefaultEstimate` (`:69-72`).
- Playlists that are still growing (EVENT, no ENDLIST, D259 §3 read-ahead) are treated as live.
  That means live half-lives (the same 3/9 s by default) and a playlist reload on every switch
  (`:354`, `:980`).

## Algorithms surveyed

| Family | Representative | How it decides | Fit for us |
| --- | --- | --- | --- |
| Throughput | hls.js `AbrController`; Shaka `SimpleAbrManager` (EWMA 2/5 s, down 0.95, up 0.85, `switchInterval` 8 s, ignores progress samples under 16 kB, trusts after 128 kB; `shaka-player/lib/util/player_configuration.js:312-343`); FESTIVE (Jiang et al., CoNEXT'12 *[unverified link]*) | the highest level under a safety factor × estimate | **Good at start and seek**, when the buffer is empty. Noisy in steady state. Shaka samples **in-flight progress**, which matters for our long 10 s chunks |
| Buffer-based | Netflix BBA (Huang et al., SIGCOMM'14, doi:10.1145/2619239.2626296) | buffer → rate map, with a reservoir and a cushion; throughput only during startup | Steady state only. Pure BBA-0 started at the lowest level and lost 700 kb/s in the first 60 s (cold-start.md §2). A 10 s preload buffer leaves it no room. It also conflicts with D247 §9's held step-up |
| Buffer-based, Lyapunov | BOLA (Spiteri, Urgaonkar, Sitaraman, INFOCOM'16, arXiv:1601.06748); dash.js `BolaRule.js` | maximises utility (log bitrate) minus a buffer penalty; `MINIMUM_BUFFER_S=10`, +2 s per level, buffer target `max(bufferTimeDefault 18, 10+2·n)` (`BolaRule.js:54-59,257`) | It needs at least 10 s of buffer to differentiate levels. Our preload holds 10 s, and steady state holds 30 s, so it would work in steady state only. Its utility assumes sizes that track the declared bitrate. Our picture-copy top rung is VBR with a peak 7× its average, so it needs per-segment sizes (EXT-X-BITRATE) to be right |
| Hybrid | dash.js DYNAMIC: Throughput below `hybridSwitchBufferTime`, BOLA from 12 s, back below 6 s (hysteresis; `controllers/AbrController.js:993-1001`, `core/Settings.js:1424`); plus InsufficientBufferRule (safety 0.5) and AbandonRequestsRule (abandon if est. download > 1.8 × segment duration, after 6 progress samples, unless buffer > `bufferTimeDefault`, `rules/abr/AbandonRequestsRule.js`, `Settings.js:1458-1464`). Spiteri et al. ACM TOMM 2019 (URL in slow-links.md) | throughput while the buffer is low, buffer-based when it is high | **The closest published match to our shape.** Note that its abandon rule runs **regardless of play state**, unlike hls.js |
| Low-latency live | L2A-LL (Karagkioules et al., MMSys'20, doi:10.1145/3339825.3397042); LoL+ (Bentaleb et al., IEEE TMM 2022, doi:10.1109/TMM.2021.3079288) | online learning or a self-organising map, tuned for buffers under about 3 s | **Not for VoD with a 30 s buffer.** Their target is latency we do not have |
| Control-theoretic | MPC / RobustMPC (Yin et al., SIGCOMM'15, doi:10.1145/2785956.2787486) | maximises QoE over a horizon of about 5 chunks from a throughput forecast (Robust: forecast ÷ (1+max recent error)) | Computation is trivial: 5 rungs over a horizon of 5 is 3125 plans. It needs segment sizes and QoE weights to tune. Its gain shows on fluctuating links with many rungs. **Not worth it at 4–5 rungs for two people.** A Robust-style error discount is a cheap idea to borrow |
| Learned | Pensieve (Mao et al., SIGCOMM'17, doi:10.1145/3098822.3098843); Puffer/Fugu (Yan et al., NSDI'20, URL in slow-links.md) | an RL policy, or a learned transmit-time predictor plus MPC | Needs training data we do not have. Puffer's finding, "simple buffer-based control (BBA) performs surprisingly well", argues **against** complexity |
| Pacing-aware | Sammy, Netflix (Spang et al., SIGCOMM'23, doi:10.1145/3603269.3604839): "application-informed pacing, which allows ABR algorithms to set an upper limit on packet-by-packet throughput, and … designing ABR algorithms that work with pacing … a joint ABR and rate-control scheme, which selects both video quality and pacing rates" (abstract) | joint choice of quality and pace; QoE no worse in production | **Directly relevant to D262 §2.** The general lesson is that **paced throughput is not link capacity**, so the ABR must not read it as such. How Sammy treats paced samples in detail was not re-read *[unverified]* |
| YouTube | none public | *[unverified]*: no engineering description of YouTube's web ABR was found in this or earlier research (cold-start.md §2) | Not a citable model |

What each would mean for our five needs:

- **A fast start from a known-good level:** every family needs a prior. Netflix uses "longer-term
  historical information about the network and device" (cold-start.md §2), which is D257 exactly.
  Throughput and hybrid schemes take the prior as the estimator's default; BOLA in dash.js starts
  in `BOLA_STATE_STARTUP` from throughput. **The prior goes into the estimator, not into a
  hard-wired level.**
- **Stepping down during the initial fill:** only an abandon rule that ignores play state (dash.js
  style) does this. Buffer-based rules can't, because the buffer is empty. **Ours must run while
  paused.**
- **VoD with a 30 s buffer:** this is where buffer awareness pays off. A buffer of 20–30 s
  tolerates the estimate falling briefly below the level's average, so the down rule's share can
  rise with the buffer (BBA's cushion; hls.js's switch from max to average bitrate at two or more
  fragments, `:969-975`).
- **The voice cap:** an outside ceiling in every family. The only subtlety is which estimate it is
  measured against (see Risks, R1).
- **Segment pacing:** Sammy. A paced sample is a lower bound on the link, informative only when it
  falls short of the pace.

## Recommended design

**`OrpheumAbr extends AbrController`**, in `web/packages/player/src/abr.ts` (shell) plus
`abr-choice.ts` (pure functions). It is built per playback through a factory that closes over its
options. The class goes in config as `abrController: orpheumAbr({ prior, monotonicMs,
isVoiceLive, onSettled })`, and the factory returns `class extends AbrController {…}`, so no
assertion is needed.

### Inputs

- **The levels:** `hls.levels`, read for `averageBitrate`, `bitrate` (BANDWIDTH), `loadError`,
  `fragmentError` and `supportedResult`.
- **`hls.minAutoLevel` and `hls.maxAutoLevel`.**
- **The buffer ahead:** `hls.mainForwardBufferInfo.len`, and whether the video is playing.
- **The fragment in flight:** `frag.stats.loading.start/first`, `stats.loaded`, `frag.duration`,
  and `frag.bitrate` when the playlist carries EXT-X-BITRATE (`m3u8-parser.ts:69,560`).
- **Estimates:** the parent's `bwEstimator` (EWMA) and its TTFB estimate.
- **The prior (bps):** the preload's handover, else D257's remembered link (`startAtBps`), else
  none.
- **`isVoiceLive()`, the pace this playback asks for, and `monotonicMs`** (injected; never
  `performance.now()` in our own logic).

### State

- the `phase`: `starting`, `filling` or `steady`;
- `ceiling`, the D247 §9 step-up cap;
- `ceilingChangedAtMs`;
- `lastChosen`, used to tell the setter's echoes from outside forces;
- `forced`, with the fragment it applies to (error-controller's switch-down);
- `inFlight`: the fragment, its start time, and whether it was paced;
- `linkBps`, the estimate built only from unpaced samples or link-limited paced ones;
- `hasMeasured`.

### Rules, in order (each one a pure function)

1. **The start (`firstAutoLevel`).**
   - If the prior is set, start at the highest level whose AVERAGE-BANDWIDTH is at most 0.7 ×
     the prior (D247 §6). The prior also seeds the estimator: pass
     `abrEwmaDefaultEstimate: prior` in userConfig so `level-controller.ts:343-354` keeps it.
   - With no prior, start at the top (D260 §2).
   - `ceiling` starts at the start level. This replaces `adaptQuality`'s `MANIFEST_PARSED`
     handler and the `hls.startLevel`/`autoLevelCapping` writes.
2. **The in-flight rule** runs on a 250–500 ms timer from `FRAG_LOADING` to
   `FRAG_LOADED`/abort, **regardless of `media.paused`**. It generalises `fillStepDown` and
   absorbs `stepDownWhileFilling`.
   - Act once the fragment has loaded for at least `max(ttfbEst, duration/2)` and the current
     level is above `minAutoLevel`. Measure the arrival rate from progress (`stats.loaded`),
     excluding TTFB.
   - Abandon if the remaining time at that rate exceeds `max(bufferAheadS, 0)` (at the fill
     that is "slower than it plays") **and** a lower level's whole fragment would arrive sooner.
   - Choose the highest level whose average is at most 0.7 × the arrival rate, **level 0
     included**.
   - Abort as hls.js does: `frag.abortRequests()` and
     `hls.trigger(FRAG_LOAD_EMERGENCY_ABORTED, …)`.
   - Lower `ceiling` to the chosen level. Do **not** reset the estimator to a nominal rate: feed
     it the partial sample instead.
3. **Steady choice (`nextAutoLevel` getter)**, when there is no outside force.
   - `target` = the highest level at most `min(ceiling, voiceCeiling, hls.maxAutoLevel)` with
     `averageBitrate ≤ share(buffer) × linkBps`, where `share` runs from 0.7 at a buffer of 10 s
     or less to 0.95 at 25 s or more, linearly (*owner decision*). The levels are filtered by
     error penalty and `supportedResult`.
   - Never return above the level being played unless `ceiling` was raised: a step up comes
     only from rule 4. The result is cached until the estimate, buffer bucket or ceiling
     changes, because the getter is polled every 100 ms.
4. **Step up (a 10 s look).** `nextCap` from `step-up.ts` stays as it is (D247 §9: one level,
   held 120 s, at least 20 s buffered, link at least 1.5 × the next level's average). It reads
   `linkBps`, and raises `ceiling`. hls.js picks the new level at the next fragment, so there is
   no flush.
5. **The voice ceiling (D262 §2):** while voice is live, the highest level with average at most
   0.5 × **`linkBps`**, the estimate frozen against pacing, never the paced throughput.
6. **Sampling.** Override `onFragBuffered`, and call `super.onFragBuffered` only when:
   - the load was not a cache hit (a load under 10 ms, or a sample that is very fast at
     negligible weight); the preload's chunks are re-read from the HTTP cache by the Play
     page's player; and
   - for a paced download, its rate was at most 0.9 × the pace asked (the link, not the pace,
     limited it) (*owner decision*).

   Otherwise record only `hasMeasured`.
7. **Forces.** A setter value other than `lastChosen` is an outside force (from error-controller
   or audio-track-controller). Expose it as `forcedAutoLevel`, return it until a fragment loads at
   that level, and lower `ceiling` to it. A setter value equal to `lastChosen` is the stream
   controller's echo, and is ignored.
8. **Handover.** `estimateKbps()` is `hasMeasured ? linkBps/1000 : undefined`, which absorbs
   `measuredKbps`. `settled()` returns `{ levelBps, linkBps }` for the Play page's player to
   use as its prior, which absorbs `settledAtBps` and hands over a measured link, not just a
   level.
9. **The pace** is owned here: `paceKbps()` is `ceil(1.25 × BANDWIDTH of the level loading)`
   while voice is live, read by `xhrSetup`. The controller then knows which samples were paced.

**Absorbed:**

- `stepDownWhileFilling` and `fillStepDown`, into rule 2;
- `measuredKbps`, into rule 8;
- `settledAtBps`, into rule 8;
- `voiceCap` and the pace, into rules 5 and 9;
- `adaptQuality`'s start and its interval, into rules 1 and 4.

`step-up.ts`, `beyondTheLink` (it reads `linkBps`) and the D247 §9 numbers stay.

**Optional later:**
- **Per-segment sizes:** the core emits EXT-X-BITRATE per segment for the picture-copy rung,
  where the file's index (D259 §2) gives the sample sizes. hls.js already uses `frag.bitrate` in
  its abandon maths (`:320`, `:533`). That would make rules 2 and 3 exact on the VBR rung, and
  would make BOLA viable if steady state ever needs it.
- **A RobustMPC-style discount:** divide `linkBps` by (1 + the largest recent relative
  prediction error) if oscillation shows up.

## Testing

All of it is deterministic: no network, no real clock.

1. **Pure functions, as table tests**, in the style of `fill.test.ts` and `step-up.test.ts`, on
   the Conan ladder: `startLevel(prior, levels)`, `inFlightVerdict(...)`,
   `steadyChoice(...)`, `voiceCeiling(...)`, `countsAsSample(sample, pace)` and `share(buffer)`.
   Each one names its D-number.
2. **The shell, on a real `new Hls(config)`** with no media attached. `hls.test.ts:33` shows that
   it constructs under vitest.
   - Drive it with `hls.trigger(Events.FRAG_LOADING/FRAG_LOADED/FRAG_BUFFERED, {frag, stats})`,
     using synthetic `LoadStats`.
   - Assert through the public surface: `hls.nextAutoLevel`, `hls.firstAutoLevel`,
     `hls.bandwidthEstimate`, and `hls.on(FRAG_LOAD_EMERGENCY_ABORTED)`.
   - Use `vi.useFakeTimers()` for the timer, with `monotonicMs` injected. Stats timestamps are
     plain numbers we set, so hls.js's own `performance.now()` is never consulted on those paths.
   - This also pins the wiring that a hls.js bump could break: listener registration, the
     accessor pair, and the echo-versus-force setter.
3. **A trace-driven simulator** (test helper, about 150 lines; the Pensieve/Puffer method): a fake
   link (bps over fake time, optionally paced), a fragment list (duration and bytes per level), a
   buffer that drains at 1× while "playing", and the controller's pure functions in the loop.
   Scenarios:
   - starting at the top on 1 Mb/s, which must reach level 0 within one half-chunk (the staging
     2026-10-07 case);
   - a known-good prior on a 20 Mb/s link, which must not step down;
   - voice going live on 20 Mb/s, which must hold the 0.5 × link level and must **not** ratchet
     (R1);
   - a link drop mid-film with 30 s buffered, which must step down once, not to the floor;
   - a preload handover through cached chunks, where the estimate must not spike.

   Each scenario asserts a stall count and switch counts. Revert-red: each scenario fails when
   its rule is removed.
4. **The device side stays in the existing e2e acceptance** under D263's netem profiles
   (`web/e2e/acceptance.spec.ts`). MSE does not exist in jsdom.

## Risks

- **R1. Voice-cap ratchet.** *(Fixed the same day in the existing player, before any device saw
  it: the cap reads the link as last measured unpaced.)* `voiceCap(levels, hls.bandwidthEstimate)`
  (`chunked.ts:145-153,209-212`) measures a link whose film downloads are paced at
  1.25 × BANDWIDTH (`chunked.ts:166-169`; the server's `SO_MAX_PACING_RATE` through
  `internal/platform/pacing`). The EWMA converges to about 1.25 × peak_i. For the Conan rungs,
  0.5 × 1.25 × peak_i = 0.625 × peak_i, which is below avg_i (avg/peak is 0.67–0.71), so the
  cap is level i−1. Then the pace drops, the estimate follows, and the cap drops again, down to
  the lowest level. The step-up never recovers it: it needs an estimate of 1.5 × the next
  average. This is inferred from the code and the arithmetic, not observed on a device. A
  simulator test would confirm it.
- **R2. Lost: MediaCapabilities filtering and codec/range/frame-rate tiering**
  (`findBestLevel` `:855-946`). Our ladder offers the picture copy only "where the player decodes
  it" (D260 §1). If that decision is the server's, we lose little. Otherwise call
  `hls.getMediaDecodingInfo(level)` ourselves or keep `supportedResult` filtering.
- **R3. Lost: the error penalty box** (`:987-989`, `errorPenaltyExpireMs`). We must honour
  `loadError`/`fragmentError` and error-controller's forced level (rule 7). The 425
  "being made" retries (`chunked.ts:44-74`) are load errors, so a level being made could be
  penalised.
- **R4. The estimator.** Keeping the parent's `bwEstimator` keeps the EWMA and TTFB that hls.js's
  other controllers read. A from-scratch class would have to reproduce `getEstimate`,
  `getEstimateTTFB` and `defaultEstimate` and keep them finite (`base-stream-controller.ts:2545`).
  Cache-hit filtering is new and must not starve the estimator. `canEstimate()` still gates it.
- **R5. Abort semantics on the first fragment.** `FRAG_LOAD_EMERGENCY_ABORTED` resets
  `startFragRequested` only before `_hasEnoughToStart`. After that, hls.js's normal IDLE tick
  reloads at the next position. Aborting at level 0 is new behaviour: hls.js deliberately
  (or accidentally) never does it (`:432`). Keep "a lower level's whole fragment would arrive
  sooner" as the guard so we do not abort-loop.
- **R6. Coupling to hls.js internals.** The design touches only public or protected members,
  but those protected handlers and the double-polled getter are undocumented behaviour. hls.js
  is pinned exactly; the shell tests in Testing 2 are the tripwire on any bump. 1.8 plans
  nullable `hls` (`:37`).
- **R7. Native HLS (Safari on iPhone) gets none of this:** `playNative` adapts by itself. The
  voice cap and pacing do not exist there today either.
- **R8. Design freeze.** The buffer-scaled share, the paced-sample rule, aborting to level 0 and
  the cache-hit filter are new numbers or rules. They go to the owner before building (AGENTS.md
  rule 1). D247 §9's step-up and §6's 70% are kept as written.
- **R9. Lint shape.** Complexity ≤ 10 and ≤ 60 lines per function
  (`web/eslint.config.js:246-249`) push toward the pure-function split above, which suits the
  tests anyway.

## Sources

**hls.js v1.7.3** (`e5ff3583`), https://github.com/video-dev/hls.js/tree/v1.7.3:

- `src/controller/abr-controller.ts`
- `src/types/component-api.ts`
- `src/hls.ts`
- `src/config.ts`
- `src/controller/level-controller.ts`
- `src/controller/stream-controller.ts`
- `src/controller/base-stream-controller.ts`
- `src/controller/error-controller.ts`
- `src/controller/cap-level-controller.ts`
- `src/utils/ewma-bandwidth-estimator.ts`
- `src/utils/ewma.ts`
- `docs/API.md` §abrController
- `web/node_modules/hls.js/dist/hls.mjs` (runtime exports)

**Other players:**

- dash.js 5.2.2, https://github.com/Dash-Industry-Forum/dash.js: `src/streaming/controllers/AbrController.js`, `src/streaming/rules/abr/{BolaRule,AbandonRequestsRule,InsufficientBufferRule}.js`, `src/core/Settings.js`.
- Shaka Player, https://github.com/shaka-project/shaka-player: `lib/util/player_configuration.js`, `lib/abr/ewma_bandwidth_estimator.js`.

**Papers** (DOIs verified via Semantic Scholar):

- Huang, Johari, McKeown, Trunnell, Watson, *A Buffer-Based Approach to Rate Adaptation*, SIGCOMM'14, https://doi.org/10.1145/2619239.2626296 (PDF https://yuba.stanford.edu/~nickm/papers/sigcomm2014-video.pdf).
- Spiteri, Urgaonkar, Sitaraman, *BOLA: Near-optimal bitrate adaptation for online videos*, INFOCOM'16, https://arxiv.org/abs/1601.06748.
- Spiteri, Sitaraman, Sparacio, *From theory to practice: improving bitrate adaptation in the DASH reference player*, ACM TOMM 2019, https://groups.cs.umass.edu/wp-content/uploads/sites/3/2019/12/From-theory-to-practice-Improving-bitrate-adaptation-in-the-DASH-reference-player.pdf.
- Yin, Jindal, Sekar, Sinopoli, *A Control-Theoretic Approach for Dynamic Adaptive Video Streaming over HTTP*, SIGCOMM'15, https://doi.org/10.1145/2785956.2787486.
- Mao, Netravali, Alizadeh, *Neural Adaptive Video Streaming with Pensieve*, SIGCOMM'17, https://doi.org/10.1145/3098822.3098843.
- Yan et al., *Learning in situ: a randomized experiment in video streaming* (Puffer), NSDI'20, https://www.usenix.org/system/files/nsdi20-paper-yan.pdf.
- Karagkioules et al., *Online learning for low-latency adaptive streaming* (L2A), MMSys'20, https://doi.org/10.1145/3339825.3397042.
- Bentaleb et al., *Catching the Moment With LoL+ in Twitch-Like Low-Latency Live Streaming Platforms*, IEEE TMM 2022, https://doi.org/10.1109/TMM.2021.3079288.
- Spang et al., *Sammy: smoothing video traffic to be a friendly internet neighbor*, SIGCOMM'23, https://doi.org/10.1145/3603269.3604839 (abstract only).
- Huang et al., *Confused, Timid, and Unstable*, IMC'12, http://yuba.stanford.edu/~nickm/papers/Confused_Timid_and_Unstable_Picking_a_Video_Streaming_Rate_is_Hard.pdf.

**Issues:**

- hls.js#5826 (cache hits read as bandwidth), https://github.com/video-dev/hls.js/issues/5826.

**This repo:**

- `docs/contributors/decisions/working-notes.md`: D247, D257, D258, D260, D262, D263
- `docs/contributors/design/research/slow-links.md` §2
- `docs/contributors/design/research/cold-start.md` §2
- `web/packages/player/src/{fill,chunked,step-up,hls,link}.ts`
- `web/apps/web/src/watching.ts`
- `internal/core/api/pacing.go`
- `internal/platform/pacing/pacing.go`
