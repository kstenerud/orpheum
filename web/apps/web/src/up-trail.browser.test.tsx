// SPDX-License-Identifier: MPL-2.0

/**
 * The trail (D367 §8) as a real browser lays it out and a press follows it: on a title's page and a
 * Play page, at a computer's, a phone's and a Fire TV's widths, nothing drawn over it, and on a Play
 * page its last part, what plays, a press away from that thing's own page (the owner, 2026-10-28).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";

import { routeHref } from "./router.ts";
import { directDecision } from "./test-fixtures.ts";
import {
  filmDetail,
  isDrawn,
  mountApp,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
} from "./test-walk.tsx";

/** A Fire TV Stick's Silk, as the owner's says itself. */
const fireTvSilk =
  "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36";

/** The screens the trail is drawn on: a computer's window, a phone's, a Fire TV's page in Silk. */
const screens = [
  { name: "a computer's window", width: 1280, height: 800, isTelevision: false },
  { name: "a phone's screen", width: 390, height: 844, isTelevision: false },
  { name: "a Fire TV's page in Silk", width: 960, height: 480, isTelevision: true },
] as const;

/** Episode 23 of Long Run, S2E3 "Low Water": its page and its file's playback (file 230). */
const episodeToPlay: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /items/23": {
    id: 23,
    kind: "episode",
    name: "Low Water",
    identificationState: "identified",
    art: {},
    versions: [],
    extras: [],
    episode: { seriesId: 5, seriesName: "Long Run", seasonNumber: 2, episodeNumber: 3 },
  },
  "POST /playback": directDecision({
    titleName: "Low Water",
    plays: {
      titleId: 23,
      kind: "episode",
      libraryId: 1,
      libraryName: "Shows",
      episode: { seriesId: 5, seriesName: "Long Run", seasonNumber: 2, episodeNumber: 3 },
    },
    url: "/no-such-media",
  }),
};

/** The page's trail, once drawn. */
async function theTrail(): Promise<HTMLElement> {
  await expect.poll(() => isDrawn(".up-trail a")).toBe(true);
  const trail = document.querySelector<HTMLElement>(".up-trail");
  if (trail === null) {
    throw new Error("the page draws no trail");
  }
  return trail;
}

/**
 * Each point of the trail's parts that something else is drawn over, as "part at y": the banner, a
 * backdrop, a player. Each part is tried across its middle, near its top and near its bottom.
 */
function coveredPointsOf(trail: HTMLElement): string[] {
  const parts = [...trail.querySelectorAll("li, a")];
  return parts.flatMap((part) => {
    const box = part.getBoundingClientRect();
    return [0.2, 0.5, 0.8]
      .map((fraction) => box.top + box.height * fraction)
      .filter((y) => {
        const drawn = document.elementFromPoint(box.left + box.width / 2, y);
        return drawn === null || !trail.contains(drawn);
      })
      .map((y) => `${part.textContent} at ${y.toFixed(0)}`);
  });
}

/**
 * Draws a backdrop on a title's page, as WideArt does, an img of the class in the article's place
 * for it: the walk serves no art's bytes, and a backdrop that fails to load is not drawn.
 */
function drawBackdrop(): void {
  const article = document.querySelector("article.title");
  if (article === null) {
    throw new Error("the page draws no title");
  }
  const backdrop = document.createElement("img");
  backdrop.className = "title__backdrop";
  backdrop.alt = "";
  backdrop.src = `data:image/svg+xml,${encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="9"><rect width="16" height="9"/></svg>',
  )}`;
  article.prepend(backdrop);
}

describe.each(screens)("the trail, on $name", ({ width, height, isTelevision }) => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  // The backdrop rose into the trail's space and, a layer of its own by its mask, was drawn over
  // the trail's lower half (the owner, 2026-10-28: "covers half of the trail text").
  it("sits below the banner on a title's page, its backdrop below it, nothing drawn over it", async () => {
    if (isTelevision) {
      vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(fireTvSilk);
    }
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices(
        {
          "GET /me": viewer,
          "GET /client-settings": {},
          "GET /items/103": filmDetail(103, "Metropolis"),
        },
        "/titles/103",
      ),
    );
    const trail = await theTrail();
    await expect.poll(() => isDrawn(".title__head")).toBe(true);
    drawBackdrop();
    const banner = document.querySelector(".app-shell__banner")?.getBoundingClientRect();
    const backdrop = document.querySelector(".title__backdrop")?.getBoundingClientRect();
    const box = trail.getBoundingClientRect();
    expect(box.top).toBeGreaterThanOrEqual(banner?.bottom ?? Infinity);
    expect(backdrop?.top).toBeGreaterThanOrEqual(box.bottom);
    expect(coveredPointsOf(trail)).toEqual([]);
  });
});

describe.each(screens.filter((screen) => !screen.isTelevision))(
  "a Play page's trail, on $name",
  ({ width, height }) => {
    let walking: Walking | undefined;
    afterEach(() => {
      walking?.unmount();
      walking = undefined;
    });

    /** The trail's parent, what plays' own page, the last link before the playing. */
    function whatPlays(): HTMLAnchorElement | null {
      return (
        [...document.querySelectorAll<HTMLAnchorElement>(".up-trail__crumbs li a")].at(-1) ?? null
      );
    }

    // The owner, 2026-10-28: "When I play a video, there's no way to go directly back to the info
    // page", then: "it always leaves a trail that goes back to the info page before going any
    // further back". The playing is the trail's last part, what plays' page its parent.
    it("shows what plays, nothing drawn over it, and goes to its page in its place", async () => {
      await page.viewport(width, height);
      walking = await mountApp(walkServices(episodeToPlay, "/play/230"));
      const { navigation } = walking.services;
      const trail = await theTrail();
      await expect.poll(() => isDrawn("video")).toBe(true);
      expect(whatPlays()?.textContent).toBe("S2E3 Low Water");
      expect(document.querySelector('.up-trail [aria-current="page"]')?.textContent).toBe(
        "Playing",
      );
      expect(coveredPointsOf(trail)).toEqual([]);
      whatPlays()?.click();
      await expect
        .poll(() => routeHref(navigation.route()))
        .toBe(routeHref({ screen: "title", itemId: 23 }));
      // Opened from outside: the episode's page in the Play page's place, nothing added.
      expect(navigation.behind()).toEqual([]);
    });

    it("goes back to the episode's page behind it, adding nothing", async () => {
      await page.viewport(width, height);
      walking = await mountApp(walkServices(episodeToPlay, "/titles/23"));
      const { navigation } = walking.services;
      await expect.poll(() => isDrawn(".title__head")).toBe(true);
      navigation.go({ screen: "play", fileId: 230 });
      await expect.poll(() => whatPlays() !== null).toBe(true);
      whatPlays()?.click();
      await expect
        .poll(() => routeHref(navigation.route()))
        .toBe(routeHref({ screen: "title", itemId: 23 }));
      expect(navigation.behind()).toEqual([]);
    });
  },
);
