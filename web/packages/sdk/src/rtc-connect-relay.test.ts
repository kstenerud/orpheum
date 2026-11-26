// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { withoutRelayServers, withUdpRelaysAlone } from "./rtc-candidates.ts";
import { connectRtc, RtcUnconnected, type Signaller } from "./rtc-connect.ts";
import { forgetRelayNeeded, relayAfterMs } from "./rtc-ladder.ts";
import { RtcTransport } from "./rtc-transport.ts";
import { forgetSteps, stepsSoFar } from "./steps.ts";

/** An offer's SDP holding one server-reflexive candidate, at a documentation address. */
const reflexiveSdp = [
  "v=0",
  "m=application 9 UDP/DTLS/SCTP webrtc-datachannel",
  "a=candidate:3 1 UDP 1685921791 203.0.113.7 40000 typ srflx raddr 0.0.0.0 rport 0",
  "",
].join("\r\n");

/**
 * A browser's peer connection made on the servers it was given, which gathers at once, takes the
 * core's answer, and connects or fails as the test says; once connected, the core holds it.
 */
const racingPeers: RacingPeer[] = [];

class RacingPeer {
  readonly iceGatheringState = "complete";
  connectionState: RTCPeerConnectionState = "new";
  iceConnectionState: RTCIceConnectionState = "new";
  readonly localDescription = { type: "offer", sdp: reflexiveSdp };
  readonly channel = Object.assign(new EventTarget(), { readyState: "connecting" });
  isClosed = false;
  readonly #onState: (() => void)[] = [];
  constructor(readonly config: RTCConfiguration) {
    racingPeers.push(this);
  }
  createDataChannel(): EventTarget & { readyState: string } {
    return this.channel;
  }
  addTransceiver(): void {
    // The video m-line is offered; nothing comes of it here.
  }
  addEventListener(type: string, listener: () => void): void {
    if (type === "connectionstatechange") {
      this.#onState.push(listener);
    }
  }
  createOffer(): Promise<{ type: string; sdp: string }> {
    return Promise.resolve(this.localDescription);
  }
  setLocalDescription(): Promise<void> {
    return Promise.resolve();
  }
  setRemoteDescription(): Promise<void> {
    return Promise.resolve();
  }
  getStats(): Promise<RTCStatsReport> {
    return Promise.resolve(new Map() as unknown as RTCStatsReport);
  }
  close(): void {
    this.isClosed = true;
  }
  connects(): void {
    this.#moves("connected");
    this.channel.readyState = "open";
    this.channel.dispatchEvent(new Event("open"));
    this.channel.dispatchEvent(
      new MessageEvent("message", { data: JSON.stringify({ isHeld: true }) }),
    );
  }
  fails(ice: RTCIceConnectionState = "failed"): void {
    this.iceConnectionState = ice;
    this.#moves("failed");
  }
  #moves(connection: RTCPeerConnectionState): void {
    this.connectionState = connection;
    this.#onState.forEach((listener) => {
      listener();
    });
  }
}

/** A STUN server and two TURN ones, with credentials; and the same with its TURN servers taken out. */
const stunAndTurn: RTCConfiguration = {
  iceServers: [
    {
      urls: ["stun:stun.example.com:3478", "turn:relay.example.com:3478"],
      username: "user",
      credential: "secret",
    },
    { urls: "turns:relay.example.com:5349", username: "user", credential: "secret" },
  ],
};
const stunOnly: RTCConfiguration = {
  iceServers: [{ urls: ["stun:stun.example.com:3478"], username: "user", credential: "secret" }],
};
/** stunAndTurn with its UDP relay alone: the TLS one taken out (D561 §2). */
const stunAndUdpTurn: RTCConfiguration = {
  iceServers: [
    {
      urls: ["stun:stun.example.com:3478", "turn:relay.example.com:3478"],
      username: "user",
      credential: "secret",
    },
  ],
};
/** A STUN server and a relay named by its TLS URL alone, with no UDP one. */
const stunAndTlsTurn: RTCConfiguration = {
  iceServers: [
    { urls: "stun:stun.example.com:3478" },
    { urls: "turns:relay.example.com:443?transport=tcp", username: "user", credential: "secret" },
  ],
};

describe("a connection asks the relays only once it has not connected without them (D559 §1)", () => {
  afterEach(() => {
    forgetSteps();
    forgetRelayNeeded();
    vi.unstubAllGlobals();
    vi.useRealTimers();
    racingPeers.length = 0;
  });

  /**
   * Starts connecting on config through signaller, a core that answers at once where not given: its
   * connecting settles as connectRtc does, with the transport or the failure.
   */
  async function startConnecting(
    config: RTCConfiguration,
    signaller: Signaller = () => Promise.resolve({ type: "answer", sdp: "v=0" }),
  ): Promise<{ connecting: Promise<unknown> }> {
    vi.useFakeTimers();
    vi.stubGlobal("RTCPeerConnection", RacingPeer);
    const connecting = connectRtc(signaller, config, { count: 1 }).catch(
      (problem: unknown) => problem,
    );
    await vi.advanceTimersByTimeAsync(0);
    return { connecting };
  }

  /** The steps so far, without their times. */
  function steps(): string[] {
    return stepsSoFar().map((step) => step.replace(/^\d+ ms /, ""));
  }

  it("tries first with every TURN server taken out", async () => {
    expect(withoutRelayServers(stunAndTurn)).toEqual(stunOnly);
    await startConnecting(stunAndTurn);
    expect(racingPeers).toHaveLength(1);
    expect(racingPeers[0]?.config).toEqual(stunOnly);
  });

  it("makes no second connection when the first connects within relayAfterMs", async () => {
    const { connecting } = await startConnecting(stunAndTurn);
    await vi.advanceTimersByTimeAsync(relayAfterMs - 1);
    racingPeers[0]?.connects();
    expect(await connecting).toBeInstanceOf(RtcTransport);
    await vi.advanceTimersByTimeAsync(relayAfterMs * 3);
    expect(racingPeers).toHaveLength(1);
    expect(racingPeers[0]?.isClosed).toBe(false);
  });

  it("asks the relays beside it once not connected relayAfterMs after the answer, the first up taken", async () => {
    const { connecting } = await startConnecting(stunAndTurn);
    await vi.advanceTimersByTimeAsync(relayAfterMs - 1);
    expect(racingPeers).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(racingPeers).toHaveLength(2);
    expect(racingPeers[1]?.config).toEqual(stunAndUdpTurn);
    racingPeers[1]?.connects();
    expect(await connecting).toBeInstanceOf(RtcTransport);
    expect(racingPeers.map((peer) => peer.isClosed)).toEqual([true, false]);
    expect(steps()).toContain(
      "WebRTC: not connected 3 s after the answer; trying with the relay over UDP too",
    );
    expect(steps()).toContain(
      "WebRTC: connected through the attempt that asked the relay over UDP",
    );
    expect(steps().filter((step) => step.startsWith("WebRTC: failed"))).toEqual([]);
  });

  it("takes the connection without the relays when it connects first, and closes the other", async () => {
    const { connecting } = await startConnecting(stunAndTurn);
    await vi.advanceTimersByTimeAsync(relayAfterMs);
    expect(racingPeers).toHaveLength(2);
    racingPeers[0]?.connects();
    expect(await connecting).toBeInstanceOf(RtcTransport);
    expect(racingPeers.map((peer) => peer.isClosed)).toEqual([false, true]);
    expect(steps()).toContain("WebRTC: connected through the attempt without the relay");
  });

  it("asks the relays at once, for the page's life, once a connection needed them", async () => {
    const { connecting: first } = await startConnecting(stunAndTurn);
    await vi.advanceTimersByTimeAsync(relayAfterMs);
    racingPeers[1]?.connects();
    await first;
    const { connecting: next } = await startConnecting(stunAndTurn);
    expect(racingPeers).toHaveLength(3);
    expect(racingPeers[2]?.config).toEqual(stunAndUdpTurn);
    racingPeers[2]?.connects();
    expect(await next).toBeInstanceOf(RtcTransport);
    await vi.advanceTimersByTimeAsync(relayAfterMs * 3);
    expect(racingPeers).toHaveLength(3);
    expect(steps()).toContain(
      "WebRTC: this page's network needed the relay over UDP; asking it at once",
    );
  });

  it("never makes a second connection where no TURN server is named", async () => {
    const { connecting } = await startConnecting(stunOnly);
    await vi.advanceTimersByTimeAsync(relayAfterMs * 3);
    racingPeers[0]?.fails();
    expect(await connecting).toBeInstanceOf(RtcUnconnected);
    await vi.advanceTimersByTimeAsync(relayAfterMs * 3);
    expect(racingPeers).toHaveLength(1);
  });

  it("asks the relays at once when the first fails after the answer, and tells their failure when both fail", async () => {
    const { connecting } = await startConnecting(stunAndTurn);
    racingPeers[0]?.fails();
    await vi.advanceTimersByTimeAsync(0);
    expect(racingPeers).toHaveLength(2);
    expect(racingPeers[1]?.config).toEqual(stunAndUdpTurn);
    expect(steps()).toContain(
      "WebRTC: failed without the relay; trying with the relay over UDP too",
    );
    racingPeers[1]?.fails();
    await vi.advanceTimersByTimeAsync(0);
    expect(racingPeers).toHaveLength(3);
    expect(racingPeers[2]?.config).toEqual(stunAndTurn);
    expect(steps()).toContain(
      "WebRTC: failed with the relay over UDP; trying with the relay's every address too",
    );
    racingPeers[2]?.fails("checking");
    const problem = await connecting;
    expect(problem instanceof Error ? problem.message : "").toBe(
      "the WebRTC connection failed (ICE checking, connection failed, no pair selected)",
    );
  });

  it("does not ask the relays when its offer reached no core", async () => {
    const { connecting } = await startConnecting(stunAndTurn, () =>
      Promise.reject(new Error("the offer reached no server")),
    );
    await vi.advanceTimersByTimeAsync(relayAfterMs * 3);
    const problem = await connecting;
    expect(problem instanceof Error ? problem.message : "").toBe("the offer reached no server");
    expect(racingPeers).toHaveLength(1);
  });
  it("asks a relay's UDP URLs alone first, keeping its STUN ones (D561 §2)", () => {
    const hosted: RTCConfiguration = {
      iceServers: [
        { urls: "stun:stun.example.com:3478" },
        {
          urls: [
            "turn:relay.example.com:3478?transport=udp",
            "turns:relay.example.com:443?transport=tcp",
            "turn:relay.example.com:3478?transport=tcp",
          ],
          username: "user",
          credential: "secret",
        },
      ],
    };
    expect(withUdpRelaysAlone(hosted)).toEqual({
      iceServers: [
        { urls: ["stun:stun.example.com:3478"] },
        {
          urls: ["turn:relay.example.com:3478?transport=udp"],
          username: "user",
          credential: "secret",
        },
      ],
    });
    expect(withUdpRelaysAlone(stunAndTurn)).toEqual(stunAndUdpTurn);
  });

  it("asks the relay's every address only once the UDP one has not connected relayAfterMs after it started (D561 §2)", async () => {
    const { connecting } = await startConnecting(stunAndTurn);
    await vi.advanceTimersByTimeAsync(relayAfterMs);
    expect(racingPeers).toHaveLength(2);
    expect(racingPeers[1]?.config).toEqual(stunAndUdpTurn);
    await vi.advanceTimersByTimeAsync(relayAfterMs - 1);
    racingPeers[1]?.connects();
    expect(await connecting).toBeInstanceOf(RtcTransport);
    await vi.advanceTimersByTimeAsync(relayAfterMs * 3);
    expect(racingPeers).toHaveLength(2);
  });

  it("asks the relay's every address relayAfterMs after the UDP attempt started, though the core never answered it (D561 §2)", async () => {
    let offers = 0;
    const { connecting } = await startConnecting(stunAndTurn, (_offer, signal) => {
      offers += 1;
      return offers !== 2
        ? Promise.resolve({ type: "answer", sdp: "v=0" })
        : new Promise((_, reject) => {
            signal?.addEventListener("abort", () => {
              reject(new Error("given up"));
            });
          });
    });
    await vi.advanceTimersByTimeAsync(relayAfterMs);
    expect(racingPeers).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(relayAfterMs - 1);
    expect(racingPeers).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(racingPeers).toHaveLength(3);
    expect(racingPeers[2]?.config).toEqual(stunAndTurn);
    expect(steps()).toContain(
      "WebRTC: not connected 3 s after trying with the relay over UDP; trying with the relay's every address too",
    );
    racingPeers[2]?.connects();
    expect(await connecting).toBeInstanceOf(RtcTransport);
    expect(racingPeers.map((peer) => peer.isClosed)).toEqual([true, true, false]);
    expect(steps()).toContain(
      "WebRTC: connected through the attempt that asked the relay's every address",
    );
  });

  it("starts a page's later connections at the relay's every address once one needed it, with no rung after (D561 §2)", async () => {
    const { connecting: first } = await startConnecting(stunAndTurn);
    await vi.advanceTimersByTimeAsync(relayAfterMs * 2);
    expect(racingPeers).toHaveLength(3);
    racingPeers[2]?.connects();
    await first;
    const { connecting: next } = await startConnecting(stunAndTurn);
    expect(racingPeers).toHaveLength(4);
    expect(racingPeers[3]?.config).toEqual(stunAndTurn);
    expect(steps()).toContain(
      "WebRTC: this page's network needed the relay's every address; asking it at once",
    );
    await vi.advanceTimersByTimeAsync(relayAfterMs * 3);
    expect(racingPeers).toHaveLength(4);
    racingPeers[3]?.connects();
    expect(await next).toBeInstanceOf(RtcTransport);
  });

  it("goes straight to the relay's every address where it names no UDP one (D561 §2)", async () => {
    const { connecting } = await startConnecting(stunAndTlsTurn);
    expect(racingPeers[0]?.config).toEqual({
      iceServers: [{ urls: ["stun:stun.example.com:3478"] }],
    });
    await vi.advanceTimersByTimeAsync(relayAfterMs);
    expect(racingPeers).toHaveLength(2);
    expect(racingPeers[1]?.config).toEqual(stunAndTlsTurn);
    expect(steps()).toContain(
      "WebRTC: not connected 3 s after the answer; trying with the relay's every address too",
    );
    racingPeers[1]?.connects();
    expect(await connecting).toBeInstanceOf(RtcTransport);
  });

  it("asks no more than the UDP relay where the relay names no other address, and tells its failure (D561 §2)", async () => {
    const { connecting } = await startConnecting(stunAndUdpTurn);
    await vi.advanceTimersByTimeAsync(relayAfterMs);
    expect(racingPeers).toHaveLength(2);
    expect(racingPeers[1]?.config).toEqual(stunAndUdpTurn);
    await vi.advanceTimersByTimeAsync(relayAfterMs * 3);
    expect(racingPeers).toHaveLength(2);
    racingPeers[0]?.fails();
    racingPeers[1]?.fails("checking");
    await vi.advanceTimersByTimeAsync(0);
    expect(racingPeers).toHaveLength(2);
    const problem = await connecting;
    expect(problem instanceof Error ? problem.message : "").toBe(
      "the WebRTC connection failed (ICE checking, connection failed, no pair selected)",
    );
  });
});
