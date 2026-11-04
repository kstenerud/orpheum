// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { CastSignOverTheFilmContext } from "./cast-sign-over-the-film.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
import { castSignOverTheFilm } from "./status-tray.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { TogetherPanel } from "./together-over-the-film.tsx";

/**
 * The signs in the overlay's top row, over the film: the film fills the window over the status tray
 * (D418 §1), so what the tray's signs open is opened from over the film too (D422, D423).
 */
describe("the player's overlay's signs over the film", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  async function drawn(): Promise<void> {
    const video = document.createElement("video");
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        // As the signed-in app hands it to the overlay (app.tsx).
        <CastSignOverTheFilmContext value={castSignOverTheFilm}>
          <div className="film-player__stage">
            <PlayerOverlay
              videoRef={videoRef}
              readLinkKbps={() => undefined}
              failure={undefined}
              offer={undefined}
              moves={undefined}
            />
          </div>
        </CastSignOverTheFilmContext>,
      ),
    );
    mounted.container.querySelector(".film-player__stage")?.prepend(video);
  }

  /** The labels of the top row's buttons, in order. */
  function topLabels(): (string | null)[] {
    const top = mounted?.container.querySelector(".player-overlay__top");
    return [...(top?.querySelectorAll("button") ?? [])].map((button) =>
      button.getAttribute("aria-label"),
    );
  }

  it("draws Cast's sign last in its top row, over the film", async () => {
    await drawn();
    const top = mounted?.container.querySelector(".player-overlay__top");
    const last = top?.lastElementChild?.querySelector("button");
    expect(last?.getAttribute("aria-label")).toBe("Cast to a device");
    expect(last?.classList.contains("icon-button--over-video")).toBe(true);
    expect(topLabels()).not.toContain("Watch Together is on: its controls");
  });

  it("draws Watch Together's sign just before Cast's while a session's panel is drawn (D423)", async () => {
    await drawn();
    const panel = await mount(<TogetherPanel>{null}</TogetherPanel>);
    try {
      expect(topLabels().slice(-2)).toEqual([
        "Watch Together is on: its controls",
        "Cast to a device",
      ]);
    } finally {
      panel.unmount();
    }
  });
});
