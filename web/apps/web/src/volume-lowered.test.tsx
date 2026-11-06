// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { ownFileGainAfterVolume } from "@orpheum/player";
import { act, createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { filmVolume, followFilmVolume, setFilmVolume } from "./film-volume.ts";
import { answerPlayerCommand } from "./player-commands.ts";
import { PlayerControls, useVideoSource } from "./player-controls.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  setFilmVolume(1);
});

/** No Web Audio: a device whose Web Audio is late takes none. */
function noWebAudio(): never {
  throw new Error("a late device takes no Web Audio");
}

/**
 * Lowers video's sound by 6.02 dB, half its amplitude, as levelling does on the owner's TCL TV, whose
 * Web Audio keeps 426 ms: by the element's own volume (D436).
 */
function lowerByHalf(video: HTMLVideoElement): void {
  const tvMemory = { recall: () => 426, keep: () => undefined };
  ownFileGainAfterVolume(video, noWebAudio, tvMemory).setDb(20 * Math.log10(0.5));
}

describe("the volume the person set, on a sound levelling lowers by the element's volume (D436)", () => {
  it("stays the tab's film volume, the lowering not taken for the person's choice", () => {
    const video = document.createElement("video");
    setFilmVolume(0.8);
    const stop = followFilmVolume(video);
    lowerByHalf(video);
    video.dispatchEvent(new Event("volumechange"));
    expect(filmVolume()).toBeCloseTo(0.8, 6);
    expect(video.volume).toBeCloseTo(0.4, 6);
    setFilmVolume(1);
    expect(video.volume).toBeCloseTo(0.5, 6);
    stop();
  });

  it("is stepped from by + and −, not from the element's lowered volume", () => {
    const video = document.createElement("video");
    lowerByHalf(video);
    // A step up from the person's 100 stays at 100, the half the lowering leaves of it.
    answerPlayerCommand(video, "volumeUp");
    expect(video.volume).toBeCloseTo(0.5, 6);
    answerPlayerCommand(video, "volumeDown");
    expect(video.volume).toBeLessThan(0.5);
    expect(video.volume).toBeGreaterThan(0.4);
  });

  it("is what the player's bar shows and sets", async () => {
    const video = document.createElement("video");
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    function Bar(): React.JSX.Element {
      return <PlayerControls source={useVideoSource(videoRef)} controls={["sound"]} />;
    }
    mounted = await mount(withServices(<Bar />));
    lowerByHalf(video);
    act(() => {
      video.dispatchEvent(new Event("volumechange"));
    });
    const slider = mounted.container.querySelector<HTMLInputElement>('input[aria-label="Volume"]');
    expect(slider?.value).toBe(slider?.max);
  });
});
