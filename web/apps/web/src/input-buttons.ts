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
  | "other";

/** One press the page heard, by where it came from. */
export type HeardInput =
  | {
      readonly kind: "key";
      readonly key: string;
      readonly keyCode: number;
      /** Whether a text field had focus: its Backspace deletes, and is no Back. */
      readonly isInField?: boolean;
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

/**
 * The button a key is: by its name, then its keyCode; Other where neither names one. Backspace in a
 * text field deletes, so it is Other there.
 */
function buttonOfKey(key: string, keyCode: number, isInField: boolean): ButtonName {
  const direction = directionOfKey(key, keyCode);
  if (direction !== undefined) {
    return direction;
  }
  if (isBackKey(key, keyCode) && !(isInField && (key === "Backspace" || keyCode === 8))) {
    return "back";
  }
  return buttonsByKey.get(key) ?? buttonsByKeyCode.get(keyCode) ?? "other";
}

/** The button a press the page heard is; Other where it names none the test shows. */
export function buttonOf(input: HeardInput): ButtonName {
  switch (input.kind) {
    case "key":
      return buttonOfKey(input.key, input.keyCode, input.isInField ?? false);
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
