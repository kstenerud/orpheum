// SPDX-License-Identifier: MPL-2.0

import "./up-next.css";
import "./play-words.ts";

import type { JSX, ReactNode, RefObject } from "react";
import { useTranslation } from "react-i18next";

import type { UpNext } from "./around-file.ts";
import { placeCode } from "./episode-place.ts";
import { OfferCard } from "./offer-card.tsx";
import { Still } from "./poster.tsx";

/**
 * How long autoplay counts down before it plays the next episode, in ms: long enough to read the
 * offer and press, as every service's countdown is. A justified constant. It starts at the start
 * of the film's credits, where a segment says where they are, else at its end, so a fixed lead never
 * cuts the programme's last seconds (D139, D339; plans/up-next.md §3, §9).
 */
export const upNextCountdownMs = 10_000;

/**
 * The Up next card (OfferCard, D538 §1): the next episode's still, "Up next: ‹place› · ‹name›", then
 * "Plays in 10 s" while the count runs, said once as it starts and not at each second, or "Play now"
 * where it is pressed; the card plays it, and has no ✕ (Back puts it away: UpNextOffer). Where
 * nothing here presses it (a cast's receiver), its words alone. The cast's remote's Up next is this
 * card, playing it there.
 */
export function UpNextCard(props: {
  readonly upNext: UpNext;
  /** The count's seconds left while it runs; none, no count. */
  readonly secondsLeft?: number | undefined;
  /** Its press, where it is pressed here. */
  readonly play?:
    | {
        readonly onPlay: () => void;
        readonly playRef?: RefObject<HTMLButtonElement | null>;
      }
    | undefined;
  /** Its buttons beside it, where its user has any: the cast remote's ✕ (D548 §3). */
  readonly buttons?: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const { upNext, secondsLeft, play } = props;
  const { episode } = upNext;
  const place = placeCode(t, upNext.ordering, { ...episode, isOnlyGroup: upNext.isOnlyGroup });
  const isCounting = secondsLeft !== undefined;
  const countdown = isCounting ? t("upNext.countdown", { seconds: secondsLeft }) : undefined;
  return (
    <OfferCard
      words={t("upNext.offer", { place, name: episode.name ?? "" })}
      said={
        isCounting
          ? t("upNext.countdownSaid", { seconds: Math.round(upNextCountdownMs / 1000) })
          : undefined
      }
      detail={countdown ?? (play === undefined ? undefined : t("upNext.playNow"))}
      still={
        <Still sourceSha256={upNext.stillSha256} className="up-next__still" isNoPosterMarked />
      }
      onTake={play?.onPlay}
      takeRef={play?.playRef}
      buttons={props.buttons}
    />
  );
}
