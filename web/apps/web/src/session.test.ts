// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { corePrefix, createSession } from "./session.ts";

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
    session.signIn("token", { deviceId: 42 });
    session.signOutBecause("session_ended");
    expect(session.token()).toBeUndefined();
    expect(session.lastEndReason()).toBe("session_ended");
    expect(createSession(storage).deviceId()).toBe(42);
    session.signIn("next");
    expect(session.lastEndReason()).toBeUndefined();
  });

  it("keeps its device secret past a sign-out, the proof of the device it resumes, and never its token (the third audit's A8)", () => {
    const storage = memoryStorage();
    const session = createSession(storage);
    session.signIn("token-of-42", { deviceId: 42, deviceSecret: "secret-of-42" });
    session.signOut();
    const reloaded = createSession(storage);
    expect(reloaded.token()).toBeUndefined();
    expect(reloaded.deviceProof()).toBe("secret-of-42");
    session.signIn("token-without-a-secret");
    expect(session.deviceProof()).toBe("secret-of-42");
    expect([...storage.items.values()]).not.toContain("token-of-42");
  });

  it("keeps the locator a sign-in hands it across a reload, and ends it with the session (discovery-core.md K7)", () => {
    const storage = memoryStorage();
    const session = createSession(storage);
    session.signIn("token", { deviceId: 42, deviceSecret: "secret", locator: "locator-of-42" });
    expect(createSession(storage).locator()).toBe("locator-of-42");
    session.signOut();
    expect(session.locator()).toBeUndefined();
    expect(createSession(storage).locator()).toBeUndefined();
  });

  it("keeps each core's credentials apart under its own id, and the own origin's where they were", () => {
    const storage = memoryStorage();
    createSession(storage).signIn("own-origin-token");
    createSession(storage, corePrefix("core-a")).signIn("a-token", { locator: "a-locator" });
    const b = createSession(storage, corePrefix("core-b"));
    expect(b.token()).toBeUndefined();
    expect(b.locator()).toBeUndefined();
    expect(createSession(storage, corePrefix("core-a")).token()).toBe("a-token");
    expect(storage.items.get("orpheum.session")).toBe("own-origin-token");
  });
});
