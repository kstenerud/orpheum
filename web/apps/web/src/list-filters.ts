// SPDX-License-Identifier: MPL-2.0

import type { paths } from "@orpheum/sdk";
import type { TFunction } from "i18next";

import type { ListFilters } from "./router.ts";
import {
  defaultOrderOf,
  sortChoiceLabel,
  sortFieldLabel,
  type SortChoice,
} from "./sort-choices.ts";

/** The chips as GET /items names them. */
type TitlesChipsQuery = Pick<
  NonNullable<paths["/items"]["get"]["parameters"]["query"]>,
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

/** The chips as GET /items and GET /items/letters take them, each left out when not chosen. */
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

/**
 * What narrows and orders a list, all of it one pop-up's (D417 §1): the text its posters' names are
 * narrowed by, as typed; its sort, run its way; and its chips. Each adds to the others.
 */
export interface ListCriteria {
  /** The text as typed, "" for none; the list is narrowed by it trimmed. */
  readonly query: string;
  readonly choice: SortChoice;
  readonly filters: ListFilters;
}

/** The criteria as one string, for a list's key: other criteria are another list. */
export function criteriaKey(criteria: ListCriteria): string {
  const { query, choice, filters } = criteria;
  return `${choice.sort}/${choice.order}/${query.trim()}/${filtersKey(filters)}`;
}

/** Whether anything narrows the list: its text or a chip. */
export function isNarrowed(criteria: ListCriteria): boolean {
  return criteria.query.trim() !== "" || hasFilters(criteria.filters);
}

/** The criteria with every filter cleared, its text and its chips, its sort kept (D417). */
export function clearedCriteria(criteria: ListCriteria): ListCriteria {
  return { query: "", choice: criteria.choice, filters: {} };
}

/**
 * The criteria in a few words on one line, the pop-up's button's: the sort, named by its field alone
 * where it runs its own way ("Name"), then the text in quotes, the genre, the decade and Unwatched,
 * each only when chosen: "Name · “heat” · Drama · 1990s · Unwatched". genreName is the chosen genre's,
 * undefined until its name is read, the genre left out until then.
 */
export function criteriaSummary(
  t: TFunction,
  criteria: ListCriteria,
  genreName: string | undefined,
): string {
  const { query, choice, filters } = criteria;
  const text = query.trim();
  return [
    choice.order === defaultOrderOf(choice.sort)
      ? sortFieldLabel(t, choice.sort)
      : sortChoiceLabel(t, choice),
    ...(text === "" ? [] : [t("criteria.quoted", { text })]),
    ...(filters.genre === undefined || genreName === undefined ? [] : [genreName]),
    ...(filters.decade === undefined ? [] : [t("chips.decadeName", { decade: filters.decade })]),
    ...(filters.isUnwatchedOnly === true ? [t("chips.unwatched")] : []),
  ].join(" · ");
}
