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

import { railLetters } from "./alphabet-rail.tsx";

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

  it("is one Tab stop: Tab enters at the card last focused, and the next leaves (the owner, 2026-11-11)", async () => {
    await page.viewport(1440, 900);
    walking = await mountApp(walkServices(filmsOnly));
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    const grid = document.querySelector(".poster-grid");
    document.querySelector<HTMLElement>(".poster-grid .poster-card__link")?.focus();
    // The arrows still move from card to card, all but one out of Tab's order.
    await userEvent.keyboard("{ArrowRight}{ArrowRight}");
    expect(focusedTile()).toBe("Film 3");
    await userEvent.keyboard("{Tab}");
    expect(grid?.contains(document.activeElement)).toBe(false);
    await userEvent.keyboard("{Shift>}{Tab}{/Shift}");
    expect(focusedTile()).toBe("Film 3");
    const inTabsOrder = [...(grid?.querySelectorAll<HTMLElement>("a, button") ?? [])].filter(
      (control) => control.tabIndex >= 0,
    );
    expect(inTabsOrder).toHaveLength(1);
  });

  it("enters the alphabet rail at the letter of the poster just left, by an arrow or by Tab (the owner, 2026-11-11)", async () => {
    await page.viewport(960, 480);
    // Each film under a letter of its own, as a list by name says it: Film 1 under B, Film 2 under C.
    const lettered = films(12).map((film, index) => ({ ...film, letter: railLetters[index + 2] }));
    walking = await mountApp(
      walkServices({ ...filmsOnly, "GET /items": { items: lettered, nextCursor: "" } }),
    );
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    // The rail's letters come from their own request: a letter not yet drawn cannot be stood at.
    await expect
      .poll(() => document.querySelectorAll(".alphabet-rail__letter").length)
      .toBeGreaterThan(0);
    const links = document.querySelectorAll<HTMLElement>(".poster-grid .poster-card__link");
    links[5]?.focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(document.activeElement?.textContent).toBe("G");
    await userEvent.keyboard("{ArrowLeft}");
    expect(focusedTile()).toBe("Film 6");
    links[1]?.focus();
    await userEvent.keyboard("{Tab}");
    expect(document.activeElement?.textContent).toBe("C");
  });

  it("fills in the posters a hundred below the view ahead, and none past them (the owner, 2026-11-11)", async () => {
    await page.viewport(960, 480);
    const withPosters = films(300).map((film, index) => ({
      ...film,
      posterSha256: index.toString(16).padStart(64, "0"),
    }));
    // Each image asked for, by its address: the page's own timing list is full of the test's modules.
    const asked = new Set<string>();
    const watcher = new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        asked.add(entry.name);
      }
    });
    watcher.observe({ type: "resource" });
    walking = await mountApp(
      walkServices({ ...filmsOnly, "GET /items": { items: withPosters, nextCursor: "" } }),
    );
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    window.scrollTo(0, 0);
    const askedFor = (place: number): boolean =>
      [...asked].some((name) =>
        name.includes(`/bytes/art/${place.toString(16).padStart(64, "0")}/`),
      );
    // Fifteen rows down, far past what the browser's own lazy loading reaches.
    await expect.poll(() => askedFor(95), { timeout: 10_000 }).toBe(true);
    expect(askedFor(150)).toBe(false);
    // Scrolled to the middle of the list, the window moves with it.
    document.querySelectorAll(".poster-grid > li")[150]?.scrollIntoView({ block: "center" });
    await expect.poll(() => askedFor(240), { timeout: 10_000 }).toBe(true);
    watcher.disconnect();
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
      walkServices({ ...filmsOnly, "GET /items": { items: films(60), nextCursor: "" } }),
    );
    await expect.poll(() => document.querySelectorAll(".poster-grid > li").length).toBe(60);
    window.scrollTo(0, document.documentElement.scrollHeight);
    expect(window.scrollY).toBeGreaterThan(0);
    expect(Math.abs(boxOf(".alphabet-rail").top - stickingTop())).toBeLessThanOrEqual(1);
  });

  it("keeps a note of the shell's over the list clear of the rail, fixed beside it from the top", async () => {
    await page.viewport(960, 480);
    walking?.unmount();
    walking = await mountApp(walkServices(filmsOnly));
    await expect.poll(() => isDrawn(".alphabet-rail__letter")).toBe(true);
    // A note as the shell draws one before the page, as the plain-http note is.
    const note = document.createElement("div");
    note.className = "app-shell__note";
    note.textContent = "A note of the shell's";
    document.querySelector(".app-shell__main")?.prepend(note);
    expect(boxOf(".app-shell__note").right).toBeLessThanOrEqual(boxOf(".alphabet-rail").left);
    note.remove();
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
  "GET /items": { items: films(12), nextCursor: "" },
};
