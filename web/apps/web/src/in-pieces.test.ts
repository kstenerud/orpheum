// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { inPieces, pieceBudgetMs } from "./in-pieces.ts";

/** A clock a test moves, each piece shown costing costPerPiece, and the yields it was handed. */
function handClock(costPerPiece: number): {
  readonly now: () => number;
  readonly shownAt: number[];
  readonly yieldedAfter: number[];
  readonly showUpTo: (shown: number) => void;
  readonly yieldNow: () => Promise<void>;
} {
  let time = 0;
  let last = 0;
  const shownAt: number[] = [];
  const yieldedAfter: number[] = [];
  return {
    now: () => time,
    shownAt,
    yieldedAfter,
    showUpTo: (shown) => {
      time += costPerPiece;
      last = shown;
      shownAt.push(shown);
    },
    yieldNow: () => {
      yieldedAfter.push(last);
      return Promise.resolve();
    },
  };
}

describe("inPieces (break-up-long-tasks; the owner, 2026-11-11)", () => {
  it("shows piece after piece until 50 ms have passed, then hands the main thread back", async () => {
    const clock = handClock(20);
    const isDone = await inPieces({ count: 100, piece: 12, isCurrent: () => true, ...clock });
    expect(isDone).toBe(true);
    expect(clock.shownAt).toEqual([12, 24, 36, 48, 60, 72, 84, 96, 100]);
    // 20 ms a piece: the third passes the budget, and each run after a yield starts a new one.
    expect(pieceBudgetMs).toBe(50);
    expect(clock.yieldedAfter).toEqual([36, 72]);
  });

  it("does cheap pieces without a yield, and never yields after the last", async () => {
    const clock = handClock(1);
    await inPieces({ count: 30, piece: 12, isCurrent: () => true, ...clock });
    expect(clock.shownAt).toEqual([12, 24, 30]);
    expect(clock.yieldedAfter).toEqual([]);
    const slow = handClock(60);
    await inPieces({ count: 24, piece: 12, isCurrent: () => true, ...slow });
    expect(slow.yieldedAfter).toEqual([12]);
  });

  it("shows none once, as none, so an empty page still arrives", async () => {
    const clock = handClock(1);
    expect(await inPieces({ count: 0, piece: 12, isCurrent: () => true, ...clock })).toBe(true);
    expect(clock.shownAt).toEqual([0]);
  });

  it("stops once what shows them is gone, and says so", async () => {
    const clock = handClock(60);
    let isCurrent = true;
    const done = inPieces({
      count: 48,
      piece: 12,
      isCurrent: () => isCurrent,
      ...clock,
      yieldNow: () => {
        isCurrent = false;
        return clock.yieldNow();
      },
    });
    expect(await done).toBe(false);
    expect(clock.shownAt).toEqual([12]);
  });
});
