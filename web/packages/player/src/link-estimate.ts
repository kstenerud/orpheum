// SPDX-License-Identifier: MPL-2.0

/**
 * The chunked player's own estimate of its link (D352 §2): the last few chunks that measured it,
 * each counted once whatever its length, their harmonic mean, and how far to trust it.
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

/** One chunk that measured the link: its quality, its bytes, how long they took to arrive, and its first byte's wait. */
export interface LinkSample {
  readonly level: number;
  readonly bytes: number;
  /** From its first byte to its last, in ms, at least 1 (arrivalMs). */
  readonly arrivedMs: number;
  /** From its ask to its first byte, in ms. */
  readonly firstByteMs: number;
}

/** A sample as the estimate keeps it: its speed, and the estimate that stood before it came. */
interface Kept extends LinkSample {
  readonly bps: number;
  readonly predictedBps: number | undefined;
}

/** The harmonic mean of speeds, in bits a second; undefined for none. */
export function harmonicMean(speedsBps: readonly number[]): number | undefined {
  if (speedsBps.length === 0) {
    return undefined;
  }
  const inverse = speedsBps.reduce((sum, bps) => sum + 1 / Math.max(bps, 1), 0);
  return speedsBps.length / inverse;
}

/**
 * The link as its last linkSamplesKept chunks measured it. Its **estimate** is their harmonic mean,
 * which every choice of quality reads (D352 §2): pessimistic about a slow sample, and rid of it
 * linkSamplesKept chunks later, where hls.js's, weighted by each chunk's seconds, held one slow first
 * sample for minutes (the research, §1.4). Its **confident estimate** is the estimate divided by one
 * plus the largest error of its own predictions over those samples, how far each fell from the
 * estimate that stood before it, as a share of the sample (RobustMPC's discount): a link whose
 * chunks vary is targeted near its slowest (§4).
 */
export class LinkEstimate {
  #samples: Kept[] = [];
  #count = 0;

  /** Takes a chunk that measured the link. */
  add(sample: LinkSample): void {
    const arrivedMs = Math.max(1, sample.arrivedMs);
    this.#samples.push({
      ...sample,
      arrivedMs,
      bps: (sample.bytes * 8 * 1000) / arrivedMs,
      predictedBps: this.estimateBps(),
    });
    this.#samples = this.#samples.slice(-linkSamplesKept);
    this.#count += 1;
  }

  /** How many chunks have measured the link in all, the ones no longer kept with them. */
  count(): number {
    return this.#count;
  }

  /** How many samples the estimate rests on now: at most linkSamplesKept. */
  kept(): number {
    return this.#samples.length;
  }

  /** The harmonic mean of the samples kept, bits a second; undefined before any. */
  estimateBps(): number | undefined {
    return harmonicMean(this.#samples.map((sample) => sample.bps));
  }

  /**
   * The largest error of the estimate's predictions of the samples kept, as a share of each
   * sample; undefined while no kept sample had an estimate before it.
   */
  largestError(): number | undefined {
    const errors = this.#samples.flatMap((sample) =>
      sample.predictedBps === undefined
        ? []
        : [Math.abs(sample.predictedBps - sample.bps) / Math.max(sample.bps, 1)],
    );
    return errors.length === 0 ? undefined : Math.max(...errors);
  }

  /** The estimate divided by one plus its largest error; the estimate itself while there is no error to read. */
  confidentBps(): number | undefined {
    const estimate = this.estimateBps();
    return estimate === undefined ? undefined : estimate / (1 + (this.largestError() ?? 0));
  }

  /** Whether the samples kept agree: their largest prediction error at most samplesAgreeWithinError. */
  agrees(): boolean {
    const error = this.largestError();
    return error !== undefined && error <= samplesAgreeWithinError;
  }

  /** The samples kept, oldest first. */
  samples(): readonly LinkSample[] {
    return this.#samples;
  }
}

/**
 * The confident estimate and its parts in a line, for a note of the controller's choice (D352 §5):
 * "confident 4200 kb/s (mean 5000, error 19%, 5 samples)"; "unmeasured" before any sample.
 */
export function estimateText(estimate: LinkEstimate): string {
  const mean = estimate.estimateBps();
  const confident = estimate.confidentBps();
  if (mean === undefined || confident === undefined) {
    return "unmeasured";
  }
  const error = estimate.largestError();
  const errorText = error === undefined ? "none" : `${String(Math.round(error * 100))}%`;
  const kept = estimate.kept();
  return (
    `confident ${String(Math.round(confident / 1000))} kb/s` +
    ` (mean ${String(Math.round(mean / 1000))}, error ${errorText},` +
    ` ${String(kept)} sample${kept === 1 ? "" : "s"})`
  );
}

/**
 * The samples in a line, oldest first, each its quality, bytes, time to arrive and wait for its
 * first byte: "samples 4 812345B 1203ms fb340ms, 4 ..."; "" for none. Kept short, as it rides in a
 * stall's and a seek's linkSamples, which the server takes up to 300 characters of.
 */
export function samplesText(samples: readonly LinkSample[]): string {
  if (samples.length === 0) {
    return "";
  }
  const each = samples.map(
    (sample) =>
      `${String(sample.level)} ${String(sample.bytes)}B ${String(Math.round(sample.arrivedMs))}ms` +
      ` fb${String(Math.round(sample.firstByteMs))}ms`,
  );
  return `samples ${each.join(", ")}`;
}
