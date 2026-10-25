// SPDX-License-Identifier: MPL-2.0

/**
 * The input probe's key log (plans/tv-device-tests.md; controller-input-2026-10-25.md §1.1): what
 * each key a remote, a controller or a keyboard sends says of itself, and which of them are a
 * platform's Back.
 */

import { isBackKey, keyCodeOf } from "./input-buttons.ts";

/** How many keys the probe shows and sends, newest first: enough to read one walk across the grid. */
export const keysKept = 30;

/** One key as the page heard it. */
export interface KeyEntry {
  readonly type: string;
  readonly key: string;
  readonly code: string;
  readonly keyCode: number;
  readonly isRepeat: boolean;
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
    timeStamp: atMs,
  };
}

/** The log with entry added at its head, keeping keysKept. */
export function withKey(log: readonly KeyEntry[], entry: KeyEntry): KeyEntry[] {
  return [entry, ...log].slice(0, keysKept);
}
