// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { appended } from "./use-pages.ts";

describe("appended", () => {
  it("adds a page after what came before, and takes its cursor", () => {
    expect(appended({ items: [1, 2], next: "a" }, { items: [3], next: "" })).toEqual({
      items: [1, 2, 3],
      next: "",
    });
  });
});
