// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  estimateText,
  assumedLogSpread,
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

  it("is not ruled by one sample that is not the link's: Nosferatu's four near 15 Mb/s and one at 14 kb/s", () => {
    // The harmonic mean made these 69 kb/s, and a Fire TV fell from Nosferatu's own picture to its
    // lowest quality for 75 s (staging, 2026-10-28; bandwidth-sampling-2026-10-29.md §1.1).
    const nosferatu = estimateOf(14, 18, 15, 16, 0.014);
    expect(nosferatu.estimateBps()).toBeGreaterThan(10_000_000);
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

  it("is discounted by the largest error of its own predictions: the confident estimate", () => {
    const steady = estimateOf(10, 10, 10, 10, 10);
    expect(steady.predictionError()).toBeCloseTo(0, 6);
    expect(steady.confidentBps()).toBeCloseTo(10_000_000, -1);
    // 8 after an estimate of 10: the prediction was a quarter of the sample out.
    const fell = estimateOf(10, 8);
    expect(fell.predictionError()).toBeCloseTo(0.25, 6);
    expect(fell.confidentBps()).toBeCloseTo((Math.sqrt(10 * 8) * 1e6) / 1.25, -1);
  });

  it("is the sample itself before any prediction, and nothing before any sample", () => {
    expect(new LinkEstimate().estimateBps()).toBeUndefined();
    expect(new LinkEstimate().confidentBps()).toBeUndefined();
    expect(estimateOf(5).confidentBps()).toBeCloseTo(5_000_000, -1);
    expect(estimateOf(5).predictionError()).toBeUndefined();
  });

  it("agrees when its predictions erred at most a fifth, and not before it has predicted", () => {
    expect(estimateOf(10).agrees()).toBe(false);
    expect(estimateOf(10, 10 / (1 + samplesAgreeWithinError) + 0.01, 10).agrees()).toBe(true);
    expect(estimateOf(10, 7, 10).agrees()).toBe(false);
  });

  it("a varying link is targeted near its slowest samples", () => {
    const varying = estimateOf(4, 12, 3, 11);
    const mean = (4 + 12 + 3 + 11) / 4;
    expect((varying.confidentBps() ?? 0) / 1e6).toBeLessThan(3);
    expect((varying.confidentBps() ?? 0) / 1e6).toBeLessThan(mean / 2);
  });

  it("is not ruled by one slow sample of five: the most extreme error is set aside", () => {
    // A Fire TV's five samples as its stall report gave them (staging, Nosferatu, 2026-10-28).
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
    expect((fireTv.confidentBps() ?? 0) / 1000).toBeGreaterThan(4000); // not 1459 kb/s
    // Two slow samples of five are a link that varies, and still count.
    expect(estimateOf(12, 12, 2, 12, 2).predictionError()).toBeGreaterThan(1); // the second slow one's
    // Fewer than five kept: every error counts.
    expect(estimateOf(12, 12, 12, 2).predictionError()).toBeCloseTo(5, 6);
  });

  it("says itself and its samples in a line for the log (D352 §5)", () => {
    expect(estimateText(new LinkEstimate())).toBe("unmeasured");
    expect(estimateText(estimateOf(10, 8))).toBe(
      "confident 7155 kb/s (estimate 8944, error 25%, 2 samples)",
    );
    expect(samplesText([])).toBe("");
    expect(samplesText(estimateOf(10, 8).samples())).toBe(
      "samples 3 1000000B 800ms fb40ms, 3 1000000B 1000ms fb40ms",
    );
  });
});
