// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayerOverlay } from "./player-overlay.tsx";
import { fullShownForMs } from "./player-overlay-state.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

// The owner, 2026-11-15, on a phone: "When I hold and slowly swipe the scrubber bar, the overlay
// disappears from under me after a short delay. The hide timer should not be running while I'm
// holding the scrubber bar". A drag noted no press, so the overlay hid its time after it showed.
describe("the full-screen overlay while its scrub bar is held", () => {
  let mounted: Mounted | undefined;
  let pageNowMs = 0;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  /** A video that says it plays, and setPaused, which says it paused or plays as the element does. */
  function playingVideo(): {
    readonly video: HTMLVideoElement;
    readonly setPaused: (isPaused: boolean) => void;
  } {
    const video = document.createElement("video");
    let isPaused = false;
    Object.defineProperty(video, "paused", { get: () => isPaused });
    return {
      video,
      setPaused: (isNowPaused) => {
        isPaused = isNowPaused;
        act(() => {
          video.dispatchEvent(new Event(isNowPaused ? "pause" : "play"));
        });
      },
    };
  }

  /** Draws the overlay over video and shows it all, as a mouse's move does, at pageNowMs 0. */
  async function shownOver(video: HTMLVideoElement): Promise<void> {
    pageNowMs = 0;
    vi.spyOn(performance, "now").mockImplementation(() => pageNowMs);
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
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    for (const screenX of [5, 50]) {
      const move = { bubbles: true, pointerType: "mouse", screenX, screenY: 5 };
      act(() => {
        document.body.dispatchEvent(new PointerEvent("pointermove", move));
      });
    }
  }

  function shown(): string | null | undefined {
    return mounted?.container.querySelector(".player-overlay")?.getAttribute("data-shown");
  }

  /** Lets the page's clock reach atMs, the overlay looking at its time on the way. */
  function at(atMs: number): void {
    pageNowMs = atMs;
    act(() => {
      vi.advanceTimersByTime(1000);
    });
  }

  function scrub(): HTMLInputElement {
    const bar = mounted?.container.querySelector<HTMLInputElement>(".player-overlay input");
    if (bar === null || bar === undefined) {
      throw new Error("no scrub bar");
    }
    return bar;
  }

  /** A finger's pointer event of type on target, bubbling to the window, as the browser sends it. */
  function finger(type: string, target: EventTarget): void {
    const init = { bubbles: true, pointerType: "touch", pointerId: 3, clientX: 40, clientY: 5 };
    act(() => {
      target.dispatchEvent(new PointerEvent(type, init));
    });
  }

  it("stays while a finger is held on the scrub bar, and hides its time after the lift", async () => {
    await shownOver(playingVideo().video);
    expect(shown()).toBe("full");
    finger("pointerdown", scrub());
    at(4 * fullShownForMs); // a slow drag, long past its time
    expect(shown()).toBe("full");
    finger("pointerup", scrub()); // the lift counts as a press
    at(4 * fullShownForMs + fullShownForMs - 100);
    expect(shown()).toBe("full");
    at(4 * fullShownForMs + fullShownForMs);
    expect(shown()).toBe("hidden");
  });

  it("lets go at a pointercancel, the browser taking the gesture", async () => {
    await shownOver(playingVideo().video);
    finger("pointerdown", scrub());
    at(10_000);
    finger("pointercancel", document.body);
    at(10_000 + fullShownForMs);
    expect(shown()).toBe("hidden");
  });

  it("lets go once no finger is left on the screen, whatever pointer event is lost", async () => {
    await shownOver(playingVideo().video);
    finger("pointerdown", scrub());
    at(10_000);
    act(() => {
      scrub().dispatchEvent(new TouchEvent("touchend", { bubbles: true, touches: [] }));
    });
    at(10_000 + fullShownForMs);
    expect(shown()).toBe("hidden");
  });

  it("counts a seek landing while the start gate holds the film paused, so it stays its time after", async () => {
    const { video, setPaused } = playingVideo();
    await shownOver(video);
    setPaused(true); // the start gate's pause at a seek to where too little is loaded
    at(10_000);
    act(() => {
      video.dispatchEvent(new Event("seeked"));
    });
    setPaused(false); // played on once enough is loaded
    at(10_000 + fullShownForMs - 100);
    expect(shown()).toBe("full");
    at(10_000 + fullShownForMs);
    expect(shown()).toBe("hidden");
  });
});
