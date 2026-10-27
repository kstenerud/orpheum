// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  estimateText,
  harmonicMean,
  LinkEstimate,
  linkSamplesKept,
  samplesAgreeWithinError,
  samplesText,
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

describe("the link's estimate (D352 §2)", () => {
  it("is the harmonic mean of the last five samples, each counted once whatever its length", () => {
    expect(harmonicMean([4_000_000, 12_000_000])).toBeCloseTo(6_000_000, -1);
    const estimate = estimateOf(1, 12, 12, 12, 12);
    expect(estimate.estimateBps()).toBeCloseTo((5 / (1 + 4 / 12)) * 1_000_000, -1);
    // Five chunks later the slow one has left: the estimate is the link's.
    for (let index = 0; index < linkSamplesKept; index += 1) {
      estimate.add(sampleAt(12));
    }
    expect(estimate.estimateBps()).toBeCloseTo(12_000_000, -1);
    expect(estimate.kept()).toBe(linkSamplesKept);
    expect(estimate.count()).toBe(10);
  });

  it("falls most of the way at one slow sample", () => {
    expect(estimateOf(12, 12, 12, 12, 1).estimateBps()).toBeLessThan(4_000_000);
  });

  it("is discounted by the largest error of its own predictions: the confident estimate", () => {
    const steady = estimateOf(10, 10, 10, 10, 10);
    expect(steady.largestError()).toBeCloseTo(0, 6);
    expect(steady.confidentBps()).toBeCloseTo(10_000_000, -1);
    // 8 after an estimate of 10: the prediction was a quarter of the sample out.
    const fell = estimateOf(10, 8);
    expect(fell.largestError()).toBeCloseTo(0.25, 6);
    expect(fell.confidentBps()).toBeCloseTo(((2 / (1 / 10 + 1 / 8)) * 1e6) / 1.25, -1);
  });

  it("is the sample itself before any prediction, and nothing before any sample", () => {
    expect(new LinkEstimate().estimateBps()).toBeUndefined();
    expect(new LinkEstimate().confidentBps()).toBeUndefined();
    expect(estimateOf(5).confidentBps()).toBeCloseTo(5_000_000, -1);
    expect(estimateOf(5).largestError()).toBeUndefined();
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

  it("says itself and its samples in a line for the log (D352 §5)", () => {
    expect(estimateText(new LinkEstimate())).toBe("unmeasured");
    expect(estimateText(estimateOf(10, 8))).toBe(
      "confident 7111 kb/s (mean 8889, error 25%, 2 samples)",
    );
    expect(samplesText([])).toBe("");
    expect(samplesText(estimateOf(10, 8).samples())).toBe(
      "samples 3 1000000B 800ms fb40ms, 3 1000000B 1000ms fb40ms",
    );
  });
});
