// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";

/**
 * Whether a copy of this server was refused, or this one was (D437 §4): for the status tray's sign
 * and the Needs attention page, kept apart from the page so the tray loads none of it.
 */
export function hasCopyWarning(value: components["schemas"]["NeedsAttentionBody"]): boolean {
  return value.copyRefusedAt > 0 || value.stoodAsideAt > 0;
}
