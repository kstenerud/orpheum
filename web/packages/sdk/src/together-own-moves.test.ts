// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { OwnMoves, ownMarkLastsMs } from "./together-own-moves.ts";

describe("a player's marks on its own moves (D268 §4)", () => {
  it("takes a seek's seeking and seeked for its own, and the next seek for its person's", () => {
    const moves = new OwnMoves();
    moves.mark("seek", 0);
    expect(moves.heard("seeking", 10)).toBe(true);
    expect(moves.heard("seeked", 5000)).toBe(true); // a seek may load for long
    expect(moves.heard("seeking", 5100)).toBe(false);
    expect(moves.heard("seeked", 5200)).toBe(false);
  });

  it("takes a pause for its own once, within the mark's time", () => {
    const moves = new OwnMoves();
    moves.mark("pause", 0);
    expect(moves.isOwn("pause", ownMarkLastsMs)).toBe(true);
    expect(moves.heard("pause", ownMarkLastsMs)).toBe(true);
    expect(moves.heard("pause", ownMarkLastsMs)).toBe(false);
  });

  it("gives up a mark whose event never came, and says which", () => {
    const moves = new OwnMoves();
    moves.mark("pause", 0);
    moves.mark("seek", 100);
    expect(moves.expired(ownMarkLastsMs)).toEqual([]);
    expect(moves.expired(ownMarkLastsMs + 1)).toEqual(["pause"]);
    expect(moves.heard("pause", ownMarkLastsMs + 1)).toBe(false);
    expect(moves.expired(ownMarkLastsMs + 101)).toEqual(["seek"]);
    expect(moves.expired(ownMarkLastsMs + 102)).toEqual([]);
  });
});
