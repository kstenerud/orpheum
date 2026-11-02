// SPDX-License-Identifier: MPL-2.0

/**
 * The player's answers to a remote's, a keyboard's and a controller's commands (input-layer.tsx;
 * controller-input-2026-10-25.md §6.1 item 4), with the player's own controls (D369): play/pause plays and
 * pauses wherever focus is; the seeking buttons and the browser's media controls seek; Left and Right
 * seek while the video has focus or nothing does, as a TV's player does with its controls hidden, or
 * its timeline; OK
 * on the video plays and pauses it; Back leaves full screen first, and only then the page. A menu or
 * dialog open over it answers Back before the player hears it.
 */

import { useEffect } from "react";

import { leaveFullScreenAsked } from "./film-fullscreen.ts";
import { arrowsAreCommandsAttribute } from "./focus-engine.ts";
import { useCommands, useMediaSeeks } from "./input-layer.tsx";
import type { Command } from "./input-buttons.ts";
import type { CommandSource, MediaTracks } from "./input-commands.ts";
import { askedByOf, seekAheadOf } from "./seek-ahead.ts";

/** Plays video where it is paused, else pauses it, making a seek being chosen first. */
function playOrPause(video: HTMLVideoElement): void {
  seekAheadOf(video).land();
  if (video.paused) {
    video.play().catch(() => undefined);
  } else {
    video.pause();
  }
}

/**
 * Moves where video's player shows a step towards direction, never before its start nor past its
 * end, and seeks it there once the presses rest (seek-ahead.ts), a key's once it is released: a
 * held key seeked at each of its repeats (the owner, Fire TV, 2026-10-29). The seek says which
 * command asked it, and from where (askedByOf): a television's overlay steps for Left and Right
 * itself, so its seek names the arrow pressed, not a seeking command.
 */
export function seekBy(
  video: HTMLVideoElement,
  direction: -1 | 1,
  asked: { readonly command: Command; readonly source: CommandSource | undefined },
): void {
  const durationMs = Number.isFinite(video.duration) ? video.duration * 1000 : Infinity;
  const film = { atMs: video.currentTime * 1000, mostMs: durationMs };
  const { command, source } = asked;
  seekAheadOf(video).step(direction, film, {
    isKey: source === "key",
    askedBy: askedByOf(command, source),
  });
}

/**
 * Whether the arrows are the video's own now: it has focus, or nothing has, or the player's own
 * timeline over it, whose Left and Right are commands (arrowsAreCommandsAttribute), as a computer's
 * bar has it (page-controls.tsx). A television's overlay answers its own timeline's before this.
 */
function isVideosToSeek(video: HTMLVideoElement): boolean {
  const focused = document.activeElement;
  if (focused === null || focused === document.body || focused === video) {
    return true;
  }
  return focused.hasAttribute(arrowsAreCommandsAttribute) && isInThePlayersControls(focused, video);
}

/** Whether element is in the player's own controls over video (playerControlsAttribute). */
function isInThePlayersControls(element: Element, video: HTMLVideoElement): boolean {
  const controls = element.closest(`[${playerControlsAttribute}]`);
  return controls !== null && controls.parentElement?.contains(video) === true;
}

/** Leaves full screen where the page is in it; whether it was. */
function leaveFullscreen(): boolean {
  if (!(document.fullscreenElement instanceof Element)) {
    return false;
  }
  leaveFullScreenAsked(document);
  return true;
}

/**
 * Each command's answer for a video, by command, told where it came from (none: a click's): whether
 * it answered. Up and Down move focus.
 */
const answers: Readonly<
  Partial<Record<Command, (video: HTMLVideoElement, source?: CommandSource) => boolean>>
> = {
  playPause: (video) => {
    playOrPause(video);
    return true;
  },
  seekBack: (video, source) => {
    seekBy(video, -1, { command: "seekBack", source });
    return true;
  },
  seekForward: (video, source) => {
    seekBy(video, 1, { command: "seekForward", source });
    return true;
  },
  left: (video, source) => {
    const isSeeking = isVideosToSeek(video);
    if (isSeeking) {
      seekBy(video, -1, { command: "left", source });
    }
    return isSeeking;
  },
  right: (video, source) => {
    const isSeeking = isVideosToSeek(video);
    if (isSeeking) {
      seekBy(video, 1, { command: "right", source });
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

/**
 * Answers command for video, as source gave it (a key's seek waits for the key's release; none is a
 * click's); whether it did.
 */
export function answerPlayerCommand(
  video: HTMLVideoElement,
  command: Command,
  source?: CommandSource,
): boolean {
  return answers[command]?.(video, source) ?? false;
}

/**
 * The attribute that marks the player's own controls over its video, as the full-screen overlay
 * (player-overlay.tsx): Space on any of them but a button plays and pauses (useSpacePlaysAndPauses).
 */
export const playerControlsAttribute = "data-player-controls";

/**
 * Whether Space is the player's while focus is where it is: on the video (the browser's own controls,
 * in its shadow, are focus on it), on nothing, or on one of the player's own controls over it that
 * Space does not press, its timeline; never on a button, which Space presses (D407 §5), nor on
 * another control the page draws over the film, as Up next, nor on anything outside the player.
 */
function isSpaceThePlayers(video: HTMLVideoElement): boolean {
  const focused = document.activeElement;
  if (focused === null || focused === document.body || focused === video) {
    return true;
  }
  return !(focused instanceof HTMLButtonElement) && isInThePlayersControls(focused, video);
}

/**
 * Plays and pauses video at Space where focus is the player's (isSpaceThePlayers), as YouTube does,
 * its keyup's own handling prevented with it: Chrome pressed the browser's full-screen button that a
 * click had left focus on, and a cast's receiver, whose video has no browser controls, did nothing
 * (the owner, 2026-10-27), and with nothing focused still plays. On one of the player's buttons
 * Space presses it, as OK does (D407 §5). Heard before what has focus, after the receiver's first press
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
    // A press on the browser's own controls leaves focus on that button, inside the video, where
    // Chrome presses it at Space before any page hears the key (the owner, 2026-10-27: the
    // full-screen button left full screen). Focus goes back to the video itself after each, and
    // after full screen changes, so Space is the player's.
    const takeFocusBack = (): void => {
      setTimeout(() => {
        video.focus({ preventScroll: true });
      }, 0);
    };
    const onFullscreenChange = (): void => {
      if (document.activeElement === video) {
        takeFocusBack();
      }
    };
    document.addEventListener("keydown", down, true);
    document.addEventListener("keyup", up, true);
    video.addEventListener("click", takeFocusBack);
    document.addEventListener("fullscreenchange", onFullscreenChange);
    return () => {
      document.removeEventListener("keydown", down, true);
      document.removeEventListener("keyup", up, true);
      video.removeEventListener("click", takeFocusBack);
      document.removeEventListener("fullscreenchange", onFullscreenChange);
    };
  }, [video]);
}

/**
 * Answers the commands for video while it is shown, the browser's media controls' seeking too, and
 * their previous and next track where tracks has them, and Space wherever focus is in the player.
 */
export function usePlayerCommands(video: HTMLVideoElement | null, tracks?: MediaTracks): void {
  useSpacePlaysAndPauses(video);
  useMediaSeeks(tracks);
  useCommands("document", (detail, event) => {
    if (video !== null && answerPlayerCommand(video, detail.command, detail.source)) {
      event.preventDefault();
    }
  });
}
