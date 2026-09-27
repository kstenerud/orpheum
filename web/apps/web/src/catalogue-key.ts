// SPDX-License-Identifier: MPL-2.0

/** A catalogue key for one of the server's snake_case names: an extra's type, a reason. */
export function camelKey(type: string): string {
  return type.replaceAll(/_([a-z])/gu, (_, letter: string) => letter.toUpperCase());
}
