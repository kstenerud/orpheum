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
