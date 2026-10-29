// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { ErrorDetails, ErrorTypes, Events, type ErrorData } from "hls.js";

import { isRecovered } from "./abr-controller.ts";
import { decodingRecoveryWindowMs, recoverFromDecodingErrors } from "./decode-recovery.ts";
import type { PlaybackFailure } from "./failure.ts";
import { failOnFatal } from "./hls.ts";

/** A clock the test moves, as performance.now would read. */
function fakeClock(): { readonly nowMs: () => number; readonly advance: (ms: number) => void } {
  let atMs = 0;
  return {
    nowMs: () => atMs,
    advance: (ms) => {
      atMs += ms;
    },
  };
}

/** A fatal hls.js error of type, its message why, nothing else about what carried it. */
function fatalError(
  type: ErrorTypes,
  why: string,
): Pick<ErrorData, "type" | "details" | "fatal" | "error"> {
  return { type, details: ErrorDetails.BUFFER_APPEND_ERROR, fatal: true, error: new Error(why) };
}

/**
 * A playback's hls.js, the video standing at positionS, recovering once from a fatal decoding
 * error (recoverFromDecodingErrors) and failing as failOnFatal tells after, each heard in the order
 * failOrLoadOn and playHls register them. recoverMediaError's reattaching of the media is made
 * instant here: the real wait for hls.js's MEDIA_ATTACHED is abr.test.ts's own test of
 * loadAgainFrom's sequencing, which this is not re-testing.
 */
function playbackRecoveringFromDecoding(
  positionS: number,
  nowMs: () => number,
): {
  readonly trigger: (data: Pick<ErrorData, "type" | "details" | "fatal" | "error">) => void;
  readonly calls: string[];
  readonly notes: string[];
  readonly failures: [PlaybackFailure, string | undefined][];
} {
  const errorListeners: ((event: unknown, data: unknown) => void)[] = [];
  const attachedListeners: (() => void)[] = [];
  const calls: string[] = [];
  const hls = {
    on: (event: Events, listener: (event: unknown, data: unknown) => void) => {
      if (event === Events.ERROR) {
        errorListeners.push(listener);
      }
    },
    off: (event: Events, listener: (event: unknown, data: unknown) => void) => {
      if (event === Events.ERROR) {
        const index = errorListeners.indexOf(listener);
        if (index >= 0) {
          errorListeners.splice(index, 1);
        }
      }
    },
    once: (event: Events, listener: () => void) => {
      if (event === Events.MEDIA_ATTACHED) {
        attachedListeners.push(listener);
      }
    },
    recoverMediaError: () => {
      calls.push("recoverMediaError");
      for (const attached of attachedListeners.splice(0)) {
        attached();
      }
    },
    startLoad: (at?: number) => calls.push(`startLoad ${String(at)}`),
    stopLoad: () => calls.push("stopLoad"),
    media: { currentTime: positionS },
  } as unknown as Parameters<typeof recoverFromDecodingErrors>[0] &
    Parameters<typeof failOnFatal>[0];
  const notes: string[] = [];
  const failures: [PlaybackFailure, string | undefined][] = [];
  recoverFromDecodingErrors(hls, { note: (note) => notes.push(note), nowMs });
  failOnFatal(hls, (failure, whatFailed) => failures.push([failure, whatFailed]));
  return {
    trigger: (data) => {
      for (const listener of [...errorListeners]) {
        listener(Events.ERROR, data);
      }
    },
    calls,
    notes,
    failures,
  };
}

// The owner, 2026-10-29: follow hls.js's own guidance for a fatal media error, recoverMediaError()
// once before giving up, bounded so a repeat soon after still fails.
describe("a fatal decoding error (hls.js's MEDIA_ERROR)", () => {
  it("is recovered once: hls.js's decoding is built anew from where the video stands, and the playback is not failed", () => {
    const clock = fakeClock();
    const { trigger, calls, notes, failures } = playbackRecoveringFromDecoding(12.5, clock.nowMs);
    const data = fatalError(ErrorTypes.MEDIA_ERROR, "decode failed");
    trigger(data);
    expect(calls).toEqual(["recoverMediaError", "startLoad 12.5"]);
    expect(failures).toEqual([]);
    expect(isRecovered(data)).toBe(true);
    expect(notes.at(-1)).toMatch(
      /^a decoding error is recovered, loading again from 12\.5 s: mediaError .*decode failed/u,
    );
  });

  it("is not recovered again, and fails as today, when a second one comes within the recovery's window", () => {
    const clock = fakeClock();
    const { trigger, calls, notes, failures } = playbackRecoveringFromDecoding(12.5, clock.nowMs);
    trigger(fatalError(ErrorTypes.MEDIA_ERROR, "decode failed"));
    calls.length = 0;
    clock.advance(decodingRecoveryWindowMs - 1);
    trigger(fatalError(ErrorTypes.MEDIA_ERROR, "decode failed again"));
    expect(calls).toEqual([]); // not recovered a second time
    expect(failures).toEqual([
      ["decode", expect.stringContaining("decode failed again") as string],
    ]);
    expect(notes.at(-1)).toMatch(/^a decoding error 29999 ms after the last recovered is not/u);
  });

  it("recovers again once the window has passed since the last recovery", () => {
    const clock = fakeClock();
    const { trigger, calls, failures } = playbackRecoveringFromDecoding(12.5, clock.nowMs);
    trigger(fatalError(ErrorTypes.MEDIA_ERROR, "decode failed"));
    calls.length = 0;
    clock.advance(decodingRecoveryWindowMs);
    trigger(fatalError(ErrorTypes.MEDIA_ERROR, "decode failed again"));
    expect(calls).toEqual(["recoverMediaError", "startLoad 12.5"]);
    expect(failures).toEqual([]);
  });
});

describe("a fatal MUX_ERROR", () => {
  it("is left to fail as today: hls.js's own guide has no recovery for it", () => {
    const clock = fakeClock();
    const { trigger, calls, failures } = playbackRecoveringFromDecoding(12.5, clock.nowMs);
    trigger(fatalError(ErrorTypes.MUX_ERROR, "remux failed"));
    expect(calls).toEqual([]);
    expect(failures).toEqual([["decode", expect.stringContaining("remux failed") as string]]);
  });
});
