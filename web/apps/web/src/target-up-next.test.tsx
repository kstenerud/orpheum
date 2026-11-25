// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import type { UpNext } from "./around-file.ts";
import type { PlaybackTarget, TargetChoices } from "./playback-target.ts";
import { buttonNamed, click, mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { TargetUpNext } from "./up-next.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The first season's third episode, "Bag", which plays file 4. */
const bag: UpNext = {
  episode: {
    itemId: 23,
    name: "Bag",
    groupNumber: 1,
    numberInGroup: 3,
    fileId: 4,
    isPartway: false,
  },
  ordering: "aired",
};

/** A target at atMs of a 20-minute film, its choices choices. */
function targetAt(atMs: number, choices: TargetChoices): PlaybackTarget {
  return {
    atMs,
    reportedAtMs: atMs,
    durationMs: 1_200_000,
    isPaused: false,
    isReady: true,
    playOrPause: () => undefined,
    seekTo: () => undefined,
    kind: choices.kind,
    choices,
  };
}

const castee: TargetChoices = {
  kind: "castee",
  playback: undefined,
  connection: undefined,
  report: undefined,
};

/** The offer's card, by its words. */
function card(on: Mounted): HTMLElement | undefined {
  return [...on.container.querySelectorAll<HTMLElement>("button")].find((each) =>
    each.textContent.includes("Bag"),
  );
}

describe("Up next over the playback target (D555 §8)", () => {
  it("offers a castee's next episode near its end, its press moving on, its ✕ putting it away here", async () => {
    const played: string[] = [];
    mounted = await mount(
      withServices(
        <TargetUpNext
          target={targetAt(1_190_000, castee)}
          upNext={bag}
          creditsStartMs={undefined}
          onPlay={() => played.push("moved on")}
        />,
      ),
    );

    expect(card(mounted)).toBeDefined();
    await click(card(mounted));
    expect(played).toEqual(["moved on"]);
    await click(buttonNamed(mounted.container, "Dismiss"));
    expect(card(mounted)).toBeUndefined();
  });

  it("offers nothing over a castee far from its end, nor over one with its credits yet to come", async () => {
    mounted = await mount(
      withServices(
        <TargetUpNext
          target={targetAt(600_000, castee)}
          upNext={bag}
          creditsStartMs={1_000_000}
          onPlay={() => undefined}
        />,
      ),
    );

    expect(card(mounted)).toBeUndefined();
  });

  it("offers the video's next episode where the video here stands, with no ✕ (Back puts it away)", async () => {
    const video = document.createElement("video");
    Object.defineProperties(video, {
      currentTime: { get: () => 1190 },
      duration: { get: () => 1200 },
    });
    const videoRef = createRef<HTMLVideoElement | null>();
    videoRef.current = video;
    // The target says the film's start: the offer reads the video itself, at its own events.
    mounted = await mount(
      withServices(
        <TargetUpNext
          target={targetAt(0, { kind: "here", videoRef })}
          upNext={bag}
          creditsStartMs={undefined}
          onPlay={() => undefined}
        />,
      ),
    );
    act(() => {
      video.dispatchEvent(new Event("timeupdate"));
    });

    expect(card(mounted)).toBeDefined();
    expect(buttonNamed(mounted.container, "Dismiss")).toBeUndefined();
  });
});
