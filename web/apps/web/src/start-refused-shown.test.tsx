// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayerOverlay } from "./player-overlay.tsx";
import { startRefusedEvent } from "./start-refused.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A paused video whose play says it plays. */
function pausedVideo(): HTMLVideoElement {
  const video = document.createElement("video");
  let isPaused = true;
  Object.defineProperty(video, "paused", { get: () => isPaused });
  video.play = () => {
    isPaused = false;
    video.dispatchEvent(new Event("play"));
    return Promise.resolve();
  };
  return video;
}

describe("a film the browser would not start, on the Play page's overlay", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.restoreAllMocks();
  });

  /** Draws the overlay over video in a stage, as the player draws it. */
  async function drawn(video: HTMLVideoElement): Promise<Mounted> {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>,
      ),
    );
    mounted.container.querySelector(".film-player__stage")?.prepend(video);
    return mounted;
  }

  /** What the overlay shows. */
  function shown(): string | null | undefined {
    return mounted?.container.querySelector(".player-overlay")?.getAttribute("data-shown");
  }

  /** The line over the picture. */
  function line(): HTMLElement | null | undefined {
    return mounted?.container.querySelector<HTMLElement>(".player-overlay-refused");
  }

  // Chrome on an iPhone refused a start asked 0.58 s after the press, and the page was black, no
  // control showing (the owner, staging, 2026-11-11).
  it("shows all of itself with Play focused and says why as the browser refuses, until the film plays", async () => {
    const video = pausedVideo();
    await drawn(video);
    expect(shown()).toBe("hidden");
    expect(line()).toBeNull();
    act(() => {
      video.dispatchEvent(new Event(startRefusedEvent));
    });
    expect(shown()).toBe("full");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Play");
    expect(line()).toBeInstanceOf(HTMLElement);
    expect(line()?.tagName).toBe("OUTPUT");
    expect(line()?.textContent).toBe("Paused — press Play to start.");
    act(() => {
      void video.play();
    });
    expect(line()).toBeNull();
  });
});
