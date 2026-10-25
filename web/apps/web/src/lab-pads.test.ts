// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  axisLean,
  okButtonOf,
  padButtonNameOf,
  padLayoutOf,
  padMappingOf,
  padVendorOf,
  stickAtRest,
  stickDirectionOf,
  stickRepeatEachMs,
  stickRepeatFirstMs,
  stickStep,
  type PadLayout,
  type StickHold,
} from "./lab-pads.ts";

/**
 * Pads' ids as browsers give them. Verified: quoted from a published capture (Enichan's gist of
 * gamepad ids, gist.github.com/Enichan/de1618d2c481523bd0c1123800bfe747; a Babylon.js forum report
 * for Firefox on Linux). Unverified: built from the browser's documented format (Chromium's
 * "<name> (STANDARD GAMEPAD Vendor: vvvv Product: pppp)", Firefox's "vvvv-pppp-<name>") and the
 * vendor and product a mode reports (denilson.sa.nom.br's 8BitDo SN30 Pro+ cheatsheet), not seen
 * from a browser; the button test's runs (H8) confirm or correct them.
 */
const ids: readonly [string, string, PadLayout][] = [
  // Verified.
  ["Chrome, Windows, an Xbox pad", "Xbox 360 Controller (XInput STANDARD GAMEPAD)", "xbox"],
  [
    "Chrome, a DualShock 4",
    "Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 05c4)",
    "playstation",
  ],
  ["Chrome, a Switch Pro unmapped", "Pro Controller (Vendor: 057e Product: 2009)", "nintendo"],
  [
    "Chrome, an 8BitDo SF30 Pro in D-input",
    "8Bitdo SF30 Pro (Vendor: 2dc8 Product: 6000)",
    "unknown",
  ],
  ["Firefox, Linux, an Xbox 360 pad", "045e-028e-Microsoft X-Box 360 pad", "xbox"],
  // Unverified.
  [
    "Chrome, an Xbox Wireless pad",
    "Xbox Wireless Controller (STANDARD GAMEPAD Vendor: 045e Product: 0b13)",
    "xbox",
  ],
  [
    "Chrome, a DualSense",
    "DualSense Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)",
    "playstation",
  ],
  [
    "Chrome, a Switch Pro",
    "Pro Controller (STANDARD GAMEPAD Vendor: 057e Product: 2009)",
    "nintendo",
  ],
  ["Firefox, Windows, an XInput pad", "xinput", "xbox"],
  ["Firefox, an Xbox Wireless pad", "045e-0b13-Xbox Wireless Controller", "xbox"],
  ["Firefox, a DualSense", "054c-0ce6-DualSense Wireless Controller", "playstation"],
  ["Firefox, a Switch Pro", "057e-2009-Pro Controller", "nintendo"],
  // An 8BitDo SN30 Pro in each of its modes (unverified): Switch, X-input, D-input, macOS.
  [
    "Chrome, an 8BitDo in Switch mode",
    "Pro Controller (STANDARD GAMEPAD Vendor: 057e Product: 2009)",
    "nintendo",
  ],
  ["Firefox, an 8BitDo in Switch mode", "057e-2009-Pro Controller", "nintendo"],
  [
    "Chrome, Windows, an 8BitDo in X-input mode",
    "Xbox 360 Controller (XInput STANDARD GAMEPAD)",
    "xbox",
  ],
  ["Firefox, Linux, an 8BitDo in X-input mode", "045e-028e-Microsoft X-Box 360 pad", "xbox"],
  [
    "Chrome, an 8BitDo in D-input mode",
    "8Bitdo SF30 Pro (STANDARD GAMEPAD Vendor: 2dc8 Product: 6102)",
    "unknown",
  ],
  ["Firefox, an 8BitDo in D-input mode", "2dc8-6102-8Bitdo SF30 Pro", "unknown"],
  [
    "Chrome, an 8BitDo in macOS mode, as a DualShock 4",
    "Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 05c4)",
    "playstation",
  ],
  // Android's ids, a Fire TV's, are a bare name (unverified).
  ["Android, an Xbox pad by its name", "Xbox Wireless Controller", "xbox"],
  ["Android, a pad naming no maker", "Gamepad", "unknown"],
];

describe("a controller's layout", () => {
  it.each(ids)("reads %s as its layout", (_, id, layout) => {
    expect(padLayoutOf(id)).toBe(layout);
  });

  it("reads the vendor Chrome and Firefox write, and none where they write none", () => {
    expect(padVendorOf("Pro Controller (Vendor: 057e Product: 2009)")).toBe("057e");
    expect(padVendorOf("054C-0CE6-DualSense Wireless Controller")).toBe("054c");
    expect(padVendorOf("Xbox 360 Controller (XInput STANDARD GAMEPAD)")).toBeUndefined();
  });

  it("reads a vendor before a name: 8BitDo's own says nothing, whatever it calls itself", () => {
    expect(padLayoutOf("2dc8-6102-8BitDo Pro Controller")).toBe("unknown");
  });

  it("confirms by the person's choice where made, else the right button on Nintendo's and the bottom elsewhere", () => {
    expect(okButtonOf("nintendo", undefined)).toBe("right");
    expect(okButtonOf("nintendo", "bottom")).toBe("bottom");
    expect(okButtonOf("xbox", undefined)).toBe("bottom");
    expect(okButtonOf("xbox", "right")).toBe("right");
    expect(okButtonOf("playstation", undefined)).toBe("bottom");
    expect(okButtonOf("unknown", undefined)).toBe("bottom");
    expect(okButtonOf("unknown", "right")).toBe("right");
  });

  it("names a standard pad's buttons by their place", () => {
    expect(padButtonNameOf(0)).toBe("faceBottom");
    expect(padButtonNameOf(1)).toBe("faceRight");
    expect(padButtonNameOf(3)).toBe("faceTop");
    expect(padButtonNameOf(8)).toBe("select");
    expect(padButtonNameOf(9)).toBe("start");
    expect(padButtonNameOf(15)).toBe("dpadRight");
    expect(padButtonNameOf(16)).toBe("home");
    expect(padButtonNameOf(17)).toBeUndefined();
  });

  it("keeps a mapping as standard, none or other", () => {
    expect(padMappingOf("standard")).toBe("standard");
    expect(padMappingOf("")).toBe("none");
    expect(padMappingOf("xr-standard")).toBe("other");
  });
});

describe("a controller's stick", () => {
  it("leans past half way only", () => {
    expect(axisLean(0.5)).toBe(0);
    expect(axisLean(0.51)).toBe(1);
    expect(axisLean(-0.6)).toBe(-1);
  });

  it("points the way its further axis leans, up being a negative y", () => {
    expect(stickDirectionOf(0.9, 0.2)).toBe("right");
    expect(stickDirectionOf(-0.7, 0.6)).toBe("left");
    expect(stickDirectionOf(0.1, -0.8)).toBe("up");
    expect(stickDirectionOf(0.3, 0.9)).toBe("down");
    expect(stickDirectionOf(0.4, 0.4)).toBeUndefined();
  });

  it("moves at once, again after the first wait, then at each shorter one, and not at rest", () => {
    const moves: number[] = [];
    let hold: StickHold = stickAtRest;
    for (let nowMs = 0; nowMs <= 700; nowMs += 10) {
      const step = stickStep(hold, "down", nowMs);
      hold = step.hold;
      if (step.shouldMove) {
        moves.push(nowMs);
      }
    }
    expect(moves).toEqual([
      0,
      stickRepeatFirstMs,
      stickRepeatFirstMs + stickRepeatEachMs,
      stickRepeatFirstMs + 2 * stickRepeatEachMs,
      stickRepeatFirstMs + 3 * stickRepeatEachMs,
    ]);
    expect(stickStep(hold, undefined, 710)).toEqual({ hold: stickAtRest, shouldMove: false });
    expect(stickStep(hold, "left", 710).shouldMove).toBe(true);
  });
});
