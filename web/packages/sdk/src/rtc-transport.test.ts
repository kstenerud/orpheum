// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  maxMessageBytes,
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
