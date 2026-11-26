// SPDX-License-Identifier: MPL-2.0

/**
 * A Watch Together guest by link's top bar as a real browser lays it out, with the stylesheets the
 * viewer loads (D555 §9): the product's mark and the session's name, then Cast and Leave at its end.
 * The guest's ⏮ and ⏭ are the Play page's overlay's (player-overlay-layout.browser.test.tsx).
 */

import "./tokens.css";
import "./base.css";
import "./together-viewer.css";
import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { mount } from "./test-dom.tsx";

describe("a Watch Together guest by link's top bar, laid out by a real browser", () => {
  it("has the mark and the session's name at its start, and Cast and Leave on one row at its end", async () => {
    // As GuestTopBar draws it, over the page's film.
    const mounted = await mount(
      <div className="together-viewer" style={{ width: "900px" }}>
        <header className="together-viewer__bar">
          <img className="together-viewer__logo" alt="Orpheum" style={{ width: "120px" }} />
          <h1 className="together-viewer__heading">Watching together</h1>
          <div className="together-viewer__bar-actions">
            <button type="button">Cast</button>
            <button type="button">Leave</button>
          </div>
        </header>
      </div>,
    );
    try {
      /** The one element of the bar's that selector finds. */
      const drawn = (selector: string): HTMLElement => {
        const found = mounted.container.querySelector<HTMLElement>(selector);
        if (found === null) {
          throw new Error(`the top bar has no ${selector}`);
        }
        return found;
      };
      const bar = drawn(".together-viewer__bar");
      const logo = drawn(".together-viewer__bar img");
      const heading = drawn(".together-viewer__bar h1");
      const cast = drawn(".together-viewer__bar-actions button:first-child");
      const leave = drawn(".together-viewer__bar-actions button:last-child");
      expect(cast.textContent).toBe("Cast");
      expect(leave.textContent).toBe("Leave");
      const box = bar.getBoundingClientRect();
      const [mark, name, before, after] = [logo, heading, cast, leave].map((each) =>
        each.getBoundingClientRect(),
      );
      if (mark === undefined || name === undefined || before === undefined || after === undefined) {
        throw new Error("no boxes");
      }
      expect(after.width).toBeGreaterThan(0);
      // One row, each centred on the same line.
      const middle = (each: DOMRect): number => each.top + each.height / 2;
      expect(Math.abs(middle(mark) - middle(name))).toBeLessThan(2);
      expect(Math.abs(middle(before) - middle(name))).toBeLessThan(2);
      expect(Math.abs(middle(after) - middle(name))).toBeLessThan(2);
      // The mark, then the name, at the bar's start.
      expect(name.left).toBeGreaterThanOrEqual(mark.right);
      expect(mark.left - box.left).toBeLessThan(24);
      // Cast, then Leave, close beside it, at the bar's end.
      expect(after.left).toBeGreaterThanOrEqual(before.right);
      expect(after.left - before.right).toBeLessThan(24);
      expect(box.right - after.right).toBeLessThan(24);
      expect(before.left).toBeGreaterThan(name.right);
    } finally {
      mounted.unmount();
    }
  });
});
