// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  buttonOf,
  commandOf,
  directionOfKey,
  isDirection,
  type ButtonName,
  type Command,
  type HeardInput,
} from "./input-buttons.ts";

/** A standard pad's button, on a layout whose OK is okButton. */
function pad(button: number, okButton: "bottom" | "right" = "bottom"): HeardInput {
  return { kind: "pad", button, isStandard: true, okButton };
}

/** A key as a browser names it, with its keyCode. */
function key(name: string, keyCode = 0): HeardInput {
  return { kind: "key", key: name, keyCode };
}

describe("which button a press is", () => {
  it.each<[string, HeardInput, ButtonName]>([
    ["an arrow by its name", key("ArrowUp", 38), "up"],
    ["an arrow by keyCode alone", key("Unidentified", 39), "right"],
    ["Enter, as a keyboard sends it", key("Enter", 13), "select"],
    ["OK on a Fire TV remote, NumpadEnter's key", key("Enter", 13), "select"],
    ["Enter by keyCode alone", key("Unidentified", 13), "select"],
    ["Fire TV's play/pause, keyCode 179", key("Unidentified", 179), "playPause"],
    ["play/pause by its name", key("MediaPlayPause", 179), "playPause"],
    ["Fire TV's rewind, keyCode 227", key("Unidentified", 227), "rewind"],
    ["Fire TV's fast forward, keyCode 228", key("Unidentified", 228), "fastForward"],
    ["Tizen's play/pause, keyCode 10252", key("Unidentified", 10_252), "playPause"],
    ["Backspace as Back", key("Backspace", 8), "back"],
    ["BrowserBack", key("BrowserBack", 166), "back"],
    ["Android's Back, keyCode 4", key("Unidentified", 4), "back"],
    ["webOS's Back, keyCode 461", key("Unidentified", 461), "back"],
    ["Tizen's Back, keyCode 10009", key("XF86Back", 10_009), "back"],
    ["a volume key", key("AudioVolumeUp", 175), "volumeUp"],
    ["any other key", key("F5", 116), "other"],
    ["a step back in the page's history", { kind: "historyStep" }, "back"],
    ["an Xbox pad's bottom button (A)", pad(0), "select"],
    ["an Xbox pad's right button (B)", pad(1), "back"],
    ["a Nintendo pad's right button (A)", pad(1, "right"), "select"],
    ["a Nintendo pad's bottom button (B)", pad(0, "right"), "back"],
    ["a controller's D-pad left", pad(14), "left"],
    ["a controller's other button", pad(3), "other"],
    [
      "a button of a pad with no standard mapping",
      { kind: "pad", button: 14, isStandard: false, okButton: "bottom" },
      "other",
    ],
    ["a stick leaning up", { kind: "stick", direction: "up" }, "up"],
    [
      "Backspace in a text field, which deletes",
      { kind: "key", key: "Backspace", keyCode: 8, isInField: true },
      "other",
    ],
    [
      "Back's keyCode in a text field",
      { kind: "key", key: "Unidentified", keyCode: 4, isInField: true },
      "back",
    ],
    ["Media Session's pause", { kind: "media", action: "pause" }, "playPause"],
    ["Media Session's seek forward", { kind: "media", action: "seekforward" }, "fastForward"],
    ["Media Session's next track", { kind: "media", action: "nexttrack" }, "other"],
  ])("names %s", (_, input, button) => {
    expect(buttonOf(input)).toBe(button);
  });

  it("names the arrows by key, by an old browser's names, and by keyCode", () => {
    expect(directionOfKey("ArrowUp", 38)).toBe("up");
    expect(directionOfKey("Left", 0)).toBe("left");
    expect(directionOfKey("Unidentified", 40)).toBe("down");
    expect(directionOfKey("Enter", 13)).toBeUndefined();
  });

  it.each<[string, HeardInput, Command | undefined]>([
    ["an arrow", { kind: "key", key: "ArrowLeft", keyCode: 37 }, "left"],
    ["OK", { kind: "key", key: "Enter", keyCode: 13 }, "select"],
    ["Escape, which goes back as Back does", { kind: "key", key: "Escape", keyCode: 27 }, "back"],
    ["webOS's Back", { kind: "key", key: "Unidentified", keyCode: 461 }, "back"],
    ["Fire TV's play/pause", { kind: "key", key: "Unidentified", keyCode: 179 }, "playPause"],
    ["rewind, which seeks back", { kind: "key", key: "MediaRewind", keyCode: 227 }, "seekBack"],
    [
      "fast forward, which seeks on",
      { kind: "key", key: "Unidentified", keyCode: 228 },
      "seekForward",
    ],
    [
      "a controller's Start",
      { kind: "pad", button: 9, isStandard: true, okButton: "bottom" },
      "playPause",
    ],
    [
      "a volume key, which asks nothing of the app",
      { kind: "key", key: "AudioVolumeUp", keyCode: 175 },
      undefined,
    ],
    ["a letter, which is no shortcut", { kind: "key", key: "k", keyCode: 75 }, undefined],
  ])("asks %s as its command", (_, input, command) => {
    expect(commandOf(input)).toBe(command);
  });

  it("knows an arrow's command from the others", () => {
    expect(isDirection("up")).toBe(true);
    expect(isDirection("select")).toBe(false);
  });
});

/** A key with its place on the keyboard and whether Shift was held, as the volume keys read it. */
function typed(name: string, code: string, isShifted = false): HeardInput {
  return { kind: "key", key: name, keyCode: 0, code, isShifted };
}

describe("the volume and track keys (D407 §10)", () => {
  it.each<[string, HeardInput, ButtonName]>([
    ["+ on a US keyboard, Shift with =", typed("+", "Equal", true), "plus"],
    ["the number pad's +", typed("+", "NumpadAdd"), "plus"],
    ["- on a US keyboard", typed("-", "Minus"), "minus"],
    ["the number pad's -", typed("-", "NumpadSubtract"), "minus"],
    ["the media keys' next track", key("MediaTrackNext", 176), "nextTrack"],
    ["the media keys' previous track", key("MediaTrackPrevious", 177), "previousTrack"],
    ["the system's volume up, still its own button", key("AudioVolumeUp", 175), "volumeUp"],
  ])("names %s", (_, input, button) => {
    expect(buttonOf(input)).toBe(button);
  });

  it.each<[string, HeardInput, Command | undefined]>([
    ["+ on a US keyboard, Shift with =", typed("+", "Equal", true), "volumeUp"],
    ["= without Shift, the same key on a US keyboard", typed("=", "Equal"), "volumeUp"],
    ["- without Shift", typed("-", "Minus"), "volumeDown"],
    ["the number pad's +", typed("+", "NumpadAdd"), "volumeUp"],
    ["the number pad's -", typed("-", "NumpadSubtract"), "volumeDown"],
    ["the number pad's + with Shift", typed("+", "NumpadAdd", true), "volumeUp"],
    ["Swiss German's +, Shift with 1", typed("+", "Digit1", true), "volumeUp"],
    ["German's +, a key of its own", typed("+", "BracketRight"), "volumeUp"],
    ["JIS's +, Shift with ;", typed("+", "Semicolon", true), "volumeUp"],
    ["German's =, Shift with 0, which is no volume key", typed("=", "Digit0", true), undefined],
    ["_, Shift with -", typed("_", "Minus", true), undefined],
    ["- with Shift, as a layout may send it", typed("-", "Digit6", true), undefined],
    ["the system's volume up, which the system answers", key("AudioVolumeUp", 175), undefined],
    ["the system's volume down", key("AudioVolumeDown", 174), undefined],
    ["the media keys' next track", key("MediaTrackNext", 176), "next"],
    ["the media keys' previous track", key("MediaTrackPrevious", 177), "previous"],
  ])("asks %s as its command", (_, input, command) => {
    expect(commandOf(input)).toBe(command);
  });
});
