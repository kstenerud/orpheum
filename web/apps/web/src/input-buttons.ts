// SPDX-License-Identifier: MPL-2.0

/**
 * Which button a press is (controller-input-2026-10-25.md §2, §6.1 item 1): a key, a step back in
 * the page's history, a controller's button or stick, or a Media Session action, each named as the
 * remote's button it stands for, so a remote's, a keyboard's and a controller's presses mean the
 * same. Keys are read by their `key` first, then by the platforms' keyCodes an old TV browser alone
 * gives. Proven by the device lab's button test (plans/tv-device-tests.md H2, H8), whose chips are
 * these buttons.
 */

/** Where an arrow moves focus. */
export type Direction = "up" | "down" | "left" | "right";

/** The directions the arrow keys name, by key: the standard names, then old Edge's and IE's. */
const directionsByKey = new Map<string, Direction>([
  ["ArrowUp", "up"],
  ["ArrowDown", "down"],
  ["ArrowLeft", "left"],
  ["ArrowRight", "right"],
  ["Up", "up"],
  ["Down", "down"],
  ["Left", "left"],
  ["Right", "right"],
]);

/** The directions the arrow keys name by keyCode, which an old TV browser alone gives. */
const directionsByKeyCode = new Map<number, Direction>([
  [37, "left"],
  [38, "up"],
  [39, "right"],
  [40, "down"],
]);

/** The direction an arrow key names, by its key or, on an old TV browser, its keyCode. */
export function directionOfKey(key: string, keyCode: number): Direction | undefined {
  return directionsByKey.get(key) ?? directionsByKeyCode.get(keyCode);
}

/** Which face button of a controller confirms: the bottom one, or the right one. */
export type OkButton = "bottom" | "right";

/**
 * The keyCodes a platform's Back is sent as, where it reaches a page at all: Android's KEYCODE_BACK
 * (4), webOS's (461), Tizen's (10009) and Backspace (8), which VIDAA and some remotes send.
 */
export const backKeyCodes: readonly number[] = [4, 8, 461, 10_009];

/** The key names a Back is sent as. */
const backKeys: readonly string[] = ["Backspace", "BrowserBack", "GoBack", "XF86Back"];

/** Whether a key is one of the platforms' Backs. */
export function isBackKey(key: string, keyCode: number): boolean {
  return backKeys.includes(key) || backKeyCodes.includes(keyCode);
}

/**
 * A key event's keyCode: deprecated on the web, and still the only name an old TV browser gives the
 * remote's keys (controller-input-2026-10-25.md §1.1); 0 where it has none.
 */
export function keyCodeOf(event: Event): number {
  const keyCode: unknown = Reflect.get(event, "keyCode");
  return typeof keyCode === "number" ? keyCode : 0;
}

/** A button a press is. */
export type ButtonName =
  | "up"
  | "down"
  | "left"
  | "right"
  | "select"
  | "back"
  | "playPause"
  | "rewind"
  | "fastForward"
  | "volumeUp"
  | "volumeDown"
  | "mute"
  | "plus"
  | "minus"
  | "previousTrack"
  | "nextTrack"
  | "other";

/** One press the page heard, by where it came from. */
export type HeardInput =
  | {
      readonly kind: "key";
      readonly key: string;
      readonly keyCode: number;
      /** Whether a text field had focus: its Backspace deletes, and is no Back. */
      readonly isInField?: boolean;
      /** The key's place on the keyboard (KeyboardEvent.code): the number pad's + and − by it. */
      readonly code?: string;
      /** Whether Shift was held: = with it is + on some layouts, and - with it is _. */
      readonly isShifted?: boolean;
    }
  | { readonly kind: "historyStep" }
  | {
      readonly kind: "pad";
      readonly button: number;
      /** Whether the pad has the standard mapping: another's buttons are only numbers. */
      readonly isStandard: boolean;
      /** Which face button confirms on the pad's layout (input-pads.ts). */
      readonly okButton: OkButton;
    }
  | { readonly kind: "stick"; readonly direction: Direction }
  | { readonly kind: "media"; readonly action: string };

/** The buttons by key name. */
const buttonsByKey = new Map<string, ButtonName>([
  ["Enter", "select"],
  ["Select", "select"],
  ["MediaPlayPause", "playPause"],
  ["MediaPlay", "playPause"],
  ["MediaPause", "playPause"],
  ["MediaRewind", "rewind"],
  ["MediaFastForward", "fastForward"],
  ["AudioVolumeUp", "volumeUp"],
  ["AudioVolumeDown", "volumeDown"],
  ["AudioVolumeMute", "mute"],
  ["MediaTrackPrevious", "previousTrack"],
  ["MediaTrackNext", "nextTrack"],
]);

/**
 * The buttons by keyCode: Enter (13); Fire TV's and Android's play/pause, rewind and fast forward
 * (179, 227, 228); webOS's and Tizen's play, pause, rewind and fast forward (415, 19, 412, 417) and
 * Tizen's play/pause (10252); the volume keys as Windows numbers them (175, 174, 173).
 */
const buttonsByKeyCode = new Map<number, ButtonName>([
  [13, "select"],
  [179, "playPause"],
  [227, "rewind"],
  [228, "fastForward"],
  [415, "playPause"],
  [19, "playPause"],
  [10_252, "playPause"],
  [412, "rewind"],
  [417, "fastForward"],
  [175, "volumeUp"],
  [174, "volumeDown"],
  [173, "mute"],
]);

/**
 * The buttons by a controller's button in the standard mapping (the Gamepad spec's), but its bottom
 * and right face buttons (0 and 1), which confirm or go back by the pad's layout: the triggers rewind
 * and fast forward, Start plays and pauses; the D-pad is 12 to 15.
 */
const buttonsByPadButton = new Map<number, ButtonName>([
  [6, "rewind"],
  [7, "fastForward"],
  [9, "playPause"],
  [12, "up"],
  [13, "down"],
  [14, "left"],
  [15, "right"],
]);

/**
 * Whether a standard pad's button seeks (rewind or fast forward), the buttons that repeat while held
 * as a stick does: a held remote's arrow seeks by the time held, and so must these. No other button
 * repeats, since a held OK or Start must not act twice.
 */
export function isSeekPadButton(button: number): boolean {
  const name = buttonsByPadButton.get(button);
  return name === "rewind" || name === "fastForward";
}

/** The buttons by Media Session action. */
const buttonsByMediaAction = new Map<string, ButtonName>([
  ["play", "playPause"],
  ["pause", "playPause"],
  ["seekbackward", "rewind"],
  ["seekforward", "fastForward"],
]);

/** The face button 0 or 1 is: the bottom one or the right one. */
const faceButtons = new Map<number, OkButton>([
  [0, "bottom"],
  [1, "right"],
]);

/**
 * The button a controller's is: Other for every button of a pad with no standard mapping, whose
 * numbers mean nothing known; a face button confirms where its layout's OK is, else goes back.
 */
function buttonOfPad(button: number, isStandard: boolean, okButton: OkButton): ButtonName {
  if (!isStandard) {
    return "other";
  }
  const face = faceButtons.get(button);
  if (face !== undefined) {
    return face === okButton ? "select" : "back";
  }
  return buttonsByPadButton.get(button) ?? "other";
}

/** A key as buttonOfKey reads it. */
type HeardKey = Extract<HeardInput, { kind: "key" }>;

/**
 * The volume key a key is (D407 §10, player-controls-2026-11-02.md §10.4 option A): plus is + with or
 * without Shift, = without it, or the number pad's +; minus is - without Shift, or the number pad's −.
 * A layout whose + needs AltGr or Ctrl+Alt (Swiss, Hungarian, Turkish Q) is left alone by the input
 * layer's modifier guard, not here; the system's AudioVolume keys are other buttons.
 */
function volumeButtonOf(heard: HeardKey): ButtonName | undefined {
  const isShifted = heard.isShifted ?? false;
  if (heard.code === "NumpadAdd" || heard.key === "+" || (heard.key === "=" && !isShifted)) {
    return "plus";
  }
  if (heard.code === "NumpadSubtract" || (heard.key === "-" && !isShifted)) {
    return "minus";
  }
  return undefined;
}

/**
 * The button a key is: by its name, then its keyCode; Other where neither names one. Backspace in a
 * text field deletes, so it is Other there.
 */
function buttonOfKey(heard: HeardKey): ButtonName {
  const { key, keyCode } = heard;
  const direction = directionOfKey(key, keyCode);
  if (direction !== undefined) {
    return direction;
  }
  const isInField = heard.isInField ?? false;
  if (isBackKey(key, keyCode) && !(isInField && (key === "Backspace" || keyCode === 8))) {
    return "back";
  }
  return volumeButtonOf(heard) ?? buttonsByKey.get(key) ?? buttonsByKeyCode.get(keyCode) ?? "other";
}

/** The button a press the page heard is; Other where it names none the test shows. */
export function buttonOf(input: HeardInput): ButtonName {
  switch (input.kind) {
    case "key":
      return buttonOfKey(input);
    case "historyStep":
      return "back";
    case "pad":
      return buttonOfPad(input.button, input.isStandard, input.okButton);
    case "stick":
      return input.direction;
    case "media":
      return buttonsByMediaAction.get(input.action) ?? "other";
  }
}

/**
 * What a press asks of the app (controller-input-2026-10-25.md §6.1 item 1): pages and the player
 * hear commands, never keys.
 */
export type Command =
  | Direction
  | "select"
  | "back"
  | "playPause"
  | "seekBack"
  | "seekForward"
  | "volumeUp"
  | "volumeDown"
  | "previous"
  | "next";

/**
 * The commands by button: rewind and fast forward seek; + and − change the volume, and the media keys'
 * next and previous track change the video (D407 §10). The system's own volume keys ask nothing: the
 * system answers them.
 */
const commandsByButton = new Map<ButtonName, Command>([
  ["up", "up"],
  ["down", "down"],
  ["left", "left"],
  ["right", "right"],
  ["select", "select"],
  ["back", "back"],
  ["playPause", "playPause"],
  ["rewind", "seekBack"],
  ["fastForward", "seekForward"],
  ["plus", "volumeUp"],
  ["minus", "volumeDown"],
  ["previousTrack", "previous"],
  ["nextTrack", "next"],
]);

/**
 * The command a press asks for: its button's, and Back for Escape, which closes what is open and
 * goes back as a remote's Back does (the owner, 2026-10-25); undefined for a press that asks none.
 */
export function commandOf(input: HeardInput): Command | undefined {
  if (input.kind === "key" && (input.key === "Escape" || input.key === "Esc")) {
    return "back";
  }
  return commandsByButton.get(buttonOf(input));
}

/** Whether command is an arrow's. */
export function isDirection(command: Command): command is Direction {
  return command === "up" || command === "down" || command === "left" || command === "right";
}
