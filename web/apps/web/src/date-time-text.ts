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

/**
 * A moment's time alone, as dateTimeText says it ("14:05"): for a moment later today. Its formatter
 * is kept for each locale: one built at each call cost 38 µs against 1.8 µs for one kept, and the
 * player's end time asks four times a second (the efficiency audit's finding 7).
 */
export function timeText(epochMs: number, locale: string): string {
  const format = keptFormat(
    shortTimes,
    locale,
    () => new Intl.DateTimeFormat(locale, { timeStyle: "short" }),
  );
  return format.format(new Date(epochMs));
}

/**
 * A length of film, or a place in one, as a player shows it, in the locale's digits: m:ss under an
 * hour ("41:12"), h:mm:ss from one ("1:02:03"). The one way a film's clock is written: the
 * timeline, a resume point, a skipped stretch and a Watch Together event all say it so.
 */
export function clockText(ms: number, locale: string): string {
  const { plain, twoDigits } = keptFormat(clockDigits, locale, () => ({
    plain: new Intl.NumberFormat(locale, { useGrouping: false }),
    twoDigits: new Intl.NumberFormat(locale, { minimumIntegerDigits: 2, useGrouping: false }),
  }));
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = twoDigits.format(totalSeconds % 60);
  return hours === 0
    ? `${plain.format(minutes)}:${seconds}`
    : `${plain.format(hours)}:${twoDigits.format(minutes)}:${seconds}`;
}

/** The short time's formatter of each locale asked for, timeText's. */
const shortTimes = new Map<string, Intl.DateTimeFormat>();

/** The digits of each locale asked for, clockText's: the timeline asks at each timeupdate. */
const clockDigits = new Map<
  string,
  { readonly plain: Intl.NumberFormat; readonly twoDigits: Intl.NumberFormat }
>();

/** The formatter kept in formats for locale, made by make the first time it is asked for. */
function keptFormat<T>(formats: Map<string, T>, locale: string, make: () => T): T {
  const kept = formats.get(locale);
  if (kept !== undefined) {
    return kept;
  }
  const made = make();
  formats.set(locale, made);
  return made;
}
