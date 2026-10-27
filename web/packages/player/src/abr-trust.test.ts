// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  lookAtChunkEveryMs,
  lookForStepUpEveryMs,
  startingPlayedMs,
  startingSamples,
} from "./abr.ts";
import { linkSamplesKept } from "./link-estimate.ts";
import { holdBeforeStepUpMsAt, relaxEveryMs, swingWithinHolds } from "./steadiness.ts";
import { stepUpBufferMs } from "./step-up.ts";
import {
  chunkAt,
  choosingFrom,
  loadAt,
  pastStarting,
  playFor,
  scene,
  useScene,
  videoAt,
  waitMs,
} from "./test-choosing.ts";

/**
 * How far the chunked player trusts its link (D352): its first seconds, the step-up, its
 * steadiness, a link that flakes. The notes of each choice are abr.test.ts's.
 */

useScene({ beforeEach, afterEach, vi });

describe("a playback's first seconds, which latch nothing (D352 §1)", () => {
  it("a low first sample does not hold the playback low once its next chunks arrive fast", () => {
    const { choosing } = choosingFrom(40_000_000);
    expect(choosing.firstLevel()).toBe(3);
    loadAt(choosing, 3, { mbps: 1, seconds: 2 }); // the owner's 1008 kb/s, on a 12 Mb/s path (staging, 2026-10-26)
    expect(choosing.nextLevel()).toBe(0); // the next chunk follows the estimate as it stands
    expect(choosing.linkKbps()).toBeUndefined(); // and a lone first sample is never reported
    for (let index = 0; index < linkSamplesKept; index += 1) {
      loadAt(choosing, 0, { mbps: 12 });
    }
    expect(choosing.nextLevel()).toBe(3); // the ceiling stood: back on the film's own picture
    expect(choosing.isStarting()).toBe(true);
    expect(scene.steps).toEqual([]);
  });

  it("a chunk given up while starting lowers the ceiling", () => {
    const { choosing, givenUp } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    chunk.stats.loaded = 750_000; // 1 Mb/s over six seconds
    chunk.stats.loading = { start: 0, first: 50, end: 0 };
    waitMs(6000);
    expect(givenUp).toEqual([chunk]);
    for (let index = 0; index < linkSamplesKept; index += 1) {
      loadAt(choosing, 0, { mbps: 12 });
    }
    expect(choosing.nextLevel()).toBe(0); // the step-up's to raise again
  });

  it("a stall while starting lowers the ceiling to what the estimate chooses", () => {
    const { choosing } = choosingFrom(40_000_000);
    choosing.firstLevel();
    loadAt(choosing, 3, { mbps: 1, seconds: 2 });
    choosing.stalled();
    expect(scene.steps).toEqual([
      "ceiling 3 to 0: a stall; confident 1000 kb/s (mean 1000, error none, 1 sample), 0.0 s ahead," +
        " steadiness 0, hold 30 s",
    ]);
    for (let index = 0; index < linkSamplesKept; index += 1) {
      loadAt(choosing, 0, { mbps: 12 });
    }
    expect(choosing.nextLevel()).toBe(0);
  });

  it("ends after four samples and 20 s played, the ceiling no higher than the start", () => {
    const { hls, choosing } = choosingFrom(4_600_000); // offered 720p
    expect(choosing.firstLevel()).toBe(2);
    for (let index = 0; index < startingSamples; index += 1) {
      loadAt(choosing, 2, { mbps: 20 });
    }
    expect(choosing.isStarting()).toBe(true); // samples alone do not end it
    hls.media = videoAt(0, { isPaused: false, readyState: 4 });
    waitMs(lookAtChunkEveryMs);
    hls.media = videoAt(600, { isPaused: false, readyState: 4 }); // a seek forward is not played
    waitMs(lookAtChunkEveryMs);
    playFor(hls, startingPlayedMs - lookAtChunkEveryMs);
    expect(choosing.isStarting()).toBe(true);
    playFor(hls, lookAtChunkEveryMs);
    expect(choosing.isStarting()).toBe(false);
    expect(scene.steps[0]).toMatch(/^started, ceiling 2 kept; confident 20000 kb\/s/u);
    expect(choosing.nextLevel()).toBe(2); // 20 Mb/s carries the picture, but not above the start
  });

  it("reports its confident estimate as the link's speed, remembered for the device's next start", () => {
    const { choosing } = choosingFrom(40_000_000);
    loadAt(choosing, 3, { mbps: 10 });
    loadAt(choosing, 3, { mbps: 8 });
    expect(choosing.linkKbps()).toBeCloseTo(7111, 0); // 8.9 Mb/s, discounted by its 25% error
  });
});

describe("the step-up (D352 §2)", () => {
  /** A playback on the lowest quality, 20 s loaded ahead, past its starting at each speed. */
  function onTheLowest(...mbps: number[]): ReturnType<typeof choosingFrom> {
    const chosen = choosingFrom(1_000_000);
    chosen.hls.mainForwardBufferInfo = { len: stepUpBufferMs / 1000 };
    chosen.hls.currentLevel = 0;
    pastStarting(chosen, 0, ...mbps);
    return chosen;
  }

  it("climbs to the highest quality consistent samples carry, after the 30 s hold", () => {
    const { choosing } = onTheLowest(10);
    expect(choosing.nextLevel()).toBe(0);
    waitMs(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(3); // 10 Mb/s carries the picture's 5.1 by 1.5
  });

  it("climbs one quality when its samples disagree, and holds again after", () => {
    const { hls, choosing } = onTheLowest(10, 14, 10, 14);
    waitMs(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(1);
    hls.currentLevel = 1;
    choosing.switched();
    waitMs(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(1); // held again at the new quality
    waitMs(holdBeforeStepUpMsAt[0] ?? 0);
    expect(choosing.nextLevel()).toBe(2);
  });
});

describe("a playback that swings (D352 §3)", () => {
  it("a climb that falls back within twice its hold raises the level, and the next climb waits twice as long, one quality", () => {
    const chosen = choosingFrom(1_000_000);
    const { hls, choosing } = chosen;
    hls.mainForwardBufferInfo = { len: stepUpBufferMs / 1000 };
    hls.currentLevel = 0;
    pastStarting(chosen, 0, 10);
    waitMs(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(3);
    hls.currentLevel = 3;
    choosing.switched();
    waitMs(lookAtChunkEveryMs);
    loadAt(choosing, 3, { mbps: 2 }); // the link falls back
    expect(choosing.nextLevel()).toBe(2);
    expect(choosing.steadiness()).toBe(1);
    expect(scene.steps.at(-1)).toMatch(/^steadiness 0 to 1: a swing, \d+ s after a step up$/u);
    hls.currentLevel = 2;
    choosing.switched();
    for (let index = 0; index < linkSamplesKept; index += 1) {
      loadAt(choosing, 2, { mbps: 20 });
    }
    waitMs(holdBeforeStepUpMsAt[0] ?? 0);
    expect(choosing.nextLevel()).toBe(2); // 30 s is no longer the hold
    waitMs((holdBeforeStepUpMsAt[1] ?? 0) - (holdBeforeStepUpMsAt[0] ?? 0));
    expect(choosing.nextLevel()).toBe(3);
  });

  it("does not count a step down long after its climb", () => {
    const chosen = choosingFrom(1_000_000);
    const { hls, choosing } = chosen;
    hls.mainForwardBufferInfo = { len: stepUpBufferMs / 1000 };
    hls.currentLevel = 0;
    pastStarting(chosen, 0, 10);
    waitMs(lookForStepUpEveryMs);
    hls.currentLevel = 3;
    choosing.switched();
    waitMs(swingWithinHolds * (holdBeforeStepUpMsAt[0] ?? 0) + lookForStepUpEveryMs);
    loadAt(choosing, 3, { mbps: 2 });
    expect(choosing.nextLevel()).toBe(2);
    expect(choosing.steadiness()).toBe(0);
  });
});

describe("a link that flakes (D352 §4)", () => {
  it("a second drop within ten minutes raises the level", () => {
    const { choosing } = choosingFrom(40_000_000);
    loadAt(choosing, 3, { mbps: 20 });
    choosing.stalled();
    expect(choosing.steadiness()).toBe(0);
    waitMs(60_000);
    choosing.stalled();
    expect(choosing.steadiness()).toBe(1);
    expect(scene.steps.at(-1)).toBe("steadiness 0 to 1: a drop 60 s after the first within 10 min");
  });

  it("relaxes as its looks find ten minutes with no swing and no drop", () => {
    const { choosing } = choosingFrom(40_000_000);
    loadAt(choosing, 3, { mbps: 20 });
    choosing.stalled();
    choosing.stalled();
    expect(choosing.steadiness()).toBe(1);
    waitMs(relaxEveryMs + lookForStepUpEveryMs);
    expect(choosing.steadiness()).toBe(0);
    expect(scene.steps.at(-1)).toBe("steadiness 1 to 0: 10 min with no swing and no drop");
  });

  it("counts each Watch Together player's drops as its own, the film's own picture kept or not (Q4)", () => {
    scene.isInAGroup = true;
    const flaky = choosingFrom(40_000_000);
    const steady = choosingFrom(40_000_000);
    loadAt(flaky.choosing, 3, { mbps: 20 });
    loadAt(steady.choosing, 3, { mbps: 20 });
    flaky.choosing.stalled();
    flaky.choosing.stalled();
    expect([flaky.choosing.steadiness(), steady.choosing.steadiness()]).toEqual([1, 0]);
  });
});
