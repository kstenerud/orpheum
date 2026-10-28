// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { arrowsAreCommandsAttribute } from "./focus-engine.ts";
import { commandEventName } from "./input-commands.ts";
import { PageControls } from "./page-controls.tsx";
import { answerPlayerCommand, playerControlsAttribute, seekStepMs } from "./player-commands.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A video in a stage, playing or paused as isPlaying says, and its ref. */
function stagedVideo(isPlaying: () => boolean): {
  readonly video: HTMLVideoElement;
  readonly videoRef: React.RefObject<HTMLVideoElement | null>;
} {
  const video = document.createElement("video");
  Object.defineProperty(video, "paused", { configurable: true, get: () => !isPlaying() });
  const videoRef = createRef<HTMLVideoElement>();
  Reflect.set(videoRef, "current", video);
  return { video, videoRef };
}

/** A command offered at target, as the input layer offers one; whether something took it. */
function offer(target: Element, command: string): boolean {
  const event = new CustomEvent(commandEventName, {
    detail: { command, source: "key" },
    bubbles: true,
    cancelable: true,
  });
  act(() => {
    target.dispatchEvent(event);
  });
  return event.defaultPrevented;
}

describe("a computer's and a phone's own player controls (D369 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  /** The controls over video in its stage, the stage holding the video as the player's does. */
  async function drawnOver(
    video: HTMLVideoElement,
    videoRef: React.RefObject<HTMLVideoElement | null>,
  ): Promise<HTMLElement> {
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <PageControls videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>,
      ),
    );
    const stage = mounted.container.querySelector<HTMLElement>(".film-player__stage");
    if (stage === null) {
      throw new Error("no stage");
    }
    stage.prepend(video);
    return stage;
  }

  it("is the player's bar over the film: play, full screen, sound and the timeline, a stop for the arrows", async () => {
    const { video, videoRef } = stagedVideo(() => false);
    await drawnOver(video, videoRef);
    const overlay = mounted?.container.querySelector(".page-controls");
    expect(overlay?.hasAttribute(playerControlsAttribute)).toBe(true);
    expect(
      [...(overlay?.querySelectorAll("button") ?? [])].map((each) =>
        each.getAttribute("aria-label"),
      ),
    ).toEqual(["Play", "Full screen", "Mute"]);
    const timeline = overlay?.querySelector('input[aria-label="Where the film stands"]');
    expect(timeline?.hasAttribute(arrowsAreCommandsAttribute)).toBe(true);
    expect(video.tabIndex).toBe(0);
  });

  it("plays or pauses at a mouse's click on the film, and puts it full screen at a double click", async () => {
    const { video, videoRef } = stagedVideo(() => false);
    const stage = await drawnOver(video, videoRef);
    const play = vi.spyOn(video, "play").mockResolvedValue();
    const asked = vi.fn(() => Promise.resolve());
    stage.requestFullscreen = asked;
    act(() => {
      video.dispatchEvent(
        new PointerEvent("click", { bubbles: true, detail: 1, pointerType: "mouse" }),
      );
    });
    expect(play).toHaveBeenCalledOnce();
    act(() => {
      video.dispatchEvent(
        new PointerEvent("click", { bubbles: true, detail: 1, pointerType: "touch" }),
      );
      video.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 0 })); // a key's
    });
    expect(play).toHaveBeenCalledOnce(); // a tap shows the bar alone, and a key's click is the key's
    act(() => {
      video.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, detail: 2 }));
    });
    expect(asked).toHaveBeenCalledOnce();
  });

  it("hides after its time while the film plays, never while paused, and shows again at a mouse's move", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    let isPlaying = false;
    const { video, videoRef } = stagedVideo(() => isPlaying);
    await drawnOver(video, videoRef);
    const overlay = mounted?.container.querySelector(".page-controls");
    let pageNowMs = 1_000_000;
    vi.spyOn(performance, "now").mockImplementation(() => pageNowMs);
    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    expect(overlay?.getAttribute("data-shown")).toBe("all"); // paused
    isPlaying = true;
    act(() => {
      video.dispatchEvent(new Event("play"));
    });
    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    expect(overlay?.getAttribute("data-shown")).toBe("none");
    const moveTo = (screenX: number): void => {
      act(() => {
        document.body.dispatchEvent(
          new PointerEvent("pointermove", { bubbles: true, pointerType: "mouse", screenX }),
        );
      });
    };
    moveTo(10);
    moveTo(20);
    expect(overlay?.getAttribute("data-shown")).toBe("all");
    pageNowMs += 1_000_000;
    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    expect(overlay?.getAttribute("data-shown")).toBe("none");
  });

  it("reads where the film stands at each timeupdate only while it shows or holds focus (the efficiency audit, finding 7)", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const { video, videoRef } = stagedVideo(() => true);
    let atS = 10;
    Object.defineProperty(video, "currentTime", { get: () => atS });
    Object.defineProperty(video, "duration", { get: () => 3600 });
    await drawnOver(video, videoRef);
    const timeline = (): HTMLInputElement | null | undefined =>
      mounted?.container.querySelector<HTMLInputElement>(".timeline__range");
    const timeUpdate = (toS: number): void => {
      atS = toS;
      act(() => {
        video.dispatchEvent(new Event("timeupdate"));
      });
    };
    timeUpdate(20);
    expect(timeline()?.getAttribute("aria-valuetext")).toBe("0:20");
    vi.spyOn(performance, "now").mockReturnValue(1_000_000); // long after anyone was seen
    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    expect(mounted?.container.querySelector(".page-controls")?.getAttribute("data-shown")).toBe(
      "none",
    );
    timeUpdate(50);
    expect(timeline()?.getAttribute("aria-valuetext")).toBe("0:20");
    atS = 70;
    act(() => {
      timeline()?.focus(); // a key's focus in the bar shows it
    });
    expect(timeline()?.getAttribute("aria-valuetext")).toBe("1:10");
    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    timeUpdate(80); // hidden again by time, but focus in it keeps it in view
    expect(timeline()?.getAttribute("aria-valuetext")).toBe("1:20");
  });

  it("takes Down on the film into the bar, to play, and leaves Down elsewhere to the page", async () => {
    const { video, videoRef } = stagedVideo(() => false);
    await drawnOver(video, videoRef);
    const playButton = mounted?.container.querySelector<HTMLButtonElement>(
      'button[aria-label="Play"]',
    );
    act(() => {
      video.focus();
    });
    expect(offer(video, "down")).toBe(true);
    expect(document.activeElement).toBe(playButton);
    expect(offer(playButton ?? document.body, "down")).toBe(false);
  });
});

describe("Left and Right on the bar's timeline (D369 §2)", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it("seek the film a step, as on the film itself, and nowhere else does a range's arrow", () => {
    const stage = document.createElement("div");
    const video = document.createElement("video");
    Object.defineProperty(video, "duration", { configurable: true, value: 600 });
    video.currentTime = 30;
    const controls = document.createElement("div");
    controls.setAttribute(playerControlsAttribute, "");
    const timeline = document.createElement("input");
    timeline.type = "range";
    timeline.setAttribute(arrowsAreCommandsAttribute, "");
    const volume = document.createElement("input");
    volume.type = "range";
    controls.append(timeline, volume);
    stage.append(video, controls);
    document.body.append(stage);
    timeline.focus();
    expect(answerPlayerCommand(video, "right")).toBe(true);
    expect(video.currentTime).toBe(30 + seekStepMs / 1000);
    volume.focus();
    expect(answerPlayerCommand(video, "left")).toBe(false);
    expect(video.currentTime).toBe(30 + seekStepMs / 1000);
  });
});
