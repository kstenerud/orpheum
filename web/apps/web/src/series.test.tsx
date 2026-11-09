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

/** The markup of each poster in a series page's row of seasons, in its order; none where it has none. */
function seasonCards(markup: string): string[] {
  const row = /<ul class="poster-row series__seasons" aria-label="Seasons"[^>]*>(.*?)<\/ul>/su.exec(
    markup,
  )?.[1];
  return [...(row ?? "").matchAll(/<li class="poster-card">(.*?)<\/li>/gsu)].map(
    (card) => card[1] ?? "",
  );
}

describe("SeriesView", () => {
  it("shows one season at a time, the first where nothing is chosen, picked from the rest (D367 §9)", () => {
    const series: SeriesDetail = {
      id: 20,
      kind: "series",
      name: "Glass Tide",
      art: { posterSha256: "p" },
      seasons: [
        { seasonNumber: 1, episodeCount: 2, stillSha256: "s31" },
        { seasonNumber: 2, episodeCount: 1, isWatched: true },
        { seasonNumber: 0, episodeCount: 1 },
      ],
    };
    // One series page whose season is a view (D367 §9), picked from a row of its seasons' posters
    // above its episodes, in place of a choice box (D379 §1; the owner, 2026-10-28).
    const markup = renderToStaticMarkup(withServices(<SeriesView series={series} />));
    expect(markup).toContain(">Season 1</h2>");
    expect(markup).not.toContain(">Season 2</h2>");
    expect(markup).not.toContain("route-chooser");
    const cards = seasonCards(markup);
    // In the server's order, the specials last, each leading to its season.
    expect(cards.map((card) => /href="([^"]*)"/u.exec(card)?.[1])).toEqual([
      "/series/20?season=1",
      "/series/20?season=2",
      "/series/20?season=0",
    ]);
    expect(cards[2]).toContain("Specials");
    expect(cards[0]).toContain("episodes: 2");
    // The season shown is marked; the others are not.
    expect(cards[0]).toContain('aria-current="page"');
    expect(cards[1]).not.toContain('aria-current="page"');
    // A season with a still shows it in its poster's place (D362 §2); one without, the series' poster.
    expect(cards[0]).toContain("/bytes/art/s31/landscape-s.webp");
    expect(cards[0]).not.toContain("/bytes/art/p/portrait-m.webp");
    expect(cards[1]).toContain("/bytes/art/p/portrait-m.webp");
    // A season watched whole shows its check over its poster (D370 §3).
    expect(cards[1]).toContain("poster-card__cues");
    expect(cards[0]).not.toContain("poster-card__cues");
    // A season chosen names the season; its episodes are read apart.
    const season = renderToStaticMarkup(withServices(<SeriesView series={series} season={0} />));
    expect(season).toContain(">Specials</h2>");
    expect(season).not.toContain(">Season 1</h2>");
    const chosen = seasonCards(season);
    expect(chosen[2]).toContain('aria-current="page"');
    expect(chosen[0]).not.toContain('aria-current="page"');
  });

  it("shows a season's own poster, and says No poster only where a season has nothing (D379 §1)", () => {
    const series: SeriesDetail = {
      id: 20,
      kind: "series",
      name: "Glass Tide",
      art: { posterSha256: "p" },
      seasons: [
        { seasonNumber: 1, episodeCount: 2, posterSha256: "sp1", stillSha256: "s31" },
        { seasonNumber: 2, episodeCount: 1, stillSha256: "s41" },
        { seasonNumber: 3, episodeCount: 1 },
      ],
    };
    const cards = seasonCards(renderToStaticMarkup(withServices(<SeriesView series={series} />)));
    // Its own poster, over its still and its series' poster.
    expect(cards[0]).toContain("/bytes/art/sp1/portrait-m.webp");
    expect(cards[0]).not.toContain("/bytes/art/s31/");
    expect(cards[0]).not.toContain("No poster");
    // None of its own: its still in its poster's place, a picture of it, so no caption.
    expect(cards[1]).toContain("/bytes/art/s41/landscape-s.webp");
    expect(cards[1]).not.toContain("No poster");
    // Neither: its series' poster.
    expect(cards[2]).toContain("/bytes/art/p/portrait-m.webp");
    expect(cards[2]).not.toContain("No poster");
    // A series with no poster at all: the season with nothing is the one named a blank.
    const bare = seasonCards(
      renderToStaticMarkup(withServices(<SeriesView series={{ ...series, art: {} }} />)),
    );
    expect(bare[1]).not.toContain("No poster");
    expect(bare[2]).toContain("No poster");
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
    itemId: 33,
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
          series={{ ...series, nextUp: { ...next, numberInGroup: 4, fileId: 8, itemId: 34 } }}
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

describe("an episode watched (D370 §2)", () => {
  // The owner, 2026-10-28: "I think the checkmark should be over the screenshot", as on a poster's
  // tray, not beside the row.
  it("is checked over its still, and one not watched is not", () => {
    const page = {
      items: [
        { ...episode(31, { seasonNumber: 1, episodeNumber: 1 }), isWatched: true },
        episode(32, { seasonNumber: 1, episodeNumber: 2 }),
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
    const frames = [
      ...markup.matchAll(/<span class="series__still-frame">(.*?<\/span>)<\/span><span>/gu),
    ].map((frame) => frame[1] ?? "");
    expect(frames).toHaveLength(2);
    expect(frames[0]).toContain('class="series__still"');
    expect(frames[0]).toContain('class="watched-row-mark"');
    expect(frames[1]).not.toContain("watched-row-mark");
    expect(markup.match(/watched-row-mark/gu)).toHaveLength(1);
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
    expect(markup).not.toContain("series__seasons");
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
    // Under the "…" of what is rarely used (the owner, 2026-10-29), which a person with nothing
    // there is not shown.
    expect(forUser).not.toContain('aria-label="More actions"');
    expect(forAdmin).toContain('aria-label="More actions"');
    expect(forAdmin).toContain("the likeliest was taken");
    expect(
      renderToStaticMarkup(withServices(<SeriesView series={{ ...series, isGuessed: false }} />)),
    ).not.toContain("This may be identified incorrectly");
  });
});
