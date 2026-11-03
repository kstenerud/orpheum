// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { afterEach, describe, expect, it, vi } from "vitest";

/** Whether Array.prototype.toSorted was there when the lab's module was loaded, set by its mock. */
const seen = vi.hoisted(() => ({ toSortedAtLabLoad: undefined as boolean | undefined }));

vi.mock("./lab.ts", () => {
  seen.toSortedAtLabLoad = "toSorted" in Array.prototype;
  return {
    memoryEveryMs: 60_000,
    LabReceiver: class {
      note(): void {}
      sampleMemory(): void {}
      report(): Promise<void> {
        return Promise.resolve();
      }
    },
  };
});

const ownToSorted = Reflect.getOwnPropertyDescriptor(Array.prototype, "toSorted");

afterEach(() => {
  vi.useRealTimers();
  if (ownToSorted !== undefined) {
    Reflect.defineProperty(Array.prototype, "toSorted", ownToSorted);
  }
});

describe("the receiver's entry point", () => {
  it("fills what Chromium 90 lacks before anything it imports is loaded", async () => {
    vi.useFakeTimers();
    // As on the 3rd-generation Chromecast's Chromium 90, which has no toSorted (Chrome 110).
    Reflect.deleteProperty(Array.prototype, "toSorted");
    await import("./main.ts");
    expect(seen.toSortedAtLabLoad).toBe(true);
    expect([3, 1, 2].toSorted()).toEqual([1, 2, 3]);
  });
});
