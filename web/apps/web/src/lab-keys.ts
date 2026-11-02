// SPDX-License-Identifier: MPL-2.0

/**
 * The input probe's key log (plans/tv-device-tests.md; controller-input-2026-10-25.md §1.1): what
 * each key a remote, a controller or a keyboard sends says of itself, its modifiers, place and
 * composing among them (D407 §10), and which of them are a platform's Back.
 */

import { isBackKey, keyCodeOf } from "./input-buttons.ts";
import type { LabInputEvent } from "./lab-events.ts";

/** How many keys the probe shows and sends, newest first: enough to read one walk across the grid. */
export const keysKept = 30;

/**
 * A key's modifiers, where it sits on the keyboard and whether an input method was composing: what
 * the volume keys' rule reads on each layout (D407 §10).
 */
export interface KeyFlags {
  readonly isShiftHeld: boolean;
  readonly isCtrlHeld: boolean;
  readonly isAltHeld: boolean;
  readonly isMetaHeld: boolean;
  /** getModifierState("AltGraph"): AltGr, which Windows also reports as Ctrl with Alt. */
  readonly isAltGraphHeld: boolean;
  /** KeyboardEvent.location: 0 the standard place, 1 left, 2 right, 3 the number pad. */
  readonly location: number;
  readonly isComposing: boolean;
}

/** One key as the page heard it. */
export interface KeyEntry {
  readonly type: string;
  readonly key: string;
  readonly code: string;
  readonly keyCode: number;
  readonly isRepeat: boolean;
  readonly flags: KeyFlags;
  /** Since the key before it, in milliseconds; undefined for the first. */
  readonly sincePreviousMs: number | undefined;
  readonly isBack: boolean;
}

/** What the log reads of a key event. */
export interface HeardKey {
  readonly type: string;
  readonly key: string;
  readonly code: string;
  readonly keyCode: number;
  readonly isRepeat: boolean;
  readonly flags: KeyFlags;
  readonly timeStamp: number;
}

/** The entry of a key heard at its timeStamp, the one before at previousAtMs. */
export function keyEntryOf(heard: HeardKey, previousAtMs: number | undefined): KeyEntry {
  return {
    type: heard.type,
    key: heard.key,
    code: heard.code,
    keyCode: heard.keyCode,
    isRepeat: heard.isRepeat,
    flags: heard.flags,
    sincePreviousMs:
      previousAtMs === undefined ? undefined : Math.round(heard.timeStamp - previousAtMs),
    isBack: isBackKey(heard.key, heard.keyCode),
  };
}

/** What the key log reads of a key event, heard at atMs on the page's clock. */
export function heardKeyOf(event: KeyboardEvent, atMs: number): HeardKey {
  return {
    type: event.type,
    key: event.key,
    code: event.code,
    keyCode: keyCodeOf(event),
    isRepeat: event.repeat,
    flags: {
      isShiftHeld: event.shiftKey,
      isCtrlHeld: event.ctrlKey,
      isAltHeld: event.altKey,
      isMetaHeld: event.metaKey,
      isAltGraphHeld: event.getModifierState("AltGraph"),
      location: event.location,
      isComposing: event.isComposing,
    },
    timeStamp: atMs,
  };
}

/** The log with entry added at its head, keeping keysKept. */
export function withKey(log: readonly KeyEntry[], entry: KeyEntry): KeyEntry[] {
  return [entry, ...log].slice(0, keysKept);
}

/** A key's flags as the server's log takes them. */
export type FlagFields = Pick<
  LabInputEvent,
  "shiftKey" | "ctrlKey" | "altKey" | "metaKey" | "altGraph" | "location" | "isComposing"
>;

/**
 * A key's flags as its event sends them: only those set, and its place only off the standard one,
 * so a plain key's line stays as short as it was.
 */
export function flagFieldsOf(flags: KeyFlags): FlagFields {
  return {
    ...(flags.isShiftHeld ? { shiftKey: true } : {}),
    ...(flags.isCtrlHeld ? { ctrlKey: true } : {}),
    ...(flags.isAltHeld ? { altKey: true } : {}),
    ...(flags.isMetaHeld ? { metaKey: true } : {}),
    ...(flags.isAltGraphHeld ? { altGraph: true } : {}),
    ...(flags.location === 0 ? {} : { location: flags.location }),
    ...(flags.isComposing ? { isComposing: true } : {}),
  };
}

/** The modifiers held, as the key table shows them: "Ctrl+Alt+Shift", or nothing. */
export function modifiersText(flags: KeyFlags): string {
  const held: readonly (readonly [boolean, string])[] = [
    [flags.isCtrlHeld, "Ctrl"],
    [flags.isAltHeld, "Alt"],
    [flags.isAltGraphHeld, "AltGr"],
    [flags.isMetaHeld, "Meta"],
    [flags.isShiftHeld, "Shift"],
  ];
  return held
    .filter(([isHeld]) => isHeld)
    .map(([, name]) => name)
    .join("+");
}
