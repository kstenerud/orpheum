// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { createTogetherConnection } from "./together-connection.ts";
import { isLeaderOf, leaderOf } from "./together-roster.ts";
import type { TogetherSocketListeners } from "./together.ts";

/** A stand-in for the server's end of one socket: what it heard, and its listeners. */
function oneSocket(): {
  readonly sent: Record<string, unknown>[];
  readonly listeners: () => TogetherSocketListeners;
  readonly open: (listeners: TogetherSocketListeners) => {
    send: (text: string) => void;
    close: () => void;
  };
} {
  const sent: Record<string, unknown>[] = [];
  let opened: TogetherSocketListeners | undefined;
  return {
    sent,
    listeners: () => {
      if (opened === undefined) {
        throw new Error("no socket opened");
      }
      return opened;
    },
    open: (listeners) => {
      opened = listeners;
      return {
        send: (text) => {
          sent.push(JSON.parse(text) as Record<string, unknown>);
        },
        close: () => undefined,
      };
    },
  };
}

/** A connection, welcomed as device 9 with the roster given, and its socket. */
function welcomed(roster: readonly unknown[]): {
  readonly connection: ReturnType<typeof createTogetherConnection>;
  readonly socket: ReturnType<typeof oneSocket>;
} {
  const socket = oneSocket();
  const connection = createTogetherConnection({
    credential: "c",
    openSocket: socket.open,
    monotonicMs: () => 1000,
  });
  connection.open();
  socket.listeners().onOpen();
  socket.listeners().onMessage(
    JSON.stringify({
      v: 2,
      type: "welcome",
      seq: 4,
      memberId: 7,
      deviceId: 9,
      group: { state: "Playing", positionMs: 10_000, rate: 1, fileId: 3 },
      roster,
    }),
  );
  return { connection, socket };
}

describe("a session's leader, as its roster says (D371 §1)", () => {
  it("reads which device leads and how each says it plays the film, and whether this one leads", () => {
    const { connection } = welcomed([
      {
        id: 7,
        name: "Karl",
        devices: [
          { id: 9, role: "video" },
          {
            id: 12,
            role: "video",
            isLeader: true,
            playback: { delivery: "converted", qualityKbps: 4200, linkKbps: 18_000 },
          },
          { id: 13, role: "video", playback: { delivery: "teleported" } },
        ],
      },
    ]);
    const snapshot = connection.snapshot();
    expect(leaderOf(snapshot)).toMatchObject({
      id: 12,
      playback: { delivery: "converted", qualityKbps: 4200, linkKbps: 18_000 },
    });
    expect(isLeaderOf(snapshot)).toBe(false);
    expect(snapshot.roster[0]?.devices[2]?.playback).toBeUndefined();
    expect(
      isLeaderOf(
        welcomed([
          { id: 7, name: "Karl", devices: [{ id: 9, isLeader: true }] },
        ]).connection.snapshot(),
      ),
    ).toBe(true);
    expect(leaderOf(welcomed([]).connection.snapshot())).toBeUndefined();
  });

  it("tells the session in its state how its player plays the film, in whole numbers", () => {
    const { connection, socket } = welcomed([]);
    connection.setPlayer(() => ({
      positionMs: 10_000,
      isPlaying: true,
      isBuffering: false,
      playback: { delivery: "converted", qualityKbps: 4200.4, linkKbps: 17_999.6 },
    }));
    connection.reportState();
    expect(socket.sent.at(-1)).toMatchObject({
      type: "state",
      playback: { delivery: "converted", qualityKbps: 4200, linkKbps: 18_000 },
    });
  });

  it("takes a seek of a group that plays on, with no leader to stand for, for a new timeline (D371 §2)", () => {
    const { connection, socket } = welcomed([]);
    socket.listeners().onMessage(
      JSON.stringify({
        v: 2,
        type: "group",
        seq: 5,
        group: {
          state: "Playing",
          positionMs: 600_000,
          rate: 1,
          fileId: 3,
          event: "seeked",
          seekedFromMs: 10_000,
        },
      }),
    );
    expect(connection.snapshot().group?.positionMs).toBe(600_000);
  });
});
