// SPDX-License-Identifier: MPL-2.0

/**
 * The app's pages walked with a remote's keys in a real browser, at a Fire TV's page in Silk (960 ×
 * 480 CSS pixels below its address bar) and a desktop's window (controller-input-2026-10-25.md
 * §6.3): the banner's menu, a film's and a series' pages, search, This device and sign-in, each
 * arrived at, every control on it reached by arrows, one chosen, and Back finding focus where it was.
 *
 * Here the banner's menu and a title's pages; input-pages-lists.browser.test.tsx and
 * input-pages-person.browser.test.tsx walk the rest, the answers and keys all three take in
 * test-input-pages.tsx (split for the browser run's time, make-check-time-2026-11-12.md §2 and §3).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { caretOf, focusTargetsIn } from "./focus-engine.ts";
import { createNavigation } from "./navigation.ts";
import { fakePlace } from "./test-services.tsx";
import {
  screens,
  signedIn,
  aLibrary,
  glassTide,
  manyLibraries,
  isScrollingWithin,
  isShownWholeIn,
  isOnFirstPlace,
  isFocusOnLinkTo,
  remoteWalk,
} from "./test-input-pages.tsx";
import {
  filmDetail,
  films,
  focusedName,
  isDrawn,
  isFocusIn,
  isFocusOnFirst,
  mountApp,
  reachedFrom,
  viewer,
  walkServices,
  type Walking,
} from "./test-walk.tsx";

const { press, pressUntil, drawn, unreachedInMain } = remoteWalk({
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

  it("opens the banner's menu by OK, goes down its places to This device, and Back returns to its icon", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices({ ...signedIn, "GET /me/devices": { devices: [] } }));
    // The page opens with focus on its first poster, its arrival (ui-audit-2026-11-04.md R3).
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    expect(await pressUntil("{ArrowUp}", () => isFocusIn(".app-shell__banner"))).toBe(true);
    expect(await pressUntil("{ArrowRight}", () => focusedName() === "Menu")).toBe(true);
    await press("{Enter}");
    expect(isDrawn("dialog")).toBe(true);
    expect(await pressUntil("{ArrowDown}", () => isFocusOnLinkTo("/this-device"))).toBe(true);
    await press("{Enter}");
    await drawn("#this-device-heading");
    await press("{Escape}");
    await drawn(".poster-grid .poster-card__link");
    await expect.poll(focusedName).toBe("Menu");
    expect(document.querySelector("dialog")).toBeNull();
  });

  it("closes the open menu by Back, keeping its icon, before going anywhere", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(signedIn));
    // The page opens with focus on its first poster, its arrival (ui-audit-2026-11-04.md R3).
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    await pressUntil("{ArrowUp}", () => isFocusIn(".app-shell__banner"));
    await pressUntil("{ArrowRight}", () => focusedName() === "Menu");
    await press("{Enter}");
    // Down waits for the menu: under load a Down sent with the Enter can come before it draws.
    await drawn("dialog ul");
    await press("{ArrowDown}");
    expect(isFocusIn("dialog ul")).toBe(true);
    await press("{Escape}");
    expect(document.querySelector("dialog")).toBeNull();
    expect(focusedName()).toBe("Menu");
    expect(walking.services.navigation.route()).toEqual({ screen: "films" });
  });

  // The owner's Fire TV, 2026-10-28: Silk's remote Back is a step of the history, never a key; "with
  // the chooser widget I can only exit without choosing by pressing the back button, which actually
  // brings me back a page."
  it("closes the open menu by the browser's own Back, a history step, keeping its icon and the page", async () => {
    await page.viewport(width, height);
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "films" });
    walking = await mountApp({ ...walkServices(signedIn), navigation });
    // The page opens with focus on its first poster, its arrival (ui-audit-2026-11-04.md R3).
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    await pressUntil("{ArrowUp}", () => isFocusIn(".app-shell__banner"));
    await pressUntil("{ArrowRight}", () => focusedName() === "Menu");
    await press("{Enter}");
    await drawn("dialog ul");
    await press("{ArrowDown}");
    expect(isFocusIn("dialog ul")).toBe(true);
    place.history.back();
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    expect(focusedName()).toBe("Menu");
    expect(navigation.route()).toEqual({ screen: "films" });
    expect(navigation.behind()).toEqual([{ screen: "home" }]);
  });

  it("closes the open menu by Left and by Right, focus on its icon, Down still moving within it", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(signedIn));
    // The page opens with focus on its first poster, its arrival (ui-audit-2026-11-04.md R3).
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    await pressUntil("{ArrowUp}", () => isFocusIn(".app-shell__banner"));
    await pressUntil("{ArrowRight}", () => focusedName() === "Menu");
    for (const arrow of ["{ArrowLeft}", "{ArrowRight}"]) {
      await press("{Enter}{ArrowDown}{ArrowDown}");
      expect(isFocusIn("dialog ul"), arrow).toBe(true);
      await press(arrow);
      expect(document.querySelector("dialog"), arrow).toBeNull();
      // Focus goes back to the icon once the dialog has closed, a frame after the press.
      await expect.poll(focusedName, { message: arrow }).toBe("Menu");
    }
    expect(walking.services.navigation.route()).toEqual({ screen: "films" });
  });

  it("reaches every control of a film's page, its own Back first", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /items/103": filmDetail(103, "Film 3") }, "/titles/103"),
    );
    await drawn(".title__play");
    expect(await unreachedInMain()).toEqual([]);
  });

  // Under the page's "…" (the owner, 2026-10-29), whose place opens its dialog in the list's place.
  it("reaches every control of an admin's film page, and of Identified incorrectly's dialog opened from its … by OK", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices(
        {
          ...signedIn,
          "GET /me": { ...viewer, role: "admin", isAdmin: true },
          "GET /items/103": filmDetail(103, "Film 3"),
          "GET /items/103/candidates": {
            provider: "tmdb",
            candidates: [
              { providerId: "1", name: "Film 3", year: 2001 },
              { providerId: "2", name: "Film Three", year: 2002 },
            ],
          },
        },
        "/titles/103",
      ),
    );
    await drawn(".more-actions__button");
    expect(await unreachedInMain()).toEqual([]);
    const more = document.querySelector<HTMLElement>(".more-actions__button");
    more?.focus();
    await press("{Enter}");
    await drawn('dialog .list-box [role="button"]');
    expect(focusedName()).toBe("Identified incorrectly");
    await press("{Enter}");
    await drawn('dialog[aria-label="Identified incorrectly"]');
    expect(document.querySelectorAll("dialog")).toHaveLength(1);
    expect(await pressUntil("{ArrowDown}", () => focusedName() === "Enter manually")).toBe(true);
    await press("{Enter}");
    await drawn('dialog[aria-label="Identified incorrectly"] li button');
    const inDialog = focusTargetsIn(
      document.querySelector('dialog[aria-label="Identified incorrectly"]') ?? document,
    );
    const first = inDialog[0];
    const reached = first === undefined ? new Set<Element>() : await reachedFrom(first, press);
    expect(
      inDialog.filter((control) => !reached.has(control)).map((each) => each.outerHTML),
    ).toEqual([]);
    // Back closes it, and focus is on the "…" again (D361 §3). Pressed from the dialog's first
    // control: the walk may end in its search field, where Backspace deletes and is no Back.
    first?.focus();
    expect(caretOf(document.activeElement ?? document.body)).toBeUndefined();
    await press("{Backspace}");
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    // Given back as the dialog's effects end, which may be a moment after it leaves the page.
    await expect.poll(() => document.activeElement).toBe(more);
  });

  // D367 §9: one series page, its season a view picked from its row of seasons' posters (D379),
  // which adds no step to Back.
  it("reaches every control of a series' page, and picks a season in its place", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /items/20": glassTide }, "/series/20"),
    );
    await drawn(".title__play");
    // Its order chosen under its "…", rarely used (the owner, 2026-10-29), not on the page.
    expect(document.querySelector("main select")).toBeNull();
    expect(document.querySelector("main .more-actions__button")).not.toBeNull();
    expect(await unreachedInMain()).toEqual([]);
    await press("{ArrowDown}");
    expect(await pressUntil("{ArrowDown}", () => isFocusIn(".series__seasons"))).toBe(true);
    expect(await pressUntil("{ArrowRight}", () => isFocusOnLinkTo("/series/20?season=2"))).toBe(
      true,
    );
    await press("{Enter}");
    await expect
      .poll(() => walking?.services.navigation.route())
      .toEqual({
        screen: "series-page",
        collectionId: 20,
        season: 2,
      });
    expect(walking.services.navigation.behind()).toEqual([]);
    await expect.poll(() => document.querySelector("main h2")?.textContent).toBe("Season 2");
  });

  // The owner on a Fire TV, 2026-10-28: "Left/right didn't close it", focus on the Close. The banner's
  // Libraries menu closes as any chooser does, and scrolls within its height (the owner, 2026-10-28).
  it("closes the banner's Libraries menu by Left and by Right from its first place, focus on its icon, its places scrolling within its height", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(manyLibraries, "/"));
    await drawn('.app-shell__watching [role="button"][aria-label="Libraries"]');
    const icon = document.querySelector<HTMLElement>(
      '.app-shell__watching [role="button"][aria-label="Libraries"]',
    );
    icon?.focus();
    for (const arrow of ["{ArrowLeft}", "{ArrowRight}"]) {
      await press("{Enter}");
      await drawn("dialog a");
      expect(await pressUntil("{ArrowUp}", isOnFirstPlace), arrow).toBe(true);
      await press(arrow);
      await expect.poll(() => document.querySelector("dialog"), { message: arrow }).toBeNull();
      await expect.poll(() => document.activeElement, { message: arrow }).toBe(icon);
    }
    // Its places, more than the screen holds, scroll within it; Down reaches the last, All
    // libraries, shown whole within the list.
    await press("{Enter}");
    await drawn("dialog a");
    const places = document.querySelector<HTMLElement>("dialog .list-box");
    expect(isScrollingWithin(places)).toBe(true);
    expect(await pressUntil("{ArrowDown}", () => isFocusOnLinkTo("/browse?library=all"), 24)).toBe(
      true,
    );
    expect(isShownWholeIn(document.activeElement, places)).toBe(true);
    expect(walking.services.navigation.route()).toEqual({ screen: "home" });
  });

  // The plan's first wave, item 5 (the owner, 2026-11-04, "Agreed: (a)"): Up from any poster of a
  // full first row, nothing in the page straight above it, reaches the list's tools before the bar.
  it.each(["/films", "/browse?library=1"])(
    "goes Up from every poster of a full first row on %s to its tools, its Sort and filter among them",
    async (path) => {
      await page.viewport(width, height);
      walking = await mountApp(
        walkServices({ ...aLibrary, "GET /items": { items: films(12), nextCursor: "" } }, path),
      );
      await drawn("main .list-criteria__button");
      await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
      const posters = [
        ...document.querySelectorAll<HTMLElement>(".poster-grid .poster-card__link"),
      ];
      const topOf = (element: Element | undefined): number | undefined =>
        element?.getBoundingClientRect().top;
      const firstRow = posters.filter((each) => topOf(each) === topOf(posters[0]));
      expect(firstRow.length).toBeLessThan(posters.length);
      for (const poster of firstRow) {
        poster.focus();
        await press("{ArrowUp}");
        expect(isFocusIn("main .browse__bar"), poster.textContent).toBe(true);
      }
      // The Sort and filter, at the tools' right end, is one arrow along them.
      expect(await pressUntil("{ArrowRight}", () => isFocusIn(".list-criteria__button"))).toBe(
        true,
      );
    },
  );
});
