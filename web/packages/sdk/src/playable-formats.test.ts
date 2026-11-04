// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { playableFormatsFrom } from "./playable-formats.ts";

const decodesHevc = (type: string): boolean => type.includes("hvc1");
const macFirefox = (version: number): string =>
  `Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:${String(version)}.0) Gecko/20100101 Firefox/${String(version)}.0`;

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

  it("offers no HEVC to Firefox on a Mac up to the last release seen to start it only at an IDR", () => {
    const formats = playableFormatsFrom({
      canPlayType: () => "",
      isTypeSupported: decodesHevc,
      userAgent: macFirefox(157),
    });
    expect(formats.videoCodecs).toEqual(["h264-sdr"]);
  });

  it("offers HEVC to a later Firefox on a Mac, to Firefox elsewhere and to Safari", () => {
    for (const userAgent of [
      macFirefox(158),
      "Mozilla/5.0 (X11; Linux x86_64; rv:157.0) Gecko/20100101 Firefox/157.0",
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15",
    ]) {
      const formats = playableFormatsFrom({
        canPlayType: () => "",
        isTypeSupported: decodesHevc,
        userAgent,
      });
      expect(formats.videoCodecs, userAgent).toEqual(["hevc-hdr10", "hevc-sdr", "h264-sdr"]);
    }
  });
});
