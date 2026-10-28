// SPDX-License-Identifier: MPL-2.0

/**
 * Casting between two of a person's own devices, a session with a device (D364; D337, D338): two
 * browsers signed in as the owner are two devices; one plays a film and casts to the other from the
 * status tray's Cast, which asks once and plays it with a television's controls while the first is
 * its remote; every film the first then plays goes to the second, which waits full screen and ready
 * between them; either takes control back by Back, or the receiver goes back to the page it was on
 * when the remote stops casting.
 */

import { expect, type Browser, type Page } from "@playwright/test";

import { test } from "./browser-map.ts";
import { oneBrowserTag, timingTag } from "./only-these-tests.ts";
import { isPlaying, openFilm, signIn } from "./pages.ts";
import { secondFilm, warmUpFilm } from "./server.ts";

/** Whether the page's video is paused. */
async function isPaused(page: Page): Promise<boolean> {
  return await page.locator("video").evaluate((video: HTMLVideoElement) => video.paused);
}

/**
 * How soon a receiver pauses after its remote's Pause, at most, in ms: a LAN's push takes tens of
 * milliseconds, and a receiver behind its remote's window took seven seconds (the owner, 2026-10-28).
 */
const pausedWithinMs = 1000;

/**
 * Shows the page as hidden, or shown again, as a browser does a window behind another: the receiver
 * and its remote side by side on one computer, the remote's window over the receiver's. A headless
 * browser's page is never hidden of itself.
 */
async function setHidden(page: Page, isHidden: boolean): Promise<void> {
  await page.evaluate((isNowHidden) => {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => (isNowHidden ? "hidden" : "visible"),
    });
    document.dispatchEvent(new Event("visibilitychange"));
  }, isHidden);
}

/** The status tray's Cast sign, white while nothing is cast and blue while a cast is on. */
function castSign(page: Page): ReturnType<Page["locator"]> {
  return page.locator(".status-tray a[href^='/casting'], .status-tray a[aria-label^='Casting']");
}

/**
 * Signs two devices in, plays the film on the first and casts to the second from the tray's Cast,
 * the film moving with it: the second is asked once, is answered Use and plays it; then runs what
 * the test does with them, and closes both, so a device of one test is never another's to cast to.
 */
async function castBetweenTwo(
  browser: Browser,
  then: (devices: { sender: Page; receiver: Page }) => Promise<void>,
): Promise<void> {
  const senderContext = await browser.newContext();
  const receiverContext = await browser.newContext();
  try {
    await then(
      await castFromOneToTheOther(await senderContext.newPage(), await receiverContext.newPage()),
    );
  } finally {
    await senderContext.close();
    await receiverContext.close();
  }
}

/** castBetweenTwo's cast, from sender to receiver. */
async function castFromOneToTheOther(
  sender: Page,
  receiver: Page,
): Promise<{ sender: Page; receiver: Page }> {
  await signIn(sender);
  await signIn(receiver); // open on its home page, as a TV left on would be
  await openFilm(sender, warmUpFilm);
  await sender.getByRole("link", { name: "Play" }).click();
  await expect.poll(async () => await isPlaying(sender), { timeout: 30_000 }).toBe(true);
  // No Cast in the top bar: the tray's sign is the entry point (D364 §12).
  await expect(sender.locator(".app-shell__watching a[href^='/casting']")).toHaveCount(0);
  await expect(castSign(sender)).toHaveClass(/status-tray__button--off/u);
  await castSign(sender).click();
  await sender.locator(".cast-to__devices button").first().click();
  // The receiver asks once (D364 §2): the sender says it is asking until it answers.
  await expect(sender.getByText(/^Asking /u)).toBeVisible({ timeout: 10_000 });
  await expect(receiver.getByText(/wants to use this device for playback$/u)).toBeVisible();
  await receiver.locator(".cast-asked").getByRole("button", { name: "Use" }).click();

  await expect(sender.getByText(/^Playing on /u)).toBeVisible({ timeout: 10_000 });
  await expect(sender.locator("video")).toHaveCount(0);
  await expect(castSign(sender)).toHaveClass(/status-tray__button--on/u);
  await expect(receiver.getByText(/^Controlled from /u)).toBeVisible({ timeout: 10_000 });
  await expect.poll(async () => await isPlaying(receiver), { timeout: 30_000 }).toBe(true);
  return { sender, receiver };
}

test(
  "a person casts to another of their devices, controls it, plays a second film there with no question, and takes control back",
  // Timed: the receiver pauses within pausedWithinMs of its remote's Pause.
  { tag: [oneBrowserTag, timingTag] },
  async ({ browser }) => {
    await castBetweenTwo(browser, async ({ sender, receiver }) => {
      await expect(receiver.locator(".tv-controls")).toHaveCount(1);

      // A computer's hidden page plays on, so its receiver follows its remote hidden or not.
      await setHidden(receiver, true);
      await sender.getByRole("button", { name: "Pause" }).click();
      await expect
        .poll(async () => await isPaused(receiver), { timeout: pausedWithinMs, intervals: [50] })
        .toBe(true);
      await setHidden(receiver, false);
      await sender.getByRole("button", { name: "Play", exact: true }).click();
      await expect.poll(async () => await isPaused(receiver), { timeout: 10_000 }).toBe(false);

      // The session goes on: the next film the sender plays plays on the receiver (D364 §3).
      const firstAddress = receiver.url();
      await openFilm(sender, secondFilm);
      await sender.getByRole("link", { name: "Play" }).click();
      await expect(sender.getByRole("heading", { name: secondFilm })).toBeVisible({
        timeout: 10_000,
      });
      await expect(sender.locator("video")).toHaveCount(0);
      await expect.poll(() => receiver.url(), { timeout: 10_000 }).not.toBe(firstAddress);
      await expect(receiver.locator(".cast-asked")).toHaveCount(0);
      await expect.poll(async () => await isPlaying(receiver), { timeout: 30_000 }).toBe(true);

      // The remote is found again from any page, by the tray's sign (D364 §8).
      await sender.goto(new URL("/films", sender.url()).href);
      await castSign(sender).click();
      await expect(sender.getByText(/^Playing on /u)).toBeVisible({ timeout: 10_000 });

      await receiver.keyboard.press("Escape");
      await expect(sender.getByText(/took control back/u)).toBeVisible({ timeout: 10_000 });
      await expect(sender.getByRole("button", { name: /^Play here from / })).toBeVisible();
      await expect(castSign(sender)).toHaveClass(/status-tray__button--off/u);
      await expect(receiver.getByText(/^Controlled from /u)).toHaveCount(0);
      expect(await isPaused(receiver)).toBe(false);
    });
  },
);

test(
  "a receiver waits ready between videos, and goes back to the page it was on when the remote stops casting",
  { tag: oneBrowserTag },
  async ({ browser }) => {
    await castBetweenTwo(browser, async ({ sender, receiver }) => {
      // Played here instead, the receiver waits full screen and ready for the next (D364 §4, §5).
      await sender.getByRole("button", { name: "Play on this device instead" }).click();
      await expect.poll(async () => await isPlaying(sender), { timeout: 30_000 }).toBe(true);
      await expect(receiver.getByText(/^Ready, controlled from /u)).toBeVisible({
        timeout: 10_000,
      });
      await expect(receiver.locator("video")).toHaveCount(0);
      await castSign(sender).click();
      await expect(sender.getByText(/^Ready on /u)).toBeVisible({ timeout: 10_000 });
      await sender.getByRole("button", { name: "Stop casting" }).click();
      await expect(sender.getByText("Casting stopped.")).toBeVisible({ timeout: 10_000 });
      await expect(receiver.getByText(/^Ready, controlled from /u)).toHaveCount(0, {
        timeout: 10_000,
      });
      expect(new URL(receiver.url()).pathname).toBe("/");
    });
  },
);
