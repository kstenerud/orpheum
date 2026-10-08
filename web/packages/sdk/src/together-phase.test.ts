// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { nextPhase, startingPhase, type CorrectionPhase } from "./together-phase.ts";

const nudging: CorrectionPhase = { kind: "nudging", untilMs: 5000 };
const returning: CorrectionPhase = { kind: "returning", behindMs: 230 };
const skipping: CorrectionPhase = { kind: "skipping", behindMs: -200 };
const waitingAhead: CorrectionPhase = { kind: "waitingAhead", atMs: 90_000 };
const every = [startingPhase, nudging, returning, skipping, waitingAhead];

describe("a following player's correction phase", () => {
  it("begins following, and nudges, jumps ahead or skips from wherever it is", () => {
    expect(startingPhase).toEqual({ kind: "following" });
    for (const phase of every) {
      expect(nextPhase(phase, { kind: "nudged", untilMs: 9000 })).toEqual({
        kind: "nudging",
        untilMs: 9000,
      });
      expect(nextPhase(phase, { kind: "jumpedAhead", atMs: 60_000 })).toEqual({
        kind: "waitingAhead",
        atMs: 60_000,
      });
      expect(nextPhase(phase, { kind: "skipped", behindMs: -150 })).toEqual({
        kind: "skipping",
        behindMs: -150,
      });
    }
  });

  it("returns when its rate goes back to the group's, and a rate changed elsewhere ends a return", () => {
    expect(nextPhase(nudging, { kind: "rateChanged", returnedBehindMs: 120 })).toEqual({
      kind: "returning",
      behindMs: 120,
    });
    expect(nextPhase(returning, { kind: "rateChanged", returnedBehindMs: undefined })).toEqual(
      startingPhase,
    );
    // A nudge going to another nudge's rate is still nudging; the nudge that follows says until when.
    expect(nextPhase(nudging, { kind: "rateChanged", returnedBehindMs: undefined })).toEqual(
      nudging,
    );
    expect(nextPhase(skipping, { kind: "rateChanged", returnedBehindMs: undefined })).toEqual(
      skipping,
    );
  });

  it("follows again once a return or a skip is learned, or a seek makes it no one's cost", () => {
    for (const event of [{ kind: "learned" }, { kind: "sought" }] as const) {
      expect(nextPhase(returning, event)).toEqual(startingPhase);
      expect(nextPhase(skipping, event)).toEqual(startingPhase);
      // A seek during a nudge leaves its end where it was, and one ahead leaves the wait.
      expect(nextPhase(nudging, event)).toEqual(nudging);
      expect(nextPhase(waitingAhead, event)).toEqual(waitingAhead);
    }
  });

  it("stops waiting ahead when the wait is over, and nothing else stops by it", () => {
    expect(nextPhase(waitingAhead, { kind: "stoppedWaiting" })).toEqual(startingPhase);
    for (const phase of [startingPhase, nudging, returning, skipping]) {
      expect(nextPhase(phase, { kind: "stoppedWaiting" })).toEqual(phase);
    }
  });
});
