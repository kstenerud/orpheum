// SPDX-License-Identifier: MPL-2.0

/**
 * The films' grid as a real browser lays it out at a Fire TV's page in Silk (960 × 480): six posters
 * across, as Continue Watching shows them, and room around each card for the focus ring, which the
 * grid's off-screen skipping (content-visibility) would otherwise clip (the owner, Silk, 2026-10-25).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { films, isDrawn, mountApp, viewer, walkServices, type Walking } from "./test-walk.tsx";

describe("the films' grid at a Fire TV's page in Silk", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  it("shows six posters across, each with room for its focus ring", async () => {
    await page.viewport(960, 480);
    walking = await mountApp(
      walkServices({
        "GET /me": viewer,
        "GET /client-settings": {},
        "GET /home": { continueWatching: [] },
        "GET /titles": { titles: films(12), nextCursor: "" },
      }),
    );
    await expect.poll(() => isDrawn(".poster-grid__card")).toBe(true);
    const items = [...document.querySelectorAll<HTMLElement>(".poster-grid > li")];
    const firstRowTop = items[0]?.getBoundingClientRect().top;
    const across = items.filter((item) => item.getBoundingClientRect().top === firstRowTop).length;
    expect(across).toBe(6);
    const card = items[0]?.querySelector(".poster-grid__card");
    const item = items[0];
    if (card === null || card === undefined || item === undefined) {
      throw new Error("the grid has no card");
    }
    // The ring reaches twice its width outside the card; the item's box must hold it.
    const ringRoom = 6;
    expect(
      card.getBoundingClientRect().left - item.getBoundingClientRect().left,
    ).toBeGreaterThanOrEqual(ringRoom);
  });
});
