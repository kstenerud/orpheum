// SPDX-License-Identifier: MPL-2.0

import { parseRoute, routeHref, type Route } from "./router.ts";

/** What navigation reads and writes: the browser's location and history, or a test's. */
export interface Place {
  readonly location: { readonly pathname: string; readonly search: string };
  readonly history: { pushState(data: unknown, unused: string, url: string): void };
  addEventListener(type: "popstate", listener: () => void): void;
}

/** The current route, moving to another, and telling listeners when it changes. */
export interface Navigation {
  readonly route: () => Route;
  readonly go: (route: Route) => void;
  readonly subscribe: (listener: () => void) => () => void;
}

/** Navigation over a place: a pushed route and the back button both tell every listener. */
export function createNavigation(place: Place): Navigation {
  const read = (): Route => parseRoute(place.location.pathname, place.location.search);
  let current = read();
  const listeners = new Set<() => void>();
  const changed = (): void => {
    current = read();
    for (const listener of listeners) {
      listener();
    }
  };
  place.addEventListener("popstate", changed);
  return {
    route: () => current,
    go: (route) => {
      place.history.pushState(null, "", routeHref(route));
      changed();
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}
