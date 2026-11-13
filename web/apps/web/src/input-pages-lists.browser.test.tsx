// SPDX-License-Identifier: MPL-2.0

/**
 * The app's pages walked with a remote's keys in a real browser, as input-pages.browser.test.tsx
 * walks them, here a list's choosers and tools, its Sort and filter, and search: each arrived at,
 * every control on it reached by arrows, one chosen, and Back finding focus where it was.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { focusTargetsIn } from "./focus-engine.ts";
import { createNavigation } from "./navigation.ts";
import { fakePlace } from "./test-services.tsx";
import {
  screens,
  signedIn,
  aLibrary,
  criteriaButton,
  sortChooser,
  isOnFirstPlace,
  isFocusOnLinkTo,
  remoteWalk,
} from "./test-walk.tsx";
import {
  filmDetail,
  films,
  focusedName,
  isFocusIn,
  isFocusOnFirst,
  mountApp,
  reachedFrom,
  walkServices,
  type Walking,
} from "./test-walk.tsx";

const { press, pressUntil, drawn } = remoteWalk({
  expect,
  keyboard: async (keys) => {
    await userEvent.keyboard(keys);
  },
});

describe.each(screens)("the pages by a remote's keys, on $name", ({ width, height }) => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  it("closes a chooser's list by the browser's own Back, a history step, choosing nothing, and draws no Close", async () => {
    await page.viewport(width, height);
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "browse", library: 1 });
    walking = await mountApp({ ...walkServices(aLibrary), navigation });
    await drawn(".poster-grid a");
    // The sort is in the list's Sort and filter (D417), a chooser inside its pop-up; the button has
    // no chevron, as the menu has none (the owner, 2026-10-30).
    const button = criteriaButton();
    expect(button?.querySelector(".route-chooser__chevron")).toBeNull();
    expect(document.querySelector(".app-shell__menu-button .route-chooser__chevron")).toBeNull();
    button?.focus();
    await press("{Enter}");
    await drawn(".list-criteria");
    const picker = sortChooser();
    picker?.focus();
    await press("{Enter}");
    await drawn('dialog .list-box [role="button"]');
    // The browser's Back closes the chooser's list alone, then the pop-up it was opened from.
    place.history.back();
    await expect.poll(() => document.querySelectorAll("dialog")).toHaveLength(1);
    await expect.poll(() => document.activeElement).toBe(picker);
    place.history.back();
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    await expect.poll(() => document.activeElement).toBe(button);
    expect(navigation.route()).toEqual({ screen: "browse", library: 1 });
    expect(navigation.behind()).toEqual([{ screen: "home" }]);
    // A pop-up, not a dialog: no Close among its places (the owner, 2026-11-02; D408 §2).
    await press("{Enter}");
    await drawn(".list-criteria");
    sortChooser()?.focus();
    await press("{Enter}");
    await drawn('dialog .list-box [role="button"]');
    expect(document.querySelector('dialog button[aria-label="Close"]')).toBeNull();
    await press("{Escape}");
    await expect.poll(() => document.querySelectorAll("dialog")).toHaveLength(1);
    await press("{Escape}");
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(button);
    expect(navigation.route()).toEqual({ screen: "browse", library: 1 });
  });

  // The owner on a Fire TV, 2026-10-28: the season picker, a chooser like this, "shifted the page up
  // before displaying it", and opened on its Close, not a season.
  it("opens a chooser mid-page on its choice, the page moving neither as it opens, its first place reached, nor as it closes", async () => {
    await page.viewport(width, height);
    const above = document.createElement("div");
    above.style.height = `${String(height * 2)}px`;
    document.body.prepend(above);
    try {
      walking = await mountApp(walkServices(aLibrary, "/browse?library=1"));
      await drawn(".poster-grid a");
      const button = criteriaButton();
      if (button === undefined) {
        throw new Error("the library's page draws no Sort and filter");
      }
      window.scrollBy({ top: button.getBoundingClientRect().top - height / 2 });
      const scrolled = window.scrollY;
      expect(scrolled).toBeGreaterThan(0);
      button.focus({ preventScroll: true });
      // The sort is a chooser in the list's Sort and filter (D417): neither pop-up moves the page.
      await press("{Enter}");
      await drawn(".list-criteria");
      expect(window.scrollY).toBe(scrolled);
      const picker = sortChooser();
      if (picker === undefined) {
        throw new Error("the Sort and filter draws no sort chooser");
      }
      picker.focus({ preventScroll: true });
      await press("{Enter}");
      await drawn('dialog .list-box [role="button"]');
      expect(isFocusIn("dialog ul")).toBe(true);
      expect(document.activeElement?.getAttribute("aria-current")).toBe("true");
      expect(window.scrollY).toBe(scrolled);
      expect(await pressUntil("{ArrowUp}", isOnFirstPlace)).toBe(true);
      expect(window.scrollY).toBe(scrolled);
      await press("{Escape}");
      await expect.poll(() => document.querySelectorAll("dialog")).toHaveLength(1);
      expect(document.activeElement).toBe(picker);
      expect(window.scrollY).toBe(scrolled);
      await press("{Escape}");
      await expect.poll(() => document.querySelector("dialog")).toBeNull();
      expect(document.activeElement).toBe(button);
      expect(window.scrollY).toBe(scrolled);
    } finally {
      above.remove();
    }
  });

  it("goes Up from a list shorter than a row to its tools, and Up again to the banner (the owner, 2026-11-04)", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices(
        { ...aLibrary, "GET /items": { items: films(2), nextCursor: "" } },
        "/browse?library=1",
      ),
    );
    await drawn("main .list-criteria__button");
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    await press("{ArrowUp}");
    expect(isFocusIn("main .browse__bar")).toBe(true);
    await press("{ArrowUp}");
    expect(isFocusIn("header")).toBe(true);
  });

  it("types a search on the Libraries page, arrows into its results, opens one and goes back", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices(
        {
          ...aLibrary,
          // A search is the list's own query (the owner, 2026-10-30): its matches are the list,
          // shorter than a row, whose tools Up still reaches before the banner (the owner, 2026-11-04).
          "GET /items": {
            items: [
              {
                id: 101,
                kind: "film",
                name: "Metropolis",
                year: 1995,
                identificationState: "identified",
                addedAt: 1,
              },
              {
                id: 102,
                kind: "film",
                name: "Metropols",
                year: 2001,
                identificationState: "identified",
                addedAt: 2,
              },
            ],
            nextCursor: "",
          },
          "GET /items/102": filmDetail(102, "Metropols"),
          "GET /genres": { genres: [] },
        },
        "/browse?library=1",
      ),
    );
    // The search is the field of the list's Sort and filter, the first thing in its pop-up (D417).
    await drawn("main .list-criteria__button");
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    const first = document.activeElement;
    expect(first).toBeInstanceOf(HTMLElement);
    const reached: Set<Element> =
      first instanceof HTMLElement ? await reachedFrom(first, press) : new Set();
    const isCriteriaButton = (control: Element): control is HTMLElement =>
      control instanceof HTMLElement && control.matches('[role="button"].list-criteria__button');
    const button = [...reached].find(isCriteriaButton);
    expect(button).toBeDefined();
    button?.focus();
    await press("{Enter}");
    await drawn("dialog input.list-criteria__name");
    expect(document.activeElement?.classList.contains("list-criteria__name")).toBe(true);
    await press("metro");
    // Closed, what was typed goes to the address (useListCriteria), the list narrowed by it.
    await press("{Escape}");
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(button);
    await expect
      .poll(() => walking?.services.navigation.route())
      .toMatchObject({ screen: "browse", library: 1, query: "metro" });
    // The list asks for the text once typing rests (listing-tools.tsx typingRestMs), drawn anew; the
    // first, unnarrowed, also has 102, and under load replaced it under focus (2026-11-05).
    await expect.poll(() => first?.isConnected).toBe(false);
    await drawn('main a[href="/titles/102"]');
    // Down from the button lands on the poster nearest below it, which its summary's width decides
    // ("Name · “metro”" once the address has the text): either result, then Right along the row.
    const isOnResult = (): boolean =>
      (document.activeElement?.closest(".poster-grid .poster-card__link") ?? null) !== null;
    expect(await pressUntil("{ArrowDown}", isOnResult)).toBe(true);
    expect(await pressUntil("{ArrowRight}", () => isFocusOnLinkTo("/titles/102"))).toBe(true);
    await press("{Enter}");
    await drawn(".title__play");
    await press("{Escape}");
    await expect.poll(() => isFocusOnLinkTo("/titles/102")).toBe(true);
  });

  // D417 §1: a list's text, chips and sort are one pop-up's, each adding to the others. On a TV
  // focus arrives in its field (R3), Up and Down reach every control in it, a genre chosen in its
  // chooser closes that chooser's list alone, and the pop-up closed puts them all in the address.
  it("opens Films' Sort and filter on its field, Down reaching every control, and a genre chosen adds to what is typed", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices(
        {
          ...signedIn,
          "GET /items": { items: films(3), nextCursor: "" },
          "GET /items/letters": { letters: [] },
          "GET /genres": {
            genres: [
              { id: 2, name: "Drama" },
              { id: 3, name: "Heist" },
            ],
          },
        },
        "/films",
      ),
    );
    await drawn("main .list-criteria__button");
    const button = criteriaButton();
    button?.focus();
    await press("{Enter}");
    await drawn("dialog input.list-criteria__name");
    expect(document.activeElement?.classList.contains("list-criteria__name")).toBe(true);
    await press("hea");
    // Clear is there once something narrows the list: the last control.
    await drawn(".list-criteria__clear");
    const controls = focusTargetsIn(document.querySelector(".list-criteria") ?? document);
    const reached = new Set<Element>([document.activeElement ?? document.body]);
    for (let stepsLeft = controls.length; stepsLeft > 0; stepsLeft -= 1) {
      await press("{ArrowDown}");
      reached.add(document.activeElement ?? document.body);
    }
    expect(
      controls.filter((control) => !reached.has(control)).map((each) => each.outerHTML),
    ).toEqual([]);
    expect(isFocusIn(".list-criteria")).toBe(true);
    expect(await pressUntil("{ArrowUp}", () => focusedName() === "Name contains")).toBe(true);
    expect(await pressUntil("{ArrowDown}", () => focusedName() === "Any genre")).toBe(true);
    await press("{Enter}");
    await drawn('dialog .list-box [role="button"]');
    expect(await pressUntil("{ArrowDown}", () => focusedName() === "Heist")).toBe(true);
    await press("{Enter}");
    await expect.poll(() => document.querySelectorAll("dialog")).toHaveLength(1);
    expect(focusedName()).toBe("Heist");
    await press("{Escape}");
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(button);
    await expect
      .poll(() => walking?.services.navigation.route())
      .toEqual({ screen: "films", query: "hea", genre: 3 });
  });
});
