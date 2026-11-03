// SPDX-License-Identifier: MPL-2.0

import { expect, type Page } from "@playwright/test";

import { test } from "./browser-map.ts";
import { isPlaying, openFilm, signIn } from "./pages.ts";
import { longTrackName, timingFilm } from "./server.ts";

/** The video's subtitle text tracks: how each is shown, and how many captions it has loaded. */
async function subtitleTracks(page: Page): Promise<{ mode: string; cues: number }[]> {
  return await page
    .locator("video")
    .evaluate((video: HTMLVideoElement) =>
      [...video.textTracks]
        .filter((track) => track.kind === "subtitles")
        .map((track) => ({ mode: track.mode, cues: track.cues?.length ?? 0 })),
    );
}

test("a chosen subtitle track is also the browser's own captions, shown only in the video's own fullscreen (D254)", async ({
  page,
  browserName,
}) => {
  await signIn(page);
  await openFilm(page, timingFilm(browserName));
  await page.getByRole("link", { name: "Play" }).click();
  // The subtitles' chooser, a pop-up since the native select went (plans/toolkit.md step 6).
  const subtitles = page.locator(".route-chooser", { hasText: "Subtitles" }).getByRole("button");
  await expect(subtitles).toBeVisible();
  await expect.poll(() => isPlaying(page), { timeout: 30_000 }).toBe(true);
  // The film's one track, after "Off".
  await subtitles.click();
  await page
    .getByRole("dialog", { name: "Subtitles" })
    .getByRole("button", { name: longTrackName })
    .click();
  // Loaded, and hidden: on the page the drawn layer shows the track as its author styled it.
  await expect
    .poll(() => subtitleTracks(page), { timeout: 30_000 })
    .toEqual([{ mode: "hidden", cues: 1 }]);
  // In the video's own fullscreen, which the drawn layer cannot reach, the browser shows them.
  await page.locator("video").evaluate((video: HTMLVideoElement) => video.requestFullscreen());
  await expect.poll(() => subtitleTracks(page)).toEqual([{ mode: "showing", cues: 1 }]);
  await page.evaluate(() => document.exitFullscreen());
  await expect.poll(() => subtitleTracks(page)).toEqual([{ mode: "hidden", cues: 1 }]);
});
