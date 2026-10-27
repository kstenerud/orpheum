// SPDX-License-Identifier: MPL-2.0

/**
 * How steady a chunked playback's link has been, and so how cautiously it climbs (D352 §3, §4): a
 * climb that falls back, a swing, or a link that keeps dropping chunks raises its level, and each
 * level holds longer before a step up and asks more headroom of it.
 */

import { stepUpHeadroom } from "./step-up.ts";

/** The steadiest level: a link that has swung or dropped three times is held as long as it gets. */
export const highestSteadiness = 3;

/**
 * The hold before a step up at each level, in ms: 30 s at 0, five to eight chunks of evidence where
 * Shaka waits 8 s between switches and Media3 10 s of buffer, doubling with each level, since each
 * swing says the last hold was too short by an unknown factor; 120 s at 2 is D247 §9's hold.
 */
export const holdBeforeStepUpMsAt: readonly number[] = [30_000, 60_000, 120_000, 240_000];

/** The headroom a step up needs at each level: the speed, as a multiple of the quality's rate. */
export const stepUpHeadroomAt: readonly number[] = [stepUpHeadroom, stepUpHeadroom, 1.75, 2];

/** A step down within this many holds after a step up is a swing: the climb did not hold. */
export const swingWithinHolds = 2;

/** How long with no swing and no drop relaxes the level by one. */
export const relaxEveryMs = 600_000;

/** How long after a drop a second one raises the level, as each after it within that time does. */
export const dropsCountWithinMs = 600_000;

/**
 * A playback's steadiness level, at its start the one its device last ended at on its network, as
 * the server remembers it, 0 for none (D352 §3–§4, Q2). Each method that can change the level
 * returns a note of the change, with what raised or relaxed it, or undefined when it changed nothing.
 */
export class Steadiness {
  #level: number;
  /** Since when there has been no swing and no drop: the level relaxes from here. */
  #calmSinceMs: number;
  /** The last step up not yet judged a swing, and the hold it was taken after. */
  #steppedUp: { readonly atMs: number; readonly holdMs: number } | undefined;
  /** The first drop of the drops counting toward a raise. */
  #firstDropAtMs: number | undefined;

  /** startLevel is where it starts, kept within 0 and highestSteadiness; it relaxes from nowMs. */
  constructor(nowMs: number, startLevel = 0) {
    this.#level = Math.min(highestSteadiness, Math.max(0, Math.round(startLevel)));
    this.#calmSinceMs = nowMs;
  }

  level(): number {
    return this.#level;
  }

  /** The hold before a step up at the level. */
  holdMs(): number {
    return holdBeforeStepUpMsAt[this.#level] ?? holdBeforeStepUpMsAt.at(-1) ?? 0;
  }

  /** The headroom a step up needs at the level. */
  headroom(): number {
    return stepUpHeadroomAt[this.#level] ?? stepUpHeadroomAt.at(-1) ?? 1;
  }

  /** Whether a step up climbs one quality at a time: at level 1 and above. */
  climbsOneAtATime(): boolean {
    return this.#level >= 1;
  }

  /** The ceiling rose. */
  steppedUp(nowMs: number): void {
    this.#steppedUp = { atMs: nowMs, holdMs: this.holdMs() };
  }

  /** The ceiling fell by the link: a swing if it came soon after a step up. */
  steppedDown(nowMs: number): string | undefined {
    const swing = this.#swing(nowMs);
    if (swing === undefined) {
      return undefined;
    }
    this.#calmSinceMs = nowMs;
    return this.#raise(swing);
  }

  /**
   * A chunk was given up or the video stalled: a swing if it came soon after a step up, and a
   * second drop within dropsCountWithinMs of the first raises the level too, each one event raising
   * it by one at most.
   */
  dropped(nowMs: number): string | undefined {
    const swing = this.#swing(nowMs);
    const firstAtMs = this.#firstDropAtMs;
    const isRepeat = firstAtMs !== undefined && nowMs - firstAtMs <= dropsCountWithinMs;
    if (!isRepeat) {
      this.#firstDropAtMs = nowMs;
    }
    this.#calmSinceMs = nowMs;
    if (swing !== undefined) {
      return this.#raise(swing);
    }
    return isRepeat
      ? this.#raise(
          `a drop ${String(Math.round((nowMs - firstAtMs) / 1000))} s after the first within ${String(dropsCountWithinMs / 60_000)} min`,
        )
      : undefined;
  }

  /** Relaxes one level for each relaxEveryMs since the last swing or drop. */
  relax(nowMs: number): string | undefined {
    const from = this.#level;
    while (this.#level > 0 && nowMs - this.#calmSinceMs >= relaxEveryMs) {
      this.#level -= 1;
      this.#calmSinceMs += relaxEveryMs;
    }
    return this.#level === from
      ? undefined
      : `steadiness ${String(from)} to ${String(this.#level)}: ${String(((from - this.#level) * relaxEveryMs) / 60_000)} min with no swing and no drop`;
  }

  /** Why a step down now is a swing, ending the step up it follows; undefined when it is not one. */
  #swing(nowMs: number): string | undefined {
    const up = this.#steppedUp;
    if (up === undefined || nowMs - up.atMs > swingWithinHolds * up.holdMs) {
      return undefined;
    }
    this.#steppedUp = undefined;
    return `a swing, ${String(Math.round((nowMs - up.atMs) / 1000))} s after a step up`;
  }

  #raise(why: string): string | undefined {
    const from = this.#level;
    this.#level = Math.min(highestSteadiness, from + 1);
    return this.#level === from
      ? undefined
      : `steadiness ${String(from)} to ${String(this.#level)}: ${why}`;
  }
}

/** Each video's chunked playback's steadiness level, as its controller last told it. */
const levels = new WeakMap<object, number>();

/**
 * What keeps the steadiness level of video's chunked playback, the latest replacing the one before,
 * for its play reports, which the server keeps beside the link's speed (D352 Q2).
 */
export function steadinessNoter(video: object): (level: number) => void {
  return (level) => {
    levels.set(video, level);
  };
}

/** The steadiness level video's chunked playback last stood at; undefined for a video with none. */
export function steadinessOf(video: object): number | undefined {
  return levels.get(video);
}
