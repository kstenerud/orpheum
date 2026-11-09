// SPDX-License-Identifier: MPL-2.0

import { ApiProblem, type components } from "@orpheum/sdk";

type TitleBody = components["schemas"]["ItemBody"];
type SeriesBody = components["schemas"]["SeriesBody"];

/** One item's detail as GET /items/{id} answers it: a union on kind (D416 §2, D425 §7). */
export type ItemDetail = TitleBody | SeriesBody;

/**
 * The item as a title's detail, for a page that shows a title: a series is no such title, refused
 * as the server refused one before every kind answered at one address.
 */
export function titleOrThrow(item: ItemDetail): TitleBody {
  if (item.kind === "series") {
    throw noSuch(item.id, "a series, not a title");
  }
  return item;
}

/** The item as a series' detail, for a page that shows a series: a title is no such series. */
export function seriesOrThrow(item: ItemDetail): SeriesBody {
  if (item.kind !== "series") {
    throw noSuch(item.id, "a title, not a series");
  }
  return item;
}

function noSuch(id: number, what: string): ApiProblem {
  return new ApiProblem({ code: "not_found", message: `item ${String(id)} is ${what}` });
}
