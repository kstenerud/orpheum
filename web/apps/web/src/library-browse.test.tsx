// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";

import { LibraryBrowse } from "./library-browse.tsx";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import { SearchResults } from "./search.tsx";
import {
  buttonWithText,
  click,
  enter,
  mount,
  servedBy,
  settleUntil,
  submit,
  type Answer,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A poller that never asks: a list is read only as the test's steps ask. */
const neverAsks: Poller = { start: () => () => undefined };

/** A library, as the server sends it. */
function library(
  id: number,
  name: string,
  kind: "movies" | "shows" | "other" = "movies",
): Record<string, unknown> {
  return {
    id,
    name,
    kind,
    metadataSource: kind === "other" ? "nfo" : "tmdb",
    changeDetection: "watch",
    isWritable: true,
    createdAt: 0,
  };
}

/** One film in a titles page, so .poster-grid draws: an empty page shows browse.empty instead. */
const titlesPage = (): Answer => [
  200,
  {
    titles: [
      {
        id: 1,
        name: "A film",
        kind: "movie",
        identificationState: "identified",
        libraryId: 1,
        addedAt: 1,
      },
    ],
    nextCursor: "",
  },
];
const lettersPage = (): Answer => [200, { letters: [] }];

/** Mounts the Libraries page with its state given by the address (app.tsx's own props). */
async function mountBrowse(
  state: Partial<{
    library: number | "all" | undefined;
    view: "folders" | undefined;
    folder: string | undefined;
    sort: "name" | "added" | "recorded" | "file_name" | "released" | undefined;
    order: "ascending" | "descending" | undefined;
    query: string;
  }>,
  answers: Record<string, Answer | readonly Answer[]>,
  heard: Heard[] = [],
): Promise<Mounted> {
  const services = servedBy(answers, heard);
  return await mount(
    <BackgroundChangesContext value={neverAsks}>
      {withServices(
        <LibraryBrowse
          library={state.library}
          view={state.view}
          folder={state.folder}
          sort={state.sort}
          order={state.order}
          query={state.query ?? ""}
        />,
        services,
      )}
    </BackgroundChangesContext>,
  );
}

describe("the Libraries page (D349)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  // Another library is the banner's Libraries menu's to choose (the owner, 2026-10-28; app-shell.tsx):
  // the page names the one it shows and keeps its view and sort choosers.
  it("has no library chooser of its own, its heading the library shown, or All libraries", async () => {
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [200, { libraries: [library(2, "Zoo"), library(1, "Alpha")] }],
        "GET /titles": titlesPage(),
        "GET /titles/letters": lettersPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const bar = mounted.container.querySelector(".browse__bar");
    expect(bar?.querySelector("h1")?.textContent).toBe("Alpha");
    expect(buttonWithText(mounted.container, "Alpha")).toBeUndefined();
    expect(bar?.querySelector('[aria-label^="Library:"]')).toBeNull();
    expect(bar?.querySelector('a[href*="library="]')).toBeNull();
    // Its sort chooser stays.
    expect(bar?.querySelectorAll(".route-chooser__button")).toHaveLength(1);
    mounted.unmount();
    mounted = await mountBrowse(
      { library: "all" },
      {
        "GET /libraries": [200, { libraries: [library(2, "Zoo"), library(1, "Alpha")] }],
        "GET /titles": titlesPage(),
        "GET /titles/letters": lettersPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    expect(mounted.container.querySelector(".browse__bar h1")?.textContent).toBe("All libraries");
  });

  it("names neither the library chooser nor the search on the page, only to a screen reader (the owner, 2026-10-27)", async () => {
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [200, { libraries: [library(1, "Alpha")] }],
        "GET /titles": titlesPage(),
        "GET /titles/letters": lettersPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const bar = mounted.container.querySelector(".browse__bar");
    const fieldNames = [...(bar?.querySelectorAll(".route-chooser__field") ?? [])].map(
      (field) => field.textContent,
    );
    expect(fieldNames).not.toContain("Library");
    expect(bar?.textContent).not.toContain("Search the library chosen, or every one");
    expect(bar?.querySelector("input[type=search]")?.getAttribute("aria-label")).toBe(
      "Search the library chosen, or every one",
    );
  });

  it("picks the first library by name when none is chosen in the address (D349 §9.1 point 1)", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [library(2, "Zoo"), library(1, "Alpha")] }],
      "GET /titles": titlesPage(),
      "GET /titles/letters": lettersPage(),
    });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(
          <LibraryBrowse
            library={undefined}
            view={undefined}
            folder={undefined}
            sort={undefined}
            query=""
          />,
          services,
        )}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => {
      const route = services.navigation.route();
      return route.screen === "browse" && route.library !== undefined;
    });
    expect(services.navigation.route()).toEqual({ screen: "browse", library: 1 });
  });

  // The owner's Fire TV, 2026-10-29: Back to a list showed its top, the entry left from on a later
  // page never drawn, so never focused.
  it("returned to by Back, loads its pages until the film it was left from is drawn, for focus to find it", async () => {
    const film = (id: number) => ({
      id,
      name: `Film ${String(id)}`,
      kind: "movie",
      identificationState: "identified",
      libraryId: 1,
      addedAt: 1,
    });
    const services = servedBy({
      "GET /libraries": [200, { libraries: [library(1, "Alpha")] }],
      "GET /titles": [
        [200, { titles: [film(1)], nextCursor: "c2" }],
        [200, { titles: [film(2)], nextCursor: "" }],
      ],
      "GET /titles/letters": lettersPage(),
    });
    services.navigation.noteOnEntry({ href: "/titles/2", ordinal: 0 });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(
          <LibraryBrowse
            library={1}
            view={undefined}
            folder={undefined}
            sort={undefined}
            query=""
          />,
          services,
        )}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector('a[href="/titles/2"]') !== null);
    expect(mounted.container.querySelector('a[href="/titles/1"]')).not.toBeNull();
  });

  it("defaults an other library's sort to newest recorded, and a films library's to name", async () => {
    const otherHeard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 5 },
      {
        "GET /libraries": [200, { libraries: [library(5, "Home movies", "other")] }],
        "GET /titles": titlesPage(),
      },
      otherHeard,
    );
    await settleUntil(() => otherHeard.some((each) => each.methodAndPath === "GET /titles"));
    const otherQuery = new URLSearchParams(
      otherHeard.find((each) => each.methodAndPath === "GET /titles")?.query,
    );
    expect(otherQuery.get("kind")).toBe("other");
    expect(otherQuery.get("sort")).toBe("recorded");
    mounted.unmount();

    const filmsHeard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films", "movies")] }],
        "GET /titles": titlesPage(),
        "GET /titles/letters": lettersPage(),
      },
      filmsHeard,
    );
    await settleUntil(() => filmsHeard.some((each) => each.methodAndPath === "GET /titles"));
    const filmsQuery = new URLSearchParams(
      filmsHeard.find((each) => each.methodAndPath === "GET /titles")?.query,
    );
    expect(filmsQuery.get("kind")).toBe("movie");
    expect(filmsQuery.get("sort")).toBe("name");
  });

  it("asks every library's titles, of any kind, for All libraries", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: "all" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films"), library(2, "Shows", "shows")] }],
        "GET /titles": titlesPage(),
        "GET /titles/letters": lettersPage(),
      },
      heard,
    );
    await settleUntil(() => heard.some((each) => each.methodAndPath === "GET /titles"));
    const query = new URLSearchParams(
      heard.find((each) => each.methodAndPath === "GET /titles")?.query,
    );
    expect(query.get("kind")).toBe("any");
    expect(query.has("library")).toBe(false);
  });

  it("offers a films library's every sort either way, by release date too, the way in the address (D366)", async () => {
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /titles": titlesPage(),
        "GET /titles/letters": lettersPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await click(buttonWithText(mounted.container, "Name, A–Z"));
    const links = [...mounted.container.querySelectorAll<HTMLAnchorElement>("a")].filter((link) =>
      link.href.includes("sort="),
    );
    expect(links.map((link) => link.textContent)).toEqual([
      "Name, A–Z",
      "Name, Z–A",
      "Newest released",
      "Oldest released",
      "Newest added",
      "Oldest added",
    ]);
    const hrefs = links.map(
      (link) => new URLSearchParams(link.getAttribute("href")?.split("?")[1]),
    );
    expect(hrefs.map((query) => `${query.get("sort") ?? ""}/${query.get("order") ?? ""}`)).toEqual([
      "name/",
      "name/descending",
      "released/",
      "released/ascending",
      "added/",
      "added/ascending",
    ]);
  });

  it("lists Z to A as its address says, with the rail's letters Z to A (D366 §2, §4)", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1, sort: "name", order: "descending" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /titles": titlesPage(),
        "GET /titles/letters": [200, { letters: ["A", "H", "Z"] }],
      },
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".alphabet-rail") !== null);
    const query = new URLSearchParams(
      heard.find((each) => each.methodAndPath === "GET /titles")?.query,
    );
    expect(query.get("sort")).toBe("name");
    expect(query.get("order")).toBe("descending");
    const shown = [...mounted.container.querySelectorAll(".alphabet-rail__letter")].map(
      (letter) => letter.textContent,
    );
    expect(shown).toEqual(["Z", "H", "A"]);
    expect(buttonWithText(mounted.container, "Name, Z–A")).toBeDefined();
  });

  it("asks an other library's folder either way (D366 §2)", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 5, view: "folders", sort: "recorded", order: "ascending" },
      {
        "GET /libraries": [200, { libraries: [library(5, "Home movies", "other")] }],
        "GET /libraries/5/folders": [200, { folder: "", subfolders: [], titles: [] }],
      },
      heard,
    );
    await settleUntil(() =>
      heard.some((each) => each.methodAndPath === "GET /libraries/5/folders"),
    );
    const folder = heard.find((each) => each.methodAndPath === "GET /libraries/5/folders");
    expect(new URLSearchParams(folder?.query).get("order")).toBe("ascending");
  });

  it("offers a library's own Configure on its list, for everyone, and none on All libraries (the owner, 2026-10-27)", async () => {
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /titles": titlesPage(),
        "GET /titles/letters": lettersPage(),
        "GET /me/preferences": [200, { isDialogueBoostOn: false, isLevellingOn: true }],
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const configure = buttonWithText(mounted.container, "Configure");
    expect(configure?.closest(".browse__bar")).not.toBeNull();
    await click(configure);
    await settleUntil(() => mounted?.container.querySelector("dialog input") !== null);
    expect(mounted.container.querySelector("dialog")?.textContent).toContain("Show in Films");
    mounted.unmount();
    mounted = await mountBrowse(
      { library: "all" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /titles": titlesPage(),
        "GET /titles/letters": lettersPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    expect(buttonWithText(mounted.container, "Configure")).toBeUndefined();
  });

  it("shows the alphabet rail only sorted by name or file name, never by newest or recorded", async () => {
    mounted = await mountBrowse(
      { library: 1, sort: "added" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /titles": titlesPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    expect(mounted.container.querySelector(".alphabet-rail")).toBeNull();
  });

  it("opens an other library on Folders, lists its subfolders and videos, and Up leaves a folder", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 5, view: "folders", folder: "2024/Summer" },
      {
        "GET /libraries": [200, { libraries: [library(5, "Home movies", "other")] }],
        "GET /libraries/5/folders": [
          200,
          {
            folder: "2024/Summer",
            subfolders: [{ name: "Beach", path: "2024/Summer/Beach", videoCount: 3 }],
            titles: [
              {
                id: 90,
                name: "Picnic",
                kind: "other",
                identificationState: "identified",
                libraryId: 5,
                addedAt: 1,
                recorded: { recordedAt: 1_700_000_000_000, from: "tag", fileName: "picnic.mp4" },
              },
            ],
          },
        ],
      },
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    expect(mounted.container.textContent).toContain("Beach");
    expect(mounted.container.textContent).toContain("Picnic");
    const crumbs = mounted.container.querySelectorAll<HTMLAnchorElement>(".up-trail a");
    const up = crumbs[crumbs.length - 1];
    expect(up?.getAttribute("href")).toContain("folder=2024");
    expect(up?.getAttribute("href")).not.toContain("folder=2024%2FSummer");
    const folderRequest = heard.find((each) => each.methodAndPath === "GET /libraries/5/folders");
    expect(new URLSearchParams(folderRequest?.query).get("folder")).toBe("2024/Summer");
  });

  it("sends the typed query, with the library chosen, to the address on submit", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [library(1, "Films")] }],
      "GET /titles": titlesPage(),
      "GET /titles/letters": lettersPage(),
    });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(
          <LibraryBrowse
            library={1}
            view={undefined}
            folder={undefined}
            sort={undefined}
            query=""
          />,
          services,
        )}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await enter(mounted.container.querySelector("input[type=search]"), "heat");
    await submit(mounted.container.querySelector("form[role=search]"));
    expect(services.navigation.route()).toEqual({ screen: "browse", library: 1, query: "heat" });
    // A search is a view of the list: it takes the entry's place, adding no step (D367 §9).
    expect(services.navigation.behind()).toEqual([]);
    // So is a sort.
    await click(buttonWithText(mounted.container, "Name, A–Z"));
    const newest = [...mounted.container.querySelectorAll<HTMLAnchorElement>("dialog a")].find(
      (link) => link.textContent === "Newest added",
    );
    await click(newest);
    expect(services.navigation.route()).toMatchObject({ screen: "browse", sort: "added" });
    expect(services.navigation.behind()).toEqual([]);
  });

  it("searches only the library chosen (GET /search's own library parameter)", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1, query: "heat" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /search": [200, { results: [{ id: 7, kind: "movie", name: "Metropolis" }] }],
      },
      heard,
    );
    await settleUntil(() => mounted?.container.textContent.includes("Metropolis") === true);
    const search = heard.find((each) => each.methodAndPath === "GET /search");
    expect(new URLSearchParams(search?.query).get("library")).toBe("1");
    expect(new URLSearchParams(search?.query).get("query")).toBe("heat");
    // The query the address names stays in the box too (as the retired Search page's did).
    expect(mounted.container.querySelector<HTMLInputElement>("input[type=search]")?.value).toBe(
      "heat",
    );
  });

  it("says when nothing matches, and keeps the query in the box", async () => {
    expect(renderToStaticMarkup(withServices(<SearchResults query="zzz" hits={[]} />))).toContain(
      "Nothing matches “zzz”.",
    );
    mounted = await mountBrowse(
      { library: 1, query: "heat" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /search": [200, { results: [] }],
      },
    );
    await settleUntil(() => mounted?.container.querySelector("form[role=search]") !== null);
    expect(mounted.container.querySelector<HTMLInputElement>("input[type=search]")?.value).toBe(
      "heat",
    );
    expect(mounted.container.querySelector("form[role=search]")).not.toBeNull();
  });
});
