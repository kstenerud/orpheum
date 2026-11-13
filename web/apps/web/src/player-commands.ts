// SPDX-License-Identifier: MPL-2.0

/**
 * The player's answers to a remote's, a keyboard's and a controller's commands (input-layer.tsx;
 * controller-input-2026-10-25.md §6.1 item 4), with the player's own controls (D369): play/pause plays and
 * pauses wherever focus is; the seeking buttons and the browser's media controls seek; Left and Right
 * seek while the video has focus or nothing does, as a TV's player does with its controls hidden, or
 * its timeline; OK
 * on the video plays and pauses it; Back leaves full screen first, and only then the page; + and −
 * move the volume a step (D407 §10). A menu or dialog open over it answers Back before the player
 * hears it.
 */

import { heardVolumeOf, setHeardVolume } from "@orpheum/player";
import { useEffect, type RefObject } from "react";

import { leaveFullScreenAsked } from "./film-fullscreen.ts";
import { arrowsAreCommandsAttribute } from "./focus-engine.ts";
import { volumeAfter } from "./volume-step.ts";
import { useCommands, useMediaSeeks } from "./input-layer.tsx";
import type { Command } from "./input-buttons.ts";
import type { CommandHold, CommandSource, MediaTracks } from "./input-commands.ts";
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
 * end, and seeks it there once the presses rest (seek-ahead.ts), a held one's once it is released,
 * a remote's key and a controller's stick alike (CommandHold): a held key seeked at each of its
 * repeats (the owner, Fire TV, 2026-10-29). The seek says which command asked it, and from where
 * (askedByOf): a television's overlay steps for Left and Right itself, so its seek names the arrow
 * pressed, not a seeking command.
 */
export function seekBy(video: HTMLVideoElement, direction: -1 | 1, asked: AskedCommand): void {
  const durationMs = Number.isFinite(video.duration) ? video.duration * 1000 : Infinity;
  const film = { atMs: video.currentTime * 1000, mostMs: durationMs };
  const { command, source, hold } = asked;
  seekAheadOf(video).step(direction, film, { hold, askedBy: askedByOf(command, source) });
}

/** A command as the player is asked it: where it came from (none: a click's), and how it is held. */
interface AskedCommand {
  readonly command: Command;
  readonly source?: CommandSource | undefined;
  readonly hold?: CommandHold | undefined;
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

/** Whether element is in the player's own controls for video (markPlayersControls). */
function isInThePlayersControls(element: Element, video: HTMLVideoElement): boolean {
  const controls = element.closest(`[${playerControlsAttribute}]`);
  return controls !== null && videosOfControls.get(controls)?.current === video;
}

/**
 * The player's own controls (playerControlsAttribute), each with the video it commands, as the
 * controls name it: the bar under the film is not in the video's stage (D407 §9), so where an element
 * lies says nothing of the video it is for (plans/player-overlay.md §6).
 */
const videosOfControls = new WeakMap<Element, RefObject<HTMLVideoElement | null>>();

/** Marks controls as the player's own for the video in videoRef; returns the unmarking. */
export function markPlayersControls(
  controls: Element,
  videoRef: RefObject<HTMLVideoElement | null>,
): () => void {
  videosOfControls.set(controls, videoRef);
  return () => {
    videosOfControls.delete(controls);
  };
}

/** Marks the controls in controlsRef as the player's own for the video in videoRef while drawn. */
export function usePlayersControls(
  controlsRef: RefObject<HTMLElement | null>,
  videoRef: RefObject<HTMLVideoElement | null>,
): void {
  useEffect(() => {
    const controls = controlsRef.current;
    return controls === null ? undefined : markPlayersControls(controls, videoRef);
  }, [controlsRef, videoRef]);
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
 * Moves video's volume a step towards direction (volumeAfter, volume-step.ts); a step up turns a muted film's sound
 * on, as a player's volume key does. The element's own properties are what set its sound.
 */
function stepVolume(video: HTMLVideoElement, direction: -1 | 1): boolean {
  // The volume the person set, not the element's, which a lowering by levelling is in (D436).
  setHeardVolume(video, volumeAfter(heardVolumeOf(video), direction));
  if (direction === 1) {
    // eslint-disable-next-line no-param-reassign -- a step up turns the sound on, as a volume key does
    video.muted = false;
  }
  return true;
}

/**
 * Each command's answer for a video, by command, told how it was asked (AskedCommand): whether it
 * answered. Up and Down move focus.
 */
const answers: Readonly<
  Partial<Record<Command, (video: HTMLVideoElement, asked: AskedCommand) => boolean>>
> = {
  playPause: (video) => {
    playOrPause(video);
    return true;
  },
  seekBack: (video, asked) => {
    seekBy(video, -1, asked);
    return true;
  },
  seekForward: (video, asked) => {
    seekBy(video, 1, asked);
    return true;
  },
  left: (video, asked) => {
    const isSeeking = isVideosToSeek(video);
    if (isSeeking) {
      seekBy(video, -1, asked);
    }
    return isSeeking;
  },
  right: (video, asked) => {
    const isSeeking = isVideosToSeek(video);
    if (isSeeking) {
      seekBy(video, 1, asked);
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
  volumeUp: (video) => stepVolume(video, 1),
  volumeDown: (video) => stepVolume(video, -1),
};

/**
 * Answers command for video, as asked: from where (none: a click's), and how it is held (a held
 * seek waits for its release); whether it did.
 */
export function answerPlayerCommand(
  video: HTMLVideoElement,
  command: Command,
  asked: Omit<AskedCommand, "command"> = {},
): boolean {
  return answers[command]?.(video, { ...asked, command }) ?? false;
}

/**
 * The attribute that marks the player's own controls for its video, the full-screen overlay
 * (player-overlay.tsx) and the bar under the film (page-controls.tsx), each marked for its video by
 * markPlayersControls: Space on any of them but a button plays and pauses (useSpacePlaysAndPauses).
 */
export const playerControlsAttribute = "data-player-controls";

/**
 * Whether Space is the player's while focus is where it is: on the video (the browser's own controls,
 * in its shadow, are focus on it), on nothing, or on one of the player's own controls over it that
 * Space does not press, its timeline; never on a button, which Space presses (D407 §5), nor on
 * another control the page draws over the film, as Up next, nor in a menu the overlay opened (⁝'s
 * switches, D407 §8), nor on anything outside the player.
 */
function isSpaceThePlayers(video: HTMLVideoElement): boolean {
  const focused = document.activeElement;
  if (focused === null || focused === document.body || focused === video) {
    return true;
  }
  return (
    !focused.matches('button, [role="button"]') &&
    focused.closest("dialog") === null &&
    isInThePlayersControls(focused, video)
  );
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
    // Leaving full screen, the bar under the film may move focus to its play first (D407 §7):
    // the video takes it back only where nothing else has.
    const onFullscreenChange = (): void => {
      if (document.activeElement === video) {
        setTimeout(() => {
          if (document.activeElement === video || document.activeElement === document.body) {
            video.focus({ preventScroll: true });
          }
        }, 0);
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
    if (video !== null && answerPlayerCommand(video, detail.command, detail)) {
      event.preventDefault();
    }
  });
}
