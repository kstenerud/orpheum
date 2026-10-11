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
  v: 2,
  type: "welcome",
  seq: 4,
  sessionId: 1,
  memberId: 7,
  deviceId: 9,
  group: { state: "Paused", positionMs: 1000, rate: 1, fileId: 3 },
  roster: [{ id: 7, name: "Gran", devices: [{ id: 9, role: "playback" }] }],
};

describe("a device's hello and welcome (cut-a §5.1)", () => {
  it("names the client its page runs in the hello, for the server's log (agent-failures A28)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "membership",
      client: "index-T3st.js",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    expect(sockets.sent[0]).toMatchObject({ type: "hello", client: "index-T3st.js" });
  });

  it("keeps the rights its welcome gives its member, and none where it says none (D271 §4)", () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "membership",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    expect(connection.snapshot().rights).toEqual({ canPlayPause: false, canSeek: false });
    sockets.opened[0]!.onMessage(
      JSON.stringify({ ...welcome, rights: { canPlayPause: true, canSeek: false } }),
    );
    expect(connection.snapshot().rights).toEqual({ canPlayPause: true, canSeek: false });
  });
});
