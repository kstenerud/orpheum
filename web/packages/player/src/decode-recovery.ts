// SPDX-License-Identifier: MPL-2.0

/**
 * A fatal decoding error (hls.js's MEDIA_ERROR) is recovered once before it fails the playback, as
 * hls.js's own guide asks (docs/API.md, "Fatal Error Recovery"): `hls.recoverMediaError()` once the
 * media element errors, gated so a repeat soon after is not retried forever. Its sample code also
 * offers `hls.swapAudioCodec()` first for devices that mishandle HE-AAC and AAC, but v1.7.3's guide
 * (the version in web/node_modules/hls.js) now says that "should no longer be required and is not
 * recommended", so it is left out here. A second fatal decoding error soon after the first gives the
 * quality loading up as one the device cannot play (D266), where a lower one is there to load, and
 * fails the playback at the lowest: a TCL TV's browser (TV Bro) said it played HEVC, and failed to
 * decode the film's own HEVC picture at every rebuild, so each such playback failed while the H.264
 * qualities below it were there (idiosyncrasies.md, "A TCL TV in TV Bro says it plays HEVC and fails
 * to decode it"; the owner's test, staging, 2026-11-06). MUX_ERROR is left to fail as today too: the guide's
 * own sample code has no case for it, only MEDIA_ERROR, so it falls to "cannot recover" there as
 * here.
 */

import Hls, { type ErrorData } from "hls.js";

import { isRecovered, loadAgainFrom, recoverFrom } from "./abr-controller.ts";
import { failureText } from "./diagnosis.ts";
import { resumePositionOf, type ResumingHls } from "./resume-position.ts";

/**
 * How long after recovering from a fatal decoding error the playback must go on before another is
 * recovered rather than failed, in ms. hls.js's own sample code instead gates on wall time alone
 * since the last attempt, 5 s, which would let a decoder stuck in the same fault retry forever at a
 * steady pace; this window is long enough that a rebuilt decoding pipeline has actually loaded and
 * played several chunks cleanly before it is trusted again, so a second fatal decoding error inside
 * it is taken for the same fault surfacing again, not a new one, and failed; short enough that a
 * later, unrelated decoding fault deep in a multi-hour film is not left to fail outright for the
 * rest of the playback. Half a minute of play.
 */
export const decodingRecoveryWindowMs = 30_000;

/** What recovering from a fatal decoding error needs of hls.js: its errors, and loadAgainFrom's. */
export type RecoveringFromDecoding = Pick<
  Hls,
  "on" | "off" | "once" | "recoverMediaError" | "startLoad"
> &
  ResumingHls;

/** The quality a decoding error a rebuild did not mend gives up: the playback's Choosing (abr.ts). */
export interface GivingUpQuality {
  /** Whether the quality loading can be given up, a lower one there to load. */
  readonly canGiveUpLoading: () => boolean;
  /** Gives the quality loading up as unplayable here, loading again from positionS below it. */
  readonly giveUpLoading: (positionS: number) => void;
}

/**
 * Recovers once from a fatal decoding error (hls.js's MEDIA_ERROR, never its MUX_ERROR): hls.js's
 * decoding is built anew from where the playback resumes (resumePositionOf; loadAgainFrom, which also
 * recovers a hole it cannot step over, D266) and the error marked recovered, so failOnFatal, told after, passes it by.
 * One already recovered by another means (isRecovered) is left to it. A second fatal decoding error
 * within decodingRecoveryWindowMs of the last recovery gives the quality loading up (quality, D266),
 * where it can be, the lower quality then having a recovery of its own; otherwise it is not recovered
 * again, and fails as today. Each attempt and its outcome is noted, with what failed and what carried it (failureText, by
 * carrierOf where given), for the core's log. Returns the stopping.
 */
export function recoverFromDecodingErrors(
  hls: RecoveringFromDecoding,
  by: {
    readonly note: (note: string) => void;
    readonly carrierOf?: (url: string) => string;
    /** The quality to give up at a second decoding error (a chunked playback's); none for a prepared copy's. */
    readonly quality?: () => GivingUpQuality | undefined;
    /** The page's clock, ms; performance.now's, as the recovery window is timed by it. */
    readonly nowMs?: () => number;
  },
): () => void {
  const nowMs = by.nowMs ?? (() => performance.now());
  let recoveredAtMs: number | undefined;
  const onError = (_event: unknown, data: ErrorData): void => {
    if (!data.fatal || data.type !== Hls.ErrorTypes.MEDIA_ERROR || isRecovered(data)) {
      return;
    }
    const now = nowMs();
    if (recoveredAtMs !== undefined && now - recoveredAtMs < decodingRecoveryWindowMs) {
      const quality = by.quality?.();
      if (quality?.canGiveUpLoading() === true) {
        recoverFrom(data);
        const sinceMs = now - recoveredAtMs;
        recoveredAtMs = undefined;
        const positionS = resumePositionOf(hls);
        by.note(
          `a decoding error ${String(sinceMs)} ms after the last recovered gives` +
            ` its quality up as unplayable here, loading again below it from` +
            ` ${positionS.toFixed(1)} s: ${failureText(data, by.carrierOf)}`,
        );
        quality.giveUpLoading(positionS);
        return;
      }
      by.note(
        `a decoding error ${String(now - recoveredAtMs)} ms after the last recovered is not` +
          ` recovered again: ${failureText(data, by.carrierOf)}`,
      );
      return; // fails as today: left unrecovered for failOnFatal
    }
    recoverFrom(data);
    recoveredAtMs = now;
    const positionS = resumePositionOf(hls);
    by.note(
      `a decoding error is recovered, loading again from ${positionS.toFixed(1)} s: ` +
        failureText(data, by.carrierOf),
    );
    loadAgainFrom(hls, positionS);
  };
  hls.on(Hls.Events.ERROR, onError);
  return () => {
    hls.off(Hls.Events.ERROR, onError);
  };
}
