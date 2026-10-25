// SPDX-License-Identifier: MPL-2.0

import { expect } from "@playwright/test";

import { test } from "./browser-map.ts";
import { oneBrowserTag, timingTag } from "./only-these-tests.ts";
import { isPlaying, openFilm, openFilmOnItsLadder, serverUrl, signIn } from "./pages.ts";
import { longTrackName, preparedFilm, timingFilm, warmUpFilm } from "./server.ts";

/** How long pressing play may take to start the picture moving (cut-a §8, M3's acceptance). */
const playsWithinMs = 8000;

/**
 * Says a film plays on its ladder (D260): its tracks name the sound transcoded at the ladder's rate,
 * chunk_sound_kbps's 96 kb/s, where a prepared copy's is 192 and the original's names none (the
 * tracks the player names, the owner, 2026-10-20).
 */
const onTheLadder = "AAC stereo, 96 kb/s (transcoded from";

test(
  "an MKV with H.264, TrueHD and ASS loads its first chunks on its page and plays from its ladder within 8 s of pressing play",
  { tag: timingTag },
  async ({ page, browserName }) => {
    await signIn(page);

    // A film played first, so the timed press is an ordinary one. Its page is opened once it is on
    // its ladder, whose timeline a loaded machine may still be reading (failed 2 of 4 check-pr
    // runs, 2026-10-09).
    await openFilmOnItsLadder(page, warmUpFilm);
    await page.getByRole("link", { name: "Play" }).click();
    await expect.poll(() => isPlaying(page), { timeout: 30_000 }).toBe(true);
    await expect(page.locator(".play__tracks")).toContainText(onTheLadder);

    // Its page fills the player's first buffer before Play is pressed (D260 §3).
    await openFilmOnItsLadder(page, timingFilm(browserName));
    const pressedAt = Date.now();
    await page.getByRole("link", { name: "Play" }).click();
    await expect
      .poll(() => isPlaying(page), { timeout: playsWithinMs, intervals: [50] })
      .toBe(true);
    const tookMs = Date.now() - pressedAt;
    test.info().annotations.push({ type: "played after", description: `${String(tookMs)} ms` });
    expect(tookMs).toBeLessThanOrEqual(playsWithinMs);
    // Its own picture, copied, is the ladder's top, its TrueHD made into sound a browser plays.
    await expect(page.locator(".play__tracks")).toContainText(onTheLadder);
  },
);

test(
  "the System page shows the machine's hardware encoding and what a prepared film's prepare used",
  { tag: oneBrowserTag },
  async ({ page }) => {
    await signIn(page);
    // Opening its page starts its prepare (D258 §1), once its page has asked how it plays.
    const asked = page.waitForResponse(
      (response) => response.request().method() === "POST" && response.url().endsWith("/playback"),
    );
    await openFilm(page, preparedFilm);
    await asked;
    await page.goto(`${serverUrl()}/system`);
    await expect(page.getByRole("heading", { name: "Hardware encoding" })).toBeVisible();
    await expect(
      page.getByText(/encoded on the GPU|encoded on the processor/u).first(),
    ).toBeVisible();
    // The prepared film's MPEG-2 picture was encoded, by whichever encoder the machine has. Its prepare
    // is listed once it has finished, which may be after the page first loads.
    const row = page.getByRole("row", { name: new RegExp(preparedFilm) });
    await expect(async () => {
      await page.reload();
      await expect(row).toContainText(/x264|Quick Sync|NVENC/u, { timeout: 1000 });
    }).toPass({ timeout: 90_000 });
    await expect(row).toContainText("× real time");
  },
);

test("on a phone's screen, the play page is no wider than the screen, whatever its tracks are called", async ({
  page,
  browserName,
}) => {
  await page.setViewportSize({ width: 412, height: 915 });
  await signIn(page);
  await openFilm(page, timingFilm(browserName));
  await page.getByRole("link", { name: "Play" }).click();
  await expect(page.getByRole("combobox", { name: "Subtitles" })).toContainText(longTrackName);
  const widths = await page.evaluate(() => ({
    page: document.documentElement.scrollWidth,
    screen: document.documentElement.clientWidth,
  }));
  expect(widths.page).toBeLessThanOrEqual(widths.screen);
});
