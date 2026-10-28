// SPDX-License-Identifier: MPL-2.0

/**
 * A phone's width, the media query of app-shell.css's own breakpoint (its `max-width: 599px` rules):
 * the shell's narrow banner, and a drop-down that opens as a pop-up over its button rather than at it
 * (drop-down.tsx). A justified constant, the stylesheets' breakpoint, which a stylesheet cannot share.
 */
export const phoneWidthQuery = "(max-width: 599px)";

/** Whether the page is as narrow as a phone's now; false where the page cannot ask. */
export function isPhoneWidth(): boolean {
  return (
    typeof globalThis.matchMedia === "function" && globalThis.matchMedia(phoneWidthQuery).matches
  );
}
