// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { createContext, use } from "react";

/**
 * How a screen asks the server again (D237): after firstMs, then, while the answer is unchanged,
 * after twice the last wait, up to longestMs; after firstMs again once it changes.
 */
export interface PollWaits {
  readonly firstMs: number;
  readonly longestMs: number;
  /** The wait after a read that failed, every time. */
  readonly failedMs: number;
}

/** What one ask of a poll came to: what it read changed what is shown, or did not, or it failed. */
export type AskOutcome = "changed" | "unchanged" | "failed";

/**
 * One ask of a poll: it reads, and tells answered what that came to. signal aborts when the poll
 * gives the ask up (askAnswersWithinMs), so a read that honours it stops, and what it answers after
 * is no longer heard; a test's hand poller gives none.
 */
export type Ask = (answered: (outcome: AskOutcome) => void, signal?: AbortSignal) => void;

/** What polls for a screen: start runs ask as its schedule says until the returned stop. */
export interface Poller {
  readonly start: (ask: Ask) => () => void;
}

/** A poller on polling's schedule, backing off while nothing changes (pollUntilStopped). */
export function backingOffPoller(
  polling: PollWaits,
  answersWithinMs: number = askAnswersWithinMs,
): Poller {
  return { start: (ask) => pollUntilStopped(polling, ask, answersWithinMs) };
}

/** A backing-off poller whose waits change, and how to change them. */
export interface ChangingWaitsPoller {
  readonly poller: Poller;
  /** Changes the waits: each poll waits by them from its next wait on. */
  readonly setWaits: (polling: PollWaits) => void;
}

/**
 * A backing-off poller on polling's schedule until setWaits changes it, when each poll it runs
 * takes the new waits from its next wait on and goes on: the server's client settings, once read.
 * One poller, so its screens are not started again: a new poller read every polled screen again at
 * once, so each was read twice as the page started (the audit of 2026-10-25, W3).
 */
export function changingWaitsPoller(
  polling: PollWaits,
  answersWithinMs: number = askAnswersWithinMs,
): ChangingWaitsPoller {
  let waits = polling;
  return {
    poller: { start: (ask) => pollUntilStopped(() => waits, ask, answersWithinMs) },
    setWaits: (changed) => {
      waits = changed;
    },
  };
}

/**
 * How long a poll waits for one ask to answer before it gives the ask up as failed and asks again
 * after the failed wait. The client's requests have no timeout of their own, and a poll asks again
 * only once the last ask answered, so one request that never answered (over the discovery's WebRTC
 * transport on a TV, staging, 2026-10-24) stopped a page's refresh for good. A fixed safety bound,
 * not a setting: twice the longest wait D237 starts with, and over twice the slowest answer staging
 * has seen, a realm sign-in's two connections at 28 s (2026-10-23), so an ask still answering is
 * never given up.
 */
export const askAnswersWithinMs = 60_000;

/**
 * The waits of a screen showing what the server changes in the background, 2 s to 30 s, and 5 s
 * after a read that failed (D237 §1):
 * the settings' starting values, which the server's client settings replace once read. The core's
 * TestRegistry_TheWebClientsWaitsBeforeItReadsThemAreTheScreenRefreshSettingsStarts holds them equal.
 */
export const backgroundChangeWaits: PollWaits = {
  firstMs: 2000,
  longestMs: 30_000,
  failedMs: 5000,
};

/**
 * The poller of a screen showing what the server changes in the background, a scan,
 * identification, artwork, the jobs (D237 §1).
 */
export const backgroundChangesPoller: Poller = backingOffPoller(backgroundChangeWaits);

/**
 * The wait before the next ask, after lastWaitMs and what the last ask came to. Only an unchanged
 * answer backs off: a failure is no answer, and a screen open while the server restarts asks again
 * after the failed wait each time, so it shows the server's answer soon after it is up (D237).
 */
export function nextWaitMs(polling: PollWaits, lastWaitMs: number, outcome: AskOutcome): number {
  if (outcome === "failed") {
    return polling.failedMs;
  }
  if (outcome === "changed") {
    return polling.firstMs;
  }
  return Math.min(lastWaitMs * 2, polling.longestMs);
}

/**
 * Asks again and again as polling says (read again for each wait, when a function), until the
 * returned function stops it: the first ask after polling.firstMs, and each after the wait nextWaitMs gives once the last ask said, through
 * answered, what it came to. An ask that has not answered within answersWithinMs is failed: its
 * signal aborts, the failure is reported (reportFailure), and what it answers later is not heard.
 */
export function pollUntilStopped(
  polling: PollWaits | (() => PollWaits),
  ask: Ask,
  answersWithinMs: number = askAnswersWithinMs,
): () => void {
  const waits = typeof polling === "function" ? polling : (): PollWaits => polling;
  let isStopped = false;
  let waitMs = waits().firstMs;
  let timer = setTimeout(askNow, waitMs);
  function askNow(): void {
    const aborter = new AbortController();
    let isAnswered = false;
    const answer = (outcome: AskOutcome): void => {
      if (isStopped || isAnswered) {
        return;
      }
      isAnswered = true;
      waitMs = nextWaitMs(waits(), waitMs, outcome);
      timer = setTimeout(askNow, waitMs);
    };
    timer = setTimeout(() => {
      const givenUp = new Error(
        `a background refresh had no answer within ${String(answersWithinMs / 1000)} s; it was given up and is asked again`,
      );
      givenUp.name = "TimeoutError";
      answer("failed");
      aborter.abort(givenUp);
      reportFailure(givenUp);
    }, answersWithinMs);
    const timeout = timer;
    ask((outcome) => {
      clearTimeout(timeout);
      answer(outcome);
    }, aborter.signal);
  }
  return () => {
    isStopped = true;
    clearTimeout(timer);
  };
}

/**
 * The waits the server's client settings say, in seconds, as a screen waits them; D237's starting
 * waits before they are read. A longest wait shorter than the first is the first.
 */
export function clientWaits(
  read:
    | {
        readonly firstSeconds: number;
        readonly longestSeconds: number;
        readonly failedSeconds: number;
      }
    | undefined,
): PollWaits {
  if (read === undefined) {
    return backgroundChangeWaits;
  }
  return {
    firstMs: read.firstSeconds * 1000,
    longestMs: Math.max(read.firstSeconds, read.longestSeconds) * 1000,
    failedMs: read.failedSeconds * 1000,
  };
}

/**
 * The poller screens showing what the server changes in the background use: backgroundChangesPoller until
 * the server's client settings say the waits an admin set (screenRefreshFirstWaitSeconds,
 * screenRefreshLongestWaitSeconds and screenRefreshFailedWaitSeconds).
 */
export const BackgroundChangesContext = createContext<Poller>(backgroundChangesPoller);

/** The poller of a screen showing what the server changes in the background (D237). */
export function useBackgroundChangesPoller(): Poller {
  return use(BackgroundChangesContext);
}
