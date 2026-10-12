// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { noRight } from "./together.ts";
import { checkDriftEveryMs, followGroup } from "./together-follow.ts";
import { FakeMedia, fakeConnection, paused } from "./test-follow.ts";
import { memoryOfTheSession, runningPositionMs } from "./together-start.ts";

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 0;
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a player's start, early by how late it starts or running up to it (D282)", () => {
  it("places a group before it starts as far short of where it starts as it is early", () => {
    const group = { positionMs: 60_000, rate: 1 };
    expect(runningPositionMs(group, 3000, 1000)).toBe(58_000);
    expect(runningPositionMs(group, 3000, 3500)).toBe(60_500);
  });

  it("presses play early by the lag it learned, held where the group starts until then", () => {
    const media = new FakeMedia();
    const { connection, setGroup, reports } = fakeConnection(paused);
    const startLag = memoryOfTheSession();
    startLag.keep({ costMs: 170, moves: 3 });
    followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs, startLag });
    connection.reportState();
    expect(reports.at(-1)?.startLagMs).toBe(170); // the group gives it no run-up
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing", startsInMs: 500 }, 1000); // it starts at 1500
    expect(media.paused).toBe(true);
    expect(media.currentTime).toBe(60);
    monotonicNowMs = 1329;
    vi.advanceTimersByTime(329);
    expect(media.paused).toBe(true);
    monotonicNowMs = 1330;
    vi.advanceTimersByTime(1);
    expect(media.paused).toBe(false);
  });

  it("with no lag learned, runs up from short of where the group starts, and learns its lag on the way", () => {
    const media = new FakeMedia();
    const { connection, setGroup, notes, reports } = fakeConnection(paused);
    const startLag = memoryOfTheSession();
    followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs, startLag });
    connection.reportState();
    expect(reports.at(-1)?.startLagMs).toBeUndefined(); // the group gives it a run-up
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing", startsInMs: 2000 }, 1000); // it starts at 3000
    expect(notes.at(-1)).toEqual({
      kind: "sought",
      positionMs: 60_000,
      toMs: 58_000,
      why: "runUp",
    });
    expect(media.currentTime).toBe(58);
    media.fire("seeked");
    expect(media.paused).toBe(false); // it runs up at once, its run-up loaded
    media.fire("playing");
    // Its picture moved 170 ms after it pressed play: at 2000 it is that far short of the run.
    monotonicNowMs = 2000;
    media.currentTime = 58 + 0.83;
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(startLag.read()).toEqual({ costMs: 170, moves: 1 });
    connection.reportState();
    expect(reports.at(-1)?.startLagMs).toBe(170);
  });

  it("runs up no earlier than what it loaded where the group starts, starting once the run reaches it", () => {
    const media = new FakeMedia();
    media.loaded = [[59, 7200]]; // the chunk the group starts in begins at 59 s
    const { connection, setGroup, notes } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs });
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing", startsInMs: 2000 }, 1000); // it starts at 3000
    expect(notes.at(-1)).toEqual({
      kind: "sought",
      positionMs: 60_000,
      toMs: 59_000,
      why: "runUp",
    });
    media.fire("seeked");
    expect(media.paused).toBe(true); // the group's run is at 58 s, short of where it loaded
    monotonicNowMs = 2000;
    vi.advanceTimersByTime(1000);
    expect(media.paused).toBe(false);
  });
});
