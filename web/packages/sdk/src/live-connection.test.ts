// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { createLiveConnection, parseLiveMessage } from "./live-connection.ts";
import { pingEveryMs, reconnectWaitMs } from "./together-connection.ts";
import type { TogetherSocketListeners } from "./together.ts";

/** A stand-in for the server's end of each socket: what was opened, and every message sent. */
function fakeSockets(): {
  readonly opened: TogetherSocketListeners[];
  readonly sent: Record<string, unknown>[];
  readonly open: (listeners: TogetherSocketListeners) => {
    send: (text: string) => void;
    close: () => void;
  };
} {
  const opened: TogetherSocketListeners[] = [];
  const sent: Record<string, unknown>[] = [];
  return {
    opened,
    sent,
    open: (listeners) => {
      opened.push(listeners);
      return {
        send: (text) => {
          sent.push(JSON.parse(text) as Record<string, unknown>);
        },
        close: () => undefined,
      };
    },
  };
}

const monotonicMs = (): number => 1000;

afterEach(() => {
  vi.useRealTimers();
});

describe("a page's live connection (D337 §2)", () => {
  it("says hello with the session token, never in the address, and keeps the devices it is told", () => {
    const sockets = fakeSockets();
    const connection = createLiveConnection({
      sessionToken: "session",
      client: "index-T3st.js",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    expect(sockets.sent[0]).toEqual({
      type: "live",
      sessionToken: "session",
      client: "index-T3st.js",
    });
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "devices", deviceIds: [3, 4] }));
    expect(connection.snapshot()).toMatchObject({ isConnected: true, deviceIds: [3, 4] });
  });

  it("connects again after a drop, with the growing wait, and pings as the session socket does", () => {
    vi.useFakeTimers();
    const sockets = fakeSockets();
    const connection = createLiveConnection({
      sessionToken: "session",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    vi.advanceTimersByTime(pingEveryMs);
    expect(sockets.sent.at(-1)).toMatchObject({ type: "ping" });
    sockets.opened[0]!.onClose(1006);
    expect(connection.snapshot().isConnected).toBe(false);
    vi.advanceTimersByTime(reconnectWaitMs(0));
    expect(sockets.opened).toHaveLength(2);
  });

  it("connects no more once the server refuses the sign-in", () => {
    vi.useFakeTimers();
    const sockets = fakeSockets();
    const connection = createLiveConnection({
      sessionToken: "lapsed",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onClose(1008);
    vi.advanceTimersByTime(60_000);
    expect(sockets.opened).toHaveLength(1);
    expect(connection.snapshot().isRefused).toBe(true);
  });

  it("connects no more once the server says the sign-in ended, the device removed from another", () => {
    vi.useFakeTimers();
    const sockets = fakeSockets();
    const connection = createLiveConnection({
      sessionToken: "removed",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "devices", deviceIds: [3] }));
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "signedOut" }));
    expect(connection.snapshot()).toMatchObject({ isConnected: false, isRefused: true });
    sockets.opened[0]!.onClose(1006);
    vi.advanceTimersByTime(60_000);
    expect(sockets.opened).toHaveLength(1);
  });

  it("keeps a cast handed to it until it is taken", () => {
    const sockets = fakeSockets();
    const connection = createLiveConnection({
      sessionToken: "session",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(
      JSON.stringify({
        type: "receive",
        receive: {
          castId: 7,
          membershipCredential: "cast",
          fileId: 3,
          titleName: "Metropolis",
          positionMs: 61_000,
          fromDeviceId: 2,
          fromDeviceName: "Pixel",
        },
      }),
    );
    expect(connection.snapshot().receive).toEqual({
      castId: 7,
      membershipCredential: "cast",
      fileId: 3,
      titleName: "Metropolis",
      positionMs: 61_000,
      fromDeviceId: 2,
      fromDeviceName: "Pixel",
    });
    connection.takeReceive();
    expect(connection.snapshot().receive).toBeUndefined();
  });

  it("knows nothing was handed on connecting once the ping after the devices is answered (D383 §2)", () => {
    const sockets = fakeSockets();
    const connection = createLiveConnection({
      sessionToken: "session",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    expect(sockets.sent).toHaveLength(1); // the hello alone, before the server has spoken
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "devices", deviceIds: [3] }));
    expect(connection.snapshot().isReceiveKnown).toBe(false);
    expect(sockets.sent.at(-1)).toEqual({ type: "ping", tMono: 1000 });
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "devices", deviceIds: [3, 4] }));
    expect(sockets.sent).toHaveLength(2); // asked once
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "pong", tMono: 999 }));
    expect(connection.snapshot().isReceiveKnown).toBe(false);
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "pong", tMono: 1000 }));
    expect(connection.snapshot()).toMatchObject({ isReceiveKnown: true, receive: undefined });
  });

  it("knows a cast was handed on connecting as it is handed", () => {
    const sockets = fakeSockets();
    const connection = createLiveConnection({
      sessionToken: "session",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "devices", deviceIds: [3] }));
    sockets.opened[0]!.onMessage(
      JSON.stringify({
        type: "receive",
        receive: { castId: 7, membershipCredential: "cast", fileId: 3, fromDeviceId: 2 },
      }),
    );
    expect(connection.snapshot()).toMatchObject({ isReceiveKnown: true, receive: { castId: 7 } });
  });

  it("says whether its tab is in front, in the hello and as it moves, and keeps the devices in front (D338 §2)", () => {
    const sockets = fakeSockets();
    let isInFront = false;
    const connection = createLiveConnection({
      sessionToken: "session",
      openSocket: sockets.open,
      monotonicMs,
      isInFront: () => isInFront,
    });
    connection.open();
    connection.showFront(true); // before the hello, which says where the tab is then
    expect(sockets.sent).toEqual([]);
    sockets.opened[0]!.onOpen();
    expect(sockets.sent[0]).toEqual({ type: "live", sessionToken: "session", behind: true });
    isInFront = true;
    connection.showFront(true);
    connection.showFront(false);
    expect(sockets.sent.slice(1)).toEqual([{ type: "front" }, { type: "front", behind: true }]);
    sockets.opened[0]!.onMessage(
      JSON.stringify({ type: "devices", deviceIds: [3, 4], inFrontDeviceIds: [4] }),
    );
    expect(connection.snapshot()).toMatchObject({ deviceIds: [3, 4], inFrontDeviceIds: [4] });
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "devices", deviceIds: [3] }));
    expect(connection.snapshot().inFrontDeviceIds).toEqual([]);
  });

  it("keeps the devices receiving a cast as it is told, which no device may cast to (D386 §1)", () => {
    const sockets = fakeSockets();
    const connection = createLiveConnection({
      sessionToken: "session",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(
      JSON.stringify({ type: "devices", deviceIds: [3, 4], receivingDeviceIds: [4] }),
    );
    expect(connection.snapshot().receivingDeviceIds).toEqual([4]);
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "devices", deviceIds: [3, 4] }));
    expect(connection.snapshot().receivingDeviceIds).toEqual([]);
  });

  it("keeps the cast this device sends as it is told, and that it sends none (D364 §6)", () => {
    const sockets = fakeSockets();
    const connection = createLiveConnection({
      sessionToken: "session",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    expect(connection.snapshot()).toMatchObject({ sending: undefined, isSendingKnown: false });
    sockets.opened[0]!.onMessage(
      JSON.stringify({
        type: "sending",
        sending: {
          castId: 7,
          receiverDeviceId: 2,
          receiverName: "Fire TV",
          fileId: 3,
          titleName: "Metropolis",
          durationMs: 6_000_000,
          isAccepted: true,
        },
      }),
    );
    expect(connection.snapshot()).toMatchObject({
      isSendingKnown: true,
      sending: {
        castId: 7,
        receiverDeviceId: 2,
        receiverName: "Fire TV",
        fileId: 3,
        titleName: "Metropolis",
        durationMs: 6_000_000,
        isAccepted: true,
      },
    });
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "sending" }));
    expect(connection.snapshot()).toMatchObject({ sending: undefined, isSendingKnown: true });
  });

  it("reads a receiver's credential for the session its person hosts (D364 §11)", () => {
    expect(
      parseLiveMessage(
        JSON.stringify({
          type: "receive",
          receive: {
            castId: 1,
            membershipCredential: "c",
            fileId: 0,
            fromDeviceId: 3,
            togetherCredential: "hosted",
          },
        }),
      ),
    ).toMatchObject({
      receive: { fileId: 0, togetherCredential: "hosted" },
    });
  });

  it("reads a receiver's credential for the session its person is a guest in, which it follows (D371 §4)", () => {
    expect(
      parseLiveMessage(
        JSON.stringify({
          type: "receive",
          receive: {
            castId: 1,
            membershipCredential: "c",
            fileId: 0,
            fromDeviceId: 3,
            togetherCredential: "guest",
            isTogetherFollower: true,
          },
        }),
      ),
    ).toMatchObject({ receive: { togetherCredential: "guest", isTogetherFollower: true } });
  });

  it("reads a cast with no position as one starting where the person left the film", () => {
    expect(
      parseLiveMessage(
        JSON.stringify({
          type: "receive",
          receive: { castId: 1, membershipCredential: "c", fileId: 2, fromDeviceId: 3 },
        }),
      ),
    ).toEqual({
      type: "receive",
      receive: {
        castId: 1,
        membershipCredential: "c",
        fileId: 2,
        titleName: "",
        positionMs: undefined,
        fromDeviceId: 3,
        fromDeviceName: "",
      },
    });
    expect(parseLiveMessage(JSON.stringify({ type: "receive", receive: { castId: 1 } }))).toBe(
      undefined,
    );
  });
});
