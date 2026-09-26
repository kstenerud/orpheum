// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { jassubTimeOffset } from "./subtitles.ts";

describe("the subtitle offset", () => {
  it("draws earlier cues to show subtitles later, and later cues to show them earlier", () => {
    expect(jassubTimeOffset(1500)).toBe(-1.5);
    expect(jassubTimeOffset(-250)).toBe(0.25);
  });
});
