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
   * has somewhere to go back to, and going up twice never returns to where it started. With no up,
   * as a remote's Back on a page with no Back of its own, nothing where the entry before is not
   * this app's. Whether it went anywhere.
   */
  readonly back: (up?: Route) => boolean;
  /** Where this app is served from ("https://media.example"), for a link to it. */
  readonly origin: () => string;
  /** What follows # in the address, without the #, or "" when nothing does. */
  readonly fragment: () => string;
  readonly subscribe: (listener: () => void) => () => void;
  /**
   * Tells listener just before go leaves the current entry, while what the page shows is still
   * there, so it can note on the entry what Back should find again; returns the stopping.
   */
  readonly beforeLeaving: (listener: () => void) => () => void;
  /** Keeps note with the current history entry, for when Back returns to it (focus-memory.ts). */
  readonly noteOnEntry: (note: unknown) => void;
  /** What was noted on the current history entry; undefined where nothing was. */
  readonly entryNote: () => unknown;
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

/** What an entry's state noted with it, as noteOnEntry kept it; undefined for none. */
function noteOf(state: unknown): unknown {
  return typeof state === "object" && state !== null && "note" in state ? state.note : undefined;
}

/** Navigation over a place: a pushed or replaced route and the back button all tell every listener. */
export function createNavigation(place: Place): Navigation {
  const readRoute = (): Route => parseRoute(place.location.pathname, place.location.search);
  let current = readRoute();
  let depth = depthOf(place.history.state);
  const listeners = new Set<() => void>();
  const leavingListeners = new Set<() => void>();
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
      for (const listener of leavingListeners) {
        listener();
      }
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
        return true;
      }
      if (up === undefined) {
        return false;
      }
      place.history.replaceState({ appDepth: 0 }, "", routeHref(up));
      rereadRoute();
      return true;
    },
    origin: () => place.location.origin,
    fragment: () => place.location.hash.replace(/^#/u, ""),
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    beforeLeaving: (listener) => {
      leavingListeners.add(listener);
      return () => leavingListeners.delete(listener);
    },
    noteOnEntry: (note) => {
      const state: unknown = place.history.state;
      const kept = typeof state === "object" && state !== null ? state : {};
      const { pathname, search, hash } = place.location;
      place.history.replaceState({ ...kept, note }, "", pathname + search + hash);
    },
    entryNote: () => noteOf(place.history.state),
  };
}
