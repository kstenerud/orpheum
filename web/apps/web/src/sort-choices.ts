// SPDX-License-Identifier: MPL-2.0

import type { TFunction } from "i18next";

import type { LibrarySort, SortOrder } from "./router.ts";

/** A list's order as a person chooses it: a sort, and the way it runs (D366 §2). */
export interface SortChoice {
  readonly sort: LibrarySort;
  readonly order: SortOrder;
}

/** Whether a sort is by a name, a title's or its file's: the alphabet rail's (D349 §9). */
export function isByName(sort: LibrarySort): boolean {
  return sort === "name" || sort === "fileName";
}

/** The way a sort runs unless the person turns it: A to Z by a name, newest first by a date. */
export function defaultOrderOf(sort: LibrarySort): SortOrder {
  return isByName(sort) ? "ascending" : "descending";
}

/** A sort's order as asked for, its own default when none is: what GET /items is asked. */
export function orderOf(sort: LibrarySort, order: SortOrder | undefined): SortOrder {
  return order ?? defaultOrderOf(sort);
}

/**
 * The order GET /items is sent: none for the sort's own way, so a list run its own way asks as it
 * always has, the other way named.
 */
export function orderQuery(choice: SortChoice): { readonly order?: SortOrder } {
  return choice.order === defaultOrderOf(choice.sort) ? {} : { order: choice.order };
}

/**
 * Every way each sort runs, as the chooser offers them: each field ascending, then descending, the
 * same for every field (the owner, 2026-10-30: "up arrow, then down arrow for each sort field").
 */
export function sortChoicesOf(sorts: readonly LibrarySort[]): SortChoice[] {
  return sorts.flatMap((sort): SortChoice[] => [
    { sort, order: "ascending" },
    { sort, order: "descending" },
  ]);
}

/** Whether two choices are the same sort run the same way. */
export function isSameChoice(first: SortChoice, second: SortChoice): boolean {
  return first.sort === second.sort && first.order === second.order;
}

/** A sort and the way it runs in the person's words: "Name, A–Z", "Newest added". */
export function sortChoiceLabel(t: TFunction, choice: SortChoice): string {
  return choice.order === "ascending"
    ? ascendingLabel(t, choice.sort)
    : descendingLabel(t, choice.sort);
}

/** A sort's words run ascending: A to Z, or oldest first. */
function ascendingLabel(t: TFunction, sort: LibrarySort): string {
  switch (sort) {
    case "name":
      return t("browse.byNameAToZ");
    case "fileName":
      return t("browse.byFileNameAToZ");
    case "added":
      return t("browse.byAddedOldest");
    case "recorded":
      return t("browse.byRecordedOldest");
    case "released":
      return t("browse.byReleasedOldest");
  }
}

/** A sort's words run descending: Z to A, or newest first. */
function descendingLabel(t: TFunction, sort: LibrarySort): string {
  switch (sort) {
    case "name":
      return t("browse.byNameZToA");
    case "fileName":
      return t("browse.byFileNameZToA");
    case "added":
      return t("browse.byAddedNewest");
    case "recorded":
      return t("browse.byRecordedNewest");
    case "released":
      return t("browse.byReleasedNewest");
  }
}

/** A sort's field in the person's words, without its way: "Name", "Released", "Added". */
export function sortFieldLabel(t: TFunction, sort: LibrarySort): string {
  switch (sort) {
    case "name":
      return t("browse.fieldName");
    case "fileName":
      return t("browse.fieldFileName");
    case "added":
      return t("browse.fieldAdded");
    case "recorded":
      return t("browse.fieldRecorded");
    case "released":
      return t("browse.fieldReleased");
  }
}
