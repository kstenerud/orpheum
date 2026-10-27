// SPDX-License-Identifier: MPL-2.0

/**
 * The speed, as a multiple of a quality's rate, that a step up to it needs at the steadiness levels
 * 0 and 1 (D247 §9, D352 §3), and below which a stall at the lowest quality is beyond the link
 * (D247 §8, beyondTheLink). Steadier levels ask more (steadiness.ts).
 */
export const stepUpHeadroom = 1.5;

/** How much must be loaded ahead before stepping up: a step up spends some of it loading anew. */
export const stepUpBufferMs = 20_000;

/**
 * A quality the player may play: its average bit rate, as its playlist's AVERAGE-BANDWIDTH says, the
 * sound's included, which the step-up weighs against the link.
 */
export interface Level {
  readonly averageBitrate: number;
}

/** Where a chunked playback is, as the step-up looks at it. */
export interface StepUpState {
  /** The qualities, lowest first, as hls.js orders them. */
  readonly levels: readonly Level[];
  /** The highest quality the player's bitrate controller may choose now (D265). */
  readonly cap: number;
  /** The quality playing. */
  readonly playing: number;
  /** The link's confident estimate, bits a second (D352 §2, LinkEstimate). */
  readonly estimateBps: number;
  readonly bufferedAheadMs: number;
  /** How long it has been since the cap or the quality playing last changed. */
  readonly sinceChangeMs: number;
  /** The hold before a step up, and the headroom it needs, at the playback's steadiness (D352 §3). */
  readonly holdMs: number;
  readonly headroom: number;
  /** Whether it climbs one quality at a time: when its samples disagree, or its link has swung or flaked. */
  readonly isOneAtATime: boolean;
}

/**
 * The highest quality the bitrate controller may choose next (D352 §2): once the player has played
 * at the cap for holdMs with stepUpBufferMs loaded ahead, the highest quality whose average rate
 * times headroom the confident estimate carries, or only the one above the cap when isOneAtATime;
 * else the cap as it is, never past the top. Stepping down is the bitrate controller's own (abr.ts,
 * D265 §3–§4), at once, on its measurement.
 */
export function nextCap(state: StepUpState): number {
  const { levels, cap } = state;
  if (
    state.playing < cap ||
    state.sinceChangeMs < state.holdMs ||
    state.bufferedAheadMs < stepUpBufferMs
  ) {
    return cap;
  }
  const isCarried = (index: number): boolean => {
    const level = levels[index];
    return level !== undefined && state.headroom * level.averageBitrate <= state.estimateBps;
  };
  if (!isCarried(cap + 1)) {
    return cap;
  }
  if (state.isOneAtATime) {
    return cap + 1;
  }
  let highest = cap + 1;
  while (isCarried(highest + 1)) {
    highest += 1;
  }
  return highest;
}
