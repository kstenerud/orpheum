// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { Browse, type Entry } from "./browse.tsx";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import { useRoute } from "./services.tsx";
import {
  buttonWithText,
  chooseIn,
  click,
  mount,
  servedBy,
  settleUntil,
  type Answer,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** Films as app.tsx shows it: its sort, search and chips read from the address it replaces. */
function RoutedFilms(): JSX.Element {
  const route = useRoute();
  return <Browse kind="movie" state={route.screen === "films" ? route : {}} />;
}

/** A poller that never asks: the list is read only as the test's steps ask. */
const neverAsks: Poller = { start: () => () => undefined };

const heat: Entry = {
  id: 4,
  name: "Metropolis",
  kind: "movie",
  identificationState: "identified",
  addedAt: 1,
};
const titles: Answer = [200, { titles: [heat], nextCursor: "" }];

/** The query of each GET /titles heard, in order. */
function titlesAsked(heard: readonly Heard[]): URLSearchParams[] {
  return heard
    .filter((each) => each.methodAndPath === "GET /titles")
    .map((each) => new URLSearchParams(each.query));
}

describe("Films' chips (search-by-more §4.3)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("narrow the list by a genre, a decade and unwatched together, kept in the address", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /titles": [titles, titles, titles, titles],
        "GET /titles/letters": [200, { letters: [] }],
        "GET /genres": [
          200,
          {
            genres: [
              { id: 18, name: "Drama", titleCount: 3 },
              { id: 80, name: "Crime", titleCount: 2 },
            ],
          },
        ],
      },
      heard,
    );
    services.navigation.replace({ screen: "films", query: "heat" });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedFilms />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => titlesAsked(heard).length >= 1);
    expect(heard.some((each) => each.methodAndPath === "GET /genres")).toBe(false);

    await chooseIn(mounted.container, "Genre", "Crime");
    expect(
      new URLSearchParams(heard.find((each) => each.methodAndPath === "GET /genres")?.query).get(
        "kind",
      ),
    ).toBe("movie");
    await chooseIn(mounted.container, "Decade", "1990s");
    await click(buttonWithText(mounted.container, "Unwatched"));
    await settleUntil(() => titlesAsked(heard).length >= 4);

    expect(services.navigation.route()).toEqual({
      screen: "films",
      query: "heat",
      genre: 80,
      decade: 1990,
      isUnwatchedOnly: true,
    });
    const last = titlesAsked(heard).at(-1);
    expect([
      last?.get("query"),
      last?.get("genre"),
      last?.get("decade"),
      last?.get("unwatched"),
    ]).toEqual(["heat", "80", "1990", "true"]);
    expect(buttonWithText(mounted.container, "Unwatched")?.getAttribute("aria-pressed")).toBe(
      "true",
    );
  });

  it("show the chips the address names, the genre by its name, as Back returns to them", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /titles": [titles],
        "GET /titles/letters": [200, { letters: [] }],
        "GET /genres": [200, { genres: [{ id: 18, name: "Drama", titleCount: 3 }] }],
      },
      heard,
    );
    services.navigation.replace({ screen: "films", genre: 18, decade: 1980 });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedFilms />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(
      () => mounted?.container.querySelector(".list-chips")?.textContent.includes("Drama") === true,
    );
    expect(mounted.container.querySelector(".list-chips")?.textContent).toContain("1980s");
    const asked = titlesAsked(heard)[0];
    expect([asked?.get("genre"), asked?.get("decade"), asked?.get("unwatched")]).toEqual([
      "18",
      "1980",
      null,
    ]);
    const letters = new URLSearchParams(
      heard.find((each) => each.methodAndPath === "GET /titles/letters")?.query,
    );
    expect([letters.get("genre"), letters.get("decade")]).toEqual(["18", "1980"]);
  });
});
