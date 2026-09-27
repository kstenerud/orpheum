// SPDX-License-Identifier: MPL-2.0

/** Every screen the client shows, and what it needs from the address to show it. */
export type Route =
  | { readonly screen: "films" }
  | { readonly screen: "series" }
  | { readonly screen: "title"; readonly id: number }
  | { readonly screen: "collection"; readonly id: number }
  | { readonly screen: "play"; readonly fileId: number }
  | { readonly screen: "search"; readonly query: string }
  | { readonly screen: "identify" }
  | { readonly screen: "libraries" }
  | { readonly screen: "files"; readonly libraryId: number }
  | { readonly screen: "about" }
  | { readonly screen: "settings" }
  | { readonly screen: "jobs" }
  | { readonly screen: "people" }
  | { readonly screen: "person"; readonly id: number }
  | { readonly screen: "account" }
  | { readonly screen: "register" }
  | { readonly screen: "not-found" };

/** One address shape: a pattern over the path, and the route its match names. */
interface Shape {
  readonly pattern: RegExp;
  readonly route: (id: number, search: URLSearchParams) => Route;
}

// An id is a positive whole number; anything else names no item.
const shapes: readonly Shape[] = [
  { pattern: /^\/(?:films\/?)?$/u, route: () => ({ screen: "films" }) },
  { pattern: /^\/series\/?$/u, route: () => ({ screen: "series" }) },
  { pattern: /^\/titles\/([1-9]\d{0,15})$/u, route: (id) => ({ screen: "title", id }) },
  { pattern: /^\/series\/([1-9]\d{0,15})$/u, route: (id) => ({ screen: "collection", id }) },
  { pattern: /^\/play\/([1-9]\d{0,15})$/u, route: (fileId) => ({ screen: "play", fileId }) },
  {
    pattern: /^\/search\/?$/u,
    route: (_, search) => ({ screen: "search", query: search.get("q") ?? "" }),
  },
  { pattern: /^\/identify\/?$/u, route: () => ({ screen: "identify" }) },
  { pattern: /^\/libraries\/?$/u, route: () => ({ screen: "libraries" }) },
  {
    pattern: /^\/libraries\/([1-9]\d{0,15})\/files$/u,
    route: (libraryId) => ({ screen: "files", libraryId }),
  },
  { pattern: /^\/about\/?$/u, route: () => ({ screen: "about" }) },
  { pattern: /^\/settings\/?$/u, route: () => ({ screen: "settings" }) },
  { pattern: /^\/jobs\/?$/u, route: () => ({ screen: "jobs" }) },
  { pattern: /^\/people\/?$/u, route: () => ({ screen: "people" }) },
  { pattern: /^\/people\/([1-9]\d{0,15})$/u, route: (id) => ({ screen: "person", id }) },
  { pattern: /^\/account\/?$/u, route: () => ({ screen: "account" }) },
  { pattern: /^\/register\/?$/u, route: () => ({ screen: "register" }) },
];

/** The route an address names; an address the client does not know is not-found, never a guess. */
export function parseRoute(pathname: string, search: string): Route {
  for (const shape of shapes) {
    const match = shape.pattern.exec(pathname);
    if (match !== null) {
      return shape.route(Number(match[1] ?? "0"), new URLSearchParams(search));
    }
  }
  return { screen: "not-found" };
}

/** The address of a route: what parseRoute reads back as the same route. */
export function routeHref(route: Route): string {
  if (route.screen === "title") {
    return `/titles/${String(route.id)}`;
  }
  if (route.screen === "collection") {
    return `/series/${String(route.id)}`;
  }
  if (route.screen === "play") {
    return `/play/${String(route.fileId)}`;
  }
  if (route.screen === "person") {
    return `/people/${String(route.id)}`;
  }
  if (route.screen === "files") {
    return `/libraries/${String(route.libraryId)}/files`;
  }
  if (route.screen === "search") {
    return route.query === ""
      ? "/search"
      : `/search?${new URLSearchParams({ q: route.query }).toString()}`;
  }
  return route.screen === "not-found" ? "/" : `/${route.screen}`;
}
