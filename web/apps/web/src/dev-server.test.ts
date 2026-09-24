// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { devServer } from "./dev-server";

describe("devServer", () => {
  it("refuses to serve without an address", () => {
    expect(() => devServer(undefined, "serve")).toThrow("ORPHEUM_DEV_SERVER is not set");
    expect(() => devServer(" ", "serve")).toThrow("ORPHEUM_DEV_SERVER is not set");
  });

  it("serves the address it was given", () => {
    expect(devServer(" http://127.0.0.1:8432 ", "serve")).toBe("http://127.0.0.1:8432");
  });

  it("needs no address to build", () => {
    expect(devServer(undefined, "build")).toBeUndefined();
  });
});
