// SPDX-License-Identifier: MPL-2.0

/**
 * The Play page's code fetched and compiled while the person browses, so a film's first start does
 * not wait on it. Under the discovery's loader the service worker caches nothing ahead: after a
 * deploy, the first film's Play page fetched its scripts (hls.js's 577 KiB among them) over the
 * page's WebRTC and compiled them afresh as the film started, and the first film cast to a Fire TV
 * after each deploy froze where later ones did not (staging, 2026-10-28, file 5671;
 * plans/navigation-and-widgets-answers-2026-10-27.md, "A first film after an update freezing").
 * The home page and a cast's ready page warm it once they have settled, once per page load, when
 * the browser is idle; a page reached straight, not under a loader, fetches from the core's own
 * name, where the browser's cache holds the scripts, and is left as it is.
 */

import { useEffect, useRef } from "react";

import { pageWorkerOf } from "./screen-load.tsx";
import { whenIdle } from "./when-idle.ts";

/**
 * The Play screen's code: the module its screen imports (later-screens.tsx), which then answers at
 * once. Imported here, not through later-screens, which names the cast's ready page that warms.
 */
async function warmPlayCode(): Promise<void> {
  await import("./play.tsx");
}

/**
 * How long a page shows before the Play page's code is warmed, in ms: past the home page's own first
 * requests and posters, which a Fire TV fetches over the same link, so the warming never delays what
 * the person is looking at. A justified constant.
 */
export const warmPlayAfterMs = 5000;

/** How long the browser's idle is waited for once warmPlayAfterMs has passed, at most, in ms. */
export const warmPlayIdleWithinMs = 5000;

/** The warming under way or done on this page load, so it is asked once. */
let warming: Promise<void> | undefined = undefined;

/**
 * Fetches and compiles the Play page's code once per page load (load, the Play screen's own import
 * by default); a failure is let go, the Play page asking again as it is shown (screen-load.tsx).
 */
export function warmPlayOnce(load: () => Promise<unknown> = warmPlayCode): Promise<void> {
  warming ??= load().then(
    () => undefined,
    () => undefined,
  );
  return warming;
}

/** Test-only: forgets the warming, as a new page load would. */
export function forgetPlayWarmed(): void {
  warming = undefined;
}

/**
 * Warms the Play page's code (warmPlayOnce) once the page using it has shown for warmPlayAfterMs
 * and the browser is idle, under the discovery's loader only; a page left before then warms nothing.
 */
export function useWarmsPlay(ways: WarmWays = {}): void {
  const waysRef = useRef(ways);
  useEffect(() => {
    const { isUnderLoader = isThisPageUnderLoader, load } = waysRef.current;
    if (warming !== undefined || !isUnderLoader()) {
      return undefined;
    }
    let stopIdle: (() => void) | undefined = undefined;
    const timer = setTimeout(() => {
      stopIdle = whenIdle(() => {
        void warmPlayOnce(load);
      }, warmPlayIdleWithinMs);
    }, warmPlayAfterMs);
    return () => {
      clearTimeout(timer);
      stopIdle?.();
    };
  }, []);
}

/** What useWarmsPlay asks, a test's own where given. */
export interface WarmWays {
  /** Whether the page runs under the discovery's loader: this page's by default. */
  readonly isUnderLoader?: () => boolean;
  /** The Play page's code's import: the Play screen's own by default. */
  readonly load?: () => Promise<unknown>;
}

/** Whether this page runs under the discovery's loader. */
function isThisPageUnderLoader(): boolean {
  return pageWorkerOf(window).isUnderLoader;
}
