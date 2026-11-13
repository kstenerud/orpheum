// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { useCallback, useEffect, useRef, useState } from "react";

import type { AskOutcome, Poller } from "./polling.ts";

/**
 * Something loaded from the server: still loading, loaded, or failed with why. A failure keeps
 * its cause, which a screen tells the person in their words (problemText), and the console is
 * told it as it is (reportFailure); one a poller will ask again after says so (willAskAgain), so
 * the person is told they need not.
 */
export type Resource<T> =
  | { readonly state: "loading" }
  | { readonly state: "loaded"; readonly value: T }
  | { readonly state: "failed"; readonly cause: unknown; readonly willAskAgain?: boolean };

/**
 * A value another read already holds, which stands for a resource's first read (D476): given, or
 * still waited on ("waiting"), when that read has not answered yet.
 */
export type Given<T> = { readonly value: T } | "waiting";

/**
 * Loads a value when the component mounts and whenever key changes, and reloads when the returned
 * function is called. An answer that arrives after the key changed is dropped. A poll's read hands
 * load the ask's signal, which aborts when the poll gives it up (askAnswersWithinMs).
 *
 * Given a poller, it also asks again for as long as the component shows it, as the poller's
 * schedule says (D237): backgroundChangesPoller backs off while the answer is unchanged. It keeps what it shows meanwhile: a
 * reload does not go back to loading, so a view that follows something moving (the jobs, a scan's
 * films) does not flicker. A new key always starts from loading. This is the one way the client
 * polls the server; usePages polls a paged list the same way.
 *
 * Given a value another read holds (given), it shows that in place of its first read and asks
 * nothing then: a poller asks next at its first wait, and a reload reads. While that read is waited
 * on ("waiting"), it asks nothing and shows loading (D476: the shell's one read hands each part on).
 */
export function useResource<T>(
  key: string,
  load: (signal?: AbortSignal) => Promise<T>,
  poller?: Poller,
  given?: Given<T>,
): [Resource<T>, () => void] {
  const [held, setHeld] = useState<{ readonly key: string; readonly resource: Resource<T> }>({
    key,
    resource: { state: "loading" },
  });
  const [attempt, setAttempt] = useState(0);
  const givenState = given === undefined ? "none" : given === "waiting" ? "waiting" : "given";
  const givenValue = given === undefined || given === "waiting" ? undefined : given;
  const givenRef = useRef(givenValue);
  givenRef.current = givenValue;
  // The key and attempt the given value stood for: a reload, or a new key, reads.
  const givenFor = useRef<{ readonly key: string; readonly attempt: number } | undefined>(
    undefined,
  );
  useEffect(() => {
    if (givenState === "waiting") {
      return undefined;
    }
    let isCurrent = true;
    let lastAnswer: string | undefined;
    const isFirstGiven =
      givenRef.current !== undefined &&
      (givenFor.current === undefined ||
        (givenFor.current.key === key && givenFor.current.attempt === attempt));
    // One read; answered, when polling, hears what it came to. A read its poll gave up is dropped,
    // its failure already reported, so the screen keeps what it shows.
    const read = (answered?: (outcome: AskOutcome) => void, signal?: AbortSignal): void => {
      load(signal).then(
        (value) => {
          if (signal?.aborted === true) {
            return;
          }
          const answer = JSON.stringify(value);
          const isChanged = answer !== lastAnswer;
          lastAnswer = answer;
          if (isCurrent && isChanged) {
            setHeld({ key, resource: { state: "loaded", value } });
          }
          answered?.(isChanged ? "changed" : "unchanged");
        },
        (cause: unknown) => {
          if (signal?.aborted === true) {
            return;
          }
          reportFailure(cause);
          lastAnswer = undefined;
          if (isCurrent) {
            const asked = poller === undefined ? {} : { willAskAgain: true };
            setHeld({ key, resource: { state: "failed", cause, ...asked } });
          }
          answered?.("failed");
        },
      );
    };
    if (isFirstGiven && givenRef.current !== undefined) {
      const { value } = givenRef.current;
      givenFor.current = { key, attempt };
      lastAnswer = JSON.stringify(value);
      setHeld({ key, resource: { state: "loaded", value } });
      const stopPolling = poller?.start(read);
      return () => {
        isCurrent = false;
        stopPolling?.();
      };
    }
    if (poller === undefined) {
      setHeld({ key, resource: { state: "loading" } });
      read();
      return () => {
        isCurrent = false;
      };
    }
    read();
    const stopPolling = poller.start(read);
    return () => {
      isCurrent = false;
      stopPolling();
    };
  }, [key, attempt, poller, givenState]);
  const reload = useCallback(() => {
    setAttempt((count) => count + 1);
  }, []);
  const resource: Resource<T> = held.key === key ? held.resource : { state: "loading" };
  // Before its effect has run, a given value shows at once, not loading for a frame.
  if (resource.state === "loading" && givenValue !== undefined && givenFor.current === undefined) {
    return [{ state: "loaded", value: givenValue.value }, reload];
  }
  return [resource, reload];
}
