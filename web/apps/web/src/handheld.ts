// SPDX-License-Identifier: MPL-2.0

import { isHandheld, type HandheldSigns, type HiddenPage } from "@orpheum/player";

/** This browser's signs of a handheld: its touch points and its main pointer. */
function browserSigns(): HandheldSigns {
  return {
    maxTouchPoints: navigator.maxTouchPoints,
    matchMedia: (query: string) => window.matchMedia(query),
  };
}

/**
 * Whether this browser is a phone's or a tablet's (isHandheld), where a hidden page is the browser
 * gone to the background and its film is paused (pauseWhenHidden); a computer's hidden page plays on.
 */
export function isThisHandheld(signs: HandheldSigns = browserSigns()): boolean {
  return isHandheld(signs);
}

/**
 * The page a following player waits on while it is hidden (followGroup's page): a handheld's, whose
 * film is paused as it hides; undefined on a computer, whose hidden page plays its film on and so
 * follows its group's every pause, play and seek, hidden or not. A cast's receiver on a Mac, hidden
 * behind the window of its remote, played on for seven seconds after the remote paused it (the
 * owner, 2026-10-28).
 */
export function pageThatPausesWhenHidden(
  page: HiddenPage = document,
  signs: HandheldSigns = browserSigns(),
): HiddenPage | undefined {
  return isThisHandheld(signs) ? page : undefined;
}
