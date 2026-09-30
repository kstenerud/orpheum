// SPDX-License-Identifier: MPL-2.0

import { parseRoute, routeHref, type Route } from "./router.ts";

/** What navigation reads and writes: the browser's location and history, or a test's. */
export interface Place {
  readonly location: {
    readonly origin: string;
    readonly pathname: string;
    readonly search: string;
    readonly hash: string;
  };
  readonly history: {
    pushState(data: unknown, unused: string, url: string): void;
    replaceState(data: unknown, unused: string, url: string): void;
  };
  addEventListener(type: "popstate", listener: () => void): void;
}

/**
 * The address bar, the one place that reads and writes it: the current route, moving to another
 * or putting one in the current one's place, the address's origin and fragment, and telling
 * listeners when the route changes.
 */
export interface Navigation {
  readonly route: () => Route;
  readonly go: (route: Route) => void;
  /** Moves to route in place of the current address, which leaves the history. */
  readonly replace: (route: Route) => void;
  /** Where this app is served from ("https://media.example"), for a link to it. */
  readonly origin: () => string;
  /** What follows # in the address, without the #, or "" when nothing does. */
  readonly fragment: () => string;
  readonly subscribe: (listener: () => void) => () => void;
}

/** Navigation over a place: a pushed or replaced route and the back button all tell every listener. */
export function createNavigation(place: Place): Navigation {
  const readRoute = (): Route => parseRoute(place.location.pathname, place.location.search);
  let current = readRoute();
  const listeners = new Set<() => void>();
  const rereadRoute = (): void => {
    current = readRoute();
    for (const listener of listeners) {
      listener();
    }
  };
  place.addEventListener("popstate", rereadRoute);
  return {
    route: () => current,
    go: (route) => {
      place.history.pushState(null, "", routeHref(route));
      rereadRoute();
    },
    replace: (route) => {
      place.history.replaceState(null, "", routeHref(route));
      rereadRoute();
    },
    origin: () => place.location.origin,
    fragment: () => place.location.hash.replace(/^#/u, ""),
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}
