// SPDX-License-Identifier: MPL-2.0

/** A device's QR code as the phone scanned it: its code and its key's hash (D321). */
export interface Scanned {
  readonly code: string;
  readonly keyHash: string;
}

/** The device lab's probes, each a page under /lab/. */
export const labPages = ["device", "input", "remote", "extend", "throughput", "cast"] as const;

/** One of the device lab's probes. */
export type LabPage = (typeof labPages)[number];

/**
 * The order a library's list comes in (D349 §9.1): recorded and file_name, an other library's;
 * released, a film's release date or a series' first air date, films' and series' (D366 §1).
 */
export type LibrarySort = "name" | "added" | "recorded" | "file_name" | "released";

const librarySorts: readonly LibrarySort[] = ["name", "added", "recorded", "file_name", "released"];

/**
 * The way a sort runs (D366 §2): ascending, A to Z or oldest first; descending, Z to A or newest
 * first. A route without one runs its sort's own way, A to Z by a name and newest first by a date.
 */
export type SortOrder = "ascending" | "descending";

const sortOrders: readonly SortOrder[] = ["ascending", "descending"];

/** Every screen the client shows, and what it needs from the address to show it. */
export type Route =
  | { readonly screen: "home" }
  | { readonly screen: "films" }
  | { readonly screen: "series" }
  | { readonly screen: "title"; readonly itemId: number }
  /** A series' page: its seasons, or one season's episodes (the owner, 2026-10-07: drill down). */
  | { readonly screen: "series-page"; readonly collectionId: number; readonly season?: number }
  /**
   * A file's Play page; with atMs, starting there in place of where the person last left the file:
   * a cast's remote playing here from where its receiver stood (D337's §9 answers, Q5). With
   * shouldPlayHere, it plays on this device while it casts, not on the receiver: the remote's "Play on this
   * device instead" (D364 §4).
   */
  | {
      readonly screen: "play";
      readonly fileId: number;
      readonly atMs?: number;
      readonly shouldPlayHere?: true;
    }
  /**
   * The Libraries page (D349 §9.1), which replaces Search: the library chosen ("all" for every one
   * the person reaches, undefined for none chosen yet, which the page itself resolves to the first
   * reachable by name and replaces the address with), its view (folders, an other library's; all
   * else), the folder shown in it, its sort and its search query, all in the address so Back and a
   * reload return to it. Its address is built by routeHref, never by hand (D227's reason).
   */
  | {
      readonly screen: "browse";
      readonly library?: number | "all";
      readonly view?: "folders";
      readonly folder?: string;
      readonly sort?: LibrarySort;
      readonly order?: SortOrder;
      readonly query?: string;
    }
  | { readonly screen: "identify" }
  /** The admin's list of libraries, each leading to its own page to manage it (D361). */
  | { readonly screen: "libraries" }
  /** One library's page, where the admin manages it: its folders, its scans, deleting it (D361). */
  | { readonly screen: "library"; readonly libraryId: number }
  /** Creating a library, a page of its own (D361). */
  | { readonly screen: "new-library" }
  | { readonly screen: "files"; readonly libraryId: number }
  | { readonly screen: "about" }
  | { readonly screen: "settings" }
  | { readonly screen: "jobs" }
  | { readonly screen: "system" }
  | { readonly screen: "needs-attention" }
  | { readonly screen: "people" }
  | { readonly screen: "person"; readonly userId: number }
  /** One's account; with scanned, a device's QR code was scanned, to activate it there (D321). */
  | { readonly screen: "account"; readonly scanned?: Scanned }
  | { readonly screen: "this-device" }
  | { readonly screen: "register" }
  | { readonly screen: "together" }
  | { readonly screen: "watch-together" }
  /**
   * An address of the casting page D384 §2 took away, as a history entry may hold one: shown as the
   * Play page of the film the cast plays, else home (old-addresses.tsx).
   */
  | { readonly screen: "casting" }
  /** A cast's receiver between videos: full screen, ready, controlled from the sender (D364 §5). */
  | { readonly screen: "cast-ready" }
  /**
   * The device lab (plans/tv-device-tests.md): its menu, or one of its probes, at a short address a
   * TV's remote types quickly; no menu leads to it.
   */
  | { readonly screen: "lab"; readonly probe?: LabPage }
  /** The lab's extend probe's player, in a window of its own on the TV's screen: no shell. */
  | { readonly screen: "lab-extend-player" }
  /** The client's own parts with made-up data, for seeing how they look; no menu leads to it. */
  | { readonly screen: "ui-gallery" }
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
  { pattern: /^\/$/u, toRoute: () => ({ screen: "home" }) },
  { pattern: /^\/films\/?$/u, toRoute: () => ({ screen: "films" }) },
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
  { pattern: /^\/play\/([1-9]\d{0,15})$/u, toRoute: (fileId, search) => playRoute(fileId, search) },
  { pattern: /^\/browse\/?$/u, toRoute: (_, search) => browseRoute(search) },
  { pattern: /^\/identify\/?$/u, toRoute: () => ({ screen: "identify" }) },
  { pattern: /^\/libraries\/?$/u, toRoute: () => ({ screen: "libraries" }) },
  { pattern: /^\/libraries\/new\/?$/u, toRoute: () => ({ screen: "new-library" }) },
  {
    pattern: /^\/libraries\/([1-9]\d{0,15})\/?$/u,
    toRoute: (libraryId) => ({ screen: "library", libraryId }),
  },
  {
    pattern: /^\/libraries\/([1-9]\d{0,15})\/files$/u,
    toRoute: (libraryId) => ({ screen: "files", libraryId }),
  },
  { pattern: /^\/about\/?$/u, toRoute: () => ({ screen: "about" }) },
  { pattern: /^\/settings\/?$/u, toRoute: () => ({ screen: "settings" }) },
  { pattern: /^\/jobs\/?$/u, toRoute: () => ({ screen: "jobs" }) },
  { pattern: /^\/system\/?$/u, toRoute: () => ({ screen: "system" }) },
  { pattern: /^\/needs-attention\/?$/u, toRoute: () => ({ screen: "needs-attention" }) },
  { pattern: /^\/people\/?$/u, toRoute: () => ({ screen: "people" }) },
  { pattern: /^\/people\/([1-9]\d{0,15})$/u, toRoute: (userId) => ({ screen: "person", userId }) },
  { pattern: /^\/account\/?$/u, toRoute: (_, search) => accountRoute(search) },
  { pattern: /^\/this-device\/?$/u, toRoute: () => ({ screen: "this-device" }) },
  { pattern: /^\/register\/?$/u, toRoute: () => ({ screen: "register" }) },
  { pattern: /^\/together\/?$/u, toRoute: () => ({ screen: "together" }) },
  { pattern: /^\/watch-together\/?$/u, toRoute: () => ({ screen: "watch-together" }) },
  { pattern: /^\/casting\/?$/u, toRoute: () => ({ screen: "casting" }) },
  { pattern: /^\/cast-ready\/?$/u, toRoute: () => ({ screen: "cast-ready" }) },
  { pattern: /^\/ui-gallery\/?$/u, toRoute: () => ({ screen: "ui-gallery" }) },
  { pattern: /^\/lab\/?$/u, toRoute: () => ({ screen: "lab" }) },
  ...labPages.map((probe) => ({
    pattern: new RegExp(`^/lab/${probe}/?$`, "u"),
    toRoute: (): Route => ({ screen: "lab", probe }),
  })),
  { pattern: /^\/lab\/extend\/player\/?$/u, toRoute: () => ({ screen: "lab-extend-player" }) },
];

/**
 * The Play page's route, with where it starts, `at` in ms, where the address says a whole number,
 * and whether it plays here while the device casts (`here`).
 */
function playRoute(fileId: number, search: URLSearchParams): Route {
  const at = search.get("at") ?? "";
  return {
    screen: "play",
    fileId,
    ...(/^\d{1,12}$/u.test(at) ? { atMs: Number(at) } : {}),
    ...(search.get("here") === "1" ? { shouldPlayHere: true } : {}),
  };
}

/** A library id from the address: a positive whole number, else undefined (not a library). */
function libraryIdOf(raw: string): number | undefined {
  return /^[1-9]\d{0,15}$/u.test(raw) ? Number(raw) : undefined;
}

/** The Libraries page's library parameter, read from its address: "all", an id, or none chosen. */
function browseLibraryOf(raw: string | null): number | "all" | undefined {
  if (raw === null) {
    return undefined;
  }
  return raw === "all" ? "all" : libraryIdOf(raw);
}

/**
 * The Libraries page's route, read from its address: library "all" for every one chosen in the
 * chooser, a library's id, or undefined for none chosen yet (the page itself picks the first
 * reachable by name, and replaces the address with it, D349 §9.1 point 2).
 */
function browseRoute(search: URLSearchParams): Route {
  const library = browseLibraryOf(search.get("library"));
  const view = search.get("view") === "folders" ? "folders" : undefined;
  const folder = search.get("folder") ?? undefined;
  const sort = librarySorts.find((candidate) => candidate === search.get("sort"));
  const order = sortOrders.find((candidate) => candidate === search.get("order"));
  const query = search.get("query") ?? "";
  return {
    screen: "browse",
    ...(library === undefined ? {} : { library }),
    ...(view === undefined ? {} : { view }),
    ...(folder === undefined ? {} : { folder }),
    ...(sort === undefined ? {} : { sort }),
    ...(order === undefined ? {} : { order }),
    ...(query === "" ? {} : { query }),
  };
}

/** The route an address names; an address the client does not know is not-found, never a guess. */
/** The account's route, with the scanned code its address carries, if both its parts are there. */
function accountRoute(search: URLSearchParams): Route {
  const code = search.get("code") ?? "";
  const keyHash = search.get("key") ?? "";
  return code === "" || keyHash === ""
    ? { screen: "account" }
    : { screen: "account", scanned: { code, keyHash } };
}

export function parseRoute(pathname: string, search: string): Route {
  for (const shape of shapes) {
    const match = shape.pattern.exec(pathname);
    if (match !== null) {
      return shape.toRoute(Number(match[1] ?? "0"), new URLSearchParams(search));
    }
  }
  return { screen: "not-found" };
}

/** The address of a route whose query carries its state, or undefined for another. */
function queryRouteHref(route: Route): string | undefined {
  if (route.screen === "account" && route.scanned !== undefined) {
    const query = new URLSearchParams({ code: route.scanned.code, key: route.scanned.keyHash });
    return `/account?${query.toString()}`;
  }
  if (route.screen === "browse") {
    return browseRouteHref(route);
  }
  if (route.screen === "play") {
    const query = new URLSearchParams();
    if (route.atMs !== undefined) {
      query.set("at", String(Math.round(route.atMs)));
    }
    if (route.shouldPlayHere === true) {
      query.set("here", "1");
    }
    const asString = query.toString();
    return `/play/${String(route.fileId)}${asString === "" ? "" : `?${asString}`}`;
  }
  return undefined;
}

/** The Libraries page's address, holding what it shows (D349 §9.1 point 2). */
function browseRouteHref(route: Route & { readonly screen: "browse" }): string {
  const query = new URLSearchParams();
  if (route.library !== undefined) {
    query.set("library", String(route.library));
  }
  if (route.view !== undefined) {
    query.set("view", route.view);
  }
  if (route.folder !== undefined) {
    query.set("folder", route.folder);
  }
  if (route.sort !== undefined) {
    query.set("sort", route.sort);
  }
  if (route.order !== undefined) {
    query.set("order", route.order);
  }
  if (route.query !== undefined && route.query !== "") {
    query.set("query", route.query);
  }
  const asString = query.toString();
  return asString === "" ? "/browse" : `/browse?${asString}`;
}

/** The address of a library's own page, of its files, or of creating one (D361); else undefined. */
function libraryRouteHref(route: Route): string | undefined {
  if (route.screen === "library") {
    return `/libraries/${String(route.libraryId)}`;
  }
  if (route.screen === "files") {
    return `/libraries/${String(route.libraryId)}/files`;
  }
  return route.screen === "new-library" ? "/libraries/new" : undefined;
}

/** The address of one of the device lab's routes, or undefined for another. */
function labRouteHref(route: Route): string | undefined {
  if (route.screen === "lab") {
    return route.probe === undefined ? "/lab" : `/lab/${route.probe}`;
  }
  return route.screen === "lab-extend-player" ? "/lab/extend/player" : undefined;
}

/** The screens at the site's root: home, and an address that names nothing, which goes there. */
const rootScreens: ReadonlySet<Route["screen"]> = new Set(["home", "not-found"]);

/** The route an address of this app's names, a path with its query: routeHref's inverse. */
export function routeOfHref(href: string): Route {
  const url = new URL(href, "http://orpheum.invalid");
  return parseRoute(url.pathname, url.search);
}

/** The address of a route: what parseRoute reads back as the same route. */
export function routeHref(route: Route): string {
  const withQuery = queryRouteHref(route);
  if (withQuery !== undefined) {
    return withQuery;
  }
  if (route.screen === "title") {
    return `/titles/${String(route.itemId)}`;
  }
  if (route.screen === "series-page") {
    const season = route.season === undefined ? "" : `?season=${String(route.season)}`;
    return `/series/${String(route.collectionId)}${season}`;
  }
  if (route.screen === "person") {
    return `/people/${String(route.userId)}`;
  }
  const library = libraryRouteHref(route);
  if (library !== undefined) {
    return library;
  }
  const lab = labRouteHref(route);
  if (lab !== undefined) {
    return lab;
  }
  return rootScreens.has(route.screen) ? "/" : `/${route.screen}`;
}
