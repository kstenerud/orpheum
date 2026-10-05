// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { meterBufferGrowth, meterEveryMs, trustAfterLoadingS } from "./link.ts";

/** A stand-in element: one buffered range a test grows, and whether it is loading. */
class FakeMedia {
  currentTime = 0;
  // eslint-disable-next-line @typescript-eslint/naming-convention -- HTMLMediaElement's own name, which this stands in for
  seeking = false;
  networkState = 2; // NETWORK_LOADING
  range: [number, number] = [0, 0];

  get buffered(): Pick<TimeRanges, "length" | "start" | "end"> {
    const [startS, endS] = this.range;
    return { length: 1, start: () => startS, end: () => endS };
  }

  /** One meter look later, with the buffer grown by grownS seconds of film. */
  loadFor(grownS: number): void {
    this.range = [this.range[0], this.range[1] + grownS];
    nowMs += meterEveryMs;
    vi.advanceTimersByTime(meterEveryMs);
  }
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

describe("measuring a player's link from its buffer (D247 §1)", () => {
  it("reads the link from how fast the buffer grows, times the file's bit rate", () => {
    const media = new FakeMedia();
    const meter = meterBufferGrowth(media, {
      fileBitRateKbps: 2400,
      monotonicMs,
    });
    media.loadFor(0.5);
    expect(meter.estimateKbps()).toBeUndefined(); // too little seen to say
    for (let second = 1; second < trustAfterLoadingS + 3; second++) {
      media.loadFor(0.5);
    }
    expect(meter.estimateKbps()).toBeCloseTo(1200);
  });

  it("counts only time spent loading, never the pauses between loads", () => {
    const media = new FakeMedia();
    const meter = meterBufferGrowth(media, {
      fileBitRateKbps: 2000,
      monotonicMs,
    });
    for (let second = 0; second < 4; second++) {
      media.loadFor(2);
    }
    media.networkState = 1; // NETWORK_IDLE: the buffer is full, the browser waits
    for (let second = 0; second < 30; second++) {
      media.loadFor(0);
    }
    media.networkState = 2;
    media.loadFor(2);
    media.loadFor(2);
    expect(meter.estimateKbps()).toBeCloseTo(4000);
  });

  it("takes a seek to another stretch for a new start, not for a burst of speed", () => {
    const media = new FakeMedia();
    const meter = meterBufferGrowth(media, {
      fileBitRateKbps: 1000,
      monotonicMs,
    });
    for (let second = 0; second < 4; second++) {
      media.loadFor(1);
    }
    media.range = [600, 601];
    media.currentTime = 600;
    media.loadFor(0);
    media.loadFor(1);
    expect(meter.estimateKbps()).toBeCloseTo(1000);
  });

  it("falls fast when the link slows, and rises slowly when it recovers", () => {
    const media = new FakeMedia();
    const meter = meterBufferGrowth(media, {
      fileBitRateKbps: 1000,
      monotonicMs,
    });
    for (let second = 0; second < 20; second++) {
      media.loadFor(4);
    }
    media.loadFor(1);
    media.loadFor(1);
    const fallen = meter.estimateKbps() ?? 0;
    expect(fallen).toBeLessThan(3000); // a quarter down in two seconds
    for (let second = 0; second < 2; second++) {
      media.loadFor(4);
    }
    const recovering = meter.estimateKbps() ?? 0;
    expect(recovering).toBeGreaterThan(fallen);
    expect(recovering).toBeLessThan(3500); // still short of 4000 two seconds on
  });

  it("says nothing of a file whose bit rate is not known", () => {
    const media = new FakeMedia();
    const meter = meterBufferGrowth(media, {
      fileBitRateKbps: 0,
      monotonicMs,
    });
    for (let second = 0; second < 5; second++) {
      media.loadFor(1);
    }
    expect(meter.estimateKbps()).toBeUndefined();
    meter.stop();
  });
});
