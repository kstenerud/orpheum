// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";

import { SeasonView, Series, type SeriesDetail } from "./series.tsx";
import {
  bodyField,
  buttonNamed,
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { Title, TitleView, type TitleDetail } from "./title.tsx";

const film: TitleDetail = {
  id: 10,
  kind: "movie",
  name: "Metropolis",
  identificationState: "identified",
  art: {},
  versions: [],
  extras: [],
};

const series: SeriesDetail = {
  id: 5,
  kind: "series",
  name: "Long Run",
  art: {},
  seasons: [
    { seasonNumber: 1, episodeCount: 2, isWatched: true },
    { seasonNumber: 2, episodeCount: 1 },
  ],
};

const episodes = {
  episodes: [
    {
      id: 21,
      seasonNumber: 2,
      episodeNumber: 1,
      name: "Low Water",
      identificationState: "identified",
      hasLiveFile: true,
      isWatched: true,
    },
    {
      id: 22,
      seasonNumber: 2,
      episodeNumber: 2,
      name: "High Water",
      identificationState: "identified",
      hasLiveFile: true,
    },
  ],
};

const nothing = (): void => undefined;

let shown: Mounted | undefined;

afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

describe("the watched check on rows (D370 §2, §3)", () => {
  it("is on each episode its person watched, and on each season they watched whole", () => {
    const season = renderToStaticMarkup(
      withServices(
        <SeasonView
          loadedEpisodes={{ state: "loaded", value: { items: episodes.episodes, nextCursor: "" } }}
          isLoadingMore={false}
          onLoadMore={nothing}
        />,
      ),
    );
    const watchedRow = season.slice(0, season.indexOf("High Water"));
    expect(watchedRow).toContain("watched-icon");
    expect(watchedRow).toContain('<span class="poster-cue__name">Watched</span>');
    expect(season.slice(season.indexOf("High Water"))).not.toContain("watched-icon");
  });

  it("is on a season watched whole, and nothing else marks its row: no fading", async () => {
    shown = await mount(
      withServices(
        <Series id={5} />,
        servedBy({
          "GET /collections/5": [200, series],
          "GET /collections/5/seasons/1": [200, { episodes: [] }],
          "PUT /interest/page": [204, null],
          "DELETE /interest/page": [204, null],
        }),
      ),
    );
    // A season's row is its poster in the series' row of seasons (D379 §1).
    const { container } = shown;
    await settleUntil(() => container.querySelector(".series__seasons") !== null);
    const rows = [...container.querySelectorAll(".series__seasons > li")];
    expect(rows).toHaveLength(2);
    expect(rows[0]?.querySelector(".watched-icon")).not.toBeNull();
    expect(rows[1]?.querySelector(".watched-icon")).toBeNull();
    expect(rows[0]?.className).toBe(rows[1]?.className);
    const links = rows.map((row) => row.querySelector("a")?.className);
    expect(links[0]).toBe(links[1]);
  });
});

describe("marking by hand (D370 §4)", () => {
  it("is not offered on an extra's page", () => {
    const extra = renderToStaticMarkup(
      withServices(<TitleView title={{ ...film, kind: "extra" }} onWatchedChanged={nothing} />),
    );
    expect(extra).not.toContain("Mark as watched");
    expect(
      renderToStaticMarkup(withServices(<TitleView title={film} onWatchedChanged={nothing} />)),
    ).toContain("Mark as watched");
  });

  it("marks a film watched at once, says so, and its Undo puts the marks back as they were", async () => {
    const heard: Heard[] = [];
    const previous = [{ titleId: 10, markedAt: 500 }];
    const answers: Answers = {
      "GET /items/10": [
        [200, film],
        [200, { ...film, isWatched: true }],
        [200, film],
      ],
      "PUT /items/10/watched": [200, { previous }],
      "PUT /watched": [204, null],
      "PUT /interest/page": [204, null],
      "DELETE /interest/page": [204, null],
    };
    shown = await mount(withServices(<Title id={10} />, servedBy(answers, heard)));
    await click(buttonWithText(shown.container, "Mark as watched"));
    expect(heard.map((each) => each.methodAndPath)).toContain("PUT /items/10/watched");
    expect(shown.container.textContent).toContain("Marked as watched.");
    // The page read again: it is watched, says so beside the check, and offers the way back.
    expect(buttonWithText(shown.container, "Mark as unwatched")).toBeDefined();
    expect(shown.container.querySelector(".status-note .watched-icon")).not.toBeNull();
    await click(buttonWithText(shown.container, "Undo"));
    const undo = heard.find((each) => each.methodAndPath === "PUT /watched");
    expect(undo === undefined ? undefined : bodyField(undo, "states")).toEqual(previous);
    expect(shown.container.textContent).not.toContain("Marked as watched.");
    expect(buttonWithText(shown.container, "Mark as watched")).toBeDefined();
  });

  it("marks a season, named for a screen reader, and reads its episodes again", async () => {
    const heard: Heard[] = [];
    const answers: Answers = {
      "GET /collections/5": [200, series],
      "GET /collections/5/seasons/2": [200, episodes],
      "PUT /collections/5/seasons/2/watched": [200, { previous: [] }],
      "PUT /interest/page": [204, null],
      "DELETE /interest/page": [204, null],
    };
    shown = await mount(withServices(<Series id={5} season={2} />, servedBy(answers, heard)));
    const reads = (): number =>
      heard.filter((each) => each.methodAndPath === "GET /collections/5/seasons/2").length;
    const before = reads();
    await click(buttonNamed(shown.container, "Mark Season 2 as watched"));
    expect(heard.map((each) => each.methodAndPath)).toContain(
      "PUT /collections/5/seasons/2/watched",
    );
    expect(reads()).toBeGreaterThan(before);
  });

  it("marks a whole series unwatched from its page once it is watched", async () => {
    const heard: Heard[] = [];
    const answers: Answers = {
      "GET /collections/5": [200, { ...series, isWatched: true }],
      "GET /collections/5/seasons/1": [200, { episodes: [] }],
      "DELETE /collections/5/watched": [200, { previous: [{ titleId: 21, watchedAt: 3 }] }],
      "PUT /interest/page": [204, null],
      "DELETE /interest/page": [204, null],
    };
    shown = await mount(withServices(<Series id={5} />, servedBy(answers, heard)));
    await click(buttonNamed(shown.container, "Mark all of Long Run as unwatched"));
    expect(heard.map((each) => each.methodAndPath)).toContain("DELETE /collections/5/watched");
    expect(shown.container.textContent).toContain("Marked as unwatched.");
  });
});
