// SPDX-License-Identifier: MPL-2.0

/**
 * A row's and a grid's memory, walked with a remote's keys in a real browser at a Fire TV's page in
 * Silk and a desktop's window (controller-input-2026-10-25.md §6.1 item 2): an arrow coming back into
 * Continue Watching or the films' grid goes to what last had focus there, as a TV app's rows do, not
 * to whatever is nearest; and a season's list of episodes walked down, one opened and gone back to.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import {
  filmDetail,
  films,
  isDrawn,
  isFocusIn,
  mountApp,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
  pressesUntil,
} from "./test-walk.tsx";

/** The screens the walks are made at: a Fire TV's page in Silk, and a desktop's window. */
const screens = [
  { name: "a Fire TV's page in Silk", width: 960, height: 480 },
  { name: "a desktop's window", width: 1280, height: 800 },
] as const;

/** A film partway watched, for Continue Watching. */
function partway(id: number, name: string): Record<string, unknown> {
  return { id, name, libraryId: 1, fileId: id * 10, positionMs: 500_000, durationMs: 1_000_000 };
}

/** Home with three films partway watched, over the newest films and series, twelve each. */
const withContinueWatching: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": {
    continueWatching: [partway(201, "Metropolis"), partway(202, "Nosferatu"), partway(203, "Sunrise")],
  },
  "GET /titles": { titles: films(12), nextCursor: "" },
};

/**
 * Continue Watching as the server has it once it has heard each hide: the rows less those whose
 * POST /items/<id>/hide is in heard. Read at each request, so a background poll before the hide
 * still finds the row, and its card's open "…" stays.
 */
function homeLessHidden(
  rows: readonly Record<string, unknown>[],
  heard: readonly string[],
): object {
  return {
    get continueWatching() {
      return rows.filter((row) => !heard.includes(`POST /items/${String(row["id"])}/hide`));
    },
  };
}

/** An episode of season 1 of series 20. */
function episode(number: number): Record<string, unknown> {
  return {
    id: 300 + number,
    seasonNumber: 1,
    episodeNumber: number,
    name: `Episode ${String(number)}`,
    identificationState: "identified",
    hasLiveFile: true,
  };
}

/** A series of two seasons, its first of four episodes. */
const aSeason: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /items/20": {
    id: 20,
    kind: "series",
    name: "Glass Tide",
    libraryId: 1,
    art: {},
    seasons: [
      { seasonNumber: 1, episodeCount: 4 },
      { seasonNumber: 2, episodeCount: 1 },
    ],
  },
  "GET /items/20/groups/1": { episodes: [1, 2, 3, 4].map(episode), nextCursor: "" },
  "GET /items/303": filmDetail(303, "Episode 3"),
};

/** aSeason with its next episode, the second, which its page opens on (R3). */
const withNextUp: WalkAnswers = {
  ...aSeason,
  "GET /items/20": {
    ...(aSeason["GET /items/20"] as Record<string, unknown>),
    orderings: [{ name: "aired", kind: "aired", episodeCount: 5 }],
    nextUp: { titleId: 302, name: "Episode 2", groupNumber: 1, numberInGroup: 2, fileId: 3020 },
  },
};

/**
 * A series of three seasons, the specials last, its first of four episodes (D379): another series
 * than aSeason's, whose episode an earlier walk reached, and which its page would focus on showing.
 */
const withThreeSeasons: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /items/21/groups/1": { episodes: [1, 2, 3, 4].map(episode), nextCursor: "" },
  "GET /items/21": {
    id: 21,
    kind: "series",
    name: "Glass Tide",
    libraryId: 1,
    art: {},
    seasons: [
      { seasonNumber: 1, episodeCount: 4 },
      { seasonNumber: 2, episodeCount: 1, isWatched: true },
      { seasonNumber: 0, episodeCount: 1 },
    ],
  },
};

/** A series of ten seasons, shown at its last, whose card lies past the row's right edge. */
const withTenSeasons: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /items/22/groups/10": { episodes: [1, 2].map(episode), nextCursor: "" },
  "GET /items/22": {
    id: 22,
    kind: "series",
    name: "Long Haul",
    libraryId: 1,
    art: {},
    seasons: Array.from({ length: 10 }, (_, index) => ({
      seasonNumber: index + 1,
      episodeCount: 2,
    })),
  },
};

/** The season in the series' row of seasons that has focus, by its name; "" for none. */
function focusedSeason(): string {
  const card = document.activeElement?.closest(".series__seasons .poster-card");
  return card?.querySelector(".poster-card__name")?.textContent ?? "";
}

/** Presses a key until a test holds, at most 8 times; whether it held. */
const pressUntil = pressesUntil((keys) => userEvent.keyboard(keys), 8);

/** Home's row of the newest films. */
const newestFilms = '[aria-labelledby="home-newest-movie"]';

/** The newest films' card that has focus: its film's name; "" for none. */
function focusedNewest(): string {
  const card = document.activeElement?.closest(`${newestFilms} .poster-card`);
  return card?.querySelector(".poster-card__name")?.textContent ?? "";
}

/** The Continue Watching card that has focus, or what is in it: its film's name; "" for none. */
function focusedCard(): string {
  const row = document.activeElement?.closest(".continue-watching .poster-card");
  return row?.querySelector(".poster-card__name")?.textContent ?? "";
}

/**
 * The sides of the focus ring around what has focus that the row selector names, which scrolls
 * sideways, clips: none where the ring is seen whole.
 */
function ringOutside(selector: string): string[] {
  const focused = document.activeElement;
  const row = document.querySelector(selector);
  if (focused === null || row === null) {
    return ["nothing has focus in the row"];
  }
  const ringPx = Number.parseFloat(getComputedStyle(focused).outlineWidth) * 2;
  const box = focused.getBoundingClientRect();
  const clip = row.getBoundingClientRect();
  const sides: [string, boolean][] = [
    ["top", box.top - ringPx < clip.top - 0.5],
    ["left", box.left - ringPx < clip.left - 0.5],
    ["bottom", box.bottom + ringPx > clip.bottom + 0.5],
    ["right", box.right + ringPx > clip.right + 0.5],
  ];
  return sides.filter(([, isClipped]) => isClipped).map(([side]) => side);
}

/** The episode that has focus, by its name; "" for none. */
function focusedEpisode(): string {
  const focused = document.activeElement;
  return focused?.closest(".series__episodes") === null
    ? ""
    : (focused?.textContent ?? "").replace(/^.*?(Episode \d+).*$/u, "$1");
}

describe.each(screens)("rows and lists by a remote's keys, on $name", ({ width, height }) => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  it("goes back to the film it left in Continue Watching, and to the one it left in the newest films", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(withContinueWatching, "/"));
    await expect.poll(() => isDrawn(`${newestFilms} .poster-card__link`)).toBe(true);
    await expect.poll(() => isDrawn(".continue-watching .poster-card__link")).toBe(true);
    // The page's arrival (R3): Down from it would leave the row now that Hide is in the card's "…".
    await expect.poll(focusedCard).toBe("Metropolis");
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedCard()).toBe("Nosferatu");
    expect(await pressUntil("{ArrowDown}", () => focusedNewest() !== "")).toBe(true);
    expect(await pressUntil("{ArrowLeft}", () => focusedNewest() === "Film 1")).toBe(true);
    // Up from under Metropolis, into the row at Nosferatu, where it was.
    expect(await pressUntil("{ArrowUp}", () => isFocusIn(".continue-watching"))).toBe(true);
    expect(focusedCard()).toBe("Nosferatu");
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedCard()).toBe("Sunrise");
    // Down from over Film 3, into the newest films at Film 1, where it was.
    expect(await pressUntil("{ArrowDown}", () => focusedNewest() !== "")).toBe(true);
    expect(focusedNewest()).toBe("Film 1");
  });

  it("leaves Continue Watching by one Down, and hides a film from the card's own …, reached by Up", async () => {
    await page.viewport(width, height);
    const heard: string[] = [];
    const answers: WalkAnswers = {
      ...withContinueWatching,
      "GET /home": homeLessHidden(
        [partway(201, "Metropolis"), partway(202, "Nosferatu"), partway(203, "Sunrise")],
        heard,
      ),
      "POST /items/202/hide": null,
    };
    walking = await mountApp(walkServices(answers, "/", heard));
    await expect.poll(() => isDrawn(`${newestFilms} .poster-card__link`)).toBe(true);
    await expect.poll(focusedCard).toBe("Metropolis");
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedCard()).toBe("Nosferatu");
    // Out of the row in one press: to the newest films, or their See all above them.
    await userEvent.keyboard("{ArrowDown}");
    expect(isFocusIn(newestFilms)).toBe(true);
    // Up again into the row at Nosferatu, where it was, and once more to its card's "…".
    expect(await pressUntil("{ArrowUp}", () => focusedCard() === "Nosferatu")).toBe(true);
    expect(document.activeElement?.classList.contains("poster-card__link")).toBe(true);
    await userEvent.keyboard("{ArrowUp}");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("More for Nosferatu");
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => isDrawn("dialog .list-box button")).toBe(true);
    expect(document.activeElement?.textContent).toBe("Hide");
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => heard).toContain("POST /items/202/hide");
    // Focus goes to the card after the one hidden, not to the page's body (D419 §3).
    await expect.poll(() => isDrawn(".continue-watching__hidden")).toBe(true);
    await expect.poll(focusedCard).toBe("Sunrise");
    expect(document.activeElement?.classList.contains("poster-card__link")).toBe(true);
  });

  it("puts focus on the card before a hidden last one, and on Undo when none is left (D419 §3)", async () => {
    await page.viewport(width, height);
    const heard: string[] = [];
    const answers: WalkAnswers = {
      ...withContinueWatching,
      "GET /home": homeLessHidden([partway(201, "Metropolis"), partway(203, "Sunrise")], heard),
      "POST /items/203/hide": null,
      "POST /items/201/hide": null,
    };
    walking = await mountApp(walkServices(answers, "/", heard));
    await expect.poll(focusedCard).toBe("Metropolis");
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedCard()).toBe("Sunrise");
    await userEvent.keyboard("{ArrowUp}");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("More for Sunrise");
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => isDrawn("dialog .list-box button")).toBe(true);
    await userEvent.keyboard("{Enter}");
    await expect.poll(focusedCard).toBe("Metropolis");
    await userEvent.keyboard("{ArrowUp}");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("More for Metropolis");
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => isDrawn("dialog .list-box button")).toBe(true);
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => document.activeElement?.textContent).toBe("Undo");
    expect(document.activeElement?.closest(".continue-watching__hidden")).not.toBeNull();
  });

  it("sets each newest row's See all beside its heading, not across the screen", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(withContinueWatching, "/"));
    await expect.poll(() => isDrawn(`${newestFilms} .home__row-heading a`)).toBe(true);
    const heading = document.querySelector(`${newestFilms} h2`)?.getBoundingClientRect();
    const seeAll = document
      .querySelector(`${newestFilms} .home__row-heading a`)
      ?.getBoundingClientRect();
    // One gap of the space scale's third step, 12px, between them.
    expect((seeAll?.left ?? 0) - (heading?.right ?? 0)).toBeLessThanOrEqual(13);
  });

  it("opens an episode from its season's list and goes back to it there", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(aSeason, "/series/20?season=1"));
    await expect.poll(() => isDrawn(".series__episode")).toBe(true);
    expect(await pressUntil("{ArrowDown}", () => focusedEpisode() === "Episode 3")).toBe(true);
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => isDrawn(".title")).toBe(true);
    await userEvent.keyboard("{Escape}");
    await expect.poll(() => isDrawn(".series__episode")).toBe(true);
    await expect.poll(focusedEpisode).toBe("Episode 3");
  });

  // R5 of ui-audit-2026-11-04.md: Back finds the episode left, not the page's arrival, its next
  // episode's Play, though the page opens on it; at the series' own address too, whose season is
  // the next episode's, read after the page.
  it.each(["/series/20?season=1", "/series/20"])(
    "goes back to the episode it opened at %s, not the page's Play",
    async (path) => {
      await page.viewport(width, height);
      walking = await mountApp(walkServices(withNextUp, path));
      await expect.poll(() => isDrawn(".series__episode")).toBe(true);
      await expect.poll(() => isFocusIn("main")).toBe(true);
      expect(await pressUntil("{ArrowDown}", () => focusedEpisode() === "Episode 3")).toBe(true);
      await userEvent.keyboard("{Enter}");
      await expect.poll(() => isDrawn(".title")).toBe(true);
      await userEvent.keyboard("{Escape}");
      await expect.poll(() => isDrawn(".series__episode")).toBe(true);
      await expect.poll(focusedEpisode).toBe("Episode 3");
      // And stays there, the arrival given no second time as the page's reads come in.
      await new Promise((resolve) => setTimeout(resolve, 300));
      expect(focusedEpisode()).toBe("Episode 3");
    },
  );

  // D379 §3: the seasons' posters are one focus row, Left and Right along it, Down into the episodes.
  it("moves along the seasons' row by Left and Right, and down from it into the season's episodes", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(withThreeSeasons, "/series/21?season=1"));
    await expect.poll(() => isDrawn(".series__episode")).toBe(true);
    document.querySelector<HTMLElement>('.series__seasons a[href="/series/21?season=1"]')?.focus();
    expect(focusedSeason()).toBe("Season 1");
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedSeason()).toBe("Season 2");
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedSeason()).toBe("Specials");
    // Right at the row's end goes nowhere else on the page.
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedSeason()).toBe("Specials");
    expect(await pressUntil("{ArrowLeft}", () => focusedSeason() === "Season 1")).toBe(true);
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedEpisode()).toBe("Episode 1");
  });

  // The Fire TV, 2026-11-02: a series opened from far down the Series list showed its seasons, its
  // name scrolled away above them, as the row brought the season shown into view down the page too.
  it("opens a series at its top, its row of seasons scrolled sideways alone to the season shown", async () => {
    await page.viewport(width, height);
    // From the top, as a page opened is (input-layer.tsx), not where the walk before left the window.
    window.scrollTo({ top: 0, left: 0 });
    walking = await mountApp(walkServices(withTenSeasons, "/series/22?season=10"));
    await expect.poll(() => isDrawn(".series__episode")).toBe(true);
    const row = document.querySelector(".series__seasons");
    const shown = row?.querySelector('[aria-current="page"]');
    await expect.poll(() => row?.scrollLeft ?? 0).toBeGreaterThan(0);
    const [rowBox, shownBox] = [row?.getBoundingClientRect(), shown?.getBoundingClientRect()];
    expect(shownBox?.right).toBeLessThanOrEqual((rowBox?.right ?? 0) + 0.5);
    expect(shownBox?.left).toBeGreaterThanOrEqual((rowBox?.left ?? 0) - 0.5);
    expect(window.scrollY).toBe(0);
  });

  it("draws the whole focus ring of a card in Continue Watching and of a place in the banner, rows that scroll sideways", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(withContinueWatching, "/"));
    await expect.poll(() => isDrawn(".continue-watching .poster-card__link")).toBe(true);
    // The page's arrival (R3): Down from it would leave the row now that Hide is in the card's "…".
    await expect.poll(focusedCard).toBe("Metropolis");
    expect(ringOutside(".continue-watching .poster-row")).toEqual([]);
    document.querySelector<HTMLElement>('.app-shell__watching a[href="/series"]')?.focus();
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement?.getAttribute("href")).toBe("/films");
    expect(ringOutside(".app-shell__watching")).toEqual([]);
  });
});
