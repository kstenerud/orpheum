// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { closesWhenIdle, idleClosesAfterMs, pausedClosesAfterMs } from "./idle.ts";

/** A video whose state the test sets, telling its watchers by the event a browser would. */
function aVideo(): {
  readonly video: HTMLVideoElement;
  readonly stand: (state: { source: string; isPaused: boolean }, event: string) => void;
} {
  const video = document.createElement("video");
  let source = "";
  let isPaused = true;
  Object.defineProperty(video, "currentSrc", { get: () => source });
  Object.defineProperty(video, "readyState", { get: () => (source === "" ? 0 : 4) });
  Object.defineProperty(video, "paused", { get: () => isPaused });
  Object.defineProperty(video, "ended", { get: () => false });
  return {
    video,
    stand: (state, event) => {
      source = state.source;
      isPaused = state.isPaused;
      video.dispatchEvent(new Event(event));
    },
  };
}

const film = "blob:https://core.example/7";

describe("the receiver's own idle close", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("closes a receiver started with nothing on it after 5 minutes, and not before", () => {
    const close = vi.fn();
    closesWhenIdle(aVideo().video, close);
    vi.advanceTimersByTime(idleClosesAfterMs - 1);
    expect(close).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(close).toHaveBeenCalledOnce();
  });

  it("never closes while a film plays, and closes 20 minutes after the film is taken off at its end", () => {
    const close = vi.fn();
    const { video, stand } = aVideo();
    closesWhenIdle(video, close);
    stand({ source: film, isPaused: false }, "playing");
    vi.advanceTimersByTime(3 * pausedClosesAfterMs);
    expect(close).not.toHaveBeenCalled();
    // The cast's film ended with nothing next: the cast plays nothing, and its film is taken off; it
    // waits as a paused film does (the owner, 2026-11-18: "20 minutes for all of those chromecast cases").
    stand({ source: "", isPaused: true }, "emptied");
    vi.advanceTimersByTime(pausedClosesAfterMs - 1);
    expect(close).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(close).toHaveBeenCalledOnce();
  });

  it("closes 20 minutes after its film was paused, timed from the pause, not from each event since", () => {
    const close = vi.fn();
    const { video, stand } = aVideo();
    closesWhenIdle(video, close);
    stand({ source: film, isPaused: false }, "playing");
    stand({ source: film, isPaused: true }, "pause");
    vi.advanceTimersByTime(idleClosesAfterMs);
    expect(close).not.toHaveBeenCalled();
    stand({ source: film, isPaused: true }, "pause");
    vi.advanceTimersByTime(pausedClosesAfterMs - idleClosesAfterMs - 1);
    expect(close).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(close).toHaveBeenCalledOnce();
  });

  it("starts its 20 minutes again once a paused film plays on, and closes on nothing once stopped", () => {
    const close = vi.fn();
    const { video, stand } = aVideo();
    const stop = closesWhenIdle(video, close);
    stand({ source: film, isPaused: true }, "pause");
    vi.advanceTimersByTime(pausedClosesAfterMs - 1);
    stand({ source: film, isPaused: false }, "playing");
    stand({ source: film, isPaused: true }, "pause");
    vi.advanceTimersByTime(pausedClosesAfterMs - 1);
    expect(close).not.toHaveBeenCalled();
    stop();
    vi.advanceTimersByTime(pausedClosesAfterMs);
    expect(close).not.toHaveBeenCalled();
  });
});
