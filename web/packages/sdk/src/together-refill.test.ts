// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { everyRight, noRight } from "./together.ts";
import { followGroup, scrubPauseWindowMs } from "./together-follow.ts";
import { refillAheadMs } from "./together-media.ts";
import {
  catchUpSeekEvent,
  groupJumpedEvent,
  holdLoadingEvent,
  loadingHoldOf,
  playerLoadedChunkEvent,
} from "./together-player-events.ts";
import { Refill, seekReadyAheadMs } from "./together-refill.ts";
import type { PlayerNote } from "./together-notes.ts";
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
    const { connection, setGroup, notes } = fakeConnection(paused);
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
    // Each told to the server's log too (the owner, 2026-10-12: "add the logging first").
    expect(notes.filter((note) => note.kind === "loadHeld")).toEqual([
      { kind: "loadHeld", why: "held" },
      { kind: "loadHeld", why: "goesOn" },
      { kind: "loadHeld", why: "held" },
      { kind: "loadHeld", why: "goesOn" },
    ]);
  });
});

describe("which ready rule a player says it is ready by (the owner, 2026-10-12)", () => {
  it("tells the group how much film it is ready on and how fast its last chunk loaded", () => {
    const media = new FakeMedia();
    const { connection, reports } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs });
    media.dispatchEvent(
      new CustomEvent(playerLoadedChunkEvent, {
        detail: {
          track: "video",
          loadMs: 3000,
          filmMs: 10_200,
          loadedKbps: 11_000,
          qualityKbps: 36_208,
        },
      }),
    );
    media.fire("seeking");
    connection.reportState();
    expect(reports.at(-1)).toMatchObject({ readyOnMs: seekReadyAheadMs, loadsFaster: 3.4 });
  });
});

describe("a group that seeks again while a player still loads (the owner's jumps, 2026-10-12)", () => {
  it("follows the group's new place at once, not once its last seek has loaded", () => {
    const media = new FakeMedia();
    const { connection, setGroup, notes } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs });
    setGroup({ ...paused, state: "Waiting", positionMs: 3_501_848 }, 0);
    media.seeking = true; // its chunk there is not made yet
    setGroup({ ...paused, state: "Waiting", positionMs: 5_372_332 }, 0);
    const holds = notes.filter((note) => note.kind === "sought" && note.why === "hold");
    expect(holds.map((note) => (note.kind === "sought" ? note.toMs : 0))).toEqual([
      60_000, 3_501_848, 5_372_332,
    ]);
    setGroup({ ...paused, state: "Waiting", positionMs: 5_373_000 }, 0); // still loading, as near
    expect(notes.filter((note) => note.kind === "sought")).toHaveLength(3);
  });
});

describe("a catch-up's seek, which is no jump (D250 §5, D284 §2)", () => {
  it("is told to its player just before it seeks, so its requests do not say a seek took it there", () => {
    const media = new FakeMedia();
    media.currentTime = 100;
    const { connection } = fakeConnection({
      ...paused,
      state: "Playing",
      positionMs: 100_000,
    });
    followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs });
    monotonicNowMs = 1000;
    media.currentTime = 101;
    media.fire("progress");
    media.currentTime = 50; // a stall the group played on through
    media.fire("waiting");
    expect(media.dispatched).toContain(catchUpSeekEvent);
    expect(media.currentTime).toBe(101); // it jumped to the group, which measured no lead yet
  });
});

describe("telling its player of the group's jumps (D286 §3)", () => {
  it("tells each jump once, and nothing else", () => {
    const media = new FakeMedia();
    const { connection, setGroup } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs });
    const jumps = (): number =>
      media.dispatched.filter((event) => event === groupJumpedEvent).length;
    setGroup(
      { ...paused, state: "Waiting", event: "seeked", seekedFromMs: 60_000, positionMs: 900_000 },
      0,
    );
    vi.advanceTimersByTime(3000);
    expect(jumps()).toBe(1);
    setGroup({ ...paused, state: "Playing", event: "leaderPlays", positionMs: 900_000 }, 0);
    setGroup(
      { ...paused, state: "Waiting", event: "seeked", seekedFromMs: 902_000, positionMs: 40_000 },
      0,
    );
    expect(jumps()).toBe(2);
  });
});

describe("a player that may seek, joining a group already playing (the owner's test, 2026-10-12)", () => {
  it("sends no jump it makes as it loads, before it is once in step, and its person's after", () => {
    const media = new FakeMedia();
    const { connection, setGroup, commands } = fakeConnection({
      ...paused,
      state: "Playing",
      positionMs: 5_424_000,
    });
    media.readyState = 0; // nothing loaded yet: the follower moves nothing
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs });
    media.readyState = 4;
    media.currentTime = 5416.678; // where its player put itself as it loaded
    media.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([]);
    setGroup({ ...paused, state: "Paused", positionMs: 5_416_678 }, 0); // held where it is: in step
    media.fire("seeking"); // the browser tells of the follower's own seeks so far
    media.fire("seeked");
    media.currentTime = 5800;
    media.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs); // a drag's last seek is the one sent
    expect(commands).toEqual([{ command: "seek", positionMs: 5_800_000 }]);
  });
});

describe("a hole in what a player held for its group's wait loaded (the owner's cast, 2026-10-28)", () => {
  /** Whether a player held at 1549.673 s after a seek, loaded to 1554.72 s past a hole of holeS, is ready. */
  function isReadyAcross(holeS: number): boolean {
    const media = new FakeMedia();
    media.currentTime = 1549.673;
    media.loaded = [
      [1547.76, 1551.21],
      [1551.21 + holeS, 1554.72],
    ];
    const { connection, reports } = fakeConnection({
      ...paused,
      state: "Waiting",
      event: "seeked",
      positionMs: 1_549_673,
    });
    followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs });
    media.dispatchEvent(
      new CustomEvent(playerLoadedChunkEvent, {
        detail: {
          track: "video",
          loadMs: 649,
          filmMs: 2753,
          loadedKbps: 10_595,
          qualityKbps: 13_758,
        },
      }),
    );
    media.fire("seeking");
    media.fire("seeked");
    connection.reportState();
    return reports.at(-1)?.isBuffering === false;
  }

  it("is ready across a hole as narrow as a playing player steps over, which a paused one never does", () => {
    // Firefox on a Mac, cast to, held paused at 1549.673 s with a hole of 60 ms in its sound 1.5 s on,
    // said it buffered as long as it was held, and its group waited on it for good.
    expect(isReadyAcross(0.06)).toBe(true);
  });

  it("is not ready across a wider hole, which would stall it as it plays", () => {
    expect(isReadyAcross(0.5)).toBe(false);
  });

  it("says once for each wait it holds up what holds it, so one held for good says why", () => {
    const media = new FakeMedia();
    media.currentTime = 60;
    media.loaded = [
      [59, 60.5],
      [61, 80],
    ];
    const waiting = { ...paused, state: "Waiting", event: "seeked" } as const;
    const { connection, setGroup, notes } = fakeConnection(waiting);
    followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs });
    media.fire("seeking");
    media.fire("seeked");
    connection.reportState();
    connection.reportState();
    const said = (): PlayerNote[] => notes.filter((note) => note.kind === "buffering");
    expect(said()).toEqual([
      {
        kind: "buffering",
        why: "seeking",
        positionMs: 60_000,
        readyState: 4,
        rate: 1,
        loadedAheadMs: 500,
      },
    ]);
    setGroup({ ...waiting, seekedFromMs: 60_000 }, 0); // the group's next wait
    connection.reportState();
    expect(said()).toHaveLength(2);
  });
});
