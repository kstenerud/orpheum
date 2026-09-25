// SPDX-License-Identifier: MPL-2.0

/**
 * The session: its token, kept in this browser so a reload stays signed in, and told to every
 * listener when it changes. Storage can be refused (a private window); the session then lasts
 * as long as the page.
 */
export interface Session {
  readonly token: () => string | undefined;
  readonly signIn: (token: string) => void;
  readonly signOut: () => void;
  readonly subscribe: (listener: () => void) => () => void;
}

const storageKey = "orpheum.session";

/** The storage a session keeps its token in: the browser's local storage, or none. */
export interface TokenStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

/** Creates a session over storage, which may throw on every call. */
export function createSession(storage: TokenStorage | undefined): Session {
  let token: string | undefined;
  try {
    token = storage?.getItem(storageKey) ?? undefined;
  } catch {
    token = undefined;
  }
  const listeners = new Set<() => void>();
  const change = (next: string | undefined): void => {
    token = next;
    try {
      if (next === undefined) {
        storage?.removeItem(storageKey);
      } else {
        storage?.setItem(storageKey, next);
      }
    } catch {
      // Kept in memory only: a private window refuses storage.
    }
    for (const listener of listeners) {
      listener();
    }
  };
  return {
    token: () => token,
    signIn: (next) => {
      change(next);
    },
    signOut: () => {
      change(undefined);
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}
