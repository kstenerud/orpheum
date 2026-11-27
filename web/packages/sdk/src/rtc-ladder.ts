// SPDX-License-Identifier: MPL-2.0

/**
 * The ladder a WebRTC connection whose ICE servers name a TURN server climbs (D559 §1, D561 §2):
 * an attempt without a relay, then one with the relays' UDP URLs alone, then one with them all, each
 * started beside those before it once they are late or failed; the first the core holds is taken.
 * rtc-connect.ts makes each attempt.
 */

import {
  hasRelayBesidesUdp,
  hasUdpRelayServer,
  withoutRelayServers,
  withUdpRelaysAlone,
} from "./rtc-candidates.ts";
import type { PeerLike } from "./rtc-transport.ts";
import { noteStep } from "./steps.ts";

/**
 * How long a rung of a connection's ladder may go unconnected before the next is tried beside it
 * (D559 §1, D561 §2): the attempt without a relay from the core's answer, one asking a relay from its
 * start. Nearly every connection connects without a relay, and the hosted relay has few ports, so
 * each connection asking every TURN server for each of its interfaces at once held allocations it
 * never used.
 */
export const relayAfterMs = 3_000;

/** Why an attempt was given up: another attempt of its connection connected first (D559 §1). */
export const lostToAnotherAttempt = "another attempt of the connection connected first";

/** What an attempt at a connection is given: its abort, and what it tells as the core answers. */
export interface AttemptHooks {
  readonly signal: AbortSignal;
  readonly onAnswered?: () => void;
}

/**
 * What a climb asks of the connection it makes: an attempt on a configuration, whether a failure is
 * the core turning the connection away (which no relay mends), and the connection's abort.
 */
export interface Climbing {
  readonly attempt: (rtcConfig: RTCConfiguration, hooks: AttemptHooks) => Promise<PeerLike>;
  readonly isTurnedAway: (failure: unknown) => boolean;
  readonly signal: AbortSignal;
  /**
   * The configuration with relay credentials good now, fetched again where stale (RelayCredentials,
   * D564 §3), asked before each rung that asks a relay; absent, the climb's own is used throughout.
   */
  readonly fresh?: (() => Promise<RTCConfiguration>) | undefined;
}

/**
 * One rung of a connection's ladder (D561 §2): the configuration its attempt asks, the relay it asks
 * in words (none for the first), and the need it stands for, which a page whose connection connected
 * by it starts its later ones at: 0 asks no relay, 1 the relay over UDP alone, 2 its every address.
 */
interface Rung {
  readonly config: RTCConfiguration;
  /** The rung's configuration out of another, its credentials fetched again (D564 §3). */
  readonly configOf: (rtcConfig: RTCConfiguration) => RTCConfiguration;
  readonly relay?: string;
  readonly need: number;
}

/**
 * The rungs a connection on rtcConfig, which names a TURN server, climbs (D561 §2): no relay; the
 * relays' UDP URLs alone, where it has one; the whole configuration, where it names a relay by a URL
 * that is not UDP. A phone with three addresses allocated six times a connection at the hosted
 * relay, which it names by a UDP and a TLS URL, and four connections passed its limit of 16 a
 * credential, the TLS allocations carrying nothing (production, 2026-11-26).
 */
function ladderOf(rtcConfig: RTCConfiguration): readonly Rung[] {
  return rungsOf(rtcConfig).map((rung) => ({ ...rung, config: rung.configOf(rtcConfig) }));
}

/** ladderOf's rungs, each with how its configuration is made out of the whole one. */
function rungsOf(rtcConfig: RTCConfiguration): readonly Omit<Rung, "config">[] {
  return [
    { configOf: withoutRelayServers, need: 0 },
    ...(hasUdpRelayServer(rtcConfig)
      ? [{ configOf: withUdpRelaysAlone, relay: "the relay over UDP", need: 1 }]
      : []),
    ...(hasRelayBesidesUdp(rtcConfig)
      ? [{ configOf: everyServer, relay: "the relay's every address", need: 2 }]
      : []),
  ];
}

/** The whole configuration, every relay in it: the last rung's. */
function everyServer(rtcConfig: RTCConfiguration): RTCConfiguration {
  return rtcConfig;
}

/** How a rung's attempt is told in steps: "without the relay", "with the relay over UDP". */
function askingText(rung: Rung): string {
  return rung.relay === undefined ? "without the relay" : `with ${rung.relay}`;
}

/**
 * The need of the rung this page's network last connected through (Rung's need): its later
 * connections start there (D559 §1, D561 §2). Kept for the page's life.
 */
let rungNeededHere = 0;

/** Forgets which relay this page's network needed, as a test starting afresh does. */
export function forgetRelayNeeded(): void {
  rungNeededHere = 0;
}

/**
 * Connects on rtcConfig, which names a TURN server, climbing its ladder (ladderOf) from the rung this
 * page's network last needed (D559 §1, D561 §2): the next rung's attempt starts beside the others
 * once the one before it has not connected relayAfterMs after the core's answer, for the rung that
 * asks no relay, or after its own start, for one that asks a relay (on a network that blocks UDP its
 * gathering finds nothing and may take long before its offer goes, and must not hold the next rung
 * back); or at once when that one fails, but for a failure the core turned away, or, on the rung
 * that asks no relay, one before the core answered (its offer reached no core): a relay mends
 * neither. The first attempt the core holds is the connection, and every other is given up; where
 * all fail, the last rung tried tells the failure, as it tried the most ways.
 */
export async function climbLadder(
  rtcConfig: RTCConfiguration,
  climbing: Climbing,
): Promise<PeerLike> {
  const ladder = ladderOf(rtcConfig);
  const needed = ladder.findIndex((rung) => rung.need >= rungNeededHere);
  const first = needed === -1 ? ladder.length - 1 : needed;
  const relay = ladder[first]?.relay;
  if (relay !== undefined) {
    noteStep(`WebRTC: this page's network needed ${relay}; asking it at once`);
  }
  return await new Promise<PeerLike>((resolve, reject) => {
    new LadderClimb(ladder.slice(first), climbing, { resolve, reject }).start(0);
  });
}

/** How a climb tells its end: the connection, or the failure of the last rung tried. */
interface ClimbEnd {
  readonly resolve: (peer: PeerLike) => void;
  readonly reject: (failure: Error) => void;
}

/** One rung's attempt under way: its abort, how far it got, and how it ended. */
class RungTry {
  readonly controller = new AbortController();
  /** Whether the next rung's start is decided, made or not: never twice. */
  isClimbed: boolean;
  isAnswered = false;
  isSettled = false;
  failure: unknown;
  #lateTimer: ReturnType<typeof setTimeout> | undefined;

  constructor(
    readonly rung: Rung,
    isLast: boolean,
  ) {
    this.isClimbed = isLast;
  }

  /** Has late told relayAfterMs from now, unless the next rung's start is decided by then. */
  startLateTimer(late: () => void): void {
    this.#lateTimer = setTimeout(late, relayAfterMs);
  }

  stopClimbing(): void {
    this.isClimbed = true;
    clearTimeout(this.#lateTimer);
  }

  settle(failure?: unknown): void {
    this.isSettled = true;
    this.failure = failure;
  }
}

/** One connection's climb up its ladder, as climbLadder says. */
class LadderClimb {
  readonly #tries: RungTry[] = [];
  #isDone = false;
  readonly #passOnAbort = (): void => {
    this.#tries.forEach((each) => {
      each.controller.abort(this.climbing.signal.reason);
    });
  };

  constructor(
    readonly ladder: readonly Rung[],
    readonly climbing: Climbing,
    readonly end: ClimbEnd,
  ) {
    climbing.signal.addEventListener("abort", this.#passOnAbort, { once: true });
  }

  /** Starts the attempt of the rung at index, and its late timer where it asks a relay. */
  start(index: number): void {
    const rung = this.ladder[index];
    if (rung === undefined) {
      return;
    }
    const each = new RungTry(rung, index + 1 >= this.ladder.length);
    this.#tries.push(each);
    const late = (): void => {
      const after = rung.relay === undefined ? "the answer" : `trying ${askingText(rung)}`;
      this.#climb(each, index, `not connected ${String(relayAfterMs / 1000)} s after ${after}`);
    };
    if (rung.relay !== undefined) {
      each.startLateTimer(late);
    }
    const hooks: AttemptHooks = {
      signal: each.controller.signal,
      onAnswered: () => {
        each.isAnswered = true;
        if (rung.relay === undefined) {
          each.startLateTimer(late);
        }
      },
    };
    const { fresh } = this.climbing;
    const attempted =
      rung.relay === undefined || fresh === undefined
        ? this.climbing.attempt(rung.config, hooks)
        : fresh().then((config) => {
            each.controller.signal.throwIfAborted();
            return this.climbing.attempt(rung.configOf(config), hooks);
          });
    attempted.then(
      (peer) => {
        this.#connected(each, peer);
      },
      (failure: unknown) => {
        this.#failed(each, index, failure);
      },
    );
  }

  /** Starts the rung after each's, at index, saying why, unless that is decided or the climb over. */
  #climb(each: RungTry, index: number, why: string): void {
    const next = this.ladder[index + 1];
    if (each.isClimbed || this.#isDone || this.climbing.signal.aborted || next === undefined) {
      return;
    }
    each.stopClimbing();
    noteStep(`WebRTC: ${why}; trying ${askingText(next)} too`);
    this.start(index + 1);
  }

  /** Takes each's peer as the connection, giving every other attempt up, unless one was taken. */
  #connected(each: RungTry, peer: PeerLike): void {
    each.settle();
    if (this.#isDone) {
      peer.close();
      return;
    }
    this.#finish();
    this.#tries.forEach((other) => {
      if (other !== each) {
        other.controller.abort(lostToAnotherAttempt);
      }
    });
    rungNeededHere = Math.max(rungNeededHere, each.rung.need);
    if (this.#tries.length > 1) {
      const { relay } = each.rung;
      const through = relay === undefined ? "without the relay" : `that asked ${relay}`;
      noteStep(`WebRTC: connected through the attempt ${through}`);
    }
    this.end.resolve(peer);
  }

  /** Climbs on from each's failure where a relay may mend it; fails the climb once all failed. */
  #failed(each: RungTry, index: number, failure: unknown): void {
    each.settle(failure);
    const canRelayMend =
      (each.isAnswered || each.rung.relay !== undefined) &&
      !this.climbing.isTurnedAway(failure) &&
      !each.controller.signal.aborted;
    if (canRelayMend) {
      this.#climb(each, index, `failed ${askingText(each.rung)}`);
    }
    each.stopClimbing();
    if (!this.#isDone && this.#tries.every((other) => other.isSettled)) {
      this.#finish();
      const told: unknown = this.#tries.at(-1)?.failure ?? failure;
      this.end.reject(told instanceof Error ? told : new Error(String(told)));
    }
  }

  #finish(): void {
    this.#isDone = true;
    this.#tries.forEach((each) => {
      each.stopClimbing();
    });
    this.climbing.signal.removeEventListener("abort", this.#passOnAbort);
  }
}
