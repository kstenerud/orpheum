// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { hlsWay, playNative, type NativeTarget } from "./hls.ts";

describe("playing a prepared rendition", () => {
  it("uses hls.js wherever there is MSE, the element's own HLS where there is only that", () => {
    expect(hlsWay({ hasMse: true, canPlayNatively: true })).toBe("hls.js");
    expect(hlsWay({ hasMse: false, canPlayNatively: true })).toBe("native");
    expect(hlsWay({ hasMse: false, canPlayNatively: false })).toBe("none");
  });

  it("hands the playlist to the element itself, and lets go of it", () => {
    const removed: string[] = [];
    let source = "";
    let loads = 0;
    const target: NativeTarget = {
      setAttribute: (name, value) => {
        source = name === "src" ? value : source;
      },
      removeAttribute: (name) => removed.push(name),
      load: () => {
        loads++;
      },
    };
    const playback = playNative(target, "/api/v1/bytes/media/hls/t/master.m3u8");
    expect(source).toBe("/api/v1/bytes/media/hls/t/master.m3u8");
    playback.destroy();
    expect(removed).toEqual(["src"]);
    expect(loads).toBe(1);
  });
});
