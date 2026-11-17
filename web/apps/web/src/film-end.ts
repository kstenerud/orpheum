// SPDX-License-Identifier: MPL-2.0

/**
 * What a film's end does where nothing plays after it (the owner, 2026-11-17): the Play page leaves
 * the player for the page above it (up-next.tsx, LeavesAtTheEnd), and a host's player stops the
 * film for their Watch Together group (together-host.tsx), each from the one hearing of the end.
 */

import { useEffect, useRef } from "react";

import { useLatest } from "./latest.ts";

/**
 * Calls atTheEnd, once, as video ends with nothing playing after its film (isNothingNext), or at
 * once where it has ended already as that comes to be known. What a film's end does where nothing
 * plays next (the owner, 2026-11-17): the player left for the page above it, a host's group stopped.
 */
export function useAtTheEndWithNothingNext(
  video: HTMLVideoElement | null,
  isNothingNext: boolean,
  atTheEnd: (() => void) | undefined,
): void {
  const atTheEndRef = useLatest(atTheEnd);
  const hasEndedRef = useRef(false);
  useEffect(() => {
    if (video === null || !isNothingNext) {
      return undefined;
    }
    const ended = (): void => {
      if (!hasEndedRef.current && atTheEndRef.current !== undefined) {
        hasEndedRef.current = true;
        atTheEndRef.current();
      }
    };
    if (video.ended) {
      ended();
      return undefined;
    }
    video.addEventListener("ended", ended);
    return () => {
      video.removeEventListener("ended", ended);
    };
  }, [video, isNothingNext, atTheEndRef]);
}
