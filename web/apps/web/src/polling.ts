// SPDX-License-Identifier: MPL-2.0

import { createContext, use } from "react";

/**
 * How a screen asks the server again (D237): after firstMs, then, while the answer is unchanged,
 * after twice the last wait, up to longestMs; after firstMs again once it changes.
 */
export interface PollWaits {
  readonly firstMs: number;
  readonly longestMs: number;
}

/** What one ask of a poll came to: what it read changed what is shown, or did not, or it failed. */
export type AskOutcome = "changed" | "unchanged" | "failed";

/** One ask of a poll: it reads, and tells answered what that came to. */
export type Ask = (answered: (outcome: AskOutcome) => void) => void;

/** What polls for a screen: start runs ask as its schedule says until the returned stop. */
export interface Poller {
  readonly start: (ask: Ask) => () => void;
}

/** A poller on polling's schedule, backing off while nothing changes (pollUntilStopped). */
export function backingOffPoller(polling: PollWaits): Poller {
  return { start: (ask) => pollUntilStopped(polling, ask) };
}

/**
 * The waits of a screen showing what the server changes in the background, 2 s to 30 s (D237 §1):
 * the settings' starting values, which the server's client settings replace once read. The core's
 * TestRegistry_TheWebClientsWaitsBeforeItReadsThemAreTheScreenRefreshSettingsStarts holds them equal.
 */
export const backgroundChangeWaits: PollWaits = { firstMs: 2000, longestMs: 30_000 };

/**
 * The poller of a screen showing what the server changes in the background, a scan,
 * identification, artwork, the jobs (D237 §1).
 */
export const backgroundChangesPoller: Poller = backingOffPoller(backgroundChangeWaits);

/**
 * The wait before the next ask, after lastWaitMs and what the last ask came to. Only an unchanged
 * answer backs off: a failure is no answer, and a screen open while the server restarts asks again
 * soon, so it shows the server's answer within the first wait of it being up (2026-10-06).
 */
export function nextWaitMs(polling: PollWaits, lastWaitMs: number, outcome: AskOutcome): number {
  if (outcome !== "unchanged") {
    return polling.firstMs;
  }
  return Math.min(lastWaitMs * 2, polling.longestMs);
}

/**
 * Asks again and again as polling says, until the returned function stops it: the first ask after
 * polling.firstMs, and each after the wait nextWaitMs gives once the last ask said, through
 * answered, what it came to.
 */
export function pollUntilStopped(polling: PollWaits, ask: Ask): () => void {
  let isStopped = false;
  let waitMs = polling.firstMs;
  let timer = setTimeout(askNow, waitMs);
  function askNow(): void {
    ask((outcome) => {
      if (isStopped) {
        return;
      }
      waitMs = nextWaitMs(polling, waitMs, outcome);
      timer = setTimeout(askNow, waitMs);
    });
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
  read: { readonly firstSeconds: number; readonly longestSeconds: number } | undefined,
): PollWaits {
  if (read === undefined) {
    return backgroundChangeWaits;
  }
  return {
    firstMs: read.firstSeconds * 1000,
    longestMs: Math.max(read.firstSeconds, read.longestSeconds) * 1000,
  };
}

/**
 * The poller screens showing what the server changes in the background use: backgroundChangesPoller until
 * the server's client settings say the waits an admin set (screen_refresh_first_wait_seconds and
 * screen_refresh_longest_wait_seconds).
 */
export const BackgroundChangesContext = createContext<Poller>(backgroundChangesPoller);

/** The poller of a screen showing what the server changes in the background (D237). */
export function useBackgroundChangesPoller(): Poller {
  return use(BackgroundChangesContext);
}
