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
  "GET /collections/20": {
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
  "GET /collections/20/seasons/1": { episodes: [1, 2, 3, 4].map(episode), nextCursor: "" },
  "GET /items/303": filmDetail(303, "Episode 3"),
};

/** Presses key until isThere, at most times times; whether it got there. */
async function pressUntil(key: string, isThere: () => boolean, times = 8): Promise<boolean> {
  for (let pressed = 0; pressed < times && !isThere(); pressed += 1) {
    await userEvent.keyboard(key);
  }
  return isThere();
}

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
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedCard()).toBe("Metropolis");
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

  it("draws the whole focus ring of a card in Continue Watching and of a place in the banner, rows that scroll sideways", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(withContinueWatching, "/"));
    await expect.poll(() => isDrawn(".continue-watching .poster-card__link")).toBe(true);
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedCard()).toBe("Metropolis");
    expect(ringOutside(".continue-watching .poster-row")).toEqual([]);
    document.querySelector<HTMLElement>('.app-shell__watching a[href="/series"]')?.focus();
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement?.getAttribute("href")).toBe("/films");
    expect(ringOutside(".app-shell__watching")).toEqual([]);
  });
});
