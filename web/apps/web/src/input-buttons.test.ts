// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { buttonOf, directionOfKey, type ButtonName, type HeardInput } from "./input-buttons.ts";

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
});
