// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { appendPage } from "./use-pages.ts";

describe("appendPage", () => {
  it("adds a page after what came before, and takes its cursor", () => {
    expect(appendPage({ items: [1, 2], nextCursor: "a" }, { items: [3], nextCursor: "" })).toEqual({
      items: [1, 2, 3],
      nextCursor: "",
    });
  });
});
