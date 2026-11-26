// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import type { JSX, RefObject } from "react";
import { useTranslation } from "react-i18next";

import type { PlayerSource } from "./playback-target.ts";
import { ClockTimes, shownAtMsOf, Timeline } from "./timeline.tsx";

/**
 * The one timeline over a playback target (D555 §8, §10): the film's place on its slider, a seek
 * sent to the target, disabled until the target is ready (or as isDisabled says), and its times,
 * in it (the bar's) or apart in a span of timesClassName (the overlay's and the cast remote's, the
 * time under it). The Play page's bar, its overlay and the cast remote draw it, so
 * each reads the target alone.
 */
export function TargetTimeline(props: {
  readonly target: PlayerSource;
  readonly isDisabled?: boolean | undefined;
  /** Whether the slider's arrows are the player's seek commands (Timeline). */
  readonly hasArrowsAsCommands?: boolean | undefined;
  readonly inputRef?: RefObject<HTMLInputElement | null> | undefined;
  /** The class of the span the times are drawn in apart from the slider; undefined: in it. */
  readonly timesClassName?: string | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { target, timesClassName } = props;
  const isApart = timesClassName !== undefined;
  return (
    <>
      <Timeline
        inputRef={props.inputRef}
        label={t("playerControls.where")}
        atMs={target.atMs}
        durationMs={target.durationMs}
        isDisabled={props.isDisabled ?? !isSeekable(target)}
        hasArrowsAsCommands={props.hasArrowsAsCommands}
        onSeek={target.seekTo}
        hasTimes={!isApart}
      />
      {isApart ? (
        <span className={timesClassName}>
          <ClockTimes
            atMs={shownAtMsOf(target.atMs, target.durationMs)}
            durationMs={target.durationMs}
          />
        </span>
      ) : null}
    </>
  );
}

/** Whether a target's timeline takes a seek: it is ready, and its seeks are taken (canSeek). */
export function isSeekable(target: PlayerSource): boolean {
  return target.isReady && target.canSeek !== false;
}
