// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { Series, type SeriesDetail } from "./series.tsx";
import { buttonWithText, click, mount, servedBy, type Answers, type Mounted } from "./test-dom.tsx";
import { fakePlace, withServices, type FakePlace } from "./test-services.tsx";
import { Title, type TitleDetail } from "./title.tsx";

const film: TitleDetail = {
  id: 10,
  kind: "movie",
  name: "Metropolis",
  identificationState: "identified",
  art: {},
  versions: [],
  extras: [],
};

const episode: TitleDetail = {
  ...film,
  id: 12,
  kind: "episode",
  name: "Pilot",
  episode: { seriesId: 5, seriesName: "Long Run", seasonNumber: 2, episodeNumber: 1 },
};

const seasons = (count: number): SeriesDetail => ({
  id: 5,
  kind: "series",
  name: "Long Run",
  art: {},
  seasons: Array.from({ length: count }, (_, index) => ({
    seasonNumber: index + 1,
    episodeCount: 1,
  })),
});

/** The stand-in server's answers for a title's or a series' page, shown in turn. */
function pageAnswers(id: number, detail: TitleDetail | SeriesDetail): Answers {
  const path = detail.kind === "series" ? `/collections/${String(id)}` : `/items/${String(id)}`;
  return {
    [`GET ${path}`]: [200, detail],
    "GET /collections/5/seasons/1": [200, { episodes: [] }],
    "GET /collections/5/seasons/2": [200, { episodes: [] }],
    [`PUT /interest/${String(id)}`]: [204, null],
    [`DELETE /interest/${String(id)}`]: [204, null],
  };
}

let shown: Mounted | undefined;

afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

/** Shows page at place's address, with answers, and presses its Back. */
async function pressBack(place: FakePlace, answers: Answers, page: JSX.Element): Promise<void> {
  const services = { ...servedBy(answers), navigation: createNavigation(place) };
  shown = await mount(withServices(page, services));
  const back = buttonWithText(shown.container, "Back");
  expect(back?.type).toBe("button");
  await click(back);
}

describe("a title's Back", () => {
  it("goes back to the page of this app's before it", async () => {
    const place = fakePlace("/series/5?season=2");
    createNavigation(place).go({ screen: "title", itemId: 12 });
    await pressBack(place, pageAnswers(12, episode), <Title id={12} />);
    expect(place.location.pathname + place.location.search).toBe("/series/5?season=2");
  });

  it("goes up from an episode opened from outside to its season, in its place", async () => {
    const place = fakePlace("/titles/12");
    await pressBack(place, pageAnswers(12, episode), <Title id={12} />);
    expect(place.addresses).toEqual(["/series/5?season=2"]);
  });

  it("goes up from a film opened from outside to the films", async () => {
    const place = fakePlace("/titles/10");
    await pressBack(place, pageAnswers(10, film), <Title id={10} />);
    expect(place.addresses).toEqual(["/films"]);
  });

  it("is there, and goes up, while its title cannot be read", async () => {
    const place = fakePlace("/titles/10");
    await pressBack(
      place,
      { ...pageAnswers(10, film), "GET /items/10": [404, { error: { code: "not_found" } }] },
      <Title id={10} />,
    );
    expect(place.addresses).toEqual(["/films"]);
  });
});

describe("a series' Back", () => {
  it("goes up from a season opened from outside to the series' seasons", async () => {
    const place = fakePlace("/series/5?season=2");
    await pressBack(place, pageAnswers(5, seasons(2)), <Series id={5} season={2} />);
    expect(place.addresses).toEqual(["/series/5"]);
  });

  it("goes up from a series opened from outside to the series", async () => {
    const place = fakePlace("/series/5");
    await pressBack(place, pageAnswers(5, seasons(2)), <Series id={5} />);
    expect(place.addresses).toEqual(["/series"]);
  });

  it("goes up from the season of a series of one season to the series, its page being the same", async () => {
    const place = fakePlace("/series/5?season=1");
    await pressBack(place, pageAnswers(5, seasons(1)), <Series id={5} season={1} />);
    expect(place.addresses).toEqual(["/series"]);
  });

  it("goes back to the page of this app's before it", async () => {
    const place = fakePlace("/series");
    createNavigation(place).go({ screen: "series-page", collectionId: 5 });
    await pressBack(place, pageAnswers(5, seasons(2)), <Series id={5} />);
    expect(place.location.pathname).toBe("/series");
  });
});
