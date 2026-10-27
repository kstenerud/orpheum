// SPDX-License-Identifier: MPL-2.0

/**
 * The Libraries page's bar as a real browser lays it out: a chooser's name beside its button,
 * centred on it with a gap between (the owner's Firefox on a Mac, 2026-10-27: "Sort" sat at the
 * button's top edge and against its side), which jsdom, laying nothing out, cannot see.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { isDrawn, mountApp, viewer, walkServices, type Walking } from "./test-walk.tsx";

describe("the Libraries page's bar in a real browser", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  it("centres a chooser's name on its button, a gap between them", async () => {
    await page.viewport(1440, 1000);
    walking = await mountApp(walkServices(oneLibrary, "/browse?library=1"));
    await expect.poll(() => isDrawn(".route-chooser__field")).toBe(true);
    for (const chooser of document.querySelectorAll(".route-chooser")) {
      const field = chooser.querySelector(".route-chooser__field");
      const button = chooser.querySelector(".route-chooser__button");
      if (field === null || button === null) {
        continue; // the library chooser's name is for a screen reader alone
      }
      const name = field.getBoundingClientRect();
      const box = button.getBoundingClientRect();
      const middle = (rect: DOMRect): number => rect.top + rect.height / 2;
      expect(Math.abs(middle(name) - middle(box)), field.textContent).toBeLessThanOrEqual(2);
      expect(box.left - name.right, field.textContent).toBeGreaterThanOrEqual(4);
    }
  });
});

/** A server with one films library holding one film, as the Libraries page asks it. */
const oneLibrary = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /libraries": {
    libraries: [
      {
        id: 1,
        name: "Films",
        kind: "movies",
        metadataSource: "tmdb",
        changeDetection: "watch",
        isWritable: true,
        createdAt: 0,
      },
    ],
  },
  "GET /titles": {
    titles: [
      {
        id: 1,
        name: "A film",
        kind: "movie",
        identificationState: "identified",
        libraryId: 1,
        addedAt: 1,
      },
    ],
    nextCursor: "",
  },
  "GET /titles/letters": { letters: [] },
};
