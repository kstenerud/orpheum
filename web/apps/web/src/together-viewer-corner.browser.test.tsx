// SPDX-License-Identifier: MPL-2.0

/**
 * The Watch Together guest's corner of the film as a real browser lays it out, with the stylesheets
 * the viewer loads: Full screen, and ⏮ and ⏭ before it for a guest given next and previous (D471).
 */

import "./tokens.css";
import "./base.css";
import "./film-player.css";
import "./player-controls.css";
import "./together-viewer.css";
import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { SequenceMoveButton } from "./player-controls.tsx";
import { mount } from "./test-dom.tsx";

describe("a Watch Together guest's corner of the film, laid out by a real browser", () => {
  it("has ⏮ and ⏭ beside Full screen, in that order on one row, at the film's lower corner", async () => {
    // As the viewer's FilmButtons draw them, in the stage over a film's picture.
    const mounted = await mount(
      <div className="film-player__stage" style={{ width: "640px", height: "360px" }}>
        <div className="together-viewer__corner">
          <SequenceMoveButton isOverVideo isForward={false} onMove={() => undefined} />
          <SequenceMoveButton isOverVideo isForward onMove={() => undefined} />
          <button type="button" className="together-viewer__fullscreen">
            Full screen
          </button>
        </div>
      </div>,
    );
    try {
      const stage = mounted.container.querySelector<HTMLElement>(".film-player__stage");
      const [previous, next, fullScreen] = [
        ...mounted.container.querySelectorAll<HTMLElement>(".together-viewer__corner button"),
      ];
      if (
        stage === null ||
        previous === undefined ||
        next === undefined ||
        fullScreen === undefined
      ) {
        throw new Error("the corner is not drawn");
      }
      expect(previous.getAttribute("aria-label")).toBe("Previous episode");
      expect(next.getAttribute("aria-label")).toBe("Next episode");
      const box = stage.getBoundingClientRect();
      const [before, after, full] = [previous, next, fullScreen].map((each) =>
        each.getBoundingClientRect(),
      );
      if (before === undefined || after === undefined || full === undefined) {
        throw new Error("no boxes");
      }
      expect(full.width).toBeGreaterThan(0);
      // One row, each centred on the same line.
      const middle = (each: DOMRect): number => each.top + each.height / 2;
      expect(Math.abs(middle(before) - middle(full))).toBeLessThan(1);
      expect(Math.abs(middle(after) - middle(full))).toBeLessThan(1);
      // ⏮, then ⏭, then Full screen, each close beside the one before it.
      expect(after.left).toBeGreaterThanOrEqual(before.right);
      expect(full.left).toBeGreaterThanOrEqual(after.right);
      expect(full.left - after.right).toBeLessThan(24);
      expect(after.left - before.right).toBeLessThan(24);
      // At the film's lower corner, its inline end.
      expect(box.right - full.right).toBeLessThan(24);
      expect(box.bottom - full.bottom).toBeLessThan(24);
      expect(box.bottom - before.bottom).toBeLessThan(24);
    } finally {
      mounted.unmount();
    }
  });
});
