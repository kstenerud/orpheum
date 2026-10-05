// SPDX-License-Identifier: MPL-2.0

/**
 * How long a player stays at a quality before it may step up: the link must have held for minutes,
 * not seconds (D247 §9). Our choice, to be measured: Media3 waits for 10 s of buffer, Shaka 8 s
 * between switches (slow-links.md §2), and a stall costs far more than a softer picture.
 */
export const holdBeforeStepUpMs = 120_000;

/** The speed, as a multiple of the next quality's rate, that a step up needs (D247 §9). */
export const stepUpHeadroom = 1.5;

/** How much must be loaded ahead before stepping up: a step up spends some of it loading anew. */
export const stepUpBufferMs = 20_000;

/** A quality hls.js may play: its peak bit rate, as its playlist says. */
export interface Level {
  readonly bitrate: number;
}

/** Where a chunked playback is, as the step-up looks at it. */
export interface StepUpState {
  /** The qualities, lowest first, as hls.js orders them. */
  readonly levels: readonly Level[];
  /** The highest quality hls.js may choose now. */
  readonly cap: number;
  /** The quality playing. */
  readonly playing: number;
  /** The link's speed, bits a second, as hls.js measures each chunk's download. */
  readonly estimateBps: number;
  readonly bufferedAheadMs: number;
  /** How long it has been since the cap or the quality playing last changed. */
  readonly sinceChangeMs: number;
}

/**
 * The highest quality hls.js may choose next: one above the cap, once the player has played at the
 * cap for holdBeforeStepUpMs with stepUpBufferMs loaded ahead and a link stepUpHeadroom times the
 * next quality's rate; else the cap as it is. One quality at a time, never past the top (D247 §9).
 * Stepping down is hls.js's own, at once, on its measurement.
 */
export function nextCap(state: StepUpState): number {
  const above = state.cap + 1;
  const next = state.levels[above];
  if (
    next === undefined ||
    state.playing < state.cap ||
    state.sinceChangeMs < holdBeforeStepUpMs ||
    state.bufferedAheadMs < stepUpBufferMs ||
    state.estimateBps < stepUpHeadroom * next.bitrate
  ) {
    return state.cap;
  }
  return above;
}
