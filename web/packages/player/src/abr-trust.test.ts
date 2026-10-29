// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  holdsQualityAheadS,
  lookAtChunkEveryMs,
  lookForStepUpEveryMs,
  startingPlayedMs,
} from "./abr.ts";
import { assumedLogSpread, linkSamplesKept } from "./link-estimate.ts";
import { holdBeforeStepUpMsAt, relaxEveryMs, swingWithinHolds } from "./steadiness.ts";
import { stepUpBufferMs } from "./step-up.ts";
import {
  chunkAt,
  choosingFrom,
  load,
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

/** A ladder like Nosferatu's on staging: its own picture at 9018 kb/s on top, q700k432p second. */
const nosferatu = [
  { bitrate: 600_000, averageBitrate: 450_000 },
  { bitrate: 1_000_000, averageBitrate: 800_000 },
  { bitrate: 3_000_000, averageBitrate: 2_200_000 },
  { bitrate: 30_000_000, averageBitrate: 9_018_000 },
];

/** The Fire TV's five samples of Nosferatu as its stall report gave them, bytes and ms to arrive (staging, 2026-10-28). */
const fireTvSamples = [
  [191_842, 159],
  [895_044, 393],
  [555_018, 349],
  [376_471, 1200],
  [801_491, 284],
] as const;

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

  it("starts its estimate from the link its device remembers: one bad first sample does not choose the lowest (P2)", () => {
    const remembered = { kbps: 17_000, logSpread: 0.3, countsAsSamples: 2 };
    const { choosing } = choosingFrom(40_000_000, undefined, { rememberedLink: remembered });
    expect(choosing.firstLevel()).toBe(3);
    loadAt(choosing, 3, { mbps: 1, seconds: 2 });
    expect(choosing.nextLevel()).toBe(3); // near 12.6 Mb/s, where alone it read 1
    loadAt(choosing, 3, { mbps: 1, seconds: 2 }); // a second agrees: the link has changed
    expect(choosing.nextLevel()).toBe(0);
    // The spread is noted once the link is reported, for the core to keep beside it: the prior
    // dropped, two samples measure none, so it is the one assumed.
    expect(scene.spreadsNoted).toEqual([assumedLogSpread]);
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

  it("a chunk given up stuck is no sample, and lowers the ceiling no further than the estimate carries (Nosferatu, P3)", () => {
    // A Fire TV's Nosferatu: picture chunks at 14–18 Mb/s, then one that delivered a few kilobytes and
    // nothing more, counted as a speed of 14 kb/s; the estimate read 69 kb/s and the TV played its
    // lowest quality for 75 s (staging, 2026-10-28; bandwidth-sampling-2026-10-29.md §1.1).
    const { hls, choosing, givenUp } = choosingFrom(40_000_000);
    hls.levels = nosferatu;
    expect(choosing.firstLevel()).toBe(3);
    for (const mbps of [14, 18, 15, 16]) {
      loadAt(choosing, 3, { mbps });
    }
    const samplesBefore = scene.samplesNoted.at(-1);
    const stuck = chunkAt(3);
    choosing.loading(stuck);
    stuck.stats.loaded = 5000;
    stuck.stats.loading = { start: scene.nowMs, first: scene.nowMs + 50, end: 0 };
    waitMs(lookAtChunkEveryMs);
    waitMs(5500); // none more for 5.5 s
    expect(givenUp).toEqual([stuck]);
    expect(scene.samplesNoted.at(-1)).toBe(samplesBefore); // no speed sample
    expect(choosing.linkKbps()).toBeGreaterThan(10_000); // where it read 69
    expect(scene.steps.at(-1)).toMatch(
      /^gave up a chunk of 3, stuck: 5000 B in 6000 ms, none for 5500 ms; /u,
    );
    expect(choosing.isStarting()).toBe(true);
    loadAt(choosing, 3, { mbps: 15 });
    expect(choosing.nextLevel()).toBe(3); // the top again, the ceiling never latched at the bottom
    // It was a drop: a second within ten minutes makes the playback steadier (D352 §4).
    choosing.stalled();
    expect(choosing.steadiness()).toBe(1);
  });

  it("a stall while starting lowers the ceiling to what the estimate chooses", () => {
    const { choosing } = choosingFrom(40_000_000);
    choosing.firstLevel();
    loadAt(choosing, 3, { mbps: 1, seconds: 2 });
    choosing.stalled();
    expect(scene.steps).toEqual([
      "ceiling 3 to 0: a stall; confident 615 kb/s (estimate 1000, spread 0.38, z 1.28, error none," +
        " 1 sample), 0.0 s ahead, steadiness 0, hold 30 s",
    ]);
    for (let index = 0; index < linkSamplesKept; index += 1) {
      loadAt(choosing, 0, { mbps: 12 });
    }
    expect(choosing.nextLevel()).toBe(0);
  });

  it("ends once its samples settle the ceiling and 20 s played, the ceiling no higher than the start (P5)", () => {
    const { hls, choosing } = choosingFrom(4_600_000); // offered 720p
    expect(choosing.firstLevel()).toBe(2);
    loadAt(choosing, 2, { mbps: 20 });
    hls.media = videoAt(0, { isPaused: false, readyState: 4 });
    waitMs(lookAtChunkEveryMs);
    hls.media = videoAt(600, { isPaused: false, readyState: 4 }); // a seek forward is not played
    waitMs(lookAtChunkEveryMs);
    playFor(hls, startingPlayedMs + lookAtChunkEveryMs);
    expect(choosing.isStarting()).toBe(true); // a lone sample settles nothing, played or not
    loadAt(choosing, 2, { mbps: 20 }); // a second that agrees: the ceiling is the same without either
    expect(choosing.isStarting()).toBe(false);
    expect(scene.steps[0]).toMatch(
      /^started, ceiling 2 kept; confident \d+ kb\/s \(estimate 20000, /u,
    );
    expect(choosing.nextLevel()).toBe(2); // 20 Mb/s carries the picture, but not above the start
  });

  it("reports its estimate as the link's speed, remembered for the device's next start's prior, never a bound of it", () => {
    const { choosing } = choosingFrom(40_000_000);
    loadAt(choosing, 3, { mbps: 10 });
    expect(choosing.linkKbps()).toBeUndefined(); // never a lone first sample
    loadAt(choosing, 3, { mbps: 8 });
    // 8.9 Mb/s, the centre of 10 and 8: a bound, remembered, would start each playback lower.
    expect(choosing.linkKbps()).toBeCloseTo(Math.sqrt(10 * 8) * 1000, 0);
  });
});

describe("the link the start gate reckons a next chunk's load over (bandwidth-sampling P5)", () => {
  const fresh17 = { kbps: 17_000, logSpread: 0.3, countsAsSamples: 2 };

  it("is the remembered link's bound before any chunk, and none with nothing remembered and nothing measured", () => {
    expect(choosingFrom(40_000_000).choosing.nextChunkLink()).toBeUndefined();
    const { choosing } = choosingFrom(40_000_000, undefined, { rememberedLink: fresh17 });
    const link = choosing.nextChunkLink();
    expect(link?.from).toBe("remembered");
    // 17 Mb/s, 1.28 spreads of 0.3 below, the next chunk's luck with it: about 10.6 Mb/s.
    expect(link?.kbps).toBeCloseTo(17_000 * Math.exp(-1.28 * 0.3 * Math.sqrt(1.5)), 0);
    expect(link?.firstByteMs).toBe(0);
  });

  it("keeps a fresh remembered 17 Mb/s through one slow sample of two: the 753 kb/s start (failure 2)", () => {
    // Ten minutes after the film before measured 17 Mb/s, a start waited 10 s on a "confident"
    // 753 kb/s from its first two samples (staging, 2026-10-28; bandwidth-sampling §1.2).
    const { choosing } = choosingFrom(40_000_000, undefined, { rememberedLink: fresh17 });
    loadAt(choosing, 3, { mbps: 17 });
    loadAt(choosing, 3, { mbps: 0.3, seconds: 4 });
    const link = choosing.nextChunkLink();
    expect(link?.from).toBe("confident");
    expect(link?.kbps).toBeGreaterThan(8000);
    expect(link?.firstByteMs).toBe(50);
  });

  it("still waits on a link that really is slow: two samples at 753 kb/s outvote the remembered link", () => {
    const { choosing } = choosingFrom(40_000_000, undefined, { rememberedLink: fresh17 });
    loadAt(choosing, 3, { mbps: 0.753, seconds: 2 });
    loadAt(choosing, 3, { mbps: 0.753, seconds: 2 });
    const link = choosing.nextChunkLink();
    expect(link?.kbps).toBeGreaterThan(350);
    expect(link?.kbps).toBeLessThan(760);
  });

  it("reads a Fire TV's fast first chunk against its remembered link, never alone: 28.4 Mb/s on a 9.6 Mb/s link", () => {
    // Staging, 2026-10-28, device 60: its first chunk of picture at 28384 kb/s started it with 1 s
    // loaded, and its next took 2.6 s at 9.6 Mb/s.
    const { hls, choosing } = choosingFrom(40_000_000, undefined, {
      rememberedLink: { kbps: 9636, countsAsSamples: 2 },
    });
    hls.mainForwardBufferInfo = { len: 1.07 };
    load(choosing, 3, { bytes: (28_384_000 * 0.74) / 8, loadMs: 790 });
    expect(choosing.nextChunkLink()?.kbps).toBeLessThan(9636);
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

describe("a playback with plenty loaded ahead, which holds its quality (the owner, 2026-10-28)", () => {
  it("keeps its quality at starting's end with 37.3 s loaded, and decides once less is loaded", () => {
    const { hls, choosing } = choosingFrom(40_000_000);
    hls.levels = nosferatu;
    expect(choosing.firstLevel()).toBe(3);
    hls.mainForwardBufferInfo = { len: 37.3 };
    // A link slower than the top, as most of its samples say.
    for (const mbps of [4, 14, 4, 14, 4]) {
      loadAt(choosing, 3, { mbps });
      expect(choosing.nextLevel()).toBe(3); // each slow one included
    }
    playFor(hls, startingPlayedMs + lookAtChunkEveryMs);
    expect(choosing.isStarting()).toBe(false);
    expect(scene.steps[0]).toMatch(
      /^started, ceiling 3 kept, held at 3: link carries [0-2], 37\.3 s loaded ahead \(holds from 20 s\); confident \d+ kb\/s/u,
    );
    expect(choosing.nextLevel()).toBe(3);
    hls.setVideoAside();
    // Less than the hold loaded: the link decides, on the evidence then.
    hls.mainForwardBufferInfo = { len: holdsQualityAheadS - 5 };
    expect(choosing.nextLevel()).toBe(2);
    expect(scene.steps.at(-1)).toMatch(/^ceiling 3 to 2: link; confident \d+ kb\/s/u);
  });

  it("keeps the top at starting's end with 10 s loaded through one slow sample of five (failure 3, P4)", () => {
    // The Fire TV's five samples cut its ceiling from the top to the second lowest: the divisor of
    // one plus its largest error read 1.3 Mb/s (staging, Nosferatu, 2026-10-28). A top of 6 Mb/s, above
    // what 70% of that divisor's reading carries over this estimate, 5.1, and within its bound's 6.6.
    const { hls, choosing } = choosingFrom(40_000_000);
    hls.levels = [...nosferatu.slice(0, 3), { bitrate: 30_000_000, averageBitrate: 6_000_000 }];
    expect(choosing.firstLevel()).toBe(3);
    hls.mainForwardBufferInfo = { len: 10 };
    for (const [bytes, arrivedMs] of fireTvSamples) {
      load(choosing, 3, { bytes, loadMs: arrivedMs + 50 });
    }
    playFor(hls, startingPlayedMs + lookAtChunkEveryMs);
    expect(choosing.isStarting()).toBe(false);
    expect(scene.steps[0]).toMatch(/^started, ceiling 3 kept; confident \d+ kb\/s .* z 0\.84, /u);
    hls.setVideoAside();
    expect(choosing.nextLevel()).toBe(3);
  });

  it("holds the quality loading against a slow link while the hold is loaded, and says so once", () => {
    const chosen = choosingFrom(40_000_000);
    const { hls, choosing } = chosen;
    pastStarting(chosen, 3, 20);
    expect(choosing.nextLevel()).toBe(3);
    hls.mainForwardBufferInfo = { len: holdsQualityAheadS };
    // A link genuinely slow: three samples of five say so.
    for (let index = 0; index < 3; index += 1) {
      loadAt(choosing, 3, { mbps: 1.5, seconds: 2 });
    }
    expect(choosing.nextLevel()).toBe(3);
    expect(choosing.nextLevel()).toBe(3);
    expect(scene.steps.slice(1)).toEqual([
      expect.stringMatching(
        /^ceiling 3 held at 3: link carries 1, 20\.0 s loaded ahead \(holds from 20 s\); confident \d+ kb\/s/u,
      ),
    ]);
    expect(choosing.steadiness()).toBe(0);
    hls.mainForwardBufferInfo = { len: holdsQualityAheadS - 0.1 };
    expect(choosing.nextLevel()).toBe(1);
    expect(scene.steps.at(-1)).toMatch(/^ceiling 3 to 1: link; /u);
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
    hls.mainForwardBufferInfo = { len: holdsQualityAheadS - 1 }; // the climb spent its buffer
    for (let index = 0; index < 3; index += 1) {
      loadAt(choosing, 3, { mbps: 4 }); // the link falls back, as three samples of five say
    }
    expect(choosing.nextLevel()).toBe(2);
    expect(choosing.steadiness()).toBe(1);
    expect(scene.steps.at(-1)).toMatch(/^steadiness 0 to 1: a swing, \d+ s after a step up$/u);
    hls.currentLevel = 2;
    choosing.switched();
    hls.mainForwardBufferInfo = { len: stepUpBufferMs / 1000 };
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
    for (let index = 0; index < 3; index += 1) {
      loadAt(choosing, 3, { mbps: 4 });
    }
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

  it("starts at the level its device last ended at, told as it starts and at each change (D352 Q2)", () => {
    const { choosing } = choosingFrom(40_000_000, undefined, 2);
    expect(choosing.steadiness()).toBe(2);
    expect(scene.steadinessNoted).toEqual([2]);
    loadAt(choosing, 3, { mbps: 20 });
    choosing.stalled();
    choosing.stalled();
    expect(choosing.steadiness()).toBe(3);
    waitMs(relaxEveryMs + lookForStepUpEveryMs);
    expect(scene.steadinessNoted).toEqual([2, 3, 2]);
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
