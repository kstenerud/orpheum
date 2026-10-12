// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { startLagMemory } from "./together-storage.ts";

/** A storage that keeps what it is given in a map. */
function mapStorage(): StringStorage & { readonly kept: Map<string, string> } {
  const kept = new Map<string, string>();
  return {
    kept,
    getItem: (key) => kept.get(key) ?? null,
    setItem: (key, value) => {
      kept.set(key, value);
    },
    removeItem: (key) => {
      kept.delete(key);
    },
  };
}

describe("how late this browser's player starts, kept for every session here (D282)", () => {
  it("keeps what was learned, and takes anything else under its key for nothing learned", () => {
    const storage = mapStorage();
    expect(startLagMemory(storage).read()).toBeUndefined();
    startLagMemory(storage).keep({ costMs: 170, moves: 2 });
    expect(startLagMemory(storage).read()).toEqual({ costMs: 170, moves: 2 });
    storage.setItem("orpheum.together.startLag", "not json");
    expect(startLagMemory(storage).read()).toBeUndefined();
    storage.setItem("orpheum.together.startLag", '{"costMs":"slow"}');
    expect(startLagMemory(storage).read()).toBeUndefined();
  });
});
