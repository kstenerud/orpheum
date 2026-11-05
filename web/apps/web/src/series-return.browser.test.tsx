// SPDX-License-Identifier: MPL-2.0

/**
 * A series returned to by its crumb in an episode's trail, in a real browser (D367 §9): the season
 * the episode is in is shown, the episode focused. Split from input-player.browser.test.tsx, whose
 * walks are of the player itself.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { routeHref } from "./router.ts";
import { isDrawn, mountApp, viewer, walkServices, type Walking } from "./test-walk.tsx";

describe("a series returned to by Up from an episode of another season (D367 §9)", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  /** An episode of Long Run (5): its id, and its place. */
  function episodeOf(id: number, season: number): Record<string, unknown> {
    return {
      id,
      seasonNumber: season,
      episodeNumber: 1,
      name: `Episode ${String(id)}`,
      identificationState: "identified",
      hasLiveFile: true,
    };
  }

  // A binge from season 1 into season 2 left the series' entry at season 1 (§3.7): returning to it
  // shows season 2, the episode last reached focused, on the same page, not one drawn anew.
  it("shows that episode's season, the episode focused", async () => {
    await page.viewport(1280, 800);
    walking = await mountApp(
      walkServices(
        {
          "GET /me": viewer,
          "GET /client-settings": {},
          "GET /items/5": {
            id: 5,
            kind: "series",
            name: "Long Run",
            art: {},
            seasons: [1, 2].map((seasonNumber) => ({ seasonNumber, episodeCount: 1 })),
          },
          "GET /collections/5/seasons/1": { episodes: [episodeOf(21, 1)] },
          "GET /collections/5/seasons/2": { episodes: [episodeOf(23, 2)] },
          "GET /items/23": {
            id: 23,
            kind: "episode",
            name: "Episode 23",
            identificationState: "identified",
            art: {},
            versions: [],
            extras: [],
            episode: { seriesId: 5, seriesName: "Long Run", seasonNumber: 2, episodeNumber: 1 },
          },
        },
        "/series/5?season=1",
      ),
    );
    const { navigation } = walking.services;
    await expect.poll(() => isDrawn('a[href="/titles/21"]')).toBe(true);
    navigation.go({ screen: "title", itemId: 23 });
    const seriesCrumb = (): HTMLAnchorElement | undefined =>
      [...document.querySelectorAll<HTMLAnchorElement>(".up-trail a")].find(
        (link) => link.textContent === "Long Run",
      );
    await expect.poll(() => seriesCrumb() !== undefined).toBe(true);
    seriesCrumb()?.click();
    await expect.poll(() => document.activeElement?.getAttribute("href")).toBe("/titles/23");
    expect(routeHref(navigation.route())).toBe("/series/5?season=2");
    expect(navigation.behind()).toEqual([]);
  });
});
