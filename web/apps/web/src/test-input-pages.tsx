// SPDX-License-Identifier: MPL-2.0

/**
 * The answers, finders and keys the walks of the app's pages with a remote's keys share
 * (input-pages.browser.test.tsx, input-pages-lists.browser.test.tsx and
 * input-pages-person.browser.test.tsx): the screens they are made at, what the server answers, and
 * the presses and waits each walk makes with vitest's own expect and keyboard, which production
 * code may not import (web/.dependency-cruiser.cjs).
 */

import { focusTargetsIn } from "./focus-engine.ts";
import {
  films,
  isDrawn,
  isFocusIn,
  pressesUntil,
  reachedFrom,
  viewer,
  type WalkAnswers,
  type WalkTools,
} from "./test-walk.tsx";

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
