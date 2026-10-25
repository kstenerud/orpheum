// SPDX-License-Identifier: MPL-2.0

/**
 * A game controller as the app reads it (controller-input-2026-10-25.md §6.1 item 5): which layout
 * its id says it has, which face button confirms on that layout, each standard button's place on the
 * pad, and its stick's directions with their repeat while held. Proven by the device lab's button
 * test (plans/tv-device-tests.md H8), which reads pads by it too.
 *
 * Under the W3C standard mapping buttons are positional: 0 bottom, 1 right, 2 left, 3 top, 4 and 5
 * the shoulders, 6 and 7 the triggers, 8 select (back, view), 9 start (menu), 10 and 11 the sticks'
 * presses, 12 to 15 the D-pad, 16 the home button. The layouts differ in which face button confirms:
 * Xbox and PlayStation confirm with the bottom one (A, cross) and go back with the right (B, circle);
 * Nintendo confirms with the right one (labelled A) and goes back with the bottom (labelled B).
 */

import type { Direction, OkButton } from "./input-buttons.ts";

/** A layout inferred from a pad's id. */
export type PadLayout = "xbox" | "playstation" | "nintendo" | "unknown";

/** A standard pad's button by its place on the pad. */
export type PadButtonName =
  | "faceBottom"
  | "faceRight"
  | "faceLeft"
  | "faceTop"
  | "shoulderLeft"
  | "shoulderRight"
  | "triggerLeft"
  | "triggerRight"
  | "select"
  | "start"
  | "stickLeft"
  | "stickRight"
  | "dpadUp"
  | "dpadDown"
  | "dpadLeft"
  | "dpadRight"
  | "home";

/** The standard mapping's buttons, by index (the Gamepad spec's "standard" mapping). */
const standardButtons: readonly PadButtonName[] = [
  "faceBottom",
  "faceRight",
  "faceLeft",
  "faceTop",
  "shoulderLeft",
  "shoulderRight",
  "triggerLeft",
  "triggerRight",
  "select",
  "start",
  "stickLeft",
  "stickRight",
  "dpadUp",
  "dpadDown",
  "dpadLeft",
  "dpadRight",
  "home",
];

/** A standard pad's button's place, or undefined past the standard mapping's 17. */
export function padButtonNameOf(button: number): PadButtonName | undefined {
  return standardButtons[button];
}

/**
 * The layouts by USB vendor: Nintendo's (057e), Sony's (054c) and Microsoft's (045e). 8BitDo's own
 * (2dc8) is missing on purpose: its pads report another vendor's by mode (Switch mode as a Nintendo
 * Pro Controller, X-input as Microsoft's, macOS mode as Sony's), so its own says nothing of the
 * labels its buttons are read by.
 */
const layoutsByVendor = new Map<string, PadLayout>([
  ["057e", "nintendo"],
  ["054c", "playstation"],
  ["045e", "xbox"],
]);

/**
 * A pad's vendor, as the browser writes it into the id: Chrome's "(… Vendor: 057e Product: 2009)",
 * Firefox's leading "057e-2009-"; undefined where it gives none (Chrome's XInput pads on Windows,
 * Firefox's "xinput", Android's bare names).
 */
export function padVendorOf(id: string): string | undefined {
  const chrome = /Vendor: (?<vendor>[\da-f]{4})/iu.exec(id)?.groups?.["vendor"];
  const firefox = /^(?<vendor>[\da-f]{4})-[\da-f]{4}-/iu.exec(id)?.groups?.["vendor"];
  return (chrome ?? firefox)?.toLowerCase();
}

/**
 * The layout a name says, where the vendor is not given: Windows' XInput pads are Xbox's by
 * definition; a name naming the maker otherwise. Read after the vendor, so an 8BitDo calling itself
 * a Pro Controller in Switch mode is read by the vendor it reports, which is Nintendo's too.
 */
function layoutOfName(id: string): PadLayout {
  if (/xinput|x-?box/iu.test(id)) {
    return "xbox";
  }
  if (/dualsense|dualshock|playstation/iu.test(id)) {
    return "playstation";
  }
  if (/nintendo|pro controller|joy-con/iu.test(id)) {
    return "nintendo";
  }
  return "unknown";
}

/** The layout a pad's id says: by its vendor where the browser gives it, else by its name. */
export function padLayoutOf(id: string): PadLayout {
  const vendor = padVendorOf(id);
  if (vendor !== undefined) {
    return layoutsByVendor.get(vendor) ?? "unknown";
  }
  return layoutOfName(id);
}

/**
 * Which face button confirms: the person's choice where they made one, whatever the layout, since
 * a pad's labels need not match what it reports (the owner's 8BitDo SN30 Pro, 2026-10-25: in
 * X-input mode it reports an Xbox pad, but its labels are Nintendo's, so its A is on the right);
 * else by layout, the right one on Nintendo's, the bottom one on Xbox's, PlayStation's and an
 * unknown layout, the more common.
 */
export function okButtonOf(layout: PadLayout, chosen: OkButton | undefined): OkButton {
  if (chosen !== undefined) {
    return chosen;
  }
  return layout === "nintendo" ? "right" : "bottom";
}

/** A pad's mapping as the log keeps it: the standard one, none, or another. */
export type PadMapping = "standard" | "none" | "other";

/** The mapping the browser gives a pad, as the log keeps it. */
export function padMappingOf(mapping: string): PadMapping {
  if (mapping === "standard") {
    return "standard";
  }
  return mapping === "" ? "none" : "other";
}

/**
 * How far an axis must lean past its centre to count: half way, the usual dead zone for a stick
 * used as arrows. A justified constant, the stick's own geometry, not a preference.
 */
export const axisLeansPast = 0.5;

/** Where an axis leans: -1 past -axisLeansPast, 1 past axisLeansPast, else 0. */
export function axisLean(value: number): -1 | 0 | 1 {
  if (value > axisLeansPast) {
    return 1;
  }
  return value < -axisLeansPast ? -1 : 0;
}

/**
 * The direction a stick points, from its x and y axes: the axis leaning further, where it leans past
 * axisLeansPast; undefined while the stick is near its centre. Up is a negative y.
 */
export function stickDirectionOf(x: number, y: number): Direction | undefined {
  if (Math.abs(x) >= Math.abs(y)) {
    const lean = axisLean(x);
    if (lean !== 0) {
      return lean > 0 ? "right" : "left";
    }
    return undefined;
  }
  const lean = axisLean(y);
  if (lean === 0) {
    return undefined;
  }
  return lean > 0 ? "down" : "up";
}

/**
 * How long a stick held one way waits before it repeats, then between repeats, in milliseconds: a
 * TV's remote's held arrow, close to what a keyboard's repeat does. Justified constants, the feel of
 * a held arrow, which the button test lets the owner try; a setting once the real UI takes them.
 */
export const stickRepeatFirstMs = 400;
export const stickRepeatEachMs = 100;

/** A stick's repeating: the direction it is held, and when it next moves focus. */
export interface StickHold {
  readonly direction: Direction | undefined;
  readonly nextAtMs: number;
}

/** A stick at its centre. */
export const stickAtRest: StickHold = { direction: undefined, nextAtMs: 0 };

/**
 * The stick's hold after a frame at nowMs pointing direction, and whether it moves focus then: at
 * once when it leans a new way, stickRepeatFirstMs later, then every stickRepeatEachMs while held;
 * never at its centre.
 */
export function stickStep(
  hold: StickHold,
  direction: Direction | undefined,
  nowMs: number,
): { readonly hold: StickHold; readonly shouldMove: boolean } {
  if (direction === undefined) {
    return { hold: stickAtRest, shouldMove: false };
  }
  if (direction !== hold.direction) {
    return { hold: { direction, nextAtMs: nowMs + stickRepeatFirstMs }, shouldMove: true };
  }
  if (nowMs >= hold.nextAtMs) {
    return { hold: { direction, nextAtMs: nowMs + stickRepeatEachMs }, shouldMove: true };
  }
  return { hold, shouldMove: false };
}

/** The D-pad's buttons in the standard mapping, by the direction each points. */
const dpadButtons: readonly (readonly [number, Direction])[] = [
  [12, "up"],
  [13, "down"],
  [14, "left"],
  [15, "right"],
];

/**
 * The direction a standard pad's D-pad is held, from the buttons held: the first of its buttons held,
 * up, down, left, right; undefined while none is.
 */
export function dpadDirectionOf(pressed: readonly number[]): Direction | undefined {
  return dpadButtons.find(([button]) => pressed.includes(button))?.[1];
}
