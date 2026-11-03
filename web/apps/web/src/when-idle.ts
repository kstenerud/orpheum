// SPDX-License-Identifier: MPL-2.0

/**
 * The one way the page waits for the browser to be idle before work that can wait: the Play page's
 * code warmed (warm-play.ts), the tray's dialogs loaded ahead (tray-dialog-code.tsx). Safari (the
 * 17.1 floor) and the older TV engines have no requestIdleCallback; there a timer's next turn stands
 * in for it.
 */

/**
 * Runs then once the browser is idle, or withinMs have passed, whichever comes first; where the
 * browser has no idle wait, on a timer's next turn. Returns what cancels it.
 */
export function whenIdle(then: () => void, withinMs: number): () => void {
  if (typeof requestIdleCallback === "function") {
    const handle = requestIdleCallback(then, { timeout: withinMs });
    return () => {
      cancelIdleCallback(handle);
    };
  }
  const timer = setTimeout(then, 0);
  return () => {
    clearTimeout(timer);
  };
}
