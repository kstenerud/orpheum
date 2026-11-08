// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { isPausedHere, positionPlayedHere, useNotesPlayingHere } from "./playing-here.ts";
import { mount, type Mounted } from "./test-dom.tsx";

/** A Play page playing fileId here, standing as its video says. */
function PlayingHere(props: {
  readonly fileId: number;
  readonly video: { currentMs: number; isPaused: boolean };
}): JSX.Element {
  useNotesPlayingHere(true, props.fileId, {
    positionMs: () => props.video.currentMs,
    isPaused: () => props.video.isPaused,
  });
  return <p>playing</p>;
}

describe("the film played here, for a cast started from it (D364 §1)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("says whether it stands paused now, and as the page goes, whether it was left paused (the owner, 2026-11-08)", async () => {
    const video = { currentMs: 61_000, isPaused: false };
    mounted = await mount(<PlayingHere fileId={4} video={video} />);
    expect(isPausedHere(4)).toBe(false);
    video.isPaused = true;
    expect(isPausedHere(4)).toBe(true);
    expect(isPausedHere(5)).toBe(false);
    mounted.unmount();
    mounted = undefined;
    video.isPaused = false; // the video gone with its page plays no more; it was left paused
    expect(isPausedHere(4)).toBe(true);
    expect(positionPlayedHere(4)).toBe(61_000);
  });
});
