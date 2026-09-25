// SPDX-License-Identifier: MPL-2.0

/** What waiting for a rendition needs: how to fetch and how to wait, so a test can supply both. */
export interface WaitOptions {
  readonly fetch?: (url: string, init: RequestInit) => Promise<Response>;
  readonly sleep?: (milliseconds: number) => Promise<void>;
  readonly signal?: AbortSignal;
  /** How many times to ask before giving up; each wait is the server's Retry-After. */
  readonly attempts?: number;
}

const tooEarly = 425;

/**
 * Waits until a prepared rendition's playlist exists: the server answers 425 with a Retry-After
 * while the first segment is being made (cut-a §6). The request carries the media cookie, as the
 * player's will. A failure other than "too early" is thrown with the server's message.
 */
export async function whenReady(url: string, options: WaitOptions = {}): Promise<void> {
  const { get, sleep, attempts } = withDefaults(options);
  for (let attempt = 0; attempt < attempts; attempt++) {
    const answer = await get(url, requestInit(options.signal));
    if (answer.ok) {
      return;
    }
    if (answer.status !== tooEarly) {
      throw new Error(await errorMessage(answer));
    }
    await sleep(retryAfter(answer) * 1000);
  }
  throw new Error("the file is still being prepared; try again in a while");
}

function withDefaults(options: WaitOptions): {
  get: (url: string, init: RequestInit) => Promise<Response>;
  sleep: (milliseconds: number) => Promise<void>;
  attempts: number;
} {
  return {
    get: options.fetch ?? ((address, init) => fetch(address, init)),
    sleep:
      options.sleep ??
      ((milliseconds) => new Promise<void>((resolve) => setTimeout(resolve, milliseconds))),
    attempts: options.attempts ?? 600,
  };
}

function requestInit(signal: AbortSignal | undefined): RequestInit {
  return signal === undefined
    ? { credentials: "same-origin" }
    : { credentials: "same-origin", signal };
}

/** The seconds a 425 asks to wait, or one when it names none that makes sense. */
function retryAfter(answer: Response): number {
  const seconds = Number(answer.headers.get("Retry-After") ?? "1");
  return Number.isFinite(seconds) && seconds > 0 ? seconds : 1;
}

async function errorMessage(answer: Response): Promise<string> {
  try {
    const body: unknown = await answer.json();
    if (typeof body === "object" && body !== null && "error" in body) {
      const detail = body.error;
      if (typeof detail === "object" && detail !== null && "message" in detail) {
        return String(detail.message);
      }
    }
  } catch {
    // Not our error shape; the status says enough.
  }
  return `the server answered ${String(answer.status)}`;
}
