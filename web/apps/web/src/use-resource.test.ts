// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { failure } from "./use-resource.ts";

describe("failure", () => {
  it("says the server's message, else the browser's", () => {
    expect(failure({ error: { code: "not_found", message: "no such title or series" } })).toEqual({
      state: "failed",
      message: "no such title or series",
    });
    expect(failure(new TypeError("Failed to fetch"))).toEqual({
      state: "failed",
      message: "Failed to fetch",
    });
    expect(failure("odd")).toEqual({ state: "failed", message: "odd" });
  });
});
