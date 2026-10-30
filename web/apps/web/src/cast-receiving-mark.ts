// SPDX-License-Identifier: MPL-2.0

/**
 * Whether the tab is receiving a cast, kept in the tab's storage so a page loaded again while it
 * received one (a reload, a deploy) knows to hold its film until the server says whether the cast goes
 * on (D383 §2). A cast that ends sends its receiver home (D384 §1), so nothing else is kept: no page
 * to go back to.
 */

import type { StringStorage } from "./browser-storage.ts";

/** The tab's key that holds the id of the cast it receives, while it receives one. */
export const castReceivingKey = "orpheum.cast.receiving";

/** Marks the tab as receiving the cast castId. */
export function markReceiving(storage: StringStorage, castId: number): void {
  storage.setItem(castReceivingKey, String(castId));
}

/**
 * Whether the tab was marked receiving: it ends only by forgetting the mark, so a page loaded again
 * with it was receiving as it unloaded (D383 §2).
 */
export function wasReceiving(storage: StringStorage): boolean {
  return storage.getItem(castReceivingKey) !== null;
}

/** Forgets the mark: receiving is over here. */
export function forgetReceiving(storage: StringStorage): void {
  storage.removeItem(castReceivingKey);
}
