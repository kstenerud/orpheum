// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { rowsWithPosters } from "./grid-columns.ts";

// A 16:9 still stands in a poster's height only in a row that holds an upright poster (D495 §4).
describe("which cards share a row with an upright poster", () => {
  it("marks every card of a row with a poster in it, and none of a row of stills alone", () => {
    const upright = [false, false, true, false, false, false, false];
    expect(rowsWithPosters(upright, 3)).toEqual([true, true, true, false, false, false, false]);
  });

  it("reads a short last row as a row of its own", () => {
    expect(rowsWithPosters([false, false, false, true], 3)).toEqual([false, false, false, true]);
  });

  it("takes one card a row where the grid has no columns yet", () => {
    expect(rowsWithPosters([false, true], 0)).toEqual([false, true]);
  });
});
