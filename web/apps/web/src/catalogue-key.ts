// SPDX-License-Identifier: MPL-2.0

/** A catalogue key for one of the server's snake_case names: an extra's type, a reason. */
export function camelKey(snakeName: string): string {
  return snakeName.replaceAll(/_([a-z])/gu, (_, letter: string) => letter.toUpperCase());
}
