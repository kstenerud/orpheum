// SPDX-License-Identifier: MPL-2.0

/**
 * A browser reaching its core through a discovery (discovery-mode.md W4b.6; webrtc-transport.md W5):
 * the core enrols with a single-target discovery and is claimed; a registration link in the
 * discovery's form, opened on the discovery's page, finds the core, reaches it over WebRTC through
 * the discovery's offer (the discovery publishes no direct name, so the race has only WebRTC), checks
 * the core's answer by the link's fingerprint, registers, and plays a film. Every public part of the
 * discovery meets the core and the web client here.
 */

import { spawn, type ChildProcess } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { expect, request, type APIRequestContext, type Locator, type Page } from "@playwright/test";

import { test } from "./browser-map.ts";
import { isPlaying, serverUrl, signIn } from "./pages.ts";
import { owner, timingFilm, warmUpFilm } from "./server.ts";
import {
  endSession,
  expectVoiceWithinBounds,
  hostPlays,
  join,
  pageWithMicrophone,
  startSessionWithLink,
  talkBothWays,
} from "./together-pages.ts";

/** How long the two talk before their voice is measured, as together.spec.ts lets them. */
const talkForMs = 15_000;

/** Starts `orpheum dev discovery` on a free loopback port, and is its process and its address. */
async function startDiscovery(): Promise<{ process: ChildProcess; address: string }> {
  const binary = process.env["ORPHEUM_TEST_BINARY"];
  if (binary === undefined) {
    throw new Error("ORPHEUM_TEST_BINARY names no binary");
  }
  const state = mkdtempSync(path.join(tmpdir(), "orpheum-discovery-"));
  const started = spawn(binary, ["dev", "discovery", "--listen", "127.0.0.1:0", "--state", state], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  let output = "";
  started.stderr.on("data", (chunk: Buffer) => {
    output += chunk.toString();
  });
  const address = await new Promise<string>((resolve, reject) => {
    started.stdout.on("data", (chunk: Buffer) => {
      const served = /serving the discovery at (\S+)/u.exec(chunk.toString());
      if (served?.[1] !== undefined) {
        resolve(served[1]);
      }
    });
    started.once("exit", (code) => {
      reject(new Error(`the discovery exited (${String(code)}):\n${output}`));
    });
  });
  return { process: started, address };
}

/** The owner's session token, signed in over the core's own API. */
async function ownersToken(api: APIRequestContext): Promise<string> {
  const answer = await api.post(`${serverUrl()}/api/v1/session`, {
    data: {
      name: owner.name,
      password: owner.password,
      device: { name: "Discovery test", platform: "web" },
    },
  });
  expect(answer.ok()).toBe(true);
  const token = await answerField(answer, "token");
  if (typeof token !== "string") {
    throw new Error("the sign-in gave no token");
  }
  return token;
}

/** A field of a JSON answer, or of a JSON value. */
function fieldOf(value: unknown, name: string): unknown {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }
  const field: unknown = Reflect.get(value, name);
  return field;
}

/** A field of a JSON answer. */
async function answerField(
  answer: Awaited<ReturnType<APIRequestContext["get"]>>,
  name: string,
): Promise<unknown> {
  return fieldOf(await answer.json(), name);
}

/** The headers a signed-in call carries. */
function signedIn(token: string): Record<string, string> {
  return Object.fromEntries([["Authorization", `Bearer ${token}`]]);
}

/** A link's locate hash, as the client makes it (discovery/link.LocateHash). */
function locateHash(secret: string): string {
  return createHash("sha256").update(`orpheum-locate${secret}`).digest("base64url");
}

/** Connects the core to the discovery as its admin does, and claims it in the operator's place. */
async function connectAndClaim(
  api: APIRequestContext,
  token: string,
  discovery: string,
): Promise<void> {
  const connected = await api.post(`${serverUrl()}/api/v1/system/discovery`, {
    headers: signedIn(token),
    data: { baseUrl: `${discovery}/core/v1` },
  });
  expect(connected.ok()).toBe(true);
  const claimed = await api.post(`${discovery}/dev/claim`, {
    data: { code: await answerField(connected, "claimCode") },
  });
  expect(claimed.status()).toBe(204);
  await expect
    .poll(
      async () =>
        await answerField(
          await api.get(`${serverUrl()}/api/v1/system/discovery`, { headers: signedIn(token) }),
          "state",
        ),
      { timeout: 60_000 },
    )
    .toBe("claimed");
}

/**
 * An invitation as a person is sent it, in the discovery's form: the link the core makes is on its
 * own origin, the discovery's being plain HTTP, so it is written here by the core's id and
 * fingerprint. Returned once the discovery finds the core by it, online on its signalling connection.
 */
async function invitationThrough(
  api: APIRequestContext,
  token: string,
  discovery: string,
): Promise<string> {
  const invited = await api.post(`${serverUrl()}/api/v1/invitations`, {
    headers: signedIn(token),
    data: { role: "admin", adminNote: "through the discovery" },
  });
  expect(invited.status()).toBe(201);
  const secret = fieldOf(await answerField(invited, "link"), "secret");
  const core: unknown = await (await api.get(`${discovery}/dev/core`)).json();
  const coreId = fieldOf(core, "coreId");
  const fingerprint = fieldOf(core, "fingerprint");
  if (typeof secret !== "string" || typeof coreId !== "string" || typeof fingerprint !== "string") {
    throw new Error("the invitation or the discovery's core is not as made");
  }
  await awaitLocatable(api, discovery, secret);
  return `${discovery}/register#${coreId}.${secret}.${fingerprint}`;
}

/** Waits until the discovery finds the core, online, by the link whose secret is secret. */
async function awaitLocatable(
  api: APIRequestContext,
  discovery: string,
  secret: string,
): Promise<void> {
  await expect
    .poll(
      async () =>
        await answerField(
          await api.post(`${discovery}/client/v1/locate`, {
            headers: { "Orpheum-Discovery-Protocol": "1.0" },
            data: { hash: locateHash(secret) },
          }),
          "state",
        ),
      { timeout: 60_000 },
    )
    .toBe("online");
}

test("a browser registers and plays through a discovery, over WebRTC, its core's answer checked by the link's fingerprint", async ({
  page,
}) => {
  test.setTimeout(240_000);
  const discovery = await startDiscovery();
  const api = await request.newContext();
  const token = await ownersToken(api);
  try {
    await connectAndClaim(api, token, discovery.address);
    const link = await invitationThrough(api, token, discovery.address);
    const movedToWebrtc = page.waitForEvent("console", (message) =>
      message.text().includes("requests now go over WebRTC"),
    );
    await page.goto(link);
    await page.getByRole("button", { name: "Connect" }).click();
    await movedToWebrtc;
    // A name of each browser's own: the browsers share one core.
    await registerAndOpenFilms(page, `guest-${test.info().project.name}`);
    const card = page
      .locator("a.poster-grid__card")
      .filter({ hasText: new RegExp(warmUpFilm, "u") });
    await expect(card).toBeVisible({ timeout: 60_000 });
    await card.click();
    await page.getByRole("link", { name: "Play" }).click();
    await page.getByRole("button", { name: /Built-in speakers/u }).click();
    await expect.poll(() => isPlaying(page), { timeout: 60_000 }).toBe(true);
  } finally {
    // The core leaves the discovery, as the next test, or the next browser, finds it free.
    await api.delete(`${serverUrl()}/api/v1/system/discovery`, { headers: signedIn(token) });
    await api.dispose();
    discovery.process.kill();
  }
});

/** The core's name as a published one is: HTTPS, which a page takes as a direct path (§3.3). */
const coreName = "https://core.orpheum.test";

/** A discovery's answer with the core's own name as its one path, its other fields as they were. */
function withDirectPath(body: unknown): Record<string, unknown> {
  const fields = [
    "coreId",
    "apiMajor",
    "state",
    "offlineSince",
    "clientPrefix",
    "iceServers",
    "bootstrapToken",
  ];
  const entries: [string, unknown][] = fields.map((name) => [name, fieldOf(body, name)]);
  entries.push(["paths", [{ kind: "ipv4", url: coreName }]]);
  return Object.fromEntries(entries);
}

/** Whether the page plays a film over a minute long: the timing film, not the warm-up's ten seconds. */
async function isPlayingLongFilm(page: Page): Promise<boolean> {
  return await page
    .locator("video")
    .evaluate(
      (video: HTMLVideoElement) => video.duration > 60 && video.currentTime > 0 && !video.paused,
    )
    .catch(() => false);
}

/** Whether the card's poster has loaded and shows. */
async function posterShows(card: Locator): Promise<boolean> {
  return await card
    .locator("img")
    .first()
    .evaluate((image: HTMLImageElement) => image.complete && image.naturalWidth > 0);
}

/** Registers as a person through the invitation the page was opened on, and opens the films. */
async function registerAndOpenFilms(page: Page, name: string): Promise<void> {
  await page.getByLabel("Your name").fill(name);
  await page.getByLabel("Password", { exact: true }).fill("guest-password");
  await page.getByLabel("The same password again").fill("guest-password");
  await page.getByRole("button", { name: "Set password and sign in" }).click();
  await page.getByRole("link", { name: "Films", exact: true }).click();
}

/**
 * Has the page's discovery name the core directly, at coreName, as the paid service names a core's
 * published name (the single-target discovery publishes none), and carries each request to that name
 * to the core's own listener, with the page's headers as they are: the browser applies CORS to the
 * answers as it would to the core's at its name.
 */
async function nameTheCoreDirectly(page: Page, discovery: string): Promise<void> {
  for (const call of ["locate", "paths"]) {
    await page.route(`${discovery}/client/v1/${call}`, async (route) => {
      const answer = await route.fetch();
      await route.fulfill({ response: answer, json: withDirectPath(await answer.json()) });
    });
  }
  await page.context().route(`${coreName}/**`, async (route) => {
    // Firefox's routed request lacks the Origin header the browser sends, which CORS answers by,
    // and names coreName as its Host, which the core's listener refuses as a name it was not given:
    // at its real name the core would be asked by that name, over HTTPS.
    const headers = Object.fromEntries(
      Object.entries(route.request().headers()).filter(([name]) => name !== "host"),
    );
    const answer = await route.fetch({
      url: route.request().url().replace(coreName, serverUrl()),
      headers: { ...headers, origin: discovery },
    });
    await route.fulfill({ response: answer });
  });
}

test("a page on the discovery's origin plays from its core's own name, its poster and film by the header alone", async ({
  page,
}) => {
  test.setTimeout(240_000);
  const discovery = await startDiscovery();
  const api = await request.newContext();
  const token = await ownersToken(api);
  // Every request the page makes of the core, which is another origin than the page's.
  const toCore: { url: string; authorization: string | undefined; cookie: string | undefined }[] =
    [];
  page.on("request", (made) => {
    if (made.url().startsWith(coreName) && !made.url().endsWith("/api/v1/health")) {
      const headers = made.headers();
      toCore.push({
        url: made.url(),
        authorization: headers["authorization"],
        cookie: headers["cookie"],
      });
    }
  });
  try {
    await connectAndClaim(api, token, discovery.address);
    const link = await invitationThrough(api, token, discovery.address);
    await nameTheCoreDirectly(page, discovery.address);
    const movedToCore = page.waitForEvent("console", (message) =>
      message.text().includes("requests now go over the core's name"),
    );
    await page.goto(link);
    const mediaToCore = page.waitForEvent("console", (message) =>
      message.text().includes("media now goes over the core's name"),
    );
    await page.getByRole("button", { name: "Connect" }).click();
    await movedToCore;
    await mediaToCore;
    await registerAndOpenFilms(page, `direct-${test.info().project.name}`);
    const card = page
      .locator("a.poster-grid__card")
      .filter({ hasText: new RegExp(warmUpFilm, "u") });
    await expect(card).toBeVisible({ timeout: 60_000 });
    // Its poster, fetched from the core with the header into a blob, shows.
    await expect.poll(() => posterShows(card), { timeout: 30_000 }).toBe(true);
    await card.click();
    await page.getByRole("link", { name: "Play" }).click();
    await page.getByRole("button", { name: /Built-in speakers/u }).click();
    await expect.poll(() => isPlaying(page), { timeout: 60_000 }).toBe(true);
    // Nothing reached the core by a cookie: every request carried the header, but the registration
    // made before the page had one.
    expect(toCore.filter((made) => made.cookie !== undefined)).toEqual([]);
    const signedIn = toCore.filter((made) => !/\/api\/v1\/registration(\/link)?$/u.test(made.url));
    expect(signedIn.length).toBeGreaterThan(0);
    expect(signedIn.filter((made) => made.authorization === undefined)).toEqual([]);
  } finally {
    await api.delete(`${serverUrl()}/api/v1/system/discovery`, { headers: signedIn(token) });
    await api.dispose();
    discovery.process.kill();
  }
});

/**
 * Watch Together as the owner and his partner use it (the owner, 2026-10-14): the host at home on the
 * core itself; the guest elsewhere, signed in to an account of their own, opening the session's link,
 * which the core gives in its discovery's form, on the discovery's page, joining with headphones; the
 * film goes on for both, and each hears the other.
 */
test("a guest signed in here joins Watch Together by its link on the discovery's page, plays the film, and talks", async ({
  browser,
}) => {
  test.setTimeout(300_000);
  const discovery = await startDiscovery();
  const api = await request.newContext();
  const token = await ownersToken(api);
  const hosting = await pageWithMicrophone(browser);
  const joining = await pageWithMicrophone(browser);
  const host = hosting.page;
  const guest = joining.page;
  try {
    await connectAndClaim(api, token, discovery.address);
    // The guest has an account of their own here, as the owner's phone had: their page's own sign-in
    // goes beside their membership on every load of the film (staging, 2026-10-14). They reach the
    // core over WebRTC through the discovery, the path no test routing carries: the film's bytes
    // through Playwright's routing held up both pages' calls for minutes.
    await guest.goto(await invitationThrough(api, token, discovery.address));
    await guest.getByRole("button", { name: "Connect" }).click();
    await registerAndOpenFilms(guest, `viewer-${test.info().project.name}`);
    await signIn(host);
    const link = await startSessionWithLink(host);
    expect(link.startsWith(`${discovery.address}/`)).toBe(true);
    const fragment = link.slice(link.indexOf("#") + 1).split(".");
    await awaitLocatable(api, discovery.address, fragment[1] ?? "");
    await guest.goto(link);
    await guest.getByRole("button", { name: "Connect" }).click();
    await join(guest, link);
    await hostPlays(host, warmUpFilm);
    // The film goes on for the guest too, its loads carrying their membership.
    await expect.poll(() => isPlaying(guest), { timeout: 60_000 }).toBe(true);
    await talkBothWays(host, guest);
    await host.waitForTimeout(talkForMs);
    await expectVoiceWithinBounds([
      ["the host", host],
      ["the guest", guest],
    ]);
    if (test.info().project.name === "chromium") {
      // The browser stops an idle service worker, and the one it starts again knows no page's path:
      // the guest's next film must still come over its transport (staging, 2026-10-14).
      const debugging = await joining.context.newCDPSession(guest);
      await debugging.send("ServiceWorker.enable");
      await debugging.send("ServiceWorker.stopAllWorkers");
      await hostPlays(host, timingFilm("chromium"));
      await expect.poll(() => isPlayingLongFilm(guest), { timeout: 60_000 }).toBe(true);
    }
    await endSession(host);
  } finally {
    await hosting.context.close();
    await joining.context.close();
    await api.delete(`${serverUrl()}/api/v1/system/discovery`, { headers: signedIn(token) });
    await api.dispose();
    discovery.process.kill();
  }
});
