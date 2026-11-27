// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { networkChangeBurstMs, onWayBack, type ChangingNetwork } from "./rtc-connect.ts";
import { canCarry, disconnectedGraceMs, peerLookEveryMs, watchGone } from "./rtc-peer.ts";
import type { ChannelLike } from "./rtc-request.ts";
import { forgetSteps } from "./steps.ts";
import { RtcTransport, settledAfterMs, type PeerLike } from "./rtc-transport.ts";

// Plan item 10, option D (the owner, 2026-11-27: "Do it"): the owner's phone, its network changed,
// held every connection disconnected for disconnectedGraceMs before it made a new one (production,
// 2026-11-26). Chrome on Android tells the change (navigator.connection); Firefox and Safari do not.

/**
 * A connection whose states the test moves, judged as a browser's is (peerOf), told no state event,
 * counting the network changes it was told.
 */
class ChangedLink implements PeerLike {
  connectionState: RTCPeerConnectionState = "connected";
  iceConnectionState: RTCIceConnectionState = "connected";
  signalingState: RTCSignalingState = "stable";
  isClosed = false;
  changes = 0;
  private readonly changedListeners: (() => void)[] = [];

  addEventListener(): void {
    // The test moves the states with no event; the looks see them.
  }

  createDataChannel(): ChannelLike {
    throw new Error("the test opens no channel");
  }

  close(): void {
    this.isClosed = true;
  }

  canCarry(): boolean {
    return canCarry(this);
  }

  networkChanged(): void {
    this.changes += 1;
    for (const each of this.changedListeners) {
      each();
    }
  }

  onGone(listener: (why: string) => void): void {
    watchGone(this, listener, {
      onNetworkChanged: (changed) => {
        this.changedListeners.push(changed);
      },
    });
  }
}

/** The browser's network information (navigator.connection), its change event fired by the test. */
function changingNetwork(): ChangingNetwork & {
  readonly fire: () => void;
  readonly hearing: () => number;
} {
  const listeners = new Set<() => void>();
  return {
    addEventListener: (_type, listener) => {
      listeners.add(listener);
    },
    removeEventListener: (_type, listener) => {
      listeners.delete(listener);
    },
    fire: () => {
      for (const listener of listeners) {
        listener();
      }
    },
    hearing: () => listeners.size,
  };
}

/** A transport on first, its tries made by open into opened, hearing network through onWayBack. */
function transportOn(
  first: ChangedLink,
  opened: ChangedLink[],
  network: ChangingNetwork | undefined,
): RtcTransport {
  return new RtcTransport(
    [first],
    {
      open: () => {
        const peer = new ChangedLink();
        opened.push(peer);
        return Promise.resolve(peer);
      },
      size: 1,
      onOnline: onWayBack(undefined, network),
    },
    () => Date.now(),
  );
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  forgetSteps();
});

describe("a connection told the browser's network changed", () => {
  it("is gone at once when disconnected then, or soon after, its grace cut short", () => {
    const peer = new ChangedLink();
    const told: string[] = [];
    peer.onGone((why) => told.push(why));
    peer.networkChanged();
    expect(told).toEqual([]); // connected yet
    vi.advanceTimersByTime(peerLookEveryMs);
    peer.iceConnectionState = "disconnected"; // the change reaches the ICE a moment later
    vi.advanceTimersByTime(peerLookEveryMs);
    expect(told).toEqual(["it went disconnected as the browser's network changed"]);
  });

  it("left connected, is left as it was, and a disconnection long after waits out its grace", () => {
    const peer = new ChangedLink();
    const told: string[] = [];
    peer.onGone((why) => told.push(why));
    peer.networkChanged();
    vi.advanceTimersByTime(disconnectedGraceMs * 3);
    expect(told).toEqual([]);
    peer.connectionState = "disconnected";
    vi.advanceTimersByTime(disconnectedGraceMs - peerLookEveryMs);
    expect(told).toEqual([]); // within its grace, a moment out of signal
    peer.connectionState = "connected";
    vi.advanceTimersByTime(disconnectedGraceMs * 2);
    expect(told).toEqual([]);
  });
});

describe("a transport whose browser says its network changed", () => {
  it("with its connection dead, makes a new one sooner than without the change", async () => {
    const network = changingNetwork();
    const told = { first: new ChangedLink(), opened: [] as ChangedLink[] };
    const untold = { first: new ChangedLink(), opened: [] as ChangedLink[] };
    const transport = transportOn(told.first, told.opened, network);
    const without = transportOn(untold.first, untold.opened, changingNetwork()); // never fired
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    told.first.iceConnectionState = "disconnected";
    untold.first.iceConnectionState = "disconnected";
    network.fire();
    await vi.advanceTimersByTimeAsync(peerLookEveryMs);
    expect(told.first.isClosed).toBe(true);
    expect(told.opened).toHaveLength(1);
    expect(transport.size).toBe(1);
    expect(untold.opened).toHaveLength(0); // still in its grace
    await vi.advanceTimersByTimeAsync(disconnectedGraceMs);
    expect(untold.opened).toHaveLength(1); // as before, a grace later
    transport.close();
    without.close();
  });

  it("with its connection live, keeps it and opens none, a burst of changes judging it once", async () => {
    const network = changingNetwork();
    const first = new ChangedLink();
    const opened: ChangedLink[] = [];
    const transport = transportOn(first, opened, network);
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    for (let each = 0; each < 5; each += 1) {
      network.fire();
      await vi.advanceTimersByTimeAsync(networkChangeBurstMs / 4);
    }
    await vi.advanceTimersByTimeAsync(disconnectedGraceMs * 2);
    expect(first.changes).toBe(1);
    expect(first.isClosed).toBe(false);
    expect(opened).toHaveLength(0);
    expect(transport.size).toBe(1);
    network.fire(); // a change after the burst is another check
    expect(first.changes).toBe(2);
    expect(network.hearing()).toBe(1);
    transport.close();
    expect(network.hearing()).toBe(0); // heard no more once closed
  });

  it("with no network information, as in Firefox and Safari, keeps and replaces its connections as before", async () => {
    vi.stubGlobal("navigator", {});
    const first = new ChangedLink();
    const opened: ChangedLink[] = [];
    const transport = new RtcTransport(
      [first],
      {
        open: () => {
          const peer = new ChangedLink();
          opened.push(peer);
          return Promise.resolve(peer);
        },
        size: 1,
        onOnline: onWayBack(undefined),
      },
      () => Date.now(),
    );
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    expect(transport.size).toBe(1);
    first.iceConnectionState = "disconnected";
    await vi.advanceTimersByTimeAsync(disconnectedGraceMs - peerLookEveryMs);
    expect(opened).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(peerLookEveryMs * 2);
    expect(opened).toHaveLength(1);
    expect(first.changes).toBe(0);
    transport.close();
  });

  it("hears navigator.connection's change where the browser has one", () => {
    const network = changingNetwork();
    vi.stubGlobal("navigator", { connection: network });
    const heard: string[] = [];
    const stop = onWayBack(undefined)((why) => heard.push(why));
    network.fire();
    expect(heard).toEqual(["changed"]);
    stop();
    expect(network.hearing()).toBe(0);
  });
});
