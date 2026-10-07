// SPDX-License-Identifier: MPL-2.0

/**
 * Whether this tab's voice is live: on, and connected with someone. While it is, the film's player
 * gives way to it (D262 §2): its quality is capped at half the link it measures, and its downloads
 * are paced. One tab carries one person's voice, so it is the tab's.
 */
let isTabVoiceLive = false;

/** Says whether this tab's voice is live; the voice session says so as its calls change. */
export function setVoiceLive(isLive: boolean): void {
  isTabVoiceLive = isLive;
}

/** Whether this tab's voice is live. */
export function isVoiceLive(): boolean {
  return isTabVoiceLive;
}
