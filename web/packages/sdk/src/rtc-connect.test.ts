// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  connected,
  connectRtc,
  connectTimeoutMs,
  gatherAfterReflexiveMs,
  gatherForRelayMs,
  gathered,
  hasRelayServer,
  isSecureUnfinished,
  isTurnedAway,
  offerWith,
  RtcUnconnected,
  slowConnection,
  slowConnectMs,
  TurnedAway,
  verdictOf,
  verdictWaitMs,
  type ConnectingPeer,
  type GatheringPeer,
} from "./rtc-connect.ts";
import type { ChannelLike, ChannelListener } from "./rtc-request.ts";
import { forgetSteps, stepsSoFar } from "./steps.ts";

/** A negotiation channel the test plays the core on: it opens, says and closes as the test says. */
class FakeChannel implements ChannelLike {
  readyState: RTCDataChannelState = "connecting";
  private listener: ChannelListener | undefined;

  send(): void {
    // The client sends nothing on its negotiation channel.
  }

  close(): void {
    this.readyState = "closed";
  }

  listen(listener: ChannelListener): void {
    this.listener = listener;
  }

  open(): void {
    this.readyState = "open";
    this.listener?.open();
  }

  answer(data: string): void {
    this.listener?.message(data);
  }

  closeFromTheCore(): void {
    this.listener?.close();
  }
}

/** The negotiation channel the core tells its verdict on, played by the test. */
describe("the core's verdict on a connection (rtcapi.Verdict)", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("is held when the core says so, or says nothing for verdictWaitMs after the channel opens", async () => {
    vi.useFakeTimers();
    const told = new FakeChannel();
    const held = verdictOf(told);
    told.open();
    told.answer(JSON.stringify({ isHeld: true }));
    await expect(held).resolves.toBeUndefined();

    const silent = new FakeChannel();
    let isSettled = false;
    void verdictOf(silent).then(() => {
      isSettled = true;
    });
    await vi.advanceTimersByTimeAsync(verdictWaitMs * 5);
    expect(isSettled).toBe(false); // not before the channel opens
    silent.open();
    await vi.advanceTimersByTimeAsync(verdictWaitMs - 1);
    expect(isSettled).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(isSettled).toBe(true);
  });

  it("fails as turned away for the network's share, which a page can tell from any other failure", async () => {
    const told = new FakeChannel();
    const refused = verdictOf(told);
    told.open();
    told.answer(JSON.stringify({ refused: "networkFull" }));
    const problem: unknown = await refused.catch((caught: unknown) => caught);
    expect(problem).toBeInstanceOf(TurnedAway);
    expect(isTurnedAway(problem)).toBe(true);
    expect(isTurnedAway(new Error("reached nothing", { cause: problem }))).toBe(true);
    expect(isTurnedAway(new Error("the WebRTC connection failed"))).toBe(false);

    const closed = new FakeChannel();
    const cut = verdictOf(closed);
    closed.open();
    closed.closeFromTheCore();
    const cutProblem: unknown = await cut.catch((caught: unknown) => caught);
    expect(isTurnedAway(cutProblem)).toBe(false);
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

describe("offerWith", () => {
  it("offers the negotiation channel and a video m-line that only receives, for Chrome's socket buffer", () => {
    const calls: unknown[][] = [];
    const channel = offerWith({
      createDataChannel: (label: string) => {
        calls.push(["createDataChannel", label]);
        return "the channel";
      },
      addTransceiver: (kind: "video", init: { direction: "recvonly" }) => {
        calls.push(["addTransceiver", kind, init]);
        return undefined;
      },
    });
    expect(channel).toBe("the channel");
    expect(calls).toEqual([
      ["createDataChannel", "hello"],
      ["addTransceiver", "video", { direction: "recvonly" }],
    ]);
  });
});

/**
 * A browser's peer connection that offers, gathers at once and takes no answer: the core's answer
 * it refuses, as a browser refuses one it cannot use.
 */
class RefusingPeer {
  readonly iceGatheringState = "complete";
  readonly localDescription = { type: "offer", sdp: "v=0" };
  isClosed = false;
  createDataChannel(): EventTarget & { readyState: string } {
    return Object.assign(new EventTarget(), { readyState: "connecting" });
  }
  addTransceiver(): void {
    // The video m-line is offered; nothing comes of it here.
  }
  addEventListener(): void {
    // Gathering is complete already.
  }
  createOffer(): Promise<{ type: string; sdp: string }> {
    return Promise.resolve(this.localDescription);
  }
  setLocalDescription(): Promise<void> {
    return Promise.resolve();
  }
  setRemoteDescription(): Promise<void> {
    return Promise.reject(new Error("the answer does not fit the offer"));
  }
  close(): void {
    this.isClosed = true;
  }
}

describe("the steps of a WebRTC connection", () => {
  afterEach(() => {
    forgetSteps();
    vi.unstubAllGlobals();
  });

  it("are told up to the one that failed, and why, for what happened", async () => {
    vi.stubGlobal("RTCPeerConnection", RefusingPeer);
    await expect(
      connectRtc(() => Promise.resolve({ type: "answer", sdp: "v=0" }), {}, 1),
    ).rejects.toThrow("the answer does not fit the offer");
    expect(stepsSoFar().map((step) => step.replace(/^\d+ ms /, ""))).toEqual([
      "WebRTC: offer sent",
      "WebRTC: the core answered",
      "WebRTC: failed: the answer does not fit the offer",
    ]);
  });
});

/**
 * A peer connection whose states the test sets, its stats holding one candidate pair that the
 * transport selected, from the browser's host candidate to the core's, or none.
 */
class StalledPeer implements ConnectingPeer {
  connectionState: RTCPeerConnectionState = "connecting";
  iceConnectionState: RTCIceConnectionState = "checking";
  hasPair = false;
  readonly #listeners: (() => void)[] = [];
  addEventListener(_type: "connectionstatechange", listener: () => void): void {
    this.#listeners.push(listener);
  }
  moves(connection: RTCPeerConnectionState): void {
    this.connectionState = connection;
    this.#listeners.forEach((listener) => {
      listener();
    });
  }
  getStats(): Promise<RTCStatsReport> {
    const entries = this.hasPair
      ? [
          { id: "t", type: "transport", selectedCandidatePairId: "p" },
          { id: "p", type: "candidate-pair", localCandidateId: "l", remoteCandidateId: "r" },
          {
            id: "l",
            type: "candidate",
            candidateType: "host",
            protocol: "udp",
            address: "a.local",
          },
          {
            id: "r",
            type: "candidate",
            candidateType: "host",
            protocol: "udp",
            address: "192.0.2.1",
          },
        ]
      : [];
    const report = new Map(entries.map((entry) => [entry.id, entry]));
    return Promise.resolve(report as unknown as RTCStatsReport);
  }
}

describe("a WebRTC connection that never connects", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  // The owner's Firefox 158 (2026-11-07): ICE checks succeeded both ways and no DTLS packet came;
  // the page said only "did not connect in time", which the dead end read as a network with no way.
  it("says how far it got: ICE through, the secure connection not, and the pair's kinds, no address", async () => {
    vi.useFakeTimers();
    const peer = new StalledPeer();
    peer.iceConnectionState = "connected";
    peer.hasPair = true;
    const failure = connected(peer).catch((problem: unknown) => problem);
    await vi.advanceTimersByTimeAsync(connectTimeoutMs);
    const problem = await failure;
    expect(problem).toBeInstanceOf(RtcUnconnected);
    expect(problem instanceof Error ? problem.message : "").toBe(
      "the WebRTC connection did not connect in time (ICE connected, connection connecting, pair host to host (udp))",
    );
    expect(isSecureUnfinished(problem)).toBe(true);
    expect(isSecureUnfinished(new Error("reaching failed", { cause: problem }))).toBe(true);
    expect(isSecureUnfinished(new AggregateError([new Error("no"), problem]))).toBe(true);
  });

  it("whose ICE never got through is no secure connection left unfinished", async () => {
    vi.useFakeTimers();
    const peer = new StalledPeer();
    const failure = connected(peer).catch((problem: unknown) => problem);
    peer.iceConnectionState = "failed";
    peer.moves("failed");
    await vi.advanceTimersByTimeAsync(0);
    const problem = await failure;
    expect(problem instanceof Error ? problem.message : "").toBe(
      "the WebRTC connection failed (ICE failed, connection failed, no pair selected)",
    );
    expect(isSecureUnfinished(problem)).toBe(false);
    expect(isSecureUnfinished(new Error("the WebRTC connection did not connect in time"))).toBe(
      false,
    );
  });

  it("connects as its connection does", async () => {
    const peer = new StalledPeer();
    const connecting = connected(peer);
    peer.moves("connected");
    await expect(connecting).resolves.toBeUndefined();
  });
});
