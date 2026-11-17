// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import { useEffect, useRef, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useIsWaitingShown } from "./film-waits.tsx";
import { useLatest } from "./latest.ts";
import { showNotice } from "./notice.ts";

/**
 * How long a film on its ladder waits for data at its lowest quality, in ms, before its person is
 * told it probably will not play on this link (D506 §3).
 */
export const tooSlowAtLowestAfterMs = 10_000;

/** How often, in ms, a film waiting reads whether it loads its lowest quality. */
const lookEveryMs = 500;

/**
 * Tells the person, once in a playback (playbackUrl), that its film probably will not play on this
 * link, the tab's notice with its OK (notice.ts), once the film has waited for data with its spinner
 * shown (useIsWaitingShown) for tooSlowAtLowestAfterMs while it loaded its lowest quality
 * (isAtLowestRung, @orpheum/player; D506 §3). Nothing stops: the player goes on trying until its
 * person leaves. A wait that ends, or a quality above the lowest loading, counts again from nothing.
 * A playback with no qualities of its own to step through, isAtLowestRung undefined (a film playing
 * its own file), or whose browser chooses its qualities itself, never at its lowest, tells none.
 */
export function useTellsTooSlowAtLowest(
  videoRef: RefObject<HTMLVideoElement | null>,
  isAtLowestRung: (() => boolean) | undefined,
  playbackUrl: string | undefined,
): void {
  const { t } = useTranslation();
  const isWaiting = useIsWaitingShown(videoRef);
  const isAtLowestRef = useLatest(isAtLowestRung);
  const hasRungs = isAtLowestRung !== undefined;
  const toldForRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!isWaiting || !hasRungs || toldForRef.current === playbackUrl) {
      return undefined;
    }
    let waitedMs = 0;
    const look = setInterval(() => {
      waitedMs = isAtLowestRef.current?.() === true ? waitedMs + lookEveryMs : 0;
      if (waitedMs >= tooSlowAtLowestAfterMs) {
        clearInterval(look);
        toldForRef.current = playbackUrl;
        showNotice(t("play.tooSlowAtLowest"));
      }
    }, lookEveryMs);
    return () => {
      clearInterval(look);
    };
  }, [isWaiting, hasRungs, playbackUrl, isAtLowestRef, t]);
}
