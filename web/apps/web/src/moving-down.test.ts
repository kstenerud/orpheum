// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  askReadyEveryMs,
  MovingDown,
  stillCannotKeepUp,
  type LowerQuality,
} from "./moving-down.ts";

const offer: LowerQuality = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  soundCarriedGainDb: 0,
  isReady: false,
  filmPictureKbps: 9_900,
};
const ready: LowerQuality = { ...offer, isReady: true };

/** A MovingDown on a link read as linkKbps, and what it asked and moved to; the film is 9.9 Mb/s. */
function rig(linkKbps: number | undefined): {
  readonly movingDown: MovingDown;
  readonly asked: number[];
  readonly moved: LowerQuality[];
} {
  const asked: number[] = [];
  const moved: LowerQuality[] = [];
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
    const keeping = { hasStalledSinceOffered: false, linkKbps: 20_000, filmPictureKbps: 9_900 };
    expect(stillCannotKeepUp(keeping)).toBe(false);
    expect(stillCannotKeepUp({ ...keeping, hasStalledSinceOffered: true })).toBe(true);
    expect(stillCannotKeepUp({ ...keeping, linkKbps: undefined })).toBe(true);
    expect(stillCannotKeepUp({ ...keeping, linkKbps: 2_200 })).toBe(true);
    expect(stillCannotKeepUp({ ...keeping, linkKbps: 14_000 })).toBe(true); // 9.9 > 9.8
    expect(stillCannotKeepUp({ ...keeping, filmPictureKbps: 0 })).toBe(true);
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
