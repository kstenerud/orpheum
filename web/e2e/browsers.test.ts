// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { pagesAtOnce } from "./browsers.ts";

describe("the test pages each browser runs at once", () => {
  it("is one for every two cores, rounded down", () => {
    expect([8, 12, 4, 5].map(pagesAtOnce)).toEqual([4, 6, 2, 2]);
  });

  it("is one on a machine of one or two cores", () => {
    expect([1, 2].map(pagesAtOnce)).toEqual([1, 1]);
  });
});
