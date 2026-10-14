// SPDX-License-Identifier: MPL-2.0

/**
 * A failure the page meets reaches the core's log, not only the browser's console, which no one
 * reads on a phone (staging, 2026-10-14).
 */

import { expect } from "@playwright/test";

import { test } from "./browser-map.ts";
import { serverUrl, signIn } from "./pages.ts";

test("a failure the page meets is sent to the core, with the page's path", async ({ page }) => {
  await signIn(page);
  // The film page's own: Firefox also reports the home page's requests the navigation cut off.
  const sent = page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      request.url().endsWith("/api/v1/client-problems") &&
      request.postData()?.includes('"page":"/play/999999"') === true,
  );
  // A film no library holds: the decision fails, and the page reports it.
  await page.goto(`${serverUrl()}/play/999999`);
  const body: unknown = (await sent).postDataJSON();
  expect(body).toMatchObject({ page: "/play/999999" });
});
