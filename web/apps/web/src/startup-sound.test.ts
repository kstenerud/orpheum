// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { moveSoundEarlier } from "@orpheum/player";
import { describe, expect, it } from "vitest";

import { soundOf } from "./startup-sound.ts";

describe("how a playback's sound reaches the speakers as it starts", () => {
  it("says how much earlier the player moves it (the owner's TCL TV, 2026-11-06)", () => {
    const video = document.createElement("video");
    expect(soundOf(video)).toEqual({});
    moveSoundEarlier(video, 300);
    expect(soundOf(video)).toEqual({ earlierMs: 300 });
  });
});
