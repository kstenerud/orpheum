// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { fillsWindowClass, fillTheScreen } from "./film-fullscreen.ts";

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
});
