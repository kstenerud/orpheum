// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { useLatest } from "./latest.ts";
import { Pill } from "./pill.tsx";
import { PauseIcon } from "./player-icons.tsx";
import type { PlayerSound, PlayerSource } from "./playback-target.ts";
import { volumeShownForMs, type OverlayState } from "./player-overlay-state.ts";
import type { SeekFeedback } from "./player-taps.ts";
import { StartRefusedShown } from "./start-refused-shown.tsx";

/** The signs over the picture besides the pause sign (OverThePicture), as the overlay has them. */
export interface PictureSigns {
  readonly isVolumeShown: boolean;
  readonly seekFeedback: SeekFeedback | undefined;
  /** Whether the browser refused the film's start and it has not played since (useStartRefused). */
  readonly isStartRefused: boolean;
}

/**
 * What shows over the picture, outside the overlay's rows (standards/ui.md, over the film, rule 15):
 * the pause sign after OK paused, with the overlay hidden alone, as the Play button says it
 * otherwise; in the centre one Pill at a time, the volume a moment or the line over a film the
 * browser would not start, the newer replacing the older; and a double tap's seek in the third
 * tapped, the one Pill outside the centre.
 */
export function OverThePicture(
  props: PictureSigns & { readonly state: OverlayState; readonly source: PlayerSource },
): JSX.Element {
  const { state, source } = props;
  const isRefusedShown = props.isStartRefused && source.isPaused && !props.isVolumeShown;
  return (
    <>
      <span className="player-overlay-paused" hidden={state.shown !== "paused"} aria-hidden="true">
        <PauseIcon />
      </span>
      <StartRefusedShown isShown={isRefusedShown} />
      <SeekFeedbackShown feedback={props.seekFeedback} />
      <VolumeShown sound={source.sound} isShown={props.isVolumeShown} />
    </>
  );
}

/**
 * Whether the volume shows, and what shows it: from a press of + or − for volumeShownForMs after
 * the last (D407 §10), as the scrub bar shows alone after a seek.
 */
export function useVolumeShown(): { readonly isShown: boolean; readonly show: () => void } {
  const [pressCount, setPressCount] = useState(0);
  const [isShown, setIsShown] = useState(false);
  useEffect(() => {
    if (pressCount === 0) {
      return undefined;
    }
    const timer = setTimeout(() => {
      setIsShown(false);
    }, volumeShownForMs);
    return () => {
      clearTimeout(timer);
    };
  }, [pressCount]);
  return {
    isShown,
    show: () => {
      setIsShown(true);
      setPressCount((count) => count + 1);
    },
  };
}

/**
 * Shows the volume as a castee's changes with no press here: its TV's own remote moved it, which
 * the caster's pad showed nothing of (the owner, 2026-11-27). The first volume heard is no change.
 */
export function useShowsACasteesVolume(sound: PlayerSound | undefined, show: () => void): void {
  const level = sound?.volume;
  const isMuted = sound?.isMuted;
  const heardRef = useRef({ level, isMuted });
  const showRef = useLatest(show);
  useEffect(() => {
    const heard = heardRef.current;
    heardRef.current = { level, isMuted };
    if (heard.level !== undefined && level !== undefined) {
      if (heard.level !== level || heard.isMuted !== isMuted) {
        showRef.current();
      }
    }
  }, [level, isMuted, showRef]);
}

/**
 * A double-tap seek's "+10" or "−10", stacking as "+20", centred in the third tapped, fading
 * shortly after the last tap (D415 §5): a silent Pill, as the seek is what a screen reader's user
 * has, not a second announcement; it takes no pointer and no focus.
 */
function SeekFeedbackShown(props: {
  readonly feedback: SeekFeedback | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { feedback } = props;
  if (feedback === undefined) {
    return null;
  }
  const key =
    feedback.direction === 1 ? "playerControls.seekedForward" : "playerControls.seekedBack";
  return (
    <span
      className="player-overlay-seeked"
      data-side={feedback.direction === 1 ? "right" : "left"}
      data-shown={feedback.isShown ? "" : undefined}
      aria-hidden="true"
    >
      <Pill voice="silent">{t(key, { seconds: feedback.seconds })}</Pill>
    </span>
  );
}

/**
 * The volume + or − left, a volume Pill in the middle of the film a moment (useVolumeShown); muted
 * is none. Its place stays while it is hidden, the Pill drawn only while it shows, so a page's own
 * line over the film is the one status there meanwhile.
 */
export function VolumeShown(props: {
  readonly sound: PlayerSound | undefined;
  readonly isShown: boolean;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { sound } = props;
  if (sound === undefined) {
    return null;
  }
  const percent = sound.isMuted ? 0 : Math.round(sound.volume * 100);
  return (
    <span className="player-overlay-centre player-overlay-volume" hidden={!props.isShown}>
      {props.isShown ? (
        <Pill voice="volume">{t("playerControls.volumeLevel", { percent })}</Pill>
      ) : null}
    </span>
  );
}
