// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  ChannelClosedError,
  sendOnChannel,
  type ChannelLike,
  type ChannelListener,
} from "./rtc-request.ts";

// Over WebRTC every JSON answer of the core came whole, a library's list three to ten times its
// gzipped size, on the links that can least carry it (the slow-link war game, 2026-11-14; D483).

/** What the test reads of node:zlib, to gzip an answer as the core does. */
interface Gzipping {
  readonly gzipSync: (data: Uint8Array) => Uint8Array;
}

const zlib = await vi.importActual<Gzipping>("node:zlib");

/** data gzipped, in an array of its own: a Buffer node gives may be a view on a shared pool. */
const gzipped = (text: string): Uint8Array =>
  new Uint8Array(zlib.gzipSync(new TextEncoder().encode(text)));

/** Every channel a test opened, so that the requests it left open are ended after it. */
const channelsOpened: AnsweringChannel[] = [];

/** An open data channel the test plays the core on: it keeps what the client sends and answers by hand. */
class AnsweringChannel implements ChannelLike {
  readyState: RTCDataChannelState = "open";
  readonly sent: (string | ArrayBuffer)[] = [];
  isClosed = false;
  private listener: ChannelListener | undefined;

  constructor() {
    channelsOpened.push(this);
  }

  send(data: string | ArrayBuffer): void {
    this.sent.push(data);
  }

  close(): void {
    this.isClosed = true;
  }

  listen(listener: ChannelListener): void {
    this.listener = listener;
  }

  answer(data: string | ArrayBuffer): void {
    this.listener?.message(data);
  }

  closeFromTheCore(): void {
    this.listener?.close();
  }

  /** The encodings the client's head asked for, if it asked for any. */
  askedEncoding(): unknown {
    const [head] = this.sent;
    const parsed: unknown = typeof head === "string" ? JSON.parse(head) : undefined;
    return (parsed as { headers: Record<string, unknown> }).headers["accept-encoding"];
  }
}

// A request a test leaves open keeps its real silence watch: each is ended once its test is.
afterEach(() => {
  for (const channel of channelsOpened.splice(0)) {
    if (!channel.isClosed) {
      channel.answer(JSON.stringify({ status: 204 }));
      channel.answer(new ArrayBuffer(0));
    }
  }
});

/** Lets the client's pending work run. */
const settle = (): Promise<void> => new Promise((resolve) => setTimeout(resolve, 0));

describe("a gzipped answer over WebRTC (D483)", () => {
  it("is asked for on a read, as the browser asks over HTTP", async () => {
    const read = new AnsweringChannel();
    void sendOnChannel(read, new Request("https://app.example/api/v1/library"));
    const ranged = new AnsweringChannel();
    void sendOnChannel(
      ranged,
      new Request("https://app.example/client/app.js", { headers: { range: "bytes=0-99" } }),
    );
    const named = new AnsweringChannel();
    void sendOnChannel(
      named,
      new Request("https://app.example/api/v1/x", { headers: { "accept-encoding": "identity" } }),
    );
    const posted = new AnsweringChannel();
    void sendOnChannel(
      posted,
      new Request("https://app.example/api/v1/session", { method: "POST", body: "{}" }),
    );
    await settle();
    expect(read.askedEncoding()).toEqual(["gzip"]);
    // Not for a byte range, which the core would serve of the gzipped bytes; not where the request
    // names its own; not for a post, as the core gzips the answers of reads alone.
    expect(ranged.askedEncoding()).toBeUndefined();
    expect(named.askedEncoding()).toEqual(["identity"]);
    expect(posted.askedEncoding()).toBeUndefined();
  });

  it("is handed on inflated, with no Content-Encoding left to inflate it again", async () => {
    const channel = new AnsweringChannel();
    const answered = sendOnChannel(channel, new Request("https://app.example/api/v1/library"));
    await settle();
    const json = JSON.stringify({ items: Array.from({ length: 200 }, (_, id) => ({ id })) });
    const body = gzipped(json);
    channel.answer(
      JSON.stringify({
        status: 200,
        headers: {
          "Content-Type": ["application/json"],
          "Content-Encoding": ["gzip"],
          "Content-Length": [String(body.byteLength)],
          vary: ["Accept-Encoding"],
        },
      }),
    );
    const response = await answered;
    const half = Math.floor(body.byteLength / 2);
    channel.answer(body.slice(0, half).buffer);
    channel.answer(body.slice(half).buffer);
    channel.answer(new ArrayBuffer(0));
    expect(response.headers.get("content-encoding")).toBeNull();
    expect(response.headers.get("content-length")).toBeNull();
    expect(response.headers.get("content-type")).toBe("application/json");
    expect(await response.text()).toBe(json);
    expect(channel.isClosed).toBe(true);
  });

  it("whose channel closes midway fails reading with why, as an answer sent whole does", async () => {
    const channel = new AnsweringChannel();
    const answered = sendOnChannel(channel, new Request("https://app.example/api/v1/library"));
    await settle();
    const body = gzipped(JSON.stringify({ items: [1, 2, 3] }));
    channel.answer(JSON.stringify({ status: 200, headers: { "Content-Encoding": ["gzip"] } }));
    const response = await answered;
    channel.answer(body.slice(0, 5).buffer);
    channel.closeFromTheCore();
    await expect(response.json()).rejects.toBeInstanceOf(ChannelClosedError);
  });
});
