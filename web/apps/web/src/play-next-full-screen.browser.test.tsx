// SPDX-License-Identifier: MPL-2.0

/**
 * The Play page's full screen across the next or previous episode in a real browser: Next and
 * Previous draw the Play page again for the other file, and full screen held by the page's own
 * stage ended as that stage went (the owner, 2026-11-05: "Pressing next or previous episode while in
 * fullscreen removes fullscreen"). The browser's full screen itself is not made here: a real
 * full-screen window outlasts its test file (idiosyncrasies.md), so what is asked to go full screen
 * is recorded instead, and it must outlive the page drawn again.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";

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

/** The full-screen button, found by its name as a viewer finds it. */
const FULL_SCREEN = 'button:is([aria-label="Full screen"], [aria-label="Leave full screen"])';

/** A film's page and its file's playback, whose media the test does not serve. */
const filmToPlay: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /items/103": filmDetail(103, "Film 3"),
  "POST /playback": directDecision({
    titleName: "Film 3",
    plays: { itemId: 103, kind: "movie", libraryId: 1 },
    url: "/no-such-media",
  }),
};

describe("the Play page in full screen, as Next or Previous plays another file", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  it("asks full screen of what the next file's Play page keeps, the page, not the stage that goes", async () => {
    const asked: Element[] = [];
    vi.spyOn(Element.prototype, "requestFullscreen").mockImplementation(function (this: Element) {
      asked.push(this);
      return Promise.resolve();
    });
    await page.viewport(1280, 800);
    const services = walkServices(filmToPlay, "/play/1030");
    walking = await mountApp(services);
    // Once its load has failed, as it does with no media served, the player changes nothing more.
    await expect.poll(() => document.querySelector("video")?.error?.code).toBeDefined();
    const stage = document.querySelector(".film-player__stage");
    const fullScreen = (): HTMLButtonElement | null =>
      document.querySelector<HTMLButtonElement>(`.player-overlay ${FULL_SCREEN}`);
    await expect.poll(fullScreen).not.toBeNull();
    fullScreen()?.click();
    await expect.poll(() => asked.length).toBe(1);
    // Next's own move (up-next.tsx): the Play page drawn again, for the other file.
    services.navigation.replace({ screen: "play", fileId: 1040 });
    await expect.poll(() => document.querySelector(".film-player__stage")).not.toBe(stage);
    await expect.poll(() => isDrawn("video")).toBe(true);
    expect(stage?.isConnected).toBe(false);
    expect(asked[0]?.isConnected).toBe(true);
    expect(asked[0]).toBe(document.documentElement);
  });
});
