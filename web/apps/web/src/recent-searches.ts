// SPDX-License-Identifier: MPL-2.0

/**
 * The searches this device made last (search-by-more §4.5), newest first, shown in the search's
 * dialog while its field is empty. Kept in this browser's storage, one list for every page's search:
 * a search is kept when it is submitted, or when a result is opened from it.
 */

import { browserStorage, type StringStorage } from "./browser-storage.ts";

/** The storage key the list is kept under (client-keys.ts). */
export const recentSearchesKey = "orpheum.search.recent";

/** How many are kept: a handful, as a phone's screen shows under the field. */
export const recentSearchesKept = 6;

/** The searches kept, newest first; none where nothing, or something this release cannot read, is. */
export function recentSearches(storage: StringStorage = browserStorage()): string[] {
  const kept = storage.getItem(recentSearchesKey);
  if (kept === null) {
    return [];
  }
  try {
    const parsed: unknown = JSON.parse(kept);
    return Array.isArray(parsed)
      ? parsed.filter((each): each is string => typeof each === "string" && each !== "")
      : [];
  } catch {
    return [];
  }
}

/**
 * Keeps query as the newest search: the same search made before, in any case, moves to the front
 * rather than showing twice, and the oldest beyond the handful kept is dropped. Nothing is kept for
 * an empty search.
 */
export function keepRecentSearch(query: string, storage: StringStorage = browserStorage()): void {
  const search = query.trim();
  if (search === "") {
    return;
  }
  const same = search.toLocaleLowerCase();
  const others = recentSearches(storage).filter((each) => each.toLocaleLowerCase() !== same);
  storage.setItem(
    recentSearchesKey,
    JSON.stringify([search, ...others].slice(0, recentSearchesKept)),
  );
}
