// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { everyRight } from "./together.ts";
import { checkDriftEveryMs, followGroup } from "./together-follow.ts";
import { FakeMedia, FakePage, fakeConnection, paused } from "./test-follow.ts";

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 0;
});

afterEach(() => {
  vi.useRealTimers();
});

/** Advances the timers and the monotonic clock together by ms. */
function advance(ms: number): void {
  monotonicNowMs += ms;
  vi.advanceTimersByTime(ms);
}

describe("a player's word that it followed its group's change (the owner, 2026-10-28)", () => {
  it("says it followed each change at once, once, and none for the group it found", () => {
    const media = new FakeMedia();
    media.paused = false;
    const { connection, setGroup, notes } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs });
    advance(1000);
    setGroup({ ...paused, event: "paused" }, monotonicNowMs);
    advance(3 * checkDriftEveryMs);
    expect(media.paused).toBe(true);
    expect(notes.filter((note) => note.kind === "groupFollowed")).toEqual([
      { kind: "groupFollowed", seq: 1, heldMs: 0, why: "atOnce" },
    ]);
  });

  it("says how long a handheld's hidden page held a change, and that its page held it", () => {
    const media = new FakeMedia();
    const page = new FakePage();
    const { connection, setGroup, notes } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs, page });
    page.hide();
    setGroup({ ...paused, positionMs: 90_000, event: "seeked" }, monotonicNowMs);
    advance(3 * checkDriftEveryMs);
    expect(notes.some((note) => note.kind === "groupFollowed")).toBe(false);
    page.show();
    advance(checkDriftEveryMs);
    expect(notes.filter((note) => note.kind === "groupFollowed")).toEqual([
      { kind: "groupFollowed", seq: 1, heldMs: 4 * checkDriftEveryMs, why: "hidden" },
    ]);
  });
});

describe("a player's word of a play its follower did not make while its group held it (the owner's Marrowgate Family, 2026-10-28)", () => {
  it("says the element's autoplay played it while its group waited, at where it stood, and pauses it", () => {
    const media = new FakeMedia();
    media.autoplay = true;
    media.currentTime = 1607.085;
    const { connection, notes } = fakeConnection({
      ...paused,
      state: "Waiting",
      positionMs: 1_607_085,
    });
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs });
    media.paused = false; // the browser's autoplay, as the film loaded enough
    media.fire("play");
    expect(media.paused).toBe(true);
    expect(notes.filter((note) => note.kind === "playedWhileHeld")).toEqual([
      { kind: "playedWhileHeld", positionMs: 1_607_085, why: "autoplay" },
    ]);
  });

  it("says the page played it where the element does not autoplay, and says nothing of a play with its group", () => {
    const media = new FakeMedia();
    const { connection, setGroup, notes } = fakeConnection({ ...paused, state: "Waiting" });
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs });
    media.paused = false;
    media.fire("play");
    setGroup({ ...paused, state: "Playing", event: "leaderPlays" }, monotonicNowMs);
    media.fire("play");
    expect(notes.filter((note) => note.kind === "playedWhileHeld")).toEqual([
      { kind: "playedWhileHeld", positionMs: 60_000, why: "page" },
    ]);
  });
});
