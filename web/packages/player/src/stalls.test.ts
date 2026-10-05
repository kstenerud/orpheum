// SPDX-License-Identifier: MPL-2.0

import { fellBehindEvent } from "@orpheum/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  slowLoadAfterMs,
  stallAfterMs,
  watchStalls,
  type Stall,
  type WatchedMedia,
} from "./stalls.ts";

/** A stand-in element: its position and play state, and its events, as a test fires them. */
class FakeMedia {
  currentTime = 0;
  /* eslint-disable @typescript-eslint/naming-convention -- HTMLMediaElement's own names, which this stands in for */
  paused = false;
  seeking = false;
  readyState = 4;
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

  it("counts a load that leaves it unable to play for long, each time as long, and not a quick one", () => {
    const media = new FakeMedia();
    media.readyState = 1;
    const stalls: Stall[] = [];
    watchStalls(media as unknown as WatchedMedia, (stall) => stalls.push(stall));
    vi.advanceTimersByTime(slowLoadAfterMs - 1);
    media.readyState = 4;
    media.fire("canplay"); // the first load, just in time
    media.paused = true; // held while a Watch Together group waits, and sought to its place
    media.currentTime = 371;
    media.readyState = 1;
    media.fire("seeking");
    vi.advanceTimersByTime(slowLoadAfterMs * 2);
    expect(stalls).toEqual([
      { count: 1, positionMs: 371_000 },
      { count: 2, positionMs: 371_000 },
    ]);
    media.readyState = 4;
    media.fire("canplay");
    vi.advanceTimersByTime(slowLoadAfterMs);
    expect(stalls).toHaveLength(2);
  });

  it("counts a load as long as it cannot play, through the seeks of a player catching up", () => {
    const media = new FakeMedia();
    const stalls: Stall[] = [];
    watchStalls(media as unknown as WatchedMedia, (stall) => stalls.push(stall));
    media.readyState = 1;
    for (const positionS of [100, 108, 116]) {
      media.currentTime = positionS;
      media.fire("seeking");
      vi.advanceTimersByTime(slowLoadAfterMs / 3);
    }
    expect(stalls).toEqual([{ count: 1, positionMs: 116_000 }]);
  });

  it("counts each time a Watch Together player falls behind its group", () => {
    const media = new FakeMedia();
    const stalls: Stall[] = [];
    watchStalls(media as unknown as WatchedMedia, (stall) => stalls.push(stall));
    media.currentTime = 200;
    media.fire(fellBehindEvent);
    expect(stalls).toEqual([{ count: 1, positionMs: 200_000 }]);
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
