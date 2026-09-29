// SPDX-License-Identifier: MPL-2.0

import { ApiProblem, problemFromAnswer } from "./problem.ts";

/**
 * What waiting for a file the server is still making needs: how to fetch and how to wait, so a
 * test can supply both.
 */
export interface WaitOptions {
  readonly fetch?: (url: string, init: RequestInit) => Promise<Response>;
  readonly sleep?: (milliseconds: number) => Promise<void>;
  readonly signal?: AbortSignal;
  /** How many times to ask before giving up; each wait is the server's Retry-After. */
  readonly maxAttempts?: number;
}

const statusTooEarly = 425;

/**
 * Waits until a prepared rendition's playlist exists: the server answers 425 with a Retry-After
 * while the first segment is being made (cut-a §6). The request carries the media cookie, as the
 * player's will. A failure other than "too early" is thrown as the ApiProblem it answered.
 */
export async function whenReady(url: string, options: WaitOptions = {}): Promise<void> {
  await readyAnswer(url, options);
}

/**
 * Reads a text the server may still be making, such as a subtitle track being extracted: it
 * answers 425 with a Retry-After until the text exists (D217 §6), and is waited for as a
 * rendition is.
 */
export async function textWhenReady(url: string, options: WaitOptions = {}): Promise<string> {
  const answer = await readyAnswer(url, options);
  return await answer.text();
}

/** The first answer that is not "too early", when it is a success. */
async function readyAnswer(url: string, options: WaitOptions): Promise<Response> {
  const { get, sleep, maxAttempts } = withDefaults(options);
  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    const answer = await get(url, requestInit(options.signal));
    if (answer.ok) {
      return answer;
    }
    if (answer.status !== statusTooEarly) {
      throw await problemFromAnswer(answer);
    }
    await sleep(retryAfterSeconds(answer) * 1000);
  }
  throw new ApiProblem({
    code: "preparing",
    message: "still being prepared; try again in a while",
  });
}

function withDefaults(options: WaitOptions): {
  get: (url: string, init: RequestInit) => Promise<Response>;
  sleep: (milliseconds: number) => Promise<void>;
  maxAttempts: number;
} {
  return {
    get: options.fetch ?? ((address, init) => fetch(address, init)),
    sleep:
      options.sleep ??
      ((milliseconds) => new Promise<void>((resolve) => setTimeout(resolve, milliseconds))),
    maxAttempts: options.maxAttempts ?? 600,
  };
}

function requestInit(signal: AbortSignal | undefined): RequestInit {
  return signal === undefined
    ? { credentials: "same-origin" }
    : { credentials: "same-origin", signal };
}

/** The seconds a 425 asks to wait, or one when it names none that makes sense. */
function retryAfterSeconds(answer: Response): number {
  const seconds = Number(answer.headers.get("Retry-After") ?? "1");
  return Number.isFinite(seconds) && seconds > 0 ? seconds : 1;
}
