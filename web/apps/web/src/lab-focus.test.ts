// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { directionOfKey, nearestInDirection, type Box, type Placed } from "./lab-focus.ts";

/** A 100-pixel tile at a row and column of a grid with 20-pixel gaps. */
function tile(row: number, column: number): Placed<string> {
  const left = column * 120;
  const top = row * 120;
  return {
    item: `${String(row)},${String(column)}`,
    box: { left, top, right: left + 100, bottom: top + 100 },
  };
}

const grid = [0, 1, 2].flatMap((row) => [0, 1, 2, 3].map((column) => tile(row, column)));

/** The box of the tile at row and column. */
function at(row: number, column: number): Box {
  return tile(row, column).box;
}

describe("the input probe's geometric focus search", () => {
  it("moves to the next tile each way, and nowhere past the grid's edge", () => {
    expect(nearestInDirection(at(1, 1), grid, "right")).toBe("1,2");
    expect(nearestInDirection(at(1, 1), grid, "left")).toBe("1,0");
    expect(nearestInDirection(at(1, 1), grid, "up")).toBe("0,1");
    expect(nearestInDirection(at(1, 1), grid, "down")).toBe("2,1");
    expect(nearestInDirection(at(0, 3), grid, "right")).toBeUndefined();
    expect(nearestInDirection(at(0, 0), grid, "up")).toBeUndefined();
  });

  it("goes straight on over a nearer tile off to the side", () => {
    const below: Placed<string> = {
      item: "below",
      box: { left: 0, top: 300, right: 100, bottom: 400 },
    };
    const nearerAside: Placed<string> = {
      item: "aside",
      box: { left: 250, top: 150, right: 350, bottom: 250 },
    };
    expect(nearestInDirection(at(0, 0), [nearerAside, below], "down")).toBe("below");
  });

  it("from a header tile goes down to the tile beneath it, not the first of the row", () => {
    const header: Box = { left: 360, top: -200, right: 460, bottom: -120 };
    expect(nearestInDirection(header, grid, "down")).toBe("0,3");
  });

  it("names the arrows by key, by an old browser's names, and by keyCode", () => {
    expect(directionOfKey("ArrowUp", 38)).toBe("up");
    expect(directionOfKey("Left", 0)).toBe("left");
    expect(directionOfKey("Unidentified", 40)).toBe("down");
    expect(directionOfKey("Enter", 13)).toBeUndefined();
  });
});
