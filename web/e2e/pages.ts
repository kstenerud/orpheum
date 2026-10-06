// SPDX-License-Identifier: MPL-2.0

/** What every browser test does to a page: reach the server, sign in, open a film, see it play. */

import { expect, type Page } from "@playwright/test";

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
  const card = page.locator("a.poster-grid__card").filter({ hasText: new RegExp(name) });
  await expect(async () => {
    await page.goto(`${serverUrl()}/films`);
    await expect(card).toBeVisible({ timeout: 1000 });
  }).toPass({ timeout: 60_000 });
  await card.click();
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
