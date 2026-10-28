// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  askReadyEveryMs,
  MovingDown,
  startOn,
  stillCannotKeepUp,
  type LadderOffer,
} from "./moving-down.ts";

const offer: LadderOffer = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  soundCarriedGainDb: 0,
  isReady: false,
  filmPictureKbps: 9_900,
  linkSharePercent: 70,
  steadiness: 0,
};
const ready: LadderOffer = { ...offer, isReady: true };

/** A MovingDown on a link read as linkKbps, and what it asked and moved to; the film is 9.9 Mb/s. */
function rig(linkKbps: number | undefined): {
  readonly movingDown: MovingDown;
  readonly asked: number[];
  readonly moved: LadderOffer[];
} {
  const asked: number[] = [];
  const moved: LadderOffer[] = [];
  const movingDown = new MovingDown({
    readLinkKbps: () => linkKbps,
    askAgain: () => asked.push(asked.length),
    onMove: (to) => moved.push(to),
  });
  return { movingDown, asked, moved };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a stalled player moving to its lower quality (D253)", () => {
  it("still cannot keep up after stalling again, unmeasured, or with the film over 70% of its link", () => {
    const keeping = {
      hasStalledSinceOffered: false,
      linkKbps: 20_000,
      filmPictureKbps: 9_900,
      linkSharePercent: 70,
    };
    expect(stillCannotKeepUp(keeping)).toBe(false);
    expect(stillCannotKeepUp({ ...keeping, hasStalledSinceOffered: true })).toBe(true);
    expect(stillCannotKeepUp({ ...keeping, linkKbps: undefined })).toBe(true);
    expect(stillCannotKeepUp({ ...keeping, linkKbps: 2_200 })).toBe(true);
    expect(stillCannotKeepUp({ ...keeping, linkKbps: 14_000 })).toBe(true); // 9.9 > 9.8
    expect(stillCannotKeepUp({ ...keeping, filmPictureKbps: 0 })).toBe(true);
  });

  it("weighs the film against the share of its link the offer says (the constants sweep, 2026-10-20)", () => {
    const keeping = {
      hasStalledSinceOffered: false,
      linkKbps: 14_000,
      filmPictureKbps: 9_900,
      linkSharePercent: 80,
    };
    expect(stillCannotKeepUp(keeping)).toBe(false); // 9.9 < 11.2, though over the starting 70%'s 9.8
    expect(stillCannotKeepUp({ ...keeping, linkSharePercent: 0 })).toBe(true); // none said: 70%
  });

  it("asks again every few seconds until it is ready, then moves on a link too slow for the film", () => {
    const { movingDown, asked, moved } = rig(2_200);
    movingDown.answered(offer);
    vi.advanceTimersByTime(askReadyEveryMs * 2);
    expect(asked).toHaveLength(2);
    expect(moved).toEqual([]);
    movingDown.answered(ready);
    vi.advanceTimersByTime(askReadyEveryMs * 2);
    expect(asked).toHaveLength(2); // ready: no more asking
    expect(moved).toEqual([ready]);
    movingDown.answered(ready);
    expect(moved).toHaveLength(1); // once
  });

  it("stays on the film when it keeps up once ready, and moves at its next stall", () => {
    const { movingDown, moved } = rig(50_000);
    movingDown.stalled(); // the first, which started the offer
    movingDown.answered(ready);
    expect(moved).toEqual([]);
    movingDown.stalled();
    movingDown.answered(ready);
    expect(moved).toEqual([ready]);
  });

  it("asks nothing and moves nowhere once stopped", () => {
    const { movingDown, asked, moved } = rig(2_200);
    movingDown.answered(offer);
    movingDown.stop();
    vi.advanceTimersByTime(askReadyEveryMs * 3);
    movingDown.answered(ready);
    expect(asked).toEqual([]);
    expect(moved).toEqual([]);
  });
});

describe("where a chunked player starts on an offer", () => {
  it("is the offer's quality and share of the link, from where the film stood", () => {
    expect(startOn({ ...offer, linkSharePercent: 80 }, 95_000)).toEqual({
      startAtBps: 1_896_000,
      linkSharePercent: 80,
      startPositionS: 95,
      startSteadiness: 0,
    });
  });

  it("knows the link the offer says its device last measured on its network, for its start rule (D257 §1)", () => {
    expect(startOn({ ...offer, rememberedLinkKbps: 9636 }, 0)).toEqual({
      startAtBps: 1_896_000,
      linkSharePercent: 70,
      startPositionS: 0,
      startSteadiness: 0,
      rememberedLinkKbps: 9636,
    });
    expect(startOn(offer, 0)).not.toHaveProperty("rememberedLinkKbps");
  });

  it("starts at the steadiness level the offer says its device last ended at (D352 Q2)", () => {
    expect(startOn({ ...offer, steadiness: 2 }, 0)).toEqual({
      startAtBps: 1_896_000,
      linkSharePercent: 70,
      startPositionS: 0,
      startSteadiness: 2,
    });
  });
});
