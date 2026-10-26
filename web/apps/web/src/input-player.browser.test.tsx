// SPDX-License-Identifier: MPL-2.0

/**
 * The player walked with a remote's keys in a real browser at a Fire TV's page in Silk and a
 * desktop's window (controller-input-2026-10-25.md §6.1 item 4): every control under the video
 * reached by arrows, the video seeked by Left and Right and played and paused by OK, and Back
 * leaving the page for the film's, focus where it was.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";

import { focusTargetsIn } from "./focus-engine.ts";
import { seekStepMs } from "./player-commands.ts";
import { directDecision } from "./test-fixtures.ts";
import {
  filmDetail,
  isDrawn,
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

/** Film 3's page and its file's playback, whose media the walk does not serve. */
const filmToPlay: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /items/103": filmDetail(103, "Film 3"),
  "POST /playback": directDecision({
    titleName: "Film 3",
    url: "/no-such-media",
    subtitles: [
      {
        streamId: 5,
        language: "en",
        isDefault: false,
        isForced: false,
        isHearingImpaired: false,
        assUrl: "/no-such-subtitle.ass",
        vttUrl: "/no-such-subtitle.vtt",
      },
    ],
    audioTracks: [
      { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
      { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "commentary" },
    ],
  }),
};

/** Presses keys as a remote does: real key events, the browser's own handling with them. */
async function press(keys: string): Promise<void> {
  await userEvent.keyboard(keys);
}

/** The page's video, once drawn. */
async function theVideo(): Promise<HTMLVideoElement> {
  await expect.poll(() => isDrawn("video")).toBe(true);
  const video = document.querySelector("video");
  if (video === null) {
    throw new Error("the player draws no video");
  }
  return video;
}

/**
 * The page's video, once its load has failed, as it does with no media served: the player then
 * says so under it and changes nothing more, so a walk from it meets the page as it stays. A walk
 * begun before, on a loaded machine, met a page the failure's words then moved (2026-10-25).
 */
async function theFailedVideo(): Promise<HTMLVideoElement> {
  const video = await theVideo();
  await expect.poll(() => video.error?.code).toBeDefined();
  return video;
}

describe.each(screens)("the player by a remote's keys, on $name", ({ width, height }) => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  it("reaches every control under the video by arrows, starting on the video", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmToPlay, "/play/1030"));
    const video = await theFailedVideo();
    await press("{ArrowDown}");
    expect(document.activeElement).toBe(video);
    const reached = await reachedFrom(video, press);
    const player = document.querySelector(".play");
    const controls = focusTargetsIn(player ?? document.body);
    // This film's: the video, Start over, the subtitle and audio menus, Find subtitles, Level
    // loudness and the bare player.
    expect(controls.map((control) => control.textContent)).toEqual(
      expect.arrayContaining(["Start over", "Find subtitles…", "Play in a bare player"]),
    );
    expect(
      controls.filter((control) => control.tagName === "SELECT").length,
    ).toBeGreaterThanOrEqual(2);
    expect(controls[0]).toBe(video);
    const unreached = controls.filter((control) => !reached.has(control));
    expect(unreached.map((control) => control.outerHTML.slice(0, 80))).toEqual([]);
  });

  it("seeks by Left and Right and plays and pauses by OK on the video", async () => {
    const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmToPlay, "/play/1030"));
    const video = await theFailedVideo();
    video.focus();
    expect(document.activeElement).toBe(video);
    // The walk serves no media, so the browser's own position comes and goes as the load fails: the
    // video stands where the player last moved it, 30 s into a ten-minute film.
    let positionS = 30;
    Object.defineProperty(video, "duration", { configurable: true, get: () => 600 });
    Object.defineProperty(video, "currentTime", {
      configurable: true,
      get: () => positionS,
      set: (toS: number) => {
        positionS = toS;
      },
    });
    await press("{ArrowRight}");
    expect(video.currentTime).toBeCloseTo(30 + seekStepMs / 1000, 1);
    await press("{ArrowLeft}{ArrowLeft}");
    expect(video.currentTime).toBeCloseTo(30 - seekStepMs / 1000, 1);
    expect(document.activeElement).toBe(video);
    const playsBefore = play.mock.calls.length;
    await press("{Enter}");
    expect(play.mock.calls.length).toBe(playsBefore + 1);
  });

  it("goes back to the film's page by Back, to its Play", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmToPlay, "/titles/103"));
    await expect.poll(() => isDrawn(".title__play")).toBe(true);
    const playLink = document.querySelector<HTMLElement>(".title__play");
    playLink?.focus();
    await press("{Enter}");
    await theVideo();
    await press("{Escape}");
    await expect.poll(() => document.activeElement?.classList.contains("title__play")).toBe(true);
  });
});
