// SPDX-License-Identifier: MPL-2.0

/**
 * How the app reaches its core: on its own origin, the browser's way; under a discovery's loader
 * (D308), the loader's, whose every part the app takes from it here.
 */

import { readMediaCarrierFrom, type OpenTogetherSocket } from "@orpheum/sdk";

import { builtScriptOf } from "./app-update.ts";
import { discoveryClientApiOf } from "./discovery-page.ts";
import { loaderConfigOf } from "./loader-config.ts";
import type { OrpheumLoader } from "./loader-interface.ts";
import { pathMovesHeardFrom, type PathMoves, type ThroughDiscovery } from "./services.tsx";

/** How the app reaches its core: what its requests are sent by, and the session socket's opener. */
export interface Reaches {
  readonly fetch: (request: Request) => Promise<Response>;
  readonly openTogetherSocket: OpenTogetherSocket;
  readonly pathMoves: PathMoves;
  /** The page's own client script, which the core's answers are compared with for an update. */
  readonly ownScript: string | undefined;
  readonly realm?: string | undefined;
  /** The page's core as its discovery finds it (Services.throughDiscovery). */
  readonly throughDiscovery?: ThroughDiscovery | undefined;
  /** Whether the loader takes a page signed out to the discovery's sign-in (Services'). */
  readonly willLeaveWhenSignedOut?: boolean | undefined;
  readonly switchHousehold?: (() => void) | undefined;
  readonly codeAddress?: string | undefined;
  readonly onLocalNetworkAsking?:
    ((listener: (isAsking: boolean) => void) => () => void) | undefined;
  /** Tells listener each time the way to the core may work again (Services.onPathBack). */
  readonly onPathBack: (listener: () => void) => () => void;
}

/** Tells listener each time the browser comes back online; returns its removal. */
export function onBrowserOnline(listener: () => void): () => void {
  window.addEventListener("online", listener);
  return () => {
    window.removeEventListener("online", listener);
  };
}

/**
 * Where a TV asks for a code under a discovery's loader: the loader's host, from the page the loader
 * served this client in, so a remote types it (the owner, 2026-10-20: the client origin's 26
 * characters are not); undefined when the page names none.
 */
function codeAddressOf(page: Document): string | undefined {
  const origin = loaderConfigOf(page)?.loaderOrigin;
  return origin === undefined ? undefined : new URL(origin).host;
}

/** The core as the discovery at clientApi finds it, or undefined where the page names no discovery. */
function throughDiscoveryOf(
  core: OrpheumLoader["core"],
  clientApi: string,
): ThroughDiscovery | undefined {
  return clientApi === ""
    ? undefined
    : { clientApi, coreId: core.coreId, fingerprint: core.fingerprint };
}

/**
 * Has the player name what carries each media load by loader's word, which carries the media in its
 * own copy of the SDK: the client's copy carries none, so said "http" of every chunk given up over
 * WebRTC (the Fire TV through the discovery on staging, 2026-10-29). An older loader, which tells
 * nothing, leaves the client's own.
 */
export function takeMediaCarriers(loader: Pick<OrpheumLoader, "mediaCarrierOf">): void {
  if (loader.mediaCarrierOf !== undefined) {
    readMediaCarrierFrom(loader.mediaCarrierOf);
  }
}

/**
 * How the app reaches its core under loader, in page: its requests, socket and path moves the
 * loader's, whether the browser asks for the local network, which the loader tells (W2c item 8), and
 * when the way to the core may work again.
 */
export function reachesUnderLoader(loader: OrpheumLoader, page: Document): Reaches {
  return {
    fetch: loader.fetch,
    openTogetherSocket: loader.openTogetherSocket,
    pathMoves: pathMovesHeardFrom(loader.onPathMove, loader.pathInUse),
    ownScript: builtScriptOf(page),
    realm: loader.core.realm,
    throughDiscovery: throughDiscoveryOf(loader.core, discoveryClientApiOf(page)),
    // An older loader says nothing, and leaves for a core reached through a realm alone.
    willLeaveWhenSignedOut: loader.willLeaveWhenSignedOut ?? loader.core.realm !== undefined,
    switchHousehold: loader.switchHousehold,
    codeAddress: codeAddressOf(page),
    onLocalNetworkAsking: loader.onLocalNetworkAsking,
    // An older loader tells nothing of its path: the browser back online is all the client hears.
    onPathBack: loader.onPathBack ?? onBrowserOnline,
  };
}
