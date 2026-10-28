// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { noRight, type RosterDevice, type RosterMember } from "./together.ts";
import type { TogetherConnection } from "./together-connection.ts";
import { checkDriftEveryMs, followGroup } from "./together-follow.ts";
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

/** This device, 8, in the roster: a cast's receiver, which plays. */
const thisDevice = 8;

/** A device of the roster, playing or a panel. */
function device(id: number, role: string): RosterDevice {
  return { id, role, isBuffering: false, cannotKeepUp: undefined, status: "watching" };
}

/** connection, its snapshot's roster the person's devices, this device among them. */
function withRoster(
  connection: TogetherConnection,
  devices: readonly RosterDevice[],
): TogetherConnection {
  const member: RosterMember = { id: 1, name: "owner", isHost: true, devices };
  return {
    ...connection,
    snapshot: () => ({ ...connection.snapshot(), deviceId: thisDevice, roster: [member] }),
  };
}

/**
 * A player that plays a second behind where its group reckons it should be once the group plays:
 * its session's devices those given.
 */
function aSecondBehind(devices: readonly RosterDevice[]): {
  media: FakeMedia;
  notes: ReturnType<typeof fakeConnection>["notes"];
} {
  const media = new FakeMedia();
  const { connection, setGroup, notes } = fakeConnection(paused);
  followGroup(media, withRoster(connection, devices), { fileId: 5, leads: noRight, monotonicMs });
  monotonicNowMs = 1000;
  setGroup({ ...paused, state: "Playing" }, 1000);
  monotonicNowMs = 1000 + checkDriftEveryMs;
  vi.advanceTimersByTime(checkDriftEveryMs); // the element did not move on: a second behind
  return { media, notes };
}

describe("a following player that is its session's only player (the owner's cast, 2026-10-28)", () => {
  it("is not nudged towards where the group reckons it should be: the group follows it", () => {
    const { media, notes } = aSecondBehind([device(thisDevice, "both"), device(33, "panel")]);
    expect(media.playbackRate).toBe(1);
    expect(notes.filter((note) => note.kind === "nudged")).toEqual([]);
  });

  it("is nudged as before with another player in its session", () => {
    const { media, notes } = aSecondBehind([
      device(thisDevice, "both"),
      device(33, "panel"),
      device(40, "video"),
    ]);
    expect(media.playbackRate).toBeCloseTo(1.1);
    expect(notes.filter((note) => note.kind === "nudged")).toHaveLength(1);
  });

  it("still seeks to a group far beyond the seek threshold, as a player joining a playing film finds it", () => {
    const { media, notes } = aSecondBehind([device(thisDevice, "both")]);
    monotonicNowMs = 6000;
    vi.advanceTimersByTime(checkDriftEveryMs); // five seconds behind
    expect(notes.filter((note) => note.kind === "sought" && note.why === "catchUp")).toHaveLength(
      1,
    );
    expect(media.currentTime).toBeGreaterThanOrEqual(65); // where the group is, or ahead of it
  });
});

describe("a following player that begins to hold its playing group up", () => {
  it("says what holds it, its readyState and rate, what it has loaded ahead and how long since its rate changed", () => {
    const { media, notes } = aSecondBehind([device(thisDevice, "both"), device(40, "video")]);
    expect(media.playbackRate).toBeCloseTo(1.1); // nudged at 2 s
    monotonicNowMs = 2350;
    media.currentTime = 61;
    media.loaded = [[0, 72]];
    media.readyState = 2;
    media.fire("waiting");
    expect(notes.filter((note) => note.kind === "buffering")).toEqual([
      {
        kind: "buffering",
        why: "waiting",
        positionMs: 61_000,
        readyState: 2,
        rate: media.playbackRate,
        loadedAheadMs: 11_000,
        sinceRateChangeMs: 350,
      },
    ]);
    media.fire("waiting");
    expect(notes.filter((note) => note.kind === "buffering")).toHaveLength(1); // said once
  });
});
