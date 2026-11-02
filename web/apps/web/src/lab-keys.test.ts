// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  flagFieldsOf,
  keyEntryOf,
  keysKept,
  modifiersText,
  withKey,
  type HeardKey,
  type KeyFlags,
} from "./lab-keys.ts";

/** A key with nothing held, in the standard place, no input method composing. */
const plain: KeyFlags = {
  isShiftHeld: false,
  isCtrlHeld: false,
  isAltHeld: false,
  isMetaHeld: false,
  isAltGraphHeld: false,
  location: 0,
  isComposing: false,
};

const arrow: HeardKey = {
  type: "keydown",
  key: "ArrowDown",
  code: "ArrowDown",
  keyCode: 40,
  isRepeat: false,
  flags: plain,
  timeStamp: 1500,
};

describe("the input probe's key log", () => {
  it("keeps what a key says of itself and how long after the one before it came", () => {
    expect(keyEntryOf(arrow, 1200)).toEqual({
      type: "keydown",
      key: "ArrowDown",
      code: "ArrowDown",
      keyCode: 40,
      isRepeat: false,
      flags: plain,
      sincePreviousMs: 300,
      isBack: false,
    });
    expect(keyEntryOf(arrow, undefined).sincePreviousMs).toBeUndefined();
  });

  it("knows each platform's Back, by name or by keyCode", () => {
    const back = (key: string, keyCode: number): boolean =>
      keyEntryOf({ ...arrow, key, keyCode }, undefined).isBack;
    expect(back("BrowserBack", 166)).toBe(true);
    expect(back("Backspace", 8)).toBe(true);
    expect(back("Unidentified", 4)).toBe(true);
    expect(back("Unidentified", 461)).toBe(true);
    expect(back("XF86Back", 10_009)).toBe(true);
    expect(back("Escape", 27)).toBe(false);
  });

  it("sends only the flags a key has set, and its place only off the standard one", () => {
    // Swiss German's +, Shift with 1: the volume keys' rule reads it by its Shift (D407 §10).
    expect(flagFieldsOf({ ...plain, isShiftHeld: true })).toEqual({ shiftKey: true });
    expect(flagFieldsOf(plain)).toEqual({});
    expect(
      flagFieldsOf({
        isShiftHeld: true,
        isCtrlHeld: true,
        isAltHeld: true,
        isMetaHeld: true,
        isAltGraphHeld: true,
        location: 3,
        isComposing: true,
      }),
    ).toEqual({
      shiftKey: true,
      ctrlKey: true,
      altKey: true,
      metaKey: true,
      altGraph: true,
      location: 3,
      isComposing: true,
    });
  });

  it("names the modifiers held, AltGr apart from Ctrl and Alt", () => {
    expect(modifiersText(plain)).toBe("");
    expect(modifiersText({ ...plain, isAltGraphHeld: true, isShiftHeld: true })).toBe(
      "AltGr+Shift",
    );
    expect(modifiersText({ ...plain, isCtrlHeld: true, isAltHeld: true, isMetaHeld: true })).toBe(
      "Ctrl+Alt+Meta",
    );
  });

  it("keeps the newest first, and only so many", () => {
    let log = withKey([], keyEntryOf(arrow, undefined));
    for (let index = 0; index < keysKept + 5; index += 1) {
      log = withKey(log, keyEntryOf({ ...arrow, key: `k${String(index)}` }, undefined));
    }
    expect(log).toHaveLength(keysKept);
    expect(log[0]?.key).toBe(`k${String(keysKept + 4)}`);
  });
});
