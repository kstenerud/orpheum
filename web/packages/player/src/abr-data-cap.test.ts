// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { underDataCap } from "./abr-choice.ts";
import { lookForStepUpEveryMs } from "./abr.ts";
import { rungsOf } from "./offered-rungs.ts";
import { choosingFrom, levels, loadAt, pastStarting, scene, useScene } from "./test-choosing.ts";

/**
 * The device's data cap (D488): a ceiling no choice of the chunked player's passes, at its start,
 * at each chunk and at the step-up, which never plays the film's own picture.
 */

useScene({ beforeEach, afterEach, vi });

/** The test ladder with its second quality the film's own picture, of a rate below the third's. */
const withOwnPictureLow = [
  { bitrate: 696_000, averageBitrate: 496_000, uri: "q400k360p/stream.m3u8" },
  { bitrate: 1_400_000, averageBitrate: 900_000, uri: "picture/stream.m3u8" },
  { bitrate: 1_896_000, averageBitrate: 1_296_000, uri: "q1200k480p/stream.m3u8" },
  { bitrate: 4_596_000, averageBitrate: 3_096_000, uri: "q3000k720p/stream.m3u8" },
];

/** Climbs as far as the link lets it: a fast link measured, a full buffer, the step-up's looks. */
function climb(chosen: ReturnType<typeof choosingFrom>, level: number): void {
  chosen.hls.currentLevel = level;
  chosen.hls.mainForwardBufferInfo = { len: 60 };
  for (let look = 0; look < 30; look += 1) {
    loadAt(chosen.choosing, chosen.hls.currentLevel, { mbps: 40 });
    scene.nowMs += 60_000;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    chosen.hls.currentLevel = chosen.choosing.nextLevel();
  }
}

describe("the device's data cap (D488)", () => {
  it("starts under it: the first chunk is of the highest quality whose rate, its sound's with it, is within it", () => {
    scene.dataCapKbps = 1400; // 480p's 1296 with its sound
    const { choosing } = choosingFrom(40_000_000);
    expect(choosing.firstLevel()).toBe(1);
    expect(choosing.topLevel()).toBe(1);
  });

  it("is the lowest quality where it is under every one", () => {
    scene.dataCapKbps = 100;
    const { choosing } = choosingFrom(40_000_000);
    expect(choosing.firstLevel()).toBe(0);
  });

  it("never lets the step-up past it, however fast the link", () => {
    scene.dataCapKbps = 1400;
    const chosen = choosingFrom(40_000_000);
    pastStarting(chosen, 1, 40);
    climb(chosen, 1);
    expect(chosen.choosing.nextLevel()).toBe(1);
  });

  it("takes the next chunk down when it is chosen mid-film, and climbs back by the step-up once lifted", () => {
    const chosen = choosingFrom(40_000_000);
    pastStarting(chosen, 3, 40);
    expect(chosen.choosing.nextLevel()).toBe(3);
    scene.dataCapKbps = 600; // 360p's 496 with its sound, as on moving to mobile data
    expect(chosen.choosing.nextLevel()).toBe(0);
    expect(scene.steps.at(-1)).toBe("ceiling 3 to 0: the device's data cap, 600 kb/s");
    scene.dataCapKbps = undefined;
    // Lifted, the ceiling stays where the cap set it, and the step-up climbs from there.
    expect(chosen.choosing.nextLevel()).toBe(0);
    climb(chosen, 0);
    expect(chosen.choosing.nextLevel()).toBe(3);
    expect(scene.steps.at(-1)).toMatch(/^ceiling 0 to 3: stepped up; /u);
  });

  it("never plays the film's own picture under a cap, its rate within it or not", () => {
    const uncapped = choosingFrom(40_000_000);
    uncapped.hls.levels = withOwnPictureLow;
    expect(underDataCap(withOwnPictureLow, 1, undefined)).toBe(1);
    scene.dataCapKbps = 1000; // the own picture's 900 is within it; 360p is the rung that is
    const capped = choosingFrom(40_000_000);
    capped.hls.levels = withOwnPictureLow;
    expect(capped.choosing.firstLevel()).toBe(0);
    scene.isInAGroup = true; // a group keeps the film's own picture where its link carries it (D286 §2)
    loadAt(capped.choosing, 0, { mbps: 40 });
    expect(capped.choosing.nextLevel()).toBe(0);
    expect(underDataCap(withOwnPictureLow, 3, 100)).toBe(0);
    const ownPictureLowest = [
      { bitrate: 400_000, averageBitrate: 300_000, uri: "picture/stream.m3u8" },
      ...levels.slice(1),
    ];
    expect(underDataCap(ownPictureLowest, 0, 100)).toBe(1);
  });
});

describe("the rungs offered for a cap (D488)", () => {
  it("are the qualities the core made, each with its sound, lowest first, the film's own picture left out", () => {
    expect(rungsOf([...withOwnPictureLow].reverse())).toEqual([496, 1296, 3096]);
    expect(rungsOf(levels)).toEqual([496, 1296, 3096, 5117]);
  });
});
