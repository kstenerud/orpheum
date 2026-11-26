// SPDX-License-Identifier: MPL-2.0

import { focusAfterNoPress } from "./focus-navigation.ts";
import type { CommandSource } from "./input-commands.ts";
import { useLatest } from "./latest.ts";
import { stageOfTarget, stageOfVideo, type PlayerStage } from "./player-commands.ts";
import type { PlaybackTarget } from "./playback-target.ts";
import type { RefObject } from "react";

/**
 * The stage the overlay's presses and the player's commands work on now (PlayerStage): the video,
 * or, given a target, the target with its picture; undefined while none is drawn.
 */
function stageOf(
  video: HTMLVideoElement | null,
  picture: HTMLElement | null,
  target: PlaybackTarget | undefined,
): PlayerStage | undefined {
  if (target === undefined) {
    return video === null ? undefined : stageOfVideo(video);
  }
  return picture === null ? undefined : stageOfTarget(target, picture);
}

/**
 * What reads the stage the overlay's presses and the player's commands work on as each comes
 * (stageOf): the video in videoRef, or, given a target, the target with the picture in pictureRef.
 */
export function useStageNow(
  refs: {
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    readonly pictureRef: RefObject<HTMLElement | null>;
  },
  target: PlaybackTarget | undefined,
): () => PlayerStage | undefined {
  const targetRef = useLatest(target);
  const { videoRef, pictureRef } = refs;
  return () => stageOf(videoRef.current, pictureRef.current, targetRef.current);
}

/**
 * Moves focus to the picture, the video or a target's, which takes it from a script alone where it
 * is no stop of its own: shown as the browser shows a script's focus after the key or controller
 * source names, and with no ring where none has pressed (focusAfterNoPress).
 */
export function focusThePicture(picture: HTMLElement, source: CommandSource | undefined): void {
  if (!picture.hasAttribute("tabindex")) {
    picture.setAttribute("tabindex", "-1");
  }
  if (source === undefined) {
    focusAfterNoPress(picture);
  } else {
    picture.focus();
  }
}
