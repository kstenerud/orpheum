// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { everyRight, type GroupView, type RosterDevice, type RosterMember } from "./together.ts";
import type { TogetherConnection } from "./together-connection.ts";
import {
  FakeMedia,
  fakeConnection,
  paused,
  followGroup,
  stopEveryFollower,
} from "./test-follow.ts";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  stopEveryFollower();
  vi.useRealTimers();
});

function device(id: number, role: string, isLeader = false): RosterDevice {
  return { id, role, isBuffering: false, cannotKeepUp: undefined, status: "watching", isLeader };
}

/** The host's player leading its session of one, with its panel. */
function leadingHost(connection: TogetherConnection): TogetherConnection {
  const member: RosterMember = {
    id: 1,
    name: "owner",
    isHost: true,
    devices: [device(1, "video", true), device(2, "panel")],
  };
  return { ...connection, snapshot: () => ({ ...connection.snapshot(), roster: [member] }) };
}

describe("a host's film put on paused (the owner, 2026-11-23)", () => {
  it("is never played for the group by the play its own leader made as the film was put on", () => {
    // The host's player, paused and loaded, is put on: the group Waits for its leader, which plays
    // at once, the film loaded; the host's pause that follows reaches it, and its play's event
    // after that, taken for its person's play at a paused group, played the group again.
    const media = new FakeMedia();
    media.currentTime = 60;
    const idle: GroupView = { ...paused, state: "idle", fileId: 0 };
    const fake = fakeConnection(idle);
    followGroup(media, leadingHost(fake.connection), {
      fileId: 5,
      leads: everyRight,
      monotonicMs: () => 0,
    });
    fake.setGroup({ ...paused, state: "waiting", event: "changedItem" }, 0);
    expect(media.paused, "the leader plays as the group waits").toBe(false);
    fake.setGroup({ ...paused, state: "paused", event: "paused" }, 10);
    expect(media.paused).toBe(true);
    media.fire("play"); // the element's play event, which follows its play's call
    media.fire("pause");
    vi.advanceTimersByTime(3000);
    expect(fake.commands).toEqual([]);
  });

  it("still sends a person's play at a paused group", () => {
    const media = new FakeMedia();
    media.currentTime = 60;
    const fake = fakeConnection(paused);
    followGroup(media, leadingHost(fake.connection), {
      fileId: 5,
      leads: everyRight,
      monotonicMs: () => 0,
    });
    media.paused = false;
    media.fire("play");
    expect(fake.commands).toEqual([{ command: "play" }]);
  });
});
