// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  grantEvery,
  maxMessageBytes,
  replaceFirstMs,
  requestsPerConnection,
  RtcTransport,
  sendOnChannel,
  SwitchableFetch,
  type ChannelLike,
  type ChannelListener,
  type PeerLike,
} from "./rtc-transport.ts";

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

describe("the WebRTC transport", () => {
  it("sends a head, the body in messages a browser carries, and an empty message", async () => {
    const channel = new FakeChannel();
    const body = new Uint8Array(maxMessageBytes + 10).fill(7);
    void sendOnChannel(
      channel,
      new Request("https://app.example/api/v1/things?x=1", {
        method: "POST",
        body,
        headers: [["Authorization", "Bearer t"]],
      }),
    );
    await settle();
    channel.open();
    expect(channel.sentHead()).toEqual({
      method: "POST",
      path: "/api/v1/things?x=1",
      headers: { authorization: ["Bearer t"] },
    });
    const sizes = channel.sent.slice(1).map((message) => (message as ArrayBuffer).byteLength);
    expect(sizes).toEqual([maxMessageBytes, 10, 0]);
  });

  it("streams the response's body as its messages arrive, and closes the channel at its end", async () => {
    const channel = new FakeChannel();
    channel.readyState = "open";
    const answered = sendOnChannel(channel, new Request("https://app.example/api/v1/big"));
    await settle();
    channel.answer(JSON.stringify({ status: 200, headers: { "Content-Type": ["text/plain"] } }));
    const response = await answered;
    channel.answer(new TextEncoder().encode("orph").buffer);
    channel.answer(new TextEncoder().encode("eum").buffer);
    channel.answer(new ArrayBuffer(0));
    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toBe("text/plain");
    expect(await response.text()).toBe("orpheum");
    expect(channel.isClosed).toBe(true);
  });

  it("grants the core more of the body only as its reader reads (rtcapi.Credit)", async () => {
    const channel = new FakeChannel();
    channel.readyState = "open";
    const answered = sendOnChannel(channel, new Request("https://app.example/api/v1/film"));
    await settle();
    channel.answer(JSON.stringify({ status: 206 }));
    const reader = (await answered).body?.getReader();
    for (let index = 0; index < grantEvery * 2; index += 1) {
      channel.answer(new Uint8Array([index]).buffer);
    }
    const grants = (): unknown[] =>
      channel.sent.filter((message) => typeof message === "string" && message.includes("credit"));
    expect(grants()).toEqual([]);
    for (let index = 0; index < grantEvery; index += 1) {
      await reader?.read();
    }
    expect(grants()).toEqual([JSON.stringify({ credit: grantEvery })]);
  });

  it("answers a status with no body without one", async () => {
    const channel = new FakeChannel();
    channel.readyState = "open";
    const answered = sendOnChannel(
      channel,
      new Request("https://app.example/api/v1/x", { method: "PUT" }),
    );
    await settle();
    channel.answer(JSON.stringify({ status: 204 }));
    channel.answer(new ArrayBuffer(0));
    const response = await answered;
    expect(response.status).toBe(204);
    expect(response.body).toBeNull();
  });

  it("fails a request whose channel closes before its response, or whose head is not one", async () => {
    const closed = new FakeChannel();
    closed.readyState = "open";
    const cut = sendOnChannel(closed, new Request("https://app.example/api/v1/x"));
    await settle();
    closed.closeFromTheCore();
    await expect(cut).rejects.toThrow("closed before its response ended");

    const garbled = new FakeChannel();
    garbled.readyState = "open";
    const wrong = sendOnChannel(garbled, new Request("https://app.example/api/v1/x"));
    await settle();
    garbled.answer(JSON.stringify({ state: "ok" }));
    await expect(wrong).rejects.toThrow("not one");
  });

  it("sends each request on the connection with the fewest under way", async () => {
    const channelsOf = [[], []] as FakeChannel[][];
    const peers: PeerLike[] = channelsOf.map((channels) => ({
      createDataChannel: () => {
        const channel = new FakeChannel();
        channel.readyState = "open";
        channels.push(channel);
        return channel;
      },
      close: () => undefined,
    }));
    const transport = new RtcTransport(peers);
    void transport.fetch(new Request("https://app.example/a"));
    void transport.fetch(new Request("https://app.example/b"));
    void transport.fetch(new Request("https://app.example/c"));
    await settle();
    expect(channelsOf.map((channels) => channels.length)).toEqual([2, 1]);
  });

  it("carries at most requestsPerConnection on a connection, the rest waiting until a body ends", async () => {
    const channels: FakeChannel[] = [];
    const peer: PeerLike = {
      createDataChannel: () => {
        const channel = new FakeChannel();
        channel.readyState = "open";
        channels.push(channel);
        return channel;
      },
      close: () => undefined,
    };
    const transport = new RtcTransport([peer]);
    const answered = Array.from({ length: requestsPerConnection + 1 }, (_, index) =>
      transport.fetch(new Request(`https://app.example/poster/${String(index)}`)),
    );
    await settle();
    expect(channels).toHaveLength(requestsPerConnection);
    // The first's head arrives, its body not yet: the core still counts it, so none more is sent.
    channels[0]?.answer(JSON.stringify({ status: 200 }));
    const first = await answered[0];
    await settle();
    expect(channels).toHaveLength(requestsPerConnection);
    channels[0]?.answer(new ArrayBuffer(0));
    expect(await first?.text()).toBe("");
    await settle();
    expect(channels).toHaveLength(requestsPerConnection + 1);
    // A request that fails gives its room back too.
    channels[1]?.closeFromTheCore();
    await expect(answered[1]).rejects.toThrow("closed before its response ended");
    void transport.fetch(new Request("https://app.example/sign-out"));
    await settle();
    expect(channels).toHaveLength(requestsPerConnection + 2);
  });
});

describe("the switchable fetch", () => {
  it("sends on the path in use, and on a new one once it is switched", async () => {
    const paths: string[] = [];
    const via = (name: string) => (request: Request) => {
      paths.push(`${name} ${new URL(request.url).pathname}`);
      return Promise.resolve(new Response(null, { status: 204 }));
    };
    const routed = new SwitchableFetch(via("https"));
    await routed.fetch(new Request("https://app.example/a"));
    routed.use(via("webrtc"), "webrtc");
    await routed.fetch(new Request("https://app.example/b"));
    expect(paths).toEqual(["https /a", "webrtc /b"]);
  });
});

/** A connection the test keeps: the channels opened on it, and its word that it is gone. */
class FakePeer implements PeerLike {
  readonly channels: FakeChannel[] = [];
  isClosed = false;
  private goneListener: (() => void) | undefined;

  createDataChannel(): ChannelLike {
    const channel = new FakeChannel();
    channel.readyState = "open";
    this.channels.push(channel);
    return channel;
  }

  close(): void {
    this.isClosed = true;
  }

  onGone(listener: () => void): void {
    this.goneListener = listener;
  }

  /** The connection failed, or stayed disconnected past the grace. */
  goes(): void {
    this.goneListener?.();
  }
}

describe("the WebRTC transport's connections (plans/webrtc-transport.md W2d)", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  /** A transport on first, opening each next connection from opened, failing while failing is set. */
  function growing(first: FakePeer): {
    transport: RtcTransport;
    opened: FakePeer[];
    failing: { is: boolean };
  } {
    const opened: FakePeer[] = [];
    const failing = { is: false };
    const transport = new RtcTransport([first], {
      size: 4,
      open: () => {
        if (failing.is) {
          return Promise.reject(new Error("it did not connect"));
        }
        const peer = new FakePeer();
        opened.push(peer);
        return Promise.resolve(peer);
      },
    });
    return { transport, opened, failing };
  }

  it("is usable with its first connection, and grows to four behind it", async () => {
    const first = new FakePeer();
    const { transport, opened } = growing(first);
    void transport.fetch(new Request("https://app.example/a"));
    expect(first.channels).toHaveLength(1);
    await settle();
    expect(transport.size).toBe(4);
    expect(opened).toHaveLength(3);
  });

  it("takes a connection that is gone out of use at once, and opens another in its place", async () => {
    const first = new FakePeer();
    const { transport, opened } = growing(first);
    await settle();
    first.goes();
    expect(first.isClosed).toBe(true);
    expect(transport.size).toBe(3);
    for (const path of ["a", "b", "c", "d", "e", "f"]) {
      void transport.fetch(new Request(`https://app.example/${path}`));
    }
    expect(first.channels).toHaveLength(0);
    await settle();
    expect(transport.size).toBe(4);
    expect(opened).toHaveLength(4);
  });

  it("tries a connection that could not be made again after a wait, and a request waits for one", async () => {
    vi.useFakeTimers();
    const first = new FakePeer();
    const { transport, failing } = growing(first);
    await vi.advanceTimersByTimeAsync(0);
    failing.is = true;
    for (const peer of [first]) {
      peer.goes();
    }
    expect(transport.size).toBe(3);
    await vi.advanceTimersByTimeAsync(0);
    failing.is = false;
    await vi.advanceTimersByTimeAsync(replaceFirstMs);
    expect(transport.size).toBe(4);

    const alone = new FakePeer();
    const lonely = new RtcTransport([alone], {
      size: 1,
      open: () => Promise.reject(new Error("none")),
    });
    alone.goes();
    let outcome = "waiting";
    void lonely.fetch(new Request("https://app.example/a")).then(
      () => {
        outcome = "answered";
      },
      () => {
        outcome = "failed";
      },
    );
    await vi.advanceTimersByTimeAsync(replaceFirstMs * 4);
    expect(outcome).toBe("waiting");
    lonely.close();
    await vi.advanceTimersByTimeAsync(0);
    expect(outcome).toBe("failed"); // closed, it fails what waited
  });
});
