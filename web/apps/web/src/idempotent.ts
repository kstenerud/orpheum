// SPDX-License-Identifier: MPL-2.0

/**
 * The header that names one click's request to the server, the same on every retry of it, so a
 * POST is done once however often it is sent (D244 §4): the options a POST is given beside its body.
 */
export function idempotent(idempotencyKey: string): { readonly headers: Record<string, string> } {
  return { headers: { "Idempotency-Key": idempotencyKey } };
}
