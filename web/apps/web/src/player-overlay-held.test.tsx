// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { overlayHeldForMs, overlayShieldClass } from "./overlay-shield.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

// The scrub bar's stray seeks on a phone (the owner, 2026-11-15, option a): a second tap meant for
// the picture landed on the bar the first had just shown. Widened to every control of the overlay
// (the owner, 2026-11-15, VQ347), and made a shield in front of them (the owner's "a second overlay
// on front that swallows all taps"). The test DOM has no hit test, so a touch here is sent to what a
// hit lands on, the shield while it is up (player-overlay-held.browser.test.tsx has the hit test).
describe("a finger on the full-screen overlay just shown", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
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

  function overlay(): Element | null | undefined {
    return mounted?.container.querySelector(".player-overlay");
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

  function scrubBar(): HTMLInputElement {
    const scrub = mounted?.container.querySelector<HTMLInputElement>(
      ".player-overlay__scrub input",
    );
    if (scrub === null || scrub === undefined) {
      throw new Error("no scrub bar");
    }
    scrub.getBoundingClientRect = () => new DOMRect(0, 0, 1000, 44);
    return scrub;
  }

  /** The overlay's shield, up just after it shows (OverlayShield), or null. */
  function shield(): HTMLElement | null {
    return (
      mounted?.container.querySelector<HTMLElement>(`.player-overlay .${overlayShieldClass}`) ??
      null
    );
  }

  /** What a touch lands on where target is drawn: the shield while it is up, else target. */
  function hitAt(target: HTMLElement): HTMLElement {
    return shield() ?? target;
  }

  /** A finger coming down where the scrub bar is drawn, at clientX: its pointerdown and touchstart. */
  function touchDownOnScrub(clientX: number): void {
    const hit = hitAt(scrubBar());
    act(() => {
      hit.dispatchEvent(
        new PointerEvent("pointerdown", { bubbles: true, pointerType: "touch", pointerId: 8 }),
      );
    });
    touchOnScrub("touchstart", clientX, hit);
  }

  /**
   * A finger's touch at clientX, of the test DOM's events, which have no Touch, sent to what it came
   * down on (a touch's events go to its first target to its end).
   */
  function touchOnScrub(
    type: "touchstart" | "touchmove",
    clientX: number,
    downOn: HTMLElement,
  ): void {
    const event = new Event(type, { bubbles: true, cancelable: true });
    Object.defineProperty(event, "touches", { value: [{ clientX, clientY: 20 }] });
    act(() => {
      downOn.dispatchEvent(event);
    });
  }

  /** A finger lifting from what it came down on: its pointerup. */
  function liftFrom(downOn: HTMLElement): void {
    act(() => {
      downOn.dispatchEvent(
        new PointerEvent("pointerup", { bubbles: true, pointerType: "touch", pointerId: 8 }),
      );
    });
  }

  /** The overlay's Forward button, which steps the film 10 s on. */
  function forward(): HTMLButtonElement {
    const button = mounted?.container.querySelector<HTMLButtonElement>(
      '.player-overlay button[aria-label="Forward 10 s"]',
    );
    if (button === null || button === undefined) {
      throw new Error("no Forward button");
    }
    return button;
  }

  /** A finger's tap where button is drawn: its pointerdown, pointerup and click, on what it hits. */
  function tapOn(button: HTMLButtonElement): void {
    const hit = hitAt(button);
    const init = { bubbles: true, pointerType: "touch", pointerId: 9 };
    act(() => {
      hit.dispatchEvent(new PointerEvent("pointerdown", init));
      hit.dispatchEvent(new PointerEvent("pointerup", init));
      hit.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, detail: 1 }));
    });
  }

  it("holds a tap, a drag and a button's press for half a second after it shows; after, each works, and a key's click never waits", async () => {
    // The hold's length is the owner's (2026-11-15: "Let's try 0.3s"), down from half a second.
    expect(overlayHeldForMs).toBe(300);
    const { video, atS } = pausedVideo();
    await drawnOver(video);
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    tapThePicture();
    expect(overlay()?.getAttribute("data-shown")).toBe("full");
    expect(shield()?.getAttribute("aria-hidden")).toBe("true");
    tapOn(forward());
    expect(atS()).toBe(100);
    // The shield swallows the tap: the overlay does not take it for one on the picture, and stays.
    expect(overlay()?.getAttribute("data-shown")).toBe("full");
    // A key's click (no count) is a remote's or a keyboard's press, on the focused control, never held.
    act(() => {
      forward().dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 0 }));
    });
    expect(atS()).toBe(110);
    const downOn = hitAt(scrubBar());
    touchDownOnScrub(500); // the half of the film, had it seeked
    touchOnScrub("touchmove", 600, downOn); // a drag that came down in the hold
    expect(atS()).toBe(110);
    act(() => {
      vi.advanceTimersByTime(overlayHeldForMs - 1);
    });
    expect(shield()).not.toBeNull();
    act(() => {
      vi.advanceTimersByTime(1);
    });
    // The drag that came down in the hold keeps the shield, held to its end.
    expect(shield()).not.toBeNull();
    touchOnScrub("touchmove", 700, downOn);
    expect(atS()).toBe(110);
    liftFrom(downOn);
    // The lift's click, which a browser sends at what is under the finger, still meets the shield.
    expect(shield()).not.toBeNull();
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(shield()).toBeNull();
    touchDownOnScrub(250);
    expect(atS()).toBe(150);
    liftFrom(scrubBar());
    tapOn(forward());
    expect(atS()).toBe(160);
    expect(overlay()?.getAttribute("data-shown")).toBe("full");
  });
});
