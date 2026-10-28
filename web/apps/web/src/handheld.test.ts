// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { pageThatPausesWhenHidden } from "./handheld.ts";

/** A browser's signs: its touch points, and whether its main pointer is a finger. */
function signs(maxTouchPoints: number, isCoarse: boolean) {
  return { maxTouchPoints, matchMedia: () => ({ matches: isCoarse }) };
}

/** A page, as a following player reads it. */
const page = {
  visibilityState: "visible" as const,
  addEventListener: () => undefined,
  removeEventListener: () => undefined,
};

describe("the page a following player waits on while it is hidden", () => {
  it("is a handheld's, whose film is paused as it hides", () => {
    expect(pageThatPausesWhenHidden(page, signs(5, true))).toBe(page);
  });

  it("is none on a computer, whose hidden page plays its film on and follows its group (the owner, 2026-10-28)", () => {
    expect(pageThatPausesWhenHidden(page, signs(0, false))).toBeUndefined();
    // A computer with a touch screen keeps a mouse or a trackpad as its main pointer.
    expect(pageThatPausesWhenHidden(page, signs(10, false))).toBeUndefined();
  });
});
