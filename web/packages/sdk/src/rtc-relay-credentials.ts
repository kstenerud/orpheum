// SPDX-License-Identifier: MPL-2.0

/**
 * The relay credentials a page's WebRTC transport holds, fetched again before an attempt that asks a
 * relay would offer ones past their life (D564 §3). The page reads its ICE servers once, at load, and
 * a client's relay credential lasts fifteen minutes (plans/hosted-relay.md §1, "Lifetimes"; the
 * core's own relay's alike, turn.IssueFound): a page whose network changed later, onto one only the
 * relay could carry, offered stale ones, the relay refused its allocations, and it could not
 * reconnect (the owner's phone, production, 2026-11-26). Only a rung that asks a relay waits on the
 * fetch: the attempt without one goes as before (D561 §2).
 */

import { noteStep } from "./steps.ts";

/**
 * How long a relay credential lasts where its username states no expiry: a client's, made in each
 * `found` answer (plans/hosted-relay.md §1, D564 §3).
 */
export const relayCredentialLifeMs: number = 15 * 60_000;

/**
 * How long before its expiry a credential counts as stale: an attempt's gathering and the relay's
 * allocation take a few seconds, and a device's clock may stand a little off the discovery's
 * (D564 §3).
 */
export const relayCredentialMarginMs = 60_000;

/** Fetches the ICE servers again, through the discovery that named them. */
export type IceServersAgain = () => Promise<RTCIceServer[]>;

/**
 * When, in Unix milliseconds, the earliest of servers' relay credentials expires: a TURN REST
 * username begins with its expiry in Unix seconds (`<expiry>:…`, plans/hosted-relay.md §1; the
 * core's own relay's `<expiry>:found`); where none states one, relayCredentialLifeMs after
 * fetchedAtMs.
 */
export function relayExpiryOf(servers: readonly RTCIceServer[], fetchedAtMs: number): number {
  const stated = servers.flatMap((server) => {
    const match = /^(\d+):/u.exec(server.username ?? "");
    return match?.[1] === undefined ? [] : [Number(match[1]) * 1000];
  });
  return stated.length > 0 ? Math.min(...stated) : fetchedAtMs + relayCredentialLifeMs;
}

/**
 * A transport's ICE servers, as last fetched, and fetched again through again where an attempt that
 * asks a relay finds them within relayCredentialMarginMs of their expiry (D564 §3). A fetch that
 * fails leaves those held, which the attempt offers as it did before; asks while one is under way
 * share it. nowMs is the wall clock, the one the credentials' expiry is stated on (Date.now where
 * absent, which a test's fake timers move).
 */
export class RelayCredentials {
  #config: RTCConfiguration;
  #expiresAtMs: number;
  #fetching: Promise<RTCConfiguration> | undefined;

  constructor(
    config: RTCConfiguration,
    readonly again: IceServersAgain | undefined,
    readonly nowMs: () => number = () => Date.now(),
  ) {
    this.#config = config;
    this.#expiresAtMs = relayExpiryOf(config.iceServers ?? [], nowMs());
  }

  /** The configuration as last fetched. */
  get config(): RTCConfiguration {
    return this.#config;
  }

  /** The configuration with credentials good for an attempt now: fetched again first where stale. */
  async fresh(): Promise<RTCConfiguration> {
    if (this.again === undefined || this.nowMs() < this.#expiresAtMs - relayCredentialMarginMs) {
      return this.#config;
    }
    this.#fetching ??= this.#fetch(this.again).finally(() => {
      this.#fetching = undefined;
    });
    return await this.#fetching;
  }

  async #fetch(again: IceServersAgain): Promise<RTCConfiguration> {
    try {
      const iceServers = await again();
      this.#config = { ...this.#config, iceServers };
      this.#expiresAtMs = relayExpiryOf(iceServers, this.nowMs());
      noteStep("WebRTC: the relay's credentials were stale; fetched them again");
    } catch (problem: unknown) {
      const why = problem instanceof Error ? problem.message : String(problem);
      noteStep(
        `WebRTC: could not fetch the relay's credentials again (${why}); offering those held`,
      );
    }
    return this.#config;
  }
}
