// SPDX-License-Identifier: MPL-2.0

/**
 * The header that names one click's request to the server, the same on every retry of it, so a
 * POST is done once however often it is sent (D244 §4): the options a POST is given beside its body.
 */
export function idempotent(idempotencyKey: string): { readonly headers: Record<string, string> } {
  return { headers: { "Idempotency-Key": idempotencyKey } };
}

/** A key's length in random bytes: 128 bits, as a UUID carries. */
const keyBytes = 16;

/**
 * A new key for one click's request. Made from crypto.getRandomValues, which every browser has,
 * not crypto.randomUUID, which a browser offers only in a secure context: a page served over plain
 * HTTP on a LAN name, as a house's server is until it has its HTTPS name, has none, and a click
 * that made its key with it did nothing at all.
 */
export function newIdempotencyKey(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(keyBytes));
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}
