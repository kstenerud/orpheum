// SPDX-License-Identifier: MPL-2.0

import { protocolVersion, type DiscoveryClient } from "./discovery.ts";
import { waitMs } from "./wait-ms.ts";

/**
 * How long a loader waits on its discovery's origin for what it can open without (D486): the loader's
 * page, past which its worker answers the page it kept, and the client floor, past which it is taken
 * as not reached. A justified constant, not a setting: a fresh connection's three round trips (TCP,
 * TLS and the request) on the worst link that still works in the slow-link war game, 800 ms each
 * (2026-11-14), fit in it, and a link that cannot answer in it showed a white page for as long as
 * its browser waited, up to minutes.
 */
export const discoveryWaitMs = 2500;

/**
 * The client versions the discovery refuses, its floor (D308 §6): a loader runs none of them. None
 * when the discovery keeps no floor, or is not reached within discoveryWaitMs (D486), so a discovery
 * without one still serves, and a link that cannot answer holds no page up for it.
 */
export async function refusedClientVersions(client: DiscoveryClient): Promise<readonly string[]> {
  const send = client.fetch ?? fetch;
  const asked = new AbortController();
  const unanswered = waitMs(discoveryWaitMs).then(() => {
    asked.abort();
    return [];
  });
  return await Promise.race([floorOf(send, client.clientApi, asked.signal), unanswered]);
}

/** The floor, as the discovery's clientApi answers it over send; none when it does not. */
async function floorOf(
  send: typeof fetch,
  clientApi: string,
  signal: AbortSignal,
): Promise<readonly string[]> {
  try {
    const response = await send(`${clientApi}/client-floor`, {
      headers: { "Orpheum-Discovery-Protocol": protocolVersion },
      signal,
    });
    const answer: unknown = response.ok ? await response.json() : undefined;
    const refused: unknown =
      typeof answer === "object" && answer !== null
        ? Reflect.get(answer, "refusedVersions")
        : undefined;
    return Array.isArray(refused)
      ? refused.filter((version): version is string => typeof version === "string")
      : [];
  } catch {
    return [];
  }
}
