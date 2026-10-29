// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { playbackStartSlowAfterMs, timePlaybackStart } from "./play-start-timing.ts";

/** A clock a test moves by hand, in ms. */
function clockOf(startMs = 0): { readonly nowMs: () => number; advance: (ms: number) => void } {
  let now = startMs;
  return {
    nowMs: () => now,
    advance: (ms) => {
      now += ms;
    },
  };
}

describe("timing the Play page's playback start", () => {
  it("tells nothing for an ordinary, fast one", () => {
    const clock = clockOf();
    const sent: string[] = [];
    const timing = timePlaybackStart({
      fileId: 5682,
      carrierOf: () => "http",
      report: (text) => sent.push(text),
      nowMs: clock.nowMs,
    });
    timing.onAttempt({ ms: 40, outcome: "ok" });
    clock.advance(40);
    timing.finish();
    expect(sent).toEqual([]);
  });

  it("tells the core's log each attempt's time and outcome, and the carrier, once it is slow", () => {
    const clock = clockOf();
    const sent: string[] = [];
    const timing = timePlaybackStart({
      fileId: 5682,
      carrierOf: () => "over WebRTC",
      report: (text) => sent.push(text),
      nowMs: clock.nowMs,
    });
    clock.advance(1200);
    timing.onAttempt({
      ms: 1200,
      outcome: "SilentRequestError: nothing came for /api/v1/playback in 40000 ms",
    });
    clock.advance(49000);
    timing.onAttempt({ ms: 49000, outcome: "ok" });
    timing.finish();
    expect(sent).toHaveLength(1);
    const [text] = sent;
    expect(text).toContain("the Play page's playback for file 5682 took 50200 ms to start");
    expect(text).toContain("carried over WebRTC");
    expect(text).toContain(
      "attempt 1: 1200 ms (SilentRequestError: nothing came for /api/v1/playback in 40000 ms)",
    );
    expect(text).toContain("attempt 2: 49000 ms (ok)");
  });

  it("is exactly the threshold the owner's near-minute wait (VQ137) is caught well before", () => {
    expect(playbackStartSlowAfterMs).toBeLessThan(10_000);
  });
});
