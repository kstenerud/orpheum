// SPDX-License-Identifier: MPL-2.0

/** The class that has a player's stage fill the browser's window where full screen was refused. */
export const fillsWindowClass = "film-player__stage--fills-window";

/**
 * Puts a player's stage full screen, as a film opens on a television or in a browser that is itself
 * full screen; where the browser refuses, the stage fills the window instead, which in a browser
 * already full screen looks the same. Safari refuses a request not made in the press itself, and
 * the player opens a moment after the press that opened it (the owner's Mac, 2026-10-25: Safari full
 * screen, the film did not fill it), and may not answer at all, so a request unanswered in time
 * counts as refused.
 */
export function fillTheScreen(stage: HTMLElement, document: Document): void {
  if (document.fullscreenElement instanceof Element) {
    return;
  }
  const fillWindow = (): void => {
    stage.classList.add(fillsWindowClass);
  };
  if (typeof stage.requestFullscreen !== "function") {
    fillWindow();
    return;
  }
  stage.requestFullscreen().catch(fillWindow);
  // A browser that neither grants nor refuses in time is taken to have refused.
  setTimeout(() => {
    if (!(document.fullscreenElement instanceof Element)) {
      fillWindow();
    }
  }, fullscreenAnswerWithinMs);
}

/**
 * A full-screen button's press, or a double click on the film: out of full screen it puts the
 * player's stage full screen (fillTheScreen), from the press the Fullscreen API needs; in full screen
 * it leaves, as the page's own asking (leaveFullScreenAsked), so a cast's receiver keeps its cast.
 */
export function toggleFullScreen(stage: Element | null | undefined, page: Document): void {
  if (page.fullscreenElement instanceof Element) {
    leaveFullScreenAsked(page);
    return;
  }
  if (stage instanceof HTMLElement) {
    fillTheScreen(stage, page);
  }
}

/** The pages that asked to leave full screen, until the browser says it changed. */
const leavingAsked = new WeakSet<Document>();

/**
 * Leaves full screen at the page's own asking, which isFullScreenLeaveAsked says until the browser's
 * next fullscreenchange has been heard: a cast's receiver takes control back where the browser leaves
 * full screen unasked (a Fire TV's Back, Escape; cast-receiver.tsx), and its own leaving, Stop
 * casting or the film's end, is not that.
 */
export function leaveFullScreenAsked(page: Document): void {
  leavingAsked.add(page);
  const forget = (): void => {
    leavingAsked.delete(page);
  };
  page.addEventListener("fullscreenchange", forget, { once: true });
  page.exitFullscreen().catch(() => {
    page.removeEventListener("fullscreenchange", forget);
    forget();
  });
}

/** Whether the full screen being left now is one the page asked to leave (leaveFullScreenAsked). */
export function isFullScreenLeaveAsked(page: Document): boolean {
  return leavingAsked.has(page);
}

/**
 * How long a browser may take to put the stage full screen before it fills the window instead, in
 * milliseconds: a full-screen change takes a fraction of a second. A justified constant.
 */
export const fullscreenAnswerWithinMs = 1000;
