// SPDX-License-Identifier: MPL-2.0

/**
 * How a screen asks the server again (D237): after firstMs, then, while the answer is unchanged,
 * after twice the last wait, up to longestMs; after firstMs again once it changes.
 */
export interface Polling {
  readonly firstMs: number;
  readonly longestMs: number;
}

/** One ask of a poll: it reads, and tells answered whether what it read changed what is shown. */
export type Ask = (answered: (isChanged: boolean) => void) => void;

/** What polls for a screen: start runs ask as its schedule says until the returned stop. */
export interface Poller {
  readonly start: (ask: Ask) => () => void;
}

/** A poller on polling's schedule, backing off while nothing changes (pollUntilStopped). */
export function backingOff(polling: Polling): Poller {
  return { start: (ask) => pollUntilStopped(polling, ask) };
}

/** The waits of a screen showing what the server changes in the background: 2 s to 30 s (D237 §1). */
export const backgroundChangeWaits: Polling = { firstMs: 2000, longestMs: 30_000 };

/**
 * The poller of a screen showing what the server changes in the background, a scan,
 * identification, artwork, the jobs (D237 §1).
 */
export const backgroundChanges: Poller = backingOff(backgroundChangeWaits);

/** The wait before the next ask, after lastWaitMs and an answer that changed or did not. */
export function nextWaitMs(polling: Polling, lastWaitMs: number, isChanged: boolean): number {
  if (isChanged) {
    return polling.firstMs;
  }
  return Math.min(lastWaitMs * 2, polling.longestMs);
}

/**
 * Asks again and again as polling says, until the returned function stops it: the first ask after
 * polling.firstMs, and each after the wait nextWaitMs gives once the last ask said, through
 * answered, whether what it read changed.
 */
export function pollUntilStopped(polling: Polling, ask: Ask): () => void {
  let isStopped = false;
  let waitMs = polling.firstMs;
  let timer = setTimeout(askNow, waitMs);
  function askNow(): void {
    ask((isChanged) => {
      if (isStopped) {
        return;
      }
      waitMs = nextWaitMs(polling, waitMs, isChanged);
      timer = setTimeout(askNow, waitMs);
    });
  }
  return () => {
    isStopped = true;
    clearTimeout(timer);
  };
}
