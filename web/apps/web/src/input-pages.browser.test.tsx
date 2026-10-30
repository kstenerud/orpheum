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
import { focusTargetsIn } from "./focus-engine.ts";
import { createNavigation } from "./navigation.ts";
import { createSession } from "./session.ts";
import { fakePlace } from "./test-services.tsx";
import {
  filmDetail,
  films,
  focusedName,
  isDrawn,
  isFocusIn,
  mountApp,
  reachedFrom,
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

/** The server's answers every signed-in page reads. */
const signedIn: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": { continueWatching: [] },
  "GET /titles": { titles: films(6), nextCursor: "" },
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
  nextUp: { titleId: 302, name: "Low Water", groupNumber: 1, numberInGroup: 2, fileId: 3020 },
};

/** Presses keys as a remote does: real key events, the browser's own handling with them. */
async function press(keys: string): Promise<void> {
  await userEvent.keyboard(keys);
}

/** Presses key until isThere, at most times times; whether it got there. */
async function pressUntil(key: string, isThere: () => boolean, times = 12): Promise<boolean> {
  for (let pressed = 0; pressed < times && !isThere(); pressed += 1) {
    await press(key);
  }
  return isThere();
}

/** Waits until the page draws selector. */
async function drawn(selector: string): Promise<void> {
  await expect.poll(() => isDrawn(selector)).toBe(true);
}

/**
 * The controls of the page's main content no arrow reaches from the first an arrow starts at, by
 * their markup's start; none on a page a remote can use.
 */
async function unreachedInMain(): Promise<string[]> {
  await press("{ArrowDown}");
  const first = document.activeElement;
  const main = document.querySelector("main");
  if (!(first instanceof HTMLElement) || main?.contains(first) !== true) {
    return ["the first arrow reached nothing in the page"];
  }
  const reached = await reachedFrom(first, press);
  return focusTargetsIn(main)
    .filter((control) => !reached.has(control))
    .map((control) => control.outerHTML.slice(0, 80));
}

/** Whether what has focus is an open drop-down's Close. */
function isOnClose(): boolean {
  return document.activeElement?.classList.contains("drop-down__close") === true;
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
    await drawn(".poster-grid .poster-card__link");
    await press("{ArrowDown}");
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
    await drawn(".poster-grid .poster-card__link");
    await press("{ArrowDown}");
    await pressUntil("{ArrowUp}", () => isFocusIn(".app-shell__banner"));
    await pressUntil("{ArrowRight}", () => focusedName() === "Menu");
    await press("{Enter}{ArrowDown}");
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
    await drawn(".poster-grid .poster-card__link");
    await press("{ArrowDown}");
    await pressUntil("{ArrowUp}", () => isFocusIn(".app-shell__banner"));
    await pressUntil("{ArrowRight}", () => focusedName() === "Menu");
    await press("{Enter}{ArrowDown}");
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
    await drawn(".poster-grid .poster-card__link");
    await press("{ArrowDown}");
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
    await drawn("dialog .drop-down__places button");
    expect(focusedName()).toBe("Identified incorrectly");
    await press("{Enter}");
    await drawn("dialog.match-correction");
    expect(document.querySelectorAll("dialog")).toHaveLength(1);
    expect(await pressUntil("{ArrowDown}", () => focusedName() === "Enter manually")).toBe(true);
    await press("{Enter}");
    await drawn(".match-correction li button");
    const inDialog = focusTargetsIn(document.querySelector("dialog.match-correction") ?? document);
    const first = inDialog[0];
    const reached = first === undefined ? new Set<Element>() : await reachedFrom(first, press);
    expect(
      inDialog.filter((control) => !reached.has(control)).map((each) => each.outerHTML),
    ).toEqual([]);
    // Back closes it, and focus is on the "…" again (D361 §3).
    await press("{Backspace}");
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    // Given back as the dialog's effects end, which may be a moment after it leaves the page.
    await expect.poll(() => document.activeElement).toBe(more);
  });

  // D367 §9: one series page, its season a view picked by a chooser, which adds no step to Back.
  it("reaches every control of a series' page, and picks a season in its place", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /collections/20": glassTide }, "/series/20"),
    );
    await drawn(".title__play");
    // Its order chosen under its "…", rarely used (the owner, 2026-10-29), not on the page.
    expect(document.querySelector("main select")).toBeNull();
    expect(document.querySelector("main .more-actions__button")).not.toBeNull();
    expect(await unreachedInMain()).toEqual([]);
    await press("{ArrowDown}");
    const isOnPicker = (): boolean =>
      document.activeElement?.classList.contains("route-chooser__button") === true;
    expect(await pressUntil("{ArrowDown}", isOnPicker)).toBe(true);
    await press("{Enter}");
    await drawn("dialog a");
    expect(await pressUntil("{ArrowDown}", () => isFocusOnLinkTo("/series/20?season=2"))).toBe(
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

  it("closes a season picker's list by the browser's own Back, a history step, choosing nothing, and by its Close", async () => {
    await page.viewport(width, height);
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "series-page", collectionId: 20 });
    walking = await mountApp({
      ...walkServices({ ...signedIn, "GET /collections/20": glassTide }),
      navigation,
    });
    await drawn(".title__play");
    const picker = document.querySelector<HTMLElement>("main .route-chooser__button");
    // The chooser shows its chevron, the menu none (the owner, 2026-10-28).
    expect(picker?.querySelector(".route-chooser__chevron")).not.toBeNull();
    expect(document.querySelector(".app-shell__menu-button .route-chooser__chevron")).toBeNull();
    picker?.focus();
    await press("{Enter}");
    await drawn("dialog a");
    place.history.back();
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(picker);
    expect(navigation.route()).toEqual({ screen: "series-page", collectionId: 20 });
    expect(navigation.behind()).toEqual([{ screen: "home" }]);
    // Its Close, the cross at its top-right, reached by the arrows.
    await press("{Enter}");
    await drawn("dialog a");
    expect(
      await pressUntil(
        "{ArrowUp}",
        () => document.activeElement?.classList.contains("drop-down__close") === true,
      ),
    ).toBe(true);
    await press("{Enter}");
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(picker);
    expect(navigation.route()).toEqual({ screen: "series-page", collectionId: 20 });
  });

  // The owner on a Fire TV, 2026-10-28: the season picker "shifted the page up before displaying
  // it", and opened on its Close, not a season.
  it("opens a season picker mid-page on its season, the page moving neither as it opens, its Close reached, nor as it closes", async () => {
    await page.viewport(width, height);
    const above = document.createElement("div");
    above.style.height = `${String(height * 2)}px`;
    document.body.prepend(above);
    // Seasons enough that the list is as tall as the screen lets it be, its Close under the banner.
    const seasons = Array.from({ length: 16 }, (_, at) => ({
      seasonNumber: at + 1,
      episodeCount: 1,
    }));
    try {
      walking = await mountApp(
        walkServices(
          { ...signedIn, "GET /collections/20": { ...glassTide, seasons } },
          "/series/20",
        ),
      );
      await drawn(".title__play");
      const picker = document.querySelector<HTMLElement>("main .route-chooser__button");
      if (picker === null) {
        throw new Error("the series' page draws no season picker");
      }
      window.scrollBy({ top: picker.getBoundingClientRect().top - height / 2 });
      const scrolled = window.scrollY;
      expect(scrolled).toBeGreaterThan(0);
      picker.focus({ preventScroll: true });
      await press("{Enter}");
      await drawn("dialog a");
      expect(isFocusIn("dialog ul")).toBe(true);
      expect(document.activeElement?.getAttribute("aria-current")).toBe("page");
      expect(window.scrollY).toBe(scrolled);
      expect(await pressUntil("{ArrowUp}", isOnClose)).toBe(true);
      expect(window.scrollY).toBe(scrolled);
      await press("{Escape}");
      await expect.poll(() => document.querySelector("dialog")).toBeNull();
      expect(document.activeElement).toBe(picker);
      expect(window.scrollY).toBe(scrolled);
    } finally {
      above.remove();
    }
  });

  // The owner on a Fire TV, 2026-10-28: "Left/right didn't close it", focus on the Close.
  it("closes a season picker's list by Left and by Right from its Close, focus on the picker", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /collections/20": glassTide }, "/series/20"),
    );
    await drawn(".title__play");
    const picker = document.querySelector<HTMLElement>("main .route-chooser__button");
    picker?.focus();
    for (const arrow of ["{ArrowLeft}", "{ArrowRight}"]) {
      await press("{Enter}");
      await drawn("dialog a");
      expect(await pressUntil("{ArrowUp}", isOnClose), arrow).toBe(true);
      await press(arrow);
      await expect.poll(() => document.querySelector("dialog"), { message: arrow }).toBeNull();
      await expect.poll(() => document.activeElement, { message: arrow }).toBe(picker);
    }
    expect(walking.services.navigation.route()).toEqual({
      screen: "series-page",
      collectionId: 20,
    });
  });

  it("types a search on the Libraries page, arrows into its results, opens one and goes back", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices(
        {
          ...signedIn,
          "GET /libraries": {
            libraries: [
              {
                id: 1,
                name: "Films",
                kind: "movies",
                metadataSource: "nfo_then_tmdb",
                changeDetection: "watch",
                isWritable: true,
                createdAt: 0,
              },
            ],
          },
          "GET /titles/letters": { letters: [] },
          "GET /search": {
            results: [
              { id: 101, kind: "movie", name: "Metropolis", year: 1995 },
              { id: 102, kind: "movie", name: "Metropolis Wave", year: 2001 },
            ],
          },
          "GET /items/102": filmDetail(102, "Metropolis Wave"),
        },
        "/browse?library=1",
      ),
    );
    await drawn("input[type=search]");
    await press("{ArrowDown}");
    const first = document.activeElement;
    expect(first).toBeInstanceOf(HTMLElement);
    const reached: Set<Element> =
      first instanceof HTMLElement ? await reachedFrom(first, press) : new Set();
    const isSearchInput = (control: Element): control is HTMLInputElement =>
      control instanceof HTMLInputElement && control.type === "search";
    const searchInput = [...reached].find(isSearchInput);
    expect(searchInput).toBeDefined();
    searchInput?.focus();
    await press("heat{Enter}");
    await drawn('main a[href="/titles/102"]');
    expect(await pressUntil("{ArrowDown}", () => isFocusOnLinkTo("/titles/102"))).toBe(true);
    await press("{Enter}");
    await drawn(".title__play");
    await press("{Escape}");
    await expect.poll(() => isFocusOnLinkTo("/titles/102")).toBe(true);
  });

  it("chooses a controller's OK on This device by arrows and OK, every control reached", async () => {
    chooseOk("automatic");
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /me/devices": { devices: [] } }, "/this-device"),
    );
    await drawn('input[value="right"]');
    try {
      expect(await unreachedInMain()).toEqual([]);
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

  it("starts Watch Together by OK, reaches every control of its session, and Back closes its invitation", async () => {
    window.sessionStorage.clear();
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices(
        {
          ...signedIn,
          "GET /together/mine": { sessions: [] },
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
    // Its code loads as the screen is first shown (later-screens.tsx).
    await drawn(".watch-together");
    try {
      await press("{ArrowDown}");
      expect(await pressUntil("{ArrowDown}", () => focusedName() === "Start a session")).toBe(true);
      await press("{Enter}");
      await expect.poll(() => document.body.textContent).toContain("Grandma");
      expect(await unreachedInMain()).toEqual([]);
      const invite = [...document.querySelectorAll<HTMLElement>("main button")].find(
        (button) => button.textContent === "Invite someone",
      );
      invite?.focus();
      await press("{Enter}");
      await drawn("dialog[open]");
      await press("{ArrowDown}{ArrowDown}");
      expect(isFocusIn("dialog[open]")).toBe(true);
      await press("{Escape}");
      await expect.poll(() => isDrawn("dialog[open]")).toBe(false);
      expect(walking.services.navigation.route()).toEqual({ screen: "watch-together" });
    } finally {
      window.sessionStorage.clear();
    }
  });
});
