// SPDX-License-Identifier: MPL-2.0

/**
 * The whole signed-in app for a browser-mode walk with a remote's keys or a controller
 * (input-walk.browser.test.tsx): the client as client-main.tsx mounts it, its API answered by a
 * stand-in server that answers what the walk gives and refuses the rest as not found, so a page's
 * every background read is answered at once and none reaches the network (AGENTS.md rule 16).
 */

import { apiPrefix, waitMs } from "@orpheum/sdk";
import { act } from "react";
import type { expect as vitestExpect } from "vitest";
import { createRoot } from "react-dom/client";

import { railLetters } from "./alphabet-rail.tsx";
import { App } from "./app.tsx";
import { focusTargetsIn } from "./focus-engine.ts";
import { focusRegion } from "./focus-navigation.ts";
// The screens the client loads as they are first shown (later-screens.tsx), loaded with the walk as
// they were before they were split: a walk's time is its own, not the dev server's first build of a
// screen, which under load took the player's walk past the 15 s a test has (2026-10-25).
import "./cast-remote.tsx";
import "./lab.tsx";
import "./lab-extend-player.tsx";
import "./play.tsx";
import "./series.tsx";
import "./settings.tsx";
import "./title.tsx";
import "./together-viewer.tsx";
import "./ui-gallery.tsx";
import "./watch-together.tsx";
import { resetLazyScreensOnMount } from "./screen-load.tsx";
import { ServicesContext, type Services } from "./services.tsx";
import { testServices } from "./test-services.tsx";
import { closeTrayDialog } from "./tray-dialog.ts";

/** What the stand-in server answers, by "METHOD /path", the path as the API names it. */
export type WalkAnswers = Readonly<Record<string, unknown>>;

/** A person signed in with no admin's rights, who may host Watch Together. */
export const viewer = {
  id: 1,
  name: "Karl",
  role: "viewer",
  isAdmin: false,
  canHostWatchTogether: true,
  canManageAccount: true,
  hasNoLibrary: false,
};

/** Films for a list, Film 1 to Film count, ids from 101. */
export function films(count: number): Record<string, unknown>[] {
  return Array.from({ length: count }, (_, index) => ({
    id: 101 + index,
    kind: "film",
    name: `Film ${String(index + 1)}`,
    year: 2000 + index,
  }));
}

/**
 * films(count), each under a rail letter as a list by name says it, in the rail's order: Film 1
 * under #, Film 2 under A, and Film 27 and every one after it under Z. The rail shows only the
 * letters its list's entries are under (D475 §2), so a list of 27 or more has the whole rail.
 */
export function letteredFilms(count: number): Record<string, unknown>[] {
  return films(count).map((film, index) => ({ ...film, letter: railLetterAt(index) }));
}

/** The rail letter of the entry at index in a stand-in list by name (letteredFilms). */
export function railLetterAt(index: number): string {
  return railLetters[Math.min(index, railLetters.length - 1)] ?? "#";
}

/** A film's page as the server sends it, with one file to play and one extra. */
export function filmDetail(id: number, name: string): Record<string, unknown> {
  return {
    id,
    kind: "film",
    name,
    year: 2001,
    identificationState: "identified",
    libraryId: 1,
    art: {},
    versions: [{ id: 1, files: [{ id: id * 10, path: `/m/${name}.mkv`, streams: [] }] }],
    extras: [{ id: id + 1000, name: `${name} trailer`, extraType: "trailer" }],
  };
}

/**
 * Services whose server answers answers by path, and every other request as not found; each request
 * is added to heard as "METHOD /path".
 */
export function walkServices(
  answers: WalkAnswers,
  path = "/films",
  heard: string[] = [],
): Services {
  return testServices("token", path, async (request) => {
    await Promise.resolve();
    const url = new URL(request.url);
    const methodAndPath = `${request.method} ${url.pathname.slice(apiPrefix.length)}`;
    heard.push(methodAndPath);
    if (!(methodAndPath in answers)) {
      return Response.json(
        { error: { code: "notFound", message: "the walk serves no such thing" } },
        { status: 404 },
      );
    }
    const body = answers[methodAndPath];
    return body === null
      ? new Response(null, { status: 204 })
      : Response.json(body, { status: 200 });
  });
}

/** The app mounted, and taking it down. */
export interface Walking {
  readonly services: Services;
  readonly unmount: () => void;
}

/**
 * Mounts the whole client over services, as client-main.tsx does, with no tray dialog open: which is
 * open is the tab's (tray-dialog.ts), so one an earlier walk left open, Cast's, would otherwise open
 * over this one's page and hold its focus; and with the tab's state reset (resetLazyScreensOnMount),
 * so a page never shows, while it loads, what an earlier walk's page left under the same id (Firefox,
 * 2026-11-04). test-dom.tsx's mount does the same.
 */
export async function mountApp(services: Services): Promise<Walking> {
  resetLazyScreensOnMount();
  closeTrayDialog();
  Reflect.set(globalThis, "IS_REACT_ACT_ENVIRONMENT", false);
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(
      <ServicesContext value={services}>
        <App />
      </ServicesContext>,
    );
    await Promise.resolve();
  });
  return {
    services,
    unmount: () => {
      root.unmount();
      container.remove();
    },
  };
}

/** What has focus, by its text or name, for a walk's checks. */
export function focusedName(): string {
  const focused = document.activeElement;
  if (focused === null || focused === document.body) {
    return "";
  }
  return (focused.getAttribute("aria-label") ?? focused.textContent).trim();
}

/**
 * What a walk's shared keys and waits take from its test file: vitest's expect, to poll by, and its
 * keyboard, which production code may not import (web/.dependency-cruiser.cjs).
 */
export interface WalkTools {
  readonly expect: typeof vitestExpect;
  readonly keyboard: (keys: string) => Promise<void>;
}

/**
 * Presses by press: a key until isThere, at most most times (times unless said), saying whether it
 * got there. One walk's pressUntil, made once with its press.
 */
export function pressesUntil(
  press: (keys: string) => Promise<void>,
  times = 12,
): (key: string, isThere: () => boolean, most?: number) => Promise<boolean> {
  return async (key, isThere, most = times) => {
    for (let pressed = 0; pressed < most && !isThere(); pressed += 1) {
      await press(key);
    }
    return isThere();
  };
}

/** Whether what has focus is inside the element selector names. */
export function isFocusIn(selector: string): boolean {
  return document.querySelector(selector)?.contains(document.activeElement) === true;
}

/** How far a box may pass the screen's edge and still count as on it: layout's rounding. */
const roundingPx = 1;

/** Whether what has focus is wholly on the screen, as a person must see it to know where they are. */
export function isFocusInView(): boolean {
  const focused = document.activeElement;
  if (focused === null || focused === document.body) {
    return false;
  }
  const box = focused.getBoundingClientRect();
  return (
    box.top >= -roundingPx &&
    box.left >= -roundingPx &&
    box.bottom <= window.innerHeight + roundingPx &&
    box.right <= window.innerWidth + roundingPx
  );
}

/** The name of the film or series whose tile in the grid has focus, or "" where none has it. */
export function focusedTile(): string {
  const focused = document.activeElement;
  return focused?.closest(".poster-grid") === null
    ? ""
    : (focused?.querySelector(".poster-card__name")?.textContent ?? "");
}

/**
 * Whether what has focus is the first element selector names: a page's arrival, where focus goes as
 * the page opens (focus-navigation.ts arriveWhenDrawn).
 */
export function isFocusOnFirst(selector: string): boolean {
  const first = document.querySelector(selector);
  return first !== null && document.activeElement === first;
}

/** Whether the page draws an element by selector, for a walk to wait on. */
export function isDrawn(selector: string): boolean {
  return document.querySelector(selector) !== null;
}

/** The arrows a walk tries from each control it reaches. */
const arrows = ["{ArrowUp}", "{ArrowDown}", "{ArrowLeft}", "{ArrowRight}"] as const;

/**
 * How many presses of one arrow a walk makes while each only scrolls the page through text, before
 * it takes the arrow to lead nowhere more: three screens' reading.
 */
const readingPresses = 4;

/** Presses arrow until focus moves or a press scrolls nothing, at most readingPresses times. */
async function pressThroughText(
  arrow: string,
  press: (keys: string) => Promise<void>,
): Promise<void> {
  const before = document.activeElement;
  for (let pressed = 0; pressed < readingPresses; pressed += 1) {
    const scrolled = window.scrollY;
    await press(arrow);
    if (document.activeElement !== before || window.scrollY === scrolled) {
      return;
    }
  }
}

/**
 * Every element an arrow reaches from start, following each arrow from each element reached in turn,
 * pressed as a remote's keys are by press, again while a press scrolls through text: what a person
 * can get to on the page.
 */
export async function reachedFrom(
  start: HTMLElement,
  press: (keys: string) => Promise<void>,
): Promise<Set<Element>> {
  const reached = new Set<Element>([start]);
  const waiting: HTMLElement[] = [start];
  // A walk whose page is gone, its test over (timed out), presses nothing more: walking on, it
  // pressed keys into the next test's page, which then saw seeks it never made (2026-10-25).
  const pressOnItsPage = async (keys: string): Promise<void> => {
    if (start.isConnected) {
      await press(keys);
    }
  };
  for (
    let next = waiting.shift();
    next !== undefined && start.isConnected;
    next = waiting.shift()
  ) {
    for (const arrow of arrows) {
      next.focus();
      await pressThroughText(arrow, pressOnItsPage);
      const now = document.activeElement;
      if (now instanceof HTMLElement && now !== document.body && !reached.has(now)) {
        reached.add(now);
        waiting.push(now);
      }
    }
  }
  return reached;
}

// ---
/*
 * The answers, pages and walk the walks of every page by a remote's arrows share
 * (page-walk.browser.test.tsx, page-walk-manage.browser.test.tsx and
 * page-walk-person.browser.test.tsx), the walk made with vitest's own expect and keyboard, which
 * production code may not import (web/.dependency-cruiser.cjs).
 */

/** An admin's answers, so Manage's pages draw, with a films library of six films. */
export const anAdmin: WalkAnswers = {
  "GET /me": { ...viewer, role: "admin", isAdmin: true },
  "GET /client-settings": {},
  // Two films partway, so Home's walk reaches each card and its "…".
  "GET /home": {
    continueWatching: [101, 102].map((id) => ({
      id,
      name: `Film ${String(id - 100)}`,
      libraryId: 1,
      fileId: id * 10,
      positionMs: 500_000,
      durationMs: 1_000_000,
    })),
  },
  "GET /items": { items: films(6), nextCursor: "" },
  "GET /items/letters": { letters: [] },
  "GET /libraries": {
    libraries: [
      {
        id: 1,
        name: "Films",
        kind: "film",
        metadataSource: "nfoThenTmdb",
        changeDetection: "watch",
        isWritable: true,
        createdAt: 0,
      },
    ],
  },
  "GET /items/103": filmDetail(103, "Film 3"),
  "GET /items/20": {
    id: 20,
    kind: "series",
    name: "Glass Tide",
    libraryId: 1,
    art: {},
    orderings: [{ name: "aired", kind: "aired", episodeCount: 2 }],
    seasons: [
      { seasonNumber: 1, episodeCount: 2 },
      { seasonNumber: 2, episodeCount: 1 },
    ],
    nextUp: { itemId: 302, name: "Episode 2", groupNumber: 1, numberInGroup: 2, fileId: 3020 },
  },
  "GET /items/20/groups/1": {
    episodes: [1, 2].map((number) => ({
      id: 300 + number,
      seasonNumber: 1,
      episodeNumber: number,
      name: `Episode ${String(number)}`,
      identificationState: "identified",
      hasLiveFile: true,
    })),
    nextCursor: "",
  },
  // Seven, more than Your account shows before All devices; the first this one.
  "GET /me/devices": {
    devices: Array.from({ length: 7 }, (_, at) => ({
      id: 5 + at,
      name: at === 0 ? "Karl's laptop" : `Device ${String(at + 1)}`,
      platform: "web",
      addedAt: 0,
      lastUsedAt: 0,
      isSignedIn: true,
      isByCode: false,
      isThisDevice: at === 0,
    })),
  },
};

/** The pages walked, each at its address, signed in as an admin: the library's first, */
export const libraryPages: readonly string[] = [
  "/",
  "/films",
  "/series",
  "/browse?library=1",
  "/titles/103",
  "/series/20?season=1",
  "/identify",
  "/libraries",
  "/libraries/1",
];

/** then Manage's, */
export const managePages: readonly string[] = [
  "/libraries/new",
  "/libraries/1/files",
  "/about",
  "/settings",
  "/jobs",
  "/system",
  "/update",
  "/needs-attention",
  "/users",
];

/** then a person's own and the rest. */
export const personPages: readonly string[] = [
  "/account",
  "/this-device",
  "/together",
  "/users/2",
  "/watch-together",
  "/casting",
  "/cast-ready",
  "/register#a-link-secret",
  "/not-a-page",
];

/**
 * The pages this walk leaves to others, and why:
 * - /play/<file>: the player, full screen on a TV, its video needing a stream the test services
 *   cannot serve; walked by input-player.browser.test.tsx and player-widgets.browser.test.tsx.
 * - /lab and /lab-extend-player: the device lab, whose tiles take the remote's keys as what they
 *   measure, not as navigation (lab-input.browser.test.tsx).
 * - /ui-gallery: every widget laid out at once for a person to look at, each alone and some without
 *   the page around it that would reach it (a pop-up's list drawn open, the player's rows without
 *   their film); not a page to use by a remote.
 */

/** What has focus, as a failure shows it. */
function focused(): string {
  return document.activeElement?.outerHTML.slice(0, 120) ?? "nothing";
}

/** Whether the page has drawn its main content: anything to read in it. */
function isMainDrawn(): boolean {
  return (document.querySelector("main")?.textContent ?? "") !== "";
}

/**
 * The controls on the page no arrow reaches from where it starts, by tools' expect and keyboard: its
 * arrival, else the first an arrow starts at; each as its markup's start.
 */
export async function unreachedOnThePage({ expect, keyboard }: WalkTools): Promise<string[]> {
  await expect.poll(isMainDrawn).toBe(true);
  // The page's reads after it opens, and the arrival given once they are drawn.
  await waitMs(300);
  if (
    !(document.activeElement instanceof HTMLElement) ||
    document.activeElement === document.body
  ) {
    await keyboard("{ArrowDown}");
  }
  const start = document.activeElement;
  if (!(start instanceof HTMLElement) || start === document.body) {
    return [`no arrow started anywhere: ${focused()}`];
  }
  const reached = await reachedFrom(start, keyboard);
  // Where a dialog opens over the page (Watch Together's address), the page behind is inert.
  return focusTargetsIn(focusRegion())
    .filter((control) => !reached.has(control))
    .map((control) => control.outerHTML.slice(0, 100));
}

// ---
/*
 * The answers, finders and keys the walks of the app's pages with a remote's keys share
 * (input-pages.browser.test.tsx, input-pages-lists.browser.test.tsx and
 * input-pages-person.browser.test.tsx): the screens they are made at, what the server answers, and
 * the presses and waits each walk makes with vitest's own expect and keyboard, which production
 * code may not import (web/.dependency-cruiser.cjs).
 */

/** The screens the walks are made at: a Fire TV's page in Silk, and a desktop's window. */
export const screens = [
  { name: "a Fire TV's page in Silk", width: 960, height: 480 },
  { name: "a desktop's window", width: 1280, height: 800 },
] as const;

/** The server's answers every signed-in page reads. */
export const signedIn: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": { continueWatching: [] },
  "GET /items": { items: films(6), nextCursor: "" },
};

/** A series of two seasons in two orderings, its next episode the second. */
export const glassTide = {
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
export function filmsLibrary(id: number, name: string): Record<string, unknown> {
  return {
    id,
    name,
    kind: "film",
    metadataSource: "nfoThenTmdb",
    changeDetection: "watch",
    isWritable: true,
    createdAt: 0,
  };
}

/** A person's one films library, its page listing six films. */
export const aLibrary: WalkAnswers = {
  ...signedIn,
  "GET /libraries": { libraries: [filmsLibrary(1, "Films")] },
  "GET /items/letters": { letters: [] },
};

/** Sixteen libraries, more than a Fire TV's screen holds in the banner's Libraries menu. */
export const manyLibraries: WalkAnswers = {
  ...signedIn,
  "GET /libraries": {
    libraries: Array.from({ length: 16 }, (_, at) =>
      filmsLibrary(at + 1, `Library ${String(at + 1).padStart(2, "0")}`),
    ),
  },
};

/** count of the person's devices, as GET /me/devices lists them, the first this one. */
export function devices(count: number): Record<string, unknown>[] {
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
export function criteriaButton(): HTMLElement | undefined {
  return document.querySelector<HTMLElement>("main .list-criteria__button") ?? undefined;
}

/** The sort's chooser in the open Sort and filter: its button, found by the field it chooses. */
export function sortChooser(): HTMLElement | undefined {
  const chooser = [...document.querySelectorAll(".list-criteria .route-chooser")].find(
    (each) => each.querySelector(".route-chooser__field")?.textContent === "Sort",
  );
  return chooser?.querySelector<HTMLElement>(".route-chooser__button") ?? undefined;
}

/** Whether element holds more than it shows, scrolling within its own height. */
export function isScrollingWithin(element: HTMLElement | null): boolean {
  return element !== null && element.scrollHeight > element.clientHeight;
}

/** Whether element is drawn whole inside container's box, to half a pixel. */
export function isShownWholeIn(element: Element | null, container: Element | null): boolean {
  if (element === null || container === null) {
    return false;
  }
  const [box, within] = [element.getBoundingClientRect(), container.getBoundingClientRect()];
  return box.top >= within.top - 0.5 && box.bottom <= within.bottom + 0.5;
}

/** Whether what has focus is the first place of an open drop-down's list. */
export function isOnFirstPlace(): boolean {
  const first = document.querySelector('dialog .list-box a, dialog .list-box [role="button"]');
  return first !== null && document.activeElement === first;
}

/** Whether what has focus is a link to href. */
export function isFocusOnLinkTo(href: string): boolean {
  return document.activeElement?.getAttribute("href") === href;
}

/** The keys and waits of a walk, made with its test file's tools. */
export interface RemoteWalk {
  readonly press: (keys: string) => Promise<void>;
  readonly pressUntil: (key: string, isThere: () => boolean, most?: number) => Promise<boolean>;
  readonly drawn: (selector: string) => Promise<void>;
  readonly unreachedInMain: (options?: { hasArrival?: boolean }) => Promise<string[]>;
  readonly unreachedIn: (selector: string) => Promise<string[]>;
}

/** A walk's keys and waits, by tools' expect and keyboard. */
export function remoteWalk({ expect, keyboard }: WalkTools): RemoteWalk {
  /** Presses keys as a remote does: real key events, the browser's own handling with them. */
  async function press(keys: string): Promise<void> {
    await keyboard(keys);
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

  return { press, pressUntil, drawn, unreachedInMain, unreachedIn };
}
