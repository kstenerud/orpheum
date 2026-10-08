// SPDX-License-Identifier: MPL-2.0

import { diagnosisOf, watchStalls, type PlaybackDiagnosis } from "@orpheum/player";
import {
  playerBufferedFirstEvent,
  playerMovedItselfEvent,
  type TogetherConnection,
} from "@orpheum/sdk";
import { useEffect, type RefObject } from "react";

/** The page's clock, in ms. */
const pageNowMs = (): number => performance.now();

/** What a note says a player holds whose playback keeps no diagnosis: nothing. */
const notDiagnosed: PlaybackDiagnosis = {
  videoBuffered: "",
  audioBuffered: "",
  lastError: "",
  mapping: "",
  streaming: "",
};

/** The fewest ms between two diagnoses of one player: a stall that repeats is said once a while. */
export const diagnoseAtMostEveryMs = 5000;

/**
 * Tells the server's log what a following player's video holds whenever it moved itself over a gap
 * or stalled: an iPhone that jumped 10 s on its own and then sat waiting for a minute said nothing
 * of what it held, and its page reports no stalls of its own (the owner's test, 2026-10-08). As
 * each of its playlists' first chunk is buffered, it tells it too, however soon after another: how
 * hls.js placed that chunk is what says whether the jump was a wrong placing (the sync review).
 */
export function useDiagnosisNotes(
  videoRef: RefObject<HTMLVideoElement | null>,
  connection: Pick<TogetherConnection, "note">,
  nowMs: () => number = pageNowMs,
): void {
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    let lastMs = Number.NEGATIVE_INFINITY;
    const diagnose = (why: "movedItself" | "stalled" | "bufferedFirst"): void => {
      if (why !== "bufferedFirst" && nowMs() - lastMs < diagnoseAtMostEveryMs) {
        return;
      }
      lastMs = nowMs();
      connection.note({
        kind: "diagnosis",
        why,
        positionMs: Math.round(video.currentTime * 1000),
        readyState: video.readyState,
        ...(diagnosisOf(video) ?? notDiagnosed),
      });
    };
    const onMoved = (): void => {
      diagnose("movedItself");
    };
    const onBufferedFirst = (): void => {
      diagnose("bufferedFirst");
    };
    video.addEventListener(playerMovedItselfEvent, onMoved);
    video.addEventListener(playerBufferedFirstEvent, onBufferedFirst);
    const stopWatching = watchStalls(video, () => {
      diagnose("stalled");
    });
    return () => {
      video.removeEventListener(playerMovedItselfEvent, onMoved);
      video.removeEventListener(playerBufferedFirstEvent, onBufferedFirst);
      stopWatching();
    };
  }, [videoRef, connection, nowMs]);
}
