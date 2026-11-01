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
 * directPathWaitMs and betterPathGraceMs unless a test names others. Once the race is decided, its
 * tell hears what came of each name, in urls' order: why the page took the path it took was otherwise
 * said nowhere (local-path-2026-10-27.md §4 item 1).
 */
export function firstDirectPath(
  urls: readonly string[],
  send: typeof fetch = fetch,
  waits: {
    readonly answerMs?: number;
    readonly betterMs?: number;
    readonly tell?: ((outcomes: readonly DirectOutcome[]) => void) | undefined;
  } = {},
): Promise<string | undefined> {
  const { answerMs: waitMs = directPathWaitMs, betterMs = betterPathGraceMs, tell } = waits;
  return new Promise((resolve) => {
    const answered: (string | undefined)[] = urls.map(() => undefined);
    const failed = urls.map(() => false);
    const outcomes = urls.map((url): DirectOutcome => ({ url, outcome: notWaitedFor }));
    let isDone = false;
    let grace: ReturnType<typeof setTimeout> | undefined;
    const finish = (): void => {
      if (!isDone) {
        isDone = true;
        clearTimeout(grace);
        tell?.(outcomes.map((each) => ({ ...each })));
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
          outcomes[index] = { url, outcome: "answered" };
          decide();
        },
        (problem: unknown) => {
          failed[index] = true;
          outcomes[index] = { url, outcome: failureOf(problem, waitMs) };
          decide();
        },
      );
    });
    decide();
  });
}

/** What came of one of a race's names: "answered", "failed: " and how, or notWaitedFor. */
export interface DirectOutcome {
  readonly url: string;
  readonly outcome: string;
}

/** The outcome of a name the race was decided without, still unanswered then. */
export const notWaitedFor = "not waited for";

/** The longest a failure's words are kept for its outcome. */
const failureMost = 200;

/** How the outcome of a name not answered within the wait begins. */
const noAnswerWords = "failed: no answer within ";

/**
 * How one of a race's names ended, as a dead end's report and words read it (D398 §2): it answered;
 * it answered with an HTTP failure, so the page's network reached it (refused); nothing answered
 * within the wait (noAnswer); it failed before the wait ended with no answer at all, as a name of an
 * address family the network has no route for does (unreachable); or the race was decided without it.
 */
export type DirectEnding = "answered" | "refused" | "noAnswer" | "unreachable" | "notWaited";

/** How a name whose race outcome is outcome ended (firstDirectPath's tell). */
export function endingOf(outcome: string): DirectEnding {
  if (outcome === "answered") {
    return "answered";
  }
  if (outcome === notWaitedFor) {
    return "notWaited";
  }
  if (outcome.startsWith(noAnswerWords)) {
    return "noAnswer";
  }
  return / answered \d{3}$/u.test(outcome) ? "refused" : "unreachable";
}

/** How a name failed, for its outcome: no answer within waitMs, or the failure's own words. */
function failureOf(problem: unknown, waitMs: number): string {
  if (problem instanceof DOMException && problem.name === "TimeoutError") {
    return `${noAnswerWords}${String(waitMs)} ms`;
  }
  const words = problem instanceof Error ? `${problem.name}: ${problem.message}` : String(problem);
  return `failed: ${words}`.slice(0, failureMost);
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
