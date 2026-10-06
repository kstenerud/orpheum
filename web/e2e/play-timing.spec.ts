// SPDX-License-Identifier: MPL-2.0

import { expect, test } from "@playwright/test";

import { isPlaying, openFilm, serverUrl, signIn } from "./pages.ts";
import { longTrackName, timingFilm, warmUpFilm } from "./server.ts";

/** How long pressing play may take to start the picture moving (cut-a §8, M3's acceptance). */
const playsWithinMs = 8000;

test("an MKV with H.264, TrueHD and ASS plays within 8 s of pressing play, with the reason on screen", async ({
  page,
  browserName,
}) => {
  await signIn(page);

  // A device is asked how it plays sound before its first film; a film played as it is answers
  // it here, so the timed press is an ordinary one.
  await openFilm(page, warmUpFilm);
  await page.getByRole("link", { name: "Play" }).click();
  await page.getByRole("button", { name: /Built-in speakers/ }).click();
  await expect.poll(() => isPlaying(page), { timeout: 30_000 }).toBe(true);
  await expect(page.locator(".play__reason")).toHaveText("Playing the original file, as it is.");

  await openFilm(page, timingFilm(browserName));
  const pressedAt = Date.now();
  await page.getByRole("link", { name: "Play" }).click();
  await expect.poll(() => isPlaying(page), { timeout: playsWithinMs, intervals: [50] }).toBe(true);
  const tookMs = Date.now() - pressedAt;
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
  await page.goto(`${serverUrl()}/system`);
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
  // Each answer to the sound question puts what it does below its name, not run into it.
  const answer = page.getByRole("button", { name: /Built-in speakers/ });
  const nameBox = await answer.locator("strong").boundingBox();
  const hintBox = await answer.locator("span").boundingBox();
  expect(hintBox?.y ?? 0).toBeGreaterThanOrEqual((nameBox?.y ?? 0) + (nameBox?.height ?? 0));
  await answer.click();
  await expect(page.getByRole("combobox", { name: "Subtitles" })).toContainText(longTrackName);
  const widths = await page.evaluate(() => ({
    page: document.documentElement.scrollWidth,
    screen: document.documentElement.clientWidth,
  }));
  expect(widths.page).toBeLessThanOrEqual(widths.screen);
});
