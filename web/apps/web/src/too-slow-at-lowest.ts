// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import type { LinkWaitWatch } from "@orpheum/player";
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

/** What the dialog reads of a film's playback on its ladder: its offer, and its waits as they come. */
export interface TooSlowLadder {
  /** The offer of its ladder; a playback is told once, by its URL. */
  readonly offer: { readonly url: string };
  /** Whether it loads its lowest quality (isAtLowestRung, @orpheum/player). */
  readonly isAtLowestRung: () => boolean;
  /** Watches a wait for data over its chunks (watchLinkWait, @orpheum/player); none before its player. */
  readonly watchLinkWait: () => LinkWaitWatch | undefined;
}

/**
 * Tells the person, once in a playback (ladder.offer.url), that its film probably will not play on
 * this link, the tab's notice with its OK (notice.ts), once the film has waited for data with its
 * spinner shown (useIsWaitingShown) for tooSlowAtLowestAfterMs while it loaded its lowest quality
 * (isAtLowestRung, @orpheum/player; D506 §3) and its wait was the link's (watchLinkWait): a wait the
 * server holds, a chunk of it answered 425 while its node makes it, is not the link's to blame (a file of one quality, always at its lowest, waited 11 s on 425s after a seek
 * and told the owner, whose link was fine; the owner, 2026-11-17). Nothing stops: the player goes on
 * trying until its person leaves. While the server makes a chunk of it the count
 * pauses, and goes on from where it was once the wait is the link's again (the owner, 2026-11-17). A
 * wait that ends, or a quality above the lowest loading, counts again from nothing. A playback with no ladder (a film playing its
 * own file), or whose browser chooses its qualities itself, never at its lowest, tells none.
 */
export function useTellsTooSlowAtLowest(
  videoRef: RefObject<HTMLVideoElement | null>,
  ladder: TooSlowLadder | undefined,
): void {
  const { t } = useTranslation();
  const isWaiting = useIsWaitingShown(videoRef);
  const ladderRef = useLatest(ladder);
  const hasRungs = ladder !== undefined;
  const playbackUrl = ladder?.offer.url;
  const toldForRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!isWaiting || !hasRungs || toldForRef.current === playbackUrl) {
      return undefined;
    }
    let watch = ladderRef.current?.watchLinkWait();
    let waitedMs = 0;
    const look = setInterval(() => {
      watch ??= ladderRef.current?.watchLinkWait();
      const isLinks = watch?.read() !== undefined;
      const isAtLowest = ladderRef.current?.isAtLowestRung() === true;
      if (!isAtLowest) {
        waitedMs = 0;
      } else if (isLinks) {
        waitedMs += lookEveryMs;
      }
      if (waitedMs >= tooSlowAtLowestAfterMs) {
        clearInterval(look);
        toldForRef.current = playbackUrl;
        showNotice(t("play.tooSlowAtLowest"));
      }
    }, lookEveryMs);
    return () => {
      clearInterval(look);
      watch?.stop();
    };
  }, [isWaiting, hasRungs, playbackUrl, ladderRef, t]);
}
