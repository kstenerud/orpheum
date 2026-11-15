// SPDX-License-Identifier: MPL-2.0

/**
 * The full-screen overlay's hold as a real browser hits it (VQ347): for 0.3 s after the
 * overlay shows, a touch, a pen or a mouse lands on none of its controls but on its shield in front
 * of them, in Chromium and Firefox; a touch that came down on the shield keeps it to its lift. The
 * hit is what holds: no listener's cancelling could hold a native range's slider (idiosyncrasies.md).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";

import { overlayHeldForMs, overlayShieldClass } from "./overlay-shield.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
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

  /** The overlay over a paused film of 600 s at 100 s, in the page; where a seek has it since (atS). */
  async function drawnOverAPausedFilm(): Promise<{ readonly atS: () => number }> {
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
    return { atS: () => atS };
  }

  /** A finger's tap on the picture, which shows the overlay. */
  function tapThePicture(): void {
    const video = mounted?.container.querySelector("video") ?? document.body;
    const init = { bubbles: true, pointerType: "touch", pointerId: 7, clientX: 320, clientY: 40 };
    act(() => {
      video.dispatchEvent(new PointerEvent("pointerdown", init));
      video.dispatchEvent(new PointerEvent("pointerup", init));
      video.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1, clientX: 320 }));
    });
  }

  /** What a touch at element's middle lands on. */
  function hitAtItsMiddle(element: HTMLElement): Element | null {
    const box = element.getBoundingClientRect();
    return document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
  }

  it("lets nothing hit its controls for half a second, and a click then does nothing; after, both work", async () => {
    const { atS } = await drawnOverAPausedFilm();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    tapThePicture();
    expect(control("").getAttribute("data-shown")).toBe("full");
    const scrub = control(".player-overlay__scrub input");
    const forward = control('button[aria-label="Forward 10 s"]');
    const play = control('button[aria-label="Play"]');
    expect(isHitAtItsMiddle(scrub)).toBe(false);
    expect(isHitAtItsMiddle(forward)).toBe(false);
    expect(isHitAtItsMiddle(play)).toBe(false);
    expect(hitAtItsMiddle(forward)?.classList.contains(overlayShieldClass)).toBe(true);
    await userEvent.click(forward, { force: true });
    expect(atS()).toBe(100);
    // The shield swallowed the click: the overlay did not take it for a tap on the picture.
    expect(control("").getAttribute("data-shown")).toBe("full");
    act(() => {
      vi.advanceTimersByTime(overlayHeldForMs);
    });
    // The click's own wait after its lift (the click came, so none is left).
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(isHitAtItsMiddle(scrub)).toBe(true);
    expect(isHitAtItsMiddle(forward)).toBe(true);
    expect(isHitAtItsMiddle(play)).toBe(true);
    await userEvent.click(forward);
    expect(atS()).toBe(110);
  });

  it("keeps a touch that came down in the hold off every control to its lift, past the half second", async () => {
    const { atS } = await drawnOverAPausedFilm();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    tapThePicture();
    const scrub = control(".player-overlay__scrub input");
    const forward = control('button[aria-label="Forward 10 s"]');
    // A finger comes down where the scrub bar is drawn, in the hold, on what is there.
    const downOn = hitAtItsMiddle(scrub);
    expect(downOn?.classList.contains(overlayShieldClass)).toBe(true);
    const init = { bubbles: true, pointerType: "touch", pointerId: 8 };
    act(() => {
      downOn?.dispatchEvent(new PointerEvent("pointerdown", init));
    });
    act(() => {
      vi.advanceTimersByTime(overlayHeldForMs + 1000);
    });
    // Still down past the hold: a slider under it would take the rest of its drag.
    expect(isHitAtItsMiddle(scrub)).toBe(false);
    expect(isHitAtItsMiddle(forward)).toBe(false);
    act(() => {
      downOn?.dispatchEvent(new PointerEvent("pointerup", init));
    });
    // Lifted: its click comes at what is under the finger, the shield still.
    expect(isHitAtItsMiddle(scrub)).toBe(false);
    act(() => {
      downOn?.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1 }));
    });
    expect(isHitAtItsMiddle(scrub)).toBe(true);
    expect(isHitAtItsMiddle(forward)).toBe(true);
    expect(atS()).toBe(100);
    expect(control("").getAttribute("data-shown")).toBe("full");
  });
});
