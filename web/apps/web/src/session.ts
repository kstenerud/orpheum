// SPDX-License-Identifier: MPL-2.0

import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";

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
const deviceIdStorageKey = "orpheum.deviceId";

/** Creates a session over storage, which may throw on every call. */
export function createSession(refusableStorage: StringStorage | undefined): Session {
  const storage = storageThatNeverThrows(refusableStorage);
  let token = storage.getItem(tokenStorageKey) ?? undefined;
  let endedReason: string | undefined;
  const keptDeviceId = Number(storage.getItem(deviceIdStorageKey) ?? "");
  let storedDeviceId =
    Number.isSafeInteger(keptDeviceId) && keptDeviceId > 0 ? keptDeviceId : undefined;
  const listeners = new Set<() => void>();
  const setToken = (next: string | undefined): void => {
    token = next;
    if (next === undefined) {
      storage.removeItem(tokenStorageKey);
    } else {
      storage.setItem(tokenStorageKey, next);
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
        storage.setItem(deviceIdStorageKey, String(deviceId));
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
