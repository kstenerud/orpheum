// SPDX-License-Identifier: MPL-2.0

/**
 * Casting between two of a person's own devices (D337; plans/cast-to-device.md §8): two browsers
 * signed in as the owner are two devices; one plays a film and casts it to the other, which plays it
 * while the first is its remote, and takes control back.
 */

import { expect, type Page } from "@playwright/test";

import { test } from "./browser-map.ts";
import { oneBrowserTag } from "./only-these-tests.ts";
import { isPlaying, openFilm, signIn } from "./pages.ts";
import { warmUpFilm } from "./server.ts";

/** Whether the page's video is paused. */
async function isPaused(page: Page): Promise<boolean> {
  return await page.locator("video").evaluate((video: HTMLVideoElement) => video.paused);
}

test(
  "a person casts a film to another of their devices, controls it from the first and takes control back",
  { tag: oneBrowserTag },
  async ({ browser }) => {
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

    await sender.getByRole("button", { name: "Pause" }).click();
    await expect.poll(async () => await isPaused(receiver), { timeout: 10_000 }).toBe(true);
    await sender.getByRole("button", { name: "Play", exact: true }).click();
    await expect.poll(async () => await isPaused(receiver), { timeout: 10_000 }).toBe(false);

    await receiver.keyboard.press("Enter");
    await expect(receiver.getByRole("button", { name: "Take back control" })).toBeVisible();
    await receiver.keyboard.press("Enter");
    await expect(sender.getByText(/took control back/u)).toBeVisible({ timeout: 10_000 });
    await expect(sender.getByRole("button", { name: /^Play here from / })).toBeVisible();
    await expect(receiver.getByText(/^Controlled from /u)).toHaveCount(0);
    expect(await isPaused(receiver)).toBe(false);
  },
);
