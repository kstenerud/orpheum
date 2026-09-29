// SPDX-License-Identifier: MPL-2.0

/**
 * A moment, in the viewer's words: its date at medium length and its time, short ("29 Sept 2026,
 * 14:05" in British English). Every screen that shows when something happened or will happen
 * says it this way.
 */
export function dateTimeText(epochMs: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(epochMs),
  );
}
