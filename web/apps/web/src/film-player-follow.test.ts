// @vitest-environment happy-dom
import type { PlayedLevel } from "@orpheum/player";
import { playerSwitchedQualityEvent } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { followChunkedPlayback } from "./film-player.tsx";

afterEach(() => {
  vi.useRealTimers();
});

describe("followChunkedPlayback", () => {
  const low: PlayedLevel = { bitrate: 1_000_000, averageBitrate: 900_000, height: 360 };
  const high: PlayedLevel = { bitrate: 6_000_000, averageBitrate: 5_000_000, height: 1080 };

  /** Follows a playback whose quality playing is whichever playNow last set. */
  function follow(): {
    readonly video: HTMLVideoElement;
    readonly shown: (PlayedLevel | undefined)[];
    readonly playNow: (level: PlayedLevel) => void;
    readonly stop: () => void;
  } {
    const video = document.createElement("video");
    // Loaded, so that the stall watch has no first load to time and sets no timer of its own.
    Object.defineProperty(video, "readyState", { value: HTMLMediaElement.HAVE_ENOUGH_DATA });
    const shown: (PlayedLevel | undefined)[] = [];
    let now: PlayedLevel | undefined = low;
    const stop = followChunkedPlayback(
      video,
      { playing: () => now, beyondTheLink: () => undefined },
      { setPlaying: (playing) => shown.push(playing), setCannotKeepUp: () => undefined },
    );
    return {
      video,
      shown,
      playNow: (level) => {
        now = level;
      },
      stop,
    };
  }

  it("reads the quality playing once as it starts and sets no timer to poll it", () => {
    vi.useFakeTimers();
    const { shown, stop } = follow();
    expect(shown).toEqual([low]);
    expect(vi.getTimerCount()).toBe(0);
    vi.advanceTimersByTime(5000);
    expect(shown).toEqual([low]);
    stop();
  });

  it("shows the quality a switch the player tells of began, and none after it stops", () => {
    const { video, shown, playNow, stop } = follow();
    playNow(high);
    video.dispatchEvent(new CustomEvent(playerSwitchedQualityEvent, { detail: {} }));
    expect(shown).toEqual([low, high]);
    stop();
    playNow(low);
    video.dispatchEvent(new CustomEvent(playerSwitchedQualityEvent, { detail: {} }));
    expect(shown).toEqual([low, high]);
  });
});
