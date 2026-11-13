// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  endMessage,
  grantEvery,
  maxMessageBytes,
  sendOnChannel,
  type ChannelLike,
  type ChannelListener,
} from "./rtc-request.ts";
import {
  closingHoldsMs,
  growWhenEachCarries,
  idleClosesAfterMs,
  replaceFirstMs,
  requestsPerConnection,
  RtcTransport,
  settledAfterMs,
  SwitchableFetch,
  type PeerLike,
} from "./rtc-transport.ts";

/** What the test reads of node:fs: the SDK's own types know no Node, as it runs in a browser. */
interface FileReading {
  readonly readFileSync: (file: URL, encoding: "utf8") => string;
}

/**
 * The text message that ends a response its head asked to end in text, as the core writes it: the
 * golden file rtcapi's test checks its End against (rtcapi_test.go).
 */
const coreEnd = (await vi.importActual<FileReading>("node:fs"))
  .readFileSync(
    new URL("../../../../internal/core/rtcapi/testdata/end.json", import.meta.url),
    "utf8",
  )
  .trim();

/** Every channel a test opened, so that the requests it left open are ended after it. */
const channelsOpened: FakeChannel[] = [];

/** A data channel the test plays the core on: it keeps what the client sends and answers by hand. */
class FakeChannel implements ChannelLike {
  readyState: RTCDataChannelState = "connecting";
  readonly sent: (string | ArrayBuffer)[] = [];
  isClosed = false;
  isHeadAnswered = false;
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

  open(): void {
    this.readyState = "open";
    this.listener?.open();
  }

  answer(data: string | ArrayBuffer): void {
    this.isHeadAnswered ||= typeof data === "string";
    this.listener?.message(data);
  }

  /** Ends a request the test left open, as the core would, with no body if no head came yet. */
  endIfOpen(): void {
    if (this.isClosed || this.listener === undefined) {
      return;
    }
    if (!this.isHeadAnswered) {
      this.answer(JSON.stringify({ status: 204 }));
    }
    this.answer(new ArrayBuffer(0));
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

// A request a test leaves open keeps its real silence watch, which fails it silentAtMostMs later, as
// another file runs in the same worker: an unhandled rejection that fails the run (CI, 2026-11-02:
// 41 of them, every test passing). Each is ended once its test is.
afterEach(() => {
  for (const channel of channelsOpened.splice(0)) {
    channel.endIfOpen();
  }
});

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
      // The browser's user agent rides in the head, as over HTTP (local-path-2026-10-27.md §4 item 7).
      // Its length declared and a text end asked for, as Chrome before 92 neither sends nor takes an
      // empty message (rtcapi's EndHeader; the owner's Chromecast, production, 2026-11-12).
      headers: {
        authorization: ["Bearer t"],
        "user-agent": [navigator.userAgent],
        "content-length": [String(maxMessageBytes + 10)],
        "x-orpheum-end": ["text"],
      },
    });
    expect(navigator.userAgent).not.toBe("");
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

  it("ends a response at the core's text end, as Chrome before 92 never takes an empty one", async () => {
    const channel = new FakeChannel();
    channel.readyState = "open";
    const answered = sendOnChannel(
      channel,
      new Request("https://app.example/api/v1/guest-receivers"),
    );
    await settle();
    expect(channel.sentHead()).toMatchObject({
      headers: { "content-length": ["0"], "x-orpheum-end": ["text"] },
    });
    channel.answer(JSON.stringify({ status: 200 }));
    const response = await answered;
    channel.answer(new TextEncoder().encode("guest").buffer);
    expect(endMessage).toBe(coreEnd);
    channel.answer(coreEnd);
    expect(await response.text()).toBe("guest");
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
    // Its body ended and the client closed it, but the core counts it until its end closes too.
    expect(channels[0]?.isClosed).toBe(true);
    expect(channels).toHaveLength(requestsPerConnection);
    channels[0]?.closeFromTheCore();
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

// One room freed lets one waiting request through, not every one: woken together, each saw the room
// before any took it, and a films page's posters crowded past the core's 16 on one connection, which
// refused them (staging, 2026-10-20: twenty request channels open at once on one connection).
it("lets one waiting request through for each room freed, however many wait", async () => {
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
  const posters = 60;
  const answered = Array.from({ length: posters }, (_, index) =>
    transport.fetch(new Request(`https://app.example/poster/${String(index)}`)),
  );
  await settle();
  expect(channels).toHaveLength(requestsPerConnection);
  channels[0]?.closeFromTheCore();
  await expect(answered[0]).rejects.toThrow("closed before its response ended");
  await settle();
  expect(channels).toHaveLength(requestsPerConnection + 1);
  channels[1]?.closeFromTheCore();
  channels[2]?.closeFromTheCore();
  await Promise.allSettled([answered[1], answered[2]]);
  await settle();
  expect(channels).toHaveLength(requestsPerConnection + 3);
  // The rest fail as the transport closes: those waiting at once, those under way as their channels do.
  transport.close();
  for (const each of channels) {
    each.closeFromTheCore();
  }
  await Promise.allSettled(answered);
});

it("gives a closed channel's room back after closingHoldsMs when its close never comes", async () => {
  vi.useFakeTimers();
  try {
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
    await vi.advanceTimersByTimeAsync(0);
    channels[0]?.answer(JSON.stringify({ status: 200 }));
    const first = await answered[0];
    channels[0]?.answer(new ArrayBuffer(0));
    expect(await first?.text()).toBe("");
    expect(channels[0]?.isClosed).toBe(true);
    await vi.advanceTimersByTimeAsync(closingHoldsMs - 1);
    expect(channels).toHaveLength(requestsPerConnection);
    await vi.advanceTimersByTimeAsync(1);
    expect(channels).toHaveLength(requestsPerConnection + 1);
  } finally {
    vi.useRealTimers();
  }
});

// A request its caller gave up, a background refresh's after a minute or a chunk hls.js no longer
// wants, kept its channel open and its room taken, as the transport never read its signal.
describe("a request whose signal aborts", () => {
  /** A transport on one connection, and the channels opened on it. */
  function onePeer(): { transport: RtcTransport; channels: FakeChannel[] } {
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
    return { transport: new RtcTransport([peer]), channels };
  }

  it("fails at once with an AbortError, opening nothing, when it aborted before it was sent", async () => {
    const { transport, channels } = onePeer();
    const aborter = new AbortController();
    aborter.abort();
    await expect(
      transport.fetch(new Request("https://app.example/a", { signal: aborter.signal })),
    ).rejects.toMatchObject({ name: "AbortError" });
    expect(channels).toHaveLength(0);
  });

  it("fails with the signal's reason and closes its channel, before its head or in its body", async () => {
    const { transport, channels } = onePeer();
    const beforeHead = new AbortController();
    const unanswered = transport.fetch(
      new Request("https://app.example/a", { signal: beforeHead.signal }),
    );
    await settle();
    beforeHead.abort();
    await expect(unanswered).rejects.toMatchObject({ name: "AbortError" });
    expect(channels[0]?.isClosed).toBe(true);

    const inBody = new AbortController();
    const answered = transport.fetch(
      new Request("https://app.example/film", { signal: inBody.signal }),
    );
    await settle();
    channels[1]?.answer(JSON.stringify({ status: 206 }));
    const reader = (await answered).body?.getReader();
    channels[1]?.answer(new Uint8Array([1]).buffer);
    expect((await reader?.read())?.value).toEqual(new Uint8Array([1]));
    const givenUp = new Error("given up");
    inBody.abort(givenUp);
    await expect(reader?.read()).rejects.toBe(givenUp);
    expect(channels[1]?.isClosed).toBe(true);
  });

  it("gives its room back once its channel's close comes", async () => {
    const { transport, channels } = onePeer();
    const aborter = new AbortController();
    void transport
      .fetch(new Request("https://app.example/0", { signal: aborter.signal }))
      .catch(() => undefined);
    for (let index = 1; index <= requestsPerConnection; index += 1) {
      void transport.fetch(new Request(`https://app.example/${String(index)}`));
    }
    await settle();
    expect(channels).toHaveLength(requestsPerConnection);
    aborter.abort();
    await settle();
    expect(channels[0]?.isClosed).toBe(true);
    channels[0]?.closeFromTheCore();
    await settle();
    expect(channels).toHaveLength(requestsPerConnection + 1);
  });

  it("leaves the queue when it aborts waiting for room, and the next waiting takes the room", async () => {
    const { transport, channels } = onePeer();
    const full = Array.from({ length: requestsPerConnection }, (_, index) =>
      transport.fetch(new Request(`https://app.example/poster/${String(index)}`)),
    );
    const aborter = new AbortController();
    const givenUp = transport.fetch(
      new Request("https://app.example/given-up", { signal: aborter.signal }),
    );
    void transport.fetch(new Request("https://app.example/still-wanted"));
    await settle();
    aborter.abort();
    await expect(givenUp).rejects.toMatchObject({ name: "AbortError" });
    channels[0]?.closeFromTheCore();
    await expect(full[0]).rejects.toThrow("closed before its response ended");
    await settle();
    expect(channels).toHaveLength(requestsPerConnection + 1);
    expect(channels.at(-1)?.sentHead()).toMatchObject({ path: "/still-wanted" });
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

  /** Sends count requests on transport that no one answers, so each stays under way. */
  function crowd(transport: RtcTransport, count: number): void {
    for (let index = 0; index < count; index += 1) {
      transport
        .fetch(new Request(`https://app.example/poster/${String(index)}`))
        .catch(() => undefined);
    }
  }

  it("is usable with its first connection, and grows to four only while its requests crowd it", async () => {
    const first = new FakePeer();
    const { transport, opened } = growing(first);
    await settle();
    // A tab with nothing under way, or one request at a time, keeps the one connection.
    expect(transport.size).toBe(1);
    void transport.fetch(new Request("https://app.example/a"));
    expect(first.channels).toHaveLength(1);
    await settle();
    expect(transport.size).toBe(1);
    expect(opened).toHaveLength(0);
    crowd(transport, 7);
    for (let more = 0; more < 4; more += 1) {
      await settle();
      crowd(transport, 2);
    }
    await settle();
    expect(transport.size).toBe(4);
    expect(opened).toHaveLength(3);
  });

  it("closes a connection beyond its last once it has carried nothing for idleClosesAfterMs", async () => {
    vi.useFakeTimers();
    const first = new FakePeer();
    const { transport, opened } = growing(first);
    crowd(transport, growWhenEachCarries);
    await vi.advanceTimersByTimeAsync(0);
    expect(transport.size).toBe(2);
    for (const channel of [...first.channels, ...(opened[0]?.channels ?? [])]) {
      channel.closeFromTheCore();
    }
    await vi.advanceTimersByTimeAsync(idleClosesAfterMs - 1);
    expect(transport.size).toBe(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(transport.size).toBe(1);
    expect([first.isClosed, opened[0]?.isClosed].filter(Boolean)).toHaveLength(1);
    // The last is kept however long it carries nothing.
    await vi.advanceTimersByTimeAsync(idleClosesAfterMs * 10);
    expect(transport.size).toBe(1);
  });

  it("keeps a connection a socket's channel is open on, idle or not", async () => {
    vi.useFakeTimers();
    const first = new FakePeer();
    const { transport, opened } = growing(first);
    crowd(transport, growWhenEachCarries);
    await vi.advanceTimersByTimeAsync(0);
    const second = opened[0];
    // The socket goes on the connection with the fewest under way: the new one.
    const socket = transport.openChannel();
    socket.listen({
      open: () => undefined,
      message: () => undefined,
      close: () => undefined,
      error: () => undefined,
    });
    for (const channel of first.channels) {
      channel.closeFromTheCore();
    }
    await vi.advanceTimersByTimeAsync(idleClosesAfterMs * 2);
    expect(transport.size).toBe(1);
    expect(first.isClosed).toBe(true);
    expect(second?.isClosed).toBe(false);
    socket.close();
    crowd(transport, growWhenEachCarries);
    await vi.advanceTimersByTimeAsync(0);
    expect(transport.size).toBe(2);
  });

  it("takes a connection that is gone out of use at once, and opens another in its place", async () => {
    vi.useFakeTimers();
    const first = new FakePeer();
    const { transport, opened } = growing(first);
    crowd(transport, growWhenEachCarries);
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    expect(transport.size).toBe(2);
    first.goes();
    expect(first.isClosed).toBe(true);
    expect(transport.size).toBe(1);
    for (const path of ["a", "b", "c", "d", "e", "f"]) {
      void transport.fetch(new Request(`https://app.example/${path}`));
    }
    expect(first.channels).toHaveLength(growWhenEachCarries);
    await vi.advanceTimersByTimeAsync(0);
    expect(transport.size).toBe(2);
    expect(opened).toHaveLength(2);
  });

  it("tries a connection that could not be made again after a wait, and a request waits for one", async () => {
    vi.useFakeTimers();
    const first = new FakePeer();
    const { transport, failing } = growing(first);
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    failing.is = true;
    for (const peer of [first]) {
      peer.goes();
    }
    expect(transport.size).toBe(0);
    await vi.advanceTimersByTimeAsync(0);
    failing.is = false;
    await vi.advanceTimersByTimeAsync(replaceFirstMs);
    expect(transport.size).toBe(1);

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

  // A connection the core closes as soon as it connects, as one past its network's share, was
  // replaced at once, each new one closed in turn: a refusal about every second for as long as the
  // page stayed open (staging, 2026-10-25).
  it("replaces a connection gone before settledAfterMs only after a wait that doubles", async () => {
    vi.useFakeTimers();
    const opened: FakePeer[] = [];
    const first = new FakePeer();
    const transport = new RtcTransport([first], {
      size: 4,
      open: () => {
        const peer = new FakePeer();
        opened.push(peer);
        return Promise.resolve(peer);
      },
    });
    first.goes();
    await vi.advanceTimersByTimeAsync(0);
    expect(opened).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(replaceFirstMs);
    expect(opened).toHaveLength(1);
    opened[0]?.goes();
    await vi.advanceTimersByTimeAsync(replaceFirstMs);
    expect(opened).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(replaceFirstMs);
    expect(opened).toHaveLength(2);
    // One that lives settledAfterMs is replaced at once when it goes.
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    opened[1]?.goes();
    await vi.advanceTimersByTimeAsync(0);
    expect(opened).toHaveLength(3);
    expect(transport.size).toBe(1);
  });
});
