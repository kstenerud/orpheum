// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { isWaitingToPlay, startGateOf } from "@orpheum/sdk";
import { useRef, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Stage } from "./film-player.tsx";
import { answerPlayerCommand } from "./player-commands.ts";
import { seekAheadOf, seekAskedEvent, seekRestMs } from "./seek-ahead.ts";
import { mount, type Mounted } from "./test-dom.tsx";

/** The player's stage, its own video in it, or keptVideo where given, as the page keeps one. */
function FilmStage(props: { readonly keptVideo?: HTMLVideoElement }): JSX.Element {
  const videoRef = useRef<HTMLVideoElement>(null);
  return (
    <Stage
      videoRef={videoRef}
      readLinkKbps={() => undefined}
      failure={undefined}
      hasControls={false}
      isAutoPlay={false}
      keptVideo={props.keptVideo}
    />
  );
}

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.useRealTimers();
});

/** The stage's video, 10 min long and standing at 100 s, the seeks asked of it counted in asked. */
function standingVideo(video: HTMLVideoElement, asked: unknown[]): HTMLVideoElement {
  Object.defineProperty(video, "duration", { value: 600 });
  video.currentTime = 100; // eslint-disable-line no-param-reassign -- where the film stands
  video.addEventListener(seekAskedEvent, (event) => {
    asked.push(event);
  });
  return video;
}

describe("a seek chosen on the stage's video, as the stage lets the video go", () => {
  // A seek a press aimed, waiting for the presses to rest, outlived its player and seeked a video
  // no page showed, on a test environment already torn down (check-pr, 2026-11-18).
  it("is never made once the stage is gone", async () => {
    mounted = await mount(<FilmStage />);
    const asked: unknown[] = [];
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("the stage has no video");
    }
    standingVideo(video, asked);
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    answerPlayerCommand(video, "seekForward", { source: "key" });
    mounted.unmount();
    mounted = undefined;
    vi.advanceTimersByTime(seekRestMs * 4);
    expect(asked).toEqual([]);
    expect(video.currentTime).toBe(100);
  });

  it("is forgotten by a video kept for the next film, which shows that film's own place", async () => {
    const asked: unknown[] = [];
    const kept = standingVideo(document.createElement("video"), asked);
    mounted = await mount(<FilmStage keptVideo={kept} />);
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    answerPlayerCommand(kept, "seekForward", { source: "key" });
    expect(seekAheadOf(kept).shownMs(100_000)).toBe(110_000); // chosen, waiting
    mounted.unmount();
    mounted = undefined;
    vi.advanceTimersByTime(seekRestMs * 4);
    expect(asked).toEqual([]);
    expect(seekAheadOf(kept).shownMs(0)).toBe(0); // the next film's own place
  });
});

describe("a play the start gate holds on the stage's video, as the stage lets the video go", () => {
  // A play a session's leader asked, waiting for enough to be loaded, outlived its player: its
  // look-again timer told a video of a page already torn down, failing the whole run (2026-11-27).
  it("is given up, whoever asked it, and never made", async () => {
    mounted = await mount(<FilmStage />);
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("the stage has no video");
    }
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const played = vi.fn<() => void>();
    startGateOf(video).play(played); // as a leader asks, not the page's own start
    expect(isWaitingToPlay(video)).toBe(true);
    mounted.unmount();
    mounted = undefined;
    expect(isWaitingToPlay(video)).toBe(false);
    Object.defineProperty(video, "networkState", { value: HTMLMediaElement.NETWORK_IDLE });
    vi.advanceTimersByTime(1000); // four of the gate's looks
    expect(played).not.toHaveBeenCalled();
  });
});
