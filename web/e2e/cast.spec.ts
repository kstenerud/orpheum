// SPDX-License-Identifier: MPL-2.0

/**
 * Casting between two of a person's own devices (D337, D338; plans/cast-to-device.md §8): two
 * browsers signed in as the owner are two devices; one plays a film and casts it to the other, which
 * plays it with a television's controls while the first is its remote, and either takes control back
 * by Back, or goes back to the page it was on when the remote stops casting.
 */

import { expect, type Browser, type Page } from "@playwright/test";

import { test } from "./browser-map.ts";
import { oneBrowserTag } from "./only-these-tests.ts";
import { isPlaying, openFilm, signIn } from "./pages.ts";
import { warmUpFilm } from "./server.ts";

/** Whether the page's video is paused. */
async function isPaused(page: Page): Promise<boolean> {
  return await page.locator("video").evaluate((video: HTMLVideoElement) => video.paused);
}

/** Signs two devices in, plays the film on the first and casts it to the second, which plays it. */
async function castBetweenTwo(browser: Browser): Promise<{ sender: Page; receiver: Page }> {
  const sender = await (await browser.newContext()).newPage();
  const receiver = await (await browser.newContext()).newPage();
  await signIn(sender);
  await signIn(receiver); // open on its home page, as a TV left on would be
  await openFilm(sender, warmUpFilm);
  await sender.getByRole("link", { name: "Play" }).click();
  await expect.poll(async () => await isPlaying(sender), { timeout: 30_000 }).toBe(true);
  await sender.getByRole("button", { name: "Cast to another device" }).click();
  await sender.locator(".cast-to__devices button").first().click();

  await expect(sender.getByText(/^Playing on /u)).toBeVisible({ timeout: 10_000 });
  await expect(sender.locator("video")).toHaveCount(0);
  await expect(receiver.getByText(/^Controlled from /u)).toBeVisible({ timeout: 10_000 });
  await expect.poll(async () => await isPlaying(receiver), { timeout: 30_000 }).toBe(true);
  return { sender, receiver };
}

test(
  "a person casts a film to another of their devices, controls it from the first and takes control back",
  { tag: oneBrowserTag },
  async ({ browser }) => {
    const { sender, receiver } = await castBetweenTwo(browser);
    await expect(receiver.locator(".tv-controls")).toHaveCount(1);

    await sender.getByRole("button", { name: "Pause" }).click();
    await expect.poll(async () => await isPaused(receiver), { timeout: 10_000 }).toBe(true);
    await sender.getByRole("button", { name: "Play", exact: true }).click();
    await expect.poll(async () => await isPaused(receiver), { timeout: 10_000 }).toBe(false);

    await receiver.keyboard.press("Escape");
    await expect(sender.getByText(/took control back/u)).toBeVisible({ timeout: 10_000 });
    await expect(sender.getByRole("button", { name: /^Play here from / })).toBeVisible();
    await expect(receiver.getByText(/^Controlled from /u)).toHaveCount(0);
    expect(await isPaused(receiver)).toBe(false);
  },
);

test(
  "a receiver goes back to the page it was on when the remote stops casting",
  { tag: oneBrowserTag },
  async ({ browser }) => {
    const { sender, receiver } = await castBetweenTwo(browser);
    await sender.getByRole("button", { name: "Stop casting" }).click();
    await expect(sender.getByText("Casting stopped.")).toBeVisible({ timeout: 10_000 });
    await expect(receiver.locator("video")).toHaveCount(0, { timeout: 10_000 });
    expect(new URL(receiver.url()).pathname).toBe("/");
  },
);
