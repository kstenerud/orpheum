// SPDX-License-Identifier: MPL-2.0

/**
 * The answers, pages and walk the walks of every page by a remote's arrows share
 * (page-walk.browser.test.tsx, page-walk-manage.browser.test.tsx and
 * page-walk-person.browser.test.tsx), the walk made with vitest's own expect and keyboard, which
 * production code may not import (web/.dependency-cruiser.cjs).
 */

import { waitMs } from "@orpheum/sdk";

import { focusTargetsIn } from "./focus-engine.ts";
import { focusRegion } from "./focus-navigation.ts";
import {
  filmDetail,
  films,
  reachedFrom,
  viewer,
  type WalkAnswers,
  type WalkTools,
} from "./test-walk.tsx";

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
