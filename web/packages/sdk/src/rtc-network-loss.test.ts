// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  coreHoldsSilentMs,
  disconnectedGraceMs,
  peerLookEveryMs,
  watchGone,
  type StatesPeer,
} from "./rtc-peer.ts";
import { onWayBack } from "./rtc-connect.ts";
import {
  hiddenTriesForMs,
  hiddenTryEveryMs,
  openTryMostMs,
  RefusedForGood,
  Unreached,
  unreachedRetryMs,
} from "./rtc-reopen.ts";
import type { ChannelLike } from "./rtc-request.ts";
import { forgetSteps, stepsSoFar } from "./steps.ts";
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

  it("is gone on its first look once its page ran again after the core ended it, still saying connected (the owner's phone, 2026-11-14)", () => {
    const peer = new SilentStates();
    const told: string[] = [];
    watchGone(peer, (why) => told.push(why));
    vi.advanceTimersByTime(peerLookEveryMs * 3);
    vi.setSystemTime(Date.now() + coreHoldsSilentMs - peerLookEveryMs * 2); // a short freeze
    vi.advanceTimersByTime(peerLookEveryMs);
    expect(told).toEqual([]);
    vi.setSystemTime(Date.now() + 219_768); // frozen in the background, no look run meanwhile
    vi.advanceTimersByTime(peerLookEveryMs);
    expect(told).toEqual([
      `its page went unrun for ${String(219_768 + peerLookEveryMs)} ms, longer than the core holds a silent one`,
    ]);
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
    // Told in what happened too, where a page that lost its connection says why.
    expect(stepsSoFar().at(-1)).toMatch(/WebRTC: a connection is gone: its signalling closed$/);
    forgetSteps();

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

  it("with no online event, as Firefox fires none, is back within unreachedRetryMs of the network, its offers having reached nothing", async () => {
    // The owner's Mac, Wi-Fi off for about a minute (VQ139's run, staging, 2026-10-29): Firefox came
    // back ~45 s after the Wi-Fi did, its wait grown to replaceMostMs and no online event to cut it.
    const first = new LostPeer();
    const online = onlineEvent(); // never fired
    const triedAtMs: number[] = [];
    let isNetworkBack = false;
    const transport = transportOn(
      first,
      () => {
        triedAtMs.push(Date.now());
        return isNetworkBack
          ? Promise.resolve(new LostPeer())
          : Promise.reject(
              new Unreached(new TypeError("NetworkError when attempting to fetch resource.")),
            );
      },
      online,
    );
    let toldBack = 0;
    transport.onBack(() => {
      toldBack += 1;
    });
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    first.goes("it stayed disconnected for 5000 ms");
    await vi.advanceTimersByTimeAsync(60_000);
    const waits = triedAtMs.slice(1).map((atMs, index) => atMs - (triedAtMs[index] ?? 0));
    expect(new Set(waits)).toEqual(new Set([unreachedRetryMs])); // never grown
    isNetworkBack = true;
    const backAtMs = Date.now();
    await vi.advanceTimersByTimeAsync(unreachedRetryMs);
    expect(transport.size).toBe(1);
    expect((triedAtMs.at(-1) ?? 0) - backAtMs).toBeLessThanOrEqual(unreachedRetryMs);
    expect(toldBack).toBe(1); // the session connection hears it back without an online event
    expect(unreachedRetryMs).toBeLessThan(replaceMostMs);
    transport.close();
  });

  it("after its offers reached nothing, starts its growing wait over for a try that reached the core", async () => {
    const first = new LostPeer();
    const triedAtMs: number[] = [];
    let tries = 0;
    transportOn(first, () => {
      tries += 1;
      triedAtMs.push(Date.now());
      // Five tries with a core that does not answer, then five on no network, then the core again.
      const hasReachedNothing = tries > 5 && tries <= 10;
      return Promise.reject(
        hasReachedNothing
          ? new Unreached(new TypeError("Failed to fetch"))
          : new Error("the discovery answered 502"),
      );
    });
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    first.goes("its ICE failed");
    await vi.advanceTimersByTimeAsync(5 * 60_000);
    const waits = triedAtMs.slice(1).map((atMs, index) => atMs - (triedAtMs[index] ?? 0));
    expect(waits.slice(4, 10)).toEqual([
      replaceMostMs, // the core's fifth failure, its wait grown
      unreachedRetryMs,
      unreachedRetryMs,
      unreachedRetryMs,
      unreachedRetryMs,
      unreachedRetryMs,
    ]);
    expect(waits[10]).toBe(replaceFirstMs); // the core's again: the wait started over
  });

  it("tries no more once a try is refused for good, failing the requests waiting and saying so", async () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const first = new LostPeer();
    const online = onlineEvent();
    let tries = 0;
    const transport = transportOn(
      first,
      () => {
        tries += 1;
        // A spent link's key, and then what the device holds now, both refused (discovery.ts).
        return Promise.reject(
          new AggregateError([new Error("a name", { cause: new RefusedForGood("refused") })]),
        );
      },
      online,
    );
    const told: string[] = [];
    transport.onRefused((refusal) => told.push(refusal.message));
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    first.goes("it stayed disconnected for 5000 ms");
    const failed = transport
      .fetch(new Request("https://core.example/api/v1/me"))
      .catch((caught: unknown) => caught);
    await vi.advanceTimersByTimeAsync(10 * 60_000);
    online.fire();
    await vi.advanceTimersByTimeAsync(replaceMostMs * 3);
    expect(tries).toBe(1);
    expect(told).toHaveLength(1);
    expect(await failed).toBeInstanceOf(AggregateError);
    const late: string[] = [];
    transport.onRefused((refusal) => late.push(refusal.message)); // told at once, refused already
    expect(late).toEqual(told);
    const reported = error.mock.calls.map(([, cause]) =>
      cause instanceof Error ? cause.message : "",
    );
    expect(reported.some((each) => each.includes("none is tried again"))).toBe(true);
    transport.close();
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

  it("lost twice in a row, its second connection gone 23 s after it came, keeps trying through two minutes of hanging tries and is back within one try's bound of the network (the owner's phone, production, 2026-11-26)", async () => {
    // Mobile data to Wi-Fi: the core heard the new connections 11 s after the old; 23 s later it
    // heard them no more (a VPN coming on, it may be), and no connection came for two minutes.
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    let network: "works" | "hangs" = "works";
    const triedAtMs: number[] = [];
    const opened: LostPeer[] = [];
    const first = new LostPeer();
    const transport = transportOn(first, (signal) => {
      triedAtMs.push(Date.now());
      if (network === "hangs") {
        // Its offer's post, or its ICE, never answers on a network that takes nothing through.
        return new Promise<PeerLike>((_resolve, reject) => {
          signal.addEventListener("abort", () => {
            reject(new Error("the try was given up"));
          });
        });
      }
      const peer = new LostPeer();
      opened.push(peer);
      return Promise.resolve(peer);
    });
    await vi.advanceTimersByTimeAsync(10 * 60_000); // a film page open long before
    first.goes("it stayed disconnected for 5000 ms");
    await vi.advanceTimersByTimeAsync(0);
    expect(transport.size).toBe(1); // the second network's connection, at once
    await vi.advanceTimersByTimeAsync(23_000); // past settledAfterMs: settled
    network = "hangs";
    const lostAgainAtMs = Date.now();
    opened[0]?.goes("it stayed disconnected for 5000 ms");
    await vi.advanceTimersByTimeAsync(120_000);
    expect(transport.size).toBe(0);
    const tries = triedAtMs.filter((atMs) => atMs >= lostAgainAtMs);
    expect(tries[0]).toBe(lostAgainAtMs); // at once, for one that had lived
    const gaps = tries.slice(1).map((atMs, index) => atMs - (tries[index] ?? 0));
    // Never a wait of minutes: a try's bound and the longest wait between tries at most.
    expect(Math.max(...gaps)).toBeLessThanOrEqual(openTryMostMs + replaceMostMs);
    expect(tries.length).toBeGreaterThanOrEqual(
      Math.floor(120_000 / (openTryMostMs + replaceMostMs)),
    );
    network = "works";
    const backAtMs = Date.now();
    await vi.advanceTimersByTimeAsync(openTryMostMs + replaceMostMs);
    expect(transport.size).toBe(1);
    expect((triedAtMs.at(-1) ?? 0) - backAtMs).toBeLessThanOrEqual(openTryMostMs + replaceMostMs);
    transport.close();
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

describe("a transport whose page is hidden (D482)", () => {
  /** A page the test hides and shows, telling its visibilitychange listeners. */
  function testPage(): {
    readonly page: {
      visibilityState: DocumentVisibilityState;
      addEventListener: (type: "visibilitychange", listener: () => void) => void;
      removeEventListener: (type: "visibilitychange", listener: () => void) => void;
    };
    readonly turn: (to: DocumentVisibilityState) => void;
  } {
    const listeners = new Set<() => void>();
    const page = {
      visibilityState: "visible" as DocumentVisibilityState,
      addEventListener: (_type: "visibilitychange", listener: () => void) => {
        listeners.add(listener);
      },
      removeEventListener: (_type: "visibilitychange", listener: () => void) => {
        listeners.delete(listener);
      },
    };
    return {
      page,
      turn: (to) => {
        page.visibilityState = to;
        for (const listener of listeners) {
          listener();
        }
      },
    };
  }

  /**
   * A transport on a page, as connectRtc makes one, whose connection goes and whose every try
   * after reaches no server, as on a phone with no network; returns when each try began.
   */
  async function lostOn(
    shown: ReturnType<typeof testPage>,
    isNeededHidden?: () => boolean,
  ): Promise<{ readonly transport: RtcTransport; readonly triedAtMs: number[] }> {
    const first = new LostPeer();
    const triedAtMs: number[] = [];
    const transport = new RtcTransport(
      [first],
      {
        open: () => {
          triedAtMs.push(Date.now());
          return Promise.reject(new Unreached(new TypeError("Failed to fetch")));
        },
        size: 4,
        onOnline: onWayBack(shown.page),
        hidden: { page: shown.page, isNeededHidden },
      },
      () => Date.now(),
    );
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    first.goes("its page went unrun for 40000 ms, longer than the core holds a silent one");
    return { transport, triedAtMs };
  }

  /** The waits between the tries begun at triedAtMs. */
  function waitsOf(triedAtMs: readonly number[]): number[] {
    return triedAtMs.slice(1).map((atMs, index) => atMs - (triedAtMs[index] ?? 0));
  }

  it("tries every hiddenTryEveryMs for its first hiddenTriesForMs hidden, then none until shown, when it tries at once", async () => {
    const shown = testPage();
    shown.turn("hidden");
    const hiddenAtMs = Date.now();
    const { transport, triedAtMs } = await lostOn(shown);
    await vi.advanceTimersByTimeAsync(10 * 60_000);
    expect(triedAtMs[0]).toBe(hiddenAtMs + settledAfterMs); // the first at once
    expect(new Set(waitsOf(triedAtMs))).toEqual(new Set([hiddenTryEveryMs]));
    expect(triedAtMs.at(-1)).toBeLessThan(hiddenAtMs + hiddenTriesForMs);
    expect(triedAtMs.at(-1)).toBeGreaterThanOrEqual(
      hiddenAtMs + hiddenTriesForMs - hiddenTryEveryMs,
    );
    const tries = triedAtMs.length;
    shown.turn("visible");
    await vi.advanceTimersByTimeAsync(0);
    expect(triedAtMs).toHaveLength(tries + 1);
    expect(triedAtMs.at(-1)).toBe(Date.now());
    await vi.advanceTimersByTimeAsync(60_000); // shown, the shown schedule again
    expect(new Set(waitsOf(triedAtMs.slice(tries)))).toEqual(new Set([unreachedRetryMs]));
    transport.close();
  });

  it("hidden again after being shown, counts its hiddenTriesForMs from the new hiding", async () => {
    const shown = testPage();
    shown.turn("hidden");
    const { transport, triedAtMs } = await lostOn(shown);
    await vi.advanceTimersByTimeAsync(hiddenTriesForMs * 2);
    shown.turn("visible");
    await vi.advanceTimersByTimeAsync(unreachedRetryMs * 3);
    shown.turn("hidden");
    const hiddenAgainAtMs = Date.now();
    await vi.advanceTimersByTimeAsync(10 * 60_000);
    const sinceHidden = triedAtMs.filter((atMs) => atMs > hiddenAgainAtMs);
    expect(new Set(waitsOf(sinceHidden))).toEqual(new Set([hiddenTryEveryMs]));
    expect(sinceHidden.at(-1)).toBeLessThan(hiddenAgainAtMs + hiddenTriesForMs);
    expect(sinceHidden.length).toBeGreaterThanOrEqual(hiddenTriesForMs / hiddenTryEveryMs - 1);
    transport.close();
  });

  it("keeps the shown schedule while hidden for as long as its app says it needs its connection: a film playing, a cast's remote", async () => {
    const shown = testPage();
    shown.turn("hidden");
    let isNeeded = true;
    const { transport, triedAtMs } = await lostOn(shown, () => isNeeded);
    await vi.advanceTimersByTimeAsync(hiddenTriesForMs * 2);
    expect(new Set(waitsOf(triedAtMs))).toEqual(new Set([unreachedRetryMs]));
    expect(triedAtMs.at(-1)).toBeGreaterThan(Date.now() - hiddenTryEveryMs);
    isNeeded = false; // the film paused, long hidden: no more tries
    const tries = triedAtMs.length;
    await vi.advanceTimersByTimeAsync(10 * 60_000);
    expect(triedAtMs.length).toBeLessThanOrEqual(tries + 1);
    transport.close();
  });

  it("shown, keeps its schedule as before", async () => {
    const shown = testPage();
    const { transport, triedAtMs } = await lostOn(shown);
    await vi.advanceTimersByTimeAsync(hiddenTriesForMs * 2);
    expect(new Set(waitsOf(triedAtMs))).toEqual(new Set([unreachedRetryMs]));
    expect(unreachedRetryMs).toBeLessThan(hiddenTryEveryMs);
    transport.close();
  });

  /**
   * A transport on a page, as connectRtc makes one, whose connection goes and whose tries hang until
   * the test answers them; returns each try's signal and the answering of the last.
   */
  async function hangingOn(shown: ReturnType<typeof testPage>): Promise<{
    readonly transport: RtcTransport;
    readonly signals: AbortSignal[];
    readonly answer: () => void;
  }> {
    const first = new LostPeer();
    const signals: AbortSignal[] = [];
    let answer = (): void => undefined;
    const transport = new RtcTransport(
      [first],
      {
        open: (signal) => {
          signals.push(signal);
          return new Promise<PeerLike>((resolve) => {
            answer = () => {
              resolve(new LostPeer());
            };
          });
        },
        size: 4,
        onOnline: onWayBack(shown.page),
        hidden: { page: shown.page },
      },
      () => Date.now(),
    );
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    first.goes("its ICE failed");
    await vi.advanceTimersByTimeAsync(0);
    return {
      transport,
      signals,
      answer: () => {
        answer();
      },
    };
  }

  // On flapping mobile data a page shown more often than one 6–8 s connect takes never connected:
  // each show gave up the try under way (the slow-link war game, 2026-11-14; D484 §2).
  it("keeps a try under way when its page is shown again, so the try connects", async () => {
    const shown = testPage();
    const { transport, signals, answer } = await hangingOn(shown);
    expect(signals).toHaveLength(1);
    for (let flap = 0; flap < 3; flap += 1) {
      await vi.advanceTimersByTimeAsync(1_000);
      shown.turn("hidden");
      await vi.advanceTimersByTimeAsync(500);
      shown.turn("visible");
    }
    await vi.advanceTimersByTimeAsync(0);
    expect(signals).toHaveLength(1);
    expect(signals[0]?.aborted).toBe(false);
    answer();
    await vi.advanceTimersByTimeAsync(0);
    expect(transport.size).toBe(1);
    transport.close();
  });

  // A phone hands a page back from the background with a try begun before it froze: once the clock
  // says it ran past its own deadline, though its timer has not yet run, the show gives it up and makes
  // another at once (the owner's phone, staging, 2026-11-14; VQ317).
  it("gives up a try under way that a frozen page held past openTryMostMs, and tries again at once", async () => {
    const shown = testPage();
    const { transport, signals } = await hangingOn(shown);
    shown.turn("hidden");
    vi.setSystemTime(Date.now() + openTryMostMs); // frozen: the clock moves, no timer runs
    shown.turn("visible");
    await vi.advanceTimersByTimeAsync(0);
    expect(signals[0]?.aborted).toBe(true);
    expect(signals).toHaveLength(2);
    expect(signals[1]?.aborted).toBe(false);
    transport.close();
  });

  it("still gives up a try under way when the browser comes back online, however young", async () => {
    const shown = testPage();
    const online = onlineEvent();
    const first = new LostPeer();
    const signals: AbortSignal[] = [];
    const transport = new RtcTransport(
      [first],
      {
        open: (signal) => {
          signals.push(signal);
          return new Promise<PeerLike>(() => undefined);
        },
        size: 4,
        onOnline: (listener) => {
          const stopShown = onWayBack(shown.page)(listener);
          const stopOnline = online.onOnline(() => {
            listener("back");
          });
          return () => {
            stopShown();
            stopOnline();
          };
        },
        hidden: { page: shown.page },
      },
      () => Date.now(),
    );
    await vi.advanceTimersByTimeAsync(settledAfterMs);
    first.goes("its ICE failed");
    await vi.advanceTimersByTimeAsync(1_000);
    online.fire();
    await vi.advanceTimersByTimeAsync(0);
    expect(signals[0]?.aborted).toBe(true);
    expect(signals).toHaveLength(2);
    transport.close();
  });
});
