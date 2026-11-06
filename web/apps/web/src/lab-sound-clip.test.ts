// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  soundTimingBeeps,
  soundTimingClipMs,
  soundTimingFlashMs,
  timingPictureAt,
} from "./lab-sound-clip.ts";

describe("the sound timing test's clip (the owner's design, 2026-10-26)", () => {
  it("beeps at the start of each of its four seconds, each higher than the last", () => {
    expect(soundTimingClipMs).toBe(4000);
    expect(soundTimingBeeps.map((beep) => beep.atMs)).toEqual([0, 1000, 2000, 3000]);
    const pitches = soundTimingBeeps.map((beep) => beep.frequencyHz);
    expect(pitches).toEqual(pitches.toSorted((first, second) => first - second));
    expect(new Set(pitches).size).toBe(4);
    for (const beep of soundTimingBeeps) {
      expect(beep.lastsMs).toBe(soundTimingFlashMs);
    }
  });

  it("flashes the second's number as its beep starts, and fills the bar across the second", () => {
    expect(timingPictureAt(0, 0)).toEqual({ beat: 1, isFlash: true, fill: 0 });
    expect(timingPictureAt(1050, 0)).toEqual({ beat: 2, isFlash: true, fill: 0.05 });
    expect(timingPictureAt(1500, 0)).toEqual({ beat: 2, isFlash: false, fill: 0.5 });
    expect(timingPictureAt(3999, 0).beat).toBe(4);
    // Looped: the clip's sound starts again at 0, and its picture with it.
    expect(timingPictureAt(4000, 0)).toEqual({ beat: 1, isFlash: true, fill: 0 });
  });

  it("holds the picture back by as much as the player moves the sound earlier", () => {
    // The second beep sounds at 1000 ms; its flash comes 300 ms after.
    expect(timingPictureAt(1000, 300)).toEqual({ beat: 1, isFlash: false, fill: 0.7 });
    expect(timingPictureAt(1300, 300)).toEqual({ beat: 2, isFlash: true, fill: 0 });
    // Before the clip's first second ends, the picture is still in the loop before.
    expect(timingPictureAt(100, 300)).toEqual({ beat: 4, isFlash: false, fill: 0.8 });
  });
});
