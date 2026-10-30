// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { silentAtMostMs, type ChannelLike, type ChannelListener } from "./rtc-request.ts";
import { RtcTransport, settledAfterMs, type PeerLike } from "./rtc-transport.ts";

/** A data channel the test plays the core on: it keeps what the client sends and answers by hand. */
class FakeChannel implements ChannelLike {
  readyState: RTCDataChannelState = "connecting";
  readonly sent: (string | ArrayBuffer)[] = [];
  isClosed = false;
  private listener: ChannelListener | undefined;

  send(data: string | ArrayBuffer): void {
    this.sent.push(data);
  }

  close(): void {
    this.isClosed = true;
  }

  listen(listener: ChannelListener): void {
    this.listener = listener;
  }

  open(): void {
    this.readyState = "open";
    this.listener?.open();
  }

  answer(data: string | ArrayBuffer): void {
    this.listener?.message(data);
  }

  closeFromTheCore(): void {
    this.listener?.close();
  }

  /** The head the client sent, read back. */
  sentHead(): unknown {
    const [head] = this.sent;
    return typeof head === "string" ? JSON.parse(head) : undefined;
  }
}

/** Lets the client's pending work run. */
const settle = (): Promise<void> => new Promise((resolve) => setTimeout(resolve, 0));

/** A connection the test keeps: the channels opened on it, and its word that it is gone. */
class FakePeer implements PeerLike {
  readonly channels: FakeChannel[] = [];
  isClosed = false;
  private goneListener: ((why: string) => void) | undefined;

  createDataChannel(): ChannelLike {
    const channel = new FakeChannel();
    channel.readyState = "open";
    this.channels.push(channel);
    return channel;
  }

  close(): void {
    this.isClosed = true;
  }

  onGone(listener: (why: string) => void): void {
    this.goneListener = listener;
  }

  /** The connection failed, or stayed disconnected past the grace. */
  goes(why = "its connection failed"): void {
    this.goneListener?.(why);
  }

  /** The index-th channel opened on it, which must have been. */
  channel(index: number): FakeChannel {
    const channel = this.channels[index];
    if (channel === undefined) {
      throw new Error(`no channel ${String(index)} was opened`);
    }
    return channel;
  }
}

/** The reader of a response's body, which must have one. */
function readerOf(response: Response): ReadableStreamDefaultReader<Uint8Array> {
  if (response.body === null) {
    throw new Error("the response has no body");
  }
  return response.body.getReader();
}

describe("the connection a request is carried on, for a stuck chunk's log (the owner, 2026-10-29)", () => {
  it("names it, how long it has been up, what it carries and how long since bytes came over it, while the request is under way", async () => {
    const clock = { nowMs: 1000 };
    const peer = new FakePeer();
    const transport = new RtcTransport([peer], undefined, () => clock.nowMs);
    const url = "https://app.example/api/v1/bytes/media/chunks/t/picture/00042.m4s";
    const answered = transport.fetch(new Request(url));
    void transport.fetch(new Request("https://app.example/api/v1/poster")).catch(() => undefined);
    await settle();
    clock.nowMs = 3000;
    const carrier = transport.carrierOf(url);
    expect(carrier?.connection).toBeGreaterThan(0);
    expect(carrier).toMatchObject({ upMs: 2000, underWay: 2, sinceDeliveredMs: undefined });
    const channel = peer.channels[0];
    channel?.answer(JSON.stringify({ status: 200 }));
    await answered;
    clock.nowMs = 3500;
    channel?.answer(new Uint8Array([1, 2, 3]).buffer);
    clock.nowMs = 9000;
    expect(transport.carrierOf(url)?.sinceDeliveredMs).toBe(5500); // none since, over any request
    expect(transport.carrierOf("https://app.example/other")).toBeUndefined();
    channel?.answer(new ArrayBuffer(0));
    channel?.closeFromTheCore();
    expect(transport.carrierOf(url)).toBeUndefined(); // no longer under way
  });
});

// The owner's Chrome, throttled until its film stalled and unthrottled: a chunk's body stopped after
// 19 bytes, and the player never loaded again (staging, 2026-10-29). Nothing over WebRTC waits for
// good: a request that hears nothing while it waits on the core fails, and a connection that carried
// nothing for as long is replaced, the requests after it going on the new one.
describe("a request that hears nothing for silentAtMostMs", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  /** A transport on first, growing from opened, its clock the fake timers'. */
  function growing(first: FakePeer): { transport: RtcTransport; opened: FakePeer[] } {
    const opened: FakePeer[] = [];
    const transport = new RtcTransport(
      [first],
      {
        size: 4,
        open: () => {
          const peer = new FakePeer();
          opened.push(peer);
          return Promise.resolve(peer);
        },
      },
      () => Date.now(),
    );
    return { transport, opened };
  }

  it("fails waiting for its head, its channel closed, and its silent connection is replaced", async () => {
    vi.useFakeTimers();
    const first = new FakePeer();
    const { transport, opened } = growing(first);
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    const url = "https://app.example/api/v1/chunk/7";
    const outcome = transport.fetch(new Request(url)).then(
      () => "answered",
      (problem: unknown) => (problem instanceof Error ? problem.name : "failed"),
    );
    await vi.advanceTimersByTimeAsync(silentAtMostMs - 1);
    expect(first.isClosed).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(await outcome).toBe("SilentRequestError");
    expect(first.channels[0]?.isClosed).toBe(true);
    expect(first.isClosed).toBe(true);
    expect(opened).toHaveLength(1);
    expect(transport.carrierOf(url)?.ended).toBe(
      `nothing came for ${String(silentAtMostMs)} ms, nor over its connection, which was replaced`,
    );
    // Asked again, it goes on the new connection, which answers.
    const again = transport.fetch(new Request(url));
    await vi.advanceTimersByTimeAsync(0);
    expect(transport.carrierOf(url)?.ended).toBeUndefined();
    opened[0]?.channels[0]?.answer(JSON.stringify({ status: 204 }));
    opened[0]?.channels[0]?.answer(new ArrayBuffer(0));
    expect((await again).status).toBe(204);
  });

  it("fails its body when its reader waits that long after some of it came, a stuck chunk", async () => {
    vi.useFakeTimers();
    const first = new FakePeer();
    const { transport } = growing(first);
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    const answered = transport.fetch(new Request("https://app.example/api/v1/chunk/8"));
    await vi.advanceTimersByTimeAsync(0);
    const channel = first.channel(0);
    channel.answer(JSON.stringify({ status: 200 }));
    const reader = readerOf(await answered);
    channel.answer(new Uint8Array(19).buffer);
    expect((await reader.read()).value).toHaveLength(19);
    const next = reader.read().then(
      () => "read",
      (problem: unknown) => (problem instanceof Error ? problem.name : "failed"),
    );
    // Another request's bytes come over the same connection meanwhile: the connection lives.
    const other = transport.fetch(new Request("https://app.example/api/v1/poster"));
    await vi.advanceTimersByTimeAsync(silentAtMostMs / 2);
    first.channel(1).answer(JSON.stringify({ status: 200 }));
    await other;
    first.channel(1).answer(new Uint8Array([1]).buffer);
    await vi.advanceTimersByTimeAsync(silentAtMostMs / 2);
    expect(await next).toBe("SilentRequestError");
    expect(channel.isClosed).toBe(true);
    expect(first.isClosed).toBe(false);
    expect(transport.carrierOf("https://app.example/api/v1/chunk/8")?.ended).toBe(
      `nothing came for ${String(silentAtMostMs)} ms`,
    );
  });

  it("waits unbounded while its reader does not read, the core having no room to send", async () => {
    vi.useFakeTimers();
    const first = new FakePeer();
    const { transport } = growing(first);
    const answered = transport.fetch(new Request("https://app.example/api/v1/film"));
    await vi.advanceTimersByTimeAsync(0);
    first.channels[0]?.answer(JSON.stringify({ status: 206 }));
    const response = await answered;
    await vi.advanceTimersByTimeAsync(silentAtMostMs * 3);
    expect(first.channels[0]?.isClosed).toBe(false);
    first.channels[0]?.answer(new Uint8Array([5]).buffer);
    first.channels[0]?.answer(new ArrayBuffer(0));
    expect(new Uint8Array(await response.arrayBuffer())).toEqual(new Uint8Array([5]));
  });

  // A playback's answer whose head came and whose body never did said nothing of which channel its
  // bytes stopped on, for the core's log of it to be found (staging, 2026-10-30).
  it("names the SCTP stream of the channel it heard nothing on", async () => {
    vi.useFakeTimers();
    const first = new FakePeer();
    const opening = first.createDataChannel.bind(first);
    first.createDataChannel = () => Object.assign(opening(), { id: 7 });
    const { transport } = growing(first);
    const url = "https://app.example/api/v1/playback";
    const outcome = transport.fetch(new Request(url)).catch(() => undefined);
    await vi.advanceTimersByTimeAsync(silentAtMostMs);
    await outcome;
    expect(transport.carrierOf(url)?.ended).toBe(
      `nothing came for ${String(silentAtMostMs)} ms on stream 7, nor over its connection, which was replaced`,
    );
  });

  it("replaces a connection that opens no channel, and gives its room back", async () => {
    vi.useFakeTimers();
    const first = new FakePeer();
    const { transport, opened } = growing(first);
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    first.createDataChannel = () => {
      throw new Error("the SCTP transport is closed");
    };
    await expect(transport.fetch(new Request("https://app.example/a"))).rejects.toThrow(
      "the SCTP transport is closed",
    );
    expect(first.isClosed).toBe(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(opened).toHaveLength(1);
    expect(transport.size).toBe(1);
  });
});
