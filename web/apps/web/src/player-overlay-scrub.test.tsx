// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayerOverlay } from "./player-overlay.tsx";
import { scrubTapHeldForMs } from "./player-taps.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

// The scrub bar's stray seeks on a phone (the owner, 2026-11-15, option a): a second tap meant for
// the picture landed on the bar the first had just shown.
describe("a finger on the full-screen overlay's scrub bar just shown", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.restoreAllMocks();
  });

  /** A paused video of 600 s standing at 100 s, and where a seek has it stand since (atS). */
  function pausedVideo(): { readonly video: HTMLVideoElement; readonly atS: () => number } {
    const video = document.createElement("video");
    let atS = 100;
    Object.defineProperty(video, "paused", { get: () => true });
    Object.defineProperty(video, "duration", { get: () => 600 });
    Object.defineProperty(video, "currentTime", {
      get: () => atS,
      set: (to: number) => {
        atS = to;
      },
    });
    return { video, atS: () => atS };
  }

  /** Draws the overlay over video, hidden as the player opens (D418 §1). */
  async function drawnOver(video: HTMLVideoElement): Promise<void> {
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
    act(() => {
      video.dispatchEvent(new Event("durationchange"));
    });
  }

  function shown(): string | null | undefined {
    return mounted?.container.querySelector(".player-overlay")?.getAttribute("data-shown");
  }

  /** A finger's tap in the middle of the picture, and the click the browser sends after. */
  function tapThePicture(): void {
    const video = mounted?.container.querySelector("video") ?? document.body;
    const init = { bubbles: true, pointerType: "touch", pointerId: 7, clientX: 500, clientY: 50 };
    act(() => {
      video.dispatchEvent(new PointerEvent("pointerdown", init));
      video.dispatchEvent(new PointerEvent("pointerup", init));
      video.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1, clientX: 500 }));
    });
  }

  /** A finger's touch on the scrub bar at clientX, of the test DOM's events, which have no Touch. */
  function touchOnScrub(type: "touchstart" | "touchmove", clientX: number): void {
    const scrub = mounted?.container.querySelector<HTMLInputElement>(
      ".player-overlay__scrub input",
    );
    if (scrub === null || scrub === undefined) {
      throw new Error("no scrub bar");
    }
    scrub.getBoundingClientRect = () => new DOMRect(0, 0, 1000, 44);
    const event = new Event(type, { bubbles: true, cancelable: true });
    Object.defineProperty(event, "touches", { value: [{ clientX, clientY: 20 }] });
    act(() => {
      scrub.dispatchEvent(event);
    });
  }

  it("does nothing at a tap on the scrub bar for half a second after it shows; a tap after seeks, and a drag at once", async () => {
    const { video, atS } = pausedVideo();
    let pageNowMs = 10_000;
    vi.spyOn(performance, "now").mockImplementation(() => pageNowMs);
    await drawnOver(video);
    tapThePicture();
    expect(shown()).toBe("full");
    pageNowMs += scrubTapHeldForMs - 100;
    touchOnScrub("touchstart", 500); // the half of the film, had it seeked
    touchOnScrub("touchmove", 505); // a tap's wobble
    expect(atS()).toBe(100);
    touchOnScrub("touchmove", 600); // a drag, which follows the finger
    expect(atS()).toBe(360);
    pageNowMs += 100;
    touchOnScrub("touchstart", 250);
    expect(atS()).toBe(150);
  });
});
