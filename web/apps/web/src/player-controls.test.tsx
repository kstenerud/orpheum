// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { PlayerSource } from "./playback-target.ts";
import { PlayerControls, useVideoTarget, type SequenceMoves } from "./player-controls.tsx";
import { buttonNamed, click, mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A source standing at atMs of a ten-minute film, paused, its commands noted. */
function noted(atMs: number): { readonly source: PlayerSource; readonly heard: string[] } {
  const heard: string[] = [];
  return {
    heard,
    source: {
      atMs,
      durationMs: 600_000,
      isPaused: true,
      isReady: true,
      playOrPause: () => heard.push("playOrPause"),
      seekTo: (ms) => heard.push(`seekTo ${String(ms)}`),
    },
  };
}

/** The accessible names of the buttons in container, in order. */
function names(container: HTMLElement): (string | null)[] {
  return [...container.querySelectorAll("button")].map((each) => each.getAttribute("aria-label"));
}

describe("the player's bar (D369 §1)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("draws the controls it is given, in the bar's order, the timeline last", async () => {
    const { source } = noted(0);
    mounted = await mount(
      withServices(
        <PlayerControls source={source} controls={["timeline", "fullScreen", "steps", "play"]} />,
      ),
    );
    expect(names(mounted.container)).toEqual(["Play", "Back 10 s", "Forward 10 s", "Full screen"]);
    const bar = mounted.container.querySelector(".player-controls");
    expect(bar?.lastElementChild?.classList.contains("timeline")).toBe(true);
  });

  it("plays and steps its source, never before the start nor past the end", async () => {
    const { source, heard } = noted(5000);
    mounted = await mount(
      withServices(<PlayerControls source={source} controls={["play", "steps"]} />),
    );
    await click(buttonNamed(mounted.container, "Play"));
    await click(buttonNamed(mounted.container, "Back 10 s"));
    await click(buttonNamed(mounted.container, "Forward 10 s"));
    expect(heard).toEqual(["playOrPause", "seekTo 0", "seekTo 15000"]);
  });

  it("holds its controls while its source is not ready, as a remote before it knows where the film is", async () => {
    const { source } = noted(0);
    mounted = await mount(
      withServices(
        <PlayerControls source={{ ...source, isReady: false }} controls={["play", "timeline"]} />,
      ),
    );
    expect(buttonNamed(mounted.container, "Play")?.matches(":disabled")).toBe(true);
    expect(mounted.container.querySelector("input")?.disabled).toBe(true);
  });

  it("mutes and sets the volume of a video on this page, and turns its sound on again", async () => {
    const video = document.createElement("video");
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    function Bar(): React.JSX.Element {
      return <PlayerControls source={useVideoTarget(videoRef)} controls={["play", "sound"]} />;
    }
    mounted = await mount(withServices(<Bar />));
    await click(buttonNamed(mounted.container, "Mute"));
    expect(video.muted).toBe(true);
    act(() => {
      video.dispatchEvent(new Event("volumechange"));
    });
    await click(buttonNamed(mounted.container, "Sound on"));
    expect(video.muted).toBe(false);
    expect(mounted.container.querySelector('input[aria-label="Volume"]')).not.toBeNull();
  });

  it("plays and pauses a video on this page, its button saying which it will do", async () => {
    const video = document.createElement("video");
    const play = vi.spyOn(video, "play").mockResolvedValue();
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    function Bar(): React.JSX.Element {
      return <PlayerControls source={useVideoTarget(videoRef)} controls={["play"]} />;
    }
    mounted = await mount(withServices(<Bar />));
    await click(buttonNamed(mounted.container, "Play"));
    expect(play).toHaveBeenCalledOnce();
    Object.defineProperty(video, "paused", { get: () => false });
    act(() => {
      video.dispatchEvent(new Event("play"));
    });
    expect(names(mounted.container)).toEqual(["Pause"]);
  });

  // "The center buttons should always behave in placement as if all 5 were always available" (the
  // owner, 2026-11-23): in the overlay's row a move there is none of keeps its place.
  it("keeps the overlay row's place of ⏮ or ⏭ where there is none, so the steps and play never shift", async () => {
    const { source } = noted(0);
    /** What the row's leading group holds: a button's label, the centre, or a kept place. */
    const leading = (): (string | null)[] =>
      [...(mounted?.container.querySelector(".player-controls__leading")?.children ?? [])].map(
        (each) => {
          if (each.classList.contains("player-controls__move-place")) {
            return "place";
          }
          return each.matches("button") ? each.getAttribute("aria-label") : "centre";
        },
      );
    const drawn = async (moves: SequenceMoves): Promise<void> => {
      mounted?.unmount();
      mounted = await mount(
        withServices(
          <PlayerControls
            source={source}
            controls={["previous", "steps", "play", "next"]}
            isOverlayRow
            moves={moves}
          />,
        ),
      );
    };
    await drawn({ next: () => undefined });
    expect(leading()).toEqual(["place", "centre", "Next episode"]);
    await drawn({ previous: () => undefined });
    expect(leading()).toEqual(["Previous episode", "centre", "place"]);
    await drawn({});
    expect(leading()).toEqual(["place", "centre", "place"]);
  });
});
