// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  connected,
  connectRtc,
  connectTimeoutMs,
  gatherAfterReflexiveMs,
  gatherForRelayMs,
  gathered,
  growthOn,
  isSecureUnfinished,
  isTurnedAway,
  offerWith,
  onWayBack,
  RtcUnconnected,
  TurnedAway,
  verdictOf,
  verdictWaitMs,
  whenIceThrough,
  type ConnectingPeer,
  type GatheringPeer,
} from "./rtc-connect.ts";
import {
  gatheringErrorsOf,
  gatheringText,
  hasRelayServer,
  isPastThisDevice,
} from "./rtc-candidates.ts";
import { connectingStepsOf, slowConnection, slowConnectMs } from "./rtc-connect-timing.ts";
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

  // A connection that took 6.5 s said only that 5.4 s went connecting, which could have been ICE's
  // pacing, DTLS's retransmits or the verdict's wait (the slow-link war game, 2026-11-14; D484 §1).
  it("says how connecting went, step by step, in the same sentence", () => {
    const timings = { gathering: 1_000, answer: 100, connecting: 5_448 };
    expect(
      slowConnection({
        ...timings,
        steps: { ice: 400, secure: 4_000, channel: 48, verdict: 1_000 },
      }),
    ).toBe(
      "a WebRTC connection took 6548 ms: gathering its candidates 1000 ms, the core's answer 100 ms, " +
        "connecting 5448 ms (ICE 400 ms, DTLS 4000 ms, SCTP and the channel 48 ms, the core's verdict 1000 ms)",
    );
    expect(
      slowConnection({
        ...timings,
        steps: { ice: undefined, secure: 4_400, channel: 48, verdict: 1_000 },
      }),
    ).toBe(
      "a WebRTC connection took 6548 ms: gathering its candidates 1000 ms, the core's answer 100 ms, " +
        "connecting 5448 ms (ICE and DTLS 4400 ms, SCTP and the channel 48 ms, the core's verdict 1000 ms)",
    );
  });
});

describe("connectingStepsOf (D484 §1)", () => {
  it("is each step's time, from when each ended", () => {
    expect(
      connectingStepsOf({
        answeredAt: 100,
        iceAt: 500,
        connectedAt: 4_500,
        channelAt: 4_548,
        heldAt: 5_548,
      }),
    ).toEqual({ ice: 400, secure: 4_000, channel: 48, verdict: 1_000 });
  });

  it("gives a step whose end went unseen, or was heard out of order, none, its time in the next", () => {
    // No ICE state told: its time goes with DTLS's.
    expect(
      connectingStepsOf({
        answeredAt: 100,
        iceAt: undefined,
        connectedAt: 4_500,
        channelAt: 4_548,
        heldAt: 5_548,
      }),
    ).toEqual({ ice: undefined, secure: 4_400, channel: 48, verdict: 1_000 });
    // The channel's open told before the connection's state, and no channel seen at all.
    expect(
      connectingStepsOf({
        answeredAt: 100,
        iceAt: 500,
        connectedAt: 4_500,
        channelAt: 4_400,
        heldAt: 5_000,
      }),
    ).toEqual({ ice: 400, secure: 4_000, channel: 0, verdict: 500 });
    expect(
      connectingStepsOf({
        answeredAt: 100,
        iceAt: 50,
        connectedAt: 4_500,
        channelAt: undefined,
        heldAt: 5_000,
      }),
    ).toEqual({ ice: 0, secure: 4_400, channel: 0, verdict: 500 });
  });
});

describe("whenIceThrough (D484 §1)", () => {
  it("is when ICE first had a pair that answered, and nothing before", () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    try {
      const listeners: (() => void)[] = [];
      const peer = {
        iceConnectionState: "checking" as RTCIceConnectionState,
        addEventListener: (_type: "iceconnectionstatechange", listener: () => void): void => {
          listeners.push(listener);
        },
      };
      const tell = (state: RTCIceConnectionState): void => {
        peer.iceConnectionState = state;
        listeners.forEach((listener) => {
          listener();
        });
      };
      const iceAt = whenIceThrough(peer);
      const startedAt = performance.now();
      tell("checking");
      expect(iceAt()).toBeUndefined();
      vi.advanceTimersByTime(400);
      tell("connected");
      vi.advanceTimersByTime(100);
      tell("completed");
      expect(iceAt()).toBe(startedAt + 400);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("the core's verdict, timed (D484 §1)", () => {
  it("says when its channel opened, once", async () => {
    vi.useFakeTimers();
    try {
      const told = new FakeChannel();
      let opened = 0;
      const held = verdictOf(told, () => {
        opened += 1;
      });
      expect(opened).toBe(0);
      told.open();
      told.open();
      expect(opened).toBe(1);
      told.answer(JSON.stringify({ isHeld: true }));
      await expect(held).resolves.toBeUndefined();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("onWayBack", () => {
  it("tells each time the page is shown again, never as it is hidden, until stopped (the owner's phone, 2026-11-14)", () => {
    const listeners = new Set<() => void>();
    const page = {
      visibilityState: "hidden" as DocumentVisibilityState,
      addEventListener: (_type: "visibilitychange", listener: () => void) =>
        listeners.add(listener),
      removeEventListener: (_type: "visibilitychange", listener: () => void) =>
        listeners.delete(listener),
    };
    const turn = (to: DocumentVisibilityState): void => {
      page.visibilityState = to;
      for (const listener of listeners) {
        listener();
      }
    };
    let told = 0;
    const stop = onWayBack(page)(() => (told += 1));
    turn("hidden");
    expect(told).toBe(0);
    turn("visible");
    expect(told).toBe(1);
    stop();
    turn("visible");
    expect(told).toBe(1);
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
  readonly localDescription = { type: "offer", sdp: sdpWith(reflexiveV4) };
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
      connectRtc(() => Promise.resolve({ type: "answer", sdp: "v=0" }), {}, { count: 1 }),
    ).rejects.toThrow("the answer does not fit the offer");
    expect(stepsSoFar().map((step) => step.replace(/^\d+ ms /, ""))).toEqual([
      "WebRTC: offer sent with 1 srflx IPv4",
      "WebRTC: the core answered",
      "WebRTC: failed: the answer does not fit the offer",
    ]);
  });
});

/** Candidate lines of each kind, at documentation addresses and an mDNS name. */
const hostMdns =
  "a=candidate:1 1 UDP 2122252543 1f3c9a2e-0b7d-4c55-9e0a-5d1b2c3d4e5f.local 50000 typ host";
const hostV4 = "a=candidate:2 1 UDP 2122187007 192.0.2.10 50001 typ host";
const reflexiveV4 =
  "a=candidate:3 1 UDP 1685921791 203.0.113.7 40000 typ srflx raddr 0.0.0.0 rport 0";
const relayedV6 =
  "a=candidate:4 1 UDP 92216831 2001:db8::7 3478 typ relay raddr 2001:db8::1 rport 50002";

/** An offer's SDP holding candidates. */
function sdpWith(...candidates: string[]): string {
  return ["v=0", "m=application 9 UDP/DTLS/SCTP webrtc-datachannel", ...candidates, ""].join(
    "\r\n",
  );
}

describe("what an offer gathered (the owner's phone on a bus's Wi-Fi, 2026-11-26)", () => {
  it("is told by kind and family, never an address or a name", () => {
    const text = gatheringText(sdpWith(hostMdns, hostMdns, hostV4, reflexiveV4, relayedV6));
    expect(text).toBe("2 host (mDNS), 1 host IPv4, 1 srflx IPv4, 1 relay IPv6");
    expect(text).not.toMatch(/192\.0\.2|203\.0\.113|2001:db8|\.local/);
  });

  it("says so when nothing in it reaches past this device, with the errors gathering met", () => {
    expect(isPastThisDevice(sdpWith(hostMdns, hostMdns))).toBe(false);
    expect(isPastThisDevice(sdpWith(hostV4))).toBe(true);
    expect(isPastThisDevice(sdpWith(reflexiveV4))).toBe(true);
    expect(gatheringText(sdpWith(hostMdns), ["turn 486"])).toBe(
      "1 host (mDNS), no candidate past this device; gathering errors: turn 486",
    );
    expect(gatheringText("v=0")).toBe("no candidate past this device");
  });

  it("tells each gathering error once, as its server's scheme and code, never its address", () => {
    const listeners: ((event: RTCPeerConnectionIceErrorEvent) => void)[] = [];
    const errors = gatheringErrorsOf({
      addEventListener: (_, listener) => {
        listeners.push(listener);
      },
    });
    const fail = (url: string, errorCode: number): void => {
      const event = { url, errorCode, address: "192.0.2.10", errorText: "x" };
      listeners.forEach((listener) => {
        listener(event as unknown as RTCPeerConnectionIceErrorEvent);
      });
    };
    fail("turn:relay.example.com:3478?transport=udp", 486);
    fail("turn:relay.example.com:3478?transport=udp", 486);
    fail("STUN:stun.example.com:3478", 701);
    fail("turns:relay.example.com:5349", 701);
    expect(errors()).toEqual(["turn 486", "stun 701", "turns 701"]);
  });
});

/**
 * A browser's peer connection whose offer holds what the test queued for it (peerOffers), gathered
 * at once or after gatherMs (never, for Infinity), its gathering errors told as it gathers, and
 * which takes no answer, as RefusingPeer.
 */
interface PeerOffer {
  readonly sdp: string;
  readonly gatherMs?: number;
  readonly errors?: readonly { url: string; errorCode: number }[];
}
const peerOffers: PeerOffer[] = [];
const madePeers: GatheringOncePeer[] = [];

class GatheringOncePeer {
  iceGatheringState: RTCIceGatheringState = "gathering";
  readonly localDescription: { type: string; sdp: string };
  isClosed = false;
  readonly #offer: PeerOffer;
  readonly #listeners = new Map<string, ((event: unknown) => void)[]>();
  constructor() {
    this.#offer = peerOffers.shift() ?? { sdp: "v=0" };
    this.localDescription = { type: "offer", sdp: this.#offer.sdp };
    madePeers.push(this);
  }
  createDataChannel(): EventTarget & { readyState: string } {
    return Object.assign(new EventTarget(), { readyState: "connecting" });
  }
  addTransceiver(): void {
    // The video m-line is offered; nothing comes of it here.
  }
  addEventListener(type: string, listener: (event: unknown) => void): void {
    this.#listeners.set(type, [...(this.#listeners.get(type) ?? []), listener]);
  }
  createOffer(): Promise<{ type: string; sdp: string }> {
    return Promise.resolve(this.localDescription);
  }
  setLocalDescription(): Promise<void> {
    for (const error of this.#offer.errors ?? []) {
      this.#tell("icecandidateerror", error);
    }
    const { gatherMs = 0 } = this.#offer;
    if (gatherMs === 0) {
      this.iceGatheringState = "complete";
    } else if (gatherMs !== Infinity) {
      setTimeout(() => {
        this.iceGatheringState = "complete";
        this.#tell("icegatheringstatechange", undefined);
      }, gatherMs);
    }
    return Promise.resolve();
  }
  setRemoteDescription(): Promise<void> {
    return Promise.reject(new Error("the answer does not fit the offer"));
  }
  close(): void {
    this.isClosed = true;
  }
  #tell(type: string, event: unknown): void {
    (this.#listeners.get(type) ?? []).forEach((listener) => {
      listener(event);
    });
  }
}

describe("an offer that gathered nothing past this device (the owner's phone, 2026-11-26)", () => {
  afterEach(() => {
    forgetSteps();
    vi.unstubAllGlobals();
    vi.useRealTimers();
    peerOffers.length = 0;
    madePeers.length = 0;
  });

  /**
   * Connects once through a signaller that records each offer's SDP, and when it was sent (onSent);
   * returns them and the steps.
   */
  async function connectRecording(
    onSent: () => void = () => undefined,
  ): Promise<{ sent: string[]; steps: string[] }> {
    vi.stubGlobal("RTCPeerConnection", GatheringOncePeer);
    const sent: string[] = [];
    await expect(
      connectRtc(
        (offer) => {
          sent.push(offer.sdp);
          onSent();
          return Promise.resolve({ type: "answer", sdp: "v=0" });
        },
        {},
        { count: 1 },
      ),
    ).rejects.toThrow("the answer does not fit the offer");
    return { sent, steps: stepsSoFar().map((step) => step.replace(/^\d+ ms /, "")) };
  }

  it("is gathered once more on a new connection, and the second offer sent", async () => {
    peerOffers.push(
      { sdp: sdpWith(hostMdns), errors: [{ url: "turn:relay.example.com:3478", errorCode: 486 }] },
      { sdp: sdpWith(hostMdns, relayedV6) },
    );
    const { sent, steps } = await connectRecording();
    expect(sent).toEqual([sdpWith(hostMdns, relayedV6)]);
    expect(madePeers).toHaveLength(2);
    expect(madePeers[0]?.isClosed).toBe(true);
    expect(steps.slice(0, 2)).toEqual([
      "WebRTC: gathered nothing past this device (gathering errors: turn 486); gathering again",
      "WebRTC: offer sent with 1 host (mDNS), 1 relay IPv6",
    ]);
  });

  it("is sent as it is the second time, never gathered a third", async () => {
    peerOffers.push({ sdp: sdpWith(hostMdns) }, { sdp: sdpWith(hostMdns) }, { sdp: sdpWith() });
    const { sent, steps } = await connectRecording();
    expect(sent).toEqual([sdpWith(hostMdns)]);
    expect(madePeers).toHaveLength(2);
    expect(steps.slice(0, 2)).toEqual([
      "WebRTC: gathered nothing past this device; gathering again",
      "WebRTC: offer sent with 1 host (mDNS), no candidate past this device",
    ]);
  });

  it("is not gathered again when the first offer reaches past this device", async () => {
    peerOffers.push({ sdp: sdpWith(hostMdns, hostV4) }, { sdp: sdpWith(reflexiveV4) });
    const { sent, steps } = await connectRecording();
    expect(sent).toEqual([sdpWith(hostMdns, hostV4)]);
    expect(madePeers).toHaveLength(1);
    expect(steps[0]).toBe("WebRTC: offer sent with 1 host (mDNS), 1 host IPv4");
  });

  it("is gathered again only within what is left of connectTimeoutMs", async () => {
    vi.useFakeTimers();
    const startedAt = performance.now();
    let sentAt: number | undefined;
    const recordSent = (): void => {
      sentAt = performance.now() - startedAt;
    };
    peerOffers.push(
      { sdp: sdpWith(hostMdns), gatherMs: 4_000 },
      { sdp: sdpWith(), gatherMs: Infinity },
    );
    const capped = connectRecording(recordSent);
    await vi.advanceTimersByTimeAsync(connectTimeoutMs - 1);
    expect(sentAt).toBeUndefined();
    await vi.advanceTimersByTimeAsync(1);
    await capped;
    expect(sentAt).toBe(connectTimeoutMs);
    expect(madePeers).toHaveLength(2);
    forgetSteps();
    madePeers.length = 0;
    peerOffers.push({ sdp: sdpWith(hostMdns), gatherMs: Infinity }, { sdp: sdpWith(reflexiveV4) });
    const spent = connectRecording();
    await vi.advanceTimersByTimeAsync(connectTimeoutMs);
    const { steps } = await spent;
    expect(madePeers).toHaveLength(1);
    expect(steps[0]).toBe("WebRTC: offer sent with 1 host (mDNS), no candidate past this device");
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

describe("how a WebRTC transport on a page grows (D482)", () => {
  const open = (): Promise<never> => Promise.reject(new Error("no connection in this test"));
  const page = {
    visibilityState: "hidden" as const,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  };

  it("is paced while its page is hidden, as its app says it needs its connection then", () => {
    const isNeededHidden = (): boolean => true;
    const growth = growthOn(page, open, { count: 3, isNeededHidden });
    expect(growth.size).toBe(3);
    expect(growth.hidden).toEqual({ page, isNeededHidden });
    expect(growth.onOnline).toBeDefined();
  });

  it("is not paced where there is no page", () => {
    expect(growthOn(undefined, open, { count: 1 }).hidden).toBeUndefined();
  });
});
