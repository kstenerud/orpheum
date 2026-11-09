// SPDX-License-Identifier: MPL-2.0

/**
 * The app's pages walked with a remote's keys in a real browser, at a Fire TV's page in Silk (960 ×
 * 480 CSS pixels below its address bar) and a desktop's window (controller-input-2026-10-25.md
 * §6.3): the banner's menu, a film's and a series' pages, search, This device and sign-in, each
 * arrived at, every control on it reached by arrows, one chosen, and Back finding focus where it was.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { chooseOk, okChoice } from "./controller-choice.ts";
import { caretOf, focusTargetsIn } from "./focus-engine.ts";
import { closeTrayDialog } from "./tray-dialog.ts";
import { createNavigation } from "./navigation.ts";
import { createSession } from "./session.ts";
import { fakePlace } from "./test-services.tsx";
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
  type WalkAnswers,
  pressesUntil,
} from "./test-walk.tsx";

/** The screens the walks are made at: a Fire TV's page in Silk, and a desktop's window. */
const screens = [
  { name: "a Fire TV's page in Silk", width: 960, height: 480 },
  { name: "a desktop's window", width: 1280, height: 800 },
] as const;

/** The server's answers every signed-in page reads. */
const signedIn: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": { continueWatching: [] },
  "GET /items": { items: films(6), nextCursor: "" },
};

/** A series of two seasons in two orderings, its next episode the second. */
const glassTide = {
  id: 20,
  kind: "series",
  name: "Glass Tide",
  libraryId: 1,
  art: {},
  orderings: [
    { name: "aired", kind: "aired", episodeCount: 4 },
    { name: "tmdb:dvd", kind: "dvd", label: "DVD Order", listedCount: 4, episodeCount: 4 },
  ],
  seasons: [
    { seasonNumber: 1, episodeCount: 3 },
    { seasonNumber: 2, episodeCount: 1 },
  ],
  nextUp: { itemId: 302, name: "Low Water", groupNumber: 1, numberInGroup: 2, fileId: 3020 },
};

/** A films library, as GET /libraries lists it. */
function filmsLibrary(id: number, name: string): Record<string, unknown> {
  return {
    id,
    name,
    kind: "film",
    metadataSource: "nfo_then_tmdb",
    changeDetection: "watch",
    isWritable: true,
    createdAt: 0,
  };
}

/** A person's one films library, its page listing six films. */
const aLibrary: WalkAnswers = {
  ...signedIn,
  "GET /libraries": { libraries: [filmsLibrary(1, "Films")] },
  "GET /items/letters": { letters: [] },
};

/** Sixteen libraries, more than a Fire TV's screen holds in the banner's Libraries menu. */
const manyLibraries: WalkAnswers = {
  ...signedIn,
  "GET /libraries": {
    libraries: Array.from({ length: 16 }, (_, at) =>
      filmsLibrary(at + 1, `Library ${String(at + 1).padStart(2, "0")}`),
    ),
  },
};

/** count of the person's devices, as GET /me/devices lists them, the first this one. */
function devices(count: number): Record<string, unknown>[] {
  return Array.from({ length: count }, (_, at) => ({
    id: at + 1,
    name: `Device ${String(at + 1)}`,
    platform: "web",
    addedAt: 0,
    lastUsedAt: 0,
    isSignedIn: true,
    isByCode: false,
    isThisDevice: at === 0,
  }));
}

/** A list's Sort and filter button, on its bar (listing-tools.tsx, D417). */
function criteriaButton(): HTMLElement | undefined {
  return document.querySelector<HTMLElement>("main .list-criteria__button") ?? undefined;
}

/** The sort's chooser in the open Sort and filter: its button, found by the field it chooses. */
function sortChooser(): HTMLElement | undefined {
  const chooser = [...document.querySelectorAll(".list-criteria .route-chooser")].find(
    (each) => each.querySelector(".route-chooser__field")?.textContent === "Sort",
  );
  return chooser?.querySelector<HTMLElement>(".route-chooser__button") ?? undefined;
}

/** Whether element holds more than it shows, scrolling within its own height. */
function isScrollingWithin(element: HTMLElement | null): boolean {
  return element !== null && element.scrollHeight > element.clientHeight;
}

/** Whether element is drawn whole inside container's box, to half a pixel. */
function isShownWholeIn(element: Element | null, container: Element | null): boolean {
  if (element === null || container === null) {
    return false;
  }
  const [box, within] = [element.getBoundingClientRect(), container.getBoundingClientRect()];
  return box.top >= within.top - 0.5 && box.bottom <= within.bottom + 0.5;
}

/** Presses keys as a remote does: real key events, the browser's own handling with them. */
async function press(keys: string): Promise<void> {
  await userEvent.keyboard(keys);
}

/** Presses a key until a test holds, at most 12 times; whether it held. */
const pressUntil = pressesUntil(press);

/** Waits until the page draws selector. */
async function drawn(selector: string): Promise<void> {
  await expect.poll(() => isDrawn(selector)).toBe(true);
}

/**
 * The controls of the page's main content no arrow reaches from its arrival, where focus is as it
 * opens (ui-audit-2026-11-04.md R3), or, on a page that has none, from the first an arrow starts at,
 * by their markup's start; none on a page a remote can use.
 */
async function unreachedInMain({ hasArrival = true } = {}): Promise<string[]> {
  if (hasArrival) {
    await expect.poll(() => isFocusIn("main")).toBe(true);
  } else {
    await press("{ArrowDown}");
  }
  return await unreachedIn("main");
}

/**
 * The controls in what selector finds that the arrows never reach from what has focus there, as
 * their markup's start; one entry saying so where focus is not in it.
 */
async function unreachedIn(selector: string): Promise<string[]> {
  const first = document.activeElement;
  const within = document.querySelector(selector);
  if (!(first instanceof HTMLElement) || within?.contains(first) !== true) {
    return [`the first arrow reached nothing in ${selector}`];
  }
  const reached = await reachedFrom(first, press);
  return focusTargetsIn(within)
    .filter((control) => !reached.has(control))
    .map((control) => control.outerHTML.slice(0, 80));
}

/** Whether what has focus is the first place of an open drop-down's list. */
function isOnFirstPlace(): boolean {
  const first = document.querySelector("dialog .list-box a, dialog .list-box button");
  return first !== null && document.activeElement === first;
}

/** Whether what has focus is a link to href. */
function isFocusOnLinkTo(href: string): boolean {
  return document.activeElement?.getAttribute("href") === href;
}

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
    await drawn("dialog .list-box button");
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
    await drawn("dialog .list-box button");
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
    await drawn("dialog .list-box button");
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
      await drawn("dialog .list-box button");
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

  // The owner on a Fire TV, 2026-10-28: "Left/right didn't close it", focus on the Close. The banner's
  // Libraries menu closes as any chooser does, and scrolls within its height (the owner, 2026-10-28).
  it("closes the banner's Libraries menu by Left and by Right from its first place, focus on its icon, its places scrolling within its height", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(manyLibraries, "/"));
    await drawn('.app-shell__watching button[aria-label="Libraries"]');
    const icon = document.querySelector<HTMLElement>(
      '.app-shell__watching button[aria-label="Libraries"]',
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
                name: "Metropolis Wave",
                year: 2001,
                identificationState: "identified",
                addedAt: 2,
              },
            ],
            nextCursor: "",
          },
          "GET /items/102": filmDetail(102, "Metropolis Wave"),
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
    const isCriteriaButton = (control: Element): control is HTMLButtonElement =>
      control instanceof HTMLButtonElement && control.classList.contains("list-criteria__button");
    const button = [...reached].find(isCriteriaButton);
    expect(button).toBeDefined();
    button?.focus();
    await press("{Enter}");
    await drawn("dialog input.list-criteria__name");
    expect(document.activeElement?.classList.contains("list-criteria__name")).toBe(true);
    await press("heat");
    // Closed, what was typed goes to the address (useListCriteria), the list narrowed by it.
    await press("{Escape}");
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(button);
    await expect
      .poll(() => walking?.services.navigation.route())
      .toMatchObject({ screen: "browse", library: 1, query: "heat" });
    // The list asks for the text once typing rests (listing-tools.tsx typingRestMs), drawn anew; the
    // first, unnarrowed, also has 102, and under load replaced it under focus (2026-11-05).
    await expect.poll(() => first?.isConnected).toBe(false);
    await drawn('main a[href="/titles/102"]');
    // Down from the button lands on the poster nearest below it, which its summary's width decides
    // ("Name · “heat”" once the address has the text): either result, then Right along the row.
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
    await drawn("dialog .list-box button");
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

  it("chooses a controller's OK on This device by arrows and OK, every control reached", async () => {
    chooseOk("automatic");
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /me/devices": { devices: [] } }, "/this-device"),
    );
    await drawn('input[value="right"]');
    try {
      // No device listed, so no Rename to arrive at.
      expect(await unreachedInMain({ hasArrival: false })).toEqual([]);
      document.querySelector<HTMLElement>('input[value="automatic"]')?.focus();
      expect(
        await pressUntil(
          "{ArrowDown}",
          () => document.activeElement?.getAttribute("value") === "right",
        ),
      ).toBe(true);
      await press("{Enter}");
      expect(okChoice()).toBe("right");
    } finally {
      chooseOk("automatic");
    }
  });

  it("goes Down from This device's Rename, its arrival, to the controller's choices", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /me/devices": { devices: devices(1) } }, "/this-device"),
    );
    await expect.poll(focusedName).toBe("Rename");
    expect(await unreachedInMain()).toEqual([]);
    document.querySelector<HTMLElement>(".your-devices__name button")?.focus();
    expect(
      await pressUntil(
        "{ArrowDown}",
        () => document.activeElement?.getAttribute("type") === "radio",
      ),
    ).toBe(true);
  });

  it("goes Right on Your account from the code, its arrival, to the name and Activate, Down to every control, the devices past five behind All devices", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /me/devices": { devices: devices(7) } }, "/account"),
    );
    await drawn("details.your-devices__all");
    await expect
      .poll(() => document.activeElement?.classList.contains("device-activation__code-field"))
      .toBe(true);
    expect(await unreachedInMain()).toEqual([]);
    document.querySelector<HTMLElement>(".device-activation__code-field")?.focus();
    await press("{ArrowRight}");
    // The device's name: the field beside the code.
    expect(document.activeElement?.getAttribute("maxlength")).toBe("100");
    await press("{ArrowRight}");
    expect(focusedName()).toBe("Activate");
    expect(document.querySelectorAll(".your-devices__device")).toHaveLength(5);
    expect(await pressUntil("{ArrowDown}", () => focusedName() === "All devices (7)", 30)).toBe(
      true,
    );
  });

  it("reaches every control of the sign-in page and types in its fields", async () => {
    await page.viewport(width, height);
    const services = walkServices({ "GET /client-settings": {} });
    const signedOut = { ...services, session: createSession(undefined) };
    walking = await mountApp(signedOut);
    await drawn("input[name=name]");
    expect(await unreachedInMain()).toEqual([]);
    document.querySelector<HTMLElement>("input[name=name]")?.focus();
    await press("karl");
    expect(document.querySelector<HTMLInputElement>("input[name=name]")?.value).toBe("karl");
    // Down leaves the one-line field for the next, as on a TV's form.
    await press("{ArrowDown}");
    expect(document.activeElement?.getAttribute("type")).toBe("password");
  });

  it("starts Watch Together by OK in its dialog, opened over home from its old address, reaches every control of its session, and Back closes its invitation (D384 §2)", async () => {
    window.sessionStorage.clear();
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices(
        {
          ...signedIn,
          "GET /me/together": { sessions: [] },
          "POST /together": { sessionId: 3, membershipCredential: "host-credential", isNew: true },
          "GET /together/3": {
            sessionId: 3,
            nextRights: { canPlayPause: true, canSeek: true },
            links: [{ id: 4, rights: { canPlayPause: true, canSeek: false }, hostNote: "Grandma" }],
            members: [{ id: 1, name: "Karl", isHost: true }],
          },
        },
        "/watch-together",
      ),
    );
    // Its code loads as the dialog is first shown (later-screens.tsx).
    await drawn('dialog[aria-label="Watch Together"][open]');
    expect(walking.services.navigation.route()).toEqual({ screen: "home" });
    try {
      expect(await pressUntil("{ArrowDown}", () => focusedName() === "Start a session")).toBe(true);
      await press("{Enter}");
      await expect.poll(() => document.body.textContent).toContain("Grandma");
      await press("{ArrowDown}");
      expect(await unreachedIn('dialog[aria-label="Watch Together"]')).toEqual([]);
      const invite = [
        ...document.querySelectorAll<HTMLElement>('dialog[aria-label="Watch Together"] button'),
      ].find((button) => button.textContent === "Invite someone");
      invite?.focus();
      await press("{Enter}");
      await expect.poll(() => document.querySelectorAll("dialog[open]").length).toBe(2);
      await press("{ArrowDown}{ArrowDown}");
      expect(isFocusIn(".watch-together__dialog")).toBe(true);
      await press("{Escape}");
      await expect.poll(() => document.querySelectorAll("dialog[open]").length).toBe(1);
      expect(walking.services.navigation.route()).toEqual({ screen: "home" });
    } finally {
      window.sessionStorage.clear();
      closeTrayDialog(); // the next test's app opens with none
    }
  });
});
