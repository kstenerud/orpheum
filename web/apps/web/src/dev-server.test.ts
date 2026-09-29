// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { devServerOrigin } from "./dev-server";

describe("devServerOrigin", () => {
  it("refuses to serve without an address", () => {
    expect(() => devServerOrigin(undefined, "serve")).toThrow("ORPHEUM_DEV_SERVER is not set");
    expect(() => devServerOrigin(" ", "serve")).toThrow("ORPHEUM_DEV_SERVER is not set");
  });

  it("serves the address it was given", () => {
    expect(devServerOrigin(" http://127.0.0.1:8432 ", "serve")).toBe("http://127.0.0.1:8432");
  });

  it("needs no address to build", () => {
    expect(devServerOrigin(undefined, "build")).toBeUndefined();
  });
});
