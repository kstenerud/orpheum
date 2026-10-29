// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { rebuilt, rebuildingFrom, resumePositionOf } from "./resume-position.ts";

/** hls.js as resuming reads it: its video at currentTime, and its start as given. */
function hlsAt(
  currentTime: number,
  start: { readonly hasEnoughToStart: boolean; readonly startPosition: number },
): Parameters<typeof resumePositionOf>[0] {
  return { ...start, media: { currentTime } };
}

describe("where a playback resumes from", () => {
  it("is where its video stands once hls.js has loaded enough to start", () => {
    expect(resumePositionOf(hlsAt(42, { hasEnoughToStart: true, startPosition: 10 }))).toBe(42);
  });

  it("is where hls.js was asked to start until it has loaded enough to start there, the video at 0 meanwhile", () => {
    expect(resumePositionOf(hlsAt(0, { hasEnoughToStart: false, startPosition: 67.9 }))).toBe(67.9);
    expect(resumePositionOf(hlsAt(0, { hasEnoughToStart: false, startPosition: -1 }))).toBe(0);
  });

  it("is where its decoding was rebuilt from until the new media source is attached, whatever hls.js holds before", () => {
    const hls = hlsAt(0, { hasEnoughToStart: false, startPosition: 3 });
    rebuildingFrom(hls, 67.9);
    expect(resumePositionOf(hls)).toBe(67.9);
    rebuilt(hls);
    expect(resumePositionOf(hls)).toBe(3);
  });
});
