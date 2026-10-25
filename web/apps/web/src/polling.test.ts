// SPDX-License-Identifier: MPL-2.0

import { sendFailuresTo } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  askAnswersWithinMs,
  backgroundChangeWaits,
  changingWaitsPoller,
  clientWaits,
  nextWaitMs,
  pollUntilStopped,
} from "./polling.ts";

describe("polling (D237)", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("doubles the wait while nothing changes, to 30 s, and goes back to 2 s after a change", () => {
    const waits: number[] = [];
    let waitMs = backgroundChangeWaits.firstMs;
    for (let ask = 0; ask < 6; ask += 1) {
      waits.push(waitMs);
      waitMs = nextWaitMs(backgroundChangeWaits, waitMs, "unchanged");
    }
    expect(waits).toEqual([2000, 4000, 8000, 16000, 30000, 30000]);
    expect(nextWaitMs(backgroundChangeWaits, 30000, "changed")).toBe(2000);
  });

  it("asks again 5 s after each failure, which is no answer to back off from (the owner, 2026-10-06)", () => {
    expect(nextWaitMs(backgroundChangeWaits, 30000, "failed")).toBe(5000);
    expect(nextWaitMs(backgroundChangeWaits, 5000, "failed")).toBe(5000);
    expect(nextWaitMs(backgroundChangeWaits, 5000, "unchanged")).toBe(10000);
  });

  it("goes on polling when its waits change, by the new waits from its next wait (W3)", async () => {
    vi.useFakeTimers();
    let asked = 0;
    const changing = changingWaitsPoller(backgroundChangeWaits);
    const stop = changing.poller.start((answered) => {
      asked += 1;
      answered("changed");
    });
    // The first wait was started on the waits before, 2 s; the one after it is the new first, 3 s.
    changing.setWaits({ firstMs: 3000, longestMs: 60_000, failedMs: 7000 });
    await vi.advanceTimersByTimeAsync(2000);
    expect(asked).toBe(1);
    await vi.advanceTimersByTimeAsync(2999);
    expect(asked).toBe(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(asked).toBe(2);
    stop();
  });

  it("asks on its schedule until it is stopped, and not after", async () => {
    vi.useFakeTimers();
    let asked = 0;
    const stop = pollUntilStopped(backgroundChangeWaits, (answered) => {
      asked += 1;
      answered("unchanged");
    });
    await vi.advanceTimersByTimeAsync(2000 + 4000 + 8000);
    expect(asked).toBe(3);
    stop();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(asked).toBe(3);
  });

  it("gives up an ask that never answers, aborts it, says so, and asks again after the failed wait (the owner, 2026-10-24)", async () => {
    vi.useFakeTimers();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const sent: string[] = [];
    sendFailuresTo((text) => sent.push(text));
    const signals: (AbortSignal | undefined)[] = [];
    const lateAnswers: ((outcome: "unchanged") => void)[] = [];
    const stop = pollUntilStopped(backgroundChangeWaits, (answered, signal) => {
      signals.push(signal);
      if (signals.length === 1) {
        lateAnswers.push(answered);
        return;
      }
      answered("unchanged");
    });
    await vi.advanceTimersByTimeAsync(2000 + askAnswersWithinMs - 1);
    expect(signals).toHaveLength(1);
    expect(signals[0]?.aborted).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(signals[0]?.aborted).toBe(true);
    expect(sent).toEqual([
      "TimeoutError: a background refresh had no answer within 60 s; it was given up and is asked again",
    ]);
    // An answer after it was given up is not heard: it schedules no ask of its own.
    lateAnswers[0]?.("unchanged");
    await vi.advanceTimersByTimeAsync(backgroundChangeWaits.failedMs);
    expect(signals).toHaveLength(2);
    // Then on its usual schedule, from the failed wait: 10 s after an unchanged answer.
    await vi.advanceTimersByTimeAsync(9999);
    expect(signals).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(signals).toHaveLength(3);
    expect(signals[1]?.aborted).toBe(false);
    stop();
    sendFailuresTo(() => undefined);
    vi.restoreAllMocks();
  });

  it("waits as the server's client settings say, D237's waits until they are read", () => {
    expect(clientWaits(undefined)).toEqual({ firstMs: 2000, longestMs: 30_000, failedMs: 5000 });
    expect(clientWaits({ firstSeconds: 5, longestSeconds: 60, failedSeconds: 7 })).toEqual({
      firstMs: 5000,
      longestMs: 60_000,
      failedMs: 7000,
    });
    // A longest wait shorter than the first is the first.
    expect(clientWaits({ firstSeconds: 10, longestSeconds: 4, failedSeconds: 5 })).toEqual({
      firstMs: 10_000,
      longestMs: 10_000,
      failedMs: 5000,
    });
  });
});
