// SPDX-License-Identifier: MPL-2.0

/**
 * The chunked player's own estimate of its link (D352 §2, revised 2026-10-29): the last few chunks
 * that measured it, each counted once whatever its length, their centre and spread in log space,
 * robust to a sample that is not the link's, and how far to trust it.
 */

/**
 * How many chunks that measured the link the estimate rests on: RobustMPC's window, a quarter of
 * FESTIVE's 20, which at our chunks' 4–10 s would remember minutes (the research, §3).
 */
export const linkSamplesKept = 5;

/**
 * The largest error of the estimate's own recent predictions at which its samples agree, and the
 * step-up may climb several qualities at once: about one rung's gap on the ladder (0.7 → 1.2 → 2 →
 * 3 Mb/s), so samples that agree within it name one rung, not two (D352 §2).
 */
export const samplesAgreeWithinError = 0.2;

/**
 * How many samples must be kept before the single most extreme prediction error is set aside
 * (predictionError): all linkSamplesKept, so what is left rests on three errors at least, and two
 * slow samples of five still count (D352 §4).
 */
export const trimsErrorFromSamples = linkSamplesKept;

/**
 * How many spreads from the centre a sample counts at most: one further counts as if it were this
 * far (Huber's estimator). A textbook starting value, 1.5–2 in the literature, not measured on our
 * links: to be tuned against the calibration log (bandwidth-sampling-2026-10-29.md §2.4, §6.1).
 */
export const clipAtSpreads = 2;

/**
 * The least spread of the samples' natural logs the estimate assumes, about ±35% for one standard
 * deviation: five clean samples' own spread can come out near 0, and a bound on it would then trust
 * them as exact. A textbook starting value, not measured on our links (bandwidth-sampling §2.3, §6.1).
 */
export const logSpreadAtLeast = 0.3;

/**
 * The spread assumed, in natural log, while too few samples are kept to measure one and nothing
 * remembered says it: a Fire TV's four clean chunks spread 0.38 (staging, 2026-10-28), and CS2P found
 * most sessions within 0.3–0.5 (bandwidth-sampling §2.3). A starting value.
 */
export const assumedLogSpread = 0.38;

/**
 * The fewest samples kept whose own spread is measured: two say almost nothing of how a link varies
 * (Student's t at two samples is 3.08 against 1.28; bandwidth-sampling §2.2).
 */
export const spreadFromSamples = 3;

/**
 * The scale from a median absolute deviation to a standard deviation where samples are normal: a
 * mathematical constant, 1/Φ⁻¹(3/4), not a tuning.
 */
const madToSpread = 1.4826;

/**
 * How many spreads from the remembered link two of the playback's own samples must lie, on the same
 * side, to drop it, and within how many spreads of each other: they agree with each other and not
 * with it, so the link has changed since (prior-data conflict; bandwidth-sampling §2.5, §6.2). One
 * sample far from it is far likelier a fresh connection or a stuck request than a link changed in the
 * minutes since. Textbook starting values, a CUSUM's "two in a row beyond 2σ", not measured on our
 * links.
 */
export const dropsPriorBeyondSpreads = 2;
export const dropsPriorWithinSpreads = 4;

/**
 * How many standard deviations below the centre a choice reads the link, by how much is loaded
 * ahead, in seconds: the 10th percentile with nothing loaded, the 20th at 10 s, the median from 20 s,
 * straight between. A too-high choice costs a stall with nothing loaded and only a later swap with
 * 20 s loaded, and under an asymmetric cost the best reading is the quantile it sets (the
 * newsvendor's rule; bandwidth-sampling-2026-10-29.md §2.7, P4); the share a quality may take
 * (D265 §4) stays the headroom over its rate. Textbook starting values, not measured on our links:
 * the calibration log (P6) is to tune them.
 */
export const boundDeviationsByAheadS: readonly (readonly [aheadS: number, deviations: number])[] = [
  [0, 1.28],
  [10, 0.84],
  [20, 0],
];

/** How many times the centre is clipped and averaged again at most: each brings it nearer, and a few settle it. */
const centreRounds = 20;

/** One chunk that measured the link: its quality, its bytes, how long they took to arrive, and its first byte's wait. */
export interface LinkSample {
  readonly level: number;
  readonly bytes: number;
  /** From its first byte to its last, in ms, at least 1 (arrivalMs). */
  readonly arrivedMs: number;
  /** From its ask to its first byte, in ms. */
  readonly firstByteMs: number;
  /**
   * How long its bytes were expected to take to arrive, in ms, at the estimate that stood before it,
   * and at that estimate's next-chunk bound with nothing loaded, its 90% bound, which it should take
   * longer than about one time in ten: its calibration (bandwidth-sampling-2026-10-29.md P6); absent
   * before any estimate.
   */
  readonly expectedMs?: number;
  readonly boundMs?: number;
  /** The next-chunk bound a choice read as it came, with what was loaded ahead then, in ms. */
  readonly readBoundMs?: number;
}

/**
 * The link its device last measured on its network, as the playback's offer says it (D257 §1): its
 * speed, the spread of the natural logs of the samples it rested on where its player said one, and
 * how many of the playback's own samples it counts as, two while under an hour old, fewer as it ages
 * (bandwidth-sampling-2026-10-29.md P2).
 */
export interface RememberedLink {
  readonly kbps: number;
  readonly logSpread?: number | undefined;
  readonly countsAsSamples: number;
}

/** A sample as the estimate keeps it: its speed, and the estimate that stood before it came. */
interface Kept extends LinkSample {
  readonly bps: number;
  readonly predictedBps: number | undefined;
}

/** A point of the estimate in log space, and how many samples it counts as. */
interface Weighed {
  readonly log: number;
  readonly weight: number;
}

/**
 * How many standard deviations below the centre a choice reads the link with bufferAheadS loaded
 * ahead (boundDeviationsByAheadS), straight between its points, the last's beyond it.
 */
export function boundDeviationsAt(bufferAheadS: number): number {
  const points = boundDeviationsByAheadS;
  const next = points.findIndex(([atS]) => bufferAheadS <= atS);
  const [toS, to] = points[next] ?? points.at(-1) ?? [0, 0];
  // Before the first point, and past the last, its own value.
  const [fromS, from] = (next > 0 ? points[next - 1] : undefined) ?? [toS, to];
  return toS === fromS ? to : from + ((to - from) * (bufferAheadS - fromS)) / (toS - fromS);
}

/** The median of values; undefined for none. */
function medianOf(values: readonly number[]): number | undefined {
  if (values.length === 0) {
    return undefined;
  }
  const sorted = values.toSorted((first, second) => first - second);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 1
    ? sorted[middle]
    : ((sorted[middle - 1] ?? 0) + (sorted[middle] ?? 0)) / 2;
}

/** The weighted median of points: the first, from the lowest, by which half their weight is reached. */
function weightedMedianOf(points: readonly Weighed[]): number {
  const sorted = points.toSorted((first, second) => first.log - second.log);
  const half = sorted.reduce((sum, point) => sum + point.weight, 0) / 2;
  let reached = 0;
  for (const point of sorted) {
    reached += point.weight;
    if (reached >= half) {
      return point.log;
    }
  }
  return sorted.at(-1)?.log ?? 0;
}

/**
 * The centre of points in log space by Huber's estimator: from their weighted median, each point
 * further than clipAtSpreads × spread from the centre counts as if it were that far, and their
 * weighted mean is taken, again until it settles (bandwidth-sampling §2.4).
 */
function huberCentreOf(points: readonly Weighed[], spread: number): number {
  const reach = clipAtSpreads * spread;
  const total = points.reduce((sum, point) => sum + point.weight, 0);
  let centre = weightedMedianOf(points);
  for (let round = 0; round < centreRounds; round += 1) {
    const next =
      points.reduce(
        (sum, point) =>
          sum + point.weight * Math.min(centre + reach, Math.max(centre - reach, point.log)),
        0,
      ) / total;
    const isSettled = Math.abs(next - centre) < 1e-9;
    centre = next;
    if (isSettled) {
      break;
    }
  }
  return centre;
}

/**
 * The link as its last linkSamplesKept chunks measured it (D352 §2, revised 2026-10-29). Each
 * sample's speed is taken in natural log, a throughput's errors being ratios, not differences. Its
 * **spread** is the median absolute deviation of those logs, as a standard deviation, never below
 * logSpreadAtLeast, and assumedLogSpread until spreadFromSamples are kept. Its **estimate**, which
 * every choice of quality reads, is e to their **centre**, Huber's: a sample further than
 * clipAtSpreads spreads from it counts as if it were that far, so no one sample rules it. The
 * harmonic mean it replaces is at most five times its slowest sample: one chunk at 14 kb/s among four
 * at 14–18 Mb/s made it 69 kb/s, and a Fire TV dropped from Nosferatu's own picture to its lowest
 * quality for 75 s (staging, 2026-10-28; bandwidth-sampling §1.1).
 *
 * A choice reads it at a **bound**, a quantile set by what is loaded ahead (boundDeviationsByAheadS):
 * the **confident estimate**, for a choice held over many chunks (the ceiling, the step-up), is
 * e^(centre − z × spread × √(1/n)), and the **next chunk's** bound, for one chunk (the start gate's
 * load), e^(centre − z × spread × √(1 + 1/n)), n the samples it counts, the prior's among them (P4).
 * They replace RobustMPC's discount, the estimate over one plus the largest error of its own recent
 * predictions, whose largest of a few errors was ruled by the slowest sample: one slow chunk of five
 * cut a Fire TV's ceiling from its top quality to its second lowest (staging, 2026-10-28; §1.3).
 * That error (predictionError) still says whether its samples agree, for a climb of several
 * qualities (D352 §2).
 *
 * Its **prior** is the link its device last measured on its network, remembered (RememberedLink): it
 * enters the centre as a point counting as its countsAsSamples, and lends its spread while too few
 * samples are kept to measure one, until two of the playback's own samples lie dropsPriorBeyondSpreads
 * from it on the same side and within dropsPriorWithinSpreads of each other, which drops it, or the
 * playback's own samples begin to leave the window, more than linkSamplesKept of them, which retires
 * it: the window is then the playback's own (bandwidth-sampling P2). There is no estimate before the
 * playback's own first sample, the start's quality being the server's, chosen by the same link; only
 * the next chunk's bound reads the prior alone, for the start gate.
 */
export class LinkEstimate {
  #samples: Kept[] = [];
  #count = 0;
  readonly #prior: RememberedLink | undefined;
  /** When the prior was dropped, by how many samples the playback had then; undefined while not. */
  #priorDroppedAt: number | undefined;

  /** An estimate with nothing measured yet, starting from what its device remembers of the link, if anything. */
  constructor(prior?: RememberedLink) {
    this.#prior =
      prior !== undefined && prior.kbps > 0 && prior.countsAsSamples > 0 ? prior : undefined;
  }

  /**
   * Takes a chunk that measured the link, noting how long the estimate before it expected it to take,
   * its 90% next-chunk bound then, and the bound a choice read with bufferAheadS loaded ahead.
   */
  add(sample: LinkSample, bufferAheadS = 0): void {
    const arrivedMs = Math.max(1, sample.arrivedMs);
    const predictedBps = this.estimateBps();
    // Calibrated against the bound read with nothing loaded, the one whose quantile the log can check
    // (about one sample in ten slower): the bound a choice reads with 20 s loaded is the centre, which
    // half the samples fall below by construction (98 of 241 on a Fire TV's Nosferatu, staging,
    // 2026-10-29), saying nothing of the estimate's calibration.
    const boundBps = this.nextChunkBps(0);
    const msAt = (bps: number | undefined): { readonly ms?: number } =>
      bps === undefined ? {} : { ms: Math.round((sample.bytes * 8 * 1000) / bps) };
    // Expected at the centre, the prior's alone before any sample, so a start's first chunk counts too.
    const centre = this.centre();
    const expected = msAt(centre === undefined ? undefined : Math.exp(centre)).ms;
    const bound = msAt(boundBps).ms;
    const readBound = msAt(this.nextChunkBps(bufferAheadS)).ms;
    this.#samples.push({
      ...sample,
      arrivedMs,
      bps: Math.max(1, (sample.bytes * 8 * 1000) / arrivedMs),
      predictedBps,
      ...(expected === undefined ? {} : { expectedMs: expected }),
      ...(bound === undefined ? {} : { boundMs: bound }),
      ...(readBound === undefined ? {} : { readBoundMs: readBound }),
    });
    this.#samples = this.#samples.slice(-linkSamplesKept);
    this.#count += 1;
    if (this.#isPriorKept() && this.#disagreesWithPrior()) {
      this.#priorDroppedAt = this.#count;
    }
  }

  /**
   * The prior the estimate began from, and whether it still counts: dropped when the playback's own
   * samples disagreed with it (at which sample), retired once its own samples fill the window.
   */
  prior():
    (RememberedLink & { readonly isKept: boolean; readonly droppedAtSample?: number }) | undefined {
    const prior = this.#prior;
    if (prior === undefined) {
      return undefined;
    }
    return {
      ...prior,
      isKept: this.#isPriorKept(),
      ...(this.#priorDroppedAt === undefined ? {} : { droppedAtSample: this.#priorDroppedAt }),
    };
  }

  /** How many samples the estimate counts in all: those kept, and the prior's while it is kept. */
  countsAsSamples(): number {
    return this.#samples.length + (this.#isPriorKept() ? (this.#prior?.countsAsSamples ?? 0) : 0);
  }

  /** How many chunks have measured the link in all, the ones no longer kept with them. */
  count(): number {
    return this.#count;
  }

  /** How many samples the estimate rests on now: at most linkSamplesKept. */
  kept(): number {
    return this.#samples.length;
  }

  /**
   * The spread of the samples' natural logs: their median absolute deviation as a standard
   * deviation, at least logSpreadAtLeast, once spreadFromSamples are kept; assumedLogSpread before.
   */
  spread(): number {
    const logs = this.#logs();
    const median = medianOf(logs);
    if (logs.length < spreadFromSamples || median === undefined) {
      const remembered = this.#isPriorKept() ? this.#prior?.logSpread : undefined;
      return remembered === undefined ? assumedLogSpread : Math.max(logSpreadAtLeast, remembered);
    }
    const deviation = medianOf(logs.map((log) => Math.abs(log - median))) ?? 0;
    return Math.max(logSpreadAtLeast, madToSpread * deviation);
  }

  /**
   * The centre of the samples and of the prior while it is kept, in natural log of bits a second;
   * undefined with neither.
   */
  centre(): number | undefined {
    const points: Weighed[] = this.#logs().map((log) => ({ log, weight: 1 }));
    const prior = this.#prior;
    if (prior !== undefined && this.#isPriorKept()) {
      points.push({ log: Math.log(prior.kbps * 1000), weight: prior.countsAsSamples });
    }
    return points.length === 0 ? undefined : huberCentreOf(points, this.spread());
  }

  /** The estimate, e to the centre, in bits a second; undefined before the playback's own first sample. */
  estimateBps(): number | undefined {
    const centre = this.centre();
    return centre === undefined || this.#samples.length === 0 ? undefined : Math.exp(centre);
  }

  /**
   * The error of the estimate's predictions of the samples kept, each as a share of its sample: the
   * largest, but once trimsErrorFromSamples are kept, the largest after the single most extreme is
   * set aside; undefined while no kept sample had an estimate before it. One sample out
   * of five ruled it before: a Fire TV's 376 KB in 1200 ms among four at 9.7–22.6 Mb/s made it
   * 460%, the confident estimate 1315 kb/s, and the ceiling fell from its top quality to its second
   * lowest (staging, Nosferatu, 2026-10-28).
   */
  predictionError(): number | undefined {
    const errors = this.#samples
      .flatMap((sample) =>
        sample.predictedBps === undefined
          ? []
          : [Math.abs(sample.predictedBps - sample.bps) / Math.max(sample.bps, 1)],
      )
      .sort((first, second) => first - second); // flatMap's own array
    const counted = this.#samples.length >= trimsErrorFromSamples ? errors.slice(0, -1) : errors;
    return counted.at(-1);
  }

  /**
   * The confident estimate, for a choice held over many chunks with bufferAheadS loaded ahead:
   * e^(centre − z × spread × √(1/n)); undefined before the playback's own first sample.
   */
  confidentBps(bufferAheadS: number): number | undefined {
    return this.estimateBps() === undefined ? undefined : this.#boundBps(bufferAheadS, 0);
  }

  /**
   * The next chunk's bound, for one chunk's load with bufferAheadS loaded ahead:
   * e^(centre − z × spread × √(1 + 1/n)), from the prior alone before any sample; undefined with
   * neither.
   */
  nextChunkBps(bufferAheadS: number): number | undefined {
    return this.#boundBps(bufferAheadS, 1);
  }

  /** e^(centre − z × spread × √(chunks + 1/n)) at bufferAheadS loaded ahead; undefined with no centre. */
  #boundBps(bufferAheadS: number, chunks: number): number | undefined {
    const centre = this.centre();
    const counted = this.countsAsSamples();
    if (centre === undefined || counted <= 0) {
      return undefined;
    }
    const deviations = boundDeviationsAt(bufferAheadS);
    return Math.exp(centre - deviations * this.spread() * Math.sqrt(chunks + 1 / counted));
  }

  /** Whether the samples kept agree: their prediction error at most samplesAgreeWithinError. */
  agrees(): boolean {
    const error = this.predictionError();
    return error !== undefined && error <= samplesAgreeWithinError;
  }

  /** The samples kept, oldest first. */
  samples(): readonly LinkSample[] {
    return this.#samples;
  }

  /**
   * The wait a chunk's first byte is expected to take, in ms: the median of the samples kept, as
   * Media3 keeps its own apart (bandwidth-sampling §5.1); 0 before any.
   */
  firstByteMs(): number {
    return medianOf(this.#samples.map((sample) => sample.firstByteMs)) ?? 0;
  }

  /**
   * The estimate each sample kept would leave without it, its prior with it while the prior counts:
   * whether a choice rests on no one sample (P5).
   */
  withoutEach(): LinkEstimate[] {
    const prior = this.prior();
    return this.#samples.map((_left, index) => {
      const without = new LinkEstimate(prior?.isKept === true ? prior : undefined);
      for (const sample of this.#samples.filter((_each, each) => each !== index)) {
        without.add(sample);
      }
      return without;
    });
  }

  /** The natural logs of the samples' speeds, oldest first. */
  #logs(): number[] {
    return this.#samples.map((sample) => Math.log(sample.bps));
  }

  /** Whether the prior counts: there is one, not dropped, and the window not yet the playback's own. */
  #isPriorKept(): boolean {
    return (
      this.#prior !== undefined &&
      this.#priorDroppedAt === undefined &&
      this.#count <= linkSamplesKept
    );
  }

  /**
   * Whether two of the samples kept lie dropsPriorBeyondSpreads from the prior on the same side and
   * within dropsPriorWithinSpreads of each other.
   */
  #disagreesWithPrior(): boolean {
    const prior = this.#prior;
    if (prior === undefined) {
      return false;
    }
    const spread = this.spread();
    const priorLog = Math.log(prior.kbps * 1000);
    const far = this.#logs().filter(
      (log) => Math.abs(log - priorLog) > dropsPriorBeyondSpreads * spread,
    );
    return far.some((first, index) =>
      far
        .slice(index + 1)
        .some(
          (second) =>
            Math.sign(first - priorLog) === Math.sign(second - priorLog) &&
            Math.abs(first - second) <= dropsPriorWithinSpreads * spread,
        ),
    );
  }
}

/** Each video's chunked playback's spread of its link, as its controller last told it. */
const spreads = new WeakMap<object, number>();

/**
 * What keeps the spread of the link of video's chunked playback, the latest replacing the one before,
 * for its play reports, which the server keeps beside the link's speed for its next start's prior
 * (bandwidth-sampling-2026-10-29.md P2).
 */
export function linkSpreadNoter(video: object): (logSpread: number) => void {
  return (logSpread) => {
    spreads.set(video, logSpread);
  };
}

/** The spread of the link video's chunked playback last reported with its speed; undefined for none. */
export function linkSpreadOf(video: object): number | undefined {
  return spreads.get(video);
}

/**
 * The confident estimate at bufferAheadS and its parts in a line, for a note of the controller's
 * choice (D352 §5): "confident 4200 kb/s (estimate 5000, spread 0.31, z 0.84, error 19%, 5 samples,
 * prior 17000 kb/s as 2)"; the prior "dropped at 2" once dropped, nothing once retired; "unmeasured"
 * before any sample.
 */
export function estimateText(estimate: LinkEstimate, bufferAheadS: number): string {
  const estimated = estimate.estimateBps();
  const confident = estimate.confidentBps(bufferAheadS);
  if (estimated === undefined || confident === undefined) {
    return "unmeasured";
  }
  const error = estimate.predictionError();
  const errorText = error === undefined ? "none" : `${String(Math.round(error * 100))}%`;
  const kept = estimate.kept();
  return (
    `confident ${String(Math.round(confident / 1000))} kb/s` +
    ` (estimate ${String(Math.round(estimated / 1000))}, spread ${estimate.spread().toFixed(2)},` +
    ` z ${boundDeviationsAt(bufferAheadS).toFixed(2)}, error ${errorText},` +
    ` ${String(kept)} sample${kept === 1 ? "" : "s"}${priorText(estimate)})`
  );
}

/** The prior in a note: ", prior 17000 kb/s as 2", ", prior dropped at 2", or "" with none or retired. */
function priorText(estimate: LinkEstimate): string {
  const prior = estimate.prior();
  if (prior === undefined) {
    return "";
  }
  if (prior.droppedAtSample !== undefined) {
    return `, prior dropped at ${String(prior.droppedAtSample)}`;
  }
  return prior.isKept
    ? `, prior ${String(Math.round(prior.kbps))} kb/s as ${String(Math.round(prior.countsAsSamples * 10) / 10)}`
    : "";
}

/**
 * A sample in a few words: its quality, bytes, time to arrive and wait for its first byte: "4
 * 812345B 1203ms fb340ms".
 */
export function sampleText(sample: LinkSample): string {
  return (
    `${String(sample.level)} ${String(sample.bytes)}B ${String(Math.round(sample.arrivedMs))}ms` +
    ` fb${String(Math.round(sample.firstByteMs))}ms`
  );
}

/**
 * A sample for its note on the link (P6): its sampleText, and where the estimate before it had one,
 * the time it expected, its 90% next-chunk bound's, the bound's a choice read then, and "below" when
 * it took longer than the 90% bound: "4 812345B 1203ms fb340ms exp900 lo1100 read950 below".
 */
export function sampleCalibrationText(sample: LinkSample): string {
  if (sample.expectedMs === undefined || sample.boundMs === undefined) {
    return sampleText(sample);
  }
  const below = sample.arrivedMs > sample.boundMs ? " below" : "";
  const read = sample.readBoundMs === undefined ? "" : ` read${String(sample.readBoundMs)}`;
  return `${sampleText(sample)} exp${String(sample.expectedMs)} lo${String(sample.boundMs)}${read}${below}`;
}

/**
 * The note on the link of the sample estimate took last (P6): its sampleCalibrationText, and the
 * prior's drop where it dropped it: "sample 3 812345B 1203ms fb340ms exp900 lo1500, prior dropped";
 * undefined before any.
 */
export function lastSampleNote(estimate: LinkEstimate): string | undefined {
  const taken = estimate.samples().at(-1);
  if (taken === undefined) {
    return undefined;
  }
  const isDropping = estimate.prior()?.droppedAtSample === estimate.count();
  return `sample ${sampleCalibrationText(taken)}${isDropping ? ", prior dropped" : ""}`;
}

/** A playback's prior for its note on the link (P6): "prior 17000 kb/s, spread 0.30, as 2 samples". */
export function priorNoteText(prior: RememberedLink): string {
  return (
    `prior ${String(Math.round(prior.kbps))} kb/s, spread ${prior.logSpread?.toFixed(2) ?? "none"},` +
    ` as ${String(Math.round(prior.countsAsSamples * 10) / 10)} samples`
  );
}

/**
 * The samples in a line, oldest first (sampleText): "samples 4 812345B 1203ms fb340ms, 4 ..."; "" for
 * none. Kept short, as it rides in a stall's and a seek's linkSamples, which the server takes up to
 * 300 characters of.
 */
export function samplesText(samples: readonly LinkSample[]): string {
  return samples.length === 0 ? "" : `samples ${samples.map(sampleText).join(", ")}`;
}

/**
 * How many of a video's notes on its link a play report carries at most, and how long each may be,
 * as the server takes them (internal/core/api's ReportInput.LinkNotes); older ones beyond are dropped.
 */
export const linkNotesKept = 20;
export const linkNoteMost = 200;

/** Each video's chunked playback's notes on its link not yet sent, oldest first. */
const linkNotes = new WeakMap<object, string[]>();

/**
 * What keeps the notes of video's chunked playback on its link, each sample with its expected time
 * and bound's, each chunk given up or stuck and what carried it, and its prior (P6), for its next
 * play report, which the server logs.
 */
export function linkNoter(video: object): (note: string) => void {
  return (note) => {
    const kept = linkNotes.get(video) ?? [];
    kept.push(note.slice(0, linkNoteMost));
    linkNotes.set(video, kept.slice(-linkNotesKept));
  };
}

/** The notes of video's chunked playback on its link since they were last taken, oldest first, taken now. */
export function takeLinkNotes(video: object): readonly string[] {
  const kept = linkNotes.get(video) ?? [];
  linkNotes.delete(video);
  return kept;
}
