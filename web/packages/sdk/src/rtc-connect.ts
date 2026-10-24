// SPDX-License-Identifier: MPL-2.0

/**
 * Making the WebRTC transport's connections to a core (D273 §8, plans/webrtc-transport.md): each
 * peer connection's offer, once its candidates are gathered, taken through a signaller to the core and
 * its answer back; slow ones reported. The requests they carry are rtc-transport.ts.
 */

import { reportFailure } from "./problem.ts";
import { clientConnections, peerOf, RtcTransport, type PeerLike } from "./rtc-transport.ts";

/** How long a connection may take to gather its candidates and connect. */
export const connectTimeoutMs = 10_000;

/**
 * How long an offer waits for more candidates after its first that reaches past this machine, a
 * server-reflexive or a relayed one, once it expects no relayed one: those a core away from it can use
 * are then in. Gathering itself completes only when every STUN attempt has answered or given up, and
 * one on a family or interface with no way out retries for some forty seconds: each offer waited out
 * connectTimeoutMs, and a realm sign-in, which makes two, took 28 s on staging (2026-10-23;
 * idiosyncrasies.md). The core's answer waits as long (rtcapi's gatherAfterReflexive).
 */
export const gatherAfterReflexiveMs = 300;

/**
 * How long, with a TURN server among its ICE servers, an offer waits for its relayed candidate after
 * its first server-reflexive one: a relay is allocated a round trip or two after the STUN binding,
 * and a client behind a NAT no punch gets through reaches the core only by it.
 */
export const gatherForRelayMs = 1_000;

/**
 * A connection that takes longer than this to connect is reported to the core's log, with where its
 * time went: a sign-in that waited half a minute on its connections said so nowhere (staging,
 * 2026-10-23).
 */
export const slowConnectMs = 3_000;

/** How long each step of making a connection took, in milliseconds. */
export interface ConnectTimings {
  readonly gathering: number;
  readonly answer: number;
  readonly connecting: number;
}

/** What to report of a connection that took over slowConnectMs, or undefined for one that did not. */
export function slowConnection(timings: ConnectTimings): string | undefined {
  const total = timings.gathering + timings.answer + timings.connecting;
  if (total <= slowConnectMs) {
    return undefined;
  }
  const ms = (value: number): string => `${String(Math.round(value))} ms`;
  return (
    `a WebRTC connection took ${ms(total)}: gathering its candidates ${ms(timings.gathering)}, ` +
    `the core's answer ${ms(timings.answer)}, connecting ${ms(timings.connecting)}`
  );
}

/** A session description: an offer or an answer, and its SDP. */
export interface Description {
  readonly type: "offer" | "answer";
  readonly sdp: string;
}

/** Takes a client's offer to the core and returns its answer: the offer endpoint, or a discovery. */
export type Signaller = (offer: Description) => Promise<Description>;

/**
 * Connects to the core through signaller: resolves once one peer connection is up, each with every
 * candidate gathered before its offer is sent, and opens the rest up to `count` behind it, replacing
 * any that is gone (plans/webrtc-transport.md W2d).
 */
export async function connectRtc(
  signaller: Signaller,
  rtcConfig: RTCConfiguration = {},
  count: number = clientConnections,
): Promise<RtcTransport> {
  const open = (): Promise<PeerLike> => connectOne(signaller, rtcConfig);
  return new RtcTransport([await open()], { open, size: count });
}

/** One peer connection, connected through signaller. */
async function connectOne(signaller: Signaller, rtcConfig: RTCConfiguration): Promise<PeerLike> {
  const peer = new RTCPeerConnection(rtcConfig);
  try {
    // An offer needs something to negotiate: a channel opened and never used, which the core knows by
    // its label and never serves (internal/core/rtcapi.NegotiationLabel).
    peer.createDataChannel("hello");
    const startedAt = performance.now();
    await peer.setLocalDescription(await peer.createOffer());
    await gathered(peer, hasRelayServer(rtcConfig));
    const gatheredAt = performance.now();
    const local = peer.localDescription;
    if (local === null) {
      throw new Error("the connection made no offer");
    }
    const answer = await signaller({ type: "offer", sdp: local.sdp });
    const answeredAt = performance.now();
    await peer.setRemoteDescription(answer);
    await connected(peer);
    const slow = slowConnection({
      gathering: gatheredAt - startedAt,
      answer: answeredAt - gatheredAt,
      connecting: performance.now() - answeredAt,
    });
    if (slow !== undefined) {
      reportFailure(new Error(slow));
    }
    return peerOf(peer);
  } catch (error: unknown) {
    peer.close();
    throw error;
  }
}

/** Whether a configuration names a TURN server, through which a relayed candidate will come. */
export function hasRelayServer(rtcConfig: RTCConfiguration): boolean {
  return (rtcConfig.iceServers ?? []).some((server) =>
    (Array.isArray(server.urls) ? server.urls : [server.urls]).some((url) => /^turns?:/i.test(url)),
  );
}

/** What gathered needs of a peer connection: the browser's, or a test's. */
export interface GatheringPeer {
  readonly iceGatheringState: RTCIceGatheringState;
  addEventListener(type: "icegatheringstatechange", listener: () => void): void;
  addEventListener(
    type: "icecandidate",
    listener: (event: RTCPeerConnectionIceEvent) => void,
  ): void;
}

/**
 * Resolves once peer has gathered its candidates; gatherAfterReflexiveMs after its first
 * server-reflexive or relayed one, but with a TURN server (willRelay) gatherForRelayMs after its
 * first server-reflexive one unless a relayed one comes first; or after connectTimeoutMs with those it
 * has: whichever is first.
 */
export function gathered(peer: GatheringPeer, willRelay = false): Promise<void> {
  return new Promise((resolve) => {
    if (peer.iceGatheringState === "complete") {
      resolve();
      return;
    }
    const timers = [setTimeout(done, connectTimeoutMs)];
    function done(): void {
      timers.forEach(clearTimeout);
      resolve();
    }
    peer.addEventListener("icegatheringstatechange", () => {
      if (peer.iceGatheringState === "complete") {
        done();
      }
    });
    let hasReflexive = false;
    let hasRelayed = false;
    peer.addEventListener("icecandidate", (event) => {
      const type = event.candidate?.type;
      if (type === "srflx" && !hasReflexive) {
        hasReflexive = true;
        timers.push(setTimeout(done, willRelay ? gatherForRelayMs : gatherAfterReflexiveMs));
      } else if (type === "relay" && !hasRelayed) {
        hasRelayed = true;
        timers.push(setTimeout(done, gatherAfterReflexiveMs));
      }
    });
  });
}

/** Resolves once peer is connected; rejects when it fails or takes over connectTimeoutMs. */
function connected(peer: RTCPeerConnection): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error("the WebRTC connection did not connect in time"));
    }, connectTimeoutMs);
    const settle = (): void => {
      if (peer.connectionState === "connected") {
        clearTimeout(timer);
        resolve();
      } else if (peer.connectionState === "failed" || peer.connectionState === "closed") {
        clearTimeout(timer);
        reject(new Error(`the WebRTC connection ${peer.connectionState}`));
      }
    };
    peer.addEventListener("connectionstatechange", settle);
    settle();
  });
}
