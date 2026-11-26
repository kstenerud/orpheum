// SPDX-License-Identifier: MPL-2.0

/**
 * A browser's peer connection as the WebRTC transport (rtc-transport.ts) uses it: its channels, and
 * its word, once, that it is gone, judged from its states (D273 §8, plans/webrtc-transport.md W2d).
 */

import { candidatePairsOf } from "./rtc-pairs.ts";
import { channelOf } from "./rtc-request.ts";
import type { PeerLike } from "./rtc-transport.ts";
import { noteStep } from "./steps.ts";

/**
 * How long a connection may stay disconnected before it counts as gone and is replaced: a phone's
 * moment out of signal comes back by itself, but one moving from Wi-Fi to mobile data never does
 * until an ICE restart, which the core cannot take yet (plans/webrtc-transport.md W2d).
 */
export const disconnectedGraceMs = 5000;

/**
 * How often a connection's states are looked at besides their events: a browser that moves them
 * without an event, or closes a connection itself on a network change (Firefox, idiosyncrasies.md),
 * would otherwise leave the transport holding one gone for good.
 */
export const peerLookEveryMs = 1000;

/**
 * How long a page may go unrun, its looks at a connection's states stopped, before the connection
 * counts as gone: the core ends one it hears nothing from after pion's ICE waits, 5 s disconnected
 * and then 25 s failed, and a phone that froze the page in the background (Android Chrome and
 * Firefox, idiosyncrasies.md) hands it back still saying connected. Judged on the first look after
 * the page runs again, it is replaced at once rather than after disconnectedGraceMs, its requests
 * otherwise lost on it (the owner's phone back from another app, staging, 2026-11-14).
 */
export const coreHoldsSilentMs = 30_000;

/** What watchGone needs of a peer connection: its states, and their events. */
export interface StatesPeer {
  readonly connectionState: RTCPeerConnectionState;
  readonly iceConnectionState: RTCIceConnectionState;
  readonly signalingState: RTCSignalingState;
  addEventListener(
    type: "connectionstatechange" | "iceconnectionstatechange" | "signalingstatechange",
    listener: () => void,
  ): void;
}

/** The events whose states watchGone judges. */
const watchedStates = [
  "connectionstatechange",
  "iceconnectionstatechange",
  "signalingstatechange",
] as const;

/**
 * Whether peer can carry a request now: neither its connection nor its ICE is disconnected, failed
 * or closed, and its signalling is not closed. A phone whose network changed has every connection
 * disconnected at once, each still held for disconnectedGraceMs, and a read asked again on one of
 * them failed at once as its channel closed, its three asks spent before a new connection was up
 * (the owner's phone, production, 2026-11-26).
 */
export function canCarry(peer: StatesPeer): boolean {
  const unable: readonly string[] = ["disconnected", "failed", "closed"];
  return (
    peer.signalingState !== "closed" &&
    !unable.includes(peer.connectionState) &&
    !unable.includes(peer.iceConnectionState)
  );
}

/** What watchGone reads the time from, and whom it tells each judging that finds a peer not gone. */
export interface WatchTold {
  readonly nowMs?: () => number;
  readonly judged?: () => void;
}

/**
 * Tells listener once, with why, when peer is gone: its connection or its ICE failed or closed, its
 * signalling closed (the browser closed it), or either stayed disconnected for disconnectedGraceMs.
 * Its states are judged at each of their events and every peerLookEveryMs besides, the connection's
 * and the ICE's apart, since browsers do not move them together (idiosyncrasies.md, Firefox). One
 * whose page went unrun for coreHoldsSilentMs between two looks, the core having ended it meanwhile,
 * is gone too, the times read from told's nowMs. Each judging that finds it not gone is told to told's judged, so
 * one back from disconnected, told by no event, is seen within peerLookEveryMs. Returns the end of
 * watching.
 */
export function watchGone(
  peer: StatesPeer,
  listener: (why: string) => void,
  told: WatchTold = {},
): () => void {
  const { nowMs = () => Date.now(), judged = () => undefined } = told;
  let isGone = false;
  const unrunFor = sinceLast(nowMs);
  let grace: ReturnType<typeof setTimeout> | undefined;
  const stop = (): void => {
    clearTimeout(grace);
    clearInterval(looking);
  };
  const gone = (why: string): void => {
    stop();
    if (!isGone) {
      isGone = true;
      noteStep(`WebRTC: a connection is gone: ${why}`);
      listener(why);
    }
  };
  const isDisconnected = (): boolean =>
    peer.connectionState === "disconnected" || peer.iceConnectionState === "disconnected";
  const judge = (): void => {
    if (isGone) {
      return;
    }
    const { connectionState, iceConnectionState, signalingState } = peer;
    if (signalingState === "closed") {
      gone("its signalling closed");
    } else if (connectionState === "failed" || connectionState === "closed") {
      gone(`its connection ${connectionState}`);
    } else if (iceConnectionState === "failed" || iceConnectionState === "closed") {
      gone(`its ICE ${iceConnectionState}`);
    } else if (isDisconnected()) {
      grace ??= setTimeout(() => {
        grace = undefined;
        // Judged again as the grace ends: one back by now, told by no event yet, is not gone.
        if (isDisconnected()) {
          gone(`it stayed disconnected for ${String(disconnectedGraceMs)} ms`);
        } else {
          judge();
        }
      }, disconnectedGraceMs);
      judged();
    } else {
      clearTimeout(grace);
      grace = undefined;
      judged();
    }
  };
  const looking = setInterval(() => {
    lookAfter(unrunFor(), judge, gone);
  }, peerLookEveryMs);
  for (const type of watchedStates) {
    peer.addEventListener(type, judge);
  }
  judge();
  return stop;
}

/** How long it has been since it was last called, or made, the times read from nowMs. */
function sinceLast(nowMs: () => number): () => number {
  let lastMs = nowMs();
  return () => {
    const sinceMs = nowMs() - lastMs;
    lastMs += sinceMs;
    return sinceMs;
  };
}

/** Says gone once its page went unrun for coreHoldsSilentMs, for unrunMs; judges otherwise. */
function lookAfter(unrunMs: number, judge: () => void, gone: (why: string) => void): void {
  if (unrunMs >= coreHoldsSilentMs) {
    gone(`its page went unrun for ${String(unrunMs)} ms, longer than the core holds a silent one`);
  } else {
    judge();
  }
}

/**
 * A browser's peer connection as the transport uses it: each channel delivers ArrayBuffers, it says
 * whether it can carry a request now (canCarry) and each time its states were judged, and it says
 * once when it is gone, and why (watchGone).
 */
export function peerOf(peer: RTCPeerConnection): PeerLike {
  let stopWatching = (): void => undefined;
  const judgedListeners: (() => void)[] = [];
  const judged = (): void => {
    for (const listener of judgedListeners) {
      listener();
    }
  };
  return {
    createDataChannel: (label) => {
      const channel = peer.createDataChannel(label);
      channel.binaryType = "arraybuffer";
      return channelOf(channel);
    },
    close: () => {
      stopWatching();
      peer.close();
    },
    candidatePairs: async () => candidatePairsOf((await peer.getStats()).values()),
    canCarry: () => canCarry(peer),
    onJudged: (listener) => {
      judgedListeners.push(listener);
    },
    onGone: (listener) => {
      stopWatching = watchGone(peer, listener, { judged });
    },
  };
}
