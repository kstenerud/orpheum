// SPDX-License-Identifier: MPL-2.0

/**
 * A moment, in the viewer's words: its date at medium length and its time, short ("29 Sept 2026,
 * 14:05" in British English). Every screen that shows when something happened or will happen
 * says it this way, or by one of the parts below when only a part is meant.
 */
export function dateTimeText(epochMs: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(epochMs),
  );
}

/** A moment's date alone, as dateTimeText says it ("29 Sept 2026"): for a day, not a time. */
export function dateText(epochMs: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(new Date(epochMs));
}

/** A moment's time alone, as dateTimeText says it ("14:05"): for a moment later today. */
export function timeText(epochMs: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, { timeStyle: "short" }).format(new Date(epochMs));
}
