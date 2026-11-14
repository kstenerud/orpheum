// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { LibraryBrowse } from "./library-browse.tsx";
import { chipsOf } from "./list-filters.ts";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import { useRoute } from "./services.tsx";
import { mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { chooseInCriteria, closeCriteria } from "./test-list-criteria.tsx";
import { withServices } from "./test-services.tsx";

/** A poller that never asks: a list is read only as the test's steps ask. */
const neverAsks: Poller = { start: () => () => undefined };

/** Library 1, of kind, as the server sends it. */
function libraryOne(kind: "film" | "other"): Record<string, unknown> {
  return {
    id: 1,
    name: kind === "film" ? "Films" : "Videos",
    kind,
    metadataSource: kind === "other" ? "nfo" : "tmdb",
    changeDetection: "watch",
    isWritable: true,
    createdAt: 0,
  };
}

/** One film in library 1, so .poster-grid draws. */
const oneFilm = {
  id: 1,
  name: "A film",
  kind: "film",
  identificationState: "identified",
  libraryId: 1,
  addedAt: 1,
};

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

/** The query of each GET /items heard, in order. */
function itemsAsked(heard: readonly Heard[]): URLSearchParams[] {
  return heard
    .filter((each) => each.methodAndPath === "GET /items")
    .map((each) => new URLSearchParams(each.query));
}

describe("a library's genre chip (D480 §1)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("offers the genres its list's first page names for the library, reading none of its own (D480 §1)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /libraries": [200, { libraries: [libraryOne("film")] }],
        "GET /items": [
          200,
          {
            items: [oneFilm],
            nextCursor: "",
            genres: [{ id: 18, name: "Drama", titleCount: 1 }],
          },
        ],
      },
      heard,
    );
    services.navigation.replace({ screen: "browse", library: 1 });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedLibraryBrowse />, services)}
      </BackgroundChangesContext>,
    );
    const { container } = mounted;
    await settleUntil(() => container.querySelector(".poster-grid") !== null);
    const asked = itemsAsked(heard)[0];
    expect([asked?.get("library"), asked?.get("genres")]).toEqual(["1", "true"]);
    await chooseInCriteria(container, "Genre", "Drama");
    await closeCriteria(container);

    expect(services.navigation.route()).toMatchObject({ screen: "browse", library: 1, genre: 18 });
    expect(heard.filter((each) => each.methodAndPath === "GET /genres")).toHaveLength(0);
  });

  it("asks an other library's list for no genres, as it has no chips", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /libraries": [200, { libraries: [libraryOne("other")] }],
        "GET /items": [200, { items: [oneFilm], nextCursor: "" }],
      },
      heard,
    );
    // Searched, as its own page is its folders.
    services.navigation.replace({ screen: "browse", library: 1, query: "film" });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(<RoutedLibraryBrowse />, services)}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => itemsAsked(heard).length > 0);
    expect(itemsAsked(heard).map((each) => each.has("genres"))).toEqual(
      itemsAsked(heard).map(() => false),
    );
  });
});
