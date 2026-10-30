// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { forgetReceiving, markReceiving, wasReceiving } from "./cast-receiving-mark.ts";

/** A tab's storage in memory. */
function memoryStorage(): StringStorage {
  const items = new Map<string, string>();
  return {
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
    removeItem: (key) => {
      items.delete(key);
    },
  };
}

describe("the mark that a tab receives a cast (D383 §2)", () => {
  it("is kept from the cast's arrival until receiving ends here", () => {
    const storage = memoryStorage();
    expect(wasReceiving(storage)).toBe(false);
    markReceiving(storage, 5);
    markReceiving(storage, 6); // a cast replacing it, or moving on
    expect(wasReceiving(storage)).toBe(true);
    forgetReceiving(storage);
    expect(wasReceiving(storage)).toBe(false);
  });
});
