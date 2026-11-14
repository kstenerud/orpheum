// SPDX-License-Identifier: MPL-2.0

/**
 * The one read of what needs the admin, which the status tray's warning sign and the Needs attention
 * page share by its key (D478): the page shows the sign's answer at once and reads again behind it,
 * and what it reads the sign shows too. Apart from either, so the shell loads none of the page's code.
 */

import type { Api, components } from "@orpheum/sdk";

import { dataOrThrow } from "./problem.ts";

/** What needs the admin, counted, as GET /needs-attention answers. */
export type NeedsAttentionCounts = components["schemas"]["NeedsAttentionBody"];

/** The key both readers read it by (useResource). */
export const needsAttentionKey = "needs-attention";

/** Reads what needs the admin. */
export async function readNeedsAttention(api: Api): Promise<NeedsAttentionCounts> {
  return dataOrThrow(await api.GET("/needs-attention"));
}
