// SPDX-License-Identifier: MPL-2.0

/**
 * The steps a page took from finding its core to running the core's client, each with the time since
 * the page loaded, for what happened when it failed (discovery-core.md K16). A guest's Safari dropped
 * its connection about 100 ms after the core held it, with no request made and nothing said of why
 * (plan item 15, 2026-11-02): which step it reached is what this keeps. A step names what the page
 * did, never an address or anything its person typed. They are kept in the tab's session storage too,
 * so the steps before a reload are told after it.
 */

/** The most steps kept, the newest: a page reaching its core takes a dozen, a reload twice that. */
export const stepsMost = 40;

/** The tab's session storage key the steps are kept under. */
export const stepsKey = "orpheum-steps";

/** The step that marks a page loading after the steps kept before it. */
export const pageLoadedAgain = "the page loaded again";

let kept: string[] | undefined;

/** The tab's session storage, or undefined where the browser refuses it. */
function tabStorage(): Storage | undefined {
  try {
    return globalThis.sessionStorage;
  } catch {
    return undefined;
  }
}

/** The steps kept, read from the tab's storage once in a page. */
function steps(): string[] {
  if (kept === undefined) {
    kept = [];
    try {
      const stored: unknown = JSON.parse(tabStorage()?.getItem(stepsKey) ?? "[]");
      if (Array.isArray(stored) && stored.length > 0) {
        kept = stored.filter((each): each is string => typeof each === "string");
        kept.push(pageLoadedAgain);
      }
    } catch {
      // A step lost is no failure of the page.
    }
  }
  return kept;
}

/** Keeps step, with the milliseconds since the page loaded, dropping the oldest past stepsMost. */
export function noteStep(step: string): void {
  const all = steps();
  all.push(`${String(Math.round(performance.now()))} ms ${step}`);
  all.splice(0, Math.max(0, all.length - stepsMost));
  try {
    tabStorage()?.setItem(stepsKey, JSON.stringify(all));
  } catch {
    // A step lost is no failure of the page.
  }
}

/** The steps kept so far, oldest first. */
export function stepsSoFar(): readonly string[] {
  return [...steps()];
}

/** Forgets the steps kept, here and in the tab's storage, as a test starting afresh does. */
export function forgetSteps(): void {
  kept = undefined;
  try {
    tabStorage()?.removeItem(stepsKey);
  } catch {
    // Nothing kept to forget.
  }
}
