// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { keyEntryOf, keysKept, withKey, type HeardKey } from "./lab-keys.ts";

const arrow: HeardKey = {
  type: "keydown",
  key: "ArrowDown",
  code: "ArrowDown",
  keyCode: 40,
  isRepeat: false,
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

  it("keeps the newest first, and only so many", () => {
    let log = withKey([], keyEntryOf(arrow, undefined));
    for (let index = 0; index < keysKept + 5; index += 1) {
      log = withKey(log, keyEntryOf({ ...arrow, key: `k${String(index)}` }, undefined));
    }
    expect(log).toHaveLength(keysKept);
    expect(log[0]?.key).toBe(`k${String(keysKept + 4)}`);
  });
});
