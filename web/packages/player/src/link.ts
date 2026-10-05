// SPDX-License-Identifier: MPL-2.0

/** HTMLMediaElement.NETWORK_LOADING, which a plain page may not define: the element is fetching. */
const networkLoading = 2;

/**
 * The half-lives of the two moving averages of the link's speed, in seconds of loading: the
 * estimate is the lower of them, so it falls fast and rises slowly. hls.js's own (`abrEwmaFastVoD`,
 * `abrEwmaSlowVoD`; slow-links.md §2).
 */
export const fastHalfLifeS = 3;
export const slowHalfLifeS = 9;

/** How many seconds of loading the meter must see before it says anything: Media3's 2 s. */
export const trustAfterLoadingS = 2;

/** How often the meter looks at the buffer. */
export const meterEveryMs = 1000;

/** The part of a video element the meter reads. */
export type MeteredMedia = Pick<
  HTMLMediaElement,
  "buffered" | "currentTime" | "networkState" | "seeking"
>;

/** The speed of a player's link, as it measures it. */
export interface LinkMeter {
  /** The link's speed in kb/s, or undefined until it has seen enough loading to say. */
  readonly estimateKbps: () => number | undefined;
  readonly stop: () => void;
}

/** A moving average weighted by seconds, as hls.js's EWMA: an estimate from zero is scaled up. */
class MovingAverage {
  readonly #alpha: number;
  #estimate = 0;
  #weightS = 0;

  constructor(halfLifeS: number) {
    this.#alpha = Math.exp(Math.log(0.5) / halfLifeS);
  }

  add(weightS: number, value: number): void {
    const alpha = this.#alpha ** weightS;
    this.#estimate = value * (1 - alpha) + alpha * this.#estimate;
    this.#weightS += weightS;
  }

  value(): number {
    return this.#estimate / (1 - this.#alpha ** this.#weightS);
  }
}

/** The buffered range the position is in, in seconds, or undefined outside one. */
function rangeAt(
  media: MeteredMedia,
): { readonly startS: number; readonly endS: number } | undefined {
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index++) {
    if (buffered.start(index) <= currentTime && currentTime <= buffered.end(index)) {
      return { startS: buffered.start(index), endS: buffered.end(index) };
    }
  }
  return undefined;
}

/**
 * Measures the link of a player playing a file as it is, whose bytes the page cannot count: while
 * the element loads, each second of film its buffer grows by is fileBitRateKbps of the link
 * (D247 §1). Only time spent loading counts, never the pauses between loads, which would read the
 * link as slower than it is (slow-links.md §2); a seek starts the measurement of a new stretch.
 */
export function meterBufferGrowth(
  media: MeteredMedia,
  options: { readonly fileBitRateKbps: number; readonly monotonicMs: () => number },
): LinkMeter {
  const fast = new MovingAverage(fastHalfLifeS);
  const slow = new MovingAverage(slowHalfLifeS);
  let loadedS = 0;
  let last: { readonly atMs: number; readonly startS: number; readonly endS: number } | undefined;
  const look = (): void => {
    const range = rangeAt(media);
    const nowMs = options.monotonicMs();
    if (media.networkState !== networkLoading || media.seeking || range === undefined) {
      last = undefined;
      return;
    }
    const { startS, endS } = range;
    // The same stretch, grown: a seek elsewhere is a new stretch, measured from its next look.
    if (last?.startS === startS && endS >= last.endS) {
      const weightS = (nowMs - last.atMs) / 1000;
      if (weightS > 0) {
        const kbps = ((endS - last.endS) * options.fileBitRateKbps) / weightS;
        fast.add(weightS, kbps);
        slow.add(weightS, kbps);
        loadedS += weightS;
      }
    }
    last = { atMs: nowMs, startS, endS };
  };
  const timer = setInterval(look, meterEveryMs);
  look();
  return {
    estimateKbps: () =>
      options.fileBitRateKbps > 0 && loadedS >= trustAfterLoadingS
        ? Math.min(fast.value(), slow.value())
        : undefined,
    stop: () => {
      clearInterval(timer);
    },
  };
}
