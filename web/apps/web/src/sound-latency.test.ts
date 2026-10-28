// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { soundLatencyKey, soundLatencyMemory } from "./sound-latency.ts";

/** A storage in memory, holding what is set on it. */
function heldStorage(): {
  readonly storage: () => StringStorage;
  readonly held: Map<string, string>;
} {
  const held = new Map<string, string>();
  const storage: StringStorage = {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => {
      held.set(key, value);
    },
    removeItem: (key) => {
      held.delete(key);
    },
  };
  return { storage: () => storage, held };
}

describe("this device's memory of its sound's latency (a Fire TV, 2026-10-28)", () => {
  it("keeps a latency in the browser's storage, for the device's next playback", () => {
    const { storage, held } = heldStorage();
    const memory = soundLatencyMemory(storage);
    expect(memory.recall()).toBeUndefined();
    memory.keep(331);
    expect(held.get(soundLatencyKey)).toBe("331");
    expect(soundLatencyMemory(storage).recall()).toBe(331);
  });

  it("reads nothing kept that is no latency, and keeps none", () => {
    const { storage, held } = heldStorage();
    for (const kept of ["", "-5", "3.5", "2001", "loud"]) {
      held.set(soundLatencyKey, kept);
      expect(soundLatencyMemory(storage).recall()).toBeUndefined();
    }
    held.delete(soundLatencyKey);
    for (const ms of [-1, 2.5, 2001, Number.NaN]) {
      soundLatencyMemory(storage).keep(ms);
    }
    expect(held.has(soundLatencyKey)).toBe(false);
  });
});
