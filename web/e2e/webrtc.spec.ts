// SPDX-License-Identifier: MPL-2.0

import { expect, test } from "@playwright/test";

import { isPlaying, serverUrl, signIn } from "./pages.ts";
import { warmUpFilm } from "./server.ts";

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
  const card = page.locator("a.poster-grid__card").filter({ hasText: new RegExp(warmUpFilm, "u") });
  await expect(card).toBeVisible({ timeout: 60_000 });
  await card.click();
  await expect(page.getByRole("heading", { name: warmUpFilm })).toBeVisible();
  await page.getByRole("link", { name: "Play" }).click();
  await page.getByRole("button", { name: /Built-in speakers/u }).click();
  await expect.poll(() => isPlaying(page), { timeout: 30_000 }).toBe(true);
  expect(overHttp).toEqual([]);
});
