// SPDX-License-Identifier: MPL-2.0

/** The part of a browser storage the client uses, so a test can stand in; each call may throw. */
export interface StringStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

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

/**
 * storage with every call made safe: a read the browser refuses finds nothing, and a write it
 * refuses (a private window, a full quota) keeps nothing, so what would have been kept lasts as
 * long as the page. Undefined, a storage refused outright, keeps nothing at all.
 */
export function storageThatNeverThrows(storage: StringStorage | undefined): StringStorage {
  return {
    getItem: (key) => {
      try {
        return storage?.getItem(key) ?? null;
      } catch {
        return null;
      }
    },
    setItem: (key, value) => {
      try {
        storage?.setItem(key, value);
      } catch {
        // Kept in memory only.
      }
    },
    removeItem: (key) => {
      try {
        storage?.removeItem(key);
      } catch {
        // Kept in memory only.
      }
    },
  };
}

/** This tab's own storage (sessionStorage), which keeps nothing where the browser refuses it. */
export function tabStorage(): StringStorage {
  return storageThatNeverThrows(storageIfAllowed(() => window.sessionStorage));
}

/** This browser's storage (localStorage), which keeps nothing where the browser refuses it. */
export function browserStorage(): StringStorage {
  return storageThatNeverThrows(storageIfAllowed(() => window.localStorage));
}
