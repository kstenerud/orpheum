// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { upNextLeadMs } from "./up-next-lead.ts";
import { isPlayableNext, noOfferChoice as noChoice, offerState } from "./up-next-offer.ts";

describe("when Up next shows and counts (plans/up-next.md)", () => {
  const durationMs = 1_200_000;

  it("shows within its lead of the end and on the end screen, and not before", () => {
    const at = (atMs: number, hasEnded = false): boolean =>
      offerState({ atMs, durationMs, hasEnded }, noChoice, true).isShown;
    expect(at(durationMs - upNextLeadMs - 1000)).toBe(false);
    expect(at(durationMs - upNextLeadMs + 1000)).toBe(true);
    expect(at(durationMs, true)).toBe(true);
    expect(offerState({ atMs: 0, durationMs: 0, hasEnded: false }, noChoice, true).isShown).toBe(
      false,
    );
  });

  it("counts only once the film has ended, with autoplay on, until a press stops it", () => {
    const nearTheEnd = { atMs: durationMs - 5000, durationMs, hasEnded: false };
    const ended = { atMs: durationMs, durationMs, hasEnded: true };
    expect(offerState(nearTheEnd, noChoice, true).isCounting).toBe(false);
    expect(offerState(ended, noChoice, true).isCounting).toBe(true);
    expect(offerState(ended, noChoice, false)).toMatchObject({ isShown: true, isCounting: false });
    expect(offerState(ended, { ...noChoice, isCancelled: true }, true)).toMatchObject({
      isShown: true,
      isCounting: false,
    });
    expect(offerState(ended, { isDismissed: true, isCancelled: true }, true).isShown).toBe(false);
  });

  it("shows and counts from the credits' start where the file has a credits segment (D139, D339)", () => {
    const creditsStartMs = 1_100_000;
    const at = (atMs: number) =>
      offerState({ atMs, durationMs, hasEnded: false, creditsStartMs }, noChoice, true);
    // Credits a minute and more before the fixed lead: the offer shows and counts from them.
    expect(at(creditsStartMs - 1000)).toMatchObject({ isShown: false, isCounting: false });
    expect(at(creditsStartMs)).toMatchObject({ isShown: true, isCounting: true });
    expect(at(durationMs - upNextLeadMs + 1000)).toMatchObject({ isShown: true, isCounting: true });
    // Credits shorter than the lead: nothing shows in the lead before them.
    const late = { atMs: 0, durationMs, hasEnded: false, creditsStartMs: durationMs - 5000 };
    expect(
      offerState({ ...late, atMs: durationMs - upNextLeadMs + 1000 }, noChoice, true),
    ).toMatchObject({ isShown: false, isCounting: false });
  });
});

describe("an episode before or after the film (D555 §3)", () => {
  it("plays where there is one and nothing keeps it from playing", () => {
    expect(isPlayableNext(undefined)).toBe(false);
    expect(isPlayableNext({})).toBe(true);
    expect(isPlayableNext({ unavailable: { reason: "fileMissing" } })).toBe(false);
  });
});
