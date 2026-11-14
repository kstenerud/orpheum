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
  mount,
  servedBy,
  settleUntil,
  type Answer,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import {
  chooseInCriteria,
  closeCriteria,
  criteriaPopUp,
  pressUnwatched,
} from "./test-list-criteria.tsx";

/** Films as app.tsx shows it: its sort, search and chips read from the address it replaces. */
function RoutedFilms(): JSX.Element {
  const route = useRoute();
  return <Browse kind="film" state={route.screen === "films" ? route : {}} />;
}

/** A poller that never asks: the list is read only as the test's steps ask. */
const neverAsks: Poller = { start: () => () => undefined };

const metropolis: Entry = {
  id: 4,
  name: "Metropolis",
  kind: "film",
  identificationState: "identified",
  addedAt: 1,
};
const titles: Answer = [200, { items: [metropolis] }];

/** The genres a films list's first page names, with genres asked (D480 §1). */
const twoGenres = [
  { id: 18, name: "Drama", titleCount: 3 },
  { id: 80, name: "Crime", titleCount: 2 },
];

/** The query of each GET /items heard, in order. */
function titlesAsked(heard: readonly Heard[]): URLSearchParams[] {
  return heard
    .filter((each) => each.methodAndPath === "GET /items")
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
        "GET /items": [[200, { items: [metropolis], genres: twoGenres }], titles, titles, titles],
      },
      heard,
    );
    services.navigation.replace({ screen: "films", query: "metro" });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedFilms />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => titlesAsked(heard).length >= 1);
    // The list's first page names the genres, searched or not (D480 §1).
    expect(titlesAsked(heard)[0]?.get("genres")).toBe("true");

    // In the list's Sort and filter (D417), the genres its first page named.
    await chooseInCriteria(mounted.container, "Genre", "Crime");
    // The choice closes the Genre's list alone, the Sort and filter left open for the next.
    expect(criteriaPopUp(mounted.container)?.querySelectorAll("dialog")).toHaveLength(0);
    expect(criteriaPopUp(mounted.container)).not.toBeNull();
    await chooseInCriteria(mounted.container, "Decade", "1990s");
    await pressUnwatched(mounted.container);
    // The list changes under the pop-up as each is chosen.
    await settleUntil(() => titlesAsked(heard).length >= 4);
    const popUp = criteriaPopUp(mounted.container);
    expect(
      popUp === null ? undefined : buttonWithText(popUp, "Unwatched")?.getAttribute("aria-pressed"),
    ).toBe("true");
    // The address takes them as the pop-up closes.
    await closeCriteria(mounted.container);

    expect(services.navigation.route()).toEqual({
      screen: "films",
      query: "metro",
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
    ]).toEqual(["metro", "80", "1990", "true"]);
    // Each new list's first page names them again, a later one's not asked to.
    expect(titlesAsked(heard).map((each) => each.get("genres"))).toEqual(
      titlesAsked(heard).map(() => "true"),
    );
  });

  it("offer the genres the list's first page names, reading none of their own (D480 §1)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items": [
          [200, { items: [metropolis], genres: twoGenres }],
          // A list a chip narrows names the same: the list's before its search and filters.
          [200, { items: [metropolis], genres: twoGenres }],
        ],
      },
      heard,
    );
    services.navigation.replace({ screen: "films" });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedFilms />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => titlesAsked(heard).length >= 1);
    await chooseInCriteria(mounted.container, "Genre", "Crime");
    await settleUntil(() => titlesAsked(heard).length >= 2);
    // Narrowed to Crime, the chip still offers every genre, and names the one chosen.
    await chooseInCriteria(mounted.container, "Genre", "Drama");
    await closeCriteria(mounted.container);

    expect(services.navigation.route()).toEqual({ screen: "films", genre: 18 });
    expect(heard.filter((each) => each.methodAndPath === "GET /genres")).toHaveLength(0);
  });

  it("show the chips the address names, the genre by its name, as Back returns to them", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items": [
          [200, { items: [metropolis], genres: [{ id: 18, name: "Drama", titleCount: 3 }] }],
          titles,
        ],
      },
      heard,
    );
    services.navigation.replace({ screen: "films", genre: 18, decade: 1980 });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedFilms />, services)}
      </BackgroundChangesContext>,
    );
    // Shown in the Sort and filter's summary, on the bar (D417).
    await settleUntil(
      () =>
        mounted?.container
          .querySelector(".list-criteria__summary")
          ?.textContent.includes("Drama") === true,
    );
    expect(mounted.container.querySelector(".list-criteria__summary")?.textContent).toContain(
      "1980s",
    );
    const asked = titlesAsked(heard)[0];
    expect([asked?.get("genre"), asked?.get("decade"), asked?.get("unwatched")]).toEqual([
      "18",
      "1980",
      null,
    ]);
    // The rail's letters are the held entries' (D475), so every read of the
    // list carries the chips the address names.
    expect(titlesAsked(heard).map((each) => [each.get("genre"), each.get("decade")])).toEqual(
      titlesAsked(heard).map(() => ["18", "1980"]),
    );
  });
});
