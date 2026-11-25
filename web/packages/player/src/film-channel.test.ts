// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { chooseFilmChannel, hearFilmChannel, type FilmChannel } from "./film-channel.ts";

describe("the film's channel of a receiver's sound (D440 §2, §5)", () => {
  it("is heard now and at each volume change, until stopped", () => {
    const video = document.createElement("video");
    video.volume = 0.5;
    const heard: FilmChannel[] = [];
    const stop = hearFilmChannel(video, (channel) => {
      heard.push(channel);
    });
    expect(heard).toEqual([{ level: 0.5, isMuted: false }]);
    video.muted = true;
    video.dispatchEvent(new Event("volumechange"));
    expect(heard.at(-1)).toEqual({ level: 0.5, isMuted: true });
    stop();
    video.dispatchEvent(new Event("volumechange"));
    expect(heard).toHaveLength(2);
  });

  it("carries a remote's choice out on the video, its level kept within 0 and 1", () => {
    const video = document.createElement("video");
    chooseFilmChannel(video, { level: 0.3, isMuted: true });
    expect(video.volume).toBeCloseTo(0.3);
    expect(video.muted).toBe(true);
    chooseFilmChannel(video, { level: 1.4, isMuted: false });
    expect(video.volume).toBe(1);
    expect(video.muted).toBe(false);
  });
});
