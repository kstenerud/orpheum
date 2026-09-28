// SPDX-License-Identifier: MPL-2.0

import { expect, test, type Page } from "@playwright/test";

import { longTrackName, owner, serverAddress, timingFilm, warmUpFilm } from "./server.ts";

/** How long pressing play may take to start the picture moving (cut-a §8, M3's acceptance). */
const playsWithinMs = 8000;

function address(): string {
  const url = process.env[serverAddress];
  if (url === undefined) {
    throw new Error("the global setup did not start a server");
  }
  return url;
}

async function signIn(page: Page): Promise<void> {
  await page.goto(address());
  await page.getByLabel("Name").fill(owner.name);
  await page.getByLabel("Password").fill(owner.password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeHidden();
}

/**
 * Opens a film's page from Films, waiting for the scan to have found it. A film played before is
 * also under Continue Watching, so its first link is taken.
 */
async function openFilm(page: Page, name: string): Promise<void> {
  const card = page.getByRole("link", { name: new RegExp(name) }).first();
  await expect(async () => {
    await page.goto(`${address()}/films`);
    await expect(card).toBeVisible({ timeout: 1000 });
  }).toPass({ timeout: 60_000 });
  await card.click();
}

/** Whether the page's video has started: its picture has moved past the start and is not paused. */
async function isPlaying(page: Page): Promise<boolean> {
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

test("an MKV with H.264, TrueHD and ASS plays within 8 s of pressing play, with the reason on screen", async ({
  page,
  browserName,
}) => {
  await signIn(page);

  // A device is asked how it plays sound before its first film; a film played as it is answers
  // it here, so the timed press is an ordinary one.
  await openFilm(page, warmUpFilm);
  await page.getByRole("link", { name: "Play" }).click();
  await page.getByRole("button", { name: /TV speakers/ }).click();
  await expect.poll(() => isPlaying(page), { timeout: 30_000 }).toBe(true);
  await expect(page.locator(".play__reason")).toHaveText("Playing the original file, as it is.");

  await openFilm(page, timingFilm(browserName));
  const pressed = Date.now();
  await page.getByRole("link", { name: "Play" }).click();
  await expect.poll(() => isPlaying(page), { timeout: playsWithinMs, intervals: [50] }).toBe(true);
  const tookMs = Date.now() - pressed;
  test.info().annotations.push({ type: "played after", description: `${String(tookMs)} ms` });
  expect(tookMs).toBeLessThanOrEqual(playsWithinMs);
  // TrueHD is the reason: no browser decodes it, so what plays is the copy prepared with AAC.
  const reason = page.locator(".play__reason");
  await expect(reason).toContainText("Playing a copy prepared for this browser");
  await expect(reason).toContainText("the browser cannot decode its sound");
});

test("the System page shows the machine's hardware encoding and what the timed film's prepare used", async ({
  page,
  browserName,
}) => {
  await signIn(page);
  await page.goto(`${address()}/system`);
  await expect(page.getByRole("heading", { name: "Hardware encoding" })).toBeVisible();
  await expect(
    page.getByText(/encoded on the GPU|encoded on the processor/u).first(),
  ).toBeVisible();
  // The timed film's video was copied: only its TrueHD was made into something a browser plays.
  // Its prepare is listed once it has finished, which may be after the page first loads.
  const row = page.getByRole("row", { name: new RegExp(timingFilm(browserName)) });
  await expect(async () => {
    await page.reload();
    await expect(row).toContainText("copied, not encoded", { timeout: 1000 });
  }).toPass({ timeout: 90_000 });
  await expect(row).toContainText("× real time");
});

test("on a phone's screen, the play page is no wider than the screen, whatever its tracks are called", async ({
  page,
  browserName,
}) => {
  await page.setViewportSize({ width: 412, height: 915 });
  await signIn(page);
  await openFilm(page, timingFilm(browserName));
  await page.getByRole("link", { name: "Play" }).click();
  await page.getByRole("button", { name: /TV speakers/ }).click();
  await expect(page.getByRole("combobox", { name: "Subtitles" })).toContainText(longTrackName);
  const widths = await page.evaluate(() => ({
    page: document.documentElement.scrollWidth,
    screen: document.documentElement.clientWidth,
  }));
  expect(widths.page).toBeLessThanOrEqual(widths.screen);
});
