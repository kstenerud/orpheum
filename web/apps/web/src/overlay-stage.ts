// SPDX-License-Identifier: MPL-2.0

import { useLatest } from "./latest.ts";
import { stageOfTarget, stageOfVideo, type PlayerStage } from "./player-commands.ts";
import type { PlaybackTarget } from "./playback-target.ts";
import type { CommandRights } from "./player-controls.tsx";
import type { RefObject } from "react";

/**
 * The stage the overlay's presses and the player's commands work on now (PlayerStage): the video,
 * its play and pause and seeks as far as rights give them (a Watch Together guest's, D557 §6), or,
 * given a target, the target with its picture; undefined while none is drawn.
 */
function stageOf(
  video: HTMLVideoElement | null,
  picture: HTMLElement | null,
  given: {
    readonly target: PlaybackTarget | undefined;
    readonly rights: CommandRights | undefined;
  },
): PlayerStage | undefined {
  const { target, rights } = given;
  if (target === undefined) {
    return video === null ? undefined : stageOfVideo(video, rights);
  }
  return picture === null ? undefined : stageOfTarget(target, picture);
}

/**
 * What reads the stage the overlay's presses and the player's commands work on as each comes
 * (stageOf): the video in videoRef, as far as rights give (every right where none is given), or,
 * given a target, the target with the picture in pictureRef.
 */
export function useStageNow(
  refs: {
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    readonly pictureRef: RefObject<HTMLElement | null>;
  },
  target: PlaybackTarget | undefined,
  rights?: CommandRights,
): () => PlayerStage | undefined {
  const targetRef = useLatest(target);
  const rightsRef = useLatest(rights);
  const { videoRef, pictureRef } = refs;
  return () =>
    stageOf(videoRef.current, pictureRef.current, {
      target: targetRef.current,
      rights: rightsRef.current,
    });
}

/**
 * Moves focus to the picture, the video or a target's, which takes it from a script alone where it
 * is no stop of its own, so the keys of the player reach it. The picture never shows the ring (the
 * owner, 2026-11-26): the ring says where a press would land, and the picture is the whole stage.
 */
export function focusThePicture(picture: HTMLElement): void {
  if (!picture.hasAttribute("tabindex")) {
    picture.setAttribute("tabindex", "-1");
  }
  picture.focus({ focusVisible: false });
}
