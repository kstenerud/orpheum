// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { removeUnownedKeys } from "./browser-storage.ts";
import { clearCookies } from "./loader-sweep.ts";

/** A Storage of these items, which says what it holds. */
function storageOf(items: Record<string, string>): Storage {
  const held = new Map(Object.entries(items));
  return {
    get length() {
      return held.size;
    },
    key: (index) => [...held.keys()][index] ?? null,
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => {
      held.set(key, value);
    },
    removeItem: (key) => {
      held.delete(key);
    },
    clear: () => {
      held.clear();
    },
  };
}

describe("what the loader clears at its start (D308 §7)", () => {
  it("expires every cookie it can see, for its host and each domain above it", () => {
    const written: string[] = [];
    clearCookies(
      {
        read: () => "tossed=1; another=2",
        write: (cookie) => {
          written.push(cookie);
        },
      },
      "k7xq2.client.example",
    );
    expect(written).toEqual([
      "tossed=; Max-Age=0; Path=/",
      "tossed=; Max-Age=0; Path=/; Domain=k7xq2.client.example",
      "tossed=; Max-Age=0; Path=/; Domain=client.example",
      "another=; Max-Age=0; Path=/",
      "another=; Max-Age=0; Path=/; Domain=k7xq2.client.example",
      "another=; Max-Age=0; Path=/; Domain=client.example",
    ]);
  });

  it("removes every Orpheum key it does not own, such as the sessions the whole client kept, and no other's", () => {
    const storage = storageOf({
      "orpheum.loader.households": "[]",
      "orpheum.core.k7xq2.session": "a token",
      "orpheum.discovery.core": "{}",
      "someone.else": "theirs",
    });
    removeUnownedKeys(storage, (key) => key === "orpheum.loader.households");
    expect([0, 1, 2].map((index) => storage.key(index))).toEqual([
      "orpheum.loader.households",
      "someone.else",
      null,
    ]);
  });
});
