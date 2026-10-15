// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { bySeason, SeriesView, type SeriesDetail } from "./series.tsx";
import { withServices } from "./test-services.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

const episode = (
  id: number,
  place: { seasonNumber: number; episodeNumber: number },
  isAvailable = true,
) => ({
  id,
  seasonNumber: place.seasonNumber,
  episodeNumber: place.episodeNumber,
  name: `Episode ${String(place.episodeNumber)}`,
  identificationState: "identified",
  hasLiveFile: isAvailable,
});

describe("SeriesView", () => {
  it("groups episodes by season, specials last, each with its still and whether it is on disk", () => {
    const series: SeriesDetail = {
      id: 20,
      kind: "series",
      name: "Glass Tide",
      libraryId: 1,
      art: { posterSha256: "p" },
      episodes: [
        { ...episode(31, { seasonNumber: 1, episodeNumber: 1 }), stillSha256: "s31" },
        episode(32, { seasonNumber: 1, episodeNumber: 2 }, false),
        episode(40, { seasonNumber: 0, episodeNumber: 1 }),
        episode(33, { seasonNumber: 2, episodeNumber: 1 }),
      ],
    };
    // The series' page lists its seasons, specials last, each with a still and its count, leading
    // to its episodes (the owner, 2026-10-07: series, then seasons, then episodes).
    const markup = renderToStaticMarkup(withServices(<SeriesView series={series} />));
    expect(markup.indexOf("Season 1")).toBeLessThan(markup.indexOf("Season 2"));
    expect(markup.indexOf("Season 2")).toBeLessThan(markup.indexOf("Specials"));
    expect(markup).toContain('src="/api/v1/bytes/art/s31/landscape-s.webp"');
    expect(markup).toContain("Episodes: 2");
    expect(markup).toContain('href="/series/20?season=1"');
    expect(markup).not.toContain('href="/titles/32"');
    // A season's page lists its episodes, each with its still and whether it is on disk.
    const season = renderToStaticMarkup(withServices(<SeriesView series={series} season={1} />));
    expect(season).toContain('src="/api/v1/bytes/art/s31/landscape-s.webp"');
    expect(season).toContain("not on disk");
    expect(season).toContain('href="/titles/32"');
    expect(season).not.toContain('href="/titles/33"');
    expect(season).toContain('href="/series/20"');
    expect(bySeason([]).length).toBe(0);
  });
});

describe("an episode that cannot be played now (D241 §3)", () => {
  it("is listed, marked with why", () => {
    const series: SeriesDetail = {
      id: 20,
      kind: "series",
      name: "Glass Tide",
      libraryId: 1,
      art: {},
      episodes: [
        {
          ...episode(31, { seasonNumber: 1, episodeNumber: 1 }),
          unavailable: { reason: "node_away", nodeName: "devnas" },
        },
      ],
    };
    const markup = renderToStaticMarkup(withServices(<SeriesView series={series} />));
    expect(markup).toContain("Episode 1");
    expect(markup).toContain(
      "This cannot be played now: devnas, the machine that holds it, is not connected.",
    );
    expect(markup).toContain("series__item--absent");
  });
});

describe("a series of one season", () => {
  it("shows its episodes at once, with no season to choose", () => {
    const series: SeriesDetail = {
      id: 21,
      kind: "series",
      name: "Short Run",
      libraryId: 1,
      art: {},
      episodes: [episode(50, { seasonNumber: 1, episodeNumber: 1 })],
    };
    const markup = renderToStaticMarkup(withServices(<SeriesView series={series} />));
    expect(markup).toContain('href="/titles/50"');
    expect(markup).not.toContain("All seasons");
  });
});

describe("a series whose match is a guess (D301)", () => {
  it("is marked over its poster, and offers an admin alone to correct it", () => {
    const series: SeriesDetail = {
      id: 20,
      kind: "series",
      name: "The Ledger Room",
      libraryId: 1,
      art: { posterSha256: "p" },
      episodes: [],
      isGuessed: true,
    };
    const corrected = (): void => undefined;
    const forUser = renderToStaticMarkup(
      withServices(<SeriesView series={series} onMatchCorrected={corrected} />),
    );
    const forAdmin = renderToStaticMarkup(
      withServices(
        <IsAdminContext value={true}>
          <SeriesView series={series} onMatchCorrected={corrected} />
        </IsAdminContext>,
      ),
    );
    expect(forUser).toContain("may be another work of the same name");
    expect(forUser).not.toContain(">Identified incorrectly<");
    expect(forAdmin).toContain(">Identified incorrectly<");
    expect(forAdmin).toContain("the likeliest was taken");
    expect(
      renderToStaticMarkup(withServices(<SeriesView series={{ ...series, isGuessed: false }} />)),
    ).not.toContain("This may be identified incorrectly");
  });
});
