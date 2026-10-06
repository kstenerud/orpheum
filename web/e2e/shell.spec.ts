// SPDX-License-Identifier: MPL-2.0

/** The app shell as a browser lays it out: what only a stylesheet decides. */

import { expect, test } from "@playwright/test";

import { signIn } from "./pages.ts";

test("the logo is one link home, exactly the logo's size", async ({ page }) => {
  await signIn(page);
  const home = page.locator("a.app-shell__home");
  const logo = home.locator("img.app-shell__logo");
  const [link, image] = await Promise.all([home.boundingBox(), logo.boundingBox()]);
  // An inline link around the logo is a line of text tall, not the logo's own box.
  expect(link?.height).toBeCloseTo(image?.height ?? -1, 0);
  expect(link?.width).toBeCloseTo(image?.width ?? -1, 0);
  await home.click();
  await expect(page).toHaveURL(/\/films$/u);
});
