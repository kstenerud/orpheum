// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { commandEventName } from "./input-commands.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { clockText, overlayAnswer, TvControls } from "./tv-controls.tsx";

describe("a television's player overlay, as Jellyfin's (the owner, Fire TV, 2026-10-25)", () => {
  it("seeks and shows the scrub bar alone on Left and Right while hidden, taking the press", () => {
    expect(overlayAnswer("none", "left", false)).toEqual({
      shown: "scrub",
      isTaken: true,
      seekByMs: -10_000,
    });
    expect(overlayAnswer("scrub", "right", false)).toEqual({
      shown: "scrub",
      isTaken: true,
      seekByMs: 10_000,
    });
  });

  it("opens all of it on OK, Up or Down while hidden, focus to play, and does nothing else", () => {
    for (const command of ["select", "up", "down"] as const) {
      expect(overlayAnswer("none", command, false)).toEqual({
        shown: "all",
        isTaken: true,
        isFocusToPlay: true,
      });
    }
  });

  it("hides on Back while it shows, and leaves Back to the page while hidden", () => {
    expect(overlayAnswer("all", "back", false)).toEqual({ shown: "none", isTaken: true });
    expect(overlayAnswer("none", "back", false)).toEqual({ shown: "none", isTaken: false });
  });

  it("while all of it shows, seeks by Left and Right on the scrub bar, and leaves other moves to focus", () => {
    expect(overlayAnswer("all", "left", true)).toEqual({
      shown: "all",
      isTaken: true,
      seekByMs: -10_000,
    });
    expect(overlayAnswer("all", "left", false)).toEqual({ shown: "all", isTaken: false });
    expect(overlayAnswer("all", "down", false)).toEqual({ shown: "all", isTaken: false });
  });

  it("leaves play, pause and the seeking keys to the player, showing the scrub bar", () => {
    expect(overlayAnswer("none", "playPause", false)).toEqual({ shown: "scrub", isTaken: false });
    expect(overlayAnswer("all", "seekForward", false)).toEqual({ shown: "all", isTaken: false });
  });

  it("says a film's times as a player does", () => {
    expect(clockText(0)).toBe("0:00");
    expect(clockText(1_466_000)).toBe("24:26");
    expect(clockText(3_725_000)).toBe("1:02:05");
  });
});

describe("the overlay as drawn", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("is play and a scrub bar a remote reaches, with no rewind or fast-forward", async () => {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", document.createElement("video"));
    mounted = await mount(
      withServices(
        <TvControls videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />,
      ),
    );
    expect(mounted.container.querySelector('button[aria-label="Play"]')).not.toBeNull();
    expect(mounted.container.querySelector('input[type="range"]')).not.toBeNull();
    expect(mounted.container.querySelectorAll("button")).toHaveLength(1);
  });

  it("leaves OK to another control over the film, Up next, and its hiding leaves focus there", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const videoRef = createRef<HTMLVideoElement>();
    const video = document.createElement("video");
    Object.defineProperty(video, "paused", { get: () => false }); // playing, so the overlay hides
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <TvControls videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
          <button type="button" className="beside">
            Up next
          </button>
        </div>,
      ),
    );
    const beside = mounted.container.querySelector<HTMLButtonElement>(".beside");
    const overlay = mounted.container.querySelector(".tv-controls");
    act(() => {
      video.dispatchEvent(new Event("play"));
      beside?.focus();
    });
    vi.spyOn(performance, "now").mockReturnValue(1_000_000); // long after the last press
    act(() => {
      vi.advanceTimersByTime(10_000); // well past overlayShownForMs, which hides it
    });
    expect(overlay?.getAttribute("data-shown")).toBe("none");
    expect(document.activeElement).toBe(beside);
    const select = new CustomEvent(commandEventName, {
      detail: { command: "select", source: "key" },
      bubbles: true,
      cancelable: true,
    });
    act(() => {
      beside?.dispatchEvent(select);
    });
    expect(select.defaultPrevented).toBe(false);
    expect(overlay?.getAttribute("data-shown")).toBe("none");
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("opens at a mouse's move, a click or a tap, as a desktop player does, and hides after its time", async () => {
    // A cast's receiver on a computer opened no controls to its mouse (the owner, 2026-10-27).
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const videoRef = createRef<HTMLVideoElement>();
    const video = document.createElement("video");
    Object.defineProperty(video, "paused", { get: () => false }); // playing, so the overlay hides
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <TvControls videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>,
      ),
    );
    const overlay = mounted.container.querySelector(".tv-controls");
    let pageNowMs = 0;
    vi.spyOn(performance, "now").mockImplementation(() => pageNowMs);
    const hideByTime = (): void => {
      pageNowMs += 1_000_000;
      act(() => {
        video.dispatchEvent(new Event("play"));
      });
      act(() => {
        vi.advanceTimersByTime(10_000); // well past overlayShownForMs, which hides it
      });
    };
    const moveTo = (screenX: number, pointerType = "mouse"): void => {
      act(() => {
        document.body.dispatchEvent(
          new PointerEvent("pointermove", { bubbles: true, pointerType, screenX, screenY: 5 }),
        );
      });
    };
    const clickWith = (detail: number): void => {
      act(() => {
        document.body.dispatchEvent(new MouseEvent("click", { bubbles: true, detail }));
      });
    };
    hideByTime();
    expect(overlay?.getAttribute("data-shown")).toBe("none");
    moveTo(10);
    moveTo(10); // the browser's own move, the pointer where it was
    moveTo(30, "touch");
    clickWith(0); // a key's click, which the overlay hears as the key
    expect(overlay?.getAttribute("data-shown")).toBe("none");
    moveTo(11);
    expect(overlay?.getAttribute("data-shown")).toBe("all");
    expect(document.activeElement).not.toBe(mounted.container.querySelector("button"));
    hideByTime();
    expect(overlay?.getAttribute("data-shown")).toBe("none");
    clickWith(1);
    expect(overlay?.getAttribute("data-shown")).toBe("all");
    vi.restoreAllMocks();
    vi.useRealTimers();
  });
});
