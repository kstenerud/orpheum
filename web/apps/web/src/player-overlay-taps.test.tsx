// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayerOverlay } from "./player-overlay.tsx";
import { doubleTapWaitMs } from "./player-overlay-state.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { playingVideo, press, shownByAMouse } from "./test-overlay.ts";
import { withServices } from "./test-services.tsx";

// The owner, 2026-11-04: "For the tap thing: agreed", and a shown overlay must stay up as paused
// playback always does, however it was hidden before (D415).
describe("a finger's taps on the full-screen overlay's picture (D415)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  /** Draws the overlay over video and shows it, then takes the clock, which mounting needs running. */
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
    shownByAMouse(); // hidden as the player opens (D418 §1)
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval"] });
  }

  function shown(): string | null | undefined {
    return mounted?.container.querySelector(".player-overlay")?.getAttribute("data-shown");
  }

  const left = 100;
  const middle = 500;
  const right = 900;

  /** A finger's tap at x on target (the picture by default), the click the browser sends after. */
  function tap(x: number, target?: Element): void {
    const at = target ?? mounted?.container.querySelector("video") ?? document.body;
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

  function pausedVideo(): HTMLVideoElement {
    const video = playingVideo();
    video.pause();
    return video;
  }

  it("hides the overlay and shows it again, a paused film's staying up after, with no wait in the middle", async () => {
    await drawnOver(pausedVideo());
    expect(shown()).toBe("full");
    tap(middle);
    expect(shown()).toBe("hidden");
    tap(middle);
    expect(shown()).toBe("full");
    wait(60_000); // a paused film's overlay never hides by itself (D407 §6)
    expect(shown()).toBe("full");
  });

  it("hides a playing film's overlay and shows it again, which hides on its time", async () => {
    await drawnOver(playingVideo());
    tap(middle);
    expect(shown()).toBe("hidden");
    tap(middle);
    expect(shown()).toBe("full");
    let pageNowMs = 0;
    vi.spyOn(performance, "now").mockImplementation(() => pageNowMs + 1000);
    pageNowMs = 1_000_000; // well past its time
    wait(10_000);
    expect(shown()).toBe("hidden");
  });

  it("leaves a tap on a control to the control", async () => {
    await drawnOver(pausedVideo());
    const button = mounted?.container.querySelector(".player-overlay button") ?? undefined;
    tap(middle, button);
    expect(shown()).toBe("full");
  });

  it("waits on a side for a second tap, then hides", async () => {
    await drawnOver(pausedVideo());
    tap(left);
    expect(shown()).toBe("full");
    wait(doubleTapWaitMs);
    expect(shown()).toBe("hidden");
  });

  it("seeks 10 s back or forward on a double tap, stacking, with no show or hide for its taps", async () => {
    const video = pausedVideo();
    Object.defineProperty(video, "duration", { get: () => 600 });
    let atS = 100;
    Object.defineProperty(video, "currentTime", {
      get: () => atS,
      set: (to: number) => {
        atS = to;
      },
    });
    await drawnOver(video);
    tap(right);
    tap(right);
    expect(atS).toBe(110);
    tap(right);
    expect(atS).toBe(120);
    wait(doubleTapWaitMs * 4);
    expect(shown()).toBe("full");
    tap(left);
    tap(left);
    expect(atS).toBe(110);
    wait(doubleTapWaitMs * 4);
    expect(shown()).toBe("full");
  });

  it("has no double-tap seek in the middle: two taps there hide and show", async () => {
    const video = pausedVideo();
    let atS = 100;
    Object.defineProperty(video, "currentTime", {
      get: () => atS,
      set: (to: number) => {
        atS = to;
      },
    });
    await drawnOver(video);
    tap(middle);
    tap(middle);
    expect(atS).toBe(100);
    expect(shown()).toBe("full");
  });

  it("leaves a mouse's click as it was: it shows the overlay and never hides it", async () => {
    await drawnOver(pausedVideo());
    press("back"); // a key hides it
    expect(shown()).toBe("hidden");
    const video = mounted?.container.querySelector("video");
    act(() => {
      video?.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1 }));
    });
    expect(shown()).toBe("full");
  });
});
