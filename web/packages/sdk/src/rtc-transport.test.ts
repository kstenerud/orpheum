// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  connectTimeoutMs,
  gatherAfterReflexiveMs,
  gatherForRelayMs,
  gathered,
  hasRelayServer,
  slowConnection,
  slowConnectMs,
  type GatheringPeer,
} from "./rtc-connect.ts";
import {
  grantEvery,
  maxMessageBytes,
  replaceFirstMs,
  closingHoldsMs,
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

/** A peer gathering candidates as the test says: each found, and gathering complete. */
class FakeGatheringPeer implements GatheringPeer {
  iceGatheringState: RTCIceGatheringState = "gathering";
  readonly #onState: (() => void)[] = [];
  readonly #onCandidate: ((event: RTCPeerConnectionIceEvent) => void)[] = [];

  addEventListener(type: "icegatheringstatechange", listener: () => void): void;
  addEventListener(
    type: "icecandidate",
    listener: (event: RTCPeerConnectionIceEvent) => void,
  ): void;
  addEventListener(type: string, listener: (event: RTCPeerConnectionIceEvent) => void): void {
    if (type === "icecandidate") {
      this.#onCandidate.push(listener);
    } else {
      this.#onState.push(() => {
        listener(undefined as never);
      });
    }
  }

  find(type: RTCIceCandidateType): void {
    const event = { candidate: { type } } as unknown as RTCPeerConnectionIceEvent;
    this.#onCandidate.forEach((listener) => {
      listener(event);
    });
  }

  complete(): void {
    this.iceGatheringState = "complete";
    this.#onState.forEach((listener) => {
      listener();
    });
  }
}

describe("gathered", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  // Gathering completes only when every STUN attempt has answered or given up, some forty seconds
  // on an interface with no way out, so each offer waited out connectTimeoutMs, and a realm sign-in
  // took 28 s on staging (2026-10-23).
  it("goes gatherAfterReflexiveMs after the first candidate that reaches past this machine", async () => {
    vi.useFakeTimers();
    const peer = new FakeGatheringPeer();
    let isDone = false;
    void gathered(peer).then(() => {
      isDone = true;
    });
    peer.find("host");
    await vi.advanceTimersByTimeAsync(gatherAfterReflexiveMs * 2);
    expect(isDone).toBe(false);
    peer.find("srflx");
    peer.find("srflx");
    await vi.advanceTimersByTimeAsync(gatherAfterReflexiveMs - 1);
    expect(isDone).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(isDone).toBe(true);
  });

  it("with a TURN server, waits for the relayed candidate, up to gatherForRelayMs", async () => {
    vi.useFakeTimers();
    const relayed = new FakeGatheringPeer();
    let isRelayedDone = false;
    void gathered(relayed, true).then(() => {
      isRelayedDone = true;
    });
    relayed.find("srflx");
    await vi.advanceTimersByTimeAsync(gatherAfterReflexiveMs);
    expect(isRelayedDone).toBe(false);
    relayed.find("relay");
    await vi.advanceTimersByTimeAsync(gatherAfterReflexiveMs);
    expect(isRelayedDone).toBe(true);
    const noRelay = new FakeGatheringPeer();
    let isNoRelayDone = false;
    void gathered(noRelay, true).then(() => {
      isNoRelayDone = true;
    });
    noRelay.find("srflx");
    await vi.advanceTimersByTimeAsync(gatherForRelayMs - 1);
    expect(isNoRelayDone).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(isNoRelayDone).toBe(true);
  });

  it("expects a relayed candidate only from a TURN server", () => {
    expect(hasRelayServer({ iceServers: [{ urls: "stun:stun.example.com:3478" }] })).toBe(false);
    expect(
      hasRelayServer({ iceServers: [{ urls: ["stun:a", "turns:relay.example.com:5349"] }] }),
    ).toBe(true);
    expect(hasRelayServer({})).toBe(false);
  });

  it("goes when gathering completes, or at connectTimeoutMs with only host candidates", async () => {
    vi.useFakeTimers();
    const completes = new FakeGatheringPeer();
    let isCompleteDone = false;
    void gathered(completes).then(() => {
      isCompleteDone = true;
    });
    completes.complete();
    await vi.advanceTimersByTimeAsync(0);
    expect(isCompleteDone).toBe(true);
    const hostOnly = new FakeGatheringPeer();
    let isHostOnlyDone = false;
    void gathered(hostOnly).then(() => {
      isHostOnlyDone = true;
    });
    hostOnly.find("host");
    await vi.advanceTimersByTimeAsync(connectTimeoutMs - 1);
    expect(isHostOnlyDone).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(isHostOnlyDone).toBe(true);
  });
});

describe("slowConnection", () => {
  it("says where a slow connection's time went, and nothing of one within slowConnectMs", () => {
    expect(
      slowConnection({ gathering: 1_000, answer: 1_000, connecting: slowConnectMs - 2_000 }),
    ).toBe(undefined);
    expect(slowConnection({ gathering: 10_004.4, answer: 2_100, connecting: 300 })).toBe(
      "a WebRTC connection took 12404 ms: gathering its candidates 10004 ms, the core's answer 2100 ms, " +
        "connecting 300 ms",
    );
  });
});
