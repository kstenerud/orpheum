// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { playerMovedItselfEvent } from "@orpheum/sdk";

import { holeEndAt, stepOverHoles, type HoleMedia } from "./holes.ts";

/** Ranges as a video's buffered gives them. */
function ranges(...spans: readonly (readonly [number, number])[]): TimeRanges {
  return {
    length: spans.length,
    start: (index: number) => spans[index]?.[0] ?? Number.NaN,
    end: (index: number) => spans[index]?.[1] ?? Number.NaN,
  };
}

/** A video asked to play, standing at currentTime with buffered loaded, and what it was told. */
function standing(
  currentTime: number,
  buffered: TimeRanges,
): HoleMedia & { readonly told: string[] } {
  const told: string[] = [];
  return {
    currentTime,
    paused: false,
    seeking: false,
    ended: false,
    buffered,
    dispatchEvent: (event: Event) => {
      told.push(`${event.type}:${String((event as CustomEvent).detail)}`);
      return true;
    },
    told,
  };
}

describe("a hole in what a video has loaded, where it stands", () => {
  it("ends at the next loaded range's start, where nothing is loaded and that is near", () => {
    expect(holeEndAt(ranges([190, 200.66], [200.78, 230]), 200.66)).toBe(200.78);
    expect(holeEndAt(ranges([200.78, 230]), 200.7)).toBe(200.78);
  });

  it("is none where it stands in what is loaded, nor where the next is a stretch to load", () => {
    expect(holeEndAt(ranges([190, 200.66], [200.78, 230]), 195)).toBeUndefined();
    expect(holeEndAt(ranges([190, 200.66], [202, 230]), 200.66)).toBeUndefined();
    expect(holeEndAt(ranges([190, 200.66]), 200.66)).toBeUndefined();
    expect(holeEndAt(ranges(), 0)).toBeUndefined();
  });
});

describe("stepOverHoles", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  // The owner's Fire TV, 2026-11-11: after the browser's bufferFullError and its flush, the video
  // stood at 200.66 s, its next picture at 200.78 s, and hls.js never stepped it on.
  it("steps a video standing still in a hole over it, as its own move, and notes it", () => {
    const video = standing(200.66, ranges([190, 200.66], [200.78, 230]));
    const notes: string[] = [];
    const stop = stepOverHoles(video, (line) => notes.push(line));
    vi.advanceTimersByTime(1500);
    stop();
    expect(video.currentTime).toBeCloseTo(200.88, 5);
    expect(video.told).toEqual([`${playerMovedItselfEvent}:hole`]);
    expect(notes).toEqual(["stepped over a 0.12 s hole at 200.66 s, stood still in it"]);
  });

  it("leaves hls.js its own second: not stepped after one look", () => {
    const video = standing(200.66, ranges([190, 200.66], [200.78, 230]));
    const stop = stepOverHoles(video, () => undefined);
    vi.advanceTimersByTime(1000);
    stop();
    expect(video.currentTime).toBe(200.66);
  });

  it("never steps a paused video, a seeking one, an ended one, or one that plays on", () => {
    for (const state of [{ paused: true }, { seeking: true }, { ended: true }]) {
      const video = { ...standing(200.66, ranges([190, 200.66], [200.78, 230])), ...state };
      const stop = stepOverHoles(video, () => undefined);
      vi.advanceTimersByTime(5000);
      stop();
      expect(video.currentTime).toBe(200.66);
    }
    const playing = standing(200.66, ranges([190, 200.66], [200.78, 230]));
    const stop = stepOverHoles(playing, () => undefined);
    for (let look = 0; look < 10; look++) {
      playing.currentTime = 195 + look * 0.5;
      vi.advanceTimersByTime(500);
    }
    stop();
    expect(playing.told).toEqual([]);
  });

  it("does no more once stopped", () => {
    const video = standing(200.66, ranges([190, 200.66], [200.78, 230]));
    stepOverHoles(video, () => undefined)();
    vi.advanceTimersByTime(5000);
    expect(video.currentTime).toBe(200.66);
  });
});
