// SPDX-License-Identifier: MPL-2.0

/**
 * Casting between two of a person's own devices, a session with a device (D364; D337, D338): two
 * browsers signed in as the owner are two devices; one plays a film and casts to the other from the
 * status tray's Cast, which asks once and plays it with a television's controls while the first is
 * its remote, the page it cast from staying where it is (D384 §2); every film the first then plays
 * goes to the second, which waits full screen and ready between them; the receiver goes home as the
 * cast ends, its control taken back by Back, the first then playing the film itself (D385), or the
 * remote's Stop casting (D384 §1). While it receives, a third device's Cast dialog shows it
 * unavailable, no device taking it over (D386 §1).
 */

import { expect, type Browser, type Page } from "@playwright/test";

import { test } from "./browser-map.ts";
import { oneBrowserTag, timingTag } from "./only-these-tests.ts";
import { isPlaying, openFilm, playFromTheStart, shownOverTheFilm, signIn } from "./pages.ts";
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

/**
 * The status tray's Cast sign, white while nothing is cast and blue while a cast is on, a button that
 * opens Cast's dialog (D384 §2).
 */
function castSign(page: Page): ReturnType<Page["locator"]> {
  return page.locator(
    ".status-tray button[aria-label='Cast to a device'], .status-tray button[aria-label^='Casting']",
  );
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
  await playFromTheStart(sender).click();
  await expect.poll(async () => await isPlaying(sender), { timeout: 30_000 }).toBe(true);
  // The film fills the window over the tray, so Cast's sign over the film opens its dialog, over
  // the Play page, which stays where it is (D384 §2, D418 §1; the owner, 2026-11-04).
  const playAddress = sender.url();
  const overTheFilm = await shownOverTheFilm(sender, "Cast to a device");
  await expect(overTheFilm).toBeVisible();
  await expect(castSign(sender)).not.toHaveClass(/icon-button--running/u);
  await overTheFilm.click();
  await sender
    .getByRole("dialog", { name: "Cast" })
    .locator(".cast-to__devices button")
    .first()
    .click();
  // The receiver asks once (D364 §2): the sender says it is asking until it answers.
  await expect(sender.getByText(/^Asking /u)).toBeVisible({ timeout: 10_000 });
  await expect(receiver.getByText(/wants to use this device for playback$/u)).toBeVisible();
  await receiver
    .getByRole("dialog", { name: /wants to use this device for playback$/u })
    .getByRole("button", { name: "Use" })
    .click();

  await expect(sender.getByText(/^Playing on /u)).toBeVisible({ timeout: 10_000 });
  await expect(sender.getByRole("dialog", { name: "Cast" })).toHaveCount(0); // closed once its receiver said Use
  expect(sender.url()).toBe(playAddress);
  await expect(sender.locator("video")).toHaveCount(0);
  await expect(castSign(sender)).toHaveClass(/icon-button--running/u);
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
      await expect(receiver.locator(".player-overlay")).toHaveCount(1);

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
      await playFromTheStart(sender).click();
      await expect(sender.getByRole("heading", { name: secondFilm })).toBeVisible({
        timeout: 10_000,
      });
      await expect(sender.locator("video")).toHaveCount(0);
      await expect.poll(() => receiver.url(), { timeout: 10_000 }).not.toBe(firstAddress);
      await expect(receiver.getByRole("dialog", { name: /wants to use this device/u })).toHaveCount(
        0,
      );
      await expect.poll(async () => await isPlaying(receiver), { timeout: 30_000 }).toBe(true);

      // The remote is found again from any page, by the tray's sign's dialog, on the film's page
      // (D364 §8, D384 §2).
      await sender.goto(new URL("/films", sender.url()).href);
      await castSign(sender).click();
      await sender
        .getByRole("dialog", { name: "Cast" })
        .getByRole("button", { name: "Back to the film" })
        .click();
      await expect(sender.getByText(/^Playing on /u)).toBeVisible({ timeout: 10_000 });
      // The remote has joined its session again, which says how the receiver plays: "Playing on"
      // comes first, from the live connection, and a cast ended as the remote said hello was
      // only "over" to it, the server knowing the session no more (check-pr, 2026-10-29).
      await expect(sender.getByText(/^Plays (the file as it is|it converted)/u)).toBeVisible({
        timeout: 10_000,
      });

      // Control taken back ends the cast: the receiver goes home, and the sender plays the film
      // itself from where the receiver stood, playing as it was, offering nothing (D385 §1, §2).
      const playAddress = sender.url();
      await receiver.keyboard.press("Escape");
      await expect.poll(() => new URL(receiver.url()).pathname, { timeout: 10_000 }).toBe("/");
      await expect(receiver.locator("video")).toHaveCount(0);
      await expect(castSign(sender)).not.toHaveClass(/icon-button--running/u, { timeout: 10_000 });
      await expect.poll(async () => await isPlaying(sender), { timeout: 30_000 }).toBe(true);
      await expect(sender.getByRole("button", { name: /^Play here from / })).toHaveCount(0);
      await expect(sender.getByText(/took control back/u)).toHaveCount(0);
      expect(sender.url()).toBe(playAddress);
    });
  },
);

test(
  "a receiver is no other device's target, and goes home when the remote stops casting, its one way to bring the film back, the remote's page staying where it is",
  { tag: oneBrowserTag },
  async ({ browser }) => {
    const thirdContext = await browser.newContext();
    try {
      await castBetweenTwo(browser, async ({ sender, receiver }) => {
        // A third device's Cast dialog shows the receiver unavailable while it receives (D386 §1).
        const third = await thirdContext.newPage();
        await signIn(third);
        await castSign(third).click();
        const receiving = third
          .getByRole("dialog", { name: "Cast" })
          .locator(".cast-to__devices li")
          .filter({ hasText: "Showing another cast" });
        await expect(receiving).toHaveCount(1, { timeout: 10_000 });
        await expect(receiving.getByRole("button")).toBeDisabled();

        // The remote brings the film back by Stop casting alone (D388).
        const remote = sender.locator(".cast-remote");
        await expect(remote.getByRole("button", { name: "Stop casting" })).toBeVisible();
        await expect(
          sender.getByRole("button", { name: "Play on this device instead" }),
        ).toHaveCount(0);
        const playAddress = sender.url();
        await remote.getByRole("button", { name: "Stop casting" }).click();
        await expect(castSign(sender)).not.toHaveClass(/icon-button--running/u, {
          timeout: 10_000,
        });
        await expect.poll(() => new URL(receiver.url()).pathname, { timeout: 10_000 }).toBe("/");
        await expect.poll(async () => await isPlaying(sender), { timeout: 30_000 }).toBe(true);
        expect(sender.url()).toBe(playAddress);
        // Its cast over, the receiver is a target again (D386 §1).
        await expect(receiving).toHaveCount(0, { timeout: 10_000 });
      });
    } finally {
      await thirdContext.close();
    }
  },
);
