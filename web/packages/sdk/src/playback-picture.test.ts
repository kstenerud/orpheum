// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { wirePicture, wirePictureChoice } from "./playback-picture.ts";

// A cast's remote moves the receiver's subtitles as the player's own timing does, and tells it a
// subtitle was fetched (the plan's "The cast remote's harder parts", items 2 and 3).
describe("a picture and its choices on the wire", () => {
  it("sends the subtitles' timing, omitting in step as the server does", () => {
    expect(wirePictureChoice({ kind: "subtitleOffset", ms: -1500 })).toEqual({
      kind: "subtitleOffset",
      ms: -1500,
    });
    expect(wirePictureChoice({ kind: "subtitleOffset", ms: 0 })).toEqual({
      kind: "subtitleOffset",
    });
  });

  it("sends a fetched subtitle's word bare, as the original's", () => {
    expect(wirePictureChoice({ kind: "subtitleFetched" })).toEqual({ kind: "subtitleFetched" });
    expect(wirePictureChoice({ kind: "original" })).toEqual({ kind: "original" });
    expect(wirePictureChoice({ kind: "subtitles", key: undefined })).toEqual({ kind: "subtitles" });
    expect(wirePictureChoice({ kind: "subtitles", key: "s3" })).toEqual({
      kind: "subtitles",
      key: "s3",
    });
  });

  it("tells the subtitles' timing with the picture, none while in step", () => {
    const subtitles = [{ key: "s3", label: "English" }];
    expect(wirePicture({ subtitles, subtitleOffsetMs: 2500, canPlayOriginal: false })).toEqual({
      subtitles,
      subtitleOffsetMs: 2500,
      canPlayOriginal: false,
    });
    expect(wirePicture({ subtitles, subtitleOffsetMs: 0, canPlayOriginal: false })).toEqual({
      subtitles,
      canPlayOriginal: false,
    });
  });
});
