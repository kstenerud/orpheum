// SPDX-License-Identifier: MPL-2.0

/**
 * Where a cast's receiver goes back to as casting ends there (D338 §1): the page it showed before a
 * cast took it over, kept in the tab's storage so a receiver loaded again still knows it.
 */

import type { StringStorage } from "./browser-storage.ts";
import { parseRoute, routeHref, type Route } from "./router.ts";

/** The tab's key that keeps the page a cast took the tab over from, while it receives. */
export const castReturnKey = "orpheum.cast.returnTo";

/** What the key holds: the cast that took the tab over last, and the address it was at before. */
interface Kept {
  readonly castId: number;
  readonly href: string;
}

/** What storage keeps, or undefined for nothing or something malformed. */
function keptIn(storage: StringStorage): Kept | undefined {
  const text = storage.getItem(castReturnKey);
  if (text === null) {
    return undefined;
  }
  try {
    const parsed: unknown = JSON.parse(text);
    if (
      typeof parsed === "object" &&
      parsed !== null &&
      "castId" in parsed &&
      typeof parsed.castId === "number" &&
      "href" in parsed &&
      typeof parsed.href === "string"
    ) {
      return { castId: parsed.castId, href: parsed.href };
    }
  } catch {
    // malformed: nothing kept
  }
  return undefined;
}

/** A cast as it is handed to the tab: which, the page the tab is at, and whether it receives already. */
export interface Handed {
  readonly castId: number;
  readonly route: Route;
  /** Whether the tab receives already: another cast replaced the one it received, or it moved on. */
  readonly isReceiving: boolean;
}

/**
 * Notes, as a cast is handed to the tab, the page to go back to: the page it is at, the page it was
 * taken over from, unless it receives already or this cast was noted before the page was loaded
 * again, when the page noted first is kept.
 */
export function noteReturn(storage: StringStorage, handed: Handed): void {
  const kept = keptIn(storage);
  const isKept = kept !== undefined && (handed.isReceiving || kept.castId === handed.castId);
  const href = isKept ? kept.href : routeHref(handed.route);
  storage.setItem(castReturnKey, JSON.stringify({ castId: handed.castId, href } satisfies Kept));
}

/** The page to go back to as casting ends here, and forgets it; home when none was noted. */
export function takeReturn(storage: StringStorage): Route {
  const kept = keptIn(storage);
  storage.removeItem(castReturnKey);
  if (kept === undefined) {
    return { screen: "home" };
  }
  const url = new URL(kept.href, "http://orpheum.invalid");
  return parseRoute(url.pathname, url.search);
}

/**
 * Whether a page to go back to is noted: the tab was receiving a cast, which ends only by taking or
 * forgetting it, so a page loaded again with one noted was receiving as it unloaded (D383 §2).
 */
export function wasReceiving(storage: StringStorage): boolean {
  return keptIn(storage) !== undefined;
}

/** Forgets the page to go back to: the receiver keeps playing here, its control taken back. */
export function forgetReturn(storage: StringStorage): void {
  storage.removeItem(castReturnKey);
}
