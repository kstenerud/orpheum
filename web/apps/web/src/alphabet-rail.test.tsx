// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { fromForLetter, railLetters } from "./alphabet-rail.tsx";
import { Browse, type Entry } from "./browse.tsx";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import {
  buttonWithText,
  choose,
  click,
  mount,
  servedBy,
  settleUntil,
  type Answer,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A poller that never asks: the list is read only as the test's steps ask. */
const neverAsks: Poller = { start: () => () => undefined };

/** A film called name, by its id. */
function film(id: number, name: string): Entry {
  return { id, name, kind: "movie", identificationState: "identified", addedAt: id };
}

/** A page of the films list, with the cursor of the next, "" on the last. */
function page(entries: readonly Entry[], nextCursor: string): Answer {
  return [200, { titles: entries, nextCursor }];
}

const firstPage = page([film(1, "12 Marrow Larks"), film(2, "Wing")], "c1");
const secondPage = page([film(3, "Greed"), film(4, "Metropolis")], "c2");
const lastPage = page([film(5, "Keepsake Hollow"), film(6, "Zelkaro")], "");

/** The films' list, by name, with the rail beside it, answered by answers in turn. */
async function mountFilms(answers: readonly Answer[], heard: Heard[]): Promise<Mounted> {
  const services = servedBy(
    { "GET /titles": answers, "GET /titles/letters": [200, { letters: railLetters }] },
    heard,
  );
  return await mount(
    <BackgroundChangesContext value={neverAsks}>
      {withServices(<Browse kind="movie" />, services)}
    </BackgroundChangesContext>,
  );
}

/** The name of the poster that has focus, or undefined when none has. */
function focusedName(): string | undefined {
  return document.activeElement?.querySelector(".poster-card__name")?.textContent ?? undefined;
}

/** The requests heard against GET /titles alone, in order: the rail's own letters are a sibling
 * request (alphabet-rail.tsx's useAlphabetRailLetters) these tests do not count by position. */
function titlesHeard(heard: readonly Heard[]): Heard[] {
  return heard.filter((each) => each.methodAndPath === "GET /titles");
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
      [firstPage, page([film(5, "Keepsake Hollow")], ""), secondPage, lastPage],
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await click(buttonWithText(mounted.container, "M"));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("Keepsake Hollow");
    const queries = titlesHeard(heard).map((each) => new URLSearchParams(each.query));
    expect(queries[1]?.get("from")).toBe("M");
    expect(queries[1]?.get("sort")).toBe("name");
    expect(queries[1]?.get("limit")).toBe("1");
    expect(queries.slice(2).map((query) => query.get("cursor"))).toEqual(["c1", "c2"]);
    expect(mounted.container.querySelectorAll(".poster-grid > li")).toHaveLength(6);
  });

  it("moves to the first title of all for #, those beginning with a digit, loading nothing more", async () => {
    const heard: Heard[] = [];
    mounted = await mountFilms([firstPage, page([film(1, "12 Marrow Larks")], "x")], heard);
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await click(buttonWithText(mounted.container, "#"));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("12 Marrow Larks");
    const onlyTitles = titlesHeard(heard);
    expect(new URLSearchParams(onlyTitles[1]?.query).has("from")).toBe(false);
    expect(onlyTitles).toHaveLength(2);
  });

  it("moves to the last title for a letter no title comes at or after", async () => {
    const heard: Heard[] = [];
    mounted = await mountFilms([firstPage, page([], ""), secondPage, lastPage], heard);
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await click(buttonWithText(mounted.container, "Z"));
    await settleUntil(() => focusedName() !== undefined);
    expect(focusedName()).toBe("Zelkaro");
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
        "GET /titles": [firstPage],
        "GET /titles/letters": [200, { letters: ["#", "B", "M"] }],
      },
      heard,
    );
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<Browse kind="movie" />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".alphabet-rail") !== null);
    const shown = [...mounted.container.querySelectorAll(".alphabet-rail__letter")].map(
      (letter) => letter.textContent,
    );
    expect(shown).toEqual(["#", "B", "M"]);
    const letters = heard.find((each) => each.methodAndPath === "GET /titles/letters");
    expect(new URLSearchParams(letters?.query).get("kind")).toBe("movie");
    expect(new URLSearchParams(letters?.query).get("sort")).toBe("name");
  });

  it("shows no rail, and asks the server for no letters, sorted newest first", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      { "GET /titles": [firstPage, firstPage], "GET /titles/letters": [200, { letters: [] }] },
      heard,
    );
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<Browse kind="series" />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    expect(mounted.container.querySelector(".alphabet-rail")).not.toBeNull();
    const lettersAsksByName = heard.filter(
      (each) => each.methodAndPath === "GET /titles/letters",
    ).length;
    expect(lettersAsksByName).toBe(1);
    await choose(mounted.container.querySelector("select"), "added");
    await settleUntil(
      () => heard.filter((each) => each.methodAndPath === "GET /titles").length >= 2,
    );
    expect(mounted.container.querySelector(".alphabet-rail")).toBeNull();
    // The rail unmounts sorted by added: its letters are not asked for again.
    expect(heard.filter((each) => each.methodAndPath === "GET /titles/letters")).toHaveLength(
      lettersAsksByName,
    );
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
        "GET /titles": [firstPage, lastPage, page([film(5, "Keepsake Hollow")], "")],
        "GET /titles/letters": [200, { letters: ["#", "B", "M"] }],
      },
      heard,
    );
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<Browse kind="movie" />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".alphabet-rail") !== null);
    const options = [...mounted.container.querySelectorAll("option")].map(
      (each) => each.textContent,
    );
    expect(options).toEqual([
      "Name, A–Z",
      "Name, Z–A",
      "Newest released",
      "Oldest released",
      "Newest added",
      "Oldest added",
    ]);
    await choose(mounted.container.querySelector("select"), "name:descending");
    await settleUntil(() => titlesHeard(heard).length >= 2);
    expect(new URLSearchParams(titlesHeard(heard)[1]?.query).get("order")).toBe("descending");
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const shown = [...mounted.container.querySelectorAll(".alphabet-rail__letter")].map(
      (letter) => letter.textContent,
    );
    expect(shown).toEqual(["M", "B", "#"]);
    await click(buttonWithText(mounted.container, "M"));
    await settleUntil(() => titlesHeard(heard).length >= 3);
    const jump = new URLSearchParams(titlesHeard(heard)[2]?.query);
    expect(jump.get("from")).toBe("N");
    expect(jump.get("order")).toBe("descending");
  });
});
