// SPDX-License-Identifier: MPL-2.0

/**
 * The input probe's key log (plans/tv-device-tests.md; controller-input-2026-10-25.md §1.1): what
 * each key a remote, a controller or a keyboard sends says of itself, and which of them are a
 * platform's Back.
 */

import { useEffect, useRef } from "react";

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

/**
 * A key event's keyCode: deprecated on the web, and still the only name an old TV browser gives the
 * remote's keys (controller-input-2026-10-25.md §1.1); 0 where it has none.
 */
export function keyCodeOf(event: Event): number {
  const keyCode: unknown = Reflect.get(event, "keyCode");
  return typeof keyCode === "number" ? keyCode : 0;
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

/**
 * Calls onFirst at the page's first key or press, once: the gesture a browser asks for before a page
 * plays sound, and the playing a browser asks for before it gives the page its media keys (owner,
 * Firefox and Silk, 2026-10-25: no media key did anything while the tone was off).
 */
export function useFirstPress(onFirst: () => void): void {
  const onFirstRef = useRef(onFirst);
  onFirstRef.current = onFirst;
  useEffect(() => {
    const heard = (): void => {
      stop();
      onFirstRef.current();
    };
    const stop = (): void => {
      globalThis.removeEventListener("keydown", heard, true);
      globalThis.removeEventListener("pointerdown", heard, true);
    };
    globalThis.addEventListener("keydown", heard, true);
    globalThis.addEventListener("pointerdown", heard, true);
    return stop;
  }, []);
}
