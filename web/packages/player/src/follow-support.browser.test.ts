// SPDX-License-Identifier: MPL-2.0

import Hls from "hls.js";
import { describe, expect, it } from "vitest";

import { canFollowAGroup } from "@orpheum/sdk";

describe("whether a page can follow a Watch Together group, in a real browser", () => {
  it("answers as hls.js's own Hls.isSupported() does, which it stands in for to keep hls.js off the home page (W1)", () => {
    expect(canFollowAGroup()).toBe(Hls.isSupported());
  });
});
