// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { highestWithin, linkShare } from "./abr-choice.ts";
import { fireTvLadder, fireTvPlaying, scene, topChunkOnItsWay, useScene } from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

/** The highest quality below the top the link's estimate carries with nothing loaded ahead. */
function carriedBelowTop(linkKbps: number | undefined): number {
  return highestWithin(fireTvLadder, 3, linkShare(0) * (linkKbps ?? 0) * 1000);
}

describe("a chunk given up chooses by the link's estimate (D382 §1)", () => {
  it("lowers a slow chunk's quality only as far as the estimate carries, its sample counted in it, never by that one chunk's rate", () => {
    const chosen = fireTvPlaying();
    // Its first piece at 500 ms, then 2 Mb/s for 1.5 s: a sample, the link slower for a while.
    const chunk = topChunkOnItsWay(chosen, { pieceAtMs: 500, afterBytes: 375_000 });
    expect(chosen.givenUp).toEqual([chunk]);
    expect(scene.linkNotes).toContainEqual(expect.stringMatching(/^sample 4 375000B 1500ms /u));
    const carried = carriedBelowTop(chosen.choosing.linkKbps());
    // The estimate, 11 Mb/s with the sample in it, carries 4 Mb/s; its own rate, 2.7 Mb/s, the lowest.
    expect(carried).toBe(2);
    expect(scene.steps).toContainEqual(
      expect.stringMatching(
        new RegExp(`^ceiling 4 to ${String(carried)}: gave up a chunk of 4,`, "u"),
      ),
    );
    expect(chosen.choosing.nextLevel()).toBe(carried);
  });
});

describe("hls.js's estimator takes a chunk given up as its bytes arrived", () => {
  it("times one whose bytes were arriving from its first piece, never the wait for its first byte", () => {
    const chosen = fireTvPlaying();
    // Its head at 487 ms, its first piece at 500 ms, then 375000 bytes in the 1500 ms to the give-up.
    topChunkOnItsWay(chosen, { pieceAtMs: 500, afterBytes: 375_000 });
    expect(chosen.fedToHls).toEqual([[1500, 375_000]]);
  });

  it("feeds it nothing of one none of whose bytes after its first piece arrived (D382 §2)", () => {
    const chosen = fireTvPlaying();
    topChunkOnItsWay(chosen, { pieceAtMs: 500, afterBytes: 0 });
    expect(chosen.givenUp).toHaveLength(1);
    expect(chosen.fedToHls).toEqual([]);
  });
});
