// SPDX-License-Identifier: MPL-2.0

/**
 * Making the WebRTC transport's connections to a core (D273 §8, plans/webrtc-transport.md): each
 * peer connection's offer, once its candidates are gathered, taken through a signaller to the core and
 * its answer back; slow ones reported. The requests they carry are rtc-transport.ts.
 */

import { reportFailure } from "./problem.ts";
import { channelOf, type ChannelLike } from "./rtc-request.ts";
import {
  errorsText,
  gatheringErrorsOf,
  gatheringText,
  hasRelayServer,
  isPastThisDevice,
} from "./rtc-candidates.ts";
import { connectingStepsOf, slowConnection } from "./rtc-connect-timing.ts";
import { RelayCredentials, type IceServersAgain } from "./rtc-relay-credentials.ts";
import { climbLadder, lostToAnotherAttempt, type AttemptHooks } from "./rtc-ladder.ts";
import { candidatePairsOf } from "./rtc-pairs.ts";
import { peerOf } from "./rtc-peer.ts";
import {
  clientConnections,
  RtcTransport,
  type Growth,
  type PeerLike,
  type WayBack,
} from "./rtc-transport.ts";
import { whenShownOrBack, type ShownPage } from "./shown-page.ts";
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
 * How long, once its negotiation channel is open, a connection waits for the core's verdict: the
 * core sends it as the channel opens, so it comes within a round trip; a core from before verdicts
 * sends none, and its connection is taken as held when this is over.
 */
export const verdictWaitMs = 1_000;

/**
 * A connection the core turned away, saying why (rtcapi.Verdict): `networkFull` when the client's
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
export const networkFull = "networkFull";

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
 * the channel closes first. Calls opened as the channel opens, for the time connecting took (D484 §1).
 */
export function verdictOf(channel: ChannelLike, opened?: () => void): Promise<void> {
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
      if (timer === undefined) {
        opened?.();
      }
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
 * How long after a network change event later ones count as the same change: Chrome fires several
 * together as a phone moves from Wi-Fi to mobile data, and they make one check (onNetworkChange).
 */
export const networkChangeBurstMs = 2000;

/** What onNetworkChange needs of the browser's network information: its change event. */
export interface ChangingNetwork {
  addEventListener(type: "change", listener: () => void): void;
  removeEventListener(type: "change", listener: () => void): void;
}

/**
 * The browser's network information (navigator.connection), where it has one: Chrome on Android
 * does; Firefox and Safari do not.
 */
function browserNetwork(): ChangingNetwork | undefined {
  if (typeof navigator === "undefined") {
    return undefined;
  }
  const connection: unknown = Reflect.get(navigator, "connection");
  return isChangingNetwork(connection) ? connection : undefined;
}

/** Whether value has a change event to hear, as navigator.connection does. */
function isChangingNetwork(value: unknown): value is ChangingNetwork {
  return (
    typeof value === "object" &&
    value !== null &&
    "addEventListener" in value &&
    typeof value.addEventListener === "function" &&
    "removeEventListener" in value &&
    typeof value.removeEventListener === "function"
  );
}

/**
 * Tells listener once for each burst of network's change events: at the first, and not again until
 * none came for networkChangeBurstMs. Nothing is heard where there is no network. Returns the end.
 */
export function onNetworkChange(
  network: ChangingNetwork | undefined,
  listener: () => void,
): () => void {
  if (network === undefined) {
    return () => undefined;
  }
  let burst: ReturnType<typeof setTimeout> | undefined;
  const changed = (): void => {
    if (burst === undefined) {
      listener();
    }
    clearTimeout(burst);
    burst = setTimeout(() => {
      burst = undefined;
    }, networkChangeBurstMs);
  };
  network.addEventListener("change", changed);
  return () => {
    clearTimeout(burst);
    network.removeEventListener("change", changed);
  };
}

/**
 * Has listener told each time a connection may open again where it could not, and why: the browser
 * came back online ("back"), or page is shown again ("shown"), where there is one; and once for each
 * burst of the browser's network change events ("changed", onNetworkChange), where network has them,
 * the connections up then judged at once (plan item 10, option D). A phone hands a
 * page back from the background with a try to connect begun before it froze, hung on a network it no
 * longer has (the owner's phone back from another app, staging, 2026-11-14; idiosyncrasies.md, a page
 * frozen in the background); the transport gives such a try up and makes another at once once it has
 * run past openTryMostMs, and keeps a younger one a mere show finds under way (D484 §2). Returns the end.
 */
export function onWayBack(
  page: ShownPage | undefined,
  network: ChangingNetwork | undefined = browserNetwork(),
): (listener: (why: WayBack) => void) => () => void {
  return (listener) => {
    const stopShownOrBack = whenShownOrBack(page, onBrowserOnline, (why) => {
      listener(why);
    });
    const stopChanged = onNetworkChange(network, () => {
      listener("changed");
    });
    return () => {
      stopShownOrBack();
      stopChanged();
    };
  };
}

/**
 * Connects to the core through signaller: resolves once one peer connection is up, each with every
 * candidate gathered before its offer is sent, and opens the rest up to `count` behind it, replacing
 * any that is gone, and trying at once when the browser comes back online or the page is shown again
 * (plans/webrtc-transport.md W2d, onWayBack). While the page is hidden its tries are paced (D482)
 * unless isNeededHidden, which its app gives, says it needs its connection as when shown: a film
 * playing in it, or a cast's remote shown.
 */
export async function connectRtc(
  signaller: Signaller,
  rtcConfig: RTCConfiguration = {},
  options: RtcConnectOptions = {},
): Promise<RtcTransport> {
  const credentials = new RelayCredentials(rtcConfig, options.iceServersAgain, options.nowMs);
  const open = (signal: AbortSignal): Promise<PeerLike> =>
    connectOne(signaller, credentials, signal);
  const page = typeof document === "undefined" ? undefined : document;
  return new RtcTransport(
    [await open(new AbortController().signal)],
    growthOn(page, open, options),
  );
}

/**
 * How a WebRTC transport grows: to count connections (clientConnections where absent), its tries
 * paced while its page is hidden unless isNeededHidden says it needs its connection then (D482).
 */
export interface RtcConnectOptions {
  readonly count?: number | undefined;
  readonly isNeededHidden?: (() => boolean) | undefined;
  /**
   * Fetches the ICE servers again, which an attempt asking a relay does first once those held are
   * near their expiry (RelayCredentials, D564 §3); absent, those given are offered for the
   * transport's life.
   */
  readonly iceServersAgain?: IceServersAgain | undefined;
  /** The wall clock the relay credentials' expiry is judged on: Date.now where absent. */
  readonly nowMs?: (() => number) | undefined;
}

/**
 * How a transport on page grows to options.count connections, each opened by open: tried again at once when
 * the browser is back online or the page shown again (onWayBack), and paced while the page is hidden
 * unless options.isNeededHidden says it needs its connection (D482). Where there is no page, nothing paces.
 */
export function growthOn(
  page: ShownPage | undefined,
  open: (signal: AbortSignal) => Promise<PeerLike>,
  { count = clientConnections, isNeededHidden }: RtcConnectOptions,
): Growth {
  return {
    open,
    size: count,
    onOnline: onWayBack(page),
    hidden: page === undefined ? undefined : { page, isNeededHidden },
  };
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

/** A peer connection with its offer made and its candidates gathered, and what it holds to connect. */
interface Offered {
  readonly peer: RTCPeerConnection;
  readonly verdict: Promise<void>;
  readonly iceAt: () => number | undefined;
  readonly channelAt: () => number | undefined;
  readonly sdp: string;
  readonly errors: readonly string[];
}

/** A new peer connection on rtcConfig, its offer made, gathered for at most waitMostMs. */
async function offerGathered(rtcConfig: RTCConfiguration, waitMostMs: number): Promise<Offered> {
  const peer = new RTCPeerConnection(rtcConfig);
  try {
    let channelAt: number | undefined;
    const verdict = verdictOf(channelOf(offerWith(peer)), () => {
      channelAt = performance.now();
    });
    verdict.catch(() => undefined);
    const iceAt = whenIceThrough(peer);
    const errors = gatheringErrorsOf(peer);
    await peer.setLocalDescription(await peer.createOffer());
    await gathered(peer, hasRelayServer(rtcConfig), waitMostMs);
    const local = peer.localDescription;
    if (local === null) {
      throw new Error("the connection made no offer");
    }
    return { peer, verdict, iceAt, channelAt: () => channelAt, sdp: local.sdp, errors: errors() };
  } catch (error: unknown) {
    peer.close();
    throw error;
  }
}

/**
 * One peer connection, connected through signaller; given up, its peer closed, once signal aborts.
 * Where the credentials' configuration names a TURN server, it climbs the ladder of attempts
 * rtc-ladder.ts gives, from the rung this page's network last needed (climbLadder; D559 §1, D561 §2),
 * each rung that asks a relay offering credentials good then, fetched again where stale (D564 §3).
 * The discovery's answer does not say which relay is a household's own, so it waits with the hosted
 * one.
 */
async function connectOne(
  signaller: Signaller,
  credentials: RelayCredentials,
  signal: AbortSignal,
): Promise<PeerLike> {
  const rtcConfig = credentials.config;
  if (!hasRelayServer(rtcConfig)) {
    return await connectAttempt(signaller, rtcConfig, { signal });
  }
  return await climbLadder(rtcConfig, {
    attempt: (config, hooks) => connectAttempt(signaller, config, hooks),
    isTurnedAway: (failure) => isTurnedAway(failure),
    signal,
    fresh: () => credentials.fresh(),
  });
}

/**
 * One attempt at a peer connection on rtcConfig, connected through signaller; given up, its peer
 * closed, once signal aborts; onAnswered told as the core's answer comes. An offer whose gathering
 * found nothing past this device is made once more, on a new peer connection, within what is left of
 * connectTimeoutMs, and the second sent whatever it holds: a phone on a bus's Wi-Fi offered only its
 * mDNS host candidates, which the core drops, and could not connect (the owner's Firefox on Android,
 * 2026-11-26).
 */
async function connectAttempt(
  signaller: Signaller,
  rtcConfig: RTCConfiguration,
  { signal, onAnswered }: AttemptHooks,
): Promise<PeerLike> {
  let peer: RTCPeerConnection | undefined;
  const giveUp = (): void => {
    peer?.close();
  };
  signal.addEventListener("abort", giveUp, { once: true });
  try {
    const startedAt = performance.now();
    let offered = await offerGathered(rtcConfig, connectTimeoutMs);
    peer = offered.peer;
    const leftMs = connectTimeoutMs - (performance.now() - startedAt);
    if (!isPastThisDevice(offered.sdp) && !signal.aborted && leftMs > 0) {
      const errors = offered.errors.length === 0 ? "" : ` (${errorsText(offered.errors)})`;
      noteStep(`WebRTC: gathered nothing past this device${errors}; gathering again`);
      peer.close();
      peer = undefined;
      offered = await offerGathered(rtcConfig, leftMs);
      peer = offered.peer;
    }
    const { verdict, sdp } = offered;
    const gatheredAt = performance.now();
    noteStep(`WebRTC: offer sent with ${gatheringText(sdp, offered.errors)}`);
    const answer = await signaller({ type: "offer", sdp }, signal);
    const answeredAt = performance.now();
    noteStep("WebRTC: the core answered");
    onAnswered?.();
    await peer.setRemoteDescription(answer);
    await connected(peer);
    const connectedAt = performance.now();
    noteStep("WebRTC: connected");
    await withinConnectTimeout(verdict);
    noteStep("WebRTC: the core held the connection");
    reportIfSlow(offered, { startedAt, gatheredAt, answeredAt, connectedAt });
    return peerOf(peer);
  } catch (error: unknown) {
    noteFailure(error, signal);
    peer?.close();
    throw error;
  } finally {
    signal.removeEventListener("abort", giveUp);
  }
}

/** When, on performance.now's clock, an attempt began and each of its steps before the verdict ended. */
interface AttemptMarks {
  readonly startedAt: number;
  readonly gatheredAt: number;
  readonly answeredAt: number;
  readonly connectedAt: number;
}

/** Reports a connection the core has just held, when it took over slowConnectMs, with where its time went. */
function reportIfSlow(offered: Offered, marks: AttemptMarks): void {
  const { startedAt, gatheredAt, answeredAt, connectedAt } = marks;
  const heldAt = performance.now();
  const slow = slowConnection({
    gathering: gatheredAt - startedAt,
    answer: answeredAt - gatheredAt,
    connecting: heldAt - answeredAt,
    steps: connectingStepsOf({
      answeredAt,
      iceAt: offered.iceAt(),
      connectedAt,
      channelAt: offered.channelAt(),
      heldAt,
    }),
  });
  if (slow !== undefined) {
    reportFailure(new Error(slow));
  }
}

/** Notes an attempt's failure as a step, unless it was given up as another connected (D559 §1). */
function noteFailure(error: unknown, signal: AbortSignal): void {
  if (signal.reason !== lostToAnotherAttempt) {
    noteStep(`WebRTC: failed: ${error instanceof Error ? error.message : String(error)}`);
  }
}

/** What whenIceThrough needs of a peer connection: the browser's, or a test's. */
export interface IcePeer {
  readonly iceConnectionState: RTCIceConnectionState;
  addEventListener(type: "iceconnectionstatechange", listener: () => void): void;
}

/**
 * When, on performance.now's clock, peer's ICE first had a pair that answered its checks (its ICE
 * connection state connected or completed), read as each state is told; undefined until it has.
 */
export function whenIceThrough(peer: IcePeer): () => number | undefined {
  let throughAt: number | undefined;
  peer.addEventListener("iceconnectionstatechange", () => {
    const state = peer.iceConnectionState;
    if (throughAt === undefined && (state === "connected" || state === "completed")) {
      throughAt = performance.now();
    }
  });
  return () => throughAt;
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
 * first server-reflexive one unless a relayed one comes first; or after waitMostMs (connectTimeoutMs
 * where not given) with those it has: whichever is first.
 */
export function gathered(
  peer: GatheringPeer,
  willRelay = false,
  waitMostMs: number = connectTimeoutMs,
): Promise<void> {
  return new Promise((resolve) => {
    if (peer.iceGatheringState === "complete") {
      resolve();
      return;
    }
    const timers = [setTimeout(done, waitMostMs)];
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
    const selected = candidatePairsOf(stats?.values() ?? []).find(
      (pair) => pair.isSelected === true,
    );
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
