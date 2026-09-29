// SPDX-License-Identifier: MPL-2.0

/**
 * One of the browser's storages (localStorage, sessionStorage), or undefined where the browser
 * refuses it: a private window may throw on merely reading the property. What would have been
 * kept then lasts as long as the page.
 */
export function storageIfAllowed(readStorage: () => Storage): Storage | undefined {
  try {
    return readStorage();
  } catch {
    return undefined;
  }
}
