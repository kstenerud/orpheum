// SPDX-License-Identifier: MPL-2.0

import { useLatest } from "./latest.ts";
import { isPausedAsAsked } from "./film-waits.tsx";
import { answerPlayerCommand, seekBy } from "./player-commands.ts";
import type { PlaybackTarget } from "./playback-target.ts";
import type { RefObject } from "react";

/**
 * What the overlay's presses work on: the video here, or another target (a castee) with its picture.
 * Over a target that takes no press now (isReady false: a castee not heard, D555 §6) its play and
 * seeks do nothing.
 */
export interface OverlayStage {
  /** What focus goes to as the overlay hides, the video or the target's picture. */
  readonly picture: HTMLElement;
  readonly isPaused: boolean;
  readonly playPause: () => void;
  /** Seeks by seekByMs, the video by a step its press names (asked), a target by seekByMs itself. */
  readonly seekBy: (seekByMs: number, asked: Parameters<typeof seekBy>[2]) => void;
}

/** The stage the overlay's presses work on now (OverlayStage); undefined while none is drawn. */
function stageOf(
  video: HTMLVideoElement | null,
  picture: HTMLElement | null,
  target: PlaybackTarget | undefined,
): OverlayStage | undefined {
  if (target === undefined) {
    return video === null
      ? undefined
      : {
          picture: video,
          isPaused: isPausedAsAsked(video),
          playPause: () => {
            answerPlayerCommand(video, "playPause");
          },
          seekBy: (seekByMs, asked) => {
            seekBy(video, seekByMs < 0 ? -1 : 1, asked);
          },
        };
  }
  if (picture === null) {
    return undefined;
  }
  return {
    picture,
    isPaused: target.isPaused,
    playPause: () => {
      if (target.isReady) {
        target.playOrPause();
      }
    },
    seekBy: (seekByMs) => {
      if (target.isReady) {
        target.seekTo(Math.max(0, target.atMs + seekByMs));
      }
    },
  };
}

/**
 * What reads the stage the overlay's presses work on as each comes (stageOf): the video in videoRef,
 * or, given a target, the target with the picture in pictureRef.
 */
export function useStageNow(
  refs: {
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    readonly pictureRef: RefObject<HTMLElement | null>;
  },
  target: PlaybackTarget | undefined,
): () => OverlayStage | undefined {
  const targetRef = useLatest(target);
  const { videoRef, pictureRef } = refs;
  return () => stageOf(videoRef.current, pictureRef.current, targetRef.current);
}

/**
 * Moves focus to the picture, the video or a target's, which takes it from a script alone where it
 * is no stop of its own.
 */
export function focusThePicture(picture: HTMLElement): void {
  if (!picture.hasAttribute("tabindex")) {
    picture.setAttribute("tabindex", "-1");
  }
  picture.focus();
}
