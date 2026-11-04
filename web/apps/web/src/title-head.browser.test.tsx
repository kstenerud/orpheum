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
    titleId: 302,
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
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /collections/20": series }, "/series/20"),
    );
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
