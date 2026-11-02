// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { chipsShown } from "./lab-buttons.ts";

describe("the button test's chips", () => {
  it("shows the volume chips only once heard, before Other", () => {
    expect(chipsShown([])).not.toContain("volumeUp");
    const shown = chipsShown(["up", "volumeDown"]);
    expect(shown).toContain("volumeDown");
    expect(shown).not.toContain("volumeUp");
    expect(shown.at(-1)).toBe("other");
    expect(shown.indexOf("volumeDown")).toBe(shown.length - 2);
  });

  it("shows the keyboard's + and - and its track keys once heard, which a remote has not", () => {
    expect(chipsShown([])).not.toContain("plus");
    const shown = chipsShown(["plus", "minus", "nextTrack", "previousTrack"]);
    expect(shown.slice(-5)).toEqual(["plus", "minus", "previousTrack", "nextTrack", "other"]);
  });
});
