// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { BackgroundChangesContext } from "./polling.ts";
import { mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { chooseSort, narrowByName, openCriteria } from "./test-list-criteria.tsx";
import {
  neverAsks,
  library,
  titlesPage,
  lettersPage,
  mountBrowse,
  RoutedLibraryBrowse,
} from "./test-library-browse.tsx";

describe("a library's sort, kept on the person's account (X5)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  // The owner, 2026-11-12 (X5): "When I choose a sort order in a library, it should save that
  // ordering to my account."
  it("keeps a sort chosen in a library among the person's preferences, and reads the libraries again", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /libraries": [
          200,
          { libraries: [library(1, "Films"), library(2, "Series", "series")] },
        ],
        "PUT /libraries/1/preferences": [204, null],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
        "GET /genres": [200, { genres: [] }],
      },
      heard,
    );
    services.navigation.replace({ screen: "browse", library: 1 });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedLibraryBrowse />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const librariesRead = (): number =>
      heard.filter((each) => each.methodAndPath === "GET /libraries").length;
    const readBefore = librariesRead();
    await narrowByName(mounted.container, "metro");
    // A search is no sort: nothing is kept.
    expect(heard.filter((each) => each.methodAndPath.startsWith("PUT "))).toEqual([]);
    await chooseSort(mounted.container, "Oldest released");
    await settleUntil(() => librariesRead() > readBefore);
    const kept = heard.filter((each) => each.methodAndPath.startsWith("PUT "));
    expect(kept.map((each) => [each.methodAndPath, each.body])).toEqual([
      ["PUT /libraries/1/preferences", { sortChoice: { sort: "released", order: "ascending" } }],
    ]);
    // The address names it too, so Back and a reload show it as chosen.
    expect(services.navigation.route()).toMatchObject({
      screen: "browse",
      library: 1,
      sort: "released",
      order: "ascending",
    });
  });

  it("keeps no sort for All libraries, which is no one library", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: "all" },
      {
        "GET /libraries": [200, { libraries: [library(1, "Films")] }],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    await chooseSort(mounted.container, "Newest added");
    expect(heard.filter((each) => each.methodAndPath.startsWith("PUT "))).toEqual([]);
  });

  it("opens a library its address names no sort for sorted as the person chose it, on any device (X5)", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [
          200,
          {
            libraries: [
              { ...library(1, "Films"), sortChoice: { sort: "released", order: "ascending" } },
            ],
          },
        ],
        "GET /items": titlesPage(),
      },
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const listed = heard.find((each) => each.methodAndPath === "GET /items");
    expect(new URLSearchParams(listed?.query).get("sort")).toBe("released");
    expect(new URLSearchParams(listed?.query).get("order")).toBe("ascending");
    const popUp = await openCriteria(mounted.container);
    expect(popUp.textContent).toContain("Oldest released");
  });

  it("opens a library sorted as its address names, over the sort the person kept (X5)", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1, sort: "added" },
      {
        "GET /libraries": [
          200,
          {
            libraries: [
              { ...library(1, "Films"), sortChoice: { sort: "released", order: "ascending" } },
            ],
          },
        ],
        "GET /items": titlesPage(),
      },
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const listed = heard.find((each) => each.methodAndPath === "GET /items");
    expect(new URLSearchParams(listed?.query).get("sort")).toBe("added");
    // Its own way, newest first, is asked as it always was: no order named.
    expect(new URLSearchParams(listed?.query).get("order")).toBeNull();
  });

  it("opens a library at its kind's default where the sort kept is one its kind no longer offers", async () => {
    const heard: Heard[] = [];
    mounted = await mountBrowse(
      { library: 1 },
      {
        "GET /libraries": [
          200,
          {
            libraries: [
              { ...library(1, "Films"), sortChoice: { sort: "recorded", order: "ascending" } },
            ],
          },
        ],
        "GET /items": titlesPage(),
        "GET /items/letters": lettersPage(),
      },
      heard,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-grid") !== null);
    const listed = heard.find((each) => each.methodAndPath === "GET /items");
    expect(new URLSearchParams(listed?.query).get("sort")).toBe("name");
  });
});
