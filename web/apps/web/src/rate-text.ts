// SPDX-License-Identifier: MPL-2.0

/** A bit rate as a person reads it: megabits a second, to one decimal. */
export function rateText(kbps: number): string {
  return `${(kbps / 1000).toFixed(1)} Mb/s`;
}
