// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { BarePlayer } from "./bare-player.tsx";
import { answerPlayerCommand, seekStepMs } from "./player-commands.ts";
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
