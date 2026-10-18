// SPDX-License-Identifier: MPL-2.0

import { ApiProblem } from "./problem.ts";

/**
 * Where a playback the server lost was, and whether its person had it paused: where the one opened
 * in its place starts, and whether it plays at once.
 */
export interface ReopenAt {
  readonly positionMs: number;
  readonly isPaused: boolean;
}

/**
 * The codes of refusals that pass: a server busy, updating, or not answering its gateway while it
 * restarts. A report refused with one goes again with the next; a lost playback's new one is asked
 * for again.
 */
const passingRefusalCodes: ReadonlySet<string> = new Set([
  "internal",
  "migrating",
  "not_answering",
  "rate_limited",
]);

/** Whether a failure to reach the server may pass: no answer at all, or a refusal that passes. */
export function mayPass(cause: unknown): boolean {
  if (cause instanceof ApiProblem) {
    return passingRefusalCodes.has(cause.code);
  }
  return cause instanceof TypeError; // fetch's own failure: no answer at all
}

/** Whether the server refused because it no longer has the playback: it restarted, or ended it. */
export function isPlaybackGone(cause: unknown): boolean {
  return cause instanceof ApiProblem && cause.code === "playback_gone";
}

/**
 * The waits between asks for a lost playback's new one while the server does not answer, about a
 * minute in all: as long as a restart takes, and longer than a person waits without being told.
 */
export const reopenWaitsMs: readonly number[] = [1000, 2000, 4000, 8000, 15_000, 15_000, 15_000];

/**
 * The shortest time between two reopens of one page's film. A playback lost again sooner is not
 * reopened but told, so a server that loses each one it opens is no loop of them.
 */
export const reopenAtMostEveryMs = 60_000;

/** Waits ms, as setTimeout does. */
function waitFor(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * What ask answers, asked again after each of reopenWaitsMs while it fails with what may pass; the
 * last failure, or the first that cannot pass, is thrown.
 */
export async function askUntilAnswered<T>(
  ask: () => Promise<T>,
  wait: (ms: number) => Promise<void> = waitFor,
): Promise<T> {
  for (const waitMs of reopenWaitsMs) {
    try {
      return await ask();
    } catch (cause) {
      if (!mayPass(cause)) {
        throw cause;
      }
    }
    await wait(waitMs);
  }
  return await ask();
}
