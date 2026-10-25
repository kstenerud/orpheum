// SPDX-License-Identifier: MPL-2.0

/**
 * The lab pages' measures over time (plans/tv-device-tests.md): a key's latency to focus painted,
 * summed up against D190's target, and the gaps in a ticker that say a page was frozen.
 */

/**
 * D190's target from a key to focus moved on screen, in milliseconds: the input probe says whether
 * a device meets it. A justified constant, D190's own figure.
 */
export const focusTargetMs = 250;

/** What a run of latencies came to. */
export interface LatencySummary {
  readonly count: number;
  readonly medianMs: number | undefined;
  readonly worstMs: number | undefined;
  /** Whether the median and the worst are within focusTargetMs; undefined with none measured. */
  readonly isWithinTarget: boolean | undefined;
}

/** The median, the worst and whether they meet the target, of latencies in milliseconds. */
export function latencySummary(latenciesMs: readonly number[]): LatencySummary {
  if (latenciesMs.length === 0) {
    return { count: 0, medianMs: undefined, worstMs: undefined, isWithinTarget: undefined };
  }
  const sorted = latenciesMs.toSorted((first, second) => first - second);
  const middle = Math.floor(sorted.length / 2);
  const median =
    sorted.length % 2 === 1
      ? (sorted[middle] ?? 0)
      : ((sorted[middle - 1] ?? 0) + (sorted[middle] ?? 0)) / 2;
  const worst = sorted.at(-1) ?? 0;
  return {
    count: sorted.length,
    medianMs: Math.round(median),
    worstMs: Math.round(worst),
    isWithinTarget: worst <= focusTargetMs,
  };
}

/** A stretch in which a ticker that should tick each second did not: the page was frozen. */
export interface Gap {
  /** When the tick before it was, in the page's milliseconds since the probe started. */
  readonly afterMs: number;
  readonly lastedMs: number;
}

/**
 * How long a 1 s ticker may go without ticking before the page counts as frozen, in milliseconds:
 * twice its tick, so a tick merely late under load is not one. A justified constant, the brief's
 * (review §2's probe).
 */
export const frozenAfterMs = 2000;

/**
 * Finds the gaps in a ticker: tick each tick's time, and it says the gap that ended there, if the
 * time since the last was over frozenAfterMs. Times are milliseconds on one clock.
 */
export class GapFinder {
  private lastAtMs: number | undefined;
  private readonly startedAtMs: number;
  readonly gaps: Gap[] = [];

  constructor(startedAtMs: number) {
    this.startedAtMs = startedAtMs;
    this.lastAtMs = startedAtMs;
  }

  /** Notes a tick at atMs; returns the gap that ended there, or undefined. */
  tick(atMs: number): Gap | undefined {
    const last = this.lastAtMs ?? atMs;
    this.lastAtMs = atMs;
    const lastedMs = atMs - last;
    if (lastedMs <= frozenAfterMs) {
      return undefined;
    }
    const gap = { afterMs: Math.round(last - this.startedAtMs), lastedMs: Math.round(lastedMs) };
    this.gaps.push(gap);
    return gap;
  }

  /** The longest gap so far, in milliseconds; 0 with none. */
  longestMs(): number {
    return this.gaps.reduce((longest, gap) => Math.max(longest, gap.lastedMs), 0);
  }
}

/** A duration in milliseconds as h:mm:ss, for the pages' elapsed time. */
export function elapsedText(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = seconds % 60;
  return `${String(hours)}:${String(minutes).padStart(2, "0")}:${String(rest).padStart(2, "0")}`;
}

/**
 * An event's time on performance.now()'s clock, as its timeStamp gives it where the browser keeps it
 * on that clock; an old WebView gives the time since 1970, or nothing, and then the time it is
 * handled, nowMs, stands in, a little late.
 */
export function eventTimeMs(timeStamp: number, nowMs: number): number {
  const isOnThePagesClock = timeStamp > 0 && timeStamp <= nowMs + 1 && nowMs - timeStamp < 10_000;
  return isOnThePagesClock ? timeStamp : nowMs;
}
