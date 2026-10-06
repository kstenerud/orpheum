// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Hls from "hls.js";

import { adaptQuality, beyondTheLink, lookForStepUpEveryMs, type AdaptingHls } from "./chunked.ts";
import { holdBeforeStepUpMs, stepUpBufferMs } from "./step-up.ts";

/** A stand-in for hls.js: three qualities, the link it measures, and its events. */
class FakeHls implements AdaptingHls {
  readonly levels = [
    { bitrate: 1_242_000, averageBitrate: 892_000, height: 432 },
    { bitrate: 1_992_000, averageBitrate: 1_392_000, height: 480 },
    { bitrate: 4_692_000, averageBitrate: 3_192_000, height: 720 },
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

describe("whether a stall is beyond the link (D247 §8)", () => {
  /** hls.js playing level, on a link measured at linkBps (0: not yet measured). */
  function playingAt(level: number, linkBps: number): FakeHls {
    const hls = new FakeHls();
    hls.currentLevel = level;
    hls.bandwidthEstimate = linkBps;
    return hls;
  }

  it("is, at the lowest quality on a link measured too slow for it, with both numbers", () => {
    expect(beyondTheLink(playingAt(0, 1_000_000))).toEqual({ linkKbps: 1000, lowestKbps: 892 });
  });

  it("is, at the lowest quality on a link not yet measured", () => {
    expect(beyondTheLink(playingAt(0, 0))).toEqual({ linkKbps: undefined, lowestKbps: 892 });
  });

  it("is not, at the lowest quality on a link measured well above it: something else stalled it", () => {
    // staging, 2026-10-06: a 98 Mbit/s link was told it could not keep up with 892 kbit/s
    expect(beyondTheLink(playingAt(0, 98_348_000))).toBeUndefined();
    expect(beyondTheLink(playingAt(0, 1_338_000))).toBeUndefined(); // 1.5 times the lowest
  });

  it("is not, above the lowest quality, where a lower one answers it", () => {
    expect(beyondTheLink(playingAt(1, 500_000))).toBeUndefined();
  });
});
