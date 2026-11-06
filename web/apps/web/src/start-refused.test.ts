// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  followPressesForRefusals,
  keepThePressForTheStart,
  pageAtRefusal,
  startRefusedNote,
} from "./start-refused.ts";

describe("a film the browser would not start", () => {
  it("says the refusal and the page's press and full screen as it came (the owner's TCL TV, 2026-11-06)", () => {
    const refusal = new DOMException(
      "play() can only be initiated by a user gesture.",
      "NotAllowedError",
    );
    expect(
      startRefusedNote(refusal, {
        isPressLive: false,
        hasBeenPressed: true,
        isFullScreen: true,
        sincePressMs: 1240,
      }),
    ).toBe(
      "the browser refused to start a film (NotAllowedError: play() can only be initiated by a user" +
        " gesture.); pressLive=false pressed=true lastPress=1240 ms ago fullScreen=true",
    );
  });

  it("says when the page had no press, and a browser that does not say whether a press is live", () => {
    expect(
      startRefusedNote("refused", {
        isPressLive: undefined,
        hasBeenPressed: undefined,
        isFullScreen: false,
        sincePressMs: undefined,
      }),
    ).toBe(
      "the browser refused to start a film (not an error: refused); pressLive=undefined" +
        " pressed=undefined lastPress=none fullScreen=false",
    );
  });

  it("counts from the page's last press, a remote's key among them", () => {
    followPressesForRefusals();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    const since = pageAtRefusal().sincePressMs;
    expect(since).toBeDefined();
    expect(since).toBeLessThan(1000);
  });
});

describe("keeping the press for a film's start", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    Reflect.deleteProperty(navigator, "userActivation");
  });

  /** A video, paused or playing, whose play and pause are counted. */
  function countedVideo(isPaused: boolean): { video: HTMLVideoElement; steps: string[] } {
    const steps: string[] = [];
    const video = document.createElement("video");
    Object.defineProperty(video, "paused", { get: () => isPaused });
    video.play = () => {
      steps.push("play");
      return Promise.reject(new DOMException("paused", "AbortError"));
    };
    video.pause = () => {
      steps.push("pause");
    };
    return { video, steps };
  }

  it("plays and pauses a paused video while the press is live", () => {
    Object.defineProperty(navigator, "userActivation", {
      configurable: true,
      value: { isActive: true, hasBeenActive: true },
    });
    const { video, steps } = countedVideo(true);
    keepThePressForTheStart(video);
    expect(steps).toEqual(["play", "pause"]);
  });

  it("leaves a playing video, and one whose press is gone, as they are", () => {
    const playing = countedVideo(false);
    keepThePressForTheStart(playing.video);
    expect(playing.steps).toEqual([]);
    Object.defineProperty(navigator, "userActivation", {
      configurable: true,
      value: { isActive: false, hasBeenActive: true },
    });
    const pressGone = countedVideo(true);
    keepThePressForTheStart(pressGone.video);
    expect(pressGone.steps).toEqual([]);
  });
});
