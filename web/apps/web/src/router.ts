// SPDX-License-Identifier: MPL-2.0

/** Every screen the client shows, and what it needs from the address to show it. */
export type Route =
  | { readonly screen: "films" }
  | { readonly screen: "series" }
  | { readonly screen: "title"; readonly id: number }
  | { readonly screen: "series-page"; readonly id: number }
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
  | { readonly screen: "person"; readonly id: number }
  | { readonly screen: "account" }
  | { readonly screen: "register" }
  | { readonly screen: "not-found" };

/** One address shape: a pattern over the path, and the route its match names. */
interface Shape {
  readonly pattern: RegExp;
  readonly toRoute: (id: number, search: URLSearchParams) => Route;
}

// An id is a positive whole number; anything else names no item.
const shapes: readonly Shape[] = [
  { pattern: /^\/(?:films\/?)?$/u, toRoute: () => ({ screen: "films" }) },
  { pattern: /^\/series\/?$/u, toRoute: () => ({ screen: "series" }) },
  { pattern: /^\/titles\/([1-9]\d{0,15})$/u, toRoute: (id) => ({ screen: "title", id }) },
  { pattern: /^\/series\/([1-9]\d{0,15})$/u, toRoute: (id) => ({ screen: "series-page", id }) },
  { pattern: /^\/play\/([1-9]\d{0,15})$/u, toRoute: (fileId) => ({ screen: "play", fileId }) },
  {
    pattern: /^\/search\/?$/u,
    toRoute: (_, search) => ({ screen: "search", query: search.get("q") ?? "" }),
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
  { pattern: /^\/people\/([1-9]\d{0,15})$/u, toRoute: (id) => ({ screen: "person", id }) },
  { pattern: /^\/account\/?$/u, toRoute: () => ({ screen: "account" }) },
  { pattern: /^\/register\/?$/u, toRoute: () => ({ screen: "register" }) },
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
    return `/titles/${String(route.id)}`;
  }
  if (route.screen === "series-page") {
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
