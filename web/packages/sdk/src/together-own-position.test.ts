// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { noRight, type RosterDevice } from "./together.ts";
import type { TogetherConnection } from "./together-connection.ts";
import {
  FakeMedia,
  fakeConnection,
  paused,
  followGroup,
  stopEveryFollower,
} from "./test-follow.ts";

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 0;
});

afterEach(() => {
  stopEveryFollower();
  vi.useRealTimers();
});

/** A player of the roster, leading its session or not. */
function player(id: number, isLeader: boolean): RosterDevice {
  return {
    id,
    role: "video",
    isBuffering: false,
    cannotKeepUp: undefined,
    status: "watching",
    isLeader,
  };
}

/** What a test says of the session as the device sees it: its own id, and which device leads. */
interface Seen {
  deviceId: number;
  leaderId: number | undefined;
}

/** connection, its roster this device and another, 40, which leads as seen says. */
function asSeen(connection: TogetherConnection, seen: Seen): TogetherConnection {
  return {
    ...connection,
    snapshot: () => ({
      ...connection.snapshot(),
      deviceId: seen.deviceId,
      roster: [
        { id: 1, name: "owner", isHost: true, devices: [player(40, seen.leaderId === 40)] },
        {
          id: 2,
          name: "partner",
          isHost: false,
          devices: [player(seen.deviceId, seen.leaderId === seen.deviceId)],
        },
      ],
    }),
  };
}

describe("a player's own position (D377)", () => {
  it("is one it says it has only once it has played along with its group: a page loaded afresh has none (§4)", () => {
    const media = new FakeMedia();
    media.currentTime = 60;
    const { connection, setGroup, reports } = fakeConnection(paused);
    followGroup(media, asSeen(connection, { deviceId: 8, leaderId: 40 }), {
      fileId: 5,
      leads: noRight,
      monotonicMs,
    });
    connection.reportState();
    expect(reports.at(-1)?.isPlaceRemembered).toBeUndefined(); // held where its paused group stands
    setGroup({ ...paused, state: "Playing" }, 0);
    connection.reportState();
    expect(media.paused).toBe(false);
    expect(reports.at(-1)?.isPlaceRemembered).toBe(true);
  });

  it("is kept on connecting again, told in its first state, until it hears of a leader, whose position it then takes (§3)", () => {
    const media = new FakeMedia();
    media.currentTime = 60;
    const seen: Seen = { deviceId: 8, leaderId: 40 };
    const { connection, setGroup, reports } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media, asSeen(connection, seen), { fileId: 5, leads: noRight, monotonicMs });
    connection.reportState(); // plays along with the group device 40 leads
    media.currentTime = 75; // it played on from what it had loaded, its connection gone
    monotonicNowMs = 15_000;
    // Its welcome on a new connection, as a new device: everyone left meanwhile, so the server holds
    // the group where the last went, and no device leads it.
    seen.deviceId = 9;
    seen.leaderId = undefined;
    setGroup({ ...paused, positionMs: 62_000, isHeldPlaying: true }, monotonicNowMs);
    connection.reportState(); // its first state, at its welcome
    expect(media.currentTime).toBe(75);
    expect(reports.at(-1)).toMatchObject({ positionMs: 75_000, isPlaceRemembered: true });
    // Another came back first and leads: it takes the leader's position, as any follower does.
    seen.leaderId = 40;
    setGroup({ ...paused, positionMs: 62_000, isHeldPlaying: true }, monotonicNowMs);
    expect(media.currentTime).toBe(62);
  });
});
