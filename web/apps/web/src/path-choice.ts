// SPDX-License-Identifier: MPL-2.0

/**
 * Why a page served by a discovery took the path it took to its core, told to the core's log once per
 * reach: the browser, whether it uses the LAN name, its local-network permission, the names the
 * discovery offered, what came of each in the first race, the path taken and, over WebRTC, the
 * connection's candidate pairs as the browser sees them. The owner's playback at home went out through
 * the carrier and back (2026-10-26), and nothing from the page said why
 * (local-path-2026-10-27.md §4 items 1, 2).
 */

import {
  createApi,
  type CandidatePairSeen,
  type DirectOutcome,
  type FetchRequest,
} from "@orpheum/sdk";

import { usesLanName, type LocalNetworkState } from "./local-network.ts";

/** A browser's family, as the page judges it from its user agent, for the log alone. */
export type BrowserFamily = "firefox" | "chrome" | "safari" | "other";

/** The family of a browser of userAgent. */
export function browserFamily(userAgent: string): BrowserFamily {
  if (/\bFirefox\//u.test(userAgent)) {
    return "firefox";
  }
  // Headless Chromium says HeadlessChrome, with no word break before Chrome (the core's log, 2026-11-01).
  if (/\b(?:HeadlessChrome|Chrome|CriOS|Edg)\//u.test(userAgent)) {
    return "chrome";
  }
  return /\bSafari\//u.test(userAgent) ? "safari" : "other";
}

/** How the page chose its first path to the core, and why. */
export interface PathChoice {
  readonly browser: BrowserFamily;
  /** Whether the page tries the core's LAN name in this browser at all (usesLanName). */
  readonly isLanNameUsed: boolean;
  readonly localNetwork: LocalNetworkState;
  readonly directOffered: number;
  readonly lanOffered: number;
  /** Each name the discovery offered, and what came of it: its race's outcome, or why it was left out. */
  readonly names: readonly { readonly name: string; readonly outcome: string }[];
  /** "webrtc", or "direct " and the origin that answered. */
  readonly reached: string;
}

/** The most names a choice tells, and the longest each name and outcome is: the API's bounds. */
const namesMost = 16;
const wordsMost = 300;

/** What a page's first race was, as pathChoiceOf reads it. */
export interface FirstRace {
  /** The core's names the discovery offered, in its order, and those of them that are LAN names. */
  readonly directUrls: readonly string[];
  readonly lanUrls: readonly string[];
  /** What came of each name raced (firstDirectPath's tell). */
  readonly raced: readonly DirectOutcome[];
  readonly localNetwork: LocalNetworkState;
  readonly userAgent: string;
  /** The path taken: a name's origin, or undefined for WebRTC. */
  readonly directOrigin: string | undefined;
}

/** Why a LAN name was left out of the first race: the browser's refusal, or its permission. */
function leftOutBecause(isLanNameUsed: boolean, localNetwork: LocalNetworkState): string {
  if (!isLanNameUsed) {
    return "left out: this browser fails a page's requests to a LAN name";
  }
  if (localNetwork === "prompt") {
    return "left out: asked apart, to raise the browser's local-network prompt";
  }
  return `left out: the local-network permission is ${localNetwork}`;
}

/** A raced name's outcome, with its time where it answered or failed (D414 §4); unraced, why. */
function outcomeOf(raced: DirectOutcome | undefined, unraced: string): string {
  if (raced === undefined) {
    return unraced;
  }
  return raced.afterMs === undefined
    ? raced.outcome
    : `${raced.outcome} (${String(raced.afterMs)} ms)`;
}

/**
 * How the page chose its first path, from its race: what came of each name offered, its outcome where
 * raced, else why it was left out.
 */
export function pathChoiceOf(race: FirstRace): PathChoice {
  const isLanNameUsed = usesLanName(race.userAgent);
  const leftOut = leftOutBecause(isLanNameUsed, race.localNetwork);
  return {
    browser: browserFamily(race.userAgent),
    isLanNameUsed,
    localNetwork: race.localNetwork,
    directOffered: race.directUrls.length,
    lanOffered: race.lanUrls.length,
    names: race.directUrls.slice(0, namesMost).map((name) => ({
      name: name.slice(0, wordsMost),
      outcome: outcomeOf(
        race.raced.find((each) => each.url === name),
        race.lanUrls.includes(name) ? leftOut : "not raced",
      ).slice(0, wordsMost),
    })),
    reached: (race.directOrigin === undefined ? "webrtc" : `direct ${race.directOrigin}`).slice(
      0,
      wordsMost,
    ),
  };
}

/**
 * How long after a page reached its core it tells how: over WebRTC, the pair to the core's LAN address
 * may still be checking when another connects first, and its state is what says whether a device in
 * the house reached the core's LAN UDP port; and a page handed off by a realm sign-in takes its
 * session in the meantime.
 */
export const pairsSettleMs = 5_000;

/** What reportPathChoice tells: the choice, over WebRTC the transport whose pairs go with it. */
export interface PathChoiceTold {
  /** The session's token once the wait is over; nothing is sent without one. */
  readonly token: () => string | undefined;
  /**
   * The choice, read once the wait is over, as a race WebRTC won may end after the page took it
   * (D414 §4); undefined where the page has none to tell.
   */
  readonly choice: () => PathChoice | undefined;
  readonly transport?:
    { readonly candidatePairs: () => Promise<readonly CandidatePairSeen[]> } | undefined;
}

/**
 * Tells the core's log how the page chose its path, once pairsSettleMs have passed, sent over send
 * with the session's token then: over WebRTC with the candidate pairs of the transport's connection.
 * Nothing is sent from a page with no session by then, or no choice; a failure to send is the caller's
 * to drop.
 */
export async function reportPathChoice(
  send: FetchRequest,
  told: PathChoiceTold,
  wait: (ms: number) => Promise<void> = (ms) =>
    new Promise((resolve) => {
      setTimeout(resolve, ms);
    }),
): Promise<void> {
  await wait(pairsSettleMs);
  const held = told.token();
  const choice = told.choice();
  if (held === undefined || choice === undefined) {
    return;
  }
  const webrtcPairs = (await told.transport?.candidatePairs()) ?? [];
  const api = createApi({ getToken: () => held, fetch: send });
  await api.POST("/client-paths", {
    body: {
      ...choice,
      names: [...choice.names],
      ...(webrtcPairs.length === 0 ? {} : { webrtcPairs: [...webrtcPairs] }),
    },
  });
}
