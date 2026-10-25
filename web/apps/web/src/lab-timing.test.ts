// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  elapsedText,
  eventTimeMs,
  focusTargetMs,
  frozenAfterMs,
  GapFinder,
  latencySummary,
} from "./lab-timing.ts";

describe("a run of key-to-focus latencies", () => {
  it("is its median and its worst, within D190's 250 ms only while the worst is", () => {
    expect(latencySummary([30, 10, 20])).toEqual({
      count: 3,
      medianMs: 20,
      worstMs: 30,
      isWithinTarget: true,
    });
    expect(latencySummary([10, 20, 40, 300])).toMatchObject({ medianMs: 30, worstMs: 300 });
    expect(latencySummary([10, focusTargetMs + 1]).isWithinTarget).toBe(false);
    expect(latencySummary([focusTargetMs]).isWithinTarget).toBe(true);
  });

  it("says nothing of none", () => {
    expect(latencySummary([])).toEqual({
      count: 0,
      medianMs: undefined,
      worstMs: undefined,
      isWithinTarget: undefined,
    });
  });
});

describe("the gaps in a 1 s ticker", () => {
  it("are the stretches over two seconds between ticks, when each began and how long it lasted", () => {
    const finder = new GapFinder(1000);
    expect(finder.tick(2000)).toBeUndefined();
    expect(finder.tick(2000 + frozenAfterMs)).toBeUndefined();
    expect(finder.tick(9000)).toEqual({ afterMs: 3000, lastedMs: 5000 });
    expect(finder.tick(10_000)).toBeUndefined();
    expect(finder.tick(13_500)).toEqual({ afterMs: 9000, lastedMs: 3500 });
    expect(finder.gaps).toHaveLength(2);
    expect(finder.longestMs()).toBe(5000);
  });

  it("counts a page frozen from its start", () => {
    const finder = new GapFinder(0);
    expect(finder.tick(60_000)).toEqual({ afterMs: 0, lastedMs: 60_000 });
  });
});

describe("an event's time on the page's clock", () => {
  it("is its own where the browser keeps it there, else when it was heard", () => {
    expect(eventTimeMs(1200, 1250)).toBe(1200);
    // An old WebView's time since 1970, and none at all.
    expect(eventTimeMs(1_761_000_000_000, 1250)).toBe(1250);
    expect(eventTimeMs(0, 1250)).toBe(1250);
  });
});

describe("a run's elapsed time", () => {
  it("reads as hours, minutes and seconds", () => {
    expect(elapsedText(0)).toBe("0:00:00");
    expect(elapsedText(7_384_900)).toBe("2:03:04");
  });
});
