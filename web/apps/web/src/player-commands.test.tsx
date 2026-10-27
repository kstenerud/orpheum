// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { BarePlayer } from "./bare-player.tsx";
import {
  answerPlayerCommand,
  playerControlsAttribute,
  seekStepMs,
  usePlayerCommands,
} from "./player-commands.ts";
import { mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A video with its own controls in the page, standing at atS of a film durationS long. */
function videoAt(atS: number, durationS = 600): HTMLVideoElement {
  const video = document.createElement("video");
  video.controls = true;
  Object.defineProperty(video, "duration", { configurable: true, value: durationS });
  video.currentTime = atS;
  document.body.append(video);
  return video;
}

describe("the player's answers to a remote's commands", () => {
  afterEach(() => {
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("plays and pauses by play/pause wherever focus is, and by OK on the video alone", () => {
    const video = videoAt(30);
    const play = vi.spyOn(video, "play").mockResolvedValue();
    const pause = vi.spyOn(video, "pause").mockImplementation(() => undefined);
    Object.defineProperty(video, "paused", { configurable: true, value: true });
    expect(answerPlayerCommand(video, "playPause")).toBe(true);
    expect(play).toHaveBeenCalledOnce();
    const elsewhere = document.createElement("button");
    document.body.append(elsewhere);
    elsewhere.focus();
    expect(answerPlayerCommand(video, "select")).toBe(false);
    video.focus();
    Object.defineProperty(video, "paused", { configurable: true, value: false });
    expect(answerPlayerCommand(video, "select")).toBe(true);
    expect(pause).toHaveBeenCalledOnce();
  });

  it("seeks a step by the seeking buttons, and by Left and Right while the video or nothing has focus", () => {
    const video = videoAt(30);
    expect(answerPlayerCommand(video, "seekForward")).toBe(true);
    expect(video.currentTime).toBe(30 + seekStepMs / 1000);
    expect(answerPlayerCommand(video, "left")).toBe(true);
    expect(video.currentTime).toBe(30);
    const control = document.createElement("button");
    document.body.append(control);
    control.focus();
    expect(answerPlayerCommand(video, "right")).toBe(false);
    expect(video.currentTime).toBe(30);
    expect(answerPlayerCommand(video, "up")).toBe(false);
  });

  it("never seeks before the start nor past the end", () => {
    const video = videoAt(4, 20);
    answerPlayerCommand(video, "seekBack");
    expect(video.currentTime).toBe(0);
    video.currentTime = 15;
    answerPlayerCommand(video, "seekForward");
    expect(video.currentTime).toBe(20);
  });

  it("leaves full screen at Back, and leaves Back to the page where it is not in full screen", () => {
    const video = videoAt(30);
    expect(answerPlayerCommand(video, "back")).toBe(false);
    const exit = vi.fn(() => Promise.resolve());
    Object.defineProperty(document, "fullscreenElement", { configurable: true, value: video });
    Object.defineProperty(document, "exitFullscreen", { configurable: true, value: exit });
    try {
      expect(answerPlayerCommand(video, "back")).toBe(true);
      expect(exit).toHaveBeenCalledOnce();
    } finally {
      Reflect.deleteProperty(document, "fullscreenElement");
      Reflect.deleteProperty(document, "exitFullscreen");
    }
  });
});

describe("the bare player's answers to a remote's commands", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it("seeks its video by Left and Right as the player does", async () => {
    const mounted = await mount(
      withServices(
        <BarePlayer
          url="/no-such-media"
          resumePositionMs={0}
          soundOffsetMs={0}
          onBack={() => undefined}
        />,
      ),
    );
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("the bare player draws no video");
    }
    act(() => {
      video.focus();
    });
    video.currentTime = 30;
    const arrow = new KeyboardEvent("keydown", {
      key: "ArrowRight",
      bubbles: true,
      cancelable: true,
    });
    act(() => {
      video.dispatchEvent(arrow);
    });
    expect(arrow.defaultPrevented).toBe(true);
    expect(video.currentTime).toBe(30 + seekStepMs / 1000);
    mounted.unmount();
  });
});

/** What answers the commands for video, as the Play page and the bare player do. */
function PlayerOf(props: { readonly video: HTMLVideoElement }): null {
  usePlayerCommands(props.video);
  return null;
}

/** Presses Space on what has focus, down and up, and whether each was taken from the browser. */
function pressSpace(isRepeat = false): {
  readonly isDownTaken: boolean;
  readonly isUpTaken: boolean;
} {
  const target = document.activeElement ?? document.body;
  const down = new KeyboardEvent("keydown", {
    key: " ",
    bubbles: true,
    cancelable: true,
    repeat: isRepeat,
  });
  const up = new KeyboardEvent("keyup", { key: " ", bubbles: true, cancelable: true });
  act(() => {
    target.dispatchEvent(down);
    target.dispatchEvent(up);
  });
  return { isDownTaken: down.defaultPrevented, isUpTaken: up.defaultPrevented };
}

/** Gives element focus, as a click or a remote's move would. */
function focusOn(element: HTMLElement): void {
  act(() => {
    // eslint-disable-next-line no-param-reassign -- focusable, as the player's video is made
    element.tabIndex = -1;
    element.focus();
  });
}

describe("Space in the player, as YouTube's (the owner, 2026-10-27)", () => {
  afterEach(() => {
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  // Chrome pressed the browser's full-screen button a click had left focus on, and a cast's
  // receiver, its video with no browser controls, did nothing.
  it("plays and pauses wherever focus is in the player, and nowhere else", async () => {
    const stage = document.createElement("div");
    const video = videoAt(30);
    const controls = document.createElement("div");
    controls.setAttribute(playerControlsAttribute, "");
    const scrub = document.createElement("input");
    scrub.type = "range";
    const upNext = document.createElement("button");
    const outside = document.createElement("button");
    controls.append(scrub);
    stage.append(video, controls, upNext);
    document.body.append(stage, outside);
    const play = vi.spyOn(video, "play").mockResolvedValue();
    Object.defineProperty(video, "paused", { configurable: true, value: true });
    const mounted = await mount(withServices(<PlayerOf video={video} />));
    for (const focused of [video, scrub]) {
      focusOn(focused);
      expect(pressSpace()).toEqual({ isDownTaken: true, isUpTaken: true });
    }
    act(() => {
      scrub.blur();
    });
    expect(pressSpace().isDownTaken).toBe(true);
    expect(play).toHaveBeenCalledTimes(3);
    expect(pressSpace(true).isDownTaken).toBe(true); // held down: once is enough
    expect(play).toHaveBeenCalledTimes(3);
    for (const focused of [upNext, outside]) {
      focusOn(focused);
      expect(pressSpace()).toEqual({ isDownTaken: false, isUpTaken: false });
    }
    expect(play).toHaveBeenCalledTimes(3);
    mounted.unmount();
  });

  // Chrome keeps focus on its own control a click pressed, inside the video, and presses it at
  // Space before the page hears the key: the full-screen button left full screen.
  it("takes focus back to the video after a press on it or its own controls", async () => {
    const video = videoAt(30);
    const outside = document.createElement("button");
    document.body.append(video, outside);
    video.tabIndex = -1;
    const mounted = await mount(withServices(<PlayerOf video={video} />));
    focusOn(outside);
    vi.useFakeTimers();
    act(() => {
      video.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      vi.runAllTimers();
    });
    expect(document.activeElement).toBe(video);
    mounted.unmount();
    vi.useRealTimers();
  });
});
