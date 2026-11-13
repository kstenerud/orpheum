// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";

import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";

/** Who a session belongs to, as the server tells it. */
type UserBody = components["schemas"]["UserBody"];

/** Whose a session is: a person's, or a guest receiver's (D465 §2). */
export type SessionKind = "person" | "guestReceiver";

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
  /**
   * Who the sign-in's answer said is signed in, with what the shell reads as it starts: the first
   * screen after a sign-in starts from it, with no read of GET /me (D476). Held by this page alone,
   * until the session ends; undefined on a page that did not sign in.
   */
  readonly handedUser: () => UserBody | undefined;
  /**
   * Whether the session is a person's or a guest receiver's (D465 §2), as its sign-in or the last
   * GET /me said, kept with it so a reload knows before GET /me answers: a person's shell and screen
   * start reading alongside that read, and a guest receiver, refused every one of them, starts none
   * (D476). Undefined until either has said.
   */
  readonly knownAs: () => SessionKind | undefined;
  /** Keeps what GET /me said the session is (knownAs). */
  readonly noteKnownAs: (kind: SessionKind) => void;
  readonly subscribe: (listener: () => void) => () => void;
}

/** The keys a session is kept under, after its prefix: "orpheum." on a core's own origin. */
const tokenStorageKey = "session";
const deviceIdStorageKey = "deviceId";
const deviceProofStorageKey = "deviceProof";
const locatorStorageKey = "locator";
const kindStorageKey = "kind";

/** The prefix of a session's keys on a core's own origin, which holds that core's alone. */
export const ownOriginPrefix = "orpheum.";

/**
 * The prefix of a session's keys on a discovery's origin, which holds a session for each core it
 * reached: every credential kept under its core's id, never one for the origin (the protocol's §3.4;
 * A8, W1).
 */
export function corePrefix(coreId: string): string {
  return `orpheum.core.${coreId}.`;
}

/**
 * What a sign-in keeps beside its token: the device, its secret, its locator, and who its answer said
 * is signed in (user), which the shell takes in place of its first read (D476).
 */
export interface KeptWithSession {
  readonly deviceId?: number | undefined;
  readonly deviceSecret?: string | undefined;
  readonly locator?: string | undefined;
  readonly user?: UserBody | undefined;
}

/** Keeps value under key when one is given, and returns it; undefined when none is. */
function keepIfGiven<T extends string | number>(
  storage: StringStorage,
  key: string,
  value: T | undefined,
): T | undefined {
  if (value !== undefined) {
    storage.setItem(key, String(value));
  }
  return value;
}

/** storage, every key under prefix. */
function prefixed(storage: StringStorage, prefix: string): StringStorage {
  return {
    getItem: (key) => storage.getItem(prefix + key),
    setItem: (key, value) => {
      storage.setItem(prefix + key, value);
    },
    removeItem: (key) => {
      storage.removeItem(prefix + key);
    },
  };
}

/**
 * Creates a session over storage, which may throw on every call, its keys under prefix: one core's
 * credentials, apart from any other's.
 */
export function createSession(
  refusableStorage: StringStorage | undefined,
  prefix: string = ownOriginPrefix,
): Session {
  const storage = prefixed(storageThatNeverThrows(refusableStorage), prefix);
  let token = storage.getItem(tokenStorageKey) ?? undefined;
  let endedReason: string | undefined;
  const keptDeviceId = Number(storage.getItem(deviceIdStorageKey) ?? "");
  let storedDeviceId =
    Number.isSafeInteger(keptDeviceId) && keptDeviceId > 0 ? keptDeviceId : undefined;
  let deviceProof = storage.getItem(deviceProofStorageKey) ?? undefined;
  let locator = storage.getItem(locatorStorageKey) ?? undefined;
  const keptKind = storage.getItem(kindStorageKey);
  let kind: SessionKind | undefined =
    keptKind === "person" || keptKind === "guestReceiver" ? keptKind : undefined;
  const noteKind = (next: SessionKind | undefined): void => {
    kind = next;
    if (next === undefined) {
      storage.removeItem(kindStorageKey);
    } else {
      storage.setItem(kindStorageKey, next);
    }
  };
  let handedUser: UserBody | undefined;
  const listeners = new Set<() => void>();
  const setToken = (next: string | undefined): void => {
    token = next;
    if (next === undefined) {
      locator = undefined;
      handedUser = undefined;
      noteKind(undefined);
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
      endedReason = undefined;
      storedDeviceId = keepIfGiven(storage, deviceIdStorageKey, kept.deviceId) ?? storedDeviceId;
      deviceProof = keepIfGiven(storage, deviceProofStorageKey, kept.deviceSecret) ?? deviceProof;
      handedUser = kept.user;
      noteKind(
        kept.user === undefined
          ? undefined
          : kept.user.isGuestReceiver === true
            ? "guestReceiver"
            : "person",
      );
      setToken(next);
      locator = keepIfGiven(storage, locatorStorageKey, kept.locator) ?? locator;
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
    handedUser: () => handedUser,
    knownAs: () => kind,
    noteKnownAs: noteKind,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}
