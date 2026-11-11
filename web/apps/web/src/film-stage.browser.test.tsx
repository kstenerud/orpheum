// SPDX-License-Identifier: MPL-2.0

/**
 * The player's stage as a real browser lays it out, with the stylesheets in the order the app loads
 * them: the overlay's, then the film player's, whose rule for the stage came later and won.
 */

import "./tokens.css";
import "./base.css";
import "./player-overlay.css";
import "./film-player.css";

import { describe, expect, it } from "vitest";

import { mount } from "./test-dom.tsx";

/** A picture four times as tall as it is wide: a video takes its shape from its poster, unplayed. */
const tallPoster =
  "data:image/svg+xml," +
  encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="100" height="400"/>');

describe("the player's stage, laid out by a real browser", () => {
  // The owner, 2026-11-03: in a browser already full screen, which refuses the player's own, The
  // Navigator (4:3) lost its top and bottom: the stage meant to fill the window
  // stayed in the page, and the film was as tall as its width made it. A film taller than the
  // window's shape stands for it here, whatever the window's size.
  it("fills the window with a stage where full screen was refused, the whole film inside it", async () => {
    const mounted = await mount(
      <main style={{ paddingTop: "200px" }}>
        <div className="film-player__stage film-player__stage--fills-window">
          <video className="film-player__video" poster={tallPoster}>
            <track kind="captions" />
          </video>
        </div>
      </main>,
    );
    try {
      const stage = mounted.container.querySelector<HTMLElement>(".film-player__stage");
      const video = mounted.container.querySelector<HTMLVideoElement>("video");
      if (stage === null || video === null) {
        throw new Error("no stage drawn");
      }
      await expect.poll(() => video.getBoundingClientRect().height).toBeGreaterThan(0);
      expect(getComputedStyle(stage).position).toBe("fixed");
      const box = stage.getBoundingClientRect();
      expect(box.top).toBe(0);
      expect(box.height).toBe(window.innerHeight);
      const film = video.getBoundingClientRect();
      expect(film.top).toBeGreaterThanOrEqual(0);
      expect(film.bottom).toBeLessThanOrEqual(window.innerHeight);
      expect(getComputedStyle(video).objectFit).toBe("contain");
    } finally {
      mounted.unmount();
    }
  });
});
