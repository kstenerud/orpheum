// SPDX-License-Identifier: MPL-2.0

/**
 * What a press does to the player's full-screen overlay, and when the overlay hides by itself (D407
 * §4–§6; plans/player-overlay.md §2.2, §2.4, §2.6): pure, so each transition has its own test.
 */

import type { Command } from "./input-buttons.ts";
import { seekStepMs } from "./seek-ahead.ts";

/**
 * What of the overlay shows: nothing; the convenience area alone, an offer (Up next, Still watching)
 * in it; the scrub bar alone after a seek; a pause sign and the scrub bar after OK paused; or all
 * three rows.
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
  /** Whether an offer shows in the overlay's convenience area, Up next or Still watching (D407 §3). */
  readonly hasOffer: boolean;
  /** Whether focus is on that offer. */
  readonly isOnOffer: boolean;
  /**
   * Whether focus is outside the overlay, on the video or the page, as a pointer or full screen's
   * start leaves it: Up and Down then bring it in (the owner, 2026-11-03).
   */
  readonly isFocusOutside?: boolean;
  /**
   * Whether one of the overlay's menus is open (💬, 🎧, ⁝; player-menus.tsx): its presses are the
   * menu's, Back closing it before anything else (D407 §6, §8).
   */
  readonly isMenuOpen?: boolean;
}

/** What a press does to the overlay, and whether the overlay takes it from everything else. */
export interface OverlayAnswer {
  readonly state: OverlayState;
  readonly isTaken: boolean;
  /** A seek the press asks for, in ms, back where negative, made as the presses rest (seek-ahead.ts). */
  readonly seekByMs?: number;
  /** Whether the overlay plays or pauses the film itself, the press taken from the video. */
  readonly isPlayPause?: boolean;
  /**
   * Where focus goes: to play, as the whole overlay opens; to the video, leaving it (Q14). A press of
   * play that plays leaves it too, by a click, OK's, Space's or a mouse's (player-overlay.tsx).
   */
  readonly focus?: "play" | "video";
  /** Whether the overlay presses the offer itself, focus being elsewhere (D407 §3). */
  readonly isOfferPressed?: boolean;
  /** Whether the overlay shows the volume a press of + or − left (D407 §10), for volumeShownForMs. */
  readonly isVolumeShown?: boolean;
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

/**
 * How long the volume shows after + or −, in ms: as long as the scrub bar alone after a seek, the
 * other change shown alone for a moment. A justified constant.
 */
export const volumeShownForMs = scrubShownForMs;

/** The volume keys, + and −: the player's (player-commands.ts), the overlay only showing the volume. */
const volumeKeys: ReadonlySet<Command> = new Set<Command>(["volumeUp", "volumeDown"]);

/**
 * What of the overlay shows, hasOffer saying whether an offer is on: hidden, the convenience area
 * alone while one is (D407 §3).
 */
export function shownWith(shown: OverlayShown, hasOffer: boolean): OverlayShown {
  return shown === "hidden" && hasOffer ? "convenience" : shown;
}

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
 * player's in every state, a seek showing the scrub bar while the overlay is not all shown; + and −
 * are the player's too, the overlay showing the volume they leave, whatever else shows (D407 §10).
 */
export function overlayAnswer(
  state: OverlayState,
  command: Command,
  context: PressContext,
): OverlayAnswer {
  if (volumeKeys.has(command)) {
    return { state, isTaken: false, isVolumeShown: true };
  }
  // An open menu's presses are its own (D407 §8).
  return context.isMenuOpen === true
    ? { state, isTaken: false }
    : answerWithNoMenu(state, command, context);
}

/** What a press does to the overlay while none of its menus is open (overlayAnswer). */
function answerWithNoMenu(
  state: OverlayState,
  command: Command,
  context: PressContext,
): OverlayAnswer {
  if (playersKeys.has(command)) {
    return answerOfPlayersKey(state, command, context);
  }
  // The offer stays through a seek's scrub bar (Q3), and OK is still its own.
  const isOfferAlone = state.shown === "hidden" || state.shown === "scrub";
  const shown = isOfferAlone && context.hasOffer ? "convenience" : state.shown;
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
 * An offer alone (D407 §3, §4): OK presses it, with no guard, the focused offer's own press or else
 * the overlay's; Back is the offer's; Left and Right still seek and show the scrub bar, the offer
 * staying (Q3); Up and Down open all of the overlay.
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
  if (command === "select" && !context.isOnOffer) {
    return { state, isTaken: true, isOfferPressed: true };
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
 * everywhere else, never seeking (plan item 17); Up and Down are focus's, between the rows, and bring
 * focus that is outside the overlay to play, as a pointer or full screen's start leaves it; OK is
 * the focused control's. On a computer Down from the buttons puts focus on the video, so the overlay
 * may hide (Q14); play's press that plays does too, as its click (player-overlay.tsx).
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
  if ((command === "up" || command === "down") && context.isFocusOutside === true) {
    return { state, isTaken: true, focus: "play" };
  }
  if (context.isTelevision) {
    return { state, isTaken: false };
  }
  if (command === "down" && context.isOnButtons) {
    return { state, isTaken: true, focus: "video" };
  }
  return { state, isTaken: false };
}

/** What holds the whole overlay or lets it hide. */
export interface HidingContext {
  readonly isPaused: boolean;
  /** Whether one of the overlay's menus is open. */
  readonly isMenuOpen: boolean;
  /** Whether focus a key put there (:focus-visible) is in the overlay (D409). */
  readonly isFocusVisibleIn: boolean;
}

/**
 * How long after the last press the overlay hides, in ms, or undefined where it stays (D407 §6;
 * plans/player-overlay.md §2.4): the whole overlay never while the film is paused or a menu is open,
 * nor while focus a key put there is in it where a pointer showed it (D409); the scrub bar alone after scrubShownForMs.
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
        (state.shownBy === "pointer" && context.isFocusVisibleIn);
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
    // A cast's receiver draws no menus (D407 §12): the remote holds its choices.
    subtitles: !context.isReceiver,
    audio: context.audioTrackCount > 1 && !context.isReceiver,
    more: !context.isReceiver,
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
