// SPDX-License-Identifier: MPL-2.0

/**
 * Whether this browser runs on a television or a streaming stick, by what its user agent says of
 * itself: a Fire TV's (AFT…), Android TV's and Google TV's, a Chromecast's, Samsung's Tizen, LG's
 * webOS, Sony's BRAVIA, and any that names itself a smart TV or HbbTV. There a film plays full
 * screen at once, with the player's own controls a remote can reach (the owner, 2026-10-25: "on a
 * streaming device like the amazon stick [it] should go fullscreen right away. Probably the only
 * place where you wouldn't fullscreen by default would be on a computer").
 */
export function isTelevision(userAgent: string): boolean {
  return televisionPattern.test(userAgent);
}

/** A Fire TV's model code, which its user agent names it by: AFTSSS, AFTKA, AFTMM... */
const fireTvPattern = /\bAFT[A-Z0-9]+\b/u;

/** Whether this browser runs on a Fire TV, by the model code its user agent names it by. */
export function isFireTv(userAgent: string): boolean {
  return fireTvPattern.test(userAgent);
}

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
    isTouch: window.matchMedia("(pointer: coarse)").matches,
  };
}
