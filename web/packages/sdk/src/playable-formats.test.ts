// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { playableFormatsFrom } from "./playable-formats.ts";

describe("the formats a browser can play", () => {
  it("always lists the baselines, even when the browser answers nothing", () => {
    const formats = playableFormatsFrom({ canPlayType: () => "", isTypeSupported: () => false });
    expect(formats.videoCodecs).toEqual(["h264-sdr"]);
    expect(formats.audioCodecs).toEqual(["aac-stereo", "aac-multi"]);
    expect(formats.containers).toEqual([]);
  });

  it("lists what the browser decodes, efficient video first and the baseline last", () => {
    const decodes = new Set(['video/mp4; codecs="hvc1.1.6.L120.90"', 'audio/mp4; codecs="opus"']);
    const formats = playableFormatsFrom({
      canPlayType: (type) => (type === "video/mp4" || type === "video/x-matroska" ? "maybe" : ""),
      isTypeSupported: (type) => decodes.has(type),
    });
    expect(formats.videoCodecs).toEqual(["hevc-sdr", "h264-sdr"]);
    expect(formats.audioCodecs).toEqual(["aac-stereo", "aac-multi", "opus"]);
    expect(formats.containers).toEqual(["mp4", "mkv"]);
  });

  it("takes canPlayType's word where there is no MSE, as on an iPhone", () => {
    const formats = playableFormatsFrom({
      canPlayType: (type) =>
        ["video/mp4", "video/mp2t", 'audio/mp4; codecs="ec-3"'].includes(type) ? "probably" : "",
      isTypeSupported: () => false,
    });
    expect(formats.audioCodecs).toContain("eac3");
    expect(formats.containers).toEqual(["mp4", "mpegts"]);
  });
});
