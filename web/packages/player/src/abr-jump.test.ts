// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { linkSamplesKept } from "./link-estimate.ts";
import { holdBeforeStepUpMsAt } from "./steadiness.ts";
import { jumpsWithAheadS } from "./start-jump.ts";
import { stepUpBufferMs } from "./step-up.ts";
import {
  type Chosen,
  chunkAt,
  choosingFrom,
  loadAt,
  pastStarting,
  scene,
  useScene,
  waitMs,
} from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

/** The start the core offered, held by its node's speed: 1896k's peak, quality 1 of four. */
const heldStartBps = 1_896_000;

/**
 * A playback started within heldStartBps, held by its node's speed unless said, past its starting
 * on a link measured at mbps, little loaded ahead yet.
 */
function startedHeld(mbps: number, isStartHeldByNodeSpeed = true): Chosen {
  const chosen = choosingFrom(heldStartBps, undefined, { isStartHeldByNodeSpeed });
  pastStarting(chosen, 1, mbps);
  for (let index = 0; index < linkSamplesKept; index += 1) {
    loadAt(chosen.choosing, 1, { mbps });
  }
  chosen.hls.currentLevel = 1;
  return chosen;
}

/** The ceiling's steps noted so far. */
function ceilingSteps(): string[] {
  return scene.steps.filter((step) => step.startsWith("ceiling"));
}

// The core held the start below what the link and the upload allow because its node could not make
// the higher qualities fast enough (start-quality-by-node-speed.md §2); once ten seconds are
// loaded ahead the player asks at once for what its link carries (§4, S4; the owner's answer 2,
// 2026-11-27: "We try jumping to the link's quality. If that fails, we drop back down and climb
// slowly.").
describe("a start its node's speed held jumps once to its link's quality (start-quality-by-node-speed.md §4)", () => {
  it("with ten seconds loaded ahead asks its link's quality at once; after a stall there drops back to where it started and climbs a quality at a time by today's holds", () => {
    const { hls, choosing } = startedHeld(20);
    expect(choosing.firstLevel()).toBe(1);
    hls.mainForwardBufferInfo = { len: jumpsWithAheadS };
    waitMs(500);
    expect(ceilingSteps()).toEqual([
      expect.stringMatching(
        /^ceiling 1 to 3: jumped to its link's quality, its start held by its node's speed; /u,
      ),
    ]);
    expect(choosing.nextLevel()).toBe(3);
    hls.currentLevel = 3;
    choosing.stalled();
    expect(ceilingSteps().at(-1)).toMatch(/^ceiling 3 to 1: back from the jump after a stall; /u);
    expect(choosing.nextLevel()).toBe(1);
    // From there today's step-up: its hold, its buffer, then one quality, never the jump's two.
    hls.currentLevel = 1;
    hls.mainForwardBufferInfo = { len: stepUpBufferMs / 1000 };
    const stepsBefore = ceilingSteps().length;
    waitMs((holdBeforeStepUpMsAt[0] ?? 0) / 2);
    expect(ceilingSteps()).toHaveLength(stepsBefore); // the hold, doubled by the swing, not yet over
    waitMs(10 * 60_000);
    expect(ceilingSteps()[stepsBefore]).toMatch(/^ceiling 1 to 2: stepped up; /u);
  });

  it("drops back after a chunk of the jumped-to quality is given up, not merely a quality", () => {
    const { hls, choosing } = startedHeld(20);
    hls.mainForwardBufferInfo = { len: jumpsWithAheadS };
    waitMs(500);
    expect(choosing.nextLevel()).toBe(3);
    hls.currentLevel = 3;
    // A chunk of the top comes no further than its first bytes, and is given up.
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    chunk.stats.loaded = 100_000;
    chunk.stats.loading = { start: scene.nowMs, first: scene.nowMs + 50, end: 0 };
    waitMs(6000);
    expect(ceilingSteps()).toContainEqual(
      expect.stringMatching(/^ceiling 3 to 1: back from the jump after /u),
    );
  });

  it("does not jump before ten seconds are loaded ahead", () => {
    const { hls, choosing } = startedHeld(20);
    hls.mainForwardBufferInfo = { len: jumpsWithAheadS - 0.5 };
    waitMs(5_000);
    expect(ceilingSteps()).toEqual([]);
    expect(choosing.nextLevel()).toBe(1);
    hls.mainForwardBufferInfo = { len: jumpsWithAheadS };
    waitMs(500);
    expect(choosing.nextLevel()).toBe(3);
  });

  it("jumps no second time: neither after its fall back nor once spent where the link carried nothing higher", () => {
    const fell = startedHeld(20);
    fell.hls.mainForwardBufferInfo = { len: jumpsWithAheadS };
    waitMs(500);
    fell.hls.currentLevel = 3;
    fell.choosing.stalled();
    fell.hls.currentLevel = 1;
    waitMs(5_000);
    expect(ceilingSteps().filter((step) => step.includes("jumped"))).toHaveLength(1);
    expect(fell.choosing.nextLevel()).toBe(1);
    scene.steps = [];
    // A link that carried nothing above its start at ten seconds ahead spends the jump.
    const slow = startedHeld(1.5);
    slow.hls.mainForwardBufferInfo = { len: jumpsWithAheadS };
    waitMs(500);
    for (let index = 0; index < linkSamplesKept; index += 1) {
      loadAt(slow.choosing, 1, { mbps: 20 });
    }
    waitMs(5_000);
    expect(ceilingSteps().filter((step) => step.includes("jumped"))).toEqual([]);
  });

  it("a start not held climbs as today: no jump, the step-up's hold and buffer first", () => {
    const { hls, choosing } = startedHeld(20, false);
    hls.mainForwardBufferInfo = { len: jumpsWithAheadS };
    waitMs(5_000);
    expect(ceilingSteps()).toEqual([]);
    expect(choosing.nextLevel()).toBe(1);
  });

  it("starts within the start its offer gave, the link its device remembers notwithstanding (D381)", () => {
    const chosen = choosingFrom(heldStartBps, undefined, {
      rememberedLink: { kbps: 20_000, countsAsSamples: 2 },
      shouldStartByRememberedLink: true,
      isStartHeldByNodeSpeed: true,
    });
    expect(chosen.choosing.firstLevel()).toBe(1);
  });
});
