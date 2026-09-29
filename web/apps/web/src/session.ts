// SPDX-License-Identifier: MPL-2.0

/**
 * The session: its token, kept in this browser so a reload stays signed in, and told to every
 * listener when it changes. Storage can be refused (a private window); the session then lasts
 * as long as the page.
 */
export interface Session {
  readonly token: () => string | undefined;
  /** Signs in, remembering the device the session is on, which the next sign-in resumes (D221). */
  readonly signIn: (token: string, deviceId?: number) => void;
  readonly signOut: () => void;
  /** Signs out because the server said the session no longer works, and why. */
  readonly signOutBecause: (reason: string) => void;
  /** Why the last session ended, when the server ended it; cleared by the next sign-in. */
  readonly lastEndReason: () => string | undefined;
  /** The device this browser was last signed in as, which a sign-in asks to resume (D221). */
  readonly deviceId: () => number | undefined;
  readonly subscribe: (listener: () => void) => () => void;
}

const tokenStorageKey = "orpheum.session";
const deviceIdStorageKey = "orpheum.device";

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
    token = storage?.getItem(tokenStorageKey) ?? undefined;
  } catch {
    token = undefined;
  }
  let endedReason: string | undefined;
  let storedDeviceId: number | undefined;
  try {
    const kept = Number(storage?.getItem(deviceIdStorageKey) ?? "");
    storedDeviceId = Number.isSafeInteger(kept) && kept > 0 ? kept : undefined;
  } catch {
    storedDeviceId = undefined;
  }
  const listeners = new Set<() => void>();
  const setToken = (next: string | undefined): void => {
    token = next;
    try {
      if (next === undefined) {
        storage?.removeItem(tokenStorageKey);
      } else {
        storage?.setItem(tokenStorageKey, next);
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
    signIn: (next, deviceId) => {
      endedReason = undefined;
      if (deviceId !== undefined) {
        storedDeviceId = deviceId;
        try {
          storage?.setItem(deviceIdStorageKey, String(deviceId));
        } catch {
          // Kept in memory only.
        }
      }
      setToken(next);
    },
    signOut: () => {
      setToken(undefined);
    },
    signOutBecause: (reason) => {
      endedReason = reason;
      setToken(undefined);
    },
    lastEndReason: () => endedReason,
    deviceId: () => storedDeviceId,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}
