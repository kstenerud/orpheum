// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { backgroundChangeWaits, nextWaitMs, pollUntilStopped } from "./polling.ts";

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
});
