// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  dropsCountWithinMs,
  highestSteadiness,
  holdBeforeStepUpMsAt,
  relaxEveryMs,
  Steadiness,
  stepUpHeadroomAt,
  swingWithinHolds,
} from "./steadiness.ts";

describe("a playback's steadiness (D352 §3)", () => {
  it("starts at 0: a 30 s hold, a headroom of 1.5, and climbs of several qualities", () => {
    const steadiness = new Steadiness(0);
    expect(steadiness.level()).toBe(0);
    expect(steadiness.holdMs()).toBe(30_000);
    expect(steadiness.headroom()).toBe(1.5);
    expect(steadiness.climbsOneAtATime()).toBe(false);
  });

  it("a climb that falls back within twice its hold raises the level and doubles the hold", () => {
    const steadiness = new Steadiness(0);
    steadiness.steppedUp(100_000);
    expect(steadiness.steppedDown(100_000 + swingWithinHolds * 30_000)).toBe(
      "steadiness 0 to 1: a swing, 60 s after a step up",
    );
    expect(steadiness.holdMs()).toBe(60_000);
    expect(steadiness.climbsOneAtATime()).toBe(true);
    // The same climb falls back once: a second step down is no second swing.
    expect(steadiness.steppedDown(170_000)).toBeUndefined();
    steadiness.steppedUp(200_000);
    steadiness.steppedDown(250_000);
    expect([steadiness.level(), steadiness.holdMs(), steadiness.headroom()]).toEqual([
      2, 120_000, 1.75,
    ]);
    steadiness.steppedUp(300_000);
    steadiness.dropped(310_000); // a chunk given up after a climb is a swing too
    expect([steadiness.level(), steadiness.holdMs(), steadiness.headroom()]).toEqual([
      highestSteadiness,
      240_000,
      2,
    ]);
    steadiness.steppedUp(400_000);
    expect(steadiness.steppedDown(400_001)).toBeUndefined(); // no higher than 3
    expect(holdBeforeStepUpMsAt).toEqual([30_000, 60_000, 120_000, 240_000]);
    expect(stepUpHeadroomAt).toEqual([1.5, 1.5, 1.75, 2]);
  });

  it("is not raised by a step down long after a step up, or with none before it", () => {
    const steadiness = new Steadiness(0);
    expect(steadiness.steppedDown(10_000)).toBeUndefined();
    steadiness.steppedUp(100_000);
    expect(steadiness.steppedDown(100_001 + swingWithinHolds * 30_000)).toBeUndefined();
    expect(steadiness.level()).toBe(0);
  });

  it("relaxes one level for each ten minutes without a swing or a drop", () => {
    const steadiness = new Steadiness(0);
    for (const atMs of [0, 1000, 2000]) {
      steadiness.steppedUp(atMs);
      steadiness.steppedDown(atMs + 500);
    }
    expect(steadiness.level()).toBe(3);
    expect(steadiness.relax(2500 + relaxEveryMs - 1)).toBeUndefined();
    expect(steadiness.relax(2500 + relaxEveryMs)).toBe(
      "steadiness 3 to 2: 10 min with no swing and no drop",
    );
    expect(steadiness.relax(2500 + 3 * relaxEveryMs)).toBe(
      "steadiness 2 to 0: 20 min with no swing and no drop",
    );
    expect(steadiness.holdMs()).toBe(30_000);
  });

  it("restarts its ten minutes at each drop, though the drop raises nothing", () => {
    const steadiness = new Steadiness(0);
    steadiness.steppedUp(0);
    steadiness.steppedDown(1000);
    steadiness.dropped(relaxEveryMs); // the first drop: no raise, but no calm either
    expect(steadiness.relax(relaxEveryMs + 1000)).toBeUndefined();
    expect(steadiness.level()).toBe(1);
  });
});

describe("a link that flakes (D352 §4)", () => {
  it("a second drop within ten minutes raises the level, as each after it does", () => {
    const steadiness = new Steadiness(0);
    expect(steadiness.dropped(0)).toBeUndefined();
    expect(steadiness.dropped(dropsCountWithinMs)).toBe(
      "steadiness 0 to 1: a drop 600 s after the first within 10 min",
    );
    expect(steadiness.dropped(dropsCountWithinMs)).toMatch(/^steadiness 1 to 2: /u);
  });

  it("counts a drop after the ten minutes as a first again", () => {
    const steadiness = new Steadiness(0);
    steadiness.dropped(0);
    expect(steadiness.dropped(dropsCountWithinMs + 1)).toBeUndefined();
    expect(steadiness.dropped(dropsCountWithinMs + 2)).toMatch(/^steadiness 0 to 1: /u);
  });

  it("raises the level once for a drop that is both a swing and a second drop", () => {
    const steadiness = new Steadiness(0);
    steadiness.dropped(0);
    steadiness.steppedUp(1000);
    steadiness.dropped(2000);
    expect(steadiness.level()).toBe(1);
  });
});
