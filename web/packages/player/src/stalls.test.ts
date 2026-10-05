// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { stallAfterMs, watchStalls, type Stall, type WatchedMedia } from "./stalls.ts";

/** A stand-in element: its position and play state, and its events, as a test fires them. */
class FakeMedia {
  currentTime = 0;
  /* eslint-disable @typescript-eslint/naming-convention -- HTMLMediaElement's own names, which this stands in for */
  paused = false;
  seeking = false;
  /* eslint-enable @typescript-eslint/naming-convention -- the fake's own names follow */
  readonly listeners = new Map<string, () => void>();

  addEventListener(type: string, listener: () => void): void {
    this.listeners.set(type, listener);
  }

  removeEventListener(type: string): void {
    this.listeners.delete(type);
  }

  fire(type: string): void {
    this.listeners.get(type)?.();
  }
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("counting a player's stalls (D247 §2)", () => {
  it("counts a wait for data while it plays that lasts, and says where it was", () => {
    const media = new FakeMedia();
    const stalls: Stall[] = [];
    watchStalls(media as unknown as WatchedMedia, (stall) => stalls.push(stall));
    media.fire("playing");
    media.currentTime = 61.5;
    media.fire("waiting");
    vi.advanceTimersByTime(stallAfterMs);
    media.fire("playing");
    media.fire("waiting");
    vi.advanceTimersByTime(stallAfterMs);
    expect(stalls).toEqual([
      { count: 1, positionMs: 61_500 },
      { count: 2, positionMs: 61_500 },
    ]);
  });

  it("does not count a short wait, a seek's loading, a first load, or a paused player", () => {
    const media = new FakeMedia();
    const stalls: Stall[] = [];
    const stop = watchStalls(media as unknown as WatchedMedia, (stall) => stalls.push(stall));
    media.fire("waiting"); // the first load, before it has played
    vi.advanceTimersByTime(stallAfterMs);
    media.fire("playing");
    media.fire("waiting");
    vi.advanceTimersByTime(stallAfterMs / 2);
    media.fire("canplay"); // a short wait
    vi.advanceTimersByTime(stallAfterMs);
    media.fire("seeking");
    media.fire("waiting"); // the seek's loading
    vi.advanceTimersByTime(stallAfterMs);
    media.fire("playing");
    media.paused = true;
    media.fire("waiting");
    vi.advanceTimersByTime(stallAfterMs);
    expect(stalls).toEqual([]);
    media.paused = false;
    stop();
    media.fire("waiting");
    vi.advanceTimersByTime(stallAfterMs);
    expect(stalls).toEqual([]);
  });
});
