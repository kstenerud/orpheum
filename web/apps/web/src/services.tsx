// SPDX-License-Identifier: MPL-2.0

import type { Api, Menu } from "@orpheum/sdk";
import { createContext, use, useSyncExternalStore } from "react";

import type { Navigation } from "./navigation.ts";
import type { Route } from "./router.ts";
import type { Session } from "./session.ts";

/** What every screen reaches the world through: the API, the session, navigation, and what this browser plays. */
export interface Services {
  readonly api: Api;
  readonly session: Session;
  readonly navigation: Navigation;
  readonly playableFormats: () => Menu;
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
