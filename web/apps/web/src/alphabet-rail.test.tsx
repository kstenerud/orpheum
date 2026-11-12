// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";

import { fromForLetter, railLetters } from "./alphabet-rail.tsx";
import type { JSX } from "react";

import { Browse, type BrowseKind, type Entry } from "./browse.tsx";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import {
  buttonNamed,
  click,
  mount,
  servedBy,
  settleUntil,
  type Answer,
  type AnswerByQuery,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { useRoute } from "./services.tsx";
import { withServices } from "./test-services.tsx";
import { chooseSort, closeCriteria, narrowByName, openCriteria } from "./test-list-criteria.tsx";

/** Films or Series as app.tsx shows it: its sort and search read from the address it replaces. */
function RoutedBrowse(props: { readonly kind: BrowseKind }): JSX.Element {
  const route = useRoute();
  const state = route.screen === "films" || route.screen === "series" ? route : {};
  return <Browse kind={props.kind} state={state} />;
}

/** A poller that never asks: the list is read only as the test's steps ask. */
const neverAsks: Poller = { start: () => () => undefined };

/** A film called name, by its id. */
function film(id: number, name: string): Entry {
  return { id, name, kind: "film", identificationState: "identified", addedAt: id };
}

/** A page of the films list, with the cursor of the next, absent on the last. */
function page(entries: readonly Entry[], nextCursor: string): Answer {
  return [200, { items: entries, ...(nextCursor === "" ? {} : { nextCursor }) }];
}

const firstPage = page([film(1, "12 Patient Sailors"), film(2, "Arkon")], "c1");
const secondPage = page([film(3, "Brindlewood"), film(4, "Hearthstead")], "c2");
const lastPage = page([film(5, "Mabel"), film(6, "Zelkaro")], "");

/**
 * GET /items as the list asks it: a page after another by its cursor, from byCursor, an empty last
 * page for a cursor it does not hold; every other ask (a first page, a page from a letter) answered
 * by turns in turn, the last repeated. Films reads its whole list as soon as its first page is shown
 * (the owner, 2026-11-11), so its pages after the first are asked while the test's steps run.
 */
function films(
  turns: readonly Answer[],
  byCursor: Readonly<Record<string, Answer>> = {},
): AnswerByQuery {
  let turn = 0;
  return (query) => {
    const cursor = query.get("cursor");
    if (cursor !== null) {
      return byCursor[cursor] ?? page([], "");
    }
    const answer = turns[Math.min(turn, turns.length - 1)] ?? page([], "");
    turn += 1;
    return answer;
  };
}

/** The films' list, by name, with the rail beside it, answered by answers. */
async function mountFilms(answers: AnswerByQuery, heard: Heard[]): Promise<Mounted> {
  const services = servedBy(
    { "GET /items": answers, "GET /items/letters": [200, { letters: railLetters }] },
    heard,
  );
  return await mount(
    <BackgroundChangesContext value={neverAsks}>
      {withServices(<RoutedBrowse kind="film" />, services)}
    </BackgroundChangesContext>,
  );
}

/** The name of the poster that has focus, or undefined when none has. */
function focusedName(): string | undefined {
  return document.activeElement?.querySelector(".poster-card__name")?.textContent ?? undefined;
}

/** The alphabet rail's letter in container, a button by role. */
function railLetter(container: HTMLElement, letter: string): HTMLElement | undefined {
  return [...container.querySelectorAll<HTMLElement>(".alphabet-rail__letter")].find(
    (each) => each.textContent === letter,
  );
}

/** The requests heard against GET /items alone, in order: the rail's own letters are a sibling
 * request (alphabet-rail.tsx's useAlphabetRailLetters) these tests do not count by position. */
function titlesHeard(heard: readonly Heard[]): Heard[] {
  return heard.filter((each) => each.methodAndPath === "GET /items");
}

/** The lists GET /items was asked for, in order: each one's start, its pages after it left out. */
function listsHeard(heard: readonly Heard[]): Heard[] {
  return titlesHeard(heard).filter((each) => !new URLSearchParams(each.query).has("cursor"));
}

/** The cursors the list's pages after the first were read by, in order. */
function cursorsHeard(heard: readonly Heard[]): (string | null)[] {
  return titlesHeard(heard)
    .map((each) => new URLSearchParams(each.query).get("cursor"))
    .filter((cursor) => cursor !== null);
}

describe("the alphabet rail (the owner, 2026-10-25)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("moves the list to a letter's first title, loading the pages before it, and focuses it", async () => {
    const heard: Heard[] = [];
    // The list's first page; the server's first title from M; then the pages up to it.
    mounted = await mountFilms(
      films([firstPage, page([film(5, "Mabel")], "")], { c1: secondPage, c2: lastPage }),
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await click(railLetter(mounted.container, "M"));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("Mabel");
    const queries = listsHeard(heard).map((each) => new URLSearchParams(each.query));
    expect(queries[1]?.get("from")).toBe("M");
    expect(queries[1]?.get("sort")).toBe("name");
    expect(queries[1]?.get("limit")).toBe("1");
    expect(cursorsHeard(heard)).toEqual(["c1", "c2"]);
    expect(mounted.container.querySelectorAll(".poster-grid > li")).toHaveLength(6);
  });

  it("loads the page after a title jumped to when the page's end stops it short of the top, and shows it there (the owner, 2026-11-02)", async () => {
    const heard: Heard[] = [];
    // Mabel ends the second page; the page ends below it, too soon to scroll it to the top,
    // until the page after it is drawn.
    const posterTop = vi
      .spyOn(HTMLElement.prototype, "getBoundingClientRect")
      .mockImplementation(function (this: HTMLElement) {
        const isShort = document.querySelectorAll(".poster-grid > li").length <= 4;
        const top = isShort && this.textContent.includes("Mabel") ? 500 : 0;
        return DOMRect.fromRect({ x: 0, y: top, width: 100, height: 100 });
      });
    // The browser keeps what is on screen in place as the list grows below it, so the page's
    // scroll moves by itself between one showing and the next; no person moved it.
    let scrolled = 0;
    const scrollY = vi.spyOn(window, "scrollY", "get").mockImplementation(() => (scrolled += 24));
    // How many posters were drawn each time Mabel's was scrolled to.
    const shownAt: number[] = [];
    const scrolledTo = vi
      .spyOn(HTMLElement.prototype, "scrollIntoView")
      .mockImplementation(function (this: HTMLElement) {
        if (this.textContent.includes("Mabel")) {
          shownAt.push(document.querySelectorAll(".poster-grid > li").length);
        }
      });
    onTestFinished(() => {
      posterTop.mockRestore();
      scrollY.mockRestore();
      scrolledTo.mockRestore();
    });
    mounted = await mountFilms(
      films([firstPage, page([film(4, "Mabel")], "")], {
        c1: page([film(3, "Brindlewood"), film(4, "Mabel")], "c2"),
        c2: page([film(5, "Zelkaro"), film(6, "Zorvane")], ""),
      }),
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await click(railLetter(mounted.container, "M"));
    await settleUntil(() => mounted?.container.querySelectorAll(".poster-grid > li").length === 6);
    await settleUntil(() => shownAt.includes(6));
    expect(focusedName()).toBe("Mabel");
    expect(cursorsHeard(heard)).toEqual(["c1", "c2"]);
  });

  it("moves to the first title of all for #, those beginning with a digit, loading nothing more", async () => {
    const heard: Heard[] = [];
    mounted = await mountFilms(
      films([firstPage, page([film(1, "12 Patient Sailors")], "x")]),
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await click(railLetter(mounted.container, "#"));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("12 Patient Sailors");
    const onlyTitles = listsHeard(heard);
    expect(new URLSearchParams(onlyTitles[1]?.query).has("from")).toBe(false);
    expect(onlyTitles).toHaveLength(2);
  });

  it("moves to the last title for a letter no title comes at or after", async () => {
    const heard: Heard[] = [];
    mounted = await mountFilms(
      films([firstPage, page([], "")], { c1: secondPage, c2: lastPage }),
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await click(railLetter(mounted.container, "Z"));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("Zelkaro");
  });

  // Silk on a Fire TV lays the page out each time a key focuses a <button>: a move onto the rail
  // took 300 ms, the first 5 s (the 11-24 batch's item 12; idiosyncrasies.md).
  it("names its letters buttons by role, not <button>s, and Space presses one", async () => {
    const heard: Heard[] = [];
    mounted = await mountFilms(
      films([firstPage, page([film(5, "Mabel")], "")], { c1: secondPage, c2: lastPage }),
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const letters = [...mounted.container.querySelectorAll(".alphabet-rail__letter")];
    expect(letters.length).toBeGreaterThan(0);
    for (const letter of letters) {
      expect(letter.tagName).not.toBe("BUTTON");
      expect(letter.getAttribute("role")).toBe("button");
    }
    const m = railLetter(mounted.container, "M");
    m?.focus();
    m?.dispatchEvent(new KeyboardEvent("keydown", { key: " ", bubbles: true, cancelable: true }));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("Mabel");
  });
});

// The owner, 2026-11-11: a list's text comes whole, up front, so a held Down or a letter never
// waits on a page; even a thousand titles is light.
describe("Films' whole list, read as soon as its first page is shown", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("reads page after page, with no view asking, 60 first and 120 a page after", async () => {
    const heard: Heard[] = [];
    mounted = await mountFilms(films([firstPage], { c1: secondPage, c2: lastPage }), heard);
    await settleUntil(() => mounted?.container.querySelector('a[href="/titles/6"]') !== null);
    expect(cursorsHeard(heard)).toEqual(["c1", "c2"]);
    const limits = titlesHeard(heard).map((each) => new URLSearchParams(each.query).get("limit"));
    expect(limits).toEqual(["60", "120", "120"]);
  });
});

describe("Films' and Series' rail shows only letters that lead somewhere (D349 §9)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows only the letters the server says the list has titles under, sorted by name", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items": films([firstPage]),
        "GET /items/letters": [200, { letters: ["#", "B", "M"] }],
        "GET /genres": [200, { genres: [] }],
      },
      heard,
    );
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedBrowse kind="film" />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".alphabet-rail") !== null);
    const shown = [...mounted.container.querySelectorAll(".alphabet-rail__letter")].map(
      (letter) => letter.textContent,
    );
    expect(shown).toEqual(["#", "B", "M"]);
    const letters = heard.find((each) => each.methodAndPath === "GET /items/letters");
    expect(new URLSearchParams(letters?.query).get("kind")).toBe("film");
    expect(new URLSearchParams(letters?.query).get("sort")).toBe("name");
  });

  it("shows no rail, and asks the server for no letters, sorted newest first", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items": films([firstPage, firstPage]),
        "GET /items/letters": [200, { letters: [] }],
        "GET /genres": [200, { genres: [] }],
      },
      heard,
    );
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedBrowse kind="series" />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    expect(mounted.container.querySelector(".alphabet-rail")).not.toBeNull();
    const lettersAsksByName = heard.filter(
      (each) => each.methodAndPath === "GET /items/letters",
    ).length;
    expect(lettersAsksByName).toBe(1);
    await chooseSort(mounted.container, "Newest added");
    await settleUntil(() => listsHeard(heard).length >= 2);
    expect(mounted.container.querySelector(".alphabet-rail")).toBeNull();
    // The rail unmounts sorted by added: its letters are not asked for again.
    expect(heard.filter((each) => each.methodAndPath === "GET /items/letters")).toHaveLength(
      lettersAsksByName,
    );
  });
});

describe("Films' and Series' search (the owner, 2026-10-30)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("lists its own kind, as sorted, filtered by the search (GET /items' query)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items": films([firstPage, page([film(4, "Hearthstead")], "")]),
        "GET /items/letters": [200, { letters: [] }],
        "GET /genres": [200, { genres: [] }],
      },
      heard,
    );
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedBrowse kind="film" />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    expect(mounted.container.querySelector("h1")?.textContent).toBe("Films");
    await narrowByName(mounted.container, "hearth");
    await settleUntil(() => listsHeard(heard).length >= 2);
    const searched = new URLSearchParams(listsHeard(heard)[1]?.query);
    expect([searched.get("kind"), searched.get("query"), searched.get("sort")]).toEqual([
      "film",
      "hearth",
      "name",
    ]);
    await settleUntil(() => mounted?.container.querySelector('a[href="/titles/4"]') !== null);
    expect(mounted.container.querySelector('a[href="/titles/1"]')).toBeNull();
    // What is typed narrows this list alone: the search across titles, people and genres is not
    // asked (D417 §2).
    expect(heard.filter((each) => each.methodAndPath.startsWith("GET /search"))).toEqual([]);
  });
});

// The Fire TV, 2026-11-02: Back from a film opened from a search showed the whole list again, the
// search and the sort held in the page, which a title's page in its place unmounts (the owner:
// keep them in the address, as the Libraries page does).
describe("Films' search and sort, kept in the address for Back to return to", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("puts a search and a sort chosen in the address", async () => {
    const services = servedBy({
      "GET /items": films([
        firstPage,
        page([film(4, "Hearthstead")], ""),
        page([film(4, "Hearthstead")], ""),
      ]),
      "GET /items/letters": [200, { letters: [] }],
      "GET /genres": [200, { genres: [] }],
    });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedBrowse kind="film" />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await narrowByName(mounted.container, "hearth");
    await settleUntil(() => mounted?.container.querySelector('a[href="/titles/1"]') === null);
    await chooseSort(mounted.container, "Oldest added");
    expect(services.navigation.route()).toEqual({
      screen: "films",
      sort: "added",
      order: "ascending",
      query: "hearth",
    });
  });

  it("lists by the search and the sort its address holds, as Back returns to it", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items": [page([film(4, "Hearthstead")], "")],
        "GET /items/letters": [200, { letters: [] }],
        "GET /genres": [200, { genres: [] }],
      },
      heard,
    );
    services.navigation.replace({ screen: "films", sort: "released", query: "hearth" });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedBrowse kind="film" />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => titlesHeard(heard).length >= 1);
    const asked = new URLSearchParams(titlesHeard(heard)[0]?.query);
    expect([asked.get("query"), asked.get("sort"), asked.get("order")]).toEqual([
      "hearth",
      "released",
      null,
    ]);
    expect(
      mounted.container.querySelector(".list-criteria__button")?.getAttribute("aria-label"),
    ).toBe("Sort and filter: Released · “hearth”");
  });
});

describe("the rail follows a name sort's direction (D366 §4)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("starts a jump Z to A at the letter after it, Z at the start and # at A", () => {
    expect(fromForLetter("M", false)).toEqual({ from: "M" });
    expect(fromForLetter("#", false)).toEqual({});
    expect(fromForLetter("M", true)).toEqual({ from: "N" });
    expect(fromForLetter("Z", true)).toEqual({});
    expect(fromForLetter("#", true)).toEqual({ from: "A" });
  });

  it("lists Films Z to A when chosen, its letters Z to A, and a jump asks Z to A from the next letter", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items": films([firstPage, lastPage, page([film(5, "Mabel")], "")]),
        "GET /items/letters": [200, { letters: ["#", "B", "M"] }],
        "GET /genres": [200, { genres: [] }],
      },
      heard,
    );
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedBrowse kind="film" />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".alphabet-rail") !== null);
    const popUp = await openCriteria(mounted.container);
    const sortChooser = [...popUp.querySelectorAll(".route-chooser")].find(
      (each) => each.querySelector(".route-chooser__field")?.textContent === "Sort",
    );
    await click(sortChooser?.querySelector("button[aria-haspopup]"));
    const options = [...(sortChooser?.querySelectorAll("dialog li button") ?? [])].map((each) =>
      each.getAttribute("aria-label"),
    );
    expect(options).toEqual([
      "Name, A–Z",
      "Name, Z–A",
      "Oldest released",
      "Newest released",
      "Oldest added",
      "Newest added",
    ]);
    await click(buttonNamed(mounted.container, "Name, Z–A"));
    await closeCriteria(mounted.container);
    await settleUntil(() => listsHeard(heard).length >= 2);
    expect(new URLSearchParams(listsHeard(heard)[1]?.query).get("order")).toBe("descending");
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const shown = [...mounted.container.querySelectorAll(".alphabet-rail__letter")].map(
      (letter) => letter.textContent,
    );
    expect(shown).toEqual(["M", "B", "#"]);
    await click(railLetter(mounted.container, "M"));
    await settleUntil(() => listsHeard(heard).length >= 3);
    const jump = new URLSearchParams(listsHeard(heard)[2]?.query);
    expect(jump.get("from")).toBe("N");
    expect(jump.get("order")).toBe("descending");
  });
});
