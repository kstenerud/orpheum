// SPDX-License-Identifier: MPL-2.0

/**
 * A title's first screen, the title template (title-head.tsx; ui-audit-2026-11-04.md §3, R8), in a
 * real browser as the app lays it out: its main action, the page's arrival, is on the first screen
 * of a laptop's window and a phone's, whatever the length of what it is about, and that is clamped
 * with More.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import {
  filmDetail,
  focusedName,
  isDrawn,
  isFocusIn,
  mountApp,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
} from "./test-walk.tsx";
import { routeHref } from "./router.ts";

/** A synopsis long enough to run past its clamp on any screen. */
const longOverview = Array.from(
  { length: 24 },
  (_, index) => `In part ${String(index + 1)} the crew plans one more job across the city.`,
).join(" ");

const signedIn: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": { continueWatching: [] },
};

/** A film with a tagline, a long synopsis and a cast, all of which once sat above its Play. */
const film = {
  ...filmDetail(103, "Metropolis"),
  tagline: "A Los Angeles crime saga",
  overview: longOverview,
  credits: {
    cast: Array.from({ length: 8 }, (_, index) => ({
      personId: index + 1,
      name: `Actor ${String(index + 1)}`,
      character: `Role ${String(index + 1)}`,
    })),
    directors: null,
    writers: null,
  },
};

/** An episode of a series, its place above its name. */
const episode = {
  ...filmDetail(230, "Low Water"),
  kind: "episode",
  overview: longOverview,
  extras: null,
  episode: { seriesId: 21, seriesName: "Glass Tide", seasonNumber: 2, episodeNumber: 3 },
};

/** A series, partway through its next episode, with a long synopsis and a backdrop. */
const series = {
  id: 20,
  kind: "series",
  name: "Glass Tide",
  libraryId: 1,
  year: 2019,
  overview: longOverview,
  art: { posterSha256: "p20", backdropSha256: "b20" },
  orderings: [{ name: "aired", kind: "aired", episodeCount: 4 }],
  seasons: [
    { seasonNumber: 1, episodeCount: 2 },
    { seasonNumber: 2, episodeCount: 2 },
  ],
  nextUp: {
    itemId: 302,
    name: "Low Water",
    groupNumber: 1,
    numberInGroup: 2,
    fileId: 3020,
    isPartway: true,
  },
};

/** A laptop's window, and a phone's, held upright. */
const screens = [
  { name: "a laptop", width: 1366, height: 768 },
  { name: "a phone", width: 390, height: 700 },
] as const;

/** The page's main action, the actions row's first. */
function mainAction(): HTMLElement | null {
  return document.querySelector<HTMLElement>(".title-head__actions > :first-child");
}

describe.each(screens)("a title's first screen, on $name", ({ width, height }) => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  it.each([
    { kind: "film", path: "/titles/103", answers: { "GET /items/103": film } },
    { kind: "episode", path: "/titles/230", answers: { "GET /items/230": episode } },
  ])("has a $kind's Play within the window, focused as it opens", async ({ path, answers }) => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices({ ...signedIn, ...answers }, path));
    await expect.poll(() => isFocusIn(".title-head__actions")).toBe(true);
    expect(focusedName()).toBe("Play");
    const top = mainAction()?.getBoundingClientRect().top ?? Infinity;
    expect(top).toBeGreaterThanOrEqual(0);
    expect(top).toBeLessThan(window.innerHeight);
  });

  it("has a series' Continue within the window, focused as it opens", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices({ ...signedIn, "GET /items/20": series }, "/series/20"));
    await expect.poll(() => isFocusIn(".title-head__actions")).toBe(true);
    expect(focusedName()).toBe("Continue S1E2");
    const top = mainAction()?.getBoundingClientRect().top ?? Infinity;
    expect(top).toBeGreaterThanOrEqual(0);
    expect(top).toBeLessThan(window.innerHeight);
  });

  it("clamps what a film is about, and More shows the rest, Less hides it again", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices({ ...signedIn, "GET /items/103": film }, "/titles/103"));
    await expect.poll(() => isDrawn(".title-head__more")).toBe(true);
    const synopsis = document.querySelector<HTMLElement>("#title-synopsis");
    const more = document.querySelector<HTMLButtonElement>(".title-head__more");
    const clamped = synopsis?.clientHeight ?? 0;
    expect(synopsis?.scrollHeight).toBeGreaterThan(clamped);
    expect(more?.textContent).toBe("More");
    expect(more?.getAttribute("aria-expanded")).toBe("false");
    if (more !== null) {
      await userEvent.click(more);
    }
    await expect.poll(() => synopsis?.clientHeight ?? 0).toBeGreaterThan(clamped);
    expect(more?.textContent).toBe("Less");
    expect(more?.getAttribute("aria-expanded")).toBe("true");
  });
});

describe("a title's actions row, by a remote", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  // D418 §7: a title with nothing to play arrives on its "…", so an OK opens a menu and never
  // changes anything, rather than on Mark as watched, then the row's first.
  it("arrives on the … of a film that cannot be played now, not on Mark as watched", async () => {
    await page.viewport(1280, 800);
    const away = { ...film, id: 104, unavailable: { reason: "nodeAway", nodeName: "devnas" } };
    walking = await mountApp(
      walkServices(
        {
          ...signedIn,
          "GET /me": { ...viewer, role: "admin", isAdmin: true },
          "GET /items/104": away,
        },
        "/titles/104",
      ),
    );
    await expect.poll(() => isFocusIn(".title-head__actions")).toBe(true);
    expect(focusedName()).toBe("More actions");
    expect(document.querySelector(".title-head__actions .title__play")).toBeNull();
    expect(document.querySelector(".title-head__actions .watched-marking")).not.toBeNull();
  });

  it("arrives on the words saying why a person's film cannot play, where it has no …, and OK there changes nothing (D420)", async () => {
    await page.viewport(1280, 800);
    const away = { ...film, id: 105, unavailable: { reason: "nodeAway", nodeName: "devnas" } };
    const heard: string[] = [];
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /items/105": away }, "/titles/105", heard),
    );
    await expect.poll(() => document.activeElement?.className ?? "").toContain("title-head__notes");
    expect(document.querySelector(".title-head__overflow button")).toBeNull();
    expect(document.activeElement?.textContent).toContain("devnas");
    const asked = heard.length;
    await userEvent.keyboard("{Enter}");
    expect(routeHref(walking.services.navigation.route())).toBe("/titles/105");
    expect(heard.slice(asked).filter((request) => !request.startsWith("GET "))).toEqual([]);
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement?.closest(".watched-marking")).not.toBeNull();
  });

  it("arrives on the … of a series with nothing next, not on Mark as watched", async () => {
    await page.viewport(1280, 800);
    const watchedThrough = {
      ...series,
      id: 22,
      nextUp: undefined,
      orderings: [
        { name: "aired", kind: "aired", episodeCount: 4 },
        { name: "tmdb:dvd", kind: "dvd", label: "DVD Order", listedCount: 4, episodeCount: 4 },
      ],
    };
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /items/22": watchedThrough }, "/series/22"),
    );
    await expect.poll(() => isFocusIn(".title-head__actions")).toBe(true);
    expect(focusedName()).toBe("More actions");
    expect(document.querySelector(".title-head__actions .title__play")).toBeNull();
  });

  // The actions row is a row (title-head.tsx): Right at its end stays, as at a poster row's end
  // (Q3), rather than falling off its line to the season's Mark as watched half a screen down.
  it("keeps Right at the end of a series' actions row in the row", async () => {
    await page.viewport(960, 480);
    const twoOrders = {
      ...series,
      id: 23,
      orderings: [
        { name: "aired", kind: "aired", episodeCount: 4 },
        { name: "tmdb:dvd", kind: "dvd", label: "DVD Order", listedCount: 4, episodeCount: 4 },
      ],
    };
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /items/23": twoOrders }, "/series/23"),
    );
    await expect.poll(() => isDrawn(".series__season-head button")).toBe(true);
    const more = document.querySelector<HTMLElement>(".title-head__actions .more-actions__button");
    more?.focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(more);
    await userEvent.keyboard("{ArrowLeft}");
    expect(focusedName()).toBe("Mark all of Glass Tide as watched");
    expect(isFocusIn(".title-head__actions")).toBe(true);
  });

  // D418 §6: an episode's Next episode starts playing it.
  it("plays the next episode from an episode's Next episode", async () => {
    await page.viewport(1280, 800);
    walking = await mountApp(
      walkServices(
        {
          ...signedIn,
          "GET /items/230": episode,
          "GET /files/2300/up-next": {
            upNext: {
              ordering: "aired",
              episode: {
                fileId: 2310,
                itemId: 231,
                groupNumber: 2,
                numberInGroup: 4,
                isPartway: false,
              },
            },
          },
        },
        "/titles/230",
      ),
    );
    await expect.poll(() => isDrawn(".title-head__actions .title__action")).toBe(true);
    const next = [...document.querySelectorAll<HTMLElement>(".title-head__actions a")].find(
      (link) => link.textContent === "Next episode",
    );
    next?.focus();
    await userEvent.keyboard("{Enter}");
    await expect
      .poll(() => walking?.services.navigation.route())
      .toMatchObject({ screen: "play", fileId: 2310 });
  });
});

describe("a title's cast list under its backdrop", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  // The owner, 2026-11-05: "The top two rows cannot be clicked at all, and the third becomes
  // clickable only halfway down." The backdrop, taller than the head it sits behind, was drawn over
  // the cast that follows the head, and took its clicks.
  it("takes a click at the centre of every entry, the backdrop never over one", async () => {
    await page.viewport(1366, 768);
    const crowded = {
      ...film,
      art: { posterSha256: "p103", backdropSha256: "b103" },
      credits: {
        ...film.credits,
        cast: Array.from({ length: 40 }, (_, index) => ({
          personId: index + 1,
          name: `Actor ${String(index + 1)}`,
          character: `Role ${String(index + 1)}`,
        })),
      },
    };
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /items/103": crowded }, "/titles/103"),
    );
    await expect.poll(() => document.querySelectorAll(".credit").length).toBe(40);
    const entries = [...document.querySelectorAll<HTMLElement>(".credit")];
    const covered = entries
      .filter((entry) => {
        entry.scrollIntoView({ block: "center" });
        const box = entry.getBoundingClientRect();
        const hit = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
        return hit === null || !entry.contains(hit);
      })
      .map((entry) => entry.textContent);
    expect(covered).toEqual([]);
  });
});
