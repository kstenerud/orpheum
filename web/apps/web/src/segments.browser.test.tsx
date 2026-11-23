// SPDX-License-Identifier: MPL-2.0

/**
 * The skip over a television's film, as a real browser draws it and a remote presses it (b1.md
 * Phase C): shown alone in the overlay's convenience area as its segment plays, focused, so OK
 * skips to the segment's end.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";

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

/** File 1030's page and playback, its intro from 60 s to 90 s, whose media the test does not serve. */
const filmWithAnIntro: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /items/103": filmDetail(103, "Film 3"),
  "POST /playback": directDecision({
    titleName: "Film 3",
    plays: { itemId: 103, kind: "film", libraryId: 1 },
    url: "/no-such-media",
    segments: [{ type: "intro", startMs: 60_000, endMs: 90_000 }],
  }),
  "GET /files/1030/up-next": {},
};

/** A Fire TV Stick's Silk, as the owner's says itself. */
const fireTvSilk =
  "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36";

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

describe("the skip over a television's film", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  it("shows alone over the film inside its segment, focused, and OK seeks to the segment's end", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(fireTvSilk);
    vi.spyOn(Element.prototype, "requestFullscreen").mockRejectedValue(new Error("refused"));
    await page.viewport(960, 480);
    const services = walkServices(filmWithAnIntro, "/play/1030", []);
    walking = await mountApp(services);
    const video = await theVideoAt(70);
    await expect
      .poll(() => {
        video.dispatchEvent(new Event("timeupdate"));
        return shown();
      })
      .toBe("convenience");
    const skip = document.querySelector(".player-overlay .offer-card__take");
    expect(skip?.textContent).toBe("Skip intro");
    await expect.poll(() => document.activeElement === skip).toBe(true);
    await userEvent.keyboard("{Enter}");
    expect(video.currentTime).toBe(90);
    video.dispatchEvent(new Event("seeked"));
    await expect.poll(() => isDrawn(".offer-card__take")).toBe(false);
    await expect.poll(shown).toBe("hidden");
  });
});
