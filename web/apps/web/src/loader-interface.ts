// SPDX-License-Identifier: MPL-2.0

/**
 * What the discovery's loader hands the core's client it runs (D308): one object on the window,
 * since the loader and the client are built apart, the one shipped with the discovery and the other
 * with each core, and so may be of different releases. Its shape is a protocol, as D273 §6's is: a
 * change to it is additive and raises `minor`; one that is not raises `major`, and the loader keeps
 * answering the major before it for D273 §7's window. A client that needs a major the loader lacks
 * says so, rather than failing obscurely.
 */

import type { FetchRequest, MediaCredentials, OpenTogetherSocket } from "@orpheum/sdk";

import type { Session } from "./session.ts";

/** The loader interface's major version: the client refuses a loader of another. */
export const loaderMajor = 1;

/** The loader interface's minor version: what this source adds within the major. */
export const loaderMinor = 0;

/** The core the loader reached: its id, the fingerprint it holds for it, and the realm, when any. */
export interface LoadedCore {
  readonly coreId: string;
  readonly fingerprint: string;
  readonly realm?: string | undefined;
}

/** The loader, as the client it runs finds it on the window. */
export interface OrpheumLoader {
  readonly major: number;
  readonly minor: number;
  readonly core: LoadedCore;
  /** Sends the client's API requests to the core, over whichever path reaches it now. */
  readonly fetch: FetchRequest;
  /** Opens the Watch Together socket over the path in use. */
  readonly openTogetherSocket: OpenTogetherSocket;
  /**
   * Tells listener each time the requests move from one path to another, with the name of the one
   * they went to (an older loader names none); returns its removal.
   */
  readonly onPathMove: (listener: (to?: string) => void) => () => void;
  /**
   * The session at this client origin, which the loader keeps, so the two never disagree about
   * where it is stored: the client signs in and out through it alone.
   */
  readonly session: Session;
  /**
   * Names what the media loads the loader carries for the client are signed in with: the session's
   * token by default, and a Watch Together viewer's membership beside it once the client says.
   */
  readonly setMediaCredentials: (credentials: () => MediaCredentials) => void;
  /** Goes to the discovery's list of households, this one among them (D308 §8). */
  readonly switchHousehold: () => void;
}

/** The window property the loader sets before it runs the client. */
export const loaderProperty = "orpheumLoader";

/** Whether found has the loader's shape: its versions, its core, and what it does. */
function isLoader(found: unknown): found is OrpheumLoader {
  if (typeof found !== "object" || found === null) {
    return false;
  }
  const field = (name: string): unknown => Reflect.get(found, name);
  return (
    typeof field("major") === "number" &&
    typeof field("minor") === "number" &&
    typeof field("core") === "object" &&
    typeof field("session") === "object" &&
    ["fetch", "openTogetherSocket", "onPathMove", "setMediaCredentials", "switchHousehold"].every(
      (name) => typeof field(name) === "function",
    )
  );
}

/** The loader on window, when a loader runs this page; undefined on the core's own origin. */
export function loaderOf(window: object): OrpheumLoader | undefined {
  const found: unknown = Reflect.get(window, loaderProperty);
  return isLoader(found) ? found : undefined;
}

/** The loader's major a client needs and the one it found, when they differ. */
export interface LoaderRefusal {
  readonly needs: number;
  readonly has: number;
}

/** Why a client cannot run under the loader it found, or undefined when it can. */
export function loaderRefusal(loader: Pick<OrpheumLoader, "major">): LoaderRefusal | undefined {
  return loader.major === loaderMajor ? undefined : { needs: loaderMajor, has: loader.major };
}
