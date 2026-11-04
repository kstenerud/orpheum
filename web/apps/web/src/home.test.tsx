// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Home } from "./home.tsx";
import { mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

/** A film or series as a list sends it. */
function entry(id: number, name: string): Record<string, unknown> {
  return { id, name, identificationState: "identified", libraryId: 1, addedAt: id };
}

describe("home (the owner, 2026-10-25)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows what is partway, then the newest films and series, every poster alike", async () => {
    const services = servedBy(
      {
        "GET /home": [
          200,
          {
            continueWatching: [
              { id: 7, name: "Metropolis", libraryId: 1, fileId: 70, positionMs: 250, durationMs: 1000 },
            ],
          },
        ],
        "GET /titles": [
          200,
          {
            titles: [{ ...entry(7, "Metropolis"), watchedPercent: 25 }, entry(8, "Nosferatu")],
            nextCursor: "",
          },
        ],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "movie" }] }],
      },
      [],
      "token",
    );
    mounted = await mount(withServices(<Home />, services));
    const headings = [...mounted.container.querySelectorAll("h2")].map((each) => each.textContent);
    expect(headings).toEqual(["Continue watching", "New films", "New series"]);
    const newFilms = mounted.container.querySelector('[aria-labelledby="home-newest-movie"]');
    const cards = newFilms?.querySelectorAll(".poster-card") ?? [];
    expect(cards).toHaveLength(2);
    // A film partway shows how far in, in every list, as the server says it.
    expect(cards[0]?.querySelector("progress")?.getAttribute("value")).toBe("25");
    expect(cards[1]?.querySelector("progress")).toBeNull();
  });

  it("asks its newest rows of the libraries the person keeps on Home (D366 §5)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /home": [200, { continueWatching: [] }],
        "GET /titles": [200, { titles: [entry(8, "Nosferatu")], nextCursor: "" }],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "movie" }] }],
      },
      heard,
      "token",
    );
    mounted = await mount(withServices(<Home />, services));
    const rows = heard.filter((each) => each.methodAndPath === "GET /titles");
    expect(rows.map((each) => new URLSearchParams(each.query).get("home"))).toEqual([
      "true",
      "true",
    ]);
  });

  it("leads each newest row's See all to its list, newest added first (the owner, 2026-11-04)", async () => {
    const services = servedBy(
      {
        "GET /home": [
          200,
          {
            continueWatching: [
              { id: 7, name: "Metropolis", libraryId: 1, fileId: 70, positionMs: 250, durationMs: 1000 },
            ],
          },
        ],
        "GET /titles": [200, { titles: [entry(8, "Nosferatu")], nextCursor: "" }],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "movie" }] }],
      },
      [],
      "token",
    );
    mounted = await mount(withServices(<Home />, services));
    const seeAll = [...mounted.container.querySelectorAll("a")].filter(
      (link) => link.textContent === "See all",
    );
    expect(
      seeAll.map((link) => [link.getAttribute("aria-label"), link.getAttribute("href")]),
    ).toEqual([
      ["See all films, newest first", "/films?sort=added"],
      ["See all series, newest first", "/series?sort=added"],
    ]);
    // Each in its own row's heading, none in Continue Watching's.
    expect(seeAll.map((link) => link.closest("section")?.getAttribute("aria-labelledby"))).toEqual([
      "home-newest-movie",
      "home-newest-series",
    ]);
  });

  it("leaves out a row with nothing in it", async () => {
    const services = servedBy(
      {
        "GET /home": [200, { continueWatching: [] }],
        "GET /titles": [200, { titles: [], nextCursor: "" }],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "movie" }] }],
      },
      [],
      "token",
    );
    mounted = await mount(withServices(<Home />, services));
    expect(mounted.container.querySelectorAll("h2")).toHaveLength(0);
    expect(mounted.container.querySelector("h1")?.textContent).toBe("Home");
  });

  it("leads an admin to make a library where there is none (staging, 2026-11-01)", async () => {
    const services = servedBy(
      {
        "GET /home": [200, { continueWatching: [] }],
        "GET /titles": [200, { titles: [], nextCursor: "" }],
        "GET /libraries": [200, { libraries: [] }],
      },
      [],
      "token",
    );
    mounted = await mount(
      withServices(
        <IsAdminContext value={true}>
          <Home />
        </IsAdminContext>,
        services,
      ),
    );
    const link = mounted.container.querySelector<HTMLAnchorElement>(".home__empty a");
    expect(link?.getAttribute("href")).toBe("/libraries/new");
  });

  it("tells anyone else with no library to ask, and says nothing once there is one", async () => {
    const empty = servedBy(
      {
        "GET /home": [200, { continueWatching: [] }],
        "GET /titles": [200, { titles: [], nextCursor: "" }],
        "GET /libraries": [200, { libraries: [] }],
      },
      [],
      "token",
    );
    mounted = await mount(withServices(<Home />, empty));
    expect(mounted.container.querySelector(".home__empty")?.textContent).toContain("Ask whoever");
    expect(mounted.container.querySelector(".home__empty a")).toBeNull();
    mounted.unmount();
    const one = servedBy(
      {
        "GET /home": [200, { continueWatching: [] }],
        "GET /titles": [200, { titles: [], nextCursor: "" }],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "movie" }] }],
      },
      [],
      "token",
    );
    mounted = await mount(withServices(<Home />, one));
    expect(mounted.container.querySelector(".home__empty")).toBeNull();
  });
});
