// SPDX-License-Identifier: MPL-2.0

/**
 * Whether a live service worker controls the page, which carryMediaOver (rtc-media.ts) waits for
 * before it says the page carries its media: a page no worker controls loads past every worker, so
 * what it loads by itself goes to its origin.
 */

/**
 * How long a page waits, its worker ready and asked to claim it, for a worker to control it: one that
 * claims does so at once, so this is a bound for one that never will, not a wait anyone sits through.
 */
export const controlWaitMs = 5000;

/**
 * The page is not controlled by a live worker, ready and asked to claim it, after controlWaitMs: its
 * loads go past every worker, so the media it loads by itself goes to its origin. Chrome can commit a
 * page reloaded as an updated worker activates under the worker before and then stop that one,
 * leaving the page uncontrolled (idiosyncrasies.md).
 */
export class PageNotControlled extends Error {
  constructor() {
    super(
      `no service worker controls the page ${String(controlWaitMs)} ms after it was asked to, so its media cannot go over its path`,
    );
    this.name = "PageNotControlled";
  }
}

/** Whether a live worker controls the page: none, or one the browser stopped for good, does not. */
export function isControlled(workers: Pick<ServiceWorkerContainer, "controller">): boolean {
  return workers.controller !== null && workers.controller.state !== "redundant";
}

/** Whether a live worker controls the page now, or does within waitMs. */
export async function controlledWithin(
  workers: Pick<ServiceWorkerContainer, "controller" | "addEventListener" | "removeEventListener">,
  waitMs: number,
): Promise<boolean> {
  if (isControlled(workers)) {
    return true;
  }
  return await new Promise<boolean>((resolve) => {
    const changed = (): void => {
      if (isControlled(workers)) {
        settle();
      }
    };
    const timer = setTimeout(() => {
      settle();
    }, waitMs);
    function settle(): void {
      clearTimeout(timer);
      workers.removeEventListener("controllerchange", changed);
      resolve(isControlled(workers));
    }
    workers.addEventListener("controllerchange", changed);
  });
}
