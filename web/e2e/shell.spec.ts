// SPDX-License-Identifier: MPL-2.0

/** The app shell as a browser lays it out: what only a stylesheet decides. */

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { expect, type Locator } from "@playwright/test";

import { test } from "./browser-map.ts";
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

test("a poster's status cues are a tray down from its top-right corner, over the poster (D301 §4)", async ({
  page,
}) => {
  await signIn(page);
  const item = page.locator(".poster-grid__item").first();
  await expect(item).toBeVisible();
  // The test's library has nothing that cannot play and no guess: the tray the grid would draw for
  // both is added to a real item, so the stylesheet alone decides where its cues sit.
  await item.evaluate((element) => {
    const tray = document.createElement("span");
    tray.className = "poster-grid__cues";
    for (const icon of ["warning-icon", "uncertain-match-icon"]) {
      const sign = document.createElement("span");
      sign.className = "warning-sign";
      const button = document.createElement("button");
      button.className = "warning-sign__button";
      const drawn = document.createElementNS("http://www.w3.org/2000/svg", "svg");
      drawn.setAttribute("class", icon);
      const disc = document.createElementNS("http://www.w3.org/2000/svg", "circle");
      disc.setAttribute("class", `${icon}__disc`);
      drawn.append(disc);
      button.append(drawn);
      sign.append(button);
      tray.append(sign);
    }
    element.append(tray);
  });
  const poster = await boxOf(item.locator(".poster"));
  const [first, second] = [
    await boxOf(item.locator(".poster-grid__cues > .warning-sign").nth(0)),
    await boxOf(item.locator(".poster-grid__cues > .warning-sign").nth(1)),
  ];
  // In the poster's top-right corner, the second under the first, both over the poster.
  expect(poster.x + poster.width - (first.x + first.width)).toBeLessThan(20);
  expect(first.y - poster.y).toBeLessThan(20);
  expect(first.y).toBeGreaterThanOrEqual(poster.y);
  expect(second.y).toBeGreaterThanOrEqual(first.y + first.height);
  expect(Math.abs(second.x + second.width - (first.x + first.width))).toBeLessThan(2);
  expect(second.y + second.height).toBeLessThan(poster.y + poster.height);
  // The uncertain match's disc in the owner's caution yellow (D301 §4).
  const disc = item.locator(".uncertain-match-icon__disc");
  expect(await disc.evaluate((element) => getComputedStyle(element).fill)).toBe(
    "rgb(247, 206, 70)",
  );
  // Each cue the size of the warning sign's, its icon drawn at the sign's size.
  expect(Math.abs(second.height - first.height)).toBeLessThan(2);
  expect(Math.abs(second.width - first.width)).toBeLessThan(2);
});

/** Where element is drawn; it fails the test when it is drawn nowhere. */
async function boxOf(
  element: Locator,
): Promise<{ x: number; y: number; width: number; height: number }> {
  const box = await element.boundingBox();
  if (box === null) {
    throw new Error("an element the test measures is not drawn");
  }
  return box;
}

/** The client's source folder, beside this one. */
const clientSource = join(import.meta.dirname, "..", "apps", "web", "src");

test("every class the client's stylesheets select is one its code draws", () => {
  const files = readdirSync(clientSource);
  const code = files
    .filter((name) => /\.tsx?$/u.test(name) && !/\.test\.tsx?$/u.test(name))
    .map((name) => readFileSync(join(clientSource, name), "utf8"))
    .join("\n");
  const unused: string[] = [];
  for (const sheet of files.filter((name) => name.endsWith(".css"))) {
    const selected = readFileSync(join(clientSource, sheet), "utf8").matchAll(
      /\.([a-z][a-z0-9-]*(?:__[a-z0-9-]+)?(?:--[a-z0-9-]+)?)/gu,
    );
    for (const [, name] of selected) {
      // A rule for a class nothing draws is dead: kept, it would say a look the page never has.
      if (name !== undefined && !code.includes(name)) {
        unused.push(`${sheet}: .${name}`);
      }
    }
  }
  expect(unused).toEqual([]);
});
