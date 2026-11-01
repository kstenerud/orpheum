// SPDX-License-Identifier: MPL-2.0

/**
 * A WebRTC transport's tries to make a connection again (rtc-transport.ts): each bounded, given up
 * when the browser comes back online, and the time the transport had none said once one is back;
 * and the failure no later try mends (RefusedForGood).
 */

import { abortReason } from "./rtc-request.ts";

/**
 * How long a try to make a connection may take before it is given up and tried again after the
 * wait: longer than its own steps' bounds together (rtc-connect.ts: gathering and connecting within
 * connectTimeoutMs each, the verdict within verdictWaitMs), so it ends only a try that hangs where
 * nothing else bounds it, as a signaller's post sent into a network that went away. Unbounded, such
 * a try held the transport's one opening for good, and no replacement was ever tried again (the
 * owner's Firefox, staging, 2026-10-29).
 */
export const openTryMostMs = 25_000;

/** Why a try to connect was given up: the browser came back online, and it is tried anew at once. */
export class BackOnline extends Error {
  constructor() {
    super("the browser came back online");
    this.name = "BackOnline";
  }
}

/**
 * What opening resolves with, a peer, unless trying aborts first, or openTryMostMs pass, which abort it:
 * then it rejects with the abort's reason, and a peer opening resolves with later is closed.
 */
export async function withinOpenTry<Opened extends { close(): void }>(
  opening: Promise<Opened>,
  trying: AbortController,
): Promise<Opened> {
  const { signal } = trying;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const givenUp = new Promise<never>((_, reject) => {
    const aborted = (): void => {
      reject(abortReason(signal));
    };
    signal.addEventListener("abort", aborted, { once: true });
    timer = setTimeout(() => {
      trying.abort(new Error(`the try to connect took over ${String(openTryMostMs)} ms`));
    }, openTryMostMs);
  });
  givenUp.catch(() => undefined);
  try {
    return await Promise.race([opening, givenUp]);
  } catch (problem: unknown) {
    opening.then(
      (late) => {
        late.close();
      },
      () => undefined,
    );
    throw problem;
  } finally {
    clearTimeout(timer);
  }
}

/**
 * The time the transport has had no connection: since when, on its clock, why the last went, how
 * many tries failed since, and the last one's failure.
 */
export interface Without {
  readonly sinceMs: number;
  readonly why: string;
  tries: number;
  lastFailure: string | undefined;
}

/**
 * What a transport's connection back after a time without one tells the core's log: how long it had
 * none, why it took the last one for gone, and how its tries went. A page that never connected again
 * after a network's loss left nothing in the log after its last connection went (the owner's Firefox,
 * staging, 2026-10-29).
 */
export function backAfter(without: Without, nowMs: number): string {
  const tried =
    without.lastFailure === undefined
      ? `${String(without.tries)} tries failed`
      : `${String(without.tries)} tries failed, the last: ${without.lastFailure}`;
  return (
    `a WebRTC connection to the core is back after ${String(Math.round(nowMs - without.sinceMs))} ms` +
    ` with none; the last was taken as gone because ${without.why}; ${tried}`
  );
}

/**
 * A try to connect that no later try mends: the core refused what the page proves its offers by,
 * having tried what else it holds (the discovery's KeyRefused). The transport tries no more after
 * one, and says so (RtcTransport.onRefused): Firefox, a Watch Together guest whose every offer was
 * refused under a spent link's key, tried every 10 s for good, its Play page spinning, saying nothing
 * (the owner's Mac, staging, 2026-10-29).
 */
export class RefusedForGood extends Error {
  constructor(message: string) {
    super(message);
    this.name = "RefusedForGood";
  }
}

/** Whether problem, or any cause or alternative behind it, is a try no later try mends. */
export function isRefusedForGood(problem: unknown): boolean {
  return isOrBehind(problem, RefusedForGood);
}

/**
 * How long the transport waits before it tries again after a try whose offer reached no server
 * (Unreached), however many failed so: a try that reaches nothing loads nothing, and the next one is
 * how the page hears its network back where the browser does not say so. Firefox on the desktop fires
 * no `online` event when the network returns, so its transport, its wait grown to replaceMostMs and a
 * try begun on no network running out its own bounds, came back about 45 s after the Mac's Wi-Fi did
 * where Chrome, told `online`, came back at once (the owner's VQ139 run, staging, 2026-10-29;
 * idiosyncrasies.md, Firefox). Short enough that a page is back within a few seconds of its network,
 * long enough that a page left without one for hours is not making a peer connection every moment.
 */
export const unreachedRetryMs = 2000;

/**
 * A try to connect whose offer reached no server: its signaller's post failed before any answer, as
 * the browser's fetch fails with no network. The transport tries again after unreachedRetryMs, its
 * wait not grown, rather than after its growing wait.
 */
export class Unreached extends Error {
  constructor(cause: unknown) {
    super(
      `the offer reached no server: ${cause instanceof Error ? cause.message : String(cause)}`,
      { cause },
    );
    this.name = "Unreached";
  }
}

/**
 * What posting resolves with, a response; a post that reached no server, which fetch fails with a
 * TypeError ("Failed to fetch", "NetworkError when attempting to fetch resource."), fails as
 * Unreached, so the transport tries again soon rather than after its growing wait (unreachedRetryMs).
 * An abort stays an abort.
 */
export async function reaching<Posted>(posting: Promise<Posted>): Promise<Posted> {
  try {
    return await posting;
  } catch (problem: unknown) {
    throw problem instanceof TypeError ? new Unreached(problem) : problem;
  }
}

/** Whether problem, or any cause or alternative behind it, is a try whose offer reached no server. */
export function isUnreached(problem: unknown): boolean {
  return isOrBehind(problem, Unreached);
}

/** Whether problem, or any cause or alternative behind it, is a kind. */
function isOrBehind(problem: unknown, kind: new (...given: never[]) => Error): boolean {
  const seen: unknown[] = [problem];
  for (let index = 0; index < seen.length && index < 32; index += 1) {
    const each = seen[index];
    if (each instanceof kind) {
      return true;
    }
    if (each instanceof AggregateError) {
      seen.push(...Array.from<unknown>(each.errors));
    }
    if (each instanceof Error && each.cause !== undefined) {
      seen.push(each.cause);
    }
  }
  return false;
}
