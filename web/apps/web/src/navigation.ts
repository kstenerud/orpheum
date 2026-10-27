// SPDX-License-Identifier: MPL-2.0

import { isTelevision } from "./tv-device.ts";
import { parseRoute, routeHref, routeOfHref, type Route } from "./router.ts";

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
    /** Moves delta entries through the history, back where negative: Up to a parent behind. */
    go(delta: number): void;
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
  /**
   * Up (D367 §7): back to the nearest of this app's entries behind whose route isParent says is the
   * page's parent, however many steps back, so the entries Up passed over leave the back history;
   * else parent in place of the current address. Never adds an entry, so Back after Up never
   * returns down the way Up came.
   */
  readonly up: (parent: Route, isParent: (route: Route) => boolean) => void;
  /** The routes of this app's entries behind the current one, the nearest last: what Up can find. */
  readonly behind: () => readonly Route[];
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
  /**
   * Tells listener when the browser's own Back reached the base laid under a page this app opened
   * on, where the page's Back goes up from; returns the stopping. With no listener, home.
   */
  readonly onBase: (listener: () => void) => () => void;
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

/**
 * The addresses of this app's entries behind an entry, the nearest last, as its state keeps them:
 * written at each go, so Up can find a parent behind it however it was reached (D367 §7).
 */
function belowOf(state: unknown): readonly string[] {
  if (typeof state !== "object" || state === null || !("below" in state)) {
    return [];
  }
  const { below } = state;
  return Array.isArray(below) && below.every((each) => typeof each === "string") ? below : [];
}

/**
 * How many addresses behind an entry its state keeps: more than a person climbs back through, few
 * enough that each entry's state stays small. A justified constant.
 */
const belowKept = 50;

/** What an entry's state noted with it, as noteOnEntry kept it; undefined for none. */
function noteOf(state: unknown): unknown {
  return typeof state === "object" && state !== null && "note" in state ? state.note : undefined;
}

/** Navigation over a place: a pushed or replaced route and the back button all tell every listener. */
export function createNavigation(
  place: Place,
  isOnTelevision: boolean = isTelevision(globalThis.navigator.userAgent),
): Navigation {
  const readRoute = (): Route => parseRoute(place.location.pathname, place.location.search);
  let current = readRoute();
  let depth = isOnTelevision ? layBase(place, current) : depthOf(place.history.state);
  const listeners = new Set<() => void>();
  const leavingListeners = new Set<() => void>();
  const baseListeners = new Set<() => void>();
  const rereadRoute = (): void => {
    current = readRoute();
    depth = depthOf(place.history.state);
    for (const listener of listeners) {
      listener();
    }
    if (isBase(place.history.state)) {
      upFromBase(place, baseListeners, rereadRoute);
    }
  };
  place.addEventListener("popstate", rereadRoute);
  return {
    route: () => current,
    go: (route) => {
      for (const listener of leavingListeners) {
        listener();
      }
      const below = [...belowOf(place.history.state), routeHref(current)].slice(-belowKept);
      place.history.pushState({ appDepth: depth + 1, below }, "", routeHref(route));
      rereadRoute();
    },
    replace: (route) => {
      replaceKeepingBelow(place, depth, route);
      rereadRoute();
    },
    up: (parent, isParent) => {
      if (!goBackTo(place, isParent)) {
        replaceKeepingBelow(place, depth, parent);
        rereadRoute();
      }
    },
    behind: () => belowOf(place.history.state).map(routeOfHref),
    back: (up) => {
      const went = backOrUp(place, depth, up);
      if (went === "up") {
        rereadRoute();
      }
      return went !== "nowhere";
    },
    origin: () => place.location.origin,
    fragment: () => place.location.hash.replace(/^#/u, ""),
    subscribe: adderTo(listeners),
    beforeLeaving: adderTo(leavingListeners),
    noteOnEntry: (note) => {
      noteOn(place, note);
    },
    entryNote: () => noteOf(place.history.state),
    onBase: adderTo(baseListeners),
  };
}

/**
 * Back: the entry before where depth says one of this app's is, which popstate then tells; else up
 * in the current entry's place; else, with no up, nowhere.
 */
function backOrUp(place: Place, depth: number, up: Route | undefined): "back" | "up" | "nowhere" {
  if (depth > 0) {
    place.history.back();
    return "back";
  }
  if (up === undefined) {
    return "nowhere";
  }
  place.history.replaceState({ appDepth: 0 }, "", routeHref(up));
  return "up";
}

/** Puts route in the current entry's place, which keeps its depth and what is behind it. */
function replaceKeepingBelow(place: Place, depth: number, route: Route): void {
  const below = belowOf(place.history.state);
  place.history.replaceState({ appDepth: depth, below }, "", routeHref(route));
}

/** Keeps note with the current entry, beside what its state holds already. */
function noteOn(place: Place, note: unknown): void {
  const state: unknown = place.history.state;
  const kept = typeof state === "object" && state !== null ? state : {};
  const { pathname, search, hash } = place.location;
  place.history.replaceState({ ...kept, note }, "", pathname + search + hash);
}

/**
 * Goes back to the nearest of this app's entries behind whose route isIt says, however many steps
 * back; whether one was. The browser tells the move by popstate, which reads the route again.
 */
function goBackTo(place: Place, isIt: (route: Route) => boolean): boolean {
  const below = belowOf(place.history.state);
  const at = below.findLastIndex((href) => isIt(routeOfHref(href)));
  if (at < 0) {
    return false;
  }
  place.history.go(at - below.length);
  return true;
}

/** What adds a listener to listeners, and returns its removal. */
function adderTo(listeners: Set<() => void>): (listener: () => void) => () => void {
  return (listener) => {
    listeners.add(listener);
    return () => listeners.delete(listener);
  };
}

/**
 * Lays a base under a page this app opened on with nothing of its own before it (a reload, an
 * address typed in, a bookmark), on a television, so the browser's own Back, which is its remote's
 * (Silk's), has an entry of this app's to step to and does not leave the app: the owner's Fire TV,
 * reloaded on the player, offered to close Silk at Back (2026-10-25). Elsewhere a Back that leaves
 * the app is the browser's to give. The base is the same address marked so, without what follows
 * its #, which may be a secret no history keeps (D218 §1); Back goes up from it (onBase). Home
 * needs none, a Back there leaving the app as it should. Returns the current entry's depth.
 */
function layBase(place: Place, current: Route): number {
  const state: unknown = place.history.state;
  if (state !== null || current.screen === "home") {
    return depthOf(state);
  }
  const { pathname, search, hash } = place.location;
  place.history.replaceState({ appDepth: 0, isBase: true }, "", pathname + search);
  place.history.pushState({ appDepth: 1 }, "", pathname + search + hash);
  return 1;
}

/** Whether a history entry's state is a base layBase laid. */
function isBase(state: unknown): boolean {
  return typeof state === "object" && state !== null && "isBase" in state && state.isBase === true;
}

/** Back reached a base: its listeners go up from it, or, with none, home takes its place. */
function upFromBase(
  place: Place,
  baseListeners: ReadonlySet<() => void>,
  reread: () => void,
): void {
  if (baseListeners.size > 0) {
    for (const listener of baseListeners) {
      listener();
    }
    return;
  }
  place.history.replaceState({ appDepth: 0 }, "", routeHref({ screen: "home" }));
  reread();
}
