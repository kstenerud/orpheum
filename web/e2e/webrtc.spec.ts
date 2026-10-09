// SPDX-License-Identifier: MPL-2.0

import { expect, test } from "@playwright/test";

import { serverUrl, signIn } from "./pages.ts";
import { warmUpFilm } from "./server.ts";

/** Where the API's paths are, which over the WebRTC path go on data channels and never over HTTP. */
const apiPrefix = "/api/v1/";

test("once its WebRTC connections are made, a tab sends the API's requests over data channels, not HTTP (D273 §8)", async ({
  page,
}) => {
  await signIn(page);
  // The app says in the console when its requests move, once every connection is made.
  const switched = page.waitForEvent("console", (message) =>
    message.text().includes("requests now go over WebRTC"),
  );
  await page.goto(`${serverUrl()}/?transport=webrtc`);
  await switched;

  const overHttp: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes(apiPrefix)) {
      overHttp.push(`${request.method()} ${new URL(request.url()).pathname}`);
    }
  });
  // Within the app, as a person moves about: a reload would start the app again, on HTTP until it
  // connects anew.
  await page.getByRole("link", { name: "Films", exact: true }).click();
  const card = page.locator("a.poster-grid__card").filter({ hasText: new RegExp(warmUpFilm, "u") });
  await expect(card).toBeVisible({ timeout: 60_000 });
  await card.click();
  await expect(page.getByRole("heading", { name: warmUpFilm })).toBeVisible();
  expect(overHttp).toEqual([]);
});
