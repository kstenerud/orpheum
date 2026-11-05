// SPDX-License-Identifier: MPL-2.0

/**
 * Every page the test services' answers draw, walked by arrows from its arrival in a real browser at
 * a Fire TV's page in Silk (960 × 480): each control on it must be reached, or a remote cannot use it
 * (ui-audit-2026-11-04.md §2, Android TV's checklist; R3 and R6). The walk follows every arrow from
 * every control reached (test-walk.tsx reachedFrom), the banner's included, or, where a dialog opens
 * over the page, the dialog's alone. A page whose reads the answers here do not give (a person's
 * record, the jobs, the update's state) draws its failure or its waiting, and is walked as it is.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { focusTargetsIn } from "./focus-engine.ts";
import { focusRegion } from "./focus-navigation.ts";
import { createSession } from "./session.ts";
import {
  filmDetail,
  films,
  mountApp,
  reachedFrom,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
} from "./test-walk.tsx";

/** An admin's answers, so Manage's pages draw, with a films library of six films. */
const anAdmin: WalkAnswers = {
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
  "GET /titles": { titles: films(6), nextCursor: "" },
  "GET /titles/letters": { letters: [] },
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
    nextUp: { titleId: 302, name: "Episode 2", groupNumber: 1, numberInGroup: 2, fileId: 3020 },
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

/** The pages walked, each at its address, signed in as an admin. */
const pages: readonly string[] = [
  "/",
  "/films",
  "/series",
  "/browse?library=1",
  "/titles/103",
  "/series/20?season=1",
  "/identify",
  "/libraries",
  "/libraries/1",
  "/libraries/new",
  "/libraries/1/files",
  "/about",
  "/settings",
  "/jobs",
  "/system",
  "/update",
  "/needs-attention",
  "/users",
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

/** Presses keys as a remote does. */
async function press(keys: string): Promise<void> {
  await userEvent.keyboard(keys);
}

/** What has focus, as a failure shows it. */
function focused(): string {
  return document.activeElement?.outerHTML.slice(0, 120) ?? "nothing";
}

/** Whether the page has drawn its main content: anything to read in it. */
function isMainDrawn(): boolean {
  return (document.querySelector("main")?.textContent ?? "") !== "";
}

/**
 * The controls on the page no arrow reaches from where it starts: its arrival, else the first an
 * arrow starts at; each as its markup's start.
 */
async function unreachedOnThePage(): Promise<string[]> {
  await expect.poll(isMainDrawn).toBe(true);
  // The page's reads after it opens, and the arrival given once they are drawn.
  await new Promise((resolve) => setTimeout(resolve, 300));
  if (
    !(document.activeElement instanceof HTMLElement) ||
    document.activeElement === document.body
  ) {
    await press("{ArrowDown}");
  }
  const start = document.activeElement;
  if (!(start instanceof HTMLElement) || start === document.body) {
    return [`no arrow started anywhere: ${focused()}`];
  }
  const reached = await reachedFrom(start, press);
  // Where a dialog opens over the page (Watch Together's address), the page behind is inert.
  return focusTargetsIn(focusRegion())
    .filter((control) => !reached.has(control))
    .map((control) => control.outerHTML.slice(0, 100));
}

describe("every page by a remote's arrows, on a Fire TV's page in Silk", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    window.scrollTo({ top: 0, left: 0 });
  });

  it.each(pages)("reaches every control of %s from its arrival", async (path) => {
    await page.viewport(960, 480);
    walking = await mountApp(walkServices(anAdmin, path));
    expect(await unreachedOnThePage()).toEqual([]);
  });

  it("reaches every control of the sign-in page from its arrival", async () => {
    await page.viewport(960, 480);
    const services = walkServices({ "GET /client-settings": {} });
    walking = await mountApp({ ...services, session: createSession(undefined) });
    expect(await unreachedOnThePage()).toEqual([]);
  });
});
