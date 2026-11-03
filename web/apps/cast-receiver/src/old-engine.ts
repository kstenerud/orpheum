// SPDX-License-Identifier: MPL-2.0

/**
 * The few methods the receiver's bundle calls that the 3rd-generation Chromecast's Chromium 90 lacks
 * (browser-support-2026-10-09.md; the 11-05 batch's item 10), filled where they are missing and left
 * alone where the engine has them: the build lowers the syntax (vite.config.ts's target), never the
 * methods. A method added to the bundle that this engine lacks is found by the build's scan, not
 * here: the list is the one the 2026-11-03 build called (`at`, `findLast`, `toSorted`,
 * `AbortSignal.timeout`). Imported first by main.ts, before anything that calls them.
 */

/** Fills name on target with value, where the engine has none. */
export function fill(target: object, name: string, value: unknown): void {
  if (!(name in target)) {
    Reflect.defineProperty(target, name, { value, writable: true, configurable: true });
  }
}

/** The element at index, counted from the end when negative (Chrome 92). */
export function at<T>(this: ArrayLike<T>, index: number): T | undefined {
  const truncated = Math.trunc(index);
  const whole = Number.isNaN(truncated) ? 0 : truncated;
  const from = whole < 0 ? this.length + whole : whole;
  return from < 0 || from >= this.length ? undefined : this[from];
}

/** The index of the last of values that passes test, or -1. */
function lastPassing<T>(values: readonly T[], test: (value: T, index: number) => boolean): number {
  return values.reduce((found, value, index) => (test(value, index) ? index : found), -1);
}

/** The index of the last element that passes test, or -1 (Chrome 97). */
export function findLastIndex<T>(
  this: readonly T[],
  test: (value: T, index: number) => boolean,
): number {
  return lastPassing(this, test);
}

/** The last element that passes test (Chrome 97). */
export function findLast<T>(
  this: readonly T[],
  test: (value: T, index: number) => boolean,
): T | undefined {
  const index = lastPassing(this, test);
  return index < 0 ? undefined : this[index];
}

/** A sorted copy, the array left as it was (Chrome 110). */
export function toSorted<T>(this: readonly T[], compare?: (a: T, b: T) => number): T[] {
  return [...this].sort(compare);
}

/** A signal that aborts with a TimeoutError after ms (Chrome 103). */
export function timeout(ms: number): AbortSignal {
  const controller = new AbortController();
  setTimeout(() => {
    controller.abort(new DOMException("signal timed out", "TimeoutError"));
  }, ms);
  return controller.signal;
}

/** Fills what the engine lacks of the methods the bundle calls. */
export function fillOldEngine(): void {
  for (const prototype of [Array.prototype, String.prototype, Uint8Array.prototype]) {
    fill(prototype, "at", at);
  }
  fill(Array.prototype, "findLast", findLast);
  fill(Array.prototype, "findLastIndex", findLastIndex);
  fill(Array.prototype, "toSorted", toSorted);
  fill(AbortSignal, "timeout", timeout);
}

fillOldEngine();
