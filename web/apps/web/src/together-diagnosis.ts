// SPDX-License-Identifier: MPL-2.0

import { diagnosisOf, watchStalls } from "@orpheum/player";
import { playerMovedItselfEvent, type TogetherConnection } from "@orpheum/sdk";
import { useEffect, type RefObject } from "react";

/** The page's clock, in ms. */
const pageNowMs = (): number => performance.now();

/** The fewest ms between two diagnoses of one player: a stall that repeats is said once a while. */
export const diagnoseAtMostEveryMs = 5000;

/**
 * Tells the server's log what a following player's video holds whenever it moved itself over a gap
 * or stalled: an iPhone that jumped 10 s on its own and then sat waiting for a minute said nothing
 * of what it held, and its page reports no stalls of its own (the owner's test, 2026-10-08).
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
    const diagnose = (why: "movedItself" | "stalled"): void => {
      if (nowMs() - lastMs < diagnoseAtMostEveryMs) {
        return;
      }
      lastMs = nowMs();
      const held = diagnosisOf(video);
      connection.note({
        kind: "diagnosis",
        why,
        positionMs: Math.round(video.currentTime * 1000),
        readyState: video.readyState,
        videoBuffered: held?.videoBuffered ?? "",
        audioBuffered: held?.audioBuffered ?? "",
        lastError: held?.lastError ?? "",
      });
    };
    const onMoved = (): void => {
      diagnose("movedItself");
    };
    video.addEventListener(playerMovedItselfEvent, onMoved);
    const stopWatching = watchStalls(video, () => {
      diagnose("stalled");
    });
    return () => {
      video.removeEventListener(playerMovedItselfEvent, onMoved);
      stopWatching();
    };
  }, [videoRef, connection, nowMs]);
}
