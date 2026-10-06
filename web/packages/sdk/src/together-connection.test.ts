// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  createTogetherConnection,
  deadAfterUnansweredPings,
  longestReconnectWaitMs,
  pingEveryMs,
  reconnectWaitMs,
  stateEveryMs,
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
  roster: [{ id: 7, name: "Gran", devices: [{ id: 9, role: "playback" }] }],
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
      role: "playback",
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
        group: { ...playing, event: "everyoneReady" },
      }),
    );
    // Three seconds on, "continuing without" someone, its position late by its own delay on the way.
    monotonicNowMs = 5000;
    socket.onMessage(
      JSON.stringify({
        v: 2,
        type: "group",
        seq: 3,
        group: { ...playing, positionMs: 12_900, event: "continuedWithout", byMemberId: 9 },
      }),
    );
    expect(connection.snapshot()).toMatchObject({
      group: { positionMs: 10_000, event: "continuedWithout", byMemberId: 9 },
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
      catchUpBufferPercent: 60,
      settleAfterChangeMs: 450,
      shouldNudgeKeepPitch: false,
      shouldProcessMicrophone: true,
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

  it("tells the group, in its state, that even its lowest quality cannot keep up, and when it can again (D247 §8)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    connection.setPlayer(() => ({ positionMs: 0, isPlaying: false, isBuffering: true }));
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    const lastState = (): Record<string, unknown> | undefined =>
      sockets.sent.filter((each) => each["type"] === "state").at(-1);
    connection.setCannotKeepUp({ linkKbps: 612.4, lowestKbps: 750 });
    expect(lastState()?.["cannotKeepUp"]).toEqual({ linkKbps: 612, lowestKbps: 750 });
    connection.setCannotKeepUp({ linkKbps: undefined, lowestKbps: 750 });
    expect(lastState()?.["cannotKeepUp"]).toEqual({ lowestKbps: 750 });
    connection.setCannotKeepUp(undefined);
    expect(lastState()).not.toHaveProperty("cannotKeepUp");
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
            devices: [
              { id: 9, role: "playback", cannotKeepUp: { linkKbps: 600, lowestKbps: 750 } },
            ],
          },
          {
            id: 8,
            name: "Kim",
            devices: [{ id: 10, role: "playback", cannotKeepUp: { lowestKbps: 750 } }],
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
    connection.note({ kind: "nudged", positionMs: 1234.6, driftMs: 80.2, rate: 1.05 });
    expect(sockets.sent.at(-1)).toEqual({
      v: 2,
      type: "note",
      note: { kind: "nudged", positionMs: 1235, driftMs: 80, rate: 1.05 },
    });
    sockets.opened[0]!.onClose(1006);
    connection.note({ kind: "sought", positionMs: 0, toMs: 1, why: "hold" }); // not connected
    monotonicNowMs = 4000;
    vi.advanceTimersByTime(reconnectWaitMs(0));
    sockets.opened[1]!.onOpen();
    sockets.opened[1]!.onMessage(JSON.stringify(welcome));
    const notes = sockets.sent.filter((each) => each["type"] === "note");
    expect(notes.map((each) => each["note"])).toEqual([
      { kind: "nudged", positionMs: 1235, driftMs: 80, rate: 1.05 },
      { kind: "reconnected", offlineMs: 3000, why: "closed" },
    ]);
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
  function welcomed(role?: "voice" | "both"): {
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
    const { sockets, connection } = welcomed("voice");
    expect(sockets.sent[0]).toEqual({ v: 2, type: "hello", credential: "c", role: "voice" });
    expect(connection.snapshot().deviceId).toBe(9);
    expect(welcomed("both").sockets.sent[0]).toMatchObject({ role: "both", canSetRate: true });
  });

  it("sends signalling to the device it names, and hands what others send to its listeners", () => {
    const { sockets, connection } = welcomed("voice");
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
