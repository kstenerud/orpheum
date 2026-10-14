// SPDX-License-Identifier: MPL-2.0

/**
 * A core reached by one of its published names, from a page on another origin (the discovery
 * protocol's §3.2, §3.3; webrtc-transport.md W2c's thin form): its direct paths raced, the first to
 * answer taken, and the page's requests sent there. The name's certificate, which only that core can
 * obtain under its own label, is what says it is the core (§2.7).
 */

import { apiPrefix } from "./client.ts";
import type { FetchRequest } from "./rtc-transport.ts";

/** How long a direct path has to answer before the race goes on without it. */
export const directPathWaitMs = 3000;

/**
 * The origin of the first of urls whose core answers its health, or undefined when none does within
 * wait: each is asked at once, so a LAN name that a phone away from home cannot reach costs no more
 * than the wait.
 */
export async function firstDirectPath(
  urls: readonly string[],
  send: typeof fetch = fetch,
  waitMs: number = directPathWaitMs,
): Promise<string | undefined> {
  const asked = urls.map(async (url) => {
    const origin = new URL(url).origin;
    const answer = await send(`${origin}${apiPrefix}/health`, {
      mode: "cors",
      credentials: "omit",
      signal: AbortSignal.timeout(waitMs),
    });
    if (!answer.ok && answer.status !== 503) {
      throw new Error(`${origin} answered ${String(answer.status)}`);
    }
    return origin;
  });
  try {
    return await Promise.any(asked);
  } catch {
    return undefined;
  }
}

/**
 * Sends each request the page makes on its own origin to the core at origin instead, path and query
 * kept, with no cookie: the session goes in the Authorization header the request already carries
 * (the protocol's §3.4).
 */
export function directFetch(origin: string, send: typeof fetch = fetch): FetchRequest {
  return async (request) => {
    const asked = new URL(request.url);
    const body =
      request.method === "GET" || request.method === "HEAD"
        ? undefined
        : await request.arrayBuffer();
    return await send(`${origin}${asked.pathname}${asked.search}`, {
      method: request.method,
      headers: request.headers,
      ...(body === undefined ? {} : { body }),
      mode: "cors",
      credentials: "omit",
      signal: request.signal,
    });
  };
}
