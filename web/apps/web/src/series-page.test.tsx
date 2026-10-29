// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

/**
 * A series is one page with a season picker (D367 §9): choosing a season replaces the history's
 * entry, and returning to the series shows the season of the episode one comes back from, that
 * episode focused.
 */

import "./i18n.ts";

import { act, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import {
  episodeReachedOf,
  noteEpisodeReached,
  useEpisodesReachedLetGoOnGoing,
} from "./episode-reached.ts";
import { createNavigation } from "./navigation.ts";
import { PlayTrail } from "./play-up.tsx";
import { Series, type SeriesDetail } from "./series.tsx";
import { useRoute } from "./services.tsx";
import { click, mount, servedBy, settleUntil, type Answers, type Mounted } from "./test-dom.tsx";
import { fakePlace, withServices } from "./test-services.tsx";

const series: SeriesDetail = {
  id: 6,
  kind: "series",
  name: "Long Run",
  art: {},
  seasons: [1, 2].map((seasonNumber) => ({ seasonNumber, episodeCount: 1 })),
};

/** An episode of season, numbered number, with id. */
function episode(id: number, season: number) {
  return {
    id,
    seasonNumber: season,
    episodeNumber: 1,
    name: `Episode ${String(id)}`,
    identificationState: "identified",
    hasLiveFile: true,
  };
}

const answers: Answers = {
  "GET /collections/6": [200, series],
  "GET /collections/6/seasons/1": [200, { episodes: [episode(41, 1)] }],
  "GET /collections/6/seasons/2": [200, { episodes: [episode(42, 2)] }],
  "PUT /interest/6": [204, null],
  "DELETE /interest/6": [204, null],
};

let shown: Mounted | undefined;

/** The Series list, standing in, and a series' page, letting episodes reached go as the app does. */
function ListAndSeries(): JSX.Element {
  useEpisodesReachedLetGoOnGoing();
  const route = useRoute();
  return route.screen === "series-page" ? <Series id={route.collectionId} /> : <p>{"list"}</p>;
}

afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

describe("a series' page, one page with a season picker (D367 §9)", () => {
  it("replaces its entry with the season chosen, adding no step to the history", async () => {
    const place = fakePlace("/series");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "series-page", collectionId: 6 });
    shown = await mount(withServices(<Series id={6} />, { ...servedBy(answers), navigation }));
    const { container } = shown;
    await settleUntil(() => container.querySelector(".route-chooser__button") !== null);
    await click(container.querySelector(".route-chooser__button"));
    const seasonTwo = [...container.querySelectorAll<HTMLAnchorElement>("dialog a")].find((link) =>
      link.textContent.startsWith("Season 2"),
    );
    await click(seasonTwo);
    // Ahead of it, as a browser keeps an entry Back left, the entry the picker's list held while it
    // was open, so the browser's own Back closed it (Silk's, the owner, 2026-10-28); Forward onto it
    // steps straight back off it, the season chosen staying.
    expect(place.addresses).toEqual(["/series", "/series/6?season=2", "/series/6"]);
    expect(navigation.behind()).toEqual([{ screen: "series" }]);
    act(() => {
      place.history.go(1);
    });
    expect(navigation.route()).toEqual({ screen: "series-page", collectionId: 6, season: 2 });
    expect(navigation.behind()).toEqual([{ screen: "series" }]);
  });

  it("returned to from an episode, shows that episode's season, the episode focused", async () => {
    const place = fakePlace("/series/6?season=1");
    const navigation = createNavigation(place, false);
    noteEpisodeReached(6, { season: 2, episodeId: 42 });
    shown = await mount(
      withServices(<Series id={6} season={1} />, { ...servedBy(answers), navigation }),
    );
    await settleUntil(() => document.activeElement?.getAttribute("href") === "/titles/42");
    expect(place.addresses).toEqual(["/series/6?season=2"]);
    expect(shown.container.querySelector("h2")?.textContent).toBe("Season 2");
  });

  // The owner's Fire TV, 2026-10-29: a series opened from the Series list showed an episode played
  // earlier, scrolled down to it, not the page's top.
  it("opened afresh by a step forward shows its own season, though one of its episodes was reached before", async () => {
    const place = fakePlace("/series");
    const navigation = createNavigation(place, false);
    shown = await mount(withServices(<ListAndSeries />, { ...servedBy(answers), navigation }));
    noteEpisodeReached(6, { season: 2, episodeId: 42 });
    act(() => {
      navigation.go({ screen: "series-page", collectionId: 6 });
    });
    await settleUntil(() => shown?.container.querySelector('a[href^="/titles/"]') !== null);
    expect(shown.container.querySelector("h2")?.textContent).toBe("Season 1");
    expect(document.activeElement?.getAttribute("href")).not.toBe("/titles/42");
    expect(episodeReachedOf(6)).toBeUndefined();
    expect(place.addresses).toEqual(["/series", "/series/6"]);
  });

  it("is told of an episode played, as of one whose page was shown, to show its season", async () => {
    shown = await mount(
      withServices(
        <PlayTrail
          upward={{ crumbs: [], here: "Episode 42" }}
          plays={{
            titleId: 42,
            kind: "episode",
            episode: { seriesId: 7, seriesName: "Other Run", seasonNumber: 3, episodeNumber: 2 },
          }}
        />,
      ),
    );
    expect(episodeReachedOf(7)).toEqual({ season: 3, episodeId: 42 });
  });
});
