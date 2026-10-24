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
    /** What the current entry was pushed or replaced with; null for one this app did not write. */
    readonly state: unknown;
    back(): void;
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
  /**
   * Back: the history's entry before when it is this app's, else up in place of the current
   * address, so a page opened from outside (a bookmark, a reload of an address typed in) still
   * has somewhere to go back to, and going up twice never returns to where it started.
   */
  readonly back: (up: Route) => void;
  /** Where this app is served from ("https://media.example"), for a link to it. */
  readonly origin: () => string;
  /** What follows # in the address, without the #, or "" when nothing does. */
  readonly fragment: () => string;
  readonly subscribe: (listener: () => void) => () => void;
}

/**
 * How many of this app's entries are in the history before the current one, as the entry's state
 * keeps it: 0 for an entry this app did not write, as the first one a tab opens on.
 */
function depthOf(state: unknown): number {
  if (typeof state !== "object" || state === null || !("appDepth" in state)) {
    return 0;
  }
  const { appDepth } = state;
  return typeof appDepth === "number" && Number.isInteger(appDepth) && appDepth > 0 ? appDepth : 0;
}

/** Navigation over a place: a pushed or replaced route and the back button all tell every listener. */
export function createNavigation(place: Place): Navigation {
  const readRoute = (): Route => parseRoute(place.location.pathname, place.location.search);
  let current = readRoute();
  let depth = depthOf(place.history.state);
  const listeners = new Set<() => void>();
  const rereadRoute = (): void => {
    current = readRoute();
    depth = depthOf(place.history.state);
    for (const listener of listeners) {
      listener();
    }
  };
  place.addEventListener("popstate", rereadRoute);
  return {
    route: () => current,
    go: (route) => {
      place.history.pushState({ appDepth: depth + 1 }, "", routeHref(route));
      rereadRoute();
    },
    replace: (route) => {
      place.history.replaceState({ appDepth: depth }, "", routeHref(route));
      rereadRoute();
    },
    back: (up) => {
      if (depth > 0) {
        // The browser tells the move by popstate, which reads the route again.
        place.history.back();
        return;
      }
      place.history.replaceState({ appDepth: 0 }, "", routeHref(up));
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
