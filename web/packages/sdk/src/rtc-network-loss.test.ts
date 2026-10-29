// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { disconnectedGraceMs, peerLookEveryMs, watchGone, type StatesPeer } from "./rtc-peer.ts";
import { openTryMostMs } from "./rtc-reopen.ts";
import type { ChannelLike } from "./rtc-request.ts";
import {
  replaceFirstMs,
  replaceMostMs,
  RtcTransport,
  settledAfterMs,
  type PeerLike,
} from "./rtc-transport.ts";

// The owner's Mac, Wi-Fi off for a minute and on again, Watch Together over WebRTC through staging's
// discovery: Chrome connected again, and Firefox never made another connection, its Play page
// spinning for good (staging, 2026-10-29; VQ130).

/** A connection the transport holds, gone when the test says. */
class LostPeer implements PeerLike {
  isClosed = false;
  private goneListener: ((why: string) => void) | undefined;

  createDataChannel(): ChannelLike {
    throw new Error("the test opens no channel");
  }

  close(): void {
    this.isClosed = true;
  }

  onGone(listener: (why: string) => void): void {
    this.goneListener = listener;
  }

  goes(why: string): void {
    this.goneListener?.(why);
  }
}

/** A browser's peer connection whose states the test moves, telling no event unless asked. */
class SilentStates implements StatesPeer {
  connectionState: RTCPeerConnectionState = "connected";
  iceConnectionState: RTCIceConnectionState = "connected";
  signalingState: RTCSignalingState = "stable";
  private readonly listeners: (() => void)[] = [];

  addEventListener(_type: string, listener: () => void): void {
    this.listeners.push(listener);
  }

  tell(): void {
    for (const listener of this.listeners) {
      listener();
    }
  }
}

/** The browser's online event, as the test fires it, and how many hear it. */
function onlineEvent(): {
  readonly onOnline: (listener: () => void) => () => void;
  readonly fire: () => void;
  readonly hearing: () => number;
} {
  const listeners = new Set<() => void>();
  return {
    onOnline: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    fire: () => {
      for (const listener of listeners) {
        listener();
      }
    },
    hearing: () => listeners.size,
  };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("a connection's states, watched for its going", () => {
  it("is gone when its ICE stays disconnected, its connection still saying connected and no event told", () => {
    const peer = new SilentStates();
    const told: string[] = [];
    watchGone(peer, (why) => told.push(why));
    peer.iceConnectionState = "disconnected"; // no event, and connectionState unmoved
    vi.advanceTimersByTime(disconnectedGraceMs + peerLookEveryMs);
    expect(told).toEqual([`it stayed disconnected for ${String(disconnectedGraceMs)} ms`]);
    vi.advanceTimersByTime(disconnectedGraceMs * 4);
    expect(told).toHaveLength(1); // once
  });

  it("is not gone when it comes back within the grace", () => {
    const peer = new SilentStates();
    const told: string[] = [];
    watchGone(peer, (why) => told.push(why));
    peer.connectionState = "disconnected";
    peer.tell();
    vi.advanceTimersByTime(disconnectedGraceMs - peerLookEveryMs);
    peer.connectionState = "connected";
    vi.advanceTimersByTime(disconnectedGraceMs * 2);
    expect(told).toEqual([]);
  });

  it("is gone at once when the browser closed it itself, and watches no more once stopped", () => {
    const peer = new SilentStates();
    const told: string[] = [];
    watchGone(peer, (why) => told.push(why));
    peer.signalingState = "closed";
    vi.advanceTimersByTime(peerLookEveryMs);
    expect(told).toEqual(["its signalling closed"]);

    const other = new SilentStates();
    const stop = watchGone(other, (why) => told.push(why));
    stop();
    other.connectionState = "failed";
    vi.advanceTimersByTime(peerLookEveryMs * 10);
    expect(told).toHaveLength(1);
  });
});

describe("a transport whose network went", () => {
  /** A transport on first, its tries made by open, hearing the online event given. */
  function transportOn(
    first: LostPeer,
    open: (signal: AbortSignal) => Promise<PeerLike>,
    online = onlineEvent(),
  ): RtcTransport {
    return new RtcTransport([first], { open, size: 4, onOnline: online.onOnline }, () =>
      Date.now(),
    );
  }

  it("tries again after a wait that doubles to replaceMostMs at the longest, however long it fails", async () => {
    const first = new LostPeer();
    const triedAtMs: number[] = [];
    transportOn(first, () => {
      triedAtMs.push(Date.now());
      return Promise.reject(new TypeError("NetworkError when attempting to fetch resource."));
    });
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    const goneAtMs = Date.now();
    first.goes("it stayed disconnected for 5000 ms");
    await vi.advanceTimersByTimeAsync(10 * 60_000);
    const waits = triedAtMs.slice(1).map((atMs, index) => atMs - (triedAtMs[index] ?? 0));
    expect(triedAtMs[0]).toBe(goneAtMs); // at once, for one that had lived
    expect(waits.slice(0, 5)).toEqual([
      replaceFirstMs,
      replaceFirstMs * 2,
      replaceFirstMs * 4,
      replaceFirstMs * 8,
      replaceMostMs,
    ]);
    expect(Math.max(...waits)).toBe(replaceMostMs);
    expect(replaceMostMs).toBeLessThanOrEqual(10_000);
  });

  it("gives up a try that hangs after openTryMostMs, aborting it, and tries again", async () => {
    const first = new LostPeer();
    const signals: AbortSignal[] = [];
    transportOn(first, (signal) => {
      signals.push(signal);
      return new Promise<PeerLike>(() => undefined); // its offer's post never answers
    });
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    first.goes("its connection failed");
    await vi.advanceTimersByTimeAsync(openTryMostMs - 1);
    expect(signals).toHaveLength(1);
    expect(signals[0]?.aborted).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(signals[0]?.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(replaceFirstMs);
    expect(signals).toHaveLength(2);
  });

  it("tries at once when the browser comes back online, giving up a try under way, and says how long it had none", async () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const first = new LostPeer();
    const online = onlineEvent();
    const signals: AbortSignal[] = [];
    let isNetworkBack = false;
    const opened: LostPeer[] = [];
    const transport = transportOn(
      first,
      (signal) => {
        signals.push(signal);
        if (!isNetworkBack) {
          return Promise.reject(new TypeError("NetworkError when attempting to fetch resource."));
        }
        const peer = new LostPeer();
        opened.push(peer);
        return Promise.resolve(peer);
      },
      online,
    );
    let toldBack = 0;
    transport.onBack(() => {
      toldBack += 1;
    });
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    first.goes("it stayed disconnected for 5000 ms");
    await vi.advanceTimersByTimeAsync(replaceFirstMs * 15); // its wait has grown to the longest
    const tries = signals.length;
    isNetworkBack = true;
    online.fire();
    await vi.advanceTimersByTimeAsync(0);
    expect(signals).toHaveLength(tries + 1);
    expect(transport.size).toBe(1);
    expect(toldBack).toBe(2); // online, then a connection up after none
    const reported = error.mock.calls.map(([, cause]) =>
      cause instanceof Error ? cause.message : "",
    );
    expect(reported).toContain(
      `a WebRTC connection to the core is back after ${String(replaceFirstMs * 15)} ms with none;` +
        ` the last was taken as gone because it stayed disconnected for 5000 ms;` +
        ` ${String(tries)} tries failed, the last: NetworkError when attempting to fetch resource.`,
    );

    // A try under way as the browser comes back online, begun on the network that went, is given up.
    let isHanging = true;
    const hanging = new LostPeer();
    const again = transportOn(
      hanging,
      (signal) => {
        signals.push(signal);
        return isHanging ? new Promise<PeerLike>(() => undefined) : Promise.resolve(new LostPeer());
      },
      online,
    );
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    hanging.goes("its ICE failed");
    await vi.advanceTimersByTimeAsync(0);
    const underWay = signals.at(-1);
    isHanging = false;
    online.fire();
    await vi.advanceTimersByTimeAsync(0);
    expect(underWay?.aborted).toBe(true);
    expect(again.size).toBe(1);
    transport.close();
    again.close();
    expect(online.hearing()).toBe(0);
  });
});
