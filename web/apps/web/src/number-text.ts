// SPDX-License-Identifier: MPL-2.0

/**
 * A number in the viewer's words, to the tenth at most ("6.5", or "6,5" in German). Every screen
 * that shows a measure with one decimal says it this way.
 */
export function tenthsText(value: number, locale: string): string {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(value);
}

/** A change in the viewer's words, always to the tenth and signed when not zero ("+0.3", "-1,5"). */
export function signedTenthsText(value: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
    signDisplay: "exceptZero",
  }).format(value);
}

/** A size in bytes in the viewer's words, in the unit that keeps it short: "12.3 GB". */
export function byteSizeText(bytes: number, locale: string): string {
  const units = ["byte", "kilobyte", "megabyte", "gigabyte", "terabyte"] as const;
  let value = bytes;
  let index = 0;
  while (value >= 1000 && index < units.length - 1) {
    value /= 1000;
    index += 1;
  }
  return new Intl.NumberFormat(locale, {
    style: "unit",
    unit: units[index] ?? "byte",
    unitDisplay: "short",
    maximumFractionDigits: index === 0 ? 0 : 1,
  }).format(value);
}
