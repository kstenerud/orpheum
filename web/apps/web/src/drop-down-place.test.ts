// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { grownPlace } from "./drop-down-place.ts";

/** A screen of 1000 by 800, its whole the room a menu has. */
const room = { left: 0, top: 0, width: 1000, height: 800 };

/** A button of 40 by 40 at left and top. */
const buttonAt = (left: number, top: number) => ({ left, top, width: 40, height: 40 });

// The owner, 2026-11-23: "make all of the bottom button menus look like they're coming out of the
// icons themselves", and "the same grow-out treatment to the top bar buttons".
describe("where a menu grown out of its button goes (grownPlace)", () => {
  it("grows up from a button in the screen's lower half, from the button's start, its tab at its foot's start", () => {
    expect(
      grownPlace({ opener: buttonAt(100, 700), popUp: { width: 200, height: 300 }, room }),
    ).toEqual({ left: 100, top: 440, height: undefined, tabOffset: 0, isDown: false });
  });

  it("grows down from a button in the screen's upper half, the top bar's, its tab at its head", () => {
    expect(
      grownPlace({ opener: buttonAt(100, 20), popUp: { width: 200, height: 300 }, room }),
    ).toEqual({ left: 100, top: 20, height: undefined, tabOffset: 0, isDown: true });
  });

  it("moves back from the room's right edge, the tab following the button to the menu's end", () => {
    const placed = grownPlace({
      opener: buttonAt(950, 700),
      popUp: { width: 200, height: 300 },
      room,
    });
    expect(placed.left).toBe(800); // its right at the room's
    expect(placed.tabOffset).toBe(150); // the tab still on the button
  });

  it("never moves so far back that the tab leaves the menu, past the room's edge where it must", () => {
    const placed = grownPlace({
      opener: buttonAt(950, 700),
      popUp: { width: 200, height: 300 },
      room: { ...room, width: 960 },
    });
    expect(placed.left).toBe(790); // the room would have it at 760
    expect(placed.tabOffset + 40).toBe(200); // the tab's end the menu's end
  });

  it("is cut at the room's top, its list scrolling, where it is taller than the room above the button", () => {
    expect(
      grownPlace({ opener: buttonAt(100, 450), popUp: { width: 200, height: 600 }, room }),
    ).toEqual({ left: 100, top: 0, height: 490, tabOffset: 0, isDown: false });
  });

  it("is cut at the room's foot where it is taller than the room below the button", () => {
    expect(
      grownPlace({ opener: buttonAt(100, 100), popUp: { width: 200, height: 800 }, room }),
    ).toEqual({ left: 100, top: 100, height: 700, tabOffset: 0, isDown: true });
  });
});
