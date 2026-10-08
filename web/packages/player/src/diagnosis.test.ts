// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import Hls from "hls.js";

import { playerBufferedFirstEvent } from "@orpheum/sdk";

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
    const video = Object.assign(new EventTarget(), { error: null as MediaError | null });
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
      mapping: "",
      mediaSourceState: "",
    });
    const decodeFailed: Pick<MediaError, "code" | "message"> = {
      code: 3,
      message: "decode failed",
    };
    video.error = decodeFailed as MediaError;
    expect(diagnosisOf(video)?.lastError).toBe("media error 3: decode failed");
  });

  it("keeps how hls.js placed each playlist's first chunk, telling the video, and whether it may load", () => {
    const listeners = new Map<string, (event: string, data: unknown) => void>();
    const hls = {
      on: (event: string, listener: (event: string, data: unknown) => void) => {
        listeners.set(event, listener);
      },
    } as unknown as DiagnosedHls;
    const video = Object.assign(new EventTarget(), { error: null });
    const told: unknown[] = [];
    video.addEventListener(playerBufferedFirstEvent, (event) => {
      told.push((event as CustomEvent).detail);
    });
    diagnose(hls, video);
    const mediaSource = new EventTarget();
    listeners.get(Hls.Events.MEDIA_ATTACHED)!("", { mediaSource });
    listeners.get(Hls.Events.INIT_PTS_FOUND)!("", {
      id: "main",
      initPTS: 900_000,
      timescale: 90_000,
    });
    // Found again at another quality's init: not kept again.
    listeners.get(Hls.Events.INIT_PTS_FOUND)!("", {
      id: "main",
      initPTS: 990_000,
      timescale: 90_000,
    });
    const buffered = listeners.get(Hls.Events.FRAG_BUFFERED)!;
    buffered("", { id: "main", frag: { sn: 30, start: 300, startPTS: 300.0417 } });
    buffered("", { id: "main", frag: { sn: 31, start: 310, startPTS: 310 } }); // not the first
    buffered("", { id: "audio", frag: { sn: 30, start: 300 } });
    mediaSource.dispatchEvent(new Event("startstreaming"));
    expect(told).toEqual(["main", "audio"]);
    expect(diagnosisOf(video)).toMatchObject({
      mapping: "initPTS main 10.000; main sn 30 at 300.00 pts 300.04; audio sn 30 at 300.00 pts ?",
      mediaSourceState: "streaming",
    });
    mediaSource.dispatchEvent(new Event("endstreaming"));
    expect(diagnosisOf(video)?.mediaSourceState).toBe("stopped");
  });
});
