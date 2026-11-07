// SPDX-License-Identifier: MPL-2.0

/**
 * Making the WebRTC transport's connections to a core (D273 §8, plans/webrtc-transport.md): each
 * peer connection's offer, once its candidates are gathered, taken through a signaller to the core and
 * its answer back; slow ones reported. The requests they carry are rtc-transport.ts.
 */

import { reportFailure } from "./problem.ts";
import { channelOf, type ChannelLike } from "./rtc-request.ts";
import { candidatePairsOf } from "./rtc-pairs.ts";
import { peerOf } from "./rtc-peer.ts";
import { clientConnections, RtcTransport, type PeerLike } from "./rtc-transport.ts";
import { noteStep } from "./steps.ts";

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

/**
 * How long, once its negotiation channel is open, a connection waits for the core's verdict: the
 * core sends it as the channel opens, so it comes within a round trip; a core from before verdicts
 * sends none, and its connection is taken as held when this is over.
 */
export const verdictWaitMs = 1_000;

/**
 * A connection the core turned away, saying why (rtcapi.Verdict): `network-full` when the client's
 * network holds its share of the core's connections already (rtcapi's perSourceLimit), which a page
 * says as too many Orpheum pages open on the network, not as a network that cannot reach the server
 * (the owner's Fire TV, 2026-10-25).
 */
export class TurnedAway extends Error {
  constructor(readonly reason: string) {
    super(`the core turned the WebRTC connection away: ${reason}`);
    this.name = "TurnedAway";
  }
}

/** The core's reason for turning a connection away for its network's share: rtcapi.RefusedNetworkFull. */
export const networkFull = "network-full";

/** Whether problem, or any cause behind it, is the core turning a connection away for reason. */
export function isTurnedAway(problem: unknown, reason: string = networkFull): boolean {
  for (let each = problem, depth = 0; each instanceof Error && depth < 8; depth += 1) {
    if (each instanceof TurnedAway && each.reason === reason) {
      return true;
    }
    each = each.cause;
  }
  return false;
}

/**
 * Settles with the core's verdict on a connection, told on its negotiation channel (rtcapi.Verdict):
 * resolves when it is held, or when verdictWaitMs pass after the channel opens with none said, as an
 * older core says none; rejects with TurnedAway when the core turned it away, and with an error when
 * the channel closes first.
 */
export function verdictOf(channel: ChannelLike): Promise<void> {
  return new Promise((resolve, reject) => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    const settle = (failure?: Error): void => {
      clearTimeout(timer);
      if (failure === undefined) {
        resolve();
      } else {
        reject(failure);
      }
    };
    const waitAfterOpen = (): void => {
      timer ??= setTimeout(settle, verdictWaitMs);
    };
    channel.listen({
      open: waitAfterOpen,
      message: (data) => {
        const refused = typeof data === "string" ? refusalIn(data) : undefined;
        settle(refused === undefined ? undefined : new TurnedAway(refused));
      },
      close: () => {
        settle(new Error("the core closed the WebRTC connection before saying it was held"));
      },
      error: () => {
        settle(new Error("the WebRTC connection's negotiation channel failed"));
      },
    });
    if (channel.readyState === "open") {
      waitAfterOpen();
    }
  });
}

/** The reason a verdict's text turns the connection away for, or undefined for one that does not. */
function refusalIn(text: string): string | undefined {
  try {
    const parsed: unknown = JSON.parse(text);
    const refused: unknown =
      typeof parsed === "object" && parsed !== null ? Reflect.get(parsed, "refused") : undefined;
    return typeof refused === "string" && refused !== "" ? refused : undefined;
  } catch {
    return undefined;
  }
}

/** A session description: an offer or an answer, and its SDP. */
export interface Description {
  readonly type: "offer" | "answer";
  readonly sdp: string;
}

/**
 * Takes a client's offer to the core and returns its answer: the offer endpoint, or a discovery. Its
 * post is given up once signal aborts, as the transport gives up a try that hangs (openTryMostMs).
 */
export type Signaller = (offer: Description, signal?: AbortSignal) => Promise<Description>;

/** Tells listener each time the browser comes back online, where there is a window; returns the end. */
function onBrowserOnline(listener: () => void): () => void {
  if (typeof globalThis.addEventListener !== "function") {
    return () => undefined;
  }
  globalThis.addEventListener("online", listener);
  return () => {
    globalThis.removeEventListener("online", listener);
  };
}

/**
 * Connects to the core through signaller: resolves once one peer connection is up, each with every
 * candidate gathered before its offer is sent, and opens the rest up to `count` behind it, replacing
 * any that is gone, and trying at once when the browser comes back online (plans/webrtc-transport.md
 * W2d).
 */
export async function connectRtc(
  signaller: Signaller,
  rtcConfig: RTCConfiguration = {},
  count: number = clientConnections,
): Promise<RtcTransport> {
  const open = (signal: AbortSignal): Promise<PeerLike> => connectOne(signaller, rtcConfig, signal);
  return new RtcTransport([await open(new AbortController().signal)], {
    open,
    size: count,
    onOnline: onBrowserOnline,
  });
}

/** What offerWith asks of a peer connection: RTCPeerConnection's. */
export interface OfferingPeer<Channel> {
  createDataChannel(label: string): Channel;
  addTransceiver(kind: "video", init: { direction: "recvonly" }): unknown;
}

/**
 * Puts on peer what its offer negotiates, and returns its negotiation channel: a channel the client
 * sends nothing on, which the core knows by its label, never serves, and tells its verdict on
 * (internal/core/rtcapi.NegotiationLabel); and a video m-line that receives, which the core answers
 * and never sends on. Chrome grows a connection's socket receive buffer from 64 KiB only when it
 * has a video m-line (kVideoRtpRecvBufferSize), and without it drops the core's packets before its
 * data channels read them (docs/contributors/idiosyncrasies.md, D397).
 */
export function offerWith<Channel>(peer: OfferingPeer<Channel>): Channel {
  const channel = peer.createDataChannel("hello");
  peer.addTransceiver("video", { direction: "recvonly" });
  return channel;
}

/** One peer connection, connected through signaller; given up, its peer closed, once signal aborts. */
async function connectOne(
  signaller: Signaller,
  rtcConfig: RTCConfiguration,
  signal: AbortSignal,
): Promise<PeerLike> {
  const peer = new RTCPeerConnection(rtcConfig);
  const giveUp = (): void => {
    peer.close();
  };
  signal.addEventListener("abort", giveUp, { once: true });
  try {
    const verdict = verdictOf(channelOf(offerWith(peer)));
    verdict.catch(() => undefined);
    const startedAt = performance.now();
    await peer.setLocalDescription(await peer.createOffer());
    await gathered(peer, hasRelayServer(rtcConfig));
    const gatheredAt = performance.now();
    const local = peer.localDescription;
    if (local === null) {
      throw new Error("the connection made no offer");
    }
    noteStep("WebRTC: offer sent");
    const answer = await signaller({ type: "offer", sdp: local.sdp }, signal);
    const answeredAt = performance.now();
    noteStep("WebRTC: the core answered");
    await peer.setRemoteDescription(answer);
    await connected(peer);
    noteStep("WebRTC: connected");
    await withinConnectTimeout(verdict);
    noteStep("WebRTC: the core held the connection");
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
    noteStep(`WebRTC: failed: ${error instanceof Error ? error.message : String(error)}`);
    peer.close();
    throw error;
  } finally {
    signal.removeEventListener("abort", giveUp);
  }
}

/** Settles as settling does, or rejects once connectTimeoutMs pass first. */
async function withinConnectTimeout(settling: Promise<void>): Promise<void> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const late = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      reject(new Error("the core said nothing of the WebRTC connection in time"));
    }, connectTimeoutMs);
  });
  try {
    await Promise.race([settling, late]);
  } finally {
    clearTimeout(timer);
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

/**
 * How far a WebRTC connection that never connected got, as the browser tells it: its ICE state, its
 * connection's state, which the DTLS handshake moves to connected once ICE has a pair, and the kinds
 * of the pair ICE selected ("host to host (udp)"), when it selected one. No address: what happened
 * carries it to whoever runs the server (the owner's Firefox 158, 2026-11-07, whose ICE checks
 * succeeded and whose DTLS never began, said only that it did not connect in time; D441).
 */
export interface RtcStage {
  readonly ice: string;
  readonly connection: string;
  readonly pair?: string | undefined;
}

/** A WebRTC connection that failed or timed out before it connected, with how far it got. */
export class RtcUnconnected extends Error {
  constructor(
    readonly stage: RtcStage,
    why: string,
  ) {
    super(`the WebRTC connection ${why} (${stageText(stage)})`);
    this.name = "RtcUnconnected";
  }
}

/** A stage in words: "ICE connected, connection connecting, pair host to host (udp)". */
function stageText(stage: RtcStage): string {
  const pair = stage.pair === undefined ? "no pair selected" : `pair ${stage.pair}`;
  return `ICE ${stage.ice}, connection ${stage.connection}, ${pair}`;
}

/**
 * Whether problem, or a cause or an alternative behind it, is a WebRTC connection whose ICE found a
 * way to the core and whose secure connection over it (DTLS) never completed: its ICE connected or
 * completed, or a pair selected, and the connection never connected. The network reached the core
 * there, and the page says so, not that this network cannot reach it (D398 §3).
 */
export function isSecureUnfinished(problem: unknown, depth = 0): boolean {
  if (!(problem instanceof Error) || depth > 8) {
    return false;
  }
  if (problem instanceof RtcUnconnected) {
    const { ice, connection, pair } = problem.stage;
    const isIceThrough = ice === "connected" || ice === "completed" || pair !== undefined;
    return isIceThrough && connection !== "connected";
  }
  const alternatives: readonly unknown[] =
    problem instanceof AggregateError ? Array.from<unknown>(problem.errors) : [];
  return [problem.cause, ...alternatives].some((each) => isSecureUnfinished(each, depth + 1));
}

/** What connected needs of a peer connection: the browser's, or a test's. */
export interface ConnectingPeer {
  readonly connectionState: RTCPeerConnectionState;
  readonly iceConnectionState: RTCIceConnectionState;
  addEventListener(type: "connectionstatechange", listener: () => void): void;
  getStats(): Promise<RTCStatsReport>;
}

/**
 * How long the stage of a connection that did not connect waits for the browser's stats before it
 * is told without its pair: they come within a few milliseconds, and the page must not hang on them.
 */
const stageStatsWaitMs = 500;

/** The kinds of the pair peer's ICE selected, or undefined where it selected none or says nothing. */
async function selectedPairOf(peer: ConnectingPeer): Promise<string | undefined> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const late = new Promise<undefined>((resolve) => {
    timer = setTimeout(resolve, stageStatsWaitMs);
  });
  try {
    const stats = await Promise.race([peer.getStats(), late]);
    const selected = candidatePairsOf(stats?.values() ?? []).find((pair) => pair.isSelected);
    if (selected === undefined) {
      return undefined;
    }
    const [browserType = "unknown", protocol = "unknown"] = selected.browser.split(" ");
    const [coreType = "unknown"] = selected.core.split(" ");
    return `${browserType} to ${coreType} (${protocol})`;
  } catch {
    return undefined;
  } finally {
    clearTimeout(timer);
  }
}

/** The stage peer reached, failing or timing out for why, as an RtcUnconnected. */
async function unconnectedOf(peer: ConnectingPeer, why: string): Promise<RtcUnconnected> {
  const ice = peer.iceConnectionState;
  const connection = peer.connectionState;
  return new RtcUnconnected({ ice, connection, pair: await selectedPairOf(peer) }, why);
}

/**
 * Resolves once peer is connected; rejects with how far it got (RtcUnconnected) when it fails or
 * takes over connectTimeoutMs.
 */
export function connected(peer: ConnectingPeer): Promise<void> {
  return new Promise((resolve, reject) => {
    let isSettled = false;
    const fail = (why: string): void => {
      isSettled = true;
      clearTimeout(timer);
      void unconnectedOf(peer, why).then(reject);
    };
    const timer = setTimeout(() => {
      fail("did not connect in time");
    }, connectTimeoutMs);
    const settle = (): void => {
      if (isSettled) {
        return;
      }
      if (peer.connectionState === "connected") {
        isSettled = true;
        clearTimeout(timer);
        resolve();
      } else if (peer.connectionState === "failed" || peer.connectionState === "closed") {
        fail(peer.connectionState);
      }
    };
    peer.addEventListener("connectionstatechange", settle);
    settle();
  });
}
