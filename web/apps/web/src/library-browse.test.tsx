// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { act, type JSX } from "react";

import { LibraryBrowse } from "./library-browse.tsx";
import { typingRestMs } from "./listing-tools.tsx";
import { chipsOf } from "./list-filters.ts";
import { useRoute } from "./services.tsx";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Answer,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import {
  chooseSort,
  closeCriteria,
  criteriaButton,
  narrowByName,
  openCriteria,
} from "./test-list-criteria.tsx";

/** A poller that never asks: a list is read only as the test's steps ask. */
const neverAsks: Poller = { start: () => () => undefined };

/** A library, as the server sends it. */
function library(
  id: number,
  name: string,
  kind: "movie" | "series" | "other" = "movie",
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
    items: [
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
  // A films' or a series' library asks its genres as its Sort and filter opens (list-chips.tsx).
  const services = servedBy({ "GET /genres": [200, { genres: [] }], ...answers }, heard);
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

/** The Libraries page as app.tsx shows it: what it lists read from the address it replaces. */
function RoutedLibraryBrowse(): JSX.Element {
  const route = useRoute();
  if (route.screen !== "browse") {
    return <p>{route.screen}</p>;
  }
  return (
    <LibraryBrowse
      library={route.library}
      view={route.view}
      folder={route.folder}
      sort={route.sort}
      order={route.order}
      query={route.query ?? ""}
      filters={chipsOf(route)}
    />
  );
}

/** The sort's options in the Sort and filter, opened for them: each its field after an arrow. */
async function sortOptions(container: HTMLElement): Promise<HTMLButtonElement[]> {
  const popUp = await openCriteria(container);
  const chooser = [...popUp.querySelectorAll(".route-chooser")].find(
    (each) => each.querySelector(".route-chooser__field")?.textContent === "Sort",
  );
  await click(chooser?.querySelector("button[aria-haspopup]"));
  return [...(chooser?.querySelectorAll<HTMLButtonElement>("dialog li button") ?? [])];
}

/** The bar's gear, a library's Configure. */
function configureButton(container: HTMLElement): HTMLButtonElement | null {
  return container.querySelector<HTMLButtonElement>('.browse__bar button[aria-label="Configure"]');
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
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const bar = mounted.container.querySelector(".browse__bar");
    expect(bar?.querySelector("h1")?.textContent).toBe("Alpha");
    expect(buttonWithText(mounted.container, "Alpha")).toBeUndefined();
    expect(bar?.querySelector('[aria-label^="Library:"]')).toBeNull();
    expect(bar?.querySelector('a[href*="library="]')).toBeNull();
    // Its sort stays, in its Sort and filter (D417).
    expect(bar?.querySelectorAll(".list-criteria__button")).toHaveLength(1);
    mounted.unmount();
    mounted = await mountBrowse(
      { library: "all" },
      {
        "GET /libraries": [200, { libraries: [library(2, "Zoo"), library(1, "Alpha")] }],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    expect(mounted.container.querySelector(".browse__bar h1")?.textContent).toBe("Libraries");
  });

  it("names neither the library chooser nor the search on the page, only to a screen reader (the owner, 2026-10-27)", async () => {
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [200, { libraries: [library(1, "Alpha")] }],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const bar = mounted.container.querySelector(".browse__bar");
    const fieldNames = [...(bar?.querySelectorAll(".route-chooser__field") ?? [])].map(
      (field) => field.textContent,
    );
    expect(fieldNames).not.toContain("Library");
    // The text filter is in the pop-up its Sort and filter opens (D417), not on the bar, named to
    // a screen reader by its own label, no label element on the page.
    expect(bar?.querySelector("input.list-criteria__name")).toBeNull();
    const popUp = await openCriteria(mounted.container);
    const field = popUp.querySelector("input.list-criteria__name");
    expect(field?.getAttribute("aria-label")).toBe("Name contains");
    expect(popUp.querySelector("label")).toBeNull();
  });

  it("picks the first library by name when none is chosen in the address (D349 §9.1 point 1)", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [library(2, "Zoo"), library(1, "Alpha")] }],
      "GET /items": titlesPage(),
      "GET /items/letters": lettersPage(),
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
      "GET /items": [
        [200, { items: [film(1)], nextCursor: "c2" }],
        [200, { items: [film(2)], nextCursor: "" }],
      ],
      "GET /items/letters": lettersPage(),
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
        "GET /items": titlesPage(),
      },
      otherHeard,
    );
    await settleUntil(() => otherHeard.some((each) => each.methodAndPath === "GET /items"));
    const otherQuery = new URLSearchParams(
      otherHeard.find((each) => each.methodAndPath === "GET /items")?.query,
    );
    expect(otherQuery.get("kind")).toBe("other");
    expect(otherQuery.get("sort")).toBe("recorded");
    mounted.unmount();

    const filmsHeard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films", "movie")] }],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
      filmsHeard,
    );
    await settleUntil(() => filmsHeard.some((each) => each.methodAndPath === "GET /items"));
    const filmsQuery = new URLSearchParams(
      filmsHeard.find((each) => each.methodAndPath === "GET /items")?.query,
    );
    expect(filmsQuery.get("kind")).toBe("movie");
    expect(filmsQuery.get("sort")).toBe("name");
  });

  it("asks every library's titles, of any kind, for All libraries", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: "all" },
      {
        "GET /libraries": [
          200,
          { libraries: [library(1, "Films"), library(2, "Shows", "series")] },
        ],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
      heard,
    );
    await settleUntil(() => heard.some((each) => each.methodAndPath === "GET /items"));
    const query = new URLSearchParams(
      heard.find((each) => each.methodAndPath === "GET /items")?.query,
    );
    expect(query.get("kind")).toBe("movie,series,other");
    expect(query.has("library")).toBe(false);
  });

  it("offers a films library's every sort either way, by release date too, the way in the address (D366)", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [library(1, "Films")] }],
      "GET /items": titlesPage(),
      "GET /items/letters": lettersPage(),
      "GET /genres": [200, { genres: [] }],
    });
    services.navigation.replace({ screen: "browse", library: 1 });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedLibraryBrowse />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const options = await sortOptions(mounted.container);
    // Each shows its field after an arrow, the way it runs said to a screen reader (the owner,
    // 2026-10-30).
    expect(options.map((option) => option.textContent)).toEqual([
      "Name",
      "Name",
      "Released",
      "Released",
      "Added",
      "Added",
    ]);
    const spoken = [
      "Name, A–Z",
      "Name, Z–A",
      "Oldest released",
      "Newest released",
      "Oldest added",
      "Newest added",
    ];
    expect(options.map((option) => option.getAttribute("aria-label"))).toEqual(spoken);
    await closeCriteria(mounted.container);
    // The address each puts the sort in, its own way left out, chosen last to first so each is a
    // change from the one before.
    const addresses: string[] = [];
    for (const label of spoken.toReversed()) {
      await chooseSort(mounted.container, label);
      const route = services.navigation.route();
      addresses.unshift(
        route.screen === "browse" ? `${route.sort ?? ""}/${route.order ?? ""}` : route.screen,
      );
    }
    expect(addresses).toEqual([
      "name/",
      "name/descending",
      "released/ascending",
      "released/",
      "added/ascending",
      "added/",
    ]);
  });

  it("lists Z to A as its address says, with the rail's letters Z to A (D366 §2, §4)", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1, sort: "name", order: "descending" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /items": titlesPage(),
        "GET /items/letters": [200, { letters: ["A", "H", "Z"] }],
      },
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".alphabet-rail") !== null);
    const query = new URLSearchParams(
      heard.find((each) => each.methodAndPath === "GET /items")?.query,
    );
    expect(query.get("sort")).toBe("name");
    expect(query.get("order")).toBe("descending");
    const shown = [...mounted.container.querySelectorAll(".alphabet-rail__letter")].map(
      (letter) => letter.textContent,
    );
    expect(shown).toEqual(["Z", "H", "A"]);
    // The sort's choice is said by the Sort and filter, to a screen reader and in its summary (D417).
    expect(criteriaButton(mounted.container)?.getAttribute("aria-label")).toBe(
      "Sort and filter: Name, Z–A",
    );
    expect(criteriaButton(mounted.container)?.textContent).toBe("Sort and filterName, Z–A");
  });

  it("shows an other library's folder whole, following each page's cursor (D432 §3)", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 5, view: "folders" },
      {
        "GET /libraries": [200, { libraries: [library(5, "Home movies", "other")] }],
        "GET /libraries/5/folders": [
          [
            200,
            {
              folder: "",
              subfolders: [{ name: "Beach", path: "Beach", videoCount: 3 }],
              titles: [],
              nextCursor: "next",
            },
          ],
          [
            200,
            {
              folder: "",
              subfolders: [{ name: "Garden", path: "Garden", videoCount: 1 }],
              titles: [],
            },
          ],
        ],
      },
      heard,
    );
    await settleUntil(() => mounted?.container.textContent.includes("Garden") === true);
    expect(mounted.container.textContent).toContain("Beach");
    const cursors = heard
      .filter((each) => each.methodAndPath === "GET /libraries/5/folders")
      .map((each) => new URLSearchParams(each.query).get("cursor"));
    expect(cursors.slice(0, 2)).toEqual([null, "next"]);
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
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
        "GET /me/preferences": [200, { isDialogueBoostOn: false, isLevellingOn: true }],
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const configure = configureButton(mounted.container);
    expect(configure?.closest(".browse__bar")).not.toBeNull();
    await click(configure);
    await settleUntil(() => mounted?.container.querySelector("dialog input") !== null);
    expect(mounted.container.querySelector("dialog")?.textContent).toContain("Show in Films");
    mounted.unmount();
    mounted = await mountBrowse(
      { library: "all" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    expect(configureButton(mounted.container)).toBeNull();
  });

  it("shows the alphabet rail only sorted by name or file name, never by newest or recorded", async () => {
    mounted = await mountBrowse(
      { library: 1, sort: "added" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /items": titlesPage(),
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

  it("sends the typed text, with the library chosen, to the address as its pop-up closes", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [library(1, "Films")] }],
      "GET /items": titlesPage(),
      "GET /items/letters": lettersPage(),
      "GET /genres": [200, { genres: [] }],
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
    await narrowByName(mounted.container, "heat");
    expect(services.navigation.route()).toEqual({ screen: "browse", library: 1, query: "heat" });
    // A search is a view of the list: it takes the entry's place, adding no step (D367 §9).
    expect(services.navigation.behind()).toEqual([]);
    // So is a sort.
    await chooseSort(mounted.container, "Newest added");
    expect(services.navigation.route()).toMatchObject({ screen: "browse", sort: "added" });
    expect(services.navigation.behind()).toEqual([]);
  });

  // The owner, 2026-10-30: "a "Show all" button that removes all search criteria, closes the dialog,
  // and refreshes the list"; D417 makes it the pop-up's Clear, which clears every filter but the
  // sort, the list changing under the pop-up, and goes as it clears.
  // Each letter of a word is not a request: the list asks for what is typed once typing rests.
  it("asks for the text typed in its Sort and filter once typing rests, not at each letter", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const popUp = await openCriteria(mounted.container);
    const field = popUp.querySelector("input.list-criteria__name");
    const queriesAsked = (): (string | null)[] =>
      heard
        .filter((each) => each.methodAndPath === "GET /items")
        .map((each) => new URLSearchParams(each.query).get("query"));
    expect(queriesAsked()).toEqual([null]);
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    try {
      for (const typed of ["h", "he", "hea", "heat"]) {
        await act(async () => {
          Reflect.set(HTMLInputElement.prototype, "value", typed, field);
          field?.dispatchEvent(new Event("input", { bubbles: true }));
          await Promise.resolve();
        });
        await act(async () => {
          vi.advanceTimersByTime(typingRestMs - 1);
          await Promise.resolve();
        });
      }
      // Shown at once in the button's summary, though not yet asked for.
      expect(criteriaButton(mounted.container)?.getAttribute("title")).toBe("Name · “heat”");
      await act(async () => {
        vi.advanceTimersByTime(1);
        await Promise.resolve();
      });
    } finally {
      vi.useRealTimers();
    }
    await settleUntil(() => queriesAsked().includes("heat"));
    expect(queriesAsked()).toEqual([null, "heat"]);
  });

  // The 11-08 batch: a list asked again after its pop-up closed replaced the list an arrow pressed
  // after Escape had moved focus onto, and focus fell to the page. Closing does not wait the rest.
  it("asks for the text typed in its Sort and filter at once as its pop-up closes", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const popUp = await openCriteria(mounted.container);
    const field = popUp.querySelector("input.list-criteria__name");
    const queriesAsked = (): (string | null)[] =>
      heard
        .filter((each) => each.methodAndPath === "GET /items")
        .map((each) => new URLSearchParams(each.query).get("query"));
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    try {
      await act(async () => {
        Reflect.set(HTMLInputElement.prototype, "value", "heat", field);
        field?.dispatchEvent(new Event("input", { bubbles: true }));
        await Promise.resolve();
      });
      await closeCriteria(mounted.container);
      // No time passes: the rest is not waited once the pop-up is closed.
      await act(async () => {
        vi.advanceTimersByTime(0);
        await Promise.resolve();
      });
      expect(queriesAsked()).toEqual([null, "heat"]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("drops the search by its pop-up's Clear, the list shown again", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
        "GET /genres": [200, { genres: [] }],
      },
      heard,
    );
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(
          <LibraryBrowse
            library={1}
            view={undefined}
            folder={undefined}
            sort={undefined}
            query="heat"
          />,
          services,
        )}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const popUp = await openCriteria(mounted.container);
    await click(buttonWithText(popUp, "Clear"));
    await settleUntil(
      () => heard.filter((each) => each.methodAndPath === "GET /items").length >= 2,
    );
    const titlesAsked = heard.filter((each) => each.methodAndPath === "GET /items");
    expect(new URLSearchParams(titlesAsked[1]?.query).get("query")).toBeNull();
    expect(popUp.querySelector<HTMLInputElement>("input.list-criteria__name")?.value).toBe("");
    expect(buttonWithText(popUp, "Clear")).toBeUndefined();
    await closeCriteria(mounted.container);
    expect(mounted.container.querySelector("dialog")).toBeNull();
    expect(services.navigation.route()).toEqual({ screen: "browse", library: 1 });
  });

  // The owner, 2026-10-30: search is a criterion of the one list, as its library and sort are.
  it("lists the library chosen, as sorted, filtered by its search (GET /items' query)", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1, query: "heat" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const titles = new URLSearchParams(
      heard.find((each) => each.methodAndPath === "GET /items")?.query,
    );
    expect([titles.get("library"), titles.get("query"), titles.get("sort")]).toEqual([
      "1",
      "heat",
      "name",
    ]);
    // The rail's letters are the search's too.
    const letters = new URLSearchParams(
      heard.find((each) => each.methodAndPath === "GET /items/letters")?.query,
    );
    expect(letters.get("query")).toBe("heat");
    // The query the address names is in the field its Sort and filter opens (as the retired Search
    // page's was).
    const popUp = await openCriteria(mounted.container);
    expect(popUp.querySelector<HTMLInputElement>("input.list-criteria__name")?.value).toBe("heat");
  });

  it("says when nothing matches its search", async () => {
    mounted = await mountBrowse(
      { library: 1, query: "zzz" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /items": [200, { items: [], nextCursor: "" }],
        "GET /items/letters": lettersPage(),
      },
    );
    await settleUntil(() => mounted?.container.querySelector(".browse__empty") !== null);
    expect(mounted.container.querySelector(".browse__empty")?.textContent).toBe(
      "Nothing matches “zzz”.",
    );
  });
});
