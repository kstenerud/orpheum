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

import {
  heardVolumeOf,
  setHeardVolume,
  watchMediaSession,
  type MediaSessionAction,
} from "@orpheum/player";
import { startGateOf } from "@orpheum/sdk";
import { useCallback, useEffect, useMemo, type RefObject } from "react";

import { leaveFullScreenAsked, toggleFullScreen } from "./film-fullscreen.ts";
import { arrowsAreCommandsAttribute, caretOf } from "./focus-engine.ts";
import { isMeantToPlay, isPausedAsAsked } from "./film-waits.tsx";
import { useLatest } from "./latest.ts";
import type { PlaybackTarget } from "./playback-target.ts";
import { remoteVolumeStep, volumeAfter } from "./volume-step.ts";
import { useCommands, useMediaPlayPause, useMediaSeeks } from "./input-layer.tsx";
import type { Command } from "./input-buttons.ts";
import type { CommandHold, CommandSource, MediaTracks } from "./input-commands.ts";
import { askedByOf, seekAheadOf, seekStepMs } from "./seek-ahead.ts";
import { keepMediaActionsOf } from "./play-state-video-events.ts";
import { monotonicMs } from "./use-together.ts";

/**
 * Plays video where it is paused as asked (isPausedAsAsked in film-waits.tsx), else pauses it, making
 * a seek being chosen first. A film meant to play that waits for enough to be loaded shows Pause, and
 * a press pauses it for good: its start gate's play is given up (the owner, 2026-11-15).
 */
function playOrPause(video: HTMLVideoElement): void {
  seekAheadOf(video).land();
  if (isMeantToPlay(video)) {
    startGateOf(video).cancel();
  } else if (video.paused) {
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
export interface AskedCommand {
  readonly command: Command;
  readonly source?: CommandSource | undefined;
  readonly hold?: CommandHold | undefined;
}

/**
 * Whether the arrows are the film's own now: its picture (the video, or a castee's stage) has focus,
 * or nothing has, or the player's own timeline over it, whose Left and Right are commands
 * (arrowsAreCommandsAttribute), as a computer's bar has it (page-controls.tsx). A television's
 * overlay answers its own timeline's before this.
 */
function isArrowsTheFilms(picture: HTMLElement): boolean {
  const focused = document.activeElement;
  if (focused === null || focused === document.body || focused === picture) {
    return true;
  }
  return (
    focused.hasAttribute(arrowsAreCommandsAttribute) && isInThePlayersControls(focused, picture)
  );
}

/** Whether element is in the player's own controls for the film in picture (markPlayersControls). */
function isInThePlayersControls(element: Element, picture: HTMLElement): boolean {
  const controls = element.closest(`[${playerControlsAttribute}]`);
  return controls !== null && picturesOfControls.get(controls)?.current === picture;
}

/**
 * The player's own controls (playerControlsAttribute), each with the picture of the film it commands
 * (the video, or a castee's stage: PlayerStage), as the controls name it: the bar under the film is
 * not in the video's stage (D407 §9), so where an element lies says nothing of the film it is for
 * (plans/player-overlay.md §6).
 */
const picturesOfControls = new WeakMap<Element, RefObject<HTMLElement | null>>();

/** Marks controls as the player's own for the film in pictureRef; returns the unmarking. */
export function markPlayersControls(
  controls: Element,
  pictureRef: RefObject<HTMLElement | null>,
): () => void {
  picturesOfControls.set(controls, pictureRef);
  return () => {
    picturesOfControls.delete(controls);
  };
}

/**
 * Marks the controls in controlsRef as the player's own for the film in pictureRef (the video, or a
 * castee's stage) while drawn.
 */
export function usePlayersControls(
  controlsRef: RefObject<HTMLElement | null>,
  pictureRef: RefObject<HTMLElement | null>,
): void {
  useEffect(() => {
    const controls = controlsRef.current;
    return controls === null ? undefined : markPlayersControls(controls, pictureRef);
  }, [controlsRef, pictureRef]);
}

/**
 * What the player's commands, its keys and its overlay's presses work on (D555 §8): the video here
 * (stageOfVideo), or another target, a castee, with its picture (stageOfTarget). A target that takes
 * nothing now (isReady false: a castee not heard, D555 §6) plays, seeks and sets no sound.
 */
export interface PlayerStage {
  /**
   * The video, or the target's picture: focus on it is focus on the film, the overlay gives focus
   * to it as it hides, and the stage it is in is what full screen fills.
   */
  readonly picture: HTMLElement;
  readonly isPaused: boolean;
  /** Whether it takes play, seeks and sound now: the video always, a castee while heard. */
  readonly isReady: boolean;
  /**
   * Whether its play and pause, and its seeks, are taken: false only for a Watch Together guest whose
   * link does not give them (CommandRights, D271 §4), whose keys then do what its buttons do, none
   * of those buttons drawn (D557 §6).
   */
  readonly rights: CommandRights;
  readonly playPause: () => void;
  /** Seeks by seekByMs, the video by a step its press names (asked), a target by seekByMs itself. */
  readonly seekBy: (seekByMs: number, asked: AskedCommand) => void;
  /** Moves the volume a step towards direction, a step up turning the sound on; whether it did. */
  readonly stepVolume: (direction: -1 | 1) => boolean;
  /** Turns the sound off, or on again (M); whether it did. */
  readonly toggleMute: () => boolean;
}

/**
 * What the controls over the video here may command: play and pause, and seeks. Every page's video
 * takes both, but a Watch Together guest's, whose link's rights say which they have (D271 §4): their
 * Play page draws only the controls those give (D555 §8, §10), its group undoing any other change
 * (followGroup).
 */
export interface CommandRights {
  readonly canPlayPause: boolean;
  readonly canSeek: boolean;
}

/** Every right a player's commands can have: any page's video's but a Watch Together guest's. */
const everyRight: CommandRights = { canPlayPause: true, canSeek: true };

/**
 * The video as the stage the player's commands work on (PlayerStage), its play and pause and its
 * seeks taken as far as rights give them: a Watch Together guest's as far as their link's (D557 §6).
 */
export function stageOfVideo(
  video: HTMLVideoElement,
  rights: CommandRights = everyRight,
): PlayerStage {
  return {
    picture: video,
    isPaused: isPausedAsAsked(video),
    isReady: true,
    rights,
    playPause: () => {
      if (rights.canPlayPause) {
        playOrPause(video);
      }
    },
    seekBy: (seekByMs, asked) => {
      if (rights.canSeek) {
        seekBy(video, seekByMs < 0 ? -1 : 1, asked);
      }
    },
    stepVolume: (direction) => stepVolume(video, direction),
    toggleMute: () => {
      // eslint-disable-next-line no-param-reassign -- the element's own property is what mutes it
      video.muted = !video.muted;
      return true;
    },
  };
}

/**
 * Another target, a castee, as the stage the player's commands work on, its picture where the video
 * would be (PlayerStage): nothing taken while it is not ready (D555 §6); its sound, where it says
 * one, stepped as the cast remote's Volume down and up did, remoteVolumeStep a press (D548 §6), a
 * step up turning a muted film's sound on first.
 */
export function stageOfTarget(target: PlaybackTarget, picture: HTMLElement): PlayerStage {
  const { sound, isReady } = target;
  const rights = { canPlayPause: target.canPlayPause !== false, canSeek: target.canSeek !== false };
  return {
    picture,
    isPaused: target.isPaused,
    isReady,
    rights,
    playPause: () => {
      if (isReady) {
        target.playOrPause();
      }
    },
    seekBy: (seekByMs) => {
      if (isReady) {
        target.seekTo(Math.max(0, target.atMs + seekByMs));
      }
    },
    stepVolume: (direction) => {
      if (!isReady || sound === undefined) {
        return false;
      }
      if (direction === 1 && sound.isMuted) {
        sound.setMuted(false);
      }
      sound.setVolume(volumeAfter(sound.volume, direction, remoteVolumeStep));
      return true;
    },
    toggleMute: () => {
      if (!isReady || sound === undefined) {
        return false;
      }
      sound.setMuted(!sound.isMuted);
      return true;
    },
  };
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

/** Whether stage takes play and pause now: it is ready, and its rights give them. */
function takesPlayPause(stage: PlayerStage): boolean {
  return stage.isReady && stage.rights.canPlayPause;
}

/** Plays or pauses stage where it takes play and pause (takesPlayPause); whether it did. */
function playsOrPauses(stage: PlayerStage): boolean {
  const isTaken = takesPlayPause(stage);
  if (isTaken) {
    stage.playPause();
  }
  return isTaken;
}

/** Seeks stage a step towards direction where it is ready and its rights give seeks; whether it did. */
function seeks(stage: PlayerStage, direction: -1 | 1, asked: AskedCommand): boolean {
  const isTaken = stage.isReady && stage.rights.canSeek;
  if (isTaken) {
    stage.seekBy(direction * seekStepMs, asked);
  }
  return isTaken;
}

/**
 * Each command's answer for a stage (PlayerStage), the video or a castee, by command, told how it
 * was asked (AskedCommand): whether it answered. Up and Down move focus.
 */
const answers: Readonly<
  Partial<Record<Command, (stage: PlayerStage, asked: AskedCommand) => boolean>>
> = {
  playPause: (stage) => playsOrPauses(stage),
  seekBack: (stage, asked) => seeks(stage, -1, asked),
  seekForward: (stage, asked) => seeks(stage, 1, asked),
  left: (stage, asked) => isArrowsTheFilms(stage.picture) && seeks(stage, -1, asked),
  right: (stage, asked) => isArrowsTheFilms(stage.picture) && seeks(stage, 1, asked),
  select: (stage) => document.activeElement === stage.picture && playsOrPauses(stage),
  back: () => leaveFullscreen(),
  volumeUp: (stage) => stage.stepVolume(1),
  volumeDown: (stage) => stage.stepVolume(-1),
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
  return answers[command]?.(stageOfVideo(video), { ...asked, command }) ?? false;
}

/**
 * The attribute that marks the player's own controls for its video, the full-screen overlay
 * (player-overlay.tsx) and the bar under the film (page-controls.tsx), each marked for its video by
 * markPlayersControls: Space on any of them but a button plays and pauses (usePlayersKeys).
 */
export const playerControlsAttribute = "data-player-controls";

/**
 * Whether a key of the player's own (playersKeyOf) is the player's while focus is where it is: on the
 * film's picture (the video, whose browser controls, in its shadow, are focus on it, or a castee's
 * stage), on nothing, or on one of the player's own controls for it; never in a text field, nor in a
 * menu the overlay opened (⁝'s switches, D407 §8), nor on anything outside the player.
 */
function isKeyThePlayers(picture: HTMLElement): boolean {
  const focused = document.activeElement;
  if (focused === null || focused === document.body || focused === picture) {
    return true;
  }
  return (
    caretOf(focused) === undefined &&
    focused.closest("dialog") === null &&
    isInThePlayersControls(focused, picture)
  );
}

/**
 * Whether Space is the player's while focus is where it is (isKeyThePlayers): never on a button,
 * which Space presses (D407 §5), so of the player's own controls its timeline alone; nor on another
 * control the page draws over the film, as Up next.
 */
function isSpaceThePlayers(picture: HTMLElement): boolean {
  const focused = document.activeElement;
  const isOnAButton = focused?.matches('button, [role="button"]') === true;
  return !isOnAButton && isKeyThePlayers(picture);
}

/**
 * The keys the player hears itself, which name no command (input-buttons.ts): Space, F and M, each
 * without Ctrl, Alt or Cmd.
 */
type PlayersKey = "space" | "fullScreen" | "mute";

/** The player's own key event is (PlayersKey), or undefined for any other. */
function playersKeyOf(event: KeyboardEvent): PlayersKey | undefined {
  if (event.ctrlKey || event.altKey || event.metaKey) {
    return undefined;
  }
  if (event.key === " ") {
    return "space";
  }
  if (event.key === "f" || event.key === "F") {
    return "fullScreen";
  }
  return event.key === "m" || event.key === "M" ? "mute" : undefined;
}

/**
 * Answers the player's own key for stage, where it is the player's; whether it is taken. Space plays
 * and pauses a stage that is ready, a held one once; F puts the film's stage full screen or leaves it,
 * as the bar's full screen does (toggleFullScreen), and is not taken over a castee's picture, which
 * is in none; M mutes or unmutes a stage that has sound, neither
 * repeating while held.
 */
function answerPlayersKey(stage: PlayerStage, key: PlayersKey, isRepeat: boolean): boolean {
  if (key === "space") {
    const isTaken = takesPlayPause(stage) && isSpaceThePlayers(stage.picture);
    if (isTaken && !isRepeat) {
      stage.playPause();
    }
    return isTaken;
  }
  if (isRepeat || !isKeyThePlayers(stage.picture)) {
    return false;
  }
  if (key === "mute") {
    return stage.toggleMute();
  }
  // A castee's picture is in no film's stage: casting is never full screen (the owner, 2026-11-26).
  const filmStage = stage.picture.closest(".film-player__stage");
  if (filmStage === null) {
    return false;
  }
  toggleFullScreen(filmStage, document);
  return true;
}

/**
 * Answers the player's own keys (PlayersKey) for the stage stageNow reads as each comes, its keyup's
 * own handling prevented with it: Space as YouTube does, as Chrome pressed the browser's full-screen
 * button that a click had left focus on, and a cast's receiver, whose video has no browser controls,
 * did nothing (the owner, 2026-10-27), and with nothing focused still plays; on one of the player's
 * buttons Space presses it, as OK does (D407 §5). Heard before what has focus, after the receiver's
 * first press that turns the sound on (cast-receiver.tsx), which takes it first.
 */
function usePlayersKeys(stageNow: () => PlayerStage | undefined): void {
  const stageNowRef = useLatest(stageNow);
  useEffect(() => {
    let taken: string | undefined;
    const down = (event: KeyboardEvent): void => {
      const stage = stageNowRef.current();
      const key = playersKeyOf(event);
      const isTaken =
        stage !== undefined &&
        key !== undefined &&
        !event.defaultPrevented &&
        answerPlayersKey(stage, key, event.repeat);
      taken = isTaken ? event.key : undefined;
      if (isTaken) {
        event.preventDefault();
      }
    };
    const up = (event: KeyboardEvent): void => {
      // A browser presses a focused button at Space's keyup: taken too, as its keydown was.
      if (taken !== undefined && event.key === taken) {
        event.preventDefault();
        taken = undefined;
      }
    };
    document.addEventListener("keydown", down, true);
    document.addEventListener("keyup", up, true);
    return () => {
      document.removeEventListener("keydown", down, true);
      document.removeEventListener("keyup", up, true);
    };
  }, [stageNowRef]);
}

/**
 * Keeps focus on video, so Space is the player's: a press on the browser's own controls leaves focus
 * on that button, inside the video, where Chrome presses it at Space before any page hears the key
 * (the owner, 2026-10-27: the full-screen button left full screen). Focus goes back to the video
 * itself after each, and after full screen changes.
 */
function useFocusStaysOnTheVideo(video: HTMLVideoElement | null): void {
  useEffect(() => {
    if (video === null) {
      return undefined;
    }
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
    video.addEventListener("click", takeFocusBack);
    document.addEventListener("fullscreenchange", onFullscreenChange);
    return () => {
      video.removeEventListener("click", takeFocusBack);
      document.removeEventListener("fullscreenchange", onFullscreenChange);
    };
  }, [video]);
}

/**
 * Answers the commands and the player's own keys (usePlayersKeys) for the stage stageNow reads as
 * each comes (PlayerStage), the video here or a castee (D555 §8): one answer whatever the target, as
 * the overlay's presses are (useStageNow, overlay-stage.ts); none while there is no stage.
 */
export function useStageCommands(stageNow: () => PlayerStage | undefined): void {
  usePlayersKeys(stageNow);
  useCommands("document", (detail, event) => {
    const stage = stageNow();
    if (stage !== undefined && answers[detail.command]?.(stage, detail) === true) {
      event.preventDefault();
    }
  });
}

/**
 * Answers the commands for video while it is shown (useStageCommands), the browser's media controls'
 * play, pause and seeking too, and their previous and next track where tracks has them, and Space, F
 * and M wherever focus is in the player. The media controls' last play or pause is kept for the
 * video's reports to say (keepMediaActionsOf). Its play and pause and its seeks are taken as far as
 * rights give them, a Watch Together guest's keys doing what its buttons do (D557 §6).
 */
export function usePlayerCommands(
  video: HTMLVideoElement | null,
  tracks?: MediaTracks,
  rights: CommandRights = everyRight,
): void {
  const stageNow = useCallback(
    () => (video === null ? undefined : stageOfVideo(video, rights)),
    [video, rights],
  );
  useStageCommands(stageNow);
  useFocusStaysOnTheVideo(video);
  useMediaSeeks(tracks);
  const hear = useMemo(
    () =>
      video === null
        ? undefined
        : (dispatch: (command: Command, source: CommandSource) => boolean) =>
            hearMediaPlayPause(dispatch, {
              showsPlay: () => isPausedAsAsked(video),
              nowMs: monotonicMs,
            }),
    [video],
  );
  const lastMediaAction = useMediaPlayPause(hear);
  useEffect(
    () => (video === null ? undefined : keepMediaActionsOf(video, lastMediaAction)),
    [video, lastMediaAction],
  );
}

/** The film the browser's media controls' play and pause act on, as far as they ask of it. */
export interface MediaPlayed {
  /** Whether the film shows Play now: paused as its person asked (isPausedAsAsked, film-waits.tsx). */
  readonly showsPlay: () => boolean;
  /** The clock an action's time is read by, for whether it caused the pause or play after it. */
  readonly nowMs: () => number;
}

/**
 * Offers the browser's media controls' play and pause (a lock screen's, a keyboard's or a headset's
 * media keys, the desktop's controls, an iPhone's own full-screen player) as the play/pause command,
 * the one a remote's key and the player's own control give (D268 §4): play while the film shows Play,
 * pause while it shows Pause, so an action that asks for what already is asks nothing. On a Watch
 * Together host's player its play and pause are then the group's as its skinned control's are, the
 * film moved by the player and the session following it (D371 §1). Registered here alone, with the
 * last action remembered (watchMediaSession in @orpheum/player), so a pause a media key made says
 * so. Returns the action of the last mediaSessionActionLastsMs, and the stopping.
 */
export function hearMediaPlayPause(
  dispatch: (command: Command, source: CommandSource) => boolean,
  played: MediaPlayed,
): { readonly lastAction: () => MediaSessionAction | undefined; readonly stop: () => void } {
  const askIf = (isAsked: boolean): void => {
    if (isAsked) {
      dispatch("playPause", "media");
    }
  };
  return watchMediaSession(
    "mediaSession" in navigator ? navigator.mediaSession : undefined,
    {
      play: () => {
        askIf(played.showsPlay());
        return Promise.resolve();
      },
      pause: () => {
        askIf(!played.showsPlay());
      },
    },
    played.nowMs,
  );
}
