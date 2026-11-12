// SPDX-License-Identifier: MPL-2.0

/**
 * How a Cast receiver reaches the core of the page that cast to it, from away: under a discovery's
 * loader, the core as the discovery finds it and the session's locator (the 11-05 batch's item 10).
 * The lab's probe hands it with the page's sign-in; the Cast menu's Chromecast hands it with a grant
 * for one cast alone (D465 §2), the receiver getting its own credential by trading it.
 */

import type { ThroughDiscovery } from "./services.tsx";

/**
 * The page's discovery as the receiver is told it: the core as the discovery finds it, and this
 * page's origin, the core's client origin, the one page origin besides the core's own names that the
 * core lets reach it (D308 §4), so the receiver goes direct only when it is served there too (the
 * owner, 2026-11-11: from the loader's origin, over WebRTC only).
 */
export interface ReceiverDiscovery extends ThroughDiscovery {
  readonly clientOrigin: string;
}

/** throughDiscovery as the receiver is told it, from a page at origin; undefined without one. */
export function receiverDiscoveryOf(
  throughDiscovery: ThroughDiscovery | undefined,
  origin: string,
): ReceiverDiscovery | undefined {
  return throughDiscovery === undefined ? undefined : { ...throughDiscovery, clientOrigin: origin };
}

/** How a receiver finds the page's core from away: its discovery and the session's locator. */
export interface ReceiverReach extends ReceiverDiscovery {
  readonly locator: string;
}

/**
 * How a receiver reaches the page's core from away through discovery, by the session's locator;
 * undefined on the core's own origin, where there is no discovery, or with no locator, signed out.
 */
export function receiverReachOf(
  discovery: ReceiverDiscovery | undefined,
  locator: string | undefined,
): ReceiverReach | undefined {
  return discovery === undefined || locator === undefined ? undefined : { ...discovery, locator };
}
