// SPDX-License-Identifier: MPL-2.0

import "./cast-words.ts";
import "./how-it-plays.css";

import { PlaybackTold, playerSwitchedQualityEvent, type PlaybackReport } from "@orpheum/sdk";
import { useEffect, useMemo, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { ConnectionIcon, PlayIcon } from "./player-icons.tsx";
import { ListNote } from "./list-box.tsx";
import { rateText } from "./rate-text.ts";

/**
 * ⁝'s How it plays (D444, amending D440 §5), the line at the foot of the menu (the menu spec of D538:
 * "▶ Converted 12.1 Mb/s · 📶 69.2 Mb/s"): a play mark, then the file as it is, "Original", or
 * "Converted", with the quality it plays; a signal's bars, then the link it chose it on. Read-only,
 * one line that informs (ListNote); the marks are the design's icons at the text's size, said to a
 * screen reader as "Playing" and "Connection". The owner: "The controller side should get information
 * about playback, such as the transcoding and bandwidth choices" (D371 §4), and, of the full-screen
 * player, "This should be available in local fullscreen as well since it's useful" (2026-11-07); and
 * of the line, "the text about how it's playing was supposed to use shortened text and icons"
 * (2026-11-23). On the cast's remote it is the receiver's, as its state comes back; in the player's
 * ⁝, this device's own (usePlaybackHere). Nothing while nothing is said.
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
  const how = t(`cast.playback.${playback.delivery}`);
  return (
    <ListNote isOneLine>
      <span className="how-it-plays">
        <span className="how-it-plays__fact">
          <PlayIcon className="how-it-plays__icon" />
          <span className="how-it-plays__said">{t("cast.playback.playing")}</span>
          {qualityKbps === undefined
            ? how
            : t("cast.playback.atRate", { how, rate: rateText(qualityKbps) })}
        </span>
        {linkKbps === undefined ? null : (
          <>
            {" · "}
            <span className="how-it-plays__fact">
              <ConnectionIcon className="how-it-plays__icon" />
              <span className="how-it-plays__said">{t("cast.playback.connection")}</span>
              {rateText(linkKbps)}
            </span>
          </>
        )}
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
