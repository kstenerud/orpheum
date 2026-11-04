// SPDX-License-Identifier: MPL-2.0

/**
 * Where a page sits and what is above it (D367): its ancestors, the top first, each a page Up can go
 * to, the last its parent; and what the page itself is called. A page's trail and its Up are made of
 * it (up-trail.tsx); the parent of a library's page is the library the person came through when one
 * listing the item is behind in the history, else the first of the person's libraries listing it, by
 * name (§3).
 */

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";

import { placeCode } from "./episode-place.ts";
import type { Route } from "./router.ts";

/** A library a title or a series is listed in, by id and name, as the server sends it. */
export type LibraryName = components["schemas"]["LibraryNameBody"];

/** An episode's series and place, as the server sends it. */
type EpisodePlace = components["schemas"]["EpisodePlaceBody"];

/** One page above another: its name, its route, and how a route behind is known to be it. */
export interface Crumb {
  readonly label: string;
  readonly to: Route;
  /** Whether a route behind in the history shows this page, whatever its view (D367 §9). */
  readonly isIt: (route: Route) => boolean;
}

/** A page's place: its ancestors, the top first, the last its parent; and its own name. */
export interface Upward {
  readonly crumbs: readonly Crumb[];
  readonly here: string;
}

/** The page's parent, the last of its ancestors; undefined for a top page, which has none. */
export function parentOf(upward: Upward): Crumb | undefined {
  return upward.crumbs.at(-1);
}

/** What a page's library holds it as: a film, a series, or an other library's video. */
export type Listed = "movie" | "series" | "other";

/** What a page's crumbs are named and found by. */
export interface LibraryContext {
  readonly t: TFunction;
  /** The routes behind the page in the app's history, the nearest last. */
  readonly behind: readonly Route[];
}

/**
 * Whether route is a list that holds what listed and libraries say: Films for a film, Series for a
 * series, All libraries, or the page of one of the libraries listing it (any library where those
 * are not known), at any folder, sort or search.
 */
function isListOf(listed: Listed, libraries: readonly LibraryName[] | undefined) {
  return (route: Route): boolean => {
    if (route.screen === "films") {
      return listed === "movie";
    }
    if (route.screen === "series") {
      return listed === "series";
    }
    if (route.screen !== "browse" || route.library === undefined) {
      return false;
    }
    const { library } = route;
    return (
      library === "all" ||
      libraries === undefined ||
      libraries.some((listing) => listing.id === library)
    );
  };
}

/**
 * A list's name: Films, Series, Libraries, or the library's own, one name for each wherever it is
 * shown (ui-audit-2026-11-04.md Q7): every library together is the Libraries page.
 */
function listLabel(
  route: Route,
  libraries: readonly LibraryName[] | undefined,
  t: TFunction,
): string {
  if (route.screen === "films") {
    return t("nav.films");
  }
  if (route.screen === "series") {
    return t("nav.series");
  }
  if (route.screen !== "browse" || route.library === undefined || route.library === "all") {
    return t("browse.libraries");
  }
  const id = route.library;
  return libraries?.find((library) => library.id === id)?.name ?? t("browse.library");
}

/**
 * The library a page goes up to (D367 §3): the nearest list behind that holds it, as the person
 * came through it; else the first of the libraries listing it, by name, as the server lists them;
 * else Films or Series, or the Libraries page for an other video.
 */
export function libraryCrumb(
  listed: Listed,
  libraries: readonly LibraryName[] | undefined,
  context: LibraryContext,
): Crumb {
  const isIt = isListOf(listed, libraries);
  const behind = context.behind.findLast(isIt);
  if (behind !== undefined) {
    return { label: listLabel(behind, libraries, context.t), to: behind, isIt };
  }
  const first = libraries?.[0];
  if (first !== undefined) {
    return { label: first.name, to: { screen: "browse", library: first.id }, isIt };
  }
  const top = topListOf(listed);
  return { label: listLabel(top, libraries, context.t), to: top, isIt };
}

/** The list that gathers what listed says, from every library: Films, Series, or the Libraries page. */
function topListOf(listed: Listed): Route {
  switch (listed) {
    case "movie":
      return { screen: "films" };
    case "series":
      return { screen: "series" };
    case "other":
      return { screen: "browse" };
  }
}

/** The crumb of a series' page, shown at a season, the one an episode is in. */
export function seriesCrumb(seriesId: number, name: string, season?: number): Crumb {
  return {
    label: name,
    to:
      season === undefined
        ? { screen: "series-page", collectionId: seriesId }
        : { screen: "series-page", collectionId: seriesId, season },
    isIt: (route) => route.screen === "series-page" && route.collectionId === seriesId,
  };
}

/** The admin's list of libraries, which a library's own page and creating one go up to (D361). */
export function adminLibrariesCrumb(t: TFunction): Crumb {
  return {
    label: t("libraries.heading"),
    to: { screen: "libraries" },
    isIt: (route) => route.screen === "libraries",
  };
}

/**
 * The crumb of a title's own page: a film's, which its extras and its playing go up to; an episode's,
 * an extra's or an other video's, which its playing's name links to.
 */
export function titleCrumb(titleId: number, name: string): Crumb {
  return {
    label: name,
    to: { screen: "title", itemId: titleId },
    isIt: (route) => route.screen === "title" && route.itemId === titleId,
  };
}

/**
 * The season of its series an episode is shown at: its season in aired order, else its group in the
 * ordering its person chose, none where that ordering leaves it out (the series' default season).
 */
export function seasonOfEpisode(place: EpisodePlace): number | undefined {
  if (place.ordering === undefined) {
    return place.seasonNumber;
  }
  return place.inOrdering?.groupNumber;
}

/** An episode's name in a trail: "S1E1 Pilot", by its place in the ordering its person watches. */
export function episodeHere(t: TFunction, place: EpisodePlace, name: string): string {
  // Aired, or an ordering that leaves it out: its aired place.
  const code =
    place.inOrdering === undefined
      ? t("title.episodeCode", { season: place.seasonNumber, number: place.episodeNumber })
      : placeCode(t, place.ordering, place.inOrdering);
  return t("trail.episode", { code, name });
}

/** An episode's ancestors: its library and its series, at its season. */
export function episodeCrumbs(
  place: EpisodePlace,
  libraries: readonly LibraryName[] | undefined,
  context: LibraryContext,
): Crumb[] {
  return [
    libraryCrumb("series", libraries, context),
    seriesCrumb(place.seriesId, place.seriesName, seasonOfEpisode(place)),
  ];
}
