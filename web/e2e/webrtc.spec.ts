// SPDX-License-Identifier: MPL-2.0

import { expect } from "@playwright/test";

import { test } from "./browser-map.ts";
import { oneBrowserTag } from "./only-these-tests.ts";
import { isPlaying, serverUrl, signIn } from "./pages.ts";
import { warmUpFilm } from "./server.ts";
import { endSession, join } from "./together-pages.ts";

/**
 * A tab on the WebRTC path sends nothing of the API over HTTP, its own requests or the loads the
 * browser makes by itself, which the service worker hands to the page's transport (D273 §8).
 */
test("once on the WebRTC path, a tab finds and plays a film with nothing of the API over HTTP (D273 §8)", async ({
  page,
  context,
}) => {
  await signIn(page);
  // The app says in the console when its requests move, and when its media does.
  const moved = (what: string): Promise<unknown> =>
    page.waitForEvent("console", (message) =>
      message.text().includes(`${what} now go over WebRTC`),
    );
  const requestsMoved = moved("requests");
  const mediaMoved = page.waitForEvent("console", (message) =>
    message.text().includes("media now goes over WebRTC"),
  );
  await page.goto(`${serverUrl()}/?transport=webrtc`);
  await Promise.all([requestsMoved, mediaMoved]);

  // What reaches the network: a load the service worker answers through the page never does.
  const overHttp: string[] = [];
  await context.route("**/api/v1/**", async (route) => {
    overHttp.push(`${route.request().method()} ${new URL(route.request().url()).pathname}`);
    await route.continue();
  });
  // Within the app, as a person moves about: a reload would start the app again, on HTTP until it
  // connects anew.
  await page.getByRole("link", { name: "Films", exact: true }).click();
  const card = page
    .locator(".poster-grid a.poster-card__link")
    .filter({ hasText: new RegExp(warmUpFilm, "u") });
  await expect(card).toBeVisible({ timeout: 60_000 });
  await card.click();
  await expect(page.getByRole("heading", { name: warmUpFilm })).toBeVisible();
  await page.getByRole("link", { name: "Play" }).click();
  await expect.poll(() => isPlaying(page), { timeout: 30_000 }).toBe(true);
  expect(overHttp).toEqual([]);
});

/**
 * A host on the WebRTC path runs a Watch Together session over a data channel, never a WebSocket,
 * and a guest on HTTP joins it and follows the host's film (D273 §8, W3).
 */
test(
  "a host on the WebRTC path runs a Watch Together session with no WebSocket, and a guest follows it",
  { tag: oneBrowserTag },
  async ({ browser }) => {
    const host = await (await browser.newContext()).newPage();
    await signIn(host);
    const moved = host.waitForEvent("console", (message) =>
      message.text().includes("media now goes over WebRTC"),
    );
    await host.goto(`${serverUrl()}/?transport=webrtc`);
    await moved;
    const sockets: string[] = [];
    host.on("websocket", (socket) => sockets.push(socket.url()));

    // Watch Together's sign opens its dialog over the page (D384 §2).
    await host.getByRole("button", { name: "Watch Together", exact: true }).click();
    const start = host.getByRole("button", { name: "Start a session" });
    const end = host.getByRole("button", { name: "End the session" });
    await expect(start.or(end)).toBeVisible();
    if (await end.isVisible()) {
      await end.click();
      await host.getByRole("button", { name: "End it" }).click();
      // Ending it closes the dialog (D385's Also): the sign opens it again to start one.
      await host.getByRole("button", { name: "Watch Together", exact: true }).click();
    }
    await start.click();
    // Inviting is a button that opens a dialog of its own (D281 §5).
    await host.getByRole("button", { name: "Invite someone" }).click();
    await host.getByLabel("Who it is for (only you see this)").fill("Anna");
    await host.getByRole("button", { name: "Make a link" }).click();
    const shown = host.locator("output.share__link");
    await expect(shown).toContainText("#");
    const link = (await shown.textContent()) ?? "";
    // The link's dialog is modal (D281 §5): it is closed before the page is used again.
    await host.locator(".watch-together__dialog").getByRole("button", { name: "Close" }).click();
    await host.locator("dialog.together-dialog > .modal-dialog__close").click();

    const guest = await (await browser.newContext()).newPage();
    await guest.goto(link);
    await join(guest, link);

    await host.getByRole("link", { name: "Films", exact: true }).click();
    const card = host
      .locator(".poster-grid a.poster-card__link")
      .filter({ hasText: new RegExp(warmUpFilm, "u") });
    await expect(card).toBeVisible({ timeout: 60_000 });
    await card.click();
    await host.getByRole("link", { name: "Play" }).click();
    await expect(host.getByText("Watch Together is on")).toBeVisible({ timeout: 30_000 });
    await expect.poll(() => isPlaying(guest), { timeout: 30_000 }).toBe(true);
    expect(sockets).toEqual([]);
    await endSession(host);
  },
);
