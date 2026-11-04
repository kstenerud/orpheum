// SPDX-License-Identifier: MPL-2.0

/** What every browser test does to a page: reach the server, sign in, open a film, see it play. */

import { expect, type Page, type Request } from "@playwright/test";

import { owner, serverAddressEnvVariable } from "./server.ts";

export function serverUrl(): string {
  const url = process.env[serverAddressEnvVariable];
  if (url === undefined) {
    throw new Error("the global setup did not start a server");
  }
  return url;
}

export async function signIn(page: Page): Promise<void> {
  await page.goto(serverUrl());
  await page.getByLabel("Name").fill(owner.name);
  await page.getByLabel("Password").fill(owner.password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeHidden();
}

/**
 * Opens a film's page from Films, waiting for the scan to have found it: its card in the grid,
 * not its row under Continue Watching, which a film played before also has and which plays it.
 */
export async function openFilm(page: Page, name: string): Promise<void> {
  const card = page
    .locator(".poster-grid a.poster-card__link")
    .filter({ hasText: new RegExp(name) });
  await expect(async () => {
    await page.goto(`${serverUrl()}/films`);
    await expect(card).toBeVisible({ timeout: 1000 });
  }).toPass({ timeout: 60_000 });
  await card.click();
}

/** A request for one of a film's chunks, which a film page makes only for a film on its ladder. */
export const chunkRequest = /\/bytes\/media\/chunks\/.+\.m4s/u;

/**
 * Opens a film's page once the film is on its ladder: its page then fills the player's first buffer
 * (D260 §3). A film whose timeline is not read yet plays as it is, which the server decides at
 * once after play_ladder_wait_ms, and under a loaded machine its read-through can take longer than
 * that; so the page is opened again until it preloads a chunk, as a person would come back to it.
 */
export async function openFilmOnItsLadder(page: Page, name: string): Promise<void> {
  // A listener heard by an assertion, which toPass retries: a waitForRequest that timed out failed
  // the test from inside it, 5.5 s into its 60 (check-pr, 2026-10-15, twice).
  let isPreloaded = false;
  const hear = (request: Request): void => {
    if (chunkRequest.test(request.url())) {
      isPreloaded = true;
    }
  };
  page.on("request", hear);
  try {
    await expect(async () => {
      isPreloaded = false;
      await openFilm(page, name);
      await expect.poll(() => isPreloaded, { timeout: 5000 }).toBe(true);
    }).toPass({ timeout: 60_000 });
  } finally {
    page.off("request", hear);
  }
}

/** Whether the page's video has started: its picture has moved past the start and is not paused. */
export async function isPlaying(page: Page): Promise<boolean> {
  try {
    return await page
      .locator("video")
      .evaluate((video: HTMLVideoElement) => video.currentTime > 0 && !video.paused, undefined, {
        timeout: 1000,
      });
  } catch {
    return false; // no video on the page yet
  }
}

/**
 * A control of the player's overlay, over the film (D418 §1), by its name: the controls are hidden
 * while the film plays, so the pointer moves over the film first, as a person's would, which shows
 * them.
 */
export async function shownOverTheFilm(
  page: Page,
  name: string | RegExp,
): Promise<ReturnType<Page["locator"]>> {
  const film = await page.locator("video").boundingBox();
  if (film !== null) {
    await page.mouse.move(film.x + film.width / 2, film.y + film.height / 2);
    await page.mouse.move(film.x + film.width / 2 + 8, film.y + film.height / 2 + 8);
  }
  return page.locator(".player-overlay").getByRole("button", { name });
}
