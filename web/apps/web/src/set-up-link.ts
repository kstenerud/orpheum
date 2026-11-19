// SPDX-License-Identifier: MPL-2.0

/**
 * The setup code a magic link carries (D527 §1.2): the server prints
 * `http://<address>:<port>/#code=<code>` for the first admin to open, and the code sits after the #
 * so a browser never sends it to the server and no log of the server or of a proxy holds it.
 * cmd/orpheum/setup_link.go writes the link; this reads it.
 */

/** The fragment's key the link carries the code under (cmd/orpheum's setupLinkCodeKey). */
const codeKey = "code";

/**
 * The setup code in fragment (what follows the #, as navigation.fragment() gives it), or undefined
 * when the fragment carries none: an invitation's or a pairing's fragment has no `code=`.
 */
export function setupCodeFromFragment(fragment: string): string | undefined {
  const bare = fragment.startsWith("#") ? fragment.slice(1) : fragment;
  const code = new URLSearchParams(bare).get(codeKey)?.trim();
  return code === undefined || code === "" ? undefined : code;
}
