// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Hls from "hls.js";

import { adaptQuality, lookForStepUpEveryMs, type AdaptingHls } from "./chunked.ts";
import { holdBeforeStepUpMs, stepUpBufferMs } from "./step-up.ts";

/** A stand-in for hls.js: three qualities, the link it measures, and its events. */
class FakeHls implements AdaptingHls {
  readonly levels = [
    { bitrate: 900_000, height: 432 },
    { bitrate: 1_800_000, height: 480 },
    { bitrate: 3_000_000, height: 540 },
  ];

  currentLevel = -1;
  bandwidthEstimate = 0;
  startLevel = -1;
  autoLevelCapping = -1;
  readonly listeners = new Map<string, () => void>();

  on(event: string, listener: () => void): void {
    this.listeners.set(event, listener);
  }

  fire(event: string): void {
    this.listeners.get(event)?.();
  }
}

/** A media element with secondsAhead loaded ahead of its position. */
function mediaAhead(secondsAhead: number): {
  buffered: Pick<TimeRanges, "length" | "start" | "end">;
  currentTime: number;
} {
  return {
    currentTime: 100,
    buffered: { length: 1, start: () => 90, end: () => 100 + secondsAhead },
  };
}

let nowMs = 0;
const monotonicMs = (): number => nowMs;

beforeEach(() => {
  vi.useFakeTimers();
  nowMs = 0;
});

afterEach(() => {
  vi.useRealTimers();
});

describe("adapting a chunked playback's quality (D247 §6, §9)", () => {
  it("starts at the highest quality within the speed the server chose by, and holds it there", () => {
    const hls = new FakeHls();
    adaptQuality(hls, mediaAhead(30), {
      startAtBps: 2_000_000,
      monotonicMs,
    });
    hls.fire(Hls.Events.MANIFEST_PARSED);
    expect(hls.startLevel).toBe(1);
    expect(hls.autoLevelCapping).toBe(1);
  });

  it("starts at the lowest when even it is beyond the speed", () => {
    const hls = new FakeHls();
    adaptQuality(hls, mediaAhead(30), {
      startAtBps: 500_000,
      monotonicMs,
    });
    hls.fire(Hls.Events.MANIFEST_PARSED);
    expect(hls.startLevel).toBe(0);
  });

  it("steps up one quality once the link has held well above it, and holds again after", () => {
    const hls = new FakeHls();
    adaptQuality(hls, mediaAhead(stepUpBufferMs / 1000), {
      startAtBps: 1_000_000,
      monotonicMs,
    });
    hls.fire(Hls.Events.MANIFEST_PARSED);
    hls.currentLevel = 0;
    hls.bandwidthEstimate = 10_000_000;
    nowMs = holdBeforeStepUpMs - lookForStepUpEveryMs;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(hls.autoLevelCapping).toBe(0); // not held long enough yet
    nowMs = holdBeforeStepUpMs;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(hls.autoLevelCapping).toBe(1);
    hls.currentLevel = 1;
    hls.fire(Hls.Events.LEVEL_SWITCHED);
    nowMs += lookForStepUpEveryMs;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(hls.autoLevelCapping).toBe(1); // held again at the new quality
  });
});
