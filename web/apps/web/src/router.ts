// SPDX-License-Identifier: MPL-2.0

/** Every screen the client shows, and what it needs from the address to show it. */
export type Route =
  | { readonly screen: "films" }
  | { readonly screen: "series" }
  | { readonly screen: "title"; readonly itemId: number }
  /** A series' page: its seasons, or one season's episodes (the owner, 2026-10-07: drill down). */
  | { readonly screen: "series-page"; readonly collectionId: number; readonly season?: number }
  | { readonly screen: "play"; readonly fileId: number }
  | { readonly screen: "search"; readonly query: string }
  | { readonly screen: "identify" }
  | { readonly screen: "libraries" }
  | { readonly screen: "files"; readonly libraryId: number }
  | { readonly screen: "about" }
  | { readonly screen: "settings" }
  | { readonly screen: "jobs" }
  | { readonly screen: "system" }
  | { readonly screen: "people" }
  | { readonly screen: "person"; readonly userId: number }
  | { readonly screen: "account" }
  | { readonly screen: "this-device" }
  | { readonly screen: "register" }
  | { readonly screen: "together" }
  | { readonly screen: "watch-together" }
  | { readonly screen: "not-found" };

/** Where an item of a kind leads: a series' page, else a title's (a film, an episode, an extra). */
export function itemRoute(kind: string, id: number): Route {
  return kind === "series"
    ? { screen: "series-page", collectionId: id }
    : { screen: "title", itemId: id };
}

/** One address shape: a pattern over the path, and the route its match names. */
interface Shape {
  readonly pattern: RegExp;
  readonly toRoute: (id: number, search: URLSearchParams) => Route;
}

// An id is a positive whole number; anything else names no item.
const shapes: readonly Shape[] = [
  { pattern: /^\/(?:films\/?)?$/u, toRoute: () => ({ screen: "films" }) },
  { pattern: /^\/series\/?$/u, toRoute: () => ({ screen: "series" }) },
  { pattern: /^\/titles\/([1-9]\d{0,15})$/u, toRoute: (itemId) => ({ screen: "title", itemId }) },
  {
    pattern: /^\/series\/([1-9]\d{0,15})$/u,
    toRoute: (collectionId, search) => {
      const season = Number(search.get("season") ?? "-1");
      return Number.isInteger(season) && season >= 0 && season < 10_000
        ? { screen: "series-page", collectionId, season }
        : { screen: "series-page", collectionId };
    },
  },
  { pattern: /^\/play\/([1-9]\d{0,15})$/u, toRoute: (fileId) => ({ screen: "play", fileId }) },
  {
    pattern: /^\/search\/?$/u,
    toRoute: (_, search) => ({ screen: "search", query: search.get("query") ?? "" }),
  },
  { pattern: /^\/identify\/?$/u, toRoute: () => ({ screen: "identify" }) },
  { pattern: /^\/libraries\/?$/u, toRoute: () => ({ screen: "libraries" }) },
  {
    pattern: /^\/libraries\/([1-9]\d{0,15})\/files$/u,
    toRoute: (libraryId) => ({ screen: "files", libraryId }),
  },
  { pattern: /^\/about\/?$/u, toRoute: () => ({ screen: "about" }) },
  { pattern: /^\/settings\/?$/u, toRoute: () => ({ screen: "settings" }) },
  { pattern: /^\/jobs\/?$/u, toRoute: () => ({ screen: "jobs" }) },
  { pattern: /^\/system\/?$/u, toRoute: () => ({ screen: "system" }) },
  { pattern: /^\/people\/?$/u, toRoute: () => ({ screen: "people" }) },
  { pattern: /^\/people\/([1-9]\d{0,15})$/u, toRoute: (userId) => ({ screen: "person", userId }) },
  { pattern: /^\/account\/?$/u, toRoute: () => ({ screen: "account" }) },
  { pattern: /^\/this-device\/?$/u, toRoute: () => ({ screen: "this-device" }) },
  { pattern: /^\/register\/?$/u, toRoute: () => ({ screen: "register" }) },
  { pattern: /^\/together\/?$/u, toRoute: () => ({ screen: "together" }) },
  { pattern: /^\/watch-together\/?$/u, toRoute: () => ({ screen: "watch-together" }) },
];

/** The route an address names; an address the client does not know is not-found, never a guess. */
export function parseRoute(pathname: string, search: string): Route {
  for (const shape of shapes) {
    const match = shape.pattern.exec(pathname);
    if (match !== null) {
      return shape.toRoute(Number(match[1] ?? "0"), new URLSearchParams(search));
    }
  }
  return { screen: "not-found" };
}

/** The address of a route: what parseRoute reads back as the same route. */
export function routeHref(route: Route): string {
  if (route.screen === "title") {
    return `/titles/${String(route.itemId)}`;
  }
  if (route.screen === "series-page") {
    const season = route.season === undefined ? "" : `?season=${String(route.season)}`;
    return `/series/${String(route.collectionId)}${season}`;
  }
  if (route.screen === "play") {
    return `/play/${String(route.fileId)}`;
  }
  if (route.screen === "person") {
    return `/people/${String(route.userId)}`;
  }
  if (route.screen === "files") {
    return `/libraries/${String(route.libraryId)}/files`;
  }
  if (route.screen === "search") {
    return route.query === ""
      ? "/search"
      : `/search?${new URLSearchParams({ query: route.query }).toString()}`;
  }
  return route.screen === "not-found" ? "/" : `/${route.screen}`;
}
