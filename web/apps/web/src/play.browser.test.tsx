// SPDX-License-Identifier: MPL-2.0

/**
 * The Play page as a real browser lays it out while its film fills the window (play.css): the page
 * under the film is hidden (player-overlay.css), but what the page tells beside the film, a failure
 * or a refusal, is drawn over the picture, a card on the film, not lost in the hidden page.
 */

import "./tokens.css";
import "./base.css";
import "./play.css";
import "./player-overlay.css";

import { afterEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { mount, type Mounted } from "./test-dom.tsx";

describe("the Play page while its film fills the window", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows the page's failure over the picture, the rest of the page hidden under it", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(
      <div className="play">
        <h1 className="play__title">A film</h1>
        <p role="alert">The film could not be played.</p>
        <div className="film-player__stage film-player__stage--fills-window">
          <video className="film-player__video" />
        </div>
      </div>,
    );
    const alert = mounted.container.querySelector<HTMLElement>("[role='alert']");
    const title = mounted.container.querySelector<HTMLElement>(".play__title");
    if (alert === null || title === null) {
      throw new Error("the page was not drawn");
    }
    expect(getComputedStyle(alert).visibility).toBe("visible");
    expect(getComputedStyle(title).visibility).toBe("hidden");
    const box = alert.getBoundingClientRect();
    const onTop = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
    expect(onTop === alert || alert.contains(onTop)).toBe(true);
  });
});
