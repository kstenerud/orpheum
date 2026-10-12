// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { noRight } from "./together.ts";
import { followGroup } from "./together-follow.ts";
import { refillAheadMs } from "./together-media.ts";
import { holdLoadingEvent, loadingHoldOf } from "./together-player-events.ts";
import { Refill, seekReadyAheadMs } from "./together-refill.ts";
import { FakeMedia, fakeConnection, paused } from "./test-follow.ts";

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 0;
});

afterEach(() => {
  vi.useRealTimers();
});

/** A video at 60 s, with aheadMs of film loaded there, able to play it. */
function loadedAhead(aheadMs: number): FakeMedia {
  const media = new FakeMedia();
  media.currentTime = 60;
  media.loaded = [[59, 60 + aheadMs / 1000]];
  return media;
}

describe("when a player is ready again (seek-resync-2026-10-12.md)", () => {
  it("is ready a few seconds into a seek when its link carries the film well faster than it plays", () => {
    const refill = new Refill();
    refill.chunkLoaded({
      track: "video",
      loadMs: 3000,
      filmMs: 10_000,
      loadedKbps: 11_000,
      qualityKbps: 36_208,
    });
    refill.heard("seeking", loadedAhead(0));
    expect(refill.isShort(loadedAhead(seekReadyAheadMs - 100))).toBe(true);
    expect(refill.isShort(loadedAhead(seekReadyAheadMs))).toBe(false);
  });

  it("needs ten seconds after a stall, a catch-up's seek, or a seek on a link only just keeping up", () => {
    const stalled = new Refill();
    stalled.chunkLoaded({
      track: "video",
      loadMs: 3000,
      filmMs: 10_000,
      loadedKbps: 11_000,
      qualityKbps: 36_208,
    });
    stalled.heard("waiting", loadedAhead(0));
    expect(stalled.isShort(loadedAhead(seekReadyAheadMs))).toBe(true);
    expect(stalled.isShort(loadedAhead(refillAheadMs))).toBe(false);

    const caughtUp = new Refill();
    caughtUp.chunkLoaded({
      track: "video",
      loadMs: 3000,
      filmMs: 10_000,
      loadedKbps: 11_000,
      qualityKbps: 36_208,
    });
    caughtUp.sought("catchUp");
    caughtUp.heard("seeking", loadedAhead(0));
    expect(caughtUp.isShort(loadedAhead(seekReadyAheadMs))).toBe(true);

    const slowLink = new Refill();
    slowLink.chunkLoaded({
      track: "video",
      loadMs: 8000,
      filmMs: 10_000,
      loadedKbps: 4000,
      qualityKbps: 3000,
    });
    slowLink.heard("seeking", loadedAhead(0));
    expect(slowLink.isShort(loadedAhead(seekReadyAheadMs))).toBe(true);
  });
});

describe("holding its loading while its group waits (the owner's jumps, 2026-10-12)", () => {
  it("holds once ready while the group waits, and lets it go on as the group plays or it stops", () => {
    const media = new FakeMedia();
    const holds: (boolean | undefined)[] = [];
    media.addEventListener(holdLoadingEvent, (event) => holds.push(loadingHoldOf(event)?.isHeld));
    const { connection, setGroup } = fakeConnection(paused);
    const stop = followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs });
    expect(holds).toEqual([]); // paused, not waiting
    setGroup({ ...paused, state: "Waiting" }, 0);
    expect(holds).toEqual([true]);
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000);
    expect(holds).toEqual([true, false]);
    setGroup({ ...paused, state: "Waiting" }, 1000);
    stop();
    expect(holds).toEqual([true, false, true, false]);
  });
});
