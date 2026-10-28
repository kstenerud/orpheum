// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { fromCacheBy, type ResourceTimings } from "./cache-hit.ts";

/** A page's Resource Timing holding entries, by name, which a test fills and empties. */
function timingsOf(entries: Record<string, object[]>): ResourceTimings & {
  readonly fill: () => void;
  readonly cleared: number[];
} {
  const listeners: (() => void)[] = [];
  const cleared: number[] = [];
  const named = (name: string): PerformanceEntryList =>
    (entries[name] ?? []).map((each) => ({
      name,
      entryType: "resource",
      startTime: 0,
      duration: 1,
      toJSON: () => each,
      ...each,
    }));
  return {
    getEntriesByName: named,
    clearResourceTimings: () => {
      cleared.push(1);
    },
    addEventListener: (_type: string, listener: () => void) => {
      listeners.push(listener);
    },
    fill: () => {
      for (const listener of listeners) {
        listener();
      }
    },
    cleared,
  };
}

describe("telling a chunk from the browser's cache", () => {
  it("is its last fetch transferring nothing for a body it has, and a transfer otherwise", () => {
    const fromCache = fromCacheBy(
      timingsOf({
        "/c/1.m4s": [{ transferSize: 0, decodedBodySize: 900_000 }],
        "/c/2.m4s": [
          { transferSize: 0, decodedBodySize: 900_000 },
          { transferSize: 900_300, decodedBodySize: 900_000 },
        ],
      }),
    );
    expect(fromCache("/c/1.m4s")).toBe(true);
    expect(fromCache("/c/2.m4s")).toBe(false);
  });

  it("says nothing of a chunk with no entry, or one whose sizes the browser keeps to itself", () => {
    const fromCache = fromCacheBy(
      timingsOf({ "/c/3.m4s": [{ transferSize: 0, decodedBodySize: 0 }], "/c/4.m4s": [{}] }),
    );
    expect(fromCache("/c/3.m4s")).toBeUndefined();
    expect(fromCache("/c/4.m4s")).toBeUndefined();
    expect(fromCache("/c/5.m4s")).toBeUndefined();
  });

  it("says nothing of a load a service worker answered with nothing transferred, as Chromium tells one it built", () => {
    // Chromium's entry for a chunk the worker built from the page's WebRTC transport (Chromium 1243,
    // 2026-10-28): the cache hit's transferSize of 0, for a body it has.
    const fromCache = fromCacheBy(
      timingsOf({
        "/c/6.m4s": [{ transferSize: 0, decodedBodySize: 300_000, workerStart: 32.8 }],
        "/c/7.m4s": [{ transferSize: 300_300, decodedBodySize: 300_000, workerStart: 36.5 }],
        "/c/8.m4s": [{ transferSize: 0, decodedBodySize: 300_000, workerStart: 0 }],
      }),
    );
    expect(fromCache("/c/6.m4s")).toBeUndefined();
    expect(fromCache("/c/7.m4s")).toBe(false);
    expect(fromCache("/c/8.m4s")).toBe(true);
  });

  it("empties the page's timing buffer each time it fills, once however many playbacks ask", () => {
    const timings = timingsOf({});
    fromCacheBy(timings);
    fromCacheBy(timings);
    timings.fill();
    expect(timings.cleared).toEqual([1]);
  });
});
