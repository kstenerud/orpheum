// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { SeasonView, SeriesView, type SeriesDetail } from "./series.tsx";
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

const loaded = <T,>(value: T) => ({ state: "loaded" as const, value });

describe("SeriesView", () => {
  it("shows one season at a time, the first where nothing is chosen, picked from the rest (D367 §9)", () => {
    const series: SeriesDetail = {
      id: 20,
      kind: "series",
      name: "Glass Tide",
      art: { posterSha256: "p" },
      seasons: [
        { seasonNumber: 1, episodeCount: 2, stillSha256: "s31" },
        { seasonNumber: 2, episodeCount: 1 },
        { seasonNumber: 0, episodeCount: 1 },
      ],
    };
    // One series page whose season is a view, picked by a chooser a remote reaches (D367 §9; the
    // owner, 2026-10-27), in place of a page of seasons each leading to its own.
    const markup = renderToStaticMarkup(withServices(<SeriesView series={series} />));
    expect(markup).toContain(">Season 1</h2>");
    expect(markup).toContain('aria-label="Season: Season 1"');
    expect(markup).not.toContain("Season 2");
    expect(markup).not.toContain("Specials");
    // A season chosen names the season; its episodes are read apart.
    const season = renderToStaticMarkup(withServices(<SeriesView series={series} season={0} />));
    expect(season).toContain(">Specials</h2>");
    expect(season).toContain('aria-label="Season: Specials"');
    expect(season).not.toContain("Season 1");
  });
});

describe("a series' next episode (the owner, 2026-10-25)", () => {
  const series: SeriesDetail = {
    id: 20,
    kind: "series",
    name: "Glass Tide",
    art: {},
    seasons: [
      { seasonNumber: 1, episodeCount: 4 },
      { seasonNumber: 2, episodeCount: 1 },
    ],
  };
  const next = {
    titleId: 33,
    name: "Low Water",
    groupNumber: 1,
    numberInGroup: 3,
    fileId: 7,
    isPartway: false,
  };

  it("continues the one partway through, playing its file, which resumes where it was", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <SeriesView
          series={{ ...series, nextUp: { ...next, isPartway: true, positionMs: 600_000 } }}
        />,
      ),
    );
    expect(markup).toContain('href="/play/7"');
    expect(markup).toContain(">Continue S1E3</a>");
    expect(markup).not.toContain("Play S1E3");
  });

  it("plays the one after the last finished, or the first of a series never watched", () => {
    const after = renderToStaticMarkup(
      withServices(
        <SeriesView
          series={{ ...series, nextUp: { ...next, numberInGroup: 4, fileId: 8, titleId: 34 } }}
        />,
      ),
    );
    expect(after).toContain('href="/play/8"');
    expect(after).toContain(">Play S1E4</a>");
    const first = renderToStaticMarkup(
      withServices(
        <SeriesView series={{ ...series, nextUp: { ...next, numberInGroup: 1, fileId: 5 } }} />,
      ),
    );
    expect(first).toContain(">Play S1E1</a>");
  });

  it("offers none when the series has no next episode, and offers it at every season", () => {
    const none = renderToStaticMarkup(withServices(<SeriesView series={series} />));
    expect(none).not.toContain("/play/");
    // One page whatever season it shows (D367 §9).
    const season = renderToStaticMarkup(
      withServices(<SeriesView series={{ ...series, nextUp: next }} season={2} />),
    );
    expect(season).toContain('href="/play/7"');
  });

  it("shows the season of the next episode where none is chosen", () => {
    const markup = renderToStaticMarkup(
      withServices(<SeriesView series={{ ...series, nextUp: { ...next, groupNumber: 2 } }} />),
    );
    expect(markup).toContain(">Season 2</h2>");
  });

  it("says why it cannot be played now, as its own page does, and offers no Play", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <SeriesView
          series={{
            ...series,
            nextUp: { ...next, unavailable: { reason: "still_reading", nodeName: "devnas" } },
          }}
        />,
      ),
    );
    expect(markup).not.toContain("/play/");
    expect(markup).toContain(
      "S1E3: This is still being read: devnas is reading it now, and it plays as soon as that is done.",
    );
    expect(markup).toContain("series__absent");
  });
});

describe("SeasonView", () => {
  it("lists a season's episodes, each with its still and whether it is on disk, and offers more", () => {
    const page = {
      items: [
        { ...episode(31, { seasonNumber: 1, episodeNumber: 1 }), stillSha256: "s31" },
        episode(32, { seasonNumber: 1, episodeNumber: 2 }, false),
      ],
      nextCursor: "10002",
    };
    const markup = renderToStaticMarkup(
      withServices(
        <SeasonView
          loadedEpisodes={loaded(page)}
          isLoadingMore={false}
          onLoadMore={() => undefined}
        />,
      ),
    );
    expect(markup).toContain('src="/api/v1/bytes/art/s31/landscape-s.webp"');
    expect(markup).toContain("not on disk");
    expect(markup).toContain('href="/titles/32"');
    expect(markup).toContain('class="end-of-list"');
    const last = renderToStaticMarkup(
      withServices(
        <SeasonView
          loadedEpisodes={loaded({ ...page, nextCursor: "" })}
          isLoadingMore={false}
          onLoadMore={() => undefined}
        />,
      ),
    );
    expect(last).not.toContain('class="end-of-list"');
  });
});

describe("an episode that cannot be played now (D241 §3)", () => {
  it("is listed, marked with why", () => {
    const page = {
      items: [
        {
          ...episode(31, { seasonNumber: 1, episodeNumber: 1 }),
          unavailable: { reason: "node_away" as const, nodeName: "devnas" },
        },
      ],
      nextCursor: "",
    };
    const markup = renderToStaticMarkup(
      withServices(
        <SeasonView
          loadedEpisodes={loaded(page)}
          isLoadingMore={false}
          onLoadMore={() => undefined}
        />,
      ),
    );
    expect(markup).toContain("Episode 1");
    expect(markup).toContain(
      "This cannot be played now: the server cannot reach devnas, the computer the film is on. Check that it is on and connected.",
    );
    expect(markup).toContain("series__item--absent");
  });
});

describe("a series of one season", () => {
  it("shows that season at once, with no season to choose", () => {
    const series: SeriesDetail = {
      id: 21,
      kind: "series",
      name: "Short Run",
      art: {},
      seasons: [{ seasonNumber: 1, episodeCount: 1 }],
    };
    const markup = renderToStaticMarkup(withServices(<SeriesView series={series} />));
    expect(markup).toContain(">Season 1</h2>");
    expect(markup).not.toContain("All seasons");
  });
});

describe("a series whose match is a guess (D301)", () => {
  it("is marked over its poster, and offers an admin alone to correct it", () => {
    const series: SeriesDetail = {
      id: 20,
      kind: "series",
      name: "The Ledger Room",
      art: { posterSha256: "p" },
      seasons: [],
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
