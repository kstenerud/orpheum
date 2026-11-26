// SPDX-License-Identifier: MPL-2.0

/**
 * The one Play page laid out by a real browser (D555 §8–9): the page's top bar shows while the page
 * is not full screen, the film in the page under it, and the stage fills the window only in full
 * screen; a device held upright has its controls under the picture, never hidden, and one on its
 * side, or a computer, the overlay.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";

import { fillsWindowClass, toggleFullScreen } from "./film-fullscreen.ts";
import { directDecision } from "./test-fixtures.ts";
import {
  filmDetail,
  mountApp,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
} from "./test-walk.tsx";
import { uprightQuery } from "./input-mode.ts";

/** Film 3's page and its file's playback, whose media the walk does not serve. */
const filmToPlay: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /items/103": filmDetail(103, "Film 3"),
  "POST /playback": directDecision({
    titleName: "Film 3",
    plays: { itemId: 103, kind: "film", libraryId: 1 },
    url: "/no-such-media",
  }),
};

let walking: Walking | undefined;

afterEach(() => {
  walking?.unmount();
  walking = undefined;
  vi.restoreAllMocks();
});

/** Makes the device a finger's, as a phone's is: upright wherever the window is taller than wide. */
function heldByAFinger(): void {
  const own = window.matchMedia.bind(window);
  vi.spyOn(window, "matchMedia").mockImplementation((query) => {
    const list = own(query);
    if (query === uprightQuery) {
      const isTall = own("(orientation: portrait)").matches;
      Object.defineProperty(list, "matches", { get: () => isTall });
    }
    return list;
  });
}

/** Grants full screen as a browser does, telling the page; returns what is full screen. */
function grantsFullScreen(): () => Element | null {
  let fullScreen: Element | null = null;
  vi.spyOn(Document.prototype, "fullscreenElement", "get").mockImplementation(() => fullScreen);
  const grant = (asked: Element): Promise<void> => {
    fullScreen = asked;
    document.dispatchEvent(new Event("fullscreenchange"));
    return Promise.resolve();
  };
  vi.spyOn(Element.prototype, "requestFullscreen").mockImplementation(function (this: Element) {
    return grant(this);
  });
  vi.spyOn(Document.prototype, "exitFullscreen").mockImplementation(() => {
    fullScreen = null;
    document.dispatchEvent(new Event("fullscreenchange"));
    return Promise.resolve();
  });
  return () => fullScreen;
}

/** The Play page opened at a screen's size, once its film's load has failed and it rests. */
async function playPageAt(width: number, height: number): Promise<HTMLElement> {
  await page.viewport(width, height);
  walking = await mountApp(walkServices(filmToPlay, "/play/1030"));
  await expect.poll(() => document.querySelector("video")?.error?.code).toBeDefined();
  const stage = document.querySelector<HTMLElement>(".film-player__stage");
  if (stage === null) {
    throw new Error("no stage drawn");
  }
  return stage;
}

/** Whether the shell's top bar shows. */
function isTopBarShown(): boolean {
  const banner = document.querySelector(".app-shell__banner");
  return (
    banner !== null &&
    getComputedStyle(banner).visibility !== "hidden" &&
    banner.getBoundingClientRect().height > 0
  );
}

describe("the Play page's top bar and full screen (D555 §9)", () => {
  it("shows the top bar over the film in the page, and fills the window only in full screen", async () => {
    const fullScreen = grantsFullScreen();
    const stage = await playPageAt(1280, 800);
    expect(stage.classList.contains(fillsWindowClass)).toBe(false);
    expect(isTopBarShown()).toBe(true);
    const bar = document.querySelector(".app-shell__banner")?.getBoundingClientRect();
    expect(stage.getBoundingClientRect().top).toBeGreaterThanOrEqual(bar?.bottom ?? 0);
    // A computer's controls are the overlay, in the page too (D555 §8).
    expect(stage.querySelector(".player-overlay")).not.toBeNull();
    expect(document.querySelector(".page-controls")).toBeNull();
    toggleFullScreen(stage, document);
    await expect.poll(() => stage.classList.contains(fillsWindowClass)).toBe(true);
    expect(fullScreen()).toBe(document.documentElement);
    expect(isTopBarShown()).toBe(false);
    expect(stage.getBoundingClientRect().height).toBe(window.innerHeight);
    toggleFullScreen(stage, document);
    await expect.poll(() => stage.classList.contains(fillsWindowClass)).toBe(false);
    expect(fullScreen()).toBeNull();
    expect(isTopBarShown()).toBe(true);
  });

  it("goes back into the page as the browser leaves full screen unasked (Escape)", async () => {
    grantsFullScreen();
    const stage = await playPageAt(1280, 800);
    toggleFullScreen(stage, document);
    await expect.poll(() => stage.classList.contains(fillsWindowClass)).toBe(true);
    await document.exitFullscreen();
    await expect.poll(() => stage.classList.contains(fillsWindowClass)).toBe(false);
    expect(isTopBarShown()).toBe(true);
  });
});

describe("the Play page upright and on its side (D555 §8)", () => {
  it("draws the controls under the picture on a phone held upright, never over it", async () => {
    heldByAFinger();
    const stage = await playPageAt(390, 844);
    expect(isTopBarShown()).toBe(true);
    expect(document.querySelector(".player-overlay")).toBeNull();
    const controls = document.querySelector(".page-controls");
    expect(controls).not.toBeNull();
    expect(stage.contains(controls)).toBe(false);
    const video = stage.querySelector("video")?.getBoundingClientRect();
    expect(controls?.getBoundingClientRect().top).toBeGreaterThanOrEqual(video?.bottom ?? 0);
    expect(controls?.querySelector('button[aria-label="Play"]')).not.toBeNull();
  });

  it("draws the overlay over the picture on a phone on its side", async () => {
    heldByAFinger();
    const stage = await playPageAt(844, 390);
    expect(stage.querySelector(".player-overlay")).not.toBeNull();
    expect(document.querySelector(".page-controls")).toBeNull();
  });

  it("draws the overlay on a phone held upright once full screen", async () => {
    heldByAFinger();
    grantsFullScreen();
    const stage = await playPageAt(390, 844);
    expect(document.querySelector(".page-controls")).not.toBeNull();
    toggleFullScreen(stage, document);
    await expect.poll(() => stage.querySelector(".player-overlay")).not.toBeNull();
    expect(document.querySelector(".page-controls")).toBeNull();
  });
});

/** Whether the page shows any of the film's choices the menus hold (Level loudness is the Sound menu's). */
function isAChoiceShown(): boolean {
  return document.body.textContent.includes("Level loudness");
}

// The owner, 2026-11-26: the overlay's menus hold the film's choices, so under the film they showed
// twice; held upright, the controls under the film are the remote's pad.
describe("the film's choices, the menus' in every layout (the owner, 2026-11-26)", () => {
  it("are not under the film on a computer, whose overlay's menus hold them", async () => {
    await playPageAt(1280, 800);
    expect(isAChoiceShown()).toBe(false);
  });

  it("are not under the film on a phone on its side, whose controls lie over the picture", async () => {
    heldByAFinger();
    await playPageAt(844, 390);
    expect(isAChoiceShown()).toBe(false);
  });

  it("are not under the film on a phone held upright, whose pad's menus' row holds them, end-aligned, its ⤢ last", async () => {
    heldByAFinger();
    await playPageAt(390, 844);
    expect(isAChoiceShown()).toBe(false);
    await expect
      .poll(() => document.querySelector(".page-controls .remote-pad__menus"))
      .not.toBeNull();
    expect(document.querySelector(".page-controls .remote-pad")).not.toBeNull();
    const menus = document.querySelector<HTMLElement>(".page-controls .remote-pad__menus")!;
    const last = [...menus.querySelectorAll<HTMLElement>("button")].at(-1)!;
    expect(last.getAttribute("aria-label")).toBe("Full screen");
    const end =
      menus.getBoundingClientRect().right - Number.parseFloat(getComputedStyle(menus).paddingRight);
    expect(Math.abs(last.getBoundingClientRect().right - end)).toBeLessThan(1.5);
  });
});

/** How many signs named name the page shows, wherever they are: the top bar's and the overlay's alike. */
function signsShown(name: string): number {
  return [...document.querySelectorAll(`[aria-label="${name}"]`)].filter(
    (sign) =>
      sign.checkVisibility({ visibilityProperty: true }) && sign.getBoundingClientRect().width > 0,
  ).length;
}

/** Shows the overlay where there is one, as a mouse moved over the page does. */
function overlayShown(): void {
  for (const screenX of [5, 50]) {
    act(() => {
      document.body.dispatchEvent(
        new PointerEvent("pointermove", {
          bubbles: true,
          pointerType: "mouse",
          screenX,
          screenY: 5,
        }),
      );
    });
  }
}

/** That Cast's sign and Watch Together's show once each, the overlay shown where there is one. */
async function expectEachSignOnce(): Promise<void> {
  overlayShown();
  await expect.poll(() => signsShown("Cast to a device")).toBe(1);
  await expect.poll(() => signsShown("Watch Together")).toBe(1);
}

// The owner, 2026-11-26: "Cast and watch together buttons only show once (top bar if it's visible,
// bottom bar otherwise)."
describe("Cast's and Watch Together's signs, once each (the owner, 2026-11-26)", () => {
  it("are the top bar's on a computer, not the overlay's", async () => {
    const stage = await playPageAt(1280, 800);
    await expectEachSignOnce();
    expect(stage.querySelector(".player-overlay__cast")).toBeNull();
  });

  it("are the top bar's on a phone on its side, not the overlay's", async () => {
    heldByAFinger();
    const stage = await playPageAt(844, 390);
    await expectEachSignOnce();
    expect(stage.querySelector(".player-overlay__cast")).toBeNull();
  });

  it("are the top bar's on a phone held upright, not the pad's", async () => {
    heldByAFinger();
    await playPageAt(390, 844);
    await expectEachSignOnce();
    expect(document.querySelector(".remote-pad__menus [aria-label='Cast to a device']")).toBeNull();
  });

  it("are the overlay's bottom row's in full screen, which hides the top bar", async () => {
    grantsFullScreen();
    const stage = await playPageAt(1280, 800);
    toggleFullScreen(stage, document);
    await expect.poll(() => isTopBarShown()).toBe(false);
    await expectEachSignOnce();
    expect(
      stage.querySelector(".player-overlay__buttons [aria-label='Cast to a device']"),
    ).not.toBeNull();
  });

  it("are the overlay's bottom row's on a phone held upright once full screen", async () => {
    heldByAFinger();
    grantsFullScreen();
    const stage = await playPageAt(390, 844);
    toggleFullScreen(stage, document);
    await expect.poll(() => stage.querySelector(".player-overlay")).not.toBeNull();
    await expectEachSignOnce();
    expect(
      stage.querySelector(".player-overlay__buttons [aria-label='Watch Together']"),
    ).not.toBeNull();
  });
});

/** That the overlay's bottom row ends with its last button, and no button of its end group sits at the row's start. */
function expectRowAtTheEnd(stage: HTMLElement): void {
  const row = stage.querySelector<HTMLElement>(".player-overlay__buttons")!;
  const trailing = row.querySelector<HTMLElement>(".player-controls__trailing")!;
  const rowBox = row.getBoundingClientRect();
  const end = rowBox.right - Number.parseFloat(getComputedStyle(row).paddingRight);
  const start = rowBox.left + Number.parseFloat(getComputedStyle(row).paddingLeft);
  const buttons = [...trailing.querySelectorAll<HTMLElement>("[role=button], button")];
  expect(Math.abs(buttons.at(-1)!.getBoundingClientRect().right - end)).toBeLessThan(1.5);
  for (const button of buttons) {
    expect(button.getBoundingClientRect().left - start, button.ariaLabel ?? "").toBeGreaterThan(
      1.5,
    );
  }
}

// The owner, 2026-11-26: "For the bottom button row, always right-aligned".
describe("the overlay's bottom row, at its end in every layout (the owner, 2026-11-26)", () => {
  for (const [width, height] of [
    [1280, 800],
    [440, 800],
  ] as const) {
    it(`ends with its last button on a computer ${String(width)} wide, none wrapped to the start`, async () => {
      grantsFullScreen();
      const stage = await playPageAt(width, height);
      toggleFullScreen(stage, document);
      await expect.poll(() => stage.querySelector(".player-overlay__buttons")).not.toBeNull();
      overlayShown();
      expectRowAtTheEnd(stage);
    });
  }

  it("ends with its last button on a phone on its side, none wrapped to the start", async () => {
    heldByAFinger();
    const stage = await playPageAt(844, 390);
    overlayShown();
    await expect.poll(() => stage.querySelector(".player-overlay__buttons")).not.toBeNull();
    expectRowAtTheEnd(stage);
  });
});
