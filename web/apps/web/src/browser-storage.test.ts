// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { storageIfAllowed } from "./browser-storage.ts";

describe("storageIfAllowed", () => {
  it("gives the storage, or undefined where reading it throws", () => {
    const storage = window.sessionStorage;
    expect(storageIfAllowed(() => storage)).toBe(storage);
    expect(
      storageIfAllowed(() => {
        throw new DOMException("denied", "SecurityError");
      }),
    ).toBeUndefined();
  });
});
