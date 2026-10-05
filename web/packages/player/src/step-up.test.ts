// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  holdBeforeStepUpMs,
  nextCap,
  stepUpBufferMs,
  stepUpHeadroom,
  type StepUpState,
} from "./step-up.ts";

/** A playback at 1.2 Mb/s, the second of three qualities, held long enough, the link ample. */
const ready: StepUpState = {
  levels: [
    { averageBitrate: 900_000 },
    { averageBitrate: 1_800_000 },
    { averageBitrate: 3_000_000 },
  ],
  cap: 1,
  playing: 1,
  estimateBps: stepUpHeadroom * 3_000_000,
  bufferedAheadMs: stepUpBufferMs,
  sinceChangeMs: holdBeforeStepUpMs,
};

describe("stepping back up a quality (D247 §9)", () => {
  it("steps up one quality once the link has held well above it for minutes", () => {
    expect(nextCap(ready)).toBe(2);
    expect(nextCap({ ...ready, cap: 0, playing: 0 })).toBe(1); // one at a time
  });

  it("stays while any condition is short of it", () => {
    expect(nextCap({ ...ready, estimateBps: stepUpHeadroom * 3_000_000 - 1 })).toBe(1);
    expect(nextCap({ ...ready, sinceChangeMs: holdBeforeStepUpMs - 1 })).toBe(1);
    expect(nextCap({ ...ready, bufferedAheadMs: stepUpBufferMs - 1 })).toBe(1);
    expect(nextCap({ ...ready, playing: 0 })).toBe(1); // hls.js stepped down beneath the cap
  });

  it("never steps past the top", () => {
    expect(nextCap({ ...ready, cap: 2, playing: 2 })).toBe(2);
  });
});
