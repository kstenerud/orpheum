// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  isArrowTheFields,
  nearestInDirection,
  nearestOffTheLine,
  type Box,
  type Caret,
  type Placed,
} from "./focus-engine.ts";

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

describe("the geometric focus search", () => {
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

  it("moves sideways only along its line, never up or down a row at a short row's end", () => {
    const shortRow = grid.filter((each) => each.item !== "2,2" && each.item !== "2,3");
    expect(nearestInDirection(at(2, 1), shortRow, "right")).toBeUndefined();
    expect(nearestInDirection(at(1, 3), grid, "right")).toBeUndefined();
    expect(nearestInDirection(at(2, 1), shortRow, "left")).toBe("2,0");
  });

  it("falls back off the line to the nearest that way, a step across weighing more Left and Right", () => {
    // A field at the right of a heading's line, the next line's field and button under it at the left.
    const codeField: Box = { left: 600, top: 0, right: 800, bottom: 40 };
    const nameField: Placed<string> = {
      item: "name",
      box: { left: 0, top: 60, right: 200, bottom: 100 },
    };
    const activate: Placed<string> = {
      item: "activate",
      box: { left: 220, top: 60, right: 320, bottom: 100 },
    };
    const farDown: Placed<string> = {
      item: "far down",
      box: { left: 400, top: 600, right: 500, bottom: 640 },
    };
    const lines = [nameField, activate, farDown];
    expect(nearestInDirection(codeField, lines, "left")).toBeUndefined();
    expect(nearestOffTheLine(codeField, lines, "left")).toBe("activate");
    expect(nearestOffTheLine(activate.box, lines, "left")).toBe("name");
    expect(nearestOffTheLine(codeField, lines, "right")).toBeUndefined();
    expect(nearestOffTheLine(at(0, 0), grid, "down")).toBe("1,0");
  });

  it("from a header tile goes down to the tile beneath it, not the first of the row", () => {
    const header: Box = { left: 360, top: -200, right: 460, bottom: -120 };
    expect(nearestInDirection(header, grid, "down")).toBe("0,3");
  });
});

/** A one-line field holding three characters, its caret at. */
function caretAt(at: number, end = at): Caret {
  return { isSingleLine: true, selectionStart: at, selectionEnd: end, valueLength: 3 };
}

describe("an arrow in a text field", () => {
  it("is the field's inside the value, and ours at the end it points to", () => {
    expect(isArrowTheFields("left", caretAt(1))).toBe(true);
    expect(isArrowTheFields("right", caretAt(1))).toBe(true);
    expect(isArrowTheFields("left", caretAt(0))).toBe(false);
    expect(isArrowTheFields("right", caretAt(3))).toBe(false);
    expect(isArrowTheFields("right", caretAt(0))).toBe(true);
  });

  it("is the field's while a selection is made, and where the field has no caret", () => {
    expect(isArrowTheFields("left", caretAt(0, 3))).toBe(true);
    const noCaret: Caret = {
      isSingleLine: true,
      selectionStart: undefined,
      selectionEnd: undefined,
      valueLength: 2,
    };
    expect(isArrowTheFields("left", noCaret)).toBe(true);
  });

  it("is always ours up and down from one line, and at the ends from several", () => {
    expect(isArrowTheFields("up", caretAt(1))).toBe(false);
    expect(isArrowTheFields("down", caretAt(1))).toBe(false);
    const lines: Caret = { ...caretAt(1), isSingleLine: false };
    expect(isArrowTheFields("up", lines)).toBe(true);
    expect(isArrowTheFields("down", { ...lines, selectionStart: 3, selectionEnd: 3 })).toBe(false);
    expect(isArrowTheFields("up", { ...lines, selectionStart: 0, selectionEnd: 0 })).toBe(false);
  });
});
