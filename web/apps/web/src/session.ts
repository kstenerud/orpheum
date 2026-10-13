// SPDX-License-Identifier: MPL-2.0

import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";

/**
 * The session: its token, kept in this browser so a reload stays signed in, and told to every
 * listener when it changes. Storage can be refused (a private window); the session then lasts
 * as long as the page.
 */
export interface Session {
  readonly token: () => string | undefined;
  /**
   * Signs in, remembering the device the session is on and its device secret, by which the next
   * sign-in resumes it (D221; the third audit's A8).
   */
  readonly signIn: (token: string, kept?: KeptWithSession) => void;
  readonly signOut: () => void;
  /** Signs out because the server said the session no longer works, and why. */
  readonly signOutBecause: (reason: string) => void;
  /** Why the last session ended, when the server ended it; cleared by the next sign-in. */
  readonly lastEndReason: () => string | undefined;
  /** The device this browser was last signed in as, which a sign-in asks to resume (D221). */
  readonly deviceId: () => number | undefined;
  /**
   * The device secret this browser was last given, kept past signing out: the proof that it was the
   * device it asks to resume, without which the server makes a new device (the second audit's K1).
   * Never a session token, which would hand whoever it was sent to a working session (the third
   * audit's A8).
   */
  readonly deviceProof: () => string | undefined;
  /**
   * The locator this session was handed, by which a discovery finds the server for this device
   * (discovery-core.md K7): it ends with the session.
   */
  readonly locator: () => string | undefined;
  readonly subscribe: (listener: () => void) => () => void;
}

const tokenStorageKey = "orpheum.session";
const deviceIdStorageKey = "orpheum.deviceId";
const deviceProofStorageKey = "orpheum.deviceProof";
const locatorStorageKey = "orpheum.locator";

/** What a sign-in keeps beside its token: the device, its secret and its locator. */
export interface KeptWithSession {
  readonly deviceId?: number | undefined;
  readonly deviceSecret?: string | undefined;
  readonly locator?: string | undefined;
}

/** Creates a session over storage, which may throw on every call. */
export function createSession(refusableStorage: StringStorage | undefined): Session {
  const storage = storageThatNeverThrows(refusableStorage);
  let token = storage.getItem(tokenStorageKey) ?? undefined;
  let endedReason: string | undefined;
  const keptDeviceId = Number(storage.getItem(deviceIdStorageKey) ?? "");
  let storedDeviceId =
    Number.isSafeInteger(keptDeviceId) && keptDeviceId > 0 ? keptDeviceId : undefined;
  let deviceProof = storage.getItem(deviceProofStorageKey) ?? undefined;
  let locator = storage.getItem(locatorStorageKey) ?? undefined;
  const listeners = new Set<() => void>();
  const setToken = (next: string | undefined): void => {
    token = next;
    if (next === undefined) {
      locator = undefined;
      storage.removeItem(locatorStorageKey);
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
    signIn: (next, kept = {}) => {
      const { deviceId, deviceSecret, locator: nextLocator } = kept;
      endedReason = undefined;
      if (deviceId !== undefined) {
        storedDeviceId = deviceId;
        storage.setItem(deviceIdStorageKey, String(deviceId));
      }
      if (deviceSecret !== undefined) {
        deviceProof = deviceSecret;
        storage.setItem(deviceProofStorageKey, deviceSecret);
      }
      setToken(next);
      if (nextLocator !== undefined) {
        locator = nextLocator;
        storage.setItem(locatorStorageKey, nextLocator);
      }
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
    deviceProof: () => deviceProof,
    locator: () => locator,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}
