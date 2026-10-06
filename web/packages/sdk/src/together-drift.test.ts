// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { startingTuning, type GroupView, type SyncTuning } from "./together.ts";
import { driftCorrection, expectedPositionMs } from "./together-drift.ts";

describe("where the group is (D66: no clock sync)", () => {
  const playing: GroupView = {
    state: "Playing",
    positionMs: 10_000,
    rate: 1.5,
    fileId: 1,
    event: undefined,
    seekedFromMs: 0,
    byMemberId: undefined,
  };

  it("moves on at its rate from when its message arrived while it plays", () => {
    expect(expectedPositionMs(playing, { receivedAtMs: 500, nowMs: 1500 })).toBe(11_500);
  });

  it("stays where it was told while it does not play", () => {
    expect(
      expectedPositionMs({ ...playing, state: "Waiting" }, { receivedAtMs: 500, nowMs: 1500 }),
    ).toBe(10_000);
    expect(
      expectedPositionMs(
        { ...playing, state: "Waiting" },
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
    expect(startingTuning.leaveDriftUpToMs).toBe(500); // the owner's 500 ms, where it was 60
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
