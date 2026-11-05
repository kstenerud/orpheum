// SPDX-License-Identifier: MPL-2.0

/**
 * What Watch Together's browser tests do to their pages: the host's session and link, the guest's
 * joining, the host's film, and each page's voice call, measured from its own WebRTC statistics.
 */

import { expect, type Browser, type BrowserContext, type Page } from "@playwright/test";

import { test } from "./browser-map.ts";
import { openFilm, playFromTheStart, serverUrl, shownOverTheFilm } from "./pages.ts";

/** D243 §9: voice one-way under 300 ms, and audible loss under 2%. */
const voiceOneWayAtMostMs = 300;
const voiceConcealedAtMostShare = 0.02;

/**
 * Opens Watch Together's dialog over the host's home page by the tray's sign (D384 §2), which is
 * where its session is started and run.
 */
export async function openTogether(host: Page): Promise<void> {
  await host.goto(`${serverUrl()}/`);
  await openTogetherHere(host);
}

/** Opens Watch Together's dialog over the host's page as it is, by the tray's sign. */
async function openTogetherHere(host: Page): Promise<void> {
  await host.locator(".status-tray button[aria-label^='Watch Together']").click();
  await expect(host.getByRole("dialog", { name: "Watch Together" })).toBeVisible();
}

/**
 * Ends the host's session from Watch Together's dialog, open over their page, which closes it, and
 * nothing opens it again (D385's Also).
 */
async function endFromTheDialog(host: Page): Promise<void> {
  await host.getByRole("button", { name: "End the session" }).click();
  await host.getByRole("button", { name: "End it" }).click();
  await expect(host.getByRole("dialog", { name: "Watch Together" })).toHaveCount(0);
  await expect(host.locator(".status-tray button[aria-label='Watch Together']")).toBeVisible();
}

/** The host's session, started in Watch Together's dialog, with one link made for Anna. */
export async function startSessionWithLink(host: Page): Promise<string> {
  await openTogether(host);
  // A session an earlier test left open, one failing before it ended it, is the host's on every
  // device signed in (D267 §2): the screen shows it, not Start. It is ended first.
  const start = host.getByRole("button", { name: "Start a session" });
  const end = host.getByRole("button", { name: "End the session" });
  await expect(start.or(end)).toBeVisible();
  if (await end.isVisible()) {
    await endFromTheDialog(host);
    await openTogetherHere(host);
  }
  await start.click();
  // Inviting is a button that opens a dialog of its own (D281 §5).
  await host.getByRole("button", { name: "Invite someone" }).click();
  await host.getByLabel("Who it is for (only you see this)").fill("Anna");
  await host.getByRole("button", { name: "Make a link" }).click();
  const link = host.locator("output.share__link");
  await expect(link).toContainText("#");
  return (await link.textContent()) ?? "";
}

/** Ends the host's session, so the next test, or the next browser's run, starts with none. */
export async function endSession(host: Page): Promise<void> {
  await openTogether(host);
  await endFromTheDialog(host);
}

/** The guest joins by link with their name, asked nothing of how they hear the film (D336 §4). */
export async function join(guest: Page, link: string): Promise<void> {
  await guest.getByLabel("Your name").fill("Anna");
  await guest.getByRole("button", { name: "Join" }).click();
  await expect(guest.getByText(/Here: /)).toBeVisible();
  expect(guest.url()).not.toContain(link.slice(link.indexOf("#") + 1));
}

/** The host plays film, which goes on for everyone in the session. */
export async function hostPlays(host: Page, film: string): Promise<void> {
  await openFilm(host, film);
  await playFromTheStart(host).click();
  await expect(host.locator("video")).toHaveCount(1, { timeout: 30_000 });
  await expect(await shownOverTheFilm(host, togetherOverTheFilm)).toBeVisible({ timeout: 30_000 });
}

/** The label of Watch Together's sign over the film, shown while a session is on (D423). */
const togetherOverTheFilm = "Watch Together is on: its controls";

/**
 * Opens Watch Together's dialog over the film by its sign there, the session's panel moved in under
 * its controls, the film filling the window over the page that holds it (D418 §1, D423, D433); a page
 * whose film does not fill it shows the panel where it stands.
 */
export async function openPanelOverTheFilm(page: Page): Promise<void> {
  const dialog = page.getByRole("dialog", { name: "Watch Together" });
  if (
    (await dialog.count()) > 0 ||
    (await page.locator(".film-player__stage--fills-window").count()) === 0
  ) {
    return;
  }
  await (await shownOverTheFilm(page, togetherOverTheFilm)).click();
  await expect(dialog).toBeVisible();
}

/** What one browser's voice call measures, from its own WebRTC statistics. */
export interface VoiceMeasure {
  readonly roundTripMs: number;
  /** How long the jitter buffer held the audio, on average, and what it aimed at, on average. */
  readonly jitterBufferMs: number;
  readonly jitterBufferTargetMs: number;
  /** The share of samples concealed apart from silence: what a listener hears go missing. */
  readonly concealedShare: number;
  readonly samplesReceived: number;
  /** Packets never received, packets that arrived too late to play, and those FEC recovered. */
  readonly packetsLost: number;
  readonly packetsLate: number;
  readonly fecPacketsReceived: number;
  /** How the call reached the other side: "direct", or the relay's "udp", "tcp" or "tls". */
  readonly path: string;
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

/** The page's voice call, measured from its own WebRTC statistics (voiceMeasureOf). */
export async function measureVoice(page: Page): Promise<VoiceMeasure> {
  return voiceMeasureOf(await voiceReports(page));
}

/** The reports of the page's connected voice call's statistics, as they are now. */
export async function voiceReports(page: Page): Promise<Record<string, unknown>[]> {
  return await page.evaluate(async () => {
    const kept: unknown = Reflect.get(window, "keptPeers");
    // The call is the connection carrying audio: a page on the discovery's origin that reached its
    // core over WebRTC holds that transport's connections too, with data channels alone.
    const peer = (Array.isArray(kept) ? kept : []).find(
      (each: unknown): each is RTCPeerConnection =>
        each instanceof RTCPeerConnection &&
        each.connectionState === "connected" &&
        each.getReceivers().some((receiver) => receiver.track.kind === "audio"),
    );
    if (peer === undefined) {
      throw new Error("no voice call connected");
    }
    const all: Record<string, unknown>[] = [];
    (await peer.getStats()).forEach((report: Record<string, unknown>) => {
      all.push({ ...report });
    });
    return all;
  });
}

/**
 * A call's measure between two readings of its reports: what its statistics, which count from the
 * call's start, added from before to after, so a window with no film beside it is measured alone.
 */
export function voiceMeasureBetween(
  before: readonly Record<string, unknown>[],
  after: readonly Record<string, unknown>[],
): VoiceMeasure {
  const counted = (reports: readonly Record<string, unknown>[]): Record<string, unknown>[] =>
    reports.map((report) => ({ ...report }));
  const inboundOf = (reports: readonly Record<string, unknown>[]): Record<string, unknown> =>
    reports.find((report) => report["type"] === "inbound-rtp" && report["kind"] === "audio") ?? {};
  const was = inboundOf(before);
  const window = counted(after).map((report) => {
    if (report["type"] !== "inbound-rtp" || report["kind"] !== "audio") {
      return report;
    }
    const since: Record<string, unknown> = { ...report };
    for (const key of [
      "jitterBufferDelay",
      "jitterBufferTargetDelay",
      "jitterBufferEmittedCount",
      "totalSamplesReceived",
      "concealedSamples",
      "silentConcealedSamples",
      "packetsLost",
      "packetsDiscarded",
      "fecPacketsReceived",
    ]) {
      const now = report[key];
      const then = was[key];
      if (typeof now === "number" && typeof then === "number") {
        since[key] = now - then;
      }
    }
    return since;
  });
  return voiceMeasureOf(window);
}

/** A call's measure from its statistics' reports: its audio's, its chosen path's. */
export function voiceMeasureOf(reports: readonly Record<string, unknown>[]): VoiceMeasure {
  const number = (report: Record<string, unknown> | undefined, key: string): number => {
    const value = report?.[key];
    return typeof value === "number" ? value : Number.NaN;
  };
  const audio = (type: string): Record<string, unknown> | undefined =>
    reports.find((report) => report["type"] === type && report["kind"] === "audio");
  const inbound = audio("inbound-rtp");
  const emitted = number(inbound, "jitterBufferEmittedCount");
  const samplesReceived = number(inbound, "totalSamplesReceived");
  const silent = number(inbound, "silentConcealedSamples");
  const audible = number(inbound, "concealedSamples") - (Number.isNaN(silent) ? 0 : silent);
  const pair = reports.find(
    (report) =>
      report["type"] === "candidate-pair" &&
      report["nominated"] === true &&
      report["state"] === "succeeded",
  );
  const local = reports.find((report) => report["id"] === pair?.["localCandidateId"]);
  const protocol = local?.["relayProtocol"];
  const relayed = typeof protocol === "string" ? protocol : "relay";
  return {
    roundTripMs: number(audio("remote-inbound-rtp"), "roundTripTime") * 1000,
    jitterBufferMs: (number(inbound, "jitterBufferDelay") / emitted) * 1000,
    jitterBufferTargetMs: (number(inbound, "jitterBufferTargetDelay") / emitted) * 1000,
    concealedShare: audible / samplesReceived,
    samplesReceived: Number.isNaN(samplesReceived) ? 0 : samplesReceived,
    packetsLost: number(inbound, "packetsLost"),
    packetsLate: number(inbound, "packetsDiscarded"),
    fecPacketsReceived: number(inbound, "fecPacketsReceived"),
    path: local?.["candidateType"] === "relay" ? relayed : "direct",
  };
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
 * Each named page's voice, measured and held to D243 §9's bounds (expectVoiceMeasureWithinBounds).
 */
export async function expectVoiceWithinBounds(
  pages: readonly (readonly [string, Page])[],
  isBestEffort = false,
): Promise<void> {
  for (const [who, page] of pages) {
    expectVoiceMeasureWithinBounds(who, await measureVoice(page), isBestEffort);
  }
}

/**
 * Each named page hears voice within heardWithinMs, held best effort (only heard at all) once it
 * does: a test whose claim is that voice arrives, not how well, waits for it rather than a fixed
 * time (the browser test audit, 2026-10-18).
 */
export async function expectHeard(
  pages: readonly (readonly [string, Page])[],
  heardWithinMs = 30_000,
): Promise<void> {
  for (const [who, page] of pages) {
    await expect
      .poll(async () => (await measureVoice(page)).samplesReceived, {
        message: `${who} heard nothing`,
        timeout: heardWithinMs,
      })
      .toBeGreaterThan(0);
  }
  await expectVoiceWithinBounds(pages, true);
}

/**
 * A voice's measure, printed and held to D243 §9's bounds: heard at all, one-way (half the round
 * trip plus the jitter buffer) under 300 ms, and under 2% of its samples concealed apart from
 * silence. Best effort, as voice is on a congested link (D261 §7), only heard at all is held.
 */
export function expectVoiceMeasureWithinBounds(
  who: string,
  measured: VoiceMeasure,
  isBestEffort: boolean,
): void {
  process.stdout.write(`${who}'s voice: ${JSON.stringify(measured)}\n`);
  test.info().annotations.push({ type: `${who}'s voice`, description: JSON.stringify(measured) });
  expect(measured.samplesReceived, `${who} heard nothing`).toBeGreaterThan(0);
  if (isBestEffort) {
    return;
  }
  const oneWayMs = measured.roundTripMs / 2 + measured.jitterBufferMs;
  expect(oneWayMs, `${who}'s one-way delay`).toBeLessThan(voiceOneWayAtMostMs);
  expect(measured.concealedShare, `${who}'s concealed share`).toBeLessThan(
    voiceConcealedAtMostShare,
  );
}

/**
 * Presses Talk until voice is on, as a person whose press missed would press again: the lines
 * above the voice controls appear as the film starts and can move Talk from under a press made at
 * that moment; a guest's voice stayed off with nothing said, its press never reaching the page, in
 * two acceptance runs on 2026-10-07.
 */
async function turnVoiceOn(page: Page): Promise<void> {
  await openPanelOverTheFilm(page);
  await expect(async () => {
    const talk = page.getByRole("button", { name: "Talk" });
    if (await talk.isVisible()) {
      await talk.click();
    }
    await expect(page.getByRole("button", { name: "Stop talking" })).toBeVisible({ timeout: 2000 });
  }).toPass({ timeout: 30_000 });
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
  await turnVoiceOn(host);
  await turnVoiceOn(guest);
  await expectHearing(host, guest, connectsWithinMs);
}

/**
 * Each page hears the other connected within connectsWithinMs: every page hears voice from the
 * start, whoever talks (D281 §1).
 */
export async function expectHearing(
  host: Page,
  guest: Page,
  connectsWithinMs = 30_000,
): Promise<void> {
  await openPanelOverTheFilm(host);
  await openPanelOverTheFilm(guest);
  await expect(host.getByText("Voice: hearing Anna")).toBeVisible({ timeout: connectsWithinMs });
  await expect(guest.getByText(/Voice: hearing /)).toBeVisible({ timeout: connectsWithinMs });
}

/**
 * The Talk button's ring moves with the voice going out while its person talks (D281 §7): it read
 * the call's statistics, as an iPhone's microphone meter reads zeros (the owner, 2026-10-11).
 */
export async function expectTalkRingMoves(page: Page): Promise<void> {
  // Firefox's fake microphone gives the ring nothing to read here, the committed code reading 0 as
  // the new one does, though the owner's Firefox moves it: the ring is held in Chromium alone.
  if (test.info().project.name !== "chromium") {
    return;
  }
  await expect(async () => {
    const level = await page
      .locator(".together-voice__talk")
      .evaluate((button: HTMLElement) => Number(button.style.getPropertyValue("--level")));
    expect(level).toBeGreaterThan(0);
  }).toPass({ timeout: 10_000 });
}

/** Turns this page's voice on alone: its person talks, and the others only listen (D281 §1). */
export async function talkAlone(page: Page): Promise<void> {
  await turnVoiceOn(page);
}
