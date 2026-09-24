// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { devAllowedHosts } from "./dev-hosts";

describe("devAllowedHosts", () => {
  it("is empty when the variable is unset or blank", () => {
    expect(devAllowedHosts(undefined)).toEqual([]);
    expect(devAllowedHosts("")).toEqual([]);
    expect(devAllowedHosts(" , ")).toEqual([]);
  });

  it("splits a comma-separated list and trims each name", () => {
    expect(devAllowedHosts("media.lan, box.home ,")).toEqual(["media.lan", "box.home"]);
  });
});
