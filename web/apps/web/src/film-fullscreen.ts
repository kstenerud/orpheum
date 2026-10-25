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
 * How long a browser may take to put the stage full screen before it fills the window instead, in
 * milliseconds: a full-screen change takes a fraction of a second. A justified constant.
 */
export const fullscreenAnswerWithinMs = 1000;
