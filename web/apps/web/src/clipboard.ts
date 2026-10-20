// SPDX-License-Identifier: MPL-2.0

/**
 * Copies text with the browser's clipboard, the one way the client does (AGENTS rule 11): it is there
 * in a secure context alone, and rejects where the page may not use it, so a caller offers the text
 * to select instead.
 */
export async function copyToClipboard(text: string): Promise<void> {
  if (!globalThis.isSecureContext) {
    throw new Error("the clipboard is there in a secure context alone");
  }
  await globalThis.navigator.clipboard.writeText(text);
}
