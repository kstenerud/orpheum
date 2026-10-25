// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  connectTimeoutMs,
  gatherAfterReflexiveMs,
  gatherForRelayMs,
  gathered,
  hasRelayServer,
  isTurnedAway,
  slowConnection,
  slowConnectMs,
  TurnedAway,
  verdictOf,
  verdictWaitMs,
  type GatheringPeer,
} from "./rtc-connect.ts";
import type { ChannelLike, ChannelListener } from "./rtc-request.ts";

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
    told.answer(JSON.stringify({ held: true }));
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
    told.answer(JSON.stringify({ refused: "network-full" }));
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
