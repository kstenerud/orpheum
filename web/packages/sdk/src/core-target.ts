// SPDX-License-Identifier: MPL-2.0

/**
 * A way into a core this device reached before, through its discovery (the discovery protocol's
 * §3.2–§3.3): the core's paths, found by the device's locator, and a WebRTC connection through the
 * discovery whose offers are proven under the device's session and whose answers the core signed.
 * Shared by the web client and the lab's cast receiver, which reaches its sender's core alike (the
 * 11-05 batch's item 10).
 */

import {
  discoverySignaller,
  pathsOf,
  sessionOfferKey,
  type DiscoveryClient,
  type Families,
  type Held,
  type Proving,
} from "./discovery.ts";
import type { DirectOutcome } from "./direct-path.ts";
import { connectRtc, type Signaller } from "./rtc-connect.ts";
import type { RtcTransport } from "./rtc-transport.ts";

/**
 * The core this device last reached at this discovery: its id, its identity key's fingerprint, and
 * the realm its person signed in to it by, when they did, which the page shows (D297 §5).
 */
export interface KeptCore {
  readonly coreId: string;
  readonly fingerprint: string;
  readonly realm?: string | undefined;
}

/** A way into the core: its id, the fingerprint held for it, what the device holds, and its key. */
export interface Target {
  readonly coreId: string;
  readonly fingerprint: string;
  readonly held: Held;
  readonly key: Uint8Array<ArrayBuffer>;
  readonly iceServers: RTCIceServer[];
  /** The core's published names, tried before WebRTC (W2c). */
  readonly directUrls: readonly string[];
  /** Those of directUrls that are the core's LAN names. */
  readonly lanUrls: readonly string[];
  /**
   * The core's public names by family, and the family the discovery saw the page from, which a dead
   * end's cause is judged by (D398 §1); absent, the cause is unknown.
   */
  readonly families?: Families | undefined;
  /**
   * Told what came of each of the target's names once the first race is decided, before WebRTC is
   * tried: a dead end's cause is judged by it (D398 §1).
   */
  readonly tellRaced?: ((raced: readonly DirectOutcome[]) => void) | undefined;
  /**
   * What the device holds by the time a later offer is made, when it holds more than the target
   * began with: the session or membership a link's person gained (DiscoverySignalling.latest).
   */
  readonly latest?: (() => Promise<Proving | undefined>) | undefined;
  /**
   * What the device holds now, asked once the core refuses a later offer's proof, which is then sent
   * once more under it (DiscoverySignalling.fallback).
   */
  readonly fallback?: (() => Promise<Proving | undefined>) | undefined;
}

/**
 * What a device holds with a core it reached before: a signed-in session's token, or a Watch Together
 * guest's membership credential, which the core proves offers under alike, with the locator the
 * discovery finds the core by for it.
 */
export interface HeldSession {
  readonly core: KeptCore;
  readonly token: string;
  readonly locator: string;
}

/** What an offer made with held is sent and proven with: its locator, under its token's key. */
export async function provingOf(held: HeldSession): Promise<Proving> {
  return { held: { locator: held.locator }, key: await sessionOfferKey(held.token) };
}

/** Finds the remembered core by this device's locator; its offers are proven under its session. */
export async function targetOfSession(client: DiscoveryClient, held: HeldSession): Promise<Target> {
  const located = await pathsOf(client, { locator: held.locator });
  if (located.coreId !== held.core.coreId) {
    throw new Error("the discovery answered for another server than this device reached");
  }
  return {
    coreId: held.core.coreId,
    fingerprint: held.core.fingerprint,
    ...(await provingOf(held)),
    iceServers: located.iceServers,
    directUrls: located.directUrls,
    lanUrls: located.lanUrls,
    families: located.families,
  };
}

/**
 * The signaller of the target's offers through the discovery: each proven by what the device holds
 * by then (latest), sent once more under what it holds now when the core refuses it (fallback), and
 * its answer checked.
 */
export function signallerOf(client: DiscoveryClient, target: Target): Signaller {
  return discoverySignaller({
    clientApi: client.clientApi,
    coreId: target.coreId,
    fingerprint: target.fingerprint,
    held: target.held,
    key: target.key,
    latest: target.latest,
    fallback: target.fallback,
    fetch: client.fetch,
  });
}

/** Connects to the target through the discovery, its offers proven and its answers checked. */
export function connectThrough(client: DiscoveryClient, target: Target): Promise<RtcTransport> {
  return connectRtc(signallerOf(client, target), { iceServers: target.iceServers });
}
