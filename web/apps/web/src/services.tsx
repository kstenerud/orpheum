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
