// SPDX-License-Identifier: MPL-2.0

/**
 * A number in the viewer's words, to the tenth at most ("6.5", or "6,5" in German). Every screen
 * that shows a measure with one decimal says it this way.
 */
export function tenthsText(value: number, locale: string): string {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(value);
}
