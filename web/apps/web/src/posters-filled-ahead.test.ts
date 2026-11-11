// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { fillOrder, postersFilledEachWay } from "./posters-filled-ahead.ts";

describe("fillOrder", () => {
  it("fills the way the person last moved first, then the other (the owner, 2026-11-11)", () => {
    const down = fillOrder(500, 6, 1000, "down");
    expect(down.slice(0, 3)).toEqual([501, 502, 503]);
    expect(down[postersFilledEachWay]).toBe(499);
    const up = fillOrder(500, 6, 1000, "up");
    expect(up.slice(0, 3)).toEqual([499, 498, 497]);
    expect(up[postersFilledEachWay]).toBe(501);
  });

  it("fills a row down, then a row up, and so on, before the person has moved (the owner, 2026-11-11)", () => {
    // Six across, the anchor at 15: its row is 12 to 17, a row down 18 to 23, a row up 6 to 11.
    const order = fillOrder(15, 6, 1000, undefined);
    expect(order.slice(0, 5)).toEqual([12, 13, 14, 16, 17]);
    expect(order.slice(5, 11)).toEqual([18, 19, 20, 21, 22, 23]);
    expect(order.slice(11, 17)).toEqual([6, 7, 8, 9, 10, 11]);
    expect(order.slice(17, 23)).toEqual([24, 25, 26, 27, 28, 29]);
  });

  it("keeps to the hundred either side, and to the list's ends", () => {
    const order = fillOrder(500, 6, 1000, "down");
    expect(order).toHaveLength(2 * postersFilledEachWay);
    expect(Math.max(...order)).toBe(500 + postersFilledEachWay);
    expect(Math.min(...order)).toBe(500 - postersFilledEachWay);
    expect(fillOrder(2, 6, 10, undefined).toSorted((one, other) => one - other)).toEqual([
      0, 1, 3, 4, 5, 6, 7, 8, 9,
    ]);
  });
});
