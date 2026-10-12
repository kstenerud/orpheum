// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  standsStillReportAfterMs,
  watchStandingStill,
  type StandingMedia,
  type Standstill,
} from "./standstill.ts";

/** A stand-in video: what a stand-still reads of it, as a test sets it. */
function fakeVideo(): { -readonly [Key in keyof StandingMedia]: StandingMedia[Key] } {
  return {
    currentTime: 0,
    paused: false,
    seeking: false,
    ended: false,
    readyState: 4,
    networkState: 1,
    buffered: { length: 1, start: () => 0, end: () => 31.2 },
    error: null,
    videoWidth: 0,
    videoHeight: 0,
    muted: false,
    volume: 1,
    getVideoPlaybackQuality: () => ({ totalVideoFrames: 0, droppedVideoFrames: 0 }),
  };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a video standing still while asked to play", () => {
  it("is told once where it stands, with what it showed: a phone's Brave stood at 0:00 'playing'", () => {
    const video = fakeVideo();
    const told: Standstill[] = [];
    watchStandingStill(
      video,
      (still) => told.push(still),
      () => false,
    );
    vi.advanceTimersByTime(standsStillReportAfterMs + 10_000);
    expect(told).toEqual([
      {
        positionMs: 0,
        forMs: standsStillReportAfterMs,
        readyState: 4,
        networkState: 1,
        buffered: "0.00-31.20",
        lastError: "",
        videoWidth: 0,
        videoHeight: 0,
        decodedFrames: 0,
        droppedFrames: 0,
        isMuted: false,
        volume: 1,
        isPageHidden: false,
      },
    ]);
  });

  it("is told again where it stands next, and never while paused, seeking or moving", () => {
    const video = fakeVideo();
    const told: number[] = [];
    const stop = watchStandingStill(
      video,
      (still) => told.push(still.positionMs),
      () => false,
    );
    video.paused = true;
    vi.advanceTimersByTime(standsStillReportAfterMs * 2);
    video.paused = false;
    video.seeking = true;
    vi.advanceTimersByTime(standsStillReportAfterMs * 2);
    video.seeking = false;
    for (let second = 0; second < 10; second++) {
      video.currentTime += 1;
      vi.advanceTimersByTime(1000);
    }
    expect(told).toEqual([]);
    vi.advanceTimersByTime(standsStillReportAfterMs + 1000);
    video.currentTime = 42;
    vi.advanceTimersByTime(standsStillReportAfterMs + 1000);
    stop();
    video.currentTime = 50;
    vi.advanceTimersByTime(standsStillReportAfterMs * 2);
    expect(told).toEqual([10_000, 42_000]);
  });
});
