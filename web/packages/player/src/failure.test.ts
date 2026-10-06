// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import Hls from "hls.js";

import {
  hlsErrorFailure,
  mediaErrorFailure,
  playbackFailures,
  type PlaybackFailure,
} from "./failure.ts";
import { playHls } from "./hls.ts";

describe("a playback failure", () => {
  it("is a code, whatever the element or hls.js said", () => {
    expect([1, 2, 3, 4].map((code) => mediaErrorFailure({ code }))).toEqual([
      "aborted",
      "network",
      "decode",
      "unsupported",
    ]);
    expect(mediaErrorFailure(null)).toBe("unknown");
    expect(hlsErrorFailure({ type: Hls.ErrorTypes.NETWORK_ERROR })).toBe("network");
    expect(
      hlsErrorFailure({
        type: Hls.ErrorTypes.NETWORK_ERROR,
        response: { url: "1.m4s", code: 429 },
      }),
    ).toBe("too_busy"); // the core has no room to make it (D250 §6)
    expect(hlsErrorFailure({ type: Hls.ErrorTypes.MEDIA_ERROR })).toBe("decode");
    expect(hlsErrorFailure({ type: Hls.ErrorTypes.OTHER_ERROR })).toBe("unknown");
    for (const failure of [
      mediaErrorFailure({ code: 9 }),
      hlsErrorFailure({ type: Hls.ErrorTypes.KEY_SYSTEM_ERROR }),
    ]) {
      expect(playbackFailures).toContain(failure);
    }
  });

  it("is what playing HLS reports where the page can play none", () => {
    const failures: PlaybackFailure[] = [];
    const video = { canPlayType: () => "" } as unknown as HTMLVideoElement;
    playHls(video, "/master.m3u8", { onFailed: (failure) => failures.push(failure) });
    expect(failures).toEqual(["hls_unsupported"]);
  });
});
