// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it, vi } from "vitest";

import {
  askPageFullScreen,
  fillsWindowClass,
  fillTheScreen,
  fullscreenAnswerWithinMs,
  keepFullScreenOnTheStage,
  toggleFullScreen,
} from "./film-fullscreen.ts";

describe("a player's stage put full screen (the owner's Mac, Safari, 2026-10-25)", () => {
  it("fills the window where the browser refuses full screen", async () => {
    const stage = document.createElement("div");
    stage.requestFullscreen = (): Promise<void> => Promise.reject(new TypeError("not in a press"));
    fillTheScreen(stage, document);
    await Promise.resolve();
    await Promise.resolve();
    expect(stage.classList.contains(fillsWindowClass)).toBe(true);
  });

  it("goes back into the page as it leaves full screen, filling the window no more (the owner's S23, 2026-10-30)", () => {
    const stage = document.createElement("div");
    stage.classList.add(fillsWindowClass);
    Object.defineProperty(document, "fullscreenElement", { configurable: true, get: () => stage });
    Object.defineProperty(document, "exitFullscreen", {
      configurable: true,
      value: () => Promise.resolve(),
    });
    toggleFullScreen(stage, document);
    Reflect.deleteProperty(document, "fullscreenElement");
    expect(stage.classList.contains(fillsWindowClass)).toBe(false);
  });

  it("goes full screen where the browser lets it, filling nothing itself", async () => {
    const stage = document.createElement("div");
    let asked = 0;
    stage.requestFullscreen = (): Promise<void> => {
      asked += 1;
      return Promise.resolve();
    };
    fillTheScreen(stage, document);
    await Promise.resolve();
    expect(asked).toBe(1);
    expect(stage.classList.contains(fillsWindowClass)).toBe(false);
  });

  it("fills the window where the browser does not answer in time", async () => {
    vi.useFakeTimers();
    try {
      const stage = document.createElement("div");
      stage.requestFullscreen = (): Promise<void> => new Promise(() => undefined);
      fillTheScreen(stage, document);
      expect(stage.classList.contains(fillsWindowClass)).toBe(false);
      await vi.advanceTimersByTimeAsync(fullscreenAnswerWithinMs);
      expect(stage.classList.contains(fillsWindowClass)).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });

  it("fills the window when the wait ends after the page's globals are gone, throwing nothing (a test file torn down first, 2026-11-01)", async () => {
    vi.useFakeTimers();
    try {
      const stage = document.createElement("div");
      stage.requestFullscreen = (): Promise<void> => new Promise(() => undefined);
      fillTheScreen(stage, document);
      vi.stubGlobal("Element", undefined);
      await vi.advanceTimersByTimeAsync(fullscreenAnswerWithinMs);
      expect(stage.classList.contains(fillsWindowClass)).toBe(true);
    } finally {
      vi.unstubAllGlobals();
      vi.useRealTimers();
    }
  });

  it("asks nothing of the stage while the page asked for itself full screen, so the page holds it across every video (D364 §5)", async () => {
    let answer: () => void = () => undefined;
    document.documentElement.requestFullscreen = (): Promise<void> =>
      new Promise((resolve) => {
        answer = resolve;
      });
    try {
      const asked = askPageFullScreen(document);
      const stage = document.createElement("div");
      let stageAsked = 0;
      stage.requestFullscreen = (): Promise<void> => {
        stageAsked += 1;
        return Promise.resolve();
      };
      fillTheScreen(stage, document);
      expect(stageAsked).toBe(0);
      answer();
      await expect(asked).resolves.toBe(true);
      fillTheScreen(stage, document);
      expect(stageAsked).toBe(1); // the page's answer given, a stage asks again as before
    } finally {
      Reflect.deleteProperty(document.documentElement, "requestFullscreen");
    }
  });
});

describe("a film full screen on an iPhone, which has no element full screen (WebKit bug 206854)", () => {
  /** A stage as an iPhone's Safari has it: no requestFullscreen, its video with its own player. */
  function stageWithoutElementFullScreen(): { stage: HTMLElement; entered: () => number } {
    const stage = document.createElement("div");
    Object.defineProperty(stage, "requestFullscreen", { configurable: true, value: undefined });
    const video = document.createElement("video");
    Object.defineProperty(video, "requestFullscreen", { configurable: true, value: undefined });
    let entered = 0;
    Object.defineProperty(video, "webkitEnterFullscreen", {
      configurable: true,
      value: () => {
        entered += 1;
      },
    });
    stage.append(video);
    return { stage, entered: () => entered };
  }

  it("puts the film in iOS's own player at the full-screen press, not under Safari's bars (the owner, 2026-11-01)", () => {
    const { stage, entered } = stageWithoutElementFullScreen();
    toggleFullScreen(stage, document);
    expect(entered()).toBe(1);
    expect(stage.classList.contains(fillsWindowClass)).toBe(false);
  });

  it("puts Watch Together's film in iOS's own player too", () => {
    const { stage, entered } = stageWithoutElementFullScreen();
    // Its Full screen, as the Play page's, asks for the stage its video is in (keepFullScreenOnTheStage).
    toggleFullScreen(stage.querySelector("video")?.closest("div"), document);
    expect(entered()).toBe(1);
  });
});

describe("a video the browser's own controls put full screen (the owner, Firefox on Android, 2026-11-07)", () => {
  it("leaves the video's full screen for its stage's, where the page's controls show", async () => {
    const stage = document.createElement("div");
    stage.className = "film-player__stage";
    const video = document.createElement("video");
    stage.append(video);
    document.body.append(stage);
    let shown: Element | null = video;
    const asked: string[] = [];
    Object.defineProperty(document, "fullscreenElement", { configurable: true, get: () => shown });
    Object.defineProperty(document, "exitFullscreen", {
      configurable: true,
      value: () => {
        asked.push("leave the video's");
        shown = null;
        return Promise.resolve();
      },
    });
    stage.requestFullscreen = (): Promise<void> => {
      asked.push("the stage's");
      shown = stage;
      return Promise.resolve();
    };
    const stop = keepFullScreenOnTheStage(video, document);
    try {
      document.dispatchEvent(new Event("fullscreenchange"));
      await Promise.resolve();
      await Promise.resolve();
      expect(asked).toEqual(["leave the video's", "the stage's"]);
      // The stage's own full screen is left as it is.
      document.dispatchEvent(new Event("fullscreenchange"));
      await Promise.resolve();
      expect(asked).toEqual(["leave the video's", "the stage's"]);
    } finally {
      stop();
      stage.remove();
      Reflect.deleteProperty(document, "fullscreenElement");
      Reflect.deleteProperty(document, "exitFullscreen");
    }
  });
});
