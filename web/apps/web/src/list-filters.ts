// SPDX-License-Identifier: MPL-2.0

import type { paths } from "@orpheum/sdk";

import type { ListFilters } from "./router.ts";

/** The chips as GET /titles names them. */
type TitlesChipsQuery = Pick<
  NonNullable<paths["/titles"]["get"]["parameters"]["query"]>,
  "genre" | "decade" | "unwatched"
>;

/**
 * The decades Films' and Series' Decade chip offers, newest first: a fixed list, as nothing asks the
 * server which decades a list has titles in (search-by-more §4.3 allows either).
 */
export const decades: readonly number[] = [
  2020, 2010, 2000, 1990, 1980, 1970, 1960, 1950, 1940, 1930, 1920,
];

/** The chips of a list's state alone, its sort and search left out. */
export function chipsOf(state: {
  readonly [Chip in keyof ListFilters]?: ListFilters[Chip] | undefined;
}): ListFilters {
  return {
    ...(state.genre === undefined ? {} : { genre: state.genre }),
    ...(state.decade === undefined ? {} : { decade: state.decade }),
    ...(state.isUnwatchedOnly === true ? { isUnwatchedOnly: true } : {}),
  };
}

/** The chips as GET /titles and GET /titles/letters take them, each left out when not chosen. */
export function filtersQuery(chips: {
  readonly [Chip in keyof ListFilters]?: ListFilters[Chip] | undefined;
}): TitlesChipsQuery {
  return {
    ...(chips.genre === undefined ? {} : { genre: chips.genre }),
    ...(chips.decade === undefined ? {} : { decade: chips.decade }),
    ...(chips.isUnwatchedOnly === true ? { unwatched: true } : {}),
  };
}

/** The chips as one string, for a list's key: another chip chosen is another list. */
export function filtersKey(filters: ListFilters | undefined): string {
  if (filters === undefined) {
    return "";
  }
  const unwatchedOnly = filters.isUnwatchedOnly === true ? "unwatched" : "";
  return `${String(filters.genre ?? "")}/${String(filters.decade ?? "")}/${unwatchedOnly}`;
}

/** Whether any chip is chosen. */
export function hasFilters(filters: ListFilters): boolean {
  return filtersKey(filters) !== "//";
}
