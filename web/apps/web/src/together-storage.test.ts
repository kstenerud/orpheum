// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";
import { keepMembership, readMembership } from "./together-storage.ts";

function memoryStorage(): StringStorage {
  const items = new Map<string, string>();
  return storageThatNeverThrows({
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
    removeItem: (key) => {
      items.delete(key);
    },
  });
}

describe("a kept Watch Together membership", () => {
  it("keeps a guest's locator, so the server is found again without the spent link", () => {
    const storage = memoryStorage();
    keepMembership(storage, "viewer", {
      sessionId: 4,
      credential: "a-credential",
      link: "a-secret",
      locator: "a-locator",
    });
    expect(readMembership(storage, "viewer")).toEqual({
      sessionId: 4,
      credential: "a-credential",
      link: "a-secret",
      locator: "a-locator",
    });
  });

  it("reads one kept before memberships kept their locator, as having none", () => {
    const storage = memoryStorage();
    storage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 4, credential: "a-credential" }),
    );
    expect(readMembership(storage, "viewer")).toEqual({ sessionId: 4, credential: "a-credential" });
  });
});
