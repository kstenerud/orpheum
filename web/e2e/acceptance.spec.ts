// SPDX-License-Identifier: MPL-2.0

/**
 * M4's acceptance (cut-a's M4 row; D243 §9; D252): under each network profile, shaped by the
 * kernel's netem on the loopback every packet between the browsers and the server crosses, a host
 * in Chrome and a guest in Firefox stay within 200 ms of each other 95% of the time for the run,
 * and their voice arrives within its bounds at the end.
 */

import { execFileSync } from "node:child_process";
import { existsSync, writeFileSync } from "node:fs";
import { expect, firefox, test, type Page } from "@playwright/test";

import { acceptanceFilm, acceptanceMinutes, networkProfiles } from "./acceptance-server.ts";
import { firefoxUserPrefs } from "./browsers.ts";
import { signIn } from "./pages.ts";
import {
  endSession,
  expectVoiceWithinBounds,
  hostPlays,
  join,
  pageWithMicrophone,
  startSessionWithLink,
  talkBothWays,
} from "./together-pages.ts";

/** D243 §9: two clients within 200 ms of each other 95% of the time. */
const inStepWithinMs = 200;
const inStepAtLeastShare = 0.95;
/** How long a voice call may take to connect under a profile's loss and delay. */
const voiceConnectsWithinMs = 120_000;
/** How often the two players' positions are compared. */
const sampleEveryMs = 1000;

/** iproute2's tc, which the development image lets the developer run on the loopback (D252 §1). */
function tc(...tcArguments: readonly string[]): void {
  const binary = ["/usr/sbin/tc", "/sbin/tc"].find((path) => existsSync(path));
  if (binary === undefined) {
    throw new Error("tc is not installed: run the acceptance in the development image (./dev)");
  }
  execFileSync(binary, tcArguments, { stdio: "pipe" });
}

/** Where a page's player is, and when it was read, on the machine's clock both pages share. */
async function positionOf(
  page: Page,
): Promise<{ atMs: number; positionMs: number; isPlaying: boolean }> {
  return await page
    .locator("video")
    .first()
    .evaluate((video: HTMLVideoElement) => ({
      atMs: Date.now(),
      positionMs: video.currentTime * 1000,
      isPlaying: !video.paused && video.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA,
    }));
}

/**
 * How far apart the two players are, in ms: each one's position taken back to a common moment
 * along its own clock, the group playing at rate 1.
 */
async function offsetMs(host: Page, guest: Page): Promise<number> {
  const [hostAt, guestAt] = await Promise.all([positionOf(host), positionOf(guest)]);
  return hostAt.positionMs - hostAt.atMs - (guestAt.positionMs - guestAt.atMs);
}

/** The value at the share of the sorted values, e.g. 0.95 for the 95th percentile. */
function percentile(values: readonly number[], share: number): number {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.min(sorted.length - 1, Math.floor(share * sorted.length))] ?? Number.NaN;
}

/**
 * Both pages as they were, kept in the failed run's results folder: what each person saw says
 * why.
 */
async function keepPages(pages: readonly (readonly [string, Page])[]): Promise<void> {
  for (const [who, page] of pages) {
    const name = who.replaceAll(" ", "-");
    await page.screenshot({ fullPage: true, path: test.info().outputPath(`${name}.png`) });
    writeFileSync(test.info().outputPath(`${name}.txt`), await page.locator("body").innerText());
  }
}

for (const profile of networkProfiles) {
  test(`two browsers stay in step and talk under the ${profile.name} profile`, async ({
    browser,
  }) => {
    tc("qdisc", "replace", "dev", "lo", "root", "netem", ...profile.netem);
    const guestBrowser = await firefox.launch({ firefoxUserPrefs });
    try {
      const hosting = await pageWithMicrophone(browser);
      const joining = await pageWithMicrophone(guestBrowser);
      const host = hosting.page;
      const guest = joining.page;
      await signIn(host);
      const link = await startSessionWithLink(host);
      await guest.goto(link);
      await join(guest, link);
      try {
        await hostPlays(host, acceptanceFilm(profile.name));
        await talkBothWays(host, guest, voiceConnectsWithinMs);
        await runAndMeasure(profile.name, host, guest);
      } catch (error) {
        await keepPages([
          ["the host", host],
          ["the guest", guest],
        ]);
        throw error;
      }
      await endSession(host);
      await hosting.context.close();
      await joining.context.close();
    } finally {
      await guestBrowser.close();
      tc("qdisc", "del", "dev", "lo", "root");
    }
  });
}

/**
 * Plays for the run's minutes, comparing the two players every second from the first moment the
 * guest is in step, and holds the share in step and the voice to D243 §9.
 */
async function runAndMeasure(profile: string, host: Page, guest: Page): Promise<void> {
  // Counted from the first moment the guest is in step: joining and loading hold no one up
  // (D246 §6), and are not what the bound is about.
  await expect
    .poll(
      async () => {
        const offset = Math.abs(await offsetMs(host, guest));
        process.stdout.write(
          `${profile}: waiting for the guest to be in step, ${offset.toFixed(0)} ms apart\n`,
        );
        return offset;
      },
      { timeout: 120_000, intervals: [5000] },
    )
    .toBeLessThanOrEqual(inStepWithinMs);
  const offsets: number[] = [];
  const endsAtMs = Date.now() + acceptanceMinutes() * 60_000;
  while (Date.now() < endsAtMs) {
    offsets.push(Math.abs(await offsetMs(host, guest)));
    await host.waitForTimeout(sampleEveryMs);
  }
  const inStepShare = offsets.filter((offset) => offset <= inStepWithinMs).length / offsets.length;
  const measured = JSON.stringify({
    samples: offsets.length,
    inStepShare,
    medianMs: percentile(offsets, 0.5),
    p95Ms: percentile(offsets, 0.95),
    maxMs: Math.max(...offsets),
  });
  // Printed as well as kept: an hour and a half's run is read from its output.
  process.stdout.write(`${profile}: in step ${measured}\n`);
  test.info().annotations.push({ type: `${profile}: in step`, description: measured });
  expect(inStepShare, `in step within ${String(inStepWithinMs)} ms`).toBeGreaterThanOrEqual(
    inStepAtLeastShare,
  );
  await expectVoiceWithinBounds([
    ["the host", host],
    ["the guest", guest],
  ]);
}
