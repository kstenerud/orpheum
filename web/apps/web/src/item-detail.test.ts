// SPDX-License-Identifier: MPL-2.0

import { ApiProblem } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { seriesOrThrow, titleOrThrow, type ItemDetail } from "./item-detail.ts";

const film: ItemDetail = {
  id: 10,
  kind: "movie",
  name: "Metropolis",
  identificationState: "matched",
  versions: [],
  extras: [],
  art: {},
};

const series: ItemDetail = {
  id: 20,
  kind: "series",
  name: "Brass Arena",
  seasons: [],
  art: {},
};

describe("one item's detail, narrowed on its kind (D416 §2)", () => {
  it("is a title's on a title's page and a series' on a series'", () => {
    expect(titleOrThrow(film).versions).toEqual([]);
    expect(seriesOrThrow(series).seasons).toEqual([]);
  });

  it("is no such title for a series, and no such series for a title", () => {
    for (const refused of [() => titleOrThrow(series), () => seriesOrThrow(film)]) {
      expect(refused).toThrow(ApiProblem);
      try {
        refused();
      } catch (error) {
        expect((error as ApiProblem).code).toBe("not_found");
      }
    }
  });
});
