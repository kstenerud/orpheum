// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { Series, type SeriesDetail } from "./series.tsx";
import { mount, servedBy, settleUntil, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const series: SeriesDetail = {
  id: 20,
  kind: "series",
  name: "Glass Tide",
  libraryId: 1,
  art: {},
  seasons: [
    { seasonNumber: 1, episodeCount: 1 },
    { seasonNumber: 2, episodeCount: 1 },
  ],
};

/** What the page said of its interest: each PUT or DELETE of /interest/20, with its query. */
function interestHeard(heard: readonly Heard[]): string[] {
  return heard
    .filter((request) => request.methodAndPath.includes("/interest/"))
    .map((request) => request.methodAndPath + (request.query ?? ""));
}

describe("usePageInterest", () => {
  it("says the season a series' page shows, and the series' own page none (the owner, 2026-10-24)", async () => {
    for (const { season, query } of [
      { season: 2, query: "?season=2" },
      { season: undefined, query: "" },
    ]) {
      const heard: Heard[] = [];
      const services = servedBy(
        {
          "GET /collections/20": [200, series],
          "GET /collections/20/seasons/2": [200, { episodes: [] }],
          "PUT /interest/20": [204, null],
          "DELETE /interest/20": [204, null],
        },
        heard,
      );
      const page = await mount(withServices(<Series id={20} season={season} />, services));
      expect(interestHeard(heard)).toContain(`PUT /interest/20${query}`);
      page.unmount();
      await settleUntil(() =>
        heard.some((request) => request.methodAndPath === "DELETE /interest/20"),
      );
      expect(interestHeard(heard).at(-1)).toBe(`DELETE /interest/20${query}`);
    }
  });
});
