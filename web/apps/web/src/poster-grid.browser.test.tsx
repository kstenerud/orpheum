// SPDX-License-Identifier: MPL-2.0

/**
 * The films' grid as a real browser lays it out at a Fire TV's page in Silk (960 × 480): six posters
 * across, as Continue Watching shows them, and room around each card for the focus ring, which the
 * grid's off-screen skipping (content-visibility) would otherwise clip (the owner, Silk, 2026-10-25).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import {
  films,
  focusedTile,
  isDrawn,
  mountApp,
  viewer,
  walkServices,
  type Walking,
} from "./test-walk.tsx";

describe("the films' grid at a Fire TV's page in Silk", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  it("shows six posters across beside the alphabet rail, each with room for its focus ring", async () => {
    await page.viewport(960, 480);
    walking = await mountApp(walkServices(filmsOnly));
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    const items = [...document.querySelectorAll<HTMLElement>(".poster-grid > li")];
    const firstRowTop = items[0]?.getBoundingClientRect().top;
    const across = items.filter((item) => item.getBoundingClientRect().top === firstRowTop).length;
    expect(across).toBe(6);
    // Beside the alphabet rail, the posters a little smaller to keep six (a prototype, 2026-10-25).
    expect(document.querySelector(".alphabet-rail")?.children).toHaveLength(27);
    const card = items[0]?.querySelector(".poster-grid .poster-card__link");
    const item = items[0];
    if (card === null || card === undefined || item === undefined) {
      throw new Error("the grid has no card");
    }
    // The ring reaches twice its width outside the card; the item's box must hold it.
    const ringRoom = 6;
    expect(
      card.getBoundingClientRect().left - item.getBoundingClientRect().left,
    ).toBeGreaterThanOrEqual(ringRoom);
  });

  it("walks into the alphabet rail from the grid's right edge and back to the tile it left", async () => {
    await page.viewport(960, 480);
    walking = await mountApp(walkServices(filmsOnly));
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    const sixth = document.querySelectorAll<HTMLElement>(".poster-grid .poster-card__link")[5];
    sixth?.focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(document.activeElement?.classList.contains("alphabet-rail__letter")).toBe(true);
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement?.classList.contains("alphabet-rail__letter")).toBe(true);
    await userEvent.keyboard("{ArrowLeft}");
    expect(focusedTile()).toBe("Film 6");
  });

  it("keeps the page's own margin left of the posters, the rail centred and whole in its gutter right of them", async () => {
    for (const [width, height] of [
      [960, 480],
      [1440, 1000],
    ] as const) {
      await page.viewport(width, height);
      walking?.unmount();
      walking = await mountApp(walkServices(filmsOnly));
      await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
      const cards = [...document.querySelectorAll(".poster-grid .poster-card__link")].map((each) =>
        each.getBoundingClientRect(),
      );
      // The page's margin alone on the left: the gutter that mirrored the rail's is gone (the owner,
      // 2026-10-27: with the rail there it is not missed, and a phone fitted two posters a row).
      const left = Math.min(...cards.map((card) => card.left));
      const pageMargin =
        Number.parseFloat(
          getComputedStyle(document.documentElement).getPropertyValue("--space-page"),
        ) * Number.parseFloat(getComputedStyle(document.documentElement).fontSize);
      expect(Math.abs(left - pageMargin)).toBeLessThanOrEqual(8);
      // The rail centred in the gutter right of the posters, and whole on the page, Z included.
      const rail = boxOf(".alphabet-rail");
      const postersRight = Math.max(...cards.map((card) => card.right));
      expect(
        Math.abs(rail.left - postersRight - (window.innerWidth - rail.right)),
      ).toBeLessThanOrEqual(2);
      expect(boxOf(".alphabet-rail > li:last-child").bottom).toBeLessThanOrEqual(
        window.innerHeight,
      );
    }
  });

  it("fits three posters a row on a phone, down to 360 pixels wide (the owner, 2026-10-27)", async () => {
    for (const [width, height] of [
      [412, 915],
      [393, 852],
      [360, 780],
    ] as const) {
      await page.viewport(width, height);
      walking?.unmount();
      walking = await mountApp(walkServices(filmsOnly));
      await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
      const items = [...document.querySelectorAll<HTMLElement>(".poster-grid > li")];
      const firstRowTop = items[0]?.getBoundingClientRect().top;
      const across = items.filter(
        (item) => item.getBoundingClientRect().top === firstRowTop,
      ).length;
      expect(across, `${String(width)} pixels wide`).toBe(3);
    }
  });

  it("runs the rail from under the banner to the window's bottom, its letters on it, unmoved as the list scrolls", async () => {
    for (const [width, height] of [
      [960, 480],
      [1440, 1000],
    ] as const) {
      await page.viewport(width, height);
      walking?.unmount();
      walking = await mountApp(walkServices(filmsOnly));
      await expect.poll(() => isDrawn(".alphabet-rail__letter")).toBe(true);
      window.scrollTo(0, 0);
      const rail = boxOf(".alphabet-rail");
      expect(rail.top).toBeGreaterThanOrEqual(boxOf(".app-shell__banner").bottom);
      expect(rail.bottom).toBeLessThanOrEqual(window.innerHeight);
      expect(boxOf(".alphabet-rail > li:first-child").top).toBeGreaterThanOrEqual(rail.top);
      expect(boxOf(".alphabet-rail > li:last-child").bottom).toBeLessThanOrEqual(rail.bottom);
      // Where it sticks once the list scrolls, so it does not move then (the owner's Fire TV).
      expect(Math.abs(rail.top - stickingTop())).toBeLessThanOrEqual(1);
    }
  });

  it("keeps the rail where it sticks with the list scrolled to its very end (the owner, 2026-11-02)", async () => {
    await page.viewport(960, 480);
    walking?.unmount();
    walking = await mountApp(
      walkServices({ ...filmsOnly, "GET /titles": { titles: films(60), nextCursor: "" } }),
    );
    await expect.poll(() => document.querySelectorAll(".poster-grid > li").length).toBe(60);
    window.scrollTo(0, document.documentElement.scrollHeight);
    expect(window.scrollY).toBeGreaterThan(0);
    expect(Math.abs(boxOf(".alphabet-rail").top - stickingTop())).toBeLessThanOrEqual(1);
  });

  it("sizes the rail's letters by the window's height, larger on a desktop's than a Fire TV's", async () => {
    const letterSize = async (width: number, height: number): Promise<number> => {
      await page.viewport(width, height);
      walking?.unmount();
      walking = await mountApp(walkServices(filmsOnly));
      await expect.poll(() => isDrawn(".alphabet-rail__letter")).toBe(true);
      const letter = document.querySelector(".alphabet-rail__letter");
      return letter === null ? 0 : Number.parseFloat(getComputedStyle(letter).fontSize);
    };
    const onTheStick = await letterSize(960, 480);
    const onADesktop = await letterSize(1440, 1000);
    expect(onADesktop).toBeGreaterThan(onTheStick * 1.5);
  });
});

/** Where the alphabet rail sticks under the banner as the page scrolls, in pixels from the top. */
function stickingTop(): number {
  const rail = document.querySelector(".alphabet-rail");
  return rail === null ? Number.NaN : Number.parseFloat(getComputedStyle(rail).top);
}

/** Where the element selector names stands on the page; the test fails where there is none. */
function boxOf(selector: string): DOMRect {
  const element = document.querySelector(selector);
  if (element === null) {
    throw new Error(`the page has no ${selector}`);
  }
  return element.getBoundingClientRect();
}

/** The films' page: twelve films, nothing partway. */
const filmsOnly = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": { continueWatching: [] },
  "GET /titles": { titles: films(12), nextCursor: "" },
};
