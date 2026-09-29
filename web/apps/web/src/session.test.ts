// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { createSession } from "./session.ts";

function memoryStorage(): StringStorage & { readonly items: Map<string, string> } {
  const items = new Map<string, string>();
  return {
    items,
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
    removeItem: (key) => {
      items.delete(key);
    },
  };
}

describe("createSession", () => {
  it("keeps the token across a reload and tells its listeners when it changes", () => {
    const storage = memoryStorage();
    const session = createSession(storage);
    let heard = 0;
    session.subscribe(() => {
      heard += 1;
    });
    session.signIn("token-1");
    expect(createSession(storage).token()).toBe("token-1");
    session.signOut();
    expect(createSession(storage).token()).toBeUndefined();
    expect(heard).toBe(2);
  });

  it("lasts as long as the page when storage refuses", () => {
    const refusing: StringStorage = {
      getItem: () => {
        throw new Error("denied");
      },
      setItem: () => {
        throw new Error("denied");
      },
      removeItem: () => {
        throw new Error("denied");
      },
    };
    const session = createSession(refusing);
    session.signIn("token-2");
    expect(session.token()).toBe("token-2");
  });

  it("remembers the device across a sign-out the server caused, and forgets why at the next sign-in", () => {
    const storage = memoryStorage();
    const session = createSession(storage);
    session.signIn("token", 42);
    session.signOutBecause("session_ended");
    expect(session.token()).toBeUndefined();
    expect(session.lastEndReason()).toBe("session_ended");
    expect(createSession(storage).deviceId()).toBe(42);
    session.signIn("next");
    expect(session.lastEndReason()).toBeUndefined();
  });
});
