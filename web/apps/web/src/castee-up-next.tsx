// SPDX-License-Identifier: MPL-2.0

import "./cast-words.ts";

import { offerState } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import type { UpNext } from "./around-file.ts";
import { IconButton } from "./icon-button.tsx";
import type { PlaybackTarget } from "./playback-target.ts";
import { CloseIcon } from "./player-icons.tsx";
import { UpNextCard } from "./up-next-card.tsx";

/**
 * Whether the castee's film is near its end for Up next (offerState, D555 §3): from its credits
 * where creditsStartMs says where they start, else upNextLeadMs before its end, read off the target
 * as the session says it stands (useCasteeTarget).
 */
export function isCasteeNearItsEnd(
  target: Pick<PlaybackTarget, "atMs" | "durationMs">,
  creditsStartMs: number | undefined,
): boolean {
  return offerState(
    { atMs: target.atMs, durationMs: target.durationMs, hasEnded: false, creditsStartMs },
    { isDismissed: false, isCancelled: false },
    false,
  ).isNearTheEnd;
}

/**
 * A castee's Up next (TargetUpNext), near its film's end (isCasteeNearItsEnd): the next episode, the
 * Up next card with no count (UpNextCard, D538 §1), which moves the cast on to it (onPlay), the
 * receiver handed it (plans/up-next.md), and its ✕, Dismiss, which puts it away on this device alone
 * until the film is moved back from its end (D548 §3, amending the owner's 2026-11-23 none): the
 * remote's page has no Back to put it away; nothing where none can play next.
 */
export function CasteeUpNext(props: {
  readonly target: PlaybackTarget;
  readonly upNext: UpNext;
  readonly creditsStartMs: number | undefined;
  readonly onPlay: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { upNext } = props;
  const [isDismissed, setIsDismissed] = useState(false);
  const isNearTheEnd = isCasteeNearItsEnd(props.target, props.creditsStartMs);
  if (isDismissed && !isNearTheEnd) {
    setIsDismissed(false); // moved back from its end: offered again as it comes back
  }
  if (!isNearTheEnd || isDismissed || upNext.episode.unavailable !== undefined) {
    return null;
  }
  return (
    <UpNextCard
      upNext={upNext}
      play={{ onPlay: props.onPlay }}
      buttons={
        <IconButton
          label={t("cast.dismissUpNext")}
          icon={<CloseIcon />}
          isOverVideo
          onClick={() => {
            setIsDismissed(true);
          }}
        />
      }
    />
  );
}
