// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { choosingFrom, loadAt, scene, useScene } from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

/**
 * A Bond film's ladder on staging: nothing between 2.5 and 12.2 Mb/s, its top the film's own
 * picture, whose peak is capped (BANDWIDTH) far above its average.
 */
const bondLevels = [
  { bitrate: 896_000, averageBitrate: 700_000 },
  { bitrate: 2_696_000, averageBitrate: 2_100_000 },
  { bitrate: 12_205_000, averageBitrate: 7_800_000 },
];

/** The server's start: the highest BANDWIDTH within 70% of the remembered link (D257 §2). */
const serversStartBps = 2_696_000;

/** The link the Fire TV remembered on its network as A Road to a Storm began, kbit/s. */
const rememberedKbps = 11_923;

// On the Fire TV, A Road to a Storm and A Harbour Carol showed black frames at their start: the
// server's low first rung switched to the top as playback began, and Silk's decoder, reset by the new
// init, blinked (D381, amending D257 §2).
describe("the quality a chunked playback starts on (D381)", () => {
  it("starts on the quality the remembered link carries by the rule it plays by, and stays there: no switch as it plays", () => {
    const chosen = choosingFrom(serversStartBps, undefined, {
      rememberedLink: { kbps: rememberedKbps, countsAsSamples: 2 },
      shouldStartByRememberedLink: true,
    });
    chosen.hls.levels = bondLevels;
    expect(chosen.choosing.firstLevel()).toBe(2);
    expect(chosen.choosing.nextLevel()).toBe(2);
    loadAt(chosen.choosing, 2, { mbps: 12 });
    expect(chosen.choosing.nextLevel()).toBe(2);
    expect(chosen.choosing.whyChose(2)?.why).toBe("first by the remembered link, 11923 kb/s");
    expect(scene.steps.filter((step) => step.startsWith("ceiling"))).toEqual([]);
  });

  it("starts where the server chose for a player that remembers nothing, and where the page gave its own start", () => {
    const forgetting = choosingFrom(serversStartBps, undefined, {
      shouldStartByRememberedLink: true,
    });
    forgetting.hls.levels = bondLevels;
    expect(forgetting.choosing.firstLevel()).toBe(1);
    // A stalled player's lower quality, or a warm-up's settled one, is the page's own start.
    const given = choosingFrom(serversStartBps, undefined, {
      rememberedLink: { kbps: rememberedKbps, countsAsSamples: 2 },
    });
    given.hls.levels = bondLevels;
    expect(given.choosing.firstLevel()).toBe(1);
  });

  it("starts low where the remembered link is slow, by the same share of it", () => {
    const chosen = choosingFrom(serversStartBps, undefined, {
      rememberedLink: { kbps: 2000, countsAsSamples: 2 },
      shouldStartByRememberedLink: true,
    });
    chosen.hls.levels = bondLevels;
    expect(chosen.choosing.firstLevel()).toBe(0); // 70% of 2 Mb/s carries 896k's average alone
  });
});
