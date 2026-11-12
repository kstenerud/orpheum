// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { rtcTogetherSocket } from "./rtc-together.ts";
import type { ChannelLike, ChannelListener } from "./rtc-request.ts";

/** A channel the test plays the core on. */
class FakeChannel implements ChannelLike {
  readyState: RTCDataChannelState = "connecting";
  readonly sent: (string | ArrayBuffer)[] = [];
  isClosed = false;
  listener: ChannelListener | undefined;

  send(data: string | ArrayBuffer): void {
    this.sent.push(data);
  }

  close(): void {
    this.isClosed = true;
  }

  listen(listener: ChannelListener): void {
    this.listener = listener;
  }
}

/** What the socket told its listeners. */
interface Heard {
  opened: number;
  messages: string[];
  closes: number[];
}

function open(channel: FakeChannel): {
  heard: Heard;
  socket: ReturnType<ReturnType<typeof rtcTogetherSocket>>;
} {
  const heard: Heard = { opened: 0, messages: [], closes: [] };
  const socket = rtcTogetherSocket(() => channel)({
    onOpen: () => {
      heard.opened += 1;
    },
    onMessage: (text) => {
      heard.messages.push(text);
    },
    onClose: (code) => {
      heard.closes.push(code);
    },
  });
  return { heard, socket };
}

describe("the session socket over a data channel", () => {
  it("closes as one that dropped when no connection to the core is up, so its connection tries again", async () => {
    const closes: number[] = [];
    const socket = rtcTogetherSocket(() => {
      throw new Error("no WebRTC connection to the core is up");
    })({
      onOpen: () => undefined,
      onMessage: () => undefined,
      onClose: (code) => {
        closes.push(code);
      },
    });
    expect(closes).toEqual([]); // not before its caller holds the socket
    await Promise.resolve();
    expect(closes).toEqual([1006]);
    socket.send("nothing goes");
    socket.close();
    expect(closes).toEqual([1006]);
  });

  it("asks to become a WebSocket, opens on 101, and carries text messages both ways", () => {
    const channel = new FakeChannel();
    const { heard, socket } = open(channel);
    channel.listener?.open();
    const [head] = channel.sent;
    expect(typeof head === "string" ? JSON.parse(head) : head).toEqual({
      method: "GET",
      path: "/api/v1/socket",
      // Declaring no body, it is whole at its head where its empty end never leaves the browser, as
      // Chrome before 92's (rtcapi's EndHeader).
      headers: { upgrade: ["websocket"], "content-length": ["0"] },
    });
    socket.send("too early");
    channel.listener?.message(JSON.stringify({ status: 101 }));
    channel.listener?.message('{"type":"welcome"}');
    socket.send('{"type":"hello"}');
    expect(heard).toEqual({ opened: 1, messages: ['{"type":"welcome"}'], closes: [] });
    expect(channel.sent.slice(2)).toEqual(['{"type":"hello"}']);
  });

  it("closes with the core's code, or as abnormal when the channel just ends", () => {
    const refused = new FakeChannel();
    const { heard } = open(refused);
    refused.listener?.message(JSON.stringify({ status: 101 }));
    refused.listener?.message(new TextEncoder().encode(JSON.stringify({ close: 1008 })).buffer);
    refused.listener?.close();
    expect(heard.closes).toEqual([1008]);
    expect(refused.isClosed).toBe(true);

    const cut = new FakeChannel();
    const lost = open(cut);
    cut.listener?.close();
    expect(lost.heard.closes).toEqual([1006]);
  });
});
