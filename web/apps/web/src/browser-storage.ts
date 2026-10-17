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

/** The prefix of every key a release of Orpheum keeps. */
const keptPrefix = "orpheum.";

/** Every key storage holds. */
function keysOf(storage: Storage): string[] {
  const keys: string[] = [];
  for (let index = 0; index < storage.length; index += 1) {
    const key = storage.key(index);
    if (key !== null) {
      keys.push(key);
    }
  }
  return keys;
}

/**
 * Removes every key under Orpheum's prefix that owned does not name: what an older release kept,
 * read by none now. A key of another prefix is not Orpheum's to remove.
 */
export function removeUnownedKeys(
  storage: Storage | undefined,
  owned: (key: string) => boolean,
): void {
  if (storage === undefined) {
    return;
  }
  let keys: string[];
  try {
    keys = keysOf(storage);
  } catch {
    return;
  }
  const removable = storageThatNeverThrows(storage);
  for (const key of keys) {
    if (key.startsWith(keptPrefix) && !owned(key)) {
      removable.removeItem(key);
    }
  }
}
