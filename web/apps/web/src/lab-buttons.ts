// SPDX-License-Identifier: MPL-2.0

/**
 * The button test's chips (plans/tv-device-tests.md H2): which of the buttons a press can be
 * (input-buttons.ts) the test shows, and in which order.
 */

import type { ButtonName } from "./input-buttons.ts";

/** The chips the test always shows, in order: what a remote has that a page may hear. */
export const shownButtons: readonly ButtonName[] = [
  "up",
  "down",
  "left",
  "right",
  "select",
  "back",
  "playPause",
  "rewind",
  "fastForward",
  "other",
];

/**
 * The chips shown only once heard, before Other: a page is not given the volume keys on the TVs
 * tried (tv-device-tests.md §5), so a chip that never lights would only puzzle. A remote has no +
 * or −, and few have next and previous track, the keyboard's volume and episode keys (D407 §10).
 */
export const shownWhenHeard: readonly ButtonName[] = [
  "volumeUp",
  "volumeDown",
  "mute",
  "plus",
  "minus",
  "previousTrack",
  "nextTrack",
];

/** The chips to show: every always-shown one, with those shown once heard, before Other. */
export function chipsShown(heard: readonly ButtonName[]): ButtonName[] {
  const extra = shownWhenHeard.filter((button) => heard.includes(button));
  return [...shownButtons.slice(0, -1), ...extra, "other"];
}
