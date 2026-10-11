// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import { filmVolume, followFilmVolume, setFilmVolume } from "./film-volume.ts";

afterEach(() => {
  setFilmVolume(1);
});

describe("the film's volume in a tab (D281 §5)", () => {
  it("is followed by the player, and takes a change made at the player's own controls", () => {
    const video = document.createElement("video");
    setFilmVolume(0.5);
    const stop = followFilmVolume(video);
    expect(video.volume).toBe(0.5);
    setFilmVolume(0.2);
    expect(video.volume).toBe(0.2);
    video.volume = 0.8;
    video.dispatchEvent(new Event("volumechange"));
    expect(filmVolume()).toBe(0.8);
    stop();
    setFilmVolume(0.1);
    expect(video.volume).toBe(0.8);
  });

  it("is kept between silent and full", () => {
    setFilmVolume(3);
    expect(filmVolume()).toBe(1);
    setFilmVolume(-1);
    expect(filmVolume()).toBe(0);
  });
});
