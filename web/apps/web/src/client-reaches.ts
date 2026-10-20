// SPDX-License-Identifier: MPL-2.0

/**
 * How the app reaches its core: on its own origin, the browser's way; under a discovery's loader
 * (D308), the loader's, whose every part the app takes from it here.
 */

import type { OpenTogetherSocket } from "@orpheum/sdk";

import { builtScriptOf } from "./app-update.ts";
import { loaderConfigOf } from "./loader-config.ts";
import type { OrpheumLoader } from "./loader-interface.ts";
import { pathMovesHeardFrom, type PathMoves } from "./services.tsx";

/** How the app reaches its core: what its requests are sent by, and the session socket's opener. */
export interface Reaches {
  readonly fetch: (request: Request) => Promise<Response>;
  readonly openTogetherSocket: OpenTogetherSocket;
  readonly pathMoves: PathMoves;
  /** The page's own client script, which the core's answers are compared with for an update. */
  readonly ownScript: string | undefined;
  readonly realm?: string | undefined;
  readonly switchHousehold?: (() => void) | undefined;
  readonly codeAddress?: string | undefined;
  readonly onLocalNetworkAsking?:
    ((listener: (isAsking: boolean) => void) => () => void) | undefined;
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

/**
 * How the app reaches its core under loader, in page: its requests, socket and path moves the
 * loader's, and whether the browser asks for the local network, which the loader tells (W2c item 8).
 */
export function reachesUnderLoader(loader: OrpheumLoader, page: Document): Reaches {
  return {
    fetch: loader.fetch,
    openTogetherSocket: loader.openTogetherSocket,
    pathMoves: pathMovesHeardFrom(loader.onPathMove),
    ownScript: builtScriptOf(page),
    realm: loader.core.realm,
    switchHousehold: loader.switchHousehold,
    codeAddress: codeAddressOf(page),
    onLocalNetworkAsking: loader.onLocalNetworkAsking,
  };
}
