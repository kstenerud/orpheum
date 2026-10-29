// SPDX-License-Identifier: MPL-2.0

/**
 * Timing for the Play page's playback request and the steps before its player starts: the decision
 * asked of the server (watching.ts's decide, play.tsx's useOpenedPlayback) and each re-ask
 * (reopen.ts's askUntilAnswered and askWhileUnanswered). The owner's Chrome, on staging, clicked a
 * Continue-watching tile at 21:14:00 UTC and saw "Loading…" for near a minute with nothing told
 * the core's log at all, until the playback started at 21:14:50 (VQ137, 2026-10-29): no report of a
 * newer-client reload, no screen-load failure, no silent-request report. This times every attempt
 * from the first ask, and tells the core's log, once the total is slow, each attempt's time and
 * outcome, and what carries the request now.
 */

import { apiPrefix, mediaCarrierOf, reportFailure, type PlayableFormats } from "@orpheum/sdk";

import { askUntilAnswered, askWhileUnanswered, type AskAttempt, type ReopenAt } from "./reopen.ts";
import type { PlaybackDecision, StartCause, Watching } from "./watching.ts";

/** A decision for the Play page, and whether its film waits to be played, as one opened again. */
export interface Opened {
  readonly decision: PlaybackDecision;
  readonly isStartPaused?: boolean;
}

/** The decision opened from at, where its video stood, in place of its resume position. */
export function openedAt(decision: PlaybackDecision, at: ReopenAt): Opened {
  return {
    decision: { ...decision, resumePositionMs: Math.round(at.positionMs) },
    isStartPaused: at.isPaused,
  };
}

/** Why a playback is opened again: the server lost it, its player gave the film's own picture up, or
 * its film could not be read and can again. */
export type ReopenCause = "playbackGone" | "ownPictureGivenUp" | "mediaBack";

/**
 * A playback opened again from where it was: the server lost it, or its player gave the film's own
 * picture up and is offered the twin in its place (D323 §2).
 */
export interface Reopening {
  readonly whereNow: () => ReopenAt;
  readonly cause: ReopenCause;
}

/**
 * How long the Play page's playback may take to open before it tells the core's log where the time
 * went: far past any ordinary decision, which answers in well under a second even over a slow
 * link's first round trip, and far short of a person wondering why nothing happened, let alone the
 * owner's near-minute wait with nothing told (VQ137). A justified constant, not a setting.
 */
export const playbackStartSlowAfterMs = 3000;

/** One attempt's line for the report: its number from 1, how long it took, and what happened. */
function attemptLine(attempt: AskAttempt, index: number): string {
  return `attempt ${String(index + 1)}: ${String(attempt.ms)} ms (${attempt.outcome})`;
}

/**
 * Collects each attempt the Play page's playback makes to open, timed from now on nowMs, and, once
 * finish() is called, tells report where the time went when the total reaches
 * playbackStartSlowAfterMs: the file, the total, each attempt's line, and carrierOf() read at that
 * moment. Tells nothing for an ordinary, fast one. onAttempt is passed to askUntilAnswered and
 * askWhileUnanswered (reopen.ts) as their own telemetry hook.
 */
export function timePlaybackStart(parts: {
  readonly fileId: number;
  readonly carrierOf: () => string;
  readonly report: (text: string) => void;
  readonly nowMs?: () => number;
}): { readonly onAttempt: (attempt: AskAttempt) => void; readonly finish: () => void } {
  const nowMs = parts.nowMs ?? (() => performance.now());
  const startedMs = nowMs();
  const attempts: AskAttempt[] = [];
  return {
    onAttempt: (attempt) => {
      attempts.push(attempt);
    },
    finish: () => {
      const totalMs = Math.round(nowMs() - startedMs);
      if (totalMs < playbackStartSlowAfterMs) {
        return;
      }
      const lines = attempts.map((attempt, index) => attemptLine(attempt, index)).join("; ");
      parts.report(
        `the Play page's playback for file ${String(parts.fileId)} took ${String(totalMs)} ms ` +
          `to start, carried ${parts.carrierOf()}: ${lines}`,
      );
    },
  };
}

/**
 * The playback request's own address, for mediaCarrierOf to say what carries it now: an exact
 * match, where the transport in use tracked this very URL, answers with its connection's own
 * detail; otherwise, still "http" or the path's name, never nothing (media-carrier.ts).
 */
function playbackRequestUrl(): string {
  return new URL(`${apiPrefix}/playback`, window.location.origin).toString();
}

/**
 * Opens the Play page's playback, timed from here (VQ137): the first ask, asked again while the
 * core answers nothing (askWhileUnanswered), or, where reopen is given, a lost one opened again,
 * asked for about a minute (askUntilAnswered). askedAt starts the decision there in place of the
 * resume position answered; reopen's own whereNow does the same once it answers.
 */
export async function openPlayback(parts: {
  readonly watching: Watching;
  readonly fileId: number;
  readonly playableFormats: PlayableFormats;
  readonly cause: StartCause;
  readonly askedAt: ReopenAt | undefined;
  readonly reopen: Reopening | undefined;
}): Promise<Opened> {
  const { watching, fileId, playableFormats, cause, askedAt, reopen } = parts;
  const timing = timePlaybackStart({
    fileId,
    carrierOf: () => mediaCarrierOf(playbackRequestUrl()),
    report: reportFailure,
  });
  try {
    if (reopen === undefined) {
      // Asked again on a new connection when the core answered nothing over WebRTC (the owner's
      // Chrome back from another site, staging, 2026-10-29: "Loading…" for good).
      const decision = await askWhileUnanswered(
        async () =>
          await watching.decide(fileId, playableFormats, {
            cause,
            startsAtMs: askedAt?.positionMs,
          }),
        reportFailure,
        { onAttempt: timing.onAttempt },
      );
      return askedAt === undefined ? { decision } : openedAt(decision, askedAt);
    }
    // The server may still be starting: asked again for about a minute.
    const decision = await askUntilAnswered(
      () => watching.decide(fileId, playableFormats, { cause: reopen.cause }),
      undefined,
      { onAttempt: timing.onAttempt },
    );
    return openedAt(decision, reopen.whereNow());
  } finally {
    timing.finish();
  }
}
