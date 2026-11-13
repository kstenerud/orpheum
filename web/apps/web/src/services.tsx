// SPDX-License-Identifier: MPL-2.0

import type { Api, BrowserVoiceDevices, OpenTogetherSocket, PlayableFormats } from "@orpheum/sdk";
import { createContext, use, useSyncExternalStore } from "react";

import type { LinkDebug } from "./link-debug.ts";
import type { OrpheumLoader } from "./loader-interface.ts";
import type { Navigation } from "./navigation.ts";
import type { Route } from "./router.ts";
import type { Session } from "./session.ts";
import type { LiveOpenMark } from "./signed-out-watch.ts";

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

/**
 * Asks the core for a grant for one cast to the Google Cast device receiverName, its name as its owner
 * gave it (D465 §2: POST /me/casts/guest-grants); answers the grant, a secret valid a minute for one
 * use, never logged, kept or put in an address (D154 §6).
 */
export type AskGuestCastGrant = (receiverName: string) => Promise<string>;

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
   * The Google Cast receiver application the Cast menu's Chromecast launches, named by the loader the
   * page came from (D465 §5: staging's or production's, each at its own loader's /cast/); undefined
   * on the core's own origin, which names none, so no Chromecast is offered there.
   */
  readonly castReceiverAppId?: string | undefined;
  /**
   * Asks the core for a grant for one cast to the Google Cast device receiverName (D465 §2), which the
   * Cast menu's Chromecast hands that device over the Cast channel and nowhere else
   * (google-cast.ts's askGuestCastGrantOf); where it is undefined, as in most tests, no Chromecast is
   * offered.
   */
  readonly askGuestCastGrant?: AskGuestCastGrant | undefined;
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
  /**
   * Marked with whether the page's live connection is open, so the session's minute check rests
   * while it is (signed-out-watch.ts); undefined where nothing checks, as in a test that gives none.
   */
  readonly liveOpen?: Pick<LiveOpenMark, "mark"> | undefined;
}

/** How many times the page's requests moved path, and who is told of each move. */
export interface PathMoves {
  readonly count: () => number;
  /** The name of the path the last move went to, as the console says it; undefined before any. */
  readonly to: () => string | undefined;
  readonly subscribe: (listener: () => void) => () => void;
  /**
   * Whether the page's media go over WebRTC now, where the service worker builds every load's
   * answer and the browser's HTTP cache keeps none (the efficiency audit's finding 1): under a loader
   * as it says, false under an older one; on the core's own origin once a tab moved by
   * `?transport=webrtc` carried them over.
   */
  readonly isOverWebrtc: () => boolean;
  /**
   * The core's name the page's requests go to directly now, its origin, under a loader: undefined
   * while they go over WebRTC, or from a loader older than its minor 6, which does not say. Absent
   * on the core's own origin, where the page's own origin is the core's. What a device that fetches
   * a playlist itself, as an AirPlay receiver does, is handed it on (plans/airplay.md item 2).
   */
  readonly directOrigin?: (() => string | undefined) | undefined;
}

/**
 * A count of path moves, and the move to tell of: what client-main.tsx keeps, and tests move by hand;
 * and, on the core's own origin, the mark that the page's media are carried over WebRTC now
 * (carriedOverWebrtc), which a tab moved by `?transport=webrtc` sets (transport-choice.ts).
 */
export function createPathMoves(): PathMoves & {
  readonly moved: (to?: string) => void;
  readonly carriedOverWebrtc: () => void;
} {
  let count = 0;
  let last: string | undefined;
  let isOverWebrtc = false;
  const listeners = new Set<() => void>();
  return {
    count: () => count,
    to: () => last,
    isOverWebrtc: () => isOverWebrtc,
    carriedOverWebrtc: () => {
      isOverWebrtc = true;
    },
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
 * read again on a move and the play report that logs it (the owner, 2026-10-17), the kind of path
 * in use, where the loader says it (since its minor 2), and the core's name it goes to directly,
 * where the loader says it (since its minor 6).
 */
export function pathMovesHeardFrom(
  onPathMove: OrpheumLoader["onPathMove"],
  pathInUse?: OrpheumLoader["pathInUse"],
  directOriginInUse?: OrpheumLoader["directOriginInUse"],
): PathMoves {
  const pathMoves = createPathMoves();
  onPathMove((to) => {
    pathMoves.moved(to);
  });
  return {
    ...pathMoves,
    isOverWebrtc: () => pathInUse?.() === "webrtc",
    directOrigin: () => directOriginInUse?.(),
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
