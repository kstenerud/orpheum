// SPDX-License-Identifier: MPL-2.0

/** The class that has a player's stage fill the browser's window where full screen was refused. */
export const fillsWindowClass = "film-player__stage--fills-window";

/**
 * Puts a player's stage full screen, as a film opens on a television or in a browser that is itself
 * full screen; where the browser refuses, the stage fills the window instead, which in a browser
 * already full screen looks the same. Safari refuses a request not made in the press itself, and
 * the player opens a moment after the press that opened it (the owner's Mac, 2026-10-25: Safari full
 * screen, the film did not fill it).
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
}
