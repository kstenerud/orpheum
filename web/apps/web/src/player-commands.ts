// SPDX-License-Identifier: MPL-2.0

/**
 * The player's answers to a remote's, a keyboard's and a controller's commands (input-layer.tsx;
 * controller-input-2026-10-25.md §6.1 item 4), over the browser's own controls: play/pause plays and
 * pauses wherever focus is; the seeking buttons and the browser's media controls seek; Left and Right
 * seek while the video has focus or nothing does, as a TV's player does with its controls hidden; OK
 * on the video plays and pauses it; Back leaves full screen first, and only then the page. A menu or
 * dialog open over it answers Back before the player hears it.
 */

import { seekTo } from "@orpheum/player";
import { useEffect } from "react";

import { leaveFullScreenAsked } from "./film-fullscreen.ts";
import { useCommands, useMediaSeeks } from "./input-layer.tsx";
import type { Command } from "./input-buttons.ts";

/**
 * How far a seek moves, in milliseconds: the usual step of a TV's player, the app having had no
 * keyboard seeking of its own to follow. A justified constant.
 */
export const seekStepMs = 10_000;

/** Plays video where it is paused, else pauses it. */
function playOrPause(video: HTMLVideoElement): void {
  if (video.paused) {
    video.play().catch(() => undefined);
  } else {
    video.pause();
  }
}

/** Moves video by byMs, never before its start nor past its end. */
function seekBy(video: HTMLVideoElement, byMs: number): void {
  const durationMs = Number.isFinite(video.duration) ? video.duration * 1000 : Infinity;
  seekTo(video, Math.min(Math.max(video.currentTime * 1000 + byMs, 0), durationMs));
}

/** Whether the arrows are the video's own now: it has focus, or nothing has. */
function isVideosToSeek(video: HTMLVideoElement): boolean {
  const focused = document.activeElement;
  return focused === null || focused === document.body || focused === video;
}

/** Leaves full screen where the page is in it; whether it was. */
function leaveFullscreen(): boolean {
  if (!(document.fullscreenElement instanceof Element)) {
    return false;
  }
  leaveFullScreenAsked(document);
  return true;
}

/** Each command's answer for a video, by command: whether it answered. Up and Down move focus. */
const answers: Readonly<Partial<Record<Command, (video: HTMLVideoElement) => boolean>>> = {
  playPause: (video) => {
    playOrPause(video);
    return true;
  },
  seekBack: (video) => {
    seekBy(video, -seekStepMs);
    return true;
  },
  seekForward: (video) => {
    seekBy(video, seekStepMs);
    return true;
  },
  left: (video) => {
    const isSeeking = isVideosToSeek(video);
    if (isSeeking) {
      seekBy(video, -seekStepMs);
    }
    return isSeeking;
  },
  right: (video) => {
    const isSeeking = isVideosToSeek(video);
    if (isSeeking) {
      seekBy(video, seekStepMs);
    }
    return isSeeking;
  },
  select: (video) => {
    const isOnVideo = document.activeElement === video;
    if (isOnVideo) {
      playOrPause(video);
    }
    return isOnVideo;
  },
  back: () => leaveFullscreen(),
};

/** Answers command for video; whether it did. */
export function answerPlayerCommand(video: HTMLVideoElement, command: Command): boolean {
  return answers[command]?.(video) ?? false;
}

/**
 * The attribute that marks the player's own controls over its video, as a television's overlay
 * (tv-controls.tsx): Space on any of them plays and pauses (useSpacePlaysAndPauses).
 */
export const playerControlsAttribute = "data-player-controls";

/**
 * Whether Space is the player's while focus is where it is: on the video (the browser's own controls,
 * in its shadow, are focus on it), on the player's own controls over it, or on nothing; never on
 * another control the page draws over the film, as Up next, nor on anything outside the player.
 */
function isSpaceThePlayers(video: HTMLVideoElement): boolean {
  const focused = document.activeElement;
  if (focused === null || focused === document.body || focused === video) {
    return true;
  }
  const controls = focused.closest(`[${playerControlsAttribute}]`);
  return controls !== null && controls.parentElement?.contains(video) === true;
}

/**
 * Plays and pauses video at Space wherever focus is in the player (isSpaceThePlayers), as YouTube
 * does, its keyup's own handling prevented with it: Chrome pressed the browser's full-screen button
 * that a click had left focus on, and a cast's receiver, whose video has no browser controls, did
 * nothing (the owner, 2026-10-27). Heard before what has focus, after the receiver's first press
 * that turns the sound on (cast-receiver.tsx), which takes it first.
 */
function useSpacePlaysAndPauses(video: HTMLVideoElement | null): void {
  useEffect(() => {
    if (video === null) {
      return undefined;
    }
    let isTaken = false;
    const isSpace = (event: KeyboardEvent): boolean =>
      event.key === " " && !event.ctrlKey && !event.altKey && !event.metaKey;
    const down = (event: KeyboardEvent): void => {
      isTaken = isSpace(event) && !event.defaultPrevented && isSpaceThePlayers(video);
      if (isTaken) {
        event.preventDefault();
        if (!event.repeat) {
          playOrPause(video);
        }
      }
    };
    const up = (event: KeyboardEvent): void => {
      // A browser presses a focused button at Space's keyup: taken too, as its keydown was.
      if (isTaken && isSpace(event)) {
        event.preventDefault();
        isTaken = false;
      }
    };
    document.addEventListener("keydown", down, true);
    document.addEventListener("keyup", up, true);
    return () => {
      document.removeEventListener("keydown", down, true);
      document.removeEventListener("keyup", up, true);
    };
  }, [video]);
}

/**
 * Answers the commands for video while it is shown, the browser's media controls' seeking too, and
 * Space wherever focus is in the player.
 */
export function usePlayerCommands(video: HTMLVideoElement | null): void {
  useSpacePlaysAndPauses(video);
  useMediaSeeks();
  useCommands("document", (detail, event) => {
    if (video !== null && answerPlayerCommand(video, detail.command)) {
      event.preventDefault();
    }
  });
}
