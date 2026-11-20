// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { ChannelClosedError } from "@orpheum/sdk";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppShell } from "./app-shell.tsx";
import { Home } from "./home.tsx";
import { LibrariesReachedContext } from "./libraries-reached.ts";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import { click, mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { testServices, withServices } from "./test-services.tsx";
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
              {
                id: 7,
                name: "Metropolis",
                libraryId: 1,
                fileId: 70,
                positionMs: 250,
                durationMs: 1000,
              },
            ],
            newestFilms: [{ ...entry(7, "Metropolis"), watchedPercent: 25 }, entry(8, "Nosferatu")],
            newestSeries: [{ ...entry(9, "Test Show"), watchedPercent: 0 }],
          },
        ],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "film" }] }],
      },
      [],
      "token",
    );
    mounted = await mount(withServices(<Home />, services));
    const headings = [...mounted.container.querySelectorAll("h2")].map((each) => each.textContent);
    expect(headings).toEqual(["Continue watching", "New films", "New series"]);
    const newFilms = mounted.container.querySelector('[aria-labelledby="home-newest-film"]');
    const cards = newFilms?.querySelectorAll(".poster-card") ?? [];
    expect(cards).toHaveLength(2);
    // A film partway shows how far in, in every list, as the server says it.
    expect(cards[0]?.querySelector("progress")?.getAttribute("value")).toBe("25");
    expect(cards[1]?.querySelector("progress")).toBeNull();
  });

  /** Home's read with one film partway and one series' next episode, 23, on file 4. */
  const withNextUp = {
    continueWatching: [
      { id: 7, name: "Metropolis", libraryId: 1, fileId: 70, positionMs: 250, durationMs: 1000 },
    ],
    nextUp: [
      {
        id: 23,
        kind: "episode",
        name: "Pilot",
        fileId: 4,
        positionMs: 0,
        durationMs: 0,
        episode: { seriesId: 9, seriesName: "Test Show", seasonNumber: 1, episodeNumber: 3 },
      },
    ],
    newestFilms: [entry(8, "Nosferatu")],
    newestSeries: [entry(9, "Test Show")],
  };

  it("shows Next up second, each card playing its episode at once when chosen (plans/b1.md §A)", async () => {
    const services = servedBy(
      {
        "GET /home": [200, withNextUp],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "film" }] }],
        "POST /playback": [200, directDecision()],
      },
      [],
      "token",
    );
    mounted = await mount(withServices(<Home />, services));
    const { container } = mounted;
    const headings = [...container.querySelectorAll("h2")].map((each) => each.textContent);
    expect(headings).toEqual(["Continue watching", "Next up", "New films", "New series"]);
    const nextUp = container.querySelector('[aria-labelledby="next-up-heading"]');
    const card = nextUp?.querySelector(".poster-card");
    // Named by its series, its place and its own name.
    expect(card?.textContent).toMatch(/Test Show · .+ · Pilot/u);
    // It plays from the start: no part of it is watched.
    expect(card?.querySelector("progress")).toBeNull();
    await click(nextUp?.querySelector('a[href="/play/4"]') ?? null);
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 4 });
  });

  it("hides a Next up card with PUT, says until when, and undoes it with DELETE (D188)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /home": [200, withNextUp],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "film" }] }],
        "PUT /items/23/hidden": [204, undefined],
        "DELETE /items/23/hidden": [204, undefined],
      },
      heard,
      "token",
    );
    mounted = await mount(withServices(<Home />, services));
    const { container } = mounted;
    const nextUp = container.querySelector('[aria-labelledby="next-up-heading"]');
    await click(nextUp?.querySelector(".continue-watching__more") ?? null);
    const hide = [...document.querySelectorAll('dialog .list-box [role="button"]')].find(
      (each) => each.textContent === "Hide",
    );
    await click(hide ?? null);
    await settleUntil(() => container.querySelector(".continue-watching__hidden") !== null);
    expect(heard.map((each) => each.methodAndPath)).toContain("PUT /items/23/hidden");
    const hidden = container.querySelector(".continue-watching__hidden");
    expect(hidden?.textContent).toContain("Pilot is hidden until you next watch its series.");
    await click(hidden?.querySelector("button") ?? null);
    await settleUntil(() => heard.some((each) => each.methodAndPath === "DELETE /items/23/hidden"));
  });

  it("asks the server for the libraries once, the banner's and Home's reading shared (the web client's architecture audit of 2026-11-10, Startup 10)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /home": [200, { continueWatching: [] }],
        "GET /libraries": [200, { libraries: [] }],
      },
      heard,
      "token",
    );
    mounted = await mount(
      withServices(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={false}
          currentScreen="home"
          onSignOut={() => undefined}
          signedInAs={undefined}
        >
          <Home />
        </AppShell>,
        services,
      ),
    );
    const { container } = mounted;
    await settleUntil(() => container.querySelector(".home__empty") !== null);
    expect(heard.filter((each) => each.methodAndPath === "GET /libraries")).toHaveLength(1);
  });

  it("warms a Continue watching tile's film up as it is pressed, not once the Play page draws (the web client's architecture audit of 2026-11-10, Playback 1)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /home": [
          200,
          {
            continueWatching: [
              {
                id: 7,
                name: "Metropolis",
                libraryId: 1,
                fileId: 70,
                positionMs: 250,
                durationMs: 1000,
              },
            ],
          },
        ],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "film" }] }],
        "POST /playback": [200, directDecision()],
      },
      heard,
      "token",
    );
    mounted = await mount(withServices(<Home />, services));
    const { container } = mounted;
    const tile = container.querySelector('.continue-watching a[href="/play/70"]');
    expect(tile).not.toBeNull();
    expect(heard.some((each) => each.methodAndPath === "POST /playback")).toBe(false);
    await click(tile);
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 70 });
    // No Play page is drawn here: the press itself asked how the film plays.
    await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /playback"));
    expect(heard.filter((each) => each.methodAndPath === "POST /playback")).toHaveLength(1);
  });

  it("reads every row in one read of the libraries the person keeps on Home, and polls it once (D366 §5, D476)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /home": [
          200,
          { continueWatching: [], newestFilms: [entry(8, "Nosferatu")], newestSeries: [] },
        ],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "film" }] }],
      },
      heard,
      "token",
    );
    // The polls running, the libraries' the shell's to keep (LibrariesReachedContext).
    let polled = 0;
    const counting: Poller = {
      start: () => {
        polled += 1;
        return () => {
          polled -= 1;
        };
      },
    };
    mounted = await mount(
      withServices(
        <BackgroundChangesContext value={counting}>
          <LibrariesReachedContext
            value={{ libraries: { state: "loaded", value: [] }, readAgain: () => undefined }}
          >
            <Home />
          </LibrariesReachedContext>
        </BackgroundChangesContext>,
        services,
      ),
    );
    expect(
      mounted.container.querySelectorAll('[aria-labelledby="home-newest-film"] .poster-card'),
    ).toHaveLength(1);
    // Every row in one read, which the server keeps to Home's libraries (D366 §5), and one poll.
    expect(heard.filter((each) => each.methodAndPath === "GET /home")).toHaveLength(1);
    expect(heard.filter((each) => each.methodAndPath === "GET /items")).toEqual([]);
    expect(polled).toBe(1);
  });

  it("fills its rows by itself once the connection to the core is back, its read cut off before (the owner's phone, 2026-11-15)", async () => {
    // A reload over a link that kept dropping drew the heading alone, and it stayed so until the
    // owner reloaded by hand: the owner chose that Home asks again when the connection is back.
    const written = vi.spyOn(console, "error").mockImplementation(() => undefined);
    let isLinkDown = true;
    const send = (request: Request): Promise<Response> => {
      if (isLinkDown) {
        return Promise.reject(new ChannelClosedError(new URL(request.url).pathname, "closed"));
      }
      const rows = { continueWatching: [], newestFilms: [entry(8, "Nosferatu")], newestSeries: [] };
      return Promise.resolve(Response.json(rows));
    };
    const backListeners = new Set<() => void>();
    const services = {
      ...testServices("token", "/", send),
      onPathBack: (listener: () => void) => {
        backListeners.add(listener);
        return () => {
          backListeners.delete(listener);
        };
      },
    };
    // A poll not due before the connection is back, as one waiting out its give-up is.
    const notDue: Poller = { start: () => () => undefined };
    mounted = await mount(
      withServices(
        <BackgroundChangesContext value={notDue}>
          <LibrariesReachedContext
            value={{ libraries: { state: "loaded", value: [] }, readAgain: () => undefined }}
          >
            <Home />
          </LibrariesReachedContext>
        </BackgroundChangesContext>,
        services,
      ),
    );
    const posters = (): number =>
      mounted?.container.querySelectorAll('[aria-labelledby="home-newest-film"] .poster-card')
        .length ?? 0;
    expect(posters()).toBe(0);
    isLinkDown = false;
    await act(async () => {
      for (const listener of backListeners) {
        listener();
      }
      await Promise.resolve();
    });
    await settleUntil(() => posters() === 1);
    expect(posters()).toBe(1);
    written.mockRestore();
  });

  it("leads each newest row's See all to its list, newest added first (the owner, 2026-11-04)", async () => {
    const services = servedBy(
      {
        "GET /home": [
          200,
          {
            continueWatching: [
              {
                id: 7,
                name: "Metropolis",
                libraryId: 1,
                fileId: 70,
                positionMs: 250,
                durationMs: 1000,
              },
            ],
            newestFilms: [entry(8, "Nosferatu")],
            newestSeries: [entry(9, "Test Show")],
          },
        ],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "film" }] }],
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
      "home-newest-film",
      "home-newest-series",
    ]);
  });

  it("leaves out a row with nothing in it", async () => {
    const services = servedBy(
      {
        "GET /home": [200, { continueWatching: [] }],
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "film" }] }],
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
        "GET /libraries": [200, { libraries: [{ id: 1, name: "Films", kind: "film" }] }],
      },
      [],
      "token",
    );
    mounted = await mount(withServices(<Home />, one));
    expect(mounted.container.querySelector(".home__empty")).toBeNull();
  });
});
