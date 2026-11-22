// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { isBrowserFullscreen, isTelevision, type BrowserWindow } from "./input-mode.ts";
import { sourcesHere } from "./test-sources.ts";

describe("a television's browser, by its user agent", () => {
  it("knows the owner's Fire TV stick in Silk, and televisions of other makes", () => {
    expect(
      isTelevision(
        "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36",
      ),
    ).toBe(true);
    expect(
      isTelevision(
        "Mozilla/5.0 (SMART-TV; Linux; Tizen 7.0) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/5.0 Chrome/94.0.4606.31 TV Safari/537.36",
      ),
    ).toBe(true);
    expect(
      isTelevision(
        "Mozilla/5.0 (Web0S; Linux/SmartTV) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36 WebAppManager",
      ),
    ).toBe(true);
  });

  it("takes a computer's and a phone's browser for no television", () => {
    expect(
      isTelevision("Mozilla/5.0 (X11; Linux x86_64; rv:158.0) Gecko/20100101 Firefox/158.0"),
    ).toBe(false);
    expect(
      isTelevision(
        "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
      ),
    ).toBe(false);
    expect(
      isTelevision(
        "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36",
      ),
    ).toBe(false);
  });
});

describe("a browser that is itself full screen", () => {
  const sized = (
    inner: readonly [number, number],
    screen: readonly [number, number],
    more: {
      readonly isFullscreenDisplay?: boolean;
      readonly isAutomated?: boolean;
      readonly isTouch?: boolean;
      readonly outer?: readonly [number, number];
    } = {},
  ): BrowserWindow => ({
    width: inner[0],
    height: inner[1],
    outerWidth: more.outer?.[0] ?? inner[0],
    outerHeight: more.outer?.[1] ?? inner[1] + 80,
    screenWidth: screen[0],
    screenHeight: screen[1],
    isFullscreenDisplay: more.isFullscreenDisplay ?? false,
    isAutomated: more.isAutomated ?? false,
    isTouch: more.isTouch ?? false,
  });

  it("is known by a window exactly its screen's size, or by Chromium's fullscreen display mode", () => {
    expect(isBrowserFullscreen(sized([1920, 1080], [1920, 1080]))).toBe(true);
    // Safari full screen on the owner's Mac, its toolbar showing over a smaller page.
    expect(isBrowserFullscreen(sized([1710, 1021], [1710, 1112], { outer: [1710, 1112] }))).toBe(
      true,
    );
    expect(
      isBrowserFullscreen(sized([1280, 700], [1920, 1080], { isFullscreenDisplay: true })),
    ).toBe(true);
  });

  it("is not a window smaller than its screen, nor a test's browser by its size alone", () => {
    expect(isBrowserFullscreen(sized([1920, 960], [1920, 1080]))).toBe(false);
    // A maximised window under a Mac's menu bar.
    expect(isBrowserFullscreen(sized([1710, 990], [1710, 1112], { outer: [1710, 1075] }))).toBe(
      false,
    );
    expect(isBrowserFullscreen(sized([1280, 800], [1280, 800], { isAutomated: true }))).toBe(false);
  });

  it("is not a phone's browser by its window alone, always its screen's size (the owner's S23, 2026-10-30)", () => {
    expect(
      isBrowserFullscreen(sized([412, 780], [412, 915], { outer: [412, 915], isTouch: true })),
    ).toBe(false);
    expect(
      isBrowserFullscreen(
        sized([412, 915], [412, 915], { isTouch: true, isFullscreenDisplay: true }),
      ),
    ).toBe(true);
  });
});

/**
 * Each source but the input-mode module that asks the user agent for a television itself, or the
 * pointer's own media queries, as `file: what`.
 */
function ownInputModeReads(sources: readonly (readonly [string, string])[]): string[] {
  return sources
    .filter(([name]) => name !== "input-mode.ts")
    .flatMap(([name, text]) =>
      [...text.matchAll(/\bisTelevision\(|\((?:pointer|hover)\s*:\s*[a-z]+\)/gu)].map(
        (match) => `${name}: ${match[0]}`,
      ),
    );
}

describe("one source for the input mode (interface rule 12, D538)", () => {
  it("leaves the television flag and the pointer's queries to input-mode.ts alone", () => {
    const sources = [...sourcesHere(".ts"), ...sourcesHere(".tsx")];
    expect(sources.map(([name]) => name)).toContain("player-overlay.tsx");
    expect(ownInputModeReads(sources)).toEqual([]);
  });

  it("finds a component asking for itself, so the check above can fail", () => {
    expect(
      ownInputModeReads([
        ["a.tsx", "const isTv = isTelevision(navigator.userAgent);"],
        ["b.tsx", 'matchMedia("(pointer: coarse)")'],
        ["input-mode.ts", "isTelevision(userAgent)"],
      ]),
    ).toEqual(["a.tsx: isTelevision(", "b.tsx: (pointer: coarse)"]);
  });
});
