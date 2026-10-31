// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  estimateText,
  assumedLogSpread,
  boundDeviationsAt,
  LinkEstimate,
  linkSamplesKept,
  logSpreadAtLeast,
  samplesAgreeWithinError,
  samplesText,
  trimsErrorFromSamples,
} from "./link-estimate.ts";

/** A chunk of level that arrived at mbps: a megabyte's bits over the time they took. */
function sampleAt(mbps: number, level = 3): Parameters<LinkEstimate["add"]>[0] {
  return { level, bytes: 1_000_000, arrivedMs: 8000 / mbps, firstByteMs: 40 };
}

/** An estimate of the samples at each speed, in Mb/s, oldest first. */
function estimateOf(...mbps: number[]): LinkEstimate {
  const estimate = new LinkEstimate();
  for (const speed of mbps) {
    estimate.add(sampleAt(speed));
  }
  return estimate;
}

describe("the link's estimate (D352 §2, revised 2026-10-29)", () => {
  it("is the centre of the last five samples in log space, each counted once whatever its length", () => {
    // Two samples: the centre of their logs, the geometric mean.
    expect(estimateOf(4, 16).estimateBps()).toBeCloseTo(8_000_000, -1);
    const estimate = estimateOf(1, 12, 12, 12, 12);
    // Five chunks later the slow one has left: the estimate is the link's.
    for (let index = 0; index < linkSamplesKept; index += 1) {
      estimate.add(sampleAt(12));
    }
    expect(estimate.estimateBps()).toBeCloseTo(12_000_000, -1);
    expect(estimate.kept()).toBe(linkSamplesKept);
    expect(estimate.count()).toBe(10);
  });

  it("is not ruled by one sample that is not the link's: Aegis's four near 15 Mb/s and one at 14 kb/s", () => {
    // The harmonic mean made these 69 kb/s, and a Fire TV fell from Aegis's own picture to its
    // lowest quality for 75 s (staging, 2026-10-28; bandwidth-sampling-2026-10-29.md §1.1).
    const aegis = estimateOf(14, 18, 15, 16, 0.014);
    expect(aegis.estimateBps()).toBeGreaterThan(10_000_000);
    // Nor by one fast one, a cache hit not told as one.
    expect(estimateOf(10, 10, 10, 10, 1000).estimateBps()).toBeLessThan(15_000_000);
    // The spread is the samples' own once three are kept, never below its floor.
    expect(estimateOf(10, 10).spread()).toBe(assumedLogSpread);
    expect(estimateOf(10, 10, 10).spread()).toBe(logSpreadAtLeast);
    expect(estimateOf(4, 12, 3, 11).spread()).toBeGreaterThan(0.6);
  });

  it("follows a fall once most of its samples say it", () => {
    expect(estimateOf(12, 12, 12, 1, 1, 1).estimateBps()).toBeLessThan(2_000_000);
  });

  it("is read at a bound set by what is loaded ahead: the confident estimate and the next chunk's (P4)", () => {
    const steady = estimateOf(10, 10, 10, 10, 10);
    expect(steady.predictionError()).toBeCloseTo(0, 6);
    // With nothing loaded, 1.28 spreads (the floor, 0.3) over the root of five samples below.
    expect(steady.confidentBps(0)).toBeCloseTo(
      10_000_000 * Math.exp((-1.28 * 0.3) / Math.sqrt(5)),
      -1,
    );
    // The next chunk alone: its own luck is not averaged away.
    expect(steady.nextChunkBps(0)).toBeCloseTo(
      10_000_000 * Math.exp(-1.28 * 0.3 * Math.sqrt(1.2)),
      -1,
    );
    // From 20 s loaded, the median: the estimate itself.
    expect(steady.confidentBps(20)).toBeCloseTo(10_000_000, -1);
    expect(steady.confidentBps(30)).toBeCloseTo(10_000_000, -1);
    // Straight between the points: 1.06 at 5 s, 0.42 at 15 s.
    expect(boundDeviationsAt(5)).toBeCloseTo(1.06, 6);
    expect(boundDeviationsAt(15)).toBeCloseTo(0.42, 6);
    // 8 after an estimate of 10: the prediction was a quarter of the sample out, which says only
    // whether its samples agree.
    expect(estimateOf(10, 8).predictionError()).toBeCloseTo(0.25, 6);
  });

  it("has no bound before any sample, and none but the next chunk's from a prior alone", () => {
    expect(new LinkEstimate().estimateBps()).toBeUndefined();
    expect(new LinkEstimate().confidentBps(0)).toBeUndefined();
    expect(new LinkEstimate().nextChunkBps(0)).toBeUndefined();
    expect(estimateOf(5).confidentBps(20)).toBeCloseTo(5_000_000, -1);
    expect(estimateOf(5).predictionError()).toBeUndefined();
    const remembered = new LinkEstimate({ kbps: 17_000, logSpread: 0.3, countsAsSamples: 2 });
    expect(remembered.confidentBps(0)).toBeUndefined();
    expect(remembered.nextChunkBps(20)).toBeCloseTo(17_000_000, -1);
  });

  it("keeps a top quality through one slow sample of five where the old divisor cut it (failure 3)", () => {
    // The Fire TV's five samples (staging, Aegis, 2026-10-28): 9.65, 18.2, 12.7, 2.51, 22.6 Mb/s.
    const fireTv = new LinkEstimate();
    for (const [bytes, arrivedMs] of [
      [191_842, 159],
      [895_044, 393],
      [555_018, 349],
      [376_471, 1200],
      [801_491, 284],
    ] as const) {
      fireTv.add({ level: 3, bytes, arrivedMs, firstByteMs: 40 });
    }
    // The 80% bound for the mean, at 10 s loaded: 9.3 Mb/s in bandwidth-sampling §1.3, where
    // RobustMPC's divisor read 1.3 Mb/s, and 7.3 over this estimate.
    expect((fireTv.confidentBps(10) ?? 0) / 1e6).toBeGreaterThan(9);
  });

  it("agrees when its predictions erred at most a fifth, and not before it has predicted", () => {
    expect(estimateOf(10).agrees()).toBe(false);
    expect(estimateOf(10, 10 / (1 + samplesAgreeWithinError) + 0.01, 10).agrees()).toBe(true);
    expect(estimateOf(10, 7, 10).agrees()).toBe(false);
  });

  it("a varying link is targeted near its slowest samples, the more so the less is loaded", () => {
    const varying = estimateOf(4, 12, 3, 11);
    const mean = (4 + 12 + 3 + 11) / 4;
    // Its spread is wide: one chunk's bound is near its slowest, under half its mean.
    expect((varying.nextChunkBps(0) ?? 0) / 1e6).toBeLessThan(3);
    expect((varying.nextChunkBps(0) ?? 0) / 1e6).toBeLessThan(mean / 2);
    expect(varying.confidentBps(0) ?? 0).toBeLessThan(varying.confidentBps(10) ?? 0);
    // A steady link's bound is far nearer its centre.
    expect((estimateOf(7, 8, 7.5, 7.2).nextChunkBps(0) ?? 0) / 1e6).toBeGreaterThan(4.5);
  });

  it("is not ruled by one slow sample of five: the most extreme error is set aside", () => {
    // A Fire TV's five samples as its stall report gave them (staging, Aegis, 2026-10-28).
    const fireTv = new LinkEstimate();
    for (const [bytes, arrivedMs] of [
      [191_842, 159],
      [895_044, 393],
      [555_018, 349],
      [376_471, 1200],
      [801_491, 284],
    ] as const) {
      fireTv.add({ level: 3, bytes, arrivedMs, firstByteMs: 40 });
    }
    expect(fireTv.kept()).toBe(trimsErrorFromSamples);
    expect((fireTv.estimateBps() ?? 0) / 1000).toBeCloseTo(11_488, -1); // the four good ones', where the harmonic mean made 7355
    expect(fireTv.predictionError()).toBeLessThan(1); // where the slow one alone made it 404%

    // Two slow samples of five are a link that varies, and still count.
    expect(estimateOf(12, 12, 2, 12, 2).predictionError()).toBeGreaterThan(1); // the second slow one's
    // Fewer than five kept: every error counts.
    expect(estimateOf(12, 12, 12, 2).predictionError()).toBeCloseTo(5, 6);
  });

  it("says itself and its samples in a line for the log (D352 §5)", () => {
    expect(estimateText(new LinkEstimate(), 0)).toBe("unmeasured");
    expect(estimateText(estimateOf(10, 8), 10)).toBe(
      "confident 7137 kb/s (estimate 8944, spread 0.38, z 0.84, error 25%, 2 samples)",
    );
    const remembered = new LinkEstimate({ kbps: 17_000, logSpread: 0.3, countsAsSamples: 2 });
    remembered.add(sampleAt(16));
    expect(estimateText(remembered, 20)).toMatch(/, 1 sample, prior 17000 kb\/s as 2\)$/u);
    remembered.add(sampleAt(1));
    remembered.add(sampleAt(1));
    expect(estimateText(remembered, 20)).toMatch(/, prior dropped at 3\)$/u);
    expect(samplesText([])).toBe("");
    expect(samplesText(estimateOf(10, 8).samples())).toBe(
      "samples 3 1000000B 800ms fb40ms, 3 1000000B 1000ms fb40ms",
    );
  });
});

describe("the bound's calibration (bandwidth-sampling-2026-10-29.md P6, §6.8 test 5)", () => {
  /** A seeded generator of uniform numbers in [0, 1) (mulberry32), so the test is the same each run. */
  function seeded(seed: number): () => number {
    let state = seed;
    return () => {
      state = (state + 0x6d2b79f5) | 0;
      let mixed = Math.imul(state ^ (state >>> 15), 1 | state);
      mixed = (mixed + Math.imul(mixed ^ (mixed >>> 7), 61 | mixed)) ^ mixed;
      return ((mixed ^ (mixed >>> 14)) >>> 0) / 4_294_967_296;
    };
  }

  /** A standard normal number from two uniform ones (Box and Muller). */
  function normalOf(random: () => number): number {
    return Math.sqrt(-2 * Math.log(Math.max(random(), 1e-12))) * Math.cos(2 * Math.PI * random());
  }

  it("is slower than its 90% bound for a tenth of the link's chunks, give or take 3%, with 5% of samples near nothing", () => {
    const random = seeded(29);
    let estimate = new LinkEstimate();
    let linksChunks = 0;
    let slower = 0;
    for (let chunk = 0; chunk < 20_000; chunk += 1) {
      if (chunk % 50 === 0) {
        estimate = new LinkEstimate(); // a playback of fifty chunks
      }
      // Log-normal with the Fire TV's spread, 0.38, about 10 Mb/s; one in twenty a stuck
      // transport's thousandth of it, taken as a sample all the same, the worst case for the
      // estimate: such a chunk is slower than any bound, and says nothing of the link's speed.
      const isStuck = random() < 0.05;
      const bps = isStuck ? 10_000 : 10_000_000 * Math.exp(0.38 * normalOf(random));
      const bound = estimate.nextChunkBps(0);
      if (bound !== undefined && !isStuck) {
        linksChunks += 1;
        slower += bps < bound ? 1 : 0;
      }
      estimate.add({ level: 0, bytes: 1_000_000, arrivedMs: 8e9 / bps, firstByteMs: 10 });
    }
    expect(slower / linksChunks).toBeGreaterThan(0.07);
    expect(slower / linksChunks).toBeLessThan(0.13);
  });
});

describe("the link its device remembers, as a prior (bandwidth-sampling-2026-10-29.md P2)", () => {
  /** An estimate from a fresh remembered 17 Mb/s, spread 0.3, counting as two samples, then each sample. */
  function fromRemembered(...mbps: number[]): LinkEstimate {
    const estimate = new LinkEstimate({ kbps: 17_000, logSpread: 0.3, countsAsSamples: 2 });
    for (const speed of mbps) {
      estimate.add(sampleAt(speed));
    }
    return estimate;
  }

  it("outlives one bad early sample: a first chunk at 1 Mb/s reads near 12.6 Mb/s, not 1", () => {
    const one = fromRemembered(1);
    expect((one.estimateBps() ?? 0) / 1e6).toBeGreaterThan(11);
    expect(one.prior()?.isKept).toBe(true);
    expect(one.countsAsSamples()).toBe(3);
    // A fast one and a slow one disagree with each other: the prior stays.
    const mixed = fromRemembered(17, 0.3);
    expect(mixed.prior()?.isKept).toBe(true);
    expect((mixed.estimateBps() ?? 0) / 1e6).toBeGreaterThan(8);
  });

  it("is dropped when two of the playback's own samples agree against it: a real fall still lowers the estimate", () => {
    const fell = fromRemembered(1);
    fell.add(sampleAt(1.05));
    expect(fell.prior()).toMatchObject({ isKept: false, droppedAtSample: 2 });
    expect((fell.estimateBps() ?? 0) / 1e6).toBeLessThan(1.2);
    fell.add(sampleAt(0.95));
    expect((fell.estimateBps() ?? 0) / 1e6).toBeLessThan(1.2);
    // Two at 753 kb/s, a link that really was slow then, drop it too.
    expect((fromRemembered(0.753, 0.753).estimateBps() ?? 0) / 1000).toBeCloseTo(753, -1);
  });

  it("is retired once the playback's own samples fill the window, and counts nothing without a sample of its own", () => {
    const retired = fromRemembered(10, 10, 10, 10, 10, 10);
    expect(retired.prior()?.isKept).toBe(false);
    expect(retired.prior()?.droppedAtSample).toBeUndefined();
    expect(retired.estimateBps()).toBeCloseTo(10_000_000, -1);
    // The prior alone is the centre, but no estimate the choices read.
    const unmeasured = fromRemembered();
    expect(unmeasured.estimateBps()).toBeUndefined();
    expect(Math.exp(unmeasured.centre() ?? 0)).toBeCloseTo(17_000_000, -1);
    expect(unmeasured.spread()).toBe(0.3);
    // One that counts as nothing, too old, is none.
    expect(new LinkEstimate({ kbps: 17_000, countsAsSamples: 0 }).prior()).toBeUndefined();
  });
});
