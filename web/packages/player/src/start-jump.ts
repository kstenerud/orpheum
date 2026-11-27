// SPDX-License-Identifier: MPL-2.0

import { highestCarried, type Level } from "./step-up.ts";

/**
 * How much must be loaded ahead, in seconds, before a start its node's speed held jumps to the
 * quality its link carries: "about ten seconds" (start-quality-by-node-speed.md §4, the owner's
 * answer 2, 2026-11-27).
 */
export const jumpsWithAheadS = 10;

/** Where a playback is, as its jump looks at it. */
export interface JumpState {
  /** The qualities, lowest first, as hls.js orders them. */
  readonly levels: readonly Level[];
  /** The ceiling now, which a jump raises and a jump that failed falls back to. */
  readonly ceiling: number;
  /** The highest the ceiling may rise to: below a quality unplayable here and the data cap. */
  readonly top: number;
  /** What is loaded ahead, in seconds. */
  readonly aheadS: number;
  /** The link's confident estimate, bits a second; undefined before it has one (LinkEstimate). */
  readonly confidentBps: number | undefined;
  /** The headroom a step up asks at the playback's steadiness (D352 §3). */
  readonly headroom: number;
}

/**
 * A start its node's speed held below its link (start-quality-by-node-speed.md §2) and how it gets
 * up (§4, S4; the owner's answer 2, 2026-11-27: "We try jumping to the link's quality. If that
 * fails, we drop back down and climb slowly."): once jumpsWithAheadS is loaded ahead and the link
 * has an estimate, it jumps the ceiling at once to the highest quality the step-up would call the
 * link's (highestCarried), its holds skipped; once a playback, the jump spent where the link carries
 * nothing higher. A stall or a give-up after the jump drops the ceiling back to where the jump was
 * made from, once, and the step-up from then climbs a quality at a time by its own holds. A start
 * not held never jumps and climbs as it would.
 */
export class StartJump {
  #phase: "notHeld" | "waiting" | "jumped" | "fellBack" | "spent";
  /** The ceiling the jump was made from, which a jump that failed falls back to. */
  #fromLevel = 0;

  constructor(isHeldByNodeSpeed: boolean) {
    this.#phase = isHeldByNodeSpeed ? "waiting" : "notHeld";
  }

  /** Whether the start was held by its node's speed, and so starts where it was offered. */
  isHeld(): boolean {
    return this.#phase !== "notHeld";
  }

  /** The ceiling to jump to now, once; undefined for none. */
  jumpTo(state: JumpState): number | undefined {
    if (
      this.#phase !== "waiting" ||
      state.aheadS < jumpsWithAheadS ||
      state.confidentBps === undefined
    ) {
      return undefined;
    }
    const carried = highestCarried({
      levels: state.levels,
      cap: state.ceiling,
      estimateBps: state.confidentBps,
      headroom: state.headroom,
    });
    const to = Math.min(carried, state.top);
    if (to <= state.ceiling) {
      this.#phase = "spent";
      return undefined;
    }
    this.#phase = "jumped";
    this.#fromLevel = state.ceiling;
    return to;
  }

  /** A stall or a give-up: the ceiling to fall back to after the jump, once; undefined for none. */
  fellBack(): number | undefined {
    if (this.#phase !== "jumped") {
      return undefined;
    }
    this.#phase = "fellBack";
    return this.#fromLevel;
  }

  /** Whether the step-up climbs a quality at a time: after a jump that failed. */
  climbsOneAtATime(): boolean {
    return this.#phase === "fellBack";
  }
}
