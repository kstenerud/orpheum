// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { backgroundChangeWaits, clientWaits, nextWaitMs, pollUntilStopped } from "./polling.ts";

describe("polling (D237)", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("doubles the wait while nothing changes, to 30 s, and goes back to 2 s after a change", () => {
    const waits: number[] = [];
    let waitMs = backgroundChangeWaits.firstMs;
    for (let ask = 0; ask < 6; ask += 1) {
      waits.push(waitMs);
      waitMs = nextWaitMs(backgroundChangeWaits, waitMs, false);
    }
    expect(waits).toEqual([2000, 4000, 8000, 16000, 30000, 30000]);
    expect(nextWaitMs(backgroundChangeWaits, 30000, true)).toBe(2000);
  });

  it("asks on its schedule until it is stopped, and not after", async () => {
    vi.useFakeTimers();
    let asked = 0;
    const stop = pollUntilStopped(backgroundChangeWaits, (answered) => {
      asked += 1;
      answered(false);
    });
    await vi.advanceTimersByTimeAsync(2000 + 4000 + 8000);
    expect(asked).toBe(3);
    stop();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(asked).toBe(3);
  });

  it("waits as the server's client settings say, D237's waits until they are read", () => {
    expect(clientWaits(undefined)).toEqual({ firstMs: 2000, longestMs: 30_000 });
    expect(clientWaits({ firstSeconds: 5, longestSeconds: 60 })).toEqual({
      firstMs: 5000,
      longestMs: 60_000,
    });
    // A longest wait shorter than the first is the first.
    expect(clientWaits({ firstSeconds: 10, longestSeconds: 4 })).toEqual({
      firstMs: 10_000,
      longestMs: 10_000,
    });
  });
});
