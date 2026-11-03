// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it, vi } from "vitest";

import { at, fill, findLast, findLastIndex, timeout, toSorted } from "./old-engine.ts";

describe("what Chromium 90 lacks, filled for the 3rd-generation Chromecast", () => {
  it("reads from the end as at does", () => {
    expect(at.call([1, 2, 3], -1)).toBe(3);
    expect(at.call([1, 2, 3], 0)).toBe(1);
    expect(at.call([1, 2, 3], 3)).toBeUndefined();
    expect(at.call([1, 2, 3], -4)).toBeUndefined();
    expect(at.call("abc", -1)).toBe("c");
  });

  it("finds from the end, and sorts a copy, the array left as it was", () => {
    const values = [3, 1, 4, 1, 5];
    const findLastOf = findLast<number>;
    const findLastIndexOf = findLastIndex<number>;
    const toSortedOf = toSorted<number>;
    expect(findLastOf.call(values, (value: number) => value < 4)).toBe(1);
    expect(findLastIndexOf.call(values, (value: number) => value < 4)).toBe(3);
    expect(findLastIndexOf.call(values, (value: number) => value > 9)).toBe(-1);
    expect(toSortedOf.call(values, (a: number, b: number) => a - b)).toEqual([1, 1, 3, 4, 5]);
    expect(values).toEqual([3, 1, 4, 1, 5]);
  });

  it("aborts a timeout's signal with a TimeoutError once its time is up", async () => {
    vi.useFakeTimers();
    try {
      const signal = timeout(1000);
      expect(signal.aborted).toBe(false);
      await vi.advanceTimersByTimeAsync(1000);
      expect(signal.aborted).toBe(true);
      expect((signal.reason as DOMException).name).toBe("TimeoutError");
    } finally {
      vi.useRealTimers();
    }
  });

  it("fills only what is missing, leaving the engine's own", () => {
    const own = (): string => "own";
    const target: Record<string, unknown> = { at: own };
    fill(target, "at", at);
    fill(target, "findLast", findLast);
    expect(target["at"]).toBe(own);
    expect(target["findLast"]).toBe(findLast);
  });
});
