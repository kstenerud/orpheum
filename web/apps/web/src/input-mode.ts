// SPDX-License-Identifier: MPL-2.0

import { fireTvPattern, isFireTv } from "@orpheum/sdk";
import { useSyncExternalStore } from "react";

/*
 * The one source of how the person presses (interface rule 12, D538): a remote's d-pad, a finger or
 * a mouse. A single television flag stands for the d-pad, read from the user agent here alone; the
 * pointer and hover media queries tell a finger from a mouse. Components ask this module, never the
 * user agent or the pointer's queries themselves (input-mode.test.ts holds every source to it).
 */

/** How the person presses: a remote's arrows and OK, a finger, or a mouse (or a trackpad). */
export type InputMode = "remote" | "touch" | "mouse";

/**
 * Whether this browser runs on a television or a streaming stick, by what its user agent says of
 * itself: a Fire TV's (AFT…), Android TV's and Google TV's, a Chromecast's, Samsung's Tizen, LG's
 * webOS, Sony's BRAVIA, and any that names itself a smart TV or HbbTV. There a film plays full
 * screen at once, with the player's own controls a remote can reach (the owner, 2026-10-25: "on a
 * streaming device like the amazon stick [it] should go fullscreen right away. Probably the only
 * place where you wouldn't fullscreen by default would be on a computer"). The user agent given is
 * a test's; the app asks isOnTelevision.
 */
export function isTelevision(userAgent: string): boolean {
  return televisionPattern.test(userAgent);
}

/**
 * Whether this device is a television's, its remote's d-pad what presses (isTelevision of this
 * browser's user agent), read as it is asked, so a test that names a television's is heard.
 */
export function isOnTelevision(): boolean {
  return isTelevision(globalThis.navigator.userAgent);
}

/** Whether the media query matches, false where the page cannot ask. */
function matches(query: string): boolean {
  return typeof globalThis.matchMedia === "function" && globalThis.matchMedia(query).matches;
}

/** Whether the device's main pointer is a finger, a phone's or a tablet's: a coarse one. */
export function isTouchPointer(): boolean {
  return matches("(pointer: coarse)");
}

/**
 * A mouse's pointer, one that hovers and is fine, as the stylesheets' own `(hover: hover)` queries
 * say: a justified constant, a stylesheet's query, as phoneWidthQuery is.
 */
export const mousePointerQuery = "(hover: hover) and (pointer: fine)";

/** Whether the device's pointer is a mouse's now (mousePointerQuery). */
export function isMousePointer(): boolean {
  return matches(mousePointerQuery);
}

/** Tells changed as the page starts or stops matching media; returns the stopping. */
function watchQuery(media: string, changed: () => void): () => void {
  if (typeof globalThis.matchMedia !== "function") {
    return () => undefined;
  }
  const query = globalThis.matchMedia(media);
  query.addEventListener("change", changed);
  return () => {
    query.removeEventListener("change", changed);
  };
}

/** Tells changed as the device's pointer becomes a mouse's or stops being one. */
function watchMousePointer(changed: () => void): () => void {
  return watchQuery(mousePointerQuery, changed);
}

/**
 * Whether the device's pointer is a mouse's (mousePointerQuery), kept as it changes: a tablet's
 * keyboard and trackpad attached; false where the page cannot ask.
 */
export function useIsMousePointer(): boolean {
  return useSyncExternalStore(watchMousePointer, isMousePointer, () => false);
}

/**
 * A phone or a tablet held upright: taller than wide, pressed by a finger (D555 §8). A computer's
 * window narrowed upright is no phone: its mouse keeps the overlay. A justified constant, a media
 * query.
 */
export const uprightQuery = "(orientation: portrait) and (pointer: coarse)";

/** Whether the device is held upright now (uprightQuery). */
function isUpright(): boolean {
  return matches(uprightQuery);
}

/** Tells changed as the device turns upright or on its side. */
function watchUpright(changed: () => void): () => void {
  return watchQuery(uprightQuery, changed);
}

/** Whether the device is held upright (uprightQuery), kept as it turns; false where it cannot ask. */
export function useIsUpright(): boolean {
  return useSyncExternalStore(watchUpright, isUpright, () => false);
}

/**
 * How the person presses now: the remote on a television, else a finger where the pointer is
 * coarse, else a mouse.
 */
export function inputModeNow(): InputMode {
  if (isOnTelevision()) {
    return "remote";
  }
  return isTouchPointer() ? "touch" : "mouse";
}

/** Whether this browser runs on a Fire TV: the SDK's, which the player sizes its buffer by too. */
export { isFireTv };

/** The words a television's or a streaming stick's user agent says of itself. */
const televisionPattern = new RegExp(
  `${fireTvPattern.source}|Android TV|GoogleTV|Google TV|\\bCrKey\\b|Tizen|Web0S|webOS|BRAVIA|SmartTV|SMART-TV|HbbTV|\\bAppleTV\\b`,
  "iu",
);

/** What isBrowserFullscreen reads of a window and its browser. */
export interface BrowserWindow {
  readonly width: number;
  readonly height: number;
  /** The window's own size, its toolbar and frame included. */
  readonly outerWidth: number;
  readonly outerHeight: number;
  readonly screenWidth: number;
  readonly screenHeight: number;
  /** Whether Chromium says the fullscreen display mode. */
  readonly isFullscreenDisplay: boolean;
  /** Whether automation drives the browser (navigator.webdriver), as a test's does. */
  readonly isAutomated: boolean;
  /**
   * Whether its main pointer is a finger (a phone's, a tablet's): its browser's window is always its
   * screen's size, full screen or not.
   */
  readonly isTouch: boolean;
}

/**
 * Whether the browser itself is full screen, as by F11 or a Mac's full-screen window, where the page
 * is told nothing: Chromium says it as the fullscreen display mode, and any browser's window then is
 * exactly its screen's size, its page so too in Chromium and Firefox, its window with its toolbar in
 * Safari, which keeps its toolbar showing in full screen (the owner's Mac, 2026-10-25: a page of
 * 1710 by 1021 on a screen of 1710 by 1112). A maximised window is not: a menu bar or a taskbar
 * keeps part of the screen. A test's browser, whose screen is whatever its window was made, is
 * judged by the display mode alone, as is a phone's or a tablet's, whose browser's window is always its
 * screen's size (the owner's S23, 2026-10-30: every Play page filled the window over the banner). A
 * film there plays full screen at once, as on a television (the owner, 2026-10-25: "Is there a way to
 * detect if a desktop browser is in fullscreen mode? If so, we should auto-fullscreen").
 */
export function isBrowserFullscreen(browser: BrowserWindow): boolean {
  const isPageScreenSized =
    browser.width === browser.screenWidth && browser.height === browser.screenHeight;
  const isWindowScreenSized =
    browser.outerWidth === browser.screenWidth && browser.outerHeight === browser.screenHeight;
  const isScreenSized = isPageScreenSized || isWindowScreenSized;
  return browser.isFullscreenDisplay || (isScreenSized && !browser.isAutomated && !browser.isTouch);
}

/** This window and browser, as isBrowserFullscreen reads them. */
export function thisBrowserWindow(): BrowserWindow {
  return {
    width: window.innerWidth,
    height: window.innerHeight,
    outerWidth: window.outerWidth,
    outerHeight: window.outerHeight,
    screenWidth: window.screen.width,
    screenHeight: window.screen.height,
    isFullscreenDisplay: window.matchMedia("(display-mode: fullscreen)").matches,
    isAutomated: navigator.webdriver,
    isTouch: isTouchPointer(),
  };
}
