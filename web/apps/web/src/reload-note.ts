// SPDX-License-Identifier: MPL-2.0

/**
 * What a page that reloads itself leaves for the page that follows to tell the core's log: a page
 * about to go has no time to send it, and anything it held goes with it. One note per tab, the
 * latest; the page that follows takes it once (newer-client.ts reportReloadForNewerClient).
 */

import type { StringStorage } from "./browser-storage.ts";

/** The tab's key for what the page reloaded for. */
const reloadNoteKey = "orpheum.screenCode.reloadNote";

/** Leaves text in storage for the page that follows this one's reload. */
export function leaveReloadNote(storage: StringStorage, text: string): void {
  storage.setItem(reloadNoteKey, text);
}

/** The note the page before this one left as it reloaded, taken so it is told once; none if none. */
export function takeReloadNote(storage: StringStorage): string | undefined {
  const note = storage.getItem(reloadNoteKey);
  if (note === null) {
    return undefined;
  }
  storage.removeItem(reloadNoteKey);
  return note;
}
