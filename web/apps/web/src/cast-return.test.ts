// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { castReturnKey, forgetReturn, noteReturn, takeReturn } from "./cast-return.ts";

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

describe("where a cast's receiver goes back to (D338 §1)", () => {
  it("is the page the cast took the tab over from, kept as the page loads again receiving it", () => {
    const storage = memoryStorage();
    noteReturn(storage, { castId: 5, route: { screen: "title", itemId: 12 }, isReceiving: false });
    // Loaded again, the tab is handed the same cast on its Play page.
    noteReturn(storage, { castId: 5, route: { screen: "play", fileId: 7 }, isReceiving: false });
    expect(takeReturn(storage)).toEqual({ screen: "title", itemId: 12 });
    expect(storage.getItem(castReturnKey)).toBeNull();
  });

  it("is kept through a cast that replaces the one received, and a new cast notes its own", () => {
    const storage = memoryStorage();
    noteReturn(storage, { castId: 5, route: { screen: "title", itemId: 12 }, isReceiving: false });
    noteReturn(storage, { castId: 6, route: { screen: "play", fileId: 7 }, isReceiving: true });
    expect(takeReturn(storage)).toEqual({ screen: "title", itemId: 12 });
    noteReturn(storage, { castId: 6, route: { screen: "title", itemId: 12 }, isReceiving: false });
    noteReturn(storage, { castId: 7, route: { screen: "home" }, isReceiving: false });
    expect(takeReturn(storage)).toEqual({ screen: "home" });
  });

  it("is home where none was noted, and nothing once forgotten", () => {
    const storage = memoryStorage();
    expect(takeReturn(storage)).toEqual({ screen: "home" });
    noteReturn(storage, { castId: 5, route: { screen: "title", itemId: 12 }, isReceiving: false });
    forgetReturn(storage);
    expect(storage.getItem(castReturnKey)).toBeNull();
  });
});
