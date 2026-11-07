// SPDX-License-Identifier: MPL-2.0

import { PlaybackTold, playerSwitchedQualityEvent, type PlaybackReport } from "@orpheum/sdk";
import { useEffect, useMemo, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { ListNote } from "./list-box.tsx";
import { rateText } from "./rate-text.ts";

/**
 * ⁝'s How it plays (D444, amending D440 §5): the file as it is or converted, the quality it plays
 * and the link it chose it on, read-only, one line that informs (ListNote). The owner: "The
 * controller side should get information about playback, such as the transcoding and bandwidth
 * choices" (D371 §4), and, of the full-screen player, "This should be available in local fullscreen
 * as well since it's useful" (2026-11-07). On the cast's remote it is the receiver's, as its state
 * comes back; in the player's ⁝, this device's own (usePlaybackHere). Nothing while nothing is said.
 */
export function HowItPlays(props: {
  readonly playback: PlaybackReport | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { playback } = props;
  if (playback === undefined) {
    return null;
  }
  const { qualityKbps, linkKbps } = playback;
  return (
    <ListNote>
      <span className="how-it-plays">
        {[
          t(`cast.playback.${playback.delivery}`),
          qualityKbps === undefined
            ? undefined
            : t("cast.playback.quality", { rate: rateText(qualityKbps) }),
          linkKbps === undefined
            ? undefined
            : t("cast.playback.link", { rate: rateText(linkKbps) }),
        ]
          .filter((each) => each !== undefined)
          .join(" · ")}
      </span>
    </ListNote>
  );
}

/**
 * How the film plays on this device, as a receiver tells its remote (PlaybackTold, D371 §4): the
 * delivery it was given, and the quality and link of the last quality its player said it began
 * (playerSwitchedQualityEvent) on the video in videoRef, which must be drawn as this is. Undefined
 * where no delivery is given.
 */
export function usePlaybackHere(
  videoRef: RefObject<HTMLVideoElement | null>,
  delivery: PlaybackReport["delivery"] | undefined,
): PlaybackReport | undefined {
  const [heard, setHeard] = useState<PlaybackReport | undefined>(undefined);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null || delivery === undefined) {
      return undefined;
    }
    const told = new PlaybackTold(delivery);
    const heardQuality = (event: Event): void => {
      if (told.heard(event)) {
        setHeard(told.report());
      }
    };
    video.addEventListener(playerSwitchedQualityEvent, heardQuality);
    return () => {
      video.removeEventListener(playerSwitchedQualityEvent, heardQuality);
    };
  }, [videoRef, delivery]);
  // Kept between renders, for what the page memoizes with it. A quality heard while the film was
  // given another way is that way's, not this one's.
  return useMemo(() => {
    if (delivery === undefined) {
      return undefined;
    }
    return heard?.delivery === delivery ? heard : { delivery };
  }, [delivery, heard]);
}
