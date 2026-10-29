// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  createTogetherConnection,
  deadAfterUnansweredPings,
  expectedGroupPositionMs,
  longestReconnectWaitMs,
  pingEveryMs,
  reconnectWaitMs,
  stateEveryMs,
  youngPingEveryMs,
  youngPings,
} from "./together-connection.ts";
import { parseTogetherMessage, startingTuning, type TogetherSocketListeners } from "./together.ts";

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
  group: { state: "Paused", positionMs: 1000, rate: 1, fileId: 3 },
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
  it("says hello with the credential, never in an address, and keeps the welcome", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "membership",
      sessionToken: "session",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    expect(sockets.sent[0]).toEqual({
      v: 2,
      type: "hello",
      credential: "membership",
      sessionToken: "session",
      role: "video",
      canSetRate: true,
    });
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    expect(connection.snapshot()).toMatchObject({
      status: "connected",
      memberId: 7,
      isHost: false,
      group: { state: "Paused", positionMs: 1000, fileId: 3 },
      groupReceivedAtMs: 1000,
      roster: [{ id: 7, name: "Gran", isHost: false }],
    });
  });

  it("keeps the timeline first heard while the group plays on, and takes a new one when it changes", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    const socket = sockets.opened[0]!;
    socket.onOpen();
    socket.onMessage(JSON.stringify(welcome));
    const playing = { state: "Playing", positionMs: 10_000, rate: 1, fileId: 3 };
    monotonicNowMs = 2000;
    socket.onMessage(
      JSON.stringify({
        v: 2,
        type: "group",
        seq: 2,
        group: { ...playing, event: "leaderPlays" },
      }),
    );
    // Three seconds on, a play told again, its position late by its own delay on the way.
    monotonicNowMs = 5000;
    socket.onMessage(
      JSON.stringify({
        v: 2,
        type: "group",
        seq: 3,
        group: { ...playing, positionMs: 12_900, event: "played", byMemberId: 9 },
      }),
    );
    expect(connection.snapshot()).toMatchObject({
      group: { positionMs: 10_000, event: "played", byMemberId: 9 },
      groupReceivedAtMs: 2000,
    });
    // A pause is a new timeline.
    monotonicNowMs = 6000;
    socket.onMessage(
      JSON.stringify({
        v: 2,
        type: "group",
        seq: 4,
        group: { ...playing, state: "Paused", positionMs: 14_000 },
      }),
    );
    expect(connection.snapshot()).toMatchObject({
      group: { positionMs: 14_000 },
      groupReceivedAtMs: 6000,
    });
  });

  it("places a playing group by the pong with the shortest round trip, noting each pong (D264)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    const socket = sockets.opened[0]!;
    socket.onOpen();
    socket.onMessage(JSON.stringify(welcome));
    const playing = { state: "Playing", positionMs: 10_000, rate: 1, fileId: 3 };
    // Its group message waited 200 ms in a full queue on the way: it says 10 s at 10.2 s.
    monotonicNowMs = 2000;
    const pingsBefore = sockets.sent.filter((each) => each["type"] === "ping").length;
    socket.onMessage(JSON.stringify({ v: 2, type: "group", seq: 2, group: playing }));
    // A new timeline is pinged for at once.
    expect(sockets.sent.filter((each) => each["type"] === "ping")).toHaveLength(pingsBefore + 1);
    // A pong sent at 2100, back at 2180 with the group at 10.14 s: 40 ms each way.
    monotonicNowMs = 2180;
    socket.onMessage(
      JSON.stringify({
        v: 2,
        type: "pong",
        tMono: 2100,
        group: { ...playing, positionMs: 10_240 },
      }),
    );
    expect(connection.snapshot()).toMatchObject({
      group: { positionMs: 10_240 },
      groupReceivedAtMs: 2180,
      groupPlacedByRttMs: 80,
    });
    expect(sockets.sent.filter((each) => each["type"] === "note").at(-1)).toMatchObject({
      // From 10.18 s, where the group message put it with no round trip yet measured, to 10.28 s.
      note: { kind: "ponged", roundTripMs: 80, aheadOfExpectedMs: 100, why: "placed" },
    });
    // A longer round trip places nothing; a pong of another timeline neither.
    monotonicNowMs = 3500;
    socket.onMessage(
      JSON.stringify({
        v: 2,
        type: "pong",
        tMono: 3000,
        group: { ...playing, positionMs: 11_000 },
      }),
    );
    socket.onMessage(
      JSON.stringify({ v: 2, type: "pong", tMono: 3490, group: { ...playing, fileId: 8 } }),
    );
    expect(connection.snapshot()).toMatchObject({
      group: { positionMs: 10_240, fileId: 3 },
      groupPlacedByRttMs: 80,
    });
    // The longer one is noted with where it said the group is: 11.25 s, against 11.6 s expected.
    // The other timeline's says nothing of this one.
    expect(
      sockets.sent.filter((each) => each["type"] === "note").map((each) => each["note"]),
    ).toEqual([
      { kind: "ponged", roundTripMs: 80, aheadOfExpectedMs: 100, why: "placed" },
      { kind: "ponged", roundTripMs: 500, aheadOfExpectedMs: -350, why: "kept" },
    ]);
    // A new timeline is placed by its group message again, until a pong places it.
    monotonicNowMs = 4000;
    socket.onMessage(
      JSON.stringify({ v: 2, type: "group", seq: 3, group: { ...playing, fileId: 8 } }),
    );
    expect(connection.snapshot().groupPlacedByRttMs).toBeUndefined();
  });

  it("lets a placing age, so a pong whose round trip is a little longer places the group minutes later (D268 §1)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    const socket = sockets.opened[0]!;
    socket.onOpen();
    socket.onMessage(JSON.stringify(welcome));
    const playing = { state: "Playing", positionMs: 10_000, rate: 1, fileId: 3 };
    monotonicNowMs = 2000;
    socket.onMessage(JSON.stringify({ v: 2, type: "group", seq: 2, group: playing }));
    monotonicNowMs = 2040;
    socket.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 2000, group: playing }));
    expect(connection.snapshot().groupPlacedByRttMs).toBe(40);
    // A minute on, 60 ms against 40: off by 30 ms, against 20 ms and 3 ms of age. Kept.
    monotonicNowMs = 62_040;
    socket.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 61_980, group: playing }));
    expect(connection.snapshot().groupPlacedByRttMs).toBe(40);
    // Seven minutes on, 21 ms of age: the 60 ms pong places it.
    monotonicNowMs = 422_040;
    socket.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 421_980, group: playing }));
    expect(connection.snapshot()).toMatchObject({
      groupPlacedByRttMs: 60,
      groupReceivedAtMs: 422_040,
    });
  });

  it("holds a group within 100 ms over two hours on a clock running 100 ppm fast (D268 §1)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    const socket = sockets.opened[0]!;
    socket.onOpen();
    socket.onMessage(JSON.stringify(welcome));
    // The device's clock at the server's time realMs: 100 ppm fast.
    const deviceMs = (realMs: number): number => 1000 + realMs * 1.0001;
    const playingAt = (positionMs: number): Record<string, unknown> => ({
      state: "Playing",
      positionMs,
      rate: 1,
      fileId: 3,
    });
    monotonicNowMs = deviceMs(0);
    socket.onMessage(JSON.stringify({ v: 2, type: "group", seq: 2, group: playingAt(0) }));
    const twoHoursMs = 2 * 60 * 60 * 1000;
    for (let realMs = 0, index = 0; realMs < twoHoursMs; realMs += pingEveryMs, index++) {
      // The shortest round trip comes first; the later ones are 40 to 80 ms, each way alike.
      const rttMs = index === 0 ? 20 : 40 + ((index * 17) % 41);
      monotonicNowMs = deviceMs(realMs + rttMs);
      socket.onMessage(
        JSON.stringify({
          v: 2,
          type: "pong",
          tMono: Math.round(deviceMs(realMs)),
          group: playingAt(realMs + rttMs / 2),
        }),
      );
    }
    monotonicNowMs = deviceMs(twoHoursMs);
    const offMs =
      (expectedGroupPositionMs(connection.snapshot(), monotonicNowMs) ?? 0) - twoHoursMs;
    // Placed once by the first pong, it would be 720 ms ahead.
    expect(Math.abs(offMs)).toBeLessThan(100);
  });

  it("pings a new timeline each second until youngPings are answered, one at a time (D268 §1)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    const socket = sockets.opened[0]!;
    socket.onOpen();
    socket.onMessage(JSON.stringify(welcome));
    monotonicNowMs = 1100;
    socket.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 1000 }));
    const pings = (): number => sockets.sent.filter((each) => each["type"] === "ping").length;
    const before = pings();
    const playing = { state: "Playing", positionMs: 10_000, rate: 1, fileId: 3 };
    socket.onMessage(JSON.stringify({ v: 2, type: "group", seq: 2, group: playing }));
    expect(pings()).toBe(before + 1);
    // Unanswered, the next waits: a link slower than a second never counts it as unanswered.
    vi.advanceTimersByTime(youngPingEveryMs);
    expect(pings()).toBe(before + 1);
    for (let answered = 1; answered < youngPings + 2; answered++) {
      socket.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 1100, group: playing }));
      vi.advanceTimersByTime(youngPingEveryMs);
    }
    expect(pings()).toBe(before + youngPings);
  });

  it("places nothing, and notes nothing of it, on a voice device, which plays nothing", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      role: "panel",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    const socket = sockets.opened[0]!;
    socket.onOpen();
    socket.onMessage(JSON.stringify(welcome));
    const playing = { state: "Playing", positionMs: 10_000, rate: 1, fileId: 3 };
    monotonicNowMs = 2000;
    socket.onMessage(JSON.stringify({ v: 2, type: "group", seq: 2, group: playing }));
    monotonicNowMs = 2180;
    socket.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 2100, group: playing }));
    expect(sockets.sent.filter((each) => each["type"] === "note")).toEqual([]);
  });

  it("follows the group by the welcome's tuning, or by the starting one where it names none (D250 §1)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    expect(connection.snapshot().tuning).toEqual(startingTuning);
    sockets.opened[0]!.onOpen();
    const tuning = {
      leaveDriftUpToMs: 750,
      nudgeMostPercent: 5,
      seekDriftBeyondMs: 4000,
      seekOnlyLeaveDriftUpToMs: 500,
      catchUpBufferPercent: 60,
      settleAfterChangeMs: 450,
      shouldNudgeKeepPitch: false,
      shouldCancelMicrophoneEcho: true,
      shouldBrowserLevelMicrophone: false,
    };
    sockets.opened[0]!.onMessage(JSON.stringify({ ...welcome, tuning }));
    expect(connection.snapshot().tuning).toEqual(tuning);
    sockets.opened[0]!.onMessage(JSON.stringify({ ...welcome, tuning: { nudgeMostPercent: 20 } }));
    expect(connection.snapshot().tuning).toEqual({ ...startingTuning, nudgeMostPercent: 20 });
  });

  it("pings at once and every so often, keeping the shortest round trip it measured", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    expect(sockets.sent.at(-1)).toEqual({ v: 2, type: "ping", tMono: 1000 });
    monotonicNowMs = 1120;
    sockets.opened[0]!.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 1000 }));
    vi.advanceTimersByTime(pingEveryMs);
    expect(sockets.sent.at(-1)).toMatchObject({ type: "ping" });
    monotonicNowMs = 1300;
    sockets.opened[0]!.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 1250 }));
    monotonicNowMs = 1500;
    sockets.opened[0]!.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 1300 }));
    expect(connection.snapshot().minimumRttMs).toBe(50);
  });

  it("reports the player's state in whole numbers when welcomed, when asked, and every minute", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    connection.setPlayer(() => ({ positionMs: 1234.6, isPlaying: true, isBuffering: false }));
    connection.reportState(); // not yet welcomed: nothing goes
    sockets.opened[0]!.onOpen();
    expect(sockets.sent.filter((each) => each["type"] === "state")).toEqual([]);
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    const states = (): Record<string, unknown>[] =>
      sockets.sent.filter((each) => each["type"] === "state");
    expect(states()).toEqual([
      {
        v: 2,
        type: "state",
        positionMs: 1235,
        tMono: 1000,
        playing: true,
        buffering: false,
        rttMs: 0,
      },
    ]);
    connection.reportState();
    expect(states()).toHaveLength(2);
    for (let elapsedMs = 0; elapsedMs < stateEveryMs; elapsedMs += pingEveryMs) {
      sockets.opened[0]!.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 1000 })); // alive
      vi.advanceTimersByTime(pingEveryMs);
    }
    expect(states()).toHaveLength(3);
  });

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
        group: { ...welcome.group, state: "Playing" },
      }),
    );
    connection.command({ command: "changeItem", fileId: 5, positionMs: 61_000.7 });
    expect(sockets.sent.at(-1)).toEqual({
      v: 2,
      type: "command",
      command: "changeItem",
      fileId: 5,
      positionMs: 61_001,
      basedOnSeq: 6,
    });
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

describe("closing and opening again, as a screen does when it remounts", () => {
  it("connects afresh, and a closed socket's last words start nothing", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    connection.close();
    connection.open();
    sockets.opened[0]!.onClose(1006);
    vi.advanceTimersByTime(longestReconnectWaitMs);
    expect(sockets.opened).toHaveLength(2);
    expect(connection.snapshot().status).toBe("connecting");
    sockets.opened[1]!.onOpen();
    sockets.opened[1]!.onMessage(JSON.stringify(welcome));
    expect(connection.snapshot().status).toBe("connected");
  });
});

describe("reading the server's messages", () => {
  it("reads a group whose zeros the server left out, and ignores what it does not know", () => {
    expect(parseTogetherMessage('{"v":2,"type":"group","seq":3,"group":{"state":"Idle"}}')).toEqual(
      {
        type: "group",
        seq: 3,
        group: {
          state: "Idle",
          positionMs: 0,
          rate: 1,
          fileId: 0,
          event: undefined,
          seekedFromMs: 0,
          byMemberId: undefined,
        },
      },
    );
    expect(parseTogetherMessage('{"type":"group","group":{"state":"Dancing"}}')).toBeUndefined();
    expect(parseTogetherMessage("not json")).toBeUndefined();
    expect(parseTogetherMessage('{"type":"rtc","sdp":"x"}')).toBeUndefined();
  });
});

describe("voice signalling on the session socket (D193 §4)", () => {
  /** A connection in role, opened and welcomed as device 9 of member 7. */
  function welcomed(role?: "panel" | "both"): {
    readonly sockets: FakeSockets;
    readonly connection: ReturnType<typeof createTogetherConnection>;
  } {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      role,
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    return { sockets, connection };
  }

  it("says hello in the role it was given, a voice device claiming no player's rate", () => {
    const { sockets, connection } = welcomed("panel");
    expect(sockets.sent[0]).toEqual({ v: 2, type: "hello", credential: "c", role: "panel" });
    expect(connection.snapshot().deviceId).toBe(9);
    expect(welcomed("both").sockets.sent[0]).toMatchObject({ role: "both", canSetRate: true });
  });

  it("sends signalling to the device it names, and hands what others send to its listeners", () => {
    const { sockets, connection } = welcomed("panel");
    const heard: unknown[] = [];
    const unsubscribe = connection.subscribeToVoiceSignals((fromDeviceId, signal) => {
      heard.push({ fromDeviceId, signal });
    });
    const before = connection.snapshot();
    connection.sendVoiceSignal(12, { sdp: "offer" });
    connection.sendVoiceSignal(12, { candidate: '{"candidate":"c1"}' });
    expect(sockets.sent.filter((each) => each["type"] === "rtc")).toEqual([
      { v: 2, type: "rtc", toDeviceId: 12, sdp: "offer" },
      { v: 2, type: "rtc", toDeviceId: 12, candidate: '{"candidate":"c1"}' },
    ]);
    sockets.opened[0]!.onMessage('{"v":2,"type":"rtc","fromDeviceId":12,"sdp":"answer"}');
    sockets.opened[0]!.onMessage('{"v":2,"type":"rtc","fromDeviceId":12,"candidate":"c2"}');
    expect(heard).toEqual([
      { fromDeviceId: 12, signal: { sdp: "answer" } },
      { fromDeviceId: 12, signal: { candidate: "c2" } },
    ]);
    expect(connection.snapshot()).toBe(before);
    unsubscribe();
    sockets.opened[0]!.onMessage('{"v":2,"type":"rtc","fromDeviceId":12,"sdp":"again"}');
    expect(heard).toHaveLength(2);
  });

  it("reads an rtc message's sender and signalling, and none that says nothing", () => {
    expect(parseTogetherMessage('{"v":2,"type":"rtc","fromDeviceId":4,"sdp":"s"}')).toEqual({
      type: "rtc",
      fromDeviceId: 4,
      signal: { sdp: "s" },
    });
    expect(parseTogetherMessage('{"type":"rtc","fromDeviceId":4,"candidate":"c"}')).toEqual({
      type: "rtc",
      fromDeviceId: 4,
      signal: { candidate: "c" },
    });
    expect(parseTogetherMessage('{"type":"rtc","fromDeviceId":4}')).toBeUndefined();
  });
});
