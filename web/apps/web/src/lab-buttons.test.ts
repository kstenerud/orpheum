// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { buttonOf, chipsShown, type ButtonName, type HeardInput } from "./lab-buttons.ts";

/** A key as a browser names it, with its keyCode. */
function key(name: string, keyCode = 0): HeardInput {
  return { kind: "key", key: name, keyCode };
}

describe("the button test's buttons", () => {
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
    ["a controller's A", { kind: "pad", button: 0 }, "select"],
    ["a controller's B", { kind: "pad", button: 1 }, "back"],
    ["a controller's D-pad left", { kind: "pad", button: 14 }, "left"],
    ["a controller's other button", { kind: "pad", button: 3 }, "other"],
    ["Media Session's pause", { kind: "media", action: "pause" }, "playPause"],
    ["Media Session's seek forward", { kind: "media", action: "seekforward" }, "fastForward"],
    ["Media Session's next track", { kind: "media", action: "nexttrack" }, "other"],
  ])("names %s", (_, input, button) => {
    expect(buttonOf(input)).toBe(button);
  });

  it("shows the volume chips only once heard, before Other", () => {
    expect(chipsShown([])).not.toContain("volumeUp");
    const shown = chipsShown(["up", "volumeDown"]);
    expect(shown).toContain("volumeDown");
    expect(shown).not.toContain("volumeUp");
    expect(shown.at(-1)).toBe("other");
    expect(shown.indexOf("volumeDown")).toBe(shown.length - 2);
  });
});
