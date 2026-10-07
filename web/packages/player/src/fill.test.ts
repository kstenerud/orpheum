// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Hls, { PlaylistLevelType, type Fragment } from "hls.js";

import {
  fillStepDown,
  lookWhileFillingEveryMs,
  stepDownWhileFilling,
  type FillingHls,
} from "./fill.ts";

/** Bram's ladder on staging: four qualities, then its own picture, its peak 36 Mb/s. */
const levels = [
  { bitrate: 696_000, averageBitrate: 496_000, height: 360 },
  { bitrate: 1_896_000, averageBitrate: 1_296_000, height: 480 },
  { bitrate: 4_596_000, averageBitrate: 3_096_000, height: 720 },
  { bitrate: 36_208_176, averageBitrate: 5_117_192, height: 816 },
];

describe("stepping down while filling the first buffer (D260)", () => {
  it("goes on with a chunk arriving as fast as it plays, or not loading half its length yet", () => {
    const filling = { levels, loadingLevel: 3, durationS: 10 };
    // 5.2 Mb/s, faster than the picture's 5.1 average.
    expect(fillStepDown({ ...filling, loadingMs: 6000, loadedBytes: 3_900_000 })).toBeUndefined();
    // Slow, but only four seconds of a ten-second chunk.
    expect(fillStepDown({ ...filling, loadingMs: 4000, loadedBytes: 100_000 })).toBeUndefined();
  });

  it("moves to the highest quality within 70% of the speed a slow chunk arrived at", () => {
    // 1 Mb/s: 0.7 Mb/s carries the 360p quality only.
    expect(
      fillStepDown({
        levels,
        loadingLevel: 3,
        durationS: 10,
        loadingMs: 6000,
        loadedBytes: 750_000,
      }),
    ).toBe(0);
    // 2 Mb/s: 1.4 Mb/s carries 480p.
    expect(
      fillStepDown({
        levels,
        loadingLevel: 3,
        durationS: 10,
        loadingMs: 6000,
        loadedBytes: 1_500_000,
      }),
    ).toBe(1);
  });

  it("moves to the lowest when nothing has arrived, and never below it", () => {
    expect(
      fillStepDown({ levels, loadingLevel: 2, durationS: 10, loadingMs: 8000, loadedBytes: 0 }),
    ).toBe(0);
    expect(
      fillStepDown({ levels, loadingLevel: 0, durationS: 10, loadingMs: 8000, loadedBytes: 0 }),
    ).toBeUndefined();
  });
});

/** A stand-in for hls.js: the ladder, where it starts, and what was done to it. */
class FakeHls implements FillingHls {
  readonly levels = levels;
  readonly startPosition = 2400;
  autoLevelCapping = 3;
  nextAutoLevel = -1;
  readonly calls: string[] = [];
  readonly listeners = new Map<
    string,
    (event: string, data: { readonly frag: Fragment }) => void
  >();

  stopLoad(): void {
    this.calls.push("stopLoad");
  }

  startLoad(startPosition?: number): void {
    this.calls.push(`startLoad ${String(startPosition)}`);
  }

  on(event: string, listener: (event: string, data: { readonly frag: Fragment }) => void): void {
    this.listeners.set(event, listener);
  }

  fire(event: string, frag: Fragment): void {
    this.listeners.get(event)?.(event, { frag });
  }
}

/** A chunk of picture at level, ten seconds long, with loaded of its bytes arrived. */
function pictureChunk(level: number, loaded: number): Fragment {
  return {
    type: PlaylistLevelType.MAIN,
    level,
    duration: 10,
    stats: { loaded },
  } as unknown as Fragment;
}

let nowMs = 0;

beforeEach(() => {
  vi.useFakeTimers();
  nowMs = 0;
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a player filling its first buffer", () => {
  it("stops a slow chunk of its top quality and loads from where it starts at the quality the link carries", () => {
    const hls = new FakeHls();
    stepDownWhileFilling(hls, () => nowMs);
    const chunk = pictureChunk(3, 0);
    hls.fire(Hls.Events.FRAG_LOADING, chunk);
    (chunk.stats as { loaded: number }).loaded = 750_000; // 1 Mb/s over six seconds
    nowMs = 6000;
    vi.advanceTimersByTime(lookWhileFillingEveryMs);
    expect(hls.calls).toEqual(["stopLoad", "startLoad 2400"]);
    expect(hls.autoLevelCapping).toBe(0);
    expect(hls.nextAutoLevel).toBe(0);
  });

  it("leaves its choosing to hls.js once its first chunk has arrived", () => {
    const hls = new FakeHls();
    stepDownWhileFilling(hls, () => nowMs);
    hls.fire(Hls.Events.FRAG_LOADED, pictureChunk(3, 3_900_000));
    hls.fire(Hls.Events.FRAG_LOADING, pictureChunk(3, 0));
    nowMs = 8000;
    vi.advanceTimersByTime(lookWhileFillingEveryMs);
    expect(hls.calls).toEqual([]);
  });
});
