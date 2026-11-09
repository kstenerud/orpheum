// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { createTogetherConnection } from "./together-connection.ts";
import type { TogetherSocketListeners } from "./together.ts";

/** A stand-in for the server's end of each socket: what was opened, and every message sent, parsed. */
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

/** A welcome of member 7's device 9, its session paused. */
const welcome = {
  v: 3,
  type: "welcome",
  seq: 4,
  sessionId: 1,
  memberId: 7,
  deviceId: 9,
  group: { state: "paused", positionMs: 1000, rate: 1, fileId: 3 },
  roster: [{ id: 7, name: "Gran", devices: [{ id: 9, role: "video" }] }],
};

describe("what a device's state tells its session of its player (cut-a §5.1)", () => {
  it("tells the group, in its state, that even its lowest quality cannot keep up, and when it can again (D247 §8)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    connection.setPlayer(() => ({ fileId: 7, positionMs: 0, isPlaying: false, isBuffering: true }));
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

  it("tells the group, in its state, that its page is hidden, its film paused, and says nothing once shown (D372 §5)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    let isPageHidden = true;
    connection.setPlayer(() => ({
      fileId: 7,
      positionMs: 0,
      isPlaying: false,
      isBuffering: false,
      ...(isPageHidden ? { isPageHidden } : {}),
    }));
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    const lastState = (): Record<string, unknown> | undefined =>
      sockets.sent.filter((each) => each["type"] === "state").at(-1);
    expect(lastState()?.["isPageHidden"]).toBe(true);
    isPageHidden = false;
    connection.reportState();
    expect(lastState()).not.toHaveProperty("isPageHidden");
  });

  it("tells the group, in its first state at its welcome, its player's position and that it is its own (D377 §3, §4)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    let isPlaceRemembered = false;
    connection.setPlayer(() => ({
      fileId: 3,
      positionMs: 75_000.4,
      isPlaying: true,
      isBuffering: false,
      isPlaceRemembered,
    }));
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    const states = (): Record<string, unknown>[] =>
      sockets.sent.filter((each) => each["type"] === "state");
    expect(states().at(-1)).not.toHaveProperty("remembersPlace"); // a page loaded afresh
    isPlaceRemembered = true;
    sockets.opened[0]!.onClose(1006); // dropped; connected again after its wait
    sockets.sent.length = 0;
    connection.open();
    sockets.opened[1]!.onOpen();
    sockets.opened[1]!.onMessage(JSON.stringify(welcome));
    expect(states()[0]).toMatchObject({ positionMs: 75_000, remembersPlace: true });
  });
});
