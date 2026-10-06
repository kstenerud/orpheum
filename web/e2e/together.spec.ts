// SPDX-License-Identifier: MPL-2.0

/**
 * Watch Together between two browsers, a host signed in and a guest by their link (M4's
 * acceptance, D252): the link works once, and only once joined; no credential is ever in an
 * address; and voice between two fake microphones arrives within D243 §9's bounds.
 */

import { readFileSync } from "node:fs";
import { expect, test, type Browser, type BrowserContext, type Page } from "@playwright/test";

import { isPlaying, openFilm, serverUrl, signIn } from "./pages.ts";
import { warmUpFilm } from "./server.ts";

/** D243 §9: voice one-way under 300 ms, and audible loss under 2%. */
const voiceOneWayAtMostMs = 300;
const voiceConcealedAtMostShare = 0.02;
/** How long two people talk before their voice is measured. */
const talkForMs = 15_000;

/** The host's session, started on the Watch Together screen, with one link made for Anna. */
async function startSessionWithLink(host: Page): Promise<string> {
  await host.goto(`${serverUrl()}/watch-together`);
  await host.getByRole("button", { name: "Start a session" }).click();
  await host.getByLabel("Who it is for (only you see this)").fill("Anna");
  await host.getByRole("button", { name: "Make a link" }).click();
  const link = host.locator("output.share__link");
  await expect(link).toContainText("#");
  return (await link.textContent()) ?? "";
}

/** Ends the host's session, so the next test, or the next browser's run, starts with none. */
async function endSession(host: Page): Promise<void> {
  await host.goto(`${serverUrl()}/watch-together`);
  await host.getByRole("button", { name: "End the session" }).click();
  await host.getByRole("button", { name: "End it" }).click();
  await expect(host.getByRole("button", { name: "Start a session" })).toBeVisible();
}

/** The guest joins by link with their name, and says they listen with headphones. */
async function join(guest: Page, link: string): Promise<void> {
  await guest.getByLabel("Your name").fill("Anna");
  await guest.getByRole("button", { name: "Join" }).click();
  await guest.getByRole("button", { name: /Headphones/ }).click();
  await expect(guest.getByText(/Here: /)).toBeVisible();
  expect(guest.url()).not.toContain(link.slice(link.indexOf("#") + 1));
}

/** The host plays the warm-up film, which goes on for everyone in the session. */
async function hostPlays(host: Page): Promise<void> {
  await openFilm(host, warmUpFilm);
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

test("an invitation works once, only once joined, and no credential is ever in an address", async ({
  browser,
}, testInfo) => {
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
  await hostPlays(host);
  await expect.poll(() => isPlaying(guest), { timeout: 30_000 }).toBe(true);

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
});

/** What one browser's voice call measures, from its own WebRTC statistics. */
interface VoiceMeasure {
  readonly roundTripMs: number;
  readonly jitterBufferMs: number;
  readonly concealedShare: number;
  readonly samplesReceived: number;
}

/** Keeps every RTCPeerConnection the page makes, so its statistics can be read. */
async function keepPeers(page: Page): Promise<void> {
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
async function measureVoice(page: Page): Promise<VoiceMeasure> {
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
async function pageWithMicrophone(
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

test("voice between two fake microphones arrives one-way under 300 ms with under 2% concealed (D243 §9)", async ({
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
  await hostPlays(host);
  await host.getByRole("button", { name: "Talk" }).click();
  await guest.getByRole("button", { name: "Talk" }).click();
  await expect(host.getByText("Voice: connected to Anna")).toBeVisible({ timeout: 30_000 });
  await expect(guest.getByText(/Voice: connected to /)).toBeVisible({ timeout: 30_000 });
  await host.waitForTimeout(talkForMs);

  for (const [who, page] of [
    ["the host", host],
    ["the guest", guest],
  ] as const) {
    const measured = await measureVoice(page);
    test.info().annotations.push({ type: `${who}'s voice`, description: JSON.stringify(measured) });
    expect(measured.samplesReceived, `${who} heard nothing`).toBeGreaterThan(0);
    const oneWayMs = measured.roundTripMs / 2 + measured.jitterBufferMs;
    expect(oneWayMs, `${who}'s one-way delay`).toBeLessThan(voiceOneWayAtMostMs);
    expect(measured.concealedShare, `${who}'s concealed share`).toBeLessThan(
      voiceConcealedAtMostShare,
    );
  }
  await endSession(host);
  await hosting.context.close();
  await joining.context.close();
});
