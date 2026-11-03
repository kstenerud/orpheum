// SPDX-License-Identifier: MPL-2.0

import type { Api, BrowserVoiceDevices, OpenTogetherSocket, PlayableFormats } from "@orpheum/sdk";
import { createContext, use, useSyncExternalStore } from "react";

import type { LinkDebug } from "./link-debug.ts";
import type { OrpheumLoader } from "./loader-interface.ts";
import type { Navigation } from "./navigation.ts";
import type { Route } from "./router.ts";
import type { Session } from "./session.ts";

/**
 * What every screen reaches the world through: the API, the session, navigation, what this browser
 * plays, the Watch Together socket, and voice's microphone and calls.
 */
/**
 * The page's core as its discovery finds it, which a device the page hands its session to reaches it
 * by from away: the lab's cast receiver (the 11-05 batch's item 10).
 */
export interface ThroughDiscovery {
  /** The discovery's client API, as the page names it. */
  readonly clientApi: string;
  readonly coreId: string;
  /** The core's identity key's fingerprint, which its answers are checked by. */
  readonly fingerprint: string;
}

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
   * The page's core as its discovery finds it, under a discovery's loader: what the lab's cast
   * receiver reaches it by from away, handed the page's session (the 11-05 batch's item 10);
   * undefined on the core's own origin.
   */
  readonly throughDiscovery?: ThroughDiscovery | undefined;
  /**
   * Whether the page, once signed out, is taken by its discovery's loader to the discovery's
   * sign-in, so it draws none of the server's own meanwhile (D308; the owner, 2026-10-30): undefined
   * on the core's own origin, where it never is.
   */
  readonly willLeaveWhenSignedOut?: boolean | undefined;
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
  /**
   * Where a device that knows no core asks for a code, as a person types it: the discovery loader's
   * host, "discovery.example", under its loader, whose service issues the code (D293 §2); undefined
   * on the core's own origin, which issues its own.
   */
  readonly codeAddress?: string | undefined;
  /**
   * Tells listener whether the browser waits on the person to allow the local network, under a
   * discovery's loader that tells it (webrtc-transport.md W2c item 8); returns its removal.
   * Undefined where nothing asks.
   */
  readonly onLocalNetworkAsking?:
    ((listener: (isAsking: boolean) => void) => () => void) | undefined;
  /**
   * Tells listener each time the way to the core may work again, so a session connection waiting to
   * connect again does at once (the SDK's TogetherConnectionOptions.pathBack): the page's WebRTC
   * transport's word under a loader that tells it, else the browser back online; returns its
   * removal. Undefined where nothing tells.
   */
  readonly onPathBack?: ((listener: () => void) => () => void) | undefined;
  /**
   * The tab's debug link, its cap and its drop, which a debug tool on the player sets (D380);
   * undefined where nothing shapes the page's link, as in a test that gives none.
   */
  readonly linkDebug?: LinkDebug | undefined;
}

/** How many times the page's requests moved path, and who is told of each move. */
export interface PathMoves {
  readonly count: () => number;
  /** The name of the path the last move went to, as the console says it; undefined before any. */
  readonly to: () => string | undefined;
  readonly subscribe: (listener: () => void) => () => void;
  /**
   * Whether the page's media go over WebRTC now, where the service worker builds every load's
   * answer and the browser's HTTP cache keeps none (the efficiency audit's finding 1); false where
   * nothing says, on the core's own origin and under an older loader.
   */
  readonly isOverWebrtc: () => boolean;
}

/** A count of path moves, and the move to tell of: what client-main.tsx keeps, and tests move by hand. */
export function createPathMoves(): PathMoves & { readonly moved: (to?: string) => void } {
  let count = 0;
  let last: string | undefined;
  const listeners = new Set<() => void>();
  return {
    count: () => count,
    to: () => last,
    isOverWebrtc: () => false,
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

/**
 * The path moves a loader tells of, with the name of the path each went to, for the screens that
 * read again on a move and the play report that logs it (the owner, 2026-10-17), and the kind of
 * path in use, where the loader says it (since its minor 2).
 */
export function pathMovesHeardFrom(
  onPathMove: OrpheumLoader["onPathMove"],
  pathInUse?: OrpheumLoader["pathInUse"],
): PathMoves {
  const pathMoves = createPathMoves();
  onPathMove((to) => {
    pathMoves.moved(to);
  });
  return {
    ...pathMoves,
    isOverWebrtc: () => pathInUse?.() === "webrtc",
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
