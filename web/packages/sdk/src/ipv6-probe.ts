// SPDX-License-Identifier: MPL-2.0

/**
 * The discovery's IPv6 probe name (its paths' `ipv6Probe`): a name with an IPv6 address alone, which
 * a page fetches during its first race to judge whether its network has IPv6 (D398 §1, §7, as the
 * owner agreed on 2026-11-03). An answer says it has; a failure before the browser could have tried
 * a connection's first exchange says it has none, as a name of a family the network has no route for
 * fails; no answer within the wait, or a failure later than that, says nothing.
 */

import { directPathWaitMs } from "./direct-path.ts";
import type { HasIpv6 } from "./discovery.ts";

/**
 * How soon a failure of the probe says the network has no IPv6: a browser with no IPv6 route fails at
 * once, while one whose packets are lost fails only at the wait's end; a failure after this is not
 * read either way.
 */
export const ipv6ProbeFastMs = 1000;

/**
 * Whether this network has IPv6, by fetching url, the discovery's probe name: "yes" when anything
 * answers it, of any status (the fetch is no-cors, so a page reads no status); "no" when it fails
 * within fastMs; "unknown" when it fails later, or nothing answers within answerMs, when the fetch is
 * given up. It never rejects. The waits are ipv6ProbeFastMs and directPathWaitMs unless a test names
 * others.
 */
export function probeIpv6(
  url: string,
  send: typeof fetch = fetch,
  waits: { readonly fastMs?: number; readonly answerMs?: number } = {},
): Promise<HasIpv6> {
  const { fastMs = ipv6ProbeFastMs, answerMs = directPathWaitMs } = waits;
  const stop = new AbortController();
  let isLate = false;
  const late = setTimeout(() => {
    isLate = true;
  }, fastMs);
  const given = setTimeout(() => {
    stop.abort();
  }, answerMs);
  const ended = (): void => {
    clearTimeout(late);
    clearTimeout(given);
  };
  let asked: Promise<Response>;
  try {
    asked = send(url, {
      mode: "no-cors",
      cache: "no-store",
      credentials: "omit",
      signal: stop.signal,
    });
  } catch (problem: unknown) {
    asked = Promise.reject(problem instanceof Error ? problem : new Error(String(problem)));
  }
  return asked.then(
    (): HasIpv6 => {
      ended();
      return "yes";
    },
    (): HasIpv6 => {
      ended();
      return isLate || stop.signal.aborted ? "unknown" : "no";
    },
  );
}
