// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { noRight } from "./together.ts";
import type { TogetherConnection } from "./together-connection.ts";
import {
  FakeMedia,
  fakeConnection,
  followGroup,
  paused,
  stopEveryFollower,
} from "./test-follow.ts";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  stopEveryFollower();
  vi.useRealTimers();
});

/** connection as a guest's device 8 sees it, leading its session or following the host's 40. */
function asGuest(connection: TogetherConnection, isLeading: boolean): TogetherConnection {
  const device = (id: number, isLeader: boolean) =>
    ({
      id,
      role: "video",
      isBuffering: false,
      cannotKeepUp: undefined,
      status: "watching",
      isLeader,
    }) as const;
  return {
    ...connection,
    snapshot: () => ({
      ...connection.snapshot(),
      deviceId: 8,
      roster: [
        { id: 1, name: "owner", isHost: true, devices: [device(40, !isLeading)] },
        { id: 2, name: "partner", isHost: false, devices: [device(8, isLeading)] },
      ],
    }),
  };
}

describe("a Watch Together player at its film's end (D516)", () => {
  it.each([
    ["leading", true],
    ["following", false],
  ])(
    "a guest's player %s says its film ended in the state it tells as it ends, and not before",
    (_, isLeading) => {
      const media = new FakeMedia();
      media.currentTime = 2_390;
      const { connection, reports } = fakeConnection({ ...paused, state: "playing" });
      followGroup(media, asGuest(connection, isLeading), {
        fileId: 5,
        leads: noRight,
        monotonicMs: () => 0,
      });
      connection.reportState();
      expect(reports.at(-1)).not.toHaveProperty("isAtTheEnd");
      media.currentTime = 2_400;
      media.ended = true;
      media.paused = true;
      media.fire("pause");
      expect(reports.at(-1)?.isAtTheEnd).toBe(true);
    },
  );
});
