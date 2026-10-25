// SPDX-License-Identifier: MPL-2.0

/**
 * A browser reaching its core through a discovery (discovery-mode.md W4b.6; webrtc-transport.md W5):
 * the core enrols with a single-target discovery and is claimed; a registration link, which leads to
 * the core's own client origin (D308), opened on the discovery's loader there, finds the core,
 * reaches it over WebRTC through the discovery's offer (the discovery publishes no direct name, so
 * the race has only WebRTC), checks the core's answer by the link's fingerprint, runs the core's own
 * client once its signed manifest holds, registers, and plays a film. Every public part of the
 * discovery meets the core, the loader and the web client here.
 */

import { createHash } from "node:crypto";

import { expect, request, type APIRequestContext, type Locator, type Page } from "@playwright/test";

import { test } from "./browser-map.ts";
import {
  answerField,
  connectAndClaim,
  fieldOf,
  ownersToken,
  signedIn,
  startDiscovery,
  type Discovery,
} from "./discovery-pages.ts";
import { isPlaying, serverUrl, signIn } from "./pages.ts";
import { secondFilm, warmUpFilm } from "./server.ts";
import {
  endSession,
  expectHeard,
  hostPlays,
  join,
  pageWithMicrophone,
  startSessionWithLink,
  talkBothWays,
} from "./together-pages.ts";

/** A link's locate hash, as the client makes it (discovery/link.LocateHash). */
function locateHash(secret: string): string {
  return createHash("sha256").update(`orpheum-locate${secret}`).digest("base64url");
}

/** The discovery the tests here share, with the owner's API, claimed once for them all. */
interface Claimed {
  readonly discovery: Discovery;
  readonly api: APIRequestContext;
  readonly token: string;
}

let claimed: Claimed | undefined;

/** The discovery claimed once for the tests here (beforeAll). */
function claimedOnce(): Claimed {
  if (claimed === undefined) {
    throw new Error("no discovery was claimed for this test");
  }
  return claimed;
}

// One discovery, its core claimed once, for every test here: a claim waits for the core's next look
// whether it is claimed, and ten claims were much of the browser tier's time (the browser test audit,
// 2026-10-18). The core holds one discovery: the realm test, in a spec of its own, starts another.
test.beforeAll(async () => {
  test.setTimeout(120_000);
  const discovery = await startDiscovery();
  const api = await request.newContext();
  const token = await ownersToken(api);
  claimed = { discovery, api, token };
  await connectAndClaim(api, token, discovery.address);
});

test.afterAll(async () => {
  if (claimed === undefined) {
    return;
  }
  const { discovery, api, token } = claimed;
  claimed = undefined;
  // The core leaves the discovery, as the realm test, or the next browser, finds it free.
  await api.delete(`${serverUrl()}/api/v1/system/discovery`, { headers: signedIn(token) });
  await api.dispose();
  discovery.process.kill();
});

/**
 * An invitation as a person is sent it, in the discovery's form: at the core's own client origin
 * (D308), by the core's id and fingerprint. Returned once the discovery finds the core by it, online
 * on its signalling connection.
 */
async function invitationThrough(
  api: APIRequestContext,
  token: string,
  discovery: Discovery,
): Promise<string> {
  const invited = await api.post(`${serverUrl()}/api/v1/invitations`, {
    headers: signedIn(token),
    data: { role: "admin", adminNote: "through the discovery" },
  });
  expect(invited.status()).toBe(201);
  const secret = fieldOf(await answerField(invited, "link"), "secret");
  const core: unknown = await (await api.get(`${discovery.address}/dev/core`)).json();
  const coreId = fieldOf(core, "coreId");
  const fingerprint = fieldOf(core, "fingerprint");
  if (typeof secret !== "string" || typeof coreId !== "string" || typeof fingerprint !== "string") {
    throw new Error("the invitation or the discovery's core is not as made");
  }
  await awaitLocatable(api, discovery.address, secret);
  return `${discovery.clientOriginOf(coreId)}/register#${coreId}.${secret}.${fingerprint}`;
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
  const { discovery, api, token } = claimedOnce();
  const link = await invitationThrough(api, token, discovery);
  const movedToWebrtc = page.waitForEvent("console", (message) =>
    message.text().includes("requests now go over WebRTC"),
  );
  await page.goto(link);
  await page.getByRole("button", { name: "Connect" }).click();
  await movedToWebrtc;
  // A name of each browser's own: the browsers share one core.
  await registerAndOpenFilms(page, `guest-${test.info().project.name}`);
  const card = page
    .locator(".poster-grid a.poster-card__link")
    .filter({ hasText: new RegExp(warmUpFilm, "u") });
  await expect(card).toBeVisible({ timeout: 60_000 });
  await card.click();
  await page.getByRole("link", { name: "Play" }).click();
  await expect.poll(() => isPlaying(page), { timeout: 60_000 }).toBe(true);
});

/**
 * The core's name as a published one is: HTTPS, under its id, which alone a page takes as a direct
 * path to it (§3.3; the device code audit's H1).
 */
function coreNameOf(coreId: string): string {
  return `https://v4.${coreId}.orpheum.test`;
}

/** Whether url is at any core's published name, as coreNameOf makes them. */
const atACoreName = /^https:\/\/v4\.[a-z0-9]+\.orpheum\.test(?=\/|$)/u;

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
  entries.push(["paths", [{ kind: "ipv4", url: coreNameOf(String(fieldOf(body, "coreId"))) }]]);
  return Object.fromEntries(entries);
}

/** Whether the page plays the second film, twice the warm-up's ten seconds. */
async function isPlayingSecondFilm(page: Page): Promise<boolean> {
  return await page
    .locator("video")
    .evaluate(
      (video: HTMLVideoElement) => video.duration > 15 && video.currentTime > 0 && !video.paused,
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
 * Has the page's discovery name the core directly, at its coreNameOf, as the paid service names a core's
 * published name (the single-target discovery publishes none), and carries each request to that name
 * to the core's own listener, with the page's headers as they are: the browser applies CORS to the
 * answers as it would to the core's at its name.
 */
async function nameTheCoreDirectly(
  page: Page,
  clientOrigin: string,
  discovery: string,
): Promise<void> {
  for (const call of ["locate", "paths"]) {
    await page.route(`${clientOrigin}/client/v1/${call}`, async (route) => {
      // Node, which fetches a routed request, resolves no *.localhost name: the discovery's own
      // loopback address serves every name it is reached by.
      const answer = await route.fetch({
        url: route.request().url().replace(clientOrigin, discovery),
      });
      await route.fulfill({ response: answer, json: withDirectPath(await answer.json()) });
    });
  }
  await page.context().route(atACoreName, async (route) => {
    // Firefox's routed request lacks the Origin header the browser sends, which CORS answers by,
    // and names the core's name as its Host, which the core's listener refuses as a name it was not given:
    // at its real name the core would be asked by that name, over HTTPS.
    const headers = Object.fromEntries(
      Object.entries(route.request().headers()).filter(([name]) => name !== "host"),
    );
    try {
      const answer = await route.fetch({
        url: route.request().url().replace(atACoreName, serverUrl()),
        headers: { ...headers, origin: clientOrigin },
      });
      await route.fulfill({ response: answer });
    } catch (error) {
      // A request the page gave up on while its answer came, as a player does a chunk it no longer
      // needs, is answered by no one, as the core's own would be; its fetched answer is disposed,
      // and fulfilling it failed the test (check-pr, 2026-10-18).
      if (!isAbandoned(error)) {
        throw error;
      }
    }
  });
}

/** Whether a routed request's handling failed because the page gave the request up, or closed. */
function isAbandoned(error: unknown): boolean {
  return (
    error instanceof Error &&
    /has been disposed|has been closed|Route is already handled|Request context disposed/u.test(
      error.message,
    )
  );
}

test("a page on the discovery's origin plays from its core's own name, its poster and film by the header alone", async ({
  page,
}) => {
  test.setTimeout(240_000);
  const { discovery, api, token } = claimedOnce();
  // Every request the page makes of the core, which is another origin than the page's.
  const toCore: { url: string; authorization: string | undefined; cookie: string | undefined }[] =
    [];
  page.on("request", (made) => {
    if (atACoreName.test(made.url()) && !made.url().endsWith("/api/v1/health")) {
      const headers = made.headers();
      toCore.push({
        url: made.url(),
        authorization: headers["authorization"],
        cookie: headers["cookie"],
      });
    }
  });
  const link = await invitationThrough(api, token, discovery);
  await nameTheCoreDirectly(page, new URL(link).origin, discovery.address);
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
    .locator(".poster-grid a.poster-card__link")
    .filter({ hasText: new RegExp(warmUpFilm, "u") });
  await expect(card).toBeVisible({ timeout: 60_000 });
  // Its poster, fetched from the core with the header into a blob, shows.
  await expect.poll(() => posterShows(card), { timeout: 30_000 }).toBe(true);
  await card.click();
  await page.getByRole("link", { name: "Play" }).click();
  await expect.poll(() => isPlaying(page), { timeout: 60_000 }).toBe(true);
  // Nothing reached the core by a cookie: every request carried the header, but the registration
  // made before the page had one.
  expect(toCore.filter((made) => made.cookie !== undefined)).toEqual([]);
  // So did the loader's loads of the core's own client, public files it checks by the manifest the
  // core signed (D308 §6), before any session.
  const loadedByTheLoader = /^https:\/\/[^/]+\/(api\/v1\/client-manifest|assets\/.*)?$/u;
  const signedIn = toCore.filter(
    (made) =>
      !/\/api\/v1\/registration(\/link)?$/u.test(made.url) && !loadedByTheLoader.test(made.url),
  );
  expect(signedIn.length).toBeGreaterThan(0);
  expect(signedIn.filter((made) => made.authorization === undefined)).toEqual([]);
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
  const { discovery, api, token } = claimedOnce();
  const hosting = await pageWithMicrophone(browser);
  const joining = await pageWithMicrophone(browser);
  const host = hosting.page;
  const guest = joining.page;
  try {
    // The guest has an account of their own here, as the owner's phone had: their page's own sign-in
    // goes beside their membership on every load of the film (staging, 2026-10-14). They reach the
    // core over WebRTC through the discovery, the path no test routing carries: the film's bytes
    // through Playwright's routing held up both pages' calls for minutes.
    await guest.goto(await invitationThrough(api, token, discovery));
    await guest.getByRole("button", { name: "Connect" }).click();
    await registerAndOpenFilms(guest, `viewer-${test.info().project.name}`);
    await signIn(host);
    const link = await startSessionWithLink(host);
    // The core's link leads to its own client origin (D308).
    expect(new URL(link).hostname.endsWith(".localhost")).toBe(true);
    const fragment = link.slice(link.indexOf("#") + 1).split(".");
    await awaitLocatable(api, discovery.address, fragment[1] ?? "");
    await guest.goto(link);
    await guest.getByRole("button", { name: "Connect" }).click();
    await join(guest, link);
    await hostPlays(host, warmUpFilm);
    // The film goes on for the guest too, its loads carrying their membership.
    await expect.poll(() => isPlaying(guest), { timeout: 60_000 }).toBe(true);
    // That voice arrives through the discovery, not how well: together.spec.ts holds the bounds.
    await talkBothWays(host, guest);
    await expectHeard([
      ["the host", host],
      ["the guest", guest],
    ]);
    if (test.info().project.name === "chromium") {
      // The browser stops an idle service worker, and the one it starts again knows no page's path:
      // the guest's next film must still come over its transport (staging, 2026-10-14).
      const debugging = await joining.context.newCDPSession(guest);
      await debugging.send("ServiceWorker.enable");
      await debugging.send("ServiceWorker.stopAllWorkers");
      await hostPlays(host, secondFilm);
      await expect.poll(() => isPlayingSecondFilm(guest), { timeout: 60_000 }).toBe(true);
    }
    await endSession(host);
  } finally {
    await hosting.context.close();
    await joining.context.close();
  }
});

/** How many of the page's WebRTC transport's connections are connected: those carrying data alone. */
async function transportConnected(page: Page): Promise<number> {
  return await page
    .evaluate(() => {
      const kept: unknown = Reflect.get(window, "keptPeers");
      return (Array.isArray(kept) ? kept : []).filter(
        (each: unknown) =>
          each instanceof RTCPeerConnection &&
          each.connectionState === "connected" &&
          each.getTransceivers().length === 0,
      ).length;
    })
    .catch(() => 0);
}

/**
 * Cuts every connection of the page's transport as a lost network does: each closes, and says so,
 * which a connection closed by the page itself would not. The ones cut are those connected as it
 * begins: the page opens their replacements into the same list at once, which must be left alone.
 */
async function cutTransport(page: Page): Promise<void> {
  await page.evaluate(() => {
    const kept: unknown = Reflect.get(window, "keptPeers");
    const connected = (Array.isArray(kept) ? kept : []).filter(
      (each: unknown): each is RTCPeerConnection =>
        each instanceof RTCPeerConnection &&
        each.connectionState === "connected" &&
        each.getTransceivers().length === 0,
    );
    for (const each of connected) {
      each.close();
      each.dispatchEvent(new Event("connectionstatechange"));
    }
  });
}

/**
 * A guest as the owner's phone was (2026-10-15): not signed in, joining by the session's link on the
 * discovery's page, over WebRTC. The link's key proves only the offer that spends it, so the guest's
 * later offers, its transport's growth and every replacement, are proven by the membership the core
 * gave it: its transport grows to four, comes back after every connection is lost, and the page,
 * loaded again, finds the server by that membership without the spent link.
 */
test("a guest not signed in keeps reaching the core through the discovery after joining: more connections, lost ones replaced, a reload", async ({
  browser,
}) => {
  test.setTimeout(300_000);
  const { discovery, api } = claimedOnce();
  const hosting = await pageWithMicrophone(browser);
  const joining = await pageWithMicrophone(browser);
  const host = hosting.page;
  const guest = joining.page;
  try {
    await signIn(host);
    const link = await startSessionWithLink(host);
    const fragment = link.slice(link.indexOf("#") + 1).split(".");
    await awaitLocatable(api, discovery.address, fragment[1] ?? "");
    await guest.goto(link);
    await guest.getByRole("button", { name: "Connect" }).click();
    await join(guest, link);
    // Its transport grows past the connection the link's offer opened.
    await expect.poll(() => transportConnected(guest), { timeout: 60_000 }).toBe(4);
    await hostPlays(host, warmUpFilm);
    await expect.poll(() => isPlaying(guest), { timeout: 60_000 }).toBe(true);
    // Wi-Fi goes off: every connection is gone, and the ones that replace them carry the next film.
    await cutTransport(guest);
    await expect.poll(() => transportConnected(guest), { timeout: 60_000 }).toBeGreaterThan(0);
    await hostPlays(host, secondFilm);
    await expect.poll(() => isPlayingSecondFilm(guest), { timeout: 60_000 }).toBe(true);
    // Loaded again, with the link spent and gone from the address, the page finds the server.
    await guest.reload();
    await expect.poll(() => transportConnected(guest), { timeout: 60_000 }).toBeGreaterThan(0);
    await expect(guest.getByText("Which server?")).toHaveCount(0);
    await endSession(host);
  } finally {
    await hosting.context.close();
    await joining.context.close();
  }
});
