// SPDX-License-Identifier: MPL-2.0

import { ApiProblem, reportFailure } from "@orpheum/sdk";
import { useCallback, useEffect, useRef, useState } from "react";

/** How long a request runs before a screen says it is still waiting (D231 §12). */
export const slowAfterMs = 2000;

/** A request a screen runs, and how it is going: running, or failed with why. */
export interface RequestState {
  readonly isBusy: boolean;
  /** Why the last request failed, in the person's words; undefined while one runs or it worked. */
  readonly problem: string | undefined;
  /** Whether the running request has taken longer than slowAfterMs: a host slow, or our own pacing. */
  readonly isSlow: boolean;
  /**
   * Seconds until a request a host asked us to wait for runs again on its own (a 429's
   * Retry-After); undefined when none waits.
   */
  readonly retryInSeconds: number | undefined;
  /** Whether the running request follows a host not answering: it is that host's check. */
  readonly isChecking: boolean;
  /**
   * Runs a request. Given as a function, a request a host asks us to wait for runs again on its
   * own once the wait is over; given as a promise, it fails with why. The same function every time.
   */
  readonly runRequest: (request: Promise<void> | (() => Promise<void>)) => void;
}

/** How long a refusal asks us to wait before asking again, or undefined when it names no time. */
function retryAfterMsOf(cause: unknown): number | undefined {
  return cause instanceof ApiProblem
    ? cause.withCode("rate_limited")?.detail?.retryAfterMs
    : undefined;
}

/** Whether a refusal is an outside host not answering. */
function isNotAnswering(cause: unknown): boolean {
  return (
    cause instanceof ApiProblem &&
    cause.withCode("provider_error")?.detail?.reason === "unavailable"
  );
}

/** Timers a hook sets, all cleared at once, and when the component goes. */
function useTimers(): {
  readonly after: (ms: number, then: () => void) => void;
  readonly clear: () => void;
} {
  const timers = useRef<ReturnType<typeof setTimeout>[]>([]);
  const clear = useCallback((): void => {
    for (const timer of timers.current) {
      clearTimeout(timer);
    }
    timers.current = [];
  }, []);
  useEffect(() => clear, [clear]);
  const after = useCallback((ms: number, then: () => void): void => {
    timers.current.push(setTimeout(then, ms));
  }, []);
  return { after, clear };
}

/**
 * One request at a time: whether it is running, and why it failed when it did, in the words
 * describeFailure gives the cause; the cause itself goes to the console (reportFailure). Every time
 * a person's request is held back they are told why and until when (D231 §12): a request running
 * long says so, one a host asked us to wait for counts down and goes again on its own, and one run
 * after a host was found not answering is that host's check.
 */
export function useRequest(describeFailure: (cause: unknown) => string): RequestState {
  const [isBusy, setIsBusy] = useState(false);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const [isSlow, setIsSlow] = useState(false);
  const [retryInSeconds, setRetryInSeconds] = useState<number | undefined>(undefined);
  const [isChecking, setIsChecking] = useState(false);
  const wasNotAnswering = useRef(false);
  const describe = useRef(describeFailure);
  describe.current = describeFailure;
  const { after, clear } = useTimers();
  const runRequest = useCallback(
    (request: Promise<void> | (() => Promise<void>)): void => {
      const countDown = (endsAt: number, again: () => Promise<void>): void => {
        const leftMs = endsAt - Date.now();
        setRetryInSeconds(leftMs <= 0 ? undefined : Math.ceil(leftMs / 1000));
        if (leftMs <= 0) {
          runRequest(again);
          return;
        }
        after(Math.min(1000, leftMs), () => {
          countDown(endsAt, again);
        });
      };
      clear();
      setIsBusy(true);
      setProblem(undefined);
      setRetryInSeconds(undefined);
      setIsChecking(wasNotAnswering.current);
      after(slowAfterMs, () => {
        setIsSlow(true);
      });
      const settle = (cause?: unknown): void => {
        clear();
        setIsBusy(false);
        setIsSlow(false);
        setIsChecking(false);
        wasNotAnswering.current = isNotAnswering(cause);
      };
      (typeof request === "function" ? request() : request).then(settle, (cause: unknown) => {
        reportFailure(cause);
        settle(cause);
        const waitMs = retryAfterMsOf(cause);
        if (waitMs !== undefined && typeof request === "function") {
          countDown(Date.now() + waitMs, request);
          return;
        }
        setProblem(describe.current(cause));
      });
    },
    [after, clear],
  );
  return { isBusy, problem, isSlow, retryInSeconds, isChecking, runRequest };
}
