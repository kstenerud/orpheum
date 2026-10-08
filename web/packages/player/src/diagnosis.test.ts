// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import Hls from "hls.js";

import { diagnose, diagnosisOf, rangesText, type DiagnosedHls } from "./diagnosis.ts";

/** Time ranges from [start, end] pairs, in seconds. */
function ranges(...pairs: [number, number][]): TimeRanges {
  return {
    length: pairs.length,
    start: (index: number) => pairs[index]![0],
    end: (index: number) => pairs[index]![1],
  };
}

describe("what a stalled video held, for its report (D266)", () => {
  it("writes buffered ranges as from-to in seconds", () => {
    expect(rangesText(ranges([296.4, 317.232], [320, 330.5]))).toBe("296.40-317.23 320.00-330.50");
    expect(rangesText(ranges())).toBe("");
  });

  it("keeps each buffer's ranges as hls.js appends, and the last error, the element's over hls.js's", () => {
    const listeners = new Map<string, (event: string, data: never) => void>();
    const hls = {
      on: (event: string, listener: (event: string, data: never) => void) => {
        listeners.set(event, listener);
      },
    } as unknown as DiagnosedHls;
    const video: { error: MediaError | null } = { error: null };
    expect(diagnosisOf(video)).toBeUndefined();
    diagnose(hls, video);
    const appended = listeners.get(Hls.Events.BUFFER_APPENDED) as (
      event: string,
      data: unknown,
    ) => void;
    const failed = listeners.get(Hls.Events.ERROR) as (event: string, data: unknown) => void;
    appended("", { timeRanges: { video: ranges([306.8, 317.06]) } });
    appended("", { timeRanges: { audio: ranges([296.4, 317.23]) } });
    failed("", { details: "bufferStalledError", fatal: false, error: new Error("stalled") });
    expect(diagnosisOf(video)).toEqual({
      videoBuffered: "306.80-317.06",
      audioBuffered: "296.40-317.23",
      lastError: "bufferStalledError: stalled",
    });
    const decodeFailed: Pick<MediaError, "code" | "message"> = {
      code: 3,
      message: "decode failed",
    };
    video.error = decodeFailed as MediaError;
    expect(diagnosisOf(video)?.lastError).toBe("media error 3: decode failed");
  });
});
