// SPDX-License-Identifier: MPL-2.0

/**
 * Watch Together between two browsers, a host signed in and a guest by their link (M4's
 * acceptance, D252): the link works once, and only once joined; no credential is ever in an
 * address; a film the browser would play only muted offers its sound at a tap; and voice between two fake microphones arrives within D243 §9's bounds.
 */

import { readFileSync } from "node:fs";
import { expect, type BrowserContext, type Page } from "@playwright/test";

import { test } from "./browser-map.ts";
import { oneBrowserTag, timingTag } from "./only-these-tests.ts";
import { isPlaying, signIn } from "./pages.ts";
import { warmUpFilm } from "./server.ts";
import {
  endSession,
  expectHeard,
  expectHearing,
  expectTalkRingMoves,
  expectVoiceWithinBounds,
  hostPlays,
  join,
  pageWithMicrophone,
  startSessionWithLink,
  talkAlone,
  talkBothWays,
} from "./together-pages.ts";

/** How long two people talk before their voice is measured against D243 §9's bounds. */
const talkForMs = 15_000;

/** Every value a context holds that lets it act: its cookies and its stored tokens. */
async function credentialsOf(context: BrowserContext, page: Page): Promise<string[]> {
  const cookies = (await context.cookies()).map((cookie) => cookie.value);
  const stored = await page.evaluate(() => {
    const values: string[] = [];
    for (const storage of [window.localStorage, window.sessionStorage]) {
      for (let index = 0; index < storage.length; index++) {
        values.push(storage.getItem(storage.key(index) ?? "") ?? "");
      }
    }
    return values;
  });
  // A stored membership is JSON: its credential is the secret, not the object around it.
  const fromJson = stored.flatMap((value) => {
    try {
      const parsed: unknown = JSON.parse(value);
      return typeof parsed === "object" && parsed !== null
        ? Object.values(parsed).filter((each): each is string => typeof each === "string")
        : [];
    } catch {
      return [value];
    }
  });
  return [...cookies, ...fromJson].filter((value) => value.length >= 16);
}

/** Every address a recorded context asked for, from its HAR. */
function addressesIn(harPath: string): string[] {
  const har: unknown = JSON.parse(readFileSync(harPath, "utf8"));
  const field = (value: unknown, name: string): unknown =>
    typeof value === "object" && value !== null ? Reflect.get(value, name) : undefined;
  const entries = field(field(har, "log"), "entries");
  return (Array.isArray(entries) ? entries : []).map((entry: unknown) =>
    String(field(field(entry, "request"), "url")),
  );
}

test(
  "an invitation works once, only once joined, and no credential is ever in an address",
  { tag: oneBrowserTag },
  async ({ browser }, testInfo) => {
    const hostHar = testInfo.outputPath("host.har");
    const guestHar = testInfo.outputPath("guest.har");
    const hostContext = await browser.newContext({ recordHar: { path: hostHar } });
    const guestContext = await browser.newContext({ recordHar: { path: guestHar } });
    const host = await hostContext.newPage();
    const guest = await guestContext.newPage();
    await signIn(host);
    const link = await startSessionWithLink(host);
    const secret = link.slice(link.indexOf("#") + 1);

    // Opened and reloaded before joining, the link still works (D252 §5).
    await guest.goto(link);
    await expect(guest.getByRole("button", { name: "Join" })).toBeVisible();
    await guest.reload();
    await join(guest, link);
    await hostPlays(host, warmUpFilm);
    await expect.poll(() => isPlaying(guest), { timeout: 30_000 }).toBe(true);

    // A browser that would play the film only muted, as an iPhone does long after the Join tap, gets
    // Tap for sound over the picture, and the tap turns the sound on.
    const video = guest.locator("video");
    await video.evaluate((element: HTMLVideoElement) => {
      // eslint-disable-next-line no-param-reassign -- muting the guest's video is what is simulated
      element.muted = true;
      element.dispatchEvent(new Event("orpheumplayedmuted"));
    });
    const soundOn = guest.getByRole("button", { name: "Tap to turn the sound back on" });
    const [picture, button] = await Promise.all([video.boundingBox(), soundOn.boundingBox()]);
    expect(
      picture !== null && button !== null && button.y + button.height <= picture.y + picture.height,
      "Tap to turn the sound back on is over the picture",
    ).toBe(true);
    await soundOn.click();
    expect(await video.evaluate((element: HTMLVideoElement) => element.muted)).toBe(false);

    // Joined, it is spent: another browser opening it is told so.
    const late = await browser.newContext();
    const latePage = await late.newPage();
    await latePage.goto(link);
    await latePage.getByLabel("Your name").fill("Someone else");
    await latePage.getByRole("button", { name: "Join" }).click();
    await expect(latePage.getByRole("alert")).toContainText("This link does not work");
    await late.close();

    const secrets = [
      secret,
      ...(await credentialsOf(hostContext, host)),
      ...(await credentialsOf(guestContext, guest)),
    ];
    await endSession(host);
    await hostContext.close();
    await guestContext.close();
    const addresses = [...addressesIn(hostHar), ...addressesIn(guestHar)];
    expect(addresses.length).toBeGreaterThan(10);
    for (const address of addresses) {
      for (const credential of secrets) {
        expect(address, "a credential in an address (D243 §3)").not.toContain(credential);
      }
    }
  },
);

/**
 * One person talks alone, the other never pressing Talk, and the other hears them (D281 §1). The
 * host's page loads again to play, so its voice device comes after the guest's: the guest offers
 * each call and the host's answers. An answerer's microphone must go out on the transceiver the
 * offer made, and an iPhone's went nowhere (the owner's test, 2026-10-11).
 */
for (const talker of ["the guest, offering", "the host, answering"] as const) {
  test(`${talker} the call, who alone talks, is heard by one who never pressed Talk (D281 §1)`, async ({
    browser,
  }) => {
    const hosting = await pageWithMicrophone(browser);
    const joining = await pageWithMicrophone(browser);
    const host = hosting.page;
    const guest = joining.page;
    await signIn(host);
    const link = await startSessionWithLink(host);
    await guest.goto(link);
    await join(guest, link);
    await hostPlays(host, warmUpFilm);
    await expectHearing(host, guest);
    const [speaker, listener] = talker === "the host, answering" ? [host, guest] : [guest, host];
    await talkAlone(speaker);
    await expectTalkRingMoves(speaker);
    await expectHeard([["the listener", listener]]);
    await endSession(host);
    await hosting.context.close();
    await joining.context.close();
  });
}

test(
  "voice between two fake microphones arrives one-way under 300 ms with under 2% concealed (D243 §9)",
  { tag: timingTag },
  async ({ browser }) => {
    const hosting = await pageWithMicrophone(browser);
    const joining = await pageWithMicrophone(browser);
    const host = hosting.page;
    const guest = joining.page;
    await signIn(host);
    const link = await startSessionWithLink(host);
    await guest.goto(link);
    await join(guest, link);
    await hostPlays(host, warmUpFilm);
    await talkBothWays(host, guest);
    await host.waitForTimeout(talkForMs);

    await expectVoiceWithinBounds([
      ["the host", host],
      ["the guest", guest],
    ]);
    await endSession(host);
    await hosting.context.close();
    await joining.context.close();
  },
);

/** Where the page's film stands, in seconds. */
async function playedTo(page: Page): Promise<number> {
  return await page.locator("video").evaluate((video: HTMLVideoElement) => video.currentTime);
}

test(
  "the film plays on for the group as the host leaves it, and Watch Together's dialog stops it for everyone (D364 §15, D384 §2)",
  { tag: oneBrowserTag },
  async ({ browser }) => {
    const hostContext = await browser.newContext();
    const guestContext = await browser.newContext();
    try {
      const host = await hostContext.newPage();
      const guest = await guestContext.newPage();
      await signIn(host);
      const link = await startSessionWithLink(host);
      await guest.goto(link);
      await join(guest, link);
      await hostPlays(host, warmUpFilm);
      await expect.poll(() => isPlaying(guest), { timeout: 30_000 }).toBe(true);
      // The host leaves the film for home; its blue sign in the tray opens Watch Together's dialog
      // over the page, never a page of its own (D384 §2).
      await host.goto(new URL("/", host.url()).href);
      const sign = host.locator(".status-tray button[aria-label^='Watch Together']");
      await expect(sign).toHaveClass(/status-tray__button--on/u);
      await sign.click();
      await expect(host.locator("video")).toHaveCount(0);
      expect(new URL(host.url()).pathname).toBe("/");
      await expect(host.getByRole("link", { name: "Back to the film" })).toBeVisible();
      // Leaving stops nothing: the guest's film goes on (D246 §5's stop on leaving withdrawn).
      const before = await playedTo(guest);
      await expect.poll(() => playedTo(guest), { timeout: 10_000 }).toBeGreaterThan(before + 1);
      expect(await isPlaying(guest)).toBe(true);
      // Stop, in the dialog, stops it for everyone.
      await host.getByRole("button", { name: "Stop the film for everyone" }).click();
      await expect.poll(() => isPlaying(guest), { timeout: 10_000 }).toBe(false);
      await endSession(host);
    } finally {
      await hostContext.close();
      await guestContext.close();
    }
  },
);
