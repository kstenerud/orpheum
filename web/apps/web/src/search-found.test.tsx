// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

/**
 * The search's grouped results (search-by-more §4.2) and recent searches (§4.5), in the search's
 * dialog: what the server's whole search finds shows under the field as typing rests, Titles, People
 * then Genres, each opening its own page and each with More; an answer to a search typed over never
 * replaces a newer one; and while the field is empty the device's recent searches show.
 */

import "./i18n.ts";

import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { GenreList } from "./search-found.tsx";
import { SearchButton } from "./listing-tools.tsx";
import { recentSearches, keepRecentSearch } from "./recent-searches.ts";
import {
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
import { testServices, withServices } from "./test-services.tsx";
import type { Services } from "./services.tsx";

/** A group of hits, with the cursor of its next page when it has one. */
function group(hits: readonly object[], nextCursor?: string): object {
  return nextCursor === undefined ? { hits } : { hits, nextCursor };
}

/** An answer of GET /search/grouped. */
function found(groups: { titles?: object; people?: object; genres?: object }): Answer {
  return [
    200,
    {
      titles: groups.titles ?? group([]),
      people: groups.people ?? group([]),
      genres: groups.genres ?? group([]),
    },
  ];
}

const metropolisFound = found({
  titles: group([
    { id: 4, kind: "movie", name: "Metropolis", year: 1995 },
    { id: 9, kind: "series", name: "Heatwave" },
  ]),
  people: group([{ id: 41, name: "Heather Graham" }], "p2"),
  genres: group([{ id: 3, name: "Heist", titleCount: 1 }]),
});

/** The search's magnifying glass over services, the list's own search reported to onSearch. */
async function mountSearch(
  services: Services,
  options: { readonly genreList?: GenreList; readonly onSearch?: (query: string) => void } = {},
): Promise<Mounted> {
  return await mount(
    withServices(
      <SearchButton
        query=""
        onSearch={options.onSearch ?? (() => undefined)}
        genreList={options.genreList ?? "films"}
      />,
      services,
    ),
  );
}

/** The dialog's field. */
function field(container: ParentNode): Element | null {
  return container.querySelector("dialog input[type=search]");
}

/** Each result shown, as its group's heading, its text and where it leads. */
function resultsIn(container: ParentNode): string[] {
  return [...container.querySelectorAll(".search-found__group")].flatMap((section) => {
    const heading = section.querySelector("h3")?.textContent ?? "";
    return [...section.querySelectorAll("a")].map(
      (link) => `${heading}|${link.textContent}|${link.getAttribute("href") ?? ""}`,
    );
  });
}

/** Sets the field to text as one keystroke does, its render and effects run at once. */
function typeAtOnce(input: Element | null, text: string): void {
  if (!(input instanceof HTMLInputElement)) {
    throw new Error("no input to type in");
  }
  act(() => {
    Reflect.set(HTMLInputElement.prototype, "value", text, input);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

/** The query each GET /search/grouped heard asked for. */
function searchesHeard(heard: readonly Heard[]): URLSearchParams[] {
  return heard
    .filter((each) => each.methodAndPath === "GET /search/grouped")
    .map((each) => new URLSearchParams(each.query));
}

describe("the search's grouped results (search-by-more §4.2)", () => {
  let mounted: Mounted | undefined;
  beforeEach(() => {
    window.localStorage.clear();
  });
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.localStorage.clear();
  });

  it("shows Titles, People then Genres under the field, each leading to its page", async () => {
    const heard: Heard[] = [];
    mounted = await mountSearch(servedBy({ "GET /search/grouped": metropolisFound }, heard));
    await click(mounted.container.querySelector('button[aria-label="Search"]'));
    await enter(field(mounted.container), "h");
    await settleUntil(() => resultsIn(mounted?.container ?? document).length > 0);
    expect(resultsIn(mounted.container)).toEqual([
      "Titles|Heat1995|/titles/4",
      "Titles|Heatwave|/series/9",
      // The initials circle before the name, as a title's people show (credits.tsx).
      "People|HGHeather Graham|/credited/41",
      "Genres|Heist1 title|/films?genre=3",
    ]);
    expect(searchesHeard(heard)[0]?.get("query")).toBe("h");
    expect(searchesHeard(heard)[0]?.has("group")).toBe(false);
  });

  it("opens a genre in Series from Series' search", async () => {
    mounted = await mountSearch(servedBy({ "GET /search/grouped": metropolisFound }), {
      genreList: "series",
    });
    await click(mounted.container.querySelector('button[aria-label="Search"]'));
    await enter(field(mounted.container), "hei");
    await settleUntil(() => resultsIn(mounted?.container ?? document).length > 0);
    expect(resultsIn(mounted.container).at(-1)).toBe("Genres|Heist1 title|/series?genre=3");
  });

  it("pages one group by its More, with that group's cursor, adding to what it shows", async () => {
    const heard: Heard[] = [];
    const nextPeople = found({ people: group([{ id: 42, name: "Heath Ledger" }]) });
    mounted = await mountSearch(
      servedBy({ "GET /search/grouped": [metropolisFound, nextPeople] }, heard),
    );
    await click(mounted.container.querySelector('button[aria-label="Search"]'));
    await enter(field(mounted.container), "hea");
    await settleUntil(() => resultsIn(mounted?.container ?? document).length > 0);
    // Only the group with a next page has More.
    const mores = mounted.container.querySelectorAll(".search-found__more");
    expect(mores).toHaveLength(1);
    await click(mores[0]);
    await settleUntil(() => resultsIn(mounted?.container ?? document).length === 5);
    const paged = searchesHeard(heard)[1];
    expect([paged?.get("query"), paged?.get("group"), paged?.get("cursor")]).toEqual([
      "hea",
      "people",
      "p2",
    ]);
    expect(resultsIn(mounted.container).filter((each) => each.startsWith("People|"))).toEqual([
      "People|HGHeather Graham|/credited/41",
      "People|HLHeath Ledger|/credited/42",
    ]);
    // Its last page: no More.
    expect(mounted.container.querySelector(".search-found__more")).toBeNull();
  });

  it("asks once typing rests, not at each letter", async () => {
    const heard: Heard[] = [];
    mounted = await mountSearch(servedBy({ "GET /search/grouped": metropolisFound }, heard));
    await click(mounted.container.querySelector('button[aria-label="Search"]'));
    typeAtOnce(field(mounted.container), "h");
    typeAtOnce(field(mounted.container), "he");
    typeAtOnce(field(mounted.container), "hea");
    await settleUntil(() => resultsIn(mounted?.container ?? document).length > 0);
    expect(searchesHeard(heard).map((query) => query.get("query"))).toEqual(["hea"]);
  });

  it("never lets a slower answer to a search typed over replace the newer one's", async () => {
    const asked: string[] = [];
    let answerOlder: (() => void) | undefined;
    const older = new Promise<void>((resolve) => {
      answerOlder = resolve;
    });
    const answer = (body: Answer): Response => Response.json(body[1], { status: body[0] });
    const services = testServices("token", "/films", async (request) => {
      const query = new URL(request.url).searchParams.get("query") ?? "";
      asked.push(query);
      if (query === "he") {
        await older;
        return answer(found({ titles: group([{ id: 1, kind: "movie", name: "Hell" }]) }));
      }
      return answer(found({ titles: group([{ id: 4, kind: "movie", name: "Metropolis" }]) }));
    });
    mounted = await mountSearch(services);
    await click(mounted.container.querySelector('button[aria-label="Search"]'));
    await enter(field(mounted.container), "he");
    await settleUntil(() => asked.includes("he"));
    await enter(field(mounted.container), "hea");
    await settleUntil(() => resultsIn(mounted?.container ?? document).length > 0);
    await act(async () => {
      answerOlder?.();
      await new Promise((resolve) => setTimeout(resolve, 50));
    });
    expect(resultsIn(mounted.container)).toEqual(["Titles|Metropolis|/titles/4"]);
  });

  it("says when nothing matches", async () => {
    mounted = await mountSearch(servedBy({ "GET /search/grouped": found({}) }));
    await click(mounted.container.querySelector('button[aria-label="Search"]'));
    await enter(field(mounted.container), "zzz");
    await settleUntil(
      () => mounted?.container.querySelector(".search-found")?.textContent !== undefined,
    );
    expect(mounted.container.querySelector(".search-found")?.textContent).toBe(
      "Nothing matches that.",
    );
  });
});

describe("recent searches (search-by-more §4.5)", () => {
  let mounted: Mounted | undefined;
  beforeEach(() => {
    window.localStorage.clear();
  });
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.localStorage.clear();
    vi.restoreAllMocks();
  });

  it("keeps a handful, newest first, the same search once in any case", () => {
    for (const search of ["a", "b", "c", "d", "e", "f", "g"]) {
      keepRecentSearch(search);
    }
    keepRecentSearch("  C ");
    keepRecentSearch("");
    expect(recentSearches()).toEqual(["C", "g", "f", "e", "d", "b"]);
    expect(window.localStorage.getItem("orpheum.search.recent")).toBe(
      JSON.stringify(["C", "g", "f", "e", "d", "b"]),
    );
  });

  it("reads nothing from a kept value it cannot read", () => {
    window.localStorage.setItem("orpheum.search.recent", "{not json");
    expect(recentSearches()).toEqual([]);
  });

  it("shows them while the field is empty, each putting itself back in the field", async () => {
    keepRecentSearch("noir");
    keepRecentSearch("nolan");
    mounted = await mountSearch(servedBy({ "GET /search/grouped": metropolisFound }));
    await click(mounted.container.querySelector('button[aria-label="Search"]'));
    const recent = [...mounted.container.querySelectorAll(".search-found__recent button")];
    expect(mounted.container.querySelector(".search-found h3")?.textContent).toBe(
      "Recent searches",
    );
    expect(recent.map((button) => button.textContent)).toEqual(["nolan", "noir"]);
    await click(recent[1]);
    expect((field(mounted.container) as HTMLInputElement | null)?.value).toBe("noir");
    await settleUntil(() => resultsIn(mounted?.container ?? document).length > 0);
    expect(mounted.container.querySelector(".search-found__recent")).toBeNull();
  });

  it("keeps a search submitted, which filters the list as before", async () => {
    const searched: string[] = [];
    mounted = await mountSearch(servedBy({ "GET /search/grouped": metropolisFound }), {
      onSearch: (query) => searched.push(query),
    });
    await click(mounted.container.querySelector('button[aria-label="Search"]'));
    await enter(field(mounted.container), " heat ");
    await submit(mounted.container.querySelector("dialog form[role=search]"));
    expect(searched).toEqual(["heat"]);
    expect(recentSearches()).toEqual(["heat"]);
  });

  it("keeps a search a result is opened from, and opening it closes the dialog for its page", async () => {
    const services = servedBy({ "GET /search/grouped": metropolisFound });
    mounted = await mountSearch(services);
    await click(mounted.container.querySelector('button[aria-label="Search"]'));
    await enter(field(mounted.container), "heath");
    await settleUntil(() => resultsIn(mounted?.container ?? document).length > 0);
    await click(mounted.container.querySelector('a[href="/credited/41"]'));
    expect(services.navigation.route()).toEqual({ screen: "credited-person", personId: 41 });
    expect(mounted.container.querySelector("dialog")).toBeNull();
    expect(recentSearches()).toEqual(["heath"]);
  });
});
