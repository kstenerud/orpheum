// SPDX-License-Identifier: MPL-2.0

import type { Api, BrowserVoiceDevices, OpenTogetherSocket, PlayableFormats } from "@orpheum/sdk";
import { createContext, use, useSyncExternalStore } from "react";

import type { Navigation } from "./navigation.ts";
import type { Route } from "./router.ts";
import type { Session } from "./session.ts";

/**
 * What every screen reaches the world through: the API, the session, navigation, what this browser
 * plays, the Watch Together socket, and voice's microphone and calls.
 */
export interface Services {
  readonly api: Api;
  readonly session: Session;
  readonly navigation: Navigation;
  readonly playableFormats: () => PlayableFormats;
  readonly openTogetherSocket: OpenTogetherSocket;
  /** Whether this browser can play in step with a Watch Together group (D250 §7). */
  readonly canFollowAGroup: () => boolean;
  /** Makes the microphone and calls a Watch Together person's voice goes through. */
  readonly voiceDevices: () => BrowserVoiceDevices;
  /**
   * Fetches what the server serves outside its API, a playlist or a subtitle's text, with the
   * media cookie: the browser's fetch, or a test's server, so no test reaches the network.
   */
  readonly fetchMedia: (url: string, init: RequestInit) => Promise<Response>;
  /**
   * The realm this page's person signed in to the server by at a paid discovery, shown in the
   * banner so they know which server they are on (D297 §5); undefined anywhere else.
   */
  readonly realm?: string | undefined;
  /**
   * The moves of the page's requests from one path to another, at a discovery's page: what the core
   * says this session may do depends on the path (an admin administers only from home, D297 §6), so
   * what was read on one path is read again on the next. Undefined where the page never moves.
   */
  readonly pathMoves?: PathMoves | undefined;
  /**
   * Goes to the discovery's list of households, under its loader (D308 §8); undefined on the core's
   * own origin, which has no list to go to.
   */
  readonly switchHousehold?: (() => void) | undefined;
}

/** How many times the page's requests moved path, and who is told of each move. */
export interface PathMoves {
  readonly count: () => number;
  /** The name of the path the last move went to, as the console says it; undefined before any. */
  readonly to: () => string | undefined;
  readonly subscribe: (listener: () => void) => () => void;
}

/** A count of path moves, and the move to tell of: what client-main.tsx keeps, and tests move by hand. */
export function createPathMoves(): PathMoves & { readonly moved: (to?: string) => void } {
  let count = 0;
  let last: string | undefined;
  const listeners = new Set<() => void>();
  return {
    count: () => count,
    to: () => last,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    moved: (to) => {
      count += 1;
      last = to;
      for (const listener of listeners) {
        listener();
      }
    },
  };
}

const neverMoves = (): number => 0;
const noMoves = (): (() => void) => () => undefined;

/** How many times the page's requests moved path, re-rendering at each move. */
export function usePathMoveCount(): number {
  const { pathMoves } = useServices();
  return useSyncExternalStore(
    pathMoves?.subscribe ?? noMoves,
    pathMoves?.count ?? neverMoves,
    pathMoves?.count ?? neverMoves,
  );
}

/** Holds the services for the tree below it; main.tsx provides the real ones, tests their own. */
export const ServicesContext = createContext<Services | undefined>(undefined);

/** The services, from the nearest provider. */
export function useServices(): Services {
  const services = use(ServicesContext);
  if (services === undefined) {
    throw new Error("a screen rendered outside ServicesContext");
  }
  return services;
}

/** The session's token, re-rendering when someone signs in or out. */
export function useToken(): string | undefined {
  const { session } = useServices();
  return useSyncExternalStore(session.subscribe, session.token, session.token);
}

/** The current route, re-rendering when it changes. */
export function useRoute(): Route {
  const { navigation } = useServices();
  return useSyncExternalStore(navigation.subscribe, navigation.route, navigation.route);
}
