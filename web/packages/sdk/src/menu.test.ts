// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { menuFrom } from "./menu.ts";

describe("the menu a browser can play", () => {
  it("always lists the baselines, even when the browser answers nothing", () => {
    const menu = menuFrom({ canPlayType: () => "", isTypeSupported: () => false });
    expect(menu.video).toEqual(["h264-sdr"]);
    expect(menu.audio).toEqual(["aac-stereo", "aac-multi"]);
    expect(menu.containers).toEqual([]);
  });

  it("lists what the browser decodes, efficient video first and the baseline last", () => {
    const decodes = new Set(['video/mp4; codecs="hvc1.1.6.L120.90"', 'audio/mp4; codecs="opus"']);
    const menu = menuFrom({
      canPlayType: (type) => (type === "video/mp4" || type === "video/x-matroska" ? "maybe" : ""),
      isTypeSupported: (type) => decodes.has(type),
    });
    expect(menu.video).toEqual(["hevc-sdr", "h264-sdr"]);
    expect(menu.audio).toEqual(["aac-stereo", "aac-multi", "opus"]);
    expect(menu.containers).toEqual(["mp4", "mkv"]);
  });

  it("takes canPlayType's word where there is no MSE, as on an iPhone", () => {
    const menu = menuFrom({
      canPlayType: (type) =>
        ["video/mp4", "video/mp2t", 'audio/mp4; codecs="ec-3"'].includes(type) ? "probably" : "",
      isTypeSupported: () => false,
    });
    expect(menu.audio).toContain("eac3");
    expect(menu.containers).toEqual(["mp4", "mpegts"]);
  });
});
