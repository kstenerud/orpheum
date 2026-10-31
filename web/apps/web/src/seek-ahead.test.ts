// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  heldKeyRestMs,
  heldStepMs,
  landedWithinMs,
  landingShownForMs,
  SeekAhead,
  seekAskedEvent,
  seekRestMs,
  seekStepMs,
  soughtByOf,
} from "./seek-ahead.ts";
import { answerPlayerCommand } from "./player-commands.ts";

/** A SeekAhead on a clock the test moves, its seeks and its changes counted. */
function aheadOnClock(): {
  readonly ahead: SeekAhead;
  readonly seeks: number[];
  readonly keys: EventTarget;
  /** Moves the clock and the timers on by ms. */
  readonly pass: (ms: number) => void;
  readonly changes: () => number;
} {
  let nowMs = 0;
  let changes = 0;
  const seeks: number[] = [];
  const keys = new EventTarget();
  const ahead = new SeekAhead({
    seek: (atMs) => seeks.push(atMs),
    changed: () => {
      changes += 1;
    },
    keys,
    now: () => nowMs,
  });
  return {
    ahead,
    seeks,
    keys,
    pass: (ms) => {
      nowMs += ms;
      vi.advanceTimersByTime(ms);
    },
    changes: () => changes,
  };
}

describe("a seek chosen before it is made (D369 §1; the owner, Fire TV, 2026-10-29)", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("moves the place shown at each press of a held key without seeking, and seeks once, there, after its release", () => {
    const { ahead, seeks, keys, pass, changes } = aheadOnClock();
    for (let press = 0; press < 5; press += 1) {
      ahead.step(1, { atMs: 30_000, mostMs: 600_000 }, { isKey: true });
      pass(50); // a remote's held key repeats
    }
    expect(ahead.shownMs(30_250)).toBe(30_000 + 5 * seekStepMs);
    expect(changes()).toBe(5);
    pass(heldKeyRestMs - 300); // a pause in the repeats, the key still down
    expect(seeks).toEqual([]);
    keys.dispatchEvent(new Event("keyup"));
    pass(seekRestMs - 1);
    expect(seeks).toEqual([]);
    pass(1);
    expect(seeks).toEqual([80_000]);
  });

  it("seeks once repeated presses rest, as a controller's that has no release", () => {
    const { ahead, seeks, pass } = aheadOnClock();
    ahead.step(-1, { atMs: 300_000, mostMs: 600_000 }, { isKey: false });
    pass(seekRestMs - 100);
    ahead.step(-1, { atMs: 300_000, mostMs: 600_000 }, { isKey: false });
    pass(seekRestMs - 1);
    expect(seeks).toEqual([]);
    pass(1);
    expect(seeks).toEqual([300_000 - 2 * seekStepMs]);
  });

  it("seeks anyway when a held key's release is never heard", () => {
    const { ahead, seeks, pass } = aheadOnClock();
    ahead.step(1, { atMs: 0, mostMs: 600_000 }, { isKey: true });
    pass(heldKeyRestMs);
    expect(seeks).toEqual([seekStepMs]);
  });

  it("steps further the longer the presses go on: 10 s, then 30 s from 2 s, then 60 s from 4 s", () => {
    expect([0, 1999, 2000, 3999, 4000, 60_000].map(heldStepMs)).toEqual([
      10_000, 10_000, 30_000, 30_000, 60_000, 60_000,
    ]);
    const { ahead, seeks, pass } = aheadOnClock();
    for (let press = 0; press < 50; press += 1) {
      ahead.step(1, { atMs: 0, mostMs: 3_600_000 }, { isKey: false });
      pass(100);
    }
    // 20 presses of 10 s, 20 of 30 s, 10 of 60 s, made as one seek.
    pass(seekRestMs);
    expect(seeks).toEqual([20 * 10_000 + 20 * 30_000 + 10 * 60_000]);
    ahead.step(1, { atMs: seeks[0] ?? 0, mostMs: 3_600_000 }, { isKey: false });
    expect(ahead.shownMs(seeks[0] ?? 0)).toBe((seeks[0] ?? 0) + seekStepMs); // a new run starts slow
  });

  it("never goes before the start nor past the end", () => {
    const { ahead, pass, seeks } = aheadOnClock();
    ahead.step(-1, { atMs: 4000, mostMs: 20_000 }, { isKey: false });
    expect(ahead.shownMs(4000)).toBe(0);
    pass(seekRestMs);
    ahead.landed();
    ahead.step(1, { atMs: 15_000, mostMs: 20_000 }, { isKey: false });
    expect(ahead.shownMs(15_000)).toBe(20_000);
    pass(seekRestMs);
    expect(seeks).toEqual([0, 20_000]);
  });

  it("shows the place a seek was sent to until the film arrives there, or a while has passed", () => {
    const { ahead, seeks, pass } = aheadOnClock();
    ahead.aim(300_000, 0);
    expect(seeks).toEqual([300_000]); // at once: a click on a video's timeline
    expect(ahead.shownMs(60_000)).toBe(300_000); // the film not there yet
    pass(landingShownForMs - 1);
    expect(ahead.shownMs(61_000)).toBe(300_000);
    expect(ahead.shownMs(300_000 + landedWithinMs)).toBe(300_000 + landedWithinMs); // arrived
    expect(ahead.shownMs(61_000)).toBe(61_000); // and its place is the film's from then on
    ahead.aim(500_000, 0);
    pass(landingShownForMs);
    expect(ahead.shownMs(61_000)).toBe(61_000); // never arrived: the film's place
  });

  it("shows the film's own place once told its seek landed, and makes a waiting seek at play or pause", () => {
    const { ahead, seeks } = aheadOnClock();
    ahead.aim(300_000, 0);
    ahead.landed();
    expect(ahead.shownMs(290_000)).toBe(290_000);
    ahead.step(1, { atMs: 290_000, mostMs: 600_000 }, { isKey: false });
    ahead.land();
    expect(seeks).toEqual([300_000, 300_000]);
  });

  it("tells the video what asked each seek, just before it is made: the command and where it came from", () => {
    // A cast's receiver on a Fire TV stepped back 10 s twice, a step no one meant, and the log
    // could not say what asked it (staging, 2026-10-29, file 5680).
    const video = document.createElement("video");
    Object.defineProperty(video, "duration", { value: 600 });
    video.currentTime = 100;
    const asked: { by: string | undefined; atS: number }[] = [];
    video.addEventListener(seekAskedEvent, (event) => {
      asked.push({ by: soughtByOf(event), atS: video.currentTime });
    });
    answerPlayerCommand(video, "seekBack", "media");
    expect(asked).toEqual([]); // not before the presses rest
    vi.advanceTimersByTime(seekRestMs);
    expect(asked).toEqual([{ by: "seekBack-media", atS: 100 }]);
    expect(video.currentTime).toBe(90);
    expect(soughtByOf(new Event(seekAskedEvent))).toBeUndefined();
  });
});
