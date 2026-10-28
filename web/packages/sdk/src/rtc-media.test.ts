// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  answerLoad,
  askPathOf,
  isPathMessage,
  isPathQueryMessage,
  responseFromPort,
  relayedAheadBytes,
  type FetchMessage,
  type PortLike,
} from "./rtc-media.ts";

/** A data channel's message: the size of a body's piece over the WebRTC path. */
const piece = 64 * 1024;

const aLoad: FetchMessage = {
  kind: "orpheum-fetch",
  url: "https://app.example/api/v1/chunks/x",
  method: "GET",
  headers: [],
};

/** Lets the ports' messages and the streams' reads run their course. */
async function settle(): Promise<void> {
  for (let turn = 0; turn < 50; turn += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

/**
 * A body of count pieces, each filled with its index, made only as read: how much was read, whether
 * it was cancelled, and the pieces it made.
 */
function piecesOf(count: number): {
  stream: ReadableStream<Uint8Array>;
  read: () => number;
  cancelled: () => boolean;
  made: Uint8Array[];
} {
  const made: Uint8Array[] = [];
  let isCancelled = false;
  const stream = new ReadableStream<Uint8Array>(
    {
      pull: (controller) => {
        if (made.length === count) {
          controller.close();
          return;
        }
        const next = new Uint8Array(piece).fill(made.length % 256);
        made.push(next);
        controller.enqueue(next);
      },
      cancel: () => {
        isCancelled = true;
      },
    },
    { highWaterMark: 0 },
  );
  return {
    stream,
    read: () => made.length * piece,
    cancelled: () => isCancelled,
    made,
  };
}

/** The page's end of a port, counting each message the worker sends it. */
function countingIncoming(port: MessagePort, counted: () => void): PortLike {
  return {
    postMessage: (message, transfer) => {
      port.postMessage(message, transfer ?? []);
    },
    addEventListener: (type, listener) => {
      port.addEventListener(type, (event) => {
        counted();
        listener(event);
      });
    },
    start: () => {
      port.start();
    },
  };
}

describe("media over the WebRTC path", () => {
  it("carries a load from the worker to the page and its body back", async () => {
    const { port1, port2 } = new MessageChannel();
    const heard: Request[] = [];
    const pieces = ["orph", "eum"].map((text) => new TextEncoder().encode(text));
    const load: FetchMessage = {
      kind: "orpheum-fetch",
      url: "https://app.example/api/v1/bytes/media/x",
      method: "GET",
      headers: [["Range", "bytes=0-"]],
    };
    void answerLoad(load, port2, {
      send: (request) => {
        heard.push(request);
        return Promise.resolve(
          new Response(new Blob(pieces).stream(), {
            status: 206,
            headers: { "Content-Range": "bytes 0-6/7" },
          }),
        );
      },
      token: "t",
      viewer: "v",
    });
    const response = await responseFromPort(port1);
    expect(response.status).toBe(206);
    expect(response.headers.get("content-range")).toBe("bytes 0-6/7");
    expect(await response.text()).toBe("orpheum");
    expect(heard[0]?.headers.get("Range")).toBe("bytes=0-");
    expect(heard[0]?.headers.get("Authorization")).toBe("Bearer t");
    // A viewer's membership beside the page's own sign-in, which no cookie reaches across origins.
    expect(heard[0]?.headers.get("Orpheum-Viewer")).toBe("v");
    port1.close();
  });

  it("sends a body ahead of the worker's reader, not a piece for each message the busy page answers", async () => {
    const { port1, port2 } = new MessageChannel();
    const source = piecesOf(128);
    let toThePage = 0;
    void answerLoad(
      aLoad,
      countingIncoming(port2, () => (toThePage += 1)),
      {
        send: () => Promise.resolve(new Response(source.stream)),
        token: undefined,
        viewer: undefined,
      },
    );
    const body = new Uint8Array(await (await responseFromPort(port1)).arrayBuffer());
    // In order, every piece.
    expect(body.byteLength).toBe(128 * piece);
    for (let index = 0; index < 128; index += 1) {
      expect(body[index * piece]).toBe(index % 256);
    }
    // 8 MiB took one message for each 64 KiB piece, a round trip through the page each, which at a
    // Fire TV's 30 ms carried 17 Mb/s (staging, 2026-10-28); granted a share of the window at a
    // time, it takes one for each 256 KiB.
    expect(toThePage).toBeLessThanOrEqual((128 * piece) / (relayedAheadBytes / 4) + 1);
    port1.close();
  });

  it("sends no more than its window ahead of a reader that takes nothing", async () => {
    const { port1, port2 } = new MessageChannel();
    const source = piecesOf(1000);
    void answerLoad(aLoad, port2, {
      send: () => Promise.resolve(new Response(source.stream)),
      token: undefined,
      viewer: undefined,
    });
    await responseFromPort(port1);
    await settle();
    expect(source.read()).toBeGreaterThanOrEqual(relayedAheadBytes);
    expect(source.read()).toBeLessThanOrEqual(relayedAheadBytes + 2 * piece);
    port1.close();
  });

  it("moves each piece's own buffer to the worker rather than copying it", async () => {
    const { port1, port2 } = new MessageChannel();
    const source = piecesOf(4);
    void answerLoad(aLoad, port2, {
      send: () => Promise.resolve(new Response(source.stream)),
      token: undefined,
      viewer: undefined,
    });
    expect((await (await responseFromPort(port1)).arrayBuffer()).byteLength).toBe(4 * piece);
    // A moved buffer is gone from the page.
    expect(source.made.map((made) => made.byteLength)).toEqual([0, 0, 0, 0]);
    port1.close();
  });

  it("stops the page's load when the worker's reader gives the body up, as a seek does", async () => {
    const { port1, port2 } = new MessageChannel();
    const source = piecesOf(1000);
    void answerLoad(aLoad, port2, {
      send: () => Promise.resolve(new Response(source.stream)),
      token: undefined,
      viewer: undefined,
    });
    const reader = (await responseFromPort(port1)).body?.getReader();
    expect((await reader?.read())?.value?.[0]).toBe(0);
    await reader?.cancel();
    await settle();
    expect(source.cancelled()).toBe(true);
    port1.close();
  });

  it("fails the body when the page's load fails partway", async () => {
    const { port1, port2 } = new MessageChannel();
    let sent = 0;
    const failing = new ReadableStream<Uint8Array>({
      pull: (controller) => {
        sent += 1;
        if (sent > 2) {
          controller.error(new Error("the channel closed"));
          return;
        }
        controller.enqueue(new Uint8Array(piece));
      },
    });
    void answerLoad(aLoad, port2, {
      send: () => Promise.resolve(new Response(failing)),
      token: undefined,
      viewer: undefined,
    });
    const response = await responseFromPort(port1);
    await expect(response.arrayBuffer()).rejects.toThrow("the channel closed");
    port1.close();
  });

  it("fails a load the page could not make", async () => {
    const { port1, port2 } = new MessageChannel();
    void answerLoad(
      { kind: "orpheum-fetch", url: "https://app.example/api/v1/x", method: "GET", headers: [] },
      port2,
      {
        send: () => Promise.reject(new Error("the channel closed")),
        token: undefined,
        viewer: undefined,
      },
    );
    await expect(responseFromPort(port1)).rejects.toThrow("the channel closed");
    port1.close();
  });

  it("knows a page's word on its path from anything else", () => {
    expect(isPathMessage({ kind: "orpheum-path", path: "webrtc" })).toBe(true);
    expect(isPathMessage({ kind: "orpheum-path", path: "pigeon" })).toBe(false);
    expect(isPathMessage("webrtc")).toBe(false);
  });

  it("asks a page its path, as a worker started again must: a page carrying its media says so, any other says nothing", async () => {
    const carrying = {
      postMessage: (message: unknown, transfer: Transferable[]) => {
        const [port] = transfer;
        if (isPathQueryMessage(message) && port instanceof MessagePort) {
          port.postMessage({ kind: "orpheum-path", path: "webrtc" });
        }
      },
    };
    expect(await askPathOf(carrying)).toBe("webrtc");
    const silent = { postMessage: () => undefined };
    expect(await askPathOf(silent, 20)).toBe("http");
  });
});
