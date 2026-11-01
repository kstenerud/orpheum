// SPDX-License-Identifier: MPL-2.0

/**
 * A page whose part's code does not come on its first load is not left black (part-load.ts): the
 * owner's Firefox, signing in at staging for the first time, met "Unable to preload CSS for
 * …/busy-group-….css" thrown past main.tsx's import, and the page stayed black until a reload
 * (2026-11-01).
 */

import { expect } from "@playwright/test";

import { test } from "./browser-map.ts";
import { oneBrowserTag } from "./only-these-tests.ts";
import { signIn } from "./pages.ts";

test(
  "a client whose stylesheet fails to come on its first load is asked again and opens",
  { tag: oneBrowserTag },
  async ({ page }) => {
    let refused = 0;
    await page.route(/\/assets\/client-main-[^/]+\.css$/u, async (route) => {
      if (refused === 0) {
        refused += 1;
        await route.abort();
        return;
      }
      await route.continue();
    });

    await signIn(page);

    expect(refused).toBe(1);
  },
);
