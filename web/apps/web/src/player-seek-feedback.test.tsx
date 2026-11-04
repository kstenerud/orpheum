// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayerOverlay } from "./player-overlay.tsx";
import { seekFeedbackShownMs } from "./player-taps.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

// The owner, 2026-11-04: "Yes, I like that +10 idea" (D415 \u00a76): a double-tap seek says how far it
// went, on the side tapped, and the total stacks.
describe("a double-tap seek's indicator (D415 \u00a76)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  const left = 100;
  const right = 900;

  /** Draws the overlay over a paused film at 100 s of 600 s and shows it, then takes the clock. */
  async function drawn(): Promise<void> {
    const video = document.createElement("video");
    Object.defineProperty(video, "paused", { get: () => true });
    Object.defineProperty(video, "duration", { get: () => 600 });
    let atS = 100;
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
        <div className="film-player__stage">
          <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>,
      ),
    );
    mounted.container.querySelector(".film-player__stage")?.prepend(video);
    // Hidden as the player opens (D418 §1), the overlay is shown as a mouse moved over it shows it.
    for (const screenX of [5, 50]) {
      act(() => {
        document.body.dispatchEvent(
          new PointerEvent("pointermove", {
            bubbles: true,
            pointerType: "mouse",
            screenX,
            screenY: 5,
          }),
        );
      });
    }
    // The clock of an event's timeStamp is faked too, so a slow machine cannot break a run of taps.
    vi.useFakeTimers({
      toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval", "performance", "Date"],
    });
  }

  function tap(x: number): void {
    const at = mounted?.container.querySelector("video") ?? document.body;
    const init = { bubbles: true, pointerType: "touch", pointerId: 7, clientX: x, clientY: 50 };
    act(() => {
      at.dispatchEvent(new PointerEvent("pointerdown", init));
      at.dispatchEvent(new PointerEvent("pointerup", init));
      at.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1, clientX: x }));
    });
  }

  function wait(ms: number): void {
    act(() => {
      vi.advanceTimersByTime(ms);
    });
  }

  const seeked = (): Element | null =>
    mounted?.container.querySelector(".player-overlay-seeked") ?? null;
  const isShown = (): boolean => seeked()?.hasAttribute("data-shown") === true;

  it("shows the run's total on the side tapped and fades it after the last tap", async () => {
    await drawn();
    tap(right);
    expect(seeked()).toBeNull(); // a lone tap seeks nothing
    wait(100);
    tap(right);
    expect(seeked()?.textContent).toBe("+10");
    expect(seeked()?.getAttribute("data-side")).toBe("right");
    expect(seeked()?.getAttribute("aria-hidden")).toBe("true");
    expect(isShown()).toBe(true);
    wait(100);
    tap(right);
    expect(seeked()?.textContent).toBe("+20");
    wait(100);
    tap(right);
    expect(seeked()?.textContent).toBe("+30");
    wait(seekFeedbackShownMs - 1); // the last tap restarted the time
    expect(isShown()).toBe(true);
    wait(1);
    expect(isShown()).toBe(false);
  });

  it("says \u221210 on the left, and starts again after the fade", async () => {
    await drawn();
    tap(left);
    wait(100);
    tap(left);
    expect(seeked()?.textContent).toBe("\u221210");
    expect(seeked()?.getAttribute("data-side")).toBe("left");
    wait(seekFeedbackShownMs);
    tap(left);
    wait(100);
    tap(left);
    expect(seeked()?.textContent).toBe("\u221210");
    expect(isShown()).toBe(true);
    expect(mounted?.container.querySelector(".player-overlay")?.getAttribute("data-shown")).toBe(
      "full",
    ); // it did not show or hide the overlay
  });
});
