// SPDX-License-Identifier: MPL-2.0

/**
 * What a press does to the player's full-screen overlay, and when the overlay hides by itself (D407
 * §4–§6; plans/player-overlay.md §2.2, §2.4, §2.6): pure, so each transition has its own test.
 */

import type { Command } from "./input-buttons.ts";
import { seekStepMs } from "./seek-ahead.ts";

/**
 * What of the overlay shows: nothing; an offer alone (Up next, Still watching) with focus on it; the
 * scrub bar alone after a seek; a pause sign and the scrub bar after OK paused; or all three rows.
 */
export type OverlayShown = "hidden" | "convenience" | "scrub" | "paused" | "full";

/**
 * What showed the whole overlay: a remote's press, after which it hides on its time whatever has
 * focus; or a pointer, a tap or a computer's key, after which focus inside it holds it too.
 */
export type ShownBy = "remote" | "pointer";

export interface OverlayState {
  readonly shown: OverlayShown;
  readonly shownBy: ShownBy;
}

/** What the overlay knows as a press comes. */
export interface PressContext {
  readonly isPaused: boolean;
  /** Whether a remote drives this device: a television's (tv-device.ts). */
  readonly isTelevision: boolean;
  /** Whether focus is on the scrub bar. */
  readonly isOnScrub: boolean;
  /** Whether focus is in the overlay's bottom row, its buttons. */
  readonly isOnButtons: boolean;
  /** Whether focus is on play's button. */
  readonly isOnPlay: boolean;
  /** Whether focus is on an offer drawn over the film beside the overlay, Up next or Still watching. */
  readonly isOnOffer: boolean;
}

/** What a press does to the overlay, and whether the overlay takes it from everything else. */
export interface OverlayAnswer {
  readonly state: OverlayState;
  readonly isTaken: boolean;
  /** A seek the press asks for, in ms, back where negative, made as the presses rest (seek-ahead.ts). */
  readonly seekByMs?: number;
  /** Whether the overlay plays or pauses the film itself, the press taken from the video. */
  readonly isPlayPause?: boolean;
  /** Where focus goes: to play, as the whole overlay opens; to the video, leaving it (Q14). */
  readonly focus?: "play" | "video";
}

/**
 * How long the whole overlay and the pause sign stay after the last press, in ms: a TV player's
 * usual few seconds (D407 §6). A justified constant.
 */
export const fullShownForMs = 3000;

/**
 * How long the scrub bar alone stays after its seek lands, in ms (the owner's Q1): long enough to
 * read where the film went. A justified constant.
 */
export const scrubShownForMs = 1500;

/** What showed the whole overlay, by a command: a remote's only on a television. */
function byCommand(context: PressContext): ShownBy {
  return context.isTelevision ? "remote" : "pointer";
}

/** The seek an arrow asks for, in ms, or undefined for any other command. */
function seekOfArrow(command: Command): number | undefined {
  if (command === "left") {
    return -seekStepMs;
  }
  return command === "right" ? seekStepMs : undefined;
}

/** The commands that stay the player's whatever the overlay shows: play, pause and the seeking keys. */
const playersKeys: ReadonlySet<Command> = new Set<Command>([
  "playPause",
  "seekBack",
  "seekForward",
]);

/**
 * What a press does to the overlay (D407 §4–§5). Play, pause and the seeking keys stay the
 * player's in every state, a seek showing the scrub bar while the overlay is not all shown.
 */
export function overlayAnswer(
  state: OverlayState,
  command: Command,
  context: PressContext,
): OverlayAnswer {
  if (playersKeys.has(command)) {
    return answerOfPlayersKey(state, command, context);
  }
  const shown = state.shown === "hidden" && context.isOnOffer ? "convenience" : state.shown;
  switch (shown) {
    case "full":
      return answerWhileFull(state, command, context);
    case "paused":
      return answerWhilePaused(state, command, context);
    case "convenience":
      return answerWithAnOffer(state, command, context);
    case "hidden":
    case "scrub":
      return answerWhileHidden(state, command, context);
  }
}

/** Play, pause and the seeking keys: the player's, never taken. */
function answerOfPlayersKey(
  state: OverlayState,
  command: Command,
  context: PressContext,
): OverlayAnswer {
  if (state.shown === "full") {
    return { state, isTaken: false };
  }
  if (command === "playPause") {
    return { state: { ...state, shown: context.isPaused ? "hidden" : "paused" }, isTaken: false };
  }
  return { state: { ...state, shown: "scrub" }, isTaken: false };
}

/** The whole overlay, from a press: focus to play. */
function openedFull(context: PressContext): OverlayAnswer {
  return { state: { shown: "full", shownBy: byCommand(context) }, isTaken: true, focus: "play" };
}

/**
 * Hidden, or the scrub bar alone (D407 §4): Left and Right seek and show the scrub bar; OK plays, or
 * pauses and shows the pause sign; Up and Down open all of it; Back hides the scrub bar, and while
 * nothing shows it is the page's.
 */
function answerWhileHidden(
  state: OverlayState,
  command: Command,
  context: PressContext,
): OverlayAnswer {
  const seek = seekOfArrow(command);
  if (seek !== undefined) {
    return { state: { ...state, shown: "scrub" }, isTaken: true, seekByMs: seek };
  }
  if (command === "select") {
    const shown = context.isPaused ? "hidden" : "paused";
    return { state: { ...state, shown }, isTaken: true, isPlayPause: true };
  }
  if (command === "up" || command === "down") {
    return openedFull(context);
  }
  return {
    state: { ...state, shown: "hidden" },
    isTaken: state.shown === "scrub" && command === "back",
  };
}

/**
 * An offer alone, focus on it (D407 §4): OK and Back are the offer's; Left and Right still seek and
 * show the scrub bar, the offer staying (Q3); Up and Down open all of the overlay.
 */
function answerWithAnOffer(
  state: OverlayState,
  command: Command,
  context: PressContext,
): OverlayAnswer {
  const seek = seekOfArrow(command);
  if (seek !== undefined) {
    return { state: { ...state, shown: "scrub" }, isTaken: true, seekByMs: seek };
  }
  if (command === "up" || command === "down") {
    return openedFull(context);
  }
  return { state: { ...state, shown: "hidden" }, isTaken: false };
}

/**
 * The pause sign and the scrub bar, after OK paused: OK plays and hides them; Left and Right seek,
 * the sign staying; Up and Down open all of the overlay; Back hides them, the film still paused.
 */
function answerWhilePaused(
  state: OverlayState,
  command: Command,
  context: PressContext,
): OverlayAnswer {
  const seek = seekOfArrow(command);
  if (seek !== undefined) {
    return { state, isTaken: true, seekByMs: seek };
  }
  if (command === "select") {
    return { state: { ...state, shown: "hidden" }, isTaken: true, isPlayPause: true };
  }
  if (command === "up" || command === "down") {
    return openedFull(context);
  }
  return { state: { ...state, shown: "hidden" }, isTaken: command === "back" };
}

/**
 * All of the overlay (D407 §5): Back hides it; Left and Right seek on the scrub bar and walk a row
 * everywhere else, never seeking (plan item 17); Up and Down are focus's, between the rows; OK is
 * the focused control's. On a computer Down from the buttons, and OK on play that plays, put focus
 * on the video, so the overlay may hide (Q14).
 */
function answerWhileFull(
  state: OverlayState,
  command: Command,
  context: PressContext,
): OverlayAnswer {
  if (command === "back") {
    return { state: { ...state, shown: "hidden" }, isTaken: true };
  }
  const seek = seekOfArrow(command);
  if (seek !== undefined && context.isOnScrub) {
    return { state, isTaken: true, seekByMs: seek };
  }
  if (context.isTelevision) {
    return { state, isTaken: false };
  }
  if (command === "down" && context.isOnButtons) {
    return { state, isTaken: true, focus: "video" };
  }
  const isPlaying = command === "select" && context.isOnPlay && context.isPaused;
  return isPlaying ? { state, isTaken: false, focus: "video" } : { state, isTaken: false };
}

/** What holds the whole overlay or lets it hide. */
export interface HidingContext {
  readonly isPaused: boolean;
  /** Whether one of the overlay's menus is open. */
  readonly isMenuOpen: boolean;
  /** Whether focus is in the overlay. */
  readonly isFocusIn: boolean;
}

/**
 * How long after the last press the overlay hides, in ms, or undefined where it stays (D407 §6;
 * plans/player-overlay.md §2.4): the whole overlay never while the film is paused or a menu is open,
 * nor while focus is in it where a pointer showed it; the scrub bar alone after scrubShownForMs.
 */
export function hideAfter(state: OverlayState, context: HidingContext): number | undefined {
  switch (state.shown) {
    case "scrub":
      return scrubShownForMs;
    case "paused":
      return fullShownForMs;
    case "full": {
      const isHeld =
        context.isPaused ||
        context.isMenuOpen ||
        (state.shownBy === "pointer" && context.isFocusIn);
      return isHeld ? undefined : fullShownForMs;
    }
    case "hidden":
    case "convenience":
      return undefined;
  }
}

/** A player control the overlay may draw (D407 §1). */
export type OverlayControl =
  | "up"
  | "previous"
  | "seekBack"
  | "play"
  | "seekForward"
  | "next"
  | "sound"
  | "subtitles"
  | "audio"
  | "more"
  | "fullScreen";

/** What decides which controls show. */
export interface ControlsContext {
  readonly isTelevision: boolean;
  readonly isTouch: boolean;
  readonly isVolumeSettable: boolean;
  readonly audioTrackCount: number;
  readonly hasPrevious: boolean;
  readonly hasNext: boolean;
  /** Whether the page is a cast's receiver, which has no way up. */
  readonly isReceiver: boolean;
}

/**
 * Which controls the overlay shows (D407 §2; plans/player-overlay.md §2.6): previous and next only
 * within a sequence; the sound not on a television or a touch screen, and only where the page can
 * set the volume; the audio tracks only where there are more than one; Up everywhere but a cast's
 * receiver; the rest always.
 */
export function controlsShown(context: ControlsContext): readonly OverlayControl[] {
  const shows: Record<OverlayControl, boolean> = {
    up: !context.isReceiver,
    previous: context.hasPrevious,
    seekBack: true,
    play: true,
    seekForward: true,
    next: context.hasNext,
    sound: !context.isTelevision && !context.isTouch && context.isVolumeSettable,
    subtitles: true,
    audio: context.audioTrackCount > 1,
    more: true,
    fullScreen: true,
  };
  return overlayControlsInOrder.filter((control) => shows[control]);
}

/** Every control the overlay may draw, in the order it draws them (D407 §1). */
const overlayControlsInOrder: readonly OverlayControl[] = [
  "up",
  "previous",
  "seekBack",
  "play",
  "seekForward",
  "next",
  "sound",
  "subtitles",
  "audio",
  "more",
  "fullScreen",
];
