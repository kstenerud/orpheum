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
 * How long a name the discovery ranks higher is waited for once a lower one answered: the race rule's
 * "a better one answering shortly after takes over" (webrtc-transport.md W2c). At home the LAN name
 * may answer a little after a public one, which reaches the core the long way, through the VPS, where
 * the core cannot tell its person is at home (the owner, 2026-10-15).
 */
export const betterPathGraceMs = 1000;

/**
 * The origin of the best of urls, in the discovery's order (the LAN's, IPv6's, IPv4's), whose core
 * answers its health, or undefined when none does within wait: each is asked at once, so a LAN name
 * that a phone away from home cannot reach costs no more than the wait; once one answers, a better
 * one is waited for at most betterMs, and not at all when every better one has failed. The waits are
 * directPathWaitMs and betterPathGraceMs unless a test names others.
 */
export function firstDirectPath(
  urls: readonly string[],
  send: typeof fetch = fetch,
  waits: { readonly answerMs?: number; readonly betterMs?: number } = {},
): Promise<string | undefined> {
  const { answerMs: waitMs = directPathWaitMs, betterMs = betterPathGraceMs } = waits;
  return new Promise((resolve) => {
    const answered: (string | undefined)[] = urls.map(() => undefined);
    const failed = urls.map(() => false);
    let isDone = false;
    let grace: ReturnType<typeof setTimeout> | undefined;
    const finish = (): void => {
      if (!isDone) {
        isDone = true;
        clearTimeout(grace);
        resolve(answered.find((origin) => origin !== undefined));
      }
    };
    // The best answer is final once every name ranked above it has failed; with none, all failed.
    const decide = (): void => {
      const best = answered.findIndex((origin) => origin !== undefined);
      if (best >= 0 && failed.slice(0, best).every(Boolean)) {
        finish();
      } else if (best >= 0) {
        grace ??= setTimeout(finish, betterMs);
      } else if (failed.every(Boolean)) {
        finish();
      }
    };
    urls.forEach((url, index) => {
      void healthOf(url, send, waitMs).then(
        (origin) => {
          answered[index] = origin;
          decide();
        },
        () => {
          failed[index] = true;
          decide();
        },
      );
    });
    decide();
  });
}

/** The origin of url when its core answers its health within waitMs; it rejects otherwise. */
async function healthOf(url: string, send: typeof fetch, waitMs: number): Promise<string> {
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
