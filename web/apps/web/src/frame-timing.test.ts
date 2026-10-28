// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { framesTimed, timeFrames, type FrameTiming } from "./frame-timing.ts";

/**
 * A playing video whose frames the test hands it, each as the browser would say of it, and whether
 * a frame's callback is asked for now.
 */
function framedVideo(): {
  readonly video: HTMLVideoElement;
  readonly isFrameAsked: () => boolean;
  readonly show: (frame: {
    nowMs: number;
    displayMs: number;
    presentedMs: number;
    mediaS: number;
    currentS: number;
  }) => void;
} {
  let next: VideoFrameRequestCallback | undefined;
  const video = Object.assign(new EventTarget(), {
    paused: false,
    seeking: false,
    currentTime: 0,
    requestVideoFrameCallback: (callback: VideoFrameRequestCallback) => {
      next = callback;
      return 1;
    },
  });
  return {
    video: video as unknown as HTMLVideoElement,
    isFrameAsked: () => next !== undefined,
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

/** Shows framesTimed frames, each on screen 16 ms after its handing over, 0.5 s behind its sound. */
function showASample(show: ReturnType<typeof framedVideo>["show"]): void {
  for (let index = 0; index < framesTimed; index += 1) {
    const nowMs = 1000 + index * 40;
    show({
      nowMs,
      displayMs: nowMs + 16,
      presentedMs: nowMs,
      mediaS: index * 0.04,
      currentS: index * 0.04 + 0.484,
    });
  }
}

describe("how late a video's picture reaches the screen (the owner's TCL TV, 2026-10-24)", () => {
  it("tells the middle of each framesTimed frames: on screen after its handing over, behind the sound", () => {
    const { video, show } = framedVideo();
    const told: (FrameTiming | undefined)[] = [];
    timeFrames(video, (timing) => told.push(timing));
    showASample(show);
    expect(told).toEqual([{ frames: framesTimed, displayAfterMs: 16, behindSoundMs: 500 }]);
  });

  it("times one sample at each start of play, asking for no frame between (the efficiency audit, finding 2)", () => {
    const { video, show, isFrameAsked } = framedVideo();
    const told: (FrameTiming | undefined)[] = [];
    timeFrames(video, (timing) => told.push(timing));
    showASample(show);
    // The film plays on with no callback on its frames.
    expect(isFrameAsked()).toBe(false);
    video.dispatchEvent(new Event("playing"));
    expect(isFrameAsked()).toBe(true);
    showASample(show);
    expect(isFrameAsked()).toBe(false);
    expect(told).toHaveLength(2);
  });

  it("says a browser that cannot say, once", () => {
    const told: (FrameTiming | undefined)[] = [];
    timeFrames({} as HTMLVideoElement, (timing) => told.push(timing));
    expect(told).toEqual([undefined]);
  });
});
