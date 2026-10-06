// SPDX-License-Identifier: MPL-2.0

/**
 * What Watch Together's browser tests do to their pages: the host's session and link, the guest's
 * joining, the host's film, and each page's voice call, measured from its own WebRTC statistics.
 */

import { expect, test, type Browser, type BrowserContext, type Page } from "@playwright/test";

import { openFilm, serverUrl } from "./pages.ts";

/** D243 §9: voice one-way under 300 ms, and audible loss under 2%. */
const voiceOneWayAtMostMs = 300;
const voiceConcealedAtMostShare = 0.02;

/** The host's session, started on the Watch Together screen, with one link made for Anna. */
export async function startSessionWithLink(host: Page): Promise<string> {
  await host.goto(`${serverUrl()}/watch-together`);
  await host.getByRole("button", { name: "Start a session" }).click();
  await host.getByLabel("Who it is for (only you see this)").fill("Anna");
  await host.getByRole("button", { name: "Make a link" }).click();
  const link = host.locator("output.share__link");
  await expect(link).toContainText("#");
  return (await link.textContent()) ?? "";
}

/** Ends the host's session, so the next test, or the next browser's run, starts with none. */
export async function endSession(host: Page): Promise<void> {
  await host.goto(`${serverUrl()}/watch-together`);
  await host.getByRole("button", { name: "End the session" }).click();
  await host.getByRole("button", { name: "End it" }).click();
  await expect(host.getByRole("button", { name: "Start a session" })).toBeVisible();
}

/** The guest joins by link with their name, and says they listen with headphones. */
export async function join(guest: Page, link: string): Promise<void> {
  await guest.getByLabel("Your name").fill("Anna");
  await guest.getByRole("button", { name: "Join" }).click();
  await guest.getByRole("button", { name: /Headphones/ }).click();
  await expect(guest.getByText(/Here: /)).toBeVisible();
  expect(guest.url()).not.toContain(link.slice(link.indexOf("#") + 1));
}

/** The host plays film, which goes on for everyone in the session. */
export async function hostPlays(host: Page, film: string): Promise<void> {
  await openFilm(host, film);
  await host.getByRole("link", { name: "Play" }).click();
  // A device not asked yet is asked how it plays sound first; one asked before plays at once.
  const question = host.getByRole("button", { name: /Built-in speakers/ });
  const leading = host.getByText("Watch Together is on");
  await expect(question.or(leading)).toBeVisible({ timeout: 30_000 });
  if (await question.isVisible()) {
    await question.click();
  }
  await expect(leading).toBeVisible({ timeout: 30_000 });
}

/** What one browser's voice call measures, from its own WebRTC statistics. */
export interface VoiceMeasure {
  readonly roundTripMs: number;
  readonly jitterBufferMs: number;
  readonly concealedShare: number;
  readonly samplesReceived: number;
}

/** Keeps every RTCPeerConnection the page makes, so its statistics can be read. */
export async function keepPeers(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const Original = window.RTCPeerConnection;
    const kept: RTCPeerConnection[] = [];
    Reflect.set(window, "keptPeers", kept);
    window.RTCPeerConnection = class extends Original {
      constructor(config?: RTCConfiguration) {
        super(config);
        kept.push(this);
      }
    };
  });
}

/** The page's voice call, measured: its round trip, its jitter buffer and its concealed share. */
export async function measureVoice(page: Page): Promise<VoiceMeasure> {
  return await page.evaluate(async () => {
    const kept: unknown = Reflect.get(window, "keptPeers");
    const peer = (Array.isArray(kept) ? kept : []).find(
      (each: unknown): each is RTCPeerConnection =>
        each instanceof RTCPeerConnection && each.connectionState === "connected",
    );
    if (peer === undefined) {
      throw new Error("no voice call connected");
    }
    let roundTripMs = Number.NaN;
    let jitterBufferMs = Number.NaN;
    let concealedShare = Number.NaN;
    let samplesReceived = 0;
    (await peer.getStats()).forEach((report: Record<string, unknown>) => {
      const number = (key: string): number => {
        const value = report[key];
        return typeof value === "number" ? value : Number.NaN;
      };
      if (report["type"] === "remote-inbound-rtp" && report["kind"] === "audio") {
        roundTripMs = number("roundTripTime") * 1000;
      }
      if (report["type"] === "inbound-rtp" && report["kind"] === "audio") {
        jitterBufferMs = (number("jitterBufferDelay") / number("jitterBufferEmittedCount")) * 1000;
        samplesReceived = number("totalSamplesReceived");
        concealedShare = number("concealedSamples") / samplesReceived;
      }
    });
    return { roundTripMs, jitterBufferMs, concealedShare, samplesReceived };
  });
}

/** A page whose browser hands it a fake microphone without asking. */
export async function pageWithMicrophone(
  browser: Browser,
): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext();
  if (browser.browserType().name() === "chromium") {
    await context.grantPermissions(["microphone"]);
  }
  const page = await context.newPage();
  await keepPeers(page);
  return { context, page };
}

/**
 * Each named page's voice, measured and held to D243 §9's bounds: heard at all, one-way (half
 * the round trip plus the jitter buffer) under 300 ms, and under 2% of its samples concealed.
 */
export async function expectVoiceWithinBounds(
  pages: readonly (readonly [string, Page])[],
): Promise<void> {
  for (const [who, page] of pages) {
    const measured = await measureVoice(page);
    process.stdout.write(`${who}'s voice: ${JSON.stringify(measured)}\n`);
    test.info().annotations.push({ type: `${who}'s voice`, description: JSON.stringify(measured) });
    expect(measured.samplesReceived, `${who} heard nothing`).toBeGreaterThan(0);
    const oneWayMs = measured.roundTripMs / 2 + measured.jitterBufferMs;
    expect(oneWayMs, `${who}'s one-way delay`).toBeLessThan(voiceOneWayAtMostMs);
    expect(measured.concealedShare, `${who}'s concealed share`).toBeLessThan(
      voiceConcealedAtMostShare,
    );
  }
}

/**
 * Both turn voice on, and each hears the other connected within connectsWithinMs: a call's set-up
 * retries through a lossy network, and took over 30 s under the congested profile.
 */
export async function talkBothWays(
  host: Page,
  guest: Page,
  connectsWithinMs = 30_000,
): Promise<void> {
  await host.getByRole("button", { name: "Talk" }).click();
  await guest.getByRole("button", { name: "Talk" }).click();
  await expect(host.getByText("Voice: connected to Anna")).toBeVisible({
    timeout: connectsWithinMs,
  });
  await expect(guest.getByText(/Voice: connected to /)).toBeVisible({ timeout: connectsWithinMs });
}
