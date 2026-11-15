// SPDX-License-Identifier: MPL-2.0

/**
 * The full-screen overlay's hold as a real browser hits it (VQ347): for half a second after the
 * overlay shows, a touch, a pen or a mouse lands on none of its controls, in Chromium and Firefox.
 * The hit is what held the scrub bar in browser full screen, where Chromium's touchstart cannot be
 * cancelled and the bar's own slider seeked at a touch whatever its listener did (idiosyncrasies.md).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";

import { PlayerOverlay } from "./player-overlay.tsx";
import { overlayHeldForMs } from "./player-taps.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

describe("the full-screen overlay just shown", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  /** The control selector names in the overlay; it fails the test when there is none. */
  function control(selector: string): HTMLElement {
    const element = mounted?.container.querySelector<HTMLElement>(`.player-overlay ${selector}`);
    if (element === null || element === undefined) {
      throw new Error(`no ${selector} in the overlay`);
    }
    return element;
  }

  /** Whether a touch at element's middle lands on it, or on what it holds. */
  function isHitAtItsMiddle(element: HTMLElement): boolean {
    const box = element.getBoundingClientRect();
    const hit = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
    return hit !== null && element.contains(hit);
  }

  it("lets nothing hit its controls for half a second, and a click then does nothing; after, both work", async () => {
    const video = document.createElement("video");
    video.style.cssText = "display: block; width: 640px; height: 360px";
    let atS = 100;
    Object.defineProperty(video, "paused", { get: () => true });
    Object.defineProperty(video, "duration", { get: () => 600 });
    Object.defineProperty(video, "currentTime", {
      get: () => atS,
      set: (to: number) => {
        atS = to;
      },
    });
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <div className="film-player__stage" style={{ position: "relative", width: "640px" }}>
          <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>,
      ),
    );
    mounted.container.querySelector(".film-player__stage")?.prepend(video);
    act(() => {
      video.dispatchEvent(new Event("durationchange"));
    });
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    // A finger's tap on the picture shows the overlay.
    const init = { bubbles: true, pointerType: "touch", pointerId: 7, clientX: 320, clientY: 40 };
    act(() => {
      video.dispatchEvent(new PointerEvent("pointerdown", init));
      video.dispatchEvent(new PointerEvent("pointerup", init));
      video.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1, clientX: 320 }));
    });
    expect(control("").getAttribute("data-shown")).toBe("full");
    const scrub = control(".player-overlay__scrub input");
    const forward = control('button[aria-label="Forward 10 s"]');
    const play = control('button[aria-label="Play"]');
    expect(isHitAtItsMiddle(scrub)).toBe(false);
    expect(isHitAtItsMiddle(forward)).toBe(false);
    expect(isHitAtItsMiddle(play)).toBe(false);
    await userEvent.click(forward, { force: true });
    expect(atS).toBe(100);
    act(() => {
      vi.advanceTimersByTime(overlayHeldForMs);
    });
    expect(isHitAtItsMiddle(scrub)).toBe(true);
    expect(isHitAtItsMiddle(forward)).toBe(true);
    expect(isHitAtItsMiddle(play)).toBe(true);
    await userEvent.click(forward);
    expect(atS).toBe(110);
  });
});
