// SPDX-License-Identifier: MPL-2.0

/** A value written once asks for it settle: the latest only, one write at a time. */
export interface LatestWrite<T> {
  /** Asks for value to be written once no newer ask has come for the wait. */
  readonly write: (value: T) => void;
  /** Writes what waits now, without waiting longer: as the page leaves what asked. */
  readonly flush: () => void;
}

/** What a latest write is told of each write it sends: kept, or failed with why. */
export interface WriteHeard {
  readonly kept: () => void;
  readonly failed: (cause: unknown) => void;
}

/**
 * Writes by send the latest value asked for, waitMs after the last ask, and never two at once: a
 * value asked for while one is being written waits for it, so the server keeps the last asked for
 * even where a later request can overtake an earlier one (over the relay each request has its own
 * channel). Each write sent is told to heard, kept or failed.
 */
export function latestWrite<T>(
  send: (value: T) => Promise<void>,
  waitMs: number,
  heard: WriteHeard,
): LatestWrite<T> {
  let waiting: { readonly value: T } | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let isSending = false;
  const sendWaiting = (): void => {
    if (isSending || waiting === undefined) {
      return;
    }
    const { value } = waiting;
    waiting = undefined;
    isSending = true;
    send(value)
      .then(heard.kept, heard.failed)
      .finally(() => {
        isSending = false;
        // One asked for meanwhile goes at once, unless its own wait still runs.
        if (timer === undefined) {
          sendWaiting();
        }
      });
  };
  const flush = (): void => {
    clearTimeout(timer);
    timer = undefined;
    sendWaiting();
  };
  return {
    write: (value) => {
      waiting = { value };
      clearTimeout(timer);
      timer = setTimeout(flush, waitMs);
    },
    flush,
  };
}
