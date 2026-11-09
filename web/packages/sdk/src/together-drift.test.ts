// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { startingTuning, type GroupView, type SyncTuning } from "./together.ts";
import {
  driftCorrection,
  learnedLossMs,
  lossAtMostMs,
  seekOnlyCorrection,
  expectedPositionMs,
} from "./together-drift.ts";

describe("where the group is (D66: no clock sync)", () => {
  const playing: GroupView = {
    state: "playing",
    positionMs: 10_000,
    rate: 1.5,
    fileId: 1,
    event: undefined,
    seekedFromMs: 0,
    byMemberId: undefined,
    isHeldPlaying: false,
  };

  it("moves on at its rate from when its message arrived while it plays", () => {
    expect(expectedPositionMs(playing, { receivedAtMs: 500, nowMs: 1500 })).toBe(11_500);
  });

  it("stays where it was told while it does not play", () => {
    expect(
      expectedPositionMs({ ...playing, state: "waiting" }, { receivedAtMs: 500, nowMs: 1500 }),
    ).toBe(10_000);
    expect(
      expectedPositionMs(
        { ...playing, state: "waiting" },
        { receivedAtMs: 500, nowMs: 1500, transitMs: 150 },
      ),
    ).toBe(10_000);
  });

  it("has moved on for the message's way from the server too (D66)", () => {
    expect(expectedPositionMs(playing, { receivedAtMs: 500, nowMs: 1500, transitMs: 150 })).toBe(
      11_725,
    );
  });
});

describe("correcting drift by the welcome's tuning (D250 §1)", () => {
  it("leaves a drift within the threshold alone", () => {
    const leftMs = startingTuning.leaveDriftUpToMs;
    expect(
      driftCorrection({ expectedMs: 1000, actualMs: 1000 - leftMs, groupRate: 1 }, startingTuning),
    ).toEqual({ kind: "none" });
    expect(startingTuning.leaveDriftUpToMs).toBe(150); // the owner's 150 ms, so the 200 ms bound holds
  });

  it("nudges the rate to close a drift within about a second, as far as the gentlest nudge allows", () => {
    const fine: SyncTuning = { ...startingTuning, leaveDriftUpToMs: 50 };
    const behind = driftCorrection({ expectedMs: 1080, actualMs: 1000, groupRate: 1 }, fine);
    expect(behind.kind).toBe("nudge");
    expect(behind.kind === "nudge" ? behind.rate : 0).toBeCloseTo(1.08);
    const ahead = driftCorrection(
      { expectedMs: 1000, actualMs: 1600, groupRate: 1 },
      startingTuning,
    );
    expect(ahead.kind === "nudge" ? ahead.rate : 0).toBeCloseTo(0.9);
  });

  it("nudges no faster than the tuning's percent of the group's rate", () => {
    expect(
      driftCorrection({ expectedMs: 3000, actualMs: 100, groupRate: 1.25 }, startingTuning),
    ).toEqual({ kind: "nudge", rate: 1.25 * 1.1 });
    const wider: SyncTuning = { ...startingTuning, nudgeMostPercent: 25 };
    expect(driftCorrection({ expectedMs: 3000, actualMs: 100, groupRate: 1 }, wider)).toEqual({
      kind: "nudge",
      rate: 1.25,
    });
  });

  it("seeks across a drift beyond the seek threshold", () => {
    const beyondMs = startingTuning.seekDriftBeyondMs + 1;
    expect(
      driftCorrection(
        { expectedMs: 9000, actualMs: 9000 - beyondMs, groupRate: 1 },
        startingTuning,
      ),
    ).toEqual({ kind: "seek", toMs: 9000 });
  });
});

describe("a player whose rate going back to its group's costs it time, as Safari's does", () => {
  it("is nudged on past the group by that cost, so it lands on it", () => {
    // 100 ms behind is within the allowance, but losing 220 ms as the nudge ends would leave it
    // 320 ms behind: the nudge goes on.
    const drift = { expectedMs: 10_100, actualMs: 10_000, groupRate: 1 };
    expect(driftCorrection(drift, startingTuning, 220)).toEqual({ kind: "nudge", rate: 1.1 });
    expect(driftCorrection(drift, startingTuning, 0)).toEqual({ kind: "none" });
    // 120 ms ahead, it will be 100 ms behind once the return costs it 220: it is left alone.
    const ahead = { expectedMs: 10_000, actualMs: 10_120, groupRate: 1 };
    expect(driftCorrection(ahead, startingTuning, 220)).toEqual({ kind: "none" });
  });

  it("learns the cost from each return, a browser that loses nothing learning nothing", () => {
    // Behind 230 ms as the nudge ended and 450 ms once settled: the return cost 220, taken whole.
    const first = learnedLossMs({ costMs: 0, moves: 0 }, 230, 450);
    expect(first).toEqual({ costMs: 220, moves: 1 });
    // Then averaged: a return that lost 180 moves it halfway.
    expect(learnedLossMs(first, 230, 410)).toEqual({ costMs: 200, moves: 2 });
    // Gained, as Chrome's drift wobbles; and a stall, not a cost, counts at the most.
    expect(learnedLossMs({ costMs: 0, moves: 0 }, 120, 100).costMs).toBe(0);
    expect(learnedLossMs({ costMs: 0, moves: 0 }, 100, 5000).costMs).toBe(lossAtMostMs);
  });
});

describe("the correction of a player that corrects by seeks alone (D268 §2)", () => {
  it("leaves a drift within its own allowance, skips past the group by its seeks' loss, and catches up beyond", () => {
    const at = (behindMs: number): { expectedMs: number; actualMs: number; groupRate: number } => ({
      expectedMs: 10_000,
      actualMs: 10_000 - behindMs,
      groupRate: 1.25,
    });
    expect(seekOnlyCorrection(at(400), startingTuning, 200)).toEqual({ kind: "none" });
    expect(seekOnlyCorrection(at(-400), startingTuning, 200)).toEqual({ kind: "none" });
    expect(seekOnlyCorrection(at(401), startingTuning, 200)).toEqual({
      kind: "skip",
      toMs: 10_250,
    });
    expect(seekOnlyCorrection(at(-401), startingTuning, 0)).toEqual({ kind: "skip", toMs: 10_000 });
    expect(seekOnlyCorrection(at(3001), startingTuning, 200)).toEqual({
      kind: "seek",
      toMs: 10_000,
    });
  });
});
