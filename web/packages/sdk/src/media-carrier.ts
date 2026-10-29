// SPDX-License-Identifier: MPL-2.0

/**
 * What carries each load of the page's media, named for a log: a stuck or failed chunk's note says
 * which WebRTC connection it waited on, or the other origin, or HTTP (the owner, 2026-10-29: the logs
 * are to show whether stalls are per-connection). Kept apart from the media's path (rtc-media.ts),
 * which sets the path's name.
 */

import type { CarrierSeen } from "./rtc-transport.ts";

/**
 * The path this page's media goes over, named; undefined while the page carries none, its loads the
 * browser's own over HTTP.
 */
let mediaPathName: string | undefined;

/** The path this page's media goes over from now, named (carryMediaOver). */
export function carriesMediaOver(name: string): void {
  mediaPathName = name;
}

/** The path this page's media goes over, named; undefined while the page carries none. */
export function mediaPathNow(): string | undefined {
  return mediaPathName;
}

/** The WebRTC connection carrying a load of the page's media, where its transport says (tellMediaCarriers). */
let mediaCarriers: (url: string) => CarrierSeen | undefined = () => undefined;

/**
 * What answers mediaCarrierOf in place of this SDK's own state, where another copy of it carries the
 * page's media: a discovery's loader runs its own bundle, so the state carryMediaOver and
 * tellMediaCarriers set is in its copy, never the client's.
 */
let carrierLookup: ((url: string) => string) | undefined;

/**
 * Whether mediaCarrierOf is asking carrierLookup now: a lookup that comes back to this copy is
 * answered from its own state, never by the lookup again.
 */
let isLookingUp = false;

/**
 * Has mediaCarrierOf answer by lookup, the copy of this SDK that carries the page's media (a
 * discovery's loader's, D308). Left as it was, the client under a loader said "http" of every chunk
 * it gave up, though WebRTC carried it (the Fire TV through the discovery on staging, 2026-10-29).
 * Where the loader and the client are one build, as at a core's client origin, they share this
 * copy, and lookup is this copy's own mediaCarrierOf: asked again by itself, it answered by lookup
 * until the stack ran out, inside hls.js's handling of each chunk that failed to load, which made
 * that error hls.js's fatal "Maximum call stack size exceeded" (the owner's Chrome offline, staging,
 * 2026-10-29; VQ133). A lookup that comes back here is answered from this copy's own state.
 */
export function readMediaCarrierFrom(lookup: (url: string) => string): void {
  carrierLookup = lookup;
}

/**
 * Has the page's media name the WebRTC connection carrying each load by carrierOf, the transport's
 * (RtcTransport.carrierOf) on the path in use, for mediaCarrierOf.
 */
export function tellMediaCarriers(carrierOf: (url: string) => CarrierSeen | undefined): void {
  mediaCarriers = carrierOf;
}

/**
 * Which transport, and which WebRTC connection of it, carries the load of url now, for a log: "http"
 * where the page carries no media itself; "over the core's name, …" where its path is another
 * origin; "WebRTC connection 2, up 41 s, 3 under way, last bytes 5512 ms ago" where a WebRTC
 * connection carries it. A chunk stuck on its way said nothing of which connection it waited on
 * (the owner, 2026-10-29: the logs are to show whether stalls are per-connection).
 */
export function mediaCarrierOf(url: string): string {
  if (carrierLookup !== undefined && !isLookingUp) {
    isLookingUp = true;
    try {
      return carrierLookup(url);
    } finally {
      isLookingUp = false;
    }
  }
  if (mediaPathName === undefined) {
    return "http";
  }
  const carrier = mediaCarriers(url);
  return carrier === undefined ? `over ${mediaPathName}` : carrierText(carrier);
}

/**
 * A WebRTC connection carrying a load, for a log: "WebRTC connection 2, up 41 s, 3 under way, last
 * bytes 5512 ms ago", or "no bytes yet" before any came over it, and why the transport ended the
 * load itself where it did ("; ended: nothing came for 40000 ms, …").
 */
export function carrierText(carrier: CarrierSeen): string {
  const delivered =
    carrier.sinceDeliveredMs === undefined
      ? "no bytes yet"
      : `last bytes ${String(Math.round(carrier.sinceDeliveredMs))} ms ago`;
  return (
    `WebRTC connection ${String(carrier.connection)}, up ${String(Math.round(carrier.upMs / 1000))} s,` +
    ` ${String(carrier.underWay)} under way, ${delivered}` +
    (carrier.ended === undefined ? "" : `; ended: ${carrier.ended}`)
  );
}
