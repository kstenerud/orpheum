// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { bySeason, SeriesView, type SeriesDetail } from "./series.tsx";
import { withServices } from "./test-services.tsx";

const episode = (id: number, place: { season: number; number: number }, isAvailable = true) => ({
  id,
  season: place.season,
  number: place.number,
  name: `Episode ${String(place.number)}`,
  ident: "identified",
  available: isAvailable,
});

describe("SeriesView", () => {
  it("groups episodes by season, specials last, each with its still and whether it is on disk", () => {
    const series: SeriesDetail = {
      id: 20,
      kind: "series",
      name: "Glass Tide",
      libraryId: 1,
      art: { poster: "p" },
      episodes: [
        { ...episode(31, { season: 1, number: 1 }), still: "s31" },
        episode(32, { season: 1, number: 2 }, false),
        episode(40, { season: 0, number: 1 }),
        episode(33, { season: 2, number: 1 }),
      ],
    };
    const markup = renderToStaticMarkup(withServices(<SeriesView series={series} />));
    expect(markup.indexOf("Season 1")).toBeLessThan(markup.indexOf("Season 2"));
    expect(markup.indexOf("Season 2")).toBeLessThan(markup.indexOf("Specials"));
    expect(markup).toContain('src="/api/v1/art/s31/landscape-s.webp"');
    expect(markup).toContain("not on disk");
    expect(markup).toContain('href="/titles/32"');
    expect(bySeason([]).length).toBe(0);
  });
});
