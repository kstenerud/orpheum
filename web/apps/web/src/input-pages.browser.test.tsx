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
import { createSession } from "./session.ts";
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
    await drawn(".poster-grid__card");
    await press("{ArrowDown}");
    expect(await pressUntil("{ArrowUp}", () => isFocusIn(".app-shell__banner"))).toBe(true);
    expect(await pressUntil("{ArrowRight}", () => focusedName() === "Menu")).toBe(true);
    await press("{Enter}");
    expect(document.querySelector("details")?.open).toBe(true);
    expect(await pressUntil("{ArrowDown}", () => isFocusOnLinkTo("/this-device"))).toBe(true);
    await press("{Enter}");
    await drawn("#this-device-heading");
    await press("{Escape}");
    await drawn(".poster-grid__card");
    await expect.poll(focusedName).toBe("Menu");
    expect(document.querySelector("details")?.open).toBe(false);
  });

  it("closes the open menu by Back, keeping its icon, before going anywhere", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(signedIn));
    await drawn(".poster-grid__card");
    await press("{ArrowDown}");
    await pressUntil("{ArrowUp}", () => isFocusIn(".app-shell__banner"));
    await pressUntil("{ArrowRight}", () => focusedName() === "Menu");
    await press("{Enter}{ArrowDown}");
    expect(isFocusIn("details ul")).toBe(true);
    await press("{Escape}");
    expect(document.querySelector("details")?.open).toBe(false);
    expect(focusedName()).toBe("Menu");
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

  it("reaches every control of an admin's film page, Identified incorrectly's chooser opened by OK", async () => {
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
    await drawn(".match-correction button");
    document.querySelector<HTMLElement>(".match-correction button")?.focus();
    await press("{Enter}");
    await drawn(".match-correction li button");
    expect(await unreachedInMain()).toEqual([]);
  });

  it("reaches every control of a series' page, opens a season and goes back to it", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /collections/20": glassTide }, "/series/20"),
    );
    await drawn(".title__play");
    expect(document.querySelector("main select")).not.toBeNull();
    expect(await unreachedInMain()).toEqual([]);
    await press("{ArrowDown}");
    expect(await pressUntil("{ArrowDown}", () => isFocusOnLinkTo("/series/20?season=1"))).toBe(
      true,
    );
    await press("{Enter}");
    await expect
      .poll(() => walking?.services.navigation.route())
      .toEqual({
        screen: "series-page",
        collectionId: 20,
        season: 1,
      });
    await press("{Escape}");
    await expect.poll(() => isFocusOnLinkTo("/series/20?season=1")).toBe(true);
  });

  it("types a search, arrows down into its results, opens one and goes back to it", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices(
        {
          ...signedIn,
          "GET /search": {
            results: [
              { id: 101, kind: "movie", name: "Metropolis", year: 1995 },
              { id: 102, kind: "movie", name: "Metropolis Wave", year: 2001 },
            ],
          },
          "GET /items/102": filmDetail(102, "Metropolis Wave"),
        },
        "/search",
      ),
    );
    await drawn("input[type=search]");
    await press("{ArrowDown}");
    expect(document.activeElement?.getAttribute("type")).toBe("search");
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
