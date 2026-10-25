// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { rowName, type EpisodeNaming } from "./continue-watching.tsx";
import { SeriesView, type SeriesDetail } from "./series.tsx";
import { choose, mount, servedBy, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { TitleView, titleUp, type TitleDetail } from "./title.tsx";

const aired = { name: "aired", kind: "aired", episodeCount: 4 };
const dvd = {
  name: "tmdb:dvd",
  kind: "dvd",
  label: "DVD Order",
  listedCount: 5,
  episodeCount: 3,
};

const nextUp = {
  titleId: 21,
  name: "Pilot",
  groupNumber: 1,
  numberInGroup: 2,
  fileId: 2,
  isPartway: false,
};

const inAired: SeriesDetail = {
  id: 20,
  kind: "series",
  name: "Glass Tide",
  libraryId: 1,
  art: {},
  orderings: [aired, dvd],
  seasons: [
    { seasonNumber: 1, name: "Disc 1", episodeCount: 2 },
    { seasonNumber: 2, episodeCount: 1 },
  ],
  notInOrderingCount: 1,
  nextUp: { ...nextUp, groupName: "Disc 1" },
};

const inDVDOrder: SeriesDetail = { ...inAired, ordering: "tmdb:dvd" };

describe("a series in the ordering its person chose (episode-orderings.md §4)", () => {
  it("heads its seasons with the ordering's groups, labels Next Up there, and lists what it leaves out", () => {
    const markup = renderToStaticMarkup(withServices(<SeriesView series={inDVDOrder} />));
    expect(markup.indexOf("Disc 1")).toBeLessThan(markup.indexOf("Part 2"));
    expect(markup).toContain(">Play Disc 1 · 2</a>");
    expect(markup).toContain(">Not in this order</h2>");
    expect(markup).not.toContain("Season 1");
  });

  it("labels one run's Next Up by its number alone", () => {
    const absolute: SeriesDetail = {
      ...inDVDOrder,
      seasons: [{ seasonNumber: 1, episodeCount: 3 }],
      nextUp: { ...nextUp, numberInGroup: 812 },
    };
    expect(renderToStaticMarkup(withServices(<SeriesView series={absolute} />))).toContain(
      ">Play Episode 812</a>",
    );
  });

  it("offers its orderings, aired first, each with its kind and how many it holds; none when it has one", () => {
    const markup = renderToStaticMarkup(withServices(<SeriesView series={inDVDOrder} />));
    expect(markup).toContain('<option value="aired">Aired</option>');
    expect(markup).toContain(
      '<option value="tmdb:dvd" selected="">DVD Order (DVD) · episodes here: 3</option>',
    );
    expect(markup.indexOf('value="aired"')).toBeLessThan(markup.indexOf('value="tmdb:dvd"'));
    const listedOnly = renderToStaticMarkup(
      withServices(
        <SeriesView series={{ ...inDVDOrder, orderings: [aired, { ...dvd, episodeCount: 0 }] }} />,
      ),
    );
    expect(listedOnly).toContain("DVD Order (DVD) · episodes listed: 5");
    const onlyAired = renderToStaticMarkup(
      withServices(<SeriesView series={{ ...inAired, orderings: [aired] }} />),
    );
    expect(onlyAired).not.toContain("<select");
    expect(
      renderToStaticMarkup(withServices(<SeriesView series={inDVDOrder} season={1} />)),
    ).not.toContain("<select");
  });

  it("keeps a choice, goes to the series' top page and reads it again (§7 Q10)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "PUT /collections/20/preferences": [204, null],
        "GET /collections/20/not-in-order": [200, { episodes: [] }],
      },
      heard,
      "token",
    );
    services.navigation.go({ screen: "series-page", collectionId: 20, season: 2 });
    let reads = 0;
    const page = await mount(
      withServices(
        <SeriesView
          series={inDVDOrder}
          onOrderingChosen={() => {
            reads++;
          }}
        />,
        services,
      ),
    );
    await choose(page.container.querySelector("select"), "aired");
    page.unmount();
    const puts = heard.filter((each) => each.methodAndPath.startsWith("PUT "));
    expect(puts.map((each) => [each.methodAndPath, each.body])).toEqual([
      ["PUT /collections/20/preferences", { episodeOrdering: "aired" }],
    ]);
    expect(services.navigation.route()).toEqual({ screen: "series-page", collectionId: 20 });
    expect(reads).toBe(1);
  });
});

describe("an episode in the ordering its person chose (§7 Q8)", () => {
  const placeInDVDOrder = {
    seriesId: 20,
    seriesName: "Glass Tide",
    seasonNumber: 1,
    episodeNumber: 1,
    ordering: "tmdb:dvd",
    inOrdering: { groupNumber: 1, numberInGroup: 2, groupName: "Disc 1" },
  };
  const pilot: TitleDetail = {
    id: 21,
    kind: "episode",
    name: "Pilot",
    identificationState: "identified",
    libraryId: 1,
    art: {},
    versions: [],
    extras: null,
    episode: placeInDVDOrder,
  };

  it("is placed in its group there, a link up, with its aired place on a second line", () => {
    const markup = renderToStaticMarkup(withServices(<TitleView title={pilot} />));
    expect(markup).toContain('<a href="/series/20?season=1">Disc 1</a> · 2');
    expect(markup).toContain("Aired as S1E1");
    expect(titleUp(pilot)).toEqual({ screen: "series-page", collectionId: 20, season: 1 });
  });

  it("says when the ordering leaves it out, and goes up to the series' top", () => {
    const left: TitleDetail = {
      ...pilot,
      episode: {
        seriesId: 20,
        seriesName: "Glass Tide",
        seasonNumber: 1,
        episodeNumber: 1,
        ordering: "tmdb:dvd",
      },
    };
    const markup = renderToStaticMarkup(withServices(<TitleView title={left} />));
    expect(markup).toContain("Not in the order you watch this series in");
    expect(markup).toContain("Aired as S1E1");
    expect(titleUp(left)).toEqual({ screen: "series-page", collectionId: 20 });
  });

  it("is named in Continue Watching by its place there", () => {
    const named: EpisodeNaming[] = [];
    rowName(
      { name: "Pilot", episode: placeInDVDOrder },
      (episode) => {
        named.push(episode);
        return "";
      },
      (ordering, place) =>
        `${ordering} ${String(place.groupNumber)}/${String(place.numberInGroup)}`,
    );
    expect(named[0]?.place).toBe("tmdb:dvd 1/2");
  });
});
