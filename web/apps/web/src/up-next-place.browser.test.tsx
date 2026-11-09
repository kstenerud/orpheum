// SPDX-License-Identifier: MPL-2.0

/**
 * Where the Up next card sits over a television's film, as a real browser lays it out: shown
 * without the rest of the overlay, in the film's top-right corner, clear of the action; at row 0's
 * end while all of the overlay shows (D407 §1; the owner, 2026-11-09).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";

import { routeHref } from "./router.ts";
import { directDecision } from "./test-fixtures.ts";
import {
  filmDetail,
  isDrawn,
  mountApp,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
} from "./test-walk.tsx";

/** File 1030's page and playback, whose media the test does not serve, and S1E3 "Bag" after it. */
const filmToPlay: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /items/103": filmDetail(103, "Film 3"),
  "POST /playback": directDecision({
    titleName: "Film 3",
    plays: { itemId: 103, kind: "film", libraryId: 1 },
    url: "/no-such-media",
  }),
  "GET /files/1030/up-next": {
    upNext: {
      episode: {
        itemId: 104,
        fileId: 1040,
        name: "Bag",
        groupNumber: 1,
        numberInGroup: 3,
        isPartway: false,
      },
      ordering: "aired",
    },
  },
};

/** A Fire TV Stick's Silk, as the owner's says itself. */
const fireTvSilk =
  "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36";

/** Presses keys as a remote does: real key events, the browser's own handling with them. */
async function press(keys: string): Promise<void> {
  await userEvent.keyboard(keys);
}

/** The overlay's state, as it draws it. */
function shown(): string | null | undefined {
  return document.querySelector(".player-overlay")?.getAttribute("data-shown");
}

/** The page's video once its load has failed, as with no media served, standing at atS of 600 s. */
async function theVideoAt(atS: number): Promise<HTMLVideoElement> {
  await expect.poll(() => isDrawn("video")).toBe(true);
  const video = document.querySelector("video");
  if (video === null) {
    throw new Error("the player draws no video");
  }
  await expect.poll(() => video.error?.code).toBeDefined();
  let positionS = atS;
  Object.defineProperty(video, "duration", { configurable: true, get: () => 600 });
  Object.defineProperty(video, "currentTime", {
    configurable: true,
    get: () => positionS,
    set: (toS: number) => {
      positionS = toS;
    },
  });
  Object.defineProperty(video, "paused", { configurable: true, get: () => false });
  return video;
}

describe("Up next over a television's film, shown without the rest of the overlay", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  /** The offer's card and the film's stage, as the browser lays them out. */
  function boxes(): { readonly card: DOMRect; readonly stage: DOMRect } {
    const card = document.querySelector(".player-overlay .up-next");
    const stage = document.querySelector(".film-player__stage");
    if (card === null || stage === null) {
      throw new Error("no Up next card over the film's stage");
    }
    return { card: card.getBoundingClientRect(), stage: stage.getBoundingClientRect() };
  }

  /**
   * Expects the card in the stage's top-right corner, clear of the film's middle: its top within the
   * stage's top 15 %, its end within 5 % of the stage's, all of it in the stage's top-right quarter.
   */
  function expectsTheTopRightCorner(): void {
    const { card, stage } = boxes();
    expect(card.top - stage.top).toBeLessThanOrEqual(stage.height * 0.15);
    expect(stage.right - card.right).toBeLessThanOrEqual(stage.width * 0.05);
    expect(card.bottom).toBeLessThanOrEqual(stage.top + stage.height / 2);
    expect(card.left).toBeGreaterThanOrEqual(stage.left + stage.width / 2);
  }

  // In row 0, its place in the full overlay, the card stood a third of the way up the film, over
  // the action, as Brindlewood's closing skits are (the owner, 2026-11-09).
  it("sits in the film's top-right corner, there through a seek's scrub bar, and OK still plays it", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(fireTvSilk);
    vi.spyOn(Element.prototype, "requestFullscreen").mockRejectedValue(new Error("refused"));
    await page.viewport(960, 480);
    const heard: string[] = [];
    const services = walkServices(filmToPlay, "/play/1030", heard);
    walking = await mountApp(services);
    const video = await theVideoAt(590); // ten seconds from the end, within Up next's lead
    // What plays next is asked for once the film has played (up-next.tsx).
    await expect
      .poll(() => {
        video.dispatchEvent(new Event("playing"));
        return heard.includes("GET /files/1030/up-next");
      })
      .toBe(true);
    await expect
      .poll(() => {
        video.dispatchEvent(new Event("timeupdate"));
        return shown();
      })
      .toBe("convenience");
    expectsTheTopRightCorner();
    // A seek shows the scrub bar along the film's bottom, and the offer stays in its corner (Q3).
    await press("{ArrowLeft}");
    await expect.poll(shown).toBe("scrub");
    expectsTheTopRightCorner();
    const { stage } = boxes();
    const scrub = document.querySelector(".player-overlay .timeline__range");
    expect(scrub?.getBoundingClientRect().top).toBeGreaterThan(stage.top + stage.height / 2);
    // All of the overlay shown, the card is at row 0's end again (D407 §1), where Up and Down walk.
    await press("{ArrowUp}");
    await expect.poll(shown).toBe("full");
    expect(boxes().card.top).toBeGreaterThan(stage.top + stage.height / 2);
    // Back hides the overlay and leaves the offer in its corner, where a remote's OK presses it.
    await press("{Escape}");
    await expect.poll(shown).toBe("convenience");
    expectsTheTopRightCorner();
    await press("{Enter}");
    await expect
      .poll(() => routeHref(services.navigation.route()))
      .toBe(routeHref({ screen: "play", fileId: 1040 }));
  });
});
