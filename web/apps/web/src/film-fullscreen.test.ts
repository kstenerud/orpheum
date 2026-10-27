// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it, vi } from "vitest";

import {
  askPageFullScreen,
  fillsWindowClass,
  fillTheScreen,
  fullscreenAnswerWithinMs,
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
