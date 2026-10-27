// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { nextCap, stepUpBufferMs, stepUpHeadroom, type StepUpState } from "./step-up.ts";

/** A ladder's rungs (0.7 → 1.2 → 2 → 3 Mb/s): a playback on the lowest, held long enough, the link ample for the top. */
const ready: StepUpState = {
  levels: [
    { averageBitrate: 700_000 },
    { averageBitrate: 1_200_000 },
    { averageBitrate: 2_000_000 },
    { averageBitrate: 3_000_000 },
  ],
  cap: 0,
  playing: 0,
  estimateBps: stepUpHeadroom * 3_000_000,
  bufferedAheadMs: stepUpBufferMs,
  sinceChangeMs: 30_000,
  holdMs: 30_000,
  headroom: stepUpHeadroom,
  isOneAtATime: false,
};

describe("stepping back up (D247 §9, D352 §2)", () => {
  it("climbs to the highest quality consistent samples carry", () => {
    expect(nextCap(ready)).toBe(3);
    expect(nextCap({ ...ready, estimateBps: stepUpHeadroom * 2_000_000 })).toBe(2);
    expect(nextCap({ ...ready, estimateBps: stepUpHeadroom * 3_000_000 - 1 })).toBe(2);
  });

  it("climbs one quality when its samples disagree", () => {
    expect(nextCap({ ...ready, isOneAtATime: true })).toBe(1);
    expect(nextCap({ ...ready, cap: 1, playing: 1, isOneAtATime: true })).toBe(2);
  });

  it("asks the headroom it is given of each quality it climbs to", () => {
    expect(nextCap({ ...ready, headroom: 2 })).toBe(2); // 2 × 3 Mb/s is more than 4.5
    expect(nextCap({ ...ready, headroom: 7 })).toBe(0); // 7 × 1.2 Mb/s is more than 4.5
  });

  it("stays while any condition is short of it", () => {
    expect(nextCap({ ...ready, estimateBps: stepUpHeadroom * 1_200_000 - 1 })).toBe(0);
    expect(nextCap({ ...ready, sinceChangeMs: ready.holdMs - 1 })).toBe(0);
    expect(nextCap({ ...ready, bufferedAheadMs: stepUpBufferMs - 1 })).toBe(0);
    expect(nextCap({ ...ready, cap: 1, playing: 0 })).toBe(1); // hls.js stepped down beneath the cap
  });

  it("never steps past the top", () => {
    expect(nextCap({ ...ready, cap: 3, playing: 3, estimateBps: 1e9 })).toBe(3);
  });
});
