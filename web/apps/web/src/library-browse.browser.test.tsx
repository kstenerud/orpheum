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

  // The owner, 2026-10-28: a chooser looked "no different from a regular button, so there's no way
  // to know that it would bring up a choice list".
  it("marks each chooser by a chevron at its button's end, drawn inside it, after its choice", async () => {
    await page.viewport(1440, 1000);
    walking = await mountApp(walkServices(oneLibrary, "/browse?library=1"));
    await expect.poll(() => isDrawn(".route-chooser__button")).toBe(true);
    const buttons = [...document.querySelectorAll(".route-chooser__button")];
    expect(buttons.length).toBeGreaterThan(0);
    for (const button of buttons) {
      const chevron = button.querySelector("svg.route-chooser__chevron");
      expect(chevron, button.textContent).not.toBeNull();
      const box = button.getBoundingClientRect();
      const mark = chevron?.getBoundingClientRect() ?? new DOMRect();
      expect(mark.width, button.textContent).toBeGreaterThanOrEqual(16);
      expect(mark.height, button.textContent).toBeGreaterThanOrEqual(16);
      expect(mark.left, button.textContent).toBeGreaterThanOrEqual(box.left);
      expect(mark.right, button.textContent).toBeLessThanOrEqual(box.right);
      expect(mark.top, button.textContent).toBeGreaterThanOrEqual(box.top);
      expect(mark.bottom, button.textContent).toBeLessThanOrEqual(box.bottom);
      // At the end, after the choice's words.
      expect(button.lastElementChild, button.textContent).toBe(chevron);
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
