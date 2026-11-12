// SPDX-License-Identifier: MPL-2.0

/**
 * Checking the core's Ed25519 signatures in any browser the clients run in: WebCrypto's Ed25519
 * where the browser has it, @noble/ed25519 where it does not (Chrome before 137, as a Chromecast's
 * Chrome 90: idiosyncrasies.md).
 */

import { verifyAsync } from "@noble/ed25519";

/**
 * Whether signature is publicKey's Ed25519 signature over message: checked by the browser's
 * WebCrypto, or, where its WebCrypto has no Ed25519 (Chrome before 137, as a Chromecast's Chrome
 * 90: idiosyncrasies.md), by @noble/ed25519.
 */
export async function isEd25519SignatureOf(
  publicKey: Uint8Array<ArrayBuffer>,
  signature: Uint8Array<ArrayBuffer>,
  message: Uint8Array<ArrayBuffer>,
): Promise<boolean> {
  let key: CryptoKey;
  try {
    key = await crypto.subtle.importKey("raw", publicKey, { name: "Ed25519" }, false, ["verify"]);
  } catch (cause) {
    // A browser without Ed25519 checks it by @noble/ed25519; a key it cannot take verifies nothing.
    return isUnknownAlgorithm(cause)
      ? await isEd25519SignatureByNoble(publicKey, signature, message)
      : false;
  }
  try {
    return await crypto.subtle.verify("Ed25519", key, signature, message);
  } catch {
    return false;
  }
}

/** Whether cause is WebCrypto refusing an algorithm it does not know, not a key it cannot take. */
function isUnknownAlgorithm(cause: unknown): boolean {
  return cause instanceof DOMException && cause.name === "NotSupportedError";
}

/**
 * isEd25519SignatureOf without WebCrypto's Ed25519: under RFC 8032's rules (zip215 off), as
 * WebCrypto and Go's crypto/ed25519 check it, its SHA-512 still WebCrypto's.
 */
async function isEd25519SignatureByNoble(
  publicKey: Uint8Array<ArrayBuffer>,
  signature: Uint8Array<ArrayBuffer>,
  message: Uint8Array<ArrayBuffer>,
): Promise<boolean> {
  try {
    return await verifyAsync(signature, message, publicKey, { zip215: false });
  } catch {
    // A key or signature of the wrong length verifies nothing.
    return false;
  }
}
