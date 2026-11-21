// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

/**
 * A title's genres on its page (b2.md Phase A): a film's and a series' page show them under the
 * name, among its facts, in the order the server sends, each opening Films or Series filtered by it
 * as the list pop-up's own genre chip does; an episode's page and a title with none show no line.
 */

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { routeHref } from "./router.ts";
import { SeriesView, type SeriesDetail } from "./series.tsx";
import { click, mount, type Mounted } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";
import { TitleView, type TitleDetail } from "./title.tsx";

const crimeThenDrama = [
  { id: 80, name: "Crime" },
  { id: 18, name: "Drama" },
];

/** A film the server sends no genres for. */
const ungenredFilm: TitleDetail = {
  id: 10,
  kind: "film",
  name: "Metropolis",
  year: 1927,
  runtimeMs: 170 * 60_000,
  identificationState: "identified",
  art: {},
  versions: null,
  extras: null,
};

const metropolis: TitleDetail = { ...ungenredFilm, genres: crimeThenDrama };

/** A series the server sends no genres for. */
const ungenredSeries: SeriesDetail = {
  id: 20,
  kind: "series",
  name: "Glass Tide",
  year: 2019,
  art: {},
  seasons: [{ seasonNumber: 1, episodeCount: 1 }],
};

const glassTide: SeriesDetail = { ...ungenredSeries, genres: crimeThenDrama };

/** Each genre link a page's facts show, its words and where it leads, in order. */
function genresIn(container: ParentNode): string[] {
  return [...container.querySelectorAll(".title-head__facts .title-head__genres a")].map(
    (link) => `${link.textContent}|${link.getAttribute("href") ?? ""}`,
  );
}

describe("a title's genres", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows a film's genres in the server's order, and one opens Films filtered by it", async () => {
    const services = testServices(undefined, "/titles/10");
    mounted = await mount(withServices(<TitleView title={metropolis} />, services));
    expect(genresIn(mounted.container)).toEqual([
      `Crime|${routeHref({ screen: "films", genre: 80 })}`,
      `Drama|${routeHref({ screen: "films", genre: 18 })}`,
    ]);
    expect(mounted.container.querySelector(".title-head__genres")?.textContent).toBe(
      "Crime, Drama",
    );
    await click(mounted.container.querySelectorAll(".title-head__genres a")[1]);
    expect(services.navigation.route()).toEqual({ screen: "films", genre: 18 });
  });

  it("shows a series' genres in the server's order, and one opens Series filtered by it", async () => {
    const services = testServices(undefined, "/series/20");
    mounted = await mount(withServices(<SeriesView series={glassTide} />, services));
    expect(genresIn(mounted.container)).toEqual([
      `Crime|${routeHref({ screen: "series", genre: 80 })}`,
      `Drama|${routeHref({ screen: "series", genre: 18 })}`,
    ]);
    await click(mounted.container.querySelector(".title-head__genres a"));
    expect(services.navigation.route()).toEqual({ screen: "series", genre: 80 });
  });

  it("shows no genre line, nor a separator, for a film or a series with none", async () => {
    const films = [
      ungenredFilm,
      { ...ungenredFilm, genres: null },
      { ...ungenredFilm, genres: [] },
    ];
    const series = [
      ungenredSeries,
      { ...ungenredSeries, genres: null },
      { ...ungenredSeries, genres: [] },
    ];
    for (const [index, film] of films.entries()) {
      mounted = await mount(withServices(<TitleView title={film} />));
      expect(mounted.container.querySelector(".title-head__genres")).toBeNull();
      expect(mounted.container.querySelector(".title-head__facts")?.textContent).toBe(
        "1927170 min",
      );
      mounted.unmount();
      mounted = await mount(withServices(<SeriesView series={series[index] ?? ungenredSeries} />));
      expect(mounted.container.querySelector(".title-head__genres")).toBeNull();
      expect(mounted.container.querySelector(".title-head__facts")?.textContent).toBe("2019");
      mounted.unmount();
      mounted = undefined;
    }
  });

  it("shows none on an episode's page: its genres are its series', shown there", async () => {
    const episode: TitleDetail = {
      ...metropolis,
      kind: "episode",
      name: "Pilot",
      episode: { seriesId: 20, seriesName: "Glass Tide", seasonNumber: 1, episodeNumber: 1 },
    };
    mounted = await mount(withServices(<TitleView title={episode} />));
    expect(mounted.container.querySelector("h1")?.textContent).toBe("Pilot");
    expect(mounted.container.querySelector(".title-head__genres")).toBeNull();
  });
});
