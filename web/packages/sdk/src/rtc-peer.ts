// SPDX-License-Identifier: MPL-2.0

/**
 * A browser's peer connection as the WebRTC transport (rtc-transport.ts) uses it: its channels, and
 * its word, once, that it is gone, judged from its states (D273 §8, plans/webrtc-transport.md W2d).
 */

import { candidatePairsOf } from "./rtc-pairs.ts";
import { channelOf } from "./rtc-request.ts";
import type { PeerLike } from "./rtc-transport.ts";

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
 * Tells listener once, with why, when peer is gone: its connection or its ICE failed or closed, its
 * signalling closed (the browser closed it), or either stayed disconnected for disconnectedGraceMs.
 * Its states are judged at each of their events and every peerLookEveryMs besides, the connection's
 * and the ICE's apart, since browsers do not move them together (idiosyncrasies.md, Firefox).
 * Returns the end of watching.
 */
export function watchGone(peer: StatesPeer, listener: (why: string) => void): () => void {
  let isGone = false;
  let grace: ReturnType<typeof setTimeout> | undefined;
  const stop = (): void => {
    clearTimeout(grace);
    clearInterval(looking);
  };
  const gone = (why: string): void => {
    stop();
    if (!isGone) {
      isGone = true;
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
    } else {
      clearTimeout(grace);
      grace = undefined;
    }
  };
  const looking = setInterval(judge, peerLookEveryMs);
  for (const type of watchedStates) {
    peer.addEventListener(type, judge);
  }
  judge();
  return stop;
}

/**
 * A browser's peer connection as the transport uses it: each channel delivers ArrayBuffers, and it
 * says once when it is gone, and why (watchGone).
 */
export function peerOf(peer: RTCPeerConnection): PeerLike {
  let stopWatching = (): void => undefined;
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
    onGone: (listener) => {
      stopWatching = watchGone(peer, listener);
    },
  };
}
