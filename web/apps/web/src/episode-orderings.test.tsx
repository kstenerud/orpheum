// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { rowName, type EpisodeNaming } from "./continue-watching.tsx";
import { SeriesView, type SeriesDetail } from "./series.tsx";
import { buttonNamed, click, mount, servedBy, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { TitleView, titleUp, type TitleDetail } from "./title.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

const aired = { name: "aired", kind: "aired", episodeCount: 4 };
const dvd = {
  name: "tmdb:dvd",
  kind: "dvd",
  label: "DVD Order",
  listedCount: 5,
  episodeCount: 3,
};

const nextUp = {
  itemId: 21,
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

/** The "…"'s entries of a series' page, its orderings' lists among them: "Order: Aired ›". */
async function entriesOf(series: SeriesDetail, isAdmin = false): Promise<string[]> {
  const page = await mount(
    withServices(
      <IsAdminContext value={isAdmin}>
        <SeriesView series={series} />
      </IsAdminContext>,
    ),
  );
  await click(buttonNamed(page.container, "More actions"));
  const entries = [...page.container.querySelectorAll(".list-box__submenu-entry")].map(
    (each) => each.textContent,
  );
  page.unmount();
  return entries;
}

/** Opens the "…"'s list named name, in the "…"'s place (D538 §3), and gives its choices. */
async function openList(page: HTMLElement, name: string): Promise<Element[]> {
  await click(buttonNamed(page, "More actions"));
  await click(
    [...page.querySelectorAll(".list-box__submenu-entry")].find((each) =>
      each.textContent.startsWith(`${name}: `),
    ),
  );
  return [
    ...page.querySelectorAll(
      `.list-box__submenu[aria-label="${name}"] .list-box__option:not(.list-box__submenu-back)`,
    ),
  ];
}

describe("a series in the ordering its person chose (episode-orderings.md §4)", () => {
  it("heads its seasons with the ordering's groups, labels Next Up there, and lists what it leaves out", () => {
    const markup = renderToStaticMarkup(withServices(<SeriesView series={inDVDOrder} />));
    // One page whose season is picked (D367 §9): the season of Next Up first, by its group's name.
    expect(markup).toContain(">Disc 1</h2>");
    expect(
      renderToStaticMarkup(withServices(<SeriesView series={inDVDOrder} season={2} />)),
    ).toContain(">Part 2</h2>");
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

  it("offers its orderings in a list in its …'s place, aired first, each with its kind and how many it holds; none when it has one", async () => {
    // The "…"'s list, not a native select, which a television's browser cannot work (the 2026-10-27
    // widget audit; plans/toolkit.md step 6), nor a dialog of its own (D538 §3).
    const offered = await mount(withServices(<SeriesView series={inDVDOrder} season={1} />));
    expect(offered.container.querySelector("select")).toBeNull();
    const choices = await openList(offered.container, "Order");
    expect(choices.map((choice) => choice.textContent)).toEqual([
      "Aired",
      "DVD Order (DVD) · episodes here: 3",
    ]);
    expect(choices.map((choice) => choice.getAttribute("aria-current"))).toEqual([null, "true"]);
    expect(offered.container.querySelectorAll("dialog")).toHaveLength(1);
    expect(offered.container.querySelector(".modal-dialog")).toBeNull();
    offered.unmount();
    expect(await entriesOf(inDVDOrder)).toEqual(["Order: DVD Order (DVD) · episodes here: 3›"]);
    const listedOnly = { ...inDVDOrder, orderings: [aired, { ...dvd, episodeCount: 0 }] };
    expect(await entriesOf(listedOnly)).toEqual(["Order: DVD Order (DVD) · episodes listed: 5›"]);
    expect(renderToStaticMarkup(withServices(<SeriesView series={listedOnly} />))).not.toContain(
      "Fetching this order",
    );
    const awaiting = renderToStaticMarkup(
      withServices(
        <SeriesView
          series={{
            ...inDVDOrder,
            orderings: [aired, { ...dvd, episodeCount: 0, isAwaitingFetch: true }],
          }}
        />,
      ),
    );
    expect(awaiting).toContain("Fetching this order; its episodes show here once it is fetched.");
    // The choice is under the page's "…", rarely used (the owner, 2026-10-29), which a series of
    // one ordering does not show to someone who is not an admin.
    const onlyAired = renderToStaticMarkup(
      withServices(<SeriesView series={{ ...inAired, orderings: [aired] }} />),
    );
    expect(onlyAired).not.toContain("<select");
    expect(onlyAired).not.toContain('aria-label="More actions"');
  });

  it("says on the page why a choice was not kept, the … closed", async () => {
    const services = servedBy(
      {
        "PUT /items/20/preferences": [
          500,
          { error: { code: "internal", message: "a note for developers" } },
        ],
        "GET /items/20/not-in-order": [200, { episodes: [] }],
        "GET /items/20/groups/1": [200, { episodes: [] }],
      },
      [],
      "token",
    );
    const page = await mount(withServices(<SeriesView series={inDVDOrder} />, services));
    await click((await openList(page.container, "Order"))[0]);
    expect(page.container.querySelector("dialog")).toBeNull();
    expect(page.container.querySelector(".title-head__notes")?.textContent).toContain(
      "The order could not be changed: ",
    );
    page.unmount();
  });

  it("keeps a choice, goes to the series' top page and reads it again (§7 Q10)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "PUT /items/20/preferences": [204, null],
        "GET /items/20/not-in-order": [200, { episodes: [] }],
        "GET /items/20/groups/1": [200, { episodes: [] }],
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
    const choices = await openList(page.container, "Order");
    await click(choices.find((choice) => choice.textContent === "Aired"));
    // Chosen, the "…" closes.
    expect(page.container.querySelector("dialog")).toBeNull();
    page.unmount();
    const puts = heard.filter((each) => each.methodAndPath.startsWith("PUT "));
    expect(puts.map((each) => [each.methodAndPath, each.body])).toEqual([
      ["PUT /items/20/preferences", { episodeOrdering: "aired" }],
    ]);
    expect(services.navigation.route()).toEqual({ screen: "series-page", collectionId: 20 });
    // In place of the season it was at, a view of the same page, not a second copy of it behind
    // (D367 §9; navigation-back-up-2026-10-27.md §3.23).
    expect(services.navigation.behind()).toHaveLength(1);
    expect(reads).toBe(1);
  });
});

describe("the order a series' files are numbered in (D210 §1, b2.md Phase B)", () => {
  it('is offered to an admin alone, under the "…" beside the viewing order', async () => {
    expect(await entriesOf(inAired, false)).toEqual(["Order: Aired›"]);
    expect(await entriesOf(inAired, true)).toEqual([
      "Order: Aired›",
      "Files are numbered in: Aired›",
    ]);
  });

  it("shows an admin the current choice, and keeps the one chosen, by its id, and reads the page again", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "PUT /items/20/files-ordering": [204, null],
        "GET /items/20/not-in-order": [200, { episodes: [] }],
        "GET /items/20/groups/1": [200, { episodes: [] }],
      },
      heard,
      "token",
    );
    let reads = 0;
    const page = await mount(
      withServices(
        <IsAdminContext value={true}>
          <SeriesView
            series={inAired}
            onOrderingChosen={() => {
              reads++;
            }}
          />
        </IsAdminContext>,
        services,
      ),
    );
    // Aired, the default, while the series says no other.
    const choices = await openList(page.container, "Files are numbered in");
    expect(choices.map((choice) => choice.getAttribute("aria-current"))).toEqual(["true", null]);
    await click(
      choices.find((choice) => choice.textContent === "DVD Order (DVD) · episodes here: 3"),
    );
    expect(page.container.querySelector("dialog")).toBeNull();
    page.unmount();
    const puts = heard.filter((each) => each.methodAndPath.startsWith("PUT "));
    expect(puts.map((each) => [each.methodAndPath, each.body])).toEqual([
      ["PUT /items/20/files-ordering", { filesOrdering: "tmdb:dvd" }],
    ]);
    expect(reads).toBe(1);
    // The series' own files' order is the one shown as chosen.
    expect(await entriesOf({ ...inAired, filesOrdering: "tmdb:dvd" }, true)).toContain(
      "Files are numbered in: DVD Order (DVD) · episodes here: 3›",
    );
  });

  it("names an ordering its provider no longer lists so, in both choices", async () => {
    const series: SeriesDetail = { ...inAired, orderings: [aired, { ...dvd, isUnlisted: true }] };
    const unlisted = "DVD Order (DVD) · episodes here: 3 (no longer listed)";
    const watched = await entriesOf({ ...series, ordering: "tmdb:dvd" });
    expect(watched[0]).toContain(unlisted);
    const filed = await entriesOf({ ...series, filesOrdering: "tmdb:dvd" }, true);
    expect(filed[1]).toContain(unlisted);
    expect((await entriesOf({ ...inAired, ordering: "tmdb:dvd" })).join()).not.toContain(
      "no longer listed",
    );
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
