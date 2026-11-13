// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useEffect, useState, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { lettersOf, placeOfLetter } from "./alphabet-rail.tsx";

import { Browse, type BrowseKind, type Entry } from "./browse.tsx";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import {
  buttonNamed,
  click,
  mount,
  servedBy,
  settleUntil,
  type Answer,
  type Heard,
  type Mounted,
  unanswered,
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

/** A film called name, by its id, under the rail letter its name begins with, # for a digit. */
function film(id: number, name: string): Entry {
  const first = name.charAt(0).toUpperCase();
  return {
    id,
    name,
    kind: "film",
    identificationState: "identified",
    addedAt: id,
    letter: first >= "A" && first <= "Z" ? first : "#",
  };
}

/** The films list, whole in one answer (D475). */
function page(entries: readonly Entry[]): Answer {
  return [200, { items: entries, total: entries.length }];
}

const sixFilms = [
  film(1, "12 Patient Sailors"),
  film(2, "Arkon"),
  film(3, "Brindlewood"),
  film(4, "Hearthstead"),
  film(5, "Mabel"),
  film(6, "Zelkaro"),
];
const wholeList = page(sixFilms);

/** The films' list, by name, with the rail beside it, answered by answers. */
async function mountFilms(answers: Answer | readonly Answer[], heard: Heard[]): Promise<Mounted> {
  const services = servedBy({ "GET /items": answers, "GET /genres": [200, { genres: [] }] }, heard);
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

/** The requests heard against GET /items alone, in order. */
function titlesHeard(heard: readonly Heard[]): Heard[] {
  return heard.filter((each) => each.methodAndPath === "GET /items");
}

/** The rail's letters shown, in order. */
function railShown(container: HTMLElement): (string | null)[] {
  return [...container.querySelectorAll(".alphabet-rail__letter")].map(
    (letter) => letter.textContent,
  );
}

describe("the alphabet rail (the owner, 2026-10-25)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("moves the list to a letter's first title among those it holds, asking nothing, and focuses it", async () => {
    const heard: Heard[] = [];
    mounted = await mountFilms(wholeList, heard);
    await settleUntil(() => railLetter(mounted?.container ?? document.body, "M") !== undefined);
    const asked = heard.length;
    await click(railLetter(mounted.container, "M"));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("Mabel");
    expect(heard).toHaveLength(asked);
  });

  it("lands on the poster a letter leads to deep in a long list, drawn through it at once", async () => {
    const heard: Heard[] = [];
    const long = Array.from({ length: 300 }, (_, index) =>
      film(index + 1, `${index < 250 ? "Arkon" : "Mabel"} ${String(index + 1).padStart(3, "0")}`),
    );
    mounted = await mountFilms(page(long), heard);
    await settleUntil(() => railLetter(mounted?.container ?? document.body, "M") !== undefined);
    const asked = heard.length;
    await click(railLetter(mounted.container, "M"));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("Mabel 251");
    expect(heard).toHaveLength(asked);
  });

  it("moves to the first title of all for #, those beginning with a digit", async () => {
    const heard: Heard[] = [];
    mounted = await mountFilms(wholeList, heard);
    await settleUntil(() => railLetter(mounted?.container ?? document.body, "#") !== undefined);
    await click(railLetter(mounted.container, "#"));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("12 Patient Sailors");
    expect(titlesHeard(heard)).toHaveLength(1);
  });

  it("finds a letter's place by the letter each entry is under, the last where none is", () => {
    const entries = sixFilms;
    expect(placeOfLetter(entries, "M", false)).toBe(4);
    expect(placeOfLetter(entries, "C", false)).toBe(3);
    expect(placeOfLetter(entries, "#", false)).toBe(0);
    const zToA = [...entries].reverse();
    expect(placeOfLetter(zToA, "M", true)).toBe(1);
    expect(placeOfLetter(zToA, "Y", true)).toBe(1);
    expect(placeOfLetter(zToA, "Z", true)).toBe(0);
    expect(placeOfLetter(zToA, "#", true)).toBe(5);
    expect(placeOfLetter(entries.slice(0, 4), "M", false)).toBe(-1);
  });

  // Silk on a Fire TV lays the page out each time a key focuses a <button>: a move onto the rail
  // took 300 ms, the first 5 s (the 11-24 batch's item 12; idiosyncrasies.md).
  it("names its letters buttons by role, not <button>s, and Space presses one", async () => {
    const heard: Heard[] = [];
    mounted = await mountFilms(wholeList, heard);
    await settleUntil(() => railLetter(mounted?.container ?? document.body, "M") !== undefined);
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
// waits on a page; even a thousand titles is light (D475).
describe("Films' whole list, read in one request (D475)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("reads the whole list in one request, with no limit, and draws every entry", async () => {
    const heard: Heard[] = [];
    const long = Array.from({ length: 150 }, (_, index) =>
      film(index + 1, `Arkon ${String(index)}`),
    );
    mounted = await mountFilms(page(long), heard);
    await settleUntil(() => mounted?.container.querySelector('a[href="/titles/150"]') !== null);
    expect(titlesHeard(heard)).toHaveLength(1);
    const asked = new URLSearchParams(titlesHeard(heard)[0]?.query);
    expect([asked.has("limit"), asked.has("cursor")]).toEqual([false, false]);
    expect(mounted.container.querySelectorAll(".poster-grid > li")).toHaveLength(150);
  });

  it("is drawn from the tab's memory at once on Back, while it is read again", async () => {
    let showList: ((isShown: boolean) => void) | undefined;
    function BackAndForth(): JSX.Element {
      const [isShown, setIsShown] = useState(true);
      useEffect(() => {
        showList = setIsShown;
      }, []);
      return isShown ? <RoutedBrowse kind="film" /> : <p>A film's page</p>;
    }
    const heard: Heard[] = [];
    const services = servedBy(
      { "GET /items": [wholeList, unanswered], "GET /genres": [200, { genres: [] }] },
      heard,
    );
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<BackAndForth />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector('a[href="/titles/6"]') !== null);
    act(() => {
      showList?.(false);
    });
    expect(mounted.container.querySelector(".poster-grid")).toBeNull();
    act(() => {
      showList?.(true);
    });
    // Drawn before the second read answers, which it never does.
    expect(mounted.container.querySelector('a[href="/titles/6"]')).not.toBeNull();
    await settleUntil(() => titlesHeard(heard).length >= 2);
    expect(titlesHeard(heard)).toHaveLength(2);
    await settleUntil(() => railLetter(mounted?.container ?? document.body, "M") !== undefined);
    expect(railShown(mounted.container)).toEqual(["#", "A", "B", "H", "M", "Z"]);
  });
});

describe("Films' and Series' rail shows only letters that lead somewhere (D349 §9)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows only the letters the list's entries are under, sorted by name, asking nothing more", async () => {
    const heard: Heard[] = [];
    mounted = await mountFilms(
      page([film(1, "12 Patient Sailors"), film(3, "Brindlewood"), film(5, "Mabel")]),
      heard,
    );
    await settleUntil(() => railLetter(mounted?.container ?? document.body, "M") !== undefined);
    expect(railShown(mounted.container)).toEqual(["#", "B", "M"]);
    expect(heard.filter((each) => each.methodAndPath.startsWith("GET /items/"))).toEqual([]);
    expect(lettersOf(undefined)).toBeUndefined();
  });

  it("shows no rail sorted newest first", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      { "GET /items": [wholeList, wholeList], "GET /genres": [200, { genres: [] }] },
      heard,
    );
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedBrowse kind="series" />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    expect(mounted.container.querySelector(".alphabet-rail")).not.toBeNull();
    await chooseSort(mounted.container, "Newest added");
    await settleUntil(() => titlesHeard(heard).length >= 2);
    expect(mounted.container.querySelector(".alphabet-rail")).toBeNull();
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
        "GET /items": [wholeList, page([film(4, "Hearthstead")])],
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
    await settleUntil(() => titlesHeard(heard).length >= 2);
    const searched = new URLSearchParams(titlesHeard(heard)[1]?.query);
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
      "GET /items": [wholeList, page([film(4, "Hearthstead")]), page([film(4, "Hearthstead")])],
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
        "GET /items": [page([film(4, "Hearthstead")])],
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

  it("lists Films Z to A when chosen, its letters Z to A, and a jump finds its letter Z to A", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items": [wholeList, page([...sixFilms].reverse())],
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
    await click(sortChooser?.querySelector("[aria-haspopup]"));
    const options = [...(sortChooser?.querySelectorAll('dialog li [role="button"]') ?? [])].map(
      (each) => each.getAttribute("aria-label"),
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
    await settleUntil(() => titlesHeard(heard).length >= 2);
    expect(new URLSearchParams(titlesHeard(heard)[1]?.query).get("order")).toBe("descending");
    await settleUntil(() => railShown(mounted?.container ?? document.body)[0] === "Z");
    expect(railShown(mounted.container)).toEqual(["Z", "M", "H", "B", "A", "#"]);
    await click(railLetter(mounted.container, "M"));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("Mabel");
    expect(titlesHeard(heard)).toHaveLength(2);
  });
});
