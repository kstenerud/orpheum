// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { FullscreenCaptions, type CaptionTimes, type CaptionTrack } from "./fullscreen-captions.ts";

/** A text track whose captions, once loaded, are at 10–12 s and 20–23 s. */
function trackOf(): CaptionTrack & { load: () => void; cues: CaptionTimes[] | null } {
  const track: CaptionTrack & { load: () => void; cues: CaptionTimes[] | null } = {
    mode: "disabled",
    cues: null,
    load: () => {
      track.cues = [
        { startTime: 10, endTime: 12 },
        { startTime: 20, endTime: 23 },
      ];
    },
  };
  return track;
}

describe("captions for the browser's own fullscreen (D254)", () => {
  it("are loaded hidden, shown only while the video is fullscreen", () => {
    const track = trackOf();
    const captions = new FullscreenCaptions(track, 0);
    expect(track.mode).toBe("hidden");
    captions.fullscreenChanged(true);
    expect(track.mode).toBe("showing");
    captions.fullscreenChanged(false);
    expect(track.mode).toBe("hidden");
  });

  it("move by the subtitle offset when loaded, and again by each change of it", () => {
    const track = trackOf();
    const captions = new FullscreenCaptions(track, 1500);
    captions.setOffsetMs(1000); // before they are loaded: nothing to move yet
    track.load();
    captions.loaded();
    expect(track.cues?.[0]).toEqual({ startTime: 11, endTime: 13 });
    captions.setOffsetMs(-500);
    expect(track.cues?.[0]).toEqual({ startTime: 9.5, endTime: 11.5 });
    expect(track.cues?.[1]).toEqual({ startTime: 19.5, endTime: 22.5 });
  });
});
