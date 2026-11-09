// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  createTogetherConnection,
  deadAfterUnansweredPings,
  longestReconnectWaitMs,
  pingEveryMs,
  reconnectWaitMs,
} from "./together-connection.ts";
import type { TogetherSocketListeners } from "./together.ts";

/** A stand-in for the server's end of each socket the connection opens. */
interface FakeSockets {
  readonly opened: TogetherSocketListeners[];
  /** Every message sent on every socket, parsed. */
  readonly sent: Record<string, unknown>[];
  readonly closedHere: number[];
  readonly open: (listeners: TogetherSocketListeners) => {
    send: (text: string) => void;
    close: () => void;
  };
}

function fakeSockets(): FakeSockets {
  const opened: TogetherSocketListeners[] = [];
  const sent: Record<string, unknown>[] = [];
  const closedHere: number[] = [];
  return {
    opened,
    sent,
    closedHere,
    open: (listeners) => {
      opened.push(listeners);
      const index = opened.length - 1;
      return {
        send: (text) => {
          sent.push(JSON.parse(text) as Record<string, unknown>);
        },
        close: () => {
          closedHere.push(index);
        },
      };
    },
  };
}

const welcome = {
  v: 2,
  type: "welcome",
  seq: 4,
  sessionId: 1,
  memberId: 7,
  deviceId: 9,
  group: { state: "paused", positionMs: 1000, rate: 1, fileId: 3 },
  roster: [{ id: 7, name: "Gran", devices: [{ id: 9, role: "video" }] }],
};

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 1000.4;
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a device's connection to its session (cut-a §5.1)", () => {
  it("reads a roster device's word that it cannot keep up, with both numbers (D247 §8)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    sockets.opened[0]!.onMessage(
      JSON.stringify({
        v: 2,
        type: "roster",
        roster: [
          {
            id: 7,
            name: "Gran",
            devices: [{ id: 9, role: "video", cannotKeepUp: { linkKbps: 600, lowestKbps: 750 } }],
          },
          {
            id: 8,
            name: "Kim",
            devices: [{ id: 10, role: "video", cannotKeepUp: { lowestKbps: 750 } }],
          },
        ],
      }),
    );
    const devices = connection.snapshot().roster.map((member) => member.devices[0]?.cannotKeepUp);
    expect(devices).toEqual([
      { linkKbps: 600, lowestKbps: 750 },
      { linkKbps: undefined, lowestKbps: 750 },
    ]);
  });

  it("sends the host's command based on the group's last seq", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify({ ...welcome, isHost: true }));
    sockets.opened[0]!.onMessage(
      JSON.stringify({
        v: 2,
        type: "group",
        seq: 6,
        group: { ...welcome.group, state: "playing" },
      }),
    );
    expect(connection.command({ command: "changeItem", fileId: 5, positionMs: 61_000.7 })).toBe(
      true,
    );
    expect(sockets.sent.at(-1)).toEqual({
      v: 2,
      type: "command",
      command: "changeItem",
      fileId: 5,
      positionMs: 61_001,
      basedOnSeq: 6,
    });
  });

  it("says a command did not go while the connection is down, and sends nothing", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify({ ...welcome, isHost: true }));
    sockets.opened[0]!.onClose(1006);
    const sentBefore = sockets.sent.length;
    expect(connection.command({ command: "stop" })).toBe(false);
    expect(sockets.sent).toHaveLength(sentBefore);
  });

  it("connects again after a drop, waiting longer each time, and says hello again", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    sockets.opened[0]!.onClose(1006);
    expect(connection.snapshot().status).toBe("reconnecting");
    vi.advanceTimersByTime(reconnectWaitMs(0));
    expect(sockets.opened).toHaveLength(2);
    sockets.opened[1]!.onOpen(); // reached the server, which dropped it
    sockets.opened[1]!.onClose(1006);
    vi.advanceTimersByTime(reconnectWaitMs(0));
    expect(sockets.opened).toHaveLength(2);
    vi.advanceTimersByTime(reconnectWaitMs(1) - reconnectWaitMs(0));
    expect(sockets.opened).toHaveLength(3);
    sockets.opened[2]!.onOpen();
    expect(sockets.sent.at(-1)).toMatchObject({ type: "hello", credential: "c" });
    expect(reconnectWaitMs(30)).toBe(longestReconnectWaitMs);
  });

  it("takes a socket nothing is heard on for two pings for dead, and connects again", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome)); // its first ping goes out
    vi.advanceTimersByTime(pingEveryMs); // the second, the first unanswered
    sockets.opened[0]!.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 1000 }));
    vi.advanceTimersByTime(pingEveryMs * deadAfterUnansweredPings);
    expect(connection.snapshot().status).toBe("connected"); // heard from since: two unanswered
    vi.advanceTimersByTime(pingEveryMs);
    expect(connection.snapshot().status).toBe("reconnecting");
    expect(sockets.closedHere).toEqual([0]);
    sockets.opened[0]!.onClose(1006); // the dead socket's late close starts nothing more
    vi.advanceTimersByTime(reconnectWaitMs(0));
    expect(sockets.opened).toHaveLength(2);
    sockets.opened[1]!.onOpen();
    sockets.opened[1]!.onMessage(JSON.stringify(welcome));
    expect(sockets.sent.find((each) => each["type"] === "note")?.["note"]).toMatchObject({
      kind: "reconnected",
      why: "silent",
    });
  });

  it("sends its player's notes in whole numbers, and says how long it was away once back", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    connection.note({
      kind: "nudged",
      positionMs: 1234.6,
      driftMs: 80.2,
      rate: 1.05,
      returnCostMs: 220,
    });
    expect(sockets.sent.at(-1)).toEqual({
      v: 2,
      type: "note",
      note: { kind: "nudged", positionMs: 1235, driftMs: 80, rate: 1.05, returnCostMs: 220 },
    });
    sockets.opened[0]!.onClose(1006);
    connection.note({ kind: "sought", positionMs: 0, toMs: 1, why: "hold" }); // not connected
    monotonicNowMs = 4000;
    vi.advanceTimersByTime(reconnectWaitMs(0));
    sockets.opened[1]!.onOpen();
    sockets.opened[1]!.onMessage(JSON.stringify(welcome));
    const notes = sockets.sent.filter((each) => each["type"] === "note");
    expect(notes.map((each) => each["note"])).toEqual([
      { kind: "nudged", positionMs: 1235, driftMs: 80, rate: 1.05, returnCostMs: 220 },
      { kind: "reconnected", offlineMs: 3000, why: "closed" },
    ]);
  });

  it("ends, saying so, when its guest's place moved to another device by a new link", () => {
    const moved = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: moved.open,
      monotonicMs,
    });
    connection.open();
    moved.opened[0]!.onOpen();
    moved.opened[0]!.onMessage(JSON.stringify(welcome));
    moved.opened[0]!.onMessage(JSON.stringify({ v: 2, type: "ended", reason: "movedAway" }));
    moved.opened[0]!.onClose(1000);
    vi.advanceTimersByTime(longestReconnectWaitMs);
    expect(connection.snapshot()).toMatchObject({ status: "ended", endReason: "movedAway" });
    expect(moved.opened).toHaveLength(1);
  });

  it("ends, without connecting again, when the server ends it or refuses the hello", () => {
    const ended = fakeSockets();
    const endedConnection = createTogetherConnection({
      credential: "c",
      openSocket: ended.open,
      monotonicMs,
    });
    endedConnection.open();
    ended.opened[0]!.onOpen();
    ended.opened[0]!.onMessage(JSON.stringify(welcome));
    ended.opened[0]!.onMessage(JSON.stringify({ v: 2, type: "ended", reason: "ejected" }));
    ended.opened[0]!.onClose(1000);
    vi.advanceTimersByTime(longestReconnectWaitMs);
    expect(endedConnection.snapshot()).toMatchObject({ status: "ended", endReason: "ejected" });
    expect(ended.opened).toHaveLength(1);

    const refused = fakeSockets();
    const refusedConnection = createTogetherConnection({
      credential: "gone",
      openSocket: refused.open,
      monotonicMs,
    });
    refusedConnection.open();
    refused.opened[0]!.onOpen();
    refused.opened[0]!.onClose(1008);
    vi.advanceTimersByTime(longestReconnectWaitMs);
    expect(refusedConnection.snapshot()).toMatchObject({ status: "ended", endReason: "gone" });
    expect(refused.opened).toHaveLength(1);

    // A hello reaching a cast just after it ended is told why before the refusal (the server's
    // CastEndedError): a remote opened as its receiver took control back says so.
    const late = fakeSockets();
    const lateConnection = createTogetherConnection({
      credential: "a cast's",
      openSocket: late.open,
      monotonicMs,
    });
    lateConnection.open();
    late.opened[0]!.onOpen();
    late.opened[0]!.onMessage(JSON.stringify({ v: 2, type: "ended", reason: "takenBack" }));
    late.opened[0]!.onClose(1008);
    vi.advanceTimersByTime(longestReconnectWaitMs);
    expect(lateConnection.snapshot()).toMatchObject({ status: "ended", endReason: "takenBack" });
    expect(late.opened).toHaveLength(1);
  });
});
