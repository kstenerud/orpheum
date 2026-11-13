// SPDX-License-Identifier: MPL-2.0

import { failureText, SilentRequestError, waitMs } from "@orpheum/sdk";

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
  "notAnswering",
  "rateLimited",
]);

/**
 * Whether a failure to reach the server may pass: no answer at all, a refusal that passes, or the
 * machine holding the film away, as it is for the seconds after the server restarts, until it is
 * heard again (the owner's test, 2026-10-18: a reopen asked 30 ms after the restart was told so, and
 * gave up).
 */
export function mayPass(cause: unknown): boolean {
  if (cause instanceof ApiProblem) {
    const isNodeAway = cause.withCode("mediaUnreachable")?.detail?.reason === "nodeAway";
    return isNodeAway || passingRefusalCodes.has(cause.code);
  }
  return cause instanceof TypeError; // fetch's own failure: no answer at all
}

/** The name a SilentRequestError of any copy of the SDK carries. */
const silentRequestErrorName = new SilentRequestError("").name;

/**
 * Whether a request failed for hearing nothing from the core for silentAtMostMs over WebRTC
 * (SilentRequestError), the failure itself or what a path's naming of it wraps: its connection is
 * replaced then, so asked again it goes on a new one. Known by its name too: under a discovery's
 * loader the request is carried, and fails, in the loader's own copy of the SDK, whose class is not
 * this copy's where the two are of different builds, as a client origin's loader and its core's
 * client are across a deploy of either.
 */
export function isUnanswered(cause: unknown): boolean {
  for (let each: unknown = cause; each instanceof Error; each = each.cause) {
    if (each instanceof SilentRequestError || each.name === silentRequestErrorName) {
      return true;
    }
  }
  return false;
}

/** Whether the server refused because it no longer has the playback: it restarted, or ended it. */
export function isPlaybackGone(cause: unknown): boolean {
  return cause instanceof ApiProblem && cause.code === "playbackGone";
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

/**
 * One attempt an ask makes to reach the server: how long it took, on the caller's clock, and what
 * happened, "ok" or a failure's words (failureText). For a playback's start timing (play.tsx,
 * play-start-timing.ts), which a slow one tells the core's log, each attempt and all (VQ137).
 */
export interface AskAttempt {
  readonly ms: number;
  readonly outcome: string;
}

/** What timedAsk tells of each attempt, and the clock it times them on; both optional. */
export interface AskTiming {
  readonly onAttempt?: (attempt: AskAttempt) => void;
  readonly nowMs?: () => number;
}

/** Runs ask once, timed on timing's clock, telling its onAttempt how long it took and what happened. */
async function timedAsk<T>(ask: () => Promise<T>, timing: AskTiming | undefined): Promise<T> {
  const nowMs = timing?.nowMs ?? (() => performance.now());
  const startedMs = nowMs();
  try {
    const result = await ask();
    timing?.onAttempt?.({ ms: Math.round(nowMs() - startedMs), outcome: "ok" });
    return result;
  } catch (cause) {
    timing?.onAttempt?.({ ms: Math.round(nowMs() - startedMs), outcome: failureText(cause) });
    throw cause;
  }
}

/**
 * What ask answers, asked again after each of reopenWaitsMs while it fails with what may pass; the
 * last failure, or the first that cannot pass, is thrown. Told timing's onAttempt, where given, how
 * long each try took and what it did, for a playback's start timing (play-start-timing.ts).
 */
export async function askUntilAnswered<T>(
  ask: () => Promise<T>,
  wait: (ms: number) => Promise<void> = waitMs,
  timing?: AskTiming,
): Promise<T> {
  for (const afterMs of reopenWaitsMs) {
    try {
      return await timedAsk(ask, timing);
    } catch (cause) {
      if (!mayPass(cause)) {
        throw cause;
      }
    }
    await wait(afterMs);
  }
  return await timedAsk(ask, timing);
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
  timing?: AskTiming,
): Promise<T> {
  for (let asked = 1; asked < unansweredAsksAtMost; asked += 1) {
    try {
      return await timedAsk(ask, timing);
    } catch (cause) {
      if (!isUnanswered(cause)) {
        throw cause;
      }
      report(cause);
    }
  }
  return await timedAsk(ask, timing);
}
