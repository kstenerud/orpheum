// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { commandEventName } from "./input-commands.ts";
import { seekAskedEvent, seekRestMs, soughtByOf } from "./seek-ahead.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { clockText } from "./timeline.tsx";
import { overlayAnswer, TvControls } from "./tv-controls.tsx";

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

  it("is play, full screen and a scrub bar a remote reaches, with no rewind or fast-forward", async () => {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", document.createElement("video"));
    mounted = await mount(
      withServices(
        <TvControls videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />,
      ),
    );
    expect(mounted.container.querySelector('button[aria-label="Play"]')).not.toBeNull();
    expect(mounted.container.querySelector('input[type="range"]')).not.toBeNull();
    const buttons = [...mounted.container.querySelectorAll("button")];
    expect(buttons.map((button) => button.getAttribute("aria-label"))).toEqual([
      "Play",
      "Full screen",
    ]);
  });

  it("says a seek it makes for Left or Right was asked by that key, not by the seeking command it answers with", async () => {
    // A Fire TV's receiver stepped back 10 s no one meant, and the log could not say what asked it
    // (staging, 2026-10-29, file 5680): Right and the remote's fast-forward must read apart.
    const videoRef = createRef<HTMLVideoElement>();
    const video = document.createElement("video");
    Object.defineProperty(video, "duration", { value: 600 });
    video.currentTime = 100;
    Reflect.set(videoRef, "current", video);
    const asked: (string | undefined)[] = [];
    video.addEventListener(seekAskedEvent, (event) => {
      asked.push(soughtByOf(event));
    });
    mounted = await mount(
      withServices(
        <TvControls videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />,
      ),
    );
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const press = (command: string): void => {
      act(() => {
        document.body.dispatchEvent(
          new CustomEvent(commandEventName, {
            detail: { command, source: "key" },
            bubbles: true,
            cancelable: true,
          }),
        );
      });
    };
    press("back"); // hidden, where Left seeks
    press("left");
    act(() => {
      window.dispatchEvent(new KeyboardEvent("keyup", { key: "ArrowLeft" }));
      vi.advanceTimersByTime(seekRestMs);
    });
    vi.useRealTimers();
    expect(asked).toEqual(["left-key"]);
    expect(video.currentTime).toBe(90);
  });

  it("puts the stage full screen from its full-screen button's press, and leaves from it in full screen", async () => {
    // A Fire TV whose player opened out of full screen offered no way into it (the owner, 2026-10-27).
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", document.createElement("video"));
    let fullscreenElement: Element | null = null;
    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      get: () => fullscreenElement,
    });
    const left = vi.fn(() => Promise.resolve());
    Object.defineProperty(document, "exitFullscreen", { configurable: true, value: left });
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <TvControls videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>,
      ),
    );
    const stage = mounted.container.querySelector<HTMLElement>(".film-player__stage");
    const asked = vi.fn(() => {
      fullscreenElement = stage;
      document.dispatchEvent(new Event("fullscreenchange"));
      return Promise.resolve();
    });
    if (stage !== null) {
      stage.requestFullscreen = asked;
    }
    const button = mounted.container.querySelector<HTMLButtonElement>(".full-screen-button");
    act(() => {
      button?.click();
    });
    expect(asked).toHaveBeenCalledOnce();
    expect(button?.getAttribute("aria-label")).toBe("Leave full screen");
    act(() => {
      button?.click();
    });
    expect(left).toHaveBeenCalledOnce();
    Reflect.deleteProperty(document, "fullscreenElement");
    Reflect.deleteProperty(document, "exitFullscreen");
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

  // VQ69 on the Fire TV, 2026-11-02: Right on Still watching's Continue sought the hidden film
  // instead of going to Stop, and OK then played the next episode.
  it("leaves Left and Right to another control over the film, Still watching, not seeking the film", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const videoRef = createRef<HTMLVideoElement>();
    const video = document.createElement("video");
    Object.defineProperty(video, "paused", { get: () => false }); // playing, so the overlay hides
    let atS = 1290;
    Object.defineProperty(video, "currentTime", {
      get: () => atS,
      set: (to: number) => {
        atS = to;
      },
    });
    Object.defineProperty(video, "duration", { get: () => 1300 });
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <TvControls videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
          <button type="button" className="beside">
            Continue
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
    const presses = (["right", "left"] as const).map(
      (command) =>
        new CustomEvent(commandEventName, {
          detail: { command, source: "key" },
          bubbles: true,
          cancelable: true,
        }),
    );
    for (const press of presses) {
      act(() => {
        beside?.dispatchEvent(press);
      });
    }
    expect(presses.map((press) => press.defaultPrevented)).toEqual([false, false]);
    expect(atS).toBe(1290);
    expect(overlay?.getAttribute("data-shown")).toBe("none");
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("reads where the film stands at each timeupdate only while it shows, at once as it opens, with one clock formatter (the efficiency audit, finding 7)", async () => {
    const formatted = vi.spyOn(Date.prototype, "toLocaleTimeString");
    const videoRef = createRef<HTMLVideoElement>();
    const video = document.createElement("video");
    let atS = 10;
    Object.defineProperty(video, "currentTime", { get: () => atS });
    Object.defineProperty(video, "duration", { get: () => 3600 });
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <TvControls videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>,
      ),
    );
    const scrub = (): string | null | undefined =>
      mounted?.container.querySelector('input[type="range"]')?.getAttribute("aria-valuetext");
    const command = (name: string): void => {
      act(() => {
        document.body.dispatchEvent(
          new CustomEvent(commandEventName, {
            detail: { command: name, source: "key" },
            bubbles: true,
            cancelable: true,
          }),
        );
      });
    };
    const timeUpdate = (toS: number): void => {
      atS = toS;
      act(() => {
        video.dispatchEvent(new Event("timeupdate"));
      });
    };
    timeUpdate(20);
    expect(scrub()).toBe("0:20");
    expect(mounted.container.textContent).toContain("Ends at");
    command("back"); // hides it
    expect(mounted.container.querySelector(".tv-controls")?.getAttribute("data-shown")).toBe(
      "none",
    );
    timeUpdate(50);
    expect(scrub()).toBe("0:20");
    atS = 70;
    command("up"); // opens it, where the film stands now
    expect(scrub()).toBe("1:10");
    expect(formatted).not.toHaveBeenCalled();
    vi.restoreAllMocks();
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
