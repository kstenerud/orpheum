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
  art: {},
  seasons: [
    { seasonNumber: 1, episodeCount: 1 },
    { seasonNumber: 2, episodeCount: 1 },
  ],
};

/** What the page said of its interest: each PUT of /interest/page with its body, each DELETE with its query. */
function interestHeard(heard: readonly Heard[]): string[] {
  return heard
    .filter((request) => request.methodAndPath.endsWith("/interest/page"))
    .map((request) =>
      request.methodAndPath.startsWith("PUT")
        ? `${request.methodAndPath} ${JSON.stringify(request.body)}`
        : request.methodAndPath + (request.query ?? ""),
    );
}

describe("usePageInterest", () => {
  it("says the season a series' page shows, and the series' own page none (the owner, 2026-10-24)", async () => {
    for (const { season, body, query } of [
      { season: 2, body: '{"item":20,"group":2}', query: "?item=20&group=2" },
      { season: undefined, body: '{"item":20}', query: "?item=20" },
    ]) {
      const heard: Heard[] = [];
      const services = servedBy(
        {
          "GET /items/20": [200, series],
          "GET /collections/20/seasons/1": [200, { episodes: [] }],
          "GET /collections/20/seasons/2": [200, { episodes: [] }],
          "PUT /interest/page": [204, null],
          "DELETE /interest/page": [204, null],
        },
        heard,
      );
      const page = await mount(withServices(<Series id={20} season={season} />, services));
      expect(interestHeard(heard)).toContain(`PUT /interest/page ${body}`);
      page.unmount();
      await settleUntil(() =>
        heard.some((request) => request.methodAndPath === "DELETE /interest/page"),
      );
      expect(interestHeard(heard).at(-1)).toBe(`DELETE /interest/page${query}`);
    }
  });
});
