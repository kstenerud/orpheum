// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { framesTimed, timeFrames, type FrameTiming } from "./frame-timing.ts";

/** A playing video whose frames the test hands it, each as the browser would say of it. */
function framedVideo(): {
  readonly video: HTMLVideoElement;
  readonly show: (frame: {
    nowMs: number;
    displayMs: number;
    presentedMs: number;
    mediaS: number;
    currentS: number;
  }) => void;
} {
  let next: VideoFrameRequestCallback | undefined;
  const video = {
    paused: false,
    seeking: false,
    currentTime: 0,
    requestVideoFrameCallback: (callback: VideoFrameRequestCallback) => {
      next = callback;
      return 1;
    },
  };
  return {
    video: video as unknown as HTMLVideoElement,
    show: (frame) => {
      video.currentTime = frame.currentS;
      const callback = next;
      next = undefined;
      callback?.(frame.nowMs, {
        expectedDisplayTime: frame.displayMs,
        presentationTime: frame.presentedMs,
        mediaTime: frame.mediaS,
        presentedFrames: 1,
        width: 1280,
        height: 720,
      });
    },
  };
}

describe("how late a video's picture reaches the screen (the owner's TCL TV, 2026-10-24)", () => {
  it("tells the middle of each framesTimed frames: on screen after its handing over, behind the sound", () => {
    const { video, show } = framedVideo();
    const told: (FrameTiming | undefined)[] = [];
    timeFrames(video, (timing) => told.push(timing));
    for (let index = 0; index < framesTimed; index += 1) {
      const nowMs = 1000 + index * 40;
      // On screen 16 ms after it was handed over; the frame is of 0.5 s before the sound playing.
      show({
        nowMs,
        displayMs: nowMs + 16,
        presentedMs: nowMs,
        mediaS: index * 0.04,
        currentS: index * 0.04 + 0.484,
      });
    }
    expect(told).toEqual([{ frames: framesTimed, displayAfterMs: 16, behindSoundMs: 500 }]);
  });

  it("says a browser that cannot say, once", () => {
    const told: (FrameTiming | undefined)[] = [];
    timeFrames({} as HTMLVideoElement, (timing) => told.push(timing));
    expect(told).toEqual([undefined]);
  });
});
