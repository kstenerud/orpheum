// SPDX-License-Identifier: MPL-2.0

import { SilentRequestError } from "@orpheum/sdk";

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

/**
 * Whether a failure to reach the server may pass: no answer at all, a refusal that passes, or the
 * machine holding the film away, as it is for the seconds after the server restarts, until it is
 * heard again (the owner's test, 2026-10-18: a reopen asked 30 ms after the restart was told so, and
 * gave up).
 */
export function mayPass(cause: unknown): boolean {
  if (cause instanceof ApiProblem) {
    const isNodeAway = cause.withCode("media_unreachable")?.detail?.reason === "node_away";
    return isNodeAway || passingRefusalCodes.has(cause.code);
  }
  return cause instanceof TypeError; // fetch's own failure: no answer at all
}

/**
 * Whether a request failed for hearing nothing from the core for silentAtMostMs over WebRTC
 * (SilentRequestError), the failure itself or what a path's naming of it wraps: its connection is
 * replaced then, so asked again it goes on a new one.
 */
export function isUnanswered(cause: unknown): boolean {
  for (let each: unknown = cause; each instanceof Error; each = each.cause) {
    if (each instanceof SilentRequestError) {
      return true;
    }
  }
  return false;
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

/**
 * How many times a request the core answered nothing for is asked in all (askWhileUnanswered): each
 * unanswered ask takes silentAtMostMs, 40 s, so three keep a page waiting two minutes at most before
 * it says it failed, and the second goes on a new connection, the silent one replaced.
 */
export const unansweredAsksAtMost = 3;

/**
 * What ask answers, asked again at once each time the core answered nothing over WebRTC
 * (isUnanswered), whose connection is replaced so the next ask goes on a new one, up to
 * unansweredAsksAtMost asks; each such failure is told to report, for the core's log. Any other
 * failure is thrown at once, as the last unanswered one is. A Play page whose playback the core
 * started but whose answer never came said "Loading…" for good, and its host's Watch Together player,
 * drawn only once the playback is, never attached (the owner's Chrome back from another site, staging,
 * 2026-10-29).
 */
export async function askWhileUnanswered<T>(
  ask: () => Promise<T>,
  report: (cause: unknown) => void,
): Promise<T> {
  for (let asked = 1; asked < unansweredAsksAtMost; asked += 1) {
    try {
      return await ask();
    } catch (cause) {
      if (!isUnanswered(cause)) {
        throw cause;
      }
      report(cause);
    }
  }
  return await ask();
}
